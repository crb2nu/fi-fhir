package fhir

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// updateMapperGolden regenerates testdata/fhir/mapper/ from the mapper:
//
//	go test ./pkg/fhir -run TestFHIRConformance_MapperGoldenFixtures -update-fhir-golden
//
// The fixtures are generated rather than hand-written on purpose. Before Slice
// 5.1a every fixture in testdata/fhir/ was hand-written, so the mapper and the
// checker were each tested against a third artefact and never against each
// other — which is precisely how a validator that rejects its own mapper's
// output stayed green in CI. A generated fixture makes mapper drift a reviewable
// diff instead of a silent pass.
var updateMapperGolden = flag.Bool("update-fhir-golden", false,
	"regenerate testdata/fhir/mapper/ from the mapper's current output")

const mapperGoldenDir = "mapper"

// TestFHIRConformance_MapperGoldenFixtures is the CI fixture for what the mapper
// actually produces.
//
// For every resource the mapper emits whose type the checker has an opinion
// about, a committed fixture holds the mapper's exact bytes. Each fixture is
// asserted twice: it must still be what the mapper produces (drift shows up as a
// diff a reviewer sees), and it must validate at --mode us-core --strict with
// zero issues (the checker must still agree with it).
func TestFHIRConformance_MapperGoldenFixtures(t *testing.T) {
	fixtures := mapperGoldenFixtures(t)
	if len(fixtures) == 0 {
		t.Fatal("no checked resources were produced; the fixture set would be vacuous")
	}
	directory := filepath.Join("..", "..", "testdata", "fhir", mapperGoldenDir)

	if *updateMapperGolden {
		if err := os.MkdirAll(directory, 0o750); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
		existing, err := filepath.Glob(filepath.Join(directory, "*.json"))
		if err != nil {
			t.Fatalf("glob %s: %v", directory, err)
		}
		for _, path := range existing {
			if err := os.Remove(path); err != nil {
				t.Fatalf("remove %s: %v", path, err)
			}
		}
		for _, fixture := range fixtures {
			path := filepath.Join(directory, fixture.name+".json")
			if err := os.WriteFile(path, fixture.encoded, 0o600); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
		}
		t.Logf("regenerated %d fixtures under %s", len(fixtures), directory)
		return
	}

	committed, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil {
		t.Fatalf("glob %s: %v", directory, err)
	}
	if len(committed) != len(fixtures) {
		t.Fatalf("testdata/fhir/%s holds %d fixtures, the mapper produces %d checked "+
			"resources; run with -update-fhir-golden", mapperGoldenDir, len(committed), len(fixtures))
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			path := filepath.Join(directory, fixture.name+".json")
			onDisk, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: run with -update-fhir-golden: %v", path, err)
			}
			if !bytes.Equal(bytes.TrimSpace(onDisk), bytes.TrimSpace(fixture.encoded)) {
				t.Errorf("%s no longer matches %s's output; run with -update-fhir-golden "+
					"and review the diff\nwant:\n%s\ngot:\n%s",
					path, fixture.entryPoint, onDisk, fixture.encoded)
			}
			outcome, err := ValidateJSON(onDisk, ValidationOptions{Mode: string(ModeUSCore)})
			if err != nil {
				t.Fatalf("ValidateJSON(%s): %v", path, err)
			}
			if len(outcome.Issue) != 0 {
				t.Errorf("%s does not validate: %s", path, describeIssues(outcome.Issue))
			}
		})
	}
}

