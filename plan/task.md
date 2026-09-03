# Task: Decoupled consensus networking mock

## Goal

Measure the network cost of decoupled consensus (Goldfish) in Prysm. The consensus
values can be wrong. The wire behavior must be real: bytes, message counts, publish
times, topics, and gossip verdicts.

## State: send path is complete

The send path is in jj change `vonqrnkz` ("Add available attestation send path").
The message is `AvailableAttestation`: 64-byte seat bits (Bitvector512), data
(slot, payload_present, beacon_block_root), a 96-byte signature and the
variable-length `scratch_space`. Total: 205 bytes plus the scratch length. It
goes on one global gossip topic: `available_attestation`.

How the send path works:

- The committee is a mock. `decoupled/available_attestation_committee.go` maps a
  slot and a validator index to seats. There are 512 seats. A validator with many
  seats signs once and sets all its seat bits.
- The validator client gives the role `RoleAvailableAttester` when: the validator
  is active, the epoch is at or after `HezeForkEpoch`, and the validator has seats.
- The runner calls `SubmitAvailableAttestation`. It waits until 25% of the slot
  (`AvailableAttestationDueBPSHeze`) or until a valid block comes.
- The client gets vote data over the `GetAvailableAttestationData` RPC. It caches
  the data for each slot. On the node, the core service calls
  `ForkchoiceFetcher.CanonicalNodeAtSlot`. This gives the head root and the
  payload_present bit. A same-slot head always gives payload_present = false.
- The client signs with a mock domain: sha256 of
  "decoupled-mock-available-attestation". The local keymanager only signs the
  root, so no real domain is necessary.
- The client sends the vote over the `ProposeAvailableAttestation` RPC. The node
  checks the signature format, the Heze gate, and the same-slot payload rule.
  Then it broadcasts with the generic `P2P.Broadcast`. The topic mapping is in
  `beacon-chain/p2p/topics.go` and `gossip_topic_mappings.go`.

Tests: seat coverage in `decoupled/`, a client happy-path test, and server tests
for the two RPC handlers with `MockBroadcaster`. The gomock files are regenerated.

Rules for the session: use ASD-STE100. Keep replies short. The user writes the
code. Claude reviews and guides. Claude makes small fixes only on request. Do not
read or refer to the old implementation commits. Build each piece fresh.

## Current task: receive path

Done, in the current jj change:

1. `decoupled/`: the inverse function `AvailableAttestationSeatsToValidatorIndices`.
   It maps seat bits to validator indices, sorted, with no duplicates. Tests:
   round trip for each validator, and a union of many validators.
2. `beacon-chain/sync/`: the gossip validator `validateAvailableAttestation`.
   It checks the slot is current with a tolerance for early votes only. Late
   votes get a hard cutoff: a vote is useless after its slot, so clock disparity
   does not apply on the stale side. It resolves the state with forkchoice:
   block root -> target root for the epoch -> checkpoint state. This never
   replays blocks. It requires the seat bits to resolve to exactly one signer.
   It verifies the signature with the mock domain through the shared batch
   verifier. Verdicts are real: ACCEPT, IGNORE, REJECT. Tests cover all nine
   paths.
3. The topic is registered at the Heze fork in `subscriber.go`, and the decoder
   mapping is in `gossip_topic_mappings.go`.

Decision: no seen-cache and no per-signer dedup. Gossipsub dedups bytes; a
per-signer cap is only a rate limit. We keep every vote, so equivocation
evidence stays complete for a future slashing path. Two conflicting votes are a
full slashing proof; production can cap at two per signer later. The simulation
sends one vote per signer, so message counts do not change.

Deferred 2026-08-14: items 1 to 3 move into the simplex work. The store
shape depends on how the Goldfish gate reads it, so we design it at use time.

Done 2026-08-15: the plan doc `decoupled-consensus-plan.html` states the
decided plan: the real gadget, no parallel FFG run, the cutover at the
fork, and the cloned attestation path for the finality vote. The four old
HTML files are removed; jj history keeps them.

Left to build on the receive path:

1. In `beacon-chain/cache/`: a plain store. Key: slot. Value: list of (vote,
   signer index). The subscriber appends. Prune slots older than two slots on
   insert. Add a getter. The Goldfish gate reads this store (decided; see below).
2. Pass the signer index from the validator to the subscriber. Wrap the vote in
   a small struct on `msg.ValidatorData`, so the subscriber does not resolve
   the seats again.
