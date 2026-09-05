# Sanitized owner-local early-timeline excerpts

Window: `2026-09-05T01:29:48+00:00` through `2026-09-05T01:34:00+00:00` (inclusive).
Long validator-key and committee lists are replaced by item counts. Engine API HTTP headers are never emitted.

## Slot 1: node 169

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:650`
  `2026-09-05T01:30:02.886823790Z [2026-09-05 01:30:02.88]  INFO client: Duties schedule attesterCount=75 attesterPubkeys=[<75 items>] proposerPubkey=0xa7120c370e8c ptcCount=4 ptcPubkeys=[<4 items>] slot=1 slotInEpoch=1 timeUntilDuty=10s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:694`
  `2026-09-05T01:30:24.002452348Z [2026-09-05 01:30:24.00] ERROR client: Failed to sign randao reveal error=could not get domain data: rpc error: code = DeadlineExceeded desc = context deadline exceeded pubkey=0xa7120c370e8c`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::beacon.log:518`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::beacon.log:518`
  - `Could not get signed attestation data for aggregation`: 3; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:718`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:757`
  - `Could not get sync subcommittee index`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:708`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:708`
  - `Could not request attestation to sign at slot`: 75; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:688`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:774`
  - `Could not request payload attestation data`: 3; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:687`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:696`
  - `Could not request sync message block root to sign`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:689`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:690`
  - `Could not submit aggregate selection proof to beacon node`: 3; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:719`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:758`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::validator.log:1206`
  `2026-09-05T01:31:36.002588491Z [2026-09-05 01:31:36.00]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<71 items>] pubkeys=[<71 items>] slot=7 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=450ms submittedSinceSlotStart=3.017s targetRoot=0x1a40155d770d targetRound=0`
- Engine `engine_forkchoiceUpdatedV4` proxy #709: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::snooper-engine.log:31640` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::snooper-engine.log:31641`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::snooper-engine.log:31663` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-169.tar.gz::snooper-engine.log:31664`; proxy duration `4 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0x000000000000...00000000","timestamp":"0x6a9b70a4","withdrawals":0},"rpc_id":201}`
  Response body fields: `{"payload_id":"0x0466ae591b684070","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":201}`

## Slot 2: node 191

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:652`
  `2026-09-05T01:30:02.615390779Z [2026-09-05 01:30:02.61]  INFO client: Duties schedule attesterCount=58 attesterPubkeys=[<58 items>] proposerPubkey=0xaf089d24fea6 ptcCount=1 ptcPubkeys=[<1 items>] slot=2 slotInEpoch=2 timeUntilDuty=22s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:780`
  `2026-09-05T01:30:36.004480876Z [2026-09-05 01:30:36.00] ERROR client: Failed to sign randao reveal error=could not get domain data: rpc error: code = DeadlineExceeded desc = context deadline exceeded pubkey=0xaf089d24fea6`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::beacon.log:510`
  `2026-09-05T01:30:16.298566991Z [2026-09-05 01:30:16.29]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=2 payloadID=0x044500b8683c`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::beacon.log:511`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::beacon.log:511`
  - `Could not check if any validator is a sync committee aggregator`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:770`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:770`
  - `Could not get signed attestation data for aggregation`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:774`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:774`
  - `Could not request attestation to sign at slot`: 58; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:771`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:835`
  - `Could not request payload attestation data`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:689`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:777`
  - `Could not request sync message block root to sign`: 3; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:779`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:819`
  - `Could not submit aggregate selection proof to beacon node`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:775`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:775`
  - `Could not submit sync committee message`: 3; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:686`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:688`
  - `Sync Committee Message is too old to broadcast, discarding it`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::beacon.log:512`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::beacon.log:513`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::validator.log:1866`
  `2026-09-05T01:33:11.953277836Z [2026-09-05 01:33:11.95]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<66 items>] pubkeys=[<66 items>] slot=15 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=32ms submittedSinceSlotStart=5.146s targetRoot=0x1a40155d770d targetRound=1`
- Engine `engine_forkchoiceUpdatedV4` proxy #715: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::snooper-engine.log:31757` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::snooper-engine.log:31758`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::snooper-engine.log:31780` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::snooper-engine.log:31781`; proxy duration `1 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0x000000000000...00000000","timestamp":"0x6a9b70b0","withdrawals":0},"rpc_id":202}`
  Response body fields: `{"payload_id":"0x044500b8683c0645","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":202}`
- Engine `engine_forkchoiceUpdatedV4` proxy #717: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::snooper-engine.log:31841` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::snooper-engine.log:31842`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::snooper-engine.log:31864` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-191.tar.gz::snooper-engine.log:31865`; proxy duration `0 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0x000000000000...00000000","timestamp":"0x6a9b70b0","withdrawals":0},"rpc_id":204}`
  Response body fields: `{"payload_id":"0x044500b8683c0645","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":204}`

## Slot 3: node 22

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:653`
  `2026-09-05T01:30:02.747057481Z [2026-09-05 01:30:02.74]  INFO client: Duties schedule attesterCount=96 attesterPubkeys=[<96 items>] proposerPubkey=0xa4a9ccf01138 ptcCount=6 ptcPubkeys=[<6 items>] slot=3 slotInEpoch=3 timeUntilDuty=34s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:800`
  `2026-09-05T01:30:48.003364340Z [2026-09-05 01:30:48.00] ERROR client: Failed to sign randao reveal error=could not get domain data: rpc error: code = DeadlineExceeded desc = context deadline exceeded pubkey=0xa4a9ccf01138`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::beacon.log:509`
  `2026-09-05T01:30:39.126161692Z [2026-09-05 01:30:39.12]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=3 payloadID=0x0432aab01f00`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::beacon.log:510`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::beacon.log:510`
  - `Could not check if any validator is a sync committee aggregator`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:696`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:775`
  - `Could not get signed attestation data for aggregation`: 3; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:806`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:877`
  - `Could not process attestation for fork choice`: 11; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::beacon.log:524`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::beacon.log:572`
  - `Could not request attestation to sign at slot`: 96; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:776`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:887`
  - `Could not request payload attestation data`: 7; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:701`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:792`
  - `Could not request sync message block root to sign`: 6; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:699`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:805`
  - `Could not submit aggregate selection proof to beacon node`: 3; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:809`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:878`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::validator.log:1633`
  `2026-09-05T01:32:46.084345882Z [2026-09-05 01:32:46.08]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<70 items>] pubkeys=[<70 items>] slot=13 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=49ms submittedSinceSlotStart=10.035s targetRoot=0x1a40155d770d targetRound=1`
- Engine `engine_forkchoiceUpdatedV4` proxy #721: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::snooper-engine.log:31992` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::snooper-engine.log:31993`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::snooper-engine.log:32015` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-22.tar.gz::snooper-engine.log:32016`; proxy duration `1 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b70bc","withdrawals":0},"rpc_id":204}`
  Response body fields: `{"payload_id":"0x0432aab01f008427","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":204}`

