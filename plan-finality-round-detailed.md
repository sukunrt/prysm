# Detailed task list: FFG votes relate to rounds, not epochs

Companion to `plan-finality-round.md`. That file holds the scope and the
reasoning; this one holds the work. Every `file:line` below was verified on
2026-08-21 against the current tip and will drift — treat them as pointers,
not addresses; grep for the symbol.

Design record: `task.md` (decisions 10, 15, 17), `plan-final.md` item 1.
The Simplex spec is NOT the design source for this plan — the gadget is
unchanged. The only spec-shaped inputs are the round helpers that already
exist (`compute_round_at_slot` ↔ `slots.RoundAt`, etc.).

## Rules

- **Never abandon or rewrite an existing jj change.** New changes on top of
  the current tip.
- One jj change per numbered group where the tree still compiles at each
  boundary; merge groups rather than land a broken intermediate. Sign with
  a single `Assisted-By: <model>` trailer. No `Co-Authored-By:`.
- `go vet` per touched package. No blanket `go modernize`. Lines under 100
  characters. Never delete data (run outputs, logs, `data*` dirs).
- Full command outputs to scratchpad log files, never through `tail`/`head`.
- No bazel spectests as routine verification. Accept commands per step:
  `go build ./...`, targeted `go test`, `bazelisk build //...`.
- Verification ladder per step: unit tests → ~3-slot single-node smoke
  (the `step4-smoke.sh` / `w2a-smoke.sh` pattern) →
  `TestEndToEnd_HezeGenesisShort` → full `TestEndToEnd_HezeGenesis` → sims.
  Iterate at the cheapest failing tier. No fixed sleeps that race setup;
  assert on observable chain state.

## The identity rule (applies to every step)

`Checkpoint.epoch` keeps its wire/SSZ shape (uint64); its VALUE becomes
the round index, and its Go type becomes `primitives.Round` via the proto
`cast_type` (step 0) so the compiler enforces the unit. With `SlotsPerRound == SlotsPerEpoch` — the shipped mainnet
(`config/params/mainnet_config.go:111`, value 32) and minimal
(`minimal_config.go:48`, value 8) configs — `slots.RoundAt(s) ==
slots.ToEpoch(s)` numerically at every slot and `RoundStart(r) ==
EpochStart(e)` for equal indices. Every site this plan changes therefore
computes *bit-identical values* under the shipped configs. The expected
count of expectation edits in existing tests is **zero** (except tests that
name renamed symbols); the spectest expected-failure set does not move.
Only the devnet/e2e configs (`SLOTS_PER_ROUND: 8` against 32-slot epochs:
`testing/endtoend/heze_e2e_test.go:63`, `fulu-devnet.yaml:40`) exercise
real per-round finality. Non-identity behaviour is covered by new unit
tests that override `SlotsPerRound` to 8 with `SlotsPerEpoch` 32 via
`params.SetupTestConfigCleanup`.

## The one thing not to get wrong

