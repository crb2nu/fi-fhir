package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseCDAProfileSelection(t *testing.T) {
	all := []string{"patient_summary", "medication_request", "allergy_intolerance", "social_history"}
	for _, tc := range []struct {
		name   string
		config string
		want   []string
	}{
		{name: "defaults", want: all},
		{name: "profile without CDA", config: "  version: 1.0.0\n", want: all},
		{name: "empty CDA config", config: "  cda: {}\n", want: all},
		{name: "selected sections", config: `  cda:
    emit_document_events: false
    sections:
      - template_id: "2.16.840.1.113883.10.20.22.2.1.1"
        emit_events: true
      - template_id: "2.16.840.1.113883.10.20.22.2.6.1"
        emit_events: false
`, want: []string{"medication_request"}},
		{name: "document only", config: "  cda:\n    emit_section_events: false\n", want: []string{"patient_summary"}},
		{name: "sections only", config: "  cda:\n    emit_document_events: false\n", want: all[1:]},
		{name: "all disabled", config: "  cda:\n    emit_document_events: false\n    emit_section_events: false\n", want: []string{}},
		{name: "empty selection preserves defaults", config: "  cda:\n    sections: []\n", want: all},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, format := range []string{"cda", "ccda"} {
				args := []string{"parse", "--format", format}
				if tc.config != "" {
					path := filepath.Join(t.TempDir(), "profile.yaml")
					if err := os.WriteFile(path, []byte("source_profile:\n  id: synthetic\n  name: Synthetic CDA\n"+tc.config), 0600); err != nil {
						t.Fatal(err)
					}
					args = append(args, "--profile", path)
				}
				args = append(args, testdataPath(t, "cda/section_selection.xml"))
				stdout, _, err := runCLI(t, args...)
				if err != nil {
					t.Fatal(err)
				}
				var output struct {
					Events []struct {
						Type string `json:"type"`
					}
					Document struct{ Sections []json.RawMessage }
					Patient  struct {
						MRN string `json:"mrn"`
					}
				}
				if err := json.Unmarshal([]byte(stdout), &output); err != nil {
					t.Fatal(err)
				}
				got := make([]string, 0, len(output.Events))
				for _, event := range output.Events {
					got = append(got, event.Type)
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Errorf("%s events = %v, want %v", format, got, tc.want)
				}
				if output.Events == nil {
					t.Error("events must be an array, even when disabled")
				}
				if len(output.Document.Sections) != 3 || output.Patient.MRN != "SYNTHETIC-001" {
					t.Error("event selection must preserve the parsed document and patient")
				}
			}
		})
	}
}
