# Round 2 remaining archive recovery

## Recovery result

The public compact archives for nodes 201--1000 were initially checked with a
self-imposed 5 MiB Content-Length guard and downloaded without overwriting any
existing archive.  The reusable guarded downloader is
`download_round2_archives.sh`.  After their sizes were confirmed, the three
remaining archives needed for the census (nodes 100, 600, and 800) were fetched
with the guard raised to 128 MiB for those explicit targets.

- 796 of the 800 initially requested node archives were added under
  `/tmp/prysm-r2-extra-logs.Rd7MjT` by the guarded compact-archive pass.
- Nodes 201 (61,275,061 bytes) and 400 (64,884,568 bytes) already had detailed
  archives under `runs/round2` and did not need duplicate downloads.
- Nodes 100 (91,436,013 bytes), 600 (61,424,375 bytes), and 800 (57,908,118
  bytes) were then downloaded under the raised guard.
- Combining both archive locations gives all 1,000 distinct nodes.

No archive was overwritten, and no authentication material was read or
printed.

## Full-union result

The existing `round2_later_slot_audit.py` was rerun over both archive roots and
wrote `/tmp/round2-all1000-union.json`.  All previously classified missing
slots retained the same owner/outcome classifications.  In particular, the
new compact archives did not add an imported block for slots 162, 171, 177, or
182, and did not expose a proposer duty or terminal outcome for those slots.

None of the 1,000 validator logs contains a proposer duty or terminal outcome
for those four slots, and none of the 1,000 beacon logs contains an imported
root. Subsequent raw-log and source audits ruled out missing validator keys
and the proposer's optional `slot` log field as explanations. They show a gap
in logged retained duty assignments and identify an active dependent-root
mismatch guard that discards proposer data. This supports a duty-distribution
explanation, but retries can update the runtime duty store without logging a
fresh schedule. The logs do not prove the precise dispatch boundary or identify
the specific discarded RPC response responsible for each slot.
See [the completed duty-hole audit](round2-empty-proposer-slots.md) and
[source analysis](round2-duty-hole-source-audit.md). Ordinary attester/PTC duty
rows at these slots (for example, on node 100) are not proposer assignments.

The full-union missing-slot rows are:

| slot(s) | recovered validator evidence |
| --- | --- |
| 130, 136, 163, 176, 191, 193, 199, 200 | named owner; block request/proposal deadline |
| 160, 210 | named owner; invalid-RANDAO rejection |
| 166 | three divergent owners; two invalid-RANDAO rejections and one deadline |
| 206 | named owner; request ended with HTTP/2 `RST_STREAM CANCEL` |
| 162, 171, 177, 182 | no proposer duty or outcome among all 1,000 nodes |
| 226 | named owner; request canceled at capture shutdown |

This remains a log-coverage statement.  It does not infer a canonical chain
from the post-slot-129 sparse import union and does not turn absence of an
owner log into a specific failure class.
