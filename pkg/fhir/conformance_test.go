package fhir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// TestFHIRConformance_MapperOutputValidatesUnderItsOwnChecker is Slice 5.1a's
// primary kill-test.
//
// Every one of the mapper's exported Map* entry points is driven with a
// representative canonical event, and every resource it produces is marshalled
// and fed back through ValidateJSON at the shipped CLI's default settings —
// `--mode us-core --strict`, i.e. zero issues, warnings included. The mapper's
// own output must validate under the mapper's own checker.
//
// That property did not hold before this slice, and could not be observed:
// pkg/fhir/validate_golden_test.go feeds the checker hand-written fixtures, so
// the two halves of this package were only ever tested against a third artefact
// and never against each other.
//
// The table is bound to the type by reflection (see the coverage assertion
// below), so adding a Map* entry point without adding a row turns this red.
//
// Negative control: `make fhir-conformance-negative-control` rebuilds under the
// `fhirdrnoteonly` tag, which restores the pre-5.1a DiagnosticReport accepted
// set, and requires exactly the MapLabResult row to fail. A control that passes
// means the table is not round-tripping the mapper's bytes.
func TestFHIRConformance_MapperOutputValidatesUnderItsOwnChecker(t *testing.T) {
	cases := mapperConformanceCases()
	assertConformanceTableCoversEveryMapEntryPoint(t, cases)

	for _, testCase := range cases {
		t.Run(testCase.entryPoint, func(t *testing.T) {
			if len(testCase.resources) == 0 {
				t.Fatalf("%s produced no resources; the row proves nothing", testCase.entryPoint)
			}
			for index, resource := range testCase.resources {
				if resource == nil || reflect.ValueOf(resource).IsNil() {
					t.Fatalf("%s produced a nil resource at index %d", testCase.entryPoint, index)
				}
				encoded, err := json.Marshal(resource)
				if err != nil {
					t.Fatalf("%s: marshal resource %d: %v", testCase.entryPoint, index, err)
				}
				outcome, err := ValidateJSON(encoded, ValidationOptions{Mode: string(ModeUSCore)})
				if err != nil {
					t.Fatalf("%s: ValidateJSON: %v", testCase.entryPoint, err)
				}
				if len(outcome.Issue) != 0 {
					t.Errorf("%s resource %d does not validate under this package's own checker: %s\n%s",
						testCase.entryPoint, index, describeIssues(outcome.Issue), encoded)
				}
			}
		})
	}
}

// TestFHIRConformance_ValidatorRejectsMapperOutputToday is the Slice 5.1a day-1
// gate, promoted.
//
// It landed in MR !168 behind the `fhirday1gate` build tag, where it FAILED on
// `main` with `meta.profile does not include an expected profile for
// DiagnosticReport`: MapLabResult stamped `us-core-diagnosticreport-lab` as a
// bare literal while the checker accepted only `-note`. The reconciliation in
// this slice flips it, so it is now an ordinary passing test and the tag is
// gone. The name is kept deliberately — it is the gate's name in
// `.loom/33-sprint5-execution-specs.md` and in the worklog, and renaming it
// would break the trail from the finding to the fix.
//
// The control is kept too: the repository's own `-note` fixture is validated in
// the same run and must be clean, so a future failure here is a mapper/checker
// disagreement and not a broken checker.
func TestFHIRConformance_ValidatorRejectsMapperOutputToday(t *testing.T) {
	notePath := filepath.Join("..", "..", "testdata", "fhir", "diagnosticreport_uscore_note.json")
	fixture, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("read %s: %v", notePath, err)
	}
	noteOutcome, err := ValidateJSON(fixture, ValidationOptions{Mode: string(ModeUSCore)})
	if err != nil {
		t.Fatalf("ValidateJSON(note fixture): %v", err)
	}
	if len(noteOutcome.Issue) != 0 {
		t.Fatalf("control failed: the repo's own -note fixture reports %d issues (%s); "+
			"the checker is broken and this gate would be measuring the wrong thing",
			len(noteOutcome.Issue), describeIssues(noteOutcome.Issue))
	}

	report, observations := NewUSCoreMapper().MapLabResult(conformanceLabResultEvent())
	if report == nil || len(observations) == 0 {
		t.Fatal("MapLabResult produced no DiagnosticReport or no Observations")
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal DiagnosticReport: %v", err)
	}
	if want := USCoreDiagnosticReportLabProfile; !strings.Contains(string(encoded), want) {
		t.Fatalf("MapLabResult no longer stamps %s; the gate is validating something else\n%s",
			want, encoded)
	}
	outcome, err := ValidateJSON(encoded, ValidationOptions{Mode: string(ModeUSCore)})
	if err != nil {
		t.Fatalf("ValidateJSON(mapper output): %v", err)
	}
	if len(outcome.Issue) != 0 {
		t.Fatalf("the shipped validator rejects the shipped mapper's own DiagnosticReport: %s\n"+
			"mapper output: %s", describeIssues(outcome.Issue), encoded)
	}
}

