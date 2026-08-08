# RALPH Iteration Plan — Phase 4 Slice 4.1c-a Destination-Scoped Identity Contract

## Review

- Roadmap milestone: Phase 4 Slice 4.1 — enforce identity, authorization, and
  PHI policy.
- Lane: S3-B of `.loom/31-sprint3-execution-specs.md`, branch
  `feat/phase4-slice-4-1c-destination-identity`.
- Spec sections: `.loom/20-product-spec-integration-engine-ide-completion.md`
  identity and isolation contracts and the 1.0 destination matrix;
  `.loom/30-implementation-plan-integration-engine-ide-completion.md` Phase 4
  Slice 4.1.
- Prior decisions to preserve:
  - Tenant, integration revision, source, destination, action, and resource
    identity are server-owned. Senders and destinations cannot assert them
    through headers, certificates, object keys, MSH fields, or any other in-band
    data.
  - Slice 4.1b1 added exactly one fail-closed `integration.submit` decision over
    exact tenant, revision, and source. 4.1b2 (MLLP) and 4.1b3 (batch) bound
    transport identity to it. A new action extends that policy module rather
    than forking a parallel one.
  - Artifact revisions are immutable and content-addressed, and the deployed
    lifecycle release pins their exact digest (Slice 1.1a).
  - Secret **values** never enter a revision, a durable record, a log line, a
    metric label, or a broker payload. Only binding names travel.
  - 4.1b3's provenance idiom: server-owned facts are trusted, remote-derived
    facts carry an `_advisory` suffix and a `COMMENT ON COLUMN` saying so, and
    new CHECK constraints land `NOT VALID` so pre-slice rows are visibly
    distinguishable rather than retroactively vouched for.
  - `internal/integration/delivery/store.go` belongs to 4.2a. This lane does not
    touch it.

## Align

### The a/b split, and why

The sprint scope assumed the engine authenticates to destinations today, so
4.1c would be "scope an existing credential". Correction 13 of
`.loom/31-sprint3-execution-specs.md` says otherwise, and this lane's day-1 gate
proves it from behavior: `Dispatcher.RunOnce` claims one outbox row, marshals a
`deliveryCommand`, and publishes it to the single constant Kafka topic
`integration.delivery.v1`. That is the entire dispatch path. `webhook`, `fhir`,
`database`, and `file` are plan-level action *classes* validated in
`internal/integration/processor/workflow_plan.go`; none is an executed
transport. There is no destination artifact to bind a credential to, and no
`SecretReference` resolver of any kind.

4.1c is therefore two slices:

- **4.1c-a (this slice)** — ship the missing contract and the missing decision.
  A digest-verified `DestinationRevision`, a `SecretResolver` interface with one
  implementation, the `integration.deliver` action and the
  `integration.destination.client` grant, enforcement on the dispatch path, and
  server-owned provenance for each decision. The transport does not change.
- **4.1c-b (not this sprint)** — the first durable HTTPS destination consumer
  that presents the scoped identity. Genuinely new runtime, comparable in size
  to Slice 2.2.

### Day-1 gate result

`TestDeliveryDispatch_ContactsNoDestination` was written first and run against
**unmodified `main` @ `7111cca1`**. It **PASSED**.

The test stands a live TLS endpoint on loopback at the address a webhook
destination would be reached on, runs one complete production submission through
the durable admission path, and dispatches it. Results:

| Assertion | Result |
|---|---|
| Accepted TCP connections on the destination endpoint | **0** |
| HTTP requests served by the destination endpoint | **0** |
| Kafka records produced | **exactly 1**, key = attempt ID, topic = `integration.delivery.v1` |
| Destination address in any durable record or broker payload | **none** — no scheme, host, or port in receipts, canonical events, lineage, attempts, or outbox |
| A URL as a destination name | **rejected at planning**, `ErrWorkflowPlanningFailed`, zero durable rows written |
| Control: test dials the endpoint itself | both counters move, so the zeros above are facts about the engine, not a broken listener |

