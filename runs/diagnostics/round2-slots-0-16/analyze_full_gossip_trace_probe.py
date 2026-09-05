#!/usr/bin/env python3
"""Join probe 14 wall records to its focused Go runtime-trace transitions."""

import argparse
import csv
import json
from pathlib import Path


SYNC_OFFSET_NS = 1788428880873456997
TRACE_EVENTS = [
    ("server_reader_wake", 277057483265664, "GoID=4914 Waiting->Runnable"),
    ("server_reader_running", 277057913732096, "GoID=4914 Runnable->Running"),
    ("loopy_writer_wake", 277057913756928, "GoID=4912 Waiting->Runnable"),
    ("handler_created", 277057913805696, "GoID=15522 NotExist->Runnable"),
    ("handler_running", 277057913817728, "GoID=15522 Runnable->Running"),
    ("handler_exit", 277057913865856, "GoID=15522 Running->NotExist"),
    ("loopy_writer_running", 277058141273856, "GoID=4912 Runnable->Running"),
    ("loopy_writer_gosched", 277058141410688, 'GoID=4912 Running->Runnable Reason="runtime.Gosched"'),
    ("loopy_writer_resumed", 277058477457408, "GoID=4912 Runnable->Running"),
    ("response_write_start", 277058477466176, "GoID=4912 Running->Syscall"),
    ("response_write_end", 277058477522752, "GoID=4912 Syscall->Running"),
    ("server_reader_next_wake", 277058487285376, "GoID=4914 Waiting->Runnable"),
]


def jsonl_probe(path, probe):
    matches = []
    for line in path.read_text().splitlines():
        if not line:
            continue
        record = json.loads(line)
        if record.get("probe") == probe:
            matches.append(record)
    if len(matches) != 1:
        raise ValueError(f"expected one probe {probe} in {path}, found {len(matches)}")
    return matches[0]


def trace_provenance(path):
    provenance = {}
    header = ""
    for line in path.read_text().splitlines():
        if line.startswith("# parsed_lines="):
            header = line.removeprefix("# ").split(" reasons=", 1)[0]
        elif line.startswith("M=") and " Time=" in line:
            timestamp = int(line.split(" Time=", 1)[1].split(" ", 1)[0])
            provenance[timestamp] = (header, line)
    return provenance


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("evidence", type=Path)
    parser.add_argument("focused_trace", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--probe", type=int, default=14)
    args = parser.parse_args()

    client = jsonl_probe(args.evidence / "client.jsonl", args.probe)
    server = jsonl_probe(args.evidence / "server.jsonl", args.probe)
    provenance = trace_provenance(args.focused_trace)
    invoke_trace = client["invoke_unix_nano"] - SYNC_OFFSET_NS

    rows = [
        ("client_invoke", invoke_trace, "client.jsonl"),
        ("server_admission", server["admission_unix_nano"] - SYNC_OFFSET_NS, "server.jsonl"),
        ("server_handler_return", server["return_unix_nano"] - SYNC_OFFSET_NS, "server.jsonl"),
        ("client_return", client["return_unix_nano"] - SYNC_OFFSET_NS, "client.jsonl"),
    ]
    for label, timestamp, needle in TRACE_EVENTS:
        if timestamp not in provenance or needle not in provenance[timestamp][1]:
            raise ValueError(f"missing expected {label} transition at {timestamp}")
        rows.append((label, timestamp, provenance[timestamp][0]))
    rows.sort(key=lambda row: row[1])

    with args.output.open("w", newline="") as output:
        writer = csv.writer(output, delimiter="\t", lineterminator="\n")
        writer.writerow(
            ["phase", "trace_ns", "wall_unix_nano", "from_invoke_ms", "from_previous_ms", "provenance"]
        )
        previous = None
        for label, timestamp, source in rows:
            writer.writerow(
                [
                    label,
                    timestamp,
                    timestamp + SYNC_OFFSET_NS,
                    f"{(timestamp - invoke_trace) / 1e6:.6f}",
                    "" if previous is None else f"{(timestamp - previous) / 1e6:.6f}",
                    source,
                ]
            )
            previous = timestamp


if __name__ == "__main__":
    main()
