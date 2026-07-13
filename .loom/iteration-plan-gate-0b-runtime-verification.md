# RALPH Iteration Plan: Gate 0B Truthful Runtime Verification

**Status**: complete
**Date**: 2026-07-12

## Review

- Roadmap milestone: Completion Gate 0B
- Spec sections: truthful verification; IDE/runtime contract
- Prior decision: make one Golden Path trustworthy before expanding the
  integration engine.
- Evidence:
  - pipeline 18415 reported successful UI and smoke jobs that exited before
    executing tests because the binary artifact was absent;
  - the binary producer only matched TypeScript SDK changes while its consumers
    matched UI, Go, schema, and CI changes;
  - the serve jobs exported an unused address variable and probed port 8081
    while the server remained on its port 8080 default;
  - smoke-test counters terminate the script on the first successful assertion
    under set -e;
  - a clean UI checkout needs SvelteKit sync before Vitest can load its config;
  - the live UI fetch shim recursively calls itself when live tests are enabled;
  - no transport-level test proves GraphQL WebSocket negotiation and delivery.

## Align

### Scope in

- Make the binary artifact required for every runtime-contract consumer.
- Fail UI and smoke jobs if the server never becomes ready.
- Run SvelteKit sync and the complete Vitest suite against the live server.
- Fix the live GraphQL fetch shim.
- Make every smoke assertion run and require a real WebSocket upgrade.
- Add an end-to-end GraphQL transport test for connection acknowledgement,
  subscription registration, mutation, and sample.added delivery.
- Declare npm 10.9.3 canonical and remove the stale pnpm lock.
- Turn on the GitLab setting that rejects merges without a successful pipeline
  after the new gate proves green.

### Scope out

- security allow-failure promotion and npm vulnerability remediation;
- advisory integration-test and lint gate promotion;
- component coverage thresholds and missing IDE journey tests;
- production MessageProcessor, durable receipts, MLLP, and connectors.

### Acceptance criteria

- UI, Go, schema, SDK, and CI changes cannot schedule a runtime consumer without
  scheduling its required binary producer.
- Missing binary or failed readiness produces a failed job, never a successful
  early exit.
- A clean npm frozen install runs SvelteKit sync and all Vitest files.
- Live HTTP integration tests execute without fetch recursion.
- Smoke positive proof reports exactly 3 passed and 0 failed.
- Smoke negative proof executes later assertions after one failure and exits
  non-zero.
- The real GraphQL handler negotiates graphql-transport-ws, acknowledges the
  connection, registers a subscription, and delivers sample.added after the
  matching HTTP mutation.
- The MR reaches a terminal green required pipeline and failed pipelines cannot
  be merged by project policy.

## Land

### Exact intended files

- Diff-size exception: the stale generated pnpm lock deletion is 5,704 lines.
  The authored change is 652 added lines, including 142 planning/changelog
  lines; the 510-line implementation/proof spans the binary producer, readiness,
  smoke, UI, and WebSocket chain that must land atomically to avoid another
  false-green intermediate state.
- .gitlab-ci.yml
- scripts/smoke-test.sh
- scripts/smoke-test_test.sh
- internal/api/graphql/server.go
- internal/api/graphql/server_live_test.go
- ui/package.json
- ui/pnpm-lock.yaml (delete)
- ui/src/lib/graphql/integration.test.ts
- CHANGELOG.md
- .loom/30-implementation-plan-integration-engine-ide-completion.md
- this iteration plan

### Implementation sequence

1. Repair the smoke harness and prove both positive and negative execution.
2. Expose the production HTTP handler and add the transport-level subscription
   test with a deterministic subscription-ready barrier.
3. Repair the live UI tests and frozen-install contract.
4. Align CI producer/consumer rules, readiness, cleanup, and required artifacts.
5. Run targeted and broad local gates, self-review, then drive the MR pipeline
   to terminal green.

## Prove

### Targeted

    bash -n scripts/smoke-test.sh scripts/smoke-test_test.sh
    bash scripts/smoke-test_test.sh
    go test -count=1 ./internal/api/graphql -run TestLiveIntegrationSessionSubscription
    npm -C ui ci --no-audit --no-fund
    npm -C ui run test:run -- src/lib/graphql/integration.test.ts

### Broad

    go test ./internal/api/graphql/...
    go test ./...
    npm -C ui run lint
    npm -C ui run lint:css
    npm -C ui run check
    npm -C ui run typecheck
    npm -C ui run test:run
    npm -C ui run build

### Negative kill-tests

- Point smoke at a fake client that fails GraphQL: health and WebSocket still
  execute, the summary reports one failure, and the script exits 1.
- Remove or suppress the binary artifact in a validation branch: test:ui and
  test:smoke must be blocked or red, never green.
- Introduce one failing UI assertion on a validation branch: test:ui must be red.

## Handoff

- MR !91 merged as commit `6bdcc8e0` after pipeline 18494 passed. Main pipeline
  18498 then passed after one benchmark retry confirmed runner noise rather than
  a threshold regression. GitLab now requires a successful pipeline before
  merge.
- Gate 0B remains open only for truthful security-job promotion and vulnerability
  remediation.
- Once Gate 0B closes, execute Phase 1 Slice 1.0 foundation contracts, then the
  shared MessageProcessor.
