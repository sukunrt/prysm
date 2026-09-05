#!/usr/bin/env python3
"""Validate every joined row in the traced finalizer-unblocked Count cohort."""

import argparse
import csv
import re
from collections import Counter
from pathlib import Path


G_RE = re.compile(r"\bG=(-?\d+)")
GOID_RE = re.compile(r"\bGoID=(\d+)")
TIME_RE = re.compile(r"\bTime=(\d+)")


def load_events(path):
    events = []
    provenance = ""
    event = []

    def flush():
        if not event:
            return
        first = event[0]
        time_match = TIME_RE.search(first)
        if time_match is None:
            return
        g_match = G_RE.search(first)
        goid_match = GOID_RE.search(first)
        events.append(
            {
                "time": int(time_match.group(1)),
                "g": None if g_match is None else int(g_match.group(1)),
                "goid": None if goid_match is None else int(goid_match.group(1)),
                "provenance": provenance,
                "text": "".join(event),
            }
        )

    for line in path.read_text().splitlines(keepends=True):
        if line.startswith("# parsed_lines="):
            flush()
            event = []
            provenance = line.removeprefix("# parsed_lines=").strip()
        elif line.startswith("M="):
            flush()
            event = [line]
        elif event:
            event.append(line)
    flush()
    return events


def one(events, timestamp, *, g=None, goid=None, needles=()):
    matches = [
        event
        for event in events
        if event["time"] == timestamp
        and (g is None or event["g"] == g)
        and (goid is None or event["goid"] == goid)
        and all(needle in event["text"] for needle in needles)
    ]
    if len(matches) != 1:
        raise ValueError(
            f"expected one event time={timestamp} g={g} goid={goid} "
            f"needles={needles}, found {len(matches)}"
        )
    return matches[0]


def check_ref(row, column, event):
    expected = row[column]
    actual = event["provenance"]
    if "-" not in expected and actual == f"{expected}-{expected}":
        actual = expected
    if expected != actual:
        raise ValueError(
            f"G{row['go']} {column}: TSV={row[column]} trace={event['provenance']}"
        )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("members", type=Path)
    parser.add_argument("transitions", type=Path)
    args = parser.parse_args()

    with args.members.open(newline="") as source:
        rows = list(csv.DictReader(source, delimiter="\t"))
    events = load_events(args.transitions)
    if len(rows) != 101:
        raise ValueError(f"expected 101 members, found {len(rows)}")
    if len({int(row["go"]) for row in rows}) != 101:
        raise ValueError("cohort Go IDs are not unique")
    if Counter(row["kind"] for row in rows) != Counter({"Len": 98, "At": 3}):
        raise ValueError("expected 98 Len readers and 3 At readers")

    for row in rows:
        goid = int(row["go"])
        kind = row["kind"]
        cp_park = one(
            events, int(row["cpPark"]), g=goid, goid=goid,
            needles=('Running->Waiting Reason="chan receive"', "async.(*Lock).Lock"),
        )
        cp_wake = one(
            events, int(row["cpWake"]), g=int(row["cpWaker"]), goid=goid,
            needles=("Waiting->Runnable", "async.(*Lock).Unlock"),
        )
        park = one(
            events, int(row["park"]), g=goid, goid=goid,
            needles=(f"Running->Waiting Reason=\"sync\"", f").{kind} @", "ActiveValidatorCount"),
        )
        wake = one(
            events, int(row["wake"]), g=6, goid=goid,
            needles=("Waiting->Runnable", ").Detach @", "finalizerCleanup"),
        )
        run = one(events, int(row["run"]), goid=goid, needles=("Runnable->Running",))
        blst_park = one(
            events, int(row["blstPark"]), g=goid, goid=goid,
            needles=('Running->Waiting Reason="chan receive"', "P1Aggregate"),
        )
        blst_wake = one(
            events, int(row["blstWake"]), g=int(row["blstChild"]), goid=goid,
            needles=("Waiting->Runnable", "P1Aggregate"),
        )
        batch_park = one(
            events, int(row["batchPark"]), g=goid, goid=goid,
            needles=('Running->Waiting Reason="chan receive"', "validateWithBatchVerifier"),
        )
        batch_wake = one(
            events, int(row["batchWake"]), goid=goid,
            needles=("Waiting->Runnable", "verifyBatch"),
        )
        exit_event = one(
            events, int(row["exit"]), g=goid, goid=goid,
            needles=("Running->NotExist",),
        )
        for column, event in (
            ("cpParkRef", cp_park), ("cpWakeRef", cp_wake), ("parkRef", park),
            ("wakeRef", wake), ("runRef", run), ("blstParkRef", blst_park),
            ("blstWakeRef", blst_wake), ("batchParkRef", batch_park),
            ("batchWakeRef", batch_wake),
        ):
            check_ref(row, column, event)
        if int(row["readyNs"]) != int(row["run"]) - int(row["wake"]):
            raise ValueError(f"G{goid} ready duration mismatch")
        first_run = min(
            event["time"]
            for event in events
            if event["goid"] == goid
            and event["time"] >= int(row["wake"])
            and "Runnable->Running" in event["text"]
        )
        if first_run != int(row["run"]):
            raise ValueError(f"G{goid} listed run is not its first run after wake")
        first_wait = min(
            event["time"]
            for event in events
            if event["g"] == goid
            and event["time"] >= int(row["run"])
            and "Running->Waiting" in event["text"]
        )
        if first_wait != int(row["blstPark"]):
            raise ValueError(f"G{goid} BLST park is not its first wait after running")
        if exit_event["time"] <= batch_wake["time"]:
            raise ValueError(f"G{goid} exits before its batch wake")

    wakes = sorted(int(row["wake"]) for row in rows)
    parks = [int(row["wake"]) - int(row["park"]) for row in rows]
    probe_invoke = 277057627472301
    exited_before_probe = sum(int(row["exit"]) < probe_invoke for row in rows)
    batch_waiting_at_probe = sum(
        int(row["batchPark"]) < probe_invoke < int(row["batchWake"]) for row in rows
    )
    if (exited_before_probe, batch_waiting_at_probe) != (10, 91):
        raise ValueError(
            f"probe join mismatch: exited={exited_before_probe} batch_waiting={batch_waiting_at_probe}"
        )
    print(
        "validated=101 len=98 at=3 "
        f"wake_start={wakes[0]} wake_end={wakes[-1]} wake_span_ns={wakes[-1]-wakes[0]} "
        f"park_min_ns={min(parks)} park_max_ns={max(parks)} "
        f"postwake_len_visits={98 * 120000} "
        f"exited_before_probe={exited_before_probe} batch_waiting_at_probe={batch_waiting_at_probe}"
    )


if __name__ == "__main__":
    main()
