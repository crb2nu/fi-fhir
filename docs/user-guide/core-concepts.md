# Core Concepts

This document explains the fundamental concepts behind fi-fhir's design and architecture.

## Philosophy: Profile-Driven Normalization

The key insight behind fi-fhir is that **the unit of scalability is the Source Profile, not "HL7v2 support"**.

In traditional healthcare integration:
- You build a monolithic "HL7v2 parser"
- Every feed requires code changes for edge cases
- Tolerance rules are scattered across the codebase

In fi-fhir:
- Each interface/feed gets its own **Source Profile**
- Parsing behavior is driven by configuration
- Adding a new feed means creating a new profile, not writing code

## Parsing Pipeline

fi-fhir processes messages through a three-phase pipeline:

```
┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────────┐
│ Phase 1             │    │ Phase 2             │    │ Phase 3             │
│ Byte Normalization  │───>│ Syntactic Parsing   │───>│ Semantic Extraction │
└─────────────────────┘    └─────────────────────┘    └─────────────────────┘
        │                          │                          │
        v                          v                          v
  Raw bytes               Parsed segments           Canonical events
  (UTF-8, line           (fields, components,      (patient_admit,
   endings, BOM)          escape sequences)         lab_result, etc.)
```

### Phase 1: Byte Normalization

**Input**: Raw bytes from source system
**Output**: Normalized UTF-8 string

Operations:
- BOM (Byte Order Mark) detection and handling
- Character encoding conversion (ISO-8859-1 → UTF-8)
- Line ending normalization (CRLF/CR → LF)
- Trailing whitespace handling

**Configuration** (in Source Profile):
```yaml
encoding:
  charset: UTF-8
  lineEnding: auto
  bomHandling: strip
```

### Phase 2: Syntactic Parsing

**Input**: Normalized string
**Output**: Parsed message structure

