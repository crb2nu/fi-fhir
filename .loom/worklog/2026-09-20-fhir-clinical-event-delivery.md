### 2026-09-20 - Clinical event delivery and workflow FHIR reference resolution

- What changed: connected six existing clinical mapper families to the shared
  projector and both delivery paths. Added workflow resource selection and
  transaction reference rewriting, resolving issue #20. Unsupported JSON events
  now fail explicitly. Medication substitution preserves `allowedBoolean:false`
  in JSON, reducing the structural fixture ledger from nine violations to seven.
- Why: the existing mapper coverage exceeded what the delivery engines could
  send, and admission transactions used a business MRN as a server resource ID.
- Evidence: repository Go race tests; golangci-lint; six canonical synthetic
  clinical fixtures with structural, identity, immutability, and planner tests;
  live HAPI FHIR v8.2.0-2 CLI read-back for Patient selection, admission and all
  six clinical families, plus a missing-reference negative control. The existing
  `test:e2e-legacy` CI job now supplies digest-pinned HAPI and requires fourteen
  passing top-level tests with no skips. Full CI remains the merge gate.
- Review scope: this exceeds 500 changed lines because the shared projector,
  legacy HTTP action, fixtures, regression tests, live E2E dependency, and
  operational documentation form one delivery contract. No schema migration,
  dependency upgrade, or UI change is included.
- Limitations: official FHIR conformance remains unclaimed. Clinical correction
  identity and literal provider/location references are documented separately
  from retry identity; workflow POST creates remain non-idempotent.
- Sources: `internal/integration/fhirout/`, `internal/workflow/actions.go`,
  `pkg/fhir/types.go`, `test/e2e/integration_test.go`, `ci/test-e2e-legacy.yml`,
  [FHIR R4 transactions](https://hl7.org/fhir/R4/http.html#transaction),
  [HAPI server configuration](https://github.com/hapifhir/hapi-fhir-jpaserver-starter).
