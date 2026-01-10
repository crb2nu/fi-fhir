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
	USCoreCoverageProfile       = USCoreBaseURL + "us-core-coverage"
	USCoreProcedureProfile      = USCoreBaseURL + "us-core-procedure"
	USCoreImmunizationProfile   = USCoreBaseURL + "us-core-immunization"
	USCoreVitalSignsProfile     = USCoreBaseURL + "us-core-vital-signs"

	// Specific vital signs profiles (derived from us-core-vital-signs)
	USCoreBloodPressureProfile     = USCoreBaseURL + "us-core-blood-pressure"
	USCoreBodyHeightProfile        = USCoreBaseURL + "us-core-body-height"
	USCoreBodyWeightProfile        = USCoreBaseURL + "us-core-body-weight"
	USCoreBodyTemperatureProfile   = USCoreBaseURL + "us-core-body-temperature"
	USCoreHeartRateProfile         = USCoreBaseURL + "us-core-heart-rate"
	USCoreRespiratoryRateProfile   = USCoreBaseURL + "us-core-respiratory-rate"
	USCorePulseOximetryProfile     = USCoreBaseURL + "us-core-pulse-oximetry"
	USCoreBMIProfile               = USCoreBaseURL + "us-core-bmi"

	// Extension URIs
	USCoreRaceExtension      = USCoreBaseURL + "us-core-race"
	USCoreEthnicityExtension = USCoreBaseURL + "us-core-ethnicity"
	USCoreBirthSexExtension  = USCoreBaseURL + "us-core-birthsex"

	// Code system URIs
	SystemLOINC                = "http://loinc.org"
	SystemSNOMED               = "http://snomed.info/sct"
	SystemICD10CM              = "http://hl7.org/fhir/sid/icd-10-cm"
	SystemCPT                  = "http://www.ama-assn.org/go/cpt"
	SystemICD10PCS             = "http://www.cms.gov/Medicare/Coding/ICD10"
	SystemCVX                  = "http://hl7.org/fhir/sid/cvx"
	SystemRxNorm               = "http://www.nlm.nih.gov/research/umls/rxnorm"
	SystemUCUM                 = "http://unitsofmeasure.org"
	SystemAdministrativeGender = "http://hl7.org/fhir/administrative-gender"
	SystemObservationCategory  = "http://terminology.hl7.org/CodeSystem/observation-category"
	SystemInterpretation       = "http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation"
	SystemEncounterClass       = "http://terminology.hl7.org/CodeSystem/v3-ActCode"
	SystemIdentifierType       = "http://terminology.hl7.org/CodeSystem/v2-0203"

	// Race/ethnicity system (CDC Race & Ethnicity)
	SystemCDCRaceEthnicity = "urn:oid:2.16.840.1.113883.6.238"

	// Condition-related code systems
	SystemConditionClinicalStatus     = "http://terminology.hl7.org/CodeSystem/condition-clinical"
	SystemConditionVerificationStatus = "http://terminology.hl7.org/CodeSystem/condition-ver-status"
	SystemConditionCategory           = "http://terminology.hl7.org/CodeSystem/condition-category"

	// Coverage-related code systems
	SystemCoverageType       = "http://terminology.hl7.org/CodeSystem/v3-ActCode"
	SystemCoverageClass      = "http://terminology.hl7.org/CodeSystem/coverage-class"
	SystemSubscriberRelation = "http://terminology.hl7.org/CodeSystem/subscriber-relationship"
	SystemCopayType          = "http://terminology.hl7.org/CodeSystem/coverage-copay-type"

	// Claim/EOB-related code systems and profiles
	DaVinciPASBaseURL      = "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/"
	DaVinciPASClaimProfile = DaVinciPASBaseURL + "profile-claim"

	PDexBaseURL      = "http://hl7.org/fhir/us/davinci-pdex/StructureDefinition/"
	PDexEOBProfile   = PDexBaseURL + "pdex-adjudication"

	// Claim type code system
	SystemClaimType = "http://terminology.hl7.org/CodeSystem/claim-type"

	// Adjudication category code system
	SystemAdjudicationCategory = "http://terminology.hl7.org/CodeSystem/adjudication"

	// Payment type code system
	SystemPaymentType = "http://terminology.hl7.org/CodeSystem/ex-paymenttype"

	// Claim/remittance status code systems
	SystemClaimStatus = "http://hl7.org/fhir/fm-status"
	SystemEOBOutcome  = "http://hl7.org/fhir/remittance-outcome"

	// HCPCS/CPT code system
	SystemHCPCS = "http://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets"

	// X12 CARC/RARC code systems
	SystemCARC = "https://x12.org/codes/claim-adjustment-reason-codes"
	SystemRARC = "https://x12.org/codes/remittance-advice-remark-codes"

	// Place of service code system
	SystemPlaceOfService = "https://www.cms.gov/Medicare/Coding/place-of-service-codes"

	// Eligibility response code systems
	SystemEligibilityOutcome     = "http://hl7.org/fhir/remittance-outcome"
	SystemBenefitType            = "http://terminology.hl7.org/CodeSystem/benefit-type"
	SystemBenefitNetwork         = "http://terminology.hl7.org/CodeSystem/benefit-network"
	SystemBenefitUnit            = "http://terminology.hl7.org/CodeSystem/benefit-unit"
	SystemBenefitTerm            = "http://terminology.hl7.org/CodeSystem/benefit-term"
	SystemEligibilityCategory    = "http://terminology.hl7.org/CodeSystem/ex-benefitcategory"
	SystemEligibilityPurpose     = "http://hl7.org/fhir/eligibilityresponse-purpose"
	SystemProcessingError        = "http://terminology.hl7.org/CodeSystem/adjudication-error"

	// Vital Signs LOINC codes (US Core required)
	LOINCHeartRate         = "8867-4"
	LOINCRespiratoryRate   = "9279-1"
	LOINCBodyTemperature   = "8310-5"
	LOINCBodyHeight        = "8302-2"
	LOINCBodyWeight        = "29463-7"
	LOINCBodyMassIndex     = "39156-5"
	LOINCOxygenSaturation  = "2708-6"  // O2 saturation (arterial)
	LOINCPulseOximetry     = "59408-5" // Oxygen saturation in arterial blood by pulse oximetry
	LOINCBloodPressurePanel = "85354-9" // Blood pressure panel
	LOINCSystolicBP        = "8480-6"
	LOINCDiastolicBP       = "8462-4"
	LOINCHeadCircumference = "9843-4"

	// Vital signs category code
	VitalSignsCategory = "vital-signs"
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

