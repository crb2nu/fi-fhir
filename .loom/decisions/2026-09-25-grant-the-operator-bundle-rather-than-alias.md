### 2026-09-25: Grant the operator bundle rather than alias the transport grant

- Decision:
  - **`graphql:operator` does not imply the operator control plane's roles,
    and no code path will make it.** It stays the transport gate's named
    compatibility grant (Lane S4-E); `integration.operator`,
    `integration.delivery.operator` and `integration.deployment.operator` stay
    the roles `operator.Service.authorize` re-checks (Slice 4.2a).
  - **Deployments grant the documented bundle** to every operator identity:
    `integration:preview,graphql:operator,clinical:read,integration.operator,integration.delivery.operator,integration.deployment.operator`
    — in `FI_FHIR_GRAPHQL_ROLES` (the static bearer, and the trusted network,
    which inherits it) and in each operator's
    `FI_FHIR_GRAPHQL_ACCESS_PRINCIPALS` entry. `.env.example`,
    `docker-compose.yaml` and `scripts/check-runtime-config.sh` now require it
    for the local IDE.
  - **`/api/auth/status` exposes the caller's principal, roles, derived
    capabilities, and missing roles** (IDE repair Lane R-A,
    `internal/api/graphql/capabilities.go`), and `serve` logs one WARN per
    configured identity that holds the grant without `integration.operator`.
- Rationale:
  - From 2026-09-05 to 2026-09-25 production granted
    `integration:preview,graphql:operator,clinical:read` to the static bearer,
    the trusted network, and both Access principals. The gate admitted every
    operator query ("admitted through the compatibility grant") and the service
    refused every one (`operator control-plane action forbidden`); the status
    probe said only `{"authenticated":true,"authVia":"network"}`. What was
    missing was the grant and a way to see it, not the design.
  - Aliasing — having the gate or the service treat `graphql:operator` as
    holding the control-plane roles — would fix the symptom by deleting 4.2a's
    defence in depth: every legacy IDE token would silently become a
    delivery-recovery and deployment operator, and the least-privilege tokens
    Lane S4-E made possible would stop meaning anything.
  - Role names and principal IDs are not PHI and not secrets. They are
    deployment configuration, visible in the manifest and already written to
    the log on every compatibility-grant admission (`principal_id`, `grant`).
    The status endpoint shows them only to a caller already authenticated as
    that identity — a LAN client *is* the trusted-network principal — and
    knowing a role name grants nothing: authorization is re-evaluated from the
    server-owned SecurityContext on every operation. The body carries no token,
    tenant, hostname, or message-derived value; the tests assert it.
- Alternatives considered:
  - **Alias the grant** (above) — rejected.
  - **Report capabilities without roles** — rejected: the IDE could say "you
    cannot do this" but not "grant `integration.operator` in
    `FI_FHIR_GRAPHQL_ROLES`", which is the sentence that ends the outage.
  - **Fail startup on the misconfiguration** — rejected: an identity with the
    grant and no operator plane is a legitimate (if odd) least-privilege
    choice for an install that does not run the control plane; a WARN names it
    without taking the IDE down.
- Consequences:
  - The platform/gitops Deployment grants the bundle (coordinator, R-0).
  - Lane R-B gates the operator page on `capabilities.operatorRead` and names
    `missingRoles.operatorRead` instead of issuing a query it knows will fail.
  - When the compatibility grant is finally narrowed, `clinical:read` in the
    bundle keeps the event browser working without a second migration.
- Sources:
  - [S1] `.loom/36-ide-repair-execution-specs.md` (evidence table row 1, R-0, R-A)
  - [S2] `internal/integration/operator/types.go` (`ReadRole`,
    `DeploymentOperatorRole`), `internal/integration/delivery/types.go`
    (`OperatorRole`), `internal/integration/operator/service.go` (`authorize`)
  - [S3] `internal/api/graphql/operation_authorization_roles.go` (the
    compatibility grant and the 4.2a role sets)
  - [S4] `TestAuthStatusCapabilitiesAgreeWithTheOperatorService` — the real
    operator service behind the real handler; aliasing the grant in the
    derivation makes it fail on the production identity
