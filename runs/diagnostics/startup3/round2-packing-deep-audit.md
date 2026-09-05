# Round 2 slots 10 and 14: packing deep audit

## Result

Packing is a necessary unfinished dependency in both failures. A new
production-path diagnostic establishes that a realistic 15,000-single legacy
pool takes 5.43--5.70 seconds of wall time on four logical processors even
without competing gossip work. The historical logs still do not support
attributing the full 29--41 second return times to this cost alone. The
strongest source-and-log explanation is broader process contention plus a
packing path with sparse cancellation: the goroutine may wait for CPU or a
pool mutex, and once running it traverses several loops that do not observe
cancellation until they finish. Pool work is now a measured substantial
amplifier, although its historical object count remains unknown. Composing the
same packer with the 15,000 slot-zero count scans observed in the controlled
startup workload produces a 59.246–61.292-second return from a 12-second
context across two runs; the identical cache-hit control returns successfully
in 5.274–5.306 seconds.

This distinction matters because the terminal `context canceled` messages are
timestamps of eventual cancellation observation, not measurements of active
packing CPU.

## Exact handler bounds

Node35's slot-10 handler entered at +7.446 seconds, obtained its FCU payload ID
at +10.015, and selected its payload at +10.338. The slot context expired at
+12 seconds, but `Could not pack deposits and attestations` appeared only at
+36.873, immediately followed by state-root/build failure at +36.874
(`beacon.log:514-522`). The joined branch therefore remained outstanding for
at least 26.535 seconds after payload selection and observed cancellation
24.873 seconds after the deadline.

Node85's slot-14 handler entered at +2.694 seconds and selected its payload at
+2.744. Packing reported cancellation at +43.994 and the build failed at
+43.996 (`beacon.log:543-563`). Its joined branch remained outstanding for at
least 41.250 seconds after payload selection and observed cancellation 31.994
seconds after the deadline.

For both, `buildBlockGloas` waits for the parallel consensus-field goroutine
after payload success. The immediate packing-error/state-root-error ordering
therefore proves that this goroutine was on the necessary path. It does not
identify where inside that goroutine wall time accrued.

## What can and cannot be counted

The two owner logs do not enable the FFG/Goldfish ledger and emit no pool
snapshot count. Detailed observer FFG lines cannot be assigned to either owner,
and accepted gossip is not identical to retained normal-pool objects. Aggregate
logs' `aggregatedCount` is a bit/participant count within one aggregate, not a
candidate-object count. Block imports later report eight included attestations,
but packing applies the maximum only after snapshot validation, deletion,
deduplication, and aggregation; the included count does not bound its input.

The new `BenchmarkDiagnosticFullProductionPacker` uses the real legacy pool,
real `SaveUnaggregatedAttestations`, production validation/deletion,
deduplication, aggregation, on-chain conversion, sorting, limiting, and final
BLS verification. Its Heze/Gloas state has 120,000 validators, six committees
of 2,500, `SlotsPerRound=8`, and block slots 10 or 14. Round-0 candidates use
attestation slot 7 and round-1 candidates use slot 9 or 13. This is important:
at slots 10 and 14 round 0 is the valid previous round, not an expired round,
so those candidates traverse the expensive path.

The mixed fixture is 50/50 by participant, not uniformly mixed within every
committee: `i%2` chooses the round while `i%6` chooses the committee, yielding
three full previous-round groups and three full current-round groups (six data
groups total). The previous-only fixture has six full groups. The state is a
production-valid synthetic Heze/Gloas state with synthetic zero roots and a
current justified checkpoint labeled round 1; it is not an SSZ dump of the
historical pre-first-block state, whose checkpoint and root values can differ.
It measures the production algorithms and scale, not byte-identical history.

At `GOMAXPROCS=4`, one timed packing call measured:

| block slot | candidates | previous round | 50/50 previous/current | wrong-source invalid |
|---:|---:|---:|---:|---:|
| 10 | 600 | 31.17 ms | 35.31 ms | 3.69 ms |
| 10 | 3,000 | 226.98 ms | 238.60 ms | 29.57 ms |
| 14 | 600 | 32.60 ms | 34.44 ms | 3.92 ms |
| 14 | 3,000 | 233.36 ms | 232.24 ms | 36.26 ms |
| 14 | 15,000 | 5.433 s | 5.697 s | 1.691 s |

