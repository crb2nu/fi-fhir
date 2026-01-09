package resolvers

// Helper functions for resolver implementations.
// These are separated from schema.resolvers.go so gqlgen doesn't try to manage them.

import (
	"fmt"
	"time"

	"github.com/cblevins/fi-fhir/internal/api/graphql/model"
	"github.com/cblevins/fi-fhir/pkg/events"
)

// strPtr returns a pointer to the string, or nil if empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// convertToGraphQLEvent converts internal event types to GraphQL model types.
func convertToGraphQLEvent(evt interface{}, source string, format model.SourceFormat, correlationID *string) model.Event {
	switch e := evt.(type) {
	case *events.PatientAdmitEvent:
		return &model.PatientAdmitEvent{
			BaseEventFields: model.BaseEventFields{
				ID:            e.ID,
				Type:          model.EventTypePatientAdmit,
				Timestamp:     e.Timestamp,
				Source:        source,
				SourceFormat:  &format,
				CorrelationID: correlationID,
			},
			Patient:   convertPatient(&e.Patient),
			Encounter: convertEncounter(&e.Encounter),
		}

	case *events.PatientDischargeEvent:
		return &model.PatientDischargeEvent{
			BaseEventFields: model.BaseEventFields{
				ID:            e.ID,
				Type:          model.EventTypePatientDischarge,
				Timestamp:     e.Timestamp,
				Source:        source,
				SourceFormat:  &format,
				CorrelationID: correlationID,
			},
			Patient:   convertPatient(&e.Patient),
			Encounter: convertEncounter(&e.Encounter),
		}

	case *events.LabResultEvent:
		return &model.LabResultEvent{
			BaseEventFields: model.BaseEventFields{
				ID:            e.ID,
				Type:          model.EventTypeLabResult,
				Timestamp:     e.Timestamp,
				Source:        source,
				SourceFormat:  &format,
				CorrelationID: correlationID,
			},
			Patient: convertPatient(&e.Patient),
			Test: model.LabTest{
				LoincCode:   strPtr(e.Test.LOINCCode),
				LocalCode:   strPtr(e.Test.LocalCode),
				Description: e.Test.Description,
				Category:    strPtr(e.Test.Category),
			},
			Result: model.LabResult{
				Value:          e.Result.Value,
				Unit:           strPtr(e.Result.Unit),
				ReferenceRange: strPtr(e.Result.ReferenceRange),
				Interpretation: strPtr(e.Result.Interpretation),
				Status:         strPtr(e.Result.Status),
			},
			IsCritical: e.IsCritical,
		}

	case *events.VitalSignEvent:
		return &model.VitalSignEvent{
			BaseEventFields: model.BaseEventFields{
				ID:            e.ID,
				Type:          model.EventTypeVitalSign,
				Timestamp:     e.Timestamp,
				Source:        source,
				SourceFormat:  &format,
				CorrelationID: correlationID,
			},
			Patient: convertPatientPtr(e.Patient),
			VitalSign: model.VitalSign{
				Name:      e.VitalSign.Name,
				LoincCode: strPtr(e.VitalSign.LOINCCode),
				Value:     e.VitalSign.Value,
				Unit:      strPtr(e.VitalSign.Unit),
			},
		}

	case *events.ConditionEvent:
		return &model.ConditionEvent{
			BaseEventFields: model.BaseEventFields{
				ID:            e.ID,
				Type:          model.EventTypeCondition,
				Timestamp:     e.Timestamp,
				Source:        source,
				SourceFormat:  &format,
				CorrelationID: correlationID,
			},
			Patient: convertPatientPtr(e.Patient),
			Condition: model.Condition{
				Name:       e.Condition.Name,
				Code:       strPtr(e.Condition.Code),
				CodeSystem: strPtr(e.Condition.CodeSystem),
				Category:   strPtr(e.Condition.Category),
			},
			ClinicalStatus: strPtr(e.ClinicalStatus),
			OnsetDate:      strPtr(e.OnsetDate),
		}

	case *events.ProcedureEvent:
		return &model.ProcedureEvent{
			BaseEventFields: model.BaseEventFields{
				ID:            e.ID,
				Type:          model.EventTypeProcedure,
				Timestamp:     e.Timestamp,
				Source:        source,
				SourceFormat:  &format,
				CorrelationID: correlationID,
			},
			Patient: convertPatientPtr(e.Patient),
			Procedure: model.Procedure{
				Name:       e.Procedure.Name,
				Code:       strPtr(e.Procedure.Code),
				CodeSystem: strPtr(e.Procedure.CodeSystem),
				Status:     strPtr(e.Procedure.Status),
			},
			PerformedDate: strPtr(e.PerformedDate),
		}

	case *events.ImmunizationEvent:
		return &model.ImmunizationEvent{
			BaseEventFields: model.BaseEventFields{
				ID:            e.ID,
				Type:          model.EventTypeImmunization,
				Timestamp:     e.Timestamp,
				Source:        source,
				SourceFormat:  &format,
				CorrelationID: correlationID,
			},
			Patient: convertPatientPtr(e.Patient),
			Immunization: model.Immunization{
				VaccineName: e.Immunization.VaccineName,
				VaccineCode: strPtr(e.Immunization.VaccineCode),
				Status:      strPtr(e.Immunization.Status),
			},
			AdministeredDate: strPtr(e.AdministeredDate),
		}

	case *events.AppointmentEvent:
		appt := model.Appointment{
			ID:        e.Appointment.ID,
			Status:    e.Appointment.Status,
			StartTime: e.Appointment.StartTime,
		}
		if !e.Appointment.EndTime.IsZero() {
			appt.EndTime = &e.Appointment.EndTime
		}
		return &model.AppointmentEvent{
			BaseEventFields: model.BaseEventFields{
				ID:            e.ID,
				Type:          model.EventTypeAppointmentScheduled,
				Timestamp:     e.Timestamp,
				Source:        source,
				SourceFormat:  &format,
				CorrelationID: correlationID,
			},
			Patient:     convertPatient(&e.Patient),
			Appointment: appt,
		}

	case *events.DocumentEvent:
		var patient *model.Patient
		if e.Patient != nil {
			p := convertPatient(e.Patient)
			patient = &p
		}
		return &model.DocumentEvent{
			BaseEventFields: model.BaseEventFields{
				ID:            e.ID,
				Type:          model.EventTypeDocument,
				Timestamp:     e.Timestamp,
				Source:        source,
				SourceFormat:  &format,
				CorrelationID: correlationID,
			},
			Patient:      patient,
			DocumentType: e.DocumentType,
			Title:        strPtr(e.Title),
		}
	}

	return nil
}

