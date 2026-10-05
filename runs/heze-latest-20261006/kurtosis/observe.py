#!/usr/bin/env python3
"""Observe 32 proposal slots and retain raw REST/metrics/log evidence."""
import concurrent.futures
import datetime
import json
import pathlib
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request

ENCLAVE = sys.argv[1]
OUT = pathlib.Path(__file__).resolve().parent
NODES = [f"cl-{i:02d}-prysm-geth" for i in range(1, 11)]


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def port(service, name):
    value = command("kurtosis", "port", "print", ENCLAVE, service, name).rstrip("/")
    return value if "://" in value else "http://" + value


def get(url, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=20) as resp:
        return json.load(resp)


def save(name, value):
    (OUT / name).write_text(json.dumps(value, indent=2) + "\n")


urls = {node: port(node, "http") for node in NODES}
metrics_urls = {node: port(node, "metrics") for node in NODES}
el_url = port("el-01-geth-prysm", "rpc")
spamoor_url = port("spamoor", "http")
save("endpoints.json", {"beacons": urls, "metrics": metrics_urls, "execution": el_url, "spamoor": spamoor_url})
clients = get(spamoor_url + "/api/clients")
save("spamoor-clients.json", clients)
assert any(c.get("name") == "01-geth-prysm" and c.get("ready") for c in clients), clients
spammers = get(spamoor_url + "/api/spammers")
save("spamoor-spammers.json", spammers)
for name in ("heze-transactions", "heze-blobs"):
    spammer = next(s for s in spammers if s["name"] == name)
    request = urllib.request.Request(spamoor_url + f"/api/spammer/{spammer['id']}/start", method="POST")
    with urllib.request.urlopen(request, timeout=20) as response:
        assert response.status == 200
genesis = get(urls[NODES[0]] + "/eth/v1/beacon/genesis")
save("genesis.json", genesis)
genesis_time = int(genesis["data"]["genesis_time"])
stop_at = genesis_time + 32 * 12 + 8
deadline = time.monotonic() + max(0, stop_at - time.time()) + 90
print(f"Genesis {genesis_time}; observing slots 1..32 until {stop_at}", flush=True)


def sample(node):
    result = {"node": node, "time": time.time()}
    try:
        result["head"] = get(urls[node] + "/eth/v1/beacon/headers/head")
        result["finality"] = get(urls[node] + "/eth/v1/beacon/states/head/finality_checkpoints")
        with urllib.request.urlopen(metrics_urls[node] + "/metrics", timeout=10) as resp:
            metrics = resp.read().decode()
        with (OUT / (node + ".metrics")).open("a") as dst:
            dst.write(f"# TS {time.time()}\n")
            dst.write("\n".join(line for line in metrics.splitlines() if line.startswith((
                "process_cpu_seconds", "process_resident_memory", "beacon_head", "beacon_finalized",
                "beacon_current_justified", "goldfish_", "aggregated_attestations_in_pool", "unaggregated_attestations_in_pool",
                "p2p_pubsub_undeliverable", "p2p_peer_count", "ffg_", "beacon_data_column",
            ))) + "\n")
    except Exception as err:
        result["error"] = str(err)
    return result


with concurrent.futures.ThreadPoolExecutor(max_workers=10) as pool:
    while time.time() < stop_at:
        if time.monotonic() > deadline:
            raise RuntimeError("observation deadline exceeded")
        values = list(pool.map(sample, NODES))
        with (OUT / "observations.jsonl").open("a") as dst:
            for value in values:
                dst.write(json.dumps(value) + "\n")
        print("sample", datetime.datetime.now(datetime.timezone.utc).isoformat(),
              [v.get("head", {}).get("data", {}).get("header", {}).get("message", {}).get("slot", v.get("error")) for v in values], flush=True)
        time.sleep(min(12, max(0, stop_at - time.time())))

