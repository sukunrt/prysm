# Detailed task list: FFG votes relate to rounds, not epochs — complete

Companion to `plan-complete.md`. That file holds the scope and the
reasoning; this one holds the work. Every `file:line` below is verified
against `ststlkmp` (a313817d), the tip this plan executes from, and will
drift as the stacks land — treat them as pointers, not addresses; grep for
the symbol.

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
  characters. Never delete data (run outputs, logs, `data*` dirs) — with
  one authorized exception: spent enclaves and Shadow `data*` dirs whose
  results are recorded in a committed summary are disposable when disk
  space is needed. Budget disk BEFORE the measurement phase: one kurtosis
  enclave is ~10-15 GB of container volumes, one Shadow run is several GB,
  and `~/.cache/bazel` grows to tens of GB; survey and clear the largest
  disposable targets first, never the smallest.
- Full command outputs to scratchpad log files, never through `tail`/`head`
  (write the file, then grep/read it).
- No bazel spectests as routine verification. Accept commands per step:
  `go build ./...`, targeted `go test`, `bazelisk build //...`.
  `beacon-chain/blockchain` tests abort without `-tags develop`.
- Verification ladder per step: unit tests → ~3-slot single-node smoke →
  `TestEndToEnd_HezeGenesisShort` → full `TestEndToEnd_HezeGenesis` → sims.
  Iterate at the cheapest failing tier. No fixed sleeps that race setup;
  assert on observable chain state. **The Short e2e is a stack-A gate, not
  a post-stack luxury** — see "Commit structure".
- **The lifetime-audit rule.** Any change to prune cadence or retention
  requires an audit of every consumer that walks the fork-choice tree by
  AGE (dependent roots, target walks), proving the node it needs is still
  in the tree at the moment it asks — value-correctness proofs on an
  unpruned tree prove nothing about a pruned one. The audit's unit tests
  must actually prune.
- **The instrument-first rule.** Before any measurement run, every path
  that can discard a message on a measured stream increments a labeled
  counter, and per-vote accounting tooling exists (step 7). On a clean
  simulation the accounting reads exactly 100%; anything less is a code
  bug to fix in a fix→run loop, never a residual to caption.

## The identity rule (applies to every step)

`Checkpoint.epoch` keeps its wire/SSZ shape (uint64); its VALUE becomes
the round index, and its Go type becomes `primitives.Round` via the proto
`cast_type` (step 0) so the compiler enforces the unit. With
`SlotsPerRound == SlotsPerEpoch` — the shipped mainnet
(`config/params/mainnet_config.go:111`, value 32) and minimal
(`minimal_config.go:48`, value 8) configs — `slots.RoundAt(s) ==
slots.ToEpoch(s)` numerically at every slot and `RoundStart(r) ==
EpochStart(e)` for equal indices. Every site this plan changes therefore
computes *bit-identical values* under the shipped configs. The expected
count of expectation edits in existing tests is **zero** (except tests
that name renamed symbols, one goldfish cold-start fixture named in 1.4,
and label-string asserts named in 5.1); the spectest expected-failure set
does not move. Only the devnet/e2e configs (`SLOTS_PER_ROUND: 8` against
32-slot epochs: `testing/endtoend/heze_e2e_test.go:63`,
`fulu-devnet.yaml:40`) exercise real per-round finality. Non-identity
behaviour is covered by new unit tests that override `SlotsPerRound` to 8
with `SlotsPerEpoch` 32 via `params.SetupTestConfigCleanup`.

## The three things not to get wrong

**First: no mixed units.** After this plan, `Checkpoint.Epoch`,
`AttestationData.Target.Epoch`, `AttestationData.Source.Epoch`, the
forkchoice `Checkpoint.Epoch` (`forkchoice/types/types.go:14`), and the
node fields `justifiedEpoch` / `finalizedEpoch` / `unrealized*Epoch`
(`forkchoice/doubly-linked-tree/types.go:64-67`) all carry ROUNDS — as
the distinct type `primitives.Round` (step 0). Wall clocks and duty
machinery keep carrying EPOCHS (shuffling, seeds, domains, registry). The
retype makes every type-level mix a compile error — for example the
goldfish walk's viability inputs at
`forkchoice/doubly-linked-tree/goldfish.go:454,466-467`, which pair
`s.justifiedCheckpoint.Epoch` with `slots.ToEpoch(current)`, stop
compiling until the wall-clock side becomes `slots.RoundAt(current)`.
What the compiler CANNOT catch is a site "fixed" with a careless cast
(`primitives.Round(someEpoch)`) instead of a real conversion — the audit
sections (1.4, 2.6, 4.0-4.3) enumerate the pairing sites and the correct
conversion for each. The rule for the executor: a bare `Round(...)` or
`Epoch(...)` cast between the two units is FORBIDDEN outside the two
named conversion helpers (`slots.RoundAt`-family, which includes
`slots.FFGTargetSlot`, and `helpers.CheckpointEpoch`, plus its one
sanctioned state-native twin — 2.6); the 8/32 unit tests are the net
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

**Second: the state side and the forkchoice side of the target move
together** (the plan-next 5.2 lesson). `helpers.FFGTargetRoot`
(`beacon-chain/core/helpers/block.go:100`), the `node.target` assignment
(`forkchoice/doubly-linked-tree/store.go:135-148` — its own comment names
the coupling), and `targetRootForEpoch` (`forkchoice.go:861`) must land in
one change, or `VerifyLmdFfgConsistency`
(`blockchain/receive_attestation.go:56-65`) rejects every vote — loud, but
two layers from the cause. Both sides read the offset from ONE function,
`slots.FFGTargetSlot` (1.1), so they cannot drift.

**Third: value-correctness is not reachability.** Per-round finality
advances the finalized checkpoint every ~2 rounds and pruning follows
finality — but `dependentRootForEpoch` serves EPOCH-keyed shuffling and
its callers ask for `currentEpoch−1`, whose anchor block sits up to two
epochs behind head. Cut the tree at the finalized round and that node is
below the tree root on every prune; every subsequent block insert fails
with `ErrNilNode` and the chain wedges on all nodes at the first
round-boundary prune after finality starts moving. The pruning horizon is
therefore a RETENTION FLOOR set by the epoch-keyed consumers, decoupled
from the finality clock (1.3a). Every proof about the dependent root must
be made twice: once for the value, once for the node's lifetime on a
pruned tree — and the tests must prune.

## Commit structure

The retype makes most of the plan's mechanical conversions COMPILE
ERRORS, so they cannot land as separate later changes. The steps below
remain the reference material — each enumerated site with its correct
conversion — but the work lands as four jj stacks:

- **Stack A — retype + sweep + target + forkchoice lifetime.** Step 0
  (Round SSZ methods → regenerate → compile-error sweep, using 1.4, 2.5,
  2.6, 3.2-3.6, 4.0-4.3 as the per-site conversion guide) together with
  step 1 entire — the target shift, offset config, AND the lifetime work
  (1.3a, 1.3b): the horizon, the unwind, and the prune bound are part of
  the same forkchoice change, not fixes for later. Tree compiles only at
  the end of the stack; merge rather than land broken intermediates.
  **Gate: run `TestEndToEnd_HezeGenesisShort` at the end of stack A**,
  before stack B starts — the lifetime class of bug is only visible at
  the chain tier, and one epoch of live chain (3 minutes) is the
  cheapest place it can surface.
- **Stack B — cadence.** Step 2's `ProcessRound` hook, the Heze
  epoch-processing pair, and the round-clock genesis guard.
- **Stack C — reporting.** Step 5's logging sweep, new metrics, and the
  two e2e evaluators.
- **Stack D — delivery + measurement.** Step 7's vote-delivery mechanisms
  and instrumentation, then step 8's runs and the survey extension. The
  instrumentation lands BEFORE the first measurement run.

Steps 3, 4, and 6 have no stacks of their own: their mechanical halves
execute inside stack A's sweep; their behavioral remainders (packing
filters, the stater resolution, evaluator swaps) ride the stack that owns
the behavior.

---

# Step 0 — the retype: checkpoints carry `primitives.Round`

Answered question 2 (user, 2026-08-21): retype, so the compiler catches
the bugs. The wire and SSZ encoding do not change — `cast_type` only
changes the generated Go type, exactly how the same field already casts
to `primitives.Epoch` today.

## 0.1 The supporting surface (order matters: extend Round FIRST)

- [ ] `consensus-types/primitives/round.go` — port the methodical-ssz
      method set from `epoch.go:114-152`: `HashTreeRoot`,
      `HashTreeRootWith`, `MarshalSSZ`, `MarshalSSZTo`, `UnmarshalSSZ`,
      `SizeSSZ` (the generated `.ssz.go` calls these on the field type —
      see `phase0.ssz.go:1972+`'s `c.Epoch.MarshalSSZTo(...)` /
      `HashTreeRootWith(hh)`; without them the regenerated code does not
      compile). This is also what makes spectest SSZ vectors unmarshal
      into the retyped struct. `Epoch` has no `String()`; do not invent
      one.
- [ ] `encoding/bytesutil` — add `RoundToBytesBigEndian`,
      `BytesToRoundBigEndian`, `RoundToBytesLittleEndian` (db keys and
      wire paths encode the retyped field).
- [ ] `encoding/ssz/equality/deep_equal.go` — add a `case "Round"`
      beside the `Epoch` case. Without it `DeepSSZEqual` PANICS (not
      fails) on any struct containing a retyped checkpoint — the
      `TestUpgradeToAltair`/`TestUpgradeToElectra` suites exercise it.
- [ ] `api/apiutil` — `Uint64ToString`'s generic type set gains
      `primitives.Round`.
- [ ] New helpers in `time/slots`: `RoundsSinceGenesis` (the wall-clock
      twin of `EpochsSinceGenesis`, needed by 1.4), `RoundEnd`, and
      `FFGTargetSlot(round) (Slot, error)` — the ONE home of the target
      offset arithmetic: `RoundStart(round) − FFG_TARGET_OFFSET_SLOTS`,
      clamped at zero. It lives here and not in `core/helpers` because
      forkchoice cannot import `core/helpers` without a cycle;
      `helpers.FFGTargetRoot` delegates to it.

## 0.2 The type change — five protos, not one

- [ ] `proto/prysm/v1alpha1/attestation.proto:95` — `Checkpoint.epoch`:
      `cast_type` → `primitives.Round`. Regenerate
      (`make gen proto ssz mode=force`).
