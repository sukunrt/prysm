# Three-slot attestation retention for Heze

Status: implemented and validated remotely, uncommitted. Updated 2026-10-05. This extends the
completed packing CPU fix; preserve its existing uncommitted changes.

## Objective and policy

Bound live proposal/aggregation storage and avoid repeatedly copying, validating,
aggregating and scoring old attestations. Heze gossip already restricts votes
to their own slot plus clock tolerance, while pool expiry remains epoch-based.
The user selected three preceding slots as an inclusion grace period and
explicitly requested deleting older entries, rather than only filtering reads.

During current slot S, retain candidates from max(0, S-3) onward: the current
slot's incoming votes and the preceding three slots. A block proposed at B may
select only B-3 <= attestation.slot < B, with a saturating lower bound at zero.
Use the attestation's data slot, not the slot of the block it votes for. Slot-7
votes for a slot-6 head remain slot-7 candidates. Do not impose a limit of three
aggregate objects; distinct signed data may require multiple aggregates.

Apply the shorter retention only once Heze is active according to the current
slot, and the packing window according to the proposed block's fork. Preserve
pre-Heze expiration and selection. At Heze activation, old pending candidates
become subject to the new retention window. Slot zero is eligible for blocks
1, 2 and 3 under this policy and expires at current slot 4. The separate slot-0
gossip acceptance change is outside this task.

This is local retention/proposal policy, not a consensus validity change.
Attestations included in received blocks still undergo normal validation and
processing. A missed-block period longer than the window can lose inclusion
opportunities; that is the accepted tradeoff. Gossip rules, payload-attestation
selection, Goldfish votes and existing reward semantics are outside scope.

## Implementation requirements

1. Add efficient slot-based pruning to the classic pool and reuse/extend the
   experimental cache's equivalent. Retire whole expired data groups directly
   under existing locks; do not clone every attestation or do BLS work merely
   to delete it. Clear associated running signatures, coverage, singleton
   indexes and slot-associated duplicate bookkeeping where applicable. Existing
   independently bounded caches may retain their normal expiry if needed;
   document that choice and avoid stale references to deleted groups.
2. Prune at slot start, including service startup at an already advanced slot.
   Use a monotonic retention cutoff protected consistently with mutations.
   Accepted candidates remain available across the exact age-3 boundary.
   Existing pre-Heze scheduling can remain unchanged.
3. Expired in-flight and later submissions must not repopulate proposal storage.
   Check the cutoff again at commit points after any work performed outside a
   lock, including incremental single promotion and peer aggregate insertion.
   An expired insertion is a harmless no-op, not an invalid-message rejection.
   Avoid new lock-order inversions and avoid serializing BLS work globally.
4. Keep a cheap Heze packing-window guard before expensive attestation validation
   and aggregation. It uses the requested block slot and protects against a
   delayed pruning tick or a pool snapshot taken across a slot boundary. A
   packing read must not advance the global cutoff based on a future request.
5. Prune proposal storage only. Do not add expired votes to fork choice or
   change its independently maintained queue and processed state. Imported
   blocks continue normal consensus processing; expired proposal candidates
   simply lose local inclusion opportunities. Document the exact affected
   storage and any independently retained entries.
6. Update pool interfaces, mocks, metrics and Bazel source/dependency declarations
   as needed. Keep the implementation small; do not add a new public CLI knob
   or redesign unrelated pool behavior.

## Validation and performance

Follow `.agents/skills/test/SKILL.md` using Go 1.26.5 and `-mod=readonly`.
No commit is requested. Tests should exercise behavior rather than copy the
implementation, covering both pools and the service integration:

- Exact ages 0, 1, 2, 3 and 4; slots 0-4; same/future-slot candidates excluded
  from packing; Heze activation and round boundaries; missed proposal slots.
- Aggregated peers, running single aggregates, singleton groups and bookkeeping
  cleanup; repeated pruning and a non-regressing cutoff.
- Writes after expiry, and writes racing with prune or promoting a singleton
  while signatures are aggregated outside locks. Use focused race tests where
  feasible and deterministic synchronization where possible.
- The pre-existing fork-choice queue remains untouched by proposal pruning.
  Received blocks retain their normal consensus processing path.
- All eligible fresh candidates preserve signatures and participant coverage;
  pool snapshots remain safe for existing consumers. Pre-Heze behavior remains.

Keep the signed benchmark with 10,000 participants across ten committees and
its signature/coverage assertions. Add a representative mixed-age workload
with expired groups plus retained fresh participants and report candidate
counts. Include steady-state prune cost and allocations; fixture construction
and signatures must remain outside timed sections. Do not claim pruning gains
by timing an empty pool. If expected older output changes in existing fixtures,
update those expectations explicitly rather than hiding the removed work.

The baseline source snapshot, including the completed packing optimization, is
`/home/sukun/.cache/prysm-attestation-retention/2026-10-05/baseline/`.
Use source overlays or an isolated copy for paired measurements and preserve
raw commands/results under the same task cache. Report when compared outputs
differ because of the intended retention policy. No additional network run is
required for the implementation/review pass; previous network results apply
to the earlier build until the changed binaries are exercised again.

