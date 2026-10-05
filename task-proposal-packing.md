# Proposal attestation packing CPU fix

Status: implementation, tests, benchmarks, review, and eight-slot Kurtosis acceptance complete. Updated 2026-10-05. Changes are uncommitted.

## Objective

Reduce the remaining CPU cost of `packAttestations` after the classic pool's
incremental single aggregation changes. Measure the current implementation
before changing it. Preserve valid signed candidates, committee isolation,
inclusion limits, and proposer reward ordering.

This implements item 1 from the proposal-performance discussion. Cancellation,
pool lock restructuring, slot-start expiry, and execution-payload recovery are
separate work. Keep this change local to proposer packing unless measurements
establish that a directly called helper must also change.

## Baseline and workload

Use the current parent revision, including incremental aggregation and bounded
max cover, as the baseline. Save a reproducible source overlay or equivalent
before changing production code. Run the same benchmark code, Go version,
build tags, and fixtures against baseline and changed production sources.

Exercise the actual classic pool ingress and getter APIs. Populate the pool
outside the timed packing region, and record the retained candidate counts.
Do not describe thousands of separately retained singles as the current normal
pool workload. Keep that shape only as a clearly labelled algorithm stress test.

Measure these representative shapes:

- Incrementally aggregated singles across several committees and data groups,
  including singleton groups and both eligible rounds.
- Running aggregates mixed with peer aggregates: exact duplicates, strict
  subsets, disjoint candidates, and overlapping incomparable candidates.
- Many equal-size distinct peer candidates and varying-size incomparable
  candidates, to expose remaining containment work.
- Small pools, empty pools, and invalid/expired candidates where practical.

Use structurally valid fork-specific fixtures and real BLS signatures for
end-to-end packing. A valid benchmark must assert nonempty output when expected,
retained participant coverage before block inclusion limits, and signature
validity. Explain any artificial committee sizes or bypassed production work.

Report full-packer time and allocations. Attribute time to snapshot/validation,
deduplication, aggregation/on-chain construction, and reward sorting/signature
filtering using focused stage benchmarks or a CPU profile with enough repeated
packing work to obtain useful samples. Exclude fixture and key generation from
the measured region. Avoid production instrumentation solely for this task.

## Proposed implementation

The main candidate is `proposerAtts.dedup`: it currently performs pairwise
containment checks per attestation data ID and is called twice during packing.

Implement a cardinality-aware containment filter, subject to baseline evidence:

1. Keep `attestation.NewId(att, attestation.Data)` grouping so versions, data,
   and committee sets remain distinct. Keep fast paths for small groups.
2. Compute each candidate's participant count once. Remove exact duplicate
   bitlists with an exact key or collision-checked hash; do not key by signature.
3. Consider unique candidates in descending participant count. Only a strictly
   larger retained candidate can contain a candidate with different bits.
   Equal-size distinct candidates need no containment scan against one another.
4. Retain a candidate unless one actual signed candidate contains all its bits.
   The union of AB and BC must never cause ABC to be discarded.
5. Keep returned objects and signatures intact. Do not mutate input attestation
   objects or pool snapshots. Slice reordering is allowed where callers already
   allow it. Preserve malformed-input error handling, including incompatible
   bitlist lengths within a group; do not hide errors behind count shortcuts.

Per the user's later direction, preserving the old behavior of malformed
zero-length bitlists is outside this change's acceptance criteria.

This removes unnecessary equal-size and duplicate comparisons; it does not
claim a universal linear bound for arbitrary different-size incomparable sets.
Measure those sets too. The three-round aggregation limit does not bound all
packing work, and the block attestation limit must not become an early arbitrary
candidate truncation.

If profiles show a larger cost in redundant grouping, validation, construction,
or reward computation, make a small directly supported improvement there too
and explain it in the results. Preserve reward semantics and validity checks.
Do not remove the second dedup pass without proving its input cannot contain
redundant candidates. Do not add a persistent cache without a demonstrated need
and a correct state/invalidation key.

