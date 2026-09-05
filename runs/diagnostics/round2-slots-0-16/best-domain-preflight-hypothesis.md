# Best-supported explanation of the domain/preflight failures

The [newly inspected H runtime trace](domain-http2-reader-trace-results.md)
now locates an ordinary RANDAO transport scheduling delay: its identified BN
HTTP/2 reader stays runnable for 135.385 ms while all 53 CPU samples execute
genesis counts. The matching RPC takes 139.232 ms versus 1.636 ms in I2's
BN3-only count ablation. This fills the ordinary-RPC mechanism gap discussed
below; it does not provide node 169's missing request timestamps. The newer
[paced, cold-sync and singleton controls](domain-data-paced-composition-results.md)
remain negative for reproducing that historical deadline.

The underlying startup defect was already identified by the earlier slot-1–3
investigation: repeated whole-genesis-registry counting overloads proposal
dependencies. This audit ranks how that established source of pressure can
reach the domain/preflight failures. The expensive checkpoint/sync dependency
is strongly supported. The additional delay on node169 in slot 1 is less
tightly located: full-process RPC admission, completion, or proposer scheduling
ranks ahead of a large proof-signing/domain-cache convoy. Slot 4 has a much
stronger ordering result and should not be treated as another measured long
RANDAO RPC. An unlogged internal wait does not erase the earlier common-cause
finding.

This is a ranked causal interpretation, not an assertion of a recovered
historical stack. Source comparisons against deployed revision
`0280403c70d88967f49d2d4c730f4c5417dabdf5` show no changes in the seven reviewed
VC files: `subnets.go`, `aggregate.go`, `aggregator_selector.go`, `runner.go`,
`duties.go`, `validator.go`, and `keymanager/local/keymanager.go`.

## The earlier established finding

[genesis_ffg_root_cause.md](../genesis_ffg_root_cause.md), introduced in jj
`0f1ddfdd206481efe26e8f9afcb54cdaff09044d`, identifies the cache-bypassing
120,000-validator scan and verifies that the relevant code applies to both
historical runs. The later full-service controls establish more than a
microbenchmark slowdown. [E1/F1](../startup3/early-gossip-results.md), introduced
in `a8d960ad2feb5e44426b6a5c1a221b255d0ea602`, changes three failed real proposals
into three successful proposals by removing the repeated counts under the same
45,000 offered and accepted source requests. F1 also removes the measured
sync/head queues. [H/I2](../startup3/wire-causation-results.md), introduced in
`86edfcade835ae184c779a04f285a323d0a0d66d`, repeats the full proposal recovery
while changing the receiving/proposing BN only, leaving vote origination and
relay unchanged, and traces delayed runnable HTTP readers.

These are strong causal controls for the shared startup defect. The cheap
DomainData TCP control below limits one proposed way that pressure reaches
node169; it does not refute the full-process counterfactual or reset slots 1–3
to an unknown root cause. The external earlier workflow's complete conclusion
was also reread at
`/tmp/claude-1000/-home-sukun-dev-prysm2/3bc6345b-1f70-4ae7-9333-641541e91da0/tasks/wn4c5njga.output`,
field `result.final`. Its common-cause diagnosis agrees with this evidence.
Its stronger claim that node169's own domain RPC lasted 12 seconds is not
required for that diagnosis. Nor does the 75-error attester burst require 75
concurrently outstanding requests; the later control-flow audit establishes
serialization through a separate attestation-data cache lock.

## Ranking and discriminating evidence

| Candidate | Assessment |
| --- | --- |
| Slot 4: late checkpoint-dependent sync preflight, then a cold signing-domain call loses the remaining budget | Leading explanation. The real warm sync method can spend more than 13 seconds in the checkpoint/count control; the ordinary domain body is cheap. The exact split before the historical sync-selection error is unlogged. |
| Slot 1: some budget consumed before dispatch, then additional delay in the real RPC envelope or scheduling of the proposer | Leading class, with lower confidence than slot 4. It fits the successful preflight followed by several role deadlines and the independent evidence of intermittent BN servicing delays. It requires an additional delay; late preflight alone is insufficient if the proposer runs promptly. |
| Slot 1: a different domain miss holds the VC's global domain-cache write lock after preflight | Possible amplifier, ranked below broader servicing delay. Source permits it, but no historical holder is identified and the same role batch supplies few candidates that reached signing. |
| Thousands of background selection signatures intrinsically consume the slot or cause thousands of serialized domain misses | Low ranking. Jobs reuse two ordered epoch/domain keys, the expensive signatures run outside the domain lock, and the measured larger E1 refresh finishes its domain-access phase quickly before qualifying gossip. |
| An earlier proposer/attester owns node169's proposer logical lock | Excluded in the proposed form. Role keys differ, slot 0 returns before proposer locking, and slot 1 is this owner's first ordinary proposal. A delay in the shared registry or OS scheduling is a separate claim. |
| All 1,000 nodes compete for one host's CPU | Contrary to the recorded user deployment clarification: one node per machine. Per-machine BN/VC resource contention remains possible, without retained quota or utilization measurements. |

