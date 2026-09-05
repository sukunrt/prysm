# Round 2 slots 5–16: audit of build failures and first successful blocks

This is a read-only re-audit of existing reports, recovered owner logs, and
historical revision `0280403c70d88967f49d2d4c730f4c5417dabdf5`. No experiment,
network rerun, script, test, or production edit was performed for this audit.
The prior reports were read before rechecking the evidence. Their controlled
experiments are cited as prior results, not rerun or independently reproduced.

Raw owner references below mean member `./beacon.log`, `./validator.log`, or
`./snooper-engine.log` of
`/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-<node>.tar.gz`. Observer400
means `runs/round2/round2-prysm-geth-400.tar.gz`. Line numbers refer to the
unmodified member, including any ANSI formatting. Historical source was read
with `jj --ignore-working-copy file show -r <revision> <path>`; source numbers
refer to that revision, not necessarily the working copy. Genesis is
`2026-09-05T01:30:00Z`; `+s` denotes seconds after the relevant slot's start.

## What previous theories survive rechecking

| Earlier interpretation | Assessment against raw evidence and exact source |
| --- | --- |
| All timeouts mean slow geth payload construction | Rejected for 5/6/8/9: the corresponding proxy responses are successful and fast. Rejected for13: the proposal never reaches payload retrieval. Rejected for10/14: payload selection succeeds. |
| Late packing errors explain every failed build | Rejected for5/6/8/9: payload plus fallback failure returns before the join. Packing cancellation is aftermath. For10/14 the parallel consensus branch really is required. |
| The packing error is returned to the caller and aborts construction | Imprecise: `setPreGloasConsensusFields` catches it, fills empty deposits/attestations, logs it, and continues. The logged terminal error for10/14 is canceled post-state computation after the join. |
| Slots5/8 were principally lost to late VC dispatch | Too strong. Late BN handler entry is directly measured; earliest visible VC lines are PTC no-block output, not proposer-dispatch timestamps. Reduced remaining budget does not by itself prove failure inevitable. |
| Slot13's late FCU is its proposal handler waiting on geth | Rejected: it is an independent payload-preparation task while the proposal has not passed parent-state preparation. |
| Slot13 spent 10.721 seconds executing skipped-slot transition | Unsupported: this is entry-to-parent-error wall time, including pre-parent checks and other state/head/cache work. Cancellation can be observed at an already-canceled first cache check. |
| Node35/85 suffered proven quadratic packing or a pool deadlock | Unsupported historically. Input pool counts, per-stage timings, CPU profiles, and lock owners are absent. Both builds eventually proceed beyond the parallel branch. |
| Controlled scan-plus-packer reproduction proves each historical owner's exact cause | Overclaim. It proves sufficiency of those production paths under the controlled input; owner-specific historic transfer remains an inference. |
| Block15 starts a successful contiguous chain and block16 extends it | Rejected: both parent beacon roots are genesis. Node400 retreats15→0 at the start of16 and16→15 at the start of17. |
| Slot16's 23/489 split means it could never be head | Rejected: the distinguished round-start proposal can temporarily become head during16. The override expires at17. |
| Successful build/import proves finality | Rejected: the import records for15/16 explicitly still report finalizedSlot=0, finalizedRound=0, genesis finalizedRoot. |

## Exact source dependency paths

`beacon-chain/rpc/prysm/v1alpha1/validator/proposer.go:65` logs handler entry,
then `:67–78` performs syncing/optimistic checks, `:80–84` prepares the parent,
and `:85–114` constructs the empty block, sets fields and proposer, and invokes
`BuildBlockParallel`. Therefore “Building block” is neither proof of an entered
packer nor the start of active state-transition work.

For Gloas, `proposer_gloas.go:25–34` starts a parallel goroutine containing
`setPreGloasConsensusFields`, payload attestations, and parent execution requests.
The caller retrieves the local payload at `:38`. On error it tries P2P fallback;
if that also fails, `:42` returns **before** the join at `:85`. On success,
bid selection completes at `:77–82`, then `wg.Wait()` precedes post-state/root
computation at `:87–89`.

