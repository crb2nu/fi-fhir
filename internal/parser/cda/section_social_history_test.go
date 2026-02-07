package cda

import (
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

func TestSocialHistorySectionParser_TemplateOID(t *testing.T) {
	parser := &SocialHistorySectionParser{}
	if parser.TemplateOID() != TemplateSectionSocialHistory {
		t.Errorf("Expected template OID %s, got %s", TemplateSectionSocialHistory, parser.TemplateOID())
	}
}

func TestSocialHistorySectionParser_Parse(t *testing.T) {
	xml := `<?xml version="1.0"?>
<section xmlns="urn:hl7-org:v3" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <templateId root="2.16.840.1.113883.10.20.22.2.17"/>
  <code code="29762-2" codeSystem="2.16.840.1.113883.6.1" displayName="Social History"/>
  <title>Social History</title>
  <entry>
    <observation classCode="OBS" moodCode="EVN">
      <templateId root="2.16.840.1.113883.10.20.22.4.78"/>
      <id root="SH-001"/>
      <code code="72166-2" codeSystem="2.16.840.1.113883.6.1" displayName="Tobacco smoking status"/>
      <statusCode code="completed"/>
      <effectiveTime value="20230601"/>
      <value xsi:type="CD" code="449868002" codeSystem="2.16.840.1.113883.6.96" displayName="Current every day smoker"/>
    </observation>
  </entry>
  <entry>
    <observation classCode="OBS" moodCode="EVN">
      <templateId root="2.16.840.1.113883.10.20.22.4.38"/>
      <id root="SH-002"/>
      <code code="74013-4" codeSystem="2.16.840.1.113883.6.1" displayName="Alcoholic drinks per day"/>
      <statusCode code="completed"/>
      <effectiveTime><low value="20200101"/></effectiveTime>
      <value xsi:type="ST">2-3 drinks per week</value>
    </observation>
  </entry>
</section>`

	parser := &SocialHistorySectionParser{}
	section, err := parseTestSection(t, xml, parser)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(section.Entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(section.Entries))
	}

	// Check smoking status entry
	smoking := section.Entries[0]
	if smoking.Code.Code != "72166-2" {
		t.Errorf("Expected LOINC code 72166-2, got %s", smoking.Code.Code)
	}
	if smoking.Text != "smoking-status" {
		t.Errorf("Expected category 'smoking-status', got '%s'", smoking.Text)
	}
	if smoking.Value == nil || smoking.Value.Code != "449868002" {
		t.Errorf("Expected value code 449868002, got %+v", smoking.Value)
	}

	// Check alcohol use entry
	alcohol := section.Entries[1]
	if alcohol.Code.Code != "74013-4" {
		t.Errorf("Expected LOINC code 74013-4, got %s", alcohol.Code.Code)
	}
	if alcohol.Text != "alcohol-use" {
		t.Errorf("Expected category 'alcohol-use', got '%s'", alcohol.Text)
	}
}

func TestSocialHistorySectionParser_SmokingStatusSpecialCase(t *testing.T) {
	xml := `<?xml version="1.0"?>
<section xmlns="urn:hl7-org:v3" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <templateId root="2.16.840.1.113883.10.20.22.2.17"/>
  <code code="29762-2" codeSystem="2.16.840.1.113883.6.1"/>
  <title>Social History</title>
  <entry>
    <observation classCode="OBS" moodCode="EVN">
      <templateId root="2.16.840.1.113883.10.20.22.4.78"/>
      <id root="SMOKING-001"/>
      <code code="72166-2" codeSystem="2.16.840.1.113883.6.1" displayName="Tobacco smoking status"/>
      <statusCode code="completed"/>
      <effectiveTime value="20240101"/>
      <value xsi:type="CD" code="266919005" codeSystem="2.16.840.1.113883.6.96" displayName="Never smoker"/>
    </observation>
  </entry>
</section>`

	parser := &SocialHistorySectionParser{}
	section, err := parseTestSection(t, xml, parser)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(section.Entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(section.Entries))
	}

	entry := section.Entries[0]
	// Should be categorized as smoking-status via template OID
	if entry.Text != "smoking-status" {
		t.Errorf("Expected category 'smoking-status', got '%s'", entry.Text)
	}
	if entry.Value.DisplayName != "Never smoker" {
		t.Errorf("Expected 'Never smoker', got '%s'", entry.Value.DisplayName)
	}
}

