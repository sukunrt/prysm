# Detailed task list: Goldfish, timing knobs, kurtosis + ethshadow

Companion to `plan-next.md`. That file holds the reasoning; this one holds
the work. Line numbers are from 2026-08-19 and will drift — treat them as
pointers, not addresses; grep for the symbol.

Design record: `task.md` decisions 12, 13, 14, 17. Spec:
`../decoupled-consensus-networking/consensus-specs/specs/_features/simplex/fork-choice.md`.

## Rules

- **Never abandon or rewrite an existing jj change.** New changes on top of
  the current tip.
- One jj change per numbered group where the tree still compiles at each
  boundary; merge groups rather than land a broken intermediate (the step-1
  lesson). Sign with a single `Assisted-By: <model>` trailer. No
  `Co-Authored-By:`.
- `go vet` per touched package. No blanket `go modernize`. Lines under 100
  characters. Never delete data (run outputs, logs, `data*` dirs).
- Full command outputs to scratchpad log files, never through `tail`/`head`.
- No bazel spectests. Accept commands per step: `go build ./...`, targeted
  `go test`, `bazelisk build //...`.
- The verification ladder applies to every step: unit tests → ~3-slot
  single-node smoke → `TestEndToEnd_HezeGenesisShort` → full
  `TestEndToEnd_HezeGenesis` → sims. Iterate at the cheapest failing tier.

## The one thing not to get wrong

**Land and unit-test the current-slot passthrough before disabling proposer
boost.** A fresh block scores 0 by construction; without the passthrough the
walk never advances past the parent and the chain silently stops extending.
Right behind it: **never reuse `f.votes`** — it is epoch-granular and never
expires (`forkchoice.go:112`); Goldfish votes expire after one slot.
Separate store, separate walk. 14 non-test sites read `f.votes`; the new
walk reads none.

## Activation gate (applies to all of step 4)

Every behaviour change in step 4 is gated on the Heze epoch being reached
(`slots.ToEpoch(current) >= params.BeaconConfig().HezeForkEpoch`, or the
store-level equivalent computed once at insert time). Shipped mainnet and
minimal configs keep `HezeForkEpoch = MaxUint64`, so the entire existing
forkchoice/blockchain suite runs the old path with **no expectation edits**
— the step-2 identity trick, applied to behaviour. Only e2e/devnet configs
(Heze at 0) exercise the new walk.

---

# Step 4 — Goldfish head vote

## 4.1 The vote store

New file(s) in `beacon-chain/forkchoice/doubly-linked-tree/` (keep it next
to the walk; the sync package only feeds it through the blockchain service
interface, mirroring how attestations flow today).

- [ ] Store: `slot → validatorIndex → (blockRoot [32]byte, payloadPresent
      bool)` plus `slot → equivocators set`. A second vote from the same
      validator for the same slot with different content moves the
      validator to the equivocation set and removes its scored vote (spec:
      an equivocator counts for viability but for no child).
- [ ] Prune at ~3 slots behind current. Sparse maps (task.md decision:
      `targets` and `timeouts` are sparse).
- [ ] Concurrency: written from gossip goroutines, read by the walk under
      the store lock. Follow the locking pattern of the existing
      `f.votes` batch (`ProcessAttestation` takes `f.votesLock` — check the
      actual lock names and mirror them).
- [ ] **Feed it**: replace the no-op body of `availableAttestationSubscriber`
      (`beacon-chain/sync/subscriber_beacon_attestation.go:66`).
      `validateAvailableAttestation` already sets `msg.ValidatorData = att`
      (`validate_beacon_attestation.go:621`) and enforces exactly one
      signer. Recover the signer index with
      `decoupled.AvailableAttestationSeatsToValidatorIndices`
      (`decoupled/available_attestation_committee.go:41`) — or better, have
      the validator stash the resolved index in `ValidatorData` alongside
      the attestation so the subscriber does not recompute it. Seat count =
      number of set bits; store it with the vote (the walk weights by it).
- [ ] **Both directions (lesson 1):** the send path already works; this is
      the receive side. There is no persistence — the store is in-memory by
      design. Confirm nothing tries to snapshot it into the DB.
