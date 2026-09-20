package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCDAProfileValidationAndLint(t *testing.T) {
	for _, tc := range []struct {
		name        string
		config      string
		wantError   string
		wantWarning string
	}{
		{name: "valid", config: `    emit_document_events: false
    emit_section_events: true
    sections:
      - template_id: "custom-template"
        emit_events: true
`},
		{name: "empty ID", config: "    sections:\n      - template_id: ''\n", wantError: "template_id must be nonempty"},
		{name: "whitespace", config: "    sections:\n      - template_id: ' 1.2.3'\n", wantError: "surrounding whitespace"},
		{name: "duplicate", config: "    sections:\n      - template_id: '1.2.3'\n        emit_events: true\n      - template_id: '1.2.3'\n        emit_events: false\n", wantError: "duplicates"},
		{name: "unknown option", config: "    emit_doc_events: false\n", wantWarning: `unknown key "emit_doc_events" at source_profile.cda`},
		{name: "unknown section option", config: "    sections:\n      - template_id: '1.2.3'\n        emit_event: true\n", wantWarning: `unknown key "emit_event" at source_profile.cda.sections[0]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte("source_profile:\n  id: cda-test\n  name: CDA test\n  version: 1.0.0\n  cda:\n" + tc.config)
			_, err := NewRegistry().LoadFromBytes(data)
			if tc.wantError == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("load error = %v, want %q", err, tc.wantError)
			}
			path := filepath.Join(t.TempDir(), "profile.yaml")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			report, err := LintProfileFile(path, LintOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for _, check := range []struct {
				values []string
				want   string
			}{{report.Errors, tc.wantError}, {report.Warnings, tc.wantWarning}} {
				if check.want == "" && len(check.values) != 0 {
					t.Errorf("unexpected diagnostics: %v", check.values)
				}
				if check.want != "" && !strings.Contains(strings.Join(check.values, "\n"), check.want) {
					t.Errorf("diagnostics %v do not contain %q", check.values, check.want)
				}
			}
		})
	}
}

func TestCDAProfileSerializationPreservesExplicitFalse(t *testing.T) {
	p, err := NewRegistry().LoadFromBytes([]byte(`source_profile:
  id: cda-test
  name: CDA test
  cda:
    emit_document_events: false
    sections:
      - template_id: "1.2.3"
        emit_events: false
`))
	if err != nil {
		t.Fatal(err)
	}
	if p.CDA.EmitDocumentEvents == nil || *p.CDA.EmitDocumentEvents || p.CDA.EmitSectionEvents != nil {
		t.Fatalf("explicit false and omission were conflated: %+v", p.CDA)
	}
	rendered, err := MarshalYAML(p)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewRegistry().LoadFromBytes(rendered)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.CDA, p.CDA) {
		t.Fatal("profile YAML renderer lost CDA settings")
	}
	for _, codec := range []struct {
		name      string
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{{"json", json.Marshal, json.Unmarshal}, {"yaml", yaml.Marshal, yaml.Unmarshal}} {
		data, err := codec.marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		var restored SourceProfile
		if err := codec.unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(restored.CDA, p.CDA) {
			t.Errorf("%s changed CDA settings", codec.name)
		}
		legacy, err := codec.marshal(&SourceProfile{ID: "old", Name: "Legacy"})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(legacy), "cda") {
			t.Errorf("%s added CDA settings to a legacy profile", codec.name)
		}
	}
}
