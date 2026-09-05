# Audit of the earlier startup explanation

This is a read-only evidence audit of the previous startup reports, archived logs,
and source revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`. No experiment,
test, simulator, or parser was rerun. Historical source was read with
`jj --ignore-working-copy file show -r REV PATH`; line numbers below refer to that
revision, not the modified working tree. No applicable ancestor `AGENTS.md` was
present for this report.

The earlier work identified concrete failure boundaries and a credible common
load mechanism. It did **not** establish the precise historical wait consuming
each proposer's budget. Its own experiments and qualifications are essential:
they prevent the attractive common-load explanation from becoming an invented
historical profile.

## What the seven pre-block failures establish

Genesis was `2026-09-05 01:30:00 UTC` (node169 validator.log:646). Exact source
`validator/client/validator.go:453–455` sets the deadline to genesis plus
`(slot + 1) * SecondsPerSlot`. `runner.go:104–105` creates the slot context;
`:147–156` calls `RolesAt` synchronously, then creates the role context with
the **same absolute deadline**. Dispatch does not grant another twelve seconds.
`RolesAt` logs a sync-selection error and returns the already collected roles
with a nil error (`validator.go:646–649`), so the runner dispatches those roles
even when the deadline has already expired.

Raw owner log pairs independently checked in this audit:

| Slot / node | Sync-preflight terminal line and UTC time | Proposer RANDAO terminal line and UTC time |
| --- | --- | --- |
| 2 / 191 | 770, `01:30:36.001289585`, cannot fetch sync subcommittee index | 780, `01:30:36.004480876` |
| 3 / 22 | 775, `01:30:48.000619225`, cannot fetch sync subcommittee index | 800, `01:30:48.003364340` |
| 4 / 91 | 832, `01:31:00.003838012`, cannot sign selection data | 839, `01:31:00.003910654` |
| 7 / 144 | 1110, `01:31:36.001455039`, cannot fetch sync subcommittee index | 1122, `01:31:36.002488933` |
| 11 / 117 | 1113, `01:32:24.001545879`, cannot fetch sync subcommittee index | 1130, `01:32:24.012453541` |
| 12 / 14 | 1368, `01:32:36.000792396`, cannot fetch sync subcommittee index | 1391, `01:32:36.007049729` |

Each RANDAO error is `could not get domain data: rpc error: code =
DeadlineExceeded desc = context deadline exceeded`. The archive path for node N
is `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-N.tar.gz`, member
`./validator.log`. Nodes169,191,22 also have directly readable logs under
`/tmp/prysm-r2-extra-logs.Rd7MjT/extracted/round2-N/`.

The directly established causal dependency is that role discovery had not
returned before the proposal deadline, and the proposer subsequently failed
its prerequisite using an expired budget. The logs do **not** show that the
particular failing sync-index/domain RPC occupied all twelve seconds. Before
sync preflight, the runner can spend time in proposer-settings work, and
`RolesAt` iterates duties and performs ordinary attestation-aggregator selection
(`runner.go:128–136`, `validator.go:572–646`). Neither runner entry, `RolesAt`
entry, nor individual historical preflight RPC entry is logged.

Slot4 is more specific than “sync signing failed.” Node91 validator.log:5 says
`keymanagerKind=direct`. In exact source, `signSyncSelectionData` first obtains
DomainData (`sync_committee.go:237–239`), computes a signing root, then invokes
the direct keymanager. `validator/keymanager/local/keymanager.go:178–189`
only reads its local key map and calls BLS signing; its errors are local
missing-key errors, not gRPC statuses. Thus the logged gRPC deadline in this
signing preflight is source-attributable to the DomainData path, not a remote
signer or slow BLS signature producing that status. The historical division
between prior work, VC domain-lock waiting, and DomainData RPC time remains
unknown.

For slots2,3,7,11,12 the index-handler source has real state/locking work:
`rpc/prysm/v1alpha1/validator/sync_committee.go:52–63` looks up the key and calls
`HeadSyncCommitteeIndices`. `blockchain/head_sync_committee_info.go:61–84,135–172`
gets a per-slot `syncHeadState` multilock; cache misses fetch head state/root and
process slots. The lookup also uses `headLock`, not directly the fork-choice
lock (`chain_info.go:185–187,238–251,355–361`). Multilock acquisition and
deferred cleanup can contend through the package-global registry
(`async/multilock.go:22–28,37–66,93–123`). These are source-supported candidate
waits, not observed historical lock owners.

## Slot1: the exact boundary and an omitted wait

Node169's slot1 proposer was scheduled before genesis work completed:
validator.log:650 names `0xa7120c370e8c`. Slot0 submissions were reported by
`:681–685` at `01:30:09.017946986–09.019213469`. These are completion reports,
not exact signing timestamps: the attestation report itself says
`submittedSinceSlotStart=3.293s`. Successful startup signing proves earlier
usability, not sustained low latency or retained domain-cache entries.

At validator.log:686, `01:30:21.915067414`, a slot1 payload duty logs “no block
for slot.” This proves dispatch had occurred by then. At :694,
`01:30:24.002452348`, the scheduled proposer fails RANDAO DomainData. Source
`propose.go:62–97` returns immediately at this failure before graffiti, head
hint, and the block-request RPC. This is stronger than inference from absence
of BN logs: that attempt could not invoke block construction or GetPayload.

The previous duty-accounting audit found all 75 attesters, four PTC duties, two
sync messages, and the aggregators failing or skipping before their later
signing domains. This audit spot-checked the raw failure interval and source;
the exhaustive key counts remain the earlier parser's result, not a new count.
Their common terminal deadline does not prove a common stalled BN handler.

The ordinary RANDAO DomainData handler is constant-sized and does not acquire
head/fork-choice/checkpoint-state locks: `server.go:176–201` reads static fork
configuration and genesis validators root and computes the signing domain.
The voluntary-exit exception does not apply. Calling this an expensive
DomainData computation is contradicted by source.

Conversely, the previous report's split between scheduling, the VC domain lock,
and RPC servicing is not exhaustive. `propose.go:54–56` acquires a proposer
multilock **before** RANDAO. Even without an earlier same-key proposal, its
global registry can contend with other VC role locks. No historical entry or
lock marker excludes time there. The source proves a cold/missed domain request
ultimately returned a deadline; it does not identify the earlier elapsed time.

The VC cache configuration is source-confirmed (`service.go:157–161`,
MaxCost 192, no IgnoreInternalCost). The miss path uses a single RWMutex across
the RPC and asynchronously sets the cache (`validator.go:721–755`). Detached
subnet work uses a background context (`duties.go:764–775`) and 16 workers over
current/next duties expanded across repeating rounds (`subnets.go:29–84`).
Those are viable competitors. The prior diagnostic's “three retained domains”
result establishes effective capacity for that diagnostic configuration; it
does not log node169's historical contents, evictions, lock ownership, or how
long background work overlapped slot1.

Node169's EL was already building a payload: execution.log:83–84 records
payload `0x0466ae591b684070` in 716.623 microseconds. Snooper request 709 at
`:31640`, `01:30:12.166999306`, returned VALID by `:31671–31675` around
`01:30:12.167426876`. That is scheduled prebuild work, not this failed
proposer's GetPayload. It refutes an unavailable EL as the direct RANDAO cause.

## Shared pressure: three different strengths of evidence

**Historical observation.** Observer node201 beacon.log:1816 records a round0
vote with attSlot1 and arrivedMs934, giving validation entry
`01:30:12.934`, while its outer log is `01:30:37.706454347`: about 24.772 seconds
later. Node400 :3260 gives entry `01:30:22.382` and logging
`01:30:51.239969989`: about 28.858 seconds. These are real observer-local
validation-to-log envelopes, not merely late network arrival. They include
validation gates, synchronous `OperationFeed.Send` before the accepted log
(`validate_beacon_attestation.go:229–245`), scheduling, and logging/collection.
They do not measure a pure count scan or transfer those observers' load to
node169 or another owner.

**Source-supported workload and conditional coupling.** Accepted gossip reaches
`AttestationTargetState` then committee validation
(`validate_beacon_attestation.go:143–149,253,290`). Round0 excludes the recent-head
shortcut (`process_attestation_helpers.go:23–27`); genesis checkpoint state is
slot0. With a populated committee cache, `ActiveValidatorCount` still requires
`s.Slot()!=0` for its constant-time return, so slot0 goes through the registry
iteration (`helpers/validators.go:145–174`). The checkpoint lookup holds a
fork-choice reader while acquiring the root-plus-round multilock
(`receive_attestation.go:49–52`, `process_attestation_helpers.go:109–120`). The
count scan occurs **after** that reader is released. CPU demand from callers
already scanning can slow readers still draining the multilock and can delay
network/RPC goroutines. This establishes reachable code and expensive work,
not historical CPU saturation, writer preference activation, or a particular
owner's causal lock chain.

**Prior experimental causality.** The existing B/D, E1/F1, H/I2 experiments
provide progressively stronger controls, as documented in reproduction-results,
early-gossip-results, and wire-causation-results. B measured a long proposal
head-root lock wait; E1 measured a 5.244-second RolesAt interval and sync-head
cleanup/global-registry waits; H tied prompt complete EL packets and kernel ACKs
to 325–682 ms of runnable Go goroutines against 300 ms timeouts. Ablating the
receiving BN's repeated count work in I2 removed the local failures. These are
actual experiments, not mere theory, but they used a different topology,
four-slot rounds, diagnostic instrumentation, synthetic gossip load, and
different runtime schedules. They prove that mechanism in those experiments.
They do not turn historical proxy timestamps into packet captures, or assign
the reproduced 90.34% CPU share to the 1000-node run.

The E1 real RANDAO RPC took 0.340 ms (raw
`/tmp/prysm-startup3-round4-early-e1/validator3.log:39870–39873`) and complete
RANDAO took 0.742 ms (:39884). Its block request then failed (:40529). Thus E1
did not reproduce slot1's RANDAO failure. The separate 4.272-second raw DomainData
probe bypassed the VC mutex and includes probe-side scheduling/output-lock
effects, so it cannot identify node169's historical wait either.

## Why the round transition supports recovery but does not date its cause

The checkpoint cache is keyed by root plus round. A round1 checkpoint with the
unchanged genesis root is processed to slot8 (`process_attestation_helpers.go:
109–174`); with a populated committee cache the active-count fast path becomes
available. Node400 beacon.log:9000 actually records attSlot8/targetRound1 in
wall slot8, with about 75 ms entry-to-log lag. Later-round traffic did not need a
first produced block or wall slot12 to become admissible.

There is no hard cutoff for expensive old-round gossip at slot8 or16. The
post-Deneb gossip age gate is current/previous **epoch**
(`helpers/attestation.go:177–185`), allowing epoch0 votes through wall slot63.
The current/previous-round test occurs later, even after target-state lookup in
`OnAttestation` (`process_attestation.go:60–66`). Node201 beacon.log:35813
actually records attSlot2/targetRound0 entry at `01:33:16.974` and logging at
`01:33:17.002933293`, both in wall slot16. Old expensive work was still
admissible while the chain recovered.

The earlier observer tables report low latency in nodes201/400 before the first
slot15 block. That contradicts “the first block itself cleared all backlog.”
The source transition explains an available cheaper path; the logs do not
partition recovery among changing arrival/admission rates, throttling, finishing
old work, or draining runtime/lock queues. It cannot derive why node32 succeeded
specifically at slot15 while previous owners failed.

## Corrections to retain in the final explanation

- `round2-slot1-duty-audit.md` cites schedule line653; raw node169 slot1 schedule
  is650. Line653 is slot4.
- `round2-slot1-cause-deep-audit.md:136` says E1's “real proposer succeeded.” Its
  RANDAO succeeded; its slot1 block request failed (raw E1 :40529).
- That report's conclusion at148–153 states genesis scan pressure degraded
  node169 servicing as historical fact, although141–146 admits the decisive
  markers are absent. State it as the leading source/experiment-supported
  explanation, not the exact historical cause.
- “Preflight consumed the whole slot” should mean dispatch remained behind
  preflight until the deadline. The logs do not time its individual call or
  exclude earlier runner/role-selection work.
- A three-entry effective cache, detached background work, a common deadline,
  or a prompt EL response does not by itself prove historical eviction,
  a seconds-long mutex holder, common handler blockage, or kernel delivery.

The tightest causal statement for these seven attempts is: an absolute next-slot
deadline governed role discovery and proposal prerequisites. Six attempts were
still inside synchronous sync-role preflight when that deadline expired, so
their subsequent proposers inherited no budget. Slot1 dispatched roles before
the deadline but its proposer failed DomainData before any block construction.
The exact source contains several routes from genesis gossip load to impaired
servicing, and earlier controlled experiments demonstrate those routes can
cause failures. Historical runner/RPC start markers, per-stage timing, cache/lock
ownership, runtime profiles, packet captures, and owner-specific ingress are
missing; without them, selecting one underlying wait for each historical
attempt exceeds the evidence.
