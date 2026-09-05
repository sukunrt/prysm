# Why the full gossip workload can form a different count cohort

The subsequent [complete native-trace join](full-gossip-finalizer-cohort-trace-audit.md)
directly observes the sequence investigated here: checkpoint handoffs delayed
by admitted callers waiting to run, then a natural finalizer collecting and
waking 101 Count readers. It also follows the later Count work back to callers
already queued for checkpoint access. The timed attestation-data copy is a
separate short event; the [matched omission pair](full-gossip-domain-no-timed-attdata-results.md)
stays fast in both arms. The design discussion below records the earlier
evidence and controls that led to these findings.

The previous real TCP DomainData control released 6,144 checkpoint/count
workers at once. Almost all were waiting inside checkpoint retrieval at the
sampled RPC admissions, with about three active count calls. That is not the
same execution state as the retained full gossip runs. Two specific production
edges can change the distribution: staggered admission past the checkpoint
lock, and a real head-state copy collecting readers at the shared validator
slice's writer lock. The second edge has a directly retained writer stack.
Neither finding by itself identifies node169's historical slot-1 RANDAO wait.

## The checkpoint lock does not serialize the count

The FFG path is `VerifyLmdFfgConsistency` → `AttestationTargetState` →
`validateUnaggregatedAttTopic` → `validateCommitteeIndexAndCount` →
`ActiveValidatorCount` (`sync/validate_beacon_attestation.go`). At round zero,
`getRecentPreState` always declines the fast path. `getAttPreState` uses the
same logical key, checkpoint root plus decimal round, for canonical genesis
votes (`blockchain/process_attestation_helpers.go:25,103`). The checkpoint
cache returns its state without copying (`cache/checkpoint_state.go:54`).

`AttestationTargetState` also holds the fork-choice read lock until retrieval
returns (`blockchain/receive_attestation.go:40`). Before that return, the
deferred multilock `Unlock` first returns the logical key's token and then
runs `async.Clean` through the package-global manager (`async/multilock.go`).
Only after all these defers does the caller start its count. There is no
semaphore holding the checkpoint key throughout `ActiveValidatorCount`.
Consequently, a large batch can queue at the global manager and limit count
entry, while arrivals that clear this short stage before the queue builds can
leave many expensive scans in flight.

Full gossip also does not synchronously release 6,144 checkpoint callers.
Prysm overrides the pubsub validation queue size, but retains libp2p's default
1,024 concurrent validators per topic and 8,192 globally
(`p2p/pubsub.go:161`; module
`github.com/libp2p/go-libp2p-pubsub@v0.17.0/validation.go:16,369,478`).
Six FFG subnets therefore permit 6,144 validations distributed across all
validation stages. E1's source uses 64 RPC workers, an unbuffered jobs channel,
and a planned due time of `firstDue + i*spread/len(atts)` for each vote
(`runs/diagnostics/startup3/ffgsource/main.go:32,290`). Its measured 15,000
source calls per slot span approximately 1.99 seconds. These are source RPC
times; remote checkpoint-stage entry times can be altered by the network and
earlier validation stages.

The competing bypass explanations are weaker. Canonical genesis FFG votes
cannot choose zero-root aliases or arbitrary checkpoint keys: consistency is
checked before retrieval. Aggregate gossip uses the same checkpoint/count
sequence. Warm committee lookup does not perform another count. Local
`ProposeAttestationElectra` broadcasts using cached `HeadValidatorsIndices`,
then retrieves the checkpoint and committee without an `ActiveValidatorCount`
call (`rpc/prysm/v1alpha1/validator/attester.go:109,239`). P2P scoring computes
and memoizes the count once under its own mutex
(`p2p/gossip_scoring_params.go:165`). The pending-attestation bucket does one
checkpoint retrieval and then counts verified votes in a serial loop; known
genesis-root gossip need not enter that pending path.

## A retained natural writer can collect count readers

The E1 profile
`/tmp/prysm-startup3-round4-early-e1/profiles/bn-goroutine-plus-37.pb.gz`
contains 919 `ActiveValidatorCount` stacks:

| Captured position | Count |
| --- | ---: |
| Parked in validator multi-value slice `Len` → `RWMutex.RLock` | 910 |
| Parked in that slice's `At` → `RWMutex.RLock` | 2 |
| Count callback, activity predicate, or `At` computation | 7 |

The same profile contains a concrete writer:

```text
GetAttestationData
  → HeadState → headState → BeaconState.Copy
  → validatorsMultiValue.Copy → sync.RWMutex.Lock
```

This writer is waiting in the production attestation-data RPC, not an
artificial diagnostic lock holder. Reproduce the inspection with
`go tool pprof -traces -focus='ActiveValidatorCount|multi-value-slice'` and that
profile. The binary profile gives call stacks, not lock addresses or a
continuous timing record.

The shared identity follows from the startup path. E1's `beacon3.log:26,30`
records startup from the existing genesis DB and initialization of head slot
zero. `StartFromSavedState` calls `StateGen.Resume`, which loads genesis,
passes that state to `SaveState` for the hot/epoch caches, and returns it for
`setupForkchoice`/`initializeHead` (`blockchain/service.go:266,330`;
`state/stategen/service.go:124`). `setHead` calls its state `Copy`
(`blockchain/head.go:228`). The first genesis checkpoint retrieval uses the
state cache's `Copy`, and then retains that copy in the checkpoint cache
(`state/stategen/hot_state_cache.go:42`). Each native `Copy` assigns the same
`validatorsMultiValue` pointer to its destination and invokes that slice's
`Copy` method (`state/state-native/state_trie.go:1049`). These are distinct
state objects sharing one validator-slice mutex, not independent registries.

