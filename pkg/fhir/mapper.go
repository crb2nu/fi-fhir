package fhir

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// Mapper defines the interface for converting canonical events to FHIR resources.
type Mapper interface {
	// MapPatient converts a canonical Patient to a FHIR Patient.
	MapPatient(p *events.Patient) *Patient

	// MapEncounter converts a canonical Encounter to a FHIR Encounter.
	MapEncounter(e *events.Encounter, patientRef string) *Encounter

	// MapLabObservation converts a lab result to a FHIR Observation.
	MapLabObservation(lab *events.LabObservation, patientRef string) *Observation

	// MapLabResult converts a full lab result event to FHIR resources.
	// Returns a DiagnosticReport and associated Observations.
	MapLabResult(event *events.LabResultEvent) (*DiagnosticReport, []*Observation)

	// MapCondition converts a canonical ConditionEvent to a FHIR Condition.
	MapCondition(event *events.ConditionEvent, patientRef string) *Condition

	// MapCoverage converts a canonical EligibilityResponseEvent to a FHIR Coverage.
	MapCoverage(event *events.EligibilityResponseEvent, beneficiaryRef string) *Coverage

	// MapClaim converts a canonical ClaimSubmittedEvent to a FHIR Claim.
	// Use "claim" for billing claims or "preauthorization" for prior auth requests.
	MapClaim(event *events.ClaimSubmittedEvent, use string) *Claim

	// MapExplanationOfBenefit converts a ClaimAdjudicatedEvent to a FHIR ExplanationOfBenefit.
	MapExplanationOfBenefit(event *events.ClaimAdjudicatedEvent) *ExplanationOfBenefit

	// MapCoverageEligibilityResponse converts an EligibilityResponseEvent to a FHIR CoverageEligibilityResponse.
	MapCoverageEligibilityResponse(event *events.EligibilityResponseEvent, patientRef string) *CoverageEligibilityResponse

	// MapProcedure converts a canonical ProcedureEvent to a FHIR Procedure.
	MapProcedure(event *events.ProcedureEvent, patientRef string) *Procedure

	// MapImmunization converts a canonical ImmunizationEvent to a FHIR Immunization.
	MapImmunization(event *events.ImmunizationEvent, patientRef string) *Immunization

	// MapVitalSign converts a canonical VitalSignEvent to a FHIR Observation (Vital Signs).
	MapVitalSign(event *events.VitalSignEvent, patientRef string) *Observation

	// MapMedicationRequest converts a canonical MedicationRequestEvent to a FHIR MedicationRequest.
	MapMedicationRequest(event *events.MedicationRequestEvent, patientRef string) *MedicationRequest

	// MapAllergyIntolerance converts a canonical AllergyIntoleranceEvent to a FHIR AllergyIntolerance.
	MapAllergyIntolerance(event *events.AllergyIntoleranceEvent, patientRef string) *AllergyIntolerance

	// MapCarePlan converts a canonical CarePlanEvent to a FHIR CarePlan.
	MapCarePlan(event *events.CarePlanEvent, patientRef string) *CarePlan

	// MapGoal converts a canonical GoalEvent to a FHIR Goal.
	MapGoal(event *events.GoalEvent, patientRef string) *Goal

	// MapCareTeam converts a canonical CareTeamEvent to a FHIR CareTeam.
	MapCareTeam(event *events.CareTeamEvent, patientRef string) *CareTeam

	// MapServiceRequest converts a canonical ServiceRequestEvent to a FHIR ServiceRequest.
	MapServiceRequest(event *events.ServiceRequestEvent, patientRef string) *ServiceRequest

	// MapDocumentReference converts a canonical DocumentReferenceEvent to a FHIR DocumentReference.
	MapDocumentReference(event *events.DocumentReferenceEvent, patientRef string) *DocumentReference

	// MapDiagnosticReportNote converts a canonical DiagnosticReportNoteEvent to a FHIR DiagnosticReport.
	MapDiagnosticReportNote(event *events.DiagnosticReportNoteEvent, patientRef string) *DiagnosticReportNote

	// MapProvenance converts a canonical ProvenanceEvent to a US Core Provenance.
	MapProvenance(event *events.ProvenanceEvent) *Provenance

	// MapLocation converts a canonical FacilityLocationEvent to a US Core Location.
	MapLocation(event *events.FacilityLocationEvent) *FHIRLocation

	// MapOrganization converts a canonical OrganizationEvent to a US Core Organization.
	MapOrganization(event *events.OrganizationEvent) *FHIROrganization

	// MapPractitioner converts a canonical PractitionerEvent to a US Core Practitioner.
	MapPractitioner(event *events.PractitionerEvent) *FHIRPractitioner

	// MapPractitionerRole converts a canonical PractitionerRoleEvent to a US Core PractitionerRole.
	MapPractitionerRole(event *events.PractitionerRoleEvent) *FHIRPractitionerRole

	// MapRelatedPerson converts a canonical RelatedPersonEvent to a US Core RelatedPerson.
	MapRelatedPerson(event *events.RelatedPersonEvent, patientRef string) *FHIRRelatedPerson
}

// USCoreMapper implements Mapper for US Core 6.1.0 compliant resources.
type USCoreMapper struct {
	// BaseURL is used for generating resource references (e.g., "https://example.com/fhir")
	BaseURL string

	// IdentifierSystemMap maps local identifier type codes to system URIs
	IdentifierSystemMap map[string]string
}

// NewUSCoreMapper creates a new US Core compliant mapper.
func NewUSCoreMapper() *USCoreMapper {
	return &USCoreMapper{
		IdentifierSystemMap: map[string]string{
			"MR":  "http://hospital.example.org/mrn",
			"SS":  "http://hl7.org/fhir/sid/us-ssn",
			"SSN": "http://hl7.org/fhir/sid/us-ssn",
			"NPI": "http://hl7.org/fhir/sid/us-npi",
			"DL":  "urn:oid:2.16.840.1.113883.4.3.51", // State-specific
			"MBI": "http://hl7.org/fhir/sid/us-mbi",
		},
	}
}

// MapPatient converts a canonical Patient to a US Core Patient.
func (m *USCoreMapper) MapPatient(p *events.Patient) *Patient {
	if p == nil {
		return nil
	}

	patient := &Patient{
		ResourceType: "Patient",
		Meta: &Meta{
			Profile: []string{USCorePatientProfile},
		},
	}

	// Map identifiers. The IdentifierSet is authoritative, but Patient.MRN is a
	// documented convenience field (pkg/events/events.go Patient.MRN) and every
	// shipped parser is not the only possible producer. Before Slice 5.1a an
	// MRN-only Patient mapped to zero identifiers and the checker then raised a
	// hard `[error] Patient.identifier is required (US Core)` — the mapper
	// produced a resource its own validator rejects. Backfill instead.
	patient.Identifier = m.mapIdentifiers(&p.Identifiers)
	patient.Identifier = m.appendMRNIdentifier(patient.Identifier, p.MRN)

	// Map name
	name := HumanName{
		Use:    "official",
		Family: p.FamilyName,
	}
	if p.GivenName != "" {
		name.Given = append(name.Given, p.GivenName)
	}
	if p.MiddleName != "" {
		name.Given = append(name.Given, p.MiddleName)
	}
	if p.Prefix != "" {
		name.Prefix = []string{p.Prefix}
	}
	if p.Suffix != "" {
		name.Suffix = []string{p.Suffix}
	}
	patient.Name = []HumanName{name}

	// Map gender (US Core required)
	patient.Gender = m.mapGender(p.Gender)

	// Map birth date (US Core required)
	if !p.DateOfBirth.IsZero() {
		patient.BirthDate = p.DateOfBirth.Format("2006-01-02")
	}

	// Map addresses
	if p.Address.Line1 != "" || p.Address.City != "" {
		patient.Address = append(patient.Address, m.mapAddress(&p.Address))
	}
	for _, addr := range p.Addresses {
		patient.Address = append(patient.Address, m.mapAddress(&addr))
	}

	// Map telecom (phone, email)
	if p.Phone != "" {
		patient.Telecom = append(patient.Telecom, ContactPoint{
			System: "phone",
			Value:  p.Phone,
			Use:    "home",
		})
	}
	for _, phone := range p.Phones {
		patient.Telecom = append(patient.Telecom, ContactPoint{
			System: "phone",
			Value:  phone,
		})
	}
	if p.Email != "" {
		patient.Telecom = append(patient.Telecom, ContactPoint{
			System: "email",
			Value:  p.Email,
		})
	}

	// Map US Core extensions (race, ethnicity)
	patient.Extension = m.buildPatientExtensions(p)

	// Map language
	if p.Language != "" {
		patient.Communication = []PatientCommunication{
			{
				Language: &CodeableConcept{
					Text: p.Language,
				},
				Preferred: true,
			},
		}
	}

	// Map PCP
	if p.PrimaryCareProvider != nil && p.PrimaryCareProvider.NPI != "" {
		patient.GeneralPractitioner = []Reference{
			{
				Reference: fmt.Sprintf("Practitioner/%s", p.PrimaryCareProvider.NPI),
				Display:   formatProviderName(p.PrimaryCareProvider),
			},
		}
	}

	return patient
}

// MapEncounter converts a canonical Encounter to a US Core Encounter.
func (m *USCoreMapper) MapEncounter(e *events.Encounter, patientRef string) *Encounter {
	if e == nil {
		return nil
	}

	encounter := &Encounter{
		ResourceType: "Encounter",
		Meta: &Meta{
			Profile: []string{USCoreEncounterProfile},
		},
		Status: m.mapEncounterStatus(e.Status),
		Class:  m.mapEncounterClass(e.Class),
	}

	// Map identifiers
	encounter.Identifier = m.mapIdentifiers(&e.Identifiers)
	if e.ID != "" && len(encounter.Identifier) == 0 {
		encounter.Identifier = []Identifier{
			{Value: e.ID},
		}
	}

	// Map subject reference
	if patientRef != "" {
		encounter.Subject = &Reference{
			Reference: patientRef,
		}
	}

	// Map period
	if !e.AdmitDateTime.IsZero() || !e.DischargeDateTime.IsZero() {
		encounter.Period = &Period{}
		if !e.AdmitDateTime.IsZero() {
			t := e.AdmitDateTime
			encounter.Period.Start = &t
		}
		if !e.DischargeDateTime.IsZero() {
			t := e.DischargeDateTime
			encounter.Period.End = &t
		}
	}

	// Map participants (providers)
	if e.AttendingProvider != nil {
		encounter.Participant = append(encounter.Participant, EncounterParticipant{
			Type: []CodeableConcept{
				{
					Coding: []Coding{
						{System: "http://terminology.hl7.org/CodeSystem/v3-ParticipationType", Code: "ATND", Display: "attender"},
					},
				},
			},
			Individual: m.providerReference(e.AttendingProvider),
		})
	}
	if e.AdmittingProvider != nil {
		encounter.Participant = append(encounter.Participant, EncounterParticipant{
			Type: []CodeableConcept{
				{
					Coding: []Coding{
						{System: "http://terminology.hl7.org/CodeSystem/v3-ParticipationType", Code: "ADM", Display: "admitter"},
					},
				},
			},
			Individual: m.providerReference(e.AdmittingProvider),
		})
	}

	// Map hospitalization details
	if e.AdmitSource != "" || e.DischargeDisposition != "" {
		encounter.Hospitalization = &Hospitalization{}
		if e.AdmitSource != "" {
			encounter.Hospitalization.AdmitSource = &CodeableConcept{
				Text: e.AdmitSource,
			}
		}
		if e.DischargeDisposition != "" {
			encounter.Hospitalization.DischargeDisposition = &CodeableConcept{
				Text: e.DischargeDisposition,
			}
		}
	}

	// Map location
	if e.Location.Facility != "" || e.Location.Unit != "" {
		encounter.Location = []EncounterLocation{
			{
				Location: &Reference{
					Display: formatLocation(&e.Location),
				},
				Status: "active",
			},
		}
	}

	return encounter
}

// MapLabObservation converts a single lab observation to a FHIR Observation.
func (m *USCoreMapper) MapLabObservation(lab *events.LabObservation, patientRef string) *Observation {
	if lab == nil {
		return nil
	}

	obs := &Observation{
		ResourceType: "Observation",
		Meta: &Meta{
			Profile: []string{USCoreObservationLabProfile},
		},
		Status: m.mapObservationStatus(lab.Result.Status),
		Category: []CodeableConcept{
			{
				Coding: []Coding{
					{
						System:  SystemObservationCategory,
						Code:    "laboratory",
						Display: "Laboratory",
					},
				},
			},
		},
		Code: m.mapLabCode(&lab.Test),
	}

	// Set subject reference
	if patientRef != "" {
		obs.Subject = &Reference{
			Reference: patientRef,
		}
	}

	// Set effective time
	if !lab.Result.ObservationTime.IsZero() {
		obs.EffectiveDateTime = lab.Result.ObservationTime.Format(time.RFC3339)
	}

	// Set value (try numeric first, then string)
	if lab.Result.Value != "" {
		if numValue, err := strconv.ParseFloat(lab.Result.Value, 64); err == nil {
			obs.ValueQuantity = &Quantity{
				Value:  numValue,
				Unit:   lab.Result.Unit,
				System: SystemUCUM,
				Code:   lab.Result.Unit, // Ideally map to UCUM code
			}
		} else {
			obs.ValueString = lab.Result.Value
		}
	}

	// Set interpretation
	if lab.Result.Interpretation != "" {
		obs.Interpretation = []CodeableConcept{
			{
				Coding: []Coding{
					m.mapInterpretation(lab.Result.Interpretation),
				},
			},
		}
	}

	// Set reference range
	if lab.Result.ReferenceRange != "" {
		obs.ReferenceRange = []ReferenceRange{
			{Text: lab.Result.ReferenceRange},
		}
	}

	return obs
}

// MapLabResult converts a full lab result event to a DiagnosticReport and Observations.
func (m *USCoreMapper) MapLabResult(event *events.LabResultEvent) (*DiagnosticReport, []*Observation) {
	if event == nil {
		return nil, nil
	}

	patientRef := fmt.Sprintf("Patient/%s", event.Patient.MRN)

	// Create observations for all results
	var observations []*Observation
	var obsRefs []Reference

	// Handle multi-OBX case
	if len(event.Results) > 0 {
		for i, result := range event.Results {
			obs := m.MapLabObservation(&result, patientRef)
			if obs != nil {
				obs.ID = fmt.Sprintf("obs-%d", i+1)
				observations = append(observations, obs)
				obsRefs = append(obsRefs, Reference{
					Reference: fmt.Sprintf("Observation/%s", obs.ID),
				})
			}
		}
	} else {
		// Single OBX (legacy format)
		singleObs := events.LabObservation{
			Test:   event.Test,
			Result: event.Result,
		}
		obs := m.MapLabObservation(&singleObs, patientRef)
		if obs != nil {
			obs.ID = "obs-1"
			observations = append(observations, obs)
			obsRefs = append(obsRefs, Reference{
				Reference: fmt.Sprintf("Observation/%s", obs.ID),
			})
		}
	}

	// Create DiagnosticReport
	report := &DiagnosticReport{
		ResourceType: "DiagnosticReport",
		Meta: &Meta{
			Profile: []string{USCoreDiagnosticReportLabProfile},
		},
		Status: m.mapDiagnosticReportStatus(event.Result.Status),
		Category: []CodeableConcept{
			{
				Coding: []Coding{
					{
						System:  SystemObservationCategory,
						Code:    "LAB",
						Display: "Laboratory",
					},
				},
			},
		},
		Code: m.mapLabCode(&event.Test),
		Subject: &Reference{
			Reference: patientRef,
		},
		Result: obsRefs,
	}

	if !event.Result.ObservationTime.IsZero() {
		report.EffectiveDateTime = event.Result.ObservationTime.Format(time.RFC3339)
	}

	if event.OrderingProvider != nil {
		report.Performer = []Reference{
			*m.providerReference(event.OrderingProvider),
		}
	}

	return report, observations
}

// Helper methods

// appendMRNIdentifier backfills the canonical Patient.MRN convenience field as
// an `MR`-typed identifier when the IdentifierSet did not already carry it.
//
// It is deliberately value-based rather than type-based: a producer that
// populated Identifiers with the same MRN under any type has already expressed
// it, and a second copy would be a duplicate rather than a correction.
func (m *USCoreMapper) appendMRNIdentifier(existing []Identifier, mrn string) []Identifier {
	mrn = strings.TrimSpace(mrn)
	if mrn == "" {
		return existing
	}
	for _, identifier := range existing {
		if identifier.Value == mrn {
			return existing
		}
	}
	backfilled := Identifier{
		Use:    "usual",
		Value:  mrn,
		System: m.IdentifierSystemMap["MR"],
		Type: &CodeableConcept{
			Coding: []Coding{{System: SystemIdentifierType, Code: "MR"}},
		},
	}
	return append(existing, backfilled)
}

func (m *USCoreMapper) mapIdentifiers(ids *events.IdentifierSet) []Identifier {
	if ids == nil || len(ids.Identifiers) == 0 {
		return nil
	}

	var result []Identifier
	for _, id := range ids.Identifiers {
		fhirID := Identifier{
			Value: id.Value,
		}

		// Map system
		if id.System != "" {
			fhirID.System = id.System
		} else if system, ok := m.IdentifierSystemMap[id.Type]; ok {
			fhirID.System = system
		}

		// Map type
		if id.Type != "" {
			fhirID.Type = &CodeableConcept{
				Coding: []Coding{
					{
						System: SystemIdentifierType,
						Code:   id.Type,
					},
				},
			}
		}

		// Map use
		if id.Use != "" {
			fhirID.Use = id.Use
		}

		result = append(result, fhirID)
	}

	return result
}

func (m *USCoreMapper) mapGender(g string) string {
	switch strings.ToUpper(g) {
	case "M", "MALE":
		return "male"
	case "F", "FEMALE":
		return "female"
	case "O", "OTHER":
		return "other"
	default:
		return "unknown"
	}
}

func (m *USCoreMapper) mapAddress(addr *events.Address) Address {
	result := Address{
		City:       addr.City,
		State:      addr.State,
		PostalCode: addr.PostalCode,
		Country:    addr.Country,
	}

	if addr.Line1 != "" {
		result.Line = append(result.Line, addr.Line1)
	}
	if addr.Line2 != "" {
		result.Line = append(result.Line, addr.Line2)
	}

	switch strings.ToUpper(addr.Type) {
	case "HOME", "H":
		result.Use = "home"
	case "WORK", "W":
		result.Use = "work"
	case "TEMP", "T":
		result.Use = "temp"
	case "OLD", "O":
		result.Use = "old"
	}

	return result
}

func (m *USCoreMapper) buildPatientExtensions(p *events.Patient) []Extension {
	var extensions []Extension

	// US Core Race extension
	if p.Race != "" {
		raceExt := Extension{
			URL: USCoreRaceExtension,
			Extension: []Extension{
				{
					URL:         "text",
					ValueString: p.Race,
				},
			},
		}
		// Try to map race to OMB category
		if ombCode := m.mapRaceToOMB(p.Race); ombCode != nil {
			raceExt.Extension = append([]Extension{
				{
					URL:         "ombCategory",
					ValueCoding: ombCode,
				},
			}, raceExt.Extension...)
		}
		extensions = append(extensions, raceExt)
	}

	// US Core Ethnicity extension
	if p.Ethnicity != "" {
		ethExt := Extension{
			URL: USCoreEthnicityExtension,
			Extension: []Extension{
				{
					URL:         "text",
					ValueString: p.Ethnicity,
				},
			},
		}
		// Try to map ethnicity to OMB category
		if ombCode := m.mapEthnicityToOMB(p.Ethnicity); ombCode != nil {
			ethExt.Extension = append([]Extension{
				{
					URL:         "ombCategory",
					ValueCoding: ombCode,
				},
			}, ethExt.Extension...)
		}
		extensions = append(extensions, ethExt)
	}

	return extensions
}

func (m *USCoreMapper) mapRaceToOMB(race string) *Coding {
	// OMB race categories (CDC Race & Ethnicity codes)
	raceMap := map[string]struct{ code, display string }{
		"WHITE":            {"2106-3", "White"},
		"W":                {"2106-3", "White"},
		"BLACK":            {"2054-5", "Black or African American"},
		"AFRICAN AMERICAN": {"2054-5", "Black or African American"},
		"B":                {"2054-5", "Black or African American"},
		"ASIAN":            {"2028-9", "Asian"},
		"A":                {"2028-9", "Asian"},
		"NATIVE AMERICAN":  {"1002-5", "American Indian or Alaska Native"},
		"AMERICAN INDIAN":  {"1002-5", "American Indian or Alaska Native"},
		"PACIFIC ISLANDER": {"2076-8", "Native Hawaiian or Other Pacific Islander"},
		"HAWAIIAN":         {"2076-8", "Native Hawaiian or Other Pacific Islander"},
	}

	if info, ok := raceMap[strings.ToUpper(race)]; ok {
		return &Coding{
			System:  SystemCDCRaceEthnicity,
			Code:    info.code,
			Display: info.display,
		}
	}
	return nil
}

func (m *USCoreMapper) mapEthnicityToOMB(ethnicity string) *Coding {
	// OMB ethnicity categories
	switch strings.ToUpper(ethnicity) {
	case "HISPANIC", "LATINO", "H":
		return &Coding{
			System:  SystemCDCRaceEthnicity,
			Code:    "2135-2",
			Display: "Hispanic or Latino",
		}
	case "NOT HISPANIC", "NON-HISPANIC", "NH", "N":
		return &Coding{
			System:  SystemCDCRaceEthnicity,
			Code:    "2186-5",
			Display: "Not Hispanic or Latino",
		}
	}
	return nil
}

func (m *USCoreMapper) mapEncounterStatus(status string) string {
	switch strings.ToLower(status) {
	case "planned", "pending":
		return "planned"
	case "arrived":
		return "arrived"
	case "triaged":
		return "triaged"
	case "in-progress", "active", "admitted":
		return "in-progress"
	case "onleave", "on-leave":
		return "onleave"
	case "finished", "discharged", "completed":
		return "finished"
	case "cancelled", "canceled":
		return "cancelled"
	default:
		return "unknown"
	}
}

func (m *USCoreMapper) mapEncounterClass(class string) Coding {
	// Map to ActCode vocabulary
	switch strings.ToUpper(class) {
	case "I", "INPATIENT", "IMP":
		return Coding{System: SystemEncounterClass, Code: "IMP", Display: "inpatient encounter"}
	case "O", "OUTPATIENT", "AMB":
		return Coding{System: SystemEncounterClass, Code: "AMB", Display: "ambulatory"}
	case "E", "EMERGENCY", "EMER":
		return Coding{System: SystemEncounterClass, Code: "EMER", Display: "emergency"}
	case "P", "PREADMIT":
		return Coding{System: SystemEncounterClass, Code: "PRENC", Display: "pre-admission"}
	case "R", "RECURRING", "RECURRING PATIENT":
		return Coding{System: SystemEncounterClass, Code: "AMB", Display: "ambulatory"}
	default:
		return Coding{System: SystemEncounterClass, Code: "AMB", Display: "ambulatory"}
	}
}

func (m *USCoreMapper) mapObservationStatus(status string) string {
	switch strings.ToLower(status) {
	case "f", "final":
		return "final"
	case "p", "preliminary":
		return "preliminary"
	case "c", "corrected":
		return "corrected"
	case "a", "amended":
		return "amended"
	case "x", "cancelled", "canceled":
		return "cancelled"
	case "r", "registered":
		return "registered"
	default:
		return "final"
	}
}

func (m *USCoreMapper) mapDiagnosticReportStatus(status string) string {
	switch strings.ToLower(status) {
	case "f", "final":
		return "final"
	case "p", "preliminary":
		return "preliminary"
	case "c", "corrected":
		return "corrected"
	case "a", "amended":
		return "amended"
	case "x", "cancelled", "canceled":
		return "cancelled"
	case "r", "registered":
		return "registered"
	case "partial":
		return "partial"
	default:
		return "final"
	}
}

func (m *USCoreMapper) mapLabCode(test *events.LabTest) CodeableConcept {
	result := CodeableConcept{
		Text: test.Description,
	}

	// A (system, code) pair may arrive both as the convenience LOINCCode field
	// and inside Code.Coding, which is the normal shape when a parser fills both.
	// Emitting it twice is a duplicate coding: cosmetic against this checker,
	// a real finding under any structural validator, and never correct.
	seen := make(map[string]bool, len(test.Code.Coding)+1)
	appendCoding := func(coding Coding) {
		key := coding.System + "|" + coding.Code
		if seen[key] {
			return
		}
		seen[key] = true
		result.Coding = append(result.Coding, coding)
	}

	// Add LOINC coding if available
	if test.LOINCCode != "" {
		appendCoding(Coding{
			System:  SystemLOINC,
			Code:    test.LOINCCode,
			Display: test.Description,
		})
	}

	// Add codings from CodeableConcept
	for _, coding := range test.Code.Coding {
		appendCoding(Coding{
			System:  coding.System,
			Code:    coding.Code,
			Display: coding.Display,
		})
	}

	// Add local code if no other codings
	if len(result.Coding) == 0 && test.LocalCode != "" {
		result.Coding = append(result.Coding, Coding{
			System:  "http://local.example.org/lab-codes",
			Code:    test.LocalCode,
			Display: test.Description,
		})
	}

	return result
}

func (m *USCoreMapper) mapInterpretation(interp string) Coding {
	switch strings.ToUpper(interp) {
	case "H", "HIGH":
		return Coding{System: SystemInterpretation, Code: "H", Display: "High"}
	case "HH", "CRITICAL HIGH", "CRITICAL_HIGH":
		return Coding{System: SystemInterpretation, Code: "HH", Display: "Critical high"}
	case "L", "LOW":
		return Coding{System: SystemInterpretation, Code: "L", Display: "Low"}
	case "LL", "CRITICAL LOW", "CRITICAL_LOW":
		return Coding{System: SystemInterpretation, Code: "LL", Display: "Critical low"}
	case "N", "NORMAL":
		return Coding{System: SystemInterpretation, Code: "N", Display: "Normal"}
	case "A", "ABNORMAL":
		return Coding{System: SystemInterpretation, Code: "A", Display: "Abnormal"}
	default:
		return Coding{System: SystemInterpretation, Code: "N", Display: "Normal"}
	}
}

