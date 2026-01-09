// Package fhir provides FHIR R4 resource types and mapping utilities.
// These types support US Core 6.1.0 profiles for regulatory compliance.
package fhir

import (
	"encoding/json"
	"time"
)

// Standard FHIR R4 URIs
const (
	// Profile URIs
	USCoreBaseURL = "http://hl7.org/fhir/us/core/StructureDefinition/"

	USCorePatientProfile        = USCoreBaseURL + "us-core-patient"
	USCoreObservationLabProfile = USCoreBaseURL + "us-core-observation-lab"
	USCoreEncounterProfile      = USCoreBaseURL + "us-core-encounter"
	USCoreConditionProfile      = USCoreBaseURL + "us-core-condition-problems-health-concerns"

	// Extension URIs
	USCoreRaceExtension      = USCoreBaseURL + "us-core-race"
	USCoreEthnicityExtension = USCoreBaseURL + "us-core-ethnicity"
	USCoreBirthSexExtension  = USCoreBaseURL + "us-core-birthsex"

	// Code system URIs
	SystemLOINC                = "http://loinc.org"
	SystemSNOMED               = "http://snomed.info/sct"
	SystemICD10CM              = "http://hl7.org/fhir/sid/icd-10-cm"
	SystemCPT                  = "http://www.ama-assn.org/go/cpt"
	SystemRxNorm               = "http://www.nlm.nih.gov/research/umls/rxnorm"
	SystemUCUM                 = "http://unitsofmeasure.org"
	SystemAdministrativeGender = "http://hl7.org/fhir/administrative-gender"
	SystemObservationCategory  = "http://terminology.hl7.org/CodeSystem/observation-category"
	SystemInterpretation       = "http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation"
	SystemEncounterClass       = "http://terminology.hl7.org/CodeSystem/v3-ActCode"
	SystemIdentifierType       = "http://terminology.hl7.org/CodeSystem/v2-0203"

	// Race/ethnicity system (CDC Race & Ethnicity)
	SystemCDCRaceEthnicity = "urn:oid:2.16.840.1.113883.6.238"
)

// Resource is the base interface for all FHIR resources.
type Resource interface {
	GetResourceType() string
}

// Meta contains resource metadata.
type Meta struct {
	Profile     []string   `json:"profile,omitempty"`
	VersionID   string     `json:"versionId,omitempty"`
	LastUpdated *time.Time `json:"lastUpdated,omitempty"`
}

// Identifier represents a business identifier.
type Identifier struct {
	System   string           `json:"system,omitempty"`
	Value    string           `json:"value,omitempty"`
	Type     *CodeableConcept `json:"type,omitempty"`
	Use      string           `json:"use,omitempty"` // usual | official | temp | secondary | old
	Assigner *Reference       `json:"assigner,omitempty"`
	Period   *Period          `json:"period,omitempty"`
}

// CodeableConcept represents a value with one or more code representations.
type CodeableConcept struct {
	Coding []Coding `json:"coding,omitempty"`
	Text   string   `json:"text,omitempty"`
}

// Coding represents a code from a terminology system.
type Coding struct {
	System       string `json:"system,omitempty"`
	Version      string `json:"version,omitempty"`
	Code         string `json:"code,omitempty"`
	Display      string `json:"display,omitempty"`
	UserSelected bool   `json:"userSelected,omitempty"`
}

// Reference is a reference to another resource.
type Reference struct {
	Reference string `json:"reference,omitempty"`
	Type      string `json:"type,omitempty"`
	Display   string `json:"display,omitempty"`
}

// Period represents a time range.
type Period struct {
	Start *time.Time `json:"start,omitempty"`
	End   *time.Time `json:"end,omitempty"`
}

// Extension represents a FHIR extension.
type Extension struct {
	URL         string      `json:"url"`
	ValueString string      `json:"valueString,omitempty"`
	ValueCoding *Coding     `json:"valueCoding,omitempty"`
	ValueCode   string      `json:"valueCode,omitempty"`
	Extension   []Extension `json:"extension,omitempty"` // Nested extensions
}

