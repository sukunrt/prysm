# Bug: REST aggregation duty 404s ("No attestations to aggregate")

## Symptom

`WARN client: No attestations to aggregate error=read until: get: HTTP request unsuccessful
(404: No matching attestations found)` — REST VCs only.

| Run | Transport | 404s | Successes | Note |
|---|---|---|---|---|
| final-02 vc-1/2/3 | REST | 96 / 97 / 96 | 25 / 26 / 26 | ~80% of ~122 aggregation duties fail |
| final-02 vc-4/5 | gRPC | 0 / 0 | 122 / 123 | aggregate every slot |
| resttest2-01 vc-1..5 | REST | 12 each | 51–53 each | older build, different vote timing (below) |
| shadow data62 | REST | 1 (slot 25, 8 of 10 VCs) | most slots | Shadow blocks import ~2–30 ms into slot |

gRPC VCs never log the equivalent (`Could not find attestation for slot and committee in pool`,
beacon-chain/rpc/prysm/v1alpha1/validator/aggregator.go:55,101, surfaced through the same WARN in
validator/client/aggregate.go:268-284): zero occurrences in vc-4/vc-5.

## Call path

REST:
1. `SubmitAggregateAndProof` — validator/client/aggregate.go:28. Waits to 50% of slot
   (`waitUntilAggregateDue`, aggregate.go:207, `AggregateDueBPSGloas=5000`), sends only
   slot + committeeIndex + pubkey + slotSig (aggregate.go:66-75).
2. `beaconApiValidatorClient.SubmitAggregateSelectionProofElectra` —
   validator/client/beacon-api/beacon_api_validator_client.go:245 →
   `submitAggregateSelectionProofElectra` — beacon-api/submit_aggregate_selection_proof.go:56.
3. **`getAttestationDataRootFromRequest`** — submit_aggregate_selection_proof.go:94-123. Line 113
   RE-FETCHES `/eth/v1/validator/attestation_data` (beacon-api/attestation_data.go:17) *at
   aggregation time* and hashes the answer. This is the bug site.
4. `aggregateAttestationElectra` — submit_aggregate_selection_proof.go:146: GET
   `/eth/v2/validator/aggregate_attestation?slot&attestation_data_root&committee_index`.
   The "read until" prefix is only the multi-node GET wrapper (api/rest/multi_handler.go:83);
   the aggregation ctx carries no freshness hint, so there is no re-poll — one round, 404 through.
5. BN: `GetAggregateAttestationV2` — beacon-chain/rpc/eth/validator/handlers.go:45 →
   `aggregatedAttestation` (handlers.go:155): consults `AttestationsPool` aggregated then
   unaggregated; `matchingAtts` (handlers.go:208) requires **exact
   `att.Data.HashTreeRoot() == attestation_data_root`** (plus committee index from committee_bits,
   handlers.go:233). No match → 404 "No matching attestations found" (handlers.go:194).

BN attestation data is head-at-call-time: core `GetAttestationData`
(beacon-chain/rpc/core/validator.go:516) returns `BeaconBlockRoot = current head`, cached per
(slot, headRoot, headFull) (validator.go:533-537) — a fetch at 50% names the slot's own block.

gRPC: `grpcValidatorClient.SubmitAggregateSelectionProofElectra`
(validator/client/grpc-api/grpc_validator_client.go:279) → BN
`SubmitAggregateSelectionProofElectra` (aggregator.go:72): pool lookup by **slot + committee index
only** (aggregator.go:88-98), no data root; `mergeByData` + `bestAggregate` (aggregator.go:302)
then prefer the group containing the aggregator's own bit. No root to mismatch → never 404s.

## Root cause

With `--decoupled-ffg-vote-at-slot-start`, attesters sign at slot start + ≤200 ms jitter
(attest.go:39-40, wait_helpers.go:77, flags.go default 200 ms), naming the head *at vote time* —
normally block(N-1). The REST aggregation duty at 50% of the slot re-fetches attestation data and
queries the pool by the root of the *current* head — block(N), imported ~100–180 ms into the slot.
The pool holds only slot-start votes, so the queried root matches nothing → 404.

