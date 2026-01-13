package cda

import (
	"strings"
	"testing"
	"time"

	"github.com/antchfx/xmlquery"
)

// Sample CCDA CCD document for testing
const sampleCCDA = `<?xml version="1.0" encoding="UTF-8"?>
<ClinicalDocument xmlns="urn:hl7-org:v3" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <realmCode code="US"/>
  <typeId root="2.16.840.1.113883.1.3" extension="POCD_HD000040"/>
  <templateId root="2.16.840.1.113883.10.20.22.1.1"/>
  <templateId root="2.16.840.1.113883.10.20.22.1.2"/>
  <id root="2.16.840.1.113883.19.5.99999.1" extension="DOC-001"/>
  <code code="34133-9" codeSystem="2.16.840.1.113883.6.1" displayName="Summarization of Episode Note"/>
  <title>Continuity of Care Document</title>
  <effectiveTime value="20240115103000-0500"/>
  <confidentialityCode code="N" codeSystem="2.16.840.1.113883.5.25"/>
  <languageCode code="en-US"/>

  <recordTarget>
    <patientRole>
      <id root="2.16.840.1.113883.4.1" extension="123-45-6789"/>
      <id root="2.16.840.1.113883.19.5" extension="MRN12345"/>
      <addr use="HP">
        <streetAddressLine>123 Main Street</streetAddressLine>
        <city>Springfield</city>
        <state>IL</state>
        <postalCode>62701</postalCode>
        <country>US</country>
      </addr>
      <telecom use="HP" value="tel:+1-555-123-4567"/>
      <telecom use="MC" value="tel:+1-555-987-6543"/>
      <patient>
        <name use="L">
          <given>John</given>
          <given>Robert</given>
          <family>Smith</family>
          <suffix>Jr</suffix>
        </name>
        <administrativeGenderCode code="M" codeSystem="2.16.840.1.113883.5.1"/>
        <birthTime value="19850315"/>
        <maritalStatusCode code="M" codeSystem="2.16.840.1.113883.5.2" displayName="Married"/>
        <raceCode code="2106-3" codeSystem="2.16.840.1.113883.6.238" displayName="White"/>
        <ethnicGroupCode code="2186-5" codeSystem="2.16.840.1.113883.6.238" displayName="Not Hispanic or Latino"/>
        <languageCommunication>
          <languageCode code="en"/>
        </languageCommunication>
      </patient>
      <providerOrganization>
        <id root="2.16.840.1.113883.19.5.9999.1"/>
        <name>Springfield General Hospital</name>
        <telecom use="WP" value="tel:+1-555-555-1234"/>
        <addr>
          <streetAddressLine>1000 Hospital Drive</streetAddressLine>
          <city>Springfield</city>
          <state>IL</state>
          <postalCode>62702</postalCode>
        </addr>
      </providerOrganization>
    </patientRole>
  </recordTarget>

  <author>
    <time value="20240115103000"/>
    <assignedAuthor>
      <id root="2.16.840.1.113883.4.6" extension="1234567890"/>
      <addr>
        <streetAddressLine>1000 Hospital Drive</streetAddressLine>
        <city>Springfield</city>
        <state>IL</state>
        <postalCode>62702</postalCode>
      </addr>
      <telecom use="WP" value="tel:+1-555-555-5678"/>
      <assignedPerson>
        <name>
          <given>Jane</given>
          <family>Doe</family>
          <prefix>Dr.</prefix>
        </name>
      </assignedPerson>
      <representedOrganization>
        <id root="2.16.840.1.113883.19.5.9999.1"/>
        <name>Springfield General Hospital</name>
      </representedOrganization>
    </assignedAuthor>
  </author>

  <custodian>
    <assignedCustodian>
      <representedCustodianOrganization>
        <id root="2.16.840.1.113883.19.5.9999.1"/>
        <name>Springfield General Hospital</name>
        <telecom use="WP" value="tel:+1-555-555-1234"/>
        <addr>
          <streetAddressLine>1000 Hospital Drive</streetAddressLine>
          <city>Springfield</city>
          <state>IL</state>
          <postalCode>62702</postalCode>
        </addr>
      </representedCustodianOrganization>
    </assignedCustodian>
  </custodian>

  <documentationOf>
    <serviceEvent classCode="PCPR">
      <effectiveTime>
        <low value="20240101"/>
        <high value="20240115"/>
      </effectiveTime>
      <performer typeCode="PRF">
        <assignedEntity>
          <id root="2.16.840.1.113883.4.6" extension="1234567890"/>
          <assignedPerson>
            <name>
              <given>Jane</given>
              <family>Doe</family>
            </name>
          </assignedPerson>
        </assignedEntity>
      </performer>
    </serviceEvent>
  </documentationOf>

  <component>
    <structuredBody>
      <component>
        <section>
          <templateId root="2.16.840.1.113883.10.20.22.2.5.1"/>
          <code code="11450-4" codeSystem="2.16.840.1.113883.6.1" displayName="Problem List"/>
          <title>Problems</title>
          <text>
            <list>
              <item>Asthma - Active since 2012-08-06</item>
              <item>Type 2 Diabetes Mellitus - Active since 2018-03-20</item>
            </list>
          </text>
          <entry typeCode="DRIV">
            <act classCode="ACT" moodCode="EVN">
              <templateId root="2.16.840.1.113883.10.20.22.4.3"/>
              <id root="2.16.840.1.113883.19.5.99999.2" extension="PROB-001"/>
              <code code="CONC" codeSystem="2.16.840.1.113883.5.6"/>
              <statusCode code="active"/>
              <effectiveTime>
                <low value="20120806"/>
              </effectiveTime>
              <entryRelationship typeCode="SUBJ">
                <observation classCode="OBS" moodCode="EVN">
                  <templateId root="2.16.840.1.113883.10.20.22.4.4"/>
                  <id root="2.16.840.1.113883.19.5.99999.3" extension="OBS-001"/>
                  <code code="64572001" codeSystem="2.16.840.1.113883.6.96" displayName="Condition"/>
                  <statusCode code="completed"/>
                  <effectiveTime>
                    <low value="20120806"/>
                  </effectiveTime>
                  <value xsi:type="CD" code="195967001" codeSystem="2.16.840.1.113883.6.96" displayName="Asthma"/>
                </observation>
              </entryRelationship>
            </act>
          </entry>
          <entry typeCode="DRIV">
            <act classCode="ACT" moodCode="EVN">
              <templateId root="2.16.840.1.113883.10.20.22.4.3"/>
              <id root="2.16.840.1.113883.19.5.99999.2" extension="PROB-002"/>
              <code code="CONC" codeSystem="2.16.840.1.113883.5.6"/>
              <statusCode code="active"/>
              <effectiveTime>
                <low value="20180320"/>
              </effectiveTime>
              <entryRelationship typeCode="SUBJ">
                <observation classCode="OBS" moodCode="EVN">
                  <templateId root="2.16.840.1.113883.10.20.22.4.4"/>
                  <id root="2.16.840.1.113883.19.5.99999.3" extension="OBS-002"/>
                  <code code="64572001" codeSystem="2.16.840.1.113883.6.96" displayName="Condition"/>
                  <statusCode code="completed"/>
                  <effectiveTime>
                    <low value="20180320"/>
                  </effectiveTime>
                  <value xsi:type="CD" code="44054006" codeSystem="2.16.840.1.113883.6.96" displayName="Type 2 Diabetes Mellitus"/>
                </observation>
              </entryRelationship>
            </act>
          </entry>
        </section>
      </component>

      <component>
        <section>
          <templateId root="2.16.840.1.113883.10.20.22.2.3.1"/>
          <code code="30954-2" codeSystem="2.16.840.1.113883.6.1" displayName="Relevant diagnostic tests/laboratory data"/>
          <title>Results</title>
          <text>
            <table>
              <thead>
                <tr><th>Test</th><th>Value</th><th>Date</th></tr>
              </thead>
              <tbody>
                <tr><td>HbA1c</td><td>7.2 %</td><td>2024-01-10</td></tr>
              </tbody>
            </table>
          </text>
          <entry typeCode="DRIV">
            <organizer classCode="CLUSTER" moodCode="EVN">
              <templateId root="2.16.840.1.113883.10.20.22.4.1"/>
              <id root="2.16.840.1.113883.19.5.99999.4" extension="RES-001"/>
              <code code="57021-8" codeSystem="2.16.840.1.113883.6.1" displayName="CBC W Auto Differential panel"/>
              <statusCode code="completed"/>
              <effectiveTime value="20240110"/>
              <component>
                <observation classCode="OBS" moodCode="EVN">
                  <templateId root="2.16.840.1.113883.10.20.22.4.2"/>
                  <id root="2.16.840.1.113883.19.5.99999.5" extension="OBS-003"/>
                  <code code="4548-4" codeSystem="2.16.840.1.113883.6.1" displayName="Hemoglobin A1c/Hemoglobin.total in Blood"/>
                  <statusCode code="completed"/>
                  <effectiveTime value="20240110"/>
                  <value xsi:type="PQ" value="7.2" unit="%"/>
                  <interpretationCode code="H" codeSystem="2.16.840.1.113883.5.83" displayName="High"/>
                </observation>
              </component>
            </organizer>
          </entry>
        </section>
      </component>

      <component>
        <section>
          <templateId root="2.16.840.1.113883.10.20.22.2.4.1"/>
          <code code="8716-3" codeSystem="2.16.840.1.113883.6.1" displayName="Vital Signs"/>
          <title>Vital Signs</title>
          <entry typeCode="DRIV">
            <organizer classCode="CLUSTER" moodCode="EVN">
              <templateId root="2.16.840.1.113883.10.20.22.4.26"/>
              <id root="2.16.840.1.113883.19.5.99999.6" extension="VS-001"/>
              <code code="46680005" codeSystem="2.16.840.1.113883.6.96" displayName="Vital Signs"/>
              <statusCode code="completed"/>
              <effectiveTime value="20240115"/>
              <component>
                <observation classCode="OBS" moodCode="EVN">
                  <templateId root="2.16.840.1.113883.10.20.22.4.27"/>
                  <id root="2.16.840.1.113883.19.5.99999.7"/>
                  <code code="8480-6" codeSystem="2.16.840.1.113883.6.1" displayName="Systolic Blood Pressure"/>
                  <statusCode code="completed"/>
                  <effectiveTime value="20240115"/>
                  <value xsi:type="PQ" value="120" unit="mm[Hg]"/>
                </observation>
              </component>
              <component>
                <observation classCode="OBS" moodCode="EVN">
                  <templateId root="2.16.840.1.113883.10.20.22.4.27"/>
                  <id root="2.16.840.1.113883.19.5.99999.8"/>
                  <code code="8462-4" codeSystem="2.16.840.1.113883.6.1" displayName="Diastolic Blood Pressure"/>
                  <statusCode code="completed"/>
                  <effectiveTime value="20240115"/>
                  <value xsi:type="PQ" value="80" unit="mm[Hg]"/>
                </observation>
              </component>
            </organizer>
          </entry>
        </section>
      </component>
    </structuredBody>
  </component>
</ClinicalDocument>`

