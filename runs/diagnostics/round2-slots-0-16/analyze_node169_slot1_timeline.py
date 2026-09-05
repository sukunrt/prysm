#!/usr/bin/env python3
"""Extract node 169's complete local slot-1 component timeline.

The input is the already extracted five-file node archive.  No other node or
archive is scanned.  Proxy request and response JSON is paired by the snooper
sequence number and retained in compact canonical form.
"""

from __future__ import annotations

import argparse
import csv
import json
import re
from datetime import datetime, timedelta, timezone
from pathlib import Path


GENESIS = datetime(2026, 9, 5, 1, 30, tzinfo=timezone.utc)
ANSI = re.compile(r"\x1b\[[0-9;]*m")
OUTER = re.compile(r"^(\S+)\s?(.*)$")
INNER = re.compile(r"\[(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.(\d+))\]")
PROXY_HEADER = re.compile(r"\b(REQUEST|RESPONSE) #(\d+):")


def instant(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def iso(value: datetime) -> str:
    return value.isoformat(timespec="microseconds").replace("+00:00", "Z")


def offset_ms(value: datetime) -> str:
    return f"{(value - GENESIS).total_seconds() * 1000:.3f}"


def read_lines(path: Path) -> list[tuple[int, datetime, str]]:
    rows = []
    # Container logs can retain bare carriage returns inside one physical LF
    # record. Text-mode universal newline iteration incorrectly counts each
    # such CR as another source line, so split bytes only on the physical LF.
    for line_number, raw in enumerate(path.read_bytes().split(b"\n"), 1):
        clean = ANSI.sub("", raw.decode(errors="replace").rstrip("\r"))
        match = OUTER.match(clean)
        if not match:
            continue
        try:
            timestamp = instant(match.group(1))
        except ValueError:
            continue
        rows.append((line_number, timestamp, match.group(2)))
    return rows


def inner_bounds(timestamp: datetime, text: str) -> tuple[str, str, str]:
    """Return inner timestamp and collector-delay bounds from its precision."""
    match = INNER.search(text)
    if not match:
        return "", "", ""
    inner = datetime.strptime(match.group(1), "%Y-%m-%d %H:%M:%S.%f").replace(tzinfo=timezone.utc)
    quantum = timedelta(seconds=10 ** -len(match.group(2)))
    maximum = (timestamp - inner).total_seconds() * 1000
    minimum = max(0.0, (timestamp - (inner + quantum)).total_seconds() * 1000)
    return match.group(1), f"{minimum:.3f}", f"{maximum:.3f}"


def parse_proxy(path: Path) -> list[dict[str, object]]:
    lines = read_lines(path)
    events: list[dict[str, object]] = []
    for index, (line_number, timestamp, text) in enumerate(lines):
        header = PROXY_HEADER.search(text)
        if not header:
            continue
        direction, sequence = header.groups()
        body_lines = []
        payload = {}
        for _, _, following in lines[index + 1:]:
            if PROXY_HEADER.search(following):
                break
            if not following.strip():
                if body_lines:
                    break
                continue
            if body_lines or following.lstrip().startswith("{"):
                body_lines.append(following)
                try:
                    payload = json.loads("\n".join(body_lines))
                    break
                except json.JSONDecodeError:
                    pass
        if body_lines and not payload:
            raise ValueError(f"proxy sequence {sequence} has invalid logged JSON")
        method_match = re.search(r"\bmethod=(\S+)", text)
        duration_match = re.search(r"\bduration_ms=(\d+)", text)
        status_match = re.search(r"\bstatus=(\d+)", text)
        events.append({
            "direction": direction.lower(),
            "sequence": sequence,
            "timestamp": timestamp,
            "line": line_number,
            "method": method_match.group(1) if method_match else "",
            "reported_duration_ms": duration_match.group(1) if duration_match else "",
            "status": status_match.group(1) if status_match else "",
            "payload": payload,
        })
    request_methods = {event["sequence"]: event["method"] for event in events if event["direction"] == "request"}
    for event in events:
        if not event["method"]:
            event["method"] = request_methods.get(event["sequence"], "")
    return events


def write_rpc(path: Path, source: Path, events: list[dict[str, object]], start: float, end: float) -> None:
    requests = {event["sequence"]: event for event in events if event["direction"] == "request"}
    responses = {event["sequence"]: event for event in events if event["direction"] == "response"}
    fields = ["proxy_sequence", "method", "request_timestamp", "request_genesis_offset_ms",
              "response_timestamp", "response_genesis_offset_ms", "header_elapsed_ms",
              "reported_duration_ms", "http_status", "json_rpc_id", "request_json",
              "response_json", "request_anchor", "response_anchor"]
    with path.open("w", newline="") as target:
        writer = csv.DictWriter(target, fieldnames=fields, delimiter="\t", lineterminator="\n")
        writer.writeheader()
        for sequence, request in requests.items():
            request_offset = (request["timestamp"] - GENESIS).total_seconds()
            if not start <= request_offset < end:
                continue
            response = responses.get(sequence)
            if response is None:
                raise ValueError(f"proxy sequence {sequence} has no response")
            writer.writerow({
                "proxy_sequence": sequence,
                "method": request["method"],
                "request_timestamp": iso(request["timestamp"]),
                "request_genesis_offset_ms": offset_ms(request["timestamp"]),
                "response_timestamp": iso(response["timestamp"]),
                "response_genesis_offset_ms": offset_ms(response["timestamp"]),
                "header_elapsed_ms": f"{(response['timestamp'] - request['timestamp']).total_seconds() * 1000:.3f}",
                "reported_duration_ms": response["reported_duration_ms"],
                "http_status": response["status"],
                "json_rpc_id": request["payload"].get("id", ""),
                "request_json": json.dumps(request["payload"], sort_keys=True, separators=(",", ":")),
                "response_json": json.dumps(response["payload"], sort_keys=True, separators=(",", ":")),
                "request_anchor": f"{source}:{request['line']}",
                "response_anchor": f"{source}:{response['line']}",
            })


def write_timeline(path: Path, input_directory: Path, proxy: list[dict[str, object]], start: float, end: float) -> None:
    rows = []
    for component in ("beacon", "validator", "execution"):
        source = input_directory / f"{component}.log"
        for line_number, timestamp, text in read_lines(source):
            offset = (timestamp - GENESIS).total_seconds()
            if not start <= offset < end:
                continue
            inner, delay_min, delay_max = inner_bounds(timestamp, text)
            rows.append({
                "timestamp": iso(timestamp), "genesis_offset_ms": offset_ms(timestamp),
                "component": component, "event_kind": "log", "method": "", "proxy_sequence": "",
                "inner_timestamp_display": inner, "collector_delay_min_ms": delay_min,
                "collector_delay_max_ms": delay_max, "source_anchor": f"{source}:{line_number}",
                "detail": text,
            })
    proxy_source = input_directory / "snooper-engine.log"
    for event in proxy:
        offset = (event["timestamp"] - GENESIS).total_seconds()
        if not start <= offset < end:
            continue
        rows.append({
            "timestamp": iso(event["timestamp"]), "genesis_offset_ms": offset_ms(event["timestamp"]),
            "component": "engine_proxy", "event_kind": event["direction"], "method": event["method"],
            "proxy_sequence": event["sequence"], "inner_timestamp_display": "",
            "collector_delay_min_ms": "", "collector_delay_max_ms": "",
            "source_anchor": f"{proxy_source}:{event['line']}",
            "detail": json.dumps(event["payload"], sort_keys=True, separators=(",", ":")),
        })
    rows.sort(key=lambda row: (row["timestamp"], row["component"], row["source_anchor"]))
    fields = list(rows[0])
    with path.open("w", newline="") as target:
        writer = csv.DictWriter(target, fieldnames=fields, delimiter="\t", lineterminator="\n")
        writer.writeheader()
        writer.writerows(rows)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input-directory", type=Path,
                        default=Path("/tmp/prysm-r2-extra-logs.Rd7MjT/extracted/round2-169"))
    parser.add_argument("--output-directory", type=Path, default=Path(__file__).resolve().parent)
    parser.add_argument("--start", type=float, default=12.0)
    parser.add_argument("--end", type=float, default=34.0)
    args = parser.parse_args()
    proxy = parse_proxy(args.input_directory / "snooper-engine.log")
    write_rpc(args.output_directory / "node169-slot1-engine-rpc.tsv",
              args.input_directory / "snooper-engine.log", proxy, args.start, args.end)
    write_timeline(args.output_directory / "node169-slot1-component-timeline.tsv",
                   args.input_directory, proxy, args.start, args.end)


if __name__ == "__main__":
    main()
