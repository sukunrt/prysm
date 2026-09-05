# Xatu and the deployed event stream

Node 1 had an actual event-stream consumer during round 2. Its Xatu log records
`Xatu/0.0.2-glamsterdam-devnet-9-3857752c`, asynchronous shipping, five workers,
a 100,000-item queue, 512-event batches and a 30-second export timeout
(`runs/round2/prysm-geth-1/xatu-sentry.log:54–60`). Its subscribed topics include
attestations, single attestations, heads and execution-payload events, but not
`payload_attributes` (`xatu-sentry.log:29–47`; these are the first startup's
equivalent subscription records).

The export path was failing near proposal 97: queue-full records at
`01:49:12.348024899Z` and `01:49:24.193688203Z` report suppressed counts of
7,462 and 13,204; 512-event upstream RPCs time out at
`01:49:13.627950516Z`, `01:49:18.622820472Z` and
`01:49:23.624021414Z` (`xatu-sentry.log:1375–1379`). These are Xatu sink/export
failures. They do not measure Prysm's HTTP write latency or establish that
Xatu stopped consuming the beacon event stream. The next minute summary
still reports 62,931 single-attestation stream events
(`xatu-sentry.log:1387`).

## Exact Prysm 028 source

The event stream has a real path back to gossip validation. Successful
ordinary FFG validation calls the synchronous operation feed before setting
the seen key, final validator data and FFG ledger record
(`beacon-chain/sync/validate_beacon_attestation.go:229–243`). Prysm re-exports
go-ethereum's event feed (`async/event/feed.go:20–26`); the deployed dependency
is v1.17.5. Its `event/feed.go:120–179` serializes sends and waits for every
subscriber channel to receive or unsubscribe.

In `beacon-chain/rpc/eth/events/events.go:242–258`, the server creates a
1,000-capacity outbox by default, then initializes the subscribed event channel
with **`len(es.outbox)`**, which is zero at startup, rather than its capacity.
Thus the event-feed subscription is unbuffered despite the later explanatory
comment about two buffers. A validation's feed send must synchronize with
the event receiver. Every operation event reaches every operation-feed
subscription before `lazyReaderForEvent` checks whether its topic was requested
(`events.go:255–269,518–526`).

For a single attestation, the receiver prepares a closure without fetching a
head state or committee (`events.go:604–612`). Attestation conversion, JSON
formatting and HTTP writes run in the separate outbox writer
(`events.go:393–448`). The receiver's `safeWrite` is nonblocking: a full outbox
returns `errSlowReader`, cancels the stream and unsubscribes
(`events.go:282–313`). Consequently, a slow HTTP client can add receiver/write
work or transient feed waiting, but the implementation does not intentionally
hold the operation feed until Xatu's upstream export RPC returns. The default
HTTP write timeout is one slot (`events.go:211–221`), independent of the
30-second Xatu export timeout.

Node 1 has no retained slow-reader warning around proposal 97. Such warnings
do appear later, at `01:53:25.900232417Z` and `01:55:25.755462424Z`
(`beacon.log:1420689,1554402`). Their presence later confirms this error path
is observable; it does not prove the earlier SSE receive loop was cost-free.

The measured `BuildBlockParallel` path does not itself send this operation
feed. Nearby proposer sends belong to later block/blob submission, separate
bid submission or proposer-preference APIs. An SSE effect on the joined
construction interval would therefore need to operate through shared runtime
resources or another shared operation/lock, rather than a demonstrated direct
wait on the Xatu export queue. The paced validation fixture's empty operation
feed omits this actual SSE delivery and JSON work. Its log-stream fixture is
a different feed; including Prysm's log stream does not include beacon SSE.

## Exact deployed Xatu source

The deployed Xatu source was subsequently retrieved and independently checked:
commit `3857752cf8e67eab7f34a05e76f9561f3f558013`, matching the logged build
identifier, at `/tmp/xatu-3857752c.HeaAu9`. Its `go.mod:32` pins
`github.com/ethpandaops/beacon v0.69.1-0.20260712074721-da5e1aac0890`.
That exact dependency was also checked at commit
`da5e1aac0890c38297f3c12f92dffc95bdb18831`, in
`/tmp/beacon-da5e1aac.PzD0Lr`.

The exact dependency's `pkg/beacon/subscriptions.go:54–75` opens one HTTP
subscription per topic, passing a one-element `Topics` slice each time. This
confirms **18 separate SSE subscriptions** for the logged topics, of which
11 subscribe to Prysm's operation feed and seven to its state feed. The
operation topics are `attestation`, `single_attestation`, `block_gossip`,
`voluntary_exit`, `contribution_and_proof`, `blob_sidecar`,
`data_column_sidecar`, `execution_payload_gossip`, `execution_payload_bid`,
`payload_attestation_message` and `proposer_preferences`. Therefore every
FFG operation must reach all 11 unbuffered operation-feed receivers before
each receiver filters by its own requested topic. This is an exact-source
consequence of the deployed subscription configuration, not an inference
from a different cached release. The logs do not track transient reconnects
or the active connection count at every instant.

