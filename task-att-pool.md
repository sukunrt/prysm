# Classic attestation pool performance fixes

Status: implemented, reviewed, and validated. Updated 2026-10-05.

Reduce the classic pool's work for large committees by aggregating single votes
as they arrive and bounding the remaining max-cover aggregation. Adapt the
experimental pool's incremental aggregation approach to the classic pool's
locking, ownership, and consumers. Signer metadata propagation and activation
of the experimental pool are outside this task.

## Problem

The classic pool retains individual single attestations until periodic
compaction. Removing those singles builds a list of seen bitlists per data and
committee group; later lookups scan that list. Other consumers can also pass
uncompressed singles into general aggregation. Thousands of votes create
unnecessary storage, scans, containment checks, and cleanup work.

The general attestation aggregator repeatedly runs greedy max cover. Once
single votes are already combined, spending many rounds trying to improve the
remaining aggregates is unlikely to justify its cost. Performance tests must
measure the tradeoff rather than assume a speedup.

## Incremental aggregation and single vote membership

Use the existing attestation data ID, including version and committee grouping.
For each group, keep one running signed aggregate of accepted singles. The
first vote remains available as a singleton. When a second distinct vote
arrives, replace the singleton with the running aggregate. Subsequent votes
extend that aggregate directly, without calling max cover.

**Discard each original single as it is absorbed.** Do not retain a parallel
list of singles, schedule their individual deletion, or pass those originals
to downstream max cover. With 2,500 distinct singles for one group and no peer
aggregates, expose one valid aggregate with 2,500 participants and retain zero
original singleton records. This is an algorithmic stress case beyond the
mainnet per-committee bound, not a consensus-valid committee.

Maintain compact single-vote coverage per group. Once the incoming single's
committee position is known, membership is a map lookup and direct bit lookup,
independent of the number of cached attestations. Route both the subscriber's
HasAggregatedAttestation preflight and insertion through this path. Update
coverage when aggregates, block attestations, and processed entries change, and
expire it with the corresponding pool state.

Extract the set position from the incoming bitlist at the pool boundary. This
still scans the bitlist; hashing, copying, signature work, and the complete
ingestion path are not O(1). Preserving signer identity across validation is
explicitly deferred. Avoid repeatedly rediscovering the position within a
single operation.

Keep single-vote coverage separate from aggregate redundancy. Seen votes AB
and BC permit rejecting a duplicate single B, but must not make signed aggregate
ABC redundant. Retain peer aggregates according to actual signed-candidate
containment. Do not manufacture an aggregate by OR-ing overlapping signatures.

## Locking and integration

Synchronize duplicate detection, signature aggregation, and publication of the
resulting bits and signature. An aggregation error must leave the previous
candidate and coverage unchanged. Readers must receive stable snapshots:
adding a later vote must not mutate a previously returned attestation or the
bitlist recorded by fork choice as processed.

The implementation serializes single writers through 256 lock stripes and
computes BLS signatures outside the pool's map locks. It rechecks coverage and
the immutable candidate before committing. The decoded running signature is
cached alongside its candidate and removed with it, avoiding repeated decoding.

Update existing getters, counts, pruning, and batching to include running
candidates. A lone vote and later arrivals must remain available to fork choice,
aggregate publishers, proposers, and pool APIs. Preserve isolation between
data roots, committees, and versions using the data ID alone. Inputs are already
validated; bitlist length is not part of the key or revalidated by the pool.

Retire fully covered candidates. Partial coverage must not remove votes whose
signatures are inseparable from the running aggregate. After retirement, new
votes can start a new candidate while duplicates remain suppressed for the
existing processed-entry lifetime.

Remove obsolete batch compaction and per-single cleanup from the hot path.
Preserve the classic pool interface where practical; a compatibility method may
become a no-op if its work is completed during insertion.

## Bound max cover work

In MaxCoverAttestationAggregation:

- Run at most three outer aggregation rounds.
- Select at most three disjoint input candidates in each inner max-cover call.
- Return useful unconsumed candidates when the budget is exhausted, alongside
  new aggregates. The limit is not a three-result truncation.
- Preserve overlap checks, signature validity, and participant coverage. A
  union of separate candidates is not proof that one signed aggregate subsumes
  another.

Apply these limits in the attestation aggregation wrapper. Leave the generic
max-cover helper and the proposer's separate profitability selection unchanged.
The bounded search still scans its candidate list and bitlists; it does not
make the complete algorithm constant-time.

## Correctness and performance acceptance

- Verify real BLS signatures for incremental aggregation, duplicates, and
  bounded max-cover results. Include one-vote groups and 2,500-vote groups.
- Assert absorbed singles are absent from storage and consumer input, and
  single ingestion does not invoke general max cover.
- Exercise aggregate-first and single-first arrivals, overlapping peer
  aggregates, deletion, expiration, later votes, and separate groups. Check
  union coverage cannot incorrectly reject an aggregate.
- Test concurrent insertion, duplicates, reads, and pruning with the race
  detector. Old snapshots and fork-choice seen masks must stay unchanged.
- Check classic fork-choice batching and affected publisher, proposer, sync,
  and pool API tests. Update old singleton-count assumptions without weakening
  the behavioral assertions.
- Cover more than three possible selections and more than three possible
  rounds; stopping must preserve useful remaining candidates.

Add reproducible Go benchmarks using the same public pool APIs on the original
and changed implementation. Use committee sizes 250, 500, 1,000, and 2,500;
prepare valid signatures outside timing. Measure ingestion including subscriber
preflight, consumer aggregation, duplicate lookup and cleanup, and allocations.
Report retained candidate counts alongside timing. Include incremental BLS
costs in the full-path comparison rather than reporting lookup in isolation.

Incremental aggregation can reduce later combination opportunities: after A
and B become AB, A cannot be separately combined with BC. The bounded search
also trades some aggregate quality for predictable work. Tests must establish
validity and retained coverage; results must distinguish measured workloads
from production guarantees.

## Implementation and review

Use GPT-6 Sol agents for implementation and performance tests. Have a GPT-6
Astra agent at xhigh reasoning review the code against this task, then use
GPT-6 Sol agents to address the findings. Repeat review and validation until
the scoped work is complete.

Completed this implementation and review cycle. Astra's final review found no
remaining introduced correctness or locking issues, including the decoded
signature cache. All task edits remain in the single Jujutsu change
`ysvxuxmr`; no separate implementation commits were created.

Relevant code:

- [Classic pool](beacon-chain/operations/attestations/kv/kv.go)
- [Single storage](beacon-chain/operations/attestations/kv/unaggregated.go)
- [Seen bits](beacon-chain/operations/attestations/kv/seen_bits.go)
- [Aggregation and lookup](beacon-chain/operations/attestations/kv/aggregated.go)
- [Experimental reference](beacon-chain/cache/attestation.go)
- [Fork-choice batching](beacon-chain/operations/attestations/prepare_forkchoice.go)
- [Attestation max cover](proto/prysm/v1alpha1/attestation/aggregation/attestations/maxcover.go)

## Final performance results

Measured on the same machine with Go 1.26.5, comparing the final code against
the saved pre-change revision `8be05e8a` through a Go source overlay. Values
below are medians of three runs. Pool benchmarks use one complete workload per
run; lookup and max-cover benchmarks use 100 iterations per run. Signature
fixtures are prepared outside timing. These are synthetic workload measurements,
not production throughput guarantees.

