# Round 2 observer timeline through slot 20

This report streams the detailed beacon logs from observer nodes 1, 50, 150,
201, and 400. Observer counts describe only that observer; they are not assigned
to a proposer. “Goldfish” is the available-attestation fork-choice ledger, not
the FFG gossip stream. An accepted ledger line is a validation decision, not by
itself proof that fork choice consumed the vote.

Genesis was 01:30:00 UTC and slots were 12 seconds. Across all five logs there
is no `Synced new block` for slots 1 through 14, nor any sync/blockchain block
line carrying one of those slot numbers. The first visible imported block is
slot 15.

## Block imports

| observer | slot 15 import | slot 16 | slot 17 | slot 18 | slot 19 | slot 20 |
| --- | --- | --- | --- | --- | --- | --- |
| 400 | 01:33:01.732, line 27512 | 01:33:14.059 | 01:33:26.686 | 01:33:46.050 | 01:34:00.562 | 01:34:05.754 |
| 201 | 01:33:01.885, line 28126 | 01:33:14.173 | 01:33:26.792 | 01:33:46.123 | 01:34:01.145 | 01:34:05.843 |
| 150 | 01:33:06.808, line 25558 | 01:33:27.152 | 01:33:32.049 | 01:33:50.429 | 01:34:01.098 | 01:34:06.552 |
| 50 | 01:33:07.653, line 29535 | 01:33:15.921 | 01:33:28.633 | 01:33:46.665 | 01:34:00.579 | 01:34:05.744 |
| 1 | 01:33:25.112, line 18160 | 01:33:29.733 | 01:33:30.304 | 01:34:11.180 | 01:34:07.768 | 01:34:08.705 |

Node 400's slot 15 import is followed by “Finished applying state transition”
at line 27513 and its execution-payload envelope at lines 27521-27522. Its
parent execution hash is the genesis payload hash. Node 1 observes the same
chain much later and out of slot order, so its timestamps demonstrate observer
backlog rather than later block production.

These info-level logs do not provide separate gossip-receipt and validation
begin/end markers for blocks. `Synced new block` is therefore the earliest
observable successful block-processing point, not a measured network-arrival
timestamp.

## Available-attestation ledger

The table shows `accepted/dropped` decisions and maximum `decidedMs-arrivedMs`
for selected vote slots. Missing vote slots have no Goldfish ledger line in that
observer. Slot 15 `queued` and `replayed` lines are lifecycle records for the
same deferred votes and are not added as unique votes.

| observer | vote slot 1 | slot 12 | slot 13 | slot 14 | slot 15 |
| --- | --- | --- | --- | --- | --- |
| 400 | 34/291, max lag 16,713 ms | 19/0, 10 ms | 358/0, 10 ms | 512/0, 20 ms | 512/0, 22 ms |
| 201 | 116/211, 19,493 ms | 19/0, 38 ms | 358/0, 25 ms | 512/0, 60 ms | 512/0, 58 ms |
| 150 | 2/260, 4,770 ms | 3/16, 15,654 ms | 0/358, 1 ms | 512/0, 13,780 ms | 362/96, 12,814 ms |
| 50 | 23/100, 20,597 ms | 19/0, 10,431 ms | 358/0, 1,923 ms | 512/0, 9,464 ms | 458/0, 6,804 ms |
| 1 | 2/144, 29,223 ms | 19/0, 34,941 ms | 2/356, 22,588 ms | 0/512, 6 ms | 458/0, 18,692 ms |

Node 400 gives the clearest early example: a vote-slot-1 message arriving at
4,186 ms was not decided until 10,284 ms (line 1135), and later slot-1 messages
were still being dropped as stale after 12 seconds (lines 1257 onward). Its
slot-1 ledger extends to `decidedMs=30357`. Yet by vote slots 12-14, node 400's
maximum decision lag is only 10-20 ms. Nodes 201 and 400 therefore show that the
early queue/decision cliff had cleared before the first block arrived; nodes 1,
50, and 150 retained substantial observer-specific lag.

The last vote-slot-1 `decidedMs` values were 30,357 (node 400), 29,244 (201),
46,188 (50), 69,706 (150), and 139,638 (node 1). Because these are offsets from
the vote's slot start, node 1 logged a slot-1 decision around 01:32:31.638,
during slot 12. This does **not** by itself prove observer-local backlog: when
`arrivedMs` is similarly large, it is a late entry from the network or a peer
queue and has little local decision lag. Only `decidedMs-arrivedMs` bounds the
interval visible inside this ledger path.

## FFG single-vote validation cohorts

For `sync: FFG vote outcome=gossip` lines, the parser derives validation-entry
time as `genesis + attSlot*12s + arrivedMs`; the precise outer timestamp is the
logging-completion upper bound. The inner timestamp is retained only as a
centisecond-quantized estimate. This excludes aggregates and local submissions.
`arrivedMs` is instrumentation at validation entry, not a packet-capture network
arrival. Completion can also include logger and container-log collection delay.
Both “by slot end” tests use a strict `< slotEnd` comparison, and the parser
rejects any summary where completions exceed entries for that cohort.

