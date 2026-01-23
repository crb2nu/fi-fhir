# Source Profiles

Source Profiles are the heart of fi-fhir's configuration system. Each profile defines how a specific data feed should be parsed, validated, and transformed.

## Why Source Profiles?

Traditional integration approaches use a one-size-fits-all parser. When edge cases appear, developers add if-statements and flags until the code becomes unmaintainable.

fi-fhir inverts this pattern: **the profile is the unit of scalability**.

Each data feed gets its own profile, allowing:
- Feed-specific tolerance rules
- Custom identifier mappings
- Different event classification logic
- Independent validation requirements

## Profile Structure

A complete Source Profile has five main sections:

```yaml
# Metadata
name: epic_adt
version: "1.0"
description: "Epic ADT interface for main hospital"

# Phase 1: Byte handling
encoding:
  charset: UTF-8
  lineEnding: auto
  bomHandling: strip

# Phase 2: Message structure
syntax:
  hl7Version: "2.5"
  fieldSeparator: "|"
  encodingChars: "^~\\&"
  escapeSequences:
    enabled: true
  strictMode: false

# Phase 3: Business logic
semantics:
  messageTypes: [ADT]
  eventTypes: [A01, A02, A03, A08]
  patientIdentifiers:
    - source_field: PID.3.1
      identifier_type: MRN
      assigning_authority: EPIC

# FHIR output
fhirMapping:
  targetVersion: R4
  bundleType: transaction
  resourceMappings: []

# Validation rules
validation:
  enabled: true
  requiredSegments: [MSH, PID, PV1]
  requiredFields: [MSH.9, PID.3]
```

## Encoding Section

Controls Phase 1 (Byte Normalization).

```yaml
encoding:
  charset: UTF-8           # UTF-8, ISO-8859-1, Windows-1252, US-ASCII
  lineEnding: auto         # LF, CRLF, CR, auto
  bomHandling: strip       # strip, preserve, error
```

### Options

| Field | Values | Description |
|-------|--------|-------------|
| `charset` | `UTF-8`, `ISO-8859-1`, `Windows-1252`, `US-ASCII` | Character encoding |
| `lineEnding` | `LF`, `CRLF`, `CR`, `auto` | Line ending style |
| `bomHandling` | `strip`, `preserve`, `error` | BOM marker handling |

### Common Scenarios

**Legacy system with Windows encoding:**
```yaml
encoding:
  charset: Windows-1252
  lineEnding: CRLF
```

**Modern UTF-8 system:**
```yaml
encoding:
  charset: UTF-8
  lineEnding: auto
  bomHandling: strip
```

## Syntax Section

Controls Phase 2 (Syntactic Parsing).

```yaml
syntax:
  hl7Version: "2.5"
  fieldSeparator: "|"
  encodingChars: "^~\\&"
  escapeSequences:
    enabled: true
    customMappings:
      "\\N\\": ""              # Null escape
      "\\.br\\": "\n"          # Line break
  strictMode: false
```

### Options

| Field | Description |
|-------|-------------|
| `hl7Version` | Expected HL7 version (2.3, 2.3.1, 2.4, 2.5, 2.5.1, 2.6, 2.7, 2.8) |
| `fieldSeparator` | Field delimiter (always `\|` in practice) |
| `encodingChars` | Component, repetition, escape, subcomponent chars |
| `strictMode` | If true, fail on any parse errors |

### Escape Sequences

Standard HL7 escapes:
- `\F\` → `|` (field separator)
- `\S\` → `^` (component separator)
- `\T\` → `&` (subcomponent separator)
- `\R\` → `~` (repetition separator)
- `\E\` → `\` (escape character)
- `\H\` → highlight start
- `\N\` → normal text (highlight end)

Custom mappings override or extend these.

## Semantics Section

Controls Phase 3 (Semantic Extraction).

```yaml
semantics:
  messageTypes: [ADT, ORU]
  eventTypes: [A01, A02, A03, A04, A08, R01]

  patientIdentifiers:
    - source_field: PID.3.1
      identifier_type: MRN
      assigning_authority: EPIC
      validation: required
      format_hint: "\\d{6,8}"

    - source_field: PID.3.1
      identifier_type: SSN
      assigning_authority: SSA
      validation: optional

  encounterIdentifiers:
    - source_field: PV1.19
      identifier_type: VN
      assigning_authority: HOSPITAL

  customExtractors:
    - name: insurance_group
      source_field: IN1.8
      target: insurance.group_number
```

### Identifier Configuration

```yaml
patientIdentifiers:
  - source_field: PID.3.1     # HL7 field path
    identifier_type: MRN       # Type code
    assigning_authority: EPIC  # Authority name
    validation: required       # required, optional, warn
    format_hint: "\\d{6,8}"    # Regex for validation
```

### Event Classification

Map HL7 trigger events to semantic events with patient class awareness:

```yaml
eventClassification:
  adt_a01:
    default: patient_admit
    patient_class_values:
      I: inpatient_admit
      O: outpatient_admit
      E: emergency_admit
  adt_a03:
    default: patient_discharge