// Condition represents a FHIR Condition resource for problems/diagnoses.
// Follows US Core Condition (Problems and Health Concerns) profile.
type Condition struct {
	ResourceType   string            `json:"resourceType"`
	ID             string            `json:"id,omitempty"`
	Meta           *Meta             `json:"meta,omitempty"`
	Identifier     []Identifier      `json:"identifier,omitempty"`
	ClinicalStatus *CodeableConcept  `json:"clinicalStatus,omitempty"` // active | recurrence | relapse | inactive | remission | resolved
	VerificationStatus *CodeableConcept `json:"verificationStatus,omitempty"` // unconfirmed | provisional | differential | confirmed | refuted | entered-in-error
	Category       []CodeableConcept `json:"category,omitempty"`        // problem-list-item | encounter-diagnosis | health-concern
	Severity       *CodeableConcept  `json:"severity,omitempty"`        // mild | moderate | severe
	Code           CodeableConcept   `json:"code"`                      // SNOMED CT or ICD-10 code
	BodySite       []CodeableConcept `json:"bodySite,omitempty"`
	Subject        *Reference        `json:"subject"`                   // Reference to Patient (required)
	Encounter      *Reference        `json:"encounter,omitempty"`       // Reference to Encounter
	OnsetDateTime  string            `json:"onsetDateTime,omitempty"`
	OnsetPeriod    *Period           `json:"onsetPeriod,omitempty"`
	OnsetAge       *Age              `json:"onsetAge,omitempty"`
	AbatementDateTime string         `json:"abatementDateTime,omitempty"`
	AbatementPeriod *Period          `json:"abatementPeriod,omitempty"`
	RecordedDate   string            `json:"recordedDate,omitempty"`
	Recorder       *Reference        `json:"recorder,omitempty"`
	Asserter       *Reference        `json:"asserter,omitempty"`
	Note           []Annotation      `json:"note,omitempty"`
}

// Age represents an age value with unit.
type Age struct {
	Value  float64 `json:"value,omitempty"`
	Unit   string  `json:"unit,omitempty"`
	System string  `json:"system,omitempty"`
	Code   string  `json:"code,omitempty"`
}

// GetResourceType returns "Condition".
func (c *Condition) GetResourceType() string {
	return "Condition"
}

// MarshalJSON ensures ResourceType is always set.
func (c *Condition) MarshalJSON() ([]byte, error) {
	type Alias Condition
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "Condition",
		Alias:        (*Alias)(c),
	})
}

