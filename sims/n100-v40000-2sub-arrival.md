# Arrival delay, Shadow run n100-v40000-2sub, slots 3 to 5

100 nodes, 20 supernodes, 40k validators, 8-slot rounds, 2 committees of 2500
on 2 subnets, every node on both subnets, 16 aggregators per committee,
aggregate due at 6000 ms. Milliseconds after slot start, receptions on all
100 nodes.

## FFG votes

| slot | p50 | p90 | p99 | p99.9 | max |
|---|---|---|---|---|---|
| 3 | 675 | 3401 | 5696 | 7240 | 8466 |
| 4 | 614 | 3475 | 5473 | 6509 | 6947 |
| 5 | 668 | 3506 | 5552 | 6675 | 7416 |

## FFG aggregates

| slot | p50 | p90 | p99 | p99.9 | max |
|---|---|---|---|---|---|
| 3 | 6068 | 6510 | 6718 | 6874 | 6874 |
| 4 | 6079 | 6579 | 6814 | 6835 | 6835 |
| 5 | 6079 | 6508 | 6924 | 6944 | 6944 |

## Goldfish head votes

| slot | p50 | p90 | p99 | p99.9 | max |
|---|---|---|---|---|---|
| 3 | 357 | 1172 | 2170 | 3323 | 3692 |
| 4 | 466 | 1621 | 2910 | 3312 | 3418 |
| 5 | 333 | 1213 | 2520 | 2987 | 3080 |

## PTC votes

| slot | p50 | p90 | p99 | p99.9 | max |
|---|---|---|---|---|---|
| 3 | 539 | 2023 | 3456 | 4394 | 5739 |
| 4 | 582 | 1800 | 2874 | 3697 | 5331 |
| 5 | 401 | 1451 | 2661 | 3457 | 4461 |

## Blocks

One reception per node per slot, so p99.9 equals max.

| slot | p50 | p90 | p99 | max |
|---|---|---|---|---|
| 3 | 77 | 151 | 182 | 182 |
| 4 | 89 | 144 | 178 | 178 |
| 5 | 87 | 152 | 191 | 191 |

## Execution payloads

| slot | p50 | p90 | p99 | max |
|---|---|---|---|---|
| 3 | 424 | 1081 | 3729 | 3729 |
| 4 | 292 | 508 | 789 | 789 |
| 5 | 284 | 1006 | 2113 | 2113 |

## Data column sidecars

Slot 5 had no blobs.

| slot | p50 | p90 | p99 | p99.9 | max |
|---|---|---|---|---|---|
| 3 | 74 | 172 | 196 | 220 | 241 |
| 4 | 83 | 150 | 199 | 232 | 246 |
