// Package companion provides payer-specific EDI X12 validation using companion guides.
// Companion guides define variations on the standard X12 format required by specific
// payers (Medicare, Blue Cross, United Healthcare, etc.).
package companion

// RequirementLevel indicates whether an element is required, optional, or situational.
type RequirementLevel string

const (
	// RequirementRequired indicates the element must be present.
	RequirementRequired RequirementLevel = "required"
	// RequirementOptional indicates the element may be present.
	RequirementOptional RequirementLevel = "optional"
	// RequirementSituational indicates the element is required in certain situations.
	RequirementSituational RequirementLevel = "situational"
	// RequirementNotUsed indicates the element should not be used.
	RequirementNotUsed RequirementLevel = "not_used"
)

// ValidationType specifies the type of validation to perform.
type ValidationType string

const (
	// ValidationPattern validates against a regex pattern.
	ValidationPattern ValidationType = "pattern"
	// ValidationLuhn validates using the Luhn algorithm (NPI).
	ValidationLuhn ValidationType = "luhn"
	// ValidationMBI validates Medicare Beneficiary Identifier format.
	ValidationMBI ValidationType = "mbi"
	// ValidationLength validates string length (min/max).
	ValidationLength ValidationType = "length"
	// ValidationRange validates numeric range.
	ValidationRange ValidationType = "range"
	// ValidationDate validates date format.
	ValidationDate ValidationType = "date"
	// ValidationCustom uses a custom validation function.
	ValidationCustom ValidationType = "custom"
)

// IssueSeverity indicates the severity of a validation issue.
type IssueSeverity string

const (
	// SeverityError indicates a fatal error that must be fixed.
	SeverityError IssueSeverity = "error"
	// SeverityWarning indicates a potential issue that should be reviewed.
	SeverityWarning IssueSeverity = "warning"
	// SeverityInfo provides informational notes.
	SeverityInfo IssueSeverity = "info"
)

// CompanionGuide defines payer-specific validation rules for X12 transactions.
type CompanionGuide struct {
	// ID is a unique identifier for this guide (e.g., "medicare_part_b").
	ID string `json:"id" yaml:"id"`

	// Name is a human-readable name for this guide.
	Name string `json:"name" yaml:"name"`

	// PayerID identifies the payer (e.g., "CMS", "BCBS", "UHC").
	PayerID string `json:"payer_id" yaml:"payer_id"`

	// ReceiverIDs are X12 receiver IDs that use this guide (for auto-detection).
	ReceiverIDs []string `json:"receiver_ids,omitempty" yaml:"receiver_ids,omitempty"`

	// BaseGuide is the X12 implementation guide version (e.g., "005010X222A1").
	BaseGuide string `json:"base_guide" yaml:"base_guide"`

	// TransactionTypes lists supported transaction types (e.g., ["837P", "837I"]).
	TransactionTypes []string `json:"transaction_types" yaml:"transaction_types"`

	// Description provides documentation about this companion guide.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Version is the version of this companion guide configuration.
	Version string `json:"version,omitempty" yaml:"version,omitempty"`

	// Overrides modify the requirement level of standard elements.
	Overrides []ElementOverride `json:"overrides,omitempty" yaml:"overrides,omitempty"`

	// Validations define custom validation rules.
	Validations []ValidationRule `json:"validations,omitempty" yaml:"validations,omitempty"`

	// CodeRestrictions limit allowed code values.
	CodeRestrictions []CodeRestriction `json:"code_restrictions,omitempty" yaml:"code_restrictions,omitempty"`

	// SegmentRequirements modify segment-level requirements.
	SegmentRequirements []SegmentRequirement `json:"segment_requirements,omitempty" yaml:"segment_requirements,omitempty"`
}

// ElementOverride changes the requirement level of a standard element.
type ElementOverride struct {
	// Path identifies the element using X12 dot notation (e.g., "2010AA.NM1.09").
	Path string `json:"path" yaml:"path"`

	// Requirement is the new requirement level.
	Requirement RequirementLevel `json:"requirement" yaml:"requirement"`

	// Note provides documentation about why this override exists.
	Note string `json:"note,omitempty" yaml:"note,omitempty"`

	// Condition specifies when this override applies (optional).
	Condition string `json:"condition,omitempty" yaml:"condition,omitempty"`
}

