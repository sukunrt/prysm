#!/usr/bin/env python3
"""Union round2 block imports and proposer outcomes across local archives."""

import argparse
import datetime as dt
import json
import re
import tarfile
from pathlib import Path

ANSI = re.compile(r"\x1b\[[0-?]*[ -/]*[@-~]")
ARCHIVE = re.compile(r"round2-prysm-geth-(\d+)\.tar\.gz$")
OUTER_TIME = re.compile(r"^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?Z)")
SLOT = re.compile(r"\bslot=(\d+)\b")
PROPOSER = re.compile(r"\bproposerPubkey=(0x[0-9a-f]+)\b")
DURATION_PART = re.compile(r"(\d+)(h|m|s)")


def stamp(line):
    match = OUTER_TIME.match(line)
    return dt.datetime.fromisoformat(match.group(1).replace("Z", "+00:00")) if match else None


def proposer_slot(line, genesis):
    """Recover proposer-only duty slots, whose log rows omit the slot field."""
    match = SLOT.search(line)
    if match:
        return int(match.group(1))
    when = stamp(line)
    duration = re.search(r"\btimeUntilDuty=([^ ]+)", line)
    if when is None or duration is None:
        return None
    seconds = sum(int(value) * {"h": 3600, "m": 60, "s": 1}[unit]
                  for value, unit in DURATION_PART.findall(duration.group(1)))
    estimate = ((when - genesis).total_seconds() + seconds) / 12
    candidate = round(estimate)
    # logDuties adds one second before truncating, so the recovered start is
    # within one second (apart from timestamp precision) of the actual slot.
    return candidate if abs(estimate * 12 - candidate * 12) <= 1.1 else None


def read_logs(path):
    result = {}
    with tarfile.open(path, "r:gz") as archive:
        for member in archive:
            name = Path(member.name).name
            if not member.isfile() or name not in ("beacon.log", "validator.log"):
                continue
            source = archive.extractfile(member)
            if source is not None:
                result[name] = [ANSI.sub("", line.decode("utf-8", "replace")).rstrip() for line in source]
    return result


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("directories", nargs="+", type=Path)
    ap.add_argument("--genesis", type=dt.datetime.fromisoformat, required=True)
    ap.add_argument("--max-slot", type=int, default=226)
    args = ap.parse_args()
    genesis = args.genesis.replace(tzinfo=dt.timezone.utc) if args.genesis.tzinfo is None else args.genesis

    # Prefer the first directory's copy when the same node archive is present twice.
    paths = {}
    for directory in args.directories:
        for path in sorted(directory.glob("round2-prysm-geth-*.tar.gz")):
            match = ARCHIVE.search(path.name)
            if match:
                paths.setdefault(int(match.group(1)), path)

    blocks = {}
    duties = {}
    outcomes = []
    reorgs = []
    coverage = {}
    for node, path in sorted(paths.items()):
        logs = read_logs(path)
        latest = None
        for line_no, line in enumerate(logs.get("beacon.log", []), 1):
            when = stamp(line)
            if when is not None:
                latest = max(latest, when) if latest else when
            if "Chain reorg occurred" in line:
                fields = {}
                for key in ("commonAncestorRoot", "newRoot", "newSlot", "oldRoot", "oldSlot", "depth", "distance"):
                    value = re.search(rf"\b{key}=([^ ]+)", line)
                    fields[key] = value.group(1) if value else None
                reorgs.append({"node": node, "line": line_no,
                               "time": when.isoformat() if when else None, **fields})
            match = SLOT.search(line)
            if not match:
                continue
            slot = int(match.group(1))
            if slot > args.max_slot:
                continue
            if "Synced new block" in line:
                root = re.search(r"\bblock=(0x[0-9a-f]+)", line)
                blocks.setdefault(slot, {}).setdefault(root.group(1) if root else "unknown", []).append(
                    {"node": node, "line": line_no, "time": when.isoformat() if when else None}
                )
            elif "Finished applying state transition" in line:
                parent = re.search(r"\bparentHash=(0x[0-9a-f]+)", line)
                payload = re.search(r"\bpayloadHash=(0x[0-9a-f]+)", line)
                blocks.setdefault(slot, {}).setdefault("execution", []).append(
                    {"node": node, "line": line_no, "parentHash": parent.group(1) if parent else None,
                     "payloadHash": payload.group(1) if payload else None}
                )
        if latest:
            coverage[node] = latest.isoformat()

        node_duties = {}
        pending_outcomes = []
        for line_no, line in enumerate(logs.get("validator.log", []), 1):
            slot_match, key_match = SLOT.search(line), PROPOSER.search(line)
            if "Duties schedule" in line and key_match:
                slot = proposer_slot(line, genesis)
                if slot is None:
                    continue
                if slot <= args.max_slot:
                    duty = {"node": node, "pubkey": key_match.group(1), "line": line_no}
                    if not any(d["pubkey"] == duty["pubkey"] for d in node_duties.setdefault(slot, [])):
                        node_duties[slot].append(duty)
                    if not any(d["node"] == node and d["pubkey"] == duty["pubkey"]
                               for d in duties.setdefault(slot, [])):
                        duties[slot].append(duty)
            if not re.search(r"Submitted new block|Failed to sign randao reveal|Failed to request block from beacon node|Failed to propose block", line):
                continue
            when = stamp(line)
            key = re.search(r"\bpubkey=(0x[0-9a-f]+)", line)
            explicit = SLOT.search(line)
            slot = int(explicit.group(1)) if explicit else None
            pending_outcomes.append((line_no, line, when, key, slot))
        for line_no, line, when, key, slot in pending_outcomes:
            if slot is None and key:
                candidates = [s for s, records in node_duties.items()
                              if any(duty["pubkey"] == key.group(1) for duty in records)]
                if when:
                    candidates = [s for s in candidates if abs((when - (genesis + dt.timedelta(seconds=12*(s+1)))).total_seconds()) < 1]
                slot = candidates[0] if len(candidates) == 1 else None
            if slot is not None and slot <= args.max_slot:
                outcomes.append({"slot": slot, "node": node, "line": line_no,
                                 "time": when.isoformat() if when else None, "text": line[:500]})

    result = {
        "archive_nodes": sorted(paths),
        "coverage": coverage,
        "reorgs": reorgs,
        "slots": [],
    }
    for slot in range(args.max_slot + 1):
        roots = blocks.get(slot, {})
        result["slots"].append({
            "slot": slot,
            "duties": duties.get(slot, []),
            "beacon_roots": {k: v for k, v in roots.items() if k != "execution"},
            "execution_lineage": roots.get("execution", []),
            "outcomes": [o for o in outcomes if o["slot"] == slot],
        })
    print(json.dumps(result, separators=(",", ":"), sort_keys=True))


if __name__ == "__main__":
    main()
