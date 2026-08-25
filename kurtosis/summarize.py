#!/usr/bin/env python3
"""Turn kurtosis/scrape.sh output into the step-6 measurement tables.

    kurtosis/summarize.py <scrapedir> [--slot-seconds 6] [--skip-slots 32]

Counters are differenced between the first and last sample of the measurement
window and divided by the number of slots in it, which gives the same
per-slot-per-node numbers the ethshadow baseline reports
(decoupled-shadow-sim/data19/baseline.md).
"""

import argparse
import pathlib
import re
import sys
from collections import defaultdict

SAMPLE = re.compile(r"^# TS (\d+)$")
LINE = re.compile(r"^([a-z_0-9]+)(?:\{(.*)\})? ([0-9.e+-]+)$")

# The gossip families the baseline tabulates, keyed by the topic name that
# appears between the fork digest and the encoding.
FAMILIES = {
    "available_attestation": "available_attestation",
    "beacon_aggregate_and_proof": "beacon_aggregate_and_proof",
    "beacon_block": "beacon_block",
    "voluntary_exit": "voluntary_exit",
}
FAMILY_PREFIXES = {
    "beacon_attestation_": "beacon_attestation_*",
    "data_column_sidecar_": "data_column_sidecar_*",
    "sync_committee_": "sync_committee_*",
}

# prysm exports no received-bytes counter (p2p_pubsub_rpc_recv_pub_bytes_total
# is never incremented), so received bytes are derived the same way the
# ethshadow baseline derived them: message count x the topic family's mean
# message size, taken from the sent counters.
COUNTERS = {
    "p2p_pubsub_deliver_total": "deliver",
    # What the beacon node's subscriber actually handed to the app, local
    # publishes included: this is the "N available attestations per slot per
    # node" the plan checks, where deliver counts only what came off the wire.
    "p2p_message_received_total": "app",
    "p2p_pubsub_duplicate_total": "dup",
    "p2p_pubsub_rpc_sent_pub_total": "sent_msgs",
    "gossipsub_topic_msg_sent_bytes": "sent_bytes",
}

GOLDFISH = [
    "goldfish_seat_fraction",
    "goldfish_gate_stop_total",
    "goldfish_gate_retreat",
    "goldfish_late_vote_total",
    "goldfish_round_proposal_total",
    "goldfish_round_proposal_conflict_total",
    "goldfish_equivocation_total",
    "beacon_reorgs_total",
]

CHAIN = [
    "beacon_head_slot",
    "beacon_clock_time_slot",
    "beacon_finalized_epoch",
    "beacon_current_justified_epoch",
]


def topic_family(labels):
    m = re.search(r'topic="([^"]*)"', labels or "")
    if not m:
        return None
    parts = m.group(1).split("/")
    if len(parts) < 4:
        return None
    name = parts[3]
    if name in FAMILIES:
        return FAMILIES[name]
    for prefix, family in FAMILY_PREFIXES.items():
        if name.startswith(prefix):
            return family
    return None


def parse(path):
    """[(ts, {metric: value}, {(family, counter): value})] for one node."""
    samples = []
    ts = None
    scalars, families = {}, defaultdict(float)
    for raw in path.read_text().splitlines():
        m = SAMPLE.match(raw)
        if m:
            if ts is not None:
                samples.append((ts, scalars, families))
            ts, scalars, families = int(m.group(1)), {}, defaultdict(float)
            continue
        if raw.startswith("#"):
            continue
        m = LINE.match(raw)
        if not m:
            continue
        name, labels, value = m.group(1), m.group(2), float(m.group(3))
        if name in COUNTERS:
            family = topic_family(labels)
            if family:
                families[(family, COUNTERS[name])] += value
        elif name == "p2p_pubsub_topic_active":
            family = topic_family(labels)
            if family:
                families[(family, "subscribed")] += value
        elif not labels:
            scalars[name] = value
        elif name == "p2p_peer_count" and 'Connected' in labels:
            scalars[name] = value
        elif name.endswith("_count") or name.endswith("_sum"):
            scalars[name] = scalars.get(name, 0.0) + value
    if ts is not None:
        samples.append((ts, scalars, families))
    return samples


