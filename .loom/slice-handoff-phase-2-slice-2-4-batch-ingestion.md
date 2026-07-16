# Slice Handoff — Phase 2 Slice 2.4 Batch Ingestion

**Status**: Complete, merged, verified, and reconciled
**Date**: 2026-07-16
**Branch**: `codex/phase2-batch-ingestion`

## Delivered boundary

- Immutable content-addressed S3/SFTP source revisions bind to the exact deployed
  lifecycle release and logical secret-binding names.
- Optional `serve` wiring streams concatenated UTF-8 HL7v2 with bounded memory
  through the shared durable production processor.
- PostgreSQL owns raw-free exact-object leases, byte/message checkpoints, phase
  transitions, and audit. Checkpoints follow durable admission and retries reuse
  deterministic message identity.
- S3 and SFTP archive exact source bytes under a SHA-256-addressed destination,
  verify content, commit completion, and only then delete the source. S3 targets
  an exact version ID; SFTP requires pinned host keys, immutable atomic
  publication, immediate pre-delete digest verification, and rejects symlinks.
- The required integration test uses PostgreSQL 16, MinIO, and a real SSH/SFTP
  server to prove replica exclusion, lease reclaim, crash recovery, mutation
  isolation, host-key rejection, exact durable cardinality, archive integrity,
  and raw-PHI exclusion.

## Deliberately not activated

- Production GitOps, credentials, provider endpoints, and public exposure remain
  unchanged. Activation is a separate reviewed operation.
- No batch UI/operator reset API, non-HL7 production processor, Temporal flow,
  or Mentatlab integration was introduced.
- The existing generic `pkg/storage` coverage backlog is separate from this
  production adapter.

## Evidence

- Focused unit/runtime race tests: green.
- Full `go test ./...`, `go vet ./...`, and scoped golangci-lint: green.
- PostgreSQL/MinIO/SFTP kill-and-resume test under `-race`: green.
- MR `!108` pipeline `19331` passed 35/35; required batch job `186259` passed.
- MR `!108` merged as `ed32915f`.
- Main pipeline `19344` passed 38/38 and independently repeated the proof in
  batch job `186476`.
- Evidence MR `!109` reconciles the canonical roadmap/spec/plan/decision/status
  records with that proof.
- Production GitOps activation remains intentionally pending.

## Recommended next slice

Phase 3 Slice 3.1: a restart-safe Integration Session Workspace with exact
artifact revisions and explicit raw-retention policy.
