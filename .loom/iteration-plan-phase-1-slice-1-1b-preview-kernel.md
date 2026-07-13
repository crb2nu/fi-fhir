# RALPH Iteration Plan: Phase 1 Slice 1.1b Deterministic Preview Kernel

**Status**: complete
**Date**: 2026-07-13

## Riskiest assumption + kill-test

**Load-bearing assumption**: one server-owned integration revision plus its exact
immutable Source Profile and workflow revisions can be compiled into a bounded,
deterministic HL7v2 ADT A01 preview path that plans routes without executing
transforms or actions and without retaining or serializing raw PHI.

**Kill test**: in a bounded proof that finishes in under 30 minutes, persist
strict and tolerant profile revisions plus versioned workflows in PostgreSQL 16,
then combine that restart proof with the pure-planner action matrix and
file/exec/event-store side-effect traps. Resolve one operator-owned integration
revision, run the same inline A01 request through fresh processor instances, and
prove:

- exact profile/workflow/source references survive in the result;
- strict versus tolerant missing-PV1 behavior follows the selected revision;
- the valid matched route and stable action identities are deterministic;
- invalid CEL is a cataloged diagnostic rather than a silent non-match;
- no transform/action trap runs and no receipt or attempt exists;
- serialized results are byte-equivalent and exclude raw/warning sentinels;
- production mode, wrong tenant, caller-invented revision, changed digest,
  unsupported MSH-9, multiple MSH, and ambiguous destination bindings fail
  before executable content is returned.

**Failure mode if the assumption is wrong**: stop before GraphQL/IDE or ingress
wiring. Redesign the executable artifact grammar or processor boundary rather
than introduce a second preview engine, trust caller-authored revisions, or
permit an execution-capable workflow engine in preview.

Positive evidence: Slice 1.1a resolves exact immutable profile/workflow bytes
after mutable pointers advance, and Slice 1.0 seals raw-free result contracts.
Disconfirming evidence: persisted profile JSON and runtime profile schemas differ;
legacy workflow YAML is permissive; `Engine.DryRun` swallows CEL errors and marks
matched routes skipped; Integration Session preview retains raw PHI; GraphQL has
no authenticated identity; parser metadata currently uses UUIDs/wall time and
does not expose exact MSH-9.

## Review

- Roadmap milestone: Engine Alpha / Golden Path 001 runtime spine.
- Parent slice: 1.1a established exact immutable profile/workflow resolution.
- Security correction:
  - the kernel may not accept a caller-supplied full integration revision;
  - GraphQL/IDE activation remains blocked on authenticated request context,
    origin policy, and a POST-only raw-payload boundary;
  - the internal kernel does not make the existing legacy GraphQL submit/session
    paths safe or canonical; the next adapter slice must contain those paths
    before exposing this processor.
- Initial executable support is deliberately one typed event: HL7v2 ADT A01.

## Align

### Scope in

- Add a storage-neutral, deployment-tenant-bound integration definition loader
  and resolver. Strictly decode exact JSON and require the requested content
  reference to equal the server-owned revision reference.
- Add additive process-result provenance for exact source, profile, and workflow
  revisions; require it for the new preview constructor/validator.
- Reject duplicate destination artifact IDs so workflow action destination
  binding is unambiguous.
- Add a versioned, strict published-workflow YAML grammar with byte, node, depth,
  collection, scalar, document, duplicate-key, alias/anchor/merge/tag, and
  unknown-field limits. Preserve exact immutable YAML bytes defensively.
- Add a pure workflow planner that evaluates type/source/CEL only, preserves
  route/action order, returns stable action identities, never exposes action
  configuration, and owns no handlers, transformer, LLM, logger, metrics,
  tracer, DLQ, or destination client. Require Boolean CEL, cap runtime cost, and
  reject dangerous YAML nesting before the YAML decoder constructs a node tree.
- Compile the exact persisted Source Profile JSON into a narrow runtime profile:
  UTF-8 HL7v2 ADT A01, supported timezone, event classification, missing-PV1
  tolerance, assigning-authority mappings, and identifier reject patterns.
  Reject every unsupported non-default authored knob.
