# Genesis `UpdateHead` diagnostics

Date: 2026-09-05. Host: AMD Ryzen 7 7840U, Linux amd64,
`GOMAXPROCS=4`, `-tags develop`.

## Fixture

`update_head_genesis_diagnostic_test.go` builds a minimal Heze genesis state
with 120,000 active validators and saves genesis through the real blockchain
test service. It then SSZ-serializes and decodes a fresh state, replaces state
generation with a cache containing that cold decoded instance, and uses it as
head state. This mirrors saved-state startup more closely than retaining the
state warmed by `SaveGenesisData`. The
fork-choice genesis node is full. Before each timed call, the service's current
head is reset either to full (no status change) or empty (a real empty-to-full
status flip when `UpdateHead` adopts fork choice). The assertion after every
call verifies that the resulting service head is full.

The selected proposer for each slot is registered in the subscribed-validator
and default-preference caches. A preflight assertion verifies payload
attributes are non-empty, so timed flips execute proposer selection,
`ProcessSlots`, and Gloas payload-withdrawal computation under the fork-choice
lock. The registry uses zero public keys but proposer indices are computed from
the real 120,000-entry active registry.

The service clock is moved two slots after genesis, and the small-pool case
inserts 50 committee-length Electra fork-choice attestations. A direct
`OnAttestation` preflight must succeed, proving the fixture reaches
`getAttPreState`, committee/index validation, and fork-choice vote processing;
each timed call also asserts the pool is drained. The votes intentionally reuse
the same seat and root, so this is a bounded valid-vote processing case rather
than a large diverse-weight scenario. Execution FCU is asynchronous and uses the test mock. There are no
artificially blocked feed subscribers.

## Command

```sh
GOMAXPROCS=4 /home/sukun/dev/go/bin/go test -tags develop \
  ./beacon-chain/blockchain \
  -run '^TestDiagnosticGenesis120KUpdateHead$' \
  -count=1 -timeout=60s -v
```

## Results

Ranges are three samples per case.

| Parent | Proposing slots | Pool | Already full | Empty-to-full flip |
|---|---:|---:|---:|---:|
| Cold decoded | 1-3 | 0 | 0.78-2.47 ms | 30.1-41.2 ms |
| Cold decoded | 1-3 | 50 | 1.08-1.99 ms | 31.8-37.9 ms |
| Root-warmed | 1-3 | 0 | 0.95-1.81 ms | 3.04-4.01 ms |
| Root-warmed | 1-3 | 50 | 1.21-2.12 ms | 3.13-3.56 ms |

The already-full case returns after `FullHead` and `isNewHead`; the flip case
also loads the cached state/block, checks `proposingAt`, evaluates non-empty payload
attributes, saves the head, emits through the normal non-blocked notifier, and
prunes the pool. With a cold decoded parent, every flip costs roughly 30 ms;
samples two and three remain equally cold even after the first same-target
request. This reproduces the exact-target skip-cache behavior: processing a
copy does not initialize the retained parent, and the cached state at exactly
the requested slot is copied then ignored. Explicitly hashing the retained
parent reduces the full flip to roughly 3-4 ms. Fifty valid same-seat
attestations add bounded vote-processing work without changing the conclusion.

This bounds an uncontended tracked-proposer `UpdateHead` status flip at about
30-41 ms for a cold 120,000-validator decoded state and about 3-4 ms once its root
is initialized. Processing 50 valid same-seat votes adds roughly 0.2-1.0 ms;
the now-current fork-choice clock also makes even the already-full path spend
about 1 ms processing votes/state rather than tens of microseconds. It does not
rule out amplification from many independent cold
copies, a large/valid fork-choice attestation pool, a competing
fork-choice tree, execution-client latency in the asynchronous FCU, or blocked
synchronous subscribers/lock waiters in the real run.
