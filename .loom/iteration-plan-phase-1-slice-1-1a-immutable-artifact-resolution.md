# RALPH Iteration Plan: Phase 1 Slice 1.1a Immutable Artifact Resolution

**Status**: proving
**Date**: 2026-07-13

## Riskiest assumption + kill-test

**Load-bearing assumption**: the existing PostgreSQL Source Profile and workflow
lifecycle stores can preserve and resolve one exact executable profile revision
and one exact workflow version by immutable ID and content digest after their
mutable current/published pointers advance, without making the future shared
processor depend on GraphQL storage types.

**Kill test**: in a dedicated PostgreSQL integration test that finishes in under
30 minutes, create profile/workflow v1, compute their domain-separated revision
references, advance both current/published pointers to v2, construct fresh store
and resolver instances against the same database, and resolve both v1 references
byte-for-byte. The same test must reject a wrong artifact owner, a changed digest,
a nonexistent revision, and a tenant outside the resolver's configured
single-deployment security domain before returning executable content.

**Failure mode if the assumption is wrong**: Slice 1.1b would either follow
mutable current pointers, duplicate artifact snapshots inside integrations, or
couple the runtime processor to GraphQL DTOs. Any of those outcomes invalidates
the content-addressed foundation and blocks processor/ingress work until the
artifact persistence boundary is redesigned.

**Status**: passed 2026-07-13 (local PostgreSQL 16 proof; CI pending)

Positive evidence: workflow versions already retain immutable opaque IDs and
remain queryable after release pointers move. Disconfirming evidence: Source
Profile creation currently writes no revision, update archives only the former
current row, and no exact current/revision lookup exists.

## Review

- Roadmap milestone: Engine Alpha / Golden Path 001 runtime spine.
- Parent slice: Slice 1.0 established content-addressed public revision/runtime
  contracts in `pkg/integration`.
- Existing workflow lifecycle:
  - versions are immutable rows and environment releases are mutable pointers;
  - PostgreSQL publication does not currently prove the version belongs to the
    requested workflow;
  - `MAX(version_number)+1` is not serialized for concurrent saves.
- Existing Source Profile lifecycle:
  - `source_profiles` is a mutable current row;
  - `profile_revisions` contains only former current values;
  - create has no revision and update does not retain the incoming current value;
  - there is no exact revision or current-revision API.
- Boundary rule: 1.0 remains one logical tenant/security domain per deployment.
  The resolver is explicitly configured for that tenant and fails closed on a
  different tenant; shared row-level multi-tenancy remains outside the 1.0 claim.

## Align

### Scope in

- Add an immutable current-revision pointer to Source Profile persistence and an
  idempotent backfill for existing mutable rows. Install compatibility triggers
  so legacy pods still running during a rolling upgrade also create/advance the
  immutable pointer.
- Make profile creation insert its initial revision transactionally; make update
  lock the current row, insert the incoming revision, and advance the pointer.
- Add exact profile-revision and current-revision lookup APIs. Preserve the
  existing serial revision ID as the immutable reference identity; version
  labels remain display metadata and need not be unique for legacy rows.
- Serialize workflow version allocation per workflow and reject cross-workflow
  publication/version ownership mismatches.
- Introduce a storage-neutral artifact loader boundary and a dependency-light
  resolver under `internal/integration/processor`.
- Compute profile digests over domain-separated canonical JSON and workflow
  digests over domain-separated exact UTF-8 YAML bytes. Reject duplicate profile
  JSON keys, malformed/non-object profile JSON, invalid Unicode, malformed
  references, not-found, ownership mismatch, and digest mismatch. Normalize
  equivalent JSON number spellings independently of PostgreSQL `JSONB` output.
- Return defensive copies so resolved executable content cannot mutate store or
  cache state.
- Add one required CI job backed by PostgreSQL 16 that runs the restart/v1-after-
  v2 kill-test independently of the existing soft-failing broad integration job.

### Scope out

- Parsing GraphQL profile JSON into the full runtime `pkg/profile.SourceProfile`;
- YAML-to-`workflow.Workflow` construction or cache changes;
- `MessageProcessor`, parser invocation, workflow planning, GraphQL/session
  migration, or production execution;
- durable receipts, idempotency, event/outbox persistence, or delivery;
- draft/approve/publish semantics for Source Profiles;
- shared-hosting tenant columns, row-level security, or cross-tenant listing;
- GraphQL schema/codegen/UI exposure of revision IDs and digests.

### Acceptance criteria

- `CreateProfile` and `UpdateProfile` leave `source_profiles.current_revision_id`
  pointing at an immutable row containing the exact current config.
- `InitSchema` upgrades existing rows exactly once and is idempotent.
- Exact lookup uses both profile ID and revision ID; workflow lookup uses both
  workflow ID and version ID.
- Profile JSON key order and insignificant whitespace do not change its digest;
  equivalent number notation also remains stable across `JSONB`; semantic
  mutation does change it. Workflow byte mutation does change its digest.
