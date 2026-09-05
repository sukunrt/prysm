# Round-2 first recovery: owner ordering and the early round-2 rejection

This is a read-only source/raw-log follow-up. All source line numbers below refer to `0280403c70d88967f49d2d4c730f4c5417dabdf5`, obtained with `jj --ignore-working-copy file show -r REV PATH`. Archive line numbers are the unmodified `./beacon.log` member. Node 69/93 archives are under `/tmp/prysm-r2-extra-logs.Rd7MjT`; detailed 201/400 archives are under `runs/round2`. Genesis is `01:30:00Z`; slots last 12 seconds and rounds contain eight slots.

## The apparent early round advance has an exact source explanation

Node 93 logs `target round 0 does not match current round 2 or prev round 1` at `01:33:10.197590711`, `beacon.log:550`. Wall-clock slot 16 starts at `01:33:12`, so this is still wall-clock slot 15/round 1. It is not evidence that the node's head state advanced its clock to slot 16.

The exact dependency path is:

1. `blockchain/receive_attestation.go:23–25` defines `reorgLateBlockCountAttestations = 2 * time.Second`.
2. Its ticker runs at slot offset 0 and `SecondsPerSlot - 2s` (`:92–103`). The latter calls `UpdateHead(ctx, slot+1)` when validating.
3. `UpdateHead` takes the fork-choice write lock (`:127–128`) and passes `MaximumGossipClockDisparityDuration() + 2s` to `processAttestations` (`:129–133`). This disparity is used on both ticker calls.
4. `blockchain/process_attestation.go:66` validates the target using **`time.Now().Add(disparity)`**.
5. `blockchain/process_attestation_helpers.go:178–187` calculates `currentSlot = slots.At(genesis, now)`, then `RoundAt(currentSlot)`. It does not derive the current round from the head/checkpoint state's slot.

The fixed two seconds alone makes the effective time at this warning at least `01:33:12.197590711`, in round 2. The mainnet default gossip disparity is another 500 ms (`config/params/mainnet_config.go:385`, duration conversion in `config/params/config.go:733–734`), but that default need not be assumed to explain this record. This is implemented future-time validation for early head preparation. Source explains why it happens; calling it protocol-correct would require a separate specification judgment.

## What the 85 warning burst actually removes

Node 93 has 85 consecutive matching warnings, `beacon.log:550–634`, from `01:33:10.197590711` through `01:33:10.352001121`: **154.410410 ms between the first and last log timestamps**. This is an observed burst span, not measured CPU time or a complete `UpdateHead` duration. `aggregatedCount` is each attestation's aggregation-bit count, not the number of queue entries scanned.

`receive_attestation.go:191–196` snapshots fork-choice attestations. For each candidate it checks time and local block/state presence (`:202–211`), **deletes the fork-choice entry before validation** (`:213–219`), checks wall-clock checkpoint round (`:221–223`), and calls `receiveAttestationNoPubsub` (`:225`). The warning is at `:245`.

Within `OnAttestation`, `getAttPreState` happens at `process_attestation.go:60`, before the future-time gate at `:66`. Thus these rejected entries still incur checkpoint-state retrieval and its locks; they skip later beacon-block validation, committee conversion/index validation and conventional fork-choice vote application (`:71–118`). On this path, `AttestationCommitteesFromState` reaches `BeaconCommitteeFromState`, whose existing committee-cache entry returns directly (`core/helpers/beacon_committee.go:181–201`). There is no unconditional `ActiveValidatorCount` call in this conversion path.

The fork-choice pool is separate from the packing pools: `operations/attestations/kv/kv.go:21–29` has separate `aggregatedAtt`, `unAggregatedAtt`, `forkchoiceAtt`, and `blockAtt` fields; `DeleteForkchoiceAttestation` at `:66–68` deletes from the `forkchoiceAtt` map only. Goldfish available votes are a further separate store (`forkchoice/doubly-linked-tree/goldfish.go:174–192`, pruning at `:212–218`). Therefore this burst proves local processing/rejection of 85 fork-choice candidates. It does not prove that 85 expensive genesis scans were avoided, that all pending FFG work was drained, that the packing pool was emptied, or that any available votes were removed.

It also cannot cause the first successful block 15: the first recorded block-15 import at `01:33:01.591600` precedes this burst by about 8.606 seconds. Node 93 itself imported block 15 at `01:33:08.199577528` (`:548`), before its burst.

## The expensive gossip path does not expire at the same round boundary

Historical `sync/validate_beacon_attestation.go` calls `ValidateAttestationTime` at `:92`, obtains the target state at `:143`, and validates its topic at `:149`. Topic validation calls `validateCommitteeIndexAndCount` (`:253`), which calls `ActiveValidatorCount` unconditionally (`:290`). This is the previously established genesis-count hot path. A successful FFG ledger record occurs at `:243`, after signature/state validation (`:195–198`).

