# CDA/CCDA Parser Design

This document describes the architecture for parsing CDA (Clinical Document Architecture) and CCDA (Consolidated CDA) documents.

## Overview

CDA and CCDA are XML-based standards for clinical documents:

```
┌────────────────────────────────────────────────────────────────┐
│                     CDA Document Structure                      │
├────────────────────────────────────────────────────────────────┤
│  ClinicalDocument                                               │
│  ├── Header                                                     │
│  │   ├── typeId, templateId (document type identification)     │
│  │   ├── id (document identifier)                               │
│  │   ├── code (document type code)                              │
│  │   ├── effectiveTime (document date)                          │
│  │   ├── recordTarget (patient information)                     │
│  │   ├── author (document author)                               │
│  │   ├── custodian (organization maintaining document)          │
│  │   └── documentationOf (encounter information)                │
│  └── Body                                                       │
│      └── structuredBody                                         │
│          └── component[] (sections)                             │
│              ├── section (Problems)                             │
│              ├── section (Medications)                          │
│              ├── section (Allergies)                            │
│              ├── section (Results)                              │
│              ├── section (Procedures)                           │
│              └── section (Vital Signs)                          │
└────────────────────────────────────────────────────────────────┘
```

## CCDA Document Types

| Document Type | Template OID | Description | Semantic Events |
|--------------|--------------|-------------|-----------------|
| CCD | 2.16.840.1.113883.10.20.22.1.2 | Continuity of Care Document | `patient_summary` |
| Discharge Summary | 2.16.840.1.113883.10.20.22.1.8 | Hospital Discharge | `patient_discharge` |
| Progress Note | 2.16.840.1.113883.10.20.22.1.9 | Clinical Progress | `clinical_note` |
| Consultation Note | 2.16.840.1.113883.10.20.22.1.4 | Specialist Consultation | `consultation` |
| History and Physical | 2.16.840.1.113883.10.20.22.1.3 | H&P Note | `admission_note` |
| Operative Note | 2.16.840.1.113883.10.20.22.1.7 | Surgical Procedure | `procedure_note` |
| Referral Note | 2.16.840.1.113883.10.20.22.1.14 | Patient Referral | `referral` |
| Transfer Summary | 2.16.840.1.113883.10.20.22.1.13 | Patient Transfer | `patient_transfer` |

## CCDA Sections

| Section | Template OID | Maps To |
|---------|--------------|---------|
| Problems | 2.16.840.1.113883.10.20.22.2.5.1 | `Patient.Conditions` |
| Medications | 2.16.840.1.113883.10.20.22.2.1.1 | `Patient.Medications` |
| Allergies | 2.16.840.1.113883.10.20.22.2.6.1 | `Patient.Allergies` |
| Results | 2.16.840.1.113883.10.20.22.2.3.1 | `LabResultEvent` |
| Vital Signs | 2.16.840.1.113883.10.20.22.2.4.1 | `VitalSignsEvent` |
| Procedures | 2.16.840.1.113883.10.20.22.2.7.1 | `ProcedureEvent` |
| Encounters | 2.16.840.1.113883.10.20.22.2.22.1 | `Encounter` |
| Immunizations | 2.16.840.1.113883.10.20.22.2.2.1 | `ImmunizationEvent` |
| Plan of Care | 2.16.840.1.113883.10.20.22.2.10 | `CarePlanEvent` |
| Goals | 2.16.840.1.113883.10.20.22.2.60 | `Patient.Goals` |
| Social History | 2.16.840.1.113883.10.20.22.2.17 | `Patient.SocialHistory` |
| Functional Status | 2.16.840.1.113883.10.20.22.2.14 | `Patient.FunctionalStatus` |

## Architecture

### Package Structure

```
internal/parser/cda/
├── parser.go          # Main CDA parser
├── header.go          # Header extraction (patient, author, custodian)
├── sections.go        # Section parser registry
├── sections/
│   ├── problems.go    # Problems section
│   ├── medications.go # Medications section
│   ├── allergies.go   # Allergies section
│   ├── results.go     # Results section
│   ├── vitals.go      # Vital signs section
│   ├── procedures.go  # Procedures section
│   ├── encounters.go  # Encounters section
│   └── immunizations.go # Immunizations section
├── types.go           # CDA/CCDA type definitions
├── namespaces.go      # XML namespace handling
└── mapper.go          # CDA to canonical event mapper
```

