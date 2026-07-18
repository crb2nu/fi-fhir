# RALPH Iteration Plan — Phase 3 Slice 3.4 Publish and Deploy

**Status**: implementation complete; landing gates pending
**Date**: 2026-07-18

## Riskiest assumption + kill-test

**Load-bearing assumption**: an immutable Integration Session can prove that an
already-validated production lifecycle definition contains byte-equivalent
profile and workflow artifacts, sign one bounded manifest over those exact
production references plus session fixtures and expectations, and later approve
and deploy only that definition after verifying the detached signature against a
configured Ed25519 trust root.

**Kill test**: in the required PostgreSQL 16 Integration Session gate, create a
session profile revision, successful runs, workflow revision, and simulation;
create production profile/workflow revisions with the same content and a
validated lifecycle definition that references them; publish and sign the
bundle; reconstruct all stores and services; advance both session drafts and
production current pointers; then verify the stored publication and deploy it.
The final runnable binding must still contain the original production profile and
workflow references. The same test must reject a content-mismatched production
artifact, retained-raw fixture, cross-session simulation/run, altered manifest,
altered signature, untrusted key, wrong definition revision, stale optimistic
version, and approval/deployment without a non-empty actor reason. No GitOps,
network, transform, action, or destination handler may run.

**Failure mode if the assumption is wrong**: the IDE could sign browser/session
draft identities that production cannot resolve, or approve a mutable/current
artifact rather than the tested bytes. A green UI flow would then create false
deployment evidence and invalidate the IDE Beta release gate.

**Status**: unit/adversarial and full dependency-free gates pass. PostgreSQL
restart coverage is implemented but awaits CI because the local environment has
neither Docker nor `POSTGRES_TEST_URL`.

### Positive and disconfirming evidence

- Positive: the lifecycle catalog already persists immutable definition and
  release records, requires current validation, uses optimistic versions, and
  resolves only deployed exact revisions.
- Positive: session runs and workflow simulations already persist exact session
  revision IDs/digests and survive PostgreSQL reconstruction.
- Disconfirming: session artifact digests are plain SHA-256 with opaque revision
  IDs, while production profile references require canonical JSON,
  domain-separated digests, and positive integer revision IDs. Directly copying
  a session reference into a production definition is invalid; publication must
  verify production artifact bytes through the existing resolver boundary and
  record both identities.
- Disconfirming: the current profile review passes the same mutated object as
  both `original` and `draft`, and the unbound change-summary textarea never
  reaches `profile_revisions.change_summary`.

## Review

- Roadmap milestone: IDE Beta — restart-safe session authoring through
  publish/deploy.
- Spec sections: durable integration IDE, governance audit requirements, signed
  bundle definition, profile-drift golden journey, and Slice 3.4 in the active
  implementation plan.
- Prior decisions to preserve:
  - use the existing closed lifecycle graph and optimistic snapshot rather than
    creating a second deployment state machine;
  - production artifact digests remain the domain-separated references produced
    by `internal/integration/processor`;
  - retained raw PHI is not exported or promoted implicitly;
  - the session path remains feature-gated and temporarily requires
    `graphql:operator` until Phase 4 authorization;
  - production GitOps activation remains an explicit, separately reviewed
    operation.

## Align

### Slice name

Signed tested-revision publication and exact lifecycle deployment.

### Scope in

- Repair Source Profile review so the diff compares an immutable loaded snapshot
  with the edited draft and the required change summary is stored with the new
  revision and attributed to the authenticated actor.
- Add an append-only, versioned Integration Session publication record containing
  exact session and production artifact references, fixture digests, bounded
  expected diagnostic/event/route/transform/action results, an exact canonical
  manifest, domain-separated manifest digest, Ed25519 key ID, and detached
  signature.
- Require a successful profile run set and one workflow simulation over the same
  ordered run IDs. Reject retained-raw samples from publication.
- Load the selected lifecycle definition and production profile/workflow bytes,
  verify exact session-content equivalence with the production digest rules, and
  allow publication only while that exact definition is validated.
- Verify the detached signature against a configured trust root before approval
  and again before deployment.
- Reuse lifecycle approval, immutable release publication, and deploy transitions
  with exact definition/revision identity and expected-version checks. Make a
  retry from the intermediate `published` state safe if deploy fails after the
  release record commits.
- Expose list/publish/approve/deploy operations and immutable evidence through
  GraphQL and a bounded Workflow Builder publication panel.
- Persist publications in PostgreSQL migration 3 and include them in restart-safe
  session export snapshots.

### Scope out

- Creating or editing source, destination, secret, schedule, capacity, or health
  policy bindings in the session UI;
- silently promoting session artifacts into mutable/current production stores;
- connection validation itself (publication consumes an already-validated exact
  lifecycle definition);
- Kubernetes/Flux/Helm mutations, production feature-gate activation, staged or
  canary rollout, rollback UI, health reporting, pause/resume/retire controls;
- fine-grained RBAC, key rotation/expiry, multiple trust roots, HSM/KMS signing,
  raw-retention expiry, and cross-replica session stream fanout;
- executing transforms or actions during publication.

