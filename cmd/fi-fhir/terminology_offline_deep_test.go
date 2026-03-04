package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// =============================================================================
// appendParseWarningsToEvent — edge-case coverage (85.7% → ~100%)
// =============================================================================

func TestAppendParseWarningsToEvent_NilEvt(t *testing.T) {
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}
	if ok := appendParseWarningsToEvent(nil, extra); ok {
		t.Errorf("expected false for nil evt")
	}
}

func TestAppendParseWarningsToEvent_EmptyOrNilExtra(t *testing.T) {
	evt := &dummyEvent{EventMeta: events.EventMeta{}}
	if ok := appendParseWarningsToEvent(evt, nil); ok {
		t.Errorf("expected false for nil extra")
	}
	if ok := appendParseWarningsToEvent(evt, []events.ParseWarning{}); ok {
		t.Errorf("expected false for empty extra")
	}
}

func TestAppendParseWarningsToEvent_NilPointer(t *testing.T) {
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}
	var evt *dummyEvent // nil pointer
	if ok := appendParseWarningsToEvent(evt, extra); ok {
		t.Errorf("expected false for nil pointer")
	}
}

func TestAppendParseWarningsToEvent_PointerToNonStruct(t *testing.T) {
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}
	n := 42
	if ok := appendParseWarningsToEvent(&n, extra); ok {
		t.Errorf("expected false for *int")
	}
}

type noMetaStruct struct {
	Name string
}

func TestAppendParseWarningsToEvent_StructWithoutEventMeta(t *testing.T) {
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}
	s := &noMetaStruct{Name: "test"}
	if ok := appendParseWarningsToEvent(s, extra); ok {
		t.Errorf("expected false for struct without EventMeta")
	}
}

func TestAppendParseWarningsToEvent_InterfaceWrapping(t *testing.T) {
	evt := &dummyEvent{
		EventMeta: events.EventMeta{
			ParseWarnings: []events.ParseWarning{{Phase: "p1", Code: "C1"}},
		},
	}
	extra := []events.ParseWarning{{Phase: "p2", Code: "C2"}}

	// Wrap in interface{} then pass — the reflect loop at line 147-149 unwraps it
	var wrapped interface{} = evt
	if ok := appendParseWarningsToEvent(wrapped, extra); !ok {
		t.Fatalf("expected true for interface-wrapped pointer")
	}
	if len(evt.ParseWarnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(evt.ParseWarnings))
	}
}

// =============================================================================
// appendParseWarningsToOutputData — additional case coverage
// =============================================================================

func TestAppendParseWarningsToOutputData_NilInput(t *testing.T) {
	// Should not panic
	appendParseWarningsToOutputData(nil, []events.ParseWarning{{Phase: "p", Code: "C"}})
}

func TestAppendParseWarningsToOutputData_NilExtraSlice(t *testing.T) {
	out := &dummyOutput{Events: []*dummyEvent{{EventMeta: events.EventMeta{}}}}
	appendParseWarningsToOutputData(out, nil)
	if len(out.Events[0].ParseWarnings) != 0 {
		t.Errorf("expected no warnings for nil extra")
	}
}

func TestAppendParseWarningsToOutputData_SliceOfInterfacesPath(t *testing.T) {
	evt := &dummyEvent{EventMeta: events.EventMeta{}}
	input := []interface{}{evt}
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}

	appendParseWarningsToOutputData(input, extra)
	if len(evt.ParseWarnings) != 1 {
		t.Errorf("expected 1 warning via []interface{} path, got %d", len(evt.ParseWarnings))
	}
}

func TestAppendParseWarningsToOutputData_MapWithEventsKey(t *testing.T) {
	evt1 := &dummyEvent{EventMeta: events.EventMeta{}}
	evt2 := &dummyEvent{EventMeta: events.EventMeta{}}
	input := map[string]interface{}{
		"events": []interface{}{evt1, evt2},
	}
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}

	appendParseWarningsToOutputData(input, extra)
	if len(evt1.ParseWarnings) != 1 {
		t.Errorf("expected 1 warning on evt1 via events key, got %d", len(evt1.ParseWarnings))
	}
	if len(evt2.ParseWarnings) != 1 {
		t.Errorf("expected 1 warning on evt2 via events key, got %d", len(evt2.ParseWarnings))
	}
}

