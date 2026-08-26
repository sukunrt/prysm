# bug-aggregation-404: what the fix sketch missed

Record for the reviewer refreshing bug-aggregation-404.md and for the next implementer.
Every decision the sketch forced or left open, with where it landed in the code.
Enclave numbers for the fix are in ~/dev/prysm2-run-logs/aggfix-01/.

## Wrong or ambiguous in the sketch

1. **Which interface methods change.** The task phrasing said "pass it through the
   SubmitAggregateSelectionProof / SubmitSignedAggregateSelectionProof interface". Only
   `SubmitAggregateSelectionProof(Electra)` needs the root (it issues the aggregate_attestation
   GET); `SubmitSigned*` carries the full aggregate already and is untouched
   (validator/client/iface/validator_client.go:145-152). The bug doc's own sketch item 1 is
   right; keep the refreshed sketch to those two methods.

2. **"Delete the re-fetch" vs "fallback: single fetch" are in tension** (sketch items 2 and 4).
   If the beacon-api client deletes its fetch, the fallback fetch must live somewhere. Decision:
   it lives in the duty — `getAttestationData`'s cache-miss path *is* the single fetch
   (validator/client/validator.go:800-822) — and the beacon-api client hard-errors on an empty
   root (validateAggregateSelectionRequest,
   validator/client/beacon-api/submit_aggregate_selection_proof.go). The refreshed sketch should
   say this explicitly, or an implementer may keep a second fetch in the client "as fallback"
   and reintroduce the bug on the miss path.

3. **How the root travels.** Options were a new proto field on `AggregateSelectionRequest`
   (protoc regen, wire change for a client-internal concern) or a plain Go parameter. Decision:
   `attDataRoot []byte` parameter, nil meaning "unavailable". The sketch said "pass the root"
   without choosing.

4. **Fetch-failure policy (beyond cache miss).** Sketch item 4 covers "cache empty"; it does not
   say what happens when even the fallback fetch errors. Decision: log a warning and pass nil
   (`signedAttDataRoot`, validator/client/aggregate.go) so the gRPC duty — which never needed
   the data — still runs; the REST client fails with "attestation data root of the signed
   attestation is required". Failing the whole duty on fetch error would regress gRPC.

5. **Pre-Electra has no cache.** `getAttestationData` pre-Electra fetches per call with the
   committee index (validator.go:776-781); "the cache holds exactly what the attest duty signed"
   is Electra+ only. Pre-Electra the change moves the fetch from the beacon-api client to the
   duty, one-for-one (same slot+committeeIndex args) — semantics unchanged, and the decoupled-FFG
   timing bug is Electra+/Gloas anyway. The sketch's cache claim needed this caveat.

6. **The cache-hit path is fork-dependent in a way the sketch skips.** Under
   `--decoupled-ffg-head-at-round-start`, `roundHead.frozen(slot)` is consulted before the
   per-slot cache (validator.go:785-789) and returns the data of the round's first vote — also
   correct ("as signed"). And post-Gloas nothing extra is needed: the attest duty signs data from
   the same `getAttestationData` (attest.go:80); Gloas-ness only changes the aggregate-and-proof
   shapes downstream, which the duty already converts (aggregate.go postGloas branches).

7. **Cache-miss fallback has side effects the sketch ignores.** The fallback fetch fills
   `cachedAttestationData` and, under head-at-round-start, freezes `roundHead` with mid-slot data
   (validator.go:817-820). Accepted: the attest duty for that slot has already run (aggregator ⊆
   attester, see below) or never will; the next slot overwrites both.

## Facts an implementer must verify (now verified)

- **Aggregator is always also attester for the slot**: `RoleAggregator` is only appended inside
  the `RoleAttester` branch (validator/client/validator.go:593-604). So the cache is warm in
  steady state; a miss means VC restart mid-slot or a failed attest duty. This is what makes
  "fall back to a single fetch" acceptable rather than "skip the duty".
- **Only two implementers** of the iface: beacon-api and grpc-api clients. The gRPC BN-side
  selection (slot+committee, own-bit preference) already matches the ruling, so the gRPC client
  just grows an ignored parameter (validator/client/grpc-api/grpc_validator_client.go:275-282).

## Mock and test ripples (none in the sketch)