// HumanName represents a person's name.
type HumanName struct {
	Use    string   `json:"use,omitempty"` // usual | official | temp | nickname | anonymous | old | maiden
	Family string   `json:"family,omitempty"`
	Given  []string `json:"given,omitempty"`
	Prefix []string `json:"prefix,omitempty"`
	Suffix []string `json:"suffix,omitempty"`
	Text   string   `json:"text,omitempty"`
	Period *Period  `json:"period,omitempty"`
}

// Address represents a postal address.
type Address struct {
	Use        string   `json:"use,omitempty"`  // home | work | temp | old | billing
	Type       string   `json:"type,omitempty"` // postal | physical | both
	Line       []string `json:"line,omitempty"`
	City       string   `json:"city,omitempty"`
	State      string   `json:"state,omitempty"`
	PostalCode string   `json:"postalCode,omitempty"`
	Country    string   `json:"country,omitempty"`
	Period     *Period  `json:"period,omitempty"`
}

// ContactPoint represents contact details (phone, email, etc.).
type ContactPoint struct {
	System string  `json:"system,omitempty"` // phone | fax | email | pager | url | sms | other
	Value  string  `json:"value,omitempty"`
	Use    string  `json:"use,omitempty"` // home | work | temp | old | mobile
	Rank   int     `json:"rank,omitempty"`
	Period *Period `json:"period,omitempty"`
}

// Quantity represents a measured amount with unit.
type Quantity struct {
	Value      float64 `json:"value,omitempty"`
	Comparator string  `json:"comparator,omitempty"` // < | <= | >= | >
	Unit       string  `json:"unit,omitempty"`
	System     string  `json:"system,omitempty"`
	Code       string  `json:"code,omitempty"`
}

// Range represents a value range.
type Range struct {
	Low  *Quantity `json:"low,omitempty"`
	High *Quantity `json:"high,omitempty"`
}

// ReferenceRange for observations.
type ReferenceRange struct {
	Low  *Quantity        `json:"low,omitempty"`
	High *Quantity        `json:"high,omitempty"`
	Type *CodeableConcept `json:"type,omitempty"`
	Text string           `json:"text,omitempty"`
}

// Patient represents a FHIR Patient resource.
type Patient struct {
	ResourceType        string                 `json:"resourceType"`
	ID                  string                 `json:"id,omitempty"`
	Meta                *Meta                  `json:"meta,omitempty"`
	Extension           []Extension            `json:"extension,omitempty"`
	Identifier          []Identifier           `json:"identifier,omitempty"`
	Active              *bool                  `json:"active,omitempty"`
	Name                []HumanName            `json:"name,omitempty"`
	Telecom             []ContactPoint         `json:"telecom,omitempty"`
	Gender              string                 `json:"gender,omitempty"`    // male | female | other | unknown
	BirthDate           string                 `json:"birthDate,omitempty"` // YYYY-MM-DD
	Address             []Address              `json:"address,omitempty"`
	MaritalStatus       *CodeableConcept       `json:"maritalStatus,omitempty"`
	Communication       []PatientCommunication `json:"communication,omitempty"`
	GeneralPractitioner []Reference            `json:"generalPractitioner,omitempty"`
}

// PatientCommunication represents patient language preferences.
type PatientCommunication struct {
	Language  *CodeableConcept `json:"language"`
	Preferred bool             `json:"preferred,omitempty"`
}

// GetResourceType returns "Patient".
func (p *Patient) GetResourceType() string {
	return "Patient"
}

// MarshalJSON ensures ResourceType is always set.
func (p *Patient) MarshalJSON() ([]byte, error) {
	type Alias Patient
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "Patient",
		Alias:        (*Alias)(p),
	})
}

