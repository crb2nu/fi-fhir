package resolvers

// Helper functions for resolver implementations.
// These are separated from schema.resolvers.go so gqlgen doesn't try to manage them.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/store"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/llm/explain"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/llm/extract"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/llm/quality"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/autoroute"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/workflow/v1"
)

// strPtr returns a pointer to the string, or nil if empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// escapeCSV properly escapes a string for CSV output.
// Fields containing commas, quotes, or newlines are wrapped in quotes.
func escapeCSV(s string) string {
	if s == "" {
		return ""
	}
	needsQuotes := strings.ContainsAny(s, ",\"\n\r")
	if needsQuotes {
		// Double any existing quotes and wrap in quotes
		escaped := strings.ReplaceAll(s, "\"", "\"\"")
		return "\"" + escaped + "\""
	}
	return s
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

// =============================================================================
// Profile Conversion Helpers
// =============================================================================

// strPtrEmpty returns a pointer to the string, or nil if empty.
// Unlike strPtr, this always returns a non-nil pointer if the string is non-empty.
func strPtrEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// convertStoreProfileToGraphQL converts a store Profile to a GraphQL SourceProfile.
func convertStoreProfileToGraphQL(p *store.Profile) (*model.SourceProfile, error) {
	if p == nil {
		return nil, nil
	}

	result := &model.SourceProfile{
		ID:        p.ID,
		Name:      p.Name,
		Version:   p.Version,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
		CreatedBy: strPtrEmpty(p.CreatedBy),
		IsActive:  p.IsActive,
	}

	// Parse config JSON
	if len(p.Config) > 0 && string(p.Config) != "{}" {
		var config store.ProfileConfig
		if err := json.Unmarshal(p.Config, &config); err == nil {
			// Convert HL7v2 config
			if config.HL7v2 != nil {
				result.Hl7v2 = convertHL7v2Config(config.HL7v2)
			}
			// Convert Identifier config
			if config.Identifiers != nil {
				result.Identifiers = convertIdentifierConfig(config.Identifiers)
			}
			// Convert Terminology config
			if config.Terminology != nil {
				result.Terminology = convertTerminologyConfig(config.Terminology)
			}
		}
	}

	return result, nil
}

func convertHL7v2Config(cfg *store.HL7v2Config) *model.HL7v2Config {
	if cfg == nil {
		return nil
	}

	result := &model.HL7v2Config{
		DefaultVersion:       cfg.DefaultVersion,
		Timezone:             cfg.Timezone,
		EventClassifications: []model.EventClassificationRule{},
	}

	if cfg.Tolerance != nil {
		result.Tolerance = &model.ToleranceConfig{
			MissingSegments:       cfg.Tolerance.MissingSegments,
			NteAnywhere:           cfg.Tolerance.NTEAnywhere,
			ExtraComponents:       cfg.Tolerance.ExtraComponents,
			UnknownSegments:       cfg.Tolerance.UnknownSegments,
			NonStandardDelimiters: cfg.Tolerance.NonStandardDelimiters,
		}
		if result.Tolerance.MissingSegments == nil {
			result.Tolerance.MissingSegments = []string{}
		}
	}

	for _, rule := range cfg.EventClassifications {
		result.EventClassifications = append(result.EventClassifications, model.EventClassificationRule{
			MessageType: rule.MessageType,
			Condition:   strPtrEmpty(rule.Condition),
			EventType:   rule.EventType,
			Priority:    rule.Priority,
		})
	}

	return result
}

func convertIdentifierConfig(cfg *store.IdentifierConfig) *model.IdentifierConfig {
	if cfg == nil {
		return nil
	}

	result := &model.IdentifierConfig{
		AssigningAuthorities: []model.AssigningAuthority{},
		PrimaryIDPreference:  []model.IDPreferenceRule{},
	}

	for _, aa := range cfg.AssigningAuthorities {
		result.AssigningAuthorities = append(result.AssigningAuthorities, model.AssigningAuthority{
			Code:   aa.Code,
			System: aa.System,
			Name:   strPtrEmpty(aa.Name),
		})
	}

	for _, pref := range cfg.PrimaryIDPreference {
		result.PrimaryIDPreference = append(result.PrimaryIDPreference, model.IDPreferenceRule{
			Type:             pref.Type,
			AssignerContains: strPtrEmpty(pref.AssignerContains),
			Priority:         pref.Priority,
		})
	}

	if cfg.Validation != nil {
		result.Validation = &model.ValidationSettingsConfig{}
		if cfg.Validation.NPI != nil {
			result.Validation.Npi = &model.ValidatorSetting{
				Enabled:   cfg.Validation.NPI.Enabled,
				OnInvalid: cfg.Validation.NPI.OnInvalid,
			}
		}
		if cfg.Validation.MBI != nil {
			result.Validation.Mbi = &model.ValidatorSetting{
				Enabled:   cfg.Validation.MBI.Enabled,
				OnInvalid: cfg.Validation.MBI.OnInvalid,
			}
		}
		if cfg.Validation.SSN != nil {
			result.Validation.Ssn = &model.ValidatorSetting{
				Enabled:   cfg.Validation.SSN.Enabled,
				OnInvalid: cfg.Validation.SSN.OnInvalid,
			}
		}
	}

	if cfg.Normalization != nil {
		rejectPatterns := cfg.Normalization.SSNRejectPatterns
		if rejectPatterns == nil {
			rejectPatterns = []string{}
		}
		result.Normalization = &model.NormalizationSettingsConfig{
			SsnStripDashes:    cfg.Normalization.SSNStripDashes,
			SsnRejectPatterns: rejectPatterns,
			PhoneNormalize:    cfg.Normalization.PhoneNormalize,
			PhoneFormat:       strPtrEmpty(cfg.Normalization.PhoneFormat),
		}
	}

	return result
}

func convertTerminologyConfig(cfg *store.TerminologyConfig) *model.TerminologyConfig {
	if cfg == nil {
		return nil
	}

	result := &model.TerminologyConfig{
		Mappings: []model.TerminologyMappingTable{},
	}

	for _, mapping := range cfg.Mappings {
		entries := make([]model.TerminologyMappingEntry, 0, len(mapping.Entries))
		for _, entry := range mapping.Entries {
			entries = append(entries, model.TerminologyMappingEntry{
				SourceCode: entry.SourceCode,
				TargetCode: entry.TargetCode,
				Display:    strPtrEmpty(entry.Display),
			})
		}
		result.Mappings = append(result.Mappings, model.TerminologyMappingTable{
			ID:           mapping.ID,
			SourceSystem: mapping.SourceSystem,
			TargetSystem: mapping.TargetSystem,
			Entries:      entries,
		})
	}

	return result
}

// convertGraphQLConfigToJSON converts GraphQL input types to JSON for storage.
func convertGraphQLConfigToJSON(input model.UpdateProfileInput) (json.RawMessage, error) {
	config := store.ProfileConfig{}

	if input.Hl7v2 != nil {
		config.HL7v2 = &store.HL7v2Config{}
		if input.Hl7v2.DefaultVersion != nil {
			config.HL7v2.DefaultVersion = *input.Hl7v2.DefaultVersion
		}
		if input.Hl7v2.Timezone != nil {
			config.HL7v2.Timezone = *input.Hl7v2.Timezone
		}
		if input.Hl7v2.Tolerance != nil {
			config.HL7v2.Tolerance = &store.ToleranceConfig{}
			if input.Hl7v2.Tolerance.MissingSegments != nil {
				config.HL7v2.Tolerance.MissingSegments = input.Hl7v2.Tolerance.MissingSegments
			}
			if input.Hl7v2.Tolerance.NteAnywhere != nil {
				config.HL7v2.Tolerance.NTEAnywhere = *input.Hl7v2.Tolerance.NteAnywhere
			}
			if input.Hl7v2.Tolerance.ExtraComponents != nil {
				config.HL7v2.Tolerance.ExtraComponents = *input.Hl7v2.Tolerance.ExtraComponents
			}
			if input.Hl7v2.Tolerance.UnknownSegments != nil {
				config.HL7v2.Tolerance.UnknownSegments = *input.Hl7v2.Tolerance.UnknownSegments
			}
			if input.Hl7v2.Tolerance.NonStandardDelimiters != nil {
				config.HL7v2.Tolerance.NonStandardDelimiters = *input.Hl7v2.Tolerance.NonStandardDelimiters
			}
		}
		if input.Hl7v2.EventClassifications != nil {
			for _, rule := range input.Hl7v2.EventClassifications {
				condition := ""
				if rule.Condition != nil {
					condition = *rule.Condition
				}
				config.HL7v2.EventClassifications = append(config.HL7v2.EventClassifications, store.EventClassRule{
					MessageType: rule.MessageType,
					Condition:   condition,
					EventType:   rule.EventType,
					Priority:    rule.Priority,
				})
			}
		}
	}

	if input.Identifiers != nil {
		config.Identifiers = &store.IdentifierConfig{}
		if input.Identifiers.AssigningAuthorities != nil {
			for _, aa := range input.Identifiers.AssigningAuthorities {
				name := ""
				if aa.Name != nil {
					name = *aa.Name
				}
				config.Identifiers.AssigningAuthorities = append(config.Identifiers.AssigningAuthorities, store.AssigningAuthority{
					Code:   aa.Code,
					System: aa.System,
					Name:   name,
				})
			}
		}
		if input.Identifiers.PrimaryIDPreference != nil {
			for _, pref := range input.Identifiers.PrimaryIDPreference {
				assigner := ""
				if pref.AssignerContains != nil {
					assigner = *pref.AssignerContains
				}
				config.Identifiers.PrimaryIDPreference = append(config.Identifiers.PrimaryIDPreference, store.IDPreferenceRule{
					Type:             pref.Type,
					AssignerContains: assigner,
					Priority:         pref.Priority,
				})
			}
		}
		if input.Identifiers.Validation != nil {
			config.Identifiers.Validation = &store.ValidationConfig{}
			if input.Identifiers.Validation.Npi != nil {
				config.Identifiers.Validation.NPI = &store.ValidatorSetting{
					Enabled:   input.Identifiers.Validation.Npi.Enabled,
					OnInvalid: input.Identifiers.Validation.Npi.OnInvalid,
				}
			}
			if input.Identifiers.Validation.Mbi != nil {
				config.Identifiers.Validation.MBI = &store.ValidatorSetting{
					Enabled:   input.Identifiers.Validation.Mbi.Enabled,
					OnInvalid: input.Identifiers.Validation.Mbi.OnInvalid,
				}
			}
			if input.Identifiers.Validation.Ssn != nil {
				config.Identifiers.Validation.SSN = &store.ValidatorSetting{
					Enabled:   input.Identifiers.Validation.Ssn.Enabled,
					OnInvalid: input.Identifiers.Validation.Ssn.OnInvalid,
				}
			}
		}
		if input.Identifiers.Normalization != nil {
			config.Identifiers.Normalization = &store.NormalizationConfig{}
			if input.Identifiers.Normalization.SsnStripDashes != nil {
				config.Identifiers.Normalization.SSNStripDashes = *input.Identifiers.Normalization.SsnStripDashes
			}
			if input.Identifiers.Normalization.SsnRejectPatterns != nil {
				config.Identifiers.Normalization.SSNRejectPatterns = input.Identifiers.Normalization.SsnRejectPatterns
			}
			if input.Identifiers.Normalization.PhoneNormalize != nil {
				config.Identifiers.Normalization.PhoneNormalize = *input.Identifiers.Normalization.PhoneNormalize
			}
			if input.Identifiers.Normalization.PhoneFormat != nil {
				config.Identifiers.Normalization.PhoneFormat = *input.Identifiers.Normalization.PhoneFormat
			}
		}
	}

	if input.Terminology != nil {
		config.Terminology = &store.TerminologyConfig{}
		if input.Terminology.Mappings != nil {
			for _, mapping := range input.Terminology.Mappings {
				entries := []store.TerminologyEntry{}
				if mapping.Entries != nil {
					for _, entry := range mapping.Entries {
						display := ""
						if entry.Display != nil {
							display = *entry.Display
						}
						entries = append(entries, store.TerminologyEntry{
							SourceCode: entry.SourceCode,
							TargetCode: entry.TargetCode,
							Display:    display,
						})
					}
				}
				config.Terminology.Mappings = append(config.Terminology.Mappings, store.TerminologyMapping{
					ID:           mapping.ID,
					SourceSystem: mapping.SourceSystem,
					TargetSystem: mapping.TargetSystem,
					Entries:      entries,
				})
			}
		}
	}

	return json.Marshal(config)
}

// incrementVersion increments a semantic version string (e.g., "1.0.0" -> "1.0.1").
func incrementVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "1.0.1"
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return "1.0.1"
	}
	return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1)
}

