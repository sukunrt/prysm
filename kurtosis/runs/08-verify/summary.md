# Run 08 — the proof run: seat fraction 1.00, round counters alive

Enclave `rounds-08-verify`, 2026-08-21 22:14-22:42 IST, genesis at
2026-08-21 16:44:24 UTC. Images `prysm-*:orovsvnwtvvx`. EL
`ethpandaops/geth:glamsterdam-devnet-8`. Run 06's recipe exactly (6 supernodes
x 22 keys = 132 validators, 6s slots, 32-slot epochs, 8-slot rounds, no late
publishers, no blobs); `network_params.yaml` in this directory, raw scrapes,
node logs and the final metrics dump in `~/dev/prysm2-run-logs/08-verify/`.

Measurement window: slot 32 -> 163 (131 slots, 16 round boundaries, 780
per-node samples).

## The two run-06 anomalies, across the three runs

| | run 06 | run 07 | **run 08** |
|---|---|---|---|
| `goldfish_seat_fraction`, window mean | 0.976 | 0.979 | **0.9995** |
| window samples reading exactly 1.00 | 496/678 | 525/696 | **775/780** |
| `justified_round_advance_total`, window | 0 | 16 | **18** |
| `finalized_round_advance_total`, window | 0 | 15 | **17** |
| `p2p_pubsub_undeliverable_total`, vote topic | 369-514 | 326-541 | **never incremented** |
| `goldfish_late_vote_total` | 0 | 0 | 0 |
| finality latency slots, min/mean/max | 16/19.4/23 | 16/19.5/23 | 16/19.5/23 |

Both fixes hold and neither moved the consensus numbers: finality latency and
the finalized-round-versus-clock table are identical to run 06's, which is what
says the instruments were wrong and the chain was not.

The round counters advance once per round boundary on all six nodes (18 and 17
over the window's 16 boundaries; the window starts and ends mid-round). The
seat fraction reads exactly 1.00 on every node for every window slot except
five, and the undeliverable counter that accounted for the whole of run 06's
shortfall never moves at all.

## The five short samples left

| node | clock slot | seat fraction | seats missing | validators |
|---|---|---|---|---|
| cl-1 | 100 | 0.914 | 44 | 11 |
| cl-2 | 98 | 0.969 | 16 | 4 |
| cl-2 | 126 | 0.930 | 36 | 9 |
| cl-5 | 73 | 0.938 | 32 | 8 |
| cl-6 | 74 | 0.836 | 84 | 21 |

0.053% of the window's votes, against 2.4% in run 06 — a 45x reduction. What
they are not:

* not warm-up: they sit at slots 73-126, and the first 40 window slots are
  clean on every node;
* not the subscription buffer: `p2p_pubsub_undeliverable_total` on the vote
  topic was never incremented on any node;
* not lateness: `goldfish_late_vote_total` is 0, and the slowest block of the
  run reached its slowest node 365ms into a 6000ms slot (median 90ms), so the
  zero-late-tolerance acceptance rule never had a late vote to refuse;
* not a bad signature: `p2p_message_failed_validation_total` on the topic was
  never incremented either.

Every one is a single node in a single slot losing a whole batch of 4 to 21
votes at once, which is the shape of one shared lookup failing for a batch that
all names the same block. The remaining silent paths are the target root and
target state lookups in `validateAvailableAttWithBlock`, which returned
`ValidationIgnore` and swallowed their error. Run 09 instruments them
(`goldfish_vote_drop_total{reason}`, change `xusyoqln`) and names the residual.

## Chain health

131 of 131 window slots produced a block, head slot equals clock slot on all
780 window samples, 5 peers per node, `goldfish_gate_retreat`,
`gate_stop_total`, `round_proposal_conflict_total`, `equivocation_total` and
window reorgs all 0, first finalized round 2 at slot 32, end of run head 163 /
justified round 19 / finalized round 18 identically on all six nodes.
