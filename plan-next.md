# Plan-next: Goldfish head vote, timing knobs, kurtosis + ethshadow

Written 2026-08-19, for execution 2026-08-20 by other agents. The design
record is `task.md`: decision 13 (Goldfish), decision 12 (timing), decision
14 (adversarial knobs), decision 17 (order). The predecessor plan
(`plan.md` / `plan-detailed.md`) is fully executed: genesis is Heze,
rounds are real, validators attest once per round, `TestEndToEnd_HezeGenesis`
is green, and the Shadow baseline run is recorded in section 5.3/5.4 there.

## Scope

Three steps, in order. Steps 4 and 5 continue decision 17's numbering.

4. Goldfish: forkchoice consumes the available attestations as the head
   vote; block proposing follows it.
5. The timing knobs: move the finality vote freely in the slot, and choose
   which slot's block a vote names.
6. Run the whole stack in kurtosis and ethshadow, and record the same
   baseline plus the new Goldfish metrics.

## Rules

Carried over from the executed plan, plus one new rule learned there.

- **Do not abandon or rewrite any existing jj change.** New changes on top.
- One jj change per logical piece. Describe each. Sign with a single
  `Assisted-By: <model>` trailer. Never `Co-Authored-By:`.
- `go vet` on touched packages. **No blanket `go modernize`** — it rewrites
  unrelated files in this tree; apply its idioms only to new code.
- Lines under 100 characters.
- **Never delete data without asking**: run outputs, logs, tarballs,
  `data*` dirs in the sim workspace.
- **Full outputs to log files** in the session scratchpad — never through
  `tail`/`head`. **No bazel spectests** as routine verification (user
  decision; one dedicated pass later). `go build ./...`, targeted
  `go test`, and `bazelisk build //...` are the per-step accept commands.
- **New: the verification ladder.** Every step's acceptance defines a cheap
  shakeout tier before any long run: unit tests → single-node ~3-slot smoke
  (the `step5a`/`step5b` scripts pattern) → `TestEndToEnd_HezeGenesisShort`
  (~3 min) → full `TestEndToEnd_HezeGenesis` (~19 min) → sims. Iterate at
  the cheapest tier that can show the failure.

## Lessons applied

The executed plan's `<added by executor>` notes name the mistake classes.
This plan was written against that list:

1. **Symmetric pairs.** Naming only the write side (db/kv marshal without
   unmarshal) cost step 3 a rework. Every mechanism below enumerates both
   directions: send+receive, store+read, fresh-start+restart+checkpoint-sync.
2. **"No change needed" claims get verified, not assumed.** Step 2's
   committee-cache claim was wrong because cache *contents* embedded a
   config value. The deliberately-unchanged tables below say *why* each row
   is safe, and the why is checkable.
3. **Config-gated activation is the identity trick.** Step 2 shipped with
   `SlotsPerRound == SlotsPerEpoch` everywhere so no existing test moved.
   The analog here: the Goldfish walk activates only when `HezeForkEpoch`
   is reached. Shipped mainnet/minimal configs keep `MaxUint64`, so the
   whole existing forkchoice and blockchain suite runs the old path
   untouched; only the e2e/devnet configs (Heze at 0) exercise the new one.
4. **Test loops wake up.** Removing versions from `unsupportedVersions`
   woke nine suites in step 3. Step 4 adds no version and no proto, so the
   blast radius is the forkchoice and blockchain packages only — but any
   test that asserts LMD weights or `IsCanonical` on a Heze-epoch-0 config
   will move. They are enumerated in the detailed file.
5. **Single-cause assumptions.** The "one missing MarkFullNode" turned out
   to be four blockers. Step 4's smoke tier exists precisely to find the
   second, third, and fourth cause cheaply; the plan budgets for them.
6. **Bazel/generated twins.** No proto or preset changes here, so the twin
   risk is only new files needing BUILD entries and gazelle. Named per step.
7. **Line numbers drift.** Every `file:line` below was verified on
   2026-08-19 and is a pointer, not an address. Grep for the symbol.

## The one thing not to get wrong

