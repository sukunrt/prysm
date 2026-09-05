# Full gossip / DomainData real-work discriminator

## Question and result

This diagnostic asks whether the registry access used by slot-zero
`ActiveValidatorCount` can impair unrelated beacon-node RPC servicing when it
is embedded in the real FFG gossip pipeline. It compares the production native
validator iterator with a test-only stable snapshot iterator over the exact
same 120,000 immutable validator records. Both arms retain the full O(120,000)
active-count traversal and every other production dependency.

The result is positive for the shared registry-access hypothesis. The native
arm let 6,877 of 15,000 successfully published messages reach the validation
wrapper, took 22.501 seconds to settle, and raised DomainData latency to 131.045
ms. The stable-snapshot arm validated and stored all 15,000, settled in 6.202
seconds, and kept DomainData below 3.939 ms. The native arm therefore shows a
real cross-method servicing delay and a large full-gossip throughput loss from
shared registry access. It did **not** reproduce a seconds-long DomainData
deadline: all 32 probes succeeded, including the first slot-one probe.

## Source-faithful fixture

The env-gated test uses one real `blockchain.Service` built through public
constructors and a real DB, doubly-linked-tree fork choice, state generator,
clock synchronizer, attestation service and legacy pool. `SaveGenesisData` and
the normal zero-checkpoint genesis resume branch create the real full,
non-optimistic Gloas genesis node. Before load, the test verifies:

- head slot zero, the actual genesis block root, `InForkchoice`, `HasFullNode`, and the
  round-zero target root;
- the warmed `AttestationTargetState(cp0)` and `HeadState` share the same
  nonzero native `validatorsMultiValue` pointer;
- all 120,000 active validators have deterministic valid BLS keys;
- the current sync committee contains 512 valid distinct members and validator
  zero has one position;
- `MaxCommitteesPerSlot=64` produces six slot-one committees of 2,500;
- the state fork comes from the active Heze config, and its attester signing
  domain equals the config-derived domain for the actual validator root;
- the snapshot arm captures stable per-index wrappers with
  `ValidatorAtIndexReadOnly`, avoiding the native iterator's reused wrapper,
  and has the same 120,000 active count as the native state.

The sender and receiver are persistent real libp2p GossipSub peers. Their
options use Prysm's unsigned strict policy, message-ID function with the actual
genesis validator root, queue size 1,000, max message size, and production mesh
parameters (D=8, Dlo=6, Dhi=12, 700 ms heartbeat, history 6/3). The sync service
uses six exact digest/subnet topics, the production subscription buffer, real
decode and validation, batch verifier limit 1,000 with its 5 ms flush, operation
feed, subscriber, and legacy pool.

All 15,000 slot-one Gloas `SingleAttestation` messages use `data.index=1`, six
committee IDs, distinct valid committee signers, and the actual genesis block
root for both block and target. The sender schedules every publication across
two seconds and retains every offer; both arms report 15,000 successful
`Publish` calls and no publish errors.

This helper uses eight slots per round and the default 64 total attestation
subnets. The retained E1/H configs instead used four slots per round and six
total subnets: they had 12 committees, and the source selected its 15,000
votes across those 12 committees. Thus the one-slot fixture and E1/H share
the real genesis-count path and finite offered volume, not identical
committee assignments or the same complete network configuration. The
[retained-config audit](ffg-receiver-acceptance-audit.md) records that distinction.

Key generation and registry hashing finish before the genesis clock starts.
The chain, signatures, topics and peers finish before slot one, and the fixture
then waits for the real slot-one boundary. The external DomainData process
warms one persistent TCP/gRPC connection before work release. The first
checkpoint iterator triggers a real cold
`HeadSyncCommitteeIndices(ctx, validator=0, slot=1)` followed immediately by
the first DomainData call. Both share the absolute slot-two deadline at
genesis+24 seconds; connection setup consumes that budget. The remaining 31
calls use fresh 12-second deadlines on a sequential 250 ms startup clock and
skip elapsed ticks instead of catching up in a burst.

No work stage contains an injected delay or held lock. There was no stack dump,
profile, or runtime trace in either primary arm. Timers only implement the
source-derived publication cadence, actual slot boundary, probe schedule, and
bounded completion wait.

