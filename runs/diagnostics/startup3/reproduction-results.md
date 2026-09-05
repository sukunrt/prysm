# Three-node local reproduction results

## Setup and interpretation

Runs A, B and D used three real Prysm beacon nodes, three Geth execution nodes, one
596-key Prysm validator client on BN3, the same 120,000-validator genesis fixture
with a fresh genesis time for each run, four
slots per round, and `GOMAXPROCS=4` without a CPU quota. The synthetic source
submitted distinct valid FFG attestations to BN1 over gRPC during slots 1--3;
BN1 then broadcast them over peer gossip to BN2 and BN3.
Slot 0 has no proposal by protocol in this configuration, so it is an expected
genesis skip and is not counted as either success or failure.

The source result JSON says that a request was accepted by BN1's gRPC
RPC. It does **not** say that the attestation subsequently completed gossip
validation or was delivered to either peer. In particular, run B recorded
15,000 accepted source RPCs per slot. BN3 recorded 44,605 pubsub validation
attempts: 29,472 were rejected as validation-throttled and 15,133 passed
validation and were delivered. Neither source RPC success nor admission to
validation should be substituted for completed validation. The difference
between 45,000 source submissions and 44,605 receiver attempts is not assigned
to a specific cause here.

## A/B result

| Run | Offered votes per slot | Slot 1 proposal | Slot 2 proposal | Slot 3 proposal |
| --- | ---: | --- | --- | --- |
| A | 2,500 | published | published | published |
| B | 15,000 | published at +5.10 s | deadline | deadline |

The source accounting was exact in both runs: A accepted 2,500/2,500 on each
of slots 1--3, and B accepted 15,000/15,000 on each slot, with zero source
errors. The raw local artifacts were
`/tmp/prysm-startup3-round4-load-{a,b}/ffgsource-result.json`,
`validator3.log`, `beacon3.log`, and `profiles/`. They intentionally remain
outside the repository because the bundles contain validator secrets.

## Direct mechanism observed in B

BN3's 50-second CPU profile attributes 136.60 seconds of 152.22 sampled CPU
seconds (89.74% cumulative) to `helpers.ActiveValidatorCount`, beneath
`validateCommitteeIndexAndCount`. Thus the loaded run exercises the real
slot-0-checkpoint full-registry scan, rather than the earlier slot-1-state
microbenchmark fast path.

The goroutine profiles and diagnostic timings also close the parent-state
blocking chain. In-progress FFG validation enters `AttestationTargetState`, takes
the fork-choice read lock, and then serializes on the checkpoint-key async lock
even when the checkpoint-state cache is warm. The profiles contain thousands
of readers stopped at that checkpoint-key lock. A fork-choice writer waiting
for those existing readers causes Go's writer preference to stop subsequent
readers. Proposal parent lookup then blocks at `CachedHeadRoot`: the slot-3
request spent 17.223 seconds there, and the complete parent-state phase took
19.830 seconds. Update-head write-lock waits of 1.923 seconds and 1.450 seconds
were also recorded, while the post-acquisition work was only milliseconds.
This is a finite read-lock/checkpoint-key convoy, not a long fork-choice write
critical section and not a deadlock.

The ordering matters: each validation obtains its checkpoint state and releases
the fork-choice read lock **before** performing the full registry scan. Scans
from callers already through this stage consume CPU needed by later callers
to drain the serialized checkpoint queue. Those queued callers still hold
fork-choice read locks. A waiting writer must drain this cohort and prevents
new readers, including proposal `CachedHeadRoot`, from entering. This feedback
between CPU pressure and nested lock queues can exceed the proposal deadline;
the scan itself does not hold a fork-choice lock.

## Bounds on comparison with the historical failures

The historical six-slot terminal outcomes are recorded in
[recovered_proposer_logs.md](recovered_proposer_logs.md#six-slot-result). Four
of those six proposals reported RANDAO `DomainData` failure, but they are not
four equivalent observations. In round1 slot3 and round2 slots2--3,
synchronous `RolesAt` sync-committee preflight had already consumed the slot
context before proposer dispatch; the immediately failing RANDAO call was a
secondary symptom of that expired context. Only round2 slot1 lacks that prior
preflight failure, and its split between the VC-global domain lock and BN RPC
remains unresolved. Run B reproduced neither variant as a slow local RANDAO
request: it reproduced slot-2/3 proposal deadlines later in BN parent lookup.
Therefore the local run proves that the genesis FFG path has enough capacity
to block proposals through fork choice; it does not prove that the historical
preflight or RANDAO delays used the same lock chain.

The local topology also differs materially from the historical 1,000-machine
deployment, including CPU scheduling, peer fanout, network limits, and process
placement. Its 89.74% profile share is evidence about this loaded BN3 only and
must not be assigned as the historical CPU share. The VC domain-data cache and
its global miss lock remain a separate viable amplifier, documented in
[domain-cache.md](domain-cache.md).

## Diagnostic ablation switch

The tracked Kurtosis package accepts `genesis_count_ablation: true`. It sets
`PRYSM_DIAGNOSTIC_GENESIS_COUNT_ABLATION=1` on beacon nodes only and is
understood only by the explicitly experimental local BN image. The default is
false. This switch changes consensus validation behavior for diagnosis and is
not a production configuration or fix.

## Counterfactual control D

Run D repeated B's 15,000-source-RPC-per-slot load with only the diagnostic
genesis-count ablation enabled. All 45,000 source RPCs were accepted, all
15,000 FFG validations per slot completed before six seconds, and all three
blocks were produced at approximately +0.39, +0.09, and +0.10 seconds from
their slot starts. An attempted run C was invalid because vote preparation
finished too late and offered no load; it is excluded entirely.

The ablation reduced BN3's sampled CPU from 152.22 to 22.64 CPU-seconds over
the same 50-second window (85.13% lower). `ActiveValidatorCount` fell from
136.60 seconds (89.74%) to 0.09 seconds (0.40%), a 99.934% reduction in its
cumulative CPU time. Atomic reference-count work fell from 68.92 seconds
(45.28%) to 0.14 seconds (0.62%). Peak sampled goroutines fell from 6,476 to
379, and the peak sampled gossip-validation stacks fell from 6,144 to 55.

Proposal-side contention disappeared with the scan. Slots 1--3 completed
`GetBeaconBlock` in 190.600, 32.168, and 36.099 milliseconds. Their parent
state phases took 80.459, 2.385, and 3.100 milliseconds; initial
`CachedHeadRoot` waits were at most 0.001 milliseconds and instrumented
`UpdateHead` lock waits were at most 0.004 milliseconds. One periodic slot-2
update held fork choice for 45.295 milliseconds, which did not delay the
proposal. Removing the repeated slot-0 active-validator count scan eliminated
the CPU/reader convoy and missed proposals in this matched local control.
It remains an experimental
causality test, not a safe production remedy. A single matched comparison
strongly supports this mechanism; it does not establish that every possible
run without the scan must succeed.

End-to-end verification in D: BN2 imported slots 1, 2 and 3 and their execution
payloads. Slot 1 contained one beacon attestation and one payload attestation.
Its execution block contained zero transactions and used zero gas, as expected
without a transaction generator. All diagnostic enclaves were stopped after
capture and the normal diagnostic image tag was restored.
