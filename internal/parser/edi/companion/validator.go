package companion

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/edi"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/validate"
)

// Validator validates EDI transactions against a companion guide.
type Validator struct {
	guide    *CompanionGuide
	npiVal   *validate.NPIValidator
	mbiVal   *validate.MBIValidator
	patterns map[string]*regexp.Regexp // Cache compiled patterns
}

// NewValidator creates a new validator for a companion guide.
func NewValidator(guide *CompanionGuide) *Validator {
	return &Validator{
		guide:    guide,
		npiVal:   validate.NewNPIValidator(),
		mbiVal:   validate.NewMBIValidator(),
		patterns: make(map[string]*regexp.Regexp),
	}
}

// Validate validates a transaction against the companion guide.
func (v *Validator) Validate(tx *edi.Transaction, delimiters edi.Delimiters) *ValidationResult {
	result := NewValidationResult(v.guide.ID, tx.SetIdentifier)

	// Determine loop structure for path resolution
	loopStruct := v.parseLoopStructure(tx)
	resolver := NewPathResolverWithLoops(tx, delimiters, loopStruct)

	// Validate overrides (required elements)
	v.validateOverrides(resolver, result)

	// Validate custom validation rules
	v.validateRules(resolver, result)

	// Validate code restrictions
	v.validateCodeRestrictions(resolver, result)

	return result
}

// ValidateWithResolver validates using a pre-created path resolver.
// This is useful when you need more control over path resolution.
func (v *Validator) ValidateWithResolver(resolver *PathResolver, transactionType string) *ValidationResult {
	result := NewValidationResult(v.guide.ID, transactionType)

	// Validate overrides (required elements)
	v.validateOverrides(resolver, result)

	// Validate custom validation rules
	v.validateRules(resolver, result)

	// Validate code restrictions
	v.validateCodeRestrictions(resolver, result)

	return result
}

// parseLoopStructure parses the transaction into a loop structure for path resolution.
func (v *Validator) parseLoopStructure(tx *edi.Transaction) interface{} {
	switch tx.SetIdentifier {
	case "837":
		// Determine if 837P, 837I, or 837D based on content
		return edi.Parse837Loops(tx)
	case "835":
		return edi.Parse835Loops(tx)
	case "270":
		return edi.Parse270Loops(tx)
	case "271":
		return edi.Parse271Loops(tx)
	case "276":
		return edi.Parse276Loops(tx)
	case "277":
		return edi.Parse277Loops(tx)
	default:
		return nil
	}
}

// validateOverrides checks that required elements are present.
func (v *Validator) validateOverrides(resolver *PathResolver, result *ValidationResult) {
	for _, override := range v.guide.Overrides {
		// Skip if condition is specified and not met
		if override.Condition != "" && !v.evaluateCondition(override.Condition, resolver) {
			continue
		}

		switch override.Requirement {
		case RequirementRequired:
			if !resolver.Exists(override.Path) {
				result.AddError(ValidationIssue{
					Code:    "MISSING_REQUIRED",
					Message: fmt.Sprintf("Required element missing: %s", override.Path),
					Path:    override.Path,
					RuleID:  fmt.Sprintf("override_%s", override.Path),
				})
			}
		case RequirementNotUsed:
			if resolver.Exists(override.Path) {
				result.AddWarning(ValidationIssue{
					Code:    "ELEMENT_NOT_USED",
					Message: fmt.Sprintf("Element should not be used: %s", override.Path),
					Path:    override.Path,
					Value:   resolver.Resolve(override.Path),
					RuleID:  fmt.Sprintf("override_%s", override.Path),
				})
			}
		}
		// RequirementOptional and RequirementSituational don't need validation
	}
}

// validateRules applies custom validation rules.
func (v *Validator) validateRules(resolver *PathResolver, result *ValidationResult) {
	for _, rule := range v.guide.Validations {
		// Skip if condition is specified and not met
		if rule.Condition != "" && !v.evaluateCondition(rule.Condition, resolver) {
			continue
		}

		// Get the value(s) at the path
		values := resolver.ResolveAll(rule.Path)

		// Handle required flag
		if rule.Required && len(values) == 0 {
			addIssue(result, rule, ValidationIssue{
				Code:    "MISSING_REQUIRED",
				Message: rule.Message,
				Path:    rule.Path,
				RuleID:  rule.ID,
			})
			continue
		}

		// Skip validation if no values
		if len(values) == 0 {
			continue
		}

		// Validate each value
		for _, value := range values {
			if value == "" {
				continue
			}

			issue := v.validateValue(value, rule)
			if issue != nil {
				addIssue(result, rule, *issue)
			}
		}
	}
}

