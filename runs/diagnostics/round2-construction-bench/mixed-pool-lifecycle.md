# Mixed Electra/Gloas pool lifecycle at the deployed revision

This source audit uses the isolated checkout at
`0280403c70d88967f49d2d4c730f4c5417dabdf5`. It distinguishes physical entries
in the raw pool from entries returned to the proposer. Those counts can differ
substantially after inclusion pruning.

## After a block includes the votes

The old block-pruning code constructs per-committee Electra dummy attestations
and calls `DeleteAggregatedAttestation`. That method first writes Electra
seen bits, then fails to find the differently keyed stored Gloas aggregate.
The aggregate remains, but the Electra seen bits still take effect.

Consequently, for Electra singles covered by that included vote:

| Operation | Old Gloas ingress | Normalized Electra ingress |
| --- | --- | --- |
| Singles stored before inclusion | Present in raw map | Present in raw map |
| Electra inclusion-prune call | Gloas aggregate remains; Electra seen bits inserted | Aggregate removed; Electra seen bits inserted |
| Proposer's raw getter after pruning | Covered singles excluded | Covered singles excluded |
| Reoffer the same Electra singles | Save rejects seen bits | Coverage/seen checks reject them |
| `DeleteSeenUnaggregatedAttestations` | Physically deletes covered raw entries | Physically deletes covered raw entries |

`UnaggregatedAttestations` scans every physical raw entry and checks its seen
bits before deciding whether to clone and return it. Thus a pool with a
physical count of N and zero eligible singles still pays N seen-bit checks.
It does not make the proposer validate and deduplicate N raw candidates.
Benchmarks must report both `UnaggregatedAttestationCount()` and
`len(UnaggregatedAttestations())`, and separate this scan from subsequent
candidate-processing costs.

Source anchors: `blockchain/process_block.go:766–790`,
`operations/attestations/kv/aggregated.go:255–270`,
`operations/attestations/kv/unaggregated.go:15–40,53–73,155–181`, and
`operations/attestations/kv/seen_bits.go:12–65`.

## Before inclusion: arrival order matters

Normal single-attestation gossip is converted to Electra before reaching the
subscriber (`sync/validate_beacon_attestation.go:172–181`). The subscriber
calls `HasAggregatedAttestation` before attempting to save the single
(`sync/subscriber_beacon_attestation.go:27–36`).

If a fresh Gloas aggregate arrives **before** covered Electra singles, the old
versioned coverage lookup can miss it. Those singles can enter the raw pool.
With normalized Electra aggregate ingress, the same coverage lookup succeeds
and suppresses them. This is a concrete mixed-pool admission difference that
an isolated counterfactual can measure.

If the singles arrive **before** the aggregate, both versions can already
have those singles in the raw map. Saving a covering aggregate alone does
not insert seen bits or remove existing raw entries. Both versions can
therefore have a mixed pool until compaction or inclusion pruning acts.

An existing Electra aggregate created by normal compaction can also suppress
later Electra singles in the old build. The wrong Gloas key only adds an
admission difference for coverage absent from the Electra entries/cache.
Creating 13,000 late arrivals after a Gloas aggregate while omitting earlier
Electra coverage is a controlled capacity test, not a demonstrated historical
arrival sequence.

The selected actual slot-91/93 aggregate ledger records arrive around eight
seconds into their slots. They establish aggregate arrival timing and voter
coverage, but do not establish a large batch of singles arriving afterward.
The [aggregate-input bounds](aggregate-input-bounds.md) record their scope.
For FFG singles, `arrivedMs` records entry into validation; the ledger line is
written after validation and does not contain the Goldfish ledger's separate
`decidedMs` field. Early arrival percentiles alone therefore cannot establish
whether the pool subscriber ran before an aggregate arrived. Neither ledger
line timestamps the subscriber's completed pool insertion.

The bounded [node-1 slot-96 timing census](node1-slot96-ffg-timing.json)
strengthens the singles-before-aggregate interpretation for the slot preceding
the measured slow proposal at 97: all 13,947 distinct single-vote ledger rows
were emitted by slot+4.157 seconds, whereas the first of 12 aggregate ledger
rows was emitted at slot+8.035 seconds. The single rows span six committees
and ten data-root groups. There are no single ledger emissions after the
scheduled seven-second compaction point. This is evidence about ledger order,
not proof of completed pool admission, compaction, or the proposal snapshot.
In particular, a control that admits all 13,000 singles after aggregates must
not be described as a replay of this observed ordering.