## Measurements

Both runs used `runtime.NumCPU=16` and `GOMAXPROCS=16`.

| Measurement | Native shared MVS | Stable snapshot |
|---|---:|---:|
| Published / publish errors | 15,000 / 0 | 15,000 / 0 |
| Entered validation | 6,877 | 15,000 |
| Accepted / subscriber / pool / feed | 6,877 each | 15,000 each |
| Ignored / rejected / subscriber failed | 0 / 0 / 0 | 0 / 0 / 0 |
| Peak active validator iterators | 378 | 24 |
| Release to settled | 22.501 s | 6.202 s |
| Maximum publication lateness | 126.591 ms | 3.728 ms |
| Cold sync-index duration | 0.121 ms | 0.458 ms |
| Slot budget at work release | 11.969 s | 11.970 s |
| DomainData calls OK / skipped | 32 / 0 | 32 / 0 |
| First DomainData | 0.223 ms | 0.444 ms |
| DomainData p50 / p95 / max | 1.881 / 63.747 / 131.045 ms | 0.586 / 0.984 / 3.938 ms |
| Invoke-to-server-admission max | 68.978 ms | 0.795 ms |
| Admission-to-handler-return upper bound | 0.082 ms | 0.023 ms |
| Server-return-to-client max | 62.063 ms | 3.561 ms |

Percentiles use the deterministic lower indexed sample selected by
[`analyze_full_gossip_domain.py`](analyze_full_gossip_domain.py); its complete
output is [`full-gossip-domain-comparison.tsv`](full-gossip-domain-comparison.tsv).

The native delays lie outside the cheap DomainData handler body. Both sides of
the RPC boundary were affected: invoke-to-admission reached 68.978 ms and
the recorded server-return-to-client tail reached 62.063 ms. That tail also
includes the interceptor's post-return stats snapshot and record append, so it
does not isolate pure gRPC transport scheduling. The recorded
admission-to-return span remained below 83 microseconds; this is a conservative
upper bound on the handler because it also includes the interceptor's
lightweight atomic/pool-count snapshot before invoking it. The unaffected
invoke-to-admission measurement is direct cross-method evidence that the full
FFG load can delay general BN/gRPC servicing even when DomainData does no
expensive state work.

At the first iterator trigger, the cold sync call still had 11.968 seconds and
returned in 0.121 ms in the native arm; the immediate DomainData call returned
in 0.223 ms. Thus this run does not reproduce node169's missing slot-one RANDAO
response. It establishes a real contributing scheduler/registry mechanism and
its magnitude in this one-BN composition.

## Interpretation and limits

The snapshot control is narrow. Its chain wrapper still calls the real
`AttestationTargetState` and replaces only `ValidatorsReadOnlySeq` on the
returned immutable checkpoint state. It preserves active-validator decisions,
committee selection, signature checks, pubsub, feed, and pool work. The strong
admission, completion, and RPC differences therefore isolate native shared-MVS
registry access, including its lock and iterator mechanics, rather than the
O(120,000) predicate loop by itself.

The 8,123 native-arm messages missing from `ValidationStarted` were accepted by
the sender's `Publish` API but were not observed by the wrapper around Prysm's
validator. The primary run did not install a raw GossipSub tracer, so it cannot
separate peer egress, validation-queue, throttle, or subscription admission
drops. They are not counted as validation ignores or rejects. The snapshot arm
receiving all 15,000 under otherwise equal construction makes this loss part of
the observed end-to-end effect, but it does not identify the exact pubsub queue.

The native run's 22.501-second settle time includes a final three-second stable
window and covers only the 6,877 messages observed by validation. The snapshot
run's 6.202 seconds likewise includes that window. These are end-to-end fixture
times, not aggregate CPU time.

This is one BN plus one lightweight publisher in one process. It retains real
Prysm and libp2p scheduling overhead but omits the retained reproductions' three-BN network,
multiple VC streams, execution traffic, concurrent attestation-data calls, and
the retained run's `GOMAXPROCS=4` setting. The shared run's peak of 378 active
iterators is much smaller than retained cohorts with roughly 910 count callers
and other concurrent work. Those differences plausibly explain why this clean
positive reached 131 ms rather than the historical seconds-scale RANDAO miss;
the measurement does not prove that extrapolation.

