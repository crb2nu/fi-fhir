### 2026-09-25 - IDE repair R-B honest surfaces

- What changed:
  - **Capabilities store.** The credential gate's existing
    `/api/auth/status` fetch now also records the body in
    `accessCapabilities` (typed from Lane R-A's contract). The old two-key
    answer, and every bearer session, stays "unknown", and unknown keeps each
    surface's pre-capability behaviour.
  - **Operator page pre-flight.** With `operatorRead` false the page renders a
    warning panel (`operator-preflight`) naming the missing role and the grant
    locations (`FI_FHIR_GRAPHQL_ROLES` — static bearer, inherited by the
    trusted network — and `FI_FHIR_GRAPHQL_ACCESS_PRINCIPALS`), and mounts no
    tab, so no query is sent. Delivery and deployment controls are disabled
    with the missing role in their title when `operatorDelivery` /
    `operatorDeployment` are false. Operator operations opt out of the global
    error toast because each has an inline home (the "forbidden" double).
  - **Streaming honesty, per subscription root.** `streamAvailability`
    resolves `eventStream`, `workflowEvents`, `debugStepEvent`,
    `integrationSessionEvents` and `sessionRunEvents` from `streaming` +
    `subscriptions`, and from what a stream answered (404 → streaming off;
    "stream operation forbidden" → not allowlisted). Events live stream,
    Workflow Monitor, Debug panel and Runtime Output render one shared
    `streaming-unavailable` state (`data-stream`, `data-reason`) and never
    subscribe.
  - **Session engine gate** (coordinator amendment, from Lane R-C): HL7
    preview and the dry-run Session source use the Integration Session engine
    only when the build flag, `capabilities.integrationSessions` and the
    session stream agree; otherwise the stateless `previewIntegrationMessage`
    path, and the dry-run panel neither offers Session nor loads sessions.
    HL7 intake shows a compact `streaming-unavailable`
    (`data-stream="integrationSessionEvents"`) when the UI build opted into
    sessions but the API cannot stream them.
  - **Copilot on the backend LLM.** The platform gate is gone;
    `llmCapability` is probed on panel open and `copilot-llm-state` shows
    ready / not responding / not configured (+ checking / unknown).
  - **Platform chrome** only when `PLATFORM_CONFIG.enabled`
    (`platform-indicator`, AlertBadge, `initializePlatform()`, HUD feed).
  - **Problems badge.** The draft counts only when live (builder opened this
    session, or the draft differs from the empty default); a fresh session
    shows no `problems-badge` and the panel explains where problems come from.
  - **`/health` poller.** One request in flight, aborted only by its own 10 s
    timeout or `stop()`, hidden-tab ticks skipped, root layout the sole owner.
- Why:
  - `.loom/36` evidence: nobody in production can reach the operator plane
    and the UI could not say so; every stream 404s; the Copilot was gated on a
    platform no deployment configures; a never-opened draft put 3 errors on
    every page; `/health` showed `ERR_ABORTED` noise.
- Evidence:
  - Day-1 gate (`OperatorPage.test.ts`) passed on unmodified main (commit
    7409b2c46) and is inverted in the operator commit; the "unknown" case is
    kept as a regression test.
  - Copilot kill-test: configured-and-healthy `llmCapability` with
    `PLATFORM_CONFIG.enabled=false` → usable, no "Platform connection
    required" (`CopilotPanel.test.ts`).
  - `npx vitest run`: 779 passed / 3 skipped (baseline on main 703 / 3);
    `npm run check` 0 errors (9 pre-existing warnings in untouched files);
    `npm run lint` and `npm run lint:css` clean; `npm run build` succeeds.
  - No GraphQL operations added, so no codegen.
- What's next:
  - Rebase on Lane R-A once it merges; the store already parses its shape.
  - Bearer sessions keep unknown capabilities until the status endpoint reads
    the bearer (the gate's probe runs before a token exists and the brief
    forbids a second fetch).
  - Lane R-D asserts the testids above; Lane R-C can flip the Dockerfile
    default now that the session engine needs the API's capability too.
- Sources:
  - [S1] `.loom/36-ide-repair-execution-specs.md` (R-B, coordinator amendments)
  - [S2] `.loom/22-toast-budget-policy.md` (B4, D2, D3)
  - [S3] `.loom/decisions/2026-09-25-the-copilot-runs-on-the-backend-llm.md`
  - [S4] `internal/api/graphql/operation_authorization.go` (stream allowlist),
    `internal/api/graphql/server.go` (404 when streaming is off)
