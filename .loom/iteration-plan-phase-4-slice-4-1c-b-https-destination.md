# RALPH Iteration Plan — Phase 4 Slice 4.1c-b First Durable HTTPS Destination Consumer

## Review

- Roadmap milestone: Phase 4 Slice 4.1 — enforce identity, authorization, and
  PHI policy on the durable delivery path.
- Lane: S4-A of `.loom/32-sprint4-execution-specs.md`, branch
  `feat/phase4-slice-4-1c-b-https-destination`. Merge order for the sprint is
  **C → A → B → E**.
- Spec sections: `.loom/32-sprint4-execution-specs.md` Lane S4-A (corrections
  1-10); `.loom/30-implementation-plan-integration-engine-ide-completion.md`
  Phase 4 Slice 4.1c.
- Prior decisions to preserve:
  - Tenant, integration revision, source, destination, action, and resource
    identity are server-owned. Nothing a destination emits — a response header,
    a redirect, a served certificate — can select, influence, or impersonate
    them.
  - Secret **values** never enter a revision, a durable record, a log line, a
    metric label, or a broker payload. Only binding names travel.
  - 4.1b3's provenance idiom: server-owned facts are trusted, remote-derived
    facts carry an `_advisory` suffix and a `COMMENT ON COLUMN` saying so, and
    new CHECK constraints land `NOT VALID`.
  - `internal/integration/delivery/store.go` belongs to 4.2a. This lane does not
    touch it. `internal/api/graphql/schema.graphql` is frozen for Sprint 4.
  - The `destination` package and the `delivery` package do not import each
    other. `delivery` declares primitives-only interfaces; `destination`
    satisfies them structurally (S3-B handoff, lines 85-90).

## Align

### The riskiest assumption, and the gate that killed it

> "4.1c-a resolves the destination revision on the dispatch path, so 4.1c-b is
> wiring a transport onto an existing resolution."

`TestDeliveryTransport_HTTPSDestinationPublishesToBrokerToday` was written first,
against unmodified `main`, and **passed**. It stands a live TLS server at the
exact URL an `https`-transport destination declares, deploys that destination in
a strict registry with a resolvable credential binding, runs one complete
production submission through durable admission, and dispatches it once:

- the TLS endpoint records **zero** accepted TCP connections and zero requests;
- Kafka records **exactly one** command for the attempt on
  `integration.delivery.v1`;
- `integration_delivery_identity_decisions` records `authorized` under the
  destination's own declared subject, against the digest this process verified,
  with the URL in `destination_endpoint_advisory` and nowhere else.

So the resolution happens, is complete, is authorized, is durably recorded with
the address — and is then discarded in favour of the broker. There is nothing to
wire onto. The slice is: **build the routing seam, the dispatch-time credential
lifetime, and the trust-closed client.**

### The transport decision

Recorded in `.loom/40-decisions.md` (2026-08-09): **transport substitution at
the `Publisher` seam**, rejecting an in-process Kafka consumer and rejecting
"both". Rationale and alternatives are in the decision entry; the short version
is that the durable state machine is already destination-aware and
transport-blind (the circuit is keyed on the destination artifact, not on
Kafka), so a transport substituted at the `Publisher` seam inherits the lease,
retry, backoff, DLQ, replay, resubmit, discard, and circuit for free and leaves
`store.go` untouched. An in-process consumer would be a **second** at-least-once
boundary over the one the outbox already owns.

The Kafka-dependency question (correction 8) is decided in the same commit:
**keep the requirement, document it**, with a named follow-up.

## Implement

1. **Day 1, test-only** — the gate above, `make destination-transport`, the new
   blocking job `test:destination-transport`, and the two decision entries.
2. **`delivery/transport.go`** — a second primitives-only seam parallel to
   `DestinationDecider`: `DestinationTransport.DeliverDestination(ctx, tenantID,
   attemptID, ref, payload) (handled bool, err error)`, plus a `TransportFailure`
   interface mirroring `Refusal` but carrying a retryable bit, and the bounded
   conversion to `Failure`.
3. **`delivery/dispatcher.go`** — route between the transport and the publisher
   between `messageForWorkItem` and `publisher.Publish`. The call is bounded by
   the existing `PublishTimeout` so it cannot outlive its lease.
4. **`destination/transport.go`** — the HTTPS transport. Resolves the deployed
   revision, returns "not mine" for `kafka`, resolves the token binding per
   dispatch and zeroes it, builds a trust-closed client (no redirect follow, no
   proxy, TLS ≥ 1.2, `CABundleBinding` roots or the system pool, never
   `InsecureSkipVerify`), bounds and discards the response body, and maps the
   status class onto the failure contract.
5. **`destination/migrations/0002_https_delivery_provenance.sql`** — server-owned
   delivery provenance in the destination package's own ledger. Destination-side
   facts carry `_advisory` and a `COMMENT ON COLUMN`; the new CHECK is
   `NOT VALID`.
6. **`cmd/fi-fhir/`** — `destinationIdentityRuntime` gains the resolver;
   `buildDeliveryDispatcher` wires the transport when the registry contains any
   `https` destination.
7. **Narrow, do not invert** `TestDeliveryDispatch_ContactsNoDestination`: same
   name, same place in `make delivery-identity`, same arity-2 guard, one extra
   sentence in the doc comment saying it now proves a **`kafka`-class**
   destination contacts nothing.
8. **Docs** — `docs/operations/DESTINATION-IDENTITY.md` stops saying the engine
   contacts no destination, states the rotation contract, and states the Kafka
   dependency; `.env.example` gains the dependency note; `.loom/30` 4.1c is
   updated.

## Prove

- `TestDeliveryTransport_HTTPSClassContactedExactlyOnceUnderScopedIdentity` —
  PostgreSQL 16 + Kafka + three in-test TLS servers and one redirect-target
  listener, one registry with six destinations, six assertions.
- Negative control — the same proof under a router that unconditionally reports
  "not mine". Assertions 1-4 must fail; assertion 5 (`kafka`-class) must still
  pass. Run in the same job so a control that passes turns the pipeline red.
- `make delivery-identity` and `make delivery-reliability` unchanged and green.
- gofmt, `golangci-lint run`, `go vet ./...`, `go test -race ./...`,
  `make check-runtime-config`, gosec, govulncheck.

## Hand off

Slice handoff on completion, with next actions: a FHIR-class destination for
5.1, mTLS to destinations, and a multi-tenant destination registry.
