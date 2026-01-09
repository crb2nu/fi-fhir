# FHIR Profiles Planning

This document details US Core, Da Vinci, and other FHIR profile requirements for fi-fhir output generation.

## FHIR Overview

### Version Timeline

| Version | Status | Key Features |
|---------|--------|--------------|
| DSTU2 | Legacy | Still in some Epic instances |
| STU3 | Legacy | Transitional |
| R4 | **Current** | US regulatory standard |
| R4B | Current | Minor updates to R4 |
| R5 | New | Not yet widely adopted |

**fi-fhir targets R4** as it's the US regulatory standard (21st Century Cures Act, CMS mandates).

### Resource Categories

| Category | Resources | Use Case |
|----------|-----------|----------|
| **Clinical** | Patient, Observation, Condition, Procedure | Core clinical data |
| **Administrative** | Encounter, Location, Organization | Visits, facilities |
| **Financial** | Claim, ClaimResponse, Coverage | Billing/insurance |
| **Workflow** | Appointment, ServiceRequest | Scheduling, orders |
| **Documents** | DocumentReference, DiagnosticReport | Reports, attachments |

## US Core Profile

### What is US Core?

US Core is the **minimum required FHIR profile for US healthcare** interoperability. Mandated by ONC for certified EHRs.

Current version: **US Core 6.1.0** (based on FHIR R4)

### US Core Resource Requirements

| Resource | Must Support Elements | Key Constraints |
|----------|----------------------|-----------------|
| **Patient** | identifier, name, gender, birthDate | At least one identifier required |
| **Observation** | status, category, code, subject | Lab results, vitals |
| **Condition** | clinicalStatus, code, subject | Active problems |
| **Encounter** | status, class, type, subject | Visit information |
| **DiagnosticReport** | status, category, code, subject | Results summary |
| **DocumentReference** | status, type, subject, content | Clinical documents |
| **Procedure** | status, code, subject | Completed procedures |
| **Medication** | code | Drug information |
| **MedicationRequest** | status, intent, medication, subject | Prescriptions |
| **Immunization** | status, vaccineCode, patient | Vaccine records |
| **AllergyIntolerance** | clinicalStatus, code, patient | Allergies |

### US Core Patient Profile

```json
{
  "resourceType": "Patient",
  "id": "example",
  "meta": {
    "profile": ["http://hl7.org/fhir/us/core/StructureDefinition/us-core-patient"]
  },
  "identifier": [
    {
      "system": "http://hospital.example.org/mrn",
      "value": "123456"
    }
  ],
  "name": [
    {
      "family": "Doe",
      "given": ["John", "William"]
    }
  ],
  "gender": "male",
  "birthDate": "1965-03-15",
  "address": [
    {
      "line": ["123 Main St"],
      "city": "Anytown",
      "state": "VA",
      "postalCode": "24101"
    }
  ],
  "extension": [
    {
      "url": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-race",
      "extension": [
        {
          "url": "ombCategory",
          "valueCoding": {
            "system": "urn:oid:2.16.840.1.113883.6.238",
            "code": "2106-3",
            "display": "White"
          }
        }
      ]
    }
  ]
}
```

### US Core Observation (Lab Result)

```json
{
  "resourceType": "Observation",
  "id": "lab-result-example",
  "meta": {
    "profile": ["http://hl7.org/fhir/us/core/StructureDefinition/us-core-observation-lab"]
  },
  "status": "final",
  "category": [
    {
      "coding": [
        {
          "system": "http://terminology.hl7.org/CodeSystem/observation-category",
          "code": "laboratory"
        }
      ]
    }
  ],
  "code": {
    "coding": [
      {
        "system": "http://loinc.org",
        "code": "6690-2",
        "display": "Leukocytes [#/volume] in Blood"
      }
    ]
  },
  "subject": {
    "reference": "Patient/example"
  },
  "effectiveDateTime": "2024-01-15T14:25:00Z",
  "valueQuantity": {
    "value": 12.5,
    "unit": "10*3/uL",
    "system": "http://unitsofmeasure.org",
    "code": "10*3/uL"
  },
  "interpretation": [
    {
      "coding": [
        {
          "system": "http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation",
          "code": "H",
          "display": "High"
        }
      ]
    }
  ],
  "referenceRange": [
    {
      "low": {"value": 4.5, "unit": "10*3/uL"},
      "high": {"value": 11.0, "unit": "10*3/uL"}
    }
  ]
}
```

## Da Vinci Implementation Guides

### Da Vinci Overview

Da Vinci is a set of FHIR implementation guides for **payer-provider data exchange**:

