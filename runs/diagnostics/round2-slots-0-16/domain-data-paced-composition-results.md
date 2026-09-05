# Paced checkpoint, cold sync, and singleton-aggregate DomainData controls

## Result

None of the three bounded compositions reproduced a seconds-scale `DomainData`
delay. The three scan workloads did create thousands of in-flight checkpoint
callers, but actual count and singleton-aggregate concurrency remained small.
Every external Domain call returned the correct warmed 32-byte response.

| Composition / arm | Work duration | Peak checkpoint / count / aggregate | Domain OK / deadline | Client p50 / p95 / max |
| --- | ---: | ---: | ---: | ---: |
| Paced memoized | 2.002703 s | 13 / 0 / n/a | 32 / 0 | 0.454 / 0.626 / 0.728 ms |
| Paced scan | 33.913716 s | 6,143 / 58 / n/a | 32 / 0 | 3.966 / 11.343 / 12.488 ms |
| Cold-sync memoized | 2.002362 s | 11 / 0 / n/a | 32 / 0 | 0.350 / 0.528 / 0.683 ms |
| Cold-sync scan | 40.111073 s | 6,144 / 5 / n/a | 32 / 0 | 2.446 / 9.724 / 9.727 ms |
| Singleton memoized | 2.002942 s | 9 / 0 / 12 | 32 / 0 | 0.556 / 1.138 / 2.144 ms |
| Singleton scan | 38.329647 s | 6,144 / 10 / 4 | 32 / 0 | 1.744 / 7.597 / 7.712 ms |

The paced scan's largest 12.487500 ms call spent at most 11.929011 ms from
client invocation to server-interceptor admission; the real Domain body took
at most 22 microseconds. This is evidence of modest scheduler/transport delay,
but remains about three orders of magnitude below the 12-second diagnostic
budget.

## Paced admission discriminator

The retained FFG source plans vote `i` at
`firstDue + i*spread/len(attestations)` and was launched with 15,000 votes over
two seconds. The source itself uses an unbuffered channel and 64 submit workers.
This control instead keeps the requested 6,144 load workers and a 15,000-entry
pending queue. The queue preserves every intended offer even when all workers
are occupied. Separate 100 ms records distinguish planned offers, actual
enqueues, and worker admission.

Both arms offered and completed all 15,000 jobs. The memoized arm offered the
last job at +2,000.368 ms with 1.723 ms maximum schedule lateness; 14,995 jobs
were admitted by +2 seconds. The scan arm offered the last job at +2,001.405 ms
with 8.702 ms maximum lateness, but admitted only 7,075 by +2 seconds. Its
schedule-to-worker-admission tail reached 17.152272 seconds.

The scan arm therefore formed a 6,143-caller checkpoint convoy while only 58
calls were simultaneously inside `ActiveValidatorCount`. Delayed offers did
not create a large concurrent count-entry cohort. All 32 scan RPCs were
admitted under recorded load and succeeded. Only five memoized RPC admissions
overlapped the very short memoized work, so that arm is primarily a transport
and response-integrity control.

## Cold slot-one sync prerequisite

The next pair kept paced admission and, immediately after the first completed
job, called the real `Service.HeadSyncCommitteeIndices(ctx, 0, 1)` without a
slot-one sync-head-state cache entry. Validator zero and current committee
position zero had a unique marker key before native-state construction, so a
successful call had to return one meaningful position. The call and the first
external Domain probe shared the absolute deadline
`workReleasedAt + 12 seconds`.

| Arm | Cold sync duration / positions | Budget after sync | Paired Domain duration | Budget after Domain |
| --- | ---: | ---: | ---: | ---: |
| Memoized | 0.146956 ms / 1 | 11.999130 s | 0.683444 ms | 11.998436 s |
| Scan | 0.151424 ms / 1 | 11.991380 s | 3.243582 ms | 11.988078 s |

