# Source Profiles Specification

This document defines the **Source Profile** system - the unit of scalability for fi-fhir integration adapters.

## Quick Reference

| Concept | Description |
|---------|-------------|
| **Source Profile** | YAML config for a single data feed (e.g., `epic_adt_hosp_a.yaml`) |
| **Why per-feed?** | Each interface has unique quirks, tolerances, and mappings |
| **3-Phase Pipeline** | Byte normalization → Syntactic parse → Semantic extraction |
| **Key config sections** | `hl7v2`, `z_segments`, `identifiers`, `terminology`, `quality` |
| **Implementation** | `pkg/profile/profile.go` - Registry, loader, and config types |

## Core Technical Thesis

> **The unit of scalability is a Source Profile (per interface / per feed), not "HL7v2 support" in general.**

A Source Profile owns:
- HL7 version expectations and tolerated drift
- Delimiter + encoding handling
- Z-segment extraction + mapping rules
- Identifier normalization and prioritization rules
- Terminology mapping rules (LOCAL → standard system)
- Event disambiguation heuristics (e.g., A01 used for OP reg)

This matches real-world variability: delimiter weirdness, missing segments, optionality drift, datatype evolution across versions.

## Source Profile Schema

```yaml
# Source Profile Definition
# Each interface/feed gets its own profile

source_profile:
  id: epic_adt_feed_hosp_a
  name: "Epic ADT Feed - Hospital A"
  version: "1.0.0"

  # ─────────────────────────────────────────────────────────────────
  # HL7v2 Configuration
  # ─────────────────────────────────────────────────────────────────
  hl7v2:
    default_version: "2.5.1"
    timezone: "America/New_York"

    # Encoding/Transport Layer
    encoding:
      charset_default: "UTF-8"           # Fallback if MSH-18 missing
      charset_detection: true            # Attempt auto-detection
      line_ending_mode: "tolerant"       # "strict" (CR only) or "tolerant" (CR/LF/CRLF)

    # Parsing Tolerance
    tolerate:
      missing_segments: ["PV1", "PD1", "OBR"]   # Don't fail if absent
      nte_anywhere: true                         # NTE can appear outside spec locations
      extra_components: true                     # Don't fail on XCN with 23 vs 14 components
      unknown_segments: true                     # Preserve but don't require mapping
      non_standard_delimiters: true              # Handle MSH-2 variations

    # Datatype Version Handling
    datatypes:
      xcn_component_count: "flexible"    # "strict" = fail if wrong count, "flexible" = parse available
      cx_component_count: "flexible"
      xpn_component_count: "flexible"

    # Event Disambiguation
    event_classification:
      # A01 behavior depends on PV1-2 (Patient Class)
      adt_a01:
        default: "patient_admit"
        rules:
          - condition: "PV1.2 == 'I'"
            event: "inpatient_admit"
          - condition: "PV1.2 == 'O'"
            event: "outpatient_registration"
          - condition: "PV1.2 == 'E'"
            event: "emergency_registration"
          - condition: "PV1.2 == 'P'"
            event: "preadmit"
          - condition: "PV1.2 == 'R'"
            event: "recurring_patient"

      # A04 can be registration or admit depending on source
      adt_a04:
        default: "outpatient_registration"

  # ─────────────────────────────────────────────────────────────────
  # EDI/X12 Configuration (Companion Guides)
  # ─────────────────────────────────────────────────────────────────
  edi:
    # Enable payer-specific companion guide validation.
    # Values:
    #   - "auto" (auto-detect from ISA/GS/ST + payer loop)
    #   - "<guide-id>" (built-in guide ID)
    #   - "<path>" (guide YAML/JSON file)
    companion_guide: "auto"

    # Optionally load additional guide files from a directory.
    # Files with .yaml/.yml/.json extensions are loaded.
    companion_guide_dir: "./guides"

  # ─────────────────────────────────────────────────────────────────
  # Z-Segment Mappings
  # ─────────────────────────────────────────────────────────────────
  z_segments:
    # Always extract raw Z-segments as structured data
    preserve_raw: true

    # Optionally map to canonical extensions
    mappings:
      ZPD:
        - field: 1
          target: patient.extensions.vip_flag
          type: boolean
        - field: 2
          target: patient.extensions.department_code
          type: string

      ZVN:
        - field: 1
          target: encounter.extensions.visit_type_detail
          type: string
        - field: 2
          target: encounter.extensions.expected_los_days
          type: integer

      ZIN:
        - field: 1
          target: coverage.extensions.auth_number
          type: string

  # ─────────────────────────────────────────────────────────────────
  # Identifier Normalization
  # ─────────────────────────────────────────────────────────────────
  identifiers:
    # Map assigning authority strings to canonical systems
    assigning_authority_map:
      "HOSP_A": "urn:oid:1.2.3.4.5.6.7"
      "HOSP_A_MRN": "urn:oid:1.2.3.4.5.6.7"
      "ENTERPRISE": "urn:oid:1.2.3.4.5"
      "SSA": "urn:oid:2.16.840.1.113883.4.1"
      "CMS": "urn:oid:2.16.840.1.113883.4.927"
      "BCBS_VA": "urn:oid:2.16.840.1.113883.3.123"

    # Priority order for selecting "primary" identifier
    # First match wins
    primary_id_preference:
      - { type: "MR", assigner_contains: "HOSP_A" }
      - { type: "MR", assigner_contains: "ENTERPRISE" }
      - { type: "PI" }
      - { type: "SS" }

    # Validation rules (quarantine if invalid)
    validation:
      npi:
        enabled: true
        on_invalid: "warn"       # "error" | "warn" | "pass"
      dea:
        enabled: false
      mbi:
        enabled: true
        on_invalid: "warn"
      ssn:
        enabled: true
        on_invalid: "warn"

    # Normalization rules
    normalization:
      ssn:
        strip_dashes: true
        reject_patterns: ["000000000", "123456789", "111111111"]
      phone:
        strip_country_code: true    # Remove leading 1 for US
        normalize_to_digits: true
      mrn:
        strip_leading_zeros: false  # Some systems use them semantically
        uppercase: true

  # ─────────────────────────────────────────────────────────────────
  # Terminology Mapping
  # ─────────────────────────────────────────────────────────────────
  terminology:
    # Global settings
    strict_validation: false
    unknown_code_behavior: "warn"    # "error" | "warn" | "pass"

    # Version expectations per code system
    versions:
      loinc: "2.76"
      snomed: "2023-09-01"           # US Edition
      icd10cm: "2024"                # Fiscal year
      cpt: "2024"                    # Calendar year

    # LOCAL → Standard mappings
    mappings:
      - source_system: "LOCAL_LAB"
        target_system: "http://loinc.org"
        file: "./mappings/hosp_a_local_to_loinc.csv"

      - source_system: "LOCAL_DX"
        target_system: "http://hl7.org/fhir/sid/icd-10-cm"
        file: "./mappings/hosp_a_dx_to_icd10.csv"

      - source_system: "LOCAL_PROC"
        target_system: "http://www.ama-assn.org/go/cpt"
        file: "./mappings/hosp_a_proc_to_cpt.csv"

    # Panel expansion (optional)
    panels:
      expand_cbc: true               # CBC panel → individual LOINC components
      expand_bmp: true
      expand_cmp: true

  # ─────────────────────────────────────────────────────────────────
  # Data Quality & Metrics
  # ─────────────────────────────────────────────────────────────────
  quality:
    # Track these metrics per-feed
    metrics:
      - message_types_distribution
      - segment_presence_rates
      - encoding_anomalies
      - identifier_validity_rates
      - terminology_coverage_rates
      - z_segment_frequency

    # Alerting thresholds
    alerts:
      invalid_npi_rate:
        threshold: 0.05              # Alert if >5% invalid
        severity: "warning"
      unknown_loinc_rate:
        threshold: 0.20              # Alert if >20% unmapped
        severity: "info"
      missing_pv1_rate:
        threshold: 0.10
        severity: "warning"
```

