#!/usr/bin/env python3
"""Join owner FCU proxy responses to Prysm's payload-ID log markers.

The beacon node logs bytesutil.Trunc(payloadID[:]), which is the first six
bytes.  The engine timeline retains the full eight-byte ID.  A match therefore
requires the same owner node, target/next slot, and exact 12-hex-digit prefix.
"""

from __future__ import annotations

import argparse
import csv
import json
from datetime import datetime
from pathlib import Path


OWNERS = {
    1: 169,
    2: 191,
    3: 22,
    4: 91,
    5: 118,
    6: 83,
    7: 144,
    8: 19,
    9: 107,
    10: 35,
    11: 117,
    12: 14,
    13: 20,
    14: 85,
    15: 32,
    16: 76,
}

OUTCOME = {
    1: "preflight_DomainData",
    2: "preflight",
    3: "preflight",
    4: "preflight_DomainData",
    5: "getPayload_timeout",
    6: "getPayload_timeout",
    7: "preflight",
    8: "getPayload_timeout",
    9: "getPayload_timeout",
    10: "payload_selected_consensus_branch_late",
    11: "preflight",
    12: "preflight",
    13: "parent_state",
    14: "payload_selected_consensus_branch_late",
    15: "success",
    16: "success",
}

BN_PAYLOAD_MARKER = "Forkchoice updated with payload attributes for proposal"
BN_GETPAYLOAD_FAILURE = "Could not get local payload, falling back to P2P bid"


