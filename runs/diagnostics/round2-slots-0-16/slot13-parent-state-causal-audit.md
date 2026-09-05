# Slot 13 parent-state causal audit

Slot 13's exact error locates cancellation at a cache/context boundary inside
slot preparation. It does not require the preceding 10.721 seconds to have
been spent executing slot transitions. A completed real-work control now
connects the count workload to that boundary: the parent dependency probe spent
11.074 seconds in `UpdateHead`, exhausted its 10.683-second cancellation budget,
and observed the exact inner cancellation in slot preparation 13 microseconds
later. The memoized-count arm prepared slot 13 in 35.377 milliseconds. A separate
optional trace directly measures a 6.848-second fork-choice write-lock wait
behind checkpoint readers queued on the checkpoint-key channel. The earlier
[sync handoff trace](observer-risk-and-sync-handoff-audit.md) documents the
additional global-manager path; these are distinct waits.

This is a source-supported explanation for historical node 20, not recovery
of its unlogged lock acquisition or cache-leader identity. The bounded real-work
control below exercises the public dependency chain without an injected blocked
head accessor or manually marked cache leader.

## Exact historical constraints

Node 20's archive is
`/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz`.

| Event | UTC on 2026-09-05 | Archive member / line |
| --- | --- | --- |
| BN build entry, slot 13 | 01:32:37.317155228 | `beacon.log:527` |
| Independent late-block FCU request | 01:32:42.084531003 | `snooper-engine.log:32149` |
| FCU response, payload ID `0x0487fed9122584fe` | 01:32:42.085875827 | `snooper-engine.log:32172–32180` |
| BN records that payload ID, `headSlot=0`, `nextSlot=13` | 01:32:46.342761692 | `beacon.log:530` |
| VC block-request deadline | 01:32:48.008413834 | `validator.log:1221` |
| BN parent-state error | 01:32:48.038076668 | `beacon.log:531` |

The terminal text is exactly:

```text
Could not process slots up to 13: could not process slots: context canceled
```

`GetBeaconBlock` obtains the parent state before allocating its empty block,
selecting the proposer, starting parallel consensus assembly, or requesting a
payload. Thus no `GetPayload` failure or packing delay is on this terminal
path. The 10.720921440-second entry-to-error interval also includes readiness
checks before parent preparation and `parentFull` after its error; it is not
an exact timer around `ProcessSlots`.

The owner records no successful block import before this failure. Its separate
FCU still reports genesis head slot zero. Neither observation identifies the
state returned by the next-slot cache: a genesis-root cache entry could already
have advanced beyond zero. The FCU completion does not prove cache priming to
13, because `refreshCaches` launches the post-Fulu advancement asynchronously.
Its post-response head read and fork-choice reads have no timestamped lower
bound after this proposal began; the asynchronous snooper timestamps cannot
supply one. The full FCU response-to-BN-log gap is not pure HTTP or scheduler
time. See [deeper-build.md](deeper-build.md) for that source-order audit.

Historical slot-13 attestations target round 1 under eight-slot rounds. A
round-1 checkpoint cached at slot 8 can use the active-validator-count cache.
A concurrent slot-zero scan cohort therefore represents lingering round-zero
work, not a claim that every new slot-13 vote scans the registry. The historical
owner lacks the per-call count and admission records needed to size that
remaining cohort.

## Which instruction returned the bare cancellation

`ProcessSlotsUsingNextSlotCache` adds `could not process slots` to the error
returned by `ProcessSlots`. The latter's unchanged source can propagate a bare
context error from either of its two `SkipSlotCache.Get` calls or from the
per-iteration `cacheBestBeaconStateOnErrFn` callback in `ProcessSlotsCore`.
The callback may cache partial progress before returning the context error.

Other processing errors have additional wrappers: `ProcessSlot` adds
`could not process slot`; round processing adds `could not process heze round`;
epoch, increment, and upgrade failures likewise carry their own text. Those
wrappers are absent historically. Consequently the final error was observed
at a cache wait/check or between iterations, rather than returned as a
state-hashing or round-transition error. This does not exclude earlier costly
hashing, a delayed prior transition, or upstream locking before that check.

`SkipSlotCache.Get` checks `ctx.Err()` before looking at its in-progress map.
An already-expired caller can therefore receive this error without having
waited for any cache leader. The existing controlled cancellation test proves
that boundary behavior; it does not establish the historical leader's presence.