- [ ] Four sibling fields are value-carriers of the same checkpoint
      index; leaving any of them `Epoch` forces a forbidden cast at its
      boundary, so they retype in the same change:
      - `p2p_messages.proto` `Status.finalized_epoch` /
        `StatusV2.finalized_epoch` — the handshake carries the finalized
        ROUND (4.2's prose, made compiler-enforced).
      - `slasher.proto` `HighestAttestation.highest_{source,target}_epoch`
        — copied straight from attestation checkpoints.
      - `validator.proto` `DoppelGangerRequest.ValidatorRequest.epoch` —
        the stored attestation target (4.0's doppelganger triple).
      - `beacon_chain.proto`
        `ChainHead.{finalized,justified,previous_justified}_epoch` —
        carries rounds; step 5 owns the reporting decision.
      (There is no `ethpbv1.Checkpoint` use in `beacon-chain/` at all;
      the API boundary is the string-typed structs layer,
      `api/server/structs/conversions.go:574-595`, which converts with
      explicit casts — nothing to retype there.)
- [ ] `beacon-chain/forkchoice/types/types.go:14` —
      `forkchoicetypes.Checkpoint.Epoch` becomes `primitives.Round`, and
      `forkchoice/doubly-linked-tree/types.go:64-67` — the node fields
      `justifiedEpoch`, `finalizedEpoch`, `unrealizedJustifiedEpoch`,
      `unrealizedFinalizedEpoch` become `primitives.Round`. **Type
      changes only — the fields keep their names** (the proto field is
      necessarily still named `epoch`, so name-consistency is
      unattainable; the TYPE carries the safety, and field renames would
      add hundreds of mechanical edits for zero compiler benefit).
      Function renames that ride a signature change
      (`TargetRootForRound`, `ValidateSlotTargetRound`) are kept.
- [ ] The state interface: `state.BeaconState.FinalizedCheckpointEpoch()`
      (`state-native/getters_checkpoint.go:116`) is renamed
      `FinalizedCheckpointRound() primitives.Round` — interface,
      implementation, and all callers — so the compiler forces the
      conversion at every one of the ~12 epoch-typed consumers (2.6).
- [ ] `core/time.CurrentRound` / `PrevRound` (mirroring
      `CurrentEpoch`/`PrevEpoch`, floor at round 0) ship in THIS stack —
      without them every `Target.Epoch == time.CurrentEpoch(st)` site in
      `core/` would need a bare cast. `CanProcessRound` and the
      `ProcessRound` hook stay in stack B.

## 0.3 The compile-error sweep IS the audit

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
- A checkpoint ROOT being resolved to a slot: `slots.FFGTargetSlot` — the
  root names the round's target block, one slot before the round at the
  default offset (4.4 is the canonical case).
- Dormant paths (phase0/pre-Altair processing, slasher, protection db):
  mechanical conversions that preserve today's arithmetic — behavior
  changes there are out of scope (step 6). Prefer keeping their internal
  variables `Round`-typed end to end over sprinkling casts (the slasher's
  whole `Parameters`/chunk arithmetic becomes `Round`; `historyLength`
  seeds from the same 4096).
- Bare cross-unit casts outside the helper families are forbidden; each
  one that seems necessary is a finding to record, not a fix. Epoch-
  denominated CONFIG constants consumed as round counts (the reorg window
  `ReorgMaxEpochsSinceFinalization` at
  `forkchoice/doubly-linked-tree/reorg_late_blocks.go:68,145`, the
  hot-state-db and cache thresholds in `blockchain/receive_block.go`, the
  protection-db pruning cutoff, the slasher history length) are
  identity-safe value reinterpretations: rename the consuming
  symbols (`roundsSinceFinality*`, `pruningRoundCutoff`) and record each
  in the executor note rather than rescaling.
- Off-list sites the sweep will reach (enumerated so nothing surprises):
  `consensus-types/hdiff/state_diff.go` (three checkpoint literals in the
  binary-diff reader), `validator/db/kv/deprecated_attester_protection.go`
  (the legacy ring buffer; add a `farFutureRound` sentinel with
  `FarFutureEpoch`'s value in the right unit).

- [ ] Landing: stack A (see "Commit structure") — the tree does not
      compile with the retype alone until the sweep is complete, so the
      retype change must contain the full sweep, and the sweep executes
      the mechanical halves of 1.4, 2.5, 2.6, 3.2-3.6, and 4.0-4.3 using
      those sections as the per-site conversion guide. Blast radius:
      ~108 non-test `Target/Source.Epoch` references plus ~117
      checkpoint `.Epoch` reads across ~30 packages (~65 packages / ~220
      files once generated code and tests are counted), of which 60-100
      are real compile errors; only ~20-25 need judgment, and each of
      those has its conversion prescribed in the enumerated lists — the
      rest are one-line mechanical conversions. The long pole is the
      verification runs, not the edits. List the touched-package tally in
      the executor note.
- [ ] New files need BUILD.bazel entries — run gazelle on every touched
      package and CHECK the packages whose imports changed without new
      files: the stack-A sweep adds a `core/helpers` import to
      `beacon-chain/sync/initial-sync/service.go`, and a missing bazel
      dep there fails only at the bazel e2e build, not `go build`.
- [ ] Accept: `go build ./...` green; full test suite green with zero
      expectation edits (identity rule — the values are unchanged
      everywhere; only types moved).

---

# Step 1 — the per-round target, state side, forkchoice side, and node lifetime

## 1.1 `FFGTargetRoot` becomes round-keyed

- [ ] `beacon-chain/core/helpers/block.go:100` — `FFGTargetRoot(state,
      epoch)` becomes `FFGTargetRoot(state, round primitives.Round)`:
      `s := slots.FFGTargetSlot(round)` (0.1's helper — thread the
      overflow error; same for every conversion site in this plan), then
      `BlockRootAtSlot`. **The offset is configurable (answered
      question 1):** a new config value `FFG_TARGET_OFFSET_SLOTS`
      (`config/params/config.go`, `spec:"true"` so it is yaml-sweepable
      per run like `AVAILABLE_ATTESTATION_DUE_BPS_HEZE`), default `1`
      (target = the block at `RoundStart(R) - 1`); `0` targets the
      round's own first slot. Underflow at any offset returns the
      anchor/genesis root (the existing `if s > 0` guard generalizes to
      the clamp). Config plumbing rows: `mainnet_config.go`,
      `minimal_config.go` (both `1`), the `loader.go` print list,
      `loader_test.go` assert list, `rpc/eth/config/handlers.go` + test
      (assignment, case, and the spec-count assert 215→216) — the same
      eight-file pattern `SlotsPerRound` used (plan.md step 2). Update
      the doc comment's pseudocode from `compute_start_slot_at_epoch` to
      `compute_start_slot_at_round` with the offset.
- [ ] Offset-0 interaction with vote timing, stated up front: at offset 0
      the target is the round's own first block. The 1/8 target-miss
      occurs ONLY when voting at slot start (the
      `--decoupled-ffg-vote-at-slot-start` flag): such a voter in the
      round's FIRST slot cannot have seen that block yet, so its target
      root resolves to the previous block and the vote misses
      `is_matching_target` — 1/8 of the round's weight at 8-slot rounds,
      so justification still clears (the decision-12 arithmetic, per
      round: 87.5% > 2/3). At STOCK vote timing (1/3 into the slot) the
      voter has already seen the round's first block and there is no
      loss at all. The offset-0 smoke (8.2) therefore pairs
      `FFGTargetOffsetSlots = 0` WITH `WithSlotStartFFGVote()`, and its
      assertions expect exactly the 1/8 miss plus the one-round warmup
      shift (2.2): at offset 0 the boundary's current-round arm no
      longer clears 2/3, so the PREVIOUS round is justified instead —
      justification still appears at slot 24 but names round 1, and
      sustained progression needs the 5-epoch arm to observe.
- [ ] Call sites, all pass the vote's round (`data.Target.Epoch`
      reinterpreted, or `time.CurrentRound/PrevRound`):
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
      assignment becomes a four-arm rule reading the SAME
      `slots.FFGTargetSlot`:
      `parent == nil → self` (anchor) `| slot <= FFGTargetSlot(round)+
      offset boundary, i.e. slot == RoundStart(R) at offset 0 → self |
      same round as parent (slots.RoundAt on both) → inherit | else →
      parent`. The second arm is the offset-0 rule (a block exactly at
      its round's first slot is its own target) and never fires at
      offset 1; the empty-round-start case (a new-round block at a LATER
      slot) falls to the fourth arm — it takes the parent, NOT itself,
      matching the state side's block-root copy-forward. The finalized
      re-anchor (`store.go:274-277`, pruned tree root is its own target)
      is unchanged. Unit test per offset — including the
      empty-round-start-slot case — asserting `TargetRootForRound ==
      FFGTargetRoot` on the same chain; if they diverge,
      `VerifyLmdFfgConsistency` rejects every vote.
- [ ] `forkchoice.go:861` — `targetRootForEpoch(root, epoch)` and its
      public wrapper `TargetRootForEpoch` (`:819`): the parameter is now a
      round value. Rename both to `targetRootForRound` /
      `TargetRootForRound` (and the interface entries
      `forkchoice/interfaces.go:94`, `blockchain/chain_info.go:106,563`,
      `forkchoice/ro.go:223`, `verification/initializer.go:31`,
      `blockchain/testing/mock.go:1016`). Inside, the `nodeEpoch ==
      slots.ToEpoch(targetNode.slot)` back-off at `:883` becomes
      `RoundAt`.
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
      cp.Epoch` → `RoundAt(node.slot)+1`. The plan-next 5.7 boundary
      lessons (strict bound; the child at the boundary's first slot makes
      its parent viable) carry over **at offset 1 only**: that
      child-at-first-slot arm (`:296-302`) encodes offset-1 geometry; at
      offset 0 a child exactly at `RoundStart` is its own target, not
      evidence for the parent — the child arm's evidence bound is
      `firstEvidenceSlot = roundStart` at offset 1, `roundStart+1` at
      offset 0. Enumerate the offset-dependent predicates (this arm, the
      1.2 insert rule, 1.3a's prune child bound) in one place and make
      each read the offset config; unit-test both offsets on each.
- [ ] `forkchoice.go:500` — `finalizedSlot := slots.EpochStart(fc.Epoch)`
      → `RoundStart`.
- [ ] `forkchoice.go:158-175` — `updateCheckpoints` compares checkpoint
      epochs to each other only (round vs round) — no change beyond
      types; say so in the change description. (Step 5.2 adds the
      advance-counter choke point beside it.)

## 1.3 The dependent root is an EPOCH concept — separate it from targets

`dependentRootForEpoch` (`forkchoice.go:831-851`, public
`DependentRootForEpoch:814`) serves duty shuffling, which stays
epoch-keyed, and it is LIVE at Heze (callers: `getRecentPreState`,
`validate_beacon_blocks.go:363`, proposer preferences, the payload bid,
data-column verification, `rpc/core/validator.go:1008`, and — the one
that makes reachability a liveness matter — `Store.insert`'s
proposer-boost shuffle check at `store.go:222-232`, which asks for
`currentEpoch−1` on EVERY block insert). It cannot be dropped.

- [ ] The reimplementation is one line: at offset 1, the last block
      before `EpochStart(E)` IS the target of the epoch's first round, so
      `dependentRootForEpoch(root, E) =
      targetRootForRound(root, slots.RoundAt(EpochStart(E)))`, and the
      existing `if ToEpoch(node.slot) >= epoch → parent` adjustment
      (`forkchoice.go:844-849`) already covers offset 0. No parent walk,
      no extra node pointer.
- [ ] **The MaxUint64 corner is load-bearing.** Callers pass
      `headEpoch−1`, which underflows to `MaxUint64` at epoch 0; the
      epoch-keyed code treats that as "past every node" and returns the
      node's own root. `slots.EpochStart(MaxUint64)` errors — the round
      reimplementation must special-case `epoch == MaxUint64` (fall back
      to `primitives.Round(math.MaxUint64)`) or
      `TestService_GetRecentPreState` regresses.
- [ ] Unit test at 8/32, both offsets: the dependent root for epoch E is
      unchanged by round targets; `TestStore_TargetRootForEpoch`'s
      dependent-root expectations (edited in plan-next 5.7) stay put
      under identity.

## 1.3a Node lifetime: the pruning horizon trails by two epochs

Per-round finality moves the finalized checkpoint every round and
`on_tick`'s `NewSlot` prunes at every round start (1.4's `IsRoundStart`
gate). The finalized round trails head by ~2 rounds, so cutting the tree
at `RoundStart(finalizedRound)` leaves ~3 rounds of history — while 1.3's
epoch-boundary lookup needs nodes up to TWO EPOCHS back (`currentEpoch−1`'s
boundary block lives in epoch `currentEpoch−2` when the boundary slot is
empty). The epoch-keyed world got that retention for free because
finality itself lagged two epochs; the round world must keep it
deliberately.

- [ ] `store.go:300` (`prune`) — introduce `pruneHorizonSlot(finalizedRound)
      = EpochStart(ToEpoch(RoundStart(finalizedRound))) − 2*SlotsPerEpoch`,
      clamped at genesis. The new tree root is the deepest ANCESTOR of
      the finalized node at or below the horizon, so the horizon only
      ever moves along the finalized chain; `s.finalizedDependentRoot`
      (`store.go:318` — saved today from the finalized node's parent) is
      saved from the node just below the new cut. Pruning is a memory
      optimization, not a correctness mechanism — head selection is
      gated by the justified/finalized checkpoints, so nothing below
      finality can win whether or not its node is in the tree; what
      pruning must not do is delete nodes that live lookups still reach.
      Keeping ~2 epochs of extra nodes (≤ 64 slots) is noise.
- [ ] **The offset-aware child bound** rides the same change: `prune`'s
      pass that removes children of the finalized node incompatible with
      the finalized checkpoint compares against the round's FFG target —
      derive the bound from `slots.FFGTargetSlot(finalizedRound)+1`, not
      a raw `RoundStart`, or at offset 0 a child sitting exactly on
      `RoundStart(R)` (its own target, contradicting the finalized root)
      survives finalization and stays a head candidate. This pass runs
      on every prune, even when the horizon leaves the tree uncut; give
      `prune` an explicit up-front `ctx.Err()` check, since the tree
      walk is no longer guaranteed to run.
- [ ] Because the cut no longer lands at the finalized node, restate the
      prune tests against the horizon contract (`TestStore_Prune_*`,
      `TestStore_PruneKeepsTheEpochStartChild` gets a rounds twin with
      the strict bound from plan-next 5.7 note 1), and add the two
      lifetime tests — **both must actually prune**:
      - `TestPrune_LeavesTheEpochDependentRootReachable` (8/32): dense
        chain to slot ~100, finalize round 12, prune, assert the tree
        root moved to the horizon (slot 32) AND
        `dependentRootForEpoch(head, currentEpoch−1)` still resolves to
        the block before `EpochStart(currentEpoch−1)`. With the horizon
        set to `RoundStart(finalizedRound)` instead, this test fails
        with `ErrNilNode` — which is the wedge it exists to prevent.
      - the offset-0 contradicting-child case relative to the new bound.

## 1.3b A failed insert unwinds

`Store.insert` (`store.go:81`) registers the node in `emptyNodeByRoot` /
`fullNodeByRoot` and its parent's children BEFORE its fallible tail (the
`errInvalidParentRoot` arm at `:117`/`:197`, the timing math, and the
proposer-boost dependent-root comparison at `:222-232`). The caller rolls
the BLOCK back out of the database when insert fails — a node left behind
names a block that no longer exists, and head selection lands on that
root and fails `Could not get head block and state: block does not exist`
for the rest of the process's life. Any transient insert error becomes a
permanent wedge.

- [ ] One `unwindInsert` closure (delete from both maps, remove from
      `parent.children`, refresh the payload-node metrics), called on
      EVERY error return after registration; the existing inline unwind
      in the `errInvalidParentRoot` arm folds into it.
- [ ] Unit test: force the proposer-boost dependent-root comparison to
      fail (head pointed at a node outside the tree, block timely), and
      assert a failed `InsertNode` leaves the store exactly as it found
      it (`HasNode` false, map sizes and parent children unchanged).

## 1.4 Mixed-units audit, forkchoice package

- [ ] `goldfish.go:454` and `:466-467` — `s.justifiedCheckpoint.Epoch`
      paired with `slots.ToEpoch(current)`: the wall-clock side becomes
      `slots.RoundAt(current)` (the checkpoint side is already a round).
      The `viableForHead` rule (`node.go:18-25`, `n.justifiedEpoch ==
      justifiedEpoch || n.justifiedEpoch+2 >= currentEpoch`) then reads
      entirely in rounds — the `+2` tolerance becomes two ROUNDS; note in
      the change description that this tightens the wall-clock window on
      the devnet by design (checkpoints advance per round). This is the
      one BEHAVIORAL consequence inside stack A, and it invalidates one
      fixture: `TestGoldfishWalk_ColdStarts`' checkpoint-sync subtest
      pins "epoch 1" against a wall clock at round 4, which is
      self-inconsistent under round viability — restate the fixture as
      what a checkpoint-synced node actually holds. The only expectation
      edit in the stack; every other suite is untouched.
- [ ] The wall-clock argument threaded into viability comes from
      `slots.EpochsSinceGenesis` in BOTH `store.head()` and
      `ForkChoice.updateBestDescendant` — not on any earlier site list;
      convert both to `slots.RoundsSinceGenesis` (0.1's helper) and grep
      every caller of `updateBestDescendantConsensusNode` /
      `viableForHead` / `leadsToViableHead` / `goldfishBestChild` to
      convert the argument at the source.
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
      (the goldfish per-slot hook at `:39-41` is untouched). This is the
      change that makes 1.3a's horizon mandatory, not optional.
- [ ] `reorg_late_blocks.go:68,145` — the late-block reorg window
      (`ReorgMaxEpochsSinceFinalization`, value 2) is consumed against
      the finalized ROUND: two rounds now, a deliberate tightening at
      8/32; record, don't rescale.

## 1.5 Verify

- [ ] Unit ladder: helpers round-target tests; forkchoice insert/target
      tests at 8/32 (a round-start block starts a new target; blocks
      within one round share it); the two 1.3a lifetime tests (pruning);
      the 1.3b unwind test; `VerifyLmdFfgConsistency` accepts a
      round-target vote and rejects an epoch-target one at 8/32.
- [ ] Whole existing forkchoice + blockchain suites green with the single
      1.4 fixture restatement as the only expectation edit. The two known
      pre-existing blockchain failures (`TestStore_NoViableHead_NewPayload`,
      `TestNoViableHead_Reboot`) stay the only ones.
- [ ] `go build ./...`, `go vet` on touched packages, then the stack-A
      gate: `TestEndToEnd_HezeGenesisShort` (8.2's invocation).

---

# Step 2 — the cadence: `ProcessRound`

## 2.1 The cadence predicate

- [ ] `beacon-chain/core/time/slot_epoch.go:126-128` — beside
      `CanProcessEpoch`, add `CanProcessRound(state)`:
      `(state.Slot()+1) % SlotsPerRound == 0`. (`CurrentRound` /
      `PrevRound` already exist — stack A pulled them forward; do not
      re-add.)

## 2.2 A Heze-only pair; `processEpochGloas` stays byte-identical

`beacon-chain/core/transition/gloas.go:137` is the epoch processing Heze
actually runs (`transition.go:337-340` sends `version >= Gloas` there;
Heze sorts above Gloas). Do NOT split `processEpochGloas` itself — it is
live in the 496-case spectest set. Add a new Heze-only pair in a new
`core/transition/heze.go` and a `version >= version.Heze` dispatch arm,
leaving `processEpochGloas` byte-identical.

**Answered question 3 (user, 2026-08-21): what moves to round cadence is
justification and finalization (J&F) — nothing else.** One technical
dependency follows from correctness: per-round J&F needs per-round
PARTICIPATION ROTATION, because the target bits J&F counts must belong to
the round being justified. And to preserve identity, rewards at an epoch
boundary must still read the PRE-rotation arrays (rotation runs late in
epoch processing, after rewards, at `gloas.go:198`). Both constraints are
satisfied by placing rotation per-boundary-kind:

- [ ] `processRoundHeze` (new): `electra.InitializePrecomputeValidators`
      (the shape of `gloas.go:144`), `electra.ProcessEpochParticipation`
      (`:148`) — the balance precompute J&F consumes —,
      `precompute.ProcessJustificationAndFinalizationPreCompute`
      (`:152`), then `electra.ProcessParticipationFlagUpdates` (`:198`)
      **only when `coreTime.CanProcessEpoch` is false** — i.e. only at a
      round boundary that is not also an epoch boundary.
- [ ] `processEpochHeze` (new): everything else from `processEpochGloas`
      in today's exact order — inactivity scores, rewards/penalties,
      registry updates, slashings, eth1-data reset, pending
      deposits/consolidations, builder payments, effective balances,
      randao/historical/sync-committee/proposer-lookahead/PTC window, and
      the rotation at its original position after rewards. It BEGINS with
      its own `InitializePrecomputeValidators` +
      `ProcessEpochParticipation`: the round and epoch parts are separate
      `ProcessSlotsCore` hooks, and the epoch body's first calls
      (`gloas.go:156-174`) consume the precompute outputs. This is
      value-safe — both precompute passes are pure reads, and J&F mutates
      only checkpoints/bits, which the precompute never reads — so
      "byte-identical under identity" survives, at the cost of a doubled
      full-registry scan at epoch boundaries and one extra scan per pure
      round boundary. Noise at sim scale; state the cost in the change
      description rather than optimizing (if it ever matters, thread the
      precompute through the hooks — never re-merge the functions). The
      ~15-call duplication from `processEpochGloas` is deliberate:
      factoring a shared tail would edit Gloas, which spectests forbid.
- [ ] At a coinciding boundary the sequence is therefore: round part
      (precompute, J&F), then the whole epoch body including its own
      precompute and its late rotation — value-identical to today under
      the identity configs. At a pure round boundary: precompute, J&F,
      rotation. Trace both orders in the executor note.
- [ ] Accounting consequences, accepted by charter and question 3, one
      paragraph in the change description: at 8/32, epoch
      rewards/penalties read the last round's participation only; and
      the prev-round quorum balance uses the EPOCH's active set
      (`InitializePrecomputeValidators` gates `IsActivePrevEpoch` by
      `PrevEpoch(state)`), so validators activated at the epoch boundary
      are excluded from the prev-round target balance. Identical under
      identity, harmless on a static-validator devnet.
- [ ] `transition.go:293-308` (`ProcessSlotsCore`) — call the new
      exported `ProcessRound` immediately before `ProcessEpoch`, inside
      the same loop iteration, after `ProcessSlot`. `ProcessRound` is a
      no-op for `state.Version() < version.Heze` or
      `!CanProcessRound(state)` — no pre-Heze behavior moves. The
      `ProcessEpoch` dispatch (`transition.go:336-340`) gains a
      `version >= version.Heze` arm selecting `processEpochHeze`, above
      the existing `>= Gloas` arm.
- [ ] **The genesis stub guard moves to the round clock.** The spec's
      early return in `process_justification_and_finalization` exists to
      protect the genesis checkpoint stub: the genesis state cannot
      contain the genesis block's root (the block embeds the state's
      hash), so the initial justified/finalized checkpoints carry
      `(unit 0, root 0x00)` and every early attestation cites that stub
      as its exact-match FFG source. A J&F pass justifies the PREVIOUS
      unit as well as the current one, so the first processed boundary
      must be the first whose previous-unit arm cannot reach unit 0 —
      unit 2. The constant 2 is `1 (stub unit) + 1 (the look-back arm)`
      and cannot shrink; what CAN change is the clock it counts on. Move
      both guards from the epoch clock to the round clock:
      `precompute/justification_finalization.go:58-64`
      (`slots.EpochStart(2)` → `slots.RoundStart(2)`) and
      `UnrealizedCheckpoints`'s twin (`:19-24`, `ToEpoch(slot) <=
      GenesisEpoch+1` → `RoundAt(slot) <= 1`). **Identity-safe by
      construction**: on every shipped and spectest config rounds equal
      epochs, so both expressions are bit-identical; only 8/32 behavior
      moves. The 8/32 timeline this pins: round processing runs at the
      LAST slot of a round, so the first processed boundary is 2→3 —
      **round 2 justified at slot 24, round 2 finalized at slot 32**
      (warmup 2 rounds instead of 8). One 8/32 unit test pins exactly
      that timeline; the step-5 evaluators and any chain-check
      assertions use it as their warmup constant.

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
      moved. The 4-bit window now spans 4 rounds. (The mechanical half of
      this lands inside stack A's sweep; stack B verifies and owns the
      cadence that invokes it.)
- [ ] Precompute tests
      (`TestProcessJustificationAndFinalizationPreCompute_*`) stay put
      under identity; add the 8/32 progression test: with full
      participation, round R justifies at the R→R+1 boundary and
      finalizes at R+1→R+2 — finality latency 2 rounds = 16 slots — with
      the warmup rounds pinned per 2.2 (rounds 0-1 skipped; round 2
      justified at slot 24, finalized at slot 32).

## 2.4 The state's participation arrays rotate per round

No proto change: `previous/current_epoch_participation`,
`justification_bits`, and the three checkpoints keep their SSZ shape
(`beacon_state.proto:71-78`, `:226-229`; state-native accessors
`getters_checkpoint.go`, `setters_checkpoint.go`,
`getters_participation.go:69`) — reinterpretation only.
`ProcessParticipationFlagUpdates` (`core/altair/epoch_spec.go:54`,
aliased at `core/electra/transition.go:29`) is called from the round
part; its body is unchanged.

- [ ] Verify no SSZ/state-native edit is needed by building and running
      the state-native suite — checkable because the fields' types and
      counts are untouched (`BeaconStateHezeFieldCount` stays 46,
      `mainnet_config.go:235`).

## 2.5 Finality delay and the inactivity leak — stays epoch-based

**Answered question 4: the leak stays epoch-based; simplest change.** The
only edit is the one the step-0 retype forces: `FinalityDelay(prevEpoch,
finalizedEpoch)` (`core/helpers/rewards_penalties.go:190-192`) now
receives a round-valued finalized checkpoint, so its callers convert that
one input — `FinalityDelay(prevEpoch,
helpers.CheckpointEpoch(state.FinalizedCheckpointRound()))` — and
everything else (`IsInInactivityLeak` `:180-182`,
`MinEpochsToInactivityPenalty`, the epoch settlement cadence) is
untouched. With per-round finality the finalized epoch advances at least
as fast as before, so the leak arms no earlier than today.

- [ ] Convert at the callers: `core/altair/epoch_precompute.go:104,269`
      (the electra/Heze path — inactivity scores and
      `AttestationsDelta`). The phase0 path
      (`core/epoch/precompute/reward_penalty.go:73`) is pre-Altair and
      unreachable at Heze — mechanical retype fixes only, say why.
- [ ] One unit test: delay 0 while finality advances per round; delay
      counts epochs when it stalls.

## 2.6 Mixed-units audit, state-internal checkpoint readers

The state's checkpoints now hold rounds, but several state-transition
consumers feed them into EPOCH-typed logic. Add ONE helper next to
`FFGTargetRoot`, named so the unit is visible:
`helpers.CheckpointEpoch(round) (Epoch, error)` :=
`slots.ToEpoch(slots.RoundStart(round))`. The
`FinalizedCheckpointRound()` rename (0.2) makes the compiler find every
consumer. The full conversion table:

| site | conversion |
|---|---|
| `core/helpers/validators.go` `IsEligibleForActivation` (+`UsingROVal`) | `helpers.CheckpointEpoch(state.FinalizedCheckpointRound())` — without it activations unlock ~4x early at 8/32 (finalized ROUND 6 read as epoch 6) |
| `core/helpers/weak_subjectivity.go:161` `LatestWeakSubjectivityEpoch` | same |
| `core/altair/epoch_precompute.go:104,269` | same (2.5's leak inputs) |
| `core/epoch/precompute/reward_penalty.go:73` | same; pre-Altair, unreachable |
| `core/electra/deposits.go:280` pending-deposit gate | `slots.EpochStart` → `slots.RoundStart` |
| `state-native/getters_gloas.go:135` `IsActiveBuilder` | state-internal twin (below) |
| `state-native/state_trie.go:1492` builder metrics | same |
| `rpc/prysm/v1alpha1/beacon/validators.go:464` activation queue | `helpers.CheckpointEpoch` |
| `rpc/core/validator.go:834` participation `Finalized` flag | `helpers.CheckpointEpoch` |
| `rpc/core/validator.go:546-551` source freshness | `coreTime.CurrentRound(headState) < targetRound` — or the source a validator signs goes one round stale at every round boundary and its vote fails the source match (a per-round-liveness bug, not accounting) |

- [ ] `state-native` sits below `core/helpers` in the import graph and
      cannot call the helper: it gets an unexported 4-line twin
      `checkpointEpoch(*ethpb.Checkpoint)` in `getters_checkpoint.go`
      with a comment naming the relationship — the ONE sanctioned
      duplicate of the helper.
- [ ] 8/32 unit tests: the activation gate (finalized ROUND 6 unlocks
      epoch 1, not epoch 6) and the source freshness.

## 2.7 Verify

- [ ] Unit ladder (2.2 guard timeline, 2.3, 2.5, 2.6 tests) plus the full
      `core/...` suites green with zero expectation edits under identity.
- [ ] 3-slot smoke at 8/32 if a runnable single-node harness exists;
      otherwise `TestProcessRound`-style unit coverage of the observable
      (round boundaries process without epoch processing firing
      mid-epoch) plus the stack-A Short e2e stand as the tier.

---

# Step 3 — attestation plumbing

## 3.1 Data construction (server)

- [ ] `rpc/core/validator.go:536-537` — `targetEpoch :=
      slots.ToEpoch(req.Slot)` → `targetRound := slots.RoundAt(req.Slot)`;
      `TargetRootForRound(headRoot, targetRound)`. The returned
      `AttestationData.Target.Epoch` field carries the round; `Source` is
      the state's justified checkpoint (already round-valued after
      step 2). The forkchoice-checkpoint cache writes (`:565-578`) carry
      rounds in their `Epoch` fields — values only.
- [ ] The freshness branch `:546-551` is 2.6's item — cross-reference,
      don't do it twice.

## 3.2 The one predicate every path calls

- [ ] `core/helpers/attestation.go:44-49` — `ValidateSlotTargetEpoch`:
      `slots.ToEpoch(data.Slot) != data.Target.Epoch` becomes
      `slots.RoundAt(data.Slot) != data.Target.Epoch` (same-unit after
      the retype). Rename to `ValidateSlotTargetRound`; callers:
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
      round-comparison (keep) or epoch-derivation (switch to slot).
      Expect ~93 non-test hits post-sweep, every one a round comparison
      or round value carrier; put the classified list in the executor
      note.
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
      the round to `FFGTargetRoot` (1.1's caller list). The timely-source
      delay bound `SqrRootSlotsPerEpoch` (`:324-327`) stays epoch-derived
      — accounting fidelity is secondary and the window still fits inside
      a round at 8/32 (√32=5 < 8). Deliberately unchanged, why noted.
- [ ] `core/altair/attestation.go:107` — the target-epoch value threaded
      into `SetParticipationAndRewardProposer` is the round; rename the
      parameter.
- [ ] `state-native/setters_gloas.go:489` — `data.Target.Epoch ==
      slots.ToEpoch(b.slot)` selects the participation array and which
      builder pending payment is charged (state-transition path; same
      class as this section): the wall-clock side becomes `RoundAt`.

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
      `RoundStart` (`:114`), `:61,148` same conversion. **Plus
      `getRecentPreState`, mixed units INSIDE one function:** `:24-25`
      (`c.Epoch+1 < headEpoch`) and `:48` (`c.Epoch <= headEpoch`) become
      round-vs-round against `RoundAt(head slot)`, while the `:35-39`
      shuffle-compat check via `DependentRootForEpoch(..., headEpoch-1)`
      STAYS epoch-keyed (it is about shuffling) — its comment's reasoning
      "headEpoch−1 equals c.Epoch if c is from the previous epoch" is
      false at 8/32, so the compat condition must be restated in terms of
      the checkpoint round's epoch (`helpers.CheckpointEpoch(c.Epoch)`),
      not the raw field; `verifyAttTargetEpoch:168-180` — target must be
      the current or previous ROUND of the wall clock
      (`slots.RoundAt(currentSlot)`). The checkpoint-state cache
      (`checkpointStateCache.StateByCheckpoint`, `:105`) is keyed by the
      checkpoint value — rounds key it just as well; state it as the
      no-change why.
- [ ] `blockchain/process_attestation.go:104` — `attData.Target.Epoch >=
      params.BeaconConfig().GloasForkEpoch` mixes a round with a config
      epoch. Rewrite as a same-unit comparison (gate on the attestation
      slot's epoch, or convert the config epoch through
      `RoundAt(EpochStart(...))`). Identity-safe either way at fork
      epoch 0.
- [ ] `sync/validate_beacon_attestation.go:498-514` — the debug helper's
      `sourceEpoch`/`targetEpoch` fields: relabel `sourceRound`/
      `targetRound` (step 5's logging sweep owns the pattern; this is the
      one inside the gossip path).
- [ ] `sync/validate_beacon_attestation.go:618-627`
      (`validateAvailableAttWithBlock`) — the goldfish vote's checkpoint
      state is resolved via `epoch := slots.ToEpoch(att.Data.Slot)` +
      `TargetRootForEpoch`: becomes `RoundAt` + `TargetRootForRound`, and
      the synthetic `Checkpoint{Epoch: ...}` carries the round. (This
      function's queue-admission predicate is step 7.3's item — the
      unit conversion here, the delivery semantics there.)
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
      helpers' Epoch suffixes — including the package's test fixtures,
      which otherwise pin the old names and units.
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

---

# Step 4 — every consumer of `checkpoint.Epoch → slot` and `checkpoint.Root → slot`

The service layer converts checkpoint epochs to slots in ~27 non-test
sites; each becomes `slots.RoundStart(...)`. A smaller, more dangerous
class converts a checkpoint ROOT to a slot — those take
`slots.FFGTargetSlot` (4.4). Grouped, with the full grep (`EpochStart(`
over `beacon-chain/`, filter checkpoint-typed arguments) re-run at
execution time:

## 4.0 The traps — each carries a conversion-choice mistake to avoid

- [ ] `db/kv/wss.go:104-118` — checkpoint-sync origin **computes** the
      checkpoint value as `blk.Slot() / SlotsPerEpoch` and saves it as
      the justified+finalized checkpoint. Must become `slots.RoundAt` —
      a careless `Round(...)` cast here preserves epoch-division
      arithmetic and every checkpoint-synced node starts on a wrong
      round.
- [ ] `state-native/getters_gloas.go:135` — `IsActiveBuilder`:
      `DepositEpoch < finalizedEpoch` — checkpoint side converts via the
      state-native twin (2.6).
- [ ] `core/electra/deposits.go:280` —
      `EpochStart(FinalizedCheckpoint().Epoch)` gating pending deposits
      → `RoundStart`.
- [ ] `blockchain/setup_forkchoice.go:76,121` (startup head-vs-finalized
      guard), `blockchain/chain_info.go:511,521,666`,
      `blockchain/receive_block.go:380` → `RoundStart` conversions.
- [ ] `cache/checkpoint_state.go:87-98` — `EvictUpTo(finalized.Epoch)`:
      with rounds the eviction bound fires ~4x as often at 8/32; the
      cache keys are rounds too (after 3.5's multilock/key change) —
      convert bound and keys together and say so.
- [ ] `execution/log_processing.go:349` → conversion.
- [ ] `verification/execution_payload_envelope.go:103-115`
      (`VerifySlotAboveFinalized` — the twin of the blob/data-column
      sites 4.2 lists), `sync/pending_payload_envelope.go:113`,
      `sync/pending_blocks_queue.go:608` → `RoundStart`.
- [ ] Initial sync slot math:
      `sync/initial-sync/blocks_fetcher_utils.go:163-166,336-337,371`
      and `p2p/peers/status.go:761` — `BestFinalized`/`BestNonFinalized`
      results are converted to slots via `SlotsPerEpoch.Mul(...)`; the
      finalized value is now a round, so its conversion goes through
      `slots.RoundStart`; the non-finalized (head-epoch) side stays
      epoch-based. The two feed
      `calculateHeadAndTargetEpochs` (`blocks_fetcher_utils.go:39,369`),
      which returns finalized ROUNDS in `modeStopOnFinalizedEpoch` and
      head EPOCHS otherwise — a latent unit bug the retype exposes.
      Rename it `calculateHeadAndTargetBounds` and return **slots** (the
      first slot past the head/target unit), comparable in both modes;
      adapt its two consumers and test.
- [ ] `sync/initial-sync/blocks_fetcher_utils.go:151` — `findFork` pairs
      a round-keyed finality guard with an epoch-keyed `EpochStart`
      rewind; the rewind becomes `RoundStart` so it cannot land 16+
      slots below the finalized round the guard just cleared.
- [ ] The doppelganger triple: `validator/client/validator.go:501`
      (sends the stored attestation target, now a `Round` after 0.2),
      `rpc/prysm/v1alpha1/validator/status.go:369` (the `+2 <
      headEpoch` recency gate), `validator/client/beacon-api/
      doppelganger.go:56-117` (REST twin). **The gate and the evidence
      must use the SAME unit**: the evidence (participation arrays,
      liveness for `currentEpoch`/`currentEpoch−1`) is epoch machinery,
      so the checkpoint side of the gate converts via
      `helpers.CheckpointEpoch` and the comparison stays in epochs. A
      gate in rounds (2 rounds = 16 slots) against epoch-wide evidence
      flags a validator's own 3-round-old attestation as a duplicate and
      refuses to start it — a false positive no e2e catches (the e2e
      asserts a duplicate IS found). 8/32 unit test on both gRPC and
      REST paths: a validator with a 3-round-old attestation is NOT its
      own doppelganger.
- [ ] Weak subjectivity — producer and consumer must agree on the unit:
      `core/helpers/weak_subjectivity.go:161` converts via 2.6;
      `rpc/prysm/beacon/handlers.go:43-47` (`GetWeakSubjectivity`, the
      prysmctl source) must EMIT the checkpoint's round, since
      `ParseWeakSubjectivityInputString` interprets the flag's value as
      a round feeding `RoundStart`. An epoch-valued emission makes the
      operator-pasted `--weak-subjectivity-checkpoint` search a
      4×-wrong window → `errWSBlockNotFoundInEpoch` → node exits.
      Unit test: the emitted string round-trips through the parser to
      the same block.

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
      N is now a ROUND — document in the flag's usage string (and see
      4.0's producer item).

## 4.2 sync, db, rpc

- [ ] `sync/service.go:571`; `sync/validate_beacon_blocks.go:155`;
      `sync/custody.go:39,53`;
      `sync/rpc_execution_payload_envelopes_by_root.go:62`;
      `sync/initial-sync/round_robin.go:587`;
      `verification/blob.go:147`, `verification/data_column.go:268`.
- [ ] `sync/rpc_status.go` — the handshake. `FinalizedEpoch` on the wire
      now carries the finalized ROUND, compiler-enforced by 0.2's
      p2p_messages retype (all peers run this software; the mock's
      network is closed). Convert `:437-452`'s `maxFinalizedEpoch =
      maxEpoch - 2` guard to rounds against the wall-clock round —
      NOTE the direction: the guard REJECTS on `msg.FinalizedEpoch >
      maxFinalizedRound`, so `maxRound-2` is strictly TIGHTER than the
      epoch version at 8/32 (4× more boundaries inside the same wall
      clock); that is acceptable — do not "loosen for slack" without a
      demonstrated need, record the analysis instead. The `xrmqllqx`
      strict child bound (`:487-497`): `startSlot` comes from
      `RoundStart(finalized round)`, keeping the strict `>` (the
      checkpoint block sits one slot before the ROUND boundary now). The
      `IsFinalized` root check (`:459`) is exercised by 4.4's stater
      rule — a wrong-slot origin fails it against every honest peer.
      `peers/status.go:721-750` best-finalized voting: the SORT compares
      peer values to each other and stays correct as rounds; the
      downstream epoch→slot conversions are 4.0's initial-sync item.
- [ ] `db/kv/state.go:1041`; `db/kv/checkpoint.go:19-63`
      (save/load are value-carriers — no change, why: they never
      convert); `db/kv/finalized_block_roots.go:53-62,164` — the
      backfill uses an epoch-RANGE filter
      (`SetStartEpoch(prevFinalized).SetEndEpoch(cp+1)`), not
      `EpochStart`: rewrite both de-index and upsert ranges as explicit
      slot ranges from `slots.RoundStart`; the index they feed is
      root-keyed (verify and record).
- [ ] `rpc/core/beacon.go:92-106` — ChainHead fSlot/jSlot/pjSlot via
      `RoundStart`; the proto fields named `*Epoch` carry rounds
      (retyped in 0.2; step 5 owns the reporting decision).
- [ ] `rpc/eth/helpers/sync.go:83` — `RoundStart`.
- [ ] `sync/checkpoint/weak-subjectivity.go:30,47,106` — checkpoint-sync
      origin WS epoch: converts via `RoundStart`; the user-facing
      `--checkpoint-sync-url`/WS inputs now speak rounds — document.
- [ ] Engine API: finalized/justified are sent to the EL as block HASHES
      resolved from roots, never epochs — verify by reading the
      forkchoiceUpdated call construction and record the no-change why.

## 4.3 Checkpoint sync and the cold-start pair

- [ ] Fresh start, restart, checkpoint sync (symmetric triple). The
      origin checkpoint is NOT a pass-through: `db/kv/wss.go:104-118`
      COMPUTES it by epoch division — 4.0's first item converts it to
      `slots.RoundAt`. Downstream of that the getters/setters are
      value-carriers. The e2e checkpoint-sync variant
      (`TestEndToEnd_HezeGenesisCheckpointSync`,
      `testing/endtoend/heze_e2e_test.go:123`) is the integration
      witness — it must pass at 8/32 with per-round finality, proving the
      anchor round threads through Status, initial sync, and forkchoice
      bootstrapping. It is also the witness for 4.4.

## 4.4 The stater rule: a checkpoint's ROOT names its round's TARGET block

A round-R checkpoint's root is the block at `RoundStart(R) − offset`
(1.1) — NOT the block at `RoundStart(R)`. Every resolution of a
checkpoint root to a slot must therefore go through
`slots.FFGTargetSlot`, or the resolved state is one block off and the
mismatch surfaces two subsystems away.

- [ ] `rpc/lookup/stater.go:143,157` — the `"finalized"` / `"justified"`
      state-id resolutions replay to `slots.FFGTargetSlot(cp.Epoch)`,
      not `RoundStart`. This is the state checkpoint-sync clients
      download: resolve it at `RoundStart` and the served state's
      `latest_block_header` names the round's FIRST block, so the
      syncing node saves a different block than the checkpoint root,
      then fails `IsFinalized` (`sync/rpc_status.go:459`) against every
      honest peer — "invalid finalized root", both directions,
      `suitable=0` peers, initial sync never starts. The failure
      presents as a peering problem; the cause is one slot of replay.
      (`StateRoot()`'s finalized path is already correct — it looks the
      block up by `checkpoint.Root`, not by slot; say so.)
- [ ] Not version-gated — `FFGTargetRoot` isn't either; the shifted
      target applies chain-wide on this codebase.
- [ ] Test pattern: mock the replayer at the FFG target slot ONLY, so a
      regression to the round's first slot fails loudly instead of
      silently returning the wrong state (a mock that answers every slot
      pins nothing).
- [ ] Witness: `TestEndToEnd_HezeGenesisCheckpointSync` — the joining
      node's own log shows the origin at `RoundStart−1` and zero
      `invalid finalized root` lines.

## 4.5 Verify

- [ ] `go build ./...`; targeted suites (`blockchain`, `sync`, `db/kv`,
      `rpc/...`) zero expectation edits under identity.
- [ ] `TestEndToEnd_HezeGenesisShort`, then the checkpoint-sync variant.

---

# Step 5 — logging, metrics, and reporting (user emphasis, 2026-08-21)

The rule: a number that is a round must not be labeled "epoch" anywhere a
human or a dashboard reads it. Sweep `grep -rn "justifiedEpoch\|
finalizedEpoch\|targetEpoch\|sourceEpoch"` over log fields after the code
moves.

## 5.1 Log fields — one grep-driven checklist

- [ ] Run the grep over the finished stack-A tree; for every hit, relabel
      (`*Epoch` → `*Round`) or convert, and list the full set in the
      executor note. Expect ~13 relabel sites and ~14 checked-and-left
      epoch-keyed sites. Worked examples: `blockchain/log_helpers.go:115,
      125-127` — `finalizedEpoch`/`justifiedEpoch` keys become
      `finalizedRound`/`justifiedRound`, plus a derived `finalizedSlot`
      (`RoundStart(round)`) on the finalized line so runs stay comparable
      to old logs; `validator/client/attest.go:188-189,336-338` —
      `sourceEpoch`/`targetEpoch` → `sourceRound`/`targetRound` (and the
      two tracing attributes beside them);
      `beacon-chain/monitor/process_block.go:133,137` — attester-slashing
      `targetEpoch1/2` keys carry rounds; the slasher/slasherkv log keys
      and the forkchecker tool follow the same pattern. Expectation
      edits: label-string asserts and error strings only — enumerate
      them (expect ~7), zero value changes.

## 5.2 Prometheus metrics

- [ ] `blockchain/metrics.go:34-66,272,357-365` — the
      `beacon_finalized_epoch` / `beacon_current_justified_epoch` /
      `beacon_previous_justified_epoch` / `head_finalized_epoch` gauges:
      keep the names and emit the EPOCH of the checkpoint's round-start
      slot via one `reportCheckpointRound` helper
      (`helpers.CheckpointEpoch`), so every existing dashboard, the
      kurtosis scrape tooling, and the e2e logs stay meaningful. Add four
      new gauges carrying the raw rounds: `beacon_finalized_round`,
      `beacon_current_justified_round`,
      `beacon_previous_justified_round`, `head_finalized_round`.
      (Answered question 6: build exactly this.)
- [ ] `finality_latency_slots` (gauge): `currentSlot −
      RoundStart(finalizedRound)`. Site: **`reportSlotMetrics`**
      (`blockchain/metrics.go:267`), which has the wall clock in hand —
      set per slot so a stall makes it GROW; updating only on
      finalized-advance freezes it at 16 during a stall, and
      `ProcessRound` has no wall clock and also runs during replay.
      Expected shape at 8/32: a 16→23 sawtooth, period 8.
- [ ] `justified_round_advance_total` / `finalized_round_advance_total`
      (counters): incremented **at the single point where the store's
      checkpoint VALUE is replaced** — add
      `Store.advanceJustifiedCheckpoint` / `advanceFinalizedCheckpoint`
      in the forkchoice package and funnel BOTH writers through them:
      `updateCheckpoints` (`forkchoice.go:158-175`, the block path) AND
      `updateUnrealizedCheckpoints` (the tick path,
      `on_tick.go` → `unrealized_justification.go`). Fork choice
      realizes a round's advance on the TICK, so counters incremented on
      the block path (e.g. beside
      `updateJustificationOnBlock`/`updateFinalizationOnBlock`,
      `receive_block.go:587,603`) compare pre/post values that have
      ALREADY moved and never fire — a counter that reads 0 forever on a
      finalizing chain. Unit test: advance checkpoints via the tick path
      at 8/32 and assert both counters moved (reverting to the block
      path shows 0).
- [ ] `reportEpochMetrics` (`blockchain/metrics.go:278`, invoked at
      `receive_block.go:214`) — reads participation, which now reflects
      the last ROUND's arrays: move it to round cadence (rename
      `reportRoundMetrics`, trigger `CurrentRound > prevRound`) AND
      rename the four participation gauges
      `beacon_prev_epoch_{active,source,target,head}_gwei` →
      `beacon_prev_round_*_gwei` — after 2.4's rotation they measure one
      round's votes; an epoch label would lie 4× per epoch. **Propagate
      the rename into every scrape config and summary tool that reads
      them** (the kurtosis tooling, 8.4) in the same change.
- [ ] New-gauge unit test at 8/32
      (`TestReportSlotMetrics_RoundGaugesAt8Over32`-shape) pinning gauge
      units where identity configs can't.

## 5.3 APIs and the e2e evaluators

- [ ] ChainHead (`rpc/core/beacon.go`) carries rounds in its retyped
      fields; one comment at the construction site names the unit. The
      REST twin must MATCH: `validator/client/beacon-api/
      beacon_api_beacon_chain_client.go` builds the same ChainHead from
      string responses — parse the checkpoint values as ROUNDS (a
      `parseRound` helper) and derive the `*Slot` fields with
      `slots.RoundStart`, the symmetric twin of `rpc/core/beacon.go` —
      an `EpochStart` there makes every derived slot 4× too large at
      8/32.
- [ ] **Verifying that justification/finalization is PROGRESSING is a
      stated requirement (user, 2026-08-21), not a nice-to-have.**
      Asserted at every ladder tier:
      - Unit (2.3): full participation justifies round R at the R→R+1
        boundary and finalizes at R+1→R+2, warmup per 2.2.
      - Smoke: the chain-check script reads the metrics dump and asserts
        the justified round advanced at every round boundary after
        warmup and the finalized round trails by exactly one — a stalled
        round is a failure, not a note.
      - e2e: two evaluators in `testing/endtoend/evaluators/`
        (`finality_rounds.go`). `FinalizationOccursInRounds(epoch)` —
        the rounds twin of `finalizationOccurs` (`finality.go:18-61`):
        finalized round == wall-clock round − 2 and justified ==
        wall-clock round − 1 in steady state, wall-clock round computed
        from the head slot; wire at `AfterNthEpoch(3)` (valid from epoch
        1 after the 2.2 guard, but 3 keeps a margin — lowering it is a
        follow-up, not this plan). And `JustificationAdvancesEveryRound`
        — run per epoch like `AttestationsInEveryRound`
        (`evaluators/rounds.go:40`), asserting the justified round
        increased in each of the epoch's rounds (no gaps), so a single
        skipped round is caught even when the endpoint check would still
        pass. Swap them into the Heze e2e runs (`heze_e2e_test.go:58,
        123,157`) via `hezeDroppedEvaluators` (the stock epoch-keyed
        `FinalizationOccurs` joins the dropped list); non-Heze suites
        keep the original.
- [ ] `ValidatorsParticipating*` evaluators read participation via the
      API — per-round arrays still show ~full participation each epoch
      query; verify once at 8/32 and record (no change expected).

## 5.4 Verify

- [ ] Scripted grep over the tree confirming no remaining Epoch-labeled
      round values in log fields or tracing attributes (full output to a
      file).
- [ ] Touched suites green; label-string edits enumerated; evaluator code
      compiles (the full e2e runs in step 8).

---

# Step 6 — slashing: out of scope, zero-change path (user, 2026-08-21)

Slashing — the consensus predicates, surround detection, the slasher, and
EIP-3076 local protection — is explicitly a non-goal. Take the easiest
path: **no behavioral change.**

- [ ] `slashings.IsSurround` (`proto/prysm/v1alpha1/slashings/
      surround_votes.go:13-15`), `IsSlashableAttestationData`
      (`core/blocks/attester_slashing.go:191-202`), and the whole
      `beacon-chain/slasher/` package: unit-agnostic value comparisons
      that keep compiling and running on round-valued inputs. The step-0
      retype makes the slasher's internal `Parameters`/chunk arithmetic
      `Round`-typed end to end (preferred over sprinkled casts);
      `historyLength` seeds from the same 4096 and now counts rounds —
      whatever the windows detect or miss is not measured.
- [ ] `validator/client/attest.go:125-141` — the first-round-of-epoch
      protection gate: **left exactly as is** (zero change). The
      protection db stores monotonically growing values either way. (A
      possible later cleanup — per-round targets would let the gate be
      removed — is noted here and deliberately not done.)
- [ ] `validator/db/kv` — mechanical retype only: the pruning cutoff
      renames (`pruningRoundCutoff`), the legacy ring buffer's
      `farFutureRound` sentinel; value-preserving conversions per the
      0.3 rules; zero behavior change.
- [ ] One line for the remote-signer path:
      `validator/keymanager/remote-web3signer/types/custom_mappers.go:133`
      now sends round values to remote signers whose own EIP-3076
      protection keys on them — consistent (monotonic values), no change.
- [ ] Verify: `validator/client` and `slasher` suites compile and pass
      with zero expectation edits (identity rule).

---

# Step 7 — goldfish vote delivery and instrumentation (stack D, BEFORE measurement)

The availability-vote stream has a shape nothing else in the client has:
one unsharded topic, every validator publishing the instant the block
reaches its node, so a whole network's votes (one per seat-holder,
`decoupled.AvailableAttestationSeats` — 512 seats over the genesis set,
`decoupled/available_attestation_committee.go:40`; every validator holds
seats every slot at sim validator counts) land in a burst a few
milliseconds wide. Four delivery mechanisms must be built for that
burst, and the stream instrumented so nothing can vanish silently. The
acceptance bar for any clean simulation: **seat fraction exactly 1.00 on
every window sample of every node, with every expected seat reconciled**
— anything less is a code bug and loops fix→run until clean. The
zero-late-tolerance acceptance rule is untouched throughout: on a clean
sim no vote is ever late (block propagation is sub-second against
6-12 s slots); votes that go missing were LOST, and each loss below has
a mechanism and a fix.

## 7.1 The subscription buffer

- [ ] libp2p's `Subscribe` hands messages to the application over a
      32-message channel; `notifySubs` does a NON-BLOCKING send and
      silently drops on overflow, reporting only the RawTracer
      `UndeliverableMessage` event (it cannot block: it runs in the one
      `processLoop` goroutine serving every topic). A 132-vote
      synchronized burst against a 32-slot buffer loses ~2.4% of all
      votes before validation ever sees them — no ignore counted, no
      late vote, invisible everywhere except the tracer. Size the
      head-vote subscription for the burst: a `subscriptionOpts(topic)`
      hook at `sync/subscriber.go:436-451` (`subscribeWithBase` →
      `SubscribeToTopic(topic, opts...)`) passing
      `pubsub.WithBufferSize(4 * decoupled.AvailableAttestationCommitteeSize)`
      for the available-attestation topic only — several slots of the
      topic's whole traffic. Other topics keep the default: none of them
      concentrates a network's messages into one instant (blocks are
      1/slot, FFG attestations are subnet-sharded and spread over the
      slot).
- [ ] Scrape `p2p_pubsub_undeliverable_total` per topic (8.4) so any
      future overflow anywhere is named, not silent.

## 7.2 The pending-queue key lifecycle

- [ ] `sync/pending_attestations_queue.go:45`
      (`processPendingAttsForBlock`) must claim-and-delete the block's
      queue key BEFORE processing the batch (or process under a key
      swap), not delete after — deleting after discards every vote that
      arrives while the batch is being processed, which during the
      burst is most of a committee. Deterministic unit test: a vote
      queued mid-drain survives.

## 7.3 Queue admission and wake-up use the SAME predicate

- [ ] The wake-up that drains the queue fires on block IMPORT (forkchoice
      insert). The admission check at
      `sync/validate_beacon_attestation.go:590-594` must therefore gate
      on `s.cfg.chain.InForkchoice(blockRoot)` — NOT on
      `hasBlockAndState` (`:489-494`), which also requires the state
      summary's DB write, an operation that completes AFTER forkchoice
      membership. With mismatched predicates a vote checking in the gap
      is queued after its own wake-up already ran, and nothing wakes a
      queue twice: one node in one slot strands a whole batch (the
      ledger shape: N accepted + M queued-and-never-replayed = the full
      committee). The validation-path gate for proceeding (`:126,133`)
      keeps its own semantics; it is the QUEUE gate that must mirror the
      wake-up condition. Unit test: a vote arriving between forkchoice
      insert and state-summary write is replayed.

## 7.4 The drain fires on every import path

- [ ] `processPendingAttsForBlock` is called from the gossip block
      subscriber (`sync/subscriber_beacon_blocks.go:84`) and the pending
      blocks queue (`sync/pending_blocks_queue.go:195`) — but a
      proposer's OWN block is imported through the RPC propose path and
      traverses neither. On its own slots a proposer strands the entire
      peer committee's queued votes (they name a block the proposer
      "already has" but never gossip-received). Hook the drain on block
      IMPORT (the blockchain processing path every import shares), not
      on the gossip subscriber. Unit test: votes queued for a block that
      arrives via the propose path are replayed.

