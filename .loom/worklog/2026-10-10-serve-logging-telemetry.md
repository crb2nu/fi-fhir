### 2026-10-10 — Serve logging and telemetry follow-on (4.4d-b)

Implemented the follow-on identified in the 2026-08-09 structured-logging
handoff and Sprint 6's deferred list:

- A private OTLP/HTTP provider consumes the existing tracing environment
  settings. It is disabled by default, requires an explicit collector URL when
  enabled, samples roots at the configured ratio, batches asynchronously, and
  flushes within a five-second shutdown budget. Export failures report a
  structured warning without collector response bodies or endpoint text.
- Context-aware callbacks observe admission, workflow planning, and claimed
  delivery. HTTP, MLLP, batch, and preview processor instances are bound at the
  serve composition boundary. Integration packages still import neither OTel
  nor the logger. Idle delivery polls produce no spans.
- Successful durable admissions and delivery logs/spans carry server-produced
  `receipt_id` and `lineage_trace_id`; delivery also carries its attempt ID.
  OTel `trace_id`/`span_id` remain distinct. Asynchronous delivery starts its own
  trace and joins by ledger identifiers without a schema migration.
- Correlation headers can contain PHI: the processor observer emits a SHA-256
  digest of the request correlation ID, never the original header or MSH-10.
  The digest remains pseudonymous metadata and needs access/retention controls.
- Identity-denied deliveries now increment `rejected` and emit a warning;
  previously the observation switch classified `OutcomeForbidden` as idle.
- Helm, Kubernetes, Compose, CLI help, environment examples, and operations docs
  now describe the implemented exporter. Deployment defaults remain disabled.
  The runtime-config check enforces opt-in defaults and Helm endpoint wiring.
- The existing blocking structured-logging proof now requires exported
  admission/planning spans and matching durable receipt/OTel IDs in real serve
  logs, in addition to its PHI and allowlist assertions. No new CI job.

Decisions:

- Do not install the provider globally: legacy workflow instrumentation has
  unrestricted attributes and errors and must not begin exporting implicitly.
- Do not reinterpret durable trace UUIDs as OTel trace IDs or persist new trace
  state in the forward-only ledgers. Preserve the durable join explicitly.
- Observation metadata excludes payloads and error text. Logs and spans use
  fixed outcome classifications; truncating an error is not redaction.
- Batch polls and session-stream aggregate logs remain tenant-scoped. Transport
  refusals before the shared processor remain outside these operation spans.

Validation:

- Full `go test -p 1 ./...` passed with `FI_FHIR_METRICS_PORT` set to a
  verified free port. The machine's occupied 9090 caused the existing graceful
  shutdown test to return early and send SIGINT to its test process on the
  initial runs; no product/test code was changed for that environment issue.
- Full affected-package suites passed with `-race`; targeted exporter tests
  additionally prove disabled/zero sampling, allowlisted scalar fields,
  parent/child correlation, global-provider isolation, invalid configuration,
  and redacted collector errors. Combined processor/delivery tests prove real
  canonical PHI sentinels never reach telemetry and identity denial is counted.
- Real PostgreSQL 16.14 + serve + OTLP collector gate passed, including receipt
  correlation. The existing structured-logging leak negative control passed.
- `make observability-replicas`, `make transport-gate`, and `make phi-audit`
  passed. The 64-way duplicate/fault/restart processor proof and PostgreSQL
  preview revision proof passed.
- A separate Kafka-backed delivery reliability test could not run: its
  testcontainers dependency has no local Docker, and the required remote
  `7900xtx` host rejected SSH while booting. Delivery unit/race tests passed.
- Scoped golangci-lint (including integration-tagged observability tests):
  zero issues. Helm lint and enabled render passed; enabling without an
  endpoint was correctly rejected. Runtime config: 23 passed, one existing
  non-blocking warning, zero failures. CI inventory: all 77 jobs unchanged.

The disposable local PostgreSQL instance was used because remote Docker was
unavailable; it is stopped after validation. No deployment setting was enabled.
