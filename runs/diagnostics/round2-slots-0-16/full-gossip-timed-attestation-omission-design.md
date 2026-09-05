# Matched omission of the timed attestation-data request

This discriminator asks whether the **one timed cold GetAttestationData RPC**
is necessary for the large Count cohort and unrelated DomainData delays in
the coherent one-BN fixture. It does not remove every state copy and does not
isolate the MVS write lock by itself.

The preceding native/snapshot comparison toggled the validator iterator
implementation. The older P4 comparison also differed in fork gates,
fork-choice head initialization, API preflight, and probe coverage. Neither
was a matched test of omitting just the timed attestation-data request.

## Fixed construction

Both new arms retain `PRYSM_DIAGNOSTIC_FULL_GOSSIP_WRITER=1`. That existing flag
continues to enable the coherent fixture and the entire absolute-slot probe
schedule. Turning it off would reintroduce the previous confounding changes.

Both arms use the same freshly compiled binary and the native shared iterator:

- 120,000 valid validators; all prerequisite fork epochs zero; eight slots
  per round; the existing six exercised GossipSub topics and queue limits.
- The same construction of 15,000 valid signed slot-one messages and planned
  two-second publication schedule. Each fresh arm generates its messages
  against its own genesis time/root, so message bytes and signatures need not
  match between arms. Validator/committee construction, message count, and
  distribution are fixed; there are no extra votes, topics, or backlog.
- Actual FullHead selection at slot one and assertions of the canonical
  genesis root and full-payload status.
- The actual pre-load GetAttestationData coherence check, including its
  initially cold temporary cache and its real head-state copy. That separate
  cache must populate in both arms.
- The same measured server dependencies, fresh measured attestation cache,
  warmed external child-process gRPC connection, first-iterator trigger, and
  actual cold sync-index lookup.
- Probe zero's goroutine and release barrier, followed by 47 sequential
  DomainData probes on the same 250 ms absolute slot-one grid. All invocations
  retain the slot-two deadline. Elapsed ticks are skipped, not replayed.
- `GOMAXPROCS=4`, with no CPU profiling, runtime trace, or stack snapshots.

Only a narrow omission flag suppresses the measured child-process
`invokeFullGossipAttestationData` call. In particular, it must not select a
different path through fixture construction or switch the probe count or
deadline behavior. The implementation emits an explicit omitted result rather
than pretending that an unissued RPC succeeded. It preserves probe zero's
release goroutine and barrier, while omitting the attestation-request launcher
goroutine and its observer channels along with the request. Those allocations
are part of the whole-request intervention; this is not an allocation-matched
or write-lock-only control.

The omitted arm must record no measured handler admission, no measured
HeadState-copy record, and no population of the measured attestation cache.
Its temporary pre-load cache remains independently verified as populated.
The enabled arm retains the existing success/deadline checks and bounded
handler drain. Neither arm asserts that a long DomainData request must occur.

## Predeclared finite run sequence

Run exactly one fresh pair, in this order:

1. Native shared registry, timed attestation-data RPC omitted.
2. Native shared registry, timed attestation-data RPC enabled.

The binary, fixture, environment, and prescribed workload are identical except
for the omission flag and distinct output locations. Record the binary/source
identity and full command/environment differences. Do not use the old native
maximum as the enabled member of this fresh pair.

Retain every result. No phase adjustment, extra offered load, conditional
repeat, or repeat-until-positive follows this pair. Both workload completion
and all 48 grid records remain required; a deadline result is a measurement,
not a fixture failure. As in the earlier pair, successful Publish calls and
observed validation admissions are different quantities.

## Comparisons and decision rules

Compare the complete per-probe records, maximum and distribution of invoked
DomainData durations, invoke-to-admission delay, handler upper bound, and
post-handler tail. Also compare peak/observed active iterators, accepted and
stored votes, cold-sync timing, setup and release offsets, realized publication
duration/lateness, and time to drain. The active-iterator counter does not
classify goroutines as running, runnable, or parked.

The observed 6.856-second untraced maximum and 850 ms traced repeat already
demonstrate substantial execution variability. One pair therefore has the
following bounded interpretations:

- A seconds-scale delay without the timed RPC shows that this timed request
  is **not necessary** to generate such a pause in this fixture. It does not
  eliminate the pre-load or sync-index copies that remain in both arms.
- A fast omitted arm and slow enabled arm support the complete timed request
  as a contributing trigger in this pair. They do not establish universal
  necessity, a stable failure probability, or a particular internal gate.
- If both arms remain fast, the pair does not discriminate the trigger of the
  earlier large cohort. The earlier positive remains recorded; the null pair
  is retained alongside it without searching for a favorable execution.
- If both are slow, the timed request is unnecessary for the reproduced
  servicing problem, although it could still change its timing or magnitude.

Omitting the RPC removes more than its HeadState copy: client request encoding,
HTTP/2 frames and stream creation, server handler work, response enqueueing and
writer wake/coalescing, allocation, and possible later cleanup all change.
The existing trace specifically shows that connection-writer wake order and
its small-response batching yield can matter. Consequently, even a clean
positive difference identifies a **whole-request effect**, not proof that
one short MVS.Copy collected the later Count cohort.

The publisher shares the measured process. If native work delays publication,
the realized arrival schedule can differ despite identical planned offers.
That is an endogenous result to report, not a reason to retime one arm.
Neither arm supplies node169's historical proposer invocation timestamp or
reconstructs its exact deadline failure.
