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

	// Document management events (MDM)
	EventDocumentOriginal     EventType = "document_original"      // MDM^T01/T02
	EventDocumentStatusChange EventType = "document_status_change" // MDM^T03/T04
	EventDocumentAddendum     EventType = "document_addendum"      // MDM^T05/T06
	EventDocumentEdit         EventType = "document_edit"          // MDM^T08/T09
	EventDocumentReplacement  EventType = "document_replacement"   // MDM^T10/T11

	// Financial transaction events (DFT)
	EventFinancialTransaction EventType = "financial_transaction" // DFT^P03/P11

	// Medications
	EventMedicationRequest EventType = "medication_request"

	// Allergies
	EventAllergyIntolerance EventType = "allergy_intolerance"

	// Social History
	EventSocialHistory EventType = "social_history"
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

	// ExtractedEntities contains clinical entities extracted from unstructured text
	// by LLM-powered extraction (e.g., from clinical notes in MDM messages).
	ExtractedEntities *ExtractedEntities `json:"extracted_entities,omitempty"`

	// QualityScore contains data quality metrics computed by LLM analysis.
	QualityScore *DataQualityScore `json:"quality_score,omitempty"`
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

	// Explanation is a human-readable explanation generated by LLM (optional)
	// This helps integration analysts understand warnings without deep format expertise.
	Explanation string `json:"explanation,omitempty"`

	// FixSuggestion provides actionable guidance on how to fix the issue (optional)
	// Generated by LLM when explain_warnings transform is applied.
	FixSuggestion string `json:"fix_suggestion,omitempty"`
}

// ExtractedEntities contains clinical entities extracted from unstructured text.
// Populated by LLM-powered extraction from clinical notes (MDM), CDA narratives, etc.
type ExtractedEntities struct {
	// Conditions are extracted diagnoses/problems
	Conditions []ExtractedCondition `json:"conditions,omitempty"`

	// Medications are extracted medication references
	Medications []ExtractedMedication `json:"medications,omitempty"`

	// VitalSigns are extracted vital sign measurements
	VitalSigns []ExtractedVitalSign `json:"vital_signs,omitempty"`

	// Allergies are extracted allergy information
	Allergies []ExtractedAllergy `json:"allergies,omitempty"`

	// Procedures are extracted procedure references
	Procedures []ExtractedProcedure `json:"procedures,omitempty"`

	// SocialHistory contains social history observations found in the text
	SocialHistory []ExtractedSocialHistory `json:"social_history,omitempty"`

	// Confidence is the overall extraction confidence (0.0-1.0)
	Confidence float64 `json:"confidence"`

	// ExtractedAt is when the extraction was performed
	ExtractedAt time.Time `json:"extracted_at"`

	// Model is the LLM model used for extraction
	Model string `json:"model,omitempty"`

	// SourceText is the text that was analyzed (for audit purposes)
	SourceText string `json:"source_text,omitempty"`
}

// ExtractedCondition represents a condition/diagnosis extracted from text.
type ExtractedCondition struct {
	// Name is the extracted condition name
	Name string `json:"name"`

	// Code is the standardized code (SNOMED, ICD-10)
	Code string `json:"code,omitempty"`

	// CodeSystem identifies the code system
	CodeSystem string `json:"code_system,omitempty"`

	// Confidence is the extraction confidence for this entity (0.0-1.0)
	Confidence float64 `json:"confidence,omitempty"`

	// Negated indicates if the condition was negated ("no diabetes")
	Negated bool `json:"negated,omitempty"`

	// Status is the clinical status (active, resolved, etc.)
	Status string `json:"status,omitempty"`

	// TextSpan is the original text span that was extracted
	TextSpan string `json:"text_span,omitempty"`
}

// ExtractedMedication represents a medication extracted from text.
type ExtractedMedication struct {
	// Name is the extracted medication name
	Name string `json:"name"`

	// Code is the standardized code (RxNorm)
	Code string `json:"code,omitempty"`

	// CodeSystem identifies the code system
	CodeSystem string `json:"code_system,omitempty"`

	// Confidence is the extraction confidence for this entity (0.0-1.0)
	Confidence float64 `json:"confidence,omitempty"`

	// Dosage is the extracted dosage information
	Dosage string `json:"dosage,omitempty"`

	// Frequency is the extracted frequency
	Frequency string `json:"frequency,omitempty"`

	// Route is the administration route
	Route string `json:"route,omitempty"`

	// TextSpan is the original text span that was extracted
	TextSpan string `json:"text_span,omitempty"`
}

// ExtractedVitalSign represents a vital sign extracted from text.
type ExtractedVitalSign struct {
	// Name is the vital sign name
	Name string `json:"name"`

	// LOINCCode is the LOINC code
	LOINCCode string `json:"loinc_code,omitempty"`

	// Value is the measured value
	Value string `json:"value"`

	// Unit is the unit of measure
	Unit string `json:"unit,omitempty"`

	// Confidence is the extraction confidence (0.0-1.0)
	Confidence float64 `json:"confidence,omitempty"`

	// Interpretation is clinical interpretation (normal, high, low, critical)
	Interpretation string `json:"interpretation,omitempty"`

	// TextSpan is the original text span
	TextSpan string `json:"text_span,omitempty"`
}

// ExtractedAllergy represents an allergy extracted from text.
type ExtractedAllergy struct {
	// Substance is the allergen
	Substance string `json:"substance"`

	// Code is the standardized code (RxNorm, SNOMED)
	Code string `json:"code,omitempty"`

	// CodeSystem identifies the code system
	CodeSystem string `json:"code_system,omitempty"`

	// Reaction is the documented reaction
	Reaction string `json:"reaction,omitempty"`

	// Severity is the reaction severity (mild, moderate, severe)
	Severity string `json:"severity,omitempty"`

	// Confidence is the extraction confidence (0.0-1.0)
	Confidence float64 `json:"confidence,omitempty"`

	// TextSpan is the original text span
	TextSpan string `json:"text_span,omitempty"`
}

