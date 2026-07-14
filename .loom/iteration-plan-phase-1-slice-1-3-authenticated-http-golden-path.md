# RALPH Iteration Plan — Phase 1 Slice 1.3 Authenticated HTTP + Golden Path 001

**Status**: Local proof complete; required CI and merge pending
**Date**: 2026-07-14
**Plan**: `plan-complete-fi-fhir-as-a-production-integration-engine-and-ide-341d98#6`
**Branch**: `codex/phase1-authenticated-http`

## Riskiest assumption + kill-test

**Load-bearing assumption**: The authenticated raw-HL7v2 HTTP adapter and the
Integration Session/IDE preview adapter can use one live, PostgreSQL-backed
`MessageProcessor` instance and the same immutable registry revisions while
preserving three boundaries: production acknowledges only the atomically durable
Slice 1.2 result, preview creates no receipt/attempt/outbox rows, and raw clinical
bytes never enter durable storage or an HTTP response.

**Kill test**: `make golden-path-001` builds the current binary and fixture
registry, owns an isolated PostgreSQL 16 environment (Compose locally or the
dedicated CI service), starts the real `serve` process, and performs the following
protocol journey within 30 minutes:

1. reject wrong method, media type, origin, credential, oversized body, malformed
   identity/idempotency, strict-profile parse, and changed-body idempotency reuse
   with structured non-PHI 4xx responses;
2. submit the tolerant-profile ADT A01 twice with one bearer credential and one
   idempotency key, requiring byte-identical `202` responses and exactly one
   receipt/event/lineage/attempt/outbox unit;
3. call the authenticated Integration Session preview mutation for the same
   payload and exact revisions, requiring equivalent normalized canonical payload,
   warning codes/paths, routes, and provenance with suppressed delivery and no
   additional durable row;
4. terminate and restart the real server process, resubmit the duplicate, and
   require the original byte-identical response plus queryable receipt, warning,
   profile/workflow, route/action, correlation, and queued-delivery evidence; and
5. scan every persisted JSON value and public response for the raw/credential
   sentinels, then write assertions, JUnit, logs, HTTP captures, and SQL exports
   under `.tmp/golden-path-001/`.

HMAC-SHA256 receives the same bounded-body and constant-time credential unit and
transport tests; the end-to-end Golden Path uses bearer auth so its browser/IDE
credential and adapter credential remain independently attributable.

**Failure mode if the assumption is wrong**: Production and IDE paths would be
parallel engines with revision or diagnostic drift, or the HTTP acknowledgement
could escape the durable transaction. Phase 2 must remain blocked until the
runtime boundary is redesigned.

**Status**: passed locally. Positive code evidence is the shared evaluator and durable
committer in `internal/integration/processor`; negative evidence is that the
existing `internal/ingest` handler wraps generic JSON, permits auth-disabled
configuration, calls the workflow engine directly, and cannot satisfy the
receipt/provenance contract. Golden Path 001 passed 20 assertions with exact
duplicate responses before/after restart, one five-row durable unit, selected-
profile divergence, exact IDE parity, suppressed preview delivery, and clean
raw/credential scans. Machine evidence is generated under
`.tmp/golden-path-001/`; the required CI job remains the merge authority.

## Review

- Roadmap milestone: Engine Alpha / Golden Path 001.
- Spec sections: product-spec kill-test recipe; implementation plan Slice 1.3;
  PostgreSQL-only admission ADR dated 2026-07-14.
- Prior decisions to preserve:
  - one logical tenant/security domain per deployment;
  - server-owned integration/source identity and exact artifact revisions;
  - preview and production share semantics, but only production can commit;
  - raw retention remains ephemeral-only;
  - legacy GraphQL submit and generic webhook execution paths remain contained;
  - durable acceptance once and at-least-once delivery starts at the outbox.

## Align

