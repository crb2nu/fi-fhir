package subscription

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/crb2nu/fi-fhir/internal/workflow"
	"github.com/crb2nu/fi-fhir/pkg/events"
)

// Package-level CEL evaluator for event mapping rules.
// Thread-safe with internal caching.
var (
	celEvaluator     *workflow.CELEvaluator
	celEvaluatorOnce sync.Once
	celEvaluatorErr  error
)

// getCELEvaluator returns the shared CEL evaluator, creating it if needed.
func getCELEvaluator() (*workflow.CELEvaluator, error) {
	celEvaluatorOnce.Do(func() {
		celEvaluator, celEvaluatorErr = workflow.NewCELEvaluator()
	})
	return celEvaluator, celEvaluatorErr
}

// FHIRMapper converts FHIR resources to canonical events.
type FHIRMapper struct {
	resourceMappers map[string]ResourceMapper
}

// ResourceMapper defines the interface for resource-specific mapping.
type ResourceMapper interface {
	Map(resource map[string]interface{}, action string, config *EventMappingConfig) (interface{}, error)
}

// NewFHIRMapper creates a new FHIR resource mapper with built-in mappers.
func NewFHIRMapper() *FHIRMapper {
	m := &FHIRMapper{
		resourceMappers: make(map[string]ResourceMapper),
	}

	// Register built-in mappers
	m.resourceMappers["Patient"] = &PatientMapper{}
	m.resourceMappers["Encounter"] = &EncounterMapper{}
	m.resourceMappers["Observation"] = &ObservationMapper{}
	m.resourceMappers["Appointment"] = &AppointmentMapper{}
	m.resourceMappers["DiagnosticReport"] = &DiagnosticReportMapper{}

	return m
}

// RegisterMapper adds a custom resource mapper.
func (m *FHIRMapper) RegisterMapper(resourceType string, mapper ResourceMapper) {
	m.resourceMappers[resourceType] = mapper
}

// MapResource converts a single FHIR resource to a canonical event.
func (m *FHIRMapper) MapResource(resource map[string]interface{}, action string, config *EventMappingConfig) (interface{}, error) {
	resourceType, _ := resource["resourceType"].(string)
	if resourceType == "" {
		return nil, fmt.Errorf("resource missing resourceType")
	}

	mapper, exists := m.resourceMappers[resourceType]
	if !exists {
		// No mapper for this resource type - not an error, just skip
		return nil, nil
	}

	return mapper.Map(resource, action, config)
}

// MapBundle converts a notification Bundle to canonical events.
func (m *FHIRMapper) MapBundle(bundle *NotificationBundle, config *EventMappingConfig) ([]interface{}, error) {
	var events []interface{}

	for _, entry := range bundle.Entry {
		action := "update"
		if entry.Request != nil {
			switch entry.Request.Method {
			case "POST":
				action = "create"
			case "PUT":
				action = "update"
			case "DELETE":
				action = "delete"
			}
		}

		event, err := m.MapResource(entry.Resource, action, config)
		if err != nil {
			return nil, err
		}

		if event != nil {
			events = append(events, event)
		}
	}

	return events, nil
}

// --- Built-in Resource Mappers ---

// PatientMapper maps FHIR Patient resources to canonical events.
type PatientMapper struct{}

func (p *PatientMapper) Map(resource map[string]interface{}, action string, config *EventMappingConfig) (interface{}, error) {
	// Determine event type based on action and config
	var eventType events.EventType
	switch action {
	case "create":
		eventType = events.EventType("patient_created")
		if config != nil && config.CreateEvent != "" {
			eventType = events.EventType(config.CreateEvent)
		}
	case "update":
		eventType = events.EventPatientUpdate
		if config != nil && config.UpdateEvent != "" {
			eventType = events.EventType(config.UpdateEvent)
		}
	case "delete":
		eventType = events.EventType("patient_deleted")
		if config != nil && config.DeleteEvent != "" {
			eventType = events.EventType(config.DeleteEvent)
		}
	default:
		return nil, nil
	}

	patient := mapFHIRPatient(resource)
	rawPayload, _ := json.Marshal(resource)

	meta := events.NewEventMeta(eventType, "fhir_subscription", events.FormatFHIR)
	if id, ok := resource["id"].(string); ok {
		meta.SourceMessageID = id
	}

	// Return a generic patient event structure
	return &PatientEvent{
		EventMeta:  meta,
		Patient:    patient,
		RawPayload: rawPayload,
	}, nil
}

