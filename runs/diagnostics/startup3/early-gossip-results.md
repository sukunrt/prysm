# Early-gossip reproduction E1

E1 used the 120,000-validator genesis fixture, three beacon nodes, and one
596-key validator client attached to BN3.  The wallet included the proposers
for slots 1–3 and two sync-committee validators (indices 119 and 138).  The
source submitted 15,000 valid FFG attestations per slot to BN1; BN1 gossiped
them to BN2 and BN3.  All 45,000 source RPCs were accepted.  For each of slots
1, 2, and 3, its 15,000-RPC batch ran from approximately 391 ms before that
slot's start through 1.599 s after it; no batch was sent around slot 0/genesis.
The genesis time was
`1788617038` (`2026-09-05T14:03:58Z`).

The 45,000 accepted source RPCs are not 45,000 completed BN3 validations.
The retained E1 BN3 `FFG votes` summaries contain **2,336, 0, and 0** for
slots 1–3, at their roughly +6-second aggregation ticks (`beacon3.log:679,
1201,1552`). `countFFGVote` runs immediately before gossip
`ValidationAccept`; the summary takes that slot's counters once and deletes
all counters through that slot. Later completions can recreate an old bucket,
which the next summary discards without reporting it. These lines therefore
measure accepted completions by the summary tick, not eventual per-slot
totals (`beacon-chain/sync/ffg_summary.go:55–119`).

The separate retained after-scrape counters contain 41,524 BN3 pubsub
validation-attempt events, including 25,968 validation-throttled rejects,
and 15,556 deliveries on the six attestation topics. The before scrape has
no corresponding populated series. These counters distinguish source RPC
success, validation attempts, and completed pubsub delivery; they do not
locate the 3,476-source-submission difference before the validation counter.
Bounded raw anchors and the comparison with F1, B/D, and H/I2 are preserved in
[the receiver-acceptance audit](../round2-slots-0-16/ffg-receiver-acceptance-audit.md).

The separate `Goldfish votes` summaries describe available-attestation votes,
not these FFG beacon attestations. Zero Goldfish votes therefore does not
mean the FFG load was absent. The retained E1/F1 configurations used four
slots per round and **six total attestation subnets**; their later slot
bursts reused those six topics.

## Validator-client timeline

All durations below come from the bounded startup diagnostic events in
`/tmp/prysm-startup3-round4-early-e1/validator3.log`.

| Slot | `RolesAt` | Sync selection | Sync-index RPCs | RANDAO | Block RPC | Result |
| ---: | ---: | ---: | --- | ---: | ---: | --- |
| 1 | 5,244.921 ms | 5,244.338 ms | 355.968 ms, 4,881.583 ms | 0.742 ms | 6,759.184 ms | deadline |
| 2 | 17.406 ms | 14.401 ms | 6.629 ms, 1.968 ms | 8.949 ms | 11,970.099 ms | deadline |
| 3 | 11.310 ms | 10.783 ms | 3.699 ms, 4.732 ms | 3.982 ms | 11,985.494 ms | deadline |

Thus E1 reproduced a material sync preflight delay in slot 1, but not a full
12-second preflight timeout.  Proposer dispatch still occurred and RANDAO was
fast in all three slots.  All three real validator block requests subsequently
reached their deadlines.

## Slot-1 sync-head stages

The real VC's first sync-index call maps to BN request 16 and its second maps
to request 21 by start/end time.  Request 23 is a concurrent probe call.

| BN request | Total | Lock acquisition | Cache | Head state | Head root | Process slots | Cache put | Residual before total end |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: |
| 16 | 331.814 ms | 11.460 ms | miss | 0.090 ms | 0.001 ms | 73.815 ms | 0.003 ms | about 246.444 ms |
| 21 | 4,803.963 ms | 8.887 ms | hit | — | — | — | — | about 4,795.074 ms |
| 23 | 4,437.511 ms | 4,437.506 ms | hit | — | — | — | — | approximately zero |

`Lock acquisition` covers the entire `mLock.Lock()` call, so it can include
both package-global registry acquisition and waiting for the logical per-key
token; the timing does not divide those components.  The residual is after
the last instrumented cache phase and before the
function's total timer ends.  Because `mLock.Unlock()` is deferred after the
total timer, Go's LIFO defer order runs `Unlock` first.  `Unlock` calls
`async.Clean`, which scans the package-global multilock registry while holding
its global lock.  Request 21 therefore gives direct evidence of approximately
4.795 seconds in the deferred unlock/cleanup path after an already warm cache
hit.  Request 23 concurrently spent 4.438 seconds acquiring through the same
global manager.  The logical reader key was `syncHeadState-1`; the FFG
checkpoint requests used different logical keys but the same package-global
registry.

