# Run 05 — run 02's shape on the fixed tip, with real blob traffic

Enclave `w4k-05-main`, still up. 2026-08-21 06:24-06:47 IST, genesis at unix
1787273678. Images `prysm-*:nxtknlrltzsv`, EL
`ethpandaops/geth:glamsterdam-devnet-8`. `network_params.yaml` in this
directory; raw scrapes, node logs, the EL block table and the per-node column
listings in `~/dev/prysm2-run-logs/05-main/`.

Same shape as run 02: 10 beacon nodes (all supernodes) x 13 keys = **130
validators**, `SECONDS_PER_SLOT: 6`, `SLOTS_PER_EPOCH: 32`,
`SLOTS_PER_ROUND: 8`, `AVAILABLE_ATTESTATION_DUE_BPS_HEZE: 2500`, late
publishers at `--decoupled-late-block-publish-bps=3500
--decoupled-late-block-publish-every-nth=4`. What is new is `kurtosis/blobsend`
driving el-01 with one blob transaction and one transfer per slot, so the
payloads are not empty.

Measurement window: slot 32 -> 215 (183 slots, 1098s of chain time).

## What the images contain

Rebuilt from this tree, not assumed:
`beacon-chain` carries `savePendingAvailableAtt` and
`validateAvailableAttWithBlock`, neither of which exists in the run-02 image
`prysm-beacon-chain:zqtmtlrvvnqs`; the validator's `promoteDuties` calls only
`snapshot` and `cloneValidatorDuty` where the old image also called
`fetchNextEpochDuties`. Symbol listings in
`~/dev/prysm2-run-logs/05-main/image-verification.txt`.

## Data columns exist and flow

Run 02's blocking anomaly is gone. Its transfers were included, charged gas
and moved nothing; they were dying out-of-gas, because Amsterdam prices a
first-touch transfer at ~207k and the tooling asked for 21000. With a 500k
limit the same transfer costs 204,600 gas on first touch and 21,000 after,
status 1, and the recipient's balance is credited: 0.219 ETH at
`0x…c0ffee`, identical on el-01 and el-05.

| evidence | value |
|---|---|
| blob transactions with a receipt, status 1, 262,144 blob gas | 129 |
| blobs on the execution chain (162 blocks scanned) | 264 in 80 blocks (49%) |
| data column sidecars written, per node (identical on all 10) | 13,184 in the window; 14,080 by the end |
| column gossip delivered to the app | 70.3 sidecars/slot/node |
| column subnet traffic received / sent | 584 KB / 611 KB per slot per node |
| column subnets subscribed | 128 of 128, every node |
| sidecars built for own proposals | 74-81 per node |
| sidecars gossip-verified | 1,821-2,334 per node (cl-01: 194, see below) |
| columns rebuilt from the execution layer | 51-62 events per node |
| sampling or data-column errors in ten ~23-minute logs | **0** |

**Served, not just held.** `/eth/v1/debug/beacon/data_column_sidecars/<slot>`
on all ten nodes, at four blob-carrying slots (95, 100, 150, 155): every node
returns 128 sidecars with indices 0..127 complete, two cells each — 40 of 40
queries. A slot whose block was orphaned by a gate retreat answers 404 on
every node, and a slot whose payload carried no blob answers with an empty
list, which is the same fork choice showing through a second API.

`rpc_data_columns_by_range` served nothing: no node ever fell behind, so the
by-range request path was never exercised. Gossip and the local execution
layer covered every column.

**Why cl-01 verifies fewer.** cl-01 is the node whose execution client
receives all the traffic, so it rebuilds columns from its own EL more often
(83 events by the end against 51-62 elsewhere) and has less left to verify off
gossip (194 successes against ~2,000). It still receives the column gossip —
289 duplicates per slot against the other nodes' 325-361 — it just already has
the columns, so almost nothing reaches the verification path.

## Goldfish: seats, gate and reorgs

Window deltas, per node:

| metric | run 05 | run 02 |
|---|---|---|
| `goldfish_gate_retreat` | **52** (all 10 nodes) | 43 |
| `beacon_reorgs_total` | **52** (all 10 nodes) | 43 |
| late-published blocks (VC logs) | **52** | 43 |
| `goldfish_gate_stop_total` | 54-60 (median 57) | 44-51 |
| `goldfish_late_vote_total` | 0 | 0 |
| `goldfish_round_proposal_total` | 44 | 45 |
| `goldfish_round_proposal_conflict_total` | 0 | 0 |
| `goldfish_equivocation_total` | 0 | 0 |
| `goldfish_seat_fraction`, mean over all samples | **0.969** | 0.920 |

**The 43=43=43 pattern persists as 52=52=52.** The count differs only because
which proposer indices land on which slots is a fresh shuffle; the equality is
what carries over. Matching the two lists with a one-slot tolerance:

```
late blocks in window 32..215:                    52
gate-retreat slots (per node):                    52
retreats with a late block at slot s or s-1:      52
late blocks with no retreat at s or s+1:           0
```

