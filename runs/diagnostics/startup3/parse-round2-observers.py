#!/usr/bin/env python3
import argparse
import datetime
import json
import re
import statistics
from collections import Counter, defaultdict
from pathlib import Path

ANSI = re.compile(r"\x1b\[[0-9;]*m")
TIME = re.compile(r"^(\S+) ")
INNER_TIME = re.compile(r"\[(2026-[^]]+)\]")
GENESIS = datetime.datetime.fromisoformat("2026-09-05T01:30:00+00:00").timestamp()


def field(line, name):
    match = re.search(r"(?:^|\s)" + re.escape(name) + r"=([^ ]+)", line)
    return match.group(1) if match else None


def parse(path):
    blocks = []
    ledger = defaultdict(lambda: {"outcomes": Counter(), "count": 0, "max_lag_ms": 0, "last_decided_ms": 0})
    ffg = defaultdict(list)
    for number, raw in enumerate(open(path, errors="replace"), 1):
        line = ANSI.sub("", raw)
        timestamp = TIME.match(line)
        if "Synced new block " in line:
            slot = field(line, "slot")
            if slot is not None and int(slot) <= 20:
                blocks.append({"slot": int(slot), "time": timestamp.group(1), "line": number, "root": field(line, "block")})
        if "Goldfish vote " in line:
            slot = field(line, "voteSlot")
            arrived = field(line, "arrivedMs")
            decided = field(line, "decidedMs")
            outcome = field(line, "outcome")
            if slot is None or arrived is None or decided is None or int(slot) > 20:
                continue
            item = ledger[int(slot)]
            item["count"] += 1
            item["outcomes"][outcome or "unknown"] += 1
            item["max_lag_ms"] = max(item["max_lag_ms"], int(decided) - int(arrived))
            item["last_decided_ms"] = max(item["last_decided_ms"], int(decided))
        if "sync: FFG vote " in line and field(line, "outcome") == "gossip":
            slot = field(line, "attSlot")
            arrived = field(line, "arrivedMs")
            target_round = field(line, "targetRound")
            inner = INNER_TIME.search(line)
            if slot is None or arrived is None or target_round is None or inner is None or int(slot) > 20:
                continue
            completed = datetime.datetime.fromisoformat(timestamp.group(1).replace("Z", "+00:00")).timestamp()
            inner_completed = datetime.datetime.fromisoformat(inner.group(1).replace(" ", "T") + "+00:00").timestamp()
            entered = GENESIS + int(slot) * 12 + int(arrived) / 1000
            ffg[int(slot)].append({
                "line": number, "att_slot": int(slot), "target_round": int(target_round), "arrived_ms": int(arrived),
                "entered_unix": entered, "completed_unix": completed,
                "inner_completed_unix": inner_completed,
                "lag_ms": (completed - entered) * 1000,
                "wall_slot": int((completed - GENESIS) // 12),
            })
    ffg_summary = {}
    for slot, rows in sorted(ffg.items()):
        lags = sorted(row["lag_ms"] for row in rows)
        p95 = lags[max(0, (95 * len(lags) + 99) // 100 - 1)]
        slot_end = GENESIS + (slot + 1) * 12
        ffg_summary[slot] = {
            "count": len(rows),
            "entered_by_slot_end": sum(row["entered_unix"] < slot_end for row in rows),
            "completed_by_slot_end": sum(row["completed_unix"] < slot_end for row in rows),
            "lag_ms": {"median": statistics.median(lags), "p95": p95, "max": max(lags)},
            "top5": sorted(rows, key=lambda row: row["lag_ms"], reverse=True)[:5],
        }
        if ffg_summary[slot]["completed_by_slot_end"] > ffg_summary[slot]["entered_by_slot_end"]:
            raise ValueError("completion cohort exceeds entry cohort for slot {}".format(slot))
    round_one = [row for rows in ffg.values() for row in rows if row["target_round"] == 1]
    round_zero = [row for rows in ffg.values() for row in rows if row["target_round"] == 0]
    return {
        "blocks": blocks,
        "goldfish": {slot: {**values, "outcomes": dict(values["outcomes"])} for slot, values in sorted(ledger.items())},
        "ffg": ffg_summary,
        "first_target_round_1": min(round_one, key=lambda row: row["completed_unix"]) if round_one else None,
        "last_target_round_0": max(round_zero, key=lambda row: row["completed_unix"]) if round_zero else None,
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("logs", nargs="+", type=Path)
    args = parser.parse_args()
    print(json.dumps({str(path): parse(path) for path in args.logs}, indent=2))


if __name__ == "__main__":
    main()
