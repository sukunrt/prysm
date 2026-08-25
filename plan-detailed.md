# Detailed task list

Companion to `plan.md`. That file holds the reasoning; this one holds the
work. Each task is one edit or one small group of edits. Line numbers are
from 2026-08-19 and will drift — treat them as pointers, not addresses.

Design record: `task.md`, sections "Session 2026-08-18" and
"Session 2026-08-19".

## Rules

- **Never abandon or rewrite an existing jj change.** Everything here is new
  changes on top of `trspsvxl`.
- One jj change per numbered group. Describe each.
- Sign with a single `Assisted-By: <model>` trailer. No `Co-Authored-By:`.
- `go vet` and `go modernize` after each group. Lines under 100 characters.
- **Never delete data without asking**: run outputs, logs, tarballs. The
  source files named here are fine to delete.

## The one thing not to get wrong

**Genesis is Heze. There is no upgrade to Heze, ever.** No fork transition is
implemented, tested, or crossed. `heze.UpgradeToHeze` is deleted, not left
unimplemented. A node starts at slot 0 with a Heze-versioned state.

This needs `GloasForkEpoch = 0` **and** `HezeForkEpoch = 0`. 86 non-test
sites compare against `GloasForkEpoch` rather than the version enum. If Gloas
stays at `MaxUint64` they all go false, and the node runs Gloas-shaped
containers with every Gloas behaviour off. It compiles, starts, and is
quietly wrong.

---

# Step 1 — revert the decoupled preset

Do this first. Every proto generates one SSZ twin per preset, so step 3 with
the preset still present would regenerate three presets instead of two.

## 1.0 Read first: two meanings of "decoupled"

`grep -rn decoupled` mixes three things. Only the **preset** goes.

| keep | why |
|---|---|
| `decoupled/` (the Go package) | the mock available-attestation seat committee |
| `beacon-chain/sync/validate_beacon_attestation.go:23,364,374` | imports that package |
| `beacon-chain/p2p/gossip_scoring_params.go:15,280` | imports that package |
| `beacon-chain/sync/backfill/blobs.go:72` | the English word, in a comment |
| `build/gen`'s N-preset generalization | reads the preset list from the `.bzl`; drops to two by itself |

## 1.1 Delete files

- [ ] `config/fieldparams/decoupled.go`
- [ ] `config/fieldparams/decoupled_test.go`
- [ ] `config/params/decoupled_config.go`
- [ ] `config/params/e2e_decoupled_test.go`
- [ ] `testing/endtoend/decoupled_e2e_test.go`
- [ ] The 16 generated twins:
      `find . -path ./bazel-\* -prune -o \( -name '*.decoupled.pb.go' -o -name '*.decoupled.ssz.go' \) -print`
      They live in `proto/prysm/v1alpha1/` (11), `proto/engine/v1/` (1),
      `proto/eth/v1/` (1), `proto/ssz_query/` (1),
      `proto/ssz_query/testing/` (1), plus `beacon_state.decoupled.pb.go`.

## 1.2 Preset definition

- [ ] `proto/ssz_proto_library.bzl:13-17` — remove `"decoupled"` from `presets`
- [ ] `proto/ssz_proto_library.bzl:117-...` — delete the whole `decoupled = {}` dict

## 1.3 Build tags

- [ ] `config/fieldparams/mainnet.go:1` — `//go:build !minimal && !decoupled`
      becomes `//go:build !minimal`
- [ ] `config/fieldparams/mainnet_test.go:1` — same
- [ ] Generated `.ssz.go` / `.pb.go` files carry the same constraint. Do not
      hand-edit; `make gen` in 1.7 rewrites them.

## 1.4 Config and CLI

- [ ] `config/params/values.go:4` — drop `DecoupledName`
- [ ] `config/params/values.go:8` — drop `EndToEndDecoupledName`
- [ ] `config/params/testnet_e2e_config.go:91-131` — delete `E2EDecoupledTestConfig`
- [ ] `config/params/preset.go:22` — drop `decoupled` from the doc comment.
      **Keep `VerifyPreset` itself.** It is a real startup check against a
      mismatched `--chain-config-file`.
- [ ] `config/params/preset_test.go:19` — drop the `case "decoupled"` arm
- [ ] `cmd/config.go:90-93` — delete the `case "decoupled"` arm that rejects
      `--e2e-config`

## 1.5 Build and test tooling

- [ ] `build/test/main.go:25,29,130-132,144-176` — remove the `decoupled`
      kind, `decoupledPackages`, `decoupledPkgs`, `decoupledExcludeRe`
- [ ] `build/test/main_test.go:107-136` — remove the decoupled subtest and
      drop `decoupled` from the expected kind list
- [ ] `build/gen/preset_consistency_test.go:19` — drop the
      `"decoupled": params.DecoupledConfig` entry
- [ ] `build/gen/proto_test.go:44-60` — drop the decoupled fixture
- [ ] `Makefile:9` — drop `decoupled` from `TEST_KINDS`
- [ ] `.github/workflows/go.yml:148-161` — delete the `test-decoupled` job

## 1.6 Bazel

- [ ] `config/BUILD.bazel:18-21` — delete the `decoupled` config_setting
- [ ] `proto/BUILD.bazel:31-34` — delete the `ssz_decoupled` setting
- [ ] `proto/prysm/v1alpha1/BUILD.bazel:243` — drop the `ssz_decoupled` select arm
- [ ] `config/fieldparams/BUILD.bazel:9` — drop the `//config:decoupled` select arm
- [ ] `config/fieldparams/BUILD.bazel:44-50` — delete `go_decoupled_test`
- [ ] `config/params/BUILD.bazel:11` — drop `decoupled_config.go` from srcs
- [ ] `testing/endtoend/BUILD.bazel:4` — drop the gazelle exclude
- [ ] `testing/endtoend/BUILD.bazel:200-225` — delete `go_decoupled_test`
- [ ] Check `proto/engine/v1/BUILD.bazel`, `proto/eth/v1/BUILD.bazel`,
      `proto/ssz_query/testing/BUILD.bazel`, `beacon-chain/p2p/BUILD.bazel`,
      `beacon-chain/sync/BUILD.bazel` for stale entries. The last two are
      probably the `decoupled/` package dependency — keep those.

