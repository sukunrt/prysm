# Source metric inventory for slot-110 construction and Xatu

This inventory describes metrics exported by exact Prysm commit
`0280403c70d88967f49d2d4c730f4c5417dabdf5` and exact Xatu commit
`3857752cf8e67eab7f34a05e76f9561f3f558013`. Source presence does not prove a
series was scraped historically; the Prometheus inventory must establish that.

## Prysm proposal construction

There is no Prometheus timer inside `BuildBlockParallel`, `buildBlockGloas`,
`setPreGloasConsensusFields`, `packAttestations`,
`computePostBlockStateAndRoot`, `CalculatePostState`, or `HashTreeRoot`.
`ProposerServer.packAttestations` and `ProposerServer.GetBeaconBlock` create
tracing spans, but those are not Prometheus series. Consequently, Prysm's own
Prometheus metrics cannot directly divide the historical 3.393-second build
among pool validation, deduplication/aggregation, reward sorting, state
transition, and state-root hashing.

Two request-level histograms may bound the whole call:

* `http_request_latency_seconds_{bucket,sum,count}` has labels `endpoint`,
  `code`, and `method`. REST Gloas construction uses endpoint
  `validator.ProduceBlockV4`, method `GET`. The middleware surrounds the whole
  HTTP handler (`beacon-chain/rpc/endpoints.go:51-78,404-421`).
* `grpc_server_handling_seconds_{bucket,sum,count}` has labels `grpc_type`,
  `grpc_service`, `grpc_method`, and `grpc_code`. Prysm enables the gRPC
  handling histogram at server start (`beacon-chain/rpc/service.go:360-363`);
  `grpc_method="GetBeaconBlock"` covers the complete gRPC request if that API
  was used.

The build path itself is visible only through the `Building block`, payload-bid
choice, and `Finished building block` logs and source tracing spans
(`proposer.go:54-125`, `proposer_gloas.go:18-120`). Histogram deltas must be
checked for exactly one request because their `_sum` and `_count` values are
cumulative.

## Prysm inputs and competing work

* `aggregated_attestations_in_pool_total`,
  `unaggregated_attestations_in_pool_total`, and
  `seen_aggregated_attestations_in_pool_total` are unlabeled gauges of physical
  pool/cache counts. They are refreshed by the per-slot expiration routine at
  slot start plus 11 seconds, not by the proposer when it takes its pool
  snapshot (`operations/attestations/metrics.go:8-68`,
  `prune_expired.go:11-21`). They do not report eligible getter counts or the
  snapshot used by a build at slot start.
* `gossip_attestation_verification_milliseconds` is an unlabeled Summary. Each
  remote ordinary attestation observes integer milliseconds at deferred return
  from the complete validation function, including the synchronous operation
  feed send (`sync/metrics.go:147-153`,
  `validate_beacon_attestation.go:43-51,229-245`). `_count` and `_sum` deltas can
  quantify aggregate validation work over a scrape interval, but do not expose
  concurrency or individual phases.
* `p2p_message_received_total{topic}` increments immediately before the wrapped
  validator. `p2p_message_failed_validation_total{topic}` and
  `p2p_message_ignored_validation_total{topic}` increment after their
  respective outcomes (`sync/subscriber.go:548-614`). Topics include fork and
  subnet, so a total ordinary-vote rate requires aggregation across them.
* `aggregate_attestations_t1` and `aggregate_attestations_t2` are unlabeled
  millisecond histograms around scheduled fork-choice pool compaction. They do
  not time proposal packing (`operations/attestations/metrics.go:49-63`,
  `prepare_forkchoice.go:35-49`).
* `replay_blocks_milliseconds` and `replay_to_slot_milliseconds` are unlabeled
  StateGen summaries. `next_slot_cache_hit` and `next_slot_cache_miss` are
  counters. None times a normal warm state copy, transition, or tree hash
  (`state/stategen/metrics.go:8-33`,
  `core/transition/trailing_slot_state_cache.go:24-71`).
* `beacon_goroutine_count{kind="instant|average|limit"}` includes an instant
  sample once per slot, a ten-slot average, and the configured limit
  (`blockchain/metrics.go:289-292`, `blockchain/goroutine_count.go:10-32`). It
  does not identify the work done by those goroutines.

Prysm serves the default Prometheus registry, which includes the standard Go
runtime and process collectors. Useful inventory candidates include
`process_cpu_seconds_total`, `process_resident_memory_bytes`, `go_goroutines`,
the `go_memstats_*` families, and GC metrics. These are scrape-level process
signals, not proposal-stage timers.

## Prysm event stream

`http_sse_error_count{endpoint,error}` is the only SSE-specific metric
(`rpc/eth/events/events.go:107-113`). Writer-loop errors increment it directly
at lines 324-330. A slow-reader error from `recvEventLoop` is assigned to a
shadowed `err` in the `if` initializer at lines 230-233, so it does **not**
reach the outer deferred increment at lines 194-199. Therefore the historical
`Client is unable to keep up` warning is not reliably countable from this
metric. There is no exported Prysm SSE outbox-depth, write-duration,
subscriber-count, or dropped-event metric in this revision.

## Xatu sentry and output processor

* `xatu_sentry_decorated_event_total{type,network_id}` increments after event
  decoration and before writing it to any sink
  (`pkg/sentry/metrics.go:5-24`, `pkg/sentry/sentry.go:1084-1106`). It is an
  input/decorated-event count, not successful export delivery.
* `xatu_processor_items_queued{processor}` is a gauge set after each successful
  enqueue and after a worker export. It is not a continuous queue sample.
* `xatu_processor_items_dropped_total{processor}` increments when async enqueue
  finds the input queue full, and for nil-item drops.
* `xatu_processor_items_failed_total{processor}` increments by batch size after
  an exporter failure.
* `xatu_processor_items_exported_total{processor}` increments by batch size
  after successful export.
* `xatu_processor_export_duration_seconds_{bucket,sum,count}{processor}` times
  only the worker's exporter call, including a timeout; it does not time beacon
  SSE reading, decoration, or enqueue.
* `xatu_processor_batch_size_{bucket,sum,count}{processor}` records successful
  export batch sizes.
* `xatu_processor_worker_count{processor}` and
  `xatu_processor_worker_export_in_progress{processor}` are worker gauges.

These names and the sole `processor` label are defined at
`pkg/processor/metrics.go:13-121`. Their observation points are
`pkg/processor/batch.go:314-329,395-453,550-619,659-679`. For the Xatu sink,
the processor label is constructed as `xatu_output_xatu_<sink-name>`
(`pkg/output/xatu/xatu.go:42-50`); the historical label value must be discovered
from the scrape.

Xatu also serves the default Prometheus registry and therefore the standard Go
and process collectors. This revision exports no dedicated metric for beacon
SSE callback duration, HTTP-body read latency, reconnects, per-topic stream
status, duplicate-cache latency, or reader backlog. Thus the processor metrics
can identify downstream queue/export pressure, while they cannot directly time
the client callback that feeds that queue.

