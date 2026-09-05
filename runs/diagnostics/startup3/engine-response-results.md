# Engine response diagnostic results

Run G tested the execution-response path during the reproduced genesis FFG CPU
pressure. A background diagnostic reused the payload ID from the first
successful real slot-1 `GetPayload` and issued one read at a time. It used the
same 300 ms Prysm `GetPayload` deadline but did not participate in a real block
proposal. Of 59 diagnostic reads, 58 succeeded and one timed out.

## Reproduced timeout

The timeout is client trace ID 109, JSON-RPC ID 94, and rpc-snooper request 95.
All three refer to cached payload ID `0x04570e240304a17c`. Geth originally built
that payload at 14:28:53.129 in 394.757 microseconds with hash
`0xe39adbbe6112cbe84ae8c2b6dd9bda6b349674583e9278ac3143daf7b3bcfc54`
(`execution3.log:78–80`). The failing probe therefore did not ask Geth to build
a new payload.

| Event | UTC timestamp | From RPC start |
| --- | --- | ---: |
| Probe attempt/RPC begin | 14:29:22.063236566 / 14:29:22.063244250 | 0 ms |
| Reused connection obtained | 14:29:22.063312628 | 0.068 ms |
| Request written | 14:29:22.063404911 | 0.161 ms |
| Approximate nominal client deadline | ~14:29:22.363244 | 300 ms |
| First response byte observed by BN | 14:29:23.837648127 | 1,774.404 ms |
| RPC returned timeout | 14:29:23.841391248 | 1,778.147 ms |

The snooper logged request 95 with JSON-RPC ID 94 and the same payload ID. It
logged a complete 2,059-byte HTTP 200 JSON result in 0 ms during second 22
(`snooper3.log:4252–4298`). Thus Geth had the cached payload and the proxy read
and copied its complete result promptly. The BN did not execute
`GotFirstResponseByte` until 1.774 seconds after writing the request, and it
observed its 300 ms deadline about 1.478 seconds late. The next diagnostic read
used JSON-RPC ID 95 and succeeded in 2.422 ms, which also rules out a persistent
unknown-payload or EL availability failure.

G alone left an important boundary open. rpc-snooper measures through copying
the upstream body into Go's `http.ResponseWriter`; it does not provide a wire
timestamp proving downstream flush or BN receipt. Run H subsequently closed
that boundary with packet capture and Go runtime tracing. In three independent
timeouts, the response reached and was acknowledged by the BN promptly, and
the `net/http` read-loop goroutine became runnable, but did not run for
324.928, 682.406, and 412.901 ms. Once scheduled, each reached the
`GotFirstResponseByte` callback in tens of microseconds. The detailed packet,
netpoll, and scheduler correlation is in
[`wire-causation-results.md`](wire-causation-results.md).

## CPU and GC context

The concurrent profile attributed 137.90 of 152.48 CPU-seconds (90.44%) to
`ActiveValidatorCount`. The +43-second goroutine snapshot immediately before
the timeout contained 4,235 validation goroutines. This is the same
registry-scan pressure and backlogged validation cohort established by the genesis
FFG diagnostics.

The pause was not a stop-the-world GC event. The largest observed
`go_gc_duration_seconds` pause was 0.000454742 seconds; 44 pauses totaled
0.003851569 seconds, with the GC count still below the pause buffer's 256-entry
retention bound. Those measurements exclude a 1.77-second GC pause in G.

## Causal conclusion

G first reproduced the essential round1 node81 shape; H then directly proved
the local mechanism: under the reproduced genesis-FFG pressure, a fast,
successful EL response can miss Prysm's 300 ms deadline because its already
runnable BN HTTP read loop is not scheduled. These are synthetic cached-payload
reads rather than real proposals. The historical archive has neither packet
capture nor runtime trace, so applying that exact final stage to node81 remains
a strong causal inference, not direct historical proof.
