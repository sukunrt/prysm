#!/bin/sh
# Hold the beacon HTTP port closed until the node can actually answer
# /eth/v1/node/identity, then proxy it.
#
# Why this exists. ethereum-package reads every CL's ENR out of
# /eth/v1/node/identity right after kurtosis sees the HTTP port accept a TCP
# connection (src/cl/prysm/prysm_launcher.star:421, a plan.request with no
# retry). Prysm opens that listener as soon as the API service starts, but
# the ENR only exists once the genesis state is loaded and the fork digest is
# known -- about 1.2s later on this fork's genesis. Lose that race and the
# whole run dies with:
#
#   No field '.data.enr' was found on input
#   '{"message":"Could not obtain enr: could not serialize nil record"}'
#
# It is roughly a coin flip per node, so a 16-node run would never start.
# Rather than fork the package to turn that request into a retrying wait, the
# image moves prysm's HTTP server to an internal port and only binds the port
# kurtosis watches once the node answers. Nothing outside sees a difference
# except that the port appears a second later, which is the point.
#
# Drop this once Prysm serves the identity endpoint only after the ENR exists,
# or once the package retries the request.
set -e

http_port=3500
args=""
skip_next=0
for arg in "$@"; do
    if [ "$skip_next" = 1 ]; then
        http_port=$arg
        skip_next=0
        continue
    fi
    case "$arg" in
        --http-port=*) http_port=${arg#--http-port=}; continue ;;
        --http-port) skip_next=1; continue ;;
    esac
    args="$args '$(printf '%s' "$arg" | sed "s/'/'\\\\''/g")'"
done
internal_port=$((http_port + 1))

eval "set -- $args"
/beacon-chain-bin "$@" --http-port="$internal_port" &
bn_pid=$!
trap 'kill -TERM "$bn_pid" 2>/dev/null' TERM INT

(
    while ! curl -sf "http://127.0.0.1:$internal_port/eth/v1/node/identity" \
            2>/dev/null | grep -q '"enr"'; do
        kill -0 "$bn_pid" 2>/dev/null || exit 0
        sleep 0.5
    done
    echo "beacon-entrypoint: identity is answerable, opening port $http_port"
    exec socat "TCP-LISTEN:$http_port,reuseaddr,fork" \
        "TCP:127.0.0.1:$internal_port"
) &
proxy_pid=$!

wait "$bn_pid"
status=$?
kill -TERM "$proxy_pid" 2>/dev/null || true
exit $status
