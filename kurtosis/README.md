# kurtosis harness for the decoupled fork

A second, non-simulated witness for the same topology ethshadow measures:
real wall clock, real docker networking, same binaries.

## Build

```sh
./kurtosis/build-images.sh
```

Builds `beacon-chain`, `validator` and `prysmctl` with `go build`
(statically linked: CGO for blst, `netgo,osusergo` so nothing reaches for
glibc's NSS at runtime), then three images:

| image | contents |
| --- | --- |
| `prysm-beacon-chain:local` | the beacon node at `/beacon-chain` |
| `prysm-validator:local` | the validator client, behind a shim entrypoint |
| `prysm-genesis-gen:local` | patched `ethereum-genesis-generator` |

Each is also tagged with the jj change id of the working copy
(`prysm-beacon-chain:yssuxmntpnsm`, ...), so a run can be traced back to the
tree it came from. Nothing is pushed; kurtosis reads the local daemon.

The static link is what lets an Arch host (glibc 2.44) produce binaries a
`debian:bookworm-slim` runtime (glibc 2.36) can execute. The binary paths are
not free: ethereum-package's prysm launcher hardcodes `/beacon-chain` and
runs it under `sh -c`, and the VC launcher relies on the image ENTRYPOINT.

## Run

```sh
kurtosis run --enclave decoupled ~/dev/ethereum-package \
    --args-file kurtosis/network_params.yaml
kurtosis enclave inspect decoupled
kurtosis service logs decoupled cl-1-prysm-geth --follow
```

`network_params.yaml` is the 2-node shakeout. For a measurement run raise
`participants[0].count` and lower `num_validator_keys_per_node` to keep the
total near 128.

## Drive the execution layer

```sh
go run ./kurtosis/blobsend -rpc http://127.0.0.1:<el-rpc> -interval 6s
```

A run on empty payloads has no data columns to carry, so the supernodes have
nothing to custody or serve. `blobsend` sends one blob transaction and one
value transfer per slot from two of the package's prefunded dev accounts
(separate accounts because geth pins a sender to one txpool subpool), waits
for each receipt and logs the block, gas and blob gas. Sidecars carry cell
proofs, which is what a Fulu txpool accepts.

Every transaction carries a 500k gas limit. Amsterdam reprices state access --
a bare transfer costs ~207k on first touch, not 21000 -- and a tool that
hardcodes the pre-Amsterdam cost has its funding transfers die out-of-gas.
That is what left run 02 with no blob transactions and therefore no columns
at all.

## Measure

```sh
kurtosis/scrape.sh <enclave> <outdir> 6      # one metrics sample per slot
kurtosis/summarize.py <outdir> --slot-seconds 6 --skip-slots 32
kurtosis/vclogs.py <outdir>                  # needs docker logs vc-* > vc-*.log
kurtosis/elscan.py <el-rpc-url>              # blob gas per execution block
```

`scrape.sh` polls every beacon node's `/metrics` and keeps only the families
the measurements use; `summarize.py` differences the counters over the window
and prints the same per-slot-per-node tables the ethshadow baseline
(`decoupled-shadow-sim/data19/baseline.md`) reports, plus the Goldfish
metrics, the slots at which `goldfish_gate_retreat` and `beacon_reorgs_total`
moved, and the supernode's column-subnet state. `vclogs.py` reads the
validator clients' logs for attesters per slot, per-round-offset flatness and
the late-published slots; `elscan.py` walks the execution chain and reports
the blob gas in every block, which is how a run proves its payloads were not
empty.

Prysm exports no received-bytes counter (`p2p_pubsub_rpc_recv_pub_bytes_total`
is declared but never incremented), so received bytes are derived from the
message count and the family's mean sent size — the same derivation the
baseline used.

`runs/` holds one directory per measurement run: the `network_params.yaml` it
ran and a `summary.md` with its numbers. The bulky raw scrapes and node logs
live outside the repo, in `~/dev/prysm2-run-logs/<run>/`.

## What had to be injected, and where

The fork's config does not exist upstream, so every value has an injection
point. In ethereum-package the CL config and the EL genesis both come out of
the genesis-generator image, which makes that image the only place a
chain-config change can land -- `cl_extra_params` cannot carry a file that
is not in the container.

| value | how |
| --- | --- |
| `GLOAS_FORK_EPOCH: 0`, `HEZE_FORK_EPOCH: 0` | native `network_params` keys |
| single-entry `BLOB_SCHEDULE` | native: one non-default `bpo_1_*` |
| Amsterdam at the EL genesis time | falls out of `gloas_fork_epoch: 0` |
| `SLOTS_PER_ROUND: 8` | new line in the image's CL config template; `extra_env` overrides it |
| `AVAILABLE_ATTESTATION_DUE_BPS_HEZE` | same route; it is the head-timing sweep axis |
| Heze not mapped to an EL fork | `genesis_add_heze` patched out |
| a Heze CL genesis state | `prysmctl` replaces `eth-genesis-state-generator` |

The generator image parameter is
`ethereum_genesis_generator_params.image`; `extra_env` under the same key
reaches `values.env`. The package needed no fork.

### Heze is CL-only

Upstream maps every CL fork to the next EL fork by ordinal, so Heze would
schedule `bogotaTime`. geth then demands the next engine-API version at that
timestamp while Prysm keeps calling `forkchoiceUpdatedV4` for its
Gloas-shaped blocks; every fcu fails with -38005 and the chain stalls. This
killed `decoupled-shadow-sim` run data12. `Dockerfile.genesis-gen` disables
`genesis_add_heze`; if a future Heze design does need an EL fork, teach
Prysm the engine-API version first.

### The genesis state

`genesis-gen/prysm-genesis-state.sh` is installed over
`eth-genesis-state-generator`, so the stock entrypoint is untouched. It
builds a `deposit_data.json` with `eth2-val-tools` over the same mnemonic
the keystores use (so the genesis registry holds exactly those pubkeys in
that order), then runs `prysmctl testnet generate-genesis --fork heze`, and
writes back the four fields the entrypoint reads out of
`parsedConsensusGenesis.json`. `genesis_validators_root` is taken from the
SSZ state at offset 8 rather than the JSON, since every client's fork digest
depends on it.

## Things that are normal

- **Config parse errors at startup.** The generator's CL template carries
  `EIP7928_*`, `VIEW_FREEZE_CUTOFF_BPS`, the inclusion-list fields and
  `CONFIRMATION_BYZANTINE_THRESHOLD`, none of which exist in this build.
  `UnmarshalStrict` logs one error line and carries on; the values are not
  used.
- **Deposit-poller chain-id mismatch.** kurtosis' network id versus the
  config's; it only disables deposit following. Not worth chasing.

## The EL is pinned

`el_image` is `ethpandaops/geth:glamsterdam-devnet-8`, commit 366048ea
(2026-08-10) -- the same binary the ethshadow baseline ran, so the two
harnesses differ in their networking, not their clients.

It cannot simply be moved to `ethereum/client-go:latest`. geth master from
2026-08-19 answers every `forkchoiceUpdatedV4` with

    Invalid payload attributes: failed to process builder deposit queue:
    empty system contract: no code at
    0x0000bFF46984e3725691FA540a8C7589300D8282

so no proposer ever gets a local payload and the chain sits at slot 0. Newer
geth expects the ePBS builder-deposit system contract in the EL genesis
alloc, and `ethereum-genesis-generator` 6.0.2 deploys only 4788, 2935, 7002,
7251 and the deposit contract. Whoever bumps the EL has to teach the
generator that contract first.

## Things that are not

- **Fewer than ~128 validators.** `prysmctl testnet generate-genesis` spins
  forever building the PTC window when there are not enough validators to
  fill it; 4 validators never returns. Keep the total at 128.
- **Prysm VC on the REST API.** The REST client has no Gloas/Heze SSZ block
  codec: `AvailableAttestationData` panics with `unimplemented: use grpc`
  the first time a duty comes due. The package passes
  `--beacon-rest-api-provider` unconditionally, and Prysm turns the REST
  client on for the flag's mere presence (`ctx.IsSet`,
  `config/features/config.go:372`), so no `vc_extra_params` value can undo
  it. `validator-entrypoint.sh` strips the flag inside the image, which is
  why the VC image has a shim entrypoint instead of the bare binary.
