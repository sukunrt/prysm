#!/usr/bin/env python3
"""Join the two bounded full-gossip DomainData diagnostic arms."""

import argparse
import json
from pathlib import Path


def percentile(values, fraction):
    ordered = sorted(values)
    return ordered[int(fraction * (len(ordered) - 1))]


def load_jsonl(path):
    return [json.loads(line) for line in path.read_text().splitlines() if line]


def arm_row(base, arm):
    directory = base / arm
    summary = json.loads((directory / "summary.json").read_text())
    clients = [row for row in load_jsonl(directory / "client.jsonl") if not row["skipped"]]
    servers = {row["probe"]: row for row in load_jsonl(directory / "server.jsonl")}
    durations = [row["duration_nano"] / 1e6 for row in clients]
    invoke_admit = [
        (servers[row["probe"]]["admission_unix_nano"] - row["invoke_unix_nano"]) / 1e6
        for row in clients
    ]
    handlers = [servers[row["probe"]]["handler_duration_nano"] / 1e6 for row in clients]
    return_to_client = [
        (row["return_unix_nano"] - servers[row["probe"]]["return_unix_nano"]) / 1e6
        for row in clients
    ]
    stats = summary["final_stats"]
    return [
        arm,
        summary["published"],
        stats["ValidationStarted"],
        stats["ValidationAccepted"],
        stats["SubscriberCompleted"],
        stats["UnaggregatedPoolEntries"],
        stats["MaximumActiveIterators"],
        (summary["settled_unix_nano"] - summary["work_released_unix_nano"]) / 1e9,
        summary["cold_sync"]["duration_nano"] / 1e6,
        len(clients),
        sum(row["status_code"] == "OK" for row in clients),
        32 - len(clients),
        durations[0],
        percentile(durations, 0.5),
        percentile(durations, 0.95),
        max(durations),
        max(invoke_admit),
        max(handlers),
        max(return_to_client),
    ]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("base", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    header = [
        "arm", "published", "validation_started", "accepted", "subscriber_completed",
        "pool_entries", "peak_iterators", "settle_seconds", "cold_sync_ms", "domain_calls",
        "domain_ok", "domain_skipped", "domain_first_ms", "domain_p50_ms", "domain_p95_ms",
        "domain_max_ms", "invoke_to_admit_max_ms", "admission_to_handler_return_bound_ms", "return_to_client_max_ms",
    ]
    rows = [arm_row(args.base, arm) for arm in ("shared", "snapshot")]
    args.output.write_text("\t".join(header) + "\n" + "\n".join("\t".join(map(str, row)) for row in rows) + "\n")


if __name__ == "__main__":
    main()
