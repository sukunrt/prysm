# Round 2 slot-1 cause deep audit

This audit uses node 169's complete recovered archive and exact round-2 revision
`0280403c70d88967f49d2d4c730f4c5417dabdf5`. It separates what the historical
timestamps prove from the remaining choice between VC scheduling/domain-lock delay
and gRPC servicing delay.

## Tight historical ordering

Genesis is `01:30:00`; the proposal context expires at the end of slot 1,
`01:30:24`.

- The initial duty update completes and logs the epoch-0 schedule at
  `01:30:02.886`; node 169 has 596 keys, including the slot-1 proposer
  `0xa7120c370e8c` (`validator.log:648-680`, proposer row `:650`).
- Slot-0 work succeeds at `01:30:09.017-09.019`: 65 attestations, one aggregate,
  three payload attestations, two sync messages, and one sync contribution are all
  signed and submitted (`validator.log:681-685`). This proves that the keymanager,
  gRPC connection, and several epoch-0 signing domains were usable before slot 1;
  it does not prove continued low latency.
- Slot-1 role dispatch has begun by the successful PTC `NotFound` result at
  `01:30:21.915067` (`validator.log:686`). Therefore `RolesAt` returned no later
  than that point. `performRoles` launches each role in a goroutine, so this does
  not timestamp when the proposer goroutine was scheduled.
- At `01:30:24.002452`, the proposer reports only
  `could not get domain data: ... DeadlineExceeded` (`validator.log:694`). It never
  reaches graffiti, head hint, block request, or the EL payload path. The same
  proposer key's attestation-data RPC expires independently at `:766`.
- The rest of node 169's role RPCs collapse at the same deadline: all 75 scheduled
  attesters match attestation-data failures, PTC and sync duties fail before their
  signing domains, and the aggregate calls fail before aggregate-and-proof signing.
  These counts are reproduced by `round2_slot1_audit.py`.

Consequently role dispatch existed by the final 2.09 seconds of the slot, but this
does **not** bound RANDAO's duration: the proposer goroutine could have started near
slot start and waited for almost 12 seconds, or it could have been scheduled much
later. Neither start nor domain-lock acquisition is logged.

## The BN DomainData handler is not state/fork-choice work

Exact-R2 `beacon-chain/rpc/prysm/v1alpha1/validator/server.go:176-203` shows the
ordinary DomainData path reads the request epoch/domain, obtains the fork directly
from static chain configuration, reads the process-global genesis validators root,
and computes the signing domain. Unlike the voluntary-exit special case, RANDAO
does not fetch head state. It takes no fork-choice, checkpoint-state, validator-
registry, or execution lock and has no registry-sized loop.

Thus the historical latency cannot be attributed to expensive code *inside* the
RANDAO DomainData handler. If the RPC reached a running handler, its computation is
constant-sized. The unlogged boundary is before handler scheduling, in transport,
or on the response path.

This source conclusion is also measured in the existing real 120k-validator
reproductions, rather than only in a mock handler. The `bn.domain_data.*` markers
surround the actual gRPC method body. Across 34 completed calls in load B and 98 in
E1, the largest measured handler-body duration was 0.015 ms and 0.014 ms,
respectively. Those runs include the real BN, VC, genesis state, and genesis FFG
load. They eliminate intrinsic DomainData computation and BN application-lock work
as a seconds-scale mechanism once the handler has been admitted; they cannot show
when node 169's historical request was admitted.

The EL was independently healthy enough to service the BN's scheduled payload
prebuild: `engine_forkchoiceUpdatedV4` request 709 at `01:30:12.166999` returns
HTTP 200/VALID with payload ID in about 0.4 ms (`snooper-engine.log:31640-31675`),
and geth builds the empty payload in 717 microseconds (`execution.log:83-84`). That
is not a proposer GetPayload request and cannot explain the RANDAO failure; it does
rule out “node 169's EL was unavailable, so RANDAO failed.”

## A concrete VC-side amplification omitted by the earlier account

The VC domain cache is much smaller in practice than its apparent 192 entries.
Exact-R2 `validator/client/service.go` configures Ristretto with `MaxCost: 192`
and does not set `IgnoreInternalCost`. `domainData` serializes every miss through
one global `RWMutex`, holds the write lock across the BN RPC, calls asynchronous
`cache.Set`, and ignores the return value.

