#!/usr/bin/env bash
# Build the three local images kurtosis/network_params.yaml refers to:
#
#   prysm-beacon-chain:local  prysm-validator:local  prysm-genesis-gen:local
#
# Each is also tagged with the jj change id of the working copy, so a run can
# be traced back to the tree it was built from. Nothing is pushed; kurtosis
# picks the images up from the local docker daemon.
#
# The binaries are statically linked (CGO for blst, netgo/osusergo so no
# glibc NSS at runtime), which is what lets an Arch host produce binaries a
# debian:bookworm-slim runtime can execute.
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

change_id=$(jj log --no-graph -r @ -T 'change_id.short()' 2>/dev/null || echo local)
ctx=$(mktemp -d -t prysm-kurtosis-ctx-XXXXXX)
trap 'rm -rf "$ctx"' EXIT

echo "==> building binaries (change $change_id)"
for target in beacon-chain validator prysmctl; do
    CGO_ENABLED=1 go build -p "${GO_BUILD_P:-6}" -tags osusergo,netgo \
        -ldflags '-linkmode external -extldflags "-static"' \
        -o "$ctx/$target" "./cmd/$target"
done
cp kurtosis/genesis-gen/prysm-genesis-state.sh \
   kurtosis/genesis-gen/patch-generator.sh \
   kurtosis/beacon-entrypoint.sh kurtosis/validator-entrypoint.sh "$ctx/"

build() {
    local dockerfile=$1 image=$2
    echo "==> docker build $image"
    docker build -f "kurtosis/$dockerfile" -t "$image:local" \
        -t "$image:$change_id" "$ctx"
}

build Dockerfile.beacon-chain prysm-beacon-chain
build Dockerfile.validator prysm-validator
build Dockerfile.genesis-gen prysm-genesis-gen

echo "==> done; images tagged :local and :$change_id"