// TestFHIRConformance_ModeIsAClosedSetAndFailsClosed covers correction 45: the
// validator used to disable every conformance check for any mode string that
// was not byte-exactly "us-core", and the CLI passed --mode through unvalidated.
// `--mode US-Core` printed "FHIR validation passed" and exited 0 on a resource
// that `--mode us-core` rejects.
func TestFHIRConformance_ModeIsAClosedSetAndFailsClosed(t *testing.T) {
	// A resource with a hard error under us-core: no identifier, no name, no
	// gender, no birthDate.
	nonConformant := []byte(`{"resourceType":"Patient"}`)

	t.Run("us-core rejects it", func(t *testing.T) {
		outcome, err := ValidateJSON(nonConformant, ValidationOptions{Mode: "us-core"})
		if err != nil {
			t.Fatalf("ValidateJSON: %v", err)
		}
		if len(outcome.Issue) == 0 {
			t.Fatal("the reference case reports no issues; the rest of this test is vacuous")
		}
	})

	t.Run("case and whitespace variants are the same mode, not a disabled one", func(t *testing.T) {
		for _, variant := range []string{"US-Core", "US-CORE", "Us-Core", " us-core ", "\tus-core\n"} {
			outcome, err := ValidateJSON(nonConformant, ValidationOptions{Mode: variant})
			if err != nil {
				t.Fatalf("ValidateJSON(mode=%q): %v", variant, err)
			}
			if len(outcome.Issue) == 0 {
				t.Errorf("mode %q reported a non-conformant Patient as clean; "+
					"the validator still fails open on mode", variant)
			}
		}
	})

	t.Run("an unrecognised mode is an error, never a quieter validation", func(t *testing.T) {
		for _, unknown := range []string{"", "  ", "uscore", "us_core", "US Core", "strict", "r4", "off"} {
			outcome, err := ValidateJSON(nonConformant, ValidationOptions{Mode: unknown})
			if err == nil {
				t.Errorf("mode %q was accepted and produced %d issues; an unknown mode must "+
					"fail closed", unknown, len(outcome.Issue))
				continue
			}
			if !strings.Contains(err.Error(), "unknown FHIR validation mode") {
				t.Errorf("mode %q failed with %v, want ErrUnknownValidationMode", unknown, err)
			}
			if outcome != nil {
				t.Errorf("mode %q returned an outcome alongside its error", unknown)
			}
		}
	})

	t.Run("none is still structural-only", func(t *testing.T) {
		for _, variant := range []string{"none", "None", "NONE"} {
			outcome, err := ValidateJSON(nonConformant, ValidationOptions{Mode: variant})
			if err != nil {
				t.Fatalf("ValidateJSON(mode=%q): %v", variant, err)
			}
			if len(outcome.Issue) != 0 {
				t.Errorf("mode %q reported %d issues, want structural-only silence",
					variant, len(outcome.Issue))
			}
		}
	})

	t.Run("the advertised set matches the accepted set", func(t *testing.T) {
		advertised := ValidationModes()
		if strings.Join(advertised, ",") != "none,us-core" {
			t.Fatalf("ValidationModes() = %v, want [none us-core]", advertised)
		}
		for _, mode := range advertised {
			if _, err := ParseValidationMode(mode); err != nil {
				t.Errorf("ValidationModes() advertises %q but ParseValidationMode rejects it: %v",
					mode, err)
			}
		}
	})
}