// TestFHIRConformance_CheckerCoverageIsDerivableFromCode turns this package's
// published coverage numbers into assertions.
//
// Correction 43 found "0 of 55" in three prose locations and nowhere in the
// code; correction 44 found the required-element figure to be exactly 6 of 24.
// Every number the repository publishes about this checker is computed here from
// the checker itself, so a doc claim and the code cannot drift apart silently.
func TestFHIRConformance_CheckerCoverageIsDerivableFromCode(t *testing.T) {
	produced := producedResourceTypes(t)

	var (
		withProfileCheck  []string
		withRequiredCheck []string
		withNeither       []string
	)
	for _, resourceType := range produced {
		profileChecked := len(expectedProfilesForResourceType(resourceType)) > 0
		requiredChecked := hasRequiredElementCheck(t, resourceType)
		if profileChecked {
			withProfileCheck = append(withProfileCheck, resourceType)
		}
		if requiredChecked {
			withRequiredCheck = append(withRequiredCheck, resourceType)
		}
		if !profileChecked && !requiredChecked {
			withNeither = append(withNeither, resourceType)
		}
	}

	t.Run("24 non-Bundle resource types are produced", func(t *testing.T) {
		if len(produced) != 24 {
			t.Fatalf("the mapper produces %d non-Bundle resource types (%v), the docs say 24",
				len(produced), produced)
		}
	})

	t.Run("6 of 24 have required-element checks", func(t *testing.T) {
		want := []string{"Condition", "Coverage", "DiagnosticReport", "Encounter",
			"Observation", "Patient"}
		if strings.Join(withRequiredCheck, ",") != strings.Join(want, ",") {
			t.Fatalf("required-element checks cover %v, want %v", withRequiredCheck, want)
		}
	})

	t.Run("21 have profile-presence checks", func(t *testing.T) {
		if len(withProfileCheck) != 21 {
			t.Fatalf("profile-presence checks cover %d produced types (%v), the docs say 21",
				len(withProfileCheck), withProfileCheck)
		}
	})

	t.Run("exactly three produced types have neither check", func(t *testing.T) {
		want := []string{"Claim", "CoverageEligibilityResponse", "ExplanationOfBenefit"}
		if strings.Join(withNeither, ",") != strings.Join(want, ",") {
			t.Fatalf("types with neither check are %v, the docs name %v", withNeither, want)
		}
	})

	// 31 before Slice 5.1a; 32 after, because this slice declares
	// USCoreDiagnosticReportLabProfile, which MapLabResult previously carried as
	// a bare literal. Correction 43's "0 of 31" was the pre-slice count.
	t.Run("32 profile constants, 0 version-pinned", func(t *testing.T) {
		constants := declaredProfileConstants()
		if len(constants) != 32 {
			t.Fatalf("this package declares %d US Core profile constants, the docs say 32; "+
				"update the docs in the same commit", len(constants))
		}
		pinned := 0
		for _, profile := range constants {
			if strings.Contains(profile, "|") {
				pinned++
			}
		}
		if pinned != 0 {
			t.Fatalf("%d profile constants are version-pinned; the policy is bare canonicals "+
				"(see ProfileCanonical)", pinned)
		}
	})
}

// hasRequiredElementCheck probes the checker rather than reading a copy of its
// switch statement: a resource carrying nothing but its type and its expected
// profile reports an issue only if a required-element check exists for it.
func hasRequiredElementCheck(t *testing.T, resourceType string) bool {
	t.Helper()
	resource := map[string]any{"resourceType": resourceType}
	if expected := expectedProfilesForResourceType(resourceType); len(expected) > 0 {
		profiles := make([]string, 0, len(expected))
		for profile := range expected {
			profiles = append(profiles, profile)
		}
		sort.Strings(profiles)
		resource["meta"] = map[string]any{"profile": profiles[:1]}
	}
	encoded, err := json.Marshal(resource)
	if err != nil {
		t.Fatalf("marshal probe for %s: %v", resourceType, err)
	}
	outcome, err := ValidateJSON(encoded, ValidationOptions{Mode: string(ModeUSCore)})
	if err != nil {
		t.Fatalf("ValidateJSON probe for %s: %v", resourceType, err)
	}
	return len(outcome.Issue) > 0
}