func (m *USCoreMapper) providerReference(prov *events.Provider) *Reference {
	if prov == nil {
		return nil
	}

	ref := &Reference{}
	if prov.NPI != "" {
		ref.Reference = fmt.Sprintf("Practitioner/%s", prov.NPI)
	} else if prov.ID != "" {
		ref.Reference = fmt.Sprintf("Practitioner/%s", prov.ID)
	}
	ref.Display = formatProviderName(prov)

	return ref
}

func formatProviderName(prov *events.Provider) string {
	if prov == nil {
		return ""
	}

	var parts []string
	if prov.Prefix != "" {
		parts = append(parts, prov.Prefix)
	}
	if prov.GivenName != "" {
		parts = append(parts, prov.GivenName)
	}
	if prov.FamilyName != "" {
		parts = append(parts, prov.FamilyName)
	}
	if prov.Suffix != "" {
		parts = append(parts, prov.Suffix)
	}

	return strings.Join(parts, " ")
}

func formatLocation(loc *events.Location) string {
	var parts []string
	if loc.Facility != "" {
		parts = append(parts, loc.Facility)
	}
	if loc.Building != "" {
		parts = append(parts, loc.Building)
	}
	if loc.Unit != "" {
		parts = append(parts, loc.Unit)
	}
	if loc.Room != "" {
		parts = append(parts, "Room "+loc.Room)
	}
	if loc.Bed != "" {
		parts = append(parts, "Bed "+loc.Bed)
	}

	return strings.Join(parts, ", ")
}

// MapCondition converts a canonical ConditionEvent to a US Core Condition.
func (m *USCoreMapper) MapCondition(event *events.ConditionEvent, patientRef string) *Condition {
	if event == nil {
		return nil
	}

	condition := &Condition{
		ResourceType: "Condition",
		Meta: &Meta{
			Profile: []string{USCoreConditionProfile},
		},
	}

	// Set subject reference (required)
	if patientRef != "" {
		condition.Subject = &Reference{
			Reference: patientRef,
		}
	} else if event.Patient != nil && event.Patient.MRN != "" {
		condition.Subject = &Reference{
			Reference: fmt.Sprintf("Patient/%s", event.Patient.MRN),
		}
	}

	// Map clinical status (required for US Core)
	condition.ClinicalStatus = m.mapConditionClinicalStatus(event.ClinicalStatus)

	// Map verification status
	condition.VerificationStatus = &CodeableConcept{
		Coding: []Coding{
			{
				System:  SystemConditionVerificationStatus,
				Code:    "confirmed",
				Display: "Confirmed",
			},
		},
	}

	// Map category
	condition.Category = m.mapConditionCategory(event.Condition.Category)

	// Map condition code (SNOMED CT or ICD-10)
	condition.Code = m.mapConditionCode(&event.Condition)

	// Map onset date
	if event.OnsetDate != "" {
		condition.OnsetDateTime = event.OnsetDate
	}

	// Map abatement date (for resolved conditions)
	if event.AbatementDate != "" {
		condition.AbatementDateTime = event.AbatementDate
	}

	// Map encounter reference
	if event.Encounter != nil && event.Encounter.ID != "" {
		condition.Encounter = &Reference{
			Reference: fmt.Sprintf("Encounter/%s", event.Encounter.ID),
		}
	}

	// Set recorded date from event timestamp
	if !event.Timestamp.IsZero() {
		condition.RecordedDate = event.Timestamp.Format("2006-01-02")
	}

	return condition
}

// MapCoverage converts a canonical EligibilityResponseEvent to a US Core Coverage.
func (m *USCoreMapper) MapCoverage(event *events.EligibilityResponseEvent, beneficiaryRef string) *Coverage {
	if event == nil {
		return nil
	}

	coverage := &Coverage{
		ResourceType: "Coverage",
		Meta: &Meta{
			Profile: []string{USCoreCoverageProfile},
		},
	}

	// Map status from eligibility status
	coverage.Status = m.mapCoverageStatus(event.Status)

	// Set beneficiary reference (required)
	if beneficiaryRef != "" {
		coverage.Beneficiary = &Reference{
			Reference: beneficiaryRef,
		}
	} else if event.Dependent != nil && event.Dependent.MRN != "" {
		coverage.Beneficiary = &Reference{
			Reference: fmt.Sprintf("Patient/%s", event.Dependent.MRN),
		}
	} else if event.Subscriber.MRN != "" {
		coverage.Beneficiary = &Reference{
			Reference: fmt.Sprintf("Patient/%s", event.Subscriber.MRN),
		}
	}

	// Set subscriber reference
	if event.Subscriber.MRN != "" {
		coverage.Subscriber = &Reference{
			Reference: fmt.Sprintf("Patient/%s", event.Subscriber.MRN),
		}
	}

	// Map subscriber ID from identifiers
	if memberId := event.Subscriber.Identifiers.GetByType("MB"); memberId != nil {
		coverage.SubscriberId = memberId.Value
	} else if event.Subscriber.MRN != "" {
		coverage.SubscriberId = event.Subscriber.MRN
	}

	// Set payor (required) from information source
	coverage.Payor = []Reference{
		{
			Display: event.InformationSource.OrganizationName,
		},
	}
	if event.InformationSource.NPI != "" {
		coverage.Payor[0].Reference = fmt.Sprintf("Organization/%s", event.InformationSource.NPI)
	}

	// Map coverage period from plan dates
	if !event.PlanBeginDate.IsZero() || !event.PlanEndDate.IsZero() {
		coverage.Period = &Period{}
		if !event.PlanBeginDate.IsZero() {
			t := event.PlanBeginDate
			coverage.Period.Start = &t
		}
		if !event.PlanEndDate.IsZero() {
			t := event.PlanEndDate
			coverage.Period.End = &t
		}
	}

	// Map plan/group information from benefits
	coverage.Class = m.extractCoverageClasses(event.Benefits)

	// Map cost-to-beneficiary (deductibles, copays) from benefits
	coverage.CostToBeneficiary = m.extractCostToBeneficiary(event.Benefits)

	// Map insurance type from benefits
	coverage.Type = m.extractCoverageType(event.Benefits)

	return coverage
}

// Helper methods for Condition mapping

func (m *USCoreMapper) mapConditionClinicalStatus(status string) *CodeableConcept {
	var code, display string

	switch strings.ToLower(status) {
	case "active", "":
		code, display = "active", "Active"
	case "recurrence":
		code, display = "recurrence", "Recurrence"
	case "relapse":
		code, display = "relapse", "Relapse"
	case "inactive":
		code, display = "inactive", "Inactive"
	case "remission":
		code, display = "remission", "Remission"
	case "resolved":
		code, display = "resolved", "Resolved"
	default:
		code, display = "active", "Active"
	}

	return &CodeableConcept{
		Coding: []Coding{
			{
				System:  SystemConditionClinicalStatus,
				Code:    code,
				Display: display,
			},
		},
	}
}

func (m *USCoreMapper) mapConditionCategory(category string) []CodeableConcept {
	var code, display string

	switch strings.ToLower(category) {
	case "problem-list-item", "problem", "":
		code, display = "problem-list-item", "Problem List Item"
	case "encounter-diagnosis", "diagnosis":
		code, display = "encounter-diagnosis", "Encounter Diagnosis"
	case "health-concern", "concern":
		code, display = "health-concern", "Health Concern"
	default:
		code, display = "problem-list-item", "Problem List Item"
	}

	return []CodeableConcept{
		{
			Coding: []Coding{
				{
					System:  SystemConditionCategory,
					Code:    code,
					Display: display,
				},
			},
		},
	}
}

func (m *USCoreMapper) mapConditionCode(cond *events.Condition) CodeableConcept {
	result := CodeableConcept{
		Text: cond.Name,
	}

	if cond.Code != "" {
		coding := Coding{
			Code:    cond.Code,
			Display: cond.Name,
		}

		// Determine code system
		if cond.CodeSystem != "" {
			coding.System = cond.CodeSystem
		} else {
			// Try to infer from code format
			coding.System = m.inferConditionCodeSystem(cond.Code)
		}

		result.Coding = []Coding{coding}
	}

	return result
}

func (m *USCoreMapper) inferConditionCodeSystem(code string) string {
	// ICD-10-CM codes start with a letter followed by digits
	if len(code) >= 3 {
		firstChar := code[0]
		if (firstChar >= 'A' && firstChar <= 'Z') || (firstChar >= 'a' && firstChar <= 'z') {
			// Check for ICD-10-CM format (e.g., E11.9, J18.9)
			if len(code) >= 3 && code[1] >= '0' && code[1] <= '9' && code[2] >= '0' && code[2] <= '9' {
				return SystemICD10CM
			}
		}
	}

	// SNOMED CT codes are purely numeric and typically 6-18 digits
	allDigits := true
	for _, c := range code {
		if c < '0' || c > '9' {
			allDigits = false
			break
		}
	}
	if allDigits && len(code) >= 6 {
		return SystemSNOMED
	}

	// Default to SNOMED CT as per US Core preference
	return SystemSNOMED
}

// Helper methods for Coverage mapping

func (m *USCoreMapper) mapCoverageStatus(status events.EligibilityStatus) string {
	switch status {
	case events.EligibilityStatusActive:
		return "active"
	case events.EligibilityStatusInactive:
		return "cancelled"
	case events.EligibilityStatusRejected:
		return "entered-in-error"
	default:
		return "draft"
	}
}

func (m *USCoreMapper) extractCoverageClasses(benefits []events.EligibilityBenefit) []CoverageClass {
	var classes []CoverageClass
	seenPlans := make(map[string]bool)

	for _, benefit := range benefits {
		// Extract plan information
		if benefit.PlanDescription != "" && !seenPlans[benefit.PlanDescription] {
			seenPlans[benefit.PlanDescription] = true
			classes = append(classes, CoverageClass{
				Type: CodeableConcept{
					Coding: []Coding{
						{System: SystemCoverageClass, Code: "plan", Display: "Plan"},
					},
				},
				Value: benefit.PlanDescription,
				Name:  benefit.PlanDescription,
			})
		}
	}

	return classes
}

func (m *USCoreMapper) extractCostToBeneficiary(benefits []events.EligibilityBenefit) []CostToBeneficiary {
	var costs []CostToBeneficiary

	for _, benefit := range benefits {
		// Map deductibles (information code C = Deductible Amount)
		if benefit.InformationCode == "C" && benefit.Amount > 0 {
			costs = append(costs, CostToBeneficiary{
				Type: &CodeableConcept{
					Coding: []Coding{
						{System: SystemCopayType, Code: "deductible", Display: "Deductible"},
					},
				},
				ValueMoney: &Money{
					Value:    benefit.Amount,
					Currency: "USD",
				},
			})
		}

		// Map copays (information code B = Copay Amount)
		if benefit.InformationCode == "B" && benefit.Amount > 0 {
			costs = append(costs, CostToBeneficiary{
				Type: &CodeableConcept{
					Coding: []Coding{
						{System: SystemCopayType, Code: "copay", Display: "CoPay"},
					},
				},
				ValueMoney: &Money{
					Value:    benefit.Amount,
					Currency: "USD",
				},
			})
		}

		// Map coinsurance (information code A = Coinsurance)
		if benefit.InformationCode == "A" && benefit.Percent > 0 {
			costs = append(costs, CostToBeneficiary{
				Type: &CodeableConcept{
					Coding: []Coding{
						{System: SystemCopayType, Code: "coinsurance", Display: "Coinsurance"},
					},
				},
				ValueQuantity: &Quantity{
					Value: benefit.Percent,
					Unit:  "%",
				},
			})
		}
	}

	return costs
}

func (m *USCoreMapper) extractCoverageType(benefits []events.EligibilityBenefit) *CodeableConcept {
	for _, benefit := range benefits {
		if benefit.InsuranceType != "" {
			return m.mapInsuranceType(benefit.InsuranceType)
		}
	}
	return nil
}

func (m *USCoreMapper) mapInsuranceType(insuranceType string) *CodeableConcept {
	var code, display string

	switch strings.ToUpper(insuranceType) {
	case "HM":
		code, display = "HMO", "health maintenance organization policy"
	case "PR":
		code, display = "PPO", "preferred provider organization policy"
	case "PS":
		code, display = "POS", "point of service policy"
	case "EP":
		code, display = "EPO", "exclusive provider organization policy"
	case "MC":
		code, display = "MCPOL", "managed care policy"
	case "IN":
		code, display = "PUBLICPOL", "public healthcare"
	case "MA":
		code, display = "MCPOL", "managed care policy"
	case "MB":
		code, display = "PUBLICPOL", "public healthcare" // Medicare Part B
	default:
		code, display = "EHCPOL", "extended healthcare"
	}

	return &CodeableConcept{
		Coding: []Coding{
			{
				System:  SystemCoverageType,
				Code:    code,
				Display: display,
			},
		},
	}
}

// CreateTransactionBundle creates a FHIR transaction bundle from resources.
func CreateTransactionBundle(resources []Resource) *Bundle {
	bundle := &Bundle{
		ResourceType: "Bundle",
		Type:         "transaction",
		Entry:        make([]BundleEntry, len(resources)),
	}

	for i, resource := range resources {
		resourceJSON, _ := json.Marshal(resource)
		bundle.Entry[i] = BundleEntry{
			Resource: resourceJSON,
			Request: &BundleEntryRequest{
				Method: "POST",
				URL:    resource.GetResourceType(),
			},
		}
	}

	return bundle
}

// CreateSearchsetBundle creates a FHIR searchset bundle from resources.
func CreateSearchsetBundle(resources []Resource, total int) *Bundle {
	bundle := &Bundle{
		ResourceType: "Bundle",
		Type:         "searchset",
		Total:        total,
		Entry:        make([]BundleEntry, len(resources)),
	}

	for i, resource := range resources {
		resourceJSON, _ := json.Marshal(resource)
		bundle.Entry[i] = BundleEntry{
			Resource: resourceJSON,
			Search: &BundleEntrySearch{
				Mode: "match",
			},
		}
	}

	return bundle
}

// MapClaim converts a canonical ClaimSubmittedEvent to a FHIR Claim.
// The use parameter controls whether this is a billing claim ("claim") or
// prior authorization request ("preauthorization").
func (m *USCoreMapper) MapClaim(event *events.ClaimSubmittedEvent, use string) *Claim {
	if event == nil {
		return nil
	}

	// Default to "claim" if not specified
	if use == "" {
		use = "claim"
	}

	// Select profile based on use
	profile := ""
	if use == "preauthorization" {
		profile = DaVinciPASClaimProfile
	}

	claim := &Claim{
		ResourceType: "Claim",
		Status:       "active",
		Use:          use,
	}

	if profile != "" {
		claim.Meta = &Meta{
			Profile: []string{profile},
		}
	}

	// Set claim type (professional for 837P)
	claim.Type = CodeableConcept{
		Coding: []Coding{
			{
				System:  SystemClaimType,
				Code:    "professional",
				Display: "Professional",
			},
		},
	}

	// Set priority
	claim.Priority = CodeableConcept{
		Coding: []Coding{
			{
				System:  "http://terminology.hl7.org/CodeSystem/processpriority",
				Code:    "normal",
				Display: "Normal",
			},
		},
	}

	// Map patient reference
	if event.Patient.MRN != "" {
		claim.Patient = &Reference{
			Reference: fmt.Sprintf("Patient/%s", event.Patient.MRN),
			Display:   fmt.Sprintf("%s, %s", event.Patient.FamilyName, event.Patient.GivenName),
		}
	}

	// Map billing provider
	if event.BillingProvider.NPI != "" {
		claim.Provider = &Reference{
			Reference: fmt.Sprintf("Organization/%s", event.BillingProvider.NPI),
			Display:   event.BillingProvider.OrganizationName,
		}
	}

	// Map payer/insurer
	if event.Payer.NPI != "" || event.Payer.OrganizationName != "" {
		claim.Insurer = &Reference{
			Display: event.Payer.OrganizationName,
		}
		if event.Payer.NPI != "" {
			claim.Insurer.Reference = fmt.Sprintf("Organization/%s", event.Payer.NPI)
		}
	}

	// Add claim identifier
	if event.Claim.ControlNumber != "" {
		claim.Identifier = []Identifier{
			{
				System: "urn:oid:2.16.840.1.113883.3.8901.2.1", // Sample submitter ID system
				Value:  event.Claim.ControlNumber,
			},
		}
	}

	// Set created date
	if !event.Claim.ServiceDate.IsZero() {
		claim.Created = event.Claim.ServiceDate.Format("2006-01-02")
	} else if !event.Timestamp.IsZero() {
		claim.Created = event.Timestamp.Format("2006-01-02")
	}

	// Map diagnosis codes
	claim.Diagnosis = m.mapClaimDiagnoses(event.Claim.DiagnosisCodes)

	// Map care team (rendering provider)
	claim.CareTeam = m.mapClaimCareTeam(event)

	// Map insurance
	claim.Insurance = m.mapClaimInsurance(event)

	// Map service line items
	claim.Item = m.mapClaimItems(event.Claim.ServiceLines, event.Claim.PlaceOfService)

	// Set total
	if event.Claim.TotalAmount > 0 {
		claim.Total = &Money{
			Value:    event.Claim.TotalAmount,
			Currency: "USD",
		}
	}

	return claim
}

func (m *USCoreMapper) mapClaimDiagnoses(codes []string) []ClaimDiagnosis {
	var diagnoses []ClaimDiagnosis

	for i, code := range codes {
		if code == "" {
			continue
		}

		diag := ClaimDiagnosis{
			Sequence: i + 1,
			DiagnosisCodeable: &CodeableConcept{
				Coding: []Coding{
					{
						System: SystemICD10CM,
						Code:   code,
					},
				},
			},
		}

		// First diagnosis is principal
		if i == 0 {
			diag.Type = []CodeableConcept{
				{
					Coding: []Coding{
						{
							System:  "http://terminology.hl7.org/CodeSystem/ex-diagnosistype",
							Code:    "principal",
							Display: "Principal Diagnosis",
						},
					},
				},
			}
		}

		diagnoses = append(diagnoses, diag)
	}

	return diagnoses
}

func (m *USCoreMapper) mapClaimCareTeam(event *events.ClaimSubmittedEvent) []ClaimCareTeam {
	var careTeam []ClaimCareTeam
	seq := 1

	// Add billing provider
	if event.BillingProvider.NPI != "" {
		providerDisplay := event.BillingProvider.OrganizationName
		if providerDisplay == "" && event.BillingProvider.FamilyName != "" {
			providerDisplay = fmt.Sprintf("%s, %s", event.BillingProvider.FamilyName, event.BillingProvider.GivenName)
		}
		careTeam = append(careTeam, ClaimCareTeam{
			Sequence: seq,
			Provider: &Reference{
				Reference: fmt.Sprintf("Practitioner/%s", event.BillingProvider.NPI),
				Display:   providerDisplay,
			},
			Role: &CodeableConcept{
				Coding: []Coding{
					{
						System:  "http://terminology.hl7.org/CodeSystem/claimcareteamrole",
						Code:    "primary",
						Display: "Primary provider",
					},
				},
			},
		})
		seq++
	}

	// Add rendering provider if different
	if event.RenderingProvider != nil && event.RenderingProvider.NPI != "" {
		renderingDisplay := event.RenderingProvider.OrganizationName
		if renderingDisplay == "" && event.RenderingProvider.FamilyName != "" {
			renderingDisplay = fmt.Sprintf("%s, %s", event.RenderingProvider.FamilyName, event.RenderingProvider.GivenName)
		}
		careTeam = append(careTeam, ClaimCareTeam{
			Sequence: seq,
			Provider: &Reference{
				Reference: fmt.Sprintf("Practitioner/%s", event.RenderingProvider.NPI),
				Display:   renderingDisplay,
			},
			Role: &CodeableConcept{
				Coding: []Coding{
					{
						System:  "http://terminology.hl7.org/CodeSystem/claimcareteamrole",
						Code:    "rendering",
						Display: "Rendering provider",
					},
				},
			},
		})
	}

	return careTeam
}

func (m *USCoreMapper) mapClaimInsurance(event *events.ClaimSubmittedEvent) []ClaimInsurance {
	insurance := ClaimInsurance{
		Sequence: 1,
		Focal:    true,
	}

	// Reference subscriber's coverage
	subscriberID := ""
	if mbID := event.Subscriber.Identifiers.GetByType("MB"); mbID != nil {
		subscriberID = mbID.Value
	} else if event.Subscriber.MRN != "" {
		subscriberID = event.Subscriber.MRN
	}

	if subscriberID != "" {
		insurance.Coverage = &Reference{
			Reference: fmt.Sprintf("Coverage/%s", subscriberID),
		}
	}

	return []ClaimInsurance{insurance}
}

func (m *USCoreMapper) mapClaimItems(lines []events.ServiceLine, placeOfService string) []ClaimItem {
	var items []ClaimItem

	for _, line := range lines {
		item := ClaimItem{
			Sequence: line.LineNumber,
			ProductOrService: CodeableConcept{
				Coding: []Coding{
					{
						System: SystemCPT,
						Code:   line.ProcedureCode,
					},
				},
			},
		}

		// Map modifiers
		for _, mod := range line.Modifiers {
			item.Modifier = append(item.Modifier, CodeableConcept{
				Coding: []Coding{
					{
						System: SystemCPT,
						Code:   mod,
					},
				},
			})
		}

		// Map quantity/units
		if line.Units > 0 {
			item.Quantity = &Quantity{
				Value: line.Units,
				Unit:  line.UnitType,
			}
		}

		// Map charge amount
		if line.ChargeAmount > 0 {
			item.UnitPrice = &Money{
				Value:    line.ChargeAmount / max(line.Units, 1),
				Currency: "USD",
			}
			item.Net = &Money{
				Value:    line.ChargeAmount,
				Currency: "USD",
			}
		}

		// Map service date
		if !line.ServiceDate.IsZero() {
			item.ServicedDate = line.ServiceDate.Format("2006-01-02")
		}

		// Map place of service
		if placeOfService != "" {
			item.LocationCodeable = &CodeableConcept{
				Coding: []Coding{
					{
						System: SystemPlaceOfService,
						Code:   placeOfService,
					},
				},
			}
		}

		// Map diagnosis pointers
		item.DiagnosisSequence = line.DiagnosisPointers

		items = append(items, item)
	}

	return items
}

// MapExplanationOfBenefit converts a ClaimAdjudicatedEvent to a FHIR ExplanationOfBenefit.
func (m *USCoreMapper) MapExplanationOfBenefit(event *events.ClaimAdjudicatedEvent) *ExplanationOfBenefit {
	if event == nil {
		return nil
	}

	eob := &ExplanationOfBenefit{
		ResourceType: "ExplanationOfBenefit",
		Meta: &Meta{
			Profile: []string{PDexEOBProfile},
		},
		Status: "active",
		Use:    "claim",
	}

	// Set type (professional for 837P/835)
	eob.Type = CodeableConcept{
		Coding: []Coding{
			{
				System:  SystemClaimType,
				Code:    "professional",
				Display: "Professional",
			},
		},
	}

	// Map payer/insurer
	if event.Payer.NPI != "" || event.Payer.OrganizationName != "" {
		eob.Insurer = &Reference{
			Display: event.Payer.OrganizationName,
		}
		if event.Payer.NPI != "" {
			eob.Insurer.Reference = fmt.Sprintf("Organization/%s", event.Payer.NPI)
		}
	}

	// Map provider from payee
	if event.Payee.NPI != "" || event.Payee.OrganizationName != "" {
		eob.Provider = &Reference{
			Display: event.Payee.OrganizationName,
		}
		if event.Payee.NPI != "" {
			eob.Provider.Reference = fmt.Sprintf("Organization/%s", event.Payee.NPI)
		}
	}

	// Set outcome based on claim status
	eob.Outcome = m.mapClaimOutcome(event.Payment.Status)

	// Add identifiers
	var identifiers []Identifier
	if event.Payment.PayerClaimID != "" {
		identifiers = append(identifiers, Identifier{
			System: "urn:oid:2.16.840.1.113883.3.8901.2.2", // Sample payer ID system
			Value:  event.Payment.PayerClaimID,
		})
	}
	if event.Payment.ClaimID != "" {
		identifiers = append(identifiers, Identifier{
			System: "urn:oid:2.16.840.1.113883.3.8901.2.1", // Sample submitter ID system
			Value:  event.Payment.ClaimID,
		})
	}
	eob.Identifier = identifiers

	// Map insurance
	eob.Insurance = []EOBInsurance{
		{
			Focal: true,
			Coverage: &Reference{
				Display: event.Payer.OrganizationName,
			},
		},
	}

	// Set created date
	if !event.Timestamp.IsZero() {
		eob.Created = event.Timestamp.Format("2006-01-02T15:04:05Z")
	}

	// Map service line payments
	eob.Item = m.mapEOBItems(event.Payment.ServiceLinePayments)

	// Map header-level adjudication totals
	eob.Total = m.mapEOBTotals(event)

	// Map payment information
	eob.Payment = m.mapEOBPayment(event)

	return eob
}

func (m *USCoreMapper) mapClaimOutcome(status string) string {
	switch strings.ToLower(status) {
	case "processed", "paid", "complete":
		return "complete"
	case "denied":
		return "error"
	case "pending", "in process":
		return "queued"
	case "partial":
		return "partial"
	default:
		return "complete"
	}
}

func (m *USCoreMapper) mapEOBItems(lines []events.ServiceLinePayment) []EOBItem {
	var items []EOBItem

	for i, line := range lines {
		item := EOBItem{
			Sequence: i + 1,
			ProductOrService: CodeableConcept{
				Coding: []Coding{
					{
						System: SystemCPT,
						Code:   line.ProcedureCode,
					},
				},
			},
		}

		// Map adjudication amounts
		item.Adjudication = m.mapLineAdjudication(line)

		items = append(items, item)
	}

	return items
}

