# Diagnostic validation and scope

The diagnostic work is in jj change `rnwupxrz`, based on `nkxqltrw`. No
production Go file was changed. The Go changes are test files and explicit
Bazel test-source registration. The other changes are extraction scripts,
derived evidence, and reports. Raw archives were read in place; no devnet was
run. Large raw/derived inputs remain on disk outside the new change.

## Tests actually executed

- Ordinary domain RPC under real checkpoint/count load: the Go TCP control
  passed all fixture and response checks, with an external client, 32 calls
  gated before invocation by fixed job-progress thresholds, and 15,000
  completed jobs in each arm. All 32 calls were admitted under load and
  succeeded in both arms. Maximum scan/memoized latency was 5.700/5.512 ms;
  this is a negative reproduction of a domain deadline, not proof that the
  historical RPC was fast. The test has no profiling or tracing and does
  not exercise the VC cache/retry stack. [Commands and all probe records](domain-data-tcp-realwork-results.md).
- Real slot-13 parent dependencies under finite checkpoint/count work: both
  Go primary arms passed and completed 15,000 jobs. Memoized counts prepared
  slot 13 in 35.377 ms. Repeated counts delayed `UpdateHead` for 11.073777 s;
  the next slot-processing call observed the canceled context in 13 us.
  The primary pair had no snapshots, profiles, or runtime tracing. A separate
  traced repetition locates 6.847655 s in the fork-choice writer lock and
  connects its wake to an actual checkpoint reader's `RUnlock`; that repetition
  finished within budget and is not another cancellation result. The invalid
  initial fixture pilot is explicitly excluded. Commands, passing logs, and
  limitations are in [the parent results](slot13-parent-dependency-realwork-results.md).
- Real pool compaction and proposer packing with matching signed votes:
  the Go diagnostic passed exact data, 15,000-validator coverage, BLS
  verification, and pool-preservation assertions. Raw packing took 5.112 s;
  compaction plus packing took 1.624 s total. A 300 ms diagnostic budget
  returned only after 5.139 s on raw input, versus 1.196 ms successful
  compact packing. Historical ledger-key checks corrected fixture assumptions
  before this passing run. The real pool and packer are combined with
  explicitly mocked head/time and deposit dependencies; no profiling or
  injected wait was used. [Command and retained output](packing-compaction-realwork-results.md).
- Real warm sync-index method under finite checkpoint/count work: the large
  Go differential completed 15,000 jobs with 6,144 workers in both arms, with
  snapshots and runtime tracing disabled. Scan maximum: 13.571680 seconds;
  memoized maximum: 11.833 milliseconds. A separate traced pair locates the
  delay at the global manager: one call spends 11.017502 seconds waiting in
  `getChan` before cache access and 4.523097 seconds waiting in deferred `Clean`
  after the hit. Both smaller null trials and the observer-confounded small
  positive remain documented in the [real-work report](sync-index-count-realwork-results.md).
  No 1,000-node rerun or production behavior change was needed.
- Real Engine HTTP client under finite genesis-count load: Go execution-package
  test, 128 workers and 1,024 completed count jobs at four Go execution
  processors. The scan arm reproduced four timeouts in 16 calls; no-load and
  memoized controls each had zero. Runtime tracing directly caught a response
  reader runnable for 342.909 ms and a write-loop cleanup delay of 343.462 ms.
  All three arms passed their assertions. The [full reproduction report](../startup3/payload-scheduler-repro-results.md)
  distinguishes the one fast-response failure from three pre-write failures
  and records the CPU attribution and raw artifacts.
- Gloas cached-payload recovery: the new Go differential passed both cases
  through the real proposer method. A raw deadline performs one fresh FCU
  and a second payload request; the execution timeout sentinel aborts after
  the first request. Command, assertions, and limits are in
  [payload-recovery-reproduction.md](payload-recovery-reproduction.md), with
  [retained Go output](payload-recovery-go-output.txt). This follow-up uses Go
  builds/tests as requested by the user; it does not depend on completing the
  older Bazel-wide precheck below.
