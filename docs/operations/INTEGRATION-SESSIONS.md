# Restart-Safe Integration Sessions

Phase 3 Slice 3.1 adds an opt-in PostgreSQL workspace for Integration Sessions.
Sessions, redacted samples, append-only artifact revisions, immutable terminal
runs, accepted decisions, and export records survive a backend restart. Preview
runs record the exact profile revision ID and SHA-256 digest they executed.

This is an author/test foundation, not a production deployment control plane.
WebSocket stage streaming, workflow simulation, bundle publication, and GitOps
activation remain separate slices.

## Enable the workspace

The workspace is disabled unless `fi-fhir serve` receives:

```bash
export FI_FHIR_INTEGRATION_SESSION_ENABLED=true
export FI_FHIR_DATABASE_DRIVER=postgres
export FI_FHIR_DATABASE_HOST=postgres
export FI_FHIR_DATABASE_NAME=fi_fhir
export FI_FHIR_DATABASE_USERNAME=fi_fhir
export FI_FHIR_DATABASE_PASSWORD='use-a-secret-provider'
export FI_FHIR_DATABASE_SSL_MODE=verify-full
```

Startup opens PostgreSQL, takes a migration advisory lock, applies the session
schema once, and wires the GraphQL session routes to the durable store. Startup
fails closed when the database or migration is unavailable.

The authenticated GraphQL server still requires the existing deployment tenant,
origin, and bearer configuration. Until fine-grained Phase 4 authorization is
implemented, session operations require the temporary `graphql:operator` role;
the narrower `integration:preview` role remains limited to the typed stateless
preview operation.

Production GitOps does not enable this setting in Slice 3.1.

## Raw sample policy

Durable samples default to `redact`. For HL7v2 this replaces selected
identifier, name, birth date, address, phone, and SSN fields in PID before the
record reaches PostgreSQL. Other formats are stored as a redaction marker until
format-specific redactors are implemented.

Explicit raw retention additionally requires an AES-256 key file:

```bash
install -m 0400 /secure/generated/session-aes-key \
  /var/run/secrets/fi-fhir-session/aes-256-key
export FI_FHIR_INTEGRATION_SESSION_RETENTION_KEY_FILE=\
/var/run/secrets/fi-fhir-session/aes-256-key
```

The file must contain exactly 32 binary bytes. Retained payloads are encrypted
with AES-256-GCM using a random nonce and tenant/session/sample identity as
authenticated additional data. The key is never stored in PostgreSQL or session
exports. Exports omit explicitly retained raw bytes by default even when the
caller requests a session containing them.

Back up the key through the deployment's secret-management process whenever
retained samples exist. Losing it makes those samples intentionally unreadable.
Key rotation and retention expiry are Phase 4 work; prefer redacted samples.

## Immutability and replay

- Each artifact save appends a revision with a stable artifact ID, unique
  revision ID, increasing version, and content digest.
- A run loads one exact mapping-profile revision, verifies its digest, and uses
  the same profile compiler as the production processor.
- Successful and failed terminal runs cannot be changed through the store.
- Accepted diagnostic decisions and exports are separate durable audit records.
- Archive is a state transition. Archived sessions are hidden from the default
  list but remain reopenable by stable ID and visible when explicitly requested.

## Verification

The required restart proof uses PostgreSQL 16:

```bash
make integration-session
```

Locally, the target starts PostgreSQL with testcontainers when
`POSTGRES_TEST_URL` is unset. In CI it requires the supplied PostgreSQL service.
The test reconstructs every store/runner object, reopens the session, executes
one redacted sample against strict and tolerant profile revisions, checks the
warning/event delta and exact provenance, and scans session records for its raw
PHI sentinel. It also proves encrypted explicit retention, terminal-run
immutability, durable decisions/exports, and archive/list/reopen behavior.

## Rollback

Unset `FI_FHIR_INTEGRATION_SESSION_ENABLED` and restart the API. Other ingestion,
delivery, and preview paths do not depend on the session tables. Do not drop the
tables during an incident; preserve them for recovery and audit. Schema removal
requires a separately reviewed data-retention operation.
