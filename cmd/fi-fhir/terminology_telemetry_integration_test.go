//go:build integration
// +build integration

package main

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"

	db "gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

func TestIntegration_TerminologyMappingDecisionCLI(t *testing.T) {
	infra := setupTestInfra(t)

	if err := runTerminologyInit([]string{"--db", infra.DatabaseURL}); err != nil {
		t.Fatalf("runTerminologyInit failed: %v", err)
	}

	conn, err := sql.Open("postgres", infra.DatabaseURL)
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	store := db.NewMappingStore(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	traceID := fmt.Sprintf("cli-integration-%d", time.Now().UnixNano())
	// The decisions table truncates SOURCE_CODE to 12 characters
	// (runTerminologyMappingDecisions -> truncate(decision.SourceCode, 12)), so a
	// full nanosecond timestamp can never round-trip through the list view. Keep
	// the fixture inside the column width and still unique per run.
	sourceCode := fmt.Sprintf("GLU-%08d", time.Now().UnixNano()%100000000)
	confidence := 0.94
	decision := &db.MappingDecision{
		TraceID:         traceID,
		SourceSystem:    "epic_labs",
		SourceCode:      sourceCode,
		SourceDisplay:   "Glucose",
		TargetSystem:    "http://loinc.org",
		DecisionType:    db.DecisionAutorouteHighConf,
		Confidence:      &confidence,
		SelectedCode:    "2345-7",
		SelectedDisplay: "Glucose [Mass/volume] in Serum or Plasma",
		DecisionTree:    marshalJSON(map[string]interface{}{"trace_id": traceID, "step": "integration_test"}),
		ProfileID:       "integration-profile",
		RequestSource:   "cli",
		DurationMs:      42,
	}
	if err := store.RecordMappingDecision(ctx, decision); err != nil {
		t.Fatalf("RecordMappingDecision failed: %v", err)
	}

	stdout, _, err := runCLI(t,
		"terminology", "mapping", "decisions",
		"--db", infra.DatabaseURL,
		"--source-system", "epic_labs",
		"--source-code", sourceCode,
	)
	assertNoError(t, err)
	assertContains(t, stdout, "Mapping Decisions")
	assertContains(t, stdout, sourceCode)

	stdout, _, err = runCLI(t,
		"terminology", "mapping", "decision",
		fmt.Sprintf("%d", decision.ID),
		"--db", infra.DatabaseURL,
	)
	assertNoError(t, err)
	assertContains(t, stdout, "Mapping Decision")
	assertContains(t, stdout, traceID)

	stdout, _, err = runCLI(t,
		"terminology", "mapping", "decision-stats",
		"--db", infra.DatabaseURL,
		"--since", time.Now().Add(-24*time.Hour).Format(time.RFC3339),
		"--until", time.Now().Add(24*time.Hour).Format(time.RFC3339),
	)
	assertNoError(t, err)
	assertContains(t, stdout, "Mapping Decision Stats")
	assertContains(t, stdout, "AUTOROUTE_HIGH_CONF")
}
