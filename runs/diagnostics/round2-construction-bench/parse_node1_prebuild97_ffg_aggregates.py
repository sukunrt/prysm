#!/usr/bin/env python3
"""Census node 1 FFG ledger rows emitted before slot-97 build start."""

from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import re
from collections import Counter, defaultdict
from datetime import datetime
from itertools import combinations
from pathlib import Path


ANSI_RE = re.compile(r"\x1b\[[0-9;]*m")
FIELD_RE = re.compile(r"(?:^|\s)([A-Za-z][A-Za-z0-9]*)=([^\s]+)")


def timestamp(text: str) -> str:
    parsed = datetime.fromisoformat(text.split(" ", 1)[0].replace("Z", "+00:00"))
    return parsed.isoformat().replace("+00:00", "Z")


def set_fingerprint(validators: frozenset[int]) -> str:
    packed = ",".join(str(value) for value in sorted(validators)).encode()
    return hashlib.sha256(packed).hexdigest()


def overlap_components(sets: list[frozenset[int]]) -> list[int]:
    unseen = set(range(len(sets)))
    sizes: list[int] = []
    while unseen:
        stack = [unseen.pop()]
        size = 0
        while stack:
            left = stack.pop()
            size += 1
            neighbors = [right for right in unseen if sets[left] & sets[right]]
            for right in neighbors:
                unseen.remove(right)
                stack.append(right)
        sizes.append(size)
    return sorted(sizes, reverse=True)


def unique_sets(rows: list[dict], outcome: str | None = None) -> dict[frozenset[int], list[dict]]:
    result: dict[frozenset[int], list[dict]] = defaultdict(list)
    for row in rows:
        if outcome is None or row["outcome"] == outcome:
            result[row["_validators"]].append(row)
    return result


def set_selection(unique: dict[frozenset[int], list[dict]]) -> dict:
    sets = list(unique)
    if not sets:
        return {
            "distinct_sets": 0,
            "largest_participant_count": None,
            "largest_set_fingerprints": [],
            "largest_sets_with_gossip": 0,
            "maximal_noncontained_set_fingerprints": [],
            "universal_cover_set_fingerprints": [],
            "validator_union": frozenset(),
            "largest_set_union": frozenset(),
        }
    largest_count = max(map(len, sets))
    largest = [value for value in sets if len(value) == largest_count]
    maximal = [value for value in sets if not any(value < other for other in sets)]
    universal = [value for value in sets if all(other <= value for other in sets)]
    return {
        "distinct_sets": len(sets),
        "largest_participant_count": largest_count,
        "largest_set_fingerprints": sorted(map(set_fingerprint, largest)),
        "largest_sets_with_gossip": sum(
            any(row["outcome"] == "gossip" for row in unique[value]) for value in largest
        ),
        "maximal_noncontained_set_fingerprints": sorted(map(set_fingerprint, maximal)),
        "universal_cover_set_fingerprints": sorted(map(set_fingerprint, universal)),
        "validator_union": frozenset().union(*sets),
        "largest_set_union": frozenset().union(*largest),
    }


def coverage(validators: frozenset[int], selection: dict) -> dict:
    all_union = selection["validator_union"]
    largest_union = selection["largest_set_union"]
    return {
        "single_validator_count": len(validators),
        "covered_by_any_observed_set": len(validators & all_union),
        "not_in_any_observed_set": len(validators - all_union),
        "covered_by_largest_observed_sets": len(validators & largest_union),
        "not_in_largest_observed_sets": len(validators - largest_union),
    }