// =============================================================================
// LLM Warning Explainer Helpers
// =============================================================================

// derefStr dereferences a string pointer, returning empty string if nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseWarningForExplain is an intermediate type for passing warnings to the explainer.
type parseWarningForExplain struct {
	Phase    string
	Code     string
	Message  string
	Path     string
	Severity string
}

// convertToEventsSourceFormat converts GraphQL SourceFormat to events.SourceFormat.
func convertToEventsSourceFormat(format model.SourceFormat) events.SourceFormat {
	switch format {
	case model.SourceFormatHL7v2:
		return events.FormatHL7v2
	case model.SourceFormatFHIR:
		return events.FormatFHIR
	case model.SourceFormatCSV:
		return events.FormatCSV
	case model.SourceFormatEDI837:
		return events.FormatEDI837
	case model.SourceFormatEDI835:
		return events.FormatEDI835
	case model.SourceFormatCDA:
		return events.FormatCDA
	default:
		return events.FormatHL7v2
	}
}

// explainWarningsBatch calls the WarningExplainer to generate explanations for warnings.
func (r *queryResolver) explainWarningsBatch(ctx context.Context, warnings []parseWarningForExplain, format events.SourceFormat) ([]explain.ExplainedWarning, error) {
	// Convert to events.ParseWarning
	eventWarnings := make([]events.ParseWarning, len(warnings))
	for i, w := range warnings {
		eventWarnings[i] = events.ParseWarning{
			Phase:    w.Phase,
			Code:     w.Code,
			Message:  w.Message,
			Path:     w.Path,
			Severity: w.Severity,
		}
	}

	// Call the explainer batch method
	return r.WarningExplainer.ExplainBatch(ctx, eventWarnings, format)
}

