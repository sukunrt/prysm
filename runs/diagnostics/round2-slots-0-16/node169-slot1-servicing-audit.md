# Node 169 slot-1 local servicing timeline

## New bounds

Node 169 was not wholly frozen while its slot-1 proposal dependency remained
unresolved. The BN completed a separate execution-head poll during the slot,
and one of four same-slot PTC roles received a real `NotFound` response from
`PayloadAttestationData` at genesis **+21.915067 s**. Three sibling PTC calls,
75 attestation-data calls, two sync-message-root calls, the RANDAO domain call,
and later aggregation calls instead reported the common +24-second deadline.
This is direct evidence of uneven request progress within one VC/BN pair. It
does not timestamp the RANDAO request's dispatch or arrival at the server.

The first slot-1 fork-choice tick produced two BN markers at +12.117788 and
+12.117835 s. Its engine FCU reached the proxy at +12.166999 s and returned a
valid payload ID at +12.167373 s. The next execution head poll reached the
proxy at +14.772568 s and its response was logged at +14.773037 s. The BN then
logged that poll's `handleETH1FollowDistance` result at +15.022065 s, **249.029
ms** after the proxy response marker. Thus both fork-choice/engine and periodic
execution-service work made progress during the slot-1 duty window.

The next `UpdateHead`-associated BN marker is `Chain reorg occurred` at
**+26.890832 s**. Source schedules the attestation routine at +22 s (late-slot
interval for slot 1) and +24 s (slot 2 start). Either call may return without a
log, and the retained line carries no proposing-slot field. The marker is
therefore at least **2.890832 s** after the later scheduled tick and would be
**4.890832 s** after the earlier one; the archive cannot select between them.
The stronger claim that this exact line is the +22-second call is not
recoverable from the retained logs.

This delay is not container log collection. BN's inner timestamp has
centisecond precision, so each true logging instant is bounded by its displayed
10 ms interval. Comparing that interval with the outer collector timestamp
gives these bounds:

| BN line | Outer offset | Marker | Collector delay bound |
| --- | ---: | --- | ---: |
| 516 | +12.117788 s | late-reorg decision | 0--7.788 ms |
| 517 | +12.117835 s | chain-reorg marker | 0--7.835 ms |
| 518 | +15.022065 s | execution follow-distance result | 2.065--12.065 ms |
| 519 | +26.890832 s | chain-reorg marker | 0.832--10.832 ms |
| 520 | +32.027596 s | execution follow-distance result | 0--7.596 ms |

The following execution poll reinforces the intermittent BN-continuation
delay. Proxy sequence 711 logged its response at +28.755420 s, while the BN's
corresponding follow-distance result appeared at +32.027596 s, **3.272177 s**
later. The proxy/EL completed both polled methods quickly; the interval can
include proxy logging, BN response read/decode, goroutine scheduling,
`processBlockHeader`, and `handleETH1FollowDistance`. It is not a pure transport
or scheduler measurement.

## Exact local sequence

| Genesis offset | Component | Event |
| ---: | --- | --- |
| +12.117788 s | BN | Late-block override decision at the first slot-1 tick |
| +12.117835 s | BN | `Chain reorg occurred`, zero-depth genesis-to-genesis update |
| +12.166757 / +12.166803 s | EL | Payload work starts and updates payload `0x0466ae591b684070` |
| +12.166999 / +12.167373 s | proxy | FCU sequence 709 request / valid response with that payload ID |
| +14.772568 / +14.773037 s | proxy | `eth_getBlockByNumber(latest,false)` sequence 710 request / response |
| +15.022065 s | BN | Follow-distance result from the poll |
| +21.915067 s | VC | One `PayloadAttestationData` call returns `NotFound`; role logs skip |
| +24.001279--+24.018254 s | VC | Common deadline burst: 88 retained role-error lines |
| +24.002452 s | VC | Proposer logs RANDAO DomainData `DeadlineExceeded` |
| +24.166650 s | EL | Payload ID 0466 expires after its 12-second work timeout |
| +26.890832 s | BN | First later retained `UpdateHead`-associated completion marker |
| +28.754657 / +28.755420 s | proxy | Next execution-head poll request / response |
| +32.027596 s | BN | Its follow-distance result appears 3.272177 s later |
| +33.849736 / +33.850656 s | proxy | Attribute-free FCU sequence 712 request / valid response |