// TestFHIRConformance_ProfileVersionPolicy covers correction 42 and records the
// chosen policy as an executable statement: the mapper asserts bare canonicals,
// and the checker accepts a bare canonical or any version-pinned form of it.
//
// Before this slice the comparison was a byte-exact map lookup against 31
// unversioned constants, so a Patient declaring `…/us-core-patient|9.0.0` — the
// form a conformant publisher is most likely to emit — failed the presence
// check while the unpinned control passed.
func TestFHIRConformance_ProfileVersionPolicy(t *testing.T) {
	t.Run("ProfileCanonical strips the version suffix", func(t *testing.T) {
		for input, want := range map[string]string{
			USCorePatientProfile:              USCorePatientProfile,
			USCorePatientProfile + "|9.0.0":   USCorePatientProfile,
			USCorePatientProfile + "|6.1.0":   USCorePatientProfile,
			" " + USCorePatientProfile + " ":  USCorePatientProfile,
			USCorePatientProfile + " | 9.0.0": USCorePatientProfile,
			"":                                "",
			"|9.0.0":                          "",
		} {
			if got := ProfileCanonical(input); got != want {
				t.Errorf("ProfileCanonical(%q) = %q, want %q", input, got, want)
			}
		}
	})

	t.Run("a pinned profile satisfies the presence check", func(t *testing.T) {
		for _, profile := range []string{
			USCorePatientProfile,
			USCorePatientProfile + "|9.0.0",
			USCorePatientProfile + "|6.1.0",
		} {
			patient := map[string]any{
				"resourceType": "Patient",
				"meta":         map[string]any{"profile": []string{profile}},
				"identifier":   []any{map[string]any{"value": "MRN-1"}},
				"name":         []any{map[string]any{"family": "Alpha"}},
				"gender":       "female",
				"birthDate":    "1980-01-01",
			}
			encoded, err := json.Marshal(patient)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			outcome, err := ValidateJSON(encoded, ValidationOptions{Mode: string(ModeUSCore)})
			if err != nil {
				t.Fatalf("ValidateJSON: %v", err)
			}
			if len(outcome.Issue) != 0 {
				t.Errorf("profile %q reported %s", profile, describeIssues(outcome.Issue))
			}
		}
	})

	t.Run("a wrong profile is still wrong, pinned or not", func(t *testing.T) {
		for _, profile := range []string{
			USCoreEncounterProfile,
			USCoreEncounterProfile + "|9.0.0",
			USCorePatientProfile + "-not-really",
		} {
			patient := map[string]any{
				"resourceType": "Patient",
				"meta":         map[string]any{"profile": []string{profile}},
				"identifier":   []any{map[string]any{"value": "MRN-1"}},
				"name":         []any{map[string]any{"family": "Alpha"}},
				"gender":       "female",
				"birthDate":    "1980-01-01",
			}
			encoded, err := json.Marshal(patient)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			outcome, err := ValidateJSON(encoded, ValidationOptions{Mode: string(ModeUSCore)})
			if err != nil {
				t.Fatalf("ValidateJSON: %v", err)
			}
			if len(outcome.Issue) == 0 {
				t.Errorf("profile %q was accepted for a Patient; version tolerance has become "+
					"profile tolerance", profile)
			}
		}
	})

	t.Run("the mapper asserts bare canonicals", func(t *testing.T) {
		for _, testCase := range mapperConformanceCases() {
			for _, resource := range testCase.resources {
				encoded, err := json.Marshal(resource)
				if err != nil {
					t.Fatalf("%s: marshal: %v", testCase.entryPoint, err)
				}
				var parsed struct {
					Meta *struct {
						Profile []string `json:"profile"`
					} `json:"meta"`
				}
				if err := json.Unmarshal(encoded, &parsed); err != nil {
					t.Fatalf("%s: unmarshal: %v", testCase.entryPoint, err)
				}
				if parsed.Meta == nil {
					continue
				}
				for _, profile := range parsed.Meta.Profile {
					if strings.Contains(profile, "|") {
						t.Errorf("%s asserts a version-pinned canonical %q; the policy is bare "+
							"canonicals", testCase.entryPoint, profile)
					}
				}
			}
		}
	})
}