## 1.7 Regenerate

- [ ] `make gen proto ssz mode=force`
- [ ] Confirm the generated build constraints collapsed from
      `!minimal && !decoupled` to `!minimal`
- [ ] `grep -rn decoupled` and clear whatever is left in the preset sense

## 1.8 Verify

- [ ] `make test mainnet`
- [ ] `make test minimal`
- [ ] `bazelisk build //...`
- [ ] No `.decoupled.*` file remains
- [ ] Nothing references the `decoupled` build tag

Known pre-existing breakage, not caused by this step and not to be fixed
here: `TestLoadConfigFile` (HEZE placeholder fields and upstream
`PAYLOAD_DUE_BPS` drift), and `go build -tags=minimal` in
`proto/prysm/v1alpha1/attestation/aggregation/testing`.

## 1.9 Sites the plan missed <added by executor agent>

Found while executing step 1 on 2026-08-19. All are done; this is the
reconciliation list.

### Three more Bazel/Starlark sites

The plan's 1.6 list did not name these, and the tree does not build without
them.

- [x] `tools/go/def.bzl:12-14` — the `elif attr.eth_network == "decoupled"`
      arm of `_go_test_transition_impl`. Deleted.
- [x] `tools/go/def.bzl:47` — `"eth_network": attr.string(values = [...])`
      still listed `"decoupled"`. Dropped.
- [x] `tools/methodical.bzl:63` — `go_build_constraint = "!minimal &&
      !decoupled"`, hardcoded. Now `"!minimal"`. **This is the Bazel twin of
      the constraint `build/gen` writes**, and unlike `build/gen` it does not
      read the preset list from the `.bzl`, so it does not collapse by
      itself. Adding a preset later means editing this string too.
- [x] `proto/ssz_proto_library.bzl:177-178` — the plan named lines 13 and 117
      only. `_ssz_proto_files_impl` has a third site: the
      `elif (ctx.attr.config.lower() == "decoupled"): subs = decoupled` arm.

### Go fallout the plan did not predict

- [x] `cmd/config.go` — deleting the `case "decoupled"` arm leaves the
      `errors` import unused. Dropped it.
- [x] `config/params/values.go` — removing `DecoupledName` and
      `EndToEndDecoupledName` (the two longest names) makes gofmt re-align
      the whole const block. The diff is bigger than two lines.
- [x] `config/params/testnet_e2e_config.go` — `E2EDecoupledTestConfig` runs
      to **EOF (91-155)**, not 91-131. The plan's range would have left the
      function body behind.
- [x] `build/gen/proto_test.go` — besides the fixture there are two more
      references: the `applyGenModes(dir, []string{"minimal", "decoupled"})`
      call and the `mode(decoupled)` assertion.

### `withoutEvaluators` dies with the e2e file

`testing/endtoend/decoupled_e2e_test.go` also held `withoutEvaluators`, the
helper that drops named evaluators from a run. Nothing else uses it, so it
went with the file — but **step 5.2's replacement e2e test needs it back**.
It is 12 lines; copy it from this change's diff rather than rewriting it.

### There were no yaml files to update

`plan.md` decision 10 says "Update the e2e and sim yaml files". No yaml in
this repo carried `SLOTS_PER_EPOCH: 8` — `fulu-devnet.yaml` and
`fulu-devnet-4.yaml` do not set it at all. The 8-slot epoch lived only in
`DecoupledConfig()`. The sim yaml lives in the ethshadow repo, not here, and
is step 5.3's problem.

### A third `TestLoadConfigFile` drift

The known-breakage note lists the HEZE placeholder fields and
`PAYLOAD_DUE_BPS`. There is a third: `MIN_BUILDER_WITHDRAWABILITY_DELAY`,
want 64, got 8192. Same cause (upstream config drift), also pre-existing.

### The `-tags=decoupled` accept criterion is not testable

`plan.md` asks that `go build -tags=decoupled ./...` "no longer resolve". It
still succeeds, and always will: Go accepts any unknown build tag, and
`mainnet.go` is now plain `!minimal`, so an arbitrary tag just selects
mainnet. The real criterion is the one 1.8 already states — nothing in the
tree references the tag. Verified by grep.

### Commit shape

The plan asks for one jj change per numbered group. That would leave several
non-compiling intermediates, because the deletions in 1.1 and the edits in
1.4-1.6 are mutually dependent. Landed as **two** changes instead:

1. `Revert the decoupled SSZ preset` — every hand-written edit plus all 16
   generated twins. Tree builds and tests at this point; only the stale
   `!minimal && !decoupled` constraint strings remain, and they are inert
   once nothing sets the tag.
2. `gen: collapse the generated build constraint to !minimal` — a pure
   `make gen proto ssz mode=force` diff, exactly 16 one-line changes.

---

# Step 2 — `primitives.Round` and `SlotsPerRound`

`SLOTS_PER_ROUND` sizes nothing in SSZ, so **no regeneration**. It is not
inert, though: Simplex reshuffles committees per round.

## 2.0 The identity rule

`SlotsPerRound == SlotsPerEpoch` reproduces today's committee math exactly.
The spec's own configs use it (mainnet 32, minimal 8). So mainnet and minimal
get their epoch length here, and **no shipped config changes behaviour in
this step**. Only the devnet and sim yaml get 8, and that happens in step 5.

