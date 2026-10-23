#!/usr/bin/env python3
# Copyright 2026 Supabase, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Exercise the real gotestsum v1.13.0 runner without Trunk or external services.

Run: python3 .github/scripts/test-run-go-tests.py
Requires go and gotestsum v1.13.0 on PATH. All fixtures/results live in a temp dir.
"""

# The filename matches the shell scripts, and test names describe each behavior.
# pylint: disable=invalid-name,missing-class-docstring,missing-function-docstring

import json
import os
import subprocess
import sys
import tempfile
import unittest
import xml.etree.ElementTree as ET
from datetime import datetime
from pathlib import Path

RUNNER = Path(__file__).with_name("run-go-tests.sh").resolve()
CHECKER = RUNNER.parent.parent / "actions" / "test-with-trunk" / "check-results.py"
GO_FIXTURE = r"""
package retryfixture

import (
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "testing"
)

func count(t *testing.T) int {
    t.Helper()
    name := strings.ReplaceAll(t.Name(), "/", "__")
    path := filepath.Join(os.Getenv("FIXTURE_STATE"), name)
    data, _ := os.ReadFile(path)
    n, _ := strconv.Atoi(string(data))
    n++
    if err := os.WriteFile(path, []byte(strconv.Itoa(n)), 0600); err != nil {
        t.Fatal(err)
    }
    return n
}

func TestPassing(t *testing.T) { count(t) }

func TestRecover(t *testing.T) {
    mode := os.Getenv("FIXTURE_MODE")
    if mode != "recover" && mode != "mixed" { t.Skip() }
    if count(t) == 1 { t.Fatal("first attempt fails") }
}

func TestPersistent(t *testing.T) {
    mode := os.Getenv("FIXTURE_MODE")
    if mode != "persistent" && mode != "mixed" { t.Skip() }
    count(t)
    t.Fatal("every attempt fails")
}

func TestPanic(t *testing.T) {
    if os.Getenv("FIXTURE_MODE") != "panic" { t.Skip() }
    count(t)
    panic("cannot safely retry an incomplete suite")
}

