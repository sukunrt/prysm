# Conditional coverage bound for proposal 97

This is a bound on the **logged eligible singles at owner node 1**, conditional
on its logged aggregates reaching the proposal's pool snapshot. It is not a
reconstruction of that snapshot and does not bound unlogged pool objects.

## Pool ingress not represented by the FFG ledger

The deployed pending-attestation queue is one concrete gap in a ledger-only
pool reconstruction. An incoming single whose referenced block/state is not
yet available is saved to the pending queue and returns `ValidationIgnore`
before reaching `logFFGVote`
(`sync/validate_beacon_attestation.go:126–130,243`). A missing-block aggregate
similarly returns before `logFFGAggregate`
(`sync/validate_aggregate_proof.go:131–145,258–261`). These are not logged as
successful FFG gossip votes at initial reception.

After block import, the pending drain groups singles by data, converts each
single to Electra, batch-verifies signatures, and calls
`processVerifiedAttestation`. That function inserts the attestation into the
legacy pool, updates the seen cache, broadcasts, and emits an operation event,
without an FFG ledger call
(`sync/pending_attestations_queue.go:234–313,374–426`). Pending aggregate replay
also validates and inserts the original aggregate without `logFFGAggregate`
(`pending_attestations_queue.go:428–452`). Its original Gloas version therefore
has the same later Electra-prune mismatch as ordinary aggregate ingress.
Pending singles use the normal Electra admission/seen-bit rules; the queue
does not force otherwise-ineligible singles to survive.

Both replay paths are enabled for the deployed Heze service: the block
subscriber and pending-block processor call the drain, and
`sync/service.go:350` starts a `BlockProcessed` event listener that also covers
locally imported proposals (`pending_attestations_queue.go:82–149`). The only
combined successful replay summary is Debug-level
`Verified and saved pending attestations to pool`
(`pending_attestations_queue.go:213–225`); no such records are retained in node
1's beacon log. Therefore the successful FFG CSV rows cannot serve as a complete
census of historical pool insertions. This establishes an observability gap,
not that a pending backlog existed during proposal 97. Its preceding blocks
were already imported before the corresponding slot-start votes, supporting
the ordinary direct-ingress case for those known roots. The conditional bound
below remains a bound on the specifically logged singles, not every possible
pool object.

Reorg salvage does **not** provide a second route for multi-committee block
attestations into this proposer's aggregate candidates. At post-Electra forks,
`blockchain/head.go:515–518` calls `SaveBlockAttestation`, which stores objects
in the separate `blockAtt` map (`operations/attestations/kv/block.go:12–38`).
The older direct `SaveAggregatedAttestation`/`SaveUnaggregatedAttestation`
branches are unreachable for Heze blocks. The proposer reads only aggregated
and unaggregated maps (`proposer_attestations.go:41–47`). Background forkchoice
preparation consumes `blockAtt` into the separate forkchoice pool and deletes
those block objects (`operations/attestations/prepare_forkchoice.go:72,98–107,
116–130`); it does not feed them into proposer `onChainAggregates`. Consequently,
the fixture hazard from manually feeding already-packed multi-committee
aggregates into the proposer is not a demonstrated reorg-salvage bug.

Node 1's last pre-proposal-97 reorg marker is beacon log line 934118 at
`01:46:36.004536822Z` (head 82 to 80). Its next marker, line 1137427 at
`01:49:36.003600844Z`, retreats from 97 to 96 after proposal 97 completed.
There is no contemporaneous reorg marker establishing salvage work during
the recorded 3.645-second construction interval.

The [slot-96 census](node1-slot96-ffg-timing.json) contains 13,947 distinct
single-vote rows across six committees and ten data-root groups. All were
emitted by slot+4.157 seconds. Twelve aggregate rows were emitted from
slot+8.035 through +8.114 seconds: eleven gossip-validation rows and one local
duty row for the 1,812-seat committee-1 aggregate. All six large aggregate
maxima used below are gossip rows, so excluding the local row leaves the
bound unchanged. Proposal 97 started at the next slot's
+0.008736 seconds and finished at +3.662623 seconds; its payload selection to
finish interval was 3.645373 seconds.

## Exact membership across slots 88-96

The bounded [pre-build-97 aggregate census](node1-prebuild97-ffg-aggregate-census.md)
parses the complete validator CSV from every node-1 aggregate ledger emission
in slots 64-96 before the slot-97 build start, and joins singles by
`(attSlot, dataRoot, committeeIndex)`. Its selected-row census has no parse
failures or aggregate seat mismatches. Exact sorted validator IDs for the 91
distinct eligible aggregate sets are retained in
`node1-prebuild97-ffg-aggregate-validator-sets.json.gz`; the compact
[JSON census](node1-prebuild97-ffg-aggregate-census.json) retains their hashes,
block-root families, row anchors, outcomes, and set relations.

