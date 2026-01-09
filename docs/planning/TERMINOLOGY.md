# Terminology Systems Planning

This document details healthcare code systems, version management, and the fi-fhir terminology normalization strategy.

## Quick Reference

| Feature | Status | Implementation |
|---------|--------|----------------|
| **Code system URIs** | ✅ | `pkg/terminology/mapper.go` (LOINC, SNOMED, ICD-10, CPT) |
| **CSV mapping loader** | ✅ | `pkg/terminology/mapper.go:LoadFromCSV()` |
| **Mapper registry** | ✅ | `pkg/terminology/mapper.go:Registry` |
| **MapToLOINC/SNOMED** | ✅ | Convenience methods for common targets |
| **LOINC file loader** | 🔲 | Direct loading from loinc.org downloads |
| **Panel expansion** | 🔲 | CBC/BMP/CMP → individual components |

## Code Systems Reference

### Clinical Coding Systems

| System | OID | URI | Domain | License |
|--------|-----|-----|--------|---------|
| **LOINC** | 2.16.840.1.113883.6.1 | `http://loinc.org` | Labs, vitals, documents | Free (registration) |
| **SNOMED CT** | 2.16.840.1.113883.6.96 | `http://snomed.info/sct` | Clinical findings | NLM license (US free) |
| **ICD-10-CM** | 2.16.840.1.113883.6.90 | `http://hl7.org/fhir/sid/icd-10-cm` | US diagnoses | Public domain |
| **ICD-10-PCS** | 2.16.840.1.113883.6.4 | `http://www.cms.gov/Medicare/Coding/ICD10` | US inpatient procedures | Public domain |
| **CPT** | 2.16.840.1.113883.6.12 | `http://www.ama-assn.org/go/cpt` | Procedures (billing) | AMA license ($) |
| **HCPCS** | 2.16.840.1.113883.6.285 | `https://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets` | Medicare items | Public domain |
| **RxNorm** | 2.16.840.1.113883.6.88 | `http://www.nlm.nih.gov/research/umls/rxnorm` | Medications | Free (UMLS) |
| **NDC** | 2.16.840.1.113883.6.69 | `http://hl7.org/fhir/sid/ndc` | Drug products | Public domain |
| **CVX** | 2.16.840.1.113883.12.292 | `http://hl7.org/fhir/sid/cvx` | Vaccines | Public domain |

### Administrative Coding Systems

| System | Domain | Example |
|--------|--------|---------|
| **HL7 Tables** | Message components | Sex (M/F/U), Marital status |
| **NUBC** | UB-04 form codes | Revenue codes, condition codes |
| **X12 Code Lists** | EDI transactions | Claim frequency, filing indicator |
| **Place of Service** | Facility types | 11=Office, 21=Inpatient, 23=ER |

## Version Management Strategy

### Annual Update Cycles

| System | Update Frequency | Effective Date | Key Changes |
|--------|------------------|----------------|-------------|
| **ICD-10-CM** | Annual | Oct 1 (Fiscal Year) | Codes added/deleted/revised |
| **ICD-10-PCS** | Annual | Oct 1 | Procedure codes updated |
| **CPT** | Annual | Jan 1 | ~300 changes per year |
| **HCPCS** | Quarterly | Jan/Apr/Jul/Oct | Level II codes |
| **LOINC** | ~2x/year | Variable | ~5,000 new codes/release |
| **SNOMED CT** | 2x/year | Jan/Jul (US Ed) | Thousands of changes |
| **RxNorm** | Monthly | 1st Monday | Drug updates |

### Version Handling in fi-fhir

```go
// Terminology version configuration
type TerminologyConfig struct {
    // Version per code system
    Versions map[string]string `yaml:"versions"`

    // Whether to validate codes against version
    StrictValidation bool `yaml:"strict_validation"`

    // Fallback behavior for unknown codes
    UnknownCodeBehavior string `yaml:"unknown_code_behavior"` // "error", "warn", "pass"
}

// Example config
/*
terminology:
  versions:
    icd10cm: "2024"      # FY2024 (Oct 2023 - Sep 2024)
    cpt: "2024"          # Calendar year
    loinc: "2.76"        # LOINC version
    snomed: "2023-09-01" # US Edition date
  strict_validation: false
  unknown_code_behavior: "warn"
*/
```

