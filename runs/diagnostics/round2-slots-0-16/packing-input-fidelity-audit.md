# Slots 10 and 14: packing mechanism and input fidelity

The existing real-packer experiments establish costly production algorithms and
late cancellation without an observer artifact. They do not establish the pool
contents of node 35 or node 85. The corrected diagnostic exposes consequential
fixture choices: artificially distinct beacon roots, an uncompacted single-vote
pool, the Gloas payload-status index, and a false maximum-committee setting.
Its completed control preserves all 15,000 represented voters and valid
signatures while reducing total compaction-plus-packing work from 5.112 seconds
to 1.624 seconds. The already-compacted pool packs in about one millisecond.

This audit read the earlier packing, build, and retention reports, the diagnostic
test implementations, complete owner beacon excerpts, and historical source
revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`. It does not run the network or
modify production. Source line numbers below refer to that revision.

## What the old experiments establish

`TestDiagnosticFullPackingSustainedGenesisScanLoad` releases 6,144 workers for
15,000 finite `ActiveValidatorCount` jobs alongside a real `packAttestations`
call. Its measured cached-state control takes about 5.3 seconds; its genesis
scan arm takes 59–61 seconds and returns after a twelve-second deadline. There
is no `runtime.Stack`, runtime trace, sleep, or deliberately held lock in this
test. The separately gated CPU-profile test does not run as part of it.
Consequently the observer objection found in an earlier sync-index experiment
does not invalidate these packer durations.

The interference workload is narrower than real gossip validation: it calls
`ActiveValidatorCount` directly, without `AttestationTargetState`, checkpoint
multilocks, or gossip/subscriber work. Its cache control changes the scan state's
slot from zero to one. Unlike the newer sync-index test, it does not keep the
state identical and replace only the count result with the verified memoized
value. It demonstrates count-scan interference, not the global lock-manager
route observed in the newer sync-index trace.

The packer's own state is at slot 14 and its committee cache is warmed during
fixture preparation. Its valid-candidate count lookups therefore take the
non-genesis cache path. Repeated genesis scans occur in the competing workers,
not once per packing candidate.

The standalone profile identifies concrete avoidable work: pairwise containment
deduplication and general maximum-cover aggregation of disjoint single-bit
votes. The source does both before limiting included block attestations.
For six disjoint groups of 2,500 singles, deduplication makes
`6 × 2500 × 2499 = 37,485,000` directional containment calls: every unordered
pair fails both directions, so no candidate is removed. This count follows
from the source and fixture; it is not a historical pool count.

## Why the existing fixture is not the historical genesis view

`fullPackingFixture` sets the current justified checkpoint to round 1, uses
that checkpoint as the current-round source, and reports a head slot equal to
the candidate block's slot. Historical slots 10 and 14 are still proposals on
the genesis head. There are no preceding imported blocks from which to justify
round 1. Genesis initialization sets both checkpoints to round 0 with a zero
root (`core/transition/state.go:176–183`), while the attestation-data service
takes the source directly from the advanced head state's current justified
checkpoint (`rpc/core/validator.go:600–608,642–645`). Source checkpoint root and
genesis **block** root must not be conflated.

The fixture also sets `BeaconBlockRoot` to a synthetic value differing for every
committee. This keeps six committee aggregates separate when
`computeOnChainAggregate` groups by the attestation **data** root
(`proposer_attestations_electra.go:42–55`). Before the first imported block,
the historical votes shown in observer 400 instead share the genesis block
root. For a single attestation slot and otherwise identical data, six committee
aggregates can become one on-chain aggregate, with concatenated participation
bits and the same represented voters (`:59–93`).

That changes cancellation semantics as well as output count.
`sortOnChainAggregates` immediately returns for fewer than two candidates
(`proposer_attestations.go:207–214`). Otherwise it calls `TotalActiveBalance`,
which can return cancellation. The final function named
`filterAttestationBySignature` also does not necessarily perform BLS
verification: matching target/fork-choice branches append directly to the
trusted result; if none remain unverified it returns without a signature batch
(`:555–590`). Describing all old timed fixture results as final BLS-verified
packing is therefore too broad. Correct signature generation and separate
explicit verification remain useful fixture checks.

The corrected single-slot fixture is only one historically possible bucket.
Votes from different attestation slots have different data roots even if they
share source, target, and beacon roots. A historical pool spanning slots 1–7
can therefore produce several on-chain aggregates and still enter balance
sorting. Neither the old six-output shape nor a corrected one-output shape
proves the historical output or cancellation substage.

Useful exact historical input anchors in observer 400 are:

| Beacon line | Attestation slot | Committee | Target round | Logged `dataRoot` |
| ---: | ---: | ---: | ---: | --- |
| 8999 | 4 | 5 | 0 | `af105c327435d258e6a435c00311591bc93afc8558a6773b6d833bdab34802` |
| 9000 | 8 | 5 | 1 | `3fb84d64e264f1396d72654b6e72a0a12db7831639fcbe07de6efd95bdd552` |

Both explicitly use beacon root
`1a40155d770d5a166e5976f7f9c1804026797f51b522c4959fdcde7fcaa010d1`.
The logged values are **31-byte committee-tagged pool identifiers**, not the
32-byte SSZ attestation-data hash. `decoupled.VoteLedgerDataRoot` renders
`attestation.NewId(att, Data)[1:]`. For Electra-shaped attestations, that ID
hashes the SSZ data root followed by the comma-separated committee indices,
then keeps digest bytes 1–31 after its version byte. The committee-5 anchors
therefore use the ASCII suffix `5` before hashing.

The corrected diagnostic matches both historical identifiers using this actual
production formatter. The corresponding full SSZ data roots are:

- Slot 4: `fc76081183fea649c92f349c4fceac1297923d3d9c94336fa56299809b09ef98`.
- Slot 8: `481332fe1ff3a0399fd2fb4ee24ef73acf985aaceeaf859aaab34bf37d93d808`.

These matches require source checkpoint `(round 0, zero root)`, the historical
genesis beacon/target root, target rounds 0/1 respectively, and **data.index=1**.
Gloas uses that field for payload status. The source returns 1 for a full head
and 0 otherwise (`rpc/core/validator.go:657–669`); the earlier Electra rule of
always zero does not apply after Gloas. Actual committee membership remains in
`CommitteeBits`, so a committee-5 vote can have data.index=1. The two matching
historical identifiers establish these data semantics, not the owners' pool
counts, keys, or validator shuffling. This corrects the initial design's
incorrect direct-SSZ-hash and index-zero assumptions.

The first corrected-root fixture also exposed an unrelated artificial limit
in the old helper: `MaxCommitteesPerSlot=6`. With the normal 2048 maximum
validators per committee, an explicitly verified 15,000-voter combined output
then exceeds the helper's synthetic `2048 × 6 = 12288` bound. The appropriate
control retains the mainnet preset's **maximum 64** committees while asserting
the **actual six** committees computed by `120000 / 8 / 2500`. The resulting
indexed-attestation bound is 131072. A maximum and the computed committee count
are different parameters. The false maximum had been hidden when the old
fixture's artificial data roots kept outputs separate and its final target
filter skipped explicit BLS batch construction. Keeping the mainnet maximum
does not change committee membership or increase the experiment's 15,000 votes.

## The real background path changes the same votes' packing cost

The normal attestation service periodically calls `batchForkChoiceAtts`, which
first invokes `Pool.AggregateUnaggregatedAttestations`
(`operations/attestations/prepare_forkchoice.go:34–45,61–73`). The latter groups
single votes by data/committee, runs the specialized
`AggregateDisjointOneBitAtts`, saves the resulting aggregates, and deletes the
successfully combined singles (`operations/attestations/kv/aggregated.go:23–62,
75–85`). The specialized combiner unions single-bit coverage and aggregates
signatures without the proposer's containment/maximum-cover search
(`aggregation/attestations/attestations.go:38–70`).

The proposer instead snapshots aggregated and unaggregated pools separately,
validates every candidate, combines those slices, deduplicates, invokes the
general aggregator, constructs on-chain aggregates, and only then applies the
block limit (`proposer_attestations.go:42–47,75–99,110–128`). A backlog of raw
singles therefore exposes avoidable quadratic work even though the repository
already has a suitable specialized combiner elsewhere.

Retaining 15,000 uncompacted singles is a workload assumption. It can occur
when new arrivals outrun background service or when background service is
delayed. Successfully executing background aggregation can make the same voter
coverage much cheaper to pack. No owner log records that service's completion
or the raw/aggregated pool split at proposer snapshot time. The old fixture
implicitly models its not having completed for those votes.

The bounded counterfactual in
`beacon-chain/rpc/prysm/v1alpha1/validator/packing_compaction_diagnostic_test.go`
uses the same correctly signed slot-7 votes in two real pools. Both pools clone
the same fixture input. One remains raw; the other runs that real production
compaction call. Uncanceled production packing must preserve exactly all 15,000
validator indices, with the same attestation data and a separately verified BLS
aggregate. Post-call pool assertions check that raw packing still has 15,000
singles while the compacted pool has six aggregates and no singles. This
prevents a warmup from silently doing the counterfactual's compaction itself.

The following calls exercise real `packDepositsAndAttestations` with a
300-millisecond deadline and a cheap, explicitly disconnected deposit mock.
The comparison isolates attestation work; it does not recreate the historical
connected deposit branch, head locks, or complete Gloas builder. The separate
already-canceled case tests the exact bare `context canceled` category without
pretending its duration recreates the old failures. A bare errgroup error
locates a child's post-body cancellation check, but by itself does not identify
which child supplied the first error.

The 300-millisecond budget exposes cancellation responsiveness with finite
work. It is **not** the historical remaining budget, which was approximately
1.662 seconds after slot-10 payload selection and 9.256 seconds after slot-14
selection. The experiment must report both compaction and packing time: the
combined cost answers whether preprocessing saves total work, while the
post-compaction pack time describes a proposal finding already-aggregated
votes. This experiment isolates an avoidable code cost without changing
production or deleting represented votes.

## Completed counterfactual and its limits

The corrected Go diagnostic passes. The final source and
`/tmp/prysm-packing-compaction-go.log` were independently reviewed; no additional
CPU experiment was run for this review. The complete command and retained
output are in [the real-work results](packing-compaction-realwork-results.md).

| Operation | Raw pool | Compacted pool |
| --- | ---: | ---: |
| Real background compaction | — | 1.622792304 s |
| Uncanceled production packing | 5.112349960 s | 1.069273 ms |
| Compaction plus uncanceled packing | 5.112349960 s | 1.623861577 s |
| Packing with 300 ms budget | 5.139071165 s | 1.195880 ms |
| Budgeted result | Bare `context deadline exceeded`, zero outputs | Success, one output |

Both uncanceled outputs contain exactly the same attestation data and all
15,000 expected validator indices, and both pass explicit production BLS batch
verification. The raw pool remains 15,000 singles and zero aggregates; the
compacted pool remains six aggregates and zero singles. The separate
already-canceled invocation returns the bare `context canceled` error in
1.107976 milliseconds. Thus the historical error category is possible without
a prolonged head lock, while the measured raw-work call observes its
300-millisecond deadline about 4.839 seconds late.

The approximately 5.1-second raw cost exceeds slot 10's 1.662-second remaining
budget on this test host if that work still needs to run. It does not exceed
slot 14's 9.256-second remaining budget, explain node 85's 41.250-second
selection-to-error interval by itself, or establish either owner's input size.
The total-cost comparison includes compaction; its 1.624-second result is not
a claim that historically moving compaction into slot 10 would reliably meet
that slot's remaining budget and all other build work.

The test deliberately holds the head view at genesis and the clock view at
slot 14. Historical node 85 finishes in wall slot 17 after importing blocks 15,
16, and 17. Final attestation filtering reads the then-current head and clock
(`proposer_attestations.go:528–551`), so its target-trust and signature paths can
change while the old request is outstanding. The experiment reproduces costly
packing and its cancellation behavior in a steady genesis view, not that full
evolving history. Its head/fork-choice fetchers and cheap disconnected deposit
branch are mocks; the pool, compaction, attestation packer, errgroup wrapper,
committee/index conversion, and explicit signature verification are real.

## What the historical owners still prove

Node 35 selects its payload at slot 10 +10.338144 and emits the bare packing
cancellation at +36.873390 (`beacon.log:519,521`). Node 85 does so at slot 14
+2.744065 and +43.993917 (`:546,562`). The required consensus branch is still
outstanding when the proposal budgets expire. Its final return is followed
within 1.23/2.34 milliseconds by the canceled state-root/build failure.

The bare error is selected by an errgroup child's **post-body** context check;
direct deposit/attestation errors have different wrappers
(`proposer_deposits.go:30–66`). It does not identify the last child, establish
the pool size, or measure CPU spent inside `packAttestations`. The earlier
eth1 warning precedes an unbounded head-data read, and final signature filtering
performs additional head/fork-choice reads. Pool locks and these reads lack
context-aware lock acquisition. None has a historically recorded lock owner.

Node 85 imports blocks 15, 16, and 17 before its old proposal branch finishes
(`beacon.log:550,553,558`). Each import's report crosses `HeadSlot` before
logging, so one head writer holding the read lock out continuously throughout
the warning-to-error interval is excluded. This does not rule out shorter lock
waits or an old reader being runnable but not scheduled. Node 35's events-stream
overflow at `01:32:39.320775417` (`:524`) is outside its completed proposal
interval and cannot time a packing dependency.

The exact code problems supported here are unnecessary quadratic processing of
raw disjoint singles, cancellation checks absent from the dominant loops, and
waiting for that branch after cancellation following successful payload
selection. Real
count-scan interference is a demonstrated upstream amplifier. Attribution of
either owner's entire delay to that amplifier, one pool shape, or one lock
still exceeds the historical measurements.
