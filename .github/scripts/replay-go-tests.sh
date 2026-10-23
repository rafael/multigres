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

# Internal raw-command adapter for run-go-tests.sh. gotestsum appends exactly
# two arguments on retries: -test.run=<escaped test name> and the Go package.
set -eu

json_file=$1
stderr_file=$2
initial_exit=$3
flag_count=$4
shift 4

if [ "$#" -eq "$flag_count" ]; then
  cat "$json_file"
  # Preserve build errors and other stderr so gotestsum can abort retries.
  cat "$stderr_file" >&2
  exit "$initial_exit"
fi
if [ "$#" -ne "$((flag_count + 2))" ]; then
  echo "Unexpected arguments from gotestsum retry" >&2
  exit 2
fi

# Rotate the fixed flags behind the two retry arguments. Drop the original
# shard's -run filter, as gotestsum does for its normal (non-raw) retry command.
while [ "$flag_count" -gt 0 ]; do
  flag=$1
  shift
  flag_count=$((flag_count - 1))
  case "$flag" in
    -run|--run|-test.run|--test.run)
      if [ "$flag_count" -eq 0 ]; then
        echo "Missing value for $flag" >&2
        exit 2
      fi
      shift
      flag_count=$((flag_count - 1))
      ;;
    -run=*|--run=*|-test.run=*|--test.run=*) ;;
    -args|--args)
      echo "run-go-tests.sh requires Go test flags without -args" >&2
      exit 2
      ;;
    *) set -- "$@" "$flag" ;;
  esac
done
run_filter=$1
package=$2
shift 2
exec go test -json "$@" "$run_filter" "$package"
