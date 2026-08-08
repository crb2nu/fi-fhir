# RALPH Iteration Plan — Phase 4 Slice 4.1b2 MLLP Certificate Service Identity

## Review

- Roadmap milestone: Phase 4 Slice 4.1 — enforce identity, authorization, and
  PHI policy.
- Spec sections: `.loom/20-product-spec-integration-engine-ide-completion.md`
  identity and isolation contracts; `.loom/30-implementation-plan-integration-
  engine-ide-completion.md` Phase 4 Slice 4.1.
- Prior decisions to preserve:
  - Tenant, integration revision, source, action, and resource identity are
    server-owned. Senders cannot assert them through headers, MSH fields, or
    any other in-band data.
  - Slice 4.1b1 added exactly one fail-closed `integration.submit` decision over
    exact tenant, revision, and source. New transports reuse it rather than
    forking a parallel policy path.
  - The MLLP source revision is immutable and content-addressed; the deployed
    lifecycle release pins its exact digest.
  - Durable admission is the last authorization boundary before receipt, event,
    attempt, and outbox writes.
  - Checked-in GitOps activation remains a separate reviewed operation.

## Align

- Slice name: verified MLLP client-certificate service identity mapped to the
  immutable source revision, plus the shared submit authorization decision.
- Scope in:
  - Add `clients.identities` to the immutable MLLP source revision: an explicit
    allowlist mapping a certificate URI SAN and/or an SPKI SHA-256 pin to one
    canonical service subject and its grants.
  - Derive one verified `ConnectionIdentity` per accepted TLS connection from
    the peer leaf certificate, immediately after the handshake and before any
    frame is read, parsed, processed, or admitted.
  - Reject a CA-valid certificate that maps to zero entries or to more than one
    entry. Rejection closes the connection with no acknowledgement.
  - Carry the verified subject, auth method, and grants per connection into the
    same `authorization.AuthorizeSubmission` decision the HTTP ingress uses, and
    evaluate it at the adapter boundary before capacity, envelope construction,
    processor artifact loading, and transaction-scoped runnable admission.
  - Keep identity mapping all-or-nothing per listener. When the source revision
    declares no identities the adapter keeps the current deployment-fixed
    principal and server-issued `integration:mllp` grant. When it declares
    identities, an unmapped connection cannot silently fall back.
  - Require mutual TLS whenever identities are declared, and add
    `FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY` so a deployment can refuse to start in
    compatibility mode.
  - Forward the complete MLLP runtime contract through Docker Compose and extend
    the runtime-config regression check to cover MLLP variables.
- Scope out:
  - Batch/S3/SFTP connector or cloud workload identity (Slice 4.1b3).
  - Destination-scoped identity, secret resolution, and delivery authorization.
  - GraphQL control-plane actions, token issuance/introspection, SPIFFE trust
    bundle rotation, and certificate revocation transport (CRL/OCSP).
  - Immutable security audit storage, PHI retention/TTL/export controls,
    WebSocket enablement, and GitOps activation.
- Acceptance criteria:
  - Two distinct allowlisted client certificates traverse the same real TLS
    listener as two distinct verified service subjects observable at the
    authorization decision.
  - Spoofed in-band provenance (MSH sending application/facility) cannot make
    one certificate present as another identity.
  - A CA-valid certificate absent from the identity map is rejected before
    artifact loading and before any durable record exists.
  - An allowlisted identity without a recognized submit grant stops before
    artifact loading or durability for the exact tenant, revision, and source.
  - Compatibility mode without an identity map preserves current behavior for
    existing fixtures, source documents, and digests.
- Dependencies/blockers:
  - Client certificates must carry a stable URI SAN (or a pinned SPKI) that the
    issuing authority controls. `CN`-only certificates are not accepted as
    identity because common names are not authority-scoped.
  - The identity map lives inside the content-addressed source revision, so
    changing it requires a new source revision, definition revision, and
    lifecycle redeploy.

## Riskiest assumption + kill-test

**Load-bearing assumption**: The MLLP adapter can replace one deployment-fixed
principal with a per-connection certificate-derived service principal without
(a) letting in-band message content select or influence identity, (b) letting a
CA-valid but unmapped certificate reach frame processing or durability, and
(c) changing the digest of existing source revisions that declare no identities.

**Kill test**: `TestPostgresMLLPRuntime_CertificateIdentityAuthorization` drives
the real MLLP listener over real TLS 1.3 mutual authentication against
PostgreSQL 16, with the production durable processor and transaction-scoped
runnable admission. It asserts:

1. Two allowlisted certificates produce two distinct subjects captured at the
   authorization decision, even when the second sender's MSH-3/MSH-4/MSH-10
   fields impersonate the first sender.
