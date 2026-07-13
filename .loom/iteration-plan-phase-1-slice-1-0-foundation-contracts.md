# RALPH Iteration Plan: Phase 1 Slice 1.0 Foundation Contracts

**Status**: complete
**Date**: 2026-07-13

## Review

- Roadmap milestone: Engine Alpha / Golden Path 001 foundation
- Spec sections: foundation contracts before durable schemas; one shared
  MessageProcessor boundary; secure production data plane
- Prior decisions:
  - one logical healthcare-organization security domain per 1.0 deployment;
  - tenant and actor identity exist on every durable/runtime contract from day
    one even though shared multi-tenant hosting is not a 1.0 claim;
  - raw payload is ephemeral by default and retained only by explicit encrypted,
    audited, purpose-bound policy;
  - preview is side-effect-free and production/preview must converge on one
    future MessageProcessor semantic path.
- Existing constraints:
  - Integration Session currently defaults omitted PHI policy to retention and
    its ad hoc preview stores raw input;
  - profile and workflow revisions use different internal persistence models and
    neither carries tenant or content digest;
  - workflow and ingest configuration can embed literal credentials;
  - diagnostics, route results, and correlation identifiers have several
    incompatible internal representations.

## Align

### Scope in

- Introduce dependency-light public contracts under `pkg/integration` for:
  - tenant, human/service principal, audit, typed secret reference, PHI
    classification, and raw-retention policy;
  - content-addressed source/profile/workflow/destination references and one
    minimal `IntegrationDefinitionRevision`;
  - `RawEnvelope`, production/preview `ProcessRequest`, stable diagnostics,
    events, receipts, deliveries, routes, and correlations in `ProcessResult`.
- Strictly decode and validate one Golden Path 001 revision fixture.
- Compute a deterministic revision digest over semantic content, deep-copy
  constructor inputs, protect the complete creation audit, and make mutation
  detectable by digest validation.
- Keep raw bytes out of JSON and return defensive payload copies.
- Project registered concrete canonical event types and cataloged diagnostics
  through package-controlled constructors that strip source-raw fields and
  parser warning text, reject schema/type mismatches, and preserve only safe
  diagnostic messages.
- Make preview side-effect freedom a result invariant: no receipt, attempt, or
  executed delivery; sandbox targets remain plan/simulation inputs only.
- Require an effective idempotency key and exact receipt correlation before any
  production event, route, or delivery result can validate.
- Bind the result back to the authenticated request's mode, actor, human reason,
  explicit idempotency key, and correlation ID.
- Validate result destinations and raw-retention attestations against the exact
  resolved integration revision, including a hard upper bound at the encrypted
  retention TTL.
- State the 1.0 deployment/toolchain support baseline without claiming
  unmeasured performance or conformance certification.

### Scope out

- MessageProcessor implementation, parser/profile/workflow adapters, or runtime
  wiring;
- receipt/outbox schemas, migrations, stores, idempotency, or restart proof;
- GraphQL schema/resolver, Integration Session, UI, HTTP, or MLLP changes;
- secret resolution or migration of existing literal workflow/config values;
- shared-hosting certification, RBAC enforcement, or standards conformance.

### Riskiest assumption and kill-test

The load-bearing assumption is that a content-addressed, reference-only revision
can carry every identity, PHI, retention, secret, and artifact fact required by
later schemas without embedding raw payload, mutable current pointers, or secret
values. The kill-test must finish in under 30 minutes and:

1. strict-decode and validate the golden revision;
2. mutate tenant, every artifact binding, destination, secret binding, and policy
   and require digest validation to fail;
3. mutate constructor input slices and require the constructed revision to stay
   unchanged;
4. marshal a raw envelope containing a known PHI sentinel and require the
   sentinel to be absent while checksum/size/source metadata remain;
5. reject a preview result containing a receipt or an executed delivery.

If the fixture requires inline profile/workflow configuration, plaintext
credentials, raw content, or a mutable `current` pointer to validate, receipt
schemas and processor implementation remain blocked and this boundary must be
redesigned.

### Acceptance criteria

- `pkg/integration` imports only the standard library and `pkg/events`.
- Unknown revision fields are rejected; typed secret references expose provider,
  key, and optional version only.
- Required IDs and `sha256:<64 lowercase hex>` digests are enforced.
- Human principals require roles and reason; service principals require source
  identity and authentication method; tenant identity matches every boundary.
