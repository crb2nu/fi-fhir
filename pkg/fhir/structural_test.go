package fhir

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func packagesDir() string {
	return filepath.Join("..", "..", "testdata", "fhir", "packages")
}

func mapperFixtureDir() string {
	return filepath.Join("..", "..", "testdata", "fhir", "mapper")
}

func loadPinnedForTest(t *testing.T) *PackageSet {
	t.Helper()
	set, err := LoadPinnedPackages(packagesDir())
	if err != nil {
		t.Fatalf("LoadPinnedPackages: %v", err)
	}
	return set
}

// TestFHIRStructural_PinnedPackagesMatchTheirRecordedDigests is the pin itself.
//
// Three records have to agree or the pin means nothing: the bytes on disk, the
// SHA256SUMS file a human regenerates, and the PinnedPackage constants the
// loader enforces. Checking the bytes against only one of the two records would
// let the other rot unnoticed.
func TestFHIRStructural_PinnedPackagesMatchTheirRecordedDigests(t *testing.T) {
	recorded, err := readSHA256SUMS(filepath.Join(packagesDir(), "SHA256SUMS"))
	if err != nil {
		t.Fatalf("read SHA256SUMS: %v", err)
	}
	if len(recorded) != len(PinnedPackages()) {
		t.Fatalf("SHA256SUMS lists %d archives, PinnedPackages() has %d", len(recorded), len(PinnedPackages()))
	}

	for _, pinned := range PinnedPackages() {
		t.Run(pinned.String(), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(packagesDir(), pinned.Filename()))
			if err != nil {
				t.Fatalf("read archive: %v", err)
			}
			sum := sha256.Sum256(raw)
			onDisk := hex.EncodeToString(sum[:])

			if onDisk != pinned.SHA256 {
				t.Errorf("archive sha256 %s, PinnedPackage constant says %s", onDisk, pinned.SHA256)
			}
			if got := recorded[pinned.Filename()]; got != onDisk {
				t.Errorf("archive sha256 %s, SHA256SUMS says %s", onDisk, got)
			}
		})
	}
}

func readSHA256SUMS(path string) (map[string]string, error) {
	file, err := os.Open(path) // #nosec G304 -- fixed test fixture path.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	sums := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		sums[strings.TrimPrefix(fields[1], "*")] = fields[0]
	}
	return sums, scanner.Err()
}

// TestFHIRStructural_PinnedPackagesLoadAndDeclareTheirIdentity asserts the
// archives are the packages they claim to be and that the loader's filters
// still see what the README says they see.
func TestFHIRStructural_PinnedPackagesLoadAndDeclareTheirIdentity(t *testing.T) {
	set := loadPinnedForTest(t)

	// StructureDefinition counts are asserted, not merely logged: a loader
	// filter that silently stopped matching would otherwise turn every
	// resolution test below into a vacuous pass.
	want := map[string]struct {
		version                  string
		structureDefinitionCount int
	}{
		PinnedR4Core.Name: {version: "4.0.1", structureDefinitionCount: 658},
		PinnedUSCore.Name: {version: "9.0.0", structureDefinitionCount: 70},
	}

	if len(set.Packages) != len(want) {
		t.Fatalf("loaded %d packages, want %d", len(set.Packages), len(want))
	}
	for _, pkg := range set.Packages {
		expected, known := want[pkg.Name]
		if !known {
			t.Fatalf("unexpected package %s", pkg.Name)
		}
		if pkg.Version != expected.version {
			t.Errorf("%s declares version %s, want %s", pkg.Name, pkg.Version, expected.version)
		}
		if len(pkg.Definition) != expected.structureDefinitionCount {
			t.Errorf("%s holds %d StructureDefinitions, want %d",
				pkg.Name, len(pkg.Definition), expected.structureDefinitionCount)
		}
	}
}

