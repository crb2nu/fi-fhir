### 2026-09-26: Streaming flips before the UI image; the stream allowlist stays narrow

- Decision:
  - **Production enables the Integration Session workspace by one API
    environment entry, `FI_FHIR_INTEGRATION_SESSION_ENABLED=true`, before the
    UI image that ships the session engine on by default reaches the cluster**
    (platform/gitops MR 811, merged 2026-09-26 04:41Z; libs/fi-fhir MR !228
    merged after it). The spec's original order — "the gitops env flip after
    R-C's image is on main" — is withdrawn.
  - **No retention key and no signing keys are mounted.** Samples stay
    `redact`; explicit raw retention is refused; publish/approve/deploy stay
    unavailable. Turning either on is a separate decision with its own secret.
  - **The SSE allowlist is not widened.** `integrationSessionEvents` and
    `sessionRunEvents` remain the only subscription roots the durable API
    streams. Events → Live Stream, Workflow Monitor, Debug and Runtime Output
    render the honest "not available on this deployment" state and never
    subscribe.
- Rationale:
  - The original order guarded against a UI whose build flag had no fallback:
    with `VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=true` and the API's
    workspace off, HL7 intake abandoned the stateless preview and every call
    failed ("legacy integration execution is unavailable"). Lane R-B replaced
    that flag with `resolveIntegrationSessionEngine(buildEnabled, state,
    stream)`: the engine runs only when the build flag is on **and**
    `/api/auth/status` reports `capabilities.integrationSessions`. Once that
    invariant held on `main`, the API side could go first: the deployed UI
    (flag off) ignored it, and the next UI degrades honestly if the entry is
    ever removed. An ordering constraint should be re-derived when the
    invariant that motivated it changes.
  - The flip's failure mode is contained by the Deployment itself: one
    replica, default rolling update, so a session-store migration failure
    keeps the new pod NotReady while the old pod serves. Rollback is deleting
    the entry. The store migrates itself at startup
    (`cmd/fi-fhir/preview_runtime.go`, `PostgresStore.Migrate`) into the
    durable database the operator control plane already opens, so no new
    secret, database or manual step was needed.
  - The allowlist is the durable API's PHI-minimal stream surface: the session
    roots carry redacted run stages, diagnostics and lineage; the legacy roots
    carry event payloads and runtime state from the pre-durable engine. Lane
    R-C's kill-test showed the legacy panels could never stream on the durable
    path by construction; making them light up would mean widening that
    surface, which is the wrong trade for a panel.
- Alternatives considered:
  - **Flip after the UI image** (the spec's order) — safe but slower by two
    merges and a rollout, for an invariant R-B had already made unnecessary.
  - **Mount the AES retention key in the same MR** — rejected: it enables
    storing raw HL7 bytes, which the docs say to prefer not to; nothing in the
    program needs it.
  - **Widen the allowlist** so the four legacy panels stream — rejected, as
    above; the honest state plus the alternative each surface names (Events
    browser, Run Diagnostics, Dry Run, step-on-request) is the product.
- Consequences:
  - Production reports `capabilities.streaming: true` and
    `subscriptions: ["integrationSessionEvents","sessionRunEvents"]` once the
    API image carrying R-A's contract rolls; `integrationSessionEvents`
    answered `200 text/event-stream` from the LAN within three minutes of
    the merge, `eventStream` answers the `FORBIDDEN` event.
  - The four legacy streaming panels are documented as working as designed
    (`RUNBOOK.md`, "Live streaming is unavailable"); a request to make one
    stream is a request to widen the allowlist and needs its own decision.
  - Open tabs keep the capabilities they loaded with. After a rollback they
    keep choosing the session engine until reloaded; re-probing the status
    when a session mutation is refused is a banked follow-up (roadmap Now).
- Sources:
  - [S1] `.loom/36-ide-repair-execution-specs.md` — R-C, "Corrections
    (2026-09-26, from Lane R-C's kill-test)"
  - [S2] platform/gitops MR 811 (`fa50a9077`, merged `4ccc2214a`); API log
    lines `integration session workspace configured` and `integration session
    durable stream fanout enabled` on pod `fi-fhir-api-c97bfcf46-brhs2`
  - [S3] `ui/src/lib/features/integration-session/api.ts`
    (`resolveIntegrationSessionEngine`), MR !226
  - [S4] `internal/api/graphql/operation_authorization.go`
    (`integrationSessionStreamRoots`), `server_security_test.go`
    ("operator is limited to session subscription roots")
  - [S5] `docs/operations/INTEGRATION-SESSIONS.md#production`,
    `docs/operations/RUNBOOK.md#live-streaming-is-unavailable` (MR !228)
