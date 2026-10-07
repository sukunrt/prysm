#!/usr/bin/env -S uv run --script
# /// script
# dependencies = ["pyyaml"]
# ///
"""Run a Shadow simulation of the decoupled fork.

    ./run-shadow-sim.py --nodes 5 --validators 128 --supernode-fraction 0.2

Steps, in order:

1. Write the ethshadow config, runs/<name>/sim.yaml. The topology is in it:
   every node gets a country sampled from country_weights.json, and every
   country present becomes an ethshadow location with latencies from
   country_latencies.json. Supernodes are a uniform sample of the nodes. They
   get the 1024/1024 Mbit class, --supernode on the beacon node, and the
   validators the home nodes do not take: a home node runs 1 to 3 validators,
   the rest is split evenly over the supernodes.
2. spamoor-premine -inject: spamoor's child wallets into the genesis premine.
3. ethshadow --gen-only: runs/<name>/data with shadow.yaml, node dirs, genesis.
4. shadow.

Binaries come from bin/ next to this file, built by build.py: prysm-beacon,
prysm-validator, geth, bootnode, spamoor, ethshadow, plus the genesis image
prysm-genesis-gen:local. The CL bootnode is lighthouse from ~/dev/lighthouse.
"""

import argparse
import json
import math
import random
import subprocess
import sys
from pathlib import Path

import yaml

HERE = Path(__file__).resolve().parent
BIN = HERE / "bin"
LIGHTHOUSE = Path.home() / "dev/lighthouse/target/release"
FALLBACK_LATENCY_MS = 100
INFRA_LOCATION = "infra-germany"
INFRA_COUNTRY = "germany"
SIM_EPOCH = 946684800  # Shadow's clock starts at 2000-01-01
GENESIS_AT_S = 300
SLOT_S = 12
GAS_LIMIT = 200_000_000
PREMINE_BEGIN = "    # BEGIN spamoor child wallets"
PREMINE_END = "    # END spamoor child wallets"

# ethshadow's DEFAULT_MNEMONIC accounts 0 and 2; the generator premines them.
EOATX_KEY = "0x306cb89d3f8c1da466d8c2762b600b98e911dd45d0daa885c073ac94f45ded31"
BLOBS_KEY = "0x47c8d566df4d9d9fa45a245901ec0fe18bc21757bcdf54a9902014e9e883ab7a"


def slug(country):
    return country.replace(" ", "-")


def parse_args():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--nodes", type=int, required=True)
    ap.add_argument("--validators", type=int, required=True,
                    help="total; prysmctl needs at least 128 to build the genesis")
    ap.add_argument("--supernode-fraction", type=float, required=True)
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--duration", type=int, default=120, help="seconds after genesis")
    ap.add_argument("--block-scratch", type=int, default=0,
                    help="CONSENSUS_BLOCK_SCRATCH_SPACE bytes on every gossiped block")
    ap.add_argument("--max-peers", type=int, default=99)
    ap.add_argument("--slots-per-round", type=int, default=8,
                    help="SLOTS_PER_ROUND; the per-slot pool is validators / this")
    ap.add_argument("--target-committee-size", type=int, default=3000,
                    help="TARGET_COMMITTEE_SIZE; committees per slot = pool / this, min 1; "
                         "with X > 1, min(64, X * subnets)")
    ap.add_argument("--aggregators-per-committee", type=int, default=64,
                    help="TARGET_AGGREGATORS_PER_COMMITTEE; expected aggregators in a committee")
    ap.add_argument("--subnets", type=int, default=1, help="ATTESTATION_SUBNET_COUNT")
    ap.add_argument("--ffg-committees-per-subnet-per-slot", type=int, default=1,
                    help="X; with X > 1, the X committees of a subnet send one after the other")
    ap.add_argument("--ffg-vote-spread", action="store_true",
                    help="--decoupled-ffg-vote-spread on each validator client")
    ap.add_argument("--subnets-per-node", type=int, default=2, help="SUBNETS_PER_NODE")
    ap.add_argument("--aggregate-due-bps", type=int, default=5000,
                    help="AGGREGATE_DUE_BPS_GLOAS; FFG votes count at this point of the slot")
    ap.add_argument("--eoa-txs", type=int, default=8, help="calldata transfers per slot")
    ap.add_argument("--eoa-bytes", type=int, default=16384, help="calldata bytes per transfer")
    ap.add_argument("--blob-txs", type=int, default=2,
                    help="blob transactions per slot, 3 sidecars each; 0 runs no blob spammer")
    ap.add_argument("--name", default=None, help="run dir name under runs/")
    ap.add_argument("--gen-only", action="store_true", help="stop before shadow")
    return ap.parse_args()


