# Why the data-column timings differ

The original 100-node comparison has matching node classes, link geography, bandwidth,
blob count and custody counts, but the two systems do not yet model the same
column publication and acquisition process. Matching the approximately 256 KiB
execution payload does not match the data columns, which are determined by the
blobs and sidecar encoding.

Unless labeled otherwise, the measurements below are only from the completed
`-fullmesh` baseline's final slot: **Prysm slot 8 / eth-slot-sim slot 7**. They
precede the `-agglog` rerun, whose results are recorded at the end of this document.

## Publication time accounts for most of the apparent gap

| Measurement | Prysm | eth-slot-sim |
|---|---:|---:|
| Proposer column publication, ms into slot | Approximately 15 | 966.014 |
| Column records, excluding proposer | 3,072 | 3,072 |
| Mean, ms into slot | 106.636 | 1,117.556 |
| p99, ms into slot | 217.000 | 1,276.592 |
| Mean after recorded publication, ms | Not directly comparable across local/gossip sources | 151.542 |
| p99 after recorded publication, ms | Not directly comparable across local/gossip sources | 310.579 |

eth-slot-sim schedules payload reveal at 500–1,000 ms and publishes the columns
with that reveal. The last slot used 966.014 ms. Prysm's self-built payload has
no equivalent configured reveal delay. Its proposer logged the 128-column batch
at 14.791–14.910 ms. Those Prysm debug times are recorded during batch preparation,
immediately before `PublishBatch`; they are not per-packet transmission timestamps.

Thus the old 107 ms versus 1,118 ms means included approximately 951 ms of
different publication timing. That difference alone is not evidence of slower
network propagation in eth-slot-sim. Subtracting the offset describes the recorded
run; actually moving publication earlier also changes contention with slot-start
FFG traffic, so it is not a prediction of the next run's propagation time.

Code: [eth-slot-sim payload/column publication](../../eth-slot-sim/driver/runner.go),
[Prysm envelope publication](../beacon-chain/rpc/prysm/v1alpha1/validator/proposer_payload_envelope.go),
[Prysm batch timing](../beacon-chain/p2p/broadcaster.go).

## Prysm has an execution-layer source for columns

| Prysm final-slot outcome | Records | Nodes | Mean ms into slot | p99 ms into slot |
|---|---:|---:|---:|---:|
| Gossip | 1,275 | 62 | 117.851 | 233.260 |
| Local | 1,797 | 73 | 98.679 | 202.000 |

58.5% of the recorded columns were local. All 73 nodes with local column records
also logged construction from their execution client. In total, 79 nodes logged
successful execution-client construction; some already had all needed columns.
There were no peer-reconstruction completion logs for this slot.

Prysm can fetch blobs already present in Geth's transaction pool, construct the
columns after receiving the consensus block/bid, and publish them to peers.
eth-slot-sim has no equivalent Geth blob cache or local construction path; its
column records all come from the modeled gossip path. This also means that
restricting Prysm analysis to gossip records does not remove the effect: peers
can gossip columns they obtained from their own EL.

Code: [construction from the EL and local logging](../beacon-chain/sync/subscriber_beacon_blocks.go).

## The timestamps represent different processing stages

- Prysm gossip records use the timestamp captured at **validation entry**. The
  log is written after acceptance, but its `arrivedMs` excludes subsequent
  validation. Deferred Gloas processing retains the original arrival time.
- Prysm local records are timestamped **after construction and storage**.
- eth-slot-sim records **subscription delivery after modeled validation**. Each
  column takes a configured 3 ms verification service, with 16 parallel slots on
  supers and four on homes; queueing can add more.

The previous row named “data column readiness” therefore mixes arrival and
completion timestamps. It should not be interpreted as a uniform readiness metric.

Code: [Prysm validation timestamp](../beacon-chain/sync/validate_data_column.go),
[deferred Gloas timestamp](../beacon-chain/sync/validate_data_column_gloas.go),
[eth-slot-sim verification](../../eth-slot-sim/node/column_verifier.go),
[eth-slot-sim receipt timestamp](../../eth-slot-sim/node/node.go).

## The encoded sizes and paths also differ

| Encoded column payload, excluding libp2p framing | Prysm | eth-slot-sim |
|---|---:|---:|
| Before compression, bytes | 12,632 (Gloas SSZ) | 13,220 (protobuf model) |
| Bytes carried as gossip message data, mean | 9,765.492 | 13,220 |
| Minimum–maximum bytes | 8,797–10,730 | 13,220–13,220 |

The Prysm values were measured by Snappy-encoding all 128 stored slot-8 columns
with the same encoder used by gossip. eth-slot-sim's `ssz_snappy` topic suffix is
only a name: its protobuf column payload is uncompressed. Its current size
formula also differs from the Gloas SSZ structure. Spamoor's default blob
workload can generate zero, repeated or duplicate blobs as well as random data,
so the compressed sizes need measuring per run.

