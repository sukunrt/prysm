# Round 2 startup: state and payload deep audit

The [continued payload code investigation](../round2-slots-0-16/payload-code-root-cause.md)
adds the real HTTP-reader scheduling reproduction and error-translation
recovery test. P2P recovery exists in code; these failures have no matching
cached bid available to that path.

This audit follows slots 5, 8, and 13 across the owner VC, BN, Engine API
snooper, and geth logs.  Genesis is `2026-09-05T01:30:00Z`; offsets below are
from the relevant 12-second slot start.  Source references are to exact Round 2
revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`.

## Stage table

| slot / owner | VC/BN request entry | engine boundary | terminal boundary |
| --- | --- | --- | --- |
| 5 / node118 | VC's first slot-tagged role output is at +10.094; BN `GetBeaconBlock` enters +10.120 (`validator.log:932-933`, `beacon.log:524`) | `engine_getPayloadV6` request +10.138, proxy response +10.142 (3 ms; `snooper-engine.log:31710-31720`) | BN reports HTTP-client timeout +12.012, then no matching cached P2P bid +14.527 (`beacon.log:527-528`); VC deadline +12.002 (`validator.log:971`) |
| 8 / node19 | first slot-tagged VC output +11.189; BN enters +11.191 (`validator.log:1402-1406`, `beacon.log:533`) | request +11.197, proxy response in the same millisecond (`snooper-engine.log:32102-32112`) | BN timeout +12.008 and no fallback +12.053 (`beacon.log:536,540`); VC deadline +12.003 (`validator.log:1419`) |
| 13 / node20 | BN enters promptly at +1.317 (`beacon.log:527`), proving the block RPC was issued early | no `engine_getPayloadV6` occurs for this attempt. A separate late-slot FCU request occurs +6.085 and gets a 1 ms response; its BN completion log is delayed until +10.343 (`snooper-engine.log:32149-32178`, `beacon.log:530`) | `getParentState` exits from `ProcessSlots` cancellation +12.038 (`beacon.log:531`); VC observes deadline +12.008 (`validator.log:1221`) |

## Slots 5 and 8: the payload was not slow in geth

Both payload IDs already existed before block construction.  Node118 logs its
FCU/payload ID at 01:30:58.010, 1.990 seconds before slot 5
(`beacon.log:522`).  Node19 logs its slot-8 ID at +0.005
(`beacon.log:531`).  Thus neither handler spent its remaining budget creating
a payload ID.

More decisively, the snooper completed the corresponding `engine_getPayloadV6`
response in 3 ms for slot 5 and under its millisecond-resolution duration for
slot 8.  Yet the BN did not return from the call before its nominal 300 ms
Gloas deadline: the gaps from the snooper response header to the BN timeout log
are about 1.870 seconds and 0.810 seconds.  Exact-R2
`beacon-chain/execution/engine_jsonrpc.go:303-326` wraps this call in a 300 ms
context and maps the failed `CallContext` to `timeout from http.Client`.

Therefore the phrase “payload retrieval exceeded the deadline” identifies the
BN client side, not EL computation.  The directly located interval is after
the proxy had obtained/logged the EL response and before the Go caller resumed
with a decoded result or timeout.  The historical proxy log alone cannot split
socket delivery from Go HTTP transport/read/decode scheduling.  It does rule
out geth taking 0.8--1.9 seconds to construct the response.  The fact that even
the 300 ms timeout was observed late is consistent with severe process
scheduling starvation, but there is no historical runtime trace for these two
specific BNs, so that final attribution remains an inference.

Their primary loss was already late dispatch.  The first visible VC role work
and BN handler entry are after +10/+11 seconds; only 1.88/0.81 seconds remained
before the slot boundary.  Numerous attestation errors arriving together at
the deadline corroborate a process-wide late burst.  The payload call then
consumed that residual budget at the BN-client boundary.  The orphaned packing
errors at +28.842 (slot 5) and +20.468 (slot 8) are not the cause: exact-R2
`proposer_gloas.go:25-43` starts packing concurrently but returns immediately
when both local-payload and P2P fallback fail, without waiting for that
goroutine.

## Slot 13: parent-state processing, not payload retrieval

Exact-R2 `proposer.go:54-114` orders `getParentState` before empty-block setup,
proposer-index calculation, `BuildBlockParallel`, and therefore before
`getLocalPayload`.  Its `getParentStateFromReorgData` obtains a head state and,
when the head is behind the requested slot, invokes
`ProcessSlotsUsingNextSlotCache(..., slot)` (`proposer.go:150-199`).

Node20's terminal error names that exact call: `Could not process slots up to
13: ... context canceled`.  Since the handler entered at +1.317 and exited at
+12.038, the combined `getParentState` region occupied about 10.721 seconds.
The logs do not divide this between `UpdateHead`, head/root access, cache
lookup, skip-slot in-progress wait, and the actual transition loop.  They do
prove that the call entered the slot-processing branch and that no payload
read was reached.

There is a concrete serialization point inside that branch.  Exact-R2
`ProcessSlots` first calls `SkipSlotCache.Get` and then marks the state-derived
cache key in progress (`transition.go:211-259`).  `SkipSlotCache.Get` polls
while another calculation for the same key is in progress, returning the
caller's context error if its deadline expires
(`beacon-chain/cache/skip_slot_cache.go:59-105`).  Genesis-root state requests
from concurrent duties, attestations, and proposal preparation can therefore
queue behind one calculation.  The observed wrapper text is compatible both
with cancellation in that cache wait and cancellation during
`ProcessSlotsCore`; it does not distinguish them.  The separate next-slot
cache also takes a mutex while selecting and copying a cached state, but the
logs expose neither its hit status nor its lock wait.

The FCU for `nextSlot=13` must not be charged as an Engine API stage of this
proposal handler.  It is the independent `lateBlockTasks` payload-preparation
path (`blockchain/process_block.go:1221-1280`); `GetBeaconBlock` cannot request
payload attributes while it remains before `BuildBlockParallel`.  Its timing is
still useful system evidence: the proxy returned FCU in 1 ms at +6.086, while
the BN did not record/cache the payload ID until +10.343, a roughly 4.257-second
post-proxy gap.  Geth started and updated the empty payload in under a
millisecond (`execution.log:100-101`).  This again places long wall time on the
BN/proxy-to-Go side rather than in EL execution.

The requested transition starts from a genesis head (`headSlot=0` in the FCU
log) and must materialize skipped slots through 13 over a 120k-validator state.
That work can be CPU-heavy and contends with the already established gossip
workload, but the historical line does not expose per-slot transition times or
a lock owner.  A standalone `ProcessSlots` benchmark would remove precisely
the concurrent workload whose interference is at issue, so it would provide a
baseline rather than identify this 10.7-second interval.

That baseline was measured with the real Heze/Gloas-shaped state and production
`ProcessSlots`/`SkipSlotCache` code in
`core/transition/genesis_120k_diagnostic_test.go`, with `SlotsPerRound=8` and
both Gloas and Heze active at epoch zero.  The earlier exploratory numbers had
precomputed the state's Merkle root and used the ambient test fork schedule;
they are not used here.

With an uninitialized Merkle tree, one invocation for targets 5, 8, 10, 13,
and 14 took 34.2, 31.8, 37.7, 33.9, and 39.6 ms.  Sixty-four simultaneous
same-state-key invocations took 1.02, 1.14, 1.13, 2.11, and 1.14 seconds total,
and reported roughly 11.3--12.1 GB of total allocations across state copying,
hashing, and transition work.  No allocation profile assigns that total to
`Copy` or any other individual operation.  Every caller nevertheless reaches
the pre-gate copy/hash region before it reaches the shared skip-slot gate.
With the state Merkle cache warm,
the corresponding single calls took 0.88, 11.1, 11.0, 12.4, and 16.4 ms, and
fanout 64 took 5.0--262.5 ms.  These are one-iteration diagnostic timings on
different hardware, not production benchmarks.

The counterfactual is nevertheless large enough to rule out an intrinsic
10-second single skipped-slot transition caused merely by a 120k registry,
dirty Merkle root, or target slot 13.  It also reveals a concrete coupled-load
amplifier: state copies happen before `SkipSlotCache.Get`, so a fanout can
allocate and hash many large copies while waiters serialize behind the
in-progress leader.  Its amplification to the historical deadline still
requires concurrent load or a slow/descheduled leader.  Concurrent historical
FFG state work and CPU saturation supply such a mechanism, but the isolated
fanout deliberately does not recreate the exact vote mix and therefore cannot
select which historical leader held the key.

## Causal conclusions and remaining boundary

1. Slots 5 and 8 are late VC/BN dispatches followed by BN-side failure to
   consume already-fast EL responses within the small remaining budget.  They
   are not slow-payload-construction failures.
2. Slot 13 is an early block RPC that remains in parent-state preparation and
   reaches neither parallel packing nor payload retrieval.  Its error directly
   locates cancellation in skipped-slot state processing, while the internal
   `UpdateHead`/cache/transition split remains unlogged.
3. Across all three, the EL and proxy respond promptly while BN-visible
   progress occurs seconds later.  Combined with the reproduced active-count
   CPU/lock pressure, runnable starvation is a coherent shared mechanism.  The
   historical records prove the stage boundaries and exclude EL compute as the
   long stage; they do not provide per-owner scheduler profiles sufficient to
   assign every gap to CPU versus a particular Go lock.

## Diagnostic command

The Go-only repository workflow (explicitly used instead of Bazel) passed:

```text
GOCACHE=/tmp/prysm-diagnostic-buildcache go test -tags develop \
  ./beacon-chain/core/transition -run '^$' \
  -bench '^BenchmarkGenesis120KProposalSlotPreparation$' \
  -benchtime=1x -count=1
