# Proposal packing integration, 2026-10-05

The reviewed packing changes passed a ten-node Prysm/geth run with 1,000 active
validators and eight observed proposal slots. All ten nodes agreed on slot 8,
reported zero sync distance, and were neither syncing nor optimistic. Every
observed block included attestations and one payload attestation. No proposal
or packing failures appeared in the captured beacon logs.

The paired CPU results and implementation are in
[the task report](../../task-proposal-packing.md). This network test verifies
integration at this scale; it is not a before/after CPU benchmark.

## Observed blocks

Genesis was `2026-10-05T14:14:37Z`. Head observations ran through
`2026-10-05T14:16:21Z`, followed by collection of the eight fixed blocks.
All ten nodes returned slot-8 root
`0x01244d9b2d402aa468d6de1e08d8a3af10de9217b969cd333fcc6900d6206e19`.

| Block slot | Beacon attestations | Unique participants from preceding slot | EL transactions | Blobs | Construction time |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1 | 13 / 125 | 0 | 0 | 31.57 ms |
| 2 | 2 | 125 / 125 | 8 | 0 | 16.30 ms |
| 3 | 2 | 125 / 125 | 21 | 1 | 18.64 ms |
| 4 | 3 | 125 / 125 | 4 | 1 | 18.29 ms |
| 5 | 1 | 125 / 125 | 5 | 1 | 23.01 ms |
| 6 | 2 | 125 / 125 | 5 | 1 | 28.39 ms |
| 7 | 2 | 125 / 125 | 5 | 1 | 21.66 ms |
| 8 | 2 | 125 / 125 | 5 | 1 | 43.48 ms |

Participant counts union the aggregation bits of all included attestations for
the preceding slot, with the SSZ delimiter removed. There is one 125-seat
committee per slot. In block 4 the 117-seat and 8-seat candidates together
cover all 125 positions. Older slot-0 votes are excluded from the fresh count.
Block 1's partial slot-0 coverage is consistent with the separate, unchanged
slot-0 gossip acceptance issue. This change does not fix that issue.

Construction time is the difference between the existing `Building block`
and `Finished building block` offsets on each proposer: median **22.34 ms**,
maximum **43.48 ms**. It covers the handler work between these messages, not
just attestation packing. All eight finishes occurred within 79 ms of their
slot start. The finalized and justified checkpoints remained at round/epoch 0;
the eight-slot startup window does not demonstrate finalization.

## End-to-end slot check

Block 8 contains two beacon attestations and one payload attestation. Its
execution envelope and geth's block-by-hash response agree on five transactions
and 131,072 blob gas (one blob), at execution hash
`0x66c76cca1c98c06dd2aacc35f28d3dc306f72b43a0b0615f1a471bc1a269dc62`.

The included slot-7 FFG attestation covers all 125 committee members. Ten
matching aggregate ledger lines across the ten beacon logs name its slot and
beacon root; for example `cl-01-prysm-geth.log:5978` records 125 seats and data
root `0xbd029cb24891401086adb3619b755f2b5b0eeb5a25705ffc8d58ce928975a2`.
The payload attestation names slot 7's beacon root
`0x2a3106278adbb2e3a22d9c95ecd1d9b89730a357fc16a034606c9583baedc20a`.
Its corresponding PTC ledger has 1,250 matching lines across nodes, representing
125 distinct validators; an example is `cl-01-prysm-geth.log:5979` with
`payloadPresent=true` and `blobDataAvailable=true`. These are copies observed
across ten nodes, not 1,250 distinct voters.

## Build and replay

- Configuration: [network_params.proposal-packing.yaml](../../kurtosis/network_params.proposal-packing.yaml).
- Enclave: `proposal-packing-20261005`, UUID `4b0404408c0d4c72acef67158209ba1f`.
- Ten beacon/validator pairs, 100 keys each; mainnet preset, Heze from genesis,
  eight-slot rounds, 12-second slots, supernodes enabled.
- Baseline parent: `072575178699676ca050d134db0222fef75717ff`.
- Local ethereum-package: `0350d2e98735ff395b088fe8c261f0c9bb652c8a`.
- Go `1.26.5`; Kurtosis `1.18.1`; geth image
  `ethpandaops/geth:glamsterdam-devnet-8`; Spamoor `v1.2.3`.
- Beacon image: `sha256:ada756760ff50f9b5a36b457721052c6495b8d32f0db9801d74d0eb96c4d2994`.
- Validator image: `sha256:78caa3a6cca6afd2a133fffe9eb78d506268b4bba3704194b6e2ba77fa2da9f1`.
- Genesis-generator image: `sha256:a9937eaa9dca43183af58718833b9ff5646ea41d87577cd06c9fef39ad64f32d`.

All ten running beacon image IDs and all ten validator image IDs were checked
against the final builds. The source hashes were unchanged after review:

```text
proposer_attestations.go f32b66d05713b11e60346d7add0bdd7d587a550e8888fd6f8312bf1e1f5d3625
core/electra/attestation.go c6af516089dbd6114310fcf937acb2937fa12528ad9fe955a46c9b5bca0ccea9
```

Build scripts, binaries, build information, source snapshots/hashes, full logs,
REST responses, execution blocks, image metadata and observation scripts are
retained in `/home/sukun/.cache/prysm-proposal-packing/2026-10-05/kurtosis/`.
`checks.json`, `audit.json` and `fresh-coverage.json` contain the summarized
checks. Spamoor's `/api/clients` confirmed `01-geth-prysm` is the configured
ready client; both the transaction and blob spammers started successfully.

```bash
bash /home/sukun/.cache/prysm-proposal-packing/2026-10-05/kurtosis/build.sh beacon-chain validator prysmctl
kurtosis run --enclave proposal-packing-20261005 /home/sukun/dev/ethereum-package --args-file kurtosis/network_params.proposal-packing.yaml --verbosity brief
python3 -u /home/sukun/.cache/prysm-proposal-packing/2026-10-05/kurtosis/observe.py proposal-packing-20261005
python3 /home/sukun/.cache/prysm-proposal-packing/2026-10-05/kurtosis/audit.py
kurtosis enclave stop proposal-packing-20261005
```

The collector initially received geth's public port without an `http://`
scheme. That collector error was fixed, and geth was queried using the same
eight saved block hashes. Original slot-8 node observations and logs were
preserved, along with `checks-initial-url-error.json`; no network rerun was
needed. The audit parser also handles the beacon's colored text log format.

Beacon startup logs contain ten reports of unrecognized generated YAML fields and
159 EL follow-distance messages while the execution chain was still at block
0. Those are the only beacon error categories in the captured logs. There
were no proposal failures during slots 1–8. Validator logs contain only the
same ten startup YAML reports at error level. The test enclave was stopped after
collection; shutdown completed at `2026-10-05T14:19:29Z`.
