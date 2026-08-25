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

## 5.0 Design clarification from the user (2026-08-19) <added by executor>

Committee membership at a given sub-round slot offset is **fixed across the
rounds of an epoch**: with 4 rounds per epoch, slot-offset k of every round
has the same voters. The shuffle is per-epoch, keyed by slot-within-round —
exactly what step 2 implemented and its partition tests pin. Consequence for
the duty under-reporting problem (2.8): the duties API does not need
per-round reshaping. A validator's committee, index and position are
constant within the epoch; only its attesting slots multiply to
`slot + k*SlotsPerRound`, k = 0..rounds-1. The fix is to fan a duty out to
its repeat slots, not to recompute assignments per round.

## 5.0a Executed 2026-08-19: the genesis block-production blockers <added by executor>

Step 4's smoke test stalled at slot 1. The step-4 notes blamed the missing
forkchoice full node; that was real but only the **first of four** blockers,
each found by re-running the smoke test after fixing the previous one. Four
jj changes (`zlwsxtvw`, `kmtkyqvs`, `qkltmqws`, `mvoupxyn`), each with a
test that fails without its fix:

1. **Forkchoice**: `store.insert` now creates the full node itself when a
   Gloas+ block has no parent and slot 0 — the one choke point both fresh
   bootstrap and restart go through (not `MarkFullNode` from
   `saveGenesisData` as the step-4 note suggested). `head.full` also set in
   `saveGenesisData` and `initializeHead`.
2. **Genesis has no payload envelope**: `applyParentExecutionPayloadToHead`
   / `setParentExecutionRequests` looked up the parent envelope in the DB;
   a slot-0 parent now yields empty execution requests — exactly what the
   genesis bid's `execution_requests_root` commits to.
