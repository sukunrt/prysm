#!/usr/bin/env python3
"""Measure the first historical slot-1/round-0 FFG validation cohort.

Only the three named detailed observers are scanned. `arrivedMs` is converted
to validation-entry time relative to the attestation's slot; the outer log
timestamp is retained separately as the successful validation/log endpoint.
"""

from __future__ import annotations

import argparse
import csv
import json
import re
from collections import Counter
from datetime import datetime, timedelta, timezone
from pathlib import Path


GENESIS = datetime(2026, 9, 5, 1, 30, tzinfo=timezone.utc)
OWNERS = {1: 169, 2: 191, 3: 22, 4: 91, 5: 118, 6: 83, 7: 144, 8: 19,
          9: 107, 10: 35, 11: 117, 12: 14, 13: 20, 14: 85, 15: 32, 16: 76}
ANSI = re.compile(r"\x1b\[[0-9;]*m")
TIMESTAMP = re.compile(r"^(\S+) ")


def field(line: str, name: str) -> str | None:
    match = re.search(r"(?:^|\s)" + re.escape(name) + r"=(\S+)", line)
    return match.group(1) if match else None


def parse_timestamp(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def iso(value: datetime) -> str:
    return value.isoformat(timespec="microseconds").replace("+00:00", "Z")


def parse_duration(value: str) -> timedelta:
    units = {"ns": 1e-9, "us": 1e-6, "µs": 1e-6, "ms": 1e-3, "s": 1.0}
    match = re.fullmatch(r"([0-9]+(?:\.[0-9]+)?)(ns|us|µs|ms|s)", value)
    if not match:
        raise ValueError(f"unsupported duration {value!r}")
    return timedelta(seconds=float(match.group(1)) * units[match.group(2)])


def scan_observer(observer: int, path: Path) -> list[dict[str, object]]:
    rows: list[dict[str, object]] = []
    with path.open(errors="replace") as source:
        for line_number, raw in enumerate(source, 1):
            if "FFG vote" not in raw:
                continue
            line = ANSI.sub("", raw)
            if "sync: FFG vote " not in line or field(line, "outcome") != "gossip":
                continue
            if field(line, "attSlot") != "1" or field(line, "targetRound") != "0":
                continue
            arrived_ms = int(field(line, "arrivedMs") or "")
            match = TIMESTAMP.match(line)
            if not match:
                raise ValueError(f"observer {observer} line {line_number}: timestamp missing")
            accepted = parse_timestamp(match.group(1))
            entered = GENESIS + timedelta(seconds=12, milliseconds=arrived_ms)
            rows.append({
                "observer": observer,
                "line": line_number,
                "arrived_ms": arrived_ms,
                "entry": entered,
                "accepted": accepted,
                "entry_to_accepted_ms": (accepted - entered).total_seconds() * 1000,
                "validator": field(line, "validator") or "",
                "committee_index": field(line, "committeeIndex") or "",
                "source_anchor": f"{path}:{line_number}",
            })
    return rows


def write_onset(path: Path, observer_rows: dict[int, list[dict[str, object]]]) -> None:
    fields = ["observer", "accepted_slot1_round0_records", "first_entry_arrived_ms",
              "first_entry_timestamp", "first_entry_accepted_timestamp",
              "first_entry_lag_ms", "first_entry_validator", "first_entry_anchor",
              "first_accepted_timestamp", "first_accepted_arrived_ms",
              "first_accepted_lag_ms", "first_accepted_validator", "first_accepted_anchor",
              "entries_lt_500ms", "entries_lt_1000ms", "entries_lt_2000ms",
              "accepted_lt_500ms", "accepted_lt_1000ms", "accepted_lt_2000ms",
              "early_entry_tail_arrived_ms", "early_entry_tail_accepted_timestamp",
              "early_entry_tail_lag_ms", "early_entry_tail_anchor"]
    with path.open("w", newline="") as target:
        writer = csv.DictWriter(target, fieldnames=fields, delimiter="\t", lineterminator="\n")
        writer.writeheader()
        for observer, rows in observer_rows.items():
            if not rows:
                writer.writerow({"observer": observer, "accepted_slot1_round0_records": 0})
                continue
            first_entry = min(rows, key=lambda row: (row["entry"], row["accepted"], row["line"]))
            first_accepted = min(rows, key=lambda row: (row["accepted"], row["line"]))
            early_entry_tail = max(
                (row for row in rows if row["arrived_ms"] < 2000),
                key=lambda row: (row["accepted"], row["line"]),
            )
            slot_start = GENESIS + timedelta(seconds=12)
            writer.writerow({
                "observer": observer,
                "accepted_slot1_round0_records": len(rows),
                "first_entry_arrived_ms": first_entry["arrived_ms"],
                "first_entry_timestamp": iso(first_entry["entry"]),
                "first_entry_accepted_timestamp": iso(first_entry["accepted"]),
                "first_entry_lag_ms": f"{first_entry['entry_to_accepted_ms']:.3f}",
                "first_entry_validator": first_entry["validator"],
                "first_entry_anchor": first_entry["source_anchor"],
                "first_accepted_timestamp": iso(first_accepted["accepted"]),
                "first_accepted_arrived_ms": first_accepted["arrived_ms"],
                "first_accepted_lag_ms": f"{first_accepted['entry_to_accepted_ms']:.3f}",
                "first_accepted_validator": first_accepted["validator"],
                "first_accepted_anchor": first_accepted["source_anchor"],
                "entries_lt_500ms": sum(row["arrived_ms"] < 500 for row in rows),
                "entries_lt_1000ms": sum(row["arrived_ms"] < 1000 for row in rows),
                "entries_lt_2000ms": sum(row["arrived_ms"] < 2000 for row in rows),
                "accepted_lt_500ms": sum(row["accepted"] < slot_start + timedelta(milliseconds=500) for row in rows),
                "accepted_lt_1000ms": sum(row["accepted"] < slot_start + timedelta(seconds=1) for row in rows),
                "accepted_lt_2000ms": sum(row["accepted"] < slot_start + timedelta(seconds=2) for row in rows),
                "early_entry_tail_arrived_ms": early_entry_tail["arrived_ms"],
                "early_entry_tail_accepted_timestamp": iso(early_entry_tail["accepted"]),
                "early_entry_tail_lag_ms": f"{early_entry_tail['entry_to_accepted_ms']:.3f}",
                "early_entry_tail_anchor": early_entry_tail["source_anchor"],
            })


def write_bins(path: Path, observer_rows: dict[int, list[dict[str, object]]]) -> None:
    fields = ["observer", "genesis_offset_bin_start_s", "genesis_offset_bin_end_s",
              "eventual_accepted_cohort_entries", "accepted_log_endpoints",
              "cumulative_entries", "cumulative_accepted_log_endpoints"]
    with path.open("w", newline="") as target:
        writer = csv.DictWriter(target, fieldnames=fields, delimiter="\t", lineterminator="\n")
        writer.writeheader()
        for observer, rows in observer_rows.items():
            entries = Counter(int((row["entry"] - GENESIS).total_seconds()) for row in rows)
            accepted = Counter(int((row["accepted"] - GENESIS).total_seconds()) for row in rows)
            cumulative_entries = 0
            cumulative_accepted = 0
            for second in range(10, 26):
                cumulative_entries += entries[second]
                cumulative_accepted += accepted[second]
                writer.writerow({
                    "observer": observer,
                    "genesis_offset_bin_start_s": second,
                    "genesis_offset_bin_end_s": second + 1,
                    "eventual_accepted_cohort_entries": entries[second],
                    "accepted_log_endpoints": accepted[second],
                    "cumulative_entries": cumulative_entries,
                    "cumulative_accepted_log_endpoints": cumulative_accepted,
                })


def write_owner_captures(path: Path, progress_path: Path) -> None:
    captured: dict[int, dict[str, str]] = {}
    with progress_path.open(newline="") as source:
        for row in csv.DictReader(source, delimiter="\t"):
            if row["message_type"] != "Submitted new attestations" or row["activity_slot"] != "1":
                continue
            node = int(row["node"])
            if node not in OWNERS.values():
                continue
            fields = json.loads(row["fields_json"])
            offset = parse_duration(row["submitted_since_slot_start"])
            capture = GENESIS + timedelta(seconds=12) + offset
            item = row | {
                "capture": iso(capture),
                "capture_genesis_offset_ms": f"{(capture - GENESIS).total_seconds() * 1000:.3f}",
                "target_round": str(fields.get("targetRound", "")),
            }
            if node in captured:
                raise ValueError(f"multiple slot-1 summary captures for owner node {node}")
            captured[node] = item

    fields = ["owned_slot", "node", "capture_status", "summary_activity_slot",
              "first_success_capture_timestamp", "first_success_capture_genesis_offset_ms",
              "submitted_since_slot_start", "submission_spread", "captured_key_count",
              "target_round", "summary_log_timestamp", "source_anchor"]
    with path.open("w", newline="") as target:
        writer = csv.DictWriter(target, fieldnames=fields, delimiter="\t", lineterminator="\n")
        writer.writeheader()
        for owned_slot, node in OWNERS.items():
            row = captured.get(node)
            writer.writerow({
                "owned_slot": owned_slot,
                "node": node,
                "capture_status": "summary_group_capture" if row else "not_present",
                "summary_activity_slot": row["activity_slot"] if row else "",
                "first_success_capture_timestamp": row["capture"] if row else "",
                "first_success_capture_genesis_offset_ms": row["capture_genesis_offset_ms"] if row else "",
                "submitted_since_slot_start": row["submitted_since_slot_start"] if row else "",
                "submission_spread": row["submission_spread"] if row else "",
                "captured_key_count": row["key_count"] if row else "",
                "target_round": row["target_round"] if row else "",
                "summary_log_timestamp": row["timestamp"] if row else "",
                "source_anchor": row["source_anchor"] if row else "",
            })


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parents[3])
    parser.add_argument("--output-directory", type=Path, default=Path(__file__).resolve().parent)
    args = parser.parse_args()
    observer_rows = {
        observer: scan_observer(observer, args.repo / f"runs/round2/prysm-geth-{observer}/beacon.log")
        for observer in (201, 400, 500)
    }
    write_onset(args.output_directory / "ffg-count-onset-observers.tsv", observer_rows)
    write_bins(args.output_directory / "ffg-count-onset-bins.tsv", observer_rows)
    write_owner_captures(
        args.output_directory / "slot1-owner-attestation-captures.tsv",
        args.output_directory / "validator_role_progress.tsv",
    )


if __name__ == "__main__":
    main()
