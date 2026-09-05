#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 <genesis-unix> <bn3-container-id> <bn3-ip> <proxy-ip> <private-new-output-dir>" >&2
  exit 2
}

[[ $# -eq 5 ]] || usage

genesis_unix=$1
container_id=$2
bn_ip=$3
proxy_ip=$4
output_dir=$5

[[ $genesis_unix =~ ^[0-9]+$ ]] || usage
[[ $container_id =~ ^[0-9a-f]{64}$ ]] || usage
[[ $bn_ip =~ ^[0-9a-fA-F:.]+$ ]] || usage
[[ $proxy_ip =~ ^[0-9a-fA-F:.]+$ ]] || usage
[[ ! -e $output_dir ]] || {
  echo "output directory already exists" >&2
  exit 1
}

for command_name in curl docker dumpcap ip python3; do
  command -v "$command_name" >/dev/null || {
    echo "required command not found: $command_name" >&2
    exit 1
  }
done

umask 077
mkdir -m 700 -- "$output_dir"
markers="$output_dir/timestamps.jsonl"
pcap="$output_dir/bn3-veth-engine.pcapng"
capture_log="$output_dir/dumpcap.stderr"
links_json="$output_dir/host-links.json"
trace="$output_dir/trace.out"
trace_log="$output_dir/trace.stderr"
touch "$pcap" "$capture_log" "$trace" "$trace_log" "$markers"

mark() {
  local event=$1
  local status=${2:-}
  printf '{"event":"%s","unix_ns":%s,"status":"%s"}\n' \
    "$event" "$(date +%s%N)" "$status" >>"$markers"
}

wait_until() {
  local target=$1
  local now delay
  now=$(date +%s)
  if (( now < target )); then
    delay=$((target - now))
    sleep "$delay"
  fi
}

inspected_id=$(docker inspect --format '{{.Id}}' "$container_id")
[[ $inspected_id == "$container_id" ]] || {
  echo "docker inspect did not resolve the exact container ID" >&2
  exit 1
}
container_pid=$(docker inspect --format '{{.State.Pid}}' "$container_id")
[[ $container_pid =~ ^[0-9]+$ ]] && (( container_pid > 1 )) || {
  echo "container has no valid running PID" >&2
  exit 1
}
container_iflink=$(docker exec "$container_id" cat /sys/class/net/eth0/iflink)
[[ $container_iflink =~ ^[0-9]+$ ]] || {
  echo "container eth0 has no valid host ifindex" >&2
  exit 1
}
ip -j link show >"$links_json"
host_veth=$(python3 -c '
import json, sys
links = json.load(open(sys.argv[1]))
matches = [link["ifname"] for link in links if link.get("ifindex") == int(sys.argv[2])]
if len(matches) != 1:
    raise SystemExit("expected exactly one host interface for container eth0 iflink")
print(matches[0])
' "$links_json" "$container_iflink")
[[ $host_veth =~ ^[a-zA-Z0-9_.:@-]+$ ]] || {
  echo "resolved host interface has an unsafe name" >&2
  exit 1
}

pcap_pid=
trace_pid=
cleanup() {
  local exit_status=$?
  trap - EXIT INT TERM
  if [[ -n ${pcap_pid:-} ]] && kill -0 "$pcap_pid" 2>/dev/null; then
    kill -INT "$pcap_pid" 2>/dev/null || true
    wait "$pcap_pid" 2>/dev/null || true
  fi
  if [[ -n ${trace_pid:-} ]] && kill -0 "$trace_pid" 2>/dev/null; then
    kill -TERM "$trace_pid" 2>/dev/null || true
    wait "$trace_pid" 2>/dev/null || true
  fi
  mark capture_cleanup "$exit_status"
  exit "$exit_status"
}
trap cleanup EXIT INT TERM

mark capture_begin
capture_end=$((genesis_unix + 52))
capture_duration=$((capture_end - $(date +%s) + 1))
(( capture_duration > 0 )) || {
  echo "genesis capture window has already ended" >&2
  exit 1
}
dumpcap -q -i "$host_veth" -f "host $proxy_ip and tcp port 8561" \
  -a "duration:$capture_duration" -w "$pcap" 2>"$capture_log" &
pcap_pid=$!
mark pcap_started

trace_start=$((genesis_unix + 10))

wait_until "$trace_start"
mark trace_request_begin
curl --fail --silent --show-error --max-time 55 \
  "http://$bn_ip:6060/debug/pprof/trace?seconds=36" \
  --output "$trace" 2>"$trace_log" &
trace_pid=$!

wait_until "$capture_end"
pcap_status=success
if kill -0 "$pcap_pid" 2>/dev/null; then
  kill -INT "$pcap_pid"
  wait "$pcap_pid" || true
elif ! wait "$pcap_pid"; then
  pcap_status=failed
fi
pcap_pid=
[[ -s $pcap ]] || pcap_status=failed
trace_status=success
if kill -0 "$trace_pid" 2>/dev/null; then
  kill -TERM "$trace_pid" 2>/dev/null || true
  trace_status=deadline
fi
if ! wait "$trace_pid" 2>/dev/null; then
  [[ $trace_status == deadline ]] || trace_status=failed
fi
mark trace_request_end "$trace_status"
trace_pid=
chmod 600 -- "$pcap" "$capture_log" "$links_json" "$trace" "$trace_log" "$markers"
mark capture_end "$pcap_status"
trap - EXIT INT TERM

if [[ $pcap_status != success ]]; then
  echo "packet capture failed; details are in the private output directory" >&2
  exit 1
fi
if [[ $trace_status != success ]]; then
  echo "runtime trace capture failed; details are in the private output directory" >&2
  exit 1
fi
echo "engine evidence captured in private directory: $output_dir"
