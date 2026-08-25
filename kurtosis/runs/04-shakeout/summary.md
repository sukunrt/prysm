# Run 04 — shakeout on the fixed tip

Enclave `w4k-01-shakeout`, 2026-08-21 06:12-06:20 IST, torn down after the
checks passed. Images `prysm-*:nxtknlrltzsv`. 2 beacon nodes (both
supernodes) x 64 keys = 128 validators, 12s slots, no late publishers.
`network_params.yaml` is this repo's top-level shakeout file, copied here;
logs in `~/dev/prysm2-run-logs/04-shakeout/`.

What it had to show before the measurement run was worth starting:

| check | result |
|---|---|
| blocks flow | head slot = clock slot at every sample, through slot 24 |
| the EL credits a transfer | 10 transfers, status 1, recipient balance 0.01 ETH |
| Amsterdam's price for a transfer | **204,600 gas** on first touch, 21,000 after |
| blob transactions are included | 12 of 12, status 1, 262,144 blob gas each |
| columns are produced | 896 sidecars written over 7 blob-carrying blocks |
| columns are served | 128 sidecars, indices 0..127, from **both** nodes at slot 19 |
| PTC | `forkchoice_ptc_vote_count` 6,656, payload attestation topic live |
| goldfish | seat fraction 1.00, gate retreats 0 — nothing published late |

The 204,600 gas is the whole story of run 02's dead execution layer: a tool
that hardcodes the pre-Amsterdam 21,000 has its transfer included, charged and
reverted out-of-gas, which reads exactly like "the EL moves no value".
