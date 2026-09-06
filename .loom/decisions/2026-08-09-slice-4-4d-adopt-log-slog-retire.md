### 2026-08-09: Slice 4.4d — adopt `log/slog`, retire the orphan logger, and redact before converting

Lane S5-C's gate decision, required before the lane writes a line of logging
code (`.loom/33-sprint5-execution-specs.md`, Lane S5-C, corrections 25-27). It
is forced by the lane's day-1 gate,
`TestStructuredLogging_ServeEmitsNoStructuredLogAndTheQueueDriverPrintsPayloads`,
which **passes on unmodified `main`**: a real `fi-fhir serve` doing real work
against PostgreSQL wrote ten output lines, none of which parses as JSON and none
of which carries `correlation_id`, `trace_id`, `span_id`, or `tenant_id`; and
the shipped `fi-fhir workflow run` surface with the `log` queue driver printed a
planted PHI sentinel verbatim on stdout.

- Decision:
  - **1. Option A — adopt `log/slog` at the `cmd/` seam, retire
    `internal/workflow/logging.go` and the dead `internal/ingest` package, and
    port the OTel correlation rather than re-deriving it.** The logger is
    constructed in `runServe` from the `pkg/config` settings that are already
    parsed and validated and read by nothing (`pkg/config/config.go:193-194`,
    `:612-613`, `:763-768`), and it reaches the components through the existing
    `Observe`/`SetObserver` callbacks adapted in
    `cmd/fi-fhir/serve_observability.go`. No logger is imported into
    `internal/integration/**`.
  - **2. Ordering rule, binding on this lane and on any future lane converting
    a print site: redaction precedes conversion.** A print statement that can
    carry a payload is redacted and given a test in its own commit *before* any
    mechanical `fmt.Printf` → structured conversion touches the same package.
    Converting first moves an ad-hoc stdout leak into a stream that log
    aggregators index and retain, which is strictly worse than the leak.
  - **3. Log fields are a bounded allowlist, enforced the way metric labels
    are.** `internal/observability/metrics.go:349-360` coerces an unknown
    outcome to `error` rather than emitting it; the logging equivalent is a
    permitted-key set, a unit test that fails on an unlisted key, and a sentinel
    assertion that a PHI value present in a durable payload appears in no
    captured line.

- Rationale:
  - "Add a structured logger" is the wrong verb. A complete `Logger` interface
    with a JSON handler (`internal/workflow/logging.go:17-32,113-286`,
    `outputJSON` at `:198-211`) that **already** correlates OTel identifiers off
    the context (`:169,:172`) exists today with zero non-test callers.
    `SetGlobalLogger`/`GetGlobalLogger` (`:301-317`) have zero callers at all,
    `globalLogger` is a `&NoOpLogger{}` nothing replaces (`:298`), and `Engine`
    defaults to `&NoOpLogger{}` (`engine.go:121,175`). Its only consumer is
    `internal/ingest/http.go:42,49`, and `internal/ingest` is itself dead:
    `git grep 'internal/ingest'` over `*.go` returns exactly one hit, its own
    subpackage import at `internal/ingest/temporal.go:10`. Shipping `slog`
    beside it creates a second abstraction and a second lie — the same
    producer-less, consumer-less shape the UX program already paid to retire.
  - It lives in `internal/workflow`, the legacy-engine package the durable path
    deliberately does not depend on for execution (the durable path consumes it
    only through `workflow.Planner`,
    `internal/integration/processor/workflow_plan.go:41-45`). Adopting it as the
    repo's logger inverts a boundary three slices have defended, and its
    bespoke `Field`/`F()` API is non-standard where `slog.Attr` is stdlib.
  - `slog` needs no dependency, and the OTel correlation it lacks is ~15 lines
    ported from `logging.go:160-180`.
  - The redaction ordering is not a preference. `LogQueuePublisher.Publish`
    (`internal/workflow/queue.go:320-323`) prints the entire serialized event —
    `"[Queue:%s] Topic: %s, Key: %s, Headers: %v, Value: %s\n"` — and `log` is
    the only registered queue driver (`:331-334`).
    `internal/workflow/actions.go:46-49` marshals the whole event at
    `level: "debug"`, and `engine.go:489` prints an unbounded `%v` on a DLQ
    error.

