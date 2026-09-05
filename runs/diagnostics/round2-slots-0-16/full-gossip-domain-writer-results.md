# Full-gossip natural-writer / DomainData composition

## Result

This discriminator adds one real cold slot-one `GetAttestationData` call to the
coherent full-gossip fixture. It runs concurrently with the first DomainData
probe over the same warmed gRPC connection after the real cold sync-index
prerequisite. The native shared-MVS arm produced a 6.856-second successful
DomainData call at slot-one +3.001 seconds. The stable-snapshot arm completed
all 48 calls below 3.299 ms. This is the first bounded one-BN composition to
reproduce a multi-second delay in an unrelated cheap RPC while the real FFG
validation pipeline is active.

The long call still returned 2.143 seconds before the absolute slot-two
deadline, so it does not reproduce node169's missing slot-one RANDAO response.
It shows that native shared-registry validation can create a seconds-scale
general servicing pause. The one early `HeadState` copy itself took only 0.193
ms and returned with seven active checkpoint iterators; this run does not show
that the copy created the later cohort of 1,324 iterators.

## Coherent real-work construction

Both arms use the same 120,000-validator chain, 15,000 valid slot-one FFG
messages, six real persistent GossipSub pipelines, production batch-verifier
limit, actual checkpoint/count path, feed and pool described in
[`full-gossip-domain-realwork-results.md`](full-gossip-domain-realwork-results.md).
This variant corrects two prerequisites that the older Domain-only pair did
not need:

- Altair, Bellatrix, Capella, Deneb, Electra, Fulu, Gloas and Heze all activate
  monotonically at epoch zero; the constructed state reports Heze and its
  attester domain matches the config-derived domain.
- At the real slot-one boundary, fork choice runs its normal locked `FullHead`
  selection over the one existing genesis node. The selected and public
  `CanonicalNodeAtSlot(1)` roots must equal the cached genesis head and report
  payload-full before load.

A separate temporary-cache preflight then calls the actual public
`rpcvalidator.Server.GetAttestationData` with the real core service and chain.
It must return slot one, payload index one, the exact genesis head/target roots,
the zero justified source, and populate its initially cold cache. It took 0.449
ms in the native arm and 0.297 ms in the snapshot arm. The measured server uses
a different initially cold cache, so the preflight does not memoize timed work.
The disclosed preflight performs one pre-load head-state copy and consumes the
absolute slot budget.

After the first real checkpoint iterator starts, the coordinator runs the cold
`HeadSyncCommitteeIndices(validator=0, slot=1)`. It then releases one cold
`GetAttestationData(slot=1, committee=0)` RPC and probe zero concurrently. Both
use the same child process, `grpc.ClientConn`, and absolute slot-two deadline.
The validator-client production cache serializes post-Electra attestation-data
misses, so one underlying call represents the 75 original attesters more
faithfully than 75 parallel calls.

The core service delegates every dependency to the real chain. A narrow test
wrapper timestamps the complete `HeadState` delegate call; it does not modify
the returned state or hold a lock. The measured cache is verified cold before
release and populated after a successful, fully drained server call.

The child schedules 48 sequential DomainData probes on the absolute slot-one
grid at 250 ms intervals. All share the slot-two deadline. If a prior RPC spans
a scheduled tick, that tick is recorded as skipped instead of being issued in
a catch-up burst. Successful responses must equal the warmed RANDAO domain;
deadline outcomes would be retained as measurements rather than fixture
failures.

There are no injected work delays or held locks. The primary arms use the
lightweight atomic/pool-count interceptor, but no stack dump, profile, or
runtime trace.

## Primary measurements

Both arms used `runtime.NumCPU=16` and `GOMAXPROCS=4`.

