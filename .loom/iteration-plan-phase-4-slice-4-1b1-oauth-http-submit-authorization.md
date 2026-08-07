# RALPH Iteration Plan — Phase 4 Slice 4.1b1 OAuth HTTP Service Identity

## Review

- Roadmap milestone: Phase 4 Slice 4.1 — enforce identity, authorization, and
  PHI policy.
- Spec sections: `.loom/20-product-spec-integration-engine-ide-completion.md`
  identity and isolation contracts; `.loom/30-implementation-plan-integration-
  engine-ide-completion.md` Phase 4 Slice 4.1.
- Prior decisions to preserve:
  - Tenant, integration revision, source, action, and resource identity are
    server-owned. Requests cannot assert them through headers or bodies.
  - Human GraphQL OIDC remains unchanged and continues to use the bounded
    `typ=at+jwt` verifier from Slice 4.1a.
  - Static HTTP bearer/HMAC authentication remains an explicit compatibility
    mode. Checked-in GitOps activation remains a separate reviewed operation.
  - Durable admission is the last authorization boundary before receipt,
    event, attempt, and outbox writes.

## Align

- Slice name: OAuth client-credentials identity for production HTTP ingress
  plus one reusable production `submit` authorization decision.
- Scope in:
  - Add `FI_FHIR_HTTP_INGRESS_AUTH_MODE=oauth2` for signed JWT access tokens
    obtained by confidential clients outside fi-fhir.
  - Reuse the existing bounded HTTPS discovery/JWKS verifier. Require exact
    issuer and single audience, protected `typ=at+jwt`, supported asymmetric
    signature, valid expiry/not-before, exact deployment tenant, canonical
    `sub` and `client_id`, and `sub == client_id`.
  - Require the client ID to be in a deployment-owned allowlist and require a
    signed `integration:submit` role. Project only the recognized submit grant;
    extra token roles cannot expand authority.
  - Return a distinct per-request service `SecurityContext` from the ingress
    authenticator. Resolve and bind `SourceID` only from the immutable registry.
  - Add one server-constructed authorization request for action `submit` on an
    exact integration revision and source. Map the existing HTTP, MLLP, and
    batch submit grants to that action without changing their persisted names.
  - Enforce the decision at the adapter boundary, in the shared processor before
    artifact loading, and again in transaction-scoped runnable admission.
  - Make OAuth and static HTTP settings mutually exclusive while preserving
    existing bearer/HMAC behavior.
- Scope out:
  - Token issuance, token-endpoint calls, opaque-token introspection, refresh
    tokens, browser login, and IdP provisioning.
  - MLLP certificate-to-principal/SPIFFE mapping and CIDR-only attribution.
  - S3/SFTP uploader or cloud-workload identity and batch timestamp provenance.
  - GraphQL control-plane actions, lifecycle administration, workflow actions,
    destination consumers, delivery/replay/export authorization, or credential
    forwarding.
  - Immutable security audit storage, PHI retention/TTL/encryption/export
    controls, Helm/Kustomize/Flux activation, and live rollback proof.
- Acceptance criteria:
  - Two allowed, validly signed OAuth clients traverse the same real HTTP handler
    as two distinct service principals; both receive registry-owned source
    identity and only the recognized submit grant.
  - Wrong tenant, audience, time window, algorithm, key, role shape, missing
    submit grant, unlisted client, or `sub != client_id` fails closed with no
    credential, claim, object-existence, or PHI disclosure.
  - Headers attempting to spoof tenant, principal, source, roles, or auth method
    cannot change the authenticated security context.
  - Wrong integration remains catalog-safe and never reaches the processor.
  - A shape-valid production request without an allowed submit grant is denied
    after exact definition resolution but before artifact loading or durable
    writes.
  - Existing static HTTP, MLLP, batch, human OIDC GraphQL, and Golden Path 001
    behavior remains green.
- Dependencies/blockers:
  - The authorization server must expose compatible HTTPS discovery/JWKS and
    issue the explicitly required JWT access-token profile. OAuth 2.0 client
    credentials alone does not mandate JWT access tokens.
  - The current Handler/Service seam must carry verified request identity while
    keeping source and integration ownership on the server.

## Riskiest assumption + kill-test

**Load-bearing assumption**: The current HTTP Handler/Service split can be
changed from one deployment-fixed principal to a per-request verified service
principal without letting request data override tenant, source, roles, or the
exact integration binding.

