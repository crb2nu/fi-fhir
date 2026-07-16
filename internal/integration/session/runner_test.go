package session

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

func TestRunnerHL7v2LifecycleDiagnosticsLineageAndStream(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	hub := NewHub()
	runner := NewRunner(store, hub)

	sess, err := store.CreateSession(ctx, CreateSessionRequest{Name: "ADT run"})
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	raw, err := os.ReadFile("../../../testdata/integration-session/adt_a01_missing_pv1.hl7")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	sample, err := store.AddSample(ctx, sess.ID, AddSampleRequest{
		Name:      "missing PV1",
		Format:    events.FormatHL7v2,
		Source:    "adt-feed",
		Raw:       string(raw),
		PHIPolicy: PHIPolicyRetain,
	})
	if err != nil {
		t.Fatalf("AddSample error: %v", err)
	}

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := hub.Subscribe(streamCtx, sess.ID)

	run, err := runner.RunHL7v2(ctx, RunRequest{SessionID: sess.ID, SampleID: sample.ID})
	if err != nil {
		t.Fatalf("RunHL7v2 error: %v", err)
	}
	if run.Status != RunStatusSucceeded {
		t.Fatalf("run status = %s, want succeeded", run.Status)
	}
	if len(run.Events) != 1 || run.Events[0].Type != string(events.EventPatientAdmit) {
		t.Fatalf("parsed events = %+v", run.Events)
	}
	if len(run.Stages) != 4 {
		t.Fatalf("stage count = %d, want 4: %+v", len(run.Stages), run.Stages)
	}
	for _, stage := range run.Stages {
		if stage.Status != StageStatusSucceeded {
			t.Fatalf("stage %s status = %s", stage.Name, stage.Status)
		}
	}
	if len(run.Diagnostics) == 0 {
		t.Fatal("expected diagnostics from missing PV1 warning")
	}
	if run.Diagnostics[0].Code != "MISSING_PV1" || run.Diagnostics[0].Severity != "warning" {
		t.Fatalf("diagnostic = %+v", run.Diagnostics[0])
	}
	if !hasLineage(run.Lineage, "MSH.9", "event.type") {
		t.Fatalf("missing MSH.9 event.type lineage: %+v", run.Lineage)
	}
	if !hasLineage(run.Lineage, "PID.5", "event.patient.name") {
		t.Fatalf("missing PID.5 patient lineage: %+v", run.Lineage)
	}

	seen := map[StreamEventType]bool{}
	wantEvents := []StreamEventType{
		StreamEventRunStarted,
		StreamEventStageStarted,
		StreamEventDiagnostic,
		StreamEventRunCompleted,
	}
	deadline := time.After(2 * time.Second)
	for !hasAllStreamEvents(seen, wantEvents) {
		select {
		case evt := <-stream:
			seen[evt.Type] = true
		case <-deadline:
			t.Fatalf("timed out waiting for stream events, saw %+v", seen)
		}
	}
}

func TestNormalizeDiagnosticsDefaults(t *testing.T) {
	diagnostics := NormalizeDiagnostics([]events.ParseWarning{{
		Code:    "ODD_FIELD",
		Message: "field looked odd",
		Path:    "PID.3",
	}})
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1", len(diagnostics))
	}
	got := diagnostics[0]
	if got.Phase != "semantic" || got.Severity != "warning" || got.Source != "hl7v2_parser" {
		t.Fatalf("diagnostic defaults = %+v", got)
	}
}

func TestBundleExportIncludesSessionArtifactsAndRuns(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	runner := NewRunner(store, NewHub())

	sess, err := store.CreateSession(ctx, CreateSessionRequest{Name: "export"})
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	raw, err := os.ReadFile("../../../testdata/integration-session/adt_a01_missing_pv1.hl7")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	sample, err := store.AddSample(ctx, sess.ID, AddSampleRequest{
		Name:   "ADT",
		Format: events.FormatHL7v2,
		Raw:    string(raw),
	})
	if err != nil {
		t.Fatalf("AddSample error: %v", err)
	}
	if _, err := store.SaveArtifactDraft(ctx, sess.ID, SaveArtifactDraftRequest{
		Kind:    ArtifactKindNotes,
		Name:    "notes",
		Content: json.RawMessage(`{"note":"review missing PV1 tolerance"}`),
	}); err != nil {
		t.Fatalf("SaveArtifactDraft error: %v", err)
	}
	if _, err := runner.RunHL7v2(ctx, RunRequest{SessionID: sess.ID, SampleID: sample.ID}); err != nil {
		t.Fatalf("RunHL7v2 error: %v", err)
	}

	bundle, err := store.ExportBundle(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ExportBundle error: %v", err)
	}
	if bundle.Session.ID != sess.ID || len(bundle.Samples) != 1 || len(bundle.Drafts) != 1 || len(bundle.Runs) != 1 {
		t.Fatalf("unexpected bundle: %+v", bundle)
	}
	if _, err := json.Marshal(bundle); err != nil {
		t.Fatalf("bundle should marshal to JSON: %v", err)
	}
}

