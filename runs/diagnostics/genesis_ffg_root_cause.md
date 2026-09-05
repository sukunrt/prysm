# Genesis FFG validation amplification: slots 1–3

No production fix applied. New diagnostic-only jj change: `lqxvxsvy`, child
of `novuklnx`. GPT-5.6 Sol authored the diagnostic Go code. These are local
unit tests and microbenchmarks, not another network simulation.

## The concrete defect

Both historical revisions (`a1679c9f` round1 and `0280403c` round2) have this
path for each FFG single that reaches topic/committee validation:

```
validateCommitteeIndexBeaconAttestation
  -> AttestationTargetState(target round 0) -> checkpoint state at SLOT 0
  -> validateUnaggregatedAttTopic
  -> validateCommitteeIndexAndCount
  -> ActiveValidatorCount
       cached count is ignored because state.Slot() == 0
       -> scan every one of the 120,000 validators
```

Source: `beacon-chain/sync/validate_beacon_attestation.go:149,251,292`;
`beacon-chain/core/helpers/validators.go:145–181`.
The relevant condition is `activeCount != 0 && s.Slot() != 0`.
The helper does not merely have a cold cache miss: even an already-populated
committee cache leads to the complete registry scan on every call.

The state slot is **not the attestation slot or wall-clock slot**. Round-0
attestations in slots 1, 2, and 3 all use the genesis checkpoint state.
`getRecentPreState` explicitly declines its fast path for checkpoint round 0;
`getAttPreState` obtains/caches state at `RoundStart(0) == 0`. An advanced
next-slot-cache state cannot satisfy a request for slot 0. Producing a slot-1
block does not turn this cached checkpoint state into a slot-1 state.
See `beacon-chain/blockchain/process_attestation_helpers.go:23,104` and
`beacon-chain/core/transition/trailing_slot_state_cache.go:41,59`.

This is O(M × N): M FFG votes reaching this check, N registered validators.
The protocol's offered set is 15,000 FFG duties per slot. If a node validates
that full set against 120,000 validators, it costs **1.8 billion validator
visits per slot**; the logs do not prove every node received the full set.
Since M grows with N at
fixed round length, this is effectively quadratic scaling in validator count.
It is in gossip validation, **not proposer attestation packing**.

## Why it consumes capacity and interferes with proposals

`ValidatorsReadOnlySeq` holds the checkpoint state's read lock across the
whole scan. Every validator visit additionally calls `validatorsMultiValue.At`,
which acquires/releases the same registry-storage RWMutex and performs a map
lookup. Copies derived from the same state share that registry storage;
copying such a state takes its write lock. Consequently concurrent scans
compete over a shared atomic lock counter,
CPU, and memory, and interact with state-copy work needed by local RPCs and
proposal preparation. This is not a permanent fork-choice deadlock.

Source: `beacon-chain/state/state-native/getters_validator.go:227`;
`container/multi-value-slice/multi_value_slice.go:180,255`;
`beacon-chain/state/state-native/state_trie.go:1049`.

The offered gossip load can exceed validation throughput, leaving many votes
in validation while accepted-vote/pool summaries stay small. Validator role
selection synchronously calls local beacon RPCs before dispatching proposals;
the proposal handler then synchronously prepares its head/parent state before
packing or fetching the payload. CPU/scheduling and shared-state contention
therefore affect both pre-dispatch and server-side work. Original logs do not
measure each individual lock wait; the excessive scan work itself is
deterministic and reproduced, rather than inferred from those gaps alone.

## Diagnostic proof and measurement

`TestGossipStartupActiveValidatorCountSlotZeroCacheBypass` calls production
committee/count validation with Electra singles for attestation slots 1, 2,
and 3. A wrapper counts actual registry traversal calls and yielded entries.
Both cases have a warm committee cache:

| Target state's slot | Three attestations | Registry entries visited |
|---|---:|---:|
| 0, the real startup condition | 3 full traversals | 360,000 |
| 1, the earlier benchmark's incorrect condition | 0 traversals | 0 |

The old fixture explicitly called `SetSlot(1)` and the mocked target-state
fetcher returned that state. This invalidated the old benchmark as a control
for this particular startup path. The 120k quiet-network control likewise
did not exercise the large incoming FFG gossip load.

Corrected full-path comparison, same machine, GOMAXPROCS=4, 64 workers,
1,000 valid encoded singles, one iteration each:

| Target state | Wall time for 1,000 accepted votes | Allocated |
|---|---:|---:|
| Slot 0 | 2.8077 s | 37.30 MB |
| Slot 1 | 79.68 ms | 36.42 MB |

This is approximately **35× slower** solely from changing the target state's
slot; production algorithms are unchanged. Both variants pass acceptance,
subscriber, and notification count assertions. The benchmark uses production
decode, validation, signature verifier, database presence checks, operation
feed, and pool insertion; fork-choice answers remain mocked and networking
is absent. It measures the bottleneck, not an exact replay of all missed
proposer requests.

The CPU profile includes both variants and setup: 79.94% cumulative samples
are in `ActiveValidatorCount`, 65.79% in registry `At`, and 44.20% flat in
atomic integer additions used by locks. These cumulative figures overlap
and must not be added. Profile: `/tmp/genesis-ffg-slot0-full-cpu.pprof`;
binary: `/tmp/prysm-genesis-ffg-sync.test`.

The isolated helper, 512 iterations, GOMAXPROCS=4:

- Serial slot 0: 2.689 ms/op; slot 1: 241.9 ns/op.
- Parallel slot 0: 2.539 ms/op; slot 1: 192.2 ns/op.
- Parallel slot 0 at eight processors: 2.472 ms/op; extra processors barely
  improve aggregate throughput because the scans share registry locks.

