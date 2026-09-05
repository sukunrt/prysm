#!/usr/bin/env python3
"""Extract the retained Round 2 slot-110 owner and observer timeline."""

from __future__ import annotations

import hashlib
import json
import re
import subprocess
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent
OWNER = OUT / "round2-slot110-owner-node148"
ARCHIVE = OUT / "round2-slot110-owner-node148.tar.gz"
SLOT_START = datetime.fromisoformat("2026-09-05T01:52:00+00:00")
ANSI = re.compile(r"\x1b\[[0-9;]*[mK]")
STAMP = re.compile(r"^(2026-09-05T\d\d:\d\d:\d\d\.\d+Z)")


def lines(path: Path):
    # Preserve physical LF line numbers. Some retained logs contain embedded CR
    # bytes; str.splitlines() would count those as extra source lines.
    return path.read_bytes().decode(errors="replace").split("\n")


def clean(s: str) -> str:
    return ANSI.sub("", s)


def offset_ms(s: str) -> float:
    stamp = STAMP.match(s)
    if not stamp:
        raise ValueError(f"missing timestamp: {s[:100]}")
    instant = datetime.fromisoformat(stamp.group(1).replace("Z", "+00:00"))
    return round((instant - SLOT_START).total_seconds() * 1000, 6)


def matching(path: Path, label_patterns):
    data = lines(path)
    found = []
    for label, pattern in label_patterns:
        hits = [(i, clean(row)) for i, row in enumerate(data, 1) if re.search(pattern, clean(row))]
        if len(hits) != 1:
            raise RuntimeError(f"expected one {label} record in {path}, found {len(hits)}")
        line, raw = hits[0]
        found.append({"event": label, "source": str(path.relative_to(ROOT)), "line": line,
                      "timestamp": STAMP.match(raw).group(1), "offset_ms": offset_ms(raw), "raw": raw})
    return found


def engine_record(data, number: int):
    header = re.compile(rf"^(.*?) (REQUEST|RESPONSE) #{number}:")
    starts = [(i, clean(row), header.match(clean(row))) for i, row in enumerate(data, 1)
              if header.match(clean(row))]
    if len(starts) != 2:
        raise RuntimeError(f"expected request and response for #{number}, found {len(starts)}")
    result = []
    all_headers = [i for i, row in enumerate(data, 1) if "REQUEST #" in row or "RESPONSE #" in row]
    request_method = re.search(r"method=(\w+)", starts[0][1]).group(1)
    for line, raw, match in starts:
        later = [i for i in all_headers if i > line]
        end = (min(later) - 1) if later else len(data)
        body = "\n".join(clean(x) for x in data[line - 1:end])
        result.append({"event": f"engine_{match.group(2).lower()}_{number}",
                       "source": str((OWNER / "snooper-engine.log").relative_to(ROOT)),
                       "line": line, "end_line": end,
                       "timestamp": STAMP.match(raw).group(1), "offset_ms": offset_ms(raw),
                       "header": raw,
                       "method": request_method,
                       "jsonrpc_id": int(re.search(r'"id": (\d+)', body).group(1)),
                       "status": (re.search(r'"status": "([A-Z]+)"', body).group(1)
                                  if re.search(r'"status": "([A-Z]+)"', body) else None),
                       "payload_id": (re.search(r'"payloadId": "([^"]+)"', body).group(1)
                                      if re.search(r'"payloadId": "([^"]+)"', body) else None),
                       "block_hash": (re.search(r'"blockHash": "([^"]+)"', body).group(1)
                                      if re.search(r'"blockHash": "([^"]+)"', body) else None),
                       "parent_hash": (re.search(r'"parentHash": "([^"]+)"', body).group(1)
                                       if re.search(r'"parentHash": "([^"]+)"', body) else None),
                       "transactions_empty": '"transactions": []' in body,
                       "gas_used_zero": '"gasUsed": "0x0"' in body,
                       "execution_requests_empty": '"executionRequests": []' in body})
    return result


