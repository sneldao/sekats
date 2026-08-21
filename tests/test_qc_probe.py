#!/usr/bin/env python3
"""Network-free checks for the any-agent probe parsers."""
from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path

SPEC = importlib.util.spec_from_file_location(
    "qc_server",
    Path(__file__).resolve().parents[1] / "tools" / "qc_server.py",
)
qc = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(qc)


class ParseAgentRefTests(unittest.TestCase):
    def test_raw_id(self):
        self.assertEqual(qc.parse_agent_ref("cmq6mrfk002yzzc1k0sonovtw"), "cmq6mrfk002yzzc1k0sonovtw")

    def test_profile_url(self):
        self.assertEqual(
            qc.parse_agent_ref("https://arena.dev.fun/agent/cmq6mrfk002yzzc1k0sonovtw"),
            "cmq6mrfk002yzzc1k0sonovtw",
        )

    def test_profile_url_with_season_query(self):
        self.assertEqual(
            qc.parse_agent_ref(
                "https://arena.dev.fun/agent/cmq0s3gu8a4uak9a5ra1qt4f5?arena=poker-tournament&season=14"
            ),
            "cmq0s3gu8a4uak9a5ra1qt4f5",
        )

    def test_rejects_foreign_host(self):
        with self.assertRaises(ValueError):
            qc.parse_agent_ref("https://example.com/agent/cmq6mrfk002yzzc1k0sonovtw")

    def test_rejects_short(self):
        with self.assertRaises(ValueError):
            qc.parse_agent_ref("abc")

    def test_rejects_handle(self):
        with self.assertRaises(ValueError):
            qc.parse_agent_ref("stack_cartographer")


class MatchScoreTests(unittest.TestCase):
    def test_exact_name(self):
        self.assertEqual(qc.match_score("marlow", "Marlow", "marlowe"), 0)

    def test_prefix_handle(self):
        self.assertEqual(qc.match_score("marlo", "Marlow", "marlowe"), 1)

    def test_too_short(self):
        self.assertIsNone(qc.match_score("a", "Marlow", "marlowe"))


class SplitAndModeTests(unittest.TestCase):
    def test_split_ids(self):
        self.assertEqual(qc.split_ids("a, b\nc"), ["a", "b", "c"])

    def test_classify_mode(self):
        self.assertEqual(qc.classify_mode("[Poker] Playground S15"), "playground")
        self.assertEqual(qc.classify_mode("[Poker] Tournament S14"), "tournament")
        self.assertEqual(qc.classify_mode("[Poker] Eval S1"), "eval")


if __name__ == "__main__":
    unittest.main()
