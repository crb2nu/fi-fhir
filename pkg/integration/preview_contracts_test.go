package integration_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestProcessResultArtifactRevisionsAreAdditiveAndExactlyBound(t *testing.T) {
	revision, legacy := validProductionResult(t)

	encodedLegacy, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy result: %v", err)
	}
	if bytes.Contains(encodedLegacy, []byte(`"artifact_revisions"`)) {
		t.Fatalf("legacy result unexpectedly emitted artifact revisions: %s", encodedLegacy)
	}
	if err := legacy.ValidateAgainst(revision); err != nil {
		t.Fatalf("legacy result without additive provenance: %v", err)
	}

	result := cloneProcessResult(legacy)
	result.ArtifactRevisions = executionArtifactRevisions(revision)
	if err := result.ValidateAgainst(revision); err != nil {
		t.Fatalf("result with exact artifact revisions: %v", err)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	for _, required := range []string{`"artifact_revisions"`, `"source"`, `"profile"`, `"workflow"`} {
		if !bytes.Contains(encoded, []byte(required)) {
			t.Fatalf("result JSON missing %s: %s", required, encoded)
		}
	}
	var decoded integration.ProcessResult
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if err := decoded.ValidateAgainst(revision); err != nil {
		t.Fatalf("validate decoded result: %v", err)
	}

	structural := cloneProcessResult(result)
	structural.ArtifactRevisions.Profile.Digest = "not-a-digest"
	if err := structural.Validate(); err == nil {
		t.Fatal("structurally invalid artifact provenance was accepted")
	}

	for _, field := range []string{"source", "profile", "workflow"} {
		t.Run(field, func(t *testing.T) {
			candidate := cloneProcessResult(result)
			switch field {
			case "source":
				candidate.ArtifactRevisions.Source.RevisionID = "source-rev-other"
			case "profile":
				candidate.ArtifactRevisions.Profile.RevisionID = "profile-rev-other"
			case "workflow":
				candidate.ArtifactRevisions.Workflow.RevisionID = "workflow-rev-other"
			}
			if err := candidate.Validate(); err != nil {
				t.Fatalf("mismatched but structurally valid provenance should pass base validation: %v", err)
			}
			if err := candidate.ValidateAgainst(revision); err == nil {
				t.Fatalf("%s provenance drift was accepted", field)
			}
		})
	}
}

func TestOptionalEventLineageIsStructurallyValidated(t *testing.T) {
	_, result := validProductionResult(t)
	result.Routes[0].EventID = "event-123"
	result.Deliveries[0].EventID = "event-123"
	if err := result.Validate(); err != nil {
		t.Fatalf("valid optional event lineage: %v", err)
	}

	for _, mutate := range []func(*integration.ProcessResult){
		func(candidate *integration.ProcessResult) { candidate.Routes[0].EventID = " event-123 " },
		func(candidate *integration.ProcessResult) { candidate.Deliveries[0].EventID = "\t" },
		func(candidate *integration.ProcessResult) { candidate.Deliveries[0].EventID = "event\x00-123" },
	} {
		candidate := cloneProcessResult(result)
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatal("noncanonical optional event lineage was accepted")
		}
	}
}

