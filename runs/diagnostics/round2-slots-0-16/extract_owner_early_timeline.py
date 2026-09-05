#!/usr/bin/env python3
"""Extract a bounded, sanitized early timeline for the round-2 slot owners.

The extractor reads only the 16 owner archives.  It deliberately does not emit
HTTP headers from snooper-engine.log; Engine API output contains selected JSON
body fields and request/response metadata only.
"""

from __future__ import annotations

import argparse
import csv
import datetime as dt
import glob
import json
import math
import re
import tarfile
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any


GENESIS = dt.datetime.fromisoformat("2026-09-05T01:30:00+00:00")
WINDOW_START = GENESIS - dt.timedelta(seconds=12)
WINDOW_END = GENESIS + dt.timedelta(seconds=240)
OWNER_BY_SLOT = {
    1: 169, 2: 191, 3: 22, 4: 91, 5: 118, 6: 83, 7: 144, 8: 19,
    9: 107, 10: 35, 11: 117, 12: 14, 13: 20, 14: 85, 15: 32, 16: 76,
}
SLOT_BY_OWNER = {node: slot for slot, node in OWNER_BY_SLOT.items()}
MEMBERS = ("validator.log", "beacon.log", "execution.log", "snooper-engine.log")
ANSI_RE = re.compile(r"\x1b\[[0-9;]*m")
OUTER_TS_RE = re.compile(r"^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?Z)\s+(.*)$")
PRYSM_RE = re.compile(r"^\[[^]]+\]\s+(TRACE|DEBUG|INFO|WARN|ERROR)\s+([^:]+):\s+(.*)$")
GETH_RE = re.compile(r"^(TRACE|DEBUG|INFO|WARN|ERROR)\s+\[[^]]+\]\s+(.*)$")
SNOOPER_LOG_RE = re.compile(r"^(TRACE|DEBUG|INFO|WARN|ERROR)\[[^]]+\]\s+(.*)$")
FIELD_RE = re.compile(r"(?:^|\s)([A-Za-z][A-Za-z0-9_]*)=(.*?)(?=\s+[A-Za-z][A-Za-z0-9_]*=|$)")
HEADER_RE = re.compile(r"(?:REQUEST|RESPONSE) #(\d+):")
SNOOPER_REQUEST_RE = re.compile(r"REQUEST #(\d+):.*?\bmethod=(engine_[A-Za-z0-9_]+)")
SNOOPER_RESPONSE_RE = re.compile(r"RESPONSE #(\d+):.*?\bduration_ms=(\d+).*?\bstatus=(\d+)")
ENGINE_METHODS = {"engine_getPayloadV6", "engine_forkchoiceUpdatedV4"}


def parse_ts(value: str) -> dt.datetime:
    return dt.datetime.fromisoformat(value.replace("Z", "+00:00"))


def normalize_member(name: str) -> str:
    return name[2:] if name.startswith("./") else name


def anchor(archive: Path, member: str, line: int) -> str:
    return f"{archive}::{member}:{line}"


def fields_from(tail: str) -> tuple[str, dict[str, str]]:
    matches = list(FIELD_RE.finditer(tail))
    message = tail[: matches[0].start()].strip() if matches else tail.strip()
    return message, {m.group(1): m.group(2).strip() for m in matches}


def list_count(value: str | None) -> int | None:
    if not value or not value.startswith("[") or not value.endswith("]"):
        return None
    inner = value[1:-1].strip()
    if not inner:
        return 0
    return len([x for x in re.split(r"[\s,]+", inner) if x])


def sanitize_text(value: str, limit: int = 900) -> str:
    value = ANSI_RE.sub("", value).replace("\t", " ").rstrip()
    if "authorization" in value.lower() or re.search(r"\bBearer\s+", value, re.IGNORECASE):
        outer = OUTER_TS_RE.match(value)
        return (outer.group(1) + " <authorization-bearing record redacted>") if outer else "<authorization-bearing record redacted>"

    def repl(match: re.Match[str]) -> str:
        count = list_count(match.group(2))
        return f"{match.group(1)}=[<{count} items>]"

    value = re.sub(
        r"\b(pubkeys|attesterPubkeys|aggregatorPubkeys|committeeIndices|validatorIndices|ptcPubkeys)=(\[[^]]*\])",
        repl,
        value,
    )
    if len(value) > limit:
        return value[:limit] + "...<truncated>"
    return value


def wall_slot(ts: dt.datetime) -> int:
    return math.floor((ts - GENESIS).total_seconds() / 12)


