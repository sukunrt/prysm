# E1 ordinary DomainData: a two-second pre-body interval

E1 contains a successful ordinary DomainData probe with approximately
**2.162415 seconds between the probe's start and entry into the matching BN
handler body**. The body takes **3.707 microseconds**. The complete 2.165041-second
probe envelope falls between discrete goroutine-profile requests. This is
separate evidence from [H's traced HTTP/2 reader delay](domain-http2-reader-trace-results.md):
E1 supplies a seconds-scale pre-body observation; H identifies a real reader
scheduling mechanism. There is no retained E1 trace that turns these into a
measured two-second reader wait.

## Exact retained records and request pairing

E1 genesis is `2026-09-05T14:03:58Z`; slot 3 starts at `14:04:34Z`.
The table uses UTC on 2026-09-05. Probe starts and envelope ends are reconstructed
from microsecond-truncated offsets/durations; BN timestamps are the caller's
nanosecond `captured_time`, recorded before asynchronous logging.

| `rpcprobe.jsonl` row | Probe start | Envelope end | Matching BN body, begin → end | BN source lines |
| --- | --- | --- | --- | --- |
| 102 | 14:04:35.300655 | 14:04:37.011405 | id 152: 37.008804583 → 37.008806697 | `beacon3.log:1539–1540` |
| 103 | 14:04:37.011420 | 14:04:38.187728 | id 154: 37.023384398 → 37.023387494 | `beacon3.log:1543–1544` |
| **104** | **14:04:38.187748** | **14:04:40.352789** | **id 155: 40.350163510 → 40.350167217** | **`beacon3.log:1566–1567`** |

Each probe requests epoch 0, domain `02000000`, using the generated DomainData
stub directly. There is one sequential Domain lane and one sequential sync-index
lane on the probe's shared gRPC connection; the probe has no VC domain-cache
mutex. Every row above reports `ok:true`. There is exactly one corresponding
BN `bn.domain_data.02000000` body inside each of these disjoint probe intervals.

The other known caller is E1's actual VC. All its own RANDAO RPCs are recorded:

| Slot | Own VC RANDAO begin → end, UTC | `validator3.log` lines |
| --- | --- | --- |
| 1 | 14:04:15.246352836 → 15.246693637 | 39870, 39873 |
| 2 | 14:04:22.019905047 → 22.028433093 | 40673, 40675 |
| 3 | 14:04:34.011901292 → 34.015406743 | 40889, 40892 |

The slot-three VC call matches the earlier BN body id 144 at
34.014393198–34.014396183 (`beacon3.log:1484–1485`). It is finished before the
probe sequence above. Probe rows 98 and 100 match body ids 148 and 150 at
34.304435423 and 34.803959191. Thus id 155 is not an overlooked own VC RANDAO
call or a body from an overlapping Domain probe. Neither retained VC nor BN
log contains a diagnostic `dropped` marker. This is a unique sequence-and-time
match among the retained callers, not a propagated request ID or packet match.

Row 104's start-to-body difference is about **2,162.415 ms**; only about
**2.622 ms** remains after the body end. Because the probe's start offset is
truncated to microseconds, the pre-body difference is between
2.162414511 and 2.162415510 seconds, assuming the shared wall clock. It includes
any client scheduling before the generated RPC invocation, transport delivery,
BN transport/handler admission, and pre-marker work. It is not an exact
server-queue duration.

## The output mutex does not explain this pre-body observation

The probe's general envelope includes context cleanup and acquisition of
`outputMu` after the RPC returns. That qualification is real: disassembly of
the retained `/tmp/prysm-startup-rpcprobe` binary shows the generated DomainData
call at `0xf50aaa`, mutex acquisition at `0xf50b99` / `0xf50bb9`, `time.Since`
at `0xf50c46`, and JSON encoding at `0xf50d2c`, in that order. This checks the
retained binary rather than assuming the current source's later missed-tick
fix was already present in E1.

For rows 103 and 104 specifically, the competing sync lane's previous emitted
record is row 101 and its next is row 105. It cannot hold the common output
mutex for seconds through these Domain records: encoding under that mutex
would place its output before them. The Domain lane cannot contend with itself.
More directly, a mutex acquired only after the RPC returns cannot manufacture
row 104's **start-to-server-body** delay. Client scheduling before RPC dispatch
remains inside that pre-body interval.

Row 103 has a different shape: about 1.164341 seconds lie **after** its body
marker. Even with the competing output-lock explanation excluded, that is an
after-body envelope, not proof of wire-response latency: client rescheduling
and post-RPC cleanup remain possible. Do not merge rows 103 and 104 into one
transport phase.

## Observer adjacency and the historical implication

The previous BN goroutine-profile request, +37, runs
14:04:35.025345920–37.024033940. The next, +43, runs
14:04:41.022993818–44.632199045. Row 104 begins **1.163714 seconds after** the
first finishes and ends **0.670205 seconds before** the second starts. Neither
its complete envelope nor its pre-body portion overlaps a BN or VC discrete
goroutine-profile request. Continuous CPU profiling was active, and an earlier
snapshot could have affected later scheduling; this is not a no-observer arm.
The separate 4.271995-second Domain probe at row 107 overlaps +43 and must not
be described as snapshot-free.

This 2.162415-second pre-body observation exceeds the **2.084933-second minimum
potential residual** inferred from node 169's historical role progress at slot
+9.915067. It therefore supplies a relevant delay magnitude that H's successful
139 ms RANDAO call alone does not. It does not establish that node 169 actually
had only that residual, that its request stalled in the same phase, or that
E1's delay was specifically its BN reader. E1's actual own RANDAO calls above
succeed. The supported combination is: the full-service count-ablation pairs
establish the overload cause, H traces an ordinary-Domain reader mechanism
under that load, and E1 independently records a seconds-scale pre-body delay
around the same cheap method.

## Retained evidence

- [Relevant probe rows with physical line numbers](domain-e1-prebody-probe-rows.txt).
- [BN body sequence and all actual VC RANDAO markers](domain-e1-prebody-markers.txt).
- [Profile request timestamps](domain-e1-prebody-profile-times.tsv).
- [Retained probe closure disassembly](domain-e1-prebody-probe.objdump.txt).

Original logs are under `/tmp/prysm-startup3-round4-early-e1/`. No runtime trace
or packet capture for this interval is retained there. Input SHA-256 values:

| Input | SHA-256 |
| --- | --- |
| `rpcprobe.jsonl` | `a5dd483425f4309557dfe6df734cc78945825ecab1d3d400c092ff3830ebbd38` |
| `beacon3.log` | `8c32faf4ae57a78599c3dbea48c5bbe16152f73f5cae65abaf306571962c3e01` |
| `validator3.log` | `400396a3cbe9092ec277175df4e7f6bbc8c2624334dd4c00b5f501f25cb8627e` |
| `profiles/timestamps.tsv` | `1b5e86e0360b57163c8b5c666a3383db9b522e0232708859e2b3b05f2601d95c` |
| `/tmp/prysm-startup-rpcprobe` | `fface64f136ad49fde6843690cc05e23c1171895a4dd49286f9dd4fc6d9dae90` |
