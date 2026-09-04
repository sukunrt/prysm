# Shadow harness

This directory runs the decoupled fork in [Shadow](https://shadow.github.io),
a discrete-event network simulator. Shadow runs the real binaries. It models
the network: latency, bandwidth and packet loss. It does not model CPU time.

The kurtosis harness in `../kurtosis` runs the same binaries on real docker
networking. Use kurtosis to see real CPU cost. Use Shadow to see the network.

## Prerequisites

| tool | why |
| --- | --- |
| `shadow` 3.3 | the simulator |
| `go`, `cargo`, `docker`, `git` | builds |
| `uv` | Python for `run-shadow-sim.py` |
| `duckdb` | analysis |
| `~/dev/lighthouse/target/release/lighthouse` and `lcli` | the CL bootnode |
| `prometheus` | the monitoring host |

## Build

```sh
./build.py
```

`build.py` clones the third-party sources into `deps/` at pinned refs and
builds every binary into `bin/`:

| binary | source |
| --- | --- |
| `prysm-beacon`, `prysm-validator` | this tree |
| `geth` | go-ethereum at the commit `kurtosis/network_params.geth-master.yaml` pins |
| `bootnode` | go-ethereum v1.13.15, the last release with `cmd/bootnode` |
| `spamoor` | the tag `kurtosis/network_params.yaml` pins |
| `ethshadow` | `sukunrt/ethshadow`, branch `decoupled` |

It also builds the docker image `prysm-genesis-gen:local`: the generator fork
`sukunrt/ethereum-genesis-generator`, branch `decoupled`, plus a static
`prysmctl` from this tree. This is the same image `kurtosis/build-images.sh`
builds. `bin/` and `deps/` are not in version control.

Run `./build.py prysm` after a change to this tree. Run `./build.py` with no
argument after a change to a pin.

## Run

```sh
./run-shadow-sim.py --nodes 10 --validators 200 --supernode-fraction 0.2
```

The script does four steps:

1. It writes `runs/<name>/sim.yaml`, the ethshadow config. Each node gets a
   country from `country_weights.json`. Each country present becomes an
   ethshadow location with latencies from `country_latencies.json`. A random
   sample of the nodes are supernodes. A supernode gets the 1024/1024 Mbit
   class and `--supernode` on its beacon node. A home node gets 25/50 Mbit and
   1 to 3 validators. The supernodes share the remaining validators evenly.
2. It runs `spamoor-premine -inject`. This puts spamoor's child wallets into
   the genesis premine, so spamoor does not fund them on chain.
3. It runs `ethshadow --gen-only`. This writes `runs/<name>/data`: the
   `shadow.yaml`, one directory for each node, and the genesis.
4. It runs `shadow`.

Options:

| option | default | sets |
| --- | --- | --- |
| `--duration` | 120 | seconds of chain time after genesis; genesis is at 300 s |
| `--seed` | 1 | the country and supernode draw |
| `--slots-per-round` | 8 | `SLOTS_PER_ROUND`; the per-slot pool is validators / this |
| `--target-committee-size` | 3000 | `TARGET_COMMITTEE_SIZE`; committees per slot = pool / this, minimum 1 |
| `--aggregators-per-committee` | 64 | `TARGET_AGGREGATORS_PER_COMMITTEE`; expected aggregators in a committee |
| `--subnets` | 1 | `ATTESTATION_SUBNET_COUNT` |
| `--subnets-per-node` | 2 | `SUBNETS_PER_NODE` |
| `--aggregate-due-bps` | 5000 | `AGGREGATE_DUE_BPS_GLOAS`; FFG votes count at this point of the slot |
| `--block-scratch` | 0 | `CONSENSUS_BLOCK_SCRATCH_SPACE`, bytes on each gossiped block |
| `--max-peers` | 99 | `--p2p-max-peers` on each beacon node |
| `--name` | `n<nodes>-v<validators>-s<seed>` | the directory under `runs/` |
| `--gen-only` | | stop before `shadow` |

The chain config keys go to the genesis generator as environment variables.
The generator template has a placeholder for each of them.

Genesis needs at least about 128 validators. Below that `prysmctl` cannot
fill the PTC window and does not return.

## Verify a run

Every run has `--goldfish-vote-ledger` on each beacon node. The ledger lines
and the per-slot summary lines are in
`runs/<name>/data/node<N>/prysm/logs/beacon-chain.log`.

```sh
# the per-slot summary lines, checked against the ledger, on every node
python3 analysis/verify-summary.py runs/<name>/data <nodes>

# every beacon log into parquet tables, with the parse and seat identities
./analysis/logs-to-parquet.py runs/<name>/data runs/<name>/parquet --validate --nodes <nodes>

# one latency table for each object type, ms after slot start
sed 's#{DIR}#runs/<name>/parquet#g' analysis/latency-report.sql | duckdb
```

Set `ffg_due_ms` at the top of `latency-report.sql` to the run's aggregate
due in milliseconds.

Deadlines, as the code counts them:

| object | counted at |
| --- | --- |
| Goldfish head vote | the next slot start, 12 000 ms |
| FFG vote | the aggregate due, `AGGREGATE_DUE_BPS_GLOAS` |
| FFG aggregate | published at the aggregate due |

The monitoring host records prometheus data in
`runs/<name>/data/node<N>monitoring/prometheus`. Serve that directory with
`prometheus --storage.tsdb.path` to query it. The families to look at: `goldfish_vote_seats`,
`goldfish_seat_fraction`, `ffg_vote_seats`, `ffg_expected_seats`, the
`*_arrival_milliseconds` histograms, and the drop counters
`p2p_pubsub_undeliverable_total` and `p2p_pubsub_reject_total`.

## Notes

- The genesis timestamp is pinned with no delay. `prysmctl` adds the delay to
  the EL genesis timestamp before it hashes the block, and geth has already
  stored the block without the delay. With a delay the CL genesis names a
  block geth does not have, and no node ever builds a payload.
- Two committees need a pool of at least twice the target. At 20 000
  validators and 4 slots per round the pool is 5 000, so a target of 2 000
  or 2 500 gives 2 committees of 2 500.
- With `--subnets-per-node` equal to `--subnets`, every node sees every raw
  vote.
- The read buffers of the attestation topics are 5 000 messages, and of the
  Goldfish topic 2 048. The PTC vote topic has the default of 32 and drops
  under load.
