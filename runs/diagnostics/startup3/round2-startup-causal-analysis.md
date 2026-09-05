# Round 2: why the first block was slot 15

## Outcome and scope

Round 2 genesis was **2026-09-05 01:30:00 UTC**, with 12-second slots,
eight-slot Goldfish rounds, 32-slot epochs, and 120,000 validators. Slot 0 is
the genesis slot, not a failed normal proposal. Slots **1–14** have consistent
scheduled proposers, explicit proposal failures, and no block import in any
of the five detailed observer logs. The first block was slot **15**:

- Owner node32 began building at **01:33:00.009**, selected its execution
  payload at **00.019**, and finished building at **01.482**.
- Its BN logged the block import at **01:33:01.650**, about **181.65 seconds
  after genesis**; its VC logged submission completion at **01.659**.
- Peer node400 imported it at **01.732**, node201 at **01.885**, and node1
  only at **01:33:25.112**. Node1's later observation is not the production time.

The subsequent complete 1,000-node census finds an earlier peer import:
node331 at **01:33:01.591600**, genesis +181.5916 seconds. This refines the
earliest observed processing time without changing the slot-15 result or the
owner's own timeline above.

The directly demonstrated mechanism in the local controlled reproduction is
repeated whole-registry work during genesis-target FFG gossip validation,
causing CPU saturation, lock contention, and runnable-goroutine starvation.
The historical source, observer validation delays, and proposer/engine traces
are consistent with this same startup pressure mechanism. They do **not**
provide a historical runtime profile locating every individual wait.

## Each failed startup slot

Offsets below are relative to that slot's start; the VC deadline is +12 seconds.
“Preflight” means synchronous VC sync-committee duty discovery/signing before
proposer dispatch, not an EL or proxy check.

| Slot | Node | Directly observed reason no proposal completed |
| ---: | ---: | --- |
| 1 | 169 | RANDAO domain lookup returned a deadline at +12.002. No block-building RPC followed. Unlike the other RANDAO failures, there is no preceding logged preflight error; the split among VC scheduling/domain-lock wait and RPC servicing is unresolved. |
| 2 | 191 | Sync-committee index preflight used the entire slot. Proposer dispatch then reached RANDAO with an expired context at +12.004. |
| 3 | 22 | Same preflight-before-dispatch failure; RANDAO expired at +12.003. |
| 4 | 91 | Sync-selection **signing/domain** preflight, rather than index lookup, expired; RANDAO immediately followed at +12.004. |
| 5 | 118 | BN building began only at +10.119. Payload retrieval hit the slot deadline; no P2P fallback existed. BN logged final failure at +14.527. |
| 6 | 83 | Building began at +3.664, but execution HTTP response retrieval timed out and no P2P fallback existed. BN failed at +4.456, well before the slot deadline. Geth and proxy evidence below rule out a slow payload build as the explanation. |
| 7 | 144 | Sync-index preflight expired; RANDAO then failed on the expired slot context at +12.002. |
| 8 | 19 | BN building began only at +11.190. Payload retrieval failed at +12.007; no fallback was available. |
| 9 | 107 | Building began at +5.981; execution response retrieval timed out and no P2P fallback existed. BN failed at +7.639. Again the proxy had already logged a successful response. |
| 10 | 35 | Building began at +7.446 and a payload was selected at +10.338. The required parallel consensus-field branch did not return before the deadline; packing cancellation and state-root/build errors appeared at +36.873/+36.874. |
| 11 | 117 | Sync-index preflight expired; RANDAO then failed at +12.012. |
| 12 | 14 | Sync-index preflight expired; RANDAO then failed at +12.007. |
| 13 | 20 | Building began at +1.316, but the FCU payload ID was not logged until +10.342. Parent-state slot processing failed at +12.038, before parallel block assembly/attestation packing. |
| 14 | 85 | Building began at +2.694 and the execution payload was selected at +2.744. The joined consensus-field branch remained unfinished through the deadline; packing cancellation appeared at +43.994, immediately followed by state-root/build failure. |

The complete archive names, proposer keys, log line anchors, and source ordering
are in [the proposer reconstruction](round2-startup-proposers.md).