### Core Types

```go
// CDADocument represents a parsed CDA document
type CDADocument struct {
    // Header information
    ID             string
    TemplateIDs    []string
    TypeCode       CodedValue
    Title          string
    EffectiveTime  time.Time
    ConfidentialityCode string
    LanguageCode   string

    // Participants
    Patient        *PatientRole
    Author         *Author
    Custodian      *Custodian
    Authenticator  *Authenticator

    // Encounter context
    ServiceEvent   *ServiceEvent
    EncompassingEncounter *EncompassingEncounter

    // Body sections
    Sections       []Section

    // Raw XML for audit
    RawXML         []byte
}

// Section represents a CDA section
type Section struct {
    TemplateID     string
    Code           CodedValue
    Title          string
    Text           string        // Narrative text
    Entries        []Entry       // Structured entries
}

// Entry represents a structured entry in a section
type Entry struct {
    TypeCode       string        // e.g., "observation", "act", "procedure"
    TemplateIDs    []string
    ID             string
    Code           CodedValue
    StatusCode     string
    EffectiveTime  *TimeInterval
    Value          *EntryValue
    Participants   []Participant
    EntryRelationships []Entry   // Nested entries
}

// CodedValue represents a coded concept (from code systems)
type CodedValue struct {
    Code           string
    CodeSystem     string        // OID
    CodeSystemName string
    DisplayName    string
    OriginalText   string
}

// TimeInterval represents effectiveTime with low/high bounds
type TimeInterval struct {
    Low            *time.Time
    High           *time.Time
    Value          *time.Time    // Point in time
}

// PatientRole contains patient information from recordTarget
type PatientRole struct {
    IDs            []Identifier  // Patient identifiers
    Addresses      []Address
    Telecoms       []Telecom
    Patient        *PatientInfo
}

// PatientInfo contains patient demographics
type PatientInfo struct {
    Names          []PersonName
    Gender         string
    BirthTime      time.Time
    MaritalStatus  *CodedValue
    RaceCode       *CodedValue
    EthnicityCode  *CodedValue
    LanguageCode   string
}
```

### Parser Implementation

```go
// Parser parses CDA/CCDA documents
type Parser struct {
    source         string
    sectionParsers map[string]SectionParser
    config         ParserConfig
}

// ParserConfig configures parser behavior
type ParserConfig struct {
    // StrictMode fails on any parsing error
    StrictMode     bool

    // ExtractNarrative includes human-readable text from sections
    ExtractNarrative bool

    // ValidateTemplates checks template OIDs against known CCDA templates
    ValidateTemplates bool

    // MaxSectionDepth limits nested entry parsing
    MaxSectionDepth int
}

// SectionParser handles parsing of specific section types
type SectionParser interface {
    TemplateOID() string
    Parse(section *xmlquery.Node) (*Section, error)
    MapToCanonical(section *Section, patient *events.Patient) ([]interface{}, error)
}

// Parse parses a CDA/CCDA document
func (p *Parser) Parse(xmlData []byte) (*CDADocument, error)

// ParseWithResult returns document with warnings
func (p *Parser) ParseWithResult(xmlData []byte) (*ParseResult, error)

// ParseResult contains parsed document and any warnings
type ParseResult struct {
    Document *CDADocument
    Events   []interface{}        // Canonical events extracted
    Warnings []events.ParseWarning
}
```

### Namespace Handling

CDA documents use multiple XML namespaces:

```go
const (
    // HL7 CDA namespace
    NSHL7V3 = "urn:hl7-org:v3"

    // SDTC extensions (HL7 structured document technical corrections)
    NSSDTC = "urn:hl7-org:sdtc"

    // XML Schema Instance
    NSXsi = "http://www.w3.org/2001/XMLSchema-instance"
)

// Common XPath prefixes for querying
var Namespaces = map[string]string{
    "cda": NSHL7V3,
    "sdtc": NSSDTC,
    "xsi": NSXsi,
}
```

## Event Mapping

### Document-Level Events

| Document Type | Event Type | Trigger |
|--------------|------------|---------|
| CCD | `patient_summary` | Any CCD document |
| Discharge Summary | `patient_discharge` | Discharge document |
| Transfer Summary | `patient_transfer` | Transfer document |
| Referral Note | `referral` | Referral document |

### Section-Level Events