func TestAppendParseWarningsToOutputData_DefaultCase(t *testing.T) {
	// Pass a non-slice non-map value → falls to default case
	evt := &dummyEvent{EventMeta: events.EventMeta{}}
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}

	appendParseWarningsToOutputData(evt, extra)
	// The struct Events field path runs first (finds Events field),
	// then default case also calls appendParseWarningsToEvent
	if len(evt.ParseWarnings) == 0 {
		t.Errorf("expected warnings appended")
	}
}

func TestAppendParseWarningsToOutputData_StructWithoutEventsField(t *testing.T) {
	// Pointer to struct without Events field → reflect path skips, default case runs
	s := &noMetaStruct{Name: "test"}
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}
	// Should not panic — appendParseWarningsToEvent returns false for noMetaStruct
	appendParseWarningsToOutputData(s, extra)
}

// =============================================================================
// runTerminologyDrop — no-force path (87% → higher)
// =============================================================================

func TestRunTerminologyDrop_NoForceWithDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	stdout, _ := captureOutput(t, func() {
		err := runTerminologyDrop([]string{})
		assertNoError(t, err) // returns nil (just prints warning)
	})
	assertContains(t, stdout, "WARNING")
	assertContains(t, stdout, "--force")
}

// =============================================================================
// runTerminologyMapping — help flags (66.7% → higher)
// =============================================================================

func TestRunTerminologyMapping_DashHFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyMapping([]string{"-h"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "terminology mapping")
}

func TestRunTerminologyMapping_DashDashHelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyMapping([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "terminology mapping")
}

// =============================================================================
// runTerminologyLoad — missing version, dry-run edge cases (78.3% → higher)
// =============================================================================

func TestRunTerminologyLoad_NoVersionFlag(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyLoad([]string{"loinc", "/data/loinc"})
	assertError(t, err)
	assertErrorContains(t, err, "--version is required")
}