The [historical FCU joins](fcu-bn-payload-delay-audit.md) strengthen the prior for
broad BN-side pressure: 12 of 13 failed owners in slots 2–14 have more than
300 ms between a fast proxy response and the matching BN payload-ID marker;
the median is 1.483 seconds. The two successful owners have 1.781/0.389 ms gaps.
These intervals also include payload-cache/head locking and logging. They are
not pure network times and do not identify a DomainData queue. Node169 has no
comparable marker because a valid source path omits it; its absent marker is
not evidence of a stalled FCU completion.

## Slot 1 needs an additional delay after successful role preparation

Node169's retained `validator.log` is under
`/tmp/prysm-r2-extra-logs.Rd7MjT/extracted/round2-169/`:

- Line 686, `01:30:21.915067414`: a dispatched PTC role receives the no-block
  response. `RolesAt` has already returned by slot offset +9.915067414.
- Line 708, `01:30:24.006131913`: an already-dispatched sync aggregator fails
  its later index call. Selecting this role required at least one successful
  sync-selection domain lookup during preflight.
- Line 694, `01:30:24.002452348`: RANDAO fails while obtaining its domain.

There were still **2.084932586 seconds** before the absolute deadline when
the PTC result was logged. This is not the proposer's measured remaining
budget: its goroutine start and first domain invocation were not logged. If
it ran promptly after dispatch, however, a merely few-millisecond remaining
budget cannot explain the failure. A later scheduling delay, cache wait, or
slow RPC envelope must be added to any late-preflight hypothesis.

The successful preflight domain lookup excludes one uninterrupted domain-cache
writer extending from slot start to the deadline. Ordinary attesters fail
before signing; available attesters use a static domain; the logged PTC and
post-dispatch sync failures also precede their signing domains. The proposer
and attester multilock keys do not collide. These exclusions make a specific
long VC lock holder harder to support than a broader servicing delay.

The terminal RANDAO error does establish that its caller eventually acquired
the domain write lock, missed the cache again, and invoked its own client
adapter. Only that adapter's error is returned by `domainData`; a cache hit
succeeds even on an expired context. The record does not establish that bytes
left the VC or that the adapter started with a live budget. Details and source
anchors are in [deeper-preflight.md](deeper-preflight.md) and
[domain-data-causal-audit.md](domain-data-causal-audit.md).

The profile-free [real TCP control](domain-data-tcp-realwork-results.md) is
substantial counterevidence to the narrower claim that checkpoint/count CPU
work alone makes ordinary DomainData take seconds: all 32 loaded scan-arm
calls finish within 5.700482 ms. The actual E1 VC RANDAO calls were fast too.
E1's separate 2.165041-second probe envelope supports the possibility of a
slow envelope around a cheap body in the full process, but continuous CPU
profiling and previous goroutine snapshots prevent treating it as a clean
no-observer latency control. No direct evidence selects HTTP/2 flow control,
a retry sequence, or the VC domain-cache mutex as node169's missing interval.

## Slot 4's terminal RANDAO is already explained by dispatch ordering

Node91's archive member `./validator.log`, lines 832 and 839, records the
sync-selection domain failure at `01:31:00.003838012`, then RANDAO at
`01:31:00.003910654`. The latter follows by **72.642 microseconds**. The first
message occurs synchronously inside `RolesAt`; the runner dispatches the
proposer only afterward, using the deadline `01:31:00`.

The cost to explain is upstream of that dispatch. A successful sync-index RPC
precedes the failed sync-selection signing-domain call. The measured real
checkpoint/multilock convoy is the strongest candidate for consuming most of
the time before that cheap call. Earlier proof work or a delay in the domain
envelope remains possible. The available evidence does not measure either
historical RPC's duration, but it does establish that RANDAO starts expired.

## What the 596-key startup workload actually does

`validator/client/subnets.go:17,34–95` expands current duties first, then next
duties, using 16 workers. With 596 keys, 32 slots per epoch and eight slots per
round, a complete two-epoch refresh has **4,768** proof jobs. These jobs access
two selection-domain cache keys, one per epoch; this is not 4,768 distinct
domain misses. Both keys fit even the mis-sized cache's roughly three-entry
effective capacity. Other domains can evict them, but that is conditional
interleaving, not an automatic miss for every signature.

`aggregate.go:200–232` releases `domainData`'s lock before computing the signing
root and calling the local keymanager. `keymanager/local/keymanager.go:178–192`
holds its map read lock only for key lookup, then signs outside it. The proof
cache is a plain map; only refresh clears it. Blocking singleflight can delay
a role behind a background winner, but it does not itself make local BLS
signing slow. `onDutiesUpdated` launches the refresh with a background context
(`duties.go:764–778`); the subscription RPC comes after all proof workers finish.

The historical node169 performs an initial epoch-0 update at
`00:50:44.992490931` (line 612), well before genesis `01:30:00`, and another at
`01:30:02.886759811` (line 648). Thus the genesis refresh is not the first chance
to fetch the two selection domains. Its completion time is unlogged.