// TestFHIRStructural_ProfileVersionResolution executes the Slice 5.1a
// profile-version assertion policy against real packages, which is the step
// 5.1a explicitly could not take.
//
// `docs/operations/SUPPORTED-1.0.md` put it as "version *tolerance* is not
// version *resolution*: stripping a `|version` suffix lets a correctly pinned
// resource pass the presence check; it does not verify that the resource
// conforms to that version of that profile." Case 3 below is the difference:
// under tolerance alone `…|8.0.0` passes, and here it does not.
func TestFHIRStructural_ProfileVersionResolution(t *testing.T) {
	set := loadPinnedForTest(t)

	t.Run("bare canonical resolves", func(t *testing.T) {
		sd, err := set.Resolve(USCorePatientProfile)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if sd.Version != "9.0.0" {
			t.Errorf("resolved version %s, want 9.0.0", sd.Version)
		}
	})

	t.Run("matching pinned canonical resolves", func(t *testing.T) {
		if _, err := set.Resolve(USCorePatientProfile + "|9.0.0"); err != nil {
			t.Fatalf("Resolve: %v", err)
		}
	})

	t.Run("mismatched pinned canonical is rejected", func(t *testing.T) {
		_, err := set.Resolve(USCorePatientProfile + "|8.0.0")
		if !errors.Is(err, ErrProfileVersionMismatch) {
			t.Fatalf("got %v, want ErrProfileVersionMismatch", err)
		}
	})

	t.Run("unknown canonical is rejected", func(t *testing.T) {
		_, err := set.Resolve(USCoreBaseURL + "us-core-not-a-profile")
		if !errors.Is(err, ErrProfileNotResolved) {
			t.Fatalf("got %v, want ErrProfileNotResolved", err)
		}
	})
}

// TestFHIRStructural_EveryProfileConstantResolvesInThePinnedPackage is what
// makes `FHIR-CONFORMANCE-MATRIX.md` §4's headline citable.
//
// Before this slice the matrix could count 32 profile constants and say nothing
// about whether any of them named a profile that exists, because no package was
// pinned. Every constant is now resolved against US Core 9.0.0, including
// `USCoreMedicationProfile`, which §4 records as declared-but-unused: unused is
// not the same as wrong, and a constant that does not resolve is wrong.
func TestFHIRStructural_EveryProfileConstantResolvesInThePinnedPackage(t *testing.T) {
	set := loadPinnedForTest(t)

	constants := map[string]string{
		"USCorePatientProfile":              USCorePatientProfile,
		"USCoreObservationLabProfile":       USCoreObservationLabProfile,
		"USCoreEncounterProfile":            USCoreEncounterProfile,
		"USCoreConditionProfile":            USCoreConditionProfile,
		"USCoreCoverageProfile":             USCoreCoverageProfile,
		"USCoreProcedureProfile":            USCoreProcedureProfile,
		"USCoreImmunizationProfile":         USCoreImmunizationProfile,
		"USCoreVitalSignsProfile":           USCoreVitalSignsProfile,
		"USCoreBloodPressureProfile":        USCoreBloodPressureProfile,
		"USCoreBodyHeightProfile":           USCoreBodyHeightProfile,
		"USCoreBodyWeightProfile":           USCoreBodyWeightProfile,
		"USCoreBodyTemperatureProfile":      USCoreBodyTemperatureProfile,
		"USCoreHeartRateProfile":            USCoreHeartRateProfile,
		"USCoreRespiratoryRateProfile":      USCoreRespiratoryRateProfile,
		"USCorePulseOximetryProfile":        USCorePulseOximetryProfile,
		"USCoreBMIProfile":                  USCoreBMIProfile,
		"USCoreMedicationRequestProfile":    USCoreMedicationRequestProfile,
		"USCoreMedicationProfile":           USCoreMedicationProfile,
		"USCoreAllergyIntoleranceProfile":   USCoreAllergyIntoleranceProfile,
		"USCoreCarePlanProfile":             USCoreCarePlanProfile,
		"USCoreGoalProfile":                 USCoreGoalProfile,
		"USCoreCareTeamProfile":             USCoreCareTeamProfile,
		"USCoreServiceRequestProfile":       USCoreServiceRequestProfile,
		"USCoreDocumentReferenceProfile":    USCoreDocumentReferenceProfile,
		"USCoreDiagnosticReportLabProfile":  USCoreDiagnosticReportLabProfile,
		"USCoreDiagnosticReportNoteProfile": USCoreDiagnosticReportNoteProfile,
		"USCoreProvenanceProfile":           USCoreProvenanceProfile,
		"USCoreLocationProfile":             USCoreLocationProfile,
		"USCoreOrganizationProfile":         USCoreOrganizationProfile,
		"USCorePractitionerProfile":         USCorePractitionerProfile,
		"USCorePractitionerRoleProfile":     USCorePractitionerRoleProfile,
		"USCoreRelatedPersonProfile":        USCoreRelatedPersonProfile,
	}

	if len(constants) != 32 {
		t.Fatalf("this table lists %d constants; FHIR-CONFORMANCE-MATRIX.md §4 counts 32", len(constants))
	}

	names := make([]string, 0, len(constants))
	for name := range constants {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		canonical := constants[name]
		sd, err := set.Resolve(canonical)
		if err != nil {
			t.Errorf("%s (%s): %v", name, canonical, err)
			continue
		}
		if sd.Version != PinnedUSCore.Version {
			t.Errorf("%s resolves to version %s, want %s", name, sd.Version, PinnedUSCore.Version)
		}
	}
}