The scan call began with seven checkpoint and three count calls active; it
returned with ten checkpoint and four count calls active. This exercised the
real cold `HeadState` copy, slot processing, cache fill, and sync-position
lookup, but it did not spend material deadline budget. The paired Domain call
was admitted after 3.153423 ms and its real handler body took 3.076 microseconds.
This pair did not reach the distinct “preflight already expired” outcome.

## Singleton BLS composition and startup clock

The final pair replaces the marker key with the public key for deterministic
valid scalar one before state and cache construction. Fixture setup warms
`AggregateKeyFromIndices([]uint64{0})` and verifies that its marshaled result
equals the singleton input. Every load job then performs the actual sequence
checkpoint retrieval, optional real active-validator scan, and
`AggregateKeyFromIndices([]uint64{0})`; completion is counted only after the
aggregate returns and matches.

All 15,000 aggregate entries returned in both arms. Peak aggregate parents were
12 in the memoized arm and four in the scan arm. Under scan load, the cold sync
prerequisite grew to 162.244450 ms while checkpoint callers increased from 15
to 1,214 and 67 additional count/aggregate jobs completed. It still returned
one position with 11.828619 seconds left. The immediately following shared-
deadline Domain call completed in 4.421989 ms with 11.824111 seconds left.

The external child owned 32 planned startup samples at 250 ms intervals. It
used one persistent TCP connection and sequential calls. The first invocation
followed the cold sync and was 171.467 ms after its planned time. Later scan
calls began within 1.135 ms of their plan; no elapsed sample was skipped and no
catch-up burst occurred. All 32 scan calls were admitted while work was active,
all succeeded, and the maximum was 7.711896 ms. The memoized work ended after
about two seconds, so only four of its 32 clock samples overlapped active work.

This stage composes the actual singleton public-key aggregation used by the
single-attester signature path, but it does not reproduce the full gossip
pipeline or the batch verifier's queue/result-channel wakeups.

## Scope

All pairs use one 120,000-validator Heze state, 15,000 finite jobs, 6,144
workers, `GOMAXPROCS=4`, the actual checkpoint lookup and active-validator
iterator, and the ordinary production validator `DomainData` method over a
pre-established localhost TCP gRPC connection to a separate client process.
Every job drained. Count and aggregate entry/exit counters matched exactly.
No artificial delay, held application lock, observer, all-goroutine dump,
profile, or runtime trace ran in these primary pairs.

The service is still a focused diagnostic fixture. It omits real pubsub decode,
DB/seen checks, fork-choice consistency, committee conversion, signature-batch
verification, operation feeds and pool storage. The gRPC server also omits the
full production interceptor stack. These null results show that the tested
compositions did not reproduce the missing delay. They do not exclude other
interleavings of these operations or their interaction with full startup gossip.

The initial paced pair used the earlier synthetic registry with duplicate zero
public keys. The cold-sync pair used a unique byte marker sufficient for the
position lookup. The singleton pair used a valid curve key and verified the
actual aggregate result. Comparisons are paired within each fixture revision,
not treated as exact performance comparisons across the three runs.

## Evidence

The gated diagnostic source is
[`domain_data_tcp_diagnostic_test.go`](../../../beacon-chain/blockchain/domain_data_tcp_diagnostic_test.go),
with fixture construction in
[`domain_data_diagnostic_export_test.go`](../../../beacon-chain/blockchain/domain_data_diagnostic_export_test.go).
No production source changed.

- [`domain-data-tcp-paced-go-evidence`](domain-data-tcp-paced-go-evidence/)
  retains the paced pair, including 20 offer bins and all worker-admission bins.
- [`domain-data-cold-sync-go-evidence`](domain-data-cold-sync-go-evidence/)
  retains the cold sync records and paired absolute-deadline Domain records.
- [`domain-data-singleton-go-evidence`](domain-data-singleton-go-evidence/)
  retains the valid-key singleton pair and all planned/actual startup samples.

Each directory includes the exact command, compile log, complete parent and
child test logs, raw JSONL phase records, and generated summary. The package
compiled with the `develop` tag using Go; no Bazel command was run.
