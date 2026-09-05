# Payload timeout root cause: repeated genesis counting starves response servicing

The strongest supported code explanation for round-2 slots **5, 6, 8, and 9**
is excessive concurrent work in `ActiveValidatorCount`, on the same beacon
process that must service a **300 ms** execution RPC. This is a concrete
implementation mechanism demonstrated by the retained H/I2 experiment, not
merely the observation that a proxy answered quickly while a caller timed out.
The historical owners have matching symptoms and execute the affected source;
their precise HTTP return site is inferred rather than measured. Proxy buffering
and a final response/deadline select are not equally evidenced competing root
causes: neither identifies the sustained work that uses up the budget, whereas
the count scan has a source explanation, a CPU profile, a runnable-reader trace,
and a receiving-node-only causal ablation.

This independent pass read historical Prysm revision
`0280403c70d88967f49d2d4c730f4c5417dabdf5`, geth RPC v1.17.5, the exact
rpc-snooper v0.0.21 checkout, retained H/I2 profiles and trace reconstruction,
and the four owner logs. It made no production changes or devnet rerun.

The complete causal chain is:

1. Round-0 FFG validation obtains the genesis checkpoint state at **state slot
   zero**, even when the attestation's own slot is nonzero. The recent-state
   shortcut excludes target round zero. `AttestationTargetState` returns this
   checkpoint to `validateCommitteeIndexAndCount`.
2. `ActiveValidatorCount` obtains a nonzero cached count, but
   `activeCount != 0 && s.Slot() != 0` refuses it for this state. Once a committee
   entry exists, the function skips its cold-cache singleflight helper and
   directly iterates the entire registry. Every qualifying concurrent vote can
   perform its own scan of the same 120,000 validators.
3. Each iteration calls `validatorsMultiValue.At`, which takes and releases the
   shared slice's read lock. A full count therefore performs 120,000 registry
   visits and at least **240,000 shared reader-count atomic updates**, in
   addition to field reads, map checks, and iterator work. Read locks permit
   concurrent readers; they do not make their shared atomic bookkeeping free.
4. This work occupies the beacon process's Go execution capacity and produces
   long runnable queues. HTTP response readiness only makes the `net/http`
   read-loop goroutine runnable; it does not reserve CPU for that goroutine.
   Under H's actual scan load, the complete response had already arrived and
   been acknowledged while that read loop waited 325–682 ms to run.
5. `execution.GetPayload` gives the RPC 300 ms of wall-clock budget. When
   transport/envelope servicing or the caller resumes after the context has
   expired, the RPC can return a deadline error despite a prompt EL response.
6. Error translation converts that deadline to the standalone `ErrHTTPTimeout`
   sentinel. The proposer's cached-payload fallback condition checks for the
   original `context.DeadlineExceeded`; the translated sentinel fails that
   check. The caller returns the exact historical `could not get cached payload`
   error. No cached P2P bid exists, so `buildBlockGloas` returns a build error
   before joining its parallel consensus work.

The principal source anchors are
`blockchain/process_attestation_helpers.go:22,104`,
`blockchain/receive_attestation.go:41`,
`sync/validate_beacon_attestation.go:270`,
`core/helpers/validators.go:145`,
`state/state-native/getters_validator.go:227`,
`container/multi-value-slice/multi_value_slice.go:255`,
`execution/engine_jsonrpc.go:311`, and
`rpc/prysm/v1alpha1/validator/proposer_execution_payload.go:90`.

For scale, **15,000 calls reaching this count** imply 1.8 billion validator
visits and at least 3.6 billion reader-count atomic updates. This is conditional
arithmetic, not an assertion that all 15,000 source submissions reached a given
historical BN or that every submitted vote completed validation. Pubsub has
admission throttles, and the earlier B experiment explicitly measured dropped
validation attempts. The libp2p version selected by the historical source has
an 8,192 global asynchronous-validation cap and a 1,024 per-topic cap; Prysm's
`p2p/pubsub.go:163` changes validation queue size, not those concurrency caps.
Those limits allow a substantial cohort to remain active relative to H's four
Go execution slots.

## The implicated cache behavior has a precise history

