package cda

import (
	"fmt"
	"strings"
	"time"

	"github.com/crb2nu/fi-fhir/pkg/events"
)

// Mapper converts CDA documents to canonical events.
type Mapper struct {
	source         string
	sectionMappers map[string]SectionMapper
}

// SectionMapper converts a CDA section to canonical events.
type SectionMapper interface {
	TemplateOID() string
	MapSection(section *Section, patient *events.Patient, docTime time.Time) ([]interface{}, error)
}

// MapperConfig configures the mapper behavior.
type MapperConfig struct {
	// Source identifier for provenance
	Source string

	// EmitDocumentEvents emits document-level events (patient_summary, etc.)
	EmitDocumentEvents bool

	// EmitSectionEvents emits per-entry events (lab_result, vital_sign, etc.)
	EmitSectionEvents bool
}

// NewMapper creates a new CDA to canonical mapper.
func NewMapper(config *MapperConfig) *Mapper {
	if config == nil {
		config = &MapperConfig{
			EmitDocumentEvents: true,
			EmitSectionEvents:  true,
		}
	}

	m := &Mapper{
		source:         config.Source,
		sectionMappers: make(map[string]SectionMapper),
	}

	// Register default section mappers
	m.registerDefaultMappers()

	return m
}

// RegisterSectionMapper adds a custom section mapper.
func (m *Mapper) RegisterSectionMapper(mapper SectionMapper) {
	m.sectionMappers[mapper.TemplateOID()] = mapper
}

// registerDefaultMappers adds built-in section mappers.
func (m *Mapper) registerDefaultMappers() {
	m.sectionMappers[TemplateSectionResults] = &ResultsSectionMapper{}
	m.sectionMappers[TemplateSectionVitalSigns] = &VitalSignsSectionMapper{}
	m.sectionMappers[TemplateSectionProblems] = &ProblemsSectionMapper{}
	m.sectionMappers[TemplateSectionProcedures] = &ProceduresSectionMapper{}
	m.sectionMappers[TemplateSectionImmunizations] = &ImmunizationsSectionMapper{}
}

// MapResult contains mapped events and any issues.
type MapResult struct {
	Events   []interface{}
	Patient  *events.Patient
	Warnings []string
}

// Map converts a CDA document to canonical events.
func (m *Mapper) Map(doc *CDADocument) (*MapResult, error) {
	result := &MapResult{
		Events:   []interface{}{},
		Warnings: []string{},
	}

	// Extract patient information
	patient := m.mapPatient(doc)
	result.Patient = patient

	// Map document-level event
	docEvent := m.mapDocumentEvent(doc, patient)
	if docEvent != nil {
		result.Events = append(result.Events, docEvent)
	}

	// Map section-level events
	for _, section := range doc.Sections {
		if mapper, ok := m.sectionMappers[section.TemplateID]; ok {
			sectionEvents, err := mapper.MapSection(&section, patient, doc.EffectiveTime)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("section %s: %v", section.TemplateID, err))
				continue
			}
			result.Events = append(result.Events, sectionEvents...)
		}
	}

	return result, nil
}

// mapPatient extracts patient information.
func (m *Mapper) mapPatient(doc *CDADocument) *events.Patient {
	if doc.Patient == nil {
		return nil
	}

	pr := doc.Patient
	patient := &events.Patient{}

	// Extract MRN
	for _, id := range pr.IDs {
		// MRN is typically the non-SSN identifier
		if id.Root != IdentifierSystemSSN {
			if id.Extension != "" {
				patient.MRN = id.Extension
				break
			}
		}
	}

	// Extract patient demographics
	if pr.Patient != nil {
		info := pr.Patient

		// Name
		if len(info.Names) > 0 {
			name := info.Names[0]
			patient.FamilyName = name.Family
			if len(name.Given) > 0 {
				patient.GivenName = name.Given[0]
				if len(name.Given) > 1 {
					patient.MiddleName = strings.Join(name.Given[1:], " ")
				}
			}
		}

		// Birth date
		if info.BirthTime != nil {
			patient.DateOfBirth = *info.BirthTime
		}

		// Gender
		switch info.Gender {
		case "M":
			patient.Gender = "male"
		case "F":
			patient.Gender = "female"
		case "UN":
			patient.Gender = "unknown"
		default:
			patient.Gender = info.Gender
		}
	}

	return patient
}

