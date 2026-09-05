#!/usr/bin/env python3
"""Extract an evidence-only Round-2 slot 0..16 census from saved archives."""

from __future__ import annotations

import argparse
import collections
import concurrent.futures
import csv
import datetime as dt
import json
import re
import tarfile
from pathlib import Path


ANSI = re.compile(r"\x1b\[[0-?]*[ -/]*[@-~]")
ARCHIVE = re.compile(r"round2-prysm-geth-(\d+)\.tar\.gz$")
STAMP = re.compile(r"^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?Z)")
FIELD = re.compile(r"\b([A-Za-z][A-Za-z0-9]*)=([^ ]+)")
SLOT = re.compile(r"\bslot=(\d+)\b")
VOTE_SLOT = re.compile(r"\bvoteSlot=(\d+)\b")
PROPOSER = re.compile(r"\bproposerPubkey=(0x[0-9a-fA-F]+)\b")

VC_MESSAGES = (
    "Failed to sign randao reveal",
    "Failed to request block from beacon node",
    "Failed to propose block",
    "Submitted new block",
    "Could not check if any validator is a sync committee aggregator",
)
BN_MESSAGES = (
    "Building block",
    "Chose payload bid",
    "Finished building block",
    "Could not build block",
    "Fail to build block: could not get parent state",
)


def clean(raw: bytes | str) -> str:
    if isinstance(raw, bytes):
        raw = raw.decode("utf-8", "replace")
    return ANSI.sub("", raw).rstrip("\r\n")


def timestamp(line: str) -> dt.datetime | None:
    match = STAMP.match(line)
    return dt.datetime.fromisoformat(match.group(1).replace("Z", "+00:00")) if match else None


def anchor(path: Path, member: str, line: int, text: str) -> dict[str, object]:
    return {
        "archive": str(path.resolve()),
        "member": member,
        "line": line,
        "timestamp": timestamp(text).isoformat() if timestamp(text) else None,
        "text": text,
    }


def parse_archive(node: int, path: Path) -> dict[str, object]:
    result: dict[str, object] = {
        "node": node,
        "archive": str(path.resolve()),
        "members": [],
        "genesis": [],
        "imports": [],
        "reorgs": [],
        "schedules": [],
        "vc_events": [],
        "bn_events": [],
        "activations": [],
        "votes": [],
    }
    with tarfile.open(path, "r:gz") as archive:
        members = [member for member in archive.getmembers() if member.isfile()]
        result["members"] = [Path(member.name).name for member in members]
        for member in members:
            name = Path(member.name).name
            if name not in ("beacon.log", "validator.log"):
                continue
            source = archive.extractfile(member)
            if source is None:
                continue
            for line_no, raw in enumerate(source, 1):
                line = clean(raw)
                fields = dict(FIELD.findall(line))
                if name == "beacon.log":
                    if "Chain genesis time reached" in line:
                        result["genesis"].append(anchor(path, name, line_no, line))
                    if "Synced new block " in line:
                        match = SLOT.search(line)
                        if match and int(match.group(1)) <= 16:
                            item = anchor(path, name, line_no, line)
                            item.update(slot=int(match.group(1)), root=fields.get("block"))
                            result["imports"].append(item)
                    if "Chain reorg occurred" in line:
                        old_slot, new_slot = fields.get("oldSlot"), fields.get("newSlot")
                        if ((old_slot and old_slot.isdigit() and int(old_slot) <= 16) or
                                (new_slot and new_slot.isdigit() and int(new_slot) <= 16)):
                            item = anchor(path, name, line_no, line)
                            item.update(old_slot=int(old_slot) if old_slot and old_slot.isdigit() else None,
                                        new_slot=int(new_slot) if new_slot and new_slot.isdigit() else None,
                                        old_root=fields.get("oldRoot"), new_root=fields.get("newRoot"),
                                        depth=int(fields["depth"]) if fields.get("depth", "").isdigit() else None)
                            result["reorgs"].append(item)
                    if any(message in line for message in BN_MESSAGES):
                        match = SLOT.search(line)
                        if match and int(match.group(1)) <= 16:
                            item = anchor(path, name, line_no, line)
                            item["slot"] = int(match.group(1))
                            result["bn_events"].append(item)
                    if node in (201, 400) and "Goldfish vote " in line:
                        match = VOTE_SLOT.search(line)
                        if match and int(match.group(1)) in (15, 16):
                            try:
                                validator = int(fields["validator"])
                                seats = int(fields["seats"])
                            except (KeyError, ValueError):
                                continue
                            item = anchor(path, name, line_no, line)
                            item.update(slot=int(match.group(1)), root=fields.get("blockRoot"),
                                        outcome=fields.get("outcome"), reason=fields.get("reason"),
                                        validator=validator, seats=seats,
                                        arrived_ms=int(fields["arrivedMs"]) if fields.get("arrivedMs", "").isdigit() else None,
                                        decided_ms=int(fields["decidedMs"]) if fields.get("decidedMs", "").isdigit() else None)
                            result["votes"].append(item)
                else:
                    if "Validator activated " in line and fields.get("validatorIndex", "").isdigit():
                        result["activations"].append(int(fields["validatorIndex"]))
                    if "Duties schedule" in line and PROPOSER.search(line):
                        match = SLOT.search(line)
                        if match and 1 <= int(match.group(1)) <= 16:
                            item = anchor(path, name, line_no, line)
                            item.update(slot=int(match.group(1)), pubkey=PROPOSER.search(line).group(1).lower())
                            result["schedules"].append(item)
                    if any(message in line for message in VC_MESSAGES):
                        item = anchor(path, name, line_no, line)
                        match = SLOT.search(line)
                        item.update(slot=int(match.group(1)) if match else None,
                                    pubkey=(fields.get("pubkey") or "").lower() or None)
                        result["vc_events"].append(item)
    return result


