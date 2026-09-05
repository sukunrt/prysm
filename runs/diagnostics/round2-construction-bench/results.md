# Exact deployed Prysm proposal-construction benchmarks

## Finding

The exact deployed Prysm revision builds the synthetic full-parent density-8
slot-97 block in 91.766 ms when isolated. Replaying all 14,355 observed node-1
slot-97 gossip validations at their original offsets did not slow construction
with `GOMAXPROCS=4`: three paired baselines were 94.929, 94.695, and 99.147 ms,
while paced builds were 89.283, 90.621, and 98.414 ms. Between 125 and 192
validations had entered by build completion; all 14,355 were ultimately
accepted and subscribed in each run.

This directly tested the dominant concurrent work visible during the historical
build. It did not reproduce the observed 3.654-second construction interval.
A companion control then added the exact deployed Xatu topology of 18 one-topic
Prysm SSE streams. With `GOMAXPROCS=4`, the same paced workload measured 89.636
ms without SSE and 86.309 ms with SSE. With `GOMAXPROCS=1`, it measured 152.248
and 160.982 ms, an 8.733 ms (1.057x) increase. All 14,355 validations and
complete `single_attestation` SSE frames drained successfully, with no
slow-reader warning or disconnect. The local, immediately drained SSE control
does not reproduce the seconds-scale interval.

The earlier `GOMAXPROCS=1` sensitivity control increased a paired build from
108.589 to 146.639 ms, showing a bounded Go-scheduler interaction under reduced
runtime parallelism. `GOMAXPROCS=1` is neither a one-core CPU quota nor evidence
of the historical host's scheduling state.

The old packer can still take seconds under a different, conditional pool
shape: 1.804 seconds for 5,000 valid mutually disjoint raw singles and 3.433
seconds for 13,000. The CPU profile attributes that work primarily to pairwise
bitlist containment in deduplication and `MaxCover` aggregation. This is a
capacity reproduction rather than historical attribution. The exact retained
set join instead finds 13,925 of node 1's 13,947 slot-96 single-vote rows covered
by the largest logged gossip aggregates for their data groups, leaving 22 rows
in two groups with no logged aggregate. The ledger does not record the proposer
pool snapshot or prove that those aggregates reached it.

The separate [slot-97 timing census](node1-slot97-ffg-timing.md) finds the 14,355
gossip votes entering validation from slot +30 through +3,489 ms, within the
observed build interval of +8.736 to +3,662.623 ms. Current-slot votes fail the
inclusion-delay check before expensive proposer deduplication, so the paced
driver exercised validation, batch verification, logging, and subscription
contention without treating them as eligible inputs to their own slot-97 block.

The bounded [build-97 message census](node1-build97-window-message-census.md)
finds 14,355 gossip FFG rows, 71 local FFG rows, and 465 accepted Goldfish
head-vote rows between the build and finish markers. Goldfish validation entry
begins near the build's tail. No FFG aggregate, PTC, or sync
committee/contribution record occurs in the interval. The preceding one-second
window has no retained record, which does not prove an idle process or empty
queues.

The other isolated components are also small: retained compact Gloas aggregates
raise ordinary slot-97 packing from 1.526 to 15.947 ms; scoring 29,847 credited
positions takes 5.188 ms; fresh state-root hashing takes 0.51-0.55 ms; and full
transition/root wrappers take 13.328 ms for three compact attestations and
32.969 ms for a synthetic maximum-density eight-attestation block. A physical
backlog of 13,000 already-seen singles adds about 23-25 ms before cleanup.

The measurements identify a seconds-scale packer capacity limit, while the
historical cause remains unresolved: neither the recovered eligible-vote shape,
isolated full construction, nor paced current-slot validation explains the
2.5-7.8 second joined intervals.

The later [slot-110 investigation](round2-slot110-timeline.md) independently
recovers owner node 148's construction interval: payload selection at slot
+24.961 ms and completion at +3.409893 s, a 3.384933-second gap. Its
[published block contents](slot110-block-inputs.md) contain 97,093 FFG
participant positions and 511 sync bits; five complete logged attestation
identities repeat records from blocks 108 or 109. A slow-SSE-reader warning
occurs during this build. The [source audit](slot110-source-comparison.md) distinguishes that observed
backpressure from a demonstrated builder wait. These are fresh historical
observations, not a slot-110 benchmark or a replay of its unlogged pool.

## Provenance and fixture boundary

