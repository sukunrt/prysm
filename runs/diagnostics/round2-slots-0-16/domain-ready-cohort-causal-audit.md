# DomainData: investigation of the count cohort

This document preserves the reasoning behind the earlier bounded controls.
The subsequent [full-gossip composition](full-gossip-domain-writer-results.md)
reproduced a 6.856-second ordinary RPC; its separate trace locates both reader
and response-writer scheduling delays. The early attestation-data state copy
was short and did not itself demonstrate formation of the later large cohort.
The [full-trace join](full-gossip-finalizer-cohort-trace-audit.md) subsequently
located a natural finalizer that wakes 101 Count readers and traced their
preceding checkpoint handoffs. A
[matched timed-request omission pair](full-gossip-domain-no-timed-attdata-results.md)
stayed fast in both arms and did not identify that request as the trigger.
Treat the proposed discriminants below as investigation history, not pending
work or proof that the timed attestation request caused that cohort.

Subsequent retained-trace analysis located an ordinary DomainData mechanism:
H's actual VC RANDAO RPC overlaps 133.254 ms of a 135.385 ms runnable delay in
its BN HTTP/2 connection reader. That reader creates the matching DomainData
handler after resuming. All 53 CPU samples during the exact reader interval
are in actual gossip Count work across all four Ps. See
[the reader trace results](domain-http2-reader-trace-results.md) for the
parent/child linkage, I2 counterfactual, and packet-arrival qualification.
The paced, cold-sync, and singleton-aggregation controls remained negative for
seconds-scale Domain delay; their nulls do not erase this full-service trace.
The analysis below records why those bounded compositions were investigated.

The later full-gossip fixture also preserves `StartFromSavedState`/`Resume`
and native state finalizers. Its controlled cold sync call occurs at only one
or two active count iterators in the P4 arm (88.136 µs), so it does not exercise
the E1 writer's later arrival amid a large accumulated cohort. A cold public
`GetAttestationData` call was the most directly observed omitted writer path
when this control was designed.
It reaches `HeadState.Copy` after its own per-slot attestation-cache miss;
for slots 1–3 at round zero it need not advance the head through a new round.

The ordinary DomainData body is cheap, but the existing null TCP experiment
does not reproduce the distribution of work observed in the full beacon node.
Its 6,144 offered checkpoint/count workers generally leave only three count
calls active while the rest wait in checkpoint lookup. Retained E1 profiles
show a different, evolving distribution: hundreds of count calls can accumulate
behind a validator-storage writer, and hundreds of scans plus over a thousand
BLS aggregation workers later appear outside parked stacks. This gives a
concrete condition to test before attributing the missing RANDAO delay to an
unobserved VC cache holder or transport feature.

This audit identifies the omitted condition. It does not turn a goroutine
profile into a trace of node 169's historical RANDAO call.

## Retained evidence

Profiles are under
`/tmp/prysm-startup3-round4-early-e1/profiles/`. The counts below were recovered
with Go 1.26.5 `go tool pprof -top` and `-traces`, focusing separately on
`ActiveValidatorCount` and `coreAggregate.func1`.

| Profile | Count stacks | Parked count stacks | Other count stack locations | BLS worker locations |
| --- | ---: | --- | --- | --- |
| `bn-goroutine-plus-37.pb.gz` | 919 | 910 at validator MVS `Len` RLock; 2 at MVS `At` RLock | 7 at count/validator lookup computation | Not classified here |
| `bn-goroutine-plus-43.pb.gz` | 286 | 48 at validator MVS `Len` RLock | 238 at count/validator lookup computation | 1,059 P1 workers at `Gosched`, 12 at P1 worker code, 16 P2 workers at `Gosched` |

The +43 profile has 7,574 total goroutine samples. Its 1,071 P1 worker stacks
are separate from the aggregation parents: 1,069 parents reach public-key
aggregation through `AggregateKeyFromIndices` and attestation signature-batch
construction. These counts describe sampled stack locations. In particular,
919 cumulative Count stacks at +37 do **not** mean 919 runnable scans, and the
profile is not an instantaneous runtime-state census.

The +37 profile also contains a writer waiting in
`GetAttestationData → HeadState → BeaconState.Copy → validatorsMultiValue.Copy
→ RWMutex.Lock`. The writer's eventual release can wake queued MVS readers;
the snapshot alone does not identify the release time or pair each reader with
that particular writer. The shared-storage identity and the corresponding
cold sync-state copy are checked in the
[shared-storage audit](checkpoint-count-cohort-shape-audit.md).

The +43 profile request overlaps the 4,271.995 ms DomainData probe. The separate
2,165.041 ms DomainData envelope lies between the +37 and +43 profile requests.
The profile collection can affect scheduling, and continuous CPU profiling was
also active. See [the timing audit](domain-data-causal-audit.md). The retained
profiles establish a workload difference, not observer-free causation of those
latencies.

## An actual omitted source stage

After checkpoint lookup and the active-validator count, successful gossip
validation constructs an attestation signature batch:

`validateCommitteeIndexBeaconAttestation → validateUnaggregatedAttWithState →
AttestationSignatureBatch → createAttestationSignatureBatch →
BeaconState.AggregateKeyFromIndices → bls.AggregatePublicKeys`.

For a valid single-attester vote, the indexed attestation contains one signer.
`crypto/bls/blst/public_key.go:77` nevertheless uses the generic aggregation
API. There is no singleton shortcut. In BLST v0.3.16,
`bindings/go/blst.go:1595` selects one worker for one input, starts a goroutine,
converts the affine key through C, explicitly calls `runtime.Gosched`, and then
sends its buffered result. The caller waits for that result. The encompassing
native `AggregateKeyFromIndices` retains the state read lock until aggregation
returns.

