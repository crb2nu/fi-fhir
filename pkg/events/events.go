// Package events defines the canonical semantic event model for healthcare integrations.
// These types abstract away format-specific details (HL7v2, FHIR, CSV, EDI) and provide
// a unified data model that workflows can operate on.
package events

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"
)

// EventType represents the semantic type of a healthcare event.
type EventType string

const (
	// Patient events
	EventPatientAdmit     EventType = "patient_admit"
	EventPatientDischarge EventType = "patient_discharge"
	EventPatientTransfer  EventType = "patient_transfer"
	EventPatientUpdate    EventType = "patient_update"
	EventPatientMerge     EventType = "patient_merge"

	// Scheduling events
	EventAppointmentScheduled   EventType = "appointment_scheduled"
	EventAppointmentCancelled   EventType = "appointment_cancelled"
	EventAppointmentRescheduled EventType = "appointment_rescheduled"
	EventAppointmentModified    EventType = "appointment_modified"
	EventAppointmentNoShow      EventType = "appointment_noshow"
	EventAppointmentCheckedIn   EventType = "appointment_checked_in"

	// Lab/results events
	EventLabResult    EventType = "lab_result"
	EventLabOrdered   EventType = "lab_ordered"
	EventLabCancelled EventType = "lab_cancelled"

	// Claims/billing events
	EventClaimSubmitted    EventType = "claim_submitted"
	EventClaimAdjudicated  EventType = "claim_adjudicated"
	EventPriorAuthRequest  EventType = "prior_auth_request"
	EventPriorAuthResponse EventType = "prior_auth_response"

	// Eligibility events
	EventEligibilityInquiry  EventType = "eligibility_inquiry"
	EventEligibilityResponse EventType = "eligibility_response"

	// Claim status events
	EventClaimStatusRequest  EventType = "claim_status_request"
	EventClaimStatusResponse EventType = "claim_status_response"

	// Clinical document events
	EventDocument     EventType = "document"
	EventVitalSign    EventType = "vital_sign"
	EventCondition    EventType = "condition"
	EventProcedure    EventType = "procedure"
	EventImmunization EventType = "immunization"
)

// SourceFormat indicates the original format of the data.
type SourceFormat string

const (
	FormatHL7v2   SourceFormat = "hl7v2"
	FormatFHIR    SourceFormat = "fhir"
	FormatCSV     SourceFormat = "csv"
	FormatEDI835  SourceFormat = "edi_835"
	FormatEDI837  SourceFormat = "edi_837"
	FormatEDI270  SourceFormat = "edi_270"
	FormatEDI271  SourceFormat = "edi_271"
	FormatEDI276  SourceFormat = "edi_276"
	FormatEDI277  SourceFormat = "edi_277"
	FormatCDA     SourceFormat = "cda"
	FormatUnknown SourceFormat = "unknown"
)

// EventMeta contains metadata common to all events.
type EventMeta struct {
	// ID is a unique identifier for this event instance
	ID string `json:"id"`

	// Type is the semantic event type
	Type EventType `json:"type"`

	// Timestamp when the event occurred (from source system)
	Timestamp time.Time `json:"timestamp"`

	// ReceivedAt when fi-fhir received/processed the event
	ReceivedAt time.Time `json:"received_at"`

	// Source identifies the originating system/interface
	Source string `json:"source"`

	// SourceFormat indicates the original data format
	SourceFormat SourceFormat `json:"source_format"`

	// SourceProfileID identifies which Source Profile was used for parsing
	SourceProfileID string `json:"source_profile_id,omitempty"`

	// SourceMessageID is the original message identifier (e.g., MSH-10 for HL7v2)
	SourceMessageID string `json:"source_message_id,omitempty"`

	// CorrelationID links related events across systems
	CorrelationID string `json:"correlation_id,omitempty"`

	// ParseWarnings captures non-fatal issues encountered during parsing
	ParseWarnings []ParseWarning `json:"parse_warnings,omitempty"`
}

// ParseWarning captures non-fatal issues during parsing.
// These allow "messy but real" data to flow through while maintaining auditability.
type ParseWarning struct {
	// Phase where warning occurred: "byte", "syntactic", "semantic"
	Phase string `json:"phase"`

	// Code is a machine-readable warning code (e.g., "MISSING_PV1", "INVALID_NPI")
	Code string `json:"code"`

	// Message is a human-readable description
	Message string `json:"message"`

	// Path is the location in the source data (e.g., "PID.3.1", "OBX[2].5")
	Path string `json:"path,omitempty"`

	// Severity indicates the importance: "info", "warning", "error"
	Severity string `json:"severity,omitempty"`
}

// Patient represents a patient in the canonical model.
type Patient struct {
	// MRN is the medical record number (primary identifier) - convenience field
	// Use Identifiers.GetMRN() for the authoritative value
	MRN string `json:"mrn"`

	// Identifiers is the first-class collection of all known identifiers
	// This properly handles PID-3 repetition and cross-facility IDs
	Identifiers IdentifierSet `json:"identifiers"`

	// Name components
	FamilyName string `json:"family_name"`
	GivenName  string `json:"given_name"`
	MiddleName string `json:"middle_name,omitempty"`
	Prefix     string `json:"prefix,omitempty"`
	Suffix     string `json:"suffix,omitempty"`

	// Demographics
	DateOfBirth time.Time `json:"date_of_birth,omitempty"`
	Gender      string    `json:"gender,omitempty"`
	Race        string    `json:"race,omitempty"`
	Ethnicity   string    `json:"ethnicity,omitempty"`

	// Contact
	Address   Address   `json:"address,omitempty"`
	Addresses []Address `json:"addresses,omitempty"` // Multiple addresses (home, work, etc.)
	Phone     string    `json:"phone,omitempty"`
	Phones    []string  `json:"phones,omitempty"` // Multiple phone numbers
	Email     string    `json:"email,omitempty"`

	// Administrative
	Language            string    `json:"language,omitempty"`
	MaritalStatus       string    `json:"marital_status,omitempty"`
	PrimaryCareProvider *Provider `json:"primary_care_provider,omitempty"`

	// Extensions holds Z-segment and other source-specific data
	// Mapped from Source Profile z_segment_mappings
	Extensions map[string]interface{} `json:"extensions,omitempty"`

	// MatchScore is set when patient is returned from MPI lookup
	MatchScore float64 `json:"match_score,omitempty"`
}

// Identifier represents a healthcare identifier with full context.
// Supports repeating CX fields from HL7v2 PID-3 and FHIR Identifier.
type Identifier struct {
	// Value is the identifier value (normalized)
	Value string `json:"value"`

	// Type is the identifier type code (from HL7 Table 0203: MR, SS, PI, NPI, etc.)
	Type string `json:"type"`

	// System is the code system URI or OID (e.g., "urn:oid:2.16.840.1.113883.4.1" for SSN)
	System string `json:"system,omitempty"`

	// Assigner is who issued this identifier (from CX.4 assigning authority)
	Assigner string `json:"assigner,omitempty"`

	// OriginalValue preserves the value before normalization
	OriginalValue string `json:"original_value,omitempty"`

	// Period indicates when this identifier is/was valid
	Period *Period `json:"period,omitempty"`

	// Use indicates the purpose: usual, official, temp, secondary, old
	Use string `json:"use,omitempty"`

	// IsValid indicates if the identifier passed validation (NPI Luhn, MBI format, etc.)
	IsValid *bool `json:"is_valid,omitempty"`

	// ValidationError describes why validation failed
	ValidationError string `json:"validation_error,omitempty"`
}

