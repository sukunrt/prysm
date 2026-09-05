# Completion audit against the original request

Subsequent user steering requested continued work on the **best hypothesis**.
That investigation continued: the [new causal synthesis](best-causal-explanation.md)
now ranks explanations, adds an all-owner historical FCU timing comparison,
and includes a passing real compaction/snapshot overlap experiment. The audit
below retains the stricter original test of unique historical attribution;
it is not the status or conclusion of the subsequent hypothesis work.

The requirement is an exact causal explanation of every first proposal
opportunity, using the retained round-2 evidence and bounded offline work.
That requirement is **not fully met**. Every outcome and terminal dependency
is accounted for, and several upstream code mechanisms have been reproduced
with real costly work. The retained observations do not uniquely identify
the upstream wait on every historical owner.

This audit follows the completed payload, sync-index, parent-state, packing,
and ordinary-domain controls. The preceding work produced substantial new
evidence; this audit does not replace the original requirement with merely
passing diagnostics.

| Requirement | Current evidence | Assessment |
| --- | --- | --- |
| Locate the historical runs and account for every slot through 16 | `archive_inventory.tsv`, `slot_summary.tsv`, owner excerpts and the complete 1,000-node census | Established. Slot 0 is genesis; 1–14 have failed proposals and no imports; 15/16 each have an import on all 1,000 nodes. |
| Read and test the previous explanations against code/history | Prior-startup/build audits, historical-revision checks, observer audit, and corrected packing-input audit | Established. Earlier claims about E1 proposer success, profiling, payload fallback, packing input roots/index, and committee limits were corrected. |
| Explain the payload timeout and recovery behavior in code | Real HTTP count-load differential, reader/cleanup traces, exact timeout mapping, real Gloas recovery differential | Mechanisms established. The historical proxy records do not recover each BN's response receipt, reader scheduling, or timeout return instruction. |
| Explain why sync discovery can consume a proposal budget | Real warm sync method under checkpoint/count work, matched memoized control, manager trace, and actual `RolesAt`/proposer deadline differential | Mechanism and downstream dependency established. Individual historical RPC entry times and prior preflight costs are unlogged. |
| Explain the exact upstream cost in slots 1 and 4 | Owner ordering, real cache-capacity/locking checks, domain source audit, and 32 loaded TCP probes per arm | Unmet. The ordinary TCP pair has no deadline or seconds-scale call. Slot 4's expired RANDAO dispatch is established; its earlier domain/preflight timing and slot 1's proposer/cache/RPC timing are missing. |
| Explain the exact consensus-branch cost in slots 10 and 14 | Owner logs, real builder dependency differential, real packing/compaction preserving data, voters and BLS validity | Mechanism and necessary unfinished result established. Historical packer entry, child timing, pool contents and head/scheduler waits are missing. |
| Explain slot 13's parent-state failure | Exact error boundary, actual-work parent dependency pair, and a separate fork-choice writer/checkpoint-reader trace | A concrete upstream cause is reproduced. The historical split among head access, locking, cache waits and slot processing is not uniquely recovered. |
| Account for slots 15 and 16 | Full import census, exact parent roots, accepted vote cohorts, and real Goldfish replay | Both block productions are established. The observed retention sequence is reproduced under the recorded cohorts; the exact historical fork-choice store snapshot is unavailable. |
| Use real costly work without rerunning the 1,000-node experiment or changing production behavior | Test-only count/checkpoint/HTTP/gRPC/packing controls, retained commands and outputs, current jj file diff | Established for the new work. Go changes are tests and a diagnostic server under `runs/diagnostics`; production Go files were not changed. |
| Use Sol for coding/scripting and Astra for causal analysis, preferring Go | Sol implementations/inventory audit, Astra Ultra causal reviews, retained Go compile/test commands | Established for the follow-up controls. Older Bazel validation limitations remain explicitly recorded in `validation-status.md`. |

## Why another sufficient reproduction would not identify the old execution

The ambiguity is in the observations, not merely in an untested preferred
mechanism:

- Slot 1 can enter its own domain RPC early and time out, or reach that RPC
  after earlier proposer/cache delay with the context already expired. Both
  histories permit the recorded earlier successful sync selection and PTC
  progress. The terminal domain error proves eventual write-lock acquisition
  and adapter invocation, but there is no corresponding entry timestamp.
- Slot 4 did obtain sync indices before its sync-selection domain failure.
  Its missing interval is therefore narrower than a generic sync-index
  timeout: the logs do not time the successful index return, later domain
  lock acquisition, or domain invocation/return. Its subsequent RANDAO role
  is necessarily dispatched after the absolute deadline.
- Slots 10/14 permit an early packer entry with expensive child work or a
  later packer entry after other delay with little pool work. The eth1 warning
  precedes head-data access, and payload selection precedes `lastBidLock` and
  the consensus join. Neither message timestamps entry into the eventual
  slow dependency. Existing controlled builder tests and real-work packing
  tests demonstrate why the terminal cancellation does not select one input
  size or earlier wait.

The remaining measurement for every group is listed in
[remaining-evidence-limit.md](remaining-evidence-limit.md). New VC-cache,
transport, or head-lock experiments could demonstrate further couplings;
without a historical discriminator they would not establish which old
execution occurred. The ordinary-domain null result is retained rather than
escalating load until another timeout appears.

## Current inventory and evidence integrity

A read-only audit on 2026-09-06 revalidated exactly 1,000 selected archive paths
and the same five member names in every `archive_inventory.tsv` row. The
993-plus-12 archive roots still have the same five duplicate node IDs. Owner
archives for slots 1/4/10/14 match the paths, membership records, and separate
`owner_timeline_archives.tsv` size records.

Name scans of the relevant `/tmp` and Prysm run directories found no additional
historical round-2 archive or runtime trace. The candidate profiles have later
local-experiment genesis times or are tracked sample data. The historical
genesis is 2026-09-05 01:30:00 UTC; the shifted local experiments are not
historical owner traces.

The new domain, packing, and parent evidence files are present and tracked.
Their retained passing outputs and recorded hashes were checked. The initial
invalid parent fixture remains explicitly excluded, and the successful traced
parent repetition remains distinct from the untraced cancellation result.
No new benchmark or network run was performed for this completion audit.

The full exact-owner attribution cannot honestly be marked complete on this
evidence. The obstruction is missing historical stage, lock, scheduler,
transport and pool observations; it is not an outstanding permission or
build action.
