#!/usr/bin/env bash
set -euo pipefail

# Read-only historical queries for Round 2 owner node 148 at slot 110.
# query-range evaluates at the requested step; these instant range-vector
# queries preserve the samples at their native scrape timestamps instead.
PANDA=${PANDA:-/home/sukun/.local/bin/panda}
DATASOURCE=${DATASOURCE:-devnets}
OUT=${OUT:-"$(cd "$(dirname "$0")" && pwd)"}
IP=143.198.65.112
NETWORK=glamsterdam-devnet-9
AT=2026-09-05T01:52:03Z
END=2026-09-05T01:55:00Z

query() {
  local name=$1
  local expression=$2
  local time=$3
  "$PANDA" -o json prometheus query "$DATASOURCE" "$expression" --time "$time" > "$OUT/$name.json"
}

query prysm-version-owner-ip-at-slot110 \
  "prysm_version{ip_address=\"$IP\",network=\"$NETWORK\"}" "$AT"
query eth-con-node-version-owner-ip-at-slot110 \
  "eth_con_node_version{ip_address=\"$IP\",network=\"$NETWORK\"}" "$AT"
query process-start-native-owner-ip \
  "process_start_time_seconds{ip_address=\"$IP\",network=\"$NETWORK\"}[3m]" \
  2026-09-05T01:53:00Z
query head-slots-native-owner-ip \
  "{__name__=~\"beacon_head_slot|doublylinkedtree_head_slot|eth_con_sync_head_slot\",ip_address=\"$IP\",network=\"$NETWORK\"}[3m]" \
  2026-09-05T01:53:00Z
query process-runtime-native-owner-ip \
  "{__name__=~\"process_cpu_seconds_total|process_resident_memory_bytes|go_goroutines|go_memstats_heap_alloc_bytes|go_memstats_heap_inuse_bytes|go_memstats_heap_sys_bytes\",ip_address=\"$IP\",network=\"$NETWORK\"}[4m]" \
  2026-09-05T01:54:00Z
query go-goroutines-native-0148-0155 \
  "go_goroutines{ip_address=\"$IP\",network=\"$NETWORK\",job=\"consensus_node\"}[7m]" "$END"
query go-runtime-native-owner-consensus \
  "{__name__=~\"go_sched_gomaxprocs_threads|go_threads|go_gc_duration_seconds_count|go_gc_duration_seconds_sum|go_memstats_gc_cpu_fraction|go_memstats_last_gc_time_seconds\",ip_address=\"$IP\",network=\"$NETWORK\",job=\"consensus_node\"}[4m]" \
  2026-09-05T01:54:00Z
query docker-runtime-native-owner-ip \
  "{__name__=~\"eth_docker_cpu_usage_percent|eth_docker_cpu_online_count|eth_docker_cpu_throttle_periods_total|eth_docker_cpu_throttled_periods_total|eth_docker_cpu_throttled_time_nanoseconds|eth_docker_memory_limit_bytes|eth_docker_memory_usage_bytes|eth_docker_process_count\",ip_address=\"$IP\",network=\"$NETWORK\"}[4m]" \
  2026-09-05T01:54:00Z
query attestation-pool-native-owner-consensus \
  "{__name__=~\"aggregated_attestations_in_pool_total|unaggregated_attestations_in_pool_total|seen_aggregated_attestations_in_pool_total\",ip_address=\"$IP\",network=\"$NETWORK\",job=\"consensus_node\"}[7m]" "$END"
query grpc-block-native-owner-consensus \
  "{__name__=~\"grpc_server_handling_seconds_count|grpc_server_handling_seconds_sum|grpc_server_handling_seconds_bucket\",ip_address=\"$IP\",network=\"$NETWORK\",job=\"consensus_node\",grpc_method=~\"GetBeaconBlock|ProposeBeaconBlock\"}[4m]" \
  2026-09-05T01:54:00Z

python3 "$OUT/summarize_slot110_metrics.py"
