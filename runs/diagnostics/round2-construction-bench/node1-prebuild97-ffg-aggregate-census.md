# Node-1 FFG ledger census before proposal 97

This bounded census reads the retained node-1 `beacon.log` prefix through LF
record 1,121,950. The stopping record is the proposal-97 build start:

```
2026-09-05T01:49:24.008736237Z ... Building block sinceSlotStartTime=8.492268ms slot=97
```

The parser observed 659 aggregate candidates in attestation slots 64-96 and
parsed all 659. Every parsed validator CSV had exactly the logged `seats`
count. It also parsed all 431,554 selected `FFG vote` candidates. There were no
selected parse failures or aggregate seat mismatches. This explicit census is
important because long validator CSV fields could otherwise be mistaken for
complete records after log splitting.

## Proposal-relevant slots 88-96

The eligible-window ledger contains 137 aggregate emissions in 64
`(attSlot, dataRoot, committeeIndex)` groups. These are 91 distinct validator
sets: 124 emissions were gossip outcomes and 13 were local outcomes. A single
observed set covers every other observed set in each of the 64 groups. This
does **not** mean every pair of earlier sets is nested; overlapping or
incomparable partial sets can both be subsets of the later covering set. The
largest covering set in every group has a gossip emission, so the all-ledger
and gossip-only coverage results are identical.

Exact validator membership strengthens the earlier cardinality-only bound:

| Slot | Aggregate groups | Emissions | Distinct sets | Singles in largest sets | Single groups without aggregate | Singles without aggregate |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 88 | 6 | 12 | 8 | 13,294 | 6 | 78 |
| 89 | 8 | 14 | 12 | 13,943 | 4 | 57 |
| 90 | 7 | 12 | 11 | 13,392 | 5 | 50 |
| 91 | 6 | 13 | 7 | 11,930 | 5 | 67 |
| 92 | 6 | 20 | 8 | 11,891 | 5 | 23 |
| 93 | 6 | 20 | 11 | 9,979 | 3 | 32 |
| 94 | 6 | 18 | 11 | 7,277 | 3 | 38 |
| 95 | 11 | 16 | 13 | 7,495 | 0 | 0 |
| 96 | 8 | 12 | 10 | 13,925 | 2 | 22 |
| **Total** | **64** | **137** | **91** | **103,126** | **33** | **367** |

All 103,126 distinct single-vote validators in groups with an aggregate occur
in that group's largest logged gossip aggregate. The 367 exact uncovered
single validators are in 33 data groups with no logged aggregate at all. Thus
the eligible ledger has 103,493 distinct `(validator, attSlot, dataRoot,
committee)` single keys, of which 367 are absent from the logged largest
aggregate sets. Slot 96 accounts for 22 of the 367; this replaces the earlier
cardinality-only upper bound of 123 with exact logged-set membership.

This is still a ledger relationship, not a reconstructed pool snapshot.
Aggregate validation/emission does not prove subscriber insertion or presence
when the proposer read the pool. The result therefore bounds logged coverage;
it does not prove proposal 97 saw these covering objects.

## Older rows and artifacts

Slots 64-87 are reported separately because they are outside the selected
round-11/12 proposal window. They contain 522 aggregate emissions, 223 groups,
and 364 distinct validator sets. These rows were not used in the eligible
coverage total.

The compact [census JSON](node1-prebuild97-ffg-aggregate-census.json) retains
full block-root families, set fingerprints, outcomes, source LF records, set
relations, and per-group coverage. The compressed
`node1-prebuild97-ffg-aggregate-validator-sets.json.gz` retains the exact sorted
validator IDs for every one of the 91 eligible sets and joins by SHA-256
fingerprint. Reproduce both artifacts from the repository root with:

```bash
python3 runs/diagnostics/round2-construction-bench/parse_node1_prebuild97_ffg_aggregates.py
```

