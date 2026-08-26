# kurtosis harness for the decoupled fork

A second, non-simulated witness for the same topology ethshadow measures:
real wall clock, real docker networking, same binaries.

## Build

```sh
./kurtosis/build-images.sh
```

Builds `beacon-chain`, `validator` and `prysmctl` with `go build`
(statically linked: CGO for blst, `netgo,osusergo` so nothing reaches for
glibc's NSS at runtime), then four images:

| image | contents |
| --- | --- |
| `prysm-beacon-chain:local` | the beacon node at `/beacon-chain` |
| `prysm-validator:local` | the validator client at `/validator` |
| `prysm-genesis-gen:local` | the fork generator branch plus this tree's `prysmctl` |
| `prysm-buildoor:local` | `sukunrt/buildoor` branch `decoupled` |

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

`network_params.yaml` is the 5-node baseline (250 validators). Node count and
keys per node are free; keep the total at 128 or more.

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
reaches `values.env`. The package needed no fork. The generator did:
the patches live as commits on
`github.com/sukunrt/ethereum-genesis-generator`, branch `decoupled`
(forked from v6.2.1), which `build-images.sh` builds straight from the git
URL; `Dockerfile.genesis-gen` only adds this tree's `prysmctl` on top.

### Heze is CL-only

Upstream maps every CL fork to the next EL fork by ordinal, so Heze would
schedule `bogotaTime`. geth then demands the next engine-API version at that
timestamp while Prysm keeps calling `forkchoiceUpdatedV4` for its
Gloas-shaped blocks; every fcu fails with -38005 and the chain stalls. This
killed `decoupled-shadow-sim` run data12. The fork branch disables
`genesis_add_heze`; if a future Heze design does need an EL fork, teach
Prysm the engine-API version first.

### The genesis state

The fork's `apps/prysm-genesis-state.sh` is installed over
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

## The EL pin

`el_image` defaults to `ethpandaops/geth:glamsterdam-devnet-8`, commit
366048ea (2026-08-10) -- the same binary the ethshadow baseline ran, so the
two harnesses differ in their networking, not their clients. The pin is
comparability, not necessity: newer geth needs the EIP-8282 builder
deposit/exit contracts predeployed in the EL genesis, which the fork
generator (based on 6.2.1) provides. geth `master-fd07354` (2026-08-26)
passed both harnesses; `network_params.geth-master.yaml` is that run's args.
Always pin a build, never `:latest`.

## Things that are not

- **Fewer than ~128 validators.** `prysmctl testnet generate-genesis` spins
  forever building the PTC window when there are not enough validators to
  fill it; 4 validators never returns. Keep the total at 128.
## Validator API transport

The REST client supports the fork's duties, available attestations included,
so both transports work. ethereum-package still defaults a 1:1 Prysm BN/VC
pair to gRPC (`--beacon-rpc-provider`); add `--enable-beacon-rest-api` to
`vc_extra_params` to select REST.
