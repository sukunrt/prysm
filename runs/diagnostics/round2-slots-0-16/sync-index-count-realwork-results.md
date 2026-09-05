# Warm sync-index under real checkpoint/count work

## Conclusion

The production-bound, no-stack differential reproduced multi-second warm
sync-index delays without changing production code or inserting a delay. With
the 6,144-worker cohort measured in E1 and 15,000 finite jobs, the real scan
arm produced consecutive warm `HeadSyncCommitteeIndices` handler durations of
4.690, 13.449, 13.572, and 1.157 seconds. The matched memoized arm's maximum
was 11.833 milliseconds. No `runtime.Stack` call, profile, sleep, or artificial
production-lock hold ran in either arm.

The count arm performed actual `helpers.ActiveValidatorCount` scans over the
same read-only 120,000-validator slot-0 state. Its work phase took 32.872
seconds and the process used 132.40 user CPU-seconds. The memoized arm completed
the same 15,000 cached checkpoint calls in 27 milliseconds and the process used
2.34 user CPU-seconds.

A smaller 256-worker no-stack pair was negative: the scan maximum was 28
microseconds despite 8.730 seconds of scan work. Together, the smaller negative
pair and larger positive pair show load and arrival-order sensitivity. They do
not locate a universal threshold or prove that exactly 6,144 workers are
necessary.

The primary E1-cohort run measures whole handler latency with stack capture and
runtime tracing disabled. A separate repetition traced the same cohort and
directly located multi-second waits in the global async lock manager on both
entry to the warm cache path and its deferred cleanup. Independently audited
call 2 spent 11.018 seconds blocked in `getChan`, 4.523 seconds blocked in
deferred `Clean`, and 3.855 milliseconds runnable after its final wake. Runtime
tracing can add overhead, so the untraced and traced durations are reported
separately: the former supplies the instrumentation-free latency
counterfactual and the latter supplies phase attribution. The smaller
256-worker stack sensitivity history remains below as supporting path evidence
rather than a duration measurement.

## Production-path fidelity

Every worker called production `AttestationTargetState` on the same cached
checkpoint before its count step. Each job therefore exercised fork-choice
`RLock`, the checkpoint-key multilock, the cache hit, and deferred
`Unlock -> Clean`. `AttestationTargetState` returned before the count began, so
no fork-choice read lock was held through a validator scan.

The only arm difference was the count step:

- `memoized` used the previously verified true value, 120,000.
- `scan` called production `helpers.ActiveValidatorCount(st, 0)` on the
  read-only slot-0 state. The slot-0 condition bypasses the committee count
  cache and scans all 120,000 validators.

The real slot-1 sync head state was populated and verified in the one-entry
cache before work began. One sync call joined the workers' common release wave;
the remaining calls were sampled at completed-job thresholds. This introduced
no injected handler delay or production-lock hold. Both arms in a pair used the
same fixture, worker count, job count, 32 sync calls, progress scheme, and
`GOMAXPROCS=4`.

The fixture creates the validators directly and performs no BLS key generation.
Validator 0 is placed in the current sync committee so both the initial linear
lookup and the warmed sync-position cache return a real index.

## E1-cohort no-stack result

The snapshot threshold was set to 600,000 milliseconds, beyond the test
duration. Both raw summaries report zero stack attempts and captures.

| Measurement | Memoized | Scan |
| --- | ---: | ---: |
| Workers / finite jobs | 6,144 / 15,000 | 6,144 / 15,000 |
| Completed jobs | 15,000 | 15,000 |
| Work-phase wall time | 27 ms | 32,872 ms |
| Warm sync p50 | 2 us | 2 us |
| Warm sync p95 | 7,287 us | 13,448,500 us |
| Warm sync maximum | 11,833 us | 13,571,680 us |
| Probes before all jobs completed | 4 / 32 | 32 / 32 |
| Probes with a count scan active | 0 / 32 | 32 / 32 |
| Stack attempts / captures | 0 / 0 | 0 / 0 |
| Whole-process user CPU | 2.34 s | 132.40 s |
| Whole-process system CPU | 0.07 s | 0.14 s |