// =============================================================================
// LLM Extraction Helpers
// =============================================================================

// derefBool dereferences a bool pointer, returning false if nil.
func derefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

// convertExtractionResult converts internal extraction result to GraphQL model.
func convertExtractionResult(result *extract.ExtractionResult, duration time.Duration) *model.ExtractionResult {
	if result == nil {
		return &model.ExtractionResult{
			Conditions:        []model.ExtractedCondition{},
			Medications:       []model.ExtractedMedication{},
			VitalSigns:        []model.ExtractedVitalSign{},
			Allergies:         []model.ExtractedAllergy{},
			Procedures:        []model.ExtractedProcedure{},
			OverallConfidence: 0,
			ProcessingTimeMs:  0,
		}
	}

	gqlResult := &model.ExtractionResult{
		Conditions:        make([]model.ExtractedCondition, 0, len(result.Conditions)),
		Medications:       make([]model.ExtractedMedication, 0, len(result.Medications)),
		VitalSigns:        make([]model.ExtractedVitalSign, 0, len(result.VitalSigns)),
		Allergies:         make([]model.ExtractedAllergy, 0, len(result.Allergies)),
		Procedures:        make([]model.ExtractedProcedure, 0, len(result.Procedures)),
		OverallConfidence: result.Confidence,
		ProcessingTimeMs:  int(duration.Milliseconds()),
		Model:             strPtrEmpty(result.Model),
	}

	// Convert conditions
	for _, c := range result.Conditions {
		gqlResult.Conditions = append(gqlResult.Conditions, model.ExtractedCondition{
			Name:       c.Name,
			Code:       strPtrEmpty(c.Code),
			CodeSystem: strPtrEmpty(c.CodeSystem),
			Confidence: c.Confidence,
			Negated:    boolPtr(c.Negated),
			TextSpan:   strPtrEmpty(c.TextSpan),
			Status:     strPtrEmpty(c.Status),
		})
	}

	// Convert medications
	for _, m := range result.Medications {
		gqlResult.Medications = append(gqlResult.Medications, model.ExtractedMedication{
			Name:       m.Name,
			Code:       strPtrEmpty(m.Code),
			CodeSystem: strPtrEmpty(m.CodeSystem),
			Dose:       strPtrEmpty(m.Dosage),
			Route:      strPtrEmpty(m.Route),
			Frequency:  strPtrEmpty(m.Frequency),
			Confidence: m.Confidence,
			Negated:    nil, // Not tracked in events.ExtractedMedication
			TextSpan:   strPtrEmpty(m.TextSpan),
		})
	}

	// Convert vital signs
	for _, v := range result.VitalSigns {
		gqlResult.VitalSigns = append(gqlResult.VitalSigns, model.ExtractedVitalSign{
			Name:           v.Name,
			LoincCode:      strPtrEmpty(v.LOINCCode),
			Value:          v.Value,
			Unit:           strPtrEmpty(v.Unit),
			Confidence:     v.Confidence,
			Interpretation: strPtrEmpty(v.Interpretation),
			TextSpan:       strPtrEmpty(v.TextSpan),
		})
	}

	// Convert allergies
	for _, a := range result.Allergies {
		gqlResult.Allergies = append(gqlResult.Allergies, model.ExtractedAllergy{
			Substance:  a.Substance,
			Code:       strPtrEmpty(a.Code),
			CodeSystem: strPtrEmpty(a.CodeSystem),
			Severity:   strPtrEmpty(a.Severity),
			Reaction:   strPtrEmpty(a.Reaction),
			Confidence: a.Confidence,
			Negated:    nil, // Not tracked in events.ExtractedAllergy
			TextSpan:   strPtrEmpty(a.TextSpan),
		})
	}

	// Convert procedures
	for _, p := range result.Procedures {
		gqlResult.Procedures = append(gqlResult.Procedures, model.ExtractedProcedure{
			Name:       p.Name,
			Code:       strPtrEmpty(p.Code),
			CodeSystem: strPtrEmpty(p.CodeSystem),
			Status:     strPtrEmpty(p.Status),
			Confidence: p.Confidence,
			Negated:    nil, // Not tracked in events.ExtractedProcedure
			TextSpan:   strPtrEmpty(p.TextSpan),
		})
	}

	return gqlResult
}

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}

