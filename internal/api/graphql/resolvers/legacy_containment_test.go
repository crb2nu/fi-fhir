package resolvers

import (
	"context"
	"errors"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
)

func init() {
	enableLegacyUnsafeExecutionForTests = func(resolver *Resolver) {
		resolver.legacyUnsafeExecution = true
	}
}

func TestLegacyRawAndExecutionResolversFailClosedByDefault(t *testing.T) {
	resolver := NewResolver()
	mutation := &mutationResolver{resolver}
	query := &queryResolver{resolver}
	subscription := &subscriptionResolver{resolver}
	ctx := context.Background()

	session, err := mutation.CreateIntegrationSession(ctx, model.CreateIntegrationSessionInput{Name: "containment"})
	if err != nil {
		t.Fatalf("CreateIntegrationSession: %v", err)
	}
	resolver.legacyUnsafeExecution = false
	retain := true
	includeRaw := true
	legacyCalls := []struct {
		name string
		call func() error
	}{
		{name: "submit message", call: func() error {
			_, err := mutation.SubmitMessage(ctx, model.SubmitMessageInput{Format: model.SourceFormatHL7v2, Source: "source", Data: "RAW-PHI-SENTINEL"})
			return err
		}},
		{name: "submit event", call: func() error {
			_, err := mutation.SubmitEvent(ctx, model.SubmitEventInput{Type: model.EventTypePatientAdmit, Source: "source", Data: map[string]any{"raw": "RAW-PHI-SENTINEL"}})
			return err
		}},
		{name: "submit batch", call: func() error {
			_, err := mutation.SubmitBatch(ctx, model.SubmitBatchInput{Messages: []model.BatchMessageItem{{Format: model.SourceFormatHL7v2, Source: "source", Data: "RAW-PHI-SENTINEL"}}})
			return err
		}},
		{name: "trigger workflow", call: func() error {
			_, err := mutation.TriggerWorkflow(ctx, "legacy", map[string]any{"raw": "RAW-PHI-SENTINEL"}, nil, nil)
			return err
		}},
		{name: "parse preview", call: func() error {
			_, err := query.ParsePreview(ctx, model.SourceFormatHL7v2, "RAW-PHI-SENTINEL", nil)
			return err
		}},
		{name: "parse preview with profile", call: func() error {
			_, err := query.ParsePreviewWithProfile(ctx, model.SourceFormatHL7v2, "RAW-PHI-SENTINEL", nil, nil)
			return err
		}},
		{name: "add retained sample", call: func() error {
			_, err := mutation.AddSessionSample(ctx, model.AddSessionSampleInput{SessionID: session.ID, Name: "raw", Format: model.SourceFormatHL7v2, Data: "RAW-PHI-SENTINEL", RetainRawPayload: &retain})
			return err
		}},
		{name: "run session preview", call: func() error {
			data := "RAW-PHI-SENTINEL"
			_, err := mutation.RunSessionPreview(ctx, model.RunSessionPreviewInput{SessionID: session.ID, Data: &data})
			return err
		}},
		{name: "raw capable export", call: func() error {
			_, err := mutation.ExportIntegrationBundle(ctx, model.ExportIntegrationBundleInput{SessionID: session.ID, IncludeRawPayload: &includeRaw})
			return err
		}},
		{name: "live parse stream", call: func() error {
			_, err := subscription.LiveParseStream(ctx, model.LiveParseInput{Format: model.SourceFormatHL7v2, Message: "RAW-PHI-SENTINEL"})
			return err
		}},
	}

	for _, tt := range legacyCalls {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, ErrLegacyExecutionUnavailable) {
				t.Fatalf("error = %v, want ErrLegacyExecutionUnavailable", err)
			}
		})
	}

	events, err := resolver.Store.QueryEvents(ctx, nil, 10, nil, nil)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if events.TotalCount != 0 {
		t.Fatalf("legacy containment persisted %d events", events.TotalCount)
	}
	samples, err := resolver.integrationSessions.listSamples(session.ID)
	if err != nil {
		t.Fatalf("listSamples: %v", err)
	}
	runs, err := resolver.integrationSessions.listRuns(session.ID)
	if err != nil {
		t.Fatalf("listRuns: %v", err)
	}
	if len(samples) != 0 || len(runs) != 0 {
		t.Fatalf("legacy containment persisted samples=%d runs=%d", len(samples), len(runs))
	}
}
