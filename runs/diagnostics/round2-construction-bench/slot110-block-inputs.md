# Observer-reported slot-110 block inputs

These records are the FFG attestations **included in the observed block**, not a dump of the slot-110 owner's proposal pool. Both retained observers report identical content. Slot 110 and head 109 are in epoch 3 and FFG round 13; those are different units.

## Block 110

The block contains 8 FFG attestations, 97,093 participant positions, and 85,775 unique validator indices across the eight sets. All seat counts equal the parsed validator counts. Its sync aggregate has 511 set bits on both observers.

| att slot | inclusion slots | seats | voted block root | data root | validator-set SHA-256 | node 1 / node 400 lines |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 109 | 1 | 14,927 | `0x4089c120c8455c58ef9cba4466bbbfa375c45b8b2140b3217473c485924f0609` | `0x241e649e7beca80b117effe6a1654b38fba44b30577725ff8202c0abca4eb6` | `9268757f0a3d9bcf6eb9d6b0b090138dbb6aa7ccee279ed29c7b23ecc13ca4b9` | runs/round2/prysm-geth-1/beacon.log:1331602 / runs/round2/prysm-geth-400/beacon.log:605301 |
| 107 | 3 | 14,924 | `0x640b2eaa3b8c9c8d9a551e5c227c8b703ce2cc767482f617ba3fa3f5aaf7f223` | `0xd0fdac2d2ff3b1742d349b7a77ae9d78fcd5d590d0709e2b31ae49ca84ba35` | `a85ccad857bbfc14a96cfd7846614710b23bfdc097917c86f7e040cb84012b2f` | runs/round2/prysm-geth-1/beacon.log:1331603 / runs/round2/prysm-geth-400/beacon.log:605302 |
| 102 | 8 | 7,139 | `0xa9ec98f85f1328a8b1290568f2270cde08bd842d0b9b7f9c99767ffc98ffe8d2` | `0x7cf4159e30221bdea92f650ac9668db06b99d6679cba4ad5e1401de6a5d395` | `070cb7640123ea77f7f9da18d5e318234ba4f08c34702e77660ff89374d89e30` | runs/round2/prysm-geth-1/beacon.log:1331604 / runs/round2/prysm-geth-400/beacon.log:605303 |
| 106 | 4 | 11,148 | `0xeb187de1dc8753c242d33810dbd495e34d939f559b49e00bf141470d804e7aae` | `0x9d7f8c0d55d590ba8dc14664440914051b1f1567f462f060a77714548d384c` | `25ddd26df09cfb99284884023cf656359034613789ec5f3cf2335618adfb04bd` | runs/round2/prysm-geth-1/beacon.log:1331605 / runs/round2/prysm-geth-400/beacon.log:605304 |
| 97 | 13 | 14,927 | `0xc3da59207794daa351ec2af7830805288c79cb20f717f2f527d0b8f0eb537237` | `0x1ea7e328e490282bb69beafca9fe6e8febb13d14e838b31c42a9bb1061018b` | `476bc4c51869259c8e4c10267a6c5ebe8b5923e2d94063dd07c2efac0cb350c1` | runs/round2/prysm-geth-1/beacon.log:1331606 / runs/round2/prysm-geth-400/beacon.log:605305 |
| 100 | 10 | 14,921 | `0x4059d3fba97aed35f8b821796903914a723def0f5c067e4c7203c2623d2a0f2a` | `0x99f45df09d3accd7a4d1f84679d9220546a17fc14a7c8b2efd86bbbc8a9b1c` | `392f9ee280f5a21526b06154215b4ec0e958cc513cc373f12060a93cffbc9746` | runs/round2/prysm-geth-1/beacon.log:1331607 / runs/round2/prysm-geth-400/beacon.log:605306 |
| 101 | 9 | 11,318 | `0x4059d3fba97aed35f8b821796903914a723def0f5c067e4c7203c2623d2a0f2a` | `0xca824abdbd8c72c0b4d04f271bc9002e62848d9458b51937bec26e51a0e361` | `c06a1a9c10d1bd29a63a0e0dd35d2d35cebf9b4071cab85274f23dc728ac34a8` | runs/round2/prysm-geth-1/beacon.log:1331608 / runs/round2/prysm-geth-400/beacon.log:605307 |
| 102 | 8 | 7,789 | `0x4059d3fba97aed35f8b821796903914a723def0f5c067e4c7203c2623d2a0f2a` | `0x90587ca9b1188b8c2c7337c13e5fb49ff515cdf49516817c40757de8fc62c5` | `7c03fa1eaa3688ec786917d61acd9fdd220ea61d22029d505fb8e12892d0a579` | runs/round2/prysm-geth-1/beacon.log:1331609 / runs/round2/prysm-geth-400/beacon.log:605308 |

The full validator-index arrays are in `slot110-block-inputs.json`. Large physical ledger records contain repeated capture-wrapper timestamps. The parser strips every wrapper timestamp before reading the CSV, records how many continuation prefixes it removed, and verifies that each record's repeated wrapper timestamps are identical.

## Exact repeated inclusion

An exact repeat requires the same attestation slot, voted block root, committee index, data root, and complete validator set. Signatures and committee-bit fields are not logged, so equality is limited to the complete logged identity.

| Compared with | repeated attestations | repeated participant positions | share of block-110 positions |
| ---: | ---: | ---: | ---: |
| block 109 | 3 / 8 | 26,246 | 27.03% |
| block 108 | 4 / 8 | 48,308 | 49.75% |
| either block 108 or 109 | 5 / 8 | 56,097 | 57.78% |

Repeated block-110 identities:

- attestation slot 107, 14,924 positions, repeated in block(s) 108; data root `0xd0fdac2d2ff3b1742d349b7a77ae9d78fcd5d590d0709e2b31ae49ca84ba35`.
- attestation slot 102, 7,139 positions, repeated in block(s) 108, 109; data root `0x7cf4159e30221bdea92f650ac9668db06b99d6679cba4ad5e1401de6a5d395`.
- attestation slot 97, 14,927 positions, repeated in block(s) 108; data root `0x1ea7e328e490282bb69beafca9fe6e8febb13d14e838b31c42a9bb1061018b`.
- attestation slot 101, 11,318 positions, repeated in block(s) 108, 109; data root `0xca824abdbd8c72c0b4d04f271bc9002e62848d9458b51937bec26e51a0e361`.
- attestation slot 102, 7,789 positions, repeated in block(s) 109; data root `0x90587ca9b1188b8c2c7337c13e5fb49ff515cdf49516817c40757de8fc62c5`.

## Observer agreement and anchors

Node 1 and node 400 each yielded 24 included records across blocks 108-110, with zero parse gaps and exact content agreement for all three blocks. The slot-110 state-transition anchors are runs/round2/prysm-geth-1/beacon.log:1331611 and runs/round2/prysm-geth-400/beacon.log:605310.

The observers establish block contents after publication. They do not establish which other aggregates or raw singles were present when the owner took its proposal-pool snapshot, which candidates were rejected during packing, or the precise owner-side construction timing.
