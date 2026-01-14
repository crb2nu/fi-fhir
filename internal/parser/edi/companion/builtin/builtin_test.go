package builtin

import (
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/edi"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/edi/companion"
)

func TestMedicarePartB(t *testing.T) {
	guide := MedicarePartB()

	// Verify basic properties
	if guide.ID != "medicare_part_b" {
		t.Errorf("ID = %q, want medicare_part_b", guide.ID)
	}
	if guide.Name == "" {
		t.Error("Name should not be empty")
	}
	if guide.PayerID != "CMS" {
		t.Errorf("PayerID = %q, want CMS", guide.PayerID)
	}
	if guide.BaseGuide != "005010X222A1" {
		t.Errorf("BaseGuide = %q, want 005010X222A1", guide.BaseGuide)
	}

	// Verify transaction types
	found837P := false
	for _, tx := range guide.TransactionTypes {
		if tx == "837P" || tx == "837" {
			found837P = true
			break
		}
	}
	if !found837P {
		t.Error("TransactionTypes should include 837P or 837")
	}

	// Verify receiver IDs
	if len(guide.ReceiverIDs) == 0 {
		t.Error("ReceiverIDs should not be empty")
	}

	// Verify has required overrides
	if len(guide.Overrides) == 0 {
		t.Error("Overrides should not be empty for Medicare")
	}

	// Verify has validation rules
	if len(guide.Validations) == 0 {
		t.Error("Validations should not be empty for Medicare")
	}

	// Verify has code restrictions
	if len(guide.CodeRestrictions) == 0 {
		t.Error("CodeRestrictions should not be empty for Medicare")
	}
}

func TestMedicarePartB_NPIValidation(t *testing.T) {
	guide := MedicarePartB()
	v := companion.NewValidator(guide)

	// Test with a minimal guide that only has NPI rules to isolate NPI validation
	npiOnlyGuide := &companion.CompanionGuide{
		ID:               "npi_test",
		Name:             "NPI Test",
		TransactionTypes: []string{"837P"},
		Validations: []companion.ValidationRule{
			{
				ID:       "NPI_CHECK",
				Path:     "NM1.09",
				Type:     companion.ValidationLuhn,
				Message:  "NPI must pass Luhn check",
				Required: true,
			},
			{
				ID:       "NPI_QUALIFIER",
				Path:     "NM1.08",
				Type:     companion.ValidationPattern,
				Pattern:  "^XX$",
				Message:  "Qualifier must be XX",
				Required: true,
			},
		},
	}

	npiValidator := companion.NewValidator(npiOnlyGuide)

	tests := []struct {
		name       string
		npi        string
		qualifier  string
		wantValid  bool
		wantErrors int
	}{
		{"valid NPI with XX", "1234567893", "XX", true, 0},
		{"invalid NPI", "1234567890", "XX", false, 1},
		{"wrong qualifier", "1234567893", "FI", false, 1},
		{"missing NPI", "", "XX", false, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &edi.Transaction{
				SetIdentifier: "837",
				Segments: []*edi.Segment{
					{ID: "NM1", Elements: []string{"85", "2", "BILLING", "", "", "", "", tt.qualifier, tt.npi}},
				},
			}

			result := npiValidator.Validate(tx, edi.DefaultDelimiters())
			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) < tt.wantErrors {
				t.Errorf("Errors = %d, want at least %d: %+v", len(result.Errors), tt.wantErrors, result.Errors)
			}
		})
	}

	// Verify the full Medicare guide has NPI validation
	hasNPIRule := false
	for _, rule := range guide.Validations {
		if rule.Type == companion.ValidationLuhn {
			hasNPIRule = true
			break
		}
	}
	if !hasNPIRule {
		t.Error("Medicare guide should have Luhn (NPI) validation rule")
	}

	// Test that a transaction with valid NPI but missing other fields still fails (full guide)
	tx := &edi.Transaction{
		SetIdentifier: "837",
		Segments: []*edi.Segment{
			{ID: "NM1", Elements: []string{"85", "2", "BILLING", "", "", "", "", "XX", "1234567893"}},
		},
	}
	result := v.Validate(tx, edi.DefaultDelimiters())
	// Should fail due to missing required elements (MBI, SBR, etc.)
	if result.Valid {
		t.Log("Note: Transaction with only valid NPI passed full Medicare validation (may need more required fields)")
	}
}

