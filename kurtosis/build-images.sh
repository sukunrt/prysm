#!/usr/bin/env bash
# Build the local images kurtosis/network_params.yaml refers to:
#
#   prysm-beacon-chain:local  prysm-validator:local  prysm-genesis-gen:local
#   prysm-buildoor:local      (patched ethpandaops/buildoor, see
#                              Dockerfile.buildoor)
#
# Each is also tagged with the jj change id of the working copy, so a run can
# be traced back to the tree it was built from. Nothing is pushed; kurtosis
# picks the images up from the local docker daemon.
#
# IMAGE_TAG replaces "local" for the movable tag. A running enclave holds its
# containers on the tag it was started from, so a second tree building :local
# silently replaces the live run's images; set IMAGE_TAG (and name the same tag
# in the run's args file) whenever another enclave is up.
#
# The binaries are statically linked (CGO for blst, netgo/osusergo so no
# glibc NSS at runtime), which is what lets an Arch host produce binaries a
# debian:bookworm-slim runtime can execute.
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

change_id=$(jj log --no-graph -r @ -T 'change_id.short()' 2>/dev/null || echo local)
tag=${IMAGE_TAG:-local}
ctx=$(mktemp -d -t prysm-kurtosis-ctx-XXXXXX)
empty_ctx=$(mktemp -d -t prysm-buildoor-ctx-XXXXXX)
trap 'rm -rf "$ctx" "$empty_ctx"' EXIT

echo "==> building binaries (change $change_id)"
for target in beacon-chain validator prysmctl; do
    CGO_ENABLED=1 go build -p "${GO_BUILD_P:-6}" -tags osusergo,netgo \
        -ldflags '-linkmode external -extldflags "-static"' \
        -o "$ctx/$target" "./cmd/$target"
done
# The generator patches live in a fork branch (see Dockerfile.genesis-gen);
# build it straight from the git URL, pinned. Docker caches the layers, so
# this is only slow the first time.
genesis_gen_rev=a40dc31d75b9362cfb62b252e346ec7d19ceefab
echo "==> docker build decoupled-genesis-gen-base ($genesis_gen_rev)"
docker build -t decoupled-genesis-gen-base \
    "https://github.com/sukunrt/ethereum-genesis-generator.git#$genesis_gen_rev"

build() {
    local dockerfile=$1 image=$2 context=${3:-$ctx}
    echo "==> docker build $image"
    docker build -f "kurtosis/$dockerfile" -t "$image:$tag" \
        -t "$image:$change_id" "$context"
}

build Dockerfile.beacon-chain prysm-beacon-chain
build Dockerfile.validator prysm-validator
build Dockerfile.genesis-gen prysm-genesis-gen
# buildoor clones its own source, so it takes an empty context rather than the
# 300 MB of prysm binaries the other three need.
build Dockerfile.buildoor prysm-buildoor "$empty_ctx"

echo "==> done; images tagged :$tag and :$change_id"