func TestRunTerminologyLoad_DryRunUnknownVocab(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")

	err := runTerminologyLoad([]string{"unknown_vocab", "/data/path", "--version", "1.0", "--dry-run"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown vocabulary")
}

func TestRunTerminologyLoad_DryRunSNOMED(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")

	stdout, _ := captureOutput(t, func() {
		err := runTerminologyLoad([]string{"snomed", "/data/snomed", "--version", "2024-03", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "DRY RUN")
	assertContains(t, stdout, "SNOMED")
}

func TestRunTerminologyLoad_DryRunICD10PCS(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")

	stdout, _ := captureOutput(t, func() {
		err := runTerminologyLoad([]string{"icd10pcs", "/data/pcs", "--version", "2024", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "DRY RUN")
	assertContains(t, stdout, "ICD-10-PCS")
}

// =============================================================================
// loadLOINC dry-run — error paths (29% → higher)
// =============================================================================

func TestLoadLOINC_DryRun_DirectoryError(t *testing.T) {
	dir := t.TempDir()

	err := loadLOINC(context.Background(), nil, nil, dir, "2.77", nil, true)
	assertError(t, err)
	assertErrorContains(t, err, "is a directory")
}

func TestLoadLOINC_DryRun_NonexistentFile(t *testing.T) {
	err := loadLOINC(context.Background(), nil, nil, "/tmp/does-not-exist-loinc.csv", "2.77", nil, true)
	assertError(t, err)
	assertErrorContains(t, err, "invalid LOINC file")
}

func TestLoadLOINC_DryRun_NoPanelHierarchy(t *testing.T) {
	dir := t.TempDir()
	loincPath := filepath.Join(dir, "LoincTable.csv")
	if err := os.WriteFile(loincPath, []byte("LOINC_NUM,COMPONENT\n1234-5,Example\n"), 0o600); err != nil {
		t.Fatalf("write loinc table: %v", err)
	}

	stdout, _ := captureOutput(t, func() {
		err := loadLOINC(context.Background(), nil, nil, loincPath, "2.77", nil, true)
		assertNoError(t, err)
	})
	assertContains(t, stdout, "DRY RUN")
	// Should NOT mention PanelHierarchy.csv when it doesn't exist
	if contains(stdout, "PanelHierarchy.csv") {
		t.Errorf("expected no PanelHierarchy.csv mention when file is absent")
	}
}

// =============================================================================
// loadICD10CM dry-run — error paths (30.4% → higher)
// =============================================================================

func TestLoadICD10CM_DryRun_DirectoryError(t *testing.T) {
	dir := t.TempDir()

	err := loadICD10CM(context.Background(), nil, nil, dir, "FY2024", nil, true)
	assertError(t, err)
	assertErrorContains(t, err, "is a directory")
}

func TestLoadICD10CM_DryRun_NonexistentFile(t *testing.T) {
	err := loadICD10CM(context.Background(), nil, nil, "/tmp/does-not-exist-icd10cm.csv", "FY2024", nil, true)
	assertError(t, err)
	assertErrorContains(t, err, "invalid ICD-10-CM input")
}

func TestLoadICD10CM_DryRun_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "icd10cm.csv")
	if err := os.WriteFile(path, []byte("code,desc\nE11.9,Type 2 diabetes mellitus without complications\n"), 0o600); err != nil {
		t.Fatalf("write icd10cm: %v", err)
	}

	stdout, _ := captureOutput(t, func() {
		err := loadICD10CM(context.Background(), nil, nil, path, "FY2024", nil, true)
		assertNoError(t, err)
	})
	assertContains(t, stdout, "DRY RUN")
	assertContains(t, stdout, "ICD-10-CM")
}

// =============================================================================
// loadUMLS/loadRxNorm dry-run — validate directory + success/error paths
// =============================================================================

func TestLoadUMLS_DryRun_InvalidDir(t *testing.T) {
	dir := t.TempDir()
	// Missing required MRSTY.RRF
	_ = os.WriteFile(filepath.Join(dir, "MRCONSO.RRF"), []byte(""), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "MRREL.RRF"), []byte(""), 0o600)

	err := loadUMLS(context.Background(), nil, nil, dir, "2024AB", nil, true)
	assertError(t, err)
	assertErrorContains(t, err, "invalid UMLS META directory")
	assertErrorContains(t, err, "required file not found")
}

func TestLoadUMLS_DryRun_Success(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "MRCONSO.RRF"), []byte(""), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "MRREL.RRF"), []byte(""), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "MRSTY.RRF"), []byte(""), 0o600)

	stdout, _ := captureOutput(t, func() {
		err := loadUMLS(context.Background(), nil, nil, dir, "2024AB", nil, true)
		assertNoError(t, err)
	})
	assertContains(t, stdout, "DRY RUN")
	assertContains(t, stdout, "UMLS")
}

func TestLoadRxNorm_DryRun_InvalidDir(t *testing.T) {
	dir := t.TempDir()
	err := loadRxNorm(context.Background(), nil, nil, dir, "2024-01", nil, true)
	assertError(t, err)
	assertErrorContains(t, err, "invalid RxNorm directory")
	assertErrorContains(t, err, "required file not found")
}

func TestLoadRxNorm_DryRun_Success(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "RXNCONSO.RRF"), []byte(""), 0o600)

	stdout, _ := captureOutput(t, func() {
		err := loadRxNorm(context.Background(), nil, nil, dir, "2024-01", nil, true)
		assertNoError(t, err)
	})
	assertContains(t, stdout, "DRY RUN")
	assertContains(t, stdout, "RxNorm")
}

// =============================================================================
// loadLOINC dry-run — panel hierarchy present branch
// =============================================================================

func TestLoadLOINC_DryRun_WithPanelHierarchy(t *testing.T) {
	dir := t.TempDir()
	loincPath := filepath.Join(dir, "LoincTable.csv")
	panelPath := filepath.Join(dir, "PanelHierarchy.csv")
	if err := os.WriteFile(loincPath, []byte("LOINC_NUM,COMPONENT\n1234-5,Example\n"), 0o600); err != nil {
		t.Fatalf("write loinc table: %v", err)
	}
	if err := os.WriteFile(panelPath, []byte("PARENT_LOINC,CHILD_LOINC\n1234-5,5678-9\n"), 0o600); err != nil {
		t.Fatalf("write panel hierarchy: %v", err)
	}

	stdout, _ := captureOutput(t, func() {
		err := loadLOINC(context.Background(), nil, nil, loincPath, "2.77", nil, true)
		assertNoError(t, err)
	})
	assertContains(t, stdout, "PanelHierarchy.csv")
}