func convertPatient(p *events.Patient) model.Patient {
	if p == nil {
		return model.Patient{}
	}
	patient := model.Patient{
		MRN:         p.MRN,
		Identifiers: []model.Identifier{},
		FamilyName:  p.FamilyName,
		GivenName:   p.GivenName,
	}
	if p.MiddleName != "" {
		patient.MiddleName = &p.MiddleName
	}
	if !p.DateOfBirth.IsZero() {
		patient.DateOfBirth = &p.DateOfBirth
	}
	if p.Gender != "" {
		patient.Gender = &p.Gender
	}
	return patient
}

func convertPatientPtr(p *events.Patient) model.Patient {
	if p == nil {
		return model.Patient{}
	}
	return convertPatient(p)
}

func convertEncounter(e *events.Encounter) model.Encounter {
	if e == nil {
		return model.Encounter{}
	}
	encounter := model.Encounter{
		ID:    e.ID,
		Class: e.Class,
	}
	if e.Status != "" {
		encounter.Status = &e.Status
	}
	if !e.AdmitDateTime.IsZero() {
		encounter.AdmitDateTime = &e.AdmitDateTime
	}
	if !e.DischargeDateTime.IsZero() {
		encounter.DischargeDateTime = &e.DischargeDateTime
	}
	if e.Location.Facility != "" || e.Location.Unit != "" || e.Location.Room != "" || e.Location.Bed != "" {
		encounter.Location = &model.Location{
			Facility: strPtr(e.Location.Facility),
			Unit:     strPtr(e.Location.Unit),
			Room:     strPtr(e.Location.Room),
			Bed:      strPtr(e.Location.Bed),
		}
	}
	return encounter
}

