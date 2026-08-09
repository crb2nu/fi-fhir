### 2026-08-09 - Slice 5.1a reconciliation: the mapper validates under its own checker

- Lane: **S5-E**, Sprint 5 (`.loom/33-sprint5-execution-specs.md`). Branch
  `feat/phase5-slice-5-1a-reconciliation`, **stacked on**
  `feat/phase5-slice-5-1a-conformance-reconciliation` (the day-1 gates, MR !168).
  Rebase this branch onto `main` after !168 merges before judging its MR diff —
  GitLab re-authors on merge, so the merge-base regresses and the diff will drag
  in the parent's files until it is rebased.

- Owned files (unchanged from the day-1 entry; no other lane touches them):
  `pkg/fhir/**`, `pkg/validate/**`, `testdata/fhir/**`,
  `internal/fhir/subscription/**` (statement only — no code change),
  `docs/planning/FHIR-*.md`, `.loom/28`, the S5-E amendment to `.loom/40-decisions.md`,
  one `.PHONY` line and one target block in `Makefile`.
  **Two files outside that list**, both flagged for the coordinator:
  - `cmd/fi-fhir/main.go` — the `--mode` rejection. Unavoidable: the fail-open
    hole (correction 45, task 4) is half in the CLI. The edit is ~8 lines inside
    `runFHIRValidate` plus three lines of usage text, and it is nowhere near
    S5-C's print sites or S5-D's `runServe` component table.
  - `docs/operations/SUPPORTED-1.0.md` — a new "FHIR profile-version assertion
    policy" section plus one clause appended to the standards row. The
    acceptance criteria require the policy to be stated there. S5-A owns the
    profile-ambiguity rows and S5-B the deployment rows; this touches neither.
  - `CHANGELOG.md` — the shared `[Unreleased]` append point.
  - **No migration number claimed. No `.gitlab-ci.yml` change** — S5-0a has not
    merged, and both proofs are ordinary tests that need no job.

- What changed (three commits):

  1. **`fix(fhir)` — the reconciliation.** Four disagreements between the mapper
     and the checker, plus the fail-open mode.
  2. **`test(fhir)` — fixtures and coverage numbers.** Generated golden fixtures
     for what the mapper actually produces; every published coverage number
     computed from the checker.
  3. **`docs(fhir)` — the 5.1 documents repaired against code**, and the inbound
     surface scoped.

