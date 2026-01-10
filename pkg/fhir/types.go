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

	// Medication and allergy profiles
	USCoreMedicationRequestProfile    = USCoreBaseURL + "us-core-medicationrequest"
	USCoreMedicationProfile           = USCoreBaseURL + "us-core-medication"
	USCoreAllergyIntoleranceProfile   = USCoreBaseURL + "us-core-allergyintolerance"

	// Care coordination profiles
	USCoreCarePlanProfile = USCoreBaseURL + "us-core-careplan"
	USCoreGoalProfile     = USCoreBaseURL + "us-core-goal"

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

	// MedicationRequest code systems
	SystemMedicationRequestIntent    = "http://hl7.org/fhir/CodeSystem/medicationrequest-intent"
	SystemMedicationRequestStatus    = "http://hl7.org/fhir/CodeSystem/medicationrequest-status"
	SystemMedicationRequestCategory  = "http://terminology.hl7.org/CodeSystem/medicationrequest-category"
	SystemMedicationAdminRoute       = "http://snomed.info/sct" // SNOMED CT for routes
	SystemTimingAbbreviation         = "http://terminology.hl7.org/CodeSystem/v3-GTSAbbreviation"
	SystemDoseForm                   = "http://snomed.info/sct" // SNOMED CT for dose forms
	SystemUNII                       = "http://fdasis.nlm.nih.gov" // FDA UNII for substances

	// CarePlan and Goal code systems
	SystemCarePlanCategory       = "http://hl7.org/fhir/us/core/CodeSystem/careplan-category"
	SystemCarePlanStatus         = "http://hl7.org/fhir/request-status"
	SystemCarePlanIntent         = "http://hl7.org/fhir/request-intent"
	SystemCarePlanActivityStatus = "http://hl7.org/fhir/care-plan-activity-status"
	SystemGoalLifecycleStatus    = "http://hl7.org/fhir/goal-status"
	SystemGoalAchievementStatus  = "http://hl7.org/fhir/goal-achievement"
	SystemGoalCategory           = "http://terminology.hl7.org/CodeSystem/goal-category"
	SystemGoalPriority           = "http://terminology.hl7.org/CodeSystem/goal-priority"

	// AllergyIntolerance code systems
	SystemAllergyIntoleranceType            = "http://hl7.org/fhir/allergy-intolerance-type"
	SystemAllergyIntoleranceCategory        = "http://hl7.org/fhir/allergy-intolerance-category"
	SystemAllergyIntoleranceCriticality     = "http://hl7.org/fhir/allergy-intolerance-criticality"
	SystemAllergyIntoleranceClinicalStatus  = "http://terminology.hl7.org/CodeSystem/allergyintolerance-clinical"
	SystemAllergyIntoleranceVerification    = "http://terminology.hl7.org/CodeSystem/allergyintolerance-verification"
	SystemReactionSeverity                  = "http://hl7.org/fhir/reaction-event-severity"

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

