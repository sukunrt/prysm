# Real attestation-data writer: source audit and bounded follow-up

The next small composition is one real cold `GetAttestationData` request,
released concurrently with the first RANDAO `DomainData` request after the
fixture's real cold sync-index call returns. This adds a production writer to
the already tested full-gossip pipeline. It is a testable mechanism, not an
identified historical owner of node169's missing response. The completed
`GOMAXPROCS=4` full-gossip comparison takes priority over this extension; neither
a large writer delay nor a large count cohort should be forced into the test.

The source ordering matters. `validator/client/runner.go:147` completes
`RolesAt` synchronously before `performRoles` starts the independent proposer
and attester goroutines (`runner.go:244`). Map iteration does not guarantee
which role runs first. `ProposeBlock` calls RANDAO before block construction;
`SubmitAttestation` calls `getAttestationData` before signing
(`validator/client/propose.go:46`; `attest.go:76`). Consequently a same-slot
attester's copy cannot explain that slot's initial role discovery. It can
overlap the proposer's subsequent domain call.

Node169's PTC result at slot offset **+9.915067414** proves that `RolesAt`
returned **by** that time. It gives no lower bound on role dispatch, attester
invocation, or RANDAO invocation. The attester's jitter is an absolute
slot-start offset (`wait_helpers.go:77`), and the historical explicit jitter
value was not recovered. An experimental release immediately after actual
cold-sync completion preserves the dependency without inventing a historical
dispatch timestamp.

The VC's post-Electra `cachedAttestationDataLock` spans the underlying RPC
(`validator/client/validator.go:804–829`). Successful data is shared across
the slot's attesters. A failed request leaves the cache empty, so later
waiters may make additional **serial** attempts, including fast failures with
the same expired context. The source supports one concurrent underlying
attestation-data request; the 75 logged attester errors do not support 75
concurrent head copies, or prove exactly one total RPC.

The BN cold path in `beacon-chain/rpc/core/validator.go:516` validates the
current slot, takes its attestation-cache write lock, checks optimism, reads
the head and round target, and calls the real `HeadState` at line 596.
`blockchain/chain_info.go:238` holds `headLock.RLock` while `headState`
(`blockchain/head.go:299`) copies the native state. `BeaconState.Copy`
preserves the `validatorsMultiValue` pointer and calls its `Copy`
(`state/state-native/state_trie.go:1106,1157`). That operation takes the shared
slice's write lock (`container/multi-value-slice/multi_value_slice.go:180`),
where count iterators take read locks in `Len` and `At`.

At slot one, both the head and requested target are in round zero. Therefore
this real RPC skips `ProcessSlotsUsingNextSlotCache`; no slot transition,
validator mutation, or fabricated long copy is needed. With unchanged genesis
and empty per-state mutation maps, slice-copy useful work is short. A pending
writer can collect newly arriving readers and release them together, but it
cannot by itself explain how hundreds of count calls first passed checkpoint
retrieval. A positive requires actual scheduling under the composed load.

The existing helper already asserts before load that its native head and
checkpoint states share the same nonzero validator-storage pointer. Its
snapshot arm delegates the actual checkpoint lookup and changes only the
returned count iterator. Thus the real head-copy writer remains present in
both arms while the snapshot count avoids its per-validator read locks. This
is a meaningful control for the interaction, without changing state values or
the `ActiveValidatorCount` predicate loop.

The retained E1 evidence is
`/tmp/prysm-startup3-round4-early-e1/profiles/bn-goroutine-plus-37.pb.gz`:
919 count stacks, including 910 parked in the validator slice's `Len.RLock`,
two in `At.RLock`, and seven in computation; a concurrent production
`GetAttestationData → HeadState → Copy → multi-value slice.Copy.Lock` writer
is also present. The source and fixture identity establish that these paths
can share storage. The profile does not expose the lock address or establish
continuous wait duration, so co-occurrence alone does not prove that this
particular writer owned the precise lock holding every captured reader.

## Minimal implementation supplied to Sol

1. In the external `package sync_test` coordinator, construct public
   `core.Service` with the fixture's real chain as `HeadFetcher`,
   `ChainInfoFetcher`, `GenesisTimeFetcher`, and `OptimisticModeFetcher`, plus
   `cache.NewAttestationDataCache()`. Set this as the existing RPC server's
   `CoreService` and provide its ordinary false `SyncChecker`. No package
   cycle or production test hook is needed.
   Before load, select the existing genesis node through the real
   `ForkChoicer().FullHead` while holding that fork-choice write lock; after
   unlocking, assert `CanonicalNodeAtSlot(1)` returns the genesis root and
   full=true. The initial writer pilot exposed an unselected internal head
   despite the cached chain head and full node being present. A zero-root
   canonical mismatch and index-zero result are not an acceptable substitute
   for this prerequisite. Keep this correction in the new writer mode;
   retained Domain-only measurements remain unchanged.
   Set every prerequisite fork epoch through Heze to zero before overriding
   the config. A second pilot reached the real full canonical head but still
   returned index zero because `attestationDataIndex` first tests the
   inherited future Electra epoch. `OverrideBeaconConfig` rebuilds the
   schedule; with all forks at zero its same-epoch ordering selects Heze.
   This maintains a coherent latest-fork genesis instead of accepting a
   contradictory earlier API branch.
