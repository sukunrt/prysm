# Slots 0–3 diagnosis

Scope: the 10 saved round1 nodes and 12 saved round2 nodes, not all 1,000
participants. Round1 ran `a1679c9fd82a47b3cea16ca65c84d2c4d4501fcb`;
round2 ran the older `0280403c70d88967f49d2d4c730f4c5417dabdf5`.
No production fix has been applied. Diagnostics live in jj change `novuklnx`.

Status: the observed round1 slot-1 failure mechanism is established, but the
underlying cause of its multi-second pre-build latency is **not yet proven**.
The other five early proposer attempts are outside the saved validator-log
sample. Diagnostic mechanisms and fast local benchmarks must not be presented
as a complete root cause for all six missed proposals.

Deployment clarification from the user: one node per machine, 800 machines
limited to 20 Mbps outbound / 50 Mbps inbound and 200 unlimited; otherwise
the same resources. No exact CPU quota, RAM limit, disk latency, or utilization
is recorded in the supplied logs. Neither co-location of the 1,000 nodes nor
CPU/RAM saturation is an established explanation. The saved node numbers have
not been mapped conclusively to the two bandwidth classes.

## Established by the logs

Slot 0 is genesis, not a missed ordinary proposal: `validator/client/propose.go:47`
explicitly returns for slot 0. Gossip validation also ignores slot-0 FFG and
available attestations (`beacon-chain/sync/validate_beacon_attestation.go:86,572`).
Local submission records do not contradict those gossip rules.

Round1 node3's slot-1 proposal has a complete failure timeline (UTC):

| Time | Evidence |
|---|---|
| 00:00:12.114 | Geth starts slot-1 payload construction (`execution.log:98`). |
| 00:00:12.115 | Geth reports an updated payload, elapsed 547 microseconds (`execution.log:99`). |
| 00:00:18.973 | Beacon RPC logs `Building block`, already 6.973 seconds into slot 1 (`beacon.log:902`). |
| 00:00:24 | Validator's request times out at the slot deadline (`validator.log:687`). |
| 00:00:30.10 | Beacon reaches graffiti generation (`beacon.log:906`). |
| 00:00:31.52 | Local payload preparation encounters the canceled context; no P2P fallback exists (`beacon.log:910–911`). |

Paths in that table are under `runs/round1/prysm-geth-3/`. Snooper also records
the slot-1 forkchoice update returning VALID with a payload ID in 2 ms
(`snooper-engine.log:20851,20874`). No getPayload request reaches the snooper
during slots 0–3. Thus this observed proposal fails before publication; the
final payload error is downstream of a missed beacon/validator deadline, not
evidence that Geth took 19 seconds to build a payload.
The early FCU was proactive preparation before the proposer RPC entered; its
fast response does not measure the later proposal's payload retrieval.

The approximately 11.1-second Building-to-graffiti gap covers the server's
optimistic-status check, parent/head preparation, empty-block construction,
and possible scheduling/logging delays. `getParentState` synchronously calls
`UpdateHead` (`beacon-chain/rpc/prysm/v1alpha1/validator/proposer.go:192`).
Existing logs cannot attribute the entire gap to an individual function or lock.
The earlier 6.97 seconds before the handler log is a separate uninstrumented gap.

The small accepted-vote summaries do not imply small incoming traffic. A
separate full-log analysis (`ffg_slot1_windows.md`) compares each slot-1 vote's
validation-entry offset with completion-log emission. It finds substantial
backlogs on individual nodes before the six-second mark. Consequently, the
packing hypothesis and the broader incoming-validation-load hypothesis must
be kept separate: accepted pool candidates can remain few while many
validation calls are pending. The logs do not separate validation computation,
lock waits, subscriber waits, scheduler delay, and completion-log output delay.

Round2 has no saved early proposer attempt from which to reconstruct the same
timeline. It does have direct evidence of receiver/validator degradation:
slot-1 submissions succeed, followed by hundreds of slot-2 and slot-3
attestation-data RPC deadlines. See `early_slots_findings.md` for counts and
`runs/round2/prysm-geth-1/validator.log:687,690` for concrete failures.
Do not infer submission time from log emission: `submittedSinceSlotStart`
records the actual submission offset and can precede emission substantially.