Operations:
- Field separator extraction from MSH.1
- Encoding characters from MSH.2
- Segment splitting
- Field/component/subcomponent splitting
- Escape sequence handling (`\H\`, `\N\`, `\.br\`)

**Configuration**:
```yaml
syntax:
  hl7Version: "2.5"
  fieldSeparator: "|"
  encodingChars: "^~\\&"
  strictMode: false
```

### Phase 3: Semantic Extraction

**Input**: Parsed message structure
**Output**: Canonical semantic events

Operations:
- Message type classification (ADT^A01 → `patient_admit`)
- Identifier extraction (MRN, SSN, NPI)
- Field mapping to canonical model
- Terminology normalization

**Configuration**:
```yaml
semantics:
  messageTypes: [ADT, ORU]
  patientIdentifiers:
    - source_field: PID.3.1
      assigning_authority: EPIC
      identifier_type: MRN
```

## Canonical Event Model

All input formats map to a common set of **semantic events**. This decouples:
- Input parsing from business logic
- Workflow routing from format specifics
- FHIR generation from source systems

### Event Types

| Category | Event Types |
|----------|-------------|
| **Patient** | `patient_admit`, `patient_discharge`, `patient_transfer`, `patient_update`, `patient_merge` |
| **Scheduling** | `appointment_scheduled`, `appointment_cancelled`, `appointment_rescheduled`, `appointment_noshow`, `appointment_checked_in` |
| **Lab/Clinical** | `lab_result`, `lab_ordered`, `lab_cancelled`, `vital_sign`, `condition`, `procedure`, `immunization` |
| **Claims** | `claim_submitted`, `claim_adjudicated`, `prior_auth_request`, `prior_auth_response` |
| **Documents** | `document`, `document_addendum`, `document_replacement`, `document_status_change` |
| **Financial** | `financial_transaction` |

### Event Structure

Every event includes:

```json
{
  "meta": {
    "id": "evt_unique_id",
    "type": "patient_admit",
    "source": "epic_adt",
    "format": "HL7v2",
    "timestamp": "2024-01-15T12:00:00Z",
    "source_message_id": "MSG12345",
    "parse_warnings": []
  },
  // Event-specific fields...
}
```

## Source Profiles

A Source Profile defines parsing behavior for a specific data feed.

### Why Profiles?

Consider two hospital systems sending ADT messages:

**Hospital A (Epic)**:
- Uses `^` as component separator
- MRN in PID.3.1 with assigning authority "EPIC"
- Missing NK1 (next of kin) segments are normal

**Hospital B (Cerner)**:
- Uses `^` as component separator (same)
- MRN in PID.3.1 with assigning authority "CERN"
- NK1 segments are always present

Same message type, different behaviors. Profiles let you configure each independently.

### Profile Structure

```yaml
id: epic_adt
name: Epic ADT Interface
version: "1.0"

# Phase 1 config
encoding:
  charset: UTF-8
  lineEnding: auto

# Phase 2 config
hl7v2:
  default_version: "2.5"
  tolerate:
    missing_segments: [NK1, NTE]
    extra_components: true

# Phase 3 config
identifiers:
  mrn:
    assigning_authority: EPIC
    validation: required

# Event classification
event_classification:
  adt_a01:
    patient_class_values:
      I: inpatient
      O: outpatient
      E: emergency
```

## Workflow Engine

The workflow engine routes events through configurable pipelines.

### Components

```
Event ─── Filter ─── Transform ─── Actions
         (match?)    (modify)      (destinations)
```

**Filters** determine which events match a route:
- Event type matching
- Source system matching
- CEL expressions for complex conditions

**Transforms** modify events before routing:
- Set/update fields
- Map terminology codes
- Redact sensitive data

**Actions** send events to destinations:
- FHIR servers
- Webhooks
- Databases
- Message queues
- Logging

### Example Workflow

```yaml
workflow:
  name: adt_routing
  routes:
    - name: critical_admits
      filter:
        event_type: patient_admit
        condition: event.encounter.class == "inpatient"
      transform:
        - set_field: processed_at = now()
      actions:
        - type: fhir
          endpoint: https://fhir.hospital.com/r4
        - type: log
          message: "Inpatient admit: {{.Patient.MRN}}"
```

## Warnings Over Errors

Healthcare data is inherently messy. fi-fhir uses a **warnings over errors** philosophy:

- Recoverable issues generate warnings, not failures
- Tolerance rules determine what's acceptable
- Warnings are recorded in event metadata for auditing

```go
// Instead of failing on missing data:
if segment == nil {
    if profile.IsMissingSegmentTolerated(segmentID) {
        addWarning("MISSING_SEGMENT", segmentID)
        return defaultValue  // Continue processing
    }
    return error  // Only fail if profile says so
}
```

## Identifier-First Design

Patient identifiers are a first-class concept:

- `IdentifierSet` handles multiple identifiers (PID-3 repetitions)
- Validators for NPI, MBI, SSN, DEA numbers
- Assigning authority mapping
- Original value preservation for audit

```json
{
  "identifiers": {
    "mrn": {
      "value": "12345",
      "assigning_authority": "EPIC",
      "type": "MR"
    },
    "ssn": {
      "value": "XXX-XX-6789",
      "original": "123-45-6789",
      "redacted": true
    }
  }
}
```

## FHIR Mapping

Canonical events map to FHIR R4 resources following US Core profiles:

| Event Type | FHIR Resource(s) |
|------------|------------------|
| `patient_admit` | Patient, Encounter |
| `lab_result` | Observation, DiagnosticReport |
| `claim_submitted` | Claim (Da Vinci PAS) |
| `vital_sign` | Observation (US Core Vital Signs) |
| `document` | DocumentReference |

See [FHIR Output](fhir-output.md) for complete mapping details.

## Next Steps

- [Source Profiles](source-profiles.md) - Configure parsing for your feeds
- [Workflows](workflows.md) - Route and transform events
- [FHIR Output](fhir-output.md) - Generate compliant FHIR resources
