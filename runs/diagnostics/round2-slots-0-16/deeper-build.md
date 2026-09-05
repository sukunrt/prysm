# Deeper build audit: proxy completion, timeout identity, and consensus-branch boundaries

This pass reads historical logs and source only. Prysm source is revision
`0280403c70d88967f49d2d4c730f4c5417dabdf5`; its `go.mod` selects
`github.com/ethereum/go-ethereum v1.17.5`. That dependency was read from
`/home/sukun/go/pkg/mod/github.com/ethereum/go-ethereum@v1.17.5`.
`/tmp/rpc-snooper-v0.0.21` is a clean checkout at
`e3bb3cb2f91eeaa35a30f2edfb224751b68aaa54`, matching the recovered snooper's
startup identity. No tests or experiments were run in this pass.

## What the proxy response record actually proves

The exact non-event-stream path in snooper `snooper/proxycall.go` is:

1. Start the call-duration clock, then `client.Do(req)` at lines 174–175.
2. Write upstream status to the downstream `http.ResponseWriter` at line 227.
3. Wrap the upstream body in a tee reader at lines 230–231.
4. Run `io.Copy(w, responseBodyReader)` at line 233.
5. Capture elapsed duration at line 238, even if that copy returned an error.
6. On error, return `proxy response stream error` at line 241.
7. Deferred reader `Close` drains remaining bytes to its log buffer, closes the
   upstream body, and starts a logging goroutine (`logging.go:130–163`).
8. That goroutine waits for request logging (`proxycall.go:308–312`), performs
   response processing, and finally logs the RESPONSE line
   (`logging.go:275–338`). `duration_ms` is the stored duration, truncated to
   milliseconds at lines 333–334, not response-log time minus request-log time.

Consequences:

- The response log cannot precede the `io.Copy` attempt and elapsed-duration
  capture. It is not merely “upstream response headers received.”
- Its timestamp can nevertheless occur before downstream `ServeHTTP` returns:
  deferred logging starts another goroutine and does not wait for that
  goroutine. There is **no explicit non-streaming `Flush`** in this path.
  `ResponseWriter.Write` completion is not a BN kernel-arrival or JSON-decoding
  event. Buffered output and later transport/runtime servicing remain outside
  the measured guarantee.
- The request and response log timestamps are also asynchronous logging events,
  not exact wire send/receive timestamps. The recorded `duration_ms` is the
  stronger measure of the synchronous proxy call/copy interval.
- A full logged body by itself would not prove a successful downstream copy:
  reader `Close` drains bytes after a failed copy too. The error path must be
  checked before using that stronger interpretation.
- JSON formatting, request-log ordering, and `processResponseModules` run in
  the detached response logger. They are not a synchronous module/logging
  barrier that forwarding must complete before returning from the proxy.

The last error branch was checked in all four full owner snoopers. Nodes 118,
19, and 107 have no `call failed`, `failed writing response`, or
`proxy response stream error` lines. Node 83 has an unrelated upstream
`proxy request error ... context canceled` at `01:31:34.007757592Z`, line 32063;
its failed slot-6 GetPayload exchange was at `01:31:16.147–16.149Z`.
No response-copy error is recorded for any of the four matched GetPayload calls.
`snooper.go:348–362` would log `call failed: proxy response stream error` if
`io.Copy` returned that error. Thus the full-body-after-copy-failure countercase
is source-real but unsupported for these historical calls; reproducing it
would not presently explain a historical observation.

The matched response records remain:

| Slot / owner | Request / response lines | Stored duration | Response timestamp |
| --- | --- | --- | --- |
| 5 / 118 | 31710 / 31720 | 3 ms | 01:31:10.141752030 |
| 6 / 83 | 31964 / 31974 | 2 ms | 01:31:16.148780294 |
| 8 / 19 | 32102 / 32112 | 0 ms | 01:31:47.197336360 |
| 9 / 107 | 32191 / 32201 | 22 ms | 01:31:54.131275320 |

All record HTTP 200 and 2060-byte upstream length with complete JSON results.
The source plus absent copy-error records supports fast upstream response and
successful writing into the downstream response abstraction. It still does
not prove delivery to, or consumption by, the BN. The prior response-to-timeout
gaps are therefore visibility gaps, not measurements of EL computation or a
uniquely identified Go scheduling stage.

The snooper Dockerfile selects Go 1.25.1. Locally available Go 1.25.12/1.26.5
stdlib files show buffered server writes and final flushing after handler
completion, but are not the exact historical snooper toolchain. This audit
does not use their buffer sizes to claim an observed historical flush point.
The exact snooper's absence of an explicit flush and the ResponseWriter API
boundary already establish the limitation.

## “timeout from http.Client” does not identify the expiring timer