// Coverage represents a FHIR Coverage resource for insurance information.
// Derived from 271 eligibility responses.
type Coverage struct {
	ResourceType   string            `json:"resourceType"`
	ID             string            `json:"id,omitempty"`
	Meta           *Meta             `json:"meta,omitempty"`
	Identifier     []Identifier      `json:"identifier,omitempty"`
	Status         string            `json:"status"`                    // active | cancelled | draft | entered-in-error
	Type           *CodeableConcept  `json:"type,omitempty"`            // Insurance plan type
	PolicyHolder   *Reference        `json:"policyHolder,omitempty"`    // Owner of the policy
	Subscriber     *Reference        `json:"subscriber,omitempty"`      // Subscriber to the policy
	SubscriberId   string            `json:"subscriberId,omitempty"`    // Member ID
	Beneficiary    *Reference        `json:"beneficiary"`               // Plan beneficiary (required)
	Dependent      string            `json:"dependent,omitempty"`       // Dependent number
	Relationship   *CodeableConcept  `json:"relationship,omitempty"`    // Beneficiary relationship to subscriber
	Period         *Period           `json:"period,omitempty"`          // Coverage start/end dates
	Payor          []Reference       `json:"payor"`                     // Issuer of the policy (required)
	Class          []CoverageClass   `json:"class,omitempty"`           // Classification (plan, group, etc.)
	Order          int               `json:"order,omitempty"`           // Relative order of coverage
	Network        string            `json:"network,omitempty"`         // Network name
	CostToBeneficiary []CostToBeneficiary `json:"costToBeneficiary,omitempty"` // Deductible, copay, etc.
}

// CoverageClass represents classification groupings.
type CoverageClass struct {
	Type  CodeableConcept `json:"type"`            // plan | group | subplan | subgroup
	Value string          `json:"value"`           // Class value (e.g., plan ID)
	Name  string          `json:"name,omitempty"`  // Human readable name
}

// CostToBeneficiary represents patient responsibility amounts.
type CostToBeneficiary struct {
	Type      *CodeableConcept     `json:"type,omitempty"`      // deductible | copay | coinsurance
	ValueQuantity *Quantity        `json:"valueQuantity,omitempty"`
	ValueMoney    *Money           `json:"valueMoney,omitempty"`
	Exception     []CostException  `json:"exception,omitempty"` // Exceptions to costs
}

// CostException represents exceptions to cost-to-beneficiary.
type CostException struct {
	Type   CodeableConcept `json:"type"`
	Period *Period         `json:"period,omitempty"`
}

// Money represents a monetary amount.
type Money struct {
	Value    float64 `json:"value,omitempty"`
	Currency string  `json:"currency,omitempty"` // ISO 4217 code (USD, etc.)
}

// GetResourceType returns "Coverage".
func (c *Coverage) GetResourceType() string {
	return "Coverage"
}

// MarshalJSON ensures ResourceType is always set.
func (c *Coverage) MarshalJSON() ([]byte, error) {
	type Alias Coverage
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "Coverage",
		Alias:        (*Alias)(c),
	})
}

// Claim represents a FHIR Claim resource for billing/prior authorization.
// Supports both Da Vinci PAS (preauthorization) and standard claims.
type Claim struct {
	ResourceType string            `json:"resourceType"`
	ID           string            `json:"id,omitempty"`
	Meta         *Meta             `json:"meta,omitempty"`
	Identifier   []Identifier      `json:"identifier,omitempty"`
	Status       string            `json:"status"`                   // active | cancelled | draft | entered-in-error
	Type         CodeableConcept   `json:"type"`                     // institutional | oral | pharmacy | professional | vision
	Use          string            `json:"use"`                      // claim | preauthorization | predetermination
	Patient      *Reference        `json:"patient"`                  // Required
	Created      string            `json:"created,omitempty"`        // Creation date
	Insurer      *Reference        `json:"insurer,omitempty"`        // Target payer
	Provider     *Reference        `json:"provider"`                 // Required - billing provider
	Priority     CodeableConcept   `json:"priority,omitempty"`       // normal | urgent | stat
	Prescription *Reference        `json:"prescription,omitempty"`   // Prescription reference
	Payee        *ClaimPayee       `json:"payee,omitempty"`          // Recipient of benefits
	Facility     *Reference        `json:"facility,omitempty"`       // Facility
	CareTeam     []ClaimCareTeam   `json:"careTeam,omitempty"`       // Care team members
	Diagnosis    []ClaimDiagnosis  `json:"diagnosis,omitempty"`      // Diagnosis codes
	Insurance    []ClaimInsurance  `json:"insurance"`                // Required - Insurance coverage
	Item         []ClaimItem       `json:"item,omitempty"`           // Service line items
	Total        *Money            `json:"total,omitempty"`          // Total claim cost
}

