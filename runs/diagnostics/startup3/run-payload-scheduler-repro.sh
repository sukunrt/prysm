#!/usr/bin/env bash
set -euo pipefail

prysm_diag_repo=$(pwd)
if [[ ! -f "$prysm_diag_repo/go.mod" ]]; then
	printf 'run from the Prysm repository root\n' >&2
	exit 2
fi

prysm_diag_go=${PRYSM_DIAGNOSTIC_GO:-/home/sukun/dev/go/bin/go}
prysm_diag_cache=${PRYSM_DIAGNOSTIC_GOCACHE:-/tmp/prysm-diagnostic-buildcache}
prysm_diag_run=${PRYSM_DIAGNOSTIC_RUN_DIR:-/tmp/prysm-payload-scheduler-repro}
prysm_diag_genesis=${PRYSM_STARTUP_REAL_GENESIS_SSZ:-/tmp/prysm-startup3-wire-h/bundle/network-configs/genesis.ssz}
prysm_diag_workers=${1:-128}
prysm_diag_jobs=${2:-1024}
prysm_diag_calls=${3:-16}
prysm_diag_server="$prysm_diag_run/payloadserver"
prysm_diag_test="$prysm_diag_run/execution.test"
prysm_diag_server_pid=

mkdir -p "$prysm_diag_run"
: >"$prysm_diag_run/timestamps.tsv"

cleanup_payload_server() {
	if [[ -n "$prysm_diag_server_pid" ]] && kill -0 "$prysm_diag_server_pid" 2>/dev/null; then
		kill "$prysm_diag_server_pid"
		wait "$prysm_diag_server_pid" 2>/dev/null || true
	fi
}
trap cleanup_payload_server EXIT INT TERM

env GOCACHE="$prysm_diag_cache" GOMAXPROCS=4 \
	"$prysm_diag_go" build -o "$prysm_diag_server" ./runs/diagnostics/startup3/payloadserver
env GOCACHE="$prysm_diag_cache" GOMAXPROCS=4 \
	"$prysm_diag_go" test -c -tags=develop -o "$prysm_diag_test" ./beacon-chain/execution

"$prysm_diag_server" -listen 127.0.0.1:18561 -response-bytes 2060 \
	>"$prysm_diag_run/server.jsonl" 2>"$prysm_diag_run/server.stderr" &
prysm_diag_server_pid=$!
for _ in $(seq 1 100); do
	if rg -q '"event":"ready"' "$prysm_diag_run/server.jsonl"; then
		break
	fi
	if ! kill -0 "$prysm_diag_server_pid" 2>/dev/null; then
		wait "$prysm_diag_server_pid"
	fi
	sleep 0.05
done
rg -q '"event":"ready"' "$prysm_diag_run/server.jsonl"

for prysm_diag_arm in none memoized scan; do
	printf '%s\t%s\tstart\t%s\n' "$(date +%s%N)" "$prysm_diag_arm" "$(date --iso-8601=ns)" \
		| tee -a "$prysm_diag_run/timestamps.tsv"
	env \
		GOMAXPROCS=4 \
		PRYSM_DIAGNOSTIC_ENGINE_ENDPOINT=http://127.0.0.1:18561 \
		PRYSM_DIAGNOSTIC_COUNT_ARM="$prysm_diag_arm" \
		PRYSM_DIAGNOSTIC_COUNT_WORKERS="$prysm_diag_workers" \
		PRYSM_DIAGNOSTIC_COUNT_JOBS="$prysm_diag_jobs" \
		PRYSM_DIAGNOSTIC_PAYLOAD_CALLS="$prysm_diag_calls" \
		PRYSM_DIAGNOSTIC_RUNTIME_TRACE="$prysm_diag_run/$prysm_diag_arm.trace" \
		PRYSM_STARTUP_REAL_GENESIS_SSZ="$prysm_diag_genesis" \
		"$prysm_diag_test" \
		-test.v \
		-test.run '^TestDiagnosticGetPayloadV6UnderGenesisCountLoad$' \
		-test.count=1 \
		-test.timeout=10m \
		-test.cpuprofile="$prysm_diag_run/$prysm_diag_arm.cpu.pb.gz" \
		2>&1 | tee "$prysm_diag_run/$prysm_diag_arm.test.log"
	printf '%s\t%s\tend\t%s\n' "$(date +%s%N)" "$prysm_diag_arm" "$(date --iso-8601=ns)" \
		| tee -a "$prysm_diag_run/timestamps.tsv"
done
