# What the retained aggregate logs can bound

The two repeated on-chain attestations at slots 91 and 93 reconstruct from
six committee aggregates apiece, with 14,920 and 14,927 participants. The
[fixture-shape record](actual_ffg_fixture_shape.json) retains their roots,
counts and selected raw anchors. This describes their compact coverage; it
does not establish a 12-object historical proposer pool.

## Bounded arrival counts

A read-only search for `FFG aggregate` with the exact attestation slot and
beacon block root finds these complete-log record counts:

| Node | Attestation slot | Gossip records | Local records | Total |
| ---: | ---: | ---: | ---: | ---: |
| 1, the slot-97 proposer | 91 | 11 | 2 | 13 |
| 1 | 93 | 19 | 1 | 20 |
| 400 | 91 | 16 | 0 | 16 |
| 400 | 93 | 26 | 0 | 26 |

The exact roots used are:

- Slot 91: `0x7b609d078b5fe84889cd4d55cbdb1dce39842760ece749b01b05943e5f93f23a`.
- Slot 93: `0xbd3087a87d2ce719d2523f4f7dec64af872539b649c0133de27387bae0b78a1c`.

The records occur around 01:48:20 and 01:48:44 UTC, approximately eight seconds
into the respective attestation slots and well before slot-97 construction.
On node 1, the slot-91 local records are `beacon.log:1065502–1065503`, followed
by the first gossip record at `:1065504`; the last three slot-93 gossip records
are `:1089496–1089498`. Node 400's selected maxima have exact anchors in the
fixture-shape JSON, beginning at `beacon.log:490289` and `:502353`.

The smallest logged aggregate in this selected node-1 flow has 2,076 seats;
the smallest in node 400's flow has 1,682. Every record therefore contains
more than half of its 2,500-member committee. Any two records sharing a
committee necessarily overlap.

## A useful next reconstruction, and its limits

For this selected flow, the production `MaxCoverAttestationAggregation`
cannot combine two same-committee records: it requests disjoint coverage,
and all such pairs overlap. Its remaining object count is governed by
coverage checks and contained-set filtering. The full logged validator CSVs
are enough to reconstruct those bit relationships without the unlogged BLS
signatures. A bounded comparison could check whether every candidate is a
subset of its committee's selected maximum; if so, that logged flow reduces
to one aggregate per committee. That subset comparison has not been performed
in this note. Equal seat counts alone would not establish equal sets.

This reconstruction would still describe a controlled replay of logged
inputs, not an observed pool snapshot. `recordFFGAggregate` runs after gossip
validation but before the message is assigned to `ValidatorData` and handed
to the subscriber (`beacon-chain/sync/validate_aggregate_proof.go:156–162`).
Its log does not timestamp successful pool insertion. The local ledger line
records publication through the RPC and likewise does not prove a new
aggregate-pool object (`beacon-chain/rpc/core/validator.go:403–430`).

The 30 gossip records on node 1 bound the number of inputs from that selected,
logged gossip flow, not the whole candidate pool. Local compaction of singles,
other attestation slots or roots, insertion/deletion timing and concurrent
pool operations remain outside this count. The initial 12-old-aggregate
benchmark should consequently be described as a compact, controlled fixture.

## Cache and stage conditions for the isolated benchmarks

- A warm committee control should explicitly populate the relevant epoch
  caches and confirm cache hits. A call to `BeaconCommitteeFromState` can
  start an asynchronous fill and return before it completes. Epoch-2 old
  votes and epoch-3 fresh votes can require separate entries.
- `helpers.ClearCache` clears committee, sync-committee and balance caches;
  using it between iterations changes more than committee warmth. The
  active-balance key at slots 96/97 includes the block root at slot 95,
  epoch 3 and validator count.
- A prepared state at slot 97 excludes advancing its parent through slots
  and round/epoch transitions. An empty-parent block also excludes applying
  a full parent's execution requests. These are explicit fixture conditions,
  not evidence that those costs were absent from every historical proposal.
- For an incremental state-root measurement, hash the base state outside
  timing, run a fresh transition from a copy for each sample, and hash that
  newly changed post-state once. Repeatedly hashing one already-hashed
  post-state would measure a cache hit. The deployed default disabled
  proposer preprocessing; the fixture should exercise that same branch.

Slot 96 with head 95 also differs from slot 97 with head 96 in the packer's
signature-filter shortcuts. The filter derives its target rounds from the
head slot but compares them with the current wall-slot round. At slot 96,
target-round-11 votes can therefore enter batch signature verification;
at slot 97 they can pass the previous-target shortcut. These paths must be
named explicitly when comparing benchmark results with the historical
slot-97 construction interval.