| IG | Purpose | Status |
|----|---------|--------|
| **PDex** (Payer Data Exchange) | Share claims data with patients | STU 2.0 |
| **CDex** (Clinical Data Exchange) | Request clinical data from providers | STU 2.0 |
| **PAS** (Prior Authorization Support) | Automate prior auth | STU 2.0 |
| **DTR** (Documentation Templates & Rules) | Smart forms for PA | STU 2.0 |
| **CRD** (Coverage Requirements Discovery) | Check coverage at order time | STU 2.0 |
| **ATR** (Member Attribution) | Attribute patients to providers | STU 1.0 |
| **PCDE** (Payer Coverage Decision Exchange) | Coverage decisions | Draft |

### CMS Mandates Using Da Vinci

**January 2026 Deadline** (CMS-0057-F):
- Medicare Advantage plans must implement:
  - Patient Access API (PDex)
  - Prior Authorization API (PAS)
  - Provider Directory API
- Electronic prior auth decisions within 72 hours (urgent: 24 hours)

### Da Vinci PAS (Prior Authorization)

Key resources:
- `Claim` with PAS profile for auth requests
- `ClaimResponse` for decisions
- `Task` for async workflow

```json
{
  "resourceType": "Claim",
  "id": "prior-auth-request",
  "meta": {
    "profile": ["http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-claim"]
  },
  "status": "active",
  "type": {
    "coding": [
      {
        "system": "http://terminology.hl7.org/CodeSystem/claim-type",
        "code": "professional"
      }
    ]
  },
  "use": "preauthorization",
  "patient": {
    "reference": "Patient/example"
  },
  "provider": {
    "reference": "Organization/requesting-provider"
  },
  "insurer": {
    "reference": "Organization/payer"
  },
  "item": [
    {
      "sequence": 1,
      "productOrService": {
        "coding": [
          {
            "system": "http://www.ama-assn.org/go/cpt",
            "code": "27447",
            "display": "Total knee replacement"
          }
        ]
      },
      "servicedDate": "2024-02-15"
    }
  ]
}
```

## Terminology Bindings

### Required Code Systems

| Element | Code System | Binding Strength |
|---------|-------------|------------------|
| Patient.gender | AdministrativeGender | Required |
| Observation.status | ObservationStatus | Required |
| Observation.category | ObservationCategory | Preferred |
| Observation.code | LOINC | Extensible |
| Condition.code | SNOMED CT / ICD-10 | Extensible |
| Procedure.code | SNOMED CT / CPT | Extensible |
| Medication.code | RxNorm | Extensible |

### Binding Strength Meanings

| Strength | Meaning |
|----------|---------|
| **Required** | Must use code from value set |
| **Extensible** | Must use if code exists, can extend |
| **Preferred** | Should use, but alternatives allowed |
| **Example** | Just suggestions |

## fi-fhir FHIR Generation Strategy

### Canonical Event → FHIR Mapping

```go
// Map semantic event to FHIR resources
type FHIRMapper interface {
    MapPatient(p *events.Patient) *fhir.Patient
    MapEncounter(e *events.Encounter, patientRef string) *fhir.Encounter
    MapObservation(lab *events.LabResultEvent) *fhir.Observation
    MapClaim(claim *events.ClaimEvent) *fhir.Claim
}

// US Core compliant mapper
type USCoreMapper struct {
    profile string // "http://hl7.org/fhir/us/core/StructureDefinition/..."
}

func (m *USCoreMapper) MapPatient(p *events.Patient) *fhir.Patient {
    patient := &fhir.Patient{
        Meta: &fhir.Meta{
            Profile: []string{m.profile + "us-core-patient"},
        },
        Identifier: m.mapIdentifiers(p.Identifiers),
        Name: []fhir.HumanName{
            {
                Family: p.FamilyName,
                Given:  []string{p.GivenName},
            },
        },
        Gender: m.mapGender(p.Gender),
        BirthDate: m.formatDate(p.DateOfBirth),
    }

    // Add US Core extensions
    if p.Race != "" {
        patient.Extension = append(patient.Extension,
            m.buildRaceExtension(p.Race))
    }

    return patient
}
```

### Profile Validation

```go
// Validate resource against profile
type ProfileValidator struct {
    profiles map[string]*StructureDefinition
}

func (v *ProfileValidator) Validate(resource interface{}, profileURL string) []ValidationError {
    profile := v.profiles[profileURL]
    if profile == nil {
        return []ValidationError{{Message: "Unknown profile: " + profileURL}}
    }

    var errors []ValidationError

    // Check required elements
    for _, element := range profile.MustSupport {
        if !hasElement(resource, element.Path) {
            errors = append(errors, ValidationError{
                Path:    element.Path,
                Message: "Missing must-support element",
            })
        }
    }

    // Check terminology bindings
    for _, binding := range profile.Bindings {
        if !validCode(resource, binding) {
            errors = append(errors, ValidationError{
                Path:    binding.Path,
                Message: "Invalid code for binding",
            })
        }
    }

    return errors
}
```