```

## FHIR Mapping Section

Controls FHIR R4 output generation.

```yaml
fhirMapping:
  targetVersion: R4           # R4 or R5
  bundleType: transaction     # batch, transaction, collection

  resourceMappings:
    - event_type: patient_admit
      resources: [Patient, Encounter]
    - event_type: lab_result
      resources: [Patient, Observation, DiagnosticReport]
```

### Bundle Types

| Type | Description |
|------|-------------|
| `batch` | Independent operations, partial success allowed |
| `transaction` | All-or-nothing, rollback on failure |
| `collection` | Read-only collection, no server processing |

## Validation Section

Controls message and field validation.

```yaml
validation:
  enabled: true

  requiredSegments: [MSH, PID, PV1]
  requiredFields: [MSH.9, PID.3, PV1.2]

  customValidators:
    - name: mrn_format
      field: PID.3.1
      pattern: "^MRN\\d{6}$"
      message: "MRN must start with 'MRN' followed by 6 digits"
```

### Validation Levels

- `enabled: true` + `requiredSegments` → Fail if segments missing
- `enabled: true` + `requiredFields` → Fail if fields empty
- `enabled: false` → Warnings only, never fail

## Tolerance Configuration

Configure what parsing issues to tolerate:

```yaml
hl7v2:
  tolerate:
    missing_segments: [NK1, NTE, OBX]
    nte_anywhere: true
    extra_components: true
    unknown_segments: true
```

### Options

| Option | Description |
|--------|-------------|
| `missing_segments` | List of segments that can be absent |
| `nte_anywhere` | Allow NTE segments after any segment |
| `extra_components` | Ignore extra components beyond expected |
| `unknown_segments` | Pass through unknown segments as raw |

## Z-Segment Configuration

Handle custom (vendor-specific) Z-segments:

```yaml
hl7v2:
  z_segments:
    ZPD:
      description: "Patient demographics extension"
      fields:
        - index: 1
          name: custom_mrn
          target: patient.identifiers.custom_mrn
        - index: 2
          name: payer_code
          target: insurance.payer_code
```

## Terminology Mapping

Configure code system mappings:

```yaml
terminology:
  race_mapping: local_to_omb           # Map local race codes
  language_mapping: local_to_bcp47     # Map language codes

  custom_mappings:
    - source_system: LOCAL
      target_system: LOINC
      mapping_file: local_to_loinc.csv
```

## Example Profiles

### Minimal Profile

```yaml
name: minimal
version: "1.0"

encoding:
  charset: UTF-8

syntax:
  hl7Version: "2.5"
```

### Production ADT Profile

```yaml
name: epic_adt_prod
version: "2.1"
description: "Epic ADT interface - Production"

encoding:
  charset: UTF-8
  lineEnding: auto
  bomHandling: strip

syntax:
  hl7Version: "2.5.1"
  fieldSeparator: "|"
  encodingChars: "^~\\&"
  escapeSequences:
    enabled: true
  strictMode: false

hl7v2:
  tolerate:
    missing_segments: [NK1, NTE, AL1, DG1]
    extra_components: true
    unknown_segments: true
  z_segments:
    ZPD:
      fields:
        - index: 1
          name: epic_csn
          target: encounter.identifiers.epic_csn

semantics:
  messageTypes: [ADT]
  eventTypes: [A01, A02, A03, A04, A08, A11, A13, A40]
  patientIdentifiers:
    - source_field: PID.3.1
      identifier_type: MRN
      assigning_authority: EPIC
      validation: required

validation:
  enabled: true
  requiredSegments: [MSH, PID, PV1]
  requiredFields: [MSH.9, MSH.10, PID.3]

fhirMapping:
  targetVersion: R4
  bundleType: transaction
```

### Lab Interface Profile

```yaml
name: lab_interface
version: "1.0"
description: "Lab results from reference lab"

encoding:
  charset: ISO-8859-1
  lineEnding: CRLF

syntax:
  hl7Version: "2.3"

semantics:
  messageTypes: [ORU]
  eventTypes: [R01]
  patientIdentifiers:
    - source_field: PID.3.1
      identifier_type: MRN

validation:
  enabled: true
  requiredSegments: [MSH, PID, OBR, OBX]
```

## CLI Commands

### Validate a Profile

```bash
fi-fhir validate profile my_profile.yaml
```

### Infer Profile from Samples

```bash
fi-fhir profile infer samples/*.hl7 --output inferred_profile.yaml
```

### Lint Profile for Best Practices

```bash
fi-fhir profile lint my_profile.yaml
```

### Parse with Profile

```bash
fi-fhir parse --format hl7v2 --profile my_profile.yaml message.hl7
```

## See Also

- [Playground Tutorial](playground-tutorial.md) - Interactive profile editing
- [Core Concepts](core-concepts.md) - Pipeline architecture
- [Planning: SOURCE-PROFILES.md](../planning/SOURCE-PROFILES.md) - Full specification
