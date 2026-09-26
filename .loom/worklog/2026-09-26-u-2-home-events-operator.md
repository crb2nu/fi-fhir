### 2026-09-26 - U-2: home, events, operator

- What changed:
  - **Home (`/`)**: `Toolbar` "Home" and four panels on real sources only.
    Recent = open workspace documents (ideStore) + the tenant's integration
    sessions (new `HomeRecentSessions` query, capability-gated), else
    "Nothing opened yet." with Open HL7 intake. Integrations =
    `operatorDeployments` read-only table behind the same operator
    pre-flight as `/operator`. Health = `SystemStatusPanel` restyled (backend
    `/health`, GraphQL `health`, the shell poll). Alerts = the store's firing
    list or "No alert source configured." Deleted `WarningTrends` (hard-coded
    figures), `DashboardStats`, `RecentEventsFeed`, `UnmappedCodesWidget` and
    the never-written `recentsStore`.
  - **No simulated data**: `observabilityStore` lost every generator and
    `isSimulated`; `alertSource` (`unconfigured | live | unavailable`) says
    why a list is empty. `AlertBadge` lost the Demo data tag.
  - **Events (`/events`)**: toolbar + underline tabs + count badge; Browse is
    a filters row, a fixed table and a details pane (keyboard Up/Down);
    `StreamingUnavailable` restyled through `EmptyState` with its contract
    unchanged (new contract test); Timeline and Statistics are tables.
  - **Operator (`/operator`)**: toolbar + tabs; Messages = receipts table with
    the trace pane beside it; Delivery and Deployments are tables; the
    reason dialog uses `Field`/`Textarea`/`Input`; `operator-preflight` is an
    `EmptyState` keeping `data-testid` and `data-missing-roles`.
  - e2e check 2's heading assertion follows the toolbar title ("Operator").
- Why: `.loom/37` lane U-2 (defects 1–3 and 5 on these routes; decision 3).
- Evidence:
  - MR !236: before/after PNGs at 1440×900 for `/`, `/events` (Browse, Live
    Stream, Timeline, Statistics), `/operator` (Messages, Delivery,
    Deployments), plus `/` in light.
  - `npx vitest run` 877 passed / 3 skipped (base 864); `npm run check`
    0 errors; eslint, stylelint and `codegen:check` clean.
- What's next: coordinator review of !236.
- Findings:
  - `/events` reads the legacy `graphql_events` store, which `serve` never
    writes (only the test-only legacy `submitMessage` path does); durable
    admissions go to `integration_canonical_events`. Events Browse,
    Statistics and Patient Timeline are empty on every real deployment.
  - The browse selection of `Event` has no message id, status or payload;
    `OperatorDeployment` has no integration class.
  - Local stack: U-1/U-3 hold `:9090`, so set `FI_FHIR_METRICS_PORT`; the
    delivery worker needs `FI_FHIR_QUEUE_DRIVER=kafka`; the durable HTTP
    ingress for the preview registry is integration id `adt-east`.
- Sources:
  - [S1] `.loom/37-ide-design-uplift-execution-specs.md` › U-2
