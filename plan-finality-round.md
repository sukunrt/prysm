# Plan-finality-round: FFG votes relate to rounds, not epochs

Written 2026-08-21. Companion: `plan-finality-round-detailed.md` (the work
items, file:line pointers, verification ladders). Successor to
`plan-next.md`; design record `task.md`; the tracked goal is
`plan-final.md` item 1 (`per-round-finality-vote-target` in the global
todo).

## Scope correction (user, 2026-08-21) — read this first

An earlier draft of this charter scoped the full Simplex finality gadget
(heights, timeouts, TSQ, view merge, a new finality-vote stream). **That is
all out of scope.** The finality gadget is NOT changed: Casper FFG's
machinery — 2/3 quorums over active balance, the justification-bits window,
the k=1/k=2 finalization rules, surround/double-vote slashing conditions —
stays exactly as it is.

What changes is the *unit* FFG runs over. Validators already attest once
per **round** (8 slots in the devnet config; plan step 1). But the FFG data
each vote carries — source and target checkpoints — and the
justification/finalization bookkeeping still name **epochs** (32 slots).
Voting once per round while justifying once per epoch is the mismatch this
plan removes: votes, checkpoints, and justification/finalization all key to
rounds. Fast finality means finality latency measured in rounds
(~2 rounds = 16 slots at the devnet config, against ~2 epochs = 64+ slots
today).

The Simplex spec at
`../decoupled-consensus-networking/consensus-specs/specs/_features/simplex/`
remains the reference for the *eventual* gadget and for the round helpers
it defines, but nothing from its gadget layer (Store heights, TSQ,
timeouts, stable-root freeze, safe confirmation, the modified
AttestationData) is implemented here.

## The change, concretely

1. **Target.** The FFG vote cast in round R targets a block at the
   round's start, with the exact slot CONFIGURABLE (user, 2026-08-21):
   `FFG_TARGET_OFFSET_SLOTS`, default 1 → `RoundStart(R) - 1` (the last
   block before the round, matching `plan-final.md` item 1), 0 → the
   round's own first slot. Underflow returns the anchor root, as the
   epoch-0 arm does today. Both offsets are exercised in the
   verification runs.
2. **Checkpoints carry rounds, as a distinct type.** `Checkpoint{epoch,
   root}` keeps its wire/SSZ shape; the field's VALUE becomes the round
   index and its Go type becomes `primitives.Round` via the proto
   `cast_type` (user, 2026-08-21: retype so the compiler catches the
   bugs). With `SlotsPerRound == SlotsPerEpoch` (shipped mainnet and
   minimal configs) the round index equals the epoch index at every
   slot, so the change is numerically the identity — the whole existing
   test suite and the spectest survey set stay put. Only the e2e/devnet
   configs (SLOTS_PER_ROUND=8 against 32-slot epochs) exercise real
   per-round finality. This is the plan-next "identity trick" applied a
   third time. (The simplex spec's own direction — slot-valued
   checkpoints — was considered and deferred to the gadget era: the user
   chose the smallest change, and slot values would break the identity
   trick.)
