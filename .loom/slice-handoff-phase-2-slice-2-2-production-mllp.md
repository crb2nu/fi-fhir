# RALPH Slice Handoff: Phase 2 Slice 2.2 Production MLLP

## Slice Summary

- Milestone: Phase 2 production channel runtime / Engine Beta
- Slice: deployed-release production MLLP source adapter
- Status: implementation and local non-PostgreSQL proof complete; CI/merge pending

## What Landed

- Strict content-addressed UTF-8 MLLP source revisions with framing, timeouts,
  mutual TLS binding names, canonical CIDRs, ACK mode, and connection/message
  bounds.
- Fragmented and sequential frame handling, safe MSH/MSA/ERR ACK construction,
  application or commit response codes, and PHI-free failures.
- Per-frame lifecycle runnable resolution and exact source/deployment binding.
- Transaction-scoped admission authorization that serializes durable commit with
  pause and retirement.
- Optional `serve` composition with one PostgreSQL pool, catalog-backed MLLP
  definition resolution, existing exact profile/workflow artifact resolution,
  and coordinated shutdown.
- Required PostgreSQL 16/TCP CI gate covering pre-commit ACK exclusion,
  concurrent pause, 32 reconnecting duplicates, resume, retirement, restart,
  cardinality, and leakage.

## Proof and Artifacts

- `go test -race -count=1 ./internal/integration/mllp` passes.
- Focused lifecycle, processor, and `cmd/fi-fhir` tests pass.
- The required integration test is discoverable with `-tags=integration`.
- PostgreSQL execution, MR pipeline, merge commit, and main evidence are pending.
- Key files:
  - `internal/integration/mllp/`
  - `internal/integration/lifecycle/admission.go`
  - `internal/integration/processor/postgres_submission.go`
  - `cmd/fi-fhir/preview_runtime.go`
  - `docs/operations/PRODUCTION-MLLP.md`

The feature diff is intentionally larger than the normal 500-line review
target: the source contract, transport, lifecycle transaction gate, runtime
composition, real PostgreSQL/TCP proof, and operator contract must land as one
coherent safety boundary. More than half of the added lines are tests and
tracked specification/evidence; splitting them would permit implementation to
merge without its required kill-test or operational constraints.

## What Is Still Open

- Authoritative PostgreSQL CI and merge evidence.
- Production GitOps Service/port/secret activation is intentionally pending.
- Authenticated HTTP remains on its verified startup definition registry.
- Destination workers, DLQ/replay/resubmit, S3/SFTP runtime wiring, and IDE
  lifecycle controls remain later slices.

## Next Action

Pass the required MR/main gates and record exact evidence. Then implement Slice
2.3 durable delivery attempts, retry/DLQ/replay/resubmit, and one real queue
transport without conflating durable admission with external delivery.

## Context

- Iteration plan:
  `.loom/iteration-plan-phase-2-slice-2-2-production-mllp.md`
- Product spec: `.loom/20-product-spec-integration-engine-ide-completion.md`
- Implementation plan: `.loom/30-implementation-plan-integration-engine-ide-completion.md`
- Decision log: `.loom/40-decisions.md`
- Persistent agent-context was unavailable; tracked repository documents are the
  durable context record.
