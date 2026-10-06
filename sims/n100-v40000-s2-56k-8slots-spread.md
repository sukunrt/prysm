# 100 nodes, 40,000 validators: Prysm staggered FFG publication

2026-10-06. The eight-slot Prysm rerun completed successfully on ethp2p in
1232.26 wall seconds. The only configuration change was
`--decoupled-ffg-vote-spread`; genesis, binaries, network mapping, process order
and workload settings matched the baseline. All 100 nodes joined both subnets.

Staggering reduced the measured FFG delivery time after publication and the
number of score-driven disconnections. It increased p95 arrival time from slot
start because publication is deliberately delayed. Final-slot missing receipt
records increased. The actual included blob workload also differed.

This repeats the [previous 8-slot experiment](n100-v40000-s2-56k-8slots.md),
adding `--decoupled-ffg-vote-spread` to all 100 validator processes. The previous
Prysm and eth-slot-sim results are retained as references; eth-slot-sim is not
rerun for this experiment.

## Results

**Prysm slot 8 in both runs.** All times are milliseconds and quantiles use
observed remote receipts. Arrival after publication subtracts the matching
validator's local publication timestamp for each receipt, before computing
quantiles. It is not a subtraction of separate quantiles.

## Arrival from slot start

| Receivers | No spread avg | p95 | p99 | Spread avg | p95 | p99 |
|---|---:|---:|---:|---:|---:|---:|
| all | 998 | 3,926 | 8,594 | 3,086 | 6,019 | 8,548 |
| home | 1,200 | 4,455 | 8,912 | 3,199 | 6,290 | 8,663 |
| super | 146 | 312 | 427 | 2,612 | 5,154 | 5,230 |

## Arrival after matching publication

| Receivers | No spread avg | p95 | p99 | Spread avg | p95 | p99 |
|---|---:|---:|---:|---:|---:|---:|
| all | 993 | 3,918 | 8,589 | 588 | 2,399 | 5,284 |
| home | 1,195 | 4,455 | 8,907 | 702 | 3,132 | 5,585 |
| super | 141 | 308 | 424 | 112 | 230 | 279 |

## Coverage and workload

| Final slot | No spread | Spread |
|---|---:|---:|
| Payload bytes | 58,259 | 57,706 |
| Blobs | 9 | 3 |
| FFG receipts missing | 171 | 477 |
| FFG receipts at/after 9s | 3,798 | 3,088 |
| Score-driven disconnections during run | 32 | 2 |

The final-slot spread run published all 5,000 votes between 0 and 5,002ms.
Its 477 missing remote receipt records are all at **home node17**, which retained
99 CL and EL peers at the final snapshot. The baseline's 171 missing records
were at nodes16/19. Receipt losses therefore cannot be attributed solely to the
links removed by peer scoring.

The spread run published 37 final-slot FFG aggregates at 9,000–9,001ms, each with
2,500 votes. All 100 nodes imported the same final block and payload and agreed
on the head. The final block contains two transactions, one FFG attestation
object and one payload attestation, with matching EL/payload hashes and the
200M gas limit. No final-slot error logs or ledger parse failures were found.

### Connectivity

Every node had 99 CL and EL peers before slot 1. At the final snapshot:

- Spread: three nodes below 99 CL peers (97–99), two missing CL links, all EL
  peers connected. Node5 explicitly disconnected nodes35 and76 at slot 8 + 3s
  for gossip scores −137.87 and −380.01, below the −100 threshold.
- Baseline: 35 nodes below 99 CL peers (81–99), 32 missing CL links and three
  missing EL links.

The complete-connectivity audit still fails for the spread run. The two score
messages exactly identify its missing CL links; existing scoring rules were
not disabled or changed.

### Publication exception in slot 6

Seven measured slots published exactly 2,500 votes per committee in the expected
six groups. **Slot 6 published 4,999 votes:** subnet0's 4s group has 416 instead
of 417. Validator22365 on supernode63 has no local or remote FFG record. The
validator client logged an EOF while posting its attestation to the local
beacon REST endpoint at slot 6 + 4s. The overall publication-count audit fails
for this reason. Its cause has not been fixed as part of this simulation run.

### Workload and coverage across the run

Both runs used the same configured Spamoor traffic, but actual inclusion varied.
The final payload sizes are close; final blob counts are **3 with spreading
versus 9 without**, so this is not a constant-blob-load comparison. Recent EL
traffic and earlier slots can also affect the final-slot network queues.

| Slot | Spread payload bytes | Spread blobs | No-spread missing vs target | Spread missing vs target |
|---|---:|---:|---:|---:|
| 1 | 853 | 0 | 0 | 0 |
| 2 | 57,403 | 0 | 0 | 0 |
| 3 | 57,983 | 6 | 6,749 | 3,341 |
| 4 | 1,444 | 6 | 5,250 | 3,634 |
| 5 | 114,218 | 3 | 10,366 | 4,136 |
| 6 | 838 | 0 | 6,725 | 2,555 |
| 7 | 114,797 | 9 | 7,565 | 4,713 |
| 8 | 57,706 | 3 | 171 | 477 |

The target is 495,000 remote receipts per slot. Slot 6's spread deficit includes
99 receipts corresponding to the unpublished vote; the other 2,456 are absent
for votes with observed publications. Slot 8 has all expected publications, so
its 477-record deficit is entirely remote receipt coverage.

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

## Analysis method

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

- [Final-slot spread comparison](../shadow/runs/compare-n100-v40000-s2-payload56k-8slots-spread-20261006/artifacts/spread-comparison.md)
- [Comparison data, including per-vote publication-adjusted statistics](../shadow/runs/compare-n100-v40000-s2-payload56k-8slots-spread-20261006/artifacts/spread-comparison.json)
- [Current run and existing eth-slot-sim final-slot results](../shadow/runs/compare-n100-v40000-s2-payload56k-8slots-spread-20261006/artifacts/last-slot/results.json)
- [Across-slot results](../shadow/runs/compare-n100-v40000-s2-payload56k-8slots-spread-20261006/artifacts/ffg-across-slots/results.json)
- [Publication and subscription audit](../shadow/runs/compare-n100-v40000-s2-payload56k-8slots-spread-20261006/artifacts/last-slot/two-subnet-audit.json)
- [Peer-disconnection evidence](../shadow/runs/compare-n100-v40000-s2-payload56k-8slots-spread-20261006/artifacts/peer-drop-analysis.json)
- [Slot 6 publication failure](../shadow/runs/compare-n100-v40000-s2-payload56k-8slots-spread-20261006/artifacts/missing-publication-audit.json)