The four scan calls that covered the finite work were:

| Call | Handler duration | Jobs at start -> end | Active scans at start -> end |
| ---: | ---: | ---: | ---: |
| 1 | 4.689694 s | 0 -> 2,198 | 1 -> 3 |
| 2 | 13.448500 s | 2,198 -> 8,339 | 3 -> 3 |
| 3 | 13.571680 s | 8,339 -> 14,480 | 3 -> 3 |
| 4 | 1.157274 s | 14,480 -> 14,997 | 3 -> 3 |

Each duration was measured inside the goroutine, immediately around the
production handler call. The observer durations exceeded them by only 7 to
1,493 microseconds. Calls 5 through 32 began with 14,997 of 15,000 jobs
complete and returned in 1 to 10 microseconds while the last three scans ran.
This tail makes the result more specific: active scans alone were insufficient;
the multi-second interval coincided with the continuing checkpoint/waiter
cohort.

Whole-process CPU includes the fixture and warmup. Fixture wall time was 2.130
seconds in the memoized process and 2.155 seconds in the scan process. The
matched user-CPU difference was 130.06 seconds. The runtime user-CPU metric did
not refresh during either work phase and returned zero, so it is unavailable
rather than evidence of zero CPU use.

The scan p50 is low because 28 tail calls ran after virtually all finite work
had completed. The p95 and explicit first-four sequence describe the loaded
phase more clearly.

## Runtime-trace phase localization

An optional runtime trace was started after fixture warmup and before the
common work release. Each production sync handler was enclosed in a named
`sync-index-probe-N` region. The same 6,144-worker / 15,000-job pair was then
repeated with stack capture disabled.

The traced run reproduced the same loaded shape. Its scan calls took 4.451,
15.544, 15.840, and 2.361 seconds; the memoized maximum was 16.322 ms. Runtime
tracing can add overhead, so these durations are reported separately. The
earlier untraced pair had already established that tracing was not required to
produce multi-second delays.

The parsed trace locates the waits precisely:

Independently audited call 2's 15.544469-second region
spent 11.017502 seconds blocked in `getChan` before the warmed cache access,
4.523097 seconds blocked in deferred `Clean` after the cache hit, and 3.855
milliseconds runnable after its final wake. The separate untraced scan run's
13.571680-second maximum establishes that the large delay does not depend on
runtime tracing.

| Call / trace goroutine | `getChan` global-registry wait | Deferred `Clean` wait | Runnable delay after final wake | Region total |
| --- | ---: | ---: | ---: | ---: |
| 1 / G10681 | 30.394 ms | 4,419.983 ms | 0.499 ms | 4,450.908 ms |
| 2 / G4525 | 11,017.502 ms | 4,523.097 ms | 3.855 ms | 15,544.469 ms |
| 3 / G4458 | 11,194.604 ms | 4,645.477 ms | 0.120 ms | 15,840.218 ms |
| 4 / G4459 | 2,354.698 ms | no blocking transition | 6.395 ms | 2,361.114 ms |

For calls 1 through 3, the first `Running -> Waiting`, reason `chan send`, has
this path:

```text
async.getChan (multilock.go:116)
async.(*Lock).Lock (multilock.go:43)
Service.getSyncCommitteeHeadState (head_sync_committee_info.go:139)
HeadSyncCommitteeIndices (head_sync_committee_info.go:67)
```

After that wake, each handler reaches the verified cache-hit return and its
second blocking transition has this path:

```text
async.Clean (multilock.go:94)
async.(*Lock).Unlock (multilock.go:66)
Service.getSyncCommitteeHeadState (head_sync_committee_info.go:148)
HeadSyncCommitteeIndices (head_sync_committee_info.go:67)
```

The wake stacks identify production checkpoint callers releasing the same
global registry token from `getChan.func1` and `Clean.func1`. Thus the traced
delays are manager waits on both entry and deferred warm-cache return, rather
than long runnable starvation. Call 4 blocked only on entry; after its wake it
had 6.395 ms of runnable delay and returned without a blocking `Clean`
transition.

