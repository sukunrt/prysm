# Round-1 to round-2 source audit

This audit compares the historical Prysm revisions used by the two distributed
runs:

- round 1: `a1679c9fd82a47b3cea16ca65c84d2c4d4501fcb`
- round 2: `0280403c70d88967f49d2d4c730f4c5417dabdf5`

It is deliberately scoped to the genesis FFG validation, fork-choice/async
locking, validator role preflight, domain lookup, and summary paths.  Source
equivalence along these paths is not equivalence of the complete builds,
network conditions, runtime schedules, or genesis/config artifacts.

## Why round 2 has no summary lines

The missing round-2 summaries are explained by source changes, not by an
inactive Heze/Gloas fork or proof that no votes arrived.

Round 1 contains `beacon-chain/sync/ffg_summary.go`.  Its counter is explicitly
defined as accepted FFG votes by vote slot and subnet (lines 18–29), and its
slot timer emits the INFO message `FFG votes` (lines 82–119).  Round-1
`validateBeaconAttestation` calls `recordFFGVote` and `countFFGVote` immediately
before returning `pubsub.ValidationAccept`
(`beacon-chain/sync/validate_beacon_attestation.go:240–246`).

Round 2 deletes `ffg_summary.go` and the `countFFGVote` call.  Its corresponding
accepted path calls only `logFFGVote` before `ValidationAccept`
(`beacon-chain/sync/validate_beacon_attestation.go:240–245` at
`0280403c...`).  That per-vote ledger is feature/flag dependent; it is not the
removed unconditional per-slot counter.

Round 1 also logs the INFO message `Goldfish votes` from
`goldfishNewSlot`, including voters and seats
(`beacon-chain/forkchoice/doubly-linked-tree/goldfish.go:215–229`).  Round 2
removes the `voters` helper and INFO log while retaining the seat metric and
vote-store pruning (`goldfish.go:208–218` at `0280403c...`).  Consequently the
absence of both messages on all three recovered round-2 proposer owners is the
expected behavior of that binary and gives no bound on round-2 FFG admission.

The recovered round-2 beacon logs independently show subscriptions to all six
`beacon_attestation_<subnet>` topics plus `available_attestation` and
`payload_attestation_message` at genesis.  Their validator logs submit
slot-0 payload attestations.  Those observations rule out interpreting the
summary silence as a simple failure to activate or subscribe to the devnet
fork, but do not count successfully validated FFG gossip.

## Relevant operations retained in round 2

### Genesis active-validator scan

Both revisions call `helpers.ActiveValidatorCount` from
`validateCommitteeIndexAndCount` after the fork-specific committee-index check
(`beacon-chain/sync/validate_beacon_attestation.go:268–310` in round 1 and
`:267–309` in round 2).  Surrounding code did change: round 1's
`validateUnaggregatedAttTopic` returns the computed subnet so the removed
summary can count it, while round 2 returns only the validation result.  The
active-count call itself and its position in validation are unchanged.

`helpers.ActiveValidatorCount` is identical at
`beacon-chain/core/helpers/validators.go:145–174`.  Its committee-cache fast
return requires both a nonzero cached count and `s.Slot() != 0` (lines
150–155).  With a slot-0 checkpoint state it reaches the fallback
`ValidatorsReadOnlySeq` iteration (lines 167–172), even when the cache contains
the count.  The round-1 reproduction therefore exercises an operation still
present in the round-2 binary.

### Fork-choice reader and checkpoint-key serialization

In both revisions, `AttestationTargetState` takes the fork-choice `RLock` and
holds it across `getAttPreState`
(`beacon-chain/blockchain/receive_attestation.go:42–53`).  In both,
`getAttPreState` creates and acquires a multilock keyed by checkpoint root plus
round before consulting the checkpoint-state cache
(`beacon-chain/blockchain/process_attestation_helpers.go:104–124`).  Warm
callers can therefore serialize while retaining the outer fork-choice reader.

The relevant `async.MultiLock` implementation has no scoped diff between the
two revisions.  Its registry is package-global (`async/multilock.go:22–28`),
and every `Unlock` invokes `Clean` (`:54–67`); `Clean` and `getChan` share the
same global registry lock (`:92–124`).  Thus the checkpoint-key reader convoy
and its cross-key cleanup coupling are source-applicable to round 2.

### Synchronous role preflight and domain cache lock

Both revisions collect all wallet keys in the current sync committee and call
`SyncCommitteeAggregators` synchronously before returning `RolesAt`
(`validator/client/validator.go:622–653`).  `runner.go:147–156` waits for that
result before dispatching proposer work.  The round-2 proposer owners' logged
ordering—sync preflight expiry followed by RANDAO on the already expired slot
context for slots 2 and 3—is therefore governed by the same source structure.

Both revisions' `domainData` first check the cache under `RLock`, then take the
single `domainDataLock` exclusively on a miss and hold it across the beacon-node
RPC (`validator/client/validator.go:721–755`).  This remains a possible
round-2 VC-side amplifier, particularly for the unresolved slot-1 RANDAO
failure, but the historical logs do not divide VC lock wait from BN RPC time.

## Functional differences and inference boundary

The revisions are not fully equivalent.  In addition to removing the summary
instrumentation, round 2 removes the consensus-block and Goldfish scratch-space
configuration fields and their validation (`config/params/config.go:106–108`,
`mainnet_config.go:147–149`, and `scratch.go` in round 1).  This changes message
size/workload relative to a round-1 build configured with nonzero scratch
space.  It can affect bandwidth, allocation, encoding, and scheduling pressure,
but it does not remove the active-count call or either lock ordering described
above.  The return-signature change in `validateUnaggregatedAttTopic` likewise
supports removal of subnet summary accounting; it is not a bypass of committee
count validation.

Other files changed between the commits and were not declared
equivalent by this scoped audit.  The 120,000-validator registry, per-machine
process topology, message arrival distribution, CPU availability, fork epochs,
and exact genesis state must come from each run's artifacts rather than source
comparison alone.

The correct conclusion is therefore limited: the round-1 reproduction proves
that causal operations which also exist in the exact round-2 source can exhaust
local proposal capacity.  It does not measure how many round-2 votes completed
validation, assign round-2 CPU share, prove that every round-2 deadline used
that chain, or transfer a quantitative threshold from the local topology to
the historical deployment.
