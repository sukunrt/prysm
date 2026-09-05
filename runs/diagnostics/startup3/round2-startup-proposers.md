# Round2 startup proposer reconstruction

Round2 genesis was exactly `2026-09-05T01:30:00Z`. The archive indexer was run
over the locally recovered round2 archives with `--min-slot 1 --max-slot 20
--round round2 --genesis 2026-09-05T01:30:00`. It reads only `validator.log`
from each tar stream and associates events with absolute slot windows, avoiding
rounded `timeUntilDuty` and overlapping late handlers.

## Slots 1–20

| Slot | Owner | Proposer pubkey prefix | VC terminal evidence | BN boundary when reached |
|---:|---:|---|---|---|
| 1 | 169 | `a7120c370e8c` | RANDAO DomainData deadline, 01:30:24.002452, VC line 694 | No block request reached |
| 2 | 191 | `af089d24fea6` | Sync-selector preflight expires at 01:30:36.001289; dispatched RANDAO then observes expired context at .004480, VC line 780 | No block request reached |
| 3 | 22 | `a4a9ccf01138` | Sync-selector preflight expires at 01:30:48.000619; RANDAO follows at .003364, VC line 800 | No block request reached |
| 4 | 91 | `b1b3d89c62f5` | Sync-selection signing in preflight expires at 01:31:00.003838; RANDAO follows at .003910, VC line 839 | No block request reached |
| 5 | 118 | `a0b75a40344e` | GetBeaconBlock deadline, 01:31:12.002351, VC line 971 | Building began +10.119 s, BN line 524 |
| 6 | 83 | `989ab25f870c` | Internal/no fallback, 01:31:16.634985, VC line 1097 | GetPayload HTTP timeout; build failed +4.455 s, validator 21291, BN lines 545–546 |
| 7 | 144 | `b3dcff8d6129` | Sync-index preflight expires at 01:31:36.001455; RANDAO follows at .002488, VC line 1122 | No block request reached |
| 8 | 19 | `8736ff704680` | GetBeaconBlock deadline, 01:31:48.002692, VC line 1419 | Building began +11.190 s, BN line 533 |
| 9 | 107 | `b3467ad53274` | Internal/no fallback, 01:31:55.923308, VC line 1207 | GetPayload HTTP timeout; build failed +7.638 s, validator 35397, BN lines 533–534 |
| 10 | 35 | `a7b6c30e5bbe` | GetBeaconBlock deadline, 01:32:12.004886, VC line 1384 | Build +7.446 s; payload chosen +10.338 s, BN lines 514–519 |
| 11 | 117 | `a61c6d4cd99b` | Sync-index preflight expires at 01:32:24.001545; RANDAO follows at .012453, VC line 1130 | No block request reached |
| 12 | 14 | `88d47493db4d` | Sync-index preflight expires at 01:32:36.000792; RANDAO follows at .007049, VC line 1391 | No block request reached |
| 13 | 20 | `8147108c9e32` | GetBeaconBlock deadline, 01:32:48.008413, VC line 1221 | Build +1.316 s; FCU payload ID only at +10.342 s, BN lines 527–530 |
| 14 | 85 | `9695b49b1c8b` | GetBeaconBlock deadline, 01:33:00.002297, VC line 1561 | Build +2.693 s; payload chosen +2.744 s, BN lines 543–546 |
| 15 | 32 | `b8046a792d6f` | Submitted +1.658630, VC line 1324 | Finished build, validator 18641, BN line 536 |
| 16 | 76 | `801b58c728f8` | Submitted +6.773790, VC line 1846 | Finished build, validator 107427, BN line 724 |
| 17 | 99 | `83807808389e` | Submitted +2.745313, VC line 1642 | Finished build, validator 30852, BN line 631 |
| 18 | 49 | `aba6b737401b` | Submitted +10.926548, VC line 1990 | Finished build, validator 59794, BN line 538 |
| 19 | 142 | `b56e04441723` | Owner's submit RPC returns deadline at 01:34:00.001012, VC line 1555 | Block `0xbb5aa408…` nevertheless exists; peer node400 imports it at +12.562 s (line 56455). Owner BN finished building at +11.945 s, validator 117488, line 559 |
| 20 | 141 | `b704bc975589` | Submitted +6.322556, VC line 1610 | Finished build, validator 116636, BN line 640 |