## Correctness acceptance

- Existing proposer packing, deduplication, aggregate construction, and reward
  ordering tests pass with required build tags.
- Add differential/property-style tests against the original containment
  behavior for deterministic generated sets. Compare maximal signed bitsets
  per group, not incidental map iteration order or equivalent duplicate
  signatures. Cover shuffled inputs, duplicates, subset chains, equal-size
  incomparable sets, varying-size incomparable sets, and union traps.
- Cover Phase0 and Electra-shaped data, committee and version isolation,
  multi-committee on-chain candidates, empty/singleton inputs, and malformed
  inputs relevant to the existing function contract.
- Verify real signatures and coverage for representative complete packing
  fixtures. Account explicitly for normal block inclusion limits.
- Verify returned snapshots remain unchanged and run focused race tests for
  new tests that exercise shared pool state, if any.

## Performance acceptance

Run repeated before/after benchmarks on the same machine (at least three
samples). Report medians, allocations, candidate counts, exact commands, and
the baseline revision. Preserve raw results outside the source tree if large.

Demonstrate a repeatable improvement in the targeted duplicate/containment
workload and check that representative compact pools do not materially regress.
Investigate regressions rather than selecting only favorable cases. Report
full-packer improvements separately from isolated dedup improvements; do not
promise a simulation speedup from a synthetic benchmark.

## Delivery and review

Use GPT-6 Sol for implementation and fixes. Use GPT-6 Astra at xhigh for review.
The user's latest instruction is to keep looping: address review findings and
repeat review until no actionable findings remain. Record the review outcome,
test results, measured limits, and final implementation here. Update `todo.md`
only for the part actually completed; leave cancellation, lock scope, and
slot-start expiry outstanding.

Use the repository's `test` skill for Go tests. Update Bazel source/dependency
lists if files change. Leave the work uncommitted unless separately requested;
the repository requires `precheck` before a commit.

## Kurtosis acceptance

After benchmarks and review fixes, build the final working tree into distinctly
tagged local images. Run ten Prysm beacon/validator pairs with 100 validator
keys per pair (1,000 total), using the repository's Heze genesis configuration
and pinned execution client. Record source hashes, image IDs, package revision,
arguments, and toolchain so the actual tested build is identifiable.

Observe eight slots, as requested by the user. Verify all ten nodes agree on
a common block root, the active registry has exactly 1,000 validators, and blocks
continue to be proposed with attestations. Capture construction timings and
proposal failures. Record justification/finalization status, but do not require
or claim a finalization proof from this short startup window.

Drive nonempty execution payloads with the existing Spamoor transaction/blob
setup. Verify the actual zero-padded client names. For at least one slot, record
EL transaction count, beacon attestation count, payload-attestation count, FFG
aggregate ledger evidence, and PTC vote ledger evidence. Retain logs, REST
responses, and a concise report. Stop the test enclave after collecting evidence.

The network run establishes integration behavior at this scale. CPU improvement
must be supported by the paired benchmarks; a healthy small devnet alone does
not establish throughput or latency at the earlier large simulation scale.

## Implementation and measured result

`proposerAtts.dedup` keeps the original pairwise algorithm for groups of at
most 32 candidates. Larger groups remove exact bitlist duplicates, cache
participant counts, and test containment only against strictly larger retained
signed candidates. It checks semantic bitlist lengths across each larger group
before count-based shortcuts. The second dedup pass remains. The baseline CPU
profile showed reward scoring as the larger remaining cost, so Electra reward
scoring now iterates state-derived committee participants directly instead of
sorting and compacting their validator indices before summing. Phase0 keeps
the original scoring path. Reward order, inclusion limit, and signature filter
remain in place.

The paired baseline is parent `yzmrxpwq` (`07257517`), with the original
proposer and reward-helper files saved in a Go source overlay. Both sides used
Go 1.26.5, `go test -mod=readonly`, the same benchmark source and fixtures,
three samples, 200 ms benchmark time, and the same Ryzen 7 7840U machine.
Raw outputs, profile, and replayable overlays are in
`/home/sukun/.cache/prysm-proposal-packing/2026-10-05/benchmarks/`.

