package fhir

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ValidationMode is the closed set of conformance modes ValidateJSON accepts.
//
// It is closed on purpose. Before Slice 5.1a the mode was an open string
// compared byte-exactly against "us-core", so every other value — including
// "US-Core", "uscore", and "" — silently disabled both the required-element and
// the profile-presence checks and reported a non-conformant resource as clean.
// A typo in a deployment's flag turned conformance checking off. Unrecognised
// modes are now an error, so the failure mode is loud instead of green.
type ValidationMode string

const (
	// ModeStructural validates JSON structure only: a payload must parse and
	// every resource must carry a resourceType. No US Core expectation applies.
	ModeStructural ValidationMode = "none"
	// ModeUSCore additionally applies this package's US Core required-element
	// and profile-presence checks.
	ModeUSCore ValidationMode = "us-core"
)

// ErrUnknownValidationMode is returned by ParseValidationMode and ValidateJSON
// for any mode outside the closed set. It never means "validate less".
var ErrUnknownValidationMode = errors.New("unknown FHIR validation mode")

// ValidationModes returns the accepted mode strings in a stable order, so a CLI
// or an API can render the set rather than hard-coding a copy of it.
func ValidationModes() []string {
	modes := make([]string, 0, len(validationModes))
	for mode := range validationModes {
		modes = append(modes, string(mode))
	}
	sort.Strings(modes)
	return modes
}

var validationModes = map[ValidationMode]bool{
	ModeStructural: true,
	ModeUSCore:     true,
}

// ParseValidationMode normalises one mode string, case-insensitively and
// ignoring surrounding whitespace, or reports ErrUnknownValidationMode.
//
// The empty string is an error rather than a synonym for "none": a zero-value
// ValidationOptions must not be the configuration that silently checks nothing.
func ParseValidationMode(mode string) (ValidationMode, error) {
	normalized := ValidationMode(strings.ToLower(strings.TrimSpace(mode)))
	if !validationModes[normalized] {
		return "", fmt.Errorf("%w: %q (expected one of %s)",
			ErrUnknownValidationMode, mode, strings.Join(ValidationModes(), ", "))
	}
	return normalized, nil
}

type ValidationOptions struct {
	// Mode controls which conformance checks to apply. It is parsed by
	// ParseValidationMode: case-insensitive, whitespace-trimmed, and closed.
	// Supported values:
	//   - "none": structural validation only
	//   - "us-core": enforce US Core-ish expectations (profile presence is a warning)
	//
	// Any other value, including the empty string, is rejected with
	// ErrUnknownValidationMode. See ValidationMode.
	Mode string
}

// ValidateJSON validates a FHIR JSON payload that may contain:
//   - a single resource object
//   - an array of resources
//   - a Bundle with entry[].resource
//
// It returns an OperationOutcome containing errors/warnings. The caller decides
// whether warnings should fail the operation.
// The mode is parsed before anything else, and an unrecognised mode is an error
// rather than a quieter validation.
func ValidateJSON(data []byte, opts ValidationOptions) (*OperationOutcome, error) {
	mode, err := ParseValidationMode(opts.Mode)
	if err != nil {
		return nil, err
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	var issues []OperationOutcomeIssue

	switch v := raw.(type) {
	case map[string]any:
		issues = append(issues, validateResourceOrBundle(v, mode, "")...)
	case []any:
		for i, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				issues = append(issues, issueError("structure", "array element is not an object", []string{fmt.Sprintf("[%d]", i)}))
				continue
			}
			issues = append(issues, validateResourceOrBundle(obj, mode, fmt.Sprintf("[%d]", i))...)
		}
	default:
		issues = append(issues, issueError("structure", "expected JSON object or array", nil))
	}

	return &OperationOutcome{
		ResourceType: "OperationOutcome",
		Issue:        issues,
	}, nil
}

func validateResourceOrBundle(obj map[string]any, mode ValidationMode, basePath string) []OperationOutcomeIssue {
	resourceType, ok := obj["resourceType"].(string)
	if !ok || resourceType == "" {
		return []OperationOutcomeIssue{issueError("required", "missing resourceType", at(basePath, "resourceType"))}
	}

	if resourceType == "Bundle" {
		return validateBundle(obj, mode, basePath)
	}
	return validateResource(obj, resourceType, mode, basePath)
}

