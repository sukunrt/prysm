# Genesis full-head and Engine timeline

Scope: round1 `prysm-geth-3` and round2 `prysm-geth-1`, slots 0--3.
Round1 used `a1679c9fd82a47b3cea16ca65c84d2c4d4501fcb`; round2 used
`0280403c70d88967f49d2d4c730f4c5417dabdf5`. The relevant historical code is
the same in both revisions. This note records causal bounds, not a production
fix or a complete explanation of the proposal delay.

## What the duplicate reorg records mean

Gloas genesis insertion creates both empty and full payload nodes, with the
full node marked `full=true` (`beacon-chain/forkchoice/doubly-linked-tree/store.go:199-210`).
Startup stores `FullBeatsEmpty(genesisRoot)` in the service head
(`beacon-chain/blockchain/service.go:397-403`; the saved-state path does the
same at lines 339-354). At slot 0 the equal-weight genesis pair selects full:
`choosePayloadContent` only applies its previous-slot exception when
`n.slot+1 == currentSlot`, which is false at slot 0
(`beacon-chain/forkchoice/doubly-linked-tree/gloas.go:319-338`). Thus the
initial cached genesis head is full.

For the same root, `isNewHead` returns true only when the full bit changes
(`beacon-chain/blockchain/forkchoice_update_execution.go:21-32`). `saveHead`
rechecks that condition, emits the misleading genesis reorg because the
genesis block's zero parent differs from the old genesis root, and commits the
new full bit before returning (`beacon-chain/blockchain/head.go:65,103-164`).
Consequently the two same-root records prove a serialized full-bit round trip:

| Run | First record | Second record | Proven transitions |
|---|---:|---:|---|
| round1 node3 | `beacon.log:899`, 00:00:12.10 | `beacon.log:903`, 00:00:22.46 | full to empty, then empty to full |
| round2 node1 | `beacon.log:1075`, 01:30:12.000992 | `beacon.log:1224`, 01:30:28.577734 | full to empty, then empty to full |

At the slot-1 boundary raw fork choice selects full: `NewSlot` clears proposer
boost (`on_tick.go:32-40`), and an equal-weight previous-slot pair extends full
when the proposer-boost root is zero (`gloas.go:280-299,319-338`). The first
full-to-empty transition therefore comes from `UpdateHead`'s later
`shouldBuildOnFullLocked` override (`receive_attestation.go:150-157`), whose
slot-1 alternatives are a payload-missing PTC majority or a known-late payload
for a tracked proposer (`blockchain/gloas.go:324-350`). Before round2's first
record, its log contains 508 distinct slot-0 PTC validator indices reporting
`payloadPresent=false` (`beacon.log:566` onward), above the 256-vote threshold.
Those are validation-side records; the logs do not timestamp insertion of each
vote into the fork-choice node.

## Gloas FCU lock bound

`UpdateHead` holds the fork-choice write lock from
`receive_attestation.go:127` through `saveHead`, pool pruning, and return
(`receive_attestation.go:183-186`). For Heze it starts
`go fcuFromReorgData(...)` before returning (`receive_attestation.go:170-173`).
The new goroutine cannot reach the Engine immediately: `fcuFromReorgData`
calls `notifyForkchoiceUpdateGloas`, which first needs the fork-choice read lock
to copy finalized and justified payload hashes
(`receive_execution_payload_envelope.go:455-462` in round1, 463-470 in
round2). It releases that read lock before calling Engine
`ForkchoiceUpdated` (round1 lines 472 onward; round2 line 480 onward). Therefore:

1. An observed Gloas Engine FCU request proves that the originating
   `UpdateHead` write lock had already been released.
2. Engine response time is not time spent holding the fork-choice lock.
3. The request does not prove that no fresh fork-choice writer acquired the
   lock afterward.

All Engine FCUs visible during slots 0--3 on the two selected nodes are:

| Run | Head transition | Engine evidence | Attributes | Lock implication |
|---|---|---|---|---|
| round1 node3 | full to empty at 12.10 | `snooper-engine.log:20851,20874`, request/response #466 at 00:00:12, 2 ms; Geth starts payload work at `execution.log:98` at 12.114 | slot-1 attributes present | The first `UpdateHead` released the fork-choice lock by 12.114. Its FCU/Engine call cannot be the multi-second lock owner. |
| round1 node3 | empty to full at 22.46 | `snooper-engine.log:20935,20950`, request/response #468 at 00:00:25, 1 ms | null | The corresponding `UpdateHead` released by the request at second-resolution 25; the snooper lacks an outer subsecond timestamp in round1. |
| round2 node1 | full to empty at 12.000992 | `snooper-engine.log:40072,40087`, request #896 at 12.004273639 and response at 12.005047640, 1 ms | null | The first writer released no later than 12.004273639. |
| round2 node1 | empty to full at 28.577734 | `snooper-engine.log:40196,40211`, request #899 at 42.756098496 and response at 42.757247712, 1 ms | null | The originating writer released by 42.756098496; the 14.18-second post-log gap can include its remaining locked work, feed delivery, pruning, scheduling, or intervening writers before the FCU goroutine obtains its read lock. |

No other `engine_forkchoiceUpdatedV4` request appears through slot 3 in either
selected snooper log. The periodic `eth_getBlockByNumber` calls are execution
metadata polling, not beacon fork-choice updates.

## Bound on round1's proposal gap

Round1 logs `Building block` at 18.973 (`beacon.log:902`), the second false
reorg at 22.46 (`beacon.log:903`), the associated null-attribute Engine FCU at
second-resolution 25 (`snooper-engine.log:20935`), and graffiti generation at
30.10 (`beacon.log:906`). Because the FCU cannot start until its originating
`UpdateHead` releases the write lock, at least roughly five seconds of the
Building-to-graffiti interval occur after that particular writer released.
They cannot be charged to that writer's synchronous reorg feed send or other
fork-choice critical-section work.

This does **not** prove that the proposal handler's own `UpdateHead` had
finished by 25: the logs do not attach caller identity to either transition or
FCU. Nor does it prove the handler was free of fork-choice waiting after 25.
The slot-2 `NewSlot` writer was nominally due at 24 seconds, other `UpdateHead`
callers may queue, and Go's `RWMutex` blocks new readers behind a waiting
writer. Fresh writers and reader/writer drain can therefore intervene before
the handler progresses. The remaining interval also includes uninstrumented
parent-state preparation, empty-block construction, scheduling, and delayed
log emission. The evidence splits the interval around one known writer; it
does not identify the residual five-second cause.
