# Spec: per-slot summary log lines

## Goal

Full-scale simulations must not run with `--goldfish-vote-ledger`. The
ledger writes one line per vote, per aggregate, per included attestation.
This spec adds four summary lines per slot that carry the numbers the
run tooling needs, and nothing else.

Summary lines are always on at `Info` once Goldfish is active. No new
flag. Volume is bounded: at most four lines per slot, plus one per extra
block or payload.

The per-vote ledger lines keep their gate and are unchanged.

## Shared helper

`decoupled/vote_ledger.go` gains three exports. Every summary line goes
through them so the values cannot drift between call sites.

| export | meaning |
|---|---|
| `SummaryPurpose = "goldfish-summary"` | the value of the `purpose` field |
| `SummaryFields(slot) logrus.Fields` | `{"purpose": SummaryPurpose, "slot": slot}` |
| `SummaryActive(slot) bool` | `slots.ToEpoch(slot) >= HezeForkEpoch` |
| `SummaryRoot(root [32]byte) string` | `fmt.Sprintf("%#x", root[:4])`, so `0x1a2b3c4d` |

`SummaryRoot` is 4 bytes with the `0x` prefix. It is not
`VoteLedgerRootPrefix`, which returns 8 hex characters without a prefix
for the ledger lines. Block and payload lines both use `SummaryRoot`, so
the two can be joined on `blockRoot`.

Gating: lines 1 and 2 are gated by `SummaryActive`. Lines 3 and 4 are
not. Line 3 replaces a line that already fired for every block, and line
4 exists only from Gloas on. Heze follows Gloas, so from Heze all four
are on.

## Lines

### 1. `Goldfish votes` — at slot start

Emitted once per slot from `goldfishNewSlot`
(`forkchoice/doubly-linked-tree/goldfish.go`), next to the
`goldfishSeatFraction` metric, before `prune`. That runs from `NewSlot`
at the slot boundary, so the line reports the slot that just ended and a
vote that arrives after the tick is not counted. `goldfishNewSlot` is
already reached only when Goldfish is active.

One gossip message carries one validator's seats, so `votes` is the
number of validators heard from. Add `voters(slot) uint64` on
`goldfishVotes`, returning `len(g.votes[slot])`. The name is `voters`
because `votes` is already the field.

| field | type | meaning |
|---|---|---|
| `slot` | Slot | the slot that ended |
| `votes` | uint64 | validators with a vote in the store |
| `seats` | uint64 | seats those votes cover (sum of aggregation bits) |
| `committeeSeats` | uint64 | `decoupled.AvailableAttestationCommitteeSize` |

```
Goldfish votes  purpose=goldfish-summary slot=1234 votes=120 seats=497
  committeeSeats=512
```

An empty slot still writes the line with zeros. Dropped and queued votes
are not in this line. `goldfish_vote_drop_total` already counts them by
reason.

### 2. `FFG votes` — at aggregation due

Emitted once per slot from a new routine in the sync service. Ticker:
`slots.NewSlotTickerWithOffset(genesis,
SlotComponentDuration(AggregateDueBPSGloas), SecondsPerSlot)`. Only the
Gloas variant is needed: the line is gated by `SummaryActive`, and Heze
follows Gloas. The existing `prepareForkChoiceAtts` ticker is not
reused: its intervals (7000/9500/11800 ms) are unrelated to the
aggregation deadline.

Start the routine from `startDiscoveryAndSubscriptions`, after
`waitForChainStart` has set `cfg.clock`. `Start()` runs before the clock
exists and `NewSlotTicker*` panics on a zero genesis time.

Counting: `validateUnaggregatedAttTopic` computes the subnet and drops
it. Change it to return the subnet. At the accept point of
`validateCommitteeIndexBeaconAttestation` call `s.ffgVotes.count(slot,
subnet, seats)`, gated by `SummaryActive(att.Data.Slot)`; nothing drains
the counters before Heze. Keyed by `(att.Data.Slot, subnet)`. Only votes
whose `Data.Slot` equals the current slot go into the current line.
Votes that arrive after the tick are not counted. Counters for `slot-1`
are dropped when the line for `slot` is written.

The counter store is a new `ffgVotes` field on `Service`, initialised in
`NewService`, in a new file `sync/ffg_summary.go` with the routine and
the line.