All authoritative runs summarized here used the isolated workspace
`/home/sukun/dev/prysm2-round2-construction-028`, whose parent is the deployed
Prysm revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`. Production code was
unchanged. The initial construction and no-SSE paced measurements used seven
diagnostic Go test files. Their frozen source hashes are:

| File | SHA-256 |
| --- | --- |
| [historical_late_packing_diagnostic_test.go](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/rpc/prysm/v1alpha1/validator/historical_late_packing_diagnostic_test.go) | `4c4668b940d9a07db9b6ebc186d0c1b5b174c07a3d5372e2d2de498e6dd0bea6` |
| [historical_state_root_diagnostic_test.go](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/rpc/prysm/v1alpha1/validator/historical_state_root_diagnostic_test.go) | `a8f8fad5e00561285963c73e1405403c2b9ab64f6b9a1f0f079c346b94d43c16` |
| [historical_full_build_diagnostic_test.go](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/rpc/prysm/v1alpha1/validator/historical_full_build_diagnostic_test.go) | `0db72d3d09dfb57bc711ba138661adc0e8846d4e868b33bca95645936a3c4395` |
| [historical_late_validation_export_test.go](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/sync/historical_late_validation_export_test.go) | `aba5b92e1c0f2bf24a2a99a66347b30a0c6066fb843c1688962bd63a38f76840` |
| [historical_paced_state_fixture_test.go](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/sync/historical_paced_state_fixture_test.go) | `9e39bc2a5799c156eee231e791e35f4382e33d96a9894b942bcc1b82f1263e14` |
| [historical_paced_full_build_fixture_test.go](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/sync/historical_paced_full_build_fixture_test.go) | `988800d03b9108f34fa56e7c68d6c78291559fc56b5f5af99c7a5d6ffa062339` |
| [historical_paced_full_build_diagnostic_test.go](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/sync/historical_paced_full_build_diagnostic_test.go) | `e4f793d0f277811cdbb35eb231f31f9a1abe602a355b8cafdd643bfa4f136e89` |

The exact seven-file snapshot used by the final P4/P1 runs is preserved with
relative paths in
[old028-paced-diagnostic-sources.tar.gz](old028-paced-diagnostic-sources.tar.gz)
(`SHA-256 383916ba48194138393bef5fa4e0053b91fcb174e5f3365f20e95eed22660160`).
Its standalone checksum manifest is
[old028-paced-diagnostic-sources.sha256](old028-paced-diagnostic-sources.sha256)
(`SHA-256 92e84f131aabcec141d4e4b20e717ed2daab4afe57837fcb44b195d7702b465e`).


The SSE companion adds one diagnostic test file and modifies only the test-only
validation export and paced driver. Its exact eight-file snapshot is
[old028-sse-diagnostic-sources.tar.gz](old028-sse-diagnostic-sources.tar.gz)
(`SHA-256 22664750ae8ded05ff4488e9208a5e3037d9ced1044afc5fd5cda851fdf4ce51`),
with [checksum manifest](old028-sse-diagnostic-sources.sha256)
(`SHA-256 fc3f76030ce576814295c26bdffdb934ab9411fa96ebc04027810ddcbbe3cc9b`).
The three SSE-era changed or added source hashes are:

| File | SHA-256 |
| --- | --- |
| `beacon-chain/sync/historical_late_validation_export_test.go` | `a9b277688f611294232610667a3c1875202d732593e86a2ff555472d0ffc5709` |
| `beacon-chain/sync/historical_paced_full_build_diagnostic_test.go` | `a7f2ffe399185d1bdc81fc1f63246da64bf5cc1d8ea93e6dfcb564a1943b0fc5` |
| `beacon-chain/sync/historical_paced_sse_fixture_test.go` | `744cca672d75dc62375cc575f788273c25a0b86446cfa233b470ebf97980bc63` |

The packing fixture uses a native Heze state at slot 97 with head slot 96,
120,000 active validators, distinct deterministic BLS keys, the deployed fork
configuration, the production 64-committee cap, six computed committees, and
valid aggregate signatures made from the actual shuffled participants. Setup,
key generation, signing, fixture validation, and cache warming are outside the
timed regions. The component benchmarks therefore describe a warm isolated
process on the [recorded benchmark host](run-environment.md), with
`GOMAXPROCS=4` and Go 1.26.5. They exclude live RPC, fork choice, networking,
validator submission, execution-engine work, and scheduler contention. The
later paced comparison adds real ordinary-attestation validation and subscriber
work while retaining immediate chain/Engine fixtures and no TCP transport.

The compact retained shapes reproduce observed node-400 aggregate coverage:

- slot 91: six committees with `[2486, 2488, 2481, 2491, 2487, 2487]`
  participants, 14,920 total;
- slot 93: six committees with `[2489, 2485, 2484, 2487, 2492, 2490]`
  participants, 14,927 total;
- fresh slot 95: six full-committee aggregates, 15,000 participants.

The old votes are marked already credited for source, target, and head. The
source/target checkpoint roots are internally coherent synthetic values because
the retained ledgers do not contain those roots. The raw-density controls use
valid one-bit Electra singles but synthesize their pool population; 5,000 and
13,000 are controlled density points rather than recovered pool counts.
Every packed result is checked for valid BLS signatures and data-keyed fresh
coverage. The separate [historical coverage bounds](historical97-coverage-bounds.md)
now join full logged validator sets rather than relying only on aggregate
cardinality. Of node 1's 13,947 slot-96 singles, 13,925 occur in the largest
logged gossip aggregates for their data groups; the remaining 22 belong to
two groups with no logged aggregate. Across slots 88-96, 103,126 logged
singles have this exact membership and 367 belong to 33 groups without an
aggregate. The earlier 123 slot-96 figure remains documented there as the
superseded conservative cardinality-only bound. These statements remain
conditional on the logged gossip aggregates reaching the proposal snapshot:
the ledger does not prove insertion or snapshot presence, and pending replay
can insert pool objects without a successful FFG ledger row.

## Final ordinary-slot results

The final 20-iteration log is
[old028-final-compact-and-state-20x.log](old028-final-compact-and-state-20x.log)
(`SHA-256 36d442b001c651a2532d4fdb299df06be5becb1684dd08de6b09fbaaf7f1b4ef`).

| Benchmark, slot 97/head 96 | Time/op | Bytes/op | Allocs/op |
| --- | ---: | ---: | ---: |
| compact Gloas retained, full pack (18 pool aggregates) | 15.947 ms | 9.18 MB | 46,348 |
| normalized Electra pruned, full pack (6 fresh aggregates) | 1.526 ms | 0.89 MB | 377 |
| compact-3 block, transition | 14.529 ms | 7.55 MB | 47,456 |
| compact-3 block, fresh post-state root | 0.550 ms | 0.69 MB | 1,377 |
| compact-3 block, full proposer wrapper | 13.328 ms | 8.24 MB | 48,664 |
| synthetic density-8 block, transition | 29.888 ms | 19.68 MB | 123,341 |
| synthetic density-8 block, fresh post-state root | 0.533 ms | 0.69 MB | 1,377 |
| synthetic density-8 block, full proposer wrapper | 32.969 ms | 20.37 MB | 124,550 |

The residual slot-advance-dirty root controls were 0.512 ms for compact-3 and
0.532 ms for density-8, close to the clean-pre-state measurements. The wrapper
asserted that all attestations remained in the block and that the production
retry path did not silently strip them. See the detailed
[state-root method boundary](old028-state-root-method.md).

The earlier slot-96/head-95 boundary run is preserved in
[old028-packing-slot96-20x.log](old028-packing-slot96-20x.log). It deliberately
hits the packer's batch-BLS fallback for target-round-11 votes, whereas the
ordinary slot-97/head-96 run accepts them through the previous-target-round
shortcut. Its retained/pruned full-pack values were 39.144/10.472 ms. Its
independent stage probes were:

| Slot-96 stage probe | Retained | Pruned |
| --- | ---: | ---: |
| pool snapshot and validation | 1.754 ms | 0.555 ms |
| dedup and aggregation | 1.926 ms | 0.578 ms |
| reward sort | 9.883 ms | 0.000287 ms |

Direct reward scoring took 5.880 ms for 29,847 uncredited positions and 5.188
ms for the same already-credited positions. Crediting removes reward, but it
does not avoid walking those participants. These probes are independent calls;
their values must not be summed as a decomposition of full packing.

## Full block construction

The corrected final full-parent log is
[old028-full-build-full-parent-final-20x.log](old028-full-build-full-parent-final-20x.log)
(`SHA-256 c8eb27097d4f32de1feea4e08836f29d23c522c1acc2cdb6227bd7c5b28d8008`).

| Exact-old `BuildBlockParallel`, slot 97 | Time/op | Bytes/op | Allocs/op |
| --- | ---: | ---: | ---: |
| full parent, compact-3 block (44,847 participant positions) | 37.530 ms | 22.14 MB | 134,786 |
| full parent, density-8 block (120,000 participant positions) | 91.766 ms | 49.65 MB | 288,655 |

Each timed call uses a real warm `StateGen` mapping from the block root to a
slot-96 native-Heze parent and performs the actual slot-96-to-97 advance. The
saved state contains the execution hash from payload 95 and the distinct bid
for payload 96. With `parentFull=true`, construction reads the persisted parent
payload envelope, applies payload 96, and builds payload 97 whose parent hash is
payload 96. This matches the delivered-parent lifecycle exercised by a normal
slot-97 proposal.

The compact arm starts from the 18 retained per-committee pool aggregates and
packs three on-chain attestations. The density arm starts from 48 valid
per-committee network aggregates across slots 88-95 and constructs eight valid
six-committee on-chain attestations. The latter matches the historical block's
attestation count, while its 120,000 participant positions are a synthetic
maximum-density bound rather than a replay of the historical attestations.
Both arms include four signed sync contributions covering 493 positions.

Block allocation, proposer lookup, RANDAO signing, key/signature generation,
fixture validation, and result assertions are outside the timer. The timed
call includes parent-state retrieval/advance, parent execution processing,
attestation and sync packing, immediate connected Eth1 reads, immediate mock
Engine FCU/GetPayload, state transition/root calculation, and envelope-cache
storage. The result checks require the expected attestation and sync coverage,
a nonzero state root, and a slot-97 cached execution envelope whose beacon root
matches the built block. The immediate mocks and warm local state store exclude
historical network, execution-engine, database-I/O, scheduler, and concurrent
validation delay.

The earlier
[old028-full-build-parallel-20x.log](old028-full-build-parallel-20x.log)
(`SHA-256 7e5e72b20a64f62af36a0c9e55fc04e72e8c40590f9dbd2e88c5d6368c0b7e6d`)
is preserved as a superseded pilot. Its full-parent fixture initialized the
state's latest execution hash equal to the payload-96 bid hash, which modeled
an incoherent post-parent state for this call rather than the historical
post-block-96/pre-payload-96 lifecycle. `ApplyParentExecutionPayload` does not
short-circuit on that equality, but the input state still had the wrong
execution hash. Its 35.362 ms compact value is therefore not used as the final
result.

## Paced current-slot validation

The final `GOMAXPROCS=4` record is
[old028-paced-validation-full-paired-count3.log](old028-paced-validation-full-paired-count3.log)
(`SHA-256 b2e23d26ba1a5c7ac41818beccf65b6dd19eb00ea258a185be11e1386eaaffe2`).

| Pair | Unloaded baseline | Paced validation | Ratio | Started / subscribed at build finish |
| ---: | ---: | ---: | ---: | ---: |
| 1 | 94.929 ms | 89.283 ms | 0.941x | 127 / 102 |
| 2 | 94.695 ms | 90.621 ms | 0.957x | 125 / 106 |
| 3 | 99.147 ms | 98.414 ms | 0.993x | 192 / 168 |

Each pair prepared the same 14,355 signed and encoded messages, density-8
proposal fixture, logging hooks, database, and caches outside the measured
call. The baseline released no validations. The paced arm anchored on the
production `Chose payload bid` log and released each message at
`bid time + arrivedMs - 17.249896 ms`, preserving node 1's offsets relative to
the historical bid marker. It reproduced 14,283 votes for the block-96 root and
72 for the stale block-69 root across the six committees. Synthetic keys were
assigned to distinct members of the same 120,000-validator committee schedule;
the unlogged source/target roots and historical key identity cannot be
reconstructed.

The driver calls the deployed ordinary-attestation validator, its 5 ms/1,000
signature batch verifier, and the production legacy-pool subscriber against
the same slot-96 state, `StateGen`, database, and pool used by
`BuildBlockParallel`. All messages are valid slot-97 singles, so all 14,355
were accepted and subscribed after the scheduled drain. They remain ineligible
for inclusion in the block being built for slot 97. The started/subscribed
figures above are snapshots taken immediately when construction returned;
their counters are independently atomic rather than one transactional snapshot.

Direct scheduling drift, measured immediately before `Validate` relative to
each intended release time, had per-run medians of 0.743, 0.503, and 0.497 ms;
the corresponding p95 values were 12.893, 2.492, and 5.948 ms and maxima were
24.643, 4.716, and 10.958 ms. This measurement is independent of the arbitrary
synthetic genesis timestamp.

The logger enables the vote ledger and performs prefixed text formatting, a
production `WriterHook`, and stream-server cache/feed work with immediate
nonblocking sinks in both arms. It omits physical file/stderr blocking and
downstream operation-feed consumers. Head/fork-choice ancestry, Eth1, Engine,
and database reads are immediate fixtures; there is no TCP gossip transport.
The [contention boundary review](ordinary-validation-contention-bounds.md)
lists the shared caches and locks exercised and the remaining omissions.

The bounded `GOMAXPROCS=1` result is
[old028-paced-validation-full-paired-gomaxprocs1.log](old028-paced-validation-full-paired-gomaxprocs1.log)
(`SHA-256 2ec9f01edd4859b72f4d718db152fa492b703fef1435c446d780f530bd0e1f50`):
108.589 ms unloaded versus 146.639 ms paced, a 38.050 ms increase (1.350x).
At build completion 501 validations had started and 43 subscribers had
completed; all 14,355 completed afterward. Its entry-drift median/p95/max was
1.908/43.762/85.816 ms. This is a runtime-parallelism sensitivity control, not
a historical CPU-allocation claim; cgo work can also execute on other threads.

## Deployed SSE topology companion

The [exact Xatu/SSE source audit](xatu-sse-contention-audit.md) confirms that the
deployed Xatu revision opened one HTTP subscription for each of 18 logged
topics. Eleven topics subscribe to Prysm's operation feed: `attestation`,
`single_attestation`, `block_gossip`, `voluntary_exit`,
`contribution_and_proof`, `blob_sidecar`, `data_column_sidecar`,
`execution_payload_gossip`, `execution_payload_bid`,
`payload_attestation_message`, and `proposer_preferences`. Seven subscribe to
the separate state feed: `block`, `chain_reorg`, `finalized_checkpoint`, `head`,
`head_v2`, `execution_payload`, and `execution_payload_available`.

The companion starts the production public `StreamEvents` handler in an
`httptest` HTTP server and opens one real HTTP client per topic. A harmless,
unrequested sentinel establishes that all 11 operation-feed and seven
state-feed subscriptions are receiving before construction begins. Clients
immediately scan response bodies. The driver counts a single-attestation frame
only after reading its event line, data line, and terminating blank line. EOF
before cancellation is an error. It uses Prysm's default event depth, keepalive,
and one-slot write timeout.

| Runtime setting | Paced, no SSE | Paced, 18 SSE streams | SSE delta/ratio | Validation started / subscribed / complete SSE frames at build finish |
| --- | ---: | ---: | ---: | ---: |
| `GOMAXPROCS=4` | 89.636 ms | 86.309 ms | -3.326 ms / 0.963x | 102 / 102 / 102 |
| `GOMAXPROCS=1` | 152.248 ms | 160.982 ms | +8.733 ms / 1.057x | 575 / 36 / 36 |

The matched P4 logs are
[without SSE](old028-sse-comparison-p4-nosse.log)
(`SHA-256 96e3b9fd7a021aaf95160a258f55c928e959e1608ea8944af85aef670f6c5c01`)
and [with SSE](old028-sse-comparison-p4-sse18.log)
(`SHA-256 f527da0501672f5183c47dd4af274d7ba740961e0b9fb873b0e401e123d12a91`).
The P1 logs are
[without SSE](old028-sse-comparison-p1-nosse.log)
(`SHA-256 3548a94fd0fd6dd65762f93cc2b4019878af1c8ef4874e008bda695e7d129443`)
and [with SSE](old028-sse-comparison-p1-sse18.log)
(`SHA-256 a0d78b1af1af54f7e15e1caacca505b96efd3ee41454a543c77f7e49b8515b27`).
The 20-row SSE smoke is also preserved in
[old028-sse-paced-validation-smoke20.log](old028-sse-paced-validation-smoke20.log).

Both SSE runs accepted and subscribed all 14,355 validations and drained all
14,355 complete frames. Neither emitted a slow-reader warning or disconnected.
At P4 the direct entry-drift median/p95/max was 0.658/5.047/9.703 ms with SSE,
versus 0.741/7.290/18.108 ms without it. At P1 it was
1.739/65.873/127.477 ms with SSE, versus 2.347/48.721/92.101 ms without it.
These are single matched observations, so their millisecond differences bound
this fixture rather than estimate a stable effect size.

This exercises Prysm's synchronous delivery to all 11 unbuffered operation-feed
receivers, event conversion, JSON formatting, local HTTP writes, and immediate
client drains. It does not run Xatu parsing, enrichment, its 100,000-item async
queue, export RPCs, network delay, file logging, or slow clients. Exact Xatu
source shows that a full async export queue drops rather than waits for its
upstream RPC. The historical logs do not prove that all 18 HTTP connections
were continuously active at the proposal instant.

## Raw, mixed, and seen-entry results

The final five-iteration log is
[old028-final-mixed-raw-seen-5x.log](old028-final-mixed-raw-seen-5x.log)
(`SHA-256 4aa386a8369ecb07952b394accf6218bae08a38532e1e8a64022dd1fa2ded62f`).
All cases use state slot 97/head slot 96.

### Raw-only density controls

| Valid mutually disjoint singles | Snapshot + validation | Dedup + aggregation | Full pack | Full-pack allocation |
| ---: | ---: | ---: | ---: | ---: |
| 5,000 across 2 committees | 63.900 ms | 2.185 s | 1.804 s | 142.86 MB, 345,260 allocs |
| 13,000 across 6 committees | 153.435 ms | 3.082 s | 3.433 s | 371.17 MB, 897,655 allocs |

This is the seconds-scale reproduction. The candidates are mutually disjoint
raw singles without a covering aggregate, so they force the expensive
deduplication/aggregation work. The separate stage probes use fresh inputs and
are not additive components of the full-pack value.

### Mixed arrival orders

Each mixed arm also contains the same compact old and fresh aggregate coverage.
The fresh compact aggregates cover 15,000 positions; the 5,000/13,000 singles
are subsets of that coverage.

| Arrival order and density | Compact Gloas retained | Normalized Electra pruned |
| --- | ---: | ---: |
| aggregate before 5,000 singles | 86.711 ms | 1.073 ms |
| aggregate before 13,000 singles | 192.912 ms | 1.088 ms |
| 5,000 singles before aggregate | 83.142 ms | 70.557 ms |
| 13,000 singles before aggregate | 199.591 ms | 183.293 ms |

When the aggregate arrives first, the old Gloas key misses the later Electra
singles, so the old arm admits them; the normalized Electra aggregate suppresses
them. When singles arrive first, both arms retain them because saving a covering
aggregate does not remove existing raw entries. This latter ordering better
matches the bounded slot-96 ledger order, though it still does not reconstruct
the pool snapshot.

Running the real `AggregateUnaggregatedAttestations` operation over the retained
mixed pool took 561.554 ms for 5,000 singles and 1.066 s for 13,000. Full packing
after that compaction fell to 14.741 and 16.148 ms. The historical service
schedules compaction at 7.0, 9.5, and 11.8 seconds into the slot; these are
scheduled times and do not show when an overloaded process completed them.

### Included/seen singles lifecycle

Both arms first store covered Electra singles, save old Gloas versus normalized
Electra aggregates, and call the same Electra inclusion-prune operation. In the
old arm, `DeleteAggregatedAttestation` inserts Electra seen bits but misses the
differently keyed Gloas aggregate. Thus the aggregate remains, while the raw
single backing entries remain physically present but the proposer getter
returns zero eligible singles. The normalized arm removes the aggregate and
also records the seen bits. Reoffering the singles does not make them eligible
in either arm.

`UnaggregatedAttestations` still scans every physical entry to check its seen
bit. For 5,000 entries this cost 7.68-7.80 ms; for 13,000 it cost 23.45-25.44
ms. After `DeleteSeenUnaggregatedAttestations` physically removed them, the
getter took 0.20-0.27 microseconds. The normalized arm's full pack fell from
7.820 ms to 0.0021 ms for 5,000 and from 25.505 ms to 0.0022 ms for 13,000.
The retained arm still processed its compact aggregates, falling from
21.229/34.056 ms to 9.394/10.628 ms. See
[mixed-pool-lifecycle.md](mixed-pool-lifecycle.md) for the audited source
sequence and timing evidence.

## Profile and production source cause

The 13,000-single full-pack profile reran the exact-old benchmark three times:
3.413 seconds/op, 371.17 MB/op, and 897,653 allocations/op. The raw profile is
[old028-heavy13k-fullpack.cpu.pprof](old028-heavy13k-fullpack.cpu.pprof)
(`SHA-256 ee0e9379e185f68b1a2d65705c8228499438afed54dd21a57335d8ab3a783d28`).
The focused text is
[old028-heavy13k-fullpack.pprof-focus.txt](old028-heavy13k-fullpack.pprof-focus.txt)
(`SHA-256 e25b89ce7f82f827470c0fa78fe7d9afe5a0c1474c60c1df6242d6d27449ae5c`).

The exact-old production path begins at
[proposer_attestations.go](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/rpc/prysm/v1alpha1/validator/proposer_attestations.go:32).
It reads and validates aggregated and unaggregated pool candidates at lines
40-46, before version conversion/deduplication at lines 59-110, reward sorting
at line 117, and the final block limit at line 128. The pairwise containment
implementation is
[dedup](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/rpc/prysm/v1alpha1/validator/proposer_attestations.go:373),
and validation is
[validateAndDeleteAttsInPool](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/rpc/prysm/v1alpha1/validator/proposer_attestations.go:419).

Within the focused descendants of `packAttestations`, pprof sampled 13.49 CPU
seconds across fixture preflight plus the three timed calls.
`proposerAtts.dedup` accumulated 6.57 s
(about 49% of focused packer CPU), including 5.80 s in `Bitlist.Contains`.
Aggregate processing accumulated 6.32 s (about 47%), with 5.01 s in
`MaxCover`, 1.92 s in `Overlaps`, 1.77 s in `AndCount`, and 0.80 s in `Count`.
Signature decode/uncompress accumulated about 1.25 s, validation 0.48 s, and
pool snapshot 0.09 s. Cumulative CPU values overlap through the call tree and
can exceed wall time; they identify the dominant path rather than additive
elapsed stages.

The whole profile contains fixture preflight/setup and the three timed calls
because `go test -cpuprofile` profiles the complete process. Its whole-process
percentages are therefore contaminated by untimed setup, especially key
generation. The published view uses `focus=packAttestations` and
`show_from=packAttestations`, excluding setup that is not a descendant of the
production packer.

The version-key lifecycle is implemented in
[DeleteAggregatedAttestation](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/operations/attestations/kv/aggregated.go:248),
the physical-entry scan in
[UnaggregatedAttestations](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/operations/attestations/kv/unaggregated.go:55),
and physical seen-entry cleanup in
[DeleteSeenUnaggregatedAttestations](/home/sukun/dev/prysm2-round2-construction-028/beacon-chain/operations/attestations/kv/unaggregated.go:157).

## Commands and artifacts

The final benchmark selections were run serially from the exact-old workspace:

```bash
GOMAXPROCS=4 go test ./beacon-chain/rpc/prysm/v1alpha1/validator \
  -run '^$' \
  -bench '^(BenchmarkDiagnosticHistoricalLatePackingSlot97|BenchmarkHistoricalStateRoot(Compact3|Density8)Block120K)$' \
  -benchtime=20x -count=1 -timeout=10m