func TestParser_Parse(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if doc == nil {
		t.Fatal("Expected document, got nil")
	}

	// Verify document ID
	if doc.ID != "DOC-001" {
		t.Errorf("Expected ID DOC-001, got %s", doc.ID)
	}

	// Verify template IDs
	if len(doc.TemplateIDs) != 2 {
		t.Errorf("Expected 2 template IDs, got %d", len(doc.TemplateIDs))
	}
	if doc.TemplateIDs[1] != TemplateOIDCCD {
		t.Errorf("Expected template ID %s, got %s", TemplateOIDCCD, doc.TemplateIDs[1])
	}

	// Verify document type
	if doc.TypeCode.Code != "34133-9" {
		t.Errorf("Expected type code 34133-9, got %s", doc.TypeCode.Code)
	}

	// Verify title
	if doc.Title != "Continuity of Care Document" {
		t.Errorf("Expected title 'Continuity of Care Document', got '%s'", doc.Title)
	}

	// Verify effective time
	expectedDate := time.Date(2024, 1, 15, 10, 30, 0, 0, time.FixedZone("", -5*3600))
	if !doc.EffectiveTime.Equal(expectedDate) {
		t.Errorf("Expected effective time %v, got %v", expectedDate, doc.EffectiveTime)
	}

	// Verify confidentiality
	if doc.ConfidentialityCode != "N" {
		t.Errorf("Expected confidentiality N, got %s", doc.ConfidentialityCode)
	}

	// Verify language
	if doc.LanguageCode != "en-US" {
		t.Errorf("Expected language en-US, got %s", doc.LanguageCode)
	}
}

