#!/usr/bin/env python3
"""Derive native-scrape host rates for the node-148 physical host."""

from __future__ import annotations

import collections
import datetime as dt
import json
from pathlib import Path


HERE = Path(__file__).resolve().parent
UTC = dt.timezone.utc


def load(name: str) -> list[dict]:
    data = json.loads((HERE / name).read_text())
    return data["data"]["result"]


def iso(ts: float) -> str:
    return dt.datetime.fromtimestamp(ts, UTC).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def interval_rates(series: list[dict], key_labels: tuple[str, ...]) -> dict[tuple[float, float], dict[tuple[str, ...], float]]:
    out: dict[tuple[float, float], dict[tuple[str, ...], float]] = collections.defaultdict(dict)
    for item in series:
        key = tuple(item["metric"].get(label, "") for label in key_labels)
        values = item["values"]
        for (t0, v0), (t1, v1) in zip(values, values[1:]):
            out[(float(t0), float(t1))][key] = (float(v1) - float(v0)) / (float(t1) - float(t0))
    return out


def instant_series(series: list[dict]) -> dict[str, list[tuple[float, float]]]:
    return {
        item["metric"]["__name__"]: [(float(ts), float(value)) for ts, value in item["values"]]
        for item in series
    }


def main() -> None:
    cpu = interval_rates(load("node-cpu-native-0148-0154.json"), ("mode", "cpu"))
    host = load("node-pressure-memory-native-0148-0154.json")
    counter_names = {
        "node_vmstat_oom_kill",
        "node_vmstat_pgmajfault",
    }
    host_rates = interval_rates(
        [
            item
            for item in host
            if item["metric"]["__name__"].endswith("_total")
            or item["metric"]["__name__"] in counter_names
        ],
        ("__name__",),
    )
    host_instants = instant_series(
        [
            item
            for item in host
            if not item["metric"]["__name__"].endswith("_total")
            and item["metric"]["__name__"] not in counter_names
        ]
    )
    disk = interval_rates(load("node-disk-native-0148-0154.json"), ("device", "__name__"))

    rows = []
    for (start, end), cpu_values in sorted(cpu.items()):
        modes: dict[str, float] = collections.defaultdict(float)
        for (mode, _cpu), value in cpu_values.items():
            modes[mode] += value
        cpu_count = len({key[1] for key in cpu_values})
        idle = modes.get("idle", 0.0)
        observed_cpu_rate = sum(modes.values())
        busy_cpu_rate = observed_cpu_rate - idle
        executing_cpu_rate = sum(modes.get(mode, 0.0) for mode in ("user", "system", "softirq", "irq", "nice"))
        disk_values = disk.get((start, end), {})
        vda = {metric: value for (device, metric), value in disk_values.items() if device == "vda"}
        rate_values = {key[0]: value for key, value in host_rates.get((start, end), {}).items()}
        rows.append(
            {
                "start": iso(start),
                "end": iso(end),
                "seconds": end - start,
                "cpu_count": cpu_count,
                "cpu_cores_by_mode": dict(sorted(modes.items())),
                "observed_cpu_seconds_per_second": observed_cpu_rate,
                "cpu_busy_cores": busy_cpu_rate,
                "cpu_busy_percent": 100.0 * busy_cpu_rate / observed_cpu_rate,
                "cpu_executing_cores": executing_cpu_rate,
                "cpu_psi_waiting_fraction": rate_values.get("node_pressure_cpu_waiting_seconds_total", 0.0),
                "memory_psi_waiting_fraction": rate_values.get("node_pressure_memory_waiting_seconds_total", 0.0),
                "memory_psi_stalled_fraction": rate_values.get("node_pressure_memory_stalled_seconds_total", 0.0),
                "io_psi_waiting_fraction": rate_values.get("node_pressure_io_waiting_seconds_total", 0.0),
                "io_psi_stalled_fraction": rate_values.get("node_pressure_io_stalled_seconds_total", 0.0),
                "major_faults_per_second": rate_values.get("node_vmstat_pgmajfault", 0.0),
                "oom_kills_per_second": rate_values.get("node_vmstat_oom_kill", 0.0),
                "vda_read_mib_per_second": vda.get("node_disk_read_bytes_total", 0.0) / (1024 * 1024),
                "vda_write_mib_per_second": vda.get("node_disk_written_bytes_total", 0.0) / (1024 * 1024),
                "vda_reads_per_second": vda.get("node_disk_reads_completed_total", 0.0),
                "vda_writes_per_second": vda.get("node_disk_writes_completed_total", 0.0),
                "vda_busy_fraction": vda.get("node_disk_io_time_seconds_total", 0.0),
                "vda_weighted_io_seconds_per_second": vda.get("node_disk_io_time_weighted_seconds_total", 0.0),
            }
        )

    result = {
        "scope": {
            "datasource": "devnets",
            "selector": {
                "network": "glamsterdam-devnet-9",
                "ip_address": "143.198.65.112",
                "job": "node",
            },
            "prometheus_instance_label": "glamsterdam-devnet-9-lighthouse-reth-6",
            "identity_note": "The instance/client labels are stale; these are physical-host node-exporter metrics selected by the owner node's public IP.",
            "evaluation": "2026-09-05T01:54:00Z",
            "range": "6m",
        },
        "events": {
            "slot96_fast_build": ["2026-09-05T01:49:12.145581Z", "2026-09-05T01:49:13.014523Z"],
            "slot110_slow_build": ["2026-09-05T01:52:00.016984Z", "2026-09-05T01:52:03.409893Z"],
            "xatu_queue_full": "2026-09-05T01:52:00.307Z",
            "prysm_sse_slow_reader": "2026-09-05T01:52:01.725Z",
        },
        "instant_samples": {
            name: [{"timestamp": iso(ts), "value": value} for ts, value in values]
            for name, values in sorted(host_instants.items())
        },
        "interval_rates": rows,
    }
    (HERE / "host-owner148-derived.json").write_text(json.dumps(result, indent=2) + "\n")

    try:
        import matplotlib.dates as mdates
        import matplotlib.pyplot as plt
        from matplotlib.lines import Line2D
        from matplotlib.patches import Patch
    except ImportError:
        return

    times = [dt.datetime.fromisoformat(row["start"].replace("Z", "+00:00")) for row in rows]
    times.append(dt.datetime.fromisoformat(rows[-1]["end"].replace("Z", "+00:00")))
    mids = [t + dt.timedelta(seconds=15) for t in times[:-1]]

    fig, axes = plt.subplots(4, 1, figsize=(12, 10), sharex=True, constrained_layout=True)
    mode_order = ["user", "system", "softirq", "steal", "iowait", "nice", "irq"]
    bottom = [0.0] * len(rows)
    for mode in mode_order:
        values = [row["cpu_cores_by_mode"].get(mode, 0.0) for row in rows]
        axes[0].bar(mids, values, width=dt.timedelta(seconds=29), bottom=bottom, label=mode)
        bottom = [a + b for a, b in zip(bottom, values)]
    axes[0].set_ylabel("non-idle CPU cores\n(30 s delta)")
    axes[0].set_ylim(0, 8)
    axes[0].legend(ncol=4, fontsize=8, loc="upper right")

    axes[1].plot(mids, [100 * row["cpu_psi_waiting_fraction"] for row in rows], marker="o", label="CPU some/waiting")
    axes[1].plot(mids, [100 * row["io_psi_waiting_fraction"] for row in rows], marker="o", label="I/O some/waiting")
    axes[1].plot(mids, [100 * row["io_psi_stalled_fraction"] for row in rows], marker="o", label="I/O full/stalled")
    axes[1].set_ylabel("PSI time rate (%)")
    axes[1].legend(fontsize=8, loc="upper right")

    load1 = host_instants["node_load1"]
    axes[2].plot([dt.datetime.fromtimestamp(t, UTC) for t, _ in load1], [v for _, v in load1], marker="o", label="load1")
    axes[2].set_ylabel("host load1")
    ax_mem = axes[2].twinx()
    mem = host_instants["node_memory_MemAvailable_bytes"]
    ax_mem.plot([dt.datetime.fromtimestamp(t, UTC) for t, _ in mem], [v / 2**30 for _, v in mem], color="tab:green", marker=".", label="MemAvailable")
    ax_mem.set_ylabel("available GiB", color="tab:green")

    axes[3].plot(mids, [row["vda_read_mib_per_second"] for row in rows], marker="o", label="vda read MiB/s")
    axes[3].plot(mids, [row["vda_write_mib_per_second"] for row in rows], marker="o", label="vda write MiB/s")
    axes[3].plot(mids, [100 * row["vda_busy_fraction"] for row in rows], marker="o", label="vda busy %")
    axes[3].set_ylabel("disk rate")
    axes[3].legend(fontsize=8, loc="upper right")

    fast_start = dt.datetime(2026, 9, 5, 1, 49, 12, 145581, tzinfo=UTC)
    fast_end = dt.datetime(2026, 9, 5, 1, 49, 13, 14523, tzinfo=UTC)
    slow_start = dt.datetime(2026, 9, 5, 1, 52, 0, 16984, tzinfo=UTC)
    slow_end = dt.datetime(2026, 9, 5, 1, 52, 3, 409893, tzinfo=UTC)
    queue_full = dt.datetime(2026, 9, 5, 1, 52, 0, 307000, tzinfo=UTC)
    slow_reader = dt.datetime(2026, 9, 5, 1, 52, 1, 725000, tzinfo=UTC)
    for axis in axes:
        axis.axvspan(fast_start, fast_end, color="tab:green", alpha=0.25, label="slot96 build")
        axis.axvspan(slow_start, slow_end, color="tab:red", alpha=0.18, label="slot110 build")
        axis.axvline(queue_full, color="tab:orange", linestyle="--", linewidth=1)
        axis.axvline(slow_reader, color="tab:red", linestyle=":", linewidth=1)
        axis.grid(alpha=0.2)
    axes[3].xaxis.set_major_formatter(mdates.DateFormatter("%H:%M:%S", tz=UTC))
    axes[3].set_xlabel("2026-09-05 UTC; points/rates have 30-second resolution")
    fig.legend(
        handles=[
            Patch(facecolor="tab:green", alpha=0.25, label="slot96 build (fast)"),
            Patch(facecolor="tab:red", alpha=0.18, label="slot110 build (slow)"),
            Line2D([0], [0], color="tab:orange", linestyle="--", label="Xatu queue full"),
            Line2D([0], [0], color="tab:red", linestyle=":", label="Prysm SSE slow reader"),
        ],
        loc="upper center",
        ncol=4,
        bbox_to_anchor=(0.5, 0.985),
        fontsize=8,
    )
    fig.suptitle("Owner node148 physical-host metrics (IP 143.198.65.112)")
    fig.savefig(HERE / "host-owner148-slot96-slot110.png", dpi=180)


if __name__ == "__main__":
    main()