func TestRunnerPinsExactProfileRevision(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	runner := NewRunner(store, NewHub())
	sess, err := store.CreateSession(ctx, CreateSessionRequest{Name: "exact profile"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	raw, err := os.ReadFile("../../../testdata/integration-session/adt_a01_missing_pv1.hl7")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	sample, err := store.AddSample(ctx, sess.ID, AddSampleRequest{
		Name: "redacted default", Format: events.FormatHL7v2, Raw: string(raw),
	})
	if err != nil {
		t.Fatalf("AddSample: %v", err)
	}
	if sample.PHIPolicy != PHIPolicyRedact {
		t.Fatalf("default PHI policy = %q, want redact", sample.PHIPolicy)
	}
	profileJSON := json.RawMessage(`{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","tolerance":{"missing_segments":["PV1"],"nte_anywhere":false,"extra_components":false,"unknown_segments":false,"non_standard_delimiters":false},"event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]}}`)
	revision, err := store.SaveArtifactDraft(ctx, sess.ID, SaveArtifactDraftRequest{
		Kind: ArtifactKindMappingProfile, Name: "tolerant profile", Content: profileJSON,
	})
	if err != nil {
		t.Fatalf("SaveArtifactDraft: %v", err)
	}
	run, err := runner.RunHL7v2(ctx, RunRequest{
		SessionID: sess.ID, SampleID: sample.ID, ProfileRevisionID: revision.RevisionID,
	})
	if err != nil {
		t.Fatalf("RunHL7v2: %v", err)
	}
	if run.ProfileID != revision.ID || run.ProfileRevisionID != revision.RevisionID || run.ProfileRevisionDigest != revision.Digest {
		t.Fatalf("run profile reference = %#v, revision = %#v", run, revision)
	}
	if !hasDiagnosticCode(run.Diagnostics, "MISSING_PV1") {
		t.Fatalf("diagnostics = %#v, want MISSING_PV1", run.Diagnostics)
	}
	if _, err := store.UpdateRun(ctx, *run); !errors.Is(err, ErrImmutable) {
		t.Fatalf("terminal run update error = %v, want ErrImmutable", err)
	}

	decision, err := store.AcceptDecision(ctx, AcceptDecisionRequest{
		SessionID: sess.ID, RunID: run.ID, DiagnosticID: run.Diagnostics[0].ID,
		AcceptedBy: "engineer-1",
	})
	if err != nil || decision.ID == "" {
		t.Fatalf("AcceptDecision = %#v, %v", decision, err)
	}
	bundle, err := store.ExportBundle(ctx, sess.ID)
	if err != nil || bundle.ID == "" || len(bundle.Decisions) != 1 {
		t.Fatalf("ExportBundle = %#v, %v", bundle, err)
	}
	reopened, err := store.GetExport(ctx, sess.ID, bundle.ID)
	if err != nil || reopened.ID != bundle.ID {
		t.Fatalf("GetExport = %#v, %v", reopened, err)
	}
	exports, err := store.ListExports(ctx, sess.ID)
	if err != nil || len(exports) != 1 || exports[0].ID != bundle.ID {
		t.Fatalf("ListExports = %#v, %v", exports, err)
	}
}

func hasDiagnosticCode(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func hasLineage(links []LineageLink, source, target string) bool {
	for _, link := range links {
		if link.SourcePath == source && link.TargetPath == target {
			return true
		}
	}
	return false
}

func hasAllStreamEvents(seen map[StreamEventType]bool, want []StreamEventType) bool {
	for _, eventType := range want {
		if !seen[eventType] {
			return false
		}
	}
	return true
}
