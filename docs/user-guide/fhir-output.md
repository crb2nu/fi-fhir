# FHIR Output

fi-fhir generates FHIR R4 resources that conform to US Core profiles. This guide covers the mapping from canonical events to FHIR resources.

## Overview

When events flow through a workflow with a FHIR action, fi-fhir:
1. Maps canonical event fields to FHIR resource properties
2. Applies US Core profile requirements
3. Validates the generated resources
4. Sends to the configured FHIR server

## Supported Resources

fi-fhir maps to 24+ FHIR R4 resources:

| Event Type | FHIR Resources | Profile |
|------------|----------------|---------|
| `patient_admit` | Patient, Encounter | US Core |
| `patient_discharge` | Encounter | US Core |
| `patient_update` | Patient | US Core |
| `lab_result` | Observation, DiagnosticReport | US Core Laboratory |
| `vital_sign` | Observation | US Core Vital Signs |
| `condition` | Condition | US Core |
| `procedure` | Procedure | US Core |
| `immunization` | Immunization | US Core |
| `medication_request` | MedicationRequest | US Core |
| `allergy` | AllergyIntolerance | US Core |
| `claim_submitted` | Claim | Da Vinci PAS |
| `claim_adjudicated` | ExplanationOfBenefit | PDex |
| `eligibility_response` | CoverageEligibilityResponse | - |
| `document` | DocumentReference | US Core |

## Patient Resource

Maps from patient events (`patient_admit`, `patient_update`).

### Field Mapping

| Event Field | FHIR Path | Notes |
|-------------|-----------|-------|
| `patient.mrn` | `identifier[0].value` | MRN identifier |
| `patient.ssn` | `identifier[1].value` | SSN (if not redacted) |
| `patient.name.family` | `name[0].family` | Family name |
| `patient.name.given` | `name[0].given` | Given names array |
| `patient.birthDate` | `birthDate` | FHIR date format |
| `patient.gender` | `gender` | male, female, other, unknown |
| `patient.address` | `address[0]` | Full address structure |
| `patient.phone` | `telecom[0]` | Phone contact point |
| `patient.email` | `telecom[1]` | Email contact point |
| `patient.race` | `extension` | US Core Race extension |
| `patient.ethnicity` | `extension` | US Core Ethnicity extension |

### Example Output

```json
{
  "resourceType": "Patient",
  "id": "patient-mrn12345",
  "meta": {
    "profile": ["http://hl7.org/fhir/us/core/StructureDefinition/us-core-patient"]
  },
  "identifier": [
    {
      "type": {
        "coding": [{
          "system": "http://terminology.hl7.org/CodeSystem/v2-0203",
          "code": "MR"
        }]
      },
      "system": "urn:oid:1.2.3.4.5.6",
      "value": "MRN12345"
    }
  ],
  "name": [{
    "use": "official",
    "family": "Smith",
    "given": ["John", "A"]
  }],
  "gender": "male",
  "birthDate": "1980-01-15",
  "address": [{
    "use": "home",
    "line": ["123 Main St"],
    "city": "Springfield",
    "state": "IL",
    "postalCode": "62701"
  }]
}
```

## Encounter Resource

Maps from admit/discharge/transfer events.

### Field Mapping

| Event Field | FHIR Path | Notes |
|-------------|-----------|-------|
| `encounter.identifier` | `identifier[0].value` | Visit number |
| `encounter.class` | `class` | inpatient, outpatient, emergency |
| `encounter.type` | `type` | Encounter type coding |
| `encounter.status` | `status` | in-progress, finished, etc. |
| `encounter.period.start` | `period.start` | Admit datetime |
| `encounter.period.end` | `period.end` | Discharge datetime |
| `encounter.location` | `location[0].location` | Location reference |
| `encounter.provider` | `participant` | Attending physician |

### Encounter Class Mapping

| Event Class | FHIR Class Code |
|-------------|-----------------|
| `inpatient` | `IMP` |
| `outpatient` | `AMB` |
| `emergency` | `EMER` |
| `observation` | `OBSENC` |

## Observation Resource (Laboratory)

Maps from `lab_result` events.

### Field Mapping

| Event Field | FHIR Path | Notes |
|-------------|-----------|-------|
| `observation.code` | `code` | LOINC coding |
| `observation.value` | `value[x]` | Quantity, string, or CodeableConcept |
| `observation.unit` | `valueQuantity.unit` | UCUM unit |
| `observation.referenceRange` | `referenceRange` | Normal range |
| `observation.interpretation` | `interpretation` | H, L, A, N, etc. |
| `observation.status` | `status` | final, preliminary, etc. |
| `observation.effectiveDateTime` | `effectiveDateTime` | Specimen collection time |
| `observation.issued` | `issued` | Result available time |

### Example Output

```json
{
  "resourceType": "Observation",
  "meta": {
    "profile": ["http://hl7.org/fhir/us/core/StructureDefinition/us-core-observation-lab"]
  },
  "status": "final",
  "category": [{
    "coding": [{
      "system": "http://terminology.hl7.org/CodeSystem/observation-category",
      "code": "laboratory"
    }]
  }],
  "code": {
    "coding": [{
      "system": "http://loinc.org",
      "code": "2345-7",
      "display": "Glucose [Mass/volume] in Serum or Plasma"
    }]
  },
  "subject": {
    "reference": "Patient/patient-mrn12345"
  },
  "effectiveDateTime": "2024-01-15T10:30:00Z",
  "valueQuantity": {
    "value": 95,
    "unit": "mg/dL",
    "system": "http://unitsofmeasure.org",
    "code": "mg/dL"
  },
  "interpretation": [{
    "coding": [{
      "system": "http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation",
      "code": "N",
      "display": "Normal"
    }]
  }]
}
```

