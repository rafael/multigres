#!/bin/sh
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

# Usage: run-go-tests.sh RESULT_PREFIX "PACKAGES..." [GO_TEST_FLAGS...]
# Run once, or set RETRY_FAILED_TESTS=true to resume a failed run after the
# Trunk quarantine check. Replaying saved events lets gotestsum select failed
# tests and retain every attempt in JUnit without running the full suite again.
set -eu

if [ "$#" -lt 2 ]; then
  echo "usage: $0 RESULT_PREFIX PACKAGES_STRING [GO_TEST_FLAGS...]" >&2
  exit 2
fi
prefix=$1
packages=$2
shift 2
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
mkdir -p "$(dirname -- "$prefix")"

if [ "${RETRY_FAILED_TESTS:-false}" = true ]; then
  for suffix in jsonl xml exitcode stderr; do
    if [ ! -f "$prefix.$suffix" ]; then
      echo "Cannot retry: missing $prefix.$suffix" >&2
      exit 1
    fi
  done
  initial_exit=$(cat "$prefix.exitcode")
  case "$initial_exit" in
    ''|*[!0-9]*)
      echo "Cannot retry: invalid initial exit code $initial_exit" >&2
      exit 1
      ;;
    0) exit 0 ;;
    1) ;;
    *)
      echo "Cannot retry: initial exit code was $initial_exit" >&2
      exit "$initial_exit"
      ;;
  esac

  # A failed command with no failed test events is an infrastructure/build
  # failure, not a retry candidate. Otherwise gotestsum's empty retry loop can
  # return success. gotestsum still parses and validates the full JSON below.
  if ! awk '
    /"Action"[[:space:]]*:[[:space:]]*"fail"/ &&
    /"Test"[[:space:]]*:[[:space:]]*"[^"]+"/ { found = 1 }
    END { exit !found }
  ' "$prefix.jsonl"; then
    echo "Cannot retry: no failed test events in $prefix.jsonl" >&2
    exit 1
  fi

  for suffix in jsonl xml exitcode stderr; do
    cp "$prefix.$suffix" "$prefix.initial.$suffix"
  done
  # Only a completed retry may publish its exit status. The action also checks
  # that these snapshots exist, in case its wrapper fails before reaching us.
  rm -f "$prefix.exitcode"
  # The replay source must differ from --jsonfile, which gotestsum truncates.
  result=0
  gotestsum --format=testname --rerun-fails=3 --rerun-fails-max-failures=10 \
    --jsonfile="$prefix.jsonl" --junitfile="$prefix.xml" --raw-command -- \
    "$script_dir/replay-go-tests.sh" "$prefix.initial.jsonl" \
    "$prefix.initial.stderr" "$initial_exit" "$#" "$@" \
    2>"$prefix.stderr" || result=$?
else
  # A failed command must never leave reports from an earlier invocation.
  for suffix in jsonl xml exitcode stderr; do
    rm -f "$prefix.$suffix" "$prefix.initial.$suffix"
  done
  result=0
  gotestsum --format=testname --packages="$packages" \
    --jsonfile="$prefix.jsonl" --junitfile="$prefix.xml" -- "$@" \
    2>"$prefix.stderr" || result=$?
fi

cat "$prefix.stderr" >&2
printf '%s\n' "$result" >"$prefix.exitcode"
exit "$result"
