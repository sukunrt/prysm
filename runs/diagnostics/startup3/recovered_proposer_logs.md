# Recovered proposer logs for slots 1–3

The proposer owners for all six missed early slots have now been recovered from
the public per-node archives. These logs establish the terminal failure seen by
each validator client. They do **not** by themselves prove how much of the
upstream delay was scheduler delay, lock waiting, CPU execution, or RPC
transport.

## Six-slot result

| Run | Slot | Node | Validator | Pubkey prefix | Direct terminal failure |
| --- | ---: | ---: | ---: | --- | --- |
| round1 | 1 | 3 | 1554 | `0x86dd193d4943` | `GetBeaconBlock` RPC reached the slot deadline (`validator.log:687`) |
| round1 | 2 | 81 | 116278 | `0x838b26a26e65` | BN returned “Could not get local payload and no P2P bid fallback” at 28.12 (`validator.log:689`) |
| round1 | 3 | 124 | 75976 | `0xae6514d46b74` | Sync-committee role preflight exhausted the slot; subsequently dispatched RANDAO failed on the expired context (`validator.log:791,796`) |
| round2 | 1 | 169 | 113126 | `0xa7120c370e8c` | RANDAO `DomainData` deadline at 01:30:24.002 (`validator.log:694`) |
| round2 | 2 | 191 | 87159 | `0xaf089d24fea6` | Sync-committee preflight failed at 36.001; RANDAO followed with expired context at 36.004 (`validator.log:770,780`) |
| round2 | 3 | 22 | 12756 | `0xa4a9ccf01138` | Sync-committee preflight failed at 48.0006; RANDAO followed with expired context at 48.003 (`validator.log:775,800`) |

The recovered files used above are under
`/tmp/prysm-r2-extra-logs.Rd7MjT/extracted/round{1,2}-<node>/validator.log`;
round1 node3 is checked in at `runs/round1/prysm-geth-3/validator.log`.
Public archive URLs are:

- [round1 node81](https://pub-6a398d8195fb4cffa21779149b904258.r2.dev/round1-prysm-geth-81.tar.gz)
- [round1 node124](https://pub-6a398d8195fb4cffa21779149b904258.r2.dev/round1-prysm-geth-124.tar.gz)
- [round2 node169](https://pub-6a398d8195fb4cffa21779149b904258.r2.dev/round2-prysm-geth-169.tar.gz)
- [round2 node191](https://pub-6a398d8195fb4cffa21779149b904258.r2.dev/round2-prysm-geth-191.tar.gz)
- [round2 node22](https://pub-6a398d8195fb4cffa21779149b904258.r2.dev/round2-prysm-geth-22.tar.gz)

## Round1 timelines

### Slot 1, node3

The execution client completed FCU with payload attributes at 12.114–12.115 in
about 2 ms (`beacon.log:900`, `execution.log:98–99`). The BN did not log
“Building block” until 18.973 (`beacon.log:902`), and the VC's block request
expired at 24.00 (`validator.log:687`). Graffiti was not reached until 30.10
and the build failed at 31.52 (`beacon.log:906,910–911`). This directly proves
late BN proposal handling and deadline cancellation. It does not identify what
occupied the 6.97-second pre-handler interval or the later handler interval.

### Slot 2, node81

The schedule identifies node81's proposer at `validator.log:651`; activation
maps it to validator 116278 at line 277. The BN entered block construction at
27.01, 3.018863399 seconds into the slot, and reached graffiti at 27.02
(`beacon.log:692–693`). At 28.12 it reported an execution HTTP-client timeout,
then no cached P2P fallback, and returned the terminal error at 4.123793386
seconds (`beacon.log:695–696`; `validator.log:689`). The snooper recorded the
execution `getPayload` response in milliseconds, so this is not evidence that
the EL spent 1.1 seconds calculating the payload. It leaves response delivery
or client scheduling inside the BN as the missing interval. The later 39.87
packing cancellation (`beacon.log:701`) belongs to parallel work after the
terminal response and did not cause it.

### Slot 3, node124

Activation maps the proposer pubkey to validator 75976 at
`validator.log:202`; its schedule is at line 652. The VC failed before asking
for a block: `signRandaoReveal` could not obtain domain data before the 48.00
deadline (`validator.log:796`). Node124's EL had already started and updated
the proactively prepared slot-3 payload at 31.601–31.603
(`execution.log:95–96`). Therefore this failure is neither an EL payload-build
failure nor a BN `GetBeaconBlock` terminal error.

## What is proved, and what remains inferential

### Production admission bound

The historical sync service registers each concrete gossip topic validator
without validator options (`beacon-chain/sync/subscriber.go:469`). With the
resolved `go-libp2p-pubsub` v0.17.0 defaults, that admits at most 1,024 active
validations per topic and 8,192 across all topics; Prysm sets the validation
input queue to its configured P2P queue size (600 by default)
(`beacon-chain/p2p/pubsub.go:161–177`, `beacon-chain/p2p/config.go:15,73–82`,
and pubsub `validation.go:13–17,126–133,360–372,478–486`). A 2,500-vote
single-subnet probe can therefore exercise the production 1,024-way per-topic
limit if its arrival rate outruns completions, but cannot exercise the
cross-subnet 8,192-way global maximum. Queue-full and validation-throttled
counts are needed to know its actual admitted concurrency.

This applies to remote gossip. Self-published attestations return before the
expensive path, and slot-0 attestations are ignored before state lookup
(`validate_beacon_attestation.go:50–60,87–89`). Remote slots 1–3 must first
pass time, duplicate, block/state, fork-choice, and consistency gates; those
that do reach `validateUnaggregatedAttTopic` perform the genesis-state active
validator count (`validate_beacon_attestation.go:92–168,251–294`).

The loaded reproduction also closes the fork-choice lock chain. Each accepted
validation calls `AttestationTargetState`, which takes the fork-choice `RLock`
and holds it through all of `getAttPreState`
(`beacon-chain/blockchain/receive_attestation.go:42–53`). `getAttPreState` then
takes a multilock keyed by checkpoint root plus round *before* checking the
checkpoint-state cache, including on a warm cache hit
(`process_attestation_helpers.go:104–124`). Consequently callers for the same
genesis checkpoint acquire the fork-choice read lock and then serialize behind
one async lock while still holding the read lock:

```
FFG validation
  -> fork-choice RLock (held)
  -> checkpoint-root/round async.Lock (thousands serialize)
  -> warm cache lookup and return
  -> async.Unlock/Clean
  -> fork-choice RUnlock
```

The `+25s` goroutine profile puts 2,436 goroutines in
`AttestationTargetState -> getAttPreState -> async.(*Lock).Lock`. That stack is
reachable only after the outer `RLock` succeeds, so these are existing
fork-choice readers, not goroutines waiting to become readers. An earlier
snapshot splits 1,447 such holders between `async.Lock` (626) and
`async.Unlock/Clean` (821). A pending fork-choice writer must wait for this
serialized holder cohort to drain; Go's writer preference then blocks new
readers, including proposal `CachedHeadRoot` and later validation
`InForkchoice` calls. Instrumented `UpdateHead` writers waited up to 21.464 s
but, once acquired, held the write lock for at most 30.043 ms in this window.
This is a finite reader/writer convoy, not a deadlock, and it resolves as the
same-key cohort exits.

`ActiveValidatorCount` runs only after `AttestationTargetState` returns, so it
does not itself hold the fork-choice lock. Its measured CPU saturation delays
multilock handoff and reader exit, amplifying the convoy. Warm checkpoint-cache
status avoids state regeneration but does not avoid the unnecessary
serialization because the multilock precedes the cache lookup.

The round1 node81 snooper logged the slot-2 `engine_getPayloadV6` response in
3 ms with a 2,060-byte body (`snooper-engine.log:14906–14917`). The snooper
identifies itself as rpc-snooper v0.0.21 at git `e3bb3cb`. In that exact source,
the duration ends after `io.Copy` has copied the upstream body into Go's
`http.ResponseWriter`; it does not prove that the buffered response reached or
was decoded by the BN (`snooper/proxycall.go:227–238`). Deferred request and
response logging starts goroutines and does not synchronously delay handler
return (`snooper/logging.go:130–163`), so there is no deterministic logging
tail that explains the timeout. The observation rules out slow EL payload
calculation much more directly than it rules out proxy flush, runtime
scheduling, body read, or decoding before the BN's 300 ms client deadline.

The exact go-ethereum v1.17.5 HTTP RPC path also has a cancellation race worth
distinguishing from an EL error. `sendHTTP` decodes the JSON-RPC envelope and
queues it on a buffered response channel before `CallContext` selects between
that channel and `ctx.Done()` (`rpc/http.go:189–203`, `rpc/client.go:146–159,
345–360`). If the deadline becomes ready before that select runs, the select
may return the context error even though the envelope is already queued. This
is a source-supported candidate, not proof that node81 took that path. Prysm
maps an error to the literal `timeout from http.Client` only when the error
implements `Timeout() == true`; arbitrary or unknown EL errors instead retain
their JSON-RPC error or are wrapped as unexpected responses
(`beacon-chain/execution/jsonrpc_error.go:34–53,96–107`).

The instrumented E1 reproduction did not reproduce node81's final-leg
failure. Its only transported `GetPayload` completed in 2.625 ms: request write
at 14:04:21.149425828, first response byte at 14:04:21.150422352, and complete
RPC return at 14:04:21.150726845. Slots 2 and 3 failed before an execution
request was transported; slot 3's already-canceled call returned in 0.493 ms
without `GetConn`, `WroteRequest`, or first-byte events. E1 therefore confirms
a healthy reached path. A later engine-probe run did reproduce the missing
shape once in 59 diagnostic reads: the proxy obtained and copied a complete
HTTP 200 result in 0 ms, while the BN observed its first response byte 1.774
seconds after writing the request and returned its nominal 300 ms timeout after
1.778 seconds. That run and its endpoint boundary are documented in
[`engine-response-results.md`](engine-response-results.md). It strongly
supports the same CPU-pressure mechanism for node81, but the historical logs
still lack packet-level or client-trace proof of the exact final-leg stage.

The direct terminal mechanisms differ: two round1 proposals reached block
construction, while round1 slot3 and every recovered round2 proposal reported
a RANDAO domain lookup failure. For round1 slot3, round2 slot2, and round2
slot3, that RANDAO error is secondary: synchronous `RolesAt` sync-committee
selection had already exhausted the shared slot context, logged its error, and
then returned the role map so proposer dispatch ran with the expired context
(`validator/client/runner.go:152–164`, `validator/client/validator.go:649–660`).
The preflight/RANDAO ordering is `validator.log:791,796` on round1 node124;
01:30:36.001289585 then 01:30:36.004480876 on round2 node191; and
01:30:48.000619225 then 01:30:48.003364340 on round2 node22. Round2 node169
has no corresponding preflight error for slot1; its later sync-index error is
after RANDAO and comes from role execution, so its RANDAO cause remains open.
The repeated deadline behavior across independent
nodes is consistent with the measured genesis-FFG registry-scan amplification
and common CPU pressure. It is not a historical scheduler profile and does not
prove that every individual delay was spent in that scan.

There is also an unresolved VC-side caveat. In the historical source,
`validator.domainData` takes `domainDataLock` exclusively on a cache miss and
holds it across the BN `DomainData` RPC (`validator/client/validator.go:721–755`).
Consequently a RANDAO deadline does not distinguish BN RPC latency from waiting
behind another VC domain-cache miss. The source-root/domain-cache mutex path is
being audited separately; no causal share is assigned to it here.

### Sync-committee preflight lock audit

The E1 early-gossip reproduction now supplies the missing stage timing; see
[`early-gossip-results.md`](early-gossip-results.md).  A real slot-1 VC
sync-index request hit the warm sync-head cache after 8.887 ms but did not
return for another approximately 4.795 seconds, with the remainder occurring
inside the deferred `async.MultiLock` unlock/cleanup path.  Concurrent
goroutine profiles place 1,583 goroutines in `async.Clean` and later 1,513 in
`async.(*Lock).Lock`.  This proves cross-key contention through the global
multilock manager in the reproduction.  It reproduced a 5.244-second total
preflight, not the historical full-slot timeout, and does not settle round2
node169 or the historical execution response path.

The synchronous `RolesAt` preflight collects every sync-committee key and
calls `SyncCommitteeAggregators` before `performRoles` dispatches any proposer
(`validator/client/validator.go:622–653`, `validator/client/runner.go:147–156`).
On the BN, sync-committee state lookup first takes a process-global async
multilock keyed only by requested slot, and takes it *before* checking the
size-one warm cache (`beacon-chain/blockchain/head_sync_committee_info.go:135–148`).
All same-slot requests therefore serialize even after the first request has
populated the cache. Only the first miss calls `HeadState`, `HeadRoot`, and
`ProcessSlotsUsingNextSlotCache`; queued warm hits do not repeat those steps.

`HeadState` and `HeadRoot` take `Service.headLock.RLock`, not the fork-choice
lock (`beacon-chain/blockchain/chain_info.go:185–207,238–252`). `HeadState`
copies the state while that read lock is held (`beacon-chain/blockchain/head.go:296–304`),
but releases it before `HeadRoot` and slot processing. Conversely `saveHead`
documents that its caller already holds fork choice and then reads and
eventually writes `headLock` (`beacon-chain/blockchain/head.go:58–61,82–98,227–245`).
This establishes fork-choice-to-head ordering for head saving, but the audited
sync-state path never acquires fork choice while holding `headLock`. It is not
evidence of a head/fork-choice lock inversion. Its direct risk is the same-slot
async-lock queue: `RolesAt` can consume the slot deadline waiting for a warm
entry, preventing proposer dispatch until the shared context is already
expired.

There is also cross-key coupling inside `async.MultiLock`. Its channel registry
and `Clean` operation use one package-global lock; every multilock unlock runs
`Clean` while holding that global lock (`async/multilock.go:22–28,56–67,92–110`).
The genesis checkpoint validators and `syncHeadState-<slot>` use different
logical keys but the same registry. Run B's profiles contain large cohorts in
`async.(*Lock).Lock` and `Unlock`, but no sampled stack retaining
`getSyncCommitteeHeadState`, so they do not prove whether the historical
preflight was stopped on the global registry, its same-slot key, `headLock`, or
the first miss's slot processing. Stage-level timing is required to divide
those alternatives.

Round2 node169 has no earlier `RolesAt` preflight timeout, so this queue does
not directly explain that slot. Its RANDAO request can still have waited on the
VC-global `domainDataLock` behind another cache miss, or on the BN RPC/runtime
path, but the available log has no boundary timestamp that divides those
alternatives.

The two summary lines must not be interchanged when applying this evidence.
`FFG votes` counts accepted beacon-attestation gossip by slot and subnet
immediately before the validator returns `ValidationAccept`
(`beacon-chain/sync/validate_beacon_attestation.go:240–246`,
`beacon-chain/sync/ffg_summary.go:18–29,105–119`). `Goldfish votes` instead
reports the fork-choice store's available-attestation voters and seats
(`beacon-chain/forkchoice/doubly-linked-tree/goldfish.go:215–229`). Therefore
a zero Goldfish summary bounds available-attestation head votes only; it says
nothing about how many FFG beacon attestations completed validation. E1's
15,000 `FFG votes` and zero `Goldfish votes` per loaded slot are the expected
combination because its source sent only the former message family.

Node169 does provide a broader process-level discriminant. At the slot-1
deadline, independent RPC families fail in the same 15 ms burst: payload
attestation data at 24.001, attestation data and sync-message head root at
24.002, proposer RANDAO at 24.002452, sync-subcommittee index at 24.006, and
aggregate selection/remaining attestations through 24.016
(`validator.log:687–737`). The VC `domainDataLock` cannot be the common cause
of that burst because it protects only domain requests. This supports a
BN-wide servicing, scheduling, or shared-lock stall, while leaving the
RANDAO request's own division between VC lock wait and BN RPC unresolved.
The BN log is silent between its 12.117 slot tick/reorg and 26.891 next reorg
(`beacon.log:516–519`), so strong evidence of a process-wide delay but not a
stage timer. Absence of a successful `RolesAt` log also means its completion
time is unbounded; no preflight *error* is not proof that preflight was fast.

### Round2 summary-log discrepancy

The recovered node169 archive is continuous from startup through shutdown and
contains ordinary INFO logs (including `PM0280` graffiti), but no `Goldfish
votes`, `FFG votes`, or `purpose=goldfish-summary` line; the same search is
empty for recovered round2 nodes 191 and 22. This is unlike round1 nodes 81
and 124, which emit the Goldfish line at each early boundary. Source says
`SummaryActive` is true at or after the Heze fork and `NewSlot` emits the
Goldfish summary (`decoupled/vote_ledger.go:104–107`,
`beacon-chain/forkchoice/doubly-linked-tree/on_tick.go:32–39`). Because other
INFO lines and the full time range survived, the archives provide no evidence
for an external log-level filter selectively removing these messages. The
runtime/configuration or built-source discrepancy remains unexplained, so
absence of the round2 summary lines is not used as evidence that no votes or
no slot ticks occurred.