The focused real-configuration diagnostic was run Go-only as requested:

```text
GOMAXPROCS=4 GOCACHE=/tmp/prysm-diagnostic-buildcache \
  go test -tags=develop ./validator/client \
  -run '^TestDomainDataCacheInternalCostDiagnostic$' -count=1 -v

retained domains: production=3 ignore-internal-cost=13
PASS
```

Ristretto's internal item cost leaves only three of 13 inserted domain entries in
this exact cache configuration. This is directly relevant, not a synthetic blocked
RPC: node 169's startup working set exceeds three epoch/domain keys. Slot-0 success
requires multiple attester, aggregate, sync, contribution, and payload-attestation
domains. The detached subnet subscription additionally signs selection proofs for
current and next-epoch duties. RANDAO epoch 0 itself is cold because slot 0 proposals
are explicitly skipped.

`UpdateDuties` launches that subscription with `context.Background`; it clears the
selection-proof map, expands current and next duties over repeated attesting slots,
and runs 16 signing workers. For this 596-key client the source expansion can yield
thousands of selection-proof jobs. Cache eviction or an asynchronous `Set` not yet
visible can turn another domain lookup into a miss; each miss is serialized and a
slow BN response holds the global domain lock. A canceled waiter cannot leave that
mutex wait until its holder releases, as independently established by
`TestDomainDataCanceledWaiterDiagnostic`.

This provides a concrete coupling from startup fan-out to RANDAO latency:

```text
small effective domain cache + detached selection signing
    -> repeated serialized DomainData misses
    -> any delayed BN/gRPC service holds the VC's global domain lock
    -> a cold RANDAO lookup can start late or wait behind that miss
```

It supersedes the weaker assumption that successful slot-0 signing means all
relevant domains remained cached. Conversely, slot-0's successful aggregate proves
at least one selection-domain lookup completed; it does not prove the detached
current+next subscription finished or that its cache entry survived.

## What the real loaded pipeline does and does not reproduce

The reproduced genesis FFG workload gives a concrete source of system-wide service
delay: repeated slot-zero `ActiveValidatorCount` scans consume CPU, create thousands
of validation goroutines, and delay otherwise cheap BN RPC/transport goroutines.
The phase markers also keep the cache-lock hypothesis honest. In loaded E1 the
actual VC RANDAO (`02000000`) path had zero measured write-lock wait and RPC times of
0.340 ms at slot 1, 8.528 ms at slot 2, and 3.505 ms at slot 3. In load B they were
0.465, 6.073, and 3.932 ms. No instrumented RANDAO call reproduced node 169's
deadline. The busiest startup domain did create short lock convoys (for example,
E1 attester-domain write-lock waits had a 17.455 ms median and 27.898 ms maximum;
load B had a 31.367 ms median and 73.586 ms maximum), but not a seconds-long holder.

An independent E1 DomainData probe did exhibit a 4,271.995 ms client envelope in
slot 3 even though the measured server body remained below 0.015 ms. That is direct
evidence that this real loaded process can delay a constant-time DomainData call
outside the handler body. It is **not** a reproduction of the historical slot-1
RANDAO failure: the real proposer succeeded, its slot-1 RANDAO RPC was 0.340 ms,
and the outlier probe did not use the VC domain-cache mutex. Accordingly it supports
transport/admission/response scheduling as a viable loaded mechanism, but does not
choose it over late proposer scheduling or a historical VC mutex wait.

Historical node 169 nevertheless lacks the decisive trace markers:

- no log records RANDAO goroutine start or domain-lock acquisition;
- no server log records DomainData handler admission/completion;
- no packet/runtime trace separates VC runnable delay, mutex waiting, HTTP/2/gRPC
  delivery, BN handler scheduling, and response-reader scheduling.

Therefore the strongest supportable causal statement is: genesis FFG scan pressure
degraded node servicing while node 169 performed a large startup role/subscription
fan-out; its cold RANDAO lookup was exposed to a three-entry effective domain cache
and globally serialized miss path, and failed before reaching any proposal/EL work.
The exact historical split between late proposer scheduling, waiting behind another
domain miss, and scheduling/transport of its own constant-time BN RPC is not logged.
Calling the terminal deadline itself the underlying cause, or claiming a particular
mutex holder persisted for 12 seconds, would exceed the evidence.