The owner archive for each row is
`/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-<owner>.tar.gz`.

## What the sequence establishes

Slots 1–14 all missed; slot 15 was the first observed block. The terminal
mechanisms are not uniform. Six slots exhausted synchronous sync-committee
preflight and then dispatched a proposer with an already expired context
(2–4, 7, 11, 12). Slot 1 has no preceding preflight error and fails directly
at RANDAO DomainData. Five reached a block request but exhausted its slot
deadline (5, 8, 10, 13, 14), and two reached local payload retrieval but lost
to the 300 ms execution HTTP timeout with no P2P fallback (6, 9). Slot 19 is a
later case where the owner's submit RPC reports a deadline after building
finished only 54 ms before the boundary, but the block was published and
imported by a peer; it is not a missed network block.

This progression explains “the chain started late” operationally: no block was
observed until slot 15 at genesis +181.650 seconds. The evidence does not
support a single persistent EL
failure. Some attempts never reached the BN block method; slots 6 and 9 reached
it but timed out reading a locally available payload; and slots 10 and 14 had
already selected a self-build payload before the VC deadline. Recovery is
gradual rather than an unlock at one exact boundary: block-path progress is
visible before slot 15, but it repeatedly arrives too late. A VC `Submitted
new block` timestamp is RPC completion, not initial publication: for example a
peer imported slot 16 at +2.059 seconds while its owner did not log RPC
completion until +6.774 seconds. The table therefore uses peer imports, where
available, to decide whether a block existed.

The reproduced diagnostics provide a common pressure mechanism—genesis-state
FFG active-count scans create extreme CPU demand, fork-choice reader convoys,
and Go runnable starvation—but they are not historical per-process profiles.
Accordingly, this table classifies each directly observed terminal boundary;
it does not relabel every historical gap as the same scheduler interval.

For the RANDAO rows in particular, the historical VC logs do not time the
RANDAO call itself. Detached `subscribeToSubnets` work can overlap early slots
and contend on the VC-wide domain lock, while the RPC transport and BN handler
are also uninstrumented. See [`round2-slot1-duty-audit.md`](round2-slot1-duty-audit.md)
for the exhaustive slot-1 role accounting and its remaining boundary.

## Block-construction paths behind VC deadlines

The five VC `GetBeaconBlock` deadlines do not share one BN terminal stage.
Following each explicit slot-tagged handler beyond the VC's 12-second window
gives the following split:

- Slot 5/node118 entered at +10.119 s. At +12.011 the local payload read timed
  out; P2P fallback was absent, and the handler logged its terminal build error
  at +14.527 (`beacon.log:524–528`, validator 72524). Its consensus-field
  packing goroutine was not joined on this early return and logged cancellation
  later at +28.842 (line 532).
- Slot 8/node19 entered at +11.190 s. The local payload timed out at +12.007
  and the no-fallback build error followed at +12.052
  (`beacon.log:533–540`, validator 10907). Its detached packing branch logged
  cancellation at +20.467 (line 542).
- Slot 10/node35 entered at +7.446 s, obtained an FCU payload ID at +10.015,
  and chose the self-build payload at +10.338. Here payload selection was not
  terminal. `buildBlockGloas` waited for its parallel consensus-field branch;
  packing returned `context canceled` only at +36.873, immediately followed by
  state-root failure and the explicit build error at +36.874
  (`beacon.log:514–522`, validator 20714).
- Slot 13/node20 entered at +1.316 s and did not reach parallel block assembly.
  FCU produced a payload ID only at +10.342; `getParentState` then failed at
  +12.038 while processing slots to 13 (`beacon.log:527–531`). No packing
  goroutine or explicit final `Could not build block` was logged for this
  attempt.
