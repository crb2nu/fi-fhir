package companion

import (
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/edi"
)

// createMinimalParseResult creates a minimal ParseResult for testing.
func createMinimalParseResult(receiverID, transactionType string, segments []*edi.Segment) *edi.ParseResult {
	return &edi.ParseResult{
		Interchange: &edi.Interchange{
			ReceiverID:        receiverID,
			ReceiverQualifier: "ZZ",
			SenderID:          "SENDER123456789",
			SenderQualifier:   "ZZ",
			ControlNumber:     "000000001",
			Delimiters: edi.Delimiters{
				Element:    '*',
				Segment:    '~',
				Subelement: ':',
			},
			FunctionalGroups: []*edi.FunctionalGroup{
				{
					ControlNumber:     "1",
					FunctionalID:      "HC",
					VersionCode:       "005010X222A1",
					ResponsibleAgency: "X",
					Transactions: []*edi.Transaction{
						{
							SetIdentifier: transactionType,
							ControlNumber: "0001",
							Segments:      segments,
						},
					},
				},
			},
		},
	}
}

func TestValidateParseResult(t *testing.T) {
	t.Run("nil interchange", func(t *testing.T) {
		result := &edi.ParseResult{Interchange: nil}
		guide := &CompanionGuide{ID: "test", Name: "Test"}

		validationResult := ValidateParseResult(result, guide)
		if validationResult == nil {
			t.Fatal("expected non-nil result")
		}
		if !validationResult.Valid {
			t.Error("empty result should be valid")
		}
	})

	t.Run("empty functional groups", func(t *testing.T) {
		result := &edi.ParseResult{
			Interchange: &edi.Interchange{
				FunctionalGroups: []*edi.FunctionalGroup{},
			},
		}
		guide := &CompanionGuide{ID: "test", Name: "Test"}

		validationResult := ValidateParseResult(result, guide)
		if validationResult == nil {
			t.Fatal("expected non-nil result")
		}
		if !validationResult.Valid {
			t.Error("empty groups should be valid")
		}
	})

	t.Run("valid transaction", func(t *testing.T) {
		segments := []*edi.Segment{
			{ID: "NM1", Elements: []string{"85", "2", "PROVIDER", "", "", "", "", "XX", "1234567893"}},
			{ID: "CLM", Elements: []string{"CLAIM001", "1500.00"}},
		}
		result := createMinimalParseResult("RECEIVER123456", "837", segments)
		guide := &CompanionGuide{
			ID:               "test",
			Name:             "Test",
			TransactionTypes: []string{"837"},
		}

		validationResult := ValidateParseResult(result, guide)
		if validationResult == nil {
			t.Fatal("expected non-nil result")
		}
		if !validationResult.Valid {
			t.Errorf("expected valid result, got errors: %v", validationResult.Errors)
		}
	})

	t.Run("invalid NPI", func(t *testing.T) {
		segments := []*edi.Segment{
			{ID: "NM1", Elements: []string{"85", "2", "PROVIDER", "", "", "", "", "XX", "1234567890"}}, // Invalid NPI
		}
		result := createMinimalParseResult("RECEIVER123456", "837", segments)
		guide := &CompanionGuide{
			ID:               "test",
			Name:             "Test",
			TransactionTypes: []string{"837"},
			Validations: []ValidationRule{
				{
					ID:      "NPI_CHECK",
					Path:    "NM1.09",
					Type:    ValidationLuhn,
					Message: "NPI must be valid",
				},
			},
		}

		validationResult := ValidateParseResult(result, guide)
		if validationResult == nil {
			t.Fatal("expected non-nil result")
		}
		if validationResult.Valid {
			t.Error("expected invalid result due to bad NPI")
		}
		if len(validationResult.Errors) == 0 {
			t.Error("expected at least one error")
		}
	})
}

