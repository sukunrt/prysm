# Run 01 — shakeout, 2 nodes, images rebuilt from the tip

Enclave `w3k-01-shakeout`, 2026-08-20 17:58-18:04 IST. Images
`prysm-{beacon-chain,validator,genesis-gen}:zqtmtlrvvnqs`, built by
`kurtosis/build-images.sh` from the current working copy (Goldfish head vote,
the validator timing knobs, the FFG target shift, the round-start proposal
stub). EL `ethpandaops/geth:glamsterdam-devnet-8`.

Point of the run: prove the rebuilt images still start after the rebase, at
the 6s slots the measurement runs use. It is not a measurement.

## Result: startup works, no drift

- 2 nodes, 128 validators (2 x 64), `SECONDS_PER_SLOT: 6`,
  `SLOTS_PER_ROUND: 8`, `AVAILABLE_ATTESTATION_DUE_BPS_HEZE: 2500`.
- Config injection verified inside the enclave
  (`/network-configs/config.yaml`): `GLOAS_FORK_EPOCH: 0`,
  `HEZE_FORK_EPOCH: 0`, `SLOTS_PER_ROUND: 8`,
  `AVAILABLE_ATTESTATION_DUE_BPS_HEZE: 2500`, `SECONDS_PER_SLOT: 6`. The BPS
  line is new in this run: `kurtosis/genesis-gen/patch-generator.sh` now
  injects it the same way it injects `SLOTS_PER_ROUND`, which is what makes
  run 03's sweep possible.
- `beacon_head_slot == beacon_clock_time_slot` at every sample from slot 2 to
  slot 48: no missed slots at 6s.
- 39 blocks proposed over 39 slots, 16.00 attesters/slot, flat over all eight
  round offsets — the same shape as the ethshadow baseline.
- No shim regressions: the beacon entrypoint's identity-port gate and the VC
  entrypoint's `--beacon-rest-api-provider` strip both still work; the only
  errors in the logs are the documented benign ones (unknown config fields,
  deposit-poller chain id).

```
validator client logs read: 2
slots with attestations: 38 (slot 8 to 45)
unaggregated attesters per slot: mean 16.00, min 16, max 16
aggregators per slot: mean 2.00
blocks proposed: 39 over 39 slots
slot % 8 == 0..7: 16.00 attesters/slot on every offset
```

## Numbers (window slot 22 -> 48, 26 slots)

| topic family | msgs/slot in | msgs/slot out | sent bytes/slot | mean msg |
|---|---|---|---|---|
| available_attestation | 61.4 | 64.0 | 12,883 | 201 B |
| beacon_attestation_* | 8.0 | 8.0 | 1,905 | 238 B |
| beacon_aggregate_and_proof | 0.9 | 0.9 | 437 | 463 B |
| beacon_block | 0.5 | 0.5 | 627 | 1291 B |
| sync_committee_* | 176.3 | 181.3 | 38,705 | 213 B |

Two nodes see 64 available attestations from the network each slot and
publish their own 64: 128 per slot, one per validator, no loss. Message sizes
match the baseline's (201 vs 202 B for available attestations, 238 vs 259 B
for unaggregated committee attestations).

Goldfish: seat fraction min 0.55, mean 0.965, max 1.00 over 52 samples;
`gate_stop` 0, `gate_retreat` 0, `late_vote` 0, `round_proposal_total` 7,
`beacon_reorgs_total` 0. Nothing is late in a 2-node run with no late
publishers, which is exactly why the measurement runs need the knob.

Supernode: 128 column subnets subscribed on both nodes, 0.18-0.19 cores per
beacon node. No columns exist because no blob transactions exist — see
`../02-main/summary.md`.

Cost per node at 6s slots: 0.18 cores, ~230 MB. Ten nodes fit on this box
with a wide margin; that is what set run 02's size.

Raw scrape and VC logs: `~/dev/prysm2-run-logs/01-shakeout/`.
