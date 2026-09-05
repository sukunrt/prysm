#!/usr/bin/env python3
"""Prepare a fresh, private startup3 Kurtosis bundle from existing artifacts."""

import argparse
import json
import os
import re
import shutil
import subprocess
import time
from pathlib import Path


def copy_file(source: Path, destination: Path, mode=None):
    if not source.is_file():
        raise SystemExit(f"missing required input: {source}")
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source, destination)
    if mode is not None:
        destination.chmod(mode)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--prepared", type=Path, default=Path("/tmp/prysm-startup120k-prepared"))
    parser.add_argument("--artifacts", type=Path, default=Path("/tmp/prysm-startup120k-artifacts"))
    parser.add_argument("--shiftgenesis", type=Path, default=Path("/tmp/prysm-startup-shiftgenesis"))
    parser.add_argument("--proposerkeys", type=Path, default=Path("/tmp/prysm-startup-proposerkeys"))
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--genesis-delay", type=int, default=900)
    args = parser.parse_args()
    if args.output.exists() or args.genesis_delay < 120:
        raise SystemExit("output must not exist and genesis-delay must be at least 120 seconds")

    args.output.mkdir(parents=True, mode=0o700)
    config_source = args.artifacts / "config.yaml"
    config = config_source.read_text()
    config, replacements = re.subn(r"(?m)^SLOTS_PER_ROUND:\s*\d+\s*$", "SLOTS_PER_ROUND: 4", config)
    if replacements != 1:
        raise SystemExit("expected exactly one SLOTS_PER_ROUND setting")
    config_path = args.output / "network-configs/config.yaml"
    config_path.parent.mkdir(parents=True)
    config_path.write_text(config)

    genesis_unix = int(time.time()) + args.genesis_delay
    source_genesis = args.artifacts / "genesis/genesis.ssz"
    output_genesis = args.output / "network-configs/genesis.ssz"
    subprocess.run(
        [str(args.shiftgenesis), "-config", str(config_path), "-input", str(source_genesis),
         "-output", str(output_genesis), "-genesis-unix", str(genesis_unix)],
        check=True,
    )
    source_bytes = source_genesis.read_bytes()
    shifted_bytes = output_genesis.read_bytes()
    if len(source_bytes) != len(shifted_bytes) or source_bytes[8:] != shifted_bytes[8:]:
        raise SystemExit("shifted genesis changed fields other than genesis_time")
    output_genesis.chmod(0o600)

    # Keep the execution genesis byte-for-byte unchanged. The consensus shifter
    # changes only genesis_time; changing the EL genesis would create a new EL
    # block hash that is not present in the state's latest execution header.
    copy_file(args.artifacts / "genesis/genesis.json", args.output / "network-configs/genesis.json")

    copy_file(args.artifacts / "jwt/jwtsecret", args.output / "jwt/jwtsecret", 0o600)
    copy_file(args.artifacts / "password/prysm-password.txt", args.output / "prysm-password/prysm-password.txt", 0o600)
    copy_file(args.artifacts / "keymanager/keymanager.txt", args.output / "keymanager/keymanager.txt", 0o600)
    copy_file(args.artifacts / "mnemonics.yaml", args.output / "mnemonics.yaml", 0o600)
    copy_file(args.artifacts / "selection.json", args.output / "selection.json", 0o600)
    copy_file(
        args.prepared / "validator-keys/prysm/direct/accounts/all-accounts.keystore.json",
        args.output / "validator-keys/prysm/direct/accounts/all-accounts.keystore.json",
        0o600,
    )
    copy_file(
        args.prepared / "validator-keys/prysm/keymanageropts.json",
        args.output / "validator-keys/prysm/keymanageropts.json",
        0o600,
    )
    selection = json.loads((args.output / "selection.json").read_text())
    selected = set(selection.get("selected_indices", []))
    check = subprocess.run(
        [str(args.proposerkeys), "-config", str(config_path), "-genesis", str(output_genesis),
         "-first-slot", "1", "-last-slot", "3", "-selected-count", str(len(selected))],
        check=True,
        capture_output=True,
        text=True,
    )
    current = json.loads(check.stdout)
    proposers = current.get("proposers", [])
    if len(proposers) != 3 or any(item.get("index") not in selected for item in proposers):
        raise SystemExit("prepared wallet does not own every proposer in slots 1..3")
    os.chmod(args.output, 0o700)
    print(json.dumps({"event": "bundle_prepared", "genesis_unix": genesis_unix,
                      "seconds_remaining": genesis_unix - int(time.time()), "output": str(args.output),
                      "proposer_slots_verified": [1, 2, 3]}))


if __name__ == "__main__":
    main()
