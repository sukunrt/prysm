# Historical Round 2 transaction-content audit

## Finding

Every inspectable Engine execution payload in the currently retained historical Round 2 evidence is empty. At or after the conservative Round 2 boundary `2026-09-05T01:30:00Z`, there are **1419 parsed `getPayload`/`newPayload` observations over 128 distinct execution block hashes, zero nonempty transaction arrays, and zero records with nonzero `gasUsed`**. The 69 payload-body recovery results in that interval, over 21 requested hashes, are also all empty.

Across the complete snooper files, including records retained from before the Round 2 execution restart, all 1449 full-payload observations over 136 hashes and all 70 body-only observations over 22 hashes are empty. This combines every recorded Engine API version suffix; method-specific counts are in `round2_payload_audit.json`.

The independent Prysm beacon markers agree: **586 `Payload envelope` records covering 122 distinct beacon block roots all report `txCount=0` and `gasUsed=0`**. The Geth execution-log `txs=` markers also contain 0 nonzero records out of 1423.

No transaction-submission or acceptance marker matched the retained logs. That does **not** prove nobody tried to send a transaction. The snooper explicitly targeted the authenticated Engine endpoint `execution:8551`, while Geth separately advertised the unauthenticated public endpoint on port `8545`; a sender using the public endpoint would not appear in these snooper records. The retained services may also omit sender traffic or rejection logs. Empty included payloads prove that no transaction was included in those payloads.

## Coverage and limits

This is a fresh full-file scan of all 12 currently retained node directories: 1, 2, 3, 50, 150, 151, 201, 300, 400, 500, 700, 900. It covers each retained `snooper-engine.log` plus available `execution.log`, `beacon.log`, `validator.log`, and `xatu-sentry.log`, over their complete retained time bounds. The startup3 reproduction and later diagnostic reproductions are excluded.

The previously advertised 1,000-node temporary union at `/tmp/prysm-r2-extra-logs.Rd7MjT` no longer exists. Consequently, this audit cannot fresh-scan payload bodies for all 1,000 nodes. The 12 snooper captures are whole retained captures, but they begin before the Round 2 execution logs and contain prior history. The report therefore separates the full-file all-zero census from observations at or after the stated Round 2 boundary. The conclusion must be stated as **all execution payloads inspectable in the currently retained Round 2 logs were empty**, rather than an unqualified statement about every node or every attempted transaction in the original run.

`eth_getBlockBy*` poll responses are reported separately because they repeatedly observe the same blocks and are not new payload constructions. They contain 11055 records over 123 distinct block hashes; none are nonempty.

## Reproduction

Run from the repository root:

```sh
python3 runs/diagnostics/transaction-contents/round2_audit_transactions.py
```

The compact JSON contains exact counts and per-file coverage. `round2_engine_payloads.tsv` has one row per target Engine payload observation; `round2_log_markers.tsv` preserves the independent beacon, execution, and submission-marker evidence.
