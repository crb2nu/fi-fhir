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
