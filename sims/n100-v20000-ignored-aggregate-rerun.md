# 100-node rerun with ignored FFG aggregate logging

2026-10-06. Prysm completed on ethp2p using newly built and synced binaries,
including unconditional logging when an FFG aggregate is ignored as already
covered. The completed eth-slot-sim run is reused. Only **Prysm slot 8 /
eth-slot-sim slot 7** is analyzed below.

Configuration remains 100 nodes, 20,000 validators, 80 home / 20 super nodes,
one FFG subnet and one 2,500-voter committee per slot, ePBS, eight live slots,
individual FFG votes at slot start with no configured delay, FFG aggregates at
9s, and PTC deadline at 6s. Runtime identity audits confirmed all 99 participant
peers per node in both simulators, including Prysm's CL and EL networks.

## Final-slot timing

Mean and continuous p99 are **milliseconds from slot start**. Publisher receipts
are excluded. These are observed receipt statistics; the measurement differences
and aggregate coverage below are material to interpretation.

| Object | Prysm avg | Prysm p99 | eth-slot-sim avg | eth-slot-sim p99 |
|---|---:|---:|---:|---:|
| Consensus block | 98 | 223 | 127 | 228 |
| Execution payload | 1,709 | 8,849 | 1,957 | 3,212 |
| Data column records | 101 | 261 | 1,118 | 1,277 |
| Availability vote | 756 | 3,583 | 403 | 929 |
| FFG vote | 633 | 5,562 | 358 | 1,929 |
| FFG aggregate | 10,200 | 12,204 | 9,145 | 9,289 |
| PTC vote | 1,161 | 6,329 | 6,175 | 6,431 |

Payload latency uses the first receipt per node and block. Nodes 2 and 15 each
logged the same payload twice; the repeats remain in the raw ledger and in
`payload_repeat_receipts`, but are excluded from the latency sample. There are
99 unique remote payload receipts. The last first receipt was at 9,891 ms.

## Aggregate receipt diagnosis

- Prysm published 19 aggregates at 9,000–9,001 ms, each covering 2,500 seats.
- **1,156 remote receipts were ignored as `already_known`**, spanning
  9,001–12,421 ms. Their mean was 10,202.097 ms and p99 12,204.150 ms.
- **Two remote receipts were accepted**, both at node 15 at 9,014 ms. Their
  aggregator indices were 2,114 and 12,447.
- The table combines those 1,158 receipts. No receiver/aggregator/data-root
  duplicate records were found.
- Each published aggregate was observed at 51–83 of its 99 remote peers:
  1,158 observed publisher/receiver pairs out of 1,881 possible. eth-slot-sim
  recorded all 1,980 pairs for its 20 publications.

`HasAggregatedAttestation` can find an aggregate already covered by the local
pool. Prysm returns `ValidationIgnore`, which also stops GossipSub forwarding
from that receiver. Complete peer connectivity does not imply broadcast to all
99 peers: the gossip mesh still has D=8. The new records establish that many
arrivals previously absent from the accepted ledger were received and ignored.
They do not establish that every aggregate reached every node. Prysm observation
ended approximately 13s into the final slot, about four seconds after aggregate
publication; eth-slot-sim had a longer drain. Unobserved pairs are not classified
as permanent network loss.

All **247,500 / 247,500** expected individual FFG receipts were present, without
duplicates. **236 arrived at or after 9s**, all at node 15, between 9,011 and
9,179 ms. This is also the node that accepted the two aggregates. That timing
is consistent with its local pool still lacking some votes when the aggregates
arrived; the receipt evidence alone does not identify the underlying delay.
All 2,500 local individual votes were published at 0–5 ms. eth-slot-sim had no
late or missing individual FFG receipts.

## Payload and column comparability

| Final-slot measurement | Prysm | eth-slot-sim |
|---|---:|---:|
| Execution payload bytes | 267,021 | 262,144 |
| Blobs | 9 | 6 |
| Column records, excluding proposer | 3,072 | 3,072 |
| Proposer column publication timing, ms | Approximately 15 | 966.014 |
| Mean encoded column bytes | 18,085.578 | 13,220 |