func (m *USCoreMapper) mapLineAdjudication(line events.ServiceLinePayment) []EOBAdjudication {
	var adjudications []EOBAdjudication

	// Submitted amount
	if line.ChargedAmount > 0 {
		adjudications = append(adjudications, EOBAdjudication{
			Category: CodeableConcept{
				Coding: []Coding{
					{
						System:  SystemAdjudicationCategory,
						Code:    "submitted",
						Display: "Submitted Amount",
					},
				},
			},
			Amount: &Money{
				Value:    line.ChargedAmount,
				Currency: "USD",
			},
		})
	}

	// Paid/benefit amount
	adjudications = append(adjudications, EOBAdjudication{
		Category: CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemAdjudicationCategory,
					Code:    "benefit",
					Display: "Benefit Amount",
				},
			},
		},
		Amount: &Money{
			Value:    line.PaidAmount,
			Currency: "USD",
		},
	})

	// Map CARC adjustments
	for _, adj := range line.Adjustments {
		adjCategory := m.mapAdjustmentGroup(adj.Group)
		adjudications = append(adjudications, EOBAdjudication{
			Category: adjCategory,
			Reason: &CodeableConcept{
				Coding: []Coding{
					{
						System: SystemCARC,
						Code:   adj.ReasonCode,
					},
				},
			},
			Amount: &Money{
				Value:    adj.Amount,
				Currency: "USD",
			},
		})
	}

	return adjudications
}

func (m *USCoreMapper) mapAdjustmentGroup(group string) CodeableConcept {
	var code, display string

	switch strings.ToUpper(group) {
	case "CO":
		code, display = "copay", "Patient Co-Payment"
	case "PR":
		code, display = "deductible", "Deductible"
	case "OA":
		code, display = "eligible", "Eligible Amount"
	case "PI":
		code, display = "paid", "Paid to Provider"
	case "CR":
		code, display = "prior", "Prior Payer Amount"
	default:
		code, display = "submitted", "Submitted Amount"
	}

	return CodeableConcept{
		Coding: []Coding{
			{
				System:  SystemAdjudicationCategory,
				Code:    code,
				Display: display,
			},
		},
		Text: fmt.Sprintf("Adjustment Group: %s", group),
	}
}

func (m *USCoreMapper) mapEOBTotals(event *events.ClaimAdjudicatedEvent) []EOBTotal {
	var totals []EOBTotal

	// Total charged
	if event.Payment.ChargedAmount > 0 {
		totals = append(totals, EOBTotal{
			Category: CodeableConcept{
				Coding: []Coding{
					{
						System:  SystemAdjudicationCategory,
						Code:    "submitted",
						Display: "Submitted Amount",
					},
				},
			},
			Amount: Money{
				Value:    event.Payment.ChargedAmount,
				Currency: "USD",
			},
		})
	}

	// Total paid
	totals = append(totals, EOBTotal{
		Category: CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemAdjudicationCategory,
					Code:    "benefit",
					Display: "Benefit Amount",
				},
			},
		},
		Amount: Money{
			Value:    event.Payment.PaidAmount,
			Currency: "USD",
		},
	})

	// Patient responsibility (sum of PR adjustments)
	patientAmount := 0.0
	for _, adj := range event.Payment.Adjustments {
		if adj.Group == "PR" {
			patientAmount += adj.Amount
		}
	}
	if patientAmount > 0 {
		totals = append(totals, EOBTotal{
			Category: CodeableConcept{
				Coding: []Coding{
					{
						System:  SystemAdjudicationCategory,
						Code:    "deductible",
						Display: "Patient Responsibility",
					},
				},
			},
			Amount: Money{
				Value:    patientAmount,
				Currency: "USD",
			},
		})
	}

	return totals
}

func (m *USCoreMapper) mapEOBPayment(event *events.ClaimAdjudicatedEvent) *EOBPayment {
	payment := &EOBPayment{}

	// Payment type
	payment.Type = &CodeableConcept{
		Coding: []Coding{
			{
				System:  SystemPaymentType,
				Code:    "complete",
				Display: "Complete",
			},
		},
	}

	// Payment date
	if !event.CheckDate.IsZero() {
		payment.Date = event.CheckDate.Format("2006-01-02")
	}

	// Payment amount
	payment.Amount = &Money{
		Value:    event.TotalPaid,
		Currency: "USD",
	}

	// Check/EFT identifier
	if event.CheckNumber != "" {
		payment.Identifier = &Identifier{
			System: "urn:oid:2.16.840.1.113883.3.8901.2.3", // Sample check number system
			Value:  event.CheckNumber,
		}
	}

	return payment
}

// MapCoverageEligibilityResponse converts an EligibilityResponseEvent to a FHIR CoverageEligibilityResponse.
func (m *USCoreMapper) MapCoverageEligibilityResponse(event *events.EligibilityResponseEvent, patientRef string) *CoverageEligibilityResponse {
	if event == nil {
		return nil
	}

	cer := &CoverageEligibilityResponse{
		ResourceType: "CoverageEligibilityResponse",
		Status:       "active",
		Purpose:      []string{"benefits"},
		Outcome:      m.mapEligibilityOutcome(event.Status, event.Errors),
	}

	// Set patient reference
	if patientRef != "" {
		cer.Patient = &Reference{Reference: patientRef}
	} else if event.Dependent != nil && event.Dependent.MRN != "" {
		cer.Patient = &Reference{
			Reference: fmt.Sprintf("Patient/%s", event.Dependent.MRN),
			Display:   fmt.Sprintf("%s, %s", event.Dependent.FamilyName, event.Dependent.GivenName),
		}
	} else if event.Subscriber.MRN != "" {
		cer.Patient = &Reference{
			Reference: fmt.Sprintf("Patient/%s", event.Subscriber.MRN),
			Display:   fmt.Sprintf("%s, %s", event.Subscriber.FamilyName, event.Subscriber.GivenName),
		}
	}

	// Set insurer from information source
	if event.InformationSource.NPI != "" || event.InformationSource.OrganizationName != "" {
		cer.Insurer = &Reference{
			Display: event.InformationSource.OrganizationName,
		}
		if event.InformationSource.NPI != "" {
			cer.Insurer.Reference = fmt.Sprintf("Organization/%s", event.InformationSource.NPI)
		}
	}

	// Set requestor from information receiver
	if event.InformationReceiver.NPI != "" || event.InformationReceiver.OrganizationName != "" {
		cer.Requestor = &Reference{
			Display: event.InformationReceiver.OrganizationName,
		}
		if event.InformationReceiver.NPI != "" {
			cer.Requestor.Reference = fmt.Sprintf("Organization/%s", event.InformationReceiver.NPI)
		}
	}

	// Set created date
	if !event.Timestamp.IsZero() {
		cer.Created = event.Timestamp.Format("2006-01-02T15:04:05Z")
	}

	// Add trace number as identifier
	if event.TraceNumber != "" {
		cer.Identifier = []Identifier{
			{
				System: "urn:oid:2.16.840.1.113883.3.8901.2.4", // Sample trace number system
				Value:  event.TraceNumber,
			},
		}
	}

	// Build insurance section
	cer.Insurance = m.buildCERInsurance(event)

	// Map errors
	if len(event.Errors) > 0 {
		cer.Error = m.mapEligibilityErrors(event.Errors)
	}

	return cer
}

func (m *USCoreMapper) mapEligibilityOutcome(status events.EligibilityStatus, errors []events.EligibilityValidationError) string {
	if len(errors) > 0 {
		return "error"
	}

	switch status {
	case events.EligibilityStatusActive:
		return "complete"
	case events.EligibilityStatusInactive:
		return "complete"
	case events.EligibilityStatusRejected:
		return "error"
	default:
		return "complete"
	}
}

func (m *USCoreMapper) buildCERInsurance(event *events.EligibilityResponseEvent) []CERInsurance {
	insurance := CERInsurance{
		Inforce: event.Status == events.EligibilityStatusActive,
	}

	// Set coverage reference from subscriber
	subscriberID := ""
	if mbID := event.Subscriber.Identifiers.GetByType("MB"); mbID != nil {
		subscriberID = mbID.Value
	} else if event.Subscriber.MRN != "" {
		subscriberID = event.Subscriber.MRN
	}
	if subscriberID != "" {
		insurance.Coverage = &Reference{
			Reference: fmt.Sprintf("Coverage/%s", subscriberID),
		}
	}

	// Set benefit period
	if !event.PlanBeginDate.IsZero() || !event.PlanEndDate.IsZero() {
		insurance.BenefitPeriod = &Period{}
		if !event.PlanBeginDate.IsZero() {
			t := event.PlanBeginDate
			insurance.BenefitPeriod.Start = &t
		}
		if !event.PlanEndDate.IsZero() {
			t := event.PlanEndDate
			insurance.BenefitPeriod.End = &t
		}
	}

	// Group benefits by service type and network indicator
	insurance.Item = m.groupBenefitsIntoItems(event.Benefits)

	return []CERInsurance{insurance}
}

func (m *USCoreMapper) groupBenefitsIntoItems(benefits []events.EligibilityBenefit) []CERItem {
	// Group benefits by service type + network indicator
	type itemKey struct {
		serviceType string
		network     string
	}
	groups := make(map[itemKey]*CERItem)

	for _, benefit := range benefits {
		key := itemKey{
			serviceType: benefit.ServiceType,
			network:     benefit.InNetworkIndicator,
		}

		item, exists := groups[key]
		if !exists {
			item = &CERItem{
				Name:        benefit.ServiceTypeDescription,
				Description: benefit.PlanDescription,
			}

			// Set category
			if benefit.ServiceType != "" {
				item.Category = m.mapServiceTypeToCategory(benefit.ServiceType, benefit.ServiceTypeDescription)
			}

			// Set network
			if benefit.InNetworkIndicator != "" {
				item.Network = m.mapNetworkIndicator(benefit.InNetworkIndicator)
			}

			// Set unit (individual/family)
			if benefit.CoverageLevel != "" {
				item.Unit = m.mapCoverageLevel(benefit.CoverageLevel)
			}

			// Check for authorization requirement
			item.AuthorizationRequired = benefit.AuthorizationRequired

			groups[key] = item
		}

		// Add benefit details
		cerBenefit := m.mapEligibilityBenefit(benefit)
		if cerBenefit != nil {
			item.Benefit = append(item.Benefit, *cerBenefit)
		}
	}

	// Convert map to slice
	var items []CERItem
	for _, item := range groups {
		items = append(items, *item)
	}
	return items
}

func (m *USCoreMapper) mapServiceTypeToCategory(code, description string) *CodeableConcept {
	// Map X12 service type codes to FHIR benefit categories
	var fhirCode, fhirDisplay string

	switch code {
	case "30":
		fhirCode, fhirDisplay = "30", "Health Benefit Plan Coverage"
	case "1":
		fhirCode, fhirDisplay = "1", "Medical Care"
	case "2":
		fhirCode, fhirDisplay = "2", "Surgical"
	case "3":
		fhirCode, fhirDisplay = "3", "Consultation"
	case "4":
		fhirCode, fhirDisplay = "4", "Diagnostic X-Ray"
	case "5":
		fhirCode, fhirDisplay = "5", "Diagnostic Lab"
	case "47":
		fhirCode, fhirDisplay = "47", "Hospital - Inpatient"
	case "48":
		fhirCode, fhirDisplay = "48", "Hospital - Outpatient"
	case "88":
		fhirCode, fhirDisplay = "88", "Emergency Services"
	case "89":
		fhirCode, fhirDisplay = "89", "Pharmacy"
	case "MH":
		fhirCode, fhirDisplay = "MH", "Mental Health"
	case "UC":
		fhirCode, fhirDisplay = "UC", "Urgent Care"
	default:
		fhirCode = code
		if description != "" {
			fhirDisplay = description
		} else {
			fhirDisplay = code
		}
	}

	return &CodeableConcept{
		Coding: []Coding{
			{
				System:  SystemEligibilityCategory,
				Code:    fhirCode,
				Display: fhirDisplay,
			},
		},
		Text: description,
	}
}

func (m *USCoreMapper) mapNetworkIndicator(indicator string) *CodeableConcept {
	var code, display string

	switch strings.ToUpper(indicator) {
	case "Y":
		code, display = "in", "In Network"
	case "N":
		code, display = "out", "Out of Network"
	default:
		code, display = "other", "Other"
	}

	return &CodeableConcept{
		Coding: []Coding{
			{
				System:  SystemBenefitNetwork,
				Code:    code,
				Display: display,
			},
		},
	}
}

func (m *USCoreMapper) mapCoverageLevel(level string) *CodeableConcept {
	var code, display string

	switch strings.ToUpper(level) {
	case "IND":
		code, display = "individual", "Individual"
	case "FAM":
		code, display = "family", "Family"
	case "CHD":
		code, display = "child", "Child"
	case "ESP":
		code, display = "spouse", "Spouse"
	case "EMP":
		code, display = "employee", "Employee"
	default:
		code, display = "individual", "Individual"
	}

	return &CodeableConcept{
		Coding: []Coding{
			{
				System:  SystemBenefitUnit,
				Code:    code,
				Display: display,
			},
		},
	}
}

func (m *USCoreMapper) mapEligibilityBenefit(benefit events.EligibilityBenefit) *CERBenefit {
	// Map information code (EB01) to benefit type
	benefitType := m.mapInformationCodeToBenefitType(benefit.InformationCode, benefit.InformationCodeDescription)

	cerBenefit := &CERBenefit{
		Type: benefitType,
	}

	// Set allowed values based on what's present
	switch benefit.InformationCode {
	case "A": // Coinsurance
		if benefit.Percent > 0 {
			pct := int(benefit.Percent)
			cerBenefit.AllowedUnsignedInt = &pct
			cerBenefit.AllowedString = fmt.Sprintf("%.0f%%", benefit.Percent)
		}
	case "B": // Copay
		if benefit.Amount > 0 {
			cerBenefit.AllowedMoney = &Money{Value: benefit.Amount, Currency: "USD"}
		}
	case "C": // Deductible
		if benefit.Amount > 0 {
			cerBenefit.AllowedMoney = &Money{Value: benefit.Amount, Currency: "USD"}
		}
	case "F": // Limitation - percent
		if benefit.Percent > 0 {
			pct := int(benefit.Percent)
			cerBenefit.AllowedUnsignedInt = &pct
		}
	case "G": // Limitation - quantity
		if benefit.Quantity > 0 {
			qty := int(benefit.Quantity)
			cerBenefit.AllowedUnsignedInt = &qty
		}
	default:
		// Generic amount handling
		if benefit.Amount > 0 {
			cerBenefit.AllowedMoney = &Money{Value: benefit.Amount, Currency: "USD"}
		} else if benefit.Percent > 0 {
			pct := int(benefit.Percent)
			cerBenefit.AllowedUnsignedInt = &pct
		} else if benefit.Quantity > 0 {
			qty := int(benefit.Quantity)
			cerBenefit.AllowedUnsignedInt = &qty
		}
	}

	// Only return if we have meaningful data
	if cerBenefit.AllowedMoney == nil && cerBenefit.AllowedUnsignedInt == nil && cerBenefit.AllowedString == "" {
		// Still return for coverage status codes (1, 6, 8)
		if benefit.InformationCode == "1" || benefit.InformationCode == "6" || benefit.InformationCode == "8" {
			cerBenefit.AllowedString = benefit.InformationCodeDescription
			return cerBenefit
		}
		return nil
	}

	return cerBenefit
}

func (m *USCoreMapper) mapInformationCodeToBenefitType(code, description string) CodeableConcept {
	var benefitCode, benefitDisplay string

	switch code {
	case "1":
		benefitCode, benefitDisplay = "benefit", "Active Coverage"
	case "6":
		benefitCode, benefitDisplay = "benefit", "Inactive Coverage"
	case "8":
		benefitCode, benefitDisplay = "benefit", "Not Covered"
	case "A":
		benefitCode, benefitDisplay = "coinsurance", "Coinsurance"
	case "B":
		benefitCode, benefitDisplay = "copay", "Co-Payment"
	case "C":
		benefitCode, benefitDisplay = "deductible", "Deductible"
	case "D":
		benefitCode, benefitDisplay = "benefit", "Benefit Description"
	case "E":
		benefitCode, benefitDisplay = "benefit", "Exclusions"
	case "F":
		benefitCode, benefitDisplay = "limitpercent", "Limitations - Percent"
	case "G":
		benefitCode, benefitDisplay = "limit", "Limitations - Quantity"
	case "H":
		benefitCode, benefitDisplay = "limit", "Unlimited"
	case "I":
		benefitCode, benefitDisplay = "benefit", "Non-Covered"
	case "J":
		benefitCode, benefitDisplay = "benefit", "Out of Pocket (Stop Loss)"
	case "K":
		benefitCode, benefitDisplay = "benefit", "Reserve"
	case "L":
		benefitCode, benefitDisplay = "benefit", "Primary Care Provider"
	case "M":
		benefitCode, benefitDisplay = "benefit", "Spend Down"
	case "N":
		benefitCode, benefitDisplay = "benefit", "Room"
	case "Y":
		benefitCode, benefitDisplay = "benefit", "Mental Health Provider"
	default:
		benefitCode, benefitDisplay = "benefit", "Benefit"
		if description != "" {
			benefitDisplay = description
		}
	}

	return CodeableConcept{
		Coding: []Coding{
			{
				System:  SystemBenefitType,
				Code:    benefitCode,
				Display: benefitDisplay,
			},
		},
		Text: description,
	}
}

func (m *USCoreMapper) mapEligibilityErrors(errors []events.EligibilityValidationError) []CERError {
	var cerErrors []CERError

	for _, err := range errors {
		cerErrors = append(cerErrors, CERError{
			Code: CodeableConcept{
				Coding: []Coding{
					{
						System:  SystemProcessingError,
						Code:    err.Code,
						Display: err.Message,
					},
				},
				Text: err.Message,
			},
		})
	}

	return cerErrors
}

// MapProcedure converts a canonical ProcedureEvent to a US Core Procedure.
func (m *USCoreMapper) MapProcedure(event *events.ProcedureEvent, patientRef string) *Procedure {
	if event == nil {
		return nil
	}

	procedure := &Procedure{
		ResourceType: "Procedure",
		ID:           event.ID,
		Meta: &Meta{
			Profile: []string{USCoreProcedureProfile},
		},
		Subject: &Reference{
			Reference: patientRef,
		},
	}

	// Map status (default to "completed" if not provided)
	procedure.Status = m.mapProcedureStatus(event.Procedure.Status)

	// Map code (required) - detect code system
	procedure.Code = m.mapProcedureCode(event.Procedure)

	// Map performed date
	if event.PerformedDate != "" {
		procedure.PerformedDateTime = event.PerformedDate
	}

	// Map performer
	if event.Performer != nil {
		procedure.Performer = m.mapProcedurePerformers(event.Performer)
	}

	// Map encounter reference
	if event.Encounter != nil && event.Encounter.ID != "" {
		procedure.Encounter = &Reference{
			Reference: fmt.Sprintf("Encounter/%s", event.Encounter.ID),
		}
	}

	// Map location
	if event.Location != nil {
		procedure.Location = m.mapProcedureLocation(event.Location)
	}

	return procedure
}

// mapProcedureStatus converts canonical status to FHIR Procedure status.
func (m *USCoreMapper) mapProcedureStatus(status string) string {
	switch strings.ToLower(status) {
	case "completed", "complete", "done":
		return "completed"
	case "in-progress", "inprogress", "active":
		return "in-progress"
	case "preparation", "prep", "scheduled":
		return "preparation"
	case "not-done", "notdone", "cancelled", "canceled":
		return "not-done"
	case "on-hold", "onhold", "paused":
		return "on-hold"
	case "stopped", "aborted":
		return "stopped"
	case "entered-in-error", "error":
		return "entered-in-error"
	case "unknown", "":
		return "completed" // Default to completed for historical data
	default:
		return "completed"
	}
}

// mapProcedureCode converts the canonical procedure code to FHIR CodeableConcept.
// Detects the code system based on format (CPT, SNOMED, ICD-10-PCS).
func (m *USCoreMapper) mapProcedureCode(proc events.Procedure) CodeableConcept {
	coding := Coding{
		Code:    proc.Code,
		Display: proc.Name,
	}

	// Determine code system
	if proc.CodeSystem != "" {
		coding.System = proc.CodeSystem
	} else {
		coding.System = m.detectProcedureCodeSystem(proc.Code)
	}

	return CodeableConcept{
		Coding: []Coding{coding},
		Text:   proc.Name,
	}
}

// detectProcedureCodeSystem attempts to identify the code system from the code format.
func (m *USCoreMapper) detectProcedureCodeSystem(code string) string {
	if code == "" {
		return SystemSNOMED // Default to SNOMED
	}

	// CPT codes: 5 digits, sometimes with modifiers
	// Pattern: NNNNN or NNNNN-NN
	if len(code) >= 5 && len(code) <= 7 {
		allDigits := true
		for i := 0; i < 5 && i < len(code); i++ {
			if code[i] < '0' || code[i] > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			// Check if it looks like a CPT code (range 00100-99999)
			if num, err := strconv.Atoi(code[:5]); err == nil && num >= 100 && num <= 99999 {
				return SystemCPT
			}
		}
	}

	// SNOMED codes: longer numeric codes, typically 6-18 digits
	if len(code) >= 6 {
		allDigits := true
		for _, ch := range code {
			if ch < '0' || ch > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return SystemSNOMED
		}
	}

	// ICD-10-PCS codes: 7 alphanumeric characters
	if len(code) == 7 {
		return SystemICD10PCS
	}

	// Default to SNOMED
	return SystemSNOMED
}

// mapProcedurePerformers converts a Provider to FHIR ProcedurePerformers.
func (m *USCoreMapper) mapProcedurePerformers(provider *events.Provider) []ProcedurePerformer {
	if provider == nil {
		return nil
	}

	displayName := m.buildProviderDisplayName(provider)
	ref := m.buildProviderReference(provider)

	return []ProcedurePerformer{
		{
			Actor: &Reference{
				Reference: ref,
				Display:   displayName,
			},
		},
	}
}

// buildProviderDisplayName creates a display name for a provider.
func (m *USCoreMapper) buildProviderDisplayName(provider *events.Provider) string {
	if provider.OrganizationName != "" {
		return provider.OrganizationName
	}
	if provider.FamilyName != "" {
		if provider.GivenName != "" {
			return fmt.Sprintf("%s, %s", provider.FamilyName, provider.GivenName)
		}
		return provider.FamilyName
	}
	return ""
}

// buildProviderReference creates a reference string for a provider.
func (m *USCoreMapper) buildProviderReference(provider *events.Provider) string {
	if provider.NPI != "" {
		return fmt.Sprintf("Practitioner/%s", provider.NPI)
	}
	if provider.ID != "" {
		return fmt.Sprintf("Practitioner/%s", provider.ID)
	}
	return "Practitioner/unknown"
}

// mapProcedureLocation converts a canonical Location to a FHIR Reference.
func (m *USCoreMapper) mapProcedureLocation(loc *events.Location) *Reference {
	if loc == nil {
		return nil
	}

	// Build a display name from location components
	var parts []string
	if loc.Facility != "" {
		parts = append(parts, loc.Facility)
	}
	if loc.Building != "" {
		parts = append(parts, loc.Building)
	}
	if loc.Unit != "" {
		parts = append(parts, loc.Unit)
	}

	display := strings.Join(parts, " - ")
	if display == "" {
		display = loc.Description
	}

	return &Reference{
		Display: display,
	}
}

// MapImmunization converts a canonical ImmunizationEvent to a US Core Immunization.
func (m *USCoreMapper) MapImmunization(event *events.ImmunizationEvent, patientRef string) *Immunization {
	if event == nil {
		return nil
	}

	immunization := &Immunization{
		ResourceType: "Immunization",
		ID:           event.ID,
		Meta: &Meta{
			Profile: []string{USCoreImmunizationProfile},
		},
		Patient: &Reference{
			Reference: patientRef,
		},
	}

	// Map status (required) - default to "completed"
	immunization.Status = m.mapImmunizationStatus(event.Immunization.Status)

	// Map vaccine code (required) - CVX code system
	immunization.VaccineCode = m.mapVaccineCode(event.Immunization)

	// Map occurrence date (required)
	if event.AdministeredDate != "" {
		immunization.OccurrenceDateTime = event.AdministeredDate
	}

	// Map lot number
	if event.Immunization.LotNumber != "" {
		immunization.LotNumber = event.Immunization.LotNumber
	}

	// Map site
	if event.Immunization.Site != "" {
		immunization.Site = m.mapImmunizationSite(event.Immunization.Site)
	}

	// Map route
	if event.Immunization.Route != "" {
		immunization.Route = m.mapImmunizationRoute(event.Immunization.Route)
	}

	// Map dose quantity
	if event.Immunization.DoseQuantity != "" {
		immunization.DoseQuantity = m.parseImmunizationDoseQuantity(event.Immunization.DoseQuantity)
	}

	// Map performer
	if event.Performer != nil {
		immunization.Performer = m.mapImmunizationPerformers(event.Performer)
	}

	// Map encounter reference
	if event.Encounter != nil && event.Encounter.ID != "" {
		immunization.Encounter = &Reference{
			Reference: fmt.Sprintf("Encounter/%s", event.Encounter.ID),
		}
	}

	// Map location
	if event.Location != nil {
		immunization.Location = m.mapProcedureLocation(event.Location)
	}

	// Primary source - default to true if not explicitly set
	primarySource := true
	immunization.PrimarySource = &primarySource

	return immunization
}

// mapImmunizationStatus converts canonical status to FHIR Immunization status.
func (m *USCoreMapper) mapImmunizationStatus(status string) string {
	switch strings.ToLower(status) {
	case "completed", "complete", "done", "given", "administered":
		return "completed"
	case "not-done", "notdone", "not_given", "refused", "contraindicated":
		return "not-done"
	case "entered-in-error", "error":
		return "entered-in-error"
	case "":
		return "completed" // Default
	default:
		return "completed"
	}
}

// mapVaccineCode converts the canonical immunization to a CVX-coded CodeableConcept.
func (m *USCoreMapper) mapVaccineCode(imm events.Immunization) CodeableConcept {
	coding := Coding{
		System:  SystemCVX,
		Code:    imm.VaccineCode,
		Display: imm.VaccineName,
	}

	return CodeableConcept{
		Coding: []Coding{coding},
		Text:   imm.VaccineName,
	}
}