## 2.1 The type

- [ ] New `consensus-types/primitives/round.go`: `type Round uint64`.
      A **distinct type, not an alias of `Epoch`** — an alias lets an
      epoch/round mix-up compile silently.
- [ ] Mirror the subset of `Epoch` that callers need. `Epoch` has 14 methods
      (`consensus-types/primitives/epoch.go:19-121`): `Mul`, `SafeMul`,
      `Div`, `SafeDiv`, `Add`, `SafeAdd`, `AddEpoch`, `SafeAddEpoch`, `Sub`,
      `SafeSub`, `Mod`, `SafeMod`, `HashTreeRoot`, `HashTreeRootWith`.
      Start with `Mul`, `Div`, `Add`, `Sub`, `Mod` and their `Safe` forms.
      Skip the SSZ methods until a container needs them.
- [ ] Add the BUILD.bazel entry beside `epoch.go`.
- [ ] Unit tests mirroring `epoch_test.go`.

## 2.2 Slot arithmetic

In `time/slots/slottime.go`, beside `ToEpoch:69`, `EpochStart:106`,
`IsEpochStart:139`:

- [ ] `RoundAt(slot primitives.Slot) primitives.Round`
- [ ] `RoundStart(round primitives.Round) (primitives.Slot, error)`
- [ ] `IsRoundStart(slot primitives.Slot) bool` — the Goldfish gate needs it
      in a later step; add it now with tests.
- [ ] Unit tests, including `SlotsPerRound = 8` against `SlotsPerEpoch = 32`.

## 2.3 Config value

- [ ] `config/params/config.go:72` — add a field beside `SlotsPerEpoch`,
      of type `primitives.Slot`, tagged `yaml:"SLOTS_PER_ROUND"` and
      `spec:"true"`. Copy the tag style from the `SlotsPerEpoch` line above it.
- [ ] `config/params/mainnet_config.go:110` — `SlotsPerRound: 32`
- [ ] `config/params/minimal_config.go:47` — `minimalConfig.SlotsPerRound = 8`
- [ ] `config/params/loader.go:190` — add `SLOTS_PER_ROUND` to the print list
- [ ] `config/params/loader_test.go` — add it to the assert list
- [ ] `beacon-chain/rpc/eth/config/handlers_test.go:247` — the count assert
      `211` becomes `212`. `prepareConfigSpec` (`handlers.go:163-183`)
      reflects over fields tagged `spec:"true"`, so the handler itself needs
      no edit.
      **<added by executor agent>** The baseline is not 211. Heze had already
      added three `spec:"true"` fields (`HEZE_FORK_VERSION`,
      `HEZE_FORK_EPOCH`, `AVAILABLE_ATTESTATION_DUE_BPS_HEZE`) without
      updating this test, so it was failing at 214. Fixed in `config: test
      the Heze fork fields instead of skipping them`. **The number to change
      is 214 → 215.** The same change also dropped `HEZE_FORK_EPOCH` and
      `HEZE_FORK_VERSION` from `loader_test.go`'s `placeholderFields`; add
      `SLOTS_PER_ROUND` to the assert list as this step already says, and do
      not re-add anything to `placeholderFields`.

## 2.4 Validation

- [ ] One check: `SlotsPerRound` is non-zero and divides `SlotsPerEpoch`.
      The spec requires it (`beacon-chain.md:303`). Put it next to the
      `VerifyPreset` call at `beacon-chain/node/node.go:336`, or in the
      config loader. Either is fine as long as a bad value is a loud startup
      failure.
- [ ] Test both failure modes.

## 2.5 Committee math

`beacon-chain/core/helpers/beacon_committee.go`:

- [ ] `SlotCommitteeCount:55` — divide the active count by `SlotsPerRound`
      instead of `SlotsPerEpoch`. Update the spec pseudocode comment above
      it. Spec: `beacon-chain.md:1161`.
- [ ] `BeaconCommittee:242` — the index offset uses slot-within-round
      (`slot - RoundStart(RoundAt(slot))`) instead of
      `slot.ModSlot(SlotsPerEpoch)`, and `count` multiplies by
      `SlotsPerRound`. Spec: `beacon-chain.md:1183`.
- [ ] The committee cache is keyed by `(slot, seed, index)`. No change.
- [ ] `CommitteeAssignments` in the same package: duty enumeration is
      per-epoch today and becomes per-round. Spec: `validator.md:228`.

## 2.6 The dedup trap

- [ ] `beacon-chain/sync/validate_aggregate_proof.go:259` —
      `hasSeenAggregatorIndexEpoch` keys on the **epoch**. With 4 rounds per
      epoch a validator can aggregate in more than one round, and the later
      aggregates are dropped as duplicates. Change the key to the round.
      Rename the function to match.
- [ ] `beacon-chain/sync/validate_beacon_attestation.go:455-461` — the
      unaggregated key is already `(slot, committee index, attester)`.
      **No change.**

## 2.7 Verify

- [ ] Full suite green with **no expectation edits**, because every shipped
      config has `SlotsPerRound == SlotsPerEpoch`.
- [ ] Spectests green — they run the mainnet and minimal configs.
      **<added by executor, 2026-08-19>** User decision: do NOT run spectests
      as routine step verification — too slow. Unit tests plus
      `bazelisk build //...` suffice per step; spectests get one dedicated
      fix-up pass at the end. When that pass runs, use the Bazel path below.
      **<added by executor agent>** Run them with **`bazelisk test
      //testing/spectest/...`**, not `make test mainnet-spectest`. The two
      paths read different copies of the spec data:

      | path | source | state on 2026-08-19 |
      |---|---|---|
      | Bazel | its own external repo, fetched from the `WORKSPACE` pin | correct (`v1.7.0-alpha.13`) |
      | `go test` | `third_party/testdata`, marker-cached | drifted from the pin |

      The `go test` corpora are ~1.26 GB behind (`consensus_spec_tests_mainnet`
      and `_minimal`). The user decided on 2026-08-19 not to refresh them, so
      the Bazel path is the one that gives a trustworthy answer here. The
      2 MB `consensus_spec` archive (spec *configs*, not vectors) was
      refreshed, which is what `TestLoadConfigFile` reads.
