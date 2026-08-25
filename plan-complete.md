# Plan-complete: FFG votes relate to rounds, not epochs

Written 2026-08-22. Companion: `plan-complete-detailed.md` (the work items,
file:line pointers, verification ladders, acceptance numbers). This is the
finality-round charter written with nothing left to discover: successor to
`plan-next.md`; design record `task.md`; the tracked goal is `plan-final.md`
item 1 (`per-round-finality-vote-target` in the global todo). All pointers
are verified against the current tip (`ststlkmp` / a313817d).

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
(2 rounds = 16 slots at the devnet config, against ~2 epochs = 64+ slots
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
   epoch-0 arm does today. Both offsets are exercised in the verification
   runs. The offset arithmetic lives in ONE function, `slots.FFGTargetSlot`
   (in `time/slots`, because forkchoice cannot import `core/helpers`
   without a cycle); the state side, the forkchoice side, and every
   checkpoint-root-to-slot resolution read it, so they cannot drift.
2. **Checkpoints carry rounds, as a distinct type.** `Checkpoint{epoch,
   root}` keeps its wire/SSZ shape; the field's VALUE becomes the round
   index and its Go type becomes `primitives.Round` via the proto
   `cast_type` (user, 2026-08-21: retype so the compiler catches the
   bugs). Four sibling proto fields are value-carriers of the same index
   and retype with it (Status handshake, slasher HighestAttestation,
   doppelganger request, ChainHead) — leaving any of them `Epoch` forces
   a forbidden cast at its boundary. With `SlotsPerRound ==
   SlotsPerEpoch` (shipped mainnet and minimal configs) the round index
   equals the epoch index at every slot, so the change is numerically the
   identity — the whole existing test suite and the spectest survey set
   stay put. Only the e2e/devnet configs (SLOTS_PER_ROUND=8 against
   32-slot epochs) exercise real per-round finality. This is the
   plan-next "identity trick" applied a third time. (The simplex spec's
   own direction — slot-valued checkpoints — was considered and deferred
   to the gadget era: the user chose the smallest change, and slot values
   would break the identity trick.)
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
   The genesis stub guard — the spec's skip-the-first-two-units early
   return that protects the `(0, 0x00)` genesis checkpoint stub — moves
   to the ROUND clock (skip while the round is ≤ 1), so first
   justification lands at slot 24 and first finalization at slot 32 on
   the devnet, instead of 72/80. The constant stays 2 because the J&F
   pass justifies the previous unit as well as the current one: the first
   processed boundary must be the first whose previous-unit arm cannot
   reach unit 0 and rewrite the stub.
4. **Sources follow.** The attestation's source checkpoint is the state's
   justified checkpoint, which is now round-based; no separate work, but
   every site that converts `checkpoint.Epoch` to a slot must use
   `RoundStart` — and every site that converts a checkpoint's ROOT to a
   slot must use `slots.FFGTargetSlot`, because the root names the
   round's target block at `RoundStart(R) − offset`, not the round's
   first slot (the "finalized"/"justified" state lookups are the
   canonical case). Every site that derives a signing domain or
   committee from `target.Epoch` must derive it from the attestation's
   slot instead (rounds are not domain/shuffle units; epochs remain
   those).
5. **Fork choice must survive the faster clock.** Per-round finality
   advances the finalized checkpoint every round, and pruning follows
   finality — but the epoch-keyed consumers of the tree (the dependent
   root that anchors committee shuffling) still reach up to two epochs
   back. Value-correctness of a lookup is not reachability of its node:
   the pruning horizon therefore trails the finalized round by two
   EPOCHS (the retention floor is set by the epoch-keyed shuffle, not by
   finality), and a failed insert must unwind its bookkeeping or a
   transient error wedges head selection forever. The detailed plan
   treats node lifetime as a first-class design axis with its own tests.
6. **Vote delivery is part of the design surface.** The goldfish
   availability-vote stream delivers a whole network's votes in one
   synchronized burst (every validator publishes the moment the block
   reaches it). Four delivery mechanisms must be built for that shape —
   the gossipsub subscription buffer, the pending-queue key lifecycle,
   the queue-admission/wake-up predicate pair, and the drain on
   RPC-imported blocks — and the stream is instrumented end to end:
   every discard path carries a labeled counter, a per-vote ledger
   reconciles every expected seat, and the acceptance bar on a clean
   simulation is seat fraction EXACTLY 1.00 with zero unaccounted seats.
   Silent drops (upstream's IGNORE convention, libp2p's tracer-only
   overflow) are indefensible in an instrumented research fork.

## What is deliberately untouched

- The FFG quorum math, justification bits, finalization rules: unchanged
  code, new inputs.
- The Goldfish head vote, walk, gate, passthrough, round-start proposal
  stub, and the available-attestation stream's acceptance rules: the
  stable root stays stubbed as the justified root — it simply advances
  per round now. The zero-late-tolerance acceptance rule (votes past the
  slot boundary are dropped) is a user decision and is never loosened;
  the vote-delivery work fixes causes of loss, never the rule.
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
  protection gate get zero behavioral changes — they are unit-agnostic
  value comparisons and keep running on round values (the retype makes
  their internal variables `Round`-typed end to end); whatever they
  detect or miss is not measured.
- Rewards/penalties fidelity is secondary (task charter: consensus values
  may be wrong; wire behavior must be real). No 1/rounds-per-epoch reward
  rescaling. The inactivity leak stays epoch-based (user, 2026-08-21;
  simplest change): one input conversion where the round-valued finalized
  checkpoint meets the epoch-typed delay computation. Two accounting
  quirks are accepted and stated up front: epoch rewards read the last
  round's participation only, and the prev-round quorum balance uses the
  epoch's active set (detailed 2.4).

## What the detailed plan covers

0. The retype (step 0): `Checkpoint.epoch` → `primitives.Round` via the
   proto `cast_type` (wire/SSZ unchanged) across all five
   checkpoint-carrying protos; the compile-error sweep is the
   mixed-units audit, with bare cross-unit casts forbidden. The
   supporting surface ships first: Round's methodical-ssz method set,
   bytesutil encoders, the SSZ deep-equality `Round` case, the
   `FinalizedCheckpointRound()` interface rename that makes the compiler
   force every state-internal conversion.
1. The target shift with its configurable offset, state side and
   forkchoice side together (the plan-next 5.2 lesson: the two must move
   as one or `VerifyLmdFfgConsistency` rejects every vote) — plus the
   fork-choice lifetime work: the two-epoch pruning horizon, the insert
   error unwind, the offset-aware prune child bound, and the epoch-0
   underflow corner of the dependent root.
2. The `ProcessRound` cadence hook as a Heze-only function pair
   (`processEpochGloas` stays byte-identical — it is live in spectests);
   the genesis stub guard on the round clock; the leak's single input
   conversion.
3. Attestation plumbing: data construction, state/gossip/pool acceptance
   windows moving from {prev,current} epoch to {prev,current} round,
   domain-from-slot, committee resolution from slot.
4. Every consumer of `checkpoint.Epoch → slot` AND of
   `checkpoint.Root → slot`: blockchain service, db/kv, forkchoice
   pruning and dependent roots, status handshake and checkpoint sync,
   the state-id resolver ("finalized"/"justified" replay to the FFG
   target slot), engine-API finalized notifications, plus the
   state-internal readers (registry activation gating, deposit
   finalization) that feed round-valued checkpoints into epoch-typed
   logic. Symmetric pairs and "no change needed" claims with checkable
   whys.
5. Logging, metrics reporting, and state management (user emphasis,
   2026-08-21): every log field and gauge that would print a round
   labeled "epoch" is enumerated and relabeled or converted; new headline
   metrics (`finality_latency_slots`, per-round justification rate,
   round-advance counters incremented at the store's single
   checkpoint-replacement choke point so the tick path counts) are named
   up front; the state split keeps SSZ shapes untouched and the
   caches/stategen surfaces are audited.
6. Slashing: the zero-change path — explicitly out of scope.
7. Goldfish vote delivery and instrumentation: the four delivery
   mechanisms sized for the synchronized burst; the labeled drop
   counters, the per-vote ledger, and seat-by-seat reconciliation as
   REQUIRED run tooling; acceptance bar seat fraction exactly 1.00 on
   clean sims.
8. Measurement and verification: finality latency in slots as the
   headline number against kurtosis run 02 and the Shadow baseline, with
   the expected values pinned as acceptance criteria (16/≈19.5/23-slot
   sawtooth; first J slot 24, first F slot 32; 4.0× vs the epoch
   baseline's 64/78.3/95; flat 16-per-slot attester traffic; per-round
   justification rate 1.00; zero inter-node checkpoint spread);
   justification/finalization PROGRESSION asserted at every tier (user
   requirement) — unit, smoke chain-check, and two e2e evaluators
   (`FinalizationOccursInRounds`, `JustificationAdvancesEveryRound`);
   a smoke per target offset (the offset-0 smoke paired with the
   slot-start vote flag, where the predicted 1/8 target miss actually
   occurs); the checkpoint-sync e2e as the cold-start witness; spectest
   survey extension (the identity configs mean the expected-failure set
   should not move; the survey pass proves it and records any delta — no
   vector fixing).

## Rules (carried over and extended, binding)

- jj discipline: new changes on top, one logical piece per change, clear
  descriptions, `Assisted-By: <model>` trailer only. Never delete data —
  except: spent kurtosis enclaves and Shadow `data*` dirs whose results
  are recorded in a committed summary are disposable when space is
  needed (user authorization, this task only). Budget disk before the
  measurement phase: enclaves and Shadow runs cost tens of GB each.
- `go vet` on touched packages; no blanket `go modernize`; lines under
  100.
- Full command outputs to scratchpad log files; no bazel spectests as
  routine verification.
- Verification ladder per step: unit tests → ~3-slot single-node smoke →
  `TestEndToEnd_HezeGenesisShort` → full `TestEndToEnd_HezeGenesis` →
  sims last. **The Short e2e runs DURING the retype stack, not after all
  stacks land** — cross-cutting interaction bugs (pruning versus
  epoch-keyed lookups) are invisible below the chain tier, and finding
  them one stack late costs a day. No fixed sleeps racing setup; assert
  on observable chain state.
- **The lifetime-audit rule:** any change to prune cadence or retention
  requires an explicit audit of everything that walks the fork-choice
  tree by age, proving reachability (not just value-correctness) for
  each consumer; the accompanying unit tests must actually prune.
- **The instrument-first rule:** before the measurement phase, every
  silent discard path on the streams being measured carries a labeled
  counter, and per-vote/per-seat accounting tooling exists. A metric
  that should read 100% on a clean simulation and does not is a bug to
  fix in code, looping fix→run until it reads exactly 100%.
- Every `file:line` in the detailed file is verified against `ststlkmp`
  and is a pointer, not an address — grep for the symbol.

## Open questions — all resolved, 2026-08-21

Full answers recorded in `plan-complete-detailed.md`'s final section.
Summary: (1) target slot is CONFIGURABLE — offset 1 (slot −1, default) or
0 (round's first slot), both first-class; (2) checkpoints are RETYPED to
`primitives.Round` so the compiler catches unit bugs (smallest-change
variant; the spec's slot-valued checkpoint is deferred to the gadget
era); (3) only justification/finalization moves to round cadence —
committees, rewards, penalties, inactivity all stay epoch-based; (4) the
inactivity leak stays epoch-based, simplest change; (5) wire/pool
retention stays as is; (6) new round metrics are built
(`finality_latency_slots`, round-advance counters, `beacon_*_round`
gauges). Additional stated requirements: verifying that justification and
finalization are PROGRESSING per round is asserted at every verification
tier (unit, smoke chain-check, two e2e evaluators); vote accounting on
clean sims must read exactly 100%, reconciled seat by seat. Nothing
blocks execution.

## Review

Adversarially reviewed; findings folded into the detailed file. The
themes a reviewer of this class of change must check, in order: bare
cross-unit casts and value-level unit bugs the compiler cannot see
(EpochStart-that-should-be-RoundStart, SlotsPerEpoch.Mul on a round);
the state/forkchoice target symmetry at BOTH offsets including empty
round-start slots; node lifetime under per-round pruning for every
epoch-keyed tree consumer; the coinciding-boundary value identity of the
cadence split; and plan conformance on the committees/slashing
no-change mandates. The retype is the right typing choice — the
compiler, not a grep, has to be the auditor — and the lifetime axis is
exactly what the type system cannot see, which is why it gets its own
sections (1.3a, 1.3b) and pruning tests.

## Deliverable

This charter plus `plan-complete-detailed.md`, one jj change per
revision. Nothing else modified.
