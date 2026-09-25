### 2026-09-24: Close the mapper's US Core cardinality gaps

- Decision: Slice 5.1c-α closes all seven cardinality violations that
  `recordedCardinalityGaps()` recorded, and the ledger is now an empty map. No
  entry was left in it. Each fix uses the source event's data where the event
  carries it. Where it does not, the mapper follows US Core's Missing Data
  rule, or it does not emit the resource. It never invents a value. The six
  choices:
  1. **`CareTeam.participant` (1..\*)**: `MapCareTeam` projects the event's
     members. A member counts only if it names somebody: a practitioner NPI, id
     or name, or an organization id or name. An event that names nobody maps
     to **nil**, so no CareTeam is emitted. A practitioner known only by name
     gets a display-only reference, not a literal reference to an id nobody
     issued. A member with no role gets `role` =
     `http://terminology.hl7.org/CodeSystem/data-absent-reason#unknown`, which
     is what US Core's own `CareTeam-missing-coded-data-example` sends. This
     is needed because `participant.role` is 1..1.
  2. **`Coverage.relationship` (1..1)**: `self`
     (`http://terminology.hl7.org/CodeSystem/subscriber-relationship#self`)
     when the eligibility response carries no dependent. X12 271 puts the
     patient in the 2100C subscriber loop and adds a 2100D dependent loop only
     when the patient is not the subscriber, and the EDI mapper sets
     `Dependent` exactly when that loop is present. When there is a dependent,
     the value is DataAbsentReason `unknown`. The real value is in the 2100D
     `INS02`, which the parser reads but the canonical event does not carry.
     `child`, `spouse` or `other` would be guesses, and `other` asserts a
     relationship outside the listed ones. The spec's "IN1-17 / IN2 family"
     does not apply: `MapCoverage` consumes the X12 271
     `EligibilityResponseEvent`, not an HL7 v2 insurance segment.
  3. **`DocumentReference.content` (1..\* in R4 and US Core)**: there are two
     layers. The serialisation defect is fixed: `content` is now `omitempty`,
     so it can never be written as JSON `null`. The product decision is
     **omit the resource**. `MapDocumentReference` maps to nil unless at least
     one content entry's attachment has a `url` or `data`. That is US Core's
     us-core-6 invariant, so an entry with neither is dropped. The other
     option, attaching the source message as `x-application/hl7-v2+er7`
     content-addressed, was rejected. The attachment would not be the
     document that `type` describes. It would copy every segment of the source
     message into a resource that claims to be one clinical note. It would
     only work for HL7 v2 sources, and no CDA, FHIR or CSV producer has an
     er7 message.
  4. **`Encounter.identifier.system` (1..1)**: `USCoreMapper` gains a
     `Source` field, which is the event's server-stamped `EventMeta.Source`.
     When it is set, an identifier with no system of its own (no CX.4, no
     assigning-authority map, no `IdentifierSystemMap` entry) is qualified as
     `urn:fi-fhir:source:<url.PathEscape(source)>`, the namespace the
     2026-09-08 decision rules for every source-assigned identifier. This
     covers the Encounter's `e.ID` visit-number fallback and bare identifiers
     in `mapIdentifiers` for both Patient and Encounter. When `Source` is
     empty, the identifier stays systemless as before. A mapper that does not
     know the source has nothing true to name. The durable `fhir` transport
     already qualified the delivered Encounter this way at bundle time
     (`fhirout.ensureIdentifier`). `TestUSCoreMapper_SourceSystemAgreesWithFhirout`
     pins the mapper's mint to `fhirout.SourceIdentifierSystem` byte for byte,
     so the two cannot drift into a duplicated identifier.
  5. **`Encounter.type` (1..\*)**: this is derived from PV1-2 (patient class),
     because that is the only encounter-kind fact the canonical Encounter
     carries. US Core 9.0.0 binds `Encounter.type` *extensible* to VSAC
     `2.16.840.1.113762.1.4.1267.23`. That value set's SNOMED CT half,
     `2.16.840.1.113883.11.20.9.52` v20241017, is every descendant of
     308335008 |Patient encounter procedure| and of 185316007. Membership was
     checked against tx.fhir.org on SNOMED CT International 20250201 and US
     edition 20250901:
     - `I` gets SNOMED CT `86181006` |Evaluation and management of inpatient|,
       plus `http://terminology.hl7.org/CodeSystem/v2-0004#I`.
     - `E` gets SNOMED CT `4525004` |Emergency department patient visit|, plus
       `v2-0004#E`.
     - `O`, `P`, `R`, `B`, `C`, `N` and `U` get the `v2-0004` code only. The
       extensible binding permits this when the value set has no suitable
       concept, and none here fits. The outpatient concepts (`30346009`,
       `37894004`, both outside the current value set) assert new versus
       established and an office setting, and PV1-2 says neither. `32485007`
       |Hospital admission| is *not* in the value set.
     - The long forms (`INPATIENT`, `IMP`, `OUTPATIENT`, `AMB`, `EMERGENCY`,
       `EMER` and so on) normalise to the table 0004 code.
     - An unrecognised class is sent as `text` only. No class at all is
       DataAbsentReason `unknown`. Both follow US Core Missing Data.
  6. **`Observation.effective[x]` (1..1, vital signs)**: `effectiveDateTime`
     is the event's `Timestamp`. That is where every producer puts the
     clinically relevant time after resolving its own fallback order. For CDA
     the order is observation `effectiveTime`, then organizer `effectiveTime`,
     then document `effectiveTime`. For FHIR it is `effectiveDateTime`. The
     HL7 v2 order the spec names (OBX-14, then OBR-7, then MSH-7) belongs in
     a producer's extractor, the way `extractADTA01SourceTimes` does it for
     A01. No HL7 v2 producer emits a `VitalSignEvent` today, so no such
     extractor exists yet. The fixture gap came from the conformance row, not
     the mapper: that row's event had no `Timestamp`, which is not the shape
     either shipped producer emits. The row now carries one. A zero
     `Timestamp` stays absent. It is never filled with `ReceivedAt`, because
     receipt time is not measurement time.