def explicit_slot(fields: dict[str, str]) -> int | None:
    for key in ("slot", "dataSlot", "nextSlot", "attSlot"):
        value = fields.get(key)
        if value and re.fullmatch(r"\d+", value):
            return int(value)
    return None


def activity_slot(ts: dt.datetime, fields: dict[str, str], owned_slot: int) -> int:
    found = explicit_slot(fields)
    if found is not None:
        return found
    slot = wall_slot(ts)
    # Proposal errors often return just after the boundary and have no slot field.
    owned_end = GENESIS + dt.timedelta(seconds=(owned_slot + 1) * 12)
    if owned_slot <= 16 and owned_end <= ts < owned_end + dt.timedelta(seconds=3):
        return owned_slot
    return slot


def parse_standard(clean: str, member: str) -> tuple[str, str, str, dict[str, str]] | None:
    outer = OUTER_TS_RE.match(clean)
    if not outer:
        return None
    rest = outer.group(2)
    match = PRYSM_RE.match(rest) if member != "execution.log" else GETH_RE.match(rest)
    if not match:
        return None
    if member != "execution.log":
        severity, logger, tail = match.groups()
    else:
        severity, tail = match.groups()
        logger = "geth"
    message, fields = fields_from(tail)
    return severity, logger, message, fields


def selected_routine(member: str, message: str) -> bool:
    if member == "validator.log":
        return message.startswith(("Schedule for epoch", "Duties schedule", "Submitted ", "Skipping payload attestation"))
    if member == "beacon.log":
        needles = (
            "Connected peers", "Building block", "Finished building", "Finished proposing",
            "Submitted new block", "Block proposal", "Forkchoice updated", "Imported new block",
            "Synced", "Executing state transition", "Finished processing block",
        )
        return message.startswith(needles)
    if member == "execution.log":
        return message.startswith((
            "Starting work on payload", "Updated payload", "Stopping work on payload",
            "Imported new potential chain segment", "Chain head was updated", "Chain reorg detected",
        ))
    return False


def metric_subset(fields: dict[str, str]) -> dict[str, Any]:
    keep = (
        "slot", "dataSlot", "nextSlot", "duty", "submittedSinceSlotStart", "submissionSpread",
        "attesterCount", "proposerCount", "ptcCount", "syncCount", "syncCommitteeCount",
        "attestations", "messages", "contributions", "blockRoot", "sourceRoot", "sourceRound",
        "targetRoot", "targetRound", "payloadID", "payloadId", "id", "reason", "total",
        "inboundTCP", "outboundTCP", "target", "number", "hash", "txs", "withdrawals",
        "gas", "elapsed", "error", "prefix", "pubkey",
        "method", "duration_ms", "status", "length", "type",
    )
    result: dict[str, Any] = {k: fields[k] for k in keep if k in fields}
    for key in ("pubkeys", "attesterPubkeys", "aggregatorPubkeys", "ptcPubkeys", "validatorIndices"):
        count = list_count(fields.get(key))
        if count is not None:
            result[key + "Count"] = count
    return result


def json_fragment(lines: list[str], header_index: int) -> tuple[Any | None, int | None, int | None]:
    fragments: list[str] = []
    first = None
    last = None
    for index in range(header_index + 1, min(len(lines), header_index + 3000)):
        clean = ANSI_RE.sub("", lines[index])
        matched = OUTER_TS_RE.match(clean)
        if not matched:
            break
        body = matched.group(2)
        if HEADER_RE.search(body):
            break
        if not fragments and not body.lstrip().startswith(("{", "[")):
            if not body.strip():
                continue
            break
        if not body.strip() and fragments:
            break
        if body.strip():
            if first is None:
                first = index + 1
            last = index + 1
            fragments.append(body)
    if not fragments:
        return None, first, last
    try:
        return json.loads("\n".join(fragments)), first, last
    except json.JSONDecodeError:
        return None, first, last


def short_hash(value: Any) -> Any:
    if isinstance(value, str) and value.startswith("0x") and len(value) > 22:
        return value[:14] + "..." + value[-8:]
    return value


