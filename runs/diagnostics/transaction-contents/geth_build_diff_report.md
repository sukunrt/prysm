# Historical Geth build source-diff inventory

Round2's `aa1f2fcf` matches the official `glamsterdam-devnet-8` branch head
checked on 2026-09-08. This supports the user's identification of the intended
devnet branch for round2. The older commit date and reverse direction of the
source comparison do not establish a wrong build or deployment error.
[Branch evidence and limits](geth-branch-provenance.md) distinguish the matching
source revision from an unrecorded historical container tag or image digest.

## Resolved builds

The embedded Geth client identifiers resolve in the official
`https://github.com/ethereum/go-ethereum.git` history as follows:

| Run/build | Retained nodes | Full commit | Commit date | Runtime |
|---|---|---|---|---|
| round2 `aa1f2fcf` | all 12 saved round2 nodes | `aa1f2fcf512988eb8890d9352e601b898d6fdb2c` | 2026-08-13 | `Geth/v1.17.6-unstable-aa1f2fcf-20260813/linux-amd64/go1.26.5` |
| round1 `ff083d45` | 50, 201, 300, 400, 500 | `ff083d4574710ea32b0cf7a6a8ae0f04521d0316` | 2026-09-03 | `Geth/v1.17.6-unstable-ff083d45-20260903/linux-amd64/go1.27.1` |
| round1 `d799b1a3` | 1, 2, 3, 700, 900 | `d799b1a3f20fe9d983bfff6cc53fb246bcab1298` | 2026-09-04 | `Geth/v1.17.6-unstable-d799b1a3-20260904/linux-amd64/go1.27.1` |

All three source trees declare Geth `1.17.6-unstable`. The exact commit and
tree-object IDs are recorded in `geth_build_source_inventory.json`.

## History topology and raw source size

The commits are on one linear ancestry chain:

```text
round2 aa1f2fcf -- 49 commits --> round1 ff083d45 -- 3 commits --> round1 d799b1a3
```

| Source comparison (older to newer) | Commits | Diffstat |
|---|---:|---|
| `aa1f2fcf..ff083d45` | 49 | 173 files, 10,682 insertions, 5,338 deletions |
| `aa1f2fcf..d799b1a3` | 52 | 176 files, 10,814 insertions, 5,339 deletions |
| `ff083d45..d799b1a3` | 3 | 4 files, 132 insertions, 1 deletion |

The inventories express the history from the round2 base to each round1 tip.
A round1-to-round2 comparison reverses the same path changes and
insertions/deletions.

The three commits between the two round1 builds are:

1. `8cd4949428da9aca2aa54be9bc805d5c79403066` — `core/rawdb: close freezer if era database fails to open (#35645)`
2. `8a6b06fef836a84850711dfa3799851c4d331fdf` — `core/vm: repay the outstanding debt first after absorbing child frame (#35633)`
3. `d799b1a3f20fe9d983bfff6cc53fb246bcab1298` — `cmd/devp2p: skip eth72 test (#35647)`

Those three touch only `cmd/devp2p/internal/ethtest/suite.go`,
`core/rawdb/chain_freezer.go`, `core/vm/eip8037_test.go`, and
`core/vm/gascosts.go`. There are no build-file changes between the two
round1 tips.

## Build configuration changes from round2 to round1

Both round1 source tips have the same build configuration relative to the
round2 source tip:

| Setting | round2 `aa1f2fcf` | round1 `ff083d45` / `d799b1a3` |
|---|---|---|
| Source version constants | `1.17.6-unstable` | `1.17.6-unstable` |
| Observed compiler/runtime | Go 1.26.5 | Go 1.27.1 |
| `Dockerfile` builder | `golang:1.26-alpine` | `golang:1.27-alpine` |
| root `go.mod` directive | `go 1.24.0` | `go 1.25.0` |
| release workflow Go setup | Go 1.24 | Go 1.27 |

The range also updates `go.mod`, `go.sum`, the keeper module files, and build
checksums. The exact dependency and workflow changes are retained in each
`*.build-config.diff` artifact.

## Exact generated inventories

For each comparison, the directory contains:

- `*.commits.tsv`: every commit in chronological order, with full hash,
  parent, author date, author, and subject;
- `*.paths.tsv`: exact `git diff --name-status --find-renames` output;
- `*.numstat.tsv`: per-path insertion/deletion counts;
- `*.diffstat.txt`: the complete diffstat; and
- `*.build-config.diff`: the exact patch for Docker, Go module, release
  workflow, checksum, and version files; and
- `*.full.diff.gz`: the complete binary-safe source patch from the older
  commit to the newer commit, deterministically compressed with gzip.

`geth_collect_build_diffs.py` reproduces these artifacts from any
go-ethereum checkout containing all three objects. The JSON inventory records
the uncompressed and compressed byte sizes and SHA-256 hashes. All three gzip
files pass `gzip -t`.

## Evidence limit

The retained material includes startup version strings, but no Geth executable
or OCI image digest. Therefore a byte-for-byte executable comparison is not
possible. The embedded hashes establish the source revisions, and the runtime
strings establish the compiler versions and platform. They do not independently
prove that the build trees were clean or capture unrecorded linker flags.
