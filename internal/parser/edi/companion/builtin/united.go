package builtin

import "github.com/crb2nu/fi-fhir/internal/parser/edi/companion"

// UnitedHealthcare returns a companion guide for UHC 837P claims.
// This is a sample guide demonstrating common UHC variations.
func UnitedHealthcare() *companion.CompanionGuide {
	return &companion.CompanionGuide{
		ID:               "uhc_sample",
		Name:             "United Healthcare (Sample)",
		PayerID:          "UHC",
		BaseGuide:        "005010X222A1",
		TransactionTypes: []string{"837P", "837"},
		Description:      "Sample United Healthcare companion guide for 837P professional claims",
		Version:          "1.0.0",
		ReceiverIDs: []string{
			"87726", // Common UHC receiver ID
			"00112", // UHC Community Plan
		},

		// Required element overrides
		Overrides: []companion.ElementOverride{
			// Billing Provider NPI required
			{
				Path:        "2010AA.NM1.09",
				Requirement: companion.RequirementRequired,
				Note:        "NPI required for UHC billing provider",
			},
			// Subscriber ID required
			{
				Path:        "2010BA.NM1.09",
				Requirement: companion.RequirementRequired,
				Note:        "UHC member ID required",
			},
			// Prior authorization when applicable
			{
				Path:        "2300.REF.02",
				Requirement: companion.RequirementSituational,
				Note:        "Prior authorization number required when applicable (REF*G1)",
				Condition:   "2300.REF.01=G1",
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
			// Rendering provider NPI when present
			{
				ID:       "RENDERING_NPI_FORMAT",
				Path:     "2310B.NM1.09",
				Type:     companion.ValidationLuhn,
				Message:  "Rendering provider NPI must be valid (Luhn check)",
				Required: false,
				Severity: companion.SeverityError,
			},
			// UHC member ID format (9-11 alphanumeric characters)
			{
				ID:        "SUBSCRIBER_ID_FORMAT",
				Path:      "2010BA.NM1.09",
				Type:      companion.ValidationLength,
				MinLength: 9,
				MaxLength: 11,
				Message:   "UHC member ID should be 9-11 characters",
				Required:  true,
				Severity:  companion.SeverityWarning,
			},
			// Prior authorization format when present
			{
				ID:        "PRIOR_AUTH_FORMAT",
				Path:      "2300.REF.02",
				Type:      companion.ValidationPattern,
				Pattern:   "^[A-Z0-9]{6,20}$",
				Message:   "Prior authorization number must be 6-20 alphanumeric characters",
				Required:  false,
				Severity:  companion.SeverityError,
				Condition: "2300.REF.01=G1",
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
			// Service date
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
			// Claim filing indicator
			{
				Path: "SBR.09",
				Values: []string{
					"CI", // Commercial Insurance
					"HM", // Health Maintenance Organization
					"MC", // Medicaid
					"16", // HMO Medicare Risk
					"AM", // Auto Medical
					"WC", // Workers' Compensation
				},
				Message:  "Claim filing indicator should be appropriate for UHC plan type",
				Severity: companion.SeverityError,
			},
			// Billing provider entity type
			{
				Path:    "2010AA.NM1.02",
				Values:  []string{"1", "2"},
				Message: "Entity type qualifier must be 1 (Person) or 2 (Non-Person Entity)",
			},
			// Reference ID qualifiers
			{
				Path: "2300.REF.01",
				Values: []string{
					"G1", // Prior Authorization
					"9F", // Referral Number
					"EA", // Medical Record Number
					"D9", // Claim Number
					"F8", // Original Reference Number
					"G3", // Predetermination of Benefits Number
				},
				Message:  "Reference qualifier must be a valid claim-level code",
				Severity: companion.SeverityWarning,
			},
			// Place of service codes (common values)
			{
				Path: "2300.CLM.05-1",
				Values: []string{
					"11", // Office
					"12", // Home
					"21", // Inpatient Hospital
					"22", // Outpatient Hospital
					"23", // Emergency Room - Hospital
					"24", // Ambulatory Surgical Center
					"31", // Skilled Nursing Facility
					"32", // Nursing Facility
					"41", // Ambulance - Land
					"42", // Ambulance - Air or Water
					"49", // Independent Clinic
					"50", // Federally Qualified Health Center
					"81", // Independent Laboratory
					"99", // Other Place of Service
				},
				Message:  "Place of service code must be valid",
				Severity: companion.SeverityWarning,
			},
		},
	}
}

// init registers the UHC guide with the default registry.
func init() {
	guide := UnitedHealthcare()
	_ = companion.RegisterGuide(guide)
}
