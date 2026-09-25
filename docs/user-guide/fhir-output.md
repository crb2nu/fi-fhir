# FHIR Output

fi-fhir maps canonical events to FHIR R4 resources with US Core profile
annotations. Validation is optional on workflow actions; the pinned structural
checks do not establish full FHIR conformance. See the
[conformance matrix](../planning/FHIR-CONFORMANCE-MATRIX.md) for measured coverage.

## Events delivered by FHIR actions

Both the legacy workflow action and the durable FHIR destination use the shared
[projector](../../internal/integration/fhirout/project.go):

| Event type | Resources delivered |
|---|---|
| `patient_admit`, `patient_transfer`, `patient_update`, `patient_discharge` | Patient and Encounter |
| `lab_result` | DiagnosticReport and Observations |
| `vital_sign` | Observation |
| `condition` | Condition |
| `procedure` | Procedure |
| `immunization` | Immunization |
| `medication_request` | MedicationRequest |
| `allergy_intolerance` | AllergyIntolerance |

Other `pkg/fhir` mapper methods remain available to library callers, but their
event types are not yet connected to these delivery paths. Unsupported events
return an error; JSON events no longer silently become Patient-only output.

The six clinical event types and lab results reference an existing Patient.
They do not overwrite demographics from a partial clinical document. Patient
and Encounter references use qualified identifiers; create those resources
first, with the same source or assigning-authority system. Other references
(such as Practitioner and Location) retain the mapper's literal IDs and require
matching resources on the destination.

## Selecting workflow resources

Set `resource: Patient` to create only a Patient from an admission, or omit
`resource` to deliver every projected resource. A selection that does not match
any output fails before sending. A DiagnosticReport-only selection also fails
if it would omit its referenced Observations.

```yaml
workflow:
  name: patient-demographics
  version: "1.0"
  routes:
    - name: patient
      filter:
        event_type: patient_admit
      actions:
        - type: fhir
          endpoint: https://fhir.example.org/r4
          resource: Patient
          validate_fhir: true
```

A selected Patient uses a direct POST unless `bundle: true` is set. Other
outputs use a transaction so the server can resolve references. Bundle entries
have `fullUrl` values, internal references point to those values, and external
Patient/Encounter references use conditional identifier searches. These workflow
creates use POST and can duplicate resources on retry. For persisted,
retry-safe delivery, use the [durable FHIR destination](../operations/DESTINATION-IDENTITY.md#the-fhir-transport-41c-c).

Durable clinical writes use the canonical event ID under
`urn:fi-fhir:event:<source>:<event_type>`. Re-delivering a stored event updates
the same resource; a new event ID creates a separate record, including a
correction submitted as a new event. Source message IDs are not used as clinical
record keys because one document can contain many records. Missing source,
event ID, or patient identifier prevents durable delivery.

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
| `encounter.identifier` | `identifier[0].value` | Visit number. The durable `fhir` transport qualifies a value sent with no system under `urn:fi-fhir:source:<source>` |
| `encounter.class` | `class` | inpatient, outpatient, emergency |
| `encounter.class` | `type` | Derived from the patient class (PV1-2); see below |
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

### Encounter Type

`Encounter.type` is required by US Core. The only encounter-kind fact the event
carries is the patient class, so the type is derived from it:

| Patient class (PV1-2) | `type.coding` |
|---|---|
| `I` inpatient | SNOMED CT `86181006` Evaluation and management of inpatient, and HL7 v2 table 0004 `I` |
| `E` emergency | SNOMED CT `4525004` Emergency department patient visit, and HL7 v2 table 0004 `E` |
| `O`, `P`, `R`, `B`, `C`, `N`, `U` | HL7 v2 table 0004 code only |
| any other value | none; the value is sent as `type.text` |
| absent | DataAbsentReason `unknown` |

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

Action configuration is a flat map of string values. Nested YAML blocks under
an action are ignored — always use the flat keys shown below. Values are also
literal: `${VAR}` references are **not** expanded by `fi-fhir`. Render the file
before loading it if you need environment-specific secrets:

```bash
envsubst < workflow.yaml.tmpl > workflow.yaml
```

### OAuth2 Client Credentials

```yaml
actions:
  - type: fhir
    endpoint: https://fhir.example.com/r4
    token_url: https://auth.example.com/oauth2/token
    client_id: my-client-id
    client_secret: my-client-secret
    scopes: system/Patient.read,system/Patient.write,system/Encounter.write
```

`token_url`, `client_id`, and `client_secret` must all be present; if any is
missing, OAuth2 is skipped and the action falls back to the static `token`
below. `scopes` is a single string — separate multiple scopes with commas or
spaces.

### Static Bearer Token

```yaml
actions:
  - type: fhir
    endpoint: https://fhir.example.com/r4
    token: my-static-bearer-token        # Sends "Authorization: Bearer <token>"
    # authorization: "Basic ..."         # Or set the Authorization header verbatim
```

### Token Caching

OAuth2 tokens are cached and automatically refreshed before expiration.

### 401 Handling

If a 401 is received:
1. Token is refreshed
2. Request is retried
3. If still failing, error is returned

## Batch Operations

When an event produces more than one resource, they are sent as a single FHIR
transaction bundle automatically. Set `bundle` to force a transaction bundle
even for a single resource:

```yaml
actions:
  - type: fhir
    endpoint: https://fhir.example.com/r4
    token: my-static-bearer-token
    bundle: "true"                   # Send as a transaction bundle
    timeout: 30s                     # Request timeout (default: 30s)
```

Bundle size is not configurable — a bundle carries exactly the resources
produced by the event being processed.

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
