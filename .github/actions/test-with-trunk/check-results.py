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

"""Validate execution integrity and write timestamped JUnit for Trunk.

gotestsum groups failed, skipped, and passed attempts in its JUnit report. Per-test
timestamps from JSONL preserve retry chronology in the separate .trunk.xml file.
"""

# This CLI follows the surrounding shell scripts' hyphenated filenames.
# pylint: disable=invalid-name

import argparse
import json
import sys
import xml.etree.ElementTree as ET
from collections import defaultdict, deque
from pathlib import Path


def check_results(prefix, retry=False, outcome=None):
    """Validate results and preserve attempt timestamps without deciding flakiness."""
    if retry:
        for suffix in ("jsonl", "xml", "exitcode", "stderr"):
            if not Path(f"{prefix}.initial.{suffix}").is_file():
                raise ValueError(f"retry did not save the initial {suffix} report")
    exitcode = Path(f"{prefix}.exitcode").read_text(encoding="utf-8").strip()
    if exitcode not in ("0", "1"):
        raise ValueError("test runner did not finish with an ordinary test exit code")
    if outcome is not None and (exitcode == "0") != (outcome == "success"):
        raise ValueError("test runner exit code does not match the command outcome")

    root = ET.parse(f"{prefix}.xml").getroot()
    if root.tag not in ("testsuite", "testsuites"):
        raise ValueError("unexpected JUnit root element")
    for element in root.iter():
        if element.tag == "error" or int(element.get("errors", "0")) != 0:
            raise ValueError("JUnit contains execution errors")

    active = {}
    attempts = defaultdict(deque)
    completed = 0
    terminal = {"pass", "fail", "skip", "bench"}
    actions = terminal | {"start", "run", "pause", "cont", "output", "attr"}
    with Path(f"{prefix}.jsonl").open(encoding="utf-8") as stream:
        for number, line in enumerate(stream, 1):
            if not line.strip():
                continue
            event = json.loads(line)
            if not isinstance(event, dict):
                raise ValueError(f"JSONL line {number} is not an event")
            action = event.get("Action")
            if action == "build-fail" or event.get("FailedBuild"):
                raise ValueError(f"build failure at JSONL line {number}")
            if action == "build-output":
                continue
            package = event.get("Package")
            test = event.get("Test", "")
            if (
                not isinstance(action, str)
                or action not in actions
                or not isinstance(package, str)
                or not package
                or not isinstance(test, str)
            ):
                raise ValueError(f"invalid test event at JSONL line {number}")
            output = event.get("Output", "")
            if not isinstance(output, str):
                raise ValueError(f"invalid output at JSONL line {number}")
            # A TestMain panic can follow completed tests, so completeness alone
            # does not detect it. Normal t.Log output is indented by go test.
            if any(
                part.startswith(("panic:", "fatal error:"))
                for part in output.splitlines()
            ):
                raise ValueError(f"test process panicked at JSONL line {number}")
            if action == "start":
                if test or package in active:
                    raise ValueError(f"unfinished invocation of {package}")
                active[package] = {"running": {}, "failed": False}
                continue
            if package not in active:
                raise ValueError(f"event outside a package invocation: {package}")
            state = active[package]
            if test:
                if action == "run":
                    if test in state["running"]:
                        raise ValueError(f"test started twice: {package}/{test}")
                    state["running"][test] = event.get("Time")
                elif action in terminal:
                    if test not in state["running"]:
                        raise ValueError(
                            f"test finished without starting: {package}/{test}"
                        )
                    attempts[package, test, action].append(state["running"].pop(test))
                    state["failed"] |= action == "fail"
            elif action in terminal:
                if state["running"]:
                    raise ValueError(f"unfinished tests in {package}")
                if action == "fail" and not state["failed"]:
                    raise ValueError(
                        f"package failed without a named test failure: {package}"
                    )
                del active[package]
                completed += 1
    if active or not completed:
        raise ValueError("missing or incomplete package results")

    for case in root.iter("testcase"):
        status = "pass"
        if case.find("failure") is not None:
            status = "fail"
        elif case.find("skipped") is not None:
            status = "skip"
        key = (case.get("classname"), case.get("name"), status)
        if not attempts[key]:
            raise ValueError(f"JUnit testcase has no matching JSONL attempt: {key}")
        timestamp = attempts[key].popleft()
        if not isinstance(timestamp, str) or not timestamp:
            raise ValueError(f"JSONL attempt has no run timestamp: {key}")
        case.set("timestamp", timestamp)
    # Keep the original report untouched: container-generated files may belong
    # to the postgres user, and Codecov still consumes the original JUnit.
    ET.ElementTree(root).write(
        f"{prefix}.trunk.xml", encoding="utf-8", xml_declaration=True
    )


def main():
    """Validate the supplied reports and report failures to GitHub Actions."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("results_prefix")
    parser.add_argument(
        "--retry", action="store_true", help="require fresh retry snapshots"
    )
    parser.add_argument(
        "--outcome",
        choices=("success", "failure"),
        help="compare the command outcome with the recorded runner exit code",
    )
    args = parser.parse_args()
    try:
        check_results(args.results_prefix, retry=args.retry, outcome=args.outcome)
    except (OSError, ValueError, ET.ParseError) as error:
        print(
            f"::error::Test execution integrity check failed: {error}", file=sys.stderr
        )
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
