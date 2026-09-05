#!/usr/bin/env python3
"""Produce a compact, deterministic summary from preserved Panda query JSON."""

from __future__ import annotations

import datetime as dt
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
UTC = dt.timezone.utc


def results(name: str) -> list[dict]:
    obj = json.loads((HERE / name).read_text())
    return obj["data"]["result"]


def series(name: str, metric: str, job: str | None = None) -> dict:
    matches = [
        item
        for item in results(name)
        if item["metric"].get("__name__") == metric
        and (job is None or item["metric"].get("job") == job)
    ]
    if len(matches) != 1:
        raise ValueError(f"expected one {metric}/{job} series in {name}, got {len(matches)}")
    return matches[0]


def at(item: dict, timestamp: str) -> float:
    target = dt.datetime.fromisoformat(timestamp.replace("Z", "+00:00")).timestamp()
    for sample_ts, value in item["values"]:
        if abs(sample_ts - target) < 0.001:
            return float(value)
    raise ValueError(f"sample {timestamp} not found")


def iso(unix: float) -> str:
    return dt.datetime.fromtimestamp(unix, UTC).isoformat(timespec="milliseconds").replace("+00:00", "Z")


version = results("prysm-version-owner-ip-at-slot110.json")[0]["metric"]
exporter_version = results("eth-con-node-version-owner-ip-at-slot110.json")[0]["metric"]
start = series("process-start-native-owner-ip.json", "process_start_time_seconds", "consensus_node")
head = series("head-slots-native-owner-ip.json", "beacon_head_slot", "consensus_node")
cpu = series("process-runtime-native-owner-ip.json", "process_cpu_seconds_total", "consensus_node")
rss = series("process-runtime-native-owner-ip.json", "process_resident_memory_bytes", "consensus_node")
goroutines = series("process-runtime-native-owner-ip.json", "go_goroutines", "consensus_node")
goroutines_wide = series("go-goroutines-native-0148-0155.json", "go_goroutines", "consensus_node")
gc_count = series("go-runtime-native-owner-consensus.json", "go_gc_duration_seconds_count", "consensus_node")
gc_sum = series("go-runtime-native-owner-consensus.json", "go_gc_duration_seconds_sum", "consensus_node")
last_gc = series("go-runtime-native-owner-consensus.json", "go_memstats_last_gc_time_seconds", "consensus_node")
gomaxprocs = series("go-runtime-native-owner-consensus.json", "go_sched_gomaxprocs_threads", "consensus_node")

docker = results("docker-runtime-native-owner-ip.json")


def docker_series(metric: str, kind: str) -> dict:
    matches = [x for x in docker if x["metric"].get("__name__") == metric and x["metric"].get("type") == kind]
    if len(matches) != 1:
        raise ValueError(f"expected one docker {metric}/{kind}, got {len(matches)}")
    return matches[0]


grpc_all = results("grpc-block-native-owner-consensus.json")


def grpc_series(metric: str, method: str) -> dict:
    matches = [x for x in grpc_all if x["metric"].get("__name__") == metric and x["metric"].get("grpc_method") == method]
    if len(matches) != 1:
        raise ValueError(f"expected one grpc {metric}/{method}, got {len(matches)}")
    return matches[0]


grpc_count = grpc_series("grpc_server_handling_seconds_count", "GetBeaconBlock")
grpc_sum = grpc_series("grpc_server_handling_seconds_sum", "GetBeaconBlock")

before = "2026-09-05T01:52:02.460Z"
after = "2026-09-05T01:52:32.460Z"
cpu_delta = at(cpu, after) - at(cpu, before)
gc_count_delta = at(gc_count, after) - at(gc_count, before)
gc_sum_delta = at(gc_sum, after) - at(gc_sum, before)

