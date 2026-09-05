# Ordinary DomainData delayed at the BN HTTP/2 reader

The retained full-service H trace identifies a concrete upstream delay for an
actual VC RANDAO DomainData RPC: the BN's HTTP/2 connection reader was ready to
run but remained unscheduled for **135.385407 ms**. The actual RPC overlapped
133.253675 ms of that interval. Once the reader ran, it created the goroutine
that executed the matching DomainData handler; the handler body took 3.597 µs.
All **53 CPU stack samples** in the reader's runnable interval were in the real
gossip-validation `ActiveValidatorCount` path, distributed across all four Ps.

This locates an ordinary-Domain transport scheduling mechanism in the existing
full-service reproduction. H's matched BN3-only count ablation, I2, reduced the
corresponding own RANDAO RPC from 139.232142 ms to 1.636109 ms. These are retained
measurements, not a new workload or an injected delay. Historical node 169's
individual request remains untraced; this result supplies a concrete mediator
for the already established startup count-overload cause without assigning H's
goroutine IDs or exact durations to that historical request.

## The actual slot-two RANDAO call

All wall times below are UTC on 2026-09-05. The VC and BN use `captured_time`,
recorded before nonblocking diagnostic enqueue. They are not logger output
times. The VC markers bracket its actual `validatorClient.DomainData` call,
after acquiring the domain cache write lock. For this request the preceding
write-lock acquisition took 350 ns, so the measured RPC interval is not a
domain-cache lock queue.

| Event | UTC or trace timestamp | Evidence |
| --- | --- | --- |
| Own VC RANDAO RPC begins | 15:11:20.107391378 | `validator3.log:40594`, phase `vc.domain_data.rpc.02000000`, id 18943, slot 2 |
| Own VC RANDAO RPC ends | 15:11:20.246623520 | `validator3.log:40612`, same phase/id/slot |
| Matching BN body begins | 15:11:20.241711156 | `beacon3.log:999`, phase `bn.domain_data.02000000`, id 63 |
| Matching BN body ends | 15:11:20.241714753 | `beacon3.log:1000`, same phase/id |
| Reader G5070 leaves network wait | 192199231802625 | Waiting → Runnable |
| Reader G5070 next runs | 192199367188032 | Runnable → Running; no intervening run or block |
| G5070 creates handler G57147 | 192199367275776 | `serveStreams.func2 → http2Server.operateHeaders → HandleStreams` |
| Handler G57147 first runs | 192199368229632 | Runnable → Running |
| G57147 enqueues its DomainData begin event | 192199368254848 | Wakes logger G5560; stack explicitly includes `Server.DomainData` |
| G57147 exits | 192199368269632 | Running → NotExist; no intervening blocked or runnable interval |

The VC invocation-to-body interval is **134.319778 ms**. The handler spends
only **0.953856 ms** runnable before its first execution. The long identified
queue is therefore before handler creation, at the connection reader. Its
preceding network wait is in:

`FD.RawRead → readyreader.ReadOnReady → bufReadyReader.Read → ReadFrameHeader
→ grpc.transport.framer.readFrame → http2Server.HandleStreams →
grpc.Server.serveStreams`.

After resumption, the trace shows the reader's actual socket-read syscall and
header processing. The parent/child relation identifies the connection and
handler directly. The handler's DomainData stack and clock-aligned diagnostic
enqueue identify the body marker; this is stronger than choosing an arbitrary
goroutine near the log time. H has one corresponding RANDAO body in this
interval, and its own VC call brackets it. The diagnostic IDs are local to each
process, so id 18943 and id 63 are not a propagated end-to-end request ID.

There is no gRPC packet capture in this evidence. G5070 became runnable
**2.131732 ms before this VC invocation**, possibly for another stream. Its
initial wake must not be called this RANDAO request's packet arrival. The
measured statement is that 133.253675 ms of this RPC occurred while its eventual
connection reader was runnable but unable to process any request. The alignment
of reader resumption, handler creation and body entry strongly attributes the
pre-handler delay to reader scheduling; it does not measure this particular
request's exact kernel-arrival-to-read interval.

## What executed while the reader waited

Selecting **every** native trace `StackSample` event between
192199231802625 and 192199367188032 yields 53 samples, all containing:

`pubsub.validation → validateCommitteeIndexBeaconAttestation →
validateUnaggregatedAttTopic → validateCommitteeIndexAndCount →
ActiveValidatorCount → ValidatorsReadOnlySeq / validator storage`.

There are 13 samples each on P0, P1 and P2, and 14 on P3, covering 52 goroutine
IDs. Thirty sampled stacks include `sync.RWMutex.RLock`. This is evidence from
the exact reader-delay interval in the same process, not a transfer from E1's
goroutine snapshots or a whole-run CPU percentage. Sampling does not establish
that every CPU instruction in the interval was Count work. For example, the
same interval also contains brief BLST aggregation workers yielding through
`runtime.Gosched`; those transitions are not measurements of their CPU share.