The main signed fixture uses a Heze state with 80,000 active validators so
slot 13 has ten distinct committees of 1,000. All 10,000 participants sign
real BLS votes. The compact case inserts all singles through the classic pool
and retains ten running aggregates. The mixed case inserts 500 singles per
committee plus sixteen overlapping peer aggregates per committee, constructed
from all 1,000 signed participants; it retains 170 candidates. Peer candidates
include equal and varying sizes and remain incomparable. The same attestation
data root allows normal multi-committee on-chain construction. Both cases
preserve 10,000 unique participant bits through deduplication, aggregation,
and on-chain construction. Compact packs one attestation and mixed packs two;
both cover all 10,000 unique participants after the block limit. Returned
signatures pass batch BLS verification outside timing. The timed packer uses
the normal matching-target fast path in its final signature filter; these
numbers do not include repeated batch BLS verification. Fixture/key generation
and ingestion are outside timed regions. A separate 1,000-validator, 125-bit-committee
Electra fixture exercises compact (four retained candidates) and mixed
(nine retained candidates) pools, including a singleton and both rounds.

| Benchmark | Baseline median | Changed median | Baseline → changed bytes/op | Baseline → changed allocs/op |
| --- | ---: | ---: | ---: | ---: |
| 10k full pack, compact | 1.227 ms | 1.172 ms | 670,274 → 670,187 | 714 → 714 |
| 10k full pack, mixed peers | 10.177 ms | 7.410 ms | 5,595,402 → 5,333,170 | 23,232 → 23,210 |
| 10k isolated dedup, mixed peers | 221.6 µs | 190.2 µs | 262,213 → 262,216 | 2,104 → 2,104 |
| 1k full pack, compact | 327.1 µs | 325.6 µs | 47,743 → 43,157 | 554 → 546 |
| 1k full pack, mixed peers | 491.1 µs | 514.7 µs | 95,741 → 88,452 | 1,081 → 1,069 |
| 500 equal-size incomparable candidates, dedup only | 5.253 ms | 0.553 ms | 537,278 → 653,390 | 6,016 → 6,524 |
| 500 varying-size incomparable candidates, dedup only | 3.958 ms | 2.892 ms | 537,273 → 653,380 | 6,016 → 6,524 |

The 500-candidate tests use placeholder signatures and bypass the pool; they
measure only containment work, not block packing. The small mixed full-packer
case is about 24 µs (4.8%) slower across these samples despite lower bytes and
allocation counts. The large mixed full pack is 27.2% faster. These figures
are fixture-specific CPU measurements, not a predicted simulation speedup.

The baseline production-only CPU profile repeated 500 mixed packing calls.
Reward sorting/scoring accounted for about 38% of sampled CPU cumulatively;
pool attestation validation and index construction about 29%; on-chain
construction about 17%. Isolated dedup was only around 2% of the baseline
large mixed full-pack time, which is why the directly called reward helper was
also changed. An older diagnostic full-packer fixture failed to construct its
Heze state under the minimal build due to sync-committee length, so it was
excluded from performance claims. Earlier raw-single stress evidence in
[the old packing benchmarks](runs/diagnostics/packing_bench_results.md) reflects
a pool shape the incremental single aggregation no longer normally retains.
The [historical compact-pool diagnostics](runs/diagnostics/round2-construction-bench/results.md)
also independently measured reward sorting at 9.883 ms and direct scoring of
29,847 already-credited participant positions at 5.188 ms. Those compact-pool
measurements support the reward-helper optimization; they are distinct from
the seconds-long raw-single deduplication stress cases.

Exact baseline benchmark commands from the repository root. Run each command
again without `-overlay=...` to measure the changed tree:

```bash
GOTOOLCHAIN=go1.26.5 go test -mod=readonly -overlay=/home/sukun/.cache/prysm-proposal-packing/2026-10-05/benchmarks/baseline/overlay-both.json -run '^$' -bench '^BenchmarkProposalPackingLarge(ClassicPool|Dedup)$' -benchtime=200ms -count=3 -benchmem -v ./beacon-chain/rpc/prysm/v1alpha1/validator
GOTOOLCHAIN=go1.26.5 go test -mod=readonly -overlay=/home/sukun/.cache/prysm-proposal-packing/2026-10-05/benchmarks/baseline/overlay-both.json -tags=minimal -run '^$' -bench '^BenchmarkProposalPacking(ClassicPool|DedupAlgorithm)$' -benchtime=200ms -count=3 -benchmem -v ./beacon-chain/rpc/prysm/v1alpha1/validator
```

Targeted proposer packing and differential tests passed with `-tags=minimal`;
the signed large-fixture and reward-equivalence tests passed in the mainnet
build; all `./beacon-chain/core/electra` tests passed. The differential tests
compare maximal signed bitsets against the old algorithm for deterministic
shuffled Phase0 and Electra groups at and across the 32-candidate threshold,
including duplicates, subset chains, incomparable sets, committee/version
isolation, union traps, incompatible lengths, and multi-committee candidates.
Fixture checks also assert that pool snapshots are unchanged. No shared pool
state is exercised concurrently by the new tests. GPT-6 Astra xhigh review
completed with no actionable findings after the corrected fixtures, coverage
checks, and reward-equivalence test; the user explicitly removed the
malformed-zero-length compatibility finding from scope.

## Integration result

The final reviewed images passed the requested ten-node, 1,000-validator
Kurtosis run over slots 1–8. All nodes agreed on the slot-8 block, were fully
synced and non-optimistic, and reported 1,000 active validators. Every block
included attestations and a payload attestation; execution transactions and
blobs were confirmed. Blocks 2–8 preserved all 125 fresh committee participants.
Block 1 included 13 slot-0 participants, consistent with the separate slot-0
acceptance issue. There were no proposal failures; median measured block
construction was 22.34 ms and maximum 43.48 ms. The enclave is stopped.

See [the integration report](runs/diagnostics/proposal-packing-kurtosis.md)
for the per-slot results, end-to-end FFG/PTC/EL evidence, build provenance,
collector correction and startup log messages. Eight startup slots do not
establish finalization or a network-scale CPU speedup.

Final Go checks, all passed with `GOTOOLCHAIN=go1.26.5`:

```bash
go test -mod=readonly -tags=minimal -count=1 -run '^(TestProposalPacking|TestProposer_ProposerAtts|TestProposer_sort|Test_packAttestations|TestPackAttestations|Test_computeOnChainAggregate)' ./beacon-chain/rpc/prysm/v1alpha1/validator
go test -mod=readonly -count=1 -run '^TestProposalPacking' ./beacon-chain/rpc/prysm/v1alpha1/validator
go test -mod=readonly -count=1 ./beacon-chain/core/electra
```

Bazel source/dependency entries were updated. Bazel itself was not available,
so validation used Go tests and successful final beacon, validator, and
genesis-tool builds. No commit was made.

## Remote Shadow result

The requested 50-node run completed eight slots with 10,000 active validators,
200 per node, split into 40 home nodes and 10 supernodes. All nodes agreed on
slot 8 and imported all eight blocks and execution payloads. Blocks 2–8 retained
all 1,250 fresh participants. No proposal failures or positive-depth reorgs
were observed. Worst payload arrival was 4.882 seconds on home nodes and
0.609 seconds on supernodes.

The 49 captured metrics snapshots show 1,862 undeliverable payload-attestation
messages and 242 sync-committee messages, leaving queue pressure as a separate
follow-up. One final metrics fetch was cut off at shutdown, so Shadow exited 1
despite successful chain checks. The run is stopped. See
[the remote Shadow report](runs/diagnostics/proposal-packing-shadow.md) for
end-to-end block evidence, configuration, provenance and collection limits.
Shadow's simulated proposal timings are not CPU performance measurements.
