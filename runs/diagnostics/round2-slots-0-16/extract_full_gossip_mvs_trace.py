#!/usr/bin/env python3
"""Extract complete MVS/count events and a transition/marker index from a parsed trace."""

import argparse
from pathlib import Path


def indexed_header(line):
    return (
        " StateTransition " in line
        or " Sync " in line
        or " RangeBegin " in line
        or " RangeEnd " in line
        or " RegionBegin " in line
        or " RegionEnd " in line
    )


def relevant_event(event):
    return any(
        "container/multi-value-slice" in line or "ActiveValidatorCount" in line
        for line in event
    )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("parsed_trace", type=Path)
    parser.add_argument("events_output", type=Path)
    parser.add_argument("index_output", type=Path)
    args = parser.parse_args()

    event = []
    event_start = 0
    selected = 0
    indexed = 0
    with (
        args.parsed_trace.open(errors="strict") as source,
        args.events_output.open("w") as events_output,
        args.index_output.open("w") as index_output,
    ):
        index_output.write("parsed_line\theader\n")

        def flush(end_line):
            nonlocal selected
            if not event or not relevant_event(event):
                return
            selected += 1
            events_output.write(f"# parsed_lines={event_start}-{end_line}\n")
            events_output.writelines(event)
            if event[-1].strip():
                events_output.write("\n")

        last_line = 0
        for line_number, line in enumerate(source, 1):
            last_line = line_number
            if line.startswith("M="):
                flush(line_number - 1)
                event = [line]
                event_start = line_number
                if indexed_header(line):
                    indexed += 1
                    index_output.write(f"{line_number}\t{line.rstrip()}\n")
            elif event:
                event.append(line)
        flush(last_line)
        events_output.write(f"# selected_events={selected}\n")
        index_output.write(f"# indexed_headers={indexed}\n")


if __name__ == "__main__":
    main()
