# RALPH Iteration Plan — Phase 3 Slice 3.3 Workflow Draft Simulation

## Review

- Roadmap milestone: Phase 3 durable IDE lifecycle, Slice 3.3.
- Spec sections: `.loom/20-product-spec-integration-engine-ide-completion.md`
  durable IDE definition and Golden Journey 001; `.loom/30-implementation-plan-integration-engine-ide-completion.md`
  Slice 3.3.
- Prior decisions to preserve: PostgreSQL is the durable session truth; session
  artifact saves are append-only revisions; terminal parse runs are immutable;
  the production `workflow.Planner` owns side-effect-free route semantics; action
  configuration, secrets, raw samples, and transformed PHI do not cross the
  planning boundary; production GitOps activation remains separate.

## Align

- Slice name: workflow draft simulation.
- Scope in:
  - save each tested workflow draft as an exact session artifact revision;
  - simulate an explicit set of immutable successful session runs against that
    exact revision using the production pure planner;
  - persist immutable, PHI-minimal route/transform/action traces and revision
    provenance so results survive restart;
  - compare two durable simulations with deterministic event, route, transform,
    and action deltas;
  - let the Workflow Builder select a durable Integration Session, save the
    current draft, run it against server-owned events, and render provenance,
    traces, and the delta from the prior simulation.
- Scope out:
  - executing transforms, actions, sandbox destinations, or external
    terminology/LLM calls;
  - workflow publication, signing, approval, promotion into production, or
    GitOps activation (Slice 3.4);
  - raw event payloads, action configuration, secrets, or transformed payloads
    in simulation records or GraphQL output;
  - multi-replica stream fanout and Phase 4 RBAC.
- Acceptance criteria:
  1. A simulation references one session-owned workflow revision by ID and
     digest plus an explicit set of successful immutable parse-run IDs.
  2. The server reconstructs events only from those durable runs, evaluates the
     existing pure planner, performs no action or transform side effects, and
     returns ordered event -> route -> transform/action traces.
  3. The simulation record survives store/service restart and contains no raw
     sample, event payload, action config, secret, or transformed PHI.
  4. Comparing two simulations produces deterministic added/removed event,
     matched-route, transform, and action keys and rejects cross-session input.
  5. Workflow Builder session mode sends no browser-local event payload: it
     saves the current YAML revision, simulates server-owned runs, and shows the
     exact revision provenance, trace, and prior-run delta.
  6. Disabled session-workspace operations and unauthorized callers continue to
     fail closed under the existing `graphql:operator` boundary.
- Dependencies/blockers: Slice 3.1 PostgreSQL session store and immutable runs;
  Slice 3.2 GraphQL/session feature gates; strict bounded YAML preflight and the
  production pure planner.
- Risk notes:
  - Riskiest assumption: session run payload snapshots contain the same canonical
    event JSON needed by the production planner, so simulation can reuse that
    planner without creating a second workflow engine.
  - Kill test: persist one parsed event, simulate two workflow revisions over the
    same run, restart the PostgreSQL store, compare the exact restored simulations,
    and prove the expected route/action delta while a filesystem side-effect trap,
    raw-PHI sentinel, and action-config sentinel remain absent.

## Land

- Planned file areas: `internal/workflow`, `internal/integration/session`,
  `internal/api/graphql`, `ui/src/lib/features/workflows`, generated GraphQL
  types, CI/Make targets, and completion evidence docs.
- Implementation steps:
  1. Extend the pure planner with safe transform metadata and add a bounded
     authoring-workflow parser that retains the planner's no-side-effect boundary.
  2. Add immutable simulation records, PostgreSQL migration/store methods,
     deterministic comparison, exports, and restart/leakage tests.
  3. Add operator-only GraphQL queries/mutation and wire Workflow Builder session
     mode to exact saved revisions and server-owned run events.

## Prove

- Tests: workflow planner/parser units; memory-store/service/GraphQL resolver
  tests; PostgreSQL restart/delta/leakage kill test; Workflow Builder/Dry Run UI
  component and API tests.
- Lint/static: `gofmt`, `go vet`, targeted `go test -race`, UI Svelte/TypeScript
  checks, ESLint/stylelint, and generated-contract cleanliness.
- Broader gates: `go test ./...`, UI test/build, repository docs validation,
  blocking Integration Session CI job, MR pipeline, and post-merge main pipeline.

## Handoff/Harvest

- Docs to update: completion implementation plan, product-spec evidence,
  decision log, status, changelog, and Integration Session operations guide.
- Agent-context entries: durable simulation contract, pure planner reuse,
  validation evidence, and remaining publication boundary.
- Next-slice candidate: Phase 3 Slice 3.4 signed publish and deploy.