3. The subscriber writes the vote to the store. Add a test.
4. Done 2026-08-14: the `gossip_scoring_params.go` entry. New
   `defaultAvailableAttestationTopicParams`, with the aggregate-topic math,
   rate = 512 messages per slot, and 1-epoch decays. Exact tuning is a
   production concern, not a simulation concern.
5. Cleanups in `validate_beacon_attestation.go`: remove the duplicate
   `eth`/`ethpb` import, fix the two copied span names, rename the `state`
   variable that shadows the package, remove the dead commented blocks.

## Direction change, 2026-08-14: build the real finality gadget

The traffic-only mock is replaced. New target: implement the finality gadget
(Fresh Simplex with height filter and timeouts) with real vote content on the
real beacon state. The simulation then measures time to finality, timeout
rounds under loss, and stalls under partition. Traffic measurement comes free.

References: `decoupled-consensus-plan.html` (the plan) and the executable
specification — the wire authority — at
`../decoupled-consensus-networking/consensus-specs/specs/_features/simplex/`.

Rejected:

- `SlotsPerEpoch = 8` with `AttestationDueBPS = 0`. FFG finalises on a fixed
  cadence, so it cannot measure finality. A slot-start attestation votes
  before the block, each head vote names the parent, and the late-block reorg
  logic then tries a reorg each slot.
- Two-tier genesis. All registry validators are live signers.

Decisions:

1. Remove today's attestations. Two streams replace them: the available
   attestation (head vote; waits for the block or 25% of the slot; built) and
   the finality vote (slot start; fields freeze at the round start). The
   slot-start collision with the block is a primary measurement.
   Decided 2026-08-14: no parallel run. Before the Heze fork, FFG runs alone.
   At the fork, the attestation duty stops and the two new duties start. The
   finality vote clones the attestation path: types, subnet and aggregate
   topics, duties, and the block-body list. The seen-cache key changes from
   epoch to round.
2. The Goldfish gate is the head input: score children with the previous
   slot's votes and apply the majority gate. Without it, all weights are zero.
3. Build order (revised 2026-08-15: networking first, then the middle).
   This is a code order, not a runtime cutover order. Finality is not
   measurable on a devnet until (e) is in; unit tests carry the
   correctness load. Skip view-merge and TSQ.
   (a) Round math: `compute_round_at_slot` and its inverse. A sub-round is
   one slot; a round is 8 slots. The duty and the seen-cache need it first.
   (b) Finality vote message with dummy content, on the cloned attestation
   path: types, subnet and aggregate topics, block-body list. Seen-cache
   key is round, not epoch.
   (c) Send path: duty at slot start, fields frozen at round start, one
   vote per validator per round. Dummy values until (e).
   (d) Receive path: gossip validators with real verdicts. Structural,
   timing, and signature checks only; semantic checks against gadget
   state arrive with (e).
   (e) The middle: new store fields beside the FFG fields (no check that
   compares the gadget with FFG: after the fork, FFG gets no votes);
   clockless state machine driven from `handleBlockAttestations`,
   branch order unit-tested; the four interface swaps: anchor cascade,
   height filter, lexicographic updates, prune from `store.F`. The FFG
   code path serves pre-fork slots only.
4. Quorums count validators.
5. `targets` and `timeouts` are sparse maps.
6. Open: pad the registry with exited validators for mainnet state size.
7. After the fork, only the gadget prunes. Until 3e lands, nothing prunes
   and memory grows; keep runs short. If the gadget stalls, pruning stops;
   watch for it.
8. The Goldfish gate must be live at the fork, or all head weights are zero.
9. Decided 2026-08-15: duty assignment moves to the beacon node, inside
   the attester duty. The committee fields are reused: before Heze they
   describe the FFG attestation, after Heze the finality vote. The
   duty's epoch is the switch; one proto comment states it. After Heze
   `AttesterSlot` stays empty (kills the TODO(goldfish) wipe); the vote
   slots come from the round math. One new repeated field on
   `AttesterDuty` and `ValidatorDuty`:
   `AvailableAttestationDuty { slot, []seats }`, one entry per slot
   with seats. The attester response carries everything, so the fetch,
   the missing-next mask, the dependent root, and promotion all come
   free; after Heze the node still returns one entry per validator.
   `buildNextDuties` copies the new field across. `RolesAt` only reads
   the snapshot; the `decoupled` package stays node-only. The seats
   come from the duty, so the data RPC turns per-slot and
   validator-agnostic: it returns head root and payload_present; the
   client sets its seat bits and signs. gRPC only: the REST client
   methods panic, as with the available attestation.