3. **Genesis block root mismatch**: step 4's `setLatestBlockHeader` header
   committed to a zero bid while `NewGenesisBlockForState` mirrors the real
   bid (upstream PR #16821 fixed the same thing). Shared
   `genesisExecutionPayloadBid()` now feeds both.
4. **Version-pair check**: `ProcessBlockNoVerifyAnySig` required
   `st.Version() == blk.Version()`; a Heze state now accepts its
   Gloas-shaped block via a local `blockVersionForState`, per-consumer
   style as step 3 established.

Smoke: geth 1.17.6 + bn + 256-key validator, 12 blocks over slots 1–13,
parent-hash chain intact, zero INVALID / zero-hash forkchoice requests
(`step5a-smoke6-*.log`; one missed slot was CPU contention from a
concurrent build). Known visible difference from other clients: our genesis
bid's `parent_block_hash` is zero, not `latest_block_hash` as in the
PR #16821 canonical devnet genesis — nothing depends on it now that the
parent is full.

## 5.0b Executed 2026-08-19: duty fan-out and the payment divisor <added by executor>

Four jj changes: `xutnklls` (builder payment divisor), `osqmuuow` (slot
helpers), `rukuuvon` (validator client fan-out plus the gossip test),
`zynzktnt` (devnet yaml `SLOTS_PER_ROUND: 8`).

### The fan-out is client-side, and the duties API does not change

Approach (b) of the two the step named. The node keeps its epoch-shaped
`CommitteeAssignments` and its epoch-shaped `AttesterDuty`; the validator
client expands the one reported slot into its repeat slots. Reasons:

- No proto, no gRPC and no REST change, so the eth-API duty path
  (`validator/client/beacon-api/duties.go`) and the deprecated gRPC path
  (`rpc/prysm/v1alpha1/validator/duties.go`) both get the fan-out for free —
  they meet in `ethpb.ValidatorDuty.AttesterSlot`.
- `CommitteeAssignment` holds one slot. Reporting four would have meant a new
  repeated field on two protos and edits in every duty consumer.
- The committee, the committee index and the position in it are constant
  across the epoch (5.0), so the extra slots carry no extra information. The
  client can compute them from `SLOTS_PER_ROUND` alone.

New in `time/slots/slottime.go`: `SinceRoundStarts`, `RoundRepeats(slot)`
listing the epoch's slots at the same round offset, and `IsRoundRepeat(a, b)`.
Under the identity config `RoundRepeats` returns the input slot alone, which is
what makes every client site a no-op there.

Four client sites take the fan-out:

- `RolesAt` (`validator/client/validator.go`): `duty.AttesterSlot == slot`
  becomes `slots.IsRoundRepeat(slot, duty.AttesterSlot)`. This is the site that
  produces the 4x traffic; the aggregator check under it already keys on the
  slot, so aggregation follows automatically.
- `subscribeToSubnets` (`subnets.go`): the node's subnet cache
  (`rpc/core/subnets.go`, `cache.SubnetIDs`) is keyed by slot, so each repeat
  slot needs its own subscription and its own aggregator decision.
- `distributedSelector.fetchSelectionProofs` (`aggregator_selector.go`): one
  DVT selection proof per repeat slot.
- `logDuties` (`duties.go`): the schedule log lists all four slots.

### Aggregator selection is slot-keyed, and stays that way

The selection proof is a BLS signature over the slot with
`DOMAIN_SELECTION_PROOF` (`signSlotWithSelectionProof`), so a validator is
selected independently in each round of the epoch. Nothing was epoch-keyed
here: `localSelector.proofCache` and `ClaimAggregateSlot` are both
`(slot, ...)`. The one epoch-keyed thing is
`distributedSelector.refreshedEpoch`, which only decides when to refetch DVT
proofs, and it refetches all four slots' proofs at once. The gossip-side dedup
was already moved to the round in step 2
(`hasSeenAggregatorIndexRound`).

### Local slashing protection had to give way

Not named by the plan, and it blocks the whole point of the step. Both
validator DBs implement EIP-3076, which stores one source/target epoch pair per
validator: `filesystem` refuses a target `<=` the recorded one, and `kv` calls
a differing signing root at the same target a `DoubleVote`. All four
attestations of an epoch share a target, so rounds 2 to 4 were refused and only
one attestation per epoch would ever have gone out.

`SubmitAttestation` now runs `SlashableAttestationCheck` only when the
attestation falls in the epoch's first round
(`slots.SinceEpochStarts(slot) < SlotsPerRound`). Under the identity config
that is every attestation, so the existing double-vote, surround and surrounded
tests pass untouched. The gate is written on the config rather than on
`slot == duty.AttesterSlot` deliberately: three existing tests call
`SubmitAttestation` at slot 30 with a duty whose `AttesterSlot` is 0, and a
duty-based gate would have needed those expectations edited.

Recorded as a deviation: a round-keyed slashing DB would be the real fix, and
it would change the EIP-3076 interchange format. Out of scope for a networking
mock.

### Nothing else drops the repeats

Checked, no change needed:

- Gossip: `validateCommitteeIndexBeaconAttestation` resolves the committee with
  `BeaconCommitteeFromState(state, data.Slot, index)`, which is round-aware
  since step 2. The unaggregated seen-cache is `(slot, index, attester)`.
  `ComputeSubnetForAttestation` keys on the slot's epoch offset, so each repeat
  rides a different subnet — consistent on both sides because both compute it
  the same way. New test
  `TestService_validateCommitteeIndexBeaconAttestation_RepeatSlotsOfARound`
  pins all of it at 8/32.
- Attestation pool: `AttCaches` keys both the store and the seen-bits on the
  attestation data id, which contains the slot.
- `ProcessAttestation`'s inclusion window is `[slot+1, slot+SLOTS_PER_EPOCH]`,
  so a repeat is includable like any other attestation.

Known and accepted: only the first attestation of an epoch earns participation
flags (`altair.ProcessAttestations` sets each flag once), and `MAX_ATTESTATIONS`
is unchanged, so a block cannot carry four times the aggregates. Both are
consensus accounting, not wire behaviour.

### The builder payment divisor

`builderQuorumThreshold` (`beacon-chain/core/gloas/pending_payment.go`) divides
by `SlotsPerRound` now. The `<spec>` block above it is left verbatim, because
`specrefs/functions.yml` pins its hash and `ethspecify check` compares them; the
deviation is a comment underneath. New test
`TestBuilderQuorumThreshold_ShortRound` shows the threshold is one slot's share
at 8/32, that a slot's whole attesting balance clears it, and that the epoch
divisor asked for four times as much.

### Left alone on purpose

- `ListValidatorAssignments` (`rpc/prysm/v1alpha1/beacon/assignments.go`) still
  reports one attester slot per validator. It is a read-only inspection API and
  nothing schedules from it; giving it the repeat slots means changing
  `ValidatorAssignments` on the wire.
- `CommitteeAssignment` keeps one slot, and `CommitteeAssignments` keeps its
  epoch-shaped signature. 2.8 left this open; 5.0's clarification is what makes
  keeping it correct.

### Pre-existing failures, confirmed unchanged

`beacon-chain/core/helpers` `TestCurrentEpochSyncSubcommitteeIndices_UsingCommittee`
(passes alone, fails in package runs), `beacon-chain/sync/sync_fuzz_test.go` vet
errors, and four `validator/client/runner.go` vet errors about discarded cancel
functions. `runner.go` is byte-identical to its state at the step-5a head.

Two more, neither on the step's known list, both confirmed by running them on
the parent change:

- `beacon-chain/rpc/eth/beacon` `TestSubmitAttestationsV2/post-electra` fails
  the same way at the step-5a head: "no attesting indices found for committee
  index 0". It is the same shape as the 34 known
  `rpc/prysm/v1alpha1/beacon` failures, and it predates this step.
- `config/params` needs `-tags develop`; without the tag the package aborts
  with "Tests in this package require extra build tag". Green with it.

### Smoke: the fan-out is visible on a running node

The step-5a setup rerun with `SLOTS_PER_ROUND: 8`
(`step5b-smoke.sh`, logs `step5b-smoke-*.log`): geth 1.17.6, one beacon node,
one 256-key validator client, `prysmctl --fork=heze` genesis.

- The client's epoch schedule reports `attesterCount=1024` — 256 validators
  times 4 rounds — where the identity config reports 256.
- 32 attesters per slot, and the pubkey set at slot k is identical to the sets
  at k+8 and k+16, for every k checked.
- `Submitted new attestations` at slots 0 to 16, 32 pubkeys each, with the
  slot-k set equal to the slot-k+8 set. So the repeats are really signed and
  sent, not just scheduled.
- `Submitted new aggregate attestations` at every one of those slots, including
  the second and third round's, so aggregation follows the fan-out.
- Zero slashing-protection failures in the client log. Blocks produced for
  slots 1 to 13, each including one aggregate.
- Remaining log noise is the single-node kind step 5a already saw: "Failed to
  find peers" and "Sync Committee Message is too old to broadcast".

### Deferred to the e2e part

- `testing/endtoend` still has no Heze/short-round config (5.2).
- No devnet or Shadow run at `SLOTS_PER_ROUND: 8` beyond the local smoke above.
- Per-slot byte counts on the attestation topics (5.4) need more than one node.

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

## 5.2a Executed 2026-08-19: the replacement e2e <added by executor>

Three jj changes carry the fixes the run needed (`unqzztmy`, `vroulvvz`,
`lumktpyp`) and one carries the test itself (`lpkuqvuq`).
`testing/endtoend/heze_e2e_test.go` holds `TestEndToEnd_HezeGenesis`,
`withoutEvaluators` recovered from the deleted file, and a `withEvaluators`
twin beside it. `testing/endtoend/evaluators/rounds.go` holds the two new
evaluators. Bazel target `//testing/endtoend:go_heze_test`,
`eth_network = "mainnet"`.

### Five epochs, not four

The step asks for four. Finalization cannot be asserted in four: the spec's
`process_justification_and_finalization` returns early while the current epoch
is at most `GENESIS_EPOCH + 1`, so epoch 1 is never justified at its own
boundary, and the first finalized checkpoint only appears during epoch 4. The
evaluator loop also stops at `EpochsToRun-1`. So four epochs runs the stock
`FinalizationOccurs(3)` never, and a `FinalizationOccurs(2)` swapped in for it
fails at epoch 3 with "expected finalized epoch to be 1, received: 0" (run 3).
Five epochs lets the stock evaluator fire at epoch 4 with finalized = 2. The
run is about 19 minutes wall clock including the sync phase.

`SECONDS_PER_SLOT` stays at 6, as the step demands. `SLOTS_PER_ROUND = 8` is
set on the test's own copy of `E2EMainnetTestConfig`, not in
`config/params`, so the other mainnet e2e tests keep the identity config.

### The iteration ladder actually used

1. A throwaway `TestEndToEnd_HezeGenesisShort` in `testing/endtoend`: one
   epoch, no checkpoint-sync phase, only the evaluators that fire at epoch 0.
   Three minutes instead of nineteen. It caught the harness problems.
   Deleted once the full test was green; it is four lines of config over
   `e2eMinimal`, so recreate rather than keep.
2. A throwaway `TestEndToEnd_AttribFuluGenesis`: the same framework, config
   and geth with `InitForkCfg(Fulu, Fulu)`. That is the attribution tool -
   it passed in 12.5 minutes while the Heze run failed, which placed the
   first failure at Amsterdam rather than in the framework.
3. Unit tests for the two consensus-side bugs, so neither needed a second
   e2e run to confirm.
4. One full mainnet-preset run as the official result.

No minimal-preset variant was built: the two epoch-scale failures were both
diagnosed from one run's logs and fixed with unit tests, so there was nothing
left to iterate.

### Three bugs the run found

1. **`fundAccount` runs out of gas at Amsterdam** (`unqzztmy`, e2e code).
   Paying into an account that does not exist yet costs about 207k gas under
   Amsterdam against 21k for a plain transfer, and the transaction generator
   funds its blob sender with a fixed 100k limit. The funding transfer is
   included but reverts out of gas, `ensureMinBalance` tops up and does not
   re-check, and the first blob transaction dies with "insufficient funds for
   gas * price + value ... have 0". The gas limit is estimated now. This is
   the first e2e to activate Amsterdam at genesis, which is why it only
   surfaced here.
2. **A Heze state has no execution payload header** (`vroulvvz`,
   `beacon-chain/blockchain/process_block.go`). `onBlockBatch` reads the
   pre-state's latest execution payload header for every block of a batch and
   the skip list stopped at Gloas, so a Heze pre-state wrapped a nil object
   and *every* initial-sync batch failed with "could not process block in
   batch: attempted to wrap nil object". A node joining a Heze chain could
   never sync. The single-block path never hit this because Gloas+ blocks
   return before the call.
3. **The engine method was picked by exact version** (`lumktpyp`,
   `beacon-chain/execution/engine_jsonrpc.go`). An empty payload attributer
   carries the beacon *state's* version, so on a Heze state every
   `forkchoiceUpdated` without attributes failed with "unknown payload
   attribute version: 8". The execution client then reported "Beacon client
   online, but no consensus updates received in a while" and its chain never
   advanced. All three nodes logged it; only the syncing node was killed by
   it, because the other two reach the engine through the Gloas proposal path
   as well.

Both consensus fixes match by version range rather than by a list of
versions, so a later consensus-only fork above Gloas does not have to be
added to them. Each has a unit test that fails without its fix.

### Evaluators dropped, and why

- `VerifyBlockGraffiti`, `FeeRecipientIsPresent`, `ValidatorsVoteWithTheMajority`,
  `ProcessesDepositsInBlocks`, `ValidatorSyncParticipation` all read blocks
  over `ListBeaconBlocks`, and `BeaconBlockContainer` has no arm for a
  Gloas-shaped block: `convertToBlockContainer` returns "block type is not
  recognized". Same family as the 34 known `rpc/prysm/v1alpha1/beacon`
  failures. `ValidatorSyncParticipation` would fail anyway, because the
  mainnet `SyncCommitteeSize` of 512 exceeds the 256 genesis validators.
- `ActivatesDepositedValidators`: mainnet `EpochsPerEth1VotingPeriod` is 64.
  It self-skips above an Electra genesis, and is dropped for clarity.
- The exit and withdrawal evaluators are appended after the options run, so
  `withoutEvaluators` cannot reach them. They are policy-gated on epoch 7 and
  simply never fire in a five-epoch run.

Nothing shared was weakened.

### Two evaluators added

`testing/endtoend/evaluators/rounds.go`:

- `AvailableAttestationsFlow` reads
  `p2p_message_received_total{topic="/eth2/<digest>/available_attestation/ssz_snappy"}`
  off every beacon node's metrics page and requires it above zero. It cannot
  use `valueOfTopic`: that helper depends on the metric name repeating in the
  HELP and TYPE comments, and those carry no labels, so a labelled sample
  reads as zero. A small line scanner reads the sample instead.
- `AttestationsInEveryRound` fetches the previous epoch's block attestations
  over `GET /eth/v2/beacon/blocks/{slot}/attestations` - the one block-reading
  API that does have a Gloas arm - and requires every round offset of an epoch
  to appear among the attested slots. If only the epoch's first round attested,
  every attested slot would land in round 0 and the check would fail.

### The run

`step5c-e2e-run6.log`, `ok ... 1138.812s`, 42 sub-tests, no failures. Node
data and component logs under `/var/tmp/claude-prysm2-e2e/logs-run6`.

- Finalization: `finalizes_at_epoch_4` passed, which asserts finalized ==
  head - 2 with no gap between the previous justified, current justified and
  current epochs. Beacon node 0 reports `just=2 fin=0` through epoch 3,
  `just=3 fin=2` at epoch 4 and `just=4 fin=3` at epoch 5.
- Available attestations: `available_attestations_flow` passed at epochs 1
  to 4 on both nodes. A live read during an earlier run showed
  `p2p_message_received_total{...available_attestation...} 512` after four
  slots.
- Every round: `attestations_in_every_round` passed at epochs 2, 3 and 4. The
  validator client's own log is the second witness - `attesterCount=512` for
  128 keys, and the same 16 pubkeys attesting at slots 33, 41, 49 and 57, the
  four round offsets of epoch 1.
- The sync phase passed too: a third beacon node joined at epoch 4, matched
  head, and the doppelganger check fired.

### Harness notes

The e2e runs as a plain `go test`, no Bazel:

```
RUNFILES_DIR=<dir> TEST_WORKSPACE=prysm TEST_TMPDIR=... E2E_LOG_PATH=...   go test ./testing/endtoend/ -run '^TestEndToEnd_HezeGenesis$' -v -count=1 -timeout 90m
```

`testing/endtoend` reaches for binaries through rules_go's `bazel.FindBinary`,
which needs a runfiles tree: `<RUNFILES_DIR>/prysm/` with `cmd/beacon-chain`,
`cmd/validator`, `cmd/geth`, `tools/bootnode` and a `testing/endtoend/static-files`
symlink. `TEST_TMPDIR` and `E2E_LOG_PATH` must be on real disk: `/tmp` is a
31G tmpfs here and one run writes a 1.6GB tracing file plus three geth data
dirs, which killed run 4 mid-flight.

## 5.3 Shadow

**<added by executor, 2026-08-19>** User parameters for the run: 10–16
nodes, ~100 validators, executed locally from
`/home/sukun/dev/decoupled-shadow-sim` (the sim workspace; the ethshadow
tool lives in `/home/sukun/dev/ethshadow`). Existing `data*` dirs and
`shadow-run*.log` files are prior run data — never deleted or overwritten;
each new run gets a fresh data dir.

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
