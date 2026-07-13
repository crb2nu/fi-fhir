package processor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/profile"
)

const maxExecutableProfileJSONBytes = 256 << 10

var (
	// ErrInvalidSourceProfile means persisted profile content is malformed or ambiguous.
	ErrInvalidSourceProfile = errors.New("invalid source profile")
	// ErrUnsupportedSourceProfile means valid authored behavior is outside the executable v1 subset.
	ErrUnsupportedSourceProfile = errors.New("unsupported source profile")

	adtA01ConditionPattern = regexp.MustCompile(`^PV1\.2 == '[A-Z0-9]'$`)
)

type compiledSourceProfile struct {
	source   *profile.SourceProfile
	timezone *time.Location
}

type storedSourceProfileV1 struct {
	HL7v2       *storedHL7v2ProfileV1      `json:"hl7v2"`
	Identifiers *storedIdentifierProfileV1 `json:"identifiers,omitempty"`
	Terminology *storedTerminologyV1       `json:"terminology,omitempty"`
}

type storedHL7v2ProfileV1 struct {
	DefaultVersion       string                   `json:"default_version"`
	Timezone             string                   `json:"timezone"`
	Tolerance            *storedToleranceV1       `json:"tolerance,omitempty"`
	EventClassifications []storedEventClassRuleV1 `json:"event_classifications"`
}

type storedToleranceV1 struct {
	MissingSegments       []string `json:"missing_segments,omitempty"`
	NTEAnywhere           bool     `json:"nte_anywhere"`
	ExtraComponents       bool     `json:"extra_components"`
	UnknownSegments       bool     `json:"unknown_segments"`
	NonStandardDelimiters bool     `json:"non_standard_delimiters"`
}

type storedEventClassRuleV1 struct {
	MessageType string `json:"message_type"`
	Condition   string `json:"condition,omitempty"`
	EventType   string `json:"event_type"`
	Priority    int    `json:"priority"`
}

type storedIdentifierProfileV1 struct {
	AssigningAuthorities []storedAssigningAuthorityV1 `json:"assigning_authorities,omitempty"`
	PrimaryIDPreference  []json.RawMessage            `json:"primary_id_preference,omitempty"`
	Validation           *storedValidationV1          `json:"validation,omitempty"`
	Normalization        *storedNormalizationV1       `json:"normalization,omitempty"`
}

type storedAssigningAuthorityV1 struct {
	Code   string `json:"code"`
	System string `json:"system"`
	Name   string `json:"name,omitempty"`
}

type storedValidationV1 struct {
	NPI *storedValidatorSettingV1 `json:"npi,omitempty"`
	MBI *storedValidatorSettingV1 `json:"mbi,omitempty"`
	SSN *storedValidatorSettingV1 `json:"ssn,omitempty"`
}

type storedValidatorSettingV1 struct {
	Enabled   bool   `json:"enabled"`
	OnInvalid string `json:"on_invalid"`
}

type storedNormalizationV1 struct {
	SSNStripDashes    *bool    `json:"ssn_strip_dashes,omitempty"`
	SSNRejectPatterns []string `json:"ssn_reject_patterns,omitempty"`
	PhoneNormalize    *bool    `json:"phone_normalize,omitempty"`
	PhoneFormat       string   `json:"phone_format,omitempty"`
}

type storedTerminologyV1 struct {
	Mappings []json.RawMessage `json:"mappings,omitempty"`
}

type orderedEventClassRuleV1 struct {
	storedEventClassRuleV1
	position int
}