// Period represents a time period with start and end.
type Period struct {
	Start *time.Time `json:"start,omitempty"`
	End   *time.Time `json:"end,omitempty"`
}

// IdentifierSet manages multiple identifiers for an entity (patient, provider, encounter).
// This is a first-class collection that handles the reality of PID-3 repetition.
type IdentifierSet struct {
	// Identifiers is the complete list of known identifiers
	Identifiers []Identifier `json:"identifiers"`

	// Primary is the selected "best" identifier based on Source Profile rules
	Primary *Identifier `json:"primary,omitempty"`
}

// GetByType returns the first identifier matching the given type.
func (s *IdentifierSet) GetByType(idType string) *Identifier {
	for i := range s.Identifiers {
		if s.Identifiers[i].Type == idType {
			return &s.Identifiers[i]
		}
	}
	return nil
}

// GetBySystem returns all identifiers from the given system.
func (s *IdentifierSet) GetBySystem(system string) []Identifier {
	var result []Identifier
	for _, id := range s.Identifiers {
		if id.System == system {
			result = append(result, id)
		}
	}
	return result
}

// GetMRN returns the MRN (type "MR") or primary identifier value.
func (s *IdentifierSet) GetMRN() string {
	if mrn := s.GetByType("MR"); mrn != nil {
		return mrn.Value
	}
	if s.Primary != nil {
		return s.Primary.Value
	}
	if len(s.Identifiers) > 0 {
		return s.Identifiers[0].Value
	}
	return ""
}

// CodeableConcept represents a coded value with optional mappings.
// Supports LOCAL codes mapped to standard systems (LOINC, SNOMED, etc.)
type CodeableConcept struct {
	// Coding contains one or more code representations
	Coding []Coding `json:"coding"`

	// Text is the plain text representation
	Text string `json:"text,omitempty"`
}

// Coding represents a single code in a code system.
type Coding struct {
	// System is the code system URI (e.g., "http://loinc.org")
	System string `json:"system"`

	// Code is the code value
	Code string `json:"code"`

	// Display is the human-readable name
	Display string `json:"display,omitempty"`

	// Version is the code system version
	Version string `json:"version,omitempty"`

	// OriginalSystem captures the source system before mapping (e.g., "LOCAL_LAB")
	OriginalSystem string `json:"original_system,omitempty"`

	// OriginalCode captures the source code before mapping
	OriginalCode string `json:"original_code,omitempty"`

	// MappingEquivalence indicates mapping quality: equivalent, wider, narrower, inexact
	MappingEquivalence string `json:"mapping_equivalence,omitempty"`

	// UserSelected indicates if this was the code selected by the user/system
	UserSelected bool `json:"user_selected,omitempty"`
}

