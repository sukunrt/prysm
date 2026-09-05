#!/usr/bin/env python3
"""Inventory transaction contents in retained round1 execution evidence.

The scanner reads the already-extracted ten-node sample under runs/round1.  It
parses rpc-snooper JSON at Engine API newPayload requests and getPayload
responses, and separately inventories execution-client transaction/gas markers
and possible transaction-submission/txpool evidence.
"""

from __future__ import annotations

import argparse
import json
import re
from collections import Counter
from pathlib import Path
from typing import Any


ANSI_RE = re.compile(r"\x1b\[[0-9;]*[A-Za-z]")
HEADER_RE = re.compile(
    r"INFO\[(?P<time>[^]]+)\] (?P<direction>REQUEST|RESPONSE) #(?P<call>\d+): POST /"
    r"(?P<fields>[^\n]*)"
)
METHOD_RE = re.compile(r"\bmethod=(?P<method>[^ ]+)")
ENGINE_PAYLOAD_RE = re.compile(r"^engine_(?:getPayload|newPayload)V\d+$")
EXECUTION_PAYLOAD_RE = re.compile(
    r"(?:Updated payload|Imported new potential chain segment).*?\btxs=(?P<txs>[0-9,]+)"
    r".*?\b(?:gas|mgas)=(?P<gas>[0-9.,]+)"
)
SUBMISSION_RE = re.compile(
    r"(?i)\b(?:eth_sendRawTransaction|eth_sendTransaction|sendRawTransaction|"
    r"submitted transaction|transaction submitted|transaction accepted|"
    r"transaction received|queued transaction|local transaction|txpool|"
    r"transaction pool|pooled transaction)\b"
)


def hex_int(value: Any) -> int | None:
    if not isinstance(value, str):
        return value if isinstance(value, int) else None
    try:
        return int(value, 0)
    except ValueError:
        return None


def payload_for(method: str, direction: str, body: Any) -> dict[str, Any] | None:
    if not isinstance(body, dict):
        return None
    if method.startswith("engine_newPayload") and direction == "REQUEST":
        params = body.get("params")
        return params[0] if isinstance(params, list) and params and isinstance(params[0], dict) else None
    if method.startswith("engine_getPayload") and direction == "RESPONSE":
        result = body.get("result")
        if not isinstance(result, dict):
            return None
        candidate = result.get("executionPayload", result)
        return candidate if isinstance(candidate, dict) else None
    return None


def rpc_methods(body: Any) -> list[str]:
    calls = body if isinstance(body, list) else [body]
    return [
        call["method"]
        for call in calls
        if isinstance(call, dict) and isinstance(call.get("method"), str)
    ]


def transaction_fields(value: Any, location: str = "$") -> list[tuple[str, Any]]:
    found: list[tuple[str, Any]] = []
    if isinstance(value, dict):
        for key, child in value.items():
            child_location = f"{location}.{key}"
            if key == "transactions":
                found.append((child_location, child))
            found.extend(transaction_fields(child, child_location))
    elif isinstance(value, list):
        for index, child in enumerate(value):
            found.extend(transaction_fields(child, f"{location}[{index}]"))
    return found


