# Run 13 — a real ePBS builder: buildoor onboards, and then cannot build

Enclave `rounds-13-buildoor`, 2026-08-22, genesis 14:45:20 UTC. Images
`prysm-*:myylspvlkrnq`, EL `ethpandaops/geth:glamsterdam-devnet-8`, builder
`ethpandaops/buildoor:main` (`sha256:852fbb29…`), two instances on
participants 1 and 2, ePBS bidding on, legacy builder API off, lifecycle on.
Topology is run 12's, unchanged: 6 supernodes x 22 keys = 132 validators, 6s
slots, 32-slot epochs, 8-slot rounds, `--goldfish-vote-ledger`. Raw scrapes and
node/builder logs in `~/dev/prysm2-run-logs/13-buildoor/`.

Exploratory. The question was not "is it green" but "does this stack meet a
real builder, and where does it not". It took four launches to get one that
runs; each failure is a finding, and the last one is the interesting one.

## The result in one line

Both builders deposited themselves, were included on chain, and **activated
into the beacon state's builder registry** — and then submitted **zero bids**,
because buildoor drives the EL with `engine_forkchoiceUpdatedV5` and our pinned
geth stops at V4. Per-round finality was untouched throughout: seat fraction
1.00 on every one of 780 window samples.

| | run 12 | **run 13** |
|---|---|---|
| `goldfish_seat_fraction` min/mean/max | 1.00 / 1.000 / 1.00 | **1.00 / 1.000 / 1.00** |
| window samples below 1.00 | 0 of 804 | **0 of 780** |
| finality latency slots, min/mean/max | 16 / 19.4 / 23 | **16 / 19.4 / 23** |
| end of run | head 167, just 19, fin 18 | **head 170, just 20, fin 19** |
| `gate_retreat` / `gate_stop` / `round_proposal_conflict` / `equivocation` | 0 | **0** |
| window reorgs | 0 | **0** |
| builders in the beacon state | 0 | **2, 50 ETH each, active from epoch 1** |
| payload bids from a builder | n/a | **0 of 175 proposals** |

Window slot 40 -> 170 (130 slots, 780 per-node samples), `--skip-slots 40`
because the builders only activate at the epoch-1 boundary.

## What worked, and it is most of it

buildoor speaks to this fork without a single version complaint. It read
`/eth/v1/config/spec` and took `SLOT_DURATION_MS` (not `SECONDS_PER_SLOT`) for
its slot clock — `Timing defaults applied … slot_time_ms=6000`, scaling its
build/bid/reveal offsets to a 6s slot on its own. It resolved our genesis
(`genesis_fork_version=0x10000038`), subscribed to seven SSE topics including
`payload_attributes`, `execution_payload_bid`, `execution_payload_available`
and `proposer_preferences`, and parsed our `"version":"heze"` envelopes
verbatim. 352 head events consumed, zero parse errors.

Then the lifecycle did exactly what it advertises:

    Deposit transaction confirmed  block_number=3  key="#0/b7f216ed"
    Epoch state cached  active_validators=132  builders=2  epoch=1
    Builder key status changed  from=depositing  key="#0/b7f216ed"  to=active

and `/eth/v2/debug/beacon/states/head` agrees — two entries in `builders`,
`balance: 50000000000` each. That is our fork's
`beacon-chain/core/gloas/builder_deposit_request.go` processing EIP-8282
request type 0x03 out of a real EL payload, driven by a real builder, for the
first time. The CL half of builder onboarding is done.

## Finding 1 — the EL pin has no `engine_forkchoiceUpdatedV5`

Every build attempt, twice a slot, on both builders:

    level=error msg="Failed to build payload from attributes"
      candidate=parent_full component=builder-service
      error="forkchoiceUpdated failed: engine API error -32601:
             the method engine_forkchoiceUpdatedV5 does not exist/is not available"

330 of them per builder over the run, 0 payloads built, 0 bids submitted, and
all 175 proposals across the six nodes logged `Chose payload bid
source=self-build valueGwei=0`.

