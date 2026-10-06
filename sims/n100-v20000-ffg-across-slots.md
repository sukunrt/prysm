# Individual FFG votes across eight slots

2026-10-06. Analysis of the completed 100-node, 20,000-validator logging rerun
on ethp2p and the existing eth-slot-sim comparison. This extends the final-slot
report at the user's request to inspect whether peer scoring changes delivery
over successive slots. No additional simulation or behavior change was made.

Prysm's super-node mean improves while its home-node tail worsens and fluctuates.
eth-slot-sim is comparatively stable. This is consistent with evolving routing
or load imbalance, but does not isolate peer scoring: the workload ramps up,
blob counts vary, and the simulators have different traffic and transport.
A more specific correlation is that the slowest receiver in every slot after
the first had recently accepted a blob transaction through its local EL RPC.

![FFG timing by receiver class](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/ffg-across-slots/ffg-by-class.png)

## Arrival mean, p95 and p99 by receiver class

Each cell is **average / continuous p95 / continuous p99, milliseconds from slot start**.
Pair rows align measured-slot order, Prysm 1–8 with eth-slot-sim 0–7.
Publisher receipts are excluded. Asterisks identify incomplete Prysm ledgers.

| Slot pair P/E | Prysm home | eth-slot-sim home | Prysm super | eth-slot-sim super |
|---|---:|---:|---:|---:|
| 1/0 | 424 / 1,530 / 2,217 | 433 / 1,300 / 2,288 | 245 / 645 / 784 | 319 / 836 / 1,036 |
| 2/1* | 514 / 1,811 / 2,503 | 369 / 1,044 / 1,751 | 192 / 521 / 669 | 261 / 478 / 700 |
| 3/2* | 660 / 1,892 / 9,013 | 381 / 1,082 / 1,695 | 197 / 526 / 908 | 269 / 629 / 812 |
| 4/3* | 772 / 2,530 / 7,959 | 356 / 866 / 1,733 | 150 / 343 / 815 | 278 / 639 / 905 |
| 5/4 | 848 / 2,750 / 7,960 | 356 / 1,135 / 1,967 | 164 / 542 / 763 | 295 / 739 / 840 |
| 6/5 | 567 / 1,842 / 2,890 | 360 / 1,144 / 2,024 | 151 / 461 / 615 | 283 / 686 / 819 |
| 7/6 | 627 / 2,087 / 5,268 | 372 / 1,213 / 2,035 | 152 / 419 / 872 | 282 / 603 / 696 |
| 8/7 | 746 / 2,536 / 5,769 | 375 / 1,142 / 1,976 | 155 / 397 / 579 | 287 / 605 / 795 |

## Overall timing, completeness and workload

Each slot has 2,500 individual publishers and 247,500 expected remote receipts.
All eth-slot-sim slots have complete, duplicate-free receipts. Its payload is
262,144 bytes with six modeled blobs throughout. Prysm's figures use its observed
receipts; missing records are not assigned an artificial latency.

| Slot pair P/E | Prysm avg / p95 / p99 ms | eth-slot-sim avg / p95 / p99 ms | Prysm missing | Prysm payload bytes | Prysm blobs |
|---|---:|---:|---:|---:|---:|
| 1/0 | 390 / 1,349 / 2,116 | 411 / 1,201 / 2,264 | 0 | 853 | 0 |
| 2/1 | 452 / 1,644 / 2,445 | 349 / 979 / 1,654 | 117 | 117,222 | 3 |
| 3/2 | 571 / 1,754 / 8,740 | 359 / 974 / 1,619 | 272 | 282,945 | 3 |
| 4/3 | 653 / 2,198 / 7,745 | 341 / 837 / 1,614 | 600 | 249,821 | 3 |
| 5/4 | 717 / 2,602 / 7,621 | 345 / 935 / 1,865 | 0 | 233,815 | 9 |
| 6/5 | 487 / 1,746 / 2,727 | 345 / 979 / 1,954 | 0 | 299,538 | 3 |
| 7/6 | 536 / 1,892 / 5,199 | 355 / 1,085 / 1,962 | 0 | 266,718 | 6 |
| 8/7 | 633 / 2,119 / 5,562 | 358 / 1,086 / 1,929 | 0 | 267,021 | 9 |

