### 2026-03-29: Make Debug Sessions Truthful Before Adding More Debug Surface

- Decision:
  - Replace the branch's always-mock debug-panel behavior with real GraphQL-backed session control, start backend debug sessions in stepping mode by default, and derive lightweight trace/lineage UI state from recorded steps until server-side trace queries are implemented.
- Rationale:
  - The backend debug mutations already exist, but the UI was still sending empty workflow input and loading mock state on mount, which made the feature look integrated while preventing real debugging.
  - Default stepping gives the panel a usable first pause without requiring pre-seeded breakpoints, matching how users expect "start debug session" to behave.
- Alternatives considered:
  - Keep mock data until full trace/subscription support exists (rejected; misleading integration state).
  - Add a larger backend breakpoint/trace API expansion first (rejected for this branch-finishing pass; higher scope than needed to make the current stack usable).
- Consequences:
  - The debug panel now reflects real workflow draft input and session lifecycle.
  - `workflowRunTrace` and `debugStepEvent` remain explicit follow-up work rather than silently implied capabilities.
- Sources:
  - [S1] `ui/src/lib/features/debug/debugApi.ts`
  - [S2] `ui/src/lib/features/debug/DebugPanel.svelte`
  - [S3] `ui/src/lib/features/debug/debugStore.ts`
  - [S4] `internal/workflow/debug.go`
  - [S5] `internal/api/graphql/resolvers/debug.resolvers.go`
