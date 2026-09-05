#!/usr/bin/env python3
"""Account for round2 node169's slot-1 duties from its validator log."""

import argparse
import datetime
import json
import re


ANSI_RE = re.compile(r"\x1b\[[0-9;]*[mK]")
TIME_RE = re.compile(r"^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d+Z)")
PUBKEY_RE = re.compile(r"pubkey=(0x[0-9a-f]+)")


def timestamp(line):
    match = TIME_RE.match(line)
    if not match:
        return None
    return datetime.datetime.fromisoformat(match.group(1).replace("Z", "+00:00"))


def bracketed_keys(line, field):
    match = re.search(rf"{field}=\[([^]]*)\]", line)
    return set(match.group(1).split()) if match else set()


def audit(path):
    with open(path, encoding="utf-8") as source:
        lines = [ANSI_RE.sub("", line) for line in source]

    randao_index = next(
        i for i, line in enumerate(lines)
        if "Failed to sign randao reveal" in line and "0xa7120c370e8c" in line
    )
    schedules = [
        (i, line) for i, line in enumerate(lines[:randao_index])
        if "Duties schedule" in line and "slot=1 " in line
    ]
    schedule_index, schedule = schedules[-1]
    start = timestamp(schedule)
    assert start is not None

    first_slot2_error = next(
        timestamp(line) for line in lines[randao_index + 1:]
        if "slot=2" in line and "DeadlineExceeded" in line
    )
    assert first_slot2_error is not None
    # Some role errors omit their slot field and precede the first explicit
    # slot-2 error by milliseconds. The deadline second is the clean boundary.
    slot2_error = first_slot2_error.replace(microsecond=0)
    window = [
        line for line in lines[schedule_index:]
        if timestamp(line) is not None and start <= timestamp(line) < slot2_error
    ]

    scheduled_attesters = bracketed_keys(schedule, "attesterPubkeys")
    scheduled_ptc = bracketed_keys(schedule, "ptcPubkeys")
    proposer = re.search(r"proposerPubkey=(0x[0-9a-f]+)", schedule).group(1)
    failed_attesters = {
        PUBKEY_RE.search(line).group(1) for line in window
        if "Could not request attestation to sign at slot" in line
        and "slot=1" in line and PUBKEY_RE.search(line)
    }

    count = lambda text: sum(text in line for line in window)
    return {
        "schedule_line": schedule_index + 1,
        "proposer_pubkey": proposer,
        "proposer_is_attester": proposer in scheduled_attesters,
        "attesters": {
            "scheduled": len(scheduled_attesters),
            "failed_prerequisite_rpc": len(failed_attesters),
            "missing_failed_pubkeys": sorted(scheduled_attesters - failed_attesters),
            "unexpected_failed_pubkeys": sorted(failed_attesters - scheduled_attesters),
        },
        "ptc": {
            "scheduled": len(scheduled_ptc),
            "not_found_before_domain": count("Skipping payload attestation: no block for slot slot=1"),
            "deadline_before_domain": count("Could not request payload attestation data"),
        },
        "sync_message_root_deadlines_before_domain": count("Could not request sync message block root to sign"),
        "sync_aggregator_index_deadlines_before_domain": count("Could not get sync subcommittee index"),
        "aggregators": {
            "attestation_data_deadlines": count("Could not get signed attestation data for aggregation"),
            "aggregate_rpc_deadlines": count("Could not submit aggregate selection proof to beacon node"),
        },
        "randao_deadlines": count("Failed to sign randao reveal"),
        "window_start": start.isoformat(),
        "window_end_exclusive": slot2_error.isoformat(),
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("validator_log")
    args = parser.parse_args()
    print(json.dumps(audit(args.validator_log), indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