- Alternatives considered:
  - **Option B — adopt `internal/workflow/logging.go` as the repo's logger and
    wire it into `runServe`** (rejected: it drags the legacy-engine package into
    the `cmd/` seam and into anything that logs, inverting the boundary, for the
    sake of ~15 lines of correlation code that ports cleanly).
  - **Option C — ship `log/slog` and leave `workflow.Logger` in place**
    (rejected: two logger abstractions, one of them provably dead; this is the
    `diagnosticsStore` shape, which cost a dedicated retirement MR the last time
    it was allowed to persist).
  - **Convert the 82 `cmd/` print sites first and redact afterwards** (rejected
    by decision 2 above).

- Consequences:
  - `internal/workflow/logging.go` (359 lines), `logging_test.go`, and
    `internal/ingest/**` are deleted in this lane. `Engine`'s logger field and
    its `NoOpLogger` default go with them.
  - `pkg/config`'s `LogLevel`/`LogFormat` stop being parsed-and-ignored, which
    makes `observability.log_level` and `observability.log_format` load-bearing
    deployment configuration for the first time; the deployment artifacts that
    set them stop being aspirational.
  - The `log` queue driver keeps working and stops printing payloads. Anyone
    relying on it to dump event bodies to a terminal loses that, deliberately.
  - S5-D and S5-F emit structured logs from birth rather than retrofitting,
    which is why this lane merges second in the sprint.

- Corrections to `.loom/33-sprint5-execution-specs.md`, proved by the gate:
  - **Correction 27 overstates the queue leak's reachability.** The `log`
    driver's payload print is **not reachable from `fi-fhir serve`**. Every
    legacy-engine GraphQL entry point is gated behind `legacyUnsafeExecution`
    (`internal/api/graphql/resolvers/schema.resolvers.go:45,219,249,328,2215,
    2534,3441`), a field only settable through
    `enableLegacyUnsafeExecutionForTests`
    (`internal/api/graphql/resolvers/resolver.go:38-41,194-195`), which is nil
    outside that package's own test binary; and the durable path uses
    `workflow.Planner`, which plans and never executes actions. The reachable
    production surfaces are the CLI — `fi-fhir workflow run`
    (`cmd/fi-fhir/main.go:2043`), `workflow record` (`:2256`),
    `workflow simulate` (`:2512`) — and `fi-fhir subscription serve` with a
    workflow router (`:4198-4202`). The gate asserts this absence directly, so
    the correction is proved rather than argued. The redaction task is unchanged;
    its severity classification is.
  - **Correction 27's "default when no driver name is configured (`:313-314`)"
    is wrong.** Those lines default the publisher's *instance name*.
    `parseQueueConfig` (`queue.go:203-206`) requires an explicit `driver` key
    and errors without one. The substantive half — `log` is the only registered
    driver — holds.
  - **The kill-test `TestStructuredLogging_CorrelatedAndPHIFree` needs the same
    repair.** Its assertion 3 premise, "with the queue driver set to `log`"
    against a running serve, is unreachable; the PHI-free assertion over serve
    output stands on its own, and the queue-driver half belongs against the CLI
    surface, where this gate already puts it.

- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` Lane S5-C, corrections 23-34
  - [S2] `internal/workflow/logging.go:17-32,113-286,169,172,198-211,298,301-317`
  - [S3] `internal/workflow/queue.go:203-206,320-323,331-334`;
    `actions.go:46-49,53-57`; `engine.go:121,175,489`
  - [S4] `internal/ingest/http.go:42,49`; `internal/ingest/temporal.go:10`
  - [S5] `internal/api/graphql/resolvers/resolver.go:38-41,194-195`;
    `schema.resolvers.go:45,219,249,328,2215,2534,3441`
  - [S6] `internal/integration/processor/workflow_plan.go:41-45`
  - [S7] `pkg/config/config.go:188-190,193-194,420-423,609-613,759-768`
  - [S8] `internal/observability/metrics.go:349-360,362-366`
  - [S9] `cmd/fi-fhir/serve_observability.go:20,38,61,80,129,144,170`
  - [S10] `internal/observability/structured_logging_gate_integration_test.go`
    (this lane's day-1 gate; executed against PostgreSQL 16, PASS)