```

The full cold/warm benchmark passed.  No production code was changed.

## Retained-real-genesis cross-check

`BenchmarkRetainedGenesisProposalSlotPreparation` loads the public retained
120k-validator Heze state from
`/tmp/prysm-startup3-wire-h/bundle/network-configs/genesis.ssz` (or
`PRYSM_STARTUP_REAL_GENESIS_SSZ`) together with its sibling `config.yaml`.  It
skips if either file is unavailable and asserts the 120,000-validator count,
the retained reproduction's four-slot rounds, and epoch-zero Heze activation.  It never reads or prints a
mnemonic or private key.

For target slot 13, one dirty-Merkle request took 64.3 ms and fanout four took
215 ms; one warm-Merkle request took 34.9 ms and fanout four took 120 ms.  This
retained fixture therefore does not turn the isolated transition into the
historical roughly ten-second operation.  It strengthens the conclusion that
the historical delay required concurrent contention or descheduling.  These
remain single-iteration local timings, not production performance estimates.

The config loader emitted warnings for newer unrelated inclusion-list fields
unknown to this diagnostic revision, but loaded the relevant values checked
above and the SSZ decoded successfully.

## Sync-subcommittee RPC lock boundary

The exact request path is
`GetSyncSubcommitteeIndex -> HeadSyncCommitteeIndices ->
getSyncCommitteeHeadState`.  For each requested slot, the latter first takes
an `async.NewMultilock("syncHeadState-<slot>")`.  Although logical keys differ,
lock creation and cleanup pass through the package-global multilock manager;
the E1 stacks directly observed waiters in both its `Lock/getChan` and
`Unlock/Clean` paths.

On a per-slot cache miss, the method calls `HeadState` and then `HeadRoot`.
Each separately takes `Service.headLock.RLock`.  `HeadState` retains that read
lock while `s.head.state.Copy()` takes the native state's `b.lock.RLock` and
copies/references its multi-value fields.  It then advances the copy using
`ProcessSlotsUsingNextSlotCache` and inserts it in the per-slot sync-head
cache.  A cache hit avoids all of those head/state/transition operations, but
does **not** avoid the outer async lock or its deferred global cleanup.

Thus the preflight failures seen at slots 2, 3, 4, 7, 11, and 12 have two
source-distinct contention boundaries: the BN's application-level global
async-lock manager, and (on a miss) the head/state-copy/slot-processing path.
They are not explained by the VC's domain-data mutex: that mutex protects a
different RPC/cache path.  E1's matched warm-hit request spending about 4.795
seconds in deferred unlock/cleanup, plus the blocked global-manager stacks,
directly proves the first boundary can consume proposal budget.  It does not
prove every historical preflight timeout used the same boundary, because the
historical build lacks these phase timers and a miss could also wait in the
second path.