**Kill test**: Drive the actual `POST /v1/hl7v2` handler with a TLS discovery/
JWKS fixture. Submit valid tokens for two allowlisted clients, each with spoofed
tenant/principal/source/role headers, and assert the processor captures the two
token client IDs, deployment tenant, immutable registry source, service kind,
`oauth2-client-credentials`, and only the recognized submit grant. Then send
signed cross-tenant, missing-role, extra-role-only, unlisted-client,
`sub != client_id`, wrong-audience, expired, unknown-key, static-token, and wrong-
integration cases. Denied cases must not advance registry/processor/durable
counters except that a wrong integration may authenticate and perform only the
catalog-safe binding lookup. Separately send a syntactically valid production
request with no recognized submit grant directly to the shared processor and
assert the definition may resolve but artifact and database hooks remain zero.

**Failure mode if the assumption is wrong**: fi-fhir would verify JWTs but still
persist one deployment identity, or it would authorize caller-spoofed provenance.
Expanding the policy to MLLP, batch, GraphQL, or delivery would then replicate a
broken trust boundary.

**Status**: passed on 2026-08-07. The real TLS handler test preserved two
distinct allowlisted client subjects and the registry-owned source while
discarding spoofed identity headers and extra token roles. Cross-tenant,
wrong-audience, missing-grant, unlisted/mismatched-client, and static-token
attempts stopped before the registry or processor. The direct no-grant
production processor test resolved the definition but never loaded artifacts or
entered durability. Focused and full-repository race suites passed.

Positive evidence: RFC 9068 defines signed JWT access-token validation, requires
`client_id`, and says `sub` should identify the client application for client-
credentials grants: <https://www.rfc-editor.org/rfc/rfc9068.html>. Disconfirming
evidence: RFC 6749 defines the client-credentials grant but leaves the access-
token representation open, so this slice makes JWT-profile support an explicit
provider prerequisite rather than claiming universal OAuth compatibility:
<https://www.rfc-editor.org/rfc/rfc6749.html#section-4.4>.

## Land

- Planned file areas:
  - `internal/api/requestsecurity/`
  - `internal/integration/authorization/`
  - `internal/integration/ingress/`
  - `internal/integration/processor/` and `internal/integration/lifecycle/`
  - focused MLLP/batch compatibility tests
  - `cmd/fi-fhir/` runtime configuration
  - operator/runtime documentation and canonical Loom records
- Implementation steps:
  1. Factor the existing OIDC verification result and add a constrained service
     projection with allowlisted `sub == client_id` identities.
  2. Carry request-specific identity through HTTP Handler/Service and bind the
     immutable source after registry resolution.
  3. Add and enforce the server-owned submit action/resource decision before
     artifacts and durable side effects, retaining current channel grants.
  4. Run the real-handler and processor kill-tests, then reconcile docs.

## Prove

- Tests to run:
  - `go test -race ./internal/api/requestsecurity/... ./internal/integration/authorization/... ./internal/integration/ingress/... ./internal/integration/processor/... ./internal/integration/lifecycle/... ./internal/integration/mllp/... ./internal/integration/batch/... ./cmd/fi-fhir/...`
  - `go test -race ./...`
  - `make golden-path-001` when its PostgreSQL/Compose prerequisites are
    available; required CI job remains authoritative otherwise.
- Lint/static checks:
  - `gofmt` on changed Go files
  - `make lint`
  - `go vet ./...`
  - `make docs-validate`
  - `go mod verify`, `go mod tidy -diff`, `git diff --check`
  - `govulncheck ./...` and focused gosec on changed security packages
- CI checks:
  - Required merge-request pipeline reaches terminal green, including security,
    integration, benchmark, and Golden Path gates selected by CI.
  - Auto-merge is armed only after independent self-review is clean.
  - The exact post-merge main pipeline is harvested to terminal green.

## Handoff/Harvest

- Docs to update:
  - Canonical Phase 4 execution plan and decision log.
  - HTTP ingress runtime/operator configuration and CLI environment help.
  - Phase 4 Slice 4.1b1 handoff with exact local and CI evidence.
- Agent-context entries to add:
  - Service-token claim/binding decision and provider limitation.
  - Submit action/resource policy and kill-test evidence.
  - MR, pipeline, merge, and post-merge refs.
- Next-slice candidates:
  - 4.1b2: map verified MLLP certificate URI SAN/SPKI identity to immutable
    source/principal and reject CA-valid unmapped certificates.
  - 4.1b3: bind batch connector/workload identity and correct remote-mtime
    provenance.
  - 4.1c: destination-scoped OAuth identity/secret resolution for the first
    durable HTTPS consumer, followed by audit and PHI controls.
