# Round 2 beacon logging deployment audit

## Retained deployment evidence

The retained Round 2 material does not contain the historical Kurtosis service
specification, beacon command line, or container data directory. Each of the 12
archives under `runs/round2` contains only `beacon.log`, `execution.log`,
`validator.log`, `xatu-sentry.log`, and `snooper-engine.log`. Consequently, the
retained Round 2 files do not establish whether the beacon command supplied
`--disable-ephemeral-log-file` or `--log-file`. The reconstructed startup-10
service specs are from a later experiment and are not used as Round 2
configuration evidence.

All 12 retained beacon startup logs do establish the historical data directory.
For example, node 1 reports `databasePath=/data/beaconchaindata` at
`runs/round2/prysm-geth-1/beacon.log:9`; the other 11 retained nodes report the
same path. The deployed source defines `--disable-ephemeral-log-file` with a
default of false (`cmd/beacon-chain/flags/base.go:361-365`). If that default was
not overridden, `before` calls `ConfigureEphemeralLogFile` with the data
directory (`cmd/beacon-chain/main.go:258-262`), yielding
`/data/logs/beacon-chain.log` (`io/logs/logutil.go:113-115`). This path is a
conditional source-derived path, not proof that the Round 2 file existed.

The hook's initialization record cannot settle the question from `beacon.log`.
`Ephemeral log file initialized` is emitted at Debug after the hook is installed
(`io/logs/logutil.go:135-143`). With the normal text configuration, the user
hook writes only the configured user levels to stderr
(`cmd/beacon-chain/main.go:199-216`), while the ephemeral hook accepts through
Debug. The retained 12 beacon captures contain INFO/WARN/ERROR records but no
DEBUG records, as expected for an info-level stderr capture whether or not the
separate debug-file hook existed.

An adjacent Round 1 node records `Failed to fire hook: chown
/data/logs/beacon-chain.log: operation not permitted` at
`runs/round1/prysm-geth-1/beacon.log:766670`. This directly proves that the
ephemeral hook and that path were used in Round 1. It is not Round 2 enablement
evidence. No corresponding hook, chown, rotation, or file-write error occurs in
the 12 retained Round 2 beacon captures.

## Exact source behavior if enabled

The ephemeral setup raises the global logrus level to Debug
(`io/logs/logutil.go:35-43`). It installs a synchronous `WriterHook`; each
accepted entry is formatted and passed to the writer before the logging call
returns (`io/logs/hook.go:25-36`). The backing lumberjack configuration is
concrete: 250 MB maximum size, one backup, and one-day maximum age
(`io/logs/logutil.go:127-139`). These settings establish a possible deployed
output dependency only if the unretained command left the default enabled.
They do not establish that Round 2 hit a rotation, waited on the file, or
encountered an I/O error.

The outer RFC3339 timestamps in the retained `beacon.log` establish that stderr
was captured by the experiment's logging layer. No retained Round 2 service spec
describes that pipe's buffering or backpressure, so its wait behavior cannot be
reconstructed. The archives also omit the in-container `/data` tree and any
rotated backup.

## Block-construction logging omitted by the benchmark

Successful Gloas construction always reaches
`computePostBlockStateAndRoot`, whose final step emits `Computed state root` at
Debug (`beacon-chain/rpc/prysm/v1alpha1/validator/proposer.go:637-649`). Other
Debug calls in the construction code are conditional:

- `Payload ID cache miss` and `Received execution payload from local engine`
  occur on the cache-miss preparation path
  (`proposer_execution_payload.go:76-107,201-215`). For historical slot 97 the
  snooper records `engine_getPayloadV6` at
  `runs/round2/prysm-geth-1/snooper-engine.log:55437-55480`, with no
  `forkchoiceUpdated` in that build interval, which identifies the cached-ID
  path; those two Debug calls were not reached.
- `Builder bids: [...]` requires at least one evaluated Builder API result
  (`proposer_bid.go:156-193`). The retained INFO record establishes that the
  self-build bid won, but it does not expose whether losing builder results were
  evaluated.
- `No pending deposits for inclusion in block` requires the legacy-deposit path
  to reach an empty pending-container result (`proposer_deposits.go:96-112`).
  The retained warning at node 1 line 1121952 identifies the Eth1-data branch,
  but does not prove this later condition.
- The Debug records for proposer-reward and MaxCover failures are error
  fallbacks (`proposer_attestations.go:220-227,287-305`); no matching proposal
  error is retained for slot 97.

The paced full-build diagnostic explicitly sets the global logger to Info and
uses an immediate in-memory counting writer
(`beacon-chain/sync/historical_paced_full_build_diagnostic_test.go:103-135`). It
therefore exercises production INFO vote-ledger formatting and hook dispatch,
but it neither generates the `Computed state root` Debug entry nor exercises
physical stderr, lumberjack file writes, rotation, or a downstream log-stream
consumer. The unloaded full-build diagnostic likewise has no deployed file
hook.

The historical completion marker remains a useful bound: node 1 reports
`sinceSlotStartTime=3.662413031s`, and its outer capture timestamp is only about
0.21 ms later (`runs/round2/prysm-geth-1/beacon.log:1136845`). This excludes the
final `Finished building block` emission itself as the missing seconds. It does
not time an earlier synchronous log write inside construction.

## Conclusion

The old source makes one Debug file write unconditionally reachable on every
successful block construction, plus conditional writes on several subpaths,
and the benchmark omits the physical sinks. The retained Round 2 command and
debug file are absent, so the audit cannot establish that the default debug
file was enabled, that rotation occurred, or that logging delayed slot 97.
Logging remains an unmeasured deployment boundary rather than a demonstrated
historical cause.
