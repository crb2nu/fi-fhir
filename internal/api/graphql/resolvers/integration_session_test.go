package resolvers

import (
	"context"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
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

	_, err = mutationResolver.AddSessionSample(context.Background(), model.AddSessionSampleInput{
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
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for integration session event")
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
