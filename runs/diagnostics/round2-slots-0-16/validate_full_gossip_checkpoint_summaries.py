#!/usr/bin/env python3
"""Validate checkpoint-chain summaries against retained full trace events."""

import argparse
import csv
import re
import statistics
from pathlib import Path


G_RE = re.compile(r"\bG=(-?\d+)")
GOID_RE = re.compile(r"\bGoID=(\d+)")
TIME_RE = re.compile(r"\bTime=(\d+)")


def read_tsv(path):
    with path.open(newline="") as source:
        return list(csv.DictReader(source, delimiter="\t"))


def load_events(path):
    events = []
    provenance = ""
    block = []

    def flush():
        if not block:
            return
        header = block[0]
        time_match = TIME_RE.search(header)
        if time_match is None:
            return
        g_match = G_RE.search(header)
        goid_match = GOID_RE.search(header)
        events.append(
            {
                "time": int(time_match.group(1)),
                "g": None if g_match is None else int(g_match.group(1)),
                "goid": None if goid_match is None else int(goid_match.group(1)),
                "provenance": provenance,
                "text": "".join(block),
            }
        )

    for line in path.read_text().splitlines(keepends=True):
        if line.startswith("# parsed_lines="):
            flush()
            block = []
            provenance = line.removeprefix("# parsed_lines=").strip()
        elif line.startswith("M="):
            flush()
            block = [line]
        elif block:
            block.append(line)
    flush()
    return events


def load_transition_headers(path):
    rows = read_tsv(path)
    events = []
    for row in rows:
        header = row["header"]
        if header is None:
            continue
        time_match = TIME_RE.search(header)
        g_match = G_RE.search(header)
        goid_match = GOID_RE.search(header)
        if time_match is None:
            raise ValueError(f"transition has no time: {header}")
        events.append(
            {
                "time": int(time_match.group(1)),
                "g": None if g_match is None else int(g_match.group(1)),
                "goid": None if goid_match is None else int(goid_match.group(1)),
                "provenance": row["parsed_line"],
                "text": header,
            }
        )
    return events


def one(events, timestamp, *, g=None, goid=None, needles=()):
    matches = [
        event
        for event in events
        if event["time"] == timestamp
        and (g is None or event["g"] == g)
        and (goid is None or event["goid"] == goid)
        and all(needle in event["text"] for needle in needles)
    ]
    if len(matches) != 1:
        raise ValueError(
            f"expected one event time={timestamp} g={g} goid={goid} "
            f"needles={needles}, found {len(matches)}"
        )
    return matches[0]


def check_ref(label, expected, event):
    actual = event["provenance"]
    if "-" not in expected and actual == f"{expected}-{expected}":
        actual = expected
    if expected != actual:
        raise ValueError(f"{label}: TSV={expected} trace={event['provenance']}")


def validate_chain(rows, histories, gc_markers):
    if len(rows) != 61 or len({row["waiter"] for row in rows}) != 61:
        raise ValueError("checkpoint chain must contain target plus 60 unique holders")
    target = rows[0]
    if int(target["waiter"]) != 4970:
        raise ValueError("checkpoint-chain target is not G4970")
    start, end = int(target["park"]), int(target["grant"])
    if (start, end, end - start) != (
        277054191954560,
        277054661603328,
        469648768,
    ):
        raise ValueError("unexpected G4970 checkpoint interval")

    intervals = []
    ready_ns = 0
    running_ns = 0
    bodies = []
    for index, row in enumerate(rows[1:], start=1):
        goid = int(row["waiter"])
        grant = int(row["grant"])
        first_run = int(row["first_run"])
        release = int(row["outgoing_release"])
        expected_waker = rows[index + 1]["waiter"] if index + 1 < len(rows) else "4931"
        if row["incoming_waker"] != expected_waker:
            raise ValueError(f"G{goid} incoming-waker chain mismatch")
        if release != int(rows[index - 1]["grant"]):
            raise ValueError(f"G{goid} outgoing release does not grant downstream waiter")
        if not grant <= first_run <= release:
            raise ValueError(f"G{goid} grant/run/release order mismatch")

        clipped_grant = max(grant, start)
        clipped_release = min(release, end)
        if clipped_grant < first_run:
            intervals.append((clipped_grant, first_run, "ready", goid))
            ready_ns += first_run - clipped_grant
        if first_run < clipped_release:
            intervals.append((first_run, clipped_release, "running", goid))
            running_ns += clipped_release - first_run
        bodies.append(release - first_run)

        intervening = [
            event
            for event in histories
            if first_run < event["time"] < release
            and (event["g"] == goid or event["goid"] == goid)
            and "StateTransition" in event["text"]
        ]
        if intervening:
            raise ValueError(
                f"G{goid} has {len(intervening)} intervening transition(s) "
                "between first run and release"
            )

    cursor = start
    for interval_start, interval_end, _, goid in sorted(intervals):
        if interval_start != cursor:
            raise ValueError(
                f"chain coverage gap/overlap at {cursor}, next G{goid} starts {interval_start}"
            )
        cursor = interval_end
    if cursor != end:
        raise ValueError(f"chain coverage stops at {cursor}, expected {end}")
    if (ready_ns, running_ns, ready_ns + running_ns) != (
        468388673,
        1260095,
        469648768,
    ):
        raise ValueError("checkpoint-chain duration partition mismatch")

    sweep_begin = sweep_end = None
    for line in gc_markers.read_text().splitlines():
        if 'Name="stop-the-world (GC sweep termination)"' not in line:
            continue
        timestamp = int(TIME_RE.search(line).group(1))
        if "RangeBegin" in line:
            sweep_begin = timestamp
        elif "RangeEnd" in line:
            sweep_end = timestamp
    if (sweep_begin, sweep_end, sweep_end - sweep_begin) != (
        277054595695552,
        277054595804288,
        108736,
    ):
        raise ValueError("GC sweep-termination marker mismatch")
    containing_ready = [
        goid
        for interval_start, interval_end, kind, goid in intervals
        if kind == "ready" and interval_start <= sweep_begin < sweep_end <= interval_end
    ]
    if containing_ready != [4961]:
        raise ValueError(f"GC sweep not inside expected G4961 ready interval: {containing_ready}")

    return {
        "holders": len(rows) - 1,
        "start": start,
        "end": end,
        "total": end - start,
        "ready": ready_ns,
        "running": running_ns,
        "ready_percent": ready_ns * 100 / (end - start),
        "body_min": min(bodies),
        "body_max": max(bodies),
        "intervening": 0,
        "sweep": sweep_end - sweep_begin,
        "sweep_holder": containing_ready[0],
    }