## Implementation and review sequence

1. GPT-6 Sol implements the spec, adds meaningful tests and benchmarks, runs
   relevant checks, and records the results here.
2. GPT-6 Astra with xhigh reasoning performs one independent review against this
   spec and the saved baseline, including concurrency and fork-choice scope.
3. GPT-6 Sol addresses that review and reruns affected validation. Record each
   finding's disposition. Do not start another review cycle without a request.

Leave source changes uncommitted and preserve unrelated diagnostics.

## Implementation and review results

The final scope is Heze proposal storage only. At slot start, the classic pool
removes old aggregated groups, running signatures, singleton indexes,
unaggregated entries, block fallback groups, coverage maps and slot-associated
seen-aggregate bookkeeping. The experimental cache removes old proposal groups.
The independently time-bounded classic `seenAtt` and `seenSingleAtt` bit caches
keep their existing expiry; they do not reference deleted proposal groups.
The fork-choice queue and processed-vote state are unchanged, and pruning adds
nothing to that queue. An old vote can lose a local inclusion opportunity as
specified; normal validation of a vote in a received block is unchanged.

The review's unconditional handoff concern is resolved by removing handoff and
prepared-coverage bookkeeping entirely. The experimental cache retains its
separate pre-Heze expiry path and uses a monotonic cutoff only for Heze.
The legacy orphan-path race finding no longer applies because there is no new
expired-vote handoff to preserve. Classic Heze pruning returns per-kind removal
counts for the existing expired counters. Tests cover exact age boundaries,
cutoff monotonicity, concurrent and paused insertions, pre-Heze admission,
Heze activation, startup and slot ticks, and unchanged pre-existing fork-choice
queue contents.

GPT-6 Sol implemented and addressed one GPT-6 Astra xhigh review. The final
scope correction removed the unnecessary fork-choice additions; there was no
second review. The saved baseline and earlier packing optimization remain intact.

## Final validation

All final checks passed on `sukun@ethp2p`, in
`/home/sukun/dev/prysm-retention-20261005`, with `GOTOOLCHAIN=go1.26.5`,
`GOMAXPROCS=8`, and `-mod=readonly`:

```sh
go test -mod=readonly -count=1 ./beacon-chain/operations/attestations/... ./beacon-chain/cache
go test -mod=readonly -count=1 -run 'Test(ProposalPackingMixedAgeRetention|ProposalWindowHeze|DiagnosticFullPackingFixtureRoundSemantics)' ./beacon-chain/rpc/prysm/v1alpha1/validator
go test -mod=readonly -race -count=1 -run 'Test(PruneBefore|AttestationCacheRetention|HezeRetentionAtSlotStart)' ./beacon-chain/operations/attestations/kv ./beacon-chain/operations/attestations ./beacon-chain/cache
```

The signed mixed-age benchmark has ten fresh committees with 1,000 participants
each and 30 additional valid older aggregates. Pruning reduced proposal
candidates from 40 to 10, added zero fork-choice entries, and retained all
10,000 fresh participants with valid packed signatures. The saved baseline
packed four slot groups; the new policy intentionally packs only the fresh one.

On the same remote host, 100 packing iterations took 26.73–27.73 ms/op on the
baseline (three samples, median 27.24 ms) and 3.03 ms/op on the final narrowed
patch (one sample). Allocations fell from about 7.82 MB / 43,215 per operation
to 0.669 MB / 709. This is a descriptive comparison with intentionally different
eligible output, not a statistical significance claim. Commands used
`-run '^$' -benchtime=100x -benchmem`, with
`BenchmarkProposalPackingMixedAgeUnpruned` on the baseline overlay and
`BenchmarkProposalPackingMixedAgeRetained` on the final patch.

The final `BenchmarkPruneSteadySlots`, over three 100-iteration samples, retired
ten groups per slot while retaining 40, taking 2.47–2.83 microseconds/op
(median 2.76) with zero allocations. It measures deletion only. Fixture creation
is outside the timer. `BenchmarkPruneBeforeMixedAge` also passed with a nonempty
60-to-30 candidate prune and zero allocations per operation.

Raw final logs are under
`/home/sukun/.cache/prysm-attestation-retention/2026-10-05/remote-results/narrow-*.txt`.
The source manifest is `narrow-source-manifest.json` in the parent task cache;
`baseline-mixed.txt` records the baseline benchmark. Earlier `final-*` and
`current-*` logs describe the superseded handoff implementation. Local timings
were contended and are not used for performance conclusions.

Earlier iterations had an aggregate-order assertion failure that passed on
retry, ticker configuration/scheduling failures that were fixed, and a historical
fixture expecting an age-seven vote that was updated for the agreed policy.
The final narrowed checks above all passed on their first remote run. Bazel was
not run because its executable is unavailable. No new network simulation was
run for this change; earlier network results apply to the earlier build.