The gate therefore **confirms** the spec's re-scoping. No correction to
`.loom/31` was required, and implementation proceeds as 4.1c-a only.

The third assertion is the sharpest finding: the published DSL restricts
destination names to `^[a-z][a-z0-9_.-]*$`
(`internal/workflow/published_yaml.go`), so a destination address is not merely
unused — it is **unrepresentable** in the production contract. That is exactly
the gap this slice closes.

### Scope in

- A new `internal/integration/destination` package owning `DestinationRevision`:
  schema version, artifact/revision/destination identity, class, transport kind,
  a non-secret transport policy, secret **binding names**, an optional client
  identity block, a semantic digest, `Validate()`, and
  `ValidateAgainst(lifecycle.RunnableBinding)` using the same `hasSecretBinding`
  discipline as `mllp/source.go` and `batch/source.go`.
- A destination `Registry` that resolves a `DestinationRevisionRef` to its exact
  revision and **verifies the digest**, refusing any semantic mutation.
- `integration.SecretResolver` in `pkg/integration` plus one file/env-backed
  implementation wired in `cmd/fi-fhir/`, never inside `internal/integration/*`.
- `ActionDeliver` / `ObjectDestinationRevision` /
  `DestinationClientGrant = "integration.destination.client"` in
  `internal/integration/authorization`, with `Authorize` restructured so the
  submit path is behavior-preserving.
- Enforcement in `Dispatcher.RunOnce` after `Claim` and before
  `messageForWorkItem`/`Publish`; denial becomes a non-retryable
  `DELIVERY_FORBIDDEN` failure routed through the existing `MarkFailed`.
- 4.1b1-style `strict` / `compatibility` modes that reject each other's
  configuration.
- Server-owned decision provenance in this lane's own migration set, with
  destination-derived facts labeled `_advisory` and CHECKs added `NOT VALID`.
- A regression guard for the already-shipped fail-closed unmapped-destination
  behavior (correction 16).

### Scope out

- No new destination transport. Kafka remains the only publisher.
- No changes to `internal/workflow`'s legacy `webhookAction`.
- No edits to `internal/integration/delivery/store.go` (4.2a).
- No GraphQL schema change (S3-C1 owns `schema.graphql` this sprint).
- No processor or session migration number (S3-C1 owns `0004` in both).
- No secret provider backends beyond file/env.
- No serve component-table restructuring (S3-A owns it).

## Riskiest assumption + kill-test

> **"The engine authenticates to destinations today, so 4.1c is about scoping an
> existing credential."**

Killed on day one, before any production code, by
`TestDeliveryDispatch_ContactsNoDestination` passing on unmodified `main`. Had it
failed — had anything connected to the TLS endpoint — the spec's re-scoping would
have been wrong and this lane would have corrected `.loom/31` and re-planned.

The residual risk this slice must not fall into is the mirror image: shipping a
credential-binding mechanism with **no consumer**, an elaborate no-op that reads
as done. The primary kill-test defends against that by proving the decision runs
on the real dispatch path with real durable consequences.

**Primary: `TestDeliveryIdentity_PostgresKafkaScopedDispatch`** — PostgreSQL 16 +
Kafka, one tenant, one integration revision with three destinations
(`dest-alpha` → identity A, `dest-beta` → identity B, `dest-orphan` planned but
absent from the deployed revision), and a sentinel planted in identity A's secret
file:

1. Alpha and beta both publish; recorded provenance names A for alpha and B for
   beta, and neither names the other.
2. An attempt for `dest-beta` carrying `dest-alpha`'s destination digest fails
   the digest check and is dead-lettered, not published.
3. `dest-orphan` produces a `DELIVERY_FORBIDDEN` DLQ entry with `attempt_count`
   unchanged and zero Kafka records for its attempt ID.
4. The sentinel appears in none of the durable record classes, the Kafka
   key/value/headers, or process output.