// TestFHIRStructural_ProfileBaseChainsTerminateInR4Core is why the R4 core
// archive is pinned at all.
//
// Two of the profiles the mapper stamps do not derive from an R4 resource
// directly: `us-core-observation-lab` derives from
// `us-core-observation-clinical-result` and `us-core-heart-rate` from
// `us-core-vital-signs`. Walking the chain to its terminus proves the two
// archives are mutually consistent and that each profile really constrains the
// resource type it is attached to.
//
// One profile's chain leaves both pinned packages, and the exception is
// asserted by name rather than tolerated: `us-core-questionnaireresponse` bases
// on `sdc-questionnaireresponse` in `hl7.fhir.uv.sdc`, one of the seven
// packages US Core 9.0.0 declares as a dependency. Pinning the transitive
// closure was rejected — `hl7.terminology.r4` and `us.cdc.phinvads` are
// terminology, which this validator does not evaluate at all, and the mapper
// emits no QuestionnaireResponse. If US Core ever adds a second out-of-package
// base this test fails, which is the point at which that decision should be
// revisited rather than inherited.
func TestFHIRStructural_ProfileBaseChainsTerminateInR4Core(t *testing.T) {
	set := loadPinnedForTest(t)

	uscore := set.Packages[1]
	if uscore.Name != PinnedUSCore.Name {
		t.Fatalf("expected US Core second in load order, got %s", uscore.Name)
	}

	// The two pins agree by US Core's own declaration, not by our assumption.
	if got := uscore.Dependencies[PinnedR4Core.Name]; got != PinnedR4Core.Version {
		t.Errorf("%s declares a dependency on %s %q, but %s is pinned",
			PinnedUSCore, PinnedR4Core.Name, got, PinnedR4Core.Version)
	}

	unresolvedBases := map[string]string{
		USCoreBaseURL + "us-core-questionnaireresponse": "http://hl7.org/fhir/uv/sdc/StructureDefinition/sdc-questionnaireresponse",
	}

	urls := make([]string, 0, len(uscore.Definition))
	for url := range uscore.Definition {
		urls = append(urls, url)
	}
	sort.Strings(urls)

	checked, exceptions := 0, 0
	for _, url := range urls {
		sd := uscore.Definition[url]
		// Extensions and logical models are not resource profiles; only
		// constraints on resources have a resource base to reach.
		if sd.Kind != "resource" || sd.Derivation != "constraint" {
			continue
		}
		checked++

		chain, err := set.BaseChain(sd)
		if err != nil {
			if expected, known := unresolvedBases[url]; known && strings.Contains(err.Error(), expected) {
				exceptions++
				continue
			}
			t.Errorf("%s: %v", url, err)
			continue
		}
		if _, known := unresolvedBases[url]; known {
			t.Errorf("%s now resolves in the pinned packages; remove it from unresolvedBases", url)
			continue
		}
		if !chainReachesR4Base(chain, sd.Type) {
			t.Errorf("%s: chain does not reach %s", url, R4BaseCanonical(sd.Type))
		}
	}

	if checked < 50 {
		t.Fatalf("only %d US Core resource profiles were chain-checked; the filter is wrong", checked)
	}
	if exceptions != len(unresolvedBases) {
		t.Errorf("%d recorded out-of-package bases were exercised, want %d", exceptions, len(unresolvedBases))
	}
}

