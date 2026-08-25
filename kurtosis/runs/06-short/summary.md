# Run 06 — per-round finality, short: finality lags the clock by 16-23 slots

Enclave `rounds-06-short`, 2026-08-21 20:19-20:37 IST, genesis at unix
1787323945. Images `prysm-*:xqqptyrvlztz`, built from the tip carrying
`yuoozyyo` "precompute: put the FFG genesis stub guard on the round clock";
EL `ethpandaops/geth:glamsterdam-devnet-8`, the same pin runs 02 and 05 used.
`network_params.yaml` in this directory; raw scrapes, node logs and the final
metrics dump in `~/dev/prysm2-run-logs/06-short/`.

A deliberately cheap run: **6 beacon nodes (all supernodes) x 22 keys = 132
validators**, `SECONDS_PER_SLOT: 6`, `SLOTS_PER_EPOCH: 32`,
`SLOTS_PER_ROUND: 8`, `AVAILABLE_ATTESTATION_DUE_BPS_HEZE: 2500`, **no late
publishers and no blob traffic** — a finality measurement wants the clean
case. The validator total does not shrink with the node count: prysmctl's
genesis builder never returns below ~128 validators, so the keys per node go
up instead, and 132 / 8 = 16.5 attesters per slot keeps run 05's per-slot
shape.

Measurement window: slot 32 -> 146 (114 slots, 684s of chain time, 14 round
boundaries).

## The headline

| | run 06 (rounds) | run 02 / run 05 (epochs) |
|---|---|---|
| **finality latency, slots** (min/mean/max) | **16 / 19.4 / 23** | 64 / 78.3 / 95 |
| **first finalized checkpoint** | round 2, at **slot 32** | epoch 2, at **slot 128** |
| **first justified checkpoint** | round 2, at **slot 24** | epoch 2, at slot 96 |
| per-round (per-epoch) advance rate | **1.00** | 1.00 |

Latency is `finality_latency_slots`: the clock slot minus the first slot of the
last finalized round, sampled once per slot on every node — 678 window samples
across the six nodes, none outside 16..23. The run-02 and run-05 columns are
the same quantity recomputed from their raw scrapes as
`clock_slot - 32 * beacon_finalized_epoch`, so the two are measured the same
way and not merely quoted.

**The improvement is 4.0x, and it is exactly `SLOTS_PER_EPOCH /
SLOTS_PER_ROUND`.** Finality still costs two boundaries; the boundaries are
just four times as close together. 78.3 / 19.4 = 4.04, and both distributions
span exactly one boundary interval (64..95 is 32 wide, 16..23 is 8 wide).

## Every round justified and finalized, none skipped

`beacon_finalized_round` against the wall clock, one column per round, all six
nodes:

| round | r0 | r1 | r2 | r3 | r4 | r5 | ... | r16 | r17 | r18 |
|---|---|---|---|---|---|---|---|---|---|---|
| finalized round | 0 | 0 | 0 | 0 | 2 | 3 | ... | 14 | 15 | 16 |

From round 4 on, `finalized = clock_round - 2` at every single round boundary,
on every node, with no stall and no double step. Over the window the justified
round went 3 -> 17 and the finalized round 1 -> 15: **14 advances over 14 round
boundaries, every one of them +1** — a per-round rate of 1.00 for both
checkpoints (0.98 if you divide by the window's 14.25 rounds).

The genesis guard behaves exactly as `yuoozyyo` specifies: rounds 0 and 1 are
skipped, round 2 justifies at the 2->3 boundary (**slot 24**) and finalizes at
the 3->4 boundary (**slot 32**). A two-round warm-up where the epoch build had
a two-epoch one.

End of run, identical on all six nodes: head 146 = clock 146, justified round
17, finalized round 16, latency 18.

## Chain health

| evidence | value |
|---|---|
| samples where head slot != clock slot, per node | **0 of 112** |
| blocks over the window | 114 of 114 slots, no misses |
| `goldfish_gate_retreat`, `goldfish_gate_stop_total` | 0 (late publishers off) |
| `beacon_reorgs_total` in the window | 0 |
| `goldfish_late_vote_total`, `_conflict_total`, `_equivocation_total` | 0 |
| `goldfish_seat_fraction`, mean over 858 samples | 0.972 |
| available attestations handed to the app | 140.9 /slot/node against 132 validators |
| cpu per beacon node | 0.19-0.24 cores |
| ERROR lines outside the two documented-benign classes | **0** |

The 8.9 available attestations per slot above the validator count are the
queued votes being replayed, the same ~7-12% run 05 saw. Run 05's one-time
INVALID payload at slot 1 **did not reproduce**: no `INVALID payload`, no
`common ancestor`, no `Could not build block` on any node.

The lifetime `beacon_reorgs_total` of 1 is a slot-0 artifact — the log line
has `depth=0 distance=0` and `oldRoot == newRoot == commonAncestorRoot` — and
it lands before the window.

## Anomaly: both round-advance counters are dead

`justified_round_advance_total` and `finalized_round_advance_total` read
**0 on every node for the whole run**, while the rounds they are supposed to
count advanced 14 times each. The per-round rate above had to be derived from
the gauges instead.

The counters are incremented in
`beacon-chain/blockchain/receive_block.go:604,623`, which compare forkchoice's
checkpoint before and after processing a block:

```go
if justified.Epoch > preJustifiedRound {
    justifiedRoundAdvanceTotal.Inc()
}
```

But in this build forkchoice's justified and finalized checkpoints move on the
**tick** path, not the block path: `on_tick.go:50` calls
`updateUnrealizedCheckpoints`, which realizes the unrealized checkpoints at the
boundary (`unrealized_justification.go:43-58`). By the time the next block is
processed, `preJustifiedRound` — read from forkchoice at the top of `onBlock`
— already holds the new round, so the comparison is never true and neither
counter ever fires.

Reported, not fixed: this is a metrics bug, not a consensus one. The
checkpoints themselves are correct, and `beacon_finalized_round`,
`beacon_current_justified_round` and `finality_latency_slots` — all read from
the post-state — are right.

## Scope

Six nodes, no late publishers, no blobs, four epochs. This run says what
finality latency and the round cadence are; it says nothing about column
traffic, gate retreats under lateness, or behaviour past epoch 4. Run 05
remains the reference for those.
