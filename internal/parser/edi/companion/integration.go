package companion

import (
	"github.com/crb2nu/fi-fhir/internal/parser/edi"
)

// ValidateParseResult validates a parse result against a companion guide.
func ValidateParseResult(result *edi.ParseResult, guide *CompanionGuide) *ValidationResult {
	if result.Interchange == nil || len(result.Interchange.FunctionalGroups) == 0 {
		return NewValidationResult(guide.ID, "")
	}

	validator := NewValidator(guide)
	delimiters := result.Interchange.Delimiters

	// Validate the first transaction (most common case)
	// For multiple transactions, the caller should iterate
	for _, group := range result.Interchange.FunctionalGroups {
		for _, tx := range group.Transactions {
			return validator.Validate(tx, delimiters)
		}
	}

	return NewValidationResult(guide.ID, "")
}

// ValidateTransaction validates a single transaction against a companion guide.
func ValidateTransaction(tx *edi.Transaction, delimiters edi.Delimiters, guide *CompanionGuide) *ValidationResult {
	validator := NewValidator(guide)
	return validator.Validate(tx, delimiters)
}

// ValidateAllTransactions validates all transactions in a parse result.
func ValidateAllTransactions(result *edi.ParseResult, guide *CompanionGuide) []*ValidationResult {
	if result.Interchange == nil {
		return nil
	}

	var results []*ValidationResult
	validator := NewValidator(guide)
	delimiters := result.Interchange.Delimiters

	for _, group := range result.Interchange.FunctionalGroups {
		for _, tx := range group.Transactions {
			validationResult := validator.Validate(tx, delimiters)
			results = append(results, validationResult)
		}
	}

	return results
}

// DetectAndValidate parses and validates EDI content using the registry.
// It auto-detects the appropriate companion guide based on receiver ID and transaction type.
func DetectAndValidate(result *edi.ParseResult, registry *Registry) *ValidationResult {
	if result.Interchange == nil {
		return nil
	}

	guide := DetectFromParseResult(result, registry)
	if guide == nil {
		return nil
	}

	return ValidateParseResult(result, guide)
}

// DetectFromParseResult attempts to auto-detect the appropriate companion guide.
// It uses the receiver ID from ISA08 and transaction type from ST01.
func DetectFromParseResult(result *edi.ParseResult, registry *Registry) *CompanionGuide {
	if result.Interchange == nil {
		return nil
	}

	receiverID := result.Interchange.ReceiverID
	payerID := ""
	transactionType := ""
	baseGuide := ""

	// Get transaction type from first transaction
	for _, group := range result.Interchange.FunctionalGroups {
		for _, tx := range group.Transactions {
			transactionType = tx.SetIdentifier
			if tx.ImplementationRef != "" {
				baseGuide = tx.ImplementationRef
			} else if group.VersionCode != "" {
				baseGuide = group.VersionCode
			}

			// Try to find payer ID in the transaction for 837
			if tx.SetIdentifier == "837" {
				loops := edi.Parse837Loops(tx)
				if loops != nil {
					// Look for payer in subscriber's payer loop
					for _, bp := range loops.BillingProviders {
						for _, sub := range bp.Subscribers {
							if sub.PayerInfo != nil && sub.PayerInfo.NM1 != nil {
								// NM1*PR segment has payer info, get identifier from element 9
								payerID = sub.PayerInfo.NM1.GetElement(9)
								break
							}
						}
						if payerID != "" {
							break
						}
					}
				}
			}
			break
		}
		if transactionType != "" {
			break
		}
	}

	return registry.Detect(receiverID, payerID, transactionType, baseGuide)
}

// ParseResultInfo extracts information useful for guide detection.
type ParseResultInfo struct {
	ReceiverID      string
	ReceiverQual    string
	SenderID        string
	SenderQual      string
	TransactionType string
	VersionCode     string
}

// GetParseResultInfo extracts guide-relevant information from a parse result.
func GetParseResultInfo(result *edi.ParseResult) ParseResultInfo {
	info := ParseResultInfo{}

	if result.Interchange == nil {
		return info
	}

	info.ReceiverID = result.Interchange.ReceiverID
	info.ReceiverQual = result.Interchange.ReceiverQualifier
	info.SenderID = result.Interchange.SenderID
	info.SenderQual = result.Interchange.SenderQualifier

	// Get first functional group and first transaction
	if len(result.Interchange.FunctionalGroups) > 0 {
		group := result.Interchange.FunctionalGroups[0]
		info.VersionCode = group.VersionCode
		if len(group.Transactions) > 0 {
			info.TransactionType = group.Transactions[0].SetIdentifier
		}
	}

	return info
}

// MustValidate validates and panics if there are errors.
// Useful for testing and scenarios where validation errors are unexpected.
func MustValidate(result *edi.ParseResult, guide *CompanionGuide) *ValidationResult {
	validationResult := ValidateParseResult(result, guide)
	if !validationResult.Valid {
		panic("validation failed: " + validationResult.Errors[0].Message)
	}
	return validationResult
}