checks = {"enclave": ENCLAVE, "slots": [], "nodes": [], "errors": []}
for node in NODES:
    try:
        root = get(urls[node] + "/eth/v1/beacon/blocks/32/root")
        syncing = get(urls[node] + "/eth/v1/node/syncing")
        finality = get(urls[node] + "/eth/v1/beacon/states/head/finality_checkpoints")
        vals = get(urls[node] + "/eth/v1/beacon/states/head/validators?status=active")
        row = {"node": node, "slot32_root": root, "syncing": syncing, "finality": finality, "active_validators": len(vals["data"])}
        save(node + "-final.json", row)
        checks["nodes"].append(row)
    except Exception as err:
        checks["errors"].append({"node": node, "error": str(err)})

for slot in range(1, 33):
    try:
        block = get(urls[NODES[0]] + f"/eth/v2/beacon/blocks/{slot}")
        save(f"block-{slot}.json", block)
        envelope = get(urls[NODES[0]] + f"/eth/v1/beacon/execution_payload_envelopes/{slot}")
        save(f"envelope-{slot}.json", envelope)
        body = block["data"]["message"]["body"]
        payload = envelope["data"]["message"]["payload"]
        el = get(el_url, {"jsonrpc": "2.0", "id": slot, "method": "eth_getBlockByHash", "params": [payload["block_hash"], False]})
        save(f"execution-block-{slot}.json", el)
        atts = body.get("attestations", [])
        row = {"slot": slot, "proposer": block["data"]["message"]["proposer_index"], "attestations": len(atts),
               "attestation_slots": [a["data"]["slot"] for a in atts], "payload_attestations": len(body.get("payload_attestations", [])),
               "transactions": len(el["result"]["transactions"]), "envelope_transactions": len(payload["transactions"]),
               "blob_gas_used": int(el["result"].get("blobGasUsed", "0x0"), 16), "execution_hash": payload["block_hash"]}
        checks["slots"].append(row)
    except Exception as err:
        checks["errors"].append({"slot": slot, "error": str(err)})

container_ids = command("docker", "ps", "-q", "--filter", "ancestor=prysm-beacon-chain:heze-latest-20261006").split()
if container_ids:
    enclave_id = command("docker", "inspect", "--format", '{{index .Config.Labels "com.kurtosistech.enclave-id"}}', container_ids[0])
    ids = command("docker", "ps", "-aq", "--filter", "label=com.kurtosistech.enclave-id=" + enclave_id).split()
    metadata = json.loads(command("docker", "inspect", *ids))
    safe_metadata = []
    for container in metadata:
        name = container["Config"]["Labels"].get("com.kurtosistech.id", container["Name"].lstrip("/"))
        safe_metadata.append({"service": name, "id": container["Id"], "image_id": container["Image"], "image": container["Config"]["Image"], "state": container["State"]})
        if name.startswith(("cl-", "vc-", "el-")) or name == "spamoor":
            with (OUT / (name + ".log")).open("w") as dst:
                subprocess.run(["docker", "logs", "--timestamps", container["Id"]], stdout=dst, stderr=subprocess.STDOUT, check=True)
    save("containers.json", safe_metadata)

roots = {n["slot32_root"]["data"]["root"] for n in checks["nodes"]}
checks["ten_nodes_agree_slot32"] = len(checks["nodes"]) == 10 and len(roots) == 1
checks["all_have_1000_active"] = len(checks["nodes"]) == 10 and all(n["active_validators"] == 1000 for n in checks["nodes"])
checks["all_blocks_with_attestations"] = len(checks["slots"]) == 32 and all(s["attestations"] > 0 for s in checks["slots"])
checks["nonempty_execution"] = any(s["transactions"] > 0 for s in checks["slots"])
checks["blob_execution"] = any(s["blob_gas_used"] > 0 for s in checks["slots"])
save("checks.json", checks)
print(json.dumps(checks, indent=2), flush=True)
if checks["errors"] or not all(checks[k] for k in ("ten_nodes_agree_slot32", "all_have_1000_active", "all_blocks_with_attestations", "nonempty_execution", "blob_execution")):
    sys.exit(1)
