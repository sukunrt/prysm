# Slot 13 parent dependency chain under real checkpoint/count work

## Result

The bounded production-method control reproduced the slot-13 cancellation
boundary without a blocked mock, held application lock, forced cache owner,
sleep, stack capture, profile, or runtime trace. The scan arm spent 11.074
seconds in the real `UpdateHead(ctx, 13)` phase. Its fresh 10.683-second request
budget expired during that call. The following real
`ProcessSlotsUsingNextSlotCache` call then returned
`could not process slots: context canceled` in 13 microseconds.

The matched memoized arm completed the whole parent dependency probe in 35.377
milliseconds and prepared a real state at slot 13. The scan arm's whole probe
took 11.074 seconds and returned the cancellation instead. Both arms drained
all 15,000 finite jobs and retained the verified active-validator count of
120,000.

This test is named a public parent-dependency-chain composition. It does not
claim to call the validator RPC package's private `getParentState` method. The
existing RPC-level cancellation control establishes that the private method
adds `Could not process slots up to 13:` around this transition error.

## Matched setup

Each arm ran in a separate process with `GOMAXPROCS=4`, 6,144 workers, 15,000
jobs, 120,000 validators, and historical eight-slot rounds. Every worker called
the production `AttestationTargetState(cp0)` method before its count step. The
sole arm change was:

- `memoized`: return the previously verified value 120,000.
- `scan`: call production `helpers.ActiveValidatorCount(st, 0)`.

The probe was dispatched by the first completed checkpoint/count job. At that
event the harness created a context with `context.WithCancel` and scheduled
`cancel` with `time.AfterFunc(10_683*time.Millisecond, cancel)`. This diagnostic
budget models cancellation propagated from the validator client; it is not a
context deadline. The primary log records that the context was canceled when
`UpdateHead` returned, but the primary harness version did not timestamp the
timer callback itself.
Inside the probe goroutine it executed and timed:

```text
Service.CachedHeadRoot
Service.UpdateHead(ctx, 13)
Service.CachedHeadRoot
Service.GetProposerHead
transition.NextSlotState(parentRoot, 13)
Service.HeadState                         on cache miss
transition.ProcessSlotsUsingNextSlotCache(parentRoot, 13)
Service.HeadRootAndFull
```

Regular-sync mode was true and the configured store was the real
`*doublylinkedtree.ForkChoice`, so `UpdateHead` followed its production path to
the store's real write lock. An uncanceled preflight transition, performed
before the workload release and followed by a fresh skip-slot cache, prepared
slot 13 successfully in both processes. This rejects an invalid state fixture
as the source of the measured result.

## Phase comparison

| Measurement | Memoized | Scan |
| --- | ---: | ---: |
| Probe elapsed | 35.377 ms | 11.073891 s |
| `UpdateHead(ctx, 13)` | 25.640 ms | 11.073777 s |
| `HeadState` copy | 73 us | 71 us |
| `ProcessSlotsUsingNextSlotCache` | 9.640 ms, slot 13 | 13 us, context canceled |
| Jobs at probe entry | 135 | 1 |
| Checkpoint calls active at probe entry | 427 | 4,346 |
| Scans active at probe entry | 0 | 0 |
| Jobs at `UpdateHead` exit | 785 | 4,364 |
| Checkpoint calls active at `UpdateHead` exit | 6,144 | 6,141 |
| Scans active at `UpdateHead` exit | 0 | 3 |
| Finite work elapsed | 53 ms | 38.980 s |
| Whole-process user CPU | 2.41 s | 156.89 s |
| Jobs completed | 15,000 | 15,000 |

The trigger is the same first-completion event in both arms. The memoized
workers can finish additional jobs during goroutine dispatch, which accounts
for the different job counters at probe entry. In the scan arm the first scan
had completed when the probe began, while 4,346 checkpoint calls were already
active. By the end of `UpdateHead`, three validator scans were active and the
checkpoint cohort still occupied 6,141 workers. Every phase record contains
both entry and exit counters.

All four observed roots were the same genesis block root in both arms:
`0xd873b444f4cdc7dc06cd5e00742625e3976bab15a5da44f495706d6f660b0b7f`.
The next-slot cache missed, `HeadState` returned slot 0, and
`HeadRootAndFull` returned the same parent root with `full=true`. This preserves
the head, parent, state, and fork-choice coherence needed for the no-reorg path.

