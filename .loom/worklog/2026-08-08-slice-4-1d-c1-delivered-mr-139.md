### 2026-08-08 — Slice 4.1d C1 delivered (MR !139)

- What:
  - Extended schema-level immutability to the six unguarded durable-runtime
    tables. Blanket `BEFORE UPDATE OR DELETE` on the append-only ledgers
    (`integration_canonical_events`, `integration_message_lineage`,
    `integration_delivery_audit`, `integration_delivery_operations`,
    `integration_batch_audit`, and the newly guarded
    `integration_session_exports`); column-scoped `BEFORE UPDATE` plus blanket
    `BEFORE DELETE` on the two state tables `integration_receipts` and
    `integration_delivery_attempts`.
  - Made every session export an attributed disclosure: `principal_json`,
    `reason`, `include_raw_payload` `NOT NULL` on
    `integration_session_exports`; verified caller identity threaded from
    `requestsecurity.SecurityContextFromContext` through a new
    `session.ExportRequest`; `reason: String!` added to
    `ExportIntegrationBundleInput` (the sprint's only schema change).
  - Gated `includeRawPayload: true` behind the new dotted grant
    `integration.phi.export`, enforced at the resolver and again as a domain
    rule on the verified principal.
  - Published `docs/operations/PHI-RETENTION.md` and corrected the 4.1 bullet in
    `.loom/30-implementation-plan-integration-engine-ide-completion.md`.
- Why:
  - "Immutable audit storage" was 60% shipped and the durable-runtime half was
    the missing half. Export controls were 80% shipped but unattributed, which
    directly contradicted the product spec's requirement that data export record
    actor, reason, timestamp, and revision.
  - The slice's third claim — retention/TTL/encryption enforcement — has **no
    subject**. Production rejects every non-ephemeral raw retention mode, so
    there is no retained production raw PHI to expire; the PHI retained forever
    carries no policy field at all. That is S3-C2, and the plan text now says so.
- Evidence:
  - Day-1 gate passed both riskiest-assumption assertions before any migration
    was written, so the spec's C1/C2 split stands and `.loom/31` needed no
    correction.
  - Kill-test `TestPhiAudit_PostgresImmutableRecordsAndAttributedExport` green on
    PostgreSQL 16 with `-race`, 8 subtests, with a negative control on a second
    independently provisioned pre-migration schema where every `UPDATE`/`DELETE`
    succeeds and exports are written unattributed.
  - The negative control caught two real defects in the proof itself: `DELETE`
    aimed at rows with dependents was blocked by pre-existing `ON DELETE
    RESTRICT` foreign keys rather than by the new guards, and an `UPDATE` against
    an empty `integration_delivery_audit` passed vacuously. Both are now
    structurally impossible.
  - Over-lock checks: `make integration-session`, `make delivery-reliability`
    (Postgres + Kafka), `make operator-control-plane`, and the 64-way durable
    submission proof all pass unchanged with the triggers active.
  - `gofmt` clean, `golangci-lint run` 0 issues, `go vet ./...` clean,
    `go test -race ./...` green, `make lint-gqlgen` up to date after committing
    the regenerated artifacts, `npm run codegen:check` clean.
- What's next:
  - S3-C2: retention policy design for canonical events, session samples, and
    export snapshots; TTL columns; a durable purge component that reconciles
    with these immutability triggers. See
    `.loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md` "Next Actions".
- Sources:
  - [S1] `.loom/iteration-plan-phase-4-slice-4-1d-c1-phi-audit.md`
  - [S2] `.loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md`
  - [S3] `docs/operations/PHI-RETENTION.md`
  - [S4] Command: `make phi-audit` with `POSTGRES_TEST_URL` set
