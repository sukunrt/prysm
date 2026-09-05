# Isolated round-2 proposal construction benchmarks

## Question and boundary

Determine whether the deployed proposal algorithms, with controlled attestation
inputs and no network or competing gossip workload, produce seconds of work in
the region between execution-payload selection and completed beacon construction.
The historical comparison range is 2.542–7.763 seconds on 13 selected slow blocks;
that range is an observation to test against, not a target to tune the fixture to.

Round 2 ran Prysm `0280403c70d88967f49d2d4c730f4c5417dabdf5`.
Final measurements must compile that exact revision in the separate jj
workspace `/home/sukun/dev/prysm2-round2-construction-028`. Only new
diagnostic test files are added there; production source and dependencies come
from the historical revision. The benchmark explicitly constructs its old
Gloas-keyed pool behavior.

An initial current-checkout pilot was used to validate the empty-block fixture.
It is retained in `state-root-pilot.log` for the audit trail and excluded from
the final historical-code results. Function-level source parity was checked,
but the user's requirement is to time the historical checkout itself.

## Independent comparisons

- Compact gossip aggregates: save the same votes in their historical Gloas form
  or normalized Electra form, apply the same inclusion-pruning operation, and
  add the same fresh votes. Record remaining entries before measuring packing.
- Credited participation: measure candidate reward scoring with participation
  already credited and with the corresponding flags clear. Assert the expected
  reward behavior; zero useful reward must not be confused with zero work.
- Raw singles: measure explicitly labeled raw-pool shapes independently from
  compact retained aggregates. The supplied 13,000/six-committee and
  5,000/two-committee arrival scales are not historical pool snapshots.
- Mixed pools: combine compact aggregates and singles, with actual old pool
  admission, inclusion pruning and snapshot filtering. Separate singles that
  arrive before an aggregate from singles offered after an aggregate. Record
  the underlying unaggregated map count and the eligible snapshot count.
- State transition and root: use a fresh copy of a coherent nonzero-slot parent,
  time transition and hashing separately, and prevent repeated hashing of an
  unchanged cached post-state from becoming the headline result.

## Measurement and validity

Use Go builds/tests, not Bazel. Keep changes in diagnostic test files. Prepare
keys, signatures, states and pools outside the measured interval. Run timing
processes sequentially after compilation has stopped; record host, Go version,
GOMAXPROCS, command, input counts, elapsed times and allocations where available.
Use the production full packer as a correctness reference for any test-only
stage decomposition. Verify signatures and per-attestation-data coverage, not
only a union of validator indices. Reject fixtures that silently lose their
attestations through the state-transition retry path.

The repeated historical votes at attestation slots 91 and 93 each comprise six
committee aggregates, with 14,920 and 14,927 participants respectively. That
does not mean that either vote comprised 15,000 pool objects. Their source and
target checkpoint roots and the original proposer pools are not recorded in the
available ledger. A coherent synthetic state can match the measured shape, but
must not be described as an exact historical state or pool replay.

The compact comparison is a minimal coverage fixture, not a census of the
historical pool: overlapping partial aggregates can leave more than one object
under a committee key. The actual slot-97 block contained eight on-chain
attestations; a fixture combining three attestation slots covers only part of
that block's possible processing shape.

Separate round-boundary slot 96/head 95 from ordinary slot 97/head 96. The
signature filter's checkpoint shortcuts differ across that boundary. None of
the 13 selected slow proposals was a round-boundary slot. Slot 97's proposer
log directly records build entry at `01:49:24.008736237Z` (line 1121950),
payload selection at `01:49:24.017249896Z` (1121953), and construction finish
at `01:49:27.662622713Z` (1136845): selection to finish is 3.645372817 s.
Its parent 96 was imported at `01:49:13.929971934Z` (1115472). These anchors
are in `runs/round2/prysm-geth-1/beacon.log`.

If compact retained aggregates cost only milliseconds in isolation, report that
negative result. A slower raw-single result would establish a different input
mechanism; it would not prove that the historical proposer held that raw pool.

## Single-vote lifecycle constraint from the deployed code

`DeleteAggregatedAttestation` inserts seen bits under the supplied Electra key
before it fails to find a Gloas aggregate. `SaveUnaggregatedAttestation` checks
those bits, and `UnaggregatedAttestations` checks them again for each stored
entry. Thus covered, already-included Electra singles must not be forced into
the eligible proposer snapshot. They can remain temporarily in the underlying
map and incur snapshot scanning cost until normal cleanup removes them.

For a fresh, unincluded aggregate, the Gloas/Electra mismatch can instead make
the aggregate-coverage lookup miss when a single is offered afterward. The
normalized comparison must execute the same lookup and save sequence. Singles
offered before their aggregate are a separate case: both versions can retain
them until compaction or inclusion. A stress case offering all 13,000 singles
after an aggregate must not be described as an observed historical ordering.