- [ ] New unit tests with `SlotsPerRound = 8` and `SlotsPerEpoch = 32` show
      the round's 8 slots partition the whole active set: no validator in two
      slots of one round, none missing, and the union equals the active set.

## 2.8 Executed 2026-08-19 <added by executor>

Done as three jj changes: `lssoxmyt` (Round type), `oltyqmro` (config field,
`VerifyRounds`, slot arithmetic), `lomukltk` (committee math, cache, dedup,
partition tests). Deviations from the list above, all reconciled here:

- **2.5 is wrong about the cache.** The key `(slot, seed, index)` is fine but
  the *contents* are not: `UpdateCommitteeCache` / `fillCommitteeCacheAsync`
  store `CommitteeCount = SlotsPerEpoch * committeesPerSlot`, and
  `beacon-chain/cache/committee.go` `Committee()` recomputes committees-per-
  slot and the offset from `SlotsPerEpoch`. All three sites now use
  `SlotsPerRound`; without them the cached and uncached paths disagree the
  moment a round is shorter than an epoch.
- `BeaconCommittee` uses `slot.ModSlot(SlotsPerRound)` rather than
  `slot - RoundStart(RoundAt(slot))` — same value, no error branch in a hot
  path.
- Validation is `params.VerifyRounds` in new `config/params/rounds.go`,
  called from `beacon-chain/node/node.go` right after `VerifyPreset`.
- `testing/slasher/simulator/simulator.go` overrides `SlotsPerEpoch = 4` at
  runtime; it now sets `SlotsPerRound` to match, else mainnet's 32 would not
  divide 4. Two tests in `beacon_committee_test.go` that set
  `SlotsPerEpoch = 4` got the same treatment.
- Sync dedup: renamed to `hasSeenAggregatorIndexRound` /
  `setAggregatorIndexRoundSeen`, taking `primitives.Round` from
  `slots.RoundAt(data.Slot)`. A third call site the plan missed:
  `beacon-chain/sync/pending_attestations_queue.go:368`.
- `handlers_test.go` also needed a `case "SLOTS_PER_ROUND"` value arm (its
  `default:` errors on unknown keys), besides the 214 → 215 count.
- **`CommitteeAssignments` keeps its epoch signature.** It enumerates the
  epoch's *first round* only; committees repeat identically each round, so
  committee and index are right, but the attester-slot field reports only the
  first round's slot. Reporting all of them needs an API change
  (`CommitteeAssignment` holds one slot). **Under a short round the validator
  duty API under-reports duties — must be dealt with in step 5.**
- `ComputeSubnetForCommitteesPerSlot` still uses `SinceEpochStarts` —
  considered and deliberately skipped: it is a subnet mapping, consistent
  across nodes either way.
- **Blanket `go modernize` is not safe in this tree** — it rewrote five
  unrelated files, one (`deposit_pruner.go`) with a likely behaviour change.
  Unrelated churn reverted; only fixes to new code kept.
- Pre-existing failures confirmed at baseline, not touched:
  `beacon-chain/core/helpers` sync-committee cache flake,
  `beacon-chain/rpc/prysm/v1alpha1/beacon` (34 failures, identical set
  before/after), `sync_fuzz_test.go` vet errors.

---

# Step 3 — Heze owns the state container

Copy `BeaconStateGloas` to `BeaconStateHeze`. Nothing in the container
changes yet.

## 3.0 Blocks stay Gloas-shaped

Deliberate, and expressible — contrary to what `task.md` decision 11
assumed. The shape rule splits by consumer:

- **State**: `encoding/ssz/detect/configfork.go` is the only place mapping a
  fork version to a state container.
- **Wire objects**: `beacon-chain/p2p/types/object_mapping.go` and
  `beacon-chain/sync/context.go` map the Heze *digest* to block, metadata,
  attestation, slashing, light-client and column containers.

So the state becomes Heze while wire objects stay Gloas, and each consumer
says so directly.

## 3.1 Proto

- [ ] Add `BeaconStateHeze` to `proto/prysm/v1alpha1/heze.proto`, a
      field-for-field copy of `BeaconStateGloas` (`gloas.proto:260-392`).
      Keep field numbers and every `ssz_size` / `ssz_max` option string
      identical.
- [ ] Wire it into `proto/prysm/v1alpha1/BUILD.bazel` beside the Gloas state.
- [ ] `make gen proto ssz mode=force`

## 3.2 state-native

Six non-test files, about 98 Gloas mentions.

- [ ] `state_trie.go:136` — add `hezeFields`, copied from `gloasFields`
- [ ] `state_trie.go:203` — `InitializeFromProtoHeze`
- [ ] `state_trie.go:814` — `InitializeFromProtoUnsafeHeze`
- [ ] `state_trie.go:888,1148` — add the `hezeFields` loops
- [ ] `state_trie.go:946,1049` — `Copy()`: add `case version.Heze` with
      `BeaconStateHezeFieldCount`
- [ ] `state_trie.go:1206` — `initializeMerkleLayers`: add `case version.Heze`
- [ ] `getters_state.go:260` — `ToProtoUnsafe`: add `case version.Heze`
- [ ] `getters_state.go:558` — `ToProto`: add `case version.Heze`
- [ ] `hasher.go:47` — `ComputeFieldRootsWithHasher`: add `case version.Heze`
- [ ] `beacon_state.go:74` — the Gloas field block is shared across versions;
      confirm no version gate is needed
