# Sync preflight: code cause and evidence ranking

**New direct Go control:** at the observed 6,144-worker cohort, 15,000 finite
checkpoint/count jobs reproduce warm sync lookups lasting **13.448500 and
13.571680 seconds with no goroutine snapshots or runtime tracing**. Reusing
the same immutable checkpoint's correct count reduces the maximum to
**11.833 ms**, with the same completed jobs and launch arrangement. The
[real-work results](sync-index-count-realwork-results.md) preserve both arms.
This is direct evidence that the production workload can consume more than
the complete twelve-second proposal budget even on a warm sync-state path.

The smaller 256-worker trials remain relevant limits: one instrumented trial
showed half-second waits, but a matched no-snapshot trial stayed fast. That
smaller series cannot isolate instrumentation from arrival ordering, and
count work does not guarantee a stall under every load or interleaving.
The larger no-snapshot result closes the observer concern for the measured
thirteen-second handler delay. A separate traced pair locates the code:
one 15.544469-second call spends **11.017502 seconds blocked in the global
manager's `getChan` send before cache access**, then **4.523097 seconds blocked
in the same manager's `Clean` send after the cache hit**. Its final runnable
delay is only 3.855 ms. These waits are distinct from the payload reader's
runnable starvation: the sync handler is queued on a shared channel.
Checkpoint lookup workers release that channel and wake the sync handler in
both cases. This directly locates the upstream delay beyond a terminal RPC
timeout; tracing is not needed to produce the long delay in the primary pair.

This audit reads historical revision
`0280403c70d88967f49d2d4c730f4c5417dabdf5` with
`jj --ignore-working-copy file show`, the recovered owner logs, and the existing
real-work E1/F1 results. It makes no production change and launches no network
deployment. Source line numbers below refer to that historical revision.
The [observer and handoff audit](observer-risk-and-sync-handoff-audit.md)
independently checks the new phase split, the measured manager handoffs, and
the effects and timing limits of the older profiling.

The main code defect is that unrelated sync-aggregator discovery gates dispatch
of an already-known proposer. The strongest measured upstream cause of slow
discovery is the combination of redundant genesis validator scans and a
package-global multilock registry used even on sync-state cache hits.

## Why a sync-state cache hit can take seconds

`GetSyncSubcommitteeIndex` maps the public key to an index and calls
`HeadSyncCommitteeIndices`. For the early slots, that calls
`headCurrentSyncCommitteeIndices`, which obtains a slot-advanced head state
before accessing the separate committee-position cache
(`beacon-chain/blockchain/head_sync_committee_info.go:61–84`).

The apparent fast path in `getSyncCommitteeHeadState` is not independent of
other beacon-node work:

1. It takes `async.NewMultilock("syncHeadState-<slot>").Lock()` **before**
   reading the cache (`:138–143`).
2. A hit assigns and returns the cached state (`:145–148`), but returning runs
   the deferred `mLock.Unlock()` first (`:140`).
3. `async.Lock.Unlock` releases the logical key token and then synchronously
   calls `Clean()` (`async/multilock.go:57–66`).
4. `Clean()` obtains the package-global channel mutex and examines every
   registered logical key (`:93–112`). `getChan` obtains that same channel
   mutex (`:115–124`). Keys with unrelated purposes therefore contend during
   both acquisition and cleanup.

The global mutex is not held throughout a checkpoint's state processing or
throughout a sync RPC. The defect is the shared serialized manager and its
mandatory return-path cleanup under a large cohort of callers. Neither this
manager nor the logical-key wait accepts a context. Client cancellation cannot
remove a beacon handler already queued there.

The channel implementation also explains how count work can slow the manager
without holding it throughout a scan. When the capacity-one channel has queued
senders, releasing it admits the next sender and keeps its buffer occupied
before that goroutine runs. Go's `runtime/chan.go:recv` copies the queued
sender's value into the buffer and then calls `goready`. Until the admitted
goroutine runs and releases the manager, everyone behind it remains queued.
The previous checkpoint caller is free to continue into its full validator
count during that handoff. The new trace catches a checkpoint caller handing
the manager onward, then continuing into count work while the newly admitted
goroutine waits runnable. Thousands of these shared handoffs allow unrelated
sync-state keys to wait behind the checkpoint cohort.

