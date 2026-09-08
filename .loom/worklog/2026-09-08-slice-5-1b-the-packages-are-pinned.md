### 2026-09-08 - Slice 5.1b — the packages are pinned and the mapper is measurable

- What changed:
  - `hl7.fhir.r4.core#4.0.1` (12,815,597 bytes) and `hl7.fhir.us.core#9.0.0`
    (2,749,959 bytes) are checked in whole as offline `.tgz` under
    `testdata/fhir/packages/`, with `SHA256SUMS` and matching constants in Go.
  - `pkg/fhir/structural.go` — a stdlib-only structural validator (Option C)
    that resolves profiles, walks `baseDefinition` chains, and enforces
    cardinality from both the R4 base snapshot and every resolved profile
    snapshot at every element depth. No new `go.mod` dependency.
  - `make fhir-structural` (CI `test:fhir-structural`, blocking) and
    `make fhir-structural-negative-control`.
  - `FHIR-CONFORMANCE-MATRIX.md` §4 headline, §5 row 3, new §5.1;
    `SUPPORTED-1.0.md` standards row and profile-version policy section.
- Why:
  - `.loom/34` correction 16: nothing was pinned, so `validate.go`'s
    required-element list and its 32 profile constants were literals nothing
    checked, and §4's external denominator was explicitly "not citable".
- Evidence:
  - The lane's riskiest assumption — "the `.tgz` files can live in the tree" —
    held, and was tested rather than assumed. Trivy 0.63.0 (the pinned
    `TRIVY_VERSION`) was run in a container with the exact `security:trivy`
    commands against a directory holding only the two archives: `--scanners
    vuln` reported `Number of language-specific files num=0` and exited 0,
    `--scanners secret` reported no issues and exited 0. Trivy does not
    decompress `.tgz`. Then the same two commands, with the job's full
    `--skip-dirs` flags, were run against the whole branch tree with both
    archives in place: both exited 0 and the archives contributed no scan
    target. No `.trivyignore` entry and no `--skip-dirs` was added; adding one
    would have suppressed findings nobody had shown existed. There is no
    `.gitattributes`, no LFS, and no file-size lint. `.dockerignore:20` is
    `testdata/`, so the confinement half of the 2026-08-08 decision holds
    unchanged: nothing from an IG package reaches the image.
  - Both archives reproduce the registry-published `dist.shasum` (SHA-1) byte
    for byte — `0e4b8d99…` for R4 core, `bc995598…` for US Core — so the
    provenance check is independent of our own SHA-256.
  - §4's *unverified* external headline was right. The pinned package holds 70
    `StructureDefinition`s: 55 resource profiles (matching the 55 the IG page
    reported on 2026-08-08) and 15 complex-type constraints. All 32 profile
    constants resolve to one of the 55; 23 profiles have neither a constant nor
    a mapper. §4 is now citable and unchanged.
  - **19 of 25 mapper fixtures validate clean. Six carry nine cardinality
    violations**, every one confirmed by hand against the fixture:
    `CareTeam.participant`, `Coverage.relationship`, `Encounter.type`,
    `Encounter.identifier.system`, `Observation.effective[x]` on the heart-rate
    profile, and two that break base R4 rather than a US Core tightening —
    `DocumentReference.content` emitted as JSON `null`, and
    `MedicationRequest.substitution` emitted as `{}` so `substitution.allowed[x]`
    is absent. Recorded in `recordedCardinalityGaps()` and asserted by exact
    equality, so a fixed gap fails the build as loudly as a new one.
  - One reported violation was a validator defect, not a mapper defect, and was
    fixed rather than recorded: `Coverage.payor` is `1..*` in R4 and `1..1` in
    US Core, and the first draft called the one-element JSON array a violation.
    A profile narrowing `max` constrains the *count*; JSON array-ness is fixed
    by the base resource definition. Array-ness is now graded only against
    specializations.
  - The pinned profiles were compared against `validate.go`'s 17 hand-written
    required-element checks. Fifteen agree exactly. Two are the shipped checker
    being **stricter** than the IG: `Patient.gender` is `0..1` with no
    `mustSupport` flag at all in US Core 9.0.0, and `Patient.birthDate` is
    `0..1` mustSupport, while `validate.go:187-188` makes both hard errors.
    Neither can produce a non-conformant `Patient`, so nothing was changed;
    the divergence is asserted so it cannot drift silently.
  - One US Core base chain leaves the pinned set:
    `us-core-questionnaireresponse` bases on `hl7.fhir.uv.sdc`. The transitive
    closure (seven declared dependencies, mostly terminology) was not pinned;
    the exception is asserted by name so a second one fails.
  - Local: `gofmt -l`, `go build ./...`, `go vet ./...`, `golangci-lint run`,
    `go test -race ./pkg/fhir/...`, `make fhir-conformance` (5.1a's gate,
    untouched and green), `make fhir-structural`,
    `make fhir-structural-negative-control` (fails on exactly `patient.json`,
    naming `Patient.name`).
- What's next:
  - The mapper slice that closes the nine gaps. Two of them —
    `DocumentReference.content: null` and `MedicationRequest.substitution: {}` —
    are defects on any reading; the other seven need a decision about what to
    emit when the source message carries nothing.
  - Option A: `validator_cli.jar` as a CI-only job, Sprint 7.
- Sources:
  - [S1] `.loom/34-sprint6-execution-specs.md` — Lane S6-D, correction 16, and
    the lane's riskiest assumption.
  - [S2] `.loom/decisions/2026-08-08-fhir-conformance-validation-strategy-for-slice-5.md`
    — the ratified confinement half.
  - [S3] `.loom/worklog/2026-08-09-slice-5-1a-reconciliation-the-mapper-validates.md`
    — the fixture set and the version policy this slice resolves.
  - [S4] `.loom/decisions/2026-09-08-record-the-mapper-s-us-core-cardinality.md`
    — why the gaps are recorded rather than fixed here.
  - [S5] `testdata/fhir/packages/README.md` — provenance and the gate evidence.
