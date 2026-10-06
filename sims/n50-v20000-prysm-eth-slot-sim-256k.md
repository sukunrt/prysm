# Prysm / eth-slot-sim comparison: 50 nodes, 20k validators

2026-10-06. Completed comparison with 40 home nodes, 10 supernodes, one subnet,
eight live slots, ePBS and decoupled consensus. Statistics below use **only the
final measured slot: Prysm slot 8 and eth-slot-sim slot 7**.

## Configuration and payloads

Both runs use the same Shadow network graph and node placement, seed 1, home
bandwidth 25 Mbit/s up and 50 Mbit/s down, and supernode bandwidth 1024 Mbit/s
in both directions. There are 87 validators on the home nodes and 19,913 on the
supernodes. FFG votes start at the slot boundary, with no spread flag and zero
configured jitter. eth-slot-sim uses validator segregation (staggered finality
rounds), with individual votes each slot. FFG aggregates are due at **9 seconds**
in both runs. The requested PTC deadline is **6 seconds**; the completed run instead
used 9 seconds. The user explicitly deferred fixing PTC, so no PTC behavior or
deadline changes have been made.

The payload targets are independently configured at approximately 256 KiB.
eth-slot-sim's size is not derived from Prysm's output.

| Final-slot measurement | Prysm, slot 8 | eth-slot-sim, slot 7 |
|---|---:|---:|
| Execution payload bytes | 250,141 | 262,144 |
| Execution payload KiB | 244.3 | 256.0 |
| Actual block gas limit | 198,830,992 | Not modeled |
| Gas used | 16,073,640 | Not modeled |
| Transactions | 17 (15 EOA + 2 blob) | Synthetic payload |
| Blobs | 6 | 6 |

Prysm's payload is 4.58% smaller. Prysm reports execution-payload SSZ size;
eth-slot-sim configures 262,144 filler bytes with a small protobuf overhead on
the wire. These sizes exclude blob sidecars and transport framing.

Spamoor produces EOA transactions with 16,384 bytes of random calldata, targeting
16 transactions per slot, 32 wallets and 64 pending transactions. Each transaction
has a 1,150,000 gas cap. A separate Spamoor instance targets two blob transactions
per slot, three blobs each, with eight pending transactions. This targets payload
size through the offered workload; it does not guarantee an exact byte count.

## Final-slot arrivals

All values are **milliseconds from slot start**, pooled over receiver-message
observations, excluding publisher self-receipts. p99 uses continuous interpolation.
These are arrival times, not publication-to-receipt network latencies.

| Message | Prysm avg | Prysm p99 | eth-slot-sim avg | eth-slot-sim p99 |
|---|---:|---:|---:|---:|
| Consensus block | 103 | 223 | 141 | 263 |
| Execution payload | 977 | 2,857 | 1,752 | 2,766 |
| Data column readiness | 90 | 205 | 775 | 912 |
| Availability vote | 590 | 1,737 | 371 | 837 |
| FFG vote | 378 | 1,765 | 319 | 1,428 |
| FFG aggregate | 9,062 (one observation) | Not meaningful | 9,129 | 9,253 |
| PTC vote | 901 | 2,372 | 9,159 | 9,300 |

Interpretation limits:

- **PTC publication differs.** Prysm wakes on `execution_payload_available`, or
  at the 9-second deadline. eth-slot-sim arms a fixed 9-second timer and does not
  attempt an early PTC vote on payload/custody readiness. Its 9.16-second average
  is mostly scheduled waiting. This is a simulator behavior mismatch for a Prysm
  comparison, not evidence of nine seconds of network transit. Both that scheduling
  mismatch and the incorrect 9-second PTC deadline remain unfixed at the user's
  request; the intended PTC fallback deadline is 6 seconds. See
  [Prysm wait helper](../validator/client/wait_helpers.go) and
  [eth-slot-sim runner](../../eth-slot-sim/driver/runner.go).
- eth-slot-sim delays payload and column publication by 500–1000 ms. Prysm column
  readiness includes local reconstruction. Column timings are not identical events.
- Prysm records accepted gossip votes; eth-slot-sim records receipts after modeled
  validation. Prysm accepted only one remote FFG aggregate into this ledger, while
  eth-slot-sim recorded 490 receipts. A comparable Prysm aggregate p99 is unavailable.
- **Prysm slot 8 only** recorded 122,410 of 122,500 expected remote individual FFG receipts: 90
  missing records, all at node 12. Saved Prometheus metrics identify 90 FFG
  validation-throttle rejections at that receiver; see the audit below.
  **eth-slot-sim slot 7 only** recorded all 121,667 expected receipts for its 2,483
  final-slot voters. Neither count pools earlier slots.
- For the directly matched FFG publication-to-receipt measure, Prysm avg/p99 are
  **372/1,759 ms** and eth-slot-sim **318/1,427 ms**.

## Individual FFG receipt audit: slot 7 versus slot 8

| Prysm slot | Published votes | Expected remote receipts | Observed remote receipts | Missing | Last observed arrival |
|---|---:|---:|---:|---:|---:|
| 7 | 2,500 | 122,500 | 122,500 | 0 | 2,839 ms |
| 8 | 2,500 | 122,500 | 122,410 | 90 | 4,998 ms |

Each row is one slot, not a cumulative count. There are no duplicate receiver/vote
records and no parser failures. In slot 8, all 90 missing receipts are at node 12:
79 votes originated at node 3, one at node 19, and ten at node 31. The other 48
remote receivers accepted each of those votes. Node 12's last accepted individual
vote arrived at 436 ms; at 9,062 ms it accepted a full 2,500-seat aggregate.

The saved Prometheus samples bracket slot 8: the preceding sample contains all
slot 7 receipts, and the next sample is about seven seconds into slot 8. On node
12's `beacon_attestation_0` topic:

| Counter | Before slot 8 | During slot 8 | Increase |
|---|---:|---:|---:|
| `p2p_pubsub_validate_total` | 19,496 | 21,996 | 2,500 |
| `p2p_pubsub_deliver_total` | 17,498 | 19,908 | 2,410 |
| `p2p_pubsub_reject_total{reason="validation throttled"}` | Series absent | 90 | 90 first recorded |

This identifies validation throttling as the cause of the 90-record gap. The
metrics do not distinguish the global concurrency throttle from the per-topic
throttle. This rejection reason is distinct from the validation queue-full reason.

The libp2p validation pipeline marks a message as seen **before** checking async
validation capacity. When the concurrency throttle rejects it, later copies are
duplicates and do not retry validation. Therefore, spare seconds in the slot do
not guarantee recovery of the individual message. The aggregate provides a
separate route for recovering its voting weight.

These runs reused the earlier binary (`0e2b5846...`, recorded workspace revision
`9ec3f5f343a3547d44989d23da4c1e455fc8f584`). It did not contain the user's subsequent
queue changes:

| Setting | Completed run | Current source |
|---|---:|---:|
| CLI pubsub validation/outbound queue | 1,000 | 20,000 |
| Global validation concurrency | 8,192 | 50,000 |
| Per-topic validation concurrency | 1,024 | 50,000 |
| FFG subscription buffer | 5,000 | 4,096 |

The concurrency increases directly address the observed rejection path. The
subscription buffer change is separate and is not the cause identified here.
The completed rerun with current-source binaries recorded **122,500 / 122,500**
individual FFG receipts in Prysm slot 8, with zero missing records. Its execution
payload was 266,691 bytes. This supports the throttle diagnosis; the gas-limit
verification still failed as described below.

Evidence: [receipt audit](../shadow/runs/compare-n50-v20000-s1-agg9s-payload256k-gas200m-20261006/artifacts/last-slot/ffg-slot7-slot8-audit.json),
[saved metric query](../shadow/runs/compare-n50-v20000-s1-agg9s-payload256k-gas200m-20261006/artifacts/last-slot/node12-ffg-prometheus.json).

## Gas-limit configuration miss and prevention

I checked the 200,000,000 genesis gas limit and Geth's
`--miner.gaslimit 200000000`, but did not initially trace the validator's proposer
preferences or verify the final produced block's gas limit. That was incomplete
configuration validation.

Prysm's `proposerConfigForKey` uses `Settings.TargetGasLimit`: per-validator
top-level `gas_limit`, then `default_config.gas_limit`, then the chain default
(60,000,000 here). The beacon node passes that preference in the Engine API's
`PayloadAttributesV4.TargetGasLimit`. Geth's miner flag alone therefore did not
hold the target at 200M. The final block drifted down to **198,830,992**.

For these runs, explicitly supply this v2 file to every validator with
`--proposer-settings-file <path> --suggested-gas-limit 200000000` while retaining
the 200M genesis and Geth settings:

```json
{
  "version": 2,
  "default_config": {
    "fee_recipient": "0xf97e180c050e5Ab072211Ad2C213Eb5AEE4DF134",
    "gas_limit": "200000000"
  }
}
```

The top-level v2 field controls proposer preferences. **The explicit CLI flag is
also necessary with the current loader:** `WithGasLimit` reads the CLI default
even when the user did not explicitly set the flag, and the v2 merge overwrites
the file's gas limit. The first v2-file rerun therefore still drifted to
198,830,992. The 100-node run supplies both values explicitly. A nested builder registration
gas limit must not be assumed to control this path. Preserve the intended fee
recipient when preparing a settings file for another run.

Before reporting a gas target as achieved, check the generated arguments for every
validator and Geth process, then check `gasLimit` in the actual EL block against
`gas_limit` in its execution payload envelope and verify that their hashes match.
The corrected run driver asserts this for all eight produced blocks.

Source: [target resolution](../config/proposer/settings.go),
[validator preference](../validator/client/validator.go),
[Engine payload attributes](../beacon-chain/rpc/prysm/v1alpha1/validator/proposer_execution_payload.go).

## End-to-end evidence and artifacts

Final Prysm slot 8: all 50 nodes imported the same block and payload; 17 EL
transactions; one beacon FFG attestation; one `payload_attestations` aggregate;
10 local FFG aggregates, each covering 2,500 seats and published at 9,000 ms;
23,079 accepted gossip PTC vote records, all with payload and blobs present.
All nodes recorded 512 availability seats. Both Shadow processes exited successfully.

Completed run artifacts:
[results JSON](../shadow/runs/compare-n50-v20000-s1-agg9s-payload256k-gas200m-20261006/artifacts/last-slot/results.json),
[avg/p99 CSV](../shadow/runs/compare-n50-v20000-s1-agg9s-payload256k-gas200m-20261006/artifacts/last-slot/avg-p99.csv),
[manifest](../shadow/runs/compare-n50-v20000-s1-agg9s-payload256k-gas200m-20261006/artifacts/manifest.json).

Remote root: `ethp2p:/home/sukun/dev/slot-compare-20261006-n50-v20000-agg9s-payload256k-gas200m`.

A Prysm run with explicit v2 proposer preferences and the current queue
changes completed in the corresponding `-gas200m-v2` remote root. Its first
preflight stopped before Shadow because the generator assigned different numeric
IDs to the same graph locations. The labelled graph, host IPs and location mappings
were verified equal, and the driver now reuses the original graph and numeric
mapping exactly. PTC remained unchanged for that rerun. Its queue check passed,
but the CLI gas override above still needed correction. The comparison table
above remains the original run, not the rerun.
