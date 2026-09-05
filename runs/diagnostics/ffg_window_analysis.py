#!/usr/bin/env python3
import collections
import datetime as dt
import glob
import json
import re
import subprocess

ANSI = re.compile(r"\x1b\[[0-9;]*m")
STAMP = re.compile(r"\[(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d(?:\.\d+)?)\]")
FIELD = re.compile(r"\b([A-Za-z][A-Za-z0-9]*)=([^ ]+)")
GENESIS = {
    "round1": dt.datetime.fromisoformat("2026-09-05 00:00:00"),
    "round2": dt.datetime.fromisoformat("2026-09-05 01:30:00"),
}


def window(ms):
    if ms < 6000:
        return "<6s"
    if ms < 12000:
        return "6-12s"
    if ms < 24000:
        return "12-24s"
    return ">=24s"


rows = []
for round_name, genesis in GENESIS.items():
    paths = glob.glob(f"runs/{round_name}/prysm-geth-*/beacon.log")
    matched = subprocess.run(
        # ANSI may surround the key/value. The parsed checks below still select
        # exact slot 1 and require an arrival clock.
        ["rg", "--no-heading", r" FFG vote .*attSlot(?:\x1b\[[0-9;]*m)*=(?:\x1b\[[0-9;]*m)*1(?:\x1b\[[0-9;]*m)*(?:[[:space:]]|$)", *paths],
        check=False, capture_output=True, text=True, errors="replace",
    ).stdout.splitlines()
    by_path = collections.defaultdict(list)
    for raw in matched:
        path, line = raw.split(":", 1)
        by_path[path].append(line)
    for path, lines in by_path.items():
        node = path.split("/")[-2].removeprefix("prysm-geth-")
        for raw in lines:
                line = ANSI.sub("", raw.rstrip("\r"))
                if " FFG vote " not in line:
                    continue
                fields = dict(FIELD.findall(line))
                if fields.get("attSlot") != "1" or "arrivedMs" not in fields:
                    continue
                stamp_match = STAMP.search(line)
                if not stamp_match:
                    continue
                stamp = dt.datetime.fromisoformat(stamp_match.group(1))
                emission_ms = int((stamp - (genesis + dt.timedelta(seconds=12))).total_seconds() * 1000)
                identity = "|".join(fields.get(k, "") for k in (
                    "validator", "committeeIndex", "dataRoot", "blockRoot", "attSlot"
                ))
                rows.append({
                    "round": round_name,
                    "node": node,
                    "outcome": fields.get("outcome", "unknown"),
                    "committee": fields.get("committeeIndex", "unknown"),
                    "arrival_window": window(int(fields["arrivedMs"])),
                    "emission_window": window(emission_ms),
                    "identity": identity,
                })

result = []
for round_name, node in sorted({(r["round"], r["node"]) for r in rows}):
    selected = [r for r in rows if r["round"] == round_name and r["node"] == node]
    item = {"round": round_name, "node": node, "records": len(selected), "unique": len({r["identity"] for r in selected})}
    for clock in ("arrival", "emission"):
        groups = []
        for win in ("<6s", "6-12s", "12-24s", ">=24s"):
            wr = [r for r in selected if r[f"{clock}_window"] == win]
            outcomes = dict(sorted(collections.Counter(r["outcome"] for r in wr).items()))
            committees = collections.Counter(r["committee"] for r in wr)
            biggest = max(committees.values(), default=0)
            groups.append({"window": win, "records": len(wr), "outcomes": outcomes, "biggest_committee": biggest})
        item[clock] = groups
    result.append(item)

print(json.dumps(result, indent=2))

for round_name in GENESIS:
    selected = [r for r in rows if r["round"] == round_name]
    print(json.dumps({
        "round": round_name,
        "records": len(selected),
        "unique_across_nodes": len({r["identity"] for r in selected}),
    }))