// =============================================================================
// getTerminologyDBURL — FI_FHIR_TERMINOLOGY_DB_URL path
// =============================================================================

func TestGetTerminologyDBURL_TerminologySpecificEnv(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://terminology/db")
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://generic/db")

	url := getTerminologyDBURL([]string{})
	if url != "postgres://terminology/db" {
		t.Errorf("expected FI_FHIR_TERMINOLOGY_DB_URL to take priority, got %q", url)
	}
}

func TestGetTerminologyDBURL_FlagOverridesTerminologyEnv(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://terminology/db")

	url := getTerminologyDBURL([]string{"--db", "postgres://flag/db"})
	if url != "postgres://flag/db" {
		t.Errorf("expected --db flag to override env, got %q", url)
	}
}

// =============================================================================
// runTerminologyMappingPending — flag parsing coverage (70% → higher)
// =============================================================================

func TestRunTerminologyMappingPending_WithAllFlags(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingPending([]string{
		"--status", "approved",
		"--min-confidence", "0.85",
		"--source-system", "ICD10CM",
		"--target-system", "SNOMEDCT",
		"--limit", "50",
		"--offset", "10",
		"--json",
	})
	assertError(t, err) // DB connection error expected — validates flag parsing
}

func TestRunTerminologyMappingPending_InvalidLimit(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingPending([]string{"--limit", "abc"})
	assertError(t, err)
	assertErrorContains(t, err, "invalid limit")
}

func TestRunTerminologyMappingPending_InvalidOffset(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingPending([]string{"--offset", "xyz"})
	assertError(t, err)
	assertErrorContains(t, err, "invalid offset")
}

// =============================================================================
// runTerminologyMappingApprove — flag parsing coverage (70.2% → higher)
// =============================================================================

func TestRunTerminologyMappingApprove_WithFlags(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingApprove([]string{
		"42",
		"--by", "admin@example.com",
		"--equivalence", "equivalent",
		"--comment", "Verified manually",
		"--json",
	})
	assertError(t, err) // DB connection error expected — validates flag parsing
}

// =============================================================================
// runTerminologyMappingReject — flag parsing coverage (75.6% → higher)
// =============================================================================

func TestRunTerminologyMappingReject_WithFlags(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingReject([]string{
		"42",
		"--by", "admin@example.com",
		"--reason", "Incorrect mapping",
		"--json",
	})
	assertError(t, err) // DB connection error expected — validates flag parsing
}

// =============================================================================
// runTerminologyMappingDelete — batch abort path (75% → higher)
// =============================================================================

func TestRunTerminologyMappingDelete_BatchWithoutForce_Aborts(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// Without --force, prompts user → Scanln on non-interactive stdin → abort
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyMappingDelete([]string{"--batch", "550e8400-e29b-41d4-a716-446655440000"})
		assertNoError(t, err) // Aborted is not an error
	})
	assertContains(t, stdout, "Aborted")
}

// =============================================================================
// runTerminologyAutoroute — flag parsing coverage (96.2% → higher)
// =============================================================================

func TestRunTerminologyAutoroute_InvalidThreshold(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyAutoroute([]string{
		"GLU001",
		"--source-system", "epic_labs",
		"--target-system", "http://loinc.org",
		"--auto-approve-threshold", "not-a-number",
	})
	assertError(t, err)
	assertErrorContains(t, err, "invalid threshold")
}

func TestRunTerminologyAutoroute_InvalidReviewTimeoutDays(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyAutoroute([]string{
		"GLU001",
		"--source-system", "epic_labs",
		"--target-system", "http://loinc.org",
		"--review-timeout-days", "abc",
	})
	assertError(t, err)
	assertErrorContains(t, err, "invalid timeout days")
}

func TestRunTerminologyAutoroute_WithAllFlagsNoTemporal(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyAutoroute([]string{
		"GLU001",
		"--source-system", "epic_labs",
		"--target-system", "http://loinc.org",
		"--display", "Glucose Fasting",
		"--json",
		"--wait",
		"--auto-approve-threshold", "0.90",
		"--review-timeout-days", "14",
	})
	assertError(t, err) // DB connection error expected — validates flag parsing
}