// ClaimPayee identifies the recipient of benefits payable.
type ClaimPayee struct {
	Type  CodeableConcept `json:"type,omitempty"`  // subscriber | provider | other
	Party *Reference      `json:"party,omitempty"` // Reference to recipient
}

// ClaimCareTeam represents a member of the care team.
type ClaimCareTeam struct {
	Sequence int             `json:"sequence"`
	Provider *Reference      `json:"provider"`
	Role     *CodeableConcept `json:"role,omitempty"` // primary | assist | supervisor
}

// ClaimDiagnosis represents a diagnosis on a claim.
type ClaimDiagnosis struct {
	Sequence           int              `json:"sequence"`
	DiagnosisCodeable  *CodeableConcept `json:"diagnosisCodeableConcept,omitempty"`
	DiagnosisReference *Reference       `json:"diagnosisReference,omitempty"`
	Type               []CodeableConcept `json:"type,omitempty"` // admitting | principal | discharge
	OnAdmission        *CodeableConcept `json:"onAdmission,omitempty"`
}

// ClaimInsurance represents insurance coverage on a claim.
type ClaimInsurance struct {
	Sequence          int        `json:"sequence"`
	Focal             bool       `json:"focal"`                       // Is this the primary insurance?
	Identifier        *Identifier `json:"identifier,omitempty"`
	Coverage          *Reference `json:"coverage"`                    // Reference to Coverage resource
	BusinessArrangement string   `json:"businessArrangement,omitempty"`
	PreAuthRef        []string   `json:"preAuthRef,omitempty"`        // Prior authorization references
	ClaimResponse     *Reference `json:"claimResponse,omitempty"`
}

// ClaimItem represents a line item on a claim.
type ClaimItem struct {
	Sequence             int              `json:"sequence"`
	CareTeamSequence     []int            `json:"careTeamSequence,omitempty"`
	DiagnosisSequence    []int            `json:"diagnosisSequence,omitempty"`
	ProcedureSequence    []int            `json:"procedureSequence,omitempty"`
	InformationSequence  []int            `json:"informationSequence,omitempty"`
	Revenue              *CodeableConcept `json:"revenue,omitempty"`        // Revenue center code
	Category             *CodeableConcept `json:"category,omitempty"`       // Service category
	ProductOrService     CodeableConcept  `json:"productOrService"`         // CPT/HCPCS code
	Modifier             []CodeableConcept `json:"modifier,omitempty"`      // Service modifiers
	ProgramCode          []CodeableConcept `json:"programCode,omitempty"`
	ServicedDate         string           `json:"servicedDate,omitempty"`
	ServicedPeriod       *Period          `json:"servicedPeriod,omitempty"`
	LocationCodeable     *CodeableConcept `json:"locationCodeableConcept,omitempty"` // Place of service
	LocationAddress      *Address         `json:"locationAddress,omitempty"`
	LocationReference    *Reference       `json:"locationReference,omitempty"`
	Quantity             *Quantity        `json:"quantity,omitempty"`
	UnitPrice            *Money           `json:"unitPrice,omitempty"`
	Factor               float64          `json:"factor,omitempty"`
	Net                  *Money           `json:"net,omitempty"`            // Line item cost
	BodySite             *CodeableConcept `json:"bodySite,omitempty"`
	SubSite              []CodeableConcept `json:"subSite,omitempty"`
	Detail               []ClaimItemDetail `json:"detail,omitempty"`
}

// ClaimItemDetail represents detail level items.
type ClaimItemDetail struct {
	Sequence         int               `json:"sequence"`
	ProductOrService CodeableConcept   `json:"productOrService"`
	Modifier         []CodeableConcept `json:"modifier,omitempty"`
	Quantity         *Quantity         `json:"quantity,omitempty"`
	UnitPrice        *Money            `json:"unitPrice,omitempty"`
	Factor           float64           `json:"factor,omitempty"`
	Net              *Money            `json:"net,omitempty"`
}

// GetResourceType returns "Claim".
func (c *Claim) GetResourceType() string {
	return "Claim"
}

// MarshalJSON ensures ResourceType is always set.
func (c *Claim) MarshalJSON() ([]byte, error) {
	type Alias Claim
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "Claim",
		Alias:        (*Alias)(c),
	})
}