func TestValidateTransaction(t *testing.T) {
	t.Run("validate single transaction", func(t *testing.T) {
		tx := &edi.Transaction{
			SetIdentifier: "837",
			ControlNumber: "0001",
			Segments: []*edi.Segment{
				{ID: "CLM", Elements: []string{"CLAIM001", "1500.00", "", "", "11:B:1"}},
			},
		}
		delimiters := edi.Delimiters{
			Element:    '*',
			Segment:    '~',
			Subelement: ':',
		}
		guide := &CompanionGuide{
			ID:               "test",
			Name:             "Test",
			TransactionTypes: []string{"837"},
		}

		result := ValidateTransaction(tx, delimiters, guide)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("with required field missing", func(t *testing.T) {
		tx := &edi.Transaction{
			SetIdentifier: "837",
			ControlNumber: "0001",
			Segments: []*edi.Segment{
				{ID: "CLM", Elements: []string{"CLAIM001"}}, // Missing required element
			},
		}
		delimiters := edi.Delimiters{
			Element:    '*',
			Segment:    '~',
			Subelement: ':',
		}
		guide := &CompanionGuide{
			ID:               "test",
			Name:             "Test",
			TransactionTypes: []string{"837"},
			Overrides: []ElementOverride{
				{Path: "CLM.02", Requirement: RequirementRequired},
			},
		}

		result := ValidateTransaction(tx, delimiters, guide)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if result.Valid {
			t.Error("expected invalid result due to missing required field")
		}
	})
}

func TestValidateAllTransactions(t *testing.T) {
	t.Run("nil interchange", func(t *testing.T) {
		result := &edi.ParseResult{Interchange: nil}
		guide := &CompanionGuide{ID: "test", Name: "Test"}

		results := ValidateAllTransactions(result, guide)
		if results != nil {
			t.Error("expected nil for nil interchange")
		}
	})

	t.Run("multiple transactions", func(t *testing.T) {
		result := &edi.ParseResult{
			Interchange: &edi.Interchange{
				Delimiters: edi.Delimiters{
					Element:    '*',
					Segment:    '~',
					Subelement: ':',
				},
				FunctionalGroups: []*edi.FunctionalGroup{
					{
						Transactions: []*edi.Transaction{
							{
								SetIdentifier: "837",
								ControlNumber: "0001",
								Segments: []*edi.Segment{
									{ID: "CLM", Elements: []string{"CLAIM001", "1500.00"}},
								},
							},
							{
								SetIdentifier: "837",
								ControlNumber: "0002",
								Segments: []*edi.Segment{
									{ID: "CLM", Elements: []string{"CLAIM002", "2000.00"}},
								},
							},
						},
					},
				},
			},
		}
		guide := &CompanionGuide{
			ID:               "test",
			Name:             "Test",
			TransactionTypes: []string{"837"},
		}

		results := ValidateAllTransactions(result, guide)
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("transactions across multiple groups", func(t *testing.T) {
		result := &edi.ParseResult{
			Interchange: &edi.Interchange{
				Delimiters: edi.Delimiters{
					Element:    '*',
					Segment:    '~',
					Subelement: ':',
				},
				FunctionalGroups: []*edi.FunctionalGroup{
					{
						Transactions: []*edi.Transaction{
							{SetIdentifier: "837", ControlNumber: "0001", Segments: []*edi.Segment{}},
						},
					},
					{
						Transactions: []*edi.Transaction{
							{SetIdentifier: "837", ControlNumber: "0002", Segments: []*edi.Segment{}},
							{SetIdentifier: "837", ControlNumber: "0003", Segments: []*edi.Segment{}},
						},
					},
				},
			},
		}
		guide := &CompanionGuide{
			ID:               "test",
			Name:             "Test",
			TransactionTypes: []string{"837"},
		}

		results := ValidateAllTransactions(result, guide)
		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
		}
	})
}

func TestDetectAndValidate(t *testing.T) {
	t.Run("nil interchange", func(t *testing.T) {
		result := &edi.ParseResult{Interchange: nil}
		registry := NewRegistry()

		validationResult := DetectAndValidate(result, registry)
		if validationResult != nil {
			t.Error("expected nil for nil interchange")
		}
	})

	t.Run("no matching guide", func(t *testing.T) {
		segments := []*edi.Segment{
			{ID: "CLM", Elements: []string{"CLAIM001", "1500.00"}},
		}
		result := createMinimalParseResult("UNKNOWN_RECV", "837", segments)
		registry := NewRegistry()

		validationResult := DetectAndValidate(result, registry)
		if validationResult != nil {
			t.Error("expected nil when no guide matches")
		}
	})

	t.Run("matching guide found", func(t *testing.T) {
		segments := []*edi.Segment{
			{ID: "CLM", Elements: []string{"CLAIM001", "1500.00"}},
		}
		result := createMinimalParseResult("MEDICARE123456", "837", segments)

		registry := NewRegistry()
		guide := &CompanionGuide{
			ID:               "medicare",
			Name:             "Medicare",
			ReceiverIDs:      []string{"MEDICARE123456"},
			TransactionTypes: []string{"837"},
		}
		if err := registry.Register(guide); err != nil {
			t.Fatalf("failed to register guide: %v", err)
		}

		validationResult := DetectAndValidate(result, registry)
		if validationResult == nil {
			t.Fatal("expected non-nil result")
		}
		if validationResult.GuideID != "medicare" {
			t.Errorf("expected guide ID 'medicare', got '%s'", validationResult.GuideID)
		}
	})
}

func TestDetectFromParseResult(t *testing.T) {
	t.Run("nil interchange", func(t *testing.T) {
		result := &edi.ParseResult{Interchange: nil}
		registry := NewRegistry()

		guide := DetectFromParseResult(result, registry)
		if guide != nil {
			t.Error("expected nil for nil interchange")
		}
	})

	t.Run("detect by receiver ID", func(t *testing.T) {
		segments := []*edi.Segment{
			{ID: "CLM", Elements: []string{"CLAIM001", "1500.00"}},
		}
		result := createMinimalParseResult("BCBS12345678", "837", segments)

		registry := NewRegistry()
		guide := &CompanionGuide{
			ID:               "bcbs",
			Name:             "BCBS",
			ReceiverIDs:      []string{"BCBS12345678"},
			TransactionTypes: []string{"837"},
		}
		if err := registry.Register(guide); err != nil {
			t.Fatalf("failed to register guide: %v", err)
		}

		detected := DetectFromParseResult(result, registry)
		if detected == nil {
			t.Fatal("expected guide to be detected")
		}
		if detected.ID != "bcbs" {
			t.Errorf("expected 'bcbs', got '%s'", detected.ID)
		}
	})

	t.Run("detect by transaction type", func(t *testing.T) {
		segments := []*edi.Segment{
			{ID: "CLM", Elements: []string{"CLAIM001", "1500.00"}},
		}
		result := createMinimalParseResult("ANYRECEIVER", "837", segments)

		registry := NewRegistry()
		guide := &CompanionGuide{
			ID:               "generic837",
			Name:             "Generic 837",
			TransactionTypes: []string{"837"},
		}
		if err := registry.Register(guide); err != nil {
			t.Fatalf("failed to register guide: %v", err)
		}

		detected := DetectFromParseResult(result, registry)
		if detected == nil {
			t.Fatal("expected guide to be detected")
		}
		if detected.ID != "generic837" {
			t.Errorf("expected 'generic837', got '%s'", detected.ID)
		}
	})

	t.Run("empty functional groups", func(t *testing.T) {
		result := &edi.ParseResult{
			Interchange: &edi.Interchange{
				ReceiverID:       "RECEIVER123",
				FunctionalGroups: []*edi.FunctionalGroup{},
			},
		}
		registry := NewRegistry()

		detected := DetectFromParseResult(result, registry)
		if detected != nil {
			t.Error("expected nil for empty groups")
		}
	})

	t.Run("empty transactions", func(t *testing.T) {
		result := &edi.ParseResult{
			Interchange: &edi.Interchange{
				ReceiverID: "RECEIVER123",
				FunctionalGroups: []*edi.FunctionalGroup{
					{Transactions: []*edi.Transaction{}},
				},
			},
		}
		registry := NewRegistry()

		detected := DetectFromParseResult(result, registry)
		if detected != nil {
			t.Error("expected nil for empty transactions")
		}
	})
}

func TestGetParseResultInfo(t *testing.T) {
	t.Run("nil interchange", func(t *testing.T) {
		result := &edi.ParseResult{Interchange: nil}

		info := GetParseResultInfo(result)
		if info.ReceiverID != "" {
			t.Error("expected empty receiver ID")
		}
	})

	t.Run("full info extraction", func(t *testing.T) {
		result := &edi.ParseResult{
			Interchange: &edi.Interchange{
				ReceiverID:        "RECEIVER123456",
				ReceiverQualifier: "ZZ",
				SenderID:          "SENDER789012",
				SenderQualifier:   "01",
				FunctionalGroups: []*edi.FunctionalGroup{
					{
						VersionCode: "005010X222A1",
						Transactions: []*edi.Transaction{
							{SetIdentifier: "837"},
						},
					},
				},
			},
		}

		info := GetParseResultInfo(result)
		if info.ReceiverID != "RECEIVER123456" {
			t.Errorf("expected 'RECEIVER123456', got '%s'", info.ReceiverID)
		}
		if info.ReceiverQual != "ZZ" {
			t.Errorf("expected 'ZZ', got '%s'", info.ReceiverQual)
		}
		if info.SenderID != "SENDER789012" {
			t.Errorf("expected 'SENDER789012', got '%s'", info.SenderID)
		}
		if info.SenderQual != "01" {
			t.Errorf("expected '01', got '%s'", info.SenderQual)
		}
		if info.TransactionType != "837" {
			t.Errorf("expected '837', got '%s'", info.TransactionType)
		}
		if info.VersionCode != "005010X222A1" {
			t.Errorf("expected '005010X222A1', got '%s'", info.VersionCode)
		}
	})

	t.Run("no functional groups", func(t *testing.T) {
		result := &edi.ParseResult{
			Interchange: &edi.Interchange{
				ReceiverID:       "RECEIVER123",
				FunctionalGroups: []*edi.FunctionalGroup{},
			},
		}

		info := GetParseResultInfo(result)
		if info.ReceiverID != "RECEIVER123" {
			t.Errorf("expected 'RECEIVER123', got '%s'", info.ReceiverID)
		}
		if info.TransactionType != "" {
			t.Error("expected empty transaction type")
		}
	})

	t.Run("no transactions", func(t *testing.T) {
		result := &edi.ParseResult{
			Interchange: &edi.Interchange{
				ReceiverID: "RECEIVER123",
				FunctionalGroups: []*edi.FunctionalGroup{
					{
						VersionCode:  "005010X222A1",
						Transactions: []*edi.Transaction{},
					},
				},
			},
		}

		info := GetParseResultInfo(result)
		if info.VersionCode != "005010X222A1" {
			t.Errorf("expected '005010X222A1', got '%s'", info.VersionCode)
		}
		if info.TransactionType != "" {
			t.Error("expected empty transaction type")
		}
	})
}

func TestMustValidate(t *testing.T) {
	t.Run("valid result does not panic", func(t *testing.T) {
		segments := []*edi.Segment{
			{ID: "CLM", Elements: []string{"CLAIM001", "1500.00"}},
		}
		result := createMinimalParseResult("RECEIVER123", "837", segments)
		guide := &CompanionGuide{
			ID:               "test",
			Name:             "Test",
			TransactionTypes: []string{"837"},
		}

		// Should not panic
		validationResult := MustValidate(result, guide)
		if !validationResult.Valid {
			t.Error("expected valid result")
		}
	})

	t.Run("invalid result panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for invalid result")
			}
		}()

		segments := []*edi.Segment{
			{ID: "CLM", Elements: []string{""}}, // Missing required claim ID
		}
		result := createMinimalParseResult("RECEIVER123", "837", segments)
		guide := &CompanionGuide{
			ID:               "test",
			Name:             "Test",
			TransactionTypes: []string{"837"},
			Overrides: []ElementOverride{
				{Path: "CLM.01", Requirement: RequirementRequired},
			},
		}

		// Should panic
		MustValidate(result, guide)
	})
}
