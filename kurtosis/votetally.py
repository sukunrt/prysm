#!/usr/bin/env python3
"""Reconcile a run's Goldfish head votes seat by seat, from the vote ledger.

    kurtosis/votetally.py <logdir> [--validators 132] [--slots-per-round 8]
                          [--skip-slots 32] [--only-short]

<logdir> holds one `cl-*.log` per beacon node, captured from a run whose nodes
ran with `--goldfish-vote-ledger`. Every vote a node saw is one line there, so
for each node and each slot this can say what became of every seat the mock
committee scheduled:

    expected = accepted + dropped(reason) + never_arrived

`expected` comes from the committee schedule itself (the same arithmetic as
decoupled.AvailableAttestationSeats), not from the logs, so a seat nobody ever
mentioned still shows up - as `never_arrived`, which on a clean localhost net is
a finding in its own right, not a rounding error.
"""

import argparse
import collections
import hashlib
import pathlib
import re
import struct
import sys

COMMITTEE_SIZE = 512
DOMAIN = b"decoupled_mock_goldfish_committee"

# The ledger line, as logrus renders it: `key=value` pairs after the message.
LEDGER = re.compile(r"Goldfish vote\s+(.*)$")
# A logrus field is key=value; a [32]byte root renders as "[1 2 ...]" on the
# builds before the hex fix, so a bracketed run counts as one value too.
FIELD = re.compile(r'(\w+)=("[^"]*"|\[[^\]]*\]|\S+)')
ANSI = re.compile(r"\x1b\[[0-9;]*m")

ACCEPTED = ("accepted", "replayed", "local")


def seat_offset(slot, validator_count):
    h = hashlib.sha256(DOMAIN + struct.pack(">Q", slot)).digest()
    return struct.unpack(">Q", h[:8])[0] % validator_count


def seats_for(slot, index, validator_count):
    """decoupled.AvailableAttestationSeats, in python."""
    if index >= validator_count:
        return []
    off = seat_offset(slot, validator_count)
    start = (index + validator_count - off) % validator_count
    return list(range(start, COMMITTEE_SIZE, validator_count))


def expected_seats(slot, validator_count):
    """{validator index: seats held} for one slot, the whole committee."""
    out = {}
    for index in range(validator_count):
        held = len(seats_for(slot, index, validator_count))
        if held:
            out[index] = held
    return out


def parse_ledger(path):
    """[(slot, validator, seats, outcome, reason, arrivedMs, decidedMs)]."""
    rows = []
    for raw in ANSI.sub("", path.read_text(errors="replace")).splitlines():
        m = LEDGER.search(raw)
        if not m:
            continue
        f = {k: v.strip('"') for k, v in FIELD.findall(m.group(1))}
        if "voteSlot" not in f or "validator" not in f:
            continue
        rows.append((
            int(f["voteSlot"]), int(f["validator"]), int(f.get("seats", 0)),
            f.get("outcome", "?"), f.get("reason", ""),
            int(f.get("arrivedMs", 0)), int(f.get("decidedMs", 0)),
        ))
    return rows


def reconcile(rows, validator_count, skip):
    """{(node-less) slot: {'accepted': seats, reason: seats, 'never': seats}}."""
    # A validator can appear more than once for a slot (queued then replayed);
    # the outcome that counts is the last one, since that is what forkchoice saw.
    final = {}
    for slot, validator, seats, outcome, reason, _, _ in rows:
        if slot < skip:
            continue
        final[(slot, validator)] = (seats, outcome, reason)

    per_slot = collections.defaultdict(collections.Counter)
    seen = collections.defaultdict(set)
    for (slot, validator), (seats, outcome, reason) in final.items():
        key = "accepted" if outcome in ACCEPTED else f"dropped:{reason or outcome}"
        per_slot[slot][key] += seats
        seen[slot].add(validator)

    for slot in list(per_slot):
        expected = expected_seats(slot, validator_count)
        per_slot[slot]["expected"] = sum(expected.values())
        never = sum(s for v, s in expected.items() if v not in seen[slot])
        if never:
            per_slot[slot]["never_arrived"] = never
    return per_slot


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("logdir")
    ap.add_argument("--validators", type=int, default=132,
                    help="MIN_GENESIS_ACTIVE_VALIDATOR_COUNT the run used")
    ap.add_argument("--skip-slots", type=int, default=32,
                    help="slots of warm-up dropped from the window")
    ap.add_argument("--only-short", action="store_true",
                    help="print a row only for slots that were not full")
    args = ap.parse_args()

    d = pathlib.Path(args.logdir)
    logs = sorted(d.glob("cl-*.log"), key=lambda p: int(p.name.split("-")[1]))
    if not logs:
        sys.exit(f"no cl-*.log under {d}")

    print("## Head vote reconciliation\n")
    print("Every seat the committee schedule expects, per node per slot, "
          "against\nwhat the node's vote ledger says became of it.\n")
    print("| node | slots | expected seats | accepted | dropped | never "
          "arrived | full slots |")
    print("|---|---|---|---|---|---|---|")
    shortfalls = []
    for path in logs:
        node = path.stem
        rows = parse_ledger(path)
        if not rows:
            print(f"| {node} | 0 | - | - | - | - | ledger empty |")
            continue
        per_slot = reconcile(rows, args.validators, args.skip_slots)
        exp = acc = drop = never = full = 0
        for slot, c in sorted(per_slot.items()):
            e, a = c["expected"], c["accepted"]
            n = c.get("never_arrived", 0)
            dr = sum(v for k, v in c.items()
                     if k.startswith("dropped:"))
            exp += e
            acc += a
            drop += dr
            never += n
            if a == e:
                full += 1
            else:
                shortfalls.append((node, slot, c))
        print(f"| {node} | {len(per_slot)} | {exp} | {acc} | {drop} | "
              f"{never} | {full}/{len(per_slot)} |")

    if not shortfalls:
        print("\nEvery seat of every window slot was accepted on every node.")
        return
    print("\n### Slots that were not full\n")
    print("| node | slot | expected | accepted | breakdown |")
    print("|---|---|---|---|---|")
    for node, slot, c in shortfalls:
        breakdown = " ".join(
            f"{k}={v}" for k, v in sorted(c.items())
            if k not in ("expected", "accepted"))
        print(f"| {node} | {slot} | {c['expected']} | {c['accepted']} | "
              f"{breakdown or '-'} |")


if __name__ == "__main__":
    main()