- Harden HL7v2 parsing for one-message input, exact MSH-9 metadata, standards-
  correct DTM offsets/fractions/legacy precision, host-independent fixed offsets,
  message-zone-aware PV1 times, safe source-time precedence, and no parser-clock
  fallback in the strict path while preserving legacy parser behavior.
- Add a preview-only `MessageProcessor` that resolves the server-owned
  integration/profile/workflow revisions, creates one fresh parser per request,
  accepts only exact A01 message types, rebuilds event metadata deterministically,
  strips raw/parser-owned enrichment, creates cataloged diagnostics, plans routes,
  suppresses all deliveries, and validates the final public result.
- Fail closed before loader access for production mode or tenant mismatch.
- Reject empty or over-1 MiB source payloads by their private in-memory length
  before loader access; transport adapters must enforce the same or a lower cap.

### Scope out

- authenticated GraphQL/IDE exposure, OIDC/API authentication, authorization,
  CORS/WebSocket origin activation, or client-generated schema/UI changes;
- changes to, or activation through, existing GraphQL submit and Integration
  Session preview paths; they remain noncanonical and are contained in Slice 1.1c;
- PostgreSQL persistence for integration definition revisions;
- production receipts, idempotency, trace persistence, transactional outbox,
  delivery execution, retries, DLQ, or replay;
- formats or message types beyond a single HL7v2 ADT A01 event;
- workflow transform execution, endpoint/auth/retry/action configuration, or
  mutable workflow/profile pointer resolution;
- retained raw payloads, session sample/run persistence, batch HL7 framing, or
  parser caches.

### Acceptance criteria

- A request can resolve only an operator-owned exact integration revision; a
  self-consistent caller-invented revision is not executable.
- Profile/workflow bytes are strictly compiled and their exact references are
  observable and validated in `ProcessResult`.
- Repeated and concurrent preview of the same request is race-clean and byte-
  deterministic; event identity changes only when source bytes or bound artifact
  revisions change.
- Strict CEL must return a Boolean within its runtime cost budget; static type
  errors, dynamic type errors, and cost exhaustion are cataloged diagnostics.
- Only exact `ADT^A01` and `ADT^A01^ADT_A01` are accepted; A04, prefix-smuggled
  values, multiple MSH segments, unsupported extensions, and blank MSH-10 fail.
- Preview contains no receipt, attempt state, raw payload, parser error text,
  warning text, extracted source text, quality enrichment, secret/action config,
  or executed side effect.
- Matched routes are not marked skipped; invalid CEL produces a bounded code/path
  diagnostic; unmatched routes produce no delivery plan.
- Every delivery plan is bound by destination artifact ID to one exact revision,
  has a stable action ID, and is `suppressed` in preview.
- Existing legacy parser/workflow APIs retain their compatibility behavior; only
  the published/executable grammar is strict.
- The kernel owns no HTTP handler, GraphQL resolver, WebSocket, persistence
  client, destination client, or session adapter and therefore is not remotely
  reachable until the authenticated adapter slice lands.
- Focused tests are proven red before implementation, then focused race,
  PostgreSQL kill-test, full Go, security, docs, MR, and post-merge main/deploy
  pipelines pass.

## Land

### Intended files

- `pkg/integration/contracts.go`
- `pkg/integration/contracts_test.go`
- `pkg/integration/revision.go`
- `pkg/integration/revision_test.go`
- `internal/integration/processor/definition_revisions.go`
- `internal/integration/processor/definition_revisions_test.go`
- `internal/integration/processor/profile_compile.go`
- `internal/integration/processor/profile_compile_test.go`
- `internal/integration/processor/workflow_plan.go`
- `internal/integration/processor/workflow_plan_test.go`
- `internal/integration/processor/adt_a01.go`
- `internal/integration/processor/adt_a01_test.go`
- `internal/integration/processor/message_processor.go`
- `internal/integration/processor/message_processor_test.go`
- `internal/integration/processor/message_processor_integration_test.go`
- `internal/parser/hl7v2/parser.go`
- `internal/parser/hl7v2/parser_hardening_test.go`
- `internal/parser/hl7v2/datetime.go`
- `internal/parser/hl7v2/strict_validation.go`
- `internal/parser/hl7v2/strict_validation_test.go`
- `internal/workflow/published_yaml.go`
- `internal/workflow/published_yaml_test.go`
- `internal/workflow/plan.go`
- `internal/workflow/plan_test.go`
- `.gitlab-ci.yml`
- `ROADMAP.md`
- `.loom/30-implementation-plan-integration-engine-ide-completion.md`
- `.loom/40-decisions.md`
- `docs/STATUS.md`
- `CHANGELOG.md`
- this iteration plan

