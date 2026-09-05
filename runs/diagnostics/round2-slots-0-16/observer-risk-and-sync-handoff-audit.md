# Observer risk and sync-manager handoff audit

The large warm-sync delay is reproduced without a goroutine snapshot, runtime
trace, or CPU profile: the matched 6,144-worker / 15,000-job scan arm has
13.448500- and 13.571680-second calls, versus an 11.833-millisecond memoized
maximum. All jobs finish and all stack-attempt flags are false. A subsequent
traced pair locates the delay in the shared multilock manager, both before the
warm cache lookup and during deferred cleanup. The traced durations and the
untraced durations are separate observations.

The 256-worker no-stack pair was negative despite active scans during all 32
probes. Its scan maximum was 28 microseconds, while the stack-enabled variant
had later calls around 545 milliseconds. Excluding only the snapshot-triggering
call cannot make the later calls unperturbed: a snapshot can change the queue
and scheduler history inherited by later calls. Conversely, one stochastic
negative repeat does not isolate the snapshot as the cause of the earlier
positive result. Arrival order and cohort size also matter. The observed
256-versus-6,144 comparison does not locate a sharp threshold or establish that
6,144 workers are necessary.

The paired raw logs and full experiment description are linked from
[sync-index-count-realwork-results.md](sync-index-count-realwork-results.md).

## E1's profile requests and the already-slow warm return

The retained collector at
`/tmp/prysm-startup-repro/runs/diagnostics/startup-repro/capture-startup-profiles.sh`
requests `/debug/pprof/goroutine` without a `debug` query. In the captured Go
1.26.5 source, `runtime/pprof.writeGoroutine` uses
`pprof_goroutineProfileWithLabels` for this binary/debug-0 request. The
`runtime.Stack(buf, true)` path is used for `debug >= 2` instead.

Binary goroutine profiling still perturbs execution. The runtime starts a
stop-the-world phase, restarts the world for cooperative goroutine collection,
and uses another stop-the-world phase for cleanup. Its HTTP request duration
includes transport and serving overhead; it is not a measurement of the
stop-the-world time. E1 also continuously collects a 50-second CPU profile.

The following timestamps come from E1's `captured_time` diagnostic fields and
`/tmp/prysm-startup3-round4-early-e1/profiles/timestamps.tsv`, in UTC on
2026-09-05:

| Event | Time |
| --- | --- |
| BN goroutine +1 profile completed | 14:03:59.066034100 |
| Request 16 cache put ended | 14:04:10.110778523 |
| Request 16 total ended | 14:04:10.357213826 |
| Request 21 warm cache hit ended | 14:04:10.444568039 |
| Request 23 lock wait began | 14:04:10.802186494 |
| BN goroutine +13 profile request started | 14:04:11.022338524 |
| BN goroutine +13 profile request completed | 14:04:11.062652452 |
| Request 21 total ended | 14:04:15.239637719 |
| Request 23 lock wait ended | 14:04:15.239692672 |

Request 16 has a complete 246.435303-millisecond post-cache-put interval before
the +13 profile request. Request 21 has already spent 577.770485 milliseconds
after its warm cache hit before that request begins. Thus the +13 capture
cannot have initiated those elapsed delays. Its full 4.795069680-second
post-hit interval overlaps the profile and its aftermath, so that entire
duration is not an observer-free measurement. The earlier +1 profile finished
well before the loaded interval, with fast warm calls in between; this is
useful timing evidence, not a proof that all earlier instrumentation had zero
effect.

The post-hit interval covers the hit counter increment, return, deferred
`Unlock`, and any intervening scheduling delay. E1's goroutine profile observes
the handler in `Clean` inside that interval. It does not measure all 4.795
seconds as uninterrupted `Clean` blocking. The phase logger captures time
before a nonblocking queue send and performs logging I/O in another goroutine;
the table uses capture time, not delayed log-print time.

Later E1 slow sync-probe intervals also overlap a profile request or its
aftermath. In particular, the +37 and +43 BN goroutine-profile HTTP requests
span approximately 1.999 and 3.609 seconds. Their full elapsed times should not
be presented as independent, unprofiled latency controls. The new large
no-stack/no-profile pair provides that control directly.

