# RALPH Slice Handoff

## Slice Summary

- Milestone: Phase 4 Slice 4.1 — identity, authorization, and PHI policy
- Slice: 4.1b1 — OAuth HTTP service identity and submit authorization
- Status: complete

## What Landed

- Key changes:
  - Added an allowlisted OAuth2 client-credentials identity path for production
    HL7v2 HTTP ingress, reusing the bounded OIDC discovery/JWKS verifier.
  - Preserved distinct per-request service subjects while binding source and
    revision only from deployment configuration and the immutable registry.
  - Added one fail-closed `integration.submit` decision over exact tenant,
    revision, and source, enforced at adapters, processor, and durable admission.
  - Preserved bearer/HMAC, MLLP, and batch compatibility through their existing
    server-issued grant names.
  - Forwarded the complete OAuth ingress contract through Docker Compose and
    added a runtime-config regression check for future variable drift.
- Key files:
  - `internal/api/requestsecurity/oidc.go`
  - `internal/integration/authorization/policy.go`
  - `internal/integration/ingress/`
  - `internal/integration/processor/message_processor.go`
  - `internal/integration/lifecycle/admission.go`
  - `cmd/fi-fhir/preview_runtime.go`
- Validation results:
  - Real-handler TLS kill-test and direct processor no-grant kill-test passed.
  - Focused race tests and `go test -race ./...` passed.
  - The tagged processor integration package compiles/passes with the new
    server-issued grant; its PostgreSQL body remains environment-gated locally.
  - Lint, vet, documentation validation, module verification/tidy, diff check,
    govulncheck, and high-severity/high-confidence gosec gates passed locally.
  - The rendered Compose configuration preserved every required/optional OAuth
    ingress setting; the runtime-config check and shellcheck passed.
  - Required merge-request and exact post-merge main CI evidence is harvested
    into Loom/context after landing.
  - Independent defect-focused review found and verified fixes for Compose
    propagation, legacy identifier compatibility, handler proof coverage, and
    the tagged production fixture; its final pass reported no findings.

## What Is Still Open

- Remaining acceptance criteria: required MR and exact main pipelines must reach
  terminal green before the RALPH task closes.
- Known issues: no implementation defect is known. Local Golden Path 001 could
  not start because the Docker daemon is unavailable, so its required CI job is
  authoritative; checked-in GitOps activation remains intentionally unchanged.
- Dependencies: the external authorization server must issue the documented
  signed JWT access-token profile; opaque-token introspection is not supported.

## Next Actions

1. Map verified MLLP certificate URI SAN/SPKI identity to immutable sources and
   reject CA-valid unmapped certificates.
2. Bind batch connector/workload identity and replace remote object modification
   time as trusted receipt provenance.
3. Add destination-scoped identity/authorization for the first durable HTTPS
   consumer, then extend policy to control-plane and PHI/audit operations.

## Context Links

- Agent-context session: `c1ddc0b2e3f25bbe`
- Task IDs: `3670d39e93141738`; plan
  `plan-complete-fi-fhir-as-a-production-integration-engine-and-ide-341d98`
- Relevant docs/specs:
  - `.loom/iteration-plan-phase-4-slice-4-1b1-oauth-http-submit-authorization.md`
  - `.loom/30-implementation-plan-integration-engine-ide-completion.md`
  - `.loom/40-decisions.md`
  - `docs/operations/PRODUCTION-HARDENING.md`