def validate_reader_gate(rows, members, events):
    if len(rows) != 98 or len({row["go"] for row in rows}) != 98:
        raise ValueError("reader gate must contain 98 unique Len entrants")
    members_by_go = {row["go"]: row for row in members}
    if sum(row["kind"] == "Len" for row in members) != 98:
        raise ValueError("member table does not contain 98 Len readers")
    member_ids = set(members_by_go)
    durations = []
    for row in rows:
        goid = int(row["go"])
        member = members_by_go.get(row["go"])
        if member is None or member["kind"] != "Len":
            raise ValueError(f"G{goid} is not a joined Len cohort member")
        release = one(
            events,
            int(row["checkpoint_release"]),
            g=goid,
            goid=int(row["next_waiter"]),
            needles=("Waiting->Runnable", "async.(*Lock).Unlock", "getAttPreState"),
        )
        check_ref(f"G{goid} checkpoint release", row["checkpoint_release_ref"], release)
        park = one(
            events,
            int(row["len_park"]),
            g=goid,
            goid=goid,
            needles=('Running->Waiting Reason="sync"', ").Len @", "ActiveValidatorCount"),
        )
        check_ref(f"G{goid} Len park", member["parkRef"], park)
        if int(member["park"]) != int(row["len_park"]):
            raise ValueError(f"G{goid} member/gate Len park mismatch")
        duration = int(row["len_park"]) - int(row["checkpoint_release"])
        if duration != int(row["release_to_park_ns"]):
            raise ValueError(f"G{goid} release-to-Len duration mismatch")
        durations.append(duration)

    next_inside = sum(row["next_waiter"] in member_ids for row in rows)
    if next_inside != 97:
        raise ValueError(f"expected 97 next waiters in cohort, found {next_inside}")
    if (min(durations), statistics.median(durations), max(durations), sum(durations)) != (
        2688,
        3232,
        201024,
        589821,
    ):
        raise ValueError("release-to-Len duration summary mismatch")
    span = int(rows[-1]["len_park"]) - int(rows[0]["checkpoint_release"])
    if span != 968256:
        raise ValueError(f"release-to-last-Len span mismatch: {span}")
    return {
        "rows": len(rows),
        "next_inside": next_inside,
        "min": min(durations),
        "median": int(statistics.median(durations)),
        "max": max(durations),
        "sum": sum(durations),
        "span": span,
    }