def discover(directories: list[Path]) -> tuple[dict[int, Path], dict[int, list[Path]], dict[str, int]]:
    choices: dict[int, list[Path]] = collections.defaultdict(list)
    directory_counts = {}
    for directory in directories:
        paths = sorted(directory.glob("round2-prysm-geth-*.tar.gz")) if directory.is_dir() else []
        directory_counts[str(directory.resolve())] = len(paths)
        for path in paths:
            match = ARCHIVE.search(path.name)
            if match:
                choices[int(match.group(1))].append(path)
    # First directory wins. This reproduces the earlier union's compact-archive
    # preference while still filling its seven absent nodes from runs/round2.
    selected = {node: paths[0] for node, paths in choices.items()}
    return selected, choices, directory_counts


def classify_vc(text: str) -> str:
    if "Submitted new block" in text:
        return "submitted"
    if "Failed to sign randao reveal" in text:
        return "randao_failure"
    if "Failed to request block from beacon node" in text:
        return "block_request_failure"
    if "Failed to propose block" in text:
        return "proposal_failure"
    if "can't fetch sync subcommittee index" in text:
        return "sync_index_preflight_failure"
    if "can't sign selection data" in text:
        return "sync_selection_signing_preflight_failure"
    return "other_vc_event"


def classify_bn(text: str) -> str:
    for label, phrase in (
        ("build_started", "Building block"),
        ("payload_selected", "Chose payload bid"),
        ("build_finished", "Finished building block"),
        ("parent_state_failure", "Fail to build block: could not get parent state"),
        ("build_failure", "Could not build block"),
    ):
        if phrase in text:
            return label
    return "other_bn_event"


def sanitize_excerpt(text: str) -> str:
    text = re.sub(r"attesterPubkeys=\[[^]]*\]", "attesterPubkeys=[omitted]", text)
    text = re.sub(r"ptcPubkeys=\[[^]]*\]", "ptcPubkeys=[omitted]", text)
    return text


def iso_min(events: list[dict[str, object]]) -> dict[str, object] | None:
    timed = [event for event in events if event.get("timestamp")]
    return min(timed, key=lambda event: str(event["timestamp"])) if timed else None


