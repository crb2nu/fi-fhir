package fhir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// TestUSCoreMapper_FixturesCarryNoCardinalityViolations is Slice 5.1c-α's
// day-1 gate, kept as the standing assertion that the mapper's own output
// carries no cardinality violation against the pinned packages.
//
// It is deliberately NOT a `TestFHIRStructural*` test. The structural gate's
// ledger test asserts equality with `recordedCardinalityGaps()`, which is a
// statement about what the ledger says; this one asserts zero, which is a
// statement about what the mapper emits. On unmodified main (1465aa516) it
// failed naming exactly the seven violations the ledger recorded. With the
// ledger empty the two agree, and either one going red means the other is
// about to.
//
// It sits outside the `^TestFHIRStructural` prefix so the ten-assertion arity
// guards in the Makefile and `ci/test-fhir-structural.yml` are unchanged. It
// still runs in `go test ./...`.
func TestUSCoreMapper_FixturesCarryNoCardinalityViolations(t *testing.T) {
	set := loadPinnedForTest(t)

	entries, err := os.ReadDir(mapperFixtureDir())
	if err != nil {
		t.Fatalf("read fixture directory: %v", err)
	}
	fixtures := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			fixtures = append(fixtures, entry.Name())
		}
	}
	sort.Strings(fixtures)

	// A zero-violation assertion over an empty or truncated directory would be
	// vacuous; the count is the same one the ledger test pins.
	if len(fixtures) != 25 {
		t.Fatalf("found %d mapper fixtures, want 25", len(fixtures))
	}

	var violations []string
	for _, name := range fixtures {
		data, err := os.ReadFile(filepath.Join(mapperFixtureDir(), name)) // #nosec G304 -- fixture directory listing.
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		outcome, err := ValidateStructuralJSON(data, set, StructuralOptions{})
		if err != nil {
			t.Fatalf("%s: ValidateStructuralJSON: %v", name, err)
		}
		for _, issue := range renderIssues(StructuralErrors(outcome)) {
			violations = append(violations, name+" :: "+issue)
		}
	}

	if len(violations) != 0 {
		t.Fatalf("%d cardinality violation(s) in the mapper's own output:\n%s",
			len(violations), indentAll(violations))
	}
}

// structuralErrorsFor marshals one mapper resource and returns its structural
// errors against the pinned packages. The fixtures prove the representative
// row; this proves the fallback shapes the fixtures never reach.
func structuralErrorsFor(t *testing.T, resource any) []string {
	t.Helper()
	encoded, err := json.Marshal(resource)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	outcome, err := ValidateStructuralJSON(encoded, loadPinnedForTest(t), StructuralOptions{})
	if err != nil {
		t.Fatalf("ValidateStructuralJSON: %v", err)
	}
	return renderIssues(StructuralErrors(outcome))
}

func codingKeys(concept CodeableConcept) []string {
	keys := make([]string, 0, len(concept.Coding))
	for _, coding := range concept.Coding {
		keys = append(keys, coding.System+"|"+coding.Code)
	}
	return keys
}