For call 2, the concrete handoff near the end is checkpoint caller G10513
waking from its own deferred `Clean`, becoming runnable for 3.553 ms, running,
and releasing the manager to sync probe G4525. The sync probe then became
runnable for 3.855 ms before running and returning the manager to another
checkpoint caller. An earlier transition for the same G10513 has an exact
`ActiveValidatorCount` preemption stack, tying the handoff to one of the real
checkpoint-then-count workers without claiming that the scan retained the
manager token.

Calls 2 and 3 each span 6,141 completed jobs in both the traced and untraced
cohort runs. That is useful cohort-cycle evidence, but completed-job counts and
endpoint scan counters do not identify an individual token owner. The trace
transitions supply the phase attribution for the traced repetition.

Raw traces remain in `/tmp/sync-index-count-{memoized,scan}-e1.trace`. Filtered
region, blocking, wake, and runnable transitions are tracked beside the test
logs. Their SHA-256 hashes are:

```text
deddf2bf963b67320ed1d0934030a82a4d4a415f6d1e236dbb16215b774a9132  memoized
c9798eaedf995e3e21245a854e546eed5a46a21a39cde53021be101f75f3b867  scan
```

## Bounded no-stack sensitivity control

The otherwise identical 256-worker / 4,096-job pair was negative:

| Measurement | Memoized | Scan |
| --- | ---: | ---: |
| Work-phase wall time | 6 ms | 8,730 ms |
| Warm sync p50 | 22 us | 18 us |
| Warm sync p95 | 549 us | 25 us |
| Warm sync maximum | 565 us | 28 us |
| Whole-process user CPU | 2.28 s | 36.85 s |

All 32 scan probes began with active scans and were distributed through
completed-job thresholds from 0 to 3,968 jobs. Active scans were therefore not
sufficient to delay the warm handler in this bounded run. The contrast with
the production-bound pair demonstrates load and arrival-order sensitivity; it
does not identify a universal concurrency threshold.

## Exact stack sensitivity run

A 256-worker run with a 25-millisecond slow-call stack trigger reported scan
p50 285.492 ms, p95 542.744 ms, and maximum 545.171 ms versus a 552 us
memoized maximum. Its first slow call triggered one `runtime.Stack(all=true)`
snapshot, which found:

```text
checkpoint_clean_waiters=243 checkpoint_lock_waiters=10 active_count_stacks=3 sync_stacks=1

goroutine 4775 [chan send]:
github.com/OffchainLabs/prysm/v7/async.Clean()
    async/multilock.go:94
github.com/OffchainLabs/prysm/v7/async.(*Lock).Unlock(...)
    async/multilock.go:66
github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain.(*Service).getSyncCommitteeHeadState(...)
    beacon-chain/blockchain/head_sync_committee_info.go:148
github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain.(*Service).HeadSyncCommitteeIndices(...)
    beacon-chain/blockchain/head_sync_committee_info.go:67
```

Line 148 is the warm-cache return. This sampled handler was blocked in deferred
`Unlock -> Clean`. The same snapshot retains a representative production
`AttestationTargetState -> getAttPreState -> Unlock -> Clean` waiter.

Call 1 took 347.088 ms and was marked as snapshot-perturbed. Calls 2 through 16
took 520.981 to 545.171 ms without further snapshots. Excluding call 1 alone
does not make those later samples clean: the all-goroutine snapshot can alter
the shared channel-lock queue and scheduler history inherited by later calls.
The smaller no-stack maximum of 28 us makes this run's latency attribution
confounded. It remains useful because the exact stack independently verifies
the warm deferred-clean route under a real checkpoint cohort.

## Launch-order observations and limits

Two exploratory 256-worker variants were also negative:

- Immediate post-release sampling issued all 32 scan probes while
  `jobs_completed=0`, before the first scan completed and before workers
  re-entered the checkpoint stage. Its scan maximum was 118 us despite 10.195
  seconds of count work.