The 88 deadline-window VC lines comprise 75 attestation-data failures, three
payload-attestation-data failures, two sync-message-root failures, one RANDAO
failure, one sync-subcommittee-index failure, three aggregate-data failures,
and three aggregate-selection failures. Counts are
log records, not outstanding-stream counts. In particular, `performRoles`
starts roles in goroutines, but no retained marker dates the proposer goroutine
start or the generated DomainData stub invocation.

## Source interpretation

At historical revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`,
`receive_attestation.go:92--113` creates an unbuffered interval ticker at slot
offsets zero and ten seconds and calls `UpdateHead` synchronously. The ticker
sender does not compute or deliver its next tick until the receiver accepts the
current one (`time/slots/slotticker.go:150--172`). `UpdateHead` holds the
fork-choice write lock while processing its pool (`receive_attestation.go:
120--148`) and may return without a marker when the head is unchanged (`:158--
159`). Its eventual `saveHead` can emit the zero-depth reorg line.

The independent execution poll comes from the execution service's 14-second
`eth1HeadTicker`. After `HeaderByNumber` returns, the same service goroutine
calls `processBlockHeader` and `handleETH1FollowDistance`; the latter emits the
BN warning (`beacon-chain/execution/service.go:637--650,493--520`). This makes
the proxy-response-to-warning interval a useful full-continuation bound, but
not a measurement of one lock.

The real VC also has a synchronous edge absent from the earlier direct-stub TCP
fixture. `validator.domainData` takes one global write lock for every uncached
domain and holds it across the complete gRPC call (`validator/client/validator.go:
721--757`). A stalled different-domain miss can therefore delay RANDAO before
it reaches the generated client stub. The historical terminal error proves
that RANDAO eventually acquired this lock, missed the cache, and invoked the
adapter; it does not identify an earlier holder. Existing evidence ranks this
as a possible amplifier rather than a recovered cause.

The production gRPC path additionally uses OTel stats, recovery, Prometheus,
OpenTracing, and a connection-tracking interceptor on the server, and OTel,
OpenTracing, Prometheus, retry, and optional debug logging on the client
(`beacon-chain/rpc/service.go:163--181`; `validator/client/service.go:352--370`).
The connection interceptor takes `clientConnectionLock` for every unary call
(`beacon-chain/rpc/service.go:424--449`). Node 169's only new-connection log is
pre-genesis at `beacon.log:39`, so slot-1 calls take the map-hit path. These are
real omissions from the simple fixture, but source supplies no seconds-scale
operation or historical holder among them. Prysm configures no custom gRPC
worker pool or reduced `MaxConcurrentStreams` in these constructors.

## Reproduction and limits

`analyze_node169_slot1_timeline.py` reads only the already extracted node-169
directory. Its default +12 to +34 second window retains every BN, VC, and EL
top-level log line plus every proxy request/response header in that interval.
It separately pairs the proxy's exact logged JSON by snooper sequence number.

Run:

```text
python3 runs/diagnostics/round2-slots-0-16/analyze_node169_slot1_timeline.py
```

Generated files:

- `node169-slot1-component-timeline.tsv`: 105 ordered event rows, including all
  five BN lines and all 89 VC lines in the window;
- `node169-slot1-engine-rpc.tsv`: four paired engine RPCs with canonical logged
  request and response JSON and raw anchors.

The snooper's header timestamps are logging events, not socket boundaries. In
sequence 709 the EL's payload-update outer timestamp precedes the proxy request
header by 0.197 ms, and the proxy reports a 4 ms request duration although its
two header timestamps are only 0.373 ms apart. This explicitly prevents using
the header delta as wire latency. The paired JSON is exact as retained by the
snooper, including its own large-field abbreviations; it is not a byte-for-byte
wire capture.

The timeline rules out an EL outage, total BN process freeze, and multi-second
container logging lag during slot 1. It supports intermittent servicing and a
multi-second delayed fork-choice/execution continuation. It cannot determine
whether RANDAO waited in VC scheduling, the domain-cache lock, grpc-go, BN
admission, or response delivery, because none of those boundaries was logged.
