# Round 2 slot 110 retained timeline

Slot 110 began at `2026-09-05T01:52:00Z`. A targeted scan of the compact
archives for validator-bearing nodes 4–149 identified node 148 as the proposer.
Its scheduled pubkey prefix was `0xa929a06ad750` and its validator index was
39774. The selected archive is
[`round2-slot110-owner-node148.tar.gz`](round2-slot110-owner-node148.tar.gz),
SHA-256 `18a7829ec2dfb63df7d8c97a02ccafff1750172182f3daa197813e82513f1d56`.
The extraction is reproducible with
[`extract_slot110_timeline.py`](extract_slot110_timeline.py); its complete
machine-readable result is [`round2-slot110-timeline.json`](round2-slot110-timeline.json).

## Timeline

Offsets below are relative to the slot-110 start. Source line numbers count
physical LF-delimited records; embedded CR bytes in the retained validator log
do not create extra line numbers.

| Offset | Event | Raw anchor |
|---:|---|---|
| -11.026 s | Parent slot 109 imported, root `0x8f962346…` | `beacon.log:931` |
| -10.604 s | Parent-109 envelope imported, execution hash `0x460535…` | `beacon.log:933` |
| -10.600 s | Geth had payload ID `0x0460a255558bbf2d` ready: block 75, hash `0x3d8b75…`, 0 tx, 0 gas, `elapsed=1.481ms` | `execution.log:338` |
| +16.984 ms | `Building block`, slot 110 | `beacon.log:934` |
| +23.183 ms | Engine `getPayloadV6` request #1180, JSON-RPC ID 672, same payload ID | `snooper-engine.log:49465-49474` |
| +23.704 ms | Eth1-data fallback branch logged | `beacon.log:935` |
| +24.144 ms | Geth stopped work on the payload, `reason=delivery` | `execution.log:339` |
| +24.961 ms | `Chose payload bid`, self-build, value 0 | `beacon.log:936` |
| +26.670 ms | Proxy captured response #1180: block hash `0x3d8b7508…`, parent hash `0x460535465…`, empty transactions, `gasUsed=0`, empty execution requests | `snooper-engine.log:49475-49511` |
| +307.372 ms | Xatu sentry reported its local queue full, event type 19, 14,990 suppressed | `xatu-sentry.log:1310` |
| +1.725043 s | Beacon event server disconnected a client that could not keep its outgoing buffer below the threshold | `beacon.log:937` |
| +3.409893 s | `Finished building block`, reported `sinceSlotStartTime=3.409432868s` | `beacon.log:938` |
| +3.639960–3.861027 s | The 12 retained observers imported root `0xf287cca3…`; owner import was +3.663232 s | node 300 `beacon.log:750`; owner `beacon.log:939`; node 1 `beacon.log:1331610` |
| +3.663287 s | Owner finished applying the imported block: 8 attestations, self-build payload, 511 sync bits | `beacon.log:940` |
| +3.666046 s | Owner published the execution-payload envelope; logged duration 272.6 µs | `beacon.log:941` |
| +3.666680 s | Validator submitted the block: 8 attestations, 1 payload attestation, root `0xf287cca3…` | `validator.log:2618` |
| +3.668260 s | Engine `newPayloadV5` request #1181 with the same empty execution payload | `snooper-engine.log:49512-49544` |
| +3.670594 s | `newPayloadV5` response: `VALID`, matching latest-valid hash | `snooper-engine.log:49545-49555` |
| +3.673015 s | Owner imported the execution-payload envelope | `beacon.log:942` |
| +3.673573–3.678164 s | Engine `forkchoiceUpdatedV4` #1182 returned `VALID` | `snooper-engine.log:49556-49585` |
| +4.379212 s | Xatu upstream batch send timed out | `xatu-sentry.log:1311` |

The proxy header says `duration_ms=0` for `getPayloadV6`. Its outer request and
response capture timestamps differ by 3.487 ms, but the response capture ends
after the beacon node's payload-choice record. These are independently emitted
logs, so their ordering does not measure literal client RPC latency. Geth's own
record proves the candidate was already ready 10.600 seconds before slot start.

The measured slow interval is therefore after the payload was selected: about
3.385 seconds from `Chose payload bid` to `Finished building block`. This compact
owner archive contains INFO/WARN beacon records and no detailed FFG vote-ledger
rows or internal block-construction spans, so it cannot split that interval among
attestation packing, reward processing, state-root work, scheduling, and logging.
The slow-reader warning and Xatu queue failure occurred during the interval and
show real concurrent activity; temporal overlap alone does not establish either
as the cause. Similar slow-reader warnings occur repeatedly elsewhere in this
node's run.

The actual block-body evidence is analyzed separately in
[`slot110-block-inputs.md`](slot110-block-inputs.md): the block had eight FFG
attestations covering 97,093 validator positions and 511 sync bits; five fully
logged vote identities repeated from blocks 108/109 cover 56,097 positions.
Those observer-ledger records constrain the body, not node 148's exact pool
snapshot. The old-source comparison is in
[`slot110-source-comparison.md`](slot110-source-comparison.md).

At `01:52:12.004516172Z`, node 148 reorged its head from slot 110 back to slot
109 before importing slot 111; that later event does not explain the already
completed slot-110 construction interval.
