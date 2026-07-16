# RALPH Iteration Plan — Phase 2 Slice 2.4 Batch Ingestion

**Status**: Ready for review; all local proof green, CI/merge evidence pending
**Date**: 2026-07-16
**Branch**: `codex/phase2-batch-ingestion`
**Base**: `main` at `6d4fd184`

## Review

- Roadmap milestone: Phase 2 secure production data plane, Slice 2.4 batch
  sources.
- Spec sections:
  - `.loom/20-product-spec-integration-engine-ide-completion.md` secure data
    plane, recovery budgets, and batch clinical import journey;
  - `.loom/30-implementation-plan-integration-engine-ide-completion.md` Slice
    2.4;
  - `.loom/slice-handoff-phase-2-slice-2-3-delivery-reliability.md`.
- Prior decisions to preserve:
  - production adapters resolve only an exact deployed release;
  - PostgreSQL owns durable admission and recovery state;
  - raw PHI remains ephemeral by default and must not enter logs/checkpoints;
  - downstream delivery remains separate and at-least-once;
  - production GitOps activation is a separate reviewed operation.

## Align

- Slice name: runtime-wired S3/SFTP batch ingestion.
- Scope in:
  - immutable, content-addressed S3/SFTP source revisions validated against the
    exact lifecycle binding and secret-binding names;
  - opt-in `serve` runtime wiring for one deployed batch source;
  - bounded-memory streaming of concatenated UTF-8 HL7v2 messages;
  - PostgreSQL object leases and byte/message checkpoints with deterministic
    per-message idempotency keys;
  - restart recovery from the last durable checkpoint without duplicate
    receipt/event/outbox records;
  - mandatory SFTP host-key verification through a bounded `known_hosts` file;
  - digest-addressed, verified, idempotent archive-before-delete behavior for S3
    and SFTP;
  - unit, race, PostgreSQL, MinIO, and SFTP recovery/security evidence plus
    operator documentation.
- Scope out:
  - Temporal or Mentatlab control-plane orchestration;
  - batch parsing for formats not yet supported by the shared production
    processor;
  - UI/GraphQL batch controls and browser trace surfaces;
  - production credentials, public exposure, and GitOps activation;
  - the separate generic `pkg/storage` coverage backlog.
- Acceptance criteria:
  1. Invalid/tampered source revisions, lifecycle mismatches, partial runtime
     configuration, insecure S3 credential transport, and unverified SFTP hosts
     fail closed before polling.
  2. Each discovered object is leased durably by exact provider/path/version;
     only one replica processes it at a time and expired work is reclaimable.
  3. The reader holds at most one bounded HL7v2 message in memory and advances
     a PostgreSQL byte/message checkpoint only after durable admission returns.
  4. A crash after admission but before checkpoint advancement repeats the same
     idempotency key and creates no duplicate durable receipt, event, or outbox
     row.
  5. Source mutation under an existing path is a new object version and cannot
     inherit an older checkpoint.
  6. Completion moves the exact source bytes to a content-digest archive path,
     verifies the archived digest, and deletes the source only after successful
     verification; retries are idempotent and collisions fail closed.
  7. Runtime shutdown stops polling, releases provider resources, and preserves
     all PostgreSQL recovery state.
  8. Targeted tests, race tests, the full Go suite, lint/static checks, and the
     required CI recovery job pass.
- Dependencies/blockers:
  - PostgreSQL 16 for lifecycle, admission, and batch checkpoint state;
  - an S3-compatible endpoint or SFTP endpoint selected by the source revision;
  - existing exact artifact registry and deployed lifecycle record.
- Risk notes:
  - the load-bearing recovery invariant is deterministic object/message identity
    across the admission/checkpoint crash window;
  - archive deletion must never precede digest verification;
  - object metadata and checkpoint diagnostics must remain raw-free;
  - provider reconnects must retain host-key and source-version checks.

## Land

- Planned file areas:
  - `internal/integration/batch/` for source contracts, streaming, durable state,
    providers, runtime, migrations, and tests;
  - `cmd/fi-fhir/preview_runtime.go` and `cmd/fi-fhir/main.go` for opt-in lifecycle;
  - `.env.example`, CI, operations docs, status/roadmap/spec/decision records.
- Implementation steps:
  1. Seal the source, provider, checkpoint, and archive contracts with unit tests.
  2. Implement PostgreSQL leasing/checkpoint transitions and the streaming
     processor loop.
  3. Implement secure S3/SFTP providers and runtime configuration.
  4. Add crash-window, restart, mutation, archive, host-key, and bounded-memory
     proofs.

## Prove

Local evidence:

- `go test ./internal/integration/batch ./cmd/fi-fhir` passes.
- `go test -race ./internal/integration/batch ./cmd/fi-fhir` passes.
- `go test ./...` and `go vet ./...` pass.
- Scoped golangci-lint for `internal/integration/batch` and `cmd/fi-fhir`
  passes with zero findings. The repository-wide lint command reports only
  pre-existing generated GraphQL findings.
- The PostgreSQL 16/MinIO/SSH-SFTP integration test passes under `-race` and
  proves lease exclusion/reclaim, kill/resume, exact admission cardinality,
  object mutation/release isolation, wrong-host-key rejection, digest-verified
  archive, S3 overwrite-safe exact-version deletion, source deletion ordering,
  and raw-PHI exclusion.
- Full repository and CI evidence will be recorded before this iteration closes.

- Tests to run:
  - `go test ./internal/integration/batch ./cmd/fi-fhir`;
  - `go test -race ./internal/integration/batch ./cmd/fi-fhir`;
  - PostgreSQL/MinIO/SFTP integration recovery test selected by the CI target;
  - `go test ./...`.
- Lint/static checks:
  - `gofmt` on changed Go files;
  - `go vet ./...`;
  - repository lint/pre-commit targets when available;
  - scoped diff and secret/PHI leakage review.
- CI checks:
  - required standard pipeline jobs;
  - required batch-ingestion recovery/security job with PostgreSQL 16 and real
    S3/SFTP-compatible services.

## Handoff/Harvest

- Docs to update:
  - this iteration plan and a Slice 2.4 handoff;
  - product spec, implementation plan, decision log, roadmap/status/changelog;
  - production batch-ingestion operations/runbook/hardening guidance.
- Agent-context entries to add:
  - durable checkpoint/admission identity decision;
  - archive verification and SFTP host-key decisions;
  - final CI/merge evidence and remaining rollout boundary.
- Next-slice candidate: Phase 3 Slice 3.1 restart-safe Integration Session
  Workspace after Phase 2 evidence reconciliation.