Prysm's payload is 1.86% above the 256 KiB target. Its final block contains
19 transactions, one FFG attestation and one payload attestation, with gas limit
200,000,000 and gas used 17,158,216. All 100 nodes imported the same block and
payload and agreed on the final head. All eight captured block gas limits were
200,000,000 in the payload envelope and EL block, with matching hashes.

The Spamoor rate target does not guarantee exactly two three-blob transactions
in each block. This final block included nine blobs, while eth-slot-sim models
six. It is therefore not a controlled column-propagation comparison. Matching
execution payload bytes does not match blob sidecar sizes.

Half of Prysm's final-slot column records were local construction, whereas
eth-slot-sim has only the modeled gossip path. Prysm gossip timestamps record
validation entry; eth-slot-sim records delivery after modeled validation. The
approximately 951 ms publication offset explains most of the slot-relative
column gap. After its recorded publication, eth-slot-sim column mean/p99 were
151.542 / 310.579 ms. Moving publication earlier can also change contention,
so subtraction is not a prediction of a new run.

PTC behavior was unchanged: Prysm may publish upon availability, while
eth-slot-sim publishes at the 6s deadline. Prysm's 462 observed PTC signers
each reached all 99 remote nodes (45,738 receipts), all reporting payload and
blobs present. Repeated validator indices can occupy the 512 committee seats;
eth-slot-sim uses 512 distinct signers. Local PTC publication records are absent,
so the ledger establishes coverage for observed signers only.

## Verification and evidence

Shadow exited successfully. No final-slot error logs or parser failures were
found. The two failed comparison checks were individual FFG delivery before 9s
and matching blob count. Other end-to-end and connectivity checks passed.

Focused aggregate-validation and ledger tests passed with Go 1.26.5:

```sh
GOTOOLCHAIN=go1.26.5 go test -mod=readonly -count=1 ./beacon-chain/sync -run 'TestValidateAggregateAndProof_|TestLogFFG'
```

The default Go 1.27.1 test run was blocked by dependency build constraints in
`cockroachdb/swiss`; the production binaries built successfully using Go 1.27.1,
matching the previous simulation toolchain. Parser smoke checks verified that
ignored records carry reasons/decision times and accepted records stay separate.

- [Results and validation checks](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/last-slot/results.json)
- [Avg/p99 CSV](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/last-slot/avg-p99.csv)
- [Column analysis](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/last-slot/column-analysis.json)
- [Peer identity audit](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/peer-audit.json)
- [Receipt audit](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/last-slot/receipt-audit.json)
- [Manifest](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/manifest.json)
- [Binary and source provenance](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/current-binary-provenance.json)
- [Reproducible analysis](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/analyze_last_slot.py)
- [Original configuration and baseline results](n100-v20000-prysm-eth-slot-sim-256k.md)
- [Why column timings differ](n100-v20000-column-timing-analysis.md)

Remote rerun root:
`ethp2p:/home/sukun/dev/slot-compare-20261006-n100-v20000-agg9s-ptc6s-payload256k-gas200m-agglog`.

## Individual FFG delay and evolving gossip meshes

The subsequent [comparison across all eight slots](n100-v20000-ffg-across-slots.md)
finds that every post-warmup slot's slowest receiver had recently accepted a blob
transaction through its execution RPC. This is a more specific lead for the
home-node tail than the scoring hypothesis alone.

The final-slot slowdown is concentrated at home receivers. Supers are faster
in Prysm in this run. All times below are milliseconds from slot start.

| Receiver class | Prysm avg | Prysm p99 | eth-slot-sim avg | eth-slot-sim p99 |
|---|---:|---:|---:|---:|
| Home | 746 | 5,769 | 375 | 1,976 |
| Super | 155 | 579 | 287 | 795 |

Every Prysm super received all its remote individual FFG votes by 769 ms.
All 3,402 Prysm receipts at or after 5s occurred at five homes: nodes 2, 13,
14, 15 and 23. Removing those five receivers as a diagnostic reduces Prysm's
average/p99 from 633/5,562 ms to 476/2,265 ms; the official table still includes
them. In eth-slot-sim only 39 receipts were at or after 3s, all at one home,
and none were at or after 5s.

