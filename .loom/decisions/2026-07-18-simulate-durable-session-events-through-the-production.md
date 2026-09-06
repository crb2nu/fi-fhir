### 2026-07-18: Simulate Durable Session Events Through the Production Pure Planner

- Decision:
  - Bind each workflow simulation to one append-only session workflow revision
    and an explicit ordered set of successful immutable session run IDs.
  - Reconstruct canonical events only from those server-owned runs and reuse the
    production `workflow.Planner`; record only revision provenance and
    configuration-free route, planned-transform, and action identity traces.
  - Persist simulations in PostgreSQL and compare deterministic trace-key sets.
    Workflow Builder must save the YAML revision before simulation and must not
    send browser event payloads.
- Rationale:
  - A browser-local draft plus browser-supplied JSON cannot prove which exact
    workflow was tested or reproduce the outcome after restart.
  - The pure planner supplies production route semantics without exposing an
    execution-capable handler path to authoring data.
- Alternatives considered:
  - Reuse `SimulationEngine` (rejected because unmocked action types can dispatch
    real handlers).
  - Persist full events, transformed payloads, or action configuration in each
    trace (rejected because it duplicates PHI and secret-bearing configuration).
  - Execute transforms during Slice 3.3 (deferred until a production-pure
    transform planner exists; current production planning reports them as
    planned rather than executed).
- Consequences:
  - Traces survive service restart and can be compared without mutable draft
    drift, while remaining PHI-minimal and side-effect free.
  - Artifact content is serialized as opaque bytes in export snapshots so YAML
    workflow revisions round-trip instead of being misclassified as JSON.
  - Signed publication, approval, deployment, and production GitOps activation
    remain Slice 3.4 work.
- Evidence:
  - Workflow/parser, session/store, GraphQL resolver, and Workflow Builder tests
    pass; the PostgreSQL 16 race/restart kill test restores two simulations and
    proves the expected delta and sentinel exclusion.
  - MR `!122` pipeline `19872` passed 37/37 with session job `191685` and
    benchmark job `191688`; merge commit `d42f7233` passed main pipeline `19878`
    40/40 with independent session job `191786` and benchmark job `191789`.
- Sources:
  - [S1] `internal/workflow/plan.go`
  - [S2] `internal/integration/session/workflow_simulation.go`
  - [S3] `internal/api/graphql/resolvers/integration_session_service.go`
  - [S4] `ui/src/lib/features/workflows/components/DryRunPanel.svelte`
  - [S5] `.loom/iteration-plan-phase-3-slice-3-3-workflow-draft-simulation.md`