Slots 88-96 contain 137 aggregate ledger emissions in 64 groups: 124 have a
gossip outcome and 13 have a local outcome. Each group has one observed set
that covers every other set in that group. This does not mean every pair of
earlier partials is nested; two incomparable partial sets can both be covered
by a later set. All 64 covering/largest sets have a gossip emission.

Exact membership gives the following conditional bound:

| Eligible scope | Logged singles in groups with an aggregate | Present in largest logged gossip set | Singles in groups with no logged aggregate |
| --- | ---: | ---: | ---: |
| Slot 96 | 13,925 | 13,925 | 22 in 2 groups |
| Slots 88-96 | 103,126 | 103,126 | 367 in 33 groups |

Thus exactly 22 of the 13,947 logged slot-96 singles, and 367 of the 103,493
logged slots-88-96 singles, are absent from the largest logged aggregate sets.
They all belong to data groups with no logged aggregate. The membership result
is identical when selecting all ledger rows or gossip rows because every
all-ledger covering set also has a gossip emission. The inputs are not
identical: the all-ledger census has 137 emissions while the gossip-only view
has 124.

These are relationships among ledger records. A gossip outcome is logged
before the subscriber completes, so it does not prove that the covering
aggregate reached the pool or remained present when proposal 97 took its
snapshot. Conversely, the pending replay path described above can insert
objects without emitting a successful FFG ledger row. The exact join therefore
narrows the logged shape but does not reconstruct the historical pool.

## Key matching and superseded cardinality arithmetic

