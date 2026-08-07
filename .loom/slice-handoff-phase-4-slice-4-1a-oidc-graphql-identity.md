# Phase 4 Slice 4.1a Handoff — OIDC GraphQL Human Identity

## Slice Summary

- Milestone: Phase 4 Slice 4.1 — identity, authorization, and PHI policy.
- Slice: 4.1a — OIDC-authenticated GraphQL security context.
- Status: complete locally; merge-request landing evidence pending.

## What Landed

- Key changes:
  - A long-lived OIDC verifier discovers one HTTPS issuer, caches its JWKS,
    refreshes on an unknown key ID, and validates a protected `typ=at+jwt`
    access-token class, signature, exact issuer, one exact audience, algorithm
    allowlist, expiry/not-before, subject, exact deployment tenant, and strict
    roles. JWKS URLs remain HTTPS and redirects are rejected; remote timeout,
    response size, and refresh frequency are bounded.
  - Verified `sub`, tenant, and roles become a human
    `integration.SecurityContext` used by the existing GraphQL POST/SSE
    middleware and pre-resolver operation authorization.
  - `FI_FHIR_GRAPHQL_AUTH_MODE` selects mutually exclusive `static` or `oidc`
    configuration. OIDC rejects all compatibility credentials and trusted-CIDR
    bypasses; static mode rejects OIDC-only settings.
  - Operator and planning docs describe the new runtime boundary while keeping
    browser login, service identity, durable audit/PHI policy, WebSocket, and
    GitOps activation explicitly open.
- Key files:
  - `internal/api/requestsecurity/oidc.go`
  - `internal/api/graphql/oidc_security_test.go`
  - `cmd/fi-fhir/preview_runtime.go`
  - `docs/planning/GRAPHQL-API.md`
  - `.loom/iteration-plan-phase-4-slice-4-1a-oidc-graphql-identity.md`
- Validation results:
  - `go test -race ./internal/api/requestsecurity/... ./internal/api/graphql/... ./cmd/fi-fhir/...`: passed.
  - `go test -race ./...`: passed.
  - `make lint`, `go vet ./...`, `make docs-validate`, `go mod verify`,
    `go mod tidy -diff`, and `git diff --check`: passed.
  - `govulncheck ./...`: no reachable vulnerabilities; focused `gosec` on the
    new request-security package: no findings.
  - Terminal merge-request and post-merge CI evidence: pending landing.

## What Is Still Open

- Remaining acceptance criteria:
  - Terminal merge-request CI and post-merge main evidence.
- Known issues:
  - Checked-in Helm/Kustomize configuration still activates the static
    compatibility mode; production OIDC activation needs a separate GitOps
    review and live rollback proof.
  - `graphql:operator` remains a broad compatibility role pending fine-grained
    policy work.
- Dependencies:
  - Production requires a reachable standards-compliant HTTPS OIDC discovery
    endpoint whose JWKS contains keys for the configured asymmetric algorithms.
    Providers must issue `typ=at+jwt` tokens and should pre-publish rotated keys
    before use because outbound refresh has a 30-second default floor.

## Next Actions

1. Add OAuth service identities and one uniform object/action authorization
   policy across REST, MLLP, batch, GraphQL, actions, and delivery.
2. Add immutable security audit events with actor/reason/revision coverage.
3. Complete durable PHI retention, expiry, encryption, access, and export
   policy enforcement before reviewed GitOps activation.

## Context Links

- Agent-context session: `86e25f150bf94d44`
- Task ID: `54730eb4ca880501`
- Plan slice:
  `plan-complete-fi-fhir-as-a-production-integration-engine-and-ide-341d98#12`
- Relevant docs/specs:
  - `ROADMAP.md`
  - `.loom/20-product-spec-integration-engine-ide-completion.md`
  - `.loom/30-implementation-plan-integration-engine-ide-completion.md`
  - `.loom/40-decisions.md`

## Landing Evidence

- Branch: `codex/phase4-oidc-graphql-identity`
- Commit, MR, terminal pipeline, merge commit, and post-merge main pipeline:
  pending.