2. A CA-valid certificate whose URI SAN and SPKI are absent from the map
   completes the handshake but is closed with no acknowledgement, no processor
   invocation, and no change in any durable record class.
3. An allowlisted identity whose grants omit a recognized submit grant is denied
   for the exact tenant/revision/source, with no artifact load and no durable
   record.
4. Compatibility mode (no `clients.identities`) still admits the existing fixture
   under the deployment-fixed principal and server-issued grant.

**Failure mode if the assumption is wrong**: fi-fhir would verify certificates
but still persist one shared identity, or it would accept sender-asserted
provenance. Extending identity to batch and destination transports would then
replicate a broken trust boundary across every remaining adapter.

**Status**: passed on 2026-08-08. The real TLS listener kept `svc-sender-a` and
`svc-sender-b` distinct at transaction-scoped admission while the second sender's
MSH-3/MSH-4 impersonated the first. A CA-valid certificate for an unmapped URI
SAN completed the handshake, received no acknowledgement, loaded no artifacts,
and left every durable record class unchanged. A mapped identity holding only a
non-submit grant returned `AE` with no artifact load and no durable record. The
compatibility listener admitted the same rejected certificate under
`mllp-listener`/`mllp-mtls` with the server-issued grant. Three negative
controls — silent fallback for unmatched certificates, ignored per-identity
grants, and a deployment-fixed principal in mapped mode — each failed the test,
and the checked-in golden source revision retained its exact pinned digest.

Positive evidence: RFC 5280 §4.2.1.6 defines `uniformResourceIdentifier`
subject alternative names as authority-scoped identifiers:
<https://www.rfc-editor.org/rfc/rfc5280.html#section-4.2.1.6>. SPIFFE X.509-SVID
requires exactly one URI SAN as the document identity:
<https://github.com/spiffe/spiffe/blob/main/standards/X509-SVID.md>.
Disconfirming evidence: RFC 6125 §6.4.4 deprecates common-name-based identity
matching, so `CN` is deliberately excluded here even though many existing HL7v2
deployments key on it: <https://www.rfc-editor.org/rfc/rfc6125.html#section-6.4.4>.

## Land

- Planned file areas:
  - `internal/integration/mllp/identity.go` (new mapping and matching)
  - `internal/integration/mllp/source.go` (immutable identity allowlist)
  - `internal/integration/mllp/server.go` (post-handshake identity derivation)
  - `internal/integration/mllp/service.go` (per-connection submit decision)
  - `cmd/fi-fhir/preview_runtime.go`, `cmd/fi-fhir/main.go` (runtime contract)
  - `docker-compose.yaml`, `.env.example`, `scripts/check-runtime-config.sh`
  - `docs/operations/PRODUCTION-MLLP.md`, `docs/operations/PRODUCTION-HARDENING.md`
  - `Makefile` and the required `test:mllp-runtime` discovery list
- Implementation steps:
  1. Add the immutable identity allowlist and its fail-closed validation, keeping
     existing revision digests unchanged when no identities are declared.
  2. Resolve one unambiguous identity per connection after the handshake and
     close unmapped or ambiguous connections before any frame read.
  3. Carry the identity into the shared submit decision, evaluated immediately
     after binding validation and before every side effect.
  4. Wire runtime configuration, Compose, and the runtime-config check, then run
     the kill test and reconcile documentation.

## Prove

- Tests to run:
  - `go test -race ./internal/integration/mllp/... ./internal/integration/authorization/... ./cmd/fi-fhir/...`
  - `go test -race ./...`
  - `POSTGRES_TEST_URL=... make mllp-runtime`
- Lint/static checks:
  - `gofmt` on changed Go files
  - `golangci-lint run`
  - `go vet ./...`
  - `make docs-validate`, `bash scripts/check-runtime-config.sh`
  - `go mod verify`, `git diff --check`
- CI checks:
  - Required merge-request pipeline reaches terminal green, including
    `test:mllp-runtime`, security, benchmark, and Golden Path gates.
  - Auto-merge armed after self-review; post-merge main pipeline harvested.

## Handoff/Harvest

- Docs to update:
  - Phase 4 execution plan (4.1b2 subsection and 4.1b1 landing evidence).
  - `docs/operations/PRODUCTION-MLLP.md` and `PRODUCTION-HARDENING.md`.
  - `.loom/50-worklog.md` and the Slice 4.1b2 handoff.
- Agent-context entries to add:
  - Certificate identity binding decision and its immutability consequence.
  - Kill-test evidence and CI/merge references.
- Next-slice candidates:
  - 4.1b3: bind batch connector/workload identity and replace remote object
    modification time as trusted receipt provenance.
  - 4.1c: destination-scoped identity and secret resolution for the first
    durable HTTPS consumer, followed by audit and PHI controls.
