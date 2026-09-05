# Round 2 slots 0–100: Goldfish vote retention audit

This audit streams only `beacon.log` from the saved node 1, 201, and 400
archives. The reproducible parser is `round2_vote_retention.py`. Validator
ownership must be derived from each archive's `Validator activated` records;
node-number arithmetic is invalid because the 596-key allocations were shuffled.

## What the ledger proves

Nodes 201 and 400 independently record the same complete 512-seat splits during
the first recovery:

| vote slot | root split at node 400 | implication |
|---:|---|---|
| 15 | 54 slot-15 / 458 genesis | slot 15 arrived early enough for only a minority of its committee |
| 16 | 23 slot-16 / 489 slot-15 | the new block did not replace the retained view for most votes |
| 17 | 112 slot-17 / 400 slot-15 | likewise |
| 18 | 512 slot-15 | complete retention of the older view |
| 19 | 512 slot-15 | complete retention of the older view |
| 20 | 255 slot-18 / 257 slot-15 | split almost exactly at the Goldfish majority boundary |
| 21 | 506 slot-21 | the newly observed view finally dominates (six votes were not accepted in node 400's ledger) |

This is stronger than merely observing that blocks existed. It shows the
available committee continued signing older roots across slots 16–20, and it
matches the independently reconstructed parent bypass: later canonical blocks
build on slot 15 rather than the intervening blocks.

Activation-derived ownership and each owner's own BN log explain the minority
splits without assuming that the globally first observer represented the voter:

| slot/root | seats and owner | owner's import of that slot |
|---|---|---|
| 15 new | 54, node 68 | `01:33:02.177`, +2.177 s (`beacon.log:554`), before due |
| 15 genesis | 458, node 69 | `01:33:12.466`, +12.466 s (`beacon.log:562`), after due |
| 16 new | 23, node 94 | `01:33:15.011`, +3.011 s (`beacon.log:543`), at/just after due |
| 16 slot-15 | 489, node 93 | `01:33:17.069`, +5.069 s (`beacon.log:637`), after due |
| 17 new | 112, node 146 | `01:33:27.035`, +3.035 s (`beacon.log:554`), just after due |
| 17 slot-15 | 400, node 145 | `01:33:29.640`, +5.639 s (`beacon.log:547`), after due |

The +3 s due point schedules wakeup; it is not a hard block-rejection cutoff
or proof of the actual data-request timestamp. The new-root voters at slots
16/17 import just after that point, illustrating that scheduling matters.
Historical data-request starts are not recorded for these owners.

The vote groups are again one root per owner. Despite globally early imports
(roughly +1.6/+1.9/+2.6 seconds), import was not logged at the dominant
committee owners by the scheduled vote wakeup, and their vote groups name the
older roots. Slots 15–17 therefore support the same explanation as slot 51:
owner-local head availability, amplified from one 596-key VC into hundreds of
available-committee votes. The exact data-request ordering is unlogged, and
these logs do not separate packet arrival from validation, scheduling, and
import processing.

The same pattern repeats after startup. Node 400 records all 512 accepted seats
for the prior retained root in vote slots 34, 39, 45, 65, 68, 70, 74, 75, 78,
81, 82, 97, and 100. Those are the same slots the envelope lineage later skips.
Thus “a block was imported” is not equivalent to “the committee retained and
voted for that block.” Nor are slots 15–100 uniformly healthy merely because
most contain an observed block.

Slot 82 is another resolved retention-gate example. Its 512 old-root votes map
to 237 keys on node 1 and 275 on node 194. Their BNs log completion of slot-82
import only at `01:46:27.901`, +3.900 (`beacon.log:933050`), and
`01:46:27.654`, +3.654 (`beacon.log:789`), respectively, after the +3
second available-attestation due point. Both groups' recorded votes name slot
80. A different observer's +2.714 first import and the proposer's +2.808 RPC
completion do not imply that either committee owner's local import completed in
time. The logs establish the old-root vote outcome and late owner-local
imports, not whether the upstream delay was network delivery or local
processing, nor node 194's exact data-request time.

Node 1 supplies stronger local ordering for its 237 slot-82 votes: its
`outcome=local` records all name slot 80 and begin at
`01:46:27.352531562` (+3.352 s, `beacon.log:931608`), before its +3.901 s
slot-82 import. The local RPC inserts into fork choice before emitting these
records. Thus the old view was actually inserted locally before that import,
not merely inferred from a remote observer's receipt time.

Node 1 is an unsuitable completeness oracle during the initial overload: it
records only 458 accepted votes in slot 15, none in slot 16, 12 in slot 17, 21
in slot 19, and 257 in slot 20. Nodes 201 and 400 agree on the root splits and
usually on all 512 seats, demonstrating observer-local processing loss/delay in
node 1 rather than a smaller global committee.

## Slot 51: a globally early block completes local import late

Slot 51 is the clearest counterexample to “arrival before +3 seconds guarantees
selection.” Node 400 imported slot 51 at about +1.458 seconds, yet its accepted
ledger is 44 votes for the slot-51 root
`0x87367018…` and 468 for the slot-50 root `0x0777d67e…`. Nodes 1 and 400 agree
exactly; node 201 has the same 468 old-root votes and records 44 new-root votes
(32 `accepted` and 12 `replayed` through its pending path).

The activation-record census resolves the two root groups exactly. All 468
old-root validators (`106325–106792`) belong to node 74, while all 44 new-root
validators (`106793–106836`) belong to node 75. Node 74 logs import completion at
`01:40:15.774`, +3.773 seconds (`round2-prysm-geth-74.tar.gz`,
`beacon.log:702`), after the +3 second due point. Node 75 logs it at
`01:40:13.476`, +1.475 seconds (`beacon.log:675`) and its
new-root votes reach node 400 at +1.686 to +1.786 seconds. Node 74's old-root
votes reach node 400 at +4.052 to +4.171 seconds. Node 400's own +1.458 import
does not help validators connected to node 74.

This eliminates the apparent mixed-wallet/cache contradiction and supplies the
direct slot-51 retention evidence: the dominant committee owner's local block
import had not completed at the scheduled available-attestation wakeup, and
its 468 resulting votes all name the previous root. This is consistent with
the source's one-response-per-slot cache; the precise request time is not
logged. The early *network-wide* first import was not an early import at that
committee owner's BN.

The source explains why a small number of machines can lock in a whole committee
view. `validator/client/available_attestation.go:33-70` waits for either the
same-slot block feed or `AvailableAttestationDueBPSHeze` (+3 seconds). After the
wait, `getAvailableAttestationData` uses one slot-keyed cached response per VC
(`validator/client/validator.go:839-875`). With 596 keys on a machine, one fetched
view is reused broadly. Slot 51 follows that source invariant: node 74's committee
subset is uniformly old and node 75's uniformly new.

The VC feed is not a raw block-gossip notification. A `head_v2` event reaches
`ProcessEvent`, which calls `setHighestSlot` and sends `slotFeed` before updating
the VC's local head tracker (`validator/client/validator.go:1080-1135`), but the
BN emits that head event only after `saveHead` has installed and persisted its
chosen head (`beacon-chain/blockchain/head.go:164-197`). The available-data RPC
then reads `ForkChoiceStore.CanonicalNodeAtSlot` under its read lock and has no
separate BN response cache (`beacon-chain/rpc/core/validator.go:1167-1190`). Thus
node 74's late local import supports the old-view explanation; a claimed
notification-before-BN-head-installation race is not supported by this path.

Exact R2 sets both `AvailableAttestationDueBPSHeze` and
`AttestationDueBPSGloas` to 2500 basis points (+3 seconds), and the Gloas branch
is active. `withHeadHint` itself only attaches metadata and does not wait. REST
freshness routing can repoll to that same deadline (with a
500 ms floor when it has already passed), while the direct gRPC path does not use
those REST options. Nothing in this changes the owner/import discriminator above:
each dominant owner's import log is later than +3 seconds. The +4.05-second
observation is the remote ledger arrival time, not itself the selection deadline.

## Accepted is not identical to retained

`outcome=accepted` is emitted by gossip validation immediately before returning
`pubsub.ValidationAccept` (`beacon-chain/sync/validate_beacon_attestation.go:605-625`).
Only the subsequent subscriber obtains the fork-choice write lock and calls
`InsertAvailableAttestation` (`sync/subscriber_beacon_attestation.go:68-78`,
`blockchain/receive_available_attestation.go:23-47`). A ledger line therefore
proves validation and the root carried by the vote, but not by itself that the
vote entered the store before the next `NewSlot` pruning/scoring cutoff. Local
votes are stronger: their RPC inserts into fork choice before recording
`outcome=local`.

This ordering makes the audit conservative. The repeated 512-seat prior-root
records prove what peers sent and validated; the actual retained store could be
smaller if fork-choice lock admission lagged. They do not prove that every
accepted vote contributed to the next head walk.

A final boundary check found no `Goldfish votes`/`goldfish-summary` records
anywhere in the node 201 or node 400 archives, and no retained-store or
gate-metrics snapshot. The exact slot-52 reorgs are node 201
`beacon.log:250342` at `01:40:24.006529` and node 400 `:248356` at
`01:40:24.006903`, both 51 to 50. At slot 83 they are node 201 `:439352` at
`01:46:36.007287` and node 400 `:436286` at `01:46:36.004922`, both 82 to
80. Thus the retreats really occur at the scoring boundary. The source's
optional summary reports total voters/seats, not per-root support; its gate
branches update metrics rather than log a deciding-store dump. These records
cannot supply the exact historical retained denominator or distinguish every
gate/viability branch.

## Reproduce

```text
python3 runs/diagnostics/startup3/round2_vote_retention.py \
  --owner-archives-dir /tmp/prysm-r2-extra-logs.Rd7MjT \
  --owner-archives-dir runs/round2 \
  runs/round2/round2-prysm-geth-1.tar.gz \
  runs/round2/round2-prysm-geth-201.tar.gz \
  runs/round2/round2-prysm-geth-400.tar.gz
```

The parser never extracts archive paths. It streams only a regular file whose
basename is exactly `beacon.log`, summarizes outcome/reason/root/seat counts and
arrival/decision ranges, and optionally emits bounded raw matching records.
