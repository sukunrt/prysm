# Plan-simplify: what a networking-only stub does not need

Written 2026-08-22. A discussion memo, not a plan: the user's directive is
that the research target is **networking logic only**, and the design we
built — and the plans that describe it (`plan-complete.md`, 3.4k words;
`plan-complete-detailed.md`, 13.7k) — is more complete than that goal
requires. Each candidate below states what it replaces, what it deletes,
which problem classes disappear, what stops being real (and whether the
WIRE stays real — the charter's one hard constraint), and a verdict.

The seed (user): FFG attestation committees do not matter. A deterministic
mock committee where everyone votes once per round — exactly the shape of
the 512-seat availability committee in
`decoupled/available_attestation_committee.go` (hash-offset rotation over
the genesis set) — is fine.

## S1 — RECOMMEND: mock FFG committee, availability-style

**Replaces:** the epoch-shuffled attester committees on the FFG vote path —
RANDAO-seeded shuffle, `BeaconCommittee`/`SlotCommitteeCount`, committee
caches, attester duty enumeration, aggregator selection, and the
epoch-domain derivation for attester signatures.

**With:** `FFGSeats(slot, index)` — the same hash-offset rotation over the
genesis set the availability committee uses, one vote per validator per
round at its slot offset, a fixed sha256 domain (the availability stream's
pattern), verification by seat arithmetic + genesis-set pubkey.

**Deletes, in the plan:** most of detailed step 3 (the committee/domain
sweep: `ValidateSlotTargetRound`'s committee coupling, domain-from-slot at
`validator/client/attest.go` and every server-side `DomainBeaconAttester`
derivation, `ActiveValidatorCount`-from-target fixes, the classified
`Target.Epoch` audit), the aggregator/dedup items in 3.6, and the entire
"committee construction stays epoch-based" corollary — not because
committees move to rounds, but because no shuffled committee sits on the
vote path at all.

**Problem classes eliminated:** the mixed-units class that dominated the
compile sweep's judgment calls (an epoch-shuffle value borrowed from a
round-valued target field); the missed-domain-site class ("a missed site is
an immediate signature failure"); aggregator-selection edge cases.

**The aggregation sub-choice** (must be made consciously): today FFG votes
are bitfield-aggregated per committee, aggregated again by aggregators, and
packed. Two options:

- *(a) Individual votes*, like the availability stream: ~one message per
  validator per round (16.5/slot at 132 validators — same count as today's
  unaggregated first hop), no aggregation layer at all. Blocks pack
  proposer-side aggregates or raw votes.
- *(b) Round-identical vote data + free aggregation*: in this design the
  FFG vote's data can be identical for every seat in a round — source and
  target are round constants, and the head vote lives in the availability
  stream, not here (that decoupling is the project). Identical data makes
  network-wide progressive BLS aggregation trivial: one bitfield over 512
  seats per round.

Option (b) keeps an aggregated wire (closer to mainnet's shape) while
deleting all matching logic (`SameTarget`, `MatchingStatus` trivialize);
option (a) makes the two streams structurally comparable. Either way the
wire stays *real* — real signatures, real gossip, real timing — but its
SHAPE changes, so the data19/run-02 "availability costs 2.8× the FFG load"
comparison needs a re-baseline run. That is the one genuine cost.

## S2 — rejected (2026-08-22): the real rotating proposer stays

**User decision: no mock proposer — RANDAO-shuffled proposer selection is
kept.** Consequence, stated plainly: the proposer shuffle remains an
epoch-shuffle consumer on the hot path, so `dependentRootForEpoch`, the
2-epoch prune horizon (detailed 1.3/1.3a), and `getRecentPreState`'s
shuffle-compat branch all STAY. S1's deletion payoff narrows to the
committee/duty/domain/aggregation mass; the fork-choice lifetime machinery
survives and its plan sections remain binding. The original recommendation
is kept below for the record.

## S2 (original recommendation, superseded) — mock proposer selection

S1 alone does not free fork choice: `dependentRootForEpoch` — the committee
anchor — is also consumed by the proposer-shuffle checks (`Store.insert`'s
proposer-boost check, `validate_beacon_blocks`' proposer verification,
`getRecentPreState`'s shuffle-compat branch, the PTC payload-attestation
path at `receive_payload_attestation_message.go`). Mock the proposer
schedule too (deterministic rotation over the genesis set; RANDAO becomes
decorative) and the last epoch-shuffle consumer on the hot path is gone.

**Then, by construction:** `dependentRootForEpoch` is deleted; the prune
horizon returns to the finalized round — **the entire prune-wedge bug
class, the 2-epoch retention floor, and its interaction with the
offset-aware child bound cease to exist** rather than being carefully
handled (detailed 1.3, 1.3a, the lifetime-audit rule's main referent);
`getRecentPreState`'s shuffle-compat restatement goes with them.
Networking research does not care who proposes — block timing and size are
what the wire sees, and those stay real.

## S3 — RECOMMEND: seat-counted quorums, seat-bitfield participation

**Replaces:** balance-weighted J&F (full-registry precompute scans per
round boundary, effective-balance sums, 2/3 of active gwei) and the
per-validator participation-flag machinery whose rotation placement forced
the trickiest part of the cadence split (the boundary-kind dance in 2.2,
the doubled precompute at epoch boundaries, the prev-round-uses-epoch-
active-set quirk).

**With:** J&F counts seats — quorum = ≥⌈2/3·512⌉ = 342 seats — over two
512-bit round bitfields (current/previous round) owned by the J&F code
alone. On the equal-stake devnet, seat count ≡ balance weight numerically,
so nothing measured changes. Justification bits and the k-finality rules
are untouched (they consume booleans).

**Consequence for the cadence:** `processRoundHeze` collapses to "count
seats, run J&F, rotate two bitfields"; the epoch boundary calls stock
`processEpochGloas` **verbatim** — the Heze epoch twin, its re-run
precompute, and the coinciding-boundary value-identity analysis all
disappear. The stock participation arrays keep rotating on the stock epoch
path feeding the (don't-care) rewards garbage-in-garbage-out; J&F never
reads them. Heze-gated, so spectests and shipped configs never see it.

## S4 — RECOMMEND (narrowed): keep the retype and identity for shared code only

Dropping the identity trick wholesale is a mistake: the `cast_type` retype
made the compiler the auditor (the review found ~25 mixed-unit sites the
hand enumeration missed), and zero-expectation-edit runs of the existing
suite caught real value bugs for free. **Keep both** — for code that stays
shared. What changes: S1–S3 move the vote path into new Heze-only
components that stock configs never execute, so the identity discipline's
surface (and the spectest survey's relevance) shrinks to the genuinely
shared remainder (forkchoice container, gossip plumbing, state). No survey
extension for new components; no "byte-identical at coinciding boundaries"
proofs anywhere, because nothing coincides anymore.

## S5 — REJECT: merging the vote streams

One message carrying head + FFG + availability would be simpler — and would
delete the experiment. The measured object of this project IS the separate
availability stream and its cost relative to the FFG stream. The three
streams stay three streams.

## S6 — REJECT (for this stub): dropping the source checkpoint

Tempting once slashing is out (surround protection is the source's other
job), but the source-match is what anchors a Casper link — "justify on
target quorum alone" is a different gadget with a new safety argument.
That is the Simplex-era container change, explicitly deferred. On the wire
a source is 40 bytes inside a vote whose data S1(b) makes round-constant —
it costs nothing to keep. Keep it.

## S7 — REJECT: slot-valued checkpoints, and removing the offset knob

Slot-valued checkpoints re-ripple every container for zero networking
gain (the user already chose smallest-change here). The
`FFG_TARGET_OFFSET_SLOTS` knob is a *vote-timing* experiment parameter —
squarely networking research — and its cost after S2 shrinks to the two
target-resolution sites (the forkchoice-geometry arms it complicated are
mostly gone with the epoch machinery). Both stay.

## S8 — NOTE: epoch-processing deletion is not worth it

With S3, the round path no longer touches the epoch body at all, and the
epoch body is stock code — untouched code is free. Deleting registry /
effective-balance / sync-committee processing would be *work* to remove
things that run correctly and quietly on a static validator set. Don't.

## The minimal design, sketched

Per slot: a (mock-rotation) proposer publishes a real block; every seat
holder of the availability committee publishes its availability vote (the
goldfish stream, unchanged); the ~1/8 of validators whose FFG seat falls
in this slot publish their FFG vote — source = justified round checkpoint,
target = round target at the configured offset, seat-verified against the
genesis set, aggregating freely because the data is round-constant. Per
round boundary: count seat bitfields, justify/finalize with the unchanged
Casper rules, prune fork choice at the finalized round. Per epoch
boundary: stock Gloas epoch processing, whose outputs nothing measured
reads. Fork choice holds LMD (from the availability/goldfish side), a
round-target tree with no epoch anchors, and checkpoints in rounds.
Everything the wire carries — blocks, availability votes, FFG votes — is
really signed, really gossiped, really timed.

What this deletes from `plan-complete-detailed.md`: most of step 3, the
1.3/1.3a dependent-root and horizon sections, 2.2's pair-and-precompute
machinery, the 4.x shuffle-compat items, and the lifetime-audit rule's
motivating case — on the order of a third of the detailed file, and
several of its hardest-won sections.

## Migration note

*From the landed implementation:* S1–S3 replace working code (sunk cost);
the deletions are mechanical but the re-baseline run (new FFG wire shape)
is mandatory before comparing against data19/run-02 numbers.

*For the replay experiment:* the simplifications favor the replay
enormously — the plan shrinks by roughly a third and sheds its two worst
bug classes by construction. But be explicit about what the experiment then
tests: **replaying the simplified design is a different-design experiment,
not a better-documentation experiment.** If the point is to measure
whether the complete plan alone speeds execution, replay the same design;
if the point is the fastest correct path to networking numbers, replay the
simplified one. Choose one; doing both arms is also coherent.

## Ranked recommendations

1. **S1+S2 (mock FFG committee + mock proposer)** — largest deletion, two
   whole bug classes (prune-lifetime, domain/committee mixed-units)
   eliminated by construction; realism lost is stake-shuffle and
   aggregation shape, neither of which the equal-stake networking
   measurements consume — at the price of one re-baseline run.
2. **S3 (seat quorums + round bitfields)** — deletes the precompute scans,
   the rotation-placement dance, and the Heze epoch twin; numerically
   identity on the devnet.
3. **S4 (identity kept, narrowed)** — keep the compiler-as-auditor and the
   free regression net where code is shared; exempt the new Heze-only
   components.

Rejected: stream merging (S5 — deletes the research object), source
removal (S6 — a different gadget, deferred), slot-valued checkpoints and
knob removal (S7).
