# Plan: revert the preset, add rounds, and start genesis at Heze

Written 2026-08-19. This is an implementation plan for another agent. The
design record and the reasoning behind these choices are in `task.md`,
sections "Session 2026-08-18" and "Session 2026-08-19".

## Scope

Five steps, in order. Each step is one or more jj changes on top of the
current stack. Steps 1 to 4 are code; step 5 is verification.

1. Revert the decoupled preset.
2. Add `primitives.Round` and `SlotsPerRound`.
3. Give Heze its own state container, copied from Gloas.
4. Build genesis directly as a Heze state.
5. Verify with e2e and a Shadow run.

## Rules

- **Do not abandon or rewrite any existing jj change.** The preset commits
  (`ptrnsnrm` through `uqqmokkz`) sit under the Heze work. Every removal in
  step 1 is a new change on top that undoes them going forward.
- One jj change per logical piece. Describe each one.
- Sign each change with a single `Assisted-By: <model>` trailer. Never add
  `Co-Authored-By:`.
- Run `go vet` and `go modernize`. Keep lines under 100 characters.
- **Never delete data without asking.** This applies to run outputs, logs,
  and tarballs, not to the source files this plan names.

## The one thing not to get wrong

**Genesis is Heze. There is no upgrade to Heze, ever.** No fork transition
is implemented, tested, or crossed. `heze.UpgradeToHeze` is deleted, not
left unimplemented. A node starts at slot 0 with a Heze-versioned state.

This requires `GloasForkEpoch = 0` **and** `HezeForkEpoch = 0`. 86 non-test
sites compare against `GloasForkEpoch` rather than the version enum (for
example `blockchain/process_block.go:819`,
`blockchain/process_attestation.go:104`, `execution/engine_jsonrpc.go:282`).
If Gloas stays at `MaxUint64`, all 86 go false and the node runs
Gloas-shaped containers with every Gloas behaviour off. It compiles, starts,
and is quietly wrong.

The schedule already supports two forks at one epoch. See the tie-break
comment at `config/params/config.go:525`. It sorts by version enum, so Gloas
sorts first and `forEpoch(0)` returns Heze.

## Not in scope

- The Goldfish head rule and the vote-timing knobs. Later steps.
- Heze **block** containers. Blocks keep the Gloas shape in this plan. See
  step 3 for why that is coherent.
- `ROUND_SCHEDULE` as a schedule. One config value instead.

---

## Step 1: revert the decoupled preset

Mostly deletion. Do it first: every proto generates one SSZ twin per preset,
and there are 16 `.decoupled.*` files today, so doing step 3 with the preset
still present would regenerate three presets instead of two.

### Warning

`decoupled/` **the Go package stays.** It holds
`available_attestation_committee.go`, the mock seat committee for the
available attestation. Only the SSZ *preset* named `decoupled` is removed.
The word appears in both senses across the tree.

Also leave `build/gen`'s N-preset generalization in place. It reads the
preset list from the `.bzl` file, so it drops to two presets by itself.

### Delete

- `config/fieldparams/decoupled.go`, `config/fieldparams/decoupled_test.go`
- `config/params/decoupled_config.go` (`DecoupledConfig`)
- `config/params/e2e_decoupled_test.go`
- `testing/endtoend/decoupled_e2e_test.go`
- The 16 generated twins:
  `find . -name '*.decoupled.pb.go' -o -name '*.decoupled.ssz.go'`
  (paths under `proto/prysm/v1alpha1/`, `proto/engine/v1/`,
  `proto/eth/v1/`, `proto/ssz_query/`, `proto/ssz_query/testing/`)

### Edit

