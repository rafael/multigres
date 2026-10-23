# Tests, quarantine, and retries

Short, race, and integration jobs use this action to make Trunk the source of
quarantine decisions while preserving gotestsum's existing retry policy.

| Initial result | Behavior |
| --- | --- |
| All tests pass | Upload and pass. |
| All failures are already quarantined | Skip retries, upload, and apply Trunk's decision. |
| Any failure is not quarantined | Retry failed tests up to three times, with the existing ten-failure limit. |
| All retried tests pass | Pass and upload both the original failures and the passing retries. |
| Failures persist | Upload all attempts and let Trunk apply quarantine. |
| Build error, crash, or incomplete results | Fail independently of quarantine. |

The quarantine probe uses the uploader's `dry-run` option. CLI version 0.16.1
still checks existing quarantine in this mode but does not upload the initial
attempt. The final upload contains every attempt once. A new pass-on-retry flake
therefore produces evidence for Trunk without making this run fail. Detection
and automatic quarantine are configured in the Trunk collection, not this action.

The probe returns a decision for the whole run. If a run mixes quarantined and
unknown failures, gotestsum retries all failed tests. Individual test filtering
would require an additional integration with Trunk's quarantine identities.

## Running and resuming tests

The supplied command must invoke `.github/scripts/run-go-tests.sh` with a result
prefix, a space-separated package list, and Go test flags. The action initially
sets `RETRY_FAILED_TESTS=false`; it sets it to `true` only when retries are needed.
The command must be safe to invoke again with that environment variable.

On retry, the runner replays saved JSON events into gotestsum's `--raw-command`
mode. This lets gotestsum select failed tests and retain all attempts without
running the initial suite a second time. The replay adapter preserves Go flags
and replaces the initial shard's `-run` filter with gotestsum's retry filter.

The integrity checks require complete results from each command. Fresh snapshots
and a completion status prevent a failed wrapper or container setup from reusing
old reports. They also produce `PREFIX.trunk.xml`, with each testcase's start
timestamp taken from the JSON events. gotestsum otherwise gives every attempt
the same suite timestamp, which prevents Trunk from recognizing a later recovery
when a different test remains failed. All attempts stay in the report; the raw
JUnit file remains unchanged, including when it belongs to the container user.

GitHub's test report is informational so it cannot override Trunk's quarantine
decision. Race results use a separate `race` variant.

Run the local checks with Go and gotestsum v1.13.0 on `PATH`:

```sh
python3 .github/actions/test-with-trunk/test-check-results.py
python3 .github/scripts/test-run-go-tests.py
```

Coverage keeps its separate whole-slice retries to preserve complete coverage
profiles. The former weekly flake aggregator and Slack report are removed.