func TestNested(t *testing.T) {
    if os.Getenv("FIXTURE_MODE") != "nested" { t.Skip() }
    t.Run("fail.[x]", func(t *testing.T) {
        if count(t) == 1 { t.Fatal("first attempt fails") }
    })
    t.Run("passing", func(t *testing.T) { count(t) })
}
"""


class RunnerTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        version = subprocess.check_output(["gotestsum", "--version"], encoding="utf-8")
        if "v1.13.0" not in version:
            raise RuntimeError("These tests require gotestsum v1.13.0: " + version)
        cls.go_version = subprocess.check_output(
            ["go", "env", "GOVERSION"], encoding="utf-8"
        ).strip()
        cls.cache = tempfile.TemporaryDirectory(prefix="gotestsum-retry-cache-")
        cls.addClassCleanup(cls.cache.cleanup)

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="gotestsum-retry-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.state = self.root / "state"
        self.state.mkdir()
        self.prefix = self.root / "results with spaces" / "tests"
        (self.root / "go.mod").write_text(
            "module example.com/retryfixture\n\ngo 1.24\n", encoding="utf-8"
        )
        (self.root / "fixture_test.go").write_text(GO_FIXTURE, encoding="utf-8")
        self.env = dict(
            os.environ,
            FIXTURE_STATE=str(self.state),
            GOWORK="off",
            GOCACHE=self.cache.name,
            GOTOOLCHAIN=self.go_version,
        )
        self.env.pop("RETRY_FAILED_TESTS", None)

    def run_suite(self, mode="recover", retry=False, flags=(), cache=False):
        env = dict(self.env, FIXTURE_MODE=mode)
        if retry:
            env["RETRY_FAILED_TESTS"] = "true"
        go_flags = [] if cache else ["-count=1"]
        return subprocess.run(
            ["sh", str(RUNNER), str(self.prefix), "./...", *go_flags, *flags],
            cwd=self.root,
            env=env,
            encoding="utf-8",
            capture_output=True,
            timeout=90,
            check=False,
        )

    def artifact(self, suffix):
        return Path(f"{self.prefix}.{suffix}")

    def count(self, test):
        path = self.state / test.replace("/", "__")
        return int(path.read_text(encoding="utf-8")) if path.exists() else 0

    def assert_status(self, result, code):
        self.assertEqual(result.returncode, code, result.stdout + result.stderr)

    def assert_integrity(self, valid=True):
        result = subprocess.run(
            [sys.executable, str(CHECKER), str(self.prefix)],
            cwd=self.root,
            encoding="utf-8",
            capture_output=True,
            check=False,
        )
        self.assert_status(result, 0 if valid else 1)

    def assert_case_timestamps(self, test, events, cases):
        starts = [
            event["Time"]
            for event in events
            if event.get("Test") == test and event["Action"] == "run"
        ]
        timestamps = [case.get("timestamp") for case in cases]
        self.assertEqual(timestamps, starts)
        for previous, current in zip(timestamps, timestamps[1:]):
            self.assertLess(
                datetime.fromisoformat(previous.replace("Z", "+00:00")),
                datetime.fromisoformat(current.replace("Z", "+00:00")),
            )

    def test_recovered_failure_preserves_evidence_and_skips_passing_tests(self):
        self.assert_status(self.run_suite(), 1)
        self.assert_integrity()
        self.assertEqual(self.count("TestRecover"), 1)
        initial = self.artifact("jsonl").read_text(encoding="utf-8")
        self.assert_status(self.run_suite(retry=True), 0)
        self.assert_integrity()
        self.assertEqual(self.count("TestRecover"), 2)
        self.assertEqual(self.count("TestPassing"), 1)
        self.assertEqual(
            self.artifact("initial.jsonl").read_text(encoding="utf-8"), initial
        )
        self.assertEqual(
            self.artifact("initial.exitcode").read_text(encoding="utf-8"), "1\n"
        )
        self.assertEqual(self.artifact("exitcode").read_text(encoding="utf-8"), "0\n")
        events = [
            json.loads(line)
            for line in self.artifact("jsonl").read_text(encoding="utf-8").splitlines()
        ]
        outcomes = [
            e["Action"]
            for e in events
            if e.get("Test") == "TestRecover" and e["Action"] in ("fail", "pass")
        ]
        self.assertEqual(outcomes, ["fail", "pass"])
        cases = [
            c
            for c in ET.parse(self.artifact("trunk.xml")).iter("testcase")
            if c.get("name") == "TestRecover"
        ]
        self.assertEqual(len(cases), 2)
        self.assertIsNotNone(cases[0].find("failure"))
        self.assertIsNone(cases[1].find("failure"))
        self.assert_case_timestamps("TestRecover", events, cases)

    def test_mixed_failures_preserve_recovery_chronology(self):
        self.assert_status(self.run_suite(mode="mixed"), 1)
        self.assert_integrity()
        self.assert_status(self.run_suite(mode="mixed", retry=True), 1)
        self.assert_integrity()
        self.assertEqual(self.count("TestRecover"), 2)
        self.assertEqual(self.count("TestPersistent"), 4)
        self.assertEqual(self.count("TestPassing"), 1)
        events = [
            json.loads(line)
            for line in self.artifact("jsonl").read_text(encoding="utf-8").splitlines()
        ]
        report = ET.parse(self.artifact("trunk.xml"))
        for test, expected in (
            ("TestRecover", ["fail", "pass"]),
            ("TestPersistent", ["fail"] * 4),
        ):
            with self.subTest(test=test):
                outcomes = [
                    event["Action"]
                    for event in events
                    if event.get("Test") == test and event["Action"] in ("fail", "pass")
                ]
                self.assertEqual(outcomes, expected)
                cases = [
                    case for case in report.iter("testcase") if case.get("name") == test
                ]
                self.assertEqual(
                    [
                        "fail" if case.find("failure") is not None else "pass"
                        for case in cases
                    ],
                    expected,
                )
                self.assert_case_timestamps(test, events, cases)

    def test_persistent_failure_has_four_physical_attempts(self):
        self.assert_status(self.run_suite(mode="persistent"), 1)
        self.assert_integrity()
        self.assert_status(self.run_suite(mode="persistent", retry=True), 1)
        self.assert_integrity()
        self.assertEqual(self.count("TestPersistent"), 4)
        self.assertEqual(self.count("TestPassing"), 1)
        cases = [
            c
            for c in ET.parse(self.artifact("xml")).iter("testcase")
            if c.get("name") == "TestPersistent"
        ]
        self.assertEqual(len(cases), 4)
        self.assertTrue(all(c.find("failure") is not None for c in cases))

    def test_nested_retry_replaces_shard_filter_and_escapes_test_name(self):
        flags = ("-run", "^TestNested$", "-timeout=10s")
        self.assert_status(self.run_suite(mode="nested", flags=flags), 1)
        self.assert_integrity()
        self.assert_status(self.run_suite(mode="nested", retry=True, flags=flags), 0)
        self.assert_integrity()
        self.assertEqual(self.count("TestNested/fail.[x]"), 2)
        self.assertEqual(self.count("TestNested/passing"), 1)

    def test_more_than_ten_failed_tests_are_not_retried(self):
        with (self.root / "fixture_test.go").open("a", encoding="utf-8") as out:
            for i in range(11):
                out.write(
                    f'\nfunc TestFailure{i}(t *testing.T) {{ count(t); t.Fatal("fail") }}\n'
                )
        self.assert_status(self.run_suite(mode="many"), 1)
        self.assert_integrity()
        self.assert_status(self.run_suite(mode="many", retry=True), 3)
        self.assert_integrity(False)
        for i in range(11):
            self.assertEqual(self.count(f"TestFailure{i}"), 1)

    def test_compile_failure_is_not_retried_or_masked(self):
        (self.root / "broken.go").write_text(
            "package retryfixture\nvar bad = notDefined\n", encoding="utf-8"
        )
        self.assert_status(self.run_suite(), 1)
        self.assert_integrity(False)
        self.assert_status(self.run_suite(retry=True), 1)
        self.assert_integrity(False)
        self.assertEqual(self.count("TestPassing"), 0)

    def test_compile_failure_beside_failed_test_prevents_retry(self):
        broken = self.root / "broken"
        broken.mkdir()
        (broken / "broken.go").write_text(
            "package broken\nvar bad = notDefined\n", encoding="utf-8"
        )
        self.assert_status(self.run_suite(), 1)
        self.assert_integrity(False)
        self.assertEqual(self.count("TestRecover"), 1)
        self.assert_status(self.run_suite(retry=True), 3)
        self.assert_integrity(False)
        self.assertEqual(self.count("TestRecover"), 1)

    def test_saved_stderr_is_preserved_and_prevents_retry(self):
        self.assert_status(self.run_suite(), 1)
        self.artifact("stderr").write_text(
            "compiler error outside JSON output\n", encoding="utf-8"
        )
        self.assert_status(self.run_suite(retry=True), 3)
        self.assert_integrity(False)
        self.assertEqual(self.count("TestRecover"), 1)
        self.assertEqual(
            self.artifact("initial.stderr").read_text(encoding="utf-8"),
            "compiler error outside JSON output\n",
        )

    def test_panic_is_not_retried(self):
        self.assert_status(self.run_suite(mode="panic"), 1)
        self.assert_integrity(False)
        self.assert_status(self.run_suite(mode="panic", retry=True), 3)
        self.assert_integrity(False)
        self.assertEqual(self.count("TestPanic"), 1)

    def test_invalid_saved_results_remain_failed(self):
        self.assert_status(self.run_suite(), 1)
        original = self.artifact("jsonl").read_text(encoding="utf-8")
        cases = {
            "missing": None,
            "empty": "",
            "no-failed-tests": '{"Action":"fail","Package":"example.com/retryfixture"}\n',
            "malformed-with-failure": original + "not JSON\n",
        }
        for label, contents in cases.items():
            with self.subTest(label=label):
                if contents is None:
                    self.artifact("jsonl").unlink()
                else:
                    self.artifact("jsonl").write_text(contents, encoding="utf-8")
                self.assert_status(
                    self.run_suite(retry=True),
                    3 if label == "malformed-with-failure" else 1,
                )
                self.assertEqual(self.count("TestRecover"), 1)

    def test_successful_run_is_not_retried(self):
        self.assert_status(self.run_suite(mode="passing"), 0)
        self.assert_integrity()
        self.assert_status(self.run_suite(mode="passing", retry=True), 0)
        self.assert_integrity()
        self.assertEqual(self.count("TestPassing"), 1)

    def test_cached_success_can_be_uploaded(self):
        (self.root / "fixture_test.go").write_text(
            'package retryfixture\nimport "testing"\nfunc TestCached(t *testing.T) {}\n',
            encoding="utf-8",
        )
        self.assert_status(self.run_suite(mode="passing", cache=True), 0)
        self.assert_integrity()
        self.assert_status(self.run_suite(mode="passing", cache=True), 0)
        self.assert_integrity()
        events = [
            json.loads(line)
            for line in self.artifact("jsonl").read_text(encoding="utf-8").splitlines()
        ]
        self.assertTrue(any("(cached)" in event.get("Output", "") for event in events))
        starts = [
            event
            for event in events
            if event.get("Test") == "TestCached" and event["Action"] == "run"
        ]
        self.assertEqual(len(starts), 1)
        cases = list(ET.parse(self.artifact("trunk.xml")).iter("testcase"))
        self.assertEqual(len(cases), 1)
        if starts[0].get("Time"):
            self.assertEqual(cases[0].get("timestamp"), starts[0]["Time"])
        else:
            self.assertTrue(cases[0].get("timestamp"))

    def test_non_test_exit_code_is_preserved(self):
        self.assert_status(self.run_suite(), 1)
        self.artifact("exitcode").write_text("127\n", encoding="utf-8")
        self.assert_status(self.run_suite(retry=True), 127)
        self.assertEqual(self.count("TestRecover"), 1)


if __name__ == "__main__":
    unittest.main(verbosity=2)