- [ ] `ProtobufBeaconStateHeze`, mirroring `ProtobufBeaconStateGloas`
- [ ] **No change** to `getters_gloas.go` / `setters_gloas.go`. They guard on
      `b.version < version.Gloas`, and Heze sorts above Gloas.

## 3.3 Field count

- [ ] `config/params/config.go:184` — add `BeaconStateHezeFieldCount int`
- [ ] `config/params/mainnet_config.go:230` — `BeaconStateHezeFieldCount: 46`,
      matching Gloas

## 3.4 Version plumbing

- [ ] `runtime/version/fork.go:40` — remove **both** `Gloas` and `Heze` from
      `unsupportedVersions`. Genesis at Heze needs both selectable, and
      `cmd/prysmctl/testnet/generate_genesis.go:137` builds its `--fork-name`
      list from `version.All()`.
- [ ] `encoding/ssz/detect/configfork.go:93-97` — replace the
      `HezeShape().VersionEnum` lookup with `fork = version.Heze`
- [ ] `configfork.go:182` — `UnmarshalBeaconState`: add `case version.Heze`
      → `&ethpb.BeaconStateHeze{}` + `InitializeFromProtoUnsafeHeze`
- [ ] `configfork.go:246` — `UnmarshalBeaconBlock`: add `case version.Heze`
      → `&ethpb.SignedBeaconBlockGloas{}`. Blocks keep the Gloas shape;
      comment it.
- [ ] `configfork.go:287` — `UnmarshalBlindedBeaconBlock`: same
- [ ] `beacon-chain/db/kv/state.go:782` — `marshalState`: add `case version.Heze`
- [ ] `beacon-chain/db/kv/state_diff_helpers.go:345` — `keyForSnapshot`: add
      `case version.Heze` with its own snapshot key byte
- [ ] `beacon-chain/rpc/eth/debug/handlers.go:117` — `getBeaconStateV2`: add
      `case version.Heze`
- [ ] `testing/util/attestation.go:178` — `GenerateAttestations`: add
      `case version.Heze`

## 3.5 Delete the shape rule and the upgrade

- [ ] `config/params/fork.go:52-64` — delete `HezeShape`
- [ ] `config/params/heze_shape_test.go` — delete
- [ ] `beacon-chain/p2p/types/object_mapping.go:272-289` — `aliasHezeEntries`
      keeps its job, but sources the shape from `cfg.GloasForkVersion`
      instead of `cfg.HezeShape().ForkVersion`. Rewrite the comment: Heze
      reuses Gloas **wire** containers; the state is its own.
- [ ] `beacon-chain/sync/context.go:94-101` — same: map `version.Heze` to
      `version.Gloas` for context bytes, sourced directly
- [ ] `beacon-chain/core/heze/upgrade.go` — delete `UpgradeToHeze`, the file,
      its test, and the BUILD entry
- [ ] `beacon-chain/sync/rpc_send_request_test.go:1743-1748` — the
      `HezeShapes` test needs rewriting or deleting

## 3.6 Deliberately unchanged

Do not edit these. They look like they need Heze arms and do not.

| site | why |
|---|---|
| the 20 `case version.Gloas` in `consensus-types/blocks/` | block version, and blocks stay Gloas |
| `consensus-types/blocks/factory.go:273,648` | same |
| `beacon-chain/rpc/prysm/v1alpha1/validator/construct_generic_block.go:60` | same |
| `beacon-chain/execution/engine_jsonrpc.go:248` | switches on `attrs.Version()`, which comes from the concrete proto type (`payload-attribute/types.go:45,107` maps V4 → Gloas). The selection ladder at `blockchain/execution_engine.go:379` is `v >= version.Gloas`, so a Heze state already picks V4. |
| `time/slots/slottime.go:74` `ToForkVersion` | maps a slot to the **wire object** version, and its callers are light-client and attestation-pool handlers. Correctly stays at Gloas. |
| `config/params/testutils.go:23` | already has a `version.Heze` arm |
| `testing/spectest/shared/common/forkchoice/runner.go:119,164` | there are no Heze spectests |
| `beacon-chain/core/gloas/upgrade.go` | `UpgradeToGloas` stays as uncalled upstream code, so rebases keep working |

## 3.7 Verify

- [ ] SSZ round trip and `HashTreeRoot` for `BeaconStateHeze` match
      `BeaconStateGloas` for identical field values
- [ ] `detect` resolves the Heze fork version to a Heze state and a Gloas block
- [ ] `grep -rn HezeShape` returns nothing
- [ ] `make test mainnet` and `bazelisk build //...`

## 3.8 Executed 2026-08-19 <added by executor>

Done as four jj changes: `lzsxwrmr` (proto), `mkonswyn` (state-native),
`xzqrwxwy` (version plumbing), `krxuztzm` (shape rule + upgrade deletion).
All acceptance tests in 3.7 added and green; `bazelisk build //...` green;
grep for `HezeShape` clean. Deviations:

Sites the plan did not name, all required:

- `proto/prysm/v1alpha1/BUILD.bazel` — the `methodical_heze_imports` genrule
  sed-stripped `encoding/binary` ("heze types are all fixed-size");
  `BeaconStateHeze` is not, so the genrule is gone and the library uses
  `:methodical_heze` directly.
- `proto/prysm/v1alpha1/heze.yaml` — `BeaconStateHeze` needs the same
  `progressive:`/`ProgressiveList` map as Gloas or roots diverge. Plus five
  new imports in `heze.proto`.
- **db/kv read side**: the plan named only `marshalState` and
  `keyForSnapshot` (write side). Also needed: `hezeKey` in `schema.go`,
  `hasHezeKey` in `key.go`, `unmarshalState` and `decodeStateSnapshot` arms —
  else every saved Heze state is unreadable.
