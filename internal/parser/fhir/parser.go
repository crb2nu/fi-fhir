package fhir

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cblevins/fi-fhir/pkg/events"
)

// Parser parses FHIR JSON resources into canonical events.
type Parser struct {
	source string
}

// NewParser creates a new FHIR parser.
func NewParser(source string) *Parser {
	return &Parser{source: source}
}

// ParseResult contains the results of parsing FHIR data.
type ParseResult struct {
	Events   []interface{}
	Warnings []ParseWarning
}

// ParseWarning represents a non-fatal issue during parsing.
type ParseWarning struct {
	Phase   string
	Code    string
	Message string
}

// ParseWithResult parses FHIR JSON and returns events with warnings.
func (p *Parser) ParseWithResult(data []byte) (*ParseResult, error) {
	result := &ParseResult{
		Events:   []interface{}{},
		Warnings: []ParseWarning{},
	}

	// Parse JSON to determine resource type
	var resource map[string]interface{}
	if err := json.Unmarshal(data, &resource); err != nil {
		return nil, fmt.Errorf("invalid FHIR JSON: %w", err)
	}

	resourceType, ok := resource["resourceType"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid resourceType")
	}

	switch resourceType {
	case "Bundle":
		entries, ok := resource["entry"].([]interface{})
		if !ok {
			result.Warnings = append(result.Warnings, ParseWarning{
				Phase:   "parsing",
				Code:    "EMPTY_BUNDLE",
				Message: "Bundle has no entries",
			})
			return result, nil
		}
		for _, entry := range entries {
			entryMap, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			entryResource, ok := entryMap["resource"].(map[string]interface{})
			if !ok {
				continue
			}
			evt := p.parseResource(entryResource)
			if evt != nil {
				result.Events = append(result.Events, evt)
			}
		}

	default:
		evt := p.parseResource(resource)
		if evt != nil {
			result.Events = append(result.Events, evt)
		}
	}

	return result, nil
}

// parseResource converts a FHIR resource to a canonical event.
func (p *Parser) parseResource(resource map[string]interface{}) interface{} {
	resourceType, _ := resource["resourceType"].(string)

	switch resourceType {
	case "Patient":
		return p.parsePatientResource(resource)

	case "Observation":
		return p.parseObservationResource(resource)

	case "Condition":
		return p.parseConditionResource(resource)

	case "Encounter":
		return p.parseEncounterResource(resource)

	case "Procedure":
		return p.parseProcedureResource(resource)

	case "Immunization":
		return p.parseImmunizationResource(resource)

	default:
		// Unknown resource type - return as document event
		return &events.DocumentEvent{
			EventMeta: events.EventMeta{
				ID:           extractString(resource, "id"),
				Type:         events.EventDocument,
				Timestamp:    time.Now(),
				Source:       p.source,
				SourceFormat: events.FormatFHIR,
			},
			DocumentType: resourceType,
		}
	}
}

func (p *Parser) parsePatientResource(resource map[string]interface{}) *events.DocumentEvent {
	patient := extractPatient(resource)
	return &events.DocumentEvent{
		EventMeta: events.EventMeta{
			ID:           extractString(resource, "id"),
			Type:         events.EventDocument,
			Timestamp:    time.Now(),
			Source:       p.source,
			SourceFormat: events.FormatFHIR,
		},
		Patient:      &patient,
		DocumentType: "Patient",
	}
}

func (p *Parser) parseObservationResource(resource map[string]interface{}) interface{} {
	patient := extractPatientRef(resource)

	// Determine if this is a lab result or vital sign based on category
	category := extractCategory(resource)

	code := extractCodeableConcept(resource, "code")
	value := extractValue(resource)

	if category == "vital-signs" {
		return &events.VitalSignEvent{
			EventMeta: events.EventMeta{
				ID:           extractString(resource, "id"),
				Type:         events.EventVitalSign,
				Timestamp:    extractTime(resource, "effectiveDateTime"),
				Source:       p.source,
				SourceFormat: events.FormatFHIR,
			},
			Patient: &patient,
			VitalSign: events.VitalSign{
				Name:      code.Display,
				LOINCCode: code.Code,
				Value:     value.Value,
				Unit:      value.Unit,
			},
		}
	}

	// Default to lab result
	return &events.LabResultEvent{
		EventMeta: events.EventMeta{
			ID:           extractString(resource, "id"),
			Type:         events.EventLabResult,
			Timestamp:    extractTime(resource, "effectiveDateTime"),
			Source:       p.source,
			SourceFormat: events.FormatFHIR,
		},
		Patient: patient,
		Test: events.LabTest{
			LOINCCode:   code.Code,
			Description: code.Display,
		},
		Result: events.LabValue{
			Value:  value.Value,
			Unit:   value.Unit,
			Status: extractString(resource, "status"),
		},
	}
}

func (p *Parser) parseConditionResource(resource map[string]interface{}) *events.ConditionEvent {
	patient := extractPatientRef(resource)
	code := extractCodeableConcept(resource, "code")

	return &events.ConditionEvent{
		EventMeta: events.EventMeta{
			ID:           extractString(resource, "id"),
			Type:         events.EventCondition,
			Timestamp:    extractTime(resource, "recordedDate"),
			Source:       p.source,
			SourceFormat: events.FormatFHIR,
		},
		Patient: &patient,
		Condition: events.Condition{
			Name:       code.Display,
			Code:       code.Code,
			CodeSystem: code.System,
		},
		ClinicalStatus: extractNestedString(resource, "clinicalStatus", "coding", "code"),
	}
}

