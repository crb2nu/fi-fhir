### 2026-09-24 - Slice 5.1c-a the mapper's cardinality gaps are closed

- What changed: all seven US Core cardinality violations in the mapper's
  own output are closed, and `recordedCardinalityGaps()` is now an empty map.
  - `MapCareTeam` projects the event's members and returns nil for an event
    that names nobody. A member with no role gets DataAbsentReason `unknown`.
  - `MapCoverage` emits `relationship`: `self` when there is no dependent
    loop, DataAbsentReason `unknown` when there is one.
  - `DocumentReference.content` is `omitempty`. `MapDocumentReference` returns
    nil when no attachment has a url or data.
  - `USCoreMapper.Source` qualifies bare source-assigned identifiers under
    `urn:fi-fhir:source:<source>`.
  - `Encounter.type` is derived from PV1-2: SNOMED CT `86181006` or `4525004`
    plus HL7 v2 table 0004.
  - The vital-sign conformance row now carries the `Timestamp` every shipped
    producer stamps.
  - The fixtures were regenerated with `-update-fhir-golden`. `clean` in the
    ledger test went from 20 to 25. The comment blocks in the CI yml and the
    Makefile were updated, matrix §5.1 now reads "25 clean, ledger empty" with
    the 5.1b table kept as history, and the Encounter type table was added to
    the user guide.
- Why: Sprint 7 Lane S7-A. The 5.1b decision named closing these "the next
  mapper slice" and deferred the DocumentReference question as a product
  decision. That decision is **omit the resource**, not attach the source
  message.
- Evidence:
  - Day-1 gate `TestUSCoreMapper_FixturesCarryNoCardinalityViolations`
    (outside the `TestFHIRStructural` prefix, so both 10-count guards are
    untouched) FAILED on unmodified `1465aa516` naming exactly the seven:
    careteam participant, coverage relationship, documentreference content
    x2, encounter identifier[0].system, encounter type, vitalsign effective.
    It passes at ship.
  - `make fhir-structural`: arity 10, green.
  - `make fhir-structural-negative-control`: red on exactly `patient.json`,
    naming `Patient.name`.
  - `make fhir-conformance` and its negative control: green, and red on
    exactly `MapLabResult`.
  - `make fhir-destination` against PostgreSQL 16 on `7900xtx`: green, with
    the ledger proof run rather than skipped. Its negative control is also
    green.
  - `go test ./...` and `go test -race ./pkg/fhir/... ./internal/integration/fhirout/...`
    pass.
  - `golangci-lint` 2.9.0 local and the CI-pinned v2.12.2 both report
    0 issues on the touched packages.
  - `gofmt -l` is clean.
  - Code choices were checked against tx.fhir.org and the pinned US Core
    9.0.0 package.
- What's next:
  - (1) One line in `internal/integration/fhirout.mapEvent` to set
    `mapper.Source = meta.Source`, so the legacy workflow engine's raw
    Encounter carries the system too. This was out of S7-A's scope.
  - (2) Carry the 271 2100D `INS02` on `EligibilityResponseEvent` so a
    dependent's relationship stops being `unknown`.
  - (3) S7-B should regenerate its official-validator ledger on top of this.
    Terminology bindings were not measured here.
  - (4) `docs/operations/SUPPORTED-1.0.md:86` still says "seven cardinality
    violations". That file belongs to S7-B and the coordinator.
- Sources: `.loom/35-sprint7-execution-specs.md` (Lane S7-A),
  `.loom/decisions/2026-09-24-close-the-mapper-s-us-core-cardinality.md`,
  `pkg/fhir/mapper.go`, `pkg/fhir/uscore_cardinality_test.go`,
  `pkg/fhir/structural_gaps_test.go`, `testdata/fhir/mapper/`.
