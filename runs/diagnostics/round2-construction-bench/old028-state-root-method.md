# Exact-old state transition and root benchmark boundary

The authoritative Go runs use revision `0280403c70d88967f49d2d4c730f4c5417dabdf5` in `/home/sukun/dev/prysm2-round2-construction-028`. The checkout adds diagnostic test files only; production code is unchanged.

## Timed paths

- `calculate_post_state` times `transition.CalculatePostState`: copying the 120,000-validator pre-state, the already-at-slot cache check, and the full block transition. The production implementation copies at `beacon-chain/core/transition/transition_no_verify_sig.go:145-146`, checks/processes slots at `:148-154`, and takes `ProcessBlockForStateRoot` because proposer preprocessing is disabled at `:156-167`.
- The block transition includes parent-payload handling, body-root and header processing, withdrawals, execution-payload-bid handling, RANDAO state mutation, Eth1 processing, block operations, and the empty sync aggregate (`beacon-chain/core/transition/transition_no_verify_sig.go:421-536`). Gloas operations dispatch to attestation processing at `beacon-chain/core/transition/gloas.go:62-103`; each included attestation is validated, resolves committees and participant indices, updates pending payment weight, participation, and proposer reward at `beacon-chain/core/altair/attestation.go:39-107`. Attestation BLS verification is excluded by this production state-root branch.
- `hash_each_fresh_post_state_once` performs `CalculatePostState` with the timer stopped, then times one `HashTreeRoot` call on that newly produced post-state. The production root implementation initializes/reuses state Merkle data and recomputes dirty fields at `beacon-chain/state/state-native/state_trie.go:1221-1241`.
- The `clean_pre_state` root arm starts from a slot-97 state rooted once outside the timer. The `residual_slot_advance_dirty` arm starts from an independent slot-97 state that retains the root-ring, header, slot, and payload-availability changes left by one real slot-96-to-97 advance; its base is never rooted directly. `ProcessSlot` creates those changes at `beacon-chain/core/transition/transition.go:109-159`.
- `full_proposer_wrapper` times the mock `StateGen` map lookup, `CalculatePostState`, one post-state root, and the wrapper's debug-log call through the deployed proposer path (`beacon-chain/rpc/prysm/v1alpha1/validator/proposer.go:637-669`). Every fixture asserts that the block's attestation count is unchanged and that no `Retrying block construction` log was emitted. This catches the deployed fallback that removes attestations at `proposer.go:679-690`.

## Fixtures and exclusions

All fixtures use native Heze state, 120,000 active validators with distinct deterministic BLS keys, coherent current/next sync committees, configured Gloas-to-Heze fork data, a proposer-signed RANDAO reveal, an empty-parent self-build bid, and a structurally accepted empty sync aggregate. Setup is at `beacon-chain/rpc/prysm/v1alpha1/validator/historical_state_root_diagnostic_test.go:51-335`; benchmark timer boundaries are at `:337-413`.

- Empty: zero attestations.
- Compact 3: three valid on-chain attestations produced by the deployed packer from the retained vote shapes, covering 44,847 participant positions. The old votes were already credited in the fixture; the fresh vote was not.
- Density 8: a synthetic maximum-density control, not a historical replay. It uses eight distinct valid votes at slots 88-95 and 120,000 controlled participant positions. The historical slot-97 block had eight attestations, but the 120,000-position density was not observed.

Key generation, state construction, signature generation and preflight verification, pool population, proposer packing, the one-time slot-96-to-97 advance, committee/cache warming, and fixture validation are outside timed regions. The benchmarks also exclude builder/engine requests, execution RPC, fork choice, network receive/broadcast, validator-client submission, and the proposal packer's pool snapshot, validation, aggregation, and reward sort. Parent payload application takes the empty-parent path, and parent execution requests are empty.

Validation logs: `old028-state-root-compact3-pilot.log` and `old028-state-root-density8-pilot.log`. Final timing logs use the same `old028-` prefix in this directory.

## Actual slot-96 ledger timing boundary

The bounded node-1 census in `node1-slot96-ffg-timing.json` scanned the complete retained `beacon.log` and found 13,947 `FFG vote` rows. They represent 13,947 unique validators and 13,947 unique `(validator, dataRoot, committee)` tuples, with no duplicate tuple rows. All ledger `arrivedMs` values were 124-4,144 ms, and all outer log emissions were 131.136-4,156.980 ms after the 01:49:12Z slot start. Thus all 13,947 fall in the requested `<=7s` bucket for both clocks; the five later buckets are zero.

Twelve aggregate emissions appeared 8,035.044-8,113.861 ms after slot start, covering eight of the ten single-vote `(dataRoot, committee)` groups. Those eight groups account for 13,925 of the 13,947 local single rows. The two unmatched minority groups are committee 0/root `ff74743b...` with 9 rows and committee 1/root `15e07dc1...` with 13 rows. Aggregate seat counts can exceed the matching node-1 single-row count because a gossip aggregate may contain votes that this observer did not log as singles. These ledger times do not prove subscriber insertion time or membership in the proposer pool snapshot.
