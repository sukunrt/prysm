# Round 2 block publication timing

Genesis was `2026-09-05T01:30:00Z` and the configured slot duration was 12 seconds. A slot's start is therefore `genesis + slot*12s`.

## Result

The first successful blocks were not first published after their slot deadlines. Slot 15 finished building at `+1.482s`, was imported by its own beacon node at `+1.650s`, and its validator RPC returned “Submitted new block” at `+1.659s`. Slot 16 finished at `+1.817s`; an independent peer had imported it by `+2.059s`, while the owner import was `+2.707s` and the validator's success log came later at `+6.774s`. The later validator line is therefore not the first network-publication time.

The same separation is visible in the retained owner for slot 97, whose start was `01:49:24Z`:

| Stage | UTC timestamp | Offset | Raw anchor |
|---|---|---:|---|
| Beacon block construction complete | `01:49:27.662622713` | `+3.662s` | `runs/round2/prysm-geth-1/beacon.log:1136845` |
| Owner beacon node imported block | `01:49:28.168476738` | `+4.168s` | `runs/round2/prysm-geth-1/beacon.log:1136901` |
| Envelope arrival captured | about `01:49:28.174` | `+4.174s` | `arrivedMs=4174`, beacon line 1136905 |
| Validator RPC returned “Submitted new block” | `01:49:28.177324632` | `+4.177s` | `runs/round2/prysm-geth-1/validator.log:2212` |
| Envelope processing lines emitted | `01:49:28.190433299–01:49:28.190452731` | `+4.190s` | beacon lines 1136904–1136905 |

No `Building block` or `Chose payload bid` line for slot 97 survives in that retained beacon log. The finish, import, envelope, and validator-return anchors are present and internally consistent.

Later retained-owner examples also finish and submit within the 12-second slot: slot 99 at `+0.285/+0.991s`, slot 105 at `+3.087/+3.934s`, slot 134 at `+5.117/+5.820s`, slot 153 at `+0.844/+1.442s`, slot 169 at `+1.457/+2.926s`, slot 197 at `+5.257/+5.958s`, and slot 208 at `+1.676/+2.238s` (finish/validator-return offsets).

## What the timers establish

- `Building block` and `Finished building block` use the beacon node's wall clock minus the slot start at entry to and return from `GetBeaconBlock`. The finish precedes validator signing and submission.
- `Submitted new block` is emitted after the validator's proposal RPC returns. It is a success-completion marker, not the initial broadcast instant.
- `Synced new block` is emitted after that beacon node processes/imports the block. It measures that node, so peers can report widely different offsets for one block.
- `Payload envelope arrivedMs` is captured when that node starts applying the envelope, relative to the envelope's own slot. The associated lines may be emitted after processing and cover gossip, queues, fetch, initial sync, and the proposer's own non-gossip publish route.

This explains apparent contradictions in the timestamps without assigning an unobserved cause. For example, among the 12 currently retained observers, slot 15 import offsets range from `+1.649s` to `+25.711s`, and slot 16 ranges from `+1.982s` to `+17.727s`. Those late tail records show delayed receipt or processing on individual nodes; the early imports prove the blocks were already available.

A few later blocks lack a retained owner record. Slot 19's earliest import among the current 12 is `+12.406s`, and slot 159's is `+12.525s`. With no owner build/submission log, these observations cannot distinguish late original publication from transport, queuing, sync, or local processing delay, and cannot establish why the delay occurred.

The slot 15/16 owner anchors survive in `runs/diagnostics/round2-slots-0-16/raw_evidence_excerpts.md` and the summarized stage table in `build-failure-audit.md`. Their source archives were in the now-missing temporary 1,000-node union; later examples above come from the currently retained `runs/round2` files.
