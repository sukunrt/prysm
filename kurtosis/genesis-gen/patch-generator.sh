#!/bin/bash -e
# Build-time patches to the stock ethereum-genesis-generator image.
# Run once from Dockerfile.genesis-gen; not used at runtime.

# ============================================================================
# !!! HEZE IS A CL-ONLY FORK IN OUR SIMS -- THE EL MUST NOT FORK WITH IT !!!
#
# The upstream generator maps every CL fork to the next EL fork by ordinal:
# deneb->cancun, electra->prague, fulu->osaka, gloas->amsterdam, and
# HEZE->BOGOTA (generate_genesis.sh, genesis_add_heze). A scheduled bogotaTime
# makes geth demand the next engine-API version at Heze's timestamp, while
# Prysm (correctly) keeps calling forkchoiceUpdatedV4 for its Gloas-shaped
# blocks. geth then rejects every fcu with -38005 "Unsupported fork", block
# production loses local payloads, and the chain stalls at the Heze boundary.
# This is exactly what killed decoupled-shadow-sim run data12 (2026-08-16).
#
# Heze only changes the consensus layer (available-attestation stream etc.),
# so we disable genesis_add_heze: HEZE_FORK_EPOCH still reaches the CL config
# via the template, but the EL genesis gets no bogota entries.
#
# If a future Heze design DOES need an EL fork, delete this AND teach Prysm
# the matching engine-API version first.
# ============================================================================
note='echo "HEZE is CL-only in this image: no bogota in the EL genesis"'
sed -i "s|&& genesis_add_heze \$tmp_dir|\&\& $note|" \
    /apps/el-gen/generate_genesis.sh
grep -q 'HEZE is CL-only' /apps/el-gen/generate_genesis.sh

# SLOTS_PER_ROUND is this fork's own config field and no upstream template
# knows about it. The image's ENV supplies the default; values.env, and so
# ethereum_genesis_generator_params.extra_env, overrides it.
comment1='# Rounds per epoch = SLOTS_PER_EPOCH / SLOTS_PER_ROUND. The committee'
comment2='# shuffle repeats every round: the attestation-traffic multiplier.'
sed -i "/^SECONDS_PER_SLOT:/i $comment1\n$comment2\nSLOTS_PER_ROUND: \$SLOTS_PER_ROUND" \
    /config/cl/config.yaml
grep -q '^SLOTS_PER_ROUND' /config/cl/config.yaml

# AVAILABLE_ATTESTATION_DUE_BPS_HEZE is the head-timing sweep axis of the
# step-6 measurements (plan-next.md step 6, open question 2). Same story as
# SLOTS_PER_ROUND: the field is this fork's own, no upstream template knows
# it, the image ENV holds the default and extra_env overrides it. Both the
# beacon node and the VC read this file (the VC gets it as
# --chain-config-file), so one line moves the whole run's vote deadline.
comment3='# How far into the slot an available attestation is due, in basis'
comment4='# points. The head-timing sweep axis.'
sed -i "/^SECONDS_PER_SLOT:/i $comment3\n$comment4\nAVAILABLE_ATTESTATION_DUE_BPS_HEZE: \$AVAILABLE_ATTESTATION_DUE_BPS_HEZE" \
    /config/cl/config.yaml
grep -q '^AVAILABLE_ATTESTATION_DUE_BPS_HEZE' /config/cl/config.yaml

# The CL genesis state: prysmctl, not eth-genesis-state-generator. See
# prysm-genesis-state.sh for the why. Installing it under the upstream name
# leaves the stock entrypoint untouched.
mv /usr/local/bin/eth-genesis-state-generator \
   /usr/local/bin/eth-genesis-state-generator.upstream
ln -s /usr/local/bin/prysm-genesis-state.sh \
      /usr/local/bin/eth-genesis-state-generator