// declaredProfileConstants collects every US Core profile canonical this package
// declares, by construction from the constant block rather than by counting
// lines in it.
func declaredProfileConstants() []string {
	constants := []string{
		USCorePatientProfile, USCoreObservationLabProfile, USCoreEncounterProfile,
		USCoreConditionProfile, USCoreCoverageProfile, USCoreProcedureProfile,
		USCoreImmunizationProfile, USCoreVitalSignsProfile,
		USCoreBloodPressureProfile, USCoreBodyHeightProfile, USCoreBodyWeightProfile,
		USCoreBodyTemperatureProfile, USCoreHeartRateProfile, USCoreRespiratoryRateProfile,
		USCorePulseOximetryProfile, USCoreBMIProfile,
		USCoreMedicationRequestProfile, USCoreMedicationProfile, USCoreAllergyIntoleranceProfile,
		USCoreCarePlanProfile, USCoreGoalProfile, USCoreCareTeamProfile,
		USCoreServiceRequestProfile,
		USCoreDocumentReferenceProfile, USCoreDiagnosticReportLabProfile,
		USCoreDiagnosticReportNoteProfile,
		USCoreProvenanceProfile, USCoreLocationProfile, USCoreOrganizationProfile,
		USCorePractitionerProfile, USCorePractitionerRoleProfile, USCoreRelatedPersonProfile,
	}
	seen := make(map[string]bool, len(constants))
	unique := make([]string, 0, len(constants))
	for _, profile := range constants {
		if seen[profile] {
			continue
		}
		seen[profile] = true
		unique = append(unique, profile)
	}
	sort.Strings(unique)
	return unique
}

// producedResourceTypes is the sorted set of non-Bundle resourceType values the
// mapper's 26 entry points emit, read off the marshalled bytes.
func producedResourceTypes(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, testCase := range mapperConformanceCases() {
		for _, resource := range testCase.resources {
			resourceType := marshalledResourceType(t, testCase.entryPoint, resource)
			if resourceType != "Bundle" {
				seen[resourceType] = true
			}
		}
	}
	types := make([]string, 0, len(seen))
	for resourceType := range seen {
		types = append(types, resourceType)
	}
	sort.Strings(types)
	return types
}

type mapperGoldenFixture struct {
	name       string
	entryPoint string
	encoded    []byte
}

// mapperGoldenFixtures is one fixture per resource the mapper produces whose
// type the checker has an opinion about. Types with neither check
// (Claim, ExplanationOfBenefit, CoverageEligibilityResponse) are deliberately
// excluded: a fixture that validates because nothing is checked would be
// evidence of nothing, and their absence is asserted by
// TestFHIRConformance_CheckerCoverageIsDerivableFromCode instead.
func mapperGoldenFixtures(t *testing.T) []mapperGoldenFixture {
	t.Helper()
	var fixtures []mapperGoldenFixture
	for _, testCase := range mapperConformanceCases() {
		kept := 0
		for _, resource := range testCase.resources {
			resourceType := marshalledResourceType(t, testCase.entryPoint, resource)
			if len(expectedProfilesForResourceType(resourceType)) == 0 &&
				!hasRequiredElementCheck(t, resourceType) {
				continue
			}
			encoded, err := json.MarshalIndent(resource, "", "  ")
			if err != nil {
				t.Fatalf("%s: marshal: %v", testCase.entryPoint, err)
			}
			kept++
			name := strings.ToLower(strings.TrimPrefix(testCase.entryPoint, "Map"))
			if len(testCase.resources) > 1 {
				name = fmt.Sprintf("%s_%d", name, kept)
			}
			fixtures = append(fixtures, mapperGoldenFixture{
				name: name, entryPoint: testCase.entryPoint,
				encoded: append(encoded, '\n'),
			})
		}
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].name < fixtures[j].name })
	return fixtures
}

func marshalledResourceType(t *testing.T, entryPoint string, resource any) string {
	t.Helper()
	encoded, err := json.Marshal(resource)
	if err != nil {
		t.Fatalf("%s: marshal: %v", entryPoint, err)
	}
	var envelope struct {
		ResourceType string `json:"resourceType"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("%s: unmarshal: %v", entryPoint, err)
	}
	if envelope.ResourceType == "" {
		t.Fatalf("%s produced a resource with no resourceType: %s", entryPoint, encoded)
	}
	return envelope.ResourceType
}