This is the exact mirror of the note in `kurtosis/README.md`. That note says
geth *master* cannot be used because it demands the builder-deposit system
contract that our genesis does not deploy, so the EL is pinned to
`glamsterdam-devnet-8` (366048ea, 2026-08-10), which answers
`forkchoiceUpdatedV4`. buildoor is built against the other end of that gap: it
calls V5. The pin cannot satisfy both, and nothing on our side can bridge it —
this is an EL-version problem, not a fork problem.

Worth recording: that geth binary already carries
`core.ProcessBuilderDepositQueue` and both EIP-8282 addresses. The pinned EL
was never missing builder-deposit *support*; it was missing the contract, which
this run fixed (finding 4). What it is actually missing is the newer engine
method.

## Finding 2 — the package checkout was too old to know buildoor's own flags

`~/dev/ethereum-package` at the commit run 12 used (`a046489`) launches buildoor
with `--builder-api-port`, which `ethpandaops/buildoor:main` rejects:

    Error: unknown flag: --builder-api-port

Upstream removed it in `608794f`. That checkout was 84 commits behind and its
buildoor support is the wrong shape besides — a network-wide `mev_type:
buildoor` running one builder and hanging an inert mev-boost off every
participant. The current schema is per-participant instances
(`additional_services: [buildoor]` + `buildoor_params.instances`), with
`lifecycle` a first-class key. Moved the checkout to `0350d2e`, detached; it
was clean, nothing local was disturbed.

Three harness dependencies survive the jump: `ethereum_genesis_generator_params`
image and `extra_env` still reach `values.env`; `heze_fork_epoch`,
`slot_duration_ms` and `bpo_*` are still native keys; Prysm's binary path is
still hardcoded `/beacon-chain`. One got *better*: `src/vc/prysm.star` now
passes `--beacon-rpc-provider` (gRPC) when a Prysm VC talks to its own Prysm BN,
so the REST-codec problem `validator-entrypoint.sh` exists to work around no
longer arises. **That shim is now dead code and the VC image could drop it.**

## Finding 3 — the newer package needs a script our genesis image lacks

    run_sh(name="generate-validator-ranges", …, image="prysm-genesis-gen:local")
      Caused by: exit code '127' … "bash: /apps/validator-mapping/merge.sh:
      No such file or directory"

The package turns each participant's key range into the validator-ranges file
it hands every buildoor by running a script *out of the genesis-generator
image*. 6.2.0 ships `/apps/validator-mapping/merge.sh`; 6.0.2, which
`prysm-genesis-gen` is built on, does not. Fixed in
`kurtosis/Dockerfile.genesis-gen` with a build stage that copies that one
directory out of 6.2.0. Rebasing the whole image on 6.2.0 was deliberately not
attempted: 6.0.2's CL config template is what carries `SLOTS_PER_ROUND` and the
fork's other patches, and merge.sh is self-contained jq/yq/sed that 6.0.2 has
every tool for.

## Finding 4 — the package stopped exporting SECONDS_PER_SLOT, and the chain halved

The first launch that reached genesis ran at **half speed** and fell a slot
behind the clock every slot:

    Building block  sinceSlotStartTime=42.055s  slot=7

with blocks landing 12s apart and `sinceSlotStartTime` growing by exactly 6s
per slot. `/eth/v1/config/spec` showed the cause:

    SECONDS_PER_SLOT = 12      SLOT_DURATION_MS = 6000

The old package exported `SLOT_DURATION_IN_SECONDS`, which 6.0.2's CL template
substitutes into `SECONDS_PER_SLOT: $SLOT_DURATION_IN_SECONDS`. The new package
dropped that export — generators from 6.1 on derive one from the other, 6.0.2
does not — so the template default of 12 survived next to a 6000ms
`SLOT_DURATION_MS`, and duties ran on a 12s cadence against a 6s clock.

Fixed with one line in `extra_env`: `SLOT_DURATION_IN_SECONDS: 6`. **This is a
trap for any future run that moves the package forward while keeping the 6.0.2
base**, and it is silent: nothing errors, the chain just runs at half rate.

## Finding 5 — the builder-deposit system contract was not in our EL genesis

Before the fix, every 30 seconds, on both builders:

    Builder key deposits deferred  component=lifecycle-manager
      error="failed to read deposit queue fee: builder system contract not
             deployed at 0x0000bFF46984e3725691FA540a8C7589300D8282"

