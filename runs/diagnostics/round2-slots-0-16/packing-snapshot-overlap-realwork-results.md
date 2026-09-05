# Real compaction after a proposer pool snapshot

## Result

The one bounded run reproduced the stale-snapshot mechanism. The old proposer
copied all **15,000** real signed singles at +29.019 ms. Real compaction then
changed the same live pool to **0 raw / 6 aggregated** while the old proposer
was still active. A fresh full pack against that compacted live pool completed
in **1.163826 ms**, produced one aggregate covering all 15,000 validators, and
passed batch BLS verification. The old proposer remained active throughout the
fresh pack and returned bare `context.Canceled` only at +5.325934 s.

The cancellation timer, modeling node 35's 1.662-second residual slot-10
budget, actually fired at +1.662235 s. The old pack therefore continued its
private snapshot work for **3.663698 s after cancellation**. Compaction itself
completed at +2.267942 s, after the modeled budget, so this pilot does not show
that a fresh proposal could have completed inside node 35's remaining 1.662
seconds. It shows the narrower causal fact: improving the live pool does not
rewrite or rescue a packer that has already copied the raw pool, while later
work can use the improved view and finish before the old work returns.

| Boundary, relative to old-pack start | Observed state |
|---|---|
| +29.019493 ms | Old getter returned a real cloned snapshot of 15,000 singles; live pool 15,000 raw / 0 aggregated; old active |
| +29.185986 ms | Real same-pool background compaction started |
| +1.662235132 s | `time.AfterFunc` callback canceled the old pack context |
| +2.267941639 s | Compaction ended after 2.238755653 s; live pool 0 raw / 6 aggregated; old active |
| +2.267989909 s | Fresh full `packDepositsAndAttestations` started; old active |
| +2.269153735 s | Fresh pack ended after 1.163826 ms; one output, coverage 15,000, valid BLS; old active |
| +5.325933580 s | Old full pack returned 0 outputs and bare `context.Canceled` |

## Control construction

The env-gated test is
`TestDiagnosticHistoricalPackingSnapshotOverlap` in
`beacon-chain/rpc/prysm/v1alpha1/validator/packing_compaction_diagnostic_test.go`.
It reuses the corrected historical-shape fixture: a 120,000-validator Heze
state, six 2,500-validator committees, and 15,000 independently signed,
disjoint one-bit attestations split across six real data/committee groups.

A test-only pool wrapper embeds the production pool and overrides only
`UnaggregatedAttestations`. It first calls the underlying getter, allowing the
production code to take its read lock, check seen bits, and clone every returned
attestation. Only after that call returns and releases its read lock does the
wrapper store the copied count and close a channel. Closing the channel neither
blocks nor holds a pool lock. The background goroutine starts the real
`AggregateUnaggregatedAttestations` call only after receiving this event.

The old and fresh operations both call the real
`packDepositsAndAttestations`; separate copy-on-write state handles avoid
sharing mutable state internals. The fresh operation uses `context.Background`
so its output can be checked independently. Its one aggregate has the same
attestation data as the fixture, exactly the expected 15,000-validator
coverage, and a valid aggregate signature. The old operation uses
`context.WithCancel` plus a 1.662-second `time.AfterFunc`, matching the
historical outer cancellation category rather than manufacturing a deadline
error identity.

No sleep, held lock, mock delay, artificial CPU load, profile, stack dump, or
production-code hook is present. No historical network run was repeated. The
test records completion order rather than asserting that compaction or the
fresh pack must beat the old pack; the structural pool, coverage, signature,
and cancellation results are asserted. This run happened to retain the old
pack through both boundaries, providing the intended positive observation.

## Interpretation and limits

This result closes one mechanical gap in the leading packing hypothesis. The
proposer's raw slice is independent of the pool after the real getter returns.
Production compaction can delete the live singles and install six aggregates,
yet the old proposal still spends seconds processing its copied singles and
observes cancellation only at a later explicit context check. A new pack reads
the six aggregates and finishes while the old computation remains outstanding.

The control does not recover the historical pools of node 35 or node 85. It
does not prove either node had 15,000 singles, that compaction overlapped its
proposal, or that all observed historical wall time was active attestation
CPU. The synthetic state uses the historical registry size and coherent signed
committee assignments, while the fixture's pool shape is a controlled worst
case rather than recovered SSZ. The single timing sample is not a distribution;
the concurrent compaction duration may include competition with the old pack.

The result also does not make compaction an instantaneous global recovery
switch. In this run it needed 2.239 seconds and finished after the 1.662-second
modeled residual budget. Its value is the demonstrated divergence between old
request state and current pool state, which explains how a late old proposal
can coexist with a much faster fresh pack.

## Command and retained output

Compilation used the Go toolchain, not Bazel:

```text
env GOCACHE=/tmp/prysm-diagnostic-buildcache GOMAXPROCS=4 /home/sukun/dev/go/bin/go test -p=2 -tags=develop -c -o /tmp/prysm-packing-overlap.test ./beacon-chain/rpc/prysm/v1alpha1/validator
```

The only timed pilot was:

```text
env GOMAXPROCS=4 PRYSM_DIAGNOSTIC_PACKING_SNAPSHOT_OVERLAP=1 /tmp/prysm-packing-overlap.test -test.run '^TestDiagnosticHistoricalPackingSnapshotOverlap$' -test.v -test.timeout=5m
```

Complete output is retained in `packing-overlap-pilot.log`; the successful
compile's empty output is retained in `packing-overlap-compile.log`.
