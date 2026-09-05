# What changes from slot 97 to slot 110 in the deployed source

This is a source comparison at deployed Prysm commit
`0280403c70d88967f49d2d4c730f4c5417dabdf5`, using the round-2 configuration
of eight slots per FFG round, 32 slots per epoch and target offset one.
The subsequently retrieved owner-148 records establish parent 109: its beacon
block and envelope were imported before proposal 110, and the next-slot
rollback identifies 109 as 110's parent/common ancestor. The slot-97
benchmarks are controls, not measurements of slot 110.

| Property | Proposal 97, parent 96 | Proposal 110, parent 109 |
| --- | --- | --- |
| Epoch | 3 | 3 |
| Current FFG round | 12 | 13 |
| Offset within round | 1 | 6 |
| Current / previous target checkpoint slots | 95 / 87 | 103 / 95 |
| Potentially eligible attestation slots before the proposal | 88–96 | 96–109 |
| Epochs used for eligible attestation committees | 2 and 3 | 3 |
| Background expiration cutoff | Before slot 64 | Before slot 64 |
| Round or epoch boundary crossed by the one-slot parent advance | Neither | Neither |

The target checkpoint slots identify the slots whose block roots are looked
up; a skipped slot can resolve to an earlier block root. Eligibility also
requires matching source checkpoints and the remaining attestation checks.
The table does not assert that all these slots, candidates or roots were
present in the historical proposer pool.

## No new boundary-processing branch for 109 to 110

`ProcessSlotsCore` calls `ProcessSlot`, `ProcessRound` and `ProcessEpoch`
before incrementing the state slot
(`beacon-chain/core/transition/transition.go:293–320`). The expensive round
and epoch bodies are gated by `(state.slot + 1) % slotsPerRound == 0` and
the analogous epoch check
(`beacon-chain/core/time/slot_epoch.go:147–156`). Both 96→97 and 109→110
skip those bodies. The round boundary was 103→104; the next is 111→112.

Heze's round body initializes validator precomputations, processes
participation, updates justification/finalization and rotates participation
arrays (`beacon-chain/core/transition/heze.go:28–49`). Thus an actual parent
at or before 103 could change this conclusion if its state needed advancing
through 104. `getParentStateFromReorgData` performs that advance before
`BuildBlockParallel` (`validator/proposer.go:157–182`), and the post-block
state calculation can independently process slots from its parent state.
The historical parent and milestone order are therefore material evidence.

For a full parent, the Gloas builder applies the parent's execution requests
before spawning consensus construction and obtaining the local payload
(`validator/proposer_gloas.go:18–39`). This branch depends on the chosen
parent's full/empty status and its requests, not on round 12 versus 13.
Both non-genesis parents take the envelope lookup path
(`validator/proposer_execution_payload.go:300–330`). Parent requests,
payload contents, cache misses and retries must be checked for proposal 110
instead of inheriting slot 97's empty-payload evidence.

## A wider eligible window, but unchanged algorithms

Attestation validation accepts only the state's current or previous FFG
round. It compares the source to the current justified checkpoint for
current-round votes and to the previous checkpoint for previous-round votes,
then checks the slot/target-round relation and minimum inclusion delay
(`beacon-chain/core/blocks/attestation.go:64–114`). This gives nine possible
attestation slots at proposal 97 and fourteen at proposal 110. At 110,
round-12 votes use previous participation and round-13 votes use current
participation, reflecting the rotation already performed at 104.

The old Gloas/Electra inclusion-pruning mismatch is unchanged. Valid retained
votes can still be reread, validated, aggregated and scored before the final
block limit. The wider eligible window permits more different slot/data
groups; it does not establish how many objects or uncovered raw singles
were present. Reward ranking uses the actual participation bits and
inclusion delays (`core/electra/attestation.go:27–105`), so the historical
selection and work cannot be inferred from the slot-97 fixture's checkpoint
and credited-vote state alone.