The full packer also normalizes Gloas aggregates before deduplication. A mixed
pool containing a full covering aggregate and many redundant singles can
discard those singles during deduplication. Its cost must be measured directly;
the much larger deduplication cost of a raw-only pool of mutually disjoint
singles cannot simply be assigned to that mixed pool.

## Compaction, cleanup and expiration

The deployed defaults schedule normal compaction at 7.0, 9.5 and 11.8 seconds
into each 12-second slot (`config/features/flags.go:58–74`;
`operations/attestations/prepare_forkchoice.go:24–36,69`). These are scheduled
times; they do not prove when an overloaded runtime completed each call.

Compaction starts from the same getter that excludes seen raw entries. It
aggregates the returned unseen singles and deletes those processed singles.
It does not physically delete already-seen entries that the getter omitted.
For those, the separate prune loop calls `DeleteSeenUnaggregatedAttestations`
at the slot's 11-second offset (`prune_expired.go:12–20,62`). A benchmark
should preserve the physical entries until this actual cleanup operation
runs when measuring their scan cost.

Expiration still deletes a retained aggregate using its original stored
version, so the inclusion-pruning bug is not permanent retention. The seen-bit
cache is initialized with a **two-epoch** TTL (`kv/kv.go:36–37`), despite the
shorter inline comment in the seen-bit helper. Under 32-slot epochs and
12-second slots that TTL is 768 seconds; ordinary physical cleanup should
occur well before it expires.

These lifecycle rules support testing both arrival orders, compaction, and
physical-versus-eligible raw counts. They do not by themselves attribute the
historical multi-second joined construction interval to one particular pool
snapshot.

## Compaction is wired into the deployed Heze node

The exact deployed source does not contain a Heze/Gloas branch that disables
legacy compaction while leaving the proposer on that pool:

- `node/node.go:168` creates one legacy pool. `registerAttestationPool` passes
  that object and `initialSyncComplete` to the operations service
  (`node.go:730–739`). The proposer receives the same object through
  `node.go:1026` and `rpc/service.go:237`.
- The node registers the attestation service unconditionally before chain and
  initial-sync services (`node.go:397–409`). `ServiceRegistry.StartAll` launches
  each `Start` in a goroutine (`runtime/service_registry.go:38–42`), so the
  attestation service waiting for initial sync does not prevent initial sync
  from starting.
- Blockchain startup calls `AttService.SetGenesisTime(saved.GenesisTime())`
  before publishing its clock (`blockchain/service.go:270–272,294–298`).
  Initial sync waits for that clock, then `markSynced` closes the same shared
  channel (`initial-sync/service.go:135–144,357–359`). The attestation service
  starts `prepareForkChoiceAtts` only after receiving from that channel
  (`operations/attestations/service.go:58–64`). Thus normal startup initializes
  the attestation service's genesis time before releasing its wait.
- `features.ConfigureBeaconChain` sets the three aggregation intervals to
  7.0, 9.5, and 11.8 seconds before service startup
  (`node.go:295`; `config/features/config.go:347`; `flags.go:58–74`). At a
  12-second slot, `prepareForkChoiceAtts` leaves these values unchanged.
- Every received tick calls `batchForkChoiceAtts`; its legacy branch first
  calls the real `Pool.AggregateUnaggregatedAttestations`
  (`prepare_forkchoice.go:35–40,66–73`). The experimental-pool switch is also
  used by the proposer, rather than selecting different pools independently.

The retained owner log supports completion of the startup gate: node 1 reports
`Genesis time has not arrived - not syncing` at
`2026-09-05T00:50:47.047952688Z`, with genesis `01:30:00Z`
(`runs/round2/prysm-geth-1/beacon.log:31`). That message follows `markSynced`
in the pre-genesis branch (`initial-sync/service.go:188–191`). The same log
contains no `failed to wait for initial sync` or `Could not prepare attestations
for fork choice` marker. Successful compaction duration logging is at Debug
level and is absent from this retained log, so these checks do not prove when
each later compaction completed.

The ticker and service process ticks serially. A slow batch or an unscheduled
goroutine can delay later work; successful source wiring is not a timing
guarantee. This audit found no startup, genesis-time, pool-identity, or fork
selection defect that explains a persistent raw backlog at proposal 97.
