### 2026-08-09 - Slice 4.4d Lane S5-C day-1 gate and adopt-or-retire decision

- What changed:
  - Added `internal/observability/structured_logging_gate_integration_test.go`,
    Lane S5-C's day-1 gate
    `TestStructuredLogging_ServeEmitsNoStructuredLogAndTheQueueDriverPrintsPayloads`,
    plus the `structured-logging` Makefile proof target and one `.PHONY` line.
  - Recorded the lane's gate decision in `.loom/40-decisions.md`: adopt
    `log/slog` at the `cmd/` seam (Option A), retire `internal/workflow/logging.go`
    and the dead `internal/ingest`, port the OTel correlation, and bind the lane
    to "redaction precedes conversion".
  - Test-only MR. No production code.

- Why:
  - The sprint scope reads "add `log/slog`, then add the OTel exporter". Both
    are already half-present in ways that make "add" the wrong verb, and the
    highest-risk item is neither: converting the queue driver's `fmt.Printf`
    without redacting first moves a stdout payload leak into an indexed,
    retained log stream. The gate forces the task order rather than leaving it
    to judgement.

- Evidence:
  - `make structured-logging` against PostgreSQL 16 on the remote Docker
    context: **PASS on unmodified `main`**, 12.3s, both subtests.
  - `serve_emits_no_structured_log`: a real `fi-fhir serve` accepted an
    authenticated HL7v2 submission (`202`, receipt + canonical event) and wrote
    10 output lines. Zero parse as JSON; zero carry `correlation_id`,
    `trace_id`, `span_id`, or `tenant_id`; zero contain `[Queue:`.
  - `log_queue_driver_prints_payloads`: `fi-fhir workflow run` with an action of
    `type: queue, driver: log` printed
    `[Queue:log] Topic: gate, Key: , Headers: map[], Value: {"data":{"family":"SENTINELFAMILY","mrn":"MRN4404DGATE"},...}`
    — the planted sentinel verbatim on stdout. Anti-vacuity guard asserts the
    driver actually published before the sentinel assertion runs.
  - Two spec corrections proved by execution and recorded in the decision entry:
    (1) the queue leak is **not** reachable from `fi-fhir serve` — every
    legacy-engine GraphQL entry point is gated behind `legacyUnsafeExecution`,
    settable only inside the resolvers package's own test binary, and the
    durable path uses `workflow.Planner`, which never executes actions; the
    reachable surfaces are the CLI and `subscription serve`. (2) `queue.go:313-314`
    defaults the publisher *instance name*, not the driver — `parseQueueConfig`
    requires an explicit `driver` key.

- Owned files (Lane S5-C, per `.loom/33` File-Ownership Map):
  - `cmd/fi-fhir/main.go` print/`Fprintf` sites only (the `runServe` component
    table is S5-D's)
  - `cmd/fi-fhir/serve_observability.go`
  - `internal/workflow/logging.go`, `queue.go:313-333`, `actions.go:38-57`,
    `internal/ingest/**`
  - `internal/api/graphql/operation_authorization.go` (compatibility-grant log
    line only; no role remapping)
  - `scripts/check-runtime-config.sh` (the tracing assertion; S5-B wires the CI
    job after this lane merges)
  - `docs/operations/PRODUCTION-HARDENING.md:583-598` (coordinate with S5-B —
    same file, different sections)
  - `internal/observability/structured_logging_gate_integration_test.go` (new)
  - `Makefile` (`structured-logging` target, one `.PHONY` line)
  - Not touched before S5-0a merges: `.gitlab-ci.yml`.

- What's next:
  - Commit 2 (security, standalone): redact `queue.go`'s payload print, stop
    `actions.go` marshalling whole events at debug level, bound `engine.go:489`'s
    `%v`. Its own test.
  - Then the `slog` logger at the `cmd/` seam, the seven observer adapters, the
    lineage-subset correlation fields, the bounded field allowlist, the
    compatibility-grant log line, and the orphan retirement.
  - Then the tracing-artifact closure (`check-runtime-config.sh` extended to the
    snake_case YAML form) and the OTLP exporter.

- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` Lane S5-C, corrections 23-34
  - [S2] `.loom/40-decisions.md` 2026-08-09 entry
  - [S3] `internal/workflow/queue.go:203-206,320-323,331-334`
  - [S4] `internal/api/graphql/resolvers/resolver.go:38-41,194-195`
  - [S5] `internal/integration/processor/workflow_plan.go:41-45`
