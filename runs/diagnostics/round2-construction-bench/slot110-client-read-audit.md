# Slot 110 SSE client-read audit

## Finding

At `2026-09-05T01:52:01.725042996Z`, node 148's exact deployed Prysm build
logged `Client is unable to keep up with event stream, shutting down`
(`round2-slot110-owner-node148/beacon.log:937`). This message has a narrow
meaning in the source: one HTTP event stream's 1,000-entry outbox was full when
Prysm tried a nonblocking enqueue, so Prysm canceled that stream
(`beacon-chain/rpc/eth/events/events.go:39,219-230,280-313` in the exact
0280403 checkout). The warning does not include the requested topic, remote
address, subscription ID, outbox occupancy history or write duration.

The deployed client architecture supplies a plausible mechanism for the reader
to fall behind. Xatu's exact beacon dependency opens one HTTP subscription per
topic (`pkg/beacon/subscriptions.go:54-75`). Xatu's selected exact
`go-eth2-client` calls the topic handler synchronously
(`http/events.go:83-102`). Its selected `r3labs/sse/v2` v2.10.0 reader sends
each parsed event through an **unbuffered** channel. While the current handler
runs, the separate read goroutine can parse at most one following event, then
blocks sending it until the handler returns (`client.go:100-109,204-245`). The
beacon dependency's emitter launches listeners as goroutines but waits for all
of them before returning (`emission/emitter.go:157-214`). Xatu's
single-attestation handler performs duplicate checking, decoration, summary
accounting and sink enqueue before returning
(`pkg/sentry/sentry.go:260-288,1084-1125`). Thus callback throughput can
backpressure the HTTP-body reader and allow Prysm's per-stream outbox to fill.

This source path does not prove which callback or operation consumed the time.
Prysm's server writer also invokes the lazy event formatter, reads its complete
JSON output, and only then performs the HTTP write
(`beacon-chain/rpc/eth/events/events.go:393-448`). An outbox overflow therefore
cannot distinguish client/network lag from server-side formatting, writer
scheduling or HTTP-write delay. The deployed asynchronous sink's full-queue branch drops immediately; it does
not wait for the upstream export RPC. Exact `ttlcache` v3.4.0 serializes
`GetOrSet` on its item mutex (`cache.go:392-430`), shared by the single and
aggregate attestation duplicate checks, but there is no retained timing for
expiry or lock contention at slot 110.

More precisely, `GetOrSetFunc` holds that mutex through lookup, LRU/expiration
heap updates and insertion (`cache.go:416-432`), while `DeleteExpired` holds the
same mutex across its expired-item loop (`cache.go:476-495`; the cleaner is
started at lines 695 onward). This is a concrete callback wait point shared by
single and aggregate processing. It remains unmeasured historically. Node 148's
first slow-reader warning was at `01:36:25.738808903Z`, only 385.739 seconds
after genesis and before the earliest normal seven-minute cache expiry, so
expiration cannot explain the initial onset. Also, graceful EOF returns nil in
the selected SSE reader (`r3labs-sse/client.go:216-221`), allowing the outer
one-second reconnect loop to run without an error-level record.

## Exact deployed sources

* Xatu: `/tmp/xatu-3857752c.HeaAu9`, commit
  `3857752cf8e67eab7f34a05e76f9561f3f558013`.
* Beacon dependency: `/tmp/beacon-da5e1aac.PzD0Lr`, commit
  `da5e1aac0890c38297f3c12f92dffc95bdb18831`. Xatu's `go.mod:32` pins the
  corresponding pseudo-version.
* Selected `go-eth2-client`: `/tmp/go-eth2-client-cba019f9.kRPQiL`, commit
  `cba019f9e0a5715e3996c106238c48090b9f4c17`. Xatu's direct requirement wins
  module selection over beacon's older `bca0dce49005` requirement.
* Selected SSE library: Go module cache
  `github.com/r3labs/sse/v2@v2.10.0`.
* Exact `ttlcache` v3.4.0: `/tmp/ttlcache-v3.4.0.LZrIMD`, commit
  `7145e12e34f243c69a0f7b5f6b86a832ad8b4fc8`.

The inspected files, their commit manifest and individual checksums are
preserved in [`slot110-sse-source-evidence.tar.gz`](slot110-sse-source-evidence.tar.gz),
with archive checksum in
[`slot110-sse-source-evidence.sha256`](slot110-sse-source-evidence.sha256).

## Retained node-148 evidence

Node 148 ran `Xatu/0.0.2-glamsterdam-devnet-9-3857752c`, async shipping,
five workers, a 100,000-entry input queue, 512-event batches and a 30-second
export timeout (`xatu-sentry.log:3-9`). At startup, it logged exactly one
subscription for each of 18 topics (`xatu-sentry.log:29-47`), including a
separate `single_attestation` stream at line 31.

The complete retained Xatu log has no later `Failed to subscribe to event
stream`, reconnect, disconnect or per-topic reader-error record. This absence
does not prove that no transient reconnect occurred: the exact HTTP client uses
internal exponential reconnect, while Xatu's connect/disconnect messages are
at trace level (`go-eth2-client/http/events.go:83-99`). The retained INFO log
therefore cannot identify which of the 18 streams Prysm canceled.

Around slot 110:

| Time | Xatu evidence |
|---|---|
| `01:51:50.930`, `01:51:55.930`, `01:51:56.326` | Three 512-event upstream RPC batches timed out (`xatu-sentry.log:1307-1309`). |
| `01:52:00.307` | Async input queue full; event type 19; 14,990 failures suppressed (`xatu-sentry.log:1310`). Event 19 is `BEACON_API_ETH_V1_EVENTS_ATTESTATION_V2`, used by both single and aggregate attestation decorators. |
| `01:52:01.725` | Prysm emitted the slow-reader warning (`beacon.log:937`). |
| `01:52:04.379` | Another 512-event upstream RPC batch timed out (`xatu-sentry.log:1311`). |
| `01:52:07.906` | One-minute summary: 68,016 `single_attestation`, 2,140 payload-attestation, 1,053 aggregate-attestation, 43 contribution, and 46 other stream events; `events_exported=70643` and `events_failed=0` (`xatu-sentry.log:1312`). The exported counter increments before sink handling and is not a successful-delivery count (`pkg/sentry/sentry.go:1101-1106`). |
| `01:52:10.462` | Async input queue full again; event type 19; 10,807 failures suppressed (`xatu-sentry.log:1313`). |
| `01:52:12.005` | Slot-110 reorg triggered a committee refetch (`xatu-sentry.log:1314`). |

The minute summary makes `single_attestation` the dominant logged stream by a
large margin, but neither the Prysm warning nor Xatu's records join the canceled
connection to a topic. It is therefore reasonable to identify high event rate
plus synchronous per-event client handling as the mechanism that can fill an
outbox, while treating “the single-attestation stream was the disconnected
one” as unproven. Queue-full and export-timeout records establish pressure in
Xatu's separate downstream pipeline; because async enqueue drops rather than
waiting for RPC completion, they are not evidence that Prysm waited on an
upstream Xatu RPC.