**No mixed units.** After this plan, `Checkpoint.Epoch`,
`AttestationData.Target.Epoch`, `AttestationData.Source.Epoch`, the
forkchoice `Checkpoint.Epoch` (`forkchoice/types/types.go:14`), and the
node fields `justifiedEpoch` / `finalizedEpoch` / `unrealized*Epoch`
(`forkchoice/doubly-linked-tree/types.go:64-67`) all carry ROUNDS — as
the distinct type `primitives.Round`, per the answered typing question
(step 0). Wall clocks and duty machinery keep carrying EPOCHS (shuffling,
seeds, domains, registry). The retype makes every type-level mix a
compile error — for example the goldfish walk's viability inputs at
`forkchoice/doubly-linked-tree/goldfish.go:454,466-467`, which pair
`s.justifiedCheckpoint.Epoch` with `slots.ToEpoch(current)`, stop
compiling until the wall-clock side becomes `slots.RoundAt(current)`.
What the compiler CANNOT catch is a site "fixed" with a careless cast
(`primitives.Round(someEpoch)`) instead of a real conversion — the audit
sections below (1.4, 2.6, 4.1-4.3) enumerate the known pairing sites and
the correct conversion for each, and the rule for the executor is: a bare
`Round(...)` or `Epoch(...)` cast between the two units is FORBIDDEN
outside the two named conversion helpers (`slots.RoundAt`-family and
step 2.6's `helpers.CheckpointEpoch`); the 8/32 unit tests are the net
under everything.

Corollary, stated explicitly (user, 2026-08-21): **committee construction
stays epoch-based.** The shuffle seed (`get_seed` by epoch), the
active-set lookups, `SlotCommitteeCount` / `BeaconCommittee`
(`core/helpers/beacon_committee.go:56,265` — epoch-seeded, round-repeated
since the executed plan step 1), duty enumeration, and aggregator
selection are all untouched by this plan. FFG data changes unit; the
committee machinery does not. The only committee-adjacent edits are the
ones where a (now round-valued) `Target.Epoch` used to be BORROWED as an
epoch for committee/domain/active-set lookups — those derive the epoch
from the attestation's slot instead (3.3, 3.5).

The second thing: **the state side and the forkchoice side of the target
move together** (the plan-next 5.2 lesson). `helpers.FFGTargetRoot`
(`beacon-chain/core/helpers/block.go:100`), the `node.target` assignment
(`forkchoice/doubly-linked-tree/store.go:135-148` — its own comment names
the coupling), and `targetRootForEpoch` (`forkchoice.go:861`) must land in
one change, or `VerifyLmdFfgConsistency`
(`blockchain/receive_attestation.go:56-65`) rejects every vote — loud, but
two layers from the cause.

---

# Step 0 — the retype: checkpoints carry `primitives.Round`

Answered question 2 (user, 2026-08-21): retype, so the compiler catches
the bugs. The wire and SSZ encoding do not change — `cast_type` only
changes the generated Go type, exactly how the same field already casts
to `primitives.Epoch` today.

## 0.1 The type change

- [ ] `proto/prysm/v1alpha1/attestation.proto:95` — `Checkpoint.epoch`:
      change the `cast_type` option from `primitives.Epoch` to
      `primitives.Round`. Regenerate (`make gen proto ssz mode=force`).
      The eth/v1 API mirror (`proto/eth/v1/attestation.proto:91`) is NOT
      retyped — the API layer converts at its translation boundary,
      keeping the public API surface stable.
- [ ] `beacon-chain/forkchoice/types/types.go:14` —
      `forkchoicetypes.Checkpoint.Epoch` becomes `primitives.Round`
      (rename the field to `Round`).
- [ ] `forkchoice/doubly-linked-tree/types.go:64-67` — the node fields
      `justifiedEpoch`, `finalizedEpoch`, `unrealizedJustifiedEpoch`,
      `unrealizedFinalizedEpoch` become `primitives.Round` (rename with
      `Round` suffixes).
- [ ] `consensus-types/primitives/round.go` — extend `Round` with
      whatever the generated code and its consumers need beyond the
      existing arithmetic (at minimum `String`; check what `Epoch`
      provides that the Checkpoint codegen and log/trace call sites use,
      and port only what compiles demand — `Epoch` is 152 lines, a subset
      is enough, per task.md decision 10).

## 0.2 The compile-error sweep IS the audit

Every site that mixes a checkpoint value with an epoch now fails to
compile — including the dormant pre-Heze fork paths (phase0/altair
justification, slasher, validator protection db), because the proto
Checkpoint is shared by all forks. The rule for fixing each error:

- A wall-clock or duty value being compared to a checkpoint: convert the
  WALL-CLOCK side with `slots.RoundAt(...)` (steps 1.4, 3.x name the
  live-path sites).
- A checkpoint value feeding epoch-typed logic (registry gating, leak,
  API responses): convert the CHECKPOINT side with the step 2.6 helper
  (`helpers.CheckpointEpoch`).
- Dormant paths (phase0/pre-Altair processing, slasher, protection db):
  mechanical conversions that preserve today's arithmetic — behavior
  changes there are out of scope (step 6). Prefer keeping their internal
  variables `Round`-typed end to end over sprinkling casts.
- Bare cross-unit casts outside the two helper families are forbidden;
  each one that seems necessary is a finding to record, not a fix.

- [ ] Land step 0 together with step 1 (or immediately before it) in the
      same jj stack — the tree does not compile with the retype alone
      until the sweep is complete, so the retype change itself must
      contain the full sweep. List the touched-package tally in the
      executor note.
- [ ] Accept: `go build ./...` green; full test suite green with zero
      expectation edits (identity rule — the values are unchanged
      everywhere; only types moved).

---

# Step 1 — the per-round target, state side and forkchoice side

## 1.1 `FFGTargetRoot` becomes round-keyed

- [ ] `beacon-chain/core/helpers/block.go:100` — `FFGTargetRoot(state,
      epoch)` becomes `FFGTargetRoot(state, round primitives.Round)`:
      `s := slots.RoundStart(round) - offset`, clamped at the anchor,
      then `BlockRootAtSlot`. **The offset is configurable (answered
      question 1):** a new config value `FFG_TARGET_OFFSET_SLOTS`
      (`config/params/config.go`, `spec:"true"` so it is yaml-sweepable
      per run like `AVAILABLE_ATTESTATION_DUE_BPS_HEZE`), default `1`
      (target = the block at `RoundStart(R) - 1`); `0` targets the
      round's own first slot. Underflow at any offset returns the
      anchor/genesis root (the existing `if s > 0` guard generalizes to
      the clamp). Config plumbing rows: `mainnet_config.go`,
      `minimal_config.go` (both `1`), the `loader.go` print list,
      `loader_test.go` assert list, `rpc/eth/config/handlers.go` + test —
      the same eight-file pattern `SlotsPerRound` used (plan.md step 2).
      Update the doc comment's pseudocode from
      `compute_start_slot_at_epoch` to `compute_start_slot_at_round` with
      the offset. **Both offsets are first-class** (decision 2026-08-21:
      voting for the block at slot -1 or at slot 0 at round
      start): both get unit tests AND a smoke run each (7.2). The
      identity claim holds only at offset 1 (today's `StartSlot(E)-1`
      shape), so the shipped default stays 1 and offset 0 is exercised in
      the 8/32 runs.
- [ ] Offset-0 interaction with vote timing, stated up front: at offset 0
      the target is the round's own first block. A voter in the round's
      FIRST slot that votes at slot start (the
      `--decoupled-ffg-vote-at-slot-start` flag) cannot have seen that
      block yet, so its target root resolves to the previous block and
      the vote misses `is_matching_target` for that slot's committee —
      1/8 of the round's weight at 8-slot rounds, so justification still
      clears (the decision-12 arithmetic, now per round: 87.5% > 2/3).
      Write this expectation into the offset-0 smoke's assertions rather
      than discovering it as a surprise; the offset-0 run is precisely
      the measurement of that loss.
- [ ] Call sites, all pass the vote's round (`data.Target.Epoch`
      reinterpreted, or `time.CurrentRound/PrevRound` from step 2.2):
      `core/altair/attestation.go:365` (`MatchingStatus`),
      `core/epoch/precompute/attestation.go:121` (`SameTarget`),
      `core/epoch/precompute/justification_finalization.go:164,172`
      (`computeCheckpoints`), `testing/util/attestation.go:210` (the test
      attestation builder — it must shift with the helper, exactly as it
      did in plan-next 5.7, or every reward test disagrees with the code).
- [ ] Existing tests `TestFFGTargetRoot_*`
      (`core/helpers/block_test.go:219,247`) get 8/32 round variants: the
      target of round 5 (slots 40-47) is the block at slot 39; round 0 is
      the genesis root.

## 1.2 The forkchoice twin

- [ ] `forkchoice/doubly-linked-tree/store.go:135-148` — the `node.target`
      assignment: the same-epoch inherit test `slots.ToEpoch(slot) ==
      slots.ToEpoch(parent.slot)` becomes `slots.RoundAt(...) ==
      slots.RoundAt(...)`. The anchor (`parent == nil → target = self`) and
      finalized re-anchor (`store.go:274-277`, pruned tree root is its own
      target) arms are unchanged.
- [ ] **The target offset is a symmetric pair.** The `node.target`
      inherit rule above encodes offset 1 (a round's blocks target the
      deepest ancestor in an earlier round). At offset 0 the rule
      changes: a round-start block is its own target and later blocks in
      the round inherit it. Implement the offset once, read from the same
      `FFG_TARGET_OFFSET_SLOTS` config in BOTH the helper (1.1) and the
      insert rule here, with a unit test per offset asserting
      `TargetRootForRound == FFGTargetRoot` on the same chain — if they
      diverge, `VerifyLmdFfgConsistency` rejects every vote.
- [ ] `forkchoice.go:861` — `targetRootForEpoch(root, epoch)` and its
      public wrapper `TargetRootForEpoch` (`:819`): the parameter is now a
      round value. Rename both to `targetRootForRound` /
      `TargetRootForRound` (and the interface entries
      `forkchoice/interfaces.go:94`, `blockchain/chain_info.go:106,563`,
      `forkchoice/ro.go:223`) — a function named `...ForEpoch` taking a
      round is the exact naming debt plan-next 5.7 renamed `BlockRoot`
      to avoid. Inside, the `nodeEpoch == slots.ToEpoch(targetNode.slot)`
      back-off at `:883` becomes `RoundAt`.