E1 directly caught this path in a real `GetSyncSubcommitteeIndex` handler. Its
second real VC request hit the state cache, spent 8.887 ms acquiring the
multilock, and spent approximately **4,795.074 ms between the cache-hit marker
and return**. This interval includes return-path bookkeeping, deferred
unlock/cleanup and scheduling; the sampled handler is specifically in
`Clean`, but the entire interval is not a measured uninterrupted `Clean`
wait. The request's total was 4,803.963 ms. A concurrent index
request spent 4,437.506 ms acquiring the manager/key. The +13-second goroutine
snapshot includes the actual sync-index handler in
`Unlock -> Clean -> runtime.chansend`, plus another in `Lock -> getChan`.
The matched cold request's slot processing took only 73.815 ms.
[E1 phase and profile evidence](../startup3/early-gossip-results.md).

This explains why "the sync state was cached" and "sync state advancement is
usually cheap" do not establish a cheap sync-index RPC. It also identifies a
real blocked code path, beyond a terminal RPC deadline.

## Where the contender cohort comes from

Unaggregated FFG gossip calls `AttestationTargetState` and then validates the
topic/committee count (`beacon-chain/sync/validate_beacon_attestation.go:143–154,
249–257,290`). `AttestationTargetState` holds a fork-choice read lock while
obtaining the checkpoint state (`beacon-chain/blockchain/receive_attestation.go:50–52`).

The recent-state shortcut refuses checkpoint zero
(`process_attestation_helpers.go:26`). The fallback therefore takes a
checkpoint-root/round multilock **before** checking the checkpoint cache
(`:109–120`). Every successful cached lookup still runs that lock's deferred
`Unlock -> Clean`; that deferred cleanup finishes before the enclosing
fork-choice read lock is released. The checkpoint and sync-state keys are
different, but the async registry is one package-global object in the BN.

After obtaining the checkpoint state, topic validation calls
`helpers.ActiveValidatorCount`. The count has already been cached, but its
fast return explicitly requires `s.Slot() != 0`
(`beacon-chain/core/helpers/validators.go:154–159`). With a populated committee
cache and the genesis state, execution instead reaches a full validator scan
(`:161–180`). The predicate concerns the **state slot**, not the attestation's
slot; using a genesis pre-state for later attestations preserves this cost.

For 120,000 validators, each such invocation visits 120,000 entries.
`ValidatorsReadOnlySeq` performs `validatorsMultiValue.At` for every entry
(`state/state-native/getters_validator.go:227–248`), and `Slice.At` obtains a
shared storage `RWMutex` per access
(`container/multi-value-slice/multi_value_slice.go:255–257`). The atomic work on that path is the
mutex's reader-count bookkeeping. It should not be described as validator
reference counting.

The resulting feedback is concrete: prior callers leave the serialized
checkpoint stage and run full scans; those scans consume CPU needed to drain
later registry/checkpoint waiters; the waiters include unrelated sync-index
handlers. Checkpoint waiters also retain fork-choice read locks, coupling the
same backlog to proposal/head writers. The full scan itself happens **after**
its caller has released fork choice, so it is inaccurate to say each scan
holds that lock throughout.

In E1, `ActiveValidatorCount` accounted for 140.28 of 153.78 sampled CPU-seconds
(91.22%). F1 retained the same offered valid gossip workload and removed only
the redundant genesis count scan diagnostically: count CPU fell to 0.03
seconds, the largest sampled gossip-validator cohort fell from 6,144 to 49,
and the largest observed sync-index probe was 6.358 ms. All three F1 proposals
succeeded. This is a real-load causal control for the scan/queue mechanism;
the ablation is not a proposed consensus change.
[E1/F1 comparison](../startup3/early-gossip-results.md).

## Why this suppresses an unrelated proposal