def parse_snooper(path: Path) -> dict[str, Any]:
    text = ANSI_RE.sub("", path.read_text(errors="replace"))
    headers = list(HEADER_RE.finditer(text))
    requests: dict[str, str] = {}
    events: list[dict[str, Any]] = []
    malformed: list[dict[str, str]] = []
    engine_event_count = 0
    engine_payload_missing = 0
    submission_rpc_calls: Counter[str] = Counter()
    generic_tx_fields: list[dict[str, Any]] = []

    for index, header in enumerate(headers):
        end = headers[index + 1].start() if index + 1 < len(headers) else len(text)
        span = text[header.end():end]
        direction = header.group("direction")
        call = header.group("call")
        method_match = METHOD_RE.search(header.group("fields"))
        method = method_match.group("method") if method_match else requests.get(call)
        brace = span.find("{")
        body = None
        if brace < 0:
            malformed.append({"call": call, "direction": direction, "method": method or "", "error": "no JSON object"})
        else:
            try:
                body, _ = json.JSONDecoder().raw_decode(span[brace:])
            except json.JSONDecodeError as exc:
                malformed.append({"call": call, "direction": direction, "method": method or "", "error": str(exc)})

        if direction == "REQUEST":
            body_methods = rpc_methods(body)
            if body_methods:
                method = ",".join(body_methods)
                requests[call] = method
            for body_method in body_methods:
                if body_method in {"eth_sendRawTransaction", "eth_sendTransaction"}:
                    submission_rpc_calls[body_method] += 1
        if body is not None:
            for location, txs in transaction_fields(body):
                generic_tx_fields.append(
                    {
                        "call": int(call),
                        "direction": direction.lower(),
                        "method": method,
                        "observed_at": header.group("time"),
                        "location": location,
                        "transaction_count": len(txs) if isinstance(txs, list) else None,
                        "transactions_field_type": type(txs).__name__,
                    }
                )
        if not method or not ENGINE_PAYLOAD_RE.match(method):
            continue

        engine_event_count += 1
        payload = payload_for(method, direction, body)
        is_payload_side = (
            method.startswith("engine_newPayload") and direction == "REQUEST"
        ) or (method.startswith("engine_getPayload") and direction == "RESPONSE")
        if not is_payload_side:
            continue
        if payload is None:
            engine_payload_missing += 1
            continue
        txs = payload.get("transactions")
        tx_count = len(txs) if isinstance(txs, list) else None
        events.append(
            {
                "node": path.parent.name,
                "call": int(call),
                "direction": direction.lower(),
                "method": method,
                "observed_at": header.group("time"),
                "block_hash": payload.get("blockHash") or payload.get("hash"),
                "block_number": hex_int(payload.get("blockNumber") or payload.get("number")),
                "slot": hex_int(payload.get("slotNumber")),
                "payload_timestamp": hex_int(payload.get("timestamp")),
                "transaction_count": tx_count,
                "transactions_field_type": type(txs).__name__,
                "gas_used": hex_int(payload.get("gasUsed")),
                "blob_gas_used": hex_int(payload.get("blobGasUsed")),
            }
        )

    times = [h.group("time") for h in headers]
    return {
        "path": str(path),
        "rpc_event_count": len(headers),
        "first_rpc_time": min(times) if times else None,
        "last_rpc_time": max(times) if times else None,
        "engine_payload_rpc_event_count": engine_event_count,
        "engine_payload_observations": events,
        "engine_payload_missing": engine_payload_missing,
        "malformed_rpc_json_count": len(malformed),
        "malformed_rpc_json": malformed,
        "submission_rpc_calls": dict(submission_rpc_calls),
        "transaction_fields": generic_tx_fields,
    }