The exact Xatu event path calls `sink.HandleNewDecoratedEvent` from
`pkg/sentry/sentry.go:1084–1108`, then `proc.Write` from
`pkg/output/xatu/xatu.go:84–95`. Its `pkg/processor/batch.go:334–392` only waits
for export completion in synchronous shipping mode. In the logged async mode,
`enqueueOrDrop` uses a nonblocking channel send and returns `ErrQueueFull`
when the queue is full (`batch.go:659–679`). Thus the observed queue-full
condition drops an export attempt; it does not wait for the failing export
RPC to complete. It may still incur decoration, logging and queue-accounting
work before returning. The export error is not proof of a blocked Prysm SSE
connection.

The processor/exporter lock audit does not reveal an indirect RPC wait in
that async call either. `BatchItemProcessor` has shutdown synchronization but
no shared mutex taken by `Write` and held across export
(`processor/batch.go:210–222,339–392`). Export runs separately in
`worker` → `exportWithTimeout` → exporter `sendUpstream`
(`batch.go:396–453,605–621`; `output/xatu/exporter.go:103–120,127`), without
holding a lock that `Write` must acquire. Slow workers can stall the batch
builder's `batchCh` send and fill the input queue (`batch.go:549–603`), but the
full input queue still takes `enqueueOrDrop`'s immediate error branch. There
is no "unless the queue is full" exception to the asynchronous RPC separation.

The enabled beacon-committee service also does not imply a beacon-node RPC
for each single vote. Exact Xatu single-attestation decoration derives local
slot/epoch metadata and reads the attester index from the message
(`pkg/sentry/event/beacon/eth/v1/events_single_attestation.go:132–190`). Its
`GetAttestationDuties` and `GetValidatorIndex` methods only read the committee
cache and return an error on a miss (`ethereum/services/duties.go:438–475`).
Committee RPCs are separately scheduled around epoch boundaries and reorg
callbacks (`duties.go:171–203,373–407`); node 1's next logged reorg-triggered
refetch is `01:49:36.004017004Z`, after build 97 (`xatu-sentry.log:1381`). No
per-single RPC amplification has been established for the measured interval.

## Executed SSE comparison

The companion control runs the exact deployed Prysm commit
`0280403c70d88967f49d2d4c730f4c5417dabdf5` with only diagnostic tests added.
It adds all 18 configured one-topic HTTP subscriptions to the same 14,355-vote
validation schedule, using the real `events.Server.StreamEvents` handler,
separate operation/state feeds and immediate HTTP client drains. Production
defaults supply the outbox capacity, keepalive interval and write timeout.
Before measurement, an unrequested sentinel event must reach exactly 11
operation-feed subscribers and seven state-feed subscribers. This confirms
subscription readiness; merely receiving an initial keepalive would not,
because the stream writer and receiver start independently.

The fixture is
`beacon-chain/sync/historical_paced_sse_fixture_test.go` in the exact-old
workspace. The paced driver captures a completed frame only after its event
header, data and terminating blank line have arrived. Live-context EOF or
reader errors fail the control. It captures build-finish counters before
correctness hashing, then drains all validations and requires all 14,355
complete single-attestation frames, no stream errors and zero slow-reader
warnings. The same bid-relative validation-entry schedule runs with and
without SSE; SSE setup finishes before the validation clock is initialized.

| Go scheduler parallelism | Paced validation, no SSE | Same pacing, 18 SSE streams | Observed difference |
| --- | ---: | ---: | ---: |
| `GOMAXPROCS=4` | 89.636 ms | 86.309 ms | -3.326 ms |
| `GOMAXPROCS=1` | 152.248 ms | 160.982 ms | +8.733 ms |

Each row is one separately run pair, so these differences are observations,
not estimates of a stable SSE overhead or speedup. The raw records are
[P4 without SSE](old028-sse-comparison-p4-nosse.log),
[P4 with SSE](old028-sse-comparison-p4-sse18.log),
[P1 without SSE](old028-sse-comparison-p1-nosse.log) and
[P1 with SSE](old028-sse-comparison-p1-sse18.log).
All four controls accepted and subscribed all 14,355 votes after draining.
The SSE arms received 102 complete frames by build finish at P4 and 36 at P1,
then all 14,355 after draining, with zero slow-reader warnings or disconnects.
At P1, 575 validations had entered by build finish; these entry and completion
counts describe different stages and must not be interchanged.

These controls measure Prysm event delivery, formatting and local HTTP
draining. They do not run the complete Xatu collector, its enrichment/export
pipeline, outgoing P2P traffic, unlogged ignored/pending messages or any
historical file/storage wait. The immediate HTTP clients share the test
process; `GOMAXPROCS=1` limits its Go scheduler, including the clients, and is
not a reconstruction of a one-core container quota or a limit on all native
BLS execution. The configured 18 historical subscriptions are source-proven;
their exact active connection count at every historical instant is not.

The observed subscription fan-out is therefore a real omitted workload that
has now been tested, but it did not reproduce the historical 3.645-second
payload-choice-to-construction-finish interval. The compact pool, full build,
paced validation and SSE controls remain far below that interval. The
separate raw-single control reproduces seconds only when thousands of
eligible, uncovered singles are actually present. Retained aggregate/FFG
records do not establish that historical pool snapshot, and the audited
pending-replay path can insert candidates without those ledger records. See
[the coverage bounds](historical97-coverage-bounds.md) and
[the complete measurements](results.md).

Accordingly, the retained evidence localizes the later delay but does not
identify its full cause. It does not support attributing the entire joined
interval to the compact retained aggregates, the measured current-slot vote
schedule, or Xatu's upstream export timeouts. No blocked reader, artificial
delay or speculative extra load was introduced to produce these results.
