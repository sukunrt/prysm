# Round 2 epoch-5 proposer-duty holes

The complete 1,000-archive union has no proposer schedule, proposer attempt, or
import for slots 162, 171, 177, and 182. This is qualitatively different from a
logged scheduled proposer failing preflight: no logged retained duty snapshot
assigned a local key to these slots. A raw audit including proposer-only rows
without an explicit slot reaches the same result; all 120,000 unique activated
validator indices are represented.

## Direct source explanation for silence

`RolesAt` takes a snapshot of the VC duty store and adds `RoleProposer` only when a
local duty's `ProposerSlots` contains the current slot
(`validator/client/validator.go:558-588`). An initialized snapshot with no such
entry is not an error. The runner therefore emits neither a RANDAO error nor a block
request, matching the four all-archive holes. The archives do not timestamp
every `RolesAt` call or dump its in-memory snapshot, so the direct observation
is the assignment/attempt gap, not an instrumented proof of each absent dispatch.
In particular, `RetryMissingNextDuties` can merge next-epoch duties without
calling `logDuties`; the logged snapshots do not enumerate every runtime
duty-store revision.

The split-duty client constructs each local duty by overlaying a globally fetched
proposer map onto that client's attester rows. Importantly, attester and proposer
responses are separate concurrent RPCs. `dropIfDivergent` compares their dependent
roots and deliberately turns the proposer response into `nil` when they disagree;
`assembleDuties` consequently leaves every `ProposerSlots` slice empty for that
snapshot (`validator/client/duties.go`, `fetchAllDuties`, `dropIfDivergent`, and
`assembleDuties`). At an epoch boundary the client can also promote next-epoch
duties previously cached from its own view. Head dependent-root events trigger
`UpdateDuties`, and missing next duties are retried, so this is not accurately
described as “the VC never refreshes.” Refreshes can instead preserve, replace, or
temporarily drop proposer data according to the local head and the pair of response
roots.

The historical logs show this mechanism active after the rollback: across the
1,000 validator logs there are 378 `Duties have divergent dependent root` warnings,
377 of them from `01:55` onward. The warning rate remains substantial through the
epoch-5 fetch interval (43 at `01:56`, 51 at `01:57`, 50 at `01:58`, 38 at `01:59`,
25 at `02:00`, and 20 at `02:01`). There are also 755 logged current-dependent-root
updates and 216 previous-root updates over the capture. Thus the evidence is
inconsistent concurrent views/responses, not simple absence of refresh.

## Why the first complete holes appear in epoch 5

All four holes are in epoch 5 (slots 160--191). The same all-archive schedule census
shows view disagreement elsewhere in that epoch: two distinct owners at slots 160,
167, 168, 174, 186, and 187, and three at slot 166. Every slot in epoch 5 otherwise
has at least one scheduled owner. By contrast, the later epoch-6 deadline/invalid-
signature misses all have at least one owner; there is no ownerless hole before the
capture ends.

The source supplies a structural reason epoch 5 is the first strongly affected
proposer schedule. Post-Fulu proposer duties use the `ProposerLookahead` stored in
the supplied head state (`helpers.BeaconProposerIndexAtSlot` and
`ProposerAssignments`), rather than a separate beacon-node map keyed only by genesis
validator count. The lookahead is advanced at epoch processing. Its newly computed
epoch uses `Seed`, whose RANDAO input is
`epoch - MIN_SEED_LOOKAHEAD - 1`; with the configured minimum lookahead of one,
epoch-5 selection depends on the epoch-3 mix. The slot-129 rollback replaces head
slot 128 with slot 117 and crosses differing epoch-3 block/RANDAO histories. Local
heads can therefore legitimately carry different epoch-5 lookaheads even though
the validator registry is identical. Epoch-4 proposer selection depends on the
earlier epoch-2 mix, consistent with blocks immediately after rollback retaining a
mostly common schedule before disagreement becomes obvious at slot 160.

This also rules out the narrower hypothesis of a single stale BN proposer cache
keyed by the genesis active count: the historical path enumerates assignments from
the head state's proposer lookahead, and the observed multiple owners require
multiple state views or duty snapshots, not one global cached assignment.

## Causal extent

For the four named slots, the source-supported assignment-level hypothesis is:

1. the post-rollback network has divergent head-dependent duty views;
2. proposer lookahead for epoch 5 is branch-sensitive;
3. per-client duty snapshots either carry another view's proposer or have proposer
   data dropped on dependent-root mismatch;
4. across all 1,000 local-key clients, no logged retained snapshot assigns a
   local proposer for 162, 171, 177, or 182;
5. such a snapshot causes `RolesAt` to dispatch no proposer, consistent with
   the absence of every proposal attempt.

The archives do not identify one unique RPC pair or one canonical proposer that
“should” have acted at each hole, because that proposition itself depends on which
post-rollback state is chosen. A decisive reproduction would capture, per VC update,
the attester/proposer dependent roots plus the full proposer result root/view and
the stored epoch-5 `ProposerSlots`; ordinary duty schedule logs expose only the
locally retained assignments.