Neither path carries the signed data to the aggregation duty today:
- The VC retains it — `v.cachedAttestationData`, per-slot, filled by the attest duty
  (validator/client/validator.go:763-819; `roundHead.frozen` at 785-789 under head-at-round-start)
  — but `SubmitAggregateAndProof` never uses it and the beacon-api layer cannot see it; it
  re-fetches (submit_aggregate_selection_proof.go:113).
- gRPC does not need it: BN-side selection by slot+committee, and `bestAggregate` prefers the
  aggregator's own-signed group — de facto "as signed".

### Verified against final-02 logs

Block imports (cl-1): block(N) synced 88–212 ms into slot N. REST votes submitted at p50
101–127 ms (jitter race). Committee ~31 seats.

- Slot 2: votes 103 ms → named block(1) `0x84f58c3e` (parent). Pool (cl-4 "FFG aggregate groups"):
  one group, 31 seats, block(1). REST aggregators queried block(2) root → all three 404. gRPC
  published the 31-seat aggregate naming block(1).
- Slot 4: block(4) imported at 88 ms, vc-1 votes at 203 ms → named block(4) (own slot). Pool
  split `23 seats:block(4), 8 seats:block(3)`. vc-1's mid-slot fetch returned block(4) → matched
  the 23-seat group → its rare "success". gRPC aggregator chose its own-signed 8-seat group.
- Rule confirmed across the run: REST succeeds exactly on slots where its own VC's votes (or some
  minority) raced past the block import and named the own-slot block; 404 otherwise. 25/121 slots.

resttest2-01 (older build, votes effectively landed after block import at ~2.2 s): most slots the
pool contained own-slot-root votes, so the mid-slot query matched — only 12/65 slots 404ed, those
where votes named only the parent (e.g. slot 6: votes named block(5), query was block(6) root).

Shadow data62: blocks import 2–30 ms into slot, before most jittered votes → votes mostly name the
own-slot block → query matches. Slot 25: block(25) arrived 83 ms, after every VC's first (cached)
data fetch, so all 250 votes named block(24); the 8 VCs whose BN had imported block(25) by 6 s
queried its root → 404; the 2 others (incl. an isolated node still on block(24)) matched. Same
mechanism, opposite prevailing timing.

Ruled out: (a) post-Gloas index semantics — committee_index is compared against committee_bits
(handlers.go:233) and successes prove the pipeline; (c) save timing — gRPC finds the votes at the
same instant; (d) read-until re-poll — no freshness hint on this path, single round.

## Impact

- FFG inclusion: **unaffected** in these runs. Proposers pack from their own pool
  (beacon-chain/rpc/prysm/v1alpha1/validator/proposer_attestations.go:32-48); one subnet means
  every BN holds every vote (final-02 ledger coverage 1.000000, 117 blocks with 8 attestations,
  finality advanced continuously to round 13). Inclusion rides on pool packing, not on the
  aggregate-and-proof channel.
- Real cost: REST VCs publish almost no aggregate-and-proofs (25 vs 122 per ~122 slots), and the
  few they publish cover only the minority group that matched the mid-slot head. Harmless in this
  one-subnet topology; in a multi-subnet network, where proposers depend on gossiped aggregates
  for committees outside their subnets, this would gut inclusion. Also one wasted attestation_data
  fetch per duty and a misleading WARN per slot.

## Fix (implemented in truqvxky; full spec)

Semantics ruling: the aggregator queries with the root of the attestation data **it signed at
attestation time**, never a fresh mid-slot fetch. A re-derived root names the head at derivation
time; the pool holds the head at signing time.

### Interface

Add `attDataRoot []byte` (nil = unavailable) to exactly two iface methods
(validator/client/iface/validator_client.go):

```go
SubmitAggregateSelectionProof(ctx, in *ethpb.AggregateSelectionRequest,
    index primitives.ValidatorIndex, committeeLength uint64,
    attDataRoot []byte) (*ethpb.AggregateSelectionResponse, error)
SubmitAggregateSelectionProofElectra(...same..., attDataRoot []byte)
    (*ethpb.AggregateSelectionElectraResponse, error)
```

