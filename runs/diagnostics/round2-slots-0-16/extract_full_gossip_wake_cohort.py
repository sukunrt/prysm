#!/usr/bin/env python3
"""Extract complete trace histories for the finalizer-unblocked Count cohort."""

import argparse
import re
from pathlib import Path


G_RE = re.compile(r"\bG=(-?\d+)")
GOID_RE = re.compile(r"\bGoID=(\d+)")
TIME_RE = re.compile(r"\bTime=(\d+)")


def header_fields(line):
    g_match = G_RE.search(line)
    goid_match = GOID_RE.search(line)
    time_match = TIME_RE.search(line)
    return (
        None if g_match is None else int(g_match.group(1)),
        None if goid_match is None else int(goid_match.group(1)),
        None if time_match is None else int(time_match.group(1)),
    )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("parsed_trace", type=Path)
    parser.add_argument("events_output", type=Path)
    parser.add_argument("targets_output", type=Path)
    parser.add_argument("--wake-start", type=int, default=277054662602688)
    parser.add_argument("--wake-end", type=int, default=277054662630784)
    parser.add_argument("--expected-targets", type=int, default=101)
    args = parser.parse_args()

    targets = set()
    with args.parsed_trace.open(errors="strict") as source:
        for line in source:
            if not line.startswith("M=") or " Waiting->Runnable " not in line:
                continue
            g, goid, timestamp = header_fields(line)
            if g == 6 and goid is not None and args.wake_start <= timestamp <= args.wake_end:
                targets.add(goid)
    if len(targets) != args.expected_targets:
        raise ValueError(f"expected {args.expected_targets} wake targets, found {len(targets)}")

    selected_ids = targets | {6, 348, 4591}
    args.targets_output.write_text(
        "goid\trole\n"
        + "".join(
            f"{goid}\t{'finalizer' if goid == 6 else 'early_copy' if goid in (348, 4591) else 'count_reader'}\n"
            for goid in sorted(selected_ids)
        )
    )

    event = []
    event_start = 0
    selected_events = 0
    with args.parsed_trace.open(errors="strict") as source, args.events_output.open("w") as output:
        def flush(end_line):
            nonlocal selected_events
            if not event:
                return
            g, goid, _ = header_fields(event[0])
            if g not in selected_ids and goid not in selected_ids:
                return
            selected_events += 1
            output.write(f"# parsed_lines={event_start}-{end_line}\n")
            output.writelines(event)
            if event[-1].strip():
                output.write("\n")

        last_line = 0
        for line_number, line in enumerate(source, 1):
            last_line = line_number
            if line.startswith("M="):
                flush(line_number - 1)
                event = [line]
                event_start = line_number
            elif event:
                event.append(line)
        flush(last_line)
        output.write(
            f"# target_count={len(targets)} actor_count=3 selected_events={selected_events}\n"
        )


if __name__ == "__main__":
    main()