// mapImmunizationSite converts site string to CodeableConcept.
// Uses SNOMED CT for body site codes.
func (m *USCoreMapper) mapImmunizationSite(site string) *CodeableConcept {
	// Map common site abbreviations to SNOMED CT codes
	siteMap := map[string]struct {
		code    string
		display string
	}{
		"LA":   {"72098002", "Left arm"},
		"RA":   {"59126009", "Right arm"},
		"LT":   {"61396006", "Left thigh"},
		"RT":   {"11207009", "Right thigh"},
		"LLFA": {"66480008", "Left lower forearm"},
		"RLFA": {"64262003", "Right lower forearm"},
		"LD":   {"46862004", "Left deltoid"},
		"RD":   {"91775009", "Right deltoid"},
		"LG":   {"85562004", "Left gluteal"},
		"RG":   {"78067005", "Right gluteal"},
		"LVL":  {"64688005", "Left vastus lateralis"},
		"RVL":  {"11207009", "Right vastus lateralis"},
	}

	if mapped, ok := siteMap[strings.ToUpper(site)]; ok {
		return &CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemSNOMED,
					Code:    mapped.code,
					Display: mapped.display,
				},
			},
			Text: mapped.display,
		}
	}

	// Return as text if not a known code
	return &CodeableConcept{
		Text: site,
	}
}

// mapImmunizationRoute converts route string to CodeableConcept.
// Uses NCI Thesaurus (NCIT) for route codes.
func (m *USCoreMapper) mapImmunizationRoute(route string) *CodeableConcept {
	const SystemNCIT = "http://ncimeta.nci.nih.gov"

	// Map common route abbreviations
	routeMap := map[string]struct {
		code    string
		display string
	}{
		"IM":    {"C28161", "Intramuscular"},
		"SC":    {"C38299", "Subcutaneous"},
		"SQ":    {"C38299", "Subcutaneous"},
		"ID":    {"C38238", "Intradermal"},
		"IV":    {"C38276", "Intravenous"},
		"PO":    {"C38288", "Oral"},
		"IN":    {"C38284", "Intranasal"},
		"TD":    {"C38305", "Transdermal"},
		"NASAL": {"C38284", "Intranasal"},
		"ORAL":  {"C38288", "Oral"},
	}

	if mapped, ok := routeMap[strings.ToUpper(route)]; ok {
		return &CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemNCIT,
					Code:    mapped.code,
					Display: mapped.display,
				},
			},
			Text: mapped.display,
		}
	}

	// Return as text if not a known code
	return &CodeableConcept{
		Text: route,
	}
}

// parseImmunizationDoseQuantity parses a dose quantity string.
func (m *USCoreMapper) parseImmunizationDoseQuantity(doseStr string) *Quantity {
	// Try to parse "value unit" format (e.g., "0.5 mL")
	parts := strings.Fields(doseStr)
	if len(parts) >= 1 {
		if val, err := strconv.ParseFloat(parts[0], 64); err == nil {
			unit := "mL" // Default unit
			if len(parts) >= 2 {
				unit = parts[1]
			}
			return &Quantity{
				Value:  val,
				Unit:   unit,
				System: SystemUCUM,
				Code:   strings.ToLower(unit),
			}
		}
	}

	// If parsing fails, return nil
	return nil
}

// mapImmunizationPerformers converts a Provider to FHIR ImmunizationPerformers.
func (m *USCoreMapper) mapImmunizationPerformers(provider *events.Provider) []ImmunizationPerformer {
	if provider == nil {
		return nil
	}

	displayName := m.buildProviderDisplayName(provider)
	ref := m.buildProviderReference(provider)

	return []ImmunizationPerformer{
		{
			Function: &CodeableConcept{
				Coding: []Coding{
					{
						System:  "http://terminology.hl7.org/CodeSystem/v2-0443",
						Code:    "AP",
						Display: "Administering Provider",
					},
				},
			},
			Actor: &Reference{
				Reference: ref,
				Display:   displayName,
			},
		},
	}
}

// MapVitalSign converts a canonical VitalSignEvent to a US Core Vital Signs Observation.
func (m *USCoreMapper) MapVitalSign(event *events.VitalSignEvent, patientRef string) *Observation {
	if event == nil {
		return nil
	}

	// Determine the appropriate US Core profile based on the vital sign type
	profile := m.determineVitalSignProfile(event.VitalSign.LOINCCode, event.VitalSign.Name)

	obs := &Observation{
		ResourceType: "Observation",
		ID:           event.ID,
		Meta: &Meta{
			Profile: []string{profile},
		},
		Status: "final", // Most vital signs are final when recorded
		Subject: &Reference{
			Reference: patientRef,
		},
	}

	// Set category to "vital-signs" (required for US Core)
	obs.Category = []CodeableConcept{
		{
			Coding: []Coding{
				{
					System:  SystemObservationCategory,
					Code:    VitalSignsCategory,
					Display: "Vital Signs",
				},
			},
		},
	}

	// Map the code (LOINC required for vital signs)
	obs.Code = m.mapVitalSignCode(event.VitalSign)

	// Map effective date time from event timestamp
	if !event.Timestamp.IsZero() {
		obs.EffectiveDateTime = event.Timestamp.Format("2006-01-02T15:04:05Z07:00")
	}

	// Map the value
	m.mapVitalSignValue(obs, event.VitalSign)

	// Map interpretation if present
	if event.VitalSign.Interpretation != "" {
		obs.Interpretation = m.mapVitalSignInterpretation(event.VitalSign.Interpretation)
	}

	// Map encounter reference if present
	if event.Encounter != nil && event.Encounter.ID != "" {
		obs.Encounter = &Reference{
			Reference: fmt.Sprintf("Encounter/%s", event.Encounter.ID),
		}
	}

	return obs
}

// determineVitalSignProfile returns the appropriate US Core vital signs profile.
func (m *USCoreMapper) determineVitalSignProfile(loincCode, name string) string {
	// Check LOINC code first for specific profiles
	switch loincCode {
	case LOINCHeartRate:
		return USCoreHeartRateProfile
	case LOINCRespiratoryRate:
		return USCoreRespiratoryRateProfile
	case LOINCBodyTemperature:
		return USCoreBodyTemperatureProfile
	case LOINCBodyHeight:
		return USCoreBodyHeightProfile
	case LOINCBodyWeight:
		return USCoreBodyWeightProfile
	case LOINCBodyMassIndex:
		return USCoreBMIProfile
	case LOINCOxygenSaturation, LOINCPulseOximetry:
		return USCorePulseOximetryProfile
	case LOINCBloodPressurePanel, LOINCSystolicBP, LOINCDiastolicBP:
		return USCoreBloodPressureProfile
	}

	// Fallback: try to determine from name
	nameLower := strings.ToLower(name)
	switch {
	case strings.Contains(nameLower, "heart rate") || strings.Contains(nameLower, "pulse"):
		return USCoreHeartRateProfile
	case strings.Contains(nameLower, "respiratory") || strings.Contains(nameLower, "breathing"):
		return USCoreRespiratoryRateProfile
	case strings.Contains(nameLower, "temperature") || strings.Contains(nameLower, "temp"):
		return USCoreBodyTemperatureProfile
	case strings.Contains(nameLower, "height") || strings.Contains(nameLower, "stature"):
		return USCoreBodyHeightProfile
	case strings.Contains(nameLower, "weight"):
		return USCoreBodyWeightProfile
	case strings.Contains(nameLower, "bmi") || strings.Contains(nameLower, "body mass"):
		return USCoreBMIProfile
	case strings.Contains(nameLower, "oxygen") || strings.Contains(nameLower, "o2 sat") || strings.Contains(nameLower, "spo2"):
		return USCorePulseOximetryProfile
	case strings.Contains(nameLower, "blood pressure") || strings.Contains(nameLower, "bp"):
		return USCoreBloodPressureProfile
	}

	// Default to base vital signs profile
	return USCoreVitalSignsProfile
}

// mapVitalSignCode creates a CodeableConcept for the vital sign.
func (m *USCoreMapper) mapVitalSignCode(vs events.VitalSign) CodeableConcept {
	coding := Coding{
		System:  SystemLOINC,
		Display: vs.Name,
	}

	// Use the provided LOINC code if available
	if vs.LOINCCode != "" {
		coding.Code = vs.LOINCCode
	} else {
		// Try to infer LOINC code from name
		coding.Code = m.inferVitalSignLOINCCode(vs.Name)
	}

	return CodeableConcept{
		Coding: []Coding{coding},
		Text:   vs.Name,
	}
}

// inferVitalSignLOINCCode attempts to determine LOINC code from vital sign name.
func (m *USCoreMapper) inferVitalSignLOINCCode(name string) string {
	nameLower := strings.ToLower(name)

	switch {
	case strings.Contains(nameLower, "heart rate") || strings.Contains(nameLower, "pulse rate"):
		return LOINCHeartRate
	case strings.Contains(nameLower, "respiratory rate") || strings.Contains(nameLower, "breathing rate"):
		return LOINCRespiratoryRate
	case strings.Contains(nameLower, "body temperature") || strings.Contains(nameLower, "temperature"):
		return LOINCBodyTemperature
	case strings.Contains(nameLower, "body height") || strings.Contains(nameLower, "height"):
		return LOINCBodyHeight
	case strings.Contains(nameLower, "body weight") || strings.Contains(nameLower, "weight"):
		return LOINCBodyWeight
	case strings.Contains(nameLower, "bmi") || strings.Contains(nameLower, "body mass index"):
		return LOINCBodyMassIndex
	case strings.Contains(nameLower, "oxygen saturation") || strings.Contains(nameLower, "spo2") || strings.Contains(nameLower, "o2 sat"):
		return LOINCPulseOximetry
	case strings.Contains(nameLower, "systolic"):
		return LOINCSystolicBP
	case strings.Contains(nameLower, "diastolic"):
		return LOINCDiastolicBP
	case strings.Contains(nameLower, "blood pressure"):
		return LOINCBloodPressurePanel
	case strings.Contains(nameLower, "head circumference"):
		return LOINCHeadCircumference
	}

	// Return empty if no match - caller should handle missing code
	return ""
}

// mapVitalSignValue maps the vital sign value to the observation.
func (m *USCoreMapper) mapVitalSignValue(obs *Observation, vs events.VitalSign) {
	if vs.Value == "" {
		return
	}

	// Try to parse as a numeric value
	if val, err := strconv.ParseFloat(vs.Value, 64); err == nil {
		ucumCode := m.mapVitalSignUnitToUCUM(vs.Unit, vs.LOINCCode)
		obs.ValueQuantity = &Quantity{
			Value:  val,
			Unit:   vs.Unit,
			System: SystemUCUM,
			Code:   ucumCode,
		}
	} else {
		// Non-numeric value - store as string
		obs.ValueString = vs.Value
	}
}

// mapVitalSignUnitToUCUM converts common unit strings to UCUM codes.
func (m *USCoreMapper) mapVitalSignUnitToUCUM(unit, loincCode string) string {
	unitLower := strings.ToLower(unit)

	// Common vital signs unit mappings to UCUM
	unitMap := map[string]string{
		// Temperature
		"°c":         "Cel",
		"°f":         "[degF]",
		"celsius":    "Cel",
		"fahrenheit": "[degF]",
		"c":          "Cel",
		"f":          "[degF]",

		// Heart/Respiratory rate
		"bpm":         "/min",
		"beats/min":   "/min",
		"breaths/min": "/min",
		"/min":        "/min",

		// Height
		"cm":     "cm",
		"in":     "[in_i]",
		"inches": "[in_i]",
		"m":      "m",
		"ft":     "[ft_i]",
		"feet":   "[ft_i]",

		// Weight
		"kg":     "kg",
		"lb":     "[lb_av]",
		"lbs":    "[lb_av]",
		"pounds": "[lb_av]",
		"oz":     "[oz_av]",
		"g":      "g",

		// Blood pressure
		"mmhg":  "mm[Hg]",
		"mm hg": "mm[Hg]",

		// Oxygen saturation
		"%":       "%",
		"percent": "%",

		// BMI
		"kg/m2":  "kg/m2",
		"kg/m^2": "kg/m2",
	}

	if ucum, ok := unitMap[unitLower]; ok {
		return ucum
	}

	// If unit looks like a valid UCUM code already, return as-is
	if unit != "" {
		return unit
	}

	// Default based on LOINC code
	switch loincCode {
	case LOINCHeartRate, LOINCRespiratoryRate:
		return "/min"
	case LOINCBodyTemperature:
		return "Cel"
	case LOINCBodyHeight:
		return "cm"
	case LOINCBodyWeight:
		return "kg"
	case LOINCBodyMassIndex:
		return "kg/m2"
	case LOINCOxygenSaturation, LOINCPulseOximetry:
		return "%"
	case LOINCSystolicBP, LOINCDiastolicBP:
		return "mm[Hg]"
	}

	return unit
}

// mapVitalSignInterpretation maps interpretation codes to FHIR CodeableConcepts.
func (m *USCoreMapper) mapVitalSignInterpretation(interpretation string) []CodeableConcept {
	interpLower := strings.ToLower(interpretation)

	// Map common interpretation strings to HL7 v3 ObservationInterpretation codes
	interpretationMap := map[string]struct {
		code    string
		display string
	}{
		"normal":        {"N", "Normal"},
		"n":             {"N", "Normal"},
		"high":          {"H", "High"},
		"h":             {"H", "High"},
		"low":           {"L", "Low"},
		"l":             {"L", "Low"},
		"critical":      {"AA", "Critical abnormal"},
		"critical high": {"HH", "Critical high"},
		"critical low":  {"LL", "Critical low"},
		"hh":            {"HH", "Critical high"},
		"ll":            {"LL", "Critical low"},
		"abnormal":      {"A", "Abnormal"},
		"a":             {"A", "Abnormal"},
		"very high":     {"HH", "Critical high"},
		"very low":      {"LL", "Critical low"},
		"panic":         {"AA", "Critical abnormal"},
		"panic high":    {"HH", "Critical high"},
		"panic low":     {"LL", "Critical low"},
	}

	if mapped, ok := interpretationMap[interpLower]; ok {
		return []CodeableConcept{
			{
				Coding: []Coding{
					{
						System:  SystemInterpretation,
						Code:    mapped.code,
						Display: mapped.display,
					},
				},
			},
		}
	}

	// Return as text if not a known code
	return []CodeableConcept{
		{
			Text: interpretation,
		},
	}
}

// MapMedicationRequest converts a canonical MedicationRequestEvent to a US Core MedicationRequest.
func (m *USCoreMapper) MapMedicationRequest(event *events.MedicationRequestEvent, patientRef string) *MedicationRequest {
	if event == nil {
		return nil
	}

	req := event.MedicationRequest
	med := req.Medication

	medReq := &MedicationRequest{
		ID: event.ID,
		Meta: &Meta{
			Profile: []string{USCoreMedicationRequestProfile},
		},
		Status: m.mapMedicationRequestStatus(req.Status),
		Intent: m.mapMedicationRequestIntent(req.Intent),
		Subject: &Reference{
			Reference: patientRef,
		},
	}

	// Map medication (required by US Core)
	medReq.MedicationCodeableConcept = m.mapMedicationCode(med)

	// Map authored date
	if req.AuthoredOn != "" {
		medReq.AuthoredOn = req.AuthoredOn
	}

	// Map prescriber/requester
	if event.Prescriber != nil {
		ref := &Reference{}
		if event.Prescriber.NPI != "" {
			ref.Reference = "Practitioner/" + event.Prescriber.NPI
		} else if event.Prescriber.ID != "" {
			ref.Reference = "Practitioner/" + event.Prescriber.ID
		}
		// Build display name from components
		if event.Prescriber.GivenName != "" || event.Prescriber.FamilyName != "" {
			displayParts := []string{}
			if event.Prescriber.Prefix != "" {
				displayParts = append(displayParts, event.Prescriber.Prefix)
			}
			if event.Prescriber.GivenName != "" {
				displayParts = append(displayParts, event.Prescriber.GivenName)
			}
			if event.Prescriber.FamilyName != "" {
				displayParts = append(displayParts, event.Prescriber.FamilyName)
			}
			if event.Prescriber.Suffix != "" {
				displayParts = append(displayParts, event.Prescriber.Suffix)
			}
			ref.Display = strings.Join(displayParts, " ")
		}
		medReq.Requester = ref
	}

	// Map encounter reference
	if event.Encounter != nil && event.Encounter.ID != "" {
		medReq.Encounter = &Reference{
			Reference: "Encounter/" + event.Encounter.ID,
		}
	}

	// Map dosage instructions
	dosage := m.mapDosageInstruction(req)
	if dosage != nil {
		medReq.DosageInstruction = []Dosage{*dosage}
	}

	// Map dispense request
	dispenseReq := m.mapDispenseRequest(req, event.PharmacyID)
	if dispenseReq != nil {
		medReq.DispenseRequest = dispenseReq
	}

	// Map substitution rules
	if !req.Substitution {
		medReq.Substitution = &MedSubstitution{
			AllowedBoolean: false,
		}
	}

	// Map reason code
	if req.ReasonCode != "" || req.ReasonText != "" {
		reasonCode := &CodeableConcept{}
		if req.ReasonCode != "" {
			// Detect code system from format (ICD-10 vs SNOMED)
			system := SystemSNOMED // Default
			if len(req.ReasonCode) >= 3 && (req.ReasonCode[0] >= 'A' && req.ReasonCode[0] <= 'Z') {
				// ICD-10-CM codes start with a letter and have format like A00.0
				system = SystemICD10CM
			}
			reasonCode.Coding = []Coding{
				{
					System: system,
					Code:   req.ReasonCode,
				},
			}
		}
		if req.ReasonText != "" {
			reasonCode.Text = req.ReasonText
		}
		medReq.ReasonCode = []CodeableConcept{*reasonCode}
	}

	// Map category (outpatient, inpatient, community, discharge)
	category := m.inferMedicationRequestCategory(event)
	if category != "" {
		medReq.Category = []CodeableConcept{
			{
				Coding: []Coding{
					{
						System:  SystemMedicationRequestCategory,
						Code:    category,
						Display: m.medicationCategoryDisplay(category),
					},
				},
			},
		}
	}

	return medReq
}

// mapMedicationRequestStatus maps input status to FHIR MedicationRequest status.
func (m *USCoreMapper) mapMedicationRequestStatus(status string) string {
	statusLower := strings.ToLower(strings.TrimSpace(status))
	statusMap := map[string]string{
		"active":           "active",
		"completed":        "completed",
		"cancelled":        "cancelled",
		"canceled":         "cancelled",
		"stopped":          "stopped",
		"draft":            "draft",
		"on-hold":          "on-hold",
		"on hold":          "on-hold",
		"onhold":           "on-hold",
		"entered-in-error": "entered-in-error",
		"error":            "entered-in-error",
		"unknown":          "unknown",
	}
	if mapped, ok := statusMap[statusLower]; ok {
		return mapped
	}
	// Default to active for prescriptions
	if status == "" {
		return "active"
	}
	return "unknown"
}

// mapMedicationRequestIntent maps input intent to FHIR MedicationRequest intent.
func (m *USCoreMapper) mapMedicationRequestIntent(intent string) string {
	intentLower := strings.ToLower(strings.TrimSpace(intent))
	intentMap := map[string]string{
		"order":          "order",
		"proposal":       "proposal",
		"plan":           "plan",
		"original-order": "original-order",
		"reflex-order":   "reflex-order",
		"filler-order":   "filler-order",
		"instance-order": "instance-order",
		"option":         "option",
	}
	if mapped, ok := intentMap[intentLower]; ok {
		return mapped
	}
	// Default to order for prescriptions
	return "order"
}

// mapMedicationCode maps medication info to a CodeableConcept with RxNorm.
func (m *USCoreMapper) mapMedicationCode(med events.Medication) *CodeableConcept {
	cc := &CodeableConcept{}

	if med.Code != "" {
		system := SystemRxNorm // Default to RxNorm
		if med.CodeSystem != "" {
			system = med.CodeSystem
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    med.Code,
				Display: med.Name,
			},
		}
	}

	// Build display text with form and strength if available
	displayText := med.Name
	if med.Strength != "" {
		displayText += " " + med.Strength
	}
	if med.Form != "" {
		displayText += " " + med.Form
	}
	cc.Text = strings.TrimSpace(displayText)

	return cc
}

// mapDosageInstruction maps medication request dosage info to FHIR Dosage.
func (m *USCoreMapper) mapDosageInstruction(req events.MedicationRequest) *Dosage {
	// Only create dosage if we have some information
	if req.DosageInstruction == "" && req.DoseQuantity == "" && req.Frequency == "" && req.Route == "" {
		return nil
	}

	dosage := &Dosage{
		Sequence: 1,
	}

	// Free text sig (most important)
	if req.DosageInstruction != "" {
		dosage.Text = req.DosageInstruction
	} else {
		// Build a sig from structured data
		dosage.Text = m.buildSigText(req)
	}

	// Dose quantity
	if req.DoseQuantity != "" {
		qty := m.parseDoseQuantity(req.DoseQuantity, req.DoseUnit)
		if qty != nil {
			dosage.DoseAndRate = []DoseAndRate{
				{
					DoseQuantity: qty,
				},
			}
		}
	}

	// Route
	if req.Route != "" {
		dosage.Route = m.mapAdministrationRoute(req.Route)
	}

	// Timing/frequency
	if req.Frequency != "" {
		dosage.Timing = m.mapFrequencyToTiming(req.Frequency)
	}

	return dosage
}

// buildSigText builds a sig from structured data.
func (m *USCoreMapper) buildSigText(req events.MedicationRequest) string {
	var parts []string

	if req.DoseQuantity != "" {
		dose := req.DoseQuantity
		if req.DoseUnit != "" {
			dose += " " + req.DoseUnit
		}
		parts = append(parts, "Take "+dose)
	}

	if req.Route != "" {
		parts = append(parts, "by "+strings.ToLower(req.Route))
	}

	if req.Frequency != "" {
		parts = append(parts, req.Frequency)
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " ")
}

// parseDoseQuantity parses a dose quantity string into a Quantity.
func (m *USCoreMapper) parseDoseQuantity(doseStr, unit string) *Quantity {
	doseStr = strings.TrimSpace(doseStr)
	if doseStr == "" {
		return nil
	}

	// Try to parse as number
	var value float64
	_, err := fmt.Sscanf(doseStr, "%f", &value)
	if err != nil {
		// If parsing fails, might be text like "1-2"
		return &Quantity{
			Unit: unit,
		}
	}

	qty := &Quantity{
		Value: value,
		Unit:  unit,
	}

	// Map common units to UCUM
	ucumUnit := m.mapMedicationUnitToUCUM(unit)
	if ucumUnit != "" {
		qty.System = SystemUCUM
		qty.Code = ucumUnit
	}

	return qty
}

// mapMedicationUnitToUCUM maps common medication units to UCUM codes.
func (m *USCoreMapper) mapMedicationUnitToUCUM(unit string) string {
	unitLower := strings.ToLower(strings.TrimSpace(unit))
	unitMap := map[string]string{
		"mg":      "mg",
		"g":       "g",
		"mcg":     "ug",
		"ml":      "mL",
		"l":       "L",
		"tablet":  "{tbl}",
		"tablets": "{tbl}",
		"tab":     "{tbl}",
		"capsule": "{cap}",
		"cap":     "{cap}",
		"patch":   "{patch}",
		"puff":    "{puff}",
		"puffs":   "{puff}",
		"drop":    "{drop}",
		"drops":   "{drop}",
		"unit":    "[iU]",
		"units":   "[iU]",
		"iu":      "[iU]",
	}
	return unitMap[unitLower]
}

// mapAdministrationRoute maps route text to a CodeableConcept (SNOMED CT).
func (m *USCoreMapper) mapAdministrationRoute(route string) *CodeableConcept {
	routeLower := strings.ToLower(strings.TrimSpace(route))

	// SNOMED CT route codes
	routeMap := map[string]struct {
		code    string
		display string
	}{
		"oral":          {"26643006", "Oral route"},
		"po":            {"26643006", "Oral route"},
		"by mouth":      {"26643006", "Oral route"},
		"sublingual":    {"37839007", "Sublingual route"},
		"sl":            {"37839007", "Sublingual route"},
		"intravenous":   {"47625008", "Intravenous route"},
		"iv":            {"47625008", "Intravenous route"},
		"intramuscular": {"78421000", "Intramuscular route"},
		"im":            {"78421000", "Intramuscular route"},
		"subcutaneous":  {"34206005", "Subcutaneous route"},
		"subq":          {"34206005", "Subcutaneous route"},
		"sc":            {"34206005", "Subcutaneous route"},
		"topical":       {"6064005", "Topical route"},
		"rectal":        {"37161004", "Rectal route"},
		"pr":            {"37161004", "Rectal route"},
		"inhalation":    {"447694001", "Respiratory tract route"},
		"inhaled":       {"447694001", "Respiratory tract route"},
		"nasal":         {"46713006", "Nasal route"},
		"ophthalmic":    {"54485002", "Ophthalmic route"},
		"otic":          {"10547007", "Otic route"},
		"vaginal":       {"16857009", "Vaginal route"},
		"transdermal":   {"45890007", "Transdermal route"},
	}

	if mapped, ok := routeMap[routeLower]; ok {
		return &CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemSNOMED,
					Code:    mapped.code,
					Display: mapped.display,
				},
			},
			Text: route,
		}
	}

	return &CodeableConcept{Text: route}
}

