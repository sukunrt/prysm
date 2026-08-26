# Plan: spamoor as a kurtosis additional_service

## Root cause

Spamoor funds its derived child wallets through a batcher contract whose funding transaction is
sized without looking at the chain's block gas limit: v1.1.17 packs up to 450 recipients at a
legacy 35k gas each (spamoor/txbatcher.go, `BatcherTxLimit`/`BatcherGasPerTx`), so the outer tx can
carry up to 15.8M gas and the 9.0M-limit EL rejects it ("exceeds block gas limit"). v1.1.17 is
also pre-Amsterdam-aware: even a batch that fit would under-budget first-touch transfers, which
cost 21000 + 120*1530 = 204,600 gas under EIP-8037 (CPSB=1530, EIPS/eip-8037.md), not 35k; the
unfunded wallet then panics spamoor (walletpool.go:636, nil big.Int in Cmp).

## What spamoor offers today (hypothesis: knob exists — yes)

Checked at master (2026-08-10) and tag v1.2.3; clone in scratch dir.

- `--without-batcher` (cmd/spamoor-daemon/main.go:63): skips the batcher contract entirely; each
  child gets its own funding tx. Exists since before v1.1.17.
- Amsterdam-aware funding gas landed after v1.1.17, first released in v1.2.0
  (commits fe8d980 "rework 8073 gas calculation", 9daa14f "remove amsterdam detection"):
  `WalletPool.FundingGasFor` (spamoor/walletpool.go:308) sizes a direct funding tx as
  21000 + 120*cpsb = 204,600 for an empty target — exactly the on-chain cost, well under 9M.
  So on v1.2.x, `--without-batcher` alone makes funding work at a 9M limit.
- Per-spammer YAML config (daemon merges it into `WalletPoolConfig`, walletpool.go:42): `seed`,
  `wallet_count`, `refill_amount`, `refill_balance`, `refill_interval`, `funding_gas_limit`.
  The ethereum-package passes each spammer's `config` dict through verbatim
  (src/spamoor/spamoor.star, startup-spammer.yaml.tmpl), so all of these are reachable.
- Skip-funding-when-premined is implicit, not a flag: `prepareWallet` (walletpool.go) only emits a
  funding request when balance < `refill_balance`, so premined children above the threshold cause
  zero funding txs and no batcher deploy.
- Still missing upstream: `packFundingBatches` (walletpool.go:387) caps batches at a constant
  `BatcherRPCGasCap` = 16M (txbatcher.go:108), never at the chain's block gas limit. With the
  batcher on, a 9M chain still gets oversized batch txs even on master. The small PR, if wanted:
  clamp the cap to min(BatcherRPCGasCap, TxPool.MaxTxGas()). Not needed for us.

Verdict: the hunch is right. Two knobs together — image >= v1.2.0 and `--without-batcher` — make
this a config-only fix. No spamoor code change required.

## Recommended fix

In kurtosis/network_params.yaml:

```yaml
additional_services:
  - spamoor
spamoor_params:
  image: ethpandaops/spamoor:v1.2.3     # exists on Docker Hub; first fixed release is v1.2.0
  extra_args:
    - --without-batcher
  start_chainload: false                 # keep traffic identical to the Shadow arms, no extras
  spammers:
    - name: eoatx
      scenario: eoatx
      config: {throughput: 8, max_pending: 16, max_wallets: 40, data: <16KiB hex>}
    - name: blobs-public
      scenario: blobs
      config: {throughput: 2, sidecars: 3, max_pending: 6, max_wallets: 10,
               client_group: <meshed group>}
    - name: blobs-private
      scenario: blobs
      config: {throughput: 2, sidecars: 3, max_pending: 6, max_wallets: 10,
               client_group: <isolated group>}
```