func TestParser_ParsePatient(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if doc.Patient == nil {
		t.Fatal("Expected patient, got nil")
	}

	// Verify identifiers
	if len(doc.Patient.IDs) != 2 {
		t.Errorf("Expected 2 patient IDs, got %d", len(doc.Patient.IDs))
	}
	if doc.Patient.IDs[1].Extension != "MRN12345" {
		t.Errorf("Expected MRN MRN12345, got %s", doc.Patient.IDs[1].Extension)
	}

	// Verify address
	if len(doc.Patient.Addresses) != 1 {
		t.Errorf("Expected 1 address, got %d", len(doc.Patient.Addresses))
	}
	addr := doc.Patient.Addresses[0]
	if addr.City != "Springfield" {
		t.Errorf("Expected city Springfield, got %s", addr.City)
	}
	if addr.State != "IL" {
		t.Errorf("Expected state IL, got %s", addr.State)
	}

	// Verify telecoms
	if len(doc.Patient.Telecoms) != 2 {
		t.Errorf("Expected 2 telecoms, got %d", len(doc.Patient.Telecoms))
	}

	// Verify patient info
	if doc.Patient.Patient == nil {
		t.Fatal("Expected patient info, got nil")
	}

	info := doc.Patient.Patient

	// Verify name
	if len(info.Names) != 1 {
		t.Errorf("Expected 1 name, got %d", len(info.Names))
	}
	name := info.Names[0]
	if name.Family != "Smith" {
		t.Errorf("Expected family name Smith, got %s", name.Family)
	}
	if len(name.Given) != 2 || name.Given[0] != "John" {
		t.Errorf("Expected given names [John, Robert], got %v", name.Given)
	}
	if len(name.Suffix) != 1 || name.Suffix[0] != "Jr" {
		t.Errorf("Expected suffix [Jr], got %v", name.Suffix)
	}

	// Verify full name
	fullName := name.FullName()
	if !strings.Contains(fullName, "John") || !strings.Contains(fullName, "Smith") {
		t.Errorf("FullName should contain John and Smith, got %s", fullName)
	}

	// Verify gender
	if info.Gender != "M" {
		t.Errorf("Expected gender M, got %s", info.Gender)
	}

	// Verify birth date
	if info.BirthTime == nil {
		t.Fatal("Expected birth time, got nil")
	}
	if info.BirthTime.Year() != 1985 || info.BirthTime.Month() != 3 || info.BirthTime.Day() != 15 {
		t.Errorf("Expected birth date 1985-03-15, got %v", info.BirthTime)
	}

	// Verify language
	if info.LanguageCode != "en" {
		t.Errorf("Expected language en, got %s", info.LanguageCode)
	}
}

