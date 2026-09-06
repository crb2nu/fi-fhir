### 2026-07-12: Make One MessageProcessor the Product Boundary

- Decision:
  - Production adapters, GraphQL submit, and Integration Session preview must use
    one `MessageProcessor` semantic path. Preview is explicitly side-effect-free;
    production adds durability/delivery around the same parse/route result.
  - Golden Path 001 is a blocking kill-test before MLLP, connector breadth, or
    deeper IDE work.
- Rationale:
  - Current interactive and headless paths compose different primitives. A second
    preview engine would inevitably drift from production profile and routing
    behavior.
- Alternatives considered:
  - Keep the Integration Session runner separate and compare output in tests
    (rejected; tests cannot make duplicate orchestration a stable product contract).
  - Rewrite the parser/workflow kernel (rejected; the existing primitives are
    mature and the gap is composition/lifecycle).
- Consequences:
  - The first alpha optimizes for one proven HL7v2 vertical slice, not connector
    count. The program stops and redesigns the boundary if parity, duplicate, or
    restart evidence fails.
- Sources:
  - [S1] `internal/api/graphql/resolvers/schema.resolvers.go`
  - [S2] `internal/integration/session/runner.go`
  - [S3] `internal/ingest/http.go`
  - [S4] `.loom/20-product-spec-integration-engine-ide-completion.md`
