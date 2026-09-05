# Round 2 failure analysis

The continued investigation of slots 0–16 is in the
[current causal explanation](../round2-slots-0-16/best-causal-explanation.md),
with the complete owner census and bounded reproductions. The P2P fallback
path exists in code; the payload failures below had no matching cached bid.

## Findings

Round 2 contains two distinct failure phases:

1. **Startup: no block in slots 1–14.** The first block was slot 15, first
   imported in the complete archive set on node331 at genesis +181.5916
   seconds (the proposing BN's import log follows at +181.650). The proposer histories
   show preflight/RANDAO deadlines, execution-response failures, and block
   construction that outlived its deadline. The shared genesis-target gossip
   workload has been causally reproduced locally: the slot-zero active-count
   cache bypass causes repeated 120k-validator scans and severe interference
   with proposal/RPC servicing.
2. **Later: a widespread rollback at slot 129, followed by sparse branches
   and proposer disagreement.** The decisive Goldfish vote split was not 392
   independent machines disagreeing with 120 others. One wallet, node62,
   supplied all 392 votes for an old fork; node61 supplied the other 120.
   This followed node62's failure to obtain the separate slot-117 execution
   envelope, and the mock committee's contiguous-index selection amplified
   that one node's view into a majority of the committee.

The archive set now covers all **1,000 round-2 nodes**, including detailed
logs from nodes 1, 50, 100, 150, 201, 400, 600, and 800. The proposer and
other-node archives supply complementary info-level histories. Log absence on
one detailed observer is not equivalent to a block never being produced.

Across slots 1–226, the union contains no imported block for 31 slots:
**1–14, 130, 136, 160, 162–163, 166, 171, 176–177, 182, 191, 193,
199–200, 206, 210, and 226**. Slot 226 overlaps shutdown; excluding it leaves
30 ordinary missing slots through slot 225. Slot 0 is genesis, not a failed
proposal.

## Startup: every missing slot

The full per-slot table is in
[Round 2 startup causal analysis](round2-startup-causal-analysis.md), with
archive-member line anchors in [proposer reconstruction](round2-startup-proposers.md).
The direct terminal split is:

| Slots | Failure boundary |
| --- | --- |
| 1 | RANDAO domain lookup deadline; exact VC/RPC wait split not logged. |
| 2, 3, 4, 7, 11, 12 | Synchronous sync-committee preflight expires before proposer dispatch; the proposer then encounters an expired context. Slot 4 stalls in selection signing/domain lookup; the others in sync-index lookup. |
| 5, 8 | Block building begins at +10.119/+11.190 seconds with a reduced remaining budget; payload retrieval fails and no matching P2P bid is cached. |
| 6, 9 | Execution HTTP response timeouts despite successful proxy responses in 2/22 ms and sub-millisecond empty-payload builds. No matching P2P bid is cached. |
| 10, 14 | Payload selection succeeds, but the joined consensus-field/packing branch is still outstanding at the slot deadline. |
| 13 | Parent-state/slot processing expires before parallel assembly; this is not packing. |

The code's reduction in startup work begins with new round-1 votes at slot 8:
the same genesis root yields a checkpoint state processed to slot 8, eligible
for the cached validator count. Old round-0 votes remain gossip-age-valid much
longer, so this is a change in the traffic/work mixture, not a hard cutoff.
Detailed observers' vote latency improves through slots 8–12 before the first
block arrives. Node32 then completes all proposal stages within slot 15.

## Later failure: how one missing envelope becomes a network-wide rollback

### Node62's local divergence

Node62 imports the slot-117 consensus block at **01:53:26.799**, but its
execution envelope is locally unavailable. Its VC explicitly votes
`payloadPresent=false`. No corresponding `engine_newPayload` request reaches
its EL; the next EL poll returns promptly with the preceding slot-116 payload.
Thus the initiating boundary is before Engine API processing, not an EL
`SYNCING`/`INVALID` rejection of slot 117.

The exact Gloas validation path defers a child that needs an unseen parent
payload and requests that envelope. Node62 does not import the ordinary next
blocks, rolls back to its old slot-110 fork, and later imports slot 123 built
on that fork. The historical logs do not show whether the missing envelope was
never received, rejected before blockchain processing, or unsuccessfully
recovered from peers. [Node62 evidence](round2-node62-divergence-audit.md).

### The committee and fork-choice amplification

The mock available committee selects **512 contiguous validator indices**
from a deterministic offset, not a shuffled cross-section of the network.
At vote slot 128, its membership splits exactly as follows:

| Wallet | Committee indices | Seats | Voted root |
| --- | --- | ---: | --- |
| Node61 | 89276–89395 | 120 | Slot 128, ordinary branch |
| Node62 | 89396–89787 | 392 | Slot 123, old fork |

Each VC caches one available-attestation-data response per slot, so all its
local committee members sign that same BN view. Detailed observers 1, 201,
and 400 independently record the identical 392/120 validation-accepted split.
The acceptance log precedes the subscriber's fork-choice insertion, so these
records are not a serialized store snapshot. The messages were accepted
roughly eight seconds or more before the next slot tick; normal subscriber
processing inserts them next, and the observed rollback plus the focused
production-code test corroborate the following score mechanism.

The roots' ancestry—not just their slot numbers—is decisive:

```text
slot 109
├── slot 110 ── slot 123                       392 votes
└── slot 111 ── ... ── slot 117 ── ... ── 128   120 votes
                       justified start
```

Reorg common-ancestor logs establish the split at slot 109. Slot123's owner158
also logs building from head slot110. Thus the old-fork votes do not support
any descendant of justified slot117.

At the slot-129 boundary, Goldfish reads vote slot128. With the recorded split
inserted, its threshold is **512 / 2 = 256**, requiring a score strictly
greater than it. All 392
old-fork votes count in that denominator, but none can credit a child below
the justified start at117. The reachable ordinary branch has only120 votes.
It fails the gate, so the walk stops at117. Slot128's round-start proposal
privilege no longer applies in slot129.

The complete census records this same depth-11 rollback at **998 nodes** in
the first **857 ms** of slot129; the other two nodes (62 and 110) already had
divergent heads. This is a concrete causal explanation for that rollback, independent
of the unlogged `payloadPresent` split. The separate checkpoint-viability
filter is not needed to establish failure: even an eligible child would have
insufficient score. A focused test using the production fork-choice path
reproduces the rollback with 392 old-fork plus120 current-branch votes; without
the old-fork denominator contribution, the same120 votes retain head128.
[Rollback and test](round2-rollback-cause.md),
[ancestry/viability audit](round2-rollback-viability-audit.md).

## Later slots without an imported block in the archive union

The complete **1,000-node** archive union finds the later holes below. The
four rows without proposer records remain empty even after recovering the
last detailed archives; they are no longer missing-owner-archive cases. This
table counts block absence, not a canonical chain after divergence. The final
coverage and recovery report is [here](round2-remaining-archive-audit.md).

| Slot | Node(s) | Direct failure |
| ---: | --- | --- |
| 130 | 112 | Building finishes at +11.389; proposal RPC expires at +12.003. |
| 136 | 80 | Payload selected; joined packing/build work outlives the request deadline, returns canceled around +17.892. |
| 160 | 145; competing duty31 | VC signs as validator37824; BN builds/checks for validator18020. Invalid RANDAO. |
| 162 | No local assignment logged | No logged retained proposer assignment or proposal attempt across all 1,000 clients; likely epoch-5 duty-view gap. |
| 163 | 158 | Payload selected; joined packing/build work returns canceled around +16.847. |
| 166 | 62, 147, 135 | VCs62/147 sign with different keys from BN-selected validator100155; both fail RANDAO. Owner135's request expires. |
| 171 | No local assignment logged | Same epoch-5 duty-view gap; no proposal attempt logged. |
| 176 | 117 | Payload selected; joined packing/build work returns canceled around +17.496. |
| 177 | No local assignment logged | Same epoch-5 duty-view gap; no proposal attempt logged. |
| 182 | No local assignment logged | Same epoch-5 duty-view gap; no proposal attempt logged. |
| 191 | 135 | Payload selected; joined packing/build work returns canceled around +19.102. |
| 193 | 80 | Payload selected at +5.990; request expires at +12.008, but packing cancellation is not logged until +81.348. |
| 199 | 44 | Payload selected; joined packing/build work returns canceled around +23.909. |
| 200 | 97 | Building finishes at +10.857; proposal RPC expires at +12.003. |
| 206 | 29 | Request deadline and HTTP/2 cancellation; joined packing/build work returns canceled around +17.879. |
| 210 | 18 | VC signs as validator10603; BN builds/checks for validator66342. Invalid RANDAO. |
| 226 | 15 | Shutdown cancellation at +0.405 during parent-state preparation, not an ordinary live-slot deadline. |

For the invalid signatures, the archive's activation records independently map
the verification keys to the different BN-selected indices. `BlockRequest`
carries the VC's reveal but not an expected proposer key/index; the BN selects
the proposer again from its current parent state. It therefore constructs a
block with validatorB's index but validatorA's reveal when the duty and serving
state disagree. These are schedule/state mismatches, not a diagnosis of broken
keystores. [RANDAO audit](round2-randao-divergence-audit.md).

The four silent slots all fall in epoch5. All 120,000 unique activated
validator indices are present in the complete archives, so missing configured
keys do not explain these holes. A raw schedule audit also handles
proposer-only log rows that omit `slot` and instead print `timeUntilDuty`;
none supplies an assignment for these four slots.

Post-Fulu proposer assignments come from the head state's `ProposerLookahead`.
With the configured seed lookahead, epoch5 depends on epoch3's RANDAO mix—the
history on which the branches diverged. Epoch4 uses the earlier epoch2 mix,
consistent with the delayed onset of obvious proposer disagreement at slot160.
The VC additionally fetches attester and proposer duties independently and
drops proposer data when the dependent roots mismatch. The archives show this
root-mismatch path repeatedly active after rollback: 377 of 378 warnings occur
after 01:55 UTC. If no local key is retained as proposer in the snapshot used
by `RolesAt`, it dispatches no proposer and produces no RANDAO/block-request
failure. This is a plausible assignment-level explanation for the observed
silence, not a dump of the execution-time snapshot: `RetryMissingNextDuties`
can merge duty-store changes without calling `logDuties`. The logs do not
identify one unique discarded RPC response or prove the precise dispatch
boundary for each hole. [Duty-view source audit](round2-duty-hole-source-audit.md).

There is independent evidence of the BN/VC discrepancy for two of these
slots: BN106 prepared payload attributes for `nextSlot=162`, and BN118 for
`nextSlot=177`, both from head slot117. That path requires a locally attached
proposer, yet no VC duty snapshot records a proposer for either slot. No
equivalent BN entry was found for slots171/182; its absence is inconclusive.
[Complete key and duty-log audit](round2-empty-proposer-slots.md).

For the deadlines, successful payload selection rules out payload retrieval as
the blocking branch in the listed packing cases. But neither a late packing
cancellation nor `aggregatedCount=14924` establishes a 14,924-object pool or a
quadratic runtime: that field counts participants in one aggregate. The logs
lack candidate counts and runtime stacks. Filtering, invalid deletion, pool
locks, deduplication, and scheduling remain unseparated inside this region.
[Owner timelines](round2-later-proposer-failures.md),
[packing source audit](round2-later-packing-audit.md).

## Interpretation and verification

- “Block produced,” “imported by a sampled node,” and “canonical” are distinct.
  Slot19 demonstrates why: the VC's submit RPC times out, but 999 nodes import
  its block, first at +12.3456 seconds relative to slot19. Later many blocks
  exist only in a few observers, so a sparse detailed
  log alone greatly overstates the number of unproduced slots.
- Startup load causation is established by the controlled 3-node/120k-registry
  experiment; applying its exact runtime mediation to each historical owner
  remains an inference. The later rollback has its own observed vote/ancestry
  inputs and production-code reproduction.
- The initiating delivery/recovery reason for node62's missing slot117
  envelope and the internal time split of long packing calls are not recorded.
  Those boundaries should not be disguised as proven bandwidth loss or proven
  quadratic packing.
- No production fix is applied. Reports, parsers, and focused tests are in
  diagnostic jj change `vpprxpww`; diagnostic code is GPT-5.6 Sol authored.
  Verification uses Go, not Bazel. Both focused tests passed five consecutive
  runs: `TestSameRootRoundCheckpointStateDiagnostic` (with `-tags develop`)
  and `TestGoldfishWalk_Round2Slot129Rollback`. The complete archive parser
  also passed the full 1,000-node scan and focused proposer-slot inference
  checks.
