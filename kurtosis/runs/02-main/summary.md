# Run 02 — the measurement run: 10 nodes, 130 validators, 6 epochs

Enclave `w3k-02-main`, 2026-08-20 18:14:35-18:36 IST (genesis at unix
1787229875). Images `prysm-*:zqtmtlrvvnqs`, EL
`ethpandaops/geth:glamsterdam-devnet-8`. `network_params.yaml` in this
directory; raw scrapes, node logs and the enclave's `config.yaml` in
`~/dev/prysm2-run-logs/02-main/`.

Shape: 10 beacon nodes (all supernodes) x 13 keys = **130 validators**,
`SECONDS_PER_SLOT: 6`, `SLOTS_PER_EPOCH: 32`, `SLOTS_PER_ROUND: 8`,
`AVAILABLE_ATTESTATION_DUE_BPS_HEZE: 2500`. Late publishers on:
`--decoupled-late-block-publish-bps=3500
--decoupled-late-block-publish-every-nth=4`.

Measurement window: slot 32 -> 212 (180 slots, 1080s of chain time).

## Finalization

All ten nodes in lockstep, head slot 212 = clock slot at every sample: **no
missed slots**, 181 blocks over 181 slots.

| | e0 | e1 | e2 | e3 | e4 | e5 | e6 |
|---|---|---|---|---|---|---|---|
| finalized epoch (identical on all 10 nodes) | 0 | 0 | 0 | 0 | 2 | 3 | 4 |

End state on every node: head 212, justified 5, finalized 4,
`beacon_reorgs_total` 51 (43 of them inside the window). The ethshadow
baseline finalized 4 / justified 5 over the same number of epochs, at one
epoch per epoch from epoch 4 — the same cadence.

## Attestation reach and per-slot counts

- **130.0 available attestations per slot per node** — one per validator, no
  loss. (The baseline's equivalent number is 128.0 for 128 validators.)
- 16.25 unaggregated committee attesters per slot (min 16, max 17), flat over
  all eight round offsets — 16.00 on offsets 0,1,2,4,5,6 and 17.00 on 3,7,
  which is 130/8 rounded, the same 4x-per-epoch shape as the baseline's exact
  16.00 at 128 validators.
- 8.45 aggregators per slot.

## Traffic vs the plan-5.4 baseline

Per slot per node, window means. Baseline = `decoupled-shadow-sim/data19/
baseline.md` (16 nodes, 128 validators, 12s slots).

Received:

| topic family | in base | in this | recv B base | recv B this |
|---|---|---|---|---|
| available_attestation | 128.0 | 111.1 | 177,353 | 172,026 |
| beacon_attestation_* | 11.2 | 12.3 | 24,225 | 23,724 |
| beacon_aggregate_and_proof | 10.7 | 7.6 | 38,886 | 30,209 |
| beacon_block | 1.0 | 0.9 | 12,789 | 3,831 |

Sent, and the mean message size:

| topic family | out base | out this | sent B base | sent B this | size |
|---|---|---|---|---|---|
| available_attestation | 869.0 | 808.5 | 175,734 | 173,282 | 202/214 B |
| beacon_attestation_* | 92.8 | 91.1 | 24,066 | 23,778 | 259/261 B |
| beacon_aggregate_and_proof | 79.6 | 62.0 | 38,564 | 30,209 | 485/487 B |
| beacon_block | 5.8 | 1.8 | 12,653 | 3,831 | 2172/2130 B |

Traffic matches: the available attestation stream costs 172 KB/slot/node
received against the baseline's 177 KB, on 214-byte messages against 202, and
the committee attestation topics are within 2%. Goldfish changed the head
dynamics, not the message counts, which is what step 6 wanted to see.

The two columns that differ are structural, not behavioural:
`beacon_aggregate_and_proof` and `beacon_block` carry fewer *forwards* here
(62 vs 80, 1.8 vs 5.8 sends per slot) because this run has 10 nodes and 9
peers each where the baseline had 16; the per-message sizes are identical, and
the app-level counters still show exactly 1.0 block and 8.5 aggregates
arriving per slot.

`sync_committee_*` is the heaviest family in this run (297 msgs/slot/node in,
493 KB/slot/node) because 130 validators means the whole set is in the sync
committee. The baseline does not tabulate it. It is noise for these
measurements, not a finding.

## Goldfish, and the late-publish -> gate-retreat evidence

Window deltas, per node:

| metric | value (all 10 nodes) |
|---|---|
| `goldfish_gate_retreat` | 43 |
| `beacon_reorgs_total` | 43 |
| `goldfish_gate_stop_total` | 44-51 (median 47) |
| `goldfish_late_vote_total` | 0 |
| `goldfish_round_proposal_total` | 45 |
| `goldfish_round_proposal_conflict_total` | 0 |
| `goldfish_equivocation_total` | 0 |
| `goldfish_seat_fraction` | min 0.09, mean 0.920, max 1.00 (2080 samples) |

**The pair the user asked for**: `goldfish_gate_retreat` and
`beacon_reorgs_total` are equal, node by node and slot by slot — every reorg
in this run is a gate retreat, and no gate retreat is missing from the reorg
counter. Recording both was the right call: `beacon_reorgs_total` alone would
read as 51 alarming reorgs in a healthy run.

**The late publishers caused them.** The validator clients logged 43 blocks
published late inside the window (2.1s = 3500 bps into a 6s slot, proposer
index a multiple of 4), and there are exactly 43 retreat slots. Matching the
two lists with a one-slot tolerance (the counter is read once per slot, and
the retreat lands at the end of the late slot):

```
late blocks in window 32..211:                    43
gate-retreat slots (per node):                    43
retreats with a late block at slot s or s-1:      43
late blocks with no retreat at s or s+1:           0
```

The first ten late slots and their retreats: late at 34,37,38,42,45,46,51,52,
69,70 -> retreat observed at 35,38,39,43,46,47,52,53,69,71. All ten nodes
retreat at the same slots; the full per-node lists are in
`~/dev/prysm2-run-logs/02-main/` via `kurtosis/summarize.py`.

`gate_stop` exceeds `gate_retreat` by 1-8 per node: some stops leave the head
where it is instead of walking back, which is the expected difference between
the two counters.

**Seat fraction did not worsen at 10 nodes.** Mean 0.920 against the 2-node
e2e's 0.82-0.99 band, and the low samples are the late slots (the gauge reads
0.09-0.10 in the slot whose block is held back, since almost no seat has
anything to vote for yet). `goldfish_late_vote_total` stayed 0 across the
whole run: no available attestation arrived after its slot had ended, so the
missing seats are seats that never published in time, not votes lost in
flight — the same conclusion the e2e reached, now at 10 nodes and 130
validators.

