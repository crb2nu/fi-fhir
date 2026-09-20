### 2026-09-08 - Slice 4.1c-c day-1 gates: the payload round-trips, redelivery duplicates

- Lane: **S6-A**, Sprint 6 (`.loom/34-sprint6-execution-specs.md`). Branch
  `feat/phase4-slice-4-1c-c-fhir-destination` off `main` @ `a3335a71f`.
  Test-only; touches no shared file. Coordinator ruling cited: **Option A — a
  new `TransportKind = "fhir"` with its own `FHIRPolicy`**, not a flag on the
  HTTPS policy. The implementation MR follows once S6-0's merge surface lands.

- What changed (five test files, no product code):
  - `pkg/integration/fhir_destination_roundtrip_test.go` +
    `pkg/integration/export_test.go` — **kill-test 1**,
    `TestFHIRDestination_DurablePayloadRoundTripsToMapperInput`. Four rows:
    the golden ADT A01 (`testdata/golden/integration/adt-http`) through the real
    `processor.MessageProcessor` (preview mode: same parse, projection and
    `NewProcessedEvent` the durable commit stores), an A01 with the widest PV1
    the strict subset admits through the same path, and an ADT A03 and the
    ORU R01 sample through the real hl7v2 parser plus `NewProcessedEvent`.
    Each takes `PayloadJSON()`, decodes it with `decodeCanonicalEventPayload`
    (reached through an `export_test.go` seam — nothing exported returns the
    concrete event today), maps both the decoded event and the parser's
    original with `MapPatient`/`MapEncounter`/`MapLabResult`, and requires the
    resource sets byte-equal and every resource clean at `us-core`.
  - `internal/integration/delivery/fhir_test_server_test.go` — an in-test FHIR
    server with transaction-Bundle semantics over an identifier-keyed store:
    `POST` always inserts, `PUT <Type>?identifier=system|value` updates in
    place or creates, a bare `?identifier=<value>` is refused with 400, and a
    transaction is all-or-nothing. `Idempotency-Key` is recorded and honoured
    for nothing, as every FHIR server does.
  - `internal/integration/delivery/fhir_destination_killtest_test.go` —
    **kill-test 2**, `TestFHIRDestination_RedeliveryDuplicatesToday`. The spec
    asks for a test that FAILS on main with `want 1 Patient, got 2`; a red test
    cannot merge, so it is encoded as a passing assertion of today's fact and
    the implementation must invert it.
  - `internal/integration/destination/revision_digest_pin_test.go` — the
    digest-stability gate: one `https` and one `kafka` revision pinned as
    string constants.

- Evidence (all on unmodified `main` @ `a3335a71f`):
  - **Kill-test 1 PASSES, all four rows.** The stored payload round-trips into
    the exact `*events.PatientAdmitEvent`, `*events.PatientDischargeEvent`, and
    `*events.LabResultEvent` the mapper consumes; the mapped resources are
    byte-identical to mapping the parser's original; every resource validates
    with zero issues. The blocker is the transport, not the decoder: S6-A's
    first task stays "build the transport".
  - **Kill-test 2: 2 Patients and 2 Encounters** after delivering the same
    attempt id twice through the real `destination.Transport` with the mapper's
    Patient+Encounter in `CreateTransactionBundle` (every entry `POST`). The
    in-test server refuses `PUT Patient?identifier=MRN-000123` with 400 and the
    transport surfaces it as terminal `DELIVERY_DESTINATION_REJECTED`; a
    `PUT ?identifier=urn:oid:1.2.3|MRN-000999` delivered twice answers
    `201 Created` then `200 OK` and the count moves by exactly one.
  - **Digest stability**: `https` revision
    `sha256:9fdfcafa70ea89ffe1ec7aacb4acbf28a918cde02060e35bc2e197004be74f63`,
    `kafka` revision
    `sha256:4ac293d946b0091f08a9589ff3bdad601e68167b2ce54d49ace95f7dcceebfd5`;
    neither encodes a `null` member or a `fhir` member.
  - **Sprint-level negative control**:
    `TestFHIRConformance_DurableEngineProducesNoFHIRResource` PASSES on
    `main` (run 2026-09-08, `go test -race -count=1 -v`, 0.03s). It must go
    red on the implementation branch for all four of its reasons before it is
    rewritten as the inverted gate.
  - `gofmt -l` clean, `go vet` clean, `golangci-lint run` 0 issues,
    `go test -race` green for `pkg/integration`, `internal/integration/delivery`,
    `internal/integration/destination`.

- Premises inverted from code while writing the gates (the implementation
  starts from these, not from the spec's wording):
  1. **A durable visit number can never carry an assigning authority.** The
     executable A01 v1 subset caps `PV1.19` at ONE component
     (`internal/parser/hl7v2/strict_validation.go`, `strictA01FieldComponentLimits`);
     `VISIT^^^HOSP^VN` is rejected with "strict validation rejected extra
     field components" under the golden profile. So every Encounter the
     engine stores has, at best, `{value: <visit>}` with no system, and
     `MapEncounter` emits exactly that bare identifier. Correction 6 ("the
     identifiers needed for conditional writes exist") is true for Patient and
     false for Encounter. The spec's rule "no `system` → projection error" would
     make every durable ADT A01 Encounter undeliverable; the implementation
     qualifies source-assigned identifiers under a deployment-owned system and
     records that as a decision.
  2. **The golden ADT A01 fixture has no PV1 at all** (`expected.json`
     `warning_codes: ["MISSING_PV1"]`), so its Encounter has no identifier
     value either. Under any conditional-write rule that event cannot deliver
     an Encounter; it is the tolerance fixture, not journey 1's message.
  3. **Strict validation admits only ADT A01** (`errStrictMessageType`), so
     the discharge and lab rows cannot go through `MessageProcessor`; they go
     through the same parser without strict validation and then through
     `NewProcessedEvent`, which is the projection the durable engine would
     apply once a projector for those types exists.
  4. **Nothing exported returns the decoded concrete event.**
     `decodeCanonicalEventPayload` is reachable only through
     `ProcessedEvent.UnmarshalJSON`, which re-projects. The implementation's
     exported thin wrapper (task 2) replaces the `export_test.go` seam.

- What's next: after S6-0 (`chore/sprint6-merge-surface`) merges, the
  implementation MR — `internal/integration/fhirout`, `TransportFHIR` +
  `FHIRPolicy`, `deliverFHIR`, the seam widening, destination migration `0003`,
  the plan-time diagnostic, runtime wiring, the inverted gate, docs, and
  `ci/test-fhir-destination.yml`.

- Sources:
  - [S1] `.loom/34-sprint6-execution-specs.md` — corrections 1–11, "The
    Sprint's Riskiest Load-Bearing Assumption", Lane S6-A tasks and the
    lane-level riskiest assumption
  - [S2] `pkg/integration/contracts.go` (`NewProcessedEvent`, `PayloadJSON`,
    `canonicalEventRegistry`, `decodeCanonicalEventPayload`,
    `forbiddenRawPayloadKeys`)
  - [S3] `internal/integration/processor/{message_processor.go,adt_a01.go,profile_compile.go}`;
    `internal/parser/hl7v2/strict_validation.go`
  - [S4] `pkg/fhir/mapper.go` (`MapPatient`, `MapEncounter`, `MapLabResult`,
    `CreateTransactionBundle`), `pkg/fhir/validate.go`
  - [S5] `internal/integration/destination/{revision.go,transport.go}`;
    `internal/integration/delivery/fhir_conformance_gate_test.go`
  - [S6] `.loom/worklog/2026-08-09-slice-5-1a-reconciliation-the-mapper-validates.md`