// mapDocumentEvent creates a document-level event.
func (m *Mapper) mapDocumentEvent(doc *CDADocument, patient *events.Patient) interface{} {
	// Determine event type from document template
	eventType := ""
	for _, tmpl := range doc.TemplateIDs {
		if et, ok := DocumentTypeToEvent[tmpl]; ok {
			eventType = et
			break
		}
	}

	if eventType == "" {
		return nil
	}

	baseEvent := events.EventMeta{
		ID:        doc.ID,
		Type:      events.EventType(eventType),
		Timestamp: doc.EffectiveTime,
		Source:    m.source,
	}

	switch eventType {
	case "patient_summary":
		return &events.DocumentEvent{
			EventMeta:    baseEvent,
			Patient:      patient,
			DocumentType: "CCD",
			Title:        doc.Title,
		}
	case "patient_discharge":
		evt := &events.PatientDischargeEvent{
			EventMeta: baseEvent,
		}
		if patient != nil {
			evt.Patient = *patient
		}
		return evt
	case "patient_transfer":
		return &events.DocumentEvent{
			EventMeta:    baseEvent,
			Patient:      patient,
			DocumentType: "Transfer Summary",
			Title:        doc.Title,
		}
	case "referral":
		return &events.DocumentEvent{
			EventMeta:    baseEvent,
			Patient:      patient,
			DocumentType: "Referral Note",
			Title:        doc.Title,
		}
	default:
		return &events.DocumentEvent{
			EventMeta:    baseEvent,
			Patient:      patient,
			DocumentType: eventType,
			Title:        doc.Title,
		}
	}
}

// ResultsSectionMapper maps Results section to lab events.
type ResultsSectionMapper struct{}

func (m *ResultsSectionMapper) TemplateOID() string {
	return TemplateSectionResults
}

func (m *ResultsSectionMapper) MapSection(section *Section, patient *events.Patient, docTime time.Time) ([]interface{}, error) {
	var results []interface{}

	for _, entry := range section.Entries {
		// Results come as organizers containing observations
		if entry.TypeCode == "organizer" {
			for _, obs := range entry.EntryRelationships {
				if obs.TypeCode == "observation" {
					event := mapLabResultObservation(&obs, patient, docTime)
					if event != nil {
						results = append(results, event)
					}
				}
			}
		} else if entry.TypeCode == "observation" {
			event := mapLabResultObservation(&entry, patient, docTime)
			if event != nil {
				results = append(results, event)
			}
		}
	}

	return results, nil
}

func mapLabResultObservation(obs *Entry, patient *events.Patient, docTime time.Time) *events.LabResultEvent {
	event := &events.LabResultEvent{
		EventMeta: events.EventMeta{
			ID:           obs.ID,
			Type:         events.EventLabResult,
			Timestamp:    docTime,
			SourceFormat: events.FormatCDA,
		},
		Test: events.LabTest{
			Description: obs.Code.DisplayName,
		},
		Result: events.LabValue{
			Status: mapStatusCode(obs.StatusCode),
		},
	}
	if patient != nil {
		event.Patient = *patient
	}

	// Set LOINC code
	if obs.Code.CodeSystem == CodeSystemLOINC {
		event.Test.LOINCCode = obs.Code.Code
	}

	// Set effective time
	if obs.EffectiveTime != nil && obs.EffectiveTime.Value != nil {
		event.Timestamp = *obs.EffectiveTime.Value
	}

	// Set result value
	if obs.Value != nil {
		switch obs.Value.Type {
		case "PQ":
			event.Result.Value = obs.Value.Value
			event.Result.Unit = obs.Value.Unit
		case "CD", "CE", "CV":
			event.Result.Value = obs.Value.DisplayName
			if event.Result.Value == "" {
				event.Result.Value = obs.Value.Code
			}
		case "IVL_PQ":
			if obs.Value.Low != "" && obs.Value.High != "" {
				event.Result.Value = fmt.Sprintf("%s-%s", obs.Value.Low, obs.Value.High)
			} else if obs.Value.Low != "" {
				event.Result.Value = ">= " + obs.Value.Low
			} else if obs.Value.High != "" {
				event.Result.Value = "<= " + obs.Value.High
			}
			event.Result.Unit = obs.Value.Unit
		default:
			event.Result.Value = obs.Value.Value
		}
	}

	return event
}

