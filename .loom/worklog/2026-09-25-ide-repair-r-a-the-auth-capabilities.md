### 2026-09-25 - IDE repair R-A the auth capabilities contract

- What changed:
  - `GET /api/auth/status` now reports, for an authenticated caller, the
    principal, the roles the transport gate sees, derived capabilities
    (`operatorRead`, `operatorDelivery`, `operatorDeployment`, `clinicalRead`,
    `integrationSessions`, `streaming`, `subscriptions`, `llm.configured`) and
    `missingRoles` per role capability. Unauthenticated callers keep exactly
    `{"authenticated":false}`. Derivation lives in
    `internal/api/graphql/capabilities.go` and reads only the SecurityContext
    and `ServerConfig`.
  - The status probe and `/graphql` now share one caller-resolution function
    (`authenticateRequest`), so a bearer is judged alone on the probe too — it
    previously reported `network` for a LAN request carrying a bearer the
    route would judge on its own.
  - The SSE allowlist is one variable, `integrationSessionStreamRoots`, read by
    both the transport check and the `subscriptions` capability (coordinator
    amendment from Lane R-C's finding).
  - `ServerConfig` gains `IntegrationSessionsConfigured` and `LLMConfigured`,
    wired from `serve`.
  - `serve` logs one WARN per configured identity (static bearer, trusted
    network, each Access principal, service bearer) holding `graphql:operator`
    without `integration.operator`: "transport grant without control-plane
    role: operator surfaces will be forbidden".
  - `scripts/check-runtime-config.sh` requires the three control-plane roles
    in the compose and `.env.example` IDE roles; both files carry the bundle.
    `.env.example` also documents the service-bearer and operator-control-plane
    variables the check had been warning about since `f7ac736d`.
  - Docs: GRAPHQL-API.md "What an operator's token must carry" and the status
    contract (with a test-pinned example); RUNBOOK "Operator page says the role
    is missing"; PRODUCTION-HARDENING roles section cites the bundle.
- Why:
  - `.loom/36-ide-repair-execution-specs.md` row 1 and row 6: from 2026-09-05
    to 2026-09-25 every production identity held the transport grant without
    the control-plane role, and the status probe could not say so.
- Evidence:
  - Day-1 gate: `TestAuthStatusTrustedNetworkShapeDay1` passed on unmodified
    main `a62e9ee6` (exactly `{"authenticated":true,"authVia":"network"}`),
    failed after the change, and ships inverted as
    `TestAuthStatusTrustedNetworkShape`.
  - Kill-test `TestAuthStatusCapabilitiesAgreeWithTheOperatorService`: the
    real `operator.Service` behind the real handler, seven role sets × three
    capabilities; the capability is true exactly when the request reaches the
    service's store. Negative control: dropping the service half of
    `operatorRead` (aliasing the grant) fails it on the production identity
    with `operator control-plane action forbidden`.
  - Live `fi-fhir serve`, trusted network `127.0.0.1/32`: with
    `integration:preview,graphql:operator,clinical:read` the status says
    `operatorRead:false`, `missingRoles.operatorRead:["integration.operator"]`
    and startup logs two warnings (`bearer`, `network`); with the bundle, all
    three operator capabilities are true and nothing is logged.
  - `go test -race` (unit and `-tags=integration` against PostgreSQL 16) for
    `./internal/api/...` and `./cmd/fi-fhir/...`; `make transport-gate` and its
    negative control; `golangci-lint`; `make docs-validate`;
    `make check-runtime-config` (24 passed, 0 warnings).
- What's next:
  - Lane R-B consumes the contract (operator pre-flight, streaming empty state).
  - The coordinator's R-0 grants the bundle in platform/gitops.
  - Open: the operator control plane's own availability
    (`FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED`) is not in the contract — the
    server config does not know it; a deployment without it answers
    `operator control plane unavailable` whatever the roles.
- Sources:
  - [S1] `.loom/36-ide-repair-execution-specs.md` (R-A)
  - [S2] Decision 2026-09-25, "Grant the operator bundle rather than alias the
    transport grant"
