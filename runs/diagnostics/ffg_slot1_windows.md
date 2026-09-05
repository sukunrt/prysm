# Slot-1 FFG validation-entry versus completion windows

`ffg_window_analysis.py` scanned the finalized per-node beacon logs. Windows
are relative to slot 1 (genesis + 12 seconds). `arrivedMs` is the vote's
validation-entry clock; the line prefix is when its completion record was
emitted. Thus the two tables classify the same records using different clocks.

Cells are `records (largest committee count on one node)`. Counts are log
records, not unique network votes: the same vote is observed at multiple
nodes. Within each node the diagnostic identity
`(validator, committeeIndex, dataRoot, blockRoot, attSlot)` was unique for every
record. Across nodes, round 1 has 46,696 records but 14,134 such identities;
round 2 has 36,024 records but 13,094 identities.

These timing tables deliberately require `arrivedMs` and therefore exclude 20
round-1 and five round-2 `FFG vote included` records. Those later inclusion
summaries have no validation-entry clock and are not individual arrival or
completion observations. Including them, the finalized slot-1 FFG log-record
checksums used by `early_slots_findings.md` are 46,716 and 36,029.

## Validation-entry (`arrivedMs`)

| round/node | <6s | 6-12s | 12-24s | >=24s | total |
|---|---:|---:|---:|---:|---:|
| r1/1 | 821 (149) | 127 (48) | 1,494 (298) | 9,570 (1,716) | 12,012 |
| r1/201 | 1,378 (271) | 365 (123) | 1,471 (281) | 7,371 (1,270) | 10,585 |
| r1/400 | 1,576 (334) | 655 (165) | 1,742 (363) | 7,695 (1,306) | 11,668 |
| r1/50 | 1,197 (235) | 560 (132) | 1,272 (248) | 9,402 (1,624) | 12,431 |
| **r1 records** | **4,972** | **1,707** | **5,979** | **34,038** | **46,696** |
| r2/1 | 1,800 (378) | 367 (99) | 1,587 (413) | 4,791 (1,048) | 8,545 |
| r2/150 | 731 (165) | 1,646 (354) | 3,122 (642) | 4,282 (976) | 9,781 |
| r2/201 | 917 (500) | 410 (252) | 899 (472) | 2,107 (1,095) | 4,333 |
| r2/400 | 718 (403) | 480 (250) | 1,236 (754) | 1,238 (742) | 3,672 |
| r2/50 | 2,565 (620) | 596 (219) | 1,362 (360) | 5,170 (1,055) | 9,693 |
| **r2 records** | **6,731** | **3,499** | **8,206** | **17,588** | **36,024** |

Round 1's <6-second entries comprise 4,832 gossip and 140 local records;
round 2's comprise 6,519 gossip and 212 local records. All later entries are
gossip in these logs.

## Completion-log emission

| round/node | <6s | 6-12s | 12-24s | >=24s | total |
|---|---:|---:|---:|---:|---:|
| r1/1 | 117 (28) | 175 (36) | 511 (107) | 11,209 (1,950) | 12,012 |
| r1/201 | 27 (9) | 0 (0) | 107 (25) | 10,451 (1,809) | 10,585 |
| r1/400 | 45 (12) | 139 (37) | 127 (36) | 11,357 (2,050) | 11,668 |
| r1/50 | 137 (30) | 264 (71) | 314 (72) | 11,716 (2,044) | 12,431 |
| **r1 records** | **326** | **578** | **1,059** | **44,733** | **46,696** |
| r2/1 | 138 (29) | 2 (2) | 21 (8) | 8,384 (1,680) | 8,545 |
| r2/150 | 259 (56) | 477 (121) | 608 (157) | 8,437 (1,621) | 9,781 |
| r2/201 | 34 (19) | 124 (71) | 178 (118) | 3,997 (2,045) | 4,333 |
| r2/400 | 240 (127) | 121 (93) | 468 (236) | 2,843 (1,461) | 3,672 |
| r2/50 | 166 (32) | 5 (2) | 239 (53) | 9,283 (1,690) | 9,693 |
| **r2 records** | **837** | **729** | **1,514** | **32,944** | **36,024** |

The <6-second emissions comprise 186 gossip plus 140 local records in round 1,
and 625 gossip plus 212 local records in round 2. All later emissions are
gossip. In the finalized logs, 95.8% of round-1 and 91.4% of round-2 slot-1
records emit at least 24 seconds after slot 1 starts. Conversely, thousands of
validation entries already existed before the six-second mark
(4,972 and 6,731 records across sampled nodes), while only 326 and 837
completion records had emitted by then. This is evidence of a large backlog
between validation entry and completion-log emission; it does not separate
validation computation, lock/subscriber waits, scheduling, or log-output delay,
nor establish how
many of the cross-node records were available to any particular proposer.

The earlier `early_slots_findings.md` totals (7,912 and 9,785) were stale
analysis totals and must not be used as full-file checksums. The saved log
files were not live captures during this analysis. Full-file counts include
slot-1 completion records emitted long after slot 1 ended; they must not be
interpreted as pool entries available during slot 1.

Reproduce with:

```sh
python3 runs/diagnostics/ffg_window_analysis.py
```
