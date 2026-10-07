# Prysm — Ethereum Consensus Layer Client

Bazel is the first-class build system. Prefer `go test` for unit tests; follow `/test`.

## Skills

The "how" lives in `.agents/skills/` — invoke these instead of running commands by hand:

- `/precheck` — format, gazelle, code generators, build. Run before every commit.
- `/test` — unit tests for affected packages. For any tests you add or modify, run this command.

## Architecture

Main executables in `cmd/` including `beacon-chain`, `validator`.

- `beacon-chain/` — core chain: state transition (`core/`, per fork), block processing & fork choice (`blockchain/`), `state/`, `db/`, `p2p/`, `sync/`, RPC/API
- `validator/` — key management, slashing protection, duties
- `consensus-types/` — shared consensus type definitions
- `proto/` — protobuf defs + generated code
- `api/` — REST + gRPC
- `config/` — chain params (`params/`) and feature flags (`features/`)
- `testing/` — test utilities, spec conformance, end-to-end

Prysm implements the [Ethereum consensus specs](https://github.com/ethereum/consensus-specs/tree/master/specs) (organized by fork - phase0, altair, bellatrix, capella, deneb, electra, fulu, gloas, etc) — they are the source of truth for protocol behavior. Each containing:

- *beacon-chain.md* — state transition / core logic
- *fork-choice.md* — fork choice rules
- *p2p-interface.md* — networking / gossip

## Conventions

- **Always run `/precheck` before every commit — it must pass** (format, gazelle, generators, build).
- Branch from and target `develop`.
- Every PR needs a changelog fragment: `changelog/<github_user>_<branch_name>.md` (managed by `unclog`).
- Keep comments short — one line, no multi-line explanations.
- Verify tests you add or modify with `/test`.

## kurtosis
- With 10 or more participants ethereum-package zero-pads names: services are
  `cl-01-prysm-geth`, `el-01-geth-prysm`; spamoor clients are `01-geth-prysm`.
  Below 10 they are `cl-1-prysm-geth`, `1-geth-prysm`. A `client_group` that
  matches no client makes spamoor log "no clients available" and every block
  is empty. Read the names from the enclave (`kurtosis enclave inspect`,
  spamoor `/api/clients`) before writing them.
- A run is not verified until one slot is checked end to end: transactions
  in the EL block, attestations and `payload_attestations` in the beacon
  block, FFG aggregates and PTC votes in the beacon log. Report the numbers.

## Go toolchain
- `go test` needs Go 1.26 (go.mod pins 1.26.5). With Go 1.27, the test dependency
  `cockroachdb/swiss` fails to build (`undefined: fastrand64`, `hashFn`): its runtime
  hooks build only for `go1.20 && !go1.27`. Run `GOTOOLCHAIN=go1.26.5 go test ...`.
  Bazel uses its own pinned Go and is not affected.