// mapFrequencyToTiming maps frequency abbreviations to Timing.
func (m *USCoreMapper) mapFrequencyToTiming(freq string) *Timing {
	freqUpper := strings.ToUpper(strings.TrimSpace(freq))

	// Map common frequency abbreviations
	freqMap := map[string]struct {
		code       string
		frequency  int
		period     float64
		periodUnit string
	}{
		"QD":    {"QD", 1, 1, "d"},
		"DAILY": {"QD", 1, 1, "d"},
		"BID":   {"BID", 2, 1, "d"},
		"TID":   {"TID", 3, 1, "d"},
		"QID":   {"QID", 4, 1, "d"},
		"Q4H":   {"Q4H", 1, 4, "h"},
		"Q6H":   {"Q6H", 1, 6, "h"},
		"Q8H":   {"Q8H", 1, 8, "h"},
		"Q12H":  {"Q12H", 1, 12, "h"},
		"QHS":   {"QHS", 1, 1, "d"}, // At bedtime
		"PRN":   {"PRN", 0, 0, ""},  // As needed
		"STAT":  {"STAT", 1, 0, ""}, // Immediately
		"QW":    {"QW", 1, 1, "wk"}, // Weekly
		"QOD":   {"QOD", 1, 2, "d"}, // Every other day
	}

	timing := &Timing{}

	if mapped, ok := freqMap[freqUpper]; ok {
		timing.Code = &CodeableConcept{
			Coding: []Coding{
				{
					System: SystemTimingAbbreviation,
					Code:   mapped.code,
				},
			},
			Text: freq,
		}

		if mapped.frequency > 0 && mapped.period > 0 {
			timing.Repeat = &TimingRepeat{
				Frequency:  mapped.frequency,
				Period:     mapped.period,
				PeriodUnit: mapped.periodUnit,
			}
		}

		// Handle PRN (as needed)
		if freqUpper == "PRN" {
			return timing
		}
	} else {
		// Just store as text
		timing.Code = &CodeableConcept{Text: freq}
	}

	return timing
}

// mapDispenseRequest maps dispense info to FHIR DispenseRequest.
func (m *USCoreMapper) mapDispenseRequest(req events.MedicationRequest, pharmacyID string) *DispenseRequest {
	if req.DispenseQuantity == 0 && req.DaysSupply == 0 && req.NumberOfRefills == 0 && pharmacyID == "" {
		return nil
	}

	dispReq := &DispenseRequest{}

	if req.DispenseQuantity > 0 {
		dispReq.Quantity = &Quantity{
			Value: req.DispenseQuantity,
		}
		if req.DispenseUnit != "" {
			dispReq.Quantity.Unit = req.DispenseUnit
			ucum := m.mapMedicationUnitToUCUM(req.DispenseUnit)
			if ucum != "" {
				dispReq.Quantity.System = SystemUCUM
				dispReq.Quantity.Code = ucum
			}
		}
	}

	if req.DaysSupply > 0 {
		daysFloat := float64(req.DaysSupply)
		dispReq.ExpectedSupplyDuration = &Duration{
			Value:  daysFloat,
			Unit:   "days",
			System: SystemUCUM,
			Code:   "d",
		}
	}

	if req.NumberOfRefills > 0 {
		dispReq.NumberOfRepeatsAllowed = req.NumberOfRefills
	}

	if pharmacyID != "" {
		dispReq.Performer = &Reference{
			Reference: "Organization/" + pharmacyID,
		}
	}

	return dispReq
}

// inferMedicationRequestCategory infers the medication category from context.
func (m *USCoreMapper) inferMedicationRequestCategory(event *events.MedicationRequestEvent) string {
	// If there's an encounter, check its class
	if event.Encounter != nil && event.Encounter.Class != "" {
		classLower := strings.ToLower(event.Encounter.Class)
		if strings.Contains(classLower, "inpatient") || classLower == "imp" {
			return "inpatient"
		}
		if strings.Contains(classLower, "outpatient") || classLower == "amb" {
			return "outpatient"
		}
		if strings.Contains(classLower, "discharge") {
			return "discharge"
		}
	}
	// Default to community (retail pharmacy)
	return "community"
}

// medicationCategoryDisplay returns the display text for a medication category.
func (m *USCoreMapper) medicationCategoryDisplay(category string) string {
	displayMap := map[string]string{
		"inpatient":  "Inpatient",
		"outpatient": "Outpatient",
		"community":  "Community",
		"discharge":  "Discharge",
	}
	if display, ok := displayMap[category]; ok {
		return display
	}
	return category
}

// MapAllergyIntolerance converts a canonical AllergyIntoleranceEvent to a US Core AllergyIntolerance.
func (m *USCoreMapper) MapAllergyIntolerance(event *events.AllergyIntoleranceEvent, patientRef string) *AllergyIntolerance {
	if event == nil {
		return nil
	}

	allergy := event.AllergyIntolerance

	ai := &AllergyIntolerance{
		ID: event.ID,
		Meta: &Meta{
			Profile: []string{USCoreAllergyIntoleranceProfile},
		},
		Patient: &Reference{
			Reference: patientRef,
		},
	}

	// Map allergen code (required by US Core)
	ai.Code = m.mapAllergenCode(allergy)

	// Map clinical status
	if allergy.ClinicalStatus != "" {
		ai.ClinicalStatus = m.mapAllergyClinicalStatus(allergy.ClinicalStatus)
	}

	// Map verification status
	if allergy.VerificationStatus != "" {
		ai.VerificationStatus = m.mapAllergyVerificationStatus(allergy.VerificationStatus)
	}

	// Map type (allergy vs intolerance)
	if allergy.Type != "" {
		ai.Type = m.mapAllergyType(allergy.Type)
	}

	// Map category
	if allergy.Category != "" {
		ai.Category = []string{m.mapAllergyCategory(allergy.Category)}
	}

	// Map criticality
	if allergy.Criticality != "" {
		ai.Criticality = m.mapAllergyCriticality(allergy.Criticality)
	}

	// Map onset date
	if allergy.OnsetDate != "" {
		ai.OnsetDateTime = allergy.OnsetDate
	}

	// Map recorded date
	if allergy.RecordedDate != "" {
		ai.RecordedDate = allergy.RecordedDate
	}

	// Map recorder (who recorded this)
	if event.Recorder != nil {
		ref := &Reference{}
		if event.Recorder.NPI != "" {
			ref.Reference = "Practitioner/" + event.Recorder.NPI
		} else if event.Recorder.ID != "" {
			ref.Reference = "Practitioner/" + event.Recorder.ID
		}
		// Build display name from components
		if event.Recorder.GivenName != "" || event.Recorder.FamilyName != "" {
			displayParts := []string{}
			if event.Recorder.Prefix != "" {
				displayParts = append(displayParts, event.Recorder.Prefix)
			}
			if event.Recorder.GivenName != "" {
				displayParts = append(displayParts, event.Recorder.GivenName)
			}
			if event.Recorder.FamilyName != "" {
				displayParts = append(displayParts, event.Recorder.FamilyName)
			}
			if event.Recorder.Suffix != "" {
				displayParts = append(displayParts, event.Recorder.Suffix)
			}
			ref.Display = strings.Join(displayParts, " ")
		}
		ai.Recorder = ref
	}

	// Map encounter reference
	if event.Encounter != nil && event.Encounter.ID != "" {
		ai.Encounter = &Reference{
			Reference: "Encounter/" + event.Encounter.ID,
		}
	}

	// Map reactions
	if len(allergy.Reactions) > 0 {
		ai.Reaction = m.mapAllergyReactions(allergy.Reactions)
	}

	return ai
}

// mapAllergenCode maps allergen info to a CodeableConcept.
func (m *USCoreMapper) mapAllergenCode(allergy events.AllergyIntolerance) *CodeableConcept {
	cc := &CodeableConcept{}

	if allergy.Code != "" {
		// Determine code system - prefer RxNorm for medications, SNOMED for others
		system := allergy.CodeSystem
		if system == "" {
			// Try to infer based on category
			if strings.ToLower(allergy.Category) == "medication" {
				system = SystemRxNorm
			} else {
				system = SystemSNOMED
			}
		}

		cc.Coding = []Coding{
			{
				System:  system,
				Code:    allergy.Code,
				Display: allergy.Name,
			},
		}
	}

	if allergy.Name != "" {
		cc.Text = allergy.Name
	}

	return cc
}

// mapAllergyClinicalStatus maps clinical status to CodeableConcept.
func (m *USCoreMapper) mapAllergyClinicalStatus(status string) *CodeableConcept {
	statusLower := strings.ToLower(strings.TrimSpace(status))
	statusMap := map[string]string{
		"active":   "active",
		"inactive": "inactive",
		"resolved": "resolved",
	}

	code := statusMap[statusLower]
	if code == "" {
		code = "active" // Default
	}

	return &CodeableConcept{
		Coding: []Coding{
			{
				System: SystemAllergyIntoleranceClinicalStatus,
				Code:   code,
			},
		},
	}
}

// mapAllergyVerificationStatus maps verification status to CodeableConcept.
func (m *USCoreMapper) mapAllergyVerificationStatus(status string) *CodeableConcept {
	statusLower := strings.ToLower(strings.TrimSpace(status))
	statusMap := map[string]string{
		"unconfirmed":      "unconfirmed",
		"confirmed":        "confirmed",
		"refuted":          "refuted",
		"entered-in-error": "entered-in-error",
		"error":            "entered-in-error",
	}

	code := statusMap[statusLower]
	if code == "" {
		code = "unconfirmed" // Default
	}

	return &CodeableConcept{
		Coding: []Coding{
			{
				System: SystemAllergyIntoleranceVerification,
				Code:   code,
			},
		},
	}
}

// mapAllergyType maps allergy type to FHIR allergy type.
func (m *USCoreMapper) mapAllergyType(allergyType string) string {
	typeLower := strings.ToLower(strings.TrimSpace(allergyType))
	if typeLower == "allergy" || typeLower == "true allergy" {
		return "allergy"
	}
	if typeLower == "intolerance" {
		return "intolerance"
	}
	return "allergy" // Default
}

// mapAllergyCategory maps category to FHIR allergy category.
func (m *USCoreMapper) mapAllergyCategory(category string) string {
	catLower := strings.ToLower(strings.TrimSpace(category))
	catMap := map[string]string{
		"food":        "food",
		"medication":  "medication",
		"drug":        "medication",
		"medicine":    "medication",
		"environment": "environment",
		"biologic":    "biologic",
	}
	if mapped, ok := catMap[catLower]; ok {
		return mapped
	}
	return category
}

// mapAllergyCriticality maps criticality to FHIR criticality.
func (m *USCoreMapper) mapAllergyCriticality(criticality string) string {
	critLower := strings.ToLower(strings.TrimSpace(criticality))
	critMap := map[string]string{
		"low":              "low",
		"high":             "high",
		"unable-to-assess": "unable-to-assess",
		"unable to assess": "unable-to-assess",
		"unknown":          "unable-to-assess",
		"critical":         "high",
		"life-threatening": "high",
		"life threatening": "high",
	}
	if mapped, ok := critMap[critLower]; ok {
		return mapped
	}
	return "unable-to-assess"
}

// mapAllergyReactions maps allergy reactions to FHIR AllergyReaction.
func (m *USCoreMapper) mapAllergyReactions(reactions []events.AllergyReaction) []AllergyReaction {
	var fhirReactions []AllergyReaction

	for _, r := range reactions {
		fhirReaction := AllergyReaction{}

		// Manifestation is required in FHIR
		manifestation := m.mapReactionManifestation(r.Manifestation, r.ManifestationText)
		fhirReaction.Manifestation = manifestation

		// Substance (if different from main allergen)
		if r.Substance != "" {
			fhirReaction.Substance = &CodeableConcept{
				Text: r.Substance,
			}
		}

		// Severity
		if r.Severity != "" {
			fhirReaction.Severity = m.mapReactionSeverity(r.Severity)
		}

		// Description
		if r.Note != "" {
			fhirReaction.Description = r.Note
		}

		// Onset
		if r.OnsetDate != "" {
			fhirReaction.Onset = r.OnsetDate
		}

		fhirReactions = append(fhirReactions, fhirReaction)
	}

	return fhirReactions
}

// mapReactionManifestation maps reaction manifestation to CodeableConcept.
func (m *USCoreMapper) mapReactionManifestation(code, text string) []CodeableConcept {
	var manifestations []CodeableConcept

	cc := &CodeableConcept{}
	if code != "" {
		// Try to map common manifestations to SNOMED CT
		mapped := m.lookupManifestationCode(code)
		if mapped != nil {
			cc.Coding = []Coding{*mapped}
		}
	}

	if text != "" {
		cc.Text = text
	} else if code != "" {
		cc.Text = code
	}

	manifestations = append(manifestations, *cc)
	return manifestations
}

// lookupManifestationCode maps common reaction manifestations to SNOMED CT.
func (m *USCoreMapper) lookupManifestationCode(manifestation string) *Coding {
	manifLower := strings.ToLower(strings.TrimSpace(manifestation))

	// Common reaction manifestations (SNOMED CT)
	manifMap := map[string]struct {
		code    string
		display string
	}{
		"rash":                 {"271807003", "Rash"},
		"hives":                {"126485001", "Urticaria"},
		"urticaria":            {"126485001", "Urticaria"},
		"itching":              {"418290006", "Itching"},
		"pruritus":             {"418290006", "Itching"},
		"swelling":             {"65124004", "Swelling"},
		"angioedema":           {"41291007", "Angioedema"},
		"anaphylaxis":          {"39579001", "Anaphylaxis"},
		"anaphylactic shock":   {"39579001", "Anaphylaxis"},
		"nausea":               {"422587007", "Nausea"},
		"vomiting":             {"422400008", "Vomiting"},
		"diarrhea":             {"62315008", "Diarrhea"},
		"difficulty breathing": {"267036007", "Dyspnea"},
		"dyspnea":              {"267036007", "Dyspnea"},
		"wheezing":             {"56018004", "Wheezing"},
		"throat swelling":      {"262577005", "Throat swelling"},
		"headache":             {"25064002", "Headache"},
	}

	if mapped, ok := manifMap[manifLower]; ok {
		return &Coding{
			System:  SystemSNOMED,
			Code:    mapped.code,
			Display: mapped.display,
		}
	}

	return nil
}

// mapReactionSeverity maps reaction severity to FHIR severity code.
func (m *USCoreMapper) mapReactionSeverity(severity string) string {
	sevLower := strings.ToLower(strings.TrimSpace(severity))
	sevMap := map[string]string{
		"mild":     "mild",
		"moderate": "moderate",
		"severe":   "severe",
		"minor":    "mild",
		"major":    "severe",
		"serious":  "severe",
	}
	if mapped, ok := sevMap[sevLower]; ok {
		return mapped
	}
	return "moderate" // Default
}

// ============================================================================
// CarePlan Mapping (US Core 6.1.0)
// ============================================================================

// MapCarePlan converts a canonical CarePlanEvent to a US Core CarePlan.
func (m *USCoreMapper) MapCarePlan(event *events.CarePlanEvent, patientRef string) *CarePlan {
	if event == nil {
		return nil
	}

	cp := event.CarePlan

	carePlan := &CarePlan{
		Meta: &Meta{
			Profile: []string{USCoreCarePlanProfile},
		},
		Status:  m.mapCarePlanStatus(cp.Status),
		Intent:  m.mapCarePlanIntent(cp.Intent),
		Subject: &Reference{Reference: patientRef},
	}

	// Category is required by US Core - must include "assess-plan"
	carePlan.Category = m.mapCarePlanCategory(cp.Category)

	// Title (optional but recommended)
	if cp.Title != "" {
		carePlan.Title = cp.Title
	}

	// Description (optional)
	if cp.Description != "" {
		carePlan.Description = cp.Description
	}

	// Period
	if cp.PeriodStart != "" || cp.PeriodEnd != "" {
		carePlan.Period = &Period{}
		if cp.PeriodStart != "" {
			if t, err := time.Parse("2006-01-02", cp.PeriodStart); err == nil {
				carePlan.Period.Start = &t
			} else if t, err := time.Parse(time.RFC3339, cp.PeriodStart); err == nil {
				carePlan.Period.Start = &t
			}
		}
		if cp.PeriodEnd != "" {
			if t, err := time.Parse("2006-01-02", cp.PeriodEnd); err == nil {
				carePlan.Period.End = &t
			} else if t, err := time.Parse(time.RFC3339, cp.PeriodEnd); err == nil {
				carePlan.Period.End = &t
			}
		}
	}

	// Created timestamp from event
	if !event.Timestamp.IsZero() {
		carePlan.Created = event.Timestamp.Format(time.RFC3339)
	}

	// Author
	if event.Author != nil {
		refStr := m.buildProviderReference(event.Author)
		carePlan.Author = &Reference{
			Reference: refStr,
			Display:   m.buildProviderDisplayName(event.Author),
		}
	}

	// Care team
	if len(event.CareTeam) > 0 {
		for _, provider := range event.CareTeam {
			if provider != nil {
				refStr := m.buildProviderReference(provider)
				carePlan.CareTeam = append(carePlan.CareTeam, Reference{
					Reference: refStr,
					Display:   m.buildProviderDisplayName(provider),
				})
			}
		}
	}

	// Encounter reference
	if event.Encounter != nil && event.Encounter.ID != "" {
		carePlan.Encounter = &Reference{
			Reference: "Encounter/" + event.Encounter.ID,
		}
	}

	// Goal references
	if len(cp.GoalIDs) > 0 {
		for _, goalID := range cp.GoalIDs {
			carePlan.Goal = append(carePlan.Goal, Reference{
				Reference: "Goal/" + goalID,
			})
		}
	}

	// Condition references (addresses)
	if len(cp.ConditionIDs) > 0 {
		for _, condID := range cp.ConditionIDs {
			carePlan.Addresses = append(carePlan.Addresses, Reference{
				Reference: "Condition/" + condID,
			})
		}
	}

	// Activities
	if len(cp.Activities) > 0 {
		for _, act := range cp.Activities {
			carePlan.Activity = append(carePlan.Activity, m.mapCarePlanActivity(act))
		}
	}

	return carePlan
}

// mapCarePlanStatus maps canonical status to FHIR CarePlan status.
func (m *USCoreMapper) mapCarePlanStatus(status string) string {
	statusLower := strings.ToLower(strings.TrimSpace(status))
	statusMap := map[string]string{
		"draft":            "draft",
		"active":           "active",
		"on-hold":          "on-hold",
		"onhold":           "on-hold",
		"revoked":          "revoked",
		"cancelled":        "revoked",
		"canceled":         "revoked",
		"completed":        "completed",
		"entered-in-error": "entered-in-error",
		"error":            "entered-in-error",
		"unknown":          "unknown",
	}
	if mapped, ok := statusMap[statusLower]; ok {
		return mapped
	}
	return "active" // Default
}

// mapCarePlanIntent maps canonical intent to FHIR CarePlan intent.
func (m *USCoreMapper) mapCarePlanIntent(intent string) string {
	intentLower := strings.ToLower(strings.TrimSpace(intent))
	intentMap := map[string]string{
		"proposal": "proposal",
		"plan":     "plan",
		"order":    "order",
		"option":   "option",
	}
	if mapped, ok := intentMap[intentLower]; ok {
		return mapped
	}
	return "plan" // Default
}

// mapCarePlanCategory maps category to US Core required CodeableConcept.
// US Core requires at least one category from http://hl7.org/fhir/us/core/CodeSystem/careplan-category
func (m *USCoreMapper) mapCarePlanCategory(category string) []CodeableConcept {
	// Always include the required "assess-plan" category for US Core
	categories := []CodeableConcept{
		{
			Coding: []Coding{
				{
					System:  SystemCarePlanCategory,
					Code:    "assess-plan",
					Display: "Assessment and Plan of Treatment",
				},
			},
		},
	}

	// If additional category provided, add it
	if category != "" && category != "assess-plan" {
		catLower := strings.ToLower(strings.TrimSpace(category))
		catMap := map[string]struct {
			code    string
			display string
		}{
			"discharge":        {"discharge", "Discharge Plan"},
			"discharge-plan":   {"discharge", "Discharge Plan"},
			"hospital":         {"hospital", "Hospital Plan"},
			"hospital-plan":    {"hospital", "Hospital Plan"},
			"longitudinal":     {"longitudinal", "Longitudinal Care Plan"},
			"home-health":      {"home-health", "Home Health Plan"},
			"homehealth":       {"home-health", "Home Health Plan"},
			"mental-health":    {"mental-health", "Mental Health Plan"},
			"mentalhealth":     {"mental-health", "Mental Health Plan"},
			"community":        {"community", "Community Health Plan"},
			"community-health": {"community", "Community Health Plan"},
		}

		if mapped, ok := catMap[catLower]; ok {
			categories = append(categories, CodeableConcept{
				Coding: []Coding{
					{
						System:  SystemCarePlanCategory,
						Code:    mapped.code,
						Display: mapped.display,
					},
				},
			})
		} else {
			// Add as text-only category
			categories = append(categories, CodeableConcept{
				Text: category,
			})
		}
	}

	return categories
}

// mapCarePlanActivity maps a canonical CarePlanActivity to FHIR.
func (m *USCoreMapper) mapCarePlanActivity(act events.CarePlanActivity) CarePlanActivity {
	activity := CarePlanActivity{}

	// Outcome description
	if act.OutcomeDescription != "" {
		activity.OutcomeCodeableConcept = []CodeableConcept{
			{Text: act.OutcomeDescription},
		}
	}

	// Activity detail
	detail := &CarePlanActivityDetail{
		Status: m.mapActivityStatus(act.Status),
	}

	// Activity code
	if act.Code != "" || act.Description != "" {
		detail.Code = m.mapActivityCode(act.Code, act.CodeSystem, act.Description)
	}

	// Activity description
	if act.Description != "" {
		detail.Description = act.Description
	}

	// Scheduled date
	if act.ScheduledDate != "" {
		detail.ScheduledString = act.ScheduledDate
	}

	activity.Detail = detail
	return activity
}

// mapActivityStatus maps activity status to FHIR.
func (m *USCoreMapper) mapActivityStatus(status string) string {
	statusLower := strings.ToLower(strings.TrimSpace(status))
	statusMap := map[string]string{
		"not-started":      "not-started",
		"notstarted":       "not-started",
		"pending":          "not-started",
		"scheduled":        "scheduled",
		"in-progress":      "in-progress",
		"inprogress":       "in-progress",
		"active":           "in-progress",
		"on-hold":          "on-hold",
		"onhold":           "on-hold",
		"completed":        "completed",
		"done":             "completed",
		"finished":         "completed",
		"cancelled":        "cancelled",
		"canceled":         "cancelled",
		"stopped":          "stopped",
		"unknown":          "unknown",
		"entered-in-error": "entered-in-error",
		"error":            "entered-in-error",
	}
	if mapped, ok := statusMap[statusLower]; ok {
		return mapped
	}
	return "not-started" // Default
}

// mapActivityCode maps activity code to CodeableConcept.
func (m *USCoreMapper) mapActivityCode(code, codeSystem, description string) *CodeableConcept {
	cc := &CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			// Detect code system
			system = m.detectActivityCodeSystem(code)
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: description,
			},
		}
	}

	if description != "" {
		cc.Text = description
	}

	return cc
}

// detectActivityCodeSystem detects the code system for an activity code.
func (m *USCoreMapper) detectActivityCodeSystem(code string) string {
	// CPT codes are 5-digit numbers
	if len(code) == 5 {
		if _, err := strconv.Atoi(code); err == nil {
			return SystemCPT
		}
	}

	// SNOMED CT codes are typically 6-18 digits
	if len(code) >= 6 && len(code) <= 18 {
		if _, err := strconv.Atoi(code); err == nil {
			return SystemSNOMED
		}
	}

	// Default to SNOMED
	return SystemSNOMED
}

// ============================================================================
// Goal Mapping (US Core 6.1.0)
// ============================================================================

// MapGoal converts a canonical GoalEvent to a US Core Goal.
func (m *USCoreMapper) MapGoal(event *events.GoalEvent, patientRef string) *Goal {
	if event == nil {
		return nil
	}

	g := event.Goal

	goal := &Goal{
		Meta: &Meta{
			Profile: []string{USCoreGoalProfile},
		},
		LifecycleStatus: m.mapGoalLifecycleStatus(g.LifecycleStatus),
		Subject:         &Reference{Reference: patientRef},
	}

	// Description is required by US Core
	goal.Description = &CodeableConcept{
		Text: g.Description,
	}

	// Achievement status
	if g.AchievementStatus != "" {
		goal.AchievementStatus = m.mapGoalAchievementStatus(g.AchievementStatus)
	}

	// Category
	if g.Category != "" {
		goal.Category = []CodeableConcept{
			m.mapGoalCategory(g.Category),
		}
	}

	// Priority
	if g.Priority != "" {
		goal.Priority = m.mapGoalPriority(g.Priority)
	}

	// Start date
	if g.StartDate != "" {
		goal.StartDate = g.StartDate
	}

	// Status date
	if g.StatusDate != "" {
		goal.StatusDate = g.StatusDate
	}

	// Status reason
	if g.StatusReason != "" {
		goal.StatusReason = g.StatusReason
	}

	// Expressed by
	if g.ExpressedBy != "" || event.Author != nil {
		goal.ExpressedBy = m.buildGoalExpressedBy(g.ExpressedBy, event.Author)
	}

	// Addresses (conditions)
	if len(g.AddressesIDs) > 0 {
		for _, condID := range g.AddressesIDs {
			goal.Addresses = append(goal.Addresses, Reference{
				Reference: "Condition/" + condID,
			})
		}
	}

	// Note
	if g.Note != "" {
		goal.Note = []Annotation{
			{Text: g.Note},
		}
	}

	// Target
	if g.Target != nil {
		goal.Target = []GoalTarget{m.mapGoalTarget(g.Target, g.TargetDate)}
	} else if g.TargetDate != "" {
		// If no target but have target date, create minimal target
		goal.Target = []GoalTarget{
			{DueDate: g.TargetDate},
		}
	}

	return goal
}

// mapGoalLifecycleStatus maps canonical status to FHIR Goal lifecycle status.
func (m *USCoreMapper) mapGoalLifecycleStatus(status string) string {
	statusLower := strings.ToLower(strings.TrimSpace(status))
	statusMap := map[string]string{
		"proposed":         "proposed",
		"planned":          "planned",
		"accepted":         "accepted",
		"active":           "active",
		"on-hold":          "on-hold",
		"onhold":           "on-hold",
		"completed":        "completed",
		"cancelled":        "cancelled",
		"canceled":         "cancelled",
		"entered-in-error": "entered-in-error",
		"error":            "entered-in-error",
		"rejected":         "rejected",
	}
	if mapped, ok := statusMap[statusLower]; ok {
		return mapped
	}
	return "active" // Default
}

