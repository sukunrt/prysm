# Corrected source, receiver, and summary accounting

The earlier `startup3/early-gossip-results.md` statement that E1's BN3 FFG
summaries reported 15,000 accepted votes in each slot was incorrect. E1's
source accepted 15,000 RPC submissions in each of slots one, two, and three;
BN3's roughly +6-second summaries reported **2,336, 0, 0**. The report has
been corrected. This distinction strengthens the evidence of processing
backlog; it does not invalidate the matched proposal outcomes or the count
ablation.

`countFFGVote` runs at the end of real gossip validation, after the signature
and duplicate checks, immediately before `ValidationAccept`
(`beacon-chain/sync/validate_beacon_attestation.go:244`). Its counter is keyed
by the attestation's own slot and actual subnet. `runFFGVoteSummary` schedules
one read at `AggregateDueBPSGloas` (`ffg_summary.go:82`), which the retained
config sets to 5000 basis points: six seconds into a 12-second slot.

At that read, `take(slot)` returns only that slot's current counters and
deletes every bucket with key at or below it (`ffg_summary.go:55`). A vote
that completes after its summary tick can recreate the old slot's bucket;
the next slot's `take` deletes that older bucket without adding it to the new
slot's total. Thus late completions are not carried into another slot's
reported votes. The output sums only the subnets currently subscribed at
the read. These are accepted-completion counts by the actual summary read,
not offered counts, reception counts, or eventual per-slot acceptance totals.
The ticker may itself run late, so the log timestamp remains the observed
read-time bound. The same code was checked at E1's retained report revision
`a8d960ad2feb5e44426b6a5c1a221b255d0ea602`.

The following are directly read from retained BN3 logs. Raw lines, including
their original ANSI bytes, filenames, and line numbers, are copied into
[ffg-summary-acceptance-audit-anchors.log](ffg-summary-acceptance-audit-anchors.log).

| Run | Slot-one summary | Slot-two summary | Slot-three summary |
|---|---:|---:|---:|
| B | 1,993 | 0 | 207 |
| D: count ablation | 15,000 | 15,000 | 15,000 |
| E1 | 2,336 | 0 | 0 |
| F1: count ablation | 15,000 | 15,000 | 15,000 |
| H | 2,281 | 0 | 235 |
| I2: BN3-only count ablation | 15,000 | 15,000 | 15,000 |

The independent pubsub after-scrape metrics give the following aggregate
counts on the six beacon-attestation topics. The corresponding before files
have no populated series for these counters. Raw matching after lines and
their source locations are in
[ffg-receiver-metrics-audit-anchors.txt](ffg-receiver-metrics-audit-anchors.txt).

| Run | Pubsub validation-attempt events | Validation-throttled rejects | Pubsub deliveries |
|---|---:|---:|---:|
| B | 44,605 | 29,472 | 15,133 |
| D | 45,000 | 0 | 45,000 |
| E1 | 41,524 | 25,968 | 15,556 |
| F1 | 45,000 | 0 | 45,000 |
| H | 39,420 | 23,377 | 16,043 |
| I2 | 45,000 | 0 | 45,000 |

`p2p_pubsub_validate_total` counts the raw tracer's `ValidateMessage` callback,
not entry into Prysm's entire validation body. In the pinned libp2p source,
this callback precedes the asynchronous global/per-topic throttle checks
(`go-libp2p-pubsub@v0.17.0/validation.go:333,367,478`). Thus a throttled message
can increment validation-attempt metrics without executing the expensive
application validator. `p2p_pubsub_deliver_total` counts `DeliverMessage`,
after pubsub acceptance; it does not prove completion of Prysm's later pool
subscriber (`beacon-chain/p2p/pubsub_tracer.go:111–122`). Neither metric should
be relabeled as source RPC success or as the current fixture's
`ValidationStarted` wrapper counter.

In each displayed run, the validation-attempt total equals throttled rejects
plus deliveries at the retained scrape. E1's 3,476 and H's 5,580 differences
from 45,000 source submissions are before this observed validation counter;
the retained counters alone do not assign those differences to a network,
source, peer, or validation queue. The aggregate scrape also does not assign
deliveries to individual slots. The table describes the retained scrape, not
an asserted final lifetime total after every possible late task.

The sibling-document check found no equivalent false E1 acceptance transfer
in `startup3/reproduction-results.md` or `startup3/wire-causation-results.md`.
The former already distinguishes B's 45,000 source RPC successes from 44,605
attempts and 15,133 deliveries; its D claim of 15,000 completions per slot by
six seconds is supported by D's own summaries. The latter explicitly says
45,000 accepted **RPC submissions** for H/I2. F1's separate 45,000 validation
attempts and deliveries claim in `early-gossip-results.md` is supported by its
own metrics and is retained. These controls must not be substituted for E1.

The same audit also found a configuration distinction relevant to proposed
follow-ups. All four retained E1/F1/H/I2 bundle configs explicitly say
`SLOTS_PER_ROUND: 4`, `TARGET_COMMITTEE_SIZE: 2500`,
`ATTESTATION_SUBNET_COUNT: 6`, and `AGGREGATE_DUE_BPS_GLOAS: 5000`.
Exact source-file anchors are preserved in
[ffg-retained-config-audit-anchors.txt](ffg-retained-config-audit-anchors.txt).
Their 120,000-validator count yields 12 committees per slot; the six-subnet
modulo maps pairs of committees onto the same six topics in successive slots.
`startup3/ffgsource/main.go:243–279` loops over seat first and committee ID
second until it has selected 15,000 votes, skipping excluded VC validators.
It therefore samples across all 12 committees, roughly 1,250 voters from
each, rather than taking the first six complete committees. The current
round-eight fixture instead uses all 2,500 members of each of its six
committees. Equal 15,000-vote offers do not make these committee assignments
identical. An earlier proposed
18-topic explanation based on the current helper's round-eight/64-subnet
configuration is withdrawn as an E1 explanation. The historical round2
eight-slot-round behavior is established separately in
[round2-round-transition-audit.md](../startup3/round2-round-transition-audit.md);
this bounded check did not recover a historical round2 configuration file
establishing its total attestation-subnet setting.

Only documentation and bounded evidence excerpts changed. No new experiment,
profile capture, production-code edit, or test was performed by this audit.
