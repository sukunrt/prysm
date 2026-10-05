# Fixes discussed

Updated: 2026-10-05. This tracks the fixes and decisions from our discussion, including completed work and items explicitly deferred. Checkboxes describe implementation status, not commit status.

## Remaining fixes

- [ ] **Size GossipSub queues for the simulation workload.** Review validation, per-peer outbound, and topic subscription queues against the slot's burst of votes. Measure drops, processing delay, and memory use before choosing sizes; increasing capacity alone does not resolve slow consumers. Validation and outbound queues currently share `pubsub-queue-size` (CLI default 1000); topic buffers have separate settings, including 5000 for beacon attestations and four times the available-attestation committee size for available attestations.
  Code: [pubsub options](beacon-chain/p2p/pubsub.go), [subscription options](beacon-chain/sync/subscriber.go), [CLI flags](cmd/flags.go).

- [ ] **Remove quadratic seen-bit bookkeeping during attestation-pool compaction.** Deleting compacted singles currently inserts each bitlist into a growing list and scans the previous entries. The quadratic cost is within each attestation-data/committee group, not across the entire validator set. Use one accumulated bitlist for single-voter coverage instead of storing individual single bitlists. Preserve aggregate coverage semantics: seeing aggregates AB and BC does not mean an aggregate ABC signature is already available.
  Code: [seen bits](beacon-chain/operations/attestations/kv/seen_bits.go), [single deletion](beacon-chain/operations/attestations/kv/unaggregated.go), [compaction](beacon-chain/operations/attestations/kv/aggregated.go).

- [ ] **Expire old singles at slot start before proposer packing.** Agreed policy: remove singles from earlier slots, preserve completed aggregates and current-slot singles, and reject late arrivals for expired slots. Skipping those late attestations is explicitly acceptable. Coordinate expiry with insertion and the proposer's pool snapshot so expired singles cannot reappear. Use bulk expiry rather than the existing per-attestation deletion path that performs seen-bit insertion.
  Code: [attestation pool](beacon-chain/operations/attestations/kv), [packing](beacon-chain/rpc/prysm/v1alpha1/validator/proposer_attestations.go).

- [ ] **Bound attestation packing work and reduce pool lock contention.** Raw singles feed expensive deduplication and aggregation; these stages can continue beyond the request's cancellation deadline. `SaveAggregatedAttestation` also aggregates while holding the shared aggregate-pool write lock. After implementing the slot-start cutoff, measure the remaining work, make expensive packing stages respect cancellation, and narrow aggregation's lock scope while preserving concurrent updates.
  Code: [packing and deduplication](beacon-chain/rpc/prysm/v1alpha1/validator/proposer_attestations.go), [aggregate insertion](beacon-chain/operations/attestations/kv/aggregated.go).
  Existing evidence: [packing measurements](runs/diagnostics/round2-slots-0-16/packing-compaction-realwork-results.md), [compaction profile](runs/diagnostics/round2-construction-bench/old028-heavy13k-raw-compaction-profile.md).

- [ ] **Fix execution-payload timeout recovery.** Execution-client HTTP timeouts are mapped to `execution.ErrHTTPTimeout`, but the cached-payload recovery path checks only `context.DeadlineExceeded`. Align timeout handling so the intended recovery path is reachable for the actual error, and cover that error identity in the regression test. Respect an already-cancelled request when deciding whether recovery can proceed.
  Code: [error mapping](beacon-chain/execution/jsonrpc_error.go), [payload retrieval](beacon-chain/rpc/prysm/v1alpha1/validator/proposer_execution_payload.go), [existing timeout tests](beacon-chain/rpc/prysm/v1alpha1/validator/proposer_execution_payload_timeout_identity_test.go).