def request_summary(method: str, body: Any) -> dict[str, Any]:
    if not isinstance(body, dict):
        return {"parse": "unavailable"}
    params = body.get("params") or []
    result: dict[str, Any] = {"rpc_id": body.get("id"), "method": body.get("method", method)}
    if method == "engine_getPayloadV6":
        result["payload_id"] = params[0] if params else None
    elif method == "engine_forkchoiceUpdatedV4":
        state = params[0] if len(params) > 0 and isinstance(params[0], dict) else {}
        attrs = params[1] if len(params) > 1 and isinstance(params[1], dict) else None
        result["forkchoice_state"] = {k: short_hash(state.get(k)) for k in (
            "headBlockHash", "safeBlockHash", "finalizedBlockHash"
        ) if k in state}
        if attrs is None:
            result["payload_attributes"] = None
        else:
            result["payload_attributes"] = {
                k: (len(attrs[k]) if k == "withdrawals" and isinstance(attrs[k], list) else short_hash(attrs[k]))
                for k in ("timestamp", "prevRandao", "suggestedFeeRecipient", "withdrawals", "parentBeaconBlockRoot")
                if k in attrs
            }
    return result


def response_summary(method: str, body: Any) -> dict[str, Any]:
    if not isinstance(body, dict):
        return {"parse": "unavailable"}
    out: dict[str, Any] = {"rpc_id": body.get("id")}
    if "error" in body:
        out["error"] = body["error"]
        return out
    result = body.get("result")
    if method == "engine_forkchoiceUpdatedV4" and isinstance(result, dict):
        status = result.get("payloadStatus") or {}
        out["payload_status"] = {
            k: short_hash(status.get(k)) for k in ("status", "latestValidHash", "validationError") if k in status
        }
        out["payload_id"] = result.get("payloadId")
    elif method == "engine_getPayloadV6" and isinstance(result, dict):
        payload = result.get("executionPayload") or {}
        out["execution_payload"] = {
            k: short_hash(payload.get(k)) for k in (
                "blockHash", "parentHash", "blockNumber", "slotNumber", "timestamp",
                "gasUsed", "gasLimit", "feeRecipient",
            ) if k in payload
        }
        out["transactions_count"] = len(payload.get("transactions", [])) if isinstance(payload.get("transactions"), list) else None
        out["withdrawals_count"] = len(payload.get("withdrawals", [])) if isinstance(payload.get("withdrawals"), list) else None
        for key in ("blockValue", "shouldOverrideBuilder"):
            if key in result:
                out[key] = result[key]
    return out


def find_archive(directory: Path, node: int) -> Path:
    matches = sorted(glob.glob(str(directory / f"round2-prysm-geth-{node}.tar.gz")))
    if len(matches) != 1:
        raise RuntimeError(f"expected one archive for node {node}, found {matches}")
    return Path(matches[0]).resolve()


