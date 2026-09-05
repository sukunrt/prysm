#!/usr/bin/env python3
"""Audit transaction contents in the execution evidence retained for Round 2."""

from __future__ import annotations

import argparse
import collections
import csv
import json
import re
import subprocess
from pathlib import Path
from typing import Any


ANSI = re.compile(r"\x1b\[[0-?]*[ -/]*[@-~]")
OUTER_STAMP = re.compile(r"^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?Z)\s?(.*)$")
HEADER = re.compile(r"\b(REQUEST|RESPONSE) #(\d+):")
METHOD = re.compile(r"\bmethods?=([^ ]+)")
NODE = re.compile(r"prysm-geth-(\d+)$")
FIELD = re.compile(r"\b([A-Za-z][A-Za-z0-9]*)=([^ ]+)")
SUBMISSION_PATTERNS = {
    "eth_sendRawTransaction": re.compile(r"eth_sendRawTransaction", re.I),
    "eth_sendTransaction": re.compile(r"eth_sendTransaction", re.I),
    "submitted_transaction": re.compile(r"submitted (?:a )?transaction", re.I),
    "accepted_transaction": re.compile(r"transaction (?:was )?accepted", re.I),
    "added_transaction": re.compile(r"(?:added|queued) (?:a )?transaction", re.I),
    "broadcast_transaction": re.compile(r"broadcast(?:ing)? (?:a )?transaction", re.I),
}
ROUND2_START = "2026-09-05T01:30:00Z"


def integer(value: Any) -> int | None:
    if isinstance(value, int):
        return value
    if not isinstance(value, str):
        return None
    try:
        return int(value, 0)
    except ValueError:
        return None


def node_number(directory: Path) -> int:
    match = NODE.search(directory.name)
    if not match:
        raise ValueError(f"unrecognized node directory: {directory}")
    return int(match.group(1))


def stripped_line(raw: str) -> tuple[str | None, str]:
    clean = ANSI.sub("", raw.rstrip("\r\n"))
    match = OUTER_STAMP.match(clean)
    return (match.group(1), match.group(2)) if match else (None, clean)