// ExplanationOfBenefit represents a FHIR ExplanationOfBenefit (EOB) resource.
// Used for claim adjudication results (835 remittance advice).
type ExplanationOfBenefit struct {
	ResourceType      string               `json:"resourceType"`
	ID                string               `json:"id,omitempty"`
	Meta              *Meta                `json:"meta,omitempty"`
	Identifier        []Identifier         `json:"identifier,omitempty"`
	Status            string               `json:"status"`            // active | cancelled | draft | entered-in-error
	Type              CodeableConcept      `json:"type"`              // institutional | oral | pharmacy | professional | vision
	Use               string               `json:"use"`               // claim | preauthorization | predetermination
	Patient           *Reference           `json:"patient"`           // Required
	BillablePeriod    *Period              `json:"billablePeriod,omitempty"`
	Created           string               `json:"created,omitempty"` // Creation date
	Insurer           *Reference           `json:"insurer"`           // Required - payer
	Provider          *Reference           `json:"provider"`          // Required - billing provider
	Outcome           string               `json:"outcome"`           // queued | complete | error | partial
	Disposition       string               `json:"disposition,omitempty"`
	Claim             *Reference           `json:"claim,omitempty"`             // Original claim
	ClaimResponse     *Reference           `json:"claimResponse,omitempty"`     // Claim response
	PreAuthRef        []string             `json:"preAuthRef,omitempty"`
	Payee             *EOBPayee            `json:"payee,omitempty"`
	CareTeam          []EOBCareTeam        `json:"careTeam,omitempty"`
	Diagnosis         []EOBDiagnosis       `json:"diagnosis,omitempty"`
	Insurance         []EOBInsurance       `json:"insurance"`                   // Required
	Item              []EOBItem            `json:"item,omitempty"`
	Adjudication      []EOBAdjudication    `json:"adjudication,omitempty"`      // Header-level adjudication
	Total             []EOBTotal           `json:"total,omitempty"`
	Payment           *EOBPayment          `json:"payment,omitempty"`
	ProcessNote       []EOBProcessNote     `json:"processNote,omitempty"`
}

// EOBPayee represents the recipient of payment.
type EOBPayee struct {
	Type  *CodeableConcept `json:"type,omitempty"`
	Party *Reference       `json:"party,omitempty"`
}

// EOBCareTeam represents a care team member in EOB.
type EOBCareTeam struct {
	Sequence int              `json:"sequence"`
	Provider *Reference       `json:"provider"`
	Role     *CodeableConcept `json:"role,omitempty"`
}

// EOBDiagnosis represents a diagnosis in EOB.
type EOBDiagnosis struct {
	Sequence           int               `json:"sequence"`
	DiagnosisCodeable  *CodeableConcept  `json:"diagnosisCodeableConcept,omitempty"`
	DiagnosisReference *Reference        `json:"diagnosisReference,omitempty"`
	Type               []CodeableConcept `json:"type,omitempty"`
	OnAdmission        *CodeableConcept  `json:"onAdmission,omitempty"`
}

// EOBInsurance represents insurance information in EOB.
type EOBInsurance struct {
	Focal    bool       `json:"focal"`
	Coverage *Reference `json:"coverage"`
}

// EOBItem represents a service line item in EOB.
type EOBItem struct {
	Sequence             int               `json:"sequence"`
	CareTeamSequence     []int             `json:"careTeamSequence,omitempty"`
	DiagnosisSequence    []int             `json:"diagnosisSequence,omitempty"`
	ProductOrService     CodeableConcept   `json:"productOrService"`
	Modifier             []CodeableConcept `json:"modifier,omitempty"`
	ServicedDate         string            `json:"servicedDate,omitempty"`
	ServicedPeriod       *Period           `json:"servicedPeriod,omitempty"`
	LocationCodeable     *CodeableConcept  `json:"locationCodeableConcept,omitempty"`
	Quantity             *Quantity         `json:"quantity,omitempty"`
	UnitPrice            *Money            `json:"unitPrice,omitempty"`
	Net                  *Money            `json:"net,omitempty"`
	Adjudication         []EOBAdjudication `json:"adjudication,omitempty"`
}

// EOBAdjudication represents adjudication information.
type EOBAdjudication struct {
	Category CodeableConcept  `json:"category"`
	Reason   *CodeableConcept `json:"reason,omitempty"`
	Amount   *Money           `json:"amount,omitempty"`
	Value    float64          `json:"value,omitempty"`
}

// EOBTotal represents totals for the EOB.
type EOBTotal struct {
	Category CodeableConcept `json:"category"`
	Amount   Money           `json:"amount"`
}

