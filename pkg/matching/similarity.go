// Package matching provides patient matching algorithms for healthcare identity resolution.
package matching

import (
	"strings"
	"unicode"
)

// JaroWinkler computes the Jaro-Winkler similarity between two strings.
// Returns a value between 0.0 (no similarity) and 1.0 (exact match).
// Jaro-Winkler gives higher scores to strings matching from the beginning,
// making it well-suited for surname/name matching.
func JaroWinkler(s1, s2 string) float64 {
	jaro := Jaro(s1, s2)

	// Winkler modification: boost for common prefix (up to 4 chars)
	prefixLen := commonPrefixLength(s1, s2, 4)

	// Scaling factor (standard is 0.1)
	const p = 0.1

	return jaro + float64(prefixLen)*p*(1-jaro)
}

// Jaro computes the Jaro similarity between two strings.
// Returns a value between 0.0 and 1.0.
func Jaro(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	len1, len2 := len(s1), len(s2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	// Calculate the match window
	matchWindow := max(len1, len2)/2 - 1
	if matchWindow < 0 {
		matchWindow = 0
	}

	s1Matches := make([]bool, len1)
	s2Matches := make([]bool, len2)

	matches := 0
	transpositions := 0

	// Find matches
	for i := 0; i < len1; i++ {
		start := max(0, i-matchWindow)
		end := min(i+matchWindow+1, len2)

		for j := start; j < end; j++ {
			if s2Matches[j] || s1[i] != s2[j] {
				continue
			}
			s1Matches[i] = true
			s2Matches[j] = true
			matches++
			break
		}
	}

	if matches == 0 {
		return 0.0
	}

	// Count transpositions
	k := 0
	for i := 0; i < len1; i++ {
		if !s1Matches[i] {
			continue
		}
		for !s2Matches[k] {
			k++
		}
		if s1[i] != s2[k] {
			transpositions++
		}
		k++
	}

	m := float64(matches)
	t := float64(transpositions) / 2

	return (m/float64(len1) + m/float64(len2) + (m-t)/m) / 3.0
}

// commonPrefixLength returns the length of the common prefix, up to maxLen.
func commonPrefixLength(s1, s2 string, maxLen int) int {
	n := min(len(s1), len(s2), maxLen)
	for i := 0; i < n; i++ {
		if s1[i] != s2[i] {
			return i
		}
	}
	return n
}

// Soundex generates the Soundex code for a string.
// Soundex is a phonetic algorithm that encodes names by sound,
// useful for catching spelling variations like "Smith" vs "Smyth".
// Returns a 4-character code (letter + 3 digits).
func Soundex(s string) string {
	if len(s) == 0 {
		return ""
	}

	// Normalize to uppercase
	s = strings.ToUpper(s)

	// Keep only letters
	var cleaned strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) {
			cleaned.WriteRune(r)
		}
	}
	s = cleaned.String()

	if len(s) == 0 {
		return ""
	}

	// Start with the first letter
	code := string(s[0])

	// Soundex mapping
	soundexMap := map[rune]byte{
		'B': '1', 'F': '1', 'P': '1', 'V': '1',
		'C': '2', 'G': '2', 'J': '2', 'K': '2', 'Q': '2', 'S': '2', 'X': '2', 'Z': '2',
		'D': '3', 'T': '3',
		'L': '4',
		'M': '5', 'N': '5',
		'R': '6',
		// A, E, I, O, U, H, W, Y are not coded
	}

	lastCode := soundexMap[rune(s[0])]

	for i := 1; i < len(s) && len(code) < 4; i++ {
		c := soundexMap[rune(s[i])]
		if c != 0 && c != lastCode {
			code += string(c)
		}
		if c != 0 {
			lastCode = c
		} else {
			// Vowels and H, W don't count as separators if they're between same codes
			// But if the letter is A, E, I, O, U, reset lastCode
			r := rune(s[i])
			if r == 'A' || r == 'E' || r == 'I' || r == 'O' || r == 'U' {
				lastCode = 0
			}
		}
	}

	// Pad with zeros
	for len(code) < 4 {
		code += "0"
	}

	return code
}