## Supernode health

Per the 2026-08-20 directive, supernodes must work, not merely satisfy the
package's PeerDAS startup gate. Evidence, all ten nodes:

- `Supernode mode enabled. Will custody all data columns going forward.` in
  every beacon log; `/eth/v1/node/identity` reports
  `custody_group_count: 128`, i.e. full custody, and it is in the ENR that
  the other nine read.
- 128 of 128 column subnets subscribed (`p2p_pubsub_topic_active`), and the
  meshes are formed: 1,152 subscribed peers across the 128 column topics
  (9 peers x 128 subnets — every node sees every other node on every column
  subnet) with 1,072 mesh entries.
- **Zero** data-column or sampling errors in 10 x ~20 minutes of beacon logs.
  The only two error lines in the whole run are the documented benign ones:
  the unknown config fields (`EIP7928_*`, `VIEW_FREEZE_CUTOFF_BPS`, ...) at
  startup, and `Beacon node is not respecting the follow distance` from the
  deposit poller, which is the same class as the recorded chain-id mismatch.
- Zero columns were gossiped, stored or served, because **no blob transaction
  can exist on this stack** — see the anomaly below. The column plumbing is
  demonstrably up and connected; it has nothing to carry.

## Cost

0.12-0.14 cores and ~350 MB per beacon node at 6s slots; the whole 32-container
enclave sat well inside this 16-core box, and the head never fell behind the
clock. 16 nodes would fit; 10 was chosen to leave headroom, not because 10 was
the ceiling.

## Anomaly worth a human eye: the EL never credits a transfer

Found while trying to drive blob transactions (a `spamoor` blob spammer, run
`w3k-01b-blobs`, 2 nodes). On `ethpandaops/geth:glamsterdam-devnet-8` with
this fork's genesis:

- spamoor's root wallet (funded in the EL genesis alloc) sent 10 simple 5 ETH
  transfers to its child wallets. All ten were included in EL block 2, which
  is canonical, with `gasUsed = 0x33450` (10 x 21000) and receipts present.
- The recipients' balances are `0x0` at block 2 and at `latest`, on both EL
  nodes.
- The sender was debited **only the gas**: 0.000276 ETH against 50 ETH of
  transfers.

So value transfers execute, charge gas and produce receipts, but move no
value. Nothing on the EL can be funded, so no blob transactions, so no data
columns. Not investigated further (receipt `status` was not captured before
the enclave was torn down); it does not affect these consensus measurements,
which run on empty payloads exactly as the ethshadow baseline did. Whoever
picks this up should start by reading the receipt's `status` field.
