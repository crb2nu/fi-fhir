# Slice Handoff — Phase 4 Slice 4.1c-a Destination-Scoped Identity Contract

**Lane**: S3-B of `.loom/31-sprint3-execution-specs.md`
**Branch**: `feat/phase4-slice-4-1c-destination-identity`
**Base**: `origin/main` @ `7111cca1`
**Plan**: `.loom/iteration-plan-phase-4-slice-4-1c-a-destination-identity.md`

## What shipped

The engine never contacts a destination. `Dispatcher.RunOnce` claims one outbox
row and publishes one command to the single constant Kafka topic
`integration.delivery.v1`; an external consumer performs the destination call.
Before this slice a destination was only
`integration.DestinationRevisionRef{artifact_id, revision_id, digest, class}`
with no resolvable bytes behind the digest, no transport, and no credential
binding — and there was no `SecretReference` resolver of any kind, so the Slice
1.0 secret contract was in practice a name-presence check.

This slice ships the missing contract and the missing decision. It does not add a
transport.

| Deliverable | Where |
|---|---|
| Immutable, content-addressed destination revision | `internal/integration/destination/revision.go` |
| Server-owned registry with byte-exact reference verification | `internal/integration/destination/registry.go` |
| `integration.deliver` authorizer, `strict`/`compatibility` modes | `internal/integration/destination/identity.go` |
| Decision provenance store + migration | `internal/integration/destination/postgres.go`, `migrations/0001_delivery_identity.sql` |
| `SecretResolver` interface | `pkg/integration/secret.go` |
| File/env resolver + runtime loader | `cmd/fi-fhir/destination_identity_runtime.go` |
| Action, object kind, and dotted grants | `internal/integration/authorization/policy.go` |
| Enforcement point + refusal contract | `internal/integration/delivery/dispatcher.go`, `identity.go` |
| Operator documentation | `docs/operations/DESTINATION-IDENTITY.md` |

## Day-1 gate result

`TestDeliveryDispatch_ContactsNoDestination` was written **before any production
code** and run against unmodified `main` @ `7111cca1`. It **PASSED**, confirming
correction 13 and keeping 4.1c split into a and b.

| Assertion | Result on unmodified main |
|---|---|
| Accepted TCP connections on a live loopback TLS endpoint standing where a webhook destination would be reached | **0** |
| HTTP requests served by that endpoint | **0** |
| Kafka records produced by one complete production submission | **exactly 1**, key = attempt ID, topic = `integration.delivery.v1` |
| Scheme/host/port in receipts, canonical events, lineage, attempts, outbox, or any Kafka field | **none** |
| A URL as a destination name | **rejected at planning** with `ErrWorkflowPlanningFailed`, zero durable rows |
| Control: the test dials the endpoint itself | both counters move, so the zeros are facts about the engine, not a broken listener |

The sharpest finding is the third row: the published DSL restricts destination
names to `^[a-z][a-z0-9_.-]*$` (`internal/workflow/published_yaml.go`), so a
destination address was not merely unused — it was **unrepresentable**. That is
the gap this slice closes.

## Kill-test and negative controls

`TestDeliveryIdentity_PostgresKafkaScopedDispatch` — PostgreSQL 16 + Kafka, one
tenant, `dest-alpha` bound to identity A, `dest-beta` to identity B, one attempt
for beta carrying alpha's digest, and `dest-orphan` absent from the deployed set.

| # | Assertion | Evidence |
|---|---|---|
| 1 | Alpha and beta publish under their own identities | Provenance names `alpha-client` for alpha and `beta-client` for beta, each with the digest it resolved and `integration.destination.client`; neither names the other |
| 2 | Crossed digest is dead-lettered, not published | Attempt `failed`, outbox `failed`, DLQ active with `DELIVERY_DESTINATION_UNVERIFIED`, zero Kafka records |
| 3 | Orphan is `DELIVERY_FORBIDDEN` | Attempt `failed` with `attempt_count` unchanged at 1, DLQ active with `DELIVERY_FORBIDDEN`, zero Kafka records |
| 4 | Secret sentinel escapes nowhere | Absent from receipts, canonical events, attempts, outbox, delivery audit, DLQ, the decision table, and every Kafka key/value/header |
| 5 | Mode isolation | `compatibility` authorizes the unbound class under `integration.destination.compatibility`; `strict` refuses a compatibility subject and refuses to load a registry containing an unbound destination |

**Negative control 1 — stub the decision to return `nil` unconditionally.** The
mandated control. Outcomes became `{published: 4}` with zero forbidden:

- assertion 1 fails — no provenance rows exist for alpha or beta
- assertion 2 fails — the crossed-digest attempt reaches `succeeded` and produces 1 Kafka record
- assertion 3 fails — the orphan attempt reaches `succeeded` and produces 1 Kafka record
- assertion 5 fails — the compatibility decision is never recorded

**Negative control 2 — remove the registry's digest equality check.** Outcomes
became `{published: 3, identity_forbidden: 1}`: the crossed-digest attempt
publishes while the orphan is still refused, isolating that single comparison as
the load-bearing check for assertion 2.

Both controls were reverted and both proofs re-verified green.

## Design decisions worth carrying forward