def summarize_votes(results: list[dict[str, object]], owners: dict[int, int]) -> tuple[dict, list[dict]]:
    summary: dict[str, dict] = {}
    unique_rows: list[dict] = []
    for result in results:
        observer = int(result["node"])
        if observer not in (201, 400):
            continue
        observer_summary = {}
        for slot in (15, 16):
            records = [row for row in result["votes"] if row["slot"] == slot]
            by_validator = collections.defaultdict(list)
            for row in records:
                by_validator[row["validator"]].append(row)
            duplicate_validators = {str(v): len(rows) for v, rows in by_validator.items() if len(rows) > 1}
            root_conflicts = {str(v): sorted({str(row["root"]) for row in rows})
                              for v, rows in by_validator.items()
                              if len({row["root"] for row in rows}) > 1}
            outcome_conflicts = {str(v): sorted({str(row["outcome"]) for row in rows})
                                 for v, rows in by_validator.items()
                                 if len({row["outcome"] for row in rows}) > 1}
            groups = collections.defaultdict(list)
            # One representative per (validator, root), chosen by earliest line.
            for validator, rows in by_validator.items():
                per_root = collections.defaultdict(list)
                for row in rows:
                    per_root[row["root"]].append(row)
                for root, root_rows in per_root.items():
                    representative = min(root_rows, key=lambda row: int(row["line"]))
                    groups[root].append(representative)
                    unique_rows.append({
                        "observer": observer, "slot": slot, "root": root,
                        "validator": validator, "validator_owner": owners.get(validator),
                        "seats": max(int(row["seats"]) for row in root_rows),
                        "outcomes": ",".join(sorted({str(row["outcome"]) for row in root_rows})),
                        "record_count": len(root_rows), "first_line": min(int(row["line"]) for row in root_rows),
                        "last_line": max(int(row["line"]) for row in root_rows),
                    })
            root_summary = {}
            for root, rows in sorted(groups.items(), key=lambda item: str(item[0])):
                accepted = [row for row in records if row["root"] == root and row["outcome"] == "accepted"]
                owner_counts = collections.Counter(owners.get(int(row["validator"])) for row in rows)
                root_summary[str(root)] = {
                    "unique_validators": len(rows),
                    "deduplicated_seats": sum(int(row["seats"]) for row in rows),
                    "validator_owner_counts": {str(k): v for k, v in sorted(owner_counts.items(), key=lambda x: str(x[0]))},
                    "outcome_record_counts": dict(sorted(collections.Counter(str(row["outcome"]) for row in records if row["root"] == root).items())),
                    "first_record": iso_min([row for row in records if row["root"] == root]),
                    "last_record": max([row for row in records if row["root"] == root and row.get("timestamp")],
                                       key=lambda row: str(row["timestamp"]), default=None),
                    "first_accepted": iso_min(accepted),
                    "last_accepted": max(accepted, key=lambda row: str(row["timestamp"]), default=None),
                    "accepted_decided_ms_range": ([min(int(row["decided_ms"]) for row in accepted if row["decided_ms"] is not None),
                                                    max(int(row["decided_ms"]) for row in accepted if row["decided_ms"] is not None)]
                                                   if any(row["decided_ms"] is not None for row in accepted) else None),
                }
            observer_summary[str(slot)] = {
                "raw_records": len(records), "unique_validators": len(by_validator),
                "duplicate_validators": duplicate_validators,
                "root_conflicts_or_equivocations": root_conflicts,
                "outcome_conflicts": outcome_conflicts,
                "roots": root_summary,
            }
        summary[str(observer)] = observer_summary
    return summary, unique_rows


def compare_vote_observers(rows: list[dict]) -> dict[str, dict]:
    comparison = {}
    for slot in (15, 16):
        maps = {}
        for observer in (201, 400):
            maps[observer] = {
                int(row["validator"]): str(row["root"])
                for row in rows if row["observer"] == observer and row["slot"] == slot
            }
        shared = set(maps[201]) & set(maps[400])
        comparison[str(slot)] = {
            "node_201_unique_validators": len(maps[201]),
            "node_400_unique_validators": len(maps[400]),
            "validator_symmetric_difference": sorted(set(maps[201]) ^ set(maps[400])),
            "shared_validator_root_disagreements": {
                str(validator): {"node_201": maps[201][validator], "node_400": maps[400][validator]}
                for validator in sorted(shared) if maps[201][validator] != maps[400][validator]
            },
        }
    return comparison