## 7.5 Instrumentation (the instrument-first rule made concrete)

- [ ] `goldfish_vote_drop_total{reason}` — a labeled counter on EVERY
      path that discards a head vote (validation ignores included; the
      upstream convention of silent IGNORE is indefensible here). A
      drop with no label is a build error in review.
- [ ] The per-vote ledger, behind a `--goldfish-vote-ledger` feature
      flag (`config/features`): one structured log line per vote —
      slot, validator index, seats, block root, arrival and decision ms
      into the vote's own slot — on every outcome
      (accepted/replayed/queued/dropped(reason)/local). ~132 lines per
      slot per node; sims run with it ON.
- [ ] `kurtosis/votetally.py` — reconciles the committee schedule
      (expected side computed from `decoupled.AvailableAttestationSeats`
      arithmetic, cross-checked against the Go implementation, NOT from
      the logs) against the ledger, per node per slot: expected =
      accepted + dropped(reason) + never_arrived, all three categories
      listed for any slot below 1.00. `goldfish_late_vote_total` and
      the undeliverable counter corroborate. The run summary includes
      the reconciliation table; "never arrived" on a clean localhost
      net is itself a finding.
- [ ] The summarizer reads seat fraction over the measurement WINDOW
      (post-warmup), never over every sample — genesis-adjacent slots
      would otherwise print a spurious headline minimum — and lists any
      short slots individually.