def validate_backlog(rows, checkpoint_events, count_events, transition_events):
    wave = 277054662602688
    if len(rows) != 134 or len({row["go"] for row in rows}) != 134:
        raise ValueError("probe14 backlog must contain 134 unique goroutines")
    for row in rows:
        goid = int(row["go"])
        waker = int(row["cpWaker"])
        next_waiter = int(row["cpNext"])
        created = one(
            transition_events,
            int(row["created"]),
            goid=goid,
            needles=("NotExist->Runnable",),
        )
        park = one(
            checkpoint_events,
            int(row["cpPark"]),
            g=goid,
            goid=goid,
            needles=('Running->Waiting Reason="chan receive"', "async.(*Lock).Lock", "getAttPreState"),
        )
        wake = one(
            checkpoint_events,
            int(row["cpWake"]),
            g=waker,
            goid=goid,
            needles=("Waiting->Runnable", "async.(*Lock).Unlock", "getAttPreState"),
        )
        first_run = one(
            transition_events,
            int(row["cpFirstRun"]),
            goid=goid,
            needles=("Runnable->Running",),
        )
        release = one(
            checkpoint_events,
            int(row["cpRelease"]),
            g=goid,
            goid=next_waiter,
            needles=("Waiting->Runnable", "async.(*Lock).Unlock", "getAttPreState"),
        )
        preempt = one(
            count_events,
            int(row["firstProbeCountPreempt"]),
            g=goid,
            goid=goid,
            needles=('Running->Runnable Reason="preempted"', "ActiveValidatorCount"),
        )
        for column, event in (
            ("createdRef", created),
            ("cpParkRef", park),
            ("cpWakeRef", wake),
            ("cpFirstRunRef", first_run),
            ("cpReleaseRef", release),
            ("countPreemptRef", preempt),
        ):
            check_ref(f"G{goid} {column}", row[column], event)
        created_time = int(row["created"])
        park_time = int(row["cpPark"])
        wake_time = int(row["cpWake"])
        run_time = int(row["cpFirstRun"])
        release_time = int(row["cpRelease"])
        preempt_time = int(row["firstProbeCountPreempt"])
        if not created_time < wave or not park_time < wave or not wake_time > wave:
            raise ValueError(f"G{goid} does not straddle finalizer wave as claimed")
        if not wake_time <= run_time < release_time < preempt_time:
            raise ValueError(f"G{goid} checkpoint-to-Count ordering mismatch")
        if int(row["cpWait"]) != wake_time - park_time:
            raise ValueError(f"G{goid} checkpoint wait mismatch")

    waits = [int(row["cpWait"]) for row in rows]
    return {
        "rows": len(rows),
        "created_min": min(int(row["created"]) for row in rows),
        "created_max": max(int(row["created"]) for row in rows),
        "park_min": min(int(row["cpPark"]) for row in rows),
        "park_max": max(int(row["cpPark"]) for row in rows),
        "wake_min": min(int(row["cpWake"]) for row in rows),
        "wake_max": max(int(row["cpWake"]) for row in rows),
        "wait_min": min(waits),
        "wait_max": max(waits),
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("chain", type=Path)
    parser.add_argument("reader_gate", type=Path)
    parser.add_argument("members", type=Path)
    parser.add_argument("backlog", type=Path)
    parser.add_argument("chain_histories", type=Path)
    parser.add_argument("cohort_events", type=Path)
    parser.add_argument("backlog_checkpoint_events", type=Path)
    parser.add_argument("backlog_transition_headers", type=Path)
    parser.add_argument("count_events", type=Path)
    parser.add_argument("gc_markers", type=Path)
    args = parser.parse_args()

    chain = validate_chain(
        read_tsv(args.chain), load_events(args.chain_histories), args.gc_markers
    )
    gate = validate_reader_gate(
        read_tsv(args.reader_gate),
        read_tsv(args.members),
        load_events(args.cohort_events),
    )
    backlog = validate_backlog(
        read_tsv(args.backlog),
        load_events(args.backlog_checkpoint_events),
        load_events(args.count_events),
        load_transition_headers(args.backlog_transition_headers),
    )
    print(
        f"chain_holders={chain['holders']} start={chain['start']} end={chain['end']} "
        f"total_ns={chain['total']} ready_ns={chain['ready']} "
        f"first_run_to_release_ns={chain['running']} "
        f"ready_percent={chain['ready_percent']:.6f} "
        f"body_min_ns={chain['body_min']} body_max_ns={chain['body_max']} "
        f"intervening_state_transitions={chain['intervening']} "
        f"gc_sweep_ns={chain['sweep']} gc_ready_holder={chain['sweep_holder']}"
    )
    print(
        f"reader_gate_rows={gate['rows']} next_waiter_in_cohort={gate['next_inside']} "
        f"release_to_len_min_ns={gate['min']} median_ns={gate['median']} "
        f"max_ns={gate['max']} sum_ns={gate['sum']} "
        f"first_release_to_last_len_park_ns={gate['span']}"
    )
    print(
        f"probe14_backlog_rows={backlog['rows']} "
        f"created={backlog['created_min']}..{backlog['created_max']} "
        f"checkpoint_park={backlog['park_min']}..{backlog['park_max']} "
        f"checkpoint_wake={backlog['wake_min']}..{backlog['wake_max']} "
        f"checkpoint_wait_ns={backlog['wait_min']}..{backlog['wait_max']} "
        "all_created_and_parked_before_wave=true all_woken_after_wave=true "
        "all_wake_run_release_preempt_ordered=true"
    )


if __name__ == "__main__":
    main()