Subnets listed are the ones the node holds a subscription for at the
tick: `subHandler.allTopics()` filtered to the attestation subnet topic
format. A subscribed subnet with zero votes appears with `0`.

| field | type | meaning |
|---|---|---|
| `slot` | Slot | the current slot |
| `subnets` | int | subscribed attestation subnets |
| `votes` | uint64 | messages accepted across those subnets |
| `seats` | uint64 | attesting bits across those messages |
| `perSubnet` | string | `subnet:votes` pairs, comma separated, sorted by subnet |

```
FFG votes  purpose=goldfish-summary slot=1234 subnets=2 votes=241 seats=241
  perSubnet=3:120,17:121
```

The aggregate topic is out of scope. Whether the node is an aggregator
does not change the line.

### 3. `Block received` — at gossip receipt

Replaces the `Debug("Received block")` line at the end of
`validateBeaconBlockPubSub` (`sync/validate_beacon_blocks.go`). Extract
the line into `logBlockReceived(blk, root, arrived, validation
time.Duration)` so a unit test can call it. The `Warn("Received block,
could not report timing information.")` arm stays as is. The `graffiti`
field is dropped.

Written after decode and validation, so a rejected block has no summary
line. Gossip only: the node's own proposal enters over RPC and gets no
`arrivedMs`. `Synced new block` already covers the import for every
route.

| field | type | meaning |
|---|---|---|
| `slot` | Slot | block slot |
| `blockRoot` | string | `SummaryRoot` |
| `proposerIndex` | ValidatorIndex | |
| `arrivedMs` | int64 | `receivedTime - slot start`, milliseconds |
| `validationMs` | int64 | validation duration |
| `bytes` | int | SSZ size, `SignedBeaconBlock.SizeSSZ()` |
| `attestations` | int | `len(Body().Attestations())` |
| `ffgSeats` | uint64 | sum of `AggregationBits().Count()` over all attestations |

```
Block received  purpose=goldfish-summary slot=1234 blockRoot=0x1a2b3c4d
  proposerIndex=77 arrivedMs=412 validationMs=9 bytes=118204 attestations=2
  ffgSeats=239
```

`ffgSeats` is the bit sum. A validator holds one position per slot by
design, so the bit sum is the validator count. No state lookup needed.

`bytes` is the SSZ size, same basis as `payloadBytes` on the payload
line. The compressed wire size (`len(msg.Data)`) is not logged.

### 4. `Payload received` — at receipt

The existing `Payload envelope` line in `logPayloadEnvelope`
(`blockchain/receive_execution_payload_envelope.go`) already carries
every field needed and is written once per envelope across all routes.
Changes: rename to `Payload received`, remove the ledger gate, build the
fields on `SummaryFields`, format `blockRoot` with `SummaryRoot`. Keep
every other field, `gasUsed` included.

| field | type | meaning |
|---|---|---|
| `slot` | Slot | |
| `blockRoot` | string | `SummaryRoot` |
| `builderIndex` | BuilderIndex | max uint64 on a self-built payload |
| `arrivedMs` | int64 | milliseconds into the slot |
| `payloadBytes` | int | `payload.SizeSSZ()` |
| `txCount` | int | |
| `gasUsed` | uint64 | |
| `blobCount` | int | from the bid, when present |

```
Payload received  purpose=goldfish-summary slot=1234 blockRoot=0x1a2b3c4d
  builderIndex=5 arrivedMs=1980 payloadBytes=402113 txCount=133 gasUsed=9981234
  blobCount=3
```

## Conventions

- Every summary line carries `purpose=goldfish-summary`, from
  `SummaryFields`. One `grep` pulls all four lines out of a node's log.
  No other line uses this key.
- Time fields are `int64` milliseconds into the slot the message belongs
  to, named `*Ms`, computed from `slots.UnsafeStartTime`. No
  `time.Duration` fields: the tooling parses integers.
- Roots use `SummaryRoot`.
- Message strings are fixed and unique so `grep` isolates each line.

## Changes by file

