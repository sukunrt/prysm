#!/usr/bin/env python3
"""Render owner construction and observation stages for Round 2 slots 0..100."""

import argparse
import datetime as dt
import json
import re
import tarfile
from pathlib import Path

ANSI = re.compile(r"\x1b\[[0-?]*[ -/]*[@-~]")
STAMP = re.compile(r"^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?Z)")
SLOT = re.compile(r"\bslot=(\d+)\b")


def timestamp(line):
    match = STAMP.match(line)
    return dt.datetime.fromisoformat(match.group(1).replace("Z", "+00:00")) if match else None


def offset(when, genesis, slot):
    if when is None:
        return "--"
    return f"{(when - genesis).total_seconds() - 12 * slot:+.3f}s"


def log_lines(path, log_name):
    with tarfile.open(path, "r:gz") as archive:
        member = next((m for m in archive if Path(m.name).name == log_name), None)
        if member is None:
            return []
        return [ANSI.sub("", raw.decode("utf-8", "replace")).rstrip()
                for raw in archive.extractfile(member)]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("union", type=Path)
    parser.add_argument("archives", nargs="+", type=Path)
    parser.add_argument("--genesis", required=True, type=dt.datetime.fromisoformat)
    args = parser.parse_args()
    genesis = args.genesis.replace(tzinfo=dt.timezone.utc) if args.genesis.tzinfo is None else args.genesis
    data = json.loads(args.union.read_text())
    paths = {}
    for directory in args.archives:
        for path in directory.glob("round2-prysm-geth-*.tar.gz"):
            match = re.search(r"-(\d+)\.tar\.gz$", path.name)
            if match:
                paths.setdefault(int(match.group(1)), path)

    rows = {row["slot"]: row for row in data["slots"] if row["slot"] <= 100}
    reorgs = {}
    for event in data["reorgs"]:
        if not event.get("oldSlot") or not event.get("time"):
            continue
        old = int(event["oldSlot"])
        wall = int((dt.datetime.fromisoformat(event["time"]) - genesis).total_seconds() // 12)
        depth = int(event.get("depth") or 0)
        if old <= 100 and wall <= 101 and depth > 0 and event.get("newSlot") != event.get("oldSlot"):
            reorgs.setdefault(old, []).append((wall, event["node"], event["newSlot"]))

    print("| slot | duty owner(s) | BN build | payload chosen | BN finish | earliest import | VC submit | injected late publish | observed reorg-out |")
    print("| ---: | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |")
    for slot in range(101):
        row = rows[slot]
        owners = sorted({d["node"] for d in row["duties"]})
        stages = {"build": None, "payload": None, "finish": None}
        late_publish = False
        for owner in owners:
            path = paths.get(owner)
            if path is None:
                continue
            for line in log_lines(path, "beacon.log"):
                match = SLOT.search(line)
                if not match or int(match.group(1)) != slot:
                    continue
                when = timestamp(line)
                if "Building block" in line:
                    stages["build"] = min(filter(None, (stages["build"], when)), default=when)
                if "Chose payload bid" in line:
                    stages["payload"] = min(filter(None, (stages["payload"], when)), default=when)
                if "Finished building block" in line:
                    stages["finish"] = min(filter(None, (stages["finish"], when)), default=when)
            for line in log_lines(path, "validator.log"):
                match = SLOT.search(line)
                if match and int(match.group(1)) == slot and "Publishing block late" in line:
                    late_publish = True
        imports = [dt.datetime.fromisoformat(e["time"])
                   for events in row["beacon_roots"].values() for e in events if e.get("time")]
        submits = [dt.datetime.fromisoformat(e["time"]) for e in row["outcomes"]
                   if e.get("time") and "Submitted new block" in e.get("text", "")]
        rs = reorgs.get(slot, [])
        reorg_text = "--" if not rs else f"wall {min(x[0] for x in rs)} ({len(rs)} logs; to {','.join(sorted({x[2] for x in rs}))})"
        print("| {} | {} | {} | {} | {} | {} | {} | {} | {} |".format(
            slot, ",".join(map(str, owners)) or "--", offset(stages["build"], genesis, slot),
            offset(stages["payload"], genesis, slot), offset(stages["finish"], genesis, slot),
            offset(min(imports) if imports else None, genesis, slot),
            offset(min(submits) if submits else None, genesis, slot), "yes" if late_publish else "--", reorg_text))


if __name__ == "__main__":
    main()
