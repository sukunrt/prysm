# Natural finalizer releases a 101-reader Count cohort

The retained 23.4-second native full-gossip trace directly identifies a
cohort-forming gate. At slot **+0.536 seconds**, the runtime finalizer's
validator-storage `Detach` wakes **101 blocked Count goroutines** in
**28.096 µs**. Their median delay from that wake to first execution is
**947.173120 ms**, and the maximum is **1.918168640 seconds**. This is an
observed writer-to-reader dependency followed by scheduler delay, rather
than an inference from an active-iterator counter or a goroutine snapshot.

The gate belongs to the traced repetition. It does not identify the
finalized state's allocation, the gate in the separate untraced 6.856-second
DomainData call, or node169's historical request. The early timed
attestation-data `HeadState` copy is a different, much smaller event.

## Checkpoint wait, reader gate, and release

The long checkpoint wait can itself be decomposed. G4970 parks on the key
at 277054191954560 and is granted it at 277054661603328: exactly
**469.648768 ms**. Walking actual key-send wakers backward gives
[60 successive admitted checkpoint callers](full-gossip-checkpoint-token-chain.tsv)
covering that entire interval. The boundary caller G4932 was already runnable
when G4970 parked; its prefix before the target wait is excluded.

| Contribution inside G4970's complete key wait | Duration |
| --- | ---: |
| Each admitted caller's key grant to its first Running event | **468.388673 ms** |
| First Running event to release of the next key waiter | **1.260095 ms** |
| Total, with no uncovered interval | **469.648768 ms** |

Thus **99.731694% of this serial handoff interval** is time when an admitted
caller is runnable but has not resumed to pass the key onward. Once it first
runs, each of the 60 callers releases the next waiter within
**5.184–125.760 µs**. Their complete native histories contain no intervening
Go state transition in those short run-to-release intervals: no additional
recorded synchronization park or syscall explains the warm-cache delay.
The sole overlapping stop-the-world interval, GC sweep termination lasting
**108.736 µs**, lies inside the ready component. The percentage is a
decomposition of this particular **serial key-wait interval**, including
that GC pause; it is not Count's CPU fraction or a claim that all ready time
is spent executing its immediate predecessor.

There is direct evidence for the preceding caller continuing into Count
while its successor remains ready. **16 of the 60 handoffs** have an actual
Count preemption stack on the releasing caller before its successor first
runs. For example, G4965 releases G4966 at 277054629258624, is subsequently
preempted in Count at 277054637082944, and G4966 first runs at
277054637085824. The source and trace agree: deferred checkpoint `Unlock`
returns the logical key before that caller performs Count. The newly
admitted caller cannot pass the key to the rest of the backlog until it
receives processor time. These joins do not require a costly cache lookup,
a continuously running key holder, or a held global-manager token.

The [101-member join](full-gossip-finalizer-cohort-members.tsv) follows each
goroutine through its actual preceding checkpoint wait, MVS park and wake,
first execution, BLS wait, and signature-verifier wake. Every member first
waits in `AttestationTargetState → getAttPreState → async.(*Lock).Lock`
at `multilock.go:44`. These are receives on the **checkpoint's logical key**,
not waits for the manager's global registry token. All are subsequently
woken by another `getAttPreState` caller's deferred `Unlock`, line61. Their
preceding key waits last **437.432768–469.852416 ms**.

The 98 members that next block in `Count → ValidatorsReadOnlySeq → MVS.Len`
are released from the checkpoint key within **986.496 µs**; 97 of those
wakers are themselves members of the 101-reader cohort. For example,
G4969 releases G4970, and G5083 releases G5133. Each caller can release the
checkpoint key before doing its expensive Count scan. The pending validator
writer then blocks those arriving readers. Three further cohort members
already inside the scan block at `MVS.At`.

The writer changes that observed progression. All **98 Len entrants** have
a recorded outgoing checkpoint key release followed by their own Len park,
with a median separation of **3.232 µs** and range **2.688–201.024 µs**.
The first interval includes the later mark-termination pause. Rather than
continuing the validator loop, these callers now become blocked readers,
allowing other ready checkpoint callers to run and pass the key onward.
The resulting rapid key handoffs collect 98 full scans behind one pending
writer; its subsequent unlock releases that accumulated work together.
The [release-to-Len table](full-gossip-checkpoint-to-reader-gate.tsv) joins
each of those 98 outgoing handoffs to its actual MVS park. This explains
how a short writer gate changes concurrency without requiring expensive
copy or finalizer mutation.

