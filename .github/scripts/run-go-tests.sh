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
# Run once and record completion so quarantine cannot hide a runner failure.
set -eu

prefix=$1
packages=$2
shift 2
mkdir -p "$(dirname -- "$prefix")"
rm -f "$prefix.jsonl" "$prefix.xml" "$prefix.exitcode"

result=0
gotestsum --format=testname --packages="$packages" \
  --jsonfile="$prefix.jsonl" --junitfile="$prefix.xml" -- "$@" || result=$?
printf '%s\n' "$result" >"$prefix.exitcode"
exit "$result"
