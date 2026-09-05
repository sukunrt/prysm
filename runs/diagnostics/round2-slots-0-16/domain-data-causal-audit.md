# DomainData deadline: narrower control flow and completed causal control

Later retained-trace analysis identifies the actual HTTP/2 reader and handler
for an ordinary RANDAO call: [the H trace](domain-http2-reader-trace-results.md)
measures 135.385 ms runnable delay while all 53 CPU samples execute genesis
counts. Its count-removal control cuts the RPC from 139.232 to 1.636 ms.
The [E1 supplement](domain-e1-prebody-envelope-results.md) separately establishes
a 2.162415-second pre-body interval. The original source analysis and null
TCP result below remain useful limits; they no longer mean that no ordinary
DomainData transport mediator has been located.

This source and retained-evidence audit preceded the bounded TCP control now
reported below. Historical source is revision
`0280403c70d88967f49d2d4c730f4c5417dabdf5`. It supplements
[deeper-preflight.md](deeper-preflight.md) and the
[slot-1 audit](../startup3/round2-slot1-cause-deep-audit.md).

## The terminal domain error proves an invocation, not a wire request

In historical `validator/client/validator.go:721–758`, `domainData` first checks
the cache under `domainDataLock.RLock`, then acquires the global write lock and
checks again. Neither lock wait checks the context. Both cache-hit returns
succeed even with a canceled context. The method's only error return is the
error from `v.validatorClient.DomainData(ctx, req)` after both cache misses and
write-lock acquisition.

Therefore node169's slot-1 `could not get domain data: ... DeadlineExceeded`
proves that the proposer eventually acquired the write lock, still missed, and
invoked its own client adapter/RPC envelope. It did not return that error while
still waiting for the cache lock. Earlier lock waiting or late goroutine
scheduling can nevertheless have exhausted the budget before this invocation.
This does not prove that request bytes left the VC.

The adapter (`grpc-api/grpc_validator_client.go:202–204`) forwards the same context
without a new timeout. The runner creates role contexts with the absolute slot
deadline, after `RolesAt` returns (`runner.go:104–106,147–156`); RANDAO does not
receive a fresh RPC budget. grpc-go v1.81.1 explicitly rejects a nonpositive
remaining deadline while constructing headers (`internal/transport/http2_client.go:
600–607`), before writing a request. The configured retry interceptor also means
the adapter invocation is an envelope, not necessarily one server attempt; the
existing retry qualifications in `deeper-preflight.md` still apply.

## Slot 4 is downstream of an already expired preflight

Raw archive: `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz`, member
`./validator.log`:

| Line | UTC timestamp | Evidence |
| --- | --- | --- |
| 5 | 00:51:19.573249881 | Opened wallet, `keymanagerKind=direct` |
| 832 | 01:31:00.003838012 | Sync aggregator preflight: `can't sign selection data: rpc error: code = DeadlineExceeded desc = context deadline exceeded` |
| 839 | 01:31:00.003910654 | RANDAO: `could not get domain data: rpc error: code = DeadlineExceeded desc = context deadline exceeded` |

`localSelector.signSyncSelectionProofs` obtains sync indices successfully before
calling `signSyncSelectionData` (`aggregator_selector.go:131–149`). The different
`can't fetch sync subcommittee index` prefix would identify failure of that
preceding RPC. Here the failing operation is in the signing-data path.

The direct wallet selects the local keymanager (`keymanager/types.go:134–139`,
`accounts/wallet/wallet.go:323–334`). Its `Sign` ignores the context and returns
only nil-public-key or missing-key errors before local BLS signing
(`keymanager/local/keymanager.go:181–192`). `ComputeSigningRoot` also does not
produce a gRPC deadline status. Thus the observed sync-selection error originated
from `domainData`, whose invocation and cache-lock facts above apply to this call
as well. It does not show when the preceding successful sync-index call finished.

The preflight error is logged synchronously inside `RolesAt`, which immediately
returns `rolesAt, nil` (`validator.go:646–649`). Only then does the runner create
the role context and dispatch `ProposeBlock`. Slot 4's absolute deadline was
`01:31:00`; the preflight error was already 3.838 ms past it. The resulting RANDAO
role therefore started with an expired budget. Its terminal error appeared only
72.642 microseconds after the preflight error. This is not evidence of a second,
seconds-long RANDAO RPC: the cost to explain is upstream, before the preflight
returned. Its exact division between earlier selection work, successful sync
indices, domain-lock waiting, and the sync-selection domain RPC remains unlogged.

