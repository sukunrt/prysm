#!/usr/bin/env python3
"""Classify node-1 log records from one second before build 97 through finish."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from collections import Counter, defaultdict
from datetime import datetime
from pathlib import Path


ANSI_RE = re.compile(r"\x1b\[[0-9;]*m")
HEAD_RE = re.compile(
    r"^(\S+)\s+\[[^]]+\]\s+(TRACE|DEBUG|INFO|WARN|ERROR|FATAL)\s+([^:]+):\s*(.*)$"
)
FIELD_RE = re.compile(r"(?:^|\s)([A-Za-z][A-Za-z0-9]*)=([^\s]+)")
SCAN_FIRST_LINE = 1_120_000
BUILD_LINE = 1_121_950
FINISH_LINE = 1_136_845
WINDOW_START = "2026-09-05T01:49:23.008736237Z"
BUILD_TIME = "2026-09-05T01:49:24.008736237Z"
FINISH_TIME = "2026-09-05T01:49:27.662622713Z"


def offset_ms(value: str) -> float:
    parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    build = datetime.fromisoformat(BUILD_TIME.replace("Z", "+00:00"))
    return round((parsed - build).total_seconds() * 1000, 3)


def classify(message: str) -> str:
    if message == "FFG vote":
        return "ffg_vote"
    if message == "Goldfish vote":
        return "head_vote_ledger"
    if message == "FFG aggregate":
        return "ffg_aggregate"
    lower = message.lower()
    if message == "PTC vote":
        return "ptc_vote"
    if "sync committee" in lower or "sync contribution" in lower:
        return "sync_committee"
    if message in {"Building block", "Chose payload bid", "Finished building block"}:
        return "block_construction"
    return "other"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--input", type=Path, default=Path("runs/round2/prysm-geth-1/beacon.log")
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=Path(
            "runs/diagnostics/round2-construction-bench/"
            "node1-build97-window-message-census.json"
        ),
    )
    args = parser.parse_args()

    accounting = Counter()
    category_counts = Counter()
    category_outcomes: dict[str, Counter] = defaultdict(Counter)
    category_components: dict[str, Counter] = defaultdict(Counter)
    category_validators: dict[str, set[str]] = defaultdict(set)
    message_counts = Counter()
    records: dict[tuple[str, str | None], dict] = {}
    anchors: list[dict] = []
    previous_record = None
    selected_digest = hashlib.sha256()

    with args.input.open("rb") as source:
        for line, raw in enumerate(source, 1):
            if line < SCAN_FIRST_LINE:
                continue
            if line > FINISH_LINE:
                break
            accounting["physical_lf_records_scanned"] += 1
            text = ANSI_RE.sub("", raw.decode("utf-8", errors="replace")).rstrip("\r\n")
            match = HEAD_RE.match(text)
            if match is None:
                accounting["header_parse_failures"] += 1
                continue
            outer, level, component, payload = match.groups()
            if outer < WINDOW_START:
                previous_record = {"line": line, "outer_timestamp": outer, "raw_prefix": text[:500]}
                accounting["records_before_window"] += 1
                continue
            if outer > FINISH_TIME:
                accounting["records_after_finish_within_line_bound"] += 1
                continue

            selected_digest.update(raw)
            accounting["records_in_window"] += 1
            phase = "preceding_one_second" if outer < BUILD_TIME else "build_through_finish"
            accounting[f"records_{phase}"] += 1
            first_field = FIELD_RE.search(payload)
            message = (payload[: first_field.start()] if first_field else payload).strip()
            fields = dict(FIELD_RE.findall(payload))
            category = classify(message)
            outcome = fields.get("outcome")
            category_counts[category] += 1
            category_outcomes[category][outcome or "(none)"] += 1
            category_components[category][component] += 1
            message_counts[message] += 1
            validator = fields.get("validator", fields.get("validatorIndex"))
            if validator is not None:
                category_validators[category].add(validator)

            key = (message, outcome)
            summary = records.setdefault(
                key,
                {
                    "message": message,
                    "outcome": outcome,
                    "count": 0,
                    "first_line": line,
                    "first_outer_timestamp": outer,
                    "last_line": line,
                    "last_outer_timestamp": outer,
                    "arrivedMs_min": None,
                    "arrivedMs_max": None,
                    "decidedMs_min": None,
                    "decidedMs_max": None,
                },
            )
            summary["count"] += 1
            summary["last_line"] = line
            summary["last_outer_timestamp"] = outer
            for field in ("arrivedMs", "decidedMs"):
                if field not in fields:
                    continue
                value = int(fields[field])
                minimum = f"{field}_min"
                maximum = f"{field}_max"
                summary[minimum] = value if summary[minimum] is None else min(summary[minimum], value)
                summary[maximum] = value if summary[maximum] is None else max(summary[maximum], value)

            if message in {
                "Building block",
                "Graffiti includes client version info appended after user graffiti",
                "Voting period before genesis + follow distance, using eth1data from head",
                "Chose payload bid",
                "Finished building block",
            }:
                anchors.append(
                    {
                        "line": line,
                        "outer_timestamp": outer,
                        "offset_from_build_ms": offset_ms(outer),
                        "message": message,
                        "raw_prefix": text[:700],
                    }
                )

    if accounting["header_parse_failures"]:
        raise SystemExit(f"header parse gaps: {dict(accounting)}")
    build_anchor = next((item for item in anchors if item["message"] == "Building block"), None)
    finish_anchor = next((item for item in anchors if item["message"] == "Finished building block"), None)
    if build_anchor is None or build_anchor["line"] != BUILD_LINE:
        raise SystemExit(f"build anchor mismatch: {build_anchor}")
    if finish_anchor is None or finish_anchor["line"] != FINISH_LINE:
        raise SystemExit(f"finish anchor mismatch: {finish_anchor}")

    for counter in (
        "header_parse_failures",
        "records_before_window",
        "records_preceding_one_second",
        "records_build_through_finish",
        "records_after_finish_within_line_bound",
        "records_in_window",
        "physical_lf_records_scanned",
    ):
        accounting[counter] += 0
    for category in (
        "ffg_vote",
        "head_vote_ledger",
        "ffg_aggregate",
        "ptc_vote",
        "sync_committee",
        "block_construction",
        "other",
    ):
        category_counts[category] += 0

    result = {
        "scope": {
            "input": str(args.input),
            "input_size_bytes": args.input.stat().st_size,
            "scan_line_bounds_inclusive": [SCAN_FIRST_LINE, FINISH_LINE],
            "window_outer_timestamp_inclusive": [WINDOW_START, FINISH_TIME],
            "build_outer_timestamp": BUILD_TIME,
            "selected_raw_records_sha256": selected_digest.hexdigest(),
            "accounting": dict(sorted(accounting.items())),
            "previous_retained_record": previous_record,
            "clock_semantics": {
                "outer_timestamp": "retained log emission timestamp",
                "event_anchor_offset_from_build_ms": "outer timestamp minus 2026-09-05T01:49:24.008736237Z build-start outer timestamp",
                "Goldfish_arrivedMs": "validation-entry time minus the vote's slot start; beacon-chain/sync/vote_ledger.go:43-50",
                "Goldfish_decidedMs": "ledger-decision/log time minus the vote's slot start; beacon-chain/sync/vote_ledger.go:50-61",
            },
            "limitations": [
                "Counts are retained outer log records, not CPU, queue-depth, subscriber-insertion, or pool-snapshot measurements.",
                "The one-second pre-build interval contains no retained record; the immediately preceding record is reported separately.",
                "Sync counts recognize PTC vote and message labels containing sync committee or sync contribution; no such record occurs in this window.",
            ],
        },
        "category_counts": dict(sorted(category_counts.items())),
        "category_outcomes": {
            category: dict(sorted(outcomes.items()))
            for category, outcomes in sorted(category_outcomes.items())
        },
        "category_components": {
            category: dict(sorted(components.items()))
            for category, components in sorted(category_components.items())
        },
        "category_unique_validators": {
            category: len(validators)
            for category, validators in sorted(category_validators.items())
        },
        "message_counts": dict(sorted(message_counts.items())),
        "message_outcome_time_ranges": sorted(
            records.values(), key=lambda item: (item["first_line"], item["message"])
        ),
        "event_anchors": anchors,
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")


if __name__ == "__main__":
    main()
