# Domain-data cache diagnostic

The validator client constructs its domain-data cache with `MaxCost: 192` and
stores each response with an explicit cost of 1. Ristretto v2.2.0 adds its
internal `storeItem` cost unless `IgnoreInternalCost` is true, however. The
production comment that one item has one cost therefore does not describe the
effective configuration.

`validator/client/domain_cache_diagnostic_test.go` is a test-only reproduction.
It inserts 13 representative epoch/domain entries into the exact production
configuration and compares it with an otherwise identical cache that ignores
internal cost. On the dependency version in this tree, the production
configuration retains 3 of 13 entries; the control retains all 13:

```
go test -run '^TestDomainDataCacheInternalCostDiagnostic$' -count=1 -v ./validator/client
retained domains: production=3 ignore-internal-cost=13
```

This makes repeated RANDAO cache misses across early slots consistent with
ordinary cache eviction. It is not by itself proof that a particular missed
proposal was caused by eviction.

The miss can be amplified by the existing synchronization in `domainData`:
all domain keys share one `domainDataLock`, and a cache miss holds its exclusive
lock while the beacon-node `DomainData` RPC is in flight. Consequently one slow
miss can serialize otherwise independent domain requests. This document and
the test do not change either production behavior.
