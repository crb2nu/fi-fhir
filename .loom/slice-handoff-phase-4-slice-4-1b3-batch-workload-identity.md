# RALPH Slice Handoff

## Slice Summary

- Milestone: Phase 4 Slice 4.1 — identity, authorization, and PHI policy
- Slice: 4.1b3 — batch workload identity and trusted receipt provenance
- Status: complete

## What Landed

- Key changes:
  - Added an optional `workload` block to the immutable, content-addressed batch
    source revision. It names one canonical service subject and its grants, and
    it is the identity under which every object that source ingests submits.
    Nothing observed on the remote side — object keys, remote directories,
    remote metadata, or MSH content — can select or influence it.
  - Moved the shared fail-closed `authorization.AuthorizeSubmission` decision to
    the connector boundary in `PollOnce`, immediately after deployed-release
    binding validation and before `provider.List`, the PostgreSQL lease claim,
    stream opening, artifact loading, and every durable write. The same decision
    still runs per message before the processor and again inside
    transaction-scoped runnable admission, and all three now evaluate one
    security context built once per poll so they cannot disagree.
  - Kept binding all-or-nothing per source. An absent `workload` block preserves
    the deployment-fixed `FI_FHIR_BATCH_PRINCIPAL_ID` principal and the
    server-issued `integration:batch` grant, and existing source revisions keep
    their exact digest because the block is omitted from the canonical digest
    input when nil. Grant order is canonicalized so presentation cannot move a
    digest. `resolvePrincipal` never falls back from bound mode.
  - Replaced remote object modification time as trusted receipt provenance. The
    authoritative `received_at` is now the server-owned custody timestamp
    written once when an exact object version is first durably admitted
    (`integration_batch_objects.created_at`), so it is stable across lease
    reclaim, worker restart, and checkpoint resume.
  - Made content provenance a SHA-256 digest computed over the exact bytes
    streamed during admission. `MessageReader` now exposes each message's raw
    byte interval, the runner hashes it, and the marshaled hash state is
    persisted with every checkpoint so a resumed poll continues the same hash
    instead of trusting a re-read. The pre-archive full re-read must agree; a
    disagreement quarantines the object with `DIGEST_MISMATCH`.
  - Pinned the S3 entity tag alongside the exact version ID and re-verified both
    at every read, archive, and delete.
  - Renamed `object_modified_at` to `remote_modified_at_advisory` and added
    `object_version`, `object_etag`, and `digest_state` in migration
    `0002_batch_provenance.sql`, with column comments stating the trust level.
    The provenance CHECK is `NOT VALID` so rows admitted before this revision
    stay visibly distinguishable rather than being given invented provenance.
  - Added `FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY` so a deployment refuses to
    start if the mounted source document drops back to compatibility mode,
    forwarded the complete batch runtime contract through Docker Compose, and
    extended `scripts/check-runtime-config.sh` with a required batch compose
    check.
  - Extended `make batch-ingestion` and the required `test:batch-ingestion` job
    to discover and run both batch runtime proofs.
- Key files:
  - `internal/integration/batch/identity.go` (new)
  - `internal/integration/batch/provenance.go` (new)
  - `internal/integration/batch/migrations/0002_batch_provenance.sql` (new)
  - `internal/integration/batch/source.go`, `provider.go`, `s3.go`, `sftp.go`
  - `internal/integration/batch/reader.go`, `service.go`, `store.go`
  - `internal/integration/batch/identity_test.go` (new)
  - `internal/integration/batch/identity_integration_test.go` (new kill test)
  - `cmd/fi-fhir/batch_runtime.go`, `cmd/fi-fhir/batch_runtime_test.go`,
    `cmd/fi-fhir/main.go`
  - `docker-compose.yaml`, `.env.example`, `scripts/check-runtime-config.sh`
  - `docs/operations/BATCH-INGESTION.md`,
    `docs/operations/PRODUCTION-HARDENING.md`
  - `Makefile`, `.gitlab-ci.yml`, `CHANGELOG.md`