## Reproduced/code-established defects, not proven run-wide causes

1. Same-root genesis payload-status changes are logged and notified as reorgs.
   Genesis parent root is zero, so `saveHead`'s parent-vs-old-head comparison
   enters its reorg branch even though old and new head roots are equal
   (`beacon-chain/blockchain/head.go:104`). Such events occur in both runs.
   Its synchronous state-feed send (`head.go:134`) executes under UpdateHead's
   fork-choice write lock (`receive_attestation.go:128`). A slow feed subscriber
   can therefore hold up fork-choice readers, including validator RPCs.
   This is a real lock-amplification path; the logs do not prove a subscriber
   was the particular blocker during the observed 11-second gap.

   A further historical-code audit establishes the status sequence as
   `full -> empty -> full`. In the actual Gloas path, the asynchronous FCU
   takes a fork-choice read lock before sending the Engine request, then
   releases it before network I/O. The first Engine request therefore proves
   that the first update had already released its write lock: by 00:00:12.114
   in round1 node3 and 01:30:12.004274 in round2 node1. It cannot be the
   multi-second lock holder after those times. Round1's second update also
   released before the FCU recorded at second-resolution 00:00:25, while
   graffiti was not reached until 30.10. Later writers and other parent-state
   work remain uninstrumented. See `genesis_head_timeline.md`; the interval
   between two reorg logs must not be treated as one measured lock hold.

2. Exact-target skip-state cache hits are ignored. Both comparisons in
   `beacon-chain/core/transition/transition.go:238,247` require cached slot to be
   strictly less than requested slot. Identical requests can wait and then
   recompute the result they just retrieved. The synthetic 120,000-validator
   Heze benchmark demonstrates repeated work and allocation. A cold copied
   genesis state costs approximately 30–41 ms and 172 MB per transition;
   a root-warmed parent costs approximately 50–240 microseconds. Runtime
   genesis is decoded again during startup, so hashing an earlier instance
   does not guarantee the runtime cached instance is warm. Full measurements
   and limitations are in `genesis_bench_results.md`.

Neither finding alone establishes the multi-second run stall. In particular,
1,000 network nodes are not 1,000 concurrent state-transition requests on one
beacon node. FFG attestation-data requests do not advance state during slots
0–3 because all are in the genesis round (`rpc/core/validator.go:602`).
There is no live-chain epoch/round boundary in this interval, but startup
duties speculatively request the next epoch and can advance copies from slot
0 to 32. Four parallel next-epoch duty endpoints reproduce 792–800 ms of
cold transition work and approximately 990 MB allocated at 120,000 validators.
A slot-1 request sharing their genesis cache key can wait behind this work
(284–309 ms for the combined two-request benchmark; not an isolated RPC
latency measurement). This is a measured startup amplifier, not
proof of the later slot-1 stall: node3 logs its duties schedule by 00:00:02.45,
well before the proposal's 00:00:18.973–30.10 gap. State copying alone is not
a full deep copy of the 120,000-validator registry.

The actual SSE event subscription channel is unbuffered: its allocation uses
`len` of a newly created outbox, which is zero, rather than its capacity.
However, its receive loop usually only queues lazy serialization, and a full
outbox causes disconnection. Xatu's observed topic set does not include the
expensive payload-attributes reader. No specific subscriber deadlock has been
established. Xatu does log committee refetches following the false reorgs and
thus confirms additional beacon requests, not their cost. Its separate
`Failed to send events upstream` errors identify the Xatu-to-central-collector
exporter (`output_name=grpc`, `output_type=xatu`), **not beacon RPC failures**.
Those exporter timeouts do not prove that a beacon feed subscriber blocked.

The validator dispatch path has another concrete dependency: `RolesAt` must
finish before any proposer role is launched. It includes serial local
attester-selection work and synchronous sync-subcommittee-index RPCs for
local sync-committee keys (four on round1 node3). This can delay proposal
dispatch, but there are no timings to assign the observed 6.97 seconds to it.
The mid-epoch next-duty retry is asynchronous. Details and the distinction
between a terminal cancellation error and the unresolved latency cause are
in `proposer_startup_audit.md`.

