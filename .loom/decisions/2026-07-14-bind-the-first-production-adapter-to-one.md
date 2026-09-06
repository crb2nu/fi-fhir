### 2026-07-14: Bind the First Production Adapter to One Trusted Integration

- Decision:
  - Mount exact `POST /v1/hl7v2` only when an explicit bearer or HMAC-SHA256
    credential is configured with one service principal and one integration ID.
  - Resolve tenant, source, format, classification, and executable revisions
    from deployment-owned state. Caller headers cannot select source identity.
  - Reject browser origins, compression, non-HL7 media, oversized bodies, and
    ambiguous headers before processing. Bound accepted bodies to at most 1 MiB.
  - Share one PostgreSQL-backed `MessageProcessor` between production ingress
    and GraphQL/IDE preview. Production commits; preview remains side-effect free.
  - Return a raw-free `202` projection only after atomic admission. Leave the
    endpoint unmounted when auth mode is unset.
- Rationale:
  - Adapter-specific authentication belongs before the shared clinical kernel.
  - A credential-to-integration binding prevents a sender from claiming another
    source while preserving exact server-owned provenance.
  - One live processor composition makes preview/production semantic drift a
    testable invariant rather than a convention.
- Alternatives considered:
  - Reuse generic `internal/ingest` (rejected because it permits auth-disabled
    configuration and calls the workflow engine outside durable admission).
  - Re-enable legacy GraphQL submit (rejected because its broad catalog and
    browser transport are not the production source boundary).
  - Accept caller-selected source/profile headers (rejected because they break
    provenance and tenant/source authorization).
- Consequences:
  - Operators need separate GraphQL and source-adapter credentials.
  - HMAC signs integration, idempotency, correlation, and exact bounded bytes.
  - `202` means durable admission and queued outbox work, not external delivery.
  - Production GitOps activation remains a separate reviewed operation.
- Evidence:
  - `make golden-path-001` passed 20 duplicate/restart/profile/IDE/leakage
    assertions against PostgreSQL 16 and a real restarted process.
- Sources:
  - [S1] `internal/integration/ingress/`
  - [S2] `cmd/fi-fhir/preview_runtime.go`
  - [S3] `scripts/golden-path-001.sh`
  - [S4] `.loom/iteration-plan-phase-1-slice-1-3-authenticated-http-golden-path.md`
