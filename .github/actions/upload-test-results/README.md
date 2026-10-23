# Trunk quarantine

Short, race, and integration tests run once, then this action uploads their
results and applies Trunk's quarantine decision. An unquarantined failure fails
CI; a run whose failures are all quarantined passes. There are no automatic
test retries in these jobs. Coverage keeps its existing whole-slice retries.

Run gotestsum with `--junitfile`, then pass that file as `junit-paths`, the test
step's outcome as `previous-step-outcome`, and `allow-quarantine: "true"` to
this action. GitHub's test reporter remains informational so it cannot override
quarantine. Execution errors use gotestsum and Trunk's standard handling.

Automatic quarantine must also be enabled in the Trunk collection. Newly
observed failures can block CI until Trunk identifies them as flaky and
quarantines them. Race results use a separate `race` variant so ordinary test
passes do not classify race failures as flaky.
