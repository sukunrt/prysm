# Run 07 — the round-advance counters live; the seat fraction is a dropped vote

Enclave `rounds-07-verify`, 2026-08-21 21:53-22:14 IST, genesis at
2026-08-21 16:27:44 UTC. Images `prysm-*:lzqnzmlpumvy`. EL
`ethpandaops/geth:glamsterdam-devnet-8`. Run 06's recipe exactly (6 supernodes
x 22 keys = 132 validators, 6s slots, 32-slot epochs, 8-slot rounds, no late
publishers, no blobs); `network_params.yaml` in this directory, raw scrapes,
node logs and the final metrics dump in `~/dev/prysm2-run-logs/07-verify/`.

Measurement window: slot 32 -> 151 (119 slots, 15 round boundaries).

## Result: one run-06 anomaly is fixed, the other is diagnosed

| | run 06 | run 07 |
|---|---|---|
| `justified_round_advance_total` over the window | **0** | **16** |
| `finalized_round_advance_total` over the window | **0** | **15** |
| `goldfish_seat_fraction` mean over the window | 0.976 | 0.979 |
| finality latency slots, min/mean/max | 16 / 19.4 / 23 | 16 / 19.5 / 23 |
| head slot == clock slot on every sample | yes | yes |
| `goldfish_late_vote_total` | 0 | 0 |

The advance counters now move once per round on every node, because the count
moved to the point in the store where the checkpoint value is actually
replaced. Forkchoice realizes a round's advance on the **tick** path
(`NewSlot` -> `updateUnrealizedCheckpoints`), so counting it on the block path
never fired. Finality latency is unchanged, which is the point: the counters
were wrong, the checkpoints were not.

## The seat fraction did not move, and that ruled out the first hypothesis

Run 06's shortfall was **not** warm-up: over the window 182 of 678 samples read
below 1.0, spread evenly from slot 32 to slot 146, not clustered near genesis.
(Run 06's headline `min 0.14` *was* a warm-up artefact — the summarizer took
the seat fraction from every sample instead of from the window. Fixed in
`tszqrkto`.)

The first fix attempt closed a genuine race in the pending available-attestation
queue: `processPendingAttsForBlock` deleted the queue key only *after*
processing the batch, so votes queued while it worked were dropped unprocessed
(`lzqnzmlp`). Run 07 shows that race was not where the votes were going — the
mean seat fraction moved from 0.976 to 0.979, inside run-to-run noise.

## Where the votes actually go: gossipsub drops them before validation

`p2p_pubsub_undeliverable_total` on the head-vote topic, per node, against the
votes the seat fraction says never arrived (missing seats divided by the 3.879
seats an average validator holds):

| | cl-1 | cl-2 | cl-3 | cl-4 | cl-5 | cl-6 |
|---|---|---|---|---|---|---|
| run 06 undeliverable | 369 | 358 | 392 | 514 | 437 | 387 |
| run 06 votes missing from the seat count | 367 | 349 | 394 | 528 | 410 | 393 |
| run 07 undeliverable | 451 | 477 | 405 | 541 | 328 | 326 |
| run 07 votes missing from the seat count | 418 | 518 | 378 | 535 | 310 | 286 |

`notifySubs` hands a message to a subscription with a non-blocking send and
drops it when the buffer is full, reporting nothing but the RawTracer event
behind that counter. The buffer defaults to 32 messages. Every validator
publishes its head vote as soon as the block reaches its own node, so the whole
network's votes for a slot land in one burst of ~132 messages a few
milliseconds wide — four times the buffer.

That is why nothing else could see it. The votes never reach validation, so no
ignore is counted; they never reach forkchoice, so `goldfish_late_vote_total`
stayed at **0** for both runs. The zero-late-tolerance acceptance rule is not
involved: on this devnet no vote is ever late, some simply never arrive.

Fixed in `orovsvnw` by sizing the head-vote subscription for the burst, and
`vtqxrwzv` puts `p2p_pubsub_undeliverable_total` in the scrape and the gossip
table so a future run cannot lose votes silently. Run 08 is the proof.

## Chain health

Identical to run 06 on every axis: every window slot produced a block, head
slot equals clock slot on all 696 window samples, `goldfish_gate_retreat`,
`gate_stop_total`, `late_vote_total`, `equivocation_total` and window reorgs
all 0, 5 peers per node, first finalized round 2 at slot 32, end of run head
151 / justified round 17 / finalized round 16 on all six nodes.