// =============================================================================
// LLM Quality Helpers
// =============================================================================

// convertQualityScore converts internal quality score to GraphQL model.
func convertQualityScore(result *quality.DataQualityScore, duration time.Duration) *model.DataQualityScore {
	if result == nil {
		return &model.DataQualityScore{
			OverallScore: 0,
			Dimensions: &model.QualityDimensions{
				Completeness: 0,
				Accuracy:     0,
				Consistency:  0,
				Conformance:  0,
				Timeliness:   0,
			},
			Issues:          []model.DataQualityIssue{},
			Recommendations: []model.QualityRecommendation{},
		}
	}

	gqlResult := &model.DataQualityScore{
		OverallScore: result.OverallScore,
		Dimensions: &model.QualityDimensions{
			Completeness: result.Dimensions[quality.DimensionCompleteness],
			Accuracy:     result.Dimensions[quality.DimensionAccuracy],
			Consistency:  result.Dimensions[quality.DimensionConsistency],
			Conformance:  result.Dimensions[quality.DimensionConformance],
			Timeliness:   result.Dimensions[quality.DimensionTimeliness],
		},
		Issues:          make([]model.DataQualityIssue, 0, len(result.Issues)),
		Recommendations: make([]model.QualityRecommendation, 0, len(result.Recommendations)),
	}

	// Set processing time if duration is non-zero
	if duration > 0 {
		ms := int(duration.Milliseconds())
		gqlResult.ProcessingTimeMs = &ms
	}

	// Set model if available
	if result.Metadata.Model != "" {
		gqlResult.Model = strPtrEmpty(result.Metadata.Model)
	}

	// Convert issues
	for _, issue := range result.Issues {
		gqlResult.Issues = append(gqlResult.Issues, model.DataQualityIssue{
			Dimension:     issue.Dimension,
			Severity:      issue.Severity,
			Field:         strPtrEmpty(issue.Field),
			Description:   issue.Description,
			ActualValue:   strPtrEmpty(issue.ActualValue),
			ExpectedValue: strPtrEmpty(issue.ExpectedValue),
		})
	}

	// Convert recommendations
	for _, rec := range result.Recommendations {
		gqlResult.Recommendations = append(gqlResult.Recommendations, model.QualityRecommendation{
			Priority:    rec.Priority,
			Category:    strPtrEmpty(rec.Category),
			Title:       rec.Title,
			Description: rec.Description,
			Impact:      strPtrEmpty(rec.Impact),
		})
	}

	return gqlResult
}

// convertToEventsEventType converts GraphQL EventType to events.EventType.
func convertToEventsEventType(et model.EventType) events.EventType {
	switch et {
	case model.EventTypePatientAdmit:
		return events.EventPatientAdmit
	case model.EventTypePatientDischarge:
		return events.EventPatientDischarge
	case model.EventTypePatientTransfer:
		return events.EventPatientTransfer
	case model.EventTypePatientUpdate:
		return events.EventPatientUpdate
	case model.EventTypeLabResult:
		return events.EventLabResult
	case model.EventTypeLabOrdered:
		return events.EventLabOrdered
	case model.EventTypeVitalSign:
		return events.EventVitalSign
	case model.EventTypeCondition:
		return events.EventCondition
	case model.EventTypeProcedure:
		return events.EventProcedure
	case model.EventTypeImmunization:
		return events.EventImmunization
	case model.EventTypeAppointmentScheduled:
		return events.EventAppointmentScheduled
	case model.EventTypeAppointmentCancelled:
		return events.EventAppointmentCancelled
	case model.EventTypeAppointmentNoshow:
		return events.EventAppointmentNoShow
	case model.EventTypeClaimSubmitted:
		return events.EventClaimSubmitted
	case model.EventTypeClaimAdjudicated:
		return events.EventClaimAdjudicated
	case model.EventTypeDocument:
		return events.EventDocument
	default:
		return events.EventType(string(et))
	}
}

