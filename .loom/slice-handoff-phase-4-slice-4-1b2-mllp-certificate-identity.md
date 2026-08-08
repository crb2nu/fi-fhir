# RALPH Slice Handoff

## Slice Summary

- Milestone: Phase 4 Slice 4.1 — identity, authorization, and PHI policy
- Slice: 4.1b2 — MLLP certificate service identity and submit authorization
- Status: complete

## What Landed

- Key changes:
  - Added an explicit, fail-closed `clients.identities` allowlist to the
    immutable, content-addressed MLLP source revision. Each entry maps one
    authority-scoped certificate criterion — a URI subject alternative name, a
    subject public key info SHA-256 pin, or both — to one canonical service
    subject and its grants. Common names are never accepted as identity.
  - Resolved one verified `ConnectionIdentity` per accepted TLS connection
    immediately after the handshake, before any frame is read, parsed,
    processed, or durably admitted. A CA-valid certificate matching zero entries
    or more than one entry closes the connection with no acknowledgement.
  - Carried the verified subject, auth method, and grants into the same
    fail-closed `authorization.AuthorizeSubmission` decision introduced in Slice
    4.1b1, and moved that decision ahead of capacity acquisition and envelope
    construction so no denial path allocates work. The processor and
    transaction-scoped runnable admission still re-evaluate it.
  - Kept tenant, integration revision, and source identity server-owned: the
    source is bound from the deployed release after binding validation, never
    from the certificate or the message.
  - Preserved compatibility: an empty identity list keeps the deployment-fixed
    `FI_FHIR_MLLP_PRINCIPAL_ID` principal and server-issued `integration:mllp`
    grant, and existing source revisions keep their exact digest because the new
    field is omitted from the canonical digest input when empty. Mapping is
    all-or-nothing per listener; `resolvePrincipal` rejects cross-mode use in
    both directions, so no connection can fall back.
  - Added `FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY` so a deployment refuses to
    start if the mounted source document drops back to compatibility mode,
    forwarded the complete MLLP runtime contract through Docker Compose, and
    extended `scripts/check-runtime-config.sh` with a required MLLP compose
    check.
  - Extended `make mllp-runtime` and the required `test:mllp-runtime` job to
    discover and run both MLLP runtime proofs.
- Key files:
  - `internal/integration/mllp/identity.go` (new)
  - `internal/integration/mllp/source.go`
  - `internal/integration/mllp/server.go`
  - `internal/integration/mllp/service.go`
  - `internal/integration/mllp/identity_test.go` (new)
  - `internal/integration/mllp/identity_integration_test.go` (new kill test)
  - `cmd/fi-fhir/preview_runtime.go`, `cmd/fi-fhir/main.go`
  - `docker-compose.yaml`, `.env.example`, `scripts/check-runtime-config.sh`
  - `docs/operations/PRODUCTION-MLLP.md`, `docs/operations/PRODUCTION-HARDENING.md`
  - `Makefile`, `.gitlab-ci.yml`
- Validation results:
  - Kill test `TestPostgresMLLPRuntime_CertificateIdentityAuthorization` passed
    with `-race` against PostgreSQL 16 over real TLS 1.3 mutual authentication,
    together with the existing `TestPostgresMLLPRuntime_DurableACKPauseRestart`.
  - Three independent negative controls each failed the kill test, proving each
    assertion is load-bearing:
    1. Silent fallback to the first configured identity for an unmatched
       certificate → "unmapped CA-valid certificate received an MLLP
       acknowledgement".
    2. Ignoring per-identity grants and always projecting the server-issued
       submit grant → sender B's projected grants were wrong, and with that
       assertion relaxed the ungranted identity returned `AA` instead of `AE`.
    3. Using the deployment-fixed principal in mapped mode → admitted subjects
       collapsed to `[mllp-listener mllp-listener]`.
  - Confirmed the checked-in golden MLLP source revision still decodes to its
    exact pinned digest `sha256:1d9517bc…6399`, so no existing deployment is
    invalidated.
  - `go test -race ./...` and the focused
    `./internal/integration/... ./cmd/fi-fhir/...` race suites passed.
  - `gofmt`, `golangci-lint run` (0 issues), `go vet ./...`,
    `go vet -tags=integration ./internal/integration/...`, `make docs-validate`,
    `bash scripts/check-runtime-config.sh`, `go mod verify`, and
    `git diff --check` passed.
  - `govulncheck ./...` reported no vulnerabilities in called code. `gosec` with
    the exact CI exclusion set reported 0 issues across 261 files.
  - CI evidence for the merge request and the exact post-merge main pipeline is
    harvested after landing.

## What Is Still Open

- Remaining acceptance criteria: required MR and exact main pipelines must reach
  terminal green before the RALPH task closes.
- Known issues: no implementation defect is known. Certificate revocation
  transport (CRL/OCSP) is deliberately out of scope; revoking a sender today
  means publishing a new source revision without its entry and redeploying
  through the lifecycle. Local Golden Path 001 was not run because no local
  Docker daemon is available; its required CI job is authoritative. Checked-in
  GitOps activation remains intentionally unchanged.
- Dependencies: client certificates must carry a stable, authority-scoped URI
  SAN (or a pinned SPKI). The client CA bundle should be scoped to the MLLP
  trust domain; a shared corporate CA leaves the identity map as the only
  remaining boundary.
- Operational consequence: because the identity map is part of the
  content-addressed source revision, adding or removing a sender requires a new
  source revision, a new integration definition revision, and a lifecycle
  redeploy. That is deliberate (tamper-evident, digest-pinned) but it is not a
  hot-reload path.

## Next Actions

1. 4.1b3: bind batch connector/workload identity and replace remote object
   modification time as trusted receipt provenance.
2. 4.1c: add destination-scoped identity and secret resolution for the first
   durable HTTPS consumer, then extend policy to control-plane and PHI/audit
   operations.
3. Consider a follow-up slice for certificate revocation and short-lived SVID
   rotation once a trust-bundle source exists; today rotation is a lifecycle
   redeploy.

## Context Links

- Relevant docs/specs:
  - `.loom/iteration-plan-phase-4-slice-4-1b2-mllp-certificate-identity.md`
  - `.loom/30-implementation-plan-integration-engine-ide-completion.md`
  - `.loom/slice-handoff-phase-4-slice-4-1b1-oauth-http-submit-authorization.md`
  - `.loom/40-decisions.md`
  - `docs/operations/PRODUCTION-MLLP.md`
  - `docs/operations/PRODUCTION-HARDENING.md`