Prysm `beacon-chain/execution/jsonrpc_error.go:35–41,99–107` maps **any** error
implementing `Timeout() bool` and returning true to the sentinel whose text is
`timeout from http.Client`. It erases the concrete error and its original
message. `context.DeadlineExceeded` implements that interface; the phrase is
not proof that the `http.Client.Timeout` field expired.

Exact source sets an authenticated HTTP client's `Timeout` to **30 seconds**
(`network/endpoint.go:116–125`, `network/auth.go:11–12`). The non-Bearer client
has no explicit Timeout (`endpoint.go:37–41`). The execution RPC receives this
client through `gethRPC.WithHTTPClient` (`endpoint.go:128–137`). Independently,
GetPayload sets the Gloas **300 ms** context deadline
(`engine_jsonrpc.go:117–118,303–326`), nested under its caller's context.

Each reached historical proposal fails fewer than two seconds after its
GetPayload exchange. The separate 30-second client timer therefore cannot
explain those failures. The source gives them a shorter 300 ms context budget;
the lossy log prevents distinguishing the precise timeout-producing return
site within the request. In particular, “HTTP-client timeout” must not be
expanded into a claim of a 30-second HTTP wait.

The geth dependency makes the remaining return sites much more specific:

- `rpc/client.go:337–360` invokes HTTP `sendHTTP` synchronously before waiting
  on the operation response channel. It is not an asynchronous multiplexed
  socket-reader path of the kind used for non-HTTP RPC.
- `rpc/http.go:189–203` sends the HTTP request, decodes the JSON-RPC envelope
  from the body, queues that envelope on a buffered channel, and closes/drains
  the body on return. Failure during request transport or body decoding can
  return before the wait.
- Then `requestOp.wait`, `rpc/client.go:146–159`, selects between `ctx.Done()`
  and the already-buffered response. If both are ready, the context branch
  remains selectable. Thus an RPC timeout can occur even after successful
  envelope decoding/queueing. This is a source-supported alternative to “the
  BN never read the HTTP response”; the logs do not select between them.
- The execution result is unmarshaled **after** the response wins that select
  (`client.go:364–374`). That result-unmarshal call receives no context and is
  followed by no context check in `CallContext`. Slow payload-specific JSON
  decoding by itself is therefore not a route to this mapped context timeout.
  Distinguish it from the earlier JSON-RPC envelope/body decoding, which reads
  a context-bound HTTP body.

The original timeout error and per-phase transport events were not recorded.
An offline reproduction of the response/deadline-select race demonstrates
possible semantics but cannot recover which historical return site ran. The
previous packet/runtime control remains stronger experimental evidence for
runnable-reader starvation in that experiment; it does not remove these
historical alternatives.

## A new boundary inside slots 10 and 14: the warning precedes head-state access

Node 35's eth1data warning is at `01:32:10.337596937Z`, BN line 518, before
payload selection at line 519. Node 85's equivalent warning is at
`01:32:50.740644488Z`, line 545, before selection at line 546. The literal
warning identifies the premine branch in
`proposer_eth1data.go:57–62`. This excludes `eth1DataMajorityVote`'s subsequent
EL `BlockByTimestamp`, deposit-count lookup, and vote-majority traversal for
those calls. The generic warning that the EL is not respecting follow distance
is not evidence that this builder is waiting for EL catch-up.

But the same marker exposes a previously insufficiently distinguished
boundary: the warning is logged at line 61 **before** calling
`HeadFetcher.HeadETH1Data()` at line 62. That accessor takes the chain's
`headLock.RLock` (`blockchain/chain_info.go:300–307`) and then the native state's
read lock (`state/state-native/getters_eth1.go:8–26`). Neither accessor receives
the two-second eth1data context. The context therefore does not bound those
lock waits. `packDepositsAndAttestations` starts only after this accessor
returns (`proposer.go:211–218`).

Accordingly, the warning-to-packing-error intervals of **26.535793 s** for node
35 and **41.253273 s** for node 85 include post-warning head/state access before
packing can begin. They cannot all be labeled time inside the packer. This
does not prove a head-lock wait occurred. No owner stack or writer-hold record
exists; source identification of a lock is not historical evidence that the
lock was contended. The principal head writer copies the incoming state while
holding that lock (`head.go:228–245`), but its duration here is unlogged.

The warning also shows `DepositRequestsStarted` was false in the supplied state:
that helper's true branch returns before the warning (`proposer_eth1data.go:39–42`).
It is therefore wrong to dismiss the deposit child solely because the fork is
Electra/Gloas. The deposit child can still traverse the legacy transition path
(`proposer_deposits.go:86–105`), including canonical eth1data selection and
`BlockExists` (`proposer_eth1data.go:100–129`). The logs do not expose which
cache branch it used, so absence of included deposits alone does not prove
zero deposit-path work.