- [ ] Callers of `TargetRootForEpoch` switch their argument from
      `slots.ToEpoch(...)` to `slots.RoundAt(...)`:
      `rpc/core/validator.go:537` (step 3.1),
      `rpc/prysm/v1alpha1/validator/proposer_attestations.go:537,546`
      (step 3.6), `sync/validate_beacon_attestation.go:619` (the
      available-attestation checkpoint-state path — it builds a
      `Checkpoint{Epoch: epoch, Root: targetRoot}`; that value becomes the
      round), `blockchain/receive_attestation.go:57`
      (`VerifyLmdFfgConsistency` passes `data.Target.Epoch` — already the
      round after step 3), `verification/data_column.go:386`.
- [ ] `forkchoice.go:267-305` — `IsViableForCheckpoint`:
      `slots.EpochStart(cp.Epoch)` → `RoundStart`, `nodeEpoch+1 ==
      cp.Epoch` → rounds. The plan-next 5.7 boundary lessons (strict
      bound; the child at the boundary's first slot makes its parent
      viable) carry over verbatim with `RoundStart` in place of
      `EpochStart`.
- [ ] `store.go:305,330-333` — `prune`: the finalized boundary
      `slots.EpochStart(finalizedEpoch)` → `RoundStart(finalizedRound)`,
      keeping the strict bound from plan-next 5.7 note 1 (the checkpoint
      block sits one slot before the boundary; the child at the boundary's
      first slot is the canonical chain). `TestStore_PruneKeepsTheEpochStartChild`
      gets a rounds twin.
- [ ] `forkchoice.go:500` — `finalizedSlot := slots.EpochStart(fc.Epoch)`
      → `RoundStart`.
- [ ] `forkchoice.go:158-175` — `updateCheckpoints` compares checkpoint
      epochs to each other only (round vs round) — no change; say so in
      the change description.

## 1.3 The dependent root is an EPOCH concept — separate it from targets

`dependentRootForEpoch` (`forkchoice.go:831-851`, public
`DependentRootForEpoch:814`) serves duty shuffling, which stays
epoch-keyed. Today it rides the `node.target` pointers, which after 1.2
point at ROUND targets — the shortcut breaks silently at 8/32.

- [ ] Read `dependentRootForEpoch` and every caller (grep
      `DependentRootForEpoch`). Reimplement it without `node.target`: walk
      parents by slot to the last slot before `EpochStart(epoch)` (the
      shape of `CanonicalNodeAtSlot`, `gloas.go:24-33`), or keep a
      separate epoch-target pointer on the node if the walk is hot-path.
      Decide after reading the callers; record the choice.
- [ ] Unit test at 8/32: the dependent root for epoch E is unchanged by
      round targets; `TestStore_TargetRootForEpoch`'s dependent-root
      expectations (edited in plan-next 5.7) stay put under identity.

## 1.4 Mixed-units audit, forkchoice package

- [ ] `goldfish.go:454` and `:466-467` — `s.justifiedCheckpoint.Epoch`
      paired with `slots.ToEpoch(current)`: the wall-clock side becomes
      `slots.RoundAt(current)` (the checkpoint side is already a round).
      The `viableForHead` rule (`node.go:18-25`, `n.justifiedEpoch ==
      justifiedEpoch || n.justifiedEpoch+2 >= currentEpoch`) then reads
      entirely in rounds — the `+2` tolerance becomes two ROUNDS; note in
      the change description that this tightens the wall-clock window on
      the devnet by design (checkpoints advance per round).
- [ ] `forkchoice.go:112-118` — `ProcessAttestation`'s vote-freshness rule
      (`targetEpoch > nextEpoch`): **deliberately unchanged.** `f.votes`
      feeds only the pre-Heze walk; the goldfish head never reads it
      (plan-next step 4.4). Leave the epoch granularity and say why.
- [ ] `unrealized_justification.go` — `pullTips:62`: the 2/3-through
      heuristic `slots.SinceEpochStarts(stateSlot)*3 <
      SlotsPerEpoch*2` (`:72`) becomes `SinceRoundStarts` /
      `SlotsPerRound`; the `unrealizedJustifiedEpoch` comparisons against
      `currentEpoch` / `currentEpoch-1` (`:67-72,87-107`) become rounds.
      `setUnrealizedJustifiedEpoch`/`setUnrealizedFinalizedEpoch`
      (`:17,29`) are value-carriers, no change beyond doc comments.
- [ ] `on_tick.go:44` — `NewSlot`'s `!slots.IsEpochStart(slot)` gate on
      `updateUnrealizedCheckpoints` + `prune` becomes `IsRoundStart`
      (the goldfish per-slot hook at `:39-41` is untouched).
- [ ] `forkchoice.go:80` / `store.go:50-51` — `updateBestDescendant`'s
      viability args: the `currentEpoch` argument the pre-Heze walk passes
      must be the round wherever it is compared against node
      justified/finalized fields. Grep every caller of
      `updateBestDescendantConsensusNode` and `viableForHead` and convert
      the wall-clock argument at the source.

## 1.5 Verify

- [ ] Unit ladder: helpers round-target tests; forkchoice insert/target
      tests at 8/32 (a round-start block starts a new target; blocks
      within one round share it); `VerifyLmdFfgConsistency` accepts a
      round-target vote and rejects an epoch-target one at 8/32.
- [ ] Whole existing forkchoice + blockchain suites green with zero
      expectation edits (identity rule). The two known pre-existing
      blockchain failures (`TestStore_NoViableHead_NewPayload`,
      `TestNoViableHead_Reboot`) stay the only ones.
- [ ] `go build ./...`, `go vet` on touched packages.

---

# Step 2 — the cadence: `ProcessRound`

## 2.1 The cadence predicate

- [ ] `beacon-chain/core/time/slot_epoch.go:126-128` — beside
      `CanProcessEpoch`, add `CanProcessRound(state)`:
      `(state.Slot()+1) % SlotsPerRound == 0`. Add `CurrentRound(state)`
      and `PrevRound(state)` helpers mirroring `CurrentEpoch`/`PrevEpoch`
      (floor at round 0).

## 2.2 Split `processEpochGloas`

`beacon-chain/core/transition/gloas.go:137` is the epoch processing Heze
actually runs (`transition.go:337-340` sends `version >= Gloas` there;
Heze sorts above Gloas). Split it into a round part and an epoch part,
**preserving today's exact call order when the boundaries coincide** —
under the identity configs they always do, which is what keeps every
existing transition test green.

**Answered question 3 (user, 2026-08-21): what moves to round cadence is
justification and finalization (J&F) — nothing else.** Rewards,
penalties, and inactivity scores stay at epoch cadence; committee
selection stays epoch-based (charter). One technical dependency follows
from correctness, not preference: per-round J&F needs per-round
PARTICIPATION ROTATION, because the target bits J&F counts must belong to
the round being justified — bits accumulated across a whole epoch would
justify round R on round R-1's votes. And to preserve identity, rewards
at an epoch boundary must still read the PRE-rotation arrays (today
rotation runs late in epoch processing, after rewards, at `gloas.go:198`).
Both constraints are satisfied by placing rotation per-boundary-kind:

- [ ] Read `processEpochGloas` (`gloas.go:137` to end) and list its call
      order in the change description. The round part
      (`processRoundGloas`, new) is exactly:
      `electra.InitializePrecomputeValidators` (`:144`),
      `electra.ProcessEpochParticipation` (`:148`) — the balance
      precompute J&F consumes —,
      `precompute.ProcessJustificationAndFinalizationPreCompute` (`:152`),
      and then `electra.ProcessParticipationFlagUpdates` (`:198`) **only
      when the boundary is NOT also an epoch boundary**. The epoch part
      keeps everything else in today's exact order — inactivity scores,
      rewards/penalties, registry updates, slashings, eth1-data reset,
      pending deposits/consolidations, builder payments, effective
      balances, randao/historical/sync-committee/proposer-lookahead/PTC
      window, and the rotation at its original position after rewards.
      At a coinciding boundary the sequence is therefore: precompute,
      J&F, then the whole remaining epoch body including its
      late rotation — byte-identical to today under the identity
      configs. At a pure round boundary: precompute, J&F, rotation.
- [ ] Accounting consequence, accepted by charter and by answered
      question 3: at 8/32, epoch rewards/penalties read the last round's
      participation only. Consensus-accounting values may be wrong; the
      finality mechanism itself may not — that is why the rotation
      dependency above is not optional. One paragraph in the change
      description states this.
- [ ] `transition.go:293-308` (`ProcessSlotsCore`) — call
      `ProcessRound` for `version >= version.Heze` states when
      `CanProcessRound`, before `ProcessEpoch` (which now runs the slimmed
      epoch part). Epoch boundaries are always round boundaries
      (`VerifyRounds`, `config/params/rounds.go:13-19`), so the coinciding
      order is: round part, then epoch part — byte-identical to today's
      single function under identity. Gloas-and-below versions keep the
      unsplit path untouched (dormant, upstream-rebase surface).
- [ ] The `slots.EpochStart(2)` genesis guard
      (`justification_finalization.go:58-64`) stays epoch-based and
      as-is: justification begins after epoch 2 exactly as today.
      Deliberate — steady-state finality latency is what we measure, and
      keeping the guard avoids re-deriving the genesis corner. One line in
      the change description.

## 2.3 `computeCheckpoints` runs on rounds

- [ ] `justification_finalization.go:153-205` — `prevEpoch/currentEpoch`
      (`:154-155`) become `PrevRound/CurrentRound`; the two justification
      arms pass rounds to `FFGTargetRoot` and stamp
      `justifiedCheckpoint.Epoch = round`; the four finalization rules
      (`:187,192,197,202` — `old.Epoch + {1,2,3} == current`) compare
      rounds. **The rules' code is unchanged** — that is the "gadget
      unchanged" invariant; only the unit the arithmetic runs over moves.
      `processJustificationBits` (`:72-85`) needs no edit at all: it
      shifts once per invocation, and the invocation cadence is what
      moved. The 4-bit window now spans 4 rounds.
- [ ] `UnrealizedCheckpoints` (`:19`) follows for free (same machinery);
      its epoch-keyed genesis guard (`:24`) stays with 2.2's rationale.
- [ ] Precompute tests
      (`TestProcessJustificationAndFinalizationPreCompute_*`) stay put
      under identity; add one 8/32 test: with full participation, round R
      justifies at the R→R+1 boundary and finalizes at R+1→R+2 — finality
      latency 2 rounds = 16 slots.

## 2.4 The state's participation arrays rotate per round

No proto change: `previous/current_epoch_participation`,
`justification_bits`, and the three checkpoints keep their SSZ shape
(`beacon_state.proto:71-78`; state-native accessors
`getters_checkpoint.go`, `setters_checkpoint.go`, `getters_participation.go:69`)
— reinterpretation only. `ProcessParticipationFlagUpdates`
(`core/altair/epoch_spec.go:54`, aliased at `core/electra/transition.go:29`)
is called from the round part; its body is unchanged.

- [ ] Verify no SSZ/state-native edit is needed by building and running
      the state-native suite — the "no change needed" claim is checkable
      because the fields' types and counts are untouched
      (`BeaconStateHezeFieldCount` stays 46, `mainnet_config.go:235`).

## 2.5 Finality delay and the inactivity leak — stays epoch-based

**Answered question 4 (user, 2026-08-21): the leak stays epoch-based;
simplest change.** The only edit is the one the step-0 retype forces:
`FinalityDelay(prevEpoch, finalizedEpoch)`
(`core/helpers/rewards_penalties.go:190-192`) now receives a
round-valued finalized checkpoint, so its callers convert that one input
with the step 2.6 helper — `FinalityDelay(prevEpoch,
helpers.CheckpointEpoch(finalized))` — and everything else
(`IsInInactivityLeak` `:180-182`, `MinEpochsToInactivityPenalty`, the
epoch settlement cadence) is untouched. With per-round finality the
finalized epoch advances at least as fast as before, so the leak arms no
earlier than today.

- [ ] Convert the finalized-round input at the callers:
      `core/altair/epoch_precompute.go:122,273` (the electra/Heze path).
      The phase0 path (`core/epoch/precompute/reward_penalty.go:100-149`)
      is pre-Altair and unreachable at Heze — mechanical retype fixes
      only, say why.
- [ ] One unit test: delay 0 while finality advances per round; delay
      counts epochs when it stalls.

## 2.6 Mixed-units audit, state-internal checkpoint readers

The state's checkpoints now hold rounds, but several state-transition
consumers feed them into EPOCH-typed logic. Each needs an explicit
conversion `epochOf(cp) := slots.ToEpoch(slots.RoundStart(cp.Epoch))`
(add one helper next to `FFGTargetRoot`; name it so the unit is visible,
e.g. `helpers.CheckpointEpoch`):

- [ ] Registry activation gating: `is_eligible_for_activation` compares
      `activation_eligibility_epoch <= finalized_checkpoint.Epoch` — grep
      `FinalizedCheckpointEpoch()` (`state-native/getters_checkpoint.go:116`)
      and `FinalizedCheckpoint().Epoch` in `core/` and convert each
      epoch-typed use. Without this, activations unlock ~4x early on the
      devnet (finalized ROUND 6 read as epoch 6). Enumerate every hit in
      the executor note.