The residual calculation is a wall-clock bound, not a CPU-time measurement:
it can include time descheduled while executing or waiting in `Unlock/Clean`.
The goroutine profiles establish that this interval involved blocking rather
than merely expensive state work. The +13-second profile specifically contains
one `GetSyncSubcommitteeIndex -> getSyncCommitteeHeadState -> Unlock -> Clean`
goroutine parked in `runtime.chansend`, and another parked in
`GetSyncSubcommitteeIndex -> getSyncCommitteeHeadState -> Lock -> getChan`.
These are waits on the global registry channel, not an inference based solely
on unrelated gossip stacks. At genesis +13 seconds, 1,583 goroutines
were stacked in `async.(*Lock).Unlock -> async.Clean`, while 6,140 were under
pubsub validation.  At +19 seconds, 1,513 goroutines were stacked in
`async.(*Lock).Lock`.  Head-state copying and slot processing were orders of
magnitude shorter in the directly matched cold request.

## Proposal path and limits of the result

The beacon-node proposal timers independently show fork-choice/head-update
pressure.  Diagnostic IDs preserve request ancestry even when a request
crosses a wall-clock slot boundary.  The slot-1 proposal (ID 46) spent 5.855
seconds in its nested `UpdateHead.waitForForkchoiceLock`; a separate
4.881-second `UpdateHead` in that period was not its child.  The slot-2
proposal (ID 83) spent 13.070 seconds in `CachedHeadRoot.before`, then 11.658
seconds in its nested `UpdateHead`, including an 11.655-second fork-choice-lock
wait.  The slot-3 proposal (ID 146) spent 12.782 seconds in its own optimistic
check and 4.534 seconds in `getParentState`, including 3.533 seconds in that
method's `UpdateHead`; no nested lock-wait phase was captured for that update.
Other background `UpdateHead` requests are deliberately not assigned to a
proposal.  Handler timings may continue after the VC has cancelled, so these
totals must not be interpreted as the VC's RPC duration.

The periodic probe also observed isolated multi-second sync-index calls, but
its scheduler issued missed ticks in a burst after a blocked call.  Those late
samples are not independent fixed-rate observations, and probe calls may warm
the per-slot sync-head cache before a real VC call.

E1 proves a local causal path from the FFG validation cohort, through the
package-global async multilock manager, to an approximately 4.795-second
sync-preflight delay.  It does not reproduce the historical 12-second
preflight failures, establish that all historical sync failures spent their
time at this exact stage, reproduce round2 node169's slot-1 RANDAO anomaly, or
resolve the separate historical execution-client response-delivery question.

## F1 scan-ablation counterfactual

F1 repeated E1 with the same source and probe binaries, 15,000 offered RPCs
per slot, and early timing. The only BN behavioral difference was the explicit
diagnostic genesis-count ablation. Invalid attempts E and F offered no valid
load and are excluded entirely.

The source accepted all 45,000 RPCs with zero errors. First/last offsets were
-300/+1,599 ms in slot 1, -392/+1,600 ms in slot 2, and -391/+1,599 ms in
slot 3. BN3's six subnet counters totaled exactly 45,000 validation attempts
and 45,000 delivered messages, with zero validation-throttled rejects.

BN3 sampled 21.72 CPU-seconds, versus E1's 153.78. `ActiveValidatorCount`
fell from 140.28 seconds (91.22%) in E1 to 0.03 seconds (0.14%) in F1;
atomic-add samples fell from 73.52 seconds (47.81%) to 0.07 seconds (0.32%).
The largest sampled goroutine cohort fell from 7,574 total with 6,144 gossip
validation stacks to 376 total with 49 validation stacks.

All three blocks were produced approximately +0.42, +0.10, and +0.09 seconds
after slot start. `GetBeaconBlock` completed in 238.173, 49.517, and 47.880
ms; its parent-state phase took 95.382, 1.995, and 1.798 ms. Initial
`CachedHeadRoot` waits rounded to zero and the maximum instrumented
`UpdateHead` fork-choice lock wait was 0.042 ms.

Unlike E1, F1 never triggered the old probe's missed-tick catch-up behavior:
all 72 samples per method retained at least 400 ms spacing. All calls
succeeded; maximum `DomainData` latency was 6.282 ms and maximum
`GetSyncSubcommitteeIndex` latency was 6.358 ms. The observer remains a
low-rate extra client and can warm the same caches used by the VC, so these
figures are a controlled comparison rather than a zero-observer baseline.

Removing only the repeated genesis-state count scan therefore removed E1's
CPU saturation, validation cohort, sync-index delay, fork-choice contention,
and proposal failures under the same offered workload. This diagnostic
ablation establishes local causality; it is not a production fix.