// mapGoalAchievementStatus maps canonical achievement status to FHIR CodeableConcept.
func (m *USCoreMapper) mapGoalAchievementStatus(status string) *CodeableConcept {
	statusLower := strings.ToLower(strings.TrimSpace(status))
	statusMap := map[string]struct {
		code    string
		display string
	}{
		"in-progress":    {"in-progress", "In Progress"},
		"inprogress":     {"in-progress", "In Progress"},
		"improving":      {"improving", "Improving"},
		"worsening":      {"worsening", "Worsening"},
		"no-change":      {"no-change", "No Change"},
		"nochange":       {"no-change", "No Change"},
		"achieved":       {"achieved", "Achieved"},
		"sustaining":     {"sustaining", "Sustaining"},
		"not-achieved":   {"not-achieved", "Not Achieved"},
		"notachieved":    {"not-achieved", "Not Achieved"},
		"no-progress":    {"no-progress", "No Progress"},
		"noprogress":     {"no-progress", "No Progress"},
		"not-attainable": {"not-attainable", "Not Attainable"},
		"notattainable":  {"not-attainable", "Not Attainable"},
	}

	if mapped, ok := statusMap[statusLower]; ok {
		return &CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemGoalAchievementStatus,
					Code:    mapped.code,
					Display: mapped.display,
				},
			},
		}
	}

	// Default to in-progress
	return &CodeableConcept{
		Coding: []Coding{
			{
				System:  SystemGoalAchievementStatus,
				Code:    "in-progress",
				Display: "In Progress",
			},
		},
	}
}

// mapGoalCategory maps canonical category to FHIR CodeableConcept.
func (m *USCoreMapper) mapGoalCategory(category string) CodeableConcept {
	catLower := strings.ToLower(strings.TrimSpace(category))

	// Common goal categories (SNOMED CT)
	catMap := map[string]struct {
		code    string
		display string
	}{
		"dietary":       {"289141003", "Dietary finding"},
		"diet":          {"289141003", "Dietary finding"},
		"nutrition":     {"289141003", "Dietary finding"},
		"safety":        {"410518001", "Personal safety status"},
		"behavioral":    {"363879005", "Mental and behavioral observation"},
		"behavior":      {"363879005", "Mental and behavioral observation"},
		"mental-health": {"363879005", "Mental and behavioral observation"},
		"mentalhealth":  {"363879005", "Mental and behavioral observation"},
		"nursing":       {"365857007", "Nursing assessment finding"},
		"physiotherapy": {"410602005", "Physical therapy assessment finding"},
		"physical":      {"410602005", "Physical therapy assessment finding"},
		"pt":            {"410602005", "Physical therapy assessment finding"},
	}

	if mapped, ok := catMap[catLower]; ok {
		return CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemSNOMED,
					Code:    mapped.code,
					Display: mapped.display,
				},
			},
			Text: category,
		}
	}

	// Return as text-only
	return CodeableConcept{
		Text: category,
	}
}

// mapGoalPriority maps canonical priority to FHIR CodeableConcept.
func (m *USCoreMapper) mapGoalPriority(priority string) *CodeableConcept {
	prioLower := strings.ToLower(strings.TrimSpace(priority))
	prioMap := map[string]struct {
		code    string
		display string
	}{
		"high":            {"high-priority", "High Priority"},
		"high-priority":   {"high-priority", "High Priority"},
		"highpriority":    {"high-priority", "High Priority"},
		"medium":          {"medium-priority", "Medium Priority"},
		"medium-priority": {"medium-priority", "Medium Priority"},
		"mediumpriority":  {"medium-priority", "Medium Priority"},
		"normal":          {"medium-priority", "Medium Priority"},
		"low":             {"low-priority", "Low Priority"},
		"low-priority":    {"low-priority", "Low Priority"},
		"lowpriority":     {"low-priority", "Low Priority"},
	}

	if mapped, ok := prioMap[prioLower]; ok {
		return &CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemGoalPriority,
					Code:    mapped.code,
					Display: mapped.display,
				},
			},
		}
	}

	return nil
}

// buildGoalExpressedBy builds the expressedBy reference for a goal.
func (m *USCoreMapper) buildGoalExpressedBy(expressedBy string, author *events.Provider) *Reference {
	// If we have an author provider, use that
	if author != nil {
		return &Reference{
			Reference: m.buildProviderReference(author),
			Display:   m.buildProviderDisplayName(author),
		}
	}

	// Otherwise, use the expressedBy string
	if expressedBy == "" {
		return nil
	}

	expLower := strings.ToLower(strings.TrimSpace(expressedBy))
	switch expLower {
	case "patient":
		return &Reference{
			Type:    "Patient",
			Display: "Patient",
		}
	case "practitioner", "provider", "physician", "doctor":
		return &Reference{
			Type:    "Practitioner",
			Display: "Practitioner",
		}
	case "related", "related-person", "relatedperson", "family", "caregiver":
		return &Reference{
			Type:    "RelatedPerson",
			Display: "Related Person",
		}
	default:
		return &Reference{
			Display: expressedBy,
		}
	}
}

// mapGoalTarget maps a canonical GoalTarget to FHIR GoalTarget.
func (m *USCoreMapper) mapGoalTarget(target *events.GoalTarget, targetDate string) GoalTarget {
	gt := GoalTarget{}

	// Measure (what is being tracked)
	if target.Measure != "" {
		system := target.MeasureSystem
		if system == "" {
			system = SystemLOINC // Default to LOINC for measurements
		}
		gt.Measure = &CodeableConcept{
			Coding: []Coding{
				{
					System: system,
					Code:   target.Measure,
				},
			},
		}
	}

	// Target value - quantity takes precedence over string
	if target.DetailQuantity != 0 || target.DetailUnit != "" {
		gt.DetailQuantity = &Quantity{
			Value: target.DetailQuantity,
		}
		if target.DetailUnit != "" {
			ucumUnit, ucumCode := m.mapGoalTargetUnit(target.DetailUnit)
			gt.DetailQuantity.Unit = ucumUnit
			gt.DetailQuantity.Code = ucumCode
			gt.DetailQuantity.System = SystemUCUM
		}
	} else if target.DetailString != "" {
		gt.DetailString = target.DetailString
	}

	// Due date (from target or passed-in targetDate)
	if target.DueDate != "" {
		gt.DueDate = target.DueDate
	} else if targetDate != "" {
		gt.DueDate = targetDate
	}

	return gt
}

// mapGoalTargetUnit maps common goal target units to UCUM.
func (m *USCoreMapper) mapGoalTargetUnit(unit string) (display, code string) {
	unitLower := strings.ToLower(strings.TrimSpace(unit))
	unitMap := map[string]struct {
		display string
		code    string
	}{
		// Weight
		"kg":        {"kg", "kg"},
		"kilogram":  {"kg", "kg"},
		"kilograms": {"kg", "kg"},
		"lb":        {"[lb_av]", "[lb_av]"},
		"lbs":       {"[lb_av]", "[lb_av]"},
		"pound":     {"[lb_av]", "[lb_av]"},
		"pounds":    {"[lb_av]", "[lb_av]"},
		// Blood pressure
		"mmhg":  {"mmHg", "mm[Hg]"},
		"mm hg": {"mmHg", "mm[Hg]"},
		// Blood glucose
		"mg/dl":  {"mg/dL", "mg/dL"},
		"mmol/l": {"mmol/L", "mmol/L"},
		// A1C
		"%":       {"%", "%"},
		"percent": {"%", "%"},
		// Steps
		"steps":     {"steps", "{steps}"},
		"steps/day": {"steps/day", "{steps}/d"},
		// Minutes
		"min":      {"min", "min"},
		"minute":   {"min", "min"},
		"minutes":  {"min", "min"},
		"min/day":  {"min/day", "min/d"},
		"min/week": {"min/week", "min/wk"},
		// General counts
		"count":    {"count", "{count}"},
		"servings": {"servings", "{servings}"},
		"glasses":  {"glasses", "{glasses}"},
	}

	if mapped, ok := unitMap[unitLower]; ok {
		return mapped.display, mapped.code
	}

	// Return as-is if no mapping
	return unit, unit
}

// ============================================================================
// CareTeam Mapping (US Core 6.1.0)
// ============================================================================

// MapCareTeam converts a canonical CareTeamEvent to a US Core CareTeam.
func (m *USCoreMapper) MapCareTeam(event *events.CareTeamEvent, patientRef string) *CareTeam {
	if event == nil {
		return nil
	}

	ct := event.CareTeam

	careTeam := &CareTeam{
		Meta: &Meta{
			Profile: []string{USCoreCareTeamProfile},
		},
		Status:  m.mapCareTeamStatus(ct.Status),
		Subject: &Reference{Reference: patientRef},
	}

	// Name (optional but recommended)
	if ct.Name != "" {
		careTeam.Name = ct.Name
	}

	// Category
	if ct.Category != "" {
		careTeam.Category = []CodeableConcept{
			m.mapCareTeamCategory(ct.Category),
		}
	}

	// Period
	if ct.PeriodStart != "" || ct.PeriodEnd != "" {
		careTeam.Period = &Period{}
		if ct.PeriodStart != "" {
			if t, err := time.Parse("2006-01-02", ct.PeriodStart); err == nil {
				careTeam.Period.Start = &t
			} else if t, err := time.Parse(time.RFC3339, ct.PeriodStart); err == nil {
				careTeam.Period.Start = &t
			}
		}
		if ct.PeriodEnd != "" {
			if t, err := time.Parse("2006-01-02", ct.PeriodEnd); err == nil {
				careTeam.Period.End = &t
			} else if t, err := time.Parse(time.RFC3339, ct.PeriodEnd); err == nil {
				careTeam.Period.End = &t
			}
		}
	}

	// Encounter reference
	if event.Encounter != nil && event.Encounter.ID != "" {
		careTeam.Encounter = &Reference{
			Reference: "Encounter/" + event.Encounter.ID,
		}
	}

	// Reason (coded)
	if ct.ReasonCode != "" || ct.ReasonText != "" {
		careTeam.ReasonCode = []CodeableConcept{
			m.mapCareTeamReason(ct.ReasonCode, ct.ReasonCodeSystem, ct.ReasonText),
		}
	}

	// Reason references (conditions)
	if len(ct.ConditionIDs) > 0 {
		for _, condID := range ct.ConditionIDs {
			careTeam.ReasonReference = append(careTeam.ReasonReference, Reference{
				Reference: "Condition/" + condID,
			})
		}
	}

	// Participants (US Core requires at least one participant with a role)
	if len(ct.Members) > 0 {
		for _, member := range ct.Members {
			careTeam.Participant = append(careTeam.Participant, m.mapCareTeamParticipant(member))
		}
	}

	// Managing organization
	if ct.ManagingOrganizationID != "" || ct.ManagingOrganizationName != "" {
		careTeam.ManagingOrganization = []Reference{
			m.buildOrganizationReference(ct.ManagingOrganizationID, ct.ManagingOrganizationName),
		}
	}

	// Note
	if ct.Note != "" {
		careTeam.Note = []Annotation{
			{Text: ct.Note},
		}
	}

	return careTeam
}

// mapCareTeamStatus maps canonical status to FHIR CareTeam status.
func (m *USCoreMapper) mapCareTeamStatus(status string) string {
	statusLower := strings.ToLower(strings.TrimSpace(status))
	statusMap := map[string]string{
		"proposed":         "proposed",
		"active":           "active",
		"suspended":        "suspended",
		"on-hold":          "suspended",
		"onhold":           "suspended",
		"inactive":         "inactive",
		"entered-in-error": "entered-in-error",
		"error":            "entered-in-error",
	}
	if mapped, ok := statusMap[statusLower]; ok {
		return mapped
	}
	return "active" // Default
}

// mapCareTeamCategory maps category to CodeableConcept.
// US Core recommends LOINC codes for care team category.
func (m *USCoreMapper) mapCareTeamCategory(category string) CodeableConcept {
	catLower := strings.ToLower(strings.TrimSpace(category))

	// LOINC care team categories
	catMap := map[string]struct {
		code    string
		display string
	}{
		"longitudinal":      {"LA27976-2", "Longitudinal care-coordination focused care team"},
		"longitudinal-care": {"LA27976-2", "Longitudinal care-coordination focused care team"},
		"episode":           {"LA27977-0", "Episode of care-focused care team"},
		"episode-of-care":   {"LA27977-0", "Episode of care-focused care team"},
		"condition":         {"LA27978-8", "Condition-focused care team"},
		"condition-focused": {"LA27978-8", "Condition-focused care team"},
		"encounter":         {"LA28865-6", "Encounter-focused care team"},
		"encounter-focused": {"LA28865-6", "Encounter-focused care team"},
		"home-health":       {"LA28866-4", "Home & Community Based Services (HCBS)-focused care team"},
		"hcbs":              {"LA28866-4", "Home & Community Based Services (HCBS)-focused care team"},
		"clinical-research": {"LA28867-2", "Clinical research-focused care team"},
		"research":          {"LA28867-2", "Clinical research-focused care team"},
		"public-health":     {"LA28868-0", "Public health-focused care team"},
	}

	if mapped, ok := catMap[catLower]; ok {
		return CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemLOINC,
					Code:    mapped.code,
					Display: mapped.display,
				},
			},
			Text: category,
		}
	}

	// Return as text-only
	return CodeableConcept{
		Text: category,
	}
}

// mapCareTeamReason maps reason to CodeableConcept.
func (m *USCoreMapper) mapCareTeamReason(code, codeSystem, text string) CodeableConcept {
	cc := CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			system = SystemSNOMED // Default to SNOMED CT
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: text,
			},
		}
	}

	if text != "" {
		cc.Text = text
	}

	return cc
}

// mapCareTeamParticipant maps a CareTeamMember to FHIR CareTeamParticipant.
func (m *USCoreMapper) mapCareTeamParticipant(member events.CareTeamMember) CareTeamParticipant {
	participant := CareTeamParticipant{}

	// Role is required by US Core
	if member.Role != "" || member.RoleCode != "" {
		participant.Role = []CodeableConcept{
			m.mapParticipantRole(member.Role, member.RoleCode, member.RoleCodeSystem),
		}
	}

	// Member reference (Practitioner or Organization)
	if member.Provider != nil {
		participant.Member = &Reference{
			Reference: m.buildProviderReference(member.Provider),
			Display:   m.buildProviderDisplayName(member.Provider),
		}
	} else if member.OrganizationID != "" || member.OrganizationName != "" {
		ref := m.buildOrganizationReference(member.OrganizationID, member.OrganizationName)
		participant.Member = &ref
	}

	// Period
	if member.PeriodStart != "" || member.PeriodEnd != "" {
		participant.Period = &Period{}
		if member.PeriodStart != "" {
			if t, err := time.Parse("2006-01-02", member.PeriodStart); err == nil {
				participant.Period.Start = &t
			}
		}
		if member.PeriodEnd != "" {
			if t, err := time.Parse("2006-01-02", member.PeriodEnd); err == nil {
				participant.Period.End = &t
			}
		}
	}

	return participant
}

// mapParticipantRole maps role to CodeableConcept.
func (m *USCoreMapper) mapParticipantRole(role, roleCode, roleCodeSystem string) CodeableConcept {
	cc := CodeableConcept{}

	if roleCode != "" {
		system := roleCodeSystem
		if system == "" {
			system = SystemSNOMED // Default to SNOMED CT
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    roleCode,
				Display: role,
			},
		}
	} else if role != "" {
		// Map common roles to SNOMED CT
		roleLower := strings.ToLower(strings.TrimSpace(role))
		roleMap := map[string]struct {
			code    string
			display string
		}{
			"primary care physician": {"446050000", "Primary care physician"},
			"pcp":                    {"446050000", "Primary care physician"},
			"primary care provider":  {"446050000", "Primary care physician"},
			"specialist":             {"309395003", "Specialist physician"},
			"nurse":                  {"224535009", "Registered nurse"},
			"registered nurse":       {"224535009", "Registered nurse"},
			"rn":                     {"224535009", "Registered nurse"},
			"nurse practitioner":     {"224571005", "Nurse practitioner"},
			"np":                     {"224571005", "Nurse practitioner"},
			"physician assistant":    {"449161006", "Physician assistant"},
			"pa":                     {"449161006", "Physician assistant"},
			"case manager":           {"768820003", "Case manager"},
			"care coordinator":       {"768820003", "Case manager"},
			"social worker":          {"106328005", "Social worker"},
			"pharmacist":             {"46255001", "Pharmacist"},
			"physical therapist":     {"36682004", "Physical therapist"},
			"pt":                     {"36682004", "Physical therapist"},
			"occupational therapist": {"80546007", "Occupational therapist"},
			"ot":                     {"80546007", "Occupational therapist"},
			"dietitian":              {"159033005", "Dietitian"},
			"nutritionist":           {"159033005", "Dietitian"},
			"psychologist":           {"59944000", "Psychologist"},
			"psychiatrist":           {"80584001", "Psychiatrist"},
			"caregiver":              {"133932002", "Caregiver"},
			"family member":          {"303071001", "Person in family of patient"},
		}

		if mapped, ok := roleMap[roleLower]; ok {
			cc.Coding = []Coding{
				{
					System:  SystemSNOMED,
					Code:    mapped.code,
					Display: mapped.display,
				},
			}
		}
	}

	if role != "" {
		cc.Text = role
	}

	return cc
}

// buildOrganizationReference builds a reference for an organization.
func (m *USCoreMapper) buildOrganizationReference(orgID, orgName string) Reference {
	ref := Reference{}

	if orgID != "" {
		ref.Reference = "Organization/" + orgID
	}
	if orgName != "" {
		ref.Display = orgName
	}

	return ref
}

// ============================================================================
// ServiceRequest Mapping (US Core 6.1.0)
// ============================================================================

// MapServiceRequest converts a canonical ServiceRequestEvent to a US Core ServiceRequest.
func (m *USCoreMapper) MapServiceRequest(event *events.ServiceRequestEvent, patientRef string) *ServiceRequest {
	if event == nil {
		return nil
	}

	sr := event.ServiceRequest

	serviceRequest := &ServiceRequest{
		Meta: &Meta{
			Profile: []string{USCoreServiceRequestProfile},
		},
		Status:  m.mapServiceRequestStatus(sr.Status),
		Intent:  m.mapServiceRequestIntent(sr.Intent),
		Subject: &Reference{Reference: patientRef},
	}

	// Category
	if sr.Category != "" {
		serviceRequest.Category = []CodeableConcept{
			m.mapServiceRequestCategory(sr.Category),
		}
	}

	// Priority
	if sr.Priority != "" {
		serviceRequest.Priority = m.mapServiceRequestPriority(sr.Priority)
	}

	// Code (required by US Core)
	if sr.Code != "" || sr.CodeText != "" {
		serviceRequest.Code = m.mapServiceCode(sr.Code, sr.CodeSystem, sr.CodeText)
	}

	// Order detail
	if sr.OrderDetail != "" {
		serviceRequest.OrderDetail = []CodeableConcept{
			{Text: sr.OrderDetail},
		}
	}

	// Quantity
	if sr.QuantityValue != 0 || sr.QuantityUnit != "" {
		serviceRequest.QuantityQuantity = &Quantity{
			Value: sr.QuantityValue,
		}
		if sr.QuantityUnit != "" {
			serviceRequest.QuantityQuantity.Unit = sr.QuantityUnit
		}
	}

	// Occurrence
	if sr.OccurrenceDateTime != "" {
		serviceRequest.OccurrenceDateTime = sr.OccurrenceDateTime
	} else if sr.OccurrencePeriodStart != "" || sr.OccurrencePeriodEnd != "" {
		serviceRequest.OccurrencePeriod = &Period{}
		if sr.OccurrencePeriodStart != "" {
			if t, err := time.Parse("2006-01-02", sr.OccurrencePeriodStart); err == nil {
				serviceRequest.OccurrencePeriod.Start = &t
			} else if t, err := time.Parse(time.RFC3339, sr.OccurrencePeriodStart); err == nil {
				serviceRequest.OccurrencePeriod.Start = &t
			}
		}
		if sr.OccurrencePeriodEnd != "" {
			if t, err := time.Parse("2006-01-02", sr.OccurrencePeriodEnd); err == nil {
				serviceRequest.OccurrencePeriod.End = &t
			} else if t, err := time.Parse(time.RFC3339, sr.OccurrencePeriodEnd); err == nil {
				serviceRequest.OccurrencePeriod.End = &t
			}
		}
	}

	// Authored on (required by US Core)
	if sr.AuthoredOn != "" {
		serviceRequest.AuthoredOn = sr.AuthoredOn
	} else if !event.Timestamp.IsZero() {
		serviceRequest.AuthoredOn = event.Timestamp.Format(time.RFC3339)
	}

	// Requester (required by US Core)
	if event.Requester != nil {
		serviceRequest.Requester = &Reference{
			Reference: m.buildProviderReference(event.Requester),
			Display:   m.buildProviderDisplayName(event.Requester),
		}
	}

	// Performer
	if event.Performer != nil {
		serviceRequest.Performer = []Reference{
			{
				Reference: m.buildProviderReference(event.Performer),
				Display:   m.buildProviderDisplayName(event.Performer),
			},
		}
	} else if event.PerformerOrgID != "" || event.PerformerOrgName != "" {
		serviceRequest.Performer = []Reference{
			m.buildOrganizationReference(event.PerformerOrgID, event.PerformerOrgName),
		}
	}

	// Encounter reference
	if event.Encounter != nil && event.Encounter.ID != "" {
		serviceRequest.Encounter = &Reference{
			Reference: "Encounter/" + event.Encounter.ID,
		}
	}

	// Reason (coded)
	if sr.ReasonCode != "" || sr.ReasonText != "" {
		serviceRequest.ReasonCode = []CodeableConcept{
			m.mapServiceRequestReason(sr.ReasonCode, sr.ReasonCodeSystem, sr.ReasonText),
		}
	}

	// Reason references (conditions)
	if len(sr.ConditionIDs) > 0 {
		for _, condID := range sr.ConditionIDs {
			serviceRequest.ReasonReference = append(serviceRequest.ReasonReference, Reference{
				Reference: "Condition/" + condID,
			})
		}
	}

	// Body site
	if sr.BodySite != "" || sr.BodySiteCode != "" {
		serviceRequest.BodySite = []CodeableConcept{
			m.mapBodySite(sr.BodySite, sr.BodySiteCode),
		}
	}

	// Note
	if sr.Note != "" {
		serviceRequest.Note = []Annotation{
			{Text: sr.Note},
		}
	}

	// Patient instruction
	if sr.PatientInstruction != "" {
		serviceRequest.PatientInstruction = sr.PatientInstruction
	}

	return serviceRequest
}

// mapServiceRequestStatus maps canonical status to FHIR ServiceRequest status.
func (m *USCoreMapper) mapServiceRequestStatus(status string) string {
	statusLower := strings.ToLower(strings.TrimSpace(status))
	statusMap := map[string]string{
		"draft":            "draft",
		"active":           "active",
		"on-hold":          "on-hold",
		"onhold":           "on-hold",
		"revoked":          "revoked",
		"cancelled":        "revoked",
		"canceled":         "revoked",
		"completed":        "completed",
		"entered-in-error": "entered-in-error",
		"error":            "entered-in-error",
		"unknown":          "unknown",
	}
	if mapped, ok := statusMap[statusLower]; ok {
		return mapped
	}
	return "active" // Default
}

// mapServiceRequestIntent maps canonical intent to FHIR ServiceRequest intent.
func (m *USCoreMapper) mapServiceRequestIntent(intent string) string {
	intentLower := strings.ToLower(strings.TrimSpace(intent))
	intentMap := map[string]string{
		"proposal":       "proposal",
		"plan":           "plan",
		"directive":      "directive",
		"order":          "order",
		"original-order": "original-order",
		"originalorder":  "original-order",
		"reflex-order":   "reflex-order",
		"reflexorder":    "reflex-order",
		"filler-order":   "filler-order",
		"fillerorder":    "filler-order",
		"instance-order": "instance-order",
		"instanceorder":  "instance-order",
		"option":         "option",
	}
	if mapped, ok := intentMap[intentLower]; ok {
		return mapped
	}
	return "order" // Default
}

// mapServiceRequestCategory maps category to CodeableConcept.
func (m *USCoreMapper) mapServiceRequestCategory(category string) CodeableConcept {
	catLower := strings.ToLower(strings.TrimSpace(category))

	// SNOMED CT service categories
	catMap := map[string]struct {
		code    string
		display string
	}{
		"laboratory":        {"108252007", "Laboratory procedure"},
		"lab":               {"108252007", "Laboratory procedure"},
		"imaging":           {"363679005", "Imaging"},
		"radiology":         {"363679005", "Imaging"},
		"procedure":         {"387713003", "Surgical procedure"},
		"surgical":          {"387713003", "Surgical procedure"},
		"counseling":        {"409063005", "Counseling"},
		"therapy":           {"276239002", "Therapy"},
		"referral":          {"3457005", "Patient referral"},
		"consultation":      {"11429006", "Consultation"},
		"consult":           {"11429006", "Consultation"},
		"education":         {"311401005", "Patient education"},
		"patient-education": {"311401005", "Patient education"},
		"screening":         {"360156006", "Screening procedure"},
		"assessment":        {"386053000", "Evaluation procedure"},
		"evaluation":        {"386053000", "Evaluation procedure"},
	}

	if mapped, ok := catMap[catLower]; ok {
		return CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemSNOMED,
					Code:    mapped.code,
					Display: mapped.display,
				},
			},
			Text: category,
		}
	}

	// Return as text-only
	return CodeableConcept{
		Text: category,
	}
}

// mapServiceRequestPriority maps priority to FHIR code.
func (m *USCoreMapper) mapServiceRequestPriority(priority string) string {
	prioLower := strings.ToLower(strings.TrimSpace(priority))
	prioMap := map[string]string{
		"routine":   "routine",
		"normal":    "routine",
		"urgent":    "urgent",
		"asap":      "asap",
		"stat":      "stat",
		"emergent":  "stat",
		"emergency": "stat",
	}
	if mapped, ok := prioMap[prioLower]; ok {
		return mapped
	}
	return "routine" // Default
}

