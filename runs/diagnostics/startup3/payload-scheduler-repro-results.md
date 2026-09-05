# Real `GetPayload` under genesis validator-count load

This local Go reproduction demonstrates two failure boundaries caused by repeated
full scans of a warm, slot-zero validator registry. With 128 workers consuming
1,024 finite scan jobs at `GOMAXPROCS=4`, 4 of 16 real
`execution.Service.GetPayload` calls crossed the production 300 ms Gloas
deadline. The no-load and memoized-count controls each completed all 16 calls
without a timeout.

The test is
[`getpayload_scheduler_diagnostic_test.go`](../../../beacon-chain/execution/getpayload_scheduler_diagnostic_test.go),
and the repeatable driver is
[`run-payload-scheduler-repro.sh`](run-payload-scheduler-repro.sh). This adds no
production behavior.

## Method

The test loads the retained Heze genesis and chain config from
`/tmp/prysm-startup3-wire-h/bundle/network-configs`. It asserts slot zero and
120,000 validators. It clears and synchronously fills the committee cache with
`UpdateCommitteeCache`, proves the entry by retrieving a nonempty epoch-zero
committee from cache, and verifies that `ActiveValidatorCount` returns 120,000.
The synchronous cache fill plus cache-only lookup is the equivalent of waiting
for the asynchronous fill and checking `HasEntry`; timed work cannot take the
cold-cache singleflight branch accidentally.

Each arm uses Prysm's production `Service.GetPayload` at slot 1, which selects
`engine_getPayloadV6` and its 300 ms deadline. The RPC implementation is
go-ethereum v1.17.5. The module is
`github.com/OffchainLabs/prysm/v7`, with working-copy parent
`cdc4b67402ff00479bc39e8cde5f3007df77f8df`, and the run used Go 1.26.5 on
Linux/amd64. It talks over real loopback HTTP to the separate standard library
response server in [`payloadserver`](payloadserver/main.go). The server writes
and flushes a valid padded 2,060-byte V6 payload response and records an
absolute timestamp after `Write` and `Flush` return. One successful request
warms the connection before tracing and timed work.

The three arms have the same configuration: 128 workers, 1,024 finite job
tokens, 16 timed payload calls, and `GOMAXPROCS=4`.

- `none` releases no count workers.
- `memoized` consumes every job and compares the already-computed scalar count.
- `scan` consumes every job by calling the real `ActiveValidatorCount` on the
  warm slot-zero state. This deliberately reaches the full validator loop.

Both work arms completed exactly 1,024 jobs, and every computed count equaled
120,000. The test accepts only `ErrHTTPTimeout` in the scan arm and asserts zero
errors in both controls. All arms passed.

## Results

| Arm | Jobs completed | Timed interval | Payload timeouts | Longest mapped call |
| --- | ---: | ---: | ---: | ---: |
| none | 0 | 3 ms | 0 / 16 | 472 microseconds |
| memoized | 1,024 | 3 ms | 0 / 16 | 469 microseconds |
| scan | 1,024 | 2,114 ms | 4 / 16 | 686.650 ms |

The runtime metric `/cpu/classes/user:cpu-seconds` returned zero before and
after every arm. It is unusable in this run and is not evidence of zero CPU.
The whole-test CPU profile below provides the usable attribution. Its scope
includes genesis decoding and cache warmup, while the test's elapsed interval
starts immediately before the worker barrier and ends after all finite work.

The four failed scan calls have two distinct boundaries. Durations below are
relative to each `PayloadCallStart`; the absolute start is included to correlate
the client, server, and runtime trace without relying on log order.

| Call | Start, Unix ns | `WroteRequest` | Server `Write` + `Flush` returned | First-byte callback | Raw return | Mapped return | Boundary |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 2 | 1788688022228197851 | +0.047 ms | +0.133 ms | +343.133 ms | +686.627 ms | +686.656 ms | response ready; reader and teardown delayed |
| 3 | 1788688022914856175 | absent | absent | absent | +303.076 ms | +303.087 ms | expired during connection acquisition, before write |
| 4 | 1788688023217946366 | absent | absent | absent | +545.495 ms | +545.503 ms | connection became available, but expired before write |
| 5 | 1788688023763451446 | absent | absent | absent | +302.966 ms | +302.974 ms | expired during connection acquisition, before write |

Call 2 reproduces the response-servicing pattern directly. The server handler
reached completed `Write` and `Flush` in 23 microseconds, 0.133 ms after call
start. Runtime trace
goroutine 140, identified at the callback as
`net/http.(*persistConn).readLoop`, changed from Waiting to Runnable at a time
that aligns with the server flush within a few microseconds. It then remained
Runnable for 342.909 ms before its first Running transition. Once scheduled,
it reached `GotFirstResponseByte` after 97.856 microseconds of elapsed time.