The original Domain-only pair also did not explicitly select the internal
fork-choice head through `FullHead`. Its checks establish the cached chain
head and the existence of the full genesis node, not a selected
`CanonicalNodeAtSlot` result. A later attestation-data writer preflight
exposed that missing setup and initializes it through the real public API
for the writer experiment. These retained Domain-only measurements are not
retroactively presented as attestation-data RPC tests.

It also set the Heze and Gloas fork epochs to zero while leaving earlier
fork-epoch fields at their inherited values. This was sufficient for the
measured Heze gossip/count path and ordinary Domain method, but the later
attestation-data pilot exposed a contradictory API gate: its pre-Electra
branch returned request index zero before reaching the Gloas payload-status
branch. The writer fixture corrects every prerequisite fork epoch to zero.
The older pair therefore isolates the recorded registry-iterator effect
within its fixture; it is not a fully coherent Heze API configuration or an
exact production startup replay.

This baseline initializes the blockchain service's cached genesis head through
`StartFromSavedState`, but does not perform a subsequent fork-choice `FullHead`
selection. Its gossip checkpoint/count path does not need that selection.
The later attestation-data composition must additionally select the canonical
head through that public API and verify agreement with the cached head before
load; accepting a zero canonical root as an attestation-data fixture would be
incorrect.

The immutable fixture set Gloas and Heze fork epochs to zero while leaving the
mainnet prerequisite fork epochs unchanged. Its Heze state, valid Gloas
messages, and real gossip validation pipeline passed, so the native-versus-
snapshot registry comparison remains applicable to the exercised path. That
configuration is not coherent for APIs such as `GetAttestationData`, whose
payload-index logic checks the Electra epoch first. These arms did not call
that API; the later natural-writer variant activates every prerequisite fork
at epoch zero.

## Artifacts and rerun

- Test-only helper: [`full_gossip_domain_export_test.go`](../../../beacon-chain/sync/full_gossip_domain_export_test.go)
- TCP coordinator: [`full_gossip_domain_tcp_diagnostic_test.go`](../../../beacon-chain/sync/full_gossip_domain_tcp_diagnostic_test.go)
- Commands: [`full-gossip-domain-go-evidence/command.txt`](full-gossip-domain-go-evidence/command.txt)
- Native arm: [`shared/summary.json`](full-gossip-domain-go-evidence/shared/summary.json),
  [`shared/client.jsonl`](full-gossip-domain-go-evidence/shared/client.jsonl),
  [`shared/server.jsonl`](full-gossip-domain-go-evidence/shared/server.jsonl), and
  [`shared/full.test.log`](full-gossip-domain-go-evidence/shared/full.test.log)
- Snapshot arm: [`snapshot/summary.json`](full-gossip-domain-go-evidence/snapshot/summary.json),
  [`snapshot/client.jsonl`](full-gossip-domain-go-evidence/snapshot/client.jsonl),
  [`snapshot/server.jsonl`](full-gossip-domain-go-evidence/snapshot/server.jsonl), and
  [`snapshot/full.test.log`](full-gossip-domain-go-evidence/snapshot/full.test.log)
- Slot-one positive preflight: [`preflight-slot1.test.log`](full-gossip-domain-go-evidence/preflight-slot1.test.log)

The two command lines in `command.txt` are the complete Go invocations. The
failed peer-discovery, subnet-assertion, and domain-comparison pilots are kept
under the same evidence directory as named fixture-development failures; none
is treated as a control result.

The immutable primary source is jj revision `2b96358e`. At that revision, the
TCP coordinator SHA-256 is
`903d025d25cff23fbf64114cd63bebb4296264b558c8d1262c7a3ec36518eb83`
and the internal fixture helper SHA-256 is
`c023cfd44e00e1ea8cb150ac14ca90ab6beebeb2204b016bd155d2946859c00c`.
These hashes preserve the exact source identity after later env-gated variants
were added in the shared working copy.