// VitalSignsSectionMapper maps Vital Signs section to vital sign events.
type VitalSignsSectionMapper struct{}

func (m *VitalSignsSectionMapper) TemplateOID() string {
	return TemplateSectionVitalSigns
}

func (m *VitalSignsSectionMapper) MapSection(section *Section, patient *events.Patient, docTime time.Time) ([]interface{}, error) {
	var results []interface{}

	for _, entry := range section.Entries {
		// Vital signs come as organizers containing observations
		if entry.TypeCode == "organizer" {
			// Get the organizer time
			orgTime := docTime
			if entry.EffectiveTime != nil && entry.EffectiveTime.Value != nil {
				orgTime = *entry.EffectiveTime.Value
			}

			for _, obs := range entry.EntryRelationships {
				if obs.TypeCode == "observation" {
					event := mapVitalSignObservation(&obs, patient, orgTime)
					if event != nil {
						results = append(results, event)
					}
				}
			}
		}
	}

	return results, nil
}

func mapVitalSignObservation(obs *Entry, patient *events.Patient, orgTime time.Time) *events.VitalSignEvent {
	if obs.Value == nil {
		return nil
	}

	event := &events.VitalSignEvent{
		EventMeta: events.EventMeta{
			ID:        obs.ID,
			Type:      events.EventVitalSign,
			Timestamp: orgTime,
		},
		Patient: patient,
		VitalSign: events.VitalSign{
			Name: obs.Code.DisplayName,
			Unit: obs.Value.Unit,
		},
	}

	// Set LOINC code
	if obs.Code.CodeSystem == CodeSystemLOINC {
		event.VitalSign.LOINCCode = obs.Code.Code
	}

	// Set effective time if different from organizer
	if obs.EffectiveTime != nil && obs.EffectiveTime.Value != nil {
		event.Timestamp = *obs.EffectiveTime.Value
	}

	// Set value
	if obs.Value.Type == "PQ" {
		event.VitalSign.Value = obs.Value.Value
		event.VitalSign.Unit = obs.Value.Unit
	}

	return event
}

// ProblemsSectionMapper maps Problems section to condition events.
type ProblemsSectionMapper struct{}

func (m *ProblemsSectionMapper) TemplateOID() string {
	return TemplateSectionProblems
}

func (m *ProblemsSectionMapper) MapSection(section *Section, patient *events.Patient, docTime time.Time) ([]interface{}, error) {
	var results []interface{}

	for _, entry := range section.Entries {
		// Problems are wrapped in Problem Concern Acts
		if entry.TypeCode == "act" {
			for _, obs := range entry.EntryRelationships {
				if obs.TypeCode == "observation" {
					event := mapConditionObservation(&obs, patient, docTime)
					if event != nil {
						// Set clinical status from act status
						if entry.StatusCode == "active" {
							event.ClinicalStatus = "active"
						} else if entry.StatusCode == "completed" {
							event.ClinicalStatus = "resolved"
						}
						results = append(results, event)
					}
				}
			}
		}
	}

	return results, nil
}

func mapConditionObservation(obs *Entry, patient *events.Patient, docTime time.Time) *events.ConditionEvent {
	if obs.Value == nil {
		return nil
	}

	event := &events.ConditionEvent{
		EventMeta: events.EventMeta{
			ID:        obs.ID,
			Type:      events.EventCondition,
			Timestamp: docTime,
		},
		Patient: patient,
		Condition: events.Condition{
			Name: obs.Value.DisplayName,
		},
	}

	// Set code
	if obs.Value.Code != "" {
		event.Condition.Code = obs.Value.Code
		if system, ok := OIDToFHIRSystem[obs.Value.CodeSystem]; ok {
			event.Condition.CodeSystem = system
		} else {
			event.Condition.CodeSystem = obs.Value.CodeSystem
		}
	}

	// Set onset date
	if obs.EffectiveTime != nil && obs.EffectiveTime.Low != nil {
		event.OnsetDate = obs.EffectiveTime.Low.Format("2006-01-02")
	}

	return event
}

