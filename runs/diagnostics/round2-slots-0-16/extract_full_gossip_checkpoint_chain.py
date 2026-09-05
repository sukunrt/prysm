#!/usr/bin/env python3
"""Follow the recorded getAttPreState key-owner chain backward from one waiter."""

import argparse
import csv
import re
from pathlib import Path


G_RE = re.compile(r"\bG=(-?\d+)")
GOID_RE = re.compile(r"\bGoID=(\d+)")
TIME_RE = re.compile(r"\bTime=(\d+)")


def fields(line):
    g = G_RE.search(line)
    goid = GOID_RE.search(line)
    timestamp = TIME_RE.search(line)
    return (
        None if g is None else int(g.group(1)),
        None if goid is None else int(goid.group(1)),
        None if timestamp is None else int(timestamp.group(1)),
    )


def parsed_events(path):
    event = []
    start = 0
    for line_number, line in enumerate(path.open(errors="strict"), 1):
        if line.startswith("M="):
            if event:
                yield start, line_number - 1, event
            start = line_number
            event = [line]
        elif event:
            event.append(line)
    if event:
        yield start, line_number, event


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("parsed_trace", type=Path)
    parser.add_argument("chain_output", type=Path)
    parser.add_argument("checkpoint_events_output", type=Path)
    parser.add_argument("full_events_output", type=Path)
    parser.add_argument("--target-goid", type=int, default=4970)
    parser.add_argument("--target-park", type=int, default=277054191954560)
    args = parser.parse_args()

    parks = []
    wakes = []
    for start, end, event in parsed_events(args.parsed_trace):
        text = "".join(event)
        if "getAttPreState" not in text:
            continue
        g, goid, timestamp = fields(event[0])
        record = {
            "g": g, "goid": goid, "time": timestamp,
            "ref": str(start) if start == end else f"{start}-{end}", "event": event,
        }
        if (
            g == goid
            and 'Running->Waiting Reason="chan receive"' in event[0]
            and "async.(*Lock).Lock" in text
        ):
            parks.append(record)
        if (
            "Waiting->Runnable" in event[0]
            and "async.(*Lock).Unlock" in text
        ):
            wakes.append(record)

    target_parks = [
        park for park in parks
        if park["g"] == args.target_goid and park["time"] == args.target_park
    ]
    if len(target_parks) != 1:
        raise ValueError(f"expected exact target park, found {len(target_parks)}")

    chain = []
    waiter = args.target_goid
    park = target_parks[0]
    upper = None
    while True:
        candidates = [
            wake for wake in wakes
            if wake["goid"] == waiter
            and wake["time"] > park["time"]
            and (upper is None or wake["time"] < upper)
        ]
        if not candidates:
            raise ValueError(f"no recorded checkpoint grant for G{waiter} after {park['time']}")
        wake = max(candidates, key=lambda record: record["time"])
        chain.append({"waiter": waiter, "park": park, "wake": wake})
        if wake["time"] <= args.target_park:
            break
        upper = wake["time"]
        waiter = wake["g"]
        prior_wakes = [candidate for candidate in wakes if candidate["goid"] == waiter and candidate["time"] < upper]
        if not prior_wakes:
            break
        incoming = max(prior_wakes, key=lambda record: record["time"])
        prior_parks = [
            candidate for candidate in parks
            if candidate["g"] == waiter and candidate["time"] < incoming["time"]
        ]
        if not prior_parks:
            break
        park = max(prior_parks, key=lambda record: record["time"])

    selected = {record["waiter"] for record in chain} | {record["wake"]["g"] for record in chain}
    first_runs = {}
    selected_events = 0
    with args.full_events_output.open("w") as output:
        for start, end, event in parsed_events(args.parsed_trace):
            g, goid, timestamp = fields(event[0])
            if g not in selected and goid not in selected:
                continue
            selected_events += 1
            if (
                goid in selected
                and "Runnable->Running" in event[0]
                and goid not in first_runs
            ):
                # Filled below per grant; retaining all candidates here avoids
                # assuming that the first run in the entire trace follows this call.
                pass
            output.write(f"# parsed_lines={start}-{end}\n")
            output.writelines(event)
            if event[-1].strip():
                output.write("\n")
        output.write(f"# selected_goids={len(selected)} selected_events={selected_events}\n")

    # Resolve the first scheduled execution after each recorded grant from the
    # complete selected histories.
    run_candidates = {goid: [] for goid in selected}
    for start, end, event in parsed_events(args.parsed_trace):
        _, goid, timestamp = fields(event[0])
        if goid in selected and "Runnable->Running" in event[0]:
            run_candidates[goid].append((timestamp, str(start) if start == end else f"{start}-{end}"))
    for record in chain:
        after = [candidate for candidate in run_candidates[record["waiter"]] if candidate[0] >= record["wake"]["time"]]
        record["run"], record["run_ref"] = min(after)

    with args.checkpoint_events_output.open("w") as output:
        for record in reversed(chain):
            for label in ("park", "wake"):
                event = record[label]
                output.write(
                    f"# waiter={record['waiter']} phase={label} parsed_lines={event['ref']}\n"
                )
                output.writelines(event["event"])
                if event["event"][-1].strip():
                    output.write("\n")

    with args.chain_output.open("w", newline="") as output:
        writer = csv.writer(output, delimiter="\t", lineterminator="\n")
        writer.writerow([
            "waiter", "park", "grant", "incoming_waker", "first_run", "ready_ns",
            "outgoing_release", "run_to_outgoing_release_ns", "park_ref", "grant_ref", "run_ref",
        ])
        for index, record in enumerate(chain):
            outgoing = "" if index == 0 else chain[index - 1]["wake"]["time"]
            run_to_outgoing = "" if outgoing == "" else outgoing - record["run"]
            writer.writerow([
                record["waiter"], record["park"]["time"], record["wake"]["time"],
                record["wake"]["g"], record["run"], record["run"] - record["wake"]["time"],
                outgoing, run_to_outgoing, record["park"]["ref"], record["wake"]["ref"],
                record["run_ref"],
            ])


if __name__ == "__main__":
    main()