**The current-slot passthrough lands before proposer boost is removed.**
A fresh block has a Goldfish score of 0 by construction — the votes that
could score it are cast in its own slot and read one slot later. The spec's
`is_available_attestation_viable` passes a child at the current slot except
at a round-start slot. Without that passthrough, no new block ever clears
the gate, the walk never advances, and the chain silently stops extending —
it looks like a liveness bug three layers away. Implement and unit-test the
passthrough first, then turn off proposer boost, the late-block reorg, and
the FFG→LMD input, in that order, in the same change or later ones.

The trap right behind it: **do not reuse `f.votes`.**
`ProcessAttestation` (`forkchoice/doubly-linked-tree/forkchoice.go:112`)
updates a vote only when `targetEpoch > nextEpoch` — epoch granularity,
never expiring. Goldfish votes are per-slot and expire after one slot.
Separate store, separate walk. There are 14 non-test reads of `f.votes` in
the package; the new walk reads none of them.

## Not in scope

- The finality gadget itself (Fresh Simplex store, timeouts, TSQ, height
  filter, view merge). The stable root is stubbed as the justified root.
- The finality-vote message stream (the 2026-08-14 plan's clone of the
  attestation path). Step 5's knobs move the *existing* FFG attestation and
  the available attestation; the dedicated finality-vote container comes
  with the gadget.
- Slashing for equivocating available attestations. The store keeps the
  evidence; nothing acts on it.
- Registry padding with exited validators (task.md open question). Not
  needed at 100-validator sim scale.

---

## Step 4: Goldfish becomes the head vote

Design source: task.md decision 13, and the executable spec at
`../decoupled-consensus-networking/consensus-specs/specs/_features/simplex/fork-choice.md`
(`is_available_attestation_viable`, the per-round equivocation sets, and
phase 2 of `get_head`). Implement phase 2 only; stub the stable root as the
justified root.

### What exists and where it stops

The receive path validates and then drops. `validateAvailableAttestation`
(`beacon-chain/sync/validate_beacon_attestation.go:519`) does the slot,
single-signer, and signature checks and sets `msg.ValidatorData = att`
(`:621`); `availableAttestationSubscriber`
(`beacon-chain/sync/subscriber_beacon_attestation.go:66`) is a no-op TODO.
The mock committee is bidirectional already:
`decoupled.AvailableAttestationSeats` and
`AvailableAttestationSeatsToValidatorIndices`
(`decoupled/available_attestation_committee.go:31,41`).

### The five pieces

1. **The vote store.** `slot → validatorIndex → (root, payload_present)`
   plus a per-slot equivocation set, pruned at ~3 slots, fed by the
   subscriber from `msg.ValidatorData`. An equivocator counts in the
   viability denominator but scores no child (spec's rule). Restart and
   checkpoint-sync start with an empty store — see "cold starts" below.
2. **The walk.** Score bottom-up like `applyWeightChanges`: for each vote,
   walk from the voted block to the justified root once, adding the
   signer's *seat count* to each ancestor (the spec committee is a
   512-entry list with repeats; a validator with k seats counts k in score
   and denominator — quorums count seats here, not balance). Then descend
   top-down applying the majority gate per child. Cost: 512 × depth, not
   depth × children × 512.
3. **The passthrough.** `is_available_attestation_viable` returns true for
   a child at the current slot, except at a round-start slot
   (`slots.IsRoundStart`, `time/slots/slottime.go:175`). This is the
   proposer-boost replacement and the one thing not to get wrong.
4. **Turn off the old head inputs, gated on the Heze epoch:** proposer
   boost (`store.go:188-218`), the late-block reorg
   (`ShouldOverrideFCU` / `GetProposerHead`,
   `forkchoice/doubly-linked-tree/reorg_late_blocks.go:34,98` — two early
   returns), and the FFG→LMD input (the new walk simply never reads
   `f.votes`; `ProcessAttestation` keeps feeding the old store for pre-Heze
   configs). `AttestationDueBPS` stays 3333 — `arrivedEarly`
   (`node.go:40`) and other windows still read it as a window.
5. **Refresh and reporting.** The head is now a function of the clock
   (score and threshold read `previous_slot`), so recompute at the slot
   boundary (`NewSlot` + `UpdateHead` already run at t=0) and on block
   insert (`process_block.go` already calls `Head`); do not recompute per
   late vote — count late votes instead. `IsCanonical`
   (`forkchoice.go:184`) breaks because `bestDescendant` no longer
   describes the head chain: stamp a generation counter during the descent
   or walk parents from `headNode`. `CanonicalNodeAtSlot` (`gloas.go:24`)
   survives (parent walk); `FullHead`/`CachedHeadRoot` survive if the walk
   assigns `s.headNode` as `store.go:61` does today.

