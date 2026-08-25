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

1. **Target.** The FFG vote cast in round R targets the block at slot
   `RoundStart(R) - 1` (the last block before the round), replacing today's
   `StartSlot(E) - 1`. The round-0 underflow returns the anchor root, as
   the epoch-0 arm does today. Open question 1 records the `-1` vs `0`
   (round's own first slot) choice; `-1` is recommended and matches
   `plan-final.md` item 1 ("the round starting at slot N targets the block
   at slot N-1").
2. **Checkpoints carry rounds.** `Checkpoint{epoch, root}` keeps its proto
   shape; the `epoch` field is reinterpreted as the round index. No proto
   or SSZ change. With `SlotsPerRound == SlotsPerEpoch` (shipped mainnet
   and minimal configs) the round index equals the epoch index at every
   slot, so the reinterpretation is numerically the identity — the whole
   existing test suite and the spectest survey set stay put. Only the
   e2e/devnet configs (SLOTS_PER_ROUND=8 against 32-slot epochs) exercise
   real per-round finality. This is the plan-next "identity trick" applied
   a third time.
3. **Cadence.** Justification/finalization processing and the
   participation-array rotation move from the epoch boundary to every round
   boundary (a `ProcessRound` hook in `ProcessSlots`, running before epoch
   processing when the boundaries coincide — which, under the identity
   configs, they always do, preserving today's ordering exactly). Epoch
   processing keeps everything else: registry, slashings, randao, sync
   committees, effective balances.
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
  rescaling. The one accounting item that MUST be fixed is the
  finality-delay/inactivity-leak computation, which would otherwise
  underflow or spuriously fire when round indices outrun epoch indices —
  see detailed step 2.

## What the detailed plan covers

1. The target shift, state side and forkchoice side together (the
   plan-next 5.2 lesson: the two must move as one or
   `VerifyLmdFfgConsistency` rejects every vote).
2. The `ProcessRound` cadence hook and the split of Heze epoch processing;
   the finality-delay conversion to rounds.
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
   headline number against kurtosis run 02 and the Shadow baseline; the
   e2e ladder with a rounds twin of the finalization evaluator; spectest
   survey extension (the identity configs mean the expected-failure set
   should not move; the survey pass proves it and records any delta — no
   vector fixing).

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

## Open questions for the user

Recorded with recommendations in `plan-finality-round-detailed.md`'s final
section; summary: (1) target slot `RoundStart(R)-1` vs `RoundStart(R)` —
recommend `-1` (matches plan-final.md item 1); (2) checkpoint unit typing
— recommend pure reinterpretation of the existing `Epoch`-typed fields,
no proto retype, with the mixed-units audit and 8/32 tests carrying the
load; (3) which accounting moves to round cadence — recommend
J&F + participation rotation + rewards + inactivity as one unit; (4) the
inactivity leak — with delay measured in rounds it arms ~4x sooner in
wall clock; recommend keeping the raw threshold, documented; (5) wire and
pool retention — recommend leaving the slot-based gossip windows and
epoch-based pool pruning as is while state acceptance narrows to the
round pair; (6) metrics compatibility — recommend the `*_epoch` gauges
emit the epoch of the round-start slot with new `*_round` gauges beside
them.

## Deliverable

This charter plus `plan-finality-round-detailed.md`, one jj change.
Nothing else modified.