// PatientEvent represents a patient lifecycle event from FHIR subscriptions.
type PatientEvent struct {
	events.EventMeta
	Patient    events.Patient  `json:"patient"`
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// EncounterMapper maps FHIR Encounter resources to canonical events.
type EncounterMapper struct{}

func (e *EncounterMapper) Map(resource map[string]interface{}, action string, config *EventMappingConfig) (interface{}, error) {
	if action == "delete" {
		return nil, nil // Encounter deletions typically not meaningful
	}

	// Determine event type based on encounter status and class
	status := getString(resource, "status")
	class := getNestedString(resource, "class", "code")

	var eventType events.EventType

	// Apply custom rules first using CEL expressions
	if config != nil && len(config.Rules) > 0 {
		evaluator, err := getCELEvaluator()
		if err == nil {
			for _, rule := range config.Rules {
				if rule.Condition == "" {
					// Empty condition always matches
					eventType = events.EventType(rule.EventType)
					break
				}
				// Evaluate CEL expression against the FHIR resource
				matched, evalErr := evaluator.Evaluate(rule.Condition, resource)
				if evalErr == nil && matched {
					eventType = events.EventType(rule.EventType)
					break
				}
			}
		}
	}

	// Default mapping based on status and class
	if eventType == "" {
		switch status {
		case "in-progress":
			if class == "IMP" || class == "ACUTE" || class == "inpatient" {
				eventType = events.EventPatientAdmit
			} else {
				eventType = events.EventPatientAdmit // Default to admit for any in-progress
			}
		case "finished", "completed":
			eventType = events.EventPatientDischarge
		case "cancelled":
			return nil, nil // Cancelled encounters typically not meaningful
		default:
			eventType = events.EventPatientUpdate
		}
	}

	patient := extractPatientFromReference(resource)
	encounter := mapFHIREncounter(resource)
	rawPayload, _ := json.Marshal(resource)

	meta := events.NewEventMeta(eventType, "fhir_subscription", events.FormatFHIR)
	if id, ok := resource["id"].(string); ok {
		meta.SourceMessageID = id
	}

	switch eventType {
	case events.EventPatientAdmit:
		return &events.PatientAdmitEvent{
			EventMeta:  meta,
			Patient:    patient,
			Encounter:  encounter,
			RawPayload: rawPayload,
		}, nil
	case events.EventPatientDischarge:
		return &events.PatientDischargeEvent{
			EventMeta:  meta,
			Patient:    patient,
			Encounter:  encounter,
			RawPayload: rawPayload,
		}, nil
	default:
		return &events.PatientAdmitEvent{
			EventMeta:  meta,
			Patient:    patient,
			Encounter:  encounter,
			RawPayload: rawPayload,
		}, nil
	}
}

// ObservationMapper maps FHIR Observation resources to canonical events.
type ObservationMapper struct{}

func (o *ObservationMapper) Map(resource map[string]interface{}, action string, config *EventMappingConfig) (interface{}, error) {
	if action == "delete" {
		return nil, nil
	}

	// Check category to determine if lab result
	categories := getArray(resource, "category")
	isLab := false
	for _, cat := range categories {
		if catMap, ok := cat.(map[string]interface{}); ok {
			codings := getArray(catMap, "coding")
			for _, c := range codings {
				if coding, ok := c.(map[string]interface{}); ok {
					code := getString(coding, "code")
					if code == "laboratory" || code == "vital-signs" {
						isLab = code == "laboratory"
						break
					}
				}
			}
		}
	}

	if !isLab {
		return nil, nil // Only map lab observations for now
	}

	eventType := events.EventLabResult
	if config != nil && config.CreateEvent != "" && action == "create" {
		eventType = events.EventType(config.CreateEvent)
	}

	patient := extractPatientFromReference(resource)
	test, result := mapFHIRObservation(resource)
	rawPayload, _ := json.Marshal(resource)

	meta := events.NewEventMeta(eventType, "fhir_subscription", events.FormatFHIR)
	if id, ok := resource["id"].(string); ok {
		meta.SourceMessageID = id
	}

	return &events.LabResultEvent{
		EventMeta:  meta,
		Patient:    patient,
		Test:       test,
		Result:     result,
		IsCritical: isCritical(result.Interpretation),
		RawPayload: rawPayload,
	}, nil
}

// AppointmentMapper maps FHIR Appointment resources to canonical events.
type AppointmentMapper struct{}

func (a *AppointmentMapper) Map(resource map[string]interface{}, action string, config *EventMappingConfig) (interface{}, error) {
	if action == "delete" {
		return nil, nil
	}

	status := getString(resource, "status")
	var eventType events.EventType

	switch status {
	case "booked", "pending", "proposed":
		eventType = events.EventAppointmentScheduled
	case "cancelled":
		eventType = events.EventAppointmentCancelled
	case "noshow":
		eventType = events.EventAppointmentNoShow
	case "checked-in", "arrived":
		eventType = events.EventAppointmentCheckedIn
	default:
		eventType = events.EventAppointmentModified
	}

	patient := extractPatientFromParticipant(resource)
	appointment := mapFHIRAppointment(resource)
	rawPayload, _ := json.Marshal(resource)

	meta := events.NewEventMeta(eventType, "fhir_subscription", events.FormatFHIR)
	if id, ok := resource["id"].(string); ok {
		meta.SourceMessageID = id
	}

	return &events.AppointmentEvent{
		EventMeta:   meta,
		Patient:     patient,
		Appointment: appointment,
		RawPayload:  rawPayload,
	}, nil
}

// DiagnosticReportMapper maps FHIR DiagnosticReport resources.
type DiagnosticReportMapper struct{}

func (d *DiagnosticReportMapper) Map(resource map[string]interface{}, action string, config *EventMappingConfig) (interface{}, error) {
	if action == "delete" {
		return nil, nil
	}

	// Map DiagnosticReport to LabResultEvent
	patient := extractPatientFromReference(resource)
	test, result := mapFHIRDiagnosticReport(resource)
	rawPayload, _ := json.Marshal(resource)

	meta := events.NewEventMeta(events.EventLabResult, "fhir_subscription", events.FormatFHIR)
	if id, ok := resource["id"].(string); ok {
		meta.SourceMessageID = id
	}

	return &events.LabResultEvent{
		EventMeta:  meta,
		Patient:    patient,
		Test:       test,
		Result:     result,
		IsCritical: isCritical(result.Interpretation),
		RawPayload: rawPayload,
	}, nil
}

// --- Helper Functions ---

func mapFHIRPatient(resource map[string]interface{}) events.Patient {
	patient := events.Patient{}

	// Map ID
	if id, ok := resource["id"].(string); ok {
		patient.Identifiers.Identifiers = append(patient.Identifiers.Identifiers, events.Identifier{
			Value:  id,
			Type:   "FHIR",
			System: "urn:ietf:rfc:3986",
		})
	}

	// Map identifiers
	if identifiers, ok := resource["identifier"].([]interface{}); ok {
		for _, ident := range identifiers {
			if identMap, ok := ident.(map[string]interface{}); ok {
				id := events.Identifier{
					Value:  getString(identMap, "value"),
					System: getString(identMap, "system"),
				}

				// Map type
				if typeMap, ok := identMap["type"].(map[string]interface{}); ok {
					if codings, ok := typeMap["coding"].([]interface{}); ok && len(codings) > 0 {
						if coding, ok := codings[0].(map[string]interface{}); ok {
							id.Type = getString(coding, "code")
						}
					}
				}

				if id.Value != "" {
					patient.Identifiers.Identifiers = append(patient.Identifiers.Identifiers, id)
				}
			}
		}
	}

	// Set MRN from identifiers
	if mrn := patient.Identifiers.GetByType("MR"); mrn != nil {
		patient.MRN = mrn.Value
	} else if len(patient.Identifiers.Identifiers) > 0 {
		patient.MRN = patient.Identifiers.Identifiers[0].Value
	}

	// Map name
	if names, ok := resource["name"].([]interface{}); ok && len(names) > 0 {
		if name, ok := names[0].(map[string]interface{}); ok {
			patient.FamilyName = getString(name, "family")
			if given, ok := name["given"].([]interface{}); ok && len(given) > 0 {
				patient.GivenName, _ = given[0].(string)
				if len(given) > 1 {
					patient.MiddleName, _ = given[1].(string)
				}
			}
			if prefix, ok := name["prefix"].([]interface{}); ok && len(prefix) > 0 {
				patient.Prefix, _ = prefix[0].(string)
			}
			if suffix, ok := name["suffix"].([]interface{}); ok && len(suffix) > 0 {
				patient.Suffix, _ = suffix[0].(string)
			}
		}
	}

	// Map birthDate
	if birthDate := getString(resource, "birthDate"); birthDate != "" {
		if t, err := time.Parse("2006-01-02", birthDate); err == nil {
			patient.DateOfBirth = t
		}
	}

	// Map gender
	patient.Gender = getString(resource, "gender")

	// Map address
	if addresses, ok := resource["address"].([]interface{}); ok && len(addresses) > 0 {
		if addr, ok := addresses[0].(map[string]interface{}); ok {
			patient.Address = mapFHIRAddress(addr)
		}
	}

	// Map phone
	if telecoms, ok := resource["telecom"].([]interface{}); ok {
		for _, t := range telecoms {
			if telecom, ok := t.(map[string]interface{}); ok {
				system := getString(telecom, "system")
				value := getString(telecom, "value")
				if system == "phone" && patient.Phone == "" {
					patient.Phone = value
				} else if system == "email" && patient.Email == "" {
					patient.Email = value
				}
			}
		}
	}

	return patient
}

func mapFHIRAddress(addr map[string]interface{}) events.Address {
	address := events.Address{
		City:       getString(addr, "city"),
		State:      getString(addr, "state"),
		PostalCode: getString(addr, "postalCode"),
		Country:    getString(addr, "country"),
		Type:       getString(addr, "use"),
	}

	if lines, ok := addr["line"].([]interface{}); ok {
		if len(lines) > 0 {
			address.Line1, _ = lines[0].(string)
		}
		if len(lines) > 1 {
			address.Line2, _ = lines[1].(string)
		}
	}

	return address
}

func mapFHIREncounter(resource map[string]interface{}) events.Encounter {
	encounter := events.Encounter{}

	// Map ID
	encounter.ID = getString(resource, "id")

	// Map class
	if class, ok := resource["class"].(map[string]interface{}); ok {
		encounter.Class = getString(class, "code")
	}

	// Map status
	encounter.Status = getString(resource, "status")

	// Map period
	if period, ok := resource["period"].(map[string]interface{}); ok {
		if start := getString(period, "start"); start != "" {
			if t, err := time.Parse(time.RFC3339, start); err == nil {
				encounter.AdmitDateTime = t
			}
		}
		if end := getString(period, "end"); end != "" {
			if t, err := time.Parse(time.RFC3339, end); err == nil {
				encounter.DischargeDateTime = t
			}
		}
	}

	// Map location
	if locations, ok := resource["location"].([]interface{}); ok && len(locations) > 0 {
		if loc, ok := locations[0].(map[string]interface{}); ok {
			if locRef, ok := loc["location"].(map[string]interface{}); ok {
				encounter.Location.Facility = getString(locRef, "display")
			}
		}
	}

	return encounter
}

func mapFHIRObservation(resource map[string]interface{}) (events.LabTest, events.LabValue) {
	test := events.LabTest{}
	result := events.LabValue{}

	// Map code
	if code, ok := resource["code"].(map[string]interface{}); ok {
		test.Code = mapCodeableConcept(code)
		test.Description = getString(code, "text")
		if len(test.Code.Coding) > 0 {
			for _, c := range test.Code.Coding {
				if strings.Contains(c.System, "loinc") {
					test.LOINCCode = c.Code
				} else {
					test.LocalCode = c.Code
				}
			}
		}
	}

	// Map value
	if valueQty, ok := resource["valueQuantity"].(map[string]interface{}); ok {
		if val, ok := valueQty["value"].(float64); ok {
			result.Value = fmt.Sprintf("%g", val)
		}
		result.Unit = getString(valueQty, "unit")
	} else if valueStr := getString(resource, "valueString"); valueStr != "" {
		result.Value = valueStr
	}

	// Map interpretation
	if interps, ok := resource["interpretation"].([]interface{}); ok && len(interps) > 0 {
		if interp, ok := interps[0].(map[string]interface{}); ok {
			if codings, ok := interp["coding"].([]interface{}); ok && len(codings) > 0 {
				if coding, ok := codings[0].(map[string]interface{}); ok {
					result.Interpretation = getString(coding, "code")
				}
			}
		}
	}

	// Map status
	result.Status = getString(resource, "status")

	// Map effectiveDateTime
	if effectiveDateTime := getString(resource, "effectiveDateTime"); effectiveDateTime != "" {
		if t, err := time.Parse(time.RFC3339, effectiveDateTime); err == nil {
			result.ObservationTime = t
		}
	}

	// Map referenceRange
	if ranges, ok := resource["referenceRange"].([]interface{}); ok && len(ranges) > 0 {
		if rng, ok := ranges[0].(map[string]interface{}); ok {
			low := ""
			high := ""
			if lowQty, ok := rng["low"].(map[string]interface{}); ok {
				if val, ok := lowQty["value"].(float64); ok {
					low = fmt.Sprintf("%g", val)
				}
			}
			if highQty, ok := rng["high"].(map[string]interface{}); ok {
				if val, ok := highQty["value"].(float64); ok {
					high = fmt.Sprintf("%g", val)
				}
			}
			if low != "" || high != "" {
				result.ReferenceRange = low + "-" + high
			}
		}
	}

	return test, result
}

func mapFHIRAppointment(resource map[string]interface{}) events.Appointment {
	appointment := events.Appointment{}

	appointment.ID = getString(resource, "id")
	appointment.Status = getString(resource, "status")

	// Map start/end
	if start := getString(resource, "start"); start != "" {
		if t, err := time.Parse(time.RFC3339, start); err == nil {
			appointment.StartTime = t
		}
	}
	if end := getString(resource, "end"); end != "" {
		if t, err := time.Parse(time.RFC3339, end); err == nil {
			appointment.EndTime = t
		}
	}

	// Map duration
	if duration, ok := resource["minutesDuration"].(float64); ok {
		appointment.Duration = int(duration)
	}

	// Map reason
	if reasons, ok := resource["reasonCode"].([]interface{}); ok && len(reasons) > 0 {
		if reason, ok := reasons[0].(map[string]interface{}); ok {
			appointment.Reason = getString(reason, "text")
		}
	}

	// Map appointment type
	if apptType, ok := resource["appointmentType"].(map[string]interface{}); ok {
		appointment.Type = getString(apptType, "text")
	}

	// Map cancellation reason
	if cancelReason, ok := resource["cancelationReason"].(map[string]interface{}); ok {
		appointment.CancellationReason = getString(cancelReason, "text")
	}

	return appointment
}

func mapFHIRDiagnosticReport(resource map[string]interface{}) (events.LabTest, events.LabValue) {
	test := events.LabTest{}
	result := events.LabValue{}

	// Map code
	if code, ok := resource["code"].(map[string]interface{}); ok {
		test.Code = mapCodeableConcept(code)
		test.Description = getString(code, "text")
	}

	// Map status
	result.Status = getString(resource, "status")

	// Map effectiveDateTime
	if effectiveDateTime := getString(resource, "effectiveDateTime"); effectiveDateTime != "" {
		if t, err := time.Parse(time.RFC3339, effectiveDateTime); err == nil {
			result.ObservationTime = t
		}
	}

	// Map conclusion
	result.Value = getString(resource, "conclusion")

	return test, result
}

func mapCodeableConcept(cc map[string]interface{}) events.CodeableConcept {
	concept := events.CodeableConcept{
		Text: getString(cc, "text"),
	}

	if codings, ok := cc["coding"].([]interface{}); ok {
		for _, c := range codings {
			if coding, ok := c.(map[string]interface{}); ok {
				concept.Coding = append(concept.Coding, events.Coding{
					System:  getString(coding, "system"),
					Code:    getString(coding, "code"),
					Display: getString(coding, "display"),
				})
			}
		}
	}

	return concept
}

func extractPatientFromReference(resource map[string]interface{}) events.Patient {
	patient := events.Patient{}

	if subject, ok := resource["subject"].(map[string]interface{}); ok {
		ref := getString(subject, "reference")
		// Extract patient ID from reference (e.g., "Patient/123" -> "123")
		if strings.HasPrefix(ref, "Patient/") {
			patient.MRN = strings.TrimPrefix(ref, "Patient/")
			patient.Identifiers.Identifiers = append(patient.Identifiers.Identifiers, events.Identifier{
				Value:  patient.MRN,
				Type:   "FHIR",
				System: "urn:ietf:rfc:3986",
			})
		}
		if display := getString(subject, "display"); display != "" {
			// Try to parse name from display
			parts := strings.SplitN(display, " ", 2)
			if len(parts) == 2 {
				patient.GivenName = parts[0]
				patient.FamilyName = parts[1]
			}
		}
	}

	return patient
}

func extractPatientFromParticipant(resource map[string]interface{}) events.Patient {
	patient := events.Patient{}

	if participants, ok := resource["participant"].([]interface{}); ok {
		for _, p := range participants {
			if part, ok := p.(map[string]interface{}); ok {
				if actor, ok := part["actor"].(map[string]interface{}); ok {
					ref := getString(actor, "reference")
					if strings.HasPrefix(ref, "Patient/") {
						patient.MRN = strings.TrimPrefix(ref, "Patient/")
						patient.Identifiers.Identifiers = append(patient.Identifiers.Identifiers, events.Identifier{
							Value:  patient.MRN,
							Type:   "FHIR",
							System: "urn:ietf:rfc:3986",
						})
						if display := getString(actor, "display"); display != "" {
							parts := strings.SplitN(display, " ", 2)
							if len(parts) == 2 {
								patient.GivenName = parts[0]
								patient.FamilyName = parts[1]
							}
						}
						break
					}
				}
			}
		}
	}

	return patient
}

func isCritical(interpretation string) bool {
	criticalCodes := map[string]bool{
		"critical": true,
		"HH":       true,
		"LL":       true,
		"AA":       true,
		"A":        true,
	}
	return criticalCodes[interpretation]
}

// Utility functions for safe type access

func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func getNestedString(m map[string]interface{}, keys ...string) string {
	current := m
	for i, key := range keys {
		if i == len(keys)-1 {
			return getString(current, key)
		}
		if nested, ok := current[key].(map[string]interface{}); ok {
			current = nested
		} else {
			return ""
		}
	}
	return ""
}

func getArray(m map[string]interface{}, key string) []interface{} {
	if val, ok := m[key].([]interface{}); ok {
		return val
	}
	return nil
}
