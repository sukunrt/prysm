#!/usr/bin/env python3
"""Census node 1's slot-97 FFG vote validation-entry and emission offsets."""

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
BASE = datetime(2026, 9, 5, 1, 49, 24, tzinfo=timezone.utc)
THRESHOLDS_MS = (25, 50, 100, 200, 500, 1_000, 3_645)
REQUIRED_FIELDS = (
    "arrivedMs",
    "dataRoot",
    "committeeIndex",
    "validator",
)


def parse_outer_timestamp(text: str) -> datetime:
    token = text.split(" ", 1)[0]
    return datetime.fromisoformat(token.replace("Z", "+00:00"))


def cumulative(rows: list[dict], key: str) -> dict[str, dict[str, int]]:
    result = {}
    for threshold in THRESHOLDS_MS:
        selected = [row for row in rows if row[key] <= threshold]
        tuples = {
            (row["validator"], row["dataRoot"], row["committeeIndex"])
            for row in selected
        }
        result[f"le_{threshold}ms"] = {
            "row_count": len(selected),
            "unique_validators": len({row["validator"] for row in selected}),
            "unique_dataRoots": len({row["dataRoot"] for row in selected}),
            "unique_validator_dataRoot_committee": len(tuples),
        }
    return result


def timing_summary(rows: list[dict]) -> dict:
    return {
        "row_count": len(rows),
        "unique_validators": len({row["validator"] for row in rows}),
        "unique_dataRoots": len({row["dataRoot"] for row in rows}),
        "unique_committees": len({row["committeeIndex"] for row in rows}),
        "arrivedMs_min": min((row["arrivedMs"] for row in rows), default=None),
        "arrivedMs_max": max((row["arrivedMs"] for row in rows), default=None),
        "emitted_offset_ms_min": min(
            (row["emitted_offset_ms"] for row in rows), default=None
        ),
        "emitted_offset_ms_max": max(
            (row["emitted_offset_ms"] for row in rows), default=None
        ),
        "arrivedMs_cumulative": cumulative(rows, "arrivedMs"),
        "outer_emitted_offset_cumulative": cumulative(rows, "emitted_offset_ms"),
        "first_by_arrivedMs": sorted(
            rows, key=lambda row: (row["arrivedMs"], row["line"])
        )[:20],
        "first_by_outer_emission": sorted(
            rows, key=lambda row: (row["emitted_offset_ms"], row["line"])
        )[:20],
    }


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
            "node1-slot97-ffg-timing.json"
        ),
    )
    parser.add_argument(
        "--schedule-output",
        type=Path,
        help="optional full outcome=gossip row schedule for concurrency replay",
    )
    args = parser.parse_args()

    rows: list[dict] = []
    digest = hashlib.sha256()
    physical_lines = 0
    candidate_vote_lines = 0
    wrong_slot_after_parse = 0
    malformed_counts: Counter[str] = Counter()

    with args.input.open("rb") as source:
        for physical_lines, raw in enumerate(source, 1):
            digest.update(raw)
            if b"attSlot" not in raw or b"=97" not in raw or b"FFG vote " not in raw:
                continue
            text = ANSI_RE.sub("", raw.decode("utf-8", errors="replace")).rstrip(
                "\r\n"
            )
            if " FFG vote " not in text or " FFG vote included " in text:
                continue
            fields = dict(FIELD_RE.findall(text))
            if fields.get("attSlot") != "97":
                continue
            candidate_vote_lines += 1
            missing = [name for name in REQUIRED_FIELDS if name not in fields]
            if missing:
                for name in missing:
                    malformed_counts[f"missing_{name}"] += 1
                continue
            try:
                outer = parse_outer_timestamp(text)
            except (ValueError, IndexError):
                malformed_counts["invalid_outer_timestamp"] += 1
                continue
            try:
                arrived_ms = int(fields["arrivedMs"])
                validator = int(fields["validator"])
                committee = int(fields["committeeIndex"])
            except ValueError:
                malformed_counts["invalid_numeric_field"] += 1
                continue
            emitted_ms = (outer - BASE).total_seconds() * 1000
            rows.append(
                {
                    "line": physical_lines,
                    "outer_timestamp": outer.isoformat().replace("+00:00", "Z"),
                    "emitted_offset_ms": round(emitted_ms, 3),
                    "arrivedMs": arrived_ms,
                    "validator": validator,
                    "dataRoot": fields["dataRoot"],
                    "blockRoot": fields.get("blockRoot"),
                    "committeeIndex": committee,
                    "outcome": fields.get("outcome"),
                }
            )

    tuples = {
        (row["validator"], row["dataRoot"], row["committeeIndex"])
        for row in rows
    }
    groups: dict[tuple[str, int], list[dict]] = defaultdict(list)
    for row in rows:
        groups[(row["dataRoot"], row["committeeIndex"])].append(row)
    gossip_rows = [row for row in rows if row["outcome"] == "gossip"]
    local_rows = [row for row in rows if row["outcome"] == "local"]

    result = {
        "scope": {
            "input": str(args.input),
            "input_size_bytes": args.input.stat().st_size,
            "input_sha256": digest.hexdigest(),
            "physical_lf_records_scanned": physical_lines,
            "attSlot": 97,
            "slot_start_utc": "2026-09-05T01:49:24Z",
            "threshold_semantics": "cumulative and inclusive (offset <= threshold)",
            "limitations": [
                "arrivedMs is validation entry for outcome=gossip",
                "arrivedMs is local submission/log time for outcome=local, not gossip validation entry",
                "outer_timestamp is the log emission prefix",
                "ledger emission does not establish completed subscriber insertion or proposal-pool membership",
                "this is node 1 only; unique-validator counts are observer-local",
            ],
        },
        "parse_accounting": {
            "candidate_vote_lines": candidate_vote_lines,
            "parsed_rows": len(rows),
            "wrong_slot_after_parse": wrong_slot_after_parse,
            "malformed_or_missing": dict(sorted(malformed_counts.items())),
            "unparsed_candidate_lines": candidate_vote_lines
            - wrong_slot_after_parse
            - len(rows),
        },
        "gossip_validation_entries": timing_summary(gossip_rows),
        "local_submission_ledger_rows": timing_summary(local_rows),
        "single_votes": {
            "row_count": len(rows),
            "unique_validators": len({row["validator"] for row in rows}),
            "unique_dataRoots": len({row["dataRoot"] for row in rows}),
            "unique_committees": len({row["committeeIndex"] for row in rows}),
            "unique_validator_dataRoot_committee": len(tuples),
            "duplicate_tuple_rows": len(rows) - len(tuples),
            "outcomes": dict(sorted(Counter(row["outcome"] for row in rows).items())),
            "arrivedMs_min": min((row["arrivedMs"] for row in rows), default=None),
            "arrivedMs_max": max((row["arrivedMs"] for row in rows), default=None),
            "emitted_offset_ms_min": min(
                (row["emitted_offset_ms"] for row in rows), default=None
            ),
            "emitted_offset_ms_max": max(
                (row["emitted_offset_ms"] for row in rows), default=None
            ),
            "arrivedMs_cumulative": cumulative(rows, "arrivedMs"),
            "outer_emitted_offset_cumulative": cumulative(
                rows, "emitted_offset_ms"
            ),
            "first_by_arrivedMs": sorted(
                rows, key=lambda row: (row["arrivedMs"], row["line"])
            )[:20],
            "first_by_outer_emission": sorted(
                rows, key=lambda row: (row["emitted_offset_ms"], row["line"])
            )[:20],
            "dataRoot_committee_groups": [
                {
                    "dataRoot": data_root,
                    "committeeIndex": committee,
                    "row_count": len(group_rows),
                    "unique_validators": len(
                        {row["validator"] for row in group_rows}
                    ),
                    "arrivedMs_min": min(row["arrivedMs"] for row in group_rows),
                    "arrivedMs_max": max(row["arrivedMs"] for row in group_rows),
                    "emitted_offset_ms_min": min(
                        row["emitted_offset_ms"] for row in group_rows
                    ),
                    "emitted_offset_ms_max": max(
                        row["emitted_offset_ms"] for row in group_rows
                    ),
                }
                for (data_root, committee), group_rows in sorted(groups.items())
            ],
        },
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    if args.schedule_output is not None:
        schedule = {
            "source": str(args.input),
            "source_sha256": digest.hexdigest(),
            "attSlot": 97,
            "slot_start_utc": "2026-09-05T01:49:24Z",
            "rows": [
                {
                    "arrivedMs": row["arrivedMs"],
                    "blockRoot": row["blockRoot"],
                    "committeeIndex": row["committeeIndex"],
                    "dataRoot": row["dataRoot"],
                    "line": row["line"],
                    "validator": row["validator"],
                }
                for row in sorted(
                    gossip_rows, key=lambda row: (row["arrivedMs"], row["line"])
                )
            ],
        }
        args.schedule_output.parent.mkdir(parents=True, exist_ok=True)
        args.schedule_output.write_text(
            json.dumps(schedule, separators=(",", ":")) + "\n"
        )


if __name__ == "__main__":
    main()
