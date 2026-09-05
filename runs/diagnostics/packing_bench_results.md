# Attestation packing and aggregation diagnostics

Date: 2026-09-05. Host: AMD Ryzen 7 7840U, Linux amd64,
`GOMAXPROCS=4`, `-tags develop`.

## Scope and fixture

`packing_diagnostic_bench_test.go` benchmarks three isolated operations on
Electra attestations:

- proposer `proposerAtts.dedup`;
- `attaggregation.Aggregate` (maximum-cover aggregation);
- `AggregateDisjointOneBitAtts`;
- the aggregation RPC's `mergeByData`, sequentially and with 4 or 16
  simultaneous requests.

The disjoint fixture has one single-bit attestation per received candidate, one
attestation-data root, a fixed 2,500-seat committee bitlist at every candidate
count, and a valid reusable BLS signature. The overlapping fixture uses adjacent
two-bit aggregates in the same fixed-width bitlist. Fixture creation is outside
benchmark timing. Inputs are synthetic and deliberately include full-committee stress
sizes; they do not establish that the run's pools contained those counts.

## Commands

```sh
GOMAXPROCS=4 /home/sukun/dev/go/bin/go test -tags develop \
  ./beacon-chain/rpc/prysm/v1alpha1/validator -run '^$' \
  -bench 'BenchmarkDiagnosticPackingDisjointSingles$' \
  -benchtime=1x -count=3 -benchmem

GOMAXPROCS=4 /home/sukun/dev/go/bin/go test -tags develop \
  ./beacon-chain/rpc/prysm/v1alpha1/validator -run '^$' \
  -bench 'BenchmarkDiagnosticPackingMergeByData$' \
  -benchtime=1x -count=3 -benchmem

GOMAXPROCS=4 /home/sukun/dev/go/bin/go test -tags develop \
  ./beacon-chain/rpc/prysm/v1alpha1/validator -run '^$' \
  -bench 'BenchmarkDiagnosticPacking(OverlappingAggregates|SixCommitteesDedup)$' \
  -benchtime=1x -count=3 -benchmem
```

## Disjoint singles

Ranges are three one-iteration samples.

| candidates | dedup | max cover | disjoint-one-bit |
|---:|---:|---:|---:|
| 10 | 17-50 us | 0.267-0.286 ms | 0.260-0.274 ms |
| 50 | 0.078-0.104 ms | 1.33-1.91 ms | 1.27-1.42 ms |
| 100 | 0.150-0.166 ms | 2.86-3.07 ms | 2.45-2.48 ms |
| 250 | 0.76-1.05 ms | 9.09-9.59 ms | 6.24-6.96 ms |
| 500 | 3.83-4.29 ms | 23.4-25.0 ms | 12.2-12.4 ms |
| 1,000 | 26.2-28.9 ms | 67.6-67.8 ms | 24.5-25.1 ms |
| 2,500 | 0.381-0.455 s | 0.339-0.340 s | 0.062-0.063 s |

Dedup's nested containment comparisons show pronounced superlinear growth:
roughly 1 ms at 250 becomes roughly 0.4 s at 2,500. Max cover also becomes
hundreds of milliseconds for a full 2,500-seat committee. The specialized
disjoint-one-bit path scales much better at that size, though BLS signature
aggregation remains material.

Six separate 2,500-seat data groups (15,000 total candidates) take
2.73-2.80 seconds in proposer dedup, about six times the one-group cost. This
is a deliberate all-seats stress case, not an observed pool size.

Adjacent two-bit overlapping aggregates have similar dedup behavior. Max cover
takes 5.11-6.48 ms at 250, 14.3-15.6 ms at 500, and 45.0-46.7 ms at 1,000.

## Aggregation RPC `mergeByData`

`SubmitAggregateSelectionProofElectra` calls `mergeByData` for each aggregation
duty before selecting the best result. It clones each input, groups by data,
and invokes max-cover aggregation. Wall times follow.

| candidates/request | sequential | 4 concurrent | 16 concurrent |
|---:|---:|---:|---:|
| 10 | 0.295-0.325 ms | 0.482-0.657 ms | 1.19-1.48 ms |
| 50 | 1.41-1.96 ms | 1.49-1.78 ms | 7.04-8.84 ms |
| 100 | 3.19-3.43 ms | 3.18-4.44 ms | 15.4-16.2 ms |
| 250 | 9.11-10.0 ms | 10.5-12.0 ms | 46.3-48.0 ms |
| 500 | 25.3-26.6 ms | 29.5-30.9 ms | 115-133 ms |
| 1,000 | 72.9-74.8 ms | 85.6-92.0 ms | 341-358 ms |
| 2,500 | 0.357-0.368 s | 0.398-0.410 s | 1.65-1.73 s |

At 2,500 candidates, 16 requests allocate about 160.6 MB and 1.12 million
objects. Thus full-committee request fanout can consume seconds and significant
CPU/GC. Round-1 slot-1 summary lines report 26-231 accepted gossip messages per
sampled node over six subnets, with at most 49 in one subnet. These are
pre-pool accepted-gossip counts: they exclude locally produced attestations and
do not bound the pool contents or the input to one aggregation request. They
are only a scale comparison. At 50 candidates, `mergeByData` takes about
1.4-2.0 ms sequential, while 16 simultaneous requests finish in about
7.0-8.8 ms. The captured summaries therefore do not provide evidence for the
hundreds or thousands of candidates needed by the slow benchmark cases, but
they also cannot rule such pool inputs out.

## Conclusion

There is an O(n-squared)-shaped proposer dedup risk and a substantial
full-committee max-cover/`mergeByData` cost. They can explain seconds only if
candidate groups actually reach the high hundreds or thousands, or many large
aggregation requests arrive together. The available slot-1 summary counts are
orders of magnitude smaller, so the benchmark establishes a latent scaling
hazard rather than causality for the observed startup incident. Round 2 lacks
the summary log, which is absence of evidence rather than evidence of either a
small or large pool.
