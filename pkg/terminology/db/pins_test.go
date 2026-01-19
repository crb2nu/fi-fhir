package db

import "testing"

func TestNormalizeVocabulary(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"loinc", VocabLOINC},
		{"LOINC", VocabLOINC},
		{"snomed", VocabSNOMEDCT},
		{"SNOMEDCT_US", VocabSNOMEDCT},
		{"icd10cm", VocabICD10CM},
		{"ICD-10-CM", VocabICD10CM},
		{"icd10pcs", VocabICD10PCS},
		{"rxnorm", VocabRxNorm},
		{"umls", VocabUMLS},
		{"cpt", VocabCPT},
		{"hcpcs", VocabHCPCS},
		{"cvx", VocabCVX},
		{"ndc", VocabNDC},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := NormalizeVocabulary(tc.in); got != tc.want {
				t.Fatalf("NormalizeVocabulary(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