A separate bounded scheduling/state-copy probe releases 1,024 concurrent
production count calls and a ready copy task, recording dispatch delay
separately from time inside `Copy`. At GOMAXPROCS=4, one iteration:

| Target state | All 1,024 calls finish | Copy-task dispatch delay | Inside Copy |
|---|---:|---:|---:|
| Slot 0 | 2.197 s | 602.66 ms | 71.62 microseconds |
| Slot 1 | 0.329 ms | 0.122 ms | 38.33 microseconds |

The large observed delay here is **scheduling**, not one long copy lock hold.
This directly demonstrates interference with ready unrelated state work,
but is not a reproduction of the whole proposal RPC or its original 11-second
gap. Dispatch order and timing vary; these are observed measurements, not
guaranteed lower bounds. Benchmark:
`BenchmarkGossipStartupValidatorScanCopyContention`.

Parallel ns/op is aggregate **wall time divided by completed operations**,
not individual request latency or CPU time. At measured full-path throughput,
15k messages would take roughly 42 seconds; this is an extrapolation on this
machine, not a measured historical wall time. The actual full-path test above
uses 1k messages only. The historical slot budget was 12 seconds.

## Link to the saved failures in both rounds

Round1 node3's saved slot-1 proposal establishes the terminal causal chain:

| UTC | What happened |
|---|---|
| 00:00:12.114–12.115 | Geth prepared a payload in 547 microseconds. |
| 00:00:18.01 | 38 FFG gossip validations completed: at least 4.56 million registry visits already performed. |
| 00:00:18.973 | Proposal handler entered, 6.973 seconds into the slot. |
| 00:00:24 | Validator canceled the block request at the slot deadline. |
| 00:00:30.10 | Beacon finally reached graffiti, still before packing. |
| 00:00:31.52 | Payload preparation failed on the already-canceled context. |

Evidence: `runs/round1/prysm-geth-3/beacon.log:900,902,906,910–911`,
`validator.log:687`, `execution.log:98–99`. No getPayload request reached the
execution snooper during slots 0–3. Thus this was failure to complete local
proposal preparation before its deadline, not slow payload execution or
publication of a block that subsequently got rejected.

The sampled receiver logs independently show heavy validation backlog in
both rounds. For example, round2 node1 has 8,384 slot-1 **gossip** completion
records emitted at least 24 seconds after that slot started. Each passed the
full scan: at least **1,006,080,000 registry visits** for those records alone.
These are not all completed inside slot 1 and are not all simultaneous pool
entries. The record classifications and local/gossip distinction are in
`ffg_slot1_windows.md`. Hundreds of local attestation-data RPC deadline
failures in slots 2–3 are consistent with this continuing backlog.

Goldfish summaries do not count pending FFG validation. Node3's slot-1
Goldfish summary at 24.22 had zero inserted Goldfish votes, eliminating an
available-vote write storm as the explanation of the initial blockage, but
not the FFG scan storm. Independent beacon timers and execution activity
continued, also consistent with overloaded paths rather than a continuous
whole-process pause.

Only round1 node3's slot-1 proposer is in the saved early proposer logs.
The other five proposer call histories cannot be reconstructed individually.
The source defect and its deterministic applicability to both rounds are
established; exact shares of each historical proposal delay attributable to
scheduling, state locks, other services, and hardware are not recorded.

## Fix scope for later approval

Use service-local memoization of the active count for an
**authenticated immutable checkpoint state**,
keyed by checkpoint root and the epoch whose active set is being counted.
Do the registry scan once per such state, not once per vote. Keep cache
eviction bounded. Reuse that result for gossip topic/committee validation.

Do **not** blindly delete the slot-0 guard globally: pregenesis state
construction can mutate the registry without changing the attester seed.
`TestActiveValidatorCount_Genesis` explicitly tests a stale seed-keyed count
against a mutable slot-0 registry. Preserve that correctness property.
Changing only packing, bandwidth, or slot deadlines does not remove this bug.

Slot-0 FFG gossip is ignored before this validation stage; slot 1 supplies the
first real burst into the pathological path. The historical round length is
eight slots. New round-1 attestations can use
a nonzero checkpoint prestate starting at slot 8, avoiding this guard, but
delayed round-0 messages can keep scanning until their backlog drains.
Do not claim recovery occurs immediately after slot 3 or the first block.

## Verification commands

```sh
GOMAXPROCS=4 GOCACHE=/tmp/prysm-diagnostic-buildcache go test -tags=develop \
  ./beacon-chain/sync \
  -run '^TestGossipStartupActiveValidatorCountSlotZeroCacheBypass$' \
  -bench '^BenchmarkGossipStartupActiveValidatorCountSlotCache$' \
  -benchtime=512x -cpu=1,4,8 -benchmem -count=1 -timeout=90s

GOMAXPROCS=4 GOCACHE=/tmp/prysm-diagnostic-buildcache go test -tags=develop \
  ./beacon-chain/sync \
  -run '^TestGossipStartupActiveValidatorCountSlotZeroCacheBypass$' \
  -bench '^BenchmarkGossipStartupFullValidationStateSlot$' \
  -benchtime=1x -benchmem -count=1 -timeout=90s \
  -cpuprofile=/tmp/genesis-ffg-slot0-full-cpu.pprof \
  -o /tmp/prysm-genesis-ffg-sync.test
```

Both commands passed. The copy/scheduling benchmark also passed with
`-bench '^BenchmarkGossipStartupValidatorScanCopyContention$' -benchtime=1x`
using the same Go command/environment without the CPU-profile options.
The modified test file remains diagnostic-only; production
source files on the confirmed path match the historical round1 revision.
