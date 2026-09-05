# Bounded owner-local preflight comparison

This is an additional historical evidence pass, not a reproduction. Source revision is `0280403c70d88967f49d2d4c730f4c5417dabdf5`, read with `jj --ignore-working-copy file show`. Archive anchors below abbreviate `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-N.tar.gz::validator.log:L` as `N VC:L`, and likewise `BN` for `beacon.log`. The new derived tables are useful indexes; raw records and exact source determine the claims.

## Positive owner differences

All seven owners completed slot-0 attester and sync-message work. This excludes initial validator activation/readiness or an inability to use the signing domain from genesis as a common explanation of their later failures. Their successful slot-0 sync-message counts were 2, 3, 3, 4, 4, 2, and 1 respectively. These are observed successful-message counts, not a separately logged membership census.

| Owned slot / node | Slot-0 completed work and anchors | Later discriminating progress before owned failure |
|---|---|---|
| 1 / 169 | 65 attestations, 2 sync messages, 1 sync contribution; VC:681–685, report by +9.019s | Slot-1 PTC NotFound at +21.915s proves the batch had dispatched; slot-1 sync-aggregator error VC:708 proves successful preflight selection/domain access earlier in that same batch. See `deeper-preflight.md`. |
| 2 / 191 | 85 attestations, 3 sync messages; VC:682–685, report by +9.006s | VC:686–688 at +24.000860 to +24.000917s fail at **SubmitSyncMessage**, which is after sync-block-root, domain-data and local signature success. These are prior slot-1 calls, not evidence that owned slot-2 preflight made progress. Owned preflight is VC:770 at +36.001290s. |
| 3 / 22 | 77 attestations, 3 sync messages, 1 contribution; VC:682–686, report by +9.008s | Slot-1 attestations and 2 sync messages reported VC:694–695 at +24.002820/+24.003026s; no corresponding successful slot-2 summary before owned preflight VC:775 at +48.000619s. |
| 4 / 91 | 76 attestations, 4 sync messages, 1 contribution; VC:682–686, report by +9.015s | Slot-1 94 attestations and 4 sync messages reported VC:692–693 at +24.003465s. Own slot-4 failure VC:832 is selection **signing**, so at least one own-batch sync-index response succeeded before its domain-data failure. Direct keymanager makes its gRPC status a domain-data failure, not a remote-signing failure. |
| 7 / 144 | 68 attestations, 4 sync messages; VC:682–685, report by +9.012s | Slot-1 73 attestations and 2 sync messages reported VC:693–694 at +24.002220s; no successful immediately preceding slot-6 summary. Owned preflight VC:1110 expires +96.001455s. |
| 11 / 117 | 88 attestations, 2 sync messages; VC:682–684, report by +8.024s | **Immediately previous summary batch 10:** 65 successful attestation records, first captured at wall-slot-10 offset 7.921s, spread 70ms (VC:1111); 2 explicitly slot-10 sync messages succeeded (VC:1112). Reports +132.003456/+132.003471s precede owned preflight VC:1113 at +144.001546s. The grouped attestation count does not prove that all 65 requested slot-10 duties completed. |
| 12 / 14 | 84 attestations, 1 sync message; VC:682–685, report by +9.006s | **Immediately previous slot 11:** PTC NotFound twice at +142.022132/+142.022347s (VC:1277–1278), then one successful sync message reported +144.021866s (VC:1367). Own preflight VC:1368 expires +156.000792s. |

Node 117's successful attestation RPCs were recorded approximately +127.921 to +127.991s, not at their +132.003s summary timestamp. The attestation grouping key omits the attestation data slot (`log_helpers.go:56–74`), and the displayed slot comes from the `LogSubmissions` caller. Overlapping work can therefore mix duty slots in one summary; the capture times establish successful RPC progress, not unique slot-10 duty completion. Sync-message grouping does include the actual message slot (`:146–154`), so its slot attribution is stronger. Sync summaries do not carry actual action timestamps: they give completion upper bounds only. `runner.go:273–285` waits for the batch's role goroutines before calling `LogSubmissions`. The immediately preceding successful sync roles for 117 and 14 prove the root/domain/signature/submission path was working in the preceding batch. They do not exclude other goroutines overlapping the next batch.

For node 191, the error stage is stronger evidence than an absence of successful summaries: exact `validator/client/sync_committee.go:50–95` orders root RPC, duty lookup, domain access, local signing, then submission; error line 96 corresponds to VC:686–688. BN:512–513 later discard slot-1 messages as too old at +25.898803/+25.899032s. This proves late slot-1 message processing while owned slot 2 was already underway. The missing per-request identity and async broadcast boundary prevent turning those BN records into the duration of a particular VC request.

The six preflight terminal pairs still have the exact synchronous causal edge: `validator.go:646–649` returns the collected roles after sync-preflight failure, then `runner.go:147–156` dispatches them with the same absolute deadline. None of the owner comparisons supplies the sync-index **entry** timestamp, so none turns an endpoint at +36/+48/+96/+144/+156 into twelve seconds spent inside that index RPC.

## Readiness, connection and peers

