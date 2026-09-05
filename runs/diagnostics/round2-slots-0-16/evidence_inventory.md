# Round-2 slots 0–16 evidence inventory

This directory is a fresh read-only extraction from the saved round-2 logs. It
does not use a devnet rerun. The parser reads the raw archive members, strips
ANSI color only for matching/output, and preserves archive path, member name,
and original member line number on every evidence row.

## Raw source census

- `/tmp/prysm-r2-extra-logs.Rd7MjT` contains 993 node archives, including the
  additional detailed logs for nodes 100, 600, and 800.
- `/home/sukun/dev/prysm2/runs/round2` contains 12 node archives.
- The five duplicates are nodes 151, 300, 500, 700, and 900. The extractor uses
  the first directory's compact copy for those nodes and excludes the detailed
  copy from node counts. The union is exactly nodes 1–1000.
- Every selected archive has `beacon.log`, `validator.log`, `execution.log`,
  `snooper-engine.log`, and `xatu-sentry.log`. No selected archive has a runtime
  trace member.
- `/tmp/prysm-r2-extra-logs.Rd7MjT/extracted/round2-{22,169,191}` are extracted
  copies of three owner archives, not additional nodes.
- Existing derived indexes found under `/tmp` include
  `round2-all1000-union.json`, `round2-later-union.json`,
  `round2-index-{1-20,check}.json`, `r2-observers.json`,
  `r2-vote-owners.json`, and `round2-vote-retention.json`. They were pointers or
  cross-checks, not primary evidence. `/tmp/proposer.r2` is Go source scratch,
  not a log or archive.

The fresh scan found all 120,000 `Validator activated` indices exactly once,
with no cross-node ownership conflicts. All proposer schedule records used for
slots 1–16 contain an explicit `slot`; no time-based slot inference was needed.
All 1,000 selected beacon logs contain `Chain genesis time reached`.

## Complete slot result

Slot 0 is genesis. Slots 1–14 have zero `Synced new block` records across the
1,000-node union. Slot 15 has exactly one import record on every node, all with
the abbreviated logged root `0x68564190...`; slot 16 likewise has exactly one
per node with `0x4b07a2a4...`.

| slots | proposer owner(s) | directly observed terminal boundary |
| --- | --- | --- |
| 1 | 169 | RANDAO domain lookup deadline |
| 2, 3, 7, 11, 12 | 191, 22, 144, 117, 14 | sync-index preflight deadline, then RANDAO deadline |
| 4 | 91 | sync-selection signing preflight deadline, then RANDAO deadline |
| 5, 8 | 118, 19 | late BN build start; VC block-request deadline; local-payload/no-fallback build failure |
| 6, 9 | 83, 107 | BN local-payload/no-fallback failure; VC returns the corresponding Internal failure |
| 10, 14 | 35, 85 | payload selected; VC block-request deadline; BN eventually returns canceled state-root/build failure |
| 13 | 20 | VC block-request deadline; BN parent-state slot processing returns canceled |
| 15, 16 | 32, 76 | block built and VC submission completed |

These are log boundaries, not historical runtime profiles. The prior reports'
common startup-pressure explanation is supported by source analysis and a
separate controlled reproduction. These historical archives themselves contain
no scheduler/runtime trace that assigns every owner delay to that mechanism.

The earliest slot-15 import is node 331 at
`2026-09-05T01:33:01.591600Z`, archive-member line 404, 1.591600 seconds after
the slot start. The earliest slot-16 import is node 19 at
`2026-09-05T01:33:13.923885Z`, line 599, 1.923885 seconds after its slot start.
The cached 1,000-node union has the same per-slot importing-node sets for all
slots 0–16.

## Goldfish vote cohorts for slots 15 and 16

The vote recount scans the detailed node 201 and node 400 beacon logs for their
per-validator Goldfish ledgers. Each observer records 512 validators at each slot.
After grouping by validator, both observers have identical validator-to-root
mappings: there are no duplicate validators or conflicting roots in these four
recorded cohorts. Every record has `outcome=accepted` and one seat. This is a
statement about the two scanned ledgers, not a network-wide equivocation audit.

| observer | vote slot | root represented | seats | activation owner | first–last accepted log time (UTC) |
| ---: | ---: | --- | ---: | ---: | --- |
| 201 | 15 | slot 15 `0x68564190…` | 54 | 68 | 01:33:02.440482–01:33:02.512597 |
| 201 | 15 | genesis `0x1a40155d…` | 458 | 69 | 01:33:09.640716–01:33:09.866405 |
| 400 | 15 | slot 15 `0x68564190…` | 54 | 68 | 01:33:02.390275–01:33:02.431655 |
| 400 | 15 | genesis `0x1a40155d…` | 458 | 69 | 01:33:09.649967–01:33:09.797016 |
| 201 | 16 | slot 16 `0x4b07a2a4…` | 23 | 94 | 01:33:15.261736–01:33:15.478036 |
| 201 | 16 | slot 15 `0x68564190…` | 489 | 93 | 01:33:15.672767–01:33:16.548660 |
| 400 | 16 | slot 16 `0x4b07a2a4…` | 23 | 94 | 01:33:15.211242–01:33:15.429185 |
| 400 | 16 | slot 15 `0x68564190…` | 489 | 93 | 01:33:15.698190–01:33:16.520535 |

