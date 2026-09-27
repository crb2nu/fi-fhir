package session

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// TestAddSample_CallerNamedIDIsIdempotent: a caller-named ID makes a retried
// write return the first sample unchanged instead of storing a second one, and
// an ID that names another session's sample, or is not a sample ID, is
// refused. The PostgreSQL store's ON CONFLICT path is proved over a real
// database by TestConnectionCapture_RacingFramesNeverExceedMaxMessages.
func TestAddSample_CallerNamedIDIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	first, err := store.CreateSession(ctx, CreateSessionRequest{Name: "first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateSession(ctx, CreateSessionRequest{Name: "second"})
	if err != nil {
		t.Fatal(err)
	}
	request := AddSampleRequest{
		ID: "sample_capture_c-1_1", Name: "capture c-1 #1", Format: events.FormatHL7v2, Source: "capture:c-1",
		Raw: syntheticCaptureMessage("|"), PHIPolicy: PHIPolicyRedact, Redaction: SampleRedactionCapture,
	}
	written, err := store.AddSample(ctx, first.ID, request)
	if err != nil || written.ID != request.ID {
		t.Fatalf("AddSample = %+v, %v; want the caller-named ID", written, err)
	}
	retry := request
	retry.Raw = "MSH|^~\\&|OTHER|FAC|FI-FHIR|FAC|20260926||ADT^A01|c-2|P|2.5.1\rPID|1||OTHER-MRN\r"
	again, err := store.AddSample(ctx, first.ID, retry)
	if err != nil {
		t.Fatalf("retried AddSample: %v", err)
	}
	if again.ID != written.ID || again.Raw != written.Raw || !again.CreatedAt.Equal(written.CreatedAt) {
		t.Fatalf("the retry returned %+v, want the first write %+v unchanged", again, written)
	}
	samples, err := store.ListSamples(ctx, first.ID)
	if err != nil || len(samples) != 1 {
		t.Fatalf("session holds %d samples, %v; want exactly one", len(samples), err)
	}

	if _, err := store.AddSample(ctx, second.ID, request); !errors.Is(err, ErrInvalid) {
		t.Fatalf("the ID of another session's sample = %v, want ErrInvalid", err)
	}
	for _, id := range []string{"capture-1", "sample_", "sample_a b", "sample_../x", "sample_" + strings.Repeat("a", maxSampleIDBytes)} {
		bad := request
		bad.ID = id
		if _, err := store.AddSample(ctx, first.ID, bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("sample ID %q = %v, want ErrInvalid", id, err)
		}
	}
	generated, err := store.AddSample(ctx, first.ID, AddSampleRequest{Name: "pasted", Format: events.FormatHL7v2, Raw: "MSH|^~\\&|X"})
	if err != nil || !validSampleID(generated.ID) {
		t.Fatalf("a generated ID %q is not a valid sample ID (%v)", generated.ID, err)
	}
}

// TestExportBundle_StripsCapturedText: an export snapshot carries no captured
// text whatever the caller's grant, exactly as it carries no retained raw,
// while a pasted sample's redacted text is exported as before.
func TestExportBundle_StripsCapturedText(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	workspace, err := store.CreateSession(ctx, CreateSessionRequest{Name: "export"})
	if err != nil {
		t.Fatal(err)
	}
	message := syntheticCaptureMessage("|")
	if _, err := store.AddSample(ctx, workspace.ID, AddSampleRequest{
		Name: "captured", Format: events.FormatHL7v2, Source: "capture:c-1", Raw: message,
		PHIPolicy: PHIPolicyRedact, Redaction: SampleRedactionCapture,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddSample(ctx, workspace.ID, AddSampleRequest{Name: "pasted", Format: events.FormatHL7v2, Raw: message}); err != nil {
		t.Fatal(err)
	}
	bundle, err := store.ExportBundle(ctx, ExportRequest{
		SessionID: workspace.ID, Reason: "hand the profile over", IncludeRawPayload: true,
		Principal: integration.Principal{
			ID: "engineer", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{PHIExportRole},
		},
	})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	if len(bundle.Samples) != 2 {
		t.Fatalf("bundle carries %d samples, want 2", len(bundle.Samples))
	}
	for _, sample := range bundle.Samples {
		switch sample.Name {
		case "captured":
			if sample.Raw != "" {
				t.Fatalf("the export snapshot carries captured text: %q", sample.Raw)
			}
		case "pasted":
			if sample.Raw != redactHL7v2(message) {
				t.Fatal("the export no longer carries a pasted sample's redacted text")
			}
		}
	}
	stored, err := store.GetSample(ctx, workspace.ID, bundle.Samples[0].ID)
	if err != nil || stored.Raw == "" {
		t.Fatalf("stripping the export emptied the stored sample: %+v, %v", stored, err)
	}
}