## Slot 4: node 91

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:654`
  `2026-09-05T01:30:03.576964255Z [2026-09-05 01:30:03.57]  INFO client: Duties schedule attesterCount=75 attesterPubkeys=[<75 items>] proposerPubkey=0xb1b3d89c62f5 ptcCount=2 ptcPubkeys=[<2 items>] slot=4 slotInEpoch=4 timeUntilDuty=45s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:839`
  `2026-09-05T01:31:00.003910654Z [2026-09-05 01:31:00.00] ERROR client: Failed to sign randao reveal error=could not get domain data: rpc error: code = DeadlineExceeded desc = context deadline exceeded pubkey=0xb1b3d89c62f5`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::beacon.log:538`
  `2026-09-05T01:30:46.014511947Z [2026-09-05 01:30:46.01]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=4 payloadID=0x04bbafa05763`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::beacon.log:539`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::beacon.log:539`
  - `Could not check if any validator is a sync committee aggregator`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:764`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:832`
  - `Could not get signed attestation data for aggregation`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:852`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:876`
  - `Could not request attestation to sign at slot`: 75; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:833`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:918`
  - `Could not request payload attestation data`: 3; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:785`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:838`
  - `Could not request sync message block root to sign`: 8; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:765`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:844`
  - `Could not submit aggregate selection proof to beacon node`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:853`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:877`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::validator.log:1272`
  `2026-09-05T01:31:48.015187596Z [2026-09-05 01:31:48.01]  INFO client: Submitted sync committee messages blockRoot=0x1a40155d770d dataSlot=8 messages=4 slot=8 validatorIndices=25983,26125,26140,26318`
- Engine `engine_forkchoiceUpdatedV4` proxy #717: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::snooper-engine.log:31923` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::snooper-engine.log:31924`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::snooper-engine.log:31946` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-91.tar.gz::snooper-engine.log:31947`; proxy duration `2 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b70c8","withdrawals":0},"rpc_id":203}`
  Response body fields: `{"payload_id":"0x04bbafa057634852","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":203}`

## Slot 5: node 118

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:655`
  `2026-09-05T01:30:02.290582399Z [2026-09-05 01:30:02.28]  INFO client: Duties schedule attesterCount=67 attesterPubkeys=[<67 items>] proposerPubkey=0xa0b75a40344e ptcCount=2 ptcPubkeys=[<2 items>] slot=5 slotInEpoch=5 timeUntilDuty=58s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:971`
  `2026-09-05T01:31:12.002351287Z [2026-09-05 01:31:12.00] ERROR client: Failed to request block from beacon node error=rpc error: code = DeadlineExceeded desc = context deadline exceeded pubkey=0xa0b75a40344e slot=5`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:1005`
  `2026-09-05T01:31:12.004383360Z [2026-09-05 01:31:12.00]  INFO client: Submitted sync committee messages blockRoot=0x1a40155d770d dataSlot=5 messages=1 slot=5 validatorIndices=72296`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::beacon.log:522`
  `2026-09-05T01:30:58.010429458Z [2026-09-05 01:30:58.01]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=5 payloadID=0x0489a8a1998c`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::beacon.log:524`
  `2026-09-05T01:31:10.119967431Z [2026-09-05 01:31:10.11]  INFO rpc/validator: Building block sinceSlotStartTime=10.119713353s slot=5`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::beacon.log:527`
  `2026-09-05T01:31:12.011576954Z [2026-09-05 01:31:12.01]  WARN rpc/validator: Could not get local payload, falling back to P2P bid error=could not get cached payload from execution client: timeout from http.Client`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::beacon.log:528`
  `2026-09-05T01:31:14.527478983Z [2026-09-05 01:31:14.52] ERROR rpc/validator: Could not build block error=rpc error: code = Internal desc = Could not get local payload and no P2P bid fallback: no cached P2P bid available sinceSlotStartTime=14.527177809s slot=5 validator=72524`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::beacon.log:523`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::beacon.log:523`
  - `Could not check if any validator is a sync committee aggregator`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:849`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:849`
  - `Could not get sync subcommittee index`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:995`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:995`
  - `Could not request sync message block root to sign`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:859`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:860`
  - `Could not submit aggregate selection proof to beacon node`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:935`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:935`
  - `Could not submit attestation to beacon node`: 67; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:934`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:1004`
  - `Could not submit sync committee message`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:980`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:980`
  - `Voting period before genesis + follow distance, using eth1data from head`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::beacon.log:526`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::beacon.log:526`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::validator.log:1005`
  `2026-09-05T01:31:12.004383360Z [2026-09-05 01:31:12.00]  INFO client: Submitted sync committee messages blockRoot=0x1a40155d770d dataSlot=5 messages=1 slot=5 validatorIndices=72296`
- Engine `engine_forkchoiceUpdatedV4` proxy #708: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::snooper-engine.log:31596` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::snooper-engine.log:31597`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::snooper-engine.log:31619` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::snooper-engine.log:31620`; proxy duration `2 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b70d4","withdrawals":0},"rpc_id":206}`
  Response body fields: `{"payload_id":"0x0489a8a1998c6091","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":206}`
- Engine `engine_getPayloadV6` proxy #711: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::snooper-engine.log:31710` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::snooper-engine.log:31711`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::snooper-engine.log:31720` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-118.tar.gz::snooper-engine.log:31721`; proxy duration `3 ms`.
  Request body fields: `{"method":"engine_getPayloadV6","payload_id":"0x0489a8a1998c6091","rpc_id":209}`
  Response body fields: `{"blockValue":"0x0","execution_payload":{"blockHash":"0x5342eb3c7651...f81df226","blockNumber":"0x1","feeRecipient":"0xf97e180c050e...ee4df134","gasLimit":"0xbe8c711","gasUsed":"0x0","parentHash":"0x296b50f55c75...dfe775a1","slotNumber":"0x5","timestamp":"0x6a9b70d4"},"rpc_id":209,"shouldOverrideBuilder":false,"transactions_count":0,"withdrawals_count":0}`

