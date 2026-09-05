# Offline build and retention diagnostics

These are bounded, test-only diagnostics against the current source, whose proposer control flow is unchanged from historical revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`. They did not run a devnet, alter production code, or change the configured snapshot size. The build diagnostic demonstrates controlled cancellation semantics; it does not recover the historical goroutine state or runtime schedule.

## Real `buildBlockGloas` controlled dependency

`proposer_build_retention_diagnostic_test.go` calls the real `buildBlockGloas` under the target's minimal preset. A test-local `HeadFetcher` embeds the normal chain mock but overrides `HeadETH1Data` to wait on a channel. A test-local attestation pool marks entry into the real packer, while empty production operation pools keep the rest of consensus assembly bounded. The warning is captured with the log hook, producing the observed stage order:

```text
premine eth1 warning -> HeadETH1Data access -> packer entry -> consensus completion
```

The two cases establish different meanings for late consensus-branch errors:

- With a controlled local payload failure and an empty P2P bid cache, `buildBlockGloas` returns `Could not get local payload and no P2P bid fallback` while `HeadETH1Data` remains blocked. Releasing the dependency afterward lets the already-abandoned consensus goroutine enter the packer and finish. The test observes the final consensus setter and then polls the diagnostic goroutine stack to prevent detached work from crossing fixture cleanup. This is the source-real control flow relevant to the late background packing errors after the terminal payload failures in slots 5, 6, 8, and 9.
- With local payload success, cancellation does not release the handler while `HeadETH1Data` remains blocked because `buildBlockGloas` waits for the consensus goroutine. After the dependency is released, the real packer runs and the handler returns the late `Could not compute state root: ... context ...` error. This is the source-real control flow relevant to slots 10 and 14, where the consensus branch was required before state-root computation.

The controlled wait is deliberately not labeled a reconstruction of historical head-lock contention. The historical run did not record a deciding lock owner, goroutine dump, or packer-entry marker.

The repository-standard command was:

```bash
/home/sukun/go/bin/bazelisk test //beacon-chain/rpc/prysm/v1alpha1/validator:go_default_test \
  --keep_going \
  --test_output=errors \
  --flaky_test_attempts=3 \
  --build_tests_only \
  '--test_filter=TestDiagnostic(BuildBlockGloasControlledCancellationSemantics|ParentStateSlot13CancellationBoundaries)'
```

The validator package and test compiled, but Bazel stopped before execution because the validation action reported five pre-existing `uintcast` findings in `beacon-chain/p2p/encoder/scratch.go:51,54,55,71,75`. That file is outside this diagnostic and was not modified.

The same focused test was then executed while skipping Bazel validation actions:

```bash
/home/sukun/go/bin/bazelisk test //beacon-chain/rpc/prysm/v1alpha1/validator:go_default_test \
  --keep_going \
  --test_output=errors \
  --flaky_test_attempts=3 \
  --build_tests_only \
  '--test_filter=TestDiagnostic(BuildBlockGloasControlledCancellationSemantics|ParentStateSlot13CancellationBoundaries)' \
  --norun_validations \
  --runs_per_test=10
```

Result: **PASS**, 10/10 runs; Bazel reported min/max/average test time of 0.2 seconds.

## Slot-13 parent-state cancellation differential

`TestDiagnosticParentStateSlot13CancellationBoundaries` calls the real `getParentStateFromReorgData` with a slot-0 Gloas state and target slot 13. It demonstrates two distinct pre-state mechanisms that produce the same externally wrapped error:

- A controlled `HeadState` dependency ignores cancellation while blocked. The parent-state call stays outstanding after cancellation; once the dependency releases and returns the state, the real slot-processing call observes cancellation and returns `Could not process slots up to 13: ... context canceled`.
- A real `transition.SkipSlotCache` is marked in progress for the state's production cache key. Before cancellation, a bounded all-goroutine stack observation requires the parent-state goroutine to contain both `getParentStateFromReorgData` and `cache.(*SkipSlotCache).Get`, establishing that it entered the actual in-progress wait. It then returns the same `Could not process slots up to 13: ... context canceled` wrapper when its context is canceled.

This differential shows why the historical slot-13 wrapper does not locate the preceding delay inside active slot transitions. It does not claim either controlled mechanism was the historical one.

## Goldfish supplied-cohort replay

`TestGoldfishWalk_Round2Slots15And16SuppliedCohorts` uses the real `setupGoldfish`, `insertGoldfishBlock`, `driftGenesisTime`, vote insertion, and head walk. Both blocks are genesis children with the historical roots:

```text
slot 15  0x6856419067eae1ba1bd63c1f519dcff0c921b3363a5f202ea5f69c6ef429df41
slot 16  0x4b07a2a4ef496b99e14601673b47caf145ce8834b57e2f365cb40c71b03e9857
```

For both `payload_present=false` and `payload_present=true`, it verifies:

1. During slot 15, a synthetic empty slot-14 electorate allows block 15 to become head.
2. After inserting 54 unique one-seat votes for block 15 and 458 for genesis, slot 16 begins with threshold 256, block-15 score 54, and head genesis.
3. Inserting the unique block 16 during its round-start slot distinguishes it and makes it head even though its ordinary node gate has no earned votes.
4. After inserting 23 unique one-seat votes for block 16 and 489 for block 15, slot 17 has no round-start distinction, threshold 256, scores 23/489, and head block 15.

This replay uses the supplied accepted-vote cohorts. It does not claim that the historical deciding store snapshot contained every accepted record.

Focused command:

```bash
/home/sukun/go/bin/bazelisk test //beacon-chain/forkchoice/doubly-linked-tree:go_default_test \
  --keep_going \
  --test_output=errors \
  --flaky_test_attempts=3 \
  --build_tests_only \
  --test_filter=TestGoldfishWalk_Round2Slots15And16SuppliedCohorts
```

Result: **PASS**, one target in 0.1 seconds.

The complete affected Goldfish test target was also run:

```bash
/home/sukun/go/bin/bazelisk test //beacon-chain/forkchoice/doubly-linked-tree:go_default_test \
  --keep_going \
  --test_output=errors \
  --flaky_test_attempts=3 \
  --build_tests_only
```

Result: **PASS**, one target in 5.9 seconds.
