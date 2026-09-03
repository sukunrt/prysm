# REST client support for Gloas/Heze duties

Goal: make the validator client's REST mode (`validator/client/beacon-api/`) duty-complete on this
fork. The kurtosis entrypoint shim that used to strip `--beacon-rest-api-provider` is being deleted
on main by separate work and is NOT part of this plan (see §3, dropped step).

This plan was refreshed against a full first implementation. Everything below — file names, mock
names, error strings, test baselines, acceptance criteria — was verified on a real run; follow it
literally.

Correction to the assumed starting point: the REST client DOES have the Gloas block codec on both
sides. Producing (`GET /eth/v4/validator/blocks/{slot}`, SSZ + JSON,
`validator/client/beacon-api/get_beacon_block.go:72-190`,
`beacon-chain/rpc/eth/validator/handlers_block_gloas.go:33`) and publishing
(`propose_beacon_block.go:147-158`, BN decoders `beacon-chain/rpc/eth/beacon/handlers.go:678,846`)
already work. The stale comment in `kurtosis/validator-entrypoint.sh:5-8` blames a missing block
codec; the actual remaining blockers are the two available-attestation methods (hard panics) and
one optional no-op (builder preferences). Everything else the fork's VC calls is REST-capable
today — though note that until this work no VC on the fork had ever exercised any of it: the shim
kept every kurtosis VC on gRPC. The verification run here is the first execution of the fork's
REST payload-attestation, Gloas block, envelope and PTC paths as well as the two new ones.

## 1. Current state

### How REST mode is selected

- `config/features/config.go:394-396`: `EnableBeaconRESTApi` turns on if the bool flag is set OR
  `ctx.IsSet(BeaconRESTApiProviderFlag)` — flag presence alone, even with an empty value.
- `validator/client/factory.go:29-31`: `NewValidatorClient` returns the REST client whenever the
  feature is on AND `GetRestConnectionProvider()` is non-nil; otherwise gRPC.
- `factory.go:11-27`: node and chain clients are REST-with-gRPC-fallback wrappers in REST mode.
- **There is no gRPC fallback in the deployment this plan targets.** A kurtosis/ethereum-package
  VC gets `--beacon-rest-api-provider` and `--enable-beacon-rest-api` but NO
  `--beacon-rpc-provider` at all (verified by `docker inspect` on a live enclave). Chain-client
  methods that delegate to the gRPC fallback — e.g. `ValidatorBalances`,
  `beacon_api_beacon_chain_client.go:160-166` — have nothing to fall back to there and are a live
  gap, out of scope for this plan but not deniable. The in-repo e2e harness is different: it
  always passes `--beacon-rpc-provider` (`testing/endtoend/components/validator.go:238`), so e2e
  runs do have the fallback.

### Duty inventory

Every `iface.ValidatorClient` call the fork's VC makes (`validator/client/iface/`
`validator_client.go:123-180`), with REST-client and BN-HTTP status. "ok" = implemented and
fork-aware; verified in the named files.

Runner roles dispatched per slot (`validator/client/runner.go:245-263`): Attester (FFG),
Proposer, Aggregator, SyncCommittee, SyncCommitteeAggregator, PTCMember, AvailableAttester.

#### Duties and setup

| VC call | Caller | REST client | BN HTTP endpoint |
|---|---|---|---|
| Duties (combined, pre-Gloas only) | duties.go:129 | ok, composed (beacon-api/duties.go:48) | composed of the split endpoints |
| AttesterDuties | duties.go (fetchAllDuties) | ok (beacon-api/duties.go:426) | GET /eth/v1/validator/duties/attester/{epoch} (eth/validator/handlers.go:960) |
| ProposerDuties | duties.go | ok, v2 post-Gloas (beacon-api/duties.go:458-465) | GET /eth/v2/validator/duties/proposer/{epoch} (handlers.go:1158) |
| SyncCommitteeDuties | duties.go | ok (beacon-api/duties.go:482) | GET /eth/v1/validator/duties/sync/{epoch} (handlers.go:1255) |
| PTCDuties | duties.go | ok (beacon-api/duties.go:517-531) | GET /eth/v1/validator/duties/ptc/{epoch} (handlers.go:1419) |
| DomainData | signing paths | ok, computed locally + genesis (beacon-api/domain_data.go:15) | /eth/v1/beacon/genesis |
| WaitForChainStart | runner.go initialize | ok (genesis poll) | /eth/v1/beacon/genesis |
| ValidatorIndex / ValidatorStatus / MultipleValidatorStatus | validator.go:1216 etc | ok (index.go, status.go) | /eth/v1/beacon/states/... |
| CheckDoppelGanger | runner.go initialize | ok (doppelganger.go) | liveness + headers endpoints |
| SubscribeCommitteeSubnets | subnets.go | ok (subscribe_committee_subnets.go) | POST /eth/v1/validator/beacon_committee_subscriptions (handlers.go:667) |
| StartEventStream | runner/service | ok, REST-native SSE | GET /eth/v1/events; topics head_v2 + payload_available (api/client/event/event_stream.go:34, eth/events/events.go:46) |