## Session 2026-08-18: slot-start FFG vote, rounds, and genesis at Heze

This session made design decisions only. No code changed.

### What this session supersedes

Three earlier items are now reversed. The old text stays above for history.

- The "Rejected" item `SlotsPerEpoch = 8 with AttestationDueBPS = 0` is
  partly reversed. The slot-start vote is back, as a flag. The reasons the
  item gave were correct; the fix is in decision 12.
- "Round is equal to epoch by config" is reversed. See decision 10.
- Heze as a consensus-only fork is reversed. See decision 11.
- Build order item (a) said "a round is 8 slots" as a consequence of
  8-slot epochs. A round is now 8 slots by its own config value.

### Decision 10: revert the 8-slot preset, then add a round type

The 8-slot epoch is a hack. `SLOTS_PER_EPOCH` is overloaded: it sets the
FFG accounting period, the committee sizes, the shuffling, the inclusion
window, and six SSZ sizes. The round needs none of that.

The decoupled preset differs from mainnet in six values only. All six come
from `SLOTS_PER_EPOCH`. See `proto/ssz_proto_library.bzl`:

| field | mainnet | decoupled |
|---|---|---|
| `eth1_data_votes.size` | 2048 | 512 |
| `previous_epoch_attestations.max` | 4096 | 1024 |
| `current_epoch_attestations.max` | 4096 | 1024 |
| `proposer_lookahead_size` | 64 | 16 |
| `ptc_window.size` | 96 | 24 |
| `builder_pending_payments.size` | 64 | 16 |

The hack also biases the primary measurement. At 8-slot epochs each
validator attests 4 times more often per second than on mainnet. Per-slot
attestation traffic is 4 times mainnet. The goal of this project is to
measure network cost, so this is a bias in the main metric, not a
cosmetic problem. `SqrRootSlotsPerEpoch` is also 2 instead of 5.

Do this instead:

1. Revert the preset. Delete the `decoupled` dict in
   `proto/ssz_proto_library.bzl`, the `.decoupled.pb.go` and
   `.decoupled.ssz.go` twins, the `-tags=decoupled` build tag,
   `DecoupledConfig()` in `config/params/decoupled_config.go`, and the
   `VerifyPreset` tag check. Update the e2e and sim yaml files.
   Regenerate for mainnet only. This is mostly deletion.
2. Add the round. `SLOTS_PER_ROUND` sizes nothing in SSZ, so this is a
   config value and a type. No regeneration.

`SLOTS_PER_ROUND` must be a distinct type, not an alias of `Epoch`. The
spec makes it a per-era scheduled value and requires it to divide
`SLOTS_PER_EPOCH`. So a round is a divisor of an epoch, not a synonym.
Round length is the main knob for finality latency, so the two values will
diverge. An alias gives the compiler nothing and lets an epoch/round mix-up
compile. A distinct type catches the mix-up.

Skip the era. The spec's `ROUND_SCHEDULE` is a list of scheduled
`SLOTS_PER_ROUND` changes with `START_ROUND` bookkeeping. It has the same
shape as `BlobSchedule`. Use one config value instead.

Revert before you add the round. If `SlotsPerRound` and `SlotsPerEpoch`
are both 8, an epoch/round mix-up is invisible. At 32 and 8 it breaks
immediately, while you build the round consumers.

Work: revert is half a session. The round type is 2 to 3 hours.
- `primitives.Round`, a distinct type, in `consensus-types/primitives/`.
  `Epoch` is 152 lines and 14 methods; a subset is enough.
- `slots.RoundAt` and `slots.RoundStart`, as mirrors of `ToEpoch` and
  `EpochStart`.
- `cfg.SlotsPerRound`, a config value, not a preset value. Eight files
  take one line each: `config.go`, `mainnet_config.go`,
  `minimal_config.go`, the print list in `loader.go`, the assert list in
  `loader_test.go`, and `rpc/eth/config/handlers.go` with its test.
- One check that `SlotsPerRound` divides `SlotsPerEpoch`. The spec
  requires it.

