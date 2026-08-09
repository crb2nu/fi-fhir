### 2026-08-08 - Slice 4.1c-b day-1 gate: https destination transport routes nothing today

- What changed:
  - Added `TestDeliveryTransport_HTTPSDestinationPublishesToBrokerToday`
    (`internal/integration/delivery/destination_transport_integration_test.go`),
    the Lane S4-A day-1 gate, plus `make destination-transport` and the new
    blocking job `test:destination-transport`.
  - Refactored `newDurableSubmissionFixture` into
    `newDurableSubmissionFixtureWithDestinations`, so a proof can seed durable
    attempts carrying a **real** content-addressed destination revision's
    reference instead of a fabricated digest. Added
    `destinationWorkflowYAMLFor`, which plans one delivery action per
    destination on one route.
  - Recorded two dated decisions in `.loom/40-decisions.md`: transport
    substitution at the `Publisher` seam (rejecting an in-process Kafka
    consumer), and keeping the Kafka requirement for HTTPS-only deployments with
    a documented dependency and a named follow-up.
  - Added `.loom/iteration-plan-phase-4-slice-4-1c-b-https-destination.md`.
- Why:
  - Lane S4-A's riskiest assumption was "4.1c-a resolves the destination
    revision on the dispatch path, so 4.1c-b is wiring a transport onto an
    existing resolution". If that were true the lane would widen
    `DestinationDecider` into a transport interface — reintroducing the import
    cycle the S3-B handoff records solving — and would discover at the end that
    a credential has to be resolved somewhere no design accounted for.
  - `.loom/32-sprint4-execution-specs.md` requires the gate to pass on
    unmodified `main` before any production code is written, and requires the
    transport decision as a day-1 docs commit.
- Evidence:
  - The gate **passes** on unmodified `main`
    (`POSTGRES_TEST_URL=... KAFKA_TEST_BROKERS=... make destination-transport`,
    4.26s): a destination declaring `transport: https` with a live TLS endpoint
    at its URL gets **zero** accepted TCP connections and zero requests; Kafka
    records **exactly one** command for the attempt on
    `integration.delivery.v1`; and
    `integration_delivery_identity_decisions` records `authorized` under the
    destination's declared subject, against the digest this process verified,
    with the URL in `destination_endpoint_advisory` and nowhere else.
  - Two controls keep the zeros honest: the test dials the endpoint itself at
    the end and both counters move, and the planted credential is read back from
    disk so the binding is proven resolvable rather than assumed.
  - `make delivery-identity` still passes unchanged (10.0s), so the fixture
    refactor is behaviour-preserving for both 4.1c-a proofs.
- What's next:
  - The lane is "build the routing seam, the dispatch-time credential lifetime,
    and the trust-closed client", not "wire the transport up".
  - Files this lane owns, per `.loom/32` parallelization map:
    `internal/integration/delivery/{dispatcher.go,identity.go,transport.go}`,
    `internal/integration/destination/**` (including
    `migrations/0002_*.sql`), `cmd/fi-fhir/{delivery_runtime.go,destination_identity_runtime.go}`,
    `docs/operations/DESTINATION-IDENTITY.md`, and append-only additions to
    `Makefile` / `.gitlab-ci.yml` under distinct names. It touches neither
    `internal/integration/delivery/store.go` (4.2a) nor
    `internal/api/graphql/schema.graphql` (frozen for Sprint 4).
- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` Lane S4-A, corrections 1-10
  - [S2] `internal/integration/delivery/destination_transport_integration_test.go`
  - [S3] `.loom/40-decisions.md` (2026-08-09, two entries)
  - [S4] `.loom/slice-handoff-phase-4-slice-4-1c-a-destination-identity.md:85-90`