## 3-Phase Parsing Pipeline

Source Profiles govern a 3-phase parsing pipeline:

```
┌─────────────────────────────────────────────────────────────────┐
│                    3-Phase Parsing Pipeline                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Phase 1: BYTE NORMALIZATION (Transport Layer)                  │
│  ──────────────────────────────────────────────────────────────│
│  • Normalize line endings (\r\n and \n → \r)                   │
│  • Detect character set (BOM + MSH-18) and decode safely       │
│  • Keep original bytes for audit/replay                         │
│  • Output: normalized UTF-8 string + original_bytes            │
│                                                                 │
│                           ↓                                     │
│                                                                 │
│  Phase 2: SYNTACTIC PARSE (Format Layer)                        │
│  ──────────────────────────────────────────────────────────────│
│  • Detect field separator and encoding chars from MSH-1/MSH-2  │
│  • Handle non-standard delimiter characters                     │
│  • Parse repetitions (~), components (^), subcomponents (&)    │
│  • Process escape sequences (\F\, \S\, \T\, \R\, \E\, \X..\)   │
│  • Preserve unknown segments and out-of-order NTEs             │
│  • Store raw component arrays for fidelity                      │
│  • Output: Message struct with segments[], raw_fields[]         │
│                                                                 │
│                           ↓                                     │
│                                                                 │
│  Phase 3: SEMANTIC EXTRACTION (Meaning Layer)                   │
│  ──────────────────────────────────────────────────────────────│
│  • Map message → canonical event(s) using Source Profile       │
│  • Apply event classification rules (A01 → inpatient_admit)    │
│  • Gracefully handle missing PV1, missing OBR, OBX-only flows  │
│  • Extract Z-segments generically + apply field mappings       │
│  • Normalize identifiers per profile rules                      │
│  • Map terminology per profile mappings                         │
│  • Generate parse_warnings[] for anomalies                      │
│  • Output: Canonical event(s) with full provenance             │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Implementation Types

```go
// SourceProfile represents configuration for a single data source/interface
type SourceProfile struct {
    ID      string `yaml:"id" json:"id"`
    Name    string `yaml:"name" json:"name"`
    Version string `yaml:"version" json:"version"`

    HL7v2       *HL7v2Config       `yaml:"hl7v2,omitempty" json:"hl7v2,omitempty"`
    ZSegments   *ZSegmentConfig    `yaml:"z_segments,omitempty" json:"z_segments,omitempty"`
    Identifiers *IdentifierConfig  `yaml:"identifiers,omitempty" json:"identifiers,omitempty"`
    Terminology *TerminologyConfig `yaml:"terminology,omitempty" json:"terminology,omitempty"`
    Quality     *QualityConfig     `yaml:"quality,omitempty" json:"quality,omitempty"`
}

