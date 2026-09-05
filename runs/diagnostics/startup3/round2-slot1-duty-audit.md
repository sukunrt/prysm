# Round2 slot-1 duty accounting

This audit narrows the round2 node169 RANDAO failure without claiming that the
historical logs expose the exact blocked stage. The source is the recovered
`round2-169/validator.log`; the relevant schedule is line 653 and the slot-1
terminal interval is lines 686–774.

Run the bounded parser with:

```sh
python3 runs/diagnostics/startup3/round2_slot1_audit.py \
  /tmp/prysm-r2-extra-logs.Rd7MjT/extracted/round2-169/validator.log
```

It establishes these exact counts:

- All 75 scheduled attester pubkeys have exactly matching prerequisite
  attestation-data failures. There are no missing or extra pubkeys. None can
  reach `signAtt` or `DomainBeaconAttester` on that attempt.
- The four scheduled PTC duties account for one `NotFound` skip at
  01:30:21.915067 and three payload-data deadlines. All return before the PTC
  domain call.
- Two sync-message duties fail `SyncMessageBlockRoot` before the sync domain.
  The sync-aggregator attempt fails `SyncSubcommitteeIndex` before signing its
  selection domain.
- Three aggregators report both failure to obtain signed attestation data and
  failure of the aggregate-selection RPC. They do not reach the later
  aggregate-and-proof domain call.
- The proposer pubkey `0xa7120c370e8c` is also one of the 75 attesters. Its
  RANDAO domain failure is at line 694; its independent attestation-data
  failure is at line 766.

The successful PTC `NotFound` response proves that `RolesAt` had returned and
role dispatch had begun by 21.915. It does not timestamp the proposer
goroutine, RANDAO start, or the onset of impaired servicing. Nor do the shared
deadlines alone distinguish a BN handler stall from gRPC/network or VC-reader
scheduling.

## Selection-proof and background-work boundary

For the local selector, an attestation proof is stored in a plain map keyed by
`(slot, validatorIndex)`. It has no Ristretto admission or ordinary eviction.
`RolesAt.isAggregator` and the later aggregate duty request the identical key,
so the ordinary post-dispatch aggregator reuses that proof. The map is cleared
only by `RefreshSelectionProofs` (`validator/client/aggregator_selector.go`).

There is nevertheless a concrete background domain-lock competitor.
`onDutiesUpdated` detaches from its caller with `context.Background` and starts
`subscribeToSubnets` outside the slot waitgroup and deadline
(`validator/client/duties.go`, `validator/client/subnets.go`). That job clears
the proof map and uses 16 workers to sign selection proofs across current and
next duties. Those signatures call the shared `domainData` path. The ordinary
genesis duty update can therefore overlap slot 1; this does not require a
missing-duty retry. Neither the VC log nor the BN subscription handler records
a completion timestamp that bounds it. A later duties update can also clear
the proof map again.

Consequently the ordinary slot-1 role goroutines provide no demonstrated
domain-lock owner ahead of RANDAO, but the detached subscription job can. A
deadline returned from `domainData` also does not prove that lock acquisition
was uncontended: the shared mutex must merely have released by the time the
error was logged. The historical evidence leaves RANDAO divided among VC
scheduling, waiting behind detached selection-domain work, and its own
DomainData RPC transport or BN servicing. It does not resolve the exact handler
cause.

## Lock-cancellation diagnostic

`TestDomainDataCanceledWaiterDiagnostic` in
`validator/client/domain_cache_diagnostic_test.go` exercises the real
`validator.domainData` implementation.  A `context.Background` selection-proof
miss, matching the detached `subscribeToSubnets` work, is held inside its mock
BN RPC while it owns `domainDataLock`.  A RANDAO miss is then canceled and is
observed not to return or enter its BN RPC for at least 150 ms.  Once the
selection request is released, RANDAO reaches the mock with the canceled
context and returns `context.Canceled`.  Ten consecutive Go-only runs passed.

This establishes the mutex semantic, not a historical duration: context
cancellation cannot interrupt an established wait for the RWMutex.  Therefore
the historical error at 24.002 rules out a detached holder continuing to own
`domainDataLock` beyond that log, but it does not rule out earlier lock delay
before 24.002 and does not identify the original root cause.

## Independent local DomainData observations

The existing E1 standalone probe called the BN's raw gRPC `DomainData` stub,
bypassing the VC domain cache, its mutex, and role dispatch. Its slot-3 records
include elapsed durations of 1.711, 1.176, 2.165, and 4.272 seconds; the F1
active-count-ablation control's maximum was 6.282 ms. Both runs recorded 72
successful domain probes. E1's slot-1 maximum was only 68.406 ms, and its real
proposer RANDAO calls were fast. These observations must not be substituted
for a reproduction of the historical slot-1 RANDAO failure.

The probe measures elapsed time after acquiring its shared output mutex, so
the recorded duration includes RPC processing plus any post-return scheduling
and output-lock wait. E1 also used the old catch-up cadence after slow calls;
the late samples are not independent fixed-rate measurements. BN goroutine
snapshots at genesis +37 and +43 seconds show thousands of active gossip
validators but no `DomainData` handler frame. Together these observations
demonstrate that seconds-scale delays in the standalone RPC measurement can
occur without the VC's domain lock, but do not distinguish BN transport or
dispatch delay from probe-side rescheduling. H's packet/runtime trace, not
these elapsed durations, supplies the separate exact scheduler-starvation
proof.

Raw inputs remain at
`/tmp/prysm-startup3-round4-early-{e1,f1}/rpcprobe.jsonl` and E1's
`profiles/bn-goroutine-plus-{37,43}.pb.gz`.
