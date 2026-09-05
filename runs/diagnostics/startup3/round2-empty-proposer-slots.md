# Round 2 empty proposer-duty slots

## Result

Slots 162, 171, 177, and 182 are holes in the logged retained proposer-duty
snapshots across the captured validator clients, not missing validator keys and not an
artifact of the original indexer's `slot=` requirement.  This does **not** mean
that the protocol had no proposer: later beacon-node views could compute a
proposer independently of the VC's cached epoch duties.

## Complete-key and logging checks

A scan of all 1,000 validator archives found exactly 120,000 distinct
`Validator activated` indices: the complete interval 0 through 119,999, with
120,000 activation rows total.  Thus no registry validator was absent from the
VC fleet.

Every VC logged at least one `Schedule for epoch 5` summary.  There are 1,231
such summaries because duty refreshes caused some nodes to log more than once;
33 summaries have a nonzero proposer count, totalling 38 proposer assignments
across the snapshots.  Logging was therefore active during the relevant epoch.

The four target slots have plentiful ordinary duty rows but no proposer field:

| slot | duty rows | distinct VCs | summed attesters | summed PTC keys | proposer rows |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 162 | 362 | 326 | 17,254 | 590 | 0 |
| 171 | 342 | 309 | 17,218 | 593 | 0 |
| 177 | 351 | 319 | 17,243 | 586 | 0 |
| 182 | 350 | 319 | 17,318 | 604 | 0 |

These sums include repeated refresh snapshots and are evidence of logging
coverage, not committee cardinalities.

## Proposer-only logger blind spot accounted for

At exact Round 2 revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`,
`validator/client/duties.go:780-905` adds `proposerPubkey` for a proposer but
adds `slot` only inside the attester branch.  A proposer-only row consequently
has no `slot=`.  The original union parser missed such rows.

The corrected scan examined every raw `Duties schedule` row containing
`proposerPubkey`.  For proposer-only rows it recovered the slot from the outer
timestamp plus `timeUntilDuty`, allowing 1.1 seconds for the logger's
`(time.Until(startTime) + 1s).Truncate(1s)` rounding.  Every proposer row was
mapped; none mapped to any of the four target slots.  In this corpus all 321
proposer rows also happened to contain `slot=` because the large proposer
wallet had an attester at that slot, so the blind spot did not alter any
historical owner.  The reusable union parser now nevertheless performs the
proposer-only recovery.

## Where dispatch is lost

The runner order in exact-R2 `validator/client/runner.go:95-156` is duty update
at an epoch boundary, proposer-settings push, `RolesAt`, then `performRoles`.
`logDuties` runs as part of a successful `UpdateDuties` before `RolesAt`.
`RolesAt` in `validator/client/validator.go` emits `RoleProposer` only from a
cached `duty.ProposerSlots` match.  Therefore a target slot absent from every
logged proposer snapshot has no VC proposer role to dispatch on those
snapshots; this is earlier than proposal preflight or RANDAO signing.
However, `RetryMissingNextDuties` can merge next-epoch duty-store changes
without calling `logDuties`. The logged snapshots therefore do not enumerate
every runtime snapshot, and the precise no-dispatch boundary for these slots
is an inference rather than directly instrumented evidence.

The source contains a concrete way to create a missing proposer component.
The post-Gloas split-duty fetch in `validator/client/duties.go` fetches attester
and proposer duties independently.  `dropIfDivergent` discards a proposer
response whose dependent root disagrees with the attester response, leaving
proposer duties missing for later retry.  Across the archives there are 378
`Duties have divergent dependent root, treating them as missing` warnings;
377 of the 378 occur after 01:55 UTC, during the post-rollback portion of the
run.  This establishes
that the guard fired at scale.  It is a source-and-log-supported mechanism for
the empty snapshots, although the warning does not name individual affected
slots and therefore is not a per-slot proof by itself.

## Independent BN computation

Beacon nodes did compute attached proposers for two holes:

- node 106 `beacon.log:1068` logs payload attributes for `nextSlot=162`, on
  head slot 117;
- node 118 `beacon.log:1061` logs payload attributes for `nextSlot=177`, also
  on head slot 117.

The `getPayloadAttribute` / `trackedProposer` path only proceeds when the
computed proposer index is present in that BN's subscribed-validator cache.
Those entries prove that these later BN views recognized a locally attached
proposer, while no VC snapshot recorded the corresponding proposer duty.
There is no equivalent payload-attribute row for 171 or 182; that absence is
inconclusive because the late-build path also depends on head/update timing.

The strongest bounded conclusion is no logged proposer assignment or attempt,
with a VC duty-distribution hole under divergent views as the supported
explanation. Missing configured keys are ruled out, and no proposal RPC timeout
is logged. Determining the exact execution-time duty snapshot or which
dependent-root response owned each missing slot would require response-level
traces that the historical logs do not contain.
