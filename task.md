# Task: Decoupled consensus networking mock

## Goal

Measure the network cost of decoupled consensus (Goldfish) in Prysm. The consensus
values can be wrong. The wire behavior must be real: bytes, message counts, publish
times, topics, and gossip verdicts.

## State: send path is complete

The send path is in jj change `vonqrnkz` ("Add available attestation send path").
The message is `AvailableAttestation`: 64-byte seat bits (Bitvector512), data
(slot, payload_present, beacon_block_root), and a 96-byte signature. Total: 201
bytes. It goes on one global gossip topic: `available_attestation`.

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
9. Decided 2026-08-15: duty assignment moves to the beacon node. A new
   submessage `DecoupledDuty` goes on `AttesterDuty` and on
   `ValidatorDuty`. It holds the available-attester slots and the
   finality-vote assignment. The attester response carries it, so the
   fetch, the missing-next mask, the dependent root, and promotion all
   come free. After Heze the node still returns one `AttesterDuty` entry
   per validator, with the attester fields empty and `DecoupledDuty`
   set. `buildNextDuties` copies the submessage into `ValidatorDuty`.
   `RolesAt` only reads the snapshot; the client-side Heze check and
   seat computation go away; the `decoupled` package stays node-only;
   the node stops setting `AttesterSlot` after Heze (kills the
   TODO(goldfish) wipe). Data RPCs return the unsigned vote with seat
   bits set, so the client only signs. gRPC only: the REST client
   methods panic, as with the available attestation.
