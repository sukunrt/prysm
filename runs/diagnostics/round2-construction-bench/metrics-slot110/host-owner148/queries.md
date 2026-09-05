# Owner148 physical-host Prometheus query manifest

Datasource: `devnets`

All selectors are scoped to
`network="glamsterdam-devnet-9",ip_address="143.198.65.112",job="node"`.
The evaluation time is `2026-09-05T01:54:00Z`; each range selector is `[6m]`.
These instant range-vector queries preserve Prometheus's native timestamps
rather than resampling through `query_range`.

```promql
node_cpu_seconds_total{network="glamsterdam-devnet-9",ip_address="143.198.65.112",job="node"}[6m]
```

```promql
{__name__=~"node_load1|node_load5|node_load15|node_memory_MemAvailable_bytes|node_pressure_cpu_waiting_seconds_total|node_pressure_io_waiting_seconds_total|node_pressure_io_stalled_seconds_total|node_pressure_memory_waiting_seconds_total|node_pressure_memory_stalled_seconds_total|node_vmstat_oom_kill|node_vmstat_pgmajfault",network="glamsterdam-devnet-9",ip_address="143.198.65.112",job="node"}[6m]
```

```promql
{__name__=~"node_disk_io_time_seconds_total|node_disk_io_time_weighted_seconds_total|node_disk_read_bytes_total|node_disk_written_bytes_total|node_disk_read_time_seconds_total|node_disk_write_time_seconds_total|node_disk_reads_completed_total|node_disk_writes_completed_total",network="glamsterdam-devnet-9",ip_address="143.198.65.112",job="node"}[6m]
```

Reproduction command shape:

```bash
panda -o json prometheus query devnets '<PROMQL>' \
  --time '2026-09-05T01:54:00Z'
```

