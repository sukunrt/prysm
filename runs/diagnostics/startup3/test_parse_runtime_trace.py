import importlib.util
import json
import pathlib
import subprocess
import sys
import tempfile
import unittest


MODULE_PATH = pathlib.Path(__file__).with_name("parse_runtime_trace.py")
SPEC = importlib.util.spec_from_file_location("parse_runtime_trace", MODULE_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class StateTimelineTest(unittest.TestCase):
    def test_counts_each_runnable_interval_across_preemption(self):
        events = [
            {"time": 10, "old": "Waiting", "new": "Runnable", "reason": ""},
            {"time": 20, "old": "Runnable", "new": "Running", "reason": ""},
            {"time": 30, "old": "Running", "new": "Runnable", "reason": "preempted"},
            {"time": 50, "old": "Runnable", "new": "Running", "reason": ""},
        ]

        intervals = MODULE.state_timeline(events, events[0], 60)

        self.assertEqual(
            ["Runnable", "Running", "Runnable", "Running"],
            [interval["state"] for interval in intervals],
        )
        self.assertEqual(
            30,
            sum(
                interval["duration_ns"]
                for interval in intervals
                if interval["state"] == "Runnable"
            ),
        )
        self.assertEqual(
            20,
            sum(
                interval["duration_ns"]
                for interval in intervals
                if interval["state"] == "Running"
            ),
        )

    def test_goid_filter_and_request_channel_then_network_wait(self):
        fixture = """\
M=1 P=0 G=7 StateTransition Time=10 GoID=7 Running->Waiting Reason=\"chan receive\" GoState=\"\"\nTransitionStack=\n\tnet/http.(*persistConn).roundTrip @ 0x1\nM=1 P=0 G=7 StateTransition Time=20 GoID=7 Waiting->Runnable Reason=\"\" GoState=\"\"\nM=1 P=0 G=99 StateTransition Time=21 GoID=99 Waiting->Runnable Reason=\"\" GoState=\"\"\nM=1 P=0 G=7 StateTransition Time=30 GoID=7 Runnable->Running Reason=\"\" GoState=\"\"\nM=1 P=0 G=7 StateTransition Time=40 GoID=7 Running->Waiting Reason=\"network\" GoState=\"\"\nTransitionStack=\n\tinternal/poll.(*FD).Read @ 0x2\nM=1 P=0 G=7 StateTransition Time=100 GoID=7 Waiting->Runnable Reason=\"\" GoState=\"\"\nM=1 P=0 G=7 StateTransition Time=120 GoID=7 Runnable->Running Reason=\"\" GoState=\"\"\nM=1 P=0 G=7 Log Time=130 Task=0 Category=\"startup.engine\" Message=\"phase=execution.GetPayload.http.GotFirstResponseByte slot=2 payload_id=abcd wall_unix_nano=1000130\"\n"""
        with tempfile.NamedTemporaryFile("w", suffix=".txt") as source:
            source.write(fixture)
            source.flush()
            output = subprocess.check_output(
                [sys.executable, str(MODULE_PATH), "--goid", "7", source.name], text=True
            )
        record = json.loads(output)
        self.assertEqual(7, record["goid"])
        self.assertEqual(60, record["network_wait_duration_ns"])
        self.assertEqual(20, record["post_network_runnable_total_ns"])
        self.assertEqual(10, record["post_network_running_total_ns"])
        self.assertNotIn("99", output)


if __name__ == "__main__":
    unittest.main()
