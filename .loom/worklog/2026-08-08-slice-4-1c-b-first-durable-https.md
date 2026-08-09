### 2026-08-08 - Slice 4.1c-b: first durable HTTPS destination consumer

- What changed:
  - `internal/integration/delivery/transport.go` (new): `DestinationTransport`, a
    second primitives-only seam **parallel to** `DestinationDecider`, plus a
    `TransportFailure` interface mirroring `Refusal` with a retryable bit and the
    bounded conversion to `Failure`. The `destination` package satisfies both
    structurally; neither package imports the other.
  - `internal/integration/delivery/dispatcher.go`: routes each authorized item
    between the transport and the broker, between `messageForWorkItem` and
    `publisher.Publish`. Success completes through the existing `MarkPublished`,
    failure through the existing `MarkFailed`. `NewDispatcherWithIdentity` now
    delegates to `NewDispatcherWithDestination`, so all four existing call sites
    are unchanged.
  - `internal/integration/destination/transport.go` (new): the HTTPS transport.
    Resolves the deployed revision, reports "not mine" for `kafka`, resolves the
    token binding per dispatch and zeroes it, builds a trust-closed client, and
    maps the status class onto the failure contract.
  - `internal/integration/destination/migrations/0002_https_delivery_provenance.sql`
    (new) + a generalized migration runner in the package's own ledger.
    `RecordDelivery` appends one row per executed delivery.
  - `cmd/fi-fhir/`: `destinationIdentityRuntime` gains the resolver and the
    provenance store; `buildDeliveryDispatcher` wires the transport when the
    registry contains at least one `https` destination.
  - Docs: `docs/operations/DESTINATION-IDENTITY.md` (transport table, rotation
    contract, HTTPS transport section, outcome mapping, delivery provenance,
    upgrade note, operator checklist), `.env.example`, `.loom/30` 4.1c-b.
- Why:
  - The day-1 gate proved the `Transport` field routes nothing: an `https`
    destination is resolved, verified, authorized, and provenance-recorded with
    its URL, then published to Kafka anyway. The slice is therefore to build the
    routing seam, the dispatch-time credential lifetime, and the client — not to
    wire up an existing transport.
  - Transport substitution at the `Publisher` seam inherits the lease, retry,
    backoff, DLQ, replay, resubmit, discard, and the per-destination-artifact
    circuit for free, and leaves `store.go` — 4.2a's file — untouched. An
    in-process consumer of `integration.delivery.v1` would have been a second
    at-least-once boundary over the outbox's, with no consumer anywhere in
    production code to build on.
- Evidence:
  - `TestDeliveryTransport_HTTPSClassContactedExactlyOnceUnderScopedIdentity`
    over PostgreSQL 16, Kafka, five TLS servers, and one redirect-target
    listener: six destinations, one registry, one production submission. Alpha
    and beta are each contacted exactly once carrying their own credential and
    never the other's; a drained queue redelivers nothing; a 503 retries under
    the existing backoff, increments the circuit, then recovers and closes it; a
    403 and a 302 dead-letter non-retryably with `attempt_count` unchanged and
    the redirect target records **zero** connections; the `kafka`-class
    destination in the same run produces exactly one broker record and contacts
    nothing; and neither planted credential reaches any of nine durable record
    classes, any broker field, or captured stdout/stderr.
  - **Negative control** in the same invocation: the whole scenario repeats
    against a router that reports it owns nothing. All four HTTPS assertions
    fail as required — `alpha served 0 requests`, `counts = [0 0 0]`,
    `flaky served 0 requests`, `no rows in result set` for the dead letter —
    while the kafka-class assertion still passes.
  - Two bugs the unit tests caught before CI did: `statusClass(0)` returned
    `"2xx"` (missing low guard), and an "untrusted certificate" case was vacuous
    because every `httptest` TLS server shares one built-in certificate, so an
    unrelated root has to be minted rather than borrowed from a second server.
  - `make delivery-identity` and `make delivery-reliability` both still pass
    unchanged. Full `go test -race ./...`, `go vet ./...`, `golangci-lint run`,
    `make check-runtime-config`, `make security-gosec`, and
    `make security-vulncheck` are clean.
- What's next:
  - Deferred and named in the slice handoff: a FHIR-class destination (5.1's
    hard prerequisite), mTLS to destinations with a per-destination client
    certificate, a multi-tenant destination registry, and a broker-free delivery
    worker for HTTPS-only deployments.
- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` Lane S4-A, tasks 1-9
  - [S2] `.loom/40-decisions.md` (2026-08-09, two entries)
  - [S3] `internal/integration/delivery/destination_transport_scenario_test.go`
  - [S4] `internal/integration/destination/migrations/0002_https_delivery_provenance.sql`