| file | change |
|---|---|
| `proto/ssz_proto_library.bzl:13` | remove `"decoupled"` from `presets` |
| `proto/ssz_proto_library.bzl:117` | delete the whole `decoupled = {...}` dict |
| `config/fieldparams/mainnet.go:1` | build tag becomes `//go:build !minimal` |
| `config/fieldparams/mainnet_test.go` | same tag change |
| `config/params/values.go:4` | drop `DecoupledName` |
| `config/params/values.go:8` | drop `EndToEndDecoupledName` |
| `config/params/testnet_e2e_config.go:91-131` | delete `E2EDecoupledTestConfig` |
| `config/params/preset.go:22` | drop `decoupled` from the doc comment |
| `cmd/config.go:90-93` | delete the `case "decoupled"` arm |
| `build/test/main.go`, `build/test/main_test.go` | drop the tag handling |
| `Makefile:9` | drop `decoupled` from `TEST_KINDS` |
| `.github/workflows/go.yml:148-161` | delete the `test-decoupled` job |
| `config/BUILD.bazel:18` | delete the `decoupled` config_setting |
| `proto/BUILD.bazel:31` | delete the `ssz_decoupled` setting |
| `proto/prysm/v1alpha1/BUILD.bazel:243` | drop the `ssz_decoupled` select arm |
| `config/fieldparams/BUILD.bazel:9,44-50` | drop the select arm and `go_decoupled_test` |
| `config/params/BUILD.bazel:11` | drop `decoupled_config.go` |
| `testing/endtoend/BUILD.bazel:4,200-225` | delete `go_decoupled_test` and the gazelle exclude |

Keep `VerifyPreset` (`config/params/preset.go`). It is a genuinely useful
startup check against a mismatched `--chain-config-file`. Only its mention
of the `decoupled` tag goes.

Then regenerate and let the generated build tags collapse from
`!minimal && !decoupled` to `!minimal`:

```
make gen proto ssz mode=force
```

Grep for remaining hits in the preset sense (`grep -rn decoupled` minus the
`decoupled/` package and its importers) and clean up comments.

### Accept

- `make test mainnet` and `make test minimal` pass.
- `bazelisk build //...` succeeds.
- No `.decoupled.*` files remain.
- `go build -tags=decoupled ./...` no longer resolves, and nothing references
  the tag. <added by executor agent> The first half is not testable: Go
  accepts any unknown tag, and `mainnet.go` is now plain `!minimal`, so the
  build succeeds and selects mainnet. Only the second half is a real check.

### Executed 2026-08-19 <added by executor agent>

Done, in two jj changes rather than one per subsection: `Revert the decoupled
SSZ preset` (every hand-written edit plus the 16 twins) and `gen: collapse the
generated build constraint to !minimal` (a pure regeneration diff). One change
per subsection would have left non-compiling intermediates, because the file
deletions and the config/Bazel edits depend on each other.

Four sites this section's Edit table did not name, all required:

| file | what |
|---|---|
| `tools/go/def.bzl:12-14,47` | the `eth_network == "decoupled"` transition arm and the `attr.string(values=...)` list |
| `tools/methodical.bzl:63` | `go_build_constraint = "!minimal && !decoupled"`, hardcoded — the Bazel twin of what `build/gen` writes, and it does **not** read the preset list, so it never collapses on its own |
| `proto/ssz_proto_library.bzl:177` | a third site: the `elif config.lower() == "decoupled"` arm of `_ssz_proto_files_impl` |
| `cmd/config.go` | the `errors` import goes unused once the case arm is deleted |

Two notes for later steps:

- **`withoutEvaluators` died with `decoupled_e2e_test.go`.** It is the helper
  that drops named evaluators from a run, and step 5's replacement e2e test
  needs it back. Recover it from that change's diff.
- **There were no yaml files to update.** Decision 10 says "update the e2e and
  sim yaml files"; no yaml in this repo set `SLOTS_PER_EPOCH: 8`. The 8-slot
  epoch lived only in `DecoupledConfig()`. The sim yaml is in the ethshadow
  repo and is step 5.3's problem.

`E2EDecoupledTestConfig` also runs to EOF (91-155), not 91-131.

---

## Step 2: `primitives.Round` and `SlotsPerRound`

`SLOTS_PER_ROUND` sizes nothing in SSZ, so no regeneration. But it is **not**
inert: Simplex reshuffles committees per round. See `task.md` decision 15.

### The identity trick

