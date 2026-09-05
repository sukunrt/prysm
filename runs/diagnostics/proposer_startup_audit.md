# Slots 1--3 proposer-path audit

Scope: sampled validator/beacon logs only, round 1 revision `a1679c9f` and round 2 revision `0280403c`. The proposer pipeline files inspected are identical between these revisions.

## What the samples establish

Only one slot-1--3 proposer is owned by a sampled VC: round 1 node 3 owns slot 1, pubkey `0x86dd193d4943`, validator index 1554. Its epoch schedule declares that key as slot-1 proposer in [validator.log line 649](../round1/prysm-geth-3/validator.log#L649). No sampled VC owns round-1 slots 2 or 3, and none owns round-2 slots 1--3 (`proposerCount=0` in those sampled schedules). Thus absence of proposer messages in those logs is expected and cannot identify why the unsampled proposers missed. It is incorrect to generalize node 3's slot-1 failure to all six missing blocks.

For the one observable proposer:

| Event | Wall time | Offset from slot-1 start |
|---|---:|---:|
| Beacon `GetBeaconBlock` handler logs its first statement | 00:00:18.973 | +6.973 s |
| VC request deadline / `DeadlineExceeded` | 00:00:24.000 | +12.000 s |
| Beacon local-payload cancellation and terminal build failure | 00:00:31.522 | +19.522 s |

Evidence: handler entry and terminal error are [beacon.log lines 902 and 910--911](../round1/prysm-geth-3/beacon.log#L902); the VC deadline is [validator.log line 687](../round1/prysm-geth-3/validator.log#L687).

The observed failure mechanism is concrete: after the VC deadline had expired, local payload preparation against `snooper-engine:8561` ended with `context canceled`, and there was no cached P2P bid fallback, so no block was returned for signing. That terminal error is not the underlying cause of the latency: the logs do not identify what delayed dispatch to +6.973 s or what then held the handler before it reached graffiti generation roughly 11.1 seconds later.

## Dispatch-path bounds

The runner handles each slot in one select-loop iteration. Before launching any roles it synchronously pushes proposer settings and computes `RolesAt`; it then starts every role in a separate goroutine ([runner.go lines 105--156](../../validator/client/runner.go#L105), [245--269](../../validator/client/runner.go#L245)). Slot 1 is not an epoch boundary, so it does not synchronously refresh current duties. The mid-epoch `MaybeRetryMissingNextDuties` call starts its retry asynchronously and returns; it does not itself wait for the duties RPC.

`RolesAt` is nevertheless a concrete uninstrumented delay candidate. It serially computes selection proofs for the slot's attesters, then `localSelector.SyncCommitteeAggregators` serially processes the four sync-committee keys visible in node 3's logs. For each key it synchronously calls `SyncSubcommitteeIndex` and signs the returned selection data before `performRoles` can launch the proposer goroutine ([validator.go lines 558--655](../../validator/client/validator.go#L558), [aggregator_selector.go lines 132--174](../../validator/client/aggregator_selector.go#L132)). This establishes a head-of-line dependency, not that those four RPCs or signatures consumed the observed 6.973 seconds.

Once the proposer goroutine starts, it takes a per-role/key multilock, signs RANDAO, obtains graffiti, attaches a head freshness hint, and calls `BeaconBlock` ([propose.go lines 46--99](../../validator/client/propose.go#L46)). The logs contain no RANDAO, graffiti, freshness-hint, roles, or proposer-settings error for slots 1--3 in either run, but absence of an error does not bound how long successful work took. They also contain no timestamp immediately before the RPC. Therefore the +6.973 s beacon handler entry cannot be attributed precisely between:

1. delay before `performRoles` (slot-loop scheduling, proposer-settings, or role computation),
2. proposer-side multilock/signing/hint work, and
3. transport/server scheduling before the first handler log.

It is nevertheless bounded: the request reached the first line of `GetBeaconBlock` no later than +6.973 s. That log is emitted before sync/optimistic checks, parent-state access, proposer-index computation, or block building ([proposer.go lines 54--80](../../beacon-chain/rpc/prysm/v1alpha1/validator/proposer.go#L54)), so none of those server-side operations caused the initial 6.973-second interval.

The server then took roughly 11.1 seconds from its `Building block` entry log to graffiti generation. That interval covers the optimistic-status check, `getParentState` (including synchronous `UpdateHead`), and empty-block setup; packing has not started yet and therefore cannot cause this gap. `BuildBlockParallel` begins only after parent-state and proposer-index preparation ([proposer.go lines 80--120](../../beacon-chain/rpc/prysm/v1alpha1/validator/proposer.go#L80)). The server reported terminal failure about 12.55 seconds after handler entry. Cancellation was nominally due at +12 s but the beacon did not emit the canceled engine/pacing result until +19.522 s. This delayed observation is consistent with severe process scheduling/contention, but the logs do not distinguish scheduler starvation from an operation that was slow to honor cancellation.

## Ruled-out explanations for sampled slot 1

- Missing/stale proposer duties: ruled out; node 3 schedules the exact proposer key well before genesis and again immediately after genesis.
- VC initialization or sync wait still in progress: ruled out; duties and slot-0 role submissions precede slot 1.
- RANDAO signing, graffiti, head-hint, or role lookup returning an error: no corresponding failure log exists.
- Beacon rejecting the proposal as syncing or optimistic: ruled out for this request; the handler passed both explicit checks and entered block construction.
- A completed block failing only during VC block signing or publication: ruled out; `BeaconBlock` itself timed out and the beacon logged `Could not build block`.

## Limit

The surviving logs provide the failure mechanism for round-1 slot 1, but not the cause of its multi-second latency: the request missed its deadline, and the subsequent canceled local-payload attempt had no P2P fallback. Slots 2--3 in round 1 and slots 1--3 in round 2 were assigned to unsampled validators, so their validator-side dispatch paths are unobserved. Beacon-wide lack of blocks alone cannot tell whether those proposers failed before RPC dispatch, reached another beacon node, or encountered the same latency and build failure.
