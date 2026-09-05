# Ten-node historical-image startup control

- Enclave: `diag-startup10-20260905`
- Genesis: `2026-09-05 07:43:53 UTC`
- Images: cached `ethpandaops/prysm-{beacon-chain,validator}:sukun-decoupled-consensus`
- Topology: 10 nodes x 13 keys = 130 validators, six attestation subnets,
  target 64 aggregators per committee, 12-second slots.
- Differences from the failing captures: 130 rather than 120,000 validators;
  no Xatu SSE subscriber; no Spamoor/Dora; empty execution payloads.

| slot | REST block attestations | payload attestations | imports | import offset across nodes | payload-envelope offset |
|---:|---:|---:|---:|---:|---:|
| 0 | 0 | 0 | genesis | n/a | n/a |
| 1 | 1 | 1 | 10/10, same root | 106.7--137.7 ms | 140--198 ms |
| 2 | 2 | 1 | 10/10, same root | 67.7--74.9 ms | 75--117 ms |
| 3 | 4 | 1 | 10/10, same root | 67.9--78.4 ms | 72--122 ms |

The validator logs report 16, 16, 16, and 17 attestation pubkey records for
slots 0--3 respectively, with no slot-tagged errors. Node 1's slot-2 aggregate
selected 14 seats from two observed data groups (14 and 2 seats), and its PTC
records show `payloadPresent=true`. Across all ten nodes, slots 0--2 logged 160
PTC message records per slot (16 local and 144 gossip); slot 3 logged 170 (17
local and 153 gossip). These are repeated node observations, not unique votes.

The retrieved execution envelopes for slots 1--3 contain block numbers 1--3,
but zero transactions, zero gas used, and zero blobs because traffic services
were intentionally omitted. This control therefore proves that the exact
historical images progress cleanly at 130 validators; it does not demonstrate
that the 120,000-validator stall is fixed, and omitting Xatu removes the
unbuffered combined state/operation SSE subscriber present in the failing runs.

Service logs and specs are retained in `kurtosis-startup10-evidence/`.
Generated volumes (including disposable validator keys and genesis state) were
moved to `/tmp/prysm-startup10-generated-volumes/` to avoid adding them to the
diagnostic jj change. No original run evidence was moved or removed.
