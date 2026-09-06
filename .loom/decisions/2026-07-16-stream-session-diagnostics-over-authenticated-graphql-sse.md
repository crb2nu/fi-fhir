### 2026-07-16: Stream Session Diagnostics over Authenticated GraphQL SSE

- Decision:
  - Add feature-gated GraphQL SSE on the existing bounded authenticated
    `POST /graphql` endpoint and allowlist only Integration Session subscription
    roots on that transport.
  - Keep `/graphql/ws` unmounted. Subscribe before starting a preview, project
    server-owned run snapshots, and reconcile against the immutable terminal run.
  - Use canonical inspector paths for lineage and exclude persisted raw field
    previews and retained sample payloads from GraphQL stream projections.
- Rationale:
  - SSE supplies low-latency one-way progression without reopening gqlgen's
    unbounded pre-authentication WebSocket frame or permitting mutations over a
    transport outside the bounded POST boundary.
  - Durable terminal snapshots preserve correctness if process-local fanout
    drops an intermediate progress event.
- Alternatives considered:
  - Re-enable GraphQL WebSocket (rejected because the existing transport does
    not expose the required pre-authentication frame bound).
  - Browser polling (rejected because it obscures real stage ordering and adds
    avoidable database load).
  - Add a message broker now (deferred to Phase 4 multi-replica operations).
- Consequences:
  - Both backend and UI feature gates must be enabled, and temporary
    `graphql:operator` authorization remains required.
  - Fanout is process-local; production GitOps activation and durable
    cross-replica replay remain pending.
- Evidence:
  - MR `!115` pipeline `19464` passed 34/34, including required session job
    `187950` and benchmark job `187953`, and merged as `36f2bb8c`.
  - Main pipeline `19482` passed 37/37 and independently repeated the session
    proof in job `188135`.
- Sources:
  - [S1] `internal/api/graphql/server.go`
  - [S2] `internal/api/graphql/operation_authorization.go`
  - [S3] `internal/api/graphql/resolvers/integration_session_service.go`
  - [S4] `ui/src/lib/graphql/subscriptions.ts`
  - [S5] `.loom/iteration-plan-phase-3-slice-3-2-streaming-diagnostics-lineage.md`