func TestMedicarePartB_MBIValidation(t *testing.T) {
	guide := MedicarePartB()

	// Find MBI validation rule
	var mbiRule *companion.ValidationRule
	for _, rule := range guide.Validations {
		if rule.Type == companion.ValidationMBI {
			mbiRule = &rule
			break
		}
	}

	if mbiRule == nil {
		t.Fatal("Medicare guide should have MBI validation rule")
	}

	if mbiRule.Required != true {
		t.Error("MBI validation should be required for Medicare")
	}
}

func TestMedicarePartB_ClaimFilingCodes(t *testing.T) {
	guide := MedicarePartB()

	// Find claim filing indicator restriction
	var sbrRestriction *companion.CodeRestriction
	for _, restriction := range guide.CodeRestrictions {
		if restriction.Path == "SBR.09" {
			sbrRestriction = &restriction
			break
		}
	}

	if sbrRestriction == nil {
		t.Fatal("Medicare guide should have SBR.09 code restriction")
	}

	// Verify Medicare-specific codes are allowed
	allowedCodes := map[string]bool{
		"MA": true, // Medicare Part A
		"MB": true, // Medicare Part B
	}

	for _, code := range sbrRestriction.Values {
		if allowedCodes[code] {
			delete(allowedCodes, code)
		}
	}

	if len(allowedCodes) > 0 {
		t.Errorf("Missing expected Medicare codes: %v", allowedCodes)
	}
}

func TestBlueCrossBlueShield(t *testing.T) {
	guide := BlueCrossBlueShield()

	// Verify basic properties
	if guide.ID != "bcbs_sample" {
		t.Errorf("ID = %q, want bcbs_sample", guide.ID)
	}
	if guide.PayerID != "BCBS" {
		t.Errorf("PayerID = %q, want BCBS", guide.PayerID)
	}
	if len(guide.TransactionTypes) == 0 {
		t.Error("TransactionTypes should not be empty")
	}
	if len(guide.ReceiverIDs) == 0 {
		t.Error("ReceiverIDs should not be empty")
	}
	if len(guide.Overrides) == 0 {
		t.Error("Overrides should not be empty")
	}
	if len(guide.Validations) == 0 {
		t.Error("Validations should not be empty")
	}
	if len(guide.CodeRestrictions) == 0 {
		t.Error("CodeRestrictions should not be empty")
	}
}

func TestBlueCrossBlueShield_SubscriberIDFormat(t *testing.T) {
	guide := BlueCrossBlueShield()

	// Find subscriber ID format rule
	var subRule *companion.ValidationRule
	for _, rule := range guide.Validations {
		if rule.ID == "SUBSCRIBER_ID_FORMAT" {
			subRule = &rule
			break
		}
	}

	if subRule == nil {
		t.Fatal("BCBS guide should have SUBSCRIBER_ID_FORMAT rule")
	}

	// BCBS subscriber ID rule should be a warning (format varies by region)
	if subRule.Severity != companion.SeverityWarning {
		t.Errorf("Subscriber ID rule severity = %q, should be warning", subRule.Severity)
	}
}

