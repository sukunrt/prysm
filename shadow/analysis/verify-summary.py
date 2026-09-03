#!/usr/bin/env python3
"""Verify the per-slot summary log lines of a shadow run.

    ./verify-summary.py runs/<name>/data <nodes>

Mirrors testing/endtoend/evaluators/summary_log.go: shape checks on every
node and slot in the window, the cross-node block check, and the ledger
cross-checks (the ledger flag is on in every shadow run).
"""
import collections
import glob
import re
import sys

data, n = sys.argv[1], int(sys.argv[2])
SLOT_MS = 12000
cfg = open(f"{data}/metadata/config.yaml").read()
bps = int(re.search(r"^AGGREGATE_DUE_BPS_GLOAS:\s*(\d+)", cfg, re.M).group(1))
DUE_MS = bps * SLOT_MS // 10000
TOK = re.compile(r'(\w+)=("(?:[^"\\]|\\.)*"|\S+)')
fails = []


def bad(m):
    fails.append(m)
    print("  FAIL:", m)


def parse(line):
    d = {}
    for k, v in TOK.findall(line):
        if v.startswith('"'):
            v = v[1:-1]
        d[k] = v
    return d


def num(d, k):
    return int(d[k])


nodes = {}
for i in range(1, n + 1):
    paths = glob.glob(f"{data}/node{i}/prysm/logs/beacon-chain.log") \
        + glob.glob(f"{data}/node{i}[a-z]*/prysm/logs/beacon-chain.log")
    assert len(paths) == 1, paths
    summary = collections.defaultdict(dict)
    dup = collections.Counter()
    ledger = collections.defaultdict(list)
    total = 0
    for line in open(paths[0], errors="replace"):
        total += 1
        if "purpose=goldfish-summary" in line:
            d = parse(line)
            key = (d["msg"], num(d, "slot"))
            if key in summary[d["msg"]]:
                dup[key] += 1
            summary[d["msg"]][num(d, "slot")] = d
        elif 'msg="Goldfish vote"' in line or 'msg="FFG vote"' in line \
                or 'msg="FFG vote included"' in line:
            d = parse(line)
            ledger[d["msg"]].append(d)
    nodes[i] = (summary, dup, ledger, total)

print(f"AGGREGATE_DUE_BPS_GLOAS={bps} dueMs={DUE_MS}")
print()
print("volume per node: total lines / summary lines / ledger lines")
for i, (summary, dup, ledger, total) in nodes.items():
    s = sum(len(v) for v in summary.values())
    l = sum(len(v) for v in ledger.values())
    print(f"  node{i}: {total} / {s} / {l}   "
          + " ".join(f"{k}={len(v)}" for k, v in sorted(summary.items())))

head = min(max(nodes[i][0]["Goldfish votes"]) for i in nodes)
window = range(2, head - 1)
print(f"\nwindow: slots {window.start}..{window.stop - 1} (head {head})")

for i, (summary, dup, ledger, total) in nodes.items():
    print(f"\n== node{i}")
    for key, c in dup.items():
        bad(f"node{i} duplicate summary line {key} x{c + 1}")
    gf, ffg = summary["Goldfish votes"], summary["FFG votes"]
    blk, pl = summary["Block received"], summary["Payload received"]
    gv = collections.defaultdict(lambda: [0, 0])
    for d in ledger["Goldfish vote"]:
        if d.get("outcome") in ("accepted", "replayed", "local"):
            e = gv[num(d, "voteSlot")]
            e[0] += 1
            e[1] += num(d, "seats")
    fv = collections.defaultdict(lambda: [0, 0])
    for d in ledger["FFG vote"]:
        if d.get("outcome") == "gossip":
            a = num(d, "arrivedMs")
            e = fv[num(d, "attSlot")]
            if a < DUE_MS:
                e[0] += 1
            if abs(a - DUE_MS) <= 50:
                e[1] += 1
    inc = collections.Counter()
    for d in ledger["FFG vote included"]:
        inc[num(d, "blockSlot")] += num(d, "seats")
    for s in window:
        if s not in gf:
            bad(f"node{i} slot {s}: no Goldfish votes line")
        else:
            d = gf[s]
            seats, votes, cs = num(d, "seats"), num(d, "votes"), num(d, "committeeSeats")
            if not (0 < seats <= cs):
                bad(f"node{i} slot {s}: seats={seats} committeeSeats={cs}")
            if seats * 3 < cs * 2:
                bad(f"node{i} slot {s}: seats={seats} below 2/3 of {cs}")
            if votes > seats:
                bad(f"node{i} slot {s}: votes={votes} > seats={seats}")
            lv, ls = gv[s]
            if (votes, seats) != (lv, ls):
                bad(f"node{i} slot {s}: Goldfish votes={votes} seats={seats}, "
                    f"ledger votes={lv} seats={ls}")
        if s not in ffg:
            bad(f"node{i} slot {s}: no FFG votes line")
        else:
            d = ffg[s]
            per = [p for p in d["perSubnet"].split(",") if p]
            votes, seats, subnets = num(d, "votes"), num(d, "seats"), num(d, "subnets")
            if len(per) != subnets:
                bad(f"node{i} slot {s}: perSubnet has {len(per)} entries, subnets={subnets}")
            if sum(int(p.split(":")[1]) for p in per) != votes:
                bad(f"node{i} slot {s}: perSubnet sum != votes={votes}")
            if seats < votes:
                bad(f"node{i} slot {s}: FFG seats={seats} < votes={votes}")
            lv, tol = fv[s]
            if abs(votes - lv) > tol:
                bad(f"node{i} slot {s}: FFG votes={votes}, ledger before due={lv} (tol {tol})")
        if s in blk:
            d = blk[s]
            a = num(d, "arrivedMs")
            if not (0 <= a < SLOT_MS):
                bad(f"node{i} slot {s}: block arrivedMs={a}")
            if num(d, "bytes") <= 0:
                bad(f"node{i} slot {s}: block bytes={d['bytes']}")
            if num(d, "ffgSeats") != inc[s]:
                bad(f"node{i} slot {s}: block ffgSeats={d['ffgSeats']}, "
                    f"ledger included seats={inc[s]}")
        if s in pl:
            d = pl[s]
            if num(d, "payloadBytes") <= 0:
                bad(f"node{i} slot {s}: payloadBytes={d['payloadBytes']}")
            if s in blk and num(d, "arrivedMs") < num(blk[s], "arrivedMs"):
                bad(f"node{i} slot {s}: payload arrivedMs={d['arrivedMs']} "
                    f"< block arrivedMs={blk[s]['arrivedMs']}")
    print(f"  slots with block line: {sorted(s for s in blk if s in window)}")
    print(f"  slots with payload line: {sorted(s for s in pl if s in window)}")
    print("  sample:", {k: v for k, v in gf[window.start].items()
                        if k not in ("time", "level", "package")})
    print("  sample:", {k: v for k, v in ffg[window.start].items()
                        if k not in ("time", "level", "package")})

print("\n== cross-node")
for s in window:
    with_payload = [i for i in nodes if s in nodes[i][0]["Payload received"]]
    with_block = [i for i in nodes if s in nodes[i][0]["Block received"]]
    if with_payload and len(with_block) < n - 1:
        bad(f"slot {s}: payload on {with_payload}, block line only on {with_block}")
    if with_payload and len(with_payload) != n:
        bad(f"slot {s}: payload line on {with_payload}, want all {n}")

print()
print("RESULT:", "PASSED" if not fails else f"{len(fails)} FAILED")
sys.exit(len(fails))