func TestParser_ParseAuthor(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if doc.Author == nil {
		t.Fatal("Expected author, got nil")
	}

	if doc.Author.AssignedAuthor == nil {
		t.Fatal("Expected assigned author, got nil")
	}

	aa := doc.Author.AssignedAuthor

	// Verify NPI
	if len(aa.IDs) != 1 {
		t.Errorf("Expected 1 author ID, got %d", len(aa.IDs))
	}
	if aa.IDs[0].Root != IdentifierSystemNPI {
		t.Errorf("Expected NPI root, got %s", aa.IDs[0].Root)
	}
	if aa.IDs[0].Extension != "1234567890" {
		t.Errorf("Expected NPI 1234567890, got %s", aa.IDs[0].Extension)
	}

	// Verify author name
	if aa.Person == nil || len(aa.Person.Names) == 0 {
		t.Fatal("Expected author person with name")
	}
	if aa.Person.Names[0].Family != "Doe" {
		t.Errorf("Expected author family name Doe, got %s", aa.Person.Names[0].Family)
	}

	// Verify organization
	if aa.Organization == nil {
		t.Fatal("Expected author organization")
	}
	if len(aa.Organization.Names) == 0 || aa.Organization.Names[0] != "Springfield General Hospital" {
		t.Errorf("Expected org name 'Springfield General Hospital', got %v", aa.Organization.Names)
	}
}

func TestParser_ParseCustodian(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if doc.Custodian == nil {
		t.Fatal("Expected custodian, got nil")
	}

	if doc.Custodian.Organization == nil {
		t.Fatal("Expected custodian organization, got nil")
	}

	org := doc.Custodian.Organization
	if len(org.Names) == 0 || org.Names[0] != "Springfield General Hospital" {
		t.Errorf("Expected org name 'Springfield General Hospital', got %v", org.Names)
	}
}

func TestParser_ParseServiceEvent(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if doc.ServiceEvent == nil {
		t.Fatal("Expected service event, got nil")
	}

	se := doc.ServiceEvent

	// Verify class code
	if se.ClassCode != "PCPR" {
		t.Errorf("Expected class code PCPR, got %s", se.ClassCode)
	}

	// Verify effective time
	if se.EffectiveTime == nil {
		t.Fatal("Expected effective time")
	}
	if se.EffectiveTime.Low == nil {
		t.Fatal("Expected effective time low")
	}
	if se.EffectiveTime.Low.Year() != 2024 || se.EffectiveTime.Low.Month() != 1 || se.EffectiveTime.Low.Day() != 1 {
		t.Errorf("Expected low date 2024-01-01, got %v", se.EffectiveTime.Low)
	}

	// Verify performers
	if len(se.Performers) != 1 {
		t.Errorf("Expected 1 performer, got %d", len(se.Performers))
	}
}

func TestParser_ParseSections(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should have 3 sections
	if len(doc.Sections) != 3 {
		t.Errorf("Expected 3 sections, got %d", len(doc.Sections))
	}

	// Verify Problems section
	problems := doc.FindSection(TemplateSectionProblems)
	if problems == nil {
		t.Fatal("Expected Problems section")
	}
	if problems.Title != "Problems" {
		t.Errorf("Expected title 'Problems', got '%s'", problems.Title)
	}
	if problems.Code.Code != "11450-4" {
		t.Errorf("Expected code 11450-4, got %s", problems.Code.Code)
	}
	if len(problems.Entries) != 2 {
		t.Errorf("Expected 2 problem entries, got %d", len(problems.Entries))
	}

	// Verify Results section
	results := doc.FindSection(TemplateSectionResults)
	if results == nil {
		t.Fatal("Expected Results section")
	}
	if len(results.Entries) != 1 {
		t.Errorf("Expected 1 result entry, got %d", len(results.Entries))
	}

	// Verify Vital Signs section
	vitals := doc.FindSection(TemplateSectionVitalSigns)
	if vitals == nil {
		t.Fatal("Expected Vital Signs section")
	}
	if len(vitals.Entries) != 1 {
		t.Errorf("Expected 1 vital signs entry, got %d", len(vitals.Entries))
	}
}