The deployed revision `0280403c70d88967f49d2d4c730f4c5417dabdf5` uses the same
BLST v0.3.16. The relevant native getter, signature-batch construction, BLST
wrapper, and gossip batch-verifier files have no diff from that revision in
this checkout. The singleton worker is therefore not a diagnostic-only path.

The worker's yield permits other runnable work to execute. It does not release
the enclosing pubsub validation capacity: the validation parent remains inside
the call until aggregation and verification finish. Treating the yield as
permission for a new gossip admission would be incorrect.

## Bounded discriminants

The first control should retain the real checkpoint/count chain and change
its offered arrival shape from one simultaneous release to 15,000 jobs paced
over two seconds, with the same maximum 6,144 workers. This matches the retained
source's offered window without a synthetic held lock. Measure actual offered
times, actual job admissions, count entries/exits, and maximum active counts.
An in-process timer can itself run late under load; a null must be interpreted
using those measured times.

The completed paced control is also negative for seconds-scale Domain delay:
32/32 scan-arm calls succeeded, with maximum 12.487 ms versus 0.728 ms in the
memoized arm. All 15,000 scan jobs completed. The last offer was approximately
2.001405 s after release and maximum recorded offer lateness was 8.701929 ms,
so grossly stretched offers do not explain the null. Maximum active count calls
reached 58, but actual server admissions sampled only 1–6 active count calls;
the inspected 100 ms offer-bin endpoints consistently show three. Pacing alone
did not sustain the large cohort seen in the full BN. Raw results are in
`domain-data-tcp-paced-go-evidence/`. These point samples also motivate early
clock-based coverage in the next composition instead of relying solely on
completed-job thresholds.

If its Domain calls remain fast, a real cold first-slot sync-index lookup is
the most direct next predecessor to test. A cache miss calls `HeadState.Copy`
before advancing and caching the state. It can introduce the actual MVS writer
that the simple count control lacks. Invoke ordinary DomainData immediately
after that public sync method returns and record the shared absolute slot
budget. Preserve a genuinely cold slot-one sync cache, verify shared validator
storage, and add no manually held writer or interposed sleep. A successful
slot-zero lookup does not warm the separately keyed slot-one cache; this does
not prove that node 169's own caller was the historical first filler.

For node 169 specifically, another role has already progressed by slot
`+9.915067414 s`. That marker has `2.084932586 s` left before the absolute
deadline; it does not timestamp RANDAO invocation or establish its remaining
budget. A useful positive result
is successful preflight followed by a slow own Domain RPC with a live budget;
record whether preflight finished by that observed bound. A cold sync call that
alone crosses the deadline reproduces expired preflight, not the missing
slot-one mediation, and must be reported separately. A real concurrent
attestation-data request after role dispatch supplies another natural
`HeadState.Copy` writer if the preflight copy does not produce the missing
condition.

Actual singleton aggregation is another precisely identified difference if
needed. Adding the unchanged production `AggregatePublicKeys` call after each
real checkpoint/count job tests that scheduling interaction. A pre-generated,
valid repeated public key warmed before measurement isolates the worker/yield
cost, while omitting distinct-key cache misses and the enclosing native-state
read lock. Those omissions must be explicit. Using real
`AggregateKeyFromIndices` with validator keys installed before fixture/cache
construction would preserve the enclosing lock too. No production change or
larger arbitrary workload is required for either control.

Any positive primary should run without snapshots, CPU profiling, or runtime
tracing. Only a subsequent matched trace should attribute the slow RPC to a
server reader, handler, writer, or runnable delay. The full-service count-removal
counterfactual remains the evidence for the common startup overload cause. The
subsequently recovered H trace above now identifies one concrete ordinary-RANDAO
transport mediator in that full service.

## Bounded alternative-writer and initialization check

The already decoded H slot-one slice contains real native finalizer cleanup,
so it need not be invented as an extra stressor. G6 first waits on balances
storage (`finalizerCleanup`, instrumented `state_trie.go:1741`). It later
releases the **validator** storage writer (`:1747`) and directly wakes Count
readers G7830 and G7869. Their recorded `At → RLock` waits are only 8.320 µs
and 2.688 µs. G6 returns to system wait by trace 192182246426240, approximately
15:11:03.119883 UTC, 33 ms before the known Domain connection-reader runnable
interval begins. The [exact finalizer/reader transitions](domain-h-natural-finalizer.transitions.txt)
preserve the distinct balances and validator call sites. This is a concrete
small writer interaction in the relevant startup, not evidence that a missing
or dominant finalizer caused H's longer reader delay. It is not a census of
all finalizers in E1 or H.

`Resume` also launches `populatePubkeyCache` (`state/stategen/service.go:166`).
That goroutine reads the validator MVS and writes a **different** mutex in the
BLS public-key cache; it is not a validator-MVS writer. Its completion marker
is Debug-level, so E1/H Info logs do not directly prove the exact completion
time. Both initialize head roughly 160 seconds before genesis (E1
`beacon3.log:30`, 14:01:16.39; H `:30`, 15:08:11.45), and neither reports key
population failure. The full-gossip fixture synchronously derives and caches
all 120,000 public keys before setting its genesis clock
(`sync/full_gossip_domain_export_test.go:239–261`), then executes the real
`Resume` path. It therefore preserves the initializer and makes the timed
signer keys warm; an exact completion join for Resume's redundant reader is
not present. These observations give no positive reason to add cold-key work
or manual finalizer calls to reproduce the E1 writer. The retained E1
`GetAttestationData → HeadState.Copy` stack was therefore the stronger
source-backed writer difference available when this control was designed.
