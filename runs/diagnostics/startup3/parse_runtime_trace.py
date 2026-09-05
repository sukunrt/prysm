#!/usr/bin/env python3
"""Extract startup.engine scheduling intervals from `go tool trace -d=parsed`."""

import argparse
import json
import re
import subprocess
import sys


EVENT_RE = re.compile(
    r"\bG=(?P<emitter>-?\d+)\s+StateTransition\s+Time=(?P<time>\d+)\s+"
    r"GoID=(?P<goid>\d+)\s+(?P<old>\w+)->(?P<new>\w+)\s+Reason=\"(?P<reason>[^\"]*)\""
)
LOG_RE = re.compile(
    r"\bG=(?P<goid>-?\d+)\s+Log\s+Time=(?P<time>\d+).*"
    r"Category=\"startup\.engine\"\s+Message=\"(?P<message>[^\"]*)\""
)
MESSAGE_RE = re.compile(
    r"phase=(?P<phase>\S+)\s+slot=(?P<slot>\d+)\s+"
    r"payload_id=(?P<payload>\S+)\s+wall_unix_nano=(?P<wall>\d+)"
)


def input_lines(path):
    if path == "-":
        yield from sys.stdin
        return
    if path.endswith(".trace") or path.endswith(".out"):
        proc = subprocess.Popen(
            ["go", "tool", "trace", "-d=parsed", path],
            stdout=subprocess.PIPE,
            text=True,
        )
        assert proc.stdout is not None
        yield from proc.stdout
        if proc.wait() != 0:
            raise SystemExit(proc.returncode)
        return
    with open(path, encoding="utf-8") as source:
        yield from source


def latest_before(events, timestamp, old, new):
    for event in reversed(events):
        if event["time"] > timestamp:
            continue
        if event["old"] == old and event["new"] == new:
            return event
    return None


def state_timeline(events, start_event, end_time):
    """Return every state interval after start_event, including preemptions."""
    state = start_event["new"]
    started = start_event["time"]
    intervals = []
    for event in events:
        if event["time"] <= started or event["time"] > end_time:
            continue
        if event["old"] != state:
            continue
        intervals.append(
            {
                "state": state,
                "start_trace_ns": started,
                "end_trace_ns": event["time"],
                "duration_ns": event["time"] - started,
                "next_state": event["new"],
                "transition_reason": event["reason"],
            }
        )
        state = event["new"]
        started = event["time"]
    intervals.append(
        {
            "state": state,
            "start_trace_ns": started,
            "end_trace_ns": end_time,
            "duration_ns": end_time - started,
            "next_state": "Log",
            "transition_reason": "",
        }
    )
    return intervals


def first_after(events, timestamp, old, new, end_time):
    for event in events:
        if event["time"] < timestamp or event["time"] > end_time:
            continue
        if event["old"] == old and event["new"] == new:
            return event
    return None


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("trace", help="runtime trace or parsed trace text; use - for stdin")
    parser.add_argument("--indent", action="store_true")
    parser.add_argument(
        "--goid",
        action="append",
        type=int,
        default=[],
        help="retain only this goroutine (repeatable); strongly recommended for large traces",
    )
    args = parser.parse_args()
    selected_goids = set(args.goid)

    transitions = {}
    logs = []
    transition_stack = None
    last_transition = None
    for line in input_lines(args.trace):
        event_match = EVENT_RE.search(line) if " StateTransition " in line else None
        if event_match:
            event = event_match.groupdict()
            event["time"] = int(event["time"])
            event["goid"] = int(event["goid"])
            if selected_goids and event["goid"] not in selected_goids:
                last_transition = None
                transition_stack = None
                continue
            event["transition_stack"] = []
            transitions.setdefault(event["goid"], []).append(event)
            last_transition = event
            transition_stack = None
            continue
        if line.startswith("TransitionStack="):
            transition_stack = last_transition
            continue
        if transition_stack is not None and line.startswith("\t"):
            transition_stack["transition_stack"].append(line.strip())
            continue
        if not line.strip() or not line.startswith("\t"):
            transition_stack = None
        if " Category=\"startup.engine\" " not in line:
            continue
        log_match = LOG_RE.search(line)
        if not log_match:
            continue
        if selected_goids and int(log_match.group("goid")) not in selected_goids:
            continue
        message_match = MESSAGE_RE.fullmatch(log_match.group("message"))
        if not message_match:
            continue
        log = message_match.groupdict()
        log["goid"] = int(log_match.group("goid"))
        log["trace_time_ns"] = int(log_match.group("time"))
        log["wall_unix_nano"] = int(log.pop("wall"))
        log["slot"] = int(log["slot"])
        logs.append(log)

    for log in logs:
        events = transitions.get(log["goid"], [])
        runnable = latest_before(events, log["trace_time_ns"], "Waiting", "Runnable")
        waiting = latest_before(events, log["trace_time_ns"], "Running", "Waiting")
        if runnable:
            log["last_unblock_trace_ns"] = runnable["time"]
            log["unblock_reason"] = runnable["reason"]
            log["unblock_to_log_ns"] = log["trace_time_ns"] - runnable["time"]
            intervals = state_timeline(events, runnable, log["trace_time_ns"])
            log["state_intervals"] = intervals
            log["runnable_total_ns"] = sum(
                interval["duration_ns"]
                for interval in intervals
                if interval["state"] == "Runnable"
            )
            log["running_total_ns"] = sum(
                interval["duration_ns"]
                for interval in intervals
                if interval["state"] == "Running"
            )
        if waiting:
            log["last_wait_trace_ns"] = waiting["time"]
            log["wait_reason"] = waiting["reason"]
            log["wait_transition_stack"] = waiting["transition_stack"]
        network_wait = None
        for event in reversed(events):
            if event["time"] > log["trace_time_ns"]:
                continue
            if event["old"] == "Running" and event["new"] == "Waiting" and (
                event["reason"] == "network" or
                any("internal/poll" in frame for frame in event["transition_stack"])
            ):
                network_wait = event
                break
        if network_wait:
            network_unblock = first_after(
                events, network_wait["time"], "Waiting", "Runnable", log["trace_time_ns"]
            )
            log["network_wait_trace_ns"] = network_wait["time"]
            log["network_wait_reason"] = network_wait["reason"]
            log["network_wait_transition_stack"] = network_wait["transition_stack"]
            if network_unblock:
                log["network_unblock_trace_ns"] = network_unblock["time"]
                log["network_wait_duration_ns"] = (
                    network_unblock["time"] - network_wait["time"]
                )
                intervals = state_timeline(events, network_unblock, log["trace_time_ns"])
                log["post_network_state_intervals"] = intervals
                log["post_network_runnable_total_ns"] = sum(
                    interval["duration_ns"]
                    for interval in intervals
                    if interval["state"] == "Runnable"
                )
                log["post_network_running_total_ns"] = sum(
                    interval["duration_ns"]
                    for interval in intervals
                    if interval["state"] == "Running"
                )
        log["wall_minus_trace_ns"] = log["wall_unix_nano"] - log["trace_time_ns"]
        print(json.dumps(log, indent=2 if args.indent else None, sort_keys=True))


if __name__ == "__main__":
    main()
