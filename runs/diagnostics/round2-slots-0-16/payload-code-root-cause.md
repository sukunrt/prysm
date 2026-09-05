# Why a fast Engine response can become a payload timeout

The demonstrated code mechanism is **repeated full genesis-validator scans
delaying the beacon node's HTTP response reader beyond its 300 ms deadline**.
The previous slot report located terminal failures but gave this root-cause
evidence insufficient prominence. Exact scheduling of each historical owner
is not recorded; the mechanism itself has a real-network causal control, CPU
profile, packet capture and runtime trace.

## The excessive work

At historical revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`, incoming
FFG beacon-attestation validation calls `ActiveValidatorCount` for its
checkpoint state (`validate_beacon_attestation.go:292`). The helper first
reads a cached count but refuses to return it when the supplied state is at
slot zero (`core/helpers/validators.go:150–155`). When the cache already
contains an entry, it falls through to a full validator loop (`:158–172`).

This is the **checkpoint state's slot**, not the current wall-clock slot.
Votes targeting the genesis checkpoint can keep using that frozen state
while later slots are underway. Moving the clock past slot zero does not
remove this work. The historical observers record old round-zero votes
continuing into wall slot 16; see [retention-deep.md](retention-deep.md).

For this 120,000-validator state, each qualifying vote visits all 120,000
validators again. `ValidatorsReadOnlySeq` holds a state read lock for the
iteration and calls the shared registry's `MultiValueSlice.At` per validator.
Each `At` takes and releases the same storage `RWMutex`
(`state/state-native/getters_validator.go:227–248`,
`container/multi-value-slice/multi_value_slice.go:255–257`). Consequently a
15,000-vote workload reaching this check requires 1.8 billion validator visits
and approximately 3.6 billion shared reader-count atomic operations. These
are redundant count calculations on an unchanged checkpoint.

The helper's context does not interrupt this loop. The resulting runtime
pressure affects unrelated RPC and HTTP goroutines in the same beacon
process, even though those goroutines do not need the validator registry.

The [historical representation audit](historical-count-path-transfer-audit.md)
confirms that this storage and iterator path is unconditional in the deployed
revision. The reproductions have the same 120,000-entry registry size and
production implementation; their genesis validator keys/root and CPU
allocation are not claimed to be identical to the historical network.

## The deadline and failure propagation

`execution.GetPayload` creates a child deadline **300 ms from entry** and
passes it to the real geth JSON-RPC client (`engine_jsonrpc.go:303–326`).
That wall-clock budget includes the beacon process's own scheduling,
transport and response-reading time. A prompt server response therefore does
not guarantee a successful call: the beacon still has to run the code that
receives and processes it before its deadline.

Prysm then maps any error implementing `Timeout() == true`, including a
context deadline, to `timeout from http.Client`
(`execution/jsonrpc_error.go:35–40,96–107`). This wording does not identify
the separate 30-second authenticated HTTP-client timer. The mapping also
changes behavior, not just logging: `ErrHTTPTimeout` does not retain the
original error for `errors.Is`.

The cached-payload caller checks **only**
`errors.Is(err, context.DeadlineExceeded)` before falling through to local
payload preparation (`proposer_execution_payload.go:95–105`). The real
execution layer has already erased that identity, so a mapped deadline takes
the immediate `could not get cached payload from execution client` return.
All four historical errors have that exact prefix. Thus the request does not
take this local recovery path after its failed cached retrieval.

The existing `TestServer_getExecutionPayloadContextTimeout` supplies a mock
that returns the raw `context.DeadlineExceeded`. Its pre-transition fixture
then falls through to an empty payload; it does not exercise the real error
translation and does not establish retry behavior for Gloas.

The new [executed Gloas differential](payload-recovery-reproduction.md)
confirms the actual recovery consequence: raw deadline → one fresh FCU and
two payload calls in total; translated timeout → no FCU and one payload call.
Both assertions pass through the real proposer method with a controlled
engine fixture and a live parent context.

The builder treats failed local retrieval as unavailable input and checks its
P2P-bid cache. All four historical errors explicitly report no cached bid.
It returns before joining the parallel consensus branch
(`proposer_gloas.go:38–42`). A later packing cancellation is abandoned work;
it cannot revive the proposal after this return.

This error-identity mismatch is a second concrete code interaction. It does
not prove that a retry would have saved each proposal: slots 5/8 exhausted
their parent deadlines as well. Slots 6/9 did still have overall slot budget
when local retrieval failed, but continued CPU pressure could have delayed a
second request too.

## The measured causal control

The retained H/I2 experiment isolates the receiving beacon node. Both runs
offered the same 45,000 valid FFG submissions through two upstream nodes;
I2 removed only BN3's repeated genesis-count scans. These are separate local
experiments, not profiles recovered from the historical 1,000-node run.

| Measurement | H: repeated scans | I2: receiver-only count ablation |
| --- | ---: | ---: |
| Beacon CPU sampled over 50 seconds | 152.51 CPU-seconds | 21.67 CPU-seconds |
| CPU attributed to `ActiveValidatorCount` | 90.34% | No samples |
| Engine probe timeouts | 3 of 64 | 0 of 70 |
| Maximum response-on-wire to HTTP first-byte callback | 684.338 ms | 13.997 ms |
| Real proposals in slots 1–3 | All failed | All submitted |

For the three H probe failures, complete responses were captured at BN3 and
acknowledged by its kernel within 9–11 microseconds. Runtime traces identify
`net/http.(*persistConn).readLoop` as **Runnable, but not Running**, for
324.930, 682.410 and 412.904 ms after network readiness. Once scheduled, the
reader used only 16.639, 21.056 and 29.247 microseconds of running time before
its callback. Thus these timeouts do not come from slow EL construction,
proxy service or missing network delivery. The response was waiting for the
loaded beacon runtime to service it.

The H metrics also exclude a large stop-the-world GC pause for these gaps:
the maximum recorded pause was 8.322460 ms; all 45 pauses totaled 12.564248 ms
(`results/bn3-metrics-after.prom:1690–1696`). The raw CPU profile independently
places 73.51 CPU-seconds, 48.20% of all samples, in atomic reader-count updates.

Raw artifacts and packet/runtime reconstruction are documented in
[wire-causation-results.md](../startup3/wire-causation-results.md). Root
independently reopened both CPU profiles and H's runtime/GC records during
this follow-up.
The [observer-risk audit](observer-risk-and-sync-handoff-audit.md) also checks
H's profile-request schedule: none of its three measured read-loop windows
overlaps a discrete goroutine-profile request. Continuous CPU profiling and
runtime tracing ran in both H and I2. The new direct Go reproduction below
provides a separate causal check without all-goroutine snapshots.

## What jj history adds

The slot-zero guard is intentional: commit
`640bba8a6c7e613735570ae9dcace52a82c620e0` added it with a regression test that
places an incorrect count in the seed-keyed cache and requires genesis to
recount correctly. The test survives as `TestActiveValidatorCount_Genesis`.
Blindly deleting the guard is therefore not a justified production fix.

Commit `134e020be1c4` later removed the old per-seed in-progress protection
from the warm-slot-zero fallback. Previously, count calls could wait for an
in-progress scan; at most one helper scan held that seed's marker. The current
fallback permits simultaneous independent scans, magnifying shared reader
atomic contention. The old version could still perform repeated serial
counts, so this history does not establish that reverting it alone would
make the historical experiment pass.

Commit `ac40200a15701d4fb972ca60c131b21d21dea37a` reduced Gloas retrieval's
budget from one second to 300 ms. This is the threshold against which the
runtime delay becomes a failed payload request. Increasing a timeout would
not remove the redundant work.

## Applying the mechanism to the four historical failures

| Slot / owner | Matched proxy duration | Proxy response to beacon timeout log |
| --- | ---: | ---: |
| 5 / 118 | 3 ms | 1,869.824 ms |
| 6 / 83 | 2 ms | 308.064 ms |
| 8 / 19 | Under 1 ms | 810.412 ms |
| 9 / 107 | 22 ms | 1,507.219 ms |

These are outer log intervals, not historical socket-read durations. All
responses are complete HTTP 200 results for the matching payload IDs, with no
matching downstream-copy error. Slots 6 and 9 fail before the overall
12-second proposal deadline; slots 5 and 8 also exhaust that larger budget.

The strongest code-backed historical explanation is the same redundant
genesis-count work starving response servicing. It is supported by exact
source, the historical startup workload, prompt matched responses and the
independent causal control. The old logs do not identify the precise
transport/context return instruction on each owner. Proxy buffering and
geth's final response/context selection describe possible visibility or error
boundaries; neither independently explains what consumed the budget.

## New bounded Go reproduction

The direct Go test calls the real `execution.Service.GetPayload` and geth
HTTP client against a separate, promptly responding server process. It uses
H's retained 120,000-validator genesis, a warm committee cache, four Go
execution processors, 128 workers and 1,024 finite count jobs. No injected
sleep stands in for the expensive count. The two count arms complete the
same 1,024 jobs; the memoized arm reuses the immutable fixture's verified
correct count.

The scan arm produces **4 timeouts in 16 payload calls**; both the no-load
and memoized controls produce **0 in 16**. All four errors are recognizable
context deadlines at the raw RPC boundary and become standalone
`ErrHTTPTimeout` values at the execution-service boundary, while the parent
test context remains live. Thus the real transport also reproduces the error
translation used in the separate Gloas recovery differential.

One failure, call 2, reproduces the fast-response pattern. The server flushes
its result in **23 microseconds**. The HTTP reader becomes runnable at that
time but waits **342.909 ms** before running; the first-byte callback follows
approximately 98 microseconds after that first scheduling. HTTP error cleanup
also waits approximately **343.464 ms** for the write loop to finish; that
goroutine itself is runnable for **343.462 ms** before executing. The raw
deadline returns at **686.606 ms** from RPC entry. A 300-ms context deadline
does not guarantee that a loaded process can return and log the error at
exactly 300 ms.

The other three failures happen before a request is written to the server;
they demonstrate additional client scheduling/acquisition failures under the
same count pressure, rather than three more fast-response failures. Whole-test
profiling attributes **8.44 of 8.53 sampled CPU-seconds (98.94%)** to
`ActiveValidatorCount`, including **5.34 seconds (62.60%)** in atomic reader
bookkeeping. The raw runtime user-CPU metric remains zero in every arm and
is unusable here; it must not be read as zero CPU consumption.

The [complete Go reproduction report](../startup3/payload-scheduler-repro-results.md)
records all four failure boundaries, exact commands and artifacts. Raw Go
output, server responses, runtime traces and profiles are retained in
`/tmp/prysm-payload-scheduler-pilot`; the executable reproduction is
`runs/diagnostics/startup3/run-payload-scheduler-repro.sh`. These results are
separate from H/I2 and from the historical owner observations above.
