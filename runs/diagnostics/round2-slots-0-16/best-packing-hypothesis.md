# Best-supported explanation of slots 10 and 14, and uneven recovery

The leading explanation is a proposal-specific backlog: the consensus branch
encountered expensive attestation-pool work while the node was still processing
startup gossip. The genesis-count defect supplies a demonstrated source of
competing work and lock queues; uncompacted attestations supply a demonstrated
algorithmic multiplier. The proposer then waits for work that does not promptly
observe cancellation. This combination explains the shape of the failures
better than an execution-engine delay, a whole-process pause, or one continuous
head-lock hold.

This is a ranked inference about the historical owners. The terminal dependency
chain and the code mechanisms below are established; the historical pool
snapshots and per-phase timings were not recorded. That distinction limits how
specifically to name the expensive phase, rather than preventing a best causal
explanation.

## What the two owners require an explanation to cover

| Observation | Slot 10 / node 35 | Slot 14 / node 85 |
| --- | ---: | ---: |
| BN build entry after slot start | 7.446879 s | 2.694093 s |
| Premine eth1 warning | 10.337597 s | 2.740644 s |
| Successful self-build payload selection | 10.338144 s | 2.744065 s |
| Budget remaining after selection | 1.661856 s | 9.255935 s |
| Bare packing cancellation | 36.873390 s | 43.993917 s |
| Selection-to-packing-return interval | 26.535246 s | 41.249852 s |
| Packing marker to terminal state-root/build error | 1.230 ms | 2.342 ms |

The source starts consensus-field work in parallel with payload retrieval and,
after payload success, must join that work before calculating the state root.
The logged packing cancellation occurs inside that required branch. After it
returns, state-root processing immediately encounters cancellation. The state
root is therefore the place that reports the failed budget, not an evidenced
26- or 41-second hash computation. The payload-selection marker is before the
bid-recording lock and the join, so it does not timestamp the exact join entry.

The eth1 warning selects the premine branch, but precedes `HeadETH1Data`. The
measured interval can include that head/state access before the packer starts.
The packer also has two errgroup children. Their direct errors are wrapped;
their successful bodies can instead return the bare context error at a final
check. The historical bare error identifies this outer cancellation category,
not which child was last or which error won its race. These details are
documented in [the build-boundary audit](deeper-build.md).

## Ranked causal paths

| Rank | Explanation | Why it fits | What constrains it |
| --- | --- | --- | --- |
| 1 | Attestation work from an uncompacted snapshot, amplified by startup work and delayed cancellation | Real signed raw input takes 5.112 s versus 1.069 ms after compaction; real count interference has increased packing to 59–61 s. The proposer keeps its private raw snapshot even if live pools and other node activity subsequently improve. | Neither owner's raw count or snapshot time is logged. The sustained interference diagnostic called counts directly rather than through the newer checkpoint path. The precise historical division of CPU time and waits is an inference. |
| 2 | A shorter pool/head/fork-choice lock wait combined with packing and scheduling delay | These context-insensitive acquisitions are on the actual consensus path. Actual checkpoint work reproduces multi-second fork-choice and global-manager queues elsewhere. | There is no packing-owner lock trace. A fork-choice queue in parent preparation is not automatically a head-lock queue in this later branch. |
| 3 | A predominantly pre-packer head/state or deposit dependency delay | The warning precedes a head read, and legacy deposit processing is not categorically disabled in these states. Both are reachable code paths. | There is no positive historical duration marker for either. The large, independently measured attestation cost gives rank 1 more explanatory support. |
| Rejected as the complete explanation | Slow execution payload, one continuous head writer, or a whole-node stop | — | Both payloads were selected. Node 85 submits attestations while its proposal is outstanding and imports blocks 15, 16, and 17 before it returns; the import reports themselves cross a head read. |

Confidence is highest in the unfinished-work and cancellation chain, and in
the existence of the two costly code mechanisms. Confidence is moderate that
their composition is the principal historical packing cause. It is lower for
any claim assigning all 41.25 seconds to `dedup`, one particular lock, or an
exact number of pool objects.

For slot 10, a late start leaves only 1.662 seconds even after payload success.
The measured 5.112-second raw fixture exceeds that remaining budget without
competing work. For slot 14, the same raw fixture alone is insufficient: its
9.256-second remainder and 41.250-second late branch require additional work,
contention, or a different historical pool shape. The sustained count/packing
control establishes that the proposed composition can reach that scale; it is
not a measurement of node 85's queue or a calibrated prediction of 41 seconds.

## Why later compaction cannot rescue an old proposal

The source supplies a concrete explanation for old and new work diverging:

1. `packAttestations` takes an aggregated-pool snapshot, validates it, then
   calls `UnaggregatedAttestations` and validates those candidates
   (`proposer_attestations.go:39–47`). The raw getter holds its map's read lock
   while checking seen bits and **cloning** each returned attestation
   (`operations/attestations/kv/unaggregated.go:59–79`).
2. Deduplication and aggregation subsequently operate on the returned slice.
   They do not ask the pool whether newer aggregates now cover those singles.
   Later compaction or deletion changes the pool, not that private snapshot.
3. The first deduplication groups by `attestation.NewId(att, Data)` and tests
   pairwise bit containment (`proposer_attestations.go:381–413`). This key
   distinguishes the attestation data and the committee. For six groups of
   2,500 disjoint singles, neither direction contains the other, giving exactly
   `6 × 2500 × 2499 = 37,485,000` containment calls. Different vote slots form
   different data groups. That arithmetic describes the fixture, not an owner
   pool census.