The current DomainData fixture already has this identity: it inserts the
original genesis state in the checkpoint cache, while `saveGenesisData`
initializes head through `setHead.Copy`
(`blockchain/domain_data_diagnostic_export_test.go:50`).

Every scan takes the slice read lock once for `Len` and once per validator
for `At` (`state/state-native/getters_validator.go:227`;
`container/multi-value-slice/multi_value_slice.go:168,255`). Native state
copying takes that slice's write lock (`multi_value_slice.go:181`). Go's
`RWMutex.Lock` announces a pending writer before waiting for existing readers;
new readers then park, and writer `Unlock` wakes the accumulated readers
(`sync/rwmutex.go:144,201` in the local Go source).

Thus even a writer whose useful work is short can change scheduling: while it
waits for the previous tiny reader sections or for processor time, new count
calls can finish checkpoint retrieval, reach `Len`, and park. Releasing the
writer makes this collected reader cohort eligible to run. This requires no
expensive registry mutation. At unchanged genesis, slice `Copy` has empty
individual/appended maps; it does not copy 120,000 validators.

A cold sync-index lookup has the same natural writer edge:
`getSyncCommitteeHeadState` cache miss → `HeadState.Copy` before processing
slot one (`blockchain/head_sync_committee_info.go:135`). Warm sync-index
lookups skip it. This is a narrower control than injecting a held writer.
There is also counterevidence against assigning every preflight delay to this
writer: E1's matched first slot-1 sync call spent only 0.090 ms in its
instrumented `HeadState` phase; its major observed delay was elsewhere, and
the second call's large delay was in deferred multilock cleanup. The +37
writer establishes an available production interaction, not that E1's first
cold sync copy was slow or that historical slot-1 RANDAO waited on this mutex.

## Singleton BLS aggregation retains another cohort

After the count, FFG signature preparation calls
`blocks.AttestationSignatureBatch` → `createAttestationSignatureBatch` →
`state.AggregateKeyFromIndices` → `bls.AggregatePublicKeys`
(`core/blocks/signature.go:168`; `state/state-native/getters_validator.go:181`;
`crypto/bls/blst/public_key.go:77`). Even a single attesting index enters
BLST's generic `P1Aggregate.coreAggregate` with `n=1`.

In the pinned BLST v0.3.16 implementation, that call spawns one child, which
performs the affine conversion and unconditionally calls `runtime.Gosched`
before sending the result. The parent waits on the result channel. The child
must run again merely to discover that there is no next item and deliver its
completed result (`bindings/go/blst.go:1595`). Local Go's `goschedImpl`
changes the goroutine to runnable and puts it on the global run queue
(`/home/sukun/dev/go/src/runtime/proc.go:4307`).

E1's retained `bn-goroutine-plus-43.pb.gz` has 1,069 FFG aggregation parents
parked on that result channel, 1,059 P1 children at `runtime.Gosched`, and 12
other P1 child stacks. It also contains P1/P2 aggregation from the batch
verifier and background attestation compaction. Inspect it with
`go tool pprof -traces -focus='coreAggregate'`. The 1,069 parents are not
runnable CPU consumers, and the snapshot does not establish continuous queue
length or the waiting duration of any specific RPC.

This stage frees processor time for other already admitted work and adds
runnable child resumptions. It does **not** release pubsub validation capacity:
the per-topic token remains held around `validateMsg`, and the global token
remains held around `doValidateTopic` until validation finishes. Consequently,
singleton aggregation can retain validation slots and enlarge the runnable
queue, but it is not a direct permission for replacement checkpoint callers.
Whether it raises or lowers count concurrency depends on the other stages.
It is not evidence by itself of a seconds-long ordinary DomainData delay.

## Distinguishable bounded controls

1. Keep real checkpoint retrieval and real counts, but offer the same finite
   15,000 jobs over two seconds rather than releasing all workers together.
   Record actual offer/entry times as well as active checkpoint/count counters.
   A larger count cohort and slower DomainData in this arm would support the
   arrival-shape explanation. A paced null would leave it insufficient.
2. Add an actual cold slot-1 sync-index lookup to that same shared-state
   fixture, while probing DomainData. Do not prewarm the per-slot sync cache,
   hold an internal lock, or enlarge the copy work. If the copy takes longer
   and count entries accumulate then advance together, that supports the
   natural writer gate. A cheap copy with unchanged cohort constrains it.
   Atomic active-count totals alone cannot distinguish scanning from a parked
   `Len`; a null does not require inventing a longer lock hold.
3. Only if these controls leave the important gap, add the actual singleton
   signature-preparation stage to each real vote job. Measure completion and
   aggregation-parent activity separately from counts. A changed runnable
   population with fewer active counts is consistent with this edge; increased
   count totals are not required. Keep the finite vote count and real work,
   with no artificial yield or sleep inside production operations.

These are predictions for diagnostic controls, not results of tests run by
this audit. No production source was changed.
