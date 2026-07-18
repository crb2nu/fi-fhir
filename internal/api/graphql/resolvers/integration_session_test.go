package resolvers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	enginesession "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const integrationSessionHL7Sample = `MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240115120000||ADT^A01|MSG00001|P|2.5
EVN|A01|20240115120000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN||19800315|M
PV1|1|I|ICU^101`

func TestIntegrationSession_CreateAndAddSample(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}
	mutationResolver := &mutationResolver{resolver}

	session, err := mutationResolver.CreateIntegrationSession(context.Background(), model.CreateIntegrationSessionInput{
		Name:        "ADT mapping",
		Description: strPtr("Profile tuning workspace"),
	})
	if err != nil {
		t.Fatalf("CreateIntegrationSession failed: %v", err)
	}
	if session.ID == "" {
		t.Fatal("expected session ID")
	}

	sample, err := mutationResolver.AddSessionSample(context.Background(), model.AddSessionSampleInput{
		SessionID:        session.ID,
		Name:             "ADT A01",
		Format:           model.SourceFormatHL7v2,
		Data:             integrationSessionHL7Sample,
		Source:           strPtr("test-feed"),
		RetainRawPayload: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("AddSessionSample failed: %v", err)
	}
	if sample.PayloadChecksum == "" {
		t.Fatal("expected payload checksum")
	}
	if sample.RawPayload == nil {
		t.Fatal("expected retained raw payload")
	}

	samples, err := queryResolver.SessionSamples(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("SessionSamples failed: %v", err)
	}
	if len(samples) != 1 || samples[0].ID != sample.ID {
		t.Fatalf("expected stored sample %q, got %#v", sample.ID, samples)
	}
}

func TestIntegrationSession_RunPreview(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	session := createTestIntegrationSession(t, mutationResolver)
	sample := addTestIntegrationSample(t, mutationResolver, session.ID)

	run, err := mutationResolver.RunSessionPreview(context.Background(), model.RunSessionPreviewInput{
		SessionID: session.ID,
		SampleID:  &sample.ID,
	})
	if err != nil {
		t.Fatalf("RunSessionPreview failed: %v", err)
	}
	if run.Status != "completed" {
		t.Fatalf("expected completed run, got %q with diagnostics %#v", run.Status, run.Diagnostics)
	}
	if len(run.Events) != 1 {
		t.Fatalf("expected one parsed event, got %d", len(run.Events))
	}
	if !hasRunStage(run.Stages, "parse_hl7v2") {
		t.Fatalf("expected parse stage with duration, got %#v", run.Stages)
	}
}

func TestIntegrationSession_WorkflowSimulationUsesDurableRevisionAndRun(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}
	mutationResolver := &mutationResolver{resolver}
	sessionRecord := createTestIntegrationSession(t, mutationResolver)
	sample := addTestIntegrationSample(t, mutationResolver, sessionRecord.ID)
	parsedRun, err := mutationResolver.RunSessionPreview(context.Background(), model.RunSessionPreviewInput{
		SessionID: sessionRecord.ID, SampleID: &sample.ID,
	})
	if err != nil || parsedRun.Status != "completed" {
		t.Fatalf("RunSessionPreview() = %#v, %v", parsedRun, err)
	}
	baselineDraft, err := mutationResolver.UpdateSessionWorkflowDraft(context.Background(), model.UpdateSessionArtifactInput{
		SessionID: sessionRecord.ID, Name: strPtr("routing"), Content: `name: baseline
version: "1"
routes:
  - name: labs
    filter: {event_type: lab_result}
    actions:
      - id: log-lab
        type: log
`,
	})
	if err != nil {
		t.Fatalf("UpdateSessionWorkflowDraft(baseline) error = %v", err)
	}
	baseline, err := mutationResolver.SimulateSessionWorkflow(context.Background(), model.SimulateSessionWorkflowInput{
		SessionID: sessionRecord.ID, WorkflowRevisionID: baselineDraft.RevisionID, SourceRunIds: []string{parsedRun.ID},
	})
	if err != nil {
		t.Fatalf("SimulateSessionWorkflow(baseline) error = %v", err)
	}
	candidateDraft, err := mutationResolver.UpdateSessionWorkflowDraft(context.Background(), model.UpdateSessionArtifactInput{
		SessionID: sessionRecord.ID, Name: strPtr("routing"), Content: `name: candidate
version: "2"
routes:
  - name: admits
    filter: {event_type: patient_admit}
    transform:
      - redact: {fields: [patient.identifiers]}
    actions:
      - id: notify
        type: exec
        destination: notification-sandbox
        command: ACTION-CONFIG-SENTINEL