def parse_snooper(path: Path) -> dict[str, Any]:
    """Reassemble the pretty-printed request and response JSON from rpc-snooper."""
    records: list[dict[str, Any]] = []
    active: dict[str, Any] | None = None
    first_stamp = last_stamp = None
    line_count = 0

    def finish(parse_error: str | None = None) -> None:
        nonlocal active
        if active is None:
            return
        text = "\n".join(active.pop("body_lines"))
        try:
            active["body"] = json.loads(text) if text else None
            active["parse_error"] = parse_error
        except json.JSONDecodeError as error:
            active["body"] = None
            active["parse_error"] = f"{error.msg} at {error.lineno}:{error.colno}"
        records.append(active)
        active = None

    with path.open(errors="replace") as source:
        for line_count, raw in enumerate(source, 1):
            stamp, text = stripped_line(raw)
            if stamp:
                first_stamp = first_stamp or stamp
                last_stamp = stamp
            match = HEADER.search(text)
            if match:
                if active is not None:
                    finish("interrupted by next header")
                method = METHOD.search(text)
                active = {
                    "kind": match.group(1).lower(),
                    "number": int(match.group(2)),
                    "method": method.group(1) if method else None,
                    "line": line_count,
                    "timestamp": stamp,
                    "body_lines": [],
                    "started": False,
                }
                continue
            if active is None:
                continue
            if not active["started"]:
                if text in ("{", "["):
                    active["started"] = True
                    active["closing"] = "}" if text == "{" else "]"
                    active["body_lines"].append(text)
                elif text.strip():
                    # Informational lines may occur between header and body.
                    continue
            else:
                active["body_lines"].append(text)
                if text == active["closing"]:
                    finish()
    if active is not None:
        finish("end of file before complete record")

    requests = {row["number"]: row for row in records if row["kind"] == "request"}
    responses = {row["number"]: row for row in records if row["kind"] == "response"}
    payload_rows: list[dict[str, Any]] = []
    block_query_rows: list[dict[str, Any]] = []

    def add_payload(
        request: dict[str, Any], response: dict[str, Any] | None, observation: str
    ) -> None:
        source = request if observation == "newPayload_request" else response
        row = {
            "node": node_number(path.parent),
            "observation": observation,
            "request_number": request["number"],
            "method": request["method"],
            "timestamp": (source or request)["timestamp"],
            "line": (source or request)["line"],
            "block_hash": None,
            "tx_count": None,
            "gas_used": None,
            "classification": None,
            "detail": None,
        }
        if source is None:
            row.update(classification="unmatched", detail="no response record")
            payload_rows.append(row)
            return
        if source["parse_error"]:
            row.update(classification="malformed", detail=source["parse_error"])
            payload_rows.append(row)
            return
        body = source["body"]
        payload = None
        if observation == "newPayload_request":
            if isinstance(body, dict) and isinstance(body.get("params"), list) and body["params"]:
                payload = body["params"][0]
        elif isinstance(body, dict) and "error" in body:
            row.update(classification="rpc_error", detail=json.dumps(body["error"], sort_keys=True))
            payload_rows.append(row)
            return
        elif isinstance(body, dict):
            result = body.get("result")
            if isinstance(result, dict):
                payload = result.get("executionPayload", result)
        if not isinstance(payload, dict):
            row.update(classification="malformed", detail="execution payload object missing")
        elif not isinstance(payload.get("transactions"), list):
            row.update(classification="malformed", detail="transactions array missing or not a list")
            row["block_hash"] = payload.get("blockHash")
            row["gas_used"] = integer(payload.get("gasUsed"))
        else:
            row["block_hash"] = payload.get("blockHash")
            row["tx_count"] = len(payload["transactions"])
            row["gas_used"] = integer(payload.get("gasUsed"))
            row["classification"] = "empty" if not payload["transactions"] else "nonempty"
        payload_rows.append(row)

    for number, request in sorted(requests.items()):
        method = request["method"] or ""
        if re.fullmatch(r"engine_newPayloadV\d+", method):
            add_payload(request, responses.get(number), "newPayload_request")
        elif re.fullmatch(r"engine_getPayloadV\d+", method):
            add_payload(request, responses.get(number), "getPayload_response")
        elif method in ("eth_getBlockByHash", "eth_getBlockByNumber"):
            response = responses.get(number)
            row = {
                "node": node_number(path.parent), "request_number": number, "method": method,
                "timestamp": (response or request)["timestamp"], "line": (response or request)["line"],
                "block_hash": None, "tx_count": None, "gas_used": None, "classification": None,
            }
            if response is None:
                row["classification"] = "unmatched"
            elif response["parse_error"]:
                row["classification"] = "malformed"
            else:
                response_body = response["body"]
                if isinstance(response_body, list) and len(response_body) == 1:
                    response_body = response_body[0]
                result = response_body.get("result") if isinstance(response_body, dict) else None
                if isinstance(result, dict) and isinstance(result.get("transactions"), list):
                    row.update(block_hash=result.get("hash"), tx_count=len(result["transactions"]),
                               gas_used=integer(result.get("gasUsed")),
                               classification="empty" if not result["transactions"] else "nonempty")
                else:
                    row["classification"] = "malformed"
            block_query_rows.append(row)

    body_rows: list[dict[str, Any]] = []
    for number, request in sorted(requests.items()):
        method = request["method"] or ""
        if not re.fullmatch(r"engine_getPayloadBodiesByHashV\d+", method):
            continue
        request_body = request["body"] if not request["parse_error"] else None
        hashes = (request_body.get("params", [[]])[0]
                  if isinstance(request_body, dict) and request_body.get("params") else [])
        response = responses.get(number)
        if response is None or response["parse_error"] or not isinstance(response["body"], dict):
            body_rows.append({
                "node": node_number(path.parent), "request_number": number, "method": method,
                "timestamp": (response or request)["timestamp"], "line": (response or request)["line"],
                "block_hash": None, "tx_count": None,
                "classification": "unmatched" if response is None else "malformed",
            })
            continue
        result = response["body"].get("result")
        if not isinstance(result, list):
            body_rows.append({
                "node": node_number(path.parent), "request_number": number, "method": method,
                "timestamp": response["timestamp"], "line": response["line"], "block_hash": None,
                "tx_count": None, "classification": "malformed",
            })
            continue
        for index in range(max(len(hashes) if isinstance(hashes, list) else 0, len(result))):
            body = result[index] if index < len(result) else None
            txs = body.get("transactions") if isinstance(body, dict) else None
            body_rows.append({
                "node": node_number(path.parent), "request_number": number, "method": method,
                "timestamp": response["timestamp"], "line": response["line"],
                "block_hash": hashes[index] if isinstance(hashes, list) and index < len(hashes) else None,
                "tx_count": len(txs) if isinstance(txs, list) else None,
                "classification": ("empty" if not txs else "nonempty") if isinstance(txs, list) else "malformed",
            })
    submission_requests = [
        {"node": node_number(path.parent), "request_number": number, "method": request["method"],
         "timestamp": request["timestamp"], "line": request["line"]}
        for number, request in sorted(requests.items())
        if request["method"] in ("eth_sendRawTransaction", "eth_sendTransaction")
    ]
    return {
        "path": str(path.resolve()), "bytes": path.stat().st_size, "lines": line_count,
        "first_timestamp": first_stamp, "last_timestamp": last_stamp,
        "records": len(records), "request_records": len(requests), "response_records": len(responses),
        "record_parse_errors": sum(bool(row["parse_error"]) for row in records),
        "payload_rows": payload_rows, "body_rows": body_rows,
        "block_query_rows": block_query_rows, "submission_requests": submission_requests,
    }