// TestFHIRStructural_MapperFixturesMatchTheirRecordedCardinalityGaps is the
// headline gate: the pinned packages applied to every fixture Slice 5.1a
// generated.
//
// It asserts EXACT equality against `recordedCardinalityGaps`, not "no new
// failures". A gap that gets fixed fails this test just as loudly as a gap that
// appears, which is the only version of the assertion that forces the ledger to
// shrink instead of quietly outliving the defect it describes. It is the same
// shape as `make fhir-conformance-negative-control`'s "exactly {MapLabResult}"
// requirement, for the same reason.
func TestFHIRStructural_MapperFixturesMatchTheirRecordedCardinalityGaps(t *testing.T) {
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

	// 25 files across 21 resource types: `labresult_1/2/3`, `labobservation`
	// and `vitalsign` are all Observations or DiagnosticReports.
	// `.loom/34-sprint6-execution-specs.md` correction 16 says "21 fixtures",
	// which is the resource-type count, not the file count.
	if len(fixtures) != 25 {
		t.Fatalf("found %d mapper fixtures, want 25", len(fixtures))
	}

	recorded := recordedCardinalityGaps()
	for name := range recorded {
		if _, err := os.Stat(filepath.Join(mapperFixtureDir(), name)); err != nil {
			t.Errorf("recordedCardinalityGaps names %s, which is not a fixture: %v", name, err)
		}
	}

	clean := 0
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(mapperFixtureDir(), name)) // #nosec G304 -- fixture directory listing.
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			data = mutateFixtureForNegativeControl(name, data)

			outcome, err := ValidateStructuralJSON(data, set, StructuralOptions{})
			if err != nil {
				t.Fatalf("ValidateStructuralJSON: %v", err)
			}

			got := renderIssues(StructuralErrors(outcome))
			want := append([]string(nil), recorded[name]...)
			sort.Strings(want)

			if !equalStrings(got, want) {
				t.Errorf("structural errors do not match the recorded ledger\n got (%d):\n%s\nwant (%d):\n%s",
					len(got), indentAll(got), len(want), indentAll(want))
			}
		})
		if len(recorded[name]) == 0 {
			clean++
		}
	}

	if clean != 19 {
		t.Errorf("%d fixtures are recorded clean, want 19 — update this count with the ledger", clean)
	}
}

// TestFHIRStructural_NegativeControl_RemovingARequiredElementFails is the
// in-process negative control, one row per required-element type Slice 5.1a
// counted.
//
// Its job is to prove the gate is capable of failing at all. A validator that
// resolved nothing and returned no issues would satisfy the ledger test above
// on the 19 clean fixtures; it cannot satisfy this one.
func TestFHIRStructural_NegativeControl_RemovingARequiredElementFails(t *testing.T) {
	set := loadPinnedForTest(t)

	cases := []struct {
		fixture string
		remove  string
		wantIn  string
	}{
		{fixture: "patient.json", remove: "name", wantIn: "Patient.name is required"},
		{fixture: "patient.json", remove: "identifier", wantIn: "Patient.identifier is required"},
		{fixture: "encounter.json", remove: "class", wantIn: "Encounter.class is required"},
		{fixture: "encounter.json", remove: "subject", wantIn: "Encounter.subject is required"},
		{fixture: "labobservation.json", remove: "status", wantIn: "Observation.status is required"},
		{fixture: "labobservation.json", remove: "code", wantIn: "Observation.code is required"},
		{fixture: "labresult_1.json", remove: "status", wantIn: "DiagnosticReport.status is required"},
		{fixture: "condition.json", remove: "subject", wantIn: "Condition.subject is required"},
		{fixture: "coverage.json", remove: "beneficiary", wantIn: "Coverage.beneficiary is required"},
		{fixture: "coverage.json", remove: "payor", wantIn: "Coverage.payor is required"},
	}

	for _, tc := range cases {
		t.Run(tc.fixture+"-"+tc.remove, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(mapperFixtureDir(), tc.fixture)) // #nosec G304 -- table of fixed fixture names.
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			var resource map[string]any
			if err := json.Unmarshal(data, &resource); err != nil {
				t.Fatalf("unmarshal fixture: %v", err)
			}
			if _, present := resource[tc.remove]; !present {
				t.Fatalf("fixture does not carry %q, so removing it proves nothing", tc.remove)
			}
			delete(resource, tc.remove)

			mutilated, err := json.Marshal(resource)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			outcome, err := ValidateStructuralJSON(mutilated, set, StructuralOptions{})
			if err != nil {
				t.Fatalf("ValidateStructuralJSON: %v", err)
			}

			errs := renderIssues(StructuralErrors(outcome))
			if !containsSubstring(errs, tc.wantIn) {
				t.Fatalf("removing %s produced no error naming %q\ngot:\n%s",
					tc.remove, tc.wantIn, indentAll(errs))
			}
		})
	}
}

