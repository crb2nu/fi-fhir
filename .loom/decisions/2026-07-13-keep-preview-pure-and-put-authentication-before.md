### 2026-07-13: Keep Preview Pure and Put Authentication Before Adapters

- Decision:
  - Compile published workflows through an explicit DSL v1 grammar with closed
    fields, action types, document shape, and resource limits. Preserve exact
    immutable YAML bytes as the artifact identity while projecting only route,
    action identity, and destination binding into a pure planner.
  - Compile only the explicitly supported Source Profile subset for HL7v2 ADT
    A01. Reject unsupported authored behavior instead of silently using a
    default, and construct a fresh strict parser for every request.
  - Derive event identity from tenant, exact integration revision, source,
    MSH-10, source digest, event type, and ordinal. Exclude request correlation
    and clocks so repeated and concurrent preview is byte-deterministic.
  - The preview processor owns no handler, resolver, session store, destination
    client, action handler, or clock. Production fails before loader access
    until a durable committer exists.
  - Insert Slice 1.1c before any GraphQL/IDE activation: authenticated tenant and
    principal context, POST-only raw payloads, bounded bodies, explicit HTTP
    origins, disabled WebSocket transport, adapter parity, and fail-closed legacy submit/session
    paths are prerequisites to exposing the kernel.
- Rationale:
  - Legacy workflow YAML and engine dry-run semantics are permissive and
    execution-capable; importing them directly would make preview depend on
    handlers, suppress CEL failures, and expose secret-bearing action config.
  - Parser wall time, permissive framing, and caller-supplied integration
    revisions would make audit identity nondeterministic or attacker-selected.
  - The existing GraphQL surface does not yet establish an authenticated
    tenant/principal boundary, permits credentialed origin reflection and broad
    WebSocket origins, and accepts queries over GET. Wiring only one preview
    resolver would leave adjacent raw-PHI and execution paths unsafe.
- Alternatives considered:
  - Reuse `Engine.DryRun` (rejected because it still owns execution-capable
    dependencies and its route semantics hide some CEL failures).
  - Accept the full legacy Source Profile schema and ignore unsupported fields
    (rejected because a published profile could claim behavior the runtime does
    not execute).
  - Disable only the GraphQL submit resolver in this slice (rejected because it
    would not establish identity, origin, method, body, or session containment
    for the rest of the transport surface).
- Consequences:
  - Slice 1.1b is an internal alpha kernel, not a remotely callable product
    feature. Slice 1.1c is now a security gate rather than optional UI wiring.
  - The published workflow/profile subset is a versioned executable contract;
    adding semantics requires tests and an explicit compatibility decision.
  - Slice 1.2 can wrap the same pure evaluation result with receipts,
    idempotency, trace, and outbox state without creating a second engine.
- Sources:
  - [S1] `internal/workflow/published_yaml.go`
  - [S2] `internal/workflow/plan.go`
  - [S3] `internal/integration/processor/profile_compile.go`
  - [S4] `internal/integration/processor/message_processor.go`
  - [S5] `internal/integration/processor/message_processor_integration_test.go`
  - [S6] `internal/api/graphql/server.go`
  - [S7] `internal/api/graphql/resolvers/schema.resolvers.go`
