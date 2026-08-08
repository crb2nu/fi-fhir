# RALPH Iteration Plan — Phase 4 Slice 4.1b3 Batch Workload Identity and Trusted Receipt Provenance

## Review

- Roadmap milestone: Phase 4 Slice 4.1 — enforce identity, authorization, and
  PHI policy.
- Spec sections: `.loom/20-product-spec-integration-engine-ide-completion.md`
  identity and isolation contracts; `.loom/30-implementation-plan-integration-
  engine-ide-completion.md` Phase 4 Slice 4.1.
- Prior decisions to preserve:
  - Tenant, integration revision, source, action, and resource identity are
    server-owned. Senders cannot assert them through headers, certificates,
    object keys, MSH fields, or any other in-band data.
  - Slice 4.1b1 added exactly one fail-closed `integration.submit` decision over
    exact tenant, revision, and source. Slice 4.1b2 bound MLLP transport
    identity to it. New transports reuse that decision rather than forking a
    parallel policy path.
  - The batch source revision is immutable and content-addressed; the deployed
    lifecycle release pins its exact digest.
  - Durable admission is the last authorization boundary before receipt, event,
    attempt, and outbox writes.
  - Slice 2.4 invariants stay intact: exact S3 object versions, PostgreSQL
    object-version leases, byte/message checkpoints written only after durable
    admission, digest-addressed archives verified before source deletion, SFTP
    `known_hosts` pinning, and symlink rejection.
  - Checked-in GitOps activation remains a separate reviewed operation.

## Align

- Slice name: batch (S3/SFTP) connector workload identity bound to the shared
  submit authorization decision, plus a trusted receipt provenance model that
  removes remote object modification time from the trust boundary.
- Scope in:
  - Add an optional `workload` block to the immutable batch source revision: one
    canonical service subject plus its grants. The block is the identity under
    which every object that source ingests submits. It is omitted from the
    canonical digest input when absent, so existing revisions keep their exact
    digest.
  - Resolve the submitting principal from the source revision only. Object keys,
    remote paths, remote metadata, and MSH content can never select, influence,
    or impersonate an identity.
  - Evaluate the shared `authorization.AuthorizeSubmission` decision at the
    connector boundary in `PollOnce`, immediately after deployed-release binding
    validation and *before* listing, leasing, opening, reading, artifact
    loading, or any durable write; again per message before the processor loads
    artifacts; and again inside transaction-scoped runnable admission.
  - Keep identity binding all-or-nothing per source. With no `workload` block
    the runner keeps the deployment-fixed `FI_FHIR_BATCH_PRINCIPAL_ID` principal
    and the server-issued `integration:batch` grant. With a `workload` block the
    deployment-fixed principal is never used, and there is no per-object
    fallback. Add `FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY` so a deployment can
    refuse to start in compatibility mode.
  - Replace remote object modification time as trusted receipt provenance:
    - Authoritative `received_at` becomes the server-owned custody timestamp
      recorded when the object row is first durably created under the exact
      (tenant, source, source revision, object version) key. It is stable across
      lease reclaim, worker restart, and checkpoint resume.
    - Content provenance becomes a SHA-256 digest computed over the exact bytes
      streamed during admission, resumed across checkpoints from persisted hash
      state, and cross-checked against a full re-read before archive.
    - S3 additionally pins the exact version ID and the ETag observed at listing
      and re-verified at every read, stat, archive, and delete.
    - Remote modification time is retained only as advisory metadata. The field,
      the column, and the docs say so; it participates in no trust or audit
      decision. It remains an input to the SFTP synthetic object version, which
      is change detection, not provenance.
  - Forward the complete batch runtime contract through Docker Compose and
    extend the runtime-config regression check to cover batch variables.
  - Document the batch identity and provenance contract in
    `docs/operations/PRODUCTION-HARDENING.md`.
- Scope out:
  - Destination-scoped identity, secret resolution, and delivery authorization
    (Slice 4.1c).
  - Token issuance/introspection, SPIFFE trust bundle rotation, and cloud
    provider workload federation (IRSA/Workload Identity) transport.
  - GraphQL control-plane actions (a sibling slice owns 4.2).
  - MLLP/HTTP behaviour changes beyond what sharing the authorization decision
    requires.
  - Immutable security audit storage, PHI retention/TTL/export controls, and
    GitOps activation.