- Validator client: all six named diagnostics passed together under the normal
  Bazel validation settings. The verbose output contains each `=== RUN` and
  `--- PASS`, including both domain-cache tests after their explicit BUILD
  registration. The final output is retained in
  [client-diagnostics-output.txt](client-diagnostics-output.txt).
- Build and slot-13 parent state: both named diagnostics passed 10/10 runs,
  with Bazel validation actions disabled because of pre-existing `uintcast`
  findings in `beacon-chain/p2p/encoder/scratch.go:51,54,55,71,75`. These
  results do not constitute passing the normal validation action. The final
  verbose output is retained in [build-diagnostics-output.txt](build-diagnostics-output.txt).
- Goldfish: the supplied-cohort replay passed for both payload-bit values;
  the complete affected Goldfish test target also passed.

The exact commands, controlled dependencies, assertions and limitations are
in [offline-preflight-results.md](offline-preflight-results.md) and
[offline-build-retention-results.md](offline-build-retention-results.md).

## Further hypothesis checks

- Same-pool compaction after a real proposer snapshot: Go compilation and the
  single `TestDiagnosticHistoricalPackingSnapshotOverlap` pilot passed. Real
  compaction completed at +2.268 s and a fresh BLS-verified full pack took
  1.164 ms while the canceled old pack continued until +5.326 s. There was no
  injected delay, competing count workload, or production change. The
  [result and command](packing-snapshot-overlap-realwork-results.md) retain the
  complete output and distinguish this from a historical pool reconstruction.
- All-owner FCU response/BN-marker join: deterministic regeneration matched
  the retained TSV and Python compilation passed. Fifteen slots have an
  unambiguous comparable marker; slot 1 correctly has no comparable marker,
  rather than an inferred stalled response. See the
  [historical timing audit](fcu-bn-payload-delay-audit.md).
- Historical first-count onset: the extractor and its TSV/anchor checks
  passed. It scans three existing observer logs and reuses the owner table;
  no network rerun or full archive rescan was required. The source/startup
  flag and entry/acceptance clocks are separated in the
  [onset audit](ffg-count-onset-audit.md).

## Continued slot-one investigation

- Existing native-trace extraction and independent script validation join all
  101 readers released by a natural validator-storage finalizer to their
  preceding checkpoint-key wait, Count Len/At park, writer wake, first
  execution, and later BLST and batch-verifier waits. The writer wakes them
  in 28.096 microseconds; their first-scheduling delays reach 1.918 seconds.
  The [cohort audit](full-gossip-finalizer-cohort-trace-audit.md) separates
  this observed wave from the different Count goroutines active during the
  later RPC. No new runtime capture was required. The
  [historical source comparison](historical-finalizer-path-audit.md) finds
  no difference in the four production files implementing this path.
  The subsequent [independent summary checks](full-gossip-checkpoint-summary-validation.txt)
  exactly partition a complete 469.648768-ms key wait across 60 admitted
  callers, verify all 98 key-release-to-Len joins, and place all 134 later
  Count callers in the checkpoint queue before the finalizer wave. The
  retained output regenerates byte-for-byte.
- The predeclared matched omission control compiled and passed exactly two
  fresh native arms: timed attestation-data RPC omitted first, enabled second.
  Both retain coherent head/fork setup, the temporary-cache API preflight,
  cold sync, and all 48 absolute-deadline probes. Maximum DomainData times
  were 7.492/10.542 ms and peak iterators six/seven. All 96 calls succeeded.
  The omission assertions verify no measured handler or head-copy record
  and an unpopulated measured cache. This null pair does not discriminate
  the trigger of the earlier seconds-scale pause. The
  [complete result](full-gossip-domain-no-timed-attdata-results.md) retains
  both arms, source identity, distinct generated genesis roots, and commands.