- A legacy create/update issued after migration still creates and advances the
  immutable current profile revision during a rolling deployment.
- A profile/workflow v1 pair resolves unchanged from fresh objects after v2 is
  current/published.
- Wrong owner, bad digest, nonexistent revision, malformed JSON/reference, and
  wrong deployment tenant fail closed.
- Concurrent workflow version saves cannot allocate the same version number.
- The dedicated PostgreSQL CI proof is required (`allow_failure: false`).
- Focused tests are proven red before implementation, then focused race, full Go,
  security, docs, MR, and post-merge main pipelines pass.

## Land

### Intended files

- `internal/api/graphql/store/profile_store.go`
- `internal/api/graphql/store/profile_store_integration_test.go`
- `internal/api/graphql/store/workflow_lifecycle_store.go`
- `internal/api/graphql/store/workflow_lifecycle_pg_store.go`
- `internal/api/graphql/store/workflow_lifecycle_store_test.go`
- `internal/api/graphql/store/workflow_lifecycle_pg_store_integration_test.go`
- `internal/api/graphql/store/artifact_revision_loader.go`
- `internal/api/graphql/store/artifact_revision_loader_test.go`
- `internal/integration/processor/revisions.go`
- `internal/integration/processor/revisions_test.go`
- `internal/integration/processor/revisions_integration_test.go`
- `.gitlab-ci.yml`
- `.loom/30-implementation-plan-integration-engine-ide-completion.md`
- `.loom/40-decisions.md`
- `.loom/iteration-plan-phase-1-slice-1-0-foundation-contracts.md`
- `docs/STATUS.md`
- `CHANGELOG.md`
- this iteration plan

### Implementation sequence

1. Write focused store, digest, resolver, ownership, concurrency, and restart
   tests; preserve their non-zero results.
2. Repair profile revision persistence and workflow ownership/allocation.
3. Implement the neutral loader plus content-verifying resolver.
4. Run the live PostgreSQL kill-test locally and in a new blocking CI job.
5. Refactor while focused/race suites remain green, independently review, and
   ship through terminal MR and main pipelines.

## Prove

### Red

    go test ./internal/integration/processor ./internal/api/graphql/store
    go test -tags=integration -run 'TestArtifactRevisionResolver|TestPostgresProfileStore' \
      ./internal/integration/processor ./internal/api/graphql/store

Observed evidence:

- profile integration tests failed to compile because exact/current revision APIs
  and `CurrentRevisionID` did not exist;
- the processor package reported `no non-test Go files` and loader tests reported
  undefined `NewArtifactRevisionLoader`;
- PostgreSQL accepted a cross-workflow publication, and 7 of 8 forced concurrent
  version saves failed the unique constraint;
- adversarial review tests proved owner-only workflow lookup and malformed JSON
  Unicode were accepted before the fixes;
- scientific and expanded JSON numbers produced different digests, and a
  post-migration legacy insert left `current_revision_id` null.

### Green

    go test -race -count=1 ./internal/integration/processor ./internal/api/graphql/store
    POSTGRES_TEST_URL=... go test -tags=integration -count=1 \
      ./internal/integration/processor ./internal/api/graphql/store

Current evidence:

- focused unit/race suites, vet, and package lint pass;
- resolver statement coverage is 82.4%;
- PostgreSQL 16 race tests pass for profile lifecycle/backfill/rollback,
  rolling-upgrade legacy writes, workflow ownership and eight-way concurrent
  version allocation, and
  `TestArtifactRevisionResolver_PostgresV1SurvivesV2`;
- the kill-test constructs fresh database/store/adapter/resolver objects, proves
  the workflow pointer advanced to v2, resolves v1 unchanged, and rejects wrong
  tenant/owner/digest/not-found cases;
- temporary Docker-context container and SSH tunnel cleanup was verified.

### Broad

    go test -count=1 ./...
    go vet ./...
    golangci-lint run --timeout=10m ./internal/integration/processor/... \
      ./internal/api/graphql/store/...
    make security-vulncheck security-gosec security-npm-audit
    bash scripts/validate-docs.sh

Current evidence: all broad commands pass; GitLab reports the CI/CD YAML valid;
`govulncheck` reports zero reachable vulnerabilities; `gosec` reports no
unwaived high-confidence/high-severity findings; npm passes the high/critical
threshold (UI: three low findings, SDK: zero); docs validation reports zero
warnings. Two independent final reviews report no remaining P0/P1 findings.

## Handoff

- Slice 1.1b consumes only this resolver boundary, converts the exact stored
  artifacts into runtime profile/workflow objects, and implements deterministic,
  side-effect-free HL7v2 preview evaluation.
- Slice 1.2 remains the first slice authorized to return a valid production
  result because it owns durable receipts, effective idempotency, outbox writes,
  restart proof, and concurrency semantics.
