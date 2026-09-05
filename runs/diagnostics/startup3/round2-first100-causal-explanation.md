# Round 2, slots 0–100: causal explanation

## Complete outcome accounting

This investigation excludes the collapse after slot 128. The next slot's parent
is used only to determine whether slot 100 survived its own boundary.

| Outcome | Slots |
| --- | --- |
| Genesis | 0 |
| No recorded block anywhere in the 1,000-node archive union | 1–14 |
| Produced, then bypassed by the continuing branch | 16–20, 34, 39, 45, 51, 65, 68, 70, 74–75, 78, 81–82, 97, 100 |
| Retained on that branch | 15, 21–33, 35–38, 40–44, 46–50, 52–64, 66–67, 69, 71–73, 76–77, 79–80, 83–96, 98–99 |

The three non-genesis rows partition slots 1–100 into **14 unproduced, 19
bypassed, and 67 retained**. This is not a claim about final canonical history
after the excluded collapse. The [101-row timeline](round2-slots-0-100.md)
contains each proposer, construction stages, earliest import, submission, and
reorg observations. [Full parent-root evidence](round2-first100-parent-lineage.md)
independently establishes bypasses; a reorg log alone does not.

## Startup: the underlying work and why cheap requests time out

FFG gossip for target round 0 uses a checkpoint state at slot 0. The historical
`ActiveValidatorCount` fast path explicitly refuses a cached count for that
state, so each admitted call scans all 120,000 validators. This is O(messages ×
registry), effectively quadratic in validator count at fixed round length.
The registry scan is not the attestation packer's deduplication loop.

The real three-node/120k-registry experiment independently established the
feedback: scan-dominated CPU, thousands of gossip goroutines, checkpoint-key
read-lock queues delaying fork-choice access, and runnable HTTP readers left
unscheduled after responses reached the kernel. Removing only the repeated
count scan on the receiving/proposing node removed those proposal failures.
[Wire/runtime control](wire-causation-results.md),
[checkpoint-lock control](reproduction-results.md).

The historical logs independently show substantial local servicing delay:
node 400 has 1,198 eventual-success slot-1 FFG validations entered before slot
end but only 361 acceptance logs by then; node 201 has 1,326 versus 157.
Their cohort median entry-to-log delays are 17.807 s and 16.454 s. These are
observer-local validation/log intervals, not complete ingress or proposer-pool
counts. They support real historical backlog without claiming the synthetic
15,000-candidate packing fixture was an observed historical pool.
[Historical cohort reconstruction](round2-observer-timeline.md).

This is the common pressure mechanism, not a claim that all historical
proposals blocked on the same mutex. The source distinguishes the affected
paths below.

| Slot(s) | Direct observed failure | Why this path is vulnerable |
| --- | --- | --- |
| 1 | Node 169's RANDAO domain lookup fails before requesting a block. | The handler itself is constant-sized and takes no head/state/EL lock. The cold VC lookup can wait on a global domain-cache mutex or on RPC/goroutine servicing. The effective cache retained only 3 of 13 domain entries in the real configuration, while detached signing work generates concurrent misses. Historical traces do not identify which wait dominated. |
| 2, 3, 7, 11, 12 | Synchronous sync-index preflight consumes the proposal context before dispatch. | `RolesAt` puts sync preflight ahead of the proposer. The BN's sync-index path encounters the shared async lock manager, and on a miss additionally head-state copying and slot processing. A delay in this separate duty therefore prevents proposal execution. |
| 4 | Sync-selection signing/domain preflight consumes the context. | Same synchronous preflight ordering, but the observed failing operation is signing/domain lookup, not sync-index lookup. |
| 5, 8 | Late block-handler entry, then a payload-read timeout. | Entry is only at +10.120/+11.191 s. The proxy nevertheless records successful payload responses in 3 ms/under 1 ms; the timeouts are logged about 1.870/0.810 s afterward. EL construction is not the long operation. |
| 6, 9 | Payload read fails even before the whole-slot deadline. | The call has its own 300 ms timeout. Geth builds empty payloads in under 1 ms; proxy responses take 2/22 ms. The local wire/runtime control proves how runnable-reader starvation can expire this shorter budget despite prompt delivery. |
| 10, 14 | Payload succeeds; the required parallel consensus/packing branch remains outstanding past the deadline. | The packer has substantial superlinear work and long regions without cancellation checks. The new full-pipeline control below reproduces tens-of-seconds delayed cancellation when composed with the registry-scan workload. |
| 13 | Parent-state preparation expires before any payload request. | The wrapper identifies slot-processing cancellation, which can arise from an in-progress skip-cache wait or transition work. The combined head/state region takes 10.721 s, whereas the retained 120k-genesis single transition to slot 13 takes about 64 ms with an uninitialized Merkle tree on this host. The baseline points to interference from concurrent work, not merely having 13 empty slots. |

For slots 5/8/6/9, historical proxy timestamps are not packet captures at the BN.
The exact division between delivery, transport, scheduling, and logging is not
recorded. The controlled packet experiment establishes scheduler starvation
locally; transferring its precise mediation to each historical owner remains
an inference. A short handler does not imply a short end-to-end RPC.

[Node 169 deep audit](round2-slot1-cause-deep-audit.md),
[state, preflight, and payload audit](round2-state-payload-deep-audit.md).

## Why packing/processing could continue for tens of seconds