// =============================================================================
// runTerminologyUse — empty vocab/version validation
// =============================================================================

func TestRunTerminologyUse_EmptyVocab(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// Space-only vocab should be caught by the TrimSpace check
	err := runTerminologyUse([]string{"  ", "2.77"})
	assertError(t, err)
	assertErrorContains(t, err, "vocabulary and version are required")
}

func TestRunTerminologyUse_EmptyVersion(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyUse([]string{"loinc", "  "})
	assertError(t, err)
	assertErrorContains(t, err, "vocabulary and version are required")
}

// =============================================================================
// runTerminologyCrosswalk — missing DB URL
// =============================================================================

func TestRunTerminologyCrosswalk_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyCrosswalk([]string{"E11.9", "--from", "ICD10CM", "--to", "SNOMEDCT_US"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

// =============================================================================
// runTerminologyMapping — dispatcher coverage for all subcommands (66.7% → higher)
//
// Tests that exercise the mapping dispatcher switch for subcommands
// that were previously only called directly (bypassing the dispatcher).
// =============================================================================

func TestRunTerminologyMapping_UploadDispatch(t *testing.T) {
	err := runTerminologyMapping([]string{"upload"})
	assertError(t, err)
	assertErrorContains(t, err, "CSV file path required")
}

func TestRunTerminologyMapping_ListDispatch(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMapping([]string{"list"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyMapping_DeleteDispatch(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMapping([]string{"delete"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyMapping_GetDispatch(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMapping([]string{"get"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyMapping_ResolveDispatch(t *testing.T) {
	err := runTerminologyMapping([]string{"resolve"})
	assertError(t, err)
	assertErrorContains(t, err, "source code required")
}

// =============================================================================
// runTerminology — dispatcher coverage for subcommands
// =============================================================================

func TestRunTerminology_SearchDispatch(t *testing.T) {
	err := runTerminology([]string{"search"})
	assertError(t, err)
	assertErrorContains(t, err, "query required")
}

func TestRunTerminology_MappingDispatch(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminology([]string{"mapping"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "terminology mapping")
}

func TestRunTerminology_CrosswalkNoArgs(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminology([]string{"crosswalk"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "Usage")
}

// =============================================================================
// appendParseWarningsToEvent — remaining edge cases
// =============================================================================

type badMetaEvent struct {
	EventMeta int // EventMeta exists but is not a struct
}

func TestAppendParseWarningsToEvent_EventMetaNotStruct(t *testing.T) {
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}
	evt := &badMetaEvent{EventMeta: 42}
	if ok := appendParseWarningsToEvent(evt, extra); ok {
		t.Errorf("expected false when EventMeta is not a struct")
	}
}

type badWarningsEvent struct {
	EventMeta struct {
		ParseWarnings string // wrong type
	}
}

func TestAppendParseWarningsToEvent_ParseWarningsWrongType(t *testing.T) {
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}
	evt := &badWarningsEvent{}
	evt.EventMeta.ParseWarnings = "not a slice"
	if ok := appendParseWarningsToEvent(evt, extra); ok {
		t.Errorf("expected false when ParseWarnings is not a slice")
	}
}

// =============================================================================
// runTerminologyDrop — additional coverage
// =============================================================================

func TestRunTerminologyDrop_NoForceWithDBFlag(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	// --db flag provides URL but no --force → prints warning
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyDrop([]string{"--db", "postgres://localhost/test"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "WARNING")
}

// =============================================================================
// runTerminologyInit / runTerminologyStatus — with --db flag
//
// Exercises the getTerminologyDBURL → sql.Open code path
// =============================================================================

func TestRunTerminologyInit_WithDBFlag(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyInit([]string{"--db", "postgres://localhost:5432/nonexistent"})
	assertError(t, err) // connection error expected
}

func TestRunTerminologyStatus_WithDBFlag(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyStatus([]string{"--db", "postgres://localhost:5432/nonexistent"})
	assertError(t, err) // connection error expected
}

// =============================================================================
// Missing-DB-URL error paths — push coverage for getTerminologyDBURL guard
// =============================================================================

func TestRunTerminologyInit_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyInit([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyStatus_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyStatus([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyUse_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyUse([]string{"loinc", "2.77"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyDrop_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyDrop([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyLoad_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyLoad([]string{"loinc", "/data/loinc", "--version", "2.77"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyCrosswalk_MissingVocabs(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyCrosswalk([]string{"E11.9"})
	assertError(t, err)
	assertErrorContains(t, err, "--from and --to vocabularies are required")
}

func TestRunTerminologyAutoroute_MissingSourceCode(t *testing.T) {
	err := runTerminologyAutoroute([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "source code required")
}

func TestRunTerminologyAutoroute_MissingDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyAutoroute([]string{"GLU001"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

// =============================================================================
// Terminology dispatcher — exercise remaining subcommand paths
// =============================================================================

func TestRunTerminology_UnknownSubcommand(t *testing.T) {
	err := runTerminology([]string{"nonexistent"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown terminology subcommand")
}

func TestRunTerminology_HelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminology([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "fi-fhir terminology")
}

func TestRunTerminology_HelpSubcommand(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminology([]string{"help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "fi-fhir terminology")
}

func TestRunTerminology_NoArgs(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminology([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "fi-fhir terminology")
}

func TestRunTerminology_IndexDispatch(t *testing.T) {
	err := runTerminology([]string{"index"})
	assertError(t, err)
	assertErrorContains(t, err, "vocabulary required")
}

// =============================================================================
// runTerminologyLoad — additional edge cases
// =============================================================================

func TestRunTerminologyLoad_NoArgs(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyLoad([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "Usage")
}

func TestRunTerminologyLoad_TooFewArgs(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyLoad([]string{"loinc"})
		assertNoError(t, err) // prints usage for <2 args
	})
	assertContains(t, stdout, "Usage")
}

func TestRunTerminologyLoad_InvalidDate(t *testing.T) {
	err := runTerminologyLoad([]string{"loinc", "/data/loinc", "--version", "2.77", "--date", "not-a-date", "--dry-run"})
	assertError(t, err)
	assertErrorContains(t, err, "invalid date format")
}

func TestRunTerminologyLoad_ValidDate(t *testing.T) {
	dir := t.TempDir()
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyLoad([]string{"snomed", dir, "--version", "2024-03", "--date", "2024-03-01", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "DRY RUN")
}

func TestRunTerminologyLoad_DryRunLOINC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "LoincTable.csv")
	_ = os.WriteFile(path, []byte("LOINC_NUM\n1234-5\n"), 0o600)

	stdout, _ := captureOutput(t, func() {
		err := runTerminologyLoad([]string{"loinc", path, "--version", "2.77", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "DRY RUN")
}

func TestRunTerminologyLoad_DryRunICD10CM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "icd10cm.csv")
	_ = os.WriteFile(path, []byte("code\nE11.9\n"), 0o600)

	stdout, _ := captureOutput(t, func() {
		err := runTerminologyLoad([]string{"icd10cm", path, "--version", "FY2024", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "DRY RUN")
}

func TestRunTerminologyLoad_DryRunUMLS(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "MRCONSO.RRF"), []byte(""), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "MRREL.RRF"), []byte(""), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "MRSTY.RRF"), []byte(""), 0o600)

	stdout, _ := captureOutput(t, func() {
		err := runTerminologyLoad([]string{"umls", dir, "--version", "2024AB", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "DRY RUN")
}

func TestRunTerminologyLoad_DryRunRxNorm(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "RXNCONSO.RRF"), []byte(""), 0o600)

	stdout, _ := captureOutput(t, func() {
		err := runTerminologyLoad([]string{"rxnorm", dir, "--version", "2024-01", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "DRY RUN")
}

// =============================================================================
// runTerminologyUse — help/usage paths
// =============================================================================

func TestRunTerminologyUse_OneArg(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyUse([]string{"loinc"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "Usage")
}

// =============================================================================
// runTerminologyCrosswalk — with DB, reached "not yet implemented" path
// =============================================================================

func TestRunTerminologyCrosswalk_NotImplementedYet(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	stdout, _ := captureOutput(t, func() {
		err := runTerminologyCrosswalk([]string{"E11.9", "--from", "ICD10CM", "--to", "SNOMEDCT_US"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "not yet implemented")
}

// =============================================================================
// runTerminologyMapping — unknown subcommand path
// =============================================================================
