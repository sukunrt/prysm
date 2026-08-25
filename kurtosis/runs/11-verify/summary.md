# Run 11 — the ledger names the last shortfall: votes queued and never woken

Enclave `rounds-11-verify`, 2026-08-21 23:06-23:20 IST. Images
`prysm-*:qztnrwwkvqln`, run with `--goldfish-vote-ledger`. EL
`ethpandaops/geth:glamsterdam-devnet-8`. Run 06's recipe (6 supernodes x 22
keys = 132 validators, 6s slots, 32-slot epochs, 8-slot rounds, no late
publishers, no blobs); raw scrapes, node logs, the vote ledger and the final
metrics dump in `~/dev/prysm2-run-logs/11-verify/`.

**Cut short at slot ~90: the host ran out of disk.** Run 10 is the same config
and died before genesis for the same reason. What the run did deliver is the
diagnosis the counters could not give, so it is recorded rather than discarded.

Window: slot 32 -> 87, 325 samples, seat fraction mean 0.995, 8 samples below
1.00, min 0.37.

## What the ledger says

The per-vote ledger accounts for every seat. Over the window, per outcome
(seats, all six nodes):

| accepted off gossip | node's own | queued | replayed out of the queue |
|---|---|---|---|
| 130,125 | 28,672 | 13,235 | 12,442 |

A vote appears twice when it is queued and later replayed, so the seats that
never reached forkchoice are `13,235 - 12,442 = 793` of 172,032, **0.46%** -
which is exactly the shortfall the seat fraction reports (mean 0.995).

Every one of them is `queued`, and there is not one `dropped` line in the
window on any node. Nothing was late (`goldfish_late_vote_total` 0), nothing
was refused, nothing was undeliverable, and nothing failed to arrive: the votes
reached the node, were parked waiting for their block, and were never woken.

The worst sample makes it plain. cl-6, slot 75, seat fraction 0.365:

| outcome | votes | seats |
|---|---|---|
| accepted | 26 | 99 |
| local | 22 | 88 |
| queued, never replayed | 84 | 325 |
| **total** | **132** | **512** |

99 + 88 = 187 seats = 0.365 x 512. All 132 votes arrived; 84 of them were
parked and forgotten.

## Why

A vote whose block has not arrived is queued against the block root and
replayed when the block is imported. The queue was gated on
`hasBlockAndState`: the block in the database *with its state summary*. The
replay is triggered by block import, which puts the block in forkchoice first
and can leave the state summary write outstanding. A vote that checked the gate
inside that window was queued *after* its wake-up had already run - and nothing
wakes a queue twice.

Fixed in `snmvztrt` by gating on forkchoice membership on both sides, the
vote's own check and `processPendingAttsForBlock`'s guard, which refused to
drain for the same reason. Import sets forkchoice membership before it drains,
so a vote is either queued in time to be drained or validated inline.

## Tooling note

Run 11's build logs the local votes without a validator index, so
`kurtosis/votetally.py` cannot attribute them and reports them as
`never_arrived` (88 seats per slot per node, exactly the 22 local validators x
4 seats). Fixed in `rqpwlqyv`; the totals above are taken from the ledger
directly and are not affected.
