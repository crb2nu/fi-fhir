# RALPH Iteration Plan — Phase 2 Slice 2.2 Production MLLP

**Status**: Implemented locally; authoritative PostgreSQL CI pending
**Date**: 2026-07-15
**Plan**: `plan-complete-fi-fhir-as-a-production-integration-engine-and-ide-341d98#8`
**Branch**: `codex/phase2-production-mllp`

## Riskiest assumption + kill-test

**Load-bearing assumption**: The deployed-only lifecycle resolver and durable
submission transaction can form one linearizable authorization boundary, so an
MLLP sender receives a positive ACK only when the exact immutable release was
still deployed at durable commit—even if an operator pauses it concurrently.

**Kill test**: A required PostgreSQL 16 integration test starts the real TCP
MLLP server and shared durable `MessageProcessor`, then completes within 30
minutes and proves all of the following:

1. a content-addressed UTF-8 MLLP source revision exactly matches the deployed
   integration's source reference before any message can run;
2. a message fragmented across every framing boundary is reconstructed and no
   positive ACK is readable while PostgreSQL admission is deliberately blocked;
3. releasing the database block commits one receipt/event/lineage/attempt/outbox
   unit before the framed `AA` or `CA` response is observed;
4. a database transaction that changes the snapshot to `paused` while admission
   waits causes a retryable negative ACK and zero new durable rows; resume permits
   the same exact release, while retirement remains closed after restart;
5. 32 concurrent duplicates over reconnecting clients return positive ACKs but
   collapse to the original durable unit;
6. malformed prefix/trailer, oversize input, invalid HL7, read timeout, queue
   saturation, rate overflow, disallowed client, and failed mutual TLS cannot
   reach the processor or produce a positive ACK; and
7. ACKs, errors, logs, and every persisted JSON value contain neither raw-message
   nor credential sentinels beyond the protocol-required safe MSH-10 echo.

**Failure mode if the assumption is wrong**: A sender could receive a positive
ACK for uncommitted work or continue submitting after pause/retire, making the
lifecycle catalog advisory instead of authoritative. Slice 2.2 must not merge
until admission and lifecycle transitions share a database serialization point.

**Status**: Unit/race and CI discovery pass. The PostgreSQL 16/TCP proof is
implemented but could not run locally because `POSTGRES_TEST_URL` and a local
PostgreSQL service were unavailable; required CI remains the merge gate. Positive evidence is Slice 2.1's deployed-only
`PostgresCatalog.ResolveRunnable` and Slice 1.2's atomic submission transaction.
Disconfirming evidence is that a preflight resolver call alone races a concurrent
pause, and the current startup registry contains no MLLP source artifact. HAPI's
MLLP constants confirm standard frame bytes 11/28/13, while its lower-layer
documentation warns that UTF-16/UTF-32 can collide with byte framing; this slice
therefore supports UTF-8 only.

Protocol evidence:

- [HAPI MLLP constants](https://hapifhir.github.io/hapi-hl7v2/base/apidocs/constant-values.html)
- [HAPI lower-layer encoding limitations](https://hapifhir.github.io/hapi-hl7v2/base/apidocs/ca/uhn/hl7v2/llp/MinLowerLayerProtocol.html)
- [HAPI original/enhanced acknowledgement codes](https://hapifhir.github.io/hapi-hl7v2/base/apidocs/ca/uhn/hl7v2/protocol/Processor.html)

## Review

- Roadmap milestone: Phase 2 production channel runtime / Engine Beta.
- Spec sections: secure production data plane, authenticated MLLP/HTTP latency,
  Golden Journey 1, and implementation-plan Slice 2.2.
- Prior decisions to preserve:
  - one deployment tenant/security domain;
  - exact content-addressed integration/profile/workflow/source revisions;
  - one shared parser/planner and PostgreSQL-only production committer;
  - raw source bytes remain ephemeral and absent from durable JSON/logs;
  - lifecycle state, not a caller or startup pointer, authorizes MLLP execution;
  - positive transport response means durable admission, not downstream delivery.

## Align

- Slice name: deployed-release production MLLP source adapter.
- Scope in:
  - a strict, content-addressed UTF-8 MLLP source revision bound to the existing
    `SourceRevisionRef`, including listener, framing, timeouts, TLS/client,
    connection, and acknowledgement policy without inline secret material;
  - standard configurable single-byte framing with safe defaults
    `VT (0x0B) ... FS (0x1C) CR (0x0D)` and fragmented/multiple-frame support;
  - TLS 1.3 mutual-auth mode plus exact CIDR client allowlists; explicit plaintext
    mode is limited to trusted loopback/sidecar deployments and remains documented;
  - per-frame deployed binding resolution, exact source/profile/workflow loading,
    and a same-transaction lifecycle authorization gate before durable admission;
  - bounded connections, in-flight work, queued work, and message rate using the
    deployed revision's capacity policy;
  - application (`AA`/`AE`/`AR`) or commit (`CA`/`CE`/`CR`) ACK policy, with safe
    MSH/MSA/ERR projection and MSH-10 correlation only;
  - optional `serve` wiring, startup/migration/config validation, graceful close,
    operator documentation, fixtures, Make target, and blocking CI job.
- Scope out:
  - destination outbox workers, retry/DLQ/replay/resubmit, and external FHIR send;
  - GraphQL/REST/UI lifecycle mutation or durable Integration Sessions;
  - production GitOps exposure, Service/Helm port changes, or live sender rollout;
  - dynamic multi-port/multi-tenant listener orchestration and staged/canary deploy;
  - UTF-16/UTF-32, arbitrary charset conversion, non-HL7v2 payloads, and enhanced
    two-phase commit-plus-application acknowledgement exchanges.
- Acceptance criteria:
  - listener activation selects no executable definition from the static registry;
    every frame begins with the catalog's deployed exact binding;
  - source artifact identity/digest and server-owned tenant/source/principal match;
  - lifecycle pause/retire and durable admission are serialized in PostgreSQL;
  - no positive ACK precedes a valid durable result; duplicate durable results are
    positive and byte-stable for the required MSA correlation fields;
  - framing, timeouts, TLS/client controls, queues, concurrency, and rate are all
    bounded and reject safely without reflecting raw PHI;
  - targeted race/unit/TCP tests, the PostgreSQL kill-test, full Go tests, lint,
    security, documentation validation, and required CI are green.
- Dependencies/blockers:
  - Slice 2.1 lifecycle and Slice 1.2 durable submission are merged and green;
  - exact profile/workflow bytes remain supplied by the verified immutable startup
    artifact registry until a later durable artifact-store slice;
  - PostgreSQL 16 is required for the transaction/pause/restart kill-test;
  - persistent agent-context is unavailable, so tracked plan/handoff docs are the
    durable context record.
- Rollback:
  - MLLP is disabled unless its source-config environment is set. Removing that
    setting leaves authenticated HTTP and preview behavior unchanged; migrations
    remain forward-only audit state.

## Land

- Planned file areas:
  - `internal/integration/mllp/`
  - `internal/integration/lifecycle/`
  - `internal/integration/processor/`
  - `cmd/fi-fhir/`
  - `testdata/golden/integration/adt-mllp/`
  - `Makefile`, `.gitlab-ci.yml`, `.loom/`, `docs/`, and `CHANGELOG.md`.
- Implementation steps:
  1. Add the source-revision contract, strict framing/ACK codec, TLS/client policy,
     and deterministic unit/transport tests.
  2. Add capacity control, deployed binding service, and transaction-scoped
     lifecycle authorization without changing HTTP/preview behavior.
  3. Compose optional MLLP into `serve`, reuse the PostgreSQL pool and shared
     processor/artifact loaders, and add graceful startup/shutdown behavior.
  4. Add the real TCP/PostgreSQL kill-test, CI discovery/gate, fixtures, runbook,
     decision log, roadmap/spec/status, and handoff updates.

## Prove

- Tests to run:
  - `go test -race -count=1 ./internal/integration/mllp ./internal/integration/lifecycle ./internal/integration/processor ./cmd/fi-fhir`
  - `go test -count=1 ./...`
  - PostgreSQL 16 MLLP kill-test with `-tags=integration -race`.
- Lint/static checks:
  - `gofmt`, `go vet`, focused golangci-lint, `git diff --check`;
  - `make security-gosec`, `make security-vulncheck`, `make docs-validate`;
  - test/CI discovery must prove the named integration test actually runs.
- CI checks:
  - new required `test:mllp-runtime` PostgreSQL 16 job;
  - existing deployment-lifecycle, durable-submission, Golden Path, unit/race,
    binary, security, image, and deployment jobs remain blocking.

## Handoff/Harvest

- Docs to update: this plan, product/implementation plans, decision log,
  `ROADMAP.md`, `docs/STATUS.md`, deployment lifecycle/runbook/hardening docs,
  environment examples, changelog, and a Slice 2.2 handoff.
- Agent-context entries: tracked locally in the plan/handoff because the service
  is unavailable; record the serialization decision, protocol limits, exact CI
  evidence, and any disconfirming finding.
- Next-slice candidate: Phase 2 Slice 2.3 durable delivery attempts,
  DLQ/replay/resubmit, and one real queue transport.