`SubmitSignedAggregateSelectionProof(Electra)` is untouched — it carries the full aggregate.
A plain Go parameter, not a proto field: `AggregateSelectionRequest` is the gRPC wire type and
the root is a client-internal concern; `index`/`committeeLength` set the precedent. Two
implementers only: beacon-api and grpc-api clients.

### Duty (validator/client/aggregate.go)

In `SubmitAggregateAndProof`, after `waitUntilAggregateDue` and the selection proof, compute
`attDataRoot := v.signedAttDataRoot(ctx, slot, duty.CommitteeIndex)` and pass it to both the
Electra and pre-Electra client calls. New helper:

- `signedAttDataRoot(ctx, slot, committeeIndex) []byte`: `v.getAttestationData` →
  `data.HashTreeRoot()`; on either error, WARN log ("Could not get signed attestation data for
  aggregation" / "Could not hash...") and return nil. Never fail the duty here: the gRPC BN
  ignores the root (server-side selection), so a fetch failure must not kill gRPC aggregation.

Why `getAttestationData` is "what was signed": the attest duty signs its return value unmutated
(attest.go:80,90 — data goes straight to signAtt). Electra+, it serves the per-slot cache filled
by the attest duty (validator.go:792-798); under `--decoupled-ffg-head-at-round-start` it serves
`roundHead.frozen(slot)` first (validator.go:785-789) — the round's first-vote data, also as
signed. An aggregator is always also an attester for the slot (`RoleAggregator` is appended only
inside the `RoleAttester` branch, validator.go:593-604), so the cache is warm in steady state.

Cache-miss policy: a miss (VC restarted mid-slot, or attest duty failed) falls through to
`getAttestationData`'s own single fetch (validator.go:800-822). That fetch IS the fallback —
do not keep a second fetch in the beacon-api client "as fallback"; that reintroduces the bug on
the miss path. Best effort, accepted: it happens at ~50% of slot (old buggy timing) and it fills
`cachedAttestationData` / freezes `roundHead` with mid-slot data (validator.go:817-820) —
harmless, the attest duty already ran or never will, next slot overwrites.

Pre-Electra caveat: no cache (validator.go:776-781, per-committee fetch), so the change moves
the fetch from the beacon-api client into the duty one-for-one — same args, same TOCTOU
semantics as before, unfixed by design (the timing bug is Electra+/Gloas). Accepted cost: gRPC
VCs pre-Electra now pay one discarded AttestationData fetch per aggregation duty (the duty can't
see the transport); Electra+ it's a cache hit. Post-Gloas needs nothing extra: the duty already
converts the Electra response (`AggregateAttestationAndProofElectraToGloas`).

### beacon-api client

- `submitAggregateSelectionProof(Electra)` (both, identically): replace
  `getAttestationDataRootFromRequest` with `validateAggregateSelectionRequest(ctx, in,
  committeeLength, attDataRoot) error` — keep the isOptimistic and IsAggregator checks, delete
  the attestation_data re-fetch and hashing, and error on `len(attDataRoot) == 0`:
  "attestation data root of the signed attestation is required". Pass `attDataRoot` straight to
  `aggregateAttestation(Electra)`.
- Do not delete `c.attestationData` (attestation_data.go:17) — still used by the
  `AttestationData` pass-through (beacon_api_validator_client.go:124).
- The outer `SubmitAggregateSelectionProof(Electra)` wrappers (beacon_api_validator_client.go)
  just thread the parameter through.

### grpc client

Both methods grow `_ []byte` and change nothing else — the BN selects by slot+committee and
`bestAggregate` prefers the aggregator's own-signed group (aggregator.go:88-98,302), which
already matches the ruling.

### Call-site audit

The iface methods have exactly one production caller: validator/client/aggregate.go. No
keymanager/web/rpc path calls them (validator/rpc tests only instantiate the mock, so they
recompile but need no edits). The nil-root REST error can therefore only fire on the duty's
fetch-failure path — a clear error instead of a misleading 404.

### Mocks

Regenerate `testing/validator-mock/validator_client_mock.go` only (mockgen v0.5.2 via
`go run go.uber.org/mock/mockgen`, command in the file header; no repo script exists). The
proto-level mocks (`testing/mock/beacon_validator_client_mock.go`, server mock) are untouched —
the proto did not change. Small diff (8 signatures/calls); include it in the fix commit.

### Tests

- beacon-api submit_aggregate_selection_proof_test.go (both variants): pin no-re-fetch with an
  explicit `Times(0)` on the attestation_data URL; success case passes the root and asserts the
  aggregate_attestation URL contains it; the old "attestation data error" case becomes
  "missing attestation data root" → the required-root error.
- aggregate_test.go Electra/Gloas subtests: prefill `validator.cachedAttestationData` and EXPECT
  the exact `expectedRoot[:]` on Submit* — no `AttestationData` EXPECT, so any re-fetch fails
  the mock. Pre-Electra subtests add an `AttestationData` EXPECT (the duty now fetches).
- `TestSubmitAggregateAndProof_CacheMissFetchesOnce`: empty cache → exactly one
  `AttestationData` call (`Times(1)`), its root passed through.
- `TestSubmitAggregateAndProof_NoAttestationData`: fetch errors → duty proceeds with nil root,
  WARN "Could not get signed attestation data for aggregation".
- runner_test.go:499: EXPECT arity and the `DoAndReturn` func signature both grow the parameter
  (gomock checks the func's arity only at runtime).

### Acceptance (enclave)

Recipe: final-02 topology — 5 nodes, vc-1..3 REST, vc-4..5 gRPC,
`--decoupled-ffg-vote-at-slot-start`; build images with a dedicated tag (never `:local` while
another enclave runs) and scope log collection by the enclave uuid label
`com.kurtosistech.enclave-id` (full uuid; `enclave inspect` prints the short one). Run ≥80
slots. Measured in ~/dev/prysm2-run-logs/aggfix-01/ and aggfix2-01/ (PASS bar):

- "No attestations to aggregate": 0 on every VC (82 slots; ~0 acceptable, slot-0 warmup only).
- "Submitted new aggregate attestations": REST miss rate equal to gRPC's and zero aggregation
  errors. Not "no misses": a ~0.4% silent per-slot miss predates the fix and shows on gRPC too
  (final-02 gRPC 121/122; aggfix2-01 2/425 across both arms).
- Aggregate roots: REST matches pool majority except on pool-split slots, where it publishes the
  group holding its own vote (aggfix-01: 232/246 majority; all 14 mismatches on the 21 split
  slots, same groups gRPC's own-bit preference chose). "Always majority" is the wrong criterion:
  expected mismatch rate == pool split rate. Note the pool-majority log ("FFG aggregate groups")
  is emitted by the gRPC BN handler only — read it from a gRPC node.
- Chain health unchanged: ledger coverage 1.000000, finality advancing, 0 engine errors.

### Standing checklist item (add to any client/transport port review)

This bug lived *between* endpoints: a value fixed at signing time was re-derived at use time,
and only the REST transport made the client derive it at all — the gRPC server reconstructed it
server-side, hiding the dependency. Per-endpoint translation review cannot see it.

> For every request the client will now construct, list each field the old server derived
> server-side. For each one, ask: is it a pure function of the request, or does it depend on
> chain state at the moment it is computed? If it names chain state (a head root, a checkpoint,
> anything hashed from fetched data), find where in the duty flow that state was first fixed —
> signed, submitted, or committed — and require the new client to reuse *that* value. Any
> "fetch it again when needed" is a time-of-check/time-of-use bug waiting for a timing change.

Applied here: `attestation_data_root` in `aggregate_attestation` is exactly such a field — gRPC
never sends it, REST derives it, its value is chain state at fetch time, and the flow fixed it
half a slot earlier at signing. The question flags the bug with no knowledge of decoupled-FFG
timing; the timing flag only decided when the mismatch became visible.