`RolesAt` already stores proposer and other roles while traversing duties.
Nevertheless, before returning **any** role it calls
`SyncCommitteeAggregators` (`validator/client/validator.go:646`). The local
selector loops over sync pubkeys sequentially; each performs an index RPC
followed by selection-domain lookup/signing
(`aggregator_selector.go:131–172`). There is no client cache of sync indices
or sync-selection signatures in that selector. A subsequent dispatched
sync-aggregator duty performs the lookup and signing again
(`sync_committee.go:127–140`).

Thus preflight latency accumulates across the sync keys. E1's two real
sync-index calls took 355.968 and 4,881.583 ms; `RolesAt` took 5,244.921 ms.
The proposer could not start during those five seconds even though its role
was already known. The code has made the launch of a time-critical proposal
dependent on completion of a different duty's aggregation eligibility.

On selection failure, `RolesAt` logs the error and returns the collected roles
with **nil** top-level error (`validator.go:646–649`). The runner then
dispatches them using the same absolute deadline it computed before preflight
(`runner.go:104–105,147–156`). Once that deadline is gone, the proposer enters
its initial RANDAO-domain step with an expired context
(`propose.go:62–70`). A RANDAO failure here does not require a second slow
DomainData request.

That final code edge is established in historical round-2 slots **2, 3, 7,
11, and 12**, where index-fetch preflight failures occur at the slot deadline.
It also applies to slot **4**, whose preflight progressed through an index
response but failed in sync-selection signing. Its direct keymanager and
gRPC error identify the selection-domain access, before local signing; its
RANDAO error followed approximately 72.6 microseconds later. Exact owner
anchors and preceding progress are recorded in
[the owner comparison](preflight-owner-comparison.md).

## What is established, and what is still an amplifier candidate

| Mechanism | Evidence and rank |
| --- | --- |
| Unrelated sync preflight blocks proposer dispatch and preserves an expired absolute deadline | Exact historical source plus the owner terminal sequence; directly accounts for why the six owned proposals never reached block production. |
| Repeated genesis count scans overload servicing and the global multilock manager delays warm sync-state reads | Exact source, real E1 blocked handler/phase measurements, and matched F1 count-ablation control. Strongest measured upstream explanation. |
| Per-slot sync-state cache holds only one entry | Exact `cache/sync_committee_head_state.go:19–22`; previous/current slot calls can evict each other while using different per-slot locks. Added state processing is possible, but it is not needed for E1's measured warm-cache stall and has not yet been isolated by an alternating-slot real-work comparison. |
| Attestation proof singleflight delays preflight before the sync-index RPC starts | Exact local selector `singleflight.Group.Do` uses the winner's context; the detached subnet worker can win. Existing controlled tests establish cancellation semantics and the same terminal category, not the historical duration or real loaded frequency. |
| Small domain cache and a global RPC-held miss lock | Real cache-configuration test retained only three entries; exact `domainData` holds one global write lock across an RPC. This can delay sync selection or a cold RANDAO lookup. Existing E1 real proposer RANDAO calls remained fast, so those measurements do not establish a seconds-long historical domain holder. |

Historical owner logs do not record the entry time of the failing index RPC,
its cache result, or its handler stack. Consequently the **4.795-second E1
post-hit interval must not be promoted into a measured twelve-second wait on each
historical owner**. The actual code cause is nevertheless more specific than
"an RPC timed out": avoidable per-message genesis scans create resource
pressure, mandatory global lock bookkeeping allows that pressure to delay
cached sync discovery, and synchronous cross-role dispatch converts that
delay into a missed proposal. Which share of each historical twelve-second
budget was spent in the manager, earlier proof waiting, or RPC scheduling is
not separately recorded.

Round-2 slot **1** is distinct. Its roles had dispatched and at least one sync
selection-domain lookup had succeeded before the RANDAO deadline. The ordinary
RANDAO DomainData handler performs constant-sized config/genesis-root work;
it does not scan validators or acquire fork choice. The relevant remaining
routes are delayed proposer scheduling, the VC domain mutex, or scheduling
and transport around that cheap handler. Neither the later preflight terminal
pairs nor E1's warm sync-index stall establishes node 169's precise route.
[Slot-1 audit](../startup3/round2-slot1-cause-deep-audit.md).
