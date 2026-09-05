# Round 2: older-build construction work and the pruning defect

Round 2 used Prysm `0280403c70d88967f49d2d4c730f4c5417dabdf5`.
That revision contains a concrete attestation-pool pruning defect fixed later
by `cabb3f8f49c9`. It can keep already-included gossip aggregates in the
proposer's candidate pool and cause repeated consensus work. This is a
relevant difference from the current build, but the historical logs do not
assign the full construction delay to this one defect. The completed old-code
benchmarks, including the observed single-vote arrival schedule and deployed
SSE subscription topology, have not reproduced the historical delay. Its
precise cause remains unresolved in the retained record.

The subsequent [benchmarks of the deployed revision](../round2-construction-bench/results.md)
put an important limit on that attribution. At ordinary slot 97, the compact
retention fixture takes **15.95 ms** to pack, versus **1.53 ms** after normalized
inclusion pruning. Adding 13,000 singles with covering aggregates takes
**199.59 ms** in the old singles-before-aggregates case. These results do not
confirm that compact retention or already-credited reward scoring accounts for
the historical multi-second interval. A different, raw-only 13,000-single
fixture does take **3.43 seconds**; its CPU profile identifies pairwise
containment deduplication and maximum-cover aggregation as the dominant work.
That is a conditional reproduction, not a recovered historical pool snapshot.

## Where the measured delay occurs