func TestValidatePreviewForRequiresCompleteSideEffectFreeLineage(t *testing.T) {
	revision, result, request := validStrictPreviewResult(t)
	if err := result.ValidatePreviewFor(request, revision); err != nil {
		t.Fatalf("valid strict preview: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*integration.ProcessResult)
	}{
		{name: "mode", mutate: func(candidate *integration.ProcessResult) { candidate.Mode = integration.ExecutionModeProduction }},
		{name: "provenance", mutate: func(candidate *integration.ProcessResult) { candidate.ArtifactRevisions = nil }},
		{name: "route event", mutate: func(candidate *integration.ProcessResult) { candidate.Routes[0].EventID = "" }},
		{name: "delivery event", mutate: func(candidate *integration.ProcessResult) { candidate.Deliveries[0].EventID = "" }},
		{name: "unknown route event", mutate: func(candidate *integration.ProcessResult) { candidate.Routes[0].EventID = "event-other" }},
		{name: "unknown delivery event", mutate: func(candidate *integration.ProcessResult) { candidate.Deliveries[0].EventID = "event-other" }},
		{name: "unmatched route", mutate: func(candidate *integration.ProcessResult) { candidate.Routes[0].Matched = false }},
		{name: "skipped route", mutate: func(candidate *integration.ProcessResult) {
			candidate.Routes[0].Skipped = true
			candidate.Routes[0].SkipReason = "not executable"
		}},
		{name: "unplanned action", mutate: func(candidate *integration.ProcessResult) { candidate.Deliveries[0].Action = "other-action" }},
		{name: "planned not suppressed", mutate: func(candidate *integration.ProcessResult) {
			candidate.Deliveries[0].Status = integration.DeliveryStatusPlanned
		}},
		{name: "attempt id", mutate: func(candidate *integration.ProcessResult) { candidate.Deliveries[0].AttemptID = "attempt-preview" }},
		{name: "attempt count", mutate: func(candidate *integration.ProcessResult) { candidate.Deliveries[0].AttemptCount = 1 }},
		{name: "attempt correlation", mutate: func(candidate *integration.ProcessResult) {
			candidate.Correlations.DeliveryAttemptIDs = []string{"attempt-preview"}
		}},
		{name: "receipt correlation", mutate: func(candidate *integration.ProcessResult) { candidate.Correlations.ReceiptID = "receipt-preview" }},
		{name: "duplicate route lineage", mutate: func(candidate *integration.ProcessResult) {
			candidate.Routes = append(candidate.Routes, candidate.Routes[0])
		}},
		{name: "duplicate delivery lineage", mutate: func(candidate *integration.ProcessResult) {
			candidate.Deliveries = append(candidate.Deliveries, candidate.Deliveries[0])
		}},
		{name: "unknown route diagnostic", mutate: func(candidate *integration.ProcessResult) {
			candidate.Routes[0].DiagnosticCodes = []string{"UNKNOWN_ROUTE_CODE"}
		}},
		{name: "unknown delivery diagnostic", mutate: func(candidate *integration.ProcessResult) {
			candidate.Deliveries[0].DiagnosticCodes = []string{"UNKNOWN_DELIVERY_CODE"}
		}},
		{name: "duplicate route diagnostic binding", mutate: func(candidate *integration.ProcessResult) {
			candidate.Routes[0].DiagnosticCodes = []string{"MISSING_PV1", "MISSING_PV1"}
		}},
		{name: "duplicate delivery diagnostic binding", mutate: func(candidate *integration.ProcessResult) {
			candidate.Deliveries[0].DiagnosticCodes = []string{"MISSING_PV1", "MISSING_PV1"}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := cloneProcessResult(result)
			tt.mutate(&candidate)
			if err := candidate.ValidatePreviewFor(request, revision); err == nil {
				t.Fatalf("strict preview accepted %s", tt.name)
			}
		})
	}
}

func TestValidatePreviewForAllowsRepeatedDiagnosticCodeAtDistinctPaths(t *testing.T) {
	revision, result, request := validStrictPreviewResult(t)
	repeated, err := integration.NewDiagnostic(integration.DiagnosticInput{
		TenantID:       revision.TenantID,
		Severity:       integration.DiagnosticSeverityWarning,
		Stage:          "semantic",
		Code:           "MISSING_PV1",
		Path:           "event.encounter.location",
		Source:         "hl7v2.parser",
		Classification: integration.DataClassificationPHI,
	})
	if err != nil {
		t.Fatalf("construct repeated diagnostic: %v", err)
	}
	result.Diagnostics[0], err = integration.NewDiagnostic(integration.DiagnosticInput{
		TenantID:       revision.TenantID,
		Severity:       integration.DiagnosticSeverityWarning,
		Stage:          "semantic",
		Code:           "MISSING_PV1",
		Path:           "event.encounter",
		Source:         "hl7v2.parser",
		Classification: integration.DataClassificationPHI,
	})
	if err != nil {
		t.Fatalf("construct first diagnostic: %v", err)
	}
	result.Diagnostics = append(result.Diagnostics, repeated)
	if err := result.ValidatePreviewFor(request, revision); err != nil {
		t.Fatalf("repeated code at distinct diagnostic paths should remain valid: %v", err)
	}
}

func validStrictPreviewResult(t *testing.T) (integration.IntegrationDefinitionRevision, integration.ProcessResult, integration.ProcessRequest) {
	t.Helper()
	revision, production := validProductionResult(t)
	request := processRequestForResult(t, revision, production)
	request.Mode = integration.ExecutionModePreview
	request.IdempotencyKey = ""

	result := cloneProcessResult(production)
	result.Mode = integration.ExecutionModePreview
	result.Receipt = nil
	result.ArtifactRevisions = executionArtifactRevisions(revision)
	result.Routes[0].EventID = result.Events[0].ID
	result.Routes[0].DiagnosticCodes = []string{"MISSING_PV1"}
	result.Deliveries[0].EventID = result.Events[0].ID
	result.Deliveries[0].Status = integration.DeliveryStatusSuppressed
	result.Deliveries[0].AttemptID = ""
	result.Deliveries[0].AttemptCount = 0
	result.Deliveries[0].DiagnosticCodes = []string{"MISSING_PV1"}
	result.Correlations.ReceiptID = ""
	result.Correlations.DeliveryAttemptIDs = nil
	return revision, result, request
}

func executionArtifactRevisions(revision integration.IntegrationDefinitionRevision) *integration.ExecutionArtifactRevisions {
	return &integration.ExecutionArtifactRevisions{
		Source:   revision.Source.ArtifactRevisionRef,
		Profile:  revision.Profile,
		Workflow: revision.Workflow,
	}
}

func TestValidatePreviewForRejectsReceiptEvenWhenOtherwiseWellFormed(t *testing.T) {
	revision, result, request := validStrictPreviewResult(t)
	result.Receipt = &integration.Receipt{
		ID:                  "receipt-preview",
		TenantID:            revision.TenantID,
		IntegrationRevision: revision.Reference(),
		Status:              integration.ReceiptStatusAccepted,
		IdempotencyKey:      "preview",
		RecordedAt:          time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC),
		CorrelationID:       result.Correlations.CorrelationID,
		RawRetentionMode:    integration.RawRetentionModeEphemeral,
		Principal:           result.Security.Principal,
	}
	result.Correlations.ReceiptID = result.Receipt.ID
	if err := result.ValidatePreviewFor(request, revision); err == nil {
		t.Fatal("strict preview accepted a receipt")
	}
}