def observer_imports():
    result = []
    paths = sorted((ROOT / "runs/round2").glob("prysm-geth-*/beacon.log"))
    scan = subprocess.run(
        ["rg", "--color", "never", "-n", r"Synced new block.*slot.*=110\b", *map(str, paths)],
        check=True, capture_output=True, text=True,
    ).stdout.splitlines()
    by_path = {}
    for hit in scan:
        name, line, raw = hit.split(":", 2)
        by_path.setdefault(Path(name), []).append((int(line), clean(raw)))
    for path in paths:
        node = int(path.parent.name.rsplit("-", 1)[1])
        hits = by_path.get(path, [])
        if len(hits) != 1:
            raise RuntimeError(f"node {node}: expected one slot-110 import, found {len(hits)}")
        line, raw = hits[0]
        result.append({"node": node, "source": str(path.relative_to(ROOT)), "line": line,
                       "timestamp": STAMP.match(raw).group(1), "offset_ms": offset_ms(raw), "raw": raw})
    return sorted(result, key=lambda x: x["offset_ms"])


def main():
    owner_events = []
    owner_events += matching(OWNER / "validator.log", [
        ("proposer_duty", r"Duties schedule .*proposerPubkey=0xa929a06ad750 .*slot=110\b"),
        ("validator_submitted", r"Submitted new block .*slot=110\b"),
    ])
    owner_events += matching(OWNER / "beacon.log", [
        ("parent109_import", r"Synced new block .*slot=109\b"),
        ("parent109_envelope", r"Synced execution payload envelope .*slot=109\b"),
        ("build_entry", r"Building block .*slot=110\b"),
        ("eth1_head_fallback", r"01:52:00\.023703683Z .*Voting period before genesis \+ follow distance"),
        ("payload_choice", r"Chose payload bid .*slot=110\b"),
        ("event_stream_slow_reader", r"01:52:01\.725042996Z .*Client is unable to keep up with event stream.*shutting down"),
        ("build_finish", r"Finished building block .*slot=110\b"),
        ("owner_import", r"Synced new block .*slot=110\b"),
        ("state_transition_applied", r"Finished applying state transition .*slot=110\b"),
        ("envelope_published", r"Published execution payload envelope .*slot=110\b"),
        ("envelope_imported", r"Synced execution payload envelope .*slot=110\b"),
    ])
    owner_events += matching(OWNER / "execution.log", [
        ("geth_payload_ready", r"01:51:49\.399985625Z .*Updated payload .*id=0x0460a255558bbf2d"),
        ("geth_payload_delivery", r"01:52:00\.024143897Z .*Stopping work on payload .*reason=delivery"),
        ("geth_import", r"01:52:03\.670057329Z .*Imported new potential chain segment .*hash=3d8b75"),
    ])
    owner_events += matching(OWNER / "xatu-sentry.log", [
        ("xatu_queue_full", r"01:52:00\.307372415Z .*\"error\":\"queue is full\""),
        ("xatu_upstream_deadline", r"01:52:04\.379211896Z .*DeadlineExceeded"),
    ])
    engine = []
    engine_data = lines(OWNER / "snooper-engine.log")
    for number in (1180, 1181, 1182):
        engine += engine_record(engine_data, number)
    imports = observer_imports()
    digest = hashlib.sha256(ARCHIVE.read_bytes()).hexdigest()
    document = {
        "scope": "Round 2 slot 110 retained owner node 148 and 12 retained observers",
        "genesis": "2026-09-05T01:30:00Z",
        "slot": 110,
        "slot_start": SLOT_START.isoformat().replace("+00:00", "Z"),
        "owner_node": 148,
        "proposer_validator_index": 39774,
        "proposer_pubkey_prefix": "0xa929a06ad750",
        "archive": str(ARCHIVE.relative_to(ROOT)),
        "archive_sha256": digest,
        "block_root": "0xf287cca39adc7e69ca3916a27edcfb2a163627e31989222d1318d902838816de",
        "execution_block_hash": "0x3d8b7508ed7d7dd30a991d82e367295879edf4c5da0deefa53bd489a5d118040",
        "execution_parent_hash": "0x4605354653950925dde75cdd48a2786966938c883187a557ca1a4012842d9832",
        "owner_events": sorted(owner_events, key=lambda x: x["timestamp"]),
        "engine_records": engine,
        "observer_imports": imports,
        "observer_import_offset_bounds_ms": [imports[0]["offset_ms"], imports[-1]["offset_ms"]],
        "boundaries": [
            "Node 148 is a compact archive: beacon.log is INFO/WARN level and has no detailed FFG vote-ledger rows.",
            "Engine proxy timestamps are from snooper-engine.log and do not provide internal consensus-stage spans.",
            "The event-stream and Xatu records are contemporaneous observations; this extraction does not attribute causation.",
        ],
    }
    (OUT / "round2-slot110-timeline.json").write_text(json.dumps(document, indent=2) + "\n")


if __name__ == "__main__":
    main()