Additional bounded probes did not reproduce the delay: 2,500 readers
exercising the production checkpoint-state lookup and fork-choice lock order
against a warmed 120k state drained and let a writer proceed in 1.645 ms.
The corrected timer starts immediately before releasing all readers' gate
and ends at writer-acquisition notification; it does not assume the writer
goroutine was already scheduled. The warmed committee/data/BLS
validation core handled 2,500 signed votes in about 254 ms on four logical
processors. These measurements do not include the entire live gossip/RPC/SSE
pipeline or the original machines' resource conditions; they are not proof
that production had spare CPU. See `gossip_startup_bench_results.md` for the
additional pool probes and their exact scope.

The expanded probe accepts all 15,000 encoded Electra singles through
production gossip validation, the verifier routine, a draining unbuffered
operation-feed subscriber, and the legacy pool in 997 ms at `GOMAXPROCS=4`
(2.855 s at `GOMAXPROCS=1`), allocating about 550 MB. It still mocks the
chain's fork-choice/target-state answers and excludes real networking and
other concurrent services. Six concurrent full-committee pool scans and
general aggregation merges take about 609 ms at `GOMAXPROCS=4`. Neither is
a reproduction of the observed delay, and neither justifies declaring the
original hardware sufficient. The source contains quadratic algorithms, but
the existing evidence does not identify them as the cause of slots 1–3's
missing proposals.

## Ten-node local controls

Both controls use the cached historical round2 beacon/validator images, not a
newly rebuilt client. All ten nodes import the same block at each of slots
1, 2, and 3.

| Validators / keys per client | Slot 1 import offset | Slot 2 | Slot 3 |
|---|---:|---:|---:|
| 130 / 13 | 107–138 ms | 68–75 ms | 68–78 ms |
| 5,960 / 596 | 630–849 ms | 174–286 ms | 162–342 ms |

The second control matches the large run's supernode key count and roughly
75 FFG duties per validator client per slot. It does not match the full
120,000-validator state, six 2,500-seat committees, network gossip fan-in,
deployment resource limits, or Xatu's SSE/refetch traffic. Both controls have
empty execution payloads (zero transactions, gas used, and blobs). This rules
out neither a large-scale bottleneck nor a subscriber-dependent stall, but
shows that 596 local keys alone are insufficient to reproduce the failure.
The per-run reports retain concrete counts and configuration limitations.

## Fix candidates for later approval

Separate payload-status updates from actual block-root reorg notifications;
remove synchronous subscriber delivery from the fork-choice critical section;
reuse exact-target skipped states with correct in-progress ownership; and
initialize the actual cached genesis state's hashing structures before the
first requests. Each needs targeted regression coverage and measurement.
These are scoped candidates, not a claim that applying them will resolve the
entire simulation failure. No diagnosis of the later chain-collapse phase is
made here.

## Verification

The diagnostic reorg/feed test passes with:

```sh
GOMAXPROCS=4 go test -tags develop ./beacon-chain/blockchain \
  -run '^TestDiagnosticGenesisFullFlipBlocksForkchoiceOnStateFeed$' \
  -count=1 -timeout=60s -v
```

It deliberately withholds subscriber delivery, observes that a fork-choice
reader cannot proceed, and verifies that draining the reorg event releases it.
This reproduces the dependency, not the original simulation's subscriber load.
All three blockchain diagnostic tests also pass together with
`GOMAXPROCS=4 GOCACHE=/tmp/prysm-diagnostic-buildcache go test -tags=develop ./beacon-chain/blockchain -run '^TestDiagnostic' -count=1 -timeout=90s`
(4.199 seconds). Four existing slot-transition/cache tests also pass via `go test`. Synthetic
benchmark commands and results are recorded separately. No Bazel build is
needed to reproduce these final checks.

After correcting the reader-convoy timer, its focused test passes again
(2.190 seconds). The broader gossip benchmark also passes with two iterations
after correcting its per-iteration state reset (936.40 ms/op); it is not
restricted to one successful iteration followed by duplicate-vote rejection.