- Acceptance criteria:
  - Two batch sources configured with two distinct workload identities produce
    two distinct verified subjects observable at transaction-scoped admission,
    with object-derived fields unable to cross identities.
  - A source whose configured identity lacks a recognized submit grant stops
    before artifact loading, creates zero durable records of any class, and
    leaves no lease or checkpoint state that could poison a later retry.
  - Compatibility mode (no `workload` block) preserves current Slice 2.4 fixture
    behaviour and the exact checked-in source digests.
  - A fixture object carrying an absurd remote modification time produces a
    receipt whose `received_at` is the server-owned custody time, not the remote
    value, and the remote value appears only in an advisory-labelled field.
  - The S3 path records the exact version ID plus a verified ETag and a
    streaming digest; the SFTP path records a streaming digest.
- Dependencies/blockers:
  - The `workload` block lives inside the content-addressed source revision, so
    changing a subject or its grants requires a new source revision, a new
    definition revision, and a lifecycle redeploy.
  - S3 provenance requires bucket versioning (already required by Slice 2.4) and
    an ETag returned by both `ListObjectVersions` and `HeadObject`.
  - Resumable streaming digests depend on `crypto/sha256` implementing
    `encoding.BinaryMarshaler`/`BinaryUnmarshaler`.

## Riskiest assumption + kill-test

**Load-bearing assumption**: the batch runner can (a) submit under a
source-declared workload subject that no object, key, or message can influence,
(b) refuse an ungranted identity early enough that no lease, checkpoint, audit
row, receipt, event, or outbox row is created and a later grant fix still
processes the object cleanly, (c) ground receipt provenance in server-owned and
content-verified facts so a spoofed remote modification time changes nothing
trusted, and (d) do all of that without changing existing source revision
digests or Slice 2.4 fixture behaviour.

**Kill test**: `TestBatchIngestion_PostgresS3SFTPWorkloadIdentityProvenance`
extends the required batch integration proof (PostgreSQL 16, MinIO, and a real
in-process SSH/SFTP server) with the production durable processor,
transaction-scoped runnable admission, and the real deployed lifecycle. It
asserts:

1. Two sources — one S3, one SFTP — configured with `svc-batch-east` and
   `svc-batch-west` produce exactly those two distinct subjects in the durable
   receipts written under transaction-scoped admission, while both objects carry
   identical MSH sending-application/facility values and the S3 object key
   embeds the *other* source's subject. Identity does not follow the bytes.
2. A source whose workload grants omit any recognized submit grant returns a
   fail-closed error from `PollOnce`, loads no artifact, and leaves every
   durable record class (`integration_batch_objects`, `integration_batch_audit`,
   `integration_receipts`, `integration_canonical_events`,
   `integration_delivery_outbox`) exactly as it was. After the grant is repaired
   in a new source revision the same object is admitted normally, proving the
   denial poisoned no checkpoint.
3. Compatibility mode (no `workload` block) admits the Slice 2.4 fixture under
   the deployment-fixed principal and the server-issued `integration:batch`
   grant, and the Slice 2.4 source revisions keep their exact digests.
4. An object whose remote modification time is set decades in the past produces
   a receipt whose canonical `received_at` equals the server-owned custody
   timestamp recorded in `integration_batch_objects.created_at`, with the spoofed
   value present only in `remote_modified_at_advisory`. The S3 row records the
   exact version ID and the ETag observed at read; both rows record a digest
   whose recorded source is the streaming path, and the streaming digest matches
   an independent hash of the fixture bytes.

**Failure mode if the assumption is wrong**: fi-fhir would keep attributing every
batch admission to one shared deployment principal, and every downstream audit,
retention, and lineage decision would keep trusting a timestamp a remote sender
controls. Extending identity to destinations would then inherit both defects.

Positive evidence: AWS S3 documents version ID plus ETag as the exact-object
identity for conditional reads:
<https://docs.aws.amazon.com/AmazonS3/latest/userguide/versioning-workflows.html>.
The Go standard library documents `crypto/sha256` digests as marshalable
intermediate state via `encoding.BinaryMarshaler`:
<https://pkg.go.dev/crypto/sha256>.
Disconfirming evidence: RFC 913 §3 and the SFTP protocol drafts expose
modification time as a client-settable attribute (`SSH_FXP_SETSTAT`), which is
exactly why remote mtime cannot be trusted provenance:
<https://datatracker.ietf.org/doc/html/draft-ietf-secsh-filexfer-02#section-6.7>.

