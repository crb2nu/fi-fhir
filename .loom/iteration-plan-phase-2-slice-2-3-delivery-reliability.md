# RALPH Iteration Plan — Phase 2 Slice 2.3 Delivery Reliability

**Status**: In progress
**Date**: 2026-07-15
**Plan**: `plan-complete-fi-fhir-as-a-production-integration-engine-and-ide-341d98#9`
**Branch**: `codex/phase2-delivery-reliability`

## Riskiest assumption + kill-test

**Load-bearing assumption**: A PostgreSQL-leased outbox and a real Kafka
publisher can preserve Slice 1.2's durable-acceptance identity while providing
bounded, restart-safe, at-least-once downstream delivery, including an operator
recovery path that cannot silently create unbounded or unaudited work.

**Kill test**: A required PostgreSQL 16 plus Kafka integration test completes
within 30 minutes and proves all of the following:

1. one durably accepted production message creates exactly one initial delivery
   attempt and outbox unit before any Kafka record is visible;
2. two workers race to lease the same due work, but only one lease owner may
   publish it during that lease;
3. with Kafka unavailable, failures follow the exact bounded retry schedule,
   preserve receipt/event/trace/attempt/outbox identity, and terminate in one
   durable DLQ record with a redacted error code and circuit state;
4. restart reclaims an expired lease and resumes queued work without another
   receipt, canonical event, lineage row, or initial outbox row;
5. replay requires an authenticated operator, non-empty reason, and idempotency
   key; repeating the operation requeues the same failed identity once, and a
   repaired broker receives it with stable message key and lineage headers;
6. resubmit creates exactly one new child attempt/outbox unit, links it to the
   failed parent, and remains idempotent under concurrent duplicate requests;
7. a simulated publish-before-database-ack crash may repeat the Kafka record but
   never duplicates durable acceptance, and every repeat carries the stable
   attempt key required for consumer-side suppression; and
8. persisted JSON, audit records, Kafka headers, errors, and logs contain no raw
   ingress bytes or credential sentinels.

**Failure mode if the assumption is wrong**: Work can be lost after durable
acceptance, delivered without bounded recovery, or multiplied by concurrent
workers/operators. Slice 2.3 must not merge unless restart, lease expiry, final
DLQ, idempotent recovery, and stable downstream identity are proved together.

**Positive evidence**: Slice 1.2 atomically creates one receipt/event/lineage/
attempt/outbox unit, and Docker Compose already provides Kafka and queue runtime
configuration.

**Disconfirming evidence**: The outbox schema has no lease, retry, circuit, DLQ,
or operation-audit fields; the only registered queue implementation is a logging
stub; no Kafka client is linked; and a publisher cannot atomically commit both a
Kafka record and the PostgreSQL success marker. The design therefore rejects a
universal exactly-once claim and makes the stable attempt ID the Kafka key.

## Review

- Roadmap milestone: Phase 2 production channel runtime / Engine Beta.
- Spec sections: secure production data plane, destination recovery, delivery
  guarantee, Golden Journey failure/replay, and implementation-plan Slice 2.3.
- Prior decisions to preserve:
  - PostgreSQL remains the sole durable production-admission authority;
  - acceptance is durable once and downstream I/O is at-least-once;
  - retries retain receipt/event/trace/destination revision identity;
  - raw source bytes remain ephemeral and absent from durable delivery payloads;
  - positive ingress ACK/HTTP response means durable acceptance, not delivery;
  - production execution remains bound to exact content-addressed revisions.

## Align

- Slice name: durable delivery reliability and Kafka outbox transport.
- Scope in:
  - forward-only delivery-reliability migration with leases, bounded scheduling,
    circuit state, DLQ state, parent linkage, and append-only operation/audit data;
  - multi-worker PostgreSQL claiming with `FOR UPDATE SKIP LOCKED`, expiring
    leases, bounded exponential backoff, and terminal failure classification;
  - real Kafka publication of raw-free delivery commands using stable attempt
    keys, lineage headers, acknowledgements from all in-sync replicas, optional
    TLS/SASL credentials supplied out of band, and bounded client timeouts;
  - durable replay of the same failed identity and resubmit as one linked child,
    both requiring principal, reason, and operator idempotency key;
  - optional `serve` worker wiring, fail-closed configuration, graceful close,
    operator documentation, a Make target, and a blocking CI integration job.