// SoundexMatch returns true if two strings have the same Soundex code.
func SoundexMatch(s1, s2 string) bool {
	code1 := Soundex(s1)
	code2 := Soundex(s2)
	return code1 != "" && code1 == code2
}

// LevenshteinDistance computes the edit distance between two strings.
func LevenshteinDistance(s1, s2 string) int {
	if s1 == s2 {
		return 0
	}

	len1, len2 := len(s1), len(s2)
	if len1 == 0 {
		return len2
	}
	if len2 == 0 {
		return len1
	}

	// Use two rows instead of full matrix for memory efficiency
	prev := make([]int, len2+1)
	curr := make([]int, len2+1)

	for j := 0; j <= len2; j++ {
		prev[j] = j
	}

	for i := 1; i <= len1; i++ {
		curr[0] = i
		for j := 1; j <= len2; j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			curr[j] = min(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}

	return prev[len2]
}

// NormalizedLevenshtein returns a similarity score between 0.0 and 1.0
// based on the Levenshtein distance.
func NormalizedLevenshtein(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	maxLen := max(len(s1), len(s2))
	if maxLen == 0 {
		return 1.0
	}

	distance := LevenshteinDistance(s1, s2)
	return 1.0 - float64(distance)/float64(maxLen)
}

// NormalizeName normalizes a name for matching.
// Removes titles, suffixes, punctuation, and normalizes whitespace.
func NormalizeName(name string) string {
	name = strings.ToUpper(strings.TrimSpace(name))

	// Remove common prefixes
	prefixes := []string{"MR ", "MRS ", "MS ", "DR ", "MISS ", "MR. ", "MRS. ", "MS. ", "DR. ", "MISS. "}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			name = strings.TrimPrefix(name, p)
			break
		}
	}

	// Remove common suffixes
	suffixes := []string{" JR", " SR", " II", " III", " IV", " MD", " PHD", " DO", " RN", " ESQ"}
	for _, s := range suffixes {
		if strings.HasSuffix(name, s) {
			name = strings.TrimSuffix(name, s)
			break
		}
	}

	// Remove punctuation
	var cleaned strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsSpace(r) {
			cleaned.WriteRune(r)
		}
	}

	// Collapse whitespace
	return strings.Join(strings.Fields(cleaned.String()), " ")
}

// NormalizePhone normalizes a phone number to 10 digits.
// Strips country code and non-digit characters.
func NormalizePhone(phone string) string {
	var digits strings.Builder
	for _, r := range phone {
		if unicode.IsDigit(r) {
			digits.WriteRune(r)
		}
	}
	result := digits.String()

	// Strip US country code
	if len(result) == 11 && result[0] == '1' {
		result = result[1:]
	}

	if len(result) != 10 {
		return ""
	}

	return result
}

// NormalizeSSN normalizes an SSN to 9 digits.
func NormalizeSSN(ssn string) string {
	var digits strings.Builder
	for _, r := range ssn {
		if unicode.IsDigit(r) {
			digits.WriteRune(r)
		}
	}
	result := digits.String()

	if len(result) != 9 {
		return ""
	}

	// Reject invalid patterns
	invalid := []string{"000000000", "123456789", "111111111", "999999999"}
	for _, pattern := range invalid {
		if result == pattern {
			return ""
		}
	}

	return result
}

// PhoneMatch compares two phone numbers.
// Returns the similarity score (1.0 for exact, partial for last N digits match).
func PhoneMatch(p1, p2 string) float64 {
	p1 = NormalizePhone(p1)
	p2 = NormalizePhone(p2)

	if p1 == "" || p2 == "" {
		return 0.0
	}

	if p1 == p2 {
		return 1.0
	}

	// Check last 7 digits (area codes can change)
	if len(p1) >= 7 && len(p2) >= 7 {
		last7_1 := p1[len(p1)-7:]
		last7_2 := p2[len(p2)-7:]
		if last7_1 == last7_2 {
			return 0.7
		}
	}

	return 0.0
}
