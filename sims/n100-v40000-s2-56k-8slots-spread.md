# 100 nodes, 40,000 validators: Prysm staggered FFG publication

2026-10-06. The Prysm-only rerun is running on ethp2p as Shadow PID 1132137,
supervised by driver PID 1132136. Results are pending. The preflight passed:
all 100 validator processes enable the flag, genesis inputs match, and the
Shadow configuration matches the baseline apart from this flag and run paths.

This repeats the [previous 8-slot experiment](n100-v40000-s2-56k-8slots.md),
adding `--decoupled-ffg-vote-spread` to all 100 validator processes. The previous
Prysm and eth-slot-sim results are retained as references; eth-slot-sim is not
rerun for this experiment.

## Configuration and flag behavior

| Setting | Value |
|---|---|
| Nodes / total validators | 100 / 40,000 |
| Node classes | 80 home / 20 super, same placement and validator distribution |
| Measured slots | Prysm 1–8 |
| FFG subnets / committees | 2 / 2, every node subscribed to both |
| Committee size / publications per slot | 2,500 / 5,000 |
| New flag | `--decoupled-ffg-vote-spread` |
| Required companion flag | `--decoupled-ffg-vote-at-slot-start` remains enabled |
| Extra random jitter | 0ms |
| FFG publication groups | 0s, 1s, 2s, 3s, 4s, 5s |
| FFG aggregate publication | 9s |
| PTC deadline | 6s; existing early-send behavior retained |
| ePBS | Enabled |
| Execution payload target | 57,344 bytes (56 KiB) |
| Spamoor EOA workload | One transaction/slot, 56,320 bytes of random calldata |
| Blob workload | Two transactions/slot, three blobs each; target six blobs/slot |
| Block gas limit | 200,000,000 |
| Configured connectivity | All 99 peers per node, CL and EL |
| Network, peer scoring, queues, binaries | Same as baseline |

The flag spreads individual validators by **position within their committee**,
not by assigning one common delay to each node. The implementation in
[wait_helpers.go](../validator/client/wait_helpers.go) calculates
`groups = (aggregate_due - 3s) / 1s`, then
`delay = floor(committee_position * groups / committee_length) * 1s`.
With a 9s aggregate deadline this produces six groups. Each 2,500-member
committee has `[417, 417, 416, 417, 417, 416]` scheduled publications at those
six offsets. The wait occurs immediately before submission, after attestation
data retrieval and signing.

The run generates fresh databases, retains the same network mapping, keys,
Spamoor seeds and workload parameters, and compares the generated Shadow
configuration against the baseline after normalizing run paths and removing
the spread flag. It also requires identical genesis SSZ, execution genesis,
chain configuration and binary hashes before launching Shadow.

The generator shuffled process order, so preparation restored the baseline's
order before the configuration assertion. Execution-genesis JSON is compared
with canonical key ordering; the SSZ and chain-config hashes use raw bytes.

## Analysis

The final-slot comparison reports average, p95 and p99 for both:

- **Arrival from slot start**, including deliberate publication delay.
- **Arrival after publication**, subtracting the matching validator's local
  publication timestamp from each observed remote receipt timestamp.

Both measurements are split by home/super receivers. Publication-group counts
are checked for both committees across all eight slots. Receipt completeness,
9s deadline misses, actual payload/blob inclusion, end-to-end block contents,
peer connectivity and score-driven disconnections are audited separately.
Failed audits are recorded while the remaining analysis continues.

The baseline's final payload was 58,259 bytes with nine blobs; it had 171
missing FFG receipts and 32 CL links disconnected for low gossip scores.
Maintaining the same workload configuration does not guarantee identical
transaction inclusion or blob counts; those are measured as run outcomes.

Remote root:
`ethp2p:/home/sukun/dev/slot-compare-20261006-n100-v40000-s2-payload56k-8slots-spread`

- [Run driver](../shadow/runs/compare-n100-v40000-s2-payload56k-8slots-spread-20261006/run_prysm.py)
- [Experiment settings](../shadow/runs/compare-n100-v40000-s2-payload56k-8slots-spread-20261006/experiment.json)
- [Comparison analysis](../shadow/runs/compare-n100-v40000-s2-payload56k-8slots-spread-20261006/compare_spread.py)
- [Peer-disconnection audit](../shadow/runs/compare-n100-v40000-s2-payload56k-8slots-spread-20261006/audit_peer_drops.py)
