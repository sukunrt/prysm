# Three-slot attestation retention for Heze

Status: implemented, unit/race and 50-node Shadow validation passed, uncommitted. Updated 2026-10-06. This extends the
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
differ because of the intended retention policy. The subsequently requested
50-node Shadow verification of these changed binaries is recorded below.

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
not run because its executable is unavailable.

## 50-node Shadow verification

The final narrowed implementation passed the requested remote eight-slot run:
50 nodes, 10,000 active validators, 200 per node, 40 home nodes and 10 supernodes.
Fresh beacon, validator and genesis-tool binaries were built with Go 1.26.5
after verifying all 4,882 source hashes. The run used Heze from genesis, the
classic pool, seed 1, eight-slot rounds and 12-second slots. Home bandwidth was
25 Mbit/s up and 50 down; supernodes used 1,024 Mbit/s both ways. The network's
committee size was 1,250 per slot, distinct from the ten-committee CPU fixture.

Shadow, the retention verifier's 19 checks, and the repository summary verifier
all exited 0. All 50 nodes agreed on slot 8, with zero sync distance, no optimistic
heads, and their execution clients online. Every node imported all eight blocks
and payloads: 400/400 of each. There were no proposal failures or positive-depth
reorgs. All 50 final metrics and pool captures completed before shutdown.

| Block | FFG aggregates | Included attestation slots | Fresh participants | Payload attestations | EL transactions | Blobs |
| ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 1 | 0 | 27 / 1,250 | 1 | 0 | 0 |
| 2 | 3 | 1, 1, 0 | 1,250 / 1,250 | 1 | 6 | 3 |
| 3 | 3 | 2, 2, 0 | 1,250 / 1,250 | 1 | 7 | 3 |
| 4 | 2 | 3, 3 | 1,250 / 1,250 | 1 | 10 | 6 |
| 5 | 2 | 4, 4 | 1,250 / 1,250 | 1 | 5 | 0 |
| 6 | 2 | 5, 5 | 1,250 / 1,250 | 1 | 10 | 6 |
| 7 | 2 | 6, 6 | 1,250 / 1,250 | 1 | 6 | 0 |
| 8 | 1 | 7 | 1,250 / 1,250 | 1 | 9 | 6 |

Every included attestation met the three-slot window. Node1's pool retained
slot-zero entries through slot 3 and had none at slot 4; its slot-4 capture
contained only current-slot entries. All 50 final pool captures contained only
slot-8 entries. Expiry counters recorded 47 deletions summed across nodes
(per-node events, not unique network votes). This directly verifies deletion
from the pool as well as the packing cutoff.

The slot-8 end-to-end check matched its nine execution transactions and six
blobs to the payload envelope. Its one FFG aggregate covered all 1,250 slot-7
participants and matched 42 aggregate-ledger observations. Its payload
attestation covered all 512 seats, with both payload-present and data-available
flags true, matching 20,825 PTC ledger observations from 425 distinct validators.
Ledger observations repeat across nodes; committee seats can repeat validators.
The common slot-8 beacon root was
`0xafa1fd056a762f72a755e9fd4e9f9dc508886f1b71d231b54ebe9994e30c9eec`.

The separate startup/slot-zero issue remains: block 1 included only 27 slot-zero
participants. Six beacon nodes logged an EL follow-distance error after genesis;
all were online and synced at the final capture. Eight startup eth1data fallback
warnings appeared, with no post-genesis validator errors. Metrics recorded
2,202 undeliverable payload-attestation and 333 sync-committee delivery events;
no positive FFG or available-attestation undeliverable counter was present.
These gossip-queue issues remain outside this fix, and the run does not establish
a change in their rate. Complete Goldfish summaries for slots 1–7 had all 512
seats on every node. Worst observed payload arrival was 4.629 seconds on home
nodes and 2.401 seconds on supernodes, within the slot. Finality remained at
round zero; eight startup slots do not establish longer-term finalization.
Shadow results are network checks, not CPU benchmark measurements.

The run stopped at virtual second 408, after blocks 1–8; genesis was second 300.
The invocation took 902.99 seconds of wall time including generation. All of
this run's Shadow/beacon/validator processes have exited. No local simulation
was started and no production source changes were needed during verification.

Remote run and reusable build/capture scripts:
`/home/sukun/dev/prysm-retention-20261005/shadow/runs/retention-n50-v10000-home40-super10-s1-8slots/`
and `shadow/retention-evidence/` under that source root. Invoke `run.py 8` only
with a fresh output name; the wrapper refuses to overwrite an existing run.

Local evidence:
`/home/sukun/.cache/prysm-attestation-retention/2026-10-05/shadow-remote/result/evidence/`.
It contains raw REST/EL/pool captures, `analysis.json`,
`retention-verification.json`, verifier logs, configuration and build provenance.
The adjacent `evidence.tar.gz` also retains all 50 nodes' full logs. Its verified
SHA256 is `c35b70c7138ea2b7b92577a17d3a2f2db69c49279fe0a4f01ccd7de22f9b1a8b`.
