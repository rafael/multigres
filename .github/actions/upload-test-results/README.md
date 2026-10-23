# Trunk quarantine

Short, race, and integration tests run once, then this action uploads their
results and applies Trunk's quarantine decision. An unquarantined failure fails
CI; a run whose failures are all quarantined passes. There are no automatic
test retries in these jobs. Coverage keeps its existing whole-slice retries.

Use `run-go-tests.sh` to write JSONL, JUnit, and a completion exit code. Pass its
`results-prefix`, the test step's outcome, and `allow-quarantine: "true"` to this
action. Before contacting Trunk, the action verifies that execution completed
without build errors or crashes and that the reports match the command result.
GitHub's test reporter remains informational so it cannot override quarantine.

Automatic quarantine must also be enabled in the Trunk collection. Newly
observed failures can block CI until Trunk identifies them as flaky and
quarantines them. Race results use a separate `race` variant so ordinary test
passes do not classify race failures as flaky.

Run the safeguards' tests with Go and gotestsum v1.13.0 on `PATH`:

```sh
python3 .github/actions/upload-test-results/test-check-results.py
```