GOMAXPROCS=4 go test ./beacon-chain/rpc/prysm/v1alpha1/validator \
  -run '^$' \
  -bench '^(BenchmarkDiagnosticHistoricalLateRawDensityControls|BenchmarkDiagnosticHistoricalLateMixedPools|BenchmarkDiagnosticHistoricalLateSeenBacklog)$' \
  -benchtime=5x -count=1 -timeout=15m

GOMAXPROCS=4 go test ./beacon-chain/rpc/prysm/v1alpha1/validator \
  -run '^$' \
  -bench '^BenchmarkDiagnosticHistoricalFullBuildParallel/full_parent_(compact3|density8)$' \
  -benchtime=20x -count=1 -timeout=15m

GOMAXPROCS=4 PRYSM_HISTORICAL_VALIDATION_LIMIT=14355 \
  go test ./beacon-chain/sync \
  -run '^TestDiagnosticHistoricalPacedValidationFullBuild$' \
  -count=3 -v -timeout=10m

GOMAXPROCS=1 PRYSM_HISTORICAL_VALIDATION_LIMIT=14355 \
  go test ./beacon-chain/sync \
  -run '^TestDiagnosticHistoricalPacedValidationFullBuild$' \
  -count=1 -v -timeout=5m


# Direct paced-only SSE comparison; run once each with GOMAXPROCS=4 and 1.
GOMAXPROCS=4 PRYSM_HISTORICAL_VALIDATION_LIMIT=14355 \
  go test ./beacon-chain/sync \
  -run '^TestDiagnosticHistoricalPacedValidationFullBuild/paced_14355$' \
  -count=1 -v -timeout=5m

