# Proposal packing remote Shadow validation

The reviewed packing changes completed the requested eight-slot observation on
2026-10-05 with 50 nodes and 10,000 active validators: 200 validators per node,
40 home nodes and 10 supernodes. All 50 nodes agreed on slot 8, had zero sync
distance, and were non-optimistic with their execution clients online. Every
node imported every block and execution payload in slots 1–8. No proposal
failures or positive-depth reorgs appeared in the captured logs.

The chain and packing checks passed, but the simulator exited 1 because one
final metrics fetch was still running at shutdown. Metrics from 49 nodes also
show remaining gossip delivery drops. The simulation is stopped.

## Coverage and execution

| Block slot | Beacon attestations | Fresh participants | Payload attestations | EL transactions | Blobs |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1 | 30 / 1,250 | 1 | 0 | 0 |
| 2 | 3 | 1,250 / 1,250 | 1 | 5 | 3 |
| 3 | 3 | 1,250 / 1,250 | 1 | 9 | 6 |
| 4 | 3 | 1,250 / 1,250 | 1 | 8 | 3 |
| 5 | 3 | 1,250 / 1,250 | 1 | 8 | 0 |
| 6 | 3 | 1,250 / 1,250 | 1 | 8 | 6 |
| 7 | 3 | 1,250 / 1,250 | 1 | 7 | 0 |
| 8 | 3 | 1,250 / 1,250 | 1 | 8 | 6 |

Fresh coverage unions included aggregation bits from the preceding slot,
excluding the SSZ delimiter. There is one 1,250-seat committee per slot.
Block 1's partial slot-0 coverage is consistent with the separate, unchanged
slot-0 gossip issue. This network has a different committee configuration from
the CPU benchmark's ten committees of 1,000 participants.

The common slot-8 beacon root is
`0x0e4804baa29a822fd61df21a9cf111da537f81ca4c5c91cbf5f0117a70563ebe`.
Execution block hashes and transaction counts match their envelopes for every
slot. Slot 8 has eight transactions and 786,432 blob gas (six blobs), at hash
`0x0618d7edd4199cbf862d81f1cac1aed3bf584d80e09491ad80c15e0c06e24a4a`.

For the end-to-end slot-8 check, the fresh FFG attestations carry 1,222 and 28
disjoint positions covering all 1,250; the third carries 30 slot-0 positions.
The matching aggregate ledger contains 41 observations of the 1,222-seat
aggregate and one of the 28-seat aggregate, matching their respective slot and
beacon roots. Examples in the saved evidence are
`data/node1/prysm/logs/beacon-chain.log:17201` and
`data/node40/prysm/logs/beacon-chain.log:17285`.

The included payload attestation has all 512 bits set for slot 7 and root
`0xc9c252bc2cd51897843cd96118d571343ecb624be24a6bee4c83a3a252bffd55`.
Both payload-present and blob-data-available flags are true. Its matching PTC
ledger has 20,678 observations across nodes, naming 422 distinct validators,
all with both flags true. Observations repeat across nodes and committee seats
can repeat a validator. One example is
`data/node1/prysm/logs/beacon-chain.log:15772`.

## Network observations and limitations

| Observation | Home nodes | Supernodes |
| --- | ---: | ---: |
| Nodes | 40 | 10 |
| Block imports | 320 / 320 | 80 / 80 |
| Payload imports | 320 / 320 | 80 / 80 |
| Worst block arrival | 226 ms | 156 ms |
| Worst payload arrival | 4,882 ms | 609 ms |
| Minimum Goldfish seat fraction, complete slots 1–7 | 100% | 100% |
| Metrics snapshots | 39 / 40 | 10 / 10 |

Arrival offsets are simulated time from slot start. Every observed payload
arrived within its 12-second slot. The existing summary/ledger verifier passed
its interior window, slots 2–5; the separate analysis checked imports, payload
receipts and block contents for all eight slots. Every node's complete
Goldfish summaries for slots 1–7 report 512 / 512 seats.