// validateValue validates a single value against a rule.
func (v *Validator) validateValue(value string, rule ValidationRule) *ValidationIssue {
	switch rule.Type {
	case ValidationPattern:
		return v.validatePattern(value, rule)
	case ValidationLuhn:
		return v.validateLuhn(value, rule)
	case ValidationMBI:
		return v.validateMBI(value, rule)
	case ValidationLength:
		return v.validateLength(value, rule)
	case ValidationRange:
		return v.validateRange(value, rule)
	case ValidationDate:
		return v.validateDate(value, rule)
	default:
		return nil
	}
}

// validatePattern validates against a regex pattern.
func (v *Validator) validatePattern(value string, rule ValidationRule) *ValidationIssue {
	if rule.Pattern == "" {
		return nil
	}

	pattern := v.getCompiledPattern(rule.Pattern)
	if pattern == nil {
		return &ValidationIssue{
			Code:    "INVALID_PATTERN",
			Message: fmt.Sprintf("Invalid regex pattern: %s", rule.Pattern),
			Path:    rule.Path,
			Value:   value,
			RuleID:  rule.ID,
		}
	}

	if !pattern.MatchString(value) {
		return &ValidationIssue{
			Code:    "PATTERN_MISMATCH",
			Message: rule.Message,
			Path:    rule.Path,
			Value:   value,
			RuleID:  rule.ID,
		}
	}
	return nil
}

// validateLuhn validates using the Luhn algorithm (NPI).
func (v *Validator) validateLuhn(value string, rule ValidationRule) *ValidationIssue {
	result := v.npiVal.Validate(value)
	if !result.Valid {
		return &ValidationIssue{
			Code:    result.Code,
			Message: rule.Message,
			Path:    rule.Path,
			Value:   value,
			RuleID:  rule.ID,
		}
	}
	return nil
}

// validateMBI validates Medicare Beneficiary Identifier format.
func (v *Validator) validateMBI(value string, rule ValidationRule) *ValidationIssue {
	result := v.mbiVal.Validate(value)
	if !result.Valid {
		return &ValidationIssue{
			Code:    result.Code,
			Message: rule.Message,
			Path:    rule.Path,
			Value:   value,
			RuleID:  rule.ID,
		}
	}
	return nil
}

// validateLength validates string length.
func (v *Validator) validateLength(value string, rule ValidationRule) *ValidationIssue {
	length := len(value)

	if rule.MinLength > 0 && length < rule.MinLength {
		return &ValidationIssue{
			Code:    "LENGTH_TOO_SHORT",
			Message: rule.Message,
			Path:    rule.Path,
			Value:   value,
			RuleID:  rule.ID,
		}
	}

	if rule.MaxLength > 0 && length > rule.MaxLength {
		return &ValidationIssue{
			Code:    "LENGTH_TOO_LONG",
			Message: rule.Message,
			Path:    rule.Path,
			Value:   value,
			RuleID:  rule.ID,
		}
	}

	return nil
}

// validateRange validates numeric range.
func (v *Validator) validateRange(value string, rule ValidationRule) *ValidationIssue {
	num, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return &ValidationIssue{
			Code:    "NOT_NUMERIC",
			Message: rule.Message,
			Path:    rule.Path,
			Value:   value,
			RuleID:  rule.ID,
		}
	}

	if rule.MinValue != nil && num < *rule.MinValue {
		return &ValidationIssue{
			Code:    "VALUE_TOO_LOW",
			Message: rule.Message,
			Path:    rule.Path,
			Value:   value,
			RuleID:  rule.ID,
		}
	}

	if rule.MaxValue != nil && num > *rule.MaxValue {
		return &ValidationIssue{
			Code:    "VALUE_TOO_HIGH",
			Message: rule.Message,
			Path:    rule.Path,
			Value:   value,
			RuleID:  rule.ID,
		}
	}

	return nil
}

