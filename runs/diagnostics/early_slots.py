#!/usr/bin/env python3
"""Summarize slots 0--3 without confusing embedded genesis fields for timestamps."""

import argparse
import datetime as dt
import json
import re
from collections import Counter, defaultdict
from pathlib import Path

ANSI = re.compile(r"\x1b\[[0-9;]*m")
PREFIX = re.compile(r"^(?:(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?Z) |\[(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d(?:\.\d+)?)\])")
FIELD = re.compile(r"(?:^| )([A-Za-z][A-Za-z0-9]*)=([^ ]+)")
ENGINE = re.compile(r"REQUEST #\d+:.* method=(engine_[A-Za-z0-9]+)")
RELEVANT_ENGINE = re.compile(r"engine_(?:forkchoiceUpdated|getPayload|newPayload)")


def instant(value):
    return dt.datetime.fromisoformat(value.replace(" ", "T").removesuffix("Z") + "+00:00")


def fields(line):
    return dict(FIELD.findall(line))


def percentile(values, fraction):
    if not values:
        return None
    values = sorted(values)
    return values[round((len(values) - 1) * fraction)]


def timing(values):
    return {"min": min(values), "p50": percentile(values, .5),
            "p95": percentile(values, .95), "max": max(values)} if values else None


def duration_ms(value):
    match = re.fullmatch(r"([0-9.]+)(ms|s)", value or "")
    if not match:
        return None
    amount = float(match.group(1))
    return round(amount * (1000 if match.group(2) == "s" else 1), 3)


def read_lines(path, needles):
    with path.open(errors="replace") as stream:
        for number, raw in enumerate(stream, 1):
            if not any(needle in raw for needle in needles):
                continue
            line = ANSI.sub("", raw.rstrip())
            match = PREFIX.match(line)
            if match:  # Deliberately only the container's leading bracket/prefix.
                yield number, instant(match.group(1) or match.group(2)), line


def analyze_node(node):
    result = {"genesis": None, "slots": {str(slot): {} for slot in range(4)}}
    votes = defaultdict(list)
    ffg = defaultdict(list)
    known_genesis = {"round1": "2026-09-05T00:00:00Z", "round2": "2026-09-05T01:30:00Z"}
    if node.parent.name not in known_genesis:
        raise ValueError(f"unknown genesis for run directory {node.parent}")
    genesis = instant(known_genesis[node.parent.name])
    result["genesis"] = known_genesis[node.parent.name]
    for _, timestamp, line in read_lines(node / "beacon.log", ("PTC vote", "Goldfish vote", "FFG vote")):
        data = fields(line)
        if "Goldfish vote" in line and data.get("voteSlot", "x").isdigit():
            slot = int(data["voteSlot"])
            if 0 <= slot < 4:
                votes[slot].append(data)
        elif "FFG vote" in line and data.get("attSlot", "x").isdigit():
            slot = int(data["attSlot"])
            if 0 <= slot < 4:
                ffg[slot].append(data)
    submissions = defaultdict(list)
    failures = defaultdict(Counter)
    validator = node / "validator.log"
    if validator.exists():
        for _, timestamp, line in read_lines(validator, ("Submitted new attestations", "ERROR", "WARN")):
            data = fields(line)
            if data.get("slot", "x").isdigit() and 0 <= int(data["slot"]) < 4:
                slot = int(data["slot"])
                if "Submitted new attestations" in line and "aggregate" not in line:
                    keys = re.search(r" pubkeys=\[([^]]*)]", line)
                    submissions[slot].append({
                        "validators": len(keys.group(1).split()) if keys else 0,
                        "submitted_ms": duration_ms(data.get("submittedSinceSlotStart")),
                        "emitted_ms": round((timestamp - genesis).total_seconds() * 1000 - slot * 12000, 3) if genesis else None,
                    })
                elif re.search(r"(?i)timeout|timed out|failed|could not|deadlineexceeded", line):
                    message = line.split(": ", 1)[-1].split(" error=", 1)[0]
                    failures[slot][message] += 1

    engine = defaultdict(Counter)
    snooper = node / "snooper-engine.log"
    if genesis and snooper.exists():
        for _, timestamp, line in read_lines(snooper, ("REQUEST #",)):
            if timestamp > genesis + dt.timedelta(seconds=48):
                break
            match = ENGINE.search(line)
            elapsed = (timestamp - genesis).total_seconds()
            slot = int(elapsed // 12)
            if match and 0 <= slot < 4 and RELEVANT_ENGINE.match(match.group(1)):
                engine[slot][match.group(1)] += 1

    for slot in range(4):
        gold = votes[slot]
        sub = submissions[slot]
        result["slots"][str(slot)] = {
            "goldfish": {"votes": len(gold),
                "seats": sum(int(x.get("seats", 1)) for x in gold),
                "arrived_since_slot_start_ms": timing([int(x["arrivedMs"]) for x in gold if x.get("arrivedMs", "").isdigit()]),
                "decided_since_slot_start_ms": timing([int(x["decidedMs"]) for x in gold if x.get("decidedMs", "").isdigit()]),
                "outcomes": dict(Counter(x.get("outcome", "unknown") for x in gold)),
                "drop_reasons": dict(Counter(x.get("reason", "unspecified") for x in gold if x.get("outcome") == "dropped"))},
            "ffg": {"votes": len(ffg[slot]), "seats": sum(int(x.get("seats", 1)) for x in ffg[slot]),
                    "arrived_since_slot_start_ms": timing([int(x["arrivedMs"]) for x in ffg[slot] if x.get("arrivedMs", "").isdigit()])},
            "validator": {"batches": len(sub), "reported_successful_submission_records": sum(x["validators"] for x in sub),
                          "submitted_ms": timing([x["submitted_ms"] for x in sub if x["submitted_ms"] is not None]),
                          "emitted_ms": timing([x["emitted_ms"] for x in sub if x["emitted_ms"] is not None]),
                          "failures": dict(failures[slot])},
            "engine_requests": dict(engine[slot]),
        }
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("rounds", nargs="*", type=Path,
                        default=[Path("runs/round1"), Path("runs/round2")])
    args = parser.parse_args()
    output = {}
    for round_path in args.rounds:
        nodes = sorted(p for p in round_path.glob("prysm-geth-*") if p.is_dir())
        output[round_path.name] = {node.name: analyze_node(node) for node in nodes}
    print(json.dumps(output, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