| observer / attSlot | logged | entered by own slot end | completed by own slot end | lag median / p95 / max |
| --- | ---: | ---: | ---: | ---: |
| 400 / 1 | 3,672 | 1,198 | 361 | 17,807 / 25,166 / 28,858 ms |
| 400 / 8 | 516 | 23 | 14 | 384 / 14,802 / 21,211 ms |
| 400 / 9 | 743 | 35 | 0 | 14 / 9,596 / 14,972 ms |
| 400 / 10 | 1,436 | 239 | 239 | 9 / 1,192 / 5,733 ms |
| 400 / 11 | 1,506 | 780 | 780 | 9 / 21 / 794 ms |
| 400 / 12 | 2,023 | 1,449 | 1,449 | 8 / 12 / 25 ms |
| 400 / 13 | 3,076 | 2,713 | 2,713 | 8 / 11 / 23 ms |
| 400 / 14 | 3,994 | 3,822 | 3,819 | 8 / 14 / 28 ms |
| 400 / 15 | 4,680 | 4,479 | 4,479 | 8 / 15 / 50 ms |
| 201 / 1 | 4,332 | 1,326 | 157 | 16,454 / 23,734 / 25,388 ms |
| 201 / 8 | 526 | 20 | 0 | 1,363 / 16,804 / 17,835 ms |
| 201 / 9 | 777 | 42 | 0 | 1,249 / 9,312 / 12,988 ms |
| 201 / 10 | 1,490 | 209 | 209 | 991 / 2,748 / 8,688 ms |
| 201 / 11 | 1,456 | 720 | 720 | 12 / 1,007 / 2,170 ms |
| 201 / 12 | 2,103 | 1,497 | 1,497 | 13 / 27 / 58 ms |
| 201 / 13 | 3,084 | 2,704 | 2,704 | 11 / 31 / 77 ms |
| 201 / 14 | 3,986 | 3,823 | 3,818 | 13 / 46 / 95 ms |
| 201 / 15 | 4,675 | 4,512 | 4,512 | 16 / 44 / 87 ms |

The five largest node-400 lags are anchored at lines 3260, 3228, 3125, 3337,
and 3343 (27,321-28,858 ms, all attSlot 1). Node 201's are lines 2916, 6305,
1816, 1794, and 1793 (24,734-25,388 ms, all attSlot 1). Several node-201 votes
entered in the first 1.6 seconds of slot 1 and completed in wall slot 3, directly
proving a long observer-local validation/log-completion cohort rather than only
late network entry.

Target-round-1 traffic was not withheld until wall slot 12. Node 400 first logs
it for attSlot 8 in wall slot 8 (line 9000, 75 ms entry-to-log lag); node 201
first logs attSlot 8/target round 1 in wall slot 9 (line 9298, 9,606 ms lag).
Old target-round-0 votes continue to appear much later, but their very large
`arrivedMs` and tiny entry-to-log lag identify late admission, not continued
local validation: node 400's last is attSlot 7 in wall slot 15, 11 ms lag (line
30001), and node 201's is attSlot 2 in wall slot 16, 29 ms lag (line 35813).
These individual records completed quickly after late entry, but they still
entered the old target-round-0 validation path.

## What changed before slot 15

The first block did not clear the earliest observers' vote backlog: on nodes
400 and 201, decision latency was already tens of milliseconds during vote slots
12-14, before the slot-15 block import. Target-round-1 FFG begins at attSlot 8,
and its validation lag falls sharply but progressively through slots 8-12; it
does not wait for slot 12 to be admitted. This is consistent with the diagnosed
expensive path being tied to the round-0/genesis target state: later-round
target-state preparation is no longer forced to perform the same slot-zero
registry scan for every vote.

This transition is not a hard global cutoff. The post-Deneb gossip age window
is epoch-based, so old target-round-0 votes remain admissible well beyond the
round transition and can continue to enter the expensive path. The data shows a
changing mixture: cheap later-round validation becomes dominant while some old
round-0 work persists. It does not show that all expensive work ended at slot 8.

That is a source-supported explanation for the load cliff, not proof from these
ledger lines alone. The logs do prove three narrower facts:

1. No valid/imported block for slots 1-14 appears at any detailed observer.
2. Some observers drained the early available-vote decision backlog before slot
   15, while others remained delayed.
3. Slot 15 was produced before 01:33:01.732 and propagated unevenly; it was not
   a late validation of a slot-1-through-14 block.

The omitted round-2 FFG summary prevents reconstruction of complete FFG ingress
or completion totals. Individual FFG lines can establish observed samples but
must not be summed as owner load or confused with the Goldfish ledger above.

## Reproduction

`parse-round2-observers.py` reads each file sequentially and emits block-import
records and per-vote-slot Goldfish outcome/lag statistics:

```bash
python3 runs/diagnostics/startup3/parse-round2-observers.py \
  runs/round2/prysm-geth-{1,50,150,201,400}/beacon.log
```
