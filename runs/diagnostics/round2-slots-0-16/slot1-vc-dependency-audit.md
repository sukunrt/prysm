# Slot 1: VC dependencies before and around RANDAO DomainData

The additional client-side source audit strengthens the case that node 169's
missing time was in RPC servicing or scheduling, rather than a hidden slow
operation inside the RANDAO implementation. The adapter's apparent global
client lock is released **before** the RPC; readiness checks also do not hold
that lock across network work. The ordinary role batch supplies no identified
domain-cache writer other than RANDAO itself after successful preflight.

This narrows the causal search. It does not turn an unlogged RPC admission
time into a measured one. The full-gossip process is materially different from
the newest count-only domain control: that control's 6,144 workers mostly wait
in checkpoint access, with about three active scans at its request admissions.
The original HTTP starvation example instead has many simultaneously active
counts. Offered worker count alone is not the scheduler workload.

## Exact adapter and connection ordering

`grpcValidatorClient.DomainData` is just
`return c.getClient().DomainData(ctx, in)`
(`validator/client/grpc-api/grpc_validator_client.go:202–204`). Go evaluates
`getClient` before invoking the generated method. That getter acquires its
manager mutex, reads the provider connection generation, optionally rebuilds
the lightweight generated stub, and returns with its deferred unlock executed
(`grpc_client_manager.go:41–56`). It does **not** retain the mutex while waiting
for another role's attestation-data or sync RPC.

The provider's `ConnectionCounter` is a mutex-protected integer read. Its
`CurrentConn` can create a connection under the provider mutex, but the manager
requests a new connection only after a generation change. Node 169 logs exactly
one gRPC endpoint, `beacon:4000`, at validator line 11, and its initial connection
is usable roughly 39 minutes before genesis. `SwitchHost` returns without a
generation change for the current index. The fallback loop does not call it at
all when there is only one configured host. This rejects normal endpoint
rotation/lazy dialing as an intrinsic per-DomainData operation. gRPC's internal
transport reconnect is separate and does not require a provider generation
change; the absence of a provider switch log cannot exclude that transport
behavior.

The source at deployed revision
`0280403c70d88967f49d2d4c730f4c5417dabdf5` was read directly for both this manager
and the pre-RANDAO proposer body. Their relevant ordering matches the working
source.

## Other candidate holders

| Dependency | Source result | Historical implication |
| --- | --- | --- |
| Health monitor | `performHealthCheck` calls `EnsureReady` before taking the monitor mutex. The node RPC client also releases its client-manager mutex before `GetHealth`. | A slow health RPC is not a network-duration holder of the proposer/client manager. |
| End-of-slot performance RPC | `LogValidatorGainsAndLosses` returns unless this is an epoch end beyond the first epoch (`metrics.go:230–234`). | The slot-0 completion goroutine cannot start this RPC and interfere with slot 1 through it. |
| Proposer key lock | Only proposer, ordinary attester, and available-attester methods use this package's multilock. The proposer uses role-string and raw-pubkey keys; the others use role-byte-prefixed pubkeys. | There is no same-key slot-0 proposer holder, and an ordinary attester does not own the same logical key. The package-global manager remains a short shared dependency, not a shared address space with the BN's thousands of checkpoint callers. |
| Tracing | Disabled `trace.StartSpan` returns the existing context and a noop span. Enabled startup emits `Starting otel exporter endpoint`; that marker is absent from node 169's full validator log. The configured exporter uses a batch processor without a blocking-full-queue option. | No positive evidence supports a synchronous exporter/network wait before this domain RPC. |
| RPC request logger | `api/grpc.LogRequests` immediately invokes the RPC when logging is below Debug. In Debug mode it logs only after invocation. | The ordinary historical Info/Error log configuration supplies no request-logging wait before RANDAO admission. |
| Local signing/keymanager | `signRandaoReveal` obtains DomainData before creating the signing root or calling `km.Sign`. | The observed `could not get domain data` wrapper precedes local signing; a slow BLS signing operation cannot produce this error at that location. |
| Domain cache | One RWMutex spans a cache-miss RPC and an asynchronous cache insertion; a waiter cannot abandon its mutex wait with its context. | This is a real amplifier, but an actual competing writer must still be identified. A cache hit alone does not return the historical error. |

The role batch's ordinary attesters fail fetching attestation data, before their
signing domains. PTC, sync-message, and contribution errors also precede their
later signing domains. Available attestations use their separate static-domain
path. Slot 0's role completion report means the completed role handlers have
returned. Thus those historical calls cannot simply be assigned as a long
post-preflight domain writer.

Detached subnet-refresh proof work remains a source-supported candidate for a
different domain writer: it uses a background context and current/next-epoch
selection domains. But its refresh began about nine seconds before the first
qualifying slot-1 gossip, and a larger previous full-VC refresh finished its
observed domain-access phase in about half a second. Successful slot-1 sync
selection also had to cross the same cache lock before role dispatch. A long
background holder after that point is possible but requires a specific later
miss/slow RPC; it is not a consequence of merely having 4,768 proof jobs.

The RANDAO return at the slot deadline plus 2.452 milliseconds weakly favors a
caller directly observing its own deadline over waiting for a long background
holder whose release is independent of that deadline. This is timing evidence,
not an exclusion: a holder could release near the boundary or share another
deadline-bound dependency.

## Retry is a residual-budget amplifier, with no identified trigger yet

Production default gRPC options allow five total middleware attempts and a
one-second linear backoff. `grpc_retry` v1.2.2 waits no backoff before attempt
zero, retries `Unavailable` and `ResourceExhausted`, and terminates with a gRPC
deadline error when the parent deadline expires during a later backoff. The
ordinary DomainData handler has no normal branch that returns either retryable
status for valid RANDAO configuration. A transport or admission failure would
be required.

Four backoffs alone cannot explain a fresh twelve-second budget, but that is
not the only relevant budget. The historical PTC success proves role dispatch
by +9.915 seconds. That marker has 2.085 seconds until the shared deadline;
it does not timestamp the RANDAO call. If RANDAO began around then, two
one-second retry delays plus
small RPC intervals could consume almost all its remaining time. The same
final error text is compatible with this route. The archive has no per-attempt
records or preceding retryable status, so this is a concrete conditional
amplifier below the main servicing/scheduling hypothesis, not a discovered
historical retry sequence.

The informative next discriminator is full-gossip scheduling and admission
around the real gRPC service, retaining the number of actively scanning,
signature-verifying, and queued goroutines. Repeating a control with thousands
of offered workers but only a few runnable workers would not test the missing
scheduler condition. This audit made no production edits and ran no additional
experiment.
