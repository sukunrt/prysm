# Genesis ProcessSlots diagnostics

Date: 2026-09-05. Host: AMD Ryzen 7 7840U, Linux amd64, `GOMAXPROCS=4`.

## Fixture and limits

`genesis_120k_diagnostic_test.go` constructs a minimal Heze state with 120,000
active validators, balances, and participation arrays. It has the same large
registry shape and state implementation as the run, but it is synthetic: keys
are zero, most non-registry fields are minimally hydrated, and it does not
model RPC routing or prove which RPC called `ProcessSlots`.

The root-warmed case calls `HashTreeRoot` on the exact parent instance before
copying it. It is a steady-state/lower-bound case once that instance's field
tries have been initialized. The uninitialized-parent case models copies made
from a newly decoded genesis instance. Source tracing does not prove the
request-time genesis instance is warm; in fact, the saved-state startup path
can retain a newly decoded, un-hashed instance as the genesis hot/head state.

## Commands

```sh
GOMAXPROCS=4 /home/sukun/dev/go/bin/go test ./beacon-chain/core/transition \
  -run '^$' -bench 'Genesis120KProcessSlots$' -benchtime=1x -count=3 -benchmem

GOMAXPROCS=4 /home/sukun/dev/go/bin/go test ./beacon-chain/core/transition \
  -run '^$' -bench 'Genesis120KConcurrentProcessSlots' \
  -benchtime=1x -count=3 -benchmem \
  -cpuprofile=/tmp/genesis120k-warm-cpu.pprof

/usr/lib/go/bin/go tool pprof -top -cum -nodecount=25 \
  /tmp/genesis120k-warm-cpu.pprof
```

## Results

All ranges below are the three one-iteration samples.

| Case | slot 1 | slot 2 | slot 3 |
|---|---:|---:|---:|
| Uninitialized parent | 30.1-35.7 ms, 172.27 MB | 34.2-41.0 ms, 172.28 MB | 30.3-34.5 ms, 172.29 MB |
| Warm parent, cache disabled | 50-108 us, 107-108 KB | 72-142 us, 140-143 KB | 89-123 us, 156-157 KB |
| Warm parent, cold cache | 79-92 us, 212-213 KB | 101-152 us, 244-245 KB | 124-240 us, 261 KB |
| Warm cache at exact target | 89-154 us, 314 KB | 111-197 us, 345 KB | 117-135 us, 362 KB |
| `ProcessSlotsIfNeeded` parent preparation | 77-105 us, 212-213 KB | 99-167 us, 243-244 KB | 114-124 us, 261 KB |

Root warming removes the 172 MB/request validator hashing cost. The
cold profile showed 49.35% of CPU samples in `gohashtree._hash`, reached through
validator field-trie initialization. This cost is paid per independently copied
state if those copies all derive from the same uninitialized parent; warming
one copy does not warm its siblings or the original parent.

The startup lifecycle makes that cold-parent case plausible but not proven for
the captured run. `node.startStateGen` loads a finalized state through
`StateByRoot`. At genesis, `State.Resume` replaces it with another
`BeaconDB.GenesisState` decode, then `SaveState` and `SaveFinalizedState` retain
that state or copies without hashing it. Fork-choice insertion and head
initialization read checkpoints/balances but do not hash the beacon state.
`spawnCountdownIfPreGenesis` does hash a genesis state, but obtains a separate
`BeaconDB.GenesisState` decode, so it cannot initialize the cached/head
instance's field tries. Similarly, hashing done while initially saving genesis
does not prove warmth after a later DB decode. The execution service receives
the earlier `finalizedStateAtStartUp`, but its startup paths only use state
fields (for example the deposit index) or make further `GenesisState` DB
decodes; no beacon-state `HashTreeRoot` call was found there. No unconditional
hash of the same runtime cached genesis instance was found before request
handling.

For simultaneous identical calls against a root-warmed parent:

| Fanout | slot 1 | slot 2 | slot 3 |
|---|---:|---:|---:|
| 75 | 3.39-4.73 ms, 24.2-27.2 MB | 5.56-9.67 ms, 29.0-30.0 MB | 4.46-7.79 ms, 31.5-32.4 MB |
| 1,000 | 72.4-83.2 ms, 393-404 MB | 93.4-114.4 ms, 436-444 MB | 95.9-124.6 ms, 454-463 MB |