At deployed revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`,
`decoupled/vote_ledger.go:74–79` renders the pool's `attestation.Data` ID with
only its version byte removed. Therefore a matching ledger dataRoot and
committee identify the same grouping used by proposer deduplication after its
Gloas-to-Electra normalization (`proposer_attestations.go:54–83`). This does
not require recovering the unlogged checkpoint roots separately.

Before full validator CSVs were joined, cardinality alone gave this
conservative estimate. For N distinct singles and an M-seat aggregate in the
same 2,500-seat committee and data group, at most `min(N, 2500-M)` singles can
be outside the aggregate.
This follows from the size of the committee's complement; it does not assume
the observed singles are exactly the aggregate's voters.

| Committee | Main dataRoot prefix | Logged singles N | Largest logged aggregate M | Maximum uncovered singles |
| --- | --- | ---: | ---: | ---: |
| 0 | `045eb045` | 2,491 | 2,491 | 9 |
| 1 | `27a818cd` | 1,812 | 2,486 | 14 |
| 2 | `71b4ddc4` | 2,403 | 2,491 | 9 |
| 3 | `1a766ff4` | 2,483 | 2,483 | 17 |
| 4 | `9053d7af` | 2,474 | 2,485 | 15 |
| 5 | `c133ac36` | 2,236 | 2,489 | 11 |
| Total | Six main groups | 13,899 | | 75 |

Four other data-root groups contain 9, 13, 15, and 11 singles: 48 total.
Treating all of them as uncovered gives **at most 75+48 = 123 uncovered
singles among these 13,947 logged singles**, if the six large aggregates are
present and valid in the proposal snapshot. The two small logged aggregates
can only improve this conservative bound. The exact set-membership join above
supersedes this estimate: it finds 22, rather than at most 123, absent from the
largest logged aggregate sets.

The getter and validation still process the raw snapshot before deduplication.
The bound concerns residual uncovered one-bit votes, not the number of raw
entries read, and does not eliminate costs from other retained aggregates.

## Why this changes interpretation of the raw benchmark

The deployed proposer deduplication groups by version-normalized Data ID and
compares bit containment within each group
(`proposer_attestations.go:373–414`). Distinct disjoint singles survive all
pairwise comparisons. Its subsequent greedy MaxCover repeatedly scores the
remaining candidates (`aggregation/maxcover.go:144–218`). For disjoint singles,
both stages scale quadratically in each group's candidate count, with bitset
length multiplying the work. Their work is governed by the sum of squared
group sizes, not just the total number of arrivals.

The controlled 13,000-single fixture has four groups of 2,167 and two of 2,166:
14,076,834 unordered pairs in the first dedup pass, with two failed containment
checks per pair. Large covering aggregates remove most redundant singles
before MaxCover. Partial aggregates can leave uncovered singles or overlapping
aggregate objects, so their exact number and coverage matter; an aggregate's
participant count is not its candidate-object count.

The [focused CPU profile](old028-heavy13k-fullpack.pprof-focus.txt) attributes
6.57 seconds of cumulative samples to proposer deduplication and 6.32 seconds
to attestation aggregation within 13.49 seconds of sampled packing calls.
The [profiled benchmark](old028-heavy13k-fullpack-profile.log) averages 3.413
seconds for three timed raw-only packs. The profile also includes an untimed
preflight pack; its unfiltered top table includes fixture key generation and
signing, and must not be interpreted as a single proposal's CPU breakdown.

These results demonstrate a seconds-long eligible raw-pool mechanism. They do
not show that proposal 97 saw that raw-only shape. If its logged covering
aggregates were available, exact membership leaves 22 slot-96 singles and 367
slots-88-96 singles in groups with no logged aggregate. Conversely, aggregate
ledger acceptance alone does not prove their availability in the snapshot,
and the ledger omits pending-replay insertion.

## Accepted aggregates can still miss the pool

The aggregate validator logs success before setting `ValidatorData` and
returning (`sync/validate_aggregate_proof.go:145–151`). Pubsub delivery and the
subscriber run afterward. `sync/subscriber.go:437–458` enlarges the single-vote
buffers but leaves aggregate topics at the pubsub default buffer of 32. The
v0.17.0 pubsub delivery path uses a nonblocking send and can drop a validated
message when that subscription buffer is full (`pubsub.go:1428–1442`). Prysm
records this in `p2p_pubsub_undeliverable_total`, not an ordinary error line
(`p2p/pubsub_tracer.go:130–132`, `p2p/monitoring.go:127`). This is a reachable
failure mode, not evidence that any of the eleven slot-96 gossip messages was
dropped. A local duty ledger row is not a completed inbound validation and
does not follow this post-acceptance delivery argument.

For delivered messages, `subscribeWithBase` launches a separate handler
goroutine (`sync/subscriber.go:523–545`); it is not a serial handler queue.
The aggregate subscriber passes Gloas through unchanged to the pool
(`sync/subscriber_beacon_aggregate_proof.go:21–34`). Pool save can wait for the
global aggregate-pool lock, held while merging a key's candidate list
(`kv/aggregated.go:139–160`), or for its coverage/seen-cache checks. Scheduler
delays can also postpone the handler. No completed insertion marker is logged.
The owner log contains no `Could not handle p2p pubsub`, `Received nil message
on pubsub`, `Panic occurred`, or `Subscription next failed` line in the
01:49:20–27 window. This excludes those explicit error markers, not silent
delivery drops or delayed goroutines.

The old Electra/Gloas mismatch itself retains Gloas aggregates; it does not
explain their absence from a later snapshot. Establishing absent coverage
requires additional evidence such as per-topic delivery-drop metrics, a pool
snapshot, successful-insertion instrumentation, or a trace identifying the
waiting subscriber. None is supplied by the acceptance ledger alone.

Current-slot-97 singles are a separate case. Their inclusion-delay check fails
before active-count lookup, committee resolution, deduplication or MaxCover
(`core/blocks/attestation.go:63–120`). They can add snapshot/filter/delete work
and concurrent gossip-validation load, but cannot directly recreate the
profiled quadratic path of 13,000 eligible previous-slot singles.

## Ordinary FFG validation does not copy a state per vote

The deployed `AttestationTargetState` acquires a forkchoice read lock and calls
`getAttPreState` (`blockchain/receive_attestation.go:41–52`). For a compatible
head at slot 96 and target round 12, the recent-state branch returns
`HeadStateReadOnly` (`blockchain/process_attestation_helpers.go:23–63`). That
method returns `s.head.state` itself, without `Copy`
(`blockchain/head.go:306–313`; `chain_info.go:260–273`). Many concurrent
validations can reference that one state; they do not create one native state
per vote.

If the recent-head path cannot serve a checkpoint, a checkpoint-cache hit
also returns the existing object without copying
(`cache/checkpoint_state.go:55–70`). A missing or ahead-of-head checkpoint may
copy a head/next-slot/stategen state and advance it, but this work runs under a
root-and-round multilock and caches the result before releasing it
(`blockchain/process_attestation_helpers.go:75–97,108–175`). The next caller for
that cached checkpoint reuses the object. This does not rule out repeated
cache misses across many distinct checkpoints or eviction, but no such churn
has been established for the observed build interval.

The successful validation attaches an attestation to `msg.ValidatorData` and
sends an attestation-only operation event; its subscriber stores that
attestation, not the validation state
(`sync/validate_beacon_attestation.go:181–241`;
`sync/subscriber_beacon_attestation.go:21–36`). Consequently, this path does not
support an explanation based on thousands of per-vote native-state copies,
MVS registrations, or later state finalizers. The normal nonzero-state cached
active-count return also avoids the startup's repeated registry scan
(`core/helpers/validators.go:145–159`).

There is still ordinary per-vote work. For a 2,500-member committee, membership
validation and Single-to-Electra conversion each search up to 2,500 indices
(`sync/validate_beacon_attestation.go:393–405`;
`proto/prysm/v1alpha1/attestation.go:533–550`). Signature preparation calls
`AttestingIndices`, which checks all 2,500 aggregation-bit positions and reserves
a temporary capacity of 2,500 uint64 indices even for a single participant
(`attestation/attestation_utils.go:85–129`). For a controlled 13,000 singles,
that last operation alone entails 32.5 million bit checks and 260 million
bytes of requested temporary index capacity across the workload. It then
reads the one participant's public key through MVS, rather than scanning the
120,000-validator registry (`state-native/getters_validator.go:181–196`).
These are source-level operation counts, not measurements of historical CPU
or elapsed time, and do not establish that this concurrent workload caused
the recorded multi-second build delay.
