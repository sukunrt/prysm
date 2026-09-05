#!/usr/bin/env python3
"""Join the matched full-gossip timed-GetAttestationData omission pair."""

import argparse
import json
from pathlib import Path


def load_jsonl(path):
    return [json.loads(line) for line in path.read_text().splitlines() if line]


def percentile(values, fraction):
    ordered = sorted(values)
    return ordered[int(fraction * (len(ordered) - 1))]


def milliseconds(value):
    return "" if value is None else value / 1e6


def arm_row(base, arm):
    directory = base / arm
    summary = json.loads((directory / "summary.json").read_text())
    clients = load_jsonl(directory / "client.jsonl")
    servers = {row["probe"]: row for row in load_jsonl(directory / "server.jsonl")}
    invoked = [row for row in clients if not row["skipped"]]
    durations = [row["duration_nano"] for row in invoked]
    slowest = max(invoked, key=lambda row: row["duration_nano"])
    invoke_admit = [
        (servers[row["probe"]]["admission_unix_nano"] - row["invoke_unix_nano"], row["probe"])
        for row in invoked
    ]
    handlers = [
        (servers[row["probe"]]["handler_duration_nano"], row["probe"])
        for row in invoked
    ]
    tails = [
        (row["return_unix_nano"] - servers[row["probe"]]["return_unix_nano"], row["probe"])
        for row in invoked
    ]
    attestation_client = summary["attestation_data_client"]
    attestation_server = summary.get("attestation_data_server")
    head_copy = summary.get("head_copy")
    stats = summary["final_stats"]
    cold_sync = summary["cold_sync"]
    return [
        arm,
        summary["timed_attestation_data"],
        summary["timed_attestation_data_omitted"],
        attestation_client["omitted"],
        summary["attestation_cache_cold"],
        summary["attestation_cache_populated"],
        summary["attestation_handler_admitted"],
        summary["attestation_handler_done"],
        summary["attestation_handler_pending"],
        summary["attestation_data_preflight"]["duration_nano"] / 1e6,
        summary["attestation_data_preflight"]["cache_cold"],
        summary["attestation_data_preflight"]["cache_populated"],
        cold_sync["duration_nano"] / 1e6,
        cold_sync["index_count"],
        cold_sync["stats_at_invoke"]["ActiveIterators"],
        cold_sync["stats_at_return"]["ActiveIterators"],
        milliseconds(None if attestation_client["omitted"] else attestation_client["duration_nano"]),
        milliseconds(None if attestation_server is None else attestation_server["admission_unix_nano"] - attestation_client["invoke_unix_nano"]),
        milliseconds(None if attestation_server is None else attestation_server["handler_duration_nano"]),
        milliseconds(None if head_copy is None else head_copy["duration_nano"]),
        milliseconds(None if attestation_server is None else attestation_client["return_unix_nano"] - attestation_server["return_unix_nano"]),
        summary["published"],
        stats["ValidationStarted"],
        stats["ValidationAccepted"],
        stats["MaximumActiveIterators"],
        (summary["offer_finished_unix_nano"] - summary["work_released_unix_nano"]) / 1e9,
        summary["maximum_publish_lateness_nano"] / 1e6,
        (summary["settled_unix_nano"] - summary["work_released_unix_nano"]) / 1e9,
        summary["slot_budget_at_work_release_nano"] / 1e9,
        len(clients),
        len(invoked),
        sum(row["status_code"] == "OK" for row in invoked),
        sum(row["skipped"] for row in clients),
        durations[0] / 1e6,
        percentile(durations, 0.5) / 1e6,
        percentile(durations, 0.95) / 1e6,
        slowest["duration_nano"] / 1e6,
        slowest["probe"],
        max(invoke_admit)[0] / 1e6,
        max(invoke_admit)[1],
        max(handlers)[0] / 1e6,
        max(handlers)[1],
        max(tails)[0] / 1e6,
        max(tails)[1],
    ]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("base", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    header = [
        "arm", "timed_attdata", "timed_attdata_omitted", "client_omitted",
        "measured_cache_cold", "measured_cache_populated", "handler_admitted", "handler_done",
        "handler_pending", "preflight_ms", "preflight_cache_cold", "preflight_cache_populated",
        "cold_sync_ms", "cold_sync_indices", "cold_sync_active_at_invoke", "cold_sync_active_at_return",
        "timed_attdata_client_ms", "timed_attdata_invoke_admit_ms", "timed_attdata_handler_bound_ms",
        "head_state_copy_ms", "timed_attdata_return_client_ms", "published", "validation_started",
        "accepted", "peak_iterators", "offer_elapsed_s", "max_publish_lateness_ms", "settle_seconds",
        "budget_at_release_s", "probe_grid", "domain_invoked", "domain_ok", "domain_skipped",
        "domain_first_ms", "domain_p50_ms", "domain_p95_ms", "domain_max_ms", "domain_max_probe",
        "invoke_admit_max_ms", "invoke_admit_max_probe", "handler_bound_max_ms", "handler_bound_max_probe",
        "return_client_max_ms", "return_client_max_probe",
    ]
    rows = [arm_row(args.base, arm) for arm in ("omitted", "enabled-reference")]
    args.output.write_text(
        "\t".join(header) + "\n" + "\n".join("\t".join(map(str, row)) for row in rows) + "\n"
    )


if __name__ == "__main__":
    main()