The new diagnostic exercises the real default pool and complete production
`packAttestations`, including filtering, deletion, deduplication, aggregation,
limiting, and BLS verification. The fixture is synthetic and production-valid,
not a replay of a historical owner's unlogged pool.

| Controlled measurement | Result |
| --- | --- |
| 15,000 valid singles in six full committee/data groups, no competing scan load | 5.43–5.70 s; about 432 MB allocated. |
| Same full pack with 300 ms/1 s deadline | Returns after 5.32/5.35 s, observing cancellation several seconds late. |
| 15,000 count jobs/6,144 workers, cached-count control, same pack, 12 s deadline | 5.306 s; independent repeat 5.274 s; six attestations selected; success. |
| Same job count, workers and pack, slot-zero count scans | 61.292 s; independent repeat 59.246 s; deadline exceeded; no selected attestations. |

The timed pack-only CPU profile attributes 55.8% cumulative time to dedup and
37.8% to aggregation. `Bitlist.Contains` alone accounts for 48.5% flat CPU.
These numbers explain why the earlier dedup-only benchmark was incomplete.

The last pair changes the count behavior, not the offered work or pack input.
There are no sleeps, artificial lock holders, or infinite jobs. All 15,000 jobs
complete and report 120,000 validators. Thus these production paths demonstrably
compose into a delay larger than the historical 29–41 s return intervals.
The independent repeat passed with the same output and cancellation assertions.

This establishes a concrete cause-and-effect mechanism. It does not establish
the historical node 35/node 85 pool size, exact number of concurrent scans, or
which internal substage consumed each second.
[Fixtures, CPU profile, cancellation tests, and commands](round2-packing-deep-audit.md).

## Why startup recovery is gradual

From slot 8, newly generated round 1 votes use a nonzero-slot checkpoint state
and can use the cached count. Old round 0 votes remain gossip-age-valid and
can continue creating expensive work; there is no hard cutoff at 8 or 16.
Detailed observers improve before the first block at 15. Different nodes
recover at different times, so the first successful proposer does not imply
that the whole network has recovered.

## Why blocks inside slots 15–100 still disappear

The proposal RPC has a 12 s deadline, but available votes are due at **3 s**, or
earlier on timely head notification. The old-root vote cohorts and next-boundary
bypasses match Goldfish's strict-majority gating of the previous slot's
continuation. Its full selection rule also applies ancestry and
checkpoint-viability checks; majority support alone is not a universal retention
guarantee. A late block can be imported broadly yet fail this gate. Accepted
gossip records precede subscriber insertion, so those records alone do not
reconstruct each deciding node's retained electorate or rejection branch.

The 3 s point is the scheduled vote wakeup, not a block-validity cutoff or a
timestamp proving every actual data request ran then. Scheduling can delay a
request: the minority voting for the new blocks at 16/17 imports just after
3 s. The recorded vote roots, owner-local import ordering where available,
and subsequent parent links establish the outcome; the timer alone does not.

For 13 later bypassed blocks, the complete owner timeline shows prompt build
entry and payload selection, followed by 2.542–7.763 s between payload selection
and construction completion. This locates their dominant wall time in joined
consensus construction/state-root work, not payload retrieval or an injected
publication delay. It does not assign that entire interval specifically to
packing CPU.

For 51 and 82, first imports elsewhere are before 3 s, but the **actual committee
owners'** imports are not:

- Slot 51: node 74 owns 468 seats and imports only at +3.774 s. All 468 votes
  name slot 50. Node 75 owns 44 seats, imports at +1.476 s, and votes 51.
  This observed 44/512 split would not pass the >256 gate if retained in full;
  the precise deciding-store snapshot is unlogged. Slot 52 builds on 50.
- Slot 82: node 1 owns 237 seats and imports at +3.901 s; node 194 owns 275 and
  imports at +3.654 s. All 512 votes name 80. Node 1's detailed local-vote
  records additionally prove its 237 votes were inserted before its block 82
  import: local submissions begin at +3.352 s. Slot 83 builds on 80.

The startup minority splits have the same owner-local explanation: most
slot 15 seats belong to node 69, whose import is +12.466 s; most slot 16 seats
belong to node 93, import +5.069 s; most slot 17 seats belong to node 145,
import +5.640 s. The mock committee's contiguous index selection concentrates
hundreds of votes in a single wallet, and one cached data response drives
that wallet's votes.

These timings are **local import completion**, not wire-arrival measurements.
The recorded vote splits and subsequent parent roots establish the retention
failure; the historical records do not uniquely divide each committee owner's
import lag between gossip propagation and local processing.
[Activation-derived ownership and vote evidence](round2-first100-vote-retention.md).

## Verification and remaining boundary

All 1,000 archives are covered. The slot table has 101 rows. The parent links
agree across three independent detailed snoopers. Diagnostic code was authored
by GPT-5.6 Sol and checked with Go tests/benchmarks and Python parser runs,
not Bazel. No production fix is applied; this follow-up is in jj change
`nkxqltrw`.

The remaining unknowns are narrower than an unexplained timeout: the exact
historical node 169 goroutine/mutex/RPC wait split, owner-specific pool/runtime
profiles, and the wire-versus-local-processing split of committee import lag.
The archives do not record those distinctions. Controlled tests establish the
mechanisms and their sufficiency, not missing historical trace events.
