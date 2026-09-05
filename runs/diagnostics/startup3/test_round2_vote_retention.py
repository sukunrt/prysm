import io
import importlib.util
import json
import subprocess
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path

MODULE = Path(__file__).with_name("round2_vote_retention.py")
SPEC = importlib.util.spec_from_file_location("round2_vote_retention", MODULE)
audit = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(audit)


def archive(path, validator_lines="", beacon_lines=""):
    with tarfile.open(path, "w:gz") as out:
        for name, text in (("validator.log", validator_lines), ("beacon.log", beacon_lines)):
            body = text.encode()
            info = tarfile.TarInfo(f"logs/{name}")
            info.size = len(body)
            out.addfile(info, io.BytesIO(body))


def activation(index):
    return f"2026-09-05 Validator activated index={index} status=ACTIVE validatorIndex={index}\n"


def vote(slot, outcome, index, root="0x01"):
    return (
        f"2026-09-05 Goldfish vote arrivedMs=1 blockRoot={root} decidedMs=2 "
        f"outcome={outcome} seats=1 validator={index} voteSlot={slot}\n"
    )


class VoteRetentionTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)

    def tearDown(self):
        self.tmp.cleanup()

    def test_activation_map_uses_records_not_node_arithmetic(self):
        archive(self.root / "round2-prysm-geth-61.tar.gz", activation(88800))
        archive(self.root / "round2-prysm-geth-62.tar.gz", activation(89396))
        self.assertEqual({88800: 61, 89396: 62}, audit.activation_owners([self.root]))
        self.assertNotEqual(61, 88800 // 596 + 1)

    def test_conflicting_activation_owner_fails(self):
        archive(self.root / "round2-prysm-geth-61.tar.gz", activation(88800))
        archive(self.root / "round2-prysm-geth-62.tar.gz", activation(88800))
        with self.assertRaisesRegex(ValueError, "activated on both node 61 and node 62"):
            audit.activation_owners([self.root])

    def test_slot_bounds_exclude_101(self):
        path = self.root / "round2-prysm-geth-400.tar.gz"
        archive(path, beacon_lines=vote(100, "accepted", 1) + vote(101, "accepted", 2))
        _, rows = audit.parse_archive(path, 0, 100, {})
        self.assertEqual([100], [row["slot"] for row in rows])

    def test_accepted_and_replayed_remain_distinct(self):
        path = self.root / "round2-prysm-geth-400.tar.gz"
        archive(path, beacon_lines=vote(51, "accepted", 1) + vote(51, "replayed", 2))
        _, rows = audit.parse_archive(path, 0, 100, {})
        self.assertEqual(["accepted", "replayed"], [row["outcome"] for row in rows])

    def test_cli_reports_derived_owner(self):
        activation_archive = self.root / "round2-prysm-geth-61.tar.gz"
        ledger_archive = self.root / "round2-prysm-geth-400.tar.gz"
        archive(activation_archive, validator_lines=activation(88800))
        archive(ledger_archive, beacon_lines=vote(51, "accepted", 88800))
        output = subprocess.check_output([
            sys.executable, str(Path(audit.__file__)), "--owner-archives-dir", str(self.root),
            "--min-slot", "0", "--max-slot", "100", str(ledger_archive),
        ], text=True)
        group = next(iter(json.loads(output)["nodes"]["400"]["slots"]["51"].values()))
        self.assertEqual({"61": 1}, group["owner_counts"])


if __name__ == "__main__":
    unittest.main()