4. The general aggregation algorithm runs on those groups before the block
   attestation limit. Deduplication, that aggregator, and on-chain grouping have
   no context parameter. Cancellation in an earlier cleanup helper does not
   make the whole packer stop; node 83's historical abandoned slot-6 branch
   demonstrates another 10.286 seconds after its cleanup cancellation marker.
5. Background compaction takes its own raw snapshot and uses the specialized
   disjoint-single combiner. Its parallel workers save each produced aggregate,
   but the method starts its raw-deletion pass only after **all** aggregation
   groups complete (`kv/aggregated.go:23–62,75–123`). A partially completed
   compaction therefore is not the same state as the completed compact-pool
   control. The aggregation and deletion routines do not continuously consult
   the proposer context either.

An aggregate already included in the proposer's first snapshot may cheaply
eliminate covered singles during deduplication; the raw worst case requires
that coverage to be absent from that snapshot. The independent snapshot order
allows that case. It does not prove that every concurrent compaction makes the
proposer slow.

The normal service performs this compaction periodically, with default slot
offsets 7, 9.5, and 11.8 seconds (`config/features/flags.go:58–75`), rather than
as an atomic prerequisite to every proposal. Those are source defaults, not
recovered owner command-line settings. Delayed gossip acceptance and delayed
compaction can change which raw votes are visible at the proposal snapshot.
Accepted votes first traverse the expensive validation path, then their
subscriber inserts them into the pool. A large raw pool must therefore be
explained by the relationship between arrivals and compaction; it cannot be
equated to all validators or all accepted observer messages.

Node 85's successful imports are compatible with this retained-snapshot
mechanism: fresh operations can use current head and pool views while an older
proposal continues its copied work. Import processing consumes a block's
already-selected attestations; it does not repeat the proposer's search over
the entire raw pool. The block-15/16/17 import logs report eight included
attestations apiece, which does not bound either historical proposer's inputs.

## Why the first successes can occur at 15 and 16

The best-supported system explanation is uneven draining of startup work,
combined with scheduled proposer ownership and the contents seen by each fresh
proposal. Node 32 was already recording fast successful attestation submission
progress before its slot-15 duty. Its build enters at +9.448 ms, selects its
payload at +18.811 ms, and completes at +1.482 s. This proposal has almost its
whole slot budget and successfully completes every required branch. Node 85's
older slot-14 branch remains outstanding for another 30.338 seconds after
node 32's successful submission. The observations select per-node/per-request
progress over any universal recovery boundary.

Two source facts explain why conditions can improve without one magic cutoff:

- New round-1 votes from slot 8 can resolve a nonzero-slot checkpoint state
  and use the cached count. Older round-0 gossip remains admissible and can
  still require scans. A finite old-work backlog can drain gradually while
  newer work becomes cheaper. The actual amount drained by any proposer duty
  remains an inference; observer 201 still accepts a round-0 vote in wall slot
  16.
- Block inclusion validation uses the **proposal state's** current and previous
  rounds (`core/blocks/attestation.go:63–74`). A fresh state for block 16 has
  rounds 2/1, so round-0 pool candidates are rejected before expensive
  committee validation and deduplication. An old block-14 proposal still owns
  a state at round 1, where round 0 is the valid previous round; wall-clock
  progression does not rewrite that state's initial candidate-validity test.
  The final signature filter separately reads current head and wall time, so
  the old request is not completely insulated from later changes.

The second fact can help slot 16 avoid old round-0 packing work, but it cannot
explain first success at slot 15, which still admits round-0 candidates. It
also does not stop old gossip validation at slot 16. Slots 15 and 16 both name
genesis as parent; success is not conditional on first extending an imported
slot-15 parent. Their later differing fork-choice retention is a separate,
deterministically reproduced vote-distribution effect, described in
[the retention audit](retention-deep.md).

## One useful bounded follow-up

**Executed:** the proposed same-pool overlap control now passes. Compaction
finishes at +2.268 s, a fresh verified pack finishes in 1.164 ms while the old
pack remains active, and the old pack returns cancellation at +5.326 s,
3.664 s after its timer fires. The
[complete result](packing-snapshot-overlap-realwork-results.md) records the
single pilot and its limits. The original design below explains the prediction
made before the experiment.

The remaining informative local control is concurrent **real compaction of the
same pool after a proposer has copied its raw snapshot**. Reuse the corrected,
signed 15,000-vote fixture. A test-only pool wrapper can close an event channel
immediately after its real raw getter returns its copied slice, without
blocking that getter. A background goroutine starts the actual compaction from
that event. Use the slot-10 residual budget of 1.662 seconds for the real
`packDepositsAndAttestations` call and timestamp compaction completion, live
raw/aggregate counts, and final pack return. Keep an uncanceled coverage/BLS
check and an already-compacted comparison.

The prediction is that compaction can complete and make a fresh proposal cheap
while the old copied pack remains outstanding and eventually observes
cancellation. A contrary ordering or result should be retained; there should
be no load escalation to force a desired duration. This test would distinguish
the concrete stale-snapshot mechanism from the assumption that successful
background compaction necessarily rescues the already-started proposal. It
would not reconstruct node 85's unlogged pool or require a network rerun.

This note is a source/log analysis and ranking. No test or production change
was made to prepare it. Existing passing controls are in
[packing compaction](packing-compaction-realwork-results.md),
[packing input fidelity](packing-input-fidelity-audit.md), and
[actual parent dependency work](slot13-parent-dependency-realwork-results.md).