## Bundle Generation

### Transaction Bundle (for writes)

```go
func CreateTransactionBundle(resources []interface{}) *fhir.Bundle {
    bundle := &fhir.Bundle{
        Type: "transaction",
        Entry: make([]fhir.BundleEntry, len(resources)),
    }

    for i, resource := range resources {
        bundle.Entry[i] = fhir.BundleEntry{
            Resource: resource,
            Request: &fhir.BundleEntryRequest{
                Method: "POST",
                URL:    getResourceType(resource),
            },
        }
    }

    return bundle
}
```

### Searchset Bundle (for reads)

```go
func CreateSearchBundle(resources []interface{}, total int) *fhir.Bundle {
    bundle := &fhir.Bundle{
        Type:  "searchset",
        Total: total,
        Entry: make([]fhir.BundleEntry, len(resources)),
    }

    for i, resource := range resources {
        bundle.Entry[i] = fhir.BundleEntry{
            Resource: resource,
            Search: &fhir.BundleEntrySearch{
                Mode: "match",
            },
        }
    }

    return bundle
}
```

## Profile Registry

### Supported Profiles

```yaml
# fi-fhir profile support configuration
profiles:
  us_core:
    version: "6.1.0"
    base_url: "http://hl7.org/fhir/us/core/StructureDefinition"
    resources:
      - Patient: us-core-patient
      - Observation: us-core-observation-lab
      - Condition: us-core-condition-problems-health-concerns
      - Encounter: us-core-encounter
      - DiagnosticReport: us-core-diagnosticreport-lab

  davinci_pas:
    version: "2.0.0"
    base_url: "http://hl7.org/fhir/us/davinci-pas/StructureDefinition"
    resources:
      - Claim: profile-claim
      - ClaimResponse: profile-claimresponse

  davinci_pdex:
    version: "2.0.0"
    base_url: "http://hl7.org/fhir/us/davinci-pdex/StructureDefinition"
    resources:
      - ExplanationOfBenefit: pdex-explanationofbenefit
```

## Implementation Plan

### Phase 1: Core FHIR Types
- [ ] FHIR R4 base types in Go
- [ ] JSON serialization
- [ ] Reference handling

### Phase 2: US Core Mapping
- [ ] Patient (with race/ethnicity extensions)
- [ ] Observation (lab results)
- [ ] Encounter
- [ ] Condition

### Phase 3: Validation
- [ ] Must-support element checking
- [ ] Terminology binding validation
- [ ] Profile metadata injection

### Phase 4: Da Vinci Support
- [ ] PAS Claim/ClaimResponse
- [ ] PDex ExplanationOfBenefit
- [ ] Coverage resources

### Phase 5: Bundle Operations
- [ ] Transaction bundles
- [ ] Batch bundles
- [ ] Document bundles (if needed)

## Testing Strategy

### Validation Testing

```go
var usCorePatientsTestCases = []struct {
    name    string
    input   *events.Patient
    valid   bool
    errors  []string
}{
    {
        name: "valid minimal patient",
        input: &events.Patient{
            MRN: "123",
            FamilyName: "Doe",
            GivenName: "John",
            Gender: "M",
            DateOfBirth: time.Date(1965, 3, 15, 0, 0, 0, 0, time.UTC),
        },
        valid: true,
    },
    {
        name: "missing gender",
        input: &events.Patient{
            MRN: "123",
            FamilyName: "Doe",
            GivenName: "John",
        },
        valid: false,
        errors: []string{"Patient.gender is required"},
    },
}
```

### Profile Conformance

Use FHIR Validator:
```bash
java -jar validator_cli.jar patient.json \
  -ig hl7.fhir.us.core#6.1.0 \
  -profile http://hl7.org/fhir/us/core/StructureDefinition/us-core-patient
```

## References

- [US Core Implementation Guide](https://www.hl7.org/fhir/us/core/)
- [Da Vinci Project](https://www.hl7.org/fhir/us/davinci-pas/)
- [FHIR R4 Specification](https://www.hl7.org/fhir/R4/)
- [ONC Cures Act Final Rule](https://www.healthit.gov/topic/oncs-cures-act-final-rule)
- [CMS Interoperability Rules](https://www.cms.gov/regulations-and-guidance/guidance/interoperability/index)