// HL7v2Config governs HL7v2 parsing behavior
type HL7v2Config struct {
    DefaultVersion string            `yaml:"default_version" json:"default_version"`
    Timezone       string            `yaml:"timezone" json:"timezone"`
    Encoding       *EncodingConfig   `yaml:"encoding,omitempty" json:"encoding,omitempty"`
    Tolerate       *ToleranceConfig  `yaml:"tolerate,omitempty" json:"tolerate,omitempty"`
    Datatypes      *DatatypeConfig   `yaml:"datatypes,omitempty" json:"datatypes,omitempty"`
    EventRules     *EventRulesConfig `yaml:"event_classification,omitempty" json:"event_classification,omitempty"`
}

// ToleranceConfig defines what the parser accepts vs rejects
type ToleranceConfig struct {
    MissingSegments        []string `yaml:"missing_segments" json:"missing_segments"`
    NTEAnywhere            bool     `yaml:"nte_anywhere" json:"nte_anywhere"`
    ExtraComponents        bool     `yaml:"extra_components" json:"extra_components"`
    UnknownSegments        bool     `yaml:"unknown_segments" json:"unknown_segments"`
    NonStandardDelimiters  bool     `yaml:"non_standard_delimiters" json:"non_standard_delimiters"`
}

// IdentifierConfig governs ID normalization and validation
type IdentifierConfig struct {
    AssigningAuthorityMap map[string]string      `yaml:"assigning_authority_map" json:"assigning_authority_map"`
    PrimaryIDPreference   []IDPreferenceRule     `yaml:"primary_id_preference" json:"primary_id_preference"`
    Validation            map[string]*ValidatorConfig `yaml:"validation" json:"validation"`
    Normalization         *NormalizationConfig   `yaml:"normalization" json:"normalization"`
}