// TestFHIRStructural_BundleEntriesAreValidatedAndLocated covers the Bundle
// path, which the fixture set does not reach — every file in
// `testdata/fhir/mapper/` is a bare resource.
//
// It also pins the issue location format inside a Bundle. An earlier revision
// rendered a Bundle entry's top-level finding as `entry[1].resource..type`,
// with a doubled separator, because the helper that joins the two path halves
// handled only an empty prefix and not an empty suffix. A location a reader
// cannot paste into a search is a defect in a validator whose whole output is
// locations.
func TestFHIRStructural_BundleEntriesAreValidatedAndLocated(t *testing.T) {
	set := loadPinnedForTest(t)

	read := func(name string) map[string]any {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(mapperFixtureDir(), name)) // #nosec G304 -- fixed fixture name.
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var resource map[string]any
		if err := json.Unmarshal(data, &resource); err != nil {
			t.Fatalf("unmarshal %s: %v", name, err)
		}
		return resource
	}

	patient := read("patient.json")
	observation := read("labobservation.json")

	bundle := func(entries ...map[string]any) []byte {
		t.Helper()
		wrapped := make([]any, 0, len(entries))
		for _, entry := range entries {
			wrapped = append(wrapped, map[string]any{"resource": entry})
		}
		data, err := json.Marshal(map[string]any{
			"resourceType": "Bundle",
			"type":         "transaction",
			"entry":        wrapped,
		})
		if err != nil {
			t.Fatalf("marshal bundle: %v", err)
		}
		return data
	}

	t.Run("a bundle of clean resources is clean", func(t *testing.T) {
		outcome, err := ValidateStructuralJSON(bundle(patient, observation), set, StructuralOptions{})
		if err != nil {
			t.Fatalf("ValidateStructuralJSON: %v", err)
		}
		if errs := renderIssues(StructuralErrors(outcome)); len(errs) != 0 {
			t.Fatalf("expected no errors, got:\n%s", indentAll(errs))
		}
	})

	t.Run("an entry violation is located by entry index", func(t *testing.T) {
		broken := read("labobservation.json")
		delete(broken, "status")

		outcome, err := ValidateStructuralJSON(bundle(patient, broken), set, StructuralOptions{})
		if err != nil {
			t.Fatalf("ValidateStructuralJSON: %v", err)
		}

		errs := renderIssues(StructuralErrors(outcome))
		if !containsSubstring(errs, "entry[1].resource.status :: Observation.status is required") {
			t.Fatalf("expected a finding located at entry[1].resource.status, got:\n%s", indentAll(errs))
		}
		for _, issue := range errs {
			if strings.Contains(issue, "..") {
				t.Errorf("issue location has a doubled separator: %s", issue)
			}
		}
	})

	t.Run("a bundle missing its own required element fails", func(t *testing.T) {
		var envelope map[string]any
		if err := json.Unmarshal(bundle(patient), &envelope); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		delete(envelope, "type") // Bundle.type is 1..1 in R4.

		data, err := json.Marshal(envelope)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		outcome, err := ValidateStructuralJSON(data, set, StructuralOptions{})
		if err != nil {
			t.Fatalf("ValidateStructuralJSON: %v", err)
		}
		if errs := renderIssues(StructuralErrors(outcome)); !containsSubstring(errs, "Bundle.type is required") {
			t.Fatalf("expected Bundle.type to be required, got:\n%s", indentAll(errs))
		}
	})
}

