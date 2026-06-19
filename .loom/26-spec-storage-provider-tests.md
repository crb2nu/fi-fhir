# 26 - Spec: Storage/Provider Integration Tests

**Status**: Ready for independent pickup
**Lane**: F - Product expansion speclets
**Tracking**: libs/fi-fhir#18

## Goal

Define a test-hardening slice for storage providers, especially S3/MinIO behavior used by ETL and CLI flows, without changing the CI pipeline in this spec lane.

The next agent should distinguish mock S3 unit coverage from live MinIO integration coverage, then add the smallest missing proof that provider routing, object operations, and ETL sink/source paths work against a real S3-compatible endpoint.

## Non-Goals

- Do not change `.gitlab-ci.yml` in this slice unless a later integration lane explicitly owns CI promotion.
- Do not add a new storage backend.
- Do not rewrite existing `pkg/storage.Provider` abstractions.
- Do not require Docker-in-Docker; use existing local MinIO, CI service variables, or build-tagged live tests.
- Do not make storage provider tests depend on terminology loader fixture fixes.

## Acceptance Criteria

- Existing local, mock S3, CLI storage, and ETL storage tests are inventoried so new tests do not duplicate already-covered behavior.
- A build-tagged live integration test path is specified or implemented for MinIO-backed `Put`, `Open`, `Stat`, `List`, `ListRecursive`, `Exists`, and `Delete`.
- The test plan covers both `s3://bucket/key` and default-bucket paths where the current provider supports both.
- ETL sink/source usage is covered at the provider boundary, proving an uploaded object can be consumed by the downstream ETL path without relying on a fake in-memory provider.
- Required environment variables are documented in the test or package docs, including `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET`, and any `FI_FHIR_MINIO_*` aliases actually used by CLI helpers.
- Tests fail fast with clear skip messages when live MinIO credentials are absent.

## Kill-Test

Run the proposed live MinIO test against an empty test bucket twice in a row. It must pass both times, proving object cleanup, prefix isolation, and idempotent bucket handling are reliable enough for repeated local or CI execution.

## Dependencies

- `pkg/storage/provider.go`, `pkg/storage/minio.go`, and existing storage tests.
- `pkg/etl/sink/minio.go` and ETL sink/source tests that currently use mock providers.
- `cmd/fi-fhir/storage.go` and CLI storage/live integration helpers.
- Existing `test:integration` service variables for MinIO as reference only; CI edits belong to a later lane.
- P2 CLI/ETL coverage guidance in the planning README.

## Sources

- `.loom/20-product-spec.md` - Pull ingestion and storage backend open questions.
- `docs/planning/README.md` - P2 ETL CLI coverage and P3 storage provider test expansion.
- `.gitlab-ci.yml` - Existing MinIO service/env shape for integration tests.
- `pkg/storage/provider.go` - Unified local and S3/MinIO provider contract.
- `pkg/etl/pipeline.go` and `pkg/etl/sink/minio.go` - ETL storage usage.

## Assignment Note

An agent can pick this up independently by starting with a storage-test inventory, adding build-tagged live tests under `pkg/storage` or the narrowest CLI package, and leaving CI wiring as an explicit follow-up unless the integration lane hands over that scope.
