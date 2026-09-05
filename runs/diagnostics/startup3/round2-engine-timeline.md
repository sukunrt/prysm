# Round 2 owner BN-to-EL timeline

This is a read-only reconstruction of the recovered logs for the slot 1, 2,
and 3 proposers. Times are the archive's outer UTC timestamps. Snooper requests
are useful evidence of work initiated by the beacon node; execution-client logs
alone are not treated as BN responsiveness evidence.

## Node 169

Sources: `/tmp/prysm-r2-extra-logs.Rd7MjT/extracted/round2-169/`.

| BN request | request time | HTTP response time | result |
| --- | --- | --- | --- |
| FCU V4 with slot 1 attributes, snooper #709 | 01:30:12.166999 | 01:30:12.167373 | VALID, payload ID `0x0466ae591b684070` |
| `eth_getBlockByNumber`, #710 | 01:30:14.772568 | 01:30:14.773037 | HTTP 200 |
| `eth_getBlockByNumber`, #711 | 01:30:28.754657 | 01:30:28.755420 | HTTP 200 |
| FCU V4 without attributes, #712 | 01:30:33.849736 | 01:30:33.850656 | VALID, null payload ID |
| `eth_getBlockByNumber`, #713 | 01:30:43.958128 | 01:30:43.958815 | HTTP 200 |

Snooper line references are 31640/31663, 31677/31688, 31724/31735,
31771/31786, and 31800/31811 (request/response). The beacon log records the
slot-boundary reorg at lines 516-517 (01:30:12.117), an execution follow-distance
message at line 518 (01:30:15.022), and the next blockchain progress message at
line 519 (01:30:26.891).

This is a blockchain-progress logging gap, not complete BN silence. Also,
14.772568 to 28.754657 is the nominal approximately 14-second execution polling
period. It is not evidence that the poll was delayed or skipped. There is no
BN-to-EL request specifically during the VC's approximately 21.915-24.016 RPC
failure interval, but the nominal polling cadence does not require one there.

## Node 191

Sources: `/tmp/prysm-r2-extra-logs.Rd7MjT/extracted/round2-191/`.

| BN request | request time | HTTP response time | result |
| --- | --- | --- | --- |
| FCU V4 without attributes, snooper #714 | 01:30:12.008325 | 01:30:12.008404 | VALID, null payload ID |
| FCU V4 with slot 2 attributes, #715 | 01:30:15.022032 | 01:30:15.023247 | VALID, payload ID `0x044500b8683c0645` |
| `eth_getBlockByNumber`, #716 | 01:30:24.172988 | 01:30:24.173653 | HTTP 200 |
| FCU V4 with slot 2 attributes, #717 | 01:30:35.766651 | 01:30:35.766898 | VALID, same payload ID |
| `eth_getBlockByNumber`, #718 | 01:30:39.011818 | 01:30:39.012729 | HTTP 200 |

Snooper line references are 31728/31743, 31757/31780, 31794/31805,
31841/31864, and 31878/31889. Beacon line 510 logs “Forkchoice updated with
payload attributes for proposal” at 01:30:16.298567, 1.275320 seconds after
snooper logged the completed #715 response. The later #717 exchange completed
in less than a millisecond.

## Node 22

Sources: `/tmp/prysm-r2-extra-logs.Rd7MjT/extracted/round2-22/`.

| BN request | request time | HTTP response time | result |
| --- | --- | --- | --- |
| FCU V4 without attributes, snooper #717 | 01:30:12.006499 | 01:30:12.007275 | VALID, null payload ID |
| `eth_getBlockByNumber`, #718 | 01:30:12.966808 | 01:30:12.969438 | HTTP 200 |
| FCU V4 without attributes, #719 | 01:30:23.776244 | 01:30:23.776365 | VALID, null payload ID |
| `eth_getBlockByNumber`, #720 | 01:30:28.192552 | 01:30:28.193740 | HTTP 200 |
| FCU V4 with slot 3 attributes, #721 | 01:30:34.556753 | 01:30:34.558255 | VALID, payload ID `0x0432aab01f008427` |
| `eth_getBlockByNumber`, #722 | 01:30:40.955214 | 01:30:40.955877 | HTTP 200 |

Snooper line references are 31840/31855, 31869/31880, 31916/31931,
31945/31956, 31992/32015, and 32029/32040. Beacon line 509 logs the corresponding
payload-attributes update at 01:30:39.126162, 4.567907 seconds after snooper
logged the completed #721 response.

## What the response-to-log intervals establish

The EL and snooper completed these FCU calls quickly. The later beacon log lines
show that the calling BN path did not reach its visible completion point until
1.275 seconds later on node 191 and 4.568 seconds later on node 22. This is
delay between the proxy recording its response and the BN's visible application
completion, but the historical logs do not identify its exact substage. The
proxy's buffered response log is not itself a packet-level delivery timestamp
at the BN.

In `lateBlockTasks`, a successful Gloas FCU return is followed by
`PayloadIDCache.Set`, then construction of the log fields including `HeadSlot()`,
then the log call and payload-attributes feed event
(`beacon-chain/blockchain/process_block.go:1271-1284`). The cache uses its own
mutex (`beacon-chain/cache/payload_id.go:43-55`), while `HeadSlot()` acquires the
chain head read lock (`beacon-chain/blockchain/chain_info.go:173-182`). Before
the HTTP call, `notifyForkchoiceUpdateGloas` also reads the fork-choice store
under its read lock (`beacon-chain/blockchain/receive_execution_payload_envelope.go:455-473`).

Consequently, response-to-log time cannot by itself be labeled scheduler delay:
it can include proxy flush/delivery, transport return into Go, runtime
scheduling, payload-cache lock wait, head-lock wait, and logging. The local H runtime trace resolves scheduler
delay for that separate controlled reproduction; no equivalent runtime trace or
packet capture exists for historical round 2.

The three recovered owners also show simultaneous VC deadline failures across
unrelated RPCs, which excludes a proposer-only VC operation as the sole issue.
The fast FCU responses exclude slow EL execution for these particular calls.
They do not prove the owner-local FFG volume because the owner beacon logs omit
individual FFG validation entries and their aggregate summaries. Sampled-peer
FFG entries demonstrate network-wide load only and must not be assigned to an
owner.
