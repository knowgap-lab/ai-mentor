"""Unit tests for D326 load pilot measure payload (no live 20-minute run)."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import release1_load_pilot as pilot


def fake_buckets() -> dict[str, dict[str, list]]:
    lat = {
        "read": [40.0, 50.0, 60.0, 70.0, 80.0] * 25,
        "answer": [100.0, 120.0, 140.0, 160.0, 180.0] * 22,
        "submit": [90.0, 110.0, 130.0, 150.0, 170.0] * 22,
        "tutor_review": [30.0, 35.0, 40.0, 45.0, 50.0] * 22,
    }
    return {
        name: {
            "latencies": values,
            "statuses": [200] * len(values),
        }
        for name, values in lat.items()
    }


PROFILE = {
    "students": 20,
    "tutors": 2,
    "profile": {"vcpu": 4, "ram_gib": 8, "observability": True},
    "warmup_seconds": 300,
    "measure_seconds": 1200,
    "mix": {"read": 0.6, "answer_or_submit": 0.3, "tutor": 0.1},
    "think_seconds": {"min": 3, "max": 10},
    "limits_ms": {
        "read": 500,
        "answer": 1000,
        "submit": 1000,
        "tutor_review": 500,
    },
    "seed": "release1-pilot-d326",
}


class BuildMeasurePayloadTest(unittest.TestCase):
    def test_emit_fields_for_check_load(self) -> None:
        buckets = fake_buckets()
        payload = pilot.build_measure_payload(
            PROFILE,
            buckets,
            started=pilot.time.monotonic(),
            unexpected_5xx=0,
            origin="https://example.test",
            students_started=20,
            tutors_started=2,
        )

        self.assertFalse(payload["attestation_pass"])
        self.assertEqual(payload["students"], 20)
        self.assertEqual(payload["tutors"], 2)
        self.assertEqual(payload["warmup_seconds"], 300)
        self.assertEqual(payload["measure_seconds"], 1200)
        self.assertEqual(payload["profile"]["vcpu"], 4)
        self.assertEqual(payload["mix"]["read"], 0.6)
        self.assertEqual(payload["seed"], "release1-pilot-d326")
        self.assertIn("browser_repeats not executed", payload["attestation_gaps"])

        for name in ("read", "answer", "submit", "tutor_review"):
            action = payload["actions"][name]
            for field in (
                "samples",
                "successes",
                "p95_ms",
                "p50_ms",
                "max_ms",
                "attempts",
                "errors",
                "throughput",
                "active_users",
                "queue_ms",
            ):
                self.assertIn(field, action, msg=f"{name}.{field}")
            self.assertGreaterEqual(action["samples"], 100)
            self.assertEqual(action["errors"], 0)

    def test_omit_empty_action_and_gap(self) -> None:
        buckets = fake_buckets()
        buckets["submit"]["latencies"] = []
        buckets["submit"]["statuses"] = []
        payload = pilot.build_measure_payload(
            PROFILE,
            buckets,
            started=pilot.time.monotonic(),
            unexpected_5xx=0,
            origin="https://example.test",
            students_started=20,
            tutors_started=2,
        )
        self.assertNotIn("submit", payload["actions"])
        self.assertIn("actions.submit not performed", payload["attestation_gaps"])


if __name__ == "__main__":
    unittest.main()