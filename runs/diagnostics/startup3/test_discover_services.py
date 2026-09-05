import unittest

from discover_services import discover


def container(service, suffix):
    return {
        "Id": "id-" + suffix,
        "Config": {"Labels": {"com.kurtosistech.id": service}},
        "NetworkSettings": {"Networks": {"kurtosis": {"IPAddress": "10.0.0." + suffix}}},
    }


class DiscoverServicesTest(unittest.TestCase):
    def test_ignores_extra_services(self):
        result = discover([
            container("el-1", "1"), container("bn-1", "2"),
            container("snooper-engine-3", "3"), container("bn-3", "4"),
            container("vc-3", "5"),
        ])
        self.assertEqual({"bn-1", "bn-3", "vc-3"}, set(result))

    def test_rejects_duplicate_target(self):
        with self.assertRaisesRegex(ValueError, "duplicate service: bn-1"):
            discover([container("bn-1", "1"), container("bn-1", "2")])


if __name__ == "__main__":
    unittest.main()
