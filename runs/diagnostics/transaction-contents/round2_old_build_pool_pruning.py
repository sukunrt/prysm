#!/usr/bin/env python3
"""Find exact FFG-vote reinclusions on Round 2's retained first-100 branch."""

from __future__ import annotations

import argparse
import collections
import hashlib
import json
import re
from pathlib import Path


ANSI_RE = re.compile(r"\x1b\[[0-9;]*m")
# The capture wrapper repeats this prefix inside very large physical lines. It
# can split a decimal validator index, so delete it before parsing the CSV.
OUTER_TIMESTAMP_RE = re.compile(r"2026-09-05T\d\d:\d\d:\d\d\.\d+Z ")
INCLUDED_RE = re.compile(
    r"FFG vote included .*?attSlot=(\d+) "
    r".*?blockRoot=(0x[0-9a-f]+) "
    r".*?blockSlot=(\d+) "
    r".*?committeeIndex=(\d+) "
    r".*?dataRoot=(0x[0-9a-f]+) "
    r".*?inclusionSlots=(\d+) "
    r".*?seats=(\d+) "
    r".*?validators=([0-9,]+)\s*$"
)

RETAINED_SLOTS = [
    15,
    *range(21, 34),
    *range(35, 39),
    *range(40, 45),
    *range(46, 51),
    *range(52, 65),
    66,
    67,
    69,
    71,
    72,
    73,
    76,
    77,
    79,
    80,
    *range(83, 97),
    98,
    99,
]

EXAMPLE_ROOTS = {
    "0x1fe5887a6af98797fcbe5d8470105e28543f2f671e55de5236cd2bc268037d",
    "0x4868980d9c37478e681c6e959156964064af28698a60009504f604a48d20c6",
}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "log",
        nargs="?",
        type=Path,
        default=Path("runs/round2/prysm-geth-400/beacon.log"),
    )
    args = parser.parse_args()

    by_slot: dict[int, dict[tuple[object, ...], dict[str, object]]] = (
        collections.defaultdict(dict)
    )
    examples: list[dict[str, object]] = []
    included_lines = 0
    parse_gaps: list[int] = []

    with args.log.open(errors="replace") as source:
        for line_number, raw in enumerate(source, 1):
            if "FFG vote included" not in raw:
                continue
            included_lines += 1
            timestamp = raw.split(" ", 1)[0]
            repaired = ANSI_RE.sub("", OUTER_TIMESTAMP_RE.sub("", raw))
            match = INCLUDED_RE.search(repaired)
            if match is None:
                parse_gaps.append(line_number)
                continue

            (
                att_slot,
                vote_block_root,
                block_slot,
                committee_index,
                data_root,
                inclusion_slots,
                seats,
                validators,
            ) = match.groups()
            block_slot_int = int(block_slot)
            key = (
                int(att_slot),
                vote_block_root,
                int(committee_index),
                data_root,
                validators,
            )
            record = {
                "line": line_number,
                "timestamp": timestamp,
                "blockSlot": block_slot_int,
                "attSlot": int(att_slot),
                "dataRoot": data_root,
                "inclusionSlots": int(inclusion_slots),
                "seats": int(seats),
                "validatorCount": len(validators.split(",")),
                "validatorCsvSha256": hashlib.sha256(validators.encode()).hexdigest(),
            }
            if block_slot_int in RETAINED_SLOTS:
                by_slot[block_slot_int][key] = record
            if block_slot_int in {94, 95, 96} and data_root in EXAMPLE_ROOTS:
                examples.append(record)

    repeats = []
    links_with_repeats = set()
    for parent, child in zip(RETAINED_SLOTS, RETAINED_SLOTS[1:]):
        for key in by_slot[parent].keys() & by_slot[child].keys():
            links_with_repeats.add((parent, child))
            repeats.append(
                {
                    "parent": parent,
                    "child": child,
                    "attSlot": key[0],
                    "dataRoot": key[3],
                    "parentLine": by_slot[parent][key]["line"],
                    "childLine": by_slot[child][key]["line"],
                }
            )

    result = {
        "source": str(args.log),
        "includedRecords": included_lines,
        "parseGaps": parse_gaps,
        "retainedLineageRecords": sum(len(records) for records in by_slot.values()),
        "retainedParentChildLinks": len(RETAINED_SLOTS) - 1,
        "exactParentChildRepeatRecords": len(repeats),
        "linksWithExactRepeats": len(links_with_repeats),
        "examples": sorted(examples, key=lambda row: (row["attSlot"], row["blockSlot"])),
    }
    print(json.dumps(result, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
