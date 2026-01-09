// Package events defines the canonical semantic event model for healthcare integrations.
// These types abstract away format-specific details (HL7v2, FHIR, CSV, EDI) and provide
// a unified data model that workflows can operate on.
package events

import (
	"encoding/json"
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
)

// SourceFormat indicates the original format of the data.
type SourceFormat string

const (
	FormatHL7v2   SourceFormat = "hl7v2"
	FormatFHIR    SourceFormat = "fhir"
	FormatCSV     SourceFormat = "csv"
	FormatEDI835  SourceFormat = "edi_835"
	FormatEDI837  SourceFormat = "edi_837"
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

// generateID creates a unique event ID.
// TODO: Use UUID or ULID for production.
func generateID() string {
	return time.Now().UTC().Format("20060102150405.000000")
}
