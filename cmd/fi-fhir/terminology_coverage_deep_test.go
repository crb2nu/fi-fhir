package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/autoroute"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// =============================================================================
// runTerminologyStatus — coverage depth tests
// =============================================================================

func TestRunTerminologyStatus_MissingDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyStatus([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

// =============================================================================
// runTerminologyUse — coverage depth tests
// =============================================================================

func TestRunTerminologyUse_NoArgs(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyUse([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "Usage")
	assertContains(t, stdout, "terminology use")
}

func TestRunTerminologyUse_OneArgOnly(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyUse([]string{"loinc"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "Usage")
}

func TestRunTerminologyUse_MissingDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyUse([]string{"loinc", "2.77"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

// =============================================================================
// printResolveResultJSON — pure function tests (0% → 100%)
// =============================================================================

func TestPrintResolveResultJSON_NoMatch(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := printResolveResultJSON(nil, nil, "NO_MATCH")
		assertNoError(t, err)
	})

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", stdout)
	}
	if result["decision"] != "NO_MATCH" {
		t.Errorf("expected decision NO_MATCH, got %v", result["decision"])
	}
	if _, ok := result["mapping"]; ok {
		t.Errorf("expected no mapping key for nil inputs")
	}
}

func TestPrintResolveResultJSON_PersistentMapping(t *testing.T) {
	mapping := &db.CustomMapping{
		SourceSystem:  "http://hl7.org/fhir/sid/icd-10-cm",
		SourceCode:    "E11.9",
		TargetSystem:  "http://snomed.info/sct",
		TargetCode:    "44054006",
		TargetDisplay: "Type 2 diabetes mellitus",
		Equivalence:   "equivalent",
		Origin:        "manual",
	}

	stdout, _ := captureOutput(t, func() {
		err := printResolveResultJSON(mapping, nil, "PERSISTENT")
		assertNoError(t, err)
	})

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", stdout)
	}
	if result["decision"] != "PERSISTENT" {
		t.Errorf("expected decision PERSISTENT, got %v", result["decision"])
	}
	m, ok := result["mapping"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected mapping object")
	}
	if m["sourceCode"] != "E11.9" {
		t.Errorf("expected sourceCode E11.9, got %v", m["sourceCode"])
	}
	if m["targetCode"] != "44054006" {
		t.Errorf("expected targetCode 44054006, got %v", m["targetCode"])
	}
	if m["origin"] != "manual" {
		t.Errorf("expected origin manual, got %v", m["origin"])
	}
}

func TestPrintResolveResultJSON_PersistentWithConfidence(t *testing.T) {
	conf := 0.95
	mapping := &db.CustomMapping{
		SourceSystem:  "http://hl7.org/fhir/sid/icd-10-cm",
		SourceCode:    "E11.9",
		TargetSystem:  "http://snomed.info/sct",
		TargetCode:    "44054006",
		TargetDisplay: "Type 2 diabetes mellitus",
		Equivalence:   "equivalent",
		Origin:        "autoroute",
		Confidence:    &conf,
	}

	stdout, _ := captureOutput(t, func() {
		err := printResolveResultJSON(mapping, nil, "PERSISTENT")
		assertNoError(t, err)
	})

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", stdout)
	}
	if result["confidence"] != 0.95 {
		t.Errorf("expected confidence 0.95, got %v", result["confidence"])
	}
}

func TestPrintResolveResultJSON_Autorouted(t *testing.T) {
	suggestion := &autoroute.SuggestResult{
		BestMatch: &autoroute.Candidate{
			System:      "http://snomed.info/sct",
			Code:        "44054006",
			Display:     "Type 2 diabetes mellitus",
			Equivalence: "equivalent",
			Confidence:  0.92,
		},
		Confidence: 0.92,
		Reasoning:  "Exact ICD-10 to SNOMED mapping",
		Alternates: []autoroute.Candidate{
			{Code: "73211009", Display: "Diabetes mellitus", Confidence: 0.85},
		},
		Trace: &autoroute.DecisionTrace{
			Request: autoroute.TraceRequest{
				SourceSystem: "http://hl7.org/fhir/sid/icd-10-cm",
				SourceCode:   "E11.9",
			},
		},
	}

	stdout, _ := captureOutput(t, func() {
		err := printResolveResultJSON(nil, suggestion, "AUTOROUTE")
		assertNoError(t, err)
	})

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", stdout)
	}
	if result["decision"] != "AUTOROUTE" {
		t.Errorf("expected decision AUTOROUTE, got %v", result["decision"])
	}
	if result["reasoning"] != "Exact ICD-10 to SNOMED mapping" {
		t.Errorf("expected reasoning, got %v", result["reasoning"])
	}

	m, ok := result["mapping"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected mapping object")
	}
	if m["origin"] != "autoroute" {
		t.Errorf("expected origin autoroute, got %v", m["origin"])
	}

	alts, ok := result["alternates"].([]interface{})
	if !ok {
		t.Fatalf("expected alternates array")
	}
	if len(alts) != 1 {
		t.Errorf("expected 1 alternate, got %d", len(alts))
	}
}