Within the parallel branch, `proposer.go:211–216` handles eth1data, `:218` calls
`packDepositsAndAttestations`, and `:219–229` swallows a packing error, fills
empty fields, and logs it. Remaining consensus fields follow at `:232–240`.
Thus a late packing log bounds visible branch progress; it is not a per-stage
CPU timer. The terminal10/14 error is produced when post-state computation
eventually reaches `handlePostBlockStateError`, whose `:681–682` explicitly
turns `ctx.Err()` into `codes.Canceled`.

`beacon-chain/execution/engine_jsonrpc.go:303–326` gives Gloas GetPayload its own
deadline and calls `CallContext`; its timeout constant at line 118 is 300 ms. This
deadline is nested under the proposal context. A getPayload failure at +4.457
or +7.639 therefore need not wait for the whole-slot +12 s deadline.

## Raw failure table

| Slot / node | Direct observed sequence, in seconds after slot start | Line anchors |
| --- | --- | --- |
| 5 /118 | Entry +10.119967; getPayload request +10.137762; response +10.141752; VC block RPC deadline +12.002351; BN payload timeout +12.011577; terminal no-fallback +14.527479; later packing cancellation +28.842353 | BN524–528,532; VC932–933,971; snooper31710–31720 |
| 6 /83 | Entry +3.664076; request +4.147146; response +4.148780; BN payload timeout +4.456845; terminal no-fallback +4.457181; later invalid-pool cleanup cancellation +5.006151 and packing cancellation +15.292456 | BN542–547,550; snooper31964–31974 |
| 8 /19 | Entry +11.191118; request +11.197293; response +11.197336; VC deadline +12.002693; BN payload timeout +12.007749; terminal no-fallback +12.052566; later packing cancellation +20.467728 | BN533–540,542; VC1402–1406,1419; snooper32102–32112 |
| 9 /107 | Entry +5.983283; request +6.109110; response +6.131275; BN payload timeout +7.638494; terminal no-fallback +7.638559; later packing cancellation +64.528148 | BN531–535,541; snooper32191–32201 |
| 10 /35 | Entry +7.446879; payload selected +10.338144; packing cancellation +36.873390; terminal canceled post-state/root computation +36.874620 | BN514,519,521–522 |
| 13 /20 | Entry +1.317155; independent FCU request +6.084531, response +6.085876; BN logs payload ID +10.342762; parent-state error +12.038077 names ProcessSlots cancellation; no slot13 getPayload | BN527–531; snooper32149–32180 |
| 14 /85 | Entry +2.694093; payload selected +2.744065; packing cancellation +43.993917; terminal canceled post-state/root computation +43.996259 | BN543–546,562–563 |

Outer log timestamps above occasionally differ slightly from embedded
`sinceSlotStartTime`. For example node83's terminal line embeds +4.455570 but
its outer timestamp is +4.457181. This audit preserves the outer values and
does not mix those clocks when subtracting intervals.

### Slots5/6/8/9: exact loss and remaining uncertainty

The request IDs and payload IDs link each request to its successful response.
Proxy duration fields are 3 ms, 2 ms, 0 ms, 22 ms respectively; each HTTP status is 200
with a 2060-byte payload response. Already-prepared payload IDs are visible at
node118 BN522, node83 BN540, node19 BN531, node107 BN529.

The gaps from proxy response header to BN timeout log are approximately 1.870 s,
0.308 s, 0.810 s, 1.507 s. These are not geth computation intervals, and the proxy
log is not a BN packet capture or decoder-completion event. It cannot isolate
buffer flush/delivery, Go transport/read/decode, scheduler service, timeout
selection, or log latency. Slots6/9 directly fail before the whole-slot
deadline. Slots5/8 enter late and fail around that deadline; their nested
300 ms timeout is also observed later than its nominal expiry.

