# Round 2 later proposer deadline audit

This follows the later no-import rows from the 205-archive census into the proposer
validator and its colocated beacon log. Times below are relative to the nominal slot
start. A validator deadline is the direct terminal failure; a beacon handler's later
log is evidence about work that outlived the caller, not a replacement timestamp for
that failure.

## Results

| Slot / node | VC terminal result | Colocated beacon construction path |
| --- | --- | --- |
| 130 / 112 | `Failed to propose block`, deadline at +12.003 (`validator.log:3021`) | Build starts +0.995, chooses local payload +3.698, and **finishes the block** +11.389 (`beacon.log:983,985-986`). This is post-build: the remaining ~0.61 s is consumed before or within the proposal RPC. No beacon log proves that publication handling was admitted. |
| 136 / 80 | `Failed to request block`, deadline at +12.009 (`validator.log:3389`) | Build +0.954, payload +4.517; invalid-attestation deletion and packing report cancellation +16.445/+17.888, followed by state-root cancellation +17.892 (`beacon.log:1058,1061,1063-1065`). |
| 163 / 158 | request deadline +12.003 (`validator.log:4774`) | Build +0.697, payload +2.287; deletion +16.384, packing +16.841, state-root/build failure +16.847 (`beacon.log:1050,1052,1055-1057`). |
| 176 / 117 | request deadline +12.006 (`validator.log:3635`) | Build +0.075, payload +0.373; deletion +17.093, packing and state-root/build failure +17.496 (`beacon.log:1098,1100,1104,1106-1107`). |
| 191 / 135 | request deadline +12.003 (`validator.log:4846`) | Build +0.039, payload +0.119; deletion +17.942, packing +19.098, state-root/build failure +19.102 (`beacon.log:1074,1076,1080-1082`). |
| 193 / 80 | request deadline +12.008 (`validator.log:4633`) | This invocation begins late at +3.355 and chooses its payload +5.990. It remains alive for 81.345 s: deletion reports cancellation +79.695, packing +81.348, then state-root/build cancellation (`beacon.log:1155,1157,1165-1167`). |
| 199 / 44 | request deadline +12.001 (`validator.log:5548`) | Build +2.516, payload +6.544; deletion +22.660, packing +23.907, state-root/build failure +23.909 (`beacon.log:1142,1144,1147-1149`). |
| 200 / 97 | `Failed to propose block`, deadline at +12.003 (`validator.log:6179`) | Build starts +1.736, chooses payload +8.682, and **finishes the block** +10.857 (`beacon.log:1102,1105-1106`). Like slot 130, construction returned, leaving only ~1.15 s for signing and proposal; the logs do not locate the delay on the client or server side of that RPC. |
| 206 / 29 | request terminates +12.014 with deadline plus HTTP/2 `RST_STREAM CANCEL` (`validator.log:5969`) | Build +0.069, payload +0.235; deletion +17.347, packing +17.874, state-root/build failure +17.879 (`beacon.log:1048,1051,1053-1055`). The reset is cancellation propagation, not evidence of network absence. |
| 226 / 15 | request canceled +0.405 during capture shutdown (`validator.log:6437`) | Build starts +0.017 and fails getting/advancing the parent state +0.427 because its context is canceled (`beacon.log:1108,1111`). This is a shutdown-boundary artifact, not an ordinary slot deadline. |

The log paths are
`/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-<node>.tar.gz` and line numbers
refer to the named member after ANSI removal (line counts are unchanged).

## What the rows establish

Seven ordinary `GetBeaconBlock` failures (136, 163, 176, 191, 193, 199, and 206;
226 is shutdown) reached block construction and, except at shutdown, logged a chosen
local payload. Their asynchronous handlers subsequently converge on the parallel
consensus-packing branch: no invalid-attestation cleanup/packing return is logged before
the caller's 12-second deadline, and the joined build eventually reports canceled
state-root computation. Slot 193 is the extreme retained handler at more than a
minute. This identifies the stalled construction region, but the logs alone do not
divide its elapsed time among pool-lock waiting, deletion, attestation filtering, or
scheduler delay.

Slots 130 and 200 are materially different. `GetBeaconBlock` completed before the
deadline; their VC then timed out in the signing/proposal tail. `Submitted new block`
is absent and the beacon logs show no admitted publication event, so it is not valid
to call either an execution-payload failure or to assert that a built block reached
the network.

These failures recur well after the slot-129 rollback and while isolated branches
continue to produce some blocks. They show repeated construction and proposal
deadline symptoms in the divergent post-rollback regime; they do not by themselves
prove that the genesis active-validator scan remained their common upstream cause.
