# Historical onset of slot-1 genesis-count work

## Result

The historical round did not wait for the ordinary +4-second attestation due
time. Node 169's validator startup explicitly enables
`decoupled-ffg-vote-at-slot-start` at `validator.log:3`; the historical source
then chooses slot-start jitter instead of `waitUntilAttestationDueOrValidBlock`.
The normal due-time premise is therefore inapplicable to this run.

The first retained successful slot-1/target-round-0 gossip record on observer
400 entered validation at **slot offset +38 ms** and logged acceptance at
**+57.085 ms** (`beacon.log:891`). Observer 201's earliest retained entry is
**+155 ms**, accepted at +252.760 ms (`:902`); its first acceptance is at
**+211.152 ms** for a record that entered at +158 ms (`:899`). These records
show that the count-bearing validation path completed within the first 58 ms on
one observed BN and within 212 ms on another.

This corrects the onset concern without moving the load before slot 1. Slot-0
FFG is rejected before target-state and committee-count validation, and the
first retained count-bearing successes are just after the slot-1 boundary.
The E1 synthetic load began 391 ms before that boundary, whereas these
historical samples did not. But a 38--57 ms historical onset can overlap almost
the entire slot-1 preflight/RANDAO window; it does not support a +4-second quiet
period. These different load start times mean E1 does not reproduce the exact
historical initial ordering.

## Entry and acceptance are separate bounds

`arrivedMs` is captured when gossip validation starts. Active-validator count
occurs later, after preliminary time, target-round, seen, block/state,
fork-choice, and target-state checks. The FFG line is emitted only after the
attestation passes the count-bearing topic check and the rest of validation.
Thus an entry timestamp does not prove that `ActiveValidatorCount` began at
that instant. An acceptance timestamp does prove that it completed before the
line was emitted.

| Observer | Retained successful slot-1/round-0 records | First retained entry | First acceptance | Entry / acceptance counts before +0.5 s | before +1 s | before +2 s |
|---|---:|---:|---:|---:|---:|---:|
| 201 | 4,332 | +155 ms | +211.152 ms | 45 / 13 | 146 / 17 | 282 / 17 |
| 400 | 3,672 | +38 ms | +57.085 ms | 22 / 22 | 73 / 43 | 157 / 105 |
| 500 | no ledger records | -- | -- | -- | -- | -- |

These are eventual-success cohort entries and successful log endpoints. Entry
counts are not simultaneous-running counts or CPU-occupancy measurements. The
ledger omits rejected, ignored, and still-unlogged validations, so the first
retained entry is not exact network ingress onset and cannot exclude earlier
work.

The cohort already has substantial delayed completion. Observer 201 line 2916
entered at slot offset +1.566 s and logged acceptance at genesis
`01:30:38.954321Z`, **25.388321 seconds after entry**. This gives an observed
early-entry tail while preserving the distinction between admission and
completion. Observer 400's first-second cohort is much less delayed at first:
43 of its 73 eventual-success entries before +1 second are logged accepted by
then. Later records establish the larger backlog summarized elsewhere.

Observer 500 did not emit the detailed vote ledger. Unlike observers 201 and
400, whose beacon startup logs explicitly enable `goldfish-vote-ledger` at
line 5, observer 500 has no such enablement record. Its zero rows mean no
instrumented evidence, not zero received FFG load.

## One-second bins

The full per-observer table is `ffg-count-onset-bins.tsv`. Bins count entry and
acceptance separately for the retained successful slot-1/round-0 cohort.

| Genesis interval | Observer 201 entry / accepted | Observer 400 entry / accepted |
|---|---:|---:|
| [+10, +11) s | 0 / 0 | 0 / 0 |
| [+11, +12) s | 0 / 0 | 0 / 0 |
| [+12, +13) s | 146 / 17 | 73 / 43 |
| [+13, +14) s | 136 / 0 | 84 / 62 |
| [+14, +15) s | 67 / 11 | 129 / 88 |
| [+15, +16) s | 184 / 5 | 220 / 26 |
| [+16, +17) s | 335 / 0 | 43 / 8 |

The generated TSV continues through `[+25,+26)` as requested. By the end of
slot 1, the strict `< +24 s` cohorts contain 1,326 entries / 157 acceptances on
observer 201 and 1,198 / 361 on observer 400, agreeing with the prior observer
audit.

## Owner-side captures

`slot1-owner-attestation-captures.tsv` extracts the first successful-call
capture from each owner VC's slot-1 `Submitted new attestations` summary. It has
13 of 16 owners; nodes 169, 191, and 107 have no such summary row. The earliest
are node 20 at +45 ms, node 19 at +176 ms, node 32 at +178 ms, node 35 at +232
ms, and node 76 at +260 ms. These are consistent with the enabled slot-start
jitter and the observer entry records.

The summary's displayed activity slot comes from the runner, while the
attestation grouping key omits the attestation data slot. Overlapping role work
can therefore be grouped under a later runner summary. The captures show
successful-call timing in those slot-1 summary groups; they are supporting
evidence rather than unique message identities joined to the observer votes.

Node 169's flag proves that its VC followed the slot-start branch. Its compact
BN log has no individual FFG ledger, and another observer's ingress does not
measure node 169's own inbound queue. Locally submitted FFG also follows a
different publication path from inbound gossip validation. The three-observer
onset establishes that count-bearing gossip existed immediately across the
historical network, while transfer to node 169 remains an inference.

## Reproduction and source boundary

`analyze_ffg_count_onset.py` sequentially scans only the already extracted full
beacon logs for observers 201, 400, and 500, then reads the existing owner role
table. It does not rescan the 1,000-node archive set. Run it with:

```text
python3 runs/diagnostics/round2-slots-0-16/analyze_ffg_count_onset.py
```

It generates:

- `ffg-count-onset-observers.tsv`: minima, subsecond counts, and an early-entry
  tail anchor;
- `ffg-count-onset-bins.tsv`: one-second entry/acceptance bins from genesis
  +10 through +25 seconds; and
- `slot1-owner-attestation-captures.tsv`: the 16-owner summary-capture census.

At historical revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`,
`validate_beacon_attestation.go:85--88` ignores slot 0 before the costly path,
`:143` gets the target state, `:253--290` reaches `ActiveValidatorCount`, and
`:243` emits the accepted FFG ledger after validation. `vote_ledger.go:64--96`
defines `arrivedMs` from the validation-entry timestamp. `attest.go:39--43`
selects the slot-start-jitter branch when the historical flag is enabled.
