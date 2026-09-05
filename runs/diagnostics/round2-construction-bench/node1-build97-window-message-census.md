# Node-1 message census during proposal 97

This census covers the retained outer-log interval from one second before the
proposal-97 build marker through the finish marker, inclusive:

- window start: `2026-09-05T01:49:23.008736237Z`;
- build start: LF record 1,121,950 at
  `2026-09-05T01:49:24.008736237Z`;
- build finish: LF record 1,136,845 at
  `2026-09-05T01:49:27.662622713Z`.

The build's retained outer timestamps span 3,653.886 ms. Offsets below are
milliseconds after that **build-start outer timestamp**, not milliseconds
after absolute slot start. The build record itself reports
`sinceSlotStartTime=8.492268ms`; the finish record reports
`sinceSlotStartTime=3.662413031s`.

| Retained message class | Outcome | Rows | First/last outer-log offset from build start |
| --- | --- | ---: | ---: |
| FFG vote | gossip | 14,355 | +28.584 / +3,523.767 ms |
| FFG vote | local | 71 | +101.494 / +257.993 ms |
| Goldfish head vote | accepted | 465 | +3,422.862 / +3,519.504 ms |
| FFG aggregate | any | 0 | - |
| PTC vote | any | 0 | - |
| sync committee/contribution | any | 0 | - |
| block construction markers | - | 3 | 0 / +3,653.886 ms |
| other | - | 2 | +3.198 / +3.418 ms |

The two `other` records are the graffiti record and the Eth1-data warning.
The message-type totals are 14,896 records. The paired validation fixture's
14,355 gossip FFG inputs reproduce the dominant logged class, while omitting
71 local FFG ledger rows and 465 accepted Goldfish rows. Those 536 other
ledger rows are 3.6% of all retained records in the interval. This is a count,
not a measurement of their CPU cost.

## Goldfish clock boundary

The deployed `logVote` receives `arrived` from validation entry and records
`arrivedMs = arrived - vote-slot start`; `decidedMs` is the decision/log time
minus that same slot start (`beacon-chain/sync/vote_ledger.go:43-61`). The 465
Goldfish records have `arrivedMs=3423-3471` and `decidedMs=3430-3523` for vote
slot 97. Because the proposal began at slot +8.492 ms, those arrival fields
place Goldfish validation entry about 3,414.5-3,462.5 ms after build start.
Their outer log emissions occur about 3,422.9-3,519.5 ms after build start.

Accordingly, these logged Goldfish validations do not begin early enough to
explain the first 3.4 seconds of this build interval. They do establish an
additional validation/forkchoice ledger class near the tail. This statement is
limited to the 465 logged rows; it does not exclude unlogged work.

## Pre-build and observability limits

There are no retained records in the requested one-second pre-build interval.
The immediately preceding retained record is a PTC vote at
`2026-09-05T01:49:21.368256250Z`, about 2.640 seconds before build start. A
quiet retained log interval does not prove that the process had no active
goroutines, queue backlog, pool contents, or CPU work.

The parser scanned 16,846 bounded LF records: 1,950 before the selected time
window and 14,896 inside it. It had zero header parse failures and no records
after the finish within the line bound. Counts are log emissions, not
subscriber completion, pool-snapshot, queue-depth, or CPU measurements.

The compact [JSON artifact](node1-build97-window-message-census.json) retains
message/outcome timestamps, ledger-field ranges, components, unique-validator
counts, raw event anchors, and parser accounting. Reproduce it with:

```bash
python3 runs/diagnostics/round2-construction-bench/parse_node1_build97_window.py
```