## Real dependency chain from checkpoint/count work

The production sequence is:

```text
getParentState
  CachedHeadRoot             fork-choice RLock
  UpdateHead                 fork-choice Lock
  CachedHeadRoot             fork-choice RLock
  GetProposerHead             fork-choice RLock
  getParentStateFromReorgData
    NextSlotState            cache mutex, state Copy on hit
    HeadState on cache miss  head RLock through native state Copy
    ProcessSlotsUsingNextSlotCache
      NextSlotState          another cache lookup/copy
      ProcessSlots           skip-cache wait and real transitions
  parentFull                 head RLock; optional fork-choice RLock
```

The competing checkpoint sequence is:

```text
AttestationTargetState
  fork-choice RLock
  getAttPreState
    checkpoint multilock Lock
    cached checkpoint read
    deferred Unlock -> global Clean
  fork-choice RUnlock
ActiveValidatorCount         outside the fork-choice read lock
```

Existing admitted readers remain fork-choice holders while queued at either
multilock acquisition or cleanup. A pending `UpdateHead` writer must wait for
those readers to leave. Writer preference then prevents new readers from
bypassing it, also delaying proposal `CachedHeadRoot` calls that encounter a
different pending writer. None of these mutex waits is released by context
cancellation. The scan runs after read-lock release, but can delay the newly
admitted manager users that still have to run before the remaining readers
can exit. The new trace provides a concrete consecutive handoff example of
this mechanism.

The earlier real E1 proposal with diagnostic ID 83 spent 13.070 seconds in
its initial `CachedHeadRoot` and 11.655 seconds waiting for the fork-choice
write lock inside its own `UpdateHead`. E1's slot-1 proposal independently
waited 5.855 seconds in that lock. F1's matched count ablation reduced the
maximum instrumented `UpdateHead` lock wait to 0.042 milliseconds and parent
preparation to 95.382, 1.995, and 1.798 milliseconds. These were profiled local
experiments, not the historical node-20 trace; their timings retain the
observer qualification discussed in the
[observer audit](observer-risk-and-sync-handoff-audit.md).

There are secondary state-copy dependencies. `HeadState` retains `headLock`'s
read lock while copying; next-slot cache hits retain the cache mutex while
copying. Native `BeaconState.Copy` obtains a writer lock on the shared
validator multi-value slice, whereas count iteration repeatedly obtains its
reader lock. This creates a direct shared-lock intersection in addition to
CPU competition, but no historical node-20 copy wait is measured.

The relevant fork-choice, checkpoint, transition, and next-slot-cache source
files have no changes between revision
`0280403c70d88967f49d2d4c730f4c5417dabdf5` and the reviewed workspace.

## A separate cache inefficiency and correction to the older baseline account

At both cache checks, `ProcessSlots` adopts a cached state only when
`cachedState.Slot() < target`. An entry already at exactly target slot 13 is
ignored. A later same-key request can therefore repeat the transition from
its original parent even after a previous caller cached the desired result.
This is concrete source behavior, also exercised by the existing
`cache_warm_equal_target` benchmark. The cache key contains starting slot and
latest block-header root, not requested target slot.

The existing [genesis benchmark report](../genesis_bench_results.md) already
retains the exact-target result. For root-warmed parents and targets 1, 2,
and 3, three one-iteration samples took 89–154, 111–197, and 117–135
microseconds, allocating about 314, 345, and 362 KB. Warm parents with a cold
skip cache took 79–92, 101–152, and 124–240 microseconds and about 212,
244, and 261 KB. These earlier synthetic/ambient-fork measurements demonstrate
wasted copying/reprocessing; they are not new slot-13 measurements or a
multi-second historical explanation. The same report notes that in-progress
ownership can permit overlapping recalculations, so strict serialization of
every recomputation must not be assumed.