`,
	})
	if err != nil {
		t.Fatalf("UpdateSessionWorkflowDraft(candidate) error = %v", err)
	}
	if candidateDraft.ID != baselineDraft.ID || candidateDraft.Version != baselineDraft.Version+1 {
		t.Fatalf("workflow draft lineage = baseline %#v, candidate %#v", baselineDraft, candidateDraft)
	}
	candidate, err := mutationResolver.SimulateSessionWorkflow(context.Background(), model.SimulateSessionWorkflowInput{
		SessionID: sessionRecord.ID, WorkflowRevisionID: candidateDraft.RevisionID,
		SourceRunIds: []string{parsedRun.ID}, BaselineSimulationID: &baseline.ID,
	})
	if err != nil {
		t.Fatalf("SimulateSessionWorkflow(candidate) error = %v", err)
	}
	if candidate.WorkflowRevisionID != candidateDraft.RevisionID || candidate.WorkflowRevisionDigest != candidateDraft.Digest {
		t.Fatalf("candidate provenance = %#v, draft = %#v", candidate, candidateDraft)
	}
	if candidate.Delta == nil || len(candidate.Delta.AddedMatchedRoutes) != 1 || len(candidate.Delta.AddedTransforms) != 1 || len(candidate.Delta.AddedActions) != 1 {
		t.Fatalf("candidate delta = %#v", candidate.Delta)
	}
	if len(candidate.Events) != 1 || len(candidate.Events[0].Routes) != 1 || !candidate.Events[0].Routes[0].Matched {
		t.Fatalf("candidate traces = %#v", candidate.Events)
	}
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatalf("json.Marshal(candidate) error = %v", err)
	}
	if strings.Contains(string(encoded), "ACTION-CONFIG-SENTINEL") || strings.Contains(string(encoded), "DOE") {
		t.Fatalf("simulation exposed config or PHI: %s", encoded)
	}
	listed, err := queryResolver.SessionWorkflowSimulations(context.Background(), sessionRecord.ID)
	if err != nil || len(listed) != 2 || listed[1].Delta != nil {
		t.Fatalf("SessionWorkflowSimulations() = %#v, %v", listed, err)
	}
	missingBaseline := "simulation_missing"
	if _, err := mutationResolver.SimulateSessionWorkflow(context.Background(), model.SimulateSessionWorkflowInput{
		SessionID: sessionRecord.ID, WorkflowRevisionID: candidateDraft.RevisionID,
		SourceRunIds: []string{parsedRun.ID}, BaselineSimulationID: &missingBaseline,
	}); err == nil {
		t.Fatal("SimulateSessionWorkflow() accepted a missing baseline")
	}
	listed, err = queryResolver.SessionWorkflowSimulations(context.Background(), sessionRecord.ID)
	if err != nil || len(listed) != 2 {
		t.Fatalf("failed baseline persisted a simulation: %#v, %v", listed, err)
	}
	bundle, err := mutationResolver.ExportIntegrationBundle(context.Background(), model.ExportIntegrationBundleInput{SessionID: sessionRecord.ID})
	if err != nil || len(bundle.WorkflowSimulations) != 2 {
		t.Fatalf("ExportIntegrationBundle() simulations = %#v, %v", bundle, err)
	}
}

func TestIntegrationSession_DiagnosticsAcceptFixAndBundle(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}
	mutationResolver := &mutationResolver{resolver}

	session := createTestIntegrationSession(t, mutationResolver)
	format := model.SourceFormatCSV
	run, err := mutationResolver.RunSessionPreview(context.Background(), model.RunSessionPreviewInput{
		SessionID: session.ID,
		Data:      strPtr("not,hl7"),
		Format:    &format,
	})
	if err != nil {
		t.Fatalf("RunSessionPreview failed: %v", err)
	}
	if run.Status != "failed" || len(run.Diagnostics) != 1 {
		t.Fatalf("expected failed run with diagnostic, got status=%q diagnostics=%d", run.Status, len(run.Diagnostics))
	}

	diagnostics, err := queryResolver.SessionDiagnostics(context.Background(), session.ID, &run.ID)
	if err != nil {
		t.Fatalf("SessionDiagnostics failed: %v", err)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("expected one diagnostic, got %d", len(diagnostics))
	}

	accepted, err := mutationResolver.AcceptDiagnosticFix(context.Background(), model.AcceptDiagnosticFixInput{
		SessionID:    session.ID,
		DiagnosticID: diagnostics[0].ID,
		AcceptedBy:   strPtr("tester"),
	})
	if err != nil {
		t.Fatalf("AcceptDiagnosticFix failed: %v", err)
	}
	if !accepted.Accepted || accepted.AcceptedAt == nil {
		t.Fatalf("expected accepted diagnostic, got %#v", accepted)
	}

	bundle, err := mutationResolver.ExportIntegrationBundle(context.Background(), model.ExportIntegrationBundleInput{
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("ExportIntegrationBundle failed: %v", err)
	}
	if bundle.SessionID != session.ID {
		t.Fatalf("expected bundle for session %q, got %q", session.ID, bundle.SessionID)
	}
	if len(bundle.Runs) != 1 || len(bundle.Diagnostics) != 1 {
		t.Fatalf("expected run and diagnostic in bundle, got runs=%d diagnostics=%d", len(bundle.Runs), len(bundle.Diagnostics))
	}
}

func TestIntegrationSession_SubscriptionFanout(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}
	subscriptionResolver := &subscriptionResolver{resolver}

	session := createTestIntegrationSession(t, mutationResolver)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := subscriptionResolver.IntegrationSessionEvents(ctx, session.ID)
	if err != nil {
		t.Fatalf("IntegrationSessionEvents failed: %v", err)
	}

	sample, err := mutationResolver.AddSessionSample(context.Background(), model.AddSessionSampleInput{
		SessionID:        session.ID,
		Name:             "ADT A01",
		Format:           model.SourceFormatHL7v2,
		Data:             integrationSessionHL7Sample,
		RetainRawPayload: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("AddSessionSample failed: %v", err)
	}

	select {
	case event := <-events:
		if event.Type != "sample.added" {
			t.Fatalf("expected sample.added event, got %q", event.Type)
		}
		if event.Session == nil || event.Session.ID != session.ID {
			t.Fatalf("expected event session payload, got %#v", event.Session)
		}
		if len(event.Session.Samples) != 0 {
			t.Fatalf("stream envelope exposed retained samples: %#v", event.Session.Samples)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for integration session event")
	}

	profile, err := mutationResolver.UpdateSessionProfileDraft(context.Background(), model.UpdateSessionArtifactInput{
		SessionID: session.ID,
		Content:   `{"hl7v2":{"default_version":"2.5","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]}}`,
	})
	if err != nil {
		t.Fatalf("UpdateSessionProfileDraft failed: %v", err)
	}
	// The draft event is outside the run progression asserted below.
	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for draft event")
	}

	run, err := mutationResolver.RunSessionPreview(context.Background(), model.RunSessionPreviewInput{
		SessionID: session.ID,
		SampleID:  &sample.ID,
	})
	if err != nil {
		t.Fatalf("RunSessionPreview failed: %v", err)
	}
	wantTypes := []string{
		"run_started",
		"stage_started", "stage_completed",
		"stage_started", "stage_completed",
		"stage_started", "stage_completed",
		"stage_started", "stage_completed",
		"run_completed",
	}
	gotTypes := make([]string, 0, len(wantTypes))
	var terminal *model.SessionRun
	deadline := time.After(2 * time.Second)
	for len(gotTypes) < len(wantTypes) {
		select {
		case event := <-events:
			gotTypes = append(gotTypes, event.Type)
			if event.Run != nil {
				terminal = event.Run
			}
		case <-deadline:
			t.Fatalf("timed out waiting for run progression: %#v", gotTypes)
		}
	}
	for index := range wantTypes {
		if gotTypes[index] != wantTypes[index] {
			t.Fatalf("stream event %d = %q, want %q; all=%#v", index, gotTypes[index], wantTypes[index], gotTypes)
		}
	}
	if terminal == nil || terminal.ID != run.ID || terminal.Status != run.Status {
		t.Fatalf("terminal stream snapshot = %#v, mutation run = %#v", terminal, run)
	}
	if terminal.ProfileRevisionID == nil || *terminal.ProfileRevisionID != profile.RevisionID ||
		terminal.ProfileRevisionDigest == nil || *terminal.ProfileRevisionDigest != profile.Digest {
		t.Fatalf("terminal stream provenance = %#v, profile = %#v", terminal, profile)
	}
	if !hasGraphQLLineage(terminal.Lineage, "PID-5", "event.patient.name") {
		t.Fatalf("terminal stream lineage = %#v", terminal.Lineage)
	}
	for _, link := range terminal.Lineage {
		if link.Description != nil {
			t.Fatalf("raw lineage previews must not cross the GraphQL boundary: %+v", link)
		}
	}
}