- Scope out:
  - universal exactly-once Kafka or external I/O semantics;
  - a distributed transaction spanning PostgreSQL and Kafka;
  - UI/GraphQL DLQ browsing or mutation surfaces;
  - runtime execution of webhook/FHIR/database/file destination protocols;
  - S3/SFTP batch sources, production GitOps activation, and live broker rollout;
  - a generalized destination artifact catalog beyond this queue transport.
- Acceptance criteria:
  - each worker owns only its live lease; expired work is safely reclaimable;
  - retries are scheduled, bounded, observable, and terminate in one DLQ record;
  - circuit-open work is deferred without consuming unbounded delivery attempts;
  - replay/resubmit are authorized, reasoned, audited, and idempotent;
  - Kafka records carry stable attempt identity and raw-free lineage metadata;
  - targeted/race/integration/full tests, lint, security, docs, and required CI pass.
- Dependencies/blockers:
  - Slices 1.2, 2.1, and 2.2 are merged and independently green;
  - PostgreSQL 16 and a protocol-level Kafka broker are required for the kill test;
  - Kafka publication cannot make PostgreSQL and broker acknowledgement atomic,
    so consumers must deduplicate by the stable attempt key.
- Rollback:
  - the worker is disabled unless explicitly configured. Removing its enablement
    stops publication while preserving pending/DLQ/audit rows; migrations remain
    forward-only and the existing ingress paths continue durable admission.

## Land

- Planned file areas:
  - `internal/integration/delivery/`
  - `internal/integration/processor/migrations/` and submission migration runner
  - `cmd/fi-fhir/`, `pkg/config/`, `docker-compose.yaml`
  - `Makefile`, `.gitlab-ci.yml`, `.loom/`, `docs/`, and `CHANGELOG.md`.
- Implementation steps:
  1. Extend the schema and migration runner, then add repository tests for claim,
     lease expiry, retry, circuit, DLQ, replay, resubmit, and audit invariants.
  2. Add the dispatcher state machine behind a narrow publisher interface and
     prove concurrency/restart behavior with a deterministic fake publisher.
  3. Add and runtime-wire the Kafka publisher with strict configuration and
     stable key/header/payload projection.
  4. Add the PostgreSQL/Kafka kill test, CI discovery/gate, operator docs,
     decision/roadmap/spec/status updates, and handoff evidence.

## Prove

- Tests to run:
  - `go test -race -count=1 ./internal/integration/delivery ./internal/integration/processor ./cmd/fi-fhir ./pkg/config`
  - `go test -count=1 ./...`
  - PostgreSQL 16 plus Kafka kill-test with `-tags=integration -race`.
- Lint/static checks:
  - `gofmt`, `go vet`, focused golangci-lint, and `git diff --check`;
  - `make security-gosec`, `make security-vulncheck`, `make docs-validate`;
  - test/CI discovery must prove the named integration test actually runs.
- CI checks:
  - new required delivery-reliability PostgreSQL 16 plus Kafka job;
  - existing durable-submission, lifecycle, MLLP, Golden Path, unit/race, binary,
    security, image, and deployment jobs remain blocking.

## Handoff/Harvest

- Docs to update: this plan, product/implementation plans, decision log,
  `ROADMAP.md`, `docs/STATUS.md`, runbook/hardening docs, environment examples,
  changelog, and a Slice 2.3 handoff.
- Agent-context entries: record the at-least-once boundary, lease/recovery
  invariants, Kafka configuration, disconfirming findings, and exact CI evidence.
- Next-slice candidate: Phase 2 Slice 2.4 runtime-wired S3/SFTP ingestion with
  checkpoint/resume and secure archive semantics.

## Completion Evidence

- MR `!106` merged as `ca968fbf07748cd76c4b01b545e571242d3ef02a`.
- MR pipeline `19226` passed 34/34; required delivery-reliability job `185433`
  passed the race-enabled PostgreSQL 16/Kafka failure/replay proof.
- Main pipeline `19235` passed 37/37; job `185505` independently repeated the
  delivery-reliability proof on the merge commit.
- Main gosec attempt `185513` was OOM-killed by the runner; GitLab's unchanged
  automatic retry `185585` passed. Production GitOps activation remains pending.
