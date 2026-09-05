# Round 2 slot-129 rollback: ancestry and viability audit

## Result

The 392 votes for slot-123 root `0x4c0f3040...` did not support the descendants
of justified root `0xde9cfb98...` at slot 117.  Slot 123 was built on an old
slot-110 fork.  The 120 votes for slot-128 root `0x4a3a9ee7...` were the only
recorded available-attestation votes for the descendant branch rooted at slot
117.  Because the threshold denominator still included all 512 seats, that
branch had 120 votes against a strict `> 256` gate.  The walk therefore
returned its starting justified node, slot 117.

This corrects the tempting but wrong interpretation that 392 votes for an
older *slot* necessarily supported the current chain.  Slot number does not
establish ancestry here.

## The slot-123 fork was outside the slot-117 subtree

The slot-123 owner's own beacon log supplies the construction ancestry.  Node
158 reorged from slot 116 to root `0xf287cca3...` at slot 110
(`beacon.log:943-944`), then prepared slot 123 with
`blockRoot=0xf287cca3... headSlot=110 nextSlot=123` at `01:54:27.053`
(`beacon.log:946`).  It began building at `01:54:36.053`, finished at `.474`,
and imported slot-123 root `0x4c0f3040...` at `.727` (`:947-951`).  Its
finalized round was still 12, whereas the ordinary observers had reached
round 13.

The later immutable-root reorg evidence independently locates the fork point.
For example, node 62 reports a transition between slot-123 root
`0x4c0f3040...` and slot-117 root `0xde9cfb98...` with common ancestor
slot 109/root `0x8f962346...` (`beacon.log:1139` in its archive).  Therefore
the slot-123 root cannot be a descendant of slot 117.  By contrast, the
slot-129 rollback from slot-128 root `0x4a3a9ee7...` to `0xde9cfb98...`
reports slot 117 itself as the common ancestor throughout the archive census.

## Exact scoring consequence

At round-2 revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`,
`goldfishScoresForSlot` in
`beacon-chain/forkchoice/doubly-linked-tree/goldfish.go:275-296` counts every
recorded seat into the denominator before calling `creditGoldfishVote`.
`creditGoldfishVote` at `:299-333` credits a vote to its named node and walks
that node's actual ancestors.  It stops if it reaches the justified node, but
a vote on a sibling fork cannot add score to a child of that justified node.

Using the recorded 512 validation-accepted messages as the input to the
slot-129 calculation:

- total seats and threshold were 512 and 256;
- the 392 votes for `0x4c0f3040...` walked the slot-110 fork, not the
  slot-117/128 branch;
- only the 120 votes for `0x4a3a9ee7...` credited the child path below slot
  117; and
- `goldfishNodeViable` requires a non-current node score strictly greater than
  the threshold (`goldfish.go:335-348`).

The payload-ancestor credit at `goldfish.go:330` has the same ancestry
property.  This is sufficient to explain why the walk could not descend from
slot 117 without knowing the votes' `payloadPresent` split.

Acceptance is logged before subscriber insertion, not after it. These messages
were accepted well before the tick, and the historical output matches the
production-path diagnostic, but the archives contain no serialized insertion
snapshot. The reconstructed exact 512-seat calculation uses the observed
accepted cohort as its input rather than claiming such a snapshot exists.

## The separate fork-choice viability predicate

`goldfishBestChild` does have an earlier eligibility filter:
`child.leadsToViableHead(justifiedRound, currentRound)` at
`goldfish.go:407-423`.  `leadsToViableHead` checks the child's best descendant,
or the child itself, with `viableForHead`; at this revision that accepts a node
when the store justified round is zero, the node's justified round equals the
store's, or `node.justifiedRound + 2 >= currentRound`
(`beacon-chain/forkchoice/doubly-linked-tree/node.go:15-32`).

That predicate means vote score alone is never a complete description of the
algorithm.  However, the archived logs do not expose the internal
`justifiedEpoch`/best-descendant fields for the first child below slot 117, so
they do not prove that this viability filter rejected that child.  It is also
not needed to explain this rollback: even if the slot-117/128 child was viable,
its ancestry-scoped score was only 120 and failed the subsequent majority
gate.  Conversely, the 392-vote sibling branch was unreachable because the
Goldfish descent starts at the justified node.

## FFG metadata is consistent but not an internal-node dump

A bounded scan of the 205 available validator archives shows that at slot 128,
199 clients submitted FFG attestations with source root `0xde9cfb98...`, source
round 15, target root `0x33b63b5c...`, and target round 16.  At slot 129 the
population split: 97 submissions retained source `0xde9cfb98...`/round 15 and
head `0x4a3a9ee7...`, while 101 used source `0x611a8f2c...`/round 13 and head
`0xde9cfb98...`.  Node 151's slot-128 record is one example of the former.

Those rows establish heterogeneous VC/BN checkpoint views around the rollback,
but they are signed-attestation fields, not direct observations of fork-choice
node `justifiedEpoch` or `bestDescendant`.  They therefore must not be used to
claim the source-level viability predicate fired.

## Causal boundary

The immediate rollback is source- and ancestry-explained: 392 seats were
counted in the majority denominator while voting on a branch that diverged
before the justified start node; the only reachable voted branch had 120 seats
and could not clear 256.  The evidence does not yet explain why so many
validators followed the stale slot-110-derived fork, nor why nodes converged
on it for their available-attestation vote.  Those are upstream causes of the
392/120 split.  No reproduction, network request, or production-code change
was made for this audit.
