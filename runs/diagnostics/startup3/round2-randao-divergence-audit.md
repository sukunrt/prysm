# Round 2 later-slot RANDAO divergence audit

## Conclusion

The invalid-RANDAO failures at slots 160, 166, and 210 are direct evidence of
divergent proposer selection, not evidence of corrupt keystores.  In all four
rejected proposals, the validator client signed the reveal with the key in its
own duty, while its attached beacon node built and checked the block using a
different proposer index.  The public key printed by signature verification is
the key belonging to another archived validator client, and at slots 160 and
166 it is exactly the competing duty recorded by the archive census.

| slot | submitting VC | signing/duty key | BN-selected validator/key | independent evidence |
| --- | ---: | --- | --- | --- |
| 160 | 145 | `0x8e7eb21275c4` (index 37824) | index 18020, `0xb1fdba074a34` | node 31 recorded `0xb1fd...` as its slot-160 duty |
| 166 | 62 | `0xab3865e52e4b` (index 89938) | index 100155, `0xb6d6335bc202` | node 135 recorded `0xb6d6...` as its slot-166 duty |
| 166 | 147 | `0x84cc85c20bed` (index 39067) | index 100155, `0xb6d6335bc202` | same competing node-135 duty |
| 210 | 18 | `0x873868060f7b` (index 10603) | index 66342, `0xae2c7595e0f1` | that key is activated in node 60's wallet |

The validator indices/key ownership come from the respective `Validator
activated` records: node 145 `validator.log:191`, node 31 `validator.log:389`,
node 62 `validator.log:101`, node 135 `validator.log:309`, node 147
`validator.log:207`, node 18 `validator.log:519`, and node 60
`validator.log:553`.  The duty rows and terminal failures are also retained in
`/tmp/round2-later-union.json`.

## What the failure text proves

The rejected node-145 proposal reports `pubkey=0x8e7e... slot=160`, but the
embedded signature diagnostic says it was checked with public key
`0xb1fd...` (`validator.log:4412`).  Its beacon log independently says the
slot-160 block was finished with `validator=18020` (`beacon.log:1015`), the
index of `0xb1fd...`, not the index of node 145's signing key.

Both rejected slot-166 proposals similarly identify their callers as
`0xab38...` and `0x84cc...`, while both signature diagnostics name
`0xb6d6...`.  Node 62's beacon log makes the state-dependent choice explicit:
it moves head 123 to head 117 at `02:03:12.012` (`beacon.log:1147`), begins
building eight milliseconds later (`:1148`), and finishes the block with
`validator=100155` (`:1153`).  Node 147 also moves head 128 to 117 at the slot
boundary (`beacon.log:1006`) and finishes with validator 100155
(`beacon.log:1010`).  Node 135, which owns validator 100155, records the
competing slot-166 duty, although its own block request reaches the deadline.

At slot 210, node 18's failure identifies its signing key as `0x8738...` and
the verification key as `0xae2c...` (`validator.log:5712`).  Its beacon node
logs `validator=66342` when construction finishes (`beacon.log:1176`), exactly
the activation index of node 60's `0xae2c...` key.  This third independent
mapping makes a one-off bad key or damaged signature encoding implausible.

The two epoch-5 failures (slots 160 and 166) have the same logged signing
message, while slot 210 (epoch 6) has the expected different message.  Thus the
observations fit the epoch-derived RANDAO object and do not point to an
incorrect slot-to-epoch conversion.

## Exact round-2 code path

The following behavior is present at round-2 revision
`0280403c70d88967f49d2d4c730f4c5417dabdf5`:

1. `validator/client/propose.go:62-97` computes the epoch from the duty slot,
   signs the RANDAO reveal with the duty pubkey, and sends only slot/reveal and
   other block parameters in `BlockRequest`.  The request does not bind a
   validator pubkey or expected proposer index.  `signRandaoReveal` at
   `:399-427` signs the epoch root in `DOMAIN_RANDAO` with that pubkey.
2. `beacon-chain/rpc/prysm/v1alpha1/validator/proposer.go:80-107` obtains the
   beacon node's parent state, copies the caller's reveal into the block, then
   independently calls `helpers.BeaconProposerIndex(ctx, head)` and sets that
   locally derived index on the block.  It does not first establish that the
   supplied reveal belongs to this locally selected validator.
3. During block transition,
   `beacon-chain/core/blocks/signature.go:132-164` again derives the proposer
   from the transition state, selects that validator's pubkey, derives the
   epoch and RANDAO domain from the same state, and adds the reveal to the
   signature batch.  The logged `public key` is therefore the BN/state-selected
   verification key, not a key inferred from the incoming signature.

Consequently, if the VC duty view says validator A while the serving BN's
parent-state view says validator B, the existing protocol flow constructs a
block with B's proposer index and A's reveal.  It can build successfully but
must fail when transition signature verification checks the reveal against B.
That is exactly the observed key pairing in all four failures.

## Causal boundary

The key mismatch proves that divergent state-derived proposer schedules are
the immediate cause of these invalid-RANDAO rejections.  The surrounding logs
strongly connect that divergence to the post-slot-129 fork-choice instability:
nodes repeatedly alternate among heads 117, 123, 128, and later sparse branch
heads, and slot 166 shows head changes immediately before construction.

It does **not** identify which fork-choice input or earlier defect originally
caused the rollback/divergence, nor does the 205-archive union establish a
canonical branch after slot 129.  Slot 210 has no second duty row in the
available census, but the BN-selected key/index mapping to node 60 still proves
the local VC/BN schedule disagreement.  This audit made no network requests,
ran no reproduction, and changed no production code.
