package matching

import (
	"testing"
	"time"
)

func TestDeterministicMatcher(t *testing.T) {
	matcher := NewDeterministicMatcher()

	tests := []struct {
		name           string
		patient1       *Patient
		patient2       *Patient
		expectedResult MatchResult
		expectedRule   string
	}{
		{
			name: "SSN exact match",
			patient1: &Patient{
				SSN:        "234567890",
				FamilyName: "Smith",
				GivenName:  "John",
			},
			patient2: &Patient{
				SSN:        "234567890",
				FamilyName: "Smyth",
				GivenName:  "Jon",
			},
			expectedResult: MatchConfirmed,
			expectedRule:   "ssn_exact",
		},
		{
			name: "MBI exact match",
			patient1: &Patient{
				MBI:        "1EG4TE5MK72",
				FamilyName: "Smith",
				GivenName:  "John",
			},
			patient2: &Patient{
				MBI:        "1EG4TE5MK72",
				FamilyName: "Smith",
				GivenName:  "John",
			},
			expectedResult: MatchConfirmed,
			expectedRule:   "mbi_exact",
		},
		{
			name: "MRN + System match",
			patient1: &Patient{
				MRN:        "123456",
				MRNSystem:  "HOSPITAL_A",
				FamilyName: "Smith",
				GivenName:  "John",
			},
			patient2: &Patient{
				MRN:        "123456",
				MRNSystem:  "HOSPITAL_A",
				FamilyName: "Smith",
				GivenName:  "John",
			},
			expectedResult: MatchConfirmed,
			expectedRule:   "mrn_system",
		},
		{
			name: "Full demographics match",
			patient1: &Patient{
				FamilyName: "Smith",
				GivenName:  "John",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
				Gender:     "M",
			},
			patient2: &Patient{
				FamilyName: "Smith",
				GivenName:  "John",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
				Gender:     "M",
			},
			expectedResult: MatchProbable,
			expectedRule:   "full_demographics",
		},
		{
			name: "Name + DOB only",
			patient1: &Patient{
				FamilyName: "Smith",
				GivenName:  "John",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
			},
			patient2: &Patient{
				FamilyName: "Smith",
				GivenName:  "John",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
			},
			expectedResult: MatchPossible,
			expectedRule:   "name_dob",
		},
		{
			name: "No match - different people",
			patient1: &Patient{
				FamilyName: "Smith",
				GivenName:  "John",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
			},
			patient2: &Patient{
				FamilyName: "Jones",
				GivenName:  "Jane",
				DOB:        time.Date(1990, 7, 20, 0, 0, 0, 0, time.UTC),
			},
			expectedResult: MatchNoMatch,
			expectedRule:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, rule := matcher.Match(tt.patient1, tt.patient2)
			if result != tt.expectedResult {
				t.Errorf("Match() result = %v, want %v", result, tt.expectedResult)
			}
			if rule != tt.expectedRule {
				t.Errorf("Match() rule = %v, want %v", rule, tt.expectedRule)
			}
		})
	}
}

func TestProbabilisticScorer(t *testing.T) {
	scorer := NewProbabilisticScorer(DefaultScorerConfig())

	tests := []struct {
		name           string
		patient1       *Patient
		patient2       *Patient
		minScore       float64
		maxScore       float64
		expectedFields []string // fields that should be available
	}{
		{
			name: "identical patients",
			patient1: &Patient{
				SSN:        "234567890",
				FamilyName: "Smith",
				GivenName:  "John",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
				Gender:     "M",
				Phone:      "5551234567",
			},
			patient2: &Patient{
				SSN:        "234567890",
				FamilyName: "Smith",
				GivenName:  "John",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
				Gender:     "M",
				Phone:      "5551234567",
			},
			minScore:       0.95,
			maxScore:       1.0,
			expectedFields: []string{"family_name", "given_name", "dob", "gender", "phone"},
		},
		{
			name: "similar names different SSN",
			patient1: &Patient{
				SSN:        "234567890",
				FamilyName: "Smith",
				GivenName:  "John",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
			},
			patient2: &Patient{
				SSN:        "987654321",
				FamilyName: "Smyth",
				GivenName:  "Jon",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
			},
			minScore: 0.40,
			maxScore: 0.98, // Allow for high name similarity
		},
		{
			name: "no common fields",
			patient1: &Patient{
				FamilyName: "Smith",
			},
			patient2: &Patient{
				GivenName: "John",
			},
			minScore: 0.0,
			maxScore: 0.10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scorer.Score(tt.patient1, tt.patient2)
			if result.Score < tt.minScore || result.Score > tt.maxScore {
				t.Errorf("Score() = %v, want between %v and %v",
					result.Score, tt.minScore, tt.maxScore)
			}

			for _, field := range tt.expectedFields {
				fs, ok := result.FieldScores[field]
				if !ok {
					t.Errorf("Expected field %q in FieldScores", field)
				} else if !fs.Available {
					t.Errorf("Field %q should be available", field)
				}
			}
		})
	}
}

