### 2026-07-16: Checkpoint Only After Durable Batch Admission

- Decision:
  - Identify a provider object by a domain-separated hash of source revision,
    provider path, and immutable provider version; persist only the resulting
    hash plus validated size and raw-free metadata. Require S3 bucket versioning
    and address the exact version ID for reads and deletion.
  - Identify each message deterministically from the source revision, object
    hash, pinned integration revision digest, message ordinal, and byte offset.
    Use that identity for the explicit durable-submission idempotency key and
    correlation ID. Refuse to resume a checkpoint under another integration
    revision.
  - Advance the PostgreSQL byte/message checkpoint only after the shared durable
    processor commits. A replica holds an expiring object lease and can reclaim
    abandoned work after the lease expires.
  - Archive by copy to a SHA-256-addressed destination, verify the destination,
    commit completed state/audit, then delete the source. Require S3 version IDs
    for exact deletion. Require SFTP `known_hosts`, immutable atomic publication,
    immediate pre-delete digest verification, and reject symlinked
    source/archive files.
- Rationale:
  - Admission and checkpoint cannot share one provider/database transaction.
    Repeating a deterministic admission after a crash closes the gap without
    pretending cross-system exactly-once delivery.
  - Archive-before-delete favors a recoverable duplicate copy over loss of the
    only verified clinical payload.
- Alternatives considered:
  - Checkpoint before admission (rejected because a crash loses a message).
  - Persist raw paths/provider versions for operator convenience (rejected to
    keep recovery state PHI-minimal and reduce provider detail exposure).
  - Rename SFTP source into archive without verification (rejected because it
    is not portable across filesystems and does not prove copied bytes).
  - Disable SSH host-key verification or learn keys on first use (rejected
    because production ingestion must fail closed against host impersonation).
- Consequences:
  - Recovery is at-least-once at the provider boundary and durable-once inside
    fi-fhir through idempotent admission.
  - Crashes after archive verification may temporarily leave both source and
    archive; the completed-object cleanup path safely retries deletion.
  - Source mutation produces a new object identity rather than inheriting an
    old checkpoint.
- Evidence:
  - MR `!108` pipeline `19331` passed 35/35, including required PostgreSQL 16/
    MinIO/SSH-SFTP job `186259`, and merged as `ed32915f`.
  - Main pipeline `19344` passed 38/38 and independently repeated the proof in
    job `186476`.
- Sources:
  - [S1] `internal/integration/batch/service.go`
  - [S2] `internal/integration/batch/store.go`
  - [S3] `internal/integration/batch/s3.go`
  - [S4] `internal/integration/batch/sftp.go`
  - [S5] `.loom/iteration-plan-phase-2-slice-2-4-batch-ingestion.md`
