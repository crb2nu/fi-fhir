### 2026-08-06: Verify GraphQL Callers Through One Exact OIDC Trust Domain

- Decision:
  - Add one long-lived OIDC discovery/JWKS verifier behind the existing
    `requestsecurity.Authenticator` boundary and reuse the established GraphQL
    POST/SSE security-context propagation and operation authorization.
  - Accept only an exact HTTPS issuer, one exact API audience, explicitly
    allowed asymmetric signing algorithms, a protected `typ=at+jwt` access-token
    class, valid expiry/not-before, a nonempty subject, the exact deployment
    tenant claim, and a strict nonempty role array. Map `sub` to a human
    principal with `auth_method=oidc`.
  - Validate discovered JWKS metadata and every outbound request as HTTPS; reject
    redirects; cap network duration/response size and rate-bound outbound
    unknown-key refresh.
  - Make `static` and `oidc` runtime modes mutually exclusive. OIDC rejects the
    compatibility bearer, deployment principal/roles, and trusted-CIDR bypass;
    static mode rejects every OIDC-only setting.
- Rationale:
  - A deployment-owned static principal cannot distinguish human callers or
    support key rotation, while accepting caller-supplied identity without
    signature and tenant proof would break the product's foundational isolation
    contract.
  - Reusing the current authenticator and immutable security context avoids a
    second authorization path and lets the actual GraphQL handler prove that
    verified roles reach the pre-resolver operation boundary.
  - Exact single-audience validation prevents a token issued jointly to another
    relying party from gaining API access merely because it also names fi-fhir.
  - The protected access-token type prevents an OIDC ID token from being
    substituted at the API boundary; this is deliberately narrower than a claim
    of complete RFC 9068 conformance.
- Alternatives considered:
  - Keep rotating a shared preview bearer (rejected because it still collapses
    all human callers into one deployment identity).
  - Parse JWTs without OIDC discovery/JWKS verification (rejected because it
    would duplicate sensitive crypto/key-rotation behavior).
  - Add browser login, service identity, full RBAC, audit, and PHI retention in
    one MR (deferred because those are independent Phase 4 policy and lifecycle
    slices with different kill-tests).
- Consequences:
  - OIDC startup requires reachable standards-compliant HTTPS discovery and
    JWKS endpoints. Remote calls time out after at most 10 seconds, JWKS bodies
    are capped at 1 MiB, and outbound unknown-key refreshes have a 30-second
    default floor; providers should publish keys before issuing rotated tokens.
  - Roles remain coarse compatibility roles until the next authorization-policy
    slice, and checked-in GitOps manifests remain static pending reviewed
    activation.
  - Verification failures expose only the existing generic credential error;
    startup configuration/discovery failures remain operator-visible.
- Sources:
  - [S1] `internal/api/requestsecurity/oidc.go`
  - [S2] `internal/api/graphql/oidc_security_test.go`
  - [S3] `cmd/fi-fhir/preview_runtime.go`
  - [S4] `.loom/iteration-plan-phase-4-slice-4-1a-oidc-graphql-identity.md`