2. Keep the existing 15,000 valid slot-one messages, pacing, native/snapshot
   pair, `GOMAXPROCS=4`, absolute slot-two deadline, and warmed external
   connection. After a successful real cold-sync prerequisite, release one
   `GetAttestationData(Slot:1, CommitteeIndex:0)` and the RANDAO Domain call
   concurrently on that connection. Do not await the data response or gate
   Domain on reaching the copy; either would add a dependency absent from the
   VC.
3. Record method, invocation/admission/return, exact absolute budget, data
   correctness, and the existing bounded counters. A test-only `HeadFetcher`
   wrapper may time one complete delegated `HeadState` call; it must neither
   hold an extra lock nor stop inside production work. This separates delay
   before the copy path from the copy path itself. Do not claim this span is
   time exclusively inside the validator slice's lock.
4. A successful data response must name slot one, Gloas data index one, the
   actual genesis block and target roots, and source round/root zero. Start
   with an empty BN attestation cache. Check the request actually entered
   `HeadState` once; a cache hit or stale-slot rejection did not test the
   writer. All measured setup must still finish before the real slot-one
   deadline.

An inexpensive copy with unchanged count and Domain behavior is an informative
null for this composition. A slow copy with reader growth is evidence for the
natural writer interaction, but only a matched Domain delay demonstrates its
effect on ordinary RPC servicing. Even that would not recover node169's
historical request timestamps.

## A proposed three-slot topology explanation was withdrawn

The retained E1/F1/H/I2 configuration audit recovered
`SLOTS_PER_ROUND: 4` and `ATTESTATION_SUBNET_COUNT: 6`, rather than the current
helper's round eight and default 64 total subnets. Exact lines are preserved
in [ffg-retained-config-audit-anchors.txt](ffg-retained-config-audit-anchors.txt).
The earlier proposal below incorrectly transferred the helper's distinct
topic sets to E1. That transfer is withdrawn: E1's later bursts reused the
same six topics, so they did not increase its topic-validation capacity to
8,192. No three-slot extension was run or approved on that premise.

A three-slot extension is the next grounded workload change, because E1's
above profile is at genesis +37 seconds, during **slot three**, after the two
previous bursts. Its retained `ffgsource-result.json` records 15,000 successful
source submissions in each of slots one, two, and three, with slot-two/three
attempts spanning offsets −391 ms to +1599 ms. These source-API successes do
not establish how many votes reached BN3's validator or their receiver-side
arrival times. The current one-slot fixture cannot reproduce prior-slot
backlog by construction.

With the helper's current configuration, six committees would map to
subnets 6–11 in slot one, 12–17 in slot two, and 18–23 in slot three
(`core/helpers/attestation.go:86–114`). That is a property of a hypothetical
extension of this helper, **not E1's topology**. With E1's six total subnets,
the modulo maps every slot back to topics 0–5; its 12 committees share those
six topics. Libp2p's 1,024-per-topic and 8,192-global defaults therefore
leave at most 6,144 asynchronous topic validators across all E1 slots.
Prior-slot backlog can still survive within that unchanged capacity. The
post-Deneb time check accepts current/previous-epoch attestations, so crossing
a slot boundary does not by itself remove old votes
(`helpers/attestation.go:131`).

Any future extension would first need to choose explicitly between the
current helper's configuration and the retained E1 configuration. It would
prepare and sign the real committees and exact topics for each of the three
slots before timing, then offer only 15,000 valid votes
per actual slot at 12-second spacing, without waiting for the previous burst
to drain. Use one real cold attestation-data request per slot after that
slot's actual preflight, preserve the cache across slots, and record per-slot
admissions and completions as well as aggregate totals. Each new slot makes
the attestation cache naturally stale; no manual cache reset is needed.
Do not reuse slot-one signatures or its committee memberships for later
slots. Keep the source's finite schedule distinct from a measured receiver
arrival schedule. Keep pool size, verification, and topic capacities at their
production settings.

This extension still omits full BN periodic head processing and unrelated
services; it is a controlled composition rather than a full E1 replay. A
slot-three positive would strengthen the retained E1 mechanism and explain
why a one-burst control missed its cohort. It cannot literally explain
node169's slot-one failure through previous nonzero slots, because those
previous slots did not yet exist and slot-zero FFG is rejected before the
count path.

No production source was changed or experiment run by this audit.
