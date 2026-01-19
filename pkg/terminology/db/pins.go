package db

import (
	"context"
	"sort"
	"strings"
)

// PinStatus describes whether the active DB release matches an expected pinned version.
type PinStatus struct {
	Vocabulary       string
	ExpectedVersion  string
	ActiveVersion    string
	Match            bool
	ActiveReleaseSet bool
}

// NormalizeVocabulary canonicalizes a user-provided vocabulary key to the schema vocabulary constants.
// It accepts common aliases and is case-insensitive.
func NormalizeVocabulary(vocab string) string {
	v := strings.TrimSpace(strings.ToUpper(vocab))
	switch v {
	case "UMLS":
		return VocabUMLS
	case "RXNORM":
		return VocabRxNorm
	case "SNOMED", "SNOMEDCT", "SNOMEDCTUS", "SNOMEDCT_US":
		return VocabSNOMEDCT
	case "LOINC":
		return VocabLOINC
	case "ICD10CM", "ICD-10-CM":
		return VocabICD10CM
	case "ICD10PCS", "ICD-10-PCS":
		return VocabICD10PCS
	case "CPT":
		return VocabCPT
	case "HCPCS":
		return VocabHCPCS
	case "CVX":
		return VocabCVX
	case "NDC":
		return VocabNDC
	default:
		return v
	}
}

// CheckPinnedReleases compares configured pins against the currently active releases in the database.
func (m *Migrator) CheckPinnedReleases(ctx context.Context, pins map[string]string) ([]PinStatus, error) {
	if len(pins) == 0 {
		return nil, nil
	}

	statuses := make([]PinStatus, 0, len(pins))
	for vocabRaw, expectedRaw := range pins {
		expected := strings.TrimSpace(expectedRaw)
		if expected == "" {
			continue
		}

		vocab := NormalizeVocabulary(vocabRaw)
		active, err := m.GetActiveRelease(ctx, vocab)
		if err != nil {
			return nil, err
		}

		ps := PinStatus{
			Vocabulary:      vocab,
			ExpectedVersion: expected,
		}

		if active != nil {
			ps.ActiveReleaseSet = true
			ps.ActiveVersion = active.Version
			ps.Match = active.Version == expected
		}

		statuses = append(statuses, ps)
	}

	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Vocabulary < statuses[j].Vocabulary })
	return statuses, nil
}