## Slot 6: node 83

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::validator.log:656`
  `2026-09-05T01:30:04.672209577Z [2026-09-05 01:30:04.67]  INFO client: Duties schedule attesterCount=67 attesterPubkeys=[<67 items>] proposerPubkey=0x989ab25f870c slot=6 slotInEpoch=6 timeUntilDuty=1m8s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::validator.log:1097`
  `2026-09-05T01:31:16.634985550Z [2026-09-05 01:31:16.63] ERROR client: Failed to request block from beacon node error=rpc error: code = Internal desc = could not build block in parallel: rpc error: code = Internal desc = Could not get local payload and no P2P bid fallback: no cached P2P bid available pubkey=0x989ab25f870c slot=6`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::validator.log:1100`
  `2026-09-05T01:31:24.003362873Z [2026-09-05 01:31:24.00]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<67 items>] pubkeys=[<67 items>] slot=6 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=227ms submittedSinceSlotStart=3.879s targetRoot=0x1a40155d770d targetRound=0`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::validator.log:1101`
  `2026-09-05T01:31:24.003395648Z [2026-09-05 01:31:24.00]  INFO client: Submitted sync committee messages blockRoot=0x1a40155d770d dataSlot=6 messages=2 slot=6 validatorIndices=21085,21203`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::beacon.log:540`
  `2026-09-05T01:31:07.247511038Z [2026-09-05 01:31:07.23]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=6 payloadID=0x04343b7fabf2`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::beacon.log:542`
  `2026-09-05T01:31:15.664075699Z [2026-09-05 01:31:15.66]  INFO rpc/validator: Building block sinceSlotStartTime=3.663808948s slot=6`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::beacon.log:545`
  `2026-09-05T01:31:16.456844982Z [2026-09-05 01:31:16.45]  WARN rpc/validator: Could not get local payload, falling back to P2P bid error=could not get cached payload from execution client: timeout from http.Client`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::beacon.log:546`
  `2026-09-05T01:31:16.457180582Z [2026-09-05 01:31:16.45] ERROR rpc/validator: Could not build block error=rpc error: code = Internal desc = Could not get local payload and no P2P bid fallback: no cached P2P bid available sinceSlotStartTime=4.45556975s slot=6 validator=21291`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::beacon.log:549`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::beacon.log:549`
  - `Could not check if any validator is a sync committee aggregator`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::validator.log:1012`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::validator.log:1012`
  - `Could not delete invalid attestations`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::beacon.log:547`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::beacon.log:547`
  - `Could not process attestation for fork choice`: 12; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::beacon.log:563`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::beacon.log:631`
  - `Could not request payload attestation data`: 3; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::validator.log:1013`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::validator.log:1016`
  - `Could not request sync message block root to sign`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::validator.log:1014`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::validator.log:1017`
  - `Could not submit aggregate selection proof to beacon node`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::validator.log:1098`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::validator.log:1099`
  - `Voting period before genesis + follow distance, using eth1data from head`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::beacon.log:544`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::beacon.log:544`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::validator.log:1100`
  `2026-09-05T01:31:24.003362873Z [2026-09-05 01:31:24.00]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<67 items>] pubkeys=[<67 items>] slot=6 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=227ms submittedSinceSlotStart=3.879s targetRoot=0x1a40155d770d targetRound=0`
- Engine `engine_forkchoiceUpdatedV4` proxy #715: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::snooper-engine.log:31880` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::snooper-engine.log:31881`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::snooper-engine.log:31903` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::snooper-engine.log:31904`; proxy duration `24 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b70e0","withdrawals":0},"rpc_id":204}`
  Response body fields: `{"payload_id":"0x04343b7fabf2c9b1","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":204}`
- Engine `engine_getPayloadV6` proxy #717: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::snooper-engine.log:31964` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::snooper-engine.log:31965`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::snooper-engine.log:31974` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-83.tar.gz::snooper-engine.log:31975`; proxy duration `2 ms`.
  Request body fields: `{"method":"engine_getPayloadV6","payload_id":"0x04343b7fabf2c9b1","rpc_id":206}`
  Response body fields: `{"blockValue":"0x0","execution_payload":{"blockHash":"0x2fbd72c7fa0a...a63a4487","blockNumber":"0x1","feeRecipient":"0xf97e180c050e...ee4df134","gasLimit":"0xbe8c711","gasUsed":"0x0","parentHash":"0x296b50f55c75...dfe775a1","slotNumber":"0x6","timestamp":"0x6a9b70e0"},"rpc_id":206,"shouldOverrideBuilder":false,"transactions_count":0,"withdrawals_count":0}`

## Slot 7: node 144

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:657`
  `2026-09-05T01:30:03.406557054Z [2026-09-05 01:30:03.40]  INFO client: Duties schedule attesterCount=87 attesterPubkeys=[<87 items>] proposerPubkey=0xb3dcff8d6129 ptcCount=4 ptcPubkeys=[<4 items>] slot=7 slotInEpoch=7 timeUntilDuty=1m21s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1122`
  `2026-09-05T01:31:36.002488933Z [2026-09-05 01:31:36.00] ERROR client: Failed to sign randao reveal error=could not get domain data: rpc error: code = DeadlineExceeded desc = context deadline exceeded pubkey=0xb3dcff8d6129`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::beacon.log:516`
  `2026-09-05T01:31:33.750881772Z [2026-09-05 01:31:33.75]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=7 payloadID=0x04466c2ab375`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::beacon.log:515`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::beacon.log:517`
  - `Could not check if any validator is a sync committee aggregator`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1015`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1110`
  - `Could not get signed attestation data for aggregation`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1186`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1186`
  - `Could not process attestation for fork choice`: 5; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::beacon.log:532`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::beacon.log:585`
  - `Could not request attestation to sign at slot`: 87; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1111`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1208`
  - `Could not request payload attestation data`: 7; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1021`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1124`
  - `Could not request sync message block root to sign`: 8; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1017`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1121`
  - `Could not submit aggregate selection proof to beacon node`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1187`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1187`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::validator.log:1612`
  `2026-09-05T01:32:36.011764035Z [2026-09-05 01:32:36.01]  INFO client: Submitted sync committee messages blockRoot=0x1a40155d770d dataSlot=12 messages=4 slot=12 validatorIndices=37236,37293,37638,37703`
- Engine `engine_forkchoiceUpdatedV4` proxy #714: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::snooper-engine.log:31819` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::snooper-engine.log:31820`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::snooper-engine.log:31842` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-144.tar.gz::snooper-engine.log:31843`; proxy duration `1 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b70ec","withdrawals":0},"rpc_id":209}`
  Response body fields: `{"payload_id":"0x04466c2ab3756cf9","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":209}`