func TestUSCoreMapper_EncounterTypeDerivesFromPatientClass(t *testing.T) {
	const (
		sct = SystemSNOMED + "|"
		v2  = SystemV2PatientClass + "|"
	)
	cases := []struct {
		class    string
		wantKeys []string
		wantText string
	}{
		// The two classes the bound value set has a concept for.
		{"I", []string{sct + "86181006", v2 + "I"}, "Inpatient"},
		{"INPATIENT", []string{sct + "86181006", v2 + "I"}, "Inpatient"},
		{"IMP", []string{sct + "86181006", v2 + "I"}, "Inpatient"},
		{"E", []string{sct + "4525004", v2 + "E"}, "Emergency"},
		{"emergency", []string{sct + "4525004", v2 + "E"}, "Emergency"},
		// The rest carry the source concept only; no SNOMED CT concept in the
		// value set says what these say without saying more.
		{"O", []string{v2 + "O"}, "Outpatient"},
		{"AMB", []string{v2 + "O"}, "Outpatient"},
		{"P", []string{v2 + "P"}, "Preadmit"},
		{"R", []string{v2 + "R"}, "Recurring patient"},
		{"B", []string{v2 + "B"}, "Obstetrics"},
		{"C", []string{v2 + "C"}, "Commercial Account"},
		{"N", []string{v2 + "N"}, "Not Applicable"},
		{"U", []string{v2 + "U"}, "Unknown"},
		// US Core Missing Data: unrecognised source text is sent as text only.
		{"  Day Surgery  ", nil, "Day Surgery"},
		// No class at all: DataAbsentReason unknown, never a guessed class.
		{"", []string{SystemDataAbsentReason + "|unknown"}, ""},
		{"   ", []string{SystemDataAbsentReason + "|unknown"}, ""},
	}

	mapper := NewUSCoreMapper()
	mapper.Source = "main-hospital-adt"
	for _, tc := range cases {
		t.Run(strings.TrimSpace(tc.class)+"|", func(t *testing.T) {
			encounter := mapper.MapEncounter(&events.Encounter{ID: "V1", Class: tc.class}, "Patient/MRN-1")
			if len(encounter.Type) != 1 {
				t.Fatalf("Encounter.type has %d entries, want 1", len(encounter.Type))
			}
			got := encounter.Type[0]
			if strings.Join(codingKeys(got), ",") != strings.Join(tc.wantKeys, ",") {
				t.Errorf("codings = %v, want %v", codingKeys(got), tc.wantKeys)
			}
			if got.Text != tc.wantText {
				t.Errorf("text = %q, want %q", got.Text, tc.wantText)
			}
			if errs := structuralErrorsFor(t, encounter); len(errs) != 0 {
				t.Errorf("structural errors:\n%s", indentAll(errs))
			}
		})
	}
}

func TestUSCoreMapper_SourceAssignedIdentifiersCarryTheSourceSystem(t *testing.T) {
	t.Run("a bare visit number is qualified under the source", func(t *testing.T) {
		mapper := NewUSCoreMapper()
		mapper.Source = "main-hospital-adt"
		encounter := mapper.MapEncounter(&events.Encounter{ID: "V1", Class: "I"}, "Patient/MRN-1")
		want := Identifier{System: "urn:fi-fhir:source:main-hospital-adt", Value: "V1"}
		if len(encounter.Identifier) != 1 || encounter.Identifier[0].System != want.System ||
			encounter.Identifier[0].Value != want.Value {
			t.Fatalf("identifier = %+v, want %+v", encounter.Identifier, want)
		}
	})

	t.Run("the source is path-escaped, as fhirout escapes it", func(t *testing.T) {
		mapper := NewUSCoreMapper()
		mapper.Source = " lab feed/2 "
		encounter := mapper.MapEncounter(&events.Encounter{ID: "V1"}, "")
		if got := encounter.Identifier[0].System; got != "urn:fi-fhir:source:lab%20feed%2F2" {
			t.Fatalf("system = %q", got)
		}
	})

	t.Run("a system the source supplied, or the type's configured one, wins", func(t *testing.T) {
		mapper := NewUSCoreMapper()
		mapper.Source = "main-hospital-adt"
		encounter := mapper.MapEncounter(&events.Encounter{Identifiers: events.IdentifierSet{
			Identifiers: []events.Identifier{
				{System: "urn:oid:1.2.3", Value: "A1"},
				{Type: "MR", Value: "M1"},
				{Type: "VN", Value: "V1"},
			},
		}}, "")
		got := make([]string, 0, len(encounter.Identifier))
		for _, identifier := range encounter.Identifier {
			got = append(got, identifier.System+"|"+identifier.Value)
		}
		want := []string{
			"urn:oid:1.2.3|A1",
			"http://hospital.example.org/mrn|M1",
			"urn:fi-fhir:source:main-hospital-adt|V1",
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("identifiers = %v, want %v", got, want)
		}
	})

	t.Run("the same rule applies to a Patient's bare identifier", func(t *testing.T) {
		mapper := NewUSCoreMapper()
		mapper.Source = "main-hospital-adt"
		patient := mapper.MapPatient(&events.Patient{FamilyName: "Alpha", Identifiers: events.IdentifierSet{
			Identifiers: []events.Identifier{{Type: "PI", Value: "P1"}},
		}})
		if got := patient.Identifier[0].System; got != "urn:fi-fhir:source:main-hospital-adt" {
			t.Fatalf("system = %q", got)
		}
	})

	t.Run("a mapper not told the source emits no system rather than a guess", func(t *testing.T) {
		encounter := NewUSCoreMapper().MapEncounter(&events.Encounter{ID: "V1"}, "")
		if got := encounter.Identifier[0].System; got != "" {
			t.Fatalf("system = %q, want empty", got)
		}
	})
}