### Cold starts

Fresh start, restart, and checkpoint-sync all begin with an empty vote
store. With zero votes for the previous slot, no child clears the gate and
the walk stops at the justified root; the passthrough re-admits the current
slot's block, and one slot later the store is populated and the head
recovers. That one-slot retreat is acceptable for the mock, but it must not
crash, must not emit a zero-hash forkchoiceUpdated (the 5.0a class), and
must be pinned by a test for each of the three start modes. The e2e's
checkpoint-sync stage is the integration witness.

### Head retreats are correct

The gate is the reorg mechanism. If block N was late and slot N's voters
named N-1, block N loses the passthrough at slot N+1, fails the gate, and
the head retreats. `saveHead` counts that as a reorg
(`blockchain/head.go:130,151`) and the first devnet run will look alarming
unless the metric is decided first — see open question 1.

### Metrics (land with the walk, not after)

- Per slot: fraction of the 512 seats whose votes arrived before the next
  slot start.
- Counter: walk stopped at the gate (no child cleared the threshold).
- Counter: votes for slot N-1 arriving after the t=0 drain (late votes).
- Counter: gate-caused head retreats, distinct from `reorgCount`.

### Verify

Unit tests carry the correctness load (the walk is 1-2 sessions of work by
decision 13's estimate). Ladder: walk unit tests with hand-built trees
(gate passes, gate blocks, passthrough at round start vs mid-round,
equivocator counted in denominator only, seat multiplicity, empty store) →
single-node 3-slot smoke → `TestEndToEnd_HezeGenesisShort` →
full e2e (finalization proves the FFG path still justifies with LMD input
off) → nothing longer in this step.

---

## Step 5: the timing knobs

Decision 12, shrunk by Goldfish exactly as decision 17 predicted: the
late-block reorg and the head-weight guards are already off (step 4), so
what remains is the slot-start FFG vote, the target shift, and the knob
that moves the head vote.

### Which stream gets the knob

After step 4 the head-moving vote is the available attestation, so the
timing that changes head stability is `AVAILABLE_ATTESTATION_DUE_BPS_HEZE`
(`config/params/config.go:103`) — already a `spec:"true"` yaml value, so it
is sweepable per run with no code. The FFG attestation's timing changes
justification accounting and target matching only. Recommendation
(open question 2): both streams get a knob — the FFG vote gets the
slot-start flag with jitter, the available attestation keeps its BPS value
as the sweep axis — and the "which slot's block it names" knob applies to
the FFG vote only (the available attestation always names the current walk
head; that is its job).

### The three pieces

1. **Slot-start FFG vote.** A validator-client flag that skips
   `waitUntilAttestationDueOrValidBlock`
   (`validator/client/attest.go:39,281`) and adds jitter. Keep
   `AttestationDueBPS` at 3333 in the config (windows still read it).
2. **The target shift, ungated.** The FFG target becomes
   `StartSlot(E) - 1`. Four non-test `helpers.BlockRoot` callers
   (`core/altair/attestation.go:363`, `core/epoch/precompute/attestation.go:121`,
   `core/epoch/precompute/justification_finalization.go:164,172`), plus the
   forkchoice pair: `node.target` assignment (`store.go`, insert path) and
   `targetRootForEpoch` (`forkchoice.go:848`). **Epoch 0 underflows** —
   return the anchor root; genesis is epoch 0, so this is on the critical
   path of the first slot of every run. Two chains diverging at the epoch
   boundary then share one target root, FFG stops separating them, the head
   rule decides; surround and double-vote slashing still work at E+1, so
   accountable safety holds.
3. **Fallout, accepted:** a slot-start vote misses `is_matching_head` every
   slot (14/64 of the attestation reward) and, at epoch starts before the
   shift, `is_matching_target`. These are consensus-accounting values —
   allowed to be wrong by the task's charter. The e2e evaluators that
   assert participation or rewards must relax under the flag; wire
   behaviour (vote bytes, publish times, topics) is unchanged except the
   publish time itself, which is the point.

### Verify

Unit tests for the epoch-0 anchor fallback and the shifted target in each
of the six caller sites; the existing spectest exclusion stands. Ladder as
in step 4; the full e2e must still finalize with the flag on and off.

---

## Step 6: kurtosis and ethshadow

One binary, two harnesses, same measurements. The point of kurtosis is a
second, non-simulated witness (real wall-clock, real docker networking) for
the same topology; ethshadow remains the measurement instrument.

### Facts on this box (checked 2026-08-19)

- `kurtosis` CLI 1.18.1 at `/usr/bin/kurtosis` (1.20.0 available; upgrade
  only if the package demands it). Docker 29.6.2 works.
- `~/dev/ethereum-package` is an ethpandaops `ethereum-package` checkout:
  participants take `cl_image` and `cl_extra_params`
  (`network_params.yaml:20,25`). `~/dev/kurtosis` is an empty directory.
- Prysm's image tooling is push-oriented (`tools/prysm_image.bzl`,
  `prysm_image_upload` in `cmd/beacon-chain/BUILD.bazel:74`). For local
  kurtosis a plain Dockerfile over the `go build` binaries is simpler and
  avoids the gitless-checkout bazel quirks; recommended.