// ProceduresSectionMapper maps Procedures section to procedure events.
type ProceduresSectionMapper struct{}

func (m *ProceduresSectionMapper) TemplateOID() string {
	return TemplateSectionProcedures
}

func (m *ProceduresSectionMapper) MapSection(section *Section, patient *events.Patient, docTime time.Time) ([]interface{}, error) {
	var results []interface{}

	for _, entry := range section.Entries {
		if entry.TypeCode == "procedure" {
			event := mapProcedureEntry(&entry, patient, docTime)
			if event != nil {
				results = append(results, event)
			}
		}
	}

	return results, nil
}

func mapProcedureEntry(entry *Entry, patient *events.Patient, docTime time.Time) *events.ProcedureEvent {
	event := &events.ProcedureEvent{
		EventMeta: events.EventMeta{
			ID:        entry.ID,
			Type:      events.EventProcedure,
			Timestamp: docTime,
		},
		Patient: patient,
		Procedure: events.Procedure{
			Name:   entry.Code.DisplayName,
			Code:   entry.Code.Code,
			Status: mapStatusCode(entry.StatusCode),
		},
	}

	// Set code system
	if system, ok := OIDToFHIRSystem[entry.Code.CodeSystem]; ok {
		event.Procedure.CodeSystem = system
	} else {
		event.Procedure.CodeSystem = entry.Code.CodeSystem
	}

	// Set performed date
	if entry.EffectiveTime != nil {
		if entry.EffectiveTime.Value != nil {
			event.PerformedDate = entry.EffectiveTime.Value.Format("2006-01-02")
		} else if entry.EffectiveTime.Low != nil {
			event.PerformedDate = entry.EffectiveTime.Low.Format("2006-01-02")
		}
	}

	return event
}

// ImmunizationsSectionMapper maps Immunizations section to immunization events.
type ImmunizationsSectionMapper struct{}

func (m *ImmunizationsSectionMapper) TemplateOID() string {
	return TemplateSectionImmunizations
}

func (m *ImmunizationsSectionMapper) MapSection(section *Section, patient *events.Patient, docTime time.Time) ([]interface{}, error) {
	var results []interface{}

	for _, entry := range section.Entries {
		if entry.TypeCode == "substanceAdministration" {
			event := mapImmunizationEntry(&entry, patient, docTime)
			if event != nil {
				results = append(results, event)
			}
		}
	}

	return results, nil
}

func mapImmunizationEntry(entry *Entry, patient *events.Patient, docTime time.Time) *events.ImmunizationEvent {
	event := &events.ImmunizationEvent{
		EventMeta: events.EventMeta{
			ID:        entry.ID,
			Type:      events.EventImmunization,
			Timestamp: docTime,
		},
		Patient: patient,
		Immunization: events.Immunization{
			Status: mapStatusCode(entry.StatusCode),
		},
	}

	// Vaccine code is in consumable/manufacturedProduct/manufacturedMaterial/code
	// For now, use the entry code if available
	if entry.Code.Code != "" {
		event.Immunization.VaccineCode = entry.Code.Code
		event.Immunization.VaccineName = entry.Code.DisplayName
	}

	// Set administered date
	if entry.EffectiveTime != nil {
		if entry.EffectiveTime.Value != nil {
			event.AdministeredDate = entry.EffectiveTime.Value.Format("2006-01-02")
		} else if entry.EffectiveTime.Low != nil {
			event.AdministeredDate = entry.EffectiveTime.Low.Format("2006-01-02")
		}
	}

	return event
}

// mapStatusCode converts CDA status codes to canonical status strings.
func mapStatusCode(code string) string {
	switch code {
	case "completed":
		return "completed"
	case "active":
		return "active"
	case "cancelled":
		return "cancelled"
	case "aborted":
		return "aborted"
	case "suspended":
		return "on-hold"
	case "held":
		return "on-hold"
	default:
		return code
	}
}