A separate fresh audit applies
`decoupled.AvailableAttestationSeatsToValidatorIndices`'s hash and big-endian
slot rule with an electorate of 120,000. Slot 15 computes the exact committee
interval 93,514–94,025, split by activation records into node 68's 54 indices
and node 69's 458. Slot 16 computes 27,068–27,579, split into node 93's 489
and node 94's 23. Both observer ledgers exactly equal their computed 512-index
committee. These owners come from archived activation records, without
validator-number or node-number arithmetic.

The activation owners' own import logs match the split. Node 68 imports slot 15
at +2.177131 seconds, while node 69 imports it at +12.466413. Node 94 imports
slot 16 at +3.010779, while node 93 imports it at +5.069302. Node 400 then logs
slot 15 → genesis at raw `beacon.log:31677`, and slot 16 → slot 15 at raw
`beacon.log:37320`. The accepted ledger line is emitted before subscriber
fork-choice insertion, so it proves validation and carried root, but does not
by itself prove retained-store admission before the scoring boundary.

## Reproduction and outputs

The independent committee/ownership check can be reproduced with:

```bash
python3 runs/diagnostics/round2-slots-0-16/validate_goldfish_committees.py \
  /tmp/prysm-r2-extra-logs.Rd7MjT runs/round2 --workers 8
```

This reads the archived activation records and the extracted vote ledger. It
does not contact or start a node.

```sh
python3 runs/diagnostics/round2-slots-0-16/extract_census.py \
  /tmp/prysm-r2-extra-logs.Rd7MjT runs/round2 \
  --cross-check-union /tmp/round2-all1000-union.json --workers 4
```

- `census.json`: structured inventory, slots, owners, anchored owner events,
  reorg observations, deduplicated vote cohorts, and cross-check result.
- `slot_summary.tsv`: one row per slot.
- `node_slot_census.tsv`: all 17,000 node/slot outcomes.
- `owner_events.tsv`: 44 anchored VC/BN owner events.
- `raw_evidence_excerpts.md`: compact ANSI-free owner and reorg excerpts with
  untouched archive-member line numbers.
- `archive_inventory.tsv`: one row per selected node, including excluded copies.
- `goldfish_vote_groups.tsv`: eight root/observer groups with first/last timing.
- `goldfish_votes_15_16_unique.tsv`: 2,048 deduplicated validator rows.
- `validate_goldfish_committees.py`: recomputes the slot-15/16 committee and
  freshly reads the corresponding validator owners from activation records.
- `goldfish_committee_intervals.tsv`: eight observer/root/activation-owner
  intervals with computed committee offsets and activation-log anchors.
- `goldfish_committee_validation.md`: compact committee and ownership result.

The committee/owner audit is reproducible with:

```sh
python3 runs/diagnostics/round2-slots-0-16/validate_goldfish_committees.py \
  /tmp/prysm-r2-extra-logs.Rd7MjT runs/round2
```

The absence conclusions are bounded to the complete saved 1,000-node archive
union. They do not establish that an event outside these captures was impossible.

## Sixteen-owner early-timeline extension

The follow-up extractor is bounded to the 16 owner archives and the interval
genesis minus 12 seconds through genesis plus 240 seconds:

```sh
python3 runs/diagnostics/round2-slots-0-16/extract_owner_early_timeline.py \
  --archive-dir /tmp/prysm-r2-extra-logs.Rd7MjT
```

It generated:

- `owner_slot_activity.tsv`: 352 compact owner/slot rows, including all parsed
  message-type counts and Engine RPC counts.
- `owner_failure_neighborhood.tsv`: 16 before/during/after proposal summaries.
- `validator_role_progress.tsv`: 1,275 startup schedules, duty schedules,
  submitted-role, and payload-skip summaries, including list cardinalities.
- `owner_message_types.tsv`: 1,328 component/severity/message-type groups with
  exact counts and first/last source anchors.
- `owner_nonroutine_events.tsv`: all 15,448 WARN/ERROR records in the bounded
  window, sanitized and individually anchored.
- `owner_early_timeline.tsv`: 17,159 nonroutine and selected routine progress
  records, with timestamps, derived slot, selected fields, and sanitized text.
- `engine_rpc_timeline.tsv`: 148 matched `engine_forkchoiceUpdatedV4` and
  `engine_getPayloadV6` pairs. It emits selected HTTP metadata and JSON body
  fields and never emits HTTP headers.
- `getpayload_timeout_discriminators.tsv`: the four slot 5/6/8/9 request,
  proxy-copy, and BN timeout boundaries.
- `owner_timeline_excerpts.md`: compact per-owner schedules, proposer events,
  error-group boundaries, subsequent role progress, and Engine summaries.
- `owner_early_timeline_findings.md`: interpreted phase split, exact timeout
  source identity, and evidence limits.
- `offline-preflight-results.md`: controlled `RolesAt` and attestation-cache
  reproductions and the Bazel command used to run them.
- `owner_timeline_archives.tsv`: exact 16 input paths and sizes.

The 16 compact archives contain the same five member types recorded above and
no historical runtime traces. The timeline extension intentionally scans no
other node and performs no devnet rerun or download.