Both give each home eight columns, but only **35 of 640** home-node/column pairs
match. Their per-topic gossip paths therefore differ. Final proposers also
differ: Prysm node 79 and eth-slot-sim node 53, both supers. The saved Prysm peer
addresses use TCP; the eth-slot-sim Shadow backend uses QUIC.

The actual `Subscribed to` beacon logs confirm exactly eight data-column
subnets for each of the 80 Prysm homes. Their initial subscriptions also match
in the ongoing 16-slot rerun, which reuses the same peer identities. Examples
(Prysm node numbering, column indices starting at zero):

| Home | Column / subnet indices |
|---|---|
| node2 | 0, 25, 61, 62, 83, 93, 111, 125 |
| node3 | 11, 17, 44, 52, 92, 100, 114, 127 |
| node4 | 2, 30, 51, 53, 76, 82, 84, 120 |

Subscription selection uses the node's discovery ID and a sampling size equal
to the maximum of `SAMPLES_PER_SLOT`, validator custody requirement and stored
custody count. That size is eight for these homes, and the selected set does
not rotate with the slot. There are 128 custody groups, 128 columns and 128
column subnets here, so the group, column and subnet indices coincide. Supers
subscribe to all 128. The one-subnet experiment setting applies to FFG votes,
not data columns.

Evidence: [all 80 home subscriptions, CSV](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/last-slot/home-column-subscriptions.csv),
[original subscription log lines, JSON](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/last-slot/home-column-subscriptions.json),
[subscription selection](../beacon-chain/sync/subscriber.go#L773).

eth-slot-sim likewise assigns eight fixed columns to every home and all 128 to
each super in this configuration. Its assignment uses a seeded random sample,
not Prysm's discovery-ID calculation. The generated 8-slot and 16-slot schedules
have identical column subscriber lists. For corresponding homes (eth-slot-sim
numbers nodes from zero), Prysm node2 / eth node1 has eth columns
44, 51, 53, 56, 60, 75, 123, 127; Prysm node3 / eth node2 has
8, 9, 55, 72, 78, 106, 112, 115; Prysm node4 / eth node3 has
5, 22, 24, 69, 83, 88, 111, 120. Across all 80 matched homes, only 35 of the
640 node-column assignments coincide. See the
[full side-by-side assignment CSV](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/last-slot/compared-home-column-subscriptions.csv).

Code: [Gloas SSZ size](../proto/prysm/v1alpha1/gloas.ssz.go),
[Prysm Snappy encoder](../beacon-chain/p2p/encoder/ssz.go),
[eth-slot-sim column builder](../../eth-slot-sim/validator/column.go).

## A controlled comparison

First align the publication schedule and record both validation-entry and
validation-completion times. Compare gossip and local acquisition separately.
Then align encoded column size, compression, proposer location and custody
assignments. An end-to-end comparison must also give eth-slot-sim an equivalent
EL blob-availability path; alternatively, a separate gossip-only experiment can
exclude EL acquisition in both systems. Changing only the analysis filter cannot
create that experiment because locally constructed columns are rebroadcast.

These are proposed follow-up controls. The logging rerun kept the previous
simulation behavior to diagnose the ignored aggregates.

Evidence and reproducible size measurements are saved under
[`column-analysis/`](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/column-analysis/).

## Completed logging rerun

The `-agglog` rerun completed on ethp2p with the ignored-aggregate logs enabled
unconditionally. The same completed eth-slot-sim run is reused. Final-slot
measurements remain Prysm slot 8 / eth-slot-sim slot 7.

| Measurement | Prysm rerun | eth-slot-sim |
|---|---:|---:|
| Proposer column batch preparation, ms into slot | 15.334–15.452 | 966.014 publication |
| Column records, excluding proposer | 3,072 | 3,072 |
| Mean, ms into slot | 101.072 | 1,117.556 |
| p99, ms into slot | 261.290 | 1,276.592 |
| Blobs in the final slot | 9 | 6 |
| Mean encoded bytes per column | 18,085.578 | 13,220 |

The same publication offset remains. Prysm recorded 1,536 gossip columns
(mean 160.817 ms, p99 279.650 ms) and 1,536 local columns (mean 41.327 ms,
p99 228 ms). The 23 nodes with local records comprise 22 nodes with EL
construction logs and one node with peer-reconstruction completion logs.

The actual nine-blob block is another workload difference: Spamoor targets a
transaction rate, not an exact number included in each block. Its approximately
256 KiB execution payload does not constrain the number or encoded size of blob
sidecars. The rerun's Gloas columns are 18,920 bytes before compression and
17,225–18,920 bytes after Snappy compression. Therefore this rerun is useful for
the logging diagnosis, but does not isolate differences in column propagation.

Saved [column analysis](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/last-slot/column-analysis.json)
and [rerun report](n100-v20000-ignored-aggregate-rerun.md).
