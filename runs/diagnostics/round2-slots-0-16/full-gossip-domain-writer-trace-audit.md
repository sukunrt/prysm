# Independent audit of the full-gossip writer trace

The separately traced repeat localizes an 850.217 ms successful ordinary
`DomainData` request to scheduling delays in the gRPC transport. The HTTP/2
reader was runnable during most of the interval before handler admission.
After the complete server RPC goroutine exited, its response writer spent
563.455 ms runnable before it could flush the response. The actual socket write
took 56.576 microseconds.

This is the native **trace repeat**, not a trace of the primary 6.856-second
request. The repeat had 110 peak active validator iterators, versus 1,324 in the
primary. Its runtime trace is an observer and the two runs' exact timings must
remain separate. Neither request exceeded its absolute slot-two deadline.

## Inputs and clock alignment

All trace-repeat records are under
[`full-gossip-domain-writer-go-evidence/trace-shared`](full-gossip-domain-writer-go-evidence/trace-shared):
`client.jsonl` line 15, `server.jsonl` line 10, and `summary.json` identify probe
14. It was the only outstanding DomainData call, on the warmed child-process
connection. The one initial GetAttestationData call had returned at slot-one
+0.062200 seconds. Probe 12 returned at +3.348531 seconds, probe 13 was skipped,
probe 14 ran at +3.500929 seconds, and the next issued call was probe 18 at
+4.500685 seconds. No other application stream can explain an intervening
server handler during probe 14.

The raw trace SHA-256 is
`5e67d6e68ba55d668f4fa2b9a381d7b4c5fb2474e3f2741b4b0fdf6b123ad0cf`.
The locally retained Go 1.26 parsed expansion SHA-256 is
`5d1a50b2a1cc081fb6035e528bca931d20b7506e14563b1e896cbc3e9b22c6e4`.
It contains no `StackSample` events; no CPU profile was running.

Native synchronization event N=3 is at original parsed line 2,953,476:

```text
Trace=277056580818624
Wall=2026-09-06T20:15:37.454275621+05:30
wall_unix_ns - trace_ns = 1788428880873456997
```

The neighboring N=4 mapping differs by only 62 ns. Using N=3, the recorded
client and server boundaries map to:

| Boundary | Trace-clock nanoseconds | Slot-one offset |
|---|---:|---:|
| Client invocation | 277057627472301 | +3.500929298 s |
| Recorded server admission | 277057913846048 | +3.787303045 s |
| Recorded handler return | 277057913851388 | +3.787308385 s |
| Client return | 277058477689297 | +4.351146294 s |

The monotonic RPC duration is 850.216986 ms; subtracting wall-clock timestamps
gives 850.216996 ms. That 10 ns clock-reading difference is immaterial to the
millisecond waits below.

## Reader, handler, and response-writer chain

These are native state-transition events, not durations inferred from a stack
snapshot. The compact event selection in
[`probe14-focused-transitions.txt`](full-gossip-domain-writer-go-evidence/trace-shared/probe14-focused-transitions.txt)
preserves original parsed line ranges and complete stacks. Its extraction
script is [`extract_full_gossip_trace_window.py`](extract_full_gossip_trace_window.py),
using `--compact`. The complete wider selection and parsed expansion remain
local rather than tracked. The joined phase table is
[`full-gossip-domain-writer-trace-probe14.tsv`](full-gossip-domain-writer-trace-probe14.tsv).

| Goroutine and event | Trace-clock nanoseconds | Original parsed line |
|---|---:|---:|
| Reader G4914: Waiting → Runnable | 277057483265664 | 3100276 |
| Reader G4914: Runnable → Running | 277057913732096 | 3119488 |
| Reader G4914 wakes writer G4912 | 277057913756928 | 3119559 |
| Reader G4914 creates handler G15522 | 277057913805696 | 3119577 |
| Handler G15522: Runnable → Running | 277057913817728 | 3119717 |
| Handler G15522: Running → NotExist | 277057913865856 | 3119718 |
| Writer G4912: Runnable → Running | 277058141273856 | 3139570 |
| Writer G4912: Running → Runnable, `runtime.Gosched` | 277058141410688 | 3139571 |
| Writer G4912: Runnable → Running | 277058477457408 | 3543636 |
| Writer G4912: Running → Syscall, socket write | 277058477466176 | 3543637 |
| Writer G4912: Syscall → Running | 277058477522752 | 3543682 |

All G4914 and G4912 transitions in the selected window are retained, including
otherwise uneventful state bookkeeping. There is no intervening execution of
the reader during its 430.466432 ms runnable interval. That interval overlaps
286.259795 ms of the measured RPC before the reader runs. Its wake occurs
144.206637 ms **before** this RPC's invocation. Consequently, this is not a
packet-arrival timestamp for probe 14, and the overlap must not be described
as an exact measurement of how long this request's packet waited in the
socket. It does establish that the connection reader could not service the
connection during that runnable interval.

