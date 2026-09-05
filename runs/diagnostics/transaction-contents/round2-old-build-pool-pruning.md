# Round 2 old-build aggregate-pool pruning discriminator

## Result

The retained Round 2 logs contain the runtime pattern expected when included
Gloas FFG aggregates are not removed from the aggregate pool: an identical
attestation data root and identical full validator list is included again in
the next retained canonical block.

This check is deliberately bounded to `prysm-geth-400/beacon.log` and slots
through 100. The retained ancestry is taken from
`runs/diagnostics/startup3/round2-first100-parent-lineage.md`; bypassed blocks
are excluded before comparing each retained block with its retained parent.
Reproduce it from the repository root with
`python3 runs/diagnostics/transaction-contents/round2_old_build_pool_pruning.py`.

Two direct examples span three consecutive retained blocks, 94, 95, and 96:

| attestation slot | data root | seats / validator count | validator-list SHA-256 prefix | containing block | raw line / timestamp |
| ---: | --- | ---: | --- | ---: | --- |
| 91 | `0x1fe5887a6af98797fcbe5d8470105e28543f2f671e55de5236cd2bc268037d` | 14,920 / 14,920 | `ff7de25eb7d8eaba` | 94 | `beacon.log:507053`, `01:48:49.007170218Z` |
| 91 | same | 14,920 / 14,920 | `ff7de25eb7d8eaba` | 95 | `beacon.log:512902`, `01:49:00.819872468Z` |
| 91 | same | 14,920 / 14,920 | `ff7de25eb7d8eaba` | 96 | `beacon.log:519927`, `01:49:13.518244919Z` |
| 93 | `0x4868980d9c37478e681c6e959156964064af28698a60009504f604a48d20c6` | 14,927 / 14,927 | `677aaf70de49aa72` | 94 | `beacon.log:507049`, `01:48:48.981578033Z` |
| 93 | same | 14,927 / 14,927 | `677aaf70de49aa72` | 95 | `beacon.log:512960`, `01:49:00.825366074Z` |
| 93 | same | 14,927 / 14,927 | `677aaf70de49aa72` | 96 | `beacon.log:519924`, `01:49:13.501154728Z` |

Equality above is byte-for-byte equality of the repaired validator CSV, not
only equality of `seats`, `attSlot`, or `dataRoot`. Across the complete bounded
scan, node 400 records 935 `FFG vote included` entries. All 935 parsed after
repairing the log framing described below. Of those, 536 belong to the retained
first-100 ancestry, and 211 are exact vote records repeated across 56 retained
parent-to-child links.

## Log framing repair

The largest `validators` fields are about 91 KB. The capture wrapper inserted
the same outer RFC3339 timestamp every roughly 16 KiB without inserting a
newline, sometimes splitting a validator index in the middle. Before parsing,
the check removes every outer prefix matching
`2026-09-05T[0-9:.]+Z ` and strips ANSI color sequences. Removing the injected
prefix rejoins the split digits. The six example records have `seats` equal to
the number of comma-separated validators, and no inclusion record has a parse
gap.

## Scope and interpretation

Slots 94, 95, and 96 are consecutive blocks in the retained ancestry, so the
two three-block repetitions cannot be explained as reinclusion on competing
forks. This is direct evidence of repeated inclusion on the continuing branch,
consistent with the deployed pruning defect. It establishes retained aggregate
work and reinclusion; it does not identify the exact failed prune path,
timestamp pool deletion, quantify its CPU cost, or by itself explain late
publication.

This observer does not reconstruct proposer-pool or ingress provenance. A
gossip record cannot be joined to these inclusion records on `dataRoot` alone,
because committee aggregation changes the grouping hash. The logs therefore do
not prove which proposer retained either aggregate in its local pool.