## Code Normalization Pipeline

### Input → Canonical → Output Flow

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Source Code    │────▶│  Canonical Form │────▶│  Target Code    │
│  (may vary)     │     │  (normalized)   │     │  (as needed)    │
└─────────────────┘     └─────────────────┘     └─────────────────┘

Examples:
HL7v2 OBX: "WBC^White Blood Count^LOCAL" → LOINC 6690-2 → FHIR Observation.code
CSV: "glucose" → LOINC 2345-7 → FHIR Observation.code
EDI 837: "99213" → CPT 99213 → FHIR Claim.item.productOrService
```

### Mapping Table Structure

```go
// Code mapping entry
type CodeMapping struct {
    SourceSystem  string `json:"source_system"`
    SourceCode    string `json:"source_code"`
    SourceDisplay string `json:"source_display,omitempty"`

    TargetSystem  string `json:"target_system"`
    TargetCode    string `json:"target_code"`
    TargetDisplay string `json:"target_display"`

    Equivalence   string `json:"equivalence"` // equivalent, wider, narrower, inexact
    Notes         string `json:"notes,omitempty"`
}
```

### Mapping File Format (CSV)

```csv
source_system,source_code,source_display,target_system,target_code,target_display,equivalence
LOCAL_LAB,GLU,Glucose,http://loinc.org,2345-7,Glucose [Mass/volume] in Serum or Plasma,equivalent
LOCAL_LAB,HGB,Hemoglobin,http://loinc.org,718-7,Hemoglobin [Mass/volume] in Blood,equivalent
LOCAL_LAB,WBC,White Count,http://loinc.org,6690-2,Leukocytes [#/volume] in Blood,equivalent
```

## LOINC Specifics

### LOINC Code Structure

```
Component|Property|Time|System|Scale|Method
   │          │      │     │      │     │
   └──────────┴──────┴─────┴──────┴─────┴── 6 axes define the observation
```

### Common LOINC Categories

| Panel/Group | LOINC Code | Tests Included |
|-------------|------------|----------------|
| **CBC** | 58410-2 | WBC, RBC, Hgb, Hct, Plt, MCV, MCH, MCHC |
| **BMP** | 51990-0 | Glucose, BUN, Creatinine, Na, K, Cl, CO2, Ca |
| **CMP** | 24323-8 | BMP + Albumin, Bilirubin, ALT, AST, ALP |
| **Lipid Panel** | 57698-3 | Total Chol, HDL, LDL, Triglycerides |
| **Urinalysis** | 24356-8 | Color, clarity, pH, specific gravity, etc. |

### LOINC Version Compatibility

```yaml
# LOINC version handling
loinc:
  preferred_version: "2.76"

  # Codes that changed meaning between versions
  deprecated_codes:
    - code: "2093-3"  # Old cholesterol code
      deprecated_in: "2.60"
      replacement: "2093-3"  # Same code, different definition

  # Cross-version mappings
  version_mappings:
    - from_version: "2.65"
      to_version: "2.76"
      file: ./mappings/loinc_2.65_to_2.76.csv
```

## SNOMED CT Specifics

### Hierarchy Navigation

SNOMED uses an "is-a" hierarchy. A child concept implies the parent:
- `44054006` (Diabetes mellitus type 2) IS-A `73211009` (Diabetes mellitus)
- Queries for "diabetes" should find both

### Expression Constraint Language (ECL)

```
# All types of diabetes
< 73211009 |Diabetes mellitus|

# Direct children only
<! 73211009 |Diabetes mellitus|

