### 2026-09-08: Source-assigned identifiers are keyed under a deployment-owned system

- Decision: A FHIR conditional write emitted by the durable `fhir` transport is
  keyed on an identifier that **always carries a system**. The system is the
  canonical identifier's own when the source supplied one (the source profile's
  assigning-authority map, or a CX.4 universal ID); otherwise it is the
  deployment-owned `urn:fi-fhir:source:<source_id>`, where `source_id` is the
  server-stamped `EventMeta.Source` of the event. A resource with **no
  identifier value at all** — a Patient with no MRN, an Encounter with no visit
  number, a lab result with neither an order number nor a source message id —
  is a projection error (`DELIVERY_FHIR_PROJECTION_FAILED`, terminal, no request
  made), never a `POST`. The key is written into the delivered resource's
  identifiers (a bare identifier with the same value gains the system rather
  than being duplicated), so the destination's next match finds this write. An
  SSN is never a key.
- Rationale:
  - `.loom/34`'s lane-level riskiest assumption says `?identifier=|MRN-1`
    matches any system and asks Task 2 to refuse a projection whose chosen
    identifier has no system. The **intent** — never emit a systemless
    conditional — is kept in full.
  - The **literal** rule is unimplementable on the durable path, found by the
    day-1 gates: the executable ADT A01 v1 subset caps `PV1.19` at ONE
    component (`internal/parser/hl7v2/strict_validation.go`,
    `strictA01FieldComponentLimits`), so a durable visit number can never carry
    an assigning authority, and `MapEncounter` emits exactly `{value}` with no
    system. Correction 6 ("the identifiers needed for conditional writes
    exist") is true for Patient and false for Encounter. A refuse-only rule
    would dead-letter every ADT A01 the engine can admit, and the acceptance
    criterion — a production admit delivering Patient **and** Encounter —
    could only be "proven" with a hand-built event the engine never stores,
    which is exactly what correction 3 forbids.
  - Naming the source is not guessing a foreign system. HL7 semantics for a CX
    with no assigning authority are "assigned by the sending facility", and the
    sending facility is the integration source. The derived system is
    deterministic (a redelivery keys the same resource) and scoped by source
    (two sources' bare values cannot collide). It is the same class of choice
    5.1a made when it backfilled an MRN-only Patient under the mapper's `MR`
    system — except this one is scoped to the deployment's source rather than
    to an `example.org` placeholder.
- Alternatives considered:
  - **Refuse every systemless identifier (spec-literal).** Rejected above:
    journey 1 could never deliver an Encounter from the durable path.
  - **Widen the strict A01 subset so PV1.19 accepts a CX with an assigning
    authority.** Not this lane's file (`internal/parser/hl7v2/**` is unowned
    this sprint) and not sufficient: sources that send a bare visit number
    would still need a rule. Worth doing later so the first case applies more
    often.
  - **Key on the mapper's placeholder systems** (`http://hospital.example.org/mrn`
    for `MR`). Rejected: two facilities' MRNs collide under one placeholder on
    one server — the exact risk the assumption names.
  - **Include the tenant in the derived system.** Not done in v1: a fi-fhir
    deployment is single-tenant (`deploymentTenantID` on both resolvers and the
    registry) and the legacy engine's events carry no tenant. Operators feeding
    one FHIR server from several deployments must keep `source_id`s distinct;
    documented in `DESTINATION-IDENTITY.md`'s operator checklist. Flagged for
    the coordinator as a follow-up if multi-tenant deployments arrive.
  - **Omit an unkeyable Encounter and deliver the Patient alone.** Rejected:
    silently dropping a resource is worse than a loud dead letter the operator
    can trace to a feed that omits PV1.
- Consequences:
  - `fhirout.SourceIdentifierSystem(source)` is the one place the namespace is
    minted; `identifier.go` documents the preference order per resource type
    (Patient: `MR`-typed → MRN → any non-SSN; Encounter: system-qualified →
    bare → visit number; DiagnosticReport: order number → source message id;
    Observation: `<report>#obs-N`).
  - The golden `adt-http` fixture (no PV1 at all) dead-letters under a `fhir`
    destination with `DELIVERY_FHIR_PROJECTION_FAILED`; under `https` or
    `kafka` it delivers the envelope as before. That is the tolerance fixture,
    not journey 1's message.
  - `TestProjectRefusesResourcesWithoutAUsableKey` and the orphan-Encounter
    half of `TestFHIRDestination_RedeliveryIsIdempotent` keep the refusal live;
    the in-test FHIR server refuses a bare `?identifier=<value>` with 400 so
    the rule is enforced by the destination side of the proof as well.
- Sources:
  - [S1] `.loom/34-sprint6-execution-specs.md` — Lane S6-A task 2, "Riskiest
    Assumption (lane-level)", corrections 3 and 6
  - [S2] `.loom/worklog/2026-09-08-slice-4-1c-c-day-1-gates.md` — premise 1
  - [S3] `internal/parser/hl7v2/strict_validation.go` (`strictA01FieldComponentLimits`, `PV1.19` max 1)
  - [S4] `internal/integration/fhirout/identifier.go`, `project_test.go`
  - [S5] `pkg/fhir/mapper.go` (`MapEncounter`, `appendMRNIdentifier`)
