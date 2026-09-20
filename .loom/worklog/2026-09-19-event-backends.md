### 2026-09-19 - Event backends and common processors

- What changed: added public Kafka, Redis Streams, and Google Cloud Pub/Sub
  backends with publish/consume contracts, common JSON/chain handlers, workflow
  queue adapters, `workflow consume`, and an event-sourcing outbox adapter.
- Why: only `log` was registered for workflow queue actions. Kafka existed in
  the separate identity-bound durable destination dispatcher; Redis and Pub/Sub
  were absent. User requested all three backends and common event processors.
- Decision: Pub/Sub means Google Cloud Pub/Sub. Redis uses Streams consumer
  groups so failed events remain available. Consumers acknowledge after success
  and stop on failure; no silent drops or claim of exactly-once delivery.
  Existing durable destination transport/authorization remains unchanged.
- Evidence: the shared acknowledgment test exposed Redis reading a fresh event
  before its own failed pending event; startup now drains its pending list
  before claiming abandoned deliveries and reading new messages. Protocol tests
  cover all three backends, including key/header preservation and cancellation.
  The blocking `test:event-backends` job adds real Kafka/Redis verification.
- Operational notes: Redis handlers must honor context deadlines; retention
  must preserve pending entries. Outbox and workflow handlers remain at least
  once, and applications must make side effects idempotent.
- Sources: `pkg/eventbus/`, `internal/workflow/queue_backends.go`,
  `pkg/eventsourcing/eventbus.go`, `cmd/fi-fhir/workflow_consume.go`, and
  `docs/operations/EVENT-BACKENDS.md`.
- Sprint 6 integration: this merge candidate preserves the exact reviewed heads
  of !204–!209. It includes FHIR destination URL/token escaping for observation
  IDs containing `#`, restores the invalid-CEL E2E regression, and uses the same
  CLI validation diagnostics for `workflow consume`. The observability harness
  allows 180 seconds for migration startup while keeping readiness/fanout
  budgets unchanged; failure logs are read only after the child output writer
  has stopped, removing the race exposed in pipeline 27728.
- Combined local verification: `go test -race ./...`, focused CLI consumer and
  validation regressions, `golangci-lint run` (zero issues), docs/worklog/decision
  checks, and CI inventory (67 jobs) passed. `make fhir-structural
  fhir-structural-negative-control fhir-destination-negative-control test-e2e`
  passed, including both deliberately failing FHIR controls. Live Redis passed;
  live Kafka and the PostgreSQL-backed proofs remain required in CI.
- CI dependency repair: pipeline 27743's Node jobs stalled installing pinned
  npm through Verdaccio. From the affected pod, the internal `/npm` metadata
  request timed out after five seconds while public npm returned HTTP 200 in
  121 ms. Node setup now probes the pinned npm metadata with a ten-second bound
  and no retries, falling back to the public registry on failure. npm version,
  lockfiles, and required checks stay pinned and enforced.
- Sprint 6's previous migration failure (job 263481, pipeline 25993) reached
  the fourth restore-attribution proof after about eight minutes, then hit the
  aggregate ten-minute timeout while running its dump/restore subprocess. The
  migration target now allows twenty minutes and emits per-test progress;
  assertions, negative controls, and measured recovery objectives are unchanged.