- [ ] Unit ladder + the acceptance loop: after 7.1-7.4 land, one short
      kurtosis run (8.4 recipe) must read seat fraction exactly 1.00
      with zero unaccounted seats and both advance counters live before
      the measurement runs begin.

---

# Step 8 — spectest survey, e2e, and measurement

## 8.1 Spectest survey extension (survey-only; fix nothing)

- [ ] Re-run the `SURVEY-2026-08-21.md` sweep
      (`testing/spectest/SURVEY-2026-08-21.md:326` has the command) on
      the finished stack. Expected result under the identity rule: the
      SAME 486 target-shift failures (the shift is now round-keyed, but
      identity makes `RoundStart-1 == EpochStart-1` on spectest configs)
      plus the 10 attributed stock-Gloas ones; delta zero. Extend the
      survey file with a dated section recording the actual delta and
      attributing every new failure. A nonzero delta is a bug in the
      identity claim — investigate before landing.

## 8.2 e2e ladder

- [ ] Invocation (no plain `go test` harness exists for the e2e —
      the bazel target owns the runfiles tree):
      `bazelisk test //testing/endtoend:go_heze_test
      --test_filter='<Test>' --test_output=all --nocache_test_results
      --flaky_test_attempts=1 > <scratchpad>/<name>.log 2>&1`.
      After every run, copy the shard `test.log`s and unzip
      `test.outputs/outputs.zip` from
      `bazel-testlogs/testing/endtoend/go_heze_test/shard_*/` into a
      per-run scratchpad dir BEFORE any rerun overwrites them; the node
      logs are the diagnosis substrate. (`TEST_TMPDIR`/logs must be on
      real disk if redirected — `/tmp` is a tmpfs here.)