// EOBPayment represents payment information.
type EOBPayment struct {
	Type           *CodeableConcept `json:"type,omitempty"`
	Adjustment     *Money           `json:"adjustment,omitempty"`
	AdjustmentReason *CodeableConcept `json:"adjustmentReason,omitempty"`
	Date           string           `json:"date,omitempty"`
	Amount         *Money           `json:"amount,omitempty"`
	Identifier     *Identifier      `json:"identifier,omitempty"` // Check/EFT number
}

// EOBProcessNote represents processing notes.
type EOBProcessNote struct {
	Number   int    `json:"number,omitempty"`
	Type     string `json:"type,omitempty"` // display | print | printoper
	Text     string `json:"text,omitempty"`
	Language *CodeableConcept `json:"language,omitempty"`
}

// GetResourceType returns "ExplanationOfBenefit".
func (e *ExplanationOfBenefit) GetResourceType() string {
	return "ExplanationOfBenefit"
}

// MarshalJSON ensures ResourceType is always set.
func (e *ExplanationOfBenefit) MarshalJSON() ([]byte, error) {
	type Alias ExplanationOfBenefit
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "ExplanationOfBenefit",
		Alias:        (*Alias)(e),
	})
}

// CoverageEligibilityResponse represents a FHIR CoverageEligibilityResponse resource.
// Used for 271 eligibility response transactions.
type CoverageEligibilityResponse struct {
	ResourceType string                         `json:"resourceType"`
	ID           string                         `json:"id,omitempty"`
	Meta         *Meta                          `json:"meta,omitempty"`
	Identifier   []Identifier                   `json:"identifier,omitempty"`
	Status       string                         `json:"status"`                    // active | cancelled | draft | entered-in-error
	Purpose      []string                       `json:"purpose"`                   // auth-requirements | benefits | discovery | validation
	Patient      *Reference                     `json:"patient"`                   // Required
	ServicedDate string                         `json:"servicedDate,omitempty"`    // Date for which eligibility was checked
	ServicedPeriod *Period                      `json:"servicedPeriod,omitempty"`  // Period for which eligibility was checked
	Created      string                         `json:"created,omitempty"`         // Response creation date
	Requestor    *Reference                     `json:"requestor,omitempty"`       // Provider making the request
	Request      *Reference                     `json:"request,omitempty"`         // Reference to original request
	Outcome      string                         `json:"outcome"`                   // queued | complete | error | partial
	Disposition  string                         `json:"disposition,omitempty"`     // Disposition message
	Insurer      *Reference                     `json:"insurer"`                   // Required - payer
	Insurance    []CERInsurance                 `json:"insurance,omitempty"`       // Coverage/benefit details
	PreAuthRef   string                         `json:"preAuthRef,omitempty"`      // Pre-authorization reference
	Form         *CodeableConcept               `json:"form,omitempty"`            // Form identifier
	Error        []CERError                     `json:"error,omitempty"`           // Processing errors
}

// CERInsurance represents insurance coverage information in CoverageEligibilityResponse.
type CERInsurance struct {
	Coverage       *Reference    `json:"coverage"`                 // Reference to Coverage resource
	Inforce        bool          `json:"inforce,omitempty"`        // Is coverage currently in force?
	BenefitPeriod  *Period       `json:"benefitPeriod,omitempty"`  // Benefit period
	Item           []CERItem     `json:"item,omitempty"`           // Benefits/services covered
}

// CERItem represents a benefit/service item in CoverageEligibilityResponse.
type CERItem struct {
	Category              *CodeableConcept  `json:"category,omitempty"`              // Benefit category
	ProductOrService      *CodeableConcept  `json:"productOrService,omitempty"`      // Billing code
	Modifier              []CodeableConcept `json:"modifier,omitempty"`              // Service modifiers
	Provider              *Reference        `json:"provider,omitempty"`              // Provider reference
	Excluded              bool              `json:"excluded,omitempty"`              // Is this excluded from coverage?
	Name                  string            `json:"name,omitempty"`                  // Benefit name
	Description           string            `json:"description,omitempty"`           // Description
	Network               *CodeableConcept  `json:"network,omitempty"`               // In or out of network
	Unit                  *CodeableConcept  `json:"unit,omitempty"`                  // Individual or family
	Term                  *CodeableConcept  `json:"term,omitempty"`                  // Annual or lifetime
	Benefit               []CERBenefit      `json:"benefit,omitempty"`               // Benefit amounts
	AuthorizationRequired bool              `json:"authorizationRequired,omitempty"` // Is prior auth needed?
	AuthorizationSupporting []CodeableConcept `json:"authorizationSupporting,omitempty"` // Documentation required
	AuthorizationUrl      string            `json:"authorizationUrl,omitempty"`      // URL for authorization
}

