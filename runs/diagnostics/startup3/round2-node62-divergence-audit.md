# Round 2 node 62 divergence at slot 117

## Finding

Node 62 imported the slot-117 consensus block but never processed its execution
payload envelope.  The local evidence rules out an Engine API rejection of that
envelope: there is no `engine_newPayloadV5` request for slot 117 or payload hash
`0x0227c072...` in the complete snooper log.  The execution client remained at
the preceding slot-116 payload and answered its ordinary polling promptly.

That missing envelope is sufficient to explain why this node stopped importing
the ordinary slot-118-and-later descendants and eventually selected the old
slot-110 fork.  Exact round-2 gossip validation queues a Gloas block when its
parent payload envelope has not been seen and asynchronously requests that
envelope.  The logs do not reveal why neither gossip nor that recovery path
delivered slot 117's envelope, so the defensible terminal boundary is “envelope
absent before Engine API,” not “EL rejected it” or “network dropped it.”

## Local timeline

Node 62 was complete through slot 116:

- slot 115 consensus block and envelope were logged at `01:53:00.808` and
  `.834` (`beacon.log:963-965`);
- slot 116 consensus block and envelope were logged at `01:53:14.153` and
  `.165` (`:968-970`); and
- the slot-116 execution payload hash was `0x1c9360dd...`.

At slot 117, it logged the consensus block `0xde9cfb98...` at
`01:53:26.799` and the state-transition summary identified the expected new
payload hash `0x0227c072...` (`beacon.log:971-972`).  There is no corresponding
`Synced execution payload envelope` row after it.  Nine seconds into the slot,
the attached VC explicitly submitted its payload attestation for block
`0xde9cfb98...` with `payloadPresent=false` (`validator.log:2920`).  This is a
positive observation of local unavailability, not an inference merely from a
missing INFO line.

The VC then repeatedly logged `Skipping payload attestation: no block for slot`
for slots 118 onward while its consensus attestations remained on the slot-117
head (`validator.log:2922-2936`).  At the wall-clock start of slot 121,
`01:54:12.005`, the BN reorged from slot 117 to slot 110 root
`0xf287cca3...`, with common ancestor slot 109 (`beacon.log:977`).  It later
imported the competing slot-123 block `0x4c0f3040...` and its envelope normally
at `01:54:36.781/.827` (`:979-981`).

## Engine-side discriminant

The snooper's first Engine interaction after importing the slot-117 consensus
block is an `eth_getBlockByNumber(latest)` poll at `01:53:31.538`; it returns in
1 ms and reports hash `0x1c9360dd...`, block number `0x4f` (79), slot number
`0x74` (116), and parent beacon root `0xd191d948...`
(`snooper-engine.log:50510-50555`).  There is no occurrence of payload hash
`0x0227c072...`, slot number `0x75`, or an Engine new-payload request for slot
117 anywhere in that log.

The BN's periodic “EL client is syncing”/follow-distance errors at
`01:53:31.539`, `:45.539`, and `:59.542` (`beacon.log:973-975`) therefore
describe the consequence of the EL remaining at block 79.  They are not an
Engine response to the missing envelope.  By contrast, when node 62 receives
the slot-123 envelope, it calls `engine_newPayloadV5` at `01:54:36.821`; the EL
returns `VALID` in 2 ms with the same hash, followed by a successful
forkchoice update in 1 ms (`snooper-engine.log:50810-50875`).  This makes a
general slow or non-responsive local EL a poor explanation for the gap.

## Exact Gloas dependency

At round-2 revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`, block
gossip validation checks `validateExecutionPayloadBidParentSeen` before full
block validation.  If the parent payload is unavailable, it verifies the
pending block signature, places the child in the pending queue, starts
`requestPayloadEnvelope(parentRoot)`, and returns `ValidationIgnore`
(`beacon-chain/sync/validate_beacon_blocks.go:195-210`).

Thus a node may know and transition the slot-117 consensus block while lacking
its separate envelope, but an ordinary slot-118 child whose bid depends on the
full slot-117 payload does not proceed through normal gossip import.  The same
condition can recur for later descendants until the missing parent envelope is
recovered.  Debug logging for this queue branch was not enabled in the
historical archive, so the absence of its message is not contrary evidence.

Fork choice also represents empty and full payload nodes separately.  The
best-descendant traversal follows the selected payload-content node
(`beacon-chain/forkchoice/doubly-linked-tree/gloas.go:249-273`), so losing the
slot-117 full node changes which descendants are reachable in that node's
local fork-choice view.  The logs show the resulting import gap and reorg, but
do not expose every internal pending-queue or payload-node transition.

## Causal boundary

The historical evidence supports this chain:

`slot-117 consensus block imported` → `slot-117 envelope locally absent` →
`VC votes payloadPresent=false` → `full-parent-dependent descendants not
imported` → `head falls back to the slot-110 fork` → `slot-123 fork imported`.

It does not distinguish whether the slot-117 envelope was never received on
gossip, rejected before the subscriber called the blockchain receiver, or
unavailable from peers when requested.  No `newPayload` call means it cannot
support an EL `SYNCING`/`INVALID` response as the initiating cause.  Xatu queue
overflow and exporter timeouts in the same interval concern telemetry delivery
and do not prove consensus-network packet loss.  No reproduction, download, or
production-code change was made for this audit.