- [ ] `TestEndToEnd_HezeGenesisShort` (1 epoch) — regression tier, run at
      the END OF STACK A and after each subsequent stack. With the 2.2
      guard it witnesses first justification (slot 24) inside its single
      epoch; finalization (slot 32) needs ~40 slots and is pinned by the
      unit tier instead.
- [ ] Full `TestEndToEnd_HezeGenesis` (5 epochs, 8/32) with
      `FinalizationOccursInRounds` and `JustificationAdvancesEveryRound`
      (5.3): justification advances every round, finalization latency 2
      rounds; the goldfish evaluators (`AvailableAttestationsFlow`,
      `AttestationsInEveryRound`, `ChainProducesBlocks`) unchanged and
      green. ~19 minutes; treat a wedge whose errors repeat every slot
      (`could not get block dependent root`, `block does not exist`) as
      the 1.3a/1.3b class and check the prune horizon first.
- [ ] Offset sweep (answered question 1): the offset-1 default is every
      run above; the offset-0 arm is a Short-run variant
      (`cfg.FFGTargetOffsetSlots = 0` beside the existing
      `cfg.SlotsPerRound = 8`) **paired with `WithSlotStartFFGVote()`**
      (1.1's reasoning — without slot-start voting there is no loss to
      measure). Assertions: per-round J/F progression still clears; the
      measured first-slot target-miss fraction is exactly 1/8 (read
      `targetRoot` per slot from the validator logs — the round's first
      slot votes a different root than slots 1-7, while offset 1 is
      uniform across all 8); the warmup shift (justification at slot 24
      names round 1, not round 2). Sustained offset-0 progression is a
      5-epoch follow-up, not this smoke.