summary = {
    "incident": {
        "slot": 110,
        "slot_start": "2026-09-05T01:52:00Z",
        "owner": "round2 prysm-geth-148",
        "owner_external_ip": "143.198.65.112",
    },
    "identity": {
        "selector": 'ip_address="143.198.65.112",network="glamsterdam-devnet-9"',
        "stored_instance_label": version["instance"],
        "stored_consensus_client_label": version["consensus_client"],
        "stored_execution_client_label": version["execution_client"],
        "prysm_commit": version["commit"],
        "prysm_version": version["version"],
        "exporter_node_version": exporter_version["version"],
        "consensus_process_start": iso(float(start["values"][0][1])),
        "interpretation": "direct process metrics identify the deployed Prysm despite stale instance/client labels",
    },
    "native_scrape": {
        "cadence_seconds": 30.0,
        "consensus_offset_samples": [iso(x[0]) for x in head["values"]],
        "exporter_offset_example": "2026-09-05T01:52:15.867Z",
    },
    "head": [[iso(ts), int(float(value))] for ts, value in head["values"]],
    "goroutines_native_window": [[iso(ts), int(float(value))] for ts, value in goroutines_wide["values"]],
    "incident_bracket_consensus_process": {
        "start": before,
        "end": after,
        "cpu_seconds_delta": cpu_delta,
        "average_cores_over_30s": cpu_delta / 30.0,
        "rss_bytes_start": at(rss, before),
        "rss_bytes_end": at(rss, after),
        "goroutines_start": at(goroutines, before),
        "goroutines_end": at(goroutines, after),
        "gc_cycles_delta": gc_count_delta,
        "gc_pause_seconds_delta": gc_sum_delta,
        "last_completed_gc_at_start_sample": iso(at(last_gc, before)),
        "last_completed_gc_at_end_sample": iso(at(last_gc, after)),
        "gomaxprocs": at(gomaxprocs, before),
    },
    "exporter_sample_during_incident": {
        "timestamp": "2026-09-05T01:52:15.867Z",
        "beacon_container_cpu_usage_percent": at(docker_series("eth_docker_cpu_usage_percent", "consensus"), "2026-09-05T01:52:15.867Z"),
        "beacon_container_memory_usage_bytes": at(docker_series("eth_docker_memory_usage_bytes", "consensus"), "2026-09-05T01:52:15.867Z"),
        "beacon_container_memory_limit_bytes": at(docker_series("eth_docker_memory_limit_bytes", "consensus"), "2026-09-05T01:52:15.867Z"),
        "online_cpus": at(docker_series("eth_docker_cpu_online_count", "consensus"), "2026-09-05T01:52:15.867Z"),
        "throttle_periods_total": at(docker_series("eth_docker_cpu_throttle_periods_total", "consensus"), "2026-09-05T01:52:15.867Z"),
        "throttled_periods_total": at(docker_series("eth_docker_cpu_throttled_periods_total", "consensus"), "2026-09-05T01:52:15.867Z"),
        "throttled_time_nanoseconds": at(docker_series("eth_docker_cpu_throttled_time_nanoseconds", "consensus"), "2026-09-05T01:52:15.867Z"),
    },
    "grpc_get_beacon_block_incident_bracket": {
        "count_delta": at(grpc_count, after) - at(grpc_count, before),
        "sum_seconds_delta": at(grpc_sum, after) - at(grpc_sum, before),
    },
    "pool_samples": {},
    "limits": [
        "The stored instance and client labels are stale; identity depends on commit, process-start, and head progression cross-checks.",
        "Thirty-second native scrapes cannot isolate CPU, memory, pool, or GC behavior during the 3.4-second build.",
        "Pool values are periodic gauges, not the proposal's exact pool snapshot.",
    ],
}

for metric in [
    "aggregated_attestations_in_pool_total",
    "unaggregated_attestations_in_pool_total",
    "seen_aggregated_attestations_in_pool_total",
]:
    item = series("attestation-pool-native-owner-consensus.json", metric, "consensus_node")
    summary["pool_samples"][metric] = [[iso(ts), int(float(value))] for ts, value in item["values"]]

(HERE / "slot110-prometheus-summary.json").write_text(json.dumps(summary, indent=2) + "\n")