func validateBundle(obj map[string]any, mode ValidationMode, basePath string) []OperationOutcomeIssue {
	var issues []OperationOutcomeIssue

	entryAny, has := obj["entry"]
	if !has {
		return []OperationOutcomeIssue{issueWarning("required", "Bundle.entry is missing", at(basePath, "entry"))}
	}

	entries, ok := entryAny.([]any)
	if !ok {
		return []OperationOutcomeIssue{issueError("structure", "Bundle.entry must be an array", at(basePath, "entry"))}
	}
	if len(entries) == 0 {
		issues = append(issues, issueWarning("required", "Bundle.entry is empty", at(basePath, "entry")))
		return issues
	}

	for i, entry := range entries {
		entryObj, ok := entry.(map[string]any)
		if !ok {
			issues = append(issues, issueError("structure", "Bundle.entry item must be an object", at(basePath, fmt.Sprintf("entry[%d]", i))))
			continue
		}
		resAny, ok := entryObj["resource"]
		if !ok {
			issues = append(issues, issueWarning("required", "Bundle.entry.resource is missing", at(basePath, fmt.Sprintf("entry[%d].resource", i))))
			continue
		}
		resObj, ok := resAny.(map[string]any)
		if !ok {
			issues = append(issues, issueError("structure", "Bundle.entry.resource must be an object", at(basePath, fmt.Sprintf("entry[%d].resource", i))))
			continue
		}
		issues = append(issues, validateResourceOrBundle(resObj, mode, atStr(basePath, fmt.Sprintf("entry[%d].resource", i)))...)
	}

	return issues
}

func validateResource(obj map[string]any, resourceType string, mode ValidationMode, basePath string) []OperationOutcomeIssue {
	var issues []OperationOutcomeIssue

	// ModeStructural is intentionally minimal/structural-only: for non-Bundle
	// resources, we accept unknown shapes as long as the payload is valid JSON
	// and has resourceType. Reaching here with any other value is impossible —
	// ValidateJSON rejects an unrecognised mode before parsing the payload.
	if mode != ModeUSCore {
		return nil
	}

	// US Core-ish expectations (kept intentionally small and evolving)
	switch resourceType {
	case "Patient":
		issues = append(issues, requireNonEmptyArray(obj, "identifier", "Patient.identifier is required (US Core)", basePath)...)
		issues = append(issues, requireNonEmptyArray(obj, "name", "Patient.name is required (US Core)", basePath)...)
		issues = append(issues, requireNonEmptyString(obj, "gender", "Patient.gender is required (US Core)", basePath)...)
		issues = append(issues, requireNonEmptyString(obj, "birthDate", "Patient.birthDate is required (US Core)", basePath)...)

	case "Encounter":
		issues = append(issues, requireNonEmptyString(obj, "status", "Encounter.status is required", basePath)...)
		issues = append(issues, requireNestedNonEmptyString(obj, []string{"class", "code"}, "Encounter.class.code is required", basePath)...)
		issues = append(issues, requireNestedNonEmptyString(obj, []string{"subject", "reference"}, "Encounter.subject.reference is required", basePath)...)

	case "Observation":
		issues = append(issues, requireNonEmptyString(obj, "status", "Observation.status is required", basePath)...)
		issues = append(issues, requireNestedNonEmptyString(obj, []string{"code", "text"}, "Observation.code.text is recommended when coding is absent", basePath)...)
		issues = append(issues, requireNestedNonEmptyString(obj, []string{"subject", "reference"}, "Observation.subject.reference is required", basePath)...)

	case "DiagnosticReport":
		issues = append(issues, requireNonEmptyString(obj, "status", "DiagnosticReport.status is required", basePath)...)
		issues = append(issues, requireNestedNonEmptyString(obj, []string{"code", "text"}, "DiagnosticReport.code.text is recommended when coding is absent", basePath)...)
		issues = append(issues, requireNestedNonEmptyString(obj, []string{"subject", "reference"}, "DiagnosticReport.subject.reference is required", basePath)...)

	case "Condition":
		issues = append(issues, requireNestedNonEmptyString(obj, []string{"subject", "reference"}, "Condition.subject.reference is required", basePath)...)

	case "Coverage":
		issues = append(issues, requireNonEmptyString(obj, "status", "Coverage.status is required", basePath)...)
		issues = append(issues, requireNestedNonEmptyString(obj, []string{"beneficiary", "reference"}, "Coverage.beneficiary.reference is required", basePath)...)
		issues = append(issues, requireNonEmptyArray(obj, "payor", "Coverage.payor is required", basePath)...)

	default:
		// Unknown resources are not an error for a generic validator.
	}

	issues = append(issues, validateUSCoreProfilePresence(obj, resourceType, basePath)...)

	return issues
}

