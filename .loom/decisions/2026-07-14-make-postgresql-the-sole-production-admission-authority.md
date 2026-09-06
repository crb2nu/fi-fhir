### 2026-07-14: Make PostgreSQL the Sole Production Admission Authority

- Decision:
  - Keep one `MessageProcessor` evaluation path for preview and production.
    Preview remains SQL-free; production is available only through an explicitly
    configured `PostgresSubmissionStore`.
  - Commit the receipt, sanitized canonical event, exact artifact/trace lineage,
    one initial queued attempt per external action, and one pending outbox row
    per attempt in one fixed-schema PostgreSQL transaction.
  - Arbitrate duplicates with one tenant-scoped unique effective key. An explicit
    key wins; otherwise derive a domain-separated digest from tenant, source,
    MSH-10, and the exact integration revision. Bind that key to a separate
    request fingerprint so changed content fails closed.
  - Treat any `COMMIT` error as outcome-unknown. A retry evaluates the same pure
    plan, loses the unique-key claim if the first commit survived, and returns
    the first stored `ProcessResult` exactly.
  - Keep legacy GraphQL production submit disabled. Slice 1.3 adds the first
    authenticated production ingress on this committer.
- Rationale:
  - A positive acknowledgement cannot safely precede event/outbox durability or
    cross two independent stores.
  - The legacy `pkg/eventsourcing.OutboxEventStore` writes event and outbox in
    separate calls and intentionally ignores an outbox-save failure; it cannot
    provide the Slice 1.2 guarantee.
  - Deterministic IDs make rollback, restart, and commit-unknown behavior
    inspectable, while the unique effective key remains the database authority.
  - Storing the validated raw-free result on the receipt makes duplicate
    responses stable even when retry correlation or receive-time metadata changes.
- Alternatives considered:
  - Reuse `OutboxEventStore` (rejected because it is non-transactional).
  - Persist the receipt first and append events/outbox afterward (rejected
    because it acknowledges a partial admission state).
  - Add a generic committer interface with memory and PostgreSQL variants
    (rejected because production durability must not be accidentally configured
    to an in-memory implementation).
  - Reactivate legacy GraphQL submit in this slice (rejected because its broader
    catalog remains intentionally contained and Slice 1.3 owns ingress policy).
- Consequences:
  - Startup must apply the numbered submission migration before constructing a
    durable processor.
  - Raw retention remains ephemeral-only; encrypted raw storage needs its own
    encrypted store, TTL, purpose, and access-audit implementation.
  - Slice 1.2 guarantees durable acceptance once and seeds at-least-once outbox
    delivery. Polling, leases, retries, DLQ, replay, and external-effect
    idempotency remain Phase 2 work.
- Sources:
  - [S1] `internal/integration/processor/postgres_submission.go`
  - [S2] `internal/integration/processor/migrations/0001_atomic_submission.sql`
  - [S3] `internal/integration/processor/postgres_submission_integration_test.go`
  - [S4] `pkg/eventsourcing/outbox.go`
  - [S5] `.loom/iteration-plan-phase-1-slice-1-2-durable-submission.md`