3. **Cadence: justification and finalization only** (user, 2026-08-21).
   J&F moves from the epoch boundary to every round boundary (a
   `ProcessRound` hook in `ProcessSlots`). Rewards, penalties, and
   inactivity stay at epoch cadence; committee selection stays
   epoch-based. The one dependency: the participation arrays J&F counts
   must rotate per round (or round R would justify on round R-1's bits);
   the detailed plan places the rotation so that epoch rewards still
   read pre-rotation arrays and today's ordering is preserved exactly
   under the identity configs. Epoch processing keeps everything else:
   registry, slashings, randao, sync committees, effective balances.
4. **Sources follow.** The attestation's source checkpoint is the state's
   justified checkpoint, which is now round-based; no separate work, but
   every site that converts `checkpoint.Epoch` to a slot must use
   `RoundStart`, and every site that derives a signing domain or committee
   from `target.Epoch` must derive it from the attestation's slot instead
   (rounds are not domain/shuffle units; epochs remain those).

## What is deliberately untouched

- The FFG quorum math, justification bits, finalization rules: unchanged
  code, new inputs.
- The Goldfish head vote, walk, gate, passthrough, round-start proposal
  stub, and the available-attestation stream: untouched. The stable root
  stays stubbed as the justified root — it simply advances per round now.
- Committee construction stays epoch-based (user, 2026-08-21, explicit).
  The shuffle seed, active-set lookup, committee counts, duty enumeration,
  and aggregator selection all keep their current epoch keying; the
  round-repeat of the shuffle within an epoch is the already-executed
  plan step 1 and is not touched here. This plan changes no committee
  code at all — where an attestation's (now round-valued) target field
  used to feed committee or domain lookups, the epoch is derived from the
  attestation's slot instead (detailed steps 3.3, 3.5).
- All of slashing (decision 2026-08-21: slashing out of scope):
  the surround/double-vote predicates, the slasher, and the EIP-3076
  protection gate are left with zero changes — they are unit-agnostic
  value comparisons and keep running on round values; whatever they
  detect or miss is not measured.
- Rewards/penalties fidelity is secondary (task charter: consensus values
  may be wrong; wire behavior must be real). No 1/rounds-per-epoch reward
  rescaling. The inactivity leak stays epoch-based (user, 2026-08-21;
  simplest change): one input conversion where the round-valued finalized
  checkpoint meets the epoch-typed delay computation — detailed step 2.5.

## What the detailed plan covers

0. The retype (step 0): `Checkpoint.epoch` → `primitives.Round` via the
   proto `cast_type` (wire/SSZ unchanged); the compile-error sweep is the
   mixed-units audit, with bare cross-unit casts forbidden.
1. The target shift with its configurable offset, state side and
   forkchoice side together (the plan-next 5.2 lesson: the two must move
   as one or `VerifyLmdFfgConsistency` rejects every vote).
2. The `ProcessRound` cadence hook and the split of Heze epoch
   processing; the leak's single input conversion.
3. Attestation plumbing: data construction, state/gossip/pool acceptance
   windows moving from {prev,current} epoch to {prev,current} round,
   domain-from-slot, committee resolution from slot.
4. Every consumer of `checkpoint.Epoch → slot`: blockchain service, db/kv,
   forkchoice pruning and dependent roots, status handshake and checkpoint
   sync, engine-API finalized notifications, plus the state-internal
   readers (registry activation gating, deposit finalization) that feed
   round-valued checkpoints into epoch-typed logic. Symmetric pairs and
   "no change needed" claims with checkable whys.
5. Logging, metrics reporting, and state management (user emphasis,
   2026-08-21): every log field and gauge that would print a round
   labeled "epoch" is enumerated and relabeled or converted; new headline
   metrics (`finality_latency_slots`, per-round justification rate) are
   named up front; the state split keeps SSZ shapes untouched and the
   caches/stategen surfaces are audited.
6. Slashing: the zero-change path — explicitly out of scope.
7. Measurement and verification: finality latency in slots as the
   headline number against kurtosis run 02 and the Shadow baseline;
   justification/finalization PROGRESSION asserted at every tier (user
   requirement) — unit, smoke chain-check, and two e2e evaluators
   (`FinalizationOccursInRounds`, `JustificationAdvancesEveryRound`);
   a smoke per target offset; spectest survey extension (the identity
   configs mean the expected-failure set should not move; the survey
   pass proves it and records any delta — no vector fixing).

## Rules (carried over, binding)

- jj discipline: new changes on top, one logical piece per change, clear
  descriptions, `Assisted-By: <model>` trailer only. Never delete data.
- `go vet` on touched packages; no blanket `go modernize`; lines under 100.
- Full command outputs to scratchpad log files; no bazel spectests as
  routine verification.
- Verification ladder per step: unit tests → ~3-slot single-node smoke →
  `TestEndToEnd_HezeGenesisShort` → full `TestEndToEnd_HezeGenesis` → sims
  last. No fixed sleeps racing setup; assert on observable chain state.
- Every `file:line` in the detailed file was verified on 2026-08-21 and is
  a pointer, not an address — grep for the symbol.

## Open questions — all resolved, 2026-08-21

Full answers recorded in `plan-finality-round-detailed.md`'s final
section. Summary: (1) target slot is CONFIGURABLE — offset 1 (slot −1,
default) or 0 (round's first slot), both first-class; (2) checkpoints are
RETYPED to `primitives.Round` so the compiler catches unit bugs
(smallest-change variant; the spec's slot-valued checkpoint is deferred
to the gadget era); (3) only justification/finalization moves to round
cadence — committees, rewards, penalties, inactivity all stay
epoch-based; (4) the inactivity leak stays epoch-based, simplest change;
(5) wire/pool retention stays as is; (6) new round metrics are built
(`finality_latency_slots`, round-advance counters, `beacon_*_round`
gauges). Additional stated requirement: verifying that justification and
finalization are PROGRESSING per round is asserted at every verification
tier (unit, smoke chain-check, two e2e evaluators). Nothing blocks
execution.

## Deliverable

This charter plus `plan-finality-round-detailed.md`, one jj change.
Nothing else modified.