func validateUSCoreProfilePresence(obj map[string]any, resourceType, basePath string) []OperationOutcomeIssue {
	expectedSet := expectedProfilesForResourceType(resourceType)
	if len(expectedSet) == 0 {
		return nil
	}

	meta, _ := obj["meta"].(map[string]any)
	profilesAny, ok := meta["profile"]
	if !ok {
		return []OperationOutcomeIssue{issueWarning("required", "meta.profile missing (US Core profile not declared)", at(basePath, "meta.profile"))}
	}
	profiles, ok := profilesAny.([]any)
	if !ok {
		return []OperationOutcomeIssue{issueWarning("structure", "meta.profile should be an array", at(basePath, "meta.profile"))}
	}

	for _, p := range profiles {
		s, _ := p.(string)
		if expectedSet[ProfileCanonical(s)] {
			return nil
		}
	}

	return []OperationOutcomeIssue{issueWarning("value", fmt.Sprintf("meta.profile does not include an expected profile for %s", resourceType), at(basePath, "meta.profile"))}
}

// ProfileCanonical reduces a FHIR canonical reference to its bare canonical URL
// by dropping any `|version` suffix and surrounding whitespace.
//
// This is the profile-version assertion policy, chosen in Slice 5.1a and
// recorded in `.loom/40-decisions.md` (2026-08-09) and
// `docs/operations/SUPPORTED-1.0.md`:
//
//	The mapper asserts BARE CANONICALS. The checker accepts a bare canonical or
//	any version-pinned form of it.
//
// FHIR R4 canonical references are `url[|version]` (FHIR R4 §2.24.1.3), so
// `…/us-core-patient` and `…/us-core-patient|9.0.0` denote the same profile with
// different version-resolution requirements. Before Slice 5.1a the comparison
// was a byte-exact map lookup against 31 unversioned constants, so *every*
// version-pinned resource — the form a conformant publisher is most likely to
// emit — failed the presence check.
//
// The alternative policy (pin all 31 constants to `|9.0.0` and require an exact
// match) was rejected: this package's checker has no package-resolution step, so
// a pinned constant would assert a version it cannot verify, and it would
// reject a correct bare canonical. Version *resolution* belongs to the real
// validator in Slice 5.1b, over the pinned offline `.tgz` IG packages.
func ProfileCanonical(profile string) string {
	trimmed := strings.TrimSpace(profile)
	if index := strings.Index(trimmed, "|"); index >= 0 {
		return strings.TrimSpace(trimmed[:index])
	}
	return trimmed
}

