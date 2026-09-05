# Round-2 owner-local early timeline

## Scope and method

`extract_owner_early_timeline.py` freshly scans only the compact archives of
the 16 owners of slots 1--16.  Its inclusive window is
`2026-09-05T01:29:48Z` through `01:34:00Z`, from genesis minus 12 seconds to
genesis plus 240 seconds.  It reads validator, beacon, execution, and Engine
API snooper logs.  Every output carries an archive member and original line
number.  Snooper HTTP headers are neither parsed nor emitted; only selected
request/response metadata and JSON body fields are retained.

The complete message-type census has 1,328 node/component/severity/message
groups.  The exhaustive WARN/ERROR ledger has 15,448 records.  The compact
per-owner/per-slot view has 352 rows (slots -1 through 20), and the validator
role table has 1,275 rows.  The role table includes all 512 startup `Duties
schedule` rows (32 per owner), all 16 `Schedule for epoch 0` rows, and 747
submitted/skip summaries.  The startup schedule format contains
`attesterCount`, `proposerCount`, and `ptcCount`; it does not contain a
`syncCount` or `syncCommitteeCount` field, so `sync_count` is blank rather than
inferred.

## Phase boundary across all owners

Every owner BN independently obtained at least one successful
`engine_forkchoiceUpdatedV4` response with non-null payload attributes and a
payload ID for its assigned proposal slot.  The slot-local response durations
are 0--24 ms.  This rules out failure to prepare the EL payload ID as the
common cause of slots 1--14.  These asynchronous FCU preparations do not prove
that the corresponding VC proposal request reached the BN handler.

The next observed phase separates the failures:

* Slots 1--4, 7, 11, and 12 never enter BN `Building block`; the owner VC ends
  with a deadline-expired proposer preflight/RANDAO path.
* Slots 5, 6, 8, and 9 enter BN build and call `engine_getPayloadV6`.  The
  proxy writes a complete slot-matched execution-payload body into its
  downstream response abstraction in 0--22 ms, with no logged copy failure.
  Each BN later maps its client call to `timeout from http.Client` and
  fails because there is no cached P2P bid.
* Slot 13 enters BN build but fails processing the parent state before any
  `engine_getPayloadV6` request.
* Slots 10 and 14 receive their local payload and choose it, then miss the VC
  deadline while computing the state root.  Their BN work continues after the
  slot boundary.
* Slots 15 and 16 receive the payload, finish the block, and the VC submits it.

The phase split is derived from `owner_failure_neighborhood.tsv` and
`engine_rpc_timeline.tsv`; exact raw lines appear in
`owner_timeline_excerpts.md`.

## What the 300 ms error means

At exact round-2 revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`,
`beacon-chain/execution/engine_jsonrpc.go:117-118` defines
`gloasGetPayloadTimeout = 300 * time.Millisecond`.
`engine_jsonrpc.go:303-307` selects it for Gloas, and lines 319--324 create a
deadline from that duration and pass the resulting context directly to
`s.rpcClient.CallContext`.  Lines 325--326 pass any error to
`handleRPCError`.  `beacon-chain/execution/jsonrpc_error.go:35-40` maps any
error implementing `Timeout() == true` to `ErrHTTPTimeout`; lines 96--107 give
that error the literal text `timeout from http.Client`.

The authenticated Engine connection does also use an `http.Client`, but its
historical timeout is 30 seconds: `network/auth.go:11-12` defines
`DefaultRPCHTTPTimeout = time.Second * 30`, and
`network/endpoint.go:122-137` installs that client in geth RPC.  Therefore the
300 ms limit in these four calls comes from the inner GetPayload context, not
the outer HTTP client's configured timeout.  The literal error text is a
generic mapping and does not identify which deadline fired.

The proxy's synchronous upstream call and downstream `io.Copy` interval ended
well before the BN logged the mapped error:

| Slot / node | Proxy duration | Proxy response to BN timeout log |
|---|---:|---:|
| 5 / 118 | 3 ms | 1,869.824 ms |
| 6 / 83 | 2 ms | 308.064 ms |
| 8 / 19 | 0 ms | 810.412 ms |
| 9 / 107 | 22 ms | 1,507.219 ms |

These values are in `getpayload_timeout_discriminators.tsv`.  Exact snooper
source performs `io.Copy` before recording the duration, and none of these
calls has its copy-error log.  There is no explicit flush, however, and its
response log is asynchronous.  The records therefore prove fast upstream
response and successful writing into the downstream response abstraction,
not delivery to or consumption by the BN.  Together with the Prysm source
path, the evidence localizes the reported timeout to completion of
`CallContext` under its 300 ms context; it cannot distinguish proxy
flush/delivery, context-bound HTTP read/envelope decoding, or runtime
scheduling in the remaining interval.

The same delayed-post-response pattern appears before the BN logs successful
FCU preparation.  For example, node 118's snooper response for payload ID
`0x0489a8a1998c6091` completes at `01:30:52.342119`, while the BN logs the same
ID at `01:30:58.010429` (`beacon.log:522`).  Node 83's corresponding times are
`01:31:06.491206` and `01:31:07.247511`.  This is evidence that the proxy's
recorded copy interval is not an upper bound on when an overloaded BN visibly
resumes after the RPC.

## Owner process progress around proposal failure

The owner-local records reject a whole-process-stop interpretation.  Nodes 83
and 107 successfully submit attestations and sync messages in their failed
proposal slots.  Nodes 118 and 19 submit a sync message for the same slot only
2.032 ms and 2.656 ms after the slot boundary, respectively, while their BN
build paths are still returning failure.  Nodes 35 and 85 submit successful
validator-role output 4.982 ms and 2.477 ms after their VC proposal deadlines,
while the BN continues state-root work much longer.  Every failed owner emits
some later successful role output within this 240-second window.  This proves
that a blocked proposer phase coexisted with progress in other role handlers;
it does not identify which mutex, RPC, or scheduler queue consumed the missing
time.

The many validator errors must not be read as concurrent RPC counts.  For
example, node 169's slot-1 schedule contains 75 attesters and its log contains
75 `Could not request attestation to sign at slot` records, but the logs carry
only client-side terminal records and no server admission/completion markers.
The table reports exact records, not inferred simultaneous requests.

## Network and logging limits

`Connected peers` is periodic rather than a per-slot tick.  Every owner has a
positive observed peer total in the window; observed totals range from 28 to
142 around the early failure interval.  A peer-health line can rule out zero
peers at that sampled instant but cannot establish peer availability for every
role request.  The four compact archives have no runtime traces, goroutine
dumps, scheduler profiles, or server-side gRPC admission spans, so the exact
unlogged wait inside VC preflight and the exact post-proxy wait inside
`CallContext` remain unresolved by historical logs alone.
