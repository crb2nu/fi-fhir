package cda

import (
	"os"
	"reflect"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

func TestMapperEventSelection(t *testing.T) {
	xml, err := os.ReadFile("../../../testdata/cda/section_selection.xml")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := NewParser("test", nil).Parse(xml)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		config *MapperConfig
		want   int
	}{
		{name: "nil defaults", want: 4},
		{name: "explicit zero disables all", config: &MapperConfig{}, want: 0},
		{name: "document only", config: &MapperConfig{EmitDocumentEvents: true}, want: 1},
		{name: "sections only", config: &MapperConfig{EmitSectionEvents: true}, want: 3},
		{name: "disabled global beats allowlist", config: &MapperConfig{SectionEvents: map[string]bool{TemplateSectionMedications: true}}, want: 0},
		{name: "explicit section exclusion", config: &MapperConfig{EmitSectionEvents: true, SectionEvents: map[string]bool{TemplateSectionMedications: false}}, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NewMapper(tc.config).Map(doc)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Events) != tc.want {
				t.Fatalf("got %d events, want %d", len(result.Events), tc.want)
			}
			if result.Patient == nil || result.Patient.MRN != "SYNTHETIC-001" {
				t.Fatal("patient lost")
			}
		})
	}
}

func TestMapperSelectionSnapshotsConfigAndSkipsDisabledMappers(t *testing.T) {
	selected := map[string]bool{"custom": true, "disabled": false}
	mapper := NewMapper(&MapperConfig{EmitSectionEvents: true, SectionEvents: selected})
	selected["custom"] = false
	selected["disabled"] = true
	custom := &selectionTestMapper{template: "custom"}
	disabled := &selectionTestMapper{template: "disabled"}
	omitted := &selectionTestMapper{template: "omitted"}
	mapper.RegisterSectionMapper(custom)
	mapper.RegisterSectionMapper(disabled)
	mapper.RegisterSectionMapper(omitted)
	doc := &CDADocument{Sections: []Section{{TemplateID: "custom"}, {TemplateID: "disabled"}, {TemplateID: "omitted"}}}
	before := append([]Section(nil), doc.Sections...)
	result, err := mapper.Map(doc)
	if err != nil {
		t.Fatal(err)
	}
	if custom.calls != 1 || disabled.calls != 0 || omitted.calls != 0 || len(result.Events) != 1 {
		t.Fatalf("mapper calls = %d/%d/%d, events = %d", custom.calls, disabled.calls, omitted.calls, len(result.Events))
	}
	if !reflect.DeepEqual(doc.Sections, before) {
		t.Fatal("selection mutated document sections")
	}
}

type selectionTestMapper struct {
	template string
	calls    int
}

func (m *selectionTestMapper) TemplateOID() string { return m.template }
func (m *selectionTestMapper) MapSection(*Section, *events.Patient, time.Time) ([]interface{}, error) {
	m.calls++
	return []interface{}{&events.DocumentEvent{}}, nil
}
