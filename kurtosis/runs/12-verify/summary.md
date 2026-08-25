# Run 12 — acceptance: seat fraction 1.00 everywhere, every seat accounted for

Enclave `rounds-12-verify`, 2026-08-22 00:41-01:07 IST, genesis at
2026-08-21 19:13:11 UTC. Images `prysm-*:pwynozmwkvkq`, run with
`--goldfish-vote-ledger`. EL `ethpandaops/geth:glamsterdam-devnet-8`. Run 06's
recipe exactly (6 supernodes x 22 keys = 132 validators, 6s slots, 32-slot
epochs, 8-slot rounds, no late publishers, no blobs); `network_params.yaml` in
this directory, raw scrapes, node logs, the vote ledger and the final metrics
dump in `~/dev/prysm2-run-logs/12-verify/`.

Measurement window: slot 32 -> 167 (135 slots, 17 round boundaries, 804
per-node samples).

## Both instruments now read what this environment has to produce

| | run 06 | **run 12** |
|---|---|---|
| `goldfish_seat_fraction` min / mean / max | 0.63 / 0.976 / 1.00 | **1.00 / 1.000 / 1.00** |
| window samples below 1.00 | 182 of 678 | **0 of 804** |
| `justified_round_advance_total` over the window | 0 | **18** |
| `finalized_round_advance_total` over the window | 0 | **17** |
| `p2p_pubsub_undeliverable_total`, vote topic | 369-514 per node | **never incremented** |
| `goldfish_vote_drop_total`, window, any reason | (did not exist) | **0** |
| `goldfish_late_vote_total` | 0 | 0 |
| finality latency slots, min/mean/max | 16 / 19.4 / 23 | 16 / 19.5 / 23 |

Finality latency and the finalized-round-versus-clock table are the same as run
06's, which is the point: the instruments were wrong, the chain was not.

## The ledger reconciliation

Every node ran with the per-vote ledger, so the seat fraction is not taken on
trust. `kurtosis/votetally.py` takes the seats the mock committee schedule
expects for each slot - from the schedule arithmetic, not from the logs - and
asks the node's ledger what became of each one:

| node | slots | expected seats | accepted | dropped | never arrived | full slots |
|---|---|---|---|---|---|---|
| cl-1-prysm-geth | 139 | 71,168 | 71,168 | 0 | 0 | 139/139 |
| cl-2-prysm-geth | 139 | 71,168 | 71,168 | 0 | 0 | 139/139 |
| cl-3-prysm-geth | 139 | 71,168 | 71,168 | 0 | 0 | 139/139 |
| cl-4-prysm-geth | 139 | 71,168 | 71,168 | 0 | 0 | 139/139 |
| cl-5-prysm-geth | 139 | 71,168 | 71,168 | 0 | 0 | 139/139 |
| cl-6-prysm-geth | 139 | 71,168 | 71,168 | 0 | 0 | 139/139 |

**427,008 seats expected, 427,008 accepted, none dropped, none unaccounted.**

By outcome, the same ledger in seats: 355,761 accepted off gossip, 71,168 the
node's own, and 79 queued waiting for their block - every one of which was
replayed inside its own slot. The queue is still used; it no longer loses
anything.

The gossip table agrees independently: **132.0 available attestations per slot
per node**, against 132 validators, on every node. Runs 06 to 08 read 139-142
because replayed votes were re-broadcast; nothing is arriving late enough to
need that now.

## What the four fixes were

**1, `orovsvnw`, 2.4%.** Gossipsub dropped votes as undeliverable: the head
vote topic's subscription buffer is libp2p's default of 32 messages and a
slot's votes land in one burst of 132. Sized the subscription for the burst.

**2, `lzqnzmlp`.** The pending queue deleted its key only after processing a
batch, discarding whatever arrived meanwhile. Take the batch and drop the key
in one critical section, then loop.

**3, `snmvztrt`, 0.5%.** A queued vote was gated on the block's state summary
while its wake-up is block import, which sets forkchoice membership first.
Gate the queue on forkchoice membership.

**4, `pwynozmw`.** A proposer stranded the whole peer committee on its own
slots: its block is imported through the RPC and never reaches the gossip
subscriber that drains the queue. Drain on block processing instead, so every
import path wakes it.

None of them touched the zero-late-tolerance acceptance rule. It never fired:
`goldfish_late_vote_total` is 0 across all four runs, and the slowest block of
this run reached its slowest node 365ms into a 6000ms slot.

## Chain health

135 of 135 window slots produced a block, head slot equals clock slot on all
804 window samples, 5 peers per node, `goldfish_gate_retreat`,
`gate_stop_total`, `round_proposal_conflict_total`, `equivocation_total` and
window reorgs all 0, 32 round proposals per node, first finalized round 2 at
slot 32, end of run head 167 / justified round 19 / finalized round 18
identically on all six nodes.