The valid 15,000 mixed case allocated 431.8 MB in about 1.05 million
allocations. A CPU profile surrounding only `packAttestations` (excluding
state/key/fixture construction) collected 5.84 CPU-seconds over 5.75 seconds.
`proposerAtts.dedup` accounted for 3.26 seconds cumulative (55.8%), and
`attestations.Aggregate` for 2.21 seconds cumulative (37.8%, including 1.78
seconds in `MaxCover`). `VerifyAttestationNoVerifySignature` consumed only
0.16 seconds cumulative and signature decoding 0.41 seconds. The largest flat
cost was `Bitlist.Contains`, 2.83 seconds (48.5%). These cumulative figures
overlap where callers nest.

The wrong-source case proves the fixture's invalid path and physical pool
deletion, but it is not a historical stale-round model: round 0 remains valid
through slot 15. The focused fixture test asserts every valid candidate passes
production no-signature validation, every wrong-source candidate fails, valid
packing produces output, and invalid packing leaves the real unaggregated pool
empty.

This closes the earlier whole-packer measurement gap and demonstrates enough
standalone work to miss a deadline. It still does not reproduce 26--41 seconds
on this host. Extrapolating the remaining interval requires either historical
pool shapes/counts that were not logged, concurrent CPU starvation already
demonstrated elsewhere, pool-lock delay, or some combination.

## Cancellation and concurrent genesis scans

A second gated diagnostic passed 300-millisecond and 1-second deadline
contexts into the same 15,000-candidate mixed pack. The calls returned only
after 5.319 and 5.353 seconds respectively, both with zero selected
attestations and `context deadline exceeded`. Thus cancellation prevents final
output but does not bound the dominant dedup/max-cover work: the 300-ms case
observed its deadline about five seconds late without any artificial lock or
sleep.

The diagnostic then released one-shot cohorts of production
`ActiveValidatorCount` calls at the same instant as the full packer. They used
a copy of the same 120,000-validator state. At slot zero each call performs the
genesis registry scan; the negative control used slot one with the cache
already warmed. Results at `GOMAXPROCS=4` were:

| concurrent calls | cached pack | genesis-scan pack | maximum scan call |
|---:|---:|---:|---:|
| 64 | 5.481 s | 5.447 s | 168.9 ms |
| 256 | 5.382 s | 6.085 s | 648.9 ms |
| 1,024 | 5.368 s | 9.090 s | 3.642 s |

Cached-control calls completed in at most 34 microseconds. All scan cohorts
completed by the corresponding pack return. The 1,024-scan cohort therefore
adds a measured 3.72 seconds to packing, but still does not reproduce the
historical 17--44-second joined-branch intervals. That negative result rules
out claiming that a single moderate cohort is sufficient. The historical
startup had repeated per-vote scans and other work over multiple seconds, so
the combined mechanism remains plausible, but matching its duration would
require a sustained workload or an observed larger pool rather than
extrapolating this bounded experiment.

A final sustained control used the workload volume observed in the local
startup reproduction: 15,000 production `ActiveValidatorCount` jobs dispatched
through 6,144 workers, concurrent with the same 15,000-candidate pack and a
12-second context. The matched slot-one cache-hit control executed exactly the
same jobs and worker topology. It completed packing in 5.306 seconds, selected
six attestations, and returned no error. With the shared state copy at slot
zero, all 15,000 calls performed the registry scan; packing returned only after
61.292 seconds, selected nothing, and reported `context deadline exceeded`.
All 15,000 jobs were counted, and the scan cohort completed at 61.292 seconds.

An independent repeat of the same test passed: the cached-count pack returned
in **5.273695304 seconds** with six attestations and no error, versus
**59.245856752 seconds** with zero attestations and `context deadline exceeded`
for the slot-zero scans. Both cohorts again completed all 15,000 count jobs
through 6,144 workers. The complete test took 76.227 seconds including fixture
setup; it ran Go-only with `-tags develop` and `-count=1`.

This directly reproduces a joined packing call observing its deadline tens of
seconds late under the known local-reproduction FFG volume. It uses no sleeps,
withheld locks, or infinite background loops. It does not prove that node35 or
node85 retained exactly 15,000 pool candidates or ran exactly 15,000 scans:
their logs do not expose those counts. It does prove that the two independently
established production paths compose strongly enough to exceed the historical
interval, while the identical cached-count control does not.

## Reproduction commands