- The five repairs, and why each was decided the way it was:

  - **DiagnosticReport (correction 41).** US Core defines *two* DiagnosticReport
    profiles and this package produces both: `MapLabResult` → `-lab` (category
    LAB, LOINC-coded, referencing lab Observations) and `MapDiagnosticReportNote`
    → `-note`. Only `-note` was accepted, and `-lab` was a bare literal that was
    never declared as a constant — which is exactly how it fell out of the set.
    **The mapper was right and the accepted set was incomplete.** Re-stamping a
    lab panel `-note` to satisfy the checker would have made the resource wrong
    to satisfy a check that was wrong. Added
    `USCoreDiagnosticReportLabProfile`, used it at the one call site, accepted
    both.
  - **Fail-open mode (correction 45).** `Mode` was an open string compared
    byte-exactly against `us-core`. Now `ValidationMode` +
    `ParseValidationMode` + `ValidationModes`: case-insensitive,
    whitespace-trimmed, closed at `{none, us-core}`, anything else is
    `ErrUnknownValidationMode`. **The empty string is an error too** — a
    zero-value `ValidationOptions` must not be the configuration that silently
    checks nothing. That is the one breaking change in the slice and it is
    called out in the CHANGELOG. `internal/workflow`'s `fhirValidationMode`
    already normalised case and already defaulted unknown → `us-core`, so the
    workflow path needed no change and S5-C's file was not touched.
  - **Version policy (correction 42; the decision entry's own open item).**
    Chosen: **the mapper asserts bare canonicals; the checker accepts a bare
    canonical or any `|version`-pinned form.** `ProfileCanonical` strips the
    suffix before the membership test. Pinning all constants to `|9.0.0` was
    rejected: this checker has no package-resolution step, so a pinned constant
    would assert a version it cannot verify *and* would reject a correct bare
    canonical. Version *tolerance* is not version *resolution*, and the docs say
    so in those words.
  - **`Patient.MRN` (correction 46).** Dropped entirely, producing a hard
    `[error] Patient.identifier is required (US Core)` — an ERROR, unlike the
    DiagnosticReport warning. Backfilled as an `MR`-typed identifier,
    value-deduplicated so a producer that already expressed the MRN under any
    type does not get a second copy.
  - **Duplicate LOINC coding.** A parser filling both `LabTest.LOINCCode` and
    `LabTest.Code.Coding` — the normal shape — emitted the same `(system, code)`
    twice. Deduped.

- Evidence:
  - `make fhir-conformance` — green.
    `TestFHIRConformance_MapperOutputValidatesUnderItsOwnChecker` drives all 26
    `Map*` entry points with representative events, marshals every resource, and
    requires **zero** issues at `--mode us-core --strict`. The table is bound to
    the type by reflection and the arity is asserted at 26, so adding a `Map*`
    without a row turns it red.
  - `make fhir-conformance-negative-control` — `negative control OK: restoring
    the -note-only set fails exactly the MapLabResult row`. The control does not
    merely assert *that* something fails: it parses the `--- FAIL` subtest lines
    and requires the failing set to be exactly `{MapLabResult}`.
  - `go test ./pkg/... ./cmd/fi-fhir/...` — all green. `go vet` clean under no
    tags, `integration`, and `fhirdrnoteonly`. `golangci-lint` 0 issues.
  - `bash scripts/validate-docs.sh` and `scripts/worklog.sh check` — pass.
  - Coverage numbers, all now computed by
    `TestFHIRConformance_CheckerCoverageIsDerivableFromCode` rather than
    transcribed: **24** non-Bundle resource types produced; **6** with
    required-element checks (Condition, Coverage, DiagnosticReport, Encounter,
    Observation, Patient) — correction 44's figure, exact; **21** with
    profile-presence checks; exactly **{Claim, CoverageEligibilityResponse,
    ExplanationOfBenefit}** with neither; **32** profile constants (31 before
    this slice), **0** version-pinned, **1** declared-but-unused
    (`USCoreMedicationProfile`).

- Two design notes worth keeping:
  - **The golden fixtures are generated, not hand-written.** Every fixture in
    `testdata/fhir/` was hand-written, so the mapper and the checker were each
    tested against a third artefact and never against each other — which is
    precisely how a validator that rejects its own mapper's output stayed green
    in CI for the life of the package. `testdata/fhir/mapper/` now holds the
    mapper's exact bytes for all 21 checked types, asserted byte-equal, so
    mapper drift is a reviewable diff instead of a silent pass. Regenerate with
    `go test ./pkg/fhir -run MapperGoldenFixtures -update-fhir-golden`.
  - **`hasRequiredElementCheck` probes the checker instead of copying its switch
    statement.** A resource carrying nothing but its type and its expected
    profile reports an issue only if a required-element check exists. That is why
    the "6 of 24" figure cannot rot: it is measured, not maintained.

- The three tasks that were answered rather than coded:
  - **`internal/fhir/subscription` (correction 50, task 8)** — inbound
    conformance is declared **out of scope** for 5.1, in a new §6 of
    FHIR-CONFORMANCE-MATRIX.md, with the reasoning and with the three questions
    a future slice must answer first (reject / warn / accept-and-record; where
    the outcome is observable to the sender; in-process or pre-ingest). Written
    down so the answer is not picked by accident. No code.
  - **`USCoreMedicationProfile`** — kept, not deleted. Deleting an exported
    constant is a breaking API change and it is the correct canonical for the day
    a Medication mapper exists. Documented at its declaration as
    declared-but-unused and excluded from the coverage count.
  - **FHIR-PROFILES.md's 6.1.0 references** — annotated in place, not bumped to
    9.0.0. Re-versioning a design document without re-verifying its must-support
    tables against the 9.0.0 package would replace a visible staleness with an
    invisible one. That belongs to the slice that pins the IG.

- What's next: **Slice 5.1b** — pin `hl7.fhir.r4.core#4.0.1` and
  `hl7.fhir.us.core#9.0.0` as offline `.tgz`, then Option C (a Go structural
  validator over them), then Option A (`validator_cli.jar`, CI-only). And
  **4.1c-c**, the FHIR destination class, which is 5.1's real prerequisite and
  needs a coordinator decision to exist at all. Nothing in 5.1b certifies a live
  path until 4.1c-c lands.

- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` — Lane S5-E tasks 1-9, acceptance
    criteria, corrections 40-52, coordinator ruling 3
  - [S2] `pkg/fhir/validate.go` (`ParseValidationMode`, `ProfileCanonical`,
    `expectedProfilesForResourceType`); `profiles_diagnosticreport.go` and its
    `fhirdrnoteonly` control; `types.go` constant block; `mapper.go`
    (`MapLabResult`, `MapPatient`, `appendMRNIdentifier`, `mapLabCode`)
  - [S3] `pkg/fhir/conformance_test.go`, `golden_mapper_test.go`,
    `validate_golden_test.go`; `testdata/fhir/mapper/` (25 files);
    `testdata/fhir/diagnosticreport_uscore_lab.json`
  - [S4] `cmd/fi-fhir/main.go` `runFHIRValidate`; `internal/workflow/actions.go`
    `fhirValidationMode` (already fail-closed, untouched)
  - [S5] `docs/planning/FHIR-CONFORMANCE-MATRIX.md` §§0,1.1,4,5,6;
    `docs/planning/FHIR-PROFILES.md`; `docs/operations/SUPPORTED-1.0.md`
  - [S6] `.loom/40-decisions.md` 2026-08-09 amendment; `.loom/28` "has been RUN"
  - [S7] `.loom/worklog/2026-08-09-slice-5-1a-day-1-gates-the.md` — the gates
