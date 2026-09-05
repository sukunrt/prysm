# Bounded full-gossip / real DomainData fallback design

This design has now been implemented and exercised; see the
[full-gossip results](full-gossip-domain-realwork-results.md). It followed the
negative checkpoint/count/cold-copy/singleton compositions. A single real
blockchain service plus a lightweight libp2p publisher exercises the missing
gossip stages; three deployed beacon nodes are not required.

## Test-package layout and the import cycle

Put fixture construction and access to private sync methods in a new
`beacon-chain/sync/*_test.go` file with `package sync`. Export a diagnostic
fixture constructor from that test file. Put TCP DomainData orchestration in
another test file with `package sync_test`.

The external test can import the augmented sync package, the real blockchain
package, and `rpc/prysm/v1alpha1/validator`. The last package itself imports
sync, so importing it from an internal `package sync` test would introduce a
cycle. The existing blockchain-package test export cannot be imported as a
dependency of sync's tests: dependency test files are not compiled. This design
needs no production export and no dependency on that existing test export.

## Public construction of the real blockchain service

Use ordinary constructors rather than `blockchain/testing.ChainService`:

1. Prepare a valid native Heze genesis state at slot zero and register it with
   `genesis.StoreStateDuringTest(t, st)` (`genesis/testing.go:32`). This step is
   essential: `db/kv.Store.GenesisState` reads the process-global genesis
   provider, not the state just written to Bolt (`db/kv/state.go:86`).
2. Create the real test DB with `db/testing.SetupDB(t)` and call its public
   `SaveGenesisData(ctx, st)` (`db/kv/genesis.go:18`). This writes the real
   genesis block, block root, state, state summary and head/genesis root keys.
   Do not substitute an all-zero block root.
3. Create `doublylinkedtree.New()`, `stategen.New(db, fc)`,
   `startup.NewClockSynchronizer()`, the real attestation pool and
   `attestations.NewService(ctx, &attestations.Config{Pool: pool})`.
4. Construct `blockchain.NewService` with public `WithDatabase`, `WithStateGen`,
   `WithForkChoiceStore`, `WithClockSynchronizer`, `WithAttestationPool`,
   `WithAttestationService`, and `WithFinalizedStateAtStartUp`. Supply any
   incidental notifier/cache options reached by the chosen probes using the
   existing `blockchain/setup_test.go:166` as the construction reference.
5. Call public `StartFromSavedState(st)` rather than `Start`. It calls real
   state-generator `Resume`, builds fork choice, initializes head and sets the
   shared clock (`blockchain/service.go:266`). The default finalized checkpoint
   at root zero makes `Resume` load/register genesis in its real hot and epoch
   caches. `LastValidatedCheckpoint` resolves zero to the saved genesis block
   root. Assert one known canonical head, head slot zero, non-optimistic status,
   and successful `TargetRootForRound(genesisRoot, 0)`.
6. Warm the actual `AttestationTargetState(ctx, cp0)` method once; assert slot
   zero, 120,000 validators, and the expected active count. This fills its real
   checkpoint cache through the production cold path. Do not directly insert
   a test checkpoint state. Head, stategen and checkpoint copies then share the
   native validator slice through ordinary `Copy` semantics.

Calling `StartFromSavedState` alone intentionally omits periodic `UpdateHead`
and late-block tasks started by `Start`. This first composition tests the
gossip pipeline, not every BN background service. It still includes real
per-vote fork-choice locking and real checkpoint retrieval. Adding unrelated
background services before this result would make a positive harder to localize.

## Correct fixture and startup timing

Prefer the already retained 120k-valid-key SSZ fixture and its corresponding
local mnemonic file, using the decoding/key derivation in
`runs/diagnostics/startup3/ffgsource/main.go:135,210` as the reference. Read key
material only for signing; do not emit it in diagnostic output. Retain the
configured eight-slot rounds and assert six committees of 2,500 for slot one.

Prepare 15,000 distinct `SingleAttestation` messages, one signer at each seat
of those six committees. Use the actual generated genesis block root as both
`BeaconBlockRoot` and round-zero target root, source round zero/root zero, and
Gloas data index **1** for the full genesis-parent view. Sign this exact data
with the actual state fork/genesis-validator-root domain. The gossip wire type
is `SingleAttestation`; production validation performs the conversion to a
one-bit Electra attestation. One payload-data index is not six committee IDs.

After registering genesis, mirror `node/node.go:209–211`: apply
`params.WithGenesisValidatorsRoot(genesis.ValidatorsRoot())` to the active
config and call `InitializeForkSchedule`. A correct clock alone is not enough:
gossip decoding resolves the topic digest through the active config's digest
schedule (`sync/decode_pubsub.go:122`).

Do not reuse the old fixture's zero roots/index zero, its state advanced to
slot one, or its post-initialization validator-key mutations. In particular,
updating 15,000 keys on an already initialized native state changes the
multi-value slice's individual-item map and the cost of its later copies.
If a synthetic fixture is necessary, populate keys in the proto first, then
initialize a fresh native state so the unchanged registry is the shared array.

Arrange a fresh real genesis/start time after expensive key preparation, with
enough bounded setup time for SSZ/DB/hash/cache work and pubsub connection
readiness. If changing the fixture's genesis timestamp, calculate and sign
against its resulting genesis block root. Do not feed yesterday's slot-one
attestations through a frozen clock: time validation also reads real time.
Keep the actual head and checkpoint state at slot zero.