Slot 1 differs: node169 had dispatched roles by the successful PTC response at
`01:30:21.915067414`, before its `01:30:24` deadline. Its proposer start and domain
invocation remain unlogged, so the same expired-at-dispatch conclusion cannot be
transferred to slot 1.

## What transport source excludes and leaves open

The ordinary BN DomainData body uses static fork configuration and the global
genesis validators root (`rpc/prysm/v1alpha1/validator/server.go:176–203`). RANDAO
and sync-selection domains do not enter its voluntary-exit head-state branch.
Neither ordinary path takes fork-choice/checkpoint/registry locks or scans the
validator registry. B/E1 body markers measured maxima of 0.015/0.014 ms.

This is a statement about the body, not every admission layer. The BN's unary
connection interceptor acquires `clientConnectionLock` before the body; for an
existing connection it performs a map lookup, and for a new connection it also
logs while holding that lock (`rpc/service.go:424–452`). The VC adapter's client
manager and connection provider have short bookkeeping locks. No retained marker
identifies one of these as a historical long holder. They are not additional
registry-sized work in DomainData.

Historical go.mod pins grpc-go v1.81.1. Prysm supplies no smaller
`MaxConcurrentStreams` or custom HTTP/2 flow-control windows in these server/client
constructors. grpc-go's server default is `math.MaxUint32`; the client starts
with 100 before initial SETTINGS and adopts `math.MaxUint32` when that first
SETTINGS omits a limit (`server.go:190`, `http2_client.go:1297–1299`). A permanent
100-stream bottleneck cannot be inferred from that initial client constant.

The client reader updates connection-level receive credit independently of
application consumption (`http2_client.go:1185–1218`). Its stream receive buffer
uses a nonblocking channel attempt and a backlog (`transport.go:79–102`). A slow
application consumer of StreamSlots therefore does not inherently prevent other
streams' responses from being read through connection flow control. Per-stream
flow control, actual socket backpressure, and scheduling of the shared HTTP/2
reader/writer remain possible; no retained DomainData wire/runtime trace selects
one. In particular, sharing a connection is not evidence that an expensive sync
handler itself owns the reader until it returns.

## E1's longest envelope overlaps a goroutine-profile request

Retained inputs:

- `/tmp/prysm-startup3-round4-early-e1/rpcprobe.jsonl`
- `/tmp/prysm-startup3-round4-early-e1/profiles/timestamps.tsv`
- `/tmp/prysm-startup3-round4-early-e1/beacon3.log`

Genesis was Unix `1788617038` (`14:03:58 UTC`). The probe records start offset and
client envelope duration to microsecond precision. The probe bypasses the VC
domain-cache mutex, shares one gRPC connection between its two methods, and uses
a 12-second per-call timeout. Its timing includes client dispatch and context
cleanup, and the end timestamp is taken after acquiring a small output mutex;
these are not packet timestamps or exact gRPC-invoker boundaries.

| Observation | Genesis-relative interval, seconds | UTC interval |
| --- | --- | --- |
| DomainData, 4,271.995 ms | +42.352810 to +46.624805 | 14:04:40.352810 to 14:04:44.624805 |
| BN goroutine profile +43 | +43.022993818 to +46.632199045 | 14:04:41.022993818 to 14:04:44.632199045 |
| DomainData, 2,165.041 ms | +40.187748 to +42.352789 | 14:04:38.187748 to 14:04:40.352789 |
| Previous BN goroutine profile +37 | +37.025345920 to +39.024033940 | 14:04:35.025345920 to 14:04:37.024033940 |

The longest DomainData envelope overlaps approximately 3.602 seconds of the +43
profile request. It had already run for approximately 670.184 ms when that request
started. The 2,165.041 ms envelope occurs entirely between the +37 and +43
goroutine-profile requests, starting 1.164 seconds after the former completed and
ending 670 ms before the latter began. Continuous BN/VC CPU profiling was active
through both intervals. A prior snapshot can also change later scheduling; the
nonoverlapping interval is not a no-observer control.