The slot-zero guard itself was intentional. Commit
`640bba8a6c7e613735570ae9dcace52a82c620e0`, “Avoid active validator count cache
for genesis (#6292),” added the guard and a test deliberately setting a cached
count of 3 for a genesis registry containing 1,000 validators. The test still
exists at `core/helpers/validators_test.go:320`. Blindly removing the guard
would discard an existing correctness condition: a seed alone is not generally
enough to identify the active count of a mutable genesis state.

Commit `134e020be1c44223193f6193d8cae8883d661bea`, “Update committee cache
(#16814),” made cache filling asynchronous. In the previous implementation,
`ActiveIndicesCount(ctx, seed)` first waited for an in-progress operation, and
`MarkInProgress(seed)` excluded concurrent full-count scans for the same seed.
A caller losing the race to `MarkInProgress` waited and returned the count.
Other callers could still subsequently start another serial genesis scan: the
old behavior was not reliable memoization of an entire cohort.

The new implementation moves singleflight into `scanActiveValidatorIndices`
and calls that helper only when the committee entry is absent. The **warm
slot-zero branch** directly counts without the old per-seed exclusion. This is
a source-demonstrable regression in concurrent scan exclusion. It makes the
existing expensive fallback capable of running in many goroutines at once,
increasing runnable competition and shared-reader atomic work. It does not
prove that an old-version 1,000-node run would have succeeded.

The per-validator slice access was already present before the iterator
conversion; this audit does not identify the iterator syntax change as the
introduction of that locking cost.

The count helper also does not inspect its `ctx` anywhere. Neither `Seed`, the
new `ActiveIndicesCount`, `HasEntry`, nor the iterator receives a cancellation
signal. Once the fallback begins, cancellation does not cut short its 120,000
visits. The gossip wrapper's context budget is **30 seconds**
(`sync/subscriber.go:40,549`), and it invokes validation synchronously. Its
timer cannot forcibly stop such code. This is an additional source-level
amplifier; the historical logs do not count how many scans outlived their own
gossip contexts.

## Retained experiment: causal evidence, independently checked

The raw H CPU profile reports **152.51 sampled CPU-seconds over 50.05 seconds**.
`ActiveValidatorCount` accounts for **137.77 seconds, 90.34%** cumulatively.
Focusing the profile on that call path leaves **73.43 seconds** of flat samples
in `sync/atomic.(*Int32).Add`. The complete profile's atomic total is 73.51
seconds; the small difference belongs to other call paths. This directly ties
the dominant atomic work to repeated counting rather than to a generic claim
about a large registry-cleanup map.

The trace reconstruction identifies `net/http.(*persistConn).readLoop` under
`internal/poll.(*FD).Read`, not geth's non-HTTP multiplexed RPC dispatcher.

| H diagnostic ID | BN kernel ACK after complete response | Read loop runnable time before callback | Actual Go running time before callback |
| ---: | ---: | ---: | ---: |
| 33 | 9 microseconds | 324.930 ms | 16.639 microseconds |
| 46 | 9 microseconds | 682.410 ms | 21.056 microseconds |
| 95 | 11 microseconds | 412.904 ms | 29.247 microseconds |

The response-to-network-unblock intervals were only about 2.393, 1.852, and
3.477 ms. Once the reader ran, it reached the response callback in tens of
microseconds. This separates CPU scheduling delay from EL computation, proxy
copy time, network delivery, and payload decoding in those three failures.

An additional check of H's retained metrics excludes a long GC pause: the
maximum `go_gc_duration_seconds` pause was **8.32246 ms**, and all 45 recorded
pauses total **12.564248 ms**. The count is below the 256-pause retention bound.
Even the total is far smaller than any of the three runnable intervals.

I2 changes only BN3's count behavior while keeping the original scan behavior
and offered load on BN1 and BN2. This pass also checked the saved container
configurations: only I2 BN3 has `PRYSM_DIAGNOSTIC_GENESIS_COUNT_ABLATION=1`;
both runs' three BNs have `GOMAXPROCS=4`. Their source result files each report
15,000 accepted submissions and zero submission errors in each of slots 1–3,
spread over approximately two seconds. BN3's sampled CPU falls to **21.67 seconds**,
its probe results change from **3/64 timeouts to 0/70**, and maximum
response-on-wire to Go callback falls from **684.338 ms to 13.997 ms**. All
three real slot-1/2/3 proposals also succeed in I2. The cached-payload probes
are diagnostic reads, so their particular timeout IDs must not be described as
the real proposal RPCs. The combination nevertheless establishes that removing
the receiving BN's repeated scan removes its measured reader starvation and
proposal failures under this load.

The source-level reason a 300 ms context can return much later is also
specific. Go 1.26.5's `context.WithDeadlineCause` installs `time.AfterFunc`;
`time.goFunc` launches the cancellation callback as a goroutine. The callback
and the RPC caller both require scheduling. Expiring the wall-clock deadline
does not grant either of them immediate CPU. The measured terminal log time
can therefore include late cancellation processing and late error reporting,
in addition to response-reader delay. No new timeout duration should be
inferred from a 0.8- or 1.8-second historical request-to-error-log interval.

Raw inputs are `/tmp/prysm-startup3-wire-h/results/profiles/bn-cpu-0-50.pb.gz`,
`/tmp/prysm-startup3-wire-h/evidence/engine-runtime-filtered.jsonl`,
`/tmp/prysm-startup3-wire-h/results/bn3-metrics-after.prom`, and the matching
I2 directory. [The retained wire comparison](../startup3/wire-causation-results.md)
documents the PCAP identities and packet extraction command. This pass reran
`go tool pprof -top` and `-focus=ActiveValidatorCount` on the retained profiles;
it did not rerun the load.

## A second concrete interaction: error translation disables the deadline fallback

This consequence of the lossy error mapping goes beyond observability:

```go
// execution/jsonrpc_error.go
if isTimeout(err) {
    return ErrHTTPTimeout // standalone errors.New sentinel
}

// validator/proposer_execution_payload.go
if !errors.Is(err, context.DeadlineExceeded) {
    return nil, errors.Wrap(err, "could not get cached payload from execution client")
}
// Only the original deadline identity reaches the uncached/prebuild fallback.
```

`ErrHTTPTimeout` has no unwrap relationship to `context.DeadlineExceeded`.
Thus the production execution service's mapped deadline cannot take this
fallback. The existing `TestServer_getExecutionPayloadContextTimeout` at line
336 injects a raw deadline through an engine mock. It tests the fallback when
deadline identity is preserved, bypassing the execution layer that actually
erases it. Its fixture then returns an empty pre-activation payload; it is not
a demonstration of a second successful Gloas Engine request.

All four historical errors contain the wrapper produced by this early return.
Their terminal build errors additionally identify `no cached P2P bid available`.
Those messages establish the early-return branch directly. Restoring deadline
identity could make the source fallback reachable; it would not by itself
prove that any particular historical proposal had enough remaining time or CPU
to recover.

## Historical fit and alternatives

| Historical slot / owner | Proxy copy duration | Proxy response to BN timeout log | Whole-slot deadline already reached at terminal payload failure? |
| --- | ---: | ---: | --- |
| 5 / 118 | 3 ms | 1,869.824 ms | Yes |
| 6 / 83 | 2 ms | 308.064 ms | No; failure at slot +4.457 s |
| 8 / 19 | under 1 ms | 810.412 ms | Yes |
| 9 / 107 | 22 ms | 1,507.219 ms | No; failure at slot +7.639 s |

These are log-to-log differences, not historical runnable durations. The
matched calls returned HTTP 200 and complete payload JSON, with no corresponding
proxy-copy failure record. The 30-second authenticated HTTP client timeout
cannot explain these calls; the separate Gloas deadline is 300 ms. Slots 6 and
9 show that the whole-slot deadline is unnecessary to produce the failure.
Slots 5 and 8 additionally reached block construction late.

The exact snooper forwards through Negroni's response wrapper and records its
duration after `io.Copy` into that response writer. Its deferred body close
starts a separate response logger. There is no synchronous response-module or
log-formatting wait that explains a missing additional 300 ms. Buffered
downstream delivery remains an unmeasured historical boundary, but buffer
existence alone is not evidence of a long delay. H closes that boundary and
finds already-acknowledged bytes waiting for BN execution instead.

Geth's HTTP client decodes the JSON-RPC envelope synchronously, then selects
between an already-buffered response and `ctx.Done()` in `requestOp.wait`.
The context can win if both are ready. This affects which return branch can
produce the timeout **after** the budget has been consumed; it does not explain
why a promptly answered request used that budget. Payload-specific result
unmarshalling occurs after this select without a subsequent context check, so
slow result unmarshalling alone cannot produce this mapped deadline error.

Finally, wall slots 8 and 9 do not exclude genesis scan work. New round-1 votes
can use a nonzero-slot checkpoint, but old round-0 votes remain admissible to
the earlier gossip check within the epoch window. Historical node 201 records
a round-0 vote entering validation in wall slot 16. No hard round transition
instant drains all earlier scans or prevents new delayed round-0 scans.

The defensible conclusion is a **demonstrated counting/CPU/scheduler failure
mechanism and a directly observed timeout/fallback abort path**, with that
mechanism the best supported explanation for the historical owners. The missing
owner traces limit attribution of the exact old response-servicing interval;
they do not reduce this code mechanism to an unsupported guess or make every
logically possible transport delay equally supported.
