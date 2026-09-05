# Does attestation volume explain the startup stall?

The source contains quadratic work, but the saved slots 0–3 do not establish
enough packing candidates to attribute the observed startup failure to it.

`proposerAtts.dedup` compares pairs within attestation-data/committee buckets
(`proposer_attestations.go:373`). Maximum-coverage aggregation repeatedly scans
remaining candidates, also making disjoint-single workloads quadratic in the
candidate count for a fixed bitmap width. This algorithm is used by block
packing, by aggregate-selection RPCs through `mergeByData`, and by parts of
background aggregation. The background unaggregated-single combiner itself
already uses `AggregateDisjointOneBitAtts`, not the general max-cover path.

## What the summaries actually show

Round1 has both summary types for all ten saved nodes in slots 0–3.

| Node | Slot-1 FFG accepted gossip messages at summary |
|---|---:|
| 1 | 56 |
| 2 | 67 |
| 3 (the captured failed proposer) | 38 |
| 50 | 59 |
| 201 | 26 |
| 300 | 55 |
| 400 | 45 |
| 500 | 37 |
| 700 | 90 |
| 900 | 231 |

All round1 slot-0, slot-2, and slot-3 FFG summaries are zero. Node3's slot-1
subnet counts are `4,8,6,1,9,10` (`runs/round1/prysm-geth-3/beacon.log:900`).
The largest total is node900's 231, split `49,36,34,44,46,22`
(`runs/round1/prysm-geth-900/beacon.log:541`).

These FFG counters are incremented after successful gossip validation and the
seen-key check, before the subscriber inserts the message into its pool. They
are sampled at the aggregation deadline, nominally six seconds into the slot
in round1; several timer/log emissions are delayed. They exclude local votes
and do not expose the number of overlapping aggregates or surviving pool
entries. A zero is not proof the pool is empty or no late messages arrived.

The boundary `Goldfish votes` summary instead counts head-vote voters/seats
already inserted into fork choice for the previous slot. It is not the FFG
attestation list passed to block packing. In round1 slot1 only nodes50 and400
report one such vote each; the other captured slot0–3 entries are zero.

Round2 has no `purpose=goldfish-summary` records in any of its twelve saved
beacon logs. Its older instrumentation cannot be interpreted as zero counts.

There is additional, closer-to-aggregation evidence: round1 node1 reports
slot-1 aggregate groups covering 15 and 19 seats (`beacon.log:1628,1629`);
round2 node1 reports one 29-seat group for slot1 (`beacon.log:1223`), even
though it logs completion at 01:30:28.57, after that slot's deadline. These are
union-seat counts, not input-list lengths; they still do not demonstrate
thousands of disjoint input candidates.

## Causality limit

The captured round1 slot-1 proposal has already missed its deadline before
it reaches `BuildBlockParallel`, which invokes its packing work. Its own
quadratic packing therefore cannot explain the earlier Building-to-graffiti
gap. Concurrent background/aggregation RPC work remains a possible source of
contention, but must not be inferred from 1,000 nodes or 120,000 validators.
Separate nodes' traffic cannot be summed as one node's pool, and six
committees are separate comparison buckets rather than one 15,000-item bucket.

Relevant packing and background aggregation functions are unchanged between
the two run revisions. The diagnostic benchmark fixes committee bitmap width
at 2,500 and varies received candidate counts; results are recorded in
`packing_bench_results.md`. It measures algorithm cost, not the original
simulation's CPU allocation, pool contents, or concurrent scheduling.
