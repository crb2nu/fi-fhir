package cda

import (
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

func TestMedicationsSectionParser_TemplateOID(t *testing.T) {
	parser := &MedicationsSectionParser{}
	if parser.TemplateOID() != TemplateSectionMedications {
		t.Errorf("Expected template OID %s, got %s", TemplateSectionMedications, parser.TemplateOID())
	}
}

func TestMedicationsSectionParser_Parse(t *testing.T) {
	xml := `<?xml version="1.0"?>
<section xmlns="urn:hl7-org:v3" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <templateId root="2.16.840.1.113883.10.20.22.2.1.1"/>
  <code code="10160-0" codeSystem="2.16.840.1.113883.6.1" displayName="Medications"/>
  <title>Medications</title>
  <entry>
    <substanceAdministration classCode="SBADM" moodCode="EVN">
      <templateId root="2.16.840.1.113883.10.20.22.4.16"/>
      <id root="MED-001"/>
      <statusCode code="active"/>
      <effectiveTime xsi:type="IVL_TS">
        <low value="20230101"/>
        <high value="20231231"/>
      </effectiveTime>
      <routeCode code="C38288" codeSystem="2.16.840.1.113883.3.26.1.1" displayName="Oral"/>
      <doseQuantity value="500" unit="mg"/>
      <consumable>
        <manufacturedProduct>
          <manufacturedMaterial>
            <code code="860975" codeSystem="2.16.840.1.113883.6.88" codeSystemName="RxNorm" displayName="Metformin 500 MG Oral Tablet"/>
          </manufacturedMaterial>
        </manufacturedProduct>
      </consumable>
    </substanceAdministration>
  </entry>
  <entry>
    <substanceAdministration classCode="SBADM" moodCode="INT">
      <templateId root="2.16.840.1.113883.10.20.22.4.16"/>
      <id root="MED-002"/>
      <statusCode code="completed"/>
      <effectiveTime value="20240115"/>
      <consumable>
        <manufacturedProduct>
          <manufacturedMaterial>
            <code code="197361" codeSystem="2.16.840.1.113883.6.88" displayName="Lisinopril 10 MG Oral Tablet"/>
          </manufacturedMaterial>
        </manufacturedProduct>
      </consumable>
      <entryRelationship>
        <supply>
          <id root="SUPPLY-001"/>
          <statusCode code="completed"/>
        </supply>
      </entryRelationship>
    </substanceAdministration>
  </entry>
</section>`

	parser := &MedicationsSectionParser{}
	section, err := parseTestSection(t, xml, parser)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(section.Entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(section.Entries))
	}

	// Check first medication (Metformin)
	med1 := section.Entries[0]
	if med1.Code.Code != "860975" {
		t.Errorf("Expected RxNorm code 860975, got %s", med1.Code.Code)
	}
	if med1.Code.DisplayName != "Metformin 500 MG Oral Tablet" {
		t.Errorf("Expected display name 'Metformin 500 MG Oral Tablet', got '%s'", med1.Code.DisplayName)
	}
	if med1.StatusCode != "active" {
		t.Errorf("Expected status active, got %s", med1.StatusCode)
	}
	if med1.Value == nil || med1.Value.Value != "500" || med1.Value.Unit != "mg" {
		t.Errorf("Expected dose 500 mg, got %+v", med1.Value)
	}
	if med1.MoodCode != "EVN" {
		t.Errorf("Expected mood code EVN, got %s", med1.MoodCode)
	}

	// Check second medication (Lisinopril) — has supply entry relationship
	med2 := section.Entries[1]
	if med2.Code.Code != "197361" {
		t.Errorf("Expected RxNorm code 197361, got %s", med2.Code.Code)
	}
	if med2.MoodCode != "INT" {
		t.Errorf("Expected mood code INT, got %s", med2.MoodCode)
	}
	if len(med2.EntryRelationships) != 1 {
		t.Fatalf("Expected 1 entry relationship, got %d", len(med2.EntryRelationships))
	}
	if med2.EntryRelationships[0].TypeCode != "supply" {
		t.Errorf("Expected supply relationship, got %s", med2.EntryRelationships[0].TypeCode)
	}
}

