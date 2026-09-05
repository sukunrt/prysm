# Round 2 later-slot archive census

Coverage update: all 1,000 archives have now been recovered and scanned. See
[the final recovery report](round2-remaining-archive-audit.md) and
[integrated failure analysis](round2-failure-analysis.md). The 205-node
measurements below remain a bounded intermediate census; its unknown-owner
rows are no longer explained by unavailable archives.

This is a bounded census of the locally available round-2 archives, not a proof of the
cause of every later miss. `round2_later_slot_audit.py` streamed only `beacon.log` and
`validator.log` from 205 distinct node archives (nodes 1--200 plus the locally retained
large-node samples) and unioned abbreviated `Synced new block` roots through slot 226.
It also records validator proposal outcomes and execution `parentHash`/`payloadHash`
fields. Those latter fields are execution lineage, **not** beacon-parent ancestry.

Reproduce with:

```console
python3 runs/diagnostics/startup3/round2_later_slot_audit.py \
  /tmp/prysm-r2-extra-logs.Rd7MjT runs/round2 \
  --genesis 2026-09-05T01:30:00 --max-slot 226
```

## The transition after slot 128

This is not a continuation of the genesis-startup failure mode. Slots 120--128 each
have the same root at 203--204 archived observers. At `01:55:48`, the slot-129
boundary, the logs instead contain a common deep rollback: 269 occurrences across
the archive set report old head slot 128/root `0x4a3a9ee7...` changing to slot
117/root `0xde9cfb98...`, depth/distance 11. Some nodes report the transition more
than once, hence the count exceeds 205. Representative anchors are node 20
`beacon.log:1057` and node 112 `beacon.log:981`. They also still report 70--84 peers
immediately beforehand/afterward, so this was not process shutdown or a simple loss
of all peer connections.

The slot-129 block `0xca031ee7...` was imported by only three archived observers.
Thereafter most produced roots appear at only one local observer (occasionally a
small group); slots 159 and 167 temporarily reach 190 and 191 observers. The union
contains no slot with two distinct abbreviated imported beacon roots, but that does
not prove a single global chain: proposer schedules themselves diverge (two owners
at slot 160 and three at slot 166), and a block confined to an unavailable archive
would not enter this union. The later sparse imports and misses must therefore be
analyzed in the post-rollback/divergent-chain regime, separately from slots 1--14.

## Union of later gaps

The imported-slot ranges after 128 are `129`, `131--135`, `137--159`, `161`,
`164--165`, `167--170`, `172--175`, `178--181`, `183--190`, `192`, `194--198`,
`201--205`, `207--209`, and `211--225`. The all-archive union has no imported root
for:

| Slot(s) | Evidence in available owner validator logs |
| --- | --- |
| 130, 200 | `Failed to propose block`: deadline exceeded |
| 136, 163, 176, 191, 193, 199 | `Failed to request block`: deadline exceeded |
| 206 | request failed after an HTTP/2 `RST_STREAM CANCEL` |
| 160, 210 | proposal rejected during broadcast/receive with invalid RANDAO signature |
| 166 | divergent schedules: two owners fail invalid-RANDAO validation and another request reaches its deadline |
| 162, 171, 177, 182 | no proposer schedule is present in the 205 available validator logs; absence of a union import is not proof that no unavailable node produced a block |
| 226 | owner request is canceled at `02:15:12.405`; this is the capture/shutdown boundary, and only 200 archives extend to the slot start |

Thus broadening from a handful of observers does **not** fill these later holes, but
only the rows with an observed owner failure support a concrete terminal class. It
does not establish the upstream cause of the deadlines or invalid signatures.

## Coverage and interpretation limits

All 205 beacon logs cover every slot start through slot 225. Their last timestamps
range from `02:15:11.421` to `02:16:25.367`; 200 cover the slot-226 start. Before the
rollback, an absent union root is strong evidence of a miss because roughly 203
independent observers repeatedly record each successful block. After the rollback,
the ordinary successful-block observer count falls as low as one, so “not imported
by this archive union” is the defensible statement; “not produced anywhere” is not.
No archive download, network request, or production-code change was made for this
census.
