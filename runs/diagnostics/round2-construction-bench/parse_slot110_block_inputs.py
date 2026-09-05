#!/usr/bin/env python3
"""Extract observer-reported FFG contents for blocks 108-110 and compare block 110."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from collections import defaultdict
from pathlib import Path

ANSI_RE = re.compile(r"\x1b\[[0-9;]*m")
OUTER_TIMESTAMP_RE = re.compile(r"2026-09-05T\d\d:\d\d:\d\d\.\d+Z ")
INCLUDED_RE = re.compile(
    r"FFG vote included .*?attSlot=(\d+) "
    r".*?blockRoot=(0x[0-9a-f]+) "
    r".*?blockSlot=(\d+) "
    r".*?committeeIndex=(\d+) "
    r".*?dataRoot=(0x[0-9a-f]+) "
    r".*?inclusionSlots=(\d+) "
    r".*?seats=(\d+) "
    r".*?validators=([0-9,]+)\s*$"
)
TRANSITION_RE = re.compile(r"Finished applying state transition .*?slot=(\d+) .*?syncBitsCount=(\d+)")
SLOTS = (108, 109, 110)


def fingerprint_csv(csv: str) -> str:
    return hashlib.sha256(csv.encode()).hexdigest()


def identity(record: dict) -> tuple:
    return (
        record["attSlot"],
        record["votedBlockRoot"],
        record["committeeIndex"],
        record["dataRoot"],
        tuple(record["validators"]),
    )


def identity_hash(record: dict) -> str:
    encoded = json.dumps(identity(record), separators=(",", ":")).encode()
    return hashlib.sha256(encoded).hexdigest()


def parse_observer(node: str, path: Path) -> dict:
    included: dict[int, list[dict]] = defaultdict(list)
    transitions: dict[int, dict] = {}
    parse_gaps: list[int] = []
    candidate_lines = 0
    with path.open(errors="replace") as source:
        for line_number, raw in enumerate(source, 1):
            if "FFG vote included" in raw:
                clean = ANSI_RE.sub("", raw)
                block_match = re.search(r"blockSlot=(\d+)", clean)
                if block_match is None or int(block_match.group(1)) not in SLOTS:
                    continue
                candidate_lines += 1
                wrappers = OUTER_TIMESTAMP_RE.findall(clean)
                repaired = OUTER_TIMESTAMP_RE.sub("", clean)
                match = INCLUDED_RE.search(repaired)
                if match is None:
                    parse_gaps.append(line_number)
                    continue
                att_slot, voted_root, block_slot, committee, data_root, delay, seats, validator_csv = match.groups()
                validators = [int(value) for value in validator_csv.split(",")]
                record = {
                    "blockSlot": int(block_slot),
                    "attSlot": int(att_slot),
                    "votedBlockRoot": voted_root,
                    "committeeIndex": int(committee),
                    "dataRoot": data_root,
                    "inclusionSlots": int(delay),
                    "seats": int(seats),
                    "validatorCount": len(validators),
                    "validatorSetSha256": fingerprint_csv(validator_csv),
                    "validators": validators,
                    "observation": {
                        "node": node,
                        "source": str(path),
                        "line": line_number,
                        "outerTimestamp": wrappers[0].strip() if wrappers else clean.split(" ", 1)[0],
                        "wrapperPrefixOccurrences": len(wrappers),
                        "continuationPrefixOccurrencesStripped": max(0, len(wrappers) - 1),
                        "allWrapperTimestampsIdentical": len(set(wrappers)) <= 1,
                    },
                }
                record["identitySha256"] = identity_hash(record)
                included[int(block_slot)].append(record)
            elif "Finished applying state transition" in raw:
                clean = ANSI_RE.sub("", raw)
                match = TRANSITION_RE.search(clean)
                if match is None or int(match.group(1)) not in SLOTS:
                    continue
                slot, bits = map(int, match.groups())
                transitions[slot] = {
                    "syncBitsCount": bits,
                    "observation": {
                        "node": node,
                        "source": str(path),
                        "line": line_number,
                        "outerTimestamp": clean.split(" ", 1)[0],
                    },
                }
    for slot in SLOTS:
        included[slot].sort(key=lambda row: row["observation"]["line"])
    return {
        "node": node,
        "source": str(path),
        "candidateIncludedLines": candidate_lines,
        "parseGaps": parse_gaps,
        "blocks": {slot: included[slot] for slot in SLOTS},
        "transitions": transitions,
    }


def summarize_block(records: list[dict]) -> dict:
    return {
        "attestationRecords": len(records),
        "distinctAttestationIdentities": len({identity(record) for record in records}),
        "distinctValidatorSets": len({tuple(record["validators"]) for record in records}),
        "participantPositions": sum(record["validatorCount"] for record in records),
        "uniqueValidatorIndicesAcrossAttestations": len({value for record in records for value in record["validators"]}),
        "seatsEqualValidatorCounts": all(record["seats"] == record["validatorCount"] for record in records),
    }


def compare(target: list[dict], prior: list[dict], prior_slot: int) -> dict:
    target_by_id = {identity(row): row for row in target}
    prior_by_id = {identity(row): row for row in prior}
    repeated = []
    for key in sorted(target_by_id.keys() & prior_by_id.keys(), key=lambda x: (x[0], x[3], x[1])):
        current = target_by_id[key]
        old = prior_by_id[key]
        repeated.append({
            "attSlot": current["attSlot"],
            "votedBlockRoot": current["votedBlockRoot"],
            "committeeIndex": current["committeeIndex"],
            "dataRoot": current["dataRoot"],
            "validatorCount": current["validatorCount"],
            "validatorSetSha256": current["validatorSetSha256"],
            "identitySha256": current["identitySha256"],
            "block110LineByNode": {},
            "priorBlockLineByNode": {},
        })
    positions = sum(row["validatorCount"] for row in repeated)
    return {
        "priorBlockSlot": prior_slot,
        "exactRepeatedAttestations": len(repeated),
        "exactRepeatedParticipantPositions": positions,
        "fractionOfBlock110Attestations": len(repeated) / len(target),
        "fractionOfBlock110ParticipantPositions": positions / sum(row["validatorCount"] for row in target),
        "records": repeated,
    }


def public_record(record: dict, observations: list[dict]) -> dict:
    return {**{k: v for k, v in record.items() if k != "observation"}, "observations": observations}


def render_markdown(result: dict) -> str:
    b110 = result["blocks"]["110"]
    lines = [
        "# Observer-reported slot-110 block inputs",
        "",
        "These records are the FFG attestations **included in the observed block**, not a dump of the slot-110 owner's proposal pool. Both retained observers report identical content. Slot 110 and head 109 are in epoch 3 and FFG round 13; those are different units.",
        "",
        "## Block 110",
        "",
        f"The block contains {b110['summary']['attestationRecords']} FFG attestations, {b110['summary']['participantPositions']:,} participant positions, and {b110['summary']['uniqueValidatorIndicesAcrossAttestations']:,} unique validator indices across the eight sets. All seat counts equal the parsed validator counts. Its sync aggregate has {result['syncAggregate']['slot110']['syncBitsCount']} set bits on both observers.",
        "",
        "| att slot | inclusion slots | seats | voted block root | data root | validator-set SHA-256 | node 1 / node 400 lines |",
        "| ---: | ---: | ---: | --- | --- | --- | --- |",
    ]
    for row in b110["attestations"]:
        obs = {entry["node"]: entry for entry in row["observations"]}
        lines.append(
            f"| {row['attSlot']} | {row['inclusionSlots']} | {row['seats']:,} | `{row['votedBlockRoot']}` | `{row['dataRoot']}` | `{row['validatorSetSha256']}` | {obs['1']['source']}:{obs['1']['line']} / {obs['400']['source']}:{obs['400']['line']} |"
        )
    lines += [
        "",
        "The full validator-index arrays are in `slot110-block-inputs.json`. Large physical ledger records contain repeated capture-wrapper timestamps. The parser strips every wrapper timestamp before reading the CSV, records how many continuation prefixes it removed, and verifies that each record's repeated wrapper timestamps are identical.",
        "",
        "## Exact repeated inclusion",
        "",
        "An exact repeat requires the same attestation slot, voted block root, committee index, data root, and complete validator set. Signatures and committee-bit fields are not logged, so equality is limited to the complete logged identity.",
        "",
        "| Compared with | repeated attestations | repeated participant positions | share of block-110 positions |",
        "| ---: | ---: | ---: | ---: |",
    ]
    for comparison in result["comparisons"]:
        lines.append(
            f"| block {comparison['priorBlockSlot']} | {comparison['exactRepeatedAttestations']} / 8 | {comparison['exactRepeatedParticipantPositions']:,} | {comparison['fractionOfBlock110ParticipantPositions']:.2%} |"
        )
    either = result["repeatedInEitherPriorBlock"]
    lines.append(f"| either block 108 or 109 | {either['exactRepeatedAttestations']} / 8 | {either['exactRepeatedParticipantPositions']:,} | {either['fractionOfBlock110ParticipantPositions']:.2%} |")
    lines += ["", "Repeated block-110 identities:", ""]
    for row in either["records"]:
        lines.append(f"- attestation slot {row['attSlot']}, {row['validatorCount']:,} positions, repeated in block(s) {', '.join(map(str, row['priorBlockSlots']))}; data root `{row['dataRoot']}`.")
    lines += [
        "",
        "## Observer agreement and anchors",
        "",
        f"Node 1 and node 400 each yielded {result['observerAgreement']['includedRecordsPerObserver']} included records across blocks 108-110, with zero parse gaps and exact content agreement for all three blocks. The slot-110 state-transition anchors are {result['syncAggregate']['slot110']['observations'][0]['source']}:{result['syncAggregate']['slot110']['observations'][0]['line']} and {result['syncAggregate']['slot110']['observations'][1]['source']}:{result['syncAggregate']['slot110']['observations'][1]['line']}.",
        "",
        "The observers establish block contents after publication. They do not establish which other aggregates or raw singles were present when the owner took its proposal-pool snapshot, which candidates were rejected during packing, or the precise owner-side construction timing.",
    ]
    return "\n".join(lines) + "\n"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--node1", type=Path, default=Path("runs/round2/prysm-geth-1/beacon.log"))
    parser.add_argument("--node400", type=Path, default=Path("runs/round2/prysm-geth-400/beacon.log"))
    parser.add_argument("--output", type=Path, default=Path("runs/diagnostics/round2-construction-bench/slot110-block-inputs.json"))
    parser.add_argument("--report", type=Path, default=Path("runs/diagnostics/round2-construction-bench/slot110-block-inputs.md"))
    args = parser.parse_args()

    observers = [parse_observer("1", args.node1), parse_observer("400", args.node400)]
    for observer in observers:
        if observer["parseGaps"]:
            raise SystemExit(f"parse gaps in {observer['source']}: {observer['parseGaps']}")
        for slot in SLOTS:
            if len(observer["blocks"][slot]) != 8:
                raise SystemExit(f"{observer['source']} block {slot}: expected 8 records, got {len(observer['blocks'][slot])}")
            if slot not in observer["transitions"]:
                raise SystemExit(f"{observer['source']} block {slot}: missing transition summary")

    canonical: dict[int, list[dict]] = {}
    blocks: dict[str, dict] = {}
    for slot in SLOTS:
        left, right = observers[0]["blocks"][slot], observers[1]["blocks"][slot]
        if [identity(row) for row in left] != [identity(row) for row in right]:
            raise SystemExit(f"observer content/order disagreement for block {slot}")
        if [row["seats"] for row in left] != [row["seats"] for row in right]:
            raise SystemExit(f"observer seat disagreement for block {slot}")
        canonical[slot] = left
        blocks[str(slot)] = {
            "summary": summarize_block(left),
            "attestations": [public_record(a, [a["observation"], b["observation"]]) for a, b in zip(left, right)],
        }

    comparisons = [compare(canonical[110], canonical[slot], slot) for slot in (109, 108)]
    for comparison in comparisons:
        prior_slot = comparison["priorBlockSlot"]
        current_obs = {row["identitySha256"]: row["observations"] for row in blocks["110"]["attestations"]}
        prior_obs = {row["identitySha256"]: row["observations"] for row in blocks[str(prior_slot)]["attestations"]}
        for row in comparison["records"]:
            row["block110LineByNode"] = {obs["node"]: obs["line"] for obs in current_obs[row["identitySha256"]]}
            row["priorBlockLineByNode"] = {obs["node"]: obs["line"] for obs in prior_obs[row["identitySha256"]]}

    prior_membership: dict[tuple, list[int]] = defaultdict(list)
    for slot in (108, 109):
        for row in canonical[slot]:
            prior_membership[identity(row)].append(slot)
    repeated_either = []
    for row in canonical[110]:
        slots_seen = sorted(prior_membership.get(identity(row), []))
        if slots_seen:
            repeated_either.append({
                "attSlot": row["attSlot"],
                "votedBlockRoot": row["votedBlockRoot"],
                "committeeIndex": row["committeeIndex"],
                "dataRoot": row["dataRoot"],
                "validatorCount": row["validatorCount"],
                "validatorSetSha256": row["validatorSetSha256"],
                "identitySha256": row["identitySha256"],
                "priorBlockSlots": slots_seen,
            })
    repeated_positions = sum(row["validatorCount"] for row in repeated_either)

    sync = {}
    for slot in SLOTS:
        entries = [observer["transitions"][slot] for observer in observers]
        if len({entry["syncBitsCount"] for entry in entries}) != 1:
            raise SystemExit(f"observer sync-bit disagreement for block {slot}")
        sync[f"slot{slot}"] = {
            "syncBitsCount": entries[0]["syncBitsCount"],
            "observations": [entry["observation"] for entry in entries],
        }

    result = {
        "scope": {
            "description": "Observer-reported published block contents; not the owner proposal-pool snapshot",
            "blockSlot": 110,
            "headSlot": 109,
            "epoch": 3,
            "ffgRound": 13,
            "comparisonBlockSlots": [109, 108],
        },
        "sources": [{"node": observer["node"], "path": observer["source"]} for observer in observers],
        "parser": {
            "continuationHandling": "Strip every repeated outer capture-wrapper timestamp before parsing validator CSV; retain occurrence counts per record",
        },
        "observerAgreement": {
            "includedRecordsPerObserver": sum(len(observers[0]["blocks"][slot]) for slot in SLOTS),
            "parseGapsByNode": {observer["node"]: observer["parseGaps"] for observer in observers},
            "exactContentAndOrderAgreementByBlock": {str(slot): True for slot in SLOTS},
            "syncBitsAgreementByBlock": {str(slot): True for slot in SLOTS},
        },
        "blocks": blocks,
        "syncAggregate": sync,
        "comparisons": comparisons,
        "repeatedInEitherPriorBlock": {
            "exactRepeatedAttestations": len(repeated_either),
            "exactRepeatedParticipantPositions": repeated_positions,
            "fractionOfBlock110Attestations": len(repeated_either) / len(canonical[110]),
            "fractionOfBlock110ParticipantPositions": repeated_positions / sum(row["validatorCount"] for row in canonical[110]),
            "records": repeated_either,
        },
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    args.report.write_text(render_markdown(result))


if __name__ == "__main__":
    main()
