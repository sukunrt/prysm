# Geth rollback: retained Engine wire comparison

## Scope

This compares the Engine JSON actually retained for Round 1 with the historical Round 2 interval at or after `2026-09-05T01:30:00Z`. Round 1 had Geth builds `ff083d45` and `d799b1a3`; Round 2 had the older `aa1f2fcf`. The source comparison uses the retained Git objects for `aa1f2fcf..d799b1a3`, focusing on glam8 commit `26d0b2171` and its hive fix `fd073543c`.

## Observed wire shapes

| Method and field | Round 1 | Round 2 | Observed difference |
|---|---:|---:|---|
| `engine_newPayloadV5` requests | 20 | 1,411 | Count only |
| Positional parameters | all 4 | all 4 | None |
| Parameter types | payload object, empty versioned-hash list, 32-byte parent-beacon-root string, empty execution-request list | same | None |
| Payload keys | same 19-key set | same 19-key set | None |
| `blockAccessList` | present and nonempty in all 20 | present and nonempty in all 1,411 | None |
| `slotNumber` | present in all 20 | present in all 1,411 | None |
| `blobGasUsed` / `excessBlobGas` | present and zero in all 20 | present and zero in all 1,411 | None |
| `engine_getPayloadV6` responses | 5 | 8 | Count only |
| Envelope keys | `blobsBundle`, `blockValue`, `executionPayload`, `executionRequests`, `shouldOverrideBuilder` | same | None |
| Execution-payload keys | same 19-key set as `newPayloadV5` payload | same | None |
| `executionRequests` | empty in all 5 | empty in all 8 | None |
| `blobsBundle` arrays | blobs, commitments, and proofs all empty | same | None |
| `parentBeaconBlockRoot` inside returned execution payload | absent in all 5 | absent in all 8 | None; it is the third `newPayloadV5` argument |

The shared 19-key execution-payload set is: `baseFeePerGas`, `blobGasUsed`, `blockAccessList`, `blockHash`, `blockNumber`, `excessBlobGas`, `extraData`, `feeRecipient`, `gasLimit`, `gasUsed`, `logsBloom`, `parentHash`, `prevRandao`, `receiptsRoot`, `slotNumber`, `stateRoot`, `timestamp`, `transactions`, and `withdrawals`.

For `engine_forkchoiceUpdatedV4`, Round 1 retains 40 requests: 33 head-only calls with null attributes and seven build calls with an attribute object. Round 2 retains 1,713 requests in the selected interval: 1,705 head-only calls and eight build calls. Every non-null attribute object in both rounds includes `targetGasLimit: 0x3938700`; no non-null object omits it. Thus the source change that removed Geth's FCUv4 requirement for `targetGasLimit` did not produce a field-shape difference in these captured calls.

## Relevant source changes

The Go JSON structures and tags are unchanged across `aa1f2fcf..d799b1a3`: `ExecutableData` already contains `blockAccessList` and `slotNumber`, while `ExecutionPayloadEnvelope` already contains `executionPayload`, `blockValue`, `blobsBundle`, `executionRequests`, `shouldOverrideBuilder`, and optional `witness`.

Commit `26d0b2171` changes validation and fork routing rather than the JSON schema:

- it treats a block access list as present for hashing and decoding only when `len(BlockAccessList) > 0`, rather than whenever the byte slice is non-nil;
- it removes the mandatory `targetGasLimit` check from FCUv4;
- it moves BPO3 through BPO5 from FCUv3/getPayloadV5/newPayloadV4 routing to FCUv4/getPayloadV6/newPayloadV5 routing, and includes Bogota in the latter route.

Commit `fd073543c` then makes newPayloadV4 reject Amsterdam-only `slotNumber` and `blockAccessList`, makes newPayloadV5 require `len(BlockAccessList) > 0`, delays block-access-list decoding until after the block-hash check, and maps a malformed access list to JSON-RPC invalid params.

Every captured `newPayloadV5` request in both rounds has a nonempty serialized `blockAccessList`, so the hive-fix empty-list rejection condition is not observed. The retained wire data also shows no Round 1/Round 2 difference in the fields relevant to these changes. This is a schema and value comparison only; it does not attribute either run's behavior to these commits.

## Genesis configuration distinction

The startup logs contain a separate genesis/configuration difference. Round 1 node 1 advertises BPO1 at genesis with blob target/max 10/15, then BPO2 at genesis with 14/21. Round 2 node 1 advertises only BPO1 at genesis with 14/21. Amsterdam is at genesis in both. Both therefore end at the same effective 14/21 blob target/max, but under different schedule names. This configuration difference is not a binary source diff.