GOMAXPROCS=4 PRYSM_HISTORICAL_VALIDATION_LIMIT=14355 PRYSM_HISTORICAL_SSE=1 \
  go test ./beacon-chain/sync \
  -run '^TestDiagnosticHistoricalPacedValidationFullBuild/paced_14355$' \
  -count=1 -v -timeout=5m


GOMAXPROCS=1 PRYSM_HISTORICAL_VALIDATION_LIMIT=14355 \
  go test ./beacon-chain/sync \
  -run '^TestDiagnosticHistoricalPacedValidationFullBuild/paced_14355$' \
  -count=1 -v -timeout=5m

GOMAXPROCS=1 PRYSM_HISTORICAL_VALIDATION_LIMIT=14355 PRYSM_HISTORICAL_SSE=1 \
  go test ./beacon-chain/sync \
  -run '^TestDiagnosticHistoricalPacedValidationFullBuild/paced_14355$' \
  -count=1 -v -timeout=5m
```

The exact CPU-profile command was:

```bash
GOMAXPROCS=4 go test ./beacon-chain/rpc/prysm/v1alpha1/validator \
  -run '^$' \
  -bench '^BenchmarkDiagnosticHistoricalLateRawDensityControls/heavy_13000_singles_6_committees/full_pack_attestations$' \
  -benchtime=3x -count=1 -timeout=10m \
  -o /home/sukun/dev/prysm2/runs/diagnostics/round2-construction-bench/validator.test \
  -cpuprofile /home/sukun/dev/prysm2/runs/diagnostics/round2-construction-bench/old028-heavy13k-fullpack.cpu.pprof

