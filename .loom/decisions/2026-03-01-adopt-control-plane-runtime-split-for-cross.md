### 2026-03-01: Adopt Control-Plane + Runtime Split for Cross-Service Integration

- Decision:
  - Integrate sibling repos with explicit role boundaries: `flexinfer` and `mentatlab` as runtime-adjacent integrations, `loom-core` as control-plane automation/operations integration.
- Rationale:
  - This aligns with existing stable API surfaces and avoids coupling clinical runtime paths to orchestration internals.
- Alternatives considered:
  - Direct point-to-point integration among all services (rejected due to contract and ops drift risk).
- Consequences:
  - Requires explicit integration policy docs and per-edge auth/timeout standards.
  - Enables phased rollout without architectural rewrite.
- Sources:
  - [S1] `/Users/cblevins/workspace/services/flexinfer/docs/user/api-compatibility.md:14`
  - [S2] `/Users/cblevins/workspace/services/mentatlab/docs/site/api-reference.md:7`
  - [S3] `/Users/cblevins/workspace/services/loom-core/docs/STREAMABLE_HTTP.md:14`
  - [S4] `/Users/cblevins/workspace/services/loom-core/docs/API_STABILITY.md:72`