Cost of the revert: epoch-scale events get 4 times slower in a sim run. At
12-second slots a 32-slot epoch is 6.4 minutes. A 30-minute run gives 4.7
epochs and 2 to 3 finality events, not 9. This is enough to show that the
chain still finalizes. It is thin for "time to finality under loss". The
cost is temporary: when the gadget lands, finality is round-based and fast
again. Do not shrink `SECONDS_PER_SLOT` to compensate. Slot duration is a
realism parameter for a network-cost study.

### Decision 11: Heze owns its containers, and genesis is Heze

Heze stops being consensus-only. It gets its own containers, copied from
Gloas. Gloas is never crossed as a fork. Genesis is Heze.

Why. Two reasons.

1. The spec puts a new list in the block body:
   `round_double_vote_evidence: List[RoundDoubleVoteEvidence,
   MAX_ROUND_DOUBLE_VOTE_EVIDENCE]`. So the block body must change at
   Heze. Build order item (b) also needs a block-body list.
2. `HezeShape()` returns one entry for both state and block shapes. Its
   three consumers are `p2p/types/object_mapping.go:279`,
   `sync/context.go:98`, and `encoding/ssz/detect/configfork.go:96`. A
   partial divergence, where blocks change but state does not, cannot be
   expressed. Full ownership deletes `HezeShape()` and returns those three
   sites to the ordinary ladder.

Duplicate the state as well, even though nothing planned changes it.
"Heze owns its shapes" is one rule. "Heze owns blocks but borrows state"
is two.

Set both `GloasForkEpoch = 0` and `HezeForkEpoch = 0`. This is required,
not optional. 86 non-test sites compare against `GloasForkEpoch`, not
against the version enum. Examples: `blockchain/process_block.go:819`,
`blockchain/process_attestation.go:104`,
`execution/engine_jsonrpc.go:282`. If Gloas stays at `MaxUint64`, all 86
go false, and the node runs Gloas-shaped containers with every Gloas
behaviour off. It compiles and starts and is quietly wrong. With both at
0, all 86 are true from slot 0.

The schedule already supports two forks at one epoch. See the tie-break
comment at `config/params/config.go:525`: "both entries are forks in a
test setup (eg starting genesis at a later fork)". It sorts by version
enum, so Gloas sorts first and `forEpoch(0)` returns Heze.

Work:
- Heze containers, copied from Gloas. Add a `case version.Heze` arm to the
  38 `case version.Gloas` sites. The 73 `>= version.Gloas` ladders keep
  working, because Heze is above Gloas in the enum. There are 0
  `== version.Gloas` sites. state-native is the bulk: 10 files, 223
  mentions.
- Delete `HezeShape()`. Change `detect` to
  `case cfg.HezeForkVersion: fork = version.Heze`.
- Remove Heze from `unsupportedVersions` in `runtime/version/fork.go:40`.
- Add a `version.Heze` case to `runtime/interop/premine-state.go`, so
  `--fork-name heze` builds genesis directly. The switch at line 69 tops
  out at Fulu today. The Heze case needs `initializePTCWindow` and
  `OnboardBuildersFromPendingDeposits`, which both exist in
  `beacon-chain/core/gloas/upgrade.go`.
- The 86 `GloasForkEpoch` sites need no edit.

Gloas's containers and its 38 switch arms stay as dead code. This keeps
upstream rebases possible. The user accepts dead code; it must only not
get in the way.

We do not test the fork transition. `UpgradeToHeze` can stay unimplemented
for a real state change. We also give up the pre-Heze FFG-only baseline
that decision 1 assumed. This is accepted.

Do decision 11 after decision 10. The revert means the container work
generates one preset of twins, not two.

### Decision 12: the FFG vote goes out at slot start

Add a flag. The validator client skips
`waitUntilAttestationDueOrValidBlock` and adds jitter.

Keep `AttestationDueBPS` at 3333 in the config. Do not set it to 0. Two
places read it as a window: `arrivedEarly` in
`forkchoice/doubly-linked-tree/node.go:42` and the proposer-boost window
in `store.go:186`. At 0 every block is late, and the reorg logic fires
every slot.

Three things break at slot start. They have very different costs.

1. The target root at the first slot of an epoch. The vote is internally
   consistent: `targetRootForEpoch` returns the node's own root when
   `epoch > nodeEpoch` (`forkchoice.go:855`). So gossip,
   `VerifyLmdFfgConsistency`, and forkchoice all accept it. It only misses
   `is_matching_target` in `core/altair/attestation.go:363`.