// IDPreferenceRule determines primary identifier selection
type IDPreferenceRule struct {
    Type            string `yaml:"type" json:"type"`
    AssignerContains string `yaml:"assigner_contains,omitempty" json:"assigner_contains,omitempty"`
    AssignerEquals   string `yaml:"assigner_equals,omitempty" json:"assigner_equals,omitempty"`
}

// ValidatorConfig for individual identifier validators
type ValidatorConfig struct {
    Enabled   bool   `yaml:"enabled" json:"enabled"`
    OnInvalid string `yaml:"on_invalid" json:"on_invalid"` // "error", "warn", "pass"
}

// TerminologyConfig governs code system mappings
type TerminologyConfig struct {
    StrictValidation    bool              `yaml:"strict_validation" json:"strict_validation"`
    UnknownCodeBehavior string            `yaml:"unknown_code_behavior" json:"unknown_code_behavior"`
    Versions            map[string]string `yaml:"versions" json:"versions"`
    Mappings            []TerminologyMapping `yaml:"mappings" json:"mappings"`
}

// TerminologyMapping defines LOCAL → Standard code mappings
type TerminologyMapping struct {
    SourceSystem string `yaml:"source_system" json:"source_system"`
    TargetSystem string `yaml:"target_system" json:"target_system"`
    File         string `yaml:"file" json:"file"`
}

// ZSegmentConfig defines Z-segment handling
type ZSegmentConfig struct {
    PreserveRaw bool                        `yaml:"preserve_raw" json:"preserve_raw"`
    Mappings    map[string][]ZFieldMapping  `yaml:"mappings" json:"mappings"`
}

// ZFieldMapping maps a Z-segment field to canonical extension
type ZFieldMapping struct {
    Field  int    `yaml:"field" json:"field"`
    Target string `yaml:"target" json:"target"`
    Type   string `yaml:"type" json:"type"` // string, boolean, integer, etc.
}
```

## Profile Loader

```go
// ProfileRegistry manages source profiles
type ProfileRegistry struct {
    profiles map[string]*SourceProfile
    mu       sync.RWMutex
}

// Load loads a profile from YAML file
func (r *ProfileRegistry) Load(path string) (*SourceProfile, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read profile: %w", err)
    }

    var wrapper struct {
        SourceProfile *SourceProfile `yaml:"source_profile"`
    }
    if err := yaml.Unmarshal(data, &wrapper); err != nil {
        return nil, fmt.Errorf("failed to parse profile: %w", err)
    }

    profile := wrapper.SourceProfile
    if err := r.validate(profile); err != nil {
        return nil, fmt.Errorf("invalid profile: %w", err)
    }

    r.mu.Lock()
    r.profiles[profile.ID] = profile
    r.mu.Unlock()

    return profile, nil
}

// Get retrieves a profile by ID
func (r *ProfileRegistry) Get(id string) (*SourceProfile, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    p, ok := r.profiles[id]
    return p, ok
}