### Implementation sequence

1. Record red contract/resolver, strict YAML/planner, profile compiler, DTM/A01,
   deterministic processor, side-effect trap, and legacy fail-closed tests.
2. Land additive public provenance/destination invariants and the server-owned
   integration revision resolver.
3. Land strict published workflow parsing plus the pure route planner.
4. Land the supported profile compiler and HL7v2 one-message/DTM hardening.
5. Compose the preview-only processor and validate the kill-test without wiring
   it to any network or legacy session surface.
6. Refactor while focused/race/PostgreSQL suites stay green; independently
   review and ship through terminal MR and main/deploy pipelines.

## Prove

### Red

    go test ./pkg/integration ./internal/workflow \
      ./internal/parser/hl7v2 ./internal/integration/processor
    go test -tags=integration -run TestMessageProcessorPreviewKernel_Postgres \
      ./internal/integration/processor

Current evidence: the public-contract, definition-resolver, published-workflow,
strict-parser/DTM, profile-compiler, and processor tests each entered red before
their corresponding implementation. The PostgreSQL composition proof is a
separate integration verification gate rather than the original unit red.
Adversarial review then reproduced red regressions for legacy action config,
non-Boolean CEL, static type short-circuit, CEL cost exhaustion, pre-decode YAML
depth/quote bypasses, legacy parser timestamp drift, all six TS precisions,
PV1-45 message-zone handling, and host-dependent explicit-offset normalization.

### Green

    go test -race -count=1 ./pkg/integration ./internal/workflow \
      ./internal/parser/hl7v2 ./internal/integration/processor
    POSTGRES_TEST_URL=... go test -tags=integration -count=1 \
      -run TestMessageProcessorPreviewKernel_Postgres \
      ./internal/integration/processor

Current evidence on 2026-07-13:

- focused regular and race suites passed for all four packages;
- the final PostgreSQL 16 race command passed both exact resolver and processor
  v1-after-v2 tests in 2.800 seconds after all parser, CEL, and YAML hardening;
- the proof advanced both mutable pointers to v2, reconstructed fresh stores and
  processors, then showed v1 byte identity, v2 strictness, exact provenance,
  cataloged CEL diagnostics, suppressed deliveries, and no raw sentinel;
- its remote database container and local tunnel were removed and verified gone.

### Broad

    go test -count=1 ./...
    go vet ./...
    golangci-lint run --timeout=10m ./pkg/integration/... \
      ./internal/workflow/... ./internal/parser/hl7v2/... \
      ./internal/integration/processor/...
    make security-vulncheck security-gosec security-npm-audit
    bash scripts/validate-docs.sh

Current evidence on 2026-07-13: the full uncached Go suite and `go vet ./...`
passed; scoped golangci-lint reported zero issues; documentation validation
reported zero warnings; govulncheck found no reachable vulnerabilities; gosec
found no unwaived high-confidence/high-severity findings; UI and SDK npm audits
passed the high/critical gate (three known low UI findings, zero SDK findings).
Three independent final reviews reported no remaining P0/P1 findings across the
processor/security boundary, parser/time semantics plus CI/docs, and workflow/
contract/resource-limit surface. The kernel shipped with Slice 1.1c in MR `!96`;
default-branch pipeline `18621`, matching API/UI images, GitOps rollout, and
public-ingress evidence are recorded in the Slice 1.1c iteration plan.

## Handoff

- Slice 1.1c completed authenticated request-security context, POST-only
  GraphQL/IDE adapters, server-owned static integration registry wiring, and
  parity/live tests that invoke this exact kernel without raw/session
  persistence.
- Slice 1.2 remains the first slice authorized to return a valid production
  result because it owns durable receipts, effective idempotency, trace/outbox
  writes, restart proof, and concurrency semantics.
