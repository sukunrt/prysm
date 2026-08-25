# Plan-complete ledger: every commit `ststlkmp..tip` against the original plan

The audit trail behind `plan-complete.md` / `plan-complete-detailed.md`.
Method: every jj commit between `ststlkmp` (a313817d, the tip
`plan-finality-round.md` was written against) and the execution tip was
classified against the original plan — **planned** (which section),
**partial** (what knowledge the plan lacked), or **MISS** (work the plan
did not know existed). Mixed commits are classified at the item level,
using the committed executor notes (`stack{A,B,C}-executor-note.md`),
which record the per-hunk deltas of the large commits, plus direct diff
reads. A reverse pass lists plan items with no commit and plan claims
execution falsified. "→ §" points at where the knowledge now lives in
`plan-complete-detailed.md`.

## Forward pass: commit → classification

| commit | description | classification |
|---|---|---|
| `kvwlkszw` (11f) | Round SSZ method set | **partial** — plan 0.1 named the methodical-ssz set; the plan lacked: bytesutil Round encoders, `slots.{RoundsSinceGenesis,RoundEnd,FFGTargetSlot}`, `FFG_TARGET_OFFSET_SLOTS` plumbing rows landing here, ssz deep-equality `Round` case (panics without it), `apiutil.Uint64ToString` type set → §0.1 |
| `vxvnsksp` (211f) | the retype + sweep + target | **planned** (0.2, 1.1-1.4, mechanical halves of 2.5/2.6/3.x/4.x) with these item-level MISSES, per stackA note §2: four extra checkpoint-carrier protos (p2p_messages, slasher, validator, beacon_chain) → §0.2; `FinalizedCheckpointEpoch→FinalizedCheckpointRound` interface rename → §0.2; `FFGTargetSlot` in `time/slots` not `core/helpers` (import cycle) → §0.1; `CurrentRound/PrevRound` pulled from stack B → §0.2; state-native 4-line `checkpointEpoch` twin → §2.6; `calculateHeadAndTargetEpochs` latent unit bug → renamed `...Bounds`, returns slots → §4.0; `finalized_block_roots` epoch-RANGE filter (plan said "read it") rewritten as slot ranges → §4.2; `EpochsSinceGenesis`→`RoundsSinceGenesis` viability args (not on any plan list) → §1.4; dependent-root `MaxUint64` underflow corner → §1.3; goldfish cold-start fixture restatement (the one real expectation edit; plan predicted zero) → §1.4; hdiff `state_diff.go` literals → §0.3; epoch-constants-as-round-counts set (reorg window, `roundsSinceFinality*`, `pruningRoundCutoff`, `farFutureRound`, slasher params Round-typed) → §0.3/§1.4/§6; 4-arm insert rule with explicit offset-0 second arm → §1.2 |
| `xuoploym` | stack A note | record-keeping (the deltas above) |
| `sprtxnqs` (1f) | initial-sync BUILD dep | **MISS** — gazelle doesn't catch import-only changes; broke only at the bazel e2e build → §0.3 |
| `rzoxuztl` (9f) | ProcessRound cadence | **planned** (2.1-2.4) with plan-lacked detail: rotation gated on `!CanProcessEpoch`; epoch part re-runs its own precompute (cost note); ~15-call deliberate duplication vs `processEpochGloas` (spectest-live, review already forbade splitting it); accepted accounting quirks stated (last-round rewards, epoch active set) → §2.2 |
| `knqprzzo` | stack B note | record-keeping |
| `nwsqxvyx` (12f) | log-field relabels | **planned** (5.1); plan lacked the concrete hit list (13 relabels incl. monitor `targetEpoch1/2`, slasher/slasherkv, forkchecker, tracing attributes; ~7 label-string test edits) → §5.1 |
| `upntouvx` (4f) | metrics: rounds + latency | **partial** — gauges/new-rounds planned (5.2); plan lacked: `finality_latency_slots` belongs in `reportSlotMetrics` per-slot (plan's "on advance" freezes during a stall; `ProcessRound` has no wall clock, runs in replay); `reportEpochMetrics`→`reportRoundMetrics` + `beacon_prev_round_*_gwei` renames with scrape propagation → §5.2. Its first-cut advance counters on the block path were WRONG — superseded by `ysmuskqy` (see below) |
| `uuolmtlr` (3f) | ChainHead carries rounds | **partial** — ChainHead comment planned (5.3); MISS: the REST twin (`beacon-api`) parsed rounds as epochs and derived `*Slot` via `EpochStart` — 4× wrong at 8/32 → §5.3 |
| `tustvtzn` (3f) | the two e2e evaluators | **planned** (5.3); detail: wiring via `hezeDroppedEvaluators`, `AfterNthEpoch(3)` gates → §5.3 |
| `xkppvvmr` | stack C note | record-keeping |
| `puuworwz` (2f) | offset-aware prune child bound | **MISS** (review B1) — plan 1.2 named prune but not that the child-compat bound is offset-dependent → §1.3a |
| `lrsxwsnk` (4f) | doppelganger unit split | **MISS** (review M1) — plan 4.0 said "convert the comparison to like units" without the gate-vs-evidence rule; rounds gate + epoch evidence = false positives → §4.0 |
| `zywurrsr` (8f) | WS emits round | **MISS** (review M2) — plan 4.0 named `GetWeakSubjectivity` but not the producer/consumer round-trip requirement → §4.0 |
| `puvrpnkv` (2f) | findFork round rewind | **MISS** (review M3) — findFork not on any plan list → §4.0 |
| `qtvxtvxx` (2f) | proposer_attestations rename finish | **partial** — 3.6 planned the renames; the package's test fixtures (and the `//go:build minimal` targets' build) were unlisted → §3.6, tests section |
| `kormpkkn` (5f) | hoist activation conversion | style completion of 2.6's conversions (no new knowledge; ledger-only) |
| `oyyuunpq` (4f) | prune horizon on the epoch rhythm | **MISS — the largest one.** Plan 1.3 proved the dependent root's VALUE and said nothing about node lifetime once `on_tick` prunes per round; every insert asks for `currentEpoch−1`, always below a finality-clocked cut → chain-wide wedge at the first post-finality prune. Fix: 2-epoch trailing horizon; tests must prune; `ctx.Err()` up front → §1.3a + the lifetime-audit rule |
| `yznzwszr` (2f) | insert unwind | **MISS** — pre-existing latent leak made reachable by the horizon class of failure; a failed insert left a node naming a rolled-back block → permanent head wedge → §1.3b |
| `lttlxurz` (3f) | stale comments | hygiene (ledger-only) |
| `zxytzvmx` (1f) | GetChainHead test asserts | **MISS in the plan's baseline bookkeeping** — the recorded "32 pre-existing rpc failures" included a stack-caused one; true pre-plan baseline is 33, established in a jj workspace at `ststlkmp` → §8.2 |
| `yuoozyyo` (4f) | genesis stub guard on round clock | **decision reversing plan 2.2** (stays epoch-based, deliberate) once its cost was visible: warmup 8 rounds → 2 (J slot 24, F slot 32); constant stays 2 (stub + prev-arm derivation); identity-safe → §2.2 |
| `qtvvnwpk` (2f) | kurtosis tooling measures rounds | **MISS** — plan 7.3 named the runs but no tooling requirements (scrape/summarize round metrics, `--slots-per-round`) → §8.4 |
| `xqqptyrv` | run 06 record | measurement; exposed the two instrument bugs below |
| `ysmuskqy` (6f) | counters at the checkpoint choke point | **MISS** — plan 5.2 didn't say WHERE; block-path counters never fire (forkchoice realizes advances on the tick) → `advance{Justified,Finalized}Checkpoint` funnel both paths → §5.2 |
| `tszqrkto` (1f) | summarizer window scope | **MISS** — seat fraction read over every sample, spurious warm-up minimum → §7.5 |
| `lzqnzmlp` (3f) | queue-deletion race | **MISS** (vote loss #2) — queue key deleted after batch processing discards concurrent arrivals → §7.2 |
| `vtqxrwzv` (2f) | scrape undeliverable counter | **MISS** — the only witness of buffer overflow was untracked → §7.1/§7.5 |
| `orovsvnw` (2f) | subscription buffer for the burst | **MISS** (vote loss #1, dominant 2.4%) — libp2p 32-msg non-blocking hand-off vs a 132-vote synchronized burst; drops before validation, invisible everywhere → §7.1 |
| `kmyrlqyy` | run 07 record | measurement (counters live; buffer fix proven; residual traced) |
| `xusyoqln` (5f) | labeled drop reasons | **MISS** — instrument-first: upstream silent-IGNORE convention hid losses; every discard path gets `goldfish_vote_drop_total{reason}` → §7.5 |
| `qyqworom` | run 08 record | measurement (0.9995; residual isolated) |
| `rqpwlqyv` (10f) | per-vote ledger flag | **MISS** — user-mandated accounting: one line per vote, all outcomes → §7.5 |
| `qztnrwwk` (2f) | votetally reconciliation | **MISS** — expected-side from committee arithmetic, seat-by-seat reconciliation as required run tooling → §7.5 |
| `snmvztrt` (3f) | queue gate == wake-up condition | **MISS** (vote loss #3) — admission on `hasBlockAndState` vs wake-up on forkchoice import; the gap strands a batch, nothing wakes a queue twice → §7.3 |
| `vwnsxolo` | run 11 record | measurement (ledger names the queued-and-forgotten shape) |
| `pwynozmw` (3f) | drain on every import path | **MISS** (vote loss #4) — drain hung off gossip subscriber + pending queue only; a proposer's own RPC-imported block strands its peers' committee → §7.4 |
| `swlwrztw` | run 12 record | acceptance: 1.00 everywhere, 427,008/427,008 seats |
| `znstxopy` (1f) | offset-0 e2e arm | **partial** — plan 7.2 planned the smoke; lacked: the 1/8 loss exists only WITH slot-start voting; the one-round warmup shift at offset 0 → §8.2/§1.1 |
| `qrqyusru` (2f) | stater resolves at FFG target slot | **MISS** — plan 4.2 listed `stater.go:143,157` as a mechanical `RoundStart` conversion; the checkpoint ROOT names the block at `RoundStart−offset`, so `RoundStart` serves a state one block off → checkpoint-synced node rejected by every peer ("invalid finalized root", presents as a discovery failure). The root→slot class gets its own rule → §4.4 |

## Reverse pass: plan items with no commit, and falsified claims

- **The 3-slot single-node smoke tier** (plan rules, 2.7, 5.4): no
  runnable harness exists in this checkout; the tier is the Short e2e via
  the bazel target. Plan-detailed's old "plain go test, no Bazel" harness
  note is stale → §8.2 pins the real invocation, `-tags develop`, and the
  log-saving discipline.
- **Plan 2.2 "genesis guard stays epoch-based, deliberate"**: reversed by
  the user during execution → designed-in as round-clock (§2.2).
- **Plan 5.2 "finality_latency_slots updated on finalized-checkpoint
  advance"**: falsified — freezes during stalls → per-slot in
  `reportSlotMetrics` (§5.2).
- **Plan 1.3's "one-line reimplementation, no extra node pointer"**:
  value-true, lifetime-false (the largest miss, `oyyuunpq`) — and silent
  on the `MaxUint64` caller corner.
- **Plan's "expected expectation edits: zero"**: falsified by exactly one
  fixture (goldfish cold-start) plus label strings → stated up front
  (identity rule, §1.4, §5.1).
- **Plan's helper placement "next to FFGTargetRoot"**: import-cycle
  falsified → `time/slots` (§0.1).
- **Plan 4.2's rpc_status m1 item**: the natural "loosen for slack" fix
  is backwards (the guard rejects on `>`, so `−2` in rounds is tighter);
  analysis recorded, no loosening → §4.2.
- **Plan 7.2's offset-0 loss prediction ("a voter at slot start …
  1/8")**: incomplete — no loss at stock vote timing; smoke must pair
  the offset with `WithSlotStartFFGVote()` → §8.2.
- **Plan 7.3 measurement expectations ("~16 vs ~128")**: refined to
  pinned acceptance numbers (16/19.5/23 sawtooth; baseline 64/78.3/95,
  first finality 128; 4.0×; rate 1.00; zero spread) → §8.3.
- **Planned, not executed in the reference run** (kept as steps): the
  spectest survey re-run (7.1 → §8.1) and the
  `TestEndToEnd_HezeGenesisSlotStartFFG` full variant (7.2 → §8.2).
- **Known-failure bookkeeping**: "32 rpc failures" was wrong (33; the
  32-list contained a stack-caused one) — baselines are established in a
  workspace at the pre-plan tip, never trusted from notes → §8.2.
- **Operational knowledge the plan lacked entirely**: disk budgeting
  (enclaves 10-15 GB, Shadow runs GBs, bazel cache tens of GB; spent
  runs with committed summaries disposable); `/tmp` is tmpfs; prysmctl's
  ~128-validator genesis floor; Shadow log NUL bytes (`grep -a`); logrus
  quoting vs the votetally regex; per-run binary basename globs → §8.4
  and the rules.

## Cross-check against the ten dictated categories

All ten categories are covered by ledger rows (1→`oyyuunpq`/`yznzwszr`/
`puuworwz`/`vxvnsksp`-corner; 2→`yuoozyyo`; 3→`kvwlkszw`/`vxvnsksp`/
`sprtxnqs`; 4→`lrsxwsnk`/`zywurrsr`/`puvrpnkv`/`vxvnsksp`/m1; 5→
`qrqyusru`; 6→`ysmuskqy`/`upntouvx`/`qtvvnwpk`; 7→the eight sync/
tooling commits; 8→`znstxopy`; 9→reverse pass; 10→§8.3).

Ledger items NOT in the dictated categories (flagged per method):
- the ssz deep-equality `Round` case (panic), `apiutil` type set, hdiff
  literals, `RoundsSinceGenesis`/`EpochsSinceGenesis` viability args;
- the epoch-constants-as-round-counts reinterpretation set (reorg
  window, `roundsSinceFinality*`, `pruningRoundCutoff`,
  `farFutureRound`, slasher params Round-typed end to end);
- `finalized_block_roots`' epoch-RANGE (not EpochStart) rewrite;
- the `viableForHead +2 → two rounds` behavioral note and its one
  fixture edit;
- the doubled-precompute cost note and the two accepted accounting
  quirks (2.2);
- the packing-filter TEST fixtures + `//go:build minimal` build gate;
- `finality_latency_slots` per-slot placement rationale (stall
  visibility);
- the un-executed plan items (spectest survey, SlotStartFFG variant)
  and the evaluator `AfterNthEpoch(3)` margin note;
- `kormpkkn`/`lttlxurz` (style/hygiene; no plan knowledge missing).
