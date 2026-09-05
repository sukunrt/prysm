# Ordinary slot-97 validation: contention boundaries

This audit reads deployed revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`.
It guides the paced comparison; it does not establish historical contention.

## Local attestation-data RPC bound

The 71 local slot-97 votes do not imply 71 template requests or native-state
copies. The deployed validator client coalesces post-Electra requests across
committees, holds a write lock during the first RPC, then reuses its successful
answer for that slot (`validator/client/validator.go:763–820`). Failed requests
can be retried; this is a bound on the successful cached path, not a census of
unlogged RPC attempts. Attestation signing domains are separately cached by
epoch and domain (`validator/client/validator.go:720–759`). The slot-start flag
is enabled in node 1's validator log, line 3; the duty can consequently begin
before the first remote gossip entry (`validator/client/attest.go:40–44`,
`wait_helpers.go:77–101`).

On a beacon-node template cache miss, `rpc/core/validator.go:597–621` obtains
one `HeadState`. That call copies the native state while holding
`headLock.RLock` (`blockchain/chain_info.go:238–252`, `head.go:299–303`). Native
copy takes the source state's read lock and invokes six shared MVS managers'
copy methods (`state-native/state_trie.go:1049–1052,1151–1157`). Each manager
takes a write lock while visiting its individual/appended entries and adding
the destination ID (`container/multi-value-slice/multi_value_slice.go:180–214`).
This can interact with the proposer's shared state managers, but it is not a
fresh copy of all 120,000 validator records per local vote. The amount of
fragmentation in those managers is not retained in the historical logs.

The subsequent template transition is conditional on crossing a **round**
boundary. Head 96 and request 97 are both round 12, so the observed parent-96
path does not execute `ProcessSlotsUsingNextSlotCache` here
(`rpc/core/validator.go:615–621`). Template-cache locks are separate from the
proposer; forkchoice/head locks cover the individual fetches, and the head lock
is released before the conditional transition. Each local submission later
uses `AttestationTargetState` and a cached committee, without a new `HeadState`
copy (`rpc/prysm/v1alpha1/validator/attester.go:113–131`). Its subnet lookup
uses the read-only head and `ActiveValidatorIndices`
(`blockchain/chain_info.go:277–286`); the ordinary cache-hit path returns cached
indices rather than rescanning the validator registry.

The first local ledger record is at `arrivedMs=108`, outer timestamp
`01:49:24.110229828Z` (`prysm-geth-1/beacon.log:1122057`). This log is after
the local broadcast, so the successful shared template had already returned
by about slot+108 ms. All 71 local records have offsets 108–261 ms; these are
submission/log offsets, not RPC-entry timestamps. The validator's delayed
summary (`validator.log:2213`) independently reports first successful
submission at 111 ms and a 166 ms spread, despite being emitted at slot+9 s
(`validator/client/log_helpers.go:236–258`). These markers allow an early
template overlap but do not demonstrate it, and exclude the successful local
template itself remaining blocked throughout the 3.645 s joined construction
interval. No additional local-RPC load experiment is justified by this audit.

The slot-97 execution response itself is bounded: node 1's snooper records
`engine_getPayloadV6` request #1302 at `01:49:24.012800179Z` and response at
`01:49:24.013735789Z`, 0.936 ms apart. The response has empty transactions,
withdrawals, execution requests and blob arrays, and zero gas used. Its block
access list is 273 bytes, so the payload is not literally byte-empty
(`runs/round2/prysm-geth-1/snooper-engine.log:55437–55480`). The later payload
selection marker establishes that this successful response was consumed before
the measured selection-to-completion interval.

The corrected unloaded full-parent construction takes 37.530 ms for the compact
three-attestation fixture and 91.766 ms for the density-eight fixture. The first
observed current-slot gossip entry is 21.264 ms after the outer build-start
marker, or 12.750 ms after payload selection. Only 102 gossip validations had
entered by slot+100 ms. Consequently, the entire eventual 14,355-vote workload
cannot be assigned to the first 100 ms of construction. A contention explanation
must account for how the early overlap prolongs work enough to encounter later
arrivals, or identify work already outstanding before construction.

## Full-build fixture review

The measured fixture saves a warmed native slot-96 parent in real `StateGen`,
separately advances a copy to prepare the proposer at 97, and asserts no
next-slot-cache hit for the actual parent root. It supplies a fresh prepared
head and block for every timed call. Their preparation and assertions are
outside the `BuildBlockParallel` timer; the production state retrieval, copies,
slot advance, full-parent envelope reads/application, operation packing,
post-state/root calculation, and final envelope-cache storage are inside it.
The eight-attestation arm starts with 48 per-committee pool objects. Both arms
preserve 493 valid sync participants. Assertions check the expected operations,
nonzero state root, and the cached envelope's matching block root.

The distinct saved latest-execution hash and parent bid hash represent the
post-block-96/pre-payload-96 lifecycle. An earlier equal-hash fixture still ran
`ApplyParentExecutionPayload`: that function has no already-applied hash
short-circuit. Correcting the hashes fixes state semantics, rather than adding
a previously skipped function. Immediate dependency mocks and warm local
storage remain limits of these unloaded measurements; see [results](results.md).

## Shared locks and state ownership

- `blockchain/receive_attestation.go:41–52` holds the forkchoice read lock only
  while obtaining the target state. The ordinary compatible-head branch returns
  the existing head state without copying. The lock is released before committee
  membership checking, signature preparation, the batch-verifier wait, and pool
  insertion. The separate [coverage audit](historical97-coverage-bounds.md)
  documents this state-ownership path and its cache-miss qualifications.
- A committee-cache hit returns a slice of cached indices
  (`cache/committee.go:107–147`), and the active-count hit returns a cached slice
  length (`committee.go:194–209`). These hits do not clone or rescan 120,000
  validators. Cold or different-seed entries are a separate condition.
- Single-vote signature preparation reads one validator through the shared MVS
  manager (`state-native/getters_validator.go:181–196`). MVS `At` holds a read
  lock around its lookup (`multi-value-slice.go:255–281`). A native state `Copy`
  holds the source state's read lock and invokes MVS copies, whose manager write
  locks cover reference bookkeeping (`state_trie.go:1049,1151–1157`;
  `multi-value-slice.go:180–214`). Ordinary validation does not itself create
  one native state per vote. The builder's independently copied state has its
  own state mutex, although MVS managers and helper caches remain shared.
- The production verifier queues signatures and waits for its result
  (`sync/batch_verifier.go:19–82`). The ordinary validation path does not retain
  a head/forkchoice/MVS lock for this whole wait. The batch limit is 1,000 and
  the flush ticker is five milliseconds; neither is a proposer dependency.
- The raw subscriber's exclusive pool lock covers insertion into the map
  (`kv/unaggregated.go:15–40`). A proposer snapshot holds a pool read lock while
  scanning and cloning entries. The aggregate pool can have a longer critical
  section: `SaveAggregatedAttestation` holds its write lock while merging a
  key's existing aggregate objects (`kv/aggregated.go:145–163`). That is an
  aggregate/compaction condition, not a lock held by an ordinary single's BLS
  validation.

These are actual points of contention, but this audit finds no warm ordinary
single-vote critical section that intrinsically holds the whole builder for
the duration of the arriving traffic. Sustained CPU competition, a slow lock
holder, an existing queue, or an external sink requires measured evidence.

## Proposal snapshots and prior-slot work

Packing snapshots and validates aggregates first, then snapshots and validates
raw singles (`proposer_attestations.go:32–47`). Once these snapshots return,
later arrivals do not join that pack. The consensus goroutine starts before
payload selection, so an unloaded run can obtain its snapshots before the
first observed current-slot validation. Its actual snapshot times should be
observed rather than inferred from the final build duration.

Even if slot-97 singles enter a slot-97 snapshot, their inclusion-delay check
fails before committee resolution, deduplication, or MaxCover. They add getter,
filter, and deletion work, not the eligible raw-only quadratic mechanism.
Earlier eligible raw entries, overlapping aggregates, delayed compaction, or
an already-running aggregate merge remain distinct conditions. The retained
logs do not timestamp completed compactions or expose the proposal's pool.

## Logging can wait, but the final marker is not the missing interval

The deployed text configuration installs a prefixed `logs.WriterHook`
(`cmd/beacon-chain/main.go:199–214`). Its formatter and writer run in
`WriterHook.Fire` (`io/logs/hook.go:25–36`), outside logrus's main write mutex.
Logrus v1.9.4 subsequently also formats and writes through its ordinary output
under that mutex (`entry.go:224–258,289–304`). RPC installs a `StreamServer` in
that output (`rpc/service.go:284`; `io/logs/stream.go:43–50,74–77`), whose feed
can wait for downstream consumers. No active historical streaming consumer
has been established.

Unless explicitly disabled, the node also installs an ephemeral log file and
enables Debug globally (`cmd/beacon-chain/main.go:258–263`;
`io/logs/logutil.go:38–45,117–148`). Its lumberjack writer holds a mutex around
file opening, rotation, and writing (`lumberjack.v2@v2.2.1/lumberjack.go:135–160`).
Historical deployment of this default and any file/output wait must be checked;
source reachability is not proof of an I/O bottleneck. A comparison using a
nonblocking sink can exercise formatting and local locks while excluding those
external waits, and should state that boundary.

The bounded [deployment audit](logging-deployment-audit.md) found that the
retained Round 2 archives omit both the historical service command and the
in-container data directory. They therefore cannot establish whether the
default ephemeral debug file was enabled. The audit records its conditional
`/data/logs/beacon-chain.log` path, exact rotation settings, assembly Debug
calls, and the benchmark's sink boundary without attributing a Round 1 hook
failure to Round 2.

Node 1's completion record contains `sinceSlotStartTime=3.662413031s`, whereas
the outer emission is at slot+3.662622713s
(`runs/round2/prysm-geth-1/beacon.log:1136845`). The elapsed field is constructed
immediately after `BuildBlockParallel` returns (`proposer.go:114–118`). The
roughly 0.21 ms difference therefore rules out explaining the missing seconds
solely as the final completion message waiting to be emitted. An internal
logging wait before the builder returns remains a different, unmeasured case.

## Paced comparison alignment

Use the existing `Chose payload bid` milestone as the replay anchor. For a
recorded gossip entry at `arrivedMs`, its target offset from that milestone is
`arrivedMs * 1ms - 17.249896ms`. The first target is about 12.750 ms; historical
`arrivedMs` is integer milliseconds. Record actual entry drift and completed
validation/subscriber counts at build finish, then drain the remaining replay
outside the measured build interval.

The gossip subset has 14,283 votes for imported block 96, root
`0xc3da59207794daa351ec2af7830805288c79cb20f717f2f527d0b8f0eb537237`, and 72 for
imported block 69, root
`0xcf463de934eda0dea542f4dbceaa587867e45ea0cb7b2513e3ee5fde3e858f8f`.
The import anchors are node1 lines 1115472 and 733377. The twelve ledger
data-root groups include committee indices; they do not establish twelve BLS
signing roots. Source/target checkpoint roots and `data.index` are unlogged.
Synthetic message substitutions must be identified.

The stale block-69 votes do not automatically force state regeneration from
69: the ordinary recent-state compatibility check compares the epoch-2
dependent roots, before slot 64, and can still return head 96 if those roots
match (`process_attestation_helpers.go:36–63`;
`forkchoice/doubly-linked-tree/forkchoice.go:864–884`). A mock returning the
shared head measures that ordinary compatible case and excludes real
forkchoice/head/checkpoint lock waits.
