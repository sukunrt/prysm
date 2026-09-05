#!/usr/bin/env python3
"""Index early proposer duties from Prysm run archives without extracting them."""

from __future__ import annotations

import argparse
import datetime as dt
import gzip
import json
import re
import sys
import tarfile
from pathlib import Path


ANSI = re.compile(r"\x1b\[[0-?]*[ -/]*[@-~]")
ARCHIVE = re.compile(r"(?P<round>round[12])-prysm-geth-(?P<node>\d+)\.tar\.gz$")
STAMP = re.compile(r"\[(?P<stamp>\d{4}-\d\d-\d\d \d\d:\d\d:\d\d(?:\.\d+)?)\]")
SLOT = re.compile(r"\bslot=(?P<slot>\d+)\b")
PROPOSER = re.compile(r"\bproposerPubkey=(?P<key>0x[0-9a-fA-F]+)\b")
UNTIL = re.compile(r"\btimeUntilDuty=(?P<duration>(?:\d+h)?(?:\d+m)?(?:\d+(?:\.\d+)?s)?)\b")
INTERESTING = re.compile(
    r"(?i)(propos|request block|block request|randao|graffiti)"
)
FAILURE = re.compile(r"(?i)(error|failed|failure|deadline|context canceled|context cancelled)")


def parse_duration(value: str) -> dt.timedelta | None:
    match = re.fullmatch(
        r"(?:(?P<h>\d+)h)?(?:(?P<m>\d+)m)?(?:(?P<s>\d+(?:\.\d+)?)s)?", value
    )
    if not match or not any(match.groupdict().values()):
        return None
    return dt.timedelta(
        hours=int(match.group("h") or 0),
        minutes=int(match.group("m") or 0),
        seconds=float(match.group("s") or 0),
    )


def timestamp(line: str) -> dt.datetime | None:
    match = STAMP.search(line)
    if not match:
        return None
    try:
        return dt.datetime.fromisoformat(match.group("stamp"))
    except ValueError:
        return None


def validator_lines(path: Path) -> list[str]:
    # Streaming mode and basename matching avoid extracting or trusting member paths.
    with path.open("rb") as raw, gzip.GzipFile(fileobj=raw) as zipped:
        with tarfile.open(fileobj=zipped, mode="r|") as archive:
            for member in archive:
                if not member.isfile() or Path(member.name).name != "validator.log":
                    continue
                source = archive.extractfile(member)
                if source is None:
                    continue
                return [ANSI.sub("", b.decode("utf-8", "replace")).rstrip("\r\n") for b in source]
    return []


def index_archive(path: Path) -> list[dict[str, object]]:
    identity = ARCHIVE.search(path.name)
    if not identity:
        return []
    lines = validator_lines(path)
    duties: list[dict[str, object]] = []
    for number, line in enumerate(lines, 1):
        if "Duties schedule" not in line:
            continue
        slot_match, proposer_match = SLOT.search(line), PROPOSER.search(line)
        if not slot_match or not proposer_match:
            continue
        slot = int(slot_match.group("slot"))
        if slot not in (1, 2, 3):
            continue
        observed = timestamp(line)
        duration_match = UNTIL.search(line)
        duration = parse_duration(duration_match.group("duration")) if duration_match else None
        duty_time = observed + duration if observed is not None and duration is not None else None
        nearby: list[dict[str, object]] = []
        for candidate_number, candidate in enumerate(lines, 1):
            # Schedule records contain very large attester pubkey lists.
            if "Duties schedule" in candidate:
                continue
            if not INTERESTING.search(candidate):
                continue
            candidate_time = timestamp(candidate)
            explicit_slot = SLOT.search(candidate)
            slot_matches = explicit_slot is not None and int(explicit_slot.group("slot")) == slot
            time_matches = (
                duty_time is not None
                and candidate_time is not None
                and -dt.timedelta(seconds=2) <= candidate_time - duty_time <= dt.timedelta(seconds=30)
            )
            if not slot_matches and not time_matches:
                continue
            nearby.append(
                {
                    "line": candidate_number,
                    "timestamp": candidate_time.isoformat(sep=" ") if candidate_time else None,
                    "failure": bool(FAILURE.search(candidate)),
                    "text": candidate[:500],
                }
            )
        duties.append(
            {
                "round": identity.group("round"),
                "node": int(identity.group("node")),
                "slot": slot,
                "proposerPubkey": proposer_match.group("key").lower(),
                "line": number,
                "dutyTime": duty_time.isoformat(sep=" ") if duty_time else None,
                "actualProposalEvents": nearby,
                "failureCount": sum(bool(event["failure"]) for event in nearby),
                "archive": str(path),
            }
        )
    # Startup commonly logs the same epoch schedule before genesis and again at
    # genesis. Keep the latest copy for one compact record per owned duty.
    deduplicated: dict[int, dict[str, object]] = {}
    for duty in duties:
        slot = int(duty["slot"])
        previous = deduplicated.get(slot)
        if previous is None or int(duty["line"]) > int(previous["line"]):
            deduplicated[slot] = duty
    return list(deduplicated.values())


def archives(directories: list[Path]) -> list[Path]:
    found: dict[str, Path] = {}
    for directory in directories:
        if not directory.is_dir():
            continue
        for path in directory.glob("round[12]-prysm-geth-*.tar.gz"):
            found[str(path.resolve())] = path
    return sorted(found.values(), key=lambda p: p.name)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "directories",
        nargs="*",
        type=Path,
        default=[Path("/tmp/prysm-r2-extra-logs.Rd7MjT")],
    )
    parser.add_argument("--include-existing", action="store_true")
    args = parser.parse_args()
    directories = list(args.directories)
    if args.include_existing:
        directories.extend([Path("runs/round1"), Path("runs/round2")])
    result: list[dict[str, object]] = []
    for path in archives(directories):
        try:
            result.extend(index_archive(path))
        except (OSError, EOFError, tarfile.TarError) as error:
            print(f"warning: {path}: {error}", file=sys.stderr)
    result.sort(key=lambda row: (str(row["round"]), int(row["slot"]), int(row["node"])))
    json.dump(result, sys.stdout, separators=(",", ":"))
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