def fmt(x, nd=1):
    return f"{x:,.{nd}f}"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("scrapedir")
    ap.add_argument("--slot-seconds", type=float, default=6.0)
    ap.add_argument("--skip-slots", type=int, default=32,
                    help="slots of warm-up dropped from the window")
    args = ap.parse_args()

    d = pathlib.Path(args.scrapedir)
    nodes = sorted(d.glob("cl-*.metrics"),
                   key=lambda p: int(p.name.split("-")[1]))
    if not nodes:
        sys.exit(f"no cl-*.metrics under {d}")

    per_node = {}
    for path in nodes:
        samples = parse(path)
        if len(samples) < 2:
            print(f"WARNING: {path.name} has {len(samples)} samples")
            continue
        per_node[path.stem] = samples

    # Window: drop the warm-up, keep everything after it.
    skip = args.skip_slots
    print("## Window\n")
    windows = {}
    for name, samples in per_node.items():
        start = None
        for i, (_, sc, _) in enumerate(samples):
            if sc.get("beacon_clock_time_slot", 0) >= skip:
                start = i
                break
        if start is None or start >= len(samples) - 1:
            start = 0
        windows[name] = (samples[start], samples[-1])
    ref = next(iter(windows.values()))
    first_slot = ref[0][1].get("beacon_clock_time_slot", 0)
    last_slot = ref[1][1].get("beacon_clock_time_slot", 0)
    span = max(1.0, last_slot - first_slot)
    print(f"slot {first_slot:.0f} -> {last_slot:.0f} ({span:.0f} slots, "
          f"{span * args.slot_seconds:.0f}s of chain time), "
          f"{len(per_node)} beacon nodes\n")

    print("## Finalization (end of run)\n")
    print("| node | head slot | justified | finalized | reorgs | peers |")
    print("|---|---|---|---|---|---|")
    for name, (_, (_, sc, _)) in windows.items():
        print(f"| {name} | {sc.get('beacon_head_slot', 0):.0f} | "
              f"{sc.get('beacon_current_justified_epoch', 0):.0f} | "
              f"{sc.get('beacon_finalized_epoch', 0):.0f} | "
              f"{sc.get('beacon_reorgs_total', 0):.0f} | "
              f"{sc.get('p2p_peer_count', 0):.0f} |")

    print("\n## Finalized epoch over time (one row per node)\n")
    marks = sorted({int(sc.get("beacon_clock_time_slot", 0)) // 32
                    for s in per_node.values() for (_, sc, _) in s})
    print("| node | " + " | ".join(f"e{m}" for m in marks) + " |")
    print("|---" * (len(marks) + 1) + "|")
    for name, samples in per_node.items():
        cells = []
        for m in marks:
            val = ""
            for _, sc, _ in samples:
                if int(sc.get("beacon_clock_time_slot", 0)) // 32 == m:
                    val = f"{sc.get('beacon_finalized_epoch', 0):.0f}"
            cells.append(val or "-")
        print(f"| {name} | " + " | ".join(cells) + " |")

    print("\n## Gossip traffic, per slot per node\n")
    fam_names = sorted({f for _, (_, _, fa) in
                        [(n, w[1]) for n, w in windows.items()]
                        for (f, _) in fa})
    for family in fam_names:
        print(f"\n### {family}\n")
        print("| node | msgs/slot in | dup msgs/slot |"
              " recv bytes/slot (derived) | msgs/slot out | sent bytes/slot |"
              " app msgs/slot |")
        print("|---|---|---|---|---|---|---|")
        agg = defaultdict(list)
        sizes = []
        for name, ((_, _, fa0), (_, _, fa1)) in windows.items():
            row = {}
            for key in ("deliver", "dup", "sent_msgs", "sent_bytes",
                        "app"):
                row[key] = (fa1[(family, key)] - fa0[(family, key)]) / span
            size = row["sent_bytes"] / row["sent_msgs"] if row["sent_msgs"] \
                else 0.0
            sizes.append(size)
            row["recv_bytes"] = (row["deliver"] + row["dup"]) * size
            for key, val in row.items():
                agg[key].append(val)
            print(f"| {name} | {fmt(row['deliver'])} | {fmt(row['dup'])} | "
                  f"{fmt(row['recv_bytes'], 0)} | {fmt(row['sent_msgs'])} | "
                  f"{fmt(row['sent_bytes'], 0)} | {fmt(row['app'])} |")
        n = max(1, len(windows))
        print(f"| **mean** | **{fmt(sum(agg['deliver']) / n)}** | "
              f"**{fmt(sum(agg['dup']) / n)}** | "
              f"**{fmt(sum(agg['recv_bytes']) / n, 0)}** | "
              f"**{fmt(sum(agg['sent_msgs']) / n)}** | "
              f"**{fmt(sum(agg['sent_bytes']) / n, 0)}** | "
              f"**{fmt(sum(agg['app']) / n)}** |")
        live = [x for x in sizes if x]
        if live:
            print(f"\nMean gossip message size on this topic family: "
                  f"{sum(live) / len(live):.0f} bytes.")

    print("\n## Goldfish metrics (window delta; seat fraction is a gauge)\n")
    print("| node | " + " | ".join(m.replace("goldfish_", "gf_")
                                   for m in GOLDFISH) + " |")
    print("|---" * (len(GOLDFISH) + 1) + "|")
    seat_all = []
    for name, ((_, sc0, _), (_, sc1, _)) in windows.items():
        cells = []
        for m in GOLDFISH:
            if m == "goldfish_seat_fraction":
                vals = [sc.get(m) for _, sc, _ in per_node[name]
                        if sc.get(m) is not None]
                vals = [v for v in vals if v > 0]
                seat_all += vals
                if vals:
                    cells.append(f"{min(vals):.2f}/{sum(vals) / len(vals):.2f}"
                                 f"/{max(vals):.2f}")
                else:
                    cells.append("-")
            else:
                cells.append(f"{sc1.get(m, 0) - sc0.get(m, 0):.0f}")
        print(f"| {name} | " + " | ".join(cells) + " |")
    if seat_all:
        print(f"\nseat_fraction over all nodes and samples: "
              f"min {min(seat_all):.2f}, mean "
              f"{sum(seat_all) / len(seat_all):.3f}, max {max(seat_all):.2f}, "
              f"samples {len(seat_all)}")

    print("\n## Slots at which a counter moved (from the 1-per-slot samples)\n")
    print("| node | gate_retreat at slots | reorgs at slots | gate_stop at"
          " slots |")
    print("|---|---|---|---|")
    for name, samples in per_node.items():
        moved = {"goldfish_gate_retreat": [], "beacon_reorgs_total": [],
                 "goldfish_gate_stop_total": []}
        for (_, prev, _), (_, cur, _) in zip(samples, samples[1:]):
            slot = int(cur.get("beacon_clock_time_slot", 0))
            if slot < skip:
                continue
            for metric, hits in moved.items():
                delta = cur.get(metric, 0) - prev.get(metric, 0)
                if delta > 0:
                    hits.append(f"{slot}" + (f"x{delta:.0f}" if delta > 1
                                             else ""))
        print(f"| {name} | " + " | ".join(
            " ".join(moved[m]) or "-" for m in
            ("goldfish_gate_retreat", "beacon_reorgs_total",
             "goldfish_gate_stop_total")) + " |")

    print("\n## Supernode health\n")
    print("| node | column subnets subscribed | column msgs/slot |"
          " columns written | columns on disk | col-by-range served |"
          " built | gossip-verified | recovered from EL | cpu |")
    print("|---|---|---|---|---|---|---|---|---|---|")
    for name, ((t0, sc0, fa0), (t1, sc1, fa1)) in windows.items():
        wall = max(1.0, t1 - t0)
        served = (sc1.get("rpc_data_columns_by_range_response_latency_"
                          "milliseconds_count", 0)
                  - sc0.get("rpc_data_columns_by_range_response_latency_"
                            "milliseconds_count", 0))
        cols = (fa1[("data_column_sidecar_*", "deliver")]
                - fa0[("data_column_sidecar_*", "deliver")]) / span
        cpu = (sc1.get("process_cpu_seconds_total", 0)
               - sc0.get("process_cpu_seconds_total", 0)) / wall

        def delta(metric):
            return sc1.get(metric, 0) - sc0.get(metric, 0)

        # Sidecars this node built for its own proposals, sidecars it verified
        # off gossip, and sidecars it rebuilt from the execution layer's blobs.
        built = delta("beacon_data_column_sidecar_computation_"
                      "milliseconds_count")
        verified = delta("beacon_data_column_sidecar_gossip_verification_"
                         "milliseconds_count")
        recovered = delta("data_columns_recovered_from_el_total")
        print(f"| {name} | {fa1[('data_column_sidecar_*', 'subscribed')]:.0f}"
              f" | {cols:.1f} | {sc1.get('data_column_written', 0):.0f} | "
              f"{sc1.get('data_column_disk_count', 0):.0f} | {served:.0f} | "
              f"{built:.0f} | {verified:.0f} | {recovered:.0f} | "
              f"{cpu:.2f} |")


if __name__ == "__main__":
    main()