func compileSourceProfile(resolved ResolvedArtifactRevisions) (compiledSourceProfile, error) {
	raw := resolved.ProfileJSON()
	if len(raw) == 0 || len(raw) > maxExecutableProfileJSONBytes {
		return compiledSourceProfile{}, invalidSourceProfile("$")
	}
	ref := resolved.ProfileReference()
	computed, err := newProfileRevisionReference(ref.ArtifactID, ref.RevisionID, raw)
	if err != nil || computed != ref {
		return compiledSourceProfile{}, invalidSourceProfile("$")
	}

	stored, err := decodeStoredSourceProfileV1(raw)
	if err != nil {
		return compiledSourceProfile{}, err
	}
	if stored.HL7v2 == nil {
		return compiledSourceProfile{}, invalidSourceProfile("hl7v2")
	}
	if !supportedHL7Version(stored.HL7v2.DefaultVersion) {
		return compiledSourceProfile{}, unsupportedSourceProfile("hl7v2.default_version")
	}
	if stored.HL7v2.Timezone == "Local" {
		return compiledSourceProfile{}, unsupportedSourceProfile("hl7v2.timezone")
	}
	location, err := time.LoadLocation(stored.HL7v2.Timezone)
	if err != nil || stored.HL7v2.Timezone == "" {
		return compiledSourceProfile{}, invalidSourceProfile("hl7v2.timezone")
	}

	tolerance, err := compileToleranceV1(stored.HL7v2.Tolerance)
	if err != nil {
		return compiledSourceProfile{}, err
	}
	eventRule, err := compileADTA01RulesV1(stored.HL7v2.EventClassifications)
	if err != nil {
		return compiledSourceProfile{}, err
	}
	identifiers, err := compileIdentifiersV1(stored.Identifiers)
	if err != nil {
		return compiledSourceProfile{}, err
	}
	if stored.Terminology != nil && len(stored.Terminology.Mappings) > 0 {
		return compiledSourceProfile{}, unsupportedSourceProfile("terminology.mappings")
	}

	compiled := &profile.SourceProfile{
		ID:      ref.ArtifactID,
		Name:    ref.ArtifactID,
		Version: ref.RevisionID,
		HL7v2: &profile.HL7v2Config{
			DefaultVersion: stored.HL7v2.DefaultVersion,
			Timezone:       stored.HL7v2.Timezone,
			Encoding: &profile.EncodingConfig{
				CharsetDefault:   "UTF-8",
				CharsetDetection: false,
				LineEndingMode:   "strict",
			},
			Tolerate: tolerance,
			EventRules: &profile.EventRulesConfig{
				ADTA01: eventRule,
			},
		},
		ZSegments: &profile.ZSegmentConfig{
			PreserveRaw: false,
			Mappings:    make(map[string][]profile.ZFieldMapping),
		},
		Identifiers: identifiers,
	}
	return compiledSourceProfile{source: compiled, timezone: location}, nil
}

func decodeStoredSourceProfileV1(raw []byte) (storedSourceProfileV1, error) {
	if _, err := canonicalJSONObject(raw); err != nil {
		return storedSourceProfileV1{}, invalidSourceProfile("$")
	}
	if err := validateStoredSourceProfileKeys(raw); err != nil {
		return storedSourceProfileV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var stored storedSourceProfileV1
	if err := decoder.Decode(&stored); err != nil {
		return storedSourceProfileV1{}, invalidSourceProfile("$")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return storedSourceProfileV1{}, invalidSourceProfile("$")
	}
	return stored, nil
}

func validateStoredSourceProfileKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return invalidSourceProfile("$")
	}
	root, err := exactProfileObject(value, "$", "hl7v2", "identifiers", "terminology")
	if err != nil {
		return err
	}
	if rawHL7, present := root["hl7v2"]; present && rawHL7 != nil {
		hl7, err := exactProfileObject(rawHL7, "hl7v2", "default_version", "timezone", "tolerance", "event_classifications")
		if err != nil {
			return err
		}
		if rawTolerance, present := hl7["tolerance"]; present && rawTolerance != nil {
			if _, err := exactProfileObject(
				rawTolerance,
				"hl7v2.tolerance",
				"missing_segments",
				"nte_anywhere",
				"extra_components",
				"unknown_segments",
				"non_standard_delimiters",
			); err != nil {
				return err
			}
		}
		if rawRules, present := hl7["event_classifications"]; present && rawRules != nil {
			rules, ok := rawRules.([]any)
			if !ok {
				return invalidSourceProfile("hl7v2.event_classifications")
			}
			for _, rawRule := range rules {
				if _, err := exactProfileObject(rawRule, "hl7v2.event_classifications", "message_type", "condition", "event_type", "priority"); err != nil {
					return err
				}
			}
		}
	}
	if rawIdentifiers, present := root["identifiers"]; present && rawIdentifiers != nil {
		identifiers, err := exactProfileObject(
			rawIdentifiers,
			"identifiers",
			"assigning_authorities",
			"primary_id_preference",
			"validation",
			"normalization",
		)
		if err != nil {
			return err
		}
		if rawAuthorities, present := identifiers["assigning_authorities"]; present && rawAuthorities != nil {
			authorities, ok := rawAuthorities.([]any)
			if !ok {
				return invalidSourceProfile("identifiers.assigning_authorities")
			}
			for _, rawAuthority := range authorities {
				if _, err := exactProfileObject(rawAuthority, "identifiers.assigning_authorities", "code", "system", "name"); err != nil {
					return err
				}
			}
		}
		if rawValidation, present := identifiers["validation"]; present && rawValidation != nil {
			validation, err := exactProfileObject(rawValidation, "identifiers.validation", "npi", "mbi", "ssn")
			if err != nil {
				return err
			}
			for _, key := range []string{"npi", "mbi", "ssn"} {
				if setting, present := validation[key]; present && setting != nil {
					if _, err := exactProfileObject(setting, "identifiers.validation", "enabled", "on_invalid"); err != nil {
						return err
					}
				}
			}
		}
		if rawNormalization, present := identifiers["normalization"]; present && rawNormalization != nil {
			if _, err := exactProfileObject(
				rawNormalization,
				"identifiers.normalization",
				"ssn_strip_dashes",
				"ssn_reject_patterns",
				"phone_normalize",
				"phone_format",
			); err != nil {
				return err
			}
		}
	}
	if rawTerminology, present := root["terminology"]; present && rawTerminology != nil {
		if _, err := exactProfileObject(rawTerminology, "terminology", "mappings"); err != nil {
			return err
		}
	}
	return nil
}

