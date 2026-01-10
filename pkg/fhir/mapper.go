package fhir

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cblevins/fi-fhir/pkg/events"
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

	// Map identifiers
	patient.Identifier = m.mapIdentifiers(&p.Identifiers)

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
			Profile: []string{USCoreBaseURL + "us-core-diagnosticreport-lab"},
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

	// Add LOINC coding if available
	if test.LOINCCode != "" {
		result.Coding = append(result.Coding, Coding{
			System:  SystemLOINC,
			Code:    test.LOINCCode,
			Display: test.Description,
		})
	}

	// Add codings from CodeableConcept
	for _, coding := range test.Code.Coding {
		result.Coding = append(result.Coding, Coding{
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
		"LA":    {"72098002", "Left arm"},
		"RA":    {"59126009", "Right arm"},
		"LT":    {"61396006", "Left thigh"},
		"RT":    {"11207009", "Right thigh"},
		"LLFA":  {"66480008", "Left lower forearm"},
		"RLFA":  {"64262003", "Right lower forearm"},
		"LD":    {"46862004", "Left deltoid"},
		"RD":    {"91775009", "Right deltoid"},
		"LG":    {"85562004", "Left gluteal"},
		"RG":    {"78067005", "Right gluteal"},
		"LVL":   {"64688005", "Left vastus lateralis"},
		"RVL":   {"11207009", "Right vastus lateralis"},
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
		"°c":          "Cel",
		"°f":          "[degF]",
		"celsius":     "Cel",
		"fahrenheit":  "[degF]",
		"c":           "Cel",
		"f":           "[degF]",

		// Heart/Respiratory rate
		"bpm":         "/min",
		"beats/min":   "/min",
		"breaths/min": "/min",
		"/min":        "/min",

		// Height
		"cm":          "cm",
		"in":          "[in_i]",
		"inches":      "[in_i]",
		"m":           "m",
		"ft":          "[ft_i]",
		"feet":        "[ft_i]",

		// Weight
		"kg":          "kg",
		"lb":          "[lb_av]",
		"lbs":         "[lb_av]",
		"pounds":      "[lb_av]",
		"oz":          "[oz_av]",
		"g":           "g",

		// Blood pressure
		"mmhg":        "mm[Hg]",
		"mm hg":       "mm[Hg]",

		// Oxygen saturation
		"%":           "%",
		"percent":     "%",

		// BMI
		"kg/m2":       "kg/m2",
		"kg/m^2":      "kg/m2",
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
		"normal":          {"N", "Normal"},
		"n":               {"N", "Normal"},
		"high":            {"H", "High"},
		"h":               {"H", "High"},
		"low":             {"L", "Low"},
		"l":               {"L", "Low"},
		"critical":        {"AA", "Critical abnormal"},
		"critical high":   {"HH", "Critical high"},
		"critical low":    {"LL", "Critical low"},
		"hh":              {"HH", "Critical high"},
		"ll":              {"LL", "Critical low"},
		"abnormal":        {"A", "Abnormal"},
		"a":               {"A", "Abnormal"},
		"very high":       {"HH", "Critical high"},
		"very low":        {"LL", "Critical low"},
		"panic":           {"AA", "Critical abnormal"},
		"panic high":      {"HH", "Critical high"},
		"panic low":       {"LL", "Critical low"},
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
		code      string
		frequency int
		period    float64
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
		"low":               "low",
		"high":              "high",
		"unable-to-assess":  "unable-to-assess",
		"unable to assess":  "unable-to-assess",
		"unknown":           "unable-to-assess",
		"critical":          "high",
		"life-threatening":  "high",
		"life threatening":  "high",
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
		"rash":               {"271807003", "Rash"},
		"hives":              {"126485001", "Urticaria"},
		"urticaria":          {"126485001", "Urticaria"},
		"itching":            {"418290006", "Itching"},
		"pruritus":           {"418290006", "Itching"},
		"swelling":           {"65124004", "Swelling"},
		"angioedema":         {"41291007", "Angioedema"},
		"anaphylaxis":        {"39579001", "Anaphylaxis"},
		"anaphylactic shock": {"39579001", "Anaphylaxis"},
		"nausea":             {"422587007", "Nausea"},
		"vomiting":           {"422400008", "Vomiting"},
		"diarrhea":           {"62315008", "Diarrhea"},
		"difficulty breathing": {"267036007", "Dyspnea"},
		"dyspnea":            {"267036007", "Dyspnea"},
		"wheezing":           {"56018004", "Wheezing"},
		"throat swelling":    {"262577005", "Throat swelling"},
		"headache":           {"25064002", "Headache"},
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