// CERBenefit represents a specific benefit amount in CoverageEligibilityResponse.
type CERBenefit struct {
	Type               CodeableConcept `json:"type"`                         // deductible | visit | copay | benefit | etc.
	AllowedUnsignedInt *int            `json:"allowedUnsignedInt,omitempty"` // Numeric allowed value
	AllowedString      string          `json:"allowedString,omitempty"`      // String allowed value
	AllowedMoney       *Money          `json:"allowedMoney,omitempty"`       // Monetary allowed value
	UsedUnsignedInt    *int            `json:"usedUnsignedInt,omitempty"`    // Numeric used value
	UsedString         string          `json:"usedString,omitempty"`         // String used value
	UsedMoney          *Money          `json:"usedMoney,omitempty"`          // Monetary used value
}

// CERError represents a processing error in CoverageEligibilityResponse.
type CERError struct {
	Code CodeableConcept `json:"code"` // Error code
}

// GetResourceType returns "CoverageEligibilityResponse".
func (c *CoverageEligibilityResponse) GetResourceType() string {
	return "CoverageEligibilityResponse"
}

// MarshalJSON ensures ResourceType is always set.
func (c *CoverageEligibilityResponse) MarshalJSON() ([]byte, error) {
	type Alias CoverageEligibilityResponse
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "CoverageEligibilityResponse",
		Alias:        (*Alias)(c),
	})
}

// Procedure represents a FHIR Procedure resource.
// Follows US Core Procedure profile.
type Procedure struct {
	ResourceType   string            `json:"resourceType"`
	ID             string            `json:"id,omitempty"`
	Meta           *Meta             `json:"meta,omitempty"`
	Identifier     []Identifier      `json:"identifier,omitempty"`
	Status         string            `json:"status"`                  // preparation | in-progress | not-done | on-hold | stopped | completed | entered-in-error | unknown
	StatusReason   *CodeableConcept  `json:"statusReason,omitempty"`  // Why procedure not performed
	Category       *CodeableConcept  `json:"category,omitempty"`      // Classification of the procedure
	Code           CodeableConcept   `json:"code"`                    // Required - SNOMED CT, CPT, or ICD-10-PCS
	Subject        *Reference        `json:"subject"`                 // Required - Reference to Patient
	Encounter      *Reference        `json:"encounter,omitempty"`     // Reference to Encounter
	PerformedDateTime string         `json:"performedDateTime,omitempty"`
	PerformedPeriod *Period          `json:"performedPeriod,omitempty"`
	PerformedString string           `json:"performedString,omitempty"`
	Performer      []ProcedurePerformer `json:"performer,omitempty"`  // Who performed the procedure
	Location       *Reference        `json:"location,omitempty"`      // Where procedure happened
	ReasonCode     []CodeableConcept `json:"reasonCode,omitempty"`    // Why procedure performed
	ReasonReference []Reference      `json:"reasonReference,omitempty"` // Condition/Observation justifying
	BodySite       []CodeableConcept `json:"bodySite,omitempty"`      // Body site
	Outcome        *CodeableConcept  `json:"outcome,omitempty"`       // Outcome of procedure
	Report         []Reference       `json:"report,omitempty"`        // Reports/DiagnosticReports
	Complication   []CodeableConcept `json:"complication,omitempty"`  // Complications
	FollowUp       []CodeableConcept `json:"followUp,omitempty"`      // Follow-up instructions
	Note           []Annotation      `json:"note,omitempty"`          // Additional notes
}

// ProcedurePerformer represents who performed the procedure.
type ProcedurePerformer struct {
	Function *CodeableConcept `json:"function,omitempty"` // Role of performer
	Actor    *Reference       `json:"actor"`              // Who performed
	OnBehalfOf *Reference     `json:"onBehalfOf,omitempty"` // Organization
}

// GetResourceType returns "Procedure".
func (p *Procedure) GetResourceType() string {
	return "Procedure"
}

// MarshalJSON ensures ResourceType is always set.
func (p *Procedure) MarshalJSON() ([]byte, error) {
	type Alias Procedure
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "Procedure",
		Alias:        (*Alias)(p),
	})
}