- Validation results:
  - Kill test `TestBatchIngestion_PostgresS3SFTPWorkloadIdentityProvenance`
    passed with `-race` against PostgreSQL 16, real MinIO, and a real in-process
    SSH/SFTP server, together with the existing Slice 2.4 proof
    `TestBatchIngestion_PostgresS3SFTPKillResumeArchive`.
  - Five independent negative controls each failed the kill test, proving every
    assertion is load-bearing:
    1. Deployment-fixed principal in bound mode → admitted subjects collapsed to
       `map[batch-identity-s3-principal:2 batch-identity-sftp-principal:2]`.
    2. No connector-boundary decision (per-message and transaction-scoped checks
       only) → `integration_batch_objects = 3 after denial, want 2`; the denied
       source created a lease and checkpoint row.
    3. Remote modification time as received-at → `canonical received_at not
       aligned with custody time: 4`.
    4. Ignoring per-identity grants and always projecting the server-issued
       grant → `ungranted poll = 1, <nil>; want 0, ErrUnavailable`.
    5. Streaming digest over normalized payload bytes rather than the raw byte
       interval → the pre-archive cross-check quarantined the object and
       `content_digest` stayed empty.
  - `go test -race ./...` passed with no failures.
  - `gofmt` clean, `golangci-lint run` reported 0 issues, `go vet ./...` and
    `go vet -tags=integration ./internal/integration/...` clean.
  - `make docs-validate`, `bash scripts/check-runtime-config.sh` (0 required
    failures, including the new batch compose check), `go mod verify`,
    `git diff --check`, and `scripts/docs-status.sh --check-drift` (exit 0) all
    passed.
  - `gosec` with the exact CI exclusion set reported 0 issues across 263 files.
    `govulncheck ./...` found no vulnerabilities in called code.
  - The checked-in golden batch source revision still decodes to its exact
    pinned digest `sha256:d8b2381b…61c3`, so no existing deployment is
    invalidated.

## Provenance Model

| Fact | Trust | Source |
| --- | --- | --- |
| `received_at` | trusted | Server-owned custody timestamp (`integration_batch_objects.created_at`) written once on first durable admission of an exact object version |
| `content_digest` | trusted | SHA-256 over the exact bytes streamed during admission, resumed across checkpoints, cross-checked against the pre-archive re-read |
| `object_version` | trusted | Exact S3 version ID, or the SFTP synthetic change-detection version; re-verified at every read, archive, and delete |
| `object_etag` | trusted | S3 entity tag observed at listing and re-verified at every read, archive, and delete; empty for SFTP |
| `remote_modified_at_advisory` | **advisory** | Remote-controlled modification time; retained for diagnostics and as a change-detection input to the SFTP synthetic version only |

## What Is Still Open

- Remaining acceptance criteria: the required merge-request pipeline and the
  exact post-merge main pipeline must reach terminal green before the RALPH task
  closes.
- Known issues: no implementation defect is known. Two deliberate limits:
  - The workload subject is recorded durably only in
    `integration_receipts.principal_json`, written under transaction-scoped
    admission. A second copy in `integration_batch_objects` was rejected because
    it would create a divergence risk against the authorization record.
  - The resumable streaming digest depends on `crypto/sha256` implementing
    `encoding.BinaryMarshaler`/`BinaryUnmarshaler`. Unusable continuation state
    releases the lease with `DIGEST_STATE_LOST` and retries rather than
    fabricating a digest.
- Dependencies: S3 provenance requires bucket versioning (already required by
  Slice 2.4) plus an entity tag returned by both `ListObjectVersions` and
  `HeadObject`. MinIO satisfies both in the pinned CI release.
- Operational consequence: because the `workload` block is part of the
  content-addressed source revision, changing a subject or a grant requires a
  new source revision, a new integration definition revision, and a lifecycle
  redeploy. That is deliberate (tamper-evident, digest-pinned) but it is not a
  hot-reload path. The kill test exercises exactly this repair path.
- Migration note: `0002_batch_provenance` renames a column and adds a `NOT VALID`
  CHECK. Rows admitted before it keep empty `object_version`/`object_etag`
  values; they are not retroactively given invented provenance.

## Next Actions

1. 4.1c: destination-scoped identity and secret resolution for the first durable
   HTTPS consumer. Delivery currently resolves destination credentials without a
   destination-scoped authorization decision, which is the last remaining
   transport boundary in Slice 4.1.
2. Control-plane authorization (GraphQL mutations), PHI retention/TTL/export
   controls, and immutable security audit storage.
3. Consider promoting `remote_modified_at_advisory` to an operator-visible
   staleness signal (alerting on objects whose advisory time is wildly divergent
   from custody time) once the observability slice lands. It must stay outside
   every trust decision.

## Context Links

- Relevant docs/specs:
  - `.loom/iteration-plan-phase-4-slice-4-1b3-batch-workload-identity.md`
  - `.loom/30-implementation-plan-integration-engine-ide-completion.md`
  - `.loom/slice-handoff-phase-4-slice-4-1b2-mllp-certificate-identity.md`
  - `.loom/iteration-plan-phase-2-slice-2-4-batch-ingestion.md`
  - `.loom/40-decisions.md`
  - `docs/operations/BATCH-INGESTION.md`
  - `docs/operations/PRODUCTION-HARDENING.md`
