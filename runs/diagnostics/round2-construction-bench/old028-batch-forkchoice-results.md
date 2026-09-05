# Exact-028 whole `batchForkChoiceAtts` diagnostic

## Finding

On Prysm parent commit `0280403c70d88967f49d2d4c730f4c5417dabdf5`, the controlled 13,000-single fixture took about **1.06 seconds** in both the whole legacy `batchForkChoiceAtts` path and the isolated raw compactor. The directly measured precompacted continuation took **0.218 ms**. This shows that almost all elapsed time in this controlled batch is in raw compaction and cleanup; it does not reproduce or explain the observed historical T1 lower bound of 5.806 seconds.

The subtraction between the independently sampled full-batch and compactor means is not a continuation measurement. Map iteration and scheduler variation can account for that small difference. The continuation arm is the direct measurement.

## Fixture and timed arms

The fixture has six Electra single-attestation groups with 2,500-bit committees and participant counts `[2167, 2167, 2167, 2167, 2166, 2166]`, totaling 13,000 raw objects. Each group has distinct attestation data and one committee bit. Deterministic BLS keys sign each group's `AttestationData.HashTreeRoot`; every signature is a real, parsable BLS encoding. These signatures omit the consensus domain and the fixture has no 120,000-validator beacon state because this pool-maintenance path parses and aggregates signatures but does not validate consensus signatures.

Setup precomputes six covering Electra aggregates and wire-converts them to six same-data Gloas aggregates. Electra and Gloas IDs are distinct because `attestation.NewId` includes the attestation version, producing 12 versioned data/committee keys. The Gloas objects model compact aggregates retained under the historical wire-version mismatch. Initial block and fork-choice pools are empty.

Each sample creates a fresh real KV pool and `Service`, including a fresh fork-choice processed-attestation LRU. Pool population, cloning, deterministic keys, signing, and assertions occur outside the benchmark timer.

| Arm | Timed operation | Initial pool |
|---|---|---|
| `raw_full_batch` | Whole `Service.batchForkChoiceAtts` | 13,000 raw Electra singles + 6 covering Gloas aggregates |
| `raw_compactor_only` | `Pool.AggregateUnaggregatedAttestations` | Same raw and retained inputs |
| `precompacted_continuation` | Whole `Service.batchForkChoiceAtts` | 6 precompacted Electra + 6 covering Gloas aggregates; no raw singles |

After timing, every arm must have zero raw backing entries and 12 aggregate outputs with the expected per-ID coverage and parsable aggregate signatures. The full and precompacted arms must have 12 fork-choice outputs. The precompacted arm does not reproduce the 13,000 entries in the KV seen-bit cache that raw deletion creates. Its continuation has an empty raw pool and does not consult that KV seen cache, while its `Service` LRU is fresh like the other arms.

## Results

Linux/amd64, AMD Ryzen 7 7840U, `GOMAXPROCS=4`, five timed operations per arm:

| Arm | ns/op | ms/op | B/op | allocs/op |
|---|---:|---:|---:|---:|
| Whole raw batch | 1,068,264,402 | 1,068.264 | 128,459,569 | 1,081,325 |
| Raw compactor only | 1,054,091,085 | 1,054.091 | 128,380,225 | 1,080,605 |
| Precompacted continuation | 218,342 | 0.218 | 77,592 | 730 |

The one-pass correctness run also passed all three arms. Its untimed wall observations were 1.11 seconds for the raw full arm, 1.09 seconds for the raw compactor, and below the test logger's 10 ms display precision for continuation.

## Focused CPU profile

A separate three-operation raw-full run measured 1.106 seconds/op. The profile includes untimed fixture construction, so the unfiltered `runtime.cgocall` total is dominated by deterministic key creation and BLS signing and is not timed-path evidence.

Focusing the profile on `batchForkChoiceAtts` accounts for 3.67 CPU-seconds across the benchmark calibration call plus three measured calls. `AggregateUnaggregatedAttestations` accounts for the same 3.67 seconds; `DeleteUnaggregatedAttestation` accounts for 3.48 seconds, `insertSeenBit` for 3.40 seconds, and `Bitlist.Contains` for 3.15 seconds. Detached `aggregateParallel` workers do not retain `batchForkChoiceAtts` in their sampled stacks, so this focus excludes their CPU and no whole-path CPU percentage is computed from it. The profile independently agrees with the isolated compactor profile: the controlled whole-batch cost is raw deletion/seen-bit coverage scanning, while the post-compaction continuation is small.

This is a controlled microbenchmark, not a historical pool replay. It omits the actual historical pool's age, previous seen-bit contents, exact aggregate diversity, concurrent validation/writes/readers, state validation, and host scheduling. Therefore a ~1.06-second result cannot bound a historically contended invocation or identify where the remaining multi-second T1 observation arose.

## Commands and artifacts

Correctness command:

```bash
GOMAXPROCS=4 go test ./beacon-chain/operations/attestations \
  -run '^TestDiagnosticHistoricalBatchForkChoiceAtts$' \
  -count=1 -v -timeout=5m
```

Five-operation benchmark:

```bash
GOMAXPROCS=4 go test ./beacon-chain/operations/attestations \
  -run '^$' -bench '^BenchmarkHistoricalBatchForkChoiceAtts$' \
  -benchtime=5x -count=1 -benchmem -timeout=10m -v
```

Focused profile:

```bash
GOMAXPROCS=4 go test ./beacon-chain/operations/attestations \
  -run '^$' -bench '^BenchmarkHistoricalBatchForkChoiceAtts/raw_full_batch$' \
  -benchtime=3x -count=1 -timeout=10m \
  -cpuprofile=old028-batch-forkchoice-raw-full.cpu.pprof \
  -o old028-batch-forkchoice-raw-full.test
go tool pprof -top -nodecount=30 -unit=ms -focus='batchForkChoiceAtts' \
  old028-batch-forkchoice-raw-full.test old028-batch-forkchoice-raw-full.cpu.pprof
```

Artifacts in this directory:

- `old028-historical_batch_forkchoice_diagnostic_test.go`: frozen diagnostic source.
- `old028-batch-forkchoice-5x.log`: final five-operation benchmark log.
- `old028-batch-forkchoice-raw-full-profile.log`: three-operation profile-run benchmark output.
- `old028-batch-forkchoice-raw-full.cpu.pprof`: CPU profile.
- `old028-batch-forkchoice-raw-full.pprof-top.txt`: unfiltered profile top.
- `old028-batch-forkchoice-raw-full.pprof-focus.txt`: profile focused on the whole batch call.
- `old028-batch-forkchoice-raw-full.test`: exact test binary retained for profile symbolization.
- `old028-heavy13k-raw-compaction-profile.md`: independent isolated-compactor profile analysis.
- `background-aggregation-source-audit.md`: source-path and historical-metric context.

SHA-256:

```text
030d7b54166160cf5ee1939455dc0134199b69ff3edca761dda6e859e74844f5  old028-historical_batch_forkchoice_diagnostic_test.go
afa24334b0aaca0ec942d07459ddb634b149d92b2776050be4c9eceaf1d186f9  old028-batch-forkchoice-5x.log
989c22aaae5e30fe69d6f037d17706c0e4816e0dba186a4986e82f66610abb4e  old028-batch-forkchoice-raw-full.cpu.pprof
```