// MedicationRequest represents a FHIR MedicationRequest resource (US Core profile).
type MedicationRequest struct {
	ID                      string                    `json:"id,omitempty"`
	Meta                    *Meta                     `json:"meta,omitempty"`
	Identifier              []Identifier              `json:"identifier,omitempty"`
	Status                  string                    `json:"status"`                         // active | completed | cancelled | entered-in-error | stopped | draft | on-hold | unknown
	StatusReason            *CodeableConcept          `json:"statusReason,omitempty"`         // Reason for current status
	Intent                  string                    `json:"intent"`                         // proposal | plan | order | original-order | reflex-order | filler-order | instance-order | option
	Category                []CodeableConcept         `json:"category,omitempty"`             // inpatient | outpatient | community | discharge
	Priority                string                    `json:"priority,omitempty"`             // routine | urgent | asap | stat
	DoNotPerform            bool                      `json:"doNotPerform,omitempty"`         // True if request should NOT be performed
	ReportedBoolean         bool                      `json:"reportedBoolean,omitempty"`      // Reported vs primary record
	ReportedReference       *Reference                `json:"reportedReference,omitempty"`    // Who reported
	MedicationCodeableConcept *CodeableConcept        `json:"medicationCodeableConcept,omitempty"` // Medication code (RxNorm)
	MedicationReference     *Reference                `json:"medicationReference,omitempty"`  // Reference to contained Medication
	Subject                 *Reference                `json:"subject"`                        // Patient reference
	Encounter               *Reference                `json:"encounter,omitempty"`            // Encounter reference
	SupportingInformation   []Reference               `json:"supportingInformation,omitempty"` // Supporting info
	AuthoredOn              string                    `json:"authoredOn,omitempty"`           // When request was authored
	Requester               *Reference                `json:"requester,omitempty"`            // Who requested (Practitioner/Organization)
	Performer               *Reference                `json:"performer,omitempty"`            // Intended performer
	PerformerType           *CodeableConcept          `json:"performerType,omitempty"`        // Type of performer
	Recorder                *Reference                `json:"recorder,omitempty"`             // Who entered order
	ReasonCode              []CodeableConcept         `json:"reasonCode,omitempty"`           // Reason for prescription
	ReasonReference         []Reference               `json:"reasonReference,omitempty"`      // Condition that supports medication
	InstantiatesCanonical   []string                  `json:"instantiatesCanonical,omitempty"` // Protocol followed
	InstantiatesURI         []string                  `json:"instantiatesUri,omitempty"`      // External protocol
	BasedOn                 []Reference               `json:"basedOn,omitempty"`              // What request fulfills
	GroupIdentifier         *Identifier               `json:"groupIdentifier,omitempty"`      // Composite request ID
	CourseOfTherapyType     *CodeableConcept          `json:"courseOfTherapyType,omitempty"`  // Overall pattern of medication administration
	Insurance               []Reference               `json:"insurance,omitempty"`            // Coverage
	Note                    []Annotation              `json:"note,omitempty"`                 // Additional notes
	DosageInstruction       []Dosage                  `json:"dosageInstruction,omitempty"`    // How the medication should be taken
	DispenseRequest         *DispenseRequest          `json:"dispenseRequest,omitempty"`      // Dispense details
	Substitution            *MedSubstitution          `json:"substitution,omitempty"`         // Substitution rules
	PriorPrescription       *Reference                `json:"priorPrescription,omitempty"`    // Previous order
	DetectedIssue           []Reference               `json:"detectedIssue,omitempty"`        // Clinical issues
	EventHistory            []Reference               `json:"eventHistory,omitempty"`         // Lifecycle events
}

// Dosage represents medication dosage instructions.
type Dosage struct {
	Sequence                 int               `json:"sequence,omitempty"`
	Text                     string            `json:"text,omitempty"`                     // Free text sig
	AdditionalInstruction    []CodeableConcept `json:"additionalInstruction,omitempty"`    // Supplemental instruction (e.g., "with meals")
	PatientInstruction       string            `json:"patientInstruction,omitempty"`       // Patient-specific instructions
	Timing                   *Timing           `json:"timing,omitempty"`                   // When to take
	AsNeededBoolean          bool              `json:"asNeededBoolean,omitempty"`          // PRN indicator
	AsNeededCodeableConcept  *CodeableConcept  `json:"asNeededCodeableConcept,omitempty"`  // Condition for PRN
	Site                     *CodeableConcept  `json:"site,omitempty"`                     // Body site
	Route                    *CodeableConcept  `json:"route,omitempty"`                    // How drug enters body
	Method                   *CodeableConcept  `json:"method,omitempty"`                   // Technique
	DoseAndRate              []DoseAndRate     `json:"doseAndRate,omitempty"`              // Amount of medication
	MaxDosePerPeriod         *Ratio            `json:"maxDosePerPeriod,omitempty"`         // Max dose per period
	MaxDosePerAdministration *Quantity         `json:"maxDosePerAdministration,omitempty"` // Max dose per administration
	MaxDosePerLifetime       *Quantity         `json:"maxDosePerLifetime,omitempty"`       // Max lifetime dose
}