The server's matching domain-body sequence is consistent with substantial delay
before body admission: body ID 155 starts at `14:04:40.350163510` and ends at
`14:04:40.350167217` (`beacon3.log:1566–1567`), near the end of the 2,165.041 ms
envelope; body ID 159 runs at `14:04:44.581407510–44.581409293`
(`:1599–1600`), near the end of the 4,271.995 ms envelope. This is temporal and
domain-order matching, not an end-to-end request ID or proof of wire-arrival time.
The phase logger captures timestamps in the caller and queues them without
blocking on log I/O (`/tmp/prysm-startup-repro/runtime/startupdiagnostic/`).

These measurements establish slow client envelopes around a cheap handler in the
instrumented E1 run. They do not establish a profile-free 4.272-second effect,
the transport instruction responsible, or a seconds-long VC domain-cache holder.
Actual E1 VC RANDAO calls completed quickly; the subsequent proposal RPCs failed,
as corrected in the slot-1 audit.

The [supplemental E1 pairing audit](domain-e1-prebody-envelope-results.md) now
preserves the unique known-caller sequence for body 155, the approximately
2.162415-second pre-body interval, and the output-lock exclusion for rows
103/104. The separate [H reader trace](domain-http2-reader-trace-results.md)
identifies a real ordinary-Domain transport scheduling mechanism; E1 does not
contain a trace proving that its two-second interval is that same reader wait.

## Completed bounded additional control

The recommended [real TCP pair](domain-data-tcp-realwork-results.md) has now
passed all fixture, finite-work, and response checks. Both arms admitted and
successfully answered all 32 probes under load. The scan/memoized medians were
1.962528/0.100469 ms, and maxima were 5.700482/5.512450 ms. Actual handler
maxima were 16.011/3.367 microseconds. No deadline expired and no seconds-scale
domain delay reproduced. No larger follow-up was run.

This is a useful negative result for the tested checkpoint/count pipeline.
It does not establish the historical VC's own invocation time or rule out the
omitted VC cache, retry, full server-interceptor, and more complex process
scheduling paths. The following rationale and bounded scope were recorded
before execution; the result above supersedes their prospective wording.

The useful missing edge is **actual checkpoint/count work in the BN process to
latency of the actual TCP gRPC DomainData path without profiling**. A new generic
timeout mock or a manually held VC domain mutex would add little. A fixed paired
test would materially improve the current causal evidence because the existing
HTTP payload test uses another transport and places the load on its client side,
while the clean sync test measures an application method with its own global
manager lock. Neither isolates the ordinary gRPC path through the loaded BN.

Recommended Sol scope, after the current CPU window is free:

1. Reuse the existing 120k, slot-zero, warm-checkpoint fixture and exactly the
   already exercised `GOMAXPROCS=4`, 6,144 workers, 15,000 finite jobs. Each job
   calls real `AttestationTargetState`; compare its real `ActiveValidatorCount`
   continuation with the verified memoized count. No larger cohort or delays.
2. In that same loaded server process expose the real ordinary DomainData method
   over TCP gRPC. Establish the external client's connection before release.
   Keep the client in a separate, otherwise idle process, with no load workers.
   Use the existing common-release/progress scheme for 32 probes in each arm,
   with the same 12-second per-call budget used by the earlier probe.
3. Record each client invocation/return and server admission/body completion in
   bounded in-memory records keyed by probe metadata; take return timestamps
   before output locking or serialization. Record completed jobs/active scans at
   admission. Buffer any gRPC stats events needed to distinguish pre-body from
   post-body delay. Do not take CPU profiles, goroutine snapshots, or runtime
   traces during this primary pair, and do not add sleeps or a synthetic holder.
4. Preserve both arms and all finite-work completion counts regardless of result.
   No escalation if the pair is null. A positive differential establishes this
   concrete BN-load-to-gRPC coupling. It still does not identify node169's
   historical scheduling split, and it cannot turn slot4's expired RANDAO into
   a long-running RANDAO request.

The VC cache's cross-domain serialization remains a concrete source-level
amplifier with existing focused tests. This additional control should not claim
to have reproduced a historical cache holder unless the real cache path is also
observed acquiring and holding that lock; such attribution is not required to
answer the narrower missing transport question.

An implementation can use an external `package blockchain_test` test, a small
exported test-only fixture helper for the existing service/state/checkpoint, and
a client subprocess of the same compiled test binary. This avoids an internal
blockchain-to-RPC-validator import cycle and production edits. Sol should choose
the smallest viable arrangement; retaining actual checkpoint work and the real
DomainData implementation matters more than a particular test-file layout.