## Slot 8: node 19

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::validator.log:658`
  `2026-09-05T01:30:01.319010189Z [2026-09-05 01:30:01.31]  INFO client: Duties schedule attesterCount=68 attesterPubkeys=[<68 items>] proposerPubkey=0x8736ff704680 ptcCount=5 ptcPubkeys=[<5 items>] slot=8 slotInEpoch=8 timeUntilDuty=1m35s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::validator.log:1419`
  `2026-09-05T01:31:48.002692639Z [2026-09-05 01:31:48.00] ERROR client: Failed to request block from beacon node error=rpc error: code = DeadlineExceeded desc = context deadline exceeded pubkey=0x8736ff704680 slot=8`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::validator.log:1479`
  `2026-09-05T01:31:48.005348318Z [2026-09-05 01:31:48.00]  INFO client: Submitted sync committee messages blockRoot=0x1a40155d770d dataSlot=8 messages=2 slot=8 validatorIndices=11255,11272`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::beacon.log:531`
  `2026-09-05T01:31:36.005036850Z [2026-09-05 01:31:36.00]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=8 payloadID=0x047cf571caa4`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::beacon.log:533`
  `2026-09-05T01:31:47.191117851Z [2026-09-05 01:31:47.19]  INFO rpc/validator: Building block sinceSlotStartTime=11.19098767s slot=8`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::beacon.log:536`
  `2026-09-05T01:31:48.007748774Z [2026-09-05 01:31:48.00]  WARN rpc/validator: Could not get local payload, falling back to P2P bid error=could not get cached payload from execution client: timeout from http.Client`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::beacon.log:540`
  `2026-09-05T01:31:48.052566176Z [2026-09-05 01:31:48.05] ERROR rpc/validator: Could not build block error=rpc error: code = Internal desc = Could not get local payload and no P2P bid fallback: no cached P2P bid available sinceSlotStartTime=12.052413482s slot=8 validator=10907`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::beacon.log:532`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::beacon.log:532`
  - `Could not get sync committee domain data`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::validator.log:1333`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::validator.log:1334`
  - `Could not submit aggregate selection proof to beacon node`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::validator.log:1416`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::validator.log:1471`
  - `Could not submit attestation to beacon node`: 68; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::validator.log:1407`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::validator.log:1478`
  - `Could not submit sync committee message`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::validator.log:1332`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::validator.log:1474`
  - `Sync Committee Message is too old to broadcast, discarding it`: 3; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::beacon.log:537`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::beacon.log:539`
  - `Voting period before genesis + follow distance, using eth1data from head`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::beacon.log:535`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::beacon.log:535`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::validator.log:1479`
  `2026-09-05T01:31:48.005348318Z [2026-09-05 01:31:48.00]  INFO client: Submitted sync committee messages blockRoot=0x1a40155d770d dataSlot=8 messages=2 slot=8 validatorIndices=11255,11272`
- Engine `engine_forkchoiceUpdatedV4` proxy #719: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::snooper-engine.log:32018` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::snooper-engine.log:32019`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::snooper-engine.log:32041` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::snooper-engine.log:32042`; proxy duration `1 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b70f8","withdrawals":0},"rpc_id":210}`
  Response body fields: `{"payload_id":"0x047cf571caa4f56d","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":210}`
- Engine `engine_getPayloadV6` proxy #721: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::snooper-engine.log:32102` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::snooper-engine.log:32103`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::snooper-engine.log:32112` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-19.tar.gz::snooper-engine.log:32113`; proxy duration `0 ms`.
  Request body fields: `{"method":"engine_getPayloadV6","payload_id":"0x047cf571caa4f56d","rpc_id":212}`
  Response body fields: `{"blockValue":"0x0","execution_payload":{"blockHash":"0xb9fd7a9ca926...e1b97080","blockNumber":"0x1","feeRecipient":"0xf97e180c050e...ee4df134","gasLimit":"0xbe8c711","gasUsed":"0x0","parentHash":"0x296b50f55c75...dfe775a1","slotNumber":"0x8","timestamp":"0x6a9b70f8"},"rpc_id":212,"shouldOverrideBuilder":false,"transactions_count":0,"withdrawals_count":0}`

## Slot 9: node 107

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::validator.log:659`
  `2026-09-05T01:30:02.999358184Z [2026-09-05 01:30:02.99]  INFO client: Duties schedule attesterCount=88 attesterPubkeys=[<88 items>] proposerPubkey=0xb3467ad53274 slot=9 slotInEpoch=9 timeUntilDuty=1m46s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::validator.log:1207`
  `2026-09-05T01:31:55.923308038Z [2026-09-05 01:31:55.91] ERROR client: Failed to request block from beacon node error=rpc error: code = Internal desc = could not build block in parallel: rpc error: code = Internal desc = Could not get local payload and no P2P bid fallback: no cached P2P bid available pubkey=0xb3467ad53274 slot=9`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::validator.log:1211`
  `2026-09-05T01:32:00.012879011Z [2026-09-05 01:32:00.00]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<88 items>] pubkeys=[<88 items>] slot=9 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=3.64s submittedSinceSlotStart=6.047s targetRoot=0x1a40155d770d targetRound=1`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::validator.log:1212`
  `2026-09-05T01:32:00.013096503Z [2026-09-05 01:32:00.01]  INFO client: Submitted sync committee messages blockRoot=0x1a40155d770d dataSlot=9 messages=6 slot=9 validatorIndices=35333,35335,35424,35492,35527,35782`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::beacon.log:529`
  `2026-09-05T01:31:46.010211982Z [2026-09-05 01:31:46.00]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=9 payloadID=0x042f29fee7c5`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::beacon.log:531`
  `2026-09-05T01:31:53.983282724Z [2026-09-05 01:31:53.98]  INFO rpc/validator: Building block sinceSlotStartTime=5.981177378s slot=9`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::beacon.log:533`
  `2026-09-05T01:31:55.638494096Z [2026-09-05 01:31:55.63]  WARN rpc/validator: Could not get local payload, falling back to P2P bid error=could not get cached payload from execution client: timeout from http.Client`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::beacon.log:534`
  `2026-09-05T01:31:55.638559131Z [2026-09-05 01:31:55.63] ERROR rpc/validator: Could not build block error=rpc error: code = Internal desc = Could not get local payload and no P2P bid fallback: no cached P2P bid available sinceSlotStartTime=7.638358237s slot=9 validator=35397`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::beacon.log:530`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::beacon.log:530`
  - `Could not check if any validator is a sync committee aggregator`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::validator.log:1132`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::validator.log:1132`
  - `Could not request sync message block root to sign`: 6; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::validator.log:1134`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::validator.log:1158`
  - `Could not submit aggregate selection proof to beacon node`: 3; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::validator.log:1208`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::validator.log:1210`
  - `Voting period before genesis + follow distance, using eth1data from head`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::beacon.log:535`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::beacon.log:535`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::validator.log:1211`
  `2026-09-05T01:32:00.012879011Z [2026-09-05 01:32:00.00]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<88 items>] pubkeys=[<88 items>] slot=9 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=3.64s submittedSinceSlotStart=6.047s targetRoot=0x1a40155d770d targetRound=1`
- Engine `engine_forkchoiceUpdatedV4` proxy #720: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::snooper-engine.log:32106` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::snooper-engine.log:32107`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::snooper-engine.log:32129` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::snooper-engine.log:32130`; proxy duration `2 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b7104","withdrawals":0},"rpc_id":211}`
  Response body fields: `{"payload_id":"0x042f29fee7c5b342","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":211}`
- Engine `engine_getPayloadV6` proxy #722: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::snooper-engine.log:32191` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::snooper-engine.log:32192`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::snooper-engine.log:32201` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-107.tar.gz::snooper-engine.log:32202`; proxy duration `22 ms`.
  Request body fields: `{"method":"engine_getPayloadV6","payload_id":"0x042f29fee7c5b342","rpc_id":213}`
  Response body fields: `{"blockValue":"0x0","execution_payload":{"blockHash":"0xcc2df59aec14...8a599b22","blockNumber":"0x1","feeRecipient":"0xf97e180c050e...ee4df134","gasLimit":"0xbe8c711","gasUsed":"0x0","parentHash":"0x296b50f55c75...dfe775a1","slotNumber":"0x9","timestamp":"0x6a9b7104"},"rpc_id":213,"shouldOverrideBuilder":false,"transactions_count":0,"withdrawals_count":0}`