func TestIntegrationSession_DurableStoreEnablesContainedRoutes(t *testing.T) {
	store := enginesession.NewMemoryStore()
	resolver := NewResolver(WithIntegrationSessionStore(store))
	resolver.legacyUnsafeExecution = false
	mutation := &mutationResolver{resolver}
	query := &queryResolver{resolver}

	session, err := mutation.CreateIntegrationSession(context.Background(), model.CreateIntegrationSessionInput{Name: "durable workspace"})
	if err != nil {
		t.Fatalf("CreateIntegrationSession: %v", err)
	}
	sample, err := mutation.AddSessionSample(context.Background(), model.AddSessionSampleInput{
		SessionID: session.ID, Name: "ADT", Format: model.SourceFormatHL7v2,
		Data: integrationSessionHL7Sample,
	})
	if err != nil {
		t.Fatalf("AddSessionSample: %v", err)
	}
	if sample.RawPayload != nil {
		t.Fatal("default durable sample exposed raw payload")
	}
	profile, err := mutation.UpdateSessionProfileDraft(context.Background(), model.UpdateSessionArtifactInput{
		SessionID: session.ID,
		Name:      strPtr("ADT profile"),
		Content:   `{"hl7v2":{"default_version":"2.5","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]}}`,
	})
	if err != nil {
		t.Fatalf("UpdateSessionProfileDraft: %v", err)
	}
	if profile.RevisionID == "" || profile.Version != 1 || profile.Digest == "" {
		t.Fatalf("profile provenance = %#v", profile)
	}
	run, err := mutation.RunSessionPreview(context.Background(), model.RunSessionPreviewInput{
		SessionID: session.ID,
		SampleID:  &sample.ID,
	})
	if err != nil {
		t.Fatalf("RunSessionPreview: %v", err)
	}
	if run.ProfileRevisionID == nil || *run.ProfileRevisionID != profile.RevisionID ||
		run.ProfileRevisionDigest == nil || *run.ProfileRevisionDigest != profile.Digest {
		t.Fatalf("run profile provenance = %#v, want revision %q digest %q", run, profile.RevisionID, profile.Digest)
	}
	listed, err := query.IntegrationSessions(context.Background(), nil)
	if err != nil || len(listed) != 1 || listed[0].ID != session.ID {
		t.Fatalf("IntegrationSessions = %#v, %v", listed, err)
	}

	format := model.SourceFormatCSV
	failed, err := mutation.RunSessionPreview(context.Background(), model.RunSessionPreviewInput{
		SessionID: session.ID, Data: strPtr("not,hl7"), Format: &format,
	})
	if err != nil || len(failed.Diagnostics) != 1 {
		t.Fatalf("failed preview = %#v, %v", failed, err)
	}
	acceptedBy := "client-spoofed"
	operatorCtx := requestsecurity.WithSecurityContext(context.Background(), integration.SecurityContext{
		TenantID: "tenant-a",
		Principal: integration.Principal{
			ID: "operator-1", Kind: integration.PrincipalKindHuman, AuthMethod: "bearer",
			Roles: []string{"graphql:operator"},
		},
	})
	if _, err := mutation.AcceptDiagnosticFix(operatorCtx, model.AcceptDiagnosticFixInput{
		SessionID: session.ID, DiagnosticID: failed.Diagnostics[0].ID, AcceptedBy: &acceptedBy,
	}); err != nil {
		t.Fatalf("AcceptDiagnosticFix: %v", err)
	}
	decisions, err := store.ListDecisions(context.Background(), session.ID)
	if err != nil || len(decisions) != 1 || decisions[0].AcceptedBy != "operator-1" {
		t.Fatalf("authenticated decisions = %#v, %v", decisions, err)
	}
}