The captured metrics show 1,862 undeliverable payload-attestation messages and
242 sync-committee messages (53, 90, 60 and 39 on subnets 0–3). These are summed
per-node delivery events, not unique lost votes, and lower bounds because
node9's metrics are missing. Home snapshots account for 706 drops and super
snapshots for 1,398. The remaining subscription queue pressure is a separate
follow-up; this run does not establish whether packing changed its rate.
No nonzero FFG, available-attestation or data-column undeliverable counter was
present. All 81,591 pubsub reject-counter events have reason
`validation ignored`; that label does not prove every ignored message was a
duplicate.

One beacon error occurred after genesis: node34 reported EL follow distance
with last execution block 1, at simulated `00:05:27`, 27 seconds after genesis.
Eight proposers logged the startup eth1data fallback warning. There were no
post-genesis validator errors. Justified/finalized checkpoints remained at
round/epoch 0; eight startup slots do not establish finalization or longer-term
stability.

Shadow models network behavior, not actual CPU execution cost. Its proposal
timings must not be used as CPU benchmark results. Paired real CPU results are
in [the packing task report](../../task-proposal-packing.md).

## Configuration and evidence

The run used `sukun@ethp2p` (`p2psims`), an isolated source snapshot and freshly
built beacon, validator and genesis-tool binaries. All 4,877 source file hashes
were verified before building with Go 1.26.5. The snapshot includes the
uncommitted packing changes atop `072575178699676ca050d134db0222fef75717ff`.

- Shadow 3.3.0, revision `v3.3.0-35-ga0b01c304`.
- Geth 1.17.6 unstable, revision `fd073543c7044fcfa266551b844ba28ff29b233f`.
- Genesis image: `sha256:5776c235199d4b9a49cefb7472e58af8aff6e019f07aced828970ffbf2fce87b`.
- Heze from genesis; eight-slot rounds; 12-second slots; target committee size
  3,000; 64 target aggregators; one attestation subnet; two subnets per node.
- Seed 1 placement across 17 countries using the repository latency matrix.
- Home: 25 Mbit/s upload, 50 Mbit/s download. Super: 1,024 Mbit/s both ways and
  `--supernode`. Generated topology and 200-validator-per-node placement were
  verified.
- EOA generator throughput 8 with 16 KiB calldata; blob generator throughput 2
  with three sidecars per transaction. Actual block contents are reported above.
- Virtual genesis at 300 seconds; stop at 408 seconds. Final invocation wall
  time: 13 minutes 28.58 seconds. The eight-slot observation was not extended.

The first attempt was stopped before genesis after discovering the collector
used the wrong metrics port. The final attempt used each beacon's actual
monitoring host and port 32001. It reached the configured stop, but the final
metrics request `node9.curl.1019` was still running, causing the sole unexpected
process end state and exit 1. Node9's head, root, sync and finality captures all
completed. Analysis and summary verification passed. Future collectors should
allow their five-second request timeout to expire before the simulation stops.
The run was not repeated to repair this missing snapshot.

Remote run:
`/home/sukun/dev/prysm-shadow-packing-20261005/shadow/runs/packing-n50-v10000-home40-super10-s1-8slots-r2/`.
Build and observation scripts remain under the source root's
`shadow/packing-evidence/`. The invocation was `run.py 8 2`; do not replay over
the existing output directory.

Local evidence:
`/home/sukun/.cache/prysm-proposal-packing/2026-10-05/shadow-remote/result/evidence/`.
It contains all 50 beacon/validator/geth logs, captured REST and EL responses,
`analysis.json`, `summary-verification.log`, `manifest.json`,
`topology-verification.json`, generated configurations, and `provenance/` with
source/binary hashes, build information and scripts. The transferred archive's
SHA256 was verified:
`0bad187d29ff03fab69752461f1b1af611fc6b0493f8d07b407a18abf03e8f3a`.
