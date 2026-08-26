# Run the decoupled-consensus fork

This document tells you how to start the decoupled-consensus devnet with kurtosis.
It is complete: you do not need access to the original machine.
`kurtosis/README.md` gives the reasons for each value.

## What this fork is

- A Prysm fork: `github.com/sukunrt/prysm`, branch `decoupled-casper`.
- It adds Heze, a fork after Gloas. Heze changes only the consensus layer.
- A round is `SLOTS_PER_ROUND` slots. Each validator sends one FFG attestation
  per round, at the start of its slot. Head and availability votes are separate.
- The chain config does not exist upstream. A patched genesis-generator image
  injects it. A stock generator cannot start this network.

## What you need

- docker and kurtosis
- Go 1.26 with CGO and a C toolchain that can make static binaries
- a stock `ethpandaops/ethereum-package` checkout, recent master
  (tested at commit `0350d2e`); you do not need a package fork

## Start the network

1. Clone the fork:

   ```sh
   git clone -b decoupled-casper https://github.com/sukunrt/prysm
   cd prysm
   ```

2. Build the images:

   ```sh
   ./kurtosis/build-images.sh
   ```

3. Start the enclave:

   ```sh
   kurtosis run --enclave decoupled <path-to-ethereum-package> \
       --args-file kurtosis/network_params.yaml
   ```

The build makes four images: `prysm-beacon-chain:local`, `prysm-validator:local`,
`prysm-genesis-gen:local`, `prysm-buildoor:local`.
The images stay in the local docker daemon.
For a run on more than one host, push the images to a registry.
Then set the image fields in the args file to the registry names.

Note: the build links the Prysm binaries statically.
This is only necessary when the host glibc is newer than the glibc in
`debian:bookworm-slim`. A static binary runs on both.

There are two args files:

- `network_params.yaml` — baseline. 5 nodes, 50 keys each.
  2 nodes have an execution layer with no peers.
- `network_params.buildoor.yaml` — baseline plus one ePBS builder.
  The builder is in the genesis registry. It bids on the p2p path.
  Build the images with `IMAGE_TAG=<tag>` and use that tag in this file.

## Values that must not change

The args files already set all of these.

| Where | Value | Reason |
| --- | --- | --- |
| `network_params` | all fork epochs = 0, also `gloas_fork_epoch` and `heze_fork_epoch` | The genesis state is a Heze state. Gloas at 0 also puts Amsterdam at the EL genesis time. `forkchoiceUpdatedV4` needs that. |
| `el_image` | a pinned geth build | The default pin is `glamsterdam-devnet-8`, for comparability with earlier runs. geth master also works: build `master-fd07354` passed both harnesses on 2026-08-26 (`network_params.geth-master.yaml`). Use a pinned build, not `:latest`. |
| generator `extra_env` | `SLOTS_PER_ROUND: 8` | The main knob of the fork. 4 rounds per 32-slot epoch. |
| generator `extra_env` | `AVAILABLE_ATTESTATION_DUE_BPS_HEZE: 2500` | When the availability vote is due, in basis points of the slot. |
| generator `extra_env` | `TARGET_COMMITTEE_SIZE: 3000` | Committees must be large, near 3000 validators, at every scale. With few validators this floors to one committee per slot. |
| generator `extra_env` | `ATTESTATION_SUBNET_COUNT` = committee count per slot | Always set this to the committee count per slot: validators / `SLOTS_PER_ROUND` / `TARGET_COMMITTEE_SIZE`, minimum 1. The args files use 1 because they have only 250 validators. |
| generator `extra_env` | `SUBNETS_PER_NODE: 2` | Each node subscribes to two subnets. |
| generator `extra_env` | `HEZE_FORK_VERSION: "0x90000000"` | The fork version of this fork. It sets the fork digest. |
| `vc_extra_params` | `--decoupled-ffg-vote-at-slot-start` | The FFG vote goes out at slot start, not on block arrival. `--decoupled-ffg-vote-jitter` adjusts the jitter; the 200 ms default is recommended. |
| `cl_extra_params` | `--goldfish-vote-ledger` | Useful to analyze runs later. |

The generator image does three things a stock generator does not:

- It adds the fork's config keys to the CL config template.
- It removes Heze from the CL-to-EL fork map. Heze must not start an EL fork.
  If it does, geth asks for a newer engine API and the chain stops.
- It makes the genesis state with `prysmctl testnet generate-genesis --fork heze`.

Newer geth needs the EIP-8282 builder deposit/exit contracts in the EL
genesis. This repo's generator image (base 6.2.1) predeploys them, so any
geth up to `master-fd07354` (2026-08-26) works. For a geth newer than that,
run one shakeout first.

## Values you can change

- `participants[*].count` and `num_validator_keys_per_node` are free.
  The args files use 5 nodes and 250 validators because the source machine
  was small. The goal of this handoff is larger runs. Scale both up.
  Keep the total validator count at 128 or more.
  Below 128, genesis generation does not complete.
- Do not use `supernode: true`. Normal column custody and sampling are wanted.
  The args files set it only because the package refuses to start a PeerDAS
  network when no node is a supernode and no node has 128 or more validators.
  At 128 or more keys per node, remove it.
- Do not use `--minimum-peers-per-subnet`. The args files set it to 4 only
  because a 5-node enclave cannot reach the default of 6.
- The blob schedule is free. The generator builds BLOB_SCHEDULE from the
  `bpo_*` values. The args files use max 21 / target 14 at epoch 0, the
  current mainnet BPO2 values.
- The VC runs on gRPC by default. The package defaults a 1:1 Prysm BN/VC
  pair to gRPC. The REST client now supports the fork's duties, available
  attestations included, so both paths work; add
  `--enable-beacon-rest-api` to `vc_extra_params` to select REST.

## Send transactions

The args files run spamoor with three arms: transfers with 16 KiB calldata
(8 per slot), a public blob arm, and a blob arm pinned to an isolated EL.
This needs spamoor v1.2.0 or later with `--without-batcher` — see
`plan-spamoor-kurtosis.md`. Do not remove the eoatx arm's `client_group`.

## Measure

```sh
kurtosis/scrape.sh <enclave> <outdir> <slot-seconds>   # metrics, one sample per slot
kurtosis/summarize.py <outdir> --slot-seconds 12 --skip-slots 32
kurtosis/vclogs.py <outdir>        # attesters per slot; needs vc-*.log dumps
kurtosis/elscan.py <el-rpc-url>    # blob gas per block
```

## Log lines that are normal

- One CL config parse error per node at start. The template has keys this
  build does not know. The node continues.
- Repeated `Could not connect to execution endpoint ... wanted chain ID X, got Y`.
  The deposit poller compares the EL chain id with DEPOSIT_CHAIN_ID in the CL
  config, and they differ in the enclave. Only deposit following stops.
  All validators are in the genesis state, so this network does not use deposits.
  The engine API connection is separate and works.
