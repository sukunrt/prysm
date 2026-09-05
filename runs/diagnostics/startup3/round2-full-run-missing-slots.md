# Round-2 full-run missing-slot census

This initial five-observer census is superseded for coverage by the
[expanded archive census](round2-later-slot-audit.md). Many of the apparent
later holes below contain blocks imported by other archived nodes. The
five-observer counts remain useful as propagation/observer evidence, not as
the latest missing-slot list.

This is a bounded census through slot 226.  It separates three observations
that are not interchangeable: a validator client's proposal result, a block
seen by at least one sampled beacon node, and a canonical block.  The archived
logs do not by themselves establish the last category.

## Sampled beacon-node union

The detailed observer archives for nodes 1, 50, 150, 201, and 400 report 120,
116, 118, 118, and 117 distinct `Synced new block` slots respectively.  Every
one first reports slot 15.  Their latest reported slots are 169, 167, 197, 167,
and 167.  The union contains 123 of slots 1–226 and does not contain:

```
1–14, 129–132, 134–143, 145–146, 148, 150–156, 158,
160–166, 168, 170–196, 198–226
```

This means only “not found in these five observer logs.”  A single observer's
absence is not a network-global missing slot, and even the union does not prove
canonicality or exclude a block seen only by an unsampled node.  Coverage also
ends around shutdown: slot 226's proposer reports `context canceled`, so the
tail must not be analyzed as an uninterrupted live network.

## Validator-client census

`index_archives.py` over the locally available round-2 archives found 30
failure records through slot 226.  The initial slots 1–14 have one consistent
scheduled owner each and all fail:

- RANDAO domain deadline: slots 1–4, 7, 11–12 (7)
- `GetBeaconBlock` deadline: slots 5, 8, 10, 13–14 (5)
- local-payload/no-P2P-bid fallback: slots 6 and 9 (2)

Slot 15 is the first VC submission and the first block in every sampled
observer.  Later indexed failures occur at slots 19, 130, 136, 160, 163, 166,
176, 191, 193, 199, 200, 206, 210, and 226.  Slot 166 has three different
failure records from different scheduled owners, and the complete index has
230 distinct `(slot, proposer pubkey)` records for only 226 slot numbers.
After chain divergence, different nodes can therefore compute different duty
schedules.  A later owner failure is not proof that no block for that slot
existed: for example, the five-observer union contains slot 19 even though one
indexed VC reports a proposal deadline there.

Six later schedule records have no matched proposal event in that owner's log
(slots 160, 167, 168, 174, 186, and 187).  They are “no matched event,” not a
new failure class.  Log coverage, a changed local schedule, or an unlogged
path can produce the same result.

## Scope of the startup diagnosis

The clean conclusion for startup is narrow and strong: slots 1–14 have
consistent owners, direct VC failures, and no block in any of the five detailed
observers; slot 15 is the first observed recovery.  The source audit and
round-transition diagnostic explain why newly generated round-1 votes can use
a nonzero-slot checkpoint state after slot 8.  Delayed epoch-0/round-0 gossip
can remain admissible and keep creating slot-0 scans after that boundary, so
neither a hard stop nor the exact recovery time at slot 15 follows from the
transition alone.

The later missing ranges are a different forensic problem.  They occur after
blocks and divergent schedules exist, include intermittent observed blocks,
and approach shutdown.  They cannot be attributed to the genesis checkpoint
scan solely because their VC error strings resemble startup errors.  Resolving
them requires root/parent-aware fork reconstruction and explicit per-archive
coverage bounds, not a slot-number union alone.