The vote-queue fix did not disturb this, which was the thing to verify: a vote
that arrives before its block is now replayed rather than dropped, and a block
that misses the deadline still fails the majority gate.

**Seat fraction improved.** 0.920 -> 0.969 on the same comparison
(all samples, warm-up included). Restricted to the measurement window the mean
is 0.974 over 1,790 samples, per node 0.967-0.984, and the distribution is

| seat_fraction | share of samples |
|---|---|
| [0.95, 1.00] | 83.6% |
| [0.80, 0.95) | 12.6% |
| [0.60, 0.80) | 3.4% |
| [0.40, 0.60) | 0.4% |
| below 0.40 | 0% |

The old 0.09-0.10 floor is gone from the window entirely; the lowest window
sample is 0.48. The dips that remain sit at the late slots, where the block
genuinely is not there yet.

**`goldfish_late_vote_total` is still 0, and that is the good outcome.** The
counter means "a replayed vote whose slot had already ended". Replays here
happen inside the vote's own slot — the late block lands 2.1s in, the queued
votes are revalidated and handed to forkchoice immediately — so nothing is
ever late by that definition. The queue is demonstrably working from a
different number: the beacon nodes handed **145.2 available attestations per
slot per node** to the application against 130 validators, where run 02 handed
exactly 130.0. The extra 15.2 per slot, about 12%, are the queued votes being
replayed and re-broadcast, which is the same order as the ~8% that used to be
dropped.

## Finalization

All ten nodes in lockstep, head slot 215 = clock slot at every sample: **no
missed slots**, 184 blocks over 184 slots in the window.

| | e0 | e1 | e2 | e3 | e4 | e5 | e6 |
|---|---|---|---|---|---|---|---|
| finalized epoch (identical on all 10 nodes) | 0 | 0 | 0 | 0 | 2 | 3 | 4 |

End of window on every node: head 215, justified 5, finalized 4 — epoch for
epoch the same cadence as run 02 and as the ethshadow baseline, now while
carrying blobs. The chain kept going after the window: at 06:47 every node
reported head 248, justified 6, finalized 5.

## PTC

| evidence | value |
|---|---|
| `forkchoice_ptc_vote_count` at the end | 126,464, **identical on all 10 nodes** |
| `Submitted payload attestations` slots, per VC | 168-199 (1,835 submissions) |
| of those, `blobDataAvailable=true` | **1,835 of 1,835** |
| of those, `payloadPresent=true` | 1,825; false on 10 |
| `no canonical shuffling block` in any CL or VC log | **0** |

The PTC members are the second independent witness that the columns were
there: `blobDataAvailable=true` on every single payload attestation in the
run. The ten `payloadPresent=false` votes are honest votes about slots whose
payload did not show.

## Traffic, per slot per node (window means)

| topic family | app msgs/slot | recv B/slot | sent B/slot | msg size |
|---|---|---|---|---|
| available_attestation | 145.2 | 174,890 | 178,265 | 214 B |
| data_column_sidecar_* | 70.3 | 583,768 | 611,199 | 1,693 B |
| sync_committee_* | 386.1 | 492,331 | 499,325 | 210 B |
| beacon_attestation_* | 14.6 | 22,018 | 22,262 | 260 B |
| beacon_aggregate_and_proof | 8.4 | 28,927 | 28,927 | 486 B |
| beacon_block | 1.0 | 4,182 | 4,184 | 2,169 B |

The attestation families are within a few percent of run 02 and of the
plan-5.4 ethshadow baseline. The column family is new — 584 KB/slot/node is
what full custody of 128 columns costs at ~2 blobs a block, and it is the
single heaviest received family in the run.

16.25 unaggregated committee attesters per slot, flat over all eight round
offsets (16.00 on offsets 0,1,2,4,5,6 and 17.00 on 3,7); 8.38 aggregators per
slot. Identical to run 02.

## Cost

0.16-0.19 cores per beacon node, up from run 02's 0.12-0.14: the columns are
not free, but the head never fell behind the clock and the 32-container
enclave stayed well inside this 16-core box.

## Anomalies

- **The blob pool backs up late in the run.** From about slot 160 blobsend
  starts getting `account limit exceeded: pooled 16 txs` — geth caps pending
  blob transactions per account at 16, and one blob transaction per 6s slot
  outruns what the proposers include. 19 sends never got a receipt. The blobs
  are therefore front-loaded: nearly every block up to slot ~155 carries two,
  and later blocks mostly carry none. Send every other slot, or one blob per
  transaction, if a run needs blobs spread evenly to the end.
- **A one-time INVALID payload at slot 1, on all ten nodes.** `Could not update
  forkchoice with engine error=received an INVALID payload`, then `Could not
  get local payload, falling back to P2P bid` and `Could not build block` for
  slot 1, plus `Could not find common ancestor root` a few seconds later.
  Every node recovered by slot 2 and the window has no missed slots, but the
  genesis-edge handoff to the execution layer is worth a look.
- Otherwise only the two documented benign classes: the unknown config fields
  at startup and the deposit poller's follow-distance complaint.
