# Real pool compaction versus raw proposer packing

## Result

The bounded diagnostic passed with the same 15,000 correctly signed slot-7
votes in two real Prysm attestation pools. The raw pool retained all 15,000
singles. The other pool ran the production
`AggregateUnaggregatedAttestations` path once and held six aggregates with no
remaining singles.

An uncanceled production `packAttestations` call produced one attestation in
both arms with the same attestation data and the same 15,000-validator coverage.
The production signature batch verified both results. The raw call took
5.112349960 seconds; the already compacted call took 1.069273 milliseconds.
Compaction itself took 1.622792304 seconds, so compaction plus its first packing
call cost 1.623861577 seconds. That total includes the preprocessing rather than
subtracting it or treating it as free.

With a separately labeled 300 millisecond cancellation-sensitivity budget, the
actual `packDepositsAndAttestations` call behaved differently:

| Pool state | Call duration | Output | Error |
| --- | ---: | ---: | --- |
| 15,000 raw singles | 5.139071165 s | 0 | bare `context deadline exceeded` |
| six production aggregates | 1.195880 ms | 1 | none |

The disconnected execution mock made the deposit child cheap and emitted its
normal warning. There was no artificial sleep, held lock, goroutine dump,
runtime trace, or profile. An already-canceled call against a fresh compacted
pool returned the bare `context canceled` value in 1.107976 milliseconds. These
errors establish the outer cancellation category; they do not identify which
errgroup child won a race.

The 300 millisecond budget probes cancellation responsiveness. It is not the
historical slot-10 or slot-14 residual budget. This one slot-7 bucket is one
possible input shape; retained owner logs do not reconstruct the proposer
pool's raw/aggregated split or prove that it held this exact bucket.

## Input fidelity and corrections

The fixture uses a 120,000-validator Heze state at block slot 14, attestation
slot 7, source checkpoint round 0 with the zero root, and target checkpoint
round 0 with the genesis block root. The target and beacon block roots are
`1a40155d770d5a166e5976f7f9c1804026797f51b522c4959fdcde7fcaa010d1`.
The mock head and recent block slots are zero. Six computed committees contain
2,500 validators each.

Preflight checks corrected consequential assumptions before the passing
measurement:

1. Observer 400's `dataRoot` field is 31 bytes by design. It is
   `decoupled.VoteLedgerDataRoot`: the production Electra pool grouping hash
   with the first byte omitted so wire versions share a key. It is not the
   direct 32-byte `AttestationData.HashTreeRoot`.
2. The historical genesis head is payload-full after Gloas, so attestation data
   index 1 reproduces both observer anchors. Index 0 produced a different key.
3. An initial fixture forced `MaxCommitteesPerSlot=6`, causing the correctly
   merged 15,000-seat output to exceed the artificial 12,288-seat verification
   bound. The final fixture retains the default mainnet cap of 64. With
   `TargetCommitteeSize=2500`, `SlotsPerRound=8`, and 120,000 validators, the
   production calculation still yields exactly six committees. This is
   consistent with the retained startup reproduction configuration, but the
   original round-2 archive does not include its chain YAML.

The final preflight reproduced the two raw observer lines through the real
formatter:

| Slot | Target round | Full SSZ data root | Full committee-5 grouping hash | Observer field |
| ---: | ---: | --- | --- | --- |
| 4 | 0 | `fc76081183fea649c92f349c4fceac1297923d3d9c94336fa56299809b09ef98` | `47af105c327435d258e6a435c00311591bc93afc8558a6773b6d833bdab34802` | `0xaf105c327435d258e6a435c00311591bc93afc8558a6773b6d833bdab34802` |
| 8 | 1 | `481332fe1ff3a0399fd2fb4ee24ef73acf985aaceeaf859aaab34bf37d93d808` | `ec3fb84d64e264f1396d72654b6e72a0a12db7831639fcbe07de6efd95bdd552` | `0x3fb84d64e264f1396d72654b6e72a0a12db7831639fcbe07de6efd95bdd552` |

The two pool arms clone the same signed inputs. After both uncanceled packing
calls, assertions confirmed that the pool shapes remained 15,000/0
raw/aggregated and 0/6 raw/aggregated. Coverage checks use production committee
resolution and indexed conversion, compare exact validator sets, compare output
attestation data, and explicitly verify the aggregate BLS signatures outside
the measured packing interval.

The mock time fetcher remains fixed at slot 14 with head slot 0. Historical
node 85's slot-14 proposal finished during wall-clock slot 17, so this
diagnostic does not reproduce that later wall-clock view or its concurrent
work.

## Command and evidence

The diagnostic source is
`beacon-chain/rpc/prysm/v1alpha1/validator/packing_compaction_diagnostic_test.go`.
It is gated by `PRYSM_DIAGNOSTIC_PACKING_COMPACTION=1`; no production file was
changed.

The binary was built with:

```text
env GOCACHE=/tmp/prysm-diagnostic-buildcache GOMAXPROCS=4 /home/sukun/dev/go/bin/go test -p=2 -tags=develop -c -o /tmp/prysm-packing-compaction.test ./beacon-chain/rpc/prysm/v1alpha1/validator
```

The passing invocation was:

```text
env GOMAXPROCS=4 PRYSM_DIAGNOSTIC_PACKING_COMPACTION=1 /tmp/prysm-packing-compaction.test -test.run '^TestDiagnosticHistoricalPackingCompaction$' -test.v -test.timeout=5m > /tmp/prysm-packing-compaction-go.log 2>&1
```

The actual process window was
`2026-09-06T11:00:54.240976576Z` through
`2026-09-06T11:01:14.360539473Z`, exit status 0. The final `/tmp` log SHA-256 is
`2e42ccf5611e7d7f0c6b7a959b6e88c2c7f4675042254849b566d24ab2f3c73b`.

Retained evidence:

- `packing-compaction-go-evidence/full.test.log`: complete passing Go output.
- `packing-compaction-go-evidence/window.tsv`: exact process start, end, and
  exit status.
- `packing-compaction-go-evidence/observer400-anchors.log`: original beacon
  lines 8999-9000 extracted from
  `runs/round2/round2-prysm-geth-400.tar.gz`.
- `packing-compaction-go-evidence/command.txt`: exact passing invocation.

The final source is `gofmt` and `goimports` clean and the Go test package
compiled with the `develop` build tag. No Bazel test was run because this
diagnostic used the requested Go path.