Prysm slot 1 is an empty execution block; the transaction workload starts at
its boundary. It is not a steady-workload baseline for attributing later changes
to scoring. Missing records are concentrated at homes: slot 2/node 2 (117);
slot 3/node 29 (271) and node 3 (1); slot 4/nodes 6 (355), 56 (155), 29 (88),
57 (1), and 35 (1). These are ledger absences across the available run, not proof
of permanent network loss. Slots 1 and 5–8 are complete.

## What the slot comparison establishes

- Prysm super mean falls from 244.530 ms in slot 1 to approximately 150–164 ms
  in slots 4–8. eth-slot-sim super mean remains 261–295 ms after its first slot.
- Prysm home p99 rises from 2,217 ms in slot 1 to 9,013 ms in slot 3, falls to
  2,890 ms in slot 6, then reaches 5,769 ms in slot 8. It is not monotonically
  worsening. eth-slot-sim home p99 stays approximately 1,695–2,288 ms.
- The worst homes change. The highest mean receiver in Prysm slots 1–8 is
  respectively node 42, 2, 2, 3, 6, 10, 11, and 2. A permanent partition of
  the same few slow homes is not established.
- Both vote bursts remain at slot start: all Prysm publications are at 0–6 ms;
  eth-slot-sim publications are approximately 0.28–1.26 ms.
- For every slot, the largest recorded validation-entry-to-log difference in
  Prysm is at most 10 ms. Allowing for the 10 ms text timestamp precision keeps
  this interval around 20 ms or less. The multi-second delay precedes entry to
  this validator; upstream transport/queueing is still unseparated.

## Peer scoring remains a hypothesis

Prysm enables first-delivery rewards and score-aware mesh pruning; eth-slot-sim
does not enable peer scoring. Prysm can attempt opportunistic grafting every
60 heartbeats (approximately 42s), when the mesh median score is below five.
This rewards routes based on accumulated delivery behavior, so the growing
super/home separation is compatible with the hypothesis. The saved run does
not contain the mesh membership, score snapshots or GRAFT/PRUNE history needed
to connect a timing change to a scoring-driven mesh change. The
mesh-delivery-deficit penalty is disabled in Prysm; first-delivery rewards remain.

A controlled scoring-on/off comparison in the same simulator, with the same
workload, transport and outbound-peer quota, would distinguish this hypothesis.
Recording mesh membership and per-host network traffic would show whether
improved super delivery coincides with home isolation or upload congestion.

