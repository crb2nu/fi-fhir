### 2026-09-05: Accept the Cloudflare Access identity for GraphQL callers, as a layer beside the bearer modes

- Decision:
  - When `FI_FHIR_GRAPHQL_ACCESS_TEAM_DOMAIN`, `_AUDIENCE`, and `_PRINCIPALS` are
    all set, `serve` verifies the application token Cloudflare Access attaches to
    each request (`Cf-Access-Jwt-Assertion`, or the browser's `CF_Authorization`
    cookie) against the team domain's published keys and the exact application
    AUD tag, requires `type=app` and an `email`, and grants that address exactly
    the roles the `_PRINCIPALS` map names. Nothing else in the token is trusted.
  - This is a layer available in both `static` and `oidc` modes, not a third
    `FI_FHIR_GRAPHQL_AUTH_MODE`. Precedence is fixed: LAN trust, then an
    `Authorization` header when present (judged alone), then the assertion.
  - `/api/auth/status` reports `authVia: "cloudflare-access"` plus the principal,
    and the IDE's credential gate steps aside on it as it does for LAN trust.
- Rationale:
  - The deployed IDE at `fi-fhir.flexinfer.ai` already sits behind Cloudflare
    Access with Google as the identity provider, yet off-LAN the browser was met
    by "Enter access token" — a second login for an identity the edge had just
    verified — and the only token to paste carried `integration:preview` alone.
  - The existing OIDC mode cannot consume the Access token: it requires
    `typ=at+jwt`, a `tenant_id` claim and a `roles` claim, and Access issues
    `typ=JWT` with neither. Access application tokens carry no authorization
    at all, so the mapping from identity to roles has to be deployment
    configuration; putting it in the map (rather than trusting any admitted
    identity) keeps Access's policy — edited outside this repository — from
    silently widening what a login can do here.
  - Making the header a *fallback* behind an explicit `Authorization` header
    means a stale or wrong bearer is never rescued by a cookie beside it, and
    machine callers (CLI, CI, service tokens) keep the bearer path unchanged.
- Alternatives considered:
  - Broaden the static token to `graphql:operator` and hand it out. Rejected as
    the fix (it is a fine stop-gap): one shared secret for every human, no
    per-person identity in the audit trail, and a second login on every visit.
  - Switch the deployment to `FI_FHIR_GRAPHQL_AUTH_MODE=oidc` pointed at the
    team domain. Rejected: the token class and claims do not match, and OIDC
    mode forbids the LAN trust and static token the CLI and on-LAN use rely on.
  - Trust the Cloudflare edge by network (put the tunnel in
    `FI_FHIR_GRAPHQL_TRUSTED_CIDRS`). Rejected: that trusts reachability, not
    identity, and every request through the tunnel would become the single
    static principal.
  - Grant a default role set to any Access-admitted identity. Rejected in favour
    of the explicit map for the reason above; the map is one line per person.
- Consequences:
  - The GitOps deployment gains three environment variables and the operator's
    email; older images ignore unknown `FI_FHIR_GRAPHQL_ACCESS_*` names, so the
    config can land before the image rolls.
  - Verification cost is one JWKS fetch at startup plus rate-limited refreshes,
    the same bound as OIDC mode; per request it is a signature check.
  - A person removed from the Access policy loses access at the edge; a person
    removed from the map loses it at the origin. Both must be edited to add
    someone.
- Sources:
  - [S1] `internal/api/requestsecurity/cloudflare_access.go`
  - [S2] `docs/operations/PRODUCTION-HARDENING.md` — "Cloudflare Access in front of the deployment"
  - [S3] Cloudflare Access application token claims: `iss`, `aud`, `email`, `sub`, `type`, `iat`, `exp`, `identity_nonce`, `country`
  - [S4] `.loom/decisions/2026-08-06-verify-graphql-callers-through-one-exact-oidc.md` — the exact-audience discipline this reuses
