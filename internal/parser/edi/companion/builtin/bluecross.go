package builtin

import "github.com/crb2nu/fi-fhir/internal/parser/edi/companion"

// BlueCrossBlueShield returns a companion guide for BCBS 837P claims.
// This is a sample guide demonstrating common BCBS variations.
// Note: Actual BCBS requirements vary by state/region.
func BlueCrossBlueShield() *companion.CompanionGuide {
	return &companion.CompanionGuide{
		ID:               "bcbs_sample",
		Name:             "Blue Cross Blue Shield (Sample)",
		PayerID:          "BCBS",
		BaseGuide:        "005010X222A1",
		TransactionTypes: []string{"837P", "837"},
		Description:      "Sample Blue Cross Blue Shield companion guide - actual requirements vary by region",
		Version:          "1.0.0",
		ReceiverIDs: []string{
			// Common BCBS prefixes (examples)
			"00060", "00590", "36273",
		},

		// Required element overrides
		Overrides: []companion.ElementOverride{
			// Billing Provider NPI required
			{
				Path:        "2010AA.NM1.09",
				Requirement: companion.RequirementRequired,
				Note:        "NPI required for BCBS billing provider",
			},
			// Tax ID required for BCBS
			{
				Path:        "2010AA.REF.02",
				Requirement: companion.RequirementRequired,
				Note:        "Tax ID (EI reference) required for BCBS",
				Condition:   "2010AA.REF.01=EI",
			},
			// Subscriber ID required
			{
				Path:        "2010BA.NM1.09",
				Requirement: companion.RequirementRequired,
				Note:        "BCBS subscriber ID required",
			},
			// Group number often required
			{
				Path:        "SBR.03",
				Requirement: companion.RequirementSituational,
				Note:        "Group number required for most BCBS commercial plans",
			},
		},

		// Validation rules
		Validations: []companion.ValidationRule{
			// Billing provider NPI
			{
				ID:       "BILLING_NPI_FORMAT",
				Path:     "2010AA.NM1.09",
				Type:     companion.ValidationLuhn,
				Message:  "Billing provider NPI must be valid (Luhn check)",
				Required: true,
				Severity: companion.SeverityError,
			},
			// NPI qualifier
			{
				ID:       "BILLING_NPI_QUALIFIER",
				Path:     "2010AA.NM1.08",
				Type:     companion.ValidationPattern,
				Pattern:  "^XX$",
				Message:  "Billing provider ID qualifier must be XX (NPI)",
				Required: true,
				Severity: companion.SeverityError,
			},
			// BCBS member ID format (commonly 3 letters + 9 digits)
			{
				ID:       "SUBSCRIBER_ID_FORMAT",
				Path:     "2010BA.NM1.09",
				Type:     companion.ValidationPattern,
				Pattern:  "^[A-Z]{3}\\d{9}$",
				Message:  "BCBS subscriber ID should be 3 letters followed by 9 digits",
				Required: true,
				Severity: companion.SeverityWarning, // Warning because format varies
			},
			// Tax ID format (EIN)
			{
				ID:        "TAX_ID_FORMAT",
				Path:      "2010AA.REF.02",
				Type:      companion.ValidationPattern,
				Pattern:   "^\\d{9}$",
				Message:   "Tax ID must be 9 digits",
				Required:  false,
				Severity:  companion.SeverityError,
				Condition: "2010AA.REF.01=EI",
			},
			// Group number length
			{
				ID:        "GROUP_NUMBER_LENGTH",
				Path:      "SBR.03",
				Type:      companion.ValidationLength,
				MinLength: 1,
				MaxLength: 17,
				Message:   "Group number must be 1-17 characters",
				Required:  false,
				Severity:  companion.SeverityWarning,
			},
			// Claim amount
			{
				ID:       "CLAIM_AMOUNT_POSITIVE",
				Path:     "CLM.02",
				Type:     companion.ValidationRange,
				MinValue: floatPtr(0.01),
				Message:  "Claim amount must be positive",
				Required: true,
				Severity: companion.SeverityError,
			},
		},

		// Code restrictions
		CodeRestrictions: []companion.CodeRestriction{
			// Claim filing indicator
			{
				Path: "SBR.09",
				Values: []string{
					"BL", // Blue Cross Blue Shield
					"CI", // Commercial Insurance
					"HM", // Health Maintenance Organization
					"OF", // Other Federal Program
				},
				Message:  "Claim filing indicator should be BL, CI, HM, or OF for BCBS",
				Severity: companion.SeverityError,
			},
			// Billing provider entity type
			{
				Path:    "2010AA.NM1.02",
				Values:  []string{"1", "2"},
				Message: "Entity type qualifier must be 1 (Person) or 2 (Non-Person Entity)",
			},
			// Reference ID qualifier for Tax ID
			{
				Path: "2010AA.REF.01",
				Values: []string{
					"EI", // Employer's Identification Number
					"SY", // Social Security Number
				},
				Message:  "Billing provider reference qualifier must be EI or SY",
				Severity: companion.SeverityError,
			},
			// Subscriber relationship
			{
				Path: "SBR.02",
				Values: []string{
					"18", // Self
					"01", // Spouse
					"19", // Child
					"20", // Employee
					"21", // Unknown
					"34", // Other Adult
					"G8", // Other Relationship
				},
				Message: "Invalid subscriber relationship code",
			},
		},
	}
}

// init registers the BCBS guide with the default registry.
func init() {
	guide := BlueCrossBlueShield()
	_ = companion.RegisterGuide(guide)
}