The exact finalizer sequence is:

| Event | Native trace time, ns | Duration or consequence |
| --- | ---: | --- |
| G6 first runs after GC mark termination | 277054661822208 | Slot +0.535279205 |
| G6 parks in validator `Detach`'s writer `Lock` | 277054661840512 | `state_trie.go:1747`; MVS line356 |
| Count G4968's `At.RUnlock` wakes G6 | 277054662598208 | **757.696 µs** blocked |
| G6 first runs again | 277054662600896 | **2.688 µs** runnable |
| G6's deferred MVS `Unlock` wakes the first reader | 277054662602688 | Finalizer line1747; MVS line399 |
| Same unlock wakes the 101st reader | 277054662630784 | **28.096 µs** wake span |
| G6 returns to its runtime wait | 277054662632256 | **31.360 µs** after resumption |

All 101 wakes join prior `Running → Waiting, reason=sync` Count parks:
**98 at Len and three at At**. Their MVS blocking intervals are only
**30.144–770.496 µs**. All 101 are simultaneously runnable at the final
wake; none has resumed yet. First execution then ranges from **1.728 µs**
to **1.918168640 s** after the individual wake. G5133 is last to receive
its first execution, at slot +2.454256165.

The 31.360 µs interval includes unlock wakeups and the remainder of the
finalizer invocation; it is not an exclusive CPU measurement of `Detach`.
Similarly, the much longer reader-ready intervals are scheduling delays,
not time spent still waiting for the MVS lock. The 98 Len waiters have not
entered the validator loop: this valid 120,000-validator fixture therefore
still requires **11.76 million validator visits** for those 98 alone. The
three At waiters' remaining indices are unrecorded. The
[historical source audit](historical-finalizer-path-audit.md) explains the
shared mutex, reader exclusion while a writer is pending, and lack of a
context check inside these repeated scans.

The finalizer follows a normal recorded GC mark-termination stop from
277054661634944 to 277054661753216 (**118.272 µs**). There is no explicit
`runtime.GC`, manual finalizer call, or injected held lock in this diagnostic.
The trace itself can alter allocation and scheduling timing; this observation
does not establish that the same GC/finalizer interleaving occurred in an
untraced run.

## Downstream work and the later RPC

For **all 101** members, the first post-release blocking point is the real
`P1Aggregate.coreAggregate` child-result receive, `blst.go:1660`, reached
through `AggregateKeyFromIndices → AttestationSignatureBatch`. Each creates
one aggregation child and is woken by that child's result send at
`blst.go:1649`. Those parent receives start between slot +0.559550437 and
+2.871100389 and last **1.011392–394.810496 ms** until their recorded wakes.
This is a blocked interval, not exclusive BLS computation time or a census
of all child execution.

All 101 later wait in `validateWithBatchVerifier`, line64, and receive a
signature-verifier wake. Their recorded batch waits range from
**6.479424 ms to 4.093018048 s**. These downstream waits are distinct from
the initial Count ready cohort; pubsub validation has not completed merely
because Count finishes.