// =============================================================================
// LLM Workflow Explainer Helpers
// =============================================================================

// workflowExplanationResult is an intermediate type for workflow explanation conversion.
type workflowExplanationResult struct {
	Summary           string
	Description       string
	RouteExplanations []routeExplanationResult
	Diagram           string
	Warnings          []string
}

// routeExplanationResult is an intermediate type for route explanation conversion.
type routeExplanationResult struct {
	Name        string
	Trigger     string
	Actions     []string
	Description string
}

// convertExplainResult converts internal workflow explanation to intermediate type.
func convertExplainResult(result *explain.WorkflowExplanation) *workflowExplanationResult {
	if result == nil {
		return &workflowExplanationResult{
			RouteExplanations: []routeExplanationResult{},
			Warnings:          []string{},
		}
	}

	routes := make([]routeExplanationResult, 0, len(result.RouteExplanations))
	for _, r := range result.RouteExplanations {
		actions := r.Actions
		if actions == nil {
			actions = []string{}
		}
		routes = append(routes, routeExplanationResult{
			Name:        r.Name,
			Trigger:     r.Trigger,
			Actions:     actions,
			Description: r.Description,
		})
	}

	warnings := result.Warnings
	if warnings == nil {
		warnings = []string{}
	}

	return &workflowExplanationResult{
		Summary:           result.Summary,
		Description:       result.Description,
		RouteExplanations: routes,
		Diagram:           result.Diagram,
		Warnings:          warnings,
	}
}

// convertWorkflowExplanation converts intermediate type to GraphQL model.
func convertWorkflowExplanation(result *workflowExplanationResult) *model.WorkflowExplanation {
	if result == nil {
		return &model.WorkflowExplanation{
			Summary:           "",
			Description:       "",
			RouteExplanations: []model.RouteExplanation{},
			Warnings:          []string{},
		}
	}

	routes := make([]model.RouteExplanation, 0, len(result.RouteExplanations))
	for _, r := range result.RouteExplanations {
		actions := r.Actions
		if actions == nil {
			actions = []string{}
		}
		routes = append(routes, model.RouteExplanation{
			Name:        r.Name,
			Trigger:     r.Trigger,
			Actions:     actions,
			Description: r.Description,
		})
	}

	warnings := result.Warnings
	if warnings == nil {
		warnings = []string{}
	}

	var diagram *string
	if result.Diagram != "" {
		diagram = &result.Diagram
	}

	return &model.WorkflowExplanation{
		Summary:           result.Summary,
		Description:       result.Description,
		RouteExplanations: routes,
		Diagram:           diagram,
		Warnings:          warnings,
	}
}

// =============================================================================
// Message Classification Helpers
// =============================================================================

// eventTypePtr returns a pointer to an EventType value.
func eventTypePtr(et model.EventType) *model.EventType {
	return &et
}

// classifyHL7v2Message performs basic HL7v2 message classification.
func classifyHL7v2Message(data string) *model.MessageClassification {
	result := &model.MessageClassification{
		MessageType:   "UNKNOWN",
		SuggestedTags: []string{},
		Confidence:    0.5,
	}

	// Simple message type detection from MSH segment
	if len(data) < 20 {
		return result
	}

	// Look for MSH segment and extract message type
	lines := splitHL7Lines(data)
	for _, line := range lines {
		if len(line) > 3 && line[:3] == "MSH" {
			fields := splitHL7Fields(line)
			if len(fields) > 8 {
				msgType := fields[8] // MSH-9
				result.MessageType = msgType
				result.Confidence = 0.9

				// Classify based on message type
				switch {
				case contains(msgType, "ADT"):
					result.SuggestedTags = []string{"adt", "patient-movement"}
					if contains(msgType, "A01") {
						result.EventType = eventTypePtr(model.EventTypePatientAdmit)
						result.Summary = strPtrEmpty("Patient admission")
					} else if contains(msgType, "A03") {
						result.EventType = eventTypePtr(model.EventTypePatientDischarge)
						result.Summary = strPtrEmpty("Patient discharge")
					} else if contains(msgType, "A02") {
						result.EventType = eventTypePtr(model.EventTypePatientTransfer)
						result.Summary = strPtrEmpty("Patient transfer")
					}
				case contains(msgType, "ORU"):
					result.SuggestedTags = []string{"oru", "results", "lab"}
					result.EventType = eventTypePtr(model.EventTypeLabResult)
					result.Summary = strPtrEmpty("Lab/observation result")
				case contains(msgType, "ORM"):
					result.SuggestedTags = []string{"orm", "orders"}
					result.EventType = eventTypePtr(model.EventTypeLabOrdered)
					result.Summary = strPtrEmpty("Order message")
				case contains(msgType, "MDM"):
					result.SuggestedTags = []string{"mdm", "document", "clinical-note"}
					result.EventType = eventTypePtr(model.EventTypeDocument)
					result.Summary = strPtrEmpty("Medical document/clinical note")
				case contains(msgType, "SIU"):
					result.SuggestedTags = []string{"siu", "scheduling", "appointment"}
					result.EventType = eventTypePtr(model.EventTypeAppointmentScheduled)
					result.Summary = strPtrEmpty("Scheduling message")
				case contains(msgType, "DFT"):
					result.SuggestedTags = []string{"dft", "billing", "financial"}
					result.Summary = strPtrEmpty("Financial transaction")
				case contains(msgType, "RDE"):
					result.SuggestedTags = []string{"rde", "pharmacy", "medication"}
					result.Summary = strPtrEmpty("Pharmacy/medication message")
				}
				break
			}
		}
	}

	return result
}