def place(args, rng):
    """Country, class and validator count for every node."""
    weights = json.loads((HERE / "country_weights.json").read_text())
    countries = list(weights)
    country = [rng.choices(countries, weights=[weights[c] for c in countries])[0]
               for _ in range(args.nodes)]
    nsuper = round(args.nodes * args.supernode_fraction)
    supers = set(rng.sample(range(args.nodes), nsuper))
    vals = [0 if i in supers else rng.randint(1, 3) for i in range(args.nodes)]
    rest = args.validators - sum(vals)
    if nsuper == 0 and rest != 0:
        sys.exit(f"{args.validators} validators but the {args.nodes} home nodes take "
                 f"{sum(vals)}; add supernodes or change the count")
    if rest < nsuper:
        sys.exit(f"{rest} validators left for {nsuper} supernodes after the home nodes "
                 f"took {sum(vals)}")
    order = sorted(supers)
    rng.shuffle(order)
    for k, i in enumerate(order):
        vals[i] = rest // nsuper + (1 if k < rest % nsuper else 0)
    return country, supers, vals


def locations(present):
    latencies = json.loads((HERE / "country_latencies.json").read_text())
    loc = {f"n-{slug(c)}": c for c in sorted(present)}
    loc[INFRA_LOCATION] = INFRA_COUNTRY

    def lat(a, b):
        return latencies.get(a, {}).get(b, FALLBACK_LATENCY_MS)

    return loc, {
        name: {
            "latency_to": {o: f"{lat(c, oc)}ms" for o, oc in loc.items()},
            "packet_loss_to": {o: 0.0 for o in loc},
        }
        for name, c in loc.items()
    }


def eoa_gas_limit(nbytes):
    return math.ceil((21000 + 64 * nbytes) * 1.07 / 50000) * 50000


def sim_config(args, country, supers, vals):
    loc, loc_yaml = locations(set(country))
    country_loc = {c: n for n, c in loc.items()}

    def infra(tag, kind, client):
        return {"location": INFRA_LOCATION, "reliability": "super", "tag": tag,
                "clients": {kind: client}}

    nodes = [{"location": INFRA_LOCATION, "reliability": "super", "tag": "boot",
              "clients": {"el": "geth_bootnode", "cl": "lighthouse_bootnode"}}]
    for i in range(args.nodes):
        sup = i in supers
        nodes.append({
            "location": country_loc[country[i]],
            "reliability": "super" if sup else "staker",
            "clients": {"el": "geth", "cl": "prysm_super" if sup else "prysm",
                        "vc": f"prysm_vc_{vals[i]}"},
        })
    nodes.append(infra("monitoring", "monitoring", "prometheus"))
    nodes.append(infra("eoaspam", "spammer", "spamoor_eoatx"))
    if args.blob_txs:
        nodes.append(infra("blobspam", "spammer", "spamoor_blobs"))

    beacon_args = (f"--p2p-max-peers {args.max_peers} --goldfish-vote-ledger "
                   "--pprof --pprofaddr=0.0.0.0")
    vc_args = ("--decoupled-ffg-vote-at-slot-start --enable-beacon-rest-api "
               "--beacon-rest-api-provider=http://127.0.0.1:31001 "
               f"--suggested-gas-limit={GAS_LIMIT}")
    if args.ffg_committees_per_subnet_per_slot > 1:
        ffg = f" --ffg-committees-per-subnet-per-slot={args.ffg_committees_per_subnet_per_slot}"
        beacon_args += ffg
        vc_args += ffg
    if args.ffg_vote_spread:
        vc_args += " --decoupled-ffg-vote-spread"
    clients = {
        "prysm": {"type": "prysm", "executable": str(BIN / "prysm-beacon"),
                  "lower_target_peers": False, "extra_args": beacon_args},
        "prysm_super": {"type": "prysm", "executable": str(BIN / "prysm-beacon"),
                        "lower_target_peers": False,
                        "extra_args": beacon_args + " --supernode"},
        "lighthouse_bootnode": {"type": "lighthouse_bootnode",
                                "executable": str(LIGHTHOUSE / "lighthouse"),
                                "lcli_executable": str(LIGHTHOUSE / "lcli")},
        "geth": {"type": "geth", "executable": str(BIN / "geth"),
                 "extra_args": f"--miner.gaslimit={GAS_LIMIT}"},
        "geth_bootnode": {"type": "geth_bootnode", "executable": str(BIN / "bootnode")},
        # Calldata transfers cost 21000 + 64 x bytes gas on this chain; 7 % over, rounded up.
        "spamoor_eoatx": {
            "type": "spamoor", "executable": str(BIN / "spamoor"), "scenario": "eoatx",
            "throughput": args.eoa_txs, "max_pending": args.eoa_txs, "max_wallets": 8,
            "el_first": 0, "el_count": args.nodes,
            "extra_args": f"--seed {args.name}-eoa --data random:{args.eoa_bytes} "
                          f"--gaslimit {eoa_gas_limit(args.eoa_bytes)} "
                          "--basefee 100 --rebroadcast 0",
            "private_key": EOATX_KEY, "start_time": f"{GENESIS_AT_S + SLOT_S}s",
        },
        # Blob transactions of three sidecars each: 6 blobs a block at the default 2.
        "spamoor_blobs": {
            "type": "spamoor", "executable": str(BIN / "spamoor"), "scenario": "blobs",
            "throughput": args.blob_txs, "max_pending": args.blob_txs, "max_wallets": 6,
            "el_first": 0, "el_count": args.nodes,
            "extra_args": f"--seed {args.name}-blob --sidecars 3 "
                          f"--fulu-activation {SIM_EPOCH + GENESIS_AT_S} --rebroadcast 0",
            "private_key": BLOBS_KEY, "start_time": f"{GENESIS_AT_S + SLOT_S}s",
        },
    }
    if not args.blob_txs:
        del clients["spamoor_blobs"]
    for k in sorted(set(vals)):
        clients[f"prysm_vc_{k}"] = {"type": "prysm_vc",
                                    "executable": str(BIN / "prysm-validator"),
                                    "validators": k, "extra_args": vc_args}

    return {
        "general": {
            # Without it the sim clock froze at 43 s on an earlier run.
            "model_unblocked_syscall_latency": True,
            "stop_time": f"{GENESIS_AT_S + args.duration}s",
            "progress": True,
            "heartbeat_interval": "1m",
        },
        # ethshadow defaults the memory manager on; it SIGSEGVs Go binaries at exec.
        "experimental": {"use_memory_manager": False},
        "ethereum": {
            "validators": args.validators,
            "use_builtin_locations": False,
            "use_builtin_reliabilities": False,
            "genesis": {
                "gaslimit": GAS_LIMIT,
                "generator_image": "prysm-genesis-gen:local",
                "electra_epoch": 0,
                "fulu_epoch": 0,
                "gloas_epoch": 0,
                "extra_env": {
                    # The EL and CL genesis are both at GENESIS_AT_S.
                    "GENESIS_TIMESTAMP": str(SIM_EPOCH + GENESIS_AT_S),
                    "GENESIS_DELAY": "0",
                    "HEZE_FORK_EPOCH": "0",
                    "HEZE_FORK_VERSION": "0x90000000",
                    "CONSENSUS_BLOCK_SCRATCH_SPACE": str(args.block_scratch),
                    "SLOTS_PER_ROUND": str(args.slots_per_round),
                    "TARGET_COMMITTEE_SIZE": str(args.target_committee_size),
                    "TARGET_AGGREGATORS_PER_COMMITTEE": str(args.aggregators_per_committee),
                    "ATTESTATION_SUBNET_COUNT": str(args.subnets),
                    "SUBNETS_PER_NODE": str(args.subnets_per_node),
                    "AGGREGATE_DUE_BPS_GLOAS": str(args.aggregate_due_bps),
                },
                "premine": {},
            },
            "reliabilities": {
                "staker": {"added_latency": "0ms", "added_packet_loss": 0.0,
                           "bandwidth_up": "25 Mbit", "bandwidth_down": "50 Mbit"},
                "super": {"added_latency": "0ms", "added_packet_loss": 0.0,
                          "bandwidth_up": "1024 Mbit", "bandwidth_down": "1024 Mbit"},
            },
            "locations": loc_yaml,
            "nodes": nodes,
            "clients": clients,
        },
    }