- Raw retention defaults to ephemeral. Encrypted retention requires positive
  TTL, purpose, storage revision, encryption-key reference, authorizing actor,
  and access audit.
- Preview can plan/suppress work but cannot contain a durable receipt or an
  executed delivery status.
- Production results require a durable receipt, effective idempotency key, actor,
  reason for human operations, and exact receipt/event/attempt correlations.
- Processed event payloads must match the registered concrete canonical schema
  for their event type and reject source-raw fields and parser warning text;
  duplicate or noncanonical JSON member names are rejected at every decoded
  contract depth.
- Every returned event and delivery is bound to the result correlation, source
  message, route, action, and attempt sets that produced it.
- Stable route plans represent matched/skipped routes and planned actions
  independently of delivery attempts.
- Stable result JSON contains no Go `error` values or source raw payload.
- The focused suite is proven red before implementation, then green; the full Go
  suite and required CI pipeline pass before merge.

## Land

### Exact intended files

- `pkg/integration/contracts.go`
- `pkg/integration/doc.go`
- `pkg/integration/revision.go`
- `pkg/integration/validation.go`
- `pkg/integration/contracts_test.go`
- `pkg/integration/revision_test.go`
- `testdata/golden/integration/adt-http/integration-revision.json`
- `docs/operations/SUPPORTED-1.0.md`
- `docs/operations/README.md`
- `docs/STATUS.md`
- `.loom/40-decisions.md`
- `.loom/30-implementation-plan-integration-engine-ide-completion.md`
- `.loom/iteration-plan-gate-0b-security-truth.md`
- `CHANGELOG.md`
- this iteration plan

### Implementation sequence

1. Write the public-contract tests and fixture; run them and preserve a non-zero
   red result.
2. Implement only the types, constructors, digesting, and validation needed to
   make the focused suite green.
3. Refactor validation/digest helpers while the focused suite remains green.
4. Record the package-boundary/content-addressing decision and support baseline.
5. Run full local gates, independent review, and terminal MR/main CI.

## Prove

### Red

    go test ./pkg/integration

Initial evidence: exited `1` with `no non-test Go files`, proving the new tests
could not pass before implementation. Review regressions were also written first
and failed on missing actor/reason, route, receipt-state, and revision-aware APIs;
same-schema wrapper/payload type substitution; and wire acceptance of canonical
`raw_payload` and `parse_warnings` members.

### Green and focused

    go test -count=1 ./pkg/integration
    go test -race -count=1 ./pkg/integration
    go test -run TestMinimalIntegrationRevisionFixtureValid -count=1 ./pkg/integration

Current local evidence: focused and race suites pass; `go vet` and package lint
pass; statement coverage is 86.3%. Two independent final reviews report no
remaining P0/P1 findings.

### Broad

    gofmt -w pkg/integration/*.go
    go test -count=1 ./...
    go test -race -count=1 ./pkg/integration
    go vet ./...
    golangci-lint run --timeout=10m ./pkg/integration/...
    make security-vulncheck security-gosec security-npm-audit
    bash scripts/validate-docs.sh

Current local evidence: all commands pass. `govulncheck` reports zero reachable
vulnerabilities; `gosec` reports no unwaived high-confidence/high-severity
findings; the npm high/critical threshold passes (UI: three low findings, SDK:
zero); documentation validation reports zero warnings.

### CI and merge

- Commit: `f8ba00b781a83ae72746504b7f9021436d483c9f`
- Merge request: `!93`
- Required MR pipeline `18526`: 20/20 jobs succeeded, including the manually
  started non-optional benchmark; coverage reported 42.70% repository-wide.
- Merge commit: `2cd27af2d0750052615a2fff87496265d01e856e`
- Post-merge `main` pipeline `18527`: 22/22 jobs succeeded (lint 5, test 9,
  security 4, build 2, scan 1, deploy 1); `deploy:docker` was required and
  succeeded. No release jobs were expected on the non-tag pipeline.

## Handoff

- Next: Slice 1.1 implements the shared MessageProcessor and adapters over these
  contracts, beginning with a real-store proof that referenced profile/workflow
  revisions remain resolvable by the same digest after a later version publishes.
- Slice 1.2 owns durable receipts, idempotency, outbox, restart, and concurrency.
