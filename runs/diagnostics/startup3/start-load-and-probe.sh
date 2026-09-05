#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "usage: $0 <private-bundle-dir> <source-bn-grpc> <probe-bn-grpc> <private-output-dir>" >&2
  exit 2
fi

bundle=$1
source_endpoint=$2
probe_endpoint=$3
output=$4
genesis="$bundle/network-configs/genesis.ssz"
config="$bundle/network-configs/config.yaml"
mnemonic="$bundle/mnemonics.yaml"
selection="$bundle/selection.json"
per_slot=${PER_SLOT:-15000}
start_offset=${START_OFFSET:--400ms}

for path in "$genesis" "$config" "$mnemonic" "$selection"; do
  [[ -f "$path" ]] || { echo "missing required input: $path" >&2; exit 1; }
done
[[ ! -e "$output" ]] || { echo "output already exists: $output" >&2; exit 1; }
mkdir -m 700 "$output"

env GOMAXPROCS=4 /tmp/prysm-startup-rpcprobe \
  -genesis "$genesis" -config "$config" -endpoint "$probe_endpoint" \
  >"$output/rpcprobe.jsonl" 2>"$output/rpcprobe.stderr" &
probe_pid=$!

env GOMAXPROCS=4 /tmp/prysm-startup-ffgsource \
  -genesis "$genesis" -config "$config" -mnemonic-file "$mnemonic" \
  -exclude-selection "$selection" -endpoint "$source_endpoint" -per-slot "$per_slot" \
  -slots 1,2,3 -spread 2s -start-offset="$start_offset" \
  >"$output/ffgsource-result.json" 2>"$output/ffgsource.stderr" &
source_pid=$!

printf '{"event":"started","rpcprobe_pid":%d,"ffgsource_pid":%d,"output":"%s"}\n' \
  "$probe_pid" "$source_pid" "$output"
wait "$probe_pid"
wait "$source_pid"