// TestFHIRStructural_RequiredElementChecksAgreeWithPinnedProfiles compares
// `validate.go`'s hand-written required-element list against the pinned
// StructureDefinitions it has always claimed to follow.
//
// This is the comparison Slice 5.1a could not run. Fourteen of the sixteen
// checks agree with US Core 9.0.0 exactly. Two do not, and both are the shipped
// checker being STRICTER than the IG:
//
//   - `Patient.gender` is `0..1` in `us-core-patient` and carries no
//     mustSupport flag at all in 9.0.0, while `validate.go:187` makes an absent
//     gender a hard error.
//   - `Patient.birthDate` is `0..1` and mustSupport, while `validate.go:188`
//     makes an absent birthDate a hard error.
//
// Neither is a conformance defect — refusing to emit a Patient without a
// birthDate cannot produce a non-conformant Patient — so nothing is changed
// here. They are recorded as deliberate local policy, and this test fails if US
// Core ever makes them required, which is the moment the note stops being true.
func TestFHIRStructural_RequiredElementChecksAgreeWithPinnedProfiles(t *testing.T) {
	set := loadPinnedForTest(t)

	type check struct {
		profile string
		path    string
		// fixture is the generated resource that must actually populate this
		// element. Agreeing with the IG about what is required proves nothing
		// on its own if the mapper emits none of it, so must-support presence
		// is asserted against real output rather than inferred.
		fixture string
		// mustSupport is the flag the pinned profile carries. It is spelled out
		// per row rather than assumed true, because one of the seventeen is
		// false and that is the finding.
		mustSupport bool
		// localOnly marks a check `validate.go` enforces that US Core 9.0.0
		// does not require. The test then asserts the IG min really is 0, so
		// the divergence cannot silently disappear in either direction.
		localOnly bool
	}

	checks := []check{
		{profile: USCorePatientProfile, path: "Patient.identifier", fixture: "patient.json", mustSupport: true},
		{profile: USCorePatientProfile, path: "Patient.name", fixture: "patient.json", mustSupport: true},
		// The only row in the table with no mustSupport flag at all: US Core
		// 9.0.0 dropped Patient.gender from must-support entirely.
		{profile: USCorePatientProfile, path: "Patient.gender", fixture: "patient.json", mustSupport: false, localOnly: true},
		{profile: USCorePatientProfile, path: "Patient.birthDate", fixture: "patient.json", mustSupport: true, localOnly: true},

		{profile: USCoreEncounterProfile, path: "Encounter.status", fixture: "encounter.json", mustSupport: true},
		{profile: USCoreEncounterProfile, path: "Encounter.class", fixture: "encounter.json", mustSupport: true},
		{profile: USCoreEncounterProfile, path: "Encounter.subject", fixture: "encounter.json", mustSupport: true},

		{profile: USCoreObservationLabProfile, path: "Observation.status", fixture: "labobservation.json", mustSupport: true},
		{profile: USCoreObservationLabProfile, path: "Observation.code", fixture: "labobservation.json", mustSupport: true},
		{profile: USCoreObservationLabProfile, path: "Observation.subject", fixture: "labobservation.json", mustSupport: true},

		{profile: USCoreDiagnosticReportLabProfile, path: "DiagnosticReport.status", fixture: "labresult_1.json", mustSupport: true},
		{profile: USCoreDiagnosticReportLabProfile, path: "DiagnosticReport.code", fixture: "labresult_1.json", mustSupport: true},
		{profile: USCoreDiagnosticReportLabProfile, path: "DiagnosticReport.subject", fixture: "labresult_1.json", mustSupport: true},

		{profile: USCoreConditionProfile, path: "Condition.subject", fixture: "condition.json", mustSupport: true},

		{profile: USCoreCoverageProfile, path: "Coverage.status", fixture: "coverage.json", mustSupport: true},
		{profile: USCoreCoverageProfile, path: "Coverage.beneficiary", fixture: "coverage.json", mustSupport: true},
		{profile: USCoreCoverageProfile, path: "Coverage.payor", fixture: "coverage.json", mustSupport: true},
	}

	if len(checks) != 17 {
		t.Fatalf("this table has %d rows; validate.go has 17 required-element checks across six types", len(checks))
	}

	localOnly := 0
	for _, tc := range checks {
		t.Run(tc.path, func(t *testing.T) {
			sd, err := set.Resolve(tc.profile)
			if err != nil {
				t.Fatalf("Resolve %s: %v", tc.profile, err)
			}
			element, found := snapshotElement(sd, tc.path)
			if !found {
				t.Fatalf("%s has no snapshot element %s", tc.profile, tc.path)
			}

			switch {
			case tc.localOnly && element.MinOrZero() != 0:
				t.Errorf("%s is now min %d in %s — it is no longer a local-only check, "+
					"update this table and the note above it",
					tc.path, element.MinOrZero(), PinnedUSCore)
			case !tc.localOnly && element.MinOrZero() < 1:
				t.Errorf("validate.go requires %s but %s makes it min %d",
					tc.path, PinnedUSCore, element.MinOrZero())
			}

			if element.MustSupport != tc.mustSupport {
				t.Errorf("%s has mustSupport=%t in %s, this table says %t",
					tc.path, element.MustSupport, PinnedUSCore, tc.mustSupport)
			}

			// Must-support presence, asserted against generated output.
			field := strings.TrimPrefix(tc.path, sd.Type+".")
			if !fixturePopulates(t, tc.fixture, field) {
				t.Errorf("%s does not populate %s, which %s marks mustSupport=%t and min %d",
					tc.fixture, field, PinnedUSCore, element.MustSupport, element.MinOrZero())
			}
		})
		if tc.localOnly {
			localOnly++
		}
	}

	if localOnly != 2 {
		t.Errorf("%d local-only checks, want 2 (Patient.gender, Patient.birthDate)", localOnly)
	}
}