**Status**: passed on 2026-08-08. Two bound sources kept `svc-batch-east` and
`svc-batch-west` distinct in the durable receipts written under transaction-scoped
admission while both objects carried identical MSH sending
application/facility naming `svc-batch-west` and the S3 key was
`incoming/svc-batch-west/adt.hl7`. An ungranted subject returned
`ErrUnavailable` with all five durable record classes unchanged and the source
object untouched, and the same object admitted cleanly once the grant was
repaired in a new source revision. Compatibility mode admitted under
`batch-identity-compat-principal` with auth method `batch-s3` and the
server-issued `integration:batch` grant. An SFTP object whose modification time
was set to 1994 produced canonical events whose `received_at` equals the
server-owned custody timestamp, with the spoofed value present only in
`remote_modified_at_advisory`; the S3 row recorded an exact `version:` version ID
and a normalized entity tag, and both rows recorded the streaming digest of the
fixture bytes. Five negative controls — a deployment-fixed principal in bound
mode, no connector-boundary decision, remote modification time as received-at,
ignored per-identity grants, and a streaming digest over normalized rather than
raw bytes — each failed the test. The checked-in golden batch source revision
retained its exact pinned digest.

## Land

- Planned file areas:
  - `internal/integration/batch/identity.go` (new workload identity contract)
  - `internal/integration/batch/source.go` (immutable workload block)
  - `internal/integration/batch/provider.go` (advisory mtime, ETag, provenance)
  - `internal/integration/batch/s3.go`, `internal/integration/batch/sftp.go`
  - `internal/integration/batch/reader.go` (exact raw byte interval per message)
  - `internal/integration/batch/service.go` (connector-boundary decision,
    custody-time envelope, streaming digest)
  - `internal/integration/batch/store.go` and
    `internal/integration/batch/migrations/0002_batch_provenance.sql`
  - `cmd/fi-fhir/batch_runtime.go` (runtime contract)
  - `docker-compose.yaml`, `.env.example`, `scripts/check-runtime-config.sh`
  - `docs/operations/PRODUCTION-HARDENING.md`,
    `docs/operations/BATCH-INGESTION.md`
  - `Makefile` and the required `test:batch-ingestion` discovery list
- Implementation steps:
  1. Add the immutable workload block and its fail-closed validation, keeping
     existing revision digests unchanged when no block is declared.
  2. Resolve the principal from the source revision and evaluate the shared
     submit decision at the connector boundary before any side effect.
  3. Move receipt provenance onto the server-owned custody timestamp and the
     streaming digest; add S3 version/ETag pinning; relabel remote mtime as
     advisory in the type, the schema, and the docs.
  4. Wire runtime configuration, Compose, and the runtime-config check, then run
     the kill test and reconcile documentation.

## Prove

- Tests to run:
  - `go test -race ./internal/integration/batch/... ./internal/integration/authorization/... ./cmd/fi-fhir/...`
  - `go test -race ./...`
  - `POSTGRES_TEST_URL=... BATCH_S3_* =... make batch-ingestion`
- Lint/static checks:
  - `gofmt` on changed Go files
  - `golangci-lint run`
  - `go vet ./...` and `go vet -tags=integration ./internal/integration/...`
  - `make docs-validate`, `bash scripts/check-runtime-config.sh`
  - `go mod verify`, `git diff --check`
- CI checks:
  - Required merge-request pipeline reaches terminal green, including
    `test:batch-ingestion`, security, benchmark, and Golden Path gates.
  - Auto-merge armed after self-review; post-merge main pipeline harvested.

## Handoff/Harvest

- Docs to update:
  - Phase 4 execution plan (4.1b3 subsection and 4.1b2 landing evidence).
  - `docs/operations/PRODUCTION-HARDENING.md` and `BATCH-INGESTION.md`.
  - `.loom/50-worklog.md` and the Slice 4.1b3 handoff.
- Agent-context entries to add:
  - Workload identity binding decision and its immutability consequence.
  - The trusted-versus-advisory provenance split and its schema consequence.
- Next-slice candidates:
  - 4.1c: destination-scoped identity and secret resolution for the first
    durable HTTPS consumer.
  - Control-plane, PHI retention, and immutable audit policy.