// Default returns a sensible default profile for unknown sources
func (r *ProfileRegistry) Default() *SourceProfile {
    return &SourceProfile{
        ID:   "default",
        Name: "Default Profile",
        HL7v2: &HL7v2Config{
            DefaultVersion: "2.5.1",
            Tolerate: &ToleranceConfig{
                MissingSegments:       []string{"PV1", "PD1"},
                NTEAnywhere:           true,
                ExtraComponents:       true,
                UnknownSegments:       true,
                NonStandardDelimiters: true,
            },
        },
        Identifiers: &IdentifierConfig{
            Validation: map[string]*ValidatorConfig{
                "npi": {Enabled: true, OnInvalid: "warn"},
                "mbi": {Enabled: true, OnInvalid: "warn"},
            },
        },
        Terminology: &TerminologyConfig{
            StrictValidation:    false,
            UnknownCodeBehavior: "warn",
        },
    }
}
```

## Profile-Aware Parser

```go
// Parser uses a SourceProfile to govern parsing behavior
type Parser struct {
    profile *SourceProfile
    logger  *slog.Logger
}

// NewParser creates a parser with the given profile
func NewParser(profile *SourceProfile) *Parser {
    if profile == nil {
        profile = DefaultProfile()
    }
    return &Parser{
        profile: profile,
        logger:  slog.Default(),
    }
}

// Parse executes the 3-phase pipeline
func (p *Parser) Parse(raw []byte) (*ParseResult, error) {
    result := &ParseResult{
        SourceProfileID: p.profile.ID,
        ParseWarnings:   make([]ParseWarning, 0),
        OriginalBytes:   raw,
    }

    // Phase 1: Byte Normalization
    normalized, warnings := p.normalizeBytes(raw)
    result.ParseWarnings = append(result.ParseWarnings, warnings...)

    // Phase 2: Syntactic Parse
    msg, warnings, err := p.syntacticParse(normalized)
    if err != nil {
        return nil, fmt.Errorf("syntactic parse failed: %w", err)
    }
    result.ParseWarnings = append(result.ParseWarnings, warnings...)
    result.RawMessage = msg

    // Phase 3: Semantic Extraction
    event, warnings, err := p.semanticExtract(msg)
    if err != nil {
        return nil, fmt.Errorf("semantic extraction failed: %w", err)
    }
    result.ParseWarnings = append(result.ParseWarnings, warnings...)
    result.Event = event

    return result, nil
}

// ParseResult contains all outputs from the 3-phase pipeline
type ParseResult struct {
    SourceProfileID string          `json:"source_profile_id"`
    Event           interface{}     `json:"event"`
    RawMessage      *RawMessage     `json:"raw_message,omitempty"`
    ParseWarnings   []ParseWarning  `json:"parse_warnings,omitempty"`
    OriginalBytes   []byte          `json:"original_bytes,omitempty"`
}

// ParseWarning captures non-fatal issues during parsing
type ParseWarning struct {
    Phase   string `json:"phase"`    // "byte", "syntactic", "semantic"
    Code    string `json:"code"`     // e.g., "MISSING_PV1", "INVALID_NPI"
    Message string `json:"message"`
    Path    string `json:"path,omitempty"` // e.g., "PID.3.1"
}
```

## CLI Integration

```bash
# Parse using a specific source profile
fi-fhir parse --profile ./profiles/epic_adt.yaml message.hl7

# Parse with inline profile override
fi-fhir parse --profile ./profiles/epic_adt.yaml \
  --set 'hl7v2.tolerate.missing_segments=["PV1","PV2"]' \
  message.hl7

# List available profiles
fi-fhir profiles list

# Validate a profile
fi-fhir profiles validate ./profiles/epic_adt.yaml