## H's payload windows and discrete profile timing

The H payload audit uses
`/tmp/prysm-startup3-wire-h/evidence/engine-runtime-filtered.jsonl` and
`/tmp/prysm-startup3-wire-h/results/profiles/timestamps.tsv`; I2 has the
corresponding profile timestamp file. None of H's three measured post-network
windows, including the long read-loop runnable wait through its first-byte
callback, overlaps a BN goroutine-profile request:

| Diagnostic ID | Post-network window through callback, UTC | Surrounding profile request boundaries |
| --- | --- | --- |
| 33 | 15:11:07.224399–15:11:07.549389 | +13 completed 15:11:04.074728; +19 started 15:11:10.019167 |
| 46 | 15:11:13.121271–15:11:13.803749 | +19 completed 15:11:10.060804; +25 started 15:11:16.021392 |
| 95 | 15:11:30.661705–15:11:31.074717 | +37 completed 15:11:28.060411; +43 started 15:11:34.019965 |

The nearest endpoint gap is 2.218 seconds. This excludes temporal coincidence
with a discrete goroutine-profile request as the explanation for those
windows. Both H and I2 continuously run CPU profiling and runtime tracing;
this comparison does not establish zero overhead from those facilities or
exclude every possible persistent effect of an earlier capture. The packet,
kernel-ACK, and runnable-state evidence and the BN3-only count ablation remain
the direct local causal evidence described in
[wire-causation-results.md](../startup3/wire-causation-results.md).

## Exact stages in the new traced sync pair

Call 2 is goroutine 4525 in
`/tmp/sync-index-count-scan-e1.trace`. Its region runs from trace time
260900890187584 to 260916434656512, totaling 15.544468928 seconds.
The retained
[probe transitions](sync-index-count-scan-e1-cohort-trace.transitions.txt)
give the following decomposition:

| Stage | Waiting | Runnable after wake |
| --- | ---: | ---: |
| `getChan`, global-manager acquisition before cache lookup | 11.017502336 s | 0.000000576 s |
| `Clean`, deferred return after the warm cache hit | 4.523096576 s | 0.003854720 s |

Only about 14.7 microseconds remain in the region outside those states. Both
wakeups come from checkpoint worker G10513 releasing the global manager, first
at `getChan.func1` and later at `Clean.func1`. The long sync delay is therefore
primarily channel waiting behind the manager cohort, not a 15-second runnable
delay of the sync goroutine itself. The cold state-transition path is absent
from both blocking stacks; the `Clean` stack points to the warm cache return.

The [compact handoff excerpt](sync-index-count-manager-handoff-evidence.txt)
also retains this consecutive local sequence:

| Trace time, ns | Observed event |
| ---: | --- |
| 260916427236224 | Checkpoint G10475 releases `Clean`, making checkpoint G10513 runnable |
| 260916430789632 | G10513 first runs, after 3.553408 ms runnable |
| 260916430791936 | G10513 releases `Clean`, making sync G4525 runnable |
| 260916434646656 | G4525 first runs, after 3.854720 ms runnable |
| 260916434648320 | G4525 releases `Clean`, making checkpoint G10512 runnable |
| 260916440435520 | Outgoing G10513 next blocks at checkpoint `getChan` |

The global manager is a capacity-one channel. In Go's buffered-channel receive
handoff, the waiting sender's value is copied into that channel before the
sender becomes runnable. Consequently the manager remains reserved while the
newly admitted goroutine waits to run; that goroutine must execute before it
can release the manager onward. The outgoing checkpoint caller can meanwhile
return and execute its count step. In this sequence G10513 keeps running for
9.643584 milliseconds after waking the sync caller before its next checkpoint
block. That interval includes the count by the harness's fixed source order;
an earlier preemption of the same G10513 also directly records
`ActiveValidatorCount -> ValidatorsReadOnlySeq -> multi-value-slice.At`.

This is a measured example of scheduler delay being serialized through manager
handoffs while outgoing callers perform count work. It does not assign the
entire 15.544 seconds to this one handoff, or transfer these exact goroutine
IDs and durations to the historical deployment.