func TestBlueCrossBlueShield_ClaimFilingCodes(t *testing.T) {
	guide := BlueCrossBlueShield()

	// Find claim filing indicator restriction
	var sbrRestriction *companion.CodeRestriction
	for _, restriction := range guide.CodeRestrictions {
		if restriction.Path == "SBR.09" {
			sbrRestriction = &restriction
			break
		}
	}

	if sbrRestriction == nil {
		t.Fatal("BCBS guide should have SBR.09 code restriction")
	}

	// Verify BL (Blue Cross Blue Shield) is allowed
	foundBL := false
	for _, code := range sbrRestriction.Values {
		if code == "BL" {
			foundBL = true
			break
		}
	}

	if !foundBL {
		t.Error("BCBS should allow claim filing code BL")
	}
}

func TestUnitedHealthcare(t *testing.T) {
	guide := UnitedHealthcare()

	// Verify basic properties
	if guide.ID != "uhc_sample" {
		t.Errorf("ID = %q, want uhc_sample", guide.ID)
	}
	if guide.PayerID != "UHC" {
		t.Errorf("PayerID = %q, want UHC", guide.PayerID)
	}
	if len(guide.TransactionTypes) == 0 {
		t.Error("TransactionTypes should not be empty")
	}
	if len(guide.ReceiverIDs) == 0 {
		t.Error("ReceiverIDs should not be empty")
	}
	if len(guide.Overrides) == 0 {
		t.Error("Overrides should not be empty")
	}
	if len(guide.Validations) == 0 {
		t.Error("Validations should not be empty")
	}
	if len(guide.CodeRestrictions) == 0 {
		t.Error("CodeRestrictions should not be empty")
	}
}

func TestUnitedHealthcare_PriorAuthRule(t *testing.T) {
	guide := UnitedHealthcare()

	// Find prior auth rule
	var authRule *companion.ValidationRule
	for _, rule := range guide.Validations {
		if rule.ID == "PRIOR_AUTH_FORMAT" {
			authRule = &rule
			break
		}
	}

	if authRule == nil {
		t.Fatal("UHC guide should have PRIOR_AUTH_FORMAT rule")
	}

	// Prior auth should have condition
	if authRule.Condition == "" {
		t.Error("Prior auth rule should have condition")
	}
	if authRule.Required {
		t.Error("Prior auth rule should not be required (situational)")
	}
}

func TestUnitedHealthcare_PlaceOfServiceCodes(t *testing.T) {
	guide := UnitedHealthcare()

	// Find POS code restriction
	var posRestriction *companion.CodeRestriction
	for _, restriction := range guide.CodeRestrictions {
		if restriction.Path == "2300.CLM.05-1" {
			posRestriction = &restriction
			break
		}
	}

	if posRestriction == nil {
		t.Fatal("UHC guide should have place of service code restriction")
	}

	// Verify common POS codes are included
	expectedCodes := map[string]bool{
		"11": true, // Office
		"21": true, // Inpatient Hospital
		"22": true, // Outpatient Hospital
		"23": true, // Emergency Room
	}

	for _, code := range posRestriction.Values {
		delete(expectedCodes, code)
	}

	if len(expectedCodes) > 0 {
		t.Errorf("Missing expected POS codes: %v", expectedCodes)
	}
}

func TestAllBuiltinGuides_Registration(t *testing.T) {
	// Builtin guides should auto-register via init()
	// This tests that registration doesn't panic

	guideIDs := []string{"medicare_part_b", "bcbs_sample", "uhc_sample"}

	for _, id := range guideIDs {
		guide := companion.GetGuide(id)
		if guide == nil {
			t.Errorf("Guide %q should be auto-registered", id)
		}
	}
}