## What the exact packing-error string adds

`packDepositsAndAttestations` creates two children and waits for both
(`proposer_deposits.go:30–66`). A direct error from `deposits` is wrapped with
`Could not get ETH1 deposits`; a direct error from `packAttestations` is wrapped
with `Could not get attestations to pack into block`. After a child body
returns successfully, its wrapper separately checks `egctx.Done()` and can
return the **bare context error** (`:36–59`).

The historical lines say exactly `error=context canceled`, without either
child-error wrapper. Therefore the error selected by the errgroup came from a
child's post-body context check, not directly from one of those wrapped body
errors. At least that child returned successfully before its wrapper observed
cancellation. The other child still has to return before `eg.Wait()` completes;
the first error does not identify which child was last or how long either
ran. Thus the line “Could not pack deposits and attestations” is even less
specific than “packAttestations was executing until the line appeared.”

The packing-error-to-terminal-build-error intervals are only **1.230 ms** for
node 35 and **2.342 ms** for node 85. Those bound the visible remaining
consensus-field work, join completion, and failed post-state attempt after that
marker. They rule out placing the preceding tens of seconds in a subsequent
successful state-root hash. The actual terminal operation immediately sees
the canceled context, as already established by the first audit.

## Which untagged pool errors can actually be assigned

Searching complete owner beacon logs finds no invalid-attestation-deletion
error between entry and completion for node 35's slot 10 or node 85's slot 14.
Their next logged proposal entries occur only at slots 21 and 42 respectively,
after the old attempts have terminated. Their packing-error markers therefore
have a unique preceding active logged proposal, but provide no finer pool phase.

Node 83 has a useful contrasting phase marker: its only preceding build is
slot 6 (BN542); payload terminal failure is BN546; invalid-attestation-deletion
cancellation follows at `01:31:17.006150920Z` (BN547), and the outer packing
error at `01:31:27.292456420Z` (BN550). Its next proposal starts at slot 39
(BN758). Production calls to `validateAndDeleteAttsInPool` are inside
`packAttestations`; its deletion cancellation at `proposer_attestations.go:436–438`
is checked before each deletion. This uniquely associates the marker with
the already-detached slot-6 branch among the recorded handlers and establishes
that at least one invalid candidate remained at a cancellation check.

It does **not** establish a lock wait on that invalid candidate: the cancellation
check precedes acquiring the deletion lock. Nor does the additional **10.286305 s**
to the outer error measure invalid deletion CPU. After logging deletion failure,
`validateAndDeleteAttsInPool` returns the valid candidates and the packer
continues to its next pool/aggregation stages (`:419–427`, `:32–125`). This is
direct historical evidence that observing cancellation inside one helper did
not immediately end the enclosing abandoned branch.

## Completed bounded offline diagnostic

The experimentally distinguishable chain is the **post-eth1-warning
HeadETH1Data boundary before packer entry**. The completed real-build test
controls that dependency and marks packer entry and final consensus completion.
With the same empty pool input, payload failure returns before the controlled
dependency is released; payload success requires its later completion and then
fails state-root processing on cancellation. This is a dependency/cancellation
test, not a recreation of historical load. The separate slot-13 differential
likewise produces the historical error category through two different
controlled paths. [Commands and passing results](offline-build-retention-results.md).

The proxy-copy-failure branch is not a priority reproduction: its predicted
historical error signature is absent. A test of the geth response/deadline
select could verify its ambiguous terminal behavior, but source already proves
the branch exists; it would not decide whether the old BN received its bytes.

## Follow-up: node 85's concurrent block import constrains the head-lock alternative

Node 85 logs `Skipping pending block already being processed` for the full
block-15 root at `01:33:07.343404421Z` (BN548), then its block-15 import at
`01:33:12.605401221Z` (BN550): a **5.261997-second** local-presence-to-import-log
interval inside the still-outstanding slot-14 build.

This is stronger than a late import timestamp alone. Exact
`sync/pending_blocks_queue.go:98–119` retrieves the full block from its local
pending queue, computes its root, and observes `BlockBeingSynced(root)` true
before logging. `blockchain/receive_block.go:87–104` sets that marker before
copying the block and requesting its prestate, and removes it on return. The
block was locally present and an import had begun before the pending marker;
initial network delivery of this block cannot explain the entire remaining
5.262-second interval. This interval can include local processing, waiting,
scheduling, and import reporting; it is not a CPU-duration measurement.

The Gloas branches provide additional exclusions:

- `receive_block.go:237–245` performs only consensus state-transition validation
  for a Gloas beacon block. It bypasses the Engine payload-validation child.
- `receive_block.go:279–282` skips block DA waiting because Gloas DA belongs to
  the separately revealed envelope.
- `process_block.go:112–116` uses `saveHeadIfNeeded` for the Gloas block rather
  than the earlier-fork `sendFCU` path.