All commands are Go-only; the expensive diagnostics are disabled by default.

```sh
env GOMAXPROCS=4 GOCACHE=/tmp/prysm-diagnostic-buildcache \
  go test ./beacon-chain/rpc/prysm/v1alpha1/validator \
  -run '^$' \
  -bench 'BenchmarkDiagnosticFullProductionPacker/slot_14/n_15000/' \
  -benchtime=1x -count=1 -benchmem

env GOMAXPROCS=4 GOCACHE=/tmp/prysm-diagnostic-buildcache \
  PRYSM_DIAGNOSTIC_PACKING_CPU_PROFILE=/tmp/prysm-full-packer-timed.cpu \
  go test ./beacon-chain/rpc/prysm/v1alpha1/validator \
  -run TestDiagnosticFullPackingCPUProfile -count=1

env GOMAXPROCS=4 GOCACHE=/tmp/prysm-diagnostic-buildcache \
  PRYSM_DIAGNOSTIC_FULL_PACKING_LOAD=1 \
  go test ./beacon-chain/rpc/prysm/v1alpha1/validator \
  -run TestDiagnosticFullPackingCancellationAndGenesisScanLoad \
  -count=1 -v -timeout=3m

env GOMAXPROCS=4 GOCACHE=/tmp/prysm-diagnostic-buildcache \
  PRYSM_DIAGNOSTIC_FULL_PACKING_SUSTAINED=1 \
  go test ./beacon-chain/rpc/prysm/v1alpha1/validator \
  -run TestDiagnosticFullPackingSustainedGenesisScanLoad \
  -count=1 -v -timeout=3m
```

## Cancellation and locks

On the historical default pool path:

- pool snapshots acquire `RLock` while copying; the unaggregated snapshot also
  checks seen bits and clones entries;
- invalid cleanup takes pool write locks during per-item deletion;
- lock acquisition itself has no context-aware escape;
- candidate filtering, containment deduplication, group aggregation/max-cover,
  on-chain conversion, reward sorting, and significant verification loops do
  not have a top-level cancellation check; and
- the surrounding errgroup observes cancellation only after its child returns.

Thus a canceled request can legitimately remain alive long after +12 seconds
without a deadlock. Conversely, there is no demonstrated lock owner, lock-hold
duration, or stack sample for node35/node85, so “pool-lock deadlock” is not a
supported diagnosis. Both calls eventually return and proceed to state-root
failure within milliseconds.

The parent state supplied to packing has advanced toward slots 10/14. Its
active-validator count is therefore cache-eligible; packing itself does not
repeat the diagnosed slot-zero full-registry scan per candidate. But the same
BN concurrently validates late round-0 FFG gossip, which can still perform
those scans. The controlled startup reproduction shows that workload can
saturate CPU and leave unrelated runnable goroutines unscheduled. Applying
that exact mediation to these historical owners remains an inference because
they have no runtime profile.

## Why slot 14 can fail while sampled observers improve

The observer cohort p95 declines across slots 8--12, but that is neither a
network-wide barrier nor a measurement of node85. Old round-0 messages remain
age-valid, observers recover unevenly, and each node has its own admission and
scheduler backlog. Node85's prompt +2.744 payload proves that some RPC work ran;
it does not prove the parallel packing goroutine received sustained CPU or an
uncontended pool lock afterward. Therefore the slot-14 failure is compatible
with declining aggregate pressure without implying that quadratic packing
suddenly grew.

## Causal conclusion

Directly established:

1. payload retrieval completed;
2. the joined consensus/packing branch did not return by the deadline;
3. cancellation was observed tens of seconds late;
4. the code contains uncancelable lock waits and long cancellation-sparse
   regions; and
5. the full production packer takes 5.4--5.7 seconds for a plausible 15,000
   valid-single shape, with nearly all sampled CPU in dedup/max-cover rather
   than state validation; and
6. composing it with 15,000 real slot-zero count scans reproduces
   59.246–61.292-second late-cancellation returns in two runs, while the
   matched cache-hit workload does not.

Not established: candidate count, same-data group sizes, lock ownership,
active CPU time, or which internal packing substage dominated on the
historical owners. The historical root cause can therefore be stated as
**global startup contention reaching a cancellation-sparse joined packing
path**, with pool size/algorithmic work as a measured but historically
unquantified amplifier—not as proven O(n-squared) packing on those owners.
