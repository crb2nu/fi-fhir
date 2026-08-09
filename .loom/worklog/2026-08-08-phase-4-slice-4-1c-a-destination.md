### 2026-08-08 - Phase 4 Slice 4.1c-a destination-scoped identity contract

- What changed:
  - Added `internal/integration/destination`: an immutable, content-addressed
    `Revision` (schema version, artifact/revision/destination identity, class,
    transport kind, non-secret transport policy carrying binding names only, an
    optional client identity block, a domain-separated semantic digest,
    `Validate()`, and `ValidateAgainst(lifecycle.RunnableBinding)`), a
    server-owned `Registry` whose `Resolve` requires the attempt's reference to
    equal the deployed revision's reference byte for byte, an `Authorizer` that
    derives the principal from the resolved revision alone, and a PostgreSQL
    decision recorder with its own numbered migration set and version ledger.
  - Added `integration.SecretResolver` in `pkg/integration` and a file/env
    implementation in `cmd/fi-fhir/destination_identity_runtime.go`. Every
    declared binding is resolved once at startup and zeroed, so a credential that
    does not resolve refuses startup instead of failing at dispatch.
  - Added `ActionDeliver` / `ObjectDestinationRevision` /
    `integration.destination.client` / `integration.destination.compatibility` to
    `internal/integration/authorization`, restructuring `Authorize` into
    per-action decisions with the submit path's conditions unchanged. The deliver
    path requires an empty `SourceID`, so a source principal can never be replayed
    as a destination client or the reverse.
  - Enforced the decision in `Dispatcher.RunOnce` after `Claim` and before the
    command is built or published, via a `DestinationDecider` interface the
    destination package satisfies structurally — neither package imports the
    other. A refusal becomes a non-retryable `DELIVERY_FORBIDDEN` or
    `DELIVERY_DESTINATION_UNVERIFIED` routed through the existing `MarkFailed`;
    an infrastructure failure is surfaced and retried, never dead-lettered.
  - Added `strict`/`compatibility` modes that reject each other's configuration,
    plus refusal of any `FI_FHIR_DELIVERY_IDENTITY_*` setting without a mode.
  - Added required CI job `test:delivery-identity` with a two-name existence
    guard, `make delivery-identity`, `docs/operations/DESTINATION-IDENTITY.md`,
    and the `FI_FHIR_DELIVERY_IDENTITY_*` block in `.env.example` and Compose.
- Why:
  - The sprint scope assumed the engine authenticates to destinations. It does
    not, and there was no destination artifact to scope a credential to. 4.1c-a
    ships the missing contract and the missing decision so 4.1c-b's HTTPS
    consumer has something exact to present.
- Evidence:
  - Day-1 gate `TestDeliveryDispatch_ContactsNoDestination` passed against
    unmodified main @ `7111cca1`, confirming correction 13 before any production
    code was written.
  - Kill-test `TestDeliveryIdentity_PostgresKafkaScopedDispatch` passes on
    PostgreSQL 16 + Kafka with `-race`.
  - Negative control 1 (stub the decision to return nil): all four attempts
    publish, zero provenance rows are written, the crossed-digest and orphan
    attempts both reach `succeeded` with one Kafka record each, and the
    compatibility decision is never recorded — assertions 1, 2, 3, and 5 fail.
  - Negative control 2 (remove the registry's digest equality check): the
    crossed-digest attempt publishes, isolating that check as load-bearing for
    assertion 2.
  - `make delivery-reliability` (Slice 2.3) passes unchanged on a clean broker.
  - `gofmt` clean, `golangci-lint run` 0 issues, `go vet ./...` and
    `go vet -tags=integration ./internal/...` clean, `go test -race ./...` green,
    `go test -race -run 'Authoriz|Submit' ./internal/integration/...` green,
    `make check-runtime-config`, `make security-gosec`, `make security-vulncheck`.
- What's next:
  - 4.1c-b: the first durable HTTPS destination consumer presenting the scoped
    identity, sized comparably to Slice 2.2.
- Sources:
  - [S1] `.loom/iteration-plan-phase-4-slice-4-1c-a-destination-identity.md`
  - [S2] `.loom/31-sprint3-execution-specs.md` Lane S3-B
  - [S3] `internal/integration/delivery/destination_identity_integration_test.go`
  - [S4] Command: `POSTGRES_TEST_URL=... KAFKA_TEST_BROKERS=... make delivery-identity`
