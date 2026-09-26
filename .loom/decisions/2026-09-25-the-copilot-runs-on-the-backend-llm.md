### 2026-09-25: The Copilot runs on the backend LLM not the loom platform

- Decision:
  - **The IDE Copilot's source of truth is the API's own `llmCapability`**,
    probed each time the Copilot panel opens, and its actions run the existing
    GraphQL LLM operations (`explainWorkflow`, `suggestMappings`,
    `generateWorkflow`, `analyzeQuality`) through `graphqlFetch`. The panel
    shows one of three states — ready, "The deployment's LLM is not
    responding" (configured, provider did not come up), "No LLM is configured
    for this deployment" (LLM off or no valid provider config) — plus
    checking/unknown while the probe runs or when it does not answer.
  - **The loom-platform MCP client stays, for HUD ambient features only**,
    and only when `PLATFORM_CONFIG.enabled` (`PUBLIC_LOOM_ENDPOINT` set at
    build time). Without it the status bar shows no Platform indicator or
    AlertBadge, `initializePlatform()` is not called, and the HUD event feed
    produces nothing (it used to fall back to simulated events).
- Rationale:
  - `.loom/23` Wave 2 already moved the Copilot's *dispatch* onto the backend
    operations; only the gate was left on `platformState.connected`. No build,
    CI job or gitops manifest sets `PUBLIC_LOOM_ENDPOINT`, so every deployment
    showed "Platform connection required" while production had a working
    `LLM_BASE_URL` (litellm) behind `llmCapability` — a dead gate in front of
    a live feature.
  - `llmCapability` is the backend's own, secret-safe answer (`.loom/23`
    Slice 3f): it knows whether LLM features are enabled, whether the provider
    config validates, and which feature rows are wired. The platform
    connection knows none of that.
- Alternatives considered:
  - Set `PUBLIC_LOOM_ENDPOINT` in the UI image so the gate opens: rejected —
    it makes a clinical-integration IDE depend on an unrelated agent platform
    for a feature the API serves itself, and bakes an endpoint into a
    PUBLIC_* build variable.
  - Gate on R-A's `capabilities.llm.configured` from `/api/auth/status`
    instead of `llmCapability`: rejected as the source of truth — it is a
    single bit read at gate time, while `llmCapability` carries status,
    warnings and per-feature reasons the panel needs to explain itself. It
    remains available as a hint.
  - Remove the loom-platform client entirely: rejected — the HUD ambient
    features are a legitimate optional integration; hiding them when
    unconfigured is enough.
- Consequences:
  - The Copilot works on any deployment whose API has an LLM configured, with
    no extra UI build configuration; `ui/src/lib/features/copilot` no longer
    imports `$lib/platform`.
  - The browser smoke gate (Lane R-D) can assert `copilot-llm-state`
    (`data-state="not-configured"` in CI, where no LLM runs) and the absence
    of `platform-indicator`.
  - A future live health probe of the provider (today `llmCapability` is
    decided at startup) would refine "not responding" without changing the
    panel.
- Sources:
  - [S1] `.loom/36-ide-repair-execution-specs.md` — evidence row 3, R-B item 4,
    coordinator decision 2
  - [S2] `.loom/23-functionality-gaps-plan.md` — Wave 2 (Copilot on the
    backend LLM), Slice 3f (`llmCapability`)
  - [S3] `ui/src/lib/platform/config.ts` (`enabled: !!PUBLIC_LOOM_ENDPOINT`);
    `cmd/fi-fhir/main.go` (serve's LLM status and warnings)
  - [S4] `.loom/worklog/2026-09-25-ide-repair-r-b-honest-surfaces.md`