// validateDate validates date format.
func (v *Validator) validateDate(value string, rule ValidationRule) *ValidationIssue {
	format := rule.DateFormat
	if format == "" {
		// Default X12 date formats
		switch len(value) {
		case 8:
			format = "20060102" // CCYYMMDD
		case 6:
			format = "060102" // YYMMDD
		default:
			return &ValidationIssue{
				Code:    "INVALID_DATE_LENGTH",
				Message: rule.Message,
				Path:    rule.Path,
				Value:   value,
				RuleID:  rule.ID,
			}
		}
	}

	_, err := time.Parse(format, value)
	if err != nil {
		return &ValidationIssue{
			Code:    "INVALID_DATE",
			Message: rule.Message,
			Path:    rule.Path,
			Value:   value,
			RuleID:  rule.ID,
		}
	}

	return nil
}

// validateCodeRestrictions checks that coded values are within allowed sets.
func (v *Validator) validateCodeRestrictions(resolver *PathResolver, result *ValidationResult) {
	for _, restriction := range v.guide.CodeRestrictions {
		// Skip if condition is specified and not met
		if restriction.Condition != "" && !v.evaluateCondition(restriction.Condition, resolver) {
			continue
		}

		// Get all values at the path
		values := resolver.ResolveAll(restriction.Path)
		if len(values) == 0 {
			continue
		}

		// Check each value
		allowedSet := make(map[string]bool)
		for _, allowed := range restriction.Values {
			allowedSet[allowed] = true
		}

		for _, value := range values {
			if value == "" {
				continue
			}
			if !allowedSet[value] {
				issue := ValidationIssue{
					Code:    "INVALID_CODE",
					Message: restriction.Message,
					Path:    restriction.Path,
					Value:   value,
					RuleID:  fmt.Sprintf("code_%s", restriction.Path),
				}
				if restriction.Message == "" {
					issue.Message = fmt.Sprintf("Invalid code '%s' at %s. Allowed values: %s",
						value, restriction.Path, strings.Join(restriction.Values, ", "))
				}

				switch restriction.Severity {
				case SeverityWarning:
					result.AddWarning(issue)
				case SeverityInfo:
					result.AddInfo(issue)
				default:
					result.AddError(issue)
				}
			}
		}
	}
}

// evaluateCondition evaluates a simple condition expression.
// Supports basic expressions like "path=value" or "path!=value".
func (v *Validator) evaluateCondition(condition string, resolver *PathResolver) bool {
	// Simple condition parsing: "path=value" or "path!=value"
	if strings.Contains(condition, "!=") {
		parts := strings.SplitN(condition, "!=", 2)
		if len(parts) == 2 {
			path := strings.TrimSpace(parts[0])
			expected := strings.TrimSpace(parts[1])
			actual := resolver.Resolve(path)
			return actual != expected
		}
	} else if strings.Contains(condition, "=") {
		parts := strings.SplitN(condition, "=", 2)
		if len(parts) == 2 {
			path := strings.TrimSpace(parts[0])
			expected := strings.TrimSpace(parts[1])
			actual := resolver.Resolve(path)
			return actual == expected
		}
	} else if strings.HasPrefix(condition, "exists(") && strings.HasSuffix(condition, ")") {
		path := condition[7 : len(condition)-1]
		return resolver.Exists(path)
	} else if strings.HasPrefix(condition, "!exists(") && strings.HasSuffix(condition, ")") {
		path := condition[8 : len(condition)-1]
		return !resolver.Exists(path)
	}

	// Default to true if condition can't be parsed
	return true
}

// getCompiledPattern returns a compiled regex pattern, caching for reuse.
func (v *Validator) getCompiledPattern(pattern string) *regexp.Regexp {
	if cached, ok := v.patterns[pattern]; ok {
		return cached
	}

	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}

	v.patterns[pattern] = compiled
	return compiled
}

// addIssue adds an issue to the result based on the rule's severity.
func addIssue(result *ValidationResult, rule ValidationRule, issue ValidationIssue) {
	switch rule.Severity {
	case SeverityWarning:
		result.AddWarning(issue)
	case SeverityInfo:
		result.AddInfo(issue)
	default:
		result.AddError(issue)
	}
}
