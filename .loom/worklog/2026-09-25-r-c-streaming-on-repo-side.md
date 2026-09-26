### 2026-09-25 - R-C: streaming on, repo side

- What changed:
  - **UI image default flipped.** `ui/Dockerfile` builds with
    `VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=true`. The comment says what the
    flag means after R-B: it only permits the session engine, and
    `resolveIntegrationSessionEngine` also needs the API's
    `capabilities.integrationSessions`. `build:docker-ui` passes no override,
    so there is no CI change. `docker-compose.yaml` builds `./ui` with no args
    and inherits the new default. Its API has sessions off, so the local stack
    degrades the same way production does.
  - **`INTEGRATION-SESSIONS.md` → Production.** The subsection covers:
    - the single gitops env entry beside
      `FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED`, as in gitops MR 811;
    - the durable connection the store reuses;
    - the seven session-ledger migrations;
    - verification: the `/api/auth/status` capabilities, plus an SSE probe
      that creates nothing;
    - rollback: delete the entry. The same image falls back to the stateless
      preview because the capability goes false.
  - **`RUNBOOK.md` → "Live streaming is unavailable".** The section covers:
    - HTTP 404 versus the sanitized `FORBIDDEN` refusal;
    - `data-reason`;
    - a per-panel table of which subscriptions can ever stream;
    - why the allowlist stays PHI-minimal;
    - how the UI decides.
  - **Sanitized forbidden message (R-B follow-up, coordinator add-on).**
    - The problem: the server's catalog-safe presenter rewrites every
      FORBIDDEN-coded error to "GraphQL operation forbidden". R-B's
      `/stream operation forbidden/i` therefore never matched a real refusal.
    - The SSE client now throws `GraphQLStreamError`, which keeps
      `extensions.code`.
    - `classifyStreamError` keys on `FORBIDDEN` and falls back to
      `/operation forbidden/i`.
    - The server presenter is untouched.
  - **Negative path as a test.** `buildOnApiOff.test.ts` fakes only `fetch`,
    with an in-process stand-in that answers the way `internal/api/graphql`
    does while the workspace is off. It drives the real credential gate,
    capability store, engine gate, `HL7PreviewPage`, `DryRunPanel`, GraphQL
    client, SSE client and toast net. Each negative case has a positive
    control (API sessions on).
- Why:
  - `.loom/36` R-C and its Corrections. The flip had to wait for R-B's
    capability gate (!226, merged as `164934af3`). The production docs have
    to name exactly what the gitops flip does.
- Evidence:
  - A read-only `kubectl get` of the live Deployment shows three things:
    - R-0's roles are live.
    - `FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true` and
      `FI_FHIR_DATABASE_SSL_MODE=disable` are already set, so
      `openSubmissionDatabaseFromEnv` already runs in production.
    - That connection defaults to `sslmode=require`, unlike the legacy
      profile/event/workflow stores, which fall back to `disable`. The
      in-cluster PostgreSQL has no TLS.

    A deployment that enables sessions as its first durable feature must set
    `FI_FHIR_DATABASE_SSL_MODE`. Production needs nothing new. The session
    store does not read `FI_FHIR_DATABASE_URL`.
  - `buildOnApiOff.test.ts`: all 4 cases pass (2 negative, 2 controls).
  - Kill-tests, each run against the host tree and then restored:
    - Reverting `resolveIntegrationSessionEngine` to the build flag alone
      (pre-R-B behaviour) fails both negative cases. Both controls still
      pass.
    - Reverting `classifyStreamError` to R-B's regex fails the three new
      sanitized-refusal tests.
  - Full UI suite in devbox (the canonical `ui/node_modules` is gone from the
    host): `npx vitest run` passes 787 tests with 3 skipped. The baseline
    after R-B was 779/3, and this lane adds 8. `npm run check` reports
    0 errors (9 pre-existing warnings), and `npm run lint` is clean.
    `scripts/validate-docs.sh` and `scripts/worklog.sh check` pass.
- What's next:
  - The API side is already on in production (gitops MR 811). This image
    completes the flip, and R-D then asserts the end state.
  - R-B follow-up, not fixed here: a tab whose capabilities predate a
    rollback keeps choosing the session engine. Its Preview fails at
    `createIntegrationSession` with `legacy integration execution is
    unavailable`, and toasts it. The stream-404 self-heal never runs, because
    the mutation fails before the subscription opens. The docs say to reload
    tabs. The fix would treat that error like a stream 404, or re-probe
    `/api/auth/status` when it arrives.
- Sources:
  - [S1] `.loom/36-ide-repair-execution-specs.md` (R-C, Corrections)
  - [S2] `cmd/fi-fhir/preview_runtime.go` (`openSubmissionDatabaseFromEnv`,
    session store wiring), `pkg/config/config.go` (`SSLMode: "require"`),
    `cmd/fi-fhir/serve_profile_store.go` (legacy `disable` fallback)
  - [S3] `internal/integration/session/postgres.go` (`Migrate`, ledger 1–7)
  - [S4] `internal/api/graphql/server.go` (`catalogSafeErrorPresenter`),
    `internal/api/graphql/server_security_test.go`
    (`TestIntegrationSessionSSEBoundary`)
