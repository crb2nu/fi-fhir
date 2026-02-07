package cda

import (
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

func TestAllergiesSectionParser_TemplateOID(t *testing.T) {
	parser := &AllergiesSectionParser{}
	if parser.TemplateOID() != TemplateSectionAllergies {
		t.Errorf("Expected template OID %s, got %s", TemplateSectionAllergies, parser.TemplateOID())
	}
}

func TestAllergiesSectionParser_Parse(t *testing.T) {
	xml := `<?xml version="1.0"?>
<section xmlns="urn:hl7-org:v3" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <templateId root="2.16.840.1.113883.10.20.22.2.6.1"/>
  <code code="48765-2" codeSystem="2.16.840.1.113883.6.1" displayName="Allergies"/>
  <title>Allergies and Adverse Reactions</title>
  <entry>
    <act classCode="ACT" moodCode="EVN">
      <templateId root="2.16.840.1.113883.10.20.22.4.30"/>
      <id root="ALLERGY-CONCERN-001"/>
      <statusCode code="active"/>
      <effectiveTime><low value="20200315"/></effectiveTime>
      <entryRelationship typeCode="SUBJ">
        <observation classCode="OBS" moodCode="EVN">
          <templateId root="2.16.840.1.113883.10.20.22.4.7"/>
          <id root="ALLERGY-OBS-001"/>
          <code code="ASSERTION" codeSystem="2.16.840.1.113883.5.4"/>
          <statusCode code="completed"/>
          <effectiveTime><low value="20200315"/></effectiveTime>
          <value xsi:type="CD" code="419511003" codeSystem="2.16.840.1.113883.6.96" displayName="Propensity to adverse reaction to drug"/>
          <participant typeCode="CSM">
            <participantRole classCode="MANU">
              <playingEntity classCode="MMAT">
                <code code="7980" codeSystem="2.16.840.1.113883.6.88" displayName="Penicillin G"/>
              </playingEntity>
            </participantRole>
          </participant>
          <entryRelationship typeCode="MFST">
            <observation classCode="OBS" moodCode="EVN">
              <templateId root="2.16.840.1.113883.10.20.22.4.9"/>
              <code code="ASSERTION"/>
              <value xsi:type="CD" code="247472004" codeSystem="2.16.840.1.113883.6.96" displayName="Hives"/>
            </observation>
          </entryRelationship>
          <entryRelationship typeCode="SUBJ">
            <observation classCode="OBS" moodCode="EVN">
              <templateId root="2.16.840.1.113883.10.20.22.4.8"/>
              <code code="SEV"/>
              <value xsi:type="CD" code="6736007" codeSystem="2.16.840.1.113883.6.96" displayName="Moderate"/>
            </observation>
          </entryRelationship>
        </observation>
      </entryRelationship>
    </act>
  </entry>
</section>`

	parser := &AllergiesSectionParser{}
	section, err := parseTestSection(t, xml, parser)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(section.Entries) != 1 {
		t.Fatalf("Expected 1 act entry, got %d", len(section.Entries))
	}

	act := section.Entries[0]
	if act.TypeCode != "act" {
		t.Errorf("Expected type code 'act', got '%s'", act.TypeCode)
	}
	if act.StatusCode != "active" {
		t.Errorf("Expected status 'active', got '%s'", act.StatusCode)
	}

	// Should have 1 observation entry relationship
	if len(act.EntryRelationships) != 1 {
		t.Fatalf("Expected 1 entry relationship, got %d", len(act.EntryRelationships))
	}

	obs := act.EntryRelationships[0]
	if obs.TypeCode != "observation" {
		t.Errorf("Expected observation, got %s", obs.TypeCode)
	}

	// Participant (allergen)
	if len(obs.Participants) != 1 {
		t.Fatalf("Expected 1 participant, got %d", len(obs.Participants))
	}
	allergen := obs.Participants[0].ParticipantRole.PlayingEntity
	if allergen.Code.Code != "7980" {
		t.Errorf("Expected allergen code 7980, got %s", allergen.Code.Code)
	}
	if allergen.Code.DisplayName != "Penicillin G" {
		t.Errorf("Expected 'Penicillin G', got '%s'", allergen.Code.DisplayName)
	}

	// Nested reactions + severity
	if len(obs.EntryRelationships) != 2 {
		t.Fatalf("Expected 2 nested relationships, got %d", len(obs.EntryRelationships))
	}
}

func TestAllergiesSectionParser_NoKnownAllergies(t *testing.T) {
	xml := `<?xml version="1.0"?>
<section xmlns="urn:hl7-org:v3" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <templateId root="2.16.840.1.113883.10.20.22.2.6.1"/>
  <code code="48765-2" codeSystem="2.16.840.1.113883.6.1"/>
  <title>Allergies</title>
  <entry>
    <act classCode="ACT" moodCode="EVN">
      <templateId root="2.16.840.1.113883.10.20.22.4.30"/>
      <id root="NKA-001"/>
      <statusCode code="active"/>
      <entryRelationship typeCode="SUBJ">
        <observation classCode="OBS" moodCode="EVN" negationInd="true">
          <templateId root="2.16.840.1.113883.10.20.22.4.7"/>
          <id root="NKA-OBS-001"/>
          <code code="ASSERTION" codeSystem="2.16.840.1.113883.5.4"/>
          <statusCode code="completed"/>
          <value xsi:type="CD" code="419199007" codeSystem="2.16.840.1.113883.6.96" displayName="Allergy to substance"/>
        </observation>
      </entryRelationship>
    </act>
  </entry>
</section>`

	parser := &AllergiesSectionParser{}
	section, err := parseTestSection(t, xml, parser)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(section.Entries) != 1 {
		t.Fatalf("Expected 1 act entry, got %d", len(section.Entries))
	}

	// The observation should be marked as negated
	obs := section.Entries[0].EntryRelationships[0]
	if obs.Text != "negated" {
		t.Errorf("Expected negated observation, got text '%s'", obs.Text)
	}
}

func TestAllergiesSectionMapper_MapSection(t *testing.T) {
	mapper := &AllergiesSectionMapper{}
	docTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	patient := &events.Patient{MRN: "MRN12345"}

	section := &Section{
		TemplateID: TemplateSectionAllergies,
		Entries: []Entry{
			{
				TypeCode:   "act",
				StatusCode: "active",
				EntryRelationships: []Entry{
					{
						ID:         "ALLERGY-001",
						TypeCode:   "observation",
						StatusCode: "completed",
						EffectiveTime: &TimeInterval{
							Low: timePtr(time.Date(2020, 3, 15, 0, 0, 0, 0, time.UTC)),
						},
						Participants: []Participant{
							{
								TypeCode: "CSM",
								ParticipantRole: &ParticipantRole{
									PlayingEntity: &PlayingEntity{
										Code: &CodedValue{
											Code:        "7980",
											CodeSystem:  CodeSystemRxNorm,
											DisplayName: "Penicillin G",
										},
									},
								},
							},
						},
						EntryRelationships: []Entry{
							{
								TypeCode: "observation",
								Text:     "MFST",
								Value: &EntryValue{
									Type:        "CD",
									Code:        "247472004",
									CodeSystem:  CodeSystemSNOMEDCT,
									DisplayName: "Hives",
								},
							},
							{
								TypeCode: "observation",
								Value: &EntryValue{
									Type:        "CD",
									Code:        "6736007",
									CodeSystem:  CodeSystemSNOMEDCT,
									DisplayName: "Moderate",
								},
							},
						},
					},
				},
			},
		},
	}

	results, err := mapper.MapSection(section, patient, docTime)
	if err != nil {
		t.Fatalf("MapSection failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 allergy event, got %d", len(results))
	}

	allergyEvt := results[0].(*events.AllergyIntoleranceEvent)
	if allergyEvt.AllergyIntolerance.Code != "7980" {
		t.Errorf("Expected code 7980, got %s", allergyEvt.AllergyIntolerance.Code)
	}
	if allergyEvt.AllergyIntolerance.Name != "Penicillin G" {
		t.Errorf("Expected name 'Penicillin G', got '%s'", allergyEvt.AllergyIntolerance.Name)
	}
	if allergyEvt.AllergyIntolerance.CodeSystem != "http://www.nlm.nih.gov/research/umls/rxnorm" {
		t.Errorf("Expected FHIR RxNorm URI, got %s", allergyEvt.AllergyIntolerance.CodeSystem)
	}
	if allergyEvt.AllergyIntolerance.ClinicalStatus != "active" {
		t.Errorf("Expected clinical status 'active', got '%s'", allergyEvt.AllergyIntolerance.ClinicalStatus)
	}
	if allergyEvt.AllergyIntolerance.OnsetDate != "2020-03-15" {
		t.Errorf("Expected onset date 2020-03-15, got %s", allergyEvt.AllergyIntolerance.OnsetDate)
	}
	if allergyEvt.Type != events.EventAllergyIntolerance {
		t.Errorf("Expected event type allergy_intolerance, got %s", allergyEvt.Type)
	}

	// Check reaction
	if len(allergyEvt.AllergyIntolerance.Reactions) != 1 {
		t.Fatalf("Expected 1 reaction, got %d", len(allergyEvt.AllergyIntolerance.Reactions))
	}
	reaction := allergyEvt.AllergyIntolerance.Reactions[0]
	if reaction.Manifestation != "247472004" {
		t.Errorf("Expected manifestation 247472004, got %s", reaction.Manifestation)
	}
	if reaction.ManifestationText != "Hives" {
		t.Errorf("Expected 'Hives', got '%s'", reaction.ManifestationText)
	}
	if reaction.Severity != "moderate" {
		t.Errorf("Expected severity 'moderate', got '%s'", reaction.Severity)
	}
}

func TestAllergiesSectionMapper_NegatedAllergy(t *testing.T) {
	mapper := &AllergiesSectionMapper{}
	docTime := time.Now()

	section := &Section{
		TemplateID: TemplateSectionAllergies,
		Entries: []Entry{
			{
				TypeCode:   "act",
				StatusCode: "active",
				EntryRelationships: []Entry{
					{
						ID:       "NKA-001",
						TypeCode: "observation",
						Text:     "negated", // No known allergies
					},
				},
			},
		},
	}

	results, err := mapper.MapSection(section, nil, docTime)
	if err != nil {
		t.Fatalf("MapSection failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 events for negated allergies, got %d", len(results))
	}
}

func TestAllergiesSectionMapper_EmptySection(t *testing.T) {
	mapper := &AllergiesSectionMapper{}
	section := &Section{Entries: []Entry{}}
	results, err := mapper.MapSection(section, nil, time.Now())
	if err != nil {
		t.Fatalf("MapSection failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 events, got %d", len(results))
	}
}