def run(cmd, log=None, **kw):
    print("+", " ".join(str(c) for c in cmd), flush=True)
    if log is None:
        subprocess.run(cmd, check=True, **kw)
        return
    with open(log, "w") as fh:
        subprocess.run(cmd, check=True, stdout=fh, stderr=subprocess.STDOUT, **kw)


def main():
    args = parse_args()
    args.name = args.name or f"n{args.nodes}-v{args.validators}-s{args.seed}"
    out = HERE / "runs" / args.name
    if out.exists():
        sys.exit(f"{out} exists")
    out.mkdir(parents=True)
    rng = random.Random(args.seed)
    country, supers, vals = place(args, rng)
    for i in range(args.nodes):
        print(f"  node{i + 1}: {country[i]:<16} {'super' if i in supers else 'home ':<5} "
              f"{vals[i]} validators")
    pool = args.validators // args.slots_per_round
    committees = max(1, pool // args.target_committee_size)
    if args.ffg_committees_per_subnet_per_slot > 1:
        committees = min(64, args.ffg_committees_per_subnet_per_slot * args.subnets)
    print(f"  {len(set(country))} countries, {len(supers)} supernodes, "
          f"{sum(vals)} validators, seed {args.seed}")
    print(f"  pool {pool} seats a slot, {committees} committees of "
          f"{pool // committees}, {args.subnets} subnets, {args.subnets_per_node} per node")

    sim = out / "sim.yaml"
    text = yaml.safe_dump(sim_config(args, country, supers, vals),
                          sort_keys=False, default_flow_style=False, width=120)
    text = text.replace("    premine: {}\n",
                        f"{PREMINE_BEGIN}\n    premine: {{}}\n{PREMINE_END}\n", 1)
    sim.write_text(text)
    run(["go", "run", "./spamoor-premine", "-sim", str(sim), "-inject"], cwd=HERE)

    data = out / "data"
    run([str(BIN / "ethshadow"), "--gen-only", "-d", str(data), str(sim)],
        log=out / "ethshadow.log")
    print(f"  generated {data}")
    if args.gen_only:
        return
    run(["shadow", "-d", str(data / "shadow"), str(data / "shadow.yaml")],
        log=out / "shadow.log")
    print(f"  shadow finished; logs in {out}")


if __name__ == "__main__":
    main()