The reader creates G15522 through
`http2Server.operateHeaders → Server.serveStreams.func2`. This is the sole
stream-handler creation in the call window. G15522 starts 12.032 microseconds
after creation, runs continuously for 48.128 microseconds, and exits. Both
recorded server boundaries fall inside that lifetime. The single connection,
sequential client calls, unique handler creation, and matching timestamps
jointly identify it as probe 14. There is no explicit DomainData stack sample
or stream-ID annotation in this repeat, so this is a uniquely matched ancestry
and timing join, not a claimed direct method-name trace annotation.

The entire G15522 lifetime also bounds the diagnostic stats reads and record
append in this repeat. They cannot explain its subsequent 563 ms response
delay. This additional bound is unavailable for the untraced primary run.

## Why the completed response was not flushed promptly

The pinned dependency is `google.golang.org/grpc@v1.81.1`. On the successful
unary path, `server.go:1430` invokes the handler, `server.go:1477` sends the
response, and `server.go:1538` writes the OK status. `Server.sendResponse`
encodes the reply and calls `ServerStream.Write`; this delegates to
`http2Server.write` (`internal/transport/http2_server.go:1142`), which enqueues
a `dataFrame` through `controlBuf.put` at line 1165. Status/trailer emission
likewise enqueues a header frame through `finishStream` at line 1340.

This successful response-enqueue path is established by source and the matched
handler's completion. There is no separate queue-insertion trace marker.
In particular, the trace's first writer wake is **not** attributed to the
handler: its actual stack is reader G4914 → `handleWindowUpdate` →
`controlBuffer.put`. The writer was already runnable when G15522 generated its
reply, so enqueuing that reply did not require another observable wake.

Writer G4912 first waits 227.516928 ms after becoming runnable. By the time it
runs, G15522 has been finished for 227.408000 ms. It then explicitly yields
inside `loopyWriter.run`, waits another 336.046720 ms runnable, resumes, and
flushes the socket. The production batching condition is exact:

```go
// internal/transport/controlbuf.go:563
const minBatchSize = 1000

// internal/transport/controlbuf.go:630–637, inside the drain loop
if gosched {
    gosched = false
    if l.framer.writer.offset < minBatchSize {
        runtime.Gosched()
        continue hasdata
    }
}
l.framer.writer.Flush()
```

The native yield event contains `loopyWriter.run` at line 633. The later write
event contains `loopyWriter.run:637 → bufWriter.Flush → net.conn.Write →
syscall.write`. Thus the small-batch optimization itself creates the second
scheduling dependency before flushing. It does not ask for a 336 ms delay;
that is how long this runnable goroutine waited to execute again under this
load.

After the handler has completely exited, the two writer runnable waits total
563.454720 ms. The socket syscall lasts only 56.576 microseconds and the child
records its successful response 166.545 microseconds after the syscall returns.
The response-side chain therefore directly localizes this repeat's dominant
post-handler delay to the server writer's scheduling, with no need to infer a
slow handler, expensive domain computation, or slow socket write.

## What this establishes and what remains bounded

The native repeat positively reproduces reader and writer scheduling delays
while the source-specific gossip workload is active. Count-loop preemption
stacks occur in the same trace window, including the ordinary path
`ActiveValidatorCount → ValidatorsReadOnlySeq → multi-value-slice.At`.
These transition stacks identify actual work at those events. With no
StackSamples, they do not quantify the CPU share of that work throughout every
wait, and 110 active iterators does not mean 110 continuously runnable callers.
The separate [complete preemption audit](full-gossip-domain-writer-count-trace-audit.md)
finds Count in 174 of 177 interrupted stacks across the three transport waits,
with Count on every P in each interval. It preserves all three exceptions,
including the trace advancer, and does not attribute an exclusive CPU share.

The primary native/snapshot pair separately establishes the native validator
registry iterator implementation's effect in this composition: maximum
DomainData latency 6.856 seconds versus 3.298 ms. The snapshot preserves all
120,000 validator predicates but bypasses the native state/registry iterator
access, including its state read lock, per-entry MVS access, and wrapper
updates. It is not a mutex-only toggle. Between the older P4 diagnostic and
this coherent writer composition, fork gates, canonical head setup, the
pre-load API preflight, and probe coverage also changed; those older runs
cannot isolate the addition of the one early head-state copy.

For historical node169 slot one, this is a reproduced mechanism and an
ordinary-RPC delay of sufficient scale in the untraced primary. It is not a
recovered historical packet timeline. The historical PTC marker establishes
that RolesAt returned **by** slot +9.915 seconds; it does not establish the
proposer's exact RPC invocation time. The primary long call completed before
+10 seconds, and later calls in that trial were fast. Its 6.856-second latency
must not be moved to a different hypothetical invocation phase to manufacture
a historical deadline failure.
