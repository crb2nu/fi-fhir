### 2026-09-08: Record the mapper's US Core cardinality gaps rather than fix them in 5.1b

- Decision:
  - Slice 5.1b pins the packages and ships the structural validator, and changes
    **no mapper behaviour**. The nine cardinality violations the validator found
    across six of the twenty-five generated fixtures are recorded in
    `recordedCardinalityGaps()` (`pkg/fhir/structural_gaps_test.go`) and gated by
    **exact equality**: a new violation fails the build, and so does a fixed one.
  - Two of the nine break base FHIR R4, not only a US Core tightening —
    `DocumentReference.content` emitted as JSON `null`, and
    `MedicationRequest.substitution` emitted as `{}` leaving the required
    `substitution.allowed[x]` absent — and they are recorded on the same terms
    as the other seven.
  - Whole archives are pinned rather than an extracted subset, because every
    gate that could have rejected them was checked and none does.
- Rationale:
  - 5.1b's deliverable is a **measurement**: for the first time this repository
    resolves what it emits against the implementation guide it claims to follow.
    Changing the mapper inside the change that first makes the mapper measurable
    would mean the measurement was taken against output nobody had reviewed, and
    the credibility of the number is the whole product of the slice.
  - Seven of the nine are not defect fixes but product decisions. What should a
    `CareTeam` built from an HL7v2 message with no participant segment emit? A
    `DocumentReference` with no attachment? Answering those inside a pinning
    slice would bury a behaviour decision in a diff full of `.tgz` bytes.
  - Fixing them means regenerating the byte-exact fixture set 5.1a created and
    re-running `TestFHIRConformance_MapperOutputValidatesUnderItsOwnChecker`
    over 26 `Map*` entry points. That is a reviewable slice on its own and an
    unreviewable rider on this one.
  - Exact equality rather than a "no new failures" ceiling is what stops the
    ledger becoming a suppression list. A tolerance that only ratchets downward
    still lets a gap outlive the defect that caused it; requiring the ledger to
    change when the code changes makes closing a gap a visible edit.
  - `.loom/34` named "the package `.tgz` files can live in the tree" as this
    lane's riskiest assumption, with partial vendoring as the fallback. The
    fallback was not taken because the evidence did not call for it: trivy
    0.63.0 does not decompress `.tgz` (probed with the exact `security:trivy`
    commands — 0 language-specific files, 0 secrets, exit 0 on both gates),
    there is no `.gitattributes`, no LFS and no size lint, and
    `.dockerignore:20` already excludes `testdata/`. An extraction script would
    also have made §4's external denominator a claim about our script rather
    than about the IG.
- Alternatives considered:
  - **Fix the two base-R4 violations and record the other seven.** Rejected: it
    splits the ledger into "gaps we thought were easy" and "gaps we did not",
    which is a judgement the next reader cannot audit, and it still regenerates
    fixtures inside this slice.
  - **Weaken the validator until the fixtures pass** — grade only top-level
    elements, or downgrade cardinality to warnings. Rejected outright. Every one
    of the nine is a genuine rule in the pinned package; a validator tuned until
    it agrees with the thing it is measuring measures nothing. This is the same
    failure `.loom/34` records as `validate.go` failing open on an unrecognised
    mode string.
  - **Record the gaps as warnings and keep the gate green.** Rejected: 5.1a's
    lesson was that a check nobody can fail is a check nobody reads.
  - **Vendor only the `StructureDefinition` resources v1 needs.** Rejected on
    the evidence above; the fallback stays available if a future gate changes.
  - **Pin US Core's whole transitive dependency closure** (seven packages).
    Rejected: `hl7.terminology.r4` and `us.cdc.phinvads` are terminology this
    validator does not evaluate at all, and only one profile
    (`us-core-questionnaireresponse`, which no mapper emits) has a base outside
    the two pinned archives. The exception is asserted by name, so a second one
    fails the build and forces this decision to be re-made rather than inherited.
- Consequences:
  - `docs/planning/FHIR-CONFORMANCE-MATRIX.md` §5.1 states plainly that six
    fixtures carry nine violations. The repository's public conformance posture
    is now worse-looking and more accurate than it was yesterday, which is the
    trade this slice makes.
  - The next mapper slice has a precise, enumerated, machine-checked work list,
    and closing any item on it requires editing the ledger in the same commit.
  - `pkg/fhir/validate.go` is unchanged, so 5.1a's `make fhir-conformance` gate
    and its negative control stay green and keep meaning what they meant.
  - Two places where `validate.go` is stricter than US Core 9.0.0 —
    `Patient.gender` and `Patient.birthDate`, both `0..1` in the IG — are now
    recorded as deliberate local policy and asserted, so they cannot drift
    silently in either direction.
- Sources:
  - [S1] `.loom/34-sprint6-execution-specs.md` — Lane S6-D scope, acceptance,
    and riskiest assumption.
  - [S2] `.loom/decisions/2026-08-08-fhir-conformance-validation-strategy-for-slice-5.md`
    — the ratified confinement half this slice does not disturb.
  - [S3] `pkg/fhir/structural_gaps_test.go` — the ledger and the nine entries.
  - [S4] `testdata/fhir/packages/README.md` — the gate probes behind the
    whole-archive choice.