// DoseAndRate represents dose amount with rate.
type DoseAndRate struct {
	Type         *CodeableConcept `json:"type,omitempty"`         // Type of dose (calculated, ordered, etc.)
	DoseQuantity *Quantity        `json:"doseQuantity,omitempty"` // Amount per dose
	DoseRange    *Range           `json:"doseRange,omitempty"`    // Amount per dose range
	RateRatio    *Ratio           `json:"rateRatio,omitempty"`    // Rate ratio
	RateRange    *Range           `json:"rateRange,omitempty"`    // Rate range
	RateQuantity *Quantity        `json:"rateQuantity,omitempty"` // Rate quantity
}

// Timing represents event timing.
type Timing struct {
	Event  []string      `json:"event,omitempty"`  // Specific times (dateTime)
	Repeat *TimingRepeat `json:"repeat,omitempty"` // When the event is to occur
	Code   *CodeableConcept `json:"code,omitempty"` // BID | TID | QID | AM | PM | QD | QOD | etc.
}

// TimingRepeat represents repeating timing details.
type TimingRepeat struct {
	BoundsDuration *Duration `json:"boundsDuration,omitempty"` // Length of timing bounds
	BoundsRange    *Range    `json:"boundsRange,omitempty"`    // Range of timing bounds
	BoundsPeriod   *Period   `json:"boundsPeriod,omitempty"`   // Period of timing bounds
	Count          int       `json:"count,omitempty"`          // Number of times to repeat
	CountMax       int       `json:"countMax,omitempty"`       // Maximum number of times
	Duration       float64   `json:"duration,omitempty"`       // How long when it happens
	DurationMax    float64   `json:"durationMax,omitempty"`    // Maximum duration
	DurationUnit   string    `json:"durationUnit,omitempty"`   // s | min | h | d | wk | mo | a
	Frequency      int       `json:"frequency,omitempty"`      // Event occurs frequency times per period
	FrequencyMax   int       `json:"frequencyMax,omitempty"`   // Maximum frequency
	Period         float64   `json:"period,omitempty"`         // Event occurs frequency times per period
	PeriodMax      float64   `json:"periodMax,omitempty"`      // Maximum period
	PeriodUnit     string    `json:"periodUnit,omitempty"`     // s | min | h | d | wk | mo | a
	DayOfWeek      []string  `json:"dayOfWeek,omitempty"`      // mon | tue | wed | thu | fri | sat | sun
	TimeOfDay      []string  `json:"timeOfDay,omitempty"`      // Time of day (hh:mm:ss format)
	When           []string  `json:"when,omitempty"`           // Code for time period (MORN, AFT, EVE, NIGHT, etc.)
	Offset         int       `json:"offset,omitempty"`         // Minutes from event
}

// Duration represents a length of time.
type Duration struct {
	Value      float64 `json:"value,omitempty"`
	Comparator string  `json:"comparator,omitempty"` // < | <= | >= | >
	Unit       string  `json:"unit,omitempty"`
	System     string  `json:"system,omitempty"`
	Code       string  `json:"code,omitempty"`
}

// DispenseRequest represents medication dispense details.
type DispenseRequest struct {
	InitialFill         *InitialFill  `json:"initialFill,omitempty"`         // First fill details
	DispenseInterval    *Duration     `json:"dispenseInterval,omitempty"`    // Minimum period between dispenses
	ValidityPeriod      *Period       `json:"validityPeriod,omitempty"`      // When prescription is valid
	NumberOfRepeatsAllowed int        `json:"numberOfRepeatsAllowed,omitempty"` // Number of refills
	Quantity            *Quantity     `json:"quantity,omitempty"`            // Quantity per dispense
	ExpectedSupplyDuration *Duration  `json:"expectedSupplyDuration,omitempty"` // Days supply
	Performer           *Reference    `json:"performer,omitempty"`           // Intended dispenser (Pharmacy)
}

// InitialFill represents initial dispensing details.
type InitialFill struct {
	Quantity *Quantity `json:"quantity,omitempty"` // First fill quantity
	Duration *Duration `json:"duration,omitempty"` // First fill duration
}