For 5/8, VC932–933 and 1402–1406 say “Skipping payload attestation: no block for
slot”. They are other-role output near the block request; they do not record
proposer goroutine dispatch, RANDAO start, or RPC transmission. The measured
BN entries leave about 1.880 s / 0.809 s of slot budget. That is an adverse observed
condition, not proof that a timely response and the remaining build could not
have completed. Calling it the **primary cause** ranks unmeasured contributions.
The exact terminal path is payload timeout plus absent fallback.

The subsequent packing logs cannot retroactively explain those returned
failures. Source permits the parallel branch to continue after the early return.
It might also consume CPU concurrently before the return; historical logs do
not quantify that indirect contribution. “Not on the necessary return path”
does not prove “used no resources during the failure.”

### Slots10/14: required outstanding consensus branch, not measured packing CPU

Because payload selection succeeded, normal completion requires joining the
parallel branch. The observed cancellation/error sequence precedes the final
canceled post-state failure by only 1.23 ms / 2.34 ms. Payload-selection-to-packing-log
intervals are 26.535 s / 41.250 s. These are wall-time progress bounds, not active CPU
measurements or proof that every moment was inside attestation packing. The
branch also does eth1data, deposits, other consensus fields, payload attestations,
and execution requests; scheduling and logging also have no phase markers.

The prior full-packer controls are relevant: a synthetic production-valid 15k
candidate input took 5.4–5.7 s alone; adding the tested 15k slot-zero registry scans
produced 59–61 s returns after a 12 s context, whereas the cached-count control
returned about 5.3 s successfully. Those results demonstrate a sufficient
interference mechanism. They do **not** establish that historical node35 or 85
had those pool objects, scan counts, worker concurrency, or profile.

Accordingly, the final assertion in
`startup3/round2-packing-deep-audit.md:247–252` that “the historical root cause
can therefore be stated as global startup contention” is stronger than its
own immediately preceding limits. Supported formulation: **the required
parallel consensus branch failed to complete before cancellation; global
startup contention plus cancellation-sparse packing is an experimentally
demonstrated explanation consistent with, but not uniquely selected by, the
historical owner records.** A particular pool lock, deadlock, candidate count,
quadratic duration, or scheduler trace cannot be supplied from those logs.

### Slot13: cancellation observed in parent-state processing

`proposer.go:177–179` wraps
`ProcessSlotsUsingNextSlotCache(..., slot)` with exactly the terminal error
recorded at node20 BN531. This proves the slot-processing branch was reached
and failed before block assembly. Handler-entry-to-error is 10.720921 s; there
are no logged substage starts. It includes syncing/optimistic checks preceding
`getParentState`, `UpdateHead`, head/root/state access, cache lookup/copying,
and any processing. It cannot be charged entirely to `ProcessSlots`.

`transition.go:227–257` obtains a cache key, checks the skip cache, arbitrates
in-progress work, then invokes `ProcessSlotsCore`. `skip_slot_cache.go:76–77`
checks `ctx.Err()` before even inspecting whether another calculation is in
progress. Thus the eventual error is compatible with prior delay followed by
immediate cancellation observation, actual cache polling, or transition-loop
cancellation. Neither a historical cache leader nor a lock duration is known.

The separate FCU response is VALID at +6.086 s; its payloadID is logged by the BN
at +10.343 s. This is corroborating local-progress delay, not progress by the
blocked proposal. Exact ordering forbids its `getLocalPayload` stage before
the parent-state call succeeds. As the earlier engine audit correctly noted,
FCU-response-to-log time can include payload-cache/head locks after RPC return,
in addition to delivery, Go servicing, and logging. It is not a pure scheduler
measurement. Standalone historical-state transition benchmarks constrain the
intrinsic-cost hypothesis on the test host; they are not a timing breakdown
for this owner.

## Slots15/16: build success, publication, import, head, and finality