The warm/lower-bound CPU profile attributes 40.85% cumulative CPU to `BeaconState.Copy`,
especially sync-committee byte copying; `SkipSlotCache.Get` accounts for 21.06%
cumulative and `Put` 13.40%, with substantial GC work. This is measurable
allocation/copy amplification, but its sub-125 ms stress-bound wall time does
not by itself explain multi-second RPC stalls.

## Cache behavior and causality

`ProcessSlots` accepts a cached state only when `cachedState.Slot() < target`.
When the cache contains the exact requested target, it ignores that state and
processes from the original parent again. Identical concurrent callers wait for
the current `inProgress` owner, wake, copy the exact-target cached state in
`Get`, discard it due to the strict comparison, and repeat the calculation.
The in-progress ownership behavior can also permit overlapping recalculations;
this is not guaranteed strict serialization. The warm-exact benchmark costs at least as much as a
cold cache and allocates more.

This establishes an exact-target cache/coalescing inefficiency. With an
uninitialized runtime parent it can serialize repeated full validator-trie
initializations; with an initialized parent its measured wall cost is far
smaller. Logs alone do not identify which state instance was used or its trie
warmth. It also does not
establish that 75 FFG requests take this path: slots 0-3 FFG attestation-data
requests do not necessarily advance the state, while sync/head/payload paths
can invoke parent-state preparation only several times per node. The 75 and
1,000 cases are respectively an upper fanout model and a stress bound.

## Revision comparison

The exact run revisions are `a1679c9fd82a` (round 1) and `0280403c70d8`
(round 2). There are no differences between them in
`beacon-chain/core/transition`, `beacon-chain/cache/skip_slot_cache.go`, or
`beacon-chain/state/state-native`. Round 1 adds scratch-space configuration
that is absent from the older round-2 revision, among broader changes, but no
revision change in this measured mechanism can explain a round-to-round delta.

## Speculative epoch-1 duties from slot 0

The duties-v3 startup path can request epoch 1 while the chain is still at
genesis, requiring a state transition from slot 0 to slot 32. The diagnostic
uses the 120,000-validator Heze fixture with `SLOTS_PER_ROUND=8` and
`TARGET_COMMITTEE_SIZE=2500`. Participation and inactivity arrays, the PTC
window, and proposer lookahead are hydrated; any transition error fails the
benchmark. Four-way fanout models four parallel next-epoch duty endpoints, not
1,000 requests per beacon node.

```sh
GOMAXPROCS=4 /home/sukun/dev/go/bin/go test -tags develop \
  ./beacon-chain/core/transition -run '^$' \
  -bench '^BenchmarkGenesis120KEpochOneDuties$' \
  -benchtime=1x -count=3 -benchmem -timeout=120s
```

| Parent/request pattern | Wall time | Allocated | Allocations |
|---|---:|---:|---:|
| Cold 0→32, single | 227-353 ms | 247-271 MB | 1.36 M |
| Cold 0→32, four identical calls | 792-800 ms | 990 MB | 5.44 M |
| Root-warmed 0→32, single | 191-193 ms | 72.2 MB | 625 K |
| Root-warmed 0→32, four identical calls | 501-709 ms | 289 MB | 2.50 M |
| Cold 0→32 plus staggered same-key 0→1 call, combined completion | 284-309 ms | 423 MB | 2.10 M |

This is substantially more expensive than slots 1-3 alone because it crosses
round boundaries at 8, 16, 24, and 32 and the epoch boundary at 32. Heze scans
the validator registry during each round transition and performs its full epoch
work at slot 32.

The skip-slot cache does not coalesce the four identical epoch-1 requests: the
first result is cached at exactly target slot 32, then each waiter copies and
ignores it because reuse requires `cachedState.Slot() < target`. Wall time is
therefore close to serialized repeated transition work. The different-target
test starts a slot-1 request one millisecond after starting the epoch-1
calculation. The source establishes that both use the genesis cache key and
that a later caller waits when the 0→32 call owns it. The measured 284-309 ms
is the benchmark's combined work-completion time, however: the one-millisecond
stagger is not an ownership handshake, so this is not a precise isolated
measurement of slot-1 RPC latency in every iteration.

These numbers establish a credible hundreds-of-milliseconds startup amplifier
for four eager next-epoch duty requests and a same-key head-of-line block for
ordinary slot-1 work. They still do not independently explain a 2.45-second log
delay: caller repetition across validator clients/nodes, other CPU contention,
or additional serialized work must be established from traces and logs.
