#!/usr/bin/env python3
"""Validate recorded slot-15/16 Goldfish voters against the real committee rule.

Owner identities are rebuilt from `Validator activated` records in the selected
archive union. They are never inferred from validator or node numbers.
"""

from __future__ import annotations

import argparse
import concurrent.futures
import csv
import hashlib
import re
import struct
import tarfile
from collections import defaultdict
from pathlib import Path


DOMAIN = b"decoupled_mock_goldfish_committee"
VALIDATOR_COUNT = 120_000
SEAT_COUNT = 512
ARCHIVE_RE = re.compile(r"round2-prysm-geth-(\d+)\.tar\.gz$")
ACTIVATION_RE = re.compile(r"\bValidator activated\b.*\bvalidatorIndex=(\d+)\b")
ANSI_RE = re.compile(rb"\x1b\[[0-?]*[ -/]*[@-~]")


def committee(slot: int) -> tuple[int, list[int]]:
    digest = hashlib.sha256(DOMAIN + struct.pack(">Q", slot)).digest()
    offset = struct.unpack(">Q", digest[:8])[0] % VALIDATOR_COUNT
    return offset, sorted((offset + seat) % VALIDATOR_COUNT for seat in range(SEAT_COUNT))


def discover(directories: list[Path]) -> dict[int, Path]:
    choices: dict[int, list[Path]] = defaultdict(list)
    for directory in directories:
        for path in sorted(directory.glob("round2-prysm-geth-*.tar.gz")):
            match = ARCHIVE_RE.search(path.name)
            if match:
                choices[int(match.group(1))].append(path.resolve())
    selected = {node: paths[0] for node, paths in choices.items()}
    expected = set(range(1, 1001))
    if set(selected) != expected:
        raise RuntimeError(
            f"archive union is not nodes 1..1000; missing={sorted(expected-set(selected))}, "
            f"extra={sorted(set(selected)-expected)}"
        )
    return selected


def scan_activations(node: int, path: Path, wanted: set[int]) -> list[tuple[int, int, str]]:
    found: list[tuple[int, int, str]] = []
    with tarfile.open(path, "r:gz") as archive:
        members = [m for m in archive.getmembers() if m.isfile() and Path(m.name).name == "validator.log"]
        if len(members) != 1:
            raise RuntimeError(f"node {node}: expected one validator.log in {path}, got {len(members)}")
        source = archive.extractfile(members[0])
        if source is None:
            raise RuntimeError(f"node {node}: cannot read validator.log in {path}")
        for line_no, raw in enumerate(source, 1):
            clean = ANSI_RE.sub(b"", raw).decode("utf-8", "replace")
            match = ACTIVATION_RE.search(clean)
            if match and int(match.group(1)) in wanted:
                found.append((int(match.group(1)), node, f"{path}::validator.log:{line_no}"))
    return found


def ranges(values: set[int]) -> list[tuple[int, int]]:
    ordered = sorted(values)
    if not ordered:
        return []
    result: list[tuple[int, int]] = []
    start = previous = ordered[0]
    for value in ordered[1:]:
        if value != previous + 1:
            result.append((start, previous))
            start = value
        previous = value
    result.append((start, previous))
    return result


