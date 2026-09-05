# Count execution while probe 14's transport goroutines wait

The native trace repetition supplies direct Count-work identity during each
of [probe 14's three transport scheduling waits](full-gossip-domain-writer-trace-audit.md).
Across those intervals, **174 of 177 preemption events** interrupt
`ActiveValidatorCount` in actual gossip validation. Each interval has Count
preemptions on all four Ps. These are interrupted execution stacks, not CPU
samples, an assumed time quantum, or an estimate of exclusive CPU share.

| Transport interval | Count preemptions / all preemptions | Distinct Count goroutines | Count preemptions on P0 / P1 / P2 / P3 |
| --- | ---: | ---: | --- |
| RPC invocation to reader first run, approximately 286.260 ms | 61 / 63 | 61 | 28 / 14 / 9 / 10 |
| Writer initially runnable, 227.516928 ms | 41 / 42 | 41 | 22 / 6 / 6 / 7 |
| Writer runnable after its explicit `Gosched`, 336.046720 ms | 72 / 72 | 69 | 35 / 12 / 14 / 11 |

The other three interrupted stacks are two in attestation signature/BLS
preparation and one in the runtime trace advancer. Retaining that last event
also makes the observer visible; the trace repetition is not the untraced
6.856-second primary.

All Count events preserve the production ancestry from pubsub validation
through `validateCommitteeIndexBeaconAttestation`,
`validateUnaggregatedAttTopic`, and `validateCommitteeIndexAndCount`. The
interrupted work includes the native validator iterator and MVS `At` access.
Thus a live Count cohort actually executes and is preempted while the gRPC
reader or writer remains ready but unscheduled. The result does not require
assuming that the active-iterator counter counts only runnable goroutines.

The exact native writer bounds are 277057913756928–277058141273856 and
277058141410688–277058477457408. Reader resumption is 277057913732096;
the start of the first row is the actual RPC invocation mapped through the
native clock synchronization records, as documented in the transport audit.
Adjacent clock mappings differ by 62 ns, with no preemption event at that
boundary, so they produce the same event inventory.

## The cohort's earlier cause remains separate

The [whole-trace follow-up](full-gossip-finalizer-cohort-trace-audit.md) now
joins a natural validator-finalizer gate to an earlier 101-reader Count wake
wave. It also follows the different Count goroutines seen during probe14
from their earlier checkpoint-key waits. The original 101 are already past
Count by this RPC; the full join keeps these populations distinct.

The complete event slice contains no `Running → Waiting`, reason `sync`,
transition in a Count or another MVS stack during this RPC's approximately
850 ms lifetime. This excludes a newly recorded MVS sync park in that slice;
it does not rule out a goroutine already waiting before the slice, an
uncontended writer, or an earlier gate that helped form the cohort.

The explicit cold attestation-data call happened much earlier. Its recorded
`HeadState` delegate took 130.617 µs at slot +0.060007–0.060138, with five
active iterators. At probe 14 admission, the recorded active count is 107 and
the peak is 110. These data do not show the early copy collecting that later
cohort. In the untraced primary, the peak is instead 1,324 and the longest
DomainData call is 6.856 seconds. Neither the exact preemption census nor a
specific cohort-formation gate should be transferred to that primary.

## Retained evidence

- [Complete classified preemption inventory](full-gossip-domain-writer-count-preemptions.tsv), including original parsed line ranges, goroutine IDs, Ps and timestamps.
- [Representative Count stacks for each phase and P, plus all three exceptions](full-gossip-domain-writer-count-preemption-excerpts.txt).
- [Transport ancestry, clocks, raw trace hash and exact wait reconstruction](full-gossip-domain-writer-trace-audit.md).

The existing complete slice is
`full-gossip-domain-writer-go-evidence/trace-shared/probe14-window-and-http2-goroutines.txt`.
Its all-event interval runs from trace 277057552744832 through
277058548218880, covering all three table intervals. The preceding 69 ms of
the reader's full 430 ms ready interval are outside that all-event slice and
are deliberately excluded from this census. No new decoding, CPU profile or
workload was run for this audit.
