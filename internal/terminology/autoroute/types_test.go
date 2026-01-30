package autoroute

import (
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

func TestSuggestResultClassify(t *testing.T) {
	highThreshold := 0.90
	medThreshold := 0.70

	tests := []struct {
		name     string
		result   *SuggestResult
		wantType DecisionType
	}{
		{
			name:     "nil best match",
			result:   &SuggestResult{BestMatch: nil, Confidence: 0.95},
			wantType: DecisionNoMatch,
		},
		{
			name:     "confidence below 0.5",
			result:   &SuggestResult{BestMatch: &Candidate{}, Confidence: 0.45},
			wantType: DecisionNoMatch,
		},
		{
			name:     "high confidence at threshold",
			result:   &SuggestResult{BestMatch: &Candidate{}, Confidence: 0.90},
			wantType: DecisionHighConfidence,
		},
		{
			name:     "high confidence above threshold",
			result:   &SuggestResult{BestMatch: &Candidate{}, Confidence: 0.95},
			wantType: DecisionHighConfidence,
		},
		{
			name:     "medium confidence at threshold",
			result:   &SuggestResult{BestMatch: &Candidate{}, Confidence: 0.70},
			wantType: DecisionMediumConfidence,
		},
		{
			name:     "medium confidence in range",
			result:   &SuggestResult{BestMatch: &Candidate{}, Confidence: 0.85},
			wantType: DecisionMediumConfidence,
		},
		{
			name:     "low confidence",
			result:   &SuggestResult{BestMatch: &Candidate{}, Confidence: 0.55},
			wantType: DecisionLowConfidence,
		},
		{
			name:     "edge case just below medium threshold",
			result:   &SuggestResult{BestMatch: &Candidate{}, Confidence: 0.69},
			wantType: DecisionLowConfidence,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.Classify(highThreshold, medThreshold)
			if got != tt.wantType {
				t.Errorf("Classify() = %v, want %v", got, tt.wantType)
			}
		})
	}
}

func TestSuggestResultToCodeMapping(t *testing.T) {
	t.Run("nil best match returns nil", func(t *testing.T) {
		result := &SuggestResult{BestMatch: nil}
		req := SuggestRequest{
			SourceCode:   "LAB001",
			SourceSystem: "custom",
			TargetSystem: "http://loinc.org",
		}

		got := result.ToCodeMapping(req, "test-user")
		if got != nil {
			t.Error("expected nil for nil best match")
		}
	})

	t.Run("creates mapping from best match", func(t *testing.T) {
		result := &SuggestResult{
			BestMatch: &Candidate{
				Code:        "2345-7",
				Display:     "Glucose [Mass/volume] in Serum or Plasma",
				System:      "http://loinc.org",
				Equivalence: db.EquivalenceEquivalent,
			},
			Confidence: 0.92,
		}

		req := SuggestRequest{
			SourceCode:    "GLU001",
			SourceSystem:  "epic_labs",
			SourceDisplay: "Glucose Level",
			TargetSystem:  "http://loinc.org",
			ProfileID:     "profile-abc",
		}

		got := result.ToCodeMapping(req, "approver@example.com")

		if got == nil {
			t.Fatal("expected non-nil mapping")
		}

		if got.SourceCode != req.SourceCode {
			t.Errorf("SourceCode = %v, want %v", got.SourceCode, req.SourceCode)
		}
		if got.SourceSystem != req.SourceSystem {
			t.Errorf("SourceSystem = %v, want %v", got.SourceSystem, req.SourceSystem)
		}
		if got.SourceDisplay != req.SourceDisplay {
			t.Errorf("SourceDisplay = %v, want %v", got.SourceDisplay, req.SourceDisplay)
		}
		if got.TargetCode != result.BestMatch.Code {
			t.Errorf("TargetCode = %v, want %v", got.TargetCode, result.BestMatch.Code)
		}
		if got.TargetDisplay != result.BestMatch.Display {
			t.Errorf("TargetDisplay = %v, want %v", got.TargetDisplay, result.BestMatch.Display)
		}
		if got.TargetSystem != result.BestMatch.System {
			t.Errorf("TargetSystem = %v, want %v", got.TargetSystem, result.BestMatch.System)
		}
		if got.Equivalence != result.BestMatch.Equivalence {
			t.Errorf("Equivalence = %v, want %v", got.Equivalence, result.BestMatch.Equivalence)
		}
		if got.Confidence == nil || *got.Confidence != result.Confidence {
			t.Errorf("Confidence = %v, want %v", got.Confidence, result.Confidence)
		}
		if got.Origin != db.OriginApprovedAutoroute {
			t.Errorf("Origin = %v, want %v", got.Origin, db.OriginApprovedAutoroute)
		}
		if got.ProfileID != req.ProfileID {
			t.Errorf("ProfileID = %v, want %v", got.ProfileID, req.ProfileID)
		}
		if got.CreatedBy != "approver@example.com" {
			t.Errorf("CreatedBy = %v, want approver@example.com", got.CreatedBy)
		}
	})
}

func TestDecisionTypeConstants(t *testing.T) {
	// Verify constant values match expected GraphQL enum values
	tests := []struct {
		dt   DecisionType
		want string
	}{
		{DecisionHighConfidence, "AUTOROUTE_HIGH_CONF"},
		{DecisionMediumConfidence, "AUTOROUTE_MED_CONF"},
		{DecisionLowConfidence, "AUTOROUTE_LOW_CONF"},
		{DecisionNoMatch, "NO_MATCH"},
	}

	for _, tt := range tests {
		t.Run(string(tt.dt), func(t *testing.T) {
			if string(tt.dt) != tt.want {
				t.Errorf("DecisionType = %v, want %v", tt.dt, tt.want)
			}
		})
	}
}