// Observation represents a FHIR Observation resource.
type Observation struct {
	ResourceType         string            `json:"resourceType"`
	ID                   string            `json:"id,omitempty"`
	Meta                 *Meta             `json:"meta,omitempty"`
	Identifier           []Identifier      `json:"identifier,omitempty"`
	Status               string            `json:"status"` // registered | preliminary | final | amended | corrected | cancelled | entered-in-error | unknown
	Category             []CodeableConcept `json:"category,omitempty"`
	Code                 CodeableConcept   `json:"code"`
	Subject              *Reference        `json:"subject,omitempty"`
	Encounter            *Reference        `json:"encounter,omitempty"`
	EffectiveDateTime    string            `json:"effectiveDateTime,omitempty"`
	Issued               *time.Time        `json:"issued,omitempty"`
	Performer            []Reference       `json:"performer,omitempty"`
	ValueQuantity        *Quantity         `json:"valueQuantity,omitempty"`
	ValueString          string            `json:"valueString,omitempty"`
	ValueCodeableConcept *CodeableConcept  `json:"valueCodeableConcept,omitempty"`
	Interpretation       []CodeableConcept `json:"interpretation,omitempty"`
	Note                 []Annotation      `json:"note,omitempty"`
	ReferenceRange       []ReferenceRange  `json:"referenceRange,omitempty"`
}

// Annotation represents a text note with optional author.
type Annotation struct {
	Text string `json:"text"`
}

// GetResourceType returns "Observation".
func (o *Observation) GetResourceType() string {
	return "Observation"
}

// MarshalJSON ensures ResourceType is always set.
func (o *Observation) MarshalJSON() ([]byte, error) {
	type Alias Observation
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "Observation",
		Alias:        (*Alias)(o),
	})
}

// Encounter represents a FHIR Encounter resource.
type Encounter struct {
	ResourceType    string                 `json:"resourceType"`
	ID              string                 `json:"id,omitempty"`
	Meta            *Meta                  `json:"meta,omitempty"`
	Identifier      []Identifier           `json:"identifier,omitempty"`
	Status          string                 `json:"status"` // planned | arrived | triaged | in-progress | onleave | finished | cancelled | entered-in-error | unknown
	Class           Coding                 `json:"class"`
	Type            []CodeableConcept      `json:"type,omitempty"`
	Subject         *Reference             `json:"subject,omitempty"`
	Participant     []EncounterParticipant `json:"participant,omitempty"`
	Period          *Period                `json:"period,omitempty"`
	ReasonCode      []CodeableConcept      `json:"reasonCode,omitempty"`
	Hospitalization *Hospitalization       `json:"hospitalization,omitempty"`
	Location        []EncounterLocation    `json:"location,omitempty"`
	ServiceProvider *Reference             `json:"serviceProvider,omitempty"`
}

// EncounterParticipant represents a participant in an encounter.
type EncounterParticipant struct {
	Type       []CodeableConcept `json:"type,omitempty"`
	Individual *Reference        `json:"individual,omitempty"`
	Period     *Period           `json:"period,omitempty"`
}

// Hospitalization contains admission/discharge details.
type Hospitalization struct {
	AdmitSource          *CodeableConcept `json:"admitSource,omitempty"`
	DischargeDisposition *CodeableConcept `json:"dischargeDisposition,omitempty"`
}

// EncounterLocation represents a location during an encounter.
type EncounterLocation struct {
	Location *Reference `json:"location,omitempty"`
	Status   string     `json:"status,omitempty"` // planned | active | reserved | completed
	Period   *Period    `json:"period,omitempty"`
}

// GetResourceType returns "Encounter".
func (e *Encounter) GetResourceType() string {
	return "Encounter"
}

// MarshalJSON ensures ResourceType is always set.
func (e *Encounter) MarshalJSON() ([]byte, error) {
	type Alias Encounter
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "Encounter",
		Alias:        (*Alias)(e),
	})
}

