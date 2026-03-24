package main

import (
	"encoding/json"
	"testing"
	"time"

	db "gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

func TestRunTerminologyMappingDecisions_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMappingDecisions([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyMappingDecision_NoArgs(t *testing.T) {
	err := runTerminologyMappingDecision([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "decision ID required")
}

func TestRunTerminologyMappingDecision_InvalidID(t *testing.T) {
	err := runTerminologyMappingDecision([]string{"abc"})
	assertError(t, err)
	assertErrorContains(t, err, "invalid decision ID")
}

func TestRunTerminologyMappingDecision_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMappingDecision([]string{"42"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyMappingDecisionStats_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMappingDecisionStats([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyMappingDecisions_Dispatch(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMapping([]string{"decisions"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyMappingDecision_Dispatch(t *testing.T) {
	err := runTerminologyMapping([]string{"decision"})
	assertError(t, err)
	assertErrorContains(t, err, "decision ID required")
}

func TestRunTerminologyMappingDecisionStats_Dispatch(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMapping([]string{"decision-stats"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestPrintTerminologyMappingUsage_IncludesDecisionTelemetryCommands(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		printTerminologyMappingUsage()
	})
	assertContains(t, stdout, "decisions")
	assertContains(t, stdout, "decision-stats")
}

func TestParseTimeFlag_DateOnly(t *testing.T) {
	got, err := parseTimeFlag("2026-03-15", true)
	assertNoError(t, err)
	if got.Hour() != 23 || got.Minute() != 59 {
		t.Fatalf("expected end-of-day timestamp, got %s", got.Format(time.RFC3339Nano))
	}
}

func TestParseTimeFlag_RFC3339(t *testing.T) {
	got, err := parseTimeFlag("2026-03-15T12:30:00Z", false)
	assertNoError(t, err)
	if got.Format(time.RFC3339) != "2026-03-15T12:30:00Z" {
		t.Fatalf("unexpected parsed time: %s", got.Format(time.RFC3339))
	}
}

func TestParseTimeFlag_Invalid(t *testing.T) {
	_, err := parseTimeFlag("not-a-time", false)
	assertError(t, err)
	assertErrorContains(t, err, "invalid time")
}

func TestFormatOptionalConfidence(t *testing.T) {
	if got := formatOptionalConfidence(nil); got != "-" {
		t.Fatalf("formatOptionalConfidence(nil) = %q, want -", got)
	}

	confidence := 0.91
	if got := formatOptionalConfidence(&confidence); got != "0.91" {
		t.Fatalf("formatOptionalConfidence(0.91) = %q", got)
	}
}

func TestDecisionSortKey_Order(t *testing.T) {
	keys := []int{
		decisionSortKey(db.DecisionPersistentHit),
		decisionSortKey(db.DecisionAutorouteHighConf),
		decisionSortKey(db.DecisionAutorouteMedConf),
		decisionSortKey(db.DecisionAutorouteLowConf),
		decisionSortKey(db.DecisionNoMatch),
	}
	for i := 1; i < len(keys); i++ {
		if keys[i] <= keys[i-1] {
			t.Fatalf("expected ascending decision sort order, got %v", keys)
		}
	}
}

func TestFormatJSON_PrettyPrints(t *testing.T) {
	raw := marshalJSON(map[string]interface{}{
		"step": "persistent_lookup",
		"hit":  true,
	})
	formatted := formatJSON(raw)
	assertContains(t, formatted, "\n")
	assertContains(t, formatted, "\"step\"")
}

func TestPrintResolveResultJSON_IncludesTraceID(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := printResolveResultJSON(&db.CustomMapping{
			SourceSystem:  "epic_labs",
			SourceCode:    "GLU001",
			TargetSystem:  "http://loinc.org",
			TargetCode:    "2345-7",
			TargetDisplay: "Glucose",
			Equivalence:   db.EquivalenceEquivalent,
			Origin:        db.OriginCSVUpload,
		}, nil, "PERSISTENT_HIT", "cli-trace-123")
		assertNoError(t, err)
	})

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if payload["traceId"] != "cli-trace-123" {
		t.Fatalf("traceId = %v, want cli-trace-123", payload["traceId"])
	}
}