The raw return was later still. At roughly the same time as the first-byte
activity, the caller entered `persistConn.mapRoundTripError` and waited on
`writeLoopDone`.
Write-loop goroutine 141 became Runnable but did not run for 343.462 ms. When it
finally ran and exited, it woke the caller; the raw call then returned the
deadline error. This shows concretely how a 300 ms deadline can be reported at
686 ms under Go scheduling pressure.

Calls 3 through 5 must not be described as response-reader failures. They have
`GetConn` and `ConnectStart` events but no `WroteRequest`, no server record, and
no first-byte callback. They demonstrate that the same pressure can exhaust the
budget before a request reaches the server. Call 4 also demonstrates delayed
error reporting: a connection was handed to it at +242.457 ms, yet no request
was written and the mapped timeout returned at +545.503 ms.

For all four failures, the real geth HTTP call returned `*url.Error` with
`Timeout() == true`, `errors.Is(raw, context.DeadlineExceeded) == true`, and its
child context expired. Prysm then returned the standalone
`*errors.fundamental` `ErrHTTPTimeout`. The mapped error satisfied
`errors.Is(mapped, ErrHTTPTimeout)` but did not satisfy
`errors.Is(mapped, context.DeadlineExceeded)`; the parent test context remained
live. This reproduces the deadline-identity loss in the actual execution layer.

## CPU attribution

The scan arm's whole-test profile spans 2.20 seconds and contains 8.53 sampled
CPU-seconds, or 387.51% of wall time. `ActiveValidatorCount` accounts for 8.44
CPU-seconds cumulatively (98.94%). Its scan-worker path alone accounts for 8.43
seconds. `sync/atomic.(*Int32).Add`, reached through
`multi-value-slice.(*Slice).At` reader locking, accounts for 5.34 flat seconds
(62.60%). Cache warmup accounts for only 0.05 cumulative seconds in this
profile.

This profile and call 2's scheduler trace connect the workload to the response
delay: the four available Go execution slots spend nearly all sampled CPU in
the repeated count, while an already-runnable HTTP read loop waits longer than
the payload deadline.

## Artifacts and limits

The original raw run directory is `/tmp/prysm-payload-scheduler-pilot`. A
compact evidence set was copied byte-for-byte into
[`payload-go-evidence`](../round2-slots-0-16/payload-go-evidence/); SHA-256
checksums matched the `/tmp` sources after copying:

- [`none.test.log`](../round2-slots-0-16/payload-go-evidence/none.test.log),
  [`memoized.test.log`](../round2-slots-0-16/payload-go-evidence/memoized.test.log),
  and [`scan.test.log`](../round2-slots-0-16/payload-go-evidence/scan.test.log)
  contain summaries, raw and mapped errors, transport callbacks, progress
  counts, active scan counts, and absolute timestamps.
- [`server.jsonl`](../round2-slots-0-16/payload-go-evidence/server.jsonl)
  contains the server write/flush boundaries. The original `server.stderr` in
  `/tmp` is empty.
- [`scan.trace`](../round2-slots-0-16/payload-go-evidence/scan.trace) is the raw
  Go runtime trace, and
  [`scan.call2.scheduler.transitions.txt`](../round2-slots-0-16/payload-go-evidence/scan.call2.scheduler.transitions.txt)
  is its 43-line call-2 scheduler extract.
- [`scan.cpu.pb.gz`](../round2-slots-0-16/payload-go-evidence/scan.cpu.pb.gz) is
  the whole-test CPU profile. Its retained summaries are
  [`scan.pprof.top.txt`](../round2-slots-0-16/payload-go-evidence/scan.pprof.top.txt)
  and
  [`scan.pprof.focus-active-count.txt`](../round2-slots-0-16/payload-go-evidence/scan.pprof.focus-active-count.txt).
- [`timestamps.tsv`](../round2-slots-0-16/payload-go-evidence/timestamps.tsv)
  records each arm's absolute start and end.

The 88 MB compiled test binary and 4 MB parsed trace remain only in `/tmp` and
are intentionally omitted from the repository evidence set. The no-load and
memoized raw traces and CPU profiles also remain in the original `/tmp` run
directory; their compact test logs are sufficient for the retained controls.

The test is a local loopback reproduction, not a rerun of the historical
1,000-node network. A server-side `Flush` return is an exact boundary inside
this fixture but is not a packet capture. The runtime transition that aligns
with that flush closes the relevant boundary for call 2. The finite memoized
control matches offered jobs and count results; it does not match the scan
arm's active-load duration because trivial scalar jobs drain immediately. The
CPU profile is whole-test rather than restricted to the timed interval.

Only one pilot was needed. The planned 6,144-worker, 15,000-job escalation was
conditional on seeing no 300 ms failure at pilot scale. Because the pilot
already produced four mapped timeouts with passing controls, the larger run was
not performed.

## Source checks

After packaging the evidence, read-only `gofmt -d` and `goimports -d` checks
reported no differences for the diagnostic test and response server. `bash -n`
passed for the runner. The three real-HTTP arms above are the final execution
check; no Bazel run or additional test rerun was needed for packaging.
