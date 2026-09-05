# Offline deadline diagnostics

These tests exercise the real validator-client paths with controlled local
dependencies.  They are mechanism reproductions, not reconstructions of the
historical scheduler load.

## RolesAt stage differential

`validator/client/startup_deadline_diagnostic_test.go` runs three expired paths with the
same 75 ms absolute context deadline, a duty that includes proposer, attester,
and sync-committee roles, the real `localSelector`, real `RolesAt`, and real
`ProposeBlock`:

1. The ordinary attestation selection proof completes promptly, then
   `SyncSubcommitteeIndex` enters with a live context and waits until the
   deadline. In the retained verbose run, `RolesAt` takes 75.982 ms; the sync
   call occupies 75.239 ms.
2. A `context.Background()` caller wins the local selector's real singleflight
   key and holds selection-proof generation until the 75 ms role context has
   expired. Before release, a bounded all-goroutine stack sample observes the
   `RolesAt` goroutine inside `singleflight.(*Group).Do`, proving that it has
   joined the in-flight key. Once the winner is released,
   `SyncSubcommitteeIndex` enters already carrying `DeadlineExceeded` and
   returns in 1.333 microseconds. `RolesAt` takes 77.603 ms.
3. Ordinary selection and `SyncSubcommitteeIndex` both complete promptly, but
   the subsequent sync-selection `DomainData` call enters live and waits for
   the deadline. `RolesAt` takes 76.131 ms and this domain call occupies
   74.977 ms. This exercises the slot-4 terminal stage separately from the
   sync-index cases.

All three expired paths return a role map containing `RoleProposer` and no
top-level error, matching the production behavior that logs the sync-aggregator
failure and continues. Passing the same expired context into `ProposeBlock` reaches the
real RANDAO domain request with `DeadlineExceeded`; the test asserts that
`BeaconBlock` is never called. Thus an end-of-slot sync-index error followed by
a RANDAO deadline error does not prove that the sync-index RPC itself occupied
the slot. An earlier non-context-aware singleflight wait can produce the same
stage and error category. The controlled mocks return plain
`context.DeadlineExceeded`; they do not reproduce the historical gRPC status
wrapping or exact log string. A live-budget control returns the same proposer role, reaches
the real RANDAO path successfully, and enters the mocked `BeaconBlock` RPC with
a live context; the RPC then returns a controlled error to stop before block
construction.

## Attestation cache fan-out

The fan-out test runs 75 concurrent callers through the real post-Electra
`getAttestationData` cache-miss path with one shared deadline. The controlled
beacon RPC waits for cancellation only for the first admitted caller. Observed
results:

* 75 RPC entries and 75 `DeadlineExceeded` returns;
* exactly one RPC entered while the context was live;
* maximum concurrent RPC count was one;
* the 74 subsequent callers entered serially with an already-expired context
  and all workers had drained 2.947 ms after the context's recorded deadline
  in the retained verbose run.

This demonstrates why 75 client terminal records, such as node 169's slot-1
attester failures, cannot be interpreted as 75 simultaneous beacon RPCs. The
historical logs do not expose admission times, so they do not prove this exact
controlled mechanism occurred in round 2.

## DomainData cache and canceled-lock diagnostics

`validator/client/domain_cache_diagnostic_test.go` is now registered in the
explicit `//validator/client:go_default_test` source list. Its two existing
diagnostics exercise the actual cache configuration and `validator.domainData`
locking path relevant to slot 1:

* With 13 one-cost domain entries and the production Ristretto configuration,
  only **3** were retained in this run. The control with
  `IgnoreInternalCost=true` retained all **13**. This demonstrates that the
  configured `MaxCost=192` is not a thirteen-entry capacity once Ristretto's
  internal cost is charged.
* A selection-proof cache miss held the real `domainDataLock` across its
  controlled RPC. A canceled RANDAO caller remained blocked for the test's
  150 ms observation interval, did not enter its RPC while the holder owned
  the lock, and returned `context.Canceled` only after holder release.

The retained verbose command was:

```text
/home/sukun/go/bin/bazelisk test //validator/client:go_default_test \
  --test_filter='TestDomainData(CacheInternalCost|CanceledWaiter)Diagnostic' \
  --keep_going --test_output=all --test_arg=-test.v --runs_per_test=1 \
  --flaky_test_attempts=3 --build_tests_only
```

It passed both tests. Bazel reported target time 0.2 seconds; verbose output
reported `production=3 ignore-internal-cost=13`, with the canceled-waiter test
passing in 0.15 seconds. These are controlled mechanism results, not a claim
that the historical slot-1 proposer spent its unlogged interval on this lock.

## Command and result

```text
/home/sukun/go/bin/bazelisk test //validator/client:go_default_test \
  --test_filter='Test(RolesAtDeadlineStageDifferentialDiagnostic|RolesAtSyncSelectionDomainDeadlineDiagnostic|RolesAtLiveBudgetReachesBeaconBlockDiagnostic|AttestationDataCanceledFanoutDiagnostic)' \
  --keep_going --test_output=errors --flaky_test_attempts=3 --build_tests_only
```

The target passed. The retained verbose command was:

```text
/home/sukun/go/bin/bazelisk test //validator/client:go_default_test \
  --test_filter='Test(RolesAtDeadlineStageDifferentialDiagnostic|RolesAtSyncSelectionDomainDeadlineDiagnostic|RolesAtLiveBudgetReachesBeaconBlockDiagnostic|AttestationDataCanceledFanoutDiagnostic)' \
  --keep_going --test_output=all --test_arg=-test.v --runs_per_test=1
```

It passed all four tests and produced the timings recorded above.
