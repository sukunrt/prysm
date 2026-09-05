# Exact-old background attestation aggregation audit

## What T1 measures

`aggregate_attestations_t1` is wall time around the entire `batchForkChoiceAtts` call, not the time for one BLS aggregation and not a mutex timer. The wrapper starts the timer immediately before the call and observes it after return (`beacon-chain/operations/attestations/prepare_forkchoice.go:37-48` in the exact `0280403c…` checkout).

For the deployed legacy pool, one pass performs all of the following (`prepare_forkchoice.go:61-109`):

1. `AggregateUnaggregatedAttestations` over the raw pool.
2. Snapshot the aggregated, block, and forkchoice pools.
3. For every candidate, compute an attestation data ID and consult/update the `forkChoiceProcessedAtts` coverage cache (`prepare_forkchoice.go:76-93,127-161`).
4. Group remaining candidates by data ID.
5. Sequentially clone and run the general maximum-cover aggregation for each group, then save the results to the forkchoice pool (`prepare_forkchoice.go:95-99,112-125`).
6. Scan and delete block attestations one at a time.

The observed T1 lower bound of 5.806 seconds therefore cannot be interpreted as “BLS aggregation took 5.806 seconds.” It includes pool snapshots, hashing/IDs, bitset containment and union, cloning, raw deletion, forkchoice cache work, mutex waits, scheduler delay, and aggregation CPU.

## The raw compactor is a specialized path

`AggregateUnaggregatedAttestations` first snapshots and clones every visible raw entry (`operations/attestations/kv/unaggregated.go:54-69`), groups them by version plus data root (`operations/attestations/kv/aggregated.go:30-47`), and distributes groups across `GOMAXPROCS` workers (`aggregated.go:65-110`).

Each group uses `AggregateDisjointOneBitAtts`, a specialized routine for mutually disjoint one-bit votes with identical data. It ORs every bitlist into one coverage bitlist, parses all signatures, and aggregates all signatures once (`proto/prysm/v1alpha1/attestation/aggregation/attestations/attestations.go:36-70`; `maxcover.go:151-200`). Afterward the compactor deletes every successfully aggregated raw entry individually, including a full ID calculation, seen-bit insertion, and a write lock per deletion (`operations/attestations/kv/aggregated.go:49-61`; `operations/attestations/kv/unaggregated.go:130-152`).

That path is simpler than the general `Aggregate` call: `Aggregate` invokes `MaxCoverAttestationAggregation`, which can run up to `n/2` rounds, and each round greedily scans candidate bitlists to choose nonoverlapping coverage (`attestation/aggregation/attestations/maxcover.go:14-95`; `attestation/aggregation/maxcover.go:111-200`). The source describes the underlying maximum-cover loop as `O(k*n*m)` for `n` candidates and bitlist length `m` (`attestation/aggregation/maxcover.go:51-53`); the outer multi-round aggregation can invoke it repeatedly. Overlapping partial aggregates are therefore materially different from one-bit disjoint raw inputs.

The scheduled pass calls both paths: specialized raw compaction first, then the general maximum-cover aggregation over aggregate, block, and prior forkchoice candidates. Under runtime contention, its histogram also includes time descheduled or waiting on shared pool/cache locks.

## Existing timings are different measurements

No existing benchmark in the exact-old checkout calls the full private `batchForkChoiceAtts` method at realistic density. `prepare_forkchoice_test.go` has only small correctness tests.

The existing 13,000-vote compaction result, 1.066 seconds/op, times only `pool.AggregateUnaggregatedAttestations` on six groups of valid mutually disjoint singles in a fresh mixed pool (`historical_late_packing_diagnostic_test.go:945-959`; [result log](old028-final-mixed-raw-seen-5x.log)). It includes specialized compaction and raw deletion, but excludes the later T1 snapshots, seen-cache pass, grouping, general aggregation, forkchoice saves, and block-pool cleanup.

A new exact-old focused profile of the same 13,000-vote arm measured 1.078 seconds/op and identifies raw deletion as its dominant CPU cost. The profile contains five compactor invocations after accounting for benchmark calibration and the parent benchmark's post-subtest call. Of 6.23 focused CPU-seconds, `DeleteUnaggregatedAttestation` accounts for 4.22 seconds, `insertSeenBit` 4.09 seconds, `Bitlist.Contains` 3.85 seconds, and the parallel disjoint-aggregation workers 1.73 seconds ([profile report](old028-heavy13k-raw-compaction-profile.md)).

The source explains this result: `insertSeenBit` retains every prior singleton bitlist and scans them all for containment instead of storing their union. The six-group fixture therefore executes 14,076,834 containment checks during serial cleanup. This cost is outside the raw and aggregate pool locks, so it proves a CPU-heavy T1 component but not a multi-second writer hold or proposer lock wait.

The 3.433-second result times `Server.packAttestations`, a proposer path over 13,000 still-raw candidates (`historical_late_packing_diagnostic_test.go:659-723`). It validates candidates, performs proposer deduplication and general aggregation, builds on-chain aggregates, sorts by reward, and signature-filters. It does not run raw compaction or `batchForkChoiceAtts`. The focused profile attributes the largest sampled descendants to pairwise `Bitlist.Contains`, general maximum-cover aggregation, overlap/count operations, and BLS signature parsing ([profile](old028-heavy13k-fullpack.pprof-focus.txt)). These results show that bitset coverage algorithms are not trivial at this density, but neither number predicts the full scheduled T1 duration.

## Exact-old full-wrapper control

A diagnostic-only benchmark in the exact-old operations package now calls the private `batchForkChoiceAtts` method on the same 13,000-single, six-group shape. Five unloaded iterations measured ([full-wrapper report](old028-batch-forkchoice-results.md), [raw log](old028-batch-forkchoice-5x.log)):

| Arm | Wall time/op | Purpose |
|---|---:|---|
| Full raw `batchForkChoiceAtts` | 1.068264402 s | Raw compaction plus the remainder of the production T1 wrapper |
| Raw compactor only | 1.054091085 s | Direct `AggregateUnaggregatedAttestations` control |
| Precompacted continuation | 0.218342 ms | Direct post-compaction wrapper path from equivalent compact inputs |

The roughly 14 ms difference between the first two independently prepared one-second arms is not a stage measurement: fixture construction, map iteration order, and run noise differ. The direct continuation is the meaningful separation. It shows that the full wrapper's measured wall time is dominated by raw compaction for this controlled shape, while the post-compaction continuation is sub-millisecond.

This result does not reproduce the historical T1 lower bound of 5.806 seconds. Historical pool contents and overlap structure are unavailable, and the production histogram can include mutex waits and scheduler delay absent from the unloaded benchmark. It does establish that the identified quadratic seen-bit cleanup is inside the actual T1 wrapper and dominates this concrete high-density fixture.
