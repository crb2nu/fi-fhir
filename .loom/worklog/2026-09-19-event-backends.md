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