EIP-8282's deposit (0x0000bFF4…, request type 0x03) and exit (0x000064D6…,
0x04) contracts are in `ethereum-genesis-generator` 6.2.0's
`/apps/el-gen/system-contracts.yaml` and absent from 6.0.2's. Fixed without
touching the image, by passing both alloc entries verbatim through the
package's native `network_params.additional_preloaded_contracts` — 6.0.2's
`generate_genesis.sh` accepts either inline JSON or the file path the package
now hands it. `eth_getCode` confirms the code, and the deposits went through on
the next launch.

## Compatibility watch: `*Epoch` fields that carry rounds

**No misread was observed, and the reason is that buildoor never touches the
mislabelled fields.** Its epoch arithmetic runs off `SLOTS_PER_EPOCH` from
`/eth/v1/config/spec` (32, correct) applied to slot numbers, and its logged
epochs (0…5 across 170 slots) are true epochs. It read no checkpoint and logged
no finality. Zero version, fork-digest, slot-range or sync complaints in 352
head events.

That is luck, not safety. The audit of our API surface says these JSON fields
are named `epoch` and carry **rounds** (`round = 8 slots`, not 32), so any
consumer computing `epoch * SLOTS_PER_EPOCH` is wrong by 4x:

- `finality_checkpoints.data.{previous_justified,current_justified,finalized}.epoch`
- `attestation_data.{source,target}.epoch` — so every block, pool attestation,
  attester slashing and `attestation` SSE event
- `/eth/v1|v2/debug/fork_choice`: `{justified,finalized}_checkpoint.epoch`,
  per-node `justified_epoch` / `finalized_epoch` / `unrealized_*`
- the SSE `finalized_checkpoint` event's `epoch`
- `/prysm/v1/beacon/chain_head`: `finalized_epoch`, `justified_epoch`,
  `previous_justified_epoch` (but `head_epoch` there is a real epoch)

The SSE stream is the sharpest edge: `finalized_checkpoint.epoch` is a **round**
while `chain_reorg.epoch` on the same stream is an **epoch**. `SLOTS_PER_ROUND`
*is* published in `/eth/v1/config/spec`, so the information a client needs is
there, but nothing in the payload says which fields need it, and there is not a
comment to that effect anywhere in `api/server/structs/`.

A second hazard, not a naming one: `/eth/v1/validator/duties/attester/{epoch}`
enumerates only the epoch's **first round**
(`beacon-chain/core/helpers/beacon_committee.go:344`), so a third-party VC
reading the standard API would miss 3 of every 4 attestation slots. Prysm's own
VC compensates client-side with `slots.RoundRepeats`. buildoor does not read
duties, so this did not fire here — it would fire for any non-Prysm client.

## What a full integration would need

1. **An EL with `engine_forkchoiceUpdatedV5`.** This is the whole blocker.
   Bumping past `glamsterdam-devnet-8` now costs nothing on the genesis side —
   finding 5's contract injection is exactly what newer geth was demanding, so
   the reason the README gives for the pin is spent. Whoever bumps should
   re-check that Prysm's own fcu call version still matches.
2. **Fold findings 3-5 into the harness properly.** The merge.sh copy is in
   `Dockerfile.genesis-gen`; `SLOT_DURATION_IN_SECONDS` and the EIP-8282 alloc
   are in this directory's `network_params.yaml`. Rebasing
   `prysm-genesis-gen` on 6.2.0 would retire all three at once, at the cost of
   re-applying `patch-generator.sh` to a newer CL template.
3. **Delete the VC REST shim** (finding 2) — upstream fixed it properly.
4. **Say which `epoch` is a round.** Nothing broke here, but the next consumer
   will not be so incurious. Cheapest honest fix is comments in
   `api/server/structs/{other,endpoints_events,endpoints_debug}.go` naming the
   round-valued fields; the real fix is expanding attester duties so the
   standard API is usable by a non-Prysm client at all.
5. Nothing in this run asks for a consensus-code change. Every gap was the EL
   pin, the genesis image, or the package.