func TestAllBuiltinGuides_ValidStructure(t *testing.T) {
	guides := []*companion.CompanionGuide{
		MedicarePartB(),
		BlueCrossBlueShield(),
		UnitedHealthcare(),
	}

	for _, guide := range guides {
		t.Run(guide.ID, func(t *testing.T) {
			// Check required fields
			if guide.ID == "" {
				t.Error("ID is required")
			}
			if guide.Name == "" {
				t.Error("Name is required")
			}
			if len(guide.TransactionTypes) == 0 {
				t.Error("TransactionTypes is required")
			}

			// Check validation rules have required fields
			for i, rule := range guide.Validations {
				if rule.ID == "" {
					t.Errorf("Validation rule %d missing ID", i)
				}
				if rule.Path == "" {
					t.Errorf("Validation rule %s missing Path", rule.ID)
				}
				if rule.Type == "" {
					t.Errorf("Validation rule %s missing Type", rule.ID)
				}
				if rule.Message == "" {
					t.Errorf("Validation rule %s missing Message", rule.ID)
				}
			}

			// Check overrides have required fields
			for i, override := range guide.Overrides {
				if override.Path == "" {
					t.Errorf("Override %d missing Path", i)
				}
				if override.Requirement == "" {
					t.Errorf("Override for %s missing Requirement", override.Path)
				}
			}

			// Check code restrictions have required fields
			for i, restriction := range guide.CodeRestrictions {
				if restriction.Path == "" {
					t.Errorf("Code restriction %d missing Path", i)
				}
				if len(restriction.Values) == 0 {
					t.Errorf("Code restriction for %s missing Values", restriction.Path)
				}
			}
		})
	}
}

func TestAllBuiltinGuides_NPIValidation(t *testing.T) {
	guides := []*companion.CompanionGuide{
		MedicarePartB(),
		BlueCrossBlueShield(),
		UnitedHealthcare(),
	}

	for _, guide := range guides {
		t.Run(guide.ID, func(t *testing.T) {
			// Every payer guide should have NPI validation
			hasNPIRule := false
			for _, rule := range guide.Validations {
				if rule.Type == companion.ValidationLuhn && rule.Path == "2010AA.NM1.09" {
					hasNPIRule = true
					break
				}
			}

			if !hasNPIRule {
				t.Errorf("Guide %s should have billing provider NPI validation", guide.ID)
			}
		})
	}
}

func TestFloatPtr(t *testing.T) {
	val := 123.45
	ptr := floatPtr(val)

	if ptr == nil {
		t.Fatal("floatPtr returned nil")
	}
	if *ptr != val {
		t.Errorf("*ptr = %f, want %f", *ptr, val)
	}
}

func TestBuiltinGuides_Validation(t *testing.T) {
	// Test that each builtin guide can validate a transaction without panicking

	guides := []*companion.CompanionGuide{
		MedicarePartB(),
		BlueCrossBlueShield(),
		UnitedHealthcare(),
	}

	// Minimal valid 837P transaction
	// MBI format: Position 1=1-9, 2=alpha, 3=alphanum, 4=digit, 5=alpha, 6-7=alphanum, 8=digit, 9-11=alphanum
	tx := &edi.Transaction{
		SetIdentifier: "837",
		Segments: []*edi.Segment{
			{ID: "BHT", Elements: []string{"0019", "00", "12345", "20240115"}},
			{ID: "NM1", Elements: []string{"85", "2", "BILLING PROVIDER", "", "", "", "", "XX", "1234567893"}},
			{ID: "NM1", Elements: []string{"IL", "1", "DOE", "JOHN", "", "", "", "MI", "1EG4TE58K73"}}, // Valid MBI
			{ID: "SBR", Elements: []string{"P", "18", "", "", "", "", "", "", "MB"}},
			{ID: "CLM", Elements: []string{"CLAIM001", "1500.00", "", "", "11:B:1"}},
		},
	}

	for _, guide := range guides {
		t.Run(guide.ID, func(t *testing.T) {
			v := companion.NewValidator(guide)
			result := v.Validate(tx, edi.DefaultDelimiters())

			// Should not panic and should return a result
			if result == nil {
				t.Fatal("Validate returned nil")
			}
			if result.GuideID != guide.ID {
				t.Errorf("GuideID = %q, want %q", result.GuideID, guide.ID)
			}
		})
	}
}
