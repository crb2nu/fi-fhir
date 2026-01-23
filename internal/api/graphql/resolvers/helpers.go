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
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
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