- Slice name: authenticated HL7v2 HTTP ingress and Golden Path 001.
- Scope in:
  - exact `POST /v1/hl7v2` endpoint with server-resolved integration/source;
  - bearer and HMAC-SHA256 credentials from direct or file-backed deployment
    secrets, mapped to a service principal;
  - 1-MiB-or-smaller bounded raw body, exact HL7 media type, no browser Origin,
    canonical correlation/idempotency headers, and cache-disabled JSON responses;
  - structured 4xx diagnostics, conflict semantics, and retryable durable 5xx;
  - production runtime composition, migration, shared processor, and graceful DB
    cleanup only when ingress is explicitly enabled;
  - response projection containing receipt, event summary, warnings, provenance,
    correlations, routes, and queued delivery state without canonical PHI payload;
  - Golden Path fixtures, orchestration, evidence, and blocking CI job.
- Scope out:
  - outbox polling/external delivery, retries, DLQ, replay, or resubmit;
  - MLLP, S3/SFTP, Kafka, or integration deployment lifecycle;
  - durable Integration Session storage or re-enabling legacy GraphQL catalog;
  - OIDC/fine-grained RBAC and production GitOps activation.
- Acceptance criteria:
  - all transport rejection/response/auth tests pass for bearer and HMAC;
  - runtime startup fails closed when enabled ingress lacks auth or PostgreSQL;
  - one shared durable processor serves production and preview;
  - `make golden-path-001` passes the exact duplicate/restart/profile/IDE parity
    assertions and emits complete machine-readable evidence;
  - CI runs the target as a required PostgreSQL 16 job;
  - roadmap/status/runbook/config docs match the executable boundary.
- Dependencies/blockers:
  - Slice 1.2 schema and committer (merged in `e8a93553`);
  - PostgreSQL 16 service for the kill test;
  - Docker/Compose only for the self-owned local path; CI may inject its isolated
    PostgreSQL service while still running the same target and real process.
- Rollback:
  - unset `FI_FHIR_HTTP_INGRESS_AUTH_MODE`; the endpoint is not mounted and the
    existing authenticated preview runtime remains unchanged.

## Land

- Planned file areas:
  - `internal/integration/ingress/`
  - `internal/api/graphql/server.go`
  - `cmd/fi-fhir/preview_runtime.go`, `cmd/fi-fhir/main.go`
  - `scripts/golden-path-001*`, `testdata/golden/integration/adt-http/`
  - `Makefile`, `.gitlab-ci.yml`, deployment/example config and documentation.
- Implementation steps:
  1. Add the trusted ingress service, bounded HTTP handler, credential verifier,
     response projection, and exhaustive transport tests.
  2. Compose optional production ingress at startup with one migrated PostgreSQL
     store and one durable processor shared by GraphQL preview.
  3. Add deterministic strict/tolerant/workflow fixtures and the real-process
     restart harness with SQL/HTTP/JUnit evidence.
  4. Add the blocking CI job and synchronize operator/developer documentation.

## Prove

- Tests to run:
  - `go test -race -count=1 ./internal/integration/ingress ./internal/api/graphql ./cmd/fi-fhir`
  - `go test -count=1 ./...`
  - `make golden-path-001`
- Lint/static checks:
  - `go vet ./internal/integration/ingress ./internal/api/graphql ./cmd/fi-fhir`
  - focused golangci-lint, `make security-gosec`, `make security-vulncheck`
  - `bash scripts/validate-docs.sh`, `git diff --check`
- CI checks:
  - required PostgreSQL 16 `test:golden-path-001` job;
  - existing binary, UI, smoke, race, security, image, and deployment jobs.

## Handoff/Harvest

- Docs to update: this plan, `.loom/30-implementation-plan-*`, `.loom/40-decisions.md`,
  `ROADMAP.md`, `docs/STATUS.md`, production hardening/runbook/getting-started, and
  changelog.
- Agent-context entries: transport identity/auth decision, Golden Path outcome,
  exact pipeline/job evidence, and any disconfirming finding.
- Next-slice candidate: Phase 2 Slice 2.1 integration deployment lifecycle only
  after Golden Path 001 passes.