### Acceptance criteria

- A profile review renders real additions/removals against the loaded profile,
  requires a non-empty bounded summary, sends it to GraphQL, and revision history
  returns the same summary plus authenticated actor.
- Publication rejects incomplete, failed, cross-session, retained-raw, or
  mismatched run/simulation evidence.
- The manifest is deterministic for its stored facts, bounded, contains no event
  payload, raw sample, transformed value, action configuration, credential, or
  secret, and records both session identities and verified production refs.
- PostgreSQL allocates publication versions serially, stores exact manifest bytes
  and detached signature, and rejects update/delete of publication records.
- Signature verification fails closed for any byte/signature/key change and is
  mandatory before lifecycle approval or deployment.
- Approval/deployment operate only on the manifest's exact lifecycle definition;
  stale version or a different definition/revision is rejected.
- Restart plus current-pointer advancement cannot change the published evidence
  or deployed v1 runnable binding.
- Deployment creates no GitOps, network, workflow-handler, or destination side
  effect; only the existing lifecycle catalog state changes.
- Backend/UI GraphQL generation is deterministic and all required session,
  lifecycle, UI, race, lint, security, docs, MR, and post-merge main gates pass.

### Dependencies and blockers

- Existing session store, workflow simulator, lifecycle catalog, profile store,
  workflow lifecycle store, and processor artifact resolver are available.
- Publication capability requires an Ed25519 PKCS#8 private-key file and trusted
  public-key file. Session authoring remains available when they are absent, but
  publish/approve/deploy fail closed as unavailable.
- The target definition must already exist in `validated` state with current
  connection evidence.

## Land

### Planned file areas

- `internal/integration/session/` publication types, signer/verifier, service,
  store methods, PostgreSQL migration 3, and restart/adversarial tests.
- `internal/integration/lifecycle/` narrow interface support or idempotent exact
  promotion helpers where required.
- `internal/api/graphql/schema.graphql`, resolver wiring, generated backend
  models, and resolver tests.
- `cmd/fi-fhir/preview_runtime.go` and serve wiring for catalog and bounded key
  loading.
- `ui/src/lib/features/workflows/` publication GraphQL documents, panel, and
  component tests.
- `ui/src/lib/features/profiles/`, profile GraphQL input/store tests, and profile
  persistence tests for real diff and change summaries.
- `.loom/`, `docs/operations/INTEGRATION-SESSIONS.md`, `docs/STATUS.md`, and
  `CHANGELOG.md`.

### Implementation steps

1. Write failing profile diff/summary tests and publication signer/manifest
   invariants.
2. Add immutable publication types, memory-store behavior, verifier, and exact
   evidence validation.
3. Add PostgreSQL migration/store implementation and the restart/tamper kill
   test.
4. Adapt the existing lifecycle and artifact-resolution boundaries for exact
   approval/deployment without broadening production execution.
5. Add fail-closed runtime key/catalog composition and GraphQL operations.
6. Add the Workflow Builder publication panel and repair Profile review/summary.
7. Regenerate backend/UI GraphQL artifacts, refactor under focused tests, and
   complete broad verification.

## Prove

### Tests to run

- Focused Go unit/race tests for session publication, lifecycle transitions,
  GraphQL resolver authorization/projection, and profile persistence.
- Required PostgreSQL 16 `make integration-session` restart/tamper/immutability
  gate plus deployment lifecycle integration tests.
- Full `go test ./...` and `go test -race` on changed packages.
- UI component/store tests, full UI suite, Svelte check, and production build.
- Backend and UI GraphQL generation followed by clean-diff checks.

### Lint/static/security checks

- `go vet ./...`
- `golangci-lint run ./...`
- UI ESLint and stylelint
- configured security gates and raw-PHI/secret sentinel scans
- `make docs-validate`
- `git diff --check`

### CI checks

- Required Integration Session and deployment lifecycle PostgreSQL jobs.
- Benchmark job, API/UI generation/lint/tests, build/image scans, and security
  jobs.
- Terminal MR pipeline, auto-merge, and post-merge `main` pipeline.

## Handoff/Harvest

- Update the active implementation plan, product spec status, decision log,
  operations guide, component status, and changelog.
- Record signature/trust, exact artifact verification, and lifecycle reuse
  decisions in agent context.
- Produce `.loom/slice-handoff-phase-3-slice-3-4-publish-deploy.md` with exact
  test and CI evidence.
- Next-slice candidate: Phase 4 Slice 4.1 identity, authorization, and PHI policy.

### Local verification result

- `go test ./...`: passed.
- `go vet ./...`: passed.
- UI Svelte check: 0 errors; 9 pre-existing unused-selector warnings.
- UI ESLint and TypeScript checks: passed.
- UI suite: 642 passed, 3 skipped; 75 files passed, 1 skipped.
- Focused publication/profile-diff tests: passed.
- Backend and UI GraphQL regeneration: byte-stable.
- `git diff --check`: passed.
- PostgreSQL integration: not run locally; Docker socket and external test URL
  were both unavailable. The migration/restart/append-only assertions are in the
  tagged session integration test and remain a required CI gate.