func TestMedicationsSectionMapper_MapSection(t *testing.T) {
	mapper := &MedicationsSectionMapper{}
	docTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	patient := &events.Patient{
		MRN:        "MRN12345",
		FamilyName: "Smith",
	}

	section := &Section{
		TemplateID: TemplateSectionMedications,
		Entries: []Entry{
			{
				ID:         "MED-001",
				TypeCode:   "substanceAdministration",
				StatusCode: "active",
				MoodCode:   "EVN",
				Code: CodedValue{
					Code:        "860975",
					CodeSystem:  CodeSystemRxNorm,
					DisplayName: "Metformin 500 MG",
				},
				Value: &EntryValue{Type: "PQ", Value: "500", Unit: "mg"},
				EffectiveTime: &TimeInterval{
					Low: timePtr(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
				},
			},
			{
				// Non-medication entry should be ignored
				ID:       "OTHER-001",
				TypeCode: "observation",
			},
		},
	}

	results, err := mapper.MapSection(section, patient, docTime)
	if err != nil {
		t.Fatalf("MapSection failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 medication event, got %d", len(results))
	}

	medEvt := results[0].(*events.MedicationRequestEvent)
	if medEvt.MedicationRequest.Medication.Code != "860975" {
		t.Errorf("Expected code 860975, got %s", medEvt.MedicationRequest.Medication.Code)
	}
	if medEvt.MedicationRequest.Medication.Name != "Metformin 500 MG" {
		t.Errorf("Expected name 'Metformin 500 MG', got '%s'", medEvt.MedicationRequest.Medication.Name)
	}
	if medEvt.MedicationRequest.Status != "active" {
		t.Errorf("Expected status 'active', got '%s'", medEvt.MedicationRequest.Status)
	}
	if medEvt.MedicationRequest.DoseQuantity != "500" {
		t.Errorf("Expected dose 500, got %s", medEvt.MedicationRequest.DoseQuantity)
	}
	if medEvt.MedicationRequest.DoseUnit != "mg" {
		t.Errorf("Expected dose unit mg, got %s", medEvt.MedicationRequest.DoseUnit)
	}
	if medEvt.MedicationRequest.AuthoredOn != "2023-01-01" {
		t.Errorf("Expected authored on 2023-01-01, got %s", medEvt.MedicationRequest.AuthoredOn)
	}
	if medEvt.MedicationRequest.Medication.CodeSystem != "http://www.nlm.nih.gov/research/umls/rxnorm" {
		t.Errorf("Expected FHIR RxNorm URI, got %s", medEvt.MedicationRequest.Medication.CodeSystem)
	}
	if medEvt.Type != events.EventMedicationRequest {
		t.Errorf("Expected event type medication_request, got %s", medEvt.Type)
	}
	if medEvt.SourceFormat != events.FormatCDA {
		t.Errorf("Expected source format cda, got %s", medEvt.SourceFormat)
	}
}

func TestMedicationsSectionMapper_MoodCodeMapping(t *testing.T) {
	mapper := &MedicationsSectionMapper{}
	docTime := time.Now()

	tests := []struct {
		moodCode   string
		wantIntent string
	}{
		{"INT", "plan"},
		{"RQO", "order"},
		{"PRMS", "order"},
		{"PRP", "proposal"},
		{"EVN", "order"}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.moodCode, func(t *testing.T) {
			section := &Section{
				Entries: []Entry{{
					TypeCode: "substanceAdministration",
					MoodCode: tt.moodCode,
					Code:     CodedValue{Code: "123"},
				}},
			}
			results, _ := mapper.MapSection(section, nil, docTime)
			if len(results) != 1 {
				t.Fatalf("Expected 1 result")
			}
			evt := results[0].(*events.MedicationRequestEvent)
			if evt.MedicationRequest.Intent != tt.wantIntent {
				t.Errorf("MoodCode %s: expected intent %s, got %s", tt.moodCode, tt.wantIntent, evt.MedicationRequest.Intent)
			}
		})
	}
}

func TestMedicationsSectionMapper_EmptySection(t *testing.T) {
	mapper := &MedicationsSectionMapper{}
	section := &Section{Entries: []Entry{}}
	results, err := mapper.MapSection(section, nil, time.Now())
	if err != nil {
		t.Fatalf("MapSection failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 events for empty section, got %d", len(results))
	}
}

// parseTestSection is a helper that parses XML and delegates to a SectionParser.
func parseTestSection(t *testing.T, xml string, parser SectionParser) (*Section, error) {
	t.Helper()
	p := NewParser("test", nil)
	p.RegisterSectionParser(parser)
	doc, err := p.Parse([]byte(wrapInCDADocument(xml)))
	if err != nil {
		return nil, err
	}
	if len(doc.Sections) == 0 {
		t.Fatal("Expected at least 1 section")
	}
	return &doc.Sections[0], nil
}

// wrapInCDADocument wraps a section in a minimal CDA document for parsing.
func wrapInCDADocument(sectionXML string) string {
	return `<?xml version="1.0"?>
<ClinicalDocument xmlns="urn:hl7-org:v3" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <id root="TEST-DOC"/>
  <effectiveTime value="20240115"/>
  <component>
    <structuredBody>
      <component>` + sectionXML + `</component>
    </structuredBody>
  </component>
</ClinicalDocument>`
}