// MedSubstitution represents medication substitution rules.
type MedSubstitution struct {
	AllowedBoolean         bool             `json:"allowedBoolean,omitempty"`
	AllowedCodeableConcept *CodeableConcept `json:"allowedCodeableConcept,omitempty"`
	Reason                 *CodeableConcept `json:"reason,omitempty"`
}

// Ratio represents a ratio of two quantities.
type Ratio struct {
	Numerator   *Quantity `json:"numerator,omitempty"`
	Denominator *Quantity `json:"denominator,omitempty"`
}

// GetResourceType returns "MedicationRequest".
func (m *MedicationRequest) GetResourceType() string {
	return "MedicationRequest"
}

// MarshalJSON ensures ResourceType is always set.
func (m *MedicationRequest) MarshalJSON() ([]byte, error) {
	type Alias MedicationRequest
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "MedicationRequest",
		Alias:        (*Alias)(m),
	})
}

// AllergyIntolerance represents a FHIR AllergyIntolerance resource (US Core profile).
type AllergyIntolerance struct {
	ID                   string                  `json:"id,omitempty"`
	Meta                 *Meta                   `json:"meta,omitempty"`
	Identifier           []Identifier            `json:"identifier,omitempty"`
	ClinicalStatus       *CodeableConcept        `json:"clinicalStatus,omitempty"`       // active | inactive | resolved
	VerificationStatus   *CodeableConcept        `json:"verificationStatus,omitempty"`   // unconfirmed | confirmed | refuted | entered-in-error
	Type                 string                  `json:"type,omitempty"`                 // allergy | intolerance
	Category             []string                `json:"category,omitempty"`             // food | medication | environment | biologic
	Criticality          string                  `json:"criticality,omitempty"`          // low | high | unable-to-assess
	Code                 *CodeableConcept        `json:"code"`                           // Allergen code (required by US Core)
	Patient              *Reference              `json:"patient"`                        // Patient reference
	Encounter            *Reference              `json:"encounter,omitempty"`            // Encounter when allergy was recorded
	OnsetDateTime        string                  `json:"onsetDateTime,omitempty"`        // When allergy was identified
	OnsetAge             *Age                    `json:"onsetAge,omitempty"`             // Onset as age
	OnsetPeriod          *Period                 `json:"onsetPeriod,omitempty"`          // Onset period
	OnsetRange           *Range                  `json:"onsetRange,omitempty"`           // Onset range
	OnsetString          string                  `json:"onsetString,omitempty"`          // Onset as string
	RecordedDate         string                  `json:"recordedDate,omitempty"`         // When allergy was recorded
	Recorder             *Reference              `json:"recorder,omitempty"`             // Who recorded
	Asserter             *Reference              `json:"asserter,omitempty"`             // Who asserted
	LastOccurrence       string                  `json:"lastOccurrence,omitempty"`       // Last occurrence date
	Note                 []Annotation            `json:"note,omitempty"`                 // Additional notes
	Reaction             []AllergyReaction       `json:"reaction,omitempty"`             // Adverse reactions
}

// AllergyReaction represents an adverse reaction event.
type AllergyReaction struct {
	Substance     *CodeableConcept  `json:"substance,omitempty"`     // Specific substance
	Manifestation []CodeableConcept `json:"manifestation"`           // Symptoms/signs (required)
	Description   string            `json:"description,omitempty"`   // Description of reaction
	Onset         string            `json:"onset,omitempty"`         // When reaction occurred
	Severity      string            `json:"severity,omitempty"`      // mild | moderate | severe
	ExposureRoute *CodeableConcept  `json:"exposureRoute,omitempty"` // How allergen was encountered
	Note          []Annotation      `json:"note,omitempty"`          // Additional notes
}

// GetResourceType returns "AllergyIntolerance".
func (a *AllergyIntolerance) GetResourceType() string {
	return "AllergyIntolerance"
}

// MarshalJSON ensures ResourceType is always set.
func (a *AllergyIntolerance) MarshalJSON() ([]byte, error) {
	type Alias AllergyIntolerance
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "AllergyIntolerance",
		Alias:        (*Alias)(a),
	})
}