| Measurement | Native shared MVS | Stable snapshot |
|---|---:|---:|
| Published / publish errors | 15,000 / 0 | 15,000 / 0 |
| Entered / accepted / stored | 6,416 each | 7,688 each |
| Ignored / rejected / subscriber failed | 0 / 0 / 0 | 0 / 0 / 0 |
| Peak active validator iterators | 1,324 | 6 |
| Publication loop elapsed | 8.188 s | 2.000 s |
| Maximum publication lateness | 6.225 s | 5.261 ms |
| Release to settled | 24.189 s | 10.401 s |
| Cold sync-index | 0.333 ms | 1.549 ms |
| Timed GetAttestationData client | 43.135 ms | 2.799 ms |
| GetAttestationData invoke to admission | 34.459 ms | 1.510 ms |
| GetAttestationData admission-to-return upper bound | 0.227 ms | 0.178 ms |
| Exact `HeadState` delegate | 0.193 ms | 0.146 ms |
| GetAttestationData recorded return-to-client tail | 8.449 ms | 1.111 ms |
| Probe grid / invoked / OK / skipped | 48 / 16 / 16 / 32 | 48 / 48 / 48 / 0 |
| First DomainData | 34.820 ms | 2.299 ms |
| Invoked DomainData p50 / p95 / max | 8.011 / 578.654 / 6,855.839 ms | 0.770 / 2.294 / 3.298 ms |
| Maximum invoke-to-admission | 4,781.042 ms (probe 12) | 3.147 ms (probe 8) |
| Maximum admission-to-return upper bound | 0.015 ms | 0.039 ms |
| Maximum recorded return-to-client tail | 2,074.794 ms (probe 12) | 1.507 ms (probe 21) |

The deterministic join is
[`full-gossip-domain-writer-comparison.tsv`](full-gossip-domain-writer-comparison.tsv),
generated by
[`analyze_full_gossip_writer.py`](analyze_full_gossip_writer.py).

Probe 12 is the central result. It was planned for slot-one +3.000 seconds,
invoked at +3.000912, admitted by the server at +7.781954, and returned to the
client at +9.856751. Its exact decomposition is:

- client invoke to server admission: 4.781041858 seconds;
- recorded admission to handler return: 3.566 microseconds;
- recorded server return to client: 2.074793723 seconds.

At admission, 6,151 validations had started but only 50 had completed. There
were 1,720 iterator entries and 431 exits, leaving 1,289 active; the recorded
peak was already 1,324. The handler-body bound remained microseconds. This
places the multi-second delay on both sides of the cheap handler and ties its
largest observed point to the native checkpoint-count cohort.

The interceptor records admission before its lightweight atomic and pool-count
snapshot, so admission-to-handler-return is an upper bound that includes that
observer work. It records handler return before the post-return stats snapshot
and record append, so the recorded return-to-client tail also includes those
diagnostic operations. Invoke-to-admission is unaffected by either boundary.

The publication producer was delayed too: its source schedule offers all
15,000 messages over two seconds, but the native loop took 8.188 seconds and
reached 6.225 seconds of lateness. Thus this arm did not deliver the intended
arrival cadence after the pause began. The producer delay shows that another
concurrent operation was delayed, but it can include GossipSub `Publish`
backpressure as well as goroutine scheduling. It limits any claim that the
later load matches the prescribed two-second cadence.

## Trace repetition

One separately gated native repetition recorded a runtime trace after the
untraced pair established the positive. It did not repeat the 6.856-second
call: peak active iterators was 110, 38 probes ran, 10 were skipped, and the
maximum was 850.217 ms at probe 14. Eight calls exceeded 100 ms. Publication
took 2.400 seconds with 488.433 ms maximum lateness.

The repetition is supporting localization evidence, not a replacement or
timing replicate for the primary run. The raw 4.7 MB trace is
[`runtime.trace`](full-gossip-domain-writer-go-evidence/trace-shared/runtime.trace)
(SHA-256
`5e67d6e68ba55d668f4fa2b9a381d7b4c5fb2474e3f2741b4b0fdf6b123ad0cf`).
Its Go 1.26 parsed expansion is retained locally beside it but is not tracked
because it is 781 MB; focused transitions should be derived from the raw trace.

