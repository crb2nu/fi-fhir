### 2026-08-07: Bind OAuth Service Tokens to One Submit Action and Exact Source

- Decision:
  - Add an `oauth2` production HTTP ingress mode that reuses the bounded OIDC
    discovery/JWKS verifier and projects one distinct service principal per
    allowlisted client. Require protected `typ=at+jwt`, the exact issuer and
    audience, valid time claims, the deployment tenant, canonical
    `sub == client_id`, and the signed `integration:submit` grant.
  - Treat the allowlist as a deployment-owned client binding. Project only the
    required submit role, then bind `SourceID` from the immutable integration
    registry after authentication; never accept tenant, principal, source,
    role, action, or revision identity from request headers.
  - Introduce one server-constructed `integration.submit` authorization request
    over the exact tenant, integration revision, and source. Enforce it at the
    adapter, shared processor, and transaction-scoped admission boundaries.
  - Map the existing HTTP, MLLP, and batch role strings to this action so the
    new decision is fail-closed without changing persisted channel provenance.
- Rationale:
  - Static bearer/HMAC credentials collapse all holders into one deployment
    principal. A verified per-request client subject is necessary for durable
    attribution, but signature verification alone does not authorize a client
    for a concrete production source.
  - Server-owned action and object identity prevents a valid token or spoofed
    header from expanding authority across tenants, revisions, or sources.
  - Rechecking before artifact loading and inside durable admission contains
    in-process callers that bypass the HTTP adapter.
- Alternatives considered:
  - Trust every signed client in the issuer (rejected because issuer membership
    is broader than deployment authorization).
  - Persist all signed token roles (rejected because unrelated claims could
    silently become future authority).
  - Replace bearer/HMAC, MLLP, batch, GraphQL, delivery, audit, and PHI controls
    in one slice (deferred because each has a different identity boundary and
    kill-test).
- Consequences:
  - The authorization server must issue the required JWT access-token profile;
    OAuth client credentials with opaque tokens is not supported in this slice.
  - `sub` and `client_id` must identify the same allowlisted confidential client.
    The immutable registry remains authoritative for revision and source.
  - Existing MLLP and batch identities remain deployment-configured pending
    certificate/workload binding slices, but now pass the same submit decision.
- Sources:
  - [S1] `internal/api/requestsecurity/oidc.go`
  - [S2] `internal/integration/authorization/policy.go`
  - [S3] `internal/integration/ingress/`
  - [S4] `.loom/iteration-plan-phase-4-slice-4-1b1-oauth-http-submit-authorization.md`