def write_tsv(path: Path, columns: list[str], rows: list[dict[str, Any]]) -> None:
    with path.open("w", newline="") as file:
        writer = csv.DictWriter(file, fieldnames=columns, delimiter="\t", extrasaction="ignore")
        writer.writeheader()
        for row in rows:
            writer.writerow({key: row.get(key, "") for key in columns})


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--archive-dir", type=Path, default=Path("/tmp/prysm-r2-extra-logs.Rd7MjT"))
    parser.add_argument("--output-dir", type=Path, default=Path(__file__).resolve().parent)
    args = parser.parse_args()
    args.output_dir.mkdir(parents=True, exist_ok=True)

    type_groups: dict[tuple[Any, ...], dict[str, Any]] = {}
    nonroutine: list[dict[str, Any]] = []
    routine: list[dict[str, Any]] = []
    all_standard: list[dict[str, Any]] = []
    submissions: list[dict[str, Any]] = []
    rpcs: list[dict[str, Any]] = []
    archive_rows: list[dict[str, Any]] = []

    for owned_slot, node in sorted(OWNER_BY_SLOT.items()):
        archive = find_archive(args.archive_dir, node)
        with tarfile.open(archive, "r:gz") as tar:
            by_name = {normalize_member(member.name): member for member in tar.getmembers() if member.isfile()}
            missing = [name for name in MEMBERS if name not in by_name]
            if missing:
                raise RuntimeError(f"{archive}: missing {missing}")
            archive_rows.append({"node": node, "owned_slot": owned_slot, "archive": archive, "bytes": archive.stat().st_size})

            for member in MEMBERS[:3]:
                handle = tar.extractfile(by_name[member])
                assert handle is not None
                for line_no, raw in enumerate(handle, 1):
                    original = raw.decode("utf-8", errors="replace").rstrip("\n")
                    clean = ANSI_RE.sub("", original)
                    ts_match = OUTER_TS_RE.match(clean)
                    if not ts_match:
                        continue
                    ts = parse_ts(ts_match.group(1))
                    if not (WINDOW_START <= ts <= WINDOW_END):
                        continue
                    parsed = parse_standard(clean, member)
                    if not parsed:
                        continue
                    severity, logger, message, fields = parsed
                    source = anchor(archive, member, line_no)
                    slot = activity_slot(ts, fields, owned_slot)
                    key = (node, owned_slot, member, severity, logger, message)
                    group = type_groups.setdefault(key, {
                        "node": node, "owned_slot": owned_slot, "component": member,
                        "severity": severity, "logger": logger, "message_type": message,
                        "count": 0, "first_timestamp": ts_match.group(1), "first_anchor": source,
                        "last_timestamp": ts_match.group(1), "last_anchor": source,
                    })
                    group["count"] += 1
                    group["last_timestamp"] = ts_match.group(1)
                    group["last_anchor"] = source
                    base = {
                        "node": node, "owned_slot": owned_slot, "activity_slot": slot,
                        "wall_slot": wall_slot(ts), "timestamp": ts_match.group(1),
                        "genesis_offset_ms": round((ts - GENESIS).total_seconds() * 1000, 3),
                        "component": member, "severity": severity, "logger": logger,
                        "message_type": message, "fields_json": json.dumps(metric_subset(fields), sort_keys=True),
                        "source_anchor": source, "sanitized_record": sanitize_text(original),
                    }
                    all_standard.append(base)
                    if severity in {"WARN", "ERROR"}:
                        nonroutine.append(base)
                    if selected_routine(member, message):
                        routine.append(base)
                    if member == "validator.log" and message.startswith((
                        "Submitted ", "Skipping payload attestation", "Schedule for epoch", "Duties schedule",
                    )):
                        counts = {key: list_count(fields.get(key)) for key in (
                            "pubkeys", "attesterPubkeys", "aggregatorPubkeys", "ptcPubkeys", "validatorIndices"
                        )}
                        submissions.append({
                            **base,
                            "explicit_slot": explicit_slot(fields),
                            "duty": fields.get("duty", ""),
                            "submitted_since_slot_start": fields.get("submittedSinceSlotStart", ""),
                            "submission_spread": fields.get("submissionSpread", ""),
                            "key_count": next((value for value in counts.values() if value is not None), ""),
                            "attestations": fields.get("attestations", ""),
                            "messages": fields.get("messages", ""),
                            "contributions": fields.get("contributions", ""),
                            "attester_count": fields.get("attesterCount", ""),
                            "proposer_count": fields.get("proposerCount", ""),
                            "proposer_pubkey": fields.get("proposerPubkey", ""),
                            "slot_in_epoch": fields.get("slotInEpoch", ""),
                            "ptc_count": fields.get("ptcCount", ""),
                            "sync_count": fields.get("syncCount", fields.get("syncCommitteeCount", "")),
                        })

            snooper = "snooper-engine.log"
            handle = tar.extractfile(by_name[snooper])
            assert handle is not None
            lines = [raw.decode("utf-8", errors="replace").rstrip("\n") for raw in handle]
            requests: dict[int, dict[str, Any]] = {}
            for index, original in enumerate(lines):
                clean = ANSI_RE.sub("", original)
                outer = OUTER_TS_RE.match(clean)
                if not outer:
                    continue
                ts = parse_ts(outer.group(1))
                if not (WINDOW_START <= ts <= WINDOW_END):
                    continue
                snooper_log = SNOOPER_LOG_RE.match(outer.group(2))
                if snooper_log:
                    severity, tail = snooper_log.groups()
                    message, fields = fields_from(tail)
                    request_header = SNOOPER_REQUEST_RE.search(tail)
                    if request_header:
                        message = "REQUEST " + request_header.group(2)
                    elif SNOOPER_RESPONSE_RE.search(tail):
                        message = "RESPONSE"
                    source = anchor(archive, snooper, index + 1)
                    key = (node, owned_slot, snooper, severity, "proxy", message)
                    group = type_groups.setdefault(key, {
                        "node": node, "owned_slot": owned_slot, "component": snooper,
                        "severity": severity, "logger": "proxy", "message_type": message,
                        "count": 0, "first_timestamp": outer.group(1), "first_anchor": source,
                        "last_timestamp": outer.group(1), "last_anchor": source,
                    })
                    group["count"] += 1
                    group["last_timestamp"] = outer.group(1)
                    group["last_anchor"] = source
                    if severity in {"WARN", "ERROR"}:
                        row = {
                            "node": node, "owned_slot": owned_slot, "activity_slot": wall_slot(ts),
                            "wall_slot": wall_slot(ts), "timestamp": outer.group(1),
                            "genesis_offset_ms": round((ts - GENESIS).total_seconds() * 1000, 3),
                            "component": snooper, "severity": severity, "logger": "proxy",
                            "message_type": message, "fields_json": json.dumps(metric_subset(fields), sort_keys=True),
                            "source_anchor": source, "sanitized_record": sanitize_text(original),
                        }
                        all_standard.append(row)
                        nonroutine.append(row)
                req = SNOOPER_REQUEST_RE.search(outer.group(2))
                if req:
                    number, method = int(req.group(1)), req.group(2)
                    if method not in ENGINE_METHODS:
                        continue
                    body, body_first, body_last = json_fragment(lines, index)
                    requests[number] = {
                        "node": node, "owned_slot": owned_slot, "proxy_id": number, "method": method,
                        "request_timestamp": outer.group(1), "request_genesis_offset_ms": round((ts - GENESIS).total_seconds() * 1000, 3),
                        "request_header_anchor": anchor(archive, snooper, index + 1),
                        "request_body_anchor": anchor(archive, snooper, body_first) if body_first else "",
                        "request_body_end_anchor": anchor(archive, snooper, body_last) if body_last else "",
                        "request_summary_json": json.dumps(request_summary(method, body), sort_keys=True, separators=(",", ":")),
                    }
                    continue
                response = SNOOPER_RESPONSE_RE.search(outer.group(2))
                if response:
                    number, duration_ms, status = map(int, response.groups())
                    row = requests.get(number)
                    if row is None:
                        continue
                    body, body_first, body_last = json_fragment(lines, index)
                    row.update({
                        "response_timestamp": outer.group(1), "response_genesis_offset_ms": round((ts - GENESIS).total_seconds() * 1000, 3),
                        "duration_ms": duration_ms, "http_status": status,
                        "response_header_anchor": anchor(archive, snooper, index + 1),
                        "response_body_anchor": anchor(archive, snooper, body_first) if body_first else "",
                        "response_body_end_anchor": anchor(archive, snooper, body_last) if body_last else "",
                        "response_summary_json": json.dumps(response_summary(row["method"], body), sort_keys=True, separators=(",", ":")),
                    })
                    rpcs.append(row)

    type_rows = sorted(type_groups.values(), key=lambda r: (r["node"], r["component"], r["severity"], r["message_type"]))
    timeline = sorted(nonroutine + routine, key=lambda r: (r["timestamp"], r["node"], r["source_anchor"]))
    nonroutine.sort(key=lambda r: (r["timestamp"], r["node"], r["source_anchor"]))
    submissions.sort(key=lambda r: (r["timestamp"], r["node"]))
    rpcs.sort(key=lambda r: (r["request_timestamp"], r["node"], r["proxy_id"]))
    for row in rpcs:
        request = json.loads(row["request_summary_json"])
        response = json.loads(row.get("response_summary_json", "{}"))
        target: int | None = None
        if row["method"] == "engine_forkchoiceUpdatedV4":
            attrs = request.get("payload_attributes")
            if isinstance(attrs, dict) and isinstance(attrs.get("timestamp"), str):
                target = math.floor((int(attrs["timestamp"], 16) - GENESIS.timestamp()) / 12)
        elif row["method"] == "engine_getPayloadV6":
            value = (response.get("execution_payload") or {}).get("slotNumber")
            if isinstance(value, str):
                target = int(value, 16)
        row["target_slot"] = target if target is not None else wall_slot(parse_ts(row["request_timestamp"]))

    event_cols = [
        "node", "owned_slot", "activity_slot", "wall_slot", "timestamp", "genesis_offset_ms",
        "component", "severity", "logger", "message_type", "fields_json", "source_anchor", "sanitized_record",
    ]
    write_tsv(args.output_dir / "owner_early_timeline.tsv", event_cols, timeline)
    write_tsv(args.output_dir / "owner_nonroutine_events.tsv", event_cols, nonroutine)
    write_tsv(args.output_dir / "owner_message_types.tsv", [
        "node", "owned_slot", "component", "severity", "logger", "message_type", "count",
        "first_timestamp", "first_anchor", "last_timestamp", "last_anchor",
    ], type_rows)
    write_tsv(args.output_dir / "validator_role_progress.tsv", [
        "node", "owned_slot", "activity_slot", "wall_slot", "timestamp", "genesis_offset_ms",
        "message_type", "explicit_slot", "duty", "submitted_since_slot_start", "submission_spread",
        "key_count", "attestations", "messages", "contributions", "attester_count", "proposer_count",
        "ptc_count", "sync_count", "proposer_pubkey", "slot_in_epoch", "fields_json", "source_anchor",
    ], submissions)
    write_tsv(args.output_dir / "engine_rpc_timeline.tsv", [
        "node", "owned_slot", "target_slot", "proxy_id", "method", "request_timestamp", "request_genesis_offset_ms",
        "request_header_anchor", "request_body_anchor", "request_body_end_anchor", "request_summary_json",
        "response_timestamp", "response_genesis_offset_ms", "duration_ms", "http_status",
        "response_header_anchor", "response_body_anchor", "response_body_end_anchor", "response_summary_json",
    ], rpcs)
    write_tsv(args.output_dir / "owner_timeline_archives.tsv", ["node", "owned_slot", "archive", "bytes"], archive_rows)

    # One compact row per owner and early slot.  Counts are exact for parsed log records.
    activity_rows: list[dict[str, Any]] = []
    for owned_slot, node in sorted(OWNER_BY_SLOT.items()):
        for slot in range(-1, 21):
            selected = [r for r in all_standard if r["node"] == node and r["activity_slot"] == slot]
            selected.sort(key=lambda r: (r["timestamp"], r["component"], r["source_anchor"]))
            vc_ok = Counter(r["message_type"] for r in selected if r["component"] == "validator.log" and r["severity"] == "INFO")
            vc_bad = Counter(r["message_type"] for r in selected if r["component"] == "validator.log" and r["severity"] in {"WARN", "ERROR"})
            bn_bad = Counter(r["message_type"] for r in selected if r["component"] == "beacon.log" and r["severity"] in {"WARN", "ERROR"})
            bn_ok = Counter(r["message_type"] for r in selected if r["component"] == "beacon.log" and r["severity"] == "INFO")
            geth = Counter(r["message_type"] for r in selected if r["component"] == "execution.log")
            peer_values = []
            for row in selected:
                if row["message_type"] == "Connected peers":
                    metrics = json.loads(row["fields_json"])
                    if str(metrics.get("total", "")).isdigit():
                        peer_values.append(int(metrics["total"]))
            rpc_selected = [r for r in rpcs if r["node"] == node and r["target_slot"] == slot]
            activity_rows.append({
                "node": node, "owned_slot": owned_slot, "slot": slot,
                "event_count": len(selected),
                "first_timestamp": selected[0]["timestamp"] if selected else "",
                "first_anchor": selected[0]["source_anchor"] if selected else "",
                "last_timestamp": selected[-1]["timestamp"] if selected else "",
                "last_anchor": selected[-1]["source_anchor"] if selected else "",
                "validator_info_counts": json.dumps(vc_ok, sort_keys=True, separators=(",", ":")),
                "validator_nonroutine_counts": json.dumps(vc_bad, sort_keys=True, separators=(",", ":")),
                "beacon_info_counts": json.dumps(bn_ok, sort_keys=True, separators=(",", ":")),
                "beacon_nonroutine_counts": json.dumps(bn_bad, sort_keys=True, separators=(",", ":")),
                "execution_counts": json.dumps(geth, sort_keys=True, separators=(",", ":")),
                "engine_fcu_count": sum(r["method"] == "engine_forkchoiceUpdatedV4" for r in rpc_selected),
                "engine_getpayload_count": sum(r["method"] == "engine_getPayloadV6" for r in rpc_selected),
                "peer_min": min(peer_values) if peer_values else "", "peer_max": max(peer_values) if peer_values else "",
            })
    write_tsv(args.output_dir / "owner_slot_activity.tsv", [
        "node", "owned_slot", "slot", "event_count", "first_timestamp", "first_anchor", "last_timestamp", "last_anchor",
        "validator_info_counts", "validator_nonroutine_counts", "beacon_info_counts", "beacon_nonroutine_counts",
        "execution_counts", "engine_fcu_count", "engine_getpayload_count", "peer_min", "peer_max",
    ], activity_rows)

    # Match the four payload requests whose BN later logged the literal timeout.
    timeout_rows: list[dict[str, Any]] = []
    for owned_slot, node in ((5, 118), (6, 83), (8, 19), (9, 107)):
        getpayload = [r for r in rpcs if r["node"] == node and r["method"] == "engine_getPayloadV6"]
        if len(getpayload) != 1:
            raise RuntimeError(f"node {node}: expected one getPayloadV6, found {len(getpayload)}")
        rpc = getpayload[0]
        timeout = [r for r in nonroutine if r["node"] == node and r["message_type"] == "Could not get local payload, falling back to P2P bid"]
        if len(timeout) != 1:
            raise RuntimeError(f"node {node}: expected one local-payload timeout, found {len(timeout)}")
        timeout_ts = parse_ts(timeout[0]["timestamp"])
        request_ts = parse_ts(rpc["request_timestamp"])
        response_ts = parse_ts(rpc["response_timestamp"])
        timeout_rows.append({
            "node": node, "slot": owned_slot, "proxy_id": rpc["proxy_id"],
            "payload_id": json.loads(rpc["request_summary_json"])["payload_id"],
            "request_timestamp": rpc["request_timestamp"], "response_timestamp": rpc["response_timestamp"],
            "proxy_duration_ms": rpc["duration_ms"], "timeout_log_timestamp": timeout[0]["timestamp"],
            "request_to_response_ms": round((response_ts - request_ts).total_seconds() * 1000, 3),
            "response_to_timeout_log_ms": round((timeout_ts - response_ts).total_seconds() * 1000, 3),
            "request_to_timeout_log_ms": round((timeout_ts - request_ts).total_seconds() * 1000, 3),
            "request_anchor": rpc["request_header_anchor"], "response_anchor": rpc["response_header_anchor"],
            "timeout_anchor": timeout[0]["source_anchor"],
            "source_timeout_ms": 300,
            "source_interpretation": "GetPayload creates a 300ms context deadline and passes it to rpcClient.CallContext",
        })
    write_tsv(args.output_dir / "getpayload_timeout_discriminators.tsv", [
        "node", "slot", "proxy_id", "payload_id", "request_timestamp", "response_timestamp", "proxy_duration_ms",
        "timeout_log_timestamp", "request_to_response_ms", "response_to_timeout_log_ms", "request_to_timeout_log_ms",
        "request_anchor", "response_anchor", "timeout_anchor", "source_timeout_ms", "source_interpretation",
    ], timeout_rows)

    # Summarize progress around each owner's proposal without reproducing every repetitive error.
    neighborhood_rows: list[dict[str, Any]] = []
    terminal_messages = {
        "Failed to sign randao reveal", "Failed to request block from beacon node", "Could not build block",
        "Fail to build block: could not get parent state", "Finished building block", "Submitted new block",
    }
    for owned_slot, node in sorted(OWNER_BY_SLOT.items()):
        node_rows = [r for r in all_standard if r["node"] == node]
        owned_rows = [r for r in node_rows if r["activity_slot"] == owned_slot]
        terminal = [r for r in owned_rows if r["message_type"] in terminal_messages]
        successes = [r for r in node_rows if r["message_type"].startswith("Submitted ")]
        slot_end = GENESIS + dt.timedelta(seconds=(owned_slot + 1) * 12)
        after = [r for r in successes if parse_ts(r["timestamp"]) >= slot_end]
        def counts_for(target: int, severity: str | None = None) -> Counter[str]:
            return Counter(
                r["message_type"] for r in node_rows
                if r["activity_slot"] == target and r["component"] == "validator.log"
                and (severity is None or r["severity"] == severity)
            )
        neighborhood_rows.append({
            "node": node, "owned_slot": owned_slot,
            "previous_slot_validator_info": json.dumps(counts_for(owned_slot - 1, "INFO"), sort_keys=True, separators=(",", ":")),
            "owned_slot_validator_info": json.dumps(counts_for(owned_slot, "INFO"), sort_keys=True, separators=(",", ":")),
            "owned_slot_validator_warn_error": json.dumps(Counter(
                r["message_type"] for r in owned_rows if r["component"] == "validator.log" and r["severity"] in {"WARN", "ERROR"}
            ), sort_keys=True, separators=(",", ":")),
            "next_slot_validator_info": json.dumps(counts_for(owned_slot + 1, "INFO"), sort_keys=True, separators=(",", ":")),
            "terminal_events": " | ".join(f'{r["message_type"]}@{r["timestamp"]}' for r in terminal),
            "terminal_anchors": " | ".join(r["source_anchor"] for r in terminal),
            "first_submission_after_slot_end": after[0]["timestamp"] if after else "",
            "first_submission_after_slot_end_type": after[0]["message_type"] if after else "",
            "first_submission_after_slot_end_anchor": after[0]["source_anchor"] if after else "",
        })
    write_tsv(args.output_dir / "owner_failure_neighborhood.tsv", [
        "node", "owned_slot", "previous_slot_validator_info", "owned_slot_validator_info",
        "owned_slot_validator_warn_error", "next_slot_validator_info", "terminal_events", "terminal_anchors",
        "first_submission_after_slot_end", "first_submission_after_slot_end_type", "first_submission_after_slot_end_anchor",
    ], neighborhood_rows)

    excerpt_lines = [
        "# Sanitized owner-local early-timeline excerpts", "",
        f"Window: `{WINDOW_START.isoformat()}` through `{WINDOW_END.isoformat()}` (inclusive).",
        "Long validator-key and committee lists are replaced by item counts. Engine API HTTP headers are never emitted.", "",
    ]
    proposal_messages = {
        "Failed to sign randao reveal", "Failed to request block from beacon node", "Building block",
        "Could not get local payload, falling back to P2P bid", "Could not build block",
        "Fail to build block: could not get parent state", "Chose payload bid", "Finished building block",
        "Submitted new block", "Forkchoice updated with payload attributes for proposal",
    }
    for owned_slot, node in sorted(OWNER_BY_SLOT.items()):
        excerpt_lines.extend((f"## Slot {owned_slot}: node {node}", ""))
        node_rows = [r for r in all_standard if r["node"] == node]
        schedule = [r for r in node_rows if r["message_type"] == "Duties schedule" and explicit_slot(json.loads(r["fields_json"])) == owned_slot]
        if schedule:
            excerpt_lines.extend((f"- Schedule `{schedule[0]['source_anchor']}`", f"  `{schedule[0]['sanitized_record']}`"))
        owned = [r for r in node_rows if r["activity_slot"] == owned_slot]
        for row in owned:
            if row["message_type"] in proposal_messages or row["message_type"].startswith("Submitted "):
                excerpt_lines.extend((f"- `{row['source_anchor']}`", f"  `{row['sanitized_record']}`"))
        grouped_errors: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for row in owned:
            if row["severity"] in {"WARN", "ERROR"} and row["message_type"] not in proposal_messages:
                grouped_errors[row["message_type"]].append(row)
        if grouped_errors:
            excerpt_lines.append("- Other owned-slot nonroutine records (exact count and boundary anchors):")
            for message, records in sorted(grouped_errors.items()):
                excerpt_lines.append(
                    f"  - `{message}`: {len(records)}; first `{records[0]['source_anchor']}`; last `{records[-1]['source_anchor']}`"
                )
        after_boundary = [
            r for r in node_rows if r["message_type"].startswith("Submitted ")
            and parse_ts(r["timestamp"]) >= GENESIS + dt.timedelta(seconds=(owned_slot + 1) * 12)
        ]
        if after_boundary:
            excerpt_lines.extend((
                f"- First later successful role output `{after_boundary[0]['source_anchor']}`",
                f"  `{after_boundary[0]['sanitized_record']}`",
            ))
        owned_rpcs = [r for r in rpcs if r["node"] == node and r["target_slot"] == owned_slot]
        for rpc in owned_rpcs:
            excerpt_lines.extend((
                f"- Engine `{rpc['method']}` proxy #{rpc['proxy_id']}: request `{rpc['request_header_anchor']}` / body `{rpc['request_body_anchor']}`; response `{rpc['response_header_anchor']}` / body `{rpc['response_body_anchor']}`; proxy duration `{rpc['duration_ms']} ms`.",
                f"  Request body fields: `{rpc['request_summary_json']}`",
                f"  Response body fields: `{rpc['response_summary_json']}`",
            ))
        excerpt_lines.append("")
    (args.output_dir / "owner_timeline_excerpts.md").write_text("\n".join(excerpt_lines) + "\n")

    print(json.dumps({
        "archives": len(archive_rows), "message_type_groups": len(type_rows), "timeline_rows": len(timeline),
        "nonroutine_rows": len(nonroutine), "validator_progress_rows": len(submissions), "engine_rpc_rows": len(rpcs),
        "timeout_discriminator_rows": len(timeout_rows), "owner_neighborhood_rows": len(neighborhood_rows),
    }, sort_keys=True))


if __name__ == "__main__":
    main()