// mapServiceCode maps service code to CodeableConcept.
func (m *USCoreMapper) mapServiceCode(code, codeSystem, text string) *CodeableConcept {
	cc := &CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			// Try to detect the code system
			system = m.detectServiceCodeSystem(code)
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: text,
			},
		}
	}

	if text != "" {
		cc.Text = text
	}

	return cc
}

// detectServiceCodeSystem attempts to detect the code system for a service code.
func (m *USCoreMapper) detectServiceCodeSystem(code string) string {
	// CPT codes are 5-digit numbers
	if len(code) == 5 {
		if _, err := strconv.Atoi(code); err == nil {
			return SystemCPT
		}
	}

	// LOINC codes typically have a hyphen (e.g., "12345-6")
	if strings.Contains(code, "-") && len(code) >= 5 && len(code) <= 10 {
		return SystemLOINC
	}

	// HCPCS codes start with a letter
	if len(code) == 5 && code[0] >= 'A' && code[0] <= 'Z' {
		return SystemHCPCS
	}

	// SNOMED CT codes are typically 6-18 digits
	if len(code) >= 6 && len(code) <= 18 {
		if _, err := strconv.Atoi(code); err == nil {
			return SystemSNOMED
		}
	}

	// Default to SNOMED CT
	return SystemSNOMED
}

// mapServiceRequestReason maps reason to CodeableConcept.
func (m *USCoreMapper) mapServiceRequestReason(code, codeSystem, text string) CodeableConcept {
	cc := CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			// Default to ICD-10-CM for reason codes
			if len(code) >= 3 && code[0] >= 'A' && code[0] <= 'Z' {
				system = SystemICD10CM
			} else {
				system = SystemSNOMED
			}
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: text,
			},
		}
	}

	if text != "" {
		cc.Text = text
	}

	return cc
}

// mapBodySite maps body site to CodeableConcept.
func (m *USCoreMapper) mapBodySite(site, siteCode string) CodeableConcept {
	cc := CodeableConcept{}

	if siteCode != "" {
		cc.Coding = []Coding{
			{
				System:  SystemSNOMED,
				Code:    siteCode,
				Display: site,
			},
		}
	} else if site != "" {
		// Try to map common body sites to SNOMED CT
		siteLower := strings.ToLower(strings.TrimSpace(site))
		siteMap := map[string]struct {
			code    string
			display string
		}{
			"head":      {"69536005", "Head structure"},
			"neck":      {"45048000", "Neck structure"},
			"chest":     {"51185008", "Thoracic structure"},
			"thorax":    {"51185008", "Thoracic structure"},
			"abdomen":   {"818983003", "Abdominal structure"},
			"back":      {"77568009", "Back structure"},
			"arm":       {"53120007", "Upper limb structure"},
			"upper arm": {"40983000", "Upper arm structure"},
			"forearm":   {"14975008", "Forearm structure"},
			"hand":      {"85562004", "Hand structure"},
			"leg":       {"61685007", "Lower limb structure"},
			"thigh":     {"68367000", "Thigh structure"},
			"knee":      {"72696002", "Knee region structure"},
			"foot":      {"56459004", "Foot structure"},
			"left":      {"7771000", "Left"},
			"right":     {"24028007", "Right"},
		}

		if mapped, ok := siteMap[siteLower]; ok {
			cc.Coding = []Coding{
				{
					System:  SystemSNOMED,
					Code:    mapped.code,
					Display: mapped.display,
				},
			}
		}
	}

	if site != "" {
		cc.Text = site
	}

	return cc
}

// ============================================================================
// DocumentReference Mapping (US Core)
// ============================================================================

// MapDocumentReference converts a canonical DocumentReferenceEvent to a US Core DocumentReference.
func (m *USCoreMapper) MapDocumentReference(event *events.DocumentReferenceEvent, patientRef string) *DocumentReference {
	if event == nil {
		return nil
	}

	dr := event.DocumentReference

	docRef := &DocumentReference{
		Meta: &Meta{
			Profile: []string{USCoreDocumentReferenceProfile},
		},
		Status:  m.mapDocumentReferenceStatus(dr.Status),
		Subject: &Reference{Reference: patientRef},
	}

	// DocStatus (composition status)
	if dr.DocStatus != "" {
		docRef.DocStatus = m.mapCompositionStatus(dr.DocStatus)
	}

	// Type (required by US Core - LOINC code)
	docRef.Type = m.mapDocumentType(dr.TypeCode, dr.TypeCodeSystem, dr.Type)

	// Category (required by US Core)
	docRef.Category = []CodeableConcept{
		m.mapDocumentCategory(dr.CategoryCode, dr.CategoryCodeSystem, dr.Category),
	}

	// Date
	if dr.Date != "" {
		docRef.Date = dr.Date
	}

	// Author
	if event.Author != nil {
		docRef.Author = []Reference{
			{
				Reference: m.buildProviderReference(event.Author),
				Display:   m.buildProviderDisplayName(event.Author),
			},
		}
	} else if event.AuthorOrgID != "" || event.AuthorOrgName != "" {
		docRef.Author = []Reference{
			m.buildOrganizationReference(event.AuthorOrgID, event.AuthorOrgName),
		}
	}

	// Authenticator
	if event.Authenticator != nil {
		docRef.Authenticator = &Reference{
			Reference: m.buildProviderReference(event.Authenticator),
			Display:   m.buildProviderDisplayName(event.Authenticator),
		}
	}

	// Custodian
	if dr.CustodianID != "" || dr.CustodianName != "" {
		docRef.Custodian = &Reference{
			Reference: "Organization/" + dr.CustodianID,
			Display:   dr.CustodianName,
		}
		if dr.CustodianID == "" {
			docRef.Custodian.Reference = ""
		}
	}

	// Description
	if dr.Description != "" {
		docRef.Description = dr.Description
	}

	// Security label
	if dr.SecurityLabel != "" {
		docRef.SecurityLabel = []CodeableConcept{
			m.mapSecurityLabel(dr.SecurityLabel),
		}
	}

	// Content (required by US Core - at least one attachment)
	if len(dr.Content) > 0 {
		for _, content := range dr.Content {
			docRef.Content = append(docRef.Content, m.mapDocumentContent(content))
		}
	}

	// Context
	if dr.Context != nil || (event.Encounter != nil && event.Encounter.ID != "") {
		docRef.Context = m.mapDocumentContext(dr.Context, event.Encounter)
	}

	// RelatesTo
	if len(dr.RelatesTo) > 0 {
		for _, rel := range dr.RelatesTo {
			docRef.RelatesTo = append(docRef.RelatesTo, DocumentReferenceRelatesTo{
				Code: m.mapDocumentRelationship(rel.Code),
				Target: &Reference{
					Reference: "DocumentReference/" + rel.TargetID,
				},
			})
		}
	}

	return docRef
}

// mapDocumentReferenceStatus maps status to FHIR DocumentReference status.
func (m *USCoreMapper) mapDocumentReferenceStatus(status string) string {
	statusLower := strings.ToLower(strings.TrimSpace(status))
	statusMap := map[string]string{
		"current":          "current",
		"active":           "current",
		"superseded":       "superseded",
		"replaced":         "superseded",
		"entered-in-error": "entered-in-error",
		"error":            "entered-in-error",
	}
	if mapped, ok := statusMap[statusLower]; ok {
		return mapped
	}
	return "current" // Default
}

// mapCompositionStatus maps composition/document status.
func (m *USCoreMapper) mapCompositionStatus(status string) string {
	statusLower := strings.ToLower(strings.TrimSpace(status))
	statusMap := map[string]string{
		"preliminary":      "preliminary",
		"draft":            "preliminary",
		"final":            "final",
		"amended":          "amended",
		"corrected":        "amended",
		"entered-in-error": "entered-in-error",
	}
	if mapped, ok := statusMap[statusLower]; ok {
		return mapped
	}
	return "final" // Default
}

// mapDocumentType maps document type to CodeableConcept.
// US Core requires LOINC codes for document type.
func (m *USCoreMapper) mapDocumentType(code, codeSystem, text string) *CodeableConcept {
	cc := &CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			system = SystemLOINC // Default to LOINC
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: text,
			},
		}
	} else if text != "" {
		// Try to map common document type text to LOINC
		typeMap := map[string]struct {
			code    string
			display string
		}{
			"discharge summary":    {"18842-5", "Discharge summary"},
			"progress note":        {"11506-3", "Progress note"},
			"history and physical": {"34117-2", "History and physical note"},
			"h&p":                  {"34117-2", "History and physical note"},
			"consultation":         {"11488-4", "Consultation note"},
			"consult note":         {"11488-4", "Consultation note"},
			"operative note":       {"11504-8", "Surgical operation note"},
			"surgical note":        {"11504-8", "Surgical operation note"},
			"procedure note":       {"28570-0", "Procedure note"},
			"referral":             {"57133-1", "Referral note"},
			"transfer summary":     {"18761-7", "Transfer summary note"},
			"continuity of care":   {"34133-9", "Summary of episode note"},
			"ccd":                  {"34133-9", "Summary of episode note"},
			"clinical note":        {"34109-9", "Clinical note"},
			"imaging report":       {"18748-4", "Diagnostic imaging study"},
			"radiology report":     {"18748-4", "Diagnostic imaging study"},
			"pathology report":     {"11526-1", "Pathology study"},
			"lab report":           {"11502-2", "Laboratory report"},
			"cardiology":           {"11524-6", "EKG study"},
		}

		textLower := strings.ToLower(strings.TrimSpace(text))
		if mapped, ok := typeMap[textLower]; ok {
			cc.Coding = []Coding{
				{
					System:  SystemLOINC,
					Code:    mapped.code,
					Display: mapped.display,
				},
			}
		}
	}

	if text != "" {
		cc.Text = text
	}

	return cc
}

// mapDocumentCategory maps document category to CodeableConcept.
func (m *USCoreMapper) mapDocumentCategory(code, codeSystem, text string) CodeableConcept {
	cc := CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			system = SystemDocumentReferenceCategory
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: text,
			},
		}
	} else if text != "" {
		// Map common category names to US Core categories
		catMap := map[string]struct {
			code    string
			display string
		}{
			"clinical-note": {"clinical-note", "Clinical Note"},
			"clinical note": {"clinical-note", "Clinical Note"},
			"note":          {"clinical-note", "Clinical Note"},
		}

		textLower := strings.ToLower(strings.TrimSpace(text))
		if mapped, ok := catMap[textLower]; ok {
			cc.Coding = []Coding{
				{
					System:  SystemDocumentReferenceCategory,
					Code:    mapped.code,
					Display: mapped.display,
				},
			}
		}
	}

	// Default to clinical-note category
	if len(cc.Coding) == 0 {
		cc.Coding = []Coding{
			{
				System:  SystemDocumentReferenceCategory,
				Code:    "clinical-note",
				Display: "Clinical Note",
			},
		}
	}

	if text != "" {
		cc.Text = text
	}

	return cc
}

// mapSecurityLabel maps confidentiality level to CodeableConcept.
func (m *USCoreMapper) mapSecurityLabel(label string) CodeableConcept {
	labelUpper := strings.ToUpper(strings.TrimSpace(label))
	labelMap := map[string]struct {
		code    string
		display string
	}{
		"U":          {"U", "Unrestricted"},
		"L":          {"L", "Low"},
		"M":          {"M", "Moderate"},
		"N":          {"N", "Normal"},
		"R":          {"R", "Restricted"},
		"V":          {"V", "Very Restricted"},
		"NORMAL":     {"N", "Normal"},
		"RESTRICTED": {"R", "Restricted"},
	}

	if mapped, ok := labelMap[labelUpper]; ok {
		return CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemConfidentiality,
					Code:    mapped.code,
					Display: mapped.display,
				},
			},
		}
	}

	return CodeableConcept{
		Coding: []Coding{
			{
				System:  SystemConfidentiality,
				Code:    "N",
				Display: "Normal",
			},
		},
	}
}

// mapDocumentContent maps document content to FHIR DocumentReferenceContent.
func (m *USCoreMapper) mapDocumentContent(content events.DocumentReferenceContent) DocumentReferenceContent {
	drc := DocumentReferenceContent{
		Attachment: &Attachment{},
	}

	if content.AttachmentURL != "" {
		drc.Attachment.URL = content.AttachmentURL
	}
	if content.AttachmentData != "" {
		drc.Attachment.Data = content.AttachmentData
	}
	if content.AttachmentContentType != "" {
		drc.Attachment.ContentType = content.AttachmentContentType
	}
	if content.AttachmentSize > 0 {
		drc.Attachment.Size = content.AttachmentSize
	}
	if content.AttachmentHash != "" {
		drc.Attachment.Hash = content.AttachmentHash
	}
	if content.AttachmentTitle != "" {
		drc.Attachment.Title = content.AttachmentTitle
	}
	if content.AttachmentCreation != "" {
		drc.Attachment.Creation = content.AttachmentCreation
	}

	// Format
	if content.Format != "" {
		system := content.FormatSystem
		if system == "" {
			system = SystemDocumentFormat
		}
		drc.Format = &Coding{
			System: system,
			Code:   content.Format,
		}
	}

	return drc
}

// mapDocumentContext maps document context.
func (m *USCoreMapper) mapDocumentContext(ctx *events.DocumentReferenceContext, encounter *events.Encounter) *DocumentReferenceContext {
	drc := &DocumentReferenceContext{}

	if encounter != nil && encounter.ID != "" {
		drc.Encounter = []Reference{
			{Reference: "Encounter/" + encounter.ID},
		}
	} else if ctx != nil && ctx.EncounterID != "" {
		drc.Encounter = []Reference{
			{Reference: "Encounter/" + ctx.EncounterID},
		}
	}

	if ctx != nil {
		// Period
		if ctx.PeriodStart != "" || ctx.PeriodEnd != "" {
			drc.Period = &Period{}
			if ctx.PeriodStart != "" {
				if t, err := time.Parse("2006-01-02", ctx.PeriodStart); err == nil {
					drc.Period.Start = &t
				} else if t, err := time.Parse(time.RFC3339, ctx.PeriodStart); err == nil {
					drc.Period.Start = &t
				}
			}
			if ctx.PeriodEnd != "" {
				if t, err := time.Parse("2006-01-02", ctx.PeriodEnd); err == nil {
					drc.Period.End = &t
				} else if t, err := time.Parse(time.RFC3339, ctx.PeriodEnd); err == nil {
					drc.Period.End = &t
				}
			}
		}

		// Facility type
		if ctx.FacilityType != "" || ctx.FacilityTypeCode != "" {
			drc.FacilityType = &CodeableConcept{}
			if ctx.FacilityTypeCode != "" {
				drc.FacilityType.Coding = []Coding{
					{
						System:  SystemFacilityType,
						Code:    ctx.FacilityTypeCode,
						Display: ctx.FacilityType,
					},
				}
			}
			if ctx.FacilityType != "" {
				drc.FacilityType.Text = ctx.FacilityType
			}
		}

		// Practice setting
		if ctx.PracticeSetting != "" || ctx.PracticeSettingCode != "" {
			drc.PracticeSetting = &CodeableConcept{}
			if ctx.PracticeSettingCode != "" {
				drc.PracticeSetting.Coding = []Coding{
					{
						System:  SystemPracticeSetting,
						Code:    ctx.PracticeSettingCode,
						Display: ctx.PracticeSetting,
					},
				}
			}
			if ctx.PracticeSetting != "" {
				drc.PracticeSetting.Text = ctx.PracticeSetting
			}
		}
	}

	return drc
}

// mapDocumentRelationship maps relationship type.
func (m *USCoreMapper) mapDocumentRelationship(code string) string {
	codeLower := strings.ToLower(strings.TrimSpace(code))
	codeMap := map[string]string{
		"replaces":   "replaces",
		"transforms": "transforms",
		"signs":      "signs",
		"appends":    "appends",
		"supersedes": "replaces",
	}
	if mapped, ok := codeMap[codeLower]; ok {
		return mapped
	}
	return "replaces" // Default
}

// ============================================================================
// DiagnosticReport (Clinical Notes) Mapping (US Core)
// ============================================================================

// MapDiagnosticReportNote converts a canonical DiagnosticReportNoteEvent to a US Core DiagnosticReport.
func (m *USCoreMapper) MapDiagnosticReportNote(event *events.DiagnosticReportNoteEvent, patientRef string) *DiagnosticReportNote {
	if event == nil {
		return nil
	}

	drn := event.DiagnosticReportNote

	report := &DiagnosticReportNote{
		Meta: &Meta{
			Profile: []string{USCoreDiagnosticReportNoteProfile},
		},
		Status:  m.mapDiagnosticReportStatus(drn.Status),
		Subject: &Reference{Reference: patientRef},
	}

	// Category (required by US Core)
	report.Category = []CodeableConcept{
		m.mapDiagnosticReportCategory(drn.CategoryCode, drn.CategoryCodeSystem, drn.Category),
	}

	// Code (required by US Core)
	report.Code = m.mapDiagnosticReportCode(drn.CodeValue, drn.CodeSystem, drn.Code)

	// Encounter
	if event.Encounter != nil && event.Encounter.ID != "" {
		report.Encounter = &Reference{
			Reference: "Encounter/" + event.Encounter.ID,
		}
	}

	// Effective date/time
	if drn.EffectiveDateTime != "" {
		report.EffectiveDateTime = drn.EffectiveDateTime
	} else if drn.EffectivePeriodStart != "" || drn.EffectivePeriodEnd != "" {
		report.EffectivePeriod = &Period{}
		if drn.EffectivePeriodStart != "" {
			if t, err := time.Parse("2006-01-02", drn.EffectivePeriodStart); err == nil {
				report.EffectivePeriod.Start = &t
			} else if t, err := time.Parse(time.RFC3339, drn.EffectivePeriodStart); err == nil {
				report.EffectivePeriod.Start = &t
			}
		}
		if drn.EffectivePeriodEnd != "" {
			if t, err := time.Parse("2006-01-02", drn.EffectivePeriodEnd); err == nil {
				report.EffectivePeriod.End = &t
			} else if t, err := time.Parse(time.RFC3339, drn.EffectivePeriodEnd); err == nil {
				report.EffectivePeriod.End = &t
			}
		}
	}

	// Issued
	if drn.Issued != "" {
		report.Issued = drn.Issued
	}

	// Performer
	if event.Performer != nil {
		report.Performer = []Reference{
			{
				Reference: m.buildProviderReference(event.Performer),
				Display:   m.buildProviderDisplayName(event.Performer),
			},
		}
	} else if event.PerformerOrgID != "" || event.PerformerOrgName != "" {
		report.Performer = []Reference{
			m.buildOrganizationReference(event.PerformerOrgID, event.PerformerOrgName),
		}
	}

	// Conclusion
	if drn.Conclusion != "" {
		report.Conclusion = drn.Conclusion
	}

	// Conclusion code
	if drn.ConclusionCode != "" {
		system := drn.ConclusionCodeSystem
		if system == "" {
			system = SystemSNOMED
		}
		report.ConclusionCode = []CodeableConcept{
			{
				Coding: []Coding{
					{
						System: system,
						Code:   drn.ConclusionCode,
					},
				},
			},
		}
	}

	// Presented form (required by US Core for notes)
	if len(drn.PresentedForm) > 0 {
		for _, pf := range drn.PresentedForm {
			report.PresentedForm = append(report.PresentedForm, Attachment{
				ContentType: pf.ContentType,
				Data:        pf.Data,
				URL:         pf.URL,
				Size:        pf.Size,
				Hash:        pf.Hash,
				Title:       pf.Title,
				Creation:    pf.Creation,
			})
		}
	}

	// Media
	if len(drn.Media) > 0 {
		for _, media := range drn.Media {
			report.Media = append(report.Media, DiagnosticReportMedia{
				Comment: media.Comment,
				Link:    &Reference{Reference: "Media/" + media.LinkID},
			})
		}
	}

	// Result references
	if len(drn.ResultIDs) > 0 {
		for _, id := range drn.ResultIDs {
			report.Result = append(report.Result, Reference{
				Reference: "Observation/" + id,
			})
		}
	}

	// Imaging study references
	if len(drn.ImagingStudyIDs) > 0 {
		for _, id := range drn.ImagingStudyIDs {
			report.ImagingStudy = append(report.ImagingStudy, Reference{
				Reference: "ImagingStudy/" + id,
			})
		}
	}

	// Specimen references
	if len(drn.SpecimenIDs) > 0 {
		for _, id := range drn.SpecimenIDs {
			report.Specimen = append(report.Specimen, Reference{
				Reference: "Specimen/" + id,
			})
		}
	}

	return report
}

// mapDiagnosticReportCategory maps category to CodeableConcept.
func (m *USCoreMapper) mapDiagnosticReportCategory(code, codeSystem, text string) CodeableConcept {
	cc := CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			system = SystemLOINC
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: text,
			},
		}
	} else if text != "" {
		// Map common category names to LOINC
		catMap := map[string]struct {
			code    string
			display string
		}{
			"radiology":         {"LP29684-5", "Radiology"},
			"imaging":           {"LP29684-5", "Radiology"},
			"pathology":         {"LP7839-6", "Pathology"},
			"cardiology":        {"LP29708-2", "Cardiology"},
			"pulmonary":         {"LP29693-6", "Pulmonary function"},
			"laboratory":        {"LAB", "Laboratory"},
			"lab":               {"LAB", "Laboratory"},
			"microbiology":      {"LP7819-8", "Microbiology"},
			"hematology":        {"LP7818-0", "Hematology and coagulation"},
			"clinical note":     {"LP173421-1", "Clinical note"},
			"consultation note": {"11488-4", "Consultation note"},
			"discharge summary": {"18842-5", "Discharge summary"},
			"progress note":     {"11506-3", "Progress note"},
		}

		textLower := strings.ToLower(strings.TrimSpace(text))
		if mapped, ok := catMap[textLower]; ok {
			cc.Coding = []Coding{
				{
					System:  SystemLOINC,
					Code:    mapped.code,
					Display: mapped.display,
				},
			}
		}
	}

	if text != "" {
		cc.Text = text
	}

	return cc
}

// mapDiagnosticReportCode maps report code to CodeableConcept.
func (m *USCoreMapper) mapDiagnosticReportCode(code, codeSystem, text string) *CodeableConcept {
	cc := &CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			system = SystemLOINC // Default to LOINC
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: text,
			},
		}
	} else if text != "" {
		// Try to map common report types to LOINC
		codeMap := map[string]struct {
			code    string
			display string
		}{
			"chest x-ray":        {"36643-5", "Chest X-ray 2 Views"},
			"chest xray":         {"36643-5", "Chest X-ray 2 Views"},
			"ct scan":            {"25056-3", "CT without contrast"},
			"ct":                 {"25056-3", "CT without contrast"},
			"mri":                {"25056-9", "MRI"},
			"echocardiogram":     {"34552-0", "Echocardiography"},
			"echo":               {"34552-0", "Echocardiography"},
			"ekg":                {"11524-6", "EKG study"},
			"ecg":                {"11524-6", "EKG study"},
			"electrocardiogram":  {"11524-6", "EKG study"},
			"ultrasound":         {"25061-3", "Ultrasound"},
			"pathology":          {"11526-1", "Pathology study"},
			"surgical pathology": {"11529-5", "Surgical pathology study"},
			"colonoscopy":        {"18746-8", "Colonoscopy study"},
			"endoscopy":          {"18751-8", "Upper GI endoscopy"},
			"bone density":       {"38269-7", "DXA Bone density"},
			"mammogram":          {"26346-7", "Mammography"},
		}

		textLower := strings.ToLower(strings.TrimSpace(text))
		if mapped, ok := codeMap[textLower]; ok {
			cc.Coding = []Coding{
				{
					System:  SystemLOINC,
					Code:    mapped.code,
					Display: mapped.display,
				},
			}
		}
	}

	if text != "" {
		cc.Text = text
	}

	return cc
}

// ============================================================================
// Provenance (US Core 6.1.0)
// ============================================================================

// MapProvenance converts a canonical ProvenanceEvent to a US Core Provenance.
// US Core Provenance is required for USCDI v3 data provenance tracking.
func (m *USCoreMapper) MapProvenance(event *events.ProvenanceEvent) *Provenance {
	if event == nil {
		return nil
	}

	p := event.Provenance

	provenance := &Provenance{
		Meta: &Meta{
			Profile: []string{USCoreProvenanceProfile},
		},
		Recorded: p.Recorded,
	}

	// Target references (required by US Core - at least one)
	if len(p.TargetReferences) > 0 {
		for i, targetRef := range p.TargetReferences {
			ref := Reference{Reference: targetRef}
			if i < len(p.TargetDisplays) && p.TargetDisplays[i] != "" {
				ref.Display = p.TargetDisplays[i]
			}
			provenance.Target = append(provenance.Target, ref)
		}
	}

	// Occurred date/time or period
	if p.OccurredDateTime != "" {
		provenance.OccurredDateTime = p.OccurredDateTime
	} else if p.OccurredPeriodStart != "" || p.OccurredPeriodEnd != "" {
		provenance.OccurredPeriod = &Period{}
		if p.OccurredPeriodStart != "" {
			if t, err := time.Parse(time.RFC3339, p.OccurredPeriodStart); err == nil {
				provenance.OccurredPeriod.Start = &t
			} else if t, err := time.Parse("2006-01-02", p.OccurredPeriodStart); err == nil {
				provenance.OccurredPeriod.Start = &t
			}
		}
		if p.OccurredPeriodEnd != "" {
			if t, err := time.Parse(time.RFC3339, p.OccurredPeriodEnd); err == nil {
				provenance.OccurredPeriod.End = &t
			} else if t, err := time.Parse("2006-01-02", p.OccurredPeriodEnd); err == nil {
				provenance.OccurredPeriod.End = &t
			}
		}
	}

	// Activity (what happened)
	if p.Activity != "" || p.ActivityCode != "" {
		provenance.Activity = m.mapProvenanceActivity(p.ActivityCode, p.ActivityCodeSystem, p.Activity)
	}

	// Location reference
	if p.LocationReference != "" {
		provenance.Location = &Reference{
			Reference: p.LocationReference,
		}
		if p.LocationDisplay != "" {
			provenance.Location.Display = p.LocationDisplay
		}
	}

	// Reason
	if p.Reason != "" || p.ReasonCode != "" {
		provenance.Reason = []CodeableConcept{
			m.mapProvenanceReason(p.ReasonCode, p.ReasonCodeSystem, p.Reason),
		}
	}

	// Policy URIs
	if len(p.Policy) > 0 {
		provenance.Policy = p.Policy
	}

	// Agents (required by US Core - at least one)
	if len(p.Agents) > 0 {
		for _, agent := range p.Agents {
			provenance.Agent = append(provenance.Agent, m.mapProvenanceAgent(agent))
		}
	}

	// Entities
	if len(p.Entities) > 0 {
		for _, entity := range p.Entities {
			provenance.Entity = append(provenance.Entity, m.mapProvenanceEntity(entity))
		}
	}

	// Signatures
	if len(p.Signatures) > 0 {
		for _, sig := range p.Signatures {
			provenance.Signature = append(provenance.Signature, m.mapProvenanceSignature(sig))
		}
	}

	return provenance
}

