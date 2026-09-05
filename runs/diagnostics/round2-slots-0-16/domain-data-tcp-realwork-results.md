# Real TCP DomainData under checkpoint/count work

## Result

The bounded pair did not reproduce a DomainData deadline or a seconds-scale
client delay. Both arms admitted all 32 probes while the finite workload was
active, and all 32 probes returned the exact 32-byte value established by the
warm call. The 12-second probe deadlines never expired.

| Arm | Work duration | RPCs OK / deadline | Client p50 / p95 / max | Admission max | Body max | Return-from-body max |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Memoized count | 37.692225 ms | 32 / 0 | 0.100469 / 0.282473 / 5.512450 ms | 5.443340 ms | 3.367 us | 112.823 us |
| Registry scan | 32.677052431 s | 32 / 0 | 1.962528 / 5.370001 / 5.700482 ms | 5.545339 ms | 16.011 us | 190.369 us |

“Admission” is the client invocation to the timestamp taken in the test
server's unary interceptor after extracting the probe metadata. “Body” begins
after the interceptor takes its bounded atomic counter snapshot and ends
immediately when the real `DomainData` method returns. “Return from body” ends
at the external client's stub return. The separate counter-snapshot interval
was at most 0.210 microseconds in the memoized arm and 0.280 microseconds in the
scan arm.

The real registry scan increased median and p95 admission delay in this pair,
but the largest complete client call was 5.700482 milliseconds. The result is
negative for the narrow hypothesis that this checkpoint/count pipeline alone
exhausts the ordinary DomainData RPC budget. It does not resolve the historical
division among VC scheduling, VC domain-cache locking, retry handling, and the
shared HTTP/2 transport described in [domain-data-causal-audit.md](domain-data-causal-audit.md).
No larger follow-up was run.

## Workload and probe controls

The fixture creates a 120,000-validator Heze state at slot zero with
`SlotsPerRound=8`, saves the genesis data, inserts the state in the real
checkpoint-state cache, and warms the committee cache. Before releasing work,
it calls the real `AttestationTargetState` path and verifies a slot-zero state
with 120,000 active validators. The external client also completes a warm TCP
DomainData call and retains that response as the expected value.

Both arms release 6,144 workers over the same 15,000 finite jobs. Every job
calls the real `AttestationTargetState` path. The scan arm then calls the real
`ActiveValidatorCount` over the returned 120,000-validator state; the memoized
arm uses the scalar verified before release. Each arm drained all 15,000 jobs,
and every computed scan count equaled the memoized count.

The client runs in a separate process on a pre-established TCP gRPC connection.
It waits outside the RPC call for one coordinator byte before each of exactly
32 sequential calls. The parent sends those bytes at fixed completed-job
thresholds `max(1, floor(i*15000/32))`, from 1 through 14,531, and records the
kick time and load counters for every probe. There is no wait inside the server
interceptor or handler.

All 32 server admissions in each arm met the recorded loaded condition: fewer
than 15,000 jobs had completed and at least one checkpoint or scan call was
active. Memoized-arm admissions spanned completed-job counts 1,548–14,583 with
417–6,144 active checkpoint calls. Scan-arm admissions spanned 2–14,532 with
465–6,144 active checkpoint calls; 31 of 32 kick snapshots also observed an
active registry scan. The first scan kick occurred after one completed job and
while 2,009 checkpoint calls were active, just before an active scan was visible.

The memoized workload advanced faster than the external client: its first call
contains the arm's 5.512450 millisecond maximum, and later coordinator bytes
could be buffered while their fixed thresholds were crossed. The admission
records still show active work for every call. The scan workload remained active
for another 1.024316327 seconds after the final client return. These are finite
offered-work controls, not equal-duration load arms.

No artificial sleep, held lock, all-goroutine snapshot, CPU profile, runtime
trace, or packet capture ran during the pair.

## Scope and limitations

The test hosts the ordinary production `rpcvalidator.Server.DomainData` method
in the same process as the checkpoint/count workers and invokes it with the
generated Prysm client stub over real localhost TCP gRPC. The client process is
otherwise idle. This directly tests the missing BN-load-to-gRPC edge with the
actual cheap domain body.

The harness does not instantiate the full production BN RPC `Service` or its
connection interceptor, and it does not pass through the validator client's
domain cache, client manager, or configured retry interceptors. Its only server
interceptor records bounded timestamps and atomic counters. Thus this null
result does not exclude historical delay in those omitted layers or scheduling
effects from the more complex historical process.

Client and server phase joins use `time.Now().UnixNano()` from two same-host
processes. They identify the broad pre-body/body/post-body split but are not
wire timestamps; microsecond-scale phase differences should not be treated as
packet boundaries. The 12-second budget matches the earlier probe envelope and
is a diagnostic bound, not proof that each historical RANDAO invocation began
with 12 seconds remaining. In particular, retained slot-4 evidence shows its
role began after the absolute deadline.

## Command and evidence

The diagnostic-only sources are
`beacon-chain/blockchain/domain_data_diagnostic_export_test.go` and
`beacon-chain/blockchain/domain_data_tcp_diagnostic_test.go`. The primary test
is gated by `PRYSM_DIAGNOSTIC_DOMAIN_ARM=scan|memoized`, and the subprocess test
has a separate child-only gate. No production file changed.

The binary was built with Go 1.26.5:

```text
env GOCACHE=/tmp/prysm-diagnostic-buildcache GOMAXPROCS=4 /home/sukun/dev/go/bin/go test -p=2 -tags=develop -c -o /tmp/prysm-domain-data.test ./beacon-chain/blockchain > /tmp/prysm-domain-data-build.log 2>&1
```

The exact arm invocations are retained in
`domain-data-tcp-go-evidence/command.txt`. The memoized process ran from
`2026-09-06T11:21:26.912575165Z` through
`2026-09-06T11:21:29.187943777Z`; the scan process ran from
`2026-09-06T11:21:29.188716263Z` through
`2026-09-06T11:22:04.122808884Z`. Both exited with status zero under
`GOMAXPROCS=4`.

The original run remains at `/tmp/prysm-domain-data-real-tcp`. The retained
`domain-data-tcp-go-evidence/` directory contains:

- `window.tsv`, `metadata.tsv`, and `command.txt` for process provenance;
- complete parent and external-client Go output for both arms;
- all 32 client, server, joined, and coordinator records for each arm; and
- the exact generated summaries for each arm and `sha256sums.txt` for retained
  artifact integrity.

The two Go sources are `gofmt` and `goimports` clean, a trailing-whitespace
check passed, and the package compiled successfully with the `develop` build
tag. No Bazel test was run because this diagnostic used the requested Go path.
