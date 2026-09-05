#!/usr/bin/env python3
"""Join the bounded full-gossip natural-writer diagnostic arms."""

import argparse
import json
from pathlib import Path


def load_jsonl(path):
    return [json.loads(line) for line in path.read_text().splitlines() if line]


def percentile(values, fraction):
    ordered = sorted(values)
    return ordered[int(fraction * (len(ordered) - 1))]


def arm_row(base, arm):
    directory = base / arm
    summary = json.loads((directory / "summary.json").read_text())
    clients = load_jsonl(directory / "client.jsonl")
    invoked = [row for row in clients if not row["skipped"]]
    servers = {row["probe"]: row for row in load_jsonl(directory / "server.jsonl")}
    durations = [row["duration_nano"] / 1e6 for row in invoked]
    invoke_admit = [
        ((servers[row["probe"]]["admission_unix_nano"] - row["invoke_unix_nano"]) / 1e6, row["probe"])
        for row in invoked if row["probe"] in servers
    ]
    handlers = [
        (servers[row["probe"]]["handler_duration_nano"] / 1e6, row["probe"])
        for row in invoked if row["probe"] in servers
    ]
    tails = [
        ((row["return_unix_nano"] - servers[row["probe"]]["return_unix_nano"]) / 1e6, row["probe"])
        for row in invoked if row["probe"] in servers
    ]
    slowest = max((row["duration_nano"] / 1e6, row["probe"]) for row in invoked)
    att_client = summary["attestation_data_client"]
    att_server = summary["attestation_data_server"]
    stats = summary["final_stats"]
    return [
        arm,
        summary["published"],
        stats["ValidationStarted"],
        stats["ValidationAccepted"],
        stats["SubscriberCompleted"],
        stats["UnaggregatedPoolEntries"],
        stats["MaximumActiveIterators"],
        (summary["offer_finished_unix_nano"] - summary["work_released_unix_nano"]) / 1e9,
        summary["maximum_publish_lateness_nano"] / 1e6,
        (summary["settled_unix_nano"] - summary["work_released_unix_nano"]) / 1e9,
        summary["cold_sync"]["duration_nano"] / 1e6,
        att_client["duration_nano"] / 1e6,
        (att_server["admission_unix_nano"] - att_client["invoke_unix_nano"]) / 1e6,
        att_server["handler_duration_nano"] / 1e6,
        summary["head_copy"]["duration_nano"] / 1e6,
        (att_client["return_unix_nano"] - att_server["return_unix_nano"]) / 1e6,
        len(clients),
        len(invoked),
        sum(row["status_code"] == "OK" for row in invoked),
        sum(row["skipped"] for row in clients),
        durations[0],
        percentile(durations, 0.5),
        percentile(durations, 0.95),
        slowest[0],
        slowest[1],
        max(invoke_admit)[0],
        max(invoke_admit)[1],
        max(handlers)[0],
        max(handlers)[1],
        max(tails)[0],
        max(tails)[1],
    ]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("base", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    header = [
        "arm", "published", "validation_started", "accepted", "subscriber_completed", "pool_entries",
        "peak_iterators", "offer_elapsed_s", "max_publish_lateness_ms", "settle_seconds", "cold_sync_ms",
        "att_data_client_ms", "att_data_invoke_admit_ms", "att_data_handler_bound_ms", "head_state_copy_ms",
        "att_data_return_client_ms", "probe_grid", "domain_invoked", "domain_ok", "domain_skipped",
        "domain_first_ms", "domain_p50_ms", "domain_p95_ms", "domain_max_ms", "domain_max_probe",
        "invoke_admit_max_ms", "invoke_admit_max_probe", "handler_bound_max_ms", "handler_bound_max_probe",
        "return_client_max_ms", "return_client_max_probe",
    ]
    rows = [arm_row(args.base, arm) for arm in ("shared", "snapshot")]
    args.output.write_text(
        "\t".join(header) + "\n" + "\n".join("\t".join(map(str, row)) for row in rows) + "\n"
    )


if __name__ == "__main__":
    main()