Probe 14's 850.217 ms call supplies that supporting localization. The server
HTTP/2 reader became runnable 144.207 ms before client invocation but did not
run until 286.260 ms after invocation. The trace does not timestamp packet
arrival, so the full runnable interval is not an RPC-read latency. Once
scheduled, the reader created the handler; its goroutine ran and exited within
48.128 microseconds, enclosing the recorded admission and handler return.

The server loopy writer was first made runnable by the reader's
`handleWindowUpdate` at +286.285 ms, rather than by the later handler response.
It first ran at +513.802 ms, called `runtime.Gosched` 0.137 ms later, remained
runnable for another 336.047 ms, and resumed at +849.985 ms. The socket write
took 56.576 microseconds and the client returned 0.167 ms later. From the
handler's exit, the two writer runnable waits total 563.455 ms. The complete
48.128-microsecond handler lifetime also bounds the interceptor's stats and
record operations in this repetition. Thus the trace places almost all of the
observed delay in HTTP/2 reader and writer scheduling around a microsecond
handler. It does not explain the untraced primary's longer 4.781-second
pre-admission and 2.075-second post-handler intervals.

The exact joined phases are in
[`full-gossip-domain-writer-trace-probe14.tsv`](full-gossip-domain-writer-trace-probe14.tsv).
The compact
[`probe14-focused-transitions.txt`](full-gossip-domain-writer-go-evidence/trace-shared/probe14-focused-transitions.txt)
retains the reader, writer and handler events, both wall-clock sync anchors,
and one nearby `ActiveValidatorCount` preemption stack on each of the four Ps,
with original parsed-line provenance. No stack sample exists at the long
boundaries, so these preemption events show concurrent count work but do not
measure continuous CPU occupancy.

The source join and complete transition accounting are documented separately
in [`full-gossip-domain-writer-trace-audit.md`](full-gossip-domain-writer-trace-audit.md).
The complete interval census in
[`full-gossip-domain-writer-count-trace-audit.md`](full-gossip-domain-writer-count-trace-audit.md)
classifies 174 of 177 preemptions during the three transport waits as actual
`ActiveValidatorCount` execution, with Count preemptions on all four Ps in
each wait. These interrupted execution stacks directly place count work while
the transport goroutines are runnable; they do not estimate exclusive CPU
share.

## Interpretation and limits

The native and snapshot arms differ only in the iterator returned from the real
immutable checkpoint state. The snapshot still traverses the same 120,000
stable validator records and runs every predicate. It removes native
`validatorsMultiValue` iterator access from gossip count callers while the
actual head-state copy continues to use the shared native state. The arm
difference therefore isolates shared native registry access and its scheduling
effects, rather than eliminating the O(120,000) count loop or other gossip
work.

The actual early natural writer is too short and too early to explain the later
cohort by itself. Its server admission saw seven active iterators and its
`HeadState` delegate returned in 0.193 ms. The primary establishes that the
source-specific full composition can produce a seconds-scale pause; it does
not establish that one head copy is necessary or that it directly collected
the 1,324 callers. The coherent fork epochs and fork-choice head selection also
differ from the immutable P16/P4 pair, so those older arms are not a matched
no-writer control for this question.

The 8,584 native and 7,312 snapshot publications absent from
`ValidationStarted` were successful sender `Publish` calls not observed by the
validation wrapper. No raw GossipSub tracer was installed. As before, this is
an end-to-end loss without a precise pubsub queue attribution.

This remains one BN plus one lightweight publisher in one process. The retained
E1/H configurations used four slots per round and six attestation subnets,
while this one-slot fixture uses eight slots per round and the default subnet
configuration with six exercised topics. The retained sources sampled roughly
1,250 votes from each of 12 committees mapped onto those six topics; this
fixture uses all 2,500 members from each of six committees. It omits the
retained reproductions' three-BN network, VC streams, execution traffic, and
other validator RPCs.