// mapProvenanceActivity maps activity to CodeableConcept.
func (m *USCoreMapper) mapProvenanceActivity(code, codeSystem, display string) *CodeableConcept {
	cc := &CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			// Default to provenance activity type
			system = "http://terminology.hl7.org/CodeSystem/v3-DataOperation"
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: display,
			},
		}
	}

	if display != "" {
		cc.Text = display
	}

	return cc
}

// mapProvenanceReason maps reason to CodeableConcept.
func (m *USCoreMapper) mapProvenanceReason(code, codeSystem, text string) CodeableConcept {
	cc := CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			system = "http://terminology.hl7.org/CodeSystem/v3-ActReason"
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: text,
			},
		}
	}

	if text != "" {
		cc.Text = text
	}

	return cc
}

// mapProvenanceAgent maps a canonical ProvenanceAgent to FHIR ProvenanceAgent.
func (m *USCoreMapper) mapProvenanceAgent(agent events.ProvenanceAgent) ProvenanceAgent {
	fhirAgent := ProvenanceAgent{}

	// Type (how the agent participated)
	if agent.Type != "" || agent.TypeCode != "" {
		fhirAgent.Type = &CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemProvenanceParticipantType,
					Code:    agent.TypeCode,
					Display: agent.Type,
				},
			},
		}
		if agent.Type != "" {
			fhirAgent.Type.Text = agent.Type
		}
	}

	// Role
	if agent.RoleCode != "" || agent.Role != "" {
		fhirAgent.Role = []CodeableConcept{
			{
				Coding: []Coding{
					{
						System:  "http://terminology.hl7.org/CodeSystem/contractsignertypecodes",
						Code:    agent.RoleCode,
						Display: agent.Role,
					},
				},
			},
		}
		if agent.Role != "" {
			fhirAgent.Role[0].Text = agent.Role
		}
	}

	// Who (required)
	if agent.WhoReference != "" {
		fhirAgent.Who = &Reference{
			Reference: agent.WhoReference,
		}
		if agent.WhoDisplay != "" {
			fhirAgent.Who.Display = agent.WhoDisplay
		}
	}

	// OnBehalfOf
	if agent.OnBehalfOfReference != "" {
		fhirAgent.OnBehalfOf = &Reference{
			Reference: agent.OnBehalfOfReference,
		}
		if agent.OnBehalfOfDisplay != "" {
			fhirAgent.OnBehalfOf.Display = agent.OnBehalfOfDisplay
		}
	}

	return fhirAgent
}

// mapProvenanceEntity maps a canonical ProvenanceEntity to FHIR ProvenanceEntity.
func (m *USCoreMapper) mapProvenanceEntity(entity events.ProvenanceEntity) ProvenanceEntity {
	fhirEntity := ProvenanceEntity{
		Role: m.mapProvenanceEntityRole(entity.Role),
	}

	// What (required)
	if entity.WhatReference != "" {
		fhirEntity.What = &Reference{
			Reference: entity.WhatReference,
		}
		if entity.WhatDisplay != "" {
			fhirEntity.What.Display = entity.WhatDisplay
		}
	}

	return fhirEntity
}

// mapProvenanceEntityRole maps role to FHIR entity role.
func (m *USCoreMapper) mapProvenanceEntityRole(role string) string {
	roleLower := strings.ToLower(strings.TrimSpace(role))
	roleMap := map[string]string{
		"derivation": "derivation",
		"derived":    "derivation",
		"revision":   "revision",
		"revised":    "revision",
		"quotation":  "quotation",
		"quoted":     "quotation",
		"source":     "source",
		"removal":    "removal",
		"removed":    "removal",
	}

	if mapped, ok := roleMap[roleLower]; ok {
		return mapped
	}
	if role != "" {
		return role
	}
	return "source" // Default
}

// mapProvenanceSignature maps a canonical ProvenanceSignature to FHIR Signature.
func (m *USCoreMapper) mapProvenanceSignature(sig events.ProvenanceSignature) Signature {
	fhirSig := Signature{
		When: sig.When,
	}

	// Type (required)
	if sig.TypeCode != "" {
		fhirSig.Type = []Coding{
			{
				System:  "urn:iso-astm:E1762-95:2013",
				Code:    sig.TypeCode,
				Display: sig.Type,
			},
		}
	}

	// Who (required)
	if sig.WhoReference != "" {
		fhirSig.Who = &Reference{
			Reference: sig.WhoReference,
		}
		if sig.WhoDisplay != "" {
			fhirSig.Who.Display = sig.WhoDisplay
		}
	}

	// Format information
	if sig.TargetFormat != "" {
		fhirSig.TargetFormat = sig.TargetFormat
	}
	if sig.SigFormat != "" {
		fhirSig.SigFormat = sig.SigFormat
	}
	if sig.Data != "" {
		fhirSig.Data = sig.Data
	}

	return fhirSig
}

// ============================================================================
// Location (US Core 6.1.0)
// ============================================================================

// MapLocation converts a canonical FacilityLocationEvent to a US Core Location.
func (m *USCoreMapper) MapLocation(event *events.FacilityLocationEvent) *FHIRLocation {
	if event == nil {
		return nil
	}

	loc := event.FacilityLocation

	location := &FHIRLocation{
		Meta: &Meta{
			Profile: []string{USCoreLocationProfile},
		},
		Name: loc.Name, // Required by US Core
	}

	// ID as identifier
	if loc.ID != "" {
		location.Identifier = []Identifier{
			{
				System: "urn:ietf:rfc:3986",
				Value:  loc.ID,
			},
		}
	}

	// Status
	if loc.Status != "" {
		location.Status = m.mapLocationStatus(loc.Status)
	}

	// Description
	if loc.Description != "" {
		location.Description = loc.Description
	}

	// Mode (instance or kind)
	if loc.Mode != "" {
		location.Mode = loc.Mode
	}

	// Type
	if loc.Type != "" || loc.TypeCode != "" {
		location.Type = []CodeableConcept{
			m.mapLocationType(loc.TypeCode, loc.TypeCodeSystem, loc.Type),
		}
	}

	// Address (required by US Core)
	if loc.Address != nil {
		addr := m.mapAddress(loc.Address)
		location.Address = &addr
	}

	// Physical type
	if loc.PhysicalType != "" || loc.PhysicalTypeCode != "" {
		location.PhysicalType = &CodeableConcept{
			Coding: []Coding{
				{
					System:  "http://terminology.hl7.org/CodeSystem/location-physical-type",
					Code:    loc.PhysicalTypeCode,
					Display: loc.PhysicalType,
				},
			},
		}
		if loc.PhysicalType != "" {
			location.PhysicalType.Text = loc.PhysicalType
		}
	}

	// Managing organization
	if loc.ManagingOrganizationID != "" || loc.ManagingOrganizationName != "" {
		location.ManagingOrganization = &Reference{}
		if loc.ManagingOrganizationID != "" {
			location.ManagingOrganization.Reference = "Organization/" + loc.ManagingOrganizationID
		}
		if loc.ManagingOrganizationName != "" {
			location.ManagingOrganization.Display = loc.ManagingOrganizationName
		}
	}

	// Part of (parent location)
	if loc.PartOfLocationID != "" {
		location.PartOf = &Reference{
			Reference: "Location/" + loc.PartOfLocationID,
		}
	}

	// Telecom
	if loc.Phone != "" || loc.Email != "" {
		if loc.Phone != "" {
			location.Telecom = append(location.Telecom, ContactPoint{
				System: "phone",
				Value:  loc.Phone,
				Use:    "work",
			})
		}
		if loc.Email != "" {
			location.Telecom = append(location.Telecom, ContactPoint{
				System: "email",
				Value:  loc.Email,
				Use:    "work",
			})
		}
	}

	return location
}

// mapLocationStatus maps canonical status to FHIR location status.
func (m *USCoreMapper) mapLocationStatus(status string) string {
	statusLower := strings.ToLower(strings.TrimSpace(status))
	statusMap := map[string]string{
		"active":    "active",
		"suspended": "suspended",
		"inactive":  "inactive",
	}

	if mapped, ok := statusMap[statusLower]; ok {
		return mapped
	}
	return "active" // Default
}

// mapLocationType maps location type to CodeableConcept.
func (m *USCoreMapper) mapLocationType(code, codeSystem, display string) CodeableConcept {
	cc := CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			system = "http://terminology.hl7.org/CodeSystem/v3-RoleCode"
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: display,
			},
		}
	}

	if display != "" {
		cc.Text = display
	}

	return cc
}

// ============================================================================
// Organization (US Core 6.1.0)
// ============================================================================

// MapOrganization converts a canonical OrganizationEvent to a US Core Organization.
func (m *USCoreMapper) MapOrganization(event *events.OrganizationEvent) *FHIROrganization {
	if event == nil {
		return nil
	}

	org := event.Organization

	organization := &FHIROrganization{
		Meta: &Meta{
			Profile: []string{USCoreOrganizationProfile},
		},
		Name: org.Name, // Required by US Core
	}

	// Identifiers (US Core requires NPI for healthcare organizations)
	if org.NPI != "" {
		organization.Identifier = append(organization.Identifier, Identifier{
			System: "http://hl7.org/fhir/sid/us-npi",
			Value:  org.NPI,
		})
	}
	if org.TIN != "" {
		organization.Identifier = append(organization.Identifier, Identifier{
			System: "urn:oid:2.16.840.1.113883.4.4", // IRS TIN
			Value:  org.TIN,
		})
	}
	if org.ID != "" && org.NPI == "" {
		organization.Identifier = append(organization.Identifier, Identifier{
			System: "urn:ietf:rfc:3986",
			Value:  org.ID,
		})
	}

	// Active status
	if org.Active {
		organization.Active = &org.Active
	}

	// Type
	if org.Type != "" || org.TypeCode != "" {
		organization.Type = []CodeableConcept{
			m.mapOrganizationType(org.TypeCode, org.TypeCodeSystem, org.Type),
		}
	}

	// Aliases
	if len(org.Alias) > 0 {
		organization.Alias = org.Alias
	}

	// Address (required by US Core)
	if org.Address != nil {
		organization.Address = []Address{m.mapAddress(org.Address)}
	}

	// Telecom
	if org.Phone != "" || org.Email != "" {
		if org.Phone != "" {
			organization.Telecom = append(organization.Telecom, ContactPoint{
				System: "phone",
				Value:  org.Phone,
				Use:    "work",
			})
		}
		if org.Email != "" {
			organization.Telecom = append(organization.Telecom, ContactPoint{
				System: "email",
				Value:  org.Email,
				Use:    "work",
			})
		}
	}

	// Part of (parent organization)
	if org.PartOfOrganizationID != "" || org.PartOfOrganizationName != "" {
		organization.PartOf = &Reference{}
		if org.PartOfOrganizationID != "" {
			organization.PartOf.Reference = "Organization/" + org.PartOfOrganizationID
		}
		if org.PartOfOrganizationName != "" {
			organization.PartOf.Display = org.PartOfOrganizationName
		}
	}

	return organization
}

// mapOrganizationType maps organization type to CodeableConcept.
func (m *USCoreMapper) mapOrganizationType(code, codeSystem, display string) CodeableConcept {
	cc := CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			system = SystemOrganizationType
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: display,
			},
		}
	}

	if display != "" {
		cc.Text = display
	}

	return cc
}

// ============================================================================
// Practitioner (US Core 6.1.0)
// ============================================================================

// MapPractitioner converts a canonical PractitionerEvent to a US Core Practitioner.
func (m *USCoreMapper) MapPractitioner(event *events.PractitionerEvent) *FHIRPractitioner {
	if event == nil {
		return nil
	}

	prac := event.Practitioner

	practitioner := &FHIRPractitioner{
		Meta: &Meta{
			Profile: []string{USCorePractitionerProfile},
		},
	}

	// Identifiers (US Core requires NPI)
	if prac.NPI != "" {
		practitioner.Identifier = append(practitioner.Identifier, Identifier{
			System: "http://hl7.org/fhir/sid/us-npi",
			Value:  prac.NPI,
		})
	}
	if prac.ID != "" && prac.NPI == "" {
		practitioner.Identifier = append(practitioner.Identifier, Identifier{
			System: "urn:ietf:rfc:3986",
			Value:  prac.ID,
		})
	}

	// Active status
	if prac.Active {
		practitioner.Active = &prac.Active
	}

	// Name (required by US Core)
	name := HumanName{
		Use:    "official",
		Family: prac.FamilyName,
	}
	if prac.GivenName != "" {
		name.Given = append(name.Given, prac.GivenName)
	}
	if prac.MiddleName != "" {
		name.Given = append(name.Given, prac.MiddleName)
	}
	if prac.Prefix != "" {
		name.Prefix = []string{prac.Prefix}
	}
	if prac.Suffix != "" {
		name.Suffix = []string{prac.Suffix}
	}
	practitioner.Name = []HumanName{name}

	// Gender
	if prac.Gender != "" {
		practitioner.Gender = m.mapGender(prac.Gender)
	}

	// Birth date
	if prac.BirthDate != "" {
		practitioner.BirthDate = prac.BirthDate
	}

	// Address
	if prac.Address != nil {
		practitioner.Address = []Address{m.mapAddress(prac.Address)}
	}

	// Telecom
	if prac.Phone != "" || prac.Email != "" {
		if prac.Phone != "" {
			practitioner.Telecom = append(practitioner.Telecom, ContactPoint{
				System: "phone",
				Value:  prac.Phone,
				Use:    "work",
			})
		}
		if prac.Email != "" {
			practitioner.Telecom = append(practitioner.Telecom, ContactPoint{
				System: "email",
				Value:  prac.Email,
				Use:    "work",
			})
		}
	}

	// Qualifications
	if len(prac.Qualifications) > 0 {
		for _, qual := range prac.Qualifications {
			practitioner.Qualification = append(practitioner.Qualification, m.mapPractitionerQualification(qual))
		}
	}

	// Communication (languages)
	if len(prac.Languages) > 0 {
		for _, lang := range prac.Languages {
			practitioner.Communication = append(practitioner.Communication, CodeableConcept{
				Coding: []Coding{
					{
						System: "urn:ietf:bcp:47",
						Code:   lang,
					},
				},
				Text: lang,
			})
		}
	}

	return practitioner
}

// mapPractitionerQualification maps a canonical PractitionerQualification to FHIR.
func (m *USCoreMapper) mapPractitionerQualification(qual events.PractitionerQualification) PractitionerQualification {
	fhirQual := PractitionerQualification{}

	// Code (required)
	if qual.Code != "" || qual.Display != "" {
		fhirQual.Code = &CodeableConcept{
			Coding: []Coding{
				{
					System:  qual.CodeSystem,
					Code:    qual.Code,
					Display: qual.Display,
				},
			},
		}
		if qual.Display != "" {
			fhirQual.Code.Text = qual.Display
		}
	}

	// Period
	if qual.PeriodStart != "" || qual.PeriodEnd != "" {
		fhirQual.Period = &Period{}
		if qual.PeriodStart != "" {
			if t, err := time.Parse("2006-01-02", qual.PeriodStart); err == nil {
				fhirQual.Period.Start = &t
			}
		}
		if qual.PeriodEnd != "" {
			if t, err := time.Parse("2006-01-02", qual.PeriodEnd); err == nil {
				fhirQual.Period.End = &t
			}
		}
	}

	// Issuer
	if qual.IssuerID != "" || qual.IssuerName != "" {
		fhirQual.Issuer = &Reference{}
		if qual.IssuerID != "" {
			fhirQual.Issuer.Reference = "Organization/" + qual.IssuerID
		}
		if qual.IssuerName != "" {
			fhirQual.Issuer.Display = qual.IssuerName
		}
	}

	return fhirQual
}

// ============================================================================
// PractitionerRole (US Core 6.1.0)
// ============================================================================

// MapPractitionerRole converts a canonical PractitionerRoleEvent to a US Core PractitionerRole.
func (m *USCoreMapper) MapPractitionerRole(event *events.PractitionerRoleEvent) *FHIRPractitionerRole {
	if event == nil {
		return nil
	}

	pr := event.PractitionerRole

	practitionerRole := &FHIRPractitionerRole{
		Meta: &Meta{
			Profile: []string{USCorePractitionerRoleProfile},
		},
	}

	// ID as identifier
	if pr.ID != "" {
		practitionerRole.Identifier = []Identifier{
			{
				System: "urn:ietf:rfc:3986",
				Value:  pr.ID,
			},
		}
	}

	// Active status
	if pr.Active {
		practitionerRole.Active = &pr.Active
	}

	// Period
	if pr.PeriodStart != "" || pr.PeriodEnd != "" {
		practitionerRole.Period = &Period{}
		if pr.PeriodStart != "" {
			if t, err := time.Parse("2006-01-02", pr.PeriodStart); err == nil {
				practitionerRole.Period.Start = &t
			} else if t, err := time.Parse(time.RFC3339, pr.PeriodStart); err == nil {
				practitionerRole.Period.Start = &t
			}
		}
		if pr.PeriodEnd != "" {
			if t, err := time.Parse("2006-01-02", pr.PeriodEnd); err == nil {
				practitionerRole.Period.End = &t
			} else if t, err := time.Parse(time.RFC3339, pr.PeriodEnd); err == nil {
				practitionerRole.Period.End = &t
			}
		}
	}

	// Practitioner reference (required by US Core)
	if pr.PractitionerID != "" {
		practitionerRole.Practitioner = &Reference{
			Reference: "Practitioner/" + pr.PractitionerID,
		}
		if pr.PractitionerName != "" {
			practitionerRole.Practitioner.Display = pr.PractitionerName
		}
	}

	// Organization reference (required by US Core)
	if pr.OrganizationID != "" {
		practitionerRole.Organization = &Reference{
			Reference: "Organization/" + pr.OrganizationID,
		}
		if pr.OrganizationName != "" {
			practitionerRole.Organization.Display = pr.OrganizationName
		}
	}

	// Code (role)
	if pr.Code != "" || pr.CodeValue != "" {
		practitionerRole.Code = []CodeableConcept{
			m.mapPractitionerRoleCode(pr.CodeValue, pr.CodeSystem, pr.Code),
		}
	}

	// Specialty
	if pr.Specialty != "" || pr.SpecialtyCode != "" {
		practitionerRole.Specialty = []CodeableConcept{
			m.mapPractitionerRoleSpecialty(pr.SpecialtyCode, pr.SpecialtyCodeSystem, pr.Specialty),
		}
	}

	// Locations
	if len(pr.LocationIDs) > 0 {
		for _, locID := range pr.LocationIDs {
			practitionerRole.Location = append(practitionerRole.Location, Reference{
				Reference: "Location/" + locID,
			})
		}
	}

	// Telecom
	if pr.Phone != "" || pr.Email != "" {
		if pr.Phone != "" {
			practitionerRole.Telecom = append(practitionerRole.Telecom, ContactPoint{
				System: "phone",
				Value:  pr.Phone,
				Use:    "work",
			})
		}
		if pr.Email != "" {
			practitionerRole.Telecom = append(practitionerRole.Telecom, ContactPoint{
				System: "email",
				Value:  pr.Email,
				Use:    "work",
			})
		}
	}

	return practitionerRole
}

// mapPractitionerRoleCode maps role code to CodeableConcept.
func (m *USCoreMapper) mapPractitionerRoleCode(code, codeSystem, display string) CodeableConcept {
	cc := CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			system = SystemPractitionerRole
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: display,
			},
		}
	}

	if display != "" {
		cc.Text = display
	}

	return cc
}

// mapPractitionerRoleSpecialty maps specialty to CodeableConcept.
func (m *USCoreMapper) mapPractitionerRoleSpecialty(code, codeSystem, display string) CodeableConcept {
	cc := CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			// Default to NUCC Health Care Provider Taxonomy
			system = "http://nucc.org/provider-taxonomy"
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: display,
			},
		}
	}

	if display != "" {
		cc.Text = display
	}

	return cc
}

// ============================================================================
// RelatedPerson (US Core 6.1.0)
// ============================================================================

// MapRelatedPerson converts a canonical RelatedPersonEvent to a US Core RelatedPerson.
func (m *USCoreMapper) MapRelatedPerson(event *events.RelatedPersonEvent, patientRef string) *FHIRRelatedPerson {
	if event == nil {
		return nil
	}

	rp := event.RelatedPerson

	relatedPerson := &FHIRRelatedPerson{
		Meta: &Meta{
			Profile: []string{USCoreRelatedPersonProfile},
		},
		Patient: &Reference{Reference: patientRef}, // Required
	}

	// ID as identifier
	if rp.ID != "" {
		relatedPerson.Identifier = []Identifier{
			{
				System: "urn:ietf:rfc:3986",
				Value:  rp.ID,
			},
		}
	}

	// Active status
	if rp.Active {
		relatedPerson.Active = &rp.Active
	}

	// Relationship (required by US Core)
	if rp.Relationship != "" || rp.RelationshipCode != "" {
		relatedPerson.Relationship = []CodeableConcept{
			m.mapRelatedPersonRelationship(rp.RelationshipCode, rp.RelationshipCodeSystem, rp.Relationship),
		}
	}

	// Name
	if rp.GivenName != "" || rp.FamilyName != "" {
		name := HumanName{
			Use:    "official",
			Family: rp.FamilyName,
		}
		if rp.GivenName != "" {
			name.Given = append(name.Given, rp.GivenName)
		}
		if rp.MiddleName != "" {
			name.Given = append(name.Given, rp.MiddleName)
		}
		if rp.Prefix != "" {
			name.Prefix = []string{rp.Prefix}
		}
		if rp.Suffix != "" {
			name.Suffix = []string{rp.Suffix}
		}
		relatedPerson.Name = []HumanName{name}
	}

	// Gender
	if rp.Gender != "" {
		relatedPerson.Gender = m.mapGender(rp.Gender)
	}

	// Birth date
	if rp.BirthDate != "" {
		relatedPerson.BirthDate = rp.BirthDate
	}

	// Address
	if rp.Address != nil {
		relatedPerson.Address = []Address{m.mapAddress(rp.Address)}
	}

	// Telecom
	if rp.Phone != "" || rp.Email != "" {
		if rp.Phone != "" {
			relatedPerson.Telecom = append(relatedPerson.Telecom, ContactPoint{
				System: "phone",
				Value:  rp.Phone,
				Use:    "home",
			})
		}
		if rp.Email != "" {
			relatedPerson.Telecom = append(relatedPerson.Telecom, ContactPoint{
				System: "email",
				Value:  rp.Email,
				Use:    "home",
			})
		}
	}

	// Period
	if rp.PeriodStart != "" || rp.PeriodEnd != "" {
		relatedPerson.Period = &Period{}
		if rp.PeriodStart != "" {
			if t, err := time.Parse("2006-01-02", rp.PeriodStart); err == nil {
				relatedPerson.Period.Start = &t
			} else if t, err := time.Parse(time.RFC3339, rp.PeriodStart); err == nil {
				relatedPerson.Period.Start = &t
			}
		}
		if rp.PeriodEnd != "" {
			if t, err := time.Parse("2006-01-02", rp.PeriodEnd); err == nil {
				relatedPerson.Period.End = &t
			} else if t, err := time.Parse(time.RFC3339, rp.PeriodEnd); err == nil {
				relatedPerson.Period.End = &t
			}
		}
	}

	// Communication (languages)
	if len(rp.Languages) > 0 {
		for _, lang := range rp.Languages {
			relatedPerson.Communication = append(relatedPerson.Communication, RelatedPersonCommunication{
				Language: &CodeableConcept{
					Coding: []Coding{
						{
							System: "urn:ietf:bcp:47",
							Code:   lang,
						},
					},
					Text: lang,
				},
			})
		}
	}

	return relatedPerson
}

// mapRelatedPersonRelationship maps relationship to CodeableConcept.
func (m *USCoreMapper) mapRelatedPersonRelationship(code, codeSystem, display string) CodeableConcept {
	cc := CodeableConcept{}

	if code != "" {
		system := codeSystem
		if system == "" {
			system = SystemRelatedPersonRelationship
		}
		cc.Coding = []Coding{
			{
				System:  system,
				Code:    code,
				Display: display,
			},
		}
	}

	// Map common relationship terms to codes
	if code == "" && display != "" {
		codeMap := map[string]struct {
			code    string
			display string
		}{
			"mother":      {"MTH", "mother"},
			"father":      {"FTH", "father"},
			"parent":      {"PRN", "parent"},
			"spouse":      {"SPS", "spouse"},
			"child":       {"CHILD", "child"},
			"sibling":     {"SIB", "sibling"},
			"guardian":    {"GUARD", "guardian"},
			"caregiver":   {"CAREGIVER", "caregiver"},
			"emergency":   {"ECON", "emergency contact"},
			"contact":     {"C", "contact"},
			"next of kin": {"N", "next of kin"},
		}

		displayLower := strings.ToLower(strings.TrimSpace(display))
		if mapped, ok := codeMap[displayLower]; ok {
			cc.Coding = []Coding{
				{
					System:  SystemRelatedPersonRelationship,
					Code:    mapped.code,
					Display: mapped.display,
				},
			}
		}
	}

	if display != "" {
		cc.Text = display
	}

	return cc
}