def summarize_group(key: tuple[int, str, int], rows: list[dict], singles: dict | None) -> tuple[dict, dict]:
    all_unique = unique_sets(rows)
    gossip_unique = unique_sets(rows, "gossip")
    sets = list(all_unique)
    pair_overlaps: list[int] = []
    subset_pairs = 0
    disjoint_pairs = 0
    for left, right in combinations(sets, 2):
        overlap = len(left & right)
        pair_overlaps.append(overlap)
        disjoint_pairs += overlap == 0
        subset_pairs += left <= right or right <= left

    all_selection = set_selection(all_unique)
    gossip_selection = set_selection(gossip_unique)
    set_metadata = []
    fixture_sets = []
    for validators, set_rows in sorted(all_unique.items(), key=lambda item: set_fingerprint(item[0])):
        fingerprint = set_fingerprint(validators)
        set_metadata.append({
            "sha256_sorted_validator_csv": fingerprint,
            "participant_count": len(validators),
            "row_count": len(set_rows),
            "outcomes": dict(sorted(Counter(row["outcome"] for row in set_rows).items())),
            "lines": sorted(row["line"] for row in set_rows),
        })
        fixture_sets.append({
            "sha256_sorted_validator_csv": fingerprint,
            "validators": sorted(validators),
        })

    emitted = [row["outer_timestamp"] for row in rows]
    block_roots = sorted({row["blockRoot"] for row in rows})
    single_validators = frozenset() if singles is None else frozenset(singles["validators"])
    single_block_roots = [] if singles is None else sorted(singles["blockRoots"])
    public = {
        "attSlot": key[0],
        "dataRoot": key[1],
        "committeeIndex": key[2],
        "blockRoots": block_roots,
        "single_blockRoots": single_block_roots,
        "row_count": len(rows),
        "outcomes": dict(sorted(Counter(row["outcome"] for row in rows).items())),
        "participant_count_min": min(map(len, sets)),
        "participant_count_max": max(map(len, sets)),
        "union_validator_count": len(frozenset().union(*sets)),
        "unique_validator_sets": len(sets),
        "maximal_noncontained_distinct_sets": len(all_selection["maximal_noncontained_set_fingerprints"]),
        "universal_cover_distinct_sets": len(all_selection["universal_cover_set_fingerprints"]),
        "one_observed_set_covers_all_others": len(all_selection["universal_cover_set_fingerprints"]) == 1,
        "pair_count": len(pair_overlaps),
        "pairwise_subset_or_equal_pairs": subset_pairs,
        "pairwise_disjoint_pairs": disjoint_pairs,
        "pairwise_positive_overlap_min": min((value for value in pair_overlaps if value), default=None),
        "pairwise_overlap_max": max(pair_overlaps, default=None),
        "overlap_component_sizes": overlap_components(sets),
        "emission_min": min(emitted),
        "emission_max": max(emitted),
        "set_fingerprints": set_metadata,
        "all_ledger_set_selection": {name: value for name, value in all_selection.items() if not isinstance(value, frozenset)},
        "gossip_only_set_selection": {name: value for name, value in gossip_selection.items() if not isinstance(value, frozenset)},
        "single_rows": 0 if singles is None else singles["rows"],
        "single_outcomes": {} if singles is None else dict(sorted(singles["outcomes"].items())),
        "all_ledger_single_coverage": coverage(single_validators, all_selection),
        "gossip_only_single_coverage": coverage(single_validators, gossip_selection),
    }
    fixture = {
        "attSlot": key[0],
        "dataRoot": key[1],
        "committeeIndex": key[2],
        "blockRoots": block_roots,
        "sets": fixture_sets,
    }
    return public, fixture