| Fact | Slot15 / node32 | Slot16 / node76 |
| --- | --- | --- |
| Handler entry | +0.009448, BN532 | +0.219198, BN717 |
| Self-build payload selected | +0.018811, BN535 | +0.703632, BN722 |
| Build complete | +1.482231, BN536 | +1.816691, BN724 |
| Own BN import complete | +1.650165, BN537 | +2.707439, BN725 |
| Example independent peer import | node400 +1.732493, BN27512 | node400 +2.059227, BN33222 |
| Envelope publication | +1.655162, BN539 | +6.771684, BN727 |
| VC “Submitted new block” | +1.658630, VC1324 | +6.773791, VC1846 |
| Parent beacon root | genesis; node400 snooper32323,32330 | genesis; node400 snooper32618,32625 |
| Included consensus fields | 8 attestations,0 payload attestations,0 deposits; VC1324 | 8 attestations,0 payload attestations,0 deposits; VC1846 |
| Execution contents | no transactions, gasUsed0; node400 snooper32318,32326 | no transactions, gasUsed0; node400 snooper32613,32621 |
| Finality at import | finalizedSlot0, finalizedRound0, genesis root; BN537 | finalizedSlot0, finalizedRound0, genesis root; BN725 |

The reported “Submitted” time is later than initial block availability and
follows envelope publication in these records. Slot16 was already imported by
node400 at+2.059s despite its VC summary at+6.774s. Neither is a missed block.

The parent fields above are the full third argument to `engine_newPayloadV5`,
not merely EL `parentHash`. They independently show genesis siblings. Node76
itself logs15→0 reorg while preparing16 (BN719 at16+0.447770), then16→15 at the
next boundary (BN730 at17+0.071683). Node400 logs the corresponding15→0 at
`01:33:12.005001737Z` (BN31677) and16→15 at
`01:33:24.008920727Z` (BN37320). The return to15 is directly observed. Calling15
retained means ancestry on the later continuing branch documented in the prior
lineage audit; it does not mean15 never lost head or was already finalized.

The prior vote census found 54 votes for 15 versus 458 genesis at vote-slot 15,
and 23 for 16 versus 489 for 15 at vote-slot 16, independently matching nodes201/400.
The census itself was not rerun here. Its owner-import discriminator was
rechecked in raw logs: node68 imports15 at+2.177131 (BN554), node69 at+12.466413
(BN562); node94 imports16 at+3.010779 (BN543), node93 at+5.069303 (BN637).
These corroborate uneven owner-local head availability. The scheduled+3s vote
wakeup is not an exact logged data-request time or a block validity cutoff.
Raw import completion cannot distinguish propagation from local processing.

There is an essential exception to a simplistic majority account. Historical
`beacon-chain/forkchoice/doubly-linked-tree/goldfish.go:438–457` permits the
unique distinguished round-start proposal to start the head walk only when
`IsRoundStart(current)` and `n.slot == current`. Block16 can therefore become
head during16 without prior-slot support. At17 this override is unavailable;
`:503–511` scores vote-slot16 and performs the ordinary descent. The23/489
accepted-root split is consistent with16 losing to15 at that boundary, but
accepted gossip precedes subscriber insertion. It is not the exact retained
electorate or proof of the precise rejection gate in every node. The observed
head change and independent parent roots are firmer outcome evidence.

## Evidence boundary for the final causal account

The historical records support three distinct build failure dependencies:
payload retrieval plus unavailable fallback (5/6/8/9), unfinished joined
consensus construction before a canceled post-state operation (10/14), and
parent-state cancellation before block assembly (13). They also establish
successful production/import for15/16, transient head changes, and genesis
finality at those imports. They do not select one exact scheduler/lock/CPU
cause for every owner. The previous controlled experiments supply a concrete
common mechanism and counterfactual evidence under their measured input;
transferring that mechanism to every missing historical interval must remain
explicitly qualified.
