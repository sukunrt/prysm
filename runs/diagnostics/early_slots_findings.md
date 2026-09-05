# Startup trace, slots 0--3

Generated from the 10 `runs/round1/prysm-geth-*` and 12
`runs/round2/prysm-geth-*` directories with `early_slots.py`. The top-level
`runs/beacon.log` was deliberately excluded.

| round/slot | Goldfish outcomes (nodes) | FFG log records, including inclusion summaries (nodes) | reported successful submission records (nodes) |
|---|---:|---:|---:|
| r1/0 | none | 148 (4) | 272 (5) |
| r1/1 | 10 accepted, 1 local, 405 dropped (4) | 46,716 (4) | 295 (5) |
| r1/2 | 290 dropped (1) | 11,615 (4) | 205 (3) |
| r1/3 | 114 accepted, 95 dropped (3) | 12,489 (4) | 192 (3) |
| r2/0 | none | 198 (3) | 427 (7) |
| r2/1 | 177 accepted, 1 local, 1,006 dropped (5) | 36,029 (5) | 424 (8) |
| r2/2 | none | 6,717 (5) | 1 (1) |
| r2/3 | 174 dropped (2) | 7,400 (5) | 1 (1) |

These finalized counts include slot-tagged completion and aggregate-summary
records emitted after the vote's own slot; they are not counts of votes that
were available during that slot. They are observed log records across sampled nodes, not unique network votes
or unique chain validators; successful-submission counts sum pubkeys reported
in validator batches and may contain duplicates across observations.
An `accepted` ledger record also does not prove fork choice consumed that vote.
Every drop in these slots is `not_current_slot`. Round 2 stalls abruptly after
slot 1: validator success falls from 424 validators on 8/12 sampled nodes to a
single validator on 1/12 nodes in each of slots 2 and 3. Five sampled nodes
eventually log FFG records tagged with slots 2 and 3; those delayed records do
not imply on-time processing. Node 900 is the lone reported slot-2 submitter
and node 500 the lone reported slot-3 submitter.

Submission time must be read from `submittedSinceSlotStart`, not the emission
prefix. In round 2 slot 1, submission values span 116--6,671 ms while emission
across sampled nodes with the declared run genesis spans 204--12,000 ms. Thus lines
emitted at the boundary can still record on-time work. Validator deadline and
failure counters are included per slot and node in the JSON output.

Round 2 has 14 validator errors in slot 1 on four nodes. Slot 2 has 438
failure/warning records on six nodes: 424 attestation-request deadlines, seven
aggregate-data failures, and seven aggregate-proof submission failures. Slot 3
has 465 records on seven nodes: 446 attestation-request deadlines, nine
aggregate-data failures, nine aggregate-proof failures, and one attestation
submission failure. Round 1 already shows four errors on two nodes in slot 1,
then 115 records on three nodes in slot 2 and 128 on three nodes in slot 3.

Raw anchors: round-2 node 1 reports the slot-1 aggregate-proof deadline at
`validator.log:687`, followed by slot-2 attestation-request deadlines beginning
at `validator.log:690`. Round-2 node 400 records a slot-1 Goldfish vote at
`beacon.log:1135` with `arrivedMs=4186`, later `decidedMs=10284`, and
`outcome=accepted`; this remains an observed ledger decision, not proof of
fork-choice consumption.