2. `is_matching_head` fails for every attestation in every slot. The head
   flag is 14/64 of the attestation reward. Proposer rewards drop with it.
   Any e2e evaluator that asserts head participation needs to relax.
3. Forkchoice. Prysm applies attestations one slot late
   (`receive_attestation.go`: "don't process the attestation until the
   subsequent slot"). So the current slot's block always has weight 0. The
   head-weight guard in `ShouldOverrideFCU` and `GetProposerHead` becomes
   dead code, and only `arrivedEarly` stops an orphan. Disable the
   late-block reorg under the flag. Two early returns.

Block building still works. At slot N the head walk finds block N-1 as
the only child of N-2, and descends to it whatever its weight. See
`updateBestDescendantPayloadNode` in
`forkchoice/doubly-linked-tree/gloas.go:195`.

Shift the target to `StartSlot(E) - 1`. Do it, and do it ungated.
`helpers.BlockRoot` has 4 non-test callers: `core/altair/attestation.go`,
`core/epoch/precompute/attestation.go`, and two in
`core/epoch/precompute/justification_finalization.go`. The shift is sound:
two chains that diverge at the epoch boundary share one target root for
epoch E, so FFG stops separating them and the head rule decides. Surround
and double-vote slashing still work at E+1, so accountable safety holds.

Genesis at Heze removes the gate. Every state is Heze from slot 0, so
there is no fork check and no config knob. There are no Heze spectests,
so there is no spectest risk. The `TARGET_SLOT_OFFSET` config value from
the earlier plan is not needed.

Still needed for the shift:
- The forkchoice target: the `node.target` assignment at
  `forkchoice/doubly-linked-tree/store.go:126` and `targetRootForEpoch` at
  `forkchoice.go:848`.
- Epoch 0. `StartSlot(0) - 1` underflows. Return the anchor root. This
  gives the same answer as today. It is now on the critical path, because
  genesis is epoch 0. It is easy to forget and it fails at slot 0 of the
  first run.

If we do not shift the target, the cost is small at 8-slot epochs and
large at 32. Slot-0 attesters lose their target vote. At 8-slot epochs
that is 1/8 of the weight, so 87.5% is left and the chain finalizes. At
32-slot epochs it is 1/32. Decision 10 moves to 32-slot epochs, so the
loss gets smaller. The shift is still the correct fix.

Work: the flag and the forkchoice guards are half a session. The target
shift is under one session, now that the gate is gone.

### Decision 13: Goldfish becomes the head vote

This is a good test rig. The gate reads the previous slot's votes. If
those votes are late, no child clears the threshold, the walk stops at the
parent, and the head stalls. Late delivery becomes visible.

It does not need the finality gadget. Stub the stable root as the
justified root. Skip TSQ and the height filter. Implement phase 2 of
`get_head` only. The spec is at
`../decoupled-consensus-networking/consensus-specs/specs/_features/simplex/fork-choice.md`.

Work:
1. The vote store. `availableAttestationSubscriber` in
   `sync/subscriber_beacon_attestation.go:66` is still a no-op TODO, so
   validated votes are dropped now. Store `slot -> validatorIndex ->
   (root, slot, payload_present)`, plus an equivocation set. The spec
   counts an equivocator for viability but for no child. Prune at about 3
   slots. Pass the signer indices through `msg.ValidatorData`. This is
   receive-path items 1 to 3 above.
2. Skip committee pinning. `cache_available_committee` exists in the spec
   because the real committee depends on the walk head. Ours is a mock and
   depends on the slot only. Keep seat multiplicity: the spec's committee
   is a 512-entry list with repeats, so a validator with k seats counts k
   in the score and in the denominator. This deviation stops being safe if
   the committee ever depends on the head.
3. The walk. Score bottom-up, like `applyWeightChanges`: for each of the
   512 or fewer votes, walk from the voted block up to the justified root
   once and add to a per-node score. Only the gated descent is top-down.
   Naive per-child scoring costs depth times children times 512 ancestry
   walks; bottom-up costs 512 times depth.
4. The current-slot passthrough is the proposer-boost replacement.
   `is_available_attestation_viable` returns true for a child at the
   current slot, except at a round-start slot. A fresh block has score 0
   by construction. Implement this before you delete proposer boost, or
   the chain never extends.
5. Turn off proposer boost, the late-block reorg, and the LMD input from
   FFG attestations. For the last one, do not read `f.votes` in the new
   walk. Leave the old path for pre-fork slots.

Trap: `ProcessAttestation` at `forkchoice/doubly-linked-tree/forkchoice.go:110`
updates a vote only when `targetEpoch > nextEpoch`. That is epoch
granularity, and votes never expire. Goldfish votes are per-slot and
expire after one slot. Do not reuse `f.votes`. Separate store, separate
walk.

Refresh. The head is now a function of the clock, because the score and
the threshold read `previous_slot = current_slot - 1`. It changes at a
slot boundary with no new input. So recompute it; do not maintain it.
Three triggers:
- The slot boundary at t=0. This one is new in effect. `NewSlot` plus
  `UpdateHead` already runs there.
- A block insert. `process_block.go:102` already calls `Head`.
- Late votes for slot N-1 that arrive during slot N. Do not recompute per
  vote. Keep the t=0 and slot_end-2s cadence, and count the votes that
  arrive after the t=0 drain. That count is a measurement.

`IsCanonical` at `forkchoice/doubly-linked-tree/forkchoice.go:182` breaks.
It compares `bestDescendant` pointers, and `bestDescendant` no longer
describes the head chain. Stamp a generation counter during the descent,
or walk parents from `headNode`. `CanonicalNodeAtSlot` at `gloas.go:24`
survives, because it walks parents. `FullHead` and `CachedHeadRoot`
survive if the walk assigns `s.headNode`, as `store.go:61` does.

The gate is the reorg mechanism, so the head can move backwards. If block
N was late and slot N's voters named N-1, then at slot N+1 block N loses
the passthrough, fails the gate, and the head retreats. This is correct
behaviour. But `saveHead` counts each one as a reorg. Decide the metric
before the first devnet run, or the first run looks alarming.

Two metrics to add with the walk:
- Per slot, the fraction of the 512 seats whose votes arrived before the
  next slot start.
- A counter for "the walk stopped at the gate".

Work: the vote store is half a session. The walk with unit tests is 1 to 2
sessions, and holds the correctness load. Wiring and the flag are small.
Stub the payload-status tiebreaker at first: PENDING for a same-slot vote,
EMPTY otherwise. Metrics and a devnet run are a day of wall clock.

### Decision 14: adversarial knobs

"What if 20% of validators vote empty" already works end to end, and
decision 11 turns it on at genesis.

The payload vote rides the attestation's `CommitteeIndex`. The node
encodes it at `rpc/core/validator.go:610`: 1 is full, 0 is empty.
`blockchain/process_attestation.go:104` decodes it and passes
`payloadStatus` to `ProcessAttestation`. With `GloasForkEpoch = 0` this is
live from slot 0.

Empty is the direction with no resistance. `attester.go:345` rejects a
same-slot `payload_present`, and `process_attestation.go:107` drops a
same-slot payload-present attestation. Both guards point one way. Nothing
rejects an empty vote, because it is a legitimate claim.

To inject it, put a flag on the beacon node that forces
`isPayloadFull = false`. Then give the flagged nodes 20% of the keys. This
is better than a fraction on the validator client: one boolean, no
arithmetic, and the stake split is a property of the key distribution.

Use one enum, not a flag per behaviour:
`--decoupled-misbehavior=empty|silent|lag|equivocate|parent-vote`.
`equivocate` already works on the receive side, because we keep no
seen-cache and no per-signer dedup, so conflicting votes survive.

What this measures now, and what it does not. Now: empty votes against LMD
weight, through `choosePayloadContent` and `shouldBuildOnFullLocked`. The
useful number is the crossover fraction at which the payload gets
orphaned. Sweep the key split to find it. Later: the same question against
the Goldfish gate, which has a different threshold. The available
attestation's `payload_present` goes nowhere until decision 13 lands. Be
clear about which stream a result describes.

### Order of work

1. Revert the 8-slot preset (decision 10, part 1).
2. Add `primitives.Round` and `SlotsPerRound` (decision 10, part 2).
3. Heze owns its containers; `GloasForkEpoch = HezeForkEpoch = 0`; genesis
   at Heze (decision 11).
4. Target shift, slot-start FFG vote, disable the late-block reorg
   (decision 12).
5. Goldfish head rule (decision 13).

The adversarial knobs (decision 14) need step 3 only. The 20%-empty
experiment can run before step 4.

### Open questions

- Is `SLOTS_PER_ROUND = 8` the value we want, with 32-slot epochs? It must
  divide 32.
- The `execution_payload_availability.size` value differs between mainnet
  and the decoupled preset (8192 against 1024) and is not obviously
  derived from `SLOTS_PER_EPOCH`. Check it during the revert.
- `runtime/interop/premine-state.go` has switches at lines 69, 106, 344,
  and 444. Confirm the count of arms a `version.Heze` case needs.

## Session 2026-08-19: rounds are real, and genesis builds Heze directly

Design decisions only. No code changed.

### The three open questions from 2026-08-18 are closed

- `execution_payload_availability.size` does not diverge. The field is
  `Bitvector[SLOTS_PER_HISTORICAL_ROOT]` (`gloas.proto:251`) carried as
  `bytes` (`:380`), so its `ssz_size` is a byte count: 8192/8 = 1024 for
  mainnet and for the decoupled preset, 64/8 = 8 for minimal. The `8192`
  in the question was `block_roots.size`, one line away in the same dict.
  Nothing to check during the revert.
- `SLOTS_PER_ROUND` is 8, with 32-slot epochs. The spec's mainnet value of
  32 is the identity setting: its own comment calls it "one epoch-length
  round (= SLOTS_PER_EPOCH), i.e. pre-fork (Gloas) behavior". A round equal
  to the epoch gives no latency gain, so it measures nothing.
- `premine-state.go` has five switches, at lines 68, 105, 343, 443, and
  628. Each needs one `version.Heze` arm. The fifth, `setExecutionPayload`,
  holds the only real work. There is no Gloas arm to copy: all five stop at
  Fulu, so Gloas-shaped genesis has never been written.

### Decision 15: rounds are real, and they change the committee shuffle

Decision 10 said `SLOTS_PER_ROUND` "sizes nothing in SSZ, so this is a
config value and a type". True for SSZ, wrong for everything else. Simplex
modifies two committee helpers:

- `get_committee_count_per_slot` divides the active set by
  `SLOTS_PER_ROUND`, not `SLOTS_PER_EPOCH` (`beacon-chain.md:1161`).
- `get_beacon_committee` uses `slot_in_round`, and
  `count = committees_per_slot * slots_per_round` (`:1183`).

The prose states the intent: "Simplex repeats the beacon-committee shuffle
in every round of an epoch. A validator therefore requests one assignment
per round, rather than one assignment per epoch" (`validator.md:222`).

So at 32/8 each validator votes 4 times per epoch, and per-slot attestation
traffic is 4 times mainnet. At `MAX_COMMITTEES_PER_SLOT = 64`:

| | mainnet | 8-slot preset | 32 epoch / 8 round |
|---|---|---|---|
| attesters per slot (N=1M) | 32,768 | 131,072 | 131,072 |
| committee size | 512 | 2048 | 2048 |
| sim, N=512, per slot | 16 | 64 | 64 |

**Correction to decision 10.** Its traffic argument does not separate the
two options. The 4x it blamed on the 8-slot preset comes back unchanged
from the 8-slot round, and this time it is intended: it is the cost of a
short round, which is the number this project exists to measure. The revert
is still correct for its other reasons — SSZ sizes, the shuffling period,
the inclusion window, `SqrRootSlotsPerEpoch`, and the overload of one
constant.

`SlotsPerRound = SlotsPerEpoch` is the identity setting. With them equal,
`slot_in_round == slot % SlotsPerEpoch` and
`count == committeesPerSlot * SlotsPerEpoch` — today's math exactly. The
spec's own configs set that: mainnet 32, minimal 8. So set mainnet 32 and
minimal 8, and every spectest and committee test stays green with no
expectation edits. Only the devnet and sim config sets 8 against a 32-slot
epoch.

Work:
- `primitives.Round`, a distinct type. `slots.RoundAt`, `slots.RoundStart`.
- `cfg.SlotsPerRound`, plus the check that it divides `SlotsPerEpoch`.
- `SlotCommitteeCount` (`beacon_committee.go:55`): divide by `SlotsPerRound`.
- `BeaconCommittee` (`:242`): slot-in-round instead of
  `ModSlot(SlotsPerEpoch)`, and `count *= SlotsPerRound`.
- The committee cache is keyed by (slot, seed, index) and needs no change.

Trap: `hasSeenAggregatorIndexEpoch` (`validate_aggregate_proof.go:259`)
keys the aggregator dedup on the epoch. At 4 rounds per epoch a validator
can aggregate in more than one round, and the later aggregates get dropped
as duplicates. That key becomes the round. This is decision 1's "seen-cache
key changes from epoch to round", and that is the site. The unaggregated
cache is already keyed by (slot, committee index, attester)
(`validate_beacon_attestation.go:457`) and is fine.

### Decision 16: genesis builds a Heze state directly

We never upgrade to Heze. Genesis is always Heze. So `heze.UpgradeToHeze`
is deleted, and `HezeShape()` with it. `UpgradeToGloas` stays as uncalled
upstream code, like the Gloas switch arms.

This supersedes decision 11's note that `UpgradeToHeze` "can stay
unimplemented". It is not unimplemented; it is gone.

The `version.Heze` arm fills the eight Gloas state fields
(`gloas.proto:373-390`). Six are trivial:

- `ptc_window` from `initializePTCWindow` (`gloas/upgrade.go:156`).
  Unexported today; export it. Its spec docstring says it is "used to
  initialize the `ptc_window` field in the beacon state at genesis and
  after forks", so genesis is its intended second caller.
- `builders` from `OnboardBuildersFromPendingDeposits()`, already exported.
- `builder_pending_payments`: a fixed vector of `2 * SLOTS_PER_EPOCH`.
  Allocate it. A nil value fails the SSZ round trip.
- `execution_payload_availability`, `builder_pending_withdrawals`,
  `payload_expected_withdrawals`, `next_withdrawal_builder_index`: zero
  and empty.

So we call the two helpers that `UpgradeToGloas` calls, without calling it.

The one real piece of work: Gloas removes `latest_execution_payload_header`
and replaces it with `latest_block_hash` (`gloas.proto:325`) and
`latest_execution_payload_bid` (`:380`). Premine already builds a payload
header from the geth genesis block, so every input exists. The arm builds
the two new fields from the same block. `upgradeToGloas:207` and `:312`
show the field mapping.

Values we pick, because the Gloas spec has no `initialize_beacon_state`:
`execution_payload_availability[0] = 1`. Slot 0 has a payload, the slot-1
transition would set that bit anyway (`beacon-chain.md:1169`), and
`beacon-chain.md:1395` asserts
`latest_block_hash == latest_execution_payload_bid.block_hash`, which holds
at genesis only if slot 0 counts as available.

Also: remove Heze from `unsupportedVersions` (`runtime/version/fork.go:40`).
`--fork-name heze` then works through `version.FromString`.

`GloasForkEpoch = HezeForkEpoch = 0` stands, for the reason decision 11
gave: 86 non-test sites compare against `GloasForkEpoch`, not the version
enum.

### Decision 17: revised order of work

This replaces the order at the end of the 2026-08-18 section.

0. Revert the 8-slot preset. First, because every proto generates one twin
   per preset — there are 16 `.decoupled.*` files today — so the container
   work in step 2 would otherwise regenerate three presets instead of two.
1. `primitives.Round` and `SlotsPerRound`, the two committee helpers, and
   the aggregator seen-key. Identity work everywhere until the devnet
   config flips to 8.
2. Heze state, copied from Gloas.
3. Genesis at Heze: premine builds the Heze state directly.
4. Goldfish: block proposing and forkchoice.
5. The timing knobs: move the finality vote freely in the slot, and choose
   which slot's block it names.

Steps 0 and 1 are one session together.

Why this beats the 2026-08-18 order: doing Goldfish before the timing work
removes most of decision 12's breakage. Two of its three problems — the
late-block reorg firing every slot, and the head-weight guards going dead —
exist only because FFG attestations feed LMD. Step 4 already turns that
off. So step 5 shrinks to the target-root shift and the participation
flags.

The Goldfish passthrough needs one round fact: a fresh block at the current
slot is viable, except at a round-start slot. Step 1 supplies it, so it is
not stubbed.

### Open questions

- Step 5: which stream gets the timing knob? After step 4 the FFG
  attestation no longer selects the head, so moving it changes
  justification accounting and target matching only. The vote whose timing
  moves the head is the available attestation. Possibly both need the knob.
- `builder_pending_payments` is sized `2 * SLOTS_PER_EPOCH` and stays
  epoch-sized. Confirm nothing in the Gloas payment path assumes the round.