func TestUSCoreMapper_CoverageRelationshipIsReadOffTheEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	base := events.EligibilityResponseEvent{
		InformationSource: events.Provider{NPI: "1234567890", OrganizationName: "Blue Cross"},
		Subscriber:        events.Patient{MRN: "SUB123"},
		Status:            events.EligibilityStatusActive,
	}

	t.Run("no dependent loop: the subscriber is the beneficiary", func(t *testing.T) {
		event := base
		coverage := mapper.MapCoverage(&event, "Patient/SUB123")
		if coverage.Relationship == nil {
			t.Fatal("relationship is absent")
		}
		if got := codingKeys(*coverage.Relationship); strings.Join(got, ",") != SystemSubscriberRelation+"|self" {
			t.Fatalf("relationship = %v, want self", got)
		}
		if errs := structuralErrorsFor(t, coverage); len(errs) != 0 {
			t.Errorf("structural errors:\n%s", indentAll(errs))
		}
	})

	t.Run("a dependent: the relationship is unknown, not guessed", func(t *testing.T) {
		event := base
		event.Dependent = &events.Patient{MRN: "DEP456"}
		coverage := mapper.MapCoverage(&event, "")
		if coverage.Relationship == nil {
			t.Fatal("relationship is absent")
		}
		if got := codingKeys(*coverage.Relationship); strings.Join(got, ",") != SystemDataAbsentReason+"|unknown" {
			t.Fatalf("relationship = %v, want data-absent-reason unknown", got)
		}
		if errs := structuralErrorsFor(t, coverage); len(errs) != 0 {
			t.Errorf("structural errors:\n%s", indentAll(errs))
		}
	})
}

func TestUSCoreMapper_CareTeamNeedsSomebodyOnIt(t *testing.T) {
	mapper := NewUSCoreMapper()
	build := func(members ...events.CareTeamMember) *CareTeam {
		return mapper.MapCareTeam(&events.CareTeamEvent{CareTeam: events.CareTeam{
			Name: "Diabetes Care Team", Status: "active", Members: members,
		}}, "Patient/MRN-1")
	}

	t.Run("no members: no CareTeam", func(t *testing.T) {
		if got := build(); got != nil {
			t.Fatalf("got a CareTeam with nobody on it: %+v", got)
		}
	})

	t.Run("members that name nobody: no CareTeam", func(t *testing.T) {
		if got := build(events.CareTeamMember{Role: "nurse"},
			events.CareTeamMember{Provider: &events.Provider{}}); got != nil {
			t.Fatalf("got a CareTeam of unnamed members: %+v", got)
		}
	})

	t.Run("an unnamed member is dropped, a named one kept", func(t *testing.T) {
		careTeam := build(events.CareTeamMember{Role: "nurse"},
			events.CareTeamMember{Role: "nurse", Provider: &events.Provider{ID: "prov-1"}})
		if careTeam == nil || len(careTeam.Participant) != 1 {
			t.Fatalf("want one participant, got %+v", careTeam)
		}
		if careTeam.Participant[0].Member.Reference != "Practitioner/prov-1" {
			t.Fatalf("member = %+v", careTeam.Participant[0].Member)
		}
	})

	t.Run("a practitioner known only by name is referenced by display", func(t *testing.T) {
		careTeam := build(events.CareTeamMember{Role: "nurse",
			Provider: &events.Provider{GivenName: "Ann", FamilyName: "Lee"}})
		member := careTeam.Participant[0].Member
		if member.Reference != "" || member.Display != "Lee, Ann" {
			t.Fatalf("member = %+v, want a display-only reference", member)
		}
	})

	t.Run("a member with no role gets data-absent-reason unknown", func(t *testing.T) {
		careTeam := build(events.CareTeamMember{OrganizationID: "org-1", OrganizationName: "General Hospital"})
		role := careTeam.Participant[0].Role
		if len(role) != 1 || strings.Join(codingKeys(role[0]), ",") != SystemDataAbsentReason+"|unknown" {
			t.Fatalf("role = %+v, want data-absent-reason unknown", role)
		}
		if errs := structuralErrorsFor(t, careTeam); len(errs) != 0 {
			t.Errorf("structural errors:\n%s", indentAll(errs))
		}
	})
}