def scan_execution(path: Path) -> dict[str, Any]:
    marker_count = zero_count = nonzero_count = malformed_count = 0
    submission_lines: list[dict[str, Any]] = []
    first_line = last_line = None
    with path.open(errors="replace") as source:
        for line_number, line in enumerate(source, 1):
            if first_line is None:
                first_line = line.rstrip("\n")
            last_line = line.rstrip("\n")
            if "Updated payload" in line or "Imported new potential chain segment" in line:
                marker_count += 1
                match = EXECUTION_PAYLOAD_RE.search(line)
                if not match:
                    malformed_count += 1
                else:
                    txs = int(match.group("txs").replace(",", ""))
                    gas = float(match.group("gas").replace(",", ""))
                    if txs == 0 and gas == 0:
                        zero_count += 1
                    else:
                        nonzero_count += 1
            if SUBMISSION_RE.search(line):
                submission_lines.append({"line": line_number, "text": ANSI_RE.sub("", line.rstrip())})
    return {
        "path": str(path),
        "first_line": first_line,
        "last_line": last_line,
        "payload_or_import_marker_count": marker_count,
        "zero_tx_and_gas_marker_count": zero_count,
        "nonzero_tx_or_gas_marker_count": nonzero_count,
        "unparsed_payload_or_import_marker_count": malformed_count,
        "submission_or_txpool_marker_count": len(submission_lines),
        "submission_or_txpool_markers": submission_lines,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("round1", nargs="?", type=Path, default=Path("runs/round1"))
    parser.add_argument("--pretty", action="store_true")
    args = parser.parse_args()

    nodes = sorted(p for p in args.round1.glob("prysm-geth-*") if p.is_dir())
    snooper = [parse_snooper(node / "snooper-engine.log") for node in nodes]
    execution = [scan_execution(node / "execution.log") for node in nodes]
    events = [event for item in snooper for event in item["engine_payload_observations"]]
    tx_counts = [event["transaction_count"] for event in events]
    gas = [event["gas_used"] for event in events]
    slots = [event["slot"] for event in events if event["slot"] is not None]
    hashes = sorted({event["block_hash"] for event in events if event["block_hash"]})
    observed_times = [event["observed_at"] for event in events]
    all_tx_fields = [field for item in snooper for field in item["transaction_fields"]]
    all_tx_counts = [field["transaction_count"] for field in all_tx_fields]
    tx_fields_by_rpc = Counter(
        f"{field['method']} {field['direction']}" for field in all_tx_fields
    )

    result = {
        "scope": {
            "round": "round1",
            "source": str(args.round1),
            "node_count": len(nodes),
            "nodes": [node.name for node in nodes],
            "provenance": "ten saved nodes, per runs/diagnostics/startup_diagnosis.md; extracted archive contents",
        },
        "engine_payload_summary": {
            "observation_count": len(events),
            "empty_transaction_array_count": sum(count == 0 for count in tx_counts),
            "nonempty_transaction_array_count": sum(isinstance(count, int) and count > 0 for count in tx_counts),
            "missing_or_nonarray_transactions_count": sum(count is None for count in tx_counts),
            "zero_gas_used_count": sum(value == 0 for value in gas),
            "nonzero_gas_used_count": sum(isinstance(value, int) and value > 0 for value in gas),
            "missing_or_malformed_gas_used_count": sum(value is None for value in gas),
            "unique_block_hash_count": len(hashes),
            "unique_block_hashes": hashes,
            "min_slot": min(slots) if slots else None,
            "max_slot": max(slots) if slots else None,
            "first_observation_time": min(observed_times) if observed_times else None,
            "last_observation_time": max(observed_times) if observed_times else None,
            "engine_payload_missing_count": sum(item["engine_payload_missing"] for item in snooper),
            "malformed_rpc_json_count_all_methods": sum(item["malformed_rpc_json_count"] for item in snooper),
        },
        "execution_marker_summary": {
            "marker_count": sum(item["payload_or_import_marker_count"] for item in execution),
            "zero_tx_and_gas_marker_count": sum(item["zero_tx_and_gas_marker_count"] for item in execution),
            "nonzero_tx_or_gas_marker_count": sum(item["nonzero_tx_or_gas_marker_count"] for item in execution),
            "unparsed_marker_count": sum(item["unparsed_payload_or_import_marker_count"] for item in execution),
        },
        "all_snooper_transaction_fields_summary": {
            "field_count": len(all_tx_fields),
            "empty_array_count": sum(count == 0 for count in all_tx_counts),
            "nonempty_array_count": sum(isinstance(count, int) and count > 0 for count in all_tx_counts),
            "missing_or_nonarray_count": sum(count is None for count in all_tx_counts),
            "by_rpc_method_and_direction": dict(sorted(tx_fields_by_rpc.items())),
        },
        "submission_evidence_summary": {
            "snooper_eth_send_calls": sum(sum(item["submission_rpc_calls"].values()) for item in snooper),
            "execution_submission_or_txpool_markers": sum(item["submission_or_txpool_marker_count"] for item in execution),
            "xatu_note": "Each retained xatu-sentry.log says 'Mempool transaction watcher disabled'.",
            "limit": "No submission-generator log is retained, so absence of these markers does not prove no sending was attempted elsewhere.",
        },
        "engine_payload_observations": events,
        "snooper_by_node": snooper,
        "execution_by_node": execution,
    }
    print(json.dumps(result, indent=2 if args.pretty else None, sort_keys=True))


if __name__ == "__main__":
    main()