func TestSocialHistorySectionMapper_MapSection(t *testing.T) {
	mapper := &SocialHistorySectionMapper{}
	docTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	patient := &events.Patient{MRN: "MRN12345"}

	section := &Section{
		TemplateID: TemplateSectionSocialHistory,
		Entries: []Entry{
			{
				ID:         "SH-001",
				TypeCode:   "observation",
				StatusCode: "completed",
				Code: CodedValue{
					Code:        "72166-2",
					CodeSystem:  CodeSystemLOINC,
					DisplayName: "Tobacco smoking status",
				},
				Value: &EntryValue{
					Type:        "CD",
					Code:        "449868002",
					CodeSystem:  CodeSystemSNOMEDCT,
					DisplayName: "Current every day smoker",
				},
				EffectiveTime: &TimeInterval{
					Value: timePtr(time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)),
				},
				Text: "smoking-status",
			},
			{
				ID:         "SH-002",
				TypeCode:   "observation",
				StatusCode: "completed",
				Code: CodedValue{
					Code:        "74013-4",
					CodeSystem:  CodeSystemLOINC,
					DisplayName: "Alcoholic drinks per day",
				},
				Value: &EntryValue{
					Type:  "ST",
					Value: "2-3 per week",
				},
				Text: "alcohol-use",
			},
		},
	}

	results, err := mapper.MapSection(section, patient, docTime)
	if err != nil {
		t.Fatalf("MapSection failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 social history events, got %d", len(results))
	}

	// Check smoking status event
	smokingEvt := results[0].(*events.SocialHistoryEvent)
	if smokingEvt.Observation.Code != "72166-2" {
		t.Errorf("Expected code 72166-2, got %s", smokingEvt.Observation.Code)
	}
	if smokingEvt.Observation.CodeSystem != "http://loinc.org" {
		t.Errorf("Expected FHIR LOINC URI, got %s", smokingEvt.Observation.CodeSystem)
	}
	if smokingEvt.Observation.Value != "Current every day smoker" {
		t.Errorf("Expected 'Current every day smoker', got '%s'", smokingEvt.Observation.Value)
	}
	if smokingEvt.Observation.ValueCode != "449868002" {
		t.Errorf("Expected value code 449868002, got %s", smokingEvt.Observation.ValueCode)
	}
	if smokingEvt.Observation.Category != "smoking-status" {
		t.Errorf("Expected category 'smoking-status', got '%s'", smokingEvt.Observation.Category)
	}
	if smokingEvt.Observation.EffectiveDate != "2023-06-01" {
		t.Errorf("Expected effective date 2023-06-01, got %s", smokingEvt.Observation.EffectiveDate)
	}
	if smokingEvt.Type != events.EventSocialHistory {
		t.Errorf("Expected event type social_history, got %s", smokingEvt.Type)
	}

	// Check alcohol use event
	alcoholEvt := results[1].(*events.SocialHistoryEvent)
	if alcoholEvt.Observation.Value != "2-3 per week" {
		t.Errorf("Expected '2-3 per week', got '%s'", alcoholEvt.Observation.Value)
	}
	if alcoholEvt.Observation.Category != "alcohol-use" {
		t.Errorf("Expected category 'alcohol-use', got '%s'", alcoholEvt.Observation.Category)
	}
}

func TestSocialHistorySectionMapper_EmptySection(t *testing.T) {
	mapper := &SocialHistorySectionMapper{}
	section := &Section{Entries: []Entry{}}
	results, err := mapper.MapSection(section, nil, time.Now())
	if err != nil {
		t.Fatalf("MapSection failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 events, got %d", len(results))
	}
}

func TestInferSocialHistoryCategory(t *testing.T) {
	tests := []struct {
		name     string
		entry    Entry
		expected string
	}{
		{
			name: "SmokingStatusTemplate",
			entry: Entry{
				TemplateIDs: []string{TemplateEntrySmokingStatus},
				Code:        CodedValue{Code: "72166-2", CodeSystem: CodeSystemLOINC},
			},
			expected: "smoking-status",
		},
		{
			name: "TobaccoUseTemplate",
			entry: Entry{
				TemplateIDs: []string{TemplateEntryTobaccoUse},
			},
			expected: "tobacco-use",
		},
		{
			name: "AlcoholUseLOINC",
			entry: Entry{
				TemplateIDs: []string{TemplateEntrySocialHistoryObs},
				Code:        CodedValue{Code: "74013-4", CodeSystem: CodeSystemLOINC},
			},
			expected: "alcohol-use",
		},
		{
			name: "DrugUseLOINC",
			entry: Entry{
				Code: CodedValue{Code: "74204-9", CodeSystem: CodeSystemLOINC},
			},
			expected: "drug-use",
		},
		{
			name: "GenericSocialHistory",
			entry: Entry{
				Code: CodedValue{Code: "OTHER", CodeSystem: "OTHER"},
			},
			expected: "social-history",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferSocialHistoryCategory(&tt.entry)
			if result != tt.expected {
				t.Errorf("Expected category '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