// CarePlan represents a FHIR CarePlan resource (US Core profile).
type CarePlan struct {
	ID              string              `json:"id,omitempty"`
	Meta            *Meta               `json:"meta,omitempty"`
	Identifier      []Identifier        `json:"identifier,omitempty"`
	Status          string              `json:"status"`                     // draft | active | on-hold | revoked | completed | entered-in-error | unknown
	Intent          string              `json:"intent"`                     // proposal | plan | order | option
	Category        []CodeableConcept   `json:"category"`                   // Required by US Core - must include "assess-plan"
	Title           string              `json:"title,omitempty"`            // Human-friendly name
	Description     string              `json:"description,omitempty"`      // Summary of plan
	Subject         *Reference          `json:"subject"`                    // Patient reference
	Encounter       *Reference          `json:"encounter,omitempty"`        // Encounter reference
	Period          *Period             `json:"period,omitempty"`           // Time period plan covers
	Created         string              `json:"created,omitempty"`          // When plan was created
	Author          *Reference          `json:"author,omitempty"`           // Who created the plan
	Contributor     []Reference         `json:"contributor,omitempty"`      // Who contributed to the plan
	CareTeam        []Reference         `json:"careTeam,omitempty"`         // Care team references
	Addresses       []Reference         `json:"addresses,omitempty"`        // Conditions addressed
	SupportingInfo  []Reference         `json:"supportingInfo,omitempty"`   // Supporting information
	Goal            []Reference         `json:"goal,omitempty"`             // Goal references
	Activity        []CarePlanActivity  `json:"activity,omitempty"`         // Planned activities
	Note            []Annotation        `json:"note,omitempty"`             // Additional notes
}

// CarePlanActivity represents a planned activity within a care plan.
type CarePlanActivity struct {
	OutcomeCodeableConcept []CodeableConcept     `json:"outcomeCodeableConcept,omitempty"` // Activity outcome
	OutcomeReference       []Reference           `json:"outcomeReference,omitempty"`       // Reference to outcome resource
	Progress               []Annotation          `json:"progress,omitempty"`               // Activity progress notes
	Reference              *Reference            `json:"reference,omitempty"`              // Reference to activity resource
	Detail                 *CarePlanActivityDetail `json:"detail,omitempty"`               // In-line activity definition
}

// CarePlanActivityDetail represents the detailed definition of a care plan activity.
type CarePlanActivityDetail struct {
	Kind                 string            `json:"kind,omitempty"`                 // Appointment | CommunicationRequest | DeviceRequest | MedicationRequest | etc.
	InstantiatesCanonical []string          `json:"instantiatesCanonical,omitempty"` // Protocol followed
	InstantiatesURI      []string          `json:"instantiatesUri,omitempty"`      // External protocol
	Code                 *CodeableConcept  `json:"code,omitempty"`                 // Activity code
	ReasonCode           []CodeableConcept `json:"reasonCode,omitempty"`           // Why activity should be done
	ReasonReference      []Reference       `json:"reasonReference,omitempty"`      // Condition that is reason
	Goal                 []Reference       `json:"goal,omitempty"`                 // Goals this activity relates to
	Status               string            `json:"status"`                         // not-started | scheduled | in-progress | on-hold | completed | cancelled | stopped | unknown | entered-in-error
	StatusReason         *CodeableConcept  `json:"statusReason,omitempty"`         // Reason for current status
	DoNotPerform         bool              `json:"doNotPerform,omitempty"`         // If true, activity should not be performed
	ScheduledTiming      *Timing           `json:"scheduledTiming,omitempty"`      // When activity should occur
	ScheduledPeriod      *Period           `json:"scheduledPeriod,omitempty"`      // When activity should occur
	ScheduledString      string            `json:"scheduledString,omitempty"`      // When activity should occur
	Location             *Reference        `json:"location,omitempty"`             // Where activity should occur
	Performer            []Reference       `json:"performer,omitempty"`            // Who will perform
	ProductCodeableConcept *CodeableConcept `json:"productCodeableConcept,omitempty"` // What is to be administered/supplied
	ProductReference     *Reference        `json:"productReference,omitempty"`     // What is to be administered/supplied
	DailyAmount          *Quantity         `json:"dailyAmount,omitempty"`          // How to consume/day
	Quantity             *Quantity         `json:"quantity,omitempty"`             // How much to administer/supply
	Description          string            `json:"description,omitempty"`          // Extra info describing activity
}