Evidence: [all statistics and per-node results](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/ffg-across-slots/results.json),
[class CSV](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/ffg-across-slots/by_class.csv), [overall CSV](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/ffg-across-slots/overall.csv),
[SVG chart](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/ffg-across-slots/ffg-by-class.svg),
[peering-rule comparison and final-slot diagnosis](n100-v20000-ignored-aggregate-rerun.md#individual-ffg-delay-and-evolving-gossip-meshes).

## Blob submission is a stronger lead for the home-node tail

The execution envelopes identify type-3 transactions by their typed transaction
encoding. Their positions match the transaction hashes in the corresponding EL
blocks (matching block hashes and transaction counts). Matching those hashes to
Geth `Submitted transaction` logs identifies which node accepted each RPC
submission and when. The 12 distinct blob transactions included in slots 1–8
have 28 submission records across clients; a transaction can be submitted to
more than one client.

| Prysm slot | Slowest receiver | FFG avg / p99 ms | Blob submission before slot start, seconds |
|---|---:|---:|---:|
| 2 | 2 | 1,558 / 11,750 | 0.492 |
| 3 | 2 | 8,955 / 9,818 | 12.492 |
| 4 | 3 | 7,229 / 9,731 | 11.035 |
| 5 | 6 | 7,967 / 8,980 | 12.790 |
| 6 | 10 | 3,492 / 11,360 | 3.939 |
| 7 | 11 | 4,660 / 7,218 | 6.429 |
| 8 | 2 | 4,192 / 6,259 | 6.082 |

All seven slowest receivers are homes. In slot 8, all five highest-mean receivers
(nodes 2, 13, 15, 14, 23) accepted blob RPC submissions during slot 7. These
submissions occurred 6.082, 11.589, 6.789, 9.824 and 4.820 seconds before slot 8,
respectively. The sixth-slowest home, node 52, also accepted one.

A concrete candidate is bandwidth contention from EL blob dissemination on the
same 25 Mbit/s upload / 50 Mbit/s download host links as CL gossip. Each Geth
client has 99 peers; eth-slot-sim has no equivalent EL transaction-gossip load.
This correlation is more specific than a generic scoring-over-time explanation,
but does not prove the packet/queue mechanism. The run has no packet trace or
per-node byte timeline to quantify that mechanism. A controlled EL-traffic
comparison and mesh/queue traces would separate it from scoring effects.

Evidence: [matched blob submissions](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/ffg-across-slots/blob-submissions.json),
[per-slot correlation](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/ffg-across-slots/blob-delay-correlation.json).

## p95 interpretation

In the final slot, overall median/p95/p99 are 317/2,119/5,562 ms for Prysm
and 251/1,086/1,929 ms for eth-slot-sim. The p95 gap is about 1.95x,
compared with a 2.88x p99 gap. The difference reaches beyond the last 1%,
while the median gap is smaller (about 1.26x).

The class split remains essential: final-slot home p95 is 2,536 versus
1,142 ms; super p95 is 397 versus 605 ms. Percentiles describe remote
receiver-vote records, not the percentile of each validator's global
completion time or the percentile of node averages.

## Home versus super receivers in the final slot

Here “local” means the 80 home nodes, not locally produced votes. All figures
below exclude the publisher's own receipt and measure milliseconds from slot
start. Prysm slot 8 and eth-slot-sim slot 7 both have complete individual FFG
receipt coverage.

| Simulator / receiver class | Nodes | Average ms | Median ms | p95 ms | p99 ms |
|---|---:|---:|---:|---:|---:|
| Prysm home | 80 | 746 | 419 | 2,536 | 5,769 |
| eth-slot-sim home | 80 | 375 | 252 | 1,142 | 1,976 |
| Prysm super | 20 | 155 | 143 | 397 | 579 |
| eth-slot-sim super | 20 | 287 | 246 | 605 | 795 |

The two classes do not have equal capacity: both configurations give homes
25 Mbit/s upload and 50 Mbit/s download, versus 1,024 Mbit/s in both directions
for supers. Prysm also runs Geth and beacon networking on the same host link.
Its EL transaction/blob dissemination is absent from eth-slot-sim, so matching
the execution payload size does not match total offered network traffic.
All 99 peer connections are present, but gossip still uses a target mesh of
eight peers; full connectivity does not imply direct delivery of every vote.

To quantify the blob-submission correlation, classify a receiver as recent
when its Geth log records acceptance of an included blob transaction during
the 15 seconds before the slot boundary. In Prysm slot 8:

| Receiver group | Nodes | Average ms | Median ms | p95 ms | p99 ms |
|---|---:|---:|---:|---:|---:|
| Home, recent matched blob RPC | 7 | 2,806 | 2,908 | 7,422 | 9,019 |
| Home, no observed recent matched blob RPC | 73 | 549 | 388 | 1,664 | 2,243 |
| Super | 20 | 155 | 143 | 397 | 579 |

The seven homes are nodes 2, 13, 14, 15, 23, 24 and 52. This diagnostic split
retains all receipts; it is not an adjustment to the official comparison.
Matching includes only blob transactions included in the measured slots, so
the other 73 homes may still relay EL traffic or accept unmatched/pending
transactions. The 15-second window is descriptive, not an established queue
drain time. Their p95 also remains above eth-slot-sim's home p95, so this split
does not explain the entire difference.

In this slot, supers originate 2,476 of the 2,500 votes (99.04%). A super
receives about 2,376 remote votes on average, versus about 2,500 at a home,
because its own publications are excluded. This small count difference alone
does not establish an explanation for the timing gap. Scoring may favor fast
paths between these publishers, but mesh histories are missing. The evidence
currently supports investigating shared EL/CL bandwidth contention first,
while retaining peer scoring as a separate, unproven contributor.

Evidence: [receiver load split](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/ffg-across-slots/receiver-load-split.json).