- **Neither package imports the other.** `delivery` declares
  `DestinationDecider` (primitives) and a `Refusal` interface; `destination`
  satisfies both structurally. The domain package stays free of transport
  concerns, and the worker stays free of the destination contract. An earlier
  shape had `destination` importing `delivery.WorkItem` and produced an import
  cycle the moment the kill-test needed both.
- **Refusal vs. failure to decide.** A `*RefusalError` is a decision and becomes
  a non-retryable dead letter. Any other error is infrastructure and is surfaced
  for retry. A provenance write failure must never discard work.
- **The identity is a function of the destination revision alone.** Nothing the
  work item asserts can select a subject; a mismatched reference fails resolution
  before any principal exists.
- **A delivery principal carries no `SourceID`.** That is the isolation boundary
  between `integration.submit` and `integration.deliver`.
- **The secret resolver has a job today.** Startup resolves every declared
  binding once and zeroes it, so a missing credential refuses startup. Without
  that, the resolver would have been a contract with no consumer until 4.1c-b.
- **Own migration set.** `internal/integration/destination/migrations/0001_*.sql`
  with its own `integration_destination_schema_migrations` ledger, so this lane
  claimed no number in `processor/` or `session/` (S3-C1 owns `0004` in both).
- **No retroactive vouching.** Dispatches predating the slice have no decision
  row at all; absence means the decision was never made, never that it was
  allowed. The provenance CHECK is `NOT VALID` so a later backfill cannot claim
  it governed rows it never saw. The destination-declared endpoint is
  `_advisory` with a `COMMENT ON COLUMN`.

## Gotchas for the next agent

- The delivery topic is a **constant**, so integration tests on a reused local
  broker accumulate records across runs. Both proofs now drain the topic and
  assert **per attempt key**, and derive unique attempt IDs per run (a unique
  explicit `IdempotencyKey` for the gate, a run-suffixed attempt ID for the
  kill-test). Slice 2.3's proof still reads the first N records from the start,
  so it fails on a dirty local broker and needs a fresh Kafka container — that is
  a local artifact, not a regression.
- `scripts/check-runtime-config.sh` intends to exclude `_test.go` but pipes
  `grep -oh` output, which carries no filename, so the filter never matches. Any
  `FI_FHIR_*` constant in a test file shows up as missing from `.env.example`.
  This lane's test env vars were renamed to `DESTINATION_TEST_*` to avoid adding
  noise; the script itself belongs to S3-A.
- `golangci-lint`'s `errname` rule requires error types to end in `Error`
  (`DenialError`, `RefusalError`), even when the type is a decision rather than a
  conventional error.

## Next actions

**4.1c-b — the first durable HTTPS destination consumer.** Sized comparably to
Slice 2.2, and the reason 4.1c-a's contract has an `https` transport it does not
execute.

1. Consume `integration.delivery.v1` in-process (or replace it for HTTPS-class
   destinations), resolving each command's destination revision through the same
   digest-verified registry this slice ships.
2. Present the scoped identity: resolve the destination's `token_binding` through
   `integration.SecretResolver` at dispatch time, never at plan time, and never
   into a struct that is marshaled into a record, a log line, a metric label, or
   a broker value.
3. Honor the existing circuit/retry/DLQ state machine
   (`internal/integration/delivery/store.go`) rather than adding a parallel one.
4. Never accept a destination-supplied header, redirect, or served certificate as
   a trust input. `destination_endpoint_advisory` already names the shape such
   facts must take when 4.1c-b records them.
5. Extend `TestDeliveryDispatch_ContactsNoDestination` into its successor: the
   TLS endpoint now **must** be contacted, exactly once, with the right identity,
   and only for HTTPS-class destinations. That test is the boundary marker this
   slice planted.

**Also open**: S3-C2 (retention policy for canonical events and session samples,
TTL columns, durable purge component), and 4.4 (tracing exporter, cardinality
budgets under load, chaos/DR, rolling upgrade).

## Files changed

New:
`internal/integration/destination/{revision,registry,identity,postgres}.go` and
tests, `internal/integration/destination/migrations/0001_delivery_identity.sql`,
`internal/integration/delivery/identity.go` and tests,
`internal/integration/delivery/destination_{fixture,contact_integration,identity_integration}_test.go`,
`internal/integration/authorization/deliver_policy_test.go`,
`pkg/integration/secret.go`,
`cmd/fi-fhir/destination_identity_runtime.go` and tests,
`docs/operations/DESTINATION-IDENTITY.md`.

Modified: `internal/integration/authorization/policy.go`,
`internal/integration/delivery/dispatcher.go`,
`internal/integration/processor/workflow_plan_test.go`,
`cmd/fi-fhir/{delivery_runtime,preview_runtime,main}.go`, `.env.example`,
`docker-compose.yaml`, `Makefile`, `.gitlab-ci.yml`,
`docs/operations/PRODUCTION-HARDENING.md`, `.loom/30-implementation-plan-*.md`,
`.loom/50-worklog.md`.

Untouched by this lane, as the lane map requires:
`internal/integration/delivery/store.go` (4.2a),
`internal/api/graphql/schema.graphql` and every regenerated artifact (S3-C1),
`scripts/*`, `deploy/**`, and the `runServe` component table (S3-A).