`SlotsPerRound == SlotsPerEpoch` reproduces today's committee math exactly.
The spec's own configs use it: mainnet 32, minimal 8. So set mainnet 32 and
minimal 8, and every existing committee test and spectest stays green with
no expectation edits.

**Do not flip any network config to 8 in this step.** Land the plumbing as a
pure identity change, and flip the devnet and e2e configs to 8 in step 5,
where the effect is measured. Cover the non-identity path with unit tests
that override `SlotsPerRound` through `params.SetupTestConfigCleanup`.

### New

- `consensus-types/primitives/round.go`: `type Round uint64`, a **distinct
  type, not an alias of `Epoch`**. `Epoch` is 152 lines and 14 methods; a
  subset is enough (`Add`, `Sub`, `Mul`, `Div`, comparison, `String`,
  SSZ marshal/unmarshal if a container ever needs it). Add the BUILD entry.
- `time/slots/`: `RoundAt(slot) Round` and `RoundStart(round) (Slot, error)`,
  as mirrors of `ToEpoch` and `EpochStart`. Add `IsRoundStart(slot) bool`;
  step 4's successor needs it for the Goldfish gate.

### Config

| file | change |
|---|---|
| `config/params/config.go` | add `SlotsPerRound primitives.Slot` beside `SlotsPerEpoch` |
| `config/params/mainnet_config.go` | `SlotsPerRound: 32` |
| `config/params/minimal_config.go` | `SlotsPerRound: 8` |
| `config/params/loader.go` | add to the print list |
| `config/params/loader_test.go` | add to the assert list |
| `beacon-chain/rpc/eth/config/handlers.go` + test | add `SLOTS_PER_ROUND` |

Add one validation: `SlotsPerRound` must be non-zero and must divide
`SlotsPerEpoch`. The spec requires it. Put it next to `VerifyPreset`'s
caller in `beacon-chain/node/node.go:336`, or in the config loader — either
is fine, as long as a bad value is a loud startup failure.

### Behaviour

Two sites, both in `beacon-chain/core/helpers/beacon_committee.go`:

- `SlotCommitteeCount:55` — divide the active count by `SlotsPerRound`
  instead of `SlotsPerEpoch`. Spec: `beacon-chain.md:1161`.
- `BeaconCommittee:242` — the index offset uses slot-within-round instead of
  `slot.ModSlot(SlotsPerEpoch)`, and `count` multiplies by `SlotsPerRound`.
  Spec: `beacon-chain.md:1183`.

The committee cache is keyed by `(slot, seed, index)` and needs no change.

**Trap.** `hasSeenAggregatorIndexEpoch`
(`beacon-chain/sync/validate_aggregate_proof.go:259`) keys the aggregator
dedup on the **epoch**. With 4 rounds per epoch a validator can aggregate in
more than one round, and the later aggregates get dropped as duplicates.
Change that key to the round. The unaggregated cache is already keyed by
`(slot, committee index, attester)`
(`validate_beacon_attestation.go:457`) and is fine as is.

Also check `CommitteeAssignments` in the same package: duty enumeration is
per-epoch today and becomes per-round. Spec: `validator.md:228`.

### Accept

- Full test suite green with no expectation edits, because every shipped
  config has `SlotsPerRound == SlotsPerEpoch`.
- New unit tests with `SlotsPerRound = 8` and `SlotsPerEpoch = 32` show the
  round's 8 slots partition the whole active set, with no validator in two
  slots of one round and none missing.

### Executed 2026-08-19 <added by executor>

Done in three jj changes (`lssoxmyt`, `oltyqmro`, `lomukltk`). The plan's
"committee cache needs no change" claim was wrong: the cache *contents*
(`CommitteeCount`, and the offset/committees-per-slot math in
`cache/committee.go`) embed `SlotsPerEpoch` and had to move to
`SlotsPerRound`. `CommitteeAssignments` keeps its epoch-shaped API and
enumerates only the epoch's first round — correct committees, but the duty
API under-reports attester slots once a config uses a short round; step 5
must address it. Full deviation list in `plan-detailed.md` 2.8.

---

## Step 3: Heze owns the state container

