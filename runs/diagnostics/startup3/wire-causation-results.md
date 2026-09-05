# Engine response wire and scheduler evidence

This diagnostic compares two otherwise equivalent three-node startup loads. Each
source submitted 15,000 valid FFG single attestations over two seconds in each of
slots 1, 2, and 3 (45,000 accepted RPC submissions, no submission errors).

- H used the original slot-zero active-validator-count behavior on every BN.
- I2 enabled `PRYSM_DIAGNOSTIC_GENESIS_COUNT_ABLATION=1` on BN3 only. BN1 and
  BN2 retained H's original image and behavior, so vote origination and relay
  were not ablated.

The ablation is diagnostic-only and deliberately unsafe as a production setting.

## Controlled result

| measurement | H: original BN3 | I2: BN3-only ablation |
| --- | ---: | ---: |
| BN3 CPU samples over 50 seconds | 152.51 CPU-s | 21.67 CPU-s |
| `ActiveValidatorCount` cumulative CPU | 137.77 CPU-s (90.34%) | 0 samples |
| engine probes represented in PCAP and diagnostic log | 64 | 70 |
| probe results | 61 success, 3 timeout | 70 success, 0 timeout |
| maximum response-on-wire to Go callback | 684.338 ms | 13.997 ms |
| maximum request-on-wire to response-on-wire | 3.291 ms | 1.666 ms |
| maximum response-on-wire to BN kernel ACK | 12.433 ms | 0.035 ms |
| slot 1/2/3 block outcome | slot 1 failed; slots 2/3 RPCs exceeded their deadlines | all submitted |
| slot 1/2/3 `GetBeaconBlock` duration | 649.308/20,669.607/9,989.437 ms | 160.710/35.124/25.866 ms |

I2 submitted blocks at slot start +0.32, +0.09, and +0.10 seconds. Its source
first-attempt offsets were 0, 7, and 7 ms, and last-attempt offsets were 1,999,
2,000, and 1,999 ms. Thus the control did not remove the load at BN1 or its
relay through BN2; it changed only BN3's repeated genesis-state count scan.

## H timeout packets and runnable delays

The packet capture is on BN3's exact host veth, filtered to the snooper proxy at
TCP port 8561. The response timestamp is the packet containing the complete
2,183-byte TCP response (2,059-byte JSON body). `ACK after response` is the next
BN-to-proxy packet acknowledging the end sequence number.

| diagnostic / JSON-RPC ID | request wire | complete response wire | ACK after response | response to callback | post-network runnable, not running |
| --- | --- | --- | ---: | ---: | ---: |
| 33 / 28 | 15:11:07.221470 | 15:11:07.222006 | 0.009 ms | 327.389 ms | 324.930 ms |
| 46 / 40 | 15:11:13.118893 | 15:11:13.119419 | 0.009 ms | 684.338 ms | 682.410 ms |
| 95 / 75 | 15:11:30.657656 | 15:11:30.658228 | 0.011 ms | 416.502 ms | 412.904 ms |

Runtime trace correlation places the corresponding Go goroutines in Runnable
state, after network readiness, for the durations above; their subsequent
Running intervals were only 16.639, 21.056, and 29.247 microseconds. Packet
readiness to netpoll unblock took approximately 2.393, 1.852, and 3.477 ms.
Therefore these three local failures were not slow EL execution, proxy service,
wire delivery, or delayed kernel acknowledgement. The response was already at
BN3 and acknowledged; the dominant delay was the runnable goroutine waiting to
be scheduled while BN3 executed the repeated validator scans.

I2 is the causal counterfactual for this local reproduction: changing only the
BN3 scan behavior removed its scan-dominated CPU profile, all three engine probe
timeouts, the long engine callback gaps, and all three proposal failures. This
directly establishes causation for the controlled local reproduction. Applying
that mechanism to the archived historical runs remains a source- and
timing-supported inference because those runs did not contain runtime traces or
packet captures.

## Reproduction of the packet table

`extract-engine-wire.py` parses only JSON bodies and TCP metadata; it does not
emit authorization headers or JWT values.

```bash
python3 runs/diagnostics/startup3/extract-engine-wire.py \
  --pcap PRIVATE/evidence/bn3-veth-engine.pcapng \
  --beacon-log PRIVATE/results/beacon3.log \
  --bn-ip BN3_IP
```

H PCAP SHA-256: `7d12c0ef5aecac7a6c590657ce745c1dbd18506e2d09102257897383fb33d2f3`.
I2 PCAP SHA-256: `13b9c3559efcf36716861d6c6ffd1ecb96f6f58a6643188042a12f48078d8e1b`.
Run I completed but is excluded from the matched comparison because a trace
analysis process competed for host CPU during its measurement window. I2 was
repeated with that analysis stopped before genesis and throughout capture.

Both retained runs used BN image
`sha256:292859db27d957440ff62b93783fd3eee3152c1fcae6cff1c1d92b82a9d3e278`.
Artifacts are under `/tmp/prysm-startup3-wire-h` and
`/tmp/prysm-startup3-wire-i2`. The filtered H scheduler reconstruction is
`evidence/engine-runtime-filtered.jsonl`; the three relevant callback goroutine
IDs are 5031, 25711, and 35860. H's first namespace-capture attempt lacked host
privileges; its valid packet evidence is the replacement exact-veth dumpcap
capture, which began before slot 1. All diagnostic enclaves were stopped after
capture.
