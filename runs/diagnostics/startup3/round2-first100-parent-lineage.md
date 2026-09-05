# Round 2: produced blocks versus the retained first-100 ancestry

## Result

The earlier statement that every slot 15–100 had a block was a production/import
census, not evidence that each block remained on the chain. Envelope ancestry
and early Goldfish reorgs expose a second distinction before slot 128.

For slots 1–100:

- **14 slots have no recorded block:** 1–14.
- **19 blocks were produced but bypassed by the continuing branch:**
  16–20, 34, 39, 45, 51, 65, 68, 70, 74, 75, 78, 81, 82, 97, and 100.
- **67 blocks lie on that branch's ancestry:**
  15, 21–33, 35–38, 40–44, 46–50, 52–64, 66–67, 69, 71–73, 76–77,
  79–80, 83–96, and 98–99.

Slot 0 is genesis. Slot 101's parent is used only to resolve the fate of
slot 100 at its next boundary, not to analyze later failures. This is the
ancestry of the observed continuing branch, not a claim about final canonical
history after the explicitly excluded later collapse.

## Independent parent evidence

The Engine snooper records each `engine_newPayloadV5` request as JSON. Its
first argument contains `slotNumber`; its third argument, `params[2]`, is the
full **parent beacon block root**, not the EL `parentHash`.

Exact-R2 source revision `0280403c70d88967f49d2d4c730f4c5417dabdf5` supplies the
meaning of this field:

- `beacon-chain/blockchain/receive_execution_payload_envelope.go` passes
  `envelope.ParentBeaconBlockRoot()` to `ExecutionEngineCaller.NewPayload`.
- `beacon-chain/execution/engine_jsonrpc.go` sends that root as argument three
  of `engine_newPayloadV5`.
- `beacon-chain/core/gloas/payload.go`, `validatePayloadConsistency`, checks
  that it equals the state's latest beacon block header's `ParentRoot`.
- `proposer_payload_envelope.go` initially constructs the envelope using the
  built beacon block's `ParentRoot`.

Snoopers for **nodes 1, 201, and 400 independently agree** on all 86 observed
slot-parent pairs between 15 and 101. None contains an envelope-19 new-payload
call. Full parent roots are recorded; matching to the block census uses its
unique logged root prefixes, corroborated by full-root reorg/Goldfish records.
No inconsistent parent or prefix assignment was observed.

The decisive nonconsecutive links are:

| Child slot | Parent slot | Consequence |
| ---: | ---: | --- |
| 15 | 0 | First non-genesis block. |
| 16 | 0 | Sibling of 15, not its child. |
| 17, 18, 20, 21 | 15 | Several competing children; continuing branch proceeds through 21. |
| 35 | 33 | Bypasses 34. |
| 40 | 38 | Bypasses 39. |
| 46 | 44 | Bypasses 45. |
| 52 | 50 | Bypasses 51. |
| 66 | 64 | Bypasses 65. |
| 69 | 67 | Bypasses 68. |
| 71 | 69 | Bypasses 70. |
| 75, 76 | 73 | Continuing branch bypasses 74 and 75. |
| 79 | 77 | Bypasses 78. |
| 82, 83 | 80 | Continuing branch bypasses 81 and 82. |
| 98 | 96 | Bypasses 97. |
| 101 | 99 | Bypasses 100 at the next slot boundary. |

For example, node400 `snooper-engine.log:32306–32329` records slot15's
new-payload request and genesis parent. Its slot16 request begins at line32601.
The slot101 request is timestamped `01:50:16.987924677Z` and names slot99 root
`0x4059d3fba97aed35f8b821796903914a723def0f5c067e4c7203c2623d2a0f2a`.

## Why production alone was not enough

Available attestations are due at one quarter of a slot: **3 seconds** for
this configuration. Timely-block notification may wake the voter earlier.
This is a scheduled wakeup, not a hard block-validity cutoff; actual vote-data
requests can run later because of scheduling.
The VC caches one returned head view per slot for its local committee keys.
At the next slot, Goldfish scores the preceding slot's votes; a block that
was built before the 12-second proposer deadline can nevertheless lack the
strict-majority support needed to remain head.

Node400's validation-accepted ledger supplies these examples:

| Vote slot | Votes for that slot's block | Other votes | Next-boundary outcome |
| ---: | ---: | --- | --- |
| 15 | 54 | 458 for genesis | Temporary retreat to genesis; 15 later regains support. |
| 16 | 23 | 489 for 15 | Head selects 15 instead of sibling 16. |
| 17 | 112 | 400 for 15 | Retreat to 15. |
| 18 | 0 | 512 for 15 | Retreat to 15. |
| 19 | 0 | 512 for 15 | Late block has no current-slot voter support. |
| 20 | 0 | 257 for 15; 255 for 18 | With the full observed cohort retained, neither child passes >256; the observed walk stops at 15. |
| 21 | 506 | No other accepted root in this observer | Next retained child of 15. |
| 34 | 0 | 512 for 33 | Retreat to 33. |
| 51 | 44 | 468 for 50 | Retreat to 50 despite early first import elsewhere. |

Slots 39, 45, 65, 68, 70, 74, 75, 78, 81, 82, 97, and 100 likewise have
512 votes naming an older block in this observer. The matching reorgs occur
at the following slot boundaries. These are **validation-accepted** records,
not a dump of the fork-choice store after subscriber insertion. They were
accepted well before the relevant next ticks; source semantics and the
observed parent/reorg outcomes corroborate the mechanism.
The threshold is computed from votes actually present at the deciding node;
the 512-seat accepted cohort is not itself proof that this store had all 512
inserted at the head decision. The logs do not independently identify the
precise rejection branch for every node.

Thus there are two separate clocks: a proposal can beat 12 seconds but miss
the actual voters' head-selection time. An early import somewhere also does
not prove that the committee's serving nodes imported the block in time.

See [the complete 101-row proposal timeline](round2-slots-0-100.md) and
[the cross-observer vote audit](round2-first100-vote-retention.md).

## Slot 19 additionally loses its self-built envelope

Node142 finishes constructing block19 at +11.9458 seconds, leaving about
54 ms before its VC deadline. The VC logs `Failed to propose block` at
+12.0010 seconds and returns. The block nevertheless reaches 999 other
nodes, first at +12.3456 seconds; node142 itself has no import record.

The exact VC `ProposeBlock` ordering in `validator/client/propose.go` is:
submit consensus block, return immediately on RPC error, then retrieve/sign/
publish the self-built execution envelope only on success. The BN broadcasts
the consensus block before its local `ReceiveBlock` finishes, and waits for
that reception result before returning RPC success. This allows gossip
publication to succeed while the caller times out and never attempts envelope
publication. No slot19 envelope request reaches the three detailed EL
snoopers above, consistent with this directly observed early-return path.

The block was already too late for its slot's Goldfish vote, independently of
this envelope failure. The missing envelope is a second consequence of the
same near-deadline build/submission ordering, not an explanation for its earlier
build delay and not evidence that the EL rejected it.