- `beacon-chain/core/blocks/genesis.go` — `NewGenesisBlockForState` needs a
  `*ethpb.BeaconStateHeze` arm returning a Gloas-shaped signed block.
  **Step 4 depends on this; it exists now.**
- `state_trie.go` `progressiveHashTreeRoot` / `initializeProgressiveMerkleTree`
  hardcoded Gloas; now a version-keyed `progressiveFields()` helper.
- `beacon-chain/core/time/slot_epoch.go` — `CanUpgradeToHeze` deleted with
  the upgrade (it was the only caller; "no upgrade to Heze, ever").
- `testing/util/state.go` — new `NewBeaconStateHeze` helper. **Step 4 can
  use it.**

Fallout of removing Gloas+Heze from `unsupportedVersions` (largest unplanned
cost — `version.All()` doubles as a test-loop driver, so nine fork-walking
suites saw Gloas/Heze for the first time):

- Light-client tests in blockchain, db/kv, light-client, rpc/eth/light-client,
  sync: the `== version.Gloas` skips widened to `>= version.Gloas` (the light
  client genuinely stops at Fulu).
- `validator/client/beacon-api/get_beacon_block_test.go` — the REST validator
  client **has no SSZ block codec for Gloas or Heze** and silently falls back
  to the Fulu codec. Test relaxed to assert coverage up to Fulu. **Real gap:
  a Heze devnet using the REST validator client will hit it. Steps 4/5 must
  either use gRPC or accept/fix this.**
- `db/kv/state_diff_test.go` — fork-transition walk stops at Fulu (a
  Gloas→Heze transition will never exist); `createState` got a Heze arm.

Named but different in practice:

- The block-unmarshal arms merged into `case version.Gloas, version.Heze:`
  rather than duplicate cases.
- `HezeShapes` sync test: kept only the gloas-scheduled subtest, renamed
  `TestReadChunkedDataColumnSidecar_HezeUsesGloasShape`; same for
  `context_test.go` → `TestContextByteVersions_HezeUsesGloas`.
- `proofs.go` field-count ladder already stops at Fulu — no arm needed.
- `tools/ssztrace` left alone (no Heze spectests, per 3.6).

---

# Step 4 — genesis is a Heze state

`runtime/interop/premine-state.go` builds genesis. **There is no Gloas arm
anywhere in it** — every switch stops at Fulu, so Gloas-shaped genesis has
never been written. There is nothing to copy.

`task.md` originally said four switches. It is **five**.

## 4.1 The five switches

- [ ] `prepare:68` — add `version.Heze` to the allowed version list
- [ ] `empty:105` — add
      `InitializeFromProtoUnsafeHeze(&ethpb.BeaconStateHeze{DepositRequestsStartIndex: params.BeaconConfig().UnsetDepositRequestsStartIndex})`,
      following the Fulu arm at `:165`
- [ ] `setFork:343` — `pv, cv = HezeForkVersion, HezeForkVersion`. Genesis has
      no predecessor, so previous and current are the same, as Phase0 does.
- [ ] `setLatestBlockHeader:443` — the body is a zero-valued
      `BeaconBlockBodyGloas` (`gloas.proto:207`), because blocks keep the
      Gloas shape. Follow the Fulu arm at `:579` for the padding pattern, but
      use the Gloas body's own fields.
- [ ] `setExecutionPayload:628` — see 4.3. This is the real work.

## 4.2 The eight Gloas state fields

All eight are new in Gloas (`gloas.proto:373-390`) and none is set anywhere
in premine today.

- [ ] `ptc_window` ← `initializePTCWindow`
      (`beacon-chain/core/gloas/upgrade.go:156`). **Export it.** Its spec
      docstring says it is "used to initialize the `ptc_window` field in the
      beacon state at genesis and after forks", so genesis is its intended
      second caller.
- [ ] `builders` ← `OnboardBuildersFromPendingDeposits()`, already an
      exported state method. Call it after deposits are processed.
- [ ] `builder_pending_payments` — a **fixed vector** of
      `2 * SLOTS_PER_EPOCH`. Allocate it with zero-valued entries. A nil
      slice fails the SSZ round trip.
- [ ] `execution_payload_availability` — zeroed bitvector, then **set bit 0
      to 1**. Slot 0 has a payload; the slot-1 transition would set that bit
      anyway (`gloas/beacon-chain.md:1169`); and `beacon-chain.md:1395`
      asserts `latest_block_hash == latest_execution_payload_bid.block_hash`,
      which holds at genesis only if slot 0 counts as available.
- [ ] `builder_pending_withdrawals` — empty list
- [ ] `payload_expected_withdrawals` — empty list
- [ ] `next_withdrawal_builder_index` — 0
- [ ] `latest_execution_payload_bid` — see 4.3

The arm calls the two helpers `UpgradeToGloas` calls, without calling
`UpgradeToGloas` itself.

## 4.3 `setExecutionPayload`

Gloas removes `latest_execution_payload_header` and replaces it with
`latest_block_hash` (`gloas.proto:325`) and `latest_execution_payload_bid`
(`:380`). The existing code builds an `ExecutionPayloadDeneb` from the geth
genesis block `s.GB` and wraps it as a header (`premine-state.go:628-745`).

The Heze arm uses the same `s.GB` to fill `ExecutionPayloadBid`
(`gloas.proto:38-67`):

| bid field | source |
|---|---|
| `parent_block_hash` | `gb.ParentHash()` |
| `parent_block_root` | zero at genesis |
| `block_hash` | `gb.Hash()` — the same value also goes in `latest_block_hash` |
| `prev_randao` | `params.BeaconConfig().ZeroHash` |
| `fee_recipient` | `gb.Coinbase()` |
| `gas_limit` | `gb.GasLimit()` |
| `builder_index`, `slot`, `value`, `execution_payment` | 0 |
| `blob_kzg_commitments` | empty |
| `execution_requests_root` | zero root |

