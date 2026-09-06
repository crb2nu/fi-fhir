### 2026-07-15: Keep Durable Acceptance Separate from At-Least-Once Kafka Delivery

- Decision:
  - Extend the Slice 1.2 submission schema forward with expiring outbox leases,
    bounded attempt schedules, destination-revision circuit state, durable DLQ,
    parent attempt links, idempotent operator operations, and append-only audit.
  - Claim due work with PostgreSQL `FOR UPDATE SKIP LOCKED`. A worker may update
    only its live lease; expiration requeues and audits work after restart.
  - Publish sanitized delivery commands to real Kafka with the stable attempt ID
    as record key and lineage headers. Require all in-sync replica acknowledgement
    and TLS whenever SASL credentials are configured.
  - Replay reuses the failed attempt identity. Resubmit creates one deterministic
    child. Both require a PostgreSQL-authenticated operator, unique operation key,
    and non-empty reason.
- Rationale:
  - PostgreSQL and Kafka cannot share the atomic admission transaction. A crash
    after Kafka acknowledgement but before the database success marker must be
    recoverable even though it can repeat a broker record.
  - Stable attempt identity gives consumers a deterministic duplicate-suppression
    key without making a false universal exactly-once claim.
  - Explicit lease/retry/circuit/DLQ state survives process and replica changes;
    the legacy memory/file workflow DLQ cannot satisfy that production boundary.
- Alternatives considered:
  - Treat the existing log queue driver as a real transport (rejected because it
    has no broker acknowledgement or cross-process durability).
  - Mark PostgreSQL published before Kafka acknowledgement (rejected because a
    crash can lose accepted work).
  - Mark only after acknowledgement and claim exactly-once delivery (rejected
    because a publish-before-database-ack crash can repeat the record).
  - Mutate failed rows directly for recovery (rejected because it loses operator,
    reason, idempotency, and parent-lineage evidence).
- Consequences:
  - Consumers must deduplicate unsafe effects by stable attempt ID.
  - The worker is explicitly enabled and can be rolled back without deleting work.
  - UI/GraphQL DLQ browsing remains Phase 3; Slice 2.3 supplies an authenticated
    PostgreSQL CLI recovery surface and backend contracts.
  - Webhook/FHIR/database/file execution and production GitOps activation remain
    separate reviewed work.
- Evidence:
  - MR `!106` pipeline `19226` passed 34/34, including PostgreSQL 16/Kafka job
    `185433`, and merged as `ca968fbf`. Main pipeline `19235` passed 37/37 and
    repeated the proof in job `185505`; evidence MR `!107` reconciled the record.
- Sources:
  - [S1] `internal/integration/delivery/`
  - [S2] `internal/integration/processor/migrations/0002_delivery_reliability.sql`
  - [S3] `cmd/fi-fhir/delivery_runtime.go`
  - [S4] `docs/operations/DELIVERY-RELIABILITY.md`
  - [S5] `.loom/iteration-plan-phase-2-slice-2-3-delivery-reliability.md`