# Generate profile from sample messages (future)
fi-fhir profiles infer ./samples/*.hl7 --output ./profiles/inferred.yaml
```

## Example Profiles

### Minimal Profile (Unknown Source)

```yaml
source_profile:
  id: unknown_default
  name: "Unknown Source - Tolerant Defaults"
  version: "1.0.0"

  hl7v2:
    default_version: "2.5.1"
    tolerate:
      missing_segments: ["PV1", "PD1", "OBR"]
      nte_anywhere: true
      extra_components: true
      unknown_segments: true
```

### Epic ADT Feed

```yaml
source_profile:
  id: epic_adt_prod
  name: "Epic ADT Production Feed"
  version: "2.1.0"

  hl7v2:
    default_version: "2.5.1"
    timezone: "America/New_York"
    encoding:
      charset_default: "UTF-8"
    tolerate:
      missing_segments: ["PD1"]
      nte_anywhere: true
    event_classification:
      adt_a01:
        rules:
          - condition: "PV1.2 == 'I'"
            event: "inpatient_admit"
          - condition: "PV1.2 == 'O'"
            event: "outpatient_registration"

  z_segments:
    preserve_raw: true
    mappings:
      ZPD:
        - field: 1
          target: patient.extensions.vip_flag
          type: boolean
        - field: 3
          target: patient.extensions.epic_mrn_checksum
          type: string

  identifiers:
    assigning_authority_map:
      "EPIC_HOSP": "urn:oid:1.2.840.114350.1.13.123.3.7.2.696570"
      "SSA": "urn:oid:2.16.840.1.113883.4.1"
    primary_id_preference:
      - { type: "MR", assigner_contains: "EPIC" }
      - { type: "SS" }
    validation:
      npi: { enabled: true, on_invalid: "warn" }

  terminology:
    strict_validation: false
    unknown_code_behavior: "warn"
```

### Cerner Lab Interface

```yaml
source_profile:
  id: cerner_lab_results
  name: "Cerner Lab Results Feed"
  version: "1.0.0"

  hl7v2:
    default_version: "2.4"
    timezone: "America/Chicago"
    tolerate:
      missing_segments: ["OBR"]   # Some Cerner configs send OBX-only

  terminology:
    mappings:
      - source_system: "CERNER_LAB"
        target_system: "http://loinc.org"
        file: "./mappings/cerner_lab_to_loinc.csv"
```

## Roadmap Impact

Moving Source Profiles to MVP requires:

### Weeks 1-4 (MVP-Hard Requirements) ✅
- [x] Profile YAML schema and loader - see `pkg/profile/profile.go:Registry`
- [x] All config types (HL7v2, ZSegments, Identifiers, Terminology, Quality)
- [x] Default() profile with sensible tolerances
- [x] Profile validation on load
- [x] Delimiter + encoding handling (Phase 1)
- [x] Tolerant missing segment handling - see `IsMissingSegmentTolerated()`
- [x] Event classification rules - see `GetEventClassification()`
- [x] Assigning authority mapping - see `GetAssigningAuthoritySystem()`

### Weeks 5-8 (Post-Stability) ⚠️
- [x] Terminology mapping tables - see `pkg/terminology/mapper.go`
- [x] NPI/MBI validators wired to quality checks - see `pkg/validate/identifiers.go`
- [ ] Z-segment field mapping beyond raw capture
- [ ] Profile inference from samples (`fi-fhir profiles infer`)

## Testing Strategy

```go
func TestSourceProfileLoading(t *testing.T) {
    tests := []struct {
        name    string
        yaml    string
        wantErr bool
    }{
        {
            name: "minimal valid",
            yaml: `source_profile:
  id: test
  name: Test
  version: "1.0"`,
            wantErr: false,
        },
        {
            name: "missing id",
            yaml: `source_profile:
  name: Test`,
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            reg := NewProfileRegistry()
            _, err := reg.LoadFromBytes([]byte(tt.yaml))
            if (err != nil) != tt.wantErr {
                t.Errorf("LoadFromBytes() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

## See Also

- [HL7V2-QUIRKS.md](HL7V2-QUIRKS.md) - Version differences and Z-segment details
- [IDENTIFIERS.md](IDENTIFIERS.md) - Identifier validation rules referenced by profiles
- [TERMINOLOGY.md](TERMINOLOGY.md) - Code mapping files referenced by profiles
- [WORKFLOW-DSL.md](WORKFLOW-DSL.md) - Routes use events produced by profile-driven parsing

## References

- [HL7 v2.5.1 Specification](http://www.hl7.eu/refactored/index.html)
- [FHIR R4 Identifier](https://www.hl7.org/fhir/R4/datatypes.html#Identifier)
- [LOINC Downloads](https://loinc.org/downloads/)