// ValidationRule defines a custom validation rule.
type ValidationRule struct {
	// ID is a unique identifier for this rule (e.g., "NPI_FORMAT").
	ID string `json:"id" yaml:"id"`

	// Path identifies the element using X12 dot notation.
	Path string `json:"path" yaml:"path"`

	// Type specifies the validation type.
	Type ValidationType `json:"type" yaml:"type"`

	// Pattern is a regex pattern (for ValidationType pattern).
	Pattern string `json:"pattern,omitempty" yaml:"pattern,omitempty"`

	// MinLength is the minimum string length (for ValidationType length).
	MinLength int `json:"min_length,omitempty" yaml:"min_length,omitempty"`

	// MaxLength is the maximum string length (for ValidationType length).
	MaxLength int `json:"max_length,omitempty" yaml:"max_length,omitempty"`

	// MinValue is the minimum numeric value (for ValidationType range).
	MinValue *float64 `json:"min_value,omitempty" yaml:"min_value,omitempty"`

	// MaxValue is the maximum numeric value (for ValidationType range).
	MaxValue *float64 `json:"max_value,omitempty" yaml:"max_value,omitempty"`

	// DateFormat is the expected date format (for ValidationType date).
	DateFormat string `json:"date_format,omitempty" yaml:"date_format,omitempty"`

	// Message is the error message when validation fails.
	Message string `json:"message" yaml:"message"`

	// Severity indicates the severity level when validation fails.
	Severity IssueSeverity `json:"severity,omitempty" yaml:"severity,omitempty"`

	// Required indicates if the element must be present for this validation.
	Required bool `json:"required,omitempty" yaml:"required,omitempty"`

	// Condition specifies when this validation applies (optional).
	Condition string `json:"condition,omitempty" yaml:"condition,omitempty"`
}

// CodeRestriction limits the allowed values for a coded element.
type CodeRestriction struct {
	// Path identifies the element using X12 dot notation.
	Path string `json:"path" yaml:"path"`

	// Values lists the allowed code values.
	Values []string `json:"values" yaml:"values"`

	// Message is the error message when an invalid code is used.
	Message string `json:"message,omitempty" yaml:"message,omitempty"`

	// Severity indicates the severity level when restriction is violated.
	Severity IssueSeverity `json:"severity,omitempty" yaml:"severity,omitempty"`

	// Condition specifies when this restriction applies (optional).
	Condition string `json:"condition,omitempty" yaml:"condition,omitempty"`
}

// SegmentRequirement modifies segment-level requirements.
type SegmentRequirement struct {
	// Loop is the loop identifier (e.g., "2010AA").
	Loop string `json:"loop" yaml:"loop"`

	// Segment is the segment identifier (e.g., "NM1").
	Segment string `json:"segment" yaml:"segment"`

	// Requirement is the new requirement level.
	Requirement RequirementLevel `json:"requirement" yaml:"requirement"`

	// Note provides documentation about why this requirement exists.
	Note string `json:"note,omitempty" yaml:"note,omitempty"`

	// Condition specifies when this requirement applies (optional).
	Condition string `json:"condition,omitempty" yaml:"condition,omitempty"`
}

// ValidationResult contains the results of validating an EDI transaction.
type ValidationResult struct {
	// Valid indicates if the transaction passed all validations.
	Valid bool `json:"valid"`

	// Errors contains validation errors (severity=error).
	Errors []ValidationIssue `json:"errors,omitempty"`

	// Warnings contains validation warnings (severity=warning).
	Warnings []ValidationIssue `json:"warnings,omitempty"`

	// Info contains informational messages (severity=info).
	Info []ValidationIssue `json:"info,omitempty"`

	// GuideID is the companion guide that was used for validation.
	GuideID string `json:"guide_id"`

	// TransactionType is the type of transaction that was validated.
	TransactionType string `json:"transaction_type"`
}

// ValidationIssue represents a single validation error, warning, or info message.
type ValidationIssue struct {
	// Code is a machine-readable error code (e.g., "INVALID_NPI").
	Code string `json:"code"`

	// Message is a human-readable description of the issue.
	Message string `json:"message"`

	// Path is the X12 path to the element with the issue.
	Path string `json:"path,omitempty"`

	// Value is the actual value that caused the issue.
	Value string `json:"value,omitempty"`

	// Severity indicates the issue severity.
	Severity IssueSeverity `json:"severity"`

	// RuleID is the ID of the validation rule that triggered this issue.
	RuleID string `json:"rule_id,omitempty"`

	// SegmentID is the X12 segment containing the issue (e.g., "NM1").
	SegmentID string `json:"segment_id,omitempty"`

	// SegmentPosition is the position of the segment in the transaction.
	SegmentPosition int `json:"segment_position,omitempty"`
}

// AddError adds an error to the validation result.
func (r *ValidationResult) AddError(issue ValidationIssue) {
	issue.Severity = SeverityError
	r.Errors = append(r.Errors, issue)
	r.Valid = false
}

// AddWarning adds a warning to the validation result.
func (r *ValidationResult) AddWarning(issue ValidationIssue) {
	issue.Severity = SeverityWarning
	r.Warnings = append(r.Warnings, issue)
}

// AddInfo adds an informational message to the validation result.
func (r *ValidationResult) AddInfo(issue ValidationIssue) {
	issue.Severity = SeverityInfo
	r.Info = append(r.Info, issue)
}

// TotalIssues returns the total number of issues (errors + warnings).
func (r *ValidationResult) TotalIssues() int {
	return len(r.Errors) + len(r.Warnings)
}

// NewValidationResult creates a new validation result.
func NewValidationResult(guideID, transactionType string) *ValidationResult {
	return &ValidationResult{
		Valid:           true,
		GuideID:         guideID,
		TransactionType: transactionType,
	}
}
