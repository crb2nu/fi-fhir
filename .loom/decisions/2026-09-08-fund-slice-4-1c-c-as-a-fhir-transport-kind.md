### 2026-09-08: Fund Slice 4.1c-c as a new `fhir` transport kind with its own policy (Option A)

- Decision:
  - Slice 4.1c-c, the FHIR destination class, is funded as Sprint 6 Lane S6-A
    (`.loom/34-sprint6-execution-specs.md`). It ships as **a new
    `destination.TransportKind = "fhir"` with a `FHIRPolicy{BaseURL,
    TokenBinding, CABundleBinding, Interaction}`**, validated the way
    `HTTPSPolicy` is, executed by its own `deliverFHIR` inside the existing
    dispatcher lease, and recorded in the destination ledger as
    `transport = 'fhir'` via migration `0003`.
  - The resource is produced by `pkg/fhir.USCoreMapper` over the durable
    payload decoded through `pkg/integration`'s existing canonical event
    registry — no second mapper — and delivered as a transaction Bundle whose
    entries are **conditional updates** (`PUT <Type>?identifier=<system>|<value>`).
    A resource whose identifier has no `system` is refused at projection, never
    sent as a `POST`.
  - Version 1 projects the three event types the legacy engine has ever
    dispatched (`PatientAdmitEvent`, `PatientDischargeEvent`,
    `LabResultEvent`); any other event type behind a `fhir` action is a
    plan-time diagnostic (`FHIR_PROJECTION_UNSUPPORTED`), not a dispatch failure.
- Rationale:
  - The Sprint 5 coordinator ruled on 2026-08-09 that 4.1c-c would receive "a
    dedicated coordinator-owned spec pass targeting Sprint 6" (`.loom/33`,
    coordinator ruling 1). The pass is `.loom/34`; this entry is the ruling on
    the shape question it left open.
  - A transport kind, not a flag on `HTTPSPolicy`, because a FHIR transaction
    response is a `200` with per-entry statuses and a `4xx` carries an
    `OperationOutcome` that must be bounded and stripped of `diagnostics` text
    before touching a ledger. Folding that into the generic HTTPS class would
    contaminate a path that is proven and byte-identical today. A distinct
    kind also makes the 5.1a gate's inversion explicit: exactly one new
    admitted vocabulary value.
  - Conditional writes, because the outbox is at-least-once and
    `pkg/fhir.CreateTransactionBundle` emits `POST` entries; the HTTPS
    transport's `Idempotency-Key` header is not honoured by FHIR servers. A
    lease reclaim would create a second Patient. The sprint's riskiest
    assumption is written down as a day-1 kill-test for exactly this.
- Alternatives considered:
  - **Option B — an encoding flag on `HTTPSPolicy`.** Rejected for the response-
    semantics and provenance reasons above.
  - **Reuse `CreateTransactionBundle` as-is.** Rejected: passes every existing
    test and duplicates every patient on the first redelivery.
  - **Cover all 26 `Map*` entry points in v1.** Deferred: journey 1 is ADT and
    the legacy switch has only ever executed three types; the plan-time
    diagnostic makes the boundary visible instead of silent.
- Consequences:
  - Destination ledger unfrozen for `0003` (`SchemaVersion` 2→3) under the 4.4a
    rules; every other ledger stays frozen this sprint.
  - `TestFHIRConformance_DurableEngineProducesNoFHIRResource` is deliberately
    inverted, not deleted; its doc comment stays as the record of the world
    before a FHIR destination class existed.
  - The IDE cannot yet show the delivery: `OperatorDeliveryAttempt` carries no
    provenance-ledger field. That is Slice 4.2c, a separate gqlgen-touching
    slice named in `.loom/34` Wave 3.
- Sources:
  - `.loom/34-sprint6-execution-specs.md` — corrections 1–11, Lane S6-A
  - `.loom/33-sprint5-execution-specs.md` — correction 40, decision 1, ruling 1
  - `internal/integration/delivery/fhir_conformance_gate_test.go`
  - `pkg/integration/contracts.go` (`canonicalEventRegistry`,
    `decodeCanonicalEventPayload`, `forbiddenRawPayloadKeys`)
  - `pkg/fhir/mapper.go` (`CreateTransactionBundle`)
