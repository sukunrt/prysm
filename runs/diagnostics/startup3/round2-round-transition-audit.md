# Round-2 empty-chain round transition audit

This note predicts the exact round-2 source behavior while the canonical head
remains the genesis block.  It does not infer arrival counts from the removed
round-2 FFG summary logs.

With `SLOTS_PER_ROUND=8`, votes in slots 0–7 target round 0.  The
`getRecentPreState` fast path explicitly rejects checkpoint round zero
(`beacon-chain/blockchain/process_attestation_helpers.go:22–29`).  The
checkpoint cache/regeneration path therefore supplies the genesis checkpoint
state at slot 0.  `ActiveValidatorCount` refuses its cached-count fast return
when `s.Slot() == 0`, then iterates `ValidatorsReadOnlySeq`
(`beacon-chain/core/helpers/validators.go:145–174`).  This is the repeated
120,000-validator scan reproduced locally.

At slot 8, new votes target round 1.  With no intervening blocks, their FFG
target slot 7 resolves to the latest/genesis block root, but checkpoint identity
is root **plus round**.  `getAttPreState` therefore uses a new key, regenerates
from the genesis root, calls `ProcessSlotsIfPossible` through
`RoundStart(1) == 8`, and caches a state whose slot is 8
(`process_attestation_helpers.go:104–172`).  The unchanged root does not imply
an unchanged state slot.  Once the committee cache has a nonzero count for the
seed, `ActiveValidatorCount` can use its constant-time return because the state
slot is no longer zero.  Round 2 at slot 16 analogously materializes slot 16.

This creates a cheap path for newly generated round-1 votes, but it does **not**
stop round-0 attestations from entering the expensive gossip-validation path at
slot 8 or slot 16.  Gossip first applies `helpers.ValidateAttestationTime`
(`beacon-chain/sync/validate_beacon_attestation.go:87–96`).  Since Deneb, that
gate retains attestations from the current or previous **32-slot epoch**, not
the current or previous eight-slot Goldfish round
(`beacon-chain/core/helpers/attestation.go:131–190`).  Attestations whose own
slots are 0–7 are epoch 0 and can therefore remain gossip-time-valid throughout
epoch 0 and as the previous epoch during wall slots 32–63 (subject to the small
future-clock tolerance at their initial arrival).  Distinct delayed round-0
votes can continue to obtain a slot-0 target pre-state and run the repeated
registry scan long after the first round transition.

The round-valued freshness check does not protect this earlier path.
`AttestationTargetState` checks only that the target round's start slot is not
unreasonably in the future via `slots.ValidateClock`; that function has no
staleness check (`beacon-chain/blockchain/receive_attestation.go:42–53`,
`time/slots/slottime.go:313–320`).  `helpers.ValidateSlotTargetRound` checks
that an attestation's target matches its own slot.  The current/previous-round
`verifyAttTargetRound` check occurs later in `OnAttestation`, after
`getAttPreState` (`beacon-chain/blockchain/process_attestation.go:38–69`).  A
late round-0 vote can consequently consume the expensive gossip validation
and then be rejected for fork-choice consumption.

This is observed, not merely theoretical.  Round-2 node201 line 35813 records
an attestation with `attSlot=2`, `targetRound=0`, and `arrivedMs=172974` at
01:33:17, wall slot 16.  It entered validation at 01:33:16.974 and was logged
roughly 29 ms later, directly showing round-0 traffic beyond the slot-8
transition.

Recovery near slot 15 therefore cannot be explained solely as “new expensive
scan creation stopped at slot 8” or as only a previously captured backlog
draining.  The slot-8 transition introduces cheap round-1 work alongside
still-admissible expensive round-0 traffic.  Recovery can reflect a decline in
late round-0 arrival/admission rate, completion or throttling of that traffic,
and drainage of already admitted lock/CPU queues, but the source transition
alone does not determine their proportions or the slot-15 timing.

The new-round cheap-path prediction has conditions.  The first caller for a missing or
new committee seed may run `scanActiveValidatorIndices`; that scan is
singleflight and fills the committee cache asynchronously
(`beacon-chain/core/helpers/beacon_committee.go:579–624`).  The target must also
be fork-choice viable and regenerable.  Failure of
checkpoint viability, state lookup, or slot processing causes validation to be
ignored before the later committee check.  Pubsub admission throttling can
prevent a message from entering this path at all.  These conditions affect
traffic and drain rate.  None makes a round-1 checkpoint backed by genesis root
remain a slot-0 state, and none supplies the previously claimed round-16 cutoff
for expensive late round-0 gossip.