## Slot 10: node 35

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::validator.log:660`
  `2026-09-05T01:30:01.835142655Z [2026-09-05 01:30:01.83]  INFO client: Duties schedule attesterCount=76 attesterPubkeys=[<76 items>] proposerPubkey=0xa7b6c30e5bbe ptcCount=6 ptcPubkeys=[<6 items>] slot=10 slotInEpoch=10 timeUntilDuty=1m59s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::validator.log:1384`
  `2026-09-05T01:32:12.004886514Z [2026-09-05 01:32:12.00] ERROR client: Failed to request block from beacon node error=rpc error: code = DeadlineExceeded desc = context deadline exceeded pubkey=0xa7b6c30e5bbe slot=10`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::validator.log:1385`
  `2026-09-05T01:32:12.004981690Z [2026-09-05 01:32:12.00]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<76 items>] pubkeys=[<76 items>] slot=10 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=87ms submittedSinceSlotStart=10.252s targetRoot=0x1a40155d770d targetRound=1`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::validator.log:1386`
  `2026-09-05T01:32:12.004992552Z [2026-09-05 01:32:12.00]  INFO client: Submitted sync committee messages blockRoot=0x1a40155d770d dataSlot=10 messages=5 slot=10 validatorIndices=20477,20522,20795,20803,20827`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::beacon.log:514`
  `2026-09-05T01:32:07.446879297Z [2026-09-05 01:32:07.44]  INFO rpc/validator: Building block sinceSlotStartTime=7.446624429s slot=10`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::beacon.log:515`
  `2026-09-05T01:32:10.015749981Z [2026-09-05 01:32:10.01]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=10 payloadID=0x04c212ceed62`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::beacon.log:519`
  `2026-09-05T01:32:10.338144388Z [2026-09-05 01:32:10.33]  INFO rpc/validator: Chose payload bid slot=10 source=self-build valueGwei=0`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::beacon.log:522`
  `2026-09-05T01:32:36.874620082Z [2026-09-05 01:32:36.87] ERROR rpc/validator: Could not build block error=rpc error: code = Internal desc = Could not compute state root: rpc error: code = Canceled desc = context error: context canceled sinceSlotStartTime=36.874415716s slot=10 validator=20714`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::beacon.log:516`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::beacon.log:516`
  - `Could not check if any validator is a sync committee aggregator`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::validator.log:1292`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::validator.log:1292`
  - `Could not get sync committee contribution`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::validator.log:1383`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::validator.log:1383`
  - `Could not request payload attestation data`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::validator.log:1311`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::validator.log:1311`
  - `Could not request sync message block root to sign`: 5; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::validator.log:1295`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::validator.log:1309`
  - `Voting period before genesis + follow distance, using eth1data from head`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::beacon.log:518`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::beacon.log:518`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::validator.log:1385`
  `2026-09-05T01:32:12.004981690Z [2026-09-05 01:32:12.00]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<76 items>] pubkeys=[<76 items>] slot=10 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=87ms submittedSinceSlotStart=10.252s targetRoot=0x1a40155d770d targetRound=1`
- Engine `engine_forkchoiceUpdatedV4` proxy #720: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::snooper-engine.log:32098` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::snooper-engine.log:32099`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::snooper-engine.log:32121` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::snooper-engine.log:32122`; proxy duration `1 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b7110","withdrawals":0},"rpc_id":211}`
  Response body fields: `{"payload_id":"0x04c212ceed626159","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":211}`
- Engine `engine_getPayloadV6` proxy #722: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::snooper-engine.log:32182` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::snooper-engine.log:32183`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::snooper-engine.log:32192` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-35.tar.gz::snooper-engine.log:32193`; proxy duration `0 ms`.
  Request body fields: `{"method":"engine_getPayloadV6","payload_id":"0x04c212ceed626159","rpc_id":213}`
  Response body fields: `{"blockValue":"0x0","execution_payload":{"blockHash":"0x4de67884e452...8adfe1cc","blockNumber":"0x1","feeRecipient":"0xf97e180c050e...ee4df134","gasLimit":"0xbe8c711","gasUsed":"0x0","parentHash":"0x296b50f55c75...dfe775a1","slotNumber":"0xa","timestamp":"0x6a9b7110"},"rpc_id":213,"shouldOverrideBuilder":false,"transactions_count":0,"withdrawals_count":0}`

## Slot 11: node 117

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:661`
  `2026-09-05T01:30:04.390302812Z [2026-09-05 01:30:04.38]  INFO client: Duties schedule attesterCount=69 attesterPubkeys=[<69 items>] proposerPubkey=0xa61c6d4cd99b ptcCount=6 ptcPubkeys=[<6 items>] slot=11 slotInEpoch=11 timeUntilDuty=2m8s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1130`
  `2026-09-05T01:32:24.012453541Z [2026-09-05 01:32:24.00] ERROR client: Failed to sign randao reveal error=could not get domain data: rpc error: code = DeadlineExceeded desc = context deadline exceeded pubkey=0xa61c6d4cd99b`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::beacon.log:527`
  `2026-09-05T01:32:07.503494698Z [2026-09-05 01:32:07.50]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=11 payloadID=0x04d95e501ce5`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::beacon.log:529`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::beacon.log:529`
  - `Could not check if any validator is a sync committee aggregator`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1113`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1113`
  - `Could not get signed attestation data for aggregation`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1120`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1120`
  - `Could not request attestation to sign at slot`: 69; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1114`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1193`
  - `Could not request payload attestation data`: 10; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1107`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1169`
  - `Could not request sync message block root to sign`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1123`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1125`
  - `Could not submit aggregate selection proof to beacon node`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1129`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1129`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::validator.log:1356`
  `2026-09-05T01:32:48.014565128Z [2026-09-05 01:32:48.01]  INFO client: Submitted sync committee messages blockRoot=0x1a40155d770d dataSlot=13 messages=2 slot=13 validatorIndices=71743,71834`
- Engine `engine_forkchoiceUpdatedV4` proxy #714: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::snooper-engine.log:31888` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::snooper-engine.log:31889`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::snooper-engine.log:31911` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-117.tar.gz::snooper-engine.log:31912`; proxy duration `2 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b711c","withdrawals":0},"rpc_id":212}`
  Response body fields: `{"payload_id":"0x04d95e501ce5d4ad","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":212}`

## Slot 12: node 14

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:662`
  `2026-09-05T01:30:03.376495979Z [2026-09-05 01:30:03.37]  INFO client: Duties schedule attesterCount=66 attesterPubkeys=[<66 items>] proposerPubkey=0x88d47493db4d ptcCount=4 ptcPubkeys=[<4 items>] slot=12 slotInEpoch=12 timeUntilDuty=2m21s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1391`
  `2026-09-05T01:32:36.007049729Z [2026-09-05 01:32:36.00] ERROR client: Failed to sign randao reveal error=could not get domain data: rpc error: code = DeadlineExceeded desc = context deadline exceeded pubkey=0x88d47493db4d`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::beacon.log:518`
  `2026-09-05T01:32:51.250282623Z [2026-09-05 01:32:51.25]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=12 payloadID=0x0412d190ebb3`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Could not check if any validator is a sync committee aggregator`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1368`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1368`
  - `Could not get signed attestation data for aggregation`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1407`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1442`
  - `Could not request attestation to sign at slot`: 66; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1369`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1443`
  - `Could not request payload attestation data`: 4; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1396`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1423`
  - `Could not request sync message block root to sign`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1401`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1401`
  - `Could not submit aggregate selection proof to beacon node`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1415`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1444`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::validator.log:1526`
  `2026-09-05T01:33:00.002180442Z [2026-09-05 01:33:00.00]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<72 items>] pubkeys=[<72 items>] slot=14 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=90ms submittedSinceSlotStart=9.728s targetRoot=0x1a40155d770d targetRound=1`