func TestUSCoreMapper_DocumentReferenceNeedsADocument(t *testing.T) {
	mapper := NewUSCoreMapper()
	build := func(content ...events.DocumentReferenceContent) *DocumentReference {
		return mapper.MapDocumentReference(&events.DocumentReferenceEvent{
			DocumentReference: events.DocumentReference{
				Status: "current", Type: "Discharge summary", Content: content,
			},
		}, "Patient/MRN-1")
	}

	t.Run("no content: no DocumentReference", func(t *testing.T) {
		if got := build(); got != nil {
			t.Fatalf("got a DocumentReference with no document: %+v", got)
		}
	})

	t.Run("content that neither locates nor carries a document: no DocumentReference", func(t *testing.T) {
		if got := build(events.DocumentReferenceContent{
			AttachmentContentType: "application/pdf", AttachmentTitle: "Discharge summary",
		}); got != nil {
			t.Fatalf("got a DocumentReference whose attachment has no url or data: %+v", got)
		}
	})

	t.Run("only the entries that locate or carry a document are kept", func(t *testing.T) {
		docRef := build(
			events.DocumentReferenceContent{AttachmentTitle: "nothing here"},
			events.DocumentReferenceContent{AttachmentContentType: "text/plain", AttachmentData: "aGVsbG8="},
		)
		if docRef == nil || len(docRef.Content) != 1 || docRef.Content[0].Attachment.Data != "aGVsbG8=" {
			t.Fatalf("content = %+v, want the one inline attachment", docRef)
		}
		if errs := structuralErrorsFor(t, docRef); len(errs) != 0 {
			t.Errorf("structural errors:\n%s", indentAll(errs))
		}
	})

	t.Run("a DocumentReference without content serialises it as absent, never null", func(t *testing.T) {
		encoded, err := json.Marshal(&DocumentReference{Status: "current"})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(encoded), `"content"`) {
			t.Fatalf("content is serialised: %s", encoded)
		}
	})
}

func TestUSCoreMapper_VitalSignEffectiveIsTheProducersClinicalTime(t *testing.T) {
	mapper := NewUSCoreMapper()
	measured := time.Date(2026, 8, 9, 14, 30, 0, 0, time.UTC)
	received := time.Date(2026, 8, 9, 16, 0, 0, 0, time.UTC)
	vital := events.VitalSign{Name: "Heart Rate", LOINCCode: LOINCHeartRate, Value: "72", Unit: "bpm"}

	observation := mapper.MapVitalSign(&events.VitalSignEvent{
		EventMeta: events.EventMeta{Timestamp: measured, ReceivedAt: received},
		VitalSign: vital,
	}, "Patient/MRN-1")
	if observation.EffectiveDateTime != "2026-08-09T14:30:00Z" {
		t.Fatalf("effectiveDateTime = %q, want the measurement time", observation.EffectiveDateTime)
	}
	if errs := structuralErrorsFor(t, observation); len(errs) != 0 {
		t.Errorf("structural errors:\n%s", indentAll(errs))
	}

	// With no clinical time the receipt time is NOT substituted: when fi-fhir
	// received a message is not when the heart rate was measured.
	untimed := mapper.MapVitalSign(&events.VitalSignEvent{
		EventMeta: events.EventMeta{ReceivedAt: received},
		VitalSign: vital,
	}, "Patient/MRN-1")
	if untimed.EffectiveDateTime != "" {
		t.Fatalf("effectiveDateTime = %q for an event with no clinical time", untimed.EffectiveDateTime)
	}
}
