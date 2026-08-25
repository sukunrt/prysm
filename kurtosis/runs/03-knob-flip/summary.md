# Run 03 — the sweep axis: the same run with one value moved

Enclave `w3k-03-knob`, 2026-08-20 18:42-18:53 IST (genesis at unix
1787231531). Identical to run 02 — same images `prysm-*:zqtmtlrvvnqs`, 10
supernodes, 130 validators, 6s slots, 8-slot rounds, the same late publishers
at 3500 bps on every fourth validator index — except for one value:

```
AVAILABLE_ATTESTATION_DUE_BPS_HEZE: 2500  ->  5000
```

At 6s slots that moves the available attestation from 1.5s into the slot to
3.0s, i.e. from *before* the late publishers' 2.1s to *after* it. Prediction:
the same late blocks are now seen by their slot's voters, clear the majority
gate, and cause no head retreat. Three epochs (window slot 32 -> 114, 82
slots) is enough to show it.

## Result: the knob turns the gate retreats off

| | run 02 (2500 bps) | run 03 (5000 bps) |
|---|---|---|
| late-published blocks in window | 43 | 21 |
| `goldfish_gate_retreat` per node | 43 | **0** |
| `beacon_reorgs_total` per node | 43 | **0** |
| `goldfish_gate_stop_total` per node | 44-51 | **0** |
| `goldfish_late_vote_total` | 0 | 0 |
| `goldfish_round_proposal_total` | 45 | 20 |
| `goldfish_seat_fraction` mean | 0.920 | 0.905 |

(The late-block and round-proposal counts differ only because run 03 is 82
slots against run 02's 180; per slot they are the same 0.24 and 0.25.)

Every one of the ten nodes reports zero stops, zero retreats and zero reorgs
over the whole run, with the late publishers still firing at 21 slots. The
sweep axis works end to end: the value reaches the CL and the VC through the
generator image's CL config template, and moving it changes exactly the
behaviour it should.

## Chain

Head slot 114 = clock slot on all ten nodes, 77 blocks over 77 slots, no
missed slots. Justified epoch 2, finalized epoch 0 at the end — expected: the
spec skips justification while the current epoch is at most
`GENESIS_EPOCH + 1`, so the first finalized checkpoint cannot appear before
epoch 4 and this run stops at epoch 3. Run 02, on the same trajectory, was
still at finalized 0 at slot 114 and reached finalized 4 by epoch 6.

## Traffic (unchanged, as intended)

Per slot per node, window means; run 02 in brackets.

| topic family | app msgs/slot | in | recv B | sent B |
|---|---|---|---|---|
| available_attestation | 130.0 [130.0] | 110.6 [111.1] | 197,178 [172,026] | 198,557 [173,282] |
| beacon_attestation_* | 13.4 [14.3] | 11.5 [12.3] | 20,076 [23,724] | 20,133 [23,778] |
| beacon_aggregate_and_proof | 8.1 [8.5] | 7.3 [7.6] | 27,320 [30,209] | 27,320 [30,209] |
| beacon_block | 1.0 [1.0] | 0.9 [0.9] | 3,684 [3,831] | 3,686 [3,831] |

130.0 available attestations per slot per node again — one per validator, no
loss — at the same 214-byte mean size. The byte columns differ by the mesh's
duplicate load (810 vs 691 duplicate messages per slot on the available
attestation topic), which is a gossipsub mesh-shape difference between two
independently bootstrapped networks, not a change in what is published:
16.25 attesters per slot, 8.12 aggregators, 1.0 block, exactly as in run 02.

## Supernode health

Same as run 02: all ten nodes log `Supernode mode enabled. Will custody all
data columns going forward.`, report `custody_group_count: 128`, subscribe to
all 128 column subnets, and log zero data-column or sampling errors. The only
error lines are the documented benign pair (unknown config fields, deposit
poller follow distance). No columns exist to carry — see run 02's anomaly
note on the EL.

Raw scrapes, node logs and the enclave's `config.yaml` (with
`AVAILABLE_ATTESTATION_DUE_BPS_HEZE: 5000` in it) are in
`~/dev/prysm2-run-logs/03-knob/`.
