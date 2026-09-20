### 2026-09-08: Slice 4.1c-c is a fhir transport kind with its own policy, Option A

- Decision: The FHIR destination class is a third `TransportKind` on the
  server-owned destination revision — `TransportFHIR = "fhir"` with its own
  `FHIRPolicy{BaseURL, TokenBinding, CABundleBinding, Interaction}` — not an
  encoding flag on `HTTPSPolicy`. `Interaction` is the closed set
  `{transaction}` in v1. Validation mirrors `validateHTTPS` (https scheme, a
  host, no userinfo, no fragment, no query, ≤ 2048 bytes, bindings by name).
  Provenance rows carry `transport = 'fhir'`. The coordinator ruled Option A
  before Lane S6-A's first implementation commit (`.loom/34`, "The
  transport-vs-flag decision"); this entry records it **as applied**.
- Rationale:
  - The two transports have different response semantics. An https delivery
    reads nothing from the response but its status class. A FHIR transaction
    answers `200` with per-entry statuses that must be read to know whether the
    write committed, and a `4xx` carries an OperationOutcome whose issue codes
    are worth recording and whose `diagnostics` must never be. A flag on the
    HTTPS class would have put body parsing into the generic transport.
  - Correction 7's principle survives either way — the transport is a property
    of the revision, never of the workflow — but only a kind makes the 5.1a
    gate's inversion explicit: `assertTransportVocabulary` flips on exactly one
    new admitted value.
  - The digest is stable. Every policy is an `omitempty` pointer in the
    digested JSON, so a nil `fhir` member is elided and no deployed `kafka` or
    `https` digest moves. `TestRevisionDigest_DeployedTransportsArePinned` pins
    one of each as string constants:
    `sha256:9fdfcafa70ea89ffe1ec7aacb4acbf28a918cde02060e35bc2e197004be74f63`
    (https) and
    `sha256:4ac293d946b0091f08a9589ff3bdad601e68167b2ce54d49ace95f7dcceebfd5`
    (kafka).
  - The shared half is shared code, not a copy: `newDestinationClient` in
    `destination/transport.go` builds the TLS client, resolves the credential,
    and applies the trust roots for both `deliverHTTPS` and `deliverFHIR`.
- Alternatives considered:
  - **Option B, `content: fhir-r4-transaction` on `HTTPSPolicy`.** Rejected: it
    contaminates the generic class with FHIR response parsing and makes "does
    this destination receive resources" a sub-field rather than a kind the
    registry, the runtime wiring (`HasTransport`), and the ledger CHECK can
    name.
  - **A destination class value (`class: fhir`).** Rejected: `DestinationClass`
    is an environment class (`production|sandbox`); the gate still asserts no
    class denotes FHIR.
  - **A `runServe` consumer of `integration.delivery.v1`.** Rejected for the
    same reasons 4.1c-b rejected it; the fhir transport runs inside the
    dispatcher's existing lease and `PublishTimeout`, and `errCh` is untouched.
- Consequences:
  - `internal/integration/fhirout` is the durable engine's only door to
    `pkg/fhir`, asserted by the inverted gate. Both engines dispatch through
    its one switch (`MapEvent` for the legacy action, `Project` for the
    transport).
  - Every bundle entry is a conditional `PUT <Type>?identifier=<system>|<value>`
    with a deterministic `urn:uuid:` fullUrl. The at-least-once outbox is safe
    against a FHIR destination (`TestFHIRDestination_RedeliveryIsIdempotent`);
    the pre-slice `POST` builder survives only behind the `fhirpostbundle`
    tag as the negative control.
  - Destination ledger `0003` (`SchemaVersion` 3) records
    `fhir_resource_types`, `fhir_entry_count`, and
    `fhir_outcome_codes_advisory` (issue codes only, never diagnostics).
  - The `fhir` action gains one plan-time diagnostic,
    `FHIR_PROJECTION_UNSUPPORTED`, on a route whose event type has no
    projection; nothing is queued for that action. Coverage is the legacy
    switch's five event types (admit/transfer/update/discharge/lab).
  - Not in scope, deliberately: per-resource PUT, `$process-message`,
    Subscriptions, SMART Backend Services (5.2), an IDE surface for the
    delivery (4.2c), and provider references — delivered literally as
    `Practitioner/<id>`.
- Sources:
  - [S1] `.loom/34-sprint6-execution-specs.md` — Lane S6-A, "The
    transport-vs-flag decision", corrections 1, 5, 7, 8, 9
  - [S2] `internal/integration/destination/{revision.go,transport.go,fhir_transport.go,postgres.go}`,
    `migrations/0003_fhir_delivery_provenance.sql`
  - [S3] `internal/integration/fhirout/{project.go,identifier.go,bundle.go,bundle_request.go,bundle_request_post_control.go}`
  - [S4] `internal/integration/delivery/{transport.go,dispatcher.go,fhir_conformance_gate_test.go,fhir_destination_killtest_test.go}`
  - [S5] `.loom/decisions/2026-09-08-source-assigned-identifiers-are-keyed-under-a.md`