But its age gate is **epoch based** after Deneb: `core/helpers/attestation.go:177–187` accepts the current or previous epoch. The data slot/target-round check at `validate_beacon_attestation.go:96` checks consistency of the attestation's own fields, not its age against the current wall-clock round. With 32 slots per epoch, slot-1–7 epoch-0 attestations remain within this particular gossip age gate through slot 63. Other checks may reject individual messages; the point is that entering round 2 is not a global gossip-age cutoff.

Raw evidence confirms this distinction:

| Observer | Last recorded accepted target-round-0 single vote | Raw anchor |
|---|---|---|
| 400 | `01:33:07.009375666`, attestation slot 7, genesis root, validator 82715, `arrivedMs=102998`, `outcome=gossip` | `beacon.log:30001` |
| 201 | **`01:33:17.002933293`**, attestation slot 2, genesis root, validator 33312, `arrivedMs=172974`, `outcome=gossip` | **`beacon.log:35813`** |

Node 201's final old-round record is during slot 16, five seconds after its boundary. It proves that the scan-bearing successful validation path still completed after round 2 began. Its `arrivedMs` is anchored to the attestation's own slot and captured on validator entry; it is not a measured queue duration (`sync/vote_ledger.go:85–96`). Nodes 69/93 do not have the detailed FFG vote ledger, so their exact accepted-old-round timelines cannot be reconstructed from missing informational records.

There is nevertheless a source-backed reduction in *newly produced* genesis-slot-state traffic starting with target round 1: the target checkpoint is keyed by round, and pre-state preparation advances to that round's start (`process_attestation_helpers.go:124–139,158–174`), slot 8 for round 1. This removes the special slot-0 cached-count exclusion once the normal count-cache conditions hold. Historical target-round-1 FFG acceptance begins at node 400 `01:31:38.519633805` (`:9000`) and node 201 `01:31:48.527602008` (`:9298`), well before block 15. Old-round-0 traffic continues afterward. This supports a transition to cheaper fresh traffic plus a lingering old backlog; it does not provide a measured owner-specific explanation of why the first successful proposal is exactly slot 15.

## Actual signed old-root cohorts precede the owners' new-block import logs

The fresh `goldfish_vote_groups.tsv` provides acceptance timestamps and raw observer anchors, and `goldfish_votes_15_16_unique.tsv` maps validator ownership from activation records. These are stronger ordering evidence than comparing an import to the nominal +3-second wakeup alone.

| Vote cohort | Observer 201 accepts the complete recorded group by | Owner's new-block import log | Observable ordering |
|---|---|---|---|
| Slot 15: 458 genesis votes, all owned by node 69 | `01:33:09.866405`, last anchor `:31868` | Node 69 block 15, `01:33:12.466413371`, `:562` | Every recorded vote is already signed/received before the import log, by at least about 2.600 s |
| Slot 16: 489 block-15 votes, all owned by node 93 | `01:33:16.548660`, last anchor `:35591` | Node 93 block 16, `01:33:17.069302781`, `:637` | Every recorded vote is already signed/received before the import log, by at least about 0.521 s |

Node 400 independently observes the same 458/489 owner-root cohorts. Receipt timestamps do not expose the owners' data-RPC start, nor the exact instant an internal head pointer was updated. The precise claim is actual old-root signed output before the newer import's completion log, not proof of an exact cache-snapshot timestamp. The shared per-slot available-data cache remains the source mechanism capable of turning one owner view into hundreds of matching signatures.

Node 93 additionally proves local block processing before its late import: block-15 root `68564190…` is already marked being processed at `01:33:03.679501812` (`:545`) and again at `01:33:07.678162598` (`:547`); its import is logged at `01:33:08.199577528` (`:548`). The first marker precedes import logging by **4.520075716 s**. The entire observed import delay therefore cannot be assigned to network transit before first local block presence. It does not isolate CPU, scheduling, locks, or state processing inside that remaining interval.

Node 93 then explicitly changes head from 15 to 16 at `01:33:17.064742867` (`:636`), after all its old-root votes are recorded at observer 201, and changes back from 16 to 15 at `01:33:24.030536448` (`:641`). This demonstrates why later successful import/head selection of 16 does not retroactively change that already emitted cohort. Node 69 independently records the 16-to-15 head change at `01:33:24.009516054` (`:570`). Both owners still report finalized slot/round 0 when importing 15/16.

## Focused offline cohort replay

The completed supplied-cohort replay uses the existing `setupGoldfish`,
`insertGoldfishBlock`, and `driftGenesisTime` helpers. It is **not recovered
historical fork-choice memory**. The following lists its original design;
steps 1–4 and the two payload-bit root selections in step 5 were implemented
and passed. Full-head envelope inspection and the optional synthetic
sensitivity table in step 6 were not needed for the root-selection finding
and are not claimed as executed checks. [Actual test scope and results](offline-build-retention-results.md).

