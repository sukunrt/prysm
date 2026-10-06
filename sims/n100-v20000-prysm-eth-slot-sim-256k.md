# Prysm / eth-slot-sim: 100 nodes, 20k validators, full connectivity

2026-10-06. Both simulations completed successfully on ethp2p. Results below use
only the final measured slot: **Prysm slot 8 / eth-slot-sim slot 7**.

A subsequent [rerun with unconditional ignored-aggregate logs](n100-v20000-ignored-aggregate-rerun.md)
records aggregate arrivals that the baseline ledger omitted. The baseline
numbers below are preserved; see also the [column timing analysis](n100-v20000-column-timing-analysis.md).

Every participant had the other 99 peers: all 100 Prysm beacon nodes and all 100
Geth clients at both runtime snapshots, and all 100 eth-slot-sim nodes at every
slot boundary. The audit checked peer identities, not just counts. This is 4,950
bidirectional participant connections per network; the GossipSub mesh remains D=8.

## Final-slot results

Mean and continuous p99 in **milliseconds from slot start**, over remote
receiver-message records. Publisher receipts are excluded.

| Object | Prysm avg | Prysm p99 | eth-slot-sim avg | eth-slot-sim p99 |
|---|---:|---:|---:|---:|
| Consensus block | 128 | 270 | 127 | 228 |
| Execution payload | 1,108 | 3,172 | 1,957 | 3,212 |
| Data column readiness | 107 | 217 | 1,118 | 1,277 |
| Availability vote | 686 | 2,235 | 403 | 929 |
| FFG vote | 480 | 2,564 | 358 | 1,929 |
| FFG aggregate | N/A | N/A | 9,145 | 9,289 |
| PTC vote | 847 | 3,561 | 6,175 | 6,431 |

| Final-slot measurement | Prysm slot 8 | eth-slot-sim slot 7 |
|---|---:|---:|
| Execution payload bytes | 250,115 | 262,144 |
| Payload KiB | 244.3 | 256.0 |
| Individual FFG voters | 2,500 | 2,500 |
| Remote individual FFG receipts | 247,500 / 247,500 | 247,500 / 247,500 |
| Missing individual FFG receipts | 0 | 0 |
| FFG receipts at or after 9s | 0 | 0 |

Prysm's payload is 4.59% below the shared 262,144-byte target. Its final block has
17 transactions (15 EOA plus two blob transactions), six blobs, gas limit
200,000,000, and gas used 16,073,640. The beacon block contains one FFG attestation
and one payload attestation. All 100 nodes imported the same block and payload,
and agreed on the slot-8 head. There were no final-slot error logs or parser failures.
All eight captured block gas limits, in both the payload envelope and EL block,
were exactly 200,000,000.

The FFG vote publications were at 0–5 ms in Prysm and approximately 1.02–1.23 ms
in eth-slot-sim. Prysm's last remote FFG receipt was at 4,393 ms. Its 19 local FFG
aggregates each covered all 2,500 voters and were published at exactly 9,000 ms;
eth-slot-sim's 20 aggregates were published at approximately 9,001 ms.

Measurement limits:

- **FFG aggregate N/A:** Prysm recorded no accepted gossip aggregates in the final
  slot. Its ledger logs accepted aggregates; aggregates already covered by the
  local pool can be ignored before logging. The local publications at 9s are
  verified, but their remote arrival average/p99 is unavailable. The empty ledger
  does not demonstrate network loss.
- **PTC:** both deadlines are 6s. Prysm can publish upon payload availability;
  eth-slot-sim publishes at the deadline (observed approximately 6,001 ms). These
  slot-relative numbers include that publication-time difference. Prysm recorded
  45,738 remote receipts from 462 distinct signers, each reaching all 99 remote
  nodes, with payload and blobs present. Its 512-seat committee can repeat
  validators. eth-slot-sim used 512 signers and recorded 50,688 receipts. The
  Prysm ledger has no local PTC publication records in this run, so its receipt
  audit establishes coverage for observed signers, not the expected signer set.
- **Payload/columns:** eth-slot-sim revealed the final payload and columns at
  966 ms. Prysm column readiness includes local reconstruction. Publication
  schedules and processing stages differ, so slot-relative timing differences
  cannot be attributed solely to network propagation.
- The same host placement and underlying network were used, with independent
  proposer and committee draws. Final proposers were Prysm node 79 and
  eth-slot-sim node 53 (one-based), both supernodes.