// GetResourceType returns "CarePlan".
func (c *CarePlan) GetResourceType() string {
	return "CarePlan"
}

// MarshalJSON ensures ResourceType is always set.
func (c *CarePlan) MarshalJSON() ([]byte, error) {
	type Alias CarePlan
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "CarePlan",
		Alias:        (*Alias)(c),
	})
}

// Goal represents a FHIR Goal resource (US Core profile).
type Goal struct {
	ID                string            `json:"id,omitempty"`
	Meta              *Meta             `json:"meta,omitempty"`
	Identifier        []Identifier      `json:"identifier,omitempty"`
	LifecycleStatus   string            `json:"lifecycleStatus"`              // proposed | planned | accepted | active | on-hold | completed | cancelled | entered-in-error | rejected
	AchievementStatus *CodeableConcept  `json:"achievementStatus,omitempty"`  // in-progress | improving | worsening | no-change | achieved | sustaining | not-achieved | no-progress | not-attainable
	Category          []CodeableConcept `json:"category,omitempty"`           // E.g., dietary, safety, behavioral
	Priority          *CodeableConcept  `json:"priority,omitempty"`           // high-priority | medium-priority | low-priority
	Description       *CodeableConcept  `json:"description"`                  // Required by US Core - what the goal describes
	Subject           *Reference        `json:"subject"`                      // Patient reference
	StartDate         string            `json:"startDate,omitempty"`          // When goal started
	StartCodeableConcept *CodeableConcept `json:"startCodeableConcept,omitempty"` // When goal started
	Target            []GoalTarget      `json:"target,omitempty"`             // Target outcome
	StatusDate        string            `json:"statusDate,omitempty"`         // When goal status changed
	StatusReason      string            `json:"statusReason,omitempty"`       // Reason for status
	ExpressedBy       *Reference        `json:"expressedBy,omitempty"`        // Who set the goal
	Addresses         []Reference       `json:"addresses,omitempty"`          // Conditions this goal addresses
	Note              []Annotation      `json:"note,omitempty"`               // Comments about the goal
	OutcomeCode       []CodeableConcept `json:"outcomeCode,omitempty"`        // What was achieved
	OutcomeReference  []Reference       `json:"outcomeReference,omitempty"`   // Observation that resulted from goal
}

// GoalTarget represents a target outcome for the goal.
type GoalTarget struct {
	Measure           *CodeableConcept `json:"measure,omitempty"`           // The parameter whose value is being tracked
	DetailQuantity    *Quantity        `json:"detailQuantity,omitempty"`    // Target value (quantity)
	DetailRange       *Range           `json:"detailRange,omitempty"`       // Target value (range)
	DetailCodeableConcept *CodeableConcept `json:"detailCodeableConcept,omitempty"` // Target value (code)
	DetailString      string           `json:"detailString,omitempty"`      // Target value (string)
	DetailBoolean     *bool            `json:"detailBoolean,omitempty"`     // Target value (boolean)
	DetailInteger     *int             `json:"detailInteger,omitempty"`     // Target value (integer)
	DetailRatio       *Ratio           `json:"detailRatio,omitempty"`       // Target value (ratio)
	DueDate           string           `json:"dueDate,omitempty"`           // When target should be reached
	DueDuration       *Duration        `json:"dueDuration,omitempty"`       // When target should be reached
}

// GetResourceType returns "Goal".
func (g *Goal) GetResourceType() string {
	return "Goal"
}

// MarshalJSON ensures ResourceType is always set.
func (g *Goal) MarshalJSON() ([]byte, error) {
	type Alias Goal
	return json.Marshal(&struct {
		ResourceType string `json:"resourceType"`
		*Alias
	}{
		ResourceType: "Goal",
		Alias:        (*Alias)(g),
	})
}