- [ ] `TestEndToEnd_HezeGenesisCheckpointSync` — the cold-start witness
      for round-valued anchors (4.3) and the stater rule (4.4): the
      joining node's log shows the origin block at `RoundStart−1` and
      zero `invalid finalized root` errors.
- [ ] `TestEndToEnd_HezeGenesisSlotStartFFG` — slot-start vote + rounds
      together; the 0.95 participation floor (`heze_e2e_test.go:165-168`)
      may need re-derivation at per-round targets — measure first, then
      set, and record the number.
- [ ] Known baseline, established EXACTLY before any triage (run the
      failing suite in a jj workspace at the pre-plan tip — do not trust
      recorded counts, which go stale the moment a stack retypes an
      assertion): `rpc/prysm/v1alpha1/beacon` carries **33** genuinely
      pre-plan failures (all "bytes array does not have the correct
      length", across assignments/attestations/committees/validators
      tests); `TestServer_GetChainHead` is NOT one of them — its
      `want Epoch, got Round` shape is stack-caused and gets the
      mechanical assertion fix. Also pre-existing: the two blockchain
      failures named in 1.5; `rpc/eth/beacon TestSubmitAttestationsV2`
      post-electra (mock bypasses every changed function; a committed
      `debug.PrintStack()` marks it); flaky-but-clean: the two
      `core/helpers` sync-committee cache tests, one slasher surround
      subtest, one aggregation-ordering test, three `execution`-package
      "replacement transaction underpriced" tests, and
      `TestSaveOrphanedOps` (duplicate voluntary-exit fixture) — each
      passes in isolation; verify by rerun before attributing anything
      new.

