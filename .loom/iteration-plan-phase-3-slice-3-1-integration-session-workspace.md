# RALPH Iteration Plan — Phase 3 Slice 3.1 Integration Session Workspace

**Status**: Ready for CI
**Date**: 2026-07-16
**Branch**: `codex/phase3-integration-session-workspace`
**Base**: `main` at `304fcb39`

## Review

- Roadmap milestone: Phase 3 durable IDE lifecycle, Slice 3.1.
- Spec sections:
  - `.loom/20-product-spec-integration-engine-ide-completion.md` durable IDE and
    healthcare-grade governance contracts;
  - `.loom/30-implementation-plan-integration-engine-ide-completion.md` Slice
    3.1;
  - `.loom/slice-handoff-phase-2-slice-2-4-batch-ingestion.md`.
- Prior decisions to preserve:
  - preview remains side-effect-free and shares the production profile compiler;
  - exact immutable artifact revisions, rather than mutable pointers, bind runs;
  - raw PHI is redacted by default and may not enter diagnostics or durable run
    records;
  - PostgreSQL owns restart recovery state;
  - streaming UI, workflow simulation, and production GitOps activation remain
    separate slices.

## Riskiest assumption + kill-test

**Load-bearing assumption**: A storage-neutral Integration Session service can
persist redacted samples, immutable executable profile revisions, terminal runs,
accepted decisions, and exports in PostgreSQL while applying the same profile
compiler as the production processor after a complete backend restart.

**Kill test**: Start a PostgreSQL-backed session service, create a workspace,
store one HL7v2 sample using the default redaction policy, save strict and
tolerant immutable profile revisions, and run the strict revision. Destroy every
in-process store/service object, construct a fresh service against the same
database, reopen the workspace, run the tolerant revision, accept a diagnostic,
archive and list/reopen the session, and export it. The proof must observe the
expected warning/event delta, exact revision IDs and digests on both immutable
runs, durable accepted-decision/export records, and zero occurrence of the raw
PHI sentinel in session tables.

**Failure mode if the assumption is wrong**: The IDE would present a workspace
that survives restart but cannot reproduce the exact profile behavior it claims
to have tested, or it would make raw clinical data durable by default. Slice 3.2
and later publish/deploy work must not proceed on that foundation.

**Status**: the integration test compiles locally. The live PostgreSQL proof is
pending the required CI job because the local Docker daemon is unavailable.

## Align

- Slice name: restart-safe Integration Session Workspace.
- Scope in:
  - storage-neutral `Store` contract with memory and PostgreSQL implementations;
  - sessions, redacted samples, append-only artifact revisions, terminal runs,
    accepted decisions, and export audit records;
  - stable create/list/get/archive and restart/reopen behavior;
  - exact profile revision binding and digest verification for HL7v2 preview;
  - GraphQL resolver/runtime composition using the durable store when configured;
  - PostgreSQL restart/raw-leakage kill test and required CI target;
  - operator and roadmap documentation.
- Scope out:
  - WebSocket/subscription production enablement and UI stage streaming (3.2);
  - workflow revision simulation and action traces (3.3);
  - signed bundles, approval, deployment, and GitOps activation (3.4);
  - Phase 4 OIDC/RBAC and multi-tenant hosting;
  - non-HL7 session preview expansion.
- Acceptance criteria:
  1. A fresh service instance reopens sessions, samples, artifacts, runs,
     decisions, and exports from PostgreSQL.
  2. Samples default to redacted durable content; retained raw content requires
     an explicit retention request and is never included in exports by default.
  3. Saving an artifact creates an append-only revision with a content digest;
     prior revisions cannot be updated or deleted through the store contract.
  4. A preview pins one exact profile revision ID and digest and rejects missing,
     mismatched, or malformed profile content.
  5. Terminal runs are immutable and retain their stages, diagnostics, lineage,
     event projection, and exact profile reference across restart.
  6. Accepted diagnostics and export operations persist as auditable records.
  7. Archive is idempotent; archived sessions are omitted by default and can be
     explicitly listed and reopened by stable ID.
  8. The PostgreSQL kill test, targeted/race tests, full Go suite, vet, lint, and
     required CI job pass without raw-PHI leakage.
- Dependencies/blockers:
  - PostgreSQL 16 for durable workspace state;
  - existing exact profile digest and compiler behavior in
    `internal/integration/processor`;
  - existing authenticated GraphQL transport and profile/session schema.
- Rollback:
  - durable session wiring is opt-in with database configuration;
  - memory-backed tests and non-session runtime paths remain compatible;
  - reverting this slice removes only the new session schema/configuration and
    leaves production ingress/delivery tables untouched.

## Land

- Planned file areas:
  - `internal/integration/session/` contracts, PostgreSQL store, migration,
    profile-aware runner, and tests;
  - `internal/integration/processor/` public exact-profile compile adapter;
  - `internal/api/graphql/resolvers/` store-neutral durable service wiring;
  - `cmd/fi-fhir/` optional PostgreSQL session composition;
  - `Makefile`, `.gitlab-ci.yml`, operations and roadmap records.
- Implementation steps:
  1. Seal store, immutable revision/run, redaction, decision, and export behavior
     with focused tests.
  2. Implement PostgreSQL migration and store with defensive JSON decoding and
     terminal-run immutability.
  3. Bind runs to exact digest-verified profile revisions through the shared
     production profile compiler.
  4. Wire durable resolver/runtime composition and stable GraphQL routes.
  5. Add the full restart/leakage kill test and CI evidence target.

## Prove

- Tests to run:
  - `go test ./internal/integration/session ./internal/api/graphql/resolvers ./cmd/fi-fhir`;
  - `go test -race ./internal/integration/session ./internal/api/graphql/resolvers ./cmd/fi-fhir`;
  - required PostgreSQL restart/raw-leakage integration test;
  - `go test ./...`.
- Lint/static checks:
  - `gofmt` on changed Go files;
  - `go vet ./...`;
  - gqlgen synchronization if the schema changes;
  - scoped golangci-lint and secret/PHI diff review.
- CI checks:
  - standard required pipeline jobs;
  - required PostgreSQL 16 Integration Session restart kill-test job.

## Handoff/Harvest

- Local evidence:
  - focused and race tests pass for session, processor, GraphQL resolvers, and
    `cmd/fi-fhir`;
  - `go test ./...` and `go vet ./...` pass;
  - the integration-tag package compiles without running the Docker-dependent
    test;
  - scoped golangci-lint reports zero issues;
  - Svelte check reports zero errors and nine pre-existing CSS warnings;
  - documentation validation passes.
- Required CI evidence: `test:integration-session` against PostgreSQL 16.

- Docs to update:
  - this plan and a Slice 3.1 handoff;
  - product spec, implementation plan, decision log, roadmap/status/changelog;
  - Integration Session operations guidance.
- Agent-context entries to add:
  - store/immutability and raw-retention decisions;
  - exact profile revision/compiler decision;
  - final local/CI/merge evidence.
- Next-slice candidate: Phase 3 Slice 3.2 streaming diagnostics and server
  lineage after evidence reconciliation.
