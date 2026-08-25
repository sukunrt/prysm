#!/usr/bin/env python3
"""Per-slot attester, proposal and late-publish counts from the VC logs.

    kurtosis/vclogs.py <logdir> [--first-slot 32] [--last-slot 999999]

<logdir> holds one `vc-*.log` per validator client (docker logs output). The
tables mirror the ethshadow baseline's "Attesters per slot, from the validator
clients' logs" section, plus the late-publisher evidence step 6 asks for:
which slots were published late, by which proposer, and by how much.
"""

import argparse
import pathlib
import re
from collections import defaultdict

ANSI = re.compile(r"\x1b\[[0-9;]*m")
SLOT = re.compile(r"slot=(\d+)")
PUBKEYS = re.compile(r"pubkeys=\[([^\]]*)\]")
PROPOSER = re.compile(r"proposerIndex=(\d+)")
DELAY = re.compile(r"delay=(\S+)")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("logdir")
    ap.add_argument("--first-slot", type=int, default=32)
    ap.add_argument("--last-slot", type=int, default=10**9)
    args = ap.parse_args()

    attesters = defaultdict(int)
    aggregators = defaultdict(int)
    blocks = defaultdict(int)
    late = []
    logs = sorted(pathlib.Path(args.logdir).glob("vc-*.log"))
    for path in logs:
        for raw in path.read_text(errors="replace").splitlines():
            line = ANSI.sub("", raw)
            m = SLOT.search(line)
            if not m:
                continue
            slot = int(m.group(1))
            if not args.first_slot <= slot <= args.last_slot:
                continue
            if "Submitted new attestations" in line:
                keys = PUBKEYS.search(line)
                attesters[slot] += len(keys.group(1).split()) if keys else 0
            elif "Submitted new aggregate attestations" in line:
                keys = PUBKEYS.search(line)
                aggregators[slot] += len(keys.group(1).split()) if keys else 0
            elif "Submitted new block" in line:
                blocks[slot] += 1
            elif "Publishing block late" in line:
                idx = PROPOSER.search(line)
                dly = DELAY.search(line)
                late.append((slot, idx.group(1) if idx else "?",
                             dly.group(1) if dly else "?"))

    slots = sorted(attesters)
    print(f"validator client logs read: {len(logs)}")
    if slots:
        print(f"slots with attestations: {len(slots)} "
              f"(slot {slots[0]} to {slots[-1]})")
        vals = [attesters[s] for s in slots]
        print(f"unaggregated attesters per slot: mean {sum(vals)/len(vals):.2f}"
              f", min {min(vals)}, max {max(vals)}")
        agg = [aggregators[s] for s in slots]
        print(f"aggregators per slot: mean {sum(agg)/len(agg):.2f}")
        print(f"blocks proposed: {sum(blocks.values())} over "
              f"{len(blocks)} slots")
        print("\nDistinct attesters per round-offset (flat if the shuffle"
              " repeats every round):")
        for off in range(8):
            sel = [attesters[s] for s in slots if s % 8 == off]
            if sel:
                print(f"  slot % 8 == {off}: {sum(sel)/len(sel):.2f}"
                      f" attesters/slot over {len(sel)} slots")

    print(f"\nlate-published blocks: {len(late)}")
    if late:
        print("\n| slot | proposer index | delay into slot |")
        print("|---|---|---|")
        for slot, idx, dly in sorted(late):
            print(f"| {slot} | {idx} | {dly} |")

    print("\n| slot | attesters | aggregators | blocks |")
    print("|---|---|---|---|")
    for slot in slots:
        print(f"| {slot} | {attesters[slot]} | {aggregators[slot]} |"
              f" {blocks.get(slot, 0)} |")


if __name__ == "__main__":
    main()