Background pool expiration remains **epoch-based** after Deneb:
`providedEpoch + 1 < currentEpoch`
(`operations/attestations/prune_expired.go:107–136`). Both proposals are in
epoch 3, so entries from slots 64 onward can survive this expiration rule.
At proposal 110, attestations before 96 fail the two-round proposal check
early, before active-validator and committee work. The proposer then deletes
invalid entries using the original pool object, preserving its version key
(`validator/proposer_attestations.go:182–197,419–454`). This differs from the
included-block Electra deletion that misses a stored Gloas aggregate.
Therefore pool retention is neither an unlimited accumulation claim nor a
proof that every retained old entry incurs the expensive packing stages.

## Cache and current-slot validation transfer

Active-validator counts and committee seeds use the **epoch derived from the
attestation slot**, not its round-valued target. Nonzero states use a cached
active count when present (`core/helpers/validators.go:145–174`), and
committee lookup uses the epoch seed plus the slot's offset within its
eight-slot round (`core/helpers/beacon_committee.go:181–202,242–272`). Moving
from round 12 to 13 inside epoch 3 does not by itself force a new active-set
scan or a new epoch shuffle. Proposal 110's eligible slots all use epoch 3;
proposal 97 could additionally use epoch 2. This is a source property, not
proof that a particular historical cache was warm.

For ordinary current-round gossip, compatible head 109 is already in round
13. `getRecentPreState` can return its read-only head directly, as compatible
head 96 could for round-12 gossip; it does not copy or advance a native
120,000-validator state per vote
(`blockchain/process_attestation_helpers.go:23–64`). A different fork or
missing state can take the checkpoint/regeneration path, whose occurrence
requires evidence. Current-slot-110 singles remain ineligible for block 110
by minimum inclusion delay, just as slot-97 singles were for block 97.

The source therefore identifies specific limits on transferring the earlier
controls: the eligible candidate window, actual checkpoint/participation
state, proposer-specific pool history, parent status and concurrent message
schedule can differ. It does not identify a new automatically expensive
round/epoch operation at slot 110 when its parent is 109, and it does not
assign proposal 110's historical delay without its retained timeline.

## Owner 148: observed slow-reader warning during construction

The retrieved owner log is
`round2-slot110-owner-node148/beacon.log`. It records parent 109 imported at
`01:51:48.974389536Z`, its envelope at `01:51:49.396396041Z`, and then:

| Event | UTC time | Raw line |
| --- | --- | ---: |
| Building block 110 | 01:52:00.016983760 | 934 |
| Chose self-build payload bid | 01:52:00.024960787 | 936 |
| SSE client cannot keep up; shutting down | 01:52:01.725042996 | 937 |
| Finished building block 110 | 01:52:03.409893292 | 938 |

The payload-choice-to-finish interval is 3.384933 seconds. The SSE warning
falls inside it; construction still takes another 1.684850 seconds after
the warning's outer timestamp. Unlike the slot-97 immediate-reader control,
this owner has direct evidence of an overflowing SSE outbox during its build.
The earlier control did not reproduce this backpressure condition, so its
negative timing result cannot exclude all SSE effects on owner 148.
The warning does not name the client, topic, queue occupancy history or how
much time the builder spent waiting. The absence of a vote ledger in this
owner's compact log prevents reconstructing its validation-entry schedule.

In exact 028, `safeWrite` does not block on a full outbox: its nonblocking
send returns `errSlowReader`, logs this warning and exits the receive loop
(`rpc/eth/events/events.go:282–313`). Deferred unsubscription runs before
the HTTP handler's `waitForExit` waits for the writer goroutine
(`events.go:232–240,253–264,378–385`). A blocked HTTP write can therefore
outlive removal of that stream from the event feed; its socket timeout is
not an automatic equivalent-duration stall of the feed or builder.
Slow conversion, a slow HTTP reader or insufficient writer scheduling can
each fill the outbox; the warning alone does not distinguish them.