// fixturePopulates reports whether a generated fixture carries a non-empty
// value at one top-level field.
func fixturePopulates(t *testing.T, fixture, field string) bool {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(mapperFixtureDir(), fixture)) // #nosec G304 -- table of fixed fixture names.
	if err != nil {
		t.Fatalf("read %s: %v", fixture, err)
	}
	var resource map[string]any
	if err := json.Unmarshal(data, &resource); err != nil {
		t.Fatalf("unmarshal %s: %v", fixture, err)
	}

	value, present := resource[field]
	if !present {
		return false
	}
	empty, _ := isEmptyValue(value)
	return !empty
}

// TestFHIRStructural_MustSupportReportingNeverChangesTheVerdict pins the
// severity contract for must-support.
//
// US Core mustSupport binds the system, not the instance: a conformant server
// must be *able* to populate the element when it has the data. Reporting an
// unpopulated must-support element is useful; failing on it would fail the IG's
// own examples. This test asserts the option only ever adds
// information-severity issues, and that it adds some — a reporting flag that
// reports nothing would be a silently dead feature.
func TestFHIRStructural_MustSupportReportingNeverChangesTheVerdict(t *testing.T) {
	set := loadPinnedForTest(t)

	entries, err := os.ReadDir(mapperFixtureDir())
	if err != nil {
		t.Fatalf("read fixture directory: %v", err)
	}

	informational := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(mapperFixtureDir(), entry.Name())) // #nosec G304 -- fixture directory listing.
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}

		quiet, err := ValidateStructuralJSON(data, set, StructuralOptions{})
		if err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		loud, err := ValidateStructuralJSON(data, set, StructuralOptions{ReportMustSupport: true})
		if err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}

		if !equalStrings(renderIssues(StructuralErrors(quiet)), renderIssues(StructuralErrors(loud))) {
			t.Errorf("%s: ReportMustSupport changed the error set", entry.Name())
		}
		for _, issue := range loud.Issue {
			if issue.Severity == "information" {
				informational++
			}
		}
	}

	if informational == 0 {
		t.Error("ReportMustSupport produced no information issues across 25 fixtures")
	}
}

func snapshotElement(sd *StructureDefinition, path string) (ElementDefinition, bool) {
	for _, element := range sd.Snapshot.Element {
		if element.Path == path && !element.IsSlice() {
			return element, true
		}
	}
	return ElementDefinition{}, false
}

// renderIssues flattens issues to sorted `location :: diagnostics` strings, the
// form the recorded ledger stores.
func renderIssues(issues []OperationOutcomeIssue) []string {
	out := make([]string, 0, len(issues))
	for _, issue := range issues {
		out = append(out, fmt.Sprintf("%s :: %s", strings.Join(issue.Location, ","), issue.Diagnostics))
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsSubstring(haystack []string, needle string) bool {
	for _, item := range haystack {
		if strings.Contains(item, needle) {
			return true
		}
	}
	return false
}

func indentAll(lines []string) string {
	if len(lines) == 0 {
		return "    (none)"
	}
	return "    " + strings.Join(lines, "\n    ")
}
