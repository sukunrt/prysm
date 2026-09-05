# Round1 retained transaction-content census

## Conclusion

Every execution payload captured in the retained round1 sample is empty. The
rpc-snooper logs contain 25 payload observations: 20 `engine_newPayloadV5`
request payloads and five `engine_getPayloadV6` response payloads. All 25 have
`transactions: []` and `gasUsed: 0x0`. They represent seven distinct block
hashes, and all seven are empty.

As a broader cross-check, every `transactions` field anywhere in the saved
snooper JSON is an empty array: 4,501 empty, zero nonempty, and zero malformed.
That census consists of the 25 full Engine payload observations above, one
`engine_getPayloadBodiesByHashV2` response, 11 `eth_getBlockByHash` responses,
and 4,464 `eth_getBlockByNumber` responses. The payload-bodies response asks
for the already-counted slot-41 hash and returns an empty transaction list; it
is supporting body-recovery evidence rather than an eighth unique full
payload.

This conclusion applies to the ten saved round1 nodes, not all 1,000
participants. `runs/diagnostics/startup_diagnosis.md` identifies those ten as
the retained historical sample. The source used here is the already-extracted
`runs/round1/prysm-geth-*` content. The matching archives under
`/home/sukun/runs/round1` compare byte-for-byte equal to the workspace
archives; the top-level node-1 copies and the node-50 archive in
`/home/sukun/Downloads` are also duplicates, so they add no coverage.

## Engine payload observations

| Observation kind | Records | Unique hashes | Transaction arrays | `gasUsed` |
|---|---:|---:|---|---|
| `engine_newPayloadV5` request | 20 | 2 | 20 empty, 0 nonempty | 20 zero, 0 nonzero |
| `engine_getPayloadV6` response | 5 | 5 | 5 empty, 0 nonempty | 5 zero, 0 nonzero |
| Total | 25 | 7 | 25 empty, 0 nonempty | 25 zero, 0 nonzero |

The two `newPayload` hashes occur once on each of the ten nodes, at payload
slots 7 and 41. The five locally retrieved payloads are distinct and occur at
slots 9, 65, 98, 107, and 130. Thus the payload-slot coverage is 7–130, with
observations from `2026-09-05T00:01:52Z` through `00:26:02Z`. This range is not
continuous: those seven slots are the only Engine payloads present in the
saved snooper logs.

| Node | Full saved snooper RPC span (UTC) | Payload records | `newPayload` | `getPayload` | Observed payload slots |
|---|---|---:|---:|---:|---|
| prysm-geth-1 | Sep 4 22:15:31 – Sep 5 00:27:59 | 4 | 2 | 2 | 7, 9, 41, 98 |
| prysm-geth-2 | Sep 4 22:25:25 – Sep 5 00:27:52 | 2 | 2 | 0 | 7, 41 |
| prysm-geth-201 | Sep 4 22:54:06 – Sep 5 00:28:15 | 2 | 2 | 0 | 7, 41 |
| prysm-geth-3 | Sep 4 22:27:00 – Sep 5 00:28:03 | 4 | 2 | 2 | 7, 41, 65, 130 |
| prysm-geth-300 | Sep 4 22:54:00 – Sep 5 00:27:59 | 2 | 2 | 0 | 7, 41 |
| prysm-geth-400 | Sep 4 22:54:06 – Sep 5 00:28:31 | 2 | 2 | 0 | 7, 41 |
| prysm-geth-50 | Sep 4 22:54:20 – Sep 5 00:28:45 | 3 | 2 | 1 | 7, 41, 107 |
| prysm-geth-500 | Sep 4 22:54:17 – Sep 5 00:28:29 | 2 | 2 | 0 | 7, 41 |
| prysm-geth-700 | Sep 4 22:54:33 – Sep 5 00:28:58 | 2 | 2 | 0 | 7, 41 |
| prysm-geth-900 | Sep 4 22:54:57 – Sep 5 00:29:23 | 2 | 2 | 0 | 7, 41 |

All snooper RPC bodies, including methods outside this census, parsed without
error. For the selected Engine events there are zero missing payloads, zero
missing or non-array `transactions` fields, and zero missing or malformed
`gasUsed` fields.

The Geth logs independently contain 27 `Updated payload` or `Imported new
potential chain segment` records. All 27 report `txs=0` and `gas=0` or
`mgas=0.000`; there are zero nonzero records and zero unparsed records. These
markers corroborate the decoded Engine bodies.

## Sending and txpool evidence

The saved evidence does not establish that no transaction submission was ever
attempted. There are:

- zero `eth_sendRawTransaction` or `eth_sendTransaction` requests in the
  snooper logs;
- zero transaction-submission or txpool-acceptance markers in the ten retained
  Geth logs; and
- a `Mempool transaction watcher disabled` message in every retained
  `xatu-sentry.log`.

The snooper targeted the authenticated Engine endpoint on port 8551, while a
sender would normally use the separate public JSON-RPC endpoint on port 8545.
No transaction-generator/sender log or exact historical launch manifest was
retained. Consequently the evidence proves zero included transactions in all
captured round1 payloads, but it cannot distinguish “nothing was submitted”
from submissions made elsewhere that were rejected, dropped, or never reached
these nodes.

## Reproduction

Run from the repository root:

```sh
python3 runs/diagnostics/transaction-contents/scan_round1_transactions.py --pretty
```

The scanner uses only Python's standard library. It reads the extracted logs,
associates snooper responses with request methods by call number, decodes the
actual `newPayload` request and `getPayload` response JSON, and emits every
payload observation plus per-node parse and marker counts.