def write_tsv(path: Path, fieldnames: list[str], rows: list[dict]) -> None:
    with path.open("w", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames, delimiter="\t", extrasaction="ignore")
        writer.writeheader()
        writer.writerows(rows)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("directories", nargs="+", type=Path)
    parser.add_argument("--genesis", type=dt.datetime.fromisoformat,
                        default=dt.datetime.fromisoformat("2026-09-05T01:30:00+00:00"))
    parser.add_argument("--output-dir", type=Path, default=Path(__file__).resolve().parent)
    parser.add_argument("--workers", type=int, default=4)
    parser.add_argument("--cross-check-union", type=Path)
    args = parser.parse_args()
    genesis = args.genesis if args.genesis.tzinfo else args.genesis.replace(tzinfo=dt.timezone.utc)
    output = args.output_dir.resolve()
    output.mkdir(parents=True, exist_ok=True)

    selected, choices, directory_counts = discover(args.directories)
    expected = set(range(1, 1001))
    if set(selected) != expected:
        missing = sorted(expected - set(selected))
        extra = sorted(set(selected) - expected)
        raise SystemExit(f"archive union is not nodes 1..1000; missing={missing}, extra={extra}")

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as executor:
        futures = {executor.submit(parse_archive, node, path): node for node, path in selected.items()}
        results = [future.result() for future in concurrent.futures.as_completed(futures)]
    results.sort(key=lambda row: int(row["node"]))

    activation_owners: dict[int, int] = {}
    conflicts = []
    for result in results:
        node = int(result["node"])
        for validator in result["activations"]:
            previous = activation_owners.setdefault(int(validator), node)
            if previous != node:
                conflicts.append({"validator": validator, "first_node": previous, "second_node": node})

    schedules = collections.defaultdict(list)
    for result in results:
        for row in result["schedules"]:
            schedules[int(row["slot"])].append({**row, "node": result["node"]})
    owners = {}
    for slot in range(1, 17):
        identities = {(int(row["node"]), str(row["pubkey"])) for row in schedules[slot]}
        if len(identities) != 1:
            raise SystemExit(f"slot {slot} has proposer identities {sorted(identities)}")
        node, pubkey = next(iter(identities))
        evidence = max(schedules[slot], key=lambda row: (str(row.get("timestamp")), int(row["line"])))
        owners[slot] = {"node": node, "pubkey": pubkey, "schedule_evidence": evidence,
                        "schedule_record_count": len(schedules[slot])}

    owner_events = collections.defaultdict(list)
    for result in results:
        node = int(result["node"])
        owned = [slot for slot, owner in owners.items() if owner["node"] == node]
        for row in result["vc_events"]:
            slot = row.get("slot")
            if slot is None and row.get("pubkey"):
                matches = [candidate for candidate in owned if owners[candidate]["pubkey"] == row["pubkey"]]
                slot = matches[0] if len(matches) == 1 else None
            if slot is None and "Could not check if any validator" in str(row["text"]) and row.get("timestamp"):
                when = dt.datetime.fromisoformat(str(row["timestamp"]))
                matches = [candidate for candidate in owned
                           if -0.1 <= (when - (genesis + dt.timedelta(seconds=12 * (candidate + 1)))).total_seconds() <= 0.1]
                slot = matches[0] if len(matches) == 1 else None
            if slot in owners and owners[int(slot)]["node"] == node:
                owner_events[int(slot)].append({**row, "node": node, "component": "VC",
                                                "event_class": classify_vc(str(row["text"]))})
        for row in result["bn_events"]:
            slot = int(row["slot"])
            if slot in owners and owners[slot]["node"] == node:
                owner_events[slot].append({**row, "node": node, "component": "BN",
                                           "event_class": classify_bn(str(row["text"]))})
    for rows in owner_events.values():
        rows.sort(key=lambda row: (str(row.get("timestamp")), str(row["component"]), int(row["line"])))

    imports = collections.defaultdict(list)
    genesis_markers = []
    reorgs = []
    member_counts = collections.Counter()
    for result in results:
        member_counts.update(set(result["members"]))
        genesis_markers.extend({**row, "node": result["node"]} for row in result["genesis"])
        reorgs.extend({**row, "node": result["node"]} for row in result["reorgs"])
        for row in result["imports"]:
            imports[int(row["slot"])].append({**row, "node": result["node"]})

    slot_rows = []
    node_rows = []
    slots_json = []
    for slot in range(17):
        events = imports[slot]
        importing_nodes = sorted({int(row["node"]) for row in events})
        roots = collections.Counter(str(row["root"]) for row in events)
        earliest = iso_min(events)
        outcome = "genesis" if slot == 0 else "import_observed" if events else "no_import_observed"
        slot_data = {
            "slot": slot, "outcome": outcome, "owner": owners.get(slot),
            "importing_node_count": len(importing_nodes), "raw_import_event_count": len(events),
            "observed_roots_by_event": dict(sorted(roots.items())), "earliest_import": earliest,
        }
        slots_json.append(slot_data)
        slot_rows.append({
            "slot": slot, "outcome": outcome,
            "owner_node": owners.get(slot, {}).get("node", ""),
            "owner_pubkey": owners.get(slot, {}).get("pubkey", ""),
            "importing_node_count": len(importing_nodes), "raw_import_event_count": len(events),
            "root_count": len(roots), "roots": ";".join(f"{root}:{count}" for root, count in sorted(roots.items())),
            "earliest_import_time": earliest.get("timestamp", "") if earliest else "",
            "earliest_import_node": earliest.get("node", "") if earliest else "",
            "earliest_import_anchor": (f"{earliest['archive']}::{earliest['member']}:{earliest['line']}" if earliest else ""),
        })
        per_node = collections.defaultdict(list)
        for event in events:
            per_node[int(event["node"])].append(event)
        for result in results:
            node = int(result["node"])
            node_events = per_node[node]
            first = iso_min(node_events)
            if slot == 0:
                node_outcome = "genesis_marker_observed" if result["genesis"] else "genesis_marker_absent"
            else:
                node_outcome = "import_observed" if node_events else "no_import_observed"
            node_rows.append({
                "node": node, "slot": slot, "outcome": node_outcome,
                "selected_archive": result["archive"], "import_event_count": len(node_events),
                "roots": ";".join(sorted({str(row["root"]) for row in node_events})),
                "first_import_time": first.get("timestamp", "") if first else "",
                "first_import_line": first.get("line", "") if first else "",
            })

    if any(imports[slot] for slot in range(1, 15)):
        raise SystemExit("unexpected import observed in slots 1..14")

    vote_summary, vote_rows = summarize_votes(results, activation_owners)
    vote_comparison = compare_vote_observers(vote_rows)
    cross_check = None
    if args.cross_check_union and args.cross_check_union.is_file():
        prior = json.loads(args.cross_check_union.read_text())
        prior_slots = {int(row["slot"]): row for row in prior["slots"] if int(row["slot"]) <= 16}
        differences = []
        for slot in range(17):
            prior_events = [event for rows in prior_slots[slot]["beacon_roots"].values() for event in rows]
            prior_nodes = sorted({int(event["node"]) for event in prior_events})
            current_nodes = sorted({int(event["node"]) for event in imports[slot]})
            if prior_nodes != current_nodes:
                differences.append({"slot": slot, "prior_nodes": prior_nodes, "current_nodes": current_nodes})
        cross_check = {"source": str(args.cross_check_union.resolve()), "node_set_differences": differences}

    inventory_rows = []
    for result in results:
        node = int(result["node"])
        alternatives = [str(path.resolve()) for path in choices[node]]
        inventory_rows.append({
            "node": node, "selected_archive": result["archive"], "all_archive_copies": ";".join(alternatives),
            "member_basenames": ";".join(result["members"]), "genesis_marker_count": len(result["genesis"]),
            "activation_count": len(result["activations"]),
        })

    payload = {
        "scope": {"round": 2, "slots": [0, 16], "genesis": genesis.isoformat(), "expected_nodes": 1000},
        "archive_inventory": {
            "directory_archive_counts": directory_counts, "selected_node_count": len(selected),
            "duplicate_nodes": {str(node): [str(path.resolve()) for path in paths]
                                for node, paths in choices.items() if len(paths) > 1},
            "selected_archive_member_basename_counts": dict(sorted(member_counts.items())),
            "runtime_trace_members": sorted(name for name in member_counts if "trace" in name.lower()),
        },
        "activation_owner_map": {"mapped_validator_count": len(activation_owners), "conflicts": conflicts},
        "genesis_marker_node_count": len({int(row["node"]) for row in genesis_markers}),
        "slots": slots_json,
        "owner_events": {str(slot): owner_events[slot] for slot in range(1, 17)},
        "reorgs_touching_slots_0_16": reorgs,
        "goldfish_votes_15_16": {"observers": vote_summary, "cross_observer": vote_comparison},
        "cross_check": cross_check,
        "uncertainties": [
            "No historical runtime trace member exists in the selected archives.",
            "No-import is bounded to the complete saved 1,000-node archive union.",
            "A Goldfish outcome=accepted line precedes subscriber fork-choice insertion; it proves validation, not retained-store admission.",
        ],
    }
    (output / "census.json").write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n")
    write_tsv(output / "slot_summary.tsv", list(slot_rows[0]), slot_rows)
    write_tsv(output / "node_slot_census.tsv", list(node_rows[0]), node_rows)
    owner_tsv = []
    for slot in range(1, 17):
        for row in owner_events[slot]:
            owner_tsv.append({
                "slot": slot, "node": row["node"], "component": row["component"],
                "event_class": row["event_class"], "timestamp": row["timestamp"],
                "archive": row["archive"], "member": row["member"], "line": row["line"], "text": row["text"],
            })
    write_tsv(output / "owner_events.tsv",
              ["slot", "node", "component", "event_class", "timestamp", "archive", "member", "line", "text"], owner_tsv)
    write_tsv(output / "archive_inventory.tsv", list(inventory_rows[0]), inventory_rows)
    write_tsv(output / "goldfish_votes_15_16_unique.tsv",
              ["observer", "slot", "root", "validator", "validator_owner", "seats", "outcomes", "record_count", "first_line", "last_line"],
              sorted(vote_rows, key=lambda row: (row["observer"], row["slot"], str(row["root"]), row["validator"])))
    vote_group_rows = []
    for observer, by_slot in sorted(vote_summary.items(), key=lambda item: int(item[0])):
        for slot, data in sorted(by_slot.items(), key=lambda item: int(item[0])):
            for root, group in sorted(data["roots"].items()):
                vote_group_rows.append({
                    "observer": observer, "slot": slot, "root": root,
                    "unique_validators": group["unique_validators"],
                    "deduplicated_seats": group["deduplicated_seats"],
                    "validator_owner_counts": json.dumps(group["validator_owner_counts"], sort_keys=True),
                    "outcome_record_counts": json.dumps(group["outcome_record_counts"], sort_keys=True),
                    "first_accepted_time": group["first_accepted"]["timestamp"] if group["first_accepted"] else "",
                    "first_accepted_line": group["first_accepted"]["line"] if group["first_accepted"] else "",
                    "last_accepted_time": group["last_accepted"]["timestamp"] if group["last_accepted"] else "",
                    "last_accepted_line": group["last_accepted"]["line"] if group["last_accepted"] else "",
                    "accepted_decided_ms_range": json.dumps(group["accepted_decided_ms_range"]),
                })
    write_tsv(output / "goldfish_vote_groups.tsv",
              ["observer", "slot", "root", "unique_validators", "deduplicated_seats",
               "validator_owner_counts", "outcome_record_counts", "first_accepted_time",
               "first_accepted_line", "last_accepted_time", "last_accepted_line",
               "accepted_decided_ms_range"], vote_group_rows)

    excerpts = [
        "# Sanitized raw evidence excerpts\n",
        "ANSI color codes and long attester/PTC pubkey arrays are removed. Archive-member line numbers refer to the untouched raw members.\n",
    ]
    for slot in range(1, 17):
        owner = owners[slot]
        excerpts.append(f"\n## Slot {slot}: node {owner['node']} / {owner['pubkey']}\n")
        schedule = owner["schedule_evidence"]
        excerpts.append(f"{schedule['archive']}::{schedule['member']}:{schedule['line']}\n")
        excerpts.append(sanitize_excerpt(str(schedule["text"])) + "\n")
        for row in owner_events[slot]:
            excerpts.append(f"{row['archive']}::{row['member']}:{row['line']} [{row['event_class']}]\n")
            excerpts.append(sanitize_excerpt(str(row["text"])) + "\n")
    excerpts.append("\n## Node 400 reorg anchors\n")
    for row in reorgs:
        if int(row["node"]) == 400 and row.get("old_slot") in (15, 16):
            excerpts.append(f"{row['archive']}::{row['member']}:{row['line']}\n{row['text']}\n")
    (output / "raw_evidence_excerpts.md").write_text("\n".join(excerpts))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
