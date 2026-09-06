### 2026-09-05 - Cloudflare Access identity for the GraphQL IDE

- What changed:
  - New `requestsecurity.CloudflareAccessAuthenticator`: verifies the token
    Cloudflare Access attaches to requests (`Cf-Access-Jwt-Assertion`, or the
    `CF_Authorization` cookie) against the team domain's keys and exact
    application audience, requires `type=app` and an `email`, and maps that
    email to deployment-configured roles. Shares the OIDC discovery and JWKS
    machinery through a small extraction (`discoverOIDCVerifier`).
  - `serve` reads `FI_FHIR_GRAPHQL_ACCESS_TEAM_DOMAIN`, `_AUDIENCE`,
    `_PRINCIPALS` (all or none) in either auth mode and wires the layer into the
    GraphQL middleware behind LAN trust and any `Authorization` header.
  - `/api/auth/status` reports `authVia: "cloudflare-access"` with the
    principal; the IDE gate treats it like LAN trust and shows who is signed in.
  - Docs (`PRODUCTION-HARDENING.md`, `GRAPHQL-API.md`, `cli-reference.md`,
    `.env.example`), CHANGELOG, and a decision entry.
- Why:
  - The deployed IDE is behind Cloudflare Access with Google sign-in, but off-LAN
    the browser still had to paste a bearer token — and the only token available
    carried `integration:preview`, not the operator role. The Access token could
    not be consumed by the existing OIDC mode (`typ=at+jwt`, tenant and roles
    claims required; Access issues none of those).
- Evidence:
  - `go test ./internal/api/requestsecurity/... ./internal/api/graphql/... ./cmd/fi-fhir/ -run 'CloudflareAccess|GraphQLAuthentication'`
    green: verified-email mapping, unmapped identity rejected, other
    application's audience rejected, multi-audience rejected, service and meta
    tokens rejected, `at+jwt` class rejected, expired rejected, header wins over
    cookie, bearer header wins over assertion, partial env rejected.
  - UI: the gate test covers the `cloudflare-access` status and an unknown
    headerless mode staying closed.
- What's next:
  - GitOps: add the three variables and the operator email to
    `k3s/fi-fhir/fi-fhir-api.yaml`; older images ignore them, so it can land
    before the image rolls. Also widen the static roles so LAN and token access
    match the Access grant on this single-operator deployment.
- Sources:
  - [S1] `.loom/decisions/2026-09-05-accept-cloudflare-access-identity-for-graphql-callers.md`
  - [S2] `https://fi-fhir.flexinfer.ai/.well-known/cloudflare-access-protected-resource/` — team domain and authorization server