1. Make genesis and its two children at slots 15 and 16. Start at slot 15 with a clearly labeled synthetic empty slot-14 electorate; insert child 15 and assert head 15.
2. Supply slot-15 votes: 54 unique one-seat validators for 15 and 458 for genesis. At slot 16 before inserting its block, assert threshold 256, score 54 for child 15, and head genesis.
3. Insert the unique slot-16 proposal during slot 16. Assert head 16 and non-nil distinguished proposal even though the ordinary current-round-start node gate refuses it without earned votes.
4. Supply slot-16 votes: 23 unique seats for 16 and 489 for 15. At slot 17 assert no distinguished proposal, threshold 256, scores 23/489, and head 15.
5. Repeat with payload bits false/true, supplying envelopes when needed to inspect full-head status. This distinguishes root support from payload-branch support. The root sequence should remain for these supplied cohorts.
6. A small sensitivity table can vary supplied slot-16 new-root seats with old-root seats fixed at 489. New-root counts 0, 23, 488 should select 15; a 489/489 tie should stop at genesis under the strict gate; 490 should select 16. The latter counts are synthetic boundary controls, not claims about the actual 512-seat committee.

The source reads only previous-slot votes (`goldfish.go:495–504`), computes half the **recorded** seats (`:267–284`), requires a strict greater-than score (`:325–335`), and enables the distinguished proposal only at the current round-start slot (`:438–457,507–509`). This replay closes the deterministic selection chain under stated inputs. It cannot turn accepted gossip logs into proof of exact insertion counts in historical memory.

## First successful proposal is not an instant of uniform recovery

The new `owner_slot_activity.tsv` and `validator_role_progress.tsv` identify recurring owner progress before the first proposal success. The following entries were also checked against raw validator logs. Their `submittedSinceSlotStart` field is materially better than the summary line's output timestamp: `validator/client/attest.go:154–175` invokes `saveSubmittedAtt` only after successful attestation-submission RPC return; `log_helpers.go:101–109` captures the time under the submission-log mutex. `:257–258` renders first captured time relative to the summary's slot start and the last-minus-first spread, rounded to milliseconds. These are successful post-RPC recording times, not RPC starts, exact wire times, or the later summary printing times.

| Owner and later proposal | Earlier successful submission records | Evidence |
|---|---|---|
| Node 32, proposer of slot 15 | Summary 7: 77 keys, first at +8.270 s, spread 11 ms. Summary 13: 89 keys, first at +0.773 s, spread 54 ms. Summary 14: 70 keys, first at +0.008 s, spread 199 ms. | `validator.log:1135,1315,1321` |
| Node 76, proposer of slot 16 | Summary 13: 55 keys, first at +9.247 s, spread 8 ms. Summary 14: 68 keys, first at +3.042 s, spread 777 ms. | `validator.log:1829,1838` |
| Node 85, failed proposer of slot 14 | Summary 13: 71 keys, first at +9.511 s, spread 33 ms. Summary 14: 74 keys, first at +2.717 s, spread 85 ms. | `validator.log:1553,1562` |

For these offsets, add the named slot's genesis-relative start. For example node 76's summary 14 is printed at `01:33:00.000985780` (the start of slot 15), but records its first successful submission at approximately **`01:32:51.042`**, not at slot 15. The summary key omits the attestation's data slot (`log_helpers.go:56–74`) and the displayed slot is supplied by `LogSubmissions`; therefore this table deliberately identifies summary batches and their captured successful-call times. It does not use these grouped records as a unique per-duty completion census when adjacent slot work overlaps.

Three precise comparisons follow:

1. **Useful work resumed before block 15, and at different rates.** Node 32 has repeated successful-call captures well before its slot-15 proposal, including a target-round-0 batch captured during wall slot 7. In summary 13 its successful-call capture begins around `01:32:36.773`; node 76's corresponding batch begins around `01:32:45.247`, about 8.474 seconds later, both within wall slot 13. Their first successful proposals cannot be used as the first instant their services became responsive. These are wall-time progress comparisons, not proof that every listed key fulfilled the summary slot's scheduled attestation duty.
2. **Successful routine calls coexist with a proposal's unfinished required branch.** Node 85 records its first summary-14 attestation at approximately `01:32:50.717`, while its proposal begins at `01:32:50.694093299` (`beacon.log:543`) and chooses a payload at `01:32:50.744064893` (`:546`). That proposal still misses its deadline (`validator.log:1561`) and reaches its packing-cancellation/build-error logs only at `01:33:31.993917337` / `01:33:31.996258911` (`beacon.log:562–563`). It therefore had successful attestation RPCs in the same period as the blocked proposal dependency; neither total node unresponsiveness nor uniformly recovered proposal service describes it.
3. **The first success does not clear old work across the network.** Node 32 logs successful block-15 submission at `01:33:01.658630063` (`validator.log:1324`). Node 85's slot-14 required consensus branch is still outstanding and its terminal build error is logged roughly **30.338 seconds later**, during slot 17. Node 93 also has at least the 4.520-second locally-present-to-import-log interval documented above. Meanwhile node 201's accepted target-round-0 gossip at `01:33:17.002933293` shows the old scan-bearing validation path has not universally stopped.

The bounded conclusion is a scheduled proposer succeeded at slot 15 while other nodes and workflows were at different stages of progress. The records support uneven improvement and overlapping old work. They do not establish a network-wide recovery switch at slot 15, or an exact historical claim that scans stopped and thereby caused that particular slot to succeed.