The comparison is longstanding and was intentional in the recorded history.
After following the 2021 package rename, revision
`cc741ed8af9551019c329a717d6b34066d532032` on 2020-02-01 changed the earlier
`<=` comparisons to `<` as part of the native-state migration (#4646). Its
commit narrative includes “Don't use cache for current slot (may not be the
right fix)” and “Align with prev logic for process slots cachedState.Slot() <
slot”. This is a historical compatibility rationale, not proof that the
inefficiency is required for current correctness. Existing
`TestSkipSlotCache_OK` and `TestSkipSlotCache_ConcurrentMixup` check state
equivalence and fork separation; they do not assert elimination of repeated
equal-target work. A production reuse change would require that correctness
review rather than merely flipping the inequality in this investigation.

The outer next-slot cache substantially limits when this inefficiency matters.
`getParentStateFromReorgData` immediately returns a state already at or beyond
target 13. Its later `ProcessSlotsUsingNextSlotCache` lookup also returns
immediately if the matching cached state is exactly at 13. An exact-target
next-slot-cache hit therefore avoids `ProcessSlots` and its strict comparison
entirely. The historical wrapped cancellation proves those fast returns were
not taken successfully on this attempt. It does not prove an exact-target
entry existed in the separate `SkipSlotCache`; that cache may have had no
entry, partial progress, another target, or an in-progress owner.

The earlier fanout benchmark copies the input before `ProcessSlots`, but its
large full-state hash does not occur before the skip-cache gate. `cacheKey`
hashes only the latest block header and starting slot; `ProcessSlot` performs
the full-state hash after cache arbitration. Thus the reported 11–12 GB of
aggregate allocations cannot be described as measured pre-gate state hashing.
Repeated equal-target work is a separate potential amplifier; an allocation
total alone does not assign bytes or time to either mechanism.

The retained real-genesis single transition to slot 13 took about 64 ms with
a cold Merkle tree on this host. It establishes a useful isolated baseline,
not the historical owner's cache state, cost under contention, or wait split.

## Completed public-dependency control

This control is now complete. The matched scan arm spent 11.074 seconds in the
real `UpdateHead(ctx, 13)` dependency while its fresh 10.683-second budget
expired, then observed `could not process slots: context canceled` at the next
real slot-cache boundary. The memoized arm prepared slot 13 in 35.377
milliseconds. See
[slot13-parent-dependency-realwork-results.md](slot13-parent-dependency-realwork-results.md)
for the full phase records, commands, and fixture checks. The primary logs are
[memoized](slot13-parent-dependency-memoized.log) and
[scan](slot13-parent-dependency-scan.log).

Both processes used `GOMAXPROCS=4`, a synthetic 120,000-active-validator Heze
state, eight-slot rounds, 6,144 workers, and 15,000 finite round-zero checkpoint
jobs. The repeated production count versus verified memoized count was the sole
workload arm change. The service used real `saveGenesisData`, a warm checkpoint
cache, regular-sync mode, and the concrete fork-choice store. The cohort is an
already exercised production-admissible size, not an estimate of node 20's
backlog at slot 13.

The corrected fixture includes the full inactivity-score vector. An uncanceled
preflight transition on a copy reached slot 13 in both processes before release;
the harness then replaced the skip-slot cache and rewarmed the committee cache.
The original invalid-fixture pilot is retained and excluded. This validates the
exercised transition path; it does not make the synthetic registry, which uses
duplicate zero public keys, the historical genesis SSZ or a fully validated
consensus fixture. All four recorded roots match each other and match between
arms, but they are not claimed to equal node 20's historical root.

The probe starts after the first completed checkpoint/count job. This is a
matched progress criterion, not an identical instantaneous reader cohort:

| Primary measurement | Memoized | Scan |
| --- | ---: | ---: |
| Jobs at first probe phase | 135 | 1 |
| Checkpoint calls active at first probe phase | 427 | 4,346 |
| Checkpoint calls active entering `UpdateHead` | 436 | 4,364 |
| `UpdateHead` elapsed | 25.640 ms | 11.073777 s |
| `HeadState` elapsed | 73 us | 71 us |
| Slot preparation elapsed/result | 9.640 ms, slot 13 | 13 us, context canceled |
| Whole probe elapsed | 35.377 ms | 11.073891 s |
| All finite work completed | 15,000 jobs in 53 ms | 15,000 jobs in 38.980 s |

The fast arm can complete more jobs before the newly launched probe is
scheduled. In the scan arm the first scan was already complete at probe entry,
while thousands of checkpoint calls remained active. Different queue formation
and dispatch timing are consequences of the matched workload and relevant to
whether its writer wait crosses the budget; count presence alone is not a
guarantee of cancellation.

