# Exact-old 13k raw-compaction CPU profile

## Measurement

This profile was collected from the exact deployed source checkout `0280403c70d88967f49d2d4c730f4c5417dabdf5`. It uses the existing diagnostic fixture with 13,000 valid one-bit attestations split across six groups (`2167/2167/2167/2167/2166/2166`) and a fresh legacy pool for each benchmark invocation. Production files were unchanged.

```text
GOMAXPROCS=4 go test ./beacon-chain/rpc/prysm/v1alpha1/validator \
  -run '^$' \
  -bench '^BenchmarkDiagnosticHistoricalLateMixedPools/heavy_13000_fresh_singles/compact_gloas_retained_aggregate_unaggregated$' \
  -benchtime=3x -count=1 -timeout=10m \
  -cpuprofile old028-heavy13k-raw-compaction.cpu.pprof
```

The timed benchmark result was **1.078471960 s/op**, 128,375,029 B/op, and 1,080,573 allocs/op ([run log](old028-heavy13k-raw-compaction-profile.log)).

The CPU profile contains five calls to `AggregateUnaggregatedAttestations`: Go's one-iteration benchmark calibration, the reported three timed iterations, and one unconditional post-subtest compaction in the parent diagnostic benchmark. The profile therefore must not be divided by the reported `b.N=3` without first accounting for those two extra calls.

## Focused CPU attribution

The main compactor call tree sampled 4.50 CPU-seconds. Its detached `aggregateParallel` workers sampled another 1.73 CPU-seconds under the `sync.WaitGroup.Go` root, for a focused total of 6.23 CPU-seconds across five calls.

| Operation | Focused CPU | Share of 6.23 CPU-s |
|---|---:|---:|
| Serial `DeleteUnaggregatedAttestation` loop | 4.22 s | 67.7% |
| `insertSeenBit` within deletion | 4.09 s | 65.7% |
| `Bitlist.Contains` beneath seen-bit insertion | 3.85 s | 61.8% |
| Parallel `AggregateDisjointOneBitAtts` workers | 1.73 s | 27.8% |

The corresponding focused outputs are [main call tree](old028-heavy13k-raw-compaction.pprof-focus.txt), [worker call tree](old028-heavy13k-raw-compaction.pprof-workers.txt), [seen-bit source attribution](old028-heavy13k-raw-compaction.pprof-insert-seen.txt), and [`Contains` source attribution](old028-heavy13k-raw-compaction.pprof-contains.txt). These are CPU sample shares, not wall-stage timings: aggregation workers overlap, and fixture setup was removed by call-graph focus rather than a separate timer.

## Why cleanup is quadratic for this fixture

`insertSeenBit` reads the prior list for a data ID, checks every entry with `Bitlist.Contains`, and appends the incoming one-bit bitlist on a miss (`operations/attestations/kv/seen_bits.go:11-40`). It does not replace the list with a union. For the six fixture groups, serial deletion therefore performs

```text
4*C(2167,2) + 2*C(2166,2) = 14,076,834
```

containment checks. The pinned bitfield implementation scans bytes from index zero until it finds the missing bit (`github.com/OffchainLabs/go-bitfield` at `5cbb6d0f5f2e`, `bitlist.go:160-171`). With singleton positions `0..2166`, this is roughly 135 bytes per failed check on average, or about 1.9 billion byte comparisons. The profile's 3.85 CPU-seconds in `Contains` is consistent with that source-level count.

The specialized disjoint aggregation itself produces only six aggregates, but every original raw attestation is then deleted separately (`operations/attestations/kv/aggregated.go:49-61`). `SaveAggregatedAttestation` does not populate this seen-bit list, so it does not collapse the cleanup search space.

## Interpretation and limits

The measurement shows that the expensive part of this controlled 13k raw-compaction pass is serial seen-bit cleanup, rather than producing the six aggregate signatures. It explains how the scheduled T1 maintenance operation can remain CPU-heavy even when aggregation sounds structurally simple.

`insertSeenBit` runs before `DeleteUnaggregatedAttestation` acquires the raw-pool write lock (`unaggregated.go:130-152`). This profile therefore does **not** show a multi-second aggregate- or raw-pool critical section, and it does not prove that the slot-110 proposer waited on this cleanup. `DeleteSeenUnaggregatedAttestations`, run by separate expiry maintenance, does hold the raw write lock while scanning the raw map and consulting these seen lists (`unaggregated.go:155-176`), but retained metrics do not time that stage.

The production T1 histogram covers the full `batchForkChoiceAtts` pass, including this compaction, later forkchoice processing, mutex waits, and scheduler delay. The exact-old full-wrapper control measured 1.068264402 s/op, versus 1.054091085 s/op for the independently prepared raw-compactor arm; a direct precompacted continuation measured 0.218342 ms/op ([full-wrapper report](old028-batch-forkchoice-results.md), [five-iteration log](old028-batch-forkchoice-5x.log)). The difference between the two independently prepared one-second arms is not a reliable stage subtraction because setup, map ordering, and run noise differ. The direct continuation shows that, for this controlled six-group fixture, nearly all measured wrapper wall time is in raw compaction rather than the post-compaction continuation.
