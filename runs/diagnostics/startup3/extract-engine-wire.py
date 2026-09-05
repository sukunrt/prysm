#!/usr/bin/env python3
import argparse
import datetime
import json
import re
import subprocess


def tshark(path, fields, display_filter=""):
    command = ["tshark", "-r", path]
    if display_filter:
        command += ["-d", "tcp.port==8561,http", "-Y", display_filter]
    command += ["-T", "fields", "-E", "separator=|", "-E", "occurrence=f"]
    for field in fields:
        command += ["-e", field]
    return subprocess.check_output(command, text=True).splitlines()


def timestamp(value):
    return datetime.datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--pcap", required=True)
    parser.add_argument("--beacon-log", required=True)
    parser.add_argument("--bn-ip", required=True)
    args = parser.parse_args()

    ansi = re.compile(r"\x1b\[[0-9;]*m")
    diagnostics = {}
    for raw in open(args.beacon_log, errors="replace"):
        line = ansi.sub("", raw)
        if "startupdiagnostic:" not in line or "execution.probe." not in line:
            continue
        fields = dict(re.findall(r"(captured_time|event|id|phase)=([^ ]+)", line))
        if not fields:
            continue
        record = diagnostics.setdefault(int(fields["id"]), {})
        captured = timestamp(fields["captured_time"])
        if fields["phase"].endswith("WroteRequest") and fields["event"] == "begin":
            record["write"] = captured
        if fields["phase"].endswith("GotFirstResponseByte") and fields["event"] == "begin":
            record["first_byte"] = captured
        if ".result." in fields["phase"] and fields["event"] == "begin":
            record["result"] = fields["phase"].rsplit(".", 1)[1]

    bodies = []
    body_fields = ["frame.number", "frame.time_epoch", "ip.src", "tcp.stream", "tcp.seq", "tcp.len", "http.file_data"]
    for line in tshark(args.pcap, body_fields, "http.file_data"):
        parts = line.split("|", 6)
        try:
            payload = json.loads(bytes.fromhex(parts[6]))
            bodies.append({
                "frame": int(parts[0]), "time": float(parts[1]), "source": parts[2],
                "stream": int(parts[3]), "sequence": int(parts[4]), "length": int(parts[5]),
                "rpc_id": payload.get("id"), "method": payload.get("method"),
                "is_response": "result" in payload,
            })
        except (ValueError, json.JSONDecodeError, UnicodeDecodeError):
            continue

    packets = []
    packet_fields = ["frame.number", "frame.time_epoch", "ip.src", "tcp.stream", "tcp.ack"]
    for line in tshark(args.pcap, packet_fields):
        parts = line.split("|")
        try:
            packets.append({"frame": int(parts[0]), "time": float(parts[1]), "source": parts[2], "stream": int(parts[3]), "ack": int(parts[4])})
        except (ValueError, IndexError):
            continue

    requests = [body for body in bodies if body["source"] == args.bn_ip and body["method"] == "engine_getPayloadV6"]
    rows = []
    for diagnostic_id, diagnostic in diagnostics.items():
        if not {"write", "first_byte", "result"} <= diagnostic.keys() or not requests:
            continue
        request = min(requests, key=lambda item: abs(item["time"] - diagnostic["write"]))
        if abs(request["time"] - diagnostic["write"]) > 0.01:
            continue
        response = next(body for body in bodies if body["source"] != args.bn_ip and body["rpc_id"] == request["rpc_id"] and body["is_response"])
        sequence_end = response["sequence"] + response["length"]
        acknowledgement = next(packet for packet in packets if packet["source"] == args.bn_ip and packet["stream"] == response["stream"] and packet["time"] >= response["time"] and packet["ack"] >= sequence_end)
        rows.append({
            "diagnostic_id": diagnostic_id, "json_rpc_id": request["rpc_id"], "result": diagnostic["result"],
            "request_wire_unix": request["time"], "response_wire_unix": response["time"],
            "ack_wire_unix": acknowledgement["time"], "callback_unix": diagnostic["first_byte"],
            "request_to_response_ms": (response["time"] - request["time"]) * 1000,
            "response_to_ack_ms": (acknowledgement["time"] - response["time"]) * 1000,
            "response_to_callback_ms": (diagnostic["first_byte"] - response["time"]) * 1000,
            "write_to_callback_ms": (diagnostic["first_byte"] - diagnostic["write"]) * 1000,
        })
    print(json.dumps({"count": len(rows), "results": {result: sum(row["result"] == result for row in rows) for result in sorted({row["result"] for row in rows})}, "rows": rows}, indent=2))


if __name__ == "__main__":
    main()
