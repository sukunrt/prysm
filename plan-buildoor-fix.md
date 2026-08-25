# plan-buildoor-fix: getting real bids out of buildoor

Written 2026-08-22, after run 13 (`kurtosis/runs/13-buildoor/`). Planning
only — nothing here is executed. The user decides.

## Current state, one paragraph

Run 13 proved builder onboarding end to end (two buildoor instances
deposited via EIP-8282 and activated in our beacon state) and proved
per-round finality is indifferent to builders (seat fraction 1.00
throughout). Zero bids were built because every
`engine_forkchoiceUpdated` buildoor sent was **V5**, which our pinned
geth answers with `-32601 method not found`. Run 13's summary attributed
this to the EL pin being old. **That attribution is wrong**, and the fix
is smaller than the summary implies.

## The latest-devnet answer

`glamsterdam-devnet-8` **is the latest glamsterdam devnet** — the user's
assumption was right. Docker Hub for `ethpandaops/geth` shows
`glamsterdam-devnet-8` (2026-08-13) as the newest devnet tag; the only
newer images are rolling `master` builds (2026-08-20); there is no
devnet-9 and no `plataberget` geth tag (Platåberget, the public Gloas
testnet announced 2026-08-17, is served by the same devnet-8-era stack).
The devnet-8 image already carries the full EIP-8282 builder machinery
(`core.ProcessBuilderDepositQueue`, both system-contract addresses) —
run 13 verified this directly. There is nothing newer to move to, and
nothing newer is needed.

## The real root cause: our Heze is CL-only; buildoor's Heze is not

`engine_forkchoiceUpdatedV5` is not "a newer fcu" — it is **Bogota's**
fcu. Upstream fork naming pairs each CL fork with an EL fork:
Gloas↔Amsterdam (V4), **Heze↔Bogota (V5)**. buildoor's mapping is
explicit and central (`pkg/chain/engineversion.go`):

    case version.DataVersionHeze:
        return enginev.DataVersionBogota, nil