func createEventFromInput(input model.SubmitEventInput) (model.Event, error) {
	base := model.BaseEventFields{
		ID:            fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Type:          input.Type,
		Timestamp:     time.Now(),
		Source:        input.Source,
		CorrelationID: input.CorrelationID,
	}

	// Create appropriate event type based on input.Type
	switch input.Type {
	case model.EventTypePatientAdmit:
		return &model.PatientAdmitEvent{
			BaseEventFields: base,
			Patient:         extractPatientFromData(input.Data),
			Encounter:       extractEncounterFromData(input.Data),
		}, nil

	case model.EventTypePatientDischarge:
		return &model.PatientDischargeEvent{
			BaseEventFields: base,
			Patient:         extractPatientFromData(input.Data),
			Encounter:       extractEncounterFromData(input.Data),
		}, nil

	case model.EventTypeLabResult:
		return &model.LabResultEvent{
			BaseEventFields: base,
			Patient:         extractPatientFromData(input.Data),
			Test:            extractLabTestFromData(input.Data),
			Result:          extractLabResultFromData(input.Data),
			IsCritical:      extractBoolFromData(input.Data, "isCritical"),
		}, nil

	case model.EventTypeVitalSign:
		return &model.VitalSignEvent{
			BaseEventFields: base,
			Patient:         extractPatientFromData(input.Data),
			VitalSign:       extractVitalSignFromData(input.Data),
		}, nil

	case model.EventTypeCondition:
		return &model.ConditionEvent{
			BaseEventFields: base,
			Patient:         extractPatientFromData(input.Data),
			Condition:       extractConditionFromData(input.Data),
			ClinicalStatus:  extractStringPtrFromData(input.Data, "clinicalStatus"),
			OnsetDate:       extractStringPtrFromData(input.Data, "onsetDate"),
		}, nil

	case model.EventTypeDocument:
		return &model.DocumentEvent{
			BaseEventFields: base,
			DocumentType:    extractStringFromData(input.Data, "documentType"),
			Title:           extractStringPtrFromData(input.Data, "title"),
		}, nil

	default:
		return nil, fmt.Errorf("unsupported event type: %s", input.Type)
	}
}

// Helper functions to extract data from map[string]interface{}
func extractPatientFromData(data map[string]interface{}) model.Patient {
	patientData, _ := data["patient"].(map[string]interface{})
	return model.Patient{
		MRN:         extractStringFromData(patientData, "mrn"),
		Identifiers: []model.Identifier{},
		FamilyName:  extractStringFromData(patientData, "familyName"),
		GivenName:   extractStringFromData(patientData, "givenName"),
		MiddleName:  extractStringPtrFromData(patientData, "middleName"),
		Gender:      extractStringPtrFromData(patientData, "gender"),
	}
}

func extractEncounterFromData(data map[string]interface{}) model.Encounter {
	encounterData, _ := data["encounter"].(map[string]interface{})
	return model.Encounter{
		ID:     extractStringFromData(encounterData, "id"),
		Class:  extractStringFromData(encounterData, "class"),
		Status: extractStringPtrFromData(encounterData, "status"),
	}
}

func extractLabTestFromData(data map[string]interface{}) model.LabTest {
	testData, _ := data["test"].(map[string]interface{})
	return model.LabTest{
		LoincCode:   extractStringPtrFromData(testData, "loincCode"),
		LocalCode:   extractStringPtrFromData(testData, "localCode"),
		Description: extractStringFromData(testData, "description"),
		Category:    extractStringPtrFromData(testData, "category"),
	}
}

func extractLabResultFromData(data map[string]interface{}) model.LabResult {
	resultData, _ := data["result"].(map[string]interface{})
	return model.LabResult{
		Value:          extractStringFromData(resultData, "value"),
		Unit:           extractStringPtrFromData(resultData, "unit"),
		ReferenceRange: extractStringPtrFromData(resultData, "referenceRange"),
		Interpretation: extractStringPtrFromData(resultData, "interpretation"),
		Status:         extractStringPtrFromData(resultData, "status"),
	}
}

func extractVitalSignFromData(data map[string]interface{}) model.VitalSign {
	vsData, _ := data["vitalSign"].(map[string]interface{})
	return model.VitalSign{
		Name:      extractStringFromData(vsData, "name"),
		LoincCode: extractStringPtrFromData(vsData, "loincCode"),
		Value:     extractStringFromData(vsData, "value"),
		Unit:      extractStringPtrFromData(vsData, "unit"),
	}
}

func extractConditionFromData(data map[string]interface{}) model.Condition {
	condData, _ := data["condition"].(map[string]interface{})
	return model.Condition{
		Name:       extractStringFromData(condData, "name"),
		Code:       extractStringPtrFromData(condData, "code"),
		CodeSystem: extractStringPtrFromData(condData, "codeSystem"),
		Category:   extractStringPtrFromData(condData, "category"),
	}
}

func extractStringFromData(data map[string]interface{}, key string) string {
	if data == nil {
		return ""
	}
	v, _ := data[key].(string)
	return v
}

func extractStringPtrFromData(data map[string]interface{}, key string) *string {
	if data == nil {
		return nil
	}
	v, ok := data[key].(string)
	if !ok || v == "" {
		return nil
	}
	return &v
}

func extractBoolFromData(data map[string]interface{}, key string) bool {
	if data == nil {
		return false
	}
	v, _ := data[key].(bool)
	return v
}