// ExtractedProcedure represents a procedure extracted from text.
type ExtractedProcedure struct {
	// Name is the procedure name
	Name string `json:"name"`

	// Code is the standardized code (CPT, SNOMED)
	Code string `json:"code,omitempty"`

	// CodeSystem identifies the code system
	CodeSystem string `json:"code_system,omitempty"`

	// Confidence is the extraction confidence (0.0-1.0)
	Confidence float64 `json:"confidence,omitempty"`

	// Status is the procedure status
	Status string `json:"status,omitempty"`

	// TextSpan is the original text span
	TextSpan string `json:"text_span,omitempty"`
}

// ExtractedSocialHistory represents a social history observation extracted from text.
type ExtractedSocialHistory struct {
	// Name is the name of the observation (e.g., "smoking status")
	Name string `json:"name"`

	// Code is the standardized code (LOINC, SNOMED)
	Code string `json:"code,omitempty"`

	// CodeSystem identifies the code system
	CodeSystem string `json:"code_system,omitempty"`

	// Value is the observation value
	Value string `json:"value,omitempty"`

	// Confidence is the extraction confidence for this entity (0.0-1.0)
	Confidence float64 `json:"confidence,omitempty"`

	// TextSpan is the original text span
	TextSpan string `json:"text_span,omitempty"`
}

// DataQualityScore contains quality metrics from LLM analysis.
type DataQualityScore struct {
	// OverallScore is the aggregate quality score (0.0-1.0)
	OverallScore float64 `json:"overall_score"`

	// Dimensions contains per-dimension scores
	// Common dimensions: completeness, accuracy, consistency, conformance, timeliness
	Dimensions map[string]float64 `json:"dimensions,omitempty"`

	// Issues contains specific quality issues identified
	Issues []DataQualityIssue `json:"issues,omitempty"`

	// Recommendations are actionable suggestions for improvement
	Recommendations []QualityRecommendation `json:"recommendations,omitempty"`

	// AnalyzedAt is when the quality analysis was performed
	AnalyzedAt time.Time `json:"analyzed_at"`

	// Model is the LLM model used for analysis
	Model string `json:"model,omitempty"`
}

// DataQualityIssue represents a specific quality issue found.
type DataQualityIssue struct {
	// Dimension is which quality dimension is affected
	Dimension string `json:"dimension"`

	// Severity is the issue severity (info, warning, error)
	Severity string `json:"severity"`

	// Field is the affected field path
	Field string `json:"field,omitempty"`

	// Description explains the issue
	Description string `json:"description"`

	// Impact describes the potential impact
	Impact string `json:"impact,omitempty"`
}