The delayed block-15 import therefore cannot be attributed to waiting for its
own execution payload/envelope validation or blob-DA delivery. Its earlier
wire-arrival instant and the division of the local interval remain unlogged.

The same import supplies an **unconditional head-lock read-progress marker**.
`reportPostBlockProcessing`, `receive_block.go:313`, calls `s.HeadSlot()` before
emitting `Synced new block` at line 318. `HeadSlot`, `chain_info.go:173–181`,
acquires and releases `headLock.RLock`. Thus another goroutine successfully
crossed that read lock before `01:33:12.605401221Z`. A writer continuously
holding the lock throughout the complete warning-to-packing-error interval
(`01:32:50.740644488Z` to `01:33:31.993917337Z`) is excluded.

This does not prove the old proposal's HeadETH1Data call had completed by the
new block's import. Its warning does not record the moment it attempts RLock,
and a released reader can remain runnable without being scheduled while
another reader makes progress. Nor does the import by itself prove an actual
head write: noncanonical imports return from `postBlockProcess:108–110` before
saving head, while `saveHeadIfNeeded`, `gloas.go:299–313`, has further early
returns. `Synced execution payload envelope` also does not universally prove
`setHeadFull` ran, because `postPayloadTasks` has earlier non-head/non-extension
returns. Using those generic import logs as proof of a particular completed
writer would overstate the evidence.

The recurring execution follow-distance warnings are still weaker for this
purpose. `execution/service.go:510–516` logs the warning when its own
`LastRequestedBlock == BlockHeight` and returns. That check never acquires the
blockchain head lock. It establishes execution-service progress, not progress
through the proposal's head-state or packing dependency.

## Slot 13: independent FCU gives a head-read progress bound, not cache completion

Node 20 records the slot-13 proposal handler at `01:32:37.317155228`
(`beacon.log:527`) and parent-state preparation failure at
`01:32:48.038076668` (`:531`). Between them, its independent late-block FCU
matches snooper request #721 / JSON-RPC id 216: request
`01:32:42.084531003` (`snooper-engine.log:32149`), successful response with
payload id `0x0487fed9122584fe` at `01:32:42.085875827` (`:32172–32180`), and
the matching BN payload-id log at `01:32:46.342761692` (`beacon.log:530`).
The response-to-BN-log timestamp difference is **4.256885865 seconds**.

Historical `blockchain/process_block.go:1214–1272` explains this separate
workflow. `lateBlockTasks` captures `currentSlot` once (`:1215`); the logged
`nextSlot=13` therefore corresponds to its captured slot 12, even though its
completion log occurs during slot 13. It calls `refreshCaches` (`:1229`) and
later calls `notifyForkchoiceUpdateGloas` (`:1260`). After successful RPC return,
`PayloadIDCache.Set` (`:1265`) and **`HeadSlot()` (`:1268`) both precede the log**.
Consequently the 4.257-second response-to-log difference is not solely an HTTP
consumption interval: it can include payload-id cache work, head-lock waiting,
goroutine scheduling, and logging.

The completed post-RPC `HeadSlot` read proves head-lock progress before the BN
completion log. However, the snooper emits its response log on a detached
logging path: its outer timestamp is not a strict lower bound on the BN's
actual RPC return. Without another timestamped causal predecessor, the source
ordering alone does not place the head read after the proposal's first build
log. Do not use this pair to exclude a head writer held throughout the proposal
interval. It also does not prove the proposal's separate state preparation
completed or identify its delay.

The fork-choice read-lock marker is weaker:
`receive_execution_payload_envelope.go:467–480` obtains and releases the FC
read lock **before** issuing the Engine RPC. The outbound request supplies an
upper bound on that read's completion, but does not prove the read happened
after the proposal's first build log. Thus this FCU does not by itself exclude
a fork-choice writer held continuously over the proposal interval.

Finally, `refreshCaches`, `process_block.go:526–538`, starts a goroutine for
Fulu and later states, including Gloas. The FCU caller does not await that
goroutine. Its success does **not** prove the next-slot cache was filled,
that cache priming to slot 13 succeeded, or that the slot-13 proposal had a
usable cached parent state.

Two final source checks further delimit the intervals. After choosing a bid,
`proposer_gloas.go:81–85` calls `recordBidSource` under `lastBidLock`
(`proposer_bid.go:272–275`) before `wg.Wait`. The unfinished required branch
is established for slots 10/14, but caller wait-entry time is not logged.
For slot 13, `proposer.go:198–199` evaluates `parentFull` even when preparation
has returned an error. That helper (`:184–189`) accesses head state and
possibly fork choice. The terminal-log interval may therefore include
post-error work as well as earlier preparation; neither check establishes a
historical lock delay.
