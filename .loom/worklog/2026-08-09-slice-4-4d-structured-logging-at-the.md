### 2026-08-09 - Slice 4.4d structured logging at the cmd seam

- What changed:
  - **Redaction first, in its own commit.** `LogQueuePublisher.Publish` records
    sizes instead of the extracted message key, the header values, and the whole
    serialized event; `logAction` at `level: "debug"` records the event's shape
    instead of marshalling it; `engine.go`'s DLQ warning is bounded at 256 bytes
    on a rune boundary.
  - **`log/slog` at the `cmd/` seam.** `internal/observability/logging.go`: JSON
    handler, level and format from the `pkg/config` settings that were parsed,
    validated, and read by nothing; a closed `LogField` allowlist enforced by a
    handler that drops unlisted attributes (including ones bound through `With`,
    including whole groups) and reports `dropped_fields`; OTel trace/span
    correlation ported from the orphan.
  - 53 print sites in `runServe` converted, plus the three `log.Printf` in the
    GraphQL server's `Start`. Four survive on purpose and say why.
  - All seven observer adapters in `serve_observability.go` close over the
    logger. The three that already printed were the model.
  - **The compatibility-grant line** in `operation_authorization.go`: one
    structured line per admission that used `graphql:operator`, naming the field
    and the principal. No token, no role list. Log line only — no role remapping.
  - **The orphan retired**: `internal/workflow/logging.go` (359 lines),
    `logging_test.go`, `internal/ingest/**` (8 files), `Engine.SetLogger`/
    `GetLogger`, and `countMatchedRoutes`, whose only caller was a discard write.
  - **Tracing artifacts made honest** (not true — the exporter is the next
    slice): `deploy/kubernetes/base/configmap.yaml`, `README.md:67`,
    `NOTES.txt`, `PRODUCTION-HARDENING.md`'s values snippet, plus a new
    `check-runtime-config.sh` assertion for the snake_case YAML form.
  - **`PRODUCTION-HARDENING.md`'s compliance log schema** replaced with the real
    shape and an honest statement of what does and does not correlate.
  - **The day-1 gate inverted** into `TestStructuredLogging_CorrelatedAndPHIFree`
    with two negative controls.

- Why:
  - The lane's premise was that 4.4d is two build items. Both were already
    half-present in ways that made "add" the wrong verb, and the riskiest item
    was neither: converting the queue driver's `fmt.Printf` before redacting it
    would have moved an ad-hoc stdout leak into a stream aggregators index and
    retain. The decision recorded that ordering as a rule; this work follows it.

- Evidence:
  - `make structured-logging` PASS (PostgreSQL 16, remote Docker context): ten
    real serve lines, every one JSON, tenant-stamped, sentinel-free, allowlisted,
    with no `dropped_fields` and no `[Queue:` line.
  - `make structured-logging-negative-control` PASS — i.e. the kill-test fails
    with the payload print restored. The first version of this control was inert
    and reported so: the build tag reached the test binary but not the `fi-fhir`
    the test shells out to. The tag now propagates to that build.
  - `make observability-replicas` PASS; `make transport-gate` and
    `make transport-gate-negative-control` PASS; `make phi-audit` PASS on a fresh
    database (it fails on a reused one because its own negative control asserts a
    pre-migration schema — environment, not this change);
    `make check-runtime-config` 22 passed / 0 failures, and 21/1 with the
    configmap line reinstated.
  - `go test ./...` clean, `golangci-lint run ./...` 0 issues,
    `git grep 'log/slog\|internal/observability' internal/integration/` empty.

- Corrections to `.loom/33-sprint5-execution-specs.md` (all proved by execution,
  recorded in `.loom/40-decisions.md` and in the kill-test's header):
  1. Correction 27 overstates the queue leak's reachability — it is not
     reachable from `fi-fhir serve`; the surfaces are the CLI and
     `subscription serve`.
  2. `queue.go:313-314` defaults the publisher instance name, not the driver.
  3. **Task 4 cannot be delivered at the Observe seam.** The four callbacks take
     `(Result, error)` and no context, and no `Result` type carries a lineage
     identifier. Every line carries `tenant_id`; `correlation_id` would require
     widening those structs, which is an `internal/integration/**` edit other
     lanes own. Documented at the top of `serve_observability.go` and in
     `PRODUCTION-HARDENING.md`.

- Owned files touched (Lane S5-C, per `.loom/33` File-Ownership Map):
  - `cmd/fi-fhir/main.go` (print statements only — not the `runServe` component
    table, which is S5-D's), `cmd/fi-fhir/serve_observability.go`,
    `cmd/fi-fhir/serve_observability_test.go`, `cmd/fi-fhir/serve_runtime_test.go`
  - `internal/workflow/{queue.go,queue_publish.go,queue_publish_leak.go,actions.go,engine.go,phi_redaction_test.go,coverage_helpers_test.go}`;
    deleted `internal/workflow/logging.go`, `logging_test.go`, `internal/ingest/**`
  - `internal/observability/{logging.go,logging_test.go,structured_logging_gate_integration_test.go,structured_logging_tags_*.go}`
  - `internal/api/graphql/{operation_authorization.go,server.go,compatibility_grant_log_test.go}`
  - `scripts/check-runtime-config.sh`, `docs/operations/PRODUCTION-HARDENING.md`
    (Audit Logging sections only — coordinate with S5-B, which owns PITR/recovery)
  - `deploy/kubernetes/base/configmap.yaml`,
    `deploy/helm/fi-fhir/templates/NOTES.txt`, `README.md`
  - `Makefile` (two targets, one `.PHONY` line)
  - Not touched: `.gitlab-ci.yml` (S5-0a owns the append point; S5-B wires
    `check-runtime-config` per correction 23).

- What's next:
  - **The tracing half of 4.4d is a follow-on slice**, which is the spec's own
    ordering ("no spans until the log half is merged and the PHI sites are
    closed"). It is: OTLP over HTTP off by default, `otlptrace`/`otlptracehttp`
    bumped from the `go.sum`-resident v1.19.0 to the SDK's v1.43.0 and promoted
    to direct requires, spans at ingress accept / workflow plan / delivery
    dispatch with the same field allowlist, and the "NOT IMPLEMENTED" labels
    removed from all nine artifacts in that MR.
  - Filed, not done: widening the four observation `Result` types with the
    lineage subset each stage holds, so component log lines can carry
    `correlation_id`/`trace_id`. Contended with S5-D and S5-F.

- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` Lane S5-C, corrections 23-34
  - [S2] `.loom/40-decisions.md` 2026-08-09 (Slice 4.4d)
  - [S3] `internal/observability/logging.go`; `metrics.go` (the label posture it
    mirrors)
  - [S4] `internal/observability/structured_logging_gate_integration_test.go`
  - [S5] MR !169 (the day-1 gate this branch inverts)