// TestFHIRConformance_PatientMRNOnlyProducesAnIdentifier covers correction 46:
// mapper.go mapped identifiers only from Patient.Identifiers, so the documented
// Patient.MRN convenience field (pkg/events/events.go) was dropped and an
// MRN-only Patient produced a hard `[error] Patient.identifier is required`.
// Unlike the DiagnosticReport disagreement this is an ERROR, not a warning.
func TestFHIRConformance_PatientMRNOnlyProducesAnIdentifier(t *testing.T) {
	mapper := NewUSCoreMapper()

	t.Run("MRN-only input validates", func(t *testing.T) {
		patient := mapper.MapPatient(&events.Patient{
			MRN: "MRN-000123", FamilyName: "Alpha", GivenName: "Ada",
			Gender: "F", DateOfBirth: time.Date(1980, 5, 15, 0, 0, 0, 0, time.UTC),
		})
		if len(patient.Identifier) != 1 {
			t.Fatalf("Patient.Identifier len = %d, want 1", len(patient.Identifier))
		}
		if patient.Identifier[0].Value != "MRN-000123" {
			t.Fatalf("Patient.Identifier[0].Value = %q, want the MRN",
				patient.Identifier[0].Value)
		}
		if patient.Identifier[0].System != "http://hospital.example.org/mrn" {
			t.Fatalf("the backfilled MRN carries no system: %q", patient.Identifier[0].System)
		}
		encoded, err := json.Marshal(patient)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		outcome, err := ValidateJSON(encoded, ValidationOptions{Mode: string(ModeUSCore)})
		if err != nil {
			t.Fatalf("ValidateJSON: %v", err)
		}
		if len(outcome.Issue) != 0 {
			t.Fatalf("an MRN-only Patient does not validate: %s", describeIssues(outcome.Issue))
		}
	})

	t.Run("an already-expressed MRN is not duplicated", func(t *testing.T) {
		patient := mapper.MapPatient(&events.Patient{
			MRN: "MRN-000123", FamilyName: "Alpha", GivenName: "Ada",
			Gender: "F", DateOfBirth: time.Date(1980, 5, 15, 0, 0, 0, 0, time.UTC),
			Identifiers: events.IdentifierSet{Identifiers: []events.Identifier{
				{Type: "MR", Value: "MRN-000123"},
				{Type: "SS", Value: "123-45-6789"},
			}},
		})
		if len(patient.Identifier) != 2 {
			t.Fatalf("Patient.Identifier len = %d, want 2 — the MRN was duplicated",
				len(patient.Identifier))
		}
	})

	t.Run("an empty MRN adds nothing", func(t *testing.T) {
		patient := mapper.MapPatient(&events.Patient{
			MRN: "   ", FamilyName: "Alpha", Gender: "F",
			DateOfBirth: time.Date(1980, 5, 15, 0, 0, 0, 0, time.UTC),
			Identifiers: events.IdentifierSet{Identifiers: []events.Identifier{
				{Type: "SS", Value: "123-45-6789"},
			}},
		})
		if len(patient.Identifier) != 1 {
			t.Fatalf("Patient.Identifier len = %d, want 1", len(patient.Identifier))
		}
	})
}

// TestFHIRConformance_LabCodeHasNoDuplicateCoding covers the second half of
// correction 46. A parser that fills both LabTest.LOINCCode and
// LabTest.Code.Coding — the normal shape — used to produce the same
// (system, code) pair twice in DiagnosticReport.code.coding.
func TestFHIRConformance_LabCodeHasNoDuplicateCoding(t *testing.T) {
	mapper := NewUSCoreMapper()
	report, _ := mapper.MapLabResult(&events.LabResultEvent{
		Patient: events.Patient{MRN: "MRN-000123"},
		Test: events.LabTest{
			LOINCCode:   "58410-2",
			Description: "CBC panel",
			Code: events.CodeableConcept{Coding: []events.Coding{
				{System: SystemLOINC, Code: "58410-2", Display: "CBC panel"},
				{System: "http://local.example.org/lab-codes", Code: "CBC"},
			}},
		},
		Result: events.LabValue{Status: "final"},
		Results: []events.LabObservation{{
			Test:   events.LabTest{LOINCCode: "6690-2", Description: "Leukocytes"},
			Result: events.LabValue{Value: "12.5", Unit: "10*3/uL", Status: "final"},
		}},
	})
	if report == nil {
		t.Fatal("MapLabResult produced no DiagnosticReport")
	}

	seen := map[string]int{}
	for _, coding := range report.Code.Coding {
		seen[coding.System+"|"+coding.Code]++
	}
	for key, count := range seen {
		if count > 1 {
			t.Errorf("DiagnosticReport.code.coding repeats %s %d times", key, count)
		}
	}
	if len(report.Code.Coding) != 2 {
		t.Fatalf("DiagnosticReport.code.coding len = %d, want 2 distinct codings; got %v",
			len(report.Code.Coding), report.Code.Coding)
	}
}