// Bundle represents a FHIR Bundle containing multiple resources.
type Bundle struct {
	ResourceType string        `json:"resourceType"`
	ID           string        `json:"id,omitempty"`
	Meta         *Meta         `json:"meta,omitempty"`
	Type         string        `json:"type"` // document | message | transaction | transaction-response | batch | batch-response | history | searchset | collection
	Total        int           `json:"total,omitempty"`
	Link         []BundleLink  `json:"link,omitempty"`
	Entry        []BundleEntry `json:"entry,omitempty"`
}

// BundleLink represents a link in a bundle (for pagination, etc.).
type BundleLink struct {
	Relation string `json:"relation"` // self | first | previous | next | last
	URL      string `json:"url"`
}

// BundleEntry represents an entry in a bundle.
type BundleEntry struct {
	FullURL  string               `json:"fullUrl,omitempty"`
	Resource json.RawMessage      `json:"resource,omitempty"`
	Request  *BundleEntryRequest  `json:"request,omitempty"`
	Response *BundleEntryResponse `json:"response,omitempty"`
	Search   *BundleEntrySearch   `json:"search,omitempty"`
}

// BundleEntryRequest represents a transaction/batch request.
type BundleEntryRequest struct {
	Method string `json:"method"` // GET | HEAD | POST | PUT | DELETE | PATCH
	URL    string `json:"url"`
}

// BundleEntryResponse represents a transaction/batch response.
type BundleEntryResponse struct {
	Status   string `json:"status"`
	Location string `json:"location,omitempty"`
	Etag     string `json:"etag,omitempty"`
}

// BundleEntrySearch represents search metadata in a searchset bundle.
type BundleEntrySearch struct {
	Mode  string  `json:"mode,omitempty"` // match | include | outcome
	Score float64 `json:"score,omitempty"`
}

// GetResourceType returns "Bundle".
func (b *Bundle) GetResourceType() string {
	return "Bundle"
}

// MarshalJSON ensures ResourceType is always set.
func (b *Bundle) MarshalJSON() ([]byte, error) {
	type Alias Bundle
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "Bundle",
		Alias:        (*Alias)(b),
	})
}

// DiagnosticReport represents a FHIR DiagnosticReport resource.
type DiagnosticReport struct {
	ResourceType      string            `json:"resourceType"`
	ID                string            `json:"id,omitempty"`
	Meta              *Meta             `json:"meta,omitempty"`
	Identifier        []Identifier      `json:"identifier,omitempty"`
	Status            string            `json:"status"` // registered | partial | preliminary | final | amended | corrected | appended | cancelled | entered-in-error | unknown
	Category          []CodeableConcept `json:"category,omitempty"`
	Code              CodeableConcept   `json:"code"`
	Subject           *Reference        `json:"subject,omitempty"`
	Encounter         *Reference        `json:"encounter,omitempty"`
	EffectiveDateTime string            `json:"effectiveDateTime,omitempty"`
	Issued            *time.Time        `json:"issued,omitempty"`
	Performer         []Reference       `json:"performer,omitempty"`
	Result            []Reference       `json:"result,omitempty"`
	Conclusion        string            `json:"conclusion,omitempty"`
}

// GetResourceType returns "DiagnosticReport".
func (d *DiagnosticReport) GetResourceType() string {
	return "DiagnosticReport"
}

// OperationOutcome represents a FHIR OperationOutcome for errors/warnings.
type OperationOutcome struct {
	ResourceType string                  `json:"resourceType"`
	ID           string                  `json:"id,omitempty"`
	Issue        []OperationOutcomeIssue `json:"issue"`
}

// OperationOutcomeIssue represents an individual issue in an OperationOutcome.
type OperationOutcomeIssue struct {
	Severity    string           `json:"severity"` // fatal | error | warning | information
	Code        string           `json:"code"`     // See http://hl7.org/fhir/issue-type
	Details     *CodeableConcept `json:"details,omitempty"`
	Diagnostics string           `json:"diagnostics,omitempty"`
	Location    []string         `json:"location,omitempty"`
	Expression  []string         `json:"expression,omitempty"`
}

// GetResourceType returns "OperationOutcome".
func (o *OperationOutcome) GetResourceType() string {
	return "OperationOutcome"
}