- The coherent full-gossip/attestation-data composition passed its native and
  snapshot Go arms with no profiles or runtime tracing. Native probe 12 took
  6.855839 seconds, including 4.781042 seconds before server admission and a
  3.566-microsecond handler upper bound. Snapshot maximum was 3.298 ms. All
  invoked calls succeeded; 32 native grid ticks were skipped while prior
  calls were pending. A separate traced repetition passed and reached
  850.217 ms. The [writer-composition report](full-gossip-domain-writer-results.md)
  retains the absolute deadlines, source revisions, changed publication
  cadence, exact phase records and excluded incoherent fixture pilots.
- The full-gossip Go diagnostic passed both native-registry and stable-snapshot
  arms with 120,000 valid validators and 15,000 correctly signed published
  votes. It verifies real startup, shared head/checkpoint validator storage,
  valid Heze signatures, and the absolute slot deadline. This older Domain-only
  fixture left prerequisite API fork gates and the internal fork-choice head
  incomplete; the coherent writer composition above corrects both. At 16 Go execution
  threads, maximum DomainData latency was 131.045 versus 3.938 ms, and clean
  invoke-to-admission delay was 68.978 versus 0.795 ms. All 32 calls succeeded
  in each arm. The analysis regenerates its comparison exactly; source, full
  output, phase records and causal limitations are in the
  [full-gossip results](full-gossip-domain-realwork-results.md).
- An unchanged-source repeat at `GOMAXPROCS=4` also passed both full-gossip
  arms. All 64 DomainData calls succeeded; native/snapshot maxima were
  8.952/3.839 ms and clean invoke-to-admission maxima were 8.704/3.601 ms.
  Actual accepted/stored counts were 6,988/10,294 of 15,000 published votes.
  The [four-thread results](full-gossip-domain-p4-results.md) preserve this
  counterevidence to a simple fewer-threads/longer-RPC extrapolation.
- The paced checkpoint/count, cold slot-one sync, and valid singleton BLS
  compositions compiled and passed with Go. All six arms drained 15,000 jobs;
  all 192 ordinary DomainData probes returned the expected value. None
  reproduced a deadline. The largest scan-arm RPC was 12.488 ms. Commands,
  full output and phase records are linked from
  [the composition results](domain-data-paced-composition-results.md).
- The retained H trace identifies the HTTP/2 reader and handler for an actual
  RANDAO RPC. Independent review checked the uninterrupted 135.385 ms runnable
  interval, native wall-clock alignment, handler ancestry, and all 53 CPU
  samples in that interval. Every sample includes the real genesis-count
  path. Neither traced target RPC overlapped a discrete goroutine-profile
  request. Continuous profiling/tracing and the shifted I2 request timing
  remain explicit limitations. See [the trace results](domain-http2-reader-trace-results.md)
  and [independent review](domain-http2-reader-independent-audit.md).
- The node-169 component-timeline extractor now counts physical LF-delimited
  source lines. All 105 event anchors and eight engine request/response
  anchors were checked against the original files. This corrects the new
  extractor's CR-related line-number drift; event timestamps and the paired
  engine JSON did not change.

## Repository precheck

`gofmt -l` and `goimports -l` were clean on all four diagnostic Go files.
No dependency, protobuf or SSZ schema was changed.

The full `bazelisk run //:gazelle -- fix --mode=diff` did not pass. It entered
existing nested source checkouts and reported:

```text
shadow/deps/go-ethereum/accounts/usbwallet/trezor:
  directory contains multiple proto packages
shadow/deps/go-ethereum-bootnode/accounts/usbwallet/trezor:
  directory contains multiple proto packages
github.com/gballet/go-verkle@v0.2.2:
  module declares its path as github.com/ethereum/go-verkle
```

The command was stopped after these errors and repeated dependency-resolution
attempts. It was a diff-only invocation and did not apply generated changes.
The output remains at `/tmp/round2-precheck-gazelle.log`.

The repository [precheck skill](../../../.agents/skills/precheck/SKILL.md)
requires “Stop and report if any step fails.” Accordingly, the subsequent
`make build` smoke check was not run, and the change is not represented as
having passed the complete precheck. Resolving the unrelated nested-checkout
and encoder-validation findings is outside this forensic task.