- Sampling only at completed-job thresholds, without a sync call in the common
  release wave, produced a 24 us scan maximum versus a 383 us memoized maximum.

These are evidence of launch-order sensitivity. The production-bound result
uses the real 6,144-worker cohort measured in E1, rather than escalating until
an arbitrary failure appeared.

This remains an in-process production-method diagnostic, not a devnet or
network-RPC reproduction. It reproduces E1-scale warm-handler delay and exceeds
a 12-second request budget locally, but it does not prove that every historical
owner spent that time in this stage or that every delayed no-stack call spent
its entire duration in `Clean`. It proposes no consensus or production change.

## Commands and tracked raw evidence

The harness is
`beacon-chain/blockchain/sync_index_count_load_diagnostic_test.go`.

Compilation passed with:

```sh
GOCACHE=/tmp/prysm-diagnostic-buildcache GOMAXPROCS=4 \
  go test -p=2 -tags=develop -c \
  -o /tmp/blockchain-sync-index-count.test ./beacon-chain/blockchain
```

The initial `-tags=develop,minimal` compile was unsuitable for this package:
pre-existing `process_block_test.go` literals use `Bitvector512`, whereas the
minimal build selects `Bitvector16`. The full-config `develop` compile passed.

The E1-cohort memoized command was:

```sh
{ time -p env \
  PRYSM_DIAGNOSTIC_SYNC_COUNT_ARM=memoized \
  PRYSM_DIAGNOSTIC_SYNC_COUNT_WORKERS=6144 \
  PRYSM_DIAGNOSTIC_SYNC_COUNT_JOBS=15000 \
  PRYSM_DIAGNOSTIC_SYNC_PROBES=32 \
  PRYSM_DIAGNOSTIC_SYNC_STACK_AFTER_MS=600000 \
  GOMAXPROCS=4 /tmp/blockchain-sync-index-count.test \
  -test.run '^TestDiagnosticWarmSyncIndexUnderCheckpointCountLoad$' \
  -test.v -test.count=1; } \
  > runs/diagnostics/round2-slots-0-16/sync-index-count-memoized-e1-cohort-no-stack.log 2>&1
```

The scan arm used the identical command with arm `scan` and output path
`sync-index-count-scan-e1-cohort-no-stack.log`.

The traced pair used the same commands and inputs, adding one arm-specific
environment variable before launching each process:

```sh
PRYSM_DIAGNOSTIC_SYNC_TRACE=/tmp/sync-index-count-memoized-e1.trace
PRYSM_DIAGNOSTIC_SYNC_TRACE=/tmp/sync-index-count-scan-e1.trace
```

All eight logs below are explicitly tracked despite the repository's default log
ignore:

- `sync-index-count-memoized-e1-cohort-no-stack.log` and
  `sync-index-count-scan-e1-cohort-no-stack.log`: primary production-bound
  clean pair.
- `sync-index-count-memoized-no-stack.log` and
  `sync-index-count-scan-no-stack.log`: bounded no-stack sensitivity control.
- `sync-index-count-memoized-final.log` and
  `sync-index-count-scan-final.log`: stack-enabled sensitivity pair, including
  the exact bounded stack and all per-call events.
- `sync-index-count-memoized-e1-cohort-trace.log` and
  `sync-index-count-scan-e1-cohort-trace.log`: complete runtime-traced
  production-cohort test output.

The tracked parsed subsets are
`sync-index-count-{memoized,scan}-e1-cohort-trace.transitions.txt`. The focused
call-2 worker and manager handoff is retained in
`sync-index-count-manager-handoff-evidence.txt`. The full scan parse is
`/tmp/sync-index-count-scan-e1.trace.txt`; it is about 501 MB and is not tracked.
It was generated with the matching Go 1.26.5 toolchain:

```sh
/home/sukun/dev/go/bin/go tool trace -d=parsed \
  /tmp/sync-index-count-scan-e1.trace \
  > /tmp/sync-index-count-scan-e1.trace.txt
```
