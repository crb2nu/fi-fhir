### 2026-09-24 - Slice 5.1c-b the official validator gates the delivered bundles

- What changed:
  - `scripts/fhir-official-validate.sh` runs HL7 `validator_cli.jar` 6.10.4
    offline over the 25 mapper fixtures and the 11 Bundles the durable `fhir`
    transport delivers, and holds the findings to
    `testdata/fhir/official/findings.ledger.txt` by exact equality. Modes
    `gate`, `negative-control`, `update`; `make fhir-official`,
    `make fhir-official-negative-control`.
  - `ci/test-fhir-official.yml`: `test:fhir-official-capture` (Go) and
    `test:fhir-official` (eclipse-temurin JRE, `needs:` the capture), both
    blocking. One include line; `ci/job-inventory.txt` regenerated.
  - `internal/integration/fhirout/capture_test.go`:
    `FI_FHIR_FHIR_CAPTURE_DIR` makes the unit tests write each delivered
    Bundle as `<event_type>.bundle.json`, byte-identical to the wire body.
    Default off, no product code change.
  - 21 more archives under `testdata/fhir/packages/` (99,247,081 bytes) with
    their `SHA256SUMS` lines; `TestFHIRStructural_PinnedPackagesMatchTheirRecordedDigests`
    now hashes every archive on disk against `SHA256SUMS`.
  - Matrix §5 row 2 ratified in full and new §5.2; `SUPPORTED-1.0.md`
    standards row and profile-version policy bullet; decision
    `2026-09-24-run-the-hl7-validator-offline-as-a.md`.
- Why:
  - `.loom/35` Lane S7-B: Option A was the last unbuilt piece of the
    2026-08-08 validation strategy, and the only engine whose output counts as
    official-validator evidence.
- Evidence:
  - **Kill-test (day 1, before any code): two archives are not enough; 23 are.**
    `--network none`, only the two 5.1b archives: `Unable to resolve package id
    hl7.fhir.r4.core#4.0.1`. Core pre-placed in the cache:
    `Installing hl7.fhir.xver-extensions#0.1.0 … Unable to fetch: fhir.org`.
    Iterating with `-no-http-access` surfaced the rest: the validator's own
    defaults (`ValidationService.java` @6.10.4 loads THO/extensions working
    versions with the core, then unversioned "latest" `hl7.terminology` and
    `hl7.fhir.uv.extensions`, which offline resolve to the newest cached
    version) and US Core 9.0.0's transitive closure (`IgLoader.loadIg` loads
    every non-core dependency). `hl7.fhir.r5.core` is declared but never
    loaded. With exactly the 23, the validator's `Package Summary` lists all
    23.
  - **offline == online: identical 23-package summary, identical findings** —
    first on `patient.json`, then over all 36 inputs (online: empty cache,
    network allowed, `-tx n/a`). Two offline runs were identical.
  - All 21 new archives reproduce the registry `dist.shasum`; none is
    licence-gated; `us.nlm.vsac` is not a dependency. Trivy 0.63.0 over the 23:
    vuln gate `num=0` exit 0, secret gate no issues exit 0.
  - Ledger at landing: 36 inputs, 163 findings — 41 error, 80 warning, 42
    information (fixtures 25/54/26, Bundles 16/26/16). The errors add what 5.1b
    could not see: invariants (`us-core-8/9/17/21`, `pd-1`), slicing
    (`LaboratorySlice`), datatype rules (OIDs, `urn:ietf:rfc:3986`, example
    URLs), in-Bundle reference profile matching, and local code-system
    membership (`LAB`, `discharge`, `physician` are not codes in their systems).
  - Negative control: exactly one added row, `mapper/patient.json error …
    Patient.name: minimum required = 1, but only found 0`. The gate was also
    shown to fail on an unrecorded finding and on a recorded-but-vanished row.
  - Local: `make fhir-official`, `make fhir-official-negative-control` (JRE in a
    `--network none` container on `docker --context 7900xtx`),
    `go test -race ./internal/integration/fhirout/...`,
    `go test -race -run '^TestFHIRStructural' ./pkg/fhir`, `gofmt -l`,
    `golangci-lint run`, `shellcheck`, `scripts/ci-job-inventory.sh --check`,
    `scripts/validate-docs.sh`, `worklog.sh check`, `decisions.sh check`.
- What's next:
  - Lane S7-A (5.1c-α) closes the structural cardinality gaps; whoever merges
    second rebases and regenerates the ledger in the same commit.
  - The remaining errors are mapper work, each now visible by row: the lab
    `DiagnosticReport` category code and `effective`/`issued`, NPI check
    digits in fixtures, `MedicationRequest.requester`, identifier value
    formats.
- Sources:
  - [S1] `.loom/35-sprint7-execution-specs.md` — Lane S7-B.
  - [S2] `.loom/decisions/2026-09-24-run-the-hl7-validator-offline-as-a.md`.
  - [S3] `docs/planning/FHIR-CONFORMANCE-MATRIX.md` §5.2.
  - [S4] `testdata/fhir/packages/README.md` — the closure table.
