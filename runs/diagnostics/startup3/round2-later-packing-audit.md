# Round 2 post-rollback packing audit

This audit tests whether the slot-129 rollback directly filled the proposer pool via
`saveOrphanedOperations`, and narrows what the later packing logs do and do not prove.
Source references are to historical round-2 revision
`0280403c70d88967f49d2d4c730f4c5417dabdf5`.

## `saveOrphanedOperations` is not the direct proposer-pool amplifier

The rollback from slot 128 to 117 traverses recent orphaned blocks and reinserts
their still-usable attestations (`beacon-chain/blockchain/head.go`,
`saveOrphanedOperations`). But the branch is version-sensitive. For Electra and
later blocks—including this Gloas run—it calls `SaveBlockAttestation`; it does not
call `SaveAggregatedAttestation` or `SaveUnaggregatedAttestation`.

With the normal pool selected, `packAttestations` reads only
`AggregatedAttestations()` and `UnaggregatedAttestations()`
(`beacon-chain/rpc/prysm/v1alpha1/validator/proposer_attestations.go:32-50`). It does
not read `BlockAttestations()`. The latter feed fork-choice preparation separately.
The experimental pool would merge this distinction, but its feature flag defaults
off and node 80's startup log contains no feature-enable message. Therefore the
deep rollback can amplify fork-choice work through restored block attestations, but
the exact source does **not** support claiming that it directly appended all
orphaned attestations to the candidate slice packed by these proposers.

## What proposer packing actually does

For both aggregated and unaggregated snapshots, packing performs these operations
before limiting the result to the block maximum:

1. `validateAndDeleteAttsInPool` linearly calls
   `VerifyAttestationNoVerifySignature` for every candidate. The outer `filter`
   loop has no explicit context check.
2. Invalid entries are deleted one at a time. Context is checked once per deletion,
   while aggregated deletion also updates seen caches, takes the aggregated-pool
   write lock, and scans the entries sharing the attestation-data ID.
3. Only after validation/deletion does packing deduplicate. `dedup` groups by data
   ID and pairwise tests containment within every group, so a group of `k`
   overlapping candidates has an O(k^2) worst-case comparison count. Aggregation,
   on-chain aggregate construction, a second deduplication, and reward sorting then
   run before `limitToMaxAttestations`.
4. The final signature filter applies only to the limited result. The initial
   validation is explicitly the no-signature variant, so the retained 12--81 second
   intervals cannot be described as BLS verification of every pool candidate.

For candidates whose target/source is already wrong for the post-rollback head,
`VerifyAttestationNoVerifySignature` rejects at its early round/checkpoint tests,
before `ActiveValidatorCount` and committee derivation. Candidates reaching those
later checks use a state advanced well beyond slot zero, so the genesis-only
active-count cache bypass is not present. Committee/index conversion can still be
per candidate, but the source offers no historical cache-miss or candidate-count
measurement.

The warnings around the rollback do not supply that missing count. For example node
80 reports attestations with `aggregatedCount=11881` and `14924`; this field is the
number of participating aggregation bits in an individual aggregate, not the number
of objects in the pool. It cannot be used as a 15,000-item loop bound.

## Connection to the owner logs

The seven ordinary later `GetBeaconBlock` deadline rows all choose a payload before
the caller deadline, then eventually converge on cancellation from the consensus
packing/join path. The retained post-deadline intervals are approximately 17.9 s
(136), 16.8 s (163), 17.5 s (176), 19.1 s (191), 81.3 s (193), 23.9 s (199), and
17.9 s (206). Slot 193/node 80 is particularly discriminating: it chooses its
payload at +5.990, the VC fails at +12.008, but invalid-attestation deletion is not
logged until +79.695 and packing until +81.348 (`beacon.log:1155,1157,1165-1167`).

Those messages establish that the packing goroutine had not returned at the caller
deadline and that cancellation was observed at/after deletion. They do not timestamp
entry into deletion, count candidates, or separate runnable starvation, pool-lock
wait, filtering, deduplication, and sorting. In particular, the late “Could not
delete invalid attestations” line is emitted only after `filter` has processed the
whole snapshot and `deleteAttsInPool` returns an error; it is not proof that the
write lock alone was held for the preceding interval.

## Bounded conclusion

The exact code supports a plausible repeated-work amplifier from a large or stale
normal gossip pool: validation occurs before deduplication and before the block-size
limit, invalid cleanup is per item, and same-data deduplication can be quadratic.
The post-rollback wrong-source aggregates visible in logs would take the cheap early
rejection path but could still add linear validation and deletion work if numerous.
However, neither archive logs nor source establish the pool object count or identify
which substage consumed the historical wall time. The stronger direct claim that
the rollback's `saveOrphanedOperations` populated the proposer candidate pool is
false for this Gloas/default-pool path.
