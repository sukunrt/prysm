# Slot 15/16 Goldfish committee validation

The validator sets below are recomputed with the production committee rule: SHA-256 of `decoupled_mock_goldfish_committee || uint64(slot, big-endian)`, the first eight digest bytes interpreted big-endian modulo 120,000, and the 512 consecutive validator indices from that offset.

Owner mappings were freshly read from `Validator activated` records across the 1,000-node archive union. No validator-to-node arithmetic was used.

| slot | offset / committee interval | owner intervals from activation logs | recorded observers |
|---:|---|---|---|
| 15 | 93514 / 93514-94025 | node 68: 54 (93514-93567); node 69: 458 (93568-94025) | 201, 400 (exact 512/512) |
| 16 | 27068 / 27068-27579 | node 93: 489 (27068-27556); node 94: 23 (27557-27579) | 201, 400 (exact 512/512) |

Both observers recorded every computed committee validator exactly once with one seat and outcome `accepted`. Each root/owner cohort is a contiguous subinterval. The 54/458 and 23/489 splits follow the shuffled activation ownership boundaries; the number of beacon nodes does not determine these seat counts.

The input ledger exposes accepted vote records. This validation does not establish a complete historical in-memory Goldfish store or payload-presence state.