Note: the available-attester duty needs no BN call at all — seats are computed locally
(`validator.go:630-638` via `decoupled.AvailableAttestationSeats`,
`decoupled/available_attestation_committee.go:36-40`).

#### Per-slot duties

| VC call | Caller | REST client | BN HTTP endpoint |
|---|---|---|---|
| AttestationData (FFG vote) | validator.go:764-824 (round-head cache) | ok (attestation_data.go:17); parses checkpoints into primitives.Round | GET /eth/v1/validator/attestation_data (handlers.go:733; committee_index optional post-Gloas :747) |
| ProposeAttestationElectra (FFG vote, post-Electra container) | attest.go:154 | ok, JSON + Eth-Consensus-Version from ToForkVersion (propose_attestation.go:44-70) | POST /eth/v2/beacon/pool/attestations (handlers_pool.go:130; Gloas index rules :314-334) |
| ProposeAttestation (pre-Electra) | attest.go:164 | ok (propose_attestation.go:15) | same endpoint |
| **AvailableAttestationData** | validator.go:855 via available_attestation.go:129 | **panic("unimplemented: use grpc") beacon_api_validator_client.go:127-130** | **none** — gRPC only: GetAvailableAttestationData (prysm/v1alpha1/validator/attester.go:55 → core/validator.go:1105-1124) |
| **ProposeAvailableAttestation** | available_attestation.go:149 | **panic beacon_api_validator_client.go:195-198** | **none** — gRPC only: attester.go:153, proposeAvailableAtt :355-428 |
| BeaconBlock | propose.go / propose_gloas.go | ok, v4 SSZ+JSON, include_payload (get_beacon_block.go:25-110) | GET /eth/v4/validator/blocks/{slot} (handlers_block_gloas.go:33; endpoints.go:414) |
| ProposeBeaconBlock | propose flow | ok, SSZ first + JSON fallback, version header "gloas" (propose_beacon_block.go:147-217) | POST /eth/v2/beacon/blocks (handlers.go:660,828 register gloas decoders) |
| GetExecutionPayloadEnvelope | propose_gloas.go:93 | ok (execution_payload_envelope.go:99-137) | GET /eth/v1/validator/execution_payload_envelopes/{slot}/{root} (handlers_block_gloas.go:193) |
| PublishExecutionPayloadEnvelope | propose_gloas.go:105 | ok (execution_payload_envelope.go:139-188) | POST /eth/v1/beacon/execution_payload_envelopes (handlers_gloas.go:96) |
| PayloadAttestationData (PTC) | payload_attestation.go:44 | ok, SSZ+JSON with freshness (beacon-api/payload_attestation.go:22-45) | GET /eth/v1/validator/payload_attestation_data (handlers.go:1673; endpoints.go:442) |
| SubmitPayloadAttestation | payload_attestation.go:108 | ok, SSZ + JSON fallback (beacon-api/payload_attestation.go:47-73) | POST /eth/v1/beacon/pool/payload_attestations (handlers_pool.go:1006; endpoints.go:965) |
| SubmitAggregateSelectionProof(Electra) | aggregate.go:75 | ok (submit_aggregate_selection_proof.go:56) | GET /eth/v2/validator/aggregate_attestation (handlers.go:45; gloas branch :70-82 emits Electra-shaped JSON, which the client parses) |
| SubmitSignedAggregateSelectionProof(Electra) | aggregate.go:123 | ok (submit_signed_aggregate_proof.go:34) | POST /eth/v2/validator/aggregate_and_proofs (handlers.go:432) |
| Sync committee family (SyncMessageBlockRoot, SubmitSyncMessage, SyncSubcommitteeIndex, SyncCommitteeContribution, SubmitSignedContributionAndProof, AggregatedSyncSelections) | sync_committee.go | ok (beacon-api/sync_committee.go etc.) | standard endpoints, registered |
| AggregatedSelections | aggregator_selector.go | ok (beacon_committee_selections.go) | POST /eth/v1/validator/beacon_committee_selections |

Post-Gloas the VC converts Gloas aggregate containers to/from Electra shape around the client
call (`aggregate.go:80-111`, `AttGloasFromConsensus` returns a `structs.AttestationElectra` —
`api/server/structs/conversions.go:518`), so the wire format needs no Gloas variant.

#### Registration / preferences

| VC call | Caller | REST client | BN HTTP endpoint |
|---|---|---|---|
| SubmitSignedProposerPreferences | validator.go (push settings, reorg resubmit) | ok (proposer_preferences.go:17) | POST /eth/v1/validator/proposer_preferences (handlers.go:244; endpoints.go:382) |
| PrepareBeaconProposer / SubmitValidatorRegistrations / ProposeExit | registration.go:45, propose.go:354 | ok | ok (post-Gloas the BN accepts prepare/register as deprecated no-ops, handlers.go:879,923) |
| **SubmitBuilderPreferences** | validator.go:979 (only when a builder relay is configured) | **silent no-op stub, beacon_api_validator_client.go:300-303** | **none** — gRPC only (prysm/v1alpha1/validator/builder_preferences.go:15) |
| **SubmitSignedExecutionPayloadBid** | **no VC caller anywhere** (only grpc-api wrapper, grpc_validator_client.go:311-313) | silent no-op stub :306-309 | exists: POST /eth/v1/beacon/execution_payload_bids (handlers_gloas.go:395; endpoints.go:995) |