The request budget is implemented with `context.WithCancel` and `time.AfterFunc`
scheduled for 10.683 seconds. This models cancellation arriving from a client
whose remaining slot budget expires and preserves the historical inner
`context canceled` identity. The primary logs prove cancellation occurred during
`UpdateHead`; they do not timestamp the callback itself or establish that it ran
exactly when due. The context was already canceled
when the subsequent `HeadState` copy and slot preparation began.

The next-slot cache missed and `HeadState` returned slot 0 in both arms.
The fresh skip-slot cache had no forced owner. Therefore this reproduction
reaches the first `SkipSlotCache.Get` with an already canceled context and
returns before any slot transition is needed. It neither depends on nor tests
the separate exact-target reuse inefficiency described above. `HeadRootAndFull`
still returns the same parent root with `full=true` after the error.

The test composes public service methods rather than calling private
`getParentState`; the existing RPC-level control supplies the outer
`Could not process slots up to 13:` wrapper transfer. The composition fixes the
`UpdateHead` proposing-slot argument to 13. Production passes
`TimeFetcher.CurrentSlot()` at invocation, which could advance during an earlier
head-root wait; the historical value is not recorded. Fork-choice acquisition
is required before that argument affects the body in either case.

## Optional trace: the actual writer wait and its final reader

The [traced scan log](slot13-parent-dependency-scan-trace.log) and
[focused transitions](slot13-parent-dependency-scan-trace.transitions.txt)
come from a separate run at the same fixed workload. It completed the parent
probe successfully in 6.895368 seconds, including `UpdateHead` in 6.870986
seconds and a real target-13 transition in 24.252 milliseconds. It did not
reproduce cancellation. Its smaller cohort at `UpdateHead` entry (3,071 active
checkpoint calls, versus 4,364 in the primary scan) also cautions against
assigning the run-to-run timing difference to tracing alone.

G4464's `slot13-parent-update-head` trace region spans 6.870981056 seconds:

| Trace segment | Duration |
| --- | ---: |
| Region entry to blocking at `UpdateHead:127` | 2.624 us |
| Waiting at the fork-choice `sync.RWMutex.Lock` | 6.847655424 s |
| Runnable after writer wake, before scheduling | 747.008 us |
| Elapsed from that scheduling to region end | 22.576 ms |

The write-lock wait is 99.660520% of the region. The final 22.576 ms is elapsed
time, not pure CPU time: it includes 32.704 us of preemption-related Waiting
and 108.608 us of additional Runnable intervals. The primary result remains a
whole-method timer; this separate trace is the direct instruction-level proof.

G6094 entered `AttestationTargetState` and held its fork-choice read lock while
waiting 6.844672640 seconds at `async.(*Lock).Lock`, line 44: the receive on the
**checkpoint-key channel**. G7865 woke it by returning the key token from
`Unlock`, line 61. G6094 remained Runnable for 2.975808 ms, then ran and released
the fork-choice read lock 7.680 us later. That `RUnlock` directly woke G4464.
The runtime's `RWMutex` source identifies this wake as the last outstanding
reader releasing the writer; G4464 was waiting for readers, not another writer.

The preceding handoff shows the same local mechanism. G7865 had also waited on
the checkpoint-key receive; G8044's `Unlock:61` woke it, followed by 1.553280 ms
Runnable before it could run. Once scheduled, G7865 returned the key token to
G6094 in 5.760 us, then continued Running for 7.386880 ms before its next
`AttestationTargetState` blocked acquiring the fork-choice read lock. By the
fixture's program order, its real count step executes between those checkpoint
calls. This interval includes cleanup and worker bookkeeping as well as count
work; it is not an isolated count timer.

These are key-token handoffs, not measured global-manager waits. In source,
`getChan` acquires the manager during `Lock` before receiving the key token;
`Unlock` returns the key token **before** global `Clean`, and the caller releases
the fork-choice read lock only after that cleanup. The prior sync trace proves
the separate global-manager path. This parent trace directly establishes the
checkpoint-key queue, delayed scheduling of its admitted callers, retained
fork-choice readers, and the resulting writer wake. Neither trace identifies
node 20's historical holder or partitions its unlogged 10.721-second interval.
