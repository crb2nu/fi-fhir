# RALPH Iteration Plan: Phase 1 Slice 1.2 Durable Submission

**Status**: implementing
**Date**: 2026-07-14

## Riskiest assumption + kill-test

**Load-bearing assumption**: PostgreSQL 16 can be the sole admission authority
for the existing deterministic `MessageProcessor` path: one tenant-scoped
idempotency claim and one transaction can make the receipt, sanitized canonical
event, revision/trace lineage, initial delivery attempt, and outbox work visible
all at once, while every duplicate and commit-unknown retry returns the first
durable result without creating a second unit of work.

**Kill test**: against PostgreSQL 16, inject a failure after each transaction
write boundary and once immediately after a successful `COMMIT`; reconstruct all
database-backed processor state; then submit the same production A01 concurrently
from 64 callers split across fresh processor/database handles. Require:

- every pre-commit fault leaves zero rows in all five durable record classes;
- the post-commit-unknown retry returns the already durable receipt;
- all 64 successful duplicate results are byte-identical to the durable result;
- exactly one receipt, canonical event, lineage record, initial attempt, and
  pending outbox row exist after handles are closed and reopened;
- an explicit idempotency key wins over the derived source + MSH-10 + exact
  integration-revision identity;
- a reused key with a different request fingerprint fails closed;
- no raw envelope bytes, raw-payload field, workflow source, secret reference,
  or uncataloged error text appears in any persisted JSON.

The proof must finish in under 30 minutes and emit exact row counts plus the
receipt/event/trace/attempt/outbox identities. The integration test is blocking,
not a best-effort local check.

**Failure mode if the assumption is wrong**: stop before authenticated HTTP
ingress. Do not acknowledge production submissions through a split receipt/event
store or the legacy non-transactional outbox wrapper; redesign the admission
boundary first.

**Status**: not run.

Positive evidence: PostgreSQL 16 documents transaction blocks as all-or-nothing
and invisible until commit, and documents unique constraints plus `ON CONFLICT`
as the concurrency arbitration mechanism.

Disconfirming evidence: `pkg/eventsourcing.OutboxEventStore` appends the event and
outbox in separate calls and deliberately ignores outbox-save failure; it cannot
satisfy this slice. PostgreSQL also exposes `transaction_resolution_unknown`
(`08007`), so a commit error cannot safely be interpreted as a rollback; the
retry contract must resolve through the durable idempotency row.

Sources:

- https://www.postgresql.org/docs/16/tutorial-transactions.html
- https://www.postgresql.org/docs/16/transaction-iso.html
- https://www.postgresql.org/docs/16/errcodes-appendix.html
- `pkg/eventsourcing/outbox.go`

## Outcome

Enable production mode on the same exact profile/parser/planner semantics used
by preview, but acknowledge only after a PostgreSQL transaction durably records
the complete admission unit. The precise guarantee is:

> Durable acceptance once per tenant-scoped effective idempotency identity;
> at-least-once delivery begins from one pending transactional-outbox row per
> planned destination attempt. Later transport retries may repeat an external
> effect unless that destination supports an idempotency token.

## Scope in

- Add a numbered PostgreSQL migration for receipts, sanitized canonical events,
  message lineage, initial delivery attempts, and delivery outbox rows.
- Keep the existing preview constructor side-effect-free and fail production
  closed unless a PostgreSQL committer is explicitly configured.
- Reuse the exact definition/profile/workflow resolver, A01 projection, and pure
  workflow planner for production evaluation.
- Define effective idempotency precedence:
  1. exact explicit request key;
  2. a domain-separated digest of tenant, source, MSH-10, and the exact active
     integration definition revision reference.
- Domain-separate and deterministically derive receipt, trace, lineage, attempt,
  and outbox identifiers from the effective admission identity.
- Treat the same effective key with a different tenant-scoped request fingerprint
  as an idempotency conflict, never as a duplicate success.
