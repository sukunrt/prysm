#!/usr/bin/env bash
set -euo pipefail
cd /home/sukun/dev/prysm2
heze_out=$PWD/runs/heze-latest-20261006
heze_ctx=$heze_out/images
mkdir -p "$heze_ctx"
export CGO_ENABLED=1
export GOTOOLCHAIN=go1.26.5
export GOCACHE=/tmp/heze-go-build
for heze_target in beacon-chain validator prysmctl; do
    go build -buildvcs=false -mod=readonly -p 2 -tags osusergo,netgo \
        -ldflags '-linkmode external -extldflags "-static"' \
        -o "$heze_ctx/$heze_target" "./cmd/$heze_target"
    sha256sum "$heze_ctx/$heze_target" > "$heze_out/$heze_target.sha256"
    go version -m "$heze_ctx/$heze_target" > "$heze_out/$heze_target.buildinfo"
done
go build -buildvcs=false -mod=readonly -p 2 -o shadow/bin/prysm-beacon ./cmd/beacon-chain
go build -buildvcs=false -mod=readonly -p 2 -o shadow/bin/prysm-validator ./cmd/validator
for heze_target in beacon-chain validator prysmctl; do
    case "$heze_target" in
        beacon-chain) heze_image=prysm-beacon-chain; heze_dockerfile=Dockerfile.beacon-chain ;;
        validator) heze_image=prysm-validator; heze_dockerfile=Dockerfile.validator ;;
        prysmctl) heze_image=prysm-genesis-gen; heze_dockerfile=Dockerfile.genesis-gen ;;
    esac
    docker build -f "kurtosis/$heze_dockerfile" \
        -t "$heze_image:heze-latest-20261006" "$heze_ctx"
    docker image inspect --format '{{.Id}}' "$heze_image:heze-latest-20261006" > "$heze_out/$heze_target.image-id"
done