Notes: setting `spamoor_params.spammers` replaces the package defaults (input_parser.star:204).
`client_group` selects RPC hosts by the groups the package writes into rpc-hosts.txt
(spamoor.star, `new_hosts_template_data`) — pin the private blob arm to the isolated ELs there.
Funding cost: 60 wallets x 204,600 = ~12.3M gas, two blocks; root (prefunded_addresses[13],
0x4d1CB4eB..., 1e9 ETH in genesis) covers 5 ETH/wallet defaults easily.

## Rejected alternatives

- Premine children via `network_params.prefunded_accounts`: works (flows to `EL_PREMINE_ADDRS` in
  values.env.tmpl; the patched decoupled generator's el-gen consumes `el_premine_addrs`), and the
  derivation is deterministic — child_priv = sha256(root_priv || be_uint64(idx) || seed),
  walletpool.go `prepareChildWallet` — but only if every spammer pins `seed:` in its config
  (default seed is `<scenario>-<random int>`, GetDefaultWalletConfig). Brittle against wallet-count
  sizing rules and misses well-known wallets some scenarios add; keep as fallback only.
- Raise `genesis_gaslimit` above 9M: would fit v1.1.17's 15.8M batch, but 9M is the Shadow
  128KB-block parity value; changing it forfeits comparability. Option, not recommended.
- Per-spammer `funding_gas_limit`: tunes per-recipient gas, cannot shrink the 16M batch cap.
- Patch spamoor (clamp batch cap to block gas limit): correct upstream PR, unnecessary here.

## Verification

1. Copy network_params.yaml with the block above; `kurtosis run --enclave decoupled-spam ...`.
2. `kurtosis service logs decoupled-spam spamoor`: expect "initialized N child wallets" and
   "funding child wallets... (N/N)" per spammer; no "exceeds block gas limit", no panic.
3. EL RPC: gasUsed of the funding blocks ~204,600 per funding tx; then steady 8 tx/slot eoatx
   and 6 blobs/slot; isolated-EL blob txs appear only in their own proposers' blocks.
4. Confirm no refill storm at `refill_interval` (default 600s): balances stay above threshold.

## blobsend: keep or delete

Verdict: delete. Spamoor covers everything blobsend did; nothing consumes its output.

- No pipeline reads it. Nothing in the repo imports or invokes `kurtosis/blobsend` except its own
  docs (kurtosis/README.md:47, kurtosis/HANDOFF.md:124). elscan.py measures blob gas from the
  chain, not from blobsend's receipt log; summarize.py/vclogs.py never mention it. The only other
  mentions are historical comments in ~/dev/prysm2-run-logs (outside the repo).
- Capabilities are matched. Deterministic N blobs/slot: `throughput: 2, sidecars: 3`. Cell-proof
  sidecars: the blobs scenario sends v1 sidecars. The two-account subpool trick: each spamoor arm
  derives its own child wallets from its own seed, so blob and calldata senders are disjoint.
- The one edge it had — `go run` against an already-running enclave with zero config — is nearly
  gone: the committed args files always start the three spamoor arms, so a bare enclave only
  exists if someone strips `additional_services`. Ad-hoc spamoor is still possible then
  (`docker run ethpandaops/spamoor:v1.2.3 spamoor-daemon --without-batcher --privkey ...
  --rpchost ...`; the one-shot `spamoor` CLI lacks `--without-batcher`, so use the daemon or keep
  wallet counts under ~40).
- Cost of keeping: 180 lines that hardcode Amsterdam gas prices (must track repricings), a
  dependency on testing/endtoend/components/eth1 internals, and a second traffic tool for
  ethpandaops to read in the handoff. go.mod is unaffected either way (go-ethereum is used
  throughout).

Removal: delete `kurtosis/blobsend/` (main.go, BUILD.bazel); drop README.md's "Drive the
execution layer" section (lines 44-61) in favor of a pointer to the spamoor arms; drop the
"quick run without spamoor" paragraph in HANDOFF.md (lines 120-125). `eth1.New4844CellTx` stays —
the e2e tests use it.
