#!/usr/bin/env python3
"""Extract checkpoint events and transition headers for a supplied Go-ID cohort."""

import argparse
import re
from pathlib import Path


G_RE = re.compile(r"\bG=(-?\d+)")
GOID_RE = re.compile(r"\bGoID=(\d+)")


def selected_header(line, goids):
    g_match = G_RE.search(line)
    goid_match = GOID_RE.search(line)
    execution_g = None if g_match is None else int(g_match.group(1))
    transitioned_g = None if goid_match is None else int(goid_match.group(1))
    return execution_g in goids or transitioned_g in goids


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("parsed_trace", type=Path)
    parser.add_argument("goids", type=Path)
    parser.add_argument("checkpoint_events_output", type=Path)
    parser.add_argument("transition_index_output", type=Path)
    parser.add_argument("--full-events-output", type=Path)
    parser.add_argument("--expected-goids", type=int, default=134)
    args = parser.parse_args()

    goids = {int(line) for line in args.goids.read_text().splitlines() if line}
    if len(goids) != args.expected_goids:
        raise ValueError(f"expected {args.expected_goids} unique Go IDs, found {len(goids)}")

    event = []
    event_start = 0
    checkpoint_events = 0
    transition_headers = 0
    selected_events = 0
    with (
        args.parsed_trace.open(errors="strict") as source,
        args.checkpoint_events_output.open("w") as checkpoint_output,
        args.transition_index_output.open("w") as transition_output,
    ):
        full_output = None if args.full_events_output is None else args.full_events_output.open("w")
        transition_output.write("parsed_line\theader\n")

        def flush(end_line):
            nonlocal checkpoint_events, transition_headers, selected_events
            if not event or not selected_header(event[0], goids):
                return
            selected_events += 1
            if full_output is not None:
                full_output.write(f"# parsed_lines={event_start}-{end_line}\n")
                full_output.writelines(event)
                if event[-1].strip():
                    full_output.write("\n")
            if " StateTransition " in event[0]:
                transition_headers += 1
                transition_output.write(f"{event_start}\t{event[0].rstrip()}\n")
            if any("getAttPreState" in line for line in event):
                checkpoint_events += 1
                checkpoint_output.write(f"# parsed_lines={event_start}-{end_line}\n")
                checkpoint_output.writelines(event)
                if event[-1].strip():
                    checkpoint_output.write("\n")

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
        checkpoint_output.write(
            f"# goids={len(goids)} checkpoint_events={checkpoint_events}\n"
        )
        transition_output.write(
            f"# goids={len(goids)} transition_headers={transition_headers}\n"
        )
        if full_output is not None:
            full_output.write(f"# goids={len(goids)} selected_events={selected_events}\n")
            full_output.close()


if __name__ == "__main__":
    main()
