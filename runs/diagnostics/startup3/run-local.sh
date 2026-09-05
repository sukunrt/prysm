#!/usr/bin/env bash
set -euo pipefail
probe=true; engine=false; start_offset=-400ms; per_slot=15000
while [[ ${1:-} == --* ]]; do case $1 in
  --no-probe) probe=false; shift;; --engine-probe) engine=true; shift;;
  --start-offset) start_offset=${2:?}; shift 2;; --per-slot) per_slot=${2:?}; shift 2;; *) exit 2;; esac; done
[[ $# -eq 1 ]] || { echo "usage: $0 [--no-probe] [--engine-probe] [--start-offset DURATION] [--per-slot N] <private-output-root>" >&2; exit 2; }
out=$1; [[ ! -e $out ]] || { echo "output exists" >&2; exit 1; }
base=$(cd "$(dirname "$0")" && pwd); name="startup3-$(date -u +%Y%m%dT%H%SZ)-$$"
mkdir -m 700 "$out" "$out/results"
python3 "$base/prepare_bundle.py" --output "$out/bundle" --genesis-delay 300 >"$out/bundle-ready.json"
genesis=$(python3 -c 'import json,sys; xs=[json.loads(x) for x in open(sys.argv[1])]; print([x for x in xs if x.get("event")=="bundle_prepared"][-1]["genesis_unix"])' "$out/bundle-ready.json")
(( genesis - $(date +%s) >= 75 )) || { echo "under 75 seconds to genesis" >&2; exit 1; }
cp "$base/kurtosis/main.star" "$base/kurtosis/kurtosis.yml" "$base/kurtosis/Dockerfile.snooper" "$out/bundle/"
args=$(python3 -c 'import json,sys;print(json.dumps({"bundle_path":".","slots_per_round":4,"ledger":False,"engine_snooper":sys.argv[1]=="true","startup_engine_probe":sys.argv[1]=="true"}))' "$engine")
kurtosis run --enclave "$name" --verbosity brief --image-download missing --show-enclave-inspect=false "$out/bundle" "$args" >"$out/kurtosis-run.log" 2>&1
uuid=$(kurtosis enclave ls --full-uuids | awk -v n="$name" '$0 ~ n {print $1;exit}')
[[ -n $uuid ]] || { echo "enclave UUID not found" >&2; exit 1; }
mapfile -t ids < <(docker ps -q --filter "label=com.kurtosistech.enclave-id=$uuid")
(( ${#ids[@]} )) || { echo "no enclave containers" >&2; exit 1; }
docker inspect "${ids[@]}" >"$out/enclave-containers.json"
python3 "$base/discover_services.py" "$out/enclave-containers.json" "$out/public-services.json"
read -r bn1 bn3 vc3 bn3id vc3id < <(python3 -c 'import json,sys;x=json.load(open(sys.argv[1]));print(x["bn-1"]["ip"],x["bn-3"]["ip"],x["vc-3"]["ip"],x["bn-3"]["container_id"],x["vc-3"]["container_id"])' "$out/public-services.json")
(( genesis - $(date +%s) >= 75 )) || { echo "services became ready under 75 seconds before genesis" >&2; exit 1; }
load_args=("$out/bundle" "$bn1:4000" "$bn3:4000" "$out/results/load")
$probe || load_args=("$out/bundle" "$bn1:4000" "$bn3:4000" "$out/results/load")
if $probe; then START_OFFSET=$start_offset PER_SLOT=$per_slot "$base/start-load-and-probe.sh" "${load_args[@]}" & else env GOMAXPROCS=4 /tmp/prysm-startup-ffgsource -genesis "$out/bundle/network-configs/genesis.ssz" -config "$out/bundle/network-configs/config.yaml" -mnemonic-file "$out/bundle/mnemonics.yaml" -exclude-selection "$out/bundle/selection.json" -endpoint "$bn1:4000" -per-slot "$per_slot" -slots 1,2,3 -spread 2s -start-offset="$start_offset" >"$out/results/ffgsource.json" 2>"$out/results/ffgsource.stderr" & fi
load_pid=$!
"/tmp/prysm-startup-repro/runs/diagnostics/startup-repro/capture-startup-profiles.sh" "$genesis" "$bn3" "$vc3" "$out/results/profiles" "$bn3id" "$vc3id" &
capture_pid=$!
printf '{"event":"started","enclave_uuid":"%s","genesis_unix":%s,"load_pid":%s,"capture_pid":%s}\n' "$uuid" "$genesis" "$load_pid" "$capture_pid" >"$out/public-run.json"
wait "$load_pid"; wait "$capture_pid"