def instant(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def elapsed_ms(start: str, end: str) -> str:
    return f"{(instant(end) - instant(start)).total_seconds() * 1000:.3f}"


def load_tsv(path: Path) -> list[dict[str, str]]:
    with path.open(newline="") as source:
        return list(csv.DictReader(source, delimiter="\t"))


def payload_prefix(payload_id: str) -> str:
    value = payload_id.lower()
    if not value.startswith("0x") or len(value) != 18:
        raise ValueError(f"expected an eight-byte payload ID, got {payload_id!r}")
    return value[:14]


def joined_rows(engine: list[dict[str, str]], owner: list[dict[str, str]]) -> list[dict[str, str]]:
    markers: dict[tuple[int, int], list[dict[str, str]]] = {}
    failures: dict[int, list[dict[str, str]]] = {}
    for row in owner:
        node = int(row["node"])
        if row["message_type"] == BN_PAYLOAD_MARKER:
            fields = json.loads(row["fields_json"])
            slot = int(fields["nextSlot"])
            markers.setdefault((node, slot), []).append(row | {"logged_payload_id": fields["payloadID"].lower()})
        elif row["message_type"] == BN_GETPAYLOAD_FAILURE:
            failures.setdefault(node, []).append(row)

    fcus: dict[tuple[int, int], list[dict[str, str]]] = {}
    getpayloads: dict[tuple[int, int], list[dict[str, str]]] = {}
    for row in engine:
        key = (int(row["node"]), int(row["target_slot"]))
        if row["method"] == "engine_forkchoiceUpdatedV4" and row["http_status"] == "200":
            response = json.loads(row["response_summary_json"])
            payload_id = response.get("payload_id")
            status = (response.get("payload_status") or {}).get("status")
            if payload_id and status == "VALID":
                fcus.setdefault(key, []).append(row | {"full_payload_id": payload_id.lower()})
        elif row["method"].startswith("engine_getPayload"):
            request = json.loads(row["request_summary_json"])
            payload_id = request.get("payload_id")
            if payload_id:
                getpayloads.setdefault(key, []).append(row | {"full_payload_id": payload_id.lower()})

    result: list[dict[str, str]] = []
    for slot, node in OWNERS.items():
        key = (node, slot)
        candidates = sorted(fcus.get(key, []), key=lambda row: row["response_timestamp"])
        bn_markers = sorted(markers.get(key, []), key=lambda row: row["timestamp"])
        if slot == 1:
            if len(candidates) != 1 or bn_markers:
                raise ValueError("slot 1 should have one FCU and no comparable BN marker")
            selected = candidates[0]
            exact = []
            preceding = []
            following = []
            marker = None
            match_status = "no_comparable_bn_marker"
        else:
            if len(bn_markers) != 1:
                raise ValueError(f"slot {slot}: expected one BN marker, found {len(bn_markers)}")
            marker = bn_markers[0]
            token = marker["logged_payload_id"]
            exact = [row for row in candidates if payload_prefix(row["full_payload_id"]) == token]
            preceding = [row for row in exact if instant(row["response_timestamp"]) <= instant(marker["timestamp"])]
            following = [row for row in exact if instant(row["response_timestamp"]) > instant(marker["timestamp"])]
            if len(preceding) != 1:
                raise ValueError(f"slot {slot}: expected one preceding exact match, found {len(preceding)}")
            selected = preceding[0]
            match_status = "exact_unique_preceding"

        full_id = selected["full_payload_id"]
        gps = sorted(
            [row for row in getpayloads.get(key, []) if row["full_payload_id"] == full_id],
            key=lambda row: row["request_timestamp"],
        )
        if len(gps) > 1:
            raise ValueError(f"slot {slot}: multiple same-ID getPayload calls")
        gp = gps[0] if gps else None

        failure = None
        if gp:
            later_failures = [
                row for row in failures.get(node, [])
                if instant(row["timestamp"]) >= instant(gp["response_timestamp"])
            ]
            failure = min(later_failures, key=lambda row: row["timestamp"], default=None)
            if OUTCOME[slot] != "getPayload_timeout":
                failure = None

        result.append({
            "slot": str(slot),
            "node": str(node),
            "owner_outcome": OUTCOME[slot],
            "match_status": match_status,
            "fcu_proxy_id": selected["proxy_id"],
            "full_payload_id": full_id,
            "bn_logged_payload_id": marker["logged_payload_id"] if marker else "",
            "fcu_proxy_duration_ms": selected["duration_ms"],
            "fcu_response_timestamp": selected["response_timestamp"],
            "bn_payload_log_timestamp": marker["timestamp"] if marker else "",
            "response_to_bn_payload_log_ms": elapsed_ms(selected["response_timestamp"], marker["timestamp"]) if marker else "",
            "same_slot_valid_fcu_count": str(len(candidates)),
            "exact_id_fcu_count": str(len(exact)),
            "later_exact_id_fcu_count": str(len(following)),
            "later_exact_id_fcu_proxy_ids": ",".join(row["proxy_id"] for row in following),
            "other_payload_id_fcu_count": str(len(candidates) - len(exact)),
            "getpayload_method": gp["method"] if gp else "",
            "getpayload_proxy_id": gp["proxy_id"] if gp else "",
            "getpayload_proxy_duration_ms": gp["duration_ms"] if gp else "",
            "getpayload_http_status": gp["http_status"] if gp else "",
            "getpayload_response_timestamp": gp["response_timestamp"] if gp else "",
            "getpayload_bn_failure_timestamp": failure["timestamp"] if failure else "",
            "getpayload_response_to_bn_failure_ms": elapsed_ms(gp["response_timestamp"], failure["timestamp"]) if failure else "",
            "fcu_response_anchor": selected["response_header_anchor"],
            "bn_payload_log_anchor": marker["source_anchor"] if marker else "",
            "getpayload_response_anchor": gp["response_header_anchor"] if gp else "",
            "getpayload_bn_failure_anchor": failure["source_anchor"] if failure else "",
        })
    return result


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--directory", type=Path, default=Path(__file__).resolve().parent)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    output = args.output or args.directory / "fcu_bn_payload_delay.tsv"
    rows = joined_rows(
        load_tsv(args.directory / "engine_rpc_timeline.tsv"),
        load_tsv(args.directory / "owner_early_timeline.tsv"),
    )
    with output.open("w", newline="") as target:
        writer = csv.DictWriter(target, fieldnames=list(rows[0]), delimiter="\t", lineterminator="\n")
        writer.writeheader()
        writer.writerows(rows)


if __name__ == "__main__":
    main()
