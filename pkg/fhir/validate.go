package fhir

import (
	"encoding/json"
	"fmt"
)

type ValidationOptions struct {
	// Mode controls which conformance checks to apply.
	// Supported values:
	//   - "none": structural validation only
	//   - "us-core": enforce US Core-ish expectations (profile presence is a warning)
	Mode string
}

// ValidateJSON validates a FHIR JSON payload that may contain:
//   - a single resource object
//   - an array of resources
//   - a Bundle with entry[].resource
//
// It returns an OperationOutcome containing errors/warnings. The caller decides
// whether warnings should fail the operation.
func ValidateJSON(data []byte, opts ValidationOptions) (*OperationOutcome, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	var issues []OperationOutcomeIssue

	switch v := raw.(type) {
	case map[string]any:
		issues = append(issues, validateResourceOrBundle(v, opts, "")...)
	case []any:
		for i, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				issues = append(issues, issueError("structure", "array element is not an object", []string{fmt.Sprintf("[%d]", i)}))
				continue
			}
			issues = append(issues, validateResourceOrBundle(obj, opts, fmt.Sprintf("[%d]", i))...)
		}
	default:
		issues = append(issues, issueError("structure", "expected JSON object or array", nil))
	}

	return &OperationOutcome{
		ResourceType: "OperationOutcome",
		Issue:        issues,
	}, nil
}

func validateResourceOrBundle(obj map[string]any, opts ValidationOptions, basePath string) []OperationOutcomeIssue {
	resourceType, ok := obj["resourceType"].(string)
	if !ok || resourceType == "" {
		return []OperationOutcomeIssue{issueError("required", "missing resourceType", at(basePath, "resourceType"))}
	}

	if resourceType == "Bundle" {
		return validateBundle(obj, opts, basePath)
	}
	return validateResource(obj, resourceType, opts, basePath)
}

func validateBundle(obj map[string]any, opts ValidationOptions, basePath string) []OperationOutcomeIssue {
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
		issues = append(issues, validateResourceOrBundle(resObj, opts, atStr(basePath, fmt.Sprintf("entry[%d].resource", i)))...)
	}

	return issues
}

func validateResource(obj map[string]any, resourceType string, opts ValidationOptions, basePath string) []OperationOutcomeIssue {
	var issues []OperationOutcomeIssue

	// "none" is intentionally minimal/structural-only: for non-Bundle resources, we
	// accept unknown shapes as long as the payload is valid JSON and has resourceType.
	if opts.Mode != "us-core" {
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
		if expectedSet[s] {
			return nil
		}
	}

	return []OperationOutcomeIssue{issueWarning("value", fmt.Sprintf("meta.profile does not include an expected profile for %s", resourceType), at(basePath, "meta.profile"))}
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
		return map[string]bool{USCoreDiagnosticReportNoteProfile: true}
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
