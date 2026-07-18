package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

func TestWorkflowSimulatorBindsExactRevisionAndProducesSafeDelta(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	sessionRecord, err := store.CreateSession(ctx, CreateSessionRequest{Name: "workflow simulation"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	sample, err := store.AddSample(ctx, sessionRecord.ID, AddSampleRequest{
		Name: "adt", Format: events.FormatHL7v2, Source: "adt-east",
		Raw: "MSH|^~\\&|APP|FAC|EHR|HOSPITAL|20240115120000||ADT^A01|MSG00001|P|2.5\r" +
			"PID|1||RAW-PHI-SENTINEL^^^HOSPITAL^MRN||SENTINEL^PATIENT||19800315|M\r" +
			"PV1|1|I|WARD^101^A||||1234^DOCTOR^ALICE",
	})
	if err != nil {
		t.Fatalf("AddSample() error = %v", err)
	}
	parsed, err := NewRunner(store, NewHub()).RunHL7v2(ctx, RunRequest{SessionID: sessionRecord.ID, SampleID: sample.ID})
	if err != nil || parsed.Status != RunStatusSucceeded || len(parsed.Events) != 1 {
		t.Fatalf("RunHL7v2() = %#v, %v", parsed, err)
	}

	baselineRevision, err := store.SaveArtifactDraft(ctx, sessionRecord.ID, SaveArtifactDraftRequest{
		Kind: ArtifactKindWorkflowDraft, Name: "routing", Content: json.RawMessage(`name: baseline
version: "1"
routes:
  - name: labs
    filter: {event_type: lab_result}
    actions:
      - id: log-lab
        type: log
`),
	})
	if err != nil {
		t.Fatalf("SaveArtifactDraft(baseline) error = %v", err)
	}
	simulator := NewWorkflowSimulator(store)
	baseline, err := simulator.Simulate(ctx, SimulateWorkflowRequest{
		SessionID: sessionRecord.ID, WorkflowRevisionID: baselineRevision.RevisionID, SourceRunIDs: []string{parsed.ID},
	})
	if err != nil {
		t.Fatalf("Simulate(baseline) error = %v", err)
	}

	trap := filepath.Join(t.TempDir(), "must-not-exist")
	candidateYAML := `name: candidate
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
        command: touch ` + trap + `
        secret: ACTION-CONFIG-SENTINEL
`
	candidateRevision, err := store.SaveArtifactDraft(ctx, sessionRecord.ID, SaveArtifactDraftRequest{
		ID: baselineRevision.ID, Kind: ArtifactKindWorkflowDraft, Name: "routing", Content: json.RawMessage(candidateYAML),
	})
	if err != nil {
		t.Fatalf("SaveArtifactDraft(candidate) error = %v", err)
	}
	candidate, err := simulator.Simulate(ctx, SimulateWorkflowRequest{
		SessionID: sessionRecord.ID, WorkflowRevisionID: candidateRevision.RevisionID, SourceRunIDs: []string{parsed.ID},
	})
	if err != nil {
		t.Fatalf("Simulate(candidate) error = %v", err)
	}
	if candidate.WorkflowRevisionID != candidateRevision.RevisionID || candidate.WorkflowRevisionDigest != candidateRevision.Digest || candidate.WorkflowArtifactID != candidateRevision.ID {
		t.Fatalf("candidate provenance = %#v, revision = %#v", candidate, candidateRevision)
	}
	if len(candidate.Events) != 1 || len(candidate.Events[0].Routes) != 1 || !candidate.Events[0].Routes[0].Matched {
		t.Fatalf("candidate trace = %#v", candidate.Events)
	}
	route := candidate.Events[0].Routes[0]
	if len(route.Transforms) != 1 || route.Transforms[0].Type != "redact" || route.Transforms[0].Status != "planned" {
		t.Fatalf("transform trace = %#v", route.Transforms)
	}
	if len(route.Actions) != 1 || route.Actions[0].ID != "notify" || route.Actions[0].DestinationArtifactID != "notification-sandbox" {
		t.Fatalf("action trace = %#v", route.Actions)
	}
	if _, err := os.Stat(trap); !os.IsNotExist(err) {
		t.Fatalf("simulation executed side effect at %s: %v", trap, err)
	}
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatalf("json.Marshal(candidate) error = %v", err)
	}
	for _, forbidden := range []string{"RAW-PHI-SENTINEL", "ACTION-CONFIG-SENTINEL", trap, "SENTINEL^PATIENT"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("simulation leaked %q: %s", forbidden, encoded)
		}
	}

	delta, err := CompareWorkflowSimulations(*baseline, *candidate)
	if err != nil {
		t.Fatalf("CompareWorkflowSimulations() error = %v", err)
	}
	if len(delta.AddedMatchedRoutes) != 1 || len(delta.AddedTransforms) != 1 || len(delta.AddedActions) != 1 {
		t.Fatalf("delta = %#v", delta)
	}
	if len(delta.AddedEvents) != 0 || len(delta.RemovedEvents) != 0 {
		t.Fatalf("event delta = %#v", delta)
	}

	stored, err := store.GetWorkflowSimulation(ctx, sessionRecord.ID, candidate.ID)
	if err != nil || stored.WorkflowRevisionDigest != candidateRevision.Digest {
		t.Fatalf("GetWorkflowSimulation() = %#v, %v", stored, err)
	}
	stored.Events[0].Routes[0].Name = "mutated"
	reloaded, err := store.GetWorkflowSimulation(ctx, sessionRecord.ID, candidate.ID)
	if err != nil || reloaded.Events[0].Routes[0].Name != "admits" {
		t.Fatalf("stored simulation was mutable: %#v, %v", reloaded, err)
	}
}

func TestCompareWorkflowSimulationsRejectsCrossSession(t *testing.T) {
	_, err := CompareWorkflowSimulations(
		WorkflowSimulation{ID: "one", SessionID: "session-one"},
		WorkflowSimulation{ID: "two", SessionID: "session-two"},
	)
	if err == nil {
		t.Fatal("CompareWorkflowSimulations() unexpectedly accepted cross-session records")
	}
}
