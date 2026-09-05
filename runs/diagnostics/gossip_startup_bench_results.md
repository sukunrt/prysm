# Startup FFG gossip validation probe

Command:

```text
GOMAXPROCS=4 go test -tags=develop ./beacon-chain/sync -run '^$' -bench '^BenchmarkGossipStartupValidation$' -benchtime=1x -count=1 -benchmem -timeout=15m
```

Host: AMD Ryzen 7 7840U. The 120k fixture is a Heze state at slot 1 with `SLOTS_PER_ROUND=8`, six committees, and exactly 2,500 seats per committee. Committee lookups are prewarmed as they were by startup duties. Every sampled vote has a distinct randomly generated BLS key and valid signature. Fixture creation, key generation, signing, and cache warming are outside benchmark timing.

| Registry | Valid single votes | Elapsed | Allocated bytes | Allocations |
|---:|---:|---:|---:|---:|
| 15,000 | 1,000 | 101.70 ms | 4,849,592 | 72,066 |
| 120,000 | 1,000 | 103.55 ms | 4,805,088 | 71,968 |
| 120,000 | 2,500 | 254.28 ms | 11,826,904 | 179,561 |

The timed path uses production `BeaconCommitteeFromState`, `validateAttesterData`, `blocks.AttestationSignatureBatch`, and `verifyBatch`, with verification batches of 64 as configured in the historical runs. Thus it includes real distinct-key BLS batch verification and the core pre-pool validation work, but deliberately excludes gossip decoding, DB/block-presence checks, fork-choice/state-target retrieval, event-feed notification, seen-cache insertion, and pool subscriber work.

The registry comparison is nearly flat at a fixed 1,000 messages (120k is 1.8% slower, within single-run noise). The measured work instead scales approximately linearly with message count: 2,500 valid votes take 254 ms and allocate 11.8 MB. On this four-logical-processor probe, the core warmed committee/data/BLS stages alone are not remotely sufficient to consume the six-second observed arrival window. This does not reproduce the live stall and does not clear the excluded DB, fork-choice, feed, pool, scheduler, or network paths.

Source: [`gossip_startup_diagnostic_test.go`](../../beacon-chain/sync/gossip_startup_diagnostic_test.go).

## Legacy Electra pool probe

The historical startup configuration did not enable the experimental attestation pool. A second probe therefore exercises the legacy pool with all 15,000 distinct seats in the six 2,500-seat committees. The fixture uses Electra-form attestations produced from distinct valid single-attestation keys and signatures. Fixture creation and pool prepopulation are outside timing.

Command:

```text
GOMAXPROCS=4 GOCACHE=/tmp/prysm-diagnostic-buildcache go test -tags=develop ./beacon-chain/sync -run '^$' -bench '^BenchmarkGossipStartupLegacyPool$' -benchtime=1x -count=1 -benchmem -timeout=15m
```

| Stage | Elapsed | Allocated bytes | Allocations |
|---|---:|---:|---:|
| 64-worker subscriber ingest, 15,000 votes | 77.00 ms | 143,187,384 | 1,268,172 |
| Six concurrent aggregator-RPC selections and merges | 608.68 ms | 39,872,768 | 330,408 |
| Proposer-style snapshot of all 15,000 unaggregated votes | 40.07 ms | 34,327,656 | 330,015 |

The six aggregation duties each scan the full 15,000-entry pool, clone their 2,500 candidates, and run the same general aggregation algorithm used by `SubmitAggregateSelectionProofElectra`; the slowest selection completed in 591.08 ms. This is a meaningful CPU and allocation burst around the observed six-second boundary, though one realistic six-duty burst still does not reproduce a multi-second stall on this host. Repeated or more numerous local aggregation RPCs remain a contention candidate. The legacy subscriber and proposer pool snapshot alone are too small to explain the delay. This probe does not cover libp2p scheduling, database and fork-choice implementations, the event feed's real consumers, or concurrent validator/proposer RPC state work.

## Full validation pipeline probe

The full probe sends all 15,000 encoded `SingleAttestation` messages through production gossip decoding, cache-key and time checks, real database state-presence lookups, topic and committee validation, the production verifier routine with its unbuffered request/result handoffs, an unbuffered operation-feed subscription with a real drainer, final seen-cache insertion, and the legacy pool subscriber. It uses 64 concurrent validation workers and a 64-signature verifier batch limit. Fork-choice, LMD/FFG consistency, and target-state answers use the standard chain mock, so their production implementations remain excluded.

```text
GOMAXPROCS=4 GOCACHE=/tmp/prysm-diagnostic-buildcache go test -tags=develop ./beacon-chain/sync -run '^$' -bench '^BenchmarkGossipStartupFullValidation$' -benchtime=1x -count=1 -benchmem -timeout=15m
```

All 15,000 messages were accepted, all 15,000 feed notifications were drained, and none entered the pending queue. The full path completed in **997.07 ms**, allocating 550,431,088 bytes in 2,853,803 allocations. This is substantially more work than the earlier 2,500-vote core probe, but it still completes well inside the observed six-second arrival window. The remaining high-value suspects are live libp2p/network scheduling, production fork-choice/state-target contention, other RPC state work, and amplification from overlapping aggregation duties rather than an unbuffered verifier or event-feed handoff by itself.

With `GOMAXPROCS=1`, the same all-accepted probe took **2.855 s** (550,165,064 bytes and 2,857,811 allocations). Even this deliberately conservative CPU bound did not reach six seconds, though it shows that scheduler/CPU competition can multiply the broad validation time by roughly 2.9.