def scan_text_logs(node_dirs: list[Path]) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    markers: list[dict[str, Any]] = []
    coverage: dict[str, Any] = {}
    paths: list[Path] = []
    for directory in node_dirs:
        node = node_number(directory)
        for name in ("execution.log", "beacon.log", "validator.log", "xatu-sentry.log"):
            path = directory / name
            if not path.is_file():
                continue
            paths.append(path)
            coverage[f"{node}/{name}"] = {
                "path": str(path.resolve()), "bytes": path.stat().st_size,
            }
    pattern = (r"Payload envelope |\btxs=|eth_sendRawTransaction|eth_sendTransaction|"
               r"submitted (a )?transaction|transaction (was )?accepted|"
               r"(added|queued) (a )?transaction|broadcast(ing)? (a )?transaction")
    command = ["rg", "-n", "--no-heading", "-H", "-i", "-e", pattern, *map(str, paths)]
    result = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    if result.returncode not in (0, 1):
        raise RuntimeError(result.stderr.decode(errors="replace"))
    for raw in result.stdout.splitlines():
        path_text, line_text, raw_text = raw.decode(errors="replace").split(":", 2)
        path = Path(path_text)
        node = node_number(path.parent)
        line_number = int(line_text)
        stamp, text = stripped_line(raw_text)
        fields = dict(FIELD.findall(text))
        if path.name == "beacon.log" and "Payload envelope " in text:
            markers.append({
                "node": node, "source": path.name, "kind": "payload_envelope",
                "timestamp": stamp, "line": line_number, "identifier": fields.get("blockRoot"),
                "slot": integer(fields.get("slot")), "tx_count": integer(fields.get("txCount")),
                "gas_used": integer(fields.get("gasUsed")), "text": text,
            })
        if path.name == "execution.log" and "txs" in fields:
            markers.append({
                "node": node, "source": path.name, "kind": "execution_txs_field",
                "timestamp": stamp, "line": line_number,
                "identifier": fields.get("hash") or fields.get("tail"), "slot": None,
                "tx_count": integer(fields.get("txs")), "gas_used": integer(fields.get("gasUsed")),
                "text": text,
            })
        for label, submission_pattern in SUBMISSION_PATTERNS.items():
            if submission_pattern.search(text):
                markers.append({
                    "node": node, "source": path.name, "kind": f"submission:{label}",
                    "timestamp": stamp, "line": line_number, "identifier": None, "slot": None,
                    "tx_count": None, "gas_used": None, "text": text,
                })
    return markers, coverage