| Section | Event Type | When |
|---------|------------|------|
| Results | `lab_result` | Per result entry |
| Vital Signs | `vital_sign` | Per vital sign entry |
| Procedures | `procedure` | Per procedure entry |
| Immunizations | `immunization` | Per immunization entry |
| Encounters | `encounter` | Per encounter entry |

### Example Mapping: Problems Section

```xml
<component>
  <section>
    <templateId root="2.16.840.1.113883.10.20.22.2.5.1"/>
    <code code="11450-4" codeSystem="2.16.840.1.113883.6.1"/>
    <title>PROBLEMS</title>
    <entry typeCode="DRIV">
      <act classCode="ACT" moodCode="EVN">
        <templateId root="2.16.840.1.113883.10.20.22.4.3"/>
        <entryRelationship typeCode="SUBJ">
          <observation classCode="OBS" moodCode="EVN">
            <templateId root="2.16.840.1.113883.10.20.22.4.4"/>
            <code code="64572001" codeSystem="2.16.840.1.113883.6.96"
                  displayName="Condition"/>
            <statusCode code="completed"/>
            <effectiveTime>
              <low value="20120806"/>
            </effectiveTime>
            <value xsi:type="CD" code="195967001"
                   codeSystem="2.16.840.1.113883.6.96"
                   displayName="Asthma"/>
          </observation>
        </entryRelationship>
      </act>
    </entry>
  </section>
</component>
```

Maps to:

```json
{
  "type": "condition",
  "code": {
    "coding": [{
      "system": "http://snomed.info/sct",
      "code": "195967001",
      "display": "Asthma"
    }]
  },
  "clinical_status": "active",
  "onset_date": "2012-08-06"
}
```

## CLI Integration

```bash
# Parse CDA document
fi-fhir parse --format cda document.xml

# Parse with pretty output
fi-fhir parse --format cda --pretty patient_ccd.xml

# Extract specific sections
fi-fhir parse --format cda --sections problems,medications document.xml

# Validate CCDA conformance
fi-fhir validate --format cda --strict document.xml
```

## Configuration

### Source Profile Integration

```yaml
source_profile:
  id: "hospital_ccda"
  name: "Hospital CCDA Interface"
  format: cda

  cda:
    # Expected document types
    document_types:
      - "2.16.840.1.113883.10.20.22.1.2"  # CCD
      - "2.16.840.1.113883.10.20.22.1.8"  # Discharge Summary

    # Sections to extract
    sections:
      - template_id: "2.16.840.1.113883.10.20.22.2.5.1"
        emit_events: true
      - template_id: "2.16.840.1.113883.10.20.22.2.3.1"
        emit_events: true

    # Code system mappings
    code_systems:
      "2.16.840.1.113883.6.96": "http://snomed.info/sct"
      "2.16.840.1.113883.6.1": "http://loinc.org"
      "2.16.840.1.113883.6.88": "http://www.nlm.nih.gov/research/umls/rxnorm"

    # Identifier type mappings
    identifier_systems:
      "2.16.840.1.113883.4.1": "SSN"
      "2.16.840.1.113883.4.6": "NPI"
```

## Implementation Plan

### Phase 1: Core Parser
- [x] Design document
- [ ] XML namespace handling
- [ ] Header parsing (patient, author, custodian)
- [ ] Basic section extraction

### Phase 2: Section Parsers
- [ ] Problems section
- [ ] Medications section
- [ ] Allergies section
- [ ] Results section

### Phase 3: Event Mapping
- [ ] Patient data to canonical Patient
- [ ] Section data to canonical events
- [ ] Document-level events

### Phase 4: CLI & Integration
- [ ] CLI `--format cda` support
- [ ] Source Profile integration
- [ ] Validation tooling

## Test Data

Sample CCDA documents for testing:
- HL7 C-CDA R2.1 samples: https://github.com/HL7/C-CDA-Examples
- SMART Health IT samples: https://github.com/smart-on-fhir/sample-patients

## See Also

- [SOURCE-PROFILES.md](SOURCE-PROFILES.md) - Profile configuration
- [WORKFLOW-DSL.md](WORKFLOW-DSL.md) - Workflow integration
- [HL7 CDA R2](http://www.hl7.org/implement/standards/product_brief.cfm?product_id=7) - CDA specification
- [HL7 C-CDA](http://www.hl7.org/implement/standards/product_brief.cfm?product_id=492) - CCDA implementation guide
