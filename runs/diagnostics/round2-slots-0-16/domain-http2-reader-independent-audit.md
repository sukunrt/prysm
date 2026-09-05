# Independent review of the retained H DomainData reader trace

The causal interpretation in [the reader report](domain-http2-reader-trace-results.md)
passes review with its stated packet-arrival and historical-transfer limits.

## Checks performed

- Read the original captured VC/BN markers and the diagnostic `Start`/`emit`
  implementation. The VC interval brackets the actual RPC after its cache
  write lock; the markers capture time before nonblocking logger enqueue.
- Recomputed the slot-two numbers independently with integer nanoseconds:
  RPC 139.232142 ms; reader runnable 135.385407 ms; overlap 133.253675 ms;
  handler dispatch wait 0.953856 ms; body 3.597 microseconds.
- Applied native Sync N=18's wall/Trace offset. The trace's explicit
  `DomainData` enqueue stack occurs 713 ns after the sole matching BN
  RANDAO body-begin marker. G5070 is the traced creator of G57147, so the
  reader-to-handler relation does not depend on choosing a nearby goroutine.
- Read the full original decode range 14287303–14389114 in
  `/tmp/prysm-H-domain-trace.parsed.txt`. Its only G5070 state transitions
  are the beginning Waiting-to-Runnable and ending Runnable-to-Running.
  Its complete 53-sample inventory matches the retained count excerpt;
  all four Ps are represented. The excerpt is not a Count-selected subset.
- Checked the recorded discrete profile-request times and I2's successful
  runtime-trace capture. Neither H target RPC overlaps a discrete goroutine
  snapshot. Continuous profiling/tracing remain an explicitly shared observer.

## What the evidence supports

The exact BN connection reader that later dispatches the local RANDAO handler
was ready but did not run for 135 ms while every sampled P executed the
gossip count path. The handler's own runnable delay and body time were small.
This establishes a concrete transport scheduling mechanism in the full-service
reproduction, beyond an undifferentiated CPU-overload explanation.

The reader became ready 2.131732 ms **before** this RANDAO RPC was invoked.
That wake cannot be identified as this request's packet arrival. With no gRPC
packet capture, the 133.253675 ms overlap is not an exact measurement of this
packet's residence in the kernel or proof that no client-side delay coexisted.

I2's count ablation is a meaningful whole-workload counterfactual. However,
H's own slot-two RANDAO begins at slot +5.107391378 s; I2's begins at
+0.008018264 s. Their 139.232142 ms versus 1.636109 ms comparison includes
the ablation's effects on preceding role timing and backlog. It is not an
isolated comparison of reader service at an identical load phase.

Both observed own RANDAO RPCs succeeded. Their trace does not by itself
reproduce historical node 169's failure. Its PTC-role marker has roughly
2.085 seconds left, but does not timestamp its RANDAO invocation. The
defensible transfer is the demonstrated
code mechanism and its count-overload cause, not the local goroutine identity,
duration, or a claim that the historical request's exact wait has now been traced.