Our fork reuses the (real) name Heze for a **consensus-only** fork: the
design deliberately strips the heze→bogota EL mapping (the
`Dockerfile.genesis-gen` patch; the EL runs Amsterdam rules throughout).
buildoor reads our `/eth/v1/config/spec` fork schedule, sees Heze active,
and — correctly by upstream's convention — speaks Bogota/V5 to an EL
that is, correctly by our design, Amsterdam/V4. Both sides are "right";
the composition can never work. This is why **no geth bump fixes it**:
even a geth with a V5 method expects a chain config that schedules
Bogota, which ours deliberately never does. (Our own beacon is proof of
the correct behavior: `engine_jsonrpc.go` deliberately matches
`version >= Gloas` by range and keeps sending V4 at Heze — "a
consensus-only fork above Gloas leaves the execution client speaking the
Gloas methods.")

## Blocker 2 is not a blocker

`/eth/v1/beacon/execution_payload_envelopes/{block_id}` **already exists
and works** (`rpc/endpoints.go:976` → `handlers_gloas.go:
GetExecutionPayloadEnvelope`, DB fetch + EL reconstruction, JSON+SSZ).
Run 13's 100 404s are the handler's own "envelope not found", logged by
buildoor's head-tracker at **debug** level and tolerated. They are
inherent ePBS timing: buildoor asks at head-event time, the envelope is
revealed later in the slot. No work needed; the confirmation run should
just verify the 404 count drops once bids/reveals actually flow (the
builder's own envelopes will be in the DB).

## The fix: patch buildoor's one mapping line, ship our own image

Exactly the accommodation we already made in the genesis generator,
applied to buildoor:

1. `kurtosis/Dockerfile.buildoor` (new): clone
   `ethpandaops/buildoor` at a pinned commit, apply a one-line patch —
   `DataVersionHeze → enginev.DataVersionAmsterdam` in
   `pkg/chain/engineversion.go` (`EngineVersion`); leave
   `BeaconVersion` alone (Amsterdam→Gloas is fine: our Heze containers
   keep Gloas shapes by the mock rule, and run 13 showed buildoor
   parsing `"version":"heze"` envelopes verbatim). Build the Go binary,
   package like upstream's image.
2. `kurtosis/build-images.sh`: add the buildoor image build
   (`prysm-buildoor:local` + jj-change tag, matching the existing
   pattern).
3. `kurtosis/runs/14-bids/network_params.yaml` (from run 13's):
   `buildoor_params.image: prysm-buildoor:local`. Everything else —
   merge.sh copy, `SLOT_DURATION_IN_SECONDS: 6`, EIP-8282 alloc via
   `additional_preloaded_contracts`, package at `0350d2e` — carries
   forward from run 13 unchanged.
4. No geth change (stay on `glamsterdam-devnet-8`). No beacon API
   change. No consensus change. The run-13 summary's "what a full
   integration would need" item 1 is superseded by this plan.

### Verification items inside the trial (cheap, before/while running)

- After the patch, buildoor's fcu must log V4 and geth must return
  `payloadId`s — visible in the first two slots of a run.
- Watch the version stamps on bid submission and payload reveal: if any
  buildoor code path derives the CL container version via
  `BeaconVersion(engineVersion)` it will say `gloas` where our API
  expects `heze`-tolerant handling. Shapes are identical (mock rule), so
  acceptance is expected; if our API rejects on the version string, that
  is a finding — the fix would be accepting `gloas` as an alias on those
  POSTs (one switch arm), not changing buildoor further.
- PTC activity: with real bids, `payload_attestation` traffic and
  builder pending payments should appear — both were 0 in run 13.

## Risk register

| risk | assessment | mitigation |
|---|---|---|
| Patched-fork drift: our buildoor image diverges from upstream `main` | low — one line, pinned commit | pin + record the commit in the Dockerfile; re-pin deliberately |
| Bid/reveal container version mismatch (`gloas` vs `heze` stamps) | medium — untested territory past the fcu wall | the verification item above; fallback is a one-arm alias in our two POST handlers |
| geth builds a payload but bid value is 0 / payment path breaks | unknown — first time any builder pays | exploratory: findings over fixes, same as run 13 |
| Silent half-speed trap (run-13 finding 4) resurfaces on any package move | known | `SLOT_DURATION_IN_SECONDS: 6` stays pinned in extra_env; check `/eth/v1/config/spec` first slot |
| e2e untouched? | yes — e2e uses the vendored go-geth, not these images; nothing in this plan touches it | — |

## Verification ladder (per doctrine: cheap first, one kurtosis run)

1. Image builds; `docker run prysm-buildoor:local --help` sane.
2. Grep the built binary/source for the patch (`strings` or unit build).
3. **One** kurtosis confirmation run (`rounds-14-bids`, run-13 topology,
   ~4 epochs): acceptance = fcu V4 with payloadIds, ≥1 proposal with
   `builderIndex != self-build`, envelope revealed by the builder, PTC
   payload attestations > 0, **and** seat fraction still 1.00 with
   finality latency 16/19.x/23 (rounds regression guard). Delete the
   enclave after committing the summary.

## Effort estimate

- Dockerfile + patch + build wiring: ~30 min agent work.
- Trial run + analysis + summary: ~45 min (mostly wall clock).
- Contingency if the version-stamp mismatch fires: +30 min (alias arm +
  rerun).

Total: one focused agent session, ~1–2 h wall clock, no consensus risk.

## Decision needed

1. Approve the buildoor-patch approach (vs. dropping the buildoor track;
   the geth-bump route is rejected above as a non-fix)?
2. If the version-stamp mismatch fires mid-trial, pre-authorize the
   one-arm `gloas`-alias fix in our two POST handlers, or stop and
   brief?
3. Run 13's leftover recommendations — genesis-gen rebase onto 6.2.0 and
   deleting the dead VC shim — fold into the same session or defer?