- [ ] Write the arm. `beacon-chain/core/gloas/upgrade.go:207` and `:312` show
      the mapping the fork path uses; mirror it, reading from the genesis
      block rather than a pre-fork header.
- [ ] Set `latest_block_hash` to the same `gb.Hash()`.

## 4.4 Config

- [ ] Set `GloasForkEpoch = 0` **and** `HezeForkEpoch = 0` in the e2e config
      (`config/params/testnet_e2e_config.go`) and the devnet yaml
      (`fulu-devnet.yaml`, `fulu-devnet-4.yaml`). Re-read "The one thing not
      to get wrong".
- [ ] Confirm the two-forks-at-one-epoch tie-break holds: sorting is by
      version enum, so Gloas sorts first and `forEpoch(0)` returns Heze
      (`config/params/config.go:525`).
- [ ] `--fork-name heze` should now resolve through `version.FromString`.

## 4.5 Verify

- [ ] `prysmctl testnet generate-genesis --fork-name heze` produces a state
      that round-trips through SSZ, with `Version() == version.Heze`.
      **Pass `--genesis-time=$(date +%s)`** — the tool otherwise takes the
      time from the input geth genesis JSON `timestamp`, not the wall clock.
- [ ] A single node starts on that genesis and reports the Heze fork version
      at slot 0.
- [ ] The chain produces and processes blocks past slot 1, which exercises
      `process_parent_execution_payload` against the genesis bid.
- [ ] `testing/endtoend/components/beacon_node.go:364` calls
      `interop.NewPreminedGenesis` with a version, so e2e picks this up for
      free once the version is Heze.

## 4.6 Executed 2026-08-19 <added by executor>

Done as four jj changes: `wyottnnr` (premine Heze arms), `suyovqzw` (config and
e2e fork plumbing), `tstzmsks` (tests), `kzqkrosx` (execution-genesis Amsterdam
time). `go build ./...`, `bazelisk build //...` and the touched packages' tests
are green.

**Genesis at Heze works. Block production does not, for a reason outside this
step.** See "The forkchoice gap" below — step 5 must fix it.

### Sites the plan did not name, all required

- **`proposer_lookahead` was never set by premine.** Only
  `InitializeFromProtoUnsafeFulu` pads the vector, so a Gloas- or Heze-shaped
  genesis failed to marshal ("bytes array does not have the correct length").
  New `setProposerLookahead` runs `helpers.InitializeProposerLookahead` for
  `version >= Fulu`. This also fixes a latent Fulu-genesis bug: with an all-zero
  lookahead, validator 0 proposes every slot of the first two epochs.
- **`execution_requests_root` must be the empty-requests root, not a zero
  root.** 4.3's table is wrong. `gloas.ProcessParentExecutionPayload` at slot 1
  hashes the block's empty `parent_execution_requests` and compares it with the
  parent bid's root; a zero root rejects the first block. The spec says
  `hash_tree_root(ExecutionRequests())` and `upgrade.go` uses
  `enginev1.EmptyExecutionRequestsHashTreeRoot()`. So does the Heze arm.
- **`testing/endtoend` does not pick Heze up for free** (4.5's last bullet is
  wrong). `types.GenesisFork()` checked down from Fulu and `types.InitForkCfg`
  had no Gloas or Heze arms, so a Heze e2e config would have produced a Fulu
  genesis. Both now handle Gloas and Heze.
- **The prysmctl flag is `--fork`, not `--fork-name`.** `heze` appears in its
  enum, as 4.4 predicted.
- **`empty` also allocates `builders`, `builder_pending_withdrawals` and
  `payload_expected_withdrawals` as empty slices**, not just the two vectors.
- **`setLatestBlockHeader`'s Gloas body needs its own new fields filled** —
  `signed_execution_payload_bid`, `payload_attestations`,
  `parent_execution_requests` — or the body root cannot be computed.
- **The execution genesis needs an Amsterdam time.** prysmctl wrote fork times
  up to Osaka. With Gloas at epoch 0 the node calls
  `engine_forkchoiceUpdatedV4`, and geth answers "Unsupported fork: fcuV4 must
  only be called for amsterdam payloads". New `interop.GethAmsterdamTime` maps
  Amsterdam to `GloasForkEpoch`. The devnet yamls also needed an explicit
  single-entry `BLOB_SCHEDULE`: without one they inherit mainnet's absolute BPO
  epochs, and geth rejects any BPO scheduled after Amsterdam.

### Named but different in practice

- The `setExecutionPayload` arm keys on `s.Version >= version.Gloas`, matching
  the `>=` ladder the rest of the function already uses.
- `ptc_window` and `builders` are set from a new `setPTCWindowAndBuilders` step
  in `populate`, after deposits; the other six fields are set in `empty`.
- Both devnet yamls got their stale comments corrected — `fulu-devnet-4.yaml`
  claimed "Setting either to 0 does not work: prysmctl cannot build a genesis
  state above Fulu", which is exactly what this step changed.

### Two traps worth recording

- **`initialize_ptc_window` does not terminate with too few validators.**
  `compute_ptc` loops until it has PTC_SIZE members, drawing from
  `get_beacon_committee`. On the mainnet preset with 10 validators most slots
  have an empty committee, so the loop spins forever. 256 validators (the
  devnet and e2e minimum) is fine. Any genesis needs at least one validator per
  slot of the epoch.
