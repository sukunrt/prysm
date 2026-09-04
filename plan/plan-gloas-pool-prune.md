# Gloas aggregates never leave the attestation pool

## Bug

Post-Gloas the aggregate topic decodes as `SignedAggregateAttestationAndProofGloas`
(`beacon-chain/p2p/gossip_topic_mappings.go:76`). `AggregateVal()` returns an
`*ethpb.AttestationGloas` (`proto/prysm/v1alpha1/attestation.go:809`). The pool saves it as is
(`beacon-chain/sync/subscriber_beacon_aggregate_proof.go:34`,
`beacon-chain/sync/pending_attestations_queue.go:368`).

`attestation.NewId` puts `att.Version()` in `id[0]`
(`proto/prysm/v1alpha1/attestation/id.go:36`). Every kv map and the seen-bits cache use it:
`kv/aggregated.go:139,266`, `kv/seen_bits.go:12,44`, `kv/block.go:17,61`.

Block import rebuilds each block attestation as an `*ethpb.AttestationElectra` dummy
(`beacon-chain/blockchain/process_block.go:775`) and calls `DeleteAggregatedAttestation`
(`kv/aggregated.go:248`). Electra key, Gloas entry: lookup misses at `kv/aggregated.go:269`,
returns nil, no log. Seen bits inserted at `kv/aggregated.go:256` get the Electra key too, so
`hasSeenBit` at `kv/aggregated.go:131` never blocks a re-save of the Gloas aggregate.

`packAttestations` (`rpc/prysm/v1alpha1/validator/proposer_attestations.go:32`) packs the
pool with no on-chain participation check, so included aggregates come back in later blocks.
Locally built aggregates are Electra (`aggregator.go:91`) and prune correctly.

## Fix: normalize to Electra on pool ingress

Use `ethpb.AttestationElectraFromAtt` (`proto/prysm/v1alpha1/attestation.go:377`). It is the
identity for Electra and copies for Gloas. `packAttestations` already assumes Electra entries
(`proposer_attestations.go:57-60`).

### Ingress points

| Site | Type today | Action |
|---|---|---|
| `sync/pending_attestations_queue.go:363` `saveAttestation` | Gloas via `:440` | convert here |
| `sync/subscriber_beacon_aggregate_proof.go:22-35` | Gloas | call `saveAttestation` |
| `sync/pending_attestations_queue.go:382` | Electra (`:283`) | no-op, same helper |
| `subscriber_beacon_attestation.go:35` | Electra (`validate_beacon_attestation.go:181`)| leave |
| `blockchain/process_block.go:604,607` block atts | Gloas | convert, one line |
| `rpc/prysm/v1alpha1/validator/attester.go:90-139` | Electra | leave |
| `rpc/eth/beacon/handlers_pool.go:408,412` | Electra (`:394`) | leave |
| `rpc/eth/beacon/handlers_pool.go:484-492` | Phase0 | leave |
| `rpc/eth/validator/handlers.go:432`, `rpc/core/validator.go:397` | broadcast only | leave |

The subscriber body duplicates `saveAttestation`. Replace it with one call and convert in
`saveAttestation`. RPC aggregate submits only broadcast; pubsub feeds the local subscriber.

Block attestations: convert at `process_block.go:604,607`. `HasAggregatedAttestation`
(`kv/aggregated.go:301`) also checks `hasBlockAtt` (`:367`) by the same key; an Electra query
must match. `BlockAttestations()` readers (`prepare_forkchoice.go:72,102`,
`prune_expired.go:75`) are type-agnostic and delete by the stored object.

### Seen check

`validate_aggregate_proof.go:120` calls `HasAggregatedAttestation(aggregate)`, and `:110`
`AggregateIsRedundant(aggregate)`. Both must get the converted attestation, or the check
misses the Electra pool entry and its seen bits. Convert into a local; keep
`msg.ValidatorData = m` (`:147`) as the Gloas signed aggregate for re-broadcast.

### Experimental pool

`beacon-chain/cache/attestation.go` keys `Add` (`:69`), `DeleteCovered` (`:146`) and
`AggregateIsRedundant` (`:199`) with `NewId(att, Data)`. Same version byte, same bug. The
fix applies unchanged because `saveAttestation` and `process_block.go:604` feed it.

### Why not change `NewId` or the dummy type

- `NewId` without the version byte mixes Electra and Gloas objects in one list.
  `maxcover.go:176-183` picks the output type from `atts[0].Version()`. Wider blast.
- A Gloas dummy in `process_block.go:775` prunes gossip entries but not the Electra entries
  from `aggregator.go:91` and `attester.go`. Two deletes and two seen-bit inserts needed.

### Downstream needs

Nothing reads a Gloas type back out of the pool. `setters.go:111` and
`rpc/eth/validator/handlers.go:72,115` convert with `AttestationGloasFromAtt`;
`kv/unaggregated.go:120`, `kv/aggregated.go:236`, `aggregator.go:91`,
`rpc/prysm/v1alpha1/beacon/attestations.go:527-533` and `ListAttestationsV2`
(`handlers_pool.go:74-78`, version header from the slot at `:103`) accept both.

## Tests

`go test ./beacon-chain/operations/attestations/kv/... ./beacon-chain/sync/... \
  ./beacon-chain/blockchain/...`

New:
- `kv/aggregated_test.go`: save `AttestationElectraFromAtt(HydrateAttestationGloas(...))`,
  delete with an Electra dummy of the same data and covering bits, assert
  `AggregatedAttestationCount() == 0`. Second subtest: save the raw Gloas attestation, same
  delete, assert count stays 1. This pins why ingress conversion is required.
- `sync/subscriber_beacon_aggregate_proof_test.go`: feed a
  `SignedAggregateAttestationAndProofGloas`; assert the single pool entry has
  `Version() == version.Electra` and equal bits and data.
- `sync/validate_aggregate_proof_test.go`: pool holds the Electra form; a Gloas gossip
  aggregate with the same bits gets `ValidationIgnore`.

Must keep passing: `TestKV_Aggregated_DeleteAggregatedAttestation`,
`TestKV_Aggregated_HasAggregatedAttestation`, `Test_pruneAttsFromPool_Electra`
(`blockchain/process_block_test.go:56`), `TestBeaconAggregateProofSubscriber_*`,
`TestValidateAggregateAndProof_ExistedInPool`, `TestValidateAggregateAndProof_CanValidate`.

## Sim verification (human, remote)

`shadow/run-shadow-sim.py --nodes 50 --validators 10000 --duration 96 \
  --target-committee-size 2500 --subnets 1` (plus the required `--supernode-fraction`).
The ledger flag is on for every run (`run-shadow-sim.py:146`). Logs:
`runs/<name>/data/node<N>/prysm/logs/beacon-chain.log`, logfmt.

Measure from `msg="FFG vote included"` lines (fields `attSlot`, `blockSlot`, `seats`):
1. Group by `blockSlot`. From slot 3 on, `attSlot` must be `blockSlot-1`, with stragglers
   allowed only when their `seats` is small relative to the committee.
2. For each `attSlot`, count distinct `blockSlot` values. Fail if any `attSlot` appears in
   3 or more consecutive blocks.
3. Poll `/eth/v2/beacon/pool/attestations` on a node at the end. Fail if any entry has
   `data.slot` older than the previous round (`--slots-per-round`, default 8).
`shadow/analysis/verify-summary.py runs/<name>/data 50` gives the ledger cross-checks.

## Open questions
- Pubsub local delivery for RPC-submitted aggregates is verified by inspection only.