For 13 later produced blocks that were subsequently bypassed, the retained
[owner timeline](../startup3/round2-slots-0-100.md#stage-breakdown-for-the-bypassed-slow-blocks)
records build entry within 9–22 ms of slot start, payload selection another
2–31 ms later, and **2.542–7.763 seconds from payload selection to construction
completion**. The latter is a joined construction interval, not a measurement
of attestation-packing CPU alone.

At the deployed revision, `buildBlockGloas` starts consensus-field assembly in
a goroutine (`beacon-chain/rpc/prysm/v1alpha1/validator/proposer_gloas.go:25–34`),
selects the payload source (`:77–83`), waits for consensus assembly (`:85`),
and computes the post-block state and root (`:87`). State transitions, state
hashing, scheduling and lock waits can therefore also contribute to the
measured interval.

## Exact defect in the deployed source

These line references were checked directly with
`jj --ignore-working-copy file show -r 0280403c`:

1. Post-Gloas aggregate gossip carries `AttestationGloas`. The aggregate
   subscriber takes `AggregateVal()` and saves the resulting object without
   conversion (`beacon-chain/sync/subscriber_beacon_aggregate_proof.go:21–34`).
2. `attestation.NewId` places `att.Version()` in the first byte of each pool
   key (`proto/prysm/v1alpha1/attestation/id.go:35–36`). The aggregated pool,
   seen-bits cache and block-attestation cache use these versioned keys.
3. Inclusion pruning decomposes each block attestation into per-committee
   **Electra** dummy attestations and calls `DeleteAggregatedAttestation`
   (`beacon-chain/blockchain/process_block.go:766–790`).
4. The Electra key does not match the stored Gloas key. Deletion inserts seen
   bits under the Electra key, then silently returns nil when the aggregated
   lookup misses (`beacon-chain/operations/attestations/kv/aggregated.go:255–270`).
   The Gloas entry remains; no pruning-error log is expected.
5. A subsequent proposer reads those remaining entries. The packer does not
   exclude an otherwise valid attestation just because its participation was
   already credited in an earlier block.

The [original defect analysis](../../../plan/plan-gloas-pool-prune.md) describes
the same version mismatch. Commit `cabb3f8f49c9` normalizes aggregate ingress
and its redundancy checks to Electra, and normalizes block-attestation cache
insertion. It changes the objects reaching the unchanged packer and pool.

## Why the extra entries cost work

The normal proposer snapshots and validates aggregated entries first, then
snapshots and validates singles. Only afterward does it normalize versions,
deduplicate, aggregate and limit the block contents
(`proposer_attestations.go:42–75`, `:89–128`). Thus extra entries have already
incurred validation work even when deduplication later removes them.

Reward sorting also resolves committees and participant indices for candidate
on-chain aggregates (`beacon-chain/core/electra/attestation.go:47–55`). It loops
over those indices and calculates base rewards before testing whether the
participation flags were already credited (`:69–93`). Already-included votes
can therefore consume work while contributing no new reward. Re-including
them also makes later block-processing paths process their attestation data
again.

The old key mismatch additionally prevents a Gloas aggregate from satisfying
an Electra single's pool-coverage lookup. This can retain redundant singles
until normal compaction or pruning processes them. It does **not** mean that
raw-single compaction was broken: the historical single-vote validation path
already converted `SingleAttestation` to Electra before the subscriber
(`beacon-chain/sync/validate_beacon_attestation.go:172–181,240–241`).

## Historical evidence and verification status

The [bounded old-build pool audit](round2-old-build-pool-pruning.md) verifies
identical full validator sets for these attestations in the consecutive
retained blocks **94 → 95 → 96** in node 400's `beacon.log`:

| Attestation slot | Data root prefix | Seats in each block | Validator-list SHA-256 prefix | Raw lines for blocks 94 / 95 / 96 |
| ---: | --- | ---: | --- | --- |
| 91 | `1fe5887a6af98797` | 14,920 | `ff7de25eb7d8eaba` | 507053 / 512902 / 519927 |
| 93 | `4868980d9c37478e` | 14,927 | `677aaf70de49aa72` | 507049 / 512960 / 519924 |

The audit parses 935 inclusion records through slot 100 with no remaining
validator-list parse gaps. Of these, 536 are on the reconstructed retained
lineage. It finds 211 exact repeat observations across 56 retained parent-to-child
links; these are repeated inclusion comparisons, not 211 distinct vote groups.
The parser accounts for outer timestamps inserted into long validator lists.

These records establish repeated inclusion on a continuing branch, consistent
with the deployed pruning defect. They do not recover the proposing owners'
pool snapshots or establish which particular gossip arrivals supplied their
entries. A direct equality join between gossip and inclusion `dataRoot` fields
would also be insufficient: the grouping hash includes committee indices, and
an on-chain aggregate can combine several single-committee gossip aggregates.

The existing focused pool regression passed through Bazelisk in 34.210 seconds.
It verifies that deleting with a covering Electra dummy leaves one raw Gloas
entry, while normalizing that same input before storage leaves zero entries
after deletion. This directly tests the version-key mismatch without rerunning
the network. The [build comparison and regression-test record](prysm-build-construction-diff.md)
retains the command and result.

The follow-up measurements now execute `0280403c` itself in the separate jj
workspace `/home/sukun/dev/prysm2-round2-construction-028`, with diagnostic
test additions only. They use Go tests, native Heze states with 120,000
distinct validators, valid signatures, real pool admission/deletion, and
both single/aggregate arrival orders. The [results and commands](../round2-construction-bench/results.md)
supersede reliance on source equivalence with the current checkout.

The same suite measures an eight-attestation density control at **32.97 ms**
for the state-transition/root wrapper; fresh post-state hashing alone is
about **0.53 ms**. The subsequent full `BuildBlockParallel` benchmark takes
**37.53 ms** with three attestations and **91.77 ms** with eight. It includes
retrieving a cached slot-96 parent, advancing it to slot 97, applying the full
parent's execution payload, reading its stored envelope, packing 493 sync
participants, computing the state root, and caching the new envelope. Its
immediate external mocks and warm state store exclude historical concurrent
workload and state-retrieval contention. These figures establish the cost of
the supplied work, not an allocation of the owner's elapsed time.

The follow-up comparison adds all 14,355 logged slot-97 gossip votes at their
recorded validation-entry pace. Across three `GOMAXPROCS=4` pairs, full builds
take **94.70–99.15 ms** without the stream and **89.28–98.41 ms** with it.
With `GOMAXPROCS=1`, the corresponding values are **108.59/146.64 ms**.
All scheduled votes are accepted and subscribed after draining each replay.
That comparison includes production decoding, committee checks, signature
batching, logging and pool insertion, with no event-stream subscribers.

The subsequent [Xatu/SSE comparison](../round2-construction-bench/xatu-sse-contention-audit.md)
adds the deployed configuration's 18 separate HTTP event streams: 11 operation
feed subscribers and seven state feed subscribers. It uses the real old SSE
handler and immediately draining clients. With the same 14,355 paced votes,
the fresh `GOMAXPROCS=4` pair takes **89.636 ms without SSE / 86.309 ms with
SSE**. At `GOMAXPROCS=1`, it takes **152.248 / 160.982 ms**. Every scheduled
vote is accepted and subscribed, all 14,355 complete single-attestation frames
arrive after draining, and no slow-reader warning or unexpected disconnect
occurs. This exercises the previously omitted delivery path without
reproducing seconds of construction time. It does not simulate the entire
Xatu process, historical host scheduling, or unlogged incoming traffic.
`GOMAXPROCS=1` limits Go runtime parallelism; it is not a one-core CPU quota.

The [logging deployment audit](../round2-construction-bench/logging-deployment-audit.md)
also checks the physical output paths omitted by the immediate benchmark sink.
The retained round-2 files do not establish the ephemeral debug-file setting
or a log-write stall. Logging therefore remains an unmeasured dependency, not
an identified cause of the delay.

Node 1's slot-96 vote ledger further constrains the raw-only explanation for
its slow slot-97 proposal: 13,947 unique single-vote ledger rows (13,871 gossip
and 76 local submissions) are logged by slot +4.157 s,
and aggregate records appear at +8.035–8.114 s, before the proposal at the
next slot's +8.7 ms. The [owner census](../round2-construction-bench/node1-slot96-ffg-timing.json)
records these times. The records do not timestamp subscriber insertion or
the proposer snapshot, but they prevent equating the single-arrival total
with 13,947 uncovered candidates in that snapshot.
The later exact validator-set join finds **13,925 of those rows covered by
the largest logged gossip aggregates**, leaving **22 in groups without a
logged aggregate**. Across eligible slots 88–96, the corresponding counts are
103,126 covered and 367 without a logged aggregate. These are conditional
coverage results, not proof that those aggregates reached the pool snapshot;
the [coverage audit](../round2-construction-bench/historical97-coverage-bounds.md)
documents the unlogged delivery and pending-replay boundaries.

## Scope and limits

- Both round-1 `a1679c9f` and round-2 `0280403c` precede the ingress fix.
  This defect distinguishes the old runs from current code; it does not alone
  explain a round-2-versus-round-1 difference.
- The later commit called “scratch space” adds intentional network-message
  padding. It is not a Merkle-hashing scratch-buffer optimization. The scoped
  source comparison provides no missing state-root optimization explanation.
- The defect concerns **inclusion pruning**, not permanent retention.
  Expiration cleanup deletes using the stored object's original version
  (`operations/attestations/prune_expired.go:51–58`), and invalid-candidate
  filtering can remove the original entry before normalization.
- Startup slots 1–14 had failed proposals and no imported blocks. Their
  established repeated genesis-registry count pressure is a separate
  [startup mechanism](../round2-slots-0-16/best-causal-explanation.md); a defect
  in pruning after inclusion cannot explain those misses by itself.
- Historical owner profiles and internal stage timings are absent. Repeated
  inclusion and an applicable source defect identify avoidable work, but do
  not prove that this defect consumed every second of a slow proposal or that
  its fix alone would make every proposal timely.