func exactProfileObject(value any, path string, allowed ...string) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, invalidSourceProfile(path)
	}
	allowedKeys := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedKeys[key] = struct{}{}
	}
	for key := range object {
		if _, allowed := allowedKeys[key]; !allowed {
			return nil, invalidSourceProfile(path)
		}
	}
	return object, nil
}

func compileToleranceV1(stored *storedToleranceV1) (*profile.ToleranceConfig, error) {
	compiled := &profile.ToleranceConfig{}
	if stored == nil {
		return compiled, nil
	}
	if stored.NTEAnywhere || stored.ExtraComponents || stored.UnknownSegments || stored.NonStandardDelimiters {
		return nil, unsupportedSourceProfile("hl7v2.tolerance")
	}
	seen := make(map[string]struct{}, len(stored.MissingSegments))
	for _, segment := range stored.MissingSegments {
		if segment != "PV1" {
			return nil, unsupportedSourceProfile("hl7v2.tolerance.missing_segments")
		}
		if _, duplicate := seen[segment]; duplicate {
			return nil, invalidSourceProfile("hl7v2.tolerance.missing_segments")
		}
		seen[segment] = struct{}{}
		compiled.MissingSegments = append(compiled.MissingSegments, segment)
	}
	return compiled, nil
}

func compileADTA01RulesV1(stored []storedEventClassRuleV1) (*profile.EventRule, error) {
	if len(stored) == 0 || len(stored) > 64 {
		return nil, invalidSourceProfile("hl7v2.event_classifications")
	}
	ordered := make([]orderedEventClassRuleV1, 0, len(stored))
	priorities := make(map[int]struct{}, len(stored))
	conditions := make(map[string]struct{}, len(stored))
	defaultEvent := ""
	for position, rule := range stored {
		if rule.MessageType != "ADT^A01" && rule.MessageType != "ADT^A01^ADT_A01" {
			return nil, unsupportedSourceProfile("hl7v2.event_classifications.message_type")
		}
		if !supportedA01Classification(rule.EventType) {
			return nil, unsupportedSourceProfile("hl7v2.event_classifications.event_type")
		}
		if rule.Priority < 0 {
			return nil, invalidSourceProfile("hl7v2.event_classifications.priority")
		}
		if _, duplicate := priorities[rule.Priority]; duplicate {
			return nil, invalidSourceProfile("hl7v2.event_classifications.priority")
		}
		priorities[rule.Priority] = struct{}{}
		if rule.Condition == "" {
			if defaultEvent != "" {
				return nil, invalidSourceProfile("hl7v2.event_classifications.condition")
			}
			defaultEvent = rule.EventType
			continue
		}
		if !adtA01ConditionPattern.MatchString(rule.Condition) {
			return nil, invalidSourceProfile("hl7v2.event_classifications.condition")
		}
		if _, duplicate := conditions[rule.Condition]; duplicate {
			return nil, invalidSourceProfile("hl7v2.event_classifications.condition")
		}
		conditions[rule.Condition] = struct{}{}
		ordered = append(ordered, orderedEventClassRuleV1{storedEventClassRuleV1: rule, position: position})
	}
	if defaultEvent == "" {
		return nil, invalidSourceProfile("hl7v2.event_classifications")
	}
	sort.SliceStable(ordered, func(left, right int) bool {
		if ordered[left].Priority == ordered[right].Priority {
			return ordered[left].position < ordered[right].position
		}
		return ordered[left].Priority < ordered[right].Priority
	})
	compiled := &profile.EventRule{Default: defaultEvent}
	for _, rule := range ordered {
		compiled.Rules = append(compiled.Rules, profile.ClassificationRule{
			Condition: rule.Condition,
			Event:     rule.EventType,
		})
	}
	return compiled, nil
}

