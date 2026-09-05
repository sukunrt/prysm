# Round 2 slot-129 rollback

At the slot-129 tick (`01:55:48 UTC`), detailed observers 400, 201, and 1 all
made the same head change: slot 128 root `0x4a3a9ee7…` to slot 117 root
`0xde9cfb98…`, depth and distance 11. The log anchors are node400 line 714684,
node201 line 718797, and node1 line 1576972. This was therefore a deterministic
fork-choice result from common vote input, not one observer discovering an
eleven-block competing chain at that instant.

## Vote input at the boundary

After stripping ANSI field decoration, each of those three ledgers contains
exactly 512 accepted Goldfish votes for vote slot 128:

| observer | `0x4a3a9ee7…` (slot 128) | `0x4c0f3040…` (slot 123) |
| ---: | ---: | ---: |
| 400 | 120 | 392 |
| 201 | 120 | 392 |
| 1 | 120 | 392 |

These are validation-accepted records, not an insertion counter or a
fork-choice store dump: the pubsub subscriber calls
`ReceiveAvailableAttestation`/`InsertAvailableAttestation` after validation
returns. All 512 were accepted early: node400's `decidedMs` range is
1263–3171, node201's 1423–3319, and node1's 1877–4124. Normal subscriber
processing therefore has roughly eight seconds or more before the next tick.
The score reconstruction below uses that recorded split as the input; the
identical historical head change and focused production-code test corroborate
the mechanism without supplying a missing historical insertion snapshot.

The 512 validators are not a shuffled cross-section. The mock committee code
chooses a contiguous range from a SHA-256-derived offset. Matching every
validator index against all archived `Validator activated` records gives an
exact two-wallet partition: node61 owns indices 89276--89395, all 120 votes for
`0x4a3a…`; node62 owns indices 89396--89787, all 392 votes for `0x4c0…`.
This concentration is amplified again in the VC: `getAvailableAttestationData`
does one BN RPC on the first cache miss and caches the response by slot for all
local validators (`validator/client/validator.go:825-862`). Node62 therefore
did not independently query the head 392 times; one stale BN answer was signed
by every committee member in that wallet.

Both roots were real, widely imported blocks. The all-archive union records
slot 123 root `0x4c0f3040…` at 204 nodes and slot 128 root `0x4a3a9ee7…` at
203 nodes. Node400 imported them at 01:54:36.881 (line 683032) and
01:55:37.049 (line 712935), respectively. Thus the 392-vote side is a stale
head vote, not a vote for an unknown block.

## Exact fork-choice trigger

`goldfishHead` reads only the previous slot's available-attestation votes, so
the slot-129 calculation reads vote slot 128. For the observed accepted split
after insertion, `goldfishScoresForSlot` sets the strict-majority threshold to
`512/2 = 256`. Only 120 votes name the current
slot-128 tip; 392 name the older slot-123 head. The current-slot passthrough
that allowed the slot-128 proposal to be head during its own slot no longer
applies at slot 129. The walk restarts from the justified node and admits a
node or payload branch only when its score is strictly greater than 256.

The relevant implementation is
`beacon-chain/forkchoice/doubly-linked-tree/goldfish.go`:

- `goldfishHead` selects `current-1` and starts at the justified node;
- `goldfishScoresForSlot` computes half of all recorded seats;
- `creditGoldfishVote` separately credits consensus nodes and empty/full
  payload branches;
- `goldfishNodeViable` and `goldfishPayloadViable` require score `> threshold`;
- `goldfishDescend` stops at the last node whose next payload/child clears the
  gate.

The wallet logs supply the missing viability discriminator. Node61 had imported
slot 128 at 01:55:37.063. Its slot-128 attestation submission names head
`0x33b63b5c…` (slot 127), source root `0xde9cfb98…`, source round 15, and its
payload attestations name `0x4a3a…`. Node62 instead logs three `no block for
slot=128` payload skips and submits all of its slot-128 attestations for
`0x4c0f…`, with the much older source root `0x611a8f2c…` and source round 13.

The archive lineage supplies the decisive ancestry discriminator. Slot 123 was
not a stale ancestor of slot 128. Its owner node158 built it on head slot 110,
root `0xf287cca3…`: FCU at 01:54:27.053, build completion at +0.471, import at
+0.727. Its execution parent `0x3d8b7508…` matches slot 110. It is therefore a
fork outside the slot-117 justified subtree on which slot 128 was built.

`creditGoldfishVote` can only credit ancestors reached while walking a named
root toward the justified node. The 392 votes for the slot-110/123 fork count
in the global 512-seat denominator, but credit no descendant of justified
slot117. The viable slot-128 side receives only 120, below the strict threshold
256. Thus no continuation below slot117 passes the gate and the walk returns
slot117. The old source round 13 on node62 is consistent with this fork and
would also constrain `leadsToViableHead`, but ancestry plus the exact scores is
already sufficient; viability need not be assumed to explain the result.

This is unrelated to retention expiry at that tick. Vote slot 128 is the exact
slot read by the slot-129 walk; pruning retains three slots and runs after the
slot summary. Nor is it ordinary LMD-GHOST accumulated weight: the reorg log's
old and new weights are both zero, while Heze activates the Goldfish walk.

## What the evidence does not establish

The immediate origin of the split is also observed: all 392 stale votes came
from node62's single wallet/BN, which had neither slot 128 nor a post-round-13
source checkpoint when it formed its duties; all 120 fresh-side committee votes
came from node61. Why node62's BN remained at slot 123/source round 13 is the
next unresolved boundary. Its VC evidence rules out independent behavior by
392 machines and exposes the mock contiguous committee as the amplifier.

## Algorithm proof

`TestGoldfishWalk_Round2Slot129Rollback` builds the observed shape in the
production fork-choice package: justified slot 117, a current branch through
128, and a slot-110-to-123 fork outside that subtree. At slot 128 the round
proposal makes 128 head. With 392 old-fork and 120 current-branch seats, the
slot-129 head is 117. The counterfactual containing only the 120 current votes
keeps head 128. Both cases use `payloadPresent=false`, demonstrating that no
particular payload-status choice is needed for the retreat.

```text
GOCACHE=/tmp/prysm-diagnostic-buildcache go test \
  ./beacon-chain/forkchoice/doubly-linked-tree \
  -run TestGoldfishWalk_Round2Slot129Rollback -count=1
ok  github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree
```