- Rationale:
  - Where the event has the data, the fix uses it. Where it does not, US Core
    already says what to send: `unknown` from DataAbsentReason for a required
    coded element whose value set has no unknown concept, or text only when
    the source has text and no code. Using that rule means the product tells
    the receiver it does not know rather than filling in a plausible value.
  - Omitting a resource is the right answer when no truthful value can fill a
    required element. An empty CareTeam and a DocumentReference with no
    document describe nothing a receiver can act on. Both are R4/US Core
    violations that no fallback could make true.
  - Every choice is pinned by a test that also runs the fallback shape through
    the structural validator. The fixtures prove the representative row, and
    `pkg/fhir/uscore_cardinality_test.go` proves the shapes the fixtures never
    reach.
- Alternatives considered:
  - **Leave individual entries in the ledger.** None needed it. Every fixture
    gap closed from event data, and the fallback shapes use US Core's own
    missing-data rule rather than fabricated clinical content.
  - **Attach the source message to a content-less DocumentReference.**
    Rejected in (3).
  - **`other` or a guessed code for a dependent's relationship.** Rejected in
    (2).
  - **Fill a missing vital-sign time with `ReceivedAt`.** Rejected: it would
    invent a clinical time.
  - **Use the DataAbsentReason extension on `_effectiveDateTime` for a vital
    sign with no time.** US Core permits this for mandatory non-coded
    elements. It was deferred because this repository's structural validator
    reads no primitive extensions and would report the element missing. The
    mapper must not emit what its own validator rejects, which was the
    lesson of 5.1a.
  - **Set `USCoreMapper.Source` in `internal/integration/fhirout`.** This is
    one line in `mapEvent`, and it is the only way the legacy workflow
    engine's Encounter gains a system. It is out of this lane's scope, since
    the spec rules out any change to fhirout. It is left as a named
    follow-up.
- Consequences:
  - `MapCareTeam` and `MapDocumentReference` can now return nil for a non-nil
    event. The `Mapper` interface documents this. No production caller maps
    either type today: `fhirout` does not project `care_team` or
    `document_reference`.
  - Durable `fhir` bundles gain `Encounter.type`. The durable Encounter
    identifier is byte-identical, because `ensureIdentifier` already added the
    system. The legacy workflow engine's raw Encounter still has a systemless
    visit number until the `fhirout` follow-up lands.
  - `recordedCardinalityGaps()` is empty and still gated by exact equality.
    `TestUSCoreMapper_FixturesCarryNoCardinalityViolations` asserts zero
    independently of the ledger. It failed on `main` at `1465aa516` naming
    exactly the seven.
  - Terminology is still unmeasured here. Whether the chosen codes satisfy
    their bindings is Slice 5.1c-β's (S7-B) measurement, and its ledger
    should be regenerated after this merges.
  - The follow-up data gap is that the canonical `EligibilityResponseEvent`
    should carry the 2100D `INS02` relationship code. That would turn the
    dependent case's `unknown` into a real code.
- Sources:
  - [S1] `.loom/35-sprint7-execution-specs.md`, Lane S7-A
  - [S2] `.loom/decisions/2026-09-08-record-the-mapper-s-us-core-cardinality.md`
  - [S3] `.loom/decisions/2026-09-08-source-assigned-identifiers-are-keyed-under-a.md`
  - [S4] `testdata/fhir/packages/hl7.fhir.us.core-9.0.0.tgz`:
    `StructureDefinition-us-core-{careteam,coverage,documentreference,encounter}.json`,
    `example/CareTeam-missing-coded-data-example.json`
  - [S5] US Core Missing Data guidance: https://hl7.org/fhir/us/core/general-requirements.html#missing-data
  - [S6] tx.fhir.org `$expand` / `$subsumes` / `$lookup` on SNOMED CT, `v2-0004`,
    `subscriber-relationship`, `data-absent-reason` (2026-09-24)
  - [S7] `pkg/fhir/mapper.go`, `pkg/fhir/uscore_cardinality_test.go`,
    `pkg/fhir/source_identifier_agreement_test.go`
