# Cached payload timeout identity: executed Gloas reproduction

The real Gloas `getLocalPayloadFromEngine` path changes behavior according to
whether it can recognize the original deadline error. This was executed with
Go after the user authorized Go builds/tests in preference to Bazel.

The new test is
`beacon-chain/rpc/prysm/v1alpha1/validator/proposer_execution_payload_timeout_identity_test.go`.
It supplies a cached payload ID, a Gloas state, a live parent context, and an
engine fixture which fails its first payload call and succeeds on its second.
The two cases differ only in that first error:

| First error | GetPayload calls | ForkchoiceUpdated calls | Result |
| --- | ---: | ---: | --- |
| `context.DeadlineExceeded` | 2 | 1 | Fresh local payload preparation and retrieval succeed. |
| `execution.ErrHTTPTimeout` | 1 | 0 | Immediate `could not get cached payload from execution client: timeout from http.Client`. |

The second case asserts that the returned error matches `ErrHTTPTimeout` but
does not match `context.DeadlineExceeded`. Both cases pass. The raw deadline
case really reaches a fresh FCU and second engine request; unlike the older
Bellatrix timeout test, it does not merely return an empty pre-transition
payload.

Executed command:

```bash
env GOCACHE=/tmp/prysm-diagnostic-buildcache GOMAXPROCS=4 \
  /home/sukun/dev/go/bin/go test -p 2 -tags=develop,minimal \
  ./beacon-chain/rpc/prysm/v1alpha1/validator \
  -run '^TestGetLocalPayloadFromEngine_CachedTimeoutErrorIdentityControlsFallback$' \
  -count=1 -v
```

Result: **PASS**, both named subtests; Go reported 0.018 seconds. The retained
output is [payload-recovery-go-output.txt](payload-recovery-go-output.txt).

This test isolates the proposer's error contract with a controlled engine
fixture. The real execution layer's conversion to `ErrHTTPTimeout` is at
`execution/jsonrpc_error.go:35–40`; the separate real-HTTP reproduction
captures that conversion around `execution.Service.GetPayload` itself. The
test does not establish that a second request would have saved any particular
historical slot under continuing CPU pressure.