Copy `BeaconStateGloas` to `BeaconStateHeze`. Nothing in the container
changes yet; ownership is what we need, so later steps can diverge it.

### Blocks stay Gloas-shaped

This is deliberate and it is expressible, contrary to what `task.md`
decision 11 assumed. The shape rule splits cleanly by consumer:

- **State**: `encoding/ssz/detect/configfork.go` is the only place that maps
  a fork version to a state container.
- **Wire objects**: `beacon-chain/p2p/types/object_mapping.go` and
  `beacon-chain/sync/context.go` map the Heze *digest* to block, metadata,
  attestation, slashing, light-client and column containers.

So state becomes Heze while wire objects stay Gloas, and each consumer says
so directly. `HezeShape()` is deleted because with `GloasForkEpoch = 0` its
answer is the constant Gloas — and its own code has an
`if b.HezeForkEpoch == 0` branch commented "not a supported setup", which is
exactly the setup we are building.

### Proto

Add `BeaconStateHeze` to `proto/prysm/v1alpha1/heze.proto`, a field-for-field
copy of `BeaconStateGloas` (`gloas.proto:260-392`). Keep the field numbers
and the `ssz_size` / `ssz_max` option strings identical. Wire it into
`proto/prysm/v1alpha1/BUILD.bazel` next to the Gloas state, then
`make gen proto ssz mode=force`.

### state-native

Six files, about 98 non-test mentions of Gloas:

| file | change |
|---|---|
| `state_trie.go:136` | add `hezeFields`, copied from `gloasFields` |
| `state_trie.go:203,814` | `InitializeFromProtoHeze`, `InitializeFromProtoUnsafeHeze` |
| `state_trie.go:888,1148` | add the `hezeFields` loops |
| `state_trie.go:946,1049,1206` | add `case version.Heze` arms |
| `getters_state.go:260,558` | add `case version.Heze` (`ToProto`, `ToProtoUnsafe`) |
| `hasher.go:47` | add `case version.Heze` |
| `beacon_state.go:74` | the Gloas field block is shared; confirm no version gate is needed |
| `getters_gloas.go`, `setters_gloas.go` | no change — they gate on `b.version < version.Gloas`, and Heze sorts above Gloas |

Add `BeaconStateHezeFieldCount: 46` to `config/params/config.go:184` and
`mainnet_config.go:230`, matching Gloas.

### Version plumbing

| file | change |
|---|---|
| `runtime/version/fork.go:40` | remove **both** `Gloas` and `Heze` from `unsupportedVersions`; genesis at Heze needs both selectable |
| `encoding/ssz/detect/configfork.go:96` | `case cfg.HezeForkVersion: fork = version.Heze` |
| `configfork.go:182` | add `case version.Heze` → `BeaconStateHeze` + `InitializeFromProtoUnsafeHeze` |
| `configfork.go:246,287` | add `case version.Heze` → `SignedBeaconBlockGloas`, because blocks keep the Gloas shape |
| `beacon-chain/db/kv/state.go:782`, `state_diff_helpers.go:345` | add `case version.Heze` |
| `config/params/fork.go:52-64` | delete `HezeShape` |
| `config/params/heze_shape_test.go` | delete |
| `p2p/types/object_mapping.go:272-289` | `aliasHezeEntries` keeps its job; source the shape from `cfg.GloasForkVersion` instead of `HezeShape()`. Restate the comment: Heze reuses Gloas *wire* containers, while the state is its own. |
| `beacon-chain/sync/context.go:98` | same: map `version.Heze` to `version.Gloas` for context bytes |

Delete `beacon-chain/core/heze/upgrade.go` (`UpgradeToHeze`) and its test.
Leave `gloas.UpgradeToGloas` in place, uncalled, so upstream rebases stay
possible. The 20 `case version.Gloas` sites in `consensus-types/blocks` need
no change, because blocks stay Gloas.

`b.version < version.Gloas` style ladders all keep working, since Heze is
above Gloas in the enum. There are 0 `== version.Gloas` sites.

### Accept