- Engine `engine_forkchoiceUpdatedV4` proxy #717: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::snooper-engine.log:31999` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::snooper-engine.log:32000`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::snooper-engine.log:32022` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-14.tar.gz::snooper-engine.log:32023`; proxy duration `1 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b7128","withdrawals":0},"rpc_id":216}`
  Response body fields: `{"payload_id":"0x0412d190ebb3b12c","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":216}`

## Slot 13: node 20

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::validator.log:663`
  `2026-09-05T01:30:03.054009797Z [2026-09-05 01:30:03.05]  INFO client: Duties schedule attesterCount=66 attesterPubkeys=[<66 items>] proposerPubkey=0x8147108c9e32 slot=13 slotInEpoch=13 timeUntilDuty=2m33s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::validator.log:1221`
  `2026-09-05T01:32:48.008413834Z [2026-09-05 01:32:48.00] ERROR client: Failed to request block from beacon node error=rpc error: code = DeadlineExceeded desc = context deadline exceeded pubkey=0x8147108c9e32 slot=13`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::validator.log:1274`
  `2026-09-05T01:32:51.035990227Z [2026-09-05 01:32:51.03]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<15 items>] pubkeys=[<15 items>] slot=13 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=7ms submittedSinceSlotStart=15.024s targetRoot=0x1a40155d770d targetRound=1`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::beacon.log:527`
  `2026-09-05T01:32:37.317155228Z [2026-09-05 01:32:37.31]  INFO rpc/validator: Building block sinceSlotStartTime=1.316930104s slot=13`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::beacon.log:530`
  `2026-09-05T01:32:46.342761692Z [2026-09-05 01:32:46.34]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=13 payloadID=0x0487fed91225`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::beacon.log:531`
  `2026-09-05T01:32:48.038076668Z [2026-09-05 01:32:48.03] ERROR rpc/validator: Fail to build block: could not get parent state error=rpc error: code = Internal desc = Could not process slots up to 13: could not process slots: context canceled slot=13`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::beacon.log:528`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::beacon.log:528`
  - `Could not get signed attestation data for aggregation`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::validator.log:1271`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::validator.log:1271`
  - `Could not request attestation to sign at slot`: 66; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::validator.log:1205`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::validator.log:1273`
  - `Could not request payload attestation data`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::validator.log:1171`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::validator.log:1175`
  - `Could not submit aggregate selection proof to beacon node`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::validator.log:1272`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::validator.log:1272`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::validator.log:1274`
  `2026-09-05T01:32:51.035990227Z [2026-09-05 01:32:51.03]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<15 items>] pubkeys=[<15 items>] slot=13 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=7ms submittedSinceSlotStart=15.024s targetRoot=0x1a40155d770d targetRound=1`
- Engine `engine_forkchoiceUpdatedV4` proxy #721: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::snooper-engine.log:32149` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::snooper-engine.log:32150`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::snooper-engine.log:32172` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-20.tar.gz::snooper-engine.log:32173`; proxy duration `1 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b7134","withdrawals":0},"rpc_id":216}`
  Response body fields: `{"payload_id":"0x0487fed9122584fe","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":216}`

## Slot 14: node 85

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::validator.log:664`
  `2026-09-05T01:30:04.103316773Z [2026-09-05 01:30:04.10]  INFO client: Duties schedule attesterCount=74 attesterPubkeys=[<74 items>] proposerPubkey=0x9695b49b1c8b ptcCount=4 ptcPubkeys=[<4 items>] slot=14 slotInEpoch=14 timeUntilDuty=2m44s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::validator.log:1561`
  `2026-09-05T01:33:00.002297931Z [2026-09-05 01:33:00.00] ERROR client: Failed to request block from beacon node error=rpc error: code = DeadlineExceeded desc = context deadline exceeded pubkey=0x9695b49b1c8b slot=14`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::validator.log:1562`
  `2026-09-05T01:33:00.002477367Z [2026-09-05 01:33:00.00]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<74 items>] pubkeys=[<74 items>] slot=14 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=85ms submittedSinceSlotStart=2.717s targetRoot=0x1a40155d770d targetRound=1`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::validator.log:1563`
  `2026-09-05T01:33:00.002545289Z [2026-09-05 01:33:00.00]  INFO client: Submitted sync committee messages blockRoot=0x1a40155d770d dataSlot=14 messages=5 slot=14 validatorIndices=22289,22332,22352,22458,22666`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::validator.log:1564`
  `2026-09-05T01:33:00.002563479Z [2026-09-05 01:33:00.00]  INFO client: Submitted sync committee contributions and proofs aggregatorIndices=22289 blockRoot=0x1a40155d770d contributions=1 dataSlot=14 slot=14 subcommittees=3 totalBits=1`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::beacon.log:542`
  `2026-09-05T01:32:44.523928605Z [2026-09-05 01:32:44.52]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=14 payloadID=0x0478efe32a8a`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::beacon.log:543`
  `2026-09-05T01:32:50.694093299Z [2026-09-05 01:32:50.69]  INFO rpc/validator: Building block sinceSlotStartTime=2.693809035s slot=14`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::beacon.log:546`
  `2026-09-05T01:32:50.744064893Z [2026-09-05 01:32:50.74]  INFO rpc/validator: Chose payload bid slot=14 source=self-build valueGwei=0`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::beacon.log:563`
  `2026-09-05T01:33:31.996258911Z [2026-09-05 01:33:31.99] ERROR rpc/validator: Could not build block error=rpc error: code = Internal desc = Could not compute state root: rpc error: code = Canceled desc = context error: context canceled sinceSlotStartTime=43.996034164s slot=14 validator=22673`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::beacon.log:547`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::beacon.log:547`
  - `Could not request payload attestation data`: 4; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::validator.log:1557`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::validator.log:1560`
  - `Could not submit aggregate selection proof to beacon node`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::validator.log:1556`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::validator.log:1556`
  - `Voting period before genesis + follow distance, using eth1data from head`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::beacon.log:545`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::beacon.log:545`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::validator.log:1562`
  `2026-09-05T01:33:00.002477367Z [2026-09-05 01:33:00.00]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<74 items>] pubkeys=[<74 items>] slot=14 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=85ms submittedSinceSlotStart=2.717s targetRoot=0x1a40155d770d targetRound=1`
- Engine `engine_forkchoiceUpdatedV4` proxy #724: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::snooper-engine.log:32270` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::snooper-engine.log:32271`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::snooper-engine.log:32293` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::snooper-engine.log:32294`; proxy duration `1 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b7140","withdrawals":0},"rpc_id":212}`
  Response body fields: `{"payload_id":"0x0478efe32a8af6aa","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":212}`