func compileIdentifiersV1(stored *storedIdentifierProfileV1) (*profile.IdentifierConfig, error) {
	compiled := &profile.IdentifierConfig{
		AssigningAuthorityMap: make(map[string]string),
	}
	if stored == nil {
		return compiled, nil
	}
	if len(stored.PrimaryIDPreference) > 0 {
		return nil, unsupportedSourceProfile("identifiers.primary_id_preference")
	}
	if stored.Validation != nil && (stored.Validation.NPI != nil || stored.Validation.MBI != nil || stored.Validation.SSN != nil) {
		return nil, unsupportedSourceProfile("identifiers.validation")
	}
	for _, authority := range stored.AssigningAuthorities {
		if !validBoundedProfileToken(authority.Code, 64) || !validBoundedProfileToken(authority.System, 256) {
			return nil, invalidSourceProfile("identifiers.assigning_authorities")
		}
		if _, duplicate := compiled.AssigningAuthorityMap[authority.Code]; duplicate {
			return nil, invalidSourceProfile("identifiers.assigning_authorities")
		}
		compiled.AssigningAuthorityMap[authority.Code] = authority.System
	}

	if stored.Normalization == nil {
		return compiled, nil
	}
	if stored.Normalization.SSNStripDashes != nil && !*stored.Normalization.SSNStripDashes {
		return nil, unsupportedSourceProfile("identifiers.normalization.ssn_strip_dashes")
	}
	if stored.Normalization.PhoneNormalize != nil && *stored.Normalization.PhoneNormalize {
		return nil, unsupportedSourceProfile("identifiers.normalization.phone_normalize")
	}
	if stored.Normalization.PhoneFormat != "" {
		return nil, unsupportedSourceProfile("identifiers.normalization.phone_format")
	}
	var patterns []string
	seenPatterns := make(map[string]struct{}, len(stored.Normalization.SSNRejectPatterns)+4)
	if stored.Normalization.SSNRejectPatterns != nil {
		patterns = []string{"000000000", "123456789", "111111111", "999999999"}
		for _, pattern := range patterns {
			seenPatterns[pattern] = struct{}{}
		}
	}
	for _, pattern := range stored.Normalization.SSNRejectPatterns {
		if len(pattern) != 9 || strings.Trim(pattern, "0123456789") != "" {
			return nil, invalidSourceProfile("identifiers.normalization.ssn_reject_patterns")
		}
		if _, duplicate := seenPatterns[pattern]; duplicate {
			continue
		}
		seenPatterns[pattern] = struct{}{}
		patterns = append(patterns, pattern)
	}
	compiled.Normalization = &profile.NormalizationConfig{
		SSN: &profile.SSNNormConfig{
			StripDashes:    true,
			RejectPatterns: patterns,
		},
	}
	return compiled, nil
}

func supportedHL7Version(version string) bool {
	switch version {
	case "2.3", "2.3.1", "2.4", "2.5", "2.5.1", "2.6", "2.7", "2.7.1", "2.8", "2.8.1", "2.8.2":
		return true
	default:
		return false
	}
}

func supportedA01Classification(event string) bool {
	switch event {
	case "patient_admit", "inpatient_admit", "outpatient_registration", "emergency_registration", "preadmit", "recurring_patient":
		return true
	default:
		return false
	}
}

func validBoundedProfileToken(value string, limit int) bool {
	return value != "" && len(value) <= limit && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "\r\n\x00")
}

func invalidSourceProfile(path string) error {
	return fmt.Errorf("%w: %s", ErrInvalidSourceProfile, path)
}

func unsupportedSourceProfile(path string) error {
	return fmt.Errorf("%w: %s", ErrUnsupportedSourceProfile, path)
}
