# Prysm deployed-build comparison: round2, round1, and current

## Answer

The later round1 Prysm build does **not** contain a proposal-construction,
state-root-hashing, attestation-packing, `ActiveValidatorCount`, or scheduling
performance optimization that is missing from round2. The exact implementation
files for those paths are byte-identical at round2 `0280403c`, round1
`a1679c9f`, and the current committed revision `f8e09cc3`.

Round1 adds diagnostic metrics and summary logging plus explicit stress-test
padding. Some of that code adds per-message or per-slot work. The item named
“scratch space” is message padding, not reusable hashing workspace, and is not
a state-root optimization.

The current tree has one later production fix relevant to late block contents:
commit `cabb3f8f` normalizes Gloas aggregates to Electra at attestation-pool
ingress, so pool deletion and seen checks use the same version key. That fix is
absent from both deployed builds. It can prevent old aggregates lingering and
being packed again, but it cannot explain a difference between round1 and
round2 because neither contained it.

## Revisions and seven-commit deployed range

- Round2: `0280403c70d88967f49d2d4c730f4c5417dabdf5`, built
  `2026-08-27 16:11:36+00:00`.
- Round1: `a1679c9fd82a47b3cea16ca65c84d2c4d4501fcb`, built
  `2026-09-04 10:02:40+00:00`.
- Current committed revision inspected with `jj --ignore-working-copy`:
  `f8e09cc3b054c571eac0c5eb601cc4ad25f28325`.

The seven descendants after the round2 revision are:

| Commit | Subject | Relevant effect |
|---|---|---|
| `e486f0699208f081c373a4f5ad3a5923b89f24ac` | Move fork planning docs into `plan/` | Comments and paths only in production files. |
| `0048cf75279eaaa844e7447b892f4c80fa53d0cc` | Build Dora and Buildoor from branch tips | Kurtosis/build tooling only. |
| `e17f9d8de9e8f13746f8318a9f2dbc5fe466d1d6` | Add vote arrival and seat metrics | Adds always-on metric observations and per-slot committee/pool seat accounting. This is diagnostic work, not an optimization. |
| `9e881271d098b22ad995d24b2601c33d16f048e6` | Add scratch space to gossiped blocks and Goldfish votes | Adds stress-test bytes to gossip; details below. |
| `ff6d1c4a36bbda4e100718b4072ddb0e7b026aa2` | Add per-slot summary log lines | Adds block/payload/vote summaries and associated counting/size work. |
| `4b4ab0c93b49c4ad15dfa9199fe12163c9e46f00` | Add Shadow tooling | Simulation tooling and data only. |
| `a1679c9fd82a47b3cea16ca65c84d2c4d4501fcb` | Add sim sizing plan and `--aggregators-per-committee`, default 64 | The option belongs to the Shadow script; it is not a new Prysm CLI option or default. |

The complete range changes 110 paths with 13,188 insertions and 284 deletions.
Most insertions are Shadow tooling and country-latency data. The production
paths are limited to API conversions, gossip encoding, vote instrumentation,
summary logging, metrics, and the scratch-space configuration/protobuf changes
shown by:

```sh
jj diff --ignore-working-copy \
  --from 0280403c70d88967f49d2d4c730f4c5417dabdf5 \
  --to a1679c9fd82a47b3cea16ca65c84d2c4d4501fcb \
  --name-only
```

No changed path is under `beacon-chain/core/transition`, the beacon state
implementations, `beacon-chain/operations/attestations`, proposer construction,
validator scheduling, or the execution client.

## Construction and performance path proof

These are SHA-256 prefixes of the file contents at round2, round1, and current:

| File | round2 | round1 | current |
|---|---|---|---|
| `beacon-chain/rpc/prysm/v1alpha1/validator/proposer_attestations.go` | `fd2f62ff50c5` | `fd2f62ff50c5` | `fd2f62ff50c5` |
| `beacon-chain/rpc/prysm/v1alpha1/validator/proposer.go` | `10537694df78` | `10537694df78` | `10537694df78` |
| `beacon-chain/core/helpers/validators.go` | `2f0761679dd7` | `2f0761679dd7` | `2f0761679dd7` |
| `beacon-chain/core/transition/transition.go` | `0c5230748cbc` | `0c5230748cbc` | `0c5230748cbc` |
| `validator/client/runner.go` | `3a64e979eb28` | `3a64e979eb28` | `3a64e979eb28` |
| `validator/client/validator.go` | `37afd3ea13d0` | `37afd3ea13d0` | `37afd3ea13d0` |
| `beacon-chain/execution/engine_client.go` | `8dee245d93c8` | `8dee245d93c8` | `8dee245d93c8` |

Consequences of the identical code:

- `packAttestations` and its packing/compaction algorithm did not change.
- Proposal parent/head preparation and block construction did not change.
- State transition and exact-target skipped-state-cache behavior did not change.
- The slot-0 `ActiveValidatorCount` path still bypasses a nonzero cache count
  and scans validators in all three revisions.
- Validator role dispatch and Engine request scheduling did not change.
- Aggregator selection still reads `TargetAggregatorsPerCommittee`; the Prysm
  mainnet default remains 16 at all three revisions.

The `a1679c9f` title refers to a `--aggregators-per-committee` argument added to
`shadow/run-shadow-sim.py` with default 64. A newly added Kurtosis scenario also
sets `TARGET_AGGREGATORS_PER_COMMITTEE=64` when that scenario is selected. These
are harness inputs, not a Prysm binary optimization or a change to its default.

## Scratch-space semantics

Round1 adds two stress knobs:

- `CONSENSUS_BLOCK_SCRATCH_SPACE`, default 0, prepends a magic header and
  random bytes to a Gloas block after SSZ encoding and removes them before SSZ
  decoding. The prefix is outside the signed SSZ block, so it does not alter
  the block root or post-state root.
- `GOLDFISH_SCRATCH_SPACE`, default 100, adds an SSZ field to
  `AvailableAttestation`. The bytes increase vote serialization, hashing, and
  gossip size. The vote signature still covers only `AvailableAttestationData`.

This code deliberately increases message load for a stress test. It does not
provide a scratch buffer to `HashTreeRoot`, state transition, or proposal
construction.

## Metrics and summary overhead

The later deployed build adds work that the older build lacks:

- accepted FFG and Goldfish votes update metrics;
- a per-slot job obtains the head state and committees and scans relevant
  attestation-pool entries to compute seat coverage;
- accepted blocks compute SSZ size, attestation count, and total aggregation
  bits for an Info summary; and
- payload and vote summaries emit additional Info lines.

These changes are bounded instrumentation in the source. Their presence does
not establish a material slowdown in round1, but they must not be described as
performance improvements over round2.

## Current post-run pool fix

Current production code after `cabb3f8f49c97727db1911decced34f84b22ddbe`
converts Gloas attestations to Electra before saving them from gossip or block
processing, and performs redundant/seen checks with the same Electra form.
`proposer_attestations.go` itself remains byte-identical; the fix changes the
pool entries supplied to it.

The focused existing regression passed:

```sh
/home/sukun/go/bin/bazelisk test \
  //beacon-chain/operations/attestations/kv:go_default_test \
  --test_filter='^TestKV_Aggregated_DeleteGloasAggregatedAttestation$' \
  --keep_going --test_output=errors --flaky_test_attempts=3 \
  --build_tests_only
```

Result: 1 test passed, build successful, 34.210 seconds. The test demonstrates
that a raw Gloas-keyed aggregate survives deletion by the equivalent Electra
key, while the normalized Electra-keyed aggregate is deleted. Both historical
builds predate this fix, so historical analysis must retain their raw
Gloas/Electra mismatch.

## Go/build evidence

`go.mod` is byte-identical across round2, round1, and current and declares
`go 1.26.5`; `go.sum` and build/toolchain paths do not change in the deployed
range. The retained Prysm startup line records the commit and build timestamp,
but not the compiler runtime version. Therefore the source establishes no Go
toolchain change between the two deployed builds, while the exact compiler
version used for each binary is not proven by the retained logs.

No causal conclusion about missed or late proposals follows from this source
comparison alone.
