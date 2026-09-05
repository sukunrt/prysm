# 10-node / 596-key startup control

Run `diag-startup10-596keys-20260905` used 10 Prysm CL/VC pairs, 596 keys per VC (5,960 validators), genesis at `2026-09-05 07:55:56 UTC`, six attestation subnets and a target of 64 aggregators per committee. Both Prysm clients are the exact historical round-2 build `0280403c70d88967f49d2d4c730f4c5417dabdf5` ([node 1 CL evidence](kurtosis-startup10-596keys-evidence/logs/cl-01-first4.log#L1)). The run has no Xatu and no transaction workload. It therefore controls per-VC key fanout, but not the original 120,000-validator registry, 1,000-node gossip fan-in, or Xatu feed subscriber.

## Outcome

All ten nodes imported the same block in slots 1, 2, and 3; no missed block, fork, `Could not`, `Failed to`, or `DeadlineExceeded` record occurred in the slot-0--3 window.

| Slot | Common root | Import offset across 10 CLs | Block FFG / payload attestations | Envelope arrival across 10 CLs |
|---:|---|---:|---:|---:|
| 1 | `0x7fcf1159...` | 629.8--849.3 ms | 1 / 1 | 516--994 ms |
| 2 | `0x8ed04b0d...` | 174.1--286.3 ms | 2 / 1 | 300--529 ms |
| 3 | `0x5ecf3b2a...` | 162.1--342.2 ms | 2 / 1 | 401--788 ms |

Node 1's exact import, transition, and envelope records are at [CL lines 4--14](kurtosis-startup10-596keys-evidence/logs/cl-01-first4.log#L4). Every envelope had zero transactions and zero gas, so this does not exercise a loaded engine path.

## Validator work

Each slot had exactly 745 FFG duties globally (the sum of the ten reported submission pubkey records), in one committee. Per-VC counts and actual `submittedSinceSlotStart` were:

| VC | slot 0 | slot 1 | slot 2 | slot 3 |
|---:|---:|---:|---:|---:|
| 1 | 71 / 2.674 s | 80 / 209 ms | 68 / 317 ms | 74 / 121 ms |
| 2 | 75 / 2.743 s | 73 / 190 ms | 79 / 204 ms | 74 / 172 ms |
| 3 | 77 / 2.329 s | 77 / 160 ms | 70 / 401 ms | 69 / 244 ms |
| 4 | 80 / 2.224 s | 67 / 196 ms | 69 / 76 ms | 75 / 76 ms |
| 5 | 71 / 3.397 s | 64 / 102 ms | 90 / 130 ms | 74 / 122 ms |
| 6 | 71 / 3.163 s | 76 / 173 ms | 66 / 181 ms | 79 / 176 ms |
| 7 | 64 / 2.511 s | 77 / 117 ms | 75 / 101 ms | 72 / 138 ms |
| 8 | 67 / 2.734 s | 68 / 255 ms | 85 / 180 ms | 85 / 184 ms |
| 9 | 86 / 3.063 s | 73 / 84 ms | 74 / 85 ms | 71 / 98 ms |
| 10 | 83 / 2.977 s | 90 / 123 ms | 69 / 108 ms | 72 / 123 ms |

These are observed submission records, not proof of unique on-chain inclusion. Node 1's raw slot records are [VC lines 11--14](kurtosis-startup10-596keys-evidence/logs/vc-01-first4.log#L11). Its cold startup initialized gRPC at `07:51:46.30` and emitted the full epoch schedule by `07:51:46.74`, about 440 ms later; slots 0--3 appear in [lines 3--10](kurtosis-startup10-596keys-evidence/logs/vc-01-first4.log#L3).

The first four node-1 FFG aggregate selections had one group/committee and chosen seats 71, 691, 745, 745 ([CL lines 3, 7, 11, 15](kurtosis-startup10-596keys-evidence/logs/cl-01-first4.log#L3)). Node 1 observed 373, 373, 380, and 367 PTC vote log records for slots 0--3 (local plus gossip), while protocol-wide PTC duty size is 512. Its Goldfish decision ledger contained 512 records per slot: slot 0 dropped all 512 because no payload was present, and slots 1--3 accepted all 512. These node-local observations must not be summed across peers.

## Runtime capture and interpretation

At 90 seconds before genesis, CL CPU was 2.21--4.64% and memory 207.5--227.1 MiB. At genesis it was 14.73--30.93% and 239.7--275.7 MiB; after slot 3 it was 2.16--3.13% and 263.2--300.3 MiB. VC CPU stayed at or below 1.28%; post-slot-3 VC memory was 53.2--74.0 MiB. Raw snapshots and the final Prometheus scrapes are in [`kurtosis-startup10-596keys-evidence`](kurtosis-startup10-596keys-evidence/).

This healthy result rules out 596 keys per VC alone as sufficient to reproduce the startup stall in this 10-node topology. It does **not** establish a safe threshold: relative to the original run it has about 20x fewer global FFG duties/gossip inputs, 100x fewer network nodes (about 9 connected peers per control node versus roughly 70 in the original), and no Xatu, despite matching the per-VC FFG duty fanout. Conversely, each VC owns a larger fraction of the fixed 512-member PTC here than in the 120,000-validator run.