- Engine `engine_getPayloadV6` proxy #725: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::snooper-engine.log:32307` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::snooper-engine.log:32308`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::snooper-engine.log:32317` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-85.tar.gz::snooper-engine.log:32318`; proxy duration `1 ms`.
  Request body fields: `{"method":"engine_getPayloadV6","payload_id":"0x0478efe32a8af6aa","rpc_id":213}`
  Response body fields: `{"blockValue":"0x0","execution_payload":{"blockHash":"0xc9217a0a79b0...95b693ad","blockNumber":"0x1","feeRecipient":"0xf97e180c050e...ee4df134","gasLimit":"0xbe8c711","gasUsed":"0x0","parentHash":"0x296b50f55c75...dfe775a1","slotNumber":"0xe","timestamp":"0x6a9b7140"},"rpc_id":213,"shouldOverrideBuilder":false,"transactions_count":0,"withdrawals_count":0}`

## Slot 15: node 32

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::validator.log:665`
  `2026-09-05T01:30:01.968107259Z [2026-09-05 01:30:01.96]  INFO client: Duties schedule attesterCount=77 attesterPubkeys=[<77 items>] proposerPubkey=0xb8046a792d6f ptcCount=2 ptcPubkeys=[<2 items>] slot=15 slotInEpoch=15 timeUntilDuty=2m59s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::validator.log:1324`
  `2026-09-05T01:33:01.658630063Z [2026-09-05 01:33:01.65]  INFO client: Submitted new block attestationCount=8 bidValue=0 blockHash=0xdf777d7bd39c blockRoot=0x6856419067ea builderIndex=self-build depositCount=0 fork=gloas gasLimit=199804689 graffiti=prysm-geth-32 GEaa1fPM0280 parentHash=0x296b50f55c75 payloadAttestationCount=0 pubkey=0xb8046a792d6f slot=15`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::validator.log:1325`
  `2026-09-05T01:33:09.003490960Z [2026-09-05 01:33:09.00]  INFO client: Submitted new attestations blockRoot=0x1a40155d770d committeeIndices=[<77 items>] pubkeys=[<77 items>] slot=15 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=185ms submittedSinceSlotStart=16ms targetRoot=0x1a40155d770d targetRound=1`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::validator.log:1326`
  `2026-09-05T01:33:09.003533695Z [2026-09-05 01:33:09.00]  INFO client: Submitted new aggregate attestations blockRoot=0x1a40155d770d committeeIndices=[<2 items>] pubkeys=[<2 items>] slot=15 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=17ms submittedSinceSlotStart=8.1s targetRoot=0x1a40155d770d targetRound=1`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::validator.log:1327`
  `2026-09-05T01:33:09.003538818Z [2026-09-05 01:33:09.00]  INFO client: Submitted payload attestations attestations=2 blobDataAvailable=true blockRoot=0x6856419067ea dataSlot=15 payloadPresent=true slot=15 validatorIndices=18502,18550`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::validator.log:1328`
  `2026-09-05T01:33:09.003542247Z [2026-09-05 01:33:09.00]  INFO client: Submitted sync committee messages blockRoot=0x6856419067ea dataSlot=15 messages=3 slot=15 validatorIndices=18822,19009,19049`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::beacon.log:530`
  `2026-09-05T01:32:51.040527444Z [2026-09-05 01:32:51.04]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=15 payloadID=0x04e83afd518b`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::beacon.log:532`
  `2026-09-05T01:33:00.009447845Z [2026-09-05 01:33:00.00]  INFO rpc/validator: Building block sinceSlotStartTime=9.237508ms slot=15`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::beacon.log:535`
  `2026-09-05T01:33:00.018810697Z [2026-09-05 01:33:00.01]  INFO rpc/validator: Chose payload bid slot=15 source=self-build valueGwei=0`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::beacon.log:536`
  `2026-09-05T01:33:01.482230895Z [2026-09-05 01:33:01.48]  INFO rpc/validator: Finished building block sinceSlotStartTime=1.481913657s slot=15 validator=18641`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Forkchoice head root does not match head root`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::beacon.log:590`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::beacon.log:590`
  - `Nil finalized block cannot evict old blobs`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::execution.log:99`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::execution.log:102`
  - `Voting period before genesis + follow distance, using eth1data from head`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::beacon.log:534`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::beacon.log:534`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::validator.log:1329`
  `2026-09-05T01:33:21.005946543Z [2026-09-05 01:33:21.00]  INFO client: Submitted new attestations blockRoot=0x6856419067ea committeeIndices=[<55 items>] pubkeys=[<55 items>] slot=16 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=189ms submittedSinceSlotStart=11ms targetRoot=0x6856419067ea targetRound=2`
- Engine `engine_forkchoiceUpdatedV4` proxy #725: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::snooper-engine.log:32301` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::snooper-engine.log:32302`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::snooper-engine.log:32324` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::snooper-engine.log:32325`; proxy duration `1 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b714c","withdrawals":0},"rpc_id":216}`
  Response body fields: `{"payload_id":"0x04e83afd518b4118","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":216}`
- Engine `engine_getPayloadV6` proxy #727: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::snooper-engine.log:32386` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::snooper-engine.log:32387`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::snooper-engine.log:32396` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::snooper-engine.log:32397`; proxy duration `1 ms`.
  Request body fields: `{"method":"engine_getPayloadV6","payload_id":"0x04e83afd518b4118","rpc_id":218}`
  Response body fields: `{"blockValue":"0x0","execution_payload":{"blockHash":"0xdf777d7bd39c...a6b81ab8","blockNumber":"0x1","feeRecipient":"0xf97e180c050e...ee4df134","gasLimit":"0xbe8c711","gasUsed":"0x0","parentHash":"0x296b50f55c75...dfe775a1","slotNumber":"0xf","timestamp":"0x6a9b714c"},"rpc_id":218,"shouldOverrideBuilder":false,"transactions_count":0,"withdrawals_count":0}`
- Engine `engine_forkchoiceUpdatedV4` proxy #729: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::snooper-engine.log:32477` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::snooper-engine.log:32478`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::snooper-engine.log:32492` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-32.tar.gz::snooper-engine.log:32493`; proxy duration `0 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0xdf777d7bd39c...a6b81ab8","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":null,"rpc_id":220}`
  Response body fields: `{"payload_id":null,"payload_status":{"latestValidHash":"0xdf777d7bd39c...a6b81ab8","status":"VALID","validationError":null},"rpc_id":220}`

## Slot 16: node 76

