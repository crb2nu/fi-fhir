# S3/SFTP Batch Ingestion

Slice 2.4 adds an optional `fi-fhir serve` worker for concatenated UTF-8 HL7v2
files. The worker is disabled unless `FI_FHIR_BATCH_SOURCE_CONFIG_PATH` is set.
It uses the same exact deployed lifecycle binding and durable message processor
as the production HTTP and MLLP adapters.

## Safety contract

- The source document is immutable and content-addressed. It contains policy and
  logical secret-binding names, never credentials or clinical data.
- PostgreSQL stores the object identity hash, provider, size, pinned integration
  revision digest, byte/message checkpoint, lease, content digest, and raw-free
  audit events. It does not store the object path, provider version, or message
  bytes. A release change cannot silently continue a partially processed file
  under different parsing/workflow semantics.
- A checkpoint advances only after durable admission commits. Repeating work
  after a crash uses the same deterministic idempotency key.
- Source mutation creates a new object identity and cannot inherit a checkpoint.
- Archive completion is copy to a SHA-256-addressed path, verify, mark completed
  in PostgreSQL, then delete the source. S3 deletion addresses the exact version
  ID. SFTP re-hashes the unchanged path immediately before removal and requires
  the immutable-drop policy below. A crash can leave both copies, but cannot
  remove the only verified copy.

Each input file must contain one or more HL7v2 messages beginning with `MSH`.
Messages may be separated with CR, LF, or CRLF. The configured
`max_message_bytes` bounds memory used by the streaming reader.

## Configuration

Set the common runtime values:

```text
FI_FHIR_BATCH_SOURCE_CONFIG_PATH=/etc/fi-fhir/batch-source.json
FI_FHIR_BATCH_DEFINITION_ID=integration-batch
FI_FHIR_BATCH_PRINCIPAL_ID=batch-ingest
FI_FHIR_BATCH_WORKER_ID=fi-fhir-batch-1
```

The definition must be deployed and its exact source ID, revision ID, digest,
provider, and secret bindings must match the source document. Startup or polling
fails closed on a mismatch.

For S3, use file-backed credentials in production:

```text
FI_FHIR_BATCH_S3_ACCESS_KEY_FILE=/var/run/secrets/fi-fhir-batch/s3-access-key
FI_FHIR_BATCH_S3_SECRET_KEY_FILE=/var/run/secrets/fi-fhir-batch/s3-secret-key
```

S3 credential transport requires TLS, and the source bucket must have versioning
enabled so reads and deletion target one immutable provider version. Plaintext
is accepted only for a loopback endpoint used by local tests or a same-pod
sidecar.

For SFTP, pin the server host key and choose exactly the auth mode declared by
the source revision:

```text
FI_FHIR_BATCH_SFTP_KNOWN_HOSTS_FILE=/var/run/secrets/fi-fhir-batch/known_hosts
FI_FHIR_BATCH_SFTP_PRIVATE_KEY_FILE=/var/run/secrets/fi-fhir-batch/id_ed25519
FI_FHIR_BATCH_SFTP_PRIVATE_KEY_PASSPHRASE_FILE=/var/run/secrets/fi-fhir-batch/key-passphrase
```

Password auth instead uses `FI_FHIR_BATCH_SFTP_PASSWORD_FILE`. An empty,
oversized, malformed, or wrong `known_hosts` file is rejected. Inputs and
archive destinations that are symlinks are also rejected.

SFTP producers must upload to a temporary name and atomically rename only a
complete file into the input directory. Once published, its path and bytes must
be immutable; server ACLs should deny overwrite/truncate to the producer. SFTP
has no conditional unlink operation, so this policy plus the worker's metadata
checks and immediate pre-delete SHA-256 verification form the deletion boundary.

## Recovery

After a process or provider outage, restore PostgreSQL and provider access, keep
the same immutable source document, and restart the service. An expired lease is
reclaimed automatically. Do not rename, rewrite, or manually delete an in-flight
source object.

A paused or temporarily unavailable deployed release stops admission without
terminating the server; polling resumes when the exact release is runnable
again. Invalid streams are quarantined as failed objects and do not stop other
files in the poll.

Failed objects are quarantined in PostgreSQL with a safe error code. Operators
must correct the source or publish a new source version; this slice intentionally
does not add an unaudited checkpoint-reset command.

The implementation and CI proof can be run with:

```bash
go test ./internal/integration/batch ./cmd/fi-fhir
make batch-ingestion
```

The required integration gate uses PostgreSQL 16, a real MinIO API, and a real
SSH/SFTP protocol server. It kills processing in the admission/checkpoint window
and verifies exact durable cardinality, resume, mutation isolation, host-key
rejection, archive bytes, and raw-PHI exclusion.