# Diabetes OR hypertension
73211009 |Diabetes mellitus| OR 38341003 |Hypertensive disorder|
```

### US Edition vs International

- US Edition adds US-specific content (ICD-10-CM mappings)
- Released January and July
- Backwards compatible with International Edition

## ICD-10 Specifics

### CM vs PCS

| Aspect | ICD-10-CM | ICD-10-PCS |
|--------|-----------|------------|
| Use | Diagnoses | Inpatient procedures |
| Length | 3-7 characters | 7 characters (always) |
| Structure | Category + etiology + details | Section + Body System + Root Operation + ... |
| Example | E11.9 (Type 2 DM) | 0SG00ZZ (Knee fusion) |

### Code Validation Rules

```go
// ICD-10-CM validation
func ValidateICD10CM(code string) error {
    // Length: 3-7 characters
    if len(code) < 3 || len(code) > 7 {
        return errors.New("ICD-10-CM must be 3-7 characters")
    }

    // Format: Letter + 2 digits + optional decimal + 0-4 chars
    pattern := `^[A-Z]\d{2}(\.\d{1,4})?$`
    if !regexp.MustCompile(pattern).MatchString(code) {
        return errors.New("invalid ICD-10-CM format")
    }

    // Note: Actual validation requires lookup against code set
    return nil
}
```

### Fiscal Year Handling

```go
// Determine which ICD-10-CM version to use
func GetICD10Version(serviceDate time.Time) string {
    // ICD-10-CM updates October 1
    fiscalYear := serviceDate.Year()
    if serviceDate.Month() >= time.October {
        fiscalYear++
    }
    return fmt.Sprintf("FY%d", fiscalYear)
}
```

## Implementation Plan

### Phase 1: Core Infrastructure ✅
- [x] Code system URIs (LOINC, SNOMED, ICD-10, CPT, etc.) - see `pkg/terminology/mapper.go`
- [x] MappingEquivalence enum (equivalent, wider, narrower, inexact, unmatched)
- [x] CodeMapping struct with full provenance fields
- [ ] Version tracking per code system (in config only)

### Phase 2: LOINC Support
- [ ] LOINC file loader (CSV from loinc.org)
- [ ] Code lookup by code/display
- [ ] Panel expansion (CBC → individual components)

### Phase 3: Mapping Engine ✅
- [x] CSV mapping file parser - see `pkg/terminology/mapper.go:LoadFromCSV()`
- [x] Mapper with lookup (Map, MapToLOINC, MapToSNOMED) - see `pkg/terminology/mapper.go`
- [x] Mapper Registry for managing multiple source→target pairs
- [x] HasMapping() check for validation
- [ ] Confidence scoring for fuzzy matches

### Phase 4: UMLS Integration (Optional)
- [ ] UMLS API client
- [ ] Cross-walk queries (ICD-10 ↔ SNOMED)
- [ ] Concept normalization

## Testing Strategy

### Unit Tests
- Valid/invalid code format detection
- Version comparison logic
- Mapping lookup accuracy

### Integration Tests
- Real LOINC file parsing
- End-to-end code normalization
- FHIR CodeableConcept generation

### Test Data
```go
var testCases = []struct {
    input    string
    system   string
    expected string
}{
    {"GLU", "LOCAL", "2345-7"},      // Local → LOINC
    {"E11.9", "ICD10CM", "E11.9"},   // Passthrough
    {"INVALID", "LOCAL", ""},        // Unknown code
}
```

## External Dependencies

| Dependency | Purpose | Source |
|------------|---------|--------|
| LOINC Table | Code definitions | https://loinc.org/downloads/ |
| SNOMED CT | US Edition files | https://www.nlm.nih.gov/healthit/snomedct/ |
| ICD-10-CM | Code list | https://www.cms.gov/medicare/coding |
| UMLS | Cross-walks | https://uts.nlm.nih.gov/uts/ |

## See Also

- [SOURCE-PROFILES.md](SOURCE-PROFILES.md) - Terminology mapping configuration per source profile
- [FHIR-PROFILES.md](FHIR-PROFILES.md) - CodeableConcept and Coding in FHIR resources
- [HL7V2-QUIRKS.md](HL7V2-QUIRKS.md) - OBX segment coding and local code systems

## References

- [LOINC User's Guide](https://loinc.org/kb/users-guide/)
- [SNOMED CT Editorial Guide](https://confluence.ihtsdotools.org/display/DOCEG)
- [ICD-10-CM Guidelines](https://www.cdc.gov/nchs/icd/icd10cm.htm)
- [FHIR Terminology Services](https://www.hl7.org/fhir/terminology-service.html)
