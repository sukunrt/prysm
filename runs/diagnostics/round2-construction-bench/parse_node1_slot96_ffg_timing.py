#!/usr/bin/env python3
"""Census node 1's slot-96 FFG vote and aggregate ledger emissions."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path


ANSI_RE = re.compile(r"\x1b\[[0-9;]*m")
FIELD_RE = re.compile(r"(?:^|\s)([A-Za-z][A-Za-z0-9]*)=([^\s]+)")
BASE = datetime(2026, 9, 5, 1, 49, 12, tzinfo=timezone.utc)
BUCKETS = (
    ("le_7s", None, 7_000),
    ("gt_7s_le_9_5s", 7_000, 9_500),
    ("gt_9_5s_le_11s", 9_500, 11_000),
    ("gt_11s_le_11_8s", 11_000, 11_800),
    ("gt_11_8s_le_12s", 11_800, 12_000),
    ("after_12s", 12_000, None),
)


def bucket_name(value_ms: float) -> str:
    for name, lower_exclusive, upper_inclusive in BUCKETS:
        if lower_exclusive is not None and value_ms <= lower_exclusive:
            continue
        if upper_inclusive is not None and value_ms > upper_inclusive:
            continue
        return name
    raise AssertionError(value_ms)


def parse_outer_timestamp(text: str) -> datetime:
    token = text.split(" ", 1)[0]
    return datetime.fromisoformat(token.replace("Z", "+00:00"))


def summarize_buckets(rows: list[dict], value_key: str) -> dict:
    grouped: dict[str, list[dict]] = defaultdict(list)
    for row in rows:
        grouped[bucket_name(row[value_key])].append(row)
    result = {}
    for name, _, _ in BUCKETS:
        selected = grouped[name]
        tuples = {
            (row["validator"], row["dataRoot"], row["committeeIndex"])
            for row in selected
        }
        result[name] = {
            "row_count": len(selected),
            "unique_validators": len({row["validator"] for row in selected}),
            "unique_validator_dataRoot_committee": len(tuples),
        }
    return result


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--input",
        type=Path,
        default=Path("runs/round2/prysm-geth-1/beacon.log"),
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=Path(
            "runs/diagnostics/round2-construction-bench/"
            "node1-slot96-ffg-timing.json"
        ),
    )
    args = parser.parse_args()

    singles: list[dict] = []
    aggregates: list[dict] = []
    digest = hashlib.sha256()
    physical_lines = 0
    with args.input.open("rb") as source:
        for physical_lines, raw in enumerate(source, 1):
            digest.update(raw)
            if b"attSlot" not in raw or b"=96" not in raw or b"FFG" not in raw:
                continue
            text = ANSI_RE.sub("", raw.decode("utf-8", errors="replace")).rstrip("\r\n")
            is_vote = " FFG vote " in text and " FFG vote included " not in text
            is_aggregate = (
                " FFG aggregate " in text and " FFG aggregate groups " not in text
            )
            if not is_vote and not is_aggregate:
                continue
            fields = dict(FIELD_RE.findall(text))
            if fields.get("attSlot") != "96":
                continue
            try:
                outer = parse_outer_timestamp(text)
            except (ValueError, IndexError):
                continue
            emitted_ms = (outer - BASE).total_seconds() * 1000
            common = {
                "line": physical_lines,
                "outer_timestamp": outer.isoformat().replace("+00:00", "Z"),
                "emitted_offset_ms": round(emitted_ms, 3),
                "arrivedMs": int(fields["arrivedMs"]),
                "dataRoot": fields["dataRoot"],
                "committeeIndex": int(fields["committeeIndex"]),
                "outcome": fields.get("outcome"),
            }
            if is_vote:
                common["validator"] = int(fields["validator"])
                singles.append(common)
            elif is_aggregate:
                common.update(
                    {
                        "aggregatorIndex": int(fields["aggregatorIndex"]),
                        "seats": int(fields["seats"]),
                    }
                )
                aggregates.append(common)

    single_tuples = {
        (row["validator"], row["dataRoot"], row["committeeIndex"])
        for row in singles
    }
    single_groups: dict[tuple[str, int], list[dict]] = defaultdict(list)
    for row in singles:
        single_groups[(row["dataRoot"], row["committeeIndex"])].append(row)
    aggregate_groups: dict[tuple[str, int], list[dict]] = defaultdict(list)
    for row in aggregates:
        aggregate_groups[(row["dataRoot"], row["committeeIndex"])].append(row)
    aggregate_summary = []
    for (data_root, committee), rows in sorted(aggregate_groups.items()):
        rows.sort(key=lambda row: (row["emitted_offset_ms"], row["line"]))
        aggregate_summary.append(
            {
                "dataRoot": data_root,
                "committeeIndex": committee,
                "emission_row_count": len(rows),
                "unique_aggregators": len({row["aggregatorIndex"] for row in rows}),
                "outcomes": dict(sorted(Counter(row["outcome"] for row in rows).items())),
                "seats_min": min(row["seats"] for row in rows),
                "seats_max": max(row["seats"] for row in rows),
                "emissions": rows,
            }
        )

    result = {
        "scope": {
            "input": str(args.input),
            "input_size_bytes": args.input.stat().st_size,
            "input_sha256": digest.hexdigest(),
            "physical_lf_records_scanned": physical_lines,
            "attSlot": 96,
            "slot_start_utc": "2026-09-05T01:49:12Z",
            "bucket_semantics": "lower bound exclusive; upper bound inclusive",
            "limitations": [
                "arrivedMs is copied from the ledger field; outer_timestamp is the log emission prefix",
                "ledger emission is not proof of subscriber insertion time or proposal-pool snapshot membership",
                "aggregate grouping uses logged dataRoot and committeeIndex only",
            ],
        },
        "single_votes": {
            "row_count": len(singles),
            "unique_validators": len({row["validator"] for row in singles}),
            "unique_dataRoots": len({row["dataRoot"] for row in singles}),
            "unique_committees": len({row["committeeIndex"] for row in singles}),
            "unique_validator_dataRoot_committee": len(single_tuples),
            "duplicate_tuple_rows": len(singles) - len(single_tuples),
            "arrivedMs_buckets": summarize_buckets(singles, "arrivedMs"),
            "outer_emitted_offset_buckets": summarize_buckets(
                singles, "emitted_offset_ms"
            ),
            "arrivedMs_min": min((row["arrivedMs"] for row in singles), default=None),
            "arrivedMs_max": max((row["arrivedMs"] for row in singles), default=None),
            "emitted_offset_ms_min": min(
                (row["emitted_offset_ms"] for row in singles), default=None
            ),
            "emitted_offset_ms_max": max(
                (row["emitted_offset_ms"] for row in singles), default=None
            ),
            "dataRoot_committee_groups": [
                {
                    "dataRoot": data_root,
                    "committeeIndex": committee,
                    "row_count": len(rows),
                    "unique_validators": len({row["validator"] for row in rows}),
                    "arrivedMs_min": min(row["arrivedMs"] for row in rows),
                    "arrivedMs_max": max(row["arrivedMs"] for row in rows),
                    "emitted_offset_ms_min": min(
                        row["emitted_offset_ms"] for row in rows
                    ),
                    "emitted_offset_ms_max": max(
                        row["emitted_offset_ms"] for row in rows
                    ),
                }
                for (data_root, committee), rows in sorted(single_groups.items())
            ],
        },
        "aggregate_emissions": {
            "row_count": len(aggregates),
            "unique_dataRoots": len({row["dataRoot"] for row in aggregates}),
            "unique_committees": len(
                {row["committeeIndex"] for row in aggregates}
            ),
            "unique_dataRoot_committee_groups": len(aggregate_groups),
            "emitted_offset_ms_min": min(
                (row["emitted_offset_ms"] for row in aggregates), default=None
            ),
            "emitted_offset_ms_max": max(
                (row["emitted_offset_ms"] for row in aggregates), default=None
            ),
            "groups": aggregate_summary,
        },
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")


if __name__ == "__main__":
    main()
