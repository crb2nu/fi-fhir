// Package profile provides Source Profile configuration for healthcare integrations.
// A Source Profile defines how to parse and normalize data from a specific interface/feed.
package profile

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// SourceProfile represents configuration for a single data source/interface.
// This is the unit of scalability - each feed gets its own profile.
type SourceProfile struct {
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`

	HL7v2       *HL7v2Config       `yaml:"hl7v2,omitempty" json:"hl7v2,omitempty"`
	EDI         *EDIConfig         `yaml:"edi,omitempty" json:"edi,omitempty"`
	ZSegments   *ZSegmentConfig    `yaml:"z_segments,omitempty" json:"z_segments,omitempty"`
	Identifiers *IdentifierConfig  `yaml:"identifiers,omitempty" json:"identifiers,omitempty"`
	Terminology *TerminologyConfig `yaml:"terminology,omitempty" json:"terminology,omitempty"`
	Quality     *QualityConfig     `yaml:"quality,omitempty" json:"quality,omitempty"`
}

// EDIConfig governs EDI/X12 parsing and companion guide validation.
type EDIConfig struct {
	// CompanionGuide enables payer-specific validation.
	//
	// Values:
	//  - "auto": auto-detect based on receiver ID / ST03 / payer NM1
	//  - "<guide-id>": a built-in guide ID
	//  - "<path>": a YAML/JSON file path
	CompanionGuide string `yaml:"companion_guide,omitempty" json:"companion_guide,omitempty"`

	// CompanionGuideDir loads additional guide files from a directory (YAML/JSON).
	CompanionGuideDir string `yaml:"companion_guide_dir,omitempty" json:"companion_guide_dir,omitempty"`
}

// HL7v2Config governs HL7v2 parsing behavior.
type HL7v2Config struct {
	DefaultVersion string            `yaml:"default_version" json:"default_version"`
	Timezone       string            `yaml:"timezone" json:"timezone"`
	Encoding       *EncodingConfig   `yaml:"encoding,omitempty" json:"encoding,omitempty"`
	Tolerate       *ToleranceConfig  `yaml:"tolerate,omitempty" json:"tolerate,omitempty"`
	Datatypes      *DatatypeConfig   `yaml:"datatypes,omitempty" json:"datatypes,omitempty"`
	EventRules     *EventRulesConfig `yaml:"event_classification,omitempty" json:"event_classification,omitempty"`
}

// EncodingConfig controls character encoding behavior.
type EncodingConfig struct {
	CharsetDefault   string `yaml:"charset_default" json:"charset_default"`
	CharsetDetection bool   `yaml:"charset_detection" json:"charset_detection"`
	LineEndingMode   string `yaml:"line_ending_mode" json:"line_ending_mode"` // "strict" or "tolerant"
}

// ToleranceConfig defines what the parser accepts vs rejects.
type ToleranceConfig struct {
	MissingSegments       []string `yaml:"missing_segments" json:"missing_segments"`
	NTEAnywhere           bool     `yaml:"nte_anywhere" json:"nte_anywhere"`
	ExtraComponents       bool     `yaml:"extra_components" json:"extra_components"`
	UnknownSegments       bool     `yaml:"unknown_segments" json:"unknown_segments"`
	NonStandardDelimiters bool     `yaml:"non_standard_delimiters" json:"non_standard_delimiters"`
}

// DatatypeConfig controls version-aware datatype parsing.
type DatatypeConfig struct {
	XCNComponentCount string `yaml:"xcn_component_count" json:"xcn_component_count"` // "strict" or "flexible"
	CXComponentCount  string `yaml:"cx_component_count" json:"cx_component_count"`
	XPNComponentCount string `yaml:"xpn_component_count" json:"xpn_component_count"`
}

// EventRulesConfig defines event classification rules.
type EventRulesConfig struct {
	ADTA01 *EventRule `yaml:"adt_a01,omitempty" json:"adt_a01,omitempty"`
	ADTA04 *EventRule `yaml:"adt_a04,omitempty" json:"adt_a04,omitempty"`
	ADTA08 *EventRule `yaml:"adt_a08,omitempty" json:"adt_a08,omitempty"`
}

// EventRule defines how to classify an event based on message content.
type EventRule struct {
	Default string               `yaml:"default" json:"default"`
	Rules   []ClassificationRule `yaml:"rules,omitempty" json:"rules,omitempty"`
}

// ClassificationRule maps a condition to a semantic event type.
type ClassificationRule struct {
	Condition string `yaml:"condition" json:"condition"` // e.g., "PV1.2 == 'I'"
	Event     string `yaml:"event" json:"event"`         // e.g., "inpatient_admit"
}

// ZSegmentConfig defines Z-segment handling.
type ZSegmentConfig struct {
	PreserveRaw bool                       `yaml:"preserve_raw" json:"preserve_raw"`
	Mappings    map[string][]ZFieldMapping `yaml:"mappings" json:"mappings"`
}

// ZFieldMapping maps a Z-segment field to a canonical extension.
type ZFieldMapping struct {
	Field  int    `yaml:"field" json:"field"`
	Target string `yaml:"target" json:"target"`
	Type   string `yaml:"type" json:"type"` // string, boolean, integer, etc.
}

// IdentifierConfig governs ID normalization and validation.
type IdentifierConfig struct {
	AssigningAuthorityMap map[string]string           `yaml:"assigning_authority_map" json:"assigning_authority_map"`
	PrimaryIDPreference   []IDPreferenceRule          `yaml:"primary_id_preference" json:"primary_id_preference"`
	Validation            map[string]*ValidatorConfig `yaml:"validation" json:"validation"`
	Normalization         *NormalizationConfig        `yaml:"normalization" json:"normalization"`
}

// IDPreferenceRule determines primary identifier selection.
type IDPreferenceRule struct {
	Type             string `yaml:"type" json:"type"`
	AssignerContains string `yaml:"assigner_contains,omitempty" json:"assigner_contains,omitempty"`
	AssignerEquals   string `yaml:"assigner_equals,omitempty" json:"assigner_equals,omitempty"`
}

// ValidatorConfig for individual identifier validators.
type ValidatorConfig struct {
	Enabled   bool   `yaml:"enabled" json:"enabled"`
	OnInvalid string `yaml:"on_invalid" json:"on_invalid"` // "error", "warn", "pass"
}

// NormalizationConfig defines how to normalize identifiers.
type NormalizationConfig struct {
	SSN   *SSNNormConfig   `yaml:"ssn,omitempty" json:"ssn,omitempty"`
	Phone *PhoneNormConfig `yaml:"phone,omitempty" json:"phone,omitempty"`
	MRN   *MRNNormConfig   `yaml:"mrn,omitempty" json:"mrn,omitempty"`
}

// SSNNormConfig for SSN normalization.
type SSNNormConfig struct {
	StripDashes    bool     `yaml:"strip_dashes" json:"strip_dashes"`
	RejectPatterns []string `yaml:"reject_patterns" json:"reject_patterns"`
}

// PhoneNormConfig for phone normalization.
type PhoneNormConfig struct {
	StripCountryCode  bool `yaml:"strip_country_code" json:"strip_country_code"`
	NormalizeToDigits bool `yaml:"normalize_to_digits" json:"normalize_to_digits"`
}

// MRNNormConfig for MRN normalization.
type MRNNormConfig struct {
	StripLeadingZeros bool `yaml:"strip_leading_zeros" json:"strip_leading_zeros"`
	Uppercase         bool `yaml:"uppercase" json:"uppercase"`
}

// TerminologyConfig governs code system mappings.
type TerminologyConfig struct {
	StrictValidation    bool                 `yaml:"strict_validation" json:"strict_validation"`
	UnknownCodeBehavior string               `yaml:"unknown_code_behavior" json:"unknown_code_behavior"` // "error", "warn", "pass"
	Versions            map[string]string    `yaml:"versions" json:"versions"`
	Mappings            []TerminologyMapping `yaml:"mappings" json:"mappings"`
}

// TerminologyMapping defines LOCAL → Standard code mappings.
type TerminologyMapping struct {
	SourceSystem string `yaml:"source_system" json:"source_system"`
	TargetSystem string `yaml:"target_system" json:"target_system"`
	File         string `yaml:"file" json:"file"`
}

// QualityConfig defines quality tracking settings.
type QualityConfig struct {
	Metrics []string                `yaml:"metrics" json:"metrics"`
	Alerts  map[string]*AlertConfig `yaml:"alerts" json:"alerts"`
}

// AlertConfig defines alerting thresholds.
type AlertConfig struct {
	Threshold float64 `yaml:"threshold" json:"threshold"`
	Severity  string  `yaml:"severity" json:"severity"`
}

// Registry manages source profiles.
type Registry struct {
	profiles map[string]*SourceProfile
	mu       sync.RWMutex
}

// NewRegistry creates a new profile registry.
func NewRegistry() *Registry {
	return &Registry{
		profiles: make(map[string]*SourceProfile),
	}
}

// LoadFromFile loads a profile from a YAML file.
func (r *Registry) LoadFromFile(path string) (*SourceProfile, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path from trusted caller
	if err != nil {
		return nil, fmt.Errorf("failed to read profile file: %w", err)
	}

	return r.LoadFromBytes(data)
}

// LoadFromBytes loads a profile from YAML bytes.
func (r *Registry) LoadFromBytes(data []byte) (*SourceProfile, error) {
	var wrapper struct {
		SourceProfile *SourceProfile `yaml:"source_profile"`
	}

	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse profile YAML: %w", err)
	}

	profile := wrapper.SourceProfile
	if profile == nil {
		return nil, fmt.Errorf("missing source_profile root element")
	}

	if err := r.validate(profile); err != nil {
		return nil, fmt.Errorf("invalid profile: %w", err)
	}

	r.mu.Lock()
	r.profiles[profile.ID] = profile
	r.mu.Unlock()

	return profile, nil
}

// validate checks that a profile is valid.
func (r *Registry) validate(p *SourceProfile) error {
	if p.ID == "" {
		return fmt.Errorf("profile id is required")
	}
	if p.Name == "" {
		return fmt.Errorf("profile name is required")
	}
	return nil
}

// Get retrieves a profile by ID.
func (r *Registry) Get(id string) (*SourceProfile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.profiles[id]
	return p, ok
}

// List returns all registered profile IDs.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.profiles))
	for id := range r.profiles {
		ids = append(ids, id)
	}
	return ids
}

// Default returns a sensible default profile for unknown sources.
func Default() *SourceProfile {
	return &SourceProfile{
		ID:      "default",
		Name:    "Default Profile",
		Version: "1.0.0",
		HL7v2: &HL7v2Config{
			DefaultVersion: "2.5.1",
			Encoding: &EncodingConfig{
				CharsetDefault:   "UTF-8",
				CharsetDetection: true,
				LineEndingMode:   "tolerant",
			},
			Tolerate: &ToleranceConfig{
				MissingSegments:       []string{"PV1", "PD1", "OBR"},
				NTEAnywhere:           true,
				ExtraComponents:       true,
				UnknownSegments:       true,
				NonStandardDelimiters: true,
			},
			Datatypes: &DatatypeConfig{
				XCNComponentCount: "flexible",
				CXComponentCount:  "flexible",
				XPNComponentCount: "flexible",
			},
			EventRules: &EventRulesConfig{
				ADTA01: &EventRule{
					Default: "patient_admit",
					Rules: []ClassificationRule{
						{Condition: "PV1.2 == 'I'", Event: "inpatient_admit"},
						{Condition: "PV1.2 == 'O'", Event: "outpatient_registration"},
						{Condition: "PV1.2 == 'E'", Event: "emergency_registration"},
						{Condition: "PV1.2 == 'P'", Event: "preadmit"},
						{Condition: "PV1.2 == 'R'", Event: "recurring_patient"},
					},
				},
				ADTA04: &EventRule{
					Default: "outpatient_registration",
				},
			},
		},
		ZSegments: &ZSegmentConfig{
			PreserveRaw: true,
			Mappings:    make(map[string][]ZFieldMapping),
		},
		Identifiers: &IdentifierConfig{
			AssigningAuthorityMap: make(map[string]string),
			Validation: map[string]*ValidatorConfig{
				"npi": {Enabled: true, OnInvalid: "warn"},
				"mbi": {Enabled: true, OnInvalid: "warn"},
				"ssn": {Enabled: true, OnInvalid: "warn"},
			},
			Normalization: &NormalizationConfig{
				SSN: &SSNNormConfig{
					StripDashes:    true,
					RejectPatterns: []string{"000000000", "123456789", "111111111"},
				},
				Phone: &PhoneNormConfig{
					StripCountryCode:  true,
					NormalizeToDigits: true,
				},
			},
		},
		Terminology: &TerminologyConfig{
			StrictValidation:    false,
			UnknownCodeBehavior: "warn",
		},
	}
}

// IsMissingSegmentTolerated checks if a missing segment should be tolerated.
func (p *SourceProfile) IsMissingSegmentTolerated(segmentID string) bool {
	if p.HL7v2 == nil || p.HL7v2.Tolerate == nil {
		return false
	}
	for _, s := range p.HL7v2.Tolerate.MissingSegments {
		if s == segmentID {
			return true
		}
	}
	return false
}

// GetEventClassification returns the semantic event type for an HL7v2 message.
func (p *SourceProfile) GetEventClassification(messageType, patientClass string) string {
	if p.HL7v2 == nil || p.HL7v2.EventRules == nil {
		return ""
	}

	var rule *EventRule
	switch messageType {
	case "ADT^A01", "ADT^A01^ADT_A01":
		rule = p.HL7v2.EventRules.ADTA01
	case "ADT^A04", "ADT^A04^ADT_A04":
		rule = p.HL7v2.EventRules.ADTA04
	case "ADT^A08", "ADT^A08^ADT_A08":
		rule = p.HL7v2.EventRules.ADTA08
	}

	if rule == nil {
		return ""
	}

	// Check classification rules based on patient class
	for _, r := range rule.Rules {
		// Simple condition matching: "PV1.2 == 'X'"
		if matchesCondition(r.Condition, patientClass) {
			return r.Event
		}
	}

	return rule.Default
}

// matchesCondition is a simple condition matcher for PV1.2 checks.
func matchesCondition(condition, patientClass string) bool {
	// Parse simple conditions like "PV1.2 == 'I'"
	// This is intentionally simple - can be extended later

	// Check if it's a PV1.2 condition
	prefix := "PV1.2 == '"
	if len(condition) < len(prefix)+2 { // need at least prefix + char + closing quote
		return false
	}

	if condition[:len(prefix)] != prefix {
		return false
	}

	// Extract the value between quotes
	rest := condition[len(prefix):]
	quoteIdx := 0
	for i, c := range rest {
		if c == '\'' {
			quoteIdx = i
			break
		}
	}

	if quoteIdx == 0 {
		return false
	}

	expected := rest[:quoteIdx]
	return expected == patientClass
}

// GetAssigningAuthoritySystem maps an assigning authority to a system URI/OID.
func (p *SourceProfile) GetAssigningAuthoritySystem(authority string) string {
	if p.Identifiers == nil || p.Identifiers.AssigningAuthorityMap == nil {
		return ""
	}
	return p.Identifiers.AssigningAuthorityMap[authority]
}

// ShouldValidateNPI returns whether NPI validation is enabled.
func (p *SourceProfile) ShouldValidateNPI() (enabled bool, onInvalid string) {
	if p.Identifiers == nil || p.Identifiers.Validation == nil {
		return false, "pass"
	}
	cfg, ok := p.Identifiers.Validation["npi"]
	if !ok || cfg == nil {
		return false, "pass"
	}
	return cfg.Enabled, cfg.OnInvalid
}
