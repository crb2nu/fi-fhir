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