- `testing/validator-mock/validator_client_mock.go` is mockgen output; there is **no repo script**
  to regenerate it (nothing in hack/ or the Makefile; mockgen not installed). The generating
  command is in the file header; `go run go.uber.org/mock/mockgen` uses go.mod's pinned tool
  (v0.5.2) and reproduces the file byte-stable — the regen diff was 8 lines, so it belongs in the
  fix commit, not a mechanical one.
- `testing/mock/beacon_validator_client_mock.go` and the server mock are proto-level
  (BeaconNodeValidator gRPC) — untouched because the proto did not change. Don't "regenerate all
  mocks" reflexively.
- Duty tests break in a non-obvious way: they had no `AttestationData` expectations, and the duty
  now calls `getAttestationData`. Pre-Electra subtests need an `AttestationData` EXPECT (the duty
  fetches every time); Electra+ subtests instead prefill `validator.cachedAttestationData` — which
  doubles as the assertion that the duty passes the *cached* root without fetching (gomock fails
  on the unexpected call). validator/client/aggregate_test.go.
- `runner_test.go:499`'s `DoAndReturn` func signature must grow the parameter along with the
  EXPECT arity — gomock only checks arity at runtime.
- In the beacon-api client test, the old "attestation data error" case is meaningless after the
  fix (no fetch exists to fail); it became "missing attestation data root". The no-re-fetch
  guarantee is pinned by an explicit `Times(0)` on the attestation_data URL
  (validator/client/beacon-api/submit_aggregate_selection_proof_test.go).

## Enclave-measurement gotchas

- **Two enclaves, same container names.** final-02/collect.sh resolves containers by docker name
  prefix (`cl-1-prysm-geth--`); with `final` still running those greps match both runs. Filter on
  the kurtosis enclave uuid docker label — and note `kurtosis enclave inspect` prints a *short*
  uuid while the label `com.kurtosistech.enclave-id` holds the full one, so match on prefix
  (aggfix-01/collect.sh).
- **Image tags.** Building `:local` would silently replace the images the live `final` enclave's
  containers were started from (kurtosis/build-images.sh header). IMAGE_TAG=aggfix and the same
  tag in the args file, as the buildoor params file already documents.
- **The pool-majority log is gRPC-only.** "FFG aggregate groups"
  (beacon-chain/rpc/prysm/v1alpha1/validator/aggregator.go) is emitted by the gRPC BN handler, so
  measurement (c) reads the majority from cl-4/cl-5 and compares the REST VCs'
  "Submitted new aggregate attestations" blockRoot per slot against it. A REST-only network would
  have no such log.
- **Counting.** 404s: "No attestations to aggregate" per VC log. Successes: "Submitted new
  aggregate attestations". Slot 0 right after genesis can 404 legitimately (warmup); expect ~0,
  not exactly 0 (aggfix-01 measured exactly 0 over 82 slots).
- **"Matches pool majority" is the wrong success criterion on split slots.** The ruling is
  "aggregate what you signed", so when the pool splits the fixed VC publishes the group holding
  its own vote — a deliberate majority mismatch, identical to gRPC's `bestAggregate` own-bit
  preference (aggfix-01: all 14 mismatches sat on the 21/82 split slots, and gRPC vc-5 chose
  the same minority groups). Expected mismatch rate == pool split rate.

## The audit question that would have caught this at planning time

The REST-support plan reviewed each endpoint as an isolated translation of its gRPC equivalent.
This bug lives *between* calls: a value fixed at signing time (the attestation data root) was
re-derived at use time, and only the REST transport made the client derive it at all — the gRPC
server reconstructed it server-side, hiding the dependency.

Standing checklist item for any client/transport port:

> **For every request the client will now construct, list each field the old server derived
> server-side. For each one, ask: is it a pure function of the request, or does it depend on
> chain state at the moment it is computed? If it names chain state (a head root, a checkpoint,
> anything hashed from fetched data), find where in the duty flow that state was first fixed —
> signed, submitted, or committed — and require the new client to reuse *that* value. Any
> "fetch it again when needed" is a time-of-check/time-of-use bug waiting for a timing change.**

Applied here at planning time: `attestation_data_root` in `aggregate_attestation` is exactly such
a field — the gRPC path never sends it, the REST path derives it, its value is chain-state at
fetch time, and the flow fixed it half a slot earlier when the attestation was signed. The
question flags it without knowing anything about decoupled-FFG vote timing; the timing flag only
decided *when* the mismatch became visible.