func TestParser_ParseProblemsEntry(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	problems := doc.FindSection(TemplateSectionProblems)
	if problems == nil || len(problems.Entries) == 0 {
		t.Fatal("Expected Problems section with entries")
	}

	// First entry is a Problem Concern Act
	entry := problems.Entries[0]
	if entry.TypeCode != "act" {
		t.Errorf("Expected type code 'act', got '%s'", entry.TypeCode)
	}
	if entry.ClassCode != "ACT" {
		t.Errorf("Expected class code 'ACT', got '%s'", entry.ClassCode)
	}
	if entry.StatusCode != "active" {
		t.Errorf("Expected status 'active', got '%s'", entry.StatusCode)
	}

	// Check for template ID
	found := false
	for _, tmpl := range entry.TemplateIDs {
		if tmpl == TemplateEntryProblemConcern {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected Problem Concern Act template ID")
	}

	// Check nested observation (entryRelationship)
	if len(entry.EntryRelationships) == 0 {
		t.Fatal("Expected entry relationships")
	}

	obs := entry.EntryRelationships[0]
	if obs.TypeCode != "observation" {
		t.Errorf("Expected type 'observation', got '%s'", obs.TypeCode)
	}

	// Check observation value (the actual condition)
	if obs.Value == nil {
		t.Fatal("Expected observation value")
	}
	if obs.Value.Code != "195967001" {
		t.Errorf("Expected SNOMED code 195967001, got %s", obs.Value.Code)
	}
	if obs.Value.DisplayName != "Asthma" {
		t.Errorf("Expected display name 'Asthma', got '%s'", obs.Value.DisplayName)
	}
}

func TestParser_ParseResultsEntry(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	results := doc.FindSection(TemplateSectionResults)
	if results == nil || len(results.Entries) == 0 {
		t.Fatal("Expected Results section with entries")
	}

	// First entry is a Result Organizer
	organizer := results.Entries[0]
	if organizer.TypeCode != "organizer" {
		t.Errorf("Expected type 'organizer', got '%s'", organizer.TypeCode)
	}

	// Check component observations
	if len(organizer.EntryRelationships) == 0 {
		t.Fatal("Expected organizer components")
	}

	// First component is HbA1c
	obs := organizer.EntryRelationships[0]
	if obs.Code.Code != "4548-4" {
		t.Errorf("Expected LOINC code 4548-4, got %s", obs.Code.Code)
	}

	// Check value
	if obs.Value == nil {
		t.Fatal("Expected observation value")
	}
	if obs.Value.Type != "PQ" {
		t.Errorf("Expected value type PQ, got %s", obs.Value.Type)
	}
	if obs.Value.Value != "7.2" {
		t.Errorf("Expected value 7.2, got %s", obs.Value.Value)
	}
	if obs.Value.Unit != "%" {
		t.Errorf("Expected unit %%, got %s", obs.Value.Unit)
	}
}

func TestParser_ParseVitalSignsEntry(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	vitals := doc.FindSection(TemplateSectionVitalSigns)
	if vitals == nil || len(vitals.Entries) == 0 {
		t.Fatal("Expected Vital Signs section with entries")
	}

	// First entry is a Vital Signs Organizer
	organizer := vitals.Entries[0]
	if organizer.TypeCode != "organizer" {
		t.Errorf("Expected type 'organizer', got '%s'", organizer.TypeCode)
	}

	// Should have 2 component observations (systolic and diastolic BP)
	if len(organizer.EntryRelationships) != 2 {
		t.Errorf("Expected 2 vital sign observations, got %d", len(organizer.EntryRelationships))
	}

	// Check systolic BP
	systolic := organizer.EntryRelationships[0]
	if systolic.Code.Code != "8480-6" {
		t.Errorf("Expected LOINC code 8480-6, got %s", systolic.Code.Code)
	}
	if systolic.Value == nil || systolic.Value.Value != "120" {
		t.Errorf("Expected systolic value 120, got %v", systolic.Value)
	}

	// Check diastolic BP
	diastolic := organizer.EntryRelationships[1]
	if diastolic.Code.Code != "8462-4" {
		t.Errorf("Expected LOINC code 8462-4, got %s", diastolic.Code.Code)
	}
	if diastolic.Value == nil || diastolic.Value.Value != "80" {
		t.Errorf("Expected diastolic value 80, got %v", diastolic.Value)
	}
}

func TestParser_ParseWithResult(t *testing.T) {
	parser := NewParser("test", &ParserConfig{
		ExtractNarrative: true,
	})

	result, err := parser.ParseWithResult([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("ParseWithResult failed: %v", err)
	}

	// Should have no warnings for valid CCDA
	if len(result.Warnings) != 0 {
		t.Errorf("Expected no warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}

	// Should have document
	if result.Document == nil {
		t.Fatal("Expected document")
	}

	// Check narrative extraction
	problems := result.Document.FindSection(TemplateSectionProblems)
	if problems == nil {
		t.Fatal("Expected Problems section")
	}
	if problems.Text == "" {
		t.Error("Expected narrative text when ExtractNarrative is true")
	}
}

func TestParser_InvalidXML(t *testing.T) {
	parser := NewParser("test", nil)
	_, err := parser.Parse([]byte("not valid xml"))
	if err == nil {
		t.Error("Expected error for invalid XML")
	}
}

func TestParser_MissingClinicalDocument(t *testing.T) {
	parser := NewParser("test", nil)
	_, err := parser.Parse([]byte(`<?xml version="1.0"?><Root><Child/></Root>`))
	if err == nil {
		t.Error("Expected error for missing ClinicalDocument")
	}
}

func TestParser_DocumentType(t *testing.T) {
	parser := NewParser("test", nil)
	doc, _ := parser.Parse([]byte(sampleCCDA))

	docType := doc.DocumentType()
	if docType != TemplateOIDUSRealmHeader {
		t.Errorf("Expected first template ID %s, got %s", TemplateOIDUSRealmHeader, docType)
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		input    string
		expected string // YYYY-MM-DD format for comparison
	}{
		{"20240115", "2024-01-15"},
		{"20240115103000", "2024-01-15"},
		{"202401", "2024-01-01"},
		{"2024", "2024-01-01"},
		{"", ""},
	}

	for _, tt := range tests {
		result := parseTimestamp(tt.input)
		if tt.expected == "" {
			if result != nil {
				t.Errorf("parseTimestamp(%q) = %v, expected nil", tt.input, result)
			}
		} else {
			if result == nil {
				t.Errorf("parseTimestamp(%q) = nil, expected %s", tt.input, tt.expected)
				continue
			}
			formatted := result.Format("2006-01-02")
			if formatted != tt.expected {
				t.Errorf("parseTimestamp(%q) = %s, expected %s", tt.input, formatted, tt.expected)
			}
		}
	}
}

func TestCodedValue_IsNull(t *testing.T) {
	cv1 := CodedValue{Code: "123", CodeSystem: "2.16.840.1.113883.6.96"}
	if cv1.IsNull() {
		t.Error("Expected IsNull to return false for coded value with code")
	}

	cv2 := CodedValue{NullFlavor: "NI"}
	if !cv2.IsNull() {
		t.Error("Expected IsNull to return true for coded value with null flavor")
	}
}

func TestOIDToFHIRSystem(t *testing.T) {
	tests := []struct {
		oid      string
		expected string
	}{
		{CodeSystemSNOMEDCT, "http://snomed.info/sct"},
		{CodeSystemLOINC, "http://loinc.org"},
		{CodeSystemRxNorm, "http://www.nlm.nih.gov/research/umls/rxnorm"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		result := OIDToFHIRSystem[tt.oid]
		if result != tt.expected {
			t.Errorf("OIDToFHIRSystem[%q] = %q, expected %q", tt.oid, result, tt.expected)
		}
	}
}

// mockSectionParser implements SectionParser for testing
type mockSectionParser struct {
	templateOID string
	called      bool
}

func (m *mockSectionParser) TemplateOID() string {
	return m.templateOID
}

func (m *mockSectionParser) Parse(section *xmlquery.Node, config *ParserConfig) (*Section, error) {
	m.called = true
	return &Section{}, nil
}

func TestParser_RegisterSectionParser(t *testing.T) {
	parser := NewParser("test", nil)

	// Register a custom section parser
	mock := &mockSectionParser{templateOID: "2.16.840.1.113883.10.20.22.2.999"}
	parser.RegisterSectionParser(mock)

	// Parse a document - the custom parser won't be called since the template doesn't exist
	// but this ensures RegisterSectionParser works
	_, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// The custom parser shouldn't be called for the sample document
	// since none of the sections have that template ID
	if mock.called {
		t.Error("Custom parser should not have been called for sections without matching template")
	}
}

// Test CDA document with authenticator
const cdaWithAuthenticator = `<?xml version="1.0" encoding="UTF-8"?>
<ClinicalDocument xmlns="urn:hl7-org:v3" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <typeId root="2.16.840.1.113883.1.3" extension="POCD_HD000040"/>
  <id root="2.16.840.1.113883.19.5" extension="DOC-002"/>
  <code code="34133-9" codeSystem="2.16.840.1.113883.6.1"/>
  <effectiveTime value="20240115"/>
  <confidentialityCode code="N" codeSystem="2.16.840.1.113883.5.25"/>

  <recordTarget>
    <patientRole>
      <id root="2.16.840.1.113883.19.5" extension="MRN001"/>
      <patient>
        <name><family>Test</family></name>
      </patient>
    </patientRole>
  </recordTarget>

  <authenticator>
    <time value="20240115120000"/>
    <signatureCode code="S"/>
    <assignedEntity>
      <id root="2.16.840.1.113883.4.6" extension="9876543210"/>
      <assignedPerson>
        <name><given>Auth</given><family>User</family></name>
      </assignedPerson>
    </assignedEntity>
  </authenticator>

  <informant>
    <assignedEntity>
      <id root="2.16.840.1.113883.19.5" extension="INF001"/>
      <assignedPerson>
        <name><given>Info</given><family>Person</family></name>
      </assignedPerson>
    </assignedEntity>
  </informant>

  <participant typeCode="IND">
    <associatedEntity classCode="NOK">
      <code code="MTH" displayName="Mother" codeSystem="2.16.840.1.113883.5.111"/>
      <associatedPerson>
        <name><given>Jane</given><family>Mother</family></name>
      </associatedPerson>
    </associatedEntity>
  </participant>

  <documentationOf>
    <serviceEvent classCode="PCPR">
      <effectiveTime><low value="20240101"/><high value="20240115"/></effectiveTime>
    </serviceEvent>
  </documentationOf>

  <componentOf>
    <encompassingEncounter>
      <id root="2.16.840.1.113883.19.5" extension="ENC001"/>
      <code code="IMP" codeSystem="2.16.840.1.113883.5.4" displayName="Inpatient"/>
      <effectiveTime><low value="20240110"/><high value="20240115"/></effectiveTime>
      <location>
        <healthCareFacility>
          <id root="2.16.840.1.113883.19.5" extension="FAC001"/>
          <code code="1160-1" codeSystem="2.16.840.1.113883.6.259" displayName="Hospital"/>
        </healthCareFacility>
      </location>
    </encompassingEncounter>
  </componentOf>

  <component><structuredBody><component>
    <section>
      <templateId root="2.16.840.1.113883.10.20.22.2.5.1"/>
      <code code="11450-4" codeSystem="2.16.840.1.113883.6.1"/>
      <title>Problems</title>
    </section>
  </component></structuredBody></component>
</ClinicalDocument>`

func TestParser_ParseAuthenticator(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(cdaWithAuthenticator))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if doc.Authenticator == nil {
		t.Fatal("Expected authenticator, got nil")
	}

	if doc.Authenticator.SignatureCode != "S" {
		t.Errorf("Expected signature code S, got %s", doc.Authenticator.SignatureCode)
	}

	if doc.Authenticator.AssignedEntity == nil {
		t.Fatal("Expected assigned entity")
	}
}

func TestParser_ParseInformant(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(cdaWithAuthenticator))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(doc.InformantList) == 0 {
		t.Fatal("Expected informants, got none")
	}

	if doc.InformantList[0].AssignedEntity == nil {
		t.Fatal("Expected assigned entity")
	}
}

func TestParser_ParseEncompassingEncounter(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(cdaWithAuthenticator))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if doc.EncompassingEncounter == nil {
		t.Fatal("Expected encompassing encounter, got nil")
	}

	enc := doc.EncompassingEncounter
	if len(enc.IDs) == 0 || enc.IDs[0].Extension != "ENC001" {
		t.Errorf("Expected encounter ID ENC001, got %v", enc.IDs)
	}

	if enc.Code == nil || enc.Code.Code != "IMP" {
		t.Error("Expected encounter code IMP")
	}
}

func TestParseEntryValue_InferAndTypes(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		validate func(t *testing.T, ev *EntryValue)
	}{
		{
			name: "ExplicitPQ",
			xml:  `<value xsi:type="PQ" value="7.2" unit="%"/>`,
			validate: func(t *testing.T, ev *EntryValue) {
				if ev.Type != "PQ" || ev.Value != "7.2" || ev.Unit != "%" {
					t.Fatalf("Expected PQ with value 7.2%%, got %+v", ev)
				}
			},
		},
		{
			name: "InferCDFromCode",
			xml:  `<value code="12345-6" codeSystem="2.16.840.1.113883.6.1" displayName="Test Code"/>`,
			validate: func(t *testing.T, ev *EntryValue) {
				if ev.Type != "CD" || ev.Code != "12345-6" || ev.CodeSystem != "2.16.840.1.113883.6.1" {
					t.Fatalf("Expected inferred CD with code 12345-6, got %+v", ev)
				}
			},
		},
		{
			name: "InferPQFromUnit",
			xml:  `<value value="1.2" unit="mg"/>`,
			validate: func(t *testing.T, ev *EntryValue) {
				if ev.Type != "PQ" || ev.Value != "1.2" || ev.Unit != "mg" {
					t.Fatalf("Expected inferred PQ with value 1.2 mg, got %+v", ev)
				}
			},
		},
		{
			name: "InferIVLPQFromBounds",
			xml:  `<value><low value="4.0" unit="mmol/L"/><high value="6.0" unit="mmol/L"/></value>`,
			validate: func(t *testing.T, ev *EntryValue) {
				if ev.Type != "IVL_PQ" || ev.Low != "4.0" || ev.High != "6.0" || ev.Unit != "mmol/L" {
					t.Fatalf("Expected IVL_PQ with bounds 4.0-6.0 mmol/L, got %+v", ev)
				}
			},
		},
		{
			name: "InferIVLTSFromBounds",
			xml:  `<value><low value="20240101"/><high value="20240131"/></value>`,
			validate: func(t *testing.T, ev *EntryValue) {
				if ev.Type != "IVL_TS" || ev.Low != "20240101" || ev.High != "20240131" {
					t.Fatalf("Expected IVL_TS with low/high set, got %+v", ev)
				}
			},
		},
		{
			name: "InferSTFromValueAttr",
			xml:  `<value value="text-value">text-value</value>`,
			validate: func(t *testing.T, ev *EntryValue) {
				if ev.Type != "ST" || ev.Value != "text-value" {
					t.Fatalf("Expected ST with value 'text-value', got %+v", ev)
				}
			},
		},
		{
			name: "DefaultInnerText",
			xml:  `<value> note content </value>`,
			validate: func(t *testing.T, ev *EntryValue) {
				if ev.Type != "ST" || ev.Value != "note content" {
					t.Fatalf("Expected ST with inner text, got %+v", ev)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := xmlquery.Parse(strings.NewReader(`<root xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` + tt.xml + `</root>`))
			if err != nil {
				t.Fatalf("failed to parse xml: %v", err)
			}

			node := xmlquery.FindOne(doc, "//value")
			if node == nil {
				t.Fatal("value node not found")
			}

			ev := parseEntryValue(node)
			tt.validate(t, ev)
		})
	}
}

func TestParser_ParseParticipant(t *testing.T) {
	parser := NewParser("test", nil)
	xml := `
<participant typeCode="IND" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <time value="20240101123000"/>
  <participantRole classCode="ROL">
    <id root="1.2.3" extension="PART-123"/>
    <addr use="HP">
      <streetAddressLine>1 Main St</streetAddressLine>
      <city>Metropolis</city>
      <state>NY</state>
      <postalCode>10001</postalCode>
      <country>US</country>
    </addr>
    <telecom use="HP" value="tel:+15551234567"/>
    <playingEntity classCode="PSN">
      <code code="407543006" codeSystem="2.16.840.1.113883.6.96" displayName="Spouse"/>
      <name>Jane Doe</name>
    </playingEntity>
  </participantRole>
</participant>`

	doc, err := xmlquery.Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("failed to parse xml: %v", err)
	}

	node := xmlquery.FindOne(doc, "//participant")
	part := parser.parseParticipant(node)
	if part == nil {
		t.Fatal("expected participant")
	}

	if part.TypeCode != "IND" {
		t.Errorf("expected typeCode IND, got %s", part.TypeCode)
	}
	if part.Time == nil || part.Time.Value == nil || part.Time.Value.Format("20060102150405") != "20240101123000" {
		t.Fatalf("unexpected participant time: %+v", part.Time)
	}
	if part.ParticipantRole == nil || len(part.ParticipantRole.IDs) != 1 || part.ParticipantRole.IDs[0].Extension != "PART-123" {
		t.Fatalf("expected participant role id PART-123, got %+v", part.ParticipantRole)
	}
	if part.ParticipantRole.PlayingEntity == nil || part.ParticipantRole.PlayingEntity.Code == nil || part.ParticipantRole.PlayingEntity.Code.Code != "407543006" {
		t.Fatalf("expected playing entity code, got %+v", part.ParticipantRole.PlayingEntity)
	}
	if len(part.ParticipantRole.PlayingEntity.Names) != 1 || part.ParticipantRole.PlayingEntity.Names[0] != "Jane Doe" {
		t.Fatalf("expected playing entity name, got %+v", part.ParticipantRole.PlayingEntity.Names)
	}
}

func TestParser_ParseInformantVariants(t *testing.T) {
	parser := NewParser("test", nil)
	xml := `
<root xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <informant>
    <assignedEntity>
      <id root="1.2.3" extension="AE-1"/>
    </assignedEntity>
    <relatedEntity classCode="PRS">
      <code code="FAMMEMB" codeSystem="2.16.840.1.113883.5.111" displayName="Family member"/>
      <relatedPerson>
        <name><family>Doe</family><given>Jane</given></name>
      </relatedPerson>
    </relatedEntity>
  </informant>
  <informant>
    <other/>
  </informant>
</root>`

	doc, err := xmlquery.Parse(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("failed to parse xml: %v", err)
	}

	informants := xmlquery.Find(doc, "//informant")
	if len(informants) != 2 {
		t.Fatalf("expected 2 informant nodes, got %d", len(informants))
	}

	first := parser.parseInformant(informants[0])
	if first == nil {
		t.Fatal("expected informant parsed")
	}
	if first.AssignedEntity == nil || len(first.AssignedEntity.IDs) != 1 || first.AssignedEntity.IDs[0].Extension != "AE-1" {
		t.Fatalf("expected assigned entity with ID AE-1, got %+v", first.AssignedEntity)
	}
	if first.RelatedEntity == nil || first.RelatedEntity.ClassCode != "PRS" {
		t.Fatalf("expected related entity PRS, got %+v", first.RelatedEntity)
	}
	if first.RelatedEntity.Code == nil || first.RelatedEntity.Code.Code != "FAMMEMB" {
		t.Fatalf("expected related entity code FAMMEMB, got %+v", first.RelatedEntity.Code)
	}
	if first.RelatedEntity.Person == nil || len(first.RelatedEntity.Person.Names) == 0 || first.RelatedEntity.Person.Names[0].Family != "Doe" {
		t.Fatalf("expected related person Doe, got %+v", first.RelatedEntity.Person)
	}

	// Informant without assignedEntity/relatedEntity should return nil
	if second := parser.parseInformant(informants[1]); second != nil {
		t.Fatalf("expected nil informant for empty node, got %+v", second)
	}
}