// Immunization represents a FHIR Immunization resource.
// Follows US Core Immunization profile.
type Immunization struct {
	ResourceType        string                    `json:"resourceType"`
	ID                  string                    `json:"id,omitempty"`
	Meta                *Meta                     `json:"meta,omitempty"`
	Identifier          []Identifier              `json:"identifier,omitempty"`
	Status              string                    `json:"status"`                    // completed | entered-in-error | not-done
	StatusReason        *CodeableConcept          `json:"statusReason,omitempty"`    // Reason not given
	VaccineCode         CodeableConcept           `json:"vaccineCode"`               // Required - CVX code
	Patient             *Reference                `json:"patient"`                   // Required - Reference to Patient
	Encounter           *Reference                `json:"encounter,omitempty"`       // Reference to Encounter
	OccurrenceDateTime  string                    `json:"occurrenceDateTime,omitempty"` // Required (choice)
	OccurrenceString    string                    `json:"occurrenceString,omitempty"`
	Recorded            string                    `json:"recorded,omitempty"`        // When first entered
	PrimarySource       *bool                     `json:"primarySource,omitempty"`   // From primary source?
	ReportOrigin        *CodeableConcept          `json:"reportOrigin,omitempty"`    // Source of secondhand info
	Location            *Reference                `json:"location,omitempty"`        // Where administered
	Manufacturer        *Reference                `json:"manufacturer,omitempty"`    // Vaccine manufacturer
	LotNumber           string                    `json:"lotNumber,omitempty"`       // Vaccine lot number
	ExpirationDate      string                    `json:"expirationDate,omitempty"`  // Vaccine expiration
	Site                *CodeableConcept          `json:"site,omitempty"`            // Body site
	Route               *CodeableConcept          `json:"route,omitempty"`           // Administration route
	DoseQuantity        *Quantity                 `json:"doseQuantity,omitempty"`    // Amount administered
	Performer           []ImmunizationPerformer   `json:"performer,omitempty"`       // Who administered
	Note                []Annotation              `json:"note,omitempty"`            // Additional notes
	ReasonCode          []CodeableConcept         `json:"reasonCode,omitempty"`      // Why given
	ReasonReference     []Reference               `json:"reasonReference,omitempty"` // Condition/observation justifying
	IsSubpotent         bool                      `json:"isSubpotent,omitempty"`     // Dose potency
	SubpotentReason     []CodeableConcept         `json:"subpotentReason,omitempty"` // Reason for being subpotent
	Education           []ImmunizationEducation   `json:"education,omitempty"`       // Educational material
	ProgramEligibility  []CodeableConcept         `json:"programEligibility,omitempty"` // Program eligibility
	FundingSource       *CodeableConcept          `json:"fundingSource,omitempty"`   // Source of funding
	Reaction            []ImmunizationReaction    `json:"reaction,omitempty"`        // Adverse reactions
	ProtocolApplied     []ImmunizationProtocol    `json:"protocolApplied,omitempty"` // Protocol followed
}

// ImmunizationPerformer represents who performed the immunization.
type ImmunizationPerformer struct {
	Function *CodeableConcept `json:"function,omitempty"` // Role of performer (AP=Administering, OP=Ordering)
	Actor    *Reference       `json:"actor"`              // Who performed
}

// ImmunizationEducation represents educational material presented.
type ImmunizationEducation struct {
	DocumentType string `json:"documentType,omitempty"`   // Type of material
	Reference    string `json:"reference,omitempty"`      // URL of material
	PublicationDate string `json:"publicationDate,omitempty"` // When published
	PresentationDate string `json:"presentationDate,omitempty"` // When presented
}

// ImmunizationReaction represents an adverse reaction.
type ImmunizationReaction struct {
	Date   string     `json:"date,omitempty"`     // When reaction started
	Detail *Reference `json:"detail,omitempty"`   // Reference to Observation
	Reported bool     `json:"reported,omitempty"` // Was reaction self-reported?
}

// ImmunizationProtocol represents a protocol applied.
type ImmunizationProtocol struct {
	Series         string            `json:"series,omitempty"`           // Series name
	Authority      *Reference        `json:"authority,omitempty"`        // Organization
	TargetDisease  []CodeableConcept `json:"targetDisease,omitempty"`    // Disease being targeted
	DoseNumberPositiveInt int        `json:"doseNumberPositiveInt,omitempty"` // Dose number
	DoseNumberString string          `json:"doseNumberString,omitempty"`
	SeriesDosesPositiveInt int       `json:"seriesDosesPositiveInt,omitempty"` // Total doses in series
	SeriesDosesString string         `json:"seriesDosesString,omitempty"`
}

// GetResourceType returns "Immunization".
func (i *Immunization) GetResourceType() string {
	return "Immunization"
}

// MarshalJSON ensures ResourceType is always set.
func (i *Immunization) MarshalJSON() ([]byte, error) {
	type Alias Immunization
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "Immunization",
		Alias:        (*Alias)(i),
	})
}