go tool pprof -top -nodecount=80 \
  -focus='packAttestations' -show_from='packAttestations' \
  /home/sukun/dev/prysm2/runs/diagnostics/round2-construction-bench/old028-heavy13k-fullpack.cpu.pprof
```

Correctness and shape checks passed before final timing. The main records are:

- [packing fixture preflight](old028-packing-preflight.log)
- [ordinary slot-97 preflight](old028-packing-slot97-preflight-1x.log)
- [mixed-pool preflight](old028-mixed-pools-preflight-1x.log)
- [seen-backlog preflight](old028-seen-backlog-preflight-1x.log)
- [compact-3 state pilot](old028-state-root-compact3-pilot.log)
- [density-8 state pilot](old028-state-root-density8-pilot.log)
- [corrected full-parent construction](old028-full-build-full-parent-final-20x.log)
- [superseded full-build pilot](old028-full-build-parallel-20x.log)
- [paced validation, three `GOMAXPROCS=4` pairs](old028-paced-validation-full-paired-count3.log)
- [paced validation, `GOMAXPROCS=1` sensitivity](old028-paced-validation-full-paired-gomaxprocs1.log)
- [paced validation + SSE, P4 without streams](old028-sse-comparison-p4-nosse.log)
- [paced validation + SSE, P4 with 18 streams](old028-sse-comparison-p4-sse18.log)
- [paced validation + SSE, P1 without streams](old028-sse-comparison-p1-nosse.log)
- [paced validation + SSE, P1 with 18 streams](old028-sse-comparison-p1-sse18.log)
- [exact Xatu and SSE source audit](xatu-sse-contention-audit.md)
- [full 14,355-row validation schedule](node1-slot97-gossip-validation-schedule.json)
- [profile benchmark output](old028-heavy13k-fullpack-profile.log)
- [unfocused profile top](old028-heavy13k-fullpack.pprof-top.txt)
- [fixture shape](actual_ffg_fixture_shape.json) and
  [aggregate input bounds](aggregate-input-bounds.md)

The preserved
[failed mixed-pool pilot](old028-mixed-pools-preflight-1x.failed-coverage-assertion.log)
is not benchmark evidence. Its assertion expected the 5,000/13,000 raw subset
as total fresh coverage, while the six compact fresh aggregates correctly cover
15,000 positions. The final fixture asserts the full 15,000 coverage and that
every raw-subset participant remains covered.