Each of the seven archives has an initial `gRPC client connected to beacon node` at BN:39 between 00:50:41 and 00:51:22 UTC, approximately 39 minutes before genesis. The bounded archive scan found no new connection record in 01:30–01:32. For node 169, BN:21 explicitly says insecure gRPC and BN:30 says genesis has not arrived, not syncing; its later successful slot-0 batch establishes readiness after genesis. No owned-window initial-sync progress message supplies a hidden startup wait that explains these seven failures.

Peer samples remain substantial through the early failure window: 169 BN:521 at 01:30:40 reports 92; 191 BN:517 at 01:30:47 reports 115; 22 BN:511 at 01:30:50 reports 98; 91 BN:534 at 01:30:20 reports 82; 144 BN:512 at 01:30:48 reports 99; 117 BN:525 at 01:31:41 reports 104; 14 BN:512 at 01:31:52 reports 95. Some are after the owned failure and are explicitly not instantaneous deadline measurements. These samples and completed local RPCs do not support a total connectivity outage; they do not measure peer throughput or rule out CPU contention.

## Is DomainData really constant work?

The RANDAO branch of `beacon-chain/rpc/prysm/v1alpha1/validator/server.go:176–201` is bounded configuration/genesis-root/domain computation; it does not read the head state, walk the validator registry, acquire forkchoice locks, or invoke the EL. The voluntary-exit-only head-state branch at line 180 is inapplicable.

That handler is not the whole server path. Exact `beacon-chain/rpc/service.go:163–194` installs OpenTelemetry stats plus recovery, Prometheus, OpenTracing, and connection-logging interceptors. It installs no application auth interceptor, concurrency limiter, circuit breaker, or application `MaxConcurrentStreams` limit. The connection interceptor at `425–432` calls `logNewClientConnection`; `435–449` acquires `clientConnectionLock` even for an already-known client unless connection logging is disabled. The critical section checks a map and logs only a newly seen connection; it is not a state/registry/EL operation. Historical BN:39 confirms connection logging was enabled at initial startup, and no new connection was observed around these failures.

Thus “RANDAO directly waited for a forkchoice/state walk in its handler” is excluded by source, while “constant handler therefore the whole RPC had constant latency” is also incorrect. This pass found no configured cross-service application queue that positively accounts for the time. Generic transport, interceptor, scheduling and client-side cache-lock delay remain different layers, not interchangeable explanations proven by the terminal status.

## Historical flag/config recovery result

Bounded searches covered tracked revision config/network files, the report directories, `/tmp/claude-1000/-home-sukun-dev-prysm2` task outputs and scratch YAML, and adjacent `prysm2-run-logs` configs. They did **not** recover a config with provenance to round 2 proving `PAYLOAD_ATTESTATION_DUE_BPS=7500` or a `grpc-retries` / `grpc-retry-delay` override.

One plausible-looking scratch hit was specifically rejected: `bd5d0355-eb7a-4320-8f99-e74a484f3b3a/scratchpad/kdata/genesis/config.yaml:116` has 7500, but lines 26/28 set minimum validator count 100 and minimum genesis time 1788468657; matching metadata has the same earlier timestamp. No evidence ties this artifact to the historical genesis at 1788571800. A task-output hit contains the literal template `$PAYLOAD_ATTESTATION_DUE_BPS`, not the deployed value. Adjacent old run configs and the separate startup120k fixture also have 7500; none establishes the historical effective setting.

Accordingly the node-169 PTC envelope derived from a +9s timer remains explicitly conditional on the historical run retaining the 7500 default. Likewise the retry implementation/default analysis in `deeper-preflight.md` remains source/default evidence, not evidence that historical requests actually retried five times.

## Search ledger and resulting bounds

1. Compared all seven owners' slot-0 progress and previous-batch outcomes against exact raw anchors: eliminated common initial unreadiness; found strongest immediately previous recovery on 117 and 14.
2. Reinterpreted node-191 terminal-stage errors using exact sync submission ordering: positively established successful prior-slot domain/signature access despite no successful final summary.
3. Checked raw connection/peer/startup windows: no new client connection during the failure interval; substantial peer samples, without treating them as throughput evidence.
4. Inspected exact server options and connection interceptor: excluded an application auth/limiter/circuit mechanism in that gRPC construction; corrected the scope of “constant DomainData.”
5. Checked likely historical config scratch artifacts and rejected mismatched/unproven provenance: no promotion of experiment/default settings into historical facts.
6. Audited derived-table semantics: `owner_slot_activity.tsv` first/last anchors are now chronological extrema after the extractor fix. Untagged errors grouped by wall time can belong to a preceding slot. Only explicit slot fields, adjacent terminal sequence and exact source support attribution.

There is still no positive elapsed-time lower bound for the historical sync-index RPC itself. The positive lower bounds are **stage progress** (successful index before node-91 selection signing failure; successful domain/signature before node-191 submission failure; successful prior batch on 117/14). The strongest shared causal chain is exhausted synchronous role-preflight budget followed by proposer dispatch with that already-expired absolute deadline. The precise wait that consumed the budget is not identified by these owner logs.