### The config injection problem

Every trap from the devnet/e2e/shadow work applies: `SLOTS_PER_ROUND: 8`,
`GLOAS_FORK_EPOCH: 0`, `HEZE_FORK_EPOCH: 0`, an explicit single-entry
`BLOB_SCHEDULE`, an Amsterdam-time EL genesis, and Heze stripped from any
CL→EL fork mapping (Heze is CL-only). In ethereum-package the CL config and
EL genesis come from the ethereum-genesis-generator image, so the injection
point is a **patched generator image** — the same pattern as
`decoupled-shadow-sim/Dockerfile.genesis-gen` — passed to the package via
its generator-image parameter. `cl_extra_params` cannot carry a
chain-config file that does not exist in the container, so it is not the
route. The validator must run gRPC (the REST client has no Gloas/Heze SSZ
block codec); verify what the package passes and override with
`vc_extra_params` if needed.

### Scale and measurements

Same shape as the recorded baseline: 10-16 nodes, ~100 validators, 5-6
epochs. Record in both harnesses: finalization cadence, available
attestations per slot per node, per-slot attestation counts and bytes, plus
the new step-4 metrics (late-vote fraction per slot, gate stops, gate
retreats). The ethshadow run reuses `decoupled-shadow-sim` (new data dir,
new run log, per the standing rules); the kurtosis run gets its own
workspace directory with its `network_params.yaml` committed.

### Verify

Ladder: `kurtosis run` with 2 nodes and ~3 slots of chain time as the
shakeout (startup problems live here: image, genesis, config injection) →
the full 10-16 node run in each harness → compare the two harnesses'
numbers against each other and against the recorded 5.4 baseline; the
Goldfish run should show the same attestation traffic and different head
dynamics, not different traffic.

---

## Open questions for the user

1. **The reorg metric.** Gate-caused head retreats are correct behaviour
   but `saveHead` counts them in `reorgCount`, so the first Goldfish run
   will look alarming. Recommendation: add a distinct
   `goldfish_gate_retreat` counter, leave `reorgCount` untouched but
   documented in the run notes. Deciding to *split* saveHead's accounting
   instead means touching a hot path for a cosmetic gain — not recommended.
2. **Which stream gets which knob.** Recommendation as in step 5: FFG vote
   gets the slot-start flag and the names-which-block knob; the available
   attestation's existing BPS config is the sweep axis for head timing.
   Alternative: give the available attestation a names-which-block knob too
   — defer until a measurement needs it.
3. **Payload-status tiebreaker.** Step 4 stubs it (PENDING for a same-slot
   vote, EMPTY otherwise, per decision 13). Promoting it to the spec's full
   rule is gadget-era work. Confirm the stub is acceptable for the first
   Goldfish measurements.
4. **Kurtosis image route.** Recommendation: plain Dockerfile over go-built
   binaries plus a patched genesis-generator image, mirroring
   `Dockerfile.genesis-gen`. The bazel `prysm_image_upload` route needs a
   registry and fights the gitless checkout.