This gives six sync-preflight failures (2, 3, 4, 7, 11, 12), one other RANDAO
failure (1), five block-request deadline failures with different internal stages
(5, 8, 10, 13, 14), and two earlier execution-response failures (6, 9).

## The shared pressure mechanism and historical load evidence

For target round 0, `getRecentPreState` rejects the recent-head shortcut and
`getAttPreState` returns the checkpoint state at slot 0. The committee cache
does not avoid the problem: `ActiveValidatorCount` explicitly requires
`s.Slot() != 0` for its nonzero cached-count fast return. With a populated
committee cache, it therefore walks all **120,000 validators again** for each
call on that genesis checkpoint. This is work proportional to admitted FFG
messages times registry size, not an inference that block-packing input itself
was quadratic.

The checkpoint path also acquires an async keyed lock while holding a
fork-choice read lock. Concurrent gossip validators can queue there; pending
fork-choice writers can then block new readers, including proposal head/state
access. The repeated registry scan itself occurs **after** target-state lookup
has released that fork-choice lock. Its CPU demand can nevertheless starve the
goroutines that release other locks or process RPC/network readiness. These
are coupled delays, not a claim that the entire scan runs under the
fork-choice read lock.

The detailed historical logs show substantial observer-local delay, not just
votes arriving late from the network:

| Observer | Slot-1 FFG validations entered before slot end | Accept log emitted before slot end | Slot-1 cohort median / p95 entry-to-log delay |
| ---: | ---: | ---: | --- |
| 400 | 1,198 | 361 | 17.807 / 25.166 s |
| 201 | 1,326 | 157 | 16.454 / 23.734 s |

Some node201 votes entered during the first 1.6 seconds of slot 1 but were not
logged accepted until wall slot 3. These cohorts are reconstructed from
eventual acceptance logs, including rows emitted after the slot deadline;
unlogged validation calls are absent. They are not complete ingress totals.
Entry is validator instrumentation, not wire arrival. The acceptance log is
after validation gates and synchronous notification, just before the validator
callback returns; it does not establish pool insertion or fork-choice
consumption. The outer log timestamp can additionally include
logging/collection delay. Nor can observer counts be assigned to node169 or
another proposer. See [observer evidence](round2-observer-timeline.md).

The existing three-node, 120k-registry experiment supplies the causal control:
with the original path, 90.34% of sampled BN CPU was under
`ActiveValidatorCount`, and slots 1–3 failed. Removing only the repeated count
scan on the receiving/proposing BN made all three succeed, leaving vote
origination and relay unchanged. Packet and Go runtime traces showed full EL
responses already received and acknowledged, while their HTTP read goroutines
waited **325–682 ms runnable but not running** against a 300 ms timeout. This
proves scheduler starvation for that controlled reproduction, not a historical
packet-delivery measurement. See [wire/runtime evidence](wire-causation-results.md).

## Slots 6 and 9: the EL was not slow to build

Both payloads were empty (`txs=0`, gas used 0). Correlation uses the exact
payload ID and snooper request number, not an arbitrary nearby response.

| Evidence | Slot 6, node83 | Slot 9, node107 |
| --- | --- | --- |
| Payload ID | `0x04343b7fabf2c9b1` | `0x042f29fee7c5b342` |
| Geth prepared payload | 01:31:06.475, 960.326 µs | 01:31:44.421, 935.15 µs |
| Proxy request | #717, 01:31:16.147146 | #722, 01:31:54.109110 |
| Proxy response | 01:31:16.148780; HTTP 200, 2,060 B, 2 ms | 01:31:54.131275; HTTP 200, 2,060 B, 22 ms |
| BN timeout log | 01:31:16.456845 | 01:31:55.638494 |

The snooper request/response anchors are node83 `snooper-engine.log` lines
31964/31974 and node107 lines 32191/32201. Geth's `reason=delivery` payload-stop
message accompanies delivery of an already built payload; it is not an EL
process crash. Historical proxy logging alone cannot establish when the BN
kernel received the response. It does establish that a slow EL payload build
is the wrong explanation; the local packet/runtime control demonstrates how
the corresponding BN-side timeout can happen despite prompt delivery.

## Why the chain can recover without an earlier block

