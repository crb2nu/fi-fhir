// Package extract provides LLM-powered clinical entity extraction from healthcare documents.
package extract

import (
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// ExtractionResult contains entities extracted from clinical text.
type ExtractionResult struct {
	// Conditions are medical conditions/diagnoses found in the text.
	Conditions []events.Condition `json:"conditions,omitempty"`

	// Medications are medications mentioned in the text.
	Medications []events.Medication `json:"medications,omitempty"`

	// VitalSigns are vital sign measurements found in the text.
	VitalSigns []events.VitalSign `json:"vital_signs,omitempty"`

	// Allergies are allergies/intolerances found in the text.
	Allergies []events.AllergyIntolerance `json:"allergies,omitempty"`

	// Procedures are medical procedures mentioned in the text.
	Procedures []events.Procedure `json:"procedures,omitempty"`

	// Confidence is the overall confidence score for the extraction (0.0-1.0).
	Confidence float64 `json:"confidence"`

	// ProcessingTime is how long the extraction took.
	ProcessingTime time.Duration `json:"processing_time,omitempty"`

	// Model is the LLM model used for extraction.
	Model string `json:"model,omitempty"`

	// Metadata contains additional extraction metadata.
	Metadata ExtractionMetadata `json:"metadata,omitempty"`
}

// ExtractionMetadata contains metadata about the extraction process.
type ExtractionMetadata struct {
	// DocumentType is the type of clinical document processed.
	DocumentType string `json:"document_type,omitempty"`

	// TextLength is the length of the input text in characters.
	TextLength int `json:"text_length,omitempty"`

	// TokensUsed is the number of tokens used for the extraction.
	TokensUsed int `json:"tokens_used,omitempty"`

	// ExtractedAt is when the extraction was performed.
	ExtractedAt time.Time `json:"extracted_at,omitempty"`

	// NegatedEntities is the count of negated entities detected.
	NegatedEntities int `json:"negated_entities,omitempty"`
}

// ExtractedCondition extends Condition with extraction-specific fields.
type ExtractedCondition struct {
	events.Condition

	// Confidence is the extraction confidence for this entity (0.0-1.0).
	Confidence float64 `json:"confidence"`

	// Negated indicates if the condition was negated in the text.
	Negated bool `json:"negated,omitempty"`

	// TextSpan is the original text that mentioned this condition.
	TextSpan string `json:"text_span,omitempty"`

	// SourceContext provides surrounding context from the text.
	SourceContext string `json:"source_context,omitempty"`
}

// ExtractedMedication extends Medication with extraction-specific fields.
type ExtractedMedication struct {
	events.Medication

	// Confidence is the extraction confidence for this entity (0.0-1.0).
	Confidence float64 `json:"confidence"`

	// Negated indicates if the medication was negated in the text.
	Negated bool `json:"negated,omitempty"`

	// TextSpan is the original text that mentioned this medication.
	TextSpan string `json:"text_span,omitempty"`

	// SourceContext provides surrounding context from the text.
	SourceContext string `json:"source_context,omitempty"`
}

// ExtractedVitalSign extends VitalSign with extraction-specific fields.
type ExtractedVitalSign struct {
	events.VitalSign

	// Confidence is the extraction confidence for this entity (0.0-1.0).
	Confidence float64 `json:"confidence"`

	// TextSpan is the original text that mentioned this vital sign.
	TextSpan string `json:"text_span,omitempty"`
}

// ExtractedProcedure extends Procedure with extraction-specific fields.
type ExtractedProcedure struct {
	events.Procedure

	// Confidence is the extraction confidence for this entity (0.0-1.0).
	Confidence float64 `json:"confidence"`

	// Negated indicates if the procedure was negated (not performed).
	Negated bool `json:"negated,omitempty"`

	// TextSpan is the original text that mentioned this procedure.
	TextSpan string `json:"text_span,omitempty"`
}

// ExtractedAllergy extends AllergyIntolerance with extraction-specific fields.
type ExtractedAllergy struct {
	events.AllergyIntolerance

	// Confidence is the extraction confidence for this entity (0.0-1.0).
	Confidence float64 `json:"confidence"`

	// Negated indicates if the allergy was negated (no known allergy).
	Negated bool `json:"negated,omitempty"`

	// TextSpan is the original text that mentioned this allergy.
	TextSpan string `json:"text_span,omitempty"`
}

// ExtractionOptions configures the extraction behavior.
type ExtractionOptions struct {
	// DocumentType specifies the type of clinical document.
	// Examples: "progress_note", "discharge_summary", "consult_note", "radiology_report"
	DocumentType string `json:"document_type,omitempty"`

	// PatientAge provides patient age for context.
	PatientAge int `json:"patient_age,omitempty"`

	// PatientGender provides patient gender for context.
	PatientGender string `json:"patient_gender,omitempty"`

	// ExtractConditions enables condition extraction.
	ExtractConditions bool `json:"extract_conditions"`

	// ExtractMedications enables medication extraction.
	ExtractMedications bool `json:"extract_medications"`

	// ExtractVitalSigns enables vital sign extraction.
	ExtractVitalSigns bool `json:"extract_vital_signs"`

	// ExtractAllergies enables allergy extraction.
	ExtractAllergies bool `json:"extract_allergies"`

	// ExtractProcedures enables procedure extraction.
	ExtractProcedures bool `json:"extract_procedures"`

	// MinConfidence is the minimum confidence threshold for returned entities.
	MinConfidence float64 `json:"min_confidence,omitempty"`

	// IncludeNegated includes entities that were negated in the text.
	IncludeNegated bool `json:"include_negated,omitempty"`

	// MaxTokens limits the response size.
	MaxTokens int `json:"max_tokens,omitempty"`
}

// DefaultExtractionOptions returns options that extract all entity types.
func DefaultExtractionOptions() ExtractionOptions {
	return ExtractionOptions{
		ExtractConditions:  true,
		ExtractMedications: true,
		ExtractVitalSigns:  true,
		ExtractAllergies:   true,
		ExtractProcedures:  true,
		MinConfidence:      0.7,
		IncludeNegated:     false,
		MaxTokens:          4096,
	}
}

// DocumentTypes contains common clinical document types.
var DocumentTypes = struct {
	ProgressNote     string
	DischargeSummary string
	ConsultNote      string
	OperativeNote    string
	RadiologyReport  string
	PathologyReport  string
	HistoryPhysical  string
	NursingNote      string
	EDNote           string
	TransferSummary  string
}{
	ProgressNote:     "progress_note",
	DischargeSummary: "discharge_summary",
	ConsultNote:      "consult_note",
	OperativeNote:    "operative_note",
	RadiologyReport:  "radiology_report",
	PathologyReport:  "pathology_report",
	HistoryPhysical:  "history_physical",
	NursingNote:      "nursing_note",
	EDNote:           "ed_note",
	TransferSummary:  "transfer_summary",
}