// classifyFHIRMessage performs basic FHIR message classification.
func classifyFHIRMessage(data string) *model.MessageClassification {
	result := &model.MessageClassification{
		MessageType:   "FHIR",
		SuggestedTags: []string{"fhir"},
		Confidence:    0.7,
	}

	// Simple resource type detection
	if contains(data, "\"resourceType\"") {
		if contains(data, "\"Patient\"") {
			result.MessageType = "Patient"
			result.SuggestedTags = append(result.SuggestedTags, "patient")
			result.Summary = strPtrEmpty("FHIR Patient resource")
		} else if contains(data, "\"Observation\"") {
			result.MessageType = "Observation"
			result.SuggestedTags = append(result.SuggestedTags, "observation", "result")
			result.EventType = eventTypePtr(model.EventTypeLabResult)
			result.Summary = strPtrEmpty("FHIR Observation resource")
		} else if contains(data, "\"Encounter\"") {
			result.MessageType = "Encounter"
			result.SuggestedTags = append(result.SuggestedTags, "encounter")
			result.Summary = strPtrEmpty("FHIR Encounter resource")
		} else if contains(data, "\"Condition\"") {
			result.MessageType = "Condition"
			result.SuggestedTags = append(result.SuggestedTags, "condition", "diagnosis")
			result.EventType = eventTypePtr(model.EventTypeCondition)
			result.Summary = strPtrEmpty("FHIR Condition resource")
		} else if contains(data, "\"Procedure\"") {
			result.MessageType = "Procedure"
			result.SuggestedTags = append(result.SuggestedTags, "procedure")
			result.EventType = eventTypePtr(model.EventTypeProcedure)
			result.Summary = strPtrEmpty("FHIR Procedure resource")
		} else if contains(data, "\"MedicationRequest\"") {
			result.MessageType = "MedicationRequest"
			result.SuggestedTags = append(result.SuggestedTags, "medication", "pharmacy")
			result.Summary = strPtrEmpty("FHIR MedicationRequest resource")
		} else if contains(data, "\"DocumentReference\"") {
			result.MessageType = "DocumentReference"
			result.SuggestedTags = append(result.SuggestedTags, "document")
			result.EventType = eventTypePtr(model.EventTypeDocument)
			result.Summary = strPtrEmpty("FHIR DocumentReference resource")
		} else if contains(data, "\"Bundle\"") {
			result.MessageType = "Bundle"
			result.SuggestedTags = append(result.SuggestedTags, "bundle")
			result.Summary = strPtrEmpty("FHIR Bundle resource")
		}
		result.Confidence = 0.85
	}

	return result
}

// splitHL7Lines splits HL7 message into lines.
func splitHL7Lines(data string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\r' || data[i] == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

// splitHL7Fields splits an HL7 segment into fields.
func splitHL7Fields(segment string) []string {
	if len(segment) < 4 {
		return nil
	}
	// Get field separator (usually |)
	sep := segment[3:4]
	var fields []string
	start := 0
	for i := 0; i < len(segment); i++ {
		if string(segment[i]) == sep {
			fields = append(fields, segment[start:i])
			start = i + 1
		}
	}
	if start < len(segment) {
		fields = append(fields, segment[start:])
	}
	return fields
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr) >= 0))
}

// findSubstring finds a substring in a string.
func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// =============================================================================
// Terminology Mapping Helpers
// =============================================================================

// toGraphQLEquivalence converts db.MappingEquivalence to GraphQL model.MappingEquivalence.
func toGraphQLEquivalence(eq db.MappingEquivalence) model.MappingEquivalence {
	switch eq {
	case db.EquivalenceEquivalent:
		return model.MappingEquivalenceEquivalent
	case db.EquivalenceWider:
		return model.MappingEquivalenceWider
	case db.EquivalenceNarrower:
		return model.MappingEquivalenceNarrower
	case db.EquivalenceInexact:
		return model.MappingEquivalenceInexact
	default:
		return model.MappingEquivalenceEquivalent
	}
}

// toDBEquivalence converts GraphQL model.MappingEquivalence to db.MappingEquivalence.
func toDBEquivalence(eq model.MappingEquivalence) db.MappingEquivalence {
	switch eq {
	case model.MappingEquivalenceEquivalent:
		return db.EquivalenceEquivalent
	case model.MappingEquivalenceWider:
		return db.EquivalenceWider
	case model.MappingEquivalenceNarrower:
		return db.EquivalenceNarrower
	case model.MappingEquivalenceInexact:
		return db.EquivalenceInexact
	default:
		return db.EquivalenceEquivalent
	}
}

// toGraphQLOrigin converts db.MappingOrigin to GraphQL model.MappingOrigin.
func toGraphQLOrigin(origin db.MappingOrigin) model.MappingOrigin {
	switch origin {
	case db.OriginCSVUpload:
		return model.MappingOriginCSVUpload
	case db.OriginApprovedAutoroute:
		return model.MappingOriginApprovedAutoroute
	case db.OriginManual:
		return model.MappingOriginManual
	default:
		return model.MappingOriginManual
	}
}