// describeIssues renders an OperationOutcome as one line per issue so a failure
// names the reason rather than a count.
func describeIssues(issues []OperationOutcomeIssue) string {
	rendered := make([]string, 0, len(issues))
	for _, issue := range issues {
		rendered = append(rendered, issue.Severity+" "+issue.Code+": "+issue.Diagnostics)
	}
	return strings.Join(rendered, "; ")
}

// conformanceCase is one Map* entry point and every resource it produced.
type conformanceCase struct {
	entryPoint string
	resources  []any
}

// assertConformanceTableCoversEveryMapEntryPoint binds the table to the type
// rather than to a comment. A Map* method added to USCoreMapper without a row
// here turns the primary kill-test red, which is the only way a table like this
// stays true.
func assertConformanceTableCoversEveryMapEntryPoint(t *testing.T, cases []conformanceCase) {
	t.Helper()

	declared := make([]string, 0, len(cases))
	seen := make(map[string]bool, len(cases))
	for _, testCase := range cases {
		if seen[testCase.entryPoint] {
			t.Fatalf("the conformance table lists %s twice", testCase.entryPoint)
		}
		seen[testCase.entryPoint] = true
		declared = append(declared, testCase.entryPoint)
	}

	mapperType := reflect.TypeOf(NewUSCoreMapper())
	exported := make([]string, 0, mapperType.NumMethod())
	for index := 0; index < mapperType.NumMethod(); index++ {
		name := mapperType.Method(index).Name
		if strings.HasPrefix(name, "Map") {
			exported = append(exported, name)
		}
	}

	sort.Strings(declared)
	sort.Strings(exported)
	if strings.Join(declared, ",") != strings.Join(exported, ",") {
		t.Fatalf("the conformance table does not cover USCoreMapper's Map* entry points\n"+
			"table:  %v\nmapper: %v", declared, exported)
	}
	if len(exported) != 26 {
		t.Fatalf("USCoreMapper exposes %d Map* entry points, the spec and the docs say 26; "+
			"update both rather than this assertion", len(exported))
	}
}

const conformancePatientRef = "Patient/MRN-000123"

func conformanceLabResultEvent() *events.LabResultEvent {
	return &events.LabResultEvent{
		Patient: events.Patient{MRN: "MRN-000123"},
		Test:    events.LabTest{LOINCCode: "58410-2", Description: "CBC panel"},
		Result:  events.LabValue{Status: "final"},
		Results: []events.LabObservation{
			{
				Test:   events.LabTest{LOINCCode: "6690-2", Description: "Leukocytes"},
				Result: events.LabValue{Value: "12.5", Unit: "10*3/uL", Status: "final"},
			},
			{
				Test:   events.LabTest{LOINCCode: "789-8", Description: "Erythrocytes"},
				Result: events.LabValue{Value: "5.2", Unit: "10*6/uL", Status: "final"},
			},
		},
	}
}