The slow homes also had delayed payload receipts: nodes 15, 14, 2 and 13 first
received the payload at 9,891, 8,828, 6,072 and 3,979 ms respectively. Late FFG
votes came from many publishers, not just one slow sender. Both simulators
published the individual FFG burst within the first few milliseconds.

Prysm records the arrival at entry to `validateCommitteeIndexBeaconAttestation`
and writes the ledger after successful validation. Across all 247,500 final-slot
receipts, the log timestamp minus recorded arrival ranged from -6 to +8 ms.
The text log timestamp is truncated to 10 ms, so negative differences are a
precision artifact; allowing for truncation puts the validation-to-log interval
below approximately 20 ms. The multi-second tail therefore precedes entry to
this validator. This does not distinguish network/transport delay from queueing
before the validator, and does not measure the receiver's later subscriber work.

Full connectivity fixes the available peers, not the per-topic delivery mesh.
These implementations have different rules for how that mesh evolves:

| Setting | Prysm | eth-slot-sim |
|---|---|---|
| Target / low / high mesh degree | 8 / 6 / 12 | 8 / 6 / 12 |
| Heartbeat | 700 ms | 700 ms |
| Peer scoring | Enabled, including first-delivery rewards | Not enabled |
| Minimum outbound mesh peers, `Dout` | 2 (library default) | 1 (explicit override) |
| Score-aware pruning | Keeps high-scoring peers when pruning an oversized mesh | No score differentiation |
| Opportunistic grafting | Checks every 60 heartbeats (~42s); seeks peers above mesh median when median score < 5 | No score-driven improvement |
| pubsub dependency | v0.17.0 | v0.16.1-0.20260515125344-5ac7695ba01b |

Prysm's first-delivery rewards can favor faster routes as messages accumulate;
this is a plausible explanation for different super/home paths over time. Its
mesh-delivery-deficit penalty is disabled (`meshDeliveryIsScored=false`), so this
is not evidence that homes were penalized for failing an expected delivery rate.
The saved peer audits contain connections and directions, not FFG mesh membership,
GRAFT/PRUNE history or scores. The existing run cannot establish that scoring
caused the measured delay split, or which intermediate peer held a late vote.

The offered traffic also differs despite equal unique FFG counts: Prysm includes
Geth transaction/blob gossip on the same home host bandwidth, and its early
column/payload/PTC traffic overlaps the FFG burst. eth-slot-sim delays the payload
and columns until 966 ms and PTC votes until 6s. Transport differs too: observed
Prysm connections use TCP; eth-slot-sim uses QUIC. These are competing explanations
for queueing and delivery differences, not isolated causes proven by this run.

Processing models differ as well. eth-slot-sim places FFG votes in their own
modeled verifier queue (10 ms batch window, 10 ms base service, 0.01 ms per item,
300-item cap). Prysm uses its actual validation path and a shared signature
verifier with a 5 ms timer. Its arrival metric excludes the receiver's validation;
eth-slot-sim's metric includes modeled validation. This can contribute to the
super-node comparison, but does not explain Prysm's multi-second pre-validation tail.

A causal follow-up should record FFG mesh members/classes, GRAFT/PRUNE events,
scores and message send/receive stages, plus per-host bytes and queueing. A
controlled run can then align scoring, `Dout`, transport and competing traffic,
changing one condition at a time. No peering or simulation behavior was changed
during this investigation.

Evidence: [per-node delay analysis](../shadow/runs/compare-n100-v20000-s1-agg9s-ptc6s-payload256k-gas200m-agglog-20261006/artifacts/last-slot/ffg-delay-analysis.json),
[Prysm pubsub options](../beacon-chain/p2p/pubsub.go),
[Prysm scoring](../beacon-chain/p2p/gossip_scoring_params.go),
[FFG validation timestamp](../beacon-chain/sync/validate_beacon_attestation.go),
[eth-slot-sim pubsub and verifier setup](../../eth-slot-sim/node/node.go),
[eth-slot-sim verification classes](../../eth-slot-sim/node/registry.go).
