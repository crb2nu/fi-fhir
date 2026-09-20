### 2026-09-20 - Roadmap and public documentation refresh

- Reconciled the roadmap with merged Sprint 6 lanes, broker processors, CDA
  profile selection, and clinical FHIR delivery through MR !211.
- Cleared the obsolete FHIR-destination prerequisite in the completion plan;
  preserved outstanding performance, official-validation, and release gates.
- Updated planning and component status; corrected the terminology integration
  command and separate-database requirement to match CI.
- Reconciled the conformance matrix and supported-baseline fixture counts with
  the seven-violation ledger; removed obsolete runner and MLLP prerequisites.
- Public site integration is shipped from services/flexinfer-site, which imports
  these documents for both flexinfer.ai and codyblevins.com.
- Evidence: merged MRs !202 and !204–!211, final feature pipeline 27875,
  .gitlab-ci.yml, internal/integration/fhirout, and pkg/eventbus.
