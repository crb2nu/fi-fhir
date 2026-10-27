### 2026-10-27 - 279 review fixes: span attribute allowlist and collector-side negative control

- What changed: `internal/observability/tracing.go` gives spans their own closed
  attribute set, `permittedSpanAttributes` — 20 keys: the four enumerations,
  eleven durable or operator-chosen identifiers, five computed scalars — a
  strict subset of the 37-field log allowlist that refuses `source_message_id`,
  `error` and `reason`, and reports whatever it refused on the span as
  `dropped_attributes`, the way the log handler reports `dropped_fields`.
  `KnownSpanAttribute`, `PermittedSpanAttributes` and `RejectedSpanAttributes`
  expose the set to the proofs. `cmd/fi-fhir/serve_tracing_correlation.go`
  holds the caller-header hash behind `spanCorrelationID`, and its `tracingleak`
  twin restores the raw header: the structured-logging proof now submits its
  HL7 message under `X-Correlation-ID: CORRSENTINEL4404`, requires the SHA-256
  of that header on the admission span and on a log line and the raw text
  nowhere, and `make tracing-negative-control` inverts the serve subtest under
  the tag. `ci/test-structured-logging.yml` runs the third existence guard and
  the new target; the integration harness gained `submitHL7Correlated`.
- Why: the 2026-10-27 review of !279 found that `traceAttributes` admitted
  every log field on spans (W1) — `source_message_id` is sender text and
  `error` may quote a record, and neither belongs in a collector the operator
  may not run — and that the merge request's "collector-side leak negative
  control" did not exist (W2): the only control, `structuredloggingleak`,
  restores a log-side print, so the span PHI scan had never been seen to fail.
- Evidence: `go build ./...`, `go build -tags tracingleak ./cmd/fi-fhir`, and
  `go vet` of the integration test package under `integration`,
  `integration tracingleak` and `integration structuredloggingleak` all pass;
  `go test -race` of `internal/observability` and `cmd/fi-fhir` passes with the
  two new unit tests (four plants under listed log names are refused, counted
  and reported as `dropped_attributes=4`; the span set is a strict subset of the
  log set; the hashed caller header is present on admission telemetry while its
  text is not); `golangci-lint` reports no issue on the shipped build;
  `scripts/ci-job-inventory.sh --check` still counts 77 jobs. Against a
  `postgres:16-alpine` container on the 7900xtx Docker context, all three
  existence guards list the test, `make structured-logging` passes (20.8 s),
  `make tracing-negative-control` reports OK — the tagged serve subtest fails
  on the raw header reaching both the admission and planning spans, on the
  admission span's unhashed `correlation_id`, and on the log line that shares
  the field — and `make structured-logging-negative-control` still reports OK.
- What's next: W3 (the asynchronous delivery trace join is not asserted in CI),
  W4 (the structured-logging proof forces tracing on, so the default-off path
  is uncovered) and S1 (the correlation hash is unkeyed; an HMAC under a
  deployment key would stop a dictionary from reversing low-entropy headers) are
  filed as follow-up issues. S2–S5 from the same review — collector partial
  success bypasses `reportingExporter`, `scripts/check-runtime-config.sh` is
  weaker than before the branch, PHI-RETENTION and SECURITY docs do not mention
  span attributes, a duration assertion in `delivery/observation_test.go` can
  flake — are recorded in the review note on the merge request.
- Sources:
  - [S1] !279 review note, 2026-10-27
  - [S2] `internal/observability/structured_logging_gate_integration_test.go`, header § Negative controls (c)
  - [S3] `internal/workflow/queue_publish_leak.go` (the control idiom)
