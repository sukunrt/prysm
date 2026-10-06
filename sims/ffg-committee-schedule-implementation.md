# Staggered FFG committees: implementation plan

Spec: `sims/ffg-committee-schedule.html`. This plan is the implementer's
contract. If the plan and the code disagree, stop and report. Do not guess.

## Summary

One flag, `--ffg-committees-per-subnet-per-slot=X`. When `X >= 2`:

- Beacon node: committees per slot = `X * S` (`S = ATTESTATION_SUBNET_COUNT`).
  No other beacon node code changes.
- Validator client: the committee at index `c` has position `i = c / S`. Its
  members wait until `B[i]` plus jitter, then run the existing attest path.
  Its aggregators wait until `B[i+1]`, then run the existing aggregate path.
- `B[i] = floor(i * D / X)` ms, `i = 0..X`. `D` is the existing aggregate
  due. The last aggregate is at `B[X] = D`.
- Harness: forwards the flag to all beacon nodes and validator clients.

`X <= 1` (default 1) keeps all current behavior. Do not change the legacy
code path.

## Out of scope

The previous attempt added all of these. Do not add any of them:

- Deadlines, cutoffs or early/late checks in the attestation pools, RPC or
  REST handlers, propose paths, pending replay, gossip validation, subscribers
  or P2P broadcast. Late votes follow the existing gossip slot window.
- New pool APIs (`SaveFFGSingle*`, `AddFFGSingle*`, `FFGSingleEligible*`),
  snapshot or copy-on-write pool storage, drop ledgers.
- Validation functions, caps or limits (`validateFFGElectorate`, `2048`,
  `ValidateFFGCommitteeConfig`, a `64` check, slot-time checks). No error for
  `X = 0`.
- Global clock or genesis state in `config/features`. Timing logic in
  `config/features`.
- A sender skip when the send is late.
- Changes to the VC attestation data cache (`validator/client/validator.go`,
  `getAttestationData`).
- New logs, metrics or diagnostics. Changes to `ffg_summary.go` or
  `vote_seats.go`.
- Changes to `ListBeaconCommittees`
  (`beacon-chain/rpc/prysm/v1alpha1/beacon/committees.go`).
- Changes to Goldfish (available attestation) votes, PTC, sync committees,
  blocks, payloads, blobs or aggregator selection.
- Changes to analysis scripts in `shadow/analysis`.
- Refactors or fixes not needed for this feature.

## Steps

Make one `jj` commit per step, with the tests of that step. Commit only the
files of the step: `jj commit -m "<msg>" <paths>`. Other working-copy edits
(for example under `sims/`) are not yours; leave them uncommitted and check
with `jj st` after each commit. No `Co-Authored-By`/`Assisted-By` trailers
or session links in descriptions.

### 1. Flag and committee count

- `config/features/flags.go`: add `FFGCommitteesPerSubnetPerSlot`, a
  `cli.Uint64Flag`, name `ffg-committees-per-subnet-per-slot`, default `1`,
  near the other decoupled FFG flags (`:244-270`). Add it to
  `ValidatorFlags` (`:303`) and to `BeaconChainFlags` (`:331`). In the usage
  text of `decoupledFFGVoteJitter`, say that it is also read when `X >= 2`.
  In the usage text of `DecoupledFFGVoteSpread`, say that it is ignored
  when `X >= 2`.
- `config/features/config.go`: add a `uint64` field to `Flags` near the
  decoupled FFG fields (`:89-109`). Set it in `ConfigureBeaconChain`
  (`:210`) and in `ConfigureValidator` (`:368`). No validation.
- `beacon-chain/core/helpers/beacon_committee.go:55` `SlotCommitteeCount`:
  when `X >= 2`, use `X * AttestationSubnetCount` in place of
  `activeValidatorCount / SlotsPerRound / TargetCommitteeSize`. Keep the
  existing clamp to `[1, MaxCommitteesPerSlot]`. Committee construction,
  the committee cache, duties, gossip index checks, the state transition and
  the subnet mapping all use this function.
- Other callers get the new count without change: `computePTC`
  (`beacon-chain/core/gloas/payload_attestation.go:141`), gossip scoring
  (`beacon-chain/p2p/gossip_scoring_params.go:665`) and REST
  `GetCommittees` (`beacon-chain/rpc/eth/beacon/handlers.go:1226`). Do not
  change them or add special cases. The PTC does not change: for any count,
  the committees of a slot together are the same range of the shuffled list,
  in the same order. The aggregate scoring params scale with `X`; this is
  expected.
- Add `//config/features:go_default_library` to the `go_library` deps of
  `beacon-chain/core/helpers/BUILD.bazel`. Also add it to the `go_test`
  deps of each package whose tests now import it.

### 2. Validator client schedule

