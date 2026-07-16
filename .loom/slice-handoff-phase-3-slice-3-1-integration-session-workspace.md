# Slice Handoff — Phase 3 Slice 3.1 Integration Session Workspace

**Status**: Complete, merged, verified, and reconciled
**Date**: 2026-07-16
**Branch**: `codex/phase3-integration-session-workspace`

## Delivered boundary

- A storage-neutral store with memory and tenant-scoped PostgreSQL
  implementations persists sessions, samples, append-only artifact revisions,
  immutable terminal runs, accepted decisions, and reopenable exports.
- Samples are redacted before durable storage by default. Explicit retention
  requires AES-256-GCM key material, uses identity-bound authenticated
  encryption, and remains excluded from normal JSON and exports.
- Every run executes one digest-verified immutable profile revision through the
  production profile compiler and records exact revision/digest provenance.
- Authenticated GraphQL routes expose stable create/list/reopen/archive and
  session operations without reopening the contained legacy catalog surface.
- Optional `serve` composition is PostgreSQL-only and separately feature-gated.
- The required restart test reconstructs stores and services, compares strict
  and tolerant profile outcomes, verifies terminal immutability and durable
  decisions/exports, and scans session state for the raw-PHI sentinel.

## Deliberately not activated

- Production GitOps activation and retention key distribution remain separate
  reviewed operations.
- Streaming diagnostics/lineage, workflow simulation, signed publication,
  promotion/deployment, key rotation/expiry, and fine-grained RBAC remain later
  slices.
- Session execution remains HL7v2/profile-only in this slice.

## Evidence

- Focused and race tests, full `go test ./...`, `go vet ./...`, scoped lint,
  security scan, GraphQL/UI codegen checks, Svelte checks, and docs validation:
  green locally.
- MR `!111` pipeline `19409` passed 37/37; required PostgreSQL session job
  `187425` passed.
- MR `!111` merged as `15746ccd`.
- Main pipeline `19424` passed 40/40 and independently repeated the PostgreSQL
  restart/exact-profile/raw-PHI proof in job `187618`.
- The first MR proof exposed PostgreSQL JSONB byte normalization of executable
  artifacts. Commit `1504a316` moved exact content to `BYTEA`; the replacement
  MR job and main job both passed against that correction.
- Production GitOps activation remains intentionally pending.

## Recommended next slice

Phase 3 Slice 3.2: stream server-owned stage, diagnostic, and lineage updates
into the Integration Session UI while preserving restart-safe durable truth.
