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

"""Run with python3 .github/actions/test-with-trunk/test-check-results.py."""

# The CLI filename matches its subject; descriptive test names explain each case.
# pylint: disable=invalid-name,missing-function-docstring,missing-class-docstring

import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path

SPEC = importlib.util.spec_from_file_location(
    "check_results", Path(__file__).with_name("check-results.py")
)
if SPEC is None or SPEC.loader is None:
    raise ImportError("Cannot load check-results.py")
CHECKER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECKER)


def event(action, test="", package="example/pkg", **fields):
    return dict(Action=action, Package=package, Test=test, **fields)


def attempt(outcome="pass", package="example/pkg"):
    return [
        event("start", package=package),
        event("run", "TestOne", package),
        event(outcome, "TestOne", package),
        event(outcome, package=package),
    ]


class IntegrityTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.prefix = Path(self.temp.name) / "results"

    def write_results(self, events, code="1", xml='<testsuites errors="0"/>'):
        Path(f"{self.prefix}.exitcode").write_text(code, encoding="utf-8")
        Path(f"{self.prefix}.xml").write_text(xml, encoding="utf-8")
        Path(f"{self.prefix}.jsonl").write_text(
            "".join(json.dumps(item) + "\n" for item in events), encoding="utf-8"
        )
        Path(f"{self.prefix}.stderr").write_text("", encoding="utf-8")

    def save_initial(self):
        for suffix in ("jsonl", "xml", "exitcode", "stderr"):
            Path(f"{self.prefix}.initial.{suffix}").write_bytes(
                Path(f"{self.prefix}.{suffix}").read_bytes()
            )

    def test_junit_grouping_preserves_attempt_chronology_and_is_idempotent(self):
        times = [f"2026-10-23T12:00:0{i}.123456789Z" for i in range(3)]
        events = []
        for status, timestamp in zip(("fail", "pass", "fail"), times):
            run = attempt(status)
            run[1]["Time"] = timestamp
            events.extend(run)
        xml = """<testsuites><testsuite timestamp="2026-10-23T12:00:00Z">
          <testcase classname="example/pkg" name="TestOne"><failure/></testcase>
          <testcase classname="example/pkg" name="TestOne"><failure/></testcase>
          <testcase classname="example/pkg" name="TestOne"/>
        </testsuite></testsuites>"""
        self.write_results(events, xml=xml)
        CHECKER.check_results(self.prefix)
        normalized = Path(f"{self.prefix}.trunk.xml")
        self.assertEqual(
            [case.get("timestamp") for case in ET.parse(normalized).iter("testcase")],
            [times[0], times[2], times[1]],
        )
        self.assertEqual(Path(f"{self.prefix}.xml").read_text(encoding="utf-8"), xml)
        first_result = normalized.read_bytes()
        CHECKER.check_results(self.prefix)
        self.assertEqual(normalized.read_bytes(), first_result)

    def test_subtest_and_skipped_timestamps_are_matched_by_name_and_outcome(self):
        names = ("TestParent", "TestParent/failed", "TestParent/skipped")
        times = [f"2026-10-23T12:00:0{i}Z" for i in range(3)]
        events = [event("start")]
        events.extend(event("run", name, Time=time) for name, time in zip(names, times))
        events.extend(
            [
                event("fail", names[1]),
                event("skip", names[2]),
                event("fail", names[0]),
                event("fail"),
            ]
        )
        xml = """<testsuites><testsuite>
          <testcase classname="example/pkg" name="TestParent/skipped"><skipped/></testcase>
          <testcase classname="example/pkg" name="TestParent"><failure/></testcase>
          <testcase classname="example/pkg" name="TestParent/failed"><failure/></testcase>
        </testsuite></testsuites>"""
        self.write_results(events, xml=xml)
        CHECKER.check_results(self.prefix)
        self.assertEqual(
            [
                case.get("timestamp")
                for case in ET.parse(f"{self.prefix}.trunk.xml").iter("testcase")
            ],
            [times[2], times[0], times[1]],
        )

    def test_unmatched_junit_cases_are_rejected_without_writing_normalized_report(self):
        for case in (
            '<testcase classname="wrong/pkg" name="TestOne"><failure/></testcase>',
            '<testcase classname="example/pkg" name="WrongTest"><failure/></testcase>',
            '<testcase classname="example/pkg" name="TestOne"/>',
        ):
            with self.subTest(case=case):
                self.write_results(
                    attempt("fail"), xml=f"<testsuite>{case}</testsuite>"
                )
                with self.assertRaisesRegex(ValueError, "no matching JSONL attempt"):
                    CHECKER.check_results(self.prefix)
                self.assertFalse(Path(f"{self.prefix}.trunk.xml").exists())

    def test_junit_case_requires_a_timestamp_on_its_run_event(self):
        xml = """<testsuite>
          <testcase classname="example/pkg" name="TestOne"><failure/></testcase>
        </testsuite>"""
        self.write_results(attempt("fail"), xml=xml)
        with self.assertRaisesRegex(ValueError, "no run timestamp"):
            CHECKER.check_results(self.prefix)
        self.assertFalse(Path(f"{self.prefix}.trunk.xml").exists())

    def test_retry_wrapper_failure_cannot_reuse_initial_reports(self):
        self.write_results(attempt("fail"))
        with self.assertRaisesRegex(ValueError, "retry did not save"):
            CHECKER.check_results(self.prefix, retry=True, outcome="failure")
        self.save_initial()
        for suffix in ("jsonl", "xml", "exitcode", "stderr"):
            with self.subTest(missing=suffix):
                snapshot = Path(f"{self.prefix}.initial.{suffix}")
                snapshot.unlink()
                with self.assertRaisesRegex(ValueError, "retry did not save"):
                    CHECKER.check_results(self.prefix, retry=True, outcome="failure")
                self.save_initial()

    def test_retry_requires_a_new_completion_status(self):
        self.write_results(attempt("fail"))
        self.save_initial()
        Path(f"{self.prefix}.exitcode").unlink()
        with self.assertRaises(OSError):
            CHECKER.check_results(self.prefix, retry=True, outcome="failure")

    def test_wrapper_failure_after_successful_runner_is_rejected(self):
        self.write_results(attempt(), "0")
        with self.assertRaisesRegex(ValueError, "does not match"):
            CHECKER.check_results(self.prefix, outcome="failure")
        self.write_results(attempt("fail"), "1")
        with self.assertRaisesRegex(ValueError, "does not match"):
            CHECKER.check_results(self.prefix, outcome="success")

    def test_cli_accepts_completed_retry_outcomes(self):
        for code, outcome in (("0", "success"), ("1", "failure")):
            with self.subTest(outcome=outcome):
                self.write_results(attempt("fail"))
                self.save_initial()
                self.write_results(attempt("fail") + attempt(), code)
                result = subprocess.run(
                    [
                        sys.executable,
                        SPEC.origin,
                        str(self.prefix),
                        "--retry",
                        "--outcome",
                        outcome,
                    ],
                    capture_output=True,
                    text=True,
                    check=False,
                )
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_complete_failures_and_recovered_retries_are_valid(self):
        for events, code in [
            (attempt(), "0"),
            (attempt("fail"), "1"),
            (attempt("fail") + attempt(), "0"),
            (attempt("fail") * 4, "1"),
        ]:
            with self.subTest(events=events, code=code):
                self.write_results(events, code)
                CHECKER.check_results(self.prefix)

    def test_parallel_packages_and_subtests(self):
        self.write_results(
            [
                event("start"),
                event("start", package="other"),
                event("run", "TestOne"),
                event("run", "TestOne/sub"),
                event("pause", "TestOne/sub"),
                event("skip", package="other"),
                event("cont", "TestOne/sub"),
                event("fail", "TestOne/sub"),
                event("fail", "TestOne"),
                event("fail"),
            ]
        )
        CHECKER.check_results(self.prefix)

    def test_all_skipped_and_no_test_packages_are_valid(self):
        self.write_results(attempt("skip") + [event("start"), event("skip")], "0")
        CHECKER.check_results(self.prefix)

    def test_build_failure_beside_completed_test_failure(self):
        for failure in [
            dict(Action="build-fail", ImportPath="broken"),
            event("fail", package="broken", FailedBuild="broken"),
            event("fail", package="broken"),
        ]:
            with self.subTest(failure=failure):
                self.write_results(
                    attempt("fail") + [event("start", package="broken"), failure]
                )
                with self.assertRaises(ValueError):
                    CHECKER.check_results(self.prefix)

    def test_package_failure_is_scoped_to_current_invocation(self):
        self.write_results(attempt("fail") + [event("start"), event("fail")])
        with self.assertRaises(ValueError):
            CHECKER.check_results(self.prefix)

    def test_incomplete_package_and_test_runs(self):
        for events in [
            attempt()[:-1],
            attempt()[:2],
            [event("start"), event("run", "TestOne"), event("fail")],
            [event("start"), event("start")],
            attempt()[1:],
            [event("start"), event("pass", "TestOne"), event("pass")],
        ]:
            with self.subTest(events=events):
                self.write_results(events)
                with self.assertRaises(ValueError):
                    CHECKER.check_results(self.prefix)

    def test_panic_after_completed_failure_is_not_quarantinable(self):
        self.write_results(
            attempt("fail")[:-1]
            + [event("output", Output="panic: TestMain crashed\n"), event("fail")]
        )
        with self.assertRaises(ValueError):
            CHECKER.check_results(self.prefix)

    def test_indented_test_logs_can_mention_panics(self):
        self.write_results(
            attempt()[:2]
            + [
                event("output", "TestOne", Output="    test.go:12: panic: expected\n"),
                *attempt()[2:],
            ],
            "0",
        )
        CHECKER.check_results(self.prefix)

    def test_missing_empty_and_malformed_reports(self):
        for suffix in ("exitcode", "jsonl", "xml"):
            for contents in (None, "", "invalid"):
                with self.subTest(suffix=suffix, contents=contents):
                    self.write_results(attempt())
                    path = Path(f"{self.prefix}.{suffix}")
                    if contents is None:
                        path.unlink()
                    else:
                        path.write_text(contents, encoding="utf-8")
                    with self.assertRaises((OSError, ValueError, ET.ParseError)):
                        CHECKER.check_results(self.prefix)

    def test_non_test_exit_codes_and_junit_errors(self):
        for code in ("3", "127", "-1"):
            with self.subTest(code=code):
                self.write_results(attempt("fail"), code)
                with self.assertRaises(ValueError):
                    CHECKER.check_results(self.prefix)
        for xml in (
            '<testsuites errors="1"/>',
            "<testsuites><error/></testsuites>",
            '<testsuite errors="invalid"/>',
            "<unrecognized/>",
        ):
            with self.subTest(xml=xml):
                self.write_results(attempt("fail"), xml=xml)
                with self.assertRaises(ValueError):
                    CHECKER.check_results(self.prefix)


if __name__ == "__main__":
    unittest.main(verbosity=2)