// toDBOrigin converts GraphQL model.MappingOrigin to db.MappingOrigin.
func toDBOrigin(origin model.MappingOrigin) db.MappingOrigin {
	switch origin {
	case model.MappingOriginCSVUpload:
		return db.OriginCSVUpload
	case model.MappingOriginApprovedAutoroute:
		return db.OriginApprovedAutoroute
	case model.MappingOriginManual:
		return db.OriginManual
	default:
		return db.OriginManual
	}
}

// toGraphQLMapping converts db.CustomMapping to GraphQL model.CodeMapping.
func toGraphQLMapping(m *db.CustomMapping) *model.CodeMapping {
	if m == nil {
		return nil
	}

	result := &model.CodeMapping{
		ID:            fmt.Sprintf("%d", m.ID),
		SourceSystem:  m.SourceSystem,
		SourceCode:    m.SourceCode,
		SourceDisplay: strPtrEmpty(m.SourceDisplay),
		TargetSystem:  m.TargetSystem,
		TargetCode:    m.TargetCode,
		TargetDisplay: strPtrEmpty(m.TargetDisplay),
		Equivalence:   toGraphQLEquivalence(m.Equivalence),
		Comment:       strPtrEmpty(m.Comment),
		Origin:        toGraphQLOrigin(m.Origin),
		ProfileID:     strPtrEmpty(m.ProfileID),
		CreatedAt:     m.CreatedAt,
		CreatedBy:     strPtrEmpty(m.CreatedBy),
	}

	if m.Confidence != nil {
		result.Confidence = m.Confidence
	}

	if m.UploadBatchID != nil {
		batchID := m.UploadBatchID.String()
		result.UploadBatchID = &batchID
	}

	if m.ApprovedAt != nil {
		result.ApprovedAt = m.ApprovedAt
	}

	if m.ApprovedBy != "" {
		result.ApprovedBy = strPtrEmpty(m.ApprovedBy)
	}

	return result
}

// =============================================================================
// Autoroute Conversion Helpers
// =============================================================================

// classifyAutorouteDecision converts confidence to decision type.
func classifyAutorouteDecision(confidence float64) model.AutorouteDecision {
	if confidence >= 0.90 {
		return model.AutorouteDecisionAutorouteHighConf
	}
	if confidence >= 0.70 {
		return model.AutorouteDecisionAutorouteMedConf
	}
	if confidence >= 0.50 {
		return model.AutorouteDecisionAutorouteLowConf
	}
	return model.AutorouteDecisionNoMatch
}

// convertAutorouteCandidates converts autoroute result to GraphQL candidates.
func convertAutorouteCandidates(result *autoroute.SuggestResult) []model.MappingCandidate {
	if result == nil {
		return []model.MappingCandidate{}
	}

	candidates := make([]model.MappingCandidate, 0)

	// Add best match first
	if result.BestMatch != nil {
		candidates = append(candidates, model.MappingCandidate{
			Code:        result.BestMatch.Code,
			Display:     result.BestMatch.Display,
			System:      result.BestMatch.System,
			Confidence:  result.BestMatch.Confidence,
			Equivalence: toGraphQLEquivalencePtr(result.BestMatch.Equivalence),
			Reasoning:   strPtrEmpty(result.BestMatch.Reasoning),
			Score:       &result.BestMatch.Score,
		})
	}

	// Add alternates
	for _, alt := range result.Alternates {
		candidates = append(candidates, model.MappingCandidate{
			Code:       alt.Code,
			Display:    alt.Display,
			System:     alt.System,
			Confidence: alt.Confidence,
			Reasoning:  strPtrEmpty(alt.Reasoning),
			Score:      &alt.Score,
		})
	}

	return candidates
}

// toGraphQLEquivalencePtr converts db.MappingEquivalence to pointer.
func toGraphQLEquivalencePtr(eq db.MappingEquivalence) *model.MappingEquivalence {
	result := toGraphQLEquivalence(eq)
	return &result
}

// convertAutorouteTrace converts autoroute trace to GraphQL type.
func convertAutorouteTrace(trace *autoroute.DecisionTrace) *model.AutorouteTrace {
	if trace == nil {
		return nil
	}

	steps := make([]model.AutorouteStep, 0, len(trace.Steps))
	for _, s := range trace.Steps {
		var metadata map[string]interface{}
		if s.Metadata != nil {
			metadata = s.Metadata
		}
		steps = append(steps, model.AutorouteStep{
			Step:       s.Step,
			Result:     s.Result,
			DurationMs: int(s.DurationMs),
			Metadata:   metadata,
		})
	}

	return &model.AutorouteTrace{
		TraceID:         trace.TraceID,
		Timestamp:       trace.Timestamp,
		Steps:           steps,
		TotalDurationMs: int(trace.Duration.TotalMs),
	}
}

// toGraphQLBatch converts db.UploadBatch to GraphQL model.UploadBatch.
func toGraphQLBatch(b *db.UploadBatch) *model.UploadBatch {
	if b == nil {
		return nil
	}

	result := &model.UploadBatch{
		ID:               b.ID.String(),
		Filename:         b.Filename,
		SourceSystem:     strPtrEmpty(b.SourceSystem),
		TargetSystem:     strPtrEmpty(b.TargetSystem),
		ProfileID:        strPtrEmpty(b.ProfileID),
		TotalRows:        b.TotalRows,
		ValidRows:        b.ValidRows,
		DuplicateRows:    b.DuplicateRows,
		ErrorRows:        b.ErrorRows,
		UploadedAt:       b.UploadedAt,
		UploadedBy:       strPtrEmpty(b.UploadedBy),
		ValidationErrors: make([]model.UploadValidationError, 0, len(b.ValidationErrors)),
	}

	for _, e := range b.ValidationErrors {
		result.ValidationErrors = append(result.ValidationErrors, model.UploadValidationError{
			Row:     e.Row,
			Column:  strPtrEmpty(e.Column),
			Message: e.Message,
		})
	}

	return result
}