// mapperConformanceCases drives every exported Map* entry point with a
// representative canonical event.
//
// "Representative" means the shape a shipped parser produces, not the minimum
// that satisfies the checker: a row that passes only because its event was
// hollowed out until nothing was checkable would be worse than no row.
func mapperConformanceCases() []conformanceCase {
	mapper := NewUSCoreMapper()
	birth := time.Date(1980, 5, 15, 0, 0, 0, 0, time.UTC)
	admit := time.Date(2026, 8, 9, 10, 30, 0, 0, time.UTC)
	observed := time.Date(2026, 8, 9, 14, 30, 0, 0, time.UTC)
	planStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	planEnd := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	serviceDate := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	patient := &events.Patient{
		MRN: "MRN-000123", FamilyName: "Alpha", GivenName: "Ada", MiddleName: "Byron",
		Prefix: "Ms.", Gender: "F", DateOfBirth: birth,
		Race: "White", Ethnicity: "Not Hispanic",
		Address: events.Address{Line1: "123 Main St", City: "Anytown", State: "VA", PostalCode: "24101"},
		Phone:   "555-123-4567", Email: "ada@example.org",
		Identifiers: events.IdentifierSet{Identifiers: []events.Identifier{
			{Type: "MR", Value: "MRN-000123"},
			{Type: "SS", Value: "123-45-6789"},
		}},
	}

	eligibility := &events.EligibilityResponseEvent{
		InformationSource: events.Provider{NPI: "1234567890", OrganizationName: "Blue Cross"},
		Subscriber: events.Patient{
			MRN: "SUB123", FamilyName: "Alpha", GivenName: "Ada",
			Identifiers: events.IdentifierSet{Identifiers: []events.Identifier{
				{Type: "MB", Value: "MEM987654"},
			}},
		},
		Status:        events.EligibilityStatusActive,
		PlanBeginDate: planStart,
		PlanEndDate:   planEnd,
		Benefits: []events.EligibilityBenefit{
			{InformationCode: "1", PlanDescription: "PPO Gold Plan", InsuranceType: "PR"},
			{InformationCode: "C", Amount: 1500.00, ServiceType: "30"},
		},
	}

	labReport, labObservations := mapper.MapLabResult(conformanceLabResultEvent())
	labResources := make([]any, 0, len(labObservations)+1)
	labResources = append(labResources, labReport)
	for _, observation := range labObservations {
		labResources = append(labResources, observation)
	}

	return []conformanceCase{
		{"MapPatient", []any{mapper.MapPatient(patient)}},

		{"MapEncounter", []any{mapper.MapEncounter(&events.Encounter{
			ID: "ENC123", Class: "I", Status: "active", AdmitDateTime: admit,
			Location: events.Location{Facility: "Main Hospital", Unit: "ICU", Room: "101", Bed: "A"},
			AttendingProvider: &events.Provider{
				NPI: "1234567890", FamilyName: "Smith", GivenName: "Jane", Prefix: "Dr.",
			},
		}, conformancePatientRef)}},

		{"MapLabObservation", []any{mapper.MapLabObservation(&events.LabObservation{
			Test: events.LabTest{LOINCCode: "6690-2", Description: "Leukocytes [#/volume] in Blood"},
			Result: events.LabValue{
				Value: "12.5", Unit: "10*3/uL", ReferenceRange: "4.5-11.0",
				Interpretation: "H", Status: "F", ObservationTime: observed,
			},
		}, conformancePatientRef)}},

		{"MapLabResult", labResources},

		{"MapCondition", []any{mapper.MapCondition(&events.ConditionEvent{
			EventMeta: events.EventMeta{Timestamp: admit},
			Patient:   &events.Patient{MRN: "MRN-000123"},
			Condition: events.Condition{
				Name: "Type 2 Diabetes Mellitus", Code: "E11.9",
				CodeSystem: SystemICD10CM, Category: "problem-list-item",
			},
			ClinicalStatus: "active", OnsetDate: "2020-03-15",
		}, conformancePatientRef)}},

		{"MapCoverage", []any{mapper.MapCoverage(eligibility, conformancePatientRef)}},

		{"MapCoverageEligibilityResponse", []any{
			mapper.MapCoverageEligibilityResponse(eligibility, conformancePatientRef),
		}},

		{"MapClaim", []any{mapper.MapClaim(&events.ClaimSubmittedEvent{
			EventMeta: events.EventMeta{Type: events.EventClaimSubmitted, Timestamp: serviceDate},
			Patient:   events.Patient{MRN: "MRN-000123", FamilyName: "Alpha", GivenName: "Ada"},
			BillingProvider: events.Provider{
				NPI: "1234567890", OrganizationName: "Acme Medical Group",
			},
			Payer: events.Provider{NPI: "5555555555", OrganizationName: "Blue Cross"},
			Subscriber: events.Patient{
				MRN: "SUB456",
				Identifiers: events.IdentifierSet{Identifiers: []events.Identifier{
					{Type: "MB", Value: "MEM789"},
				}},
			},
			Claim: events.Claim{
				ControlNumber: "CLM-001", TotalAmount: 250.00, PlaceOfService: "11",
				ServiceDate: serviceDate, DiagnosisCodes: []string{"J06.9"},
			},
		}, "professional")}},

		{"MapExplanationOfBenefit", []any{mapper.MapExplanationOfBenefit(
			&events.ClaimAdjudicatedEvent{
				EventMeta: events.EventMeta{
					Type: events.EventClaimAdjudicated, Timestamp: serviceDate,
				},
				Payer:       events.Provider{NPI: "5555555555", OrganizationName: "Blue Cross"},
				Payee:       events.Provider{NPI: "1234567890", OrganizationName: "Acme Medical Group"},
				CheckNumber: "CHK123456", CheckDate: serviceDate, TotalPaid: 180.00,
				Payment: events.ClaimPayment{
					ClaimID: "CLM-001", PayerClaimID: "PCN-999", Status: "Processed",
					ChargedAmount: 250.00, PaidAmount: 180.00,
				},
			})}},

		{"MapProcedure", []any{mapper.MapProcedure(&events.ProcedureEvent{
			EventMeta: events.EventMeta{ID: "proc-123"},
			Procedure: events.Procedure{
				Name: "Appendectomy", Code: "80146002", Status: "completed",
			},
			PerformedDate: "2026-08-01",
		}, conformancePatientRef)}},

		{"MapImmunization", []any{mapper.MapImmunization(&events.ImmunizationEvent{
			EventMeta: events.EventMeta{ID: "imm-123"},
			Immunization: events.Immunization{
				VaccineCode: "140", VaccineName: "Influenza, seasonal", Status: "completed",
			},
			AdministeredDate: "2026-08-01",
		}, conformancePatientRef)}},

		{"MapVitalSign", []any{mapper.MapVitalSign(&events.VitalSignEvent{
			EventMeta: events.EventMeta{ID: "vs-123"},
			VitalSign: events.VitalSign{
				Name: "Heart Rate", LOINCCode: LOINCHeartRate, Value: "72",
				Unit: "bpm", Interpretation: "normal",
			},
		}, conformancePatientRef)}},

		{"MapMedicationRequest", []any{mapper.MapMedicationRequest(
			&events.MedicationRequestEvent{
				EventMeta: events.EventMeta{ID: "med-req-001"},
				Patient:   &events.Patient{MRN: "MRN-000123"},
				MedicationRequest: events.MedicationRequest{
					Medication: events.Medication{
						Code: "197361", Name: "Lisinopril 10 MG Oral Tablet",
					},
					Status: "active", Intent: "order", AuthoredOn: "2026-08-01T10:30:00Z",
					DosageInstruction: "Take 1 tablet by mouth daily",
					DispenseQuantity:  30, DispenseUnit: "tablet",
					DaysSupply: 30, NumberOfRefills: 3,
				},
			}, conformancePatientRef)}},

		{"MapAllergyIntolerance", []any{mapper.MapAllergyIntolerance(
			&events.AllergyIntoleranceEvent{
				EventMeta: events.EventMeta{ID: "allergy-001"},
				Patient:   &events.Patient{MRN: "MRN-000123"},
				AllergyIntolerance: events.AllergyIntolerance{
					Code: "7980", Name: "Penicillin", Category: "medication",
					ClinicalStatus: "active", VerificationStatus: "confirmed",
					Criticality: "high", Type: "allergy",
				},
			}, conformancePatientRef)}},

		{"MapCarePlan", []any{mapper.MapCarePlan(&events.CarePlanEvent{
			EventMeta: events.EventMeta{ID: "cp-001"},
			CarePlan: events.CarePlan{
				Title: "Diabetes Management Plan", Description: "Comprehensive plan",
				Status: "active", Intent: "plan", Category: "discharge",
				PeriodStart: "2026-01-01", PeriodEnd: "2026-12-31",
			},
		}, conformancePatientRef)}},

		{"MapGoal", []any{mapper.MapGoal(&events.GoalEvent{
			EventMeta: events.EventMeta{ID: "goal-001"},
			Goal: events.Goal{
				Description: "Maintain HbA1c below 7%", LifecycleStatus: "active",
				AchievementStatus: "in-progress", Category: "dietary", Priority: "high",
				StartDate: "2026-01-01", TargetDate: "2026-06-01",
			},
		}, conformancePatientRef)}},

		{"MapCareTeam", []any{mapper.MapCareTeam(&events.CareTeamEvent{
			EventMeta: events.EventMeta{ID: "ct-001"},
			CareTeam: events.CareTeam{
				Name: "Diabetes Care Team", Status: "active", Category: "longitudinal",
				PeriodStart: "2026-01-01", PeriodEnd: "2026-12-31",
			},
		}, conformancePatientRef)}},

		{"MapServiceRequest", []any{mapper.MapServiceRequest(&events.ServiceRequestEvent{
			EventMeta: events.EventMeta{ID: "sr-001"},
			ServiceRequest: events.ServiceRequest{
				Status: "active", Intent: "order", Category: "laboratory",
				Code: "80053", CodeSystem: SystemCPT,
				CodeText: "Comprehensive metabolic panel", AuthoredOn: "2026-08-01",
			},
			Requester: &events.Provider{ID: "doc-001", GivenName: "John", FamilyName: "Doe"},
		}, conformancePatientRef)}},

		{"MapDocumentReference", []any{mapper.MapDocumentReference(
			&events.DocumentReferenceEvent{
				EventMeta: events.EventMeta{ID: "doc-001", Type: "document_created"},
				Patient:   &events.Patient{MRN: "MRN-000123"},
				DocumentReference: events.DocumentReference{
					Status: "current", DocStatus: "final", Type: "Discharge summary",
					TypeCode: "18842-5", TypeCodeSystem: SystemLOINC,
					Category: "Clinical Note", Date: "2026-08-01T10:30:00Z",
					Description: "Patient discharge summary",
				},
			}, conformancePatientRef)}},

		{"MapDiagnosticReportNote", []any{mapper.MapDiagnosticReportNote(
			&events.DiagnosticReportNoteEvent{
				EventMeta: events.EventMeta{ID: "report-001", Type: "diagnostic_report_created"},
				Patient:   &events.Patient{MRN: "MRN-000123"},
				DiagnosticReportNote: events.DiagnosticReportNote{
					Status: "final", Category: "Radiology", CategoryCode: "LP29684-5",
					Code: "Chest X-ray", CodeValue: "36643-5", CodeSystem: SystemLOINC,
					EffectiveDateTime: "2026-08-01T10:30:00Z", Issued: "2026-08-01T12:00:00Z",
				},
			}, conformancePatientRef)}},

		{"MapProvenance", []any{mapper.MapProvenance(&events.ProvenanceEvent{
			Provenance: events.Provenance{
				TargetReferences: []string{conformancePatientRef},
				TargetDisplays:   []string{"Ada Alpha"},
				Recorded:         "2026-08-01T10:30:00Z",
				OccurredDateTime: "2026-08-01T10:00:00Z",
				Activity:         "Create", ActivityCode: "CREATE",
				Agents: []events.ProvenanceAgent{{
					Type: "Author", TypeCode: "author",
					WhoReference: "Practitioner/prac-1", WhoDisplay: "Dr. Jane Smith",
				}},
			},
		})}},

		{"MapLocation", []any{mapper.MapLocation(&events.FacilityLocationEvent{
			FacilityLocation: events.FacilityLocation{
				ID: "loc-1", Status: "active", Name: "Main Hospital",
				Description: "Primary care facility", Mode: "instance",
				Type: "Hospital", TypeCode: "HOSP",
				Address: &events.Address{
					Line1: "123 Health St", City: "Boston", State: "MA", PostalCode: "02101",
				},
				PhysicalType: "Building", PhysicalTypeCode: "bu",
			},
		})}},

		{"MapOrganization", []any{mapper.MapOrganization(&events.OrganizationEvent{
			Organization: events.Organization{
				ID: "org-1", Active: true, Name: "General Hospital",
				NPI: "1234567890", TIN: "12-3456789",
				Type: "Healthcare Provider", TypeCode: "prov",
				Address: &events.Address{
					Line1: "100 Medical Center Dr", City: "Boston",
					State: "MA", PostalCode: "02101",
				},
				Phone: "555-999-0000", Email: "admin@genhospital.org",
			},
		})}},

		{"MapPractitioner", []any{mapper.MapPractitioner(&events.PractitionerEvent{
			Practitioner: events.Practitioner{
				ID: "prac-1", Active: true, NPI: "1234567890",
				GivenName: "Jane", FamilyName: "Smith", Prefix: "Dr.", Suffix: "MD",
				Gender: "F", BirthDate: "1975-03-20",
				Address: &events.Address{
					Line1: "500 Doctor Way", City: "Cambridge", State: "MA", PostalCode: "02139",
				},
			},
		})}},

		{"MapPractitionerRole", []any{mapper.MapPractitionerRole(&events.PractitionerRoleEvent{
			PractitionerRole: events.PractitionerRole{
				ID: "role-1", Active: true,
				PractitionerID: "prac-1", PractitionerName: "Dr. Jane Smith",
				OrganizationID: "org-1", OrganizationName: "General Hospital",
				Code: "Physician", CodeValue: "physician",
				Specialty: "Internal Medicine", SpecialtyCode: "207R00000X",
			},
		})}},

		{"MapRelatedPerson", []any{mapper.MapRelatedPerson(&events.RelatedPersonEvent{
			RelatedPerson: events.RelatedPerson{
				ID: "rp-1", Active: true, PatientID: "MRN-000123",
				Relationship: "Mother", RelationshipCode: "MTH",
				GivenName: "Mary", FamilyName: "Alpha",
				Gender: "F", BirthDate: "1950-06-15",
			},
		}, conformancePatientRef)}},
	}
}