def write_tsv(path: Path, rows: list[dict[str, object]]) -> None:
    columns = list(rows[0])
    with path.open("w", newline="") as output:
        writer = csv.DictWriter(output, fieldnames=columns, delimiter="\t", lineterminator="\n")
        writer.writeheader()
        writer.writerows(rows)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("directories", nargs="+", type=Path)
    parser.add_argument("--votes", type=Path, default=Path(__file__).with_name("goldfish_votes_15_16_unique.tsv"))
    parser.add_argument("--output-dir", type=Path, default=Path(__file__).resolve().parent)
    parser.add_argument("--workers", type=int, default=8)
    args = parser.parse_args()

    expected_by_slot: dict[int, set[int]] = {}
    offsets: dict[int, int] = {}
    for slot in (15, 16):
        offsets[slot], validators = committee(slot)
        expected_by_slot[slot] = set(validators)
    wanted = set().union(*expected_by_slot.values())

    observed: list[dict[str, str]] = []
    with args.votes.open(newline="") as source:
        for row in csv.DictReader(source, delimiter="\t"):
            if int(row["slot"]) in expected_by_slot:
                observed.append(row)
    observed_sets: dict[tuple[int, int], set[int]] = defaultdict(set)
    for row in observed:
        if row["outcomes"] != "accepted" or int(row["seats"]) != 1 or int(row["record_count"]) != 1:
            raise RuntimeError(f"ledger row is not one unique accepted one-seat record: {row}")
        observed_sets[(int(row["observer"]), int(row["slot"]))].add(int(row["validator"]))
    if set(observed_sets) != {(201, 15), (201, 16), (400, 15), (400, 16)}:
        raise RuntimeError(f"unexpected observer/slot cohorts: {sorted(observed_sets)}")
    for key, validators in observed_sets.items():
        if validators != expected_by_slot[key[1]]:
            raise RuntimeError(f"recorded cohort {key} differs from computed committee")

    selected = discover(args.directories)
    activation_records: dict[int, list[tuple[int, str]]] = defaultdict(list)
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as executor:
        futures = [executor.submit(scan_activations, node, path, wanted) for node, path in selected.items()]
        for future in concurrent.futures.as_completed(futures):
            for validator, node, source_anchor in future.result():
                activation_records[validator].append((node, source_anchor))
    missing = sorted(wanted - set(activation_records))
    conflicts = {v: records for v, records in activation_records.items() if len({n for n, _ in records}) != 1}
    duplicate_records = {v: records for v, records in activation_records.items() if len(records) != 1}
    if missing or conflicts or duplicate_records:
        raise RuntimeError(
            f"activation mapping invalid: missing={missing}, conflicts={conflicts}, "
            f"duplicate_records={duplicate_records}"
        )
    owners = {validator: records[0][0] for validator, records in activation_records.items()}
    activation_anchors = {validator: records[0][1] for validator, records in activation_records.items()}

    rows: list[dict[str, object]] = []
    markdown = [
        "# Slot 15/16 Goldfish committee validation",
        "",
        "The validator sets below are recomputed with the production committee rule: "
        "SHA-256 of `decoupled_mock_goldfish_committee || uint64(slot, big-endian)`, "
        "the first eight digest bytes interpreted big-endian modulo 120,000, and the "
        "512 consecutive validator indices from that offset.",
        "",
        "Owner mappings were freshly read from `Validator activated` records across the "
        "1,000-node archive union. No validator-to-node arithmetic was used.",
        "",
        "| slot | offset / committee interval | owner intervals from activation logs | recorded observers |",
        "|---:|---|---|---|",
    ]
    for slot in (15, 16):
        committee_set = expected_by_slot[slot]
        owner_sets: dict[int, set[int]] = defaultdict(set)
        for validator in committee_set:
            owner_sets[owners[validator]].add(validator)
        owner_text = []
        for owner, validators in sorted(owner_sets.items()):
            span_text = ",".join(f"{lo}-{hi}" for lo, hi in ranges(validators))
            owner_text.append(f"node {owner}: {len(validators)} ({span_text})")
        committee_text = ",".join(f"{lo}-{hi}" for lo, hi in ranges(committee_set))
        markdown.append(f"| {slot} | {offsets[slot]} / {committee_text} | {'; '.join(owner_text)} | 201, 400 (exact 512/512) |")

        for observer in (201, 400):
            cohort = [row for row in observed if int(row["observer"]) == observer and int(row["slot"]) == slot]
            by_root_owner: dict[tuple[str, int], list[dict[str, str]]] = defaultdict(list)
            for row in cohort:
                validator = int(row["validator"])
                recorded_owner = int(row["validator_owner"])
                if recorded_owner != owners[validator]:
                    raise RuntimeError(f"recorded/fresh owner mismatch for validator {validator}")
                by_root_owner[(row["root"], owners[validator])].append(row)
            for (root, owner), group in sorted(by_root_owner.items(), key=lambda item: min(int(r["validator"]) for r in item[1])):
                validators = {int(row["validator"]) for row in group}
                spans = ranges(validators)
                rows.append({
                    "slot": slot,
                    "committee_offset": offsets[slot],
                    "committee_ranges": ";".join(f"{lo}-{hi}" for lo, hi in ranges(committee_set)),
                    "observer": observer,
                    "root": root,
                    "activation_owner_node": owner,
                    "validator_ranges": ";".join(f"{lo}-{hi}" for lo, hi in spans),
                    "validator_min": min(validators),
                    "validator_max": max(validators),
                    "unique_validators": len(validators),
                    "seat_sum": sum(int(row["seats"]) for row in group),
                    "outcomes": ";".join(sorted({row["outcomes"] for row in group})),
                    "record_count_sum": sum(int(row["record_count"]) for row in group),
                    "contiguous": len(spans) == 1,
                    "observer_committee_exact": observed_sets[(observer, slot)] == committee_set,
                    "min_validator_activation_anchor": activation_anchors[min(validators)],
                    "max_validator_activation_anchor": activation_anchors[max(validators)],
                })

    args.output_dir.mkdir(parents=True, exist_ok=True)
    write_tsv(args.output_dir / "goldfish_committee_intervals.tsv", rows)
    markdown.extend((
        "",
        "Both observers recorded every computed committee validator exactly once with one "
        "seat and outcome `accepted`. Each root/owner cohort is a contiguous subinterval. "
        "The 54/458 and 23/489 splits follow the shuffled activation ownership boundaries; "
        "the number of beacon nodes does not determine these seat counts.",
        "",
        "The input ledger exposes accepted vote records. This validation does not establish "
        "a complete historical in-memory Goldfish store or payload-presence state.",
    ))
    (args.output_dir / "goldfish_committee_validation.md").write_text("\n".join(markdown) + "\n")
    print(f"validated {len(wanted)} unique committee validators, {len(rows)} root/owner/observer intervals")


if __name__ == "__main__":
    main()