The full H profile independently attributes 137.77 of 152.51 CPU-seconds
(90.34%) to Count. The unchanged source path repeatedly scans the slot-zero
registry during warm committee validation. See
[the historical source-transfer audit](historical-count-path-transfer-audit.md)
and [the H/I2 comparison](../startup3/wire-causation-results.md).

### Exact running intervals during the reader wait

Reconstructing every goroutine's Running-state intervals from the existing
bounded trace adds a stronger scheduling account than the samples alone.
While G5070 remains runnable, **891 other goroutines** receive **963 Running
intervals**. Their summed Running-state wall time across the four Ps is
**540.824700 ms**, out of a four-P window of 541.541628 ms. This measures Go
runtime state, not CPU-clock time; OS descheduling can occur inside a Running
interval, and syscall/scheduler intervals are excluded.

| P | All goroutine Running time | Running time of the Count-sampled goroutines |
| --- | ---: | ---: |
| 0 | 135.275007 ms | 131.680703 ms |
| 1 | 135.343615 ms | 133.725759 ms |
| 2 | 135.101695 ms | 132.202752 ms |
| 3 | 135.104383 ms | 133.760575 ms |
| Total | **540.824700 ms** | **531.369789 ms** |

Each of the 52 Count-sampled goroutines has one uninterrupted Running interval
in this window. **All 52 end by waiting for a BLS aggregation child**, with
the exact production stack `P1Aggregate.coreAggregate → AggregatePublicKeys →
AggregateKeyFromIndices → AttestationSignatureBatch → gossip validation`.
None of those intervals ends in a preemption event. The same window separately
contains 50 explicit `Running → Runnable`, `runtime.Gosched` events from BLST
children. For example, G16624 on P1 runs from 192199239392128 to
192199250029568 (10.637440 ms) before entering the aggregation-parent channel
wait. This duration is measured, not an assumed scheduler quantum.

The 531.369789 ms is the Running time of goroutines whose Count work is
independently sampled, not an exact exclusive duration of the Count function;
it also includes their transitions into signature preparation. Conversely,
some remaining intervals can contain unsampled Count work. The trace therefore
shows repeated dispatch of count-heavy gossip goroutines and their children
while the ready transport reader receives no execution, rather than inferring
this sequence from a CPU-profile percentage.

Go 1.26.5's `runtime.findRunnable` selects ordinary local/global runnable
queues with periodic fairness checks (`runtime/proc.go:3439–3474`); an RPC's
context deadline does not give its connection reader scheduling priority.
The trace does not record queue position, queue-selection source or
`schedtick`, so it does not establish that a particular queue order, 61-tick
check, or fixed time slice accounts for the exact 135 ms. The concrete result
is the measured absence of reader execution while the other work runs.

The [complete clipped Running intervals](domain-http2-reader-running-intervals.tsv)
retain all 963 intervals, the Count-sample classification and boundary flags.
The [52 terminal stacks](domain-http2-reader-count-parent-exits.txt) establish
the actual parent waits. Their source is the same raw H trace and bounded
decode already cited below; no new capture, profiling or workload was run.

## A second ordinary-domain call and the count counterfactual

H's slot-one sync-selection DomainData call supplies the same independent
reader-to-handler chain: G5070 is runnable for **126.415936 ms**, then creates
G7995, whose diagnostic enqueue stack explicitly identifies DomainData. Its
handler has a further 1.041984 ms runnable wait. The captured RPC is
15:11:03.145206137–03.281451356; the body is
03.280458872–03.280461447 (`validator3.log:39865–39866`,
`beacon3.log:429–430`). Thus 135.252735 ms elapse before a 2.575 µs body.

| Actual own-VC RPC | H, original count | I2, BN3-only count ablation |
| --- | ---: | ---: |
| Slot 1 sync-selection DomainData | 136.245219 ms | 0.300169 ms |
| Slot 1 RANDAO DomainData | 30.500804 ms | 0.280030 ms |
| Slot 2 RANDAO DomainData, traced above | 139.232142 ms | 1.636109 ms |

I2's corresponding source lines are `validator3.log:39605–39606`,
`39626–39628`, and `40329–40332`. I2 preserves the complete source/relay service
and the 45,000 submitted votes; only BN3's genesis count behavior changes.
Its profile has zero Count samples, and all three proposals submit, compared
with three failures in H. These are matched workload arms, not identical
event-by-event schedules. Their result supports count overload as the cause
of the scheduling pressure; it does not isolate BLST yields or a particular
Go run-queue policy as a separate necessary cause.
In particular, H's slot-two own RANDAO starts at slot +5.107391 s, whereas I2's
starts at +0.008018 s. Count removal also changes preceding role timing and the
backlog present when the RPC begins. This is a whole-workload counterfactual,
not an isolated reader-service benchmark at a fixed load phase. Both measured
RANDAO RPCs succeed. The trace locates a real delay mechanism; it does not itself
reproduce node 169's deadline failure. Its observed role-progress marker has
2.085 s left; the remaining budget at its RANDAO invocation is unlogged.

## Clocks and observer checks

The trace's own Sync records supply wall alignment. Use their `Trace` value,
not the event header's `Time` value:

| Target | Sync | Trace | UTC wall | Wall minus trace, ns |
| --- | --- | ---: | --- | ---: |
| Slot 1 | N=2 | 192182054357120 | 15:11:02.927814060 | 1788428880873456940 |
| Slot 2 | N=18 | 192199135467392 | 15:11:20.008924413 | 1788428880873457021 |

For slot 2 this puts the reader's runnable interval at
15:11:20.105259646–20.240645053. The handler's enqueue is 713 ns after the
captured body-begin timestamp, consistent with `time.Now()` preceding enqueue.
For slot 1 the reader interval is 03.152925164–03.279341100. Nearby Sync offsets
agree within sub-microsecond resolution. The older engine-analysis marker
offset is about 5.9 µs different; the numbers here use these native Sync pairs.

Neither complete target RPC overlaps a discrete BN or VC goroutine-profile
request:

| Target RPC | Adjacent BN profile requests |
| --- | --- |
| Slot 1, 03.145206137–03.281451356 | +1 completed 15:10:52.047657892; +13 starts 15:11:04.027662382 |
| Slot 2, 20.107391378–20.246623520 | +25 completed 15:11:16.388366814; +31 starts 15:11:22.018772402 |

The closest gaps are 746.211 ms after the first RPC, and 1.772149 s after the
second. Continuous CPU profiling and runtime tracing were active in both H and
I2; these are not observer-free measurements. The exclusions address discrete
snapshot overlap, while the matched observer design and count ablation support
the causal comparison.

## Retained evidence and reproduction

- [Complete H ordinary-Domain marker inventory](domain-http2-reader-h-complete-marker-inventory.tsv), including all 27 request/body pairs and pre/body/post durations.
- [Exact reader/handler transitions and stacks](domain-http2-reader-trace.transitions.txt).
- [All 53 CPU stack samples in the slot-two reader interval](domain-http2-reader-count-samples.txt), selected by time and event type, without filtering for Count.
- [H captured markers with original physical line numbers](domain-http2-reader-h-markers.txt).
- [I2 captured markers with original physical line numbers](domain-http2-reader-i2-markers.txt).
- [H profile request timestamps](domain-http2-reader-h-profile-times.tsv).

Raw H trace: `/tmp/prysm-startup3-wire-h/evidence/trace.out`.
SHA-256: `cd1c4fb122bc8f2fc472f4575ac37baf477a7107634748a3bb479eb3f01b7fd2`.
Decode with Go 1.26.5:
`/home/sukun/dev/go/bin/go tool trace -d=parsed PATH/trace.out`.
The retained excerpts preserve original event timestamps, goroutine IDs, and
stack frames. The full decode is `/tmp/prysm-H-domain-trace.parsed.txt`; the
bounded working extraction is `/tmp/prysm-H-domain-windows.txt`.
An independent review checked full-decode lines 14287303–14389114: they contain
the same 53 CPU samples and only the two bounding G5070 state transitions,
confirming that neither the sample denominator nor an intervening reader run
was removed by the excerpt filter.

Both H and I2 used BN image
`sha256:292859db27d957440ff62b93783fd3eee3152c1fcae6cff1c1d92b82a9d3e278`.
The trace itself identifies Go 1.26.5 paths and gRPC v1.81.1 frames. Instrumented
source in `/tmp/prysm-startup-repro/runtime/startupdiagnostic/startupdiagnostic.go`
captures timestamps before a nonblocking queue send; its DomainData begin call
appears in the retained trace at `validator/server.go:180`. Consequently these
phase spans avoid the separate probe's post-RPC output-mutex timing ambiguity.

## Follow-up inventory: no longer marked Domain RPC in H

A complete scan of H's VC and BN phase markers finds **27 RPC begin/end pairs
and 27 body begin/end pairs**, with equal counts for every domain type. Pairing
by domain and sequential occurrence places every BN body entirely inside one
VC RPC interval. No begin or end is unmatched, and neither log contains a
diagnostic `dropped` marker. The longest RPCs are the 139.232142 ms and
136.245219 ms calls above, followed by the 30.500804 ms slot-one RANDAO and an
11.131532 ms attester-domain call. No other marked ordinary-domain request
provides a seconds-scale trace candidate.

H's retained run metadata enables the in-BN engine probe; it does not contain
E1's separate repeating Domain/sync-index probe or a `rpcprobe.jsonl`. Its BN
log has one client-connection announcement (`beacon3.log:41`), for the VC.
The complete marker inventory therefore supplies no additional external-Domain
population to inspect in H's trace.

The [later aggregate server metrics](domain-http2-reader-h-domain-metrics.txt)
count 38 successful DomainData calls, all within the 5 ms handler-interceptor
bucket. This differs from the 27 marked calls: startup phase instrumentation
explicitly stops after slot 3, and the retained VC continues into slot 4.
The metrics do not timestamp those extra unmarked calls or bound time before
interceptor admission. H's retained native trace ends at approximately
15:11:37.933, before slot 4 begins at 15:11:39. Thus the later aggregate metrics
cannot supply a new traced seconds-scale Domain admission or callback delay.
The existing E1 two-second pre-body observation remains separate evidence;
this inventory does not convert it into a traced wait.