| file | change |
|---|---|
| `decoupled/vote_ledger.go` | `SummaryPurpose`, `SummaryFields`, `SummaryActive`, `SummaryRoot` |
| `forkchoice/doubly-linked-tree/goldfish.go` | `voters(slot)`; `goldfishNewSlot` logs `Goldfish votes` |
| `sync/ffg_summary.go` (new) | counter store, the aggregation-due routine, the `FFG votes` line |
| `sync/service.go` | `ffgVotes` field, init in `NewService`, start routine in `startDiscoveryAndSubscriptions` |
| `sync/validate_beacon_attestation.go` | `validateUnaggregatedAttTopic` returns the subnet; count at the accept point |
| `sync/validate_beacon_attestation_test.go` | follow the signature change |
| `sync/validate_beacon_blocks.go` | extract `logBlockReceived`, replace `Received block` |
| `blockchain/receive_execution_payload_envelope.go` | ungate, rename, `SummaryFields`, `SummaryRoot` |
| `blockchain/receive_execution_payload_envelope_test.go` | rename `TestLogPayloadEnvelope_QuietUnlessTheLedgerIsOn` to `..._WrittenWithoutTheLedger`; drop the `features` import |
| `testing/endtoend/types/types.go` | `WithGoldfishVoteLedger()` |
| `testing/endtoend/evaluators/summary_log.go` (new) | the evaluator |
| `testing/endtoend/heze_e2e_test.go` | the new run; add the evaluator to `TestEndToEnd_HezeGenesis` |
| `BUILD.bazel` in `decoupled`, `blockchain`, `forkchoice/doubly-linked-tree`, `sync`, `endtoend/types`, `endtoend/evaluators` | new deps and srcs |

Run `bazel run //:gazelle -- fix` for the BUILD files. It also reports
unrelated drift (a `//time` dep in `validator/client/BUILD.bazel`).
Revert anything outside the files above.

## Tests

- `goldfish_test.go`: `TestGoldfishNewSlot_WritesTheSummaryLine`. Log
  hook, one `Goldfish votes` line per `NewSlot`, `votes` and `seats`
  match the inserted store, an empty slot writes zeros. Needs the
  `logrus/hooks/test` dep in the forkchoice BUILD.
- `ffg_summary_test.go`: counters keyed by slot and subnet; a vote for
  `slot-1` does not land in `slot`; counters for `slot-1` are gone after
  the tick; a subscribed subnet with no votes is listed with `0`.
- `validate_beacon_blocks_test.go`: `TestLogBlockReceived` calls
  `logBlockReceived` on a block with two attestations and asserts every
  field. There is no existing test of the old Debug line.
- `receive_execution_payload_envelope_test.go`: the renamed test asserts
  `Payload received` is written with the ledger flag off.
- Every line test asserts `purpose` equals `SummaryPurpose`.
- Unit test commands: `go test ./decoupled/ ./beacon-chain/sync/
  ./beacon-chain/forkchoice/doubly-linked-tree/
  ./testing/endtoend/evaluators/` and `go test -tags develop
  ./beacon-chain/blockchain/`. `blockchain` has two pre-existing
  failures, `TestStore_NoViableHead_NewPayload` and
  `TestNoViableHead_Reboot`, both `want 2 (Round), got 0`. Leave them.

## E2E test

Purpose: one three-minute run that proves every summary line is written,
parses, and agrees with the ledger. Shape is `TestEndToEnd_HezeGenesisShort`
(`heze_e2e_test.go`): Heze at genesis, mainnet preset, 8-slot rounds,
one epoch, no sync or deposits.

### Run

`TestEndToEnd_HezeGenesisSummaryLog`:

- Same config and options as the Short run.
- `types.WithGoldfishVoteLedger()`: appends
  `"--" + features.GoldfishVoteLedger.Name` to `cfg.BeaconFlags`, the
  `WithStateDiff` pattern. `WithSlotStartFFGVote` is not a model: it sets
  a validator-side config bool. This pulls a `config/features` dep into
  `testing/endtoend/types`.
- Evaluators: `ChainProducesBlocks`, `AvailableAttestationsFlow`, and the
  new `SummaryLogLines`.

The evaluator also joins the full `TestEndToEnd_HezeGenesis` run with the
ledger off. There it only runs the shape checks below, so it proves the
lines are on without the flag. That run is already red on
`justification_advances_every_round_4` (`rounds [2 2 3 3 3]` over
boundaries `[96 104 112 120 128]`) at the base change, with no summary
work applied. Acceptance for this spec is `summary_log_lines_0..4` all
passing there. Do not chase the finality failure.