func TestCombinedMatcher(t *testing.T) {
	matcher := NewMatcher(DefaultMatcherConfig())

	tests := []struct {
		name           string
		patient1       *Patient
		patient2       *Patient
		expectedResult MatchResult
		expectedSource string
	}{
		{
			name: "deterministic SSN match",
			patient1: &Patient{
				SSN:        "234567890",
				FamilyName: "Smith",
			},
			patient2: &Patient{
				SSN:        "234567890",
				FamilyName: "Jones",
			},
			expectedResult: MatchConfirmed,
			expectedSource: "deterministic",
		},
		{
			name: "probabilistic high score",
			patient1: &Patient{
				SSN:        "234567890",
				FamilyName: "Smith",
				GivenName:  "John",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
				Gender:     "M",
				Phone:      "5551234567",
				Email:      "john.smith@example.com",
			},
			patient2: &Patient{
				SSN:        "234567890",
				FamilyName: "Smith",
				GivenName:  "John",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
				Gender:     "M",
				Phone:      "5551234567",
				Email:      "john.smith@example.com",
			},
			expectedResult: MatchConfirmed,
			expectedSource: "deterministic", // SSN match triggers deterministic
		},
		{
			name: "combined with demographics rule",
			patient1: &Patient{
				FamilyName: "Smith",
				GivenName:  "John",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
				Gender:     "M",
			},
			patient2: &Patient{
				FamilyName: "Smith",
				GivenName:  "John",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
				Gender:     "M",
			},
			expectedResult: MatchProbable,
			expectedSource: "combined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(tt.patient1, tt.patient2)
			if result.Result != tt.expectedResult {
				t.Errorf("Match() result = %v, want %v", result.Result, tt.expectedResult)
			}
			if result.Source != tt.expectedSource {
				t.Errorf("Match() source = %v, want %v", result.Source, tt.expectedSource)
			}
		})
	}
}

func TestFindCandidates(t *testing.T) {
	matcher := NewMatcher(DefaultMatcherConfig())

	target := &Patient{
		FamilyName: "Smith",
		GivenName:  "John",
		DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
	}

	candidates := []*Patient{
		{
			FamilyName: "Smith",
			GivenName:  "John",
			DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			FamilyName: "Smith",
			GivenName:  "Jane",
			DOB:        time.Date(1990, 7, 20, 0, 0, 0, 0, time.UTC),
		},
		{
			FamilyName: "Jones",
			GivenName:  "Bob",
			DOB:        time.Date(1975, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	results := matcher.FindCandidates(target, candidates, 0.5)

	if len(results) == 0 {
		t.Fatal("Expected at least one candidate match")
	}

	// First result should be the best match
	if results[0].Score < 0.9 {
		t.Errorf("Expected first result to have high score, got %v", results[0].Score)
	}
}

func TestBlockingKey(t *testing.T) {
	tests := []struct {
		name     string
		patient  *Patient
		expected string
	}{
		{
			name: "normal patient",
			patient: &Patient{
				FamilyName: "Smith",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
			},
			expected: "19850315_S",
		},
		{
			name: "lowercase name",
			patient: &Patient{
				FamilyName: "smith",
				DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
			},
			expected: "19850315_S",
		},
		{
			name: "no DOB",
			patient: &Patient{
				FamilyName: "Smith",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BlockingKey(tt.patient)
			if result != tt.expected {
				t.Errorf("BlockingKey() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestBatchMatcherFindDuplicates(t *testing.T) {
	matcher := NewMatcher(DefaultMatcherConfig())
	batcher := NewBatchMatcher(matcher)

	patients := []*Patient{
		{
			MRN:        "001",
			MRNSystem:  "SYS_A",
			FamilyName: "Smith",
			GivenName:  "John",
			DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			MRN:        "002",
			MRNSystem:  "SYS_B",
			FamilyName: "Smith",
			GivenName:  "John",
			DOB:        time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			MRN:        "003",
			MRNSystem:  "SYS_A",
			FamilyName: "Jones",
			GivenName:  "Jane",
			DOB:        time.Date(1990, 7, 20, 0, 0, 0, 0, time.UTC),
		},
	}

	duplicates := batcher.FindDuplicates(patients, 0.5)

	if len(duplicates) == 0 {
		t.Fatal("Expected at least one duplicate pair")
	}

	// Should find Smith/Smith as duplicates
	found := false
	for _, dup := range duplicates {
		if len(dup.Patients) >= 2 &&
			dup.Patients[0].FamilyName == "Smith" &&
			dup.Patients[1].FamilyName == "Smith" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find Smith duplicate pair")
	}
}