- Slot 14/node85 entered at +2.693 s and chose its self-build payload at +2.744.
  As in slot 10, the handler then waited for the consensus-field branch.
  Packing returned cancellation at +43.993 and state-root/build failure followed
  at +43.996 (`beacon.log:543–563`, validator 22673).

The source ordering explains which late packing messages are causal.
`buildBlockGloas` starts `setPreGloasConsensusFields` in a `WaitGroup` goroutine,
does payload selection on the caller, then normally calls `wg.Wait()` before
computing the post-block state root (`proposer_gloas.go:27–89`). If local
payload and fallback both fail, it returns before `wg.Wait`; that already
started goroutine can subsequently emit an orphaned packing cancellation. Thus
the late packing errors for slots 5 and 8—and similarly the separately
classified payload failures at slots 6 and 9—do not replace their payload
failure as the terminal cause. For slots 10 and 14, payload selection succeeded
and the immediate packing-error/state-root-error sequence is on the necessary
joined path. Slot 13 fails earlier in `getParentState`.

These logs establish ordering, not the internal reason the parallel branch took
29.4 or 41.3 seconds from handler entry to observe cancellation. In particular, an untagged invalid-
attestation cleanup error or later context-canceled log is not assigned to an
old handler unless its explicit slot-tagged completion and source lifetime make
that association possible.

### What can delay cancellation inside packing

`packDepositsAndAttestations` runs deposits and attestations in a second
errgroup. On the attestation side, `packAttestations` performs the following
sequence (`proposer_attestations.go:32–112,157–244,350–430`):

1. snapshot aggregated and unaggregated pools;
2. validate every candidate against the supplied head state and delete invalid
   candidates;
3. normalize, deduplicate by containment, group and aggregate by attestation
   data;
4. construct on-chain aggregates, deduplicate again, sort by proposer reward,
   cap the list, and verify signatures.

With the non-experimental pool path, snapshotting takes the aggregated and
unaggregated pool `RLock`s while copying entries; unaggregated snapshotting also
checks the seen-bit cache and clones each returned attestation. Invalid deletion
takes the corresponding pool write lock one candidate at a time. The
experimental cache, if enabled, instead holds its cache `RLock` while returning
all entries and its write lock while `DeleteCovered` scans a data group. The
historical node85 log does not record a pool-size or lock-owner snapshot.

Cancellation is sparse. The outer errgroup checks `ctx.Done` only after
`packAttestations` or `deposits` returns. `deleteAttsInPool` checks `ctx.Err`
before each deletion, but acquiring an individual pool lock has no contextual
wait. Candidate filtering, containment deduplication, aggregation/max-cover,
sorting, and most per-attestation verification loops have no top-level
cancellation check. `VerifyAttestationNoVerifySignature` receives the context,
but still performs its state and committee checks synchronously. Because the
working parent state passed into packing has been processed through empty slots
toward slots 10 and 14, its `ActiveValidatorCount` is eligible for the normal cache; the confirmed
slot-0 full-registry bypass is not itself a deterministic per-candidate scan in
these handlers.

The isolated packing benchmarks establish a possible CPU-heavy shape, not the
historical input: containment dedup is superlinear and measured about 0.4 s for
2,500 same-data candidates and 2.7–2.8 s for six such groups; max-cover is also
hundreds of milliseconds at 2,500. Existing historical logs provide no evidence
that node35 or node85 had the hundreds or thousands of pool candidates needed
to extrapolate those measurements to seconds, much less 29–41 seconds. The H/E
profiles establish severe concurrent gossip CPU and runnable starvation, but
do not profile these historical packing goroutines.

The defensible alternatives for slots 10 and 14 are therefore: an attestation
pool lock wait behind concurrent mutation; uncancelable validation,
deduplication, aggregation, sorting, or signature work on an unknown-sized
snapshot; or a runnable goroutine that was starved before it could reach its
next cancellation observation. The final `context canceled` identifies what
the branch observed when it eventually returned, not which alternative
consumed the preceding wall time. There is no source-supported nested
fork-choice lock in this packing sequence and no historical evidence of a
deadlock: both branches eventually returned and the joined handler proceeded
to its state-root call within milliseconds.