| Workload | Original | Final |
|---|---:|---:|
| Full lifecycle, 250 singles | 12.30 ms | 10.97 ms |
| Full lifecycle, 500 singles | 26.30 ms | 21.13 ms |
| Full lifecycle, 1,000 singles | 62.67 ms | 45.80 ms |
| Full lifecycle, 2,500 singles | 322.34 ms | 105.52 ms |
| Concurrent ingress and flush, 16 groups of 250 | 72.61 ms | 28.68 ms |
| Ingress and consumer before flush, 250 singles | 10.41 ms | 10.54 ms |
| Ingress and consumer before flush, 500 singles | 21.76 ms | 20.92 ms |
| Ingress and consumer before flush, 1,000 singles | 56.32 ms | 41.63 ms |
| Ingress and consumer before flush, 2,500 singles | 399.62 ms | 103.89 ms |

The 2,500-vote lifecycle is 3.05 times faster and allocates 17.55 MB instead of
53.30 MB. Immediately after ingestion, original singleton storage falls from
2,500 entries to zero, with one retained aggregate. The concurrent workload
retains 16 aggregates instead of 4,000 singles and is 2.53 times faster. The
smallest consumer-before-flush case is effectively unchanged in this sample.

Seen lookup uses a fixed 4,096-bit mask and an absent single, varying the number
of processed aggregate masks. It includes hashing and position extraction, so
the near-flat result isolates removal of the growing candidate-list scan.

| Cached masks | Original lookup | Final lookup |
|---|---:|---:|
| 250 | 54.7 µs | 2.7 µs |
| 500 | 102.8 µs | 2.9 µs |
| 1,000 | 243.8 µs | 2.9 µs |
| 2,500 | 628.3 µs | 3.3 µs |

The max-cover benchmark uses actual BLS signatures and four participants per
input aggregate. All output signatures and total participant coverage are
verified. The cap deliberately reduces the size of the best combined result:

| Input aggregates | Original time | Final time | Largest result, original → final |
|---|---:|---:|---:|
| 3 disjoint | 0.089 ms | 0.086 ms | 12 → 12 |
| 10 disjoint | 0.308 ms | 0.273 ms | 40 → 12 |
| 64 disjoint | 2.025 ms | 0.315 ms | 256 → 12 |
| 3 overlapping | 0.056 ms | 0.070 ms | 8 → 8 |
| 10 overlapping | 0.366 ms | 0.320 ms | 20 → 12 |
| 64 overlapping | 1.927 ms | 0.326 ms | 128 → 12 |

The three-input overlapping case measured slower; this change does not promise
a speedup for every aggregate set. For 64 disjoint inputs, all 256 participants
remain represented across 58 output candidates rather than one large candidate.

## Final validation

After the machine restart, all of these checks passed on the final code with
`GOTOOLCHAIN=go1.26.5 go test -mod=readonly -count=1 -p=2`:

- Full classic pool, attestation service, and attestation aggregation unit suites
  with `-tags=develop`.
- Focused `-race` checks for incremental insertion, duplicates, pruning,
  snapshots, 2,500-vote collapse, and fork-choice batching.
- REST aggregate retrieval and pool listing tests.
- Aggregate publisher and proposer packing tests with `-tags=develop,minimal`.
- Relevant sync pending-attestation, subscriber, block pruning, and reorg tests.

Formatting checks are clean and changed Bazel source lists include the new
benchmark files. Bazel itself was unavailable, so Bazel execution was not run.
Three pre-existing REST submission subtests
(`TestSubmitAttestationsV2/post-electra/{single,ssz,multiple}`) failed because of
empty committee fixtures on both the original and changed implementation; they
are outside this fix and are not included in the passing checks above.

Benchmark sources:

- [Pool benchmarks and immediate-collapse regression](beacon-chain/operations/attestations/kv/classic_pool_perf_test.go)
- [Bounded max-cover benchmarks](proto/prysm/v1alpha1/attestation/aggregation/attestations/maxcover_bounded_bench_test.go)

Exact test commands, benchmark commands, baseline overlay, raw logs, and median
metrics are saved in
`/home/sukun/.cache/prysm-att-pool-review/2026-10-05/`. The benchmark commands use
`-count=3 -run '^$'`, the same toolchain and build tags as above, and
`-benchtime=1x` for pool workloads or `-benchtime=100x` for lookup/max cover.
