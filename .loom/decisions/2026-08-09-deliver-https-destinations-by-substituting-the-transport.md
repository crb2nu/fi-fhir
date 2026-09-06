### 2026-08-09: Deliver HTTPS Destinations by Substituting the Transport at the `Publisher` Seam

- Decision:
  - Slice 4.1c-b delivers an `https`-transport destination **by substituting the
    transport at the `Publisher` seam inside `Dispatcher.RunOnce`**, not by
    adding an in-process consumer of `integration.delivery.v1`. A new
    primitives-only `DestinationTransport` interface is declared in
    `internal/integration/delivery` alongside `DestinationDecider`; the
    `destination` package satisfies it structurally; neither package imports the
    other [S1]. The dispatcher asks the transport whether it owns the claimed
    item's destination. An `https` destination is delivered over TLS and marked
    with the existing `MarkPublished`/`MarkFailed`; every other destination
    publishes to the constant Kafka topic exactly as before.
  - The Kafka topic remains the transport for `kafka`-class destinations, so
    `TestDeliveryDispatch_ContactsNoDestination` stays a true and meaningful
    boundary marker. It is **narrowed** to `kafka`-class, never inverted.
  - A successful HTTPS delivery reports the existing `OutcomePublished`. The
    outcome means "handed off successfully", which is exactly what it now is for
    both transports; introducing a second success outcome would force a change
    in `cmd/fi-fhir/serve_observability.go` and
    `internal/observability/metrics.go` for zero operator benefit.
  - The credential is resolved **per dispatch** through `integration.SecretResolver`,
    used to build one request, and zeroed. It never enters `Decision`, a log
    line, a metric label, a `Failure.Detail`, or any struct that is
    JSON-marshaled. There is no credential cache, so a file or environment
    rotation takes effect on the next dispatch [S5].
- Rationale:
  - The durable state machine is already destination-aware and transport-blind.
    `Claim` keys the circuit breaker on the **destination artifact**, not on
    Kafka [S2]; retry, backoff, DLQ, replay, resubmit, and discard all live in
    `store.go` and know nothing about a broker. A transport substituted at the
    `Publisher` seam inherits all of it and leaves `store.go` — still 4.2a's
    file — untouched.
  - The decision already runs at the right point. `decideIdentity` resolves the
    full destination revision, including `Transport` and `HTTPS.URL`, one line
    before the publish [S3] — and then discards it.
  - There is **no Kafka consumer anywhere in production code**. `kgo.ConsumeTopics`
    appears only in two integration tests [S4]. An in-process consumer is a new
    consumer group, new offset commits, new rebalance handling, and a **second**
    at-least-once boundary layered on the one the outbox already owns. It cannot
    reuse the circuit, the DLQ, or `recover`, because those are keyed to an
    outbox row the consumer holds no lease on.
  - The day-1 gate `TestDeliveryTransport_HTTPSDestinationPublishesToBrokerToday`
    passed on unmodified `main`: an `https` destination with a live TLS endpoint
    is fully resolved, digest-verified, authorized, and provenance-recorded with
    its URL — and is then published to Kafka anyway, with the endpoint recording
    zero accepted connections. That is the empirical basis for "substitute the
    transport", not "wire up the existing one".
- Alternatives considered:
  - **B. In-process consumer of `integration.delivery.v1`** (rejected: builds the
    larger thing. Two independent at-least-once layers make duplicate delivery a
    product rather than a bound, and the spec's P0 definition includes duplicate
    durable work for one idempotency key. Preserving an external-consumer
    contract that no production code implements is not a benefit.)
  - **C. Both — publish *and* deliver** (rejected on sight: two systems
    contacting one destination for one attempt is a duplicate-delivery
    generator.)
  - **Widen `DestinationDecider.Decide` into a transport interface** (rejected:
    reintroduces the import cycle the S3-B handoff records solving, and couples
    the authorization decision to a transport concern the `destination` package
    was deliberately kept free of.)
  - **Cache the HTTPS client per destination digest** (rejected: the revision is
    immutable, but the CA bundle and the token behind its binding names are
    files that rotate in place with no version to invalidate a cache on [S5]. A
    cached client would silently pin a rotated-out root. One client per dispatch
    costs one TLS handshake and keeps rotation honest.)
  - **A separate `FI_FHIR_DELIVERY_HTTPS_TIMEOUT` knob** (rejected: `Config.validate`
    already requires `PublishTimeout < LeaseDuration` [S6], which is precisely
    the invariant that stops a slow destination outliving its lease and being
    delivered twice after reclaim. A second knob would be a second way to break
    it.)
- Consequences:
  - A deployment that has already declared `transport: https` on a destination
    in its registry starts contacting that destination on upgrade. That is the
    slice, and the registry is server-owned, so declaring `https` is an explicit
    operator act — but it is a behaviour change on upgrade and is called out in
    `docs/operations/DESTINATION-IDENTITY.md`.
  - `MarkPublished` now means "handed off successfully" for two transports. Its
    doc comment says so; it is not renamed.
  - `destinationIdentityRuntime` gains a resolver field. That is a new object
    lifetime in `cmd/`, which is the one package deliberately chosen so no
    `internal/integration/*` type can hold resolved material.
- Sources:
  - [S1] `.loom/slice-handoff-phase-4-slice-4-1c-a-destination-identity.md:85-90`
  - [S2] `internal/integration/delivery/store.go:86-88`, `:198`, `:620-660`, `:663+`
  - [S3] `internal/integration/delivery/dispatcher.go:129,166-182`; `internal/integration/destination/identity.go:182`
  - [S4] `internal/integration/delivery/kafka.go:91`; `delivery_integration_test.go:473-486`; `destination_fixture_test.go:326-338`
  - [S5] `cmd/fi-fhir/destination_identity_runtime.go:123-141,189-191,202-211`
  - [S6] `internal/integration/delivery/types.go:107-114`
  - [S7] `.loom/32-sprint4-execution-specs.md` Lane S4-A, corrections 2, 3, 4, 5, 6