At probe14's actual invocation, **10 of the original 101 have exited and
91 are waiting for signature-batch verification**. None contributes any
of the [174 Count preemptions during that RPC's transport waits](full-gossip-domain-writer-count-trace-audit.md).
Those preemptions belong to **134 different validation goroutines**. All
134 were already created at slot +0.097480–0.124077, before the finalizer
wave. A [separate complete join](full-gossip-probe14-checkpoint-backlog.tsv)
shows that all 134 then parked on the checkpoint logical key at slot
+0.097492–0.124092 and were woken by checkpoint `Unlock` **after** the
finalizer wave, at slot +2.913444–4.330534. Their key waits lasted
**2.815952384–4.206442240 seconds**. Each subsequently runs, releases the
next key waiter, and is observed being preempted in Count during probe14.
Thus an already-created checkpoint backlog demonstrably progresses into
later Count work. This does not require newly created goroutines or a
particular pubsub capacity-release mechanism. Precise Count-entry times
are not marked. The
[separate transport audit](full-gossip-domain-writer-trace-audit.md) supplies
the exact reader/handler/writer joins for the later RPC.

## Whole-trace writer census and limits

The [complete recorded Copy/Detach event inventory](full-gossip-mvs-writer-event-inventory.tsv)
contains two Copy parks and five Copy reader wakes, plus two finalizer
parks and 108 finalizer reader wakes. Every one identifies **validator**
storage by its native-state caller line. The first cold sync-head Copy
at slot about +0.058 wakes three mid-scan readers; the timed attestation-data
Copy at about +0.060 wakes two. The latter's complete measured delegate
duration remains **130.617 µs**, with five active iterators.

After the 101-reader finalizer wave, there are only **seven additional
recorded validator-Detach reader wakes**, around slot +4.888 seconds; all
join Count At parks, lasting **4.224–293.824 µs**. There is no further
recorded Copy/Detach wake family in the remaining trace. This census does
not see uncontended copies/detaches and cannot identify the finalizing
object or prove that no other synchronization contributes to scheduling.
Balances would instead appear at native Copy line1153 or Detach line1741;
the observed gates use validator Copy line1157 or Detach line1747.

The causal result is a concrete natural path from checkpoint waiters through
a short writer gate into a large ready Count cohort. Count's presence alone
does not guarantee that interleaving: the fresh matched request-omission
pair was fast in both arms, as recorded in the
[matched omission report](full-gossip-domain-no-timed-attdata-results.md). This trace
contains **no CPU StackSample events**; none of these counts or durations is
an exclusive Count CPU fraction, fixed scheduler quantum, or historical
request-stack identification.

## Evidence and reproduction

- [Focused complete native event blocks](full-gossip-finalizer-cohort.transitions.txt), with original parsed line ranges, cover both early Copy gates, representative checkpoint/reader joins, G6's park and reader-origin wake, and downstream BLS/batch boundaries.
- [Full backward checkpoint-key chain](full-gossip-checkpoint-token-chain.tsv) preserves the target and all 60 preceding callers; [focused native stacks](full-gossip-checkpoint-token-chain.transitions.txt) retain boundary and representative Count continuations. The [chain extractor](extract_full_gossip_checkpoint_chain.py) follows actual waker IDs, retaining full histories in `/tmp/full-gossip-checkpoint-backchain-full-histories.txt`. Clipped coverage uses `[G4970 park, G4970 grant)`, rather than summing the boundary caller's entire ready interval.
- [All 98 outgoing key releases followed by Len parks](full-gossip-checkpoint-to-reader-gate.tsv) quantify the changed progression while the validator writer is pending.
- [All 101 joined members](full-gossip-finalizer-cohort-members.tsv) preserve the denominator, every park/wake time and source range, first-Running event, aggregation child ID, and batch boundary.
- [Independent member-join validation](validate_full_gossip_finalizer_cohort.py) checks every member against the complete extracted native history, including source ranges, first run, subsequent waits, and the 10-exited/91-batch-waiting state at probe14.
- [All 134 later Count goroutines](full-gossip-probe14-checkpoint-backlog.tsv) join their native creation, checkpoint park/grant, first run, next key release, and first Count preemption in probe14. The [checkpoint extractor](extract_full_gossip_gid_checkpoint.py) retains their complete checkpoint event stacks in `/tmp/full-gossip-probe14-count-checkpoint-events.txt`.
- [Independent checkpoint-summary validation](full-gossip-checkpoint-summary-validation.txt), generated by [the deterministic checker](validate_full_gossip_checkpoint_summaries.py), verifies the exact 60-holder interval partition, absence of intervening holder state transitions, all 98 release-to-Len joins, and all 134 pre-wave checkpoint-backlog joins against the retained complete event blocks.
- [GC markers](full-gossip-finalizer-gc-markers.tsv) preserve the preceding mark-termination timing.
- The [whole-trace event extractor](extract_full_gossip_mvs_trace.py) selects all 1,807 events mentioning MVS or Count, including writer wake stacks. The [cohort extractor](extract_full_gossip_wake_cohort.py) derives the 101 readers from G6's actual wakes and retains their complete event histories. Local full extracts are `/tmp/full-gossip-mvs-count-events.txt` and `/tmp/full-gossip-finalizer-cohort-events.txt`.
- Raw trace: `full-gossip-domain-writer-go-evidence/trace-shared/runtime.trace`, SHA-256 `5e67d6e68ba55d668f4fa2b9a381d7b4c5fb2474e3f2741b4b0fdf6b123ad0cf`. Existing parsed trace SHA-256 `5d1a50b2a1cc081fb6035e528bca931d20b7506e14563b1e896cbc3e9b22c6e4`.
- Slot offsets above use the native N3 calibration `wall = trace + 1788428880873456997 ns`, with slot1 beginning at Unix `1788705935000000000 ns`. Adjacent calibration differs by 62 ns; native interval durations do not depend on that wall conversion.
