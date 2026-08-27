#!/usr/bin/env bash
# Build the local images kurtosis/network_params.yaml refers to:
#
#   prysm-beacon-chain:local  prysm-validator:local  prysm-genesis-gen:local
#   prysm-buildoor:local      prysm-dora:local  (built straight from the
#                             sukunrt fork branches, no local Dockerfile)
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
trap 'rm -rf "$ctx"' EXIT

echo "==> building binaries (change $change_id)"
for target in beacon-chain validator prysmctl; do
    CGO_ENABLED=1 go build -p "${GO_BUILD_P:-6}" -tags osusergo,netgo \
        -ldflags '-linkmode external -extldflags "-static"' \
        -o "$ctx/$target" "./cmd/$target"
done
# The generator patches live in a fork branch (see Dockerfile.genesis-gen);
# build it straight from the git URL at the branch tip. BuildKit re-resolves
# the ref on every build, and docker caches the layers, so this is only slow
# when the branch moves. dora and buildoor carry their own Dockerfiles, so
# their forks build directly into the final images -- no overlay needed.
echo "==> docker build decoupled-genesis-gen-base (branch decoupled)"
docker build -t decoupled-genesis-gen-base \
    "https://github.com/sukunrt/ethereum-genesis-generator.git#decoupled"
echo "==> docker build prysm-dora (branch decoupled)"
docker build -t "prysm-dora:$tag" -t "prysm-dora:$change_id" \
    "https://github.com/sukunrt/dora.git#decoupled"
echo "==> docker build prysm-buildoor (branch decoupled)"
docker build -t "prysm-buildoor:$tag" -t "prysm-buildoor:$change_id" \
    "https://github.com/sukunrt/buildoor.git#decoupled"

build() {
    local dockerfile=$1 image=$2 context=${3:-$ctx}
    echo "==> docker build $image"
    docker build -f "kurtosis/$dockerfile" -t "$image:$tag" \
        -t "$image:$change_id" "$context"
}

build Dockerfile.beacon-chain prysm-beacon-chain
build Dockerfile.validator prysm-validator
build Dockerfile.genesis-gen prysm-genesis-gen

echo "==> done; images tagged :$tag and :$change_id"