5. `compatibility` mode authorizes the unbound class; a `strict` deployment
   carrying compatibility-only configuration **fails startup**.

**Negative control:** stub `AuthorizeDelivery` to return `nil` unconditionally
and confirm assertions 2, 3, and 5 fail. A pipeline where the stubbed build
passes means the decision is not on the dispatch path.

**Existence guard:** the CI job's first step lists **both** test names and
asserts `NR != 2` exits non-zero, so a renamed or skipped proof cannot make the
job greener.

## Land

- Planned file areas (this lane's owned files):
  - `internal/integration/destination/` (new package: revision, registry,
    identity, provenance, migrations)
  - `internal/integration/authorization/policy.go` (deliver action, restructured
    `Authorize`)
  - `internal/integration/delivery/dispatcher.go`,
    `internal/integration/delivery/types.go`,
    `internal/integration/delivery/identity.go` (new)
  - `pkg/integration/secret.go` (new: `SecretResolver` interface)
  - `cmd/fi-fhir/destination_identity_runtime.go` (new) and one appended block
    in `cmd/fi-fhir/main.go` after the delivery block
  - `.env.example`, `docker-compose.yaml`, `Makefile`, `.gitlab-ci.yml`
    (appended job only), `docs/operations/*`
- Implementation steps:
  1. Land the day-1 gate as a test-only commit proving `main`'s behavior.
  2. Add the destination revision contract, its digest, and its registry.
  3. Add `SecretResolver` and the file/env implementation.
  4. Restructure `Authorize` behavior-preservingly and add `AuthorizeDelivery`.
  5. Enforce on the dispatch path with a non-retryable `DELIVERY_FORBIDDEN`.
  6. Add strict/compatibility modes and mutual configuration rejection.
  7. Add decision provenance in this lane's own migration set.
  8. Wire runtime configuration, run the kill test and its negative controls,
     reconcile documentation.

## Prove

- Tests to run:
  - `go test -race ./internal/integration/destination/... ./internal/integration/authorization/... ./internal/integration/delivery/... ./internal/integration/processor/... ./cmd/fi-fhir/...`
  - `go test -race -run 'Authoriz|Submit' ./internal/integration/...` (submit-path preservation)
  - `go test -race ./...`
  - `POSTGRES_TEST_URL=... KAFKA_TEST_BROKERS=... make delivery-identity`
  - `POSTGRES_TEST_URL=... KAFKA_TEST_BROKERS=... make delivery-reliability` (2.3's proof must still pass)
- Lint/static checks:
  - `gofmt` on changed Go files, `golangci-lint run`, `go vet ./...` and
    `go vet -tags=integration ./internal/integration/...`
  - `make check-runtime-config` with the new `FI_FHIR_DELIVERY_IDENTITY_*` vars
  - `make security-gosec`, `make security-vulncheck`
- CI checks:
  - New blocking job `test:delivery-identity` appended at the end of the test
    stage with the two-name existence guard and `allow_failure: false`.
  - Required merge-request pipeline reaches terminal green including the manual
    `test:benchmark` play and the security stage.

## Handoff/Harvest

- Docs to update:
  - `.loom/30-implementation-plan-integration-engine-ide-completion.md` 4.1
    section: add the 4.1c-a subsection and record the a/b split with the
    day-1 gate as its justification.
  - `.loom/50-worklog.md` dated entry, with owned files recorded before the
    first commit.
  - `.loom/slice-handoff-phase-4-slice-4-1c-a-destination-identity.md` on
    completion.
  - `docs/operations/*` for the new runtime configuration.
- Next-slice candidates:
  - **4.1c-b** — the first durable HTTPS destination consumer presenting the
    scoped identity resolved here, honoring the existing circuit/retry/DLQ state
    machine, never accepting a destination-supplied header as a trust input.
    Sized like Slice 2.2.
  - S3-C2 retention policy; 4.4 tracing, chaos, and numeric budgets.
