#!/usr/bin/env python3
"""Collect reproducible source-diff inventories for the historical Geth builds."""

from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import subprocess
from pathlib import Path


BUILDS = {
    "round2-aa1f2fcf": "aa1f2fcf512988eb8890d9352e601b898d6fdb2c",
    "round1-ff083d45": "ff083d4574710ea32b0cf7a6a8ae0f04521d0316",
    "round1-d799b1a3": "d799b1a3f20fe9d983bfff6cc53fb246bcab1298",
}
PAIRS = (
    ("round2-aa1f2fcf", "round1-ff083d45"),
    ("round2-aa1f2fcf", "round1-d799b1a3"),
    ("round1-ff083d45", "round1-d799b1a3"),
)
BUILD_PATHS = (
    ".gitea/workflows/release-azure-cleanup.yml",
    ".gitea/workflows/release-ppa.yml",
    ".gitea/workflows/release.yml",
    "Dockerfile",
    "go.mod",
    "go.sum",
    "cmd/keeper/go.mod",
    "cmd/keeper/go.sum",
    "build/checksums.txt",
    "version/version.go",
)


def git(repo: Path, *args: str, check: bool = True) -> str:
    result = subprocess.run(
        ["git", "-C", str(repo), *args],
        check=check,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    return result.stdout


def write(path: Path, content: str) -> None:
    path.write_text(content)


def git_bytes(repo: Path, *args: str) -> bytes:
    return subprocess.run(
        ["git", "-C", str(repo), *args],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    ).stdout


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("repo", type=Path, help="go-ethereum checkout containing all three commits")
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("runs/diagnostics/transaction-contents"),
    )
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)

    inventory: dict[str, object] = {
        "source_remote": "https://github.com/ethereum/go-ethereum.git",
        "builds": {},
        "comparisons": {},
    }
    for name, commit in BUILDS.items():
        resolved = git(args.repo, "rev-parse", f"{commit}^{{commit}}").strip()
        if resolved != commit:
            raise RuntimeError(f"{name}: expected {commit}, resolved {resolved}")
        fields = git(
            args.repo,
            "show",
            "-s",
            "--format=%H%x00%T%x00%P%x00%aI%x00%cI%x00%an%x00%s",
            commit,
        ).rstrip("\n").split("\0")
        inventory["builds"][name] = dict(
            zip(("commit", "tree", "parents", "author_date", "commit_date", "author", "subject"), fields)
        )

    for older_name, newer_name in PAIRS:
        older, newer = BUILDS[older_name], BUILDS[newer_name]
        stem = f"geth_{older_name}_to_{newer_name}"
        ancestor = subprocess.run(
            ["git", "-C", str(args.repo), "merge-base", "--is-ancestor", older, newer]
        ).returncode == 0
        commits = git(
            args.repo,
            "log",
            "--reverse",
            "--format=%H%x09%P%x09%aI%x09%an%x09%s",
            f"{older}..{newer}",
        )
        path_status = git(args.repo, "diff", "--name-status", "--find-renames", older, newer)
        numstat = git(args.repo, "diff", "--numstat", "--find-renames", older, newer)
        diffstat = git(args.repo, "diff", "--stat", "--find-renames", older, newer)
        shortstat = git(args.repo, "diff", "--shortstat", "--find-renames", older, newer).strip()
        build_diff = git(args.repo, "diff", "--find-renames", older, newer, "--", *BUILD_PATHS)
        full_diff = git_bytes(args.repo, "diff", "--binary", "--find-renames", older, newer)

        write(args.output / f"{stem}.commits.tsv", commits)
        write(args.output / f"{stem}.paths.tsv", path_status)
        write(args.output / f"{stem}.numstat.tsv", numstat)
        write(args.output / f"{stem}.diffstat.txt", diffstat)
        write(args.output / f"{stem}.build-config.diff", build_diff)
        compressed_diff = gzip.compress(full_diff, mtime=0)
        (args.output / f"{stem}.full.diff.gz").write_bytes(compressed_diff)
        inventory["comparisons"][f"{older_name}..{newer_name}"] = {
            "older": older,
            "newer": newer,
            "older_is_ancestor": ancestor,
            "commit_count": int(git(args.repo, "rev-list", "--count", f"{older}..{newer}").strip()),
            "merge_base": git(args.repo, "merge-base", older, newer).strip(),
            "shortstat_older_to_newer": shortstat,
            "full_diff_bytes": len(full_diff),
            "full_diff_sha256": hashlib.sha256(full_diff).hexdigest(),
            "full_diff_gzip_bytes": len(compressed_diff),
            "full_diff_gzip_sha256": hashlib.sha256(compressed_diff).hexdigest(),
            "artifacts": {
                "commits": f"{stem}.commits.tsv",
                "paths": f"{stem}.paths.tsv",
                "numstat": f"{stem}.numstat.tsv",
                "diffstat": f"{stem}.diffstat.txt",
                "build_config_diff": f"{stem}.build-config.diff",
                "full_diff_gzip": f"{stem}.full.diff.gz",
            },
        }

    write(
        args.output / "geth_build_source_inventory.json",
        json.dumps(inventory, indent=2, sort_keys=True) + "\n",
    )


if __name__ == "__main__":
    main()