The primary pair localizes the differential to the whole public `UpdateHead`
call; it deliberately does not instrument the method's internal lock wait.
Source order and the asserted regular-sync/concrete-store conditions show that
the call reaches the real fork-choice lock, while the earlier manager-handoff
trace supplies separate internal queue evidence. The control cannot recover
the historical node's unlogged lock owner or cache contents, and it does not
show that every loaded run must cancel.

## Optional runtime-trace localization

A separate scan-only repetition enabled the harness's optional runtime trace
after fixture preparation and before the common worker release. It enclosed
the whole parent probe and `UpdateHead` in named regions. This repetition is
kept separate from the untraced primary comparison because tracing changes
scheduler behavior and added observer work.

The traced `UpdateHead` region took 6.870981056 seconds. Parent probe G4464
entered `sync.(*RWMutex).Lock` from production `Service.UpdateHead` and remained
waiting there for 6.847655424 seconds, or 99.660520% of the region. After the
writer became runnable it waited 747.008 microseconds to run; the remaining
elapsed interval from that first post-lock scheduling through the region end
was 22.576 milliseconds. That remainder contained 22.434688 milliseconds
running, 32.704 microseconds waiting after later preemptions, and 108.608
microseconds runnable.

The wake event connects the lock wait to an actual checkpoint reader. G6094
was inside `AttestationTargetState`, retaining the fork-choice RLock while it
waited 6.844672640 seconds in `async.(*Lock).Lock` for the checkpoint-key
channel. Checkpoint worker G7865 handed that key channel to G6094 from
`async.(*Lock).Unlock`; when G6094 ran and returned, its production
`RWMutex.RUnlock` made the `UpdateHead` writer runnable. The focused parsed
events are retained in
`slot13-parent-dependency-scan-trace.transitions.txt`.

This direct sample is the checkpoint-key receive at `async.Lock:44`, not a
global `getChan` or `Clean` wait. The earlier sync trace separately records the
global-manager handoff mechanism; the two observations should not be merged
into one sampled wait.

This traced repetition completed the parent probe in 6.895 seconds, including
a successful 24.252-millisecond transition to slot 13. Its diagnostic
10.683-second cancellation timer therefore never fired, and no normalized
timer-cancel event exists for this run. It is positive phase attribution and a
negative cancellation repetition. The untraced scan remains the primary
cancellation result.

Both scan runs used the first completed job as their trigger, but that progress
criterion does not make their scheduler and lock queues identical. The primary
scan entered `UpdateHead` with 4,364 checkpoint calls active, versus 3,071 in
the traced repetition. This queue-state difference and trace overhead are
material reasons the traced call acquired the writer lock before its budget.

## Commands and raw evidence

Compilation used Go rather than Bazel, as required for this diagnostic:

```sh
GOCACHE=/tmp/prysm-diagnostic-buildcache GOMAXPROCS=4 \
  go test -p=2 -tags=develop -c \
  -o /tmp/blockchain-parent-dependency.test ./beacon-chain/blockchain
```

Each primary arm used this command with `ARM` set to `memoized` or `scan` and
redirected all output before process launch:

```sh
{ time -p env PRYSM_DIAGNOSTIC_PARENT_COUNT_ARM="$ARM" GOMAXPROCS=4 \
  /tmp/blockchain-parent-dependency.test \
  -test.run '^TestDiagnosticSlot13ParentDependencyChainUnderCheckpointCountLoad$' \
  -test.v -test.count=1; } > "$LOG" 2>&1
```

The primary raw logs are
`slot13-parent-dependency-memoized.log` and
`slot13-parent-dependency-scan.log`. Their SHA-256 hashes are:

```text
0b361db52fd04134f8781046614bda20507252fccf5863e7206e10efa08188ce  memoized
5756e97a258f1ef0f2f9332d2890849ce6985cff0dead7d54e92a0930267b7fd  scan
```

The optional trace repetition is in
`slot13-parent-dependency-scan-trace.log`. Its log and raw trace hashes are:

```text
67ff5e32831fb19e69534c2baad89d083f6b429cb46602733964481bff0a134f  scan trace log
51a1f5ce0c964f23c06f1706c86437f762ff7168921df09fe3df1b24bda8f165  /tmp/slot13-parent-dependency-scan.trace
0164502e95c780a1e97eaa1293d4c293904e57c4a1071618a05c93f72239d2f9  focused transitions
```

The retained `slot13-parent-dependency-memoized-pilot-invalid-fixture.log` is
excluded from the comparison. It exposed a missing 120,000-entry inactivity
score vector before the corrected preflight was added; its non-context state
transition error is why the final test rejects every error except context
cancellation and requires the memoized arm to reach slot 13.
