# Full-gossip DomainData fidelity check at `GOMAXPROCS=4`

## Result

This unchanged-source rerun repeats the native shared-MVS and stable-snapshot
arms from [`full-gossip-domain-realwork-results.md`](full-gossip-domain-realwork-results.md)
with `GOMAXPROCS=4`, matching the retained H/E1 runtime setting. The shared-MVS
effect remains clear in end-to-end validation throughput: the native arm
accepted and stored 6,988 messages and settled in 19.702 seconds, while the
snapshot arm accepted and stored 10,294 and settled in 7.801 seconds. The
native arm therefore observed 3,306 fewer validations and took 2.526 times as
long to settle.

The reduced processor limit did not amplify the DomainData delay. All 32 calls
succeeded in each arm. Native DomainData reached 8.952 ms maximum versus 3.839
ms with the snapshot, and invoke-to-server-admission reached 8.704 ms versus
3.601 ms. This is much smaller than the `GOMAXPROCS=16` native arm's 131.045 ms
maximum and 68.978 ms invoke-to-admission maximum. The shared registry remains
a real throughput and servicing contributor, but this fidelity check does not
reproduce the historical seconds-long slot-one RANDAO miss.

## Measurements

Both runs reported `runtime.NumCPU=16` and `GOMAXPROCS=4`. They used the exact
same compiled fixture, 120,000-validator states, 15,000 valid publications,
real GossipSub pipelines, cold sync prerequisite, and 32-probe schedule as the
primary pair; only the process setting changed.

The fidelity change is specifically `GOMAXPROCS=4`. This fixture retains eight
slots per round and 64 total subnets, while the recovered E1/H configurations
used four slots per round and six total subnets, with a different 12-committee
source selection. See the [receiver/config audit](ffg-receiver-acceptance-audit.md).
The Domain-only fixture also predates the later writer experiment's explicit
fork-choice `FullHead` initialization. Neither distinction changes the
recorded comparison, but this is not an exact E1/H setup replay.
It also inherited pre-Heze fork-epoch fields while setting Heze/Gloas to zero;
the writer pilot exposed that inconsistency in the separate attestation-data
API's pre-Electra branch. The later writer fixture sets all prerequisite fork
epochs to zero. These retained measurements still describe their original
gossip/count and ordinary Domain paths; they do not validate the old fixture's
broader attestation-data API behavior.

| Measurement | Native shared MVS | Stable snapshot |
|---|---:|---:|
| Published / publish errors | 15,000 / 0 | 15,000 / 0 |
| Entered validation | 6,988 | 10,294 |
| Accepted / subscriber / pool / feed | 6,988 each | 10,294 each |
| Ignored / rejected / subscriber failed | 0 / 0 / 0 | 0 / 0 / 0 |
| Peak active validator iterators | 6 | 6 |
| Release to settled | 19.702 s | 7.801 s |
| Maximum publication lateness | 9.496 ms | 3.230 ms |
| Cold sync-index duration | 0.088 ms | 0.203 ms |
| Slot budget at work release | 11.968 s | 11.970 s |
| DomainData calls OK / skipped | 32 / 0 | 32 / 0 |
| First DomainData | 0.324 ms | 0.816 ms |
| DomainData p50 / p95 / max | 2.882 / 7.082 / 8.952 ms | 0.592 / 1.183 / 3.839 ms |
| Invoke-to-server-admission max | 8.704 ms | 3.601 ms |
| Admission-to-handler-return upper bound | 0.022 ms | 0.016 ms |
| Server-return-to-client max | 0.612 ms | 0.905 ms |

Percentiles use the deterministic lower indexed sample from the unchanged
[`analyze_full_gossip_domain.py`](analyze_full_gossip_domain.py). Its complete
output is [`full-gossip-domain-p4-comparison.tsv`](full-gossip-domain-p4-comparison.tsv).

The interceptor records admission before its lightweight atomic and
pool-count snapshot, so admission-to-handler-return is an upper bound that
includes that observer work. It records handler return before the post-return
stats snapshot and append, so server-return-to-client includes those operations
as well as gRPC servicing. Invoke-to-admission is outside both boundary issues.

The first native cold-sync call began with 11.968 seconds left on the absolute
slot-two deadline and returned in 88 microseconds; its immediate DomainData
call returned in 0.324 ms. The 32 calls sample only the first 7.75 seconds of
slot one. Consequently this rerun does not test the final 4.25 seconds before
the slot-two deadline and cannot exclude a later transient delay.

The 8,012 native and 4,706 snapshot publications absent from
`ValidationStarted` were successful sender `Publish` calls that the receiver's
validation wrapper did not observe. As in the primary pair, there is no raw
GossipSub tracer, so these are end-to-end losses without a more precise queue
attribution. The stable snapshot still improves completion under identical
construction, while `GOMAXPROCS=4` constrains both arms enough that even the
snapshot does not receive all 15,000.

As in the immutable primary fixture, Gloas and Heze activated at epoch zero
while the mainnet prerequisite fork epochs remained unchanged. This does not
alter the native-versus-snapshot iterator comparison in the successfully
validated Heze gossip path, but it is not coherent for the attestation-data
API, which checks the Electra epoch before its Gloas payload index. These P4
arms did not call that API. The later natural-writer variant activates every
prerequisite fork at epoch zero.

## Artifacts

- Commands: [`command.txt`](full-gossip-domain-p4-go-evidence/command.txt)
- Native arm: [`shared/summary.json`](full-gossip-domain-p4-go-evidence/shared/summary.json),
  [`shared/client.jsonl`](full-gossip-domain-p4-go-evidence/shared/client.jsonl),
  [`shared/server.jsonl`](full-gossip-domain-p4-go-evidence/shared/server.jsonl), and
  [`shared/full.test.log`](full-gossip-domain-p4-go-evidence/shared/full.test.log)
- Snapshot arm: [`snapshot/summary.json`](full-gossip-domain-p4-go-evidence/snapshot/summary.json),
  [`snapshot/client.jsonl`](full-gossip-domain-p4-go-evidence/snapshot/client.jsonl),
  [`snapshot/server.jsonl`](full-gossip-domain-p4-go-evidence/snapshot/server.jsonl), and
  [`snapshot/full.test.log`](full-gossip-domain-p4-go-evidence/snapshot/full.test.log)

No source changed for this pair. There was no stack dump, profile, or runtime
trace. The command lines in `command.txt` are the complete invocations.
The immutable baseline is jj revision `2b96358e`: its TCP coordinator hashes to
`903d025d25cff23fbf64114cd63bebb4296264b558c8d1262c7a3ec36518eb83`
and its internal fixture helper hashes to
`c023cfd44e00e1ea8cb150ac14ca90ab6beebeb2204b016bd155d2946859c00c`.
Those hashes distinguish this unchanged pair from later diagnostic variants in
the shared working copy.
