# Implementation Plan: Integration Platform Evolution

## Scope

Execute the integration platform product spec across 4 core areas: Ingestion/Extraction, Terminology Governance, FHIR Interoperability, and Profile Management.

## Milestones

1. **M1**: Universal Ingestion & CCDA Expansion
2. **M2**: Terminology Approval Workflow
3. **M3**: FHIR Interoperability (USCDI v3, Bulk, SMART)
4. **M4**: Dynamic Profile Management & Tracing

---

## 1. M1: Universal Ingestion & CCDA Expansion

### Code Changes

- **`internal/parser/cda/`**: Extend `mapper.go` and `parser.go` with specialized XPath extractors for Medications, Allergies, and Social History sections.
- **`cmd/fi-fhir/etl.go` & `storage/s3.go` (NEW)**: Add pull-based watchers for S3 and SFTP. Alternatively, provide Mentatlab YAML configurations to push batches.
- **`pkg/events/`**: Guarantee canonical event representations for the newly extracted CDA concepts.

### Verification Plan

- **Automated Tests**: Write Go unit tests (`cda_medications_test.go`, etc.) feeding sample CCDA XML files with known Medications/Allergies and asserting canonical event output.
- **Integration**: Start the ETL pipeline `go run ./cmd/fi-fhir serve` and simulate S3 bucket events to verify batch processing endpoints.

---

## 2. M2: Terminology Approval Workflow

### Code Changes

- **`pkg/terminology/`**: Add a `Status` field to mappings (`PENDING`, `APPROVED`, `REJECTED`).
- **`internal/api/graphql/`**: Add mutations `approveMapping(id)` and `rejectMapping(id)`, and a query `pendingMappings()`.
- **`ui/` (Frontend)**: Build the HITL Terminology Dashboard hitting the new GraphQL endpoints.

### Verification Plan

- **Automated Tests**: Add integration tests to `cmd/fi-fhir/terminology_approval_test.go` that submit an unknown code, mock the LLM authoroute, query pending status, and approve the mapping.
- **Manual Verification**: Run the local Frontend and execute the approval steps via the Svelte application UI.

---

## 3. M3: FHIR Interoperability

### Code Changes

- **`internal/fhir/export/` (NEW)**: Implement `$export` operation handlers using NDJSON format streaming.
- **`internal/fhir/smart/` (NEW)**: Implement OAuth2 flows, integrating with OIDC providers for SMART App Launch contexts.
- **`internal/fhir/adapter/`**: Adjust resource mappers to comply with USCDI v3 constraints (e.g. strict systems and terminology bindings).

### Verification Plan

- **Automated Tests**: Leverage existing FHIR testing frameworks. Create a test executing `GET [Base]/Patient/$export` and validating the NDJSON output format.
- **Manual Verification**: Run a SMART App Launch sandbox (like SMART Launcher) against a locally running `fi-fhir` instance to ensure the authorization handshake passes.

---

## 4. M4: Dynamic Profile Management & Tracing

### Code Changes

- **`internal/api/graphql/`**: Expose CRUD for `SourceProfile` resources, backing them via DB instead of purely `configs/`.
- **`pkg/eventsourcing/`**: Introduce tracing metadata (`TraceID`, `SpanID`) attached to every step of Byte Normalization, Syntactic Parse, and Semantic Extraction.
- **`ui/` (Frontend)**: Build an interface to graphically view a message's lifecycle using the tracing headers.

### Verification Plan

- **Automated Tests**: Inject an HL7v2 message and fetch its EventStore audit trail, validating that all transition states and applied profiles are recorded.
- **Manual Verification**: Validate the UI rendering of a processed event pipeline from raw byte string to structured canonical JSON.