The target checkpoint is keyed by **root plus round**, not root alone. At
slot 8, newly generated votes target round 1. Even if the only block is genesis,
the checkpoint lookup processes empty slots to materialize a state at slot 8.
Once the committee cache is populated, this nonzero-slot state can use the
cached active count. A new Go diagnostic exercises the actual checkpoint
lookup/regeneration and verifies states 0, 8, and 16 for the same root without
mutating the earlier cached state. It passed five consecutive runs against the
current diagnostic tree; the relevant algorithm is retained in the exact
round-2 source, as established by the scoped source audit.

This explains a reduction in ongoing work, **not a hard load cutoff at slot 8**.
The gossip age gate remains epoch-based after Deneb: old round-0 votes can
remain age-valid through wall slot 63, subject to the other validation gates.
The current/previous Goldfish-round check is later in fork-choice consumption,
after checkpoint lookup; it does not protect the earlier gossip path. Node201
actually admits an attSlot-2/target-round-0 vote at 01:33:16.974 and logs its
acceptance about 29 ms later, both in wall slot 16. Old queued
and newly admitted old-round work can coexist with cheaper round-1 work.

The observed improvement starts **before the first block**. On node400, the
FFG slot-cohort p95 entry-to-log delay falls from 14.802 seconds at vote slot 8
to 1.192 seconds at slot 10 and 12 milliseconds at slot 12. Node201 likewise
falls from 16.804 seconds to 2.748 seconds and 27 milliseconds. Other observers
remain slower. This supports declining pressure and uneven recovery, rather
than a first block somehow enabling all earlier processing. The logs do not
measure the relative contribution of late-arrival decline, admission
throttling, completed work, and queue drainage.

Node32's first successful proposal was not explained by having no sync duties:
it had 596 local validators, three sync-committee members, and 77 slot-15
attesters. Some failed owners had fewer sync members. It simply completed the
necessary stages within its slot: prompt dispatch, prompt payload retrieval,
and consensus/state-root work finished by +1.482 seconds. The missing-slot
table explains the preceding failures; no protocol rule mandated waiting for
slot 15 specifically. See [round-transition source audit](round2-round-transition-audit.md).

## Remaining boundaries, especially packing

For slots **10 and 14**, attestation/consensus-field assembly really is on the
blocking path: execution payload selection succeeded, so `buildBlockGloas`
joined that branch. This differs from slots 5, 6, 8, and 9, where payload
failure returned before the join and a later packing cancellation could be an
orphaned background result.

Packing has uncancelable pool-lock waits and long loops with sparse context
checks, including potentially quadratic containment deduplication. The owner
logs do not record pool size, lock ownership, or CPU stacks. The 29–41 seconds
between building entry and eventual cancellation therefore cannot be honestly
assigned specifically to quadratic packing, pool contention, or runnable
starvation. The terminal dependency is established; that internal time split
is not. Slot 1 has the separate uninstrumented RANDAO boundary documented in
[the duty audit](round2-slot1-duty-audit.md).

The later breakdown after slot 128 must be kept separate. A VC timeout does
not necessarily mean a missing block: slot 19's submit RPC expired, but peers
imported its block. Later proposer schedules diverge, and shutdown overlaps
the end of log coverage. The [expanded archive census](round2-later-slot-audit.md)
and [slot-129 rollback explanation](round2-rollback-cause.md) establish a
separate mechanism: one stale-fork wallet held 392 of the 512 mock available
committee seats, whose votes raised the majority denominator without
supporting any descendant of the ordinary observers' justified root.

## Changes and verification

This investigation is isolated in diagnostic jj change `vpprxpww`, on top of
the earlier diagnostic changes. This change adds reports, log parsers, and a
checkpoint-state test; it applies no production fix. Diagnostic code was
authored by GPT-5.6 Sol subagents. Go-only verification, per the user's request:

```sh
GOCACHE=/tmp/prysm-diagnostic-buildcache go test -tags develop \
  -run '^TestSameRootRoundCheckpointStateDiagnostic$' -count=5 \
  ./beacon-chain/blockchain
```

Result: PASS, five consecutive runs. The parser also checks that cohort
completions before a slot boundary never exceed entries before that boundary.