def section(groups: list[dict], include_groups: bool = True) -> dict:
    def total(path: str, field: str) -> int:
        return sum(group[path][field] for group in groups)

    result = {
        "group_count": len(groups),
        "row_count": sum(group["row_count"] for group in groups),
        "unique_validator_set_count": sum(group["unique_validator_sets"] for group in groups),
        "maximal_noncontained_distinct_set_count": sum(group["maximal_noncontained_distinct_sets"] for group in groups),
        "universal_cover_distinct_set_count": sum(group["universal_cover_distinct_sets"] for group in groups),
        "groups_with_one_observed_set_covering_all_others": sum(group["one_observed_set_covers_all_others"] for group in groups),
        "maximum_rows_in_group": max((group["row_count"] for group in groups), default=0),
        "maximum_unique_sets_in_group": max((group["unique_validator_sets"] for group in groups), default=0),
        "single_coverage": {
            "groups_with_aggregates": len(groups),
            "single_validator_count_in_these_groups": total("all_ledger_single_coverage", "single_validator_count"),
            "all_ledger": {
                field: total("all_ledger_single_coverage", field)
                for field in (
                    "covered_by_any_observed_set",
                    "not_in_any_observed_set",
                    "covered_by_largest_observed_sets",
                    "not_in_largest_observed_sets",
                )
            },
            "gossip_only": {
                field: total("gossip_only_single_coverage", field)
                for field in (
                    "covered_by_any_observed_set",
                    "not_in_any_observed_set",
                    "covered_by_largest_observed_sets",
                    "not_in_largest_observed_sets",
                )
            },
        },
    }
    if include_groups:
        result["groups"] = groups
    return result


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, default=Path("runs/round2/prysm-geth-1/beacon.log"))
    parser.add_argument("--output", type=Path, default=Path("runs/diagnostics/round2-construction-bench/node1-prebuild97-ffg-aggregate-census.json"))
    parser.add_argument("--validator-sets-output", type=Path, default=Path("runs/diagnostics/round2-construction-bench/node1-prebuild97-ffg-aggregate-validator-sets.json.gz"))
    args = parser.parse_args()

    aggregate_groups: dict[tuple[int, str, int], list[dict]] = defaultdict(list)
    single_groups: dict[tuple[int, str, int], dict] = {}
    parsing = Counter()
    prefix_digest = hashlib.sha256()
    physical_lines = 0
    cutoff = None
    with args.input.open("rb") as source:
        for physical_lines, raw in enumerate(source, 1):
            prefix_digest.update(raw)
            text = ANSI_RE.sub("", raw.decode("utf-8", errors="replace")).rstrip("\r\n")
            fields = dict(FIELD_RE.findall(text))
            if " Building block " in text and fields.get("slot") == "97":
                cutoff = {"line": physical_lines, "outer_timestamp": timestamp(text), "raw_prefix": text[:500]}
                break

            is_aggregate = " FFG aggregate " in text and " FFG aggregate groups " not in text
            is_single = " FFG vote " in text and " FFG vote included " not in text
            if not is_aggregate and not is_single:
                continue
            kind = "aggregate" if is_aggregate else "single"
            parsing[f"{kind}_candidate_rows_before_cutoff"] += 1
            try:
                att_slot = int(fields["attSlot"])
            except (KeyError, ValueError):
                parsing[f"{kind}_missing_or_invalid_attSlot"] += 1
                continue
            if not 64 <= att_slot <= 96:
                continue
            parsing[f"{kind}_selected_attSlots_64_96"] += 1
            try:
                key = (att_slot, fields["dataRoot"], int(fields["committeeIndex"]))
                block_root = fields["blockRoot"]
                outcome = fields["outcome"]
                outer = timestamp(text)
                arrived = int(fields["arrivedMs"])
                if is_aggregate:
                    validators = frozenset(int(value) for value in fields["validators"].split(","))
                    logged_seats = int(fields["seats"])
                    aggregator = int(fields["aggregatorIndex"])
                    if len(validators) != logged_seats:
                        parsing["aggregate_seat_mismatch"] += 1
                        raise ValueError(f"line {physical_lines}: parsed {len(validators)} validators, seats={logged_seats}")
                    aggregate_groups[key].append({
                        "line": physical_lines,
                        "outer_timestamp": outer,
                        "arrivedMs": arrived,
                        "outcome": outcome,
                        "aggregatorIndex": aggregator,
                        "logged_seats": logged_seats,
                        "blockRoot": block_root,
                        "_validators": validators,
                    })
                else:
                    validator = int(fields["validator"])
                    group = single_groups.setdefault(key, {"validators": set(), "blockRoots": set(), "outcomes": Counter(), "rows": 0})
                    group["validators"].add(validator)
                    group["blockRoots"].add(block_root)
                    group["outcomes"][outcome] += 1
                    group["rows"] += 1
                parsing[f"{kind}_parsed_attSlots_64_96"] += 1
            except (KeyError, ValueError) as err:
                parsing[f"{kind}_parse_failures_attSlots_64_96"] += 1
                if att_slot >= 88:
                    raise SystemExit(f"eligible {kind} parse failure at LF record {physical_lines}: {err}") from err

    if cutoff is None:
        raise SystemExit("slot-97 Building block cutoff not found")
    if parsing["aggregate_parse_failures_attSlots_64_96"] or parsing["aggregate_seat_mismatch"]:
        raise SystemExit(f"aggregate parse gaps: {dict(parsing)}")
    if parsing["single_parse_failures_attSlots_64_96"]:
        raise SystemExit(f"single parse gaps: {dict(parsing)}")

    summaries = []
    fixture_groups = []
    for key, rows in sorted(aggregate_groups.items()):
        public, fixture = summarize_group(key, rows, single_groups.get(key))
        summaries.append(public)
        if key[0] >= 88:
            fixture_groups.append(fixture)
    eligible = [group for group in summaries if group["attSlot"] >= 88]
    expired = [group for group in summaries if group["attSlot"] < 88]

    aggregate_keys = set(aggregate_groups)
    eligible_single_without_aggregate = {
        key: value for key, value in single_groups.items() if key[0] >= 88 and key not in aggregate_keys
    }
    missing_single_count = sum(len(value["validators"]) for value in eligible_single_without_aggregate.values())
    eligible_section = section(eligible)
    eligible_section["single_coverage"]["groups_without_logged_aggregate"] = len(eligible_single_without_aggregate)
    eligible_section["single_coverage"]["single_validators_without_logged_aggregate"] = missing_single_count
    eligible_section["single_coverage"]["single_rows_in_groups_with_aggregates"] = sum(group["single_rows"] for group in eligible)
    eligible_section["single_coverage"]["groups_without_logged_aggregate_detail"] = [
        {
            "attSlot": key[0],
            "dataRoot": key[1],
            "committeeIndex": key[2],
            "blockRoots": sorted(value["blockRoots"]),
            "row_count": value["rows"],
            "unique_validators": len(value["validators"]),
            "outcomes": dict(sorted(value["outcomes"].items())),
        }
        for key, value in sorted(eligible_single_without_aggregate.items())
    ]
    for view in ("all_ledger", "gossip_only"):
        eligible_section["single_coverage"][view]["total_not_in_largest_including_groups_without_aggregate"] = eligible_section["single_coverage"][view]["not_in_largest_observed_sets"] + missing_single_count

    validator_sets_payload = {
        "scope": {
            "description": "Exact sorted validator IDs for every distinct eligible aggregate set; group/set fingerprints join to the census JSON.",
            "attSlots": [88, 96],
            "cutoff": cutoff,
        },
        "groups": fixture_groups,
    }
    args.validator_sets_output.parent.mkdir(parents=True, exist_ok=True)
    encoded_sets = json.dumps(validator_sets_payload, sort_keys=True, separators=(",", ":")).encode() + b"\n"
    with gzip.GzipFile(filename=str(args.validator_sets_output), mode="wb", compresslevel=9, mtime=0) as destination:
        destination.write(encoded_sets)

    result = {
        "scope": {
            "input": str(args.input),
            "input_size_bytes": args.input.stat().st_size,
            "prefix_through_cutoff_sha256": prefix_digest.hexdigest(),
            "physical_lf_records_through_cutoff": physical_lines,
            "cutoff": cutoff,
            "group_key": ["attSlot", "dataRoot", "committeeIndex"],
            "parsing_census": dict(sorted(parsing.items())),
            "validator_sets_artifact": str(args.validator_sets_output),
            "validator_sets_uncompressed_sha256": hashlib.sha256(encoded_sets).hexdigest(),
            "limitations": [
                "Rows are ledger emissions, not a proposal-pool snapshot or proof of subscriber insertion.",
                "Coverage means exact membership of logged singles in logged aggregate validator CSVs sharing attSlot, dataRoot, and committee.",
                "Gossip-only excludes local ledger rows; neither view proves which objects were present when proposal 97 took its pool snapshot.",
                "A unique universal-cover set covers all other observed sets in its group even when earlier partial sets overlap and are pairwise incomparable.",
                "Slots 88-96 are proposal-relevant round-11/12 inputs; slots 64-87 are separated as outside that eligible window.",
            ],
        },
        "eligible_attSlots_88_96": eligible_section,
        "expired_or_ineligible_attSlots_64_87": section(expired, include_groups=False),
        "all_attSlots_64_96": section(summaries, include_groups=False),
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, sort_keys=True, separators=(",", ":")) + "\n")


if __name__ == "__main__":
    main()