func createTestIntegrationSession(t *testing.T, mutationResolver *mutationResolver) *model.IntegrationSession {
	t.Helper()
	session, err := mutationResolver.CreateIntegrationSession(context.Background(), model.CreateIntegrationSessionInput{Name: "Test session"})
	if err != nil {
		t.Fatalf("CreateIntegrationSession failed: %v", err)
	}
	return session
}

func addTestIntegrationSample(t *testing.T, mutationResolver *mutationResolver, sessionID string) *model.SessionSample {
	t.Helper()
	sample, err := mutationResolver.AddSessionSample(context.Background(), model.AddSessionSampleInput{
		SessionID:        sessionID,
		Name:             "ADT A01",
		Format:           model.SourceFormatHL7v2,
		Data:             integrationSessionHL7Sample,
		Source:           strPtr("test-feed"),
		RetainRawPayload: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("AddSessionSample failed: %v", err)
	}
	return sample
}

func hasRunStage(stages []model.RunStage, name string) bool {
	for _, stage := range stages {
		if stage.Name == name && stage.DurationMs >= 0 {
			return true
		}
	}
	return false
}

func hasGraphQLLineage(links []model.LineageLink, sourcePath, targetPath string) bool {
	for _, link := range links {
		if link.SourcePath == sourcePath && link.TargetPath != nil && *link.TargetPath == targetPath {
			return true
		}
	}
	return false
}