#### Chain/node clients (not ValidatorClient, still on the REST path)

- `ValidatorPerformance` (validator/client/metrics.go:250) is REST-implemented via the
  Prysm-only `/prysm/validators/performance` (beacon_api_beacon_chain_client.go:318-358).
- Several chain-client methods delegate to the gRPC fallback (e.g. ValidatorBalances,
  beacon_api_beacon_chain_client.go:160-166). Pre-existing upstream behavior, unchanged by this
  plan — but see §1's warning: in a kurtosis deployment the fallback has no connection.

Summary: exactly two hard blockers (both available-attestation methods, both sides missing), one
optional soft gap (builder preferences), one dead interface method (payload bid).

## 2. Spec

The available-attestation containers are Heze-defined and not in the upstream beacon-APIs, so
both endpoints are fork-local. Shapes follow the fork's own payload-attestation precedent. They
get no OpenAPI/spec artifact — no fork-local endpoint in this repo has one; do not go looking.

Wire shapes (proto/prysm/v1alpha1/heze.proto:14-49, SSZ in proto/prysm/v1alpha1/heze.ssz.go):

- `AvailableAttestationData` — fixed 41 bytes SSZ: `slot` (uint64), `payload_present` (bool),
  `beacon_block_root` (32 bytes).
- `AvailableAttestation` — variable-size SSZ, 205 bytes plus the scratch length:
  `aggregation_bits` (Bitvector512, 64 bytes), `data`, `signature` (96 bytes) and
  `scratch_space` (List[byte, 65536], offset-encoded).

Type detail that ripples through the structs work: `AvailableAttestation.AggregationBits` is
typed `bitfield.Bitvector512` (`proto/prysm/v1alpha1/heze.pb.go:92`, Go import
`github.com/OffchainLabs/go-bitfield`, Bazel label `@com_github_prysmaticlabs_go_bitfield`), not
`[]byte`. `MarshalSSZ` hard-fails on any length != 64 (`heze.ssz.go:28-32`), so `ToConsensus`
must validate length at decode time — use `bytesutil.DecodeHexWithLength(..., 64)` behind a
file-local `const availableAttestationBitsLength = 64` (there is no `fieldparams` constant, and
the only real one, `decoupled.AvailableAttestationCommitteeSize`, would drag `decoupled` into
`api/server/structs`) — and convert with an explicit `bitfield.Bitvector512(bytes)`.

### 2.1 GET /eth/v1/validator/available_attestation_data  [fork-local]

Query: `slot` (required uint). Handler `GetAvailableAttestationData` on the eth/validator
`Server`, in a new `beacon-chain/rpc/eth/validator/handlers_heze.go`. Skeleton, in order:

- `trace.StartSpan`, then syncing gate via `shared.IsSyncing` (503 when syncing).
- Slot via `_, slot, ok := shared.UintFromQuery(w, r, "slot", true)` (three return values).
- Build `&ethpbalpha.AvailableAttestationDataRequest{Slot: primitives.Slot(slot)}` and delegate
  to `s.CoreService.GetAvailableAttestationData(ctx, req)` (core/validator.go:1105) — the same
  code the gRPC path uses. Unlike the payload-attestation core method it takes a proto request,
  not a bare slot; the REST handler constructs one. (The request proto is marked
  `option deprecated = true`, validator.proto:1380 — live anyway, ignore the marker.) On error,
  `httputil.HandleError(w, rpcErr.Err.Error(), core.ErrorReasonToHTTP(rpcErr.Reason))`. Pre-Heze
  slots 400 with the message `"available attestation data is only available for heze fork"` —
  lowercase, NOT the payload-attestation path's "Gloas fork" capitalization; tests must match
  this string, a literal mirror of `TestGetPayloadAttestationData` fails.
- Only after the error branch, set `Eth-Consensus-Version: heze`
  (`w.Header().Set(api.VersionHeader, version.String(version.Heze))`) — set it earlier and a
  400 carries a fork header. It applies to both encodings.
- SSZ response when `httputil.RespondWithSsz(r)`: `data.MarshalSSZ()` (41 bytes) via
  `httputil.WriteSsz`.
- JSON response otherwise:
  `{"version":"heze","data":{"slot":"7","payload_present":true,"beacon_block_root":"0x…"}}`
  — numbers as decimal strings, roots hex, bool as JSON bool, exactly like
  `structs.PayloadAttestationData` (api/server/structs/block.go:531-536).
- Registration in `beacon-chain/rpc/endpoints.go` `validatorEndpoints`, cloned from the
  payload_attestation_data entry (endpoints.go:441-450): AcceptHeaderHandler(json, octet-stream)
  + AcceptEncodingHeaderHandler, GET, name `namespace + ".GetAvailableAttestationData"`.

Semantics note, intended: the core method has no NoContent arm and no future-slot guard
(contrast `ListPayloadAttestations`, handlers_pool.go:1136-1141), so `?slot=999999999` returns a
zero-root 200. That is gRPC-path parity; do not add a guard.

