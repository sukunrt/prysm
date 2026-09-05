#!/usr/bin/env python3
"""Summarize round-2 Goldfish vote-ledger records without extracting archives."""

import argparse
import collections
import json
import re
import tarfile
from pathlib import Path

ANSI = re.compile(r"\x1b\[[0-?]*[ -/]*[@-~]")
FIELD = re.compile(r"\b([A-Za-z][A-Za-z0-9]*)=([^ ]+)")
NODE = re.compile(r"(?:round2-)?prysm-geth-(\d+)")


def beacon_lines(path):
    if path.is_dir():
        with (path / "beacon.log").open(errors="replace") as src:
            yield from src
        return
    with tarfile.open(path, "r|gz") as archive:
        for member in archive:
            # Stream only a regular file whose basename is exactly beacon.log.
            if not member.isfile() or Path(member.name).name != "beacon.log":
                continue
            src = archive.extractfile(member)
            if src is not None:
                for raw in src:
                    yield raw.decode("utf-8", "replace")
            return


def activation_owners(directories):
    owners = {}
    paths = {path for directory in directories for path in directory.glob("round2-prysm-geth-*.tar.gz")}
    for path in sorted(paths):
        match = NODE.search(path.name)
        if not match:
            continue
        node = int(match.group(1))
        with tarfile.open(path, "r|gz") as archive:
            for member in archive:
                if not member.isfile() or Path(member.name).name != "validator.log":
                    continue
                src = archive.extractfile(member)
                if src is not None:
                    for raw in src:
                        line = ANSI.sub("", raw.decode("utf-8", "replace")).strip()
                        if "Validator activated " not in line:
                            continue
                        fields = dict(FIELD.findall(line))
                        if fields.get("validatorIndex", "").isdigit():
                            index = int(fields["validatorIndex"])
                            previous = owners.get(index)
                            if previous is not None and previous != node:
                                raise ValueError(
                                    f"validator index {index} activated on both node {previous} and node {node}"
                                )
                            owners[index] = node
                break
    return owners


def parse_archive(path, min_slot, max_slot, owners):
    match = NODE.search(path.name)
    node = int(match.group(1)) if match else None
    records = []
    for line_no, raw in enumerate(beacon_lines(path), 1):
        line = ANSI.sub("", raw.rstrip())
        if "Goldfish vote " not in line:
            continue
        fields = dict(FIELD.findall(line))
        try:
            slot = int(fields["voteSlot"])
            validator = int(fields["validator"])
            seats = int(fields["seats"])
        except (KeyError, ValueError):
            continue
        if not min_slot <= slot <= max_slot:
            continue
        records.append({
            "node": node,
            "line": line_no,
            "time": line[:30],
            "slot": slot,
            "root": fields.get("blockRoot"),
            "outcome": fields.get("outcome"),
            "reason": fields.get("reason"),
            "validator": validator,
            "validator_owner": owners.get(validator),
            "seats": seats,
            "arrived_ms": int(fields["arrivedMs"]) if fields.get("arrivedMs", "").isdigit() else None,
            "decided_ms": int(fields["decidedMs"]) if fields.get("decidedMs", "").isdigit() else None,
        })
    return node, records


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("archives", nargs="+")
    parser.add_argument("--min-slot", type=int, default=0)
    parser.add_argument("--max-slot", type=int, default=100)
    parser.add_argument("--records", action="store_true")
    parser.add_argument("--owner-archives-dir", type=Path, action="append",
                        help="derive validator-index ownership from activation rows in this archive directory")
    args = parser.parse_args()
    owners = activation_owners(args.owner_archives_dir) if args.owner_archives_dir else {}
    output = {"nodes": {}, "owner_map_size": len(owners)}
    for name in args.archives:
        node, records = parse_archive(Path(name), args.min_slot, args.max_slot, owners)
        slots = collections.defaultdict(lambda: collections.defaultdict(lambda: {"seats": 0, "validators": [], "arrived_ms": [], "decided_ms": []}))
        for row in records:
            key = f"{row['outcome']}:{row['reason'] or ''}:{row['root']}"
            item = slots[row["slot"]][key]
            item["seats"] += row["seats"]
            item["validators"].append(row["validator"])
            if row["arrived_ms"] is not None:
                item["arrived_ms"].append(row["arrived_ms"])
            if row["decided_ms"] is not None:
                item["decided_ms"].append(row["decided_ms"])
        summary = {}
        for slot, groups in sorted(slots.items()):
            summary[str(slot)] = {}
            for key, item in sorted(groups.items()):
                validators = item.pop("validators")
                for timing in ("arrived_ms", "decided_ms"):
                    values = item[timing]
                    item[timing] = [min(values), max(values)] if values else None
                item["records"] = len(validators)
                item["validator_range"] = [min(validators), max(validators)]
                known = (owners[v] for v in validators if v in owners)
                item["owner_counts"] = dict(sorted(collections.Counter(known).items()))
                summary[str(slot)][key] = item
        result = {"source": name, "record_count": len(records), "slots": summary}
        if args.records:
            result["records"] = records
        output["nodes"][str(node)] = result
    print(json.dumps(output, separators=(",", ":"), sort_keys=True))


if __name__ == "__main__":
    main()
