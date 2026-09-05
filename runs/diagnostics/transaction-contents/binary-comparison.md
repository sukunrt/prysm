# Historical round1/round2 binary comparison

The retained startup logs confirm that both Prysm components and Geth changed
between the runs. Round2 used an older Prysm source revision and older recorded
build timestamps. This is a comparison of runtime-reported build identities;
the archives do not provide executable SHA checksums.

## Coverage

All 22 locally retained archives were scanned for startup identities in
`beacon.log`, `validator.log`, and `execution.log`:

- Round1: nodes 1, 2, 3, 50, 201, 300, 400, 500, 700, 900 (10 archives).
- Round2: those nodes plus 150 and 151 (12 archives).
- All 20 round1 Prysm startup records (10 beacon, 10 validator) report
  `a1679c9fd82a47b3cea16ca65c84d2c4d4501fcb`.
- All 24 round2 Prysm startup records (12 beacon, 12 validator) report
  `0280403c70d88967f49d2d4c730f4c5417dabdf5`.
- No additional startup identities appeared in the scanned members.

This confirms the change across the retained sample, not an independently
verified census of all 1,000 deployed nodes in either run. Snooper and Xatu logs
were excluded from this identity comparison because retained round2 copies can
contain earlier run history.

## Prysm identities and raw anchors

Both rounds label Prysm `v5.0.3`; the embedded source revision and build time differ.

| Component | Round1 revision / build time (UTC) | Round2 revision / build time (UTC) |
| --- | --- | --- |
| Beacon chain | `a1679c9` / 2026-09-04 10:02:40 | `0280403` / 2026-08-27 16:11:36 |
| Validator | `a1679c9` / 2026-09-04 10:03:08 | `0280403` / 2026-08-27 16:11:36 |

Raw evidence is archive-member line 1 of both `./beacon.log` and
`./validator.log` in each of:

- `runs/round1/round1-prysm-geth-1.tar.gz`
- `runs/round2/round2-prysm-geth-1.tar.gz`

In those same archives, `./beacon.log:28` associates each process with the
appropriate genesis: 2026-09-05 00:00:00 UTC for round1 and 01:30:00 UTC for round2,
both with 120,000 validators.

Read-only jj history confirms that `0280403` is an ancestor of `a1679c9`.
The revision range `0280403..a1679c9` contains seven commits, including the
scratch-space change, per-slot summary logs, and aggregator configuration work.
Thus the observed Prysm change went back to an earlier source revision in round2.

## Geth identities and raw anchors

| Run / sampled nodes | Runtime instance string |
| --- | --- |
| Round1: 1, 2, 3, 700, 900 | `Geth/v1.17.6-unstable-d799b1a3-20260904/linux-amd64/go1.27.1` |
| Round1: 50, 201, 300, 400, 500 | `Geth/v1.17.6-unstable-ff083d45-20260903/linux-amd64/go1.27.1` |
| Round2: all 12 retained nodes | `Geth/v1.17.6-unstable-aa1f2fcf-20260813/linux-amd64/go1.26.5` |

Representative raw `Starting peer-to-peer node` records:

- `runs/round1/round1-prysm-geth-1.tar.gz`, member `./execution.log:66`.
- `runs/round1/round1-prysm-geth-400.tar.gz`, member `./execution.log:66`.
- `runs/round2/round2-prysm-geth-1.tar.gz`, member `./execution.log:65`.

Round1 therefore already contained two Geth build identities within the sample;
round2's sampled Geth identity differs from both. The logs establish this build
change without establishing which changes caused the runs' behavioral differences.
