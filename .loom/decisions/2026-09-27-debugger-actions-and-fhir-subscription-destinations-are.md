### 2026-09-27: Debugger actions and FHIR subscription destinations are deployment-owned allowlists

- Decision: What the workflow debugger may execute, which executables the
  `exec` action may run, and which FHIR servers subscription management may
  contact are decided by the deployment's environment, never by the GraphQL
  caller's input. Defaults are the most restrictive setting: the debugger stubs
  every action as a recording no-op (`FI_FHIR_WORKFLOW_DEBUG_ACTIONS` empty) and
  never runs `exec`; `exec` refuses every command
  (`FI_FHIR_WORKFLOW_EXEC_ALLOWLIST` empty) and the YAML `allowlist` can only
  narrow the deployment's; subscription management refuses every destination
  (`FI_FHIR_FHIR_SUBSCRIPTION_ALLOWED_HOSTS` empty; https only, exact host or
  `*.suffix`, redirects re-checked). Debug sessions are bounded per process
  (`FI_FHIR_WORKFLOW_DEBUG_MAX_SESSIONS` 8, `FI_FHIR_WORKFLOW_DEBUG_SESSION_TTL`
  15m) and the subscription client cache is an LRU
  (`FI_FHIR_FHIR_SUBSCRIPTION_MAX_CLIENTS` 32). A malformed value, or `exec` in
  the debugger list, fails serve startup.
- Rationale: The public-demo survey (`.loom/40` § "Two production findings")
  found that any `graphql:operator` holder could make the API pod run any binary
  in the image (the debugger ran production actions from pasted YAML, and
  `exec`'s allowlist came from that same YAML) and issue requests to any URL
  (`createFhirSubscription`). Both are closed as SEC-2026-09-27-1 and -2 in
  `docs/operations/SECURITY.md`. A debugger that performs side effects is the
  wrong default regardless of who may call it; the stub keeps it useful by
  putting the action's resolved (redacted, bounded) inputs on the paused step.
- Alternatives considered: (a) Remove the debugger and subscription mutations
  from `graphql:operator` — rejected: it hides the surfaces rather than making
  them safe, and the IDE uses the debugger. (b) Resolve the subscription host
  and refuse private ranges in-process — rejected as the primary control: DNS
  rebinding makes it racy, and the `fi-fhir` namespace NetworkPolicy
  (platform/gitops `k3s/fi-fhir/network-policy.yaml`) enforces egress at the
  network layer instead. (c) Ignore the YAML `allowlist` key entirely —
  rejected: intersection keeps existing workflows valid and lets an author
  narrow further. (d) Bound debug sessions per principal — deferred: sessions
  carry no principal today; a per-process bound with a TTL caps memory and
  goroutines, which is the actual exposure.
- Consequences: A deployment that used `exec` must now set
  `FI_FHIR_WORKFLOW_EXEC_ALLOWLIST`; a deployment that uses GraphQL subscription
  management must set `FI_FHIR_FHIR_SUBSCRIPTION_ALLOWED_HOSTS`. Production
  (`k3s/fi-fhir`) sets neither and uses neither. The six keys appear in
  `serve --help` and in the Engine tab. Debug-step variables gain
  `action.stubbed` and `action.inputs`; no GraphQL schema change.
  New metrics `fi_fhir_workflow_debug_sessions` and
  `fi_fhir_workflow_debug_sessions_total{outcome}`.
- Sources:
  - [S1] `.loom/40-public-demo-execution-specs.md` § "Two production findings", § "D-4"
  - [S2] `docs/operations/SECURITY.md`
  - [S3] `internal/workflow/debug_actions.go`, `internal/workflow/exec_policy.go`,
    `internal/api/graphql/resolvers/debug_policy.go`,
    `internal/api/graphql/resolvers/subscription_policy.go`,
    `cmd/fi-fhir/workflow_hardening_runtime.go`
