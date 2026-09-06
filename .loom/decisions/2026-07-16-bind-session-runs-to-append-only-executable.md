### 2026-07-16: Bind Session Runs to Append-Only Executable Profile Revisions

- Decision:
  - Persist Integration Sessions through a storage-neutral store with a
    tenant-scoped PostgreSQL implementation for sessions, samples, artifact
    revisions, runs, accepted decisions, and exports.
  - Redact samples before durable storage by default. Permit explicit retention
    only with AES-256-GCM protection bound to tenant/session/sample identity, and
    omit retained raw bytes from exports.
  - Append every artifact save as a new digest-bound revision. A preview run
    records and executes one exact profile revision through the production
    profile compiler; successful and failed terminal runs are immutable.
- Rationale:
  - Restart safety without exact executable provenance would let the IDE claim a
    result that a later mutable profile cannot reproduce.
  - Default redaction keeps the workspace useful for authoring while avoiding an
    implicit durable raw-PHI repository.
- Alternatives considered:
  - Continue with resolver-owned in-memory maps (rejected because sessions,
    decisions, and evidence disappear on restart).
  - Store only the current artifact head (rejected because prior runs would lose
    their executable input).
  - Retain raw samples by default (rejected because authoring convenience does
    not justify durable PHI without explicit policy and key material).
- Consequences:
  - Session execution remains HL7v2/profile-only in Slice 3.1.
  - Streaming, workflow simulation, signed publication, key rotation/expiry,
    fine-grained RBAC, and production GitOps activation remain separate work.
- Evidence:
  - Local focused/race/full tests, vet, scoped lint, UI type checks, and docs
    validation pass.
  - MR `!111` pipeline `19409` passed 37/37, including required PostgreSQL 16
    restart/raw-leakage job `187425`, and merged as `15746ccd`.
  - Main pipeline `19424` passed 40/40 and independently repeated the proof in
    job `187618`.
- Sources:
  - [S1] `internal/integration/session/`
  - [S2] `internal/integration/processor/profile_public.go`
  - [S3] `docs/operations/INTEGRATION-SESSIONS.md`
  - [S4] `.loom/iteration-plan-phase-3-slice-3-1-integration-session-workspace.md`
