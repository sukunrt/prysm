#!/usr/bin/env python3
"""Extract complete parsed-trace events for the bounded writer RPC window."""

import argparse
import re
from pathlib import Path


TIME_RE = re.compile(r"\bTime=(\d+)")
TARGET_G_RE = re.compile(r"\b(?:GoID|G)=(?:4912|4914|15522)(?=\s|$)")
SYNC_RE = re.compile(r"\bSync\b.*\bN=(?:3|4)(?=\s|$)")
P_RE = re.compile(r"\bP=(-?\d+)")


def keep_event(first_line, start, end):
    match = TIME_RE.search(first_line)
    in_window = match is not None and start <= int(match.group(1)) <= end
    target_g = TARGET_G_RE.search(first_line) is not None
    reasons = []
    if in_window:
        reasons.append("probe14_window")
    if target_g:
        reasons.append("target_goroutine")
    return reasons


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("parsed_trace", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--start", type=int, default=277057550000000)
    parser.add_argument("--end", type=int, default=277058550000000)
    parser.add_argument(
        "--sample-start",
        type=int,
        help="lower bound for compact ActiveValidatorCount preemption samples",
    )
    parser.add_argument(
        "--compact",
        action="store_true",
        help="keep target goroutines only inside the time window, Sync N3/N4, "
        "and one nearby ActiveValidatorCount preemption per P",
    )
    args = parser.parse_args()
    sample_start = args.start if args.sample_start is None else args.sample_start

    start_line = 0
    event = []
    selected = 0
    sampled_count_preemption_ps = set()
    with args.parsed_trace.open(errors="strict") as source, args.output.open("w") as output:
        def flush(end_line):
            nonlocal selected
            if not event:
                return
            if args.compact:
                first = event[0]
                match = TIME_RE.search(first)
                in_window = (
                    match is not None
                    and args.start <= int(match.group(1)) <= args.end
                )
                reasons = []
                if in_window and TARGET_G_RE.search(first):
                    reasons.append("target_goroutine_window")
                if SYNC_RE.search(first):
                    reasons.append("wall_sync_anchor")
                p_match = P_RE.search(first)
                if (
                    in_window
                    and int(match.group(1)) >= sample_start
                    and p_match is not None
                    and int(p_match.group(1)) >= 0
                    and int(p_match.group(1)) not in sampled_count_preemption_ps
                    and 'Running->Runnable Reason="preempted"' in first
                    and any("ActiveValidatorCount" in line for line in event)
                ):
                    sampled_count_preemption_ps.add(int(p_match.group(1)))
                    reasons.append("representative_count_preemption")
            else:
                reasons = keep_event(event[0], args.start, args.end)
            if not reasons:
                return
            selected += 1
            output.write(
                f"# parsed_lines={start_line}-{end_line} reasons={','.join(reasons)}\n"
            )
            output.writelines(event)
            if event[-1].strip():
                output.write("\n")

        for line_number, line in enumerate(source, 1):
            if line.startswith("M="):
                flush(line_number - 1)
                event = [line]
                start_line = line_number
            elif event:
                event.append(line)
        flush(line_number)
        output.write(f"# selected_events={selected}\n")


if __name__ == "__main__":
    main()