- **`UpgradeToGloas` is never reached at genesis, even with
  `GloasForkEpoch = 0`.** `transition.UpgradeState` is only called after
  `SetSlot(slot+1)`, so it never sees slot 0, and `CanUpgradeToGloas` needs an
  epoch-start slot. Checked because a stray upgrade would silently downgrade
  the Heze genesis to Gloas.

### The forkchoice gap (step 5)

A single node was run: geth 1.17.6 plus a beacon node plus a 256-key validator
client on a `prysmctl --fork=heze` genesis and `fulu-devnet.yaml`. Results:

- The node **starts on the Heze genesis** and runs at the Heze fork digest.
  The validator client gets attester, proposer and PTC duties.
- It **never produces a block**. Every slot logs
  `could not prepare payload: payload status is INVALID`, and geth logs
  `Forkchoice requested update to zero hash`.

Cause: forkchoice never gets a **full** payload node for the genesis block.
`vs.parentFull(genesisRoot)` is therefore false, so
`getParentBlockHash` returns the genesis bid's `parent_block_hash` — which is
the zero hash, because the geth genesis block has no parent — instead of its
`block_hash`. The engine gets a zero head and answers INVALID.

The genesis block *is* full: `execution_payload_availability` bit 0 is set and
`latest_block_hash == latest_execution_payload_bid.block_hash`. The fix is in
the bootstrap, not in the genesis state: at Gloas and later, the genesis block
must be inserted into forkchoice with a full payload node
(`ForkChoice.MarkFullNode(genesisRoot, bid.GasLimit)` or equivalent) alongside
its empty node. Today the only paths that create a full node are
`InsertPayload` (from an execution payload envelope) and the tree-reconstruction
path in `forkchoice.go`, and neither runs for genesis.

Also seen and **not** a Heze problem: `wanted chain ID 1, got 1337` from the
eth1 deposit poller, because `interop.GethTestnetGenesis` hardcodes chain id
1337 while `fulu-devnet.yaml` sets `DEPOSIT_CHAIN_ID: 4242`. Pre-existing; it
only disables eth1 deposit following.

### The open question, answered

`builder_pending_payments` stays epoch-sized and epoch-indexed, and that is
fine: `process_execution_payload_bid` writes at
`SLOTS_PER_EPOCH + slot % SLOTS_PER_EPOCH`, `apply_parent_execution_payload`
settles at `parent_slot % SLOTS_PER_EPOCH`, and `RotateBuilderPendingPayments`
shifts by `SLOTS_PER_EPOCH` each epoch. Every slot of the epoch still maps to a
distinct entry when the round is shorter.

**But one payment-path constant does assume the round.**
`get_builder_payment_quorum_threshold`
(`beacon-chain/core/gloas/pending_payment.go`) is
`total_active_balance / SLOTS_PER_EPOCH * numerator / denominator`. That divisor
is the balance expected to attest in one slot. With `SLOTS_PER_ROUND = 8` and
`SLOTS_PER_EPOCH = 32` each slot's committees hold a quarter of the active set,
so the threshold is four times the achievable weight and no builder payment
ever reaches quorum. `UpdatePendingPaymentWeight` accumulates real attester
balances, so the mismatch is real, not cosmetic. The divisor should follow
`SLOTS_PER_ROUND` once a config uses a short round.

---

# Step 5 — verify

## 5.1 There is no devnet preset

A preset is compile-time and fixes SSZ array sizes. `SLOTS_PER_ROUND` sizes
nothing, and `SLOTS_PER_EPOCH` stays 32, so every array keeps its mainnet
length. **One mainnet-preset binary** runs e2e, the devnet, and Shadow.

The devnet's differences are all runtime yaml:

```yaml
SLOTS_PER_ROUND: 8
GLOAS_FORK_EPOCH: 0
HEZE_FORK_EPOCH: 0
```

`VerifyPreset` passes because the yaml keeps `SLOTS_PER_EPOCH = 32`. Under
the old preset the yaml had 8-slot epochs, so it demanded a
`-tags=decoupled` binary.

## 5.2 e2e

- [ ] Write the replacement for the deleted `TestEndToEnd_DecoupledHezeConfig`:
      genesis at Heze on the mainnet preset, no fork transition anywhere.
- [ ] Run **4 epochs**, about 13 minutes. The e2e config runs 6-second slots
      (`config/params/testnet_e2e_config.go:42`), so a 32-slot epoch is 3.2
      minutes. 4 epochs is enough for finalization, which needs roughly 3
      epochs at any round length.
- [ ] **Do not** shrink `SECONDS_PER_SLOT` to compensate. Slot duration is a
      realism parameter for a network-cost study.
- [ ] Assert finalization and that available attestations still flow.

One round is 8 slots, so the committee partition and "a Heze genesis produces
and processes blocks" are both provable inside the first round. Only
finalization needs the epochs.

## 5.3 Shadow

- [ ] Flip the sim config to `SLOTS_PER_ROUND = 8` here, not earlier.
- [ ] `ethshadow` has no heze knob, so `HEZE_FORK_EPOCH` goes through
      `sim.yaml` `extra_env`.
- [ ] The genesis-generator image strips the heze→bogota EL mapping, because
      Heze is CL-only. See `Dockerfile.genesis-gen`.
- [ ] Expect roughly **4 times** the per-slot attestation traffic of a
      mainnet-shaped run: at 8-slot rounds each validator attests once per
      round, so 4 times per epoch. That is the intended result, not a
      regression.

## 5.4 Record

- [ ] All nodes finalize at cadence.
- [ ] Available attestations reach every node: 64 per slot per node.
- [ ] Per-slot attester counts and per-slot bytes on the attestation topics.
      This is the baseline the later Goldfish work is measured against.

---

# Open question for the user

- `builder_pending_payments` is sized `2 * SLOTS_PER_EPOCH` and stays
  epoch-sized. Confirm nothing in the Gloas payment path assumes the round.