- [ ] Unit tests: insert, overwrite-same-content (idempotent), equivocate,
      prune, seat multiplicity, concurrent insert during read.

## 4.2 The walk

In `beacon-chain/forkchoice/doubly-linked-tree/`, beside `store.head`
(`store.go:22`).

- [ ] Bottom-up scoring: for each stored vote of `previous_slot`, walk
      parents from the voted node to the justified root, adding the seat
      count to each node on the path. One pass, 512 × depth. Mirror the
      shape of `applyWeightChangesConsensusNode` (`gloas.go:75`).
- [ ] Denominator: 512 minus nothing — equivocators stay in the
      denominator; their seats score no child.
- [ ] Top-down gated descent from the justified root: a child is taken when
      its score clears the majority gate of the previous slot's counted
      seats, else the walk stops at the parent.
- [ ] The passthrough: a child at the *current* slot is viable regardless
      of score, **except at a round-start slot**
      (`slots.IsRoundStart`, `time/slots/slottime.go:175`). Spec:
      `is_available_attestation_viable`.
- [ ] Payload-status derivation — **no stub (user decision 2026-08-20,
      overriding decision 13)**: implement the spec's
      `get_available_vote_payload_status` — same-slot vote → PENDING;
      older block → FULL/EMPTY by the vote's `payload_present` bit. It is
      byte-for-byte the Gloas mainnet `get_supported_node` rule. The vote
      store already carries the bit (4.1) and `NodeV2.PayloadStatus`
      exists; the derivation is a few lines in the walk. Unit test both
      arms plus the same-slot case.
- [ ] Stable root: use the justified root. No TSQ, no height filter.
- [ ] The walk assigns `s.headNode` exactly where `store.go:61` does today,
      so `FullHead` and `CachedHeadRoot` keep working.
- [ ] Empty-store behaviour (cold start): walk stops at the justified root;
      passthrough still admits a current-slot child; no panic, no zero-hash
      head. Unit test all three: fresh start, restart (populated tree,
      empty votes), post-checkpoint-sync (justified != genesis).
- [ ] Unit tests with hand-built trees: gate passes / blocks; head retreats
      when a late block loses the passthrough; round-start slot refuses the
      passthrough; equivocator excluded from scores but in denominator;
      seat multiplicity changes the winner; two competing children.

## 4.3 IsCanonical and the refresh triggers

- [ ] `IsCanonical` (`forkchoice.go:184`) compares `bestDescendant`
      pointers, which the gated walk no longer maintains. Under the Heze
      gate, reimplement: stamp a generation counter on nodes during the
      descent, or walk parents from `s.headNode`. Check every caller
      (grep `IsCanonical(`) for hot-path sensitivity before choosing.
- [ ] `CanonicalNodeAtSlot` (`gloas.go:24`) walks parents — no change, but
      add a test at the new walk to prove it (lesson 2: verify, not assume).
- [ ] Refresh: the head is a function of the clock. Confirm the two
      existing triggers fire under Heze: slot boundary (`NewSlot` +
      `UpdateHead` at t=0) and block insert (`process_block.go` calls
      `Head`). Do NOT add a per-vote recompute; count post-drain late votes
      instead (4.5).
