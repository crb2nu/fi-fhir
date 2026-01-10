package matching

import (
	"math"
	"testing"
)

func TestJaroWinkler(t *testing.T) {
	tests := []struct {
		name     string
		s1, s2   string
		minScore float64
		maxScore float64
	}{
		{
			name:     "exact match",
			s1:       "SMITH",
			s2:       "SMITH",
			minScore: 1.0,
			maxScore: 1.0,
		},
		{
			name:     "similar names",
			s1:       "SMITH",
			s2:       "SMYTH",
			minScore: 0.80,
			maxScore: 0.95,
		},
		{
			name:     "common prefix boost",
			s1:       "JOHNSON",
			s2:       "JOHNSEN",
			minScore: 0.90,
			maxScore: 0.98,
		},
		{
			name:     "completely different",
			s1:       "SMITH",
			s2:       "JONES",
			minScore: 0.0,
			maxScore: 0.60,
		},
		{
			name:     "empty string",
			s1:       "SMITH",
			s2:       "",
			minScore: 0.0,
			maxScore: 0.0,
		},
		{
			name:     "transposition",
			s1:       "MARTHA",
			s2:       "MARHTA",
			minScore: 0.90,
			maxScore: 0.98,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := JaroWinkler(tt.s1, tt.s2)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("JaroWinkler(%q, %q) = %v, want between %v and %v",
					tt.s1, tt.s2, score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestSoundex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple name", "Smith", "S530"},
		{"variant spelling", "Smyth", "S530"},
		{"robert", "Robert", "R163"},
		{"rupert", "Rupert", "R163"},
		{"ashcraft", "Ashcraft", "A261"},
		{"ashcroft", "Ashcroft", "A261"},
		{"tymczak", "Tymczak", "T522"},
		{"pfister", "Pfister", "P236"},
		{"honeyman", "Honeyman", "H555"},
		{"empty", "", ""},
		{"short", "A", "A000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Soundex(tt.input)
			if result != tt.expected {
				t.Errorf("Soundex(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSoundexMatch(t *testing.T) {
	tests := []struct {
		name    string
		s1, s2  string
		matches bool
	}{
		{"Smith variants", "Smith", "Smyth", true},
		{"Robert variants", "Robert", "Rupert", true},
		{"different names", "Smith", "Jones", false},
		{"same name", "John", "John", true},
		{"phonetically similar", "Katherine", "Katharine", true}, // Same first letter, phonetically similar
		{"case insensitive", "SMITH", "smith", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SoundexMatch(tt.s1, tt.s2)
			if result != tt.matches {
				t.Errorf("SoundexMatch(%q, %q) = %v, want %v",
					tt.s1, tt.s2, result, tt.matches)
			}
		})
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		name     string
		s1, s2   string
		expected int
	}{
		{"same string", "hello", "hello", 0},
		{"one insertion", "hello", "helloo", 1},
		{"one deletion", "hello", "hell", 1},
		{"one substitution", "hello", "hallo", 1},
		{"empty string", "hello", "", 5},
		{"both empty", "", "", 0},
		{"kitten sitting", "kitten", "sitting", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LevenshteinDistance(tt.s1, tt.s2)
			if result != tt.expected {
				t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d",
					tt.s1, tt.s2, result, tt.expected)
			}
		})
	}
}

func TestNormalizedLevenshtein(t *testing.T) {
	tests := []struct {
		name     string
		s1, s2   string
		minScore float64
		maxScore float64
	}{
		{"exact match", "hello", "hello", 1.0, 1.0},
		{"one char diff", "hello", "hallo", 0.75, 0.85},
		{"empty strings", "", "", 1.0, 1.0},
		{"completely different", "abc", "xyz", 0.0, 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizedLevenshtein(tt.s1, tt.s2)
			if result < tt.minScore || result > tt.maxScore {
				t.Errorf("NormalizedLevenshtein(%q, %q) = %v, want between %v and %v",
					tt.s1, tt.s2, result, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple uppercase", "John Smith", "JOHN SMITH"},
		{"with prefix", "Dr. John Smith", "JOHN SMITH"},
		{"with suffix", "John Smith Jr", "JOHN SMITH"},
		{"with punctuation", "O'Brien-Smith", "OBRIENSMITH"},
		{"extra whitespace", "  John    Smith  ", "JOHN SMITH"},
		{"complex", "Mr. John W. Smith III", "JOHN W SMITH"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeName(%q) = %q, want %q",
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"formatted", "(555) 123-4567", "5551234567"},
		{"with country code", "+1-555-123-4567", "5551234567"},
		{"just digits", "5551234567", "5551234567"},
		{"with dots", "555.123.4567", "5551234567"},
		{"too short", "123456", ""},
		{"too long", "123456789012", ""},
		{"with spaces", "555 123 4567", "5551234567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizePhone(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizePhone(%q) = %q, want %q",
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeSSN(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"formatted", "234-56-7890", "234567890"},
		{"just digits", "345678901", "345678901"},
		{"with spaces", "456 78 9012", "456789012"},
		{"invalid pattern", "000000000", ""},
		{"sequential", "123456789", ""}, // Sequential is invalid
		{"all ones", "111111111", ""},
		{"too short", "12345678", ""},
		{"too long", "1234567890", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeSSN(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeSSN(%q) = %q, want %q",
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestPhoneMatch(t *testing.T) {
	tests := []struct {
		name     string
		p1, p2   string
		expected float64
	}{
		{"exact match", "5551234567", "5551234567", 1.0},
		{"formatted match", "(555) 123-4567", "555-123-4567", 1.0},
		{"last 7 match", "5551234567", "4441234567", 0.7},
		{"no match", "5551234567", "5559876543", 0.0},
		{"empty", "", "5551234567", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PhoneMatch(tt.p1, tt.p2)
			if math.Abs(result-tt.expected) > 0.01 {
				t.Errorf("PhoneMatch(%q, %q) = %v, want %v",
					tt.p1, tt.p2, result, tt.expected)
			}
		})
	}
}