- [ ] **Finish migrating shuffling callers to `DependentRootAtEpoch`.** The method, interfaces, wrappers, tests, and attestation-state compatibility calls already exist. Remaining callers include data-column verification, proposer boost, payload bids, proposer preferences, and RPC shuffling checks. Pass the duty epoch directly instead of doing caller-side subtraction; preserve genesis and early-epoch behavior. This is API cleanup and prevention of future mistakes: the remaining old calls have not been established as correctness bugs merely because they use the old API.
  Code: [fork-choice API](beacon-chain/forkchoice/doubly-linked-tree/forkchoice.go), [data columns](beacon-chain/verification/data_column.go), [block validation](beacon-chain/sync/validate_beacon_blocks.go), [payload bids](beacon-chain/sync/validate_execution_payload_bid.go), [proposer preferences](beacon-chain/sync/validate_signed_proposer_preferences.go), [RPC checks](beacon-chain/rpc/core/validator.go).

## Deferred fixes and investigations

- [ ] **Make `GetAttestationData` use a consistent head snapshot.** Deferred correctness fix: head root, state, target, and payload status are obtained separately. A fork-choice root mismatch is logged, but the payload status from that mismatched root is still used. Derive these values from a consistent snapshot. Code: [attestation data RPC](beacon-chain/rpc/core/validator.go).
- [ ] **Investigate the previously discussed rollback behavior.** Deferred; retain as an investigation rather than asserting a diagnosed cause.
- [ ] **Revisit the active Goldfish electorate.** Deferred design/investigation item.
- [ ] **Consider precomputing FFG aggregator selection.** Candidate optimization, not an agreed implementation yet.
- [ ] **Consider epoch-based reuse in `getRecentPreState`.** Reusing a compatible state for a later round in the same epoch was discussed; the current branch still compares rounds. Leave this as an optional follow-up, not a reason to restart the broader refactor. Code: [attestation state helpers](beacon-chain/blockchain/process_attestation_helpers.go).
- [ ] **Consider skipping legacy LMD weight calculations after Heze.** Newly identified cleanup candidate: `Head()` still runs legacy balance/weight/best-descendant calculations before selecting Goldfish. Audit other consumers before bypassing that bookkeeping. Code: [fork-choice head](beacon-chain/forkchoice/doubly-linked-tree/forkchoice.go).

## Completed or settled

- [x] Count each validated FFG subnet vote as one in `countFFGVote(slot, subnet)`; remove the attestation parameter and the redundant aggregation-bit scan from the summary counter.
- [x] Restore the original `getAttPreState` structure; keep the compatibility checks on `DependentRootAtEpoch`. The broader refactor was deliberately dropped because the expected benefit was small.
- [x] Remove the round-zero exclusion in `getRecentPreState` and add the small regression test. It checks reuse for a genesis target with heads in rounds zero and one, and rejection once stale in round two. The focused tests passed; restoring the exclusion through a temporary overlay made the positive cases fail.
- [x] Reuse the cached active-validator count at slot zero. Keep the separate uncached `ActiveValidatorCountAtGenesis` path for mutable genesis construction. A supplied genesis state bypasses that construction path; an ordinary cold cache can still require a scan.
- [x] Randomize Goldfish committee selection.
- [x] Defer sync-committee aggregator selection so it does not block validator duty assignment.
- [x] Check the sync-committee head-state cache before taking the shared lock.
- [x] Use per-domain locks for validator domain-data requests.
- [x] Normalize Gloas attestations to the Electra representation in the aggregation-pool path.

## Decisions to preserve

- Keep `ForkChoice.ProcessAttestation`'s legacy `f.votes` freshness comparison epoch-based. It was deliberately left unchanged for the pre-Heze LMD walk. Goldfish head selection uses a separate per-slot available-vote store; changing this comparison to rounds is not a fix for Goldfish.
- FFG participation is recorded in beacon-state participation flags when attestations are included in blocks, and justification/finalization runs per round. Gossip subscribers save those attestations to the pool before proposer packing. The legacy `f.votes` array is not the FFG finality record.

## Consider later

- [ ] **Reduce linear scans in attester validation.** Deferred for now. The pre-Electra `Count()` / `BitIndices()` check makes multiple passes over the committee-sized bitlist; `BitIndices()` calls `Count()` internally. Our single-attestation path instead uses `slices.Contains(committee, attestingIndex)`, which is also O(n) in committee size. Consider a cached validator-to-committee-position map built once per committee, and simplifying the legacy bitfield check. Code: [attester validation](beacon-chain/sync/validate_beacon_attestation.go).