New structs in `api/server/structs`: `AvailableAttestationData`, `AvailableAttestation`
(block.go, next to PayloadAttestation :540), `GetAvailableAttestationDataResponse{Version,
Data}` (endpoints_validator.go, next to GetPayloadAttestationDataResponse :34), conversions
`AvailableAttestationDataFromConsensus` / `AvailableAttestationFromConsensus` and `ToConsensus`
on both (conversions_block.go, next to PayloadAttestationDataFromConsensus :2979; decode errors
via `server.NewDecodeError`, lengths checked per the Bitvector512 note above — the
payload-attestation precedent's unchecked `hexutil.Decode` is NOT good enough here).

### 2.2 POST /eth/v1/beacon/pool/available_attestations  [fork-local]

Mirrors the payload-attestation pool endpoint (handlers_pool.go:1006, endpoints.go:955-975).
Handler `SubmitAvailableAttestations` on the eth/beacon `Server`, in a new
`beacon-chain/rpc/eth/beacon/handlers_heze.go`. In order:

- Syncing gate via `shared.IsSyncing` (503).
- `Eth-Consensus-Version` header: required (400 `"Eth-Consensus-Version header is required"` if
  absent), parsed with `version.FromString` (400 on garbage), and 400
  `"Available attestations require the Heze fork"` when `v < version.Heze`. No current-epoch
  gate in the handler: the delegate (`proposeAvailableAtt`, attester.go:369-380) already checks
  both the current epoch and the attestation's epoch, and duplicating it here would be the only
  place the two could drift. (Its model `SubmitPayloadAttestations` does gate on the fork epoch,
  handlers_pool.go:1010-1014; deliberately not copied.)
- Body JSON: array of `structs.AvailableAttestation`
  `{"aggregation_bits":"0x…64 bytes…","data":{…as above…},"signature":"0x…"}`; empty array is a
  400 `"no data submitted"`.
- Body SSZ (`Content-Type: application/octet-stream`, `httputil.IsRequestSsz`): the SSZ list
  encoding of a variable-size element, an offset table followed by the elements; an empty or
  malformed body is a 400 `"Invalid SSZ available attestation list size"`.
- Per-element decode failures collect into `server.IndexedError`s (index + message) and the
  element is skipped (nil slot); decode of the list as a whole failing is a plain 400.
- Delegate each decoded element to `s.V1Alpha1ValidatorServer.ProposeAvailableAttestation` —
  the eth/beacon Server already holds that interface (eth/beacon/server.go:48). This reuses the
  full gRPC submit path: signature sanity check, Heze gating, payload_present rule, broadcast,
  local forkchoice delivery, and the Goldfish vote-ledger log line (attester.go:355-428).
- **Error shape — one shape, mandated:** the pool precedent. On any failures respond
  `server.IndexedErrorContainer{Code: http.StatusBadRequest, Message:
  server.ErrIndexedValidationFail, Failures: …}` (handlers_pool.go:1110-1117). Unwrap delegate
  errors with a small file-local helper `grpcErrorMessage(err)`: `status.FromError`, return
  `st.Message()` when the code is not `codes.Unknown`, else `err.Error()`. Do NOT use the
  bid endpoint's per-status HTTP mapping (handlers_gloas.go:441-458) — a list endpoint carries
  one Code for the whole response and cannot map per element. Accepted consequence: a delegate
  failure that is really `codes.Internal` (e.g. broadcast failure) reports as HTTP 400, not 500.
- Response on full success: 200, empty body. The attestation data root is not carried on the
  wire in either direction; gRPC and REST agree only because server (attester.go:361-364) and
  client (§2.3) each compute `att.Data.HashTreeRoot()` independently.
- Registration in `beaconEndpoints`, cloned from the SubmitPayloadAttestations entry
  (endpoints.go:965-974): ContentTypeHandler(json, octet-stream) + AcceptHeaderHandler(json) +
  AcceptEncodingHeaderHandler, POST, name `namespace + ".SubmitAvailableAttestations"`.

### 2.3 REST client methods

New `validator/client/beacon-api/available_attestation.go`, modeled on `payload_attestation.go`,
with a package const `availableAttestationsEndpoint = "/eth/v1/beacon/pool/available_attestations"`:

- `availableAttestationData(ctx, slot)`: `c.handler.GetSSZ(ctx,
  "/eth/v1/validator/available_attestation_data?slot=%d",
  availableAttestationFreshnessOptions(ctx)...)`. GetSSZ negotiates; branch on the response
  `Content-Type`: octet-stream → `UnmarshalSSZ` into `ethpb.AvailableAttestationData`; else JSON
  into `structs.GetAvailableAttestationDataResponse`, error `"available attestation data is
  nil"` on nil Data, then `Data.ToConsensus()`.
- **Freshness — refactor, do not clone.** `readFreshnessOptions` (freshness.go:100) is the WRONG
  helper: it takes `func(json.RawMessage, iface.Head) bool` and installs `rest.WithAccept`,
  which is JSON-only, while this read is SSZ-preferred and needs
  `rest.WithSSZAccept(func([]byte, http.Header) bool)`. The right model is the body of
  `payloadAttestationFreshnessOptions` (freshness.go:194-230). Mandated shape: extract that body
  into `sszRootFreshnessOptions(ctx, extract func([]byte, http.Header) ([32]byte, bool))`;
  `payloadAttestationFreshnessOptions` becomes a one-liner passing
  `payloadAttestationBeaconBlockRoot`, and the new `availableAttestationFreshnessOptions` passes
  a new `availableAttestationBeaconBlockRoot` that decodes SSZ when the Content-Type is
  octet-stream (root via `bytesutil.ToBytes32`) and otherwise reuses
  `rootExtractor("beacon_block_root")` on the JSON body.
- Why freshness matters here: the head hint is already put in ctx by
  `getAvailableAttestationData` (validator.go:833, `withHeadHint`), so the multi-host read
  prefers a node that has imported the announced head, like the FFG read
  (attestation_data.go:29). Premise correction: the hint deadline is
  `attestationDueComponent(slot)` — the FFG due time — not the Goldfish
  `AVAILABLE_ATTESTATION_DUE_BPS_HEZE` due time. Pre-existing, identical on the gRPC path; do
  not change it, but do not claim the read is bounded by the Goldfish deadline either.
- `proposeAvailableAttestation(ctx, att)`: nil-guard att and att.Data (`"available attestation
  is nil"`); compute `root := att.Data.HashTreeRoot()` up front; `PostSSZ` of the one-element
  SSZ list — a 4-byte offset table holding the value 4, then `att.MarshalSSZ()` — to
  `availableAttestationsEndpoint` with header `Eth-Consensus-Version: heze`. On a
  `httputil.DefaultJsonError` with Code 415, warn once and fall back to `Post` of the JSON array
  `[]*structs.AvailableAttestation{AvailableAttestationFromConsensus(att)}` with the same
  header, like submitPayloadAttestation (payload_attestation.go:59-72). Any other error returns
  as is. Success returns `&ethpb.AttestResponse{AttestationDataRoot: root[:]}`, as
  proposeAttestation does (propose_attestation.go:36-41).
- Replace the two panics in beacon_api_validator_client.go:127-130 and :195-198 with
  `trace.StartSpan` + `wrapInMetrics` wrappers matching every other method in that file.

### 2.4 Versioning policy

- Existing endpoints keep saying `gloas` at Heze epochs: `slots.ToForkVersion` caps at Gloas
  (time/slots/slottime.go:80-84) and every Gloas-era handler hardcodes
  `version.String(version.Gloas)`. Heze changes no container these endpoints carry, so nothing
  breaks and nothing should change.
- The two new endpoints use `heze`, because their containers are Heze-defined.
  `version.FromString("heze")` already works (runtime/version/fork.go:18,30). The GET sets the
  response header on both encodings (after the error branch, §2.1); the POST requires the
  request header and treats it via `v >= version.Heze`, not string equality — missing header,
  unparseable header, and `gloas` are all 400 (§2.2).
- Minor latent quirk, no action needed: the v3 block SSZ codec cascade (get_beacon_block.go:205)
  and the v3 JSON factories (:362) stop at Fulu, but the v3 path is only taken pre-Gloas
  (:32-36).

### 2.5 Builder preferences and the dead bid method

- `SubmitBuilderPreferences` fires only when proposer settings configure builder relays
  (validator.go:1536-1545); no kurtosis/e2e flow on this fork does. Keep the no-op stub, but
  make it loud: a package-scope `var builderPreferencesWarning sync.Once` guarding a `log.Warn`
  ("Builder preferences are dropped in REST mode; a relay-configured validator needs the gRPC
  client until a REST endpoint exists"). There is no rate-limited logger anywhere under
  `validator/` — do not go hunting for one; once per process is the right cardinality for a
  static configuration gap. Implement a real REST endpoint (fork-local
  `POST /eth/v1/validator/builder_preferences` delegating to
  `V1Alpha1Server.SubmitBuilderPreferences`, builder_preferences.go:15) only when the builder
  flow is actually exercised under REST.
- `SubmitSignedExecutionPayloadBid` has no VC caller (grep: only the grpc-api wrapper). Delete
  it from exactly these four places: `validator/client/iface/validator_client.go:163`,
  `validator/client/beacon-api/beacon_api_validator_client.go:306-309` (stub + its TODO
  comment), `validator/client/grpc-api/grpc_validator_client.go:311-313`, and the generated
  mock `testing/validator-mock/validator_client_mock.go` (hand-edit the two
  SubmitSignedExecutionPayloadBid blocks out; regenerating with mockgen is a separate toolchain
  step, not required). Do NOT touch `testing/mock/beacon_validator_client_mock.go` — it mocks
  the proto gRPC client `eth.BeaconNodeValidatorClient`, which keeps the method. The BN's REST
  bid endpoint (used by the external buildoor) is untouched.

## 3. Implementation plan

Each step is one atomic commit, buildable and tested on its own. Run the test tier named in §4
after each.

1. **structs: available attestation JSON containers.**
   Files: `api/server/structs/block.go` (AvailableAttestationData, AvailableAttestation),
   `api/server/structs/endpoints_validator.go` (GetAvailableAttestationDataResponse),
   `api/server/structs/conversions_block.go` (FromConsensus/ToConsensus both ways, the
   Bitvector512 handling of §2, new `github.com/OffchainLabs/go-bitfield` import),
   `api/server/structs/BUILD.bazel` (`@com_github_prysmaticlabs_go_bitfield` in BOTH go_library
   deps and go_test deps; new test src).
   Test: new `conversions_block_heze_test.go` — round-trip both containers, and ToConsensus
   error cases: nil receiver, short aggregation bits, bad signature hex, bad slot, short root,
   nil data.

2. **BN: GET /eth/v1/validator/available_attestation_data.**
   Files: new `beacon-chain/rpc/eth/validator/handlers_heze.go` (§2.1),
   `beacon-chain/rpc/endpoints.go` (validatorEndpoints),
   `beacon-chain/rpc/endpoints_test.go` — the golden route list `Test_endpoints` fails on any
   unregistered route; add `"/eth/v1/validator/available_attestation_data": {http.MethodGet}`
   to `validatorRoutes` (:121) —
   and `beacon-chain/rpc/eth/validator/BUILD.bazel` (new src + test src).
   Test: new `handlers_heze_test.go`. Mocks: `mockChain.ChainService`
   (`beacon-chain/blockchain/testing`) with `MockCanonicalRoots
   map[primitives.Slot][32]byte` and `MockCanonicalFull map[primitives.Slot]bool` — the pair
   `CanonicalNodeAtSlot` (mock.go:1020-1029) reads; `MockCanonicalFull` must be non-nil or
   `payload_present` is always false — plus `mockSync.Sync` and a `core.Service` wired with the
   same chain service (GenesisTimeFetcher, ForkchoiceFetcher, HeadFetcher, ChainInfoFetcher).
   Do NOT reach for `ChainService.AvailableAttestations` / `ReceiveAvailableAttestationErr`
   (mock.go:86-87): those belong to the v1alpha1 server's test surface and are unreachable from
   this handler. Cases: pre-Heze 400 (assert the lowercase `"heze fork"` string, §2.1), missing
   slot 400, syncing 503, JSON 200 (version "heze", fields, response header), payload_present
   false when MockCanonicalFull says false, SSZ 200 (41 bytes decode + header).

3. **BN: POST /eth/v1/beacon/pool/available_attestations.**
   Files: new `beacon-chain/rpc/eth/beacon/handlers_heze.go` (§2.2, incl. the two decode
   helpers and `grpcErrorMessage`), `beacon-chain/rpc/endpoints.go` (beaconEndpoints),
   `beacon-chain/rpc/endpoints_test.go` — add
   `"/eth/v1/beacon/pool/available_attestations": {http.MethodPost}` to `beaconRoutes` (:54) —
   and `beacon-chain/rpc/eth/beacon/BUILD.bazel` (new src + test src).
   Test: new `handlers_heze_test.go`. Mock the delegate with
   `mock2.NewMockBeaconNodeValidatorServer` (gomock, import
   `github.com/OffchainLabs/prysm/v7/testing/mock`) EXPECTing `ProposeAvailableAttestation`;
   chain/sync mocks as in step 2. Cases: JSON single element reaches the delegate intact, SSZ
   single element ditto, SSZ two concatenated elements → two delegate calls, missing header
   400, unparseable header 400, `gloas` header 400 ("Heze fork"), syncing 503, empty JSON array
   400, misaligned SSZ 400, bad JSON element → indexed failure naming the field, delegate
   gRPC error → 400 whose body carries the unwrapped status message.
   Note: steps 2 and 3 are logically independent but both touch `endpoints.go` and
   `endpoints_test.go`, so they conflict textually if built in parallel — land sequentially.

4. **VC REST client: implement both methods, remove the panics.**
   Files: new `validator/client/beacon-api/available_attestation.go` (§2.3),
   `validator/client/beacon-api/freshness.go` (the `sszRootFreshnessOptions` refactor +
   `availableAttestationFreshnessOptions` + `availableAttestationBeaconBlockRoot`),
   `validator/client/beacon-api/beacon_api_validator_client.go` (:127-130, :195-198 →
   wrapInMetrics), `validator/client/beacon-api/BUILD.bazel` (new src + test src, and
   `@com_github_prysmaticlabs_go_bitfield` in the TEST deps).
   Test: new `available_attestation_test.go` with gomock `mock.NewMockHandler`
   (`validator/client/beacon-api/mock`): GetSSZ returning JSON and returning SSZ (both decode),
   nil JSON data error, endpoint error passthrough; PostSSZ success returns the locally
   computed data root, 415 falls back to Post with the JSON body, non-415 error does not fall
   back, nil att / nil data errors. Extend `freshness_test.go`: no hint → nil options; with a
   hint the SSZAccept matches the announced head against both an SSZ and a JSON response and
   rejects a wrong root and garbage SSZ.

5. **Cleanup: delete SubmitSignedExecutionPayloadBid from the VC interface.**
   Exactly the four files in §2.5, and only those. Test: build + existing suites; no new tests.

6. **Builder-preferences stub: make the gap loud.**
   File: `beacon_api_validator_client.go:300-303` — the `sync.Once` warn of §2.5. Test: none
   beyond compile; behavior is log-only.

7. **e2e: run the Heze suite over REST.**
   File: `testing/endtoend/heze_e2e_test.go` — add `TestEndToEnd_HezeGenesisRESTApi`: the exact
   config of `TestEndToEnd_HezeGenesis` (:69-87 — `params.E2EMainnetTestConfig()`,
   `types.InitForkCfg(version.Heze, version.Heze, cfg)`, `cfg.SlotsPerRound = 8`,
   `types.WithEpochs(5)`, `withoutEvaluators(hezeDroppedEvaluators...)`, `withEvaluators(`
   `ev.ChainProducesBlocks, ev.AvailableAttestationsFlow, ev.AttestationsInEveryRound,`
   `ev.FinalizationOccursInRounds(3), ev.JustificationAdvancesEveryRound)`) plus
   `types.WithValidatorRESTApi()` (types/types.go:49-53; plumbing already exists,
   components/validator.go:247-256). This is the only run that exercises the fork-local
   available-attestation endpoints — every other run uses their gRPC counterparts;
   AvailableAttestationsFlow is what proves they carried the votes.

8. **Dropped: kurtosis shim retirement.** An earlier revision of this plan deleted
   `kurtosis/validator-entrypoint.sh` here. That is now separate work already landed on main;
   duplicating it in this stack would only conflict on rebase. Do not touch anything under
   `kurtosis/` in this stack. The consequence for verification is in §4.3: if the tree you
   build images from still carries the shim, a kurtosis "REST" run silently exercises gRPC and
   passes for the wrong reason, so §4.3's entrypoint check is mandatory, and its image override
   is the bypass when the shim is present.

9. **Flag-presence behavior: leave `ctx.IsSet` as is.**
   `config/features/config.go:394` enabling REST on the provider flag's mere presence is
   upstream Prysm behavior and, once REST works, is exactly what makes a stock
   ethereum-package (which always passes `--beacon-rest-api-provider`) run the REST path with
   zero fork-side configuration — verified live: that flag alone put the VC on the REST client.
   Changing it would silently push kurtosis VCs back onto gRPC. Only revisit if a deployment
   needs the flag present-but-inert; then add an explicit `--enable-beacon-rest-api=false`
   override rather than removing IsSet. No code change; decision recorded here.

Order: 1→4 are the functional chain (4 needs 1; 2 and 3 are independent in content but land
sequentially, see step 3). 5 and 6 can land any time. 7 requires 1-4.

## 4. Testing

Plain `go test`, NEVER bazel — the spectest/bazel split in this repo makes bazel test runs a
separate project. Write full outputs to files.

### 4.1 Baseline first

"All green" is unattainable: the relevant tree has 32 pre-existing failures ON THE PLAN COMMIT,
before any change — `beacon-chain/rpc/eth/beacon` `TestSubmitAttestationsV2` (3 subtests,
attestation-pool timing) and `beacon-chain/rpc/prysm/v1alpha1/beacon` (29 tests, mostly
"bytes array does not have the correct length"). Before writing any code, capture a baseline on
the plan commit and diff against it at the end; do NOT chase these as regressions:

```
go test ./beacon-chain/rpc/... ./validator/client/... ./api/server/structs/... \
    2>&1 | tee gotest-baseline.txt
```

### 4.2 Per-step and final unit runs

After each step, the touched packages; at the end, the same command as the baseline plus the
route golden test. Must-pass set (everything not in the baseline's failing pair):

- `go test ./api/server/structs/...`
- `go test ./beacon-chain/rpc/` — `Test_endpoints`, the golden route list; fastest failing
  test for this change if a route registration or endpoints_test.go line is missing
- `go test ./beacon-chain/rpc/eth/validator/...`
- `go test ./beacon-chain/rpc/eth/beacon/...` — expect exactly the TestSubmitAttestationsV2
  failures from the baseline, nothing new
- `go test ./validator/client/...` — whole tree, passes entirely

The mocks, by name (found the hard way; do not rediscover them): `mock.NewMockHandler`
(`validator/client/beacon-api/mock`) for the client; `mock2.NewMockBeaconNodeValidatorServer`
(`testing/mock`) for the POST handler's delegate; `mockChain.ChainService{MockCanonicalRoots,
MockCanonicalFull}` (`beacon-chain/blockchain/testing`) for the GET handler.

### 4.3 Kurtosis verification run (acceptance)

The e2e scenario (step 7) runs as a plain `go test` with the runfiles-tree environment
(`RUNFILES_DIR`/`TEST_WORKSPACE=prysm` etc. — see plan-detailed.md "Harness notes"); the
kurtosis run below is the deployment-shaped acceptance and is what the criteria refer to.

Enclave recipe:

- Build images from this workspace tree with a dedicated tag so no other enclave's images are
  touched: `IMAGE_TAG=resttest kurtosis/build-images.sh`.
- Copy `kurtosis/network_params.yaml` to scratch (do NOT commit the copy, do NOT edit the
  original): replace every `:local` image tag with `:resttest` (cl_image, vc_image in both
  participant blocks, and the genesis `image:`), and append `--enable-beacon-rest-api` to BOTH
  `vc_extra_params` lists.
- Shim check, mandatory before trusting anything: if the built validator image's entrypoint is
  the shim (`docker inspect` shows `/entrypoint.sh` — i.e. `kurtosis/validator-entrypoint.sh`
  still exists in the tree you built from), the run will silently strip
  `--beacon-rest-api-provider` and exercise gRPC. Bypass with a one-line derived image and use
  it in the params copy:
  `FROM prysm-validator:resttest` / `ENTRYPOINT ["/validator"]`. If main's shim removal is
  already in your tree, the plain image is fine — but still verify the entrypoint.
- Run in a fresh enclave (`kurtosis run --enclave resttest ~/dev/ethereum-package --args-file
  <scratch copy>`); do not touch other enclaves. Save all logs and probe outputs to a run-log
  directory; leave the enclave running when done.

Acceptance criteria — all of them, none optional:

1. **The run actually took the REST path.** `docker inspect` on a VC container: entrypoint is
   `[/validator]`, argv contains `--beacon-rest-api-provider=...` and `--enable-beacon-rest-api`
   and — expectedly — NO `--beacon-rpc-provider` (ethereum-package does not pass one; REST mode
   there has no gRPC fallback, §1). Without this check the whole run is meaningless.
2. **Zero VC panics and zero container restarts**, all nodes, VC and BN both.
3. **`Goldfish vote` ledger lines with `outcome=local` on every BN** — the only proof the new
   POST endpoint reached `proposeAvailableAtt` (attester.go:405-425); requires the
   `--goldfish-vote-ledger` BN flag already in network_params.yaml. Reference volume from the
   verified run (5 nodes x 50 keys, 12 s slots): ~3050 per node by slot 60, alongside ~1930
   `FFG vote` lines.
4. **Direct endpoint probes** against a BN's REST port (3500):
   - GET `/eth/v1/validator/available_attestation_data?slot=<recent>`: 200, JSON
     `"version":"heze"`, `Eth-Consensus-Version: heze` header; with
     `Accept: application/octet-stream`: 200, exactly 41 bytes, same header.
   - POST `/eth/v1/beacon/pool/available_attestations` with no version header: 400; with
     `Eth-Consensus-Version: gloas`: 400 "Available attestations require the Heze fork".
5. **Finality advances**, sampled ~once a minute from
   `/eth/v1/beacon/states/head/finality_checkpoints`. Unit warning: on this fork the checkpoint
   `epoch` field reports ROUNDS (`SLOTS_PER_ROUND=8`), not epochs. So `finalized >= 2` lands
   about 7 minutes after genesis (verified: justified 2 at +5 min, finalized 2 at +7 min,
   finalized 4 at +10 min) — budget ~10 minutes, not the ~20 an epoch reading suggests.
6. **Noise floor:** the verified run's only VC error over ~10 minutes was one transient
   `read: connection reset` on a sync-committee POST. More than stray one-offs is a finding.

## 5. Risks and open questions

- **Latency of the data fetch.** The Goldfish vote is due
  `AVAILABLE_ATTESTATION_DUE_BPS_HEZE` into the slot; REST adds an HTTP round trip plus the
  multi-host freshness race where gRPC had one stream. The freshness matcher and the per-slot
  VC-side cache (validator.go:838-863) bound this — but note the hint deadline is the FFG due
  time, not the Goldfish one (§2.3), so the bound is looser than it looks. Only a
  Shadow/kurtosis run tells whether the vote-arrival distribution shifts; watch the vote ledger
  on the first instrumented run.
- **No gRPC fallback in kurtosis REST mode** (§1). Chain-client methods that delegate to the
  gRPC fallback fail there. Nothing on the duty path uses them, and the verification run was
  clean, but any future feature leaning on e.g. `ValidatorBalances` under REST must add a REST
  implementation first.
- **Version header for the new endpoints.** This plan says `heze`. If cross-tooling ever reads
  these fork-local endpoints expecting `gloas`, the constant is trivial to change; nothing else
  depends on it.
- **Batch semantics of the POST.** The VC always submits one attestation. The endpoint accepts a
  list for beacon-API symmetry; per-element delegation makes partial failure reporting slightly
  ad hoc (one IndexedErrorContainer Code for the whole response — §2.2's accepted
  Internal-as-400 consequence). If that ever matters, switch to a single-element body. The
  1-seat aggregate work in flight may add a second producer.
- **Builder preferences endpoint** is deliberately deferred (§2.5). If the buildoor/relay flow
  is ever run with a REST-mode VC, the real endpoint must be implemented first; until then the
  once-per-process warn is the only signal.
- **`AvailableAttestationDataRequest` is marked `option deprecated = true`**
  (proto/prysm/v1alpha1/validator.proto:1380) while being the live request type; unclear
  intent. The REST handler constructs one (§2.1), so the marker is cosmetic but misleading.