- [ ] Pending-deposit finalization gating (electra path) — same sweep.
- [ ] `rpc/core/validator.go:546-551` — the "advance the head state so
      the justified checkpoint is fresh" branch: `CurrentEpoch(headState)
      < ToEpoch(req.Slot)` becomes rounds, or the source a validator signs
      goes one round stale at every round boundary and its vote fails the
      source match on chains that advanced (this is a per-round-liveness
      bug, not an accounting one).
- [ ] 8/32 unit test for the activation gate and the source freshness.

## 2.7 Verify

- [ ] Unit ladder (2.3, 2.5, 2.6 tests) plus the full `core/...` suites
      green with zero expectation edits under identity.
- [ ] 3-slot smoke at 8/32: the node starts and processes round
      boundaries without epoch processing firing mid-epoch (log-assert
      on observable state: justified checkpoint advances at slot 15/16
      boundary region, not before).

---

# Step 3 — attestation plumbing

## 3.1 Data construction (server)

- [ ] `rpc/core/validator.go:536-537` — `targetEpoch :=
      slots.ToEpoch(req.Slot)` → `targetRound := slots.RoundAt(req.Slot)`;
      `TargetRootForRound(headRoot, targetRound)`. The returned
      `AttestationData.Target.Epoch` field carries the round; `Source` is
      the state's justified checkpoint (already round-valued after
      step 2). The forkchoice-checkpoint cache writes
      (`:565-578`) carry rounds in their `Epoch` fields — values only.
- [ ] The freshness branch `:546-551` is 2.6's item — cross-reference,
      don't do it twice.

## 3.2 The one predicate every path calls

- [ ] `core/helpers/attestation.go:44-49` — `ValidateSlotTargetEpoch`:
      `slots.ToEpoch(data.Slot) != data.Target.Epoch` becomes
      `slots.RoundAt(data.Slot) != primitives.Round(data.Target.Epoch)`
      (or an equivalent same-unit comparison). Rename to
      `ValidateSlotTargetRound`; callers:
      `core/blocks/attestation.go:85`,
      `sync/validate_beacon_attestation.go:96`,
      `sync/validate_aggregate_proof.go:79`,
      `blockchain/process_attestation.go:49`.

## 3.3 State-transition validation windows

`core/blocks/attestation.go:52` (`VerifyAttestationNoVerifySignature`):

- [ ] `:63-73` — the inclusion window `Target.Epoch ∈ {prev, current}`
      epoch becomes `{PrevRound, CurrentRound}`. At 8/32 an attestation is
      includable for up to 2 rounds (16 slots) — this narrowing is the
      intended semantics (votes for round R are consumed by the R→R+1
      boundary; R+1 inclusion feeds the previous-round bit).
- [ ] `:75-83` — the source match (current-round target →
      `MatchCurrentJustifiedCheckpoint`, else previous): logic unchanged,
      units follow. No edit beyond the epoch/round variable rename.
- [ ] `:100-110` — the pre-Deneb `state.Slot() <= slot + SlotsPerEpoch`
      cap: unreachable at Heze (post-Deneb). Deliberately unchanged.
- [ ] `:111` — `ActiveValidatorCount(ctx, st, data.Target.Epoch)`: the
      active set is an EPOCH concept; derive it from the slot —
      `slots.ToEpoch(data.Slot)` — not from the (now round-valued) target
      field. Same class of fix wherever `Target.Epoch` reached committee,
      domain, or active-set logic: grep `Target.Epoch` across
      `beacon-chain/` and `validator/` and classify every hit as
      round-comparison (keep) or epoch-derivation (switch to slot). Put
      the classified list in the executor note.
- [ ] Phase0 pending-attestation path (`core/blocks/attestation.go:203,
      215,229,268`): pre-Altair, unreachable at Heze — deliberately
      unchanged, why noted.

## 3.4 Participation selection and flag matching

- [ ] `core/altair/attestation.go:141-143` and
      `core/electra/attestation.go:58-61` — `targetEpoch == currentEpoch`
      selects current vs previous participation array: becomes rounds.
- [ ] `core/altair/attestation.go:302-350`
      (`AttestationParticipationFlagIndices`): the source-oracle selection
      by target round (`:303-309`); `MatchingStatus` (`:362-371`) passes
      the round to `FFGTargetRoot` (already step 1.1's caller list).
      The timely-source delay bound `SqrRootSlotsPerEpoch` (`:324-327`)
      stays epoch-derived — accounting fidelity is explicitly secondary
      (task charter) and the window still fits inside a round at 8/32
      (√32=5 < 8). Deliberately unchanged, why noted.
- [ ] `core/altair/attestation.go:107` — the target-epoch value threaded
      into `SetParticipationAndRewardProposer` is the round; rename the
      parameter.

## 3.5 Gossip and on_attestation

- [ ] `sync/validate_beacon_attestation.go:137-143` —
      `VerifyLmdFfgConsistency` + `AttestationTargetState`: consistency is
      step 1.2; the target state resolution
      (`blockchain/receive_attestation.go:41-53`) converts
      `slots.EpochStart(target.Epoch)` → `RoundStart`, and its wall-clock
      guard follows.
- [ ] `blockchain/process_attestation_helpers.go` — `getAttPreState:94`
      (multilock key root+epoch → root+round, `:101`), the
      checkpoint-state materialization `slots.EpochStart(c.Epoch)` →
      `RoundStart` (`:114`), `:61,148` same conversion;
      `verifyAttTargetEpoch:168-180` — target must be the current or
      previous ROUND of the wall clock (`slots.RoundAt(currentSlot)`).
      The checkpoint-state cache
      (`checkpointStateCache.StateByCheckpoint`, `:105`) is keyed by the
      checkpoint value — rounds key it just as well; state it as the
      no-change why.
- [ ] `blockchain/process_attestation.go:104` — `attData.Target.Epoch >=
      params.BeaconConfig().GloasForkEpoch` mixes a round with a config
      epoch. Rewrite as a same-unit comparison
      (`primitives.Round(attData.Target.Epoch) >=
      slots.RoundAt(slots.EpochStart(GloasForkEpoch))`, or gate on the
      attestation slot's epoch). Identity-safe either way at fork epoch 0.
- [ ] `sync/validate_beacon_attestation.go:498-514` — the debug helper's
      `sourceEpoch`/`targetEpoch` fields: relabel `sourceRound`/
      `targetRound` (step 5's logging sweep owns the pattern; this is the
      one inside the gossip path).
- [ ] `sync/validate_beacon_attestation.go:618-627`
      (`validateAvailableAttWithBlock`) — the goldfish vote's checkpoint
      state is resolved via `epoch := slots.ToEpoch(att.Data.Slot)` +
      `TargetRootForEpoch`: becomes `RoundAt` + `TargetRootForRound`, and
      the synthetic `Checkpoint{Epoch: ...}` carries the round.
- [ ] Signing domains: `validator/client/attest.go:242` —
      `domainData(ctx, data.Target.Epoch, ...)` must derive from the SLOT
      (`slots.ToEpoch(data.Slot)`), and the server-side verification
      domain derivation must move in the SAME change — grep
      `DomainBeaconAttester` for every derivation from `Target.Epoch`
      (core/blocks signature sets, sync, validator client, slasher
      verification) and convert each to slot-derived epoch. Under
      identity the values are equal, so nothing shifts; at 8/32 a missed
      site is an immediate signature-verification failure — loud, but
      enumerate anyway (symmetric pairs).

## 3.6 Pool, packing, aggregation

- [ ] Proposer packing filters
      (`rpc/prysm/v1alpha1/validator/proposer_attestations.go:460-560`):
      `isAttestationFromCurrentEpoch/PreviousEpoch` (`:460-466`) compare
      the target round to `slots.RoundAt(current slot)`; the
      target-root filters (`:501-519`) resolve both roots via
      `TargetRootForRound` with `targetRound = slots.RoundAt(headSlot)`
      and `prevTargetRound = targetRound-1` (`:528-560`). Rename the
      helpers' Epoch suffixes.
- [ ] Pool pruning (`operations/attestations/prune_expired.go:96-125`)
      — **deliberately unchanged**: the epoch-based retention keeps
      attestations longer than the state's round window accepts, which
      wastes a little pool memory and nothing else; the packing filters
      above are what guarantee only includable attestations are packed.
      Checkable why: `expired` is consulted only by pool pruning, not by
      inclusion validation.
- [ ] Aggregator dedup: already round-keyed
      (`sync/validate_aggregate_proof.go:264-284`, key = round ‖
      aggregator). No change; verified.
- [ ] Seen-bits caches (`operations/attestations/kv/kv.go:36`,
      `seen_bits.go:35-39`) — TTL-based, unit-agnostic; no change, why
      noted.

## 3.7 Verify

- [ ] Unit ladder: 8/32 tests for the window (round-R vote includable in
      R and R+1, rejected in R+2), the source match across a round
      boundary, domain-from-slot (sign at round 5 epoch 1: domain uses
      epoch 1), packing filter round window.
- [ ] Existing `core/blocks`, `core/altair`, `core/electra`, `sync`,
      `operations` suites: zero expectation edits (identity).
- [ ] 3-slot smoke, then `TestEndToEnd_HezeGenesisShort`.

---

# Step 4 — every consumer of `checkpoint.Epoch → slot`

The service layer converts checkpoint epochs to slots in ~27 non-test
sites. Each becomes `slots.RoundStart(...)`. Grouped, with the full grep
(`EpochStart(` over `beacon-chain/`, filter checkpoint-typed arguments)
re-run at execution time — the list below is the verified 2026-08-21 set:

## 4.1 blockchain

- [ ] `process_block_helpers.go:287` (`verifyBlkFinalizedSlot`), `:383`
      (`fillInForkChoiceMissingBlocks` floor; the `:376` sanity check
      `fCheckpoint.Epoch > jCheckpoint.Epoch` is round-vs-round, no
      change), `updateFinalized` `:301-312` (round-vs-round compare, no
      change beyond doc).
- [ ] `process_block.go:296-297,389-390,439,445` — checkpoint threading
      into forkchoice/db: value-carriers, no conversion; verify by
      reading each (the claim's why: they pass checkpoints whole, never
      convert to slots).
- [ ] `weak_subjectivity_checks.go:40` — WS checkpoint `EpochStart` →
      `RoundStart`; the `--weak-subjectivity-checkpoint=<root>:<N>` flag's
      N is now a ROUND — document in the flag's usage string.

## 4.2 sync, db, rpc

- [ ] `sync/service.go:571`; `sync/validate_beacon_blocks.go:155`;
      `sync/custody.go:39,53`;
      `sync/rpc_execution_payload_envelopes_by_root.go:62`;
      `sync/initial-sync/round_robin.go:587`;
      `verification/blob.go:147`, `verification/data_column.go:268`.
- [ ] `sync/rpc_status.go` — the handshake. `FinalizedEpoch` on the wire
      now carries the finalized ROUND (all peers run this software; no
      compatibility concern — the mock's network is closed). Convert
      `:437-452`'s `maxFinalizedEpoch = maxEpoch - 2` guard to rounds
      against the wall-clock round, and the `xrmqllqx` strict child bound
      (`:487-497`): `startSlot` comes from `RoundStart(finalized round)`,
      keeping the strict `>` (the checkpoint block sits one slot before
      the ROUND boundary now). `peers/status.go:721-750` best-finalized
      voting compares peer values to each other — rounds sort the same;
      no change, why noted.
- [ ] `db/kv/state.go:1041`; `db/kv/checkpoint.go:19-63`
      (save/load are value-carriers — no change, why: they never convert);
      `db/kv/finalized_block_roots.go:53-62,164` — read it: if the
      backfill iterates epochs from `previousFinalizedCheckpoint.Epoch`,
      the iteration unit becomes rounds; verify the index it feeds is
      root-keyed (expected) and record the finding.
- [ ] `rpc/core/beacon.go:92-106` — ChainHead fSlot/jSlot/pjSlot via
      `RoundStart`; the proto fields named `*Epoch` now carry rounds
      (step 5 owns the reporting decision).
- [ ] `rpc/lookup/stater.go:143,157` — `"justified"`/`"finalized"`
      state-id resolution → `RoundStart`.
- [ ] `rpc/eth/helpers/sync.go:83` — same.
- [ ] `sync/checkpoint/weak-subjectivity.go:30,47,106` — checkpoint-sync
      origin WS epoch: converts via `RoundStart`; the user-facing
      `--checkpoint-sync-url`/WS inputs now speak rounds — document.
- [ ] Engine API: finalized/justified are sent to the EL as block HASHES
      resolved from roots, never epochs — verify by reading the
      forkchoiceUpdated call construction and record the no-change why.

## 4.3 Checkpoint sync and the cold-start pair

- [ ] Fresh start, restart, checkpoint sync (symmetric triple): the
      origin checkpoint loaded from db carries a round; `getters/setters`
      pass it through untouched. The e2e checkpoint-sync variant
      (`TestEndToEnd_HezeGenesisCheckpointSync`,
      `testing/endtoend/heze_e2e_test.go:123`) is the integration
      witness — it must pass at 8/32 with per-round finality, proving the
      anchor round threads through Status, initial sync, and forkchoice
      bootstrapping.

## 4.4 Verify

- [ ] `go build ./...`; targeted suites (`blockchain`, `sync`, `db/kv`,
      `rpc/...`) zero expectation edits under identity.
- [ ] `TestEndToEnd_HezeGenesisShort`, then the checkpoint-sync variant.

---

# Step 5 — logging, metrics, and reporting (user emphasis, 2026-08-21)

The rule: a number that is a round must not be labeled "epoch" anywhere a
human or a dashboard reads it. Sweep `grep -rn "justifiedEpoch\|
finalizedEpoch\|targetEpoch\|sourceEpoch"` over log fields after the code
moves.

## 5.1 Log fields

- [ ] `blockchain/log_helpers.go:115,125-127` — `finalizedEpoch` /
      `justifiedEpoch` log keys become `finalizedRound` /
      `justifiedRound`; add a derived `finalizedSlot`
      (`RoundStart(round)`) to the finalized line so runs remain
      comparable to old logs at a glance.
- [ ] `validator/client/attest.go:188-189,336-338` — trace/log fields
      `sourceEpoch`/`targetEpoch` → `sourceRound`/`targetRound`.
- [ ] `sync/validate_beacon_attestation.go:498-514` — done in 3.5;
      cross-reference.
- [ ] Sweep the remaining hits of the grep above; relabel or convert
      each; list them in the executor note.

## 5.2 Prometheus metrics

- [ ] `blockchain/metrics.go:34-66,272,357-365` — the
      `beacon_finalized_epoch` / `beacon_current_justified_epoch` /
      `beacon_previous_justified_epoch` / `head_finalized_epoch` gauges:
      keep the names and make them emit the EPOCH of the checkpoint's
      round-start slot (`ToEpoch(RoundStart(round))`), so every existing
      dashboard, the kurtosis scrape tooling, and the e2e logs stay
      meaningful. Add four new gauges carrying the raw rounds:
      `beacon_finalized_round`, `beacon_current_justified_round`,
      `beacon_previous_justified_round`, `head_finalized_round`.
      (Answered question 6: build exactly this.)
- [ ] New headline metrics, landed WITH step 2 (the run-05 pattern —
      name them up front):
      - `finality_latency_slots` (gauge): `currentSlot -
        RoundStart(finalizedRound)`, updated on finalized-checkpoint
        advance.
      - `justified_round_advance_total` (counter): incremented each time
        the justified checkpoint's round increases — with
        `rounds_elapsed` derivable from slot, the per-round justification
        rate is `advance_total / (currentRound - startRound)`.
      - `finalized_round_advance_total` (counter): same for finality.
- [ ] `reportEpochMetrics` (`blockchain/metrics.go:277`) — reads
      participation; it now reflects the last ROUND's arrays. Read it and
      either move its invocation to round cadence or relabel its outputs;
      record the choice (accounting fidelity is secondary, legibility is
      not).

## 5.3 APIs and the e2e evaluators

- [ ] ChainHead (`rpc/core/beacon.go`) and the eth API checkpoint
      responses now carry rounds in `*Epoch` fields. No proto change
      (mock rule); one comment at the ChainHead construction site names
      the unit.
- [ ] **Verifying that justification/finalization is PROGRESSING is a
      stated requirement (user, 2026-08-21), not a nice-to-have.** It is
      asserted at every ladder tier:
      - Unit (2.3): full participation justifies round R at the R→R+1
        boundary and finalizes at R+1→R+2.
      - Smoke: the chain-check script (the `w2a-chain-check.py` pattern)
        reads the metrics dump and asserts the justified round advanced
        at every round boundary after warmup and the finalized round
        trails by exactly one — a stalled round is a failure, not a note.
      - e2e: two evaluators. `FinalizationOccursInRounds` — the rounds
        twin of `finalizationOccurs` (`finality.go:18-61`): finalized
        round == wall-clock round − 2 and justified == wall-clock round
        − 1 in steady state (after the epoch-2 genesis guard clears),
        wall-clock round computed from the head slot. And
        `JustificationAdvancesEveryRound` — run per epoch like
        `AttestationsInEveryRound` (`evaluators/rounds.go:40`), asserting
        the justified checkpoint's round increased in each of the epoch's
        rounds (no gaps), so a single skipped round is caught even when
        the endpoint check would still pass. Swap them into the Heze e2e
        runs (`heze_e2e_test.go:58,123,157`) in place of the stock
        evaluator; non-Heze suites keep the original.
- [ ] `ValidatorsParticipating*` evaluators read participation via the
      API — per-round arrays still show ~full participation each epoch
      query; verify once at 8/32 and record (no change expected).

## 5.4 Verify

- [ ] 3-slot smoke: grep the smoke log for `finalizedRound`,
      `finality_latency_slots`, and confirm no remaining `Epoch`-labeled
      round values (scripted grep, full output to file).
- [ ] Full e2e with `FinalizationOccursInRounds` green.

---

# Step 6 — slashing: out of scope, zero-change path (user, 2026-08-21)

Slashing — the consensus predicates, surround detection, the slasher, and
EIP-3076 local protection — is explicitly a non-goal. Take the easiest
path: **change nothing.**

- [ ] `slashings.IsSurround` (`proto/prysm/v1alpha1/slashings/
      surround_votes.go:13-15`), `IsSlashableAttestationData`
      (`core/blocks/attester_slashing.go:191-202`), and the whole
      `beacon-chain/slasher/` package: untouched. The predicates are
      unit-agnostic value comparisons and keep compiling and running on
      round-valued inputs; whatever they detect or miss is not measured.
- [ ] `validator/client/attest.go:125-141` — the first-round-of-epoch
      protection gate: **left exactly as is** (zero change). The
      protection db stores monotonically growing values either way. (A
      possible later cleanup — per-round targets would let the gate be
      removed — is noted here and deliberately not done.)
- [ ] The only slashing-adjacent edits anywhere in this plan are (a) the
      signing-domain derivation in step 3.5, needed for signature
      validity, not slashing, and (b) the mechanical type fixes the
      step-0 retype forces in the slasher and protection-db code —
      value-preserving conversions only, per the step 0.2 rules; zero
      behavior change.
- [ ] Verify: `validator/client` and `slasher` suites compile and pass
      with zero expectation edits (identity rule).

---

# Step 7 — spectest survey, e2e, and measurement

## 7.1 Spectest survey extension (survey-only; fix nothing)

- [ ] Re-run the `SURVEY-2026-08-21.md` sweep
      (`testing/spectest/SURVEY-2026-08-21.md:326` has the command) on
      the finished stack. Expected result under the identity rule: the
      SAME 486 target-shift failures (the shift is now round-keyed, but
      identity makes `RoundStart-1 == EpochStart-1` on spectest configs)
      plus the 10 attributed stock-Gloas ones; delta zero. Extend the
      survey file with a dated section recording the actual delta and
      attributing every new failure (the vopzvxty/wvornuor pattern). A
      nonzero delta is a bug in the identity claim — investigate before
      landing, do not record-and-move-on.

## 7.2 e2e ladder

- [ ] `TestEndToEnd_HezeGenesisShort` (1 epoch) — regression tier, after
      each step.
- [ ] Full `TestEndToEnd_HezeGenesis` (5 epochs, 8/32) with
      `FinalizationOccursInRounds` and `JustificationAdvancesEveryRound`
      (5.3): justification advances every round, finalization latency 2
      rounds; the goldfish evaluators (`AvailableAttestationsFlow`,
      `AttestationsInEveryRound`, `ChainProducesBlocks`) unchanged and
      green.
- [ ] Offset sweep (answered question 1): one smoke per target offset —
      offset 1 (default) and offset 0 — each asserting per-round J/F
      progression via the chain-check script; the offset-0 run
      additionally records the first-slot target-miss fraction predicted
      in 1.1. The offset is yaml (`FFG_TARGET_OFFSET_SLOTS`), so the
      sweep is config-only, no build variants.
- [ ] `TestEndToEnd_HezeGenesisCheckpointSync` — the cold-start witness
      for round-valued anchors (step 4.3).
- [ ] `TestEndToEnd_HezeGenesisSlotStartFFG` — slot-start vote + rounds
      together; the 0.95 participation floor
      (`heze_e2e_test.go:165-168`) may need re-derivation at per-round
      targets — measure first, then set, and record the number.
- [ ] Known infra caveat from plan-next 4.9: the vendored-geth
      `blockAccessList` JSON mismatch may still block the full e2e in
      this checkout; if it does, the single-node smoke on local geth
      1.17.6 (the `w2a-smoke.sh` shape: 256 validators, 8/32, ~5 epochs)
      is the substitute witness, with the chain-check script asserting
      per-round justified/finalized advance from the metrics dump.

## 7.3 Measurement (the point of the whole plan)

- [ ] Kurtosis: rerun the run-02 shape (10 nodes, 130 validators, 6
      epochs, late publishers on) on the new stack. Record, against
      `kurtosis/runs/02-main/summary.md` and the Shadow `data19`
      baseline:
      - **finality latency in slots** (headline): expect ~16 (2 rounds)
        against run 02's ~128 (2 epochs, finalized epoch 4 of 6);
      - per-round justification rate (from
        `justified_round_advance_total`);
      - unchanged: attestation traffic per slot per node (rounds change
        WHEN finality happens, not how many votes exist — divergence in
        traffic is a bug, not a finding), goldfish seat fraction, gate
        retreats matching late publishers.
- [ ] New run directory + `summary.md` under `kurtosis/runs/`, configs
      committed; bulky logs to `~/dev/prysm2-run-logs/` (never delete
      prior runs).
- [ ] The ethshadow half remains open from plan-next 6.3 and is not made
      worse here; a Shadow rerun is optional scope — flag to the user at
      execution time rather than deciding here.

---

# Deliberately unchanged (whole-plan table)

| site | why it is safe |
|---|---|
| FFG quorum math, justification bits shift, k-finality rules | code untouched; only invocation cadence and input units move (the "gadget unchanged" invariant) |
| Goldfish walk, gate, passthrough, proposal stub, vote store | reads the justified checkpoint as its stable-root stub; per-round justification only makes the stub advance faster; 1.4 converts its two wall-clock pairings |
| Available-attestation stream (topic, validator, RPCs, mock committee) | carries no FFG data; its checkpoint-state resolution is converted in 3.5 and nothing else touches units |
| Committees, shuffle, seeds, proposer selection, RANDAO | epoch-keyed by design; rounds reuse the epoch shuffle via slot-in-round (plan step 2, executed) |
| `f.votes` + `ProcessAttestation` epoch granularity | pre-Heze head path only; goldfish never reads it (plan-next 4.4) |
| Pool pruning epoch windows (`prune_expired.go`) | retention only; packing filters (3.6) gate inclusion |
| Phase0/pre-Altair attestation + reward paths | unreachable at Heze (version dispatch at `transition.go:337`) |
| All of slashing: predicates, slasher, EIP-3076 gate | non-goal (user 2026-08-21); unit-agnostic comparisons keep working on round values; see step 6 |
| Gossip attestation propagation window (`LATEST_MESSAGE`-style slot ranges) | wire acceptance is slot-based and unchanged; state acceptance narrows via 3.3 — wire behavior must stay real (task charter) |
| db/kv checkpoint save/load, state schema, SSZ shapes | value-carriers; no field type or count changes anywhere in this plan |
| Slasher span algorithms | unit-agnostic comparisons; window reinterpretation documented in 6.1 |
| `AttestationDueBPS = 3333`, `AVAILABLE_ATTESTATION_DUE_BPS_HEZE` | timing knobs, orthogonal (plan-next steps 4-5, executed) |

# Tests that will wake up (enumerate before running)

Under identity the expectation-edit count is zero; the suites to RUN and
watch: `core/helpers`, `core/blocks`, `core/altair`, `core/electra`,
`core/epoch/precompute`, `core/transition`, `forkchoice/doubly-linked-tree`,
`blockchain`, `sync`, `operations/attestations`,
`rpc/core` + `rpc/prysm/v1alpha1/validator`, `validator/client`,
`validator/db/kv`, `slasher`. Renamed symbols (`ValidateSlotTargetEpoch`,
`TargetRootForEpoch`, `FFGTargetRoot` signature) mean mechanical test
edits in their own test files — mechanical renames are not expectation
edits; anything beyond a rename is a finding to investigate. The two known
pre-existing blockchain failures and the 32 pre-existing rpc failures
(plan-next 5.7) stay the baseline. New files need BUILD entries; run
gazelle on touched packages.

# Open questions — all resolved, 2026-08-21

1. **Target slot.** ANSWERED: **configurable.** `FFG_TARGET_OFFSET_SLOTS`
   config value (yaml-sweepable), default 1 (`RoundStart(R) - 1`), 0
   targets the round's own first slot. Built in 1.1, with the forkchoice
   twin honoring the same value (1.2).
2. **Checkpoint unit typing.** ANSWERED: **retype, so the compiler
   catches bugs** — step 0: proto `cast_type` of `Checkpoint.epoch` →
   `primitives.Round`, plus the forkchoice checkpoint and node fields.
   A follow-up question compared this with the simplex spec's own
   direction — `Checkpoint{slot, root}` carrying the proposal slot, which
   removes unit conversion structurally — and the user chose **smallest
   change**: round-valued stands (identity trick intact, zero expectation
   edits); slot-valued checkpoints are the gadget-era container change,
   not this plan's.
3. **What moves to round cadence.** ANSWERED: **justification and
   finalization only.** Committee selection stays epoch-based;
   rewards/penalties/inactivity stay epoch-based. The one dependency —
   per-round participation rotation, needed so J&F counts the right
   round's bits — is encoded in 2.2 with the boundary-kind placement that
   keeps epoch rewards reading pre-rotation arrays (identity preserved).
4. **The inactivity leak.** Resolved: stays epoch-based;
   simplest change — one input conversion at the callers (2.5),
   nothing else.
5. **Wire/pool retention.** ANSWERED: **leave as is** — slot-based gossip
   windows and epoch-based pool pruning unchanged while state acceptance
   narrows to the round pair (3.3, 3.6).
6. **Metrics.** ANSWERED: **new metrics are needed** — build 5.2 as
   written: the new `beacon_*_round` gauges, `finality_latency_slots`,
   and the round-advance counters, with the existing `*_epoch` gauges
   emitting the epoch of the round-start slot so dashboards and the
   scrape tooling keep meaning.

(The former question 7, slashing-protection cadence, was removed when
slashing became a non-goal — step 6.)