- Schedule `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::validator.log:666`
  `2026-09-05T01:30:01.627103311Z [2026-09-05 01:30:01.62]  INFO client: Duties schedule attesterCount=63 attesterPubkeys=[<63 items>] proposerPubkey=0x801b58c728f8 ptcCount=7 ptcPubkeys=[<7 items>] slot=16 slotInEpoch=16 timeUntilDuty=3m11s`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::validator.log:1846`
  `2026-09-05T01:33:18.773790607Z [2026-09-05 01:33:18.77]  INFO client: Submitted new block attestationCount=8 bidValue=0 blockHash=0x03b9534aa356 blockRoot=0x4b07a2a4ef49 builderIndex=self-build depositCount=0 fork=gloas gasLimit=199804689 graffiti=prysm-geth-76 GEaa1fPM0280 parentHash=0x296b50f55c75 payloadAttestationCount=0 pubkey=0x801b58c728f8 slot=16`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::validator.log:1847`
  `2026-09-05T01:33:23.591426566Z [2026-09-05 01:33:23.59]  INFO client: Submitted new attestations blockRoot=0x6856419067ea committeeIndices=[<63 items>] pubkeys=[<63 items>] slot=16 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=87ms submittedSinceSlotStart=1.581s targetRoot=0x6856419067ea targetRound=2`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::validator.log:1848`
  `2026-09-05T01:33:23.591489733Z [2026-09-05 01:33:23.59]  INFO client: Submitted new aggregate attestations blockRoot=0x6856419067ea committeeIndices=[<1 items>] pubkeys=[<1 items>] slot=16 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=0s submittedSinceSlotStart=9.924s targetRoot=0x6856419067ea targetRound=2`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::validator.log:1849`
  `2026-09-05T01:33:23.591494056Z [2026-09-05 01:33:23.59]  INFO client: Submitted payload attestations attestations=7 blobDataAvailable=true blockRoot=0x4b07a2a4ef49 dataSlot=16 payloadPresent=false slot=16 validatorIndices=107398,107444,107635,107683,107786,107839,107973`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::validator.log:1850`
  `2026-09-05T01:33:23.591496263Z [2026-09-05 01:33:23.59]  INFO client: Submitted sync committee messages blockRoot=0x4b07a2a4ef49 dataSlot=16 messages=4 slot=16 validatorIndices=107467,107596,107818,107956`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::beacon.log:561`
  `2026-09-05T01:33:03.078126141Z [2026-09-05 01:33:03.07]  INFO blockchain: Forkchoice updated with payload attributes for proposal blockRoot=0x1a40155d770d headSlot=0 nextSlot=16 payloadID=0x04b55ca6c261`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::beacon.log:717`
  `2026-09-05T01:33:12.219198051Z [2026-09-05 01:33:12.21]  INFO rpc/validator: Building block sinceSlotStartTime=218.999257ms slot=16`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::beacon.log:722`
  `2026-09-05T01:33:12.703632422Z [2026-09-05 01:33:12.70]  INFO rpc/validator: Chose payload bid slot=16 source=self-build valueGwei=0`
- `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::beacon.log:724`
  `2026-09-05T01:33:13.816690994Z [2026-09-05 01:33:13.81]  INFO rpc/validator: Finished building block sinceSlotStartTime=1.81648205s slot=16 validator=107427`
- Other owned-slot nonroutine records (exact count and boundary anchors):
  - `Beacon node is not respecting the follow distance. EL client is syncing.`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::beacon.log:729`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::beacon.log:729`
  - `Forkchoice head root does not match head root`: 2; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::beacon.log:720`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::beacon.log:731`
  - `Nil finalized block cannot evict old blobs`: 3; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::execution.log:101`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::execution.log:109`
  - `Voting period before genesis + follow distance, using eth1data from head`: 1; first `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::beacon.log:723`; last `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::beacon.log:723`
- First later successful role output `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::validator.log:1851`
  `2026-09-05T01:33:33.971577811Z [2026-09-05 01:33:33.97]  INFO client: Submitted new attestations blockRoot=0x4b07a2a4ef49 committeeIndices=[<90 items>] pubkeys=[<90 items>] slot=17 sourceRoot=0x000000000000 sourceRound=0 submissionSpread=28ms submittedSinceSlotStart=1.175s targetRoot=0x1a40155d770d targetRound=2`
- Engine `engine_forkchoiceUpdatedV4` proxy #724: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32260` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32261`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32283` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32284`; proxy duration `1 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b7158","withdrawals":0},"rpc_id":215}`
  Response body fields: `{"payload_id":"0x04b55ca6c261714b","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":215}`
- Engine `engine_forkchoiceUpdatedV4` proxy #726: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32341` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32342`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32364` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32365`; proxy duration `4 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0xdf777d7bd39c...a6b81ab8","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x6856419067ea...f429df41","prevRandao":"0xa2769748724c...937d4966","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b7158","withdrawals":0},"rpc_id":217}`
  Response body fields: `{"payload_id":"0x04d65b973b554dd1","payload_status":{"latestValidHash":"0xdf777d7bd39c...a6b81ab8","status":"VALID","validationError":null},"rpc_id":217}`
- Engine `engine_forkchoiceUpdatedV4` proxy #730: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32495` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32496`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32518` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32519`; proxy duration `2 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x296b50f55c75...dfe775a1","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":{"parentBeaconBlockRoot":"0x1a40155d770d...caa010d1","prevRandao":"0x296b50f55c75...dfe775a1","suggestedFeeRecipient":"0xf97e180c050e...ee4df134","timestamp":"0x6a9b7158","withdrawals":0},"rpc_id":221}`
  Response body fields: `{"payload_id":"0x04b55ca6c261714b","payload_status":{"latestValidHash":"0x296b50f55c75...dfe775a1","status":"VALID","validationError":null},"rpc_id":221}`
- Engine `engine_getPayloadV6` proxy #731: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32532` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32533`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32542` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32543`; proxy duration `0 ms`.
  Request body fields: `{"method":"engine_getPayloadV6","payload_id":"0x04b55ca6c261714b","rpc_id":222}`
  Response body fields: `{"blockValue":"0x0","execution_payload":{"blockHash":"0x03b9534aa356...41dfd5d3","blockNumber":"0x1","feeRecipient":"0xf97e180c050e...ee4df134","gasLimit":"0xbe8c711","gasUsed":"0x0","parentHash":"0x296b50f55c75...dfe775a1","slotNumber":"0x10","timestamp":"0x6a9b7158"},"rpc_id":222,"shouldOverrideBuilder":false,"transactions_count":0,"withdrawals_count":0}`
- Engine `engine_forkchoiceUpdatedV4` proxy #733: request `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32623` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32624`; response `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32638` / body `/tmp/prysm-r2-extra-logs.Rd7MjT/round2-prysm-geth-76.tar.gz::snooper-engine.log:32639`; proxy duration `1 ms`.
  Request body fields: `{"forkchoice_state":{"finalizedBlockHash":"0x000000000000...00000000","headBlockHash":"0x03b9534aa356...41dfd5d3","safeBlockHash":"0x000000000000...00000000"},"method":"engine_forkchoiceUpdatedV4","payload_attributes":null,"rpc_id":224}`
  Response body fields: `{"payload_id":null,"payload_status":{"latestValidHash":"0x03b9534aa356...41dfd5d3","status":"VALID","validationError":null},"rpc_id":224}`