- `validator/client/wait_helpers.go`: add one small function that returns
  `B[i]` and `B[i+1]` for a slot and a committee index. `D` uses the same
  component choice as `waitUntilAggregateDue` (`aggregate.go:234`):
  `AggregateDueBPSGloas` from the Gloas fork epoch, else `AggregateDueBPS`.
  Use integer milliseconds: `B[i] = i * D / X`. Compute each boundary from
  `i`, not by adding steps. Wait to absolute times as `waitFFGVoteSpread`
  does (`:119-140`). Do not use basis points for `B[i]`.
- `validator/client/attest.go`, `SubmitAttestation`:
  - When `X >= 2`, skip the existing first wait (`:39-43`). After the duty
    lookup and its error and empty-committee returns (`:64-76`), before
    `getAttestationData` (`:80`), wait until
    `slot_start + B[i] + ffgVoteJitter(...)`. Then continue with the
    existing path (get data, sign, publish).
  - When `X >= 2`, skip the spread waits (`:154-158`, `:169-173`).
  - The attester lock is per pubkey (`:45-60`). It can stay held during
    the wait.
- `validator/client/aggregate.go:51`: when `X >= 2`, wait until
  `slot_start + B[i+1]` in place of `waitUntilAggregateDue`. Keep the rest
  of the aggregate path as is.

### 3. Harness and docs

- `shadow/run-shadow-sim.py`: add `--ffg-committees-per-subnet-per-slot`
  (int, default 1) near `--subnets` (`:75`). When the value is greater than
  1, append the flag to `beacon_args` (`:146`) and `vc_args` (`:148`). When
  it is 1, add nothing, so older binaries still start. Fix the committee
  print-out (`:258-263`): when `X > 1`, show `min(64, X * subnets)`
  committees. Update the `--target-committee-size` help (`:72`): with
  `X > 1`, committees per slot = `min(64, X * subnets)`. Change no other
  simulation parameter.
- `shadow/README.md`: add a row to the flag table (near `:77`). Update the
  `--target-committee-size` row (`:73`) as in the help text. Change the FFG
  aggregate row (`:115`) to: `| FFG aggregate | published at the aggregate
  due; with X >= 2, position i publishes at (i + 1) * D / X ms, the last at
  the aggregate due |`. Keep the FFG vote row: votes still count at the
  aggregate due.

## Tests

Follow `/test` (`go test -mod=readonly -count=1` on the changed packages).
Tests that set the flag must reset the features config. Committee tests must
clear the committee cache, because the cache key is the seed only.

1. Flag: `ConfigureBeaconChain` and `ConfigureValidator` set the field to 3
   with `--ffg-committees-per-subnet-per-slot=3`, and to 1 without it
   (pattern: `config/features/config_test.go:190`). For `X = 0` and `X = 1`,
   `SlotCommitteeCount` equals the old formula. Existing VC wait tests (they
   run with `X = 0`) pass unchanged.
2. `S = 2`, `X = 3`: six committees; subnets give `0,2,4` and `1,3,5`.
   Cached and uncached committees agree. Each active validator is in
   exactly one committee per round. `computePTC` gives the same PTC for a
   slot with `X = 3` and `X = 1`.
3. Offsets match the spec table for `D = 6000` and `D = 9000`, `X = 2,3,4`.
   `X = 7`, `D = 6000`: `B = 0, 857, 1714, 2571, 3428, 4285, 5142, 6000`.
   `X = 7`, `D = 9000`: `B = 0, 1285, 2571, 3857, 5142, 6428, 7714, 9000`.
4. VC, `X = 3`, a short `D` from the test config, jitter smaller than
   `B[1]`. Run it with `--decoupled-ffg-vote-at-slot-start` and
   `--decoupled-ffg-vote-spread` both set, and with both unset and no block
   event. A vote at position `i` does not request `AttestationData` before
   `B[i]`, and it publishes in `[B[i], B[i+1])`. An aggregator at position
   `i < X-1` requests the aggregate in `[B[i+1], D)`. Duties at positions 0
   and 1 on one VC: position 0 publishes before `B[1]`.
5. Harness: do not run the script. Load `shadow/run-shadow-sim.py` with
   `importlib.util.spec_from_file_location`, set `sys.argv`, then call
   `parse_args()`, `place(args, random.Random(args.seed))` and
   `sim_config(args, country, supers, vals)`. For `X = 3`, the `extra_args`
   of `prysm`, `prysm_super` and every `prysm_vc_*` client have the flag.
   For `X = 1`, none has it. Write nothing under `shadow/runs`. This is a
   one-off check; add no test file for it.

## Checks before each commit

Follow `/precheck` with these changes. This repo uses jj, so list changed
files with `jj diff --name-only` in place of `git diff`. Bazel runs as
`bazelisk`. For the build step, run `go build ./cmd/beacon-chain
./cmd/validator` (one command, so it writes no binary) in place of
`make build`, because `make build` overwrites `dist/`. Do not run
`shadow/build.py` or write to `shadow/bin`. If a step cannot run, report
it. Add no changelog fragment.

Do not run simulations, contact remote hosts or push.

## Report

Changed files, commits, exact test and check commands and results, and any
open point.