## 8.3 Measurement acceptance criteria (the point of the whole plan)

Pinned numbers — a run that misses one is a bug hunt, not a shrug:

- **Finality latency in slots** (headline): min 16, mean ≈19.5, max 23 —
  a 16→23 sawtooth with period 8 (two round boundaries, plus position
  within the round). Against the epoch baseline (kurtosis run 02/05
  recomputed the same way): 64 / 78.3 / 95, first finality slot 128 —
  a **4.0× improvement, exactly SlotsPerEpoch/SlotsPerRound**. Finality
  still costs two boundaries; the boundaries are four times closer.
- **First justification slot 24 (round 2); first finalization slot 32**
  (the 2.2 guard timeline).
- **Per-round justification rate 1.00** — justified == wallRound−1 and
  finalized == wallRound−2 at every round boundary after warmup, no
  gaps, derived from the `beacon_*_round` gauges and corroborated by
  the advance counters (one increment per boundary per node).
- **Zero inter-node spread** — every node moves its checkpoints on the
  same slot, every round.
- **Attestation traffic unchanged**: exactly validators/8 FFG attesters
  per slot, flat across all 8 round offsets (rounds change WHEN finality
  happens, not how many votes exist — divergence in traffic is a bug,
  not a finding); goldfish seat fraction exactly 1.00 (step 7's bar);
  gate retreats matching the scenario's late publishers.

## 8.4 Kurtosis and Shadow recipes, with the harness gotchas

- [ ] Kurtosis: the run-02/05 shape scaled small for the short runs —
      6 nodes, 6 s slots, 32-slot epochs, `SLOTS_PER_ROUND: 8`, ~4
      epochs, late publishers off. **`prysmctl testnet
      generate-genesis` hangs below ~128 validators** — per-node key
      counts must keep the total ≥128 (6 × 22 = 132). Scrape the round
      gauges, `finality_latency_slots`, both advance counters,
      `p2p_pubsub_undeliverable_total`, `goldfish_vote_drop_total`, and
      the renamed `beacon_prev_round_*_gwei` family (5.2's rename lands
      in `scrape.sh`/`summarize.py` in the same change as the code).
      New run directory + `summary.md` under `kurtosis/runs/`, configs
      committed; bulky logs to `~/dev/prysm2-run-logs/`; run the
      votetally reconciliation as part of every summary. Enclaves:
      fresh name per run; spent ones (results committed) are
      disposable; one enclave holds ~10-15 GB of volumes — clear before
      the disk does it for you.
- [ ] Shadow (`/home/sukun/dev/decoupled-shadow-sim`; ethshadow at
      `/home/sukun/dev/ethshadow`): 8 nodes × 16 keys = 128 validators,
      12 s slots, ~25 min simulated (100 slots), `SLOTS_PER_ROUND: 8`
      via the sim config, `--goldfish-vote-ledger` on, fresh `dataN`
      dir. Expect the same pinned numbers (latency 16/19.4/23 at 12 s
      slots = 192-276 s wall). Gotchas: prysm's Shadow beacon logs
      contain a NUL byte — plain `grep` calls them binary, use
      `grep -a` or python; Shadow's logrus output quotes the message
      (`msg="Goldfish vote"`) where kurtosis logs emit it bare — the
      votetally regex must accept both; per-run binary basenames need
      the analysis scripts' globs widened (`prysm-validator*`).
      Aggregator counts scale with node count (halve the nodes, ~halve
      the aggregators) — not a regression.
- [ ] Record against `kurtosis/runs/02-main/summary.md` and the prior
      Shadow baseline; never delete prior run data outside the
      standing authorization.

---

# Deliberately unchanged (whole-plan table)

| site | why it is safe |
|---|---|
| FFG quorum math, justification bits shift, k-finality rules | code untouched; only invocation cadence and input units move (the "gadget unchanged" invariant) |
| Goldfish walk, gate, passthrough, proposal stub, vote store | reads the justified checkpoint as its stable-root stub; per-round justification only makes the stub advance faster; 1.4 converts its wall-clock pairings |
| The zero-late-tolerance vote acceptance rule | user decision; step 7 fixes loss CAUSES, never the rule — on clean sims no vote is late anyway |
| Available-attestation stream semantics (topic, validator, RPCs, mock committee arithmetic) | carries no FFG data; its checkpoint-state resolution converts in 3.5; delivery mechanics (buffer, queue, drain) are step 7's additions around unchanged acceptance rules |
| Committees, shuffle, seeds, proposer selection, RANDAO | epoch-keyed by design; rounds reuse the epoch shuffle via slot-in-round (plan step 2, executed) |
| `f.votes` + `ProcessAttestation` epoch granularity | pre-Heze head path only; goldfish never reads it (plan-next 4.4) |
| Pool pruning epoch windows (`prune_expired.go`) | retention only; packing filters (3.6) gate inclusion |
| Phase0/pre-Altair attestation + reward paths | unreachable at Heze (version dispatch at `transition.go:337`) |
| All of slashing: predicates, slasher, EIP-3076 gate | non-goal (user 2026-08-21); unit-agnostic comparisons keep working on round values; see step 6 |
| Gossip attestation propagation window | wire acceptance is slot-based and unchanged; state acceptance narrows via 3.3 — wire behavior must stay real (task charter) |
| db/kv checkpoint save/load, state schema, SSZ shapes | value-carriers; no field type or count changes anywhere in this plan |
| `processEpochGloas` | byte-identical; live in the 496-case spectest set; Heze gets its own pair (2.2) |
| The genesis guard CONSTANT (2 units) | derived, not chosen: the (0,0x00) stub + the prev-unit arm; only its clock moves (2.2) |
| `AttestationDueBPS = 3333`, `AVAILABLE_ATTESTATION_DUE_BPS_HEZE` | timing knobs, orthogonal (plan-next steps 4-5, executed) |

# Tests that will wake up (enumerate before running)

Under identity the expectation-edit count is: ONE fixture restatement
(the 1.4 goldfish cold-start subtest, by design) plus the 5.1 label
strings — everything else zero. The suites to RUN and watch:
`core/helpers`, `core/blocks`, `core/altair`, `core/electra`,
`core/epoch/precompute`, `core/transition`,
`forkchoice/doubly-linked-tree`, `blockchain` (`-tags develop`), `sync`,
`operations/attestations`, `rpc/core` + `rpc/prysm/v1alpha1/validator`
(+ its packing-filter fixtures, 3.6) + `rpc/prysm/v1alpha1/beacon`
(against the TRUE baseline, 8.2), `validator/client`, `validator/db/kv`,
`slasher`, and the `//go:build minimal` test targets (they must at least
BUILD — `-tags minimal` outside bazel flips fieldparams without the
runtime preset and is not a usable gate; bazel builds them). Renamed
symbols (`ValidateSlotTargetRound`, `TargetRootForRound`,
`FFGTargetRoot` signature, `FinalizedCheckpointRound`,
`calculateHeadAndTargetBounds`) mean mechanical test edits in their own
test files — mechanical renames are not expectation edits; anything
beyond a rename is a finding to investigate. New files need BUILD
entries; run gazelle on touched packages and re-check packages whose
imports changed (0.3). The prune tests are specified against the
horizon contract from the start (1.3a).

# Open questions — all resolved, 2026-08-21

1. **Target slot.** ANSWERED: **configurable.** `FFG_TARGET_OFFSET_SLOTS`
   config value (yaml-sweepable), default 1 (`RoundStart(R) - 1`), 0
   targets the round's own first slot. Built in 1.1, with the forkchoice
   twin honoring the same value (1.2) and the offset-0 smoke paired with
   slot-start voting (8.2).
2. **Checkpoint unit typing.** ANSWERED: **retype, so the compiler
   catches bugs** — step 0: proto `cast_type` of `Checkpoint.epoch` →
   `primitives.Round` (plus the four sibling carrier fields), the
   forkchoice checkpoint and node fields, and the state interface
   rename. A follow-up question compared this with the simplex spec's
   own direction — `Checkpoint{slot, root}` — and the user chose
   **smallest change**: round-valued stands (identity trick intact);
   slot-valued checkpoints are the gadget-era container change.
3. **What moves to round cadence.** ANSWERED: **justification and
   finalization only.** Committee selection stays epoch-based;
   rewards/penalties/inactivity stay epoch-based. The one dependency —
   per-round participation rotation — is encoded in 2.2 with the
   boundary-kind placement that keeps epoch rewards reading pre-rotation
   arrays (identity preserved).
4. **The inactivity leak.** Resolved: stays epoch-based;
   simplest change — one input conversion at the callers (2.5).
5. **Wire/pool retention.** ANSWERED: **leave as is** — slot-based gossip
   windows and epoch-based pool pruning unchanged while state acceptance
   narrows to the round pair (3.3, 3.6).
6. **Metrics.** ANSWERED: **new metrics are needed** — build 5.2 as
   written: the new `beacon_*_round` gauges, `finality_latency_slots`,
   the round-advance counters at the store's choke point, with the
   existing `*_epoch` gauges emitting the epoch of the round-start slot
   so dashboards and the scrape tooling keep meaning.

Additional stated requirements: per-round J/F PROGRESSION asserted at
every verification tier; vote accounting on clean simulations reads
exactly 100%, reconciled seat by seat (step 7); first finality lands at
slot 32 on the devnet (2.2's guard) so short runs measure finality
without an epoch-scale warmup. Nothing blocks execution.
