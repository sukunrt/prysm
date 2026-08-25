# Task: scope the per-round finality vote (the gadget era)

Written 2026-08-21 for a planning agent. The deliverable is a full
implementation plan in the style of `plan.md` / `plan-next.md` — spec
deltas, containers, wire paths, forkchoice integration, verification
ladder — ready for executor agents. Do not implement anything under this
task; the plan is reviewed by the user first. Flag design decisions as
open questions rather than deciding them.

## The goal, in the user's words

The finality vote in the round starting at slot N targets the block at
slot N-1 — a per-ROUND target; that is what fast finality means
(user, 2026-08-20; recorded in `plan-final.md` item 1 and the global todo
`per-round-finality-vote-target`).

Today's finalization is stock per-epoch Casper FFG with one deviation: the
target is shifted to `StartSlot(E) - 1` (plan-next step 5.2). That is an
interim mock. This task scopes its replacement: a finality vote stream
whose target moves every round and whose justification/finalization
bookkeeping is per-round, so finality latency is measured in rounds, not
epochs.

## Design sources — read before writing anything

- `task.md`: the design record. Decisions 12-17 cover the goldfish head
  vote, timing, adversarial knobs, and ordering; the gadget items were
  deliberately deferred out of plan-next.
- The executable spec:
  `../decoupled-consensus-networking/consensus-specs/specs/_features/simplex/`
  (fork-choice.md was the source for the goldfish walk; the gadget pieces
  named in plan-next's not-in-scope list live here: the Fresh Simplex
  store, timeouts, TSQ, height filter, view merge, and the stable root).
  Where the spec and this repo's mock disagree, the spec wins; where the
  spec is silent, raise an open question.
- `plan-next.md` "Not in scope" — the exact debt this task pays down:
  the finality gadget store, the finality-vote message stream (the
  2026-08-14 plan's clone of the attestation path), slashing for
  equivocating votes (evidence kept, nothing acts on it), and the stable
  root stubbed as the justified root.
- `plan.md`, `plan-detailed.md`, `plan-next-detailed.md`: the executed
  record, including every `<added by executor>` deviation note. The
  "Lessons applied" section of plan-next is binding style for the new
  plan (symmetric pairs, verified no-change claims, identity-trick
  activation, budgeted multi-cause debugging, BUILD twins, line drift).

## What exists today (verified state, tip `wvornuor`)

- Rounds are real: `SlotsPerRound`, per-round committees, validators
  attest once per round. Genesis is Heze; there is no fork transition.
- Head selection is the goldfish walk over available attestations
  (`forkchoice/doubly-linked-tree/goldfish.go` and neighbors): per-slot
  vote store, seat scoring, majority gate, current-slot passthrough,
  round-start behavior. The stable root is stubbed as the justified root.
- The FFG mock: target `StartSlot(E) - 1` via `helpers.FFGTargetRoot`
  (`beacon-chain/core/helpers/block.go`), unconditional; epoch-0
  underflow falls back to the anchor root. Slot-start vote flag
  `--decoupled-ffg-vote-at-slot-start` (+ jitter flag), head-source knob,
  `AVAILABLE_ATTESTATION_DUE_BPS_HEZE` as the head-timing sweep axis.
- A pending queue replays available attestations that arrive before
  their block (`beacon-chain/sync`); the gate abstains on empty vote
  slots (cold start; commit `ukxoopmz` — deviation awaiting sign-off);
  checkpoint sync works and is witnessed in e2e; five genesis-edge
  payload bugs are fixed (parent hash / envelope / common-ancestor).
- Verification stack: 2-slot EL witnesses, 6-slot and 1-epoch
  send-and-verify state windows, `TestEndToEnd_HezeGenesis` (5 epochs),
  SlotStartFFG variant, checkpoint-sync variant, kurtosis harness with
  scrape tooling (run 05 recorded), ethshadow baseline (data19) — the
  numbers new work is measured against. Metrics live in
  `forkchoice/doubly-linked-tree/metrics.go` (`goldfish_*`) and the
  payload-attestation counters.
- Spectests: survey-only policy. `testing/spectest/SURVEY-2026-08-21.md`
  records 486 expected failures from the target shift and 10 pre-existing
  stock-Gloas ones. The new plan must state its expected spectest impact
  and extend the survey, not fix vectors.

## What the plan must cover

1. **The finality-vote stream.** Container, signature domain, gossip
   topic, validation pipeline, pool/aggregation (if any), block inclusion
   or standalone propagation, and the symmetric pairs: send+receive,
   store+read, fresh-start+restart+checkpoint-sync. State explicitly
   whether the existing committee attestation keeps carrying FFG data
   during a transition window, and how both being live at once behaves.
2. **Per-round justification/finalization.** The bookkeeping that replaces
   the epoch machinery for this stream: quorum accounting per round
   (seats vs balance — say which and why, consistent with the spec),
   justified/finalized advancement, and the interaction with the existing
   per-epoch FFG processing while both exist. The gadget store pieces from
   the spec (timeouts, TSQ, height filter, view merge) — scope each or
   explicitly defer it with a reason.
3. **The stable root becomes real.** What replaces the justified-root
   stub, and every reader of the stub today (goldfish gate, round-start
   proposal path, status handshake `xrmqllqx`) re-examined against the
   real definition.
4. **Retiring the mock.** The per-epoch target shift
   (`FFGTargetRoot` and its six call sites) is replaced by this work per
   `plan-final.md`; the plan says when in the sequence that happens and
   what the spectest survey's expected-failure set becomes.
5. **Equivocation and slashing.** At minimum the evidence path for the
   new votes; whether slashing activates now or stays recorded-only is an
   open question for the user.
6. **Metrics and measurement.** Finality latency in slots/rounds as the
   headline metric, against the recorded kurtosis run 05 and Shadow
   baselines; per-round justification rate; vote arrival fractions.
   Name the exact new metric names up front (the run 05 pattern).
7. **Verification ladder per step** (binding convention): unit tests →
   2-3 slot witness → `TestEndToEnd_HezeGenesisShort` → full 5-epoch e2e
   → sims last. Full outputs to files. No spectest fixing — survey only.
   The user rejects fixed sleeps that race setup; tests assert on
   observable chain state.

## Rules for the plan document itself

- House rules carry over verbatim from plan-next (jj discipline, one
  change per logical piece, `Assisted-By:` trailer, never
  `Co-Authored-By:`, no data deletion, line length 100, go vet, no
  blanket modernize).
- Every `file:line` pointer verified against tip `wvornuor` on the day of
  writing, named as pointer-not-address.
- Every "no change needed" claim carries its checkable why.
- Open questions section at the end, one crisp recommendation each —
  the user answers there, as in the prior plans.

## Deliverable

`plan-final.md` expanded into the full plan (or a sibling
`plan-final-detailed.md` pair, matching the existing convention), as one
jj change with a clear description. Nothing else modified.
