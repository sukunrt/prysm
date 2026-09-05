# Round 2 slot 110 Prometheus audit

## Scope and access

This is a read-only historical query of Panda's `devnets` Prometheus/VictoriaMetrics datasource around Round 2 slot 110. Slot 110 started at `2026-09-05T01:52:00Z`; node 148 entered block construction at `01:52:00.016984Z` and finished at `01:52:03.409893Z`.

The reproducible commands are in [`query_slot110_prometheus.sh`](query_slot110_prometheus.sh). They use instant PromQL range vectors such as `metric{...}[4m]`, which preserve native sample timestamps. `panda prometheus query-range --step 1s` would evaluate the expression every second but would not create one-second source observations. The native cadence for these targets is exactly 30 seconds, with consensus samples at `...:02.460Z`/`...:32.460Z` and exporter samples at `...:15.867Z`/`...:45.867Z`.

## Process identity despite stale inventory labels

The retained owner log identifies external IP `143.198.65.112` and the deployed binary at [`beacon.log:1`](../round2-slot110-owner-node148/beacon.log#L1) and [`beacon.log:37`](../round2-slot110-owner-node148/beacon.log#L37). At the incident, Prometheus stored that IP under:

```
network="glamsterdam-devnet-9"
instance="glamsterdam-devnet-9-lighthouse-reth-6"
consensus_client="lighthouse"
execution_client="reth"
```

The instance and client labels are stale. Direct process metrics identify the process as the Round 2 owner:

- `prysm_version` reports commit `0280403c70d88967f49d2d4c730f4c5417dabdf5`, version `v5.0.3`.
- The exporter reports `Prysm/v5.0.3-0280403 (linux amd64)`.
- Consensus `process_start_time_seconds` is `2026-09-05T00:50:34.620Z`; the retained Prysm startup line is `00:50:35.254Z`.
- `beacon_head_slot` is 109 at `01:52:02.460Z` and 112 at `01:52:32.460Z`. The exporter observes head 110 at `01:52:15.867Z`, consistent with retained logs importing slot 110 at `01:52:03.663Z`.

The raw identity and head records are [`prysm-version-owner-ip-at-slot110.json`](prysm-version-owner-ip-at-slot110.json), [`eth-con-node-version-owner-ip-at-slot110.json`](eth-con-node-version-owner-ip-at-slot110.json), [`process-start-native-owner-ip.json`](process-start-native-owner-ip.json), and [`head-slots-native-owner-ip.json`](head-slots-native-owner-ip.json). IP alone would not establish identity on this reused host; the commit, process-start, and head progression provide the cross-check.

## Runtime observations

The closest consensus scrape is `01:52:02.460Z`, 2.460 seconds after slot start and while the 3.410-second block request was still running.

| Metric | `01:52:02.460Z` | `01:52:32.460Z` | Bound |
|---|---:|---:|---|
| `process_cpu_seconds_total` | 4017.00 s | 4072.36 s | +55.36 CPU-s in 30 s, or 1.845 cores averaged across the interval |
| `go_goroutines` | 15,438 | 870 | Prior sample at `01:51:32.460Z` was 851; this records a concurrency spike during construction |
| process RSS | 2.998 GB | 3.096 GB | No memory-limit pressure in this coarse view |
| GC count | 216 | 219 | Three completed cycles somewhere in the 30-second interval |
| cumulative GC pause | 0.293462 s | 0.299895 s | +6.433 ms across the interval |
| `GOMAXPROCS` | 8 | 8 | Eight Go scheduler processors |

The 60-second native bracket enclosing the whole build, `01:51:32.460Z` to `01:52:32.460Z`, accumulated 122.49 CPU-seconds (2.042 cores on average), five completed GC cycles, and 8.629 ms total recorded GC pause. The last-GC gauge at `01:52:02.460Z` names a completion at `01:51:51.606Z`; the next scrape names the last of the new cycles at `01:52:26.891Z`. These samples do not place each GC within the interval, but their aggregate pause excludes seconds of recorded stop-the-world GC as the explanation.

The broader `01:48`–`01:55` series shows 882 goroutines at slot 90 +2.460 s, 847 at slot 95 +2.460 s, 3,821 at slot 100 +2.460 s, 849 at slot 105 +2.460 s, **15,438 at slot 110 +2.460 s**, and 870 at slot 115 +2.460 s. Thus the slot 110 sample is the largest nearby same-phase sample; the metric does not identify what created those goroutines or whether they delayed construction. See [`go-goroutines-native-0148-0155.json`](go-goroutines-native-0148-0155.json).

The exporter recorded the beacon container at `610.94%` CPU and 4.986 GB memory at `01:52:15.867Z`, with eight online CPUs and a 33.662 GB memory limit. This CPU gauge's source averaging window is not encoded in the series, so it is evidence of high activity around the incident rather than a measurement of the 3.4-second build itself. All three CPU throttle counters remained zero in every preserved exporter sample from `01:50:15.867Z` through `01:53:45.867Z`. The metrics therefore show no recorded CPU-quota throttling during that window; they do not show whether a quota was configured but never hit.

The queried `eth_docker_*` family exposes only `container_name="beacon"` and `container_name="execution"` on this IP. It contains no Xatu/sentry container series. That absence means these exporter metrics cannot bound Xatu CPU or memory; it is not evidence that Xatu was idle.

Raw runtime records are [`process-runtime-native-owner-ip.json`](process-runtime-native-owner-ip.json), [`go-runtime-native-owner-consensus.json`](go-runtime-native-owner-consensus.json), and [`docker-runtime-native-owner-ip.json`](docker-runtime-native-owner-ip.json).

## Block RPC and pool observations

Between consensus scrapes at `01:52:02.460Z` and `01:52:32.460Z`, `grpc_server_handling_seconds_count{grpc_method="GetBeaconBlock"}` increases by one and its cumulative sum increases by **3.394066093 seconds**. The histogram moves from one to two calls in `+Inf` and remains at one in the `le="2.5"` bucket. This independently records the slot 110 call as lasting between 2.5 and 5 seconds and matches the 3.393-second build span in the retained log. See [`grpc-block-native-owner-consensus.json`](grpc-block-native-owner-consensus.json).

At the in-build consensus scrape (`01:52:02.460Z`), the exposed pool gauges were:

- `aggregated_attestations_in_pool_total = 114`
- `unaggregated_attestations_in_pool_total = 1775`
- `seen_aggregated_attestations_in_pool_total = 864`

These are not an exact proposal snapshot. In the deployed code they are refreshed by the slot+11 expiry ticker. The aggregate gauge counts map keys (`DataVersionID` groups), and a key may contain more than one aggregate object; the unaggregated gauge counts physical map entries. The values nevertheless show substantial pool state and align temporally with the 15,438-goroutine scrape. The full native series is [`attestation-pool-native-owner-consensus.json`](attestation-pool-native-owner-consensus.json).

## What the metrics establish

The metrics independently verify that Panda retained the exact old Prysm owner process and the slow slot 110 `GetBeaconBlock` call. They show a large goroutine spike and high container activity around construction, while showing ample memory, eight CPUs, zero container throttle counters, and only milliseconds of aggregate recorded GC pause. Goroutine count combines runnable work with blocked and waiting goroutines, so the count alone does not prove CPU contention.

They do not attribute the 3.394 seconds to a specific Prysm stage. The 30-second cadence folds construction together with gossip validation, subscriber work, and other activity; pool gauges are asynchronously refreshed. These observations establish substantial concurrent activity and rule out recorded quota throttling or seconds of recorded GC pause, but they do not identify the historical causal scheduler or callback path.

Machine-readable derived values are in [`slot110-prometheus-summary.json`](slot110-prometheus-summary.json), generated by [`summarize_slot110_metrics.py`](summarize_slot110_metrics.py).