E1 also has 596 keys but four-slot rounds: a complete two-epoch refresh implies
**9,536** jobs. Its `validator3.log:19773` records the genesis refresh at
`14:03:59.35`, approximately genesis +1.35 seconds. The final retained
selection-domain lookup at lines 39800–39801 is timestamped
`14:03:59.856585026–14:03:59.856585507`, about half a second later. This bounds
the observed domain-access phase, not the final BLS or subscription completion.
Its only selection-domain RPC pairs occur before genesis, at lines 669/680
and 10221/10236, lasting 0.558 and 0.628 ms. The raw file is
`/tmp/prysm-startup3-round4-early-e1/validator3.log`.

E1 offered its first count-driving gossip at genesis +11.609 seconds, so the
observed refresh domain accesses did not overlap that load. Historical slot-zero FFG and
available gossip are also rejected before their expensive validation paths
(`beacon-chain/sync/validate_beacon_attestation.go:86–88,572–575`). Historical
node169 therefore has roughly nine seconds between its refresh and slot 1,
slightly less allowing early-gossip tolerance. Ordinary startup work can still
run in that interval, but assuming a large count cohort already overlaps the
refresh is unsupported. E1's timing lowers the pure-proof hypothesis; it does
not prove the historical refresh completed at the same speed.

The first qualifying gossip is not constrained to the usual one-third-slot
attestation timer. Node169's `validator.log:3`, at
`00:50:38.745258087`, explicitly enables `decoupled-ffg-vote-at-slot-start`.
`SubmitAttestation` therefore selects `waitSlotStartJitter` instead of the
ordinary due/block wait (`attest.go:38–42`). The jitter uses the absolute slot
start (`wait_helpers.go:76–100`); the source default is 200 ms, although the
historical explicit jitter value is not recovered.

The newly extracted [observer onset records](ffg-count-onset-observers.tsv)
confirm that this is an actual historical timing difference:

| Observer | Earliest retained accepted-cohort entry offset | Earliest accepted completion offset | Entries / completion records before +500 ms |
| --- | ---: | ---: | ---: |
| 400 | +38 ms | +57.085279 ms | 22 / 22 |
| 201 | +155 ms | +211.152107 ms | 45 / 13 |

The raw first completion is `runs/round2/prysm-geth-400/beacon.log:891`,
`01:30:12.057085279`, with `attSlot=1`, `targetRound=0`, `outcome=gossip`,
`arrivedMs=38`, validator 14336. Node201's first completion is line 899 at
`01:30:12.211152107`, with `arrivedMs=158`, validator 50454; its earlier entry
at +155 ms completes later at line 902. These accepted records prove the count
path had executed by those completion times, before the usual +4-second
attestation timer. They do not date node169's own ingress, enumerate all
in-flight work, or identify concurrent scan counts. Entry offsets are validation
entry, not packet arrival or count-function entry; the earliest retained record
need not be the first actual request.

The early wave also produces long-lived work: node201 line 2916 enters at
+1.566 seconds and emits its accepted record at `01:30:38.954321434`, an
entry-to-log interval of 25.388321434 seconds. The count function alone is not
timed by that interval. This provides positive historical support for an early
validation cohort persisting while proposal prerequisites are running.

Successful slot-zero sync roles do not establish a prepared slot-one sync
state. `getSyncCommitteeHeadState` keys both its cache and its logical lock by
the requested slot (`head_sync_committee_info.go:133–169`). A first filler for
slot one obtains head state and calls `ProcessSlotsUsingNextSlotCache` while
holding that key; the next-slot cache may satisfy that call. All callers enter
the shared multilock registry even on a hit. This gives initial
state preparation a plausible way to overlap a new slot-start gossip wave,
after which the demonstrated global-registry convoy can prolong the lookup.
Another client may have performed the first fill historically; this is not
proof that node169's particular VC call was cold. The ordinary DomainData body
does not have this checkpoint/cache dependency.

## Experimental judgment

A forced simultaneous launch of the proof refresh and the checkpoint/count
cohort would test sensitivity to overlap, but would insert an ordering not
established historically. Adding real `RolesAt` would also let the already
proved sync convoy consume the budget independently of the proof work, making
the result less discriminating for node169's additional delay. No such new
experiment is recommended at this point, and no extra run was performed.

The positive causal ranking is stronger than a list of equally plausible
unknowns: checkpoint-dependent preflight is the leading expensive prerequisite;
slot 4's RANDAO failure follows directly from expired dispatch; slot 1 needs
additional servicing or scheduling delay, with a specific VC cache convoy
ranked lower. The original deployment note in
[startup_diagnosis.md](../startup_diagnosis.md:20) records one node per machine,
800 machines at 20 Mbps outbound/50 Mbps inbound and 200 unlimited. Exact CPU,
RAM, disk, and node-to-bandwidth-class data are absent. Those missing facts do
not justify replacing this ranking with an assumed shared-host CPU bottleneck.
