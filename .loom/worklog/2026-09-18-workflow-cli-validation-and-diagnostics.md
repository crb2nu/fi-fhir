### 2026-09-18 - Workflow CLI validation and diagnostics

- Fixed issue #21: `workflow validate`, `workflow run`, and `workflow dry-run`
  now share configuration validation before event input is read. Keep the
  existing structural requirements, then use `workflow.ValidateWorkflow` for
  CEL compilation, transforms, and built-in action settings. Report severity,
  code, and path on stderr; warnings remain non-blocking.
- Added fixture-backed regressions for invalid CEL syntax, undeclared variables,
  action/transform errors, required routes/actions, valid conditions, and warning
  output across all three commands. The tests failed against the original CLI.
- Updated the CLI reference, workflow guide, DSL diagnostic example, and
  changelog. Corrected the DSL dry-run example to accept JSON events, and explain
  the limits of configuration checks and the separate published workflow compiler.
- Verification: repository pre-commit hook; targeted CLI/validator tests;
  `TestConfigValidation` end-to-end, including `invalid_cel`, without skips;
  pinned `make lint` (zero issues); documentation/worklog/job-inventory checks.
  The full `go test -race ./...` run passed every package except requestsecurity's
  unchanged 40 ms OIDC discovery timeout under compilation load; rerunning that
  package alone with `-race -count=1` passed.
- Pending merge coordination: !204 introduces an issue-21 skip that does not
  exist on this branch's `main` base. When reconciling that MR, remove its
  `invalid_cel` skip and the corresponding expected-skip/issue-id entries in
  `ci/test-e2e-legacy.yml`. This change already passes the original unskipped test.
