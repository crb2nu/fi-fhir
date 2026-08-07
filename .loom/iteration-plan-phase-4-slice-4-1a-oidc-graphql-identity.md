# RALPH Iteration Plan — Phase 4 Slice 4.1a OIDC GraphQL Identity

## Review

- Roadmap milestone: Phase 4 Slice 4.1 — enforce identity, authorization, and
  PHI policy.
- Spec sections: `.loom/20-product-spec-integration-engine-ide-completion.md`
  identity, tenancy, authorization, audit, and PHI boundaries; `.loom/30-
  implementation-plan-integration-engine-ide-completion.md` Phase 4 Slice 4.1.
- Prior decisions to preserve:
  - Server-owned tenant and principal identity; never accept caller identity in
    request bodies or resolver arguments.
  - Exact-origin GraphQL policy and the bounded authenticated POST/SSE surface.
  - `/graphql/ws` remains closed until a bounded pre-authentication transport is
    available.
  - Static bearer authentication remains a compatibility path for local and
    preview deployments, not a production identity model.

## Align

- Slice name: OIDC-authenticated GraphQL security context.
- Scope in:
  - Discover one configured OIDC issuer and verify bearer JWTs against its
    cached JWKS using an explicit signing-algorithm allowlist.
  - Accept only the signed JWT access-token class identified by protected
    `typ=at+jwt`; require HTTPS discovery/JWKS, reject redirects, and bound remote
    request time, response size, and unknown-key refresh frequency.
  - Validate signature, issuer, audience, expiry, not-before, subject, exact
    deployment tenant, and a strict role list.
  - Project verified `sub`, tenant, and roles into the existing immutable
    `integration.SecurityContext` as a human principal.
  - Select `static` or `oidc` authentication explicitly at runtime. OIDC mode
    rejects every static principal, role, bearer, and trusted-CIDR bypass knob.
  - Apply the verifier through the existing GraphQL POST and SSE middleware;
    keep the existing operation authorization boundary.
- Scope out:
  - Browser authorization-code/PKCE login, refresh tokens, logout, and token
    storage.
  - OAuth service identity for REST ingress, MLLP, batch, actions, or delivery.
  - Fine-grained policy administration, durable audit storage, token
    administration, PHI retention administration, and export controls.
  - GraphQL WebSocket enablement and production GitOps activation.
- Acceptance criteria:
  - A valid token for the deployment tenant with `integration:preview` or
    `graphql:operator` reaches an allowed GraphQL POST/SSE operation with the
    token subject as the request principal.
  - Wrong issuer, audience, algorithm, signature, key, time window, subject,
    token class, tenant, or role shape fails closed without exposing verifier or
    token details.
  - A valid cross-tenant token is rejected during authentication, and a valid
    same-tenant token without an allowed role is rejected before resolver data
    is returned.
  - An unknown key ID triggers the verifier's bounded JWKS refresh path and a
    rotated valid key can authenticate without rebuilding the server.
  - Existing static-mode behavior and tests remain green.
- Dependencies/blockers:
  - The issuer must expose standards-compliant OIDC discovery and JWKS
    endpoints over HTTPS.
  - The existing `requestsecurity.Authenticator` and
    `integration.SecurityContext` seam must preserve request-specific identity
    through GraphQL authorization.
- Riskiest load-bearing assumption:
  - The current authenticator/security-context seam carries the verified caller
    identity far enough into operation authorization without resolver or schema
    redesign.
- Kill-test:
  - Exercise the real GraphQL handler with signed tokens from one test issuer.
    The expected tenant plus allowed role succeeds; cross-tenant, wrong
    audience, expired/not-yet-valid, unknown-key, and missing-role cases return
    no resolver data. If deployment-owned identity appears after successful
    verification, stop and repair context propagation before expanding scope.

## Land

- Planned file areas:
  - `internal/api/requestsecurity/`
  - `internal/api/graphql/`
  - `cmd/fi-fhir/preview_runtime.go` and focused tests
  - module dependency metadata
  - operator/runtime documentation and canonical Loom plan records
- Implementation steps:
  1. Add and adversarially test a long-lived OIDC/JWKS authenticator.
  2. Add an explicit, mutually exclusive runtime authentication mode and wire
     OIDC configuration into the GraphQL server.
  3. Prove caller identity, tenant isolation, operation authorization, and key
     rotation through the real handler, then reconcile documentation.

## Prove

- Tests to run:
  - `go test -race ./internal/api/requestsecurity ./internal/api/graphql ./cmd/fi-fhir`
  - `go test -race ./...`
- Lint/static checks:
  - `gofmt` on changed Go files
  - `go vet ./...`
  - `git diff --check`
  - repository contract/security gates selected by the Makefile and CI
- CI checks:
  - Required merge-request pipeline reaches a terminal green state.
  - Auto-merge is armed only after the scoped self-review is clean.
  - The post-merge main pipeline is harvested when available.

## Handoff/Harvest

- Docs to update:
  - Canonical completion execution plan and decision log.
  - GraphQL/runtime operator configuration.
  - Phase 4 Slice 4.1a handoff with exact local and CI evidence.
- Agent-context entries to add:
  - OIDC claim/configuration decision and security invariants.
  - Kill-test and terminal validation evidence.
  - MR, pipeline, merge, and post-merge refs.
- Next-slice candidates:
  - Phase 4 Slice 4.1b: OAuth service identities and uniform authorization
    policy across REST, MLLP, batch, actions, and delivery.
  - Phase 4 Slice 4.1c: immutable security audit plus durable PHI retention,
    expiry, encryption, and export controls.
