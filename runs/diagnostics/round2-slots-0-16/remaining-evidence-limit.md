# What is and is not identified

The [slot report](explanation.md) establishes every observed proposal outcome
and the necessary dependency at which each failed. The investigation has also
reproduced upstream code causes using finite real work, rather than stopping
at those terminal dependencies:

- [Payload retrieval](payload-code-root-cause.md): repeated genesis counts
  delay the real HTTP response reader beyond the 300 ms budget. A separate
  error-translation defect suppresses the cached-payload recovery branch.
- [Sync discovery](sync-index-count-realwork-results.md): the same counts
  prolong the global multilock queue, including on a warm sync-state cache
  hit. A no-snapshot, no-trace pair measures a 13.572-second maximum versus
  11.833 milliseconds with memoized counts; a separate trace locates the
  actual queue waits before and after the cache hit.
- [Domain lookup](domain-http2-reader-trace-results.md): the retained H trace
  now identifies the ordinary RANDAO connection reader waiting runnable for
  135.385 ms, then creating the matching handler. All 53 native CPU samples
  during that wait execute the genesis-count path. The actual RPC falls from
  139.232 to 1.636 ms with BN3's counts removed. E1 separately records
  [2.162415 seconds before an ordinary DomainData body](domain-e1-prebody-envelope-results.md),
  without a native trace of that interval. The newer
  [bounded full-gossip composition](full-gossip-domain-writer-results.md)
  measures a 6.856-second RPC without profiles/tracing, including 4.781 seconds
  before server admission; the matched stable-registry control stays below
  3.299 ms. Its separate trace also
  [identifies a natural finalizer releasing 101 Count readers](full-gossip-finalizer-cohort-trace-audit.md)
  together, with first-execution delays reaching 1.918 seconds. The finalized
  object's origin and the corresponding historical interleaving are unrecorded.
  These locate a real ordinary-RPC mechanism and reproduce its
  seconds-scale effect. Smaller compositions,
  including the [four-thread full-gossip pair](full-gossip-domain-p4-results.md),
  do not reproduce the deadline. A subsequent
  [matched omission pair](full-gossip-domain-no-timed-attdata-results.md)
  stays below 7.493/10.543 ms with the timed attestation request omitted or
  enabled; neither arm forms the earlier large count cohort. It does not
  identify that request as a necessary trigger. Node 169's own admission and scheduling
  timestamps remain unlogged. Slot 4's later RANDAO dispatch is already expired
  by its preceding synchronous preflight error.
- [Packing](packing-compaction-realwork-results.md): identical signed votes
  require 5.112 seconds raw versus 1.624 seconds including real compaction;
  raw work notices its 300 ms cancellation only after 5.139 seconds.
- [Parent preparation](slot13-parent-dependency-realwork-results.md): actual
  checkpoint/count work delays `UpdateHead` for 11.074 seconds, after which
  slot processing returns the historical cancellation category in 13 us.
  Memoized counts allow preparation in 35.377 ms. A separate trace proves a
  fork-choice writer waiting for a checkpoint reader, while finishing within
  budget on that particular repetition.

These measured mechanisms do not recover every underlying historical wait.
The table identifies the specific missing observations; it is not a list of
untested preferred explanations.

The subsequent [completion audit](completion-audit.md) checks the full original
scope against these results and revalidates the archive inventory. Exact
upstream attribution for every historical owner remains unmet.

| Slots | Established causal link | Unrecorded fact that prevents a more specific historical attribution |
| --- | --- | --- |
| 0 | Genesis is explicitly skipped by the proposer. | None for this outcome. |
| 1 | Failed RANDAO domain lookup returns before the block request. | Proposer entry, role-lock acquisition, domain-cache lock acquisition, RPC admission/return, retry and scheduler timing. |
| 2, 3, 7, 11, 12 | Synchronous role discovery has not returned by the deadline; proposer dispatch retains that expired deadline; RANDAO fails before a block request. | How much budget was already spent before sync-index entry, and where the remaining call was delayed. |
| 4 | Sync-selection signing/domain preflight has not returned by the deadline; expired proposer dispatch then fails RANDAO. | Entry and wait timing in preceding work, the domain cache and the RPC path. |
| 5, 6, 8, 9 | Local payload retrieval times out despite prompt matched proxy responses; absence of a P2P bid is logged; the builder returns without a usable payload. | Actual BN response receipt/consumption, scheduling and the precise timeout return site. Slots 6/9 return this failure before the whole-slot deadline. |
| 10, 14 | A usable payload is selected but the necessary consensus result remains unavailable at the deadline; after its late completion, canceled state-root processing terminates the build. | Packer entry, head-access duration, pool contents, deposit/attestation child timing and scheduler state. |
| 13 | Parent-state preparation reports cancellation from the slot-processing path; block assembly and payload retrieval are never reached. | Division of time before, within and after slot processing, including head access and the skip-slot cache. |
| 15, 16 | Both blocks are built and imported by all 1,000 recorded nodes. Both are genesis children. Recorded cohorts and the real fork-choice replay explain the observed 15 → genesis → 16 → 15 sequence under those inputs. | Exact historical fork-choice store contents at the deciding instruction; internal timing of the voter owners' earlier head observations. |

The complete selected archive union contains five log members per node. The
additional folders containing packet captures and CPU/runtime profiles belong
to separate local experiments; their configurations, genesis and run identity
were checked before using them as mechanism evidence. They cannot supply the
missing observations for these historical owners.

Three completed differentials demonstrate why more certainty cannot be
extracted from the existing terminal messages:

1. In the real client, a live sync-index call that consumes the budget and a
   preceding singleflight wait followed by an immediately expired sync-index
   call produce the same final sync-index/RANDAO failure stages. The historical
   logs lack the entry markers that distinguish these cases.
2. In the real builder, an empty-pool fixture with a controlled pre-packer
   head dependency produces the late canceled state-root path. The previous
   loaded-packer experiments also delay consensus completion. A late packing
   cancellation therefore does not identify historical pool size or cost.
3. In the real slot-13 parent preparation, an earlier blocked head dependency
   and an observed in-progress skip-slot-cache wait produce the same wrapped
   slot-processing cancellation. The wrapper identifies error observation,
   not which earlier dependency consumed the budget.

These are source-real mechanisms with controlled dependencies, not restored
historical runtime schedules. They demonstrate ambiguity in the available
observations; they do not establish that any chosen alternative occurred.
Repeating a successful mechanism experiment would not recover the missing
historical measurements.