- [ ] `updateBestDescendantPayloadNode` (`gloas.go`, empty/full selection):
      confirm the full/empty choice still resolves under the new walk —
      block building at slot N must still descend to N-1 whatever its
      weight (decision 12's observation). Add the regression test.

## 4.4 Turn off the old head inputs (all under the Heze gate)

- [ ] Proposer boost: skip the boost application in `applyWeightChanges`
      (`store.go:188-218`) and the boost assignment on insert. The
      passthrough (4.2) must already be merged.
- [ ] Late-block reorg: two early returns in `ShouldOverrideFCU` and
      `GetProposerHead`
      (`forkchoice/doubly-linked-tree/reorg_late_blocks.go:34,98`).
- [ ] FFG→LMD input: nothing to delete — the new walk never reads
      `f.votes`. `ProcessAttestation` (`forkchoice.go:86`) keeps running
      (it also carries the payload vote at `:116`'s caller,
      `blockchain/process_attestation.go:103-116`, which
      `choosePayloadContent` still consumes — **deliberately kept**).
- [ ] `AttestationDueBPS` stays 3333. Windows still read it:
      `arrivedEarly` (`node.go:40`) and the boost window. Do not zero it.
- [ ] Proposer behaviour: the proposer builds on the walk head via the
      existing `HeadRootAndFull` path — verify `getParentBlockHash`
      (`proposer_execution_payload.go`) agrees with the gated head in a
      unit test (the 5.0a lesson: this path finds bugs three layers away).

## 4.5 Metrics

- [ ] Per-slot gauge/histogram: fraction of 512 seats whose votes arrived
      before the next slot start.
- [ ] Counter: walk stopped at the gate.
- [ ] Counter: late votes (for slot N-1, arriving after the t=0 drain).
- [ ] Counter: gate-caused head retreats (`goldfish_gate_retreat`) —
      incremented by the walk when the new head is an ancestor of the
      previous head. `saveHead`'s `reorgCount` (`blockchain/head.go:151`)
      is left untouched. **Decided by user 2026-08-20: keep both. The sims
      must include late publishers so this counter carries signal — see the
      late-publisher knob in plan-next.md step 6.**

## 4.6 Tests that will wake up (lesson 4)

Enumerate before running; all are gated off by the activation gate, so the
expected count of expectation edits is **zero**. Verify by running:

- [ ] `beacon-chain/forkchoice/doubly-linked-tree/...` — full package.
- [ ] `beacon-chain/blockchain/...` — head/reorg tests; the two known
      pre-existing failures (`TestStore_NoViableHead_NewPayload`,
      `TestNoViableHead_Reboot`) stay the only ones.
- [ ] `beacon-chain/sync/...` — the subscriber is no longer a no-op; any
      test asserting the no-op behaviour moves.
- [ ] New files need BUILD entries; run gazelle on the touched packages
      (lesson 6).

## 4.7 Deliberately unchanged

| site | why it is safe |
|---|---|
| `f.votes` and its 14 readers | pre-Heze path only; new walk reads none |
| `ProcessAttestation` (`forkchoice.go:86`) | still decodes the payload vote for `choosePayloadContent`; FFG weight it writes is only read by the old walk |
| `validateAvailableAttestation` | verdicts already real; receive-path checks are done |
| `decoupled/` committee | mock stays slot-keyed; spec's `cache_available_committee` exists for head-dependent committees, ours is not (stops being safe if that changes) |
| the attestation pool / operations service | FFG attestations still aggregate into blocks; justification needs them |
| `CanonicalNodeAtSlot`, `FullHead`, `CachedHeadRoot` | parent walks / headNode assignment survive (tested in 4.3) |
| `MarkFullNode`, genesis full-node insert (`store.insert`) | 5.0a work; the walk consumes it unchanged |

## 4.8 Verify

- [ ] Unit ladder green (4.1-4.4 tests).
- [ ] 3-slot single-node smoke (reuse the `step5b-smoke.sh` pattern):
      blocks extend past slot 1 with boost off — proves the passthrough.
- [ ] `TestEndToEnd_HezeGenesisShort` green.
- [ ] Full `TestEndToEnd_HezeGenesis` green: finalization still happens
      (FFG justification consumes block attestations, untouched), available
      attestations flow, and the new metrics are nonzero in the logs.
- [ ] `go build ./...`, `go vet`, `bazelisk build //...` (full logs).

## 4.9 Executed 2026-08-20 <added by executor>

Seven jj changes on top of `rnslyvxp`, oldest first:

| change | what |
|---|---|
| `rqlvotsr` | the vote store, the receive path, the vote metrics |
| `pmlpymvn` | the walk, the passthrough, `IsCanonical`, gate metrics |
| `oztyywyl` | proposer boost, late-block reorg off under the gate |
| `syylkqsn` | geth genesis: Amsterdam blob schedule entry (e2e blocker) |
| `nxzstrpp` | `TestEndToEnd_HezeGenesisShort` recreated |
| `ktzlulyk` | local votes fed through the RPC; the vote's due time |
| `rssoytzn` | one electorate for the mock committee |

### What the plan did not say, and the smoke found

1. **A node never saw its own votes.** `subscribeWithBase` skips every message
   whose `ReceivedFrom` is our own peer ID, so the subscriber - the plan's
   feeding point - is dead for locally published available attestations. On the
   single-node smoke that is the whole electorate; on a two-node devnet it is
   half. The local vote now goes to forkchoice from
   `ProposeAvailableAttestation`, right after the broadcast. Symptom before the
   fix: `goldfish_seat_fraction` pinned at 0 and every block after the first
   built on genesis.
2. **`AVAILABLE_ATTESTATION_DUE_BPS_HEZE` had no value anywhere**, so it was 0
   and every validator published its head vote at the start of the slot, before
   that slot's block existed. Every vote named the previous block and no block
   could ever clear the gate. It now defaults to 2500 like the spec.
3. **The publisher and the receiver disagreed about the electorate.** The
   validator client resolved seats against
   `MIN_GENESIS_ACTIVE_VALIDATOR_COUNT`, the beacon node against the live
   registry size. They agree until the first deposit - which the e2e sends on
   purpose - and then every seat names the wrong validator. Both sides now call
   `decoupled.CommitteeValidatorCount`, and an index outside it gets no seats.
   This is the one expectation edit in the step:
   `TestService_validateAvailableAttestation` built its seat bits from the test
   state's validator count.
4. **The proposer boost root is still recorded on insert**, against 4.4's
   letter. Only the boost *weight* is skipped. `shouldExtendPayload` reads the
   boost root, and the walk's payload tiebreaker reads `shouldExtendPayload`;
   without the record every previous-slot payload decision resolves to FULL,
   which strands a block that built on the empty node and freezes the head at
   its parent. `TestGoldfish_PreviousSlotPayloadDecisionFollowsTheNewBlock`
   fails exactly that way if the record is removed.
5. **Equivocators score every child**, per the spec's
   `get_available_attestation_score` (`score += 1` for each equivocating seat,
   for every child), not "no child" as 4.1's prose says. The spec's own comment
   is "Equivocator counted for viability", which is what 4.1 was paraphrasing.

### Round-start proposals are orphaned, and that is phase 2 without phase 1

`is_available_attestation_viable` refuses a current-slot proposal at a
round-start slot because the spec's phase 2 admits it through the *stable root
proposal* instead - the TSQ-distinguished proposal, which does not exist while
the stable root is stubbed as the justified root. So the round-start block
never becomes head, its own slot's voters name its parent, and the next slot's
proposer builds on that parent. One block per round is lost: with
`SLOTS_PER_ROUND = 8` that is 12.5% of blocks: in the four-and-a-bit epoch
smoke every one of slots 8, 16, 24 ... 128 was orphaned and every other slot
was canonical. Pinned by
`TestGoldfish_RoundStartProposalIsOrphaned`. If this cost is not acceptable
before the gadget lands, the smallest spec-shaped stub is to treat the unique
round-start block descending from the stable root as the distinguished
proposal. **That stub was built in step 5's wave (user decision 2026-08-20);
see 5.7. The pinning test is now
`TestGoldfish_RoundStartProposalAdmitted` and its three siblings.**

### The e2e cannot judge this step in this checkout

Two pre-existing infrastructure failures, both in the execution layer:

- geth refuses the e2e genesis because `AmsterdamTime` is set with no
  `blobSchedule.amsterdam` entry. Fixed here (`syylkqsn`) because nothing else
  could run.
- With that fixed the run starts, but the vendored geth 1.17.4 answers
  `engine_getPayloadV6` with a structured `blockAccessList`, while Prysm's
  `ExecutionPayloadGloasJSON` expects `hexutil.Bytes`. Every block build fails
  with "cannot unmarshal non-string into ... blockAccessList", so the e2e chain
  produces no blocks at all. `TestEndToEnd_HezeGenesisShort` still passes -
  its early evaluators do not need blocks - which is worth knowing about that
  tier. The locally installed geth 1.17.6-unstable does not have the problem,
  which is why the single-node smoke works. Fixing it means either bumping the
  vendored geth or teaching the Gloas payload JSON the structured BAL; neither
  belongs in this step.

The evidence for this step therefore comes from the unit ladder and from
single-node smokes on geth 1.17.6 (`step4-smoke.sh`, `step4-long.sh` and
`step4-fin.sh` in the session scratchpad), not from the full e2e.

### What the long smoke shows

256 validators, one node, `SLOTS_PER_ROUND = 8`, Heze genesis, 12-second slots,
five epochs:

- **It finalizes twice.** `finalizedEpoch=2` at slot 129, and by the last slot
  of the run (160) the head reports `justifiedEpoch=4`, `finalizedEpoch=3`.
  FFG justification still consumes block attestations with the LMD input off,
  which is what the full e2e was meant to prove.
- **Blocks at every slot but the round starts.** Slots 1-160 all produced a
  block; the only orphans are slots 8, 16, 24 ... 152.
- `goldfish_seat_fraction` 1.0 every slot (all 512 seats heard from before the
  next slot started), `goldfish_late_vote_total` 0,
  `goldfish_gate_stop_total` 77, `goldfish_gate_retreat` 0,
  `goldfish_equivocation_total` 0, and `beacon_reorgs_total` 0 - the head never
  moved backwards, the round-start blocks simply never became head.

A run with late block publishers (step 6's knob) is what will move
`goldfish_gate_retreat` off zero.

---

# Step 5 — timing knobs

## 5.1 Slot-start FFG vote

- [ ] Validator-client flag (name it with the `decoupled-` prefix like the
      misbehaviour enum will use): skip
      `waitUntilAttestationDueOrValidBlock`
      (`validator/client/attest.go:39,281`), add jitter (a bounded random
      delay; pick the bound from the sim's needs, default ~200ms).
- [ ] The flag threads through the e2e components so a run can flip it
      (`testing/endtoend/components/validator.go` — check how existing
      flags are passed).
- [ ] Do NOT change `AttestationDueBPS` (3333 stays; see 4.4).

## 5.2 The target shift

Ungated (genesis is Heze; there are no Heze spectests; the shift is sound —
plan-next.md step 5).

- [ ] `helpers.BlockRoot(state, epoch)` semantics change to
      `StartSlot(epoch) - 1`. Prefer changing the helper itself over the
      four callers if all four want the shift — they do (all compute the
      FFG target/matching): `core/altair/attestation.go:363`,
      `core/epoch/precompute/attestation.go:121`,
      `core/epoch/precompute/justification_finalization.go:164,172`.
      Decide helper-vs-callers after reading `helpers.BlockRoot`'s other
      test uses; record the choice.
- [ ] **Epoch 0 underflow → return the anchor (genesis) root.** On the
      critical path of slot 0 of every run. Unit test it first.
- [ ] Forkchoice twin (lesson: symmetric pairs — state-side and
      forkchoice-side must agree): the `node.target` assignment on insert
      (`store.go`, insert path) and `targetRootForEpoch`
      (`forkchoice.go:848`) shift the same way, including the epoch-0 arm.
      If they disagree, `VerifyLmdFfgConsistency` rejects every vote — the
      failure is loud but two layers from the cause.
- [ ] Validator-client side: the attester gets its target from the node
      (`GetAttestationData`), so no client change — verify with a test.
- [ ] Sweep test loops: `core/altair`, `core/epoch/precompute`,
      forkchoice target tests all pin the old target. This step DOES edit
      expectations, unlike step 4 — enumerate them in the executor note.

## 5.3 Accounting fallout (accepted, documented)

- [ ] `is_matching_head` misses every slot under the flag; at 14/64 of the
      reward. `is_matching_target` misses at pre-shift epoch starts. Wire
      behaviour unchanged. No code change; note in the run docs.
- [ ] e2e: participation/reward evaluators must relax **only when the flag
      is on**. Add a flagged variant or a `withoutEvaluators` twin, do not
      weaken the default run.

## 5.4 The knobs as measurement parameters

- [ ] `AVAILABLE_ATTESTATION_DUE_BPS_HEZE` (`config/params/config.go:103`)
      is already yaml-sweepable — no code. Add it to the sim config
      template's documented knobs.
- [ ] The names-which-block knob (FFG vote): a validator-client option:
      `head-at-vote-time` (today's behaviour) vs `head-at-round-start`
      (fields freeze at round start, the 2026-08-14 decision 1 semantics).
      Implement as: at round start, cache the head answer the node returned;
      under the knob, reuse it for the round's remaining FFG votes.
      Recommendation is round-start default only when the gadget lands;
      until then default stays head-at-vote-time (open question 2).
- [ ] Unit tests for the freeze cache: crosses a round boundary, resets.

## 5.5 Deliberately unchanged

| site | why |
|---|---|
| `AttestationDueBPS = 3333` | read as a window by `arrivedEarly` and the boost window; zeroing it makes every block "late" |
| `PayloadAttestationDueBPS`, PTC paths | Gloas machinery, orthogonal to both knobs |
| the available attestation's wait-for-block behaviour (`SubmitAvailableAttestation`) | its due time is already the config knob; no flag needed |
| slashing / EIP-3076 gate from 5.0b | timing changes neither targets nor sources |

## 5.6 Verify

- [ ] Unit ladder, then `TestEndToEnd_HezeGenesisShort` with the flag off
      (regression) and on (new variant).
- [ ] Full e2e with flag on: still finalizes (the 87.5%→96.9% weight
      argument in decision 12 says it must; the run proves it).
- [ ] `go build ./...`, `go vet`, `bazelisk build //...`.

## 5.7 Executed 2026-08-20 <added by executor>

Three jj changes on top of `yssuxmnt`, oldest first:

| change | what |
|---|---|
| `kztuytom` | the FFG target shift, state side and forkchoice side |
| `pmpxlyos` | the flagged e2e variant with participation relaxed there only |
| `vlzvtvzq` | the round-start proposal stub in the walk |

### Helper versus callers (5.2)

The helper, renamed. `helpers.BlockRoot` had exactly five callers, all of them
computing the FFG target: the four in the plan plus `testing/util/attestation.go`,
which builds the attestations the tests feed back into the same matching code.
It had no other use and no test of its own. Changing its body and keeping the
name would have left a function documented as the spec's `get_block_root`
returning something else, so it is now `helpers.FFGTargetRoot`, with the five
call sites renamed and the deviation written where the implementation is.

### Test expectations edited

Every one, and why:

- `TestStore_TargetRootForEpoch` (forkchoice): six values. The slot-32 and
  slot-33 blocks now target the genesis block rather than the slot-32 block;
  after finalizing the slot-33 block the pruned tree root is its own target and
  the dependent root falls back to `finalizedDependentRoot`; the slot-96 and
  slot-97 blocks target the slot-33 block rather than the slot-96 one; the
  dependent root for epoch 1 from the slot-129 block is the slot-32 block
  rather than zero.
- `TestProcessJustificationAndFinalizationPreCompute_*` (precompute): three
  tests, justified root `byte(64)` becomes `byte(63)`.
- `TestAttestedCurrentEpoch` (precompute): the marked block root moves from
  index `SlotsPerEpoch` to `SlotsPerEpoch-1`.
- `TestStore_OnBlockBatch` (blockchain): the justified root is `blks[62]`, the
  block at slot 63, not `blks[63]`.
- `TestGoldfishWalk_NoPassthroughAtRoundStart` (forkchoice): unchanged
  expectation, but the round-start block now has to arrive in the previous
  round to stay outside the proposal mechanism, so the test isolates
  `is_available_attestation_viable` as it did before.
- `TestGoldfish_RoundStartProposalIsOrphaned` (forkchoice): replaced by
  `TestGoldfish_RoundStartProposalAdmitted`, since the orphaning it pinned is
  the thing the stub removes.

`core/altair` needed none: its attestation tests build their attestations
through `testing/util`, which shifted with the helper.

### What the plan did not say

1. **Pruning had to move with the target.** `prune` treats a child of the
   finalized node at or before `EpochStart(finalizedEpoch)` as incompatible
   with the checkpoint. That was right while the checkpoint block sat *at* the
   epoch's first slot; with the shift the checkpoint block sits one slot
   earlier and its child at the first slot is the finalized chain itself. Left
   alone, finalization deleted the canonical chain and `postBlockProcess` then
   failed with "no node found for root". The bound is now strict and the early
   return moved with it. `TestStore_PruneKeepsTheEpochStartChild` pins it.
2. **The pruned tree root is its own target**, rather than `nil`. A nil target
   makes `targetRootForEpoch` answer the zero root for the anchor and for every
   later block of its epoch, and the epoch-0 arm already says the answer for an
   unreachable target is the anchor.
3. **`IsViableForCheckpoint` moved with the same boundary**: a child at exactly
   the epoch's first slot now makes its parent a viable checkpoint, because
   that child's target for the epoch is the parent.

### The round-start proposal stub (task 2)

`on_block` records the round-start blocks that arrive during their own round,
as `update_round_proposals` does, and `get_head` phase 2 starts the walk at the
round's single such block when it descends from the stable root (stubbed as the
justified root) and is viable for the head. The disambiguation rule is the
spec's: two distinct round-start blocks in a round distinguish neither, and the
round-start slot is orphaned as it was before the stub.

Two spec checks cannot be made here and are deliberately absent: the freeze at
the round's available-vote action, and the "descends from the live
available-confirmed head" test, both of which need the finality gadget. Without
the freeze the decision is re-evaluated on every walk, so a second proposal
arriving later in the slot withdraws the first one's distinction instead of
being ignored. That is the conservative direction.

### The smoke

Same shape as step 4's `step4-fin.sh` (256 validators, one node, geth 1.17.6,
`SLOTS_PER_ROUND = 8`, Heze genesis, 12-second slots), rerun as
`w2a-smoke.sh`; ~4.8 epochs.

- **No orphans at all.** Slots 1-153 each produced a block and every block's
  parent is the previous slot's block, the nineteen round starts (8, 16, ...,
  152) included. Step 4's equivalent run orphaned every one of them.
- `goldfish_gate_stop_total` 0, against 77 in the step-4 run: those stops were
  the walk refusing the round-start proposals.
- `goldfish_round_proposal_total` 38 (the walk runs more than once per slot),
  `goldfish_round_proposal_conflict_total` 0, `goldfish_gate_retreat` 0,
  `beacon_reorgs_total` 0, `goldfish_seat_fraction` 1.
- **It still finalizes with the shifted target**: `beacon_finalized_epoch` 2 and
  `beacon_current_justified_epoch` 3 at the end of the run. Justification is
  the end-to-end proof that the two sides of the shift agree - a state-side
  target that disagreed with forkchoice would have had
  `VerifyLmdFfgConsistency` reject every attestation.

Logs: `w2a-smoke-bn.log`, `w2a-smoke-vc.log`, `w2a-smoke-metrics.txt`,
`w2a-chain-check.py` in the session scratchpad.

### Not verified here

The e2e still cannot run in this checkout (the vendored geth payload JSON,
4.9). `TestEndToEnd_HezeGenesisSlotStartFFG` is therefore written but unrun,
and the flagged participation floor is untested at runtime. The rpc packages
`rpc/eth/beacon` and `rpc/prysm/v1alpha1/beacon` have 32 failures that this
work did not cause: the same 32 fail on `yssuxmnt`.

---

# Step 6 — kurtosis and ethshadow

## 6.0 Facts (checked 2026-08-19; re-verify, they age)

- kurtosis CLI 1.18.1 at `/usr/bin/kurtosis`; docker 29.6.2;
  `~/dev/ethereum-package` (ethpandaops) with `participants[].cl_image`,
  `cl_extra_params` (`network_params.yaml:20,25`); `~/dev/kurtosis` is an
  empty dir (ignore it).
- Prysm image tooling is push-oriented (`tools/prysm_image.bzl`;
  `prysm_image_upload`, `cmd/beacon-chain/BUILD.bazel:74`). Recommended
  local route: plain Dockerfile over `go build` binaries (open question 4).
- The ethshadow workspace is `~/dev/decoupled-shadow-sim`; the baseline
  run's config (SLOTS_PER_ROUND etc.) is already there from plan step 5.3.

## 6.1 Prysm images

- [ ] Dockerfile(s) building beacon-chain, validator (and prysmctl if the
      package's genesis flow needs it) from the repo's `go build` output.
      Keep them in a new `kurtosis/` directory in this repo, with the
      `network_params.yaml`.
- [ ] Build and `docker load`; tag with the jj change id for traceability.

## 6.2 Genesis/config injection

- [ ] Patch the ethereum-genesis-generator: a new Dockerfile mirroring
      `decoupled-shadow-sim/Dockerfile.genesis-gen` — inject
      `SLOTS_PER_ROUND: 8`, `GLOAS_FORK_EPOCH: 0`, `HEZE_FORK_EPOCH: 0`,
      the single-entry `BLOB_SCHEDULE`, Amsterdam at the EL genesis time,
      and strip any heze→EL fork mapping (Heze is CL-only). Read the
      existing Dockerfile.genesis-gen first — it solved the same problems.
- [ ] Point `~/dev/ethereum-package` at it via the generator-image
      parameter (find the exact key in the package's `network_params.yaml`
      docs; do not fork the package unless the parameter is missing —
      if it is missing, that is the fork reason, record it).
- [ ] Validator client must be gRPC (REST has no Gloas/Heze SSZ codec):
      check the package's prysm launcher; override via `vc_extra_params`.
- [ ] Known benign: the deposit-poller chain-id mismatch (kurtosis network
      id vs config) — same class as the recorded `1 vs 1337`; it only
      disables deposit following. Note it, do not chase it.

## 6.3 The runs

- [ ] Shakeout: `kurtosis run` with 2 nodes, assert startup + first blocks
      (~3 slots). Iterate here — image, genesis, and injection bugs all
      surface at this tier.
- [ ] Full kurtosis run: 10-16 nodes, ~100 validators, 5-6 epochs, matching
      the recorded baseline's shape.
- [ ] ethshadow run: same shape, new data dir + new `shadow-runN.log` in
      `decoupled-shadow-sim` (never touch existing `data*`), Goldfish on.
- [ ] Both runs also flip one knob once (e.g. available-attestation BPS)
      to prove the sweep axis works end to end.

## 6.4 Record

- [ ] Per harness: finalization cadence; available attestations per slot
      per node; per-slot attestation counts and bytes; the step-4 metrics
      (late-vote fraction, gate stops, gate retreats).
- [ ] Kurtosis vs ethshadow vs the recorded plan-5.4 baseline: traffic
      should match the baseline (Goldfish changes head dynamics, not
      message counts); head dynamics may differ. Divergence in traffic is a
      bug, not a finding.
- [ ] Write the numbers into a summary in each run's directory and into an
      executor note in this file.

## 6.5 Verify

- [ ] Both harnesses complete their full runs; summaries written; configs
      committed (this repo: `kurtosis/`; sim workspace: its own history).

---

# Open questions — all resolved, 2026-08-20

Full answers recorded in `plan-next.md`'s open-questions section. Summary:
(1) keep both — add `goldfish_gate_retreat`, leave `reorgCount`; sims must
include late publishers (knob spec'd in plan-next.md step 6).
(2) as recommended — FFG gets the slot-start flag; the available
attestation's BPS config is the head-timing sweep axis; its naming knob is
deferred. (3) NO stub — implement the spec's payload-status derivation
(identical to Gloas mainnet `get_supported_node`); see 4.2. (4) kurtosis
images: whatever is easiest; the Dockerfile route is the starting point and
the executor may switch without asking. Nothing blocks execution.
