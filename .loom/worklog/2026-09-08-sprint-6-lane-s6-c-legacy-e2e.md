### 2026-09-08 - Sprint 6 Lane S6-C: legacy e2e tree repaired and made blocking

- What changed:
  - `test/e2e/e2e_test.go` and `test/e2e/integration_test.go` repaired against
    the CLI's actual contract; both now pass. New CI job `test:e2e-legacy`
    (`ci/test-e2e-legacy.yml`), blocking, runs the whole tree against a
    PostgreSQL container, an HTTP echo container and a `fi-fhir serve` it starts
    itself.
  - Retired `test/e2e/docker-compose.yaml` and the `e2e-up` / `e2e-down` /
    `test-e2e-full` / `test-golden` targets, plus the golden-file machinery
    (`GoldenDir`, `UPDATE_GOLDEN`, `compareOrUpdateGolden`, `WorkflowDir`).
  - `test/e2e/README.md` rewritten; the two stale `AGENTS.md` rows corrected.
  - Two engine defects filed: [#20] (fhir action) and [#21] (`workflow validate`).

- Why:
  - `.loom/34` correction 15: nine tests red, nothing runs them, so
    "docs/status match executable behavior" was false while `make test-e2e` was
    a target that failed and no job said so [S1].

- Evidence:
  - **Day-1 run, against the S5-B service set.** Fourteen tests/subtests failed,
    not nine. The nine correction 15 predicts —
    `TestWorkflowCELFilter` ×3, `TestWorkflowTransform`,
    `TestConfigValidation/invalid_cel`, `TestEndToEndPipeline`,
    `TestDatabaseAction`, `TestWebhookAction`, `TestWorkflowWithRetry` — all
    failed as recorded. Five more did too:
    `TestParseHL7v2ADT/ADT_A01`, `TestParseHL7v2ORU`, `TestParseCSV`,
    `TestWorkflowDryRun`, `TestWorkflowWithWebhook`.
  - **The tree had not moved; S5-B under-recorded.** `runParse` and
    `runWorkflowRun` are byte-identical between `2f8b3f609` (the commit S5-B
    executed at) and `a3335a71f`, so all five extra failures were red then too.
    Nothing in `test/e2e/` has changed since `42a1d8f68`.
  - **Kill-test on the lane's riskiest assumption — REFUTED.** "It is only
    template drift." Repaired all six `{{.Patient.Status}}`-style Go dot-paths
    to the snake_case JSON keys the engine binds (`5d07101c4`'s shape),
    changed nothing else, re-ran: **the failing set was identical, test for
    test.** Zero of fourteen failures were template drift. The drift was real
    and entirely masked — no test reached a rendered template, because four
    other contracts were wrong upstream of it:
    - `parse` emits the event, not a `{"events": [...]}` envelope (hl7v2 → one
      object, csv → an array). Three tests read a key that has never existed.
    - `workflow run` has no `--dry-run` flag; dry run is the `workflow dry-run`
      subcommand, and it prints route decisions, not rendered actions.
    - Event input must be a JSON array or NDJSON; every test embedded a
      pretty-printed multi-line object, so `parseEventInput` rejected it and no
      action ever ran.
    - Action config is flat scalars only (`Action.UnmarshalYAML`), so the
      `headers:`, `retry:`, `auth:` and `fields:` blocks the tests declared were
      dropped before any action saw them.
  - **The panic was a test defect, not an engine one.** `e2e_test.go:493` read
    `parseResult["events"].([]interface{})` unchecked against an envelope the
    CLI never emitted; the nil assertion panicked the test binary and aborted
    the rest of the run. Fixed, not skipped: `TestEndToEndPipeline` now feeds
    the parse output straight to `workflow run` and passes.
  - **The timing assertion got the `TestQuickBenchmark` treatment.**
    `TestWorkflowWithRetry` asserted elapsed wall clock, which the `k3s-ci`
    pool cannot make honest across a 5.3x CPU spread. It now counts attempts at
    an `httptest` destination that answers 503, and asserts exactly
    `retry_max + 1`. Its nested `retry:` block was dropped YAML anyway; the real
    keys are the flat `retry_max` / `retry_delay`.
  - **Two failures survived and are filed with reproductions, not hidden.**
    - [#20] `TestFHIRAction`. The `fhir` action's `patient_admit` transaction
      bundle gives its Patient entry no `fullUrl` and points the Encounter's
      `subject` at `Patient/<MRN>`, a server id that does not exist, so a stock
      `hapiproject/hapi` rejects the whole transaction with 400 HAPI-1094 and
      writes nothing. `resource: Patient` is also ignored — the action always
      emits both resources for an admit, which forces the bundle path.
      Reproduced end to end; the exact request body is on the issue.
    - [#21] `TestConfigValidation/invalid_cel`. `workflow validate` calls the
      shallow `Workflow.Validate()`; `workflow.Validator`, which compiles the
      CEL condition and emits `INVALID_CEL`, has no caller in `cmd/`.
    Both skip with the issue id in the message; both keep their repaired bodies,
    so deleting the `t.Skipf` is the whole of the verification when they land.
  - **Compose file retired (correction 15 confirmed).** It stood up PostgreSQL,
    HAPI FHIR, Kafka, Redis, Jaeger and an echo server and no application
    container, which is why `TestObservabilityEndpoints` skipped with the whole
    stack up; two of its six services had no test at all; and the runner has no
    Docker socket, so no job could have used it. `test/e2e/README.md` carries
    the three `docker run` lines that replace it.
  - **Golden machinery retired.** `test/e2e/golden/` and `test/e2e/workflows/`
    never existed, so `compareOrUpdateGolden` always took its "file does not
    exist" branch and logged — and it could not have worked if they had, since
    `parse` stamps a fresh UUID, `timestamp` and `received_at` into every event.
    A comparison that cannot fail is the shape `AGENTS.md` refuses.
  - **The job cannot go green vacuously.** An existence guard (14 top-level
    tests listed), an exact skip-set diff against the two parked tests, a check
    that both skip messages still name their issue, and PASS-count floors on
    both halves (13 tagged+integration, 9 under `-tags=e2e` alone).
    `set -o pipefail` before each `make ... | tee`, without which the pipeline's
    exit status would be `tee`'s and a red suite would report a green job.
  - Local: `make test-e2e` 9/9 pass; `make test-integration` 13 pass, 1 skip
    (`TestFHIRAction`) plus `TestConfigValidation/invalid_cel`;
    `gofmt -l`, `go vet -tags=e2e,integration`, `golangci-lint run` all clean.

- What's next:
  - [#20] and [#21] are the two open follow-ups. Neither is on the 1.0 critical
    path; both are one-file fixes in `internal/workflow` / `cmd/fi-fhir`.
  - `.go-changes` in `.gitlab-ci.yml` still does not include `test/**`, so this
    job declares its own change set. Folding `test/**/*` into the shared list
    belongs to whoever next owns that file.

- Sources:
  - [S1] `.loom/34-sprint6-execution-specs.md` — correction 15, Lane S6-C
  - [S2] `ci/s5b-chaos-dr.yml:85-230` — the S5-B execution record and the
    service-container pattern this job reuses
  - [S3] `5d07101c4` — the schema rewrite whose template shape was applied here
  - [S4] https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/20
  - [S5] https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/21