Warm committee and signer-public-key caches before timed work, reflecting the
historical pre-genesis startup. `StateGen.Resume` also launches public-key
cache population; ensure this initialization work has finished before counting
it as the offered gossip load. Run arms in separate test processes so global
genesis/cache/config state does not leak between arms.

## The real pubsub and signature pipeline

`p2p/testing.NewTestP2PWithPubsubOptions` provides a real libp2p host and
`pubsub.NewGossipSub` (`p2p/testing/p2p.go:80`). Use one persistent receiver and
one persistent sender, and connect once. Apply production message-ID, signature
policy, queue and maximum-message-size options; retain the library's default
1,024-per-topic and 8,192-global validation throttles. The CLI validation and
outbound queue default is 1,000 (`cmd/flags.go:188`); distinguish it from the
separate p2p test/default-config value. Prejoin all six exact digest/subnet
topics, verify the remote subscription is visible, then publish pre-encoded
messages with the same finite two-second offered cadence.

Do not call `TestP2P.ReceivePubSub` per vote: it creates a new host and waits
100 ms for each call (`p2p/testing/p2p.go:175`). A persistent sender can call
its joined topic's `Publish` directly. Local publication on the receiver would
also be wrong: Prysm accepts its own peer ID before validation.

Construct the real sync service with `NewService`, the actual chain/DB/pool,
an initial-sync checker reporting complete, a valid clock and operation
notifier. From the internal test helper, start `verifierRoutine`, mark chain
start, and call `subscribeWithBase` for each topic with the real
`validateCommitteeIndexBeaconAttestation` and
`committeeIndexBeaconAttestationSubscriber` methods. This preserves pubsub's
async validation and production subscriber goroutine creation instead of a
64-worker loop calling validation and storage back-to-back.

Use `WithBatchVerifierLimit(1000)`. Both current and historical revision
`0280403` give that CLI default (`cmd/beacon-chain/flags/base.go:350`); the
old full-gossip benchmark instead hardcodes 64. The production verifier has a
5 ms flush ticker, a signature queue sized to that limit, and an **unbuffered
result channel per waiting validation** (`sync/batch_verifier.go:17,59`). It
aggregates compatible signature sets, verifies them, and then sends each
result synchronously. These parent wakeups and queue handoffs are additional
real scheduling edges beyond the already tested singleton public-key stage.

The full path now includes SSZ/snappy decode, seen/root checks, DB/state-summary
checks, real fork-choice consistency, real checkpoint/count work, committee
conversion, signature preparation and batch verification, operation feed,
seen-cache update, and legacy pool storage. Drain an operation-feed subscriber
without deliberate delay, and account separately for accepted gossip and
completed subscriber storage. Real pubsub may reject/throttle some offered
messages; record those outcomes rather than forcing all 15,000 past admission.

## Measurements and a focused success criterion

Return the real chain pointer and bounded counters from the exported test
fixture. The external `sync_test` file hosts the ordinary production
`rpcvalidator.Server.DomainData` over localhost TCP and reuses the separate
process/client timing approach from the existing Domain diagnostic. Keep its
handler body unchanged. The lightweight publisher is not another blockchain
service; if it shares the receiver process, explicitly retain that modest
offering/transport overhead as a fixture limitation.

A test-only chain wrapper may embed `*blockchain.Service`, override only
`AttestationTargetState` to call the real method, and wrap the returned
read-only state's iterator with one start/end counter. Forward the entire
underlying iterator call; do not add an atomic per validator. This measures
active registry iterations without modifying state slots, lock ordering,
validator data or the 120k inner loop. It still cannot distinguish a parked
multi-value-slice reader from a computing reader without a targeted follow-up.

The first control can remain entirely in test code: return a state wrapper
whose iterator traverses a precomputed snapshot of the exact checkpoint's
120,000 read-only validators. All other methods still delegate to the real
state, and checkpoint retrieval still uses the production method. This
preserves slot zero and the full count traversal while removing native registry
access and its shared multi-value-slice locks. It is an **MVS access ablation**,
not a scalar count or cache-fix ablation. A positive result isolates that access
cost; a null does not establish that the full count traversal is harmless.

There is a crucial snapshot trap: `ValidatorsReadOnlySeq` reuses one
`readOnlyValidator` pointer, replacing its `validator` field for each yield
(`state-native/getters_validator.go:239–251`). Appending those yielded interfaces
would produce 120,000 aliases to the final record. Obtain stable wrappers with
`ValidatorAtIndexReadOnly(index)` for each index outside the timed workload;
that method calls `NewValidatorFromCompact` separately. Verify actual per-index
public keys and active-count equality before load. The snapshot is valid only
for this immutable slot-zero checkpoint. Native head-state copies and their
registry writer path must remain real in both arms.

Keep two quantities separate: general Domain admission/return latency under
load, and the node169-specific sequence. The latter needs a successful sync
preflight followed by its own DomainData call with a measured live budget.
The historical role-progress marker has about 2.085 seconds remaining but
does not timestamp that RPC's invocation. A preflight that alone exhausts 12 seconds
reproduces an already explained failure mode. Do not fabricate a successful
historical timing by sleeping inside a handler or retaining a lock.

If this composition is positive, an additional narrow count counterfactual can use the
existing diagnostic source change in a Go build overlay, changing only the
genesis count-cache guard while keeping production files on disk untouched.
Do not implement the memoized arm by advancing the checkpoint state to slot
one or bypassing real checkpoint retrieval. Retain the overlay and its exact
diff as part of the result. Initial null/positive measurements should run
without goroutine-profile capture or runtime tracing; instrument a reproduced
positive separately.