// =============================================================================
// Pending Autoroute Conversion Helpers
// =============================================================================

// toDBPendingStatus converts GraphQL PendingAutorouteStatus to db.PendingStatus.
func toDBPendingStatus(status model.PendingAutorouteStatus) db.PendingStatus {
	switch status {
	case model.PendingAutorouteStatusPending:
		return db.StatusPending
	case model.PendingAutorouteStatusApproved:
		return db.StatusApproved
	case model.PendingAutorouteStatusRejected:
		return db.StatusRejected
	case model.PendingAutorouteStatusExpired:
		return db.StatusExpired
	default:
		return db.StatusPending
	}
}

// toGraphQLPendingStatus converts db.PendingStatus to GraphQL PendingAutorouteStatus.
func toGraphQLPendingStatus(status db.PendingStatus) model.PendingAutorouteStatus {
	switch status {
	case db.StatusPending:
		return model.PendingAutorouteStatusPending
	case db.StatusApproved:
		return model.PendingAutorouteStatusApproved
	case db.StatusRejected:
		return model.PendingAutorouteStatusRejected
	case db.StatusExpired:
		return model.PendingAutorouteStatusExpired
	default:
		return model.PendingAutorouteStatusPending
	}
}

// toGraphQLPendingAutoroute converts db.PendingAutoroute to GraphQL model.PendingAutoroute.
func toGraphQLPendingAutoroute(p *db.PendingAutoroute) *model.PendingAutoroute {
	if p == nil {
		return nil
	}

	result := &model.PendingAutoroute{
		ID:               fmt.Sprintf("%d", p.ID),
		SourceSystem:     p.SourceSystem,
		SourceCode:       p.SourceCode,
		SourceDisplay:    strPtrEmpty(p.SourceDisplay),
		TargetSystem:     p.TargetSystem,
		SuggestedCode:    p.SuggestedCode,
		SuggestedDisplay: strPtrEmpty(p.SuggestedDisplay),
		Confidence:       p.Confidence,
		Reasoning:        strPtrEmpty(p.Reasoning),
		Status:           toGraphQLPendingStatus(p.Status),
		CreatedAt:        p.CreatedAt,
		Alternates:       []model.MappingCandidate{},
	}

	// Convert equivalence if present
	if p.Equivalence != "" {
		eq := toGraphQLEquivalence(db.MappingEquivalence(p.Equivalence))
		result.Equivalence = &eq
	}

	// Convert decision trace if present
	if len(p.DecisionTrace) > 0 {
		var trace autoroute.DecisionTrace
		if err := json.Unmarshal(p.DecisionTrace, &trace); err == nil {
			result.DecisionTrace = convertAutorouteTrace(&trace)
		}
	}

	// Convert alternates if present
	if len(p.Alternates) > 0 {
		var alts []db.Alternate
		if err := json.Unmarshal(p.Alternates, &alts); err == nil {
			for _, alt := range alts {
				result.Alternates = append(result.Alternates, model.MappingCandidate{
					Code:       alt.Code,
					Display:    alt.Display,
					System:     "", // Not stored in alternates
					Confidence: alt.Confidence,
					Reasoning:  strPtrEmpty(alt.Reasoning),
				})
			}
		}
	}

	// Optional timestamps
	result.ExpiresAt = p.ExpiresAt
	result.ReviewedAt = p.ReviewedAt
	result.ReviewedBy = strPtrEmpty(p.ReviewedBy)
	result.RejectionReason = strPtrEmpty(p.RejectionReason)

	return result
}

// =============================================================================
// Temporal Workflow Helpers
// =============================================================================

// toGraphQLTemporalWorkflow converts a Temporal workflow execution info to GraphQL model.
func toGraphQLTemporalWorkflow(info *workflow.WorkflowExecutionInfo) model.TemporalWorkflow {
	if info == nil {
		return model.TemporalWorkflow{}
	}

	result := model.TemporalWorkflow{
		ID:           info.Execution.WorkflowId,
		RunID:        info.Execution.RunId,
		WorkflowType: info.Type.Name,
		TaskQueue:    info.TaskQueue,
		StartTime:    info.StartTime.AsTime(),
	}

	// Convert status
	result.Status = toGraphQLTemporalStatus(info.Status)

	// Close time (if finished)
	if info.CloseTime != nil {
		closeTime := info.CloseTime.AsTime()
		result.CloseTime = &closeTime
		duration := int(closeTime.Sub(result.StartTime).Milliseconds())
		result.DurationMs = &duration
	}

	return result
}

// toGraphQLTemporalStatus converts Temporal workflow status to GraphQL enum.
func toGraphQLTemporalStatus(status enums.WorkflowExecutionStatus) model.TemporalWorkflowStatus {
	switch status {
	case enums.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return model.TemporalWorkflowStatusRunning
	case enums.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return model.TemporalWorkflowStatusCompleted
	case enums.WORKFLOW_EXECUTION_STATUS_FAILED:
		return model.TemporalWorkflowStatusFailed
	case enums.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return model.TemporalWorkflowStatusCanceled
	case enums.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return model.TemporalWorkflowStatusTerminated
	case enums.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return model.TemporalWorkflowStatusContinuedAsNew
	case enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return model.TemporalWorkflowStatusTimedOut
	default:
		return model.TemporalWorkflowStatusRunning
	}
}
