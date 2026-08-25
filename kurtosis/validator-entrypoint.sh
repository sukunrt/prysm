#!/bin/sh
# Strip --beacon-rest-api-provider before exec'ing the validator, then hand
# over. Everything else passes through untouched.
#
# Why this exists. The Prysm VC's REST client has no Gloas/Heze SSZ block
# codec: AvailableAttestationData panics with "unimplemented: use grpc"
# (validator/client/beacon-api/beacon_api_validator_client.go:129) the first
# time a duty comes due, and the client dies. So the VC has to talk gRPC.
#
# ethereum-package's prysm VC launcher passes --beacon-rpc-provider AND
# --beacon-rest-api-provider unconditionally (src/vc/prysm.star), and Prysm
# turns the REST client on for the mere presence of the second one:
#
#   if ctx.Bool(EnableBeaconRESTApi.Name) ||
#      ctx.IsSet(validatorflags.BeaconRESTApiProviderFlag.Name)
#                                       (config/features/config.go:372)
#
# ctx.IsSet is true even for an empty value, so no vc_extra_params override
# can undo it -- not --enable-beacon-rest-api=false, not an empty provider.
# The choices were to fork the package or to drop the flag here; dropping it
# here keeps ~/dev/ethereum-package a stock read-only checkout.
#
# Delete this shim once the REST client speaks Gloas/Heze, or once the
# package stops passing the flag to a Prysm-on-Prysm pair.

args=""
skip_next=0
for arg in "$@"; do
    if [ "$skip_next" = 1 ]; then
        skip_next=0
        continue
    fi
    case "$arg" in
        --beacon-rest-api-provider=*) continue ;;
        --beacon-rest-api-provider) skip_next=1; continue ;;
    esac
    args="$args '$(printf '%s' "$arg" | sed "s/'/'\\\\''/g")'"
done
eval "set -- $args"
exec /validator "$@"