- Store the validated raw-free production `ProcessResult` on the receipt so a
  duplicate or commit-unknown retry can return the first durable result exactly.
- Queue one initial attempt and one outbox row for every planned external action;
  log-only actions remain route lineage and do not create delivery work.
- Provide restart-safe receipt lookup through the same PostgreSQL committer.

## Scope out

- HTTP/MLLP ingress, transport acknowledgements, GraphQL production submission,
  or changes to the authenticated preview surface (Slice 1.3).
- Outbox polling, leases, destination execution, retries, DLQ, replay, or
  resubmit (Phase 2 delivery reliability).
- Generic event-sourcing replacement or repair of the legacy outbox wrapper.
- Retained raw payload support. Slice 1.2 accepts only ephemeral raw retention;
  encrypted raw storage requires its separate storage/audit implementation.
- Multiple formats or event types beyond the existing exact HL7v2 ADT A01 path.
- Shared-hosting multi-tenancy or row-level-security claims.

## Acceptance criteria

- Production without an initialized PostgreSQL committer still returns the
  catalog-safe durable-committer-required error before artifact loading.
- Preview behavior and byte determinism remain unchanged and perform no SQL.
- PostgreSQL migration is idempotent and serialized; every durable table carries
  tenant identity and foreign-key lineage.
- A successful call returns only after commit and passes a strict production
  result validator with exact artifact revisions and event-bound route/action
  lineage.
- Duplicate explicit or derived submissions return the original receipt/result
  and create no new durable rows.
- Raw source bytes are never an SQL argument. Canonical event JSON and persisted
  result JSON contain no `raw_payload` member.
- Every queued delivery exposes one deterministic attempt ID, and every attempt
  has one deterministic pending outbox record containing references rather than
  raw or canonical payload bytes.
- Transaction failures roll back every record class; commit-unknown retries are
  resolved by durable lookup rather than blind re-execution.
- Focused unit/race tests, the PostgreSQL 64-way kill test, full Go tests, lint,
  security, documentation validation, MR CI, and default-branch CI pass.

## Intended files

- `pkg/integration/contracts.go`
- `pkg/integration/contracts_test.go`
- `internal/integration/processor/message_processor.go`
- `internal/integration/processor/message_processor_test.go`
- `internal/integration/processor/adt_a01.go`
- `internal/integration/processor/workflow_plan.go`
- `internal/integration/processor/postgres_submission.go`
- `internal/integration/processor/postgres_submission_test.go`
- `internal/integration/processor/postgres_submission_integration_test.go`
- `internal/integration/processor/migrations/0001_atomic_submission.sql`
- `.gitlab-ci.yml`
- `ROADMAP.md`
- `.loom/30-implementation-plan-integration-engine-ide-completion.md`
- `.loom/40-decisions.md`
- `docs/STATUS.md`
- `CHANGELOG.md`
- this iteration plan

## Implementation sequence

1. Add failing production-contract, idempotency, migration, atomicity, and
   64-caller tests.
2. Generalize the existing pure A01 evaluation and planner across preview and
   production without adding destination execution.
3. Add strict production result finalization and validation.
4. Add the fixed-schema PostgreSQL migration and transactional committer.
5. Run pre-commit and commit-unknown fault/restart proofs, then the 64-caller
   duplicate gate across fresh handles.
6. Update roadmap/status/decision evidence, self-review, and ship through
   terminal MR and default-branch pipelines.

## Test commands

```bash
go test -race -count=1 ./pkg/integration ./internal/integration/processor
POSTGRES_TEST_URL=... go test -tags=integration -race -count=1 \
  -run 'TestPostgresProductionSubmission_(AtomicFaultRestart|Duplicate64Way)' \
  ./internal/integration/processor
go test -count=1 ./...
go vet ./...
golangci-lint run --timeout=30m ./pkg/integration/... \
  ./internal/integration/processor/...
bash scripts/validate-docs.sh
make security-vulncheck security-gosec
```