The 15,000 valid publications are a controlled offer count. In retained E1,
15,000 denotes source RPC submissions, while BN3's slot-one summary observed
2,336 accepted validation completions by its approximately six-second read.
The corrected source, receiver and summary accounting is in
[`ffg-receiver-acceptance-audit.md`](ffg-receiver-acceptance-audit.md); none of
these values is an exact historical slot-one acceptance total.

A real validator client caches the first successful RANDAO domain response and
would not issue 48 repeated calls in one slot. The probe grid is an explicit
servicing observer used to locate transient BN availability; only the first
call composes the source-like role dispatch.

## Artifacts and source identity

- Commands: [`command.txt`](full-gossip-domain-writer-go-evidence/command.txt)
- Hash manifest: [`sha256sums.txt`](full-gossip-domain-writer-go-evidence/sha256sums.txt)
- Native primary: [`shared/summary.json`](full-gossip-domain-writer-go-evidence/shared/summary.json),
  [`shared/client.jsonl`](full-gossip-domain-writer-go-evidence/shared/client.jsonl),
  [`shared/server.jsonl`](full-gossip-domain-writer-go-evidence/shared/server.jsonl),
  [`shared/attestation-data-client.json`](full-gossip-domain-writer-go-evidence/shared/attestation-data-client.json),
  [`shared/head-copy.json`](full-gossip-domain-writer-go-evidence/shared/head-copy.json), and
  [`shared/full.test.log`](full-gossip-domain-writer-go-evidence/shared/full.test.log)
- Snapshot primary: [`snapshot/summary.json`](full-gossip-domain-writer-go-evidence/snapshot/summary.json),
  [`snapshot/client.jsonl`](full-gossip-domain-writer-go-evidence/snapshot/client.jsonl),
  [`snapshot/server.jsonl`](full-gossip-domain-writer-go-evidence/snapshot/server.jsonl),
  [`snapshot/attestation-data-client.json`](full-gossip-domain-writer-go-evidence/snapshot/attestation-data-client.json),
  [`snapshot/head-copy.json`](full-gossip-domain-writer-go-evidence/snapshot/head-copy.json), and
  [`snapshot/full.test.log`](full-gossip-domain-writer-go-evidence/snapshot/full.test.log)
- Trace repetition: [`trace-shared/summary.json`](full-gossip-domain-writer-go-evidence/trace-shared/summary.json),
  [`trace-shared/client.jsonl`](full-gossip-domain-writer-go-evidence/trace-shared/client.jsonl),
  [`trace-shared/server.jsonl`](full-gossip-domain-writer-go-evidence/trace-shared/server.jsonl), and
  [`trace-shared/full.test.log`](full-gossip-domain-writer-go-evidence/trace-shared/full.test.log)

The unprofiled, untraced primary pair's source is preserved at jj revision
`fab0471490a4`. At that revision, the internal fixture helper SHA-256 is
`695d2f8068620b53fb3f407e97ea4738fa9e7e2f155f46f43136a26a646c9766`
and the TCP coordinator SHA-256 is
`10d2387c4174d0d8513fe715275099462d67f84e4b3236f3c3fdf42a53e31711`.
The trace-gated coordinator has a different hash and is associated only with
the trace repetition: its SHA-256 is
`324ce90b773d662d3161221829b91c0e2fafce8720dc9936d569cf4e6ccdd6bc`.
The helper was unchanged from the primary pair.

Three development attempts are retained under the evidence directory and are
not controls: `shared-index-assertion-pilot` completed with an unselected
fork-choice head and incomplete fork gates; `shared-incoherent-head-aborted-pilot`
was stopped once that issue was identified; and
`shared-electra-gate-assertion-pilot` selected the head but left Electra at its
mainnet epoch, so the API correctly took its pre-Electra index branch. None is
used in the result above.
