# Product Spec: fi-fhir Integration Platform Evolution

## Summary

Evolve `fi-fhir` from a robust healthcare format parser into a comprehensive, multi-tenant Integration Platform. This phase shifts focus from internal sibling-service integrations (inference, orchestrator) to user-facing integration capabilities, terminology governance, and strict healthcare interoperability standards.

## Goals

1. **Universal Ingestion & Extraction Edge**: Expand the ingestion boundary to support pull-based batch workflows (S3/SFTP) and deepen clinical data extraction (CDA/CCDA Medications, Allergies, Social History).
2. **Terminology Governance (HITL)**: Introduce a Human-In-The-Loop review workflow for LLM-powered "Autoroute" terminology suggestions to ensure clinical safety and SME trust.
3. **Advanced Interoperability standards**: Achieve compliance with modern FHIR standards including USCDI v3, FHIR Bulk Data Export/Import, and SMART App Launch.
4. **Multi-tenant Profile Management**: Expose configuration and observability APIs to manage source-specific profiles dynamically, moving beyond static repository configurations.

## Users / Stakeholders

- **Implementation Engineers**: Configuring new data feeds, mapping legacy formats to profiles.
- **Clinical SMEs**: Reviewing and approving mapping decisions produced by the autoroute engine.
- **External Integration Partners**: Utilizing standardized FHIR APIs, SMART apps, and bulk data mechanisms.

## Requirements

### 1. Ingestion & Extraction (CDA/CCDA & Batch)

- Implement S3/SFTP consumers with temporal-cron or mentatlab orchestration for periodic polling.
- Expand CDA/CCDA extractors to accurately map Medications, Allergies, and Social History into the canonical semantic event model (`pkg/events`).
- Ensure large batch files are streamed or chunked to prevent OOM errors during Phase 1 Byte Normalization.

### 2. Terminology Approval Workflow

- Design a GraphQL API supporting `ReviewRequired` terminology mappings.
- Integrate with an internal dashboard for SMEs to accept, reject, or modify LLM-suggested SNOMED/LOINC/RxNorm codes.
- Persist confirmed mappings to the high-speed mapping index and emit audit events.

### 3. FHIR IG Support

- **USCDI v3**: Ensure canonical events map faithfully to USCDI v3 profiles in the outbound FHIR adapter.
- **Bulk Data**: Implement `$export` (system/group/patient level) generating NDJSON files and `$import` for bulk ingestion.
- **SMART App Launch**: Support OAuth2 handshake handling and context sharing for SMART on FHIR applications.

### 4. Platform Observability & Management

- CRUD API for `SourceProfile` resources, enabling dynamic onboarding of new feeds.
- Message tracing: Provide cross-system correlation of a message from the Raw Payload to the Semantic Event, including all applied terminology transformations and triggered webhooks.

## End-to-End Flows

1. **Pull Ingestion Flow**:
   Cron triggers S3 download → File chunked and pushed to fi-fhir pipeline → Extracted into canonical events → Outbound webhooks/FHIR Subscriptions fired.
2. **Terminology Governance Flow**:
   Format parsed → Unrecognized local code detected → Flexinfer suggests mapping → Placed in Review Queue → SME approves via UI → Routing resumes.
3. **FHIR Bulk Flow**:
   Partner requests `$export` → Mentatlab orchestrated job compiled NDJSON → Returns URL → Partner downloads.

## Rollout

- Phase 1: Extraction expansions and Batch Ingestion.
- Phase 2: Terminology UI and GraphQL extensions.
- Phase 3: FHIR Standard features (USCDI v3, Bulk).
- Phase 4: Dynamic Source Profile APIs and Observability.

## Open Questions

- Should pull-based ingestion (S3/SFTP) be a native Go component inside `fi-fhir`, or completely delegated to `mentatlab` which pushes via webhooks?
- What are the required SLAs for Bulk Data Generation tasks and the related storage backend (S3 presigned URLs)?