// QualityRecommendation provides actionable guidance.
type QualityRecommendation struct {
	// Category is the recommendation category
	Category string `json:"category"`

	// Priority is the recommendation priority (low, medium, high)
	Priority string `json:"priority"`

	// Recommendation is the actionable suggestion
	Recommendation string `json:"recommendation"`

	// ExpectedImprovement describes what would improve
	ExpectedImprovement string `json:"expected_improvement,omitempty"`
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
	EligibilityServiceHealth             EligibilityServiceType = "30" // Health Benefit Plan Coverage
	EligibilityServiceMedicalCare        EligibilityServiceType = "1"  // Medical Care
	EligibilityServiceSurgical           EligibilityServiceType = "2"  // Surgical
	EligibilityServiceConsultation       EligibilityServiceType = "3"  // Consultation
	EligibilityServiceDiagnosticXRay     EligibilityServiceType = "4"  // Diagnostic X-Ray
	EligibilityServiceDiagnosticLab      EligibilityServiceType = "5"  // Diagnostic Lab
	EligibilityServiceRadiation          EligibilityServiceType = "6"  // Radiation Therapy
	EligibilityServiceAnesthesia         EligibilityServiceType = "7"  // Anesthesia
	EligibilityServiceSurgicalAssistance EligibilityServiceType = "8"  // Surgical Assistance
	EligibilityServiceProfessionalPhys   EligibilityServiceType = "96" // Professional (Physician)
	EligibilityServiceEmergencyServices  EligibilityServiceType = "88" // Emergency Services
	EligibilityServicePharmacy           EligibilityServiceType = "89" // Pharmacy
	EligibilityServiceDME                EligibilityServiceType = "12" // DME (Durable Medical Equipment)
	EligibilityServiceMentalHealth       EligibilityServiceType = "MH" // Mental Health
	EligibilityServiceSubstanceAbuse     EligibilityServiceType = "AJ" // Substance Abuse
	EligibilityServiceHospitalInpatient  EligibilityServiceType = "47" // Hospital - Inpatient
	EligibilityServiceHospitalOutpatient EligibilityServiceType = "48" // Hospital - Outpatient
	EligibilityServiceUrgentCare         EligibilityServiceType = "UC" // Urgent Care
	EligibilityServicePreventive         EligibilityServiceType = "A4" // Preventive Care
	EligibilityServiceChiropractic       EligibilityServiceType = "CH" // Chiropractic
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
	Patient    *Patient        `json:"patient,omitempty"`
	VitalSign  VitalSign       `json:"vital_sign"`
	Encounter  *Encounter      `json:"encounter,omitempty"`
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

// SocialHistoryObservation represents a social history observation (smoking, alcohol, etc.).
type SocialHistoryObservation struct {
	// Code is the observation code (SNOMED, LOINC)
	Code string `json:"code,omitempty"`

	// CodeSystem is the code system for the observation code
	CodeSystem string `json:"code_system,omitempty"`

	// Name is the display name of the observation
	Name string `json:"name,omitempty"`

	// Value is the observation value (e.g., "Current every day smoker")
	Value string `json:"value,omitempty"`

	// ValueCode is the coded value (e.g., SNOMED code for smoking status)
	ValueCode string `json:"value_code,omitempty"`

	// Category is the inferred category (smoking-status, tobacco-use, alcohol-use, drug-use, etc.)
	Category string `json:"category,omitempty"`

	// EffectiveDate is when this observation was recorded
	EffectiveDate string `json:"effective_date,omitempty"`

	// Status is the observation status
	Status string `json:"status,omitempty"`
}

// SocialHistoryEvent is emitted for social history observations.
type SocialHistoryEvent struct {
	EventMeta
	Patient     *Patient                 `json:"patient,omitempty"`
	Observation SocialHistoryObservation `json:"observation"`
	Encounter   *Encounter               `json:"encounter,omitempty"`
	RawPayload  json.RawMessage          `json:"raw_payload,omitempty"`
}

// DocumentEvent is emitted for clinical document events (including MDM messages).
type DocumentEvent struct {
	EventMeta
	Patient      *Patient   `json:"patient,omitempty"`
	DocumentType string     `json:"document_type"`
	Title        string     `json:"title,omitempty"`
	Author       *Provider  `json:"author,omitempty"`
	Custodian    string     `json:"custodian,omitempty"`
	Encounter    *Encounter `json:"encounter,omitempty"`

	// MDM-specific fields
	UniqueDocumentNumber     string    `json:"unique_document_number,omitempty"`     // TXA-12
	ParentDocumentNumber     string    `json:"parent_document_number,omitempty"`     // TXA-13 (for addendum/replacement)
	DocumentStatus           string    `json:"document_status,omitempty"`            // TXA-17 code
	DocumentCompletionStatus string    `json:"document_completion_status,omitempty"` // TXA-17 mapped to readable status
	OriginationDateTime      time.Time `json:"origination_datetime,omitempty"`       // TXA-4
	TranscriptionDateTime    time.Time `json:"transcription_datetime,omitempty"`     // TXA-6
	EditDateTime             time.Time `json:"edit_datetime,omitempty"`              // TXA-7
	AuthenticationDateTime   time.Time `json:"authentication_datetime,omitempty"`    // TXA-22
	ContentType              string    `json:"content_type,omitempty"`               // OBX-2 value type (ED, TX, ST, FT)
	Content                  string    `json:"content,omitempty"`                    // OBX-5 document content
	ContentEncoding          string    `json:"content_encoding,omitempty"`           // "base64" or "text"

	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
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
	Patient          *Patient        `json:"patient,omitempty"`
	ServiceRequest   ServiceRequest  `json:"service_request"`
	Requester        *Provider       `json:"requester,omitempty"`
	Performer        *Provider       `json:"performer,omitempty"`
	PerformerOrgID   string          `json:"performer_org_id,omitempty"`
	PerformerOrgName string          `json:"performer_org_name,omitempty"`
	Encounter        *Encounter      `json:"encounter,omitempty"`
	RawPayload       json.RawMessage `json:"raw_payload,omitempty"`
}

// ============================================================================
// DocumentReference - Clinical Documents (US Core)
// ============================================================================

// DocumentReferenceContent represents the actual document content or reference.
type DocumentReferenceContent struct {
	// AttachmentURL is the URL where the document can be retrieved
	AttachmentURL string `json:"attachment_url,omitempty"`

	// AttachmentData is base64-encoded document content (for inline documents)
	AttachmentData string `json:"attachment_data,omitempty"`

	// AttachmentContentType is the MIME type (e.g., "application/pdf", "text/xml")
	AttachmentContentType string `json:"attachment_content_type,omitempty"`

	// AttachmentSize is the document size in bytes
	AttachmentSize int64 `json:"attachment_size,omitempty"`

	// AttachmentHash is the SHA-1 hash of the document (base64)
	AttachmentHash string `json:"attachment_hash,omitempty"`

	// AttachmentTitle is the document title
	AttachmentTitle string `json:"attachment_title,omitempty"`

	// AttachmentCreation is when the document was created
	AttachmentCreation string `json:"attachment_creation,omitempty"`

	// Format is the format code (e.g., "urn:hl7-org:sdwg:ccda-structuredBody:2.1")
	Format string `json:"format,omitempty"`

	// FormatSystem is the format code system
	FormatSystem string `json:"format_system,omitempty"`
}

// DocumentReferenceContext represents the clinical context of the document.
type DocumentReferenceContext struct {
	// EncounterID is the encounter during which the document was created
	EncounterID string `json:"encounter_id,omitempty"`

	// PeriodStart is the start of the time period the document covers
	PeriodStart string `json:"period_start,omitempty"`

	// PeriodEnd is the end of the time period the document covers
	PeriodEnd string `json:"period_end,omitempty"`

	// FacilityType is the type of facility (e.g., "Hospital", "Clinic")
	FacilityType string `json:"facility_type,omitempty"`

	// FacilityTypeCode is the coded facility type
	FacilityTypeCode string `json:"facility_type_code,omitempty"`

	// PracticeSetting is the clinical specialty (e.g., "Cardiology")
	PracticeSetting string `json:"practice_setting,omitempty"`

	// PracticeSettingCode is the coded practice setting
	PracticeSettingCode string `json:"practice_setting_code,omitempty"`
}

// DocumentReference represents a reference to a clinical document.
type DocumentReference struct {
	// Status is the document status (current, superseded, entered-in-error)
	Status string `json:"status,omitempty"`

	// DocStatus is the status of the underlying document (preliminary, final, amended)
	DocStatus string `json:"doc_status,omitempty"`

	// Type is the document type (e.g., "Discharge Summary", "Progress Note")
	Type string `json:"type,omitempty"`

	// TypeCode is the LOINC code for the document type
	TypeCode string `json:"type_code,omitempty"`

	// TypeCodeSystem is the code system for type (usually LOINC)
	TypeCodeSystem string `json:"type_code_system,omitempty"`

	// Category is the document category (clinical-note, cardiology, etc.)
	Category string `json:"category,omitempty"`

	// CategoryCode is the coded category
	CategoryCode string `json:"category_code,omitempty"`

	// CategoryCodeSystem is the category code system
	CategoryCodeSystem string `json:"category_code_system,omitempty"`

	// Date is when the document was indexed/created
	Date string `json:"date,omitempty"`

	// Description is a human-readable description
	Description string `json:"description,omitempty"`

	// SecurityLabel is the confidentiality level (e.g., "N" for normal, "R" for restricted)
	SecurityLabel string `json:"security_label,omitempty"`

	// Content is the document content/attachment
	Content []DocumentReferenceContent `json:"content,omitempty"`

	// Context is the clinical context
	Context *DocumentReferenceContext `json:"context,omitempty"`

	// CustodianID is the organization maintaining the document
	CustodianID string `json:"custodian_id,omitempty"`

	// CustodianName is the custodian organization name
	CustodianName string `json:"custodian_name,omitempty"`

	// RelatesTo is related documents (replaces, transforms, etc.)
	RelatesTo []DocumentReferenceRelation `json:"relates_to,omitempty"`
}

// DocumentReferenceRelation represents a relationship to another document.
type DocumentReferenceRelation struct {
	// Code is the relationship type (replaces, transforms, signs, appends)
	Code string `json:"code,omitempty"`

	// TargetID is the ID of the related document
	TargetID string `json:"target_id,omitempty"`
}

// DocumentReferenceEvent represents a document reference event for workflow processing.
type DocumentReferenceEvent struct {
	EventMeta
	Patient           *Patient          `json:"patient,omitempty"`
	DocumentReference DocumentReference `json:"document_reference"`
	Author            *Provider         `json:"author,omitempty"`
	AuthorOrgID       string            `json:"author_org_id,omitempty"`
	AuthorOrgName     string            `json:"author_org_name,omitempty"`
	Authenticator     *Provider         `json:"authenticator,omitempty"`
	Encounter         *Encounter        `json:"encounter,omitempty"`
	RawPayload        json.RawMessage   `json:"raw_payload,omitempty"`
}

// ============================================================================
// DiagnosticReport (Clinical Notes) - US Core DiagnosticReport for Report and Note Exchange
// ============================================================================

// DiagnosticReportNote represents a clinical note/report (not lab).
type DiagnosticReportNote struct {
	// Status is the report status (registered, partial, preliminary, final, amended, etc.)
	Status string `json:"status,omitempty"`

	// Category is the report category (e.g., "Radiology", "Pathology", "Cardiology")
	Category string `json:"category,omitempty"`

	// CategoryCode is the coded category (LOINC)
	CategoryCode string `json:"category_code,omitempty"`

	// CategoryCodeSystem is the category code system
	CategoryCodeSystem string `json:"category_code_system,omitempty"`

	// Code is the report type (e.g., "Chest X-ray", "Echocardiogram")
	Code string `json:"code,omitempty"`

	// CodeValue is the LOINC or other code for the report type
	CodeValue string `json:"code_value,omitempty"`

	// CodeSystem is the code system for the report code
	CodeSystem string `json:"code_system,omitempty"`

	// EffectiveDateTime is when the report was clinically relevant
	EffectiveDateTime string `json:"effective_date_time,omitempty"`

	// EffectivePeriodStart is the start of the effective period
	EffectivePeriodStart string `json:"effective_period_start,omitempty"`

	// EffectivePeriodEnd is the end of the effective period
	EffectivePeriodEnd string `json:"effective_period_end,omitempty"`

	// Issued is when the report was issued
	Issued string `json:"issued,omitempty"`

	// Conclusion is the clinical conclusion/interpretation
	Conclusion string `json:"conclusion,omitempty"`

	// ConclusionCode is a coded conclusion
	ConclusionCode string `json:"conclusion_code,omitempty"`

	// ConclusionCodeSystem is the code system for the conclusion
	ConclusionCodeSystem string `json:"conclusion_code_system,omitempty"`

	// PresentedForm is the rendered report (PDF, etc.)
	PresentedForm []DiagnosticReportAttachment `json:"presented_form,omitempty"`

	// Media is embedded images/media
	Media []DiagnosticReportMedia `json:"media,omitempty"`

	// ResultIDs are references to Observation resources
	ResultIDs []string `json:"result_ids,omitempty"`

	// ImagingStudyIDs are references to ImagingStudy resources
	ImagingStudyIDs []string `json:"imaging_study_ids,omitempty"`

	// SpecimenIDs are references to Specimen resources
	SpecimenIDs []string `json:"specimen_ids,omitempty"`
}

// DiagnosticReportAttachment represents an attached document.
type DiagnosticReportAttachment struct {
	// ContentType is the MIME type
	ContentType string `json:"content_type,omitempty"`

	// Data is base64-encoded content
	Data string `json:"data,omitempty"`

	// URL is where the content can be retrieved
	URL string `json:"url,omitempty"`

	// Size is the content size in bytes
	Size int64 `json:"size,omitempty"`

	// Hash is the SHA-1 hash (base64)
	Hash string `json:"hash,omitempty"`

	// Title is the attachment title
	Title string `json:"title,omitempty"`

	// Creation is when the attachment was created
	Creation string `json:"creation,omitempty"`
}

// DiagnosticReportMedia represents embedded media.
type DiagnosticReportMedia struct {
	// Comment is a description of the media
	Comment string `json:"comment,omitempty"`

	// Link is the media reference (usually to a Media resource)
	LinkID string `json:"link_id,omitempty"`
}

// DiagnosticReportNoteEvent represents a clinical note/report event.
type DiagnosticReportNoteEvent struct {
	EventMeta
	Patient              *Patient             `json:"patient,omitempty"`
	DiagnosticReportNote DiagnosticReportNote `json:"diagnostic_report_note"`
	Performer            *Provider            `json:"performer,omitempty"`
	PerformerOrgID       string               `json:"performer_org_id,omitempty"`
	PerformerOrgName     string               `json:"performer_org_name,omitempty"`
	Encounter            *Encounter           `json:"encounter,omitempty"`
	RawPayload           json.RawMessage      `json:"raw_payload,omitempty"`
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

// ============================================================================
// Provenance (US Core)
// ============================================================================

// ProvenanceAgent represents an actor (person, device, organization) that was
// involved in creating, modifying, or transmitting the target resources.
type ProvenanceAgent struct {
	// Type indicates the type of agent (author, performer, verifier, etc.)
	// US Core requires provenance-participant-type from http://terminology.hl7.org/CodeSystem/provenance-participant-type
	Type string `json:"type,omitempty"`

	// TypeCode is the coded agent type
	TypeCode string `json:"type_code,omitempty"`

	// TypeCodeSystem is the code system for the agent type
	TypeCodeSystem string `json:"type_code_system,omitempty"`

	// Role specifies the functional role of the agent (optional)
	Role string `json:"role,omitempty"`

	// RoleCode is the coded role
	RoleCode string `json:"role_code,omitempty"`

	// RoleCodeSystem is the code system for the role
	RoleCodeSystem string `json:"role_code_system,omitempty"`

	// Who identifies the agent - can be a Provider, Organization, or Device
	// Reference format: "Practitioner/id", "Organization/id", "Device/id"
	WhoReference string `json:"who_reference,omitempty"`

	// WhoDisplay is the display name for the agent
	WhoDisplay string `json:"who_display,omitempty"`

	// OnBehalfOf indicates the organization the agent was acting on behalf of
	OnBehalfOfReference string `json:"on_behalf_of_reference,omitempty"`

	// OnBehalfOfDisplay is the display name for the organization
	OnBehalfOfDisplay string `json:"on_behalf_of_display,omitempty"`
}

// ProvenanceEntity represents an entity used in an activity that produced the
// target resource (e.g., source document, input data).
type ProvenanceEntity struct {
	// Role indicates how the entity was used (derivation, revision, quotation, source, removal)
	Role string `json:"role,omitempty"`

	// WhatReference is a reference to the entity resource
	WhatReference string `json:"what_reference,omitempty"`

	// WhatDisplay is the display name for the entity
	WhatDisplay string `json:"what_display,omitempty"`

	// Agent is the agent that was involved with the entity
	Agent *ProvenanceAgent `json:"agent,omitempty"`
}

// Provenance captures information about the origin, derivation, and attestation
// of a set of resources. Used for data provenance tracking per USCDI v3.
type Provenance struct {
	// TargetReferences are the resources this provenance statement is about
	// US Core requires at least one target
	TargetReferences []string `json:"target_references"`

	// TargetDisplays are display names for the target resources
	TargetDisplays []string `json:"target_displays,omitempty"`

	// Recorded is when the activity was recorded (required by US Core)
	Recorded string `json:"recorded"`

	// OccurredDateTime is when the activity occurred (optional)
	OccurredDateTime string `json:"occurred_date_time,omitempty"`

	// OccurredPeriodStart is the start of the activity period (optional)
	OccurredPeriodStart string `json:"occurred_period_start,omitempty"`

	// OccurredPeriodEnd is the end of the activity period (optional)
	OccurredPeriodEnd string `json:"occurred_period_end,omitempty"`

	// Activity describes what activity occurred
	Activity string `json:"activity,omitempty"`

	// ActivityCode is the coded activity type
	ActivityCode string `json:"activity_code,omitempty"`

	// ActivityCodeSystem is the code system for the activity
	ActivityCodeSystem string `json:"activity_code_system,omitempty"`

	// Location is where the activity occurred
	LocationReference string `json:"location_reference,omitempty"`

	// LocationDisplay is the display name for the location
	LocationDisplay string `json:"location_display,omitempty"`

	// Reason describes why the activity occurred
	Reason string `json:"reason,omitempty"`

	// ReasonCode is the coded reason
	ReasonCode string `json:"reason_code,omitempty"`

	// ReasonCodeSystem is the code system for the reason
	ReasonCodeSystem string `json:"reason_code_system,omitempty"`

	// Agents are the actors involved in the activity (required by US Core - at least one)
	Agents []ProvenanceAgent `json:"agents"`

	// Entities are the entities used in the activity (optional)
	Entities []ProvenanceEntity `json:"entities,omitempty"`

	// Policy references external policy documents that apply
	Policy []string `json:"policy,omitempty"`

	// Signature contains digital signatures for attestation
	Signatures []ProvenanceSignature `json:"signatures,omitempty"`
}

// ProvenanceSignature represents a digital signature on provenance.
type ProvenanceSignature struct {
	// Type indicates the type of signature
	Type string `json:"type,omitempty"`

	// TypeCode is the coded signature type
	TypeCode string `json:"type_code,omitempty"`

	// When is when the signature was created
	When string `json:"when,omitempty"`

	// WhoReference is who signed
	WhoReference string `json:"who_reference,omitempty"`

	// WhoDisplay is the display name of the signer
	WhoDisplay string `json:"who_display,omitempty"`

	// TargetFormat is the MIME type of the signed content
	TargetFormat string `json:"target_format,omitempty"`

	// SigFormat is the MIME type of the signature
	SigFormat string `json:"sig_format,omitempty"`

	// Data is the base64-encoded signature value
	Data string `json:"data,omitempty"`
}

// ProvenanceEvent is a canonical event for provenance tracking.
type ProvenanceEvent struct {
	EventMeta

	// Provenance contains the provenance data
	Provenance Provenance `json:"provenance"`

	// RawPayload contains the original message if available
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// ============================================================================
// Location (US Core)
// ============================================================================

// FacilityLocation represents a physical place where healthcare services are provided.
// Note: Named FacilityLocation to avoid conflict with the simple Location struct used in Encounter.
type FacilityLocation struct {
	// ID is the unique identifier
	ID string `json:"id,omitempty"`

	// Status indicates whether the location is active
	Status string `json:"status,omitempty"`

	// Name is the human-readable name
	Name string `json:"name"`

	// Description provides additional information about the location
	Description string `json:"description,omitempty"`

	// Mode indicates whether this is a specific instance or a class
	Mode string `json:"mode,omitempty"`

	// Type indicates the type of location (e.g., hospital, clinic)
	Type string `json:"type,omitempty"`

	// TypeCode is the coded location type
	TypeCode string `json:"type_code,omitempty"`

	// TypeCodeSystem is the code system for the type
	TypeCodeSystem string `json:"type_code_system,omitempty"`

	// Address is the physical address
	Address *Address `json:"address,omitempty"`

	// PhysicalType describes what kind of physical space (building, room, etc.)
	PhysicalType string `json:"physical_type,omitempty"`

	// PhysicalTypeCode is the coded physical type
	PhysicalTypeCode string `json:"physical_type_code,omitempty"`

	// ManagingOrganizationID is the organization responsible for the location
	ManagingOrganizationID string `json:"managing_organization_id,omitempty"`

	// ManagingOrganizationName is the organization name
	ManagingOrganizationName string `json:"managing_organization_name,omitempty"`

	// PartOfLocationID is the parent location (for nested locations)
	PartOfLocationID string `json:"part_of_location_id,omitempty"`

	// Telecom contains contact information
	Phone string `json:"phone,omitempty"`
	Fax   string `json:"fax,omitempty"`
	Email string `json:"email,omitempty"`
}

// FacilityLocationEvent is a canonical event for facility location data.
type FacilityLocationEvent struct {
	EventMeta

	// FacilityLocation contains the location data
	FacilityLocation FacilityLocation `json:"facility_location"`

	// RawPayload contains the original message if available
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// ============================================================================
// Organization (US Core)
// ============================================================================

// Organization represents a formally or informally recognized grouping of
// people or organizations formed for the purpose of achieving some form of
// collective action (healthcare provider organizations, payers, etc.).
type Organization struct {
	// ID is the unique identifier
	ID string `json:"id,omitempty"`

	// Active indicates whether the organization is still in use
	Active bool `json:"active,omitempty"`

	// Type indicates the type of organization
	Type string `json:"type,omitempty"`

	// TypeCode is the coded organization type
	TypeCode string `json:"type_code,omitempty"`

	// TypeCodeSystem is the code system for the type
	TypeCodeSystem string `json:"type_code_system,omitempty"`

	// Name is the organization's name (required by US Core)
	Name string `json:"name"`

	// Alias contains alternative names
	Alias []string `json:"alias,omitempty"`

	// NPI is the National Provider Identifier (for healthcare organizations)
	NPI string `json:"npi,omitempty"`

	// TIN is the Tax Identification Number
	TIN string `json:"tin,omitempty"`

	// Address is the organization's address
	Address *Address `json:"address,omitempty"`

	// Telecom contains contact information
	Phone string `json:"phone,omitempty"`
	Fax   string `json:"fax,omitempty"`
	Email string `json:"email,omitempty"`

	// PartOfOrganizationID is the parent organization
	PartOfOrganizationID string `json:"part_of_organization_id,omitempty"`

	// PartOfOrganizationName is the parent organization name
	PartOfOrganizationName string `json:"part_of_organization_name,omitempty"`
}

// OrganizationEvent is a canonical event for organization data.
type OrganizationEvent struct {
	EventMeta

	// Organization contains the organization data
	Organization Organization `json:"organization"`

	// RawPayload contains the original message if available
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// ============================================================================
// Practitioner (US Core)
// ============================================================================

// Practitioner represents a person who is directly or indirectly involved
// in the provisioning of healthcare (physicians, nurses, technicians, etc.).
type Practitioner struct {
	// ID is the unique identifier
	ID string `json:"id,omitempty"`

	// Active indicates whether the practitioner is currently active
	Active bool `json:"active,omitempty"`

	// NPI is the National Provider Identifier (required by US Core)
	NPI string `json:"npi,omitempty"`

	// GivenName is the practitioner's first name
	GivenName string `json:"given_name,omitempty"`

	// MiddleName is the practitioner's middle name
	MiddleName string `json:"middle_name,omitempty"`

	// FamilyName is the practitioner's last name
	FamilyName string `json:"family_name,omitempty"`

	// Prefix is the name prefix (Dr., Mr., etc.)
	Prefix string `json:"prefix,omitempty"`

	// Suffix is the name suffix (Jr., MD, etc.)
	Suffix string `json:"suffix,omitempty"`

	// Gender is the administrative gender
	Gender string `json:"gender,omitempty"`

	// BirthDate is the practitioner's date of birth
	BirthDate string `json:"birth_date,omitempty"`

	// Address is the practitioner's address
	Address *Address `json:"address,omitempty"`

	// Telecom contains contact information
	Phone string `json:"phone,omitempty"`
	Email string `json:"email,omitempty"`

	// Qualifications are the practitioner's professional qualifications
	Qualifications []PractitionerQualification `json:"qualifications,omitempty"`

	// Communication languages
	Languages []string `json:"languages,omitempty"`
}

// PractitionerQualification represents a professional qualification.
type PractitionerQualification struct {
	// Code is the qualification code
	Code string `json:"code,omitempty"`

	// CodeSystem is the code system
	CodeSystem string `json:"code_system,omitempty"`

	// Display is the display name
	Display string `json:"display,omitempty"`

	// Issuer is the organization that issued the qualification
	IssuerID string `json:"issuer_id,omitempty"`

	// IssuerName is the issuer name
	IssuerName string `json:"issuer_name,omitempty"`

	// PeriodStart is when the qualification became effective
	PeriodStart string `json:"period_start,omitempty"`

	// PeriodEnd is when the qualification expires
	PeriodEnd string `json:"period_end,omitempty"`
}

// PractitionerEvent is a canonical event for practitioner data.
type PractitionerEvent struct {
	EventMeta

	// Practitioner contains the practitioner data
	Practitioner Practitioner `json:"practitioner"`

	// RawPayload contains the original message if available
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// ============================================================================
// PractitionerRole (US Core)
// ============================================================================

// PractitionerRole represents the role a practitioner plays at an organization.
type PractitionerRole struct {
	// ID is the unique identifier
	ID string `json:"id,omitempty"`

	// Active indicates whether the role is currently active
	Active bool `json:"active,omitempty"`

	// PractitionerID is the practitioner reference
	PractitionerID string `json:"practitioner_id,omitempty"`

	// PractitionerName is the practitioner display name
	PractitionerName string `json:"practitioner_name,omitempty"`

	// OrganizationID is the organization reference
	OrganizationID string `json:"organization_id,omitempty"`

	// OrganizationName is the organization display name
	OrganizationName string `json:"organization_name,omitempty"`

	// Code is the role code (e.g., physician, nurse)
	Code string `json:"code,omitempty"`

	// CodeValue is the coded role
	CodeValue string `json:"code_value,omitempty"`

	// CodeSystem is the code system for the role
	CodeSystem string `json:"code_system,omitempty"`

	// Specialty is the practitioner's specialty in this role
	Specialty string `json:"specialty,omitempty"`

	// SpecialtyCode is the coded specialty
	SpecialtyCode string `json:"specialty_code,omitempty"`

	// SpecialtyCodeSystem is the code system for the specialty
	SpecialtyCodeSystem string `json:"specialty_code_system,omitempty"`

	// LocationIDs are the locations where this role applies
	LocationIDs []string `json:"location_ids,omitempty"`

	// Telecom contains role-specific contact information
	Phone string `json:"phone,omitempty"`
	Email string `json:"email,omitempty"`

	// AvailableTimeStart is when the practitioner is available
	AvailableTimeStart string `json:"available_time_start,omitempty"`

	// AvailableTimeEnd is when availability ends
	AvailableTimeEnd string `json:"available_time_end,omitempty"`

	// AvailableDays are the days of the week available
	AvailableDays []string `json:"available_days,omitempty"`

	// PeriodStart is when this role started
	PeriodStart string `json:"period_start,omitempty"`

	// PeriodEnd is when this role ended
	PeriodEnd string `json:"period_end,omitempty"`
}

// PractitionerRoleEvent is a canonical event for practitioner role data.
type PractitionerRoleEvent struct {
	EventMeta

	// PractitionerRole contains the practitioner role data
	PractitionerRole PractitionerRole `json:"practitioner_role"`

	// RawPayload contains the original message if available
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// ============================================================================
// RelatedPerson (US Core)
// ============================================================================

// RelatedPerson represents a person who has a personal or non-healthcare-specific
// relationship to the patient (family members, guardians, caregivers).
type RelatedPerson struct {
	// ID is the unique identifier
	ID string `json:"id,omitempty"`

	// Active indicates whether the relationship is currently active
	Active bool `json:"active,omitempty"`

	// PatientID is the patient this person is related to
	PatientID string `json:"patient_id,omitempty"`

	// PatientName is the patient display name
	PatientName string `json:"patient_name,omitempty"`

	// Relationship describes how this person is related to the patient
	Relationship string `json:"relationship,omitempty"`

	// RelationshipCode is the coded relationship
	RelationshipCode string `json:"relationship_code,omitempty"`

	// RelationshipCodeSystem is the code system for the relationship
	RelationshipCodeSystem string `json:"relationship_code_system,omitempty"`

	// GivenName is the person's first name
	GivenName string `json:"given_name,omitempty"`

	// MiddleName is the person's middle name
	MiddleName string `json:"middle_name,omitempty"`

	// FamilyName is the person's last name
	FamilyName string `json:"family_name,omitempty"`

	// Prefix is the name prefix
	Prefix string `json:"prefix,omitempty"`

	// Suffix is the name suffix
	Suffix string `json:"suffix,omitempty"`

	// Gender is the administrative gender
	Gender string `json:"gender,omitempty"`

	// BirthDate is the person's date of birth
	BirthDate string `json:"birth_date,omitempty"`

	// Address is the person's address
	Address *Address `json:"address,omitempty"`

	// Telecom contains contact information
	Phone string `json:"phone,omitempty"`
	Email string `json:"email,omitempty"`

	// PeriodStart is when the relationship started
	PeriodStart string `json:"period_start,omitempty"`

	// PeriodEnd is when the relationship ended
	PeriodEnd string `json:"period_end,omitempty"`

	// Communication languages
	Languages []string `json:"languages,omitempty"`
}

// RelatedPersonEvent is a canonical event for related person data.
type RelatedPersonEvent struct {
	EventMeta

	// RelatedPerson contains the related person data
	RelatedPerson RelatedPerson `json:"related_person"`

	// RawPayload contains the original message if available
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}

// FinancialTransaction represents a financial transaction from DFT message (FT1 segment).
type FinancialTransaction struct {
	// SetID is the sequence number for this transaction (FT1-1)
	SetID int `json:"set_id,omitempty"`

	// TransactionID is the unique transaction identifier (FT1-2)
	TransactionID string `json:"transaction_id,omitempty"`

	// BatchID is the transaction batch identifier (FT1-3)
	BatchID string `json:"batch_id,omitempty"`

	// TransactionDate is when the transaction occurred (FT1-4)
	TransactionDate time.Time `json:"transaction_date,omitempty"`

	// PostingDate is when the transaction was posted (FT1-5)
	PostingDate time.Time `json:"posting_date,omitempty"`

	// TransactionType indicates the type: CG=Charge, CR=Credit, PA=Payment, etc. (FT1-6)
	TransactionType string `json:"transaction_type"`

	// TransactionCode is the charge/service code (FT1-7)
	TransactionCode CodeableConcept `json:"transaction_code"`

	// Quantity is the number of units (FT1-10)
	Quantity float64 `json:"quantity,omitempty"`

	// Amount is the extended transaction amount (FT1-11)
	Amount float64 `json:"amount,omitempty"`

	// UnitAmount is the per-unit amount (FT1-12)
	UnitAmount float64 `json:"unit_amount,omitempty"`

	// PatientLocation where service was performed (FT1-16)
	PatientLocation *Location `json:"patient_location,omitempty"`

	// DiagnosisCodes from FT1-19
	DiagnosisCodes []CodeableConcept `json:"diagnosis_codes,omitempty"`

	// PerformedBy is the provider who performed the service (FT1-20)
	PerformedBy *Provider `json:"performed_by,omitempty"`

	// OrderedBy is the provider who ordered the service (FT1-21)
	OrderedBy *Provider `json:"ordered_by,omitempty"`

	// FillerOrderNumber links to the order (FT1-23)
	FillerOrderNumber string `json:"filler_order_number,omitempty"`

	// EnteredBy is who entered the transaction (FT1-24)
	EnteredBy *Provider `json:"entered_by,omitempty"`

	// ProcedureCode is the CPT/HCPCS procedure code (FT1-25)
	ProcedureCode *CodeableConcept `json:"procedure_code,omitempty"`

	// ProcedureModifiers are CPT modifiers (FT1-26)
	ProcedureModifiers []string `json:"procedure_modifiers,omitempty"`

	// Diagnoses from associated DG1 segments
	Diagnoses []Diagnosis `json:"diagnoses,omitempty"`

	// Procedures from associated PR1 segments
	Procedures []ProcedureInfo `json:"procedures,omitempty"`
}

// Diagnosis represents a diagnosis from DG1 segment.
type Diagnosis struct {
	// SetID is the sequence number (DG1-1)
	SetID int `json:"set_id,omitempty"`

	// CodingMethod indicates the coding system: I9=ICD-9, I10=ICD-10 (DG1-2)
	CodingMethod string `json:"coding_method,omitempty"`

	// Code is the diagnosis code (DG1-3)
	Code CodeableConcept `json:"code"`

	// Description is the diagnosis description (DG1-4)
	Description string `json:"description,omitempty"`

	// DiagnosisDate is when the diagnosis was made (DG1-5)
	DiagnosisDate time.Time `json:"diagnosis_date,omitempty"`

	// DiagnosisType indicates: A=Admitting, W=Working, F=Final (DG1-6)
	DiagnosisType string `json:"diagnosis_type,omitempty"`

	// DiagnosingClinician is who made the diagnosis (DG1-16)
	DiagnosingClinician *Provider `json:"diagnosing_clinician,omitempty"`

	// IsPrimary indicates if this is the primary diagnosis (DG1-15 = 1)
	IsPrimary bool `json:"is_primary,omitempty"`
}

// ProcedureInfo represents a procedure from PR1 segment.
type ProcedureInfo struct {
	// SetID is the sequence number (PR1-1)
	SetID int `json:"set_id,omitempty"`

	// CodingMethod indicates the coding system (PR1-2)
	CodingMethod string `json:"coding_method,omitempty"`

	// Code is the procedure code (PR1-3)
	Code CodeableConcept `json:"code"`

	// Description is the procedure description (PR1-4)
	Description string `json:"description,omitempty"`

	// ProcedureDate is when the procedure was performed (PR1-5)
	ProcedureDate time.Time `json:"procedure_date,omitempty"`

	// FunctionalType indicates: A=Anesthesia, P=Procedure, I=Incision (PR1-6)
	FunctionalType string `json:"functional_type,omitempty"`

	// ProcedureMinutes is the duration in minutes (PR1-7)
	ProcedureMinutes int `json:"procedure_minutes,omitempty"`

	// Practitioner is who performed the procedure (PR1-8)
	Practitioner *Provider `json:"practitioner,omitempty"`

	// AnesthesiaCode is the anesthesia type code (PR1-9)
	AnesthesiaCode string `json:"anesthesia_code,omitempty"`
}

// InsuranceInfo represents insurance information from IN1 segment.
type InsuranceInfo struct {
	// SetID is the sequence number indicating coordination order (IN1-1)
	SetID int `json:"set_id,omitempty"`

	// PlanID is the insurance plan identifier (IN1-2)
	PlanID string `json:"plan_id,omitempty"`

	// CompanyID is the insurance company identifier (IN1-3)
	CompanyID string `json:"company_id,omitempty"`

	// CompanyName is the insurance company name (IN1-4)
	CompanyName string `json:"company_name,omitempty"`

	// GroupNumber is the group/policy number (IN1-8)
	GroupNumber string `json:"group_number,omitempty"`

	// GroupName is the group name (IN1-9)
	GroupName string `json:"group_name,omitempty"`

	// PolicyNumber is the policy number (IN1-36)
	PolicyNumber string `json:"policy_number,omitempty"`

	// SubscriberID is the subscriber identifier (IN1-49)
	SubscriberID string `json:"subscriber_id,omitempty"`

	// CoordinationOrder indicates primary (1), secondary (2), etc.
	CoordinationOrder int `json:"coordination_order,omitempty"`

	// EffectiveDate is when coverage begins (IN1-12)
	EffectiveDate time.Time `json:"effective_date,omitempty"`

	// ExpirationDate is when coverage ends (IN1-13)
	ExpirationDate time.Time `json:"expiration_date,omitempty"`
}

// FinancialTransactionEvent is emitted for DFT messages.
type FinancialTransactionEvent struct {
	EventMeta

	// Patient is the patient associated with the transactions
	Patient Patient `json:"patient"`

	// Encounter is the associated visit/encounter
	Encounter *Encounter `json:"encounter,omitempty"`

	// Transactions contains all FT1 segments from the message
	Transactions []FinancialTransaction `json:"transactions"`

	// TotalChargeAmount is the sum of all transaction amounts
	TotalChargeAmount float64 `json:"total_charge_amount,omitempty"`

	// InsuranceInfo contains coverage from IN1 segments
	InsuranceInfo []InsuranceInfo `json:"insurance_info,omitempty"`

	// AccountNumber is the billing account (PID-18)
	AccountNumber string `json:"account_number,omitempty"`

	// RawPayload contains the original message
	RawPayload json.RawMessage `json:"raw_payload,omitempty"`
}
