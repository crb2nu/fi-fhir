// Package builtin provides pre-configured companion guides for common payers.
package builtin

import "github.com/crb2nu/fi-fhir/internal/parser/edi/companion"

// MedicarePartB returns a companion guide for Medicare Part B 837P claims.
// This guide enforces CMS-specific requirements for professional claims.
func MedicarePartB() *companion.CompanionGuide {
	return &companion.CompanionGuide{
		ID:               "medicare_part_b",
		Name:             "Medicare Part B (Professional)",
		PayerID:          "CMS",
		BaseGuide:        "005010X222A1",
		TransactionTypes: []string{"837P", "837"},
		Description:      "CMS Medicare Part B companion guide for 837P professional claims",
		Version:          "1.0.0",
		ReceiverIDs: []string{
			"CMS", "80840", // Common Medicare receiver IDs
		},

		// Required element overrides
		Overrides: []companion.ElementOverride{
			// Billing Provider NPI is required
			{
				Path:        "2010AA.NM1.09",
				Requirement: companion.RequirementRequired,
				Note:        "NPI required for Medicare billing provider",
			},
			// Billing Provider qualifier must be XX (NPI)
			{
				Path:        "2010AA.NM1.08",
				Requirement: companion.RequirementRequired,
				Note:        "NPI qualifier XX required for Medicare",
			},
			// Subscriber ID (MBI) is required
			{
				Path:        "2010BA.NM1.09",
				Requirement: companion.RequirementRequired,
				Note:        "Medicare Beneficiary Identifier required",
			},
			// Claim filing indicator code required
			{
				Path:        "SBR.09",
				Requirement: companion.RequirementRequired,
				Note:        "Claim filing indicator code required for Medicare",
			},
			// Place of service required
			{
				Path:        "2300.CLM.05-1",
				Requirement: companion.RequirementRequired,
				Note:        "Facility type code required for Medicare claims",
			},
		},

		// Validation rules
		Validations: []companion.ValidationRule{
			// NPI validation using Luhn algorithm
			{
				ID:       "BILLING_NPI_FORMAT",
				Path:     "2010AA.NM1.09",
				Type:     companion.ValidationLuhn,
				Message:  "Billing provider NPI must be valid (Luhn check)",
				Required: true,
				Severity: companion.SeverityError,
			},
			// NPI qualifier must be XX
			{
				ID:       "BILLING_NPI_QUALIFIER",
				Path:     "2010AA.NM1.08",
				Type:     companion.ValidationPattern,
				Pattern:  "^XX$",
				Message:  "Billing provider ID qualifier must be XX (NPI). Legacy identifiers not accepted by Medicare.",
				Required: true,
				Severity: companion.SeverityError,
			},
			// Rendering provider NPI when present
			{
				ID:       "RENDERING_NPI_FORMAT",
				Path:     "2310B.NM1.09",
				Type:     companion.ValidationLuhn,
				Message:  "Rendering provider NPI must be valid (Luhn check)",
				Required: false,
				Severity: companion.SeverityError,
			},
			// MBI format validation
			{
				ID:       "SUBSCRIBER_MBI_FORMAT",
				Path:     "2010BA.NM1.09",
				Type:     companion.ValidationMBI,
				Message:  "Subscriber ID must be a valid Medicare Beneficiary Identifier (MBI)",
				Required: true,
				Severity: companion.SeverityError,
			},
			// Subscriber ID qualifier
			{
				ID:       "SUBSCRIBER_ID_QUALIFIER",
				Path:     "2010BA.NM1.08",
				Type:     companion.ValidationPattern,
				Pattern:  "^MI$",
				Message:  "Subscriber ID qualifier must be MI (Member Identification Number)",
				Required: true,
				Severity: companion.SeverityError,
			},
			// Claim amount validation
			{
				ID:       "CLAIM_AMOUNT_POSITIVE",
				Path:     "CLM.02",
				Type:     companion.ValidationRange,
				MinValue: floatPtr(0.01),
				Message:  "Claim amount must be positive",
				Required: true,
				Severity: companion.SeverityError,
			},
			// Service line charge
			{
				ID:        "SERVICE_CHARGE_POSITIVE",
				Path:      "2400.SV1.02",
				Type:      companion.ValidationRange,
				MinValue:  floatPtr(0.00),
				Message:   "Service line charge must be non-negative",
				Required:  false,
				Severity:  companion.SeverityError,
				Condition: "exists(2400.SV1.01)",
			},
			// Date format validation
			{
				ID:       "SERVICE_DATE_FORMAT",
				Path:     "2400.DTP.03",
				Type:     companion.ValidationDate,
				Message:  "Service date must be valid (CCYYMMDD format)",
				Required: false,
				Severity: companion.SeverityError,
			},
		},

		// Code restrictions
		CodeRestrictions: []companion.CodeRestriction{
			// Claim filing indicator codes for Medicare
			{
				Path: "SBR.09",
				Values: []string{
					"MA", // Medicare Part A
					"MB", // Medicare Part B
					"MC", // Medicaid (when Medicare is secondary)
					"09", // Self-pay (rare)
				},
				Message:  "Claim filing indicator must be MA, MB, MC, or 09 for Medicare claims",
				Severity: companion.SeverityError,
			},
			// Entity identifier code for billing provider
			{
				Path: "2010AA.NM1.01",
				Values: []string{
					"85", // Billing Provider
				},
				Message:  "Billing provider NM1 entity identifier must be 85",
				Severity: companion.SeverityError,
			},
			// Entity type qualifier
			{
				Path: "2010AA.NM1.02",
				Values: []string{
					"1", // Person
					"2", // Non-Person Entity
				},
				Message:  "Entity type qualifier must be 1 (Person) or 2 (Non-Person Entity)",
				Severity: companion.SeverityError,
			},
			// Payer identifier
			{
				Path: "2010BB.NM1.01",
				Values: []string{
					"PR", // Payer
				},
				Message:  "Payer NM1 entity identifier must be PR",
				Severity: companion.SeverityError,
			},
			// Subscriber relationship codes
			{
				Path: "SBR.02",
				Values: []string{
					"18", // Self
					"01", // Spouse
					"19", // Child
					"20", // Employee
					"21", // Unknown
					"39", // Organ Donor
					"40", // Cadaver Donor
					"53", // Life Partner
					"G8", // Other Relationship
				},
				Message:  "Invalid subscriber relationship code for Medicare",
				Severity: companion.SeverityError,
			},
			// Provider taxonomy code qualifier
			{
				Path: "2000A.PRV.01",
				Values: []string{
					"BI", // Billing
					"PE", // Performing
					"RF", // Referring
				},
				Message:  "Provider code qualifier must be BI, PE, or RF",
				Severity: companion.SeverityError,
			},
		},
	}
}

// floatPtr is a helper to create a pointer to a float64.
func floatPtr(f float64) *float64 {
	return &f
}

// init registers the Medicare Part B guide with the default registry.
func init() {
	guide := MedicarePartB()
	// We don't error check here because init() can't return errors
	// In production, you might log this instead
	_ = companion.RegisterGuide(guide)
}