Commands, from the repo root:

```
bazelisk test //testing/endtoend:go_heze_test \
  --test_filter='TestEndToEnd_HezeGenesisSummaryLog' \
  --test_output=streamed --nocache_test_results --flaky_test_attempts=1
```

Same for `TestEndToEnd_HezeGenesisShort$` and `TestEndToEnd_HezeGenesis$`.
Node logs land in `bazel-testlogs/testing/endtoend/go_heze_test/test.outputs/outputs.zip`.

### Evaluator `SummaryLogLines`

`evaluators/summary_log.go`, policy `AllEpochs`, name
`summary_log_lines_%d`. Runs per beacon node on
`path.Join(e2e.TestParams.LogPath, fmt.Sprintf(e2e.BeaconNodeLogFileName, i))`.
The evaluator opens its own handle; `helpers.LogOutput` opens, reads and
closes its own and shares nothing.

Log format, verified: the file is written by
`logs.ConfigurePersistentLogging` with a `TextFormatter` whose
`ForceFormatting` is false, so lines are logfmt with keys sorted,
`msg="..."` quoted, and a `package=...` field the terminal formatter
does not show. Example:

```
time="2026-09-03 11:13:50.13" level=info msg="Block received" arrivedMs=136
  attestations=2 blockRoot=0x7ef99de6 bytes=1616 ffgSeats=45
  package="beacon-chain/sync" proposerIndex=225 purpose=goldfish-summary
  slot=2 validationMs=1
```

Parsing: scan the file once, keep lines that contain
`purpose=goldfish-summary`, split into `key=value` tokens with quote
handling. Ledger lines (`msg="Goldfish vote"`, `msg="FFG vote"`,
`msg="FFG vote included"`) are collected in the same pass when present.
The `logfmt` helper lives in `evaluators/`;
`helpers.FindFollowingTextInFile` splits on spaces and is not enough.

Window: slots `2 .. headSlot-2`, with `headSlot` from `GetChainHead` on
`conns[0]` at evaluation time. Slots 0 and 1 have no full committee
history; the last slot may still be open.

Shape checks, every node, every slot in the window:

| line | check |
|---|---|
| `Goldfish votes` | exactly one line per slot; `0 < seats <= committeeSeats`; `seats >= 2/3 committeeSeats`; `votes <= seats` |
| `FFG votes` | exactly one line per slot; `len(perSubnet) == subnets`; sum of `perSubnet` counts `== votes`; `seats >= votes` |
| `Block received` | at most one per slot; `0 <= arrivedMs < slotMs`; `bytes > 0`; `ffgSeats <= attestations * MaxValidatorsPerCommittee * MaxCommitteesPerSlot` |
| `Payload received` | at most one per slot; `payloadBytes > 0`; when `Block received` exists for the slot on that node, `payload.arrivedMs >= block.arrivedMs` |

Across nodes, for every slot in the window where any node has
`Payload received`: at least `BeaconNodeCount-1` nodes have
`Block received`. The one allowed gap is the proposer, whose block
arrives over RPC. Every node has `Payload received`, because the payload
line is written on every route.

Ledger cross-checks, only when ledger lines are present in the file:

| summary field | must equal |
|---|---|
| `Goldfish votes.votes` | count of `Goldfish vote` lines with `voteSlot=s` and `outcome` in `accepted, replayed, local` |
| `Goldfish votes.seats` | sum of `seats` over the same lines |
| `FFG votes.votes` | count of `FFG vote` lines with `outcome=gossip`, `attSlot=s`, `arrivedMs < dueMs` |
| `Block received.ffgSeats` | sum of `seats` over `FFG vote included` lines with `blockSlot=s` |

`dueMs` is `SlotComponentDuration(AggregateDueBPSGloas)` in
milliseconds. A vote with `arrivedMs` within 50 ms of `dueMs` may land on
either side of the tick, so the `FFG votes` check tolerates a difference
up to the number of such lines.

Failures name the node, the slot, the line, and both values.

### Cost

Same as the Short run: about three minutes. No new components. Log
parsing is one pass per node per evaluation.

## Out of scope

- FFG aggregate topic counts.
- Trimming the ledger lines themselves.
- Run tooling that parses the new lines outside the e2e evaluator.
- The pre-existing `justification_advances_every_round_4` failure.