// Address represents a physical address.
type Address struct {
	Line1      string `json:"line1,omitempty"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city,omitempty"`
	State      string `json:"state,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	Country    string `json:"country,omitempty"`
	Type       string `json:"type,omitempty"` // HOME, WORK, TEMP, etc.
}

// Provider represents a healthcare provider.
type Provider struct {
	// NPI is the National Provider Identifier (convenience field)
	NPI string `json:"npi,omitempty"`

	// ID is the internal/local provider ID (convenience field)
	ID string `json:"id,omitempty"`

	// Identifiers contains all known provider IDs (NPI, DEA, State License, PTAN, etc.)
	Identifiers IdentifierSet `json:"identifiers,omitempty"`

	// Name components
	FamilyName string `json:"family_name"`
	GivenName  string `json:"given_name"`
	MiddleName string `json:"middle_name,omitempty"`
	Prefix     string `json:"prefix,omitempty"` // Dr., etc.
	Suffix     string `json:"suffix,omitempty"` // MD, DO, NP, etc.
	Degree     string `json:"degree,omitempty"` // Professional degree

	// Classification
	Specialty    string   `json:"specialty,omitempty"`
	Specialties  []string `json:"specialties,omitempty"`
	ProviderType string   `json:"provider_type,omitempty"` // Type 1 (individual) or Type 2 (org)
	Taxonomy     string   `json:"taxonomy,omitempty"`      // Healthcare provider taxonomy code

	// Organization (for Type 2 NPIs)
	OrganizationName string `json:"organization_name,omitempty"`

	// Extensions holds source-specific data
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// Location represents a healthcare facility or unit.
type Location struct {
	Facility    string `json:"facility,omitempty"`
	Building    string `json:"building,omitempty"`
	Floor       string `json:"floor,omitempty"`
	Unit        string `json:"unit,omitempty"`
	Room        string `json:"room,omitempty"`
	Bed         string `json:"bed,omitempty"`
	Description string `json:"description,omitempty"`
}

// Encounter represents a patient encounter/visit.
type Encounter struct {
	// ID is the encounter/visit identifier
	ID string `json:"id"`

	// Identifiers contains all known encounter IDs (visit number, account number, etc.)
	Identifiers IdentifierSet `json:"identifiers,omitempty"`

	// Class indicates the encounter type: INPATIENT, OUTPATIENT, EMERGENCY, PREADMIT, etc.
	// Derived from PV1-2 (Patient Class) with Source Profile event classification rules
	Class string `json:"class"`

	// ClassifiedEventType is the semantic event type after disambiguation
	// e.g., "inpatient_admit" vs "outpatient_registration" for A01
	ClassifiedEventType string `json:"classified_event_type,omitempty"`

	Type   string `json:"type,omitempty"`
	Status string `json:"status,omitempty"`

	// Timing
	AdmitDateTime     time.Time `json:"admit_datetime,omitempty"`
	DischargeDateTime time.Time `json:"discharge_datetime,omitempty"`

	// Location
	Location Location `json:"location,omitempty"`

	// Providers
	AttendingProvider  *Provider `json:"attending_provider,omitempty"`
	AdmittingProvider  *Provider `json:"admitting_provider,omitempty"`
	ReferringProvider  *Provider `json:"referring_provider,omitempty"`
	ConsultingProvider *Provider `json:"consulting_provider,omitempty"`

	// Administrative
	AdmitSource          string `json:"admit_source,omitempty"`
	DischargeDisposition string `json:"discharge_disposition,omitempty"`
	ServiceType          string `json:"service_type,omitempty"`
	FinancialClass       string `json:"financial_class,omitempty"`

	// Extensions holds Z-segment and other source-specific data
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// LabTest represents a laboratory test.
type LabTest struct {
	// Code is the test code (uses CodeableConcept for proper terminology handling)
	Code CodeableConcept `json:"code"`

	// Convenience fields (derived from Code)
	LOINCCode   string `json:"loinc_code,omitempty"`
	LocalCode   string `json:"local_code,omitempty"`
	Description string `json:"description"`

	// Category (Chemistry, Hematology, Microbiology, etc.)
	Category string `json:"category,omitempty"`

	// Panel indicates if this is part of a panel (e.g., CBC, BMP)
	Panel string `json:"panel,omitempty"`

	// OrderID links back to the order that requested this test
	OrderID string `json:"order_id,omitempty"`
}

// LabValue represents a laboratory result value.
type LabValue struct {
	Value           string    `json:"value"`
	Unit            string    `json:"unit,omitempty"`
	ReferenceRange  string    `json:"reference_range,omitempty"`
	Interpretation  string    `json:"interpretation,omitempty"` // Normal, High, Low, Critical
	Status          string    `json:"status,omitempty"`         // Final, Preliminary, Corrected
	ObservationTime time.Time `json:"observation_time,omitempty"`
}

// Appointment represents a scheduled appointment.
type Appointment struct {
	ID           string    `json:"id"`
	Type         string    `json:"type,omitempty"`
	Status       string    `json:"status,omitempty"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time,omitempty"`
	Duration     int       `json:"duration_minutes,omitempty"`
	Location     Location  `json:"location,omitempty"`
	Provider     *Provider `json:"provider,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	Instructions string    `json:"instructions,omitempty"`

	// Rescheduling/cancellation fields
	PreviousStatus     string `json:"previous_status,omitempty"`
	CancellationReason string `json:"cancellation_reason,omitempty"`
	NoShow             bool   `json:"noshow,omitempty"`
}

// PatientAdmitEvent is emitted when a patient is admitted.
type PatientAdmitEvent struct {
	EventMeta
	Patient    Patient         `json:"patient"`
	Encounter  Encounter       `json:"encounter"`
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// PatientDischargeEvent is emitted when a patient is discharged.
type PatientDischargeEvent struct {
	EventMeta
	Patient    Patient         `json:"patient"`
	Encounter  Encounter       `json:"encounter"`
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// LabObservation pairs a test with its result value.
// Used for multi-OBX messages where each OBX is a separate observation.
type LabObservation struct {
	Test   LabTest  `json:"test"`
	Result LabValue `json:"result"`
}

// LabResultEvent is emitted when a lab result is received.
type LabResultEvent struct {
	EventMeta
	Patient          Patient          `json:"patient"`
	OrderingProvider *Provider        `json:"ordering_provider,omitempty"`
	Test             LabTest          `json:"test"`              // Primary/first test (for single-OBX compat)
	Result           LabValue         `json:"result"`            // Primary/first result (for single-OBX compat)
	Results          []LabObservation `json:"results,omitempty"` // All observations (for multi-OBX)
	IsCritical       bool             `json:"is_critical"`
	Encounter        *Encounter       `json:"encounter,omitempty"`
	RawPayload       json.RawMessage  `json:"raw_payload,omitempty"`
}

// AppointmentEvent is emitted for scheduling events.
type AppointmentEvent struct {
	EventMeta
	Patient     Patient         `json:"patient"`
	Appointment Appointment     `json:"appointment"`
	RawPayload  json.RawMessage `json:"raw_payload,omitempty"`
}

// Claim represents a healthcare claim.
type Claim struct {
	// ID is the claim identifier
	ID string `json:"id"`

	// ControlNumber is the submitter's claim control number
	ControlNumber string `json:"control_number,omitempty"`

	// TotalAmount is the total claim charge
	TotalAmount float64 `json:"total_amount"`

	// PlaceOfService is the place of service code
	PlaceOfService string `json:"place_of_service,omitempty"`

	// ServiceDate is the date of service
	ServiceDate time.Time `json:"service_date,omitempty"`

	// ServiceLines are the individual service line items
	ServiceLines []ServiceLine `json:"service_lines,omitempty"`

	// DiagnosisCodes is the list of diagnosis codes
	DiagnosisCodes []string `json:"diagnosis_codes,omitempty"`

	// PayerClaimID is the payer's control number (from 835)
	PayerClaimID string `json:"payer_claim_id,omitempty"`

	// Extensions holds source-specific data
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// ServiceLine represents a single service line item on a claim.
type ServiceLine struct {
	// LineNumber is the sequence number
	LineNumber int `json:"line_number"`

	// ProcedureCode is the CPT/HCPCS code
	ProcedureCode string `json:"procedure_code"`

	// Modifiers are the procedure modifiers
	Modifiers []string `json:"modifiers,omitempty"`

	// ChargeAmount is the amount charged
	ChargeAmount float64 `json:"charge_amount"`

	// Units is the number of units
	Units float64 `json:"units"`

	// UnitType is the unit basis code (UN=Unit, etc.)
	UnitType string `json:"unit_type,omitempty"`

	// ServiceDate is the date of this specific service
	ServiceDate time.Time `json:"service_date,omitempty"`

	// DiagnosisPointers link to claim-level diagnosis codes
	DiagnosisPointers []int `json:"diagnosis_pointers,omitempty"`
}

// ClaimAdjustment represents an adjustment to a claim payment.
type ClaimAdjustment struct {
	// Group is the adjustment group code (CO, PR, OA, PI, CR)
	Group string `json:"group"`

	// ReasonCode is the CARC (Claim Adjustment Reason Code)
	ReasonCode string `json:"reason_code"`

	// Amount is the adjustment amount
	Amount float64 `json:"amount"`

	// Quantity is the adjustment quantity
	Quantity int `json:"quantity,omitempty"`
}

// ClaimPayment holds payment information for a claim.
type ClaimPayment struct {
	// ClaimID is the original claim identifier
	ClaimID string `json:"claim_id"`

	// PayerClaimID is the payer's control number
	PayerClaimID string `json:"payer_claim_id,omitempty"`

	// Status is the claim status (Processed, Denied, etc.)
	Status string `json:"status"`

	// ChargedAmount is the original charge amount
	ChargedAmount float64 `json:"charged_amount"`

	// PaidAmount is the amount paid
	PaidAmount float64 `json:"paid_amount"`

	// PatientResponsibility is the patient responsibility amount
	PatientResponsibility float64 `json:"patient_responsibility,omitempty"`

	// Adjustments are the claim-level adjustments
	Adjustments []ClaimAdjustment `json:"adjustments,omitempty"`

	// ServiceLinePayments are the line-level payments
	ServiceLinePayments []ServiceLinePayment `json:"service_line_payments,omitempty"`
}

// ServiceLinePayment holds payment information for a service line.
type ServiceLinePayment struct {
	// ProcedureCode is the service code
	ProcedureCode string `json:"procedure_code"`

	// ChargedAmount is the charged amount
	ChargedAmount float64 `json:"charged_amount"`

	// PaidAmount is the paid amount
	PaidAmount float64 `json:"paid_amount"`

	// Units is the number of units paid
	Units float64 `json:"units,omitempty"`

	// Adjustments are line-level adjustments
	Adjustments []ClaimAdjustment `json:"adjustments,omitempty"`
}

// ClaimSubmittedEvent is emitted when a claim is submitted.
type ClaimSubmittedEvent struct {
	EventMeta
	Patient           Patient         `json:"patient"`
	BillingProvider   Provider        `json:"billing_provider"`
	RenderingProvider *Provider       `json:"rendering_provider,omitempty"`
	Payer             Provider        `json:"payer"`
	Subscriber        Patient         `json:"subscriber"`
	Claim             Claim           `json:"claim"`
	RawPayload        json.RawMessage `json:"raw_payload,omitempty"`
}

// ClaimAdjudicatedEvent is emitted when a claim is adjudicated (835 remittance).
type ClaimAdjudicatedEvent struct {
	EventMeta
	Payer       Provider        `json:"payer"`
	Payee       Provider        `json:"payee"`
	CheckNumber string          `json:"check_number,omitempty"`
	CheckDate   time.Time       `json:"check_date,omitempty"`
	TotalPaid   float64         `json:"total_paid"`
	Payment     ClaimPayment    `json:"payment"`
	RawPayload  json.RawMessage `json:"raw_payload,omitempty"`
}

// Event is a generic container that can hold any event type.
type Event struct {
	EventMeta
	Data json.RawMessage `json:"data"`
}

// EligibilityServiceType indicates what type of coverage is being inquired about.
type EligibilityServiceType string

const (
	EligibilityServiceHealth              EligibilityServiceType = "30" // Health Benefit Plan Coverage
	EligibilityServiceMedicalCare         EligibilityServiceType = "1"  // Medical Care
	EligibilityServiceSurgical            EligibilityServiceType = "2"  // Surgical
	EligibilityServiceConsultation        EligibilityServiceType = "3"  // Consultation
	EligibilityServiceDiagnosticXRay      EligibilityServiceType = "4"  // Diagnostic X-Ray
	EligibilityServiceDiagnosticLab       EligibilityServiceType = "5"  // Diagnostic Lab
	EligibilityServiceRadiation           EligibilityServiceType = "6"  // Radiation Therapy
	EligibilityServiceAnesthesia          EligibilityServiceType = "7"  // Anesthesia
	EligibilityServiceSurgicalAssistance  EligibilityServiceType = "8"  // Surgical Assistance
	EligibilityServiceProfessionalPhys    EligibilityServiceType = "96" // Professional (Physician)
	EligibilityServiceEmergencyServices   EligibilityServiceType = "88" // Emergency Services
	EligibilityServicePharmacy            EligibilityServiceType = "89" // Pharmacy
	EligibilityServiceDME                 EligibilityServiceType = "12" // DME (Durable Medical Equipment)
	EligibilityServiceMentalHealth        EligibilityServiceType = "MH" // Mental Health
	EligibilityServiceSubstanceAbuse      EligibilityServiceType = "AJ" // Substance Abuse
	EligibilityServiceHospitalInpatient   EligibilityServiceType = "47" // Hospital - Inpatient
	EligibilityServiceHospitalOutpatient  EligibilityServiceType = "48" // Hospital - Outpatient
	EligibilityServiceUrgentCare          EligibilityServiceType = "UC" // Urgent Care
	EligibilityServicePreventive          EligibilityServiceType = "A4" // Preventive Care
	EligibilityServiceChiropractic        EligibilityServiceType = "CH" // Chiropractic
)

// EligibilityInquiry represents a request for eligibility information.
type EligibilityInquiry struct {
	// ServiceTypes are the benefit categories being inquired about
	ServiceTypes []EligibilityServiceType `json:"service_types,omitempty"`

	// ServiceDate is the date of service for eligibility check
	ServiceDate time.Time `json:"service_date,omitempty"`

	// DateRange for eligibility period
	DateRangeStart time.Time `json:"date_range_start,omitempty"`
	DateRangeEnd   time.Time `json:"date_range_end,omitempty"`
}

// EligibilityBenefit represents coverage/benefit information from a 271 response.
type EligibilityBenefit struct {
	// InformationCode is the EB01 code indicating the type of benefit info
	// Common values: 1=Active Coverage, 6=Inactive, 8=Not Covered, C=Deductible
	InformationCode string `json:"information_code"`

	// InformationCodeDescription is human-readable description
	InformationCodeDescription string `json:"information_code_description,omitempty"`

	// CoverageLevel indicates individual, family, etc. (EB02)
	CoverageLevel string `json:"coverage_level,omitempty"`

	// ServiceType is the benefit category (EB03)
	ServiceType string `json:"service_type,omitempty"`

	// ServiceTypeDescription is human-readable service type
	ServiceTypeDescription string `json:"service_type_description,omitempty"`

	// InsuranceType indicates plan type (EB04): HM=HMO, PR=PPO, PS=POS, etc.
	InsuranceType string `json:"insurance_type,omitempty"`

	// PlanDescription is the plan/product name (EB05)
	PlanDescription string `json:"plan_description,omitempty"`

	// TimePeriodQualifier indicates unit of time (EB06): 7=Day, 13=Week, 21=Year, 22=Visit, 23=Visit, 24=Hour
	TimePeriodQualifier string `json:"time_period_qualifier,omitempty"`

	// Amount is the monetary amount (EB07): deductible, copay, coinsurance, etc.
	Amount float64 `json:"amount,omitempty"`

	// Percent is the percentage (EB08): coinsurance percentage
	Percent float64 `json:"percent,omitempty"`

	// QuantityQualifier (EB09): 99=Quantity, VS=Visit
	QuantityQualifier string `json:"quantity_qualifier,omitempty"`

	// Quantity is the numeric quantity (EB10): number of visits, days, etc.
	Quantity float64 `json:"quantity,omitempty"`

	// AuthorizationRequired indicates if prior auth is needed (EB11)
	AuthorizationRequired bool `json:"authorization_required,omitempty"`

	// InNetworkIndicator indicates in-network or out-of-network (EB12)
	// Y = Yes (In-Network), N = No (Out-of-Network), W = Not Applicable
	InNetworkIndicator string `json:"in_network_indicator,omitempty"`

	// EffectiveDate is when this benefit becomes effective
	EffectiveDate time.Time `json:"effective_date,omitempty"`

	// TerminationDate is when this benefit ends
	TerminationDate time.Time `json:"termination_date,omitempty"`

	// Messages are additional benefit information (MSG segments)
	Messages []string `json:"messages,omitempty"`
}

// EligibilityStatus represents the overall eligibility status.
type EligibilityStatus string

const (
	EligibilityStatusActive   EligibilityStatus = "active"
	EligibilityStatusInactive EligibilityStatus = "inactive"
	EligibilityStatusRejected EligibilityStatus = "rejected"
	EligibilityStatusUnknown  EligibilityStatus = "unknown"
)

// EligibilityValidationError represents an error/rejection from the payer.
type EligibilityValidationError struct {
	// Code is the AAA01 error code
	Code string `json:"code"`

	// RejectReasonCode is the AAA03 reject reason
	RejectReasonCode string `json:"reject_reason_code"`

	// FollowUpActionCode is the AAA04 follow-up action
	FollowUpActionCode string `json:"follow_up_action_code,omitempty"`

	// Message is human-readable error description
	Message string `json:"message,omitempty"`
}

// EligibilityInquiryEvent is emitted when an eligibility inquiry is submitted (270).
type EligibilityInquiryEvent struct {
	EventMeta
	// InformationSource is typically the payer being queried
	InformationSource Provider `json:"information_source"`

	// InformationReceiver is typically the provider making the inquiry
	InformationReceiver Provider `json:"information_receiver"`

	// Subscriber is the insurance subscriber
	Subscriber Patient `json:"subscriber"`

	// Dependent is the patient if different from subscriber
	Dependent *Patient `json:"dependent,omitempty"`

	// Inquiry contains the service types being asked about
	Inquiry EligibilityInquiry `json:"inquiry"`

	// TraceNumber is the submitter's trace/reference number
	TraceNumber string `json:"trace_number,omitempty"`

	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// EligibilityResponseEvent is emitted when an eligibility response is received (271).
type EligibilityResponseEvent struct {
	EventMeta
	// InformationSource is the payer responding
	InformationSource Provider `json:"information_source"`

	// InformationReceiver is the provider receiving the response
	InformationReceiver Provider `json:"information_receiver"`

	// Subscriber is the insurance subscriber
	Subscriber Patient `json:"subscriber"`

	// Dependent is the patient if different from subscriber
	Dependent *Patient `json:"dependent,omitempty"`

	// Status is the overall eligibility status
	Status EligibilityStatus `json:"status"`

	// Benefits contains detailed coverage information
	Benefits []EligibilityBenefit `json:"benefits,omitempty"`

	// Errors contains validation errors/rejections
	Errors []EligibilityValidationError `json:"errors,omitempty"`

	// TraceNumber is the original trace/reference number from inquiry
	TraceNumber string `json:"trace_number,omitempty"`

	// PlanDates
	PlanBeginDate time.Time `json:"plan_begin_date,omitempty"`
	PlanEndDate   time.Time `json:"plan_end_date,omitempty"`

	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// --- Claim Status Types (276/277) ---

// ClaimStatusCategoryCode represents the category of claim status (STC01-01).
// Based on Code Source 507 - Health Care Claim Status Category Codes.
type ClaimStatusCategoryCode string

const (
	ClaimStatusCategoryAcknowledgement ClaimStatusCategoryCode = "A0" // Acknowledgement
	ClaimStatusCategoryPending         ClaimStatusCategoryCode = "A1" // Pending
	ClaimStatusCategoryFinalized       ClaimStatusCategoryCode = "A2" // Finalized
	ClaimStatusCategoryRequest         ClaimStatusCategoryCode = "A3" // Request for additional information
	ClaimStatusCategoryAdjudicated     ClaimStatusCategoryCode = "A4" // Adjudicated
	ClaimStatusCategoryDenied          ClaimStatusCategoryCode = "A5" // Denied
	ClaimStatusCategoryPartialPay      ClaimStatusCategoryCode = "A6" // Partial payment
	ClaimStatusCategoryPaid            ClaimStatusCategoryCode = "A7" // Paid in full
	ClaimStatusCategoryRejected        ClaimStatusCategoryCode = "A8" // Rejected
	ClaimStatusCategoryRecovery        ClaimStatusCategoryCode = "F0" // Recovery
	ClaimStatusCategoryDataReporting   ClaimStatusCategoryCode = "DR" // Data Reporting
	ClaimStatusCategoryError           ClaimStatusCategoryCode = "E0" // Error
	ClaimStatusCategoryPriorAuth       ClaimStatusCategoryCode = "P0" // Prior Authorization
	ClaimStatusCategoryReferral        ClaimStatusCategoryCode = "R0" // Referral
)

// ClaimStatusInquiry represents the claim identification for a status request (276).
type ClaimStatusInquiry struct {
	// ClaimSubmitterID is the original claim ID submitted by the provider
	ClaimSubmitterID string `json:"claim_submitter_id,omitempty"`

	// PayerClaimID is the payer's internal claim identifier
	PayerClaimID string `json:"payer_claim_id,omitempty"`

	// ClearinghouseTraceNumber is the clearinghouse trace number
	ClearinghouseTraceNumber string `json:"clearinghouse_trace_number,omitempty"`

	// PatientControlNumber is the patient account number
	PatientControlNumber string `json:"patient_control_number,omitempty"`

	// ServiceDateStart is the start of service date range
	ServiceDateStart time.Time `json:"service_date_start,omitempty"`

	// ServiceDateEnd is the end of service date range
	ServiceDateEnd time.Time `json:"service_date_end,omitempty"`

	// TotalClaimChargeAmount from the original claim
	TotalClaimChargeAmount float64 `json:"total_claim_charge_amount,omitempty"`
}

// ClaimStatusInfo represents claim status information from a 277 response.
type ClaimStatusInfo struct {
	// StatusCategoryCode is the high-level status category (STC01-01, Code Source 507)
	StatusCategoryCode ClaimStatusCategoryCode `json:"status_category_code"`

	// StatusCategoryDescription is human-readable category
	StatusCategoryDescription string `json:"status_category_description,omitempty"`

	// StatusCode is the detailed status code (STC01-02, Code Source 508)
	StatusCode string `json:"status_code"`

	// StatusCodeDescription is human-readable status
	StatusCodeDescription string `json:"status_code_description,omitempty"`

	// EntityIdentifier indicates which entity the status relates to
	EntityIdentifier string `json:"entity_identifier,omitempty"`

	// EffectiveDate is when this status became effective
	EffectiveDate time.Time `json:"effective_date,omitempty"`

	// ActionCode indicates what action is required (if any)
	ActionCode string `json:"action_code,omitempty"`

	// TotalClaimChargeAmount is the claim amount
	TotalClaimChargeAmount float64 `json:"total_claim_charge_amount,omitempty"`

	// PaymentAmount is the paid amount (if applicable)
	PaymentAmount float64 `json:"payment_amount,omitempty"`

	// PaymentDate is when payment was issued
	PaymentDate time.Time `json:"payment_date,omitempty"`

	// CheckNumber is the payment check/EFT number
	CheckNumber string `json:"check_number,omitempty"`
}

// ClaimServiceLineStatus represents status for a specific service line.
type ClaimServiceLineStatus struct {
	// LineNumber is the service line number
	LineNumber int `json:"line_number,omitempty"`

	// ServiceIDQualifier indicates the code type (HC=HCPCS, N4=NDC)
	ServiceIDQualifier string `json:"service_id_qualifier,omitempty"`

	// ProcedureCode is the CPT/HCPCS/NDC code
	ProcedureCode string `json:"procedure_code,omitempty"`

	// Modifiers are procedure modifiers
	Modifiers []string `json:"modifiers,omitempty"`

	// ChargeAmount is the line charge
	ChargeAmount float64 `json:"charge_amount,omitempty"`

	// PaidAmount is the line paid amount
	PaidAmount float64 `json:"paid_amount,omitempty"`

	// Units is the quantity
	Units float64 `json:"units,omitempty"`

	// ServiceDate is when service was rendered
	ServiceDate time.Time `json:"service_date,omitempty"`

	// Statuses are the status details for this line
	Statuses []ClaimStatusInfo `json:"statuses,omitempty"`
}

// ClaimStatusRequestEvent is emitted when a claim status inquiry is submitted (276).
type ClaimStatusRequestEvent struct {
	EventMeta

	// Payer is the payer being queried
	Payer Provider `json:"payer"`

	// Provider is the provider making the inquiry
	Provider Provider `json:"provider"`

	// Subscriber is the insurance subscriber
	Subscriber Patient `json:"subscriber"`

	// Dependent is the patient if different from subscriber
	Dependent *Patient `json:"dependent,omitempty"`

	// Inquiry contains the claim identification
	Inquiry ClaimStatusInquiry `json:"inquiry"`

	// TraceNumber is the submitter's trace/reference number
	TraceNumber string `json:"trace_number,omitempty"`

	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// ClaimStatusResponseEvent is emitted when a claim status response is received (277).
type ClaimStatusResponseEvent struct {
	EventMeta

	// Payer is the payer responding
	Payer Provider `json:"payer"`

	// Provider is the provider receiving the response
	Provider Provider `json:"provider"`

	// Subscriber is the insurance subscriber
	Subscriber Patient `json:"subscriber"`

	// Dependent is the patient if different from subscriber
	Dependent *Patient `json:"dependent,omitempty"`

	// ClaimIdentification echoes back the claim identification
	ClaimSubmitterID     string `json:"claim_submitter_id,omitempty"`
	PayerClaimID         string `json:"payer_claim_id,omitempty"`
	PatientControlNumber string `json:"patient_control_number,omitempty"`

	// Statuses are the claim-level status details
	Statuses []ClaimStatusInfo `json:"statuses,omitempty"`

	// ServiceLines are the line-level status details (if returned)
	ServiceLines []ClaimServiceLineStatus `json:"service_lines,omitempty"`

	// TraceNumber is the original trace/reference number from request
	TraceNumber string `json:"trace_number,omitempty"`

	// TotalClaimChargeAmount from the original claim
	TotalClaimChargeAmount float64 `json:"total_claim_charge_amount,omitempty"`

	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// --- Clinical Document Events ---

// VitalSign represents a vital sign measurement.
type VitalSign struct {
	// Name is the display name of the vital sign
	Name string `json:"name"`

	// LOINCCode is the LOINC code for this vital sign
	LOINCCode string `json:"loinc_code,omitempty"`

	// Value is the measured value
	Value string `json:"value"`

	// Unit is the unit of measure
	Unit string `json:"unit,omitempty"`

	// Interpretation is the clinical interpretation (normal, high, low, critical)
	Interpretation string `json:"interpretation,omitempty"`
}

// VitalSignEvent is emitted for vital sign measurements.
type VitalSignEvent struct {
	EventMeta
	Patient   *Patient        `json:"patient,omitempty"`
	VitalSign VitalSign       `json:"vital_sign"`
	Encounter *Encounter      `json:"encounter,omitempty"`
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// Condition represents a medical condition/diagnosis.
type Condition struct {
	// Name is the display name of the condition
	Name string `json:"name"`

	// Code is the condition code (SNOMED, ICD-10, etc.)
	Code string `json:"code,omitempty"`

	// CodeSystem is the code system URI
	CodeSystem string `json:"code_system,omitempty"`

	// Category is the condition category (problem-list-item, encounter-diagnosis, etc.)
	Category string `json:"category,omitempty"`
}

// ConditionEvent is emitted for medical conditions/diagnoses.
type ConditionEvent struct {
	EventMeta
	Patient        *Patient        `json:"patient,omitempty"`
	Condition      Condition       `json:"condition"`
	ClinicalStatus string          `json:"clinical_status,omitempty"` // active, resolved, inactive
	OnsetDate      string          `json:"onset_date,omitempty"`
	AbatementDate  string          `json:"abatement_date,omitempty"`
	Encounter      *Encounter      `json:"encounter,omitempty"`
	RawPayload     json.RawMessage `json:"raw_payload,omitempty"`
}

// Procedure represents a medical procedure.
type Procedure struct {
	// Name is the display name of the procedure
	Name string `json:"name"`

	// Code is the procedure code (CPT, SNOMED, etc.)
	Code string `json:"code,omitempty"`

	// CodeSystem is the code system URI
	CodeSystem string `json:"code_system,omitempty"`

	// Status is the procedure status
	Status string `json:"status,omitempty"`
}

// ProcedureEvent is emitted for medical procedures.
type ProcedureEvent struct {
	EventMeta
	Patient       *Patient        `json:"patient,omitempty"`
	Procedure     Procedure       `json:"procedure"`
	PerformedDate string          `json:"performed_date,omitempty"`
	Performer     *Provider       `json:"performer,omitempty"`
	Location      *Location       `json:"location,omitempty"`
	Encounter     *Encounter      `json:"encounter,omitempty"`
	RawPayload    json.RawMessage `json:"raw_payload,omitempty"`
}

// Immunization represents a vaccine administration.
type Immunization struct {
	// VaccineCode is the vaccine code (CVX, etc.)
	VaccineCode string `json:"vaccine_code,omitempty"`

	// VaccineName is the display name of the vaccine
	VaccineName string `json:"vaccine_name,omitempty"`

	// Status is the immunization status
	Status string `json:"status,omitempty"`

	// LotNumber is the vaccine lot number
	LotNumber string `json:"lot_number,omitempty"`

	// Site is the administration site
	Site string `json:"site,omitempty"`

	// Route is the administration route
	Route string `json:"route,omitempty"`

	// DoseQuantity is the dose administered
	DoseQuantity string `json:"dose_quantity,omitempty"`
}

// ImmunizationEvent is emitted for immunization/vaccine administration.
type ImmunizationEvent struct {
	EventMeta
	Patient          *Patient        `json:"patient,omitempty"`
	Immunization     Immunization    `json:"immunization"`
	AdministeredDate string          `json:"administered_date,omitempty"`
	Performer        *Provider       `json:"performer,omitempty"`
	Location         *Location       `json:"location,omitempty"`
	Encounter        *Encounter      `json:"encounter,omitempty"`
	RawPayload       json.RawMessage `json:"raw_payload,omitempty"`
}

// Medication represents a medication/drug.
type Medication struct {
	// Code is the medication code (RxNorm preferred)
	Code string `json:"code,omitempty"`

	// CodeSystem is the code system (e.g., "http://www.nlm.nih.gov/research/umls/rxnorm")
	CodeSystem string `json:"code_system,omitempty"`

	// Name is the display name of the medication
	Name string `json:"name,omitempty"`

	// Form is the dose form (tablet, capsule, injection, etc.)
	Form string `json:"form,omitempty"`

	// Strength is the medication strength (e.g., "500mg", "10mg/5mL")
	Strength string `json:"strength,omitempty"`

	// Manufacturer is the drug manufacturer
	Manufacturer string `json:"manufacturer,omitempty"`
}

// MedicationRequest represents a prescription or medication order.
type MedicationRequest struct {
	// Medication contains the medication details
	Medication Medication `json:"medication"`

	// Status is the request status (active, completed, cancelled, etc.)
	Status string `json:"status,omitempty"`

	// Intent is the request intent (order, plan, proposal, etc.)
	Intent string `json:"intent,omitempty"`

	// AuthoredOn is when the request was created (RFC3339 format)
	AuthoredOn string `json:"authored_on,omitempty"`

	// DosageInstruction contains sig/directions
	DosageInstruction string `json:"dosage_instruction,omitempty"`

	// DoseQuantity is the dose amount per administration
	DoseQuantity string `json:"dose_quantity,omitempty"`

	// DoseUnit is the unit for the dose (e.g., "mg", "mL", "tablet")
	DoseUnit string `json:"dose_unit,omitempty"`

	// Frequency is the dosing frequency (e.g., "BID", "TID", "Q8H")
	Frequency string `json:"frequency,omitempty"`

	// Route is the administration route (oral, IV, topical, etc.)
	Route string `json:"route,omitempty"`

	// DispenseQuantity is the total quantity to dispense
	DispenseQuantity float64 `json:"dispense_quantity,omitempty"`

	// DispenseUnit is the unit for dispense quantity
	DispenseUnit string `json:"dispense_unit,omitempty"`

	// DaysSupply is the expected supply duration
	DaysSupply int `json:"days_supply,omitempty"`

	// NumberOfRefills is the number of authorized refills
	NumberOfRefills int `json:"number_of_refills,omitempty"`

	// Substitution indicates if generic substitution is allowed
	Substitution bool `json:"substitution,omitempty"`

	// ReasonCode is the indication/reason for the medication
	ReasonCode string `json:"reason_code,omitempty"`

	// ReasonText is the textual reason for the medication
	ReasonText string `json:"reason_text,omitempty"`

	// PriorAuthRequired indicates if prior authorization is needed
	PriorAuthRequired bool `json:"prior_auth_required,omitempty"`
}

// MedicationRequestEvent is emitted for prescription/medication order events.
type MedicationRequestEvent struct {
	EventMeta
	Patient           *Patient          `json:"patient,omitempty"`
	MedicationRequest MedicationRequest `json:"medication_request"`
	Prescriber        *Provider         `json:"prescriber,omitempty"`
	PharmacyID        string            `json:"pharmacy_id,omitempty"`
	PharmacyName      string            `json:"pharmacy_name,omitempty"`
	Encounter         *Encounter        `json:"encounter,omitempty"`
	RawPayload        json.RawMessage   `json:"raw_payload,omitempty"`
}

// AllergyIntolerance represents a patient allergy or intolerance.
type AllergyIntolerance struct {
	// Code is the allergy/substance code (RxNorm, SNOMED, UNII)
	Code string `json:"code,omitempty"`

	// CodeSystem is the code system for the allergy code
	CodeSystem string `json:"code_system,omitempty"`

	// Name is the display name of the allergen/substance
	Name string `json:"name,omitempty"`

	// Type distinguishes allergy from intolerance
	Type string `json:"type,omitempty"` // allergy, intolerance

	// Category is the category of the allergen (food, medication, environment, biologic)
	Category string `json:"category,omitempty"`

	// Criticality is the potential for harm (low, high, unable-to-assess)
	Criticality string `json:"criticality,omitempty"`

	// ClinicalStatus is the clinical status (active, inactive, resolved)
	ClinicalStatus string `json:"clinical_status,omitempty"`

	// VerificationStatus is the verification state (unconfirmed, confirmed, refuted, entered-in-error)
	VerificationStatus string `json:"verification_status,omitempty"`

	// OnsetDate is when the allergy was first identified
	OnsetDate string `json:"onset_date,omitempty"`

	// RecordedDate is when this record was created
	RecordedDate string `json:"recorded_date,omitempty"`

	// Reactions contains the reaction manifestations
	Reactions []AllergyReaction `json:"reactions,omitempty"`
}

// AllergyReaction represents a specific reaction to an allergen.
type AllergyReaction struct {
	// Substance is the specific substance that caused the reaction (if different from main allergen)
	Substance string `json:"substance,omitempty"`

	// Manifestation is the clinical manifestation code (SNOMED)
	Manifestation string `json:"manifestation,omitempty"`

	// ManifestationText is the description of the reaction
	ManifestationText string `json:"manifestation_text,omitempty"`

	// Severity is the reaction severity (mild, moderate, severe)
	Severity string `json:"severity,omitempty"`

	// OnsetDate is when this reaction occurred
	OnsetDate string `json:"onset_date,omitempty"`

	// Note contains additional details about the reaction
	Note string `json:"note,omitempty"`
}

// AllergyIntoleranceEvent is emitted for patient allergy/intolerance events.
type AllergyIntoleranceEvent struct {
	EventMeta
	Patient            *Patient           `json:"patient,omitempty"`
	AllergyIntolerance AllergyIntolerance `json:"allergy_intolerance"`
	Recorder           *Provider          `json:"recorder,omitempty"`
	Encounter          *Encounter         `json:"encounter,omitempty"`
	RawPayload         json.RawMessage    `json:"raw_payload,omitempty"`
}

// DocumentEvent is emitted for clinical document events.
type DocumentEvent struct {
	EventMeta
	Patient      *Patient        `json:"patient,omitempty"`
	DocumentType string          `json:"document_type"`
	Title        string          `json:"title,omitempty"`
	Author       *Provider       `json:"author,omitempty"`
	Custodian    string          `json:"custodian,omitempty"`
	Encounter    *Encounter      `json:"encounter,omitempty"`
	RawPayload   json.RawMessage `json:"raw_payload,omitempty"`
}

// CarePlan represents a care plan for a patient.
type CarePlan struct {
	// Title is the human-readable title of the care plan
	Title string `json:"title,omitempty"`

	// Description provides additional details about the care plan
	Description string `json:"description,omitempty"`

	// Status is the plan status (draft, active, on-hold, revoked, completed, entered-in-error, unknown)
	Status string `json:"status,omitempty"`

	// Intent is the plan intent (proposal, plan, order, option)
	Intent string `json:"intent,omitempty"`

	// Category is the type of care plan (assess-plan, discharge, etc.)
	Category string `json:"category,omitempty"`

	// Period is the time period the plan covers
	PeriodStart string `json:"period_start,omitempty"`
	PeriodEnd   string `json:"period_end,omitempty"`

	// Goals are the goals addressed by this care plan
	GoalIDs []string `json:"goal_ids,omitempty"`

	// Conditions are the health issues addressed by this plan
	ConditionIDs []string `json:"condition_ids,omitempty"`

	// Activities are the planned activities
	Activities []CarePlanActivity `json:"activities,omitempty"`
}

// CarePlanActivity represents a planned activity in a care plan.
type CarePlanActivity struct {
	// OutcomeDescription describes the activity outcome
	OutcomeDescription string `json:"outcome_description,omitempty"`

	// Status is the activity status (not-started, scheduled, in-progress, on-hold, completed, cancelled, stopped, unknown, entered-in-error)
	Status string `json:"status,omitempty"`

	// Code is the activity code (SNOMED, CPT, etc.)
	Code string `json:"code,omitempty"`

	// CodeSystem is the code system for the activity code
	CodeSystem string `json:"code_system,omitempty"`

	// Description is the activity description
	Description string `json:"description,omitempty"`

	// ScheduledDate is when the activity is scheduled
	ScheduledDate string `json:"scheduled_date,omitempty"`
}

// CarePlanEvent is emitted for care plan events.
type CarePlanEvent struct {
	EventMeta
	Patient    *Patient        `json:"patient,omitempty"`
	CarePlan   CarePlan        `json:"care_plan"`
	Author     *Provider       `json:"author,omitempty"`
	CareTeam   []*Provider     `json:"care_team,omitempty"`
	Encounter  *Encounter      `json:"encounter,omitempty"`
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// Goal represents a patient goal.
type Goal struct {
	// Description is the goal description (required by US Core)
	Description string `json:"description"`

	// LifecycleStatus is the goal status (proposed, planned, accepted, active, on-hold, completed, cancelled, entered-in-error, rejected)
	LifecycleStatus string `json:"lifecycle_status,omitempty"`

	// AchievementStatus is the achievement status (in-progress, improving, worsening, no-change, achieved, sustaining, not-achieved, no-progress, not-attainable)
	AchievementStatus string `json:"achievement_status,omitempty"`

	// Category is the goal category (dietary, safety, behavioral, nursing, physiotherapy, etc.)
	Category string `json:"category,omitempty"`

	// Priority is the goal priority (high-priority, medium-priority, low-priority)
	Priority string `json:"priority,omitempty"`

	// StartDate is when the goal was established
	StartDate string `json:"start_date,omitempty"`

	// TargetDate is the target date for achieving the goal
	TargetDate string `json:"target_date,omitempty"`

	// StatusDate is when the status was last updated
	StatusDate string `json:"status_date,omitempty"`

	// StatusReason explains why the goal has its current status
	StatusReason string `json:"status_reason,omitempty"`

	// ExpressedBy indicates who set the goal (patient, practitioner, related person)
	ExpressedBy string `json:"expressed_by,omitempty"`

	// Addresses are the conditions/diagnoses this goal addresses
	AddressesIDs []string `json:"addresses_ids,omitempty"`

	// Note contains additional details about the goal
	Note string `json:"note,omitempty"`

	// Target contains the measurable outcome target
	Target *GoalTarget `json:"target,omitempty"`
}

// GoalTarget represents a measurable target for a goal.
type GoalTarget struct {
	// Measure is what is being measured (LOINC code, etc.)
	Measure string `json:"measure,omitempty"`

	// MeasureSystem is the code system for measure
	MeasureSystem string `json:"measure_system,omitempty"`

	// DetailQuantity is the target value as a quantity
	DetailQuantity float64 `json:"detail_quantity,omitempty"`

	// DetailUnit is the unit for the quantity
	DetailUnit string `json:"detail_unit,omitempty"`

	// DetailString is the target as a string description
	DetailString string `json:"detail_string,omitempty"`

	// DueDate is when the target should be achieved
	DueDate string `json:"due_date,omitempty"`
}

// GoalEvent is emitted for patient goal events.
type GoalEvent struct {
	EventMeta
	Patient    *Patient        `json:"patient,omitempty"`
	Goal       Goal            `json:"goal"`
	Author     *Provider       `json:"author,omitempty"`
	Encounter  *Encounter      `json:"encounter,omitempty"`
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// CareTeamMember represents a participant in a care team.
type CareTeamMember struct {
	// Role is the role of the member (e.g., "primary care physician", "nurse", "case manager")
	Role string `json:"role,omitempty"`

	// RoleCode is the coded role (SNOMED CT preferred)
	RoleCode string `json:"role_code,omitempty"`

	// RoleCodeSystem is the code system for the role
	RoleCodeSystem string `json:"role_code_system,omitempty"`

	// Provider is the practitioner/organization member
	Provider *Provider `json:"provider,omitempty"`

	// OrganizationID references an organization member
	OrganizationID string `json:"organization_id,omitempty"`

	// OrganizationName is the display name of the organization
	OrganizationName string `json:"organization_name,omitempty"`

	// PeriodStart is when the member joined the care team
	PeriodStart string `json:"period_start,omitempty"`

	// PeriodEnd is when the member left the care team
	PeriodEnd string `json:"period_end,omitempty"`
}

// CareTeam represents a care team for a patient.
type CareTeam struct {
	// Name is the human-readable name of the care team
	Name string `json:"name,omitempty"`

	// Status is the care team status (proposed, active, suspended, inactive, entered-in-error)
	Status string `json:"status,omitempty"`

	// Category is the type of care team (e.g., "episode", "condition", "longitudinal")
	Category string `json:"category,omitempty"`

	// PeriodStart is when the care team was established
	PeriodStart string `json:"period_start,omitempty"`

	// PeriodEnd is when the care team was disbanded
	PeriodEnd string `json:"period_end,omitempty"`

	// ReasonCode is the coded reason for the care team (SNOMED CT)
	ReasonCode string `json:"reason_code,omitempty"`

	// ReasonCodeSystem is the code system for the reason
	ReasonCodeSystem string `json:"reason_code_system,omitempty"`

	// ReasonText is the text description of the reason
	ReasonText string `json:"reason_text,omitempty"`

	// ConditionIDs are the conditions this care team addresses
	ConditionIDs []string `json:"condition_ids,omitempty"`

	// Members are the care team participants
	Members []CareTeamMember `json:"members,omitempty"`

	// ManagingOrganizationID is the organization responsible for the care team
	ManagingOrganizationID string `json:"managing_organization_id,omitempty"`

	// ManagingOrganizationName is the display name of the managing organization
	ManagingOrganizationName string `json:"managing_organization_name,omitempty"`

	// Note contains additional notes about the care team
	Note string `json:"note,omitempty"`
}

// CareTeamEvent is emitted for care team events.
type CareTeamEvent struct {
	EventMeta
	Patient    *Patient        `json:"patient,omitempty"`
	CareTeam   CareTeam        `json:"care_team"`
	Encounter  *Encounter      `json:"encounter,omitempty"`
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// ServiceRequest represents a request for a service (order, referral, procedure request).
type ServiceRequest struct {
	// Status is the request status (draft, active, on-hold, revoked, completed, entered-in-error, unknown)
	Status string `json:"status,omitempty"`

	// Intent is the request intent (proposal, plan, directive, order, original-order, reflex-order, filler-order, instance-order, option)
	Intent string `json:"intent,omitempty"`

	// Category is the type of service (e.g., "laboratory", "imaging", "procedure", "counseling", "referral")
	Category string `json:"category,omitempty"`

	// Priority is the request priority (routine, urgent, asap, stat)
	Priority string `json:"priority,omitempty"`

	// Code is the service being requested (CPT, SNOMED CT, LOINC)
	Code string `json:"code,omitempty"`

	// CodeSystem is the code system for the service code
	CodeSystem string `json:"code_system,omitempty"`

	// CodeText is the text description of the service
	CodeText string `json:"code_text,omitempty"`

	// OrderDetail provides additional details about the order
	OrderDetail string `json:"order_detail,omitempty"`

	// QuantityValue is the quantity of the service requested
	QuantityValue float64 `json:"quantity_value,omitempty"`

	// QuantityUnit is the unit for the quantity
	QuantityUnit string `json:"quantity_unit,omitempty"`

	// OccurrenceDateTime is when the service should occur
	OccurrenceDateTime string `json:"occurrence_date_time,omitempty"`

	// OccurrencePeriodStart is the start of the occurrence period
	OccurrencePeriodStart string `json:"occurrence_period_start,omitempty"`

	// OccurrencePeriodEnd is the end of the occurrence period
	OccurrencePeriodEnd string `json:"occurrence_period_end,omitempty"`

	// AuthoredOn is when the request was created
	AuthoredOn string `json:"authored_on,omitempty"`

	// ReasonCode is the coded reason for the request
	ReasonCode string `json:"reason_code,omitempty"`

	// ReasonCodeSystem is the code system for the reason
	ReasonCodeSystem string `json:"reason_code_system,omitempty"`

	// ReasonText is the text description of the reason
	ReasonText string `json:"reason_text,omitempty"`

	// ConditionIDs are conditions that justify the service request
	ConditionIDs []string `json:"condition_ids,omitempty"`

	// BodySite is the anatomical location (SNOMED CT)
	BodySite string `json:"body_site,omitempty"`

	// BodySiteCode is the coded body site
	BodySiteCode string `json:"body_site_code,omitempty"`

	// Note contains additional instructions or comments
	Note string `json:"note,omitempty"`

	// PatientInstruction is instructions for the patient
	PatientInstruction string `json:"patient_instruction,omitempty"`
}

// ServiceRequestEvent is emitted for service request events (orders, referrals).
type ServiceRequestEvent struct {
	EventMeta
	Patient        *Patient        `json:"patient,omitempty"`
	ServiceRequest ServiceRequest  `json:"service_request"`
	Requester      *Provider       `json:"requester,omitempty"`
	Performer      *Provider       `json:"performer,omitempty"`
	PerformerOrgID string          `json:"performer_org_id,omitempty"`
	PerformerOrgName string        `json:"performer_org_name,omitempty"`
	Encounter      *Encounter      `json:"encounter,omitempty"`
	RawPayload     json.RawMessage `json:"raw_payload,omitempty"`
}

// LabTest convenience fields
func (t *LabTest) GetName() string {
	if t.Description != "" {
		return t.Description
	}
	if t.Code.Text != "" {
		return t.Code.Text
	}
	if len(t.Code.Coding) > 0 && t.Code.Coding[0].Display != "" {
		return t.Code.Coding[0].Display
	}
	return ""
}

// LabResult is an alias for LabValue for mapper compatibility
type LabResult = LabValue

// NewEventMeta creates a new EventMeta with default values.
func NewEventMeta(eventType EventType, source string, format SourceFormat) EventMeta {
	now := time.Now().UTC()
	return EventMeta{
		ID:           generateID(),
		Type:         eventType,
		Timestamp:    now,
		ReceivedAt:   now,
		Source:       source,
		SourceFormat: format,
	}
}

// generateID creates a unique event ID using UUID v4.
// UUID v4 uses random bytes and is suitable for distributed systems.
func generateID() string {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		// Fallback to timestamp if crypto/rand fails (should never happen)
		return time.Now().UTC().Format("20060102150405.000000")
	}
	// Set version (4) and variant (RFC 4122)
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant is 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