Saved results: [raw results and checks](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-fullmesh-20261006/artifacts/last-slot/results.json),
[avg/p99 CSV](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-fullmesh-20261006/artifacts/last-slot/avg-p99.csv),
[peer identity audit](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-fullmesh-20261006/artifacts/peer-audit.json),
[receipt audit](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-fullmesh-20261006/artifacts/last-slot/receipt-audit.json),
[manifest](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-fullmesh-20261006/artifacts/manifest.json).

## Configuration and reproducibility

| Setting | Both simulations |
|---|---|
| Participants | 100: 80 home nodes, 20 supernodes |
| Validators | 20,000 total |
| Validator placement | 153 across homes (1–3 each); 19,847 across supers (992–993 each) |
| Slots | Eight live slots; report Prysm slot 8 / eth-slot-sim slot 7 only |
| FFG | One subnet, one committee of exactly 2,500 voters per slot |
| Individual FFG publication | Slot start; no spread and zero configured jitter |
| FFG aggregation deadline | 9 seconds |
| PTC deadline | 6 seconds |
| ePBS | Enabled |
| Payload target | Approximately 256 KiB, excluding blob sidecars |
| Network | Complete graph: 99 participant peers per node |
| Home bandwidth | 25 Mbit/s up, 50 Mbit/s down |
| Supernode bandwidth | 1024 Mbit/s up and down |
| Gossip mesh | D=8, Dlow=6, Dhigh=12 |

Prysm uses the freshly built queue changes: 20,000-message pubsub queues,
50,000 global and per-topic validation concurrency, and 4,096 subscription buffers.
Its gas target is explicitly 200,000,000 in the EL genesis, Geth miner setting,
v2 validator proposer settings, **and** `--suggested-gas-limit`.

The PTC deadline change is configuration only. Prysm still publishes on payload
availability or the deadline; eth-slot-sim still publishes at the deadline. Their
slot-relative PTC arrival times therefore measure different publication schedules.

The Spamoor workload targets 16 EOA transactions per slot with 16 KiB random
calldata each, 32 wallets and 64 pending transactions; a separate workload targets
two three-blob transactions per slot. eth-slot-sim independently uses a fixed
262,144-byte payload. The actual final Prysm payload was 250,115 bytes.

eth-slot-sim's staggered/segregated voting rule is enabled. Its normal round draw
has binomial variation around 2,500 voters; this run uses a seeded shuffled
partition into eight equal groups of 2,500 through a run-local schedule override.
No validator votes in multiple groups.

The same Shadow graph and host locations are used by both simulators. Each Prysm
beacon node bootstraps from the other 99 participants without marking them as
GossipSub direct peers. All 100 Geth clients are also connected through explicit
static peer lists. eth-slot-sim's peer graph has degree 99. Actual peer identities
are captured for Prysm before slot 1 and near the end of slot 8, and eth-slot-sim
has run-local logging of connected peers before every slot and after the final slot.
All of those runtime connectivity observations passed.

The first Prysm attempt was stopped after the runtime audit found only 50–96 EL
peers per node, despite all CL nodes having 99 peers. Raising Geth's `MaxPeers`
from 99 to 300 in the `-elmesh` attempt still left incomplete execution connections.
An isolated 100-client Geth probe reached 99 peers on every client when only the
lower-numbered node initiates each pair's connection. These connections are
bidirectional. The final `-fullmesh` run uses that static-dial rule, `MaxPeers=300`,
and discovery disabled. Runtime peer identities, rather than configured limits,
determine whether the run passes. The completed eth-slot-sim run is reused; all
900 of its slot-boundary peer audits showed exactly 99 connected peers and no
missing peers.

The eth-slot-sim binary uses a Go build overlay solely for the peer audit. The
repository source and message publication behavior are unchanged.

Artifacts: [driver](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-fullmesh-20261006/run_comparison.py),
[eth-slot-sim config](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-20261006/eth-config.yaml),
[placement](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-20261006/placement.json),
[Prysm binary provenance](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-20261006/current-binary-provenance.json),
[eth-slot-sim binary provenance](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-20261006/eth-binary-provenance.json).

Current Prysm/analysis root:
`ethp2p:/home/sukun/dev/slot-compare-20261006-n100-v20000-agg9s-ptc6s-payload256k-gas200m-fullmesh`.

eth-slot-sim root:
`ethp2p:/home/sukun/dev/slot-compare-20261006-n100-v20000-agg9s-ptc6s-payload256k-gas200m`.