def counts(rows: list[dict[str, Any]]) -> dict[str, Any]:
    classes = collections.Counter(row["classification"] for row in rows)
    hashes = {row["block_hash"] for row in rows if row.get("block_hash")}
    nonzero_gas = sum((row.get("gas_used") or 0) > 0 for row in rows)
    return {
        "records": len(rows), "classifications": dict(sorted(classes.items())),
        "unique_block_hashes": len(hashes), "nonzero_gas_records": nonzero_gas,
        "by_method": dict(sorted(collections.Counter(row["method"] for row in rows).items())),
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--round2", type=Path, default=Path("runs/round2"))
    parser.add_argument("--output", type=Path, default=Path("runs/diagnostics/transaction-contents"))
    args = parser.parse_args()
    node_dirs = sorted(
        (path for path in args.round2.glob("prysm-geth-*") if path.is_dir()),
        key=node_number,
    )
    snoopers = [parse_snooper(path / "snooper-engine.log") for path in node_dirs
                if (path / "snooper-engine.log").is_file()]
    payload_rows = [row for result in snoopers for row in result.pop("payload_rows")]
    body_rows = [row for result in snoopers for row in result.pop("body_rows")]
    block_query_rows = [row for result in snoopers for row in result.pop("block_query_rows")]
    snooper_submission_rows = [row for result in snoopers for row in result.pop("submission_requests")]
    markers, text_coverage = scan_text_logs(node_dirs)
    envelope_rows = [row for row in markers if row["kind"] == "payload_envelope"]
    execution_rows = [row for row in markers if row["kind"] == "execution_txs_field"]
    submission_rows = [row for row in markers if row["kind"].startswith("submission:")]
    round2_payload_rows = [row for row in payload_rows
                           if row.get("timestamp") and row["timestamp"] >= ROUND2_START]
    round2_body_rows = [row for row in body_rows
                        if row.get("timestamp") and row["timestamp"] >= ROUND2_START]

    payload_by_kind = {
        kind: counts([row for row in payload_rows if row["observation"] == kind])
        for kind in ("getPayload_response", "newPayload_request")
    }
    summary = {
        "scope": {
            "run": "historical round2 only",
            "node_count": len(node_dirs),
            "nodes": [node_number(path) for path in node_dirs],
            "missing_advertised_archive_union": "/tmp/prysm-r2-extra-logs.Rd7MjT",
            "missing_advertised_archive_union_exists": Path("/tmp/prysm-r2-extra-logs.Rd7MjT").exists(),
            "note": "The startup3 reproduction and diagnostic reproduction outputs are excluded.",
        },
        "engine_payloads": {
            "full_snooper_files": counts(payload_rows),
            "round2_at_or_after_2026_09_05T01_30_00Z": counts(round2_payload_rows),
            "by_observation_full_snooper_files": payload_by_kind,
            "rows_with_tx_count": sum(row["tx_count"] is not None for row in payload_rows),
            "transaction_total_across_records": sum(row["tx_count"] or 0 for row in payload_rows),
        },
        "payload_body_recovery": {
            "full_snooper_files": counts(body_rows),
            "round2_at_or_after_2026_09_05T01_30_00Z": counts(round2_body_rows),
            "transaction_total_across_records": sum(row["tx_count"] or 0 for row in body_rows),
        },
        "eth_block_queries": counts(block_query_rows),
        "beacon_payload_envelopes": {
            "records": len(envelope_rows),
            "unique_block_roots": len({row["identifier"] for row in envelope_rows if row["identifier"]}),
            "zero_tx_records": sum(row["tx_count"] == 0 for row in envelope_rows),
            "nonzero_tx_records": sum((row["tx_count"] or 0) > 0 for row in envelope_rows),
            "missing_tx_count_records": sum(row["tx_count"] is None for row in envelope_rows),
            "nonzero_gas_records": sum((row["gas_used"] or 0) > 0 for row in envelope_rows),
        },
        "execution_txs_markers": {
            "records": len(execution_rows),
            "zero_tx_records": sum(row["tx_count"] == 0 for row in execution_rows),
            "nonzero_tx_records": sum((row["tx_count"] or 0) > 0 for row in execution_rows),
        },
        "transaction_submission_markers": {
            "records": len(submission_rows) + len(snooper_submission_rows),
            "snooper_rpc_requests": len(snooper_submission_rows),
            "other_log_markers": len(submission_rows),
            "by_pattern": dict(sorted(collections.Counter(row["kind"] for row in submission_rows).items())),
            "interpretation": "Absence only means no configured retained log recorded these markers.",
        },
        "snooper_coverage": snoopers,
        "text_log_coverage": text_coverage,
    }
    args.output.mkdir(parents=True, exist_ok=True)
    with (args.output / "round2_payload_audit.json").open("w") as sink:
        json.dump(summary, sink, indent=2, sort_keys=True)
        sink.write("\n")
    with (args.output / "round2_engine_payloads.tsv").open("w", newline="") as sink:
        columns = ("node", "observation", "request_number", "method", "timestamp", "line",
                   "block_hash", "tx_count", "gas_used", "classification", "detail")
        writer = csv.DictWriter(sink, fieldnames=columns, delimiter="\t", extrasaction="ignore")
        writer.writeheader()
        writer.writerows(payload_rows + [dict(row, observation="payloadBody_response",
                                              gas_used=None, detail=None) for row in body_rows])
    with (args.output / "round2_log_markers.tsv").open("w", newline="") as sink:
        columns = ("node", "source", "kind", "timestamp", "line", "identifier", "slot",
                   "tx_count", "gas_used", "text")
        writer = csv.DictWriter(sink, fieldnames=columns, delimiter="\t", extrasaction="ignore")
        writer.writeheader()
        writer.writerows(markers)

    combined = summary["engine_payloads"]["full_snooper_files"]
    in_round = summary["engine_payloads"]["round2_at_or_after_2026_09_05T01_30_00Z"]
    body_full = summary["payload_body_recovery"]["full_snooper_files"]
    body_round = summary["payload_body_recovery"]["round2_at_or_after_2026_09_05T01_30_00Z"]
    envelope = summary["beacon_payload_envelopes"]
    report = f"""# Historical Round 2 transaction-content audit

## Finding

Every inspectable Engine execution payload in the currently retained historical Round 2 evidence is empty. At or after the conservative Round 2 boundary `2026-09-05T01:30:00Z`, there are **{in_round['records']} parsed `getPayload`/`newPayload` observations over {in_round['unique_block_hashes']} distinct execution block hashes, zero nonempty transaction arrays, and zero records with nonzero `gasUsed`**. The {body_round['records']} payload-body recovery results in that interval, over {body_round['unique_block_hashes']} requested hashes, are also all empty.

Across the complete snooper files, including records retained from before the Round 2 execution restart, all {combined['records']} full-payload observations over {combined['unique_block_hashes']} hashes and all {body_full['records']} body-only observations over {body_full['unique_block_hashes']} hashes are empty. This combines every recorded Engine API version suffix; method-specific counts are in `round2_payload_audit.json`.

The independent Prysm beacon markers agree: **{envelope['records']} `Payload envelope` records covering {envelope['unique_block_roots']} distinct beacon block roots all report `txCount=0` and `gasUsed=0`**. The Geth execution-log `txs=` markers also contain {summary['execution_txs_markers']['nonzero_tx_records']} nonzero records out of {summary['execution_txs_markers']['records']}.

No transaction-submission or acceptance marker matched the retained logs. That does **not** prove nobody tried to send a transaction. The snooper explicitly targeted the authenticated Engine endpoint `execution:8551`, while Geth separately advertised the unauthenticated public endpoint on port `8545`; a sender using the public endpoint would not appear in these snooper records. The retained services may also omit sender traffic or rejection logs. Empty included payloads prove that no transaction was included in those payloads.

## Coverage and limits

This is a fresh full-file scan of all {summary['scope']['node_count']} currently retained node directories: {', '.join(map(str, summary['scope']['nodes']))}. It covers each retained `snooper-engine.log` plus available `execution.log`, `beacon.log`, `validator.log`, and `xatu-sentry.log`, over their complete retained time bounds. The startup3 reproduction and later diagnostic reproductions are excluded.

The previously advertised 1,000-node temporary union at `/tmp/prysm-r2-extra-logs.Rd7MjT` no longer exists. Consequently, this audit cannot fresh-scan payload bodies for all 1,000 nodes. The 12 snooper captures are whole retained captures, but they begin before the Round 2 execution logs and contain prior history. The report therefore separates the full-file all-zero census from observations at or after the stated Round 2 boundary. The conclusion must be stated as **all execution payloads inspectable in the currently retained Round 2 logs were empty**, rather than an unqualified statement about every node or every attempted transaction in the original run.

`eth_getBlockBy*` poll responses are reported separately because they repeatedly observe the same blocks and are not new payload constructions. They contain {summary['eth_block_queries']['records']} records over {summary['eth_block_queries']['unique_block_hashes']} distinct block hashes; none are nonempty.

## Reproduction

Run from the repository root:

```sh
python3 runs/diagnostics/transaction-contents/round2_audit_transactions.py
```

The compact JSON contains exact counts and per-file coverage. `round2_engine_payloads.tsv` has one row per target Engine payload observation; `round2_log_markers.tsv` preserves the independent beacon, execution, and submission-marker evidence.
"""
    (args.output / "round2_report.md").write_text(report)


if __name__ == "__main__":
    main()