func TestPrintResolveResultJSON_AutoroutedNoAlternates(t *testing.T) {
	suggestion := &autoroute.SuggestResult{
		BestMatch: &autoroute.Candidate{
			System:      "http://snomed.info/sct",
			Code:        "44054006",
			Display:     "Type 2 diabetes mellitus",
			Equivalence: "equivalent",
			Confidence:  0.99,
		},
		Confidence: 0.99,
		Reasoning:  "High confidence match",
		Alternates: nil,
		Trace: &autoroute.DecisionTrace{
			Request: autoroute.TraceRequest{
				SourceSystem: "http://hl7.org/fhir/sid/icd-10-cm",
				SourceCode:   "E11.9",
			},
		},
	}

	stdout, _ := captureOutput(t, func() {
		err := printResolveResultJSON(nil, suggestion, "AUTOROUTE")
		assertNoError(t, err)
	})

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON, got: %s", stdout)
	}
	if _, ok := result["alternates"]; ok {
		t.Errorf("expected no alternates key when none provided")
	}
}

// =============================================================================
// runTerminologyMappingUpload — deeper flag parsing tests
// =============================================================================

func TestRunTerminologyMappingUpload_DryRunWithValidCSV(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	tmpDir := t.TempDir()
	csvContent := "source_system,source_code,target_system,target_code,equivalence\nICD10CM,E11.9,SNOMEDCT,44054006,equivalent\n"
	csvPath := createTempFile(t, tmpDir, "mappings*.csv", csvContent)

	stdout, _ := captureOutput(t, func() {
		err := runTerminologyMappingUpload([]string{csvPath, "--dry-run"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "DRY RUN")
	assertContains(t, stdout, "CSV Parse Results")
}

func TestRunTerminologyMappingUpload_WithSourceSystem(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	tmpDir := t.TempDir()
	csvContent := "source_code,target_code,equivalence\nE11.9,44054006,equivalent\n"
	csvPath := createTempFile(t, tmpDir, "mappings*.csv", csvContent)

	stdout, _ := captureOutput(t, func() {
		err := runTerminologyMappingUpload([]string{csvPath, "--source-system", "ICD10CM", "--target-system", "SNOMEDCT", "--dry-run"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "DRY RUN")
}

func TestRunTerminologyMappingUpload_WithProfile(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	tmpDir := t.TempDir()
	csvContent := "source_system,source_code,target_system,target_code,equivalence\nICD10CM,E11.9,SNOMEDCT,44054006,equivalent\n"
	csvPath := createTempFile(t, tmpDir, "mappings*.csv", csvContent)

	stdout, _ := captureOutput(t, func() {
		err := runTerminologyMappingUpload([]string{csvPath, "--profile", "test-profile", "--dry-run"})
		assertNoError(t, err)
	})

	assertContains(t, stdout, "DRY RUN")
}

// =============================================================================
// runTerminologyMappingList — deeper flag parsing tests
// =============================================================================

func TestRunTerminologyMappingList_InvalidLimit(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingList([]string{"--limit", "abc"})
	assertError(t, err)
	assertErrorContains(t, err, "invalid limit")
}

func TestRunTerminologyMappingList_InvalidOffset(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingList([]string{"--offset", "abc"})
	assertError(t, err)
	assertErrorContains(t, err, "invalid offset")
}

func TestRunTerminologyMappingList_InvalidBatchID(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingList([]string{"--batch", "not-a-uuid"})
	assertError(t, err)
	assertErrorContains(t, err, "invalid batch ID")
}

// =============================================================================
// runTerminologyMappingGet — deeper tests
// =============================================================================

func TestRunTerminologyMappingGet_NoIDNoBatch(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// No numeric positional arg and no --batch → falls through to "mapping ID or --batch required"
	// But it tries to connect to DB first, so we expect a connection error
	err := runTerminologyMappingGet([]string{})
	assertError(t, err)
	// Connection will be attempted; any error is expected
}

func TestRunTerminologyMappingGet_InvalidBatchUUID(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// A --batch with invalid UUID should error after connecting
	// Since we use a fake DB URL, the connection attempt will produce an error first
	err := runTerminologyMappingGet([]string{"--batch", "not-a-uuid"})
	assertError(t, err)
}

// =============================================================================
// runTerminologyMappingDelete — deeper tests
// =============================================================================

func TestRunTerminologyMappingDelete_NoIDNoBatch(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingDelete([]string{})
	assertError(t, err)
}

// =============================================================================
// checkTerminologyPins — deeper tests
// =============================================================================

func TestCheckTerminologyPins_EmptyPinsMap(t *testing.T) {
	warnings, err := checkTerminologyPins(context.Background(), "postgres://example.invalid/db", map[string]string{}, "warn")
	assertNoError(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for empty pins, got %d", len(warnings))
	}
}

func TestCheckTerminologyPins_NilPinsMap(t *testing.T) {
	warnings, err := checkTerminologyPins(context.Background(), "postgres://example.invalid/db", nil, "warn")
	assertNoError(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for nil pins, got %d", len(warnings))
	}
}

func TestCheckTerminologyPins_EmptyStringPolicyDefaultsToWarn(t *testing.T) {
	// Empty policy should default to "warn" and then skip because dbURL is empty
	warnings, err := checkTerminologyPins(context.Background(), "", map[string]string{"loinc": "2.77"}, "")
	assertNoError(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %d", len(warnings))
	}
}

func TestCheckTerminologyPins_ErrorPolicyEmptyDB(t *testing.T) {
	// "error" policy with empty DB URL → skip (returns nil, nil)
	warnings, err := checkTerminologyPins(context.Background(), "", map[string]string{"loinc": "2.77"}, "error")
	assertNoError(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %d", len(warnings))
	}
}

func TestCheckTerminologyPins_WhitespacePolicyNormalized(t *testing.T) {
	// "  WARN  " should normalize to "warn" and skip because dbURL is empty
	warnings, err := checkTerminologyPins(context.Background(), "", map[string]string{"loinc": "2.77"}, "  WARN  ")
	assertNoError(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %d", len(warnings))
	}
}

// =============================================================================
// loadTerminologyPinConfigFromEnv — pure function tests
// =============================================================================

func TestLoadTerminologyPinConfigFromEnv_DefaultPolicy(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_POLICY", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_PINS", "")

	_, pins, policy := loadTerminologyPinConfigFromEnv()

	if policy != "warn" {
		t.Errorf("expected default policy 'warn', got %q", policy)
	}
	if pins == nil {
		t.Errorf("expected non-nil pins map")
	}
}

func TestLoadTerminologyPinConfigFromEnv_FallbackDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://fallback/db")
	t.Setenv("FI_FHIR_TERMINOLOGY_POLICY", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_PINS", "")

	dbURL, _, _ := loadTerminologyPinConfigFromEnv()

	if !strings.Contains(dbURL, "fallback") {
		t.Errorf("expected fallback DB URL, got %q", dbURL)
	}
}