## Observation Resource (Vital Signs)

Maps from `vital_sign` events to specific US Core Vital Signs profiles.

### Supported Vital Signs

| Vital Sign | LOINC Code | US Core Profile |
|------------|------------|-----------------|
| Blood Pressure | 85354-9 | us-core-blood-pressure |
| BMI | 39156-5 | us-core-bmi |
| Body Height | 8302-2 | us-core-body-height |
| Body Weight | 29463-7 | us-core-body-weight |
| Body Temperature | 8310-5 | us-core-body-temperature |
| Heart Rate | 8867-4 | us-core-heart-rate |
| Respiratory Rate | 9279-1 | us-core-respiratory-rate |
| Pulse Oximetry | 2708-6 | us-core-pulse-oximetry |

## DiagnosticReport Resource

Groups related observations for lab panels.

### Field Mapping

| Event Field | FHIR Path | Notes |
|-------------|-----------|-------|
| `diagnosticReport.code` | `code` | Panel/test code |
| `diagnosticReport.status` | `status` | final, preliminary |
| `diagnosticReport.category` | `category` | LAB category |
| `diagnosticReport.effectiveDateTime` | `effectiveDateTime` | Collection time |
| `diagnosticReport.results` | `result` | References to Observations |

## Claim Resource

Maps from `claim_submitted` events (EDI 837P/837I).

### Field Mapping

| Event Field | FHIR Path | Notes |
|-------------|-----------|-------|
| `claim.identifier` | `identifier` | Claim control number |
| `claim.status` | `status` | active, draft |
| `claim.type` | `type` | institutional, professional |
| `claim.patient` | `patient` | Patient reference |
| `claim.provider` | `provider` | Billing provider |
| `claim.diagnosis` | `diagnosis` | ICD-10 codes |
| `claim.procedure` | `procedure` | CPT/HCPCS codes |
| `claim.total` | `total` | Claim total amount |

## FHIR Bundles

Multiple resources are grouped into bundles:

### Transaction Bundle

```json
{
  "resourceType": "Bundle",
  "type": "transaction",
  "entry": [
    {
      "fullUrl": "urn:uuid:patient-1",
      "resource": { /* Patient */ },
      "request": {
        "method": "PUT",
        "url": "Patient/mrn12345"
      }
    },
    {
      "fullUrl": "urn:uuid:encounter-1",
      "resource": { /* Encounter */ },
      "request": {
        "method": "POST",
        "url": "Encounter"
      }
    }
  ]
}
```

### Batch Bundle

Same structure, but `type: "batch"` for independent operations.

## Validation

### Enable Validation

```yaml
actions:
  - type: fhir
    endpoint: https://fhir.example.com
    validate_fhir: true              # Validate before sending
```

### Validation Levels

- **Structural**: JSON schema validation
- **Profile**: US Core profile conformance
- **Business Rules**: Required fields, code systems

### CLI Validation

```bash
# Validate a FHIR resource
fi-fhir fhir validate patient.json

# Validate a bundle
fi-fhir fhir validate bundle.json --profile us-core
```

## Authentication

### OAuth2 Client Credentials

```yaml
actions:
  - type: fhir
    endpoint: https://fhir.example.com/r4
    auth:
      type: oauth2
      tokenUrl: https://auth.example.com/token
      clientId: ${FHIR_CLIENT_ID}
      clientSecret: ${FHIR_CLIENT_SECRET}
      scopes:
        - system/Patient.read
        - system/Patient.write
        - system/Encounter.write
```

### Token Caching

OAuth2 tokens are cached and automatically refreshed before expiration.

### 401 Handling

If a 401 is received:
1. Token is refreshed
2. Request is retried
3. If still failing, error is returned

## Batch Operations

For high-volume scenarios:

```yaml
actions:
  - type: fhir
    endpoint: https://fhir.example.com/r4
    batch:
      enabled: true
      size: 100                      # Bundle size
      timeout: 30s
```

## US Core Profile Compliance

All generated resources include:

1. **Profile declaration** in `meta.profile`
2. **Must-Support elements** populated when available
3. **Required extensions** (race, ethnicity for Patient)
4. **Standard terminologies** (LOINC, SNOMED, ICD-10)

### Checking Compliance

```bash
# Generate FHIR from event
fi-fhir parse sample.hl7 | fi-fhir fhir generate --profile us-core

# Validate against US Core
fi-fhir fhir validate output.json --profile us-core
```

## Custom Mappings

Override default mappings in Source Profile:

```yaml
fhirMapping:
  targetVersion: R4
  bundleType: transaction

  resourceMappings:
    - event_type: patient_admit
      resources: [Patient, Encounter]
      customFields:
        - source: patient.custom_field
          target: Patient.extension
          extension_url: http://example.com/fhir/extension
```

## See Also

- [Workflow Configuration](workflows.md) - FHIR action setup
- [Source Profiles](source-profiles.md) - FHIR mapping configuration
- [Planning: FHIR-PROFILES.md](../planning/FHIR-PROFILES.md) - Complete mapping specification
- [Playground Tutorial](playground-tutorial.md) - Interactive mapping visualizer
