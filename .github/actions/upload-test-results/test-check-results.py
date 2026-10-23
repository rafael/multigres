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

"""Exercise the quarantine boundary with synthetic reports and real gotestsum."""

# pylint: disable=invalid-name,missing-function-docstring,missing-class-docstring

import json
import os
import runpy
import subprocess
import tempfile
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path

ACTION = Path(__file__).resolve().parent
RUNNER = ACTION.parents[1] / "scripts/run-go-tests.sh"
CHECK = runpy.run_path(str(ACTION / "check-results.py"))["check_results"]


def event(action, test="", package="example/pkg", **fields):
    return dict(Action=action, Package=package, Test=test, **fields)


def attempt(outcome="pass"):
    return [
        event("start"),
        event("run", "TestOne"),
        event(outcome, "TestOne"),
        event(outcome),
    ]


class QuarantineTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.prefix = self.directory / "results"

    def write_results(self, events, code="1"):
        Path(f"{self.prefix}.exitcode").write_text(code, encoding="utf-8")
        Path(f"{self.prefix}.jsonl").write_text(
            "".join(json.dumps(item) + "\n" for item in events), encoding="utf-8"
        )
        root = ET.Element("testsuites")
        for item in events:
            if item.get("Test") and item["Action"] == "fail":
                case = ET.SubElement(
                    root, "testcase", classname=item["Package"], name=item["Test"]
                )
                ET.SubElement(case, "failure")
        ET.ElementTree(root).write(f"{self.prefix}.xml")

    def test_completed_pass_fail_and_skip_are_valid(self):
        for status, code in (("pass", "0"), ("skip", "0"), ("fail", "1")):
            with self.subTest(status=status):
                self.write_results(attempt(status), code)
                CHECK(self.prefix, "failure" if code == "1" else "success")

    def test_parallel_packages_and_subtests(self):
        self.write_results(
            [
                event("start"),
                event("start", package="empty"),
                event("run", "TestParent"),
                event("run", "TestParent/sub"),
                event("pause", "TestParent/sub"),
                event("skip", package="empty"),
                event("cont", "TestParent/sub"),
                event("fail", "TestParent/sub"),
                event("fail", "TestParent"),
                event("fail"),
            ]
        )
        CHECK(self.prefix, "failure")

    def test_build_crash_and_incomplete_runs_cannot_be_quarantined(self):
        for events in (
            attempt("fail")[:-1],
            attempt("fail") + [event("start", package="broken")],
            attempt("fail") + [event("start"), event("fail")],
            attempt("fail") + [{"Action": "build-fail", "ImportPath": "broken"}],
            attempt("fail") + [event("fail", FailedBuild="broken")],
            attempt("fail")[:-1]
            + [event("output", Output="panic: TestMain crashed\n"), event("fail")],
        ):
            with self.subTest(events=events):
                self.write_results(events)
                with self.assertRaises(ValueError):
                    CHECK(self.prefix, "failure")

    def test_indented_logs_can_mention_panics(self):
        self.write_results(
            attempt()[:2]
            + [event("output", "TestOne", Output="    test.go:12: panic: expected\n")]
            + attempt()[2:],
            "0",
        )
        CHECK(self.prefix, "success")

    def test_missing_malformed_and_mismatched_reports_fail(self):
        for suffix, contents in (
            ("jsonl", None),
            ("jsonl", ""),
            ("jsonl", "invalid"),
            ("xml", None),
            ("xml", "invalid"),
            ("xml", '<testsuites errors="1"/>'),
            ("xml", "<testsuites/>"),
            ("exitcode", None),
            ("exitcode", "127"),
            ("exitcode", "0"),
        ):
            with self.subTest(suffix=suffix, contents=contents):
                self.write_results(attempt("fail"))
                path = Path(f"{self.prefix}.{suffix}")
                if contents is None:
                    path.unlink()
                else:
                    path.write_text(contents, encoding="utf-8")
                with self.assertRaises((OSError, ValueError, ET.ParseError)):
                    CHECK(self.prefix, "failure")

    def test_wrapper_failure_after_passing_tests_is_rejected(self):
        self.write_results(attempt(), "0")
        with self.assertRaises(ValueError):
            CHECK(self.prefix, "failure")

    def test_preparation_gates_quarantine_and_preserves_original_report(self):
        for status, code, allowed in (
            ("fail", "1", True),
            ("pass", "0", True),
            ("fail", "2", False),
        ):
            with self.subTest(status=status, code=code):
                self.write_results(attempt(status), code)
                original = Path(f"{self.prefix}.xml").read_bytes()
                result = subprocess.run(
                    ["bash", str(ACTION / "prepare-junit.sh")],
                    env=dict(
                        os.environ,
                        RESULTS_PREFIX=str(self.prefix),
                        ALLOW_QUARANTINE="true",
                        PREV_OUTCOME="success" if code == "0" else "failure",
                        GITHUB_OUTPUT=str(self.directory / "output"),
                    ),
                    capture_output=True,
                    text=True,
                    check=False,
                )
                self.assertEqual(result.returncode == 0, allowed, result.stderr)
                self.assertEqual(Path(f"{self.prefix}.xml").read_bytes(), original)

    def test_real_gotestsum_runs_a_recoverable_failure_only_once(self):
        (self.directory / "go.mod").write_text("module example/pkg\n\ngo 1.26\n")
        (self.directory / "example_test.go").write_text("""package example
import ("os"; "testing")
func TestFlake(t *testing.T) {
    if _, err := os.Stat("attempted"); err == nil { return }
    err := os.WriteFile("attempted", []byte("ran"), 0600)
    if err != nil { t.Fatal(err) }
    t.Fatal("first attempt fails")
}
""")
        for expected in (1, 0):
            result = subprocess.run(
                [str(RUNNER), str(self.prefix), "./...", "-count=1"],
                cwd=self.directory,
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
            events = [
                json.loads(line)
                for line in Path(f"{self.prefix}.jsonl")
                .read_text(encoding="utf-8")
                .splitlines()
            ]
            self.assertEqual(
                sum(item["Action"] == "run" for item in events),
                1,
                result.stdout + result.stderr,
            )
            try:
                CHECK(self.prefix, "failure" if expected else "success")
            except ValueError as error:
                self.fail(f"{error}\n{result.stdout}\n{result.stderr}")


if __name__ == "__main__":
    unittest.main(verbosity=2)
