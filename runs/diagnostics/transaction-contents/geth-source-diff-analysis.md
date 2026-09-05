# Geth source changes between the historical builds

Round2's reported Geth revision, `aa1f2fcf`, matches the official
`glamsterdam-devnet-8` branch head checked on 2026-09-08. The user's branch
identification is supported for round2. Its older date does not establish a
wrong devnet build or deployment mistake. See
[geth-branch-provenance.md](geth-branch-provenance.md).

The two round1 builds are newer descendants. Their source differences span
Engine API, execution, RPC, and network-protocol changes, in addition to a Go
compiler change. Earlier references to a "rollback" describe commit direction
only; they do not establish that selecting the devnet branch was an error.

## Identity and direction

The official source objects form one ancestor chain:

`aa1f2fcf512988eb8890d9352e601b898d6fdb2c` (round2)
→ `ff083d4574710ea32b0cf7a6a8ae0f04521d0316` (round1, five saved nodes)
→ `d799b1a3f20fe9d983bfff6cc53fb246bcab1298` (round1, five saved nodes).

| Forward source comparison | Commits | Changed files | Added / removed lines |
| --- | ---: | ---: | ---: |
| Round2 `aa1f2fcf` → round1 `ff083d45` | 49 | 173 | 10,682 / 5,338 |
| Round2 `aa1f2fcf` → round1 `d799b1a3` | 52 | 176 | 10,814 / 5,339 |
| Within round1: `ff083d45` → `d799b1a3` | 3 | 4 | 132 / 1 |

These totals include tests, fixtures, documentation, and build files. The actual
run transition was in the reverse direction: round2 used the devnet branch
revision preceding the intervening changes. Full commit lists, file counts,
and build diffs are linked from
[geth_build_diff_report.md](geth_build_diff_report.md). Runtime identities and
archive-member anchors are in [binary-comparison.md](binary-comparison.md).

## Changes shared by both round1 builds, absent in round2