func (p *Parser) parseEncounterResource(resource map[string]interface{}) *events.PatientAdmitEvent {
	patient := extractPatientRef(resource)

	return &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			ID:           extractString(resource, "id"),
			Type:         events.EventPatientAdmit,
			Timestamp:    extractTime(resource, "period.start"),
			Source:       p.source,
			SourceFormat: events.FormatFHIR,
		},
		Patient: patient,
		Encounter: events.Encounter{
			ID:     extractString(resource, "id"),
			Class:  extractNestedString(resource, "class", "code"),
			Status: extractString(resource, "status"),
		},
	}
}

func (p *Parser) parseProcedureResource(resource map[string]interface{}) *events.ProcedureEvent {
	patient := extractPatientRef(resource)
	code := extractCodeableConcept(resource, "code")

	return &events.ProcedureEvent{
		EventMeta: events.EventMeta{
			ID:           extractString(resource, "id"),
			Type:         events.EventProcedure,
			Timestamp:    extractTime(resource, "performedDateTime"),
			Source:       p.source,
			SourceFormat: events.FormatFHIR,
		},
		Patient: &patient,
		Procedure: events.Procedure{
			Name:       code.Display,
			Code:       code.Code,
			CodeSystem: code.System,
			Status:     extractString(resource, "status"),
		},
	}
}

func (p *Parser) parseImmunizationResource(resource map[string]interface{}) *events.ImmunizationEvent {
	patient := extractPatientRef(resource)
	vaccineCode := extractCodeableConcept(resource, "vaccineCode")

	return &events.ImmunizationEvent{
		EventMeta: events.EventMeta{
			ID:           extractString(resource, "id"),
			Type:         events.EventImmunization,
			Timestamp:    extractTime(resource, "occurrenceDateTime"),
			Source:       p.source,
			SourceFormat: events.FormatFHIR,
		},
		Patient: &patient,
		Immunization: events.Immunization{
			VaccineName: vaccineCode.Display,
			VaccineCode: vaccineCode.Code,
			Status:      extractString(resource, "status"),
		},
	}
}

// Helper types and functions

type codeValue struct {
	Code    string
	Display string
	System  string
}

type valueQuantity struct {
	Value string
	Unit  string
}

func extractString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func extractNestedString(m map[string]interface{}, keys ...string) string {
	current := m
	for i, key := range keys {
		if i == len(keys)-1 {
			return extractString(current, key)
		}
		if next, ok := current[key].(map[string]interface{}); ok {
			current = next
		} else if arr, ok := current[key].([]interface{}); ok && len(arr) > 0 {
			if next, ok := arr[0].(map[string]interface{}); ok {
				current = next
			} else {
				return ""
			}
		} else {
			return ""
		}
	}
	return ""
}

func extractTime(m map[string]interface{}, key string) time.Time {
	if v, ok := m[key].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
		if t, err := time.Parse("2006-01-02", v); err == nil {
			return t
		}
	}
	return time.Now()
}

func extractPatient(m map[string]interface{}) events.Patient {
	patient := events.Patient{
		MRN: extractString(m, "id"),
	}

	// Extract name
	if names, ok := m["name"].([]interface{}); ok && len(names) > 0 {
		if name, ok := names[0].(map[string]interface{}); ok {
			patient.FamilyName = extractString(name, "family")
			if given, ok := name["given"].([]interface{}); ok && len(given) > 0 {
				if g, ok := given[0].(string); ok {
					patient.GivenName = g
				}
			}
		}
	}

	// Extract gender
	patient.Gender = extractString(m, "gender")

	// Extract birth date
	if bd := extractString(m, "birthDate"); bd != "" {
		if t, err := time.Parse("2006-01-02", bd); err == nil {
			patient.DateOfBirth = t
		}
	}

	return patient
}

func extractPatientRef(m map[string]interface{}) events.Patient {
	// Extract patient reference
	if subject, ok := m["subject"].(map[string]interface{}); ok {
		ref := extractString(subject, "reference")
		// Reference format: "Patient/12345"
		if len(ref) > 8 && ref[:8] == "Patient/" {
			return events.Patient{MRN: ref[8:]}
		}
	}
	return events.Patient{}
}

func extractCodeableConcept(m map[string]interface{}, key string) codeValue {
	if cc, ok := m[key].(map[string]interface{}); ok {
		result := codeValue{
			Display: extractString(cc, "text"),
		}
		if codings, ok := cc["coding"].([]interface{}); ok && len(codings) > 0 {
			if coding, ok := codings[0].(map[string]interface{}); ok {
				result.Code = extractString(coding, "code")
				result.System = extractString(coding, "system")
				if result.Display == "" {
					result.Display = extractString(coding, "display")
				}
			}
		}
		return result
	}
	return codeValue{}
}

func extractValue(m map[string]interface{}) valueQuantity {
	if vq, ok := m["valueQuantity"].(map[string]interface{}); ok {
		val := ""
		if v, ok := vq["value"].(float64); ok {
			val = fmt.Sprintf("%g", v)
		} else if v, ok := vq["value"].(string); ok {
			val = v
		}
		return valueQuantity{
			Value: val,
			Unit:  extractString(vq, "unit"),
		}
	}
	if vs, ok := m["valueString"].(string); ok {
		return valueQuantity{Value: vs}
	}
	return valueQuantity{}
}

func extractCategory(m map[string]interface{}) string {
	if categories, ok := m["category"].([]interface{}); ok && len(categories) > 0 {
		if cat, ok := categories[0].(map[string]interface{}); ok {
			if codings, ok := cat["coding"].([]interface{}); ok && len(codings) > 0 {
				if coding, ok := codings[0].(map[string]interface{}); ok {
					return extractString(coding, "code")
				}
			}
		}
	}
	return ""
}