func expectedProfilesForResourceType(resourceType string) map[string]bool {
	switch resourceType {
	case "Patient":
		return map[string]bool{USCorePatientProfile: true}
	case "Encounter":
		return map[string]bool{USCoreEncounterProfile: true}
	case "Observation":
		return map[string]bool{
			USCoreObservationLabProfile:  true,
			USCoreVitalSignsProfile:      true,
			USCoreBloodPressureProfile:   true,
			USCoreBodyHeightProfile:      true,
			USCoreBodyWeightProfile:      true,
			USCoreBodyTemperatureProfile: true,
			USCoreHeartRateProfile:       true,
			USCoreRespiratoryRateProfile: true,
			USCorePulseOximetryProfile:   true,
			USCoreBMIProfile:             true,
		}
	case "Condition":
		return map[string]bool{USCoreConditionProfile: true}
	case "Coverage":
		return map[string]bool{USCoreCoverageProfile: true}
	case "Procedure":
		return map[string]bool{USCoreProcedureProfile: true}
	case "Immunization":
		return map[string]bool{USCoreImmunizationProfile: true}
	case "MedicationRequest":
		return map[string]bool{USCoreMedicationRequestProfile: true}
	case "AllergyIntolerance":
		return map[string]bool{USCoreAllergyIntoleranceProfile: true}
	case "CarePlan":
		return map[string]bool{USCoreCarePlanProfile: true}
	case "Goal":
		return map[string]bool{USCoreGoalProfile: true}
	case "CareTeam":
		return map[string]bool{USCoreCareTeamProfile: true}
	case "ServiceRequest":
		return map[string]bool{USCoreServiceRequestProfile: true}
	case "DiagnosticReport":
		return diagnosticReportProfiles()
	case "DocumentReference":
		return map[string]bool{USCoreDocumentReferenceProfile: true}
	case "Provenance":
		return map[string]bool{USCoreProvenanceProfile: true}
	case "Location":
		return map[string]bool{USCoreLocationProfile: true}
	case "Organization":
		return map[string]bool{USCoreOrganizationProfile: true}
	case "Practitioner":
		return map[string]bool{USCorePractitionerProfile: true}
	case "PractitionerRole":
		return map[string]bool{USCorePractitionerRoleProfile: true}
	case "RelatedPerson":
		return map[string]bool{USCoreRelatedPersonProfile: true}
	default:
		return nil
	}
}

func requireNonEmptyString(obj map[string]any, key, msg, basePath string) []OperationOutcomeIssue {
	v, ok := obj[key]
	if !ok {
		return []OperationOutcomeIssue{issueError("required", msg, at(basePath, key))}
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return []OperationOutcomeIssue{issueError("value", msg, at(basePath, key))}
	}
	return nil
}

func requireNestedNonEmptyString(obj map[string]any, keys []string, msg, basePath string) []OperationOutcomeIssue {
	v, ok := nestedValue(obj, keys)
	if !ok {
		// For some fields like code.text, treat as warning rather than error.
		if keys[len(keys)-1] == "text" {
			return []OperationOutcomeIssue{issueWarning("required", msg, at(basePath, joinPath(keys)))}
		}
		return []OperationOutcomeIssue{issueError("required", msg, at(basePath, joinPath(keys)))}
	}
	s, ok := v.(string)
	if !ok || s == "" {
		if keys[len(keys)-1] == "text" {
			return []OperationOutcomeIssue{issueWarning("value", msg, at(basePath, joinPath(keys)))}
		}
		return []OperationOutcomeIssue{issueError("value", msg, at(basePath, joinPath(keys)))}
	}
	return nil
}

func requireNonEmptyArray(obj map[string]any, key, msg, basePath string) []OperationOutcomeIssue {
	v, ok := obj[key]
	if !ok {
		return []OperationOutcomeIssue{issueError("required", msg, at(basePath, key))}
	}
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return []OperationOutcomeIssue{issueError("value", msg, at(basePath, key))}
	}
	return nil
}

func nestedValue(obj map[string]any, keys []string) (any, bool) {
	var cur any = obj
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[k]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func issueError(code, diagnostics string, location []string) OperationOutcomeIssue {
	return OperationOutcomeIssue{
		Severity:    "error",
		Code:        code,
		Diagnostics: diagnostics,
		Location:    location,
	}
}

func issueWarning(code, diagnostics string, location []string) OperationOutcomeIssue {
	return OperationOutcomeIssue{
		Severity:    "warning",
		Code:        code,
		Diagnostics: diagnostics,
		Location:    location,
	}
}

func at(basePath, field string) []string {
	if basePath == "" {
		return []string{field}
	}
	return []string{basePath + "." + field}
}

func atStr(basePath, field string) string {
	if basePath == "" {
		return field
	}
	return basePath + "." + field
}

func joinPath(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	out := keys[0]
	for i := 1; i < len(keys); i++ {
		out += "." + keys[i]
	}
	return out
}
