# Phase 3 Slice 3.4 Handoff — Signed Publish and Deploy

## Outcome

Slice 3.4 closes the durable IDE lifecycle. Integration Session can append a
versioned, signed manifest over exact tested profile/workflow revisions, fixture
digests, and bounded expected results after proving those bytes match one exact
validated production definition. Workflow Builder exposes signed publication,
approval, and deployment; Source Profile review now uses a real immutable
baseline and durable actor-attributed change summary.

## Safety boundary

- Session and production identities are both recorded; production refs are
  recomputed and verified through the production artifact resolver.
- Manifests contain digests and expected identities, not raw/event payloads,
  transformed values, action configuration, credentials, or secrets.
- Retained-raw fixtures and any production content mismatch fail publication.
- Ed25519 signatures are verified against the configured trust root before
  approval and again before deployment.
- Existing lifecycle validation freshness, optimistic versions, immutable
  releases, and deployed-only resolution remain authoritative.
- Publication performs no transform, action, destination, network, or GitOps
  operation. Production GitOps activation remains separately reviewed.

## Runtime configuration

Publication is enabled only when signing key ID, PKCS#8 Ed25519 private-key file,
and matching PKIX public trust-root file are all configured. Partial or mismatched
configuration fails startup. Session authoring/simulation remains available when
all publication key settings are absent.

## Verification

- `go test ./...`: passed.
- `go vet ./...`: passed.
- Publication adversarial tests cover content drift, retained raw, manifest
  tampering, mismatched trust root, exact approval, and approve/publish/deploy.
- UI Svelte check: 0 errors; 9 pre-existing warnings.
- UI ESLint and TypeScript checks: passed.
- UI tests: 642 passed, 3 skipped; 75 files passed, 1 skipped.
- Backend and UI GraphQL generation is byte-stable.
- `git diff --check`: passed.
- PostgreSQL restart/append-only coverage is implemented in the tagged session
  integration test. It was not runnable locally because neither Docker nor
  `POSTGRES_TEST_URL` was available and must pass in CI before merge.

## Remaining work

- Phase 4 Slice 4.1: fine-grained identity, authorization, key rotation/expiry,
  and broader PHI policy enforcement.
- Production GitOps activation, staged rollout, rollback UI, and health controls
  remain separately reviewed operations work.
- Durable cross-replica session stream fanout/replay remains Phase 4 work.

## Landing evidence

- MR `!124` pipeline `19939` passed and the slice merged as `84d2fab2` on
  2026-07-18.
- Post-merge main pipeline `19944` passed on the same merge commit.