- SSZ round trip and `HashTreeRoot` for `BeaconStateHeze` match
  `BeaconStateGloas` for the same field values.
- `detect` resolves the Heze fork version to a Heze state and a Gloas block.
- No reference to `HezeShape` remains.

### Executed 2026-08-19 <added by executor>

Done in four jj changes (`lzsxwrmr`, `mkonswyn`, `xzqrwxwy`, `krxuztzm`),
all acceptance criteria green. Biggest surprises: the db/kv **read** side
(schema key, `unmarshalState`, `decodeStateSnapshot`) was missing from the
plan; `NewGenesisBlockForState` needed a Heze arm (step 4 depends on it);
and unhiding Gloas/Heze in `version.All()` woke nine fork-walking test
suites — including exposing that the REST validator client has **no SSZ
block codec for Gloas/Heze** (falls back to Fulu; steps 4/5 must use gRPC or
fix it). Full list in `plan-detailed.md` 3.8.

---

## Step 4: genesis is a Heze state

`runtime/interop/premine-state.go` builds the genesis state. There is **no
Gloas arm anywhere in it** — all its switches stop at Fulu, so
Gloas-shaped genesis has never been written. There is nothing to copy.

`task.md` says four switches. It is **five**. The fifth is
`setExecutionPayload`, and it holds the only real work.

| line | function | Heze arm |
|---|---|---|
| 68 | `prepare` | add `version.Heze` to the allowed list |
| 105 | `empty` | `InitializeFromProtoUnsafeHeze(&ethpb.BeaconStateHeze{DepositRequestsStartIndex: ...UnsetDepositRequestsStartIndex})` |
| 343 | `setFork` | `pv, cv = HezeForkVersion, HezeForkVersion` — genesis has no predecessor |
| 443 | `setLatestBlockHeader` | body is `BeaconBlockBodyGloas` (`gloas.proto:207`), zero-valued, since blocks keep the Gloas shape |
| 628 | `setExecutionPayload` | see below |

### The Gloas state fields

Eight fields exist in the Gloas state and in no earlier one
(`gloas.proto:373-390`). Six are cheap:

- `ptc_window` ← `initializePTCWindow` (`beacon-chain/core/gloas/upgrade.go:156`).
  It is unexported today; export it. Its own spec docstring says it is "used
  to initialize the `ptc_window` field in the beacon state **at genesis** and
  after forks", so genesis is its intended second caller.
- `builders` ← `OnboardBuildersFromPendingDeposits()`, already an exported
  state method. Call it after deposits are processed.
- `builder_pending_payments` — a **fixed vector** of `2 * SLOTS_PER_EPOCH`.
  Allocate it with zero-valued entries. A nil slice fails the SSZ round trip.
- `execution_payload_availability` — a zeroed bitvector, then set **bit 0 to
  1**. Slot 0 has a payload, the slot-1 transition would set that bit anyway
  (`gloas/beacon-chain.md:1169`), and `beacon-chain.md:1395` asserts
  `latest_block_hash == latest_execution_payload_bid.block_hash`, which only
  holds at genesis if slot 0 counts as available.
- `builder_pending_withdrawals`, `payload_expected_withdrawals` — empty lists.
- `next_withdrawal_builder_index` — 0.

So the arm calls the two helpers that `UpgradeToGloas` calls, without calling
`UpgradeToGloas` itself.

### The real work: `setExecutionPayload`

Gloas removes `latest_execution_payload_header` and replaces it with
`latest_block_hash` (`gloas.proto:325`) and `latest_execution_payload_bid`
(`:380`). The existing code builds an `ExecutionPayloadDeneb` from the geth
genesis block `s.GB` and wraps it as a header (`premine-state.go:628-745`).

The Heze arm uses the same `s.GB` inputs to fill `ExecutionPayloadBid`
(`gloas.proto:38-67`) instead:

| bid field | source |
|---|---|
| `parent_block_hash` | `gb.ParentHash()` |
| `parent_block_root` | zero at genesis |
| `block_hash` | `gb.Hash()` — the same value goes in `latest_block_hash` |
| `prev_randao` | `params.BeaconConfig().ZeroHash` |
| `fee_recipient` | `gb.Coinbase()` |
| `gas_limit` | `gb.GasLimit()` |
| `builder_index`, `slot`, `value`, `execution_payment` | 0 |
| `blob_kzg_commitments` | empty |
| `execution_requests_root` | zero root |

`beacon-chain/core/gloas/upgrade.go:207` and `:312` show the field mapping
the fork path uses. Mirror it, but read from the genesis block rather than
from a pre-fork header.

### Config

- Set `GloasForkEpoch = 0` and `HezeForkEpoch = 0` in the devnet and e2e
  configs. Re-read "The one thing not to get wrong" above.
- `--fork-name heze` works once step 3 removes Heze from
  `unsupportedVersions`; `cmd/prysmctl/testnet/generate_genesis.go:137`
  builds its list from `version.All()`.
- `prysmctl testnet generate-genesis` takes the genesis time from the input
  geth genesis JSON `timestamp`, **not** the wall clock. Pass
  `--genesis-time=$(date +%s)`.

### Accept

- `prysmctl testnet generate-genesis --fork-name heze` produces a state that
  round-trips through SSZ and whose `Version()` is `version.Heze`.
- A single node starts on that genesis, and its state at slot 0 reports the
  Heze fork version.
- The chain produces and processes blocks past slot 1, which exercises
  `process_parent_execution_payload` against the genesis bid.

---

## Step 5: verify

### e2e

`testing/endtoend/` currently has `TestEndToEnd_DecoupledHezeConfig`, deleted
in step 1. Write its replacement: genesis at Heze on the mainnet preset, no
fork transition at all.

Assert on fewer epochs rather than extending the run. Epochs are 32 slots
again, so an epoch takes 4 times as long as it did under the 8-slot preset.
The e2e config runs 6-second slots (`config/params/testnet_e2e_config.go:42`),
so an epoch is 3.2 minutes.

Run **4 epochs**, about 13 minutes. That is enough for finalization, which
needs roughly 3 epochs at any round length. The old decoupled run did 10
epochs in 8 minutes, so this is fewer epochs for a modestly longer run.

Do **not** shrink `SECONDS_PER_SLOT` to compensate. Slot duration is a
realism parameter for a network-cost study.

One round is 8 slots, so the committee partition and "a Heze genesis
produces and processes blocks" are both provable inside the first round.
Only finalization needs the epochs.

### Shadow

**There is no devnet preset.** A preset is compile-time and fixes SSZ array
sizes; `SLOTS_PER_ROUND` sizes nothing, and `SLOTS_PER_EPOCH` stays 32, so
every array keeps its mainnet length. One mainnet-preset binary runs e2e,
the devnet, and Shadow. The devnet's differences are all runtime yaml:

```yaml
SLOTS_PER_ROUND: 8
GLOAS_FORK_EPOCH: 0
HEZE_FORK_EPOCH: 0
```

`VerifyPreset` passes, because the yaml keeps `SLOTS_PER_EPOCH = 32`. Under
the old preset the yaml had 8-slot epochs and so demanded a `-tags=decoupled`
binary.

Flip the sim config to `SLOTS_PER_ROUND = 8` here, not earlier. Expect roughly 4 times the per-slot attestation traffic
of a mainnet-shaped run: at 8-slot rounds each validator attests once per
round, so 4 times per epoch. That is the intended result, not a regression.

`ethshadow` has no heze knob, so `HEZE_FORK_EPOCH` goes through `sim.yaml`
`extra_env`. The genesis-generator image strips the heze→bogota EL mapping,
because Heze is CL-only; see `Dockerfile.genesis-gen`.

### Check

- All nodes finalize at cadence.
- Available attestations still reach every node: 64 per slot per node.
- Record per-slot attester counts and per-slot bytes on the attestation
  topics. This is the baseline the later Goldfish work is measured against.

---

## Open questions for the user

1. `builder_pending_payments` is sized `2 * SLOTS_PER_EPOCH` and stays
   epoch-sized. Confirm nothing in the Gloas payment path assumes the round.