**RPC request decoding changed.** Commit
[`5515722` / #35518](https://github.com/ethereum/go-ethereum/commit/5515722cbc34bc5fbb6a82fa56ad9763fd661fd8)
reduces repeated JSON scans and copies on HTTP/WebSocket request ingestion.
It reads a complete transport frame, validates JSON once, and passes slices to
argument decoding. Follow-up changes reject `null` for required arguments
(`#35576`) and correctly unescape JSON field names (`#35587`). This code runs
for small Engine requests as well as large transaction-bearing payloads.
It does not change HTTP response writing. The source establishes a parsing
difference, not a measured explanation of these runs' timeouts.

**Amsterdam Engine validation changed.**
[`26d0b21` / #35514](https://github.com/ethereum/go-ethereum/commit/26d0b2171c17339bb8fd164d8ed6830738d3bf13)
makes `targetGasLimit` optional in `engine_forkchoiceUpdatedV4`; round2 still
requires it when payload attributes are supplied. It also assigns BPO3–5 and
Bogota to the Amsterdam Engine methods rather than their older counterparts.
Both runs log Amsterdam active at genesis. The changed allowed-fork list is the
only change to the `GetPayloadV6` method body in this range; the underlying
`miner/` payload-building source has no changes.

**Malformed block-access-list handling changed.**
[`fd07354` / #35580](https://github.com/ethereum/go-ethereum/commit/fd073543c7044fcfa266551b844ba28ff29b233f)
requires a nonzero-length encoded BAL in `newPayloadV5`, rejects Amsterdam-only
fields in `newPayloadV4`, checks block hash before decoding the BAL body, and
returns an invalid-parameters RPC error for malformed BAL encoding rather than
an `INVALID` payload status. A zero-transaction payload still carries an encoded
BAL, so transaction emptiness alone does not remove these validation paths.
Sources: `beacon/engine/types.go:276–398` and `eth/catalyst/api.go:861–955`
at `d799b1a3`.

**Request system contracts must contain code.** The same `#35514` update adds
`empty system contract: no code at %v` in
[`core/state_processor.go:400`](https://github.com/ethereum/go-ethereum/blob/d799b1a3f20fe9d983bfff6cc53fb246bcab1298/core/state_processor.go#L400).
The check covers withdrawal, consolidation, builder-deposit, and builder-exit
queues (EIP-7002/7251/8282). `PostExecution` calls them according to the active
forks, including for blocks with zero transactions (`state_processor.go:177–212`).
Round2 lacks this explicit rejection. The accompanying genesis-allocation helper
refactor concerns built-in dev/test genesis creation; it does not establish the
contents of the historical custom genesis.

**Execution validation and resource accounting changed.**
[`e9e35a4` / #35575](https://github.com/ethereum/go-ethereum/commit/e9e35a42f8213235da1fde4f9ac8f3e9ff666b87)
enforces block gas limits during BAL-driven parallel transaction execution,
including execution/state-gas dimensions and oversized individual transactions.
Its worker count is capped at transaction count, so zero transactions create no
transaction workers (`core/state_processor_parallel.go:304–306`). Other changes
honor the caller's tracing choice before selecting execution strategy (`#35512`),
charge precompile-cache input bytes and entry overhead against its budget
(`#35526`, `#35578`), remove the mainnet-specific EIP-7610 deployment-collision
guard (`#35581`), and add fork checks for access-list/blob/set-code messages
(`#35588`, particularly relevant to calls bypassing normal signer checks).

**Blob transport, receipts, and snap sync changed.** The intervening commits
improve reconstruction/caching when serving blobs to pre-eth/72 peers
(`#35528`, `#35529`, `#35543`), merge repeated cell deliveries and reject
overlapping custody indices (`#35572`), cap blocked blob-pool suffixes at a
default 50% of its storage budget (`#35367`), and avoid advertising incomplete
sparse blobs to peers unable to retrieve them (`#35589`). They also fix an
eth/70 partial-receipt re-request deadlock (`#35537`), reject empty incomplete
receipt responses and correct timing accounting (`#35593`), parallelize snap
response persistence/trie construction (`#35533`), and stop snap storage-range
serving at its limit (`#35299`, the `ff083d45` tip). These require their respective
traffic or sync paths; their presence in the diff does not show they affected
the observed transaction-empty blocks. Production discovery/RLPx code under
`p2p/` and consensus-engine code under `consensus/` are unchanged in this range.

## The smaller difference within round1

`ff083d45` → `d799b1a3` consists of exactly three commits:

1. `8cd4949`: close the freezer if opening the era database fails.
2. [`8a6b06f` / #35633](https://github.com/ethereum/go-ethereum/commit/8a6b06fef836a84850711dfa3799851c4d331fdf):
   after absorbing a child EVM frame, repay outstanding execution-gas borrowing
   from returned state gas. Concretely, transfer `min(StateGas, Spilled)` back
   into execution gas and reduce both state gas and debt by that amount.
   This is a semantic gas-accounting change, not just a build-label change.
3. `d799b1a3`: skip an eth/72 test in `cmd/devp2p`; no production Geth change
   in that tip commit itself.

## Build and observed-run limits

All sampled round1 Geth processes report **Go 1.27.1**, whereas all sampled
round2 processes report **Go 1.26.5**. Source build changes include the Docker
builder image moving from Go 1.26 to 1.27, the module minimum from 1.24 to 1.25,
and dependency updates including `cockroachdb/swiss`. Module minimum, compiler
version, source revision, image digest, and executable checksum are different
identities. The historical logs establish the first-party runtime version
strings; this audit does not have the historical executable checksums or a
byte-for-byte binary comparison.

A fresh scan of all 22 retained `execution.log` members found no occurrences of
`empty system contract`, `system call failed to execute`, `missing target gas
limit`, `missing block access list`, `nil block access list`, `failed to decode
BAL`, or `malformed payload`. This is a bounded negative log check, not proof
that every possible failure was logged. No observed failure is attributed here
to a missing system contract or malformed BAL.

The saved round2 diagnosis already records prompt Geth/proxy responses for
several failed early proposals. Losing a request-decoding optimization does
not establish a Geth response delay in those cases. Empty payloads still use
Engine RPC, BAL validation, and system calls, but they do not demonstrate that
the changed transaction-processing paths were exercised. Source differences
and the compiler change make the two rounds an uncontrolled client comparison;
they do not by themselves identify the cause of their different outcomes.