The operation feed is synchronous and its subscribed receive channels are
unbuffered. It can delay FFG validation, which sends after its state-check
calls and before the subscriber inserts the vote into the candidate pool
(`sync/validate_beacon_attestation.go:229–243`). That send does not hold the
candidate-pool mutex or retain `AttestationTargetState`'s forkchoice read
lock. `BuildBlockParallel` itself sends no operation event. This permits
shared-runtime effects, but supplies no direct socket-to-builder lock chain.

There is a narrower transitive route for **state** events: `saveHead` emits
a reorg event while its caller holds forkchoice
(`blockchain/head.go:60,134`; `receive_attestation.go:127–128,189`), and
proposer target-root lookup takes a forkchoice read lock
(`blockchain/chain_info.go:563–566`). Ordinary new-head notifications instead
run in goroutines after `setHead` has released the head lock
(`head.go:164–185,229–247`). Owner 148 records no new import or reorg during
the joined interval: parent 109 was already available and the rollback occurs
at `01:52:12.004516172Z`, after construction. Thus this possible state-feed
bridge has not been demonstrated as the slot-110 wait.

Owner Xatu also reports a full asynchronous export queue at
`01:52:00.307372415Z` (`xatu-sentry.log:1310`), followed by a minute summary
with 68,016 single-attestation events (`:1312`). Its same deployed async
implementation drops attempts when full; the prior
[exact Xatu lock audit](xatu-sse-contention-audit.md) still applies. Neither
that export error nor the SSE warning establishes which side caused the
construction delay.

A source-specific control, if needed, would test the actual overflowing
outbox lifecycle: hold only an SSE writer, fill its default outbox, observe
the warning/unsubscription, and verify that other feed sends progress while
the writer remains held. It would test the direct-backpressure mechanism,
without inventing owner-148 vote timings or asserting a historical client
pause duration. Existing `TestStuckReaderScenarios`
(`rpc/eth/events/events_test.go:1139–1224`) already exercises overflow and
write-timeout shutdown, though its early return skips the later
`eventsWritten` assertion. A new paced build with an arbitrary sleeping
client would not identify why this historical build took 3.4 seconds.
The warning is therefore an observed symptom of a consumer falling behind,
not an identified cause of the block-construction delay. No additional
synthetic backpressure benchmark is recommended to claim historical attribution.

## One real payload-attestation aggregate was omitted by earlier controls

Owner 148's validator record reports one payload-attestation aggregate in
block 110, in addition to eight FFG attestations and 511 sync bits. The
earlier full-build fixtures used an empty payload-attestation pool. This is
a concrete input difference, with a bounded production path:

- `getPayloadAttestations` requests the previous-slot entries, filters their
  slot and parent root, then sorts them
  (`validator/proposer_payload_attestation.go:20–77`).
- `PendingPayloadAttestations` takes the PTC pool mutex, scans the map and
  copies matching aggregates (`operations/payloadattestation/pool.go:56–69`).
  Insertions take that same mutex, prune older entries and combine BLS
  signatures under it (`pool.go:94–152`). Concurrent insertions can contend
  with the getter, but neither operation scans the validator registry.
- The block transition looks up the cached PTC for slot 109, visits its
  selected indices and performs one aggregate-signature verification
  (`core/gloas/payload_attestation.go:47–111,325–361`). The cached lookup
  returns the state's committee slice under a short read lock
  (`state/state-native/getters_gloas.go:730–750`). It does not call
  `computePTC` or regenerate committees. The mainnet PTC has 512 positions
  (`config/fieldparams/mainnet.go:55`).

Consequently the omitted aggregate adds at most 512 committee positions and
their public-key reads/signature work, plus its actual pool snapshot/lock
cost. It is not a newly discovered 120,000-validator mapping or round
transition. A one-PTC full-build arm could quantify the isolated input
difference; it would not establish an unlogged multi-second mutex wait.
