# Node 1 slot-97 FFG validation-entry timing

## Result

Node 1 logged 14,426 distinct slot-97 FFG single-vote ledger rows while the
slot-97 proposal was being built. Of these, 14,355 `outcome=gossip` rows record
gossip validation entry; 71 `outcome=local` rows come from the node's own
submission path and do not record gossip validation.

The owner logged `Building block` at +8.736 ms
([beacon.log](/home/sukun/dev/prysm2/runs/round2/prysm-geth-1/beacon.log:1121950)).
The first gossip `arrivedMs` was +30 ms and its outer emission was +37.320 ms,
21.264 and 28.584 ms after that build-start log. By +1 second, 6,021 gossip
validation entries had arrived and 4,330 gossip rows had been emitted. All
14,355 gossip votes had entered validation by +3,489 ms and all their rows had
been emitted by +3,532.503 ms.

The retained proposal-completion line is at +3,662.623 ms
([beacon.log](/home/sukun/dev/prysm2/runs/round2/prysm-geth-1/beacon.log:1136845)).
Thus all logged current-slot FFG validation entries and emissions fall before
proposal completion, including all of them before the requested +3,645 ms
cutoff. The observed gossip validation-entry interval from +30 through +3,489
ms lies inside the logged proposal-build interval from +8.736 through
+3,662.623 ms. The first observed gossip validation entry is therefore +21.264
ms after the build-start log, and the final one is +173.623 ms before the
finish.

This timing does not prove CPU contention, subscriber completion, pool
insertion, or proposal-pool membership. For gossip rows, `arrivedMs` is captured
at validation entry and the outer timestamp records ledger emission after the
work leading to that log line. For local rows, `arrivedMs` is computed in the
node's own submission/logging path. A focused concurrency experiment would
therefore test a real contemporaneous gossip-validation load pattern, but these
records alone do not attribute the 3.654-second observed build interval to
validation.

## Cumulative counts

Thresholds are inclusive and measured from the slot start. Because every row
has a distinct validator and `(validator, dataRoot, committee)` tuple, the row
and unique-validator counts are identical in every bucket. The gossip columns
are the validation-entry evidence; the all-ledger columns additionally include
local submissions.

| Offset | Gossip validation entries | All-ledger `arrivedMs` | Gossip emissions | All-ledger emissions |
| ---: | ---: | ---: | ---: | ---: |
| +25 ms | 0 | 0 | 0 | 0 |
| +50 ms | 20 | 20 | 13 | 13 |
| +100 ms | 102 | 102 | 102 | 102 |
| +200 ms | 708 | 741 | 487 | 520 |
| +500 ms | 2,428 | 2,499 | 1,791 | 1,862 |
| +1,000 ms | 6,021 | 6,092 | 4,330 | 4,401 |
| +3,645 ms | 14,355 | 14,426 | 14,355 | 14,426 |

The first row was validator 31,761, committee 2, data root
`0xaa7200a6ae8b4f09379a084ceaef99241ad51f620a3f77eed2d2ca61747adf`,
with `arrivedMs=30` and outer emission at +37.320 ms
([beacon.log](/home/sukun/dev/prysm2/runs/round2/prysm-geth-1/beacon.log:1121954)).

## Coverage and parse accounting

The 14,426 all-ledger rows contain 14,426 unique validators, 14,426 unique
`(validator, dataRoot, committee)` tuples, 12 data roots, and all six
committees. There are no duplicate tuple rows. Six majority root/committee
groups account for 14,354 rows; six minority groups account for the remaining
72. The gossip subset itself has 14,355 unique validators across the same 12
roots and six committees. The local subset has 71 unique validators across six
roots and all six committees; its `arrivedMs` range is +108 to +261 ms.

The source explicitly defines gossip FFG `arrivedMs` as validation entry in
[sync/vote_ledger.go](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/sync/vote_ledger.go:97).
The separate local line is emitted by
[LogLocalFFGVote](/home/sukun/dev/prysm2-round2-construction-028/decoupled/vote_ledger.go:19), which
computes the timestamp in the node's own submission path. For comparison, the
existing slot-96 artifact's 13,947 rows split into 13,871 gossip-validation and
76 local-submission rows; its report should be read as an all-ledger census
unless the outcome is stated.

The script scanned all 2,857,343 LF-delimited records in the extracted
1,336,022,480-byte log. It found and parsed 14,426 exact slot-97 FFG vote rows,
with zero missing required fields, malformed numeric/timestamp fields, or
unparsed candidates. The input SHA-256 is
`fc3836a9ebe0399ddb0e6e4fc23671d5e04d4edc665cd5253c89332edd4c2fbf`.

This is an observer-local census. Competing block/data roots and local versus
gossip outcomes are retained rather than collapsed, and the JSON includes the
first 20 rows by each clock plus every data-root/committee group's range.

## Reproduction

The slot-96 parser and JSON remain unchanged. The slot-97 copy is
[parse_node1_slot97_ffg_timing.py](parse_node1_slot97_ffg_timing.py), and its
machine-readable result is
[node1-slot97-ffg-timing.json](node1-slot97-ffg-timing.json).

```bash
python3 runs/diagnostics/round2-construction-bench/parse_node1_slot97_ffg_timing.py
```

The source log is the already extracted
`runs/round2/prysm-geth-1/beacon.log`; no archive decompression is required.
