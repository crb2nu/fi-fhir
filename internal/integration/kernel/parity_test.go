package kernel_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/kernel"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// goldenStrictProfile is the adt-http golden profile the session runner
// stores as a mapping-profile draft; the kernel receives the built-in YAML
// form of the same profile (TestBuiltInProfilesMatchGoldenJSON).
const goldenStrictProfile = "../../../testdata/golden/integration/adt-http/strict-profile.json"

// volatileEventKeys are the event-meta members events.NewEventMeta fills
// from the clock and crypto/rand on every parse, on both paths. They are
// masked after both are checked present and well-formed; every other byte
// must match.
var volatileEventKeys = map[string]*regexp.Regexp{
	"id":          regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`),
	"timestamp":   regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`),
	"received_at": regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`),
}

// TestWASMKernelMatchesSessionRunner is the riskiest-assumption kill-test of
// .loom/40 (D-0): the browser kernel produces the same events and
// diagnostics as the IDE's integration-session runner for every demo sample
// the IDE ships, with the adt-http golden profile and without a profile.
func TestWASMKernelMatchesSessionRunner(t *testing.T) {
	samples, err := kernel.Samples()
	if err != nil {
		t.Fatalf("Samples: %v", err)
	}
	if len(samples) != 6 {
		t.Fatalf("built-in samples = %d, want the IDE's 6", len(samples))
	}
	goldenJSON, err := os.ReadFile(goldenStrictProfile)
	if err != nil {
		t.Fatalf("read golden profile: %v", err)
	}
	strictYAML := builtInProfileYAML(t, "adt-http-strict")

	profiles := []struct {
		name       string
		storedJSON []byte
		kernelYAML string
	}{
		{name: "adt-http-strict", storedJSON: goldenJSON, kernelYAML: strictYAML},
		{name: "no-profile"},
	}
	for _, prof := range profiles {
		for _, sample := range samples {
			t.Run(prof.name+"/"+sample.ID, func(t *testing.T) {
				runner := runSession(t, sample.Source, sample.Text, prof.storedJSON)
				preview := runKernel(t, sample.Source, sample.Text, prof.kernelYAML, runner)

				assertParity(t, runner, preview)
				t.Logf("status=%s events=%d diagnostics=%d bundle=%t", runner.status, len(runner.events), len(runner.diagnostics), preview.Bundle != nil)
			})
		}
	}
}

// TestWASMKernelMatchesSessionRunner_NegativeControl mutates one PID field
// (PID-5 family name) and asserts that both paths change, and change
// identically: the comparison above is sensitive to engine output, not
// vacuously equal.
func TestWASMKernelMatchesSessionRunner_NegativeControl(t *testing.T) {
	samples, err := kernel.Samples()
	if err != nil {
		t.Fatalf("Samples: %v", err)
	}
	sample := samples[0] // ADT A01 - ICU Admission
	mutated := strings.Replace(sample.Text, "|DOE^JANE^MARIE^^MS|", "|ROE^JANE^MARIE^^MS|", 1)
	if mutated == sample.Text {
		t.Fatal("negative control did not mutate PID-5")
	}
	goldenJSON, err := os.ReadFile(goldenStrictProfile)
	if err != nil {
		t.Fatalf("read golden profile: %v", err)
	}
	strictYAML := builtInProfileYAML(t, "adt-http-strict")

	originalRun := runSession(t, sample.Source, sample.Text, goldenJSON)
	mutatedRun := runSession(t, sample.Source, mutated, goldenJSON)
	originalPreview := runKernel(t, sample.Source, sample.Text, strictYAML, originalRun)
	mutatedPreview := runKernel(t, sample.Source, mutated, strictYAML, mutatedRun)

	// Both paths still agree with each other on the mutated message.
	assertParity(t, mutatedRun, mutatedPreview)

	// And the mutation is visible to the comparator on both sides: a kernel
	// that ignored its input, or a comparator that masked too much, fails here.
	originalKernel := canonicalEvents(t, "kernel", kernelEvents(originalPreview))
	mutatedKernel := canonicalEvents(t, "kernel", kernelEvents(mutatedPreview))
	originalSession := canonicalEvents(t, "session", originalRun.events)
	mutatedSession := canonicalEvents(t, "session", mutatedRun.events)
	if bytes.Equal(originalKernel, mutatedKernel) {
		t.Fatal("kernel events did not change when PID-5 changed")
	}
	if bytes.Equal(originalSession, mutatedSession) {
		t.Fatal("session events did not change when PID-5 changed")
	}
	// The failing line the parity assertion would print had only one side
	// seen the mutation (session original vs kernel mutated).
	t.Logf("negative control: %s", firstDifference(originalSession, mutatedKernel))
}

type sessionOutcome struct {
	status            session.RunStatus
	events            []kernel.Event
	diagnostics       []kernel.Diagnostic
	profileArtifactID string
	profileRevision   int
}

func runSession(t *testing.T, source, raw string, profileJSON []byte) sessionOutcome {
	t.Helper()
	ctx := context.Background()
	store := session.NewMemoryStore()
	runner := session.NewRunner(store, nil)
	sess, err := store.CreateSession(ctx, session.CreateSessionRequest{Name: "wasm parity"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	// Retain: the playground never redacts, and the samples are synthetic.
	sample, err := store.AddSample(ctx, sess.ID, session.AddSampleRequest{
		Name:      "parity sample",
		Format:    events.FormatHL7v2,
		Source:    source,
		Raw:       raw,
		PHIPolicy: session.PHIPolicyRetain,
	})
	if err != nil {
		t.Fatalf("AddSample: %v", err)
	}
	request := session.RunRequest{SessionID: sess.ID, SampleID: sample.ID}
	outcome := sessionOutcome{}
	if profileJSON != nil {
		draft, err := store.SaveArtifactDraft(ctx, sess.ID, session.SaveArtifactDraftRequest{
			Kind:    session.ArtifactKindMappingProfile,
			Name:    "adt-http strict",
			Content: profileJSON,
		})
		if err != nil {
			t.Fatalf("SaveArtifactDraft: %v", err)
		}
		request.ProfileRevisionID = draft.RevisionID
		outcome.profileArtifactID = draft.ID
		outcome.profileRevision = draft.Version
	}
	run, err := runner.RunHL7v2(ctx, request)
	if run == nil {
		t.Fatalf("RunHL7v2 returned no run: %v", err)
	}
	if err != nil && run.Status != session.RunStatusFailed {
		t.Fatalf("RunHL7v2 error %v with status %s", err, run.Status)
	}
	outcome.status = run.Status
	outcome.events = make([]kernel.Event, 0, len(run.Events))
	for _, event := range run.Events {
		outcome.events = append(outcome.events, kernel.Event{Type: event.Type, Payload: event.Payload})
	}
	outcome.diagnostics = make([]kernel.Diagnostic, 0, len(run.Diagnostics))
	for _, diagnostic := range run.Diagnostics {
		outcome.diagnostics = append(outcome.diagnostics, kernel.Diagnostic{
			Severity: diagnostic.Severity,
			Code:     diagnostic.Code,
			Path:     diagnostic.Path,
			Message:  diagnostic.Message,
		})
	}
	return outcome
}

func runKernel(t *testing.T, source, raw, profileYAML string, runner sessionOutcome) kernel.PreviewResponse {
	t.Helper()
	identity := kernel.ProfileIdentity{ArtifactID: kernel.PlaygroundProfileID, Revision: 1}
	if runner.profileArtifactID != "" {
		// The session store names the draft; the compiled profile's ID is the
		// artifact ID on both paths and lands in source_profile_id.
		identity = kernel.ProfileIdentity{ArtifactID: runner.profileArtifactID, Revision: runner.profileRevision}
	}
	return kernel.PreviewWith(kernel.PreviewRequest{
		Message:     raw,
		Format:      kernel.FormatHL7v2,
		ProfileYAML: profileYAML,
		Source:      source,
	}, identity)
}

func assertParity(t *testing.T, runner sessionOutcome, preview kernel.PreviewResponse) {
	t.Helper()
	wantOK := runner.status == session.RunStatusSucceeded
	if preview.OK != wantOK {
		t.Fatalf("kernel ok = %t, session run status = %s (problems %+v)", preview.OK, runner.status, preview.Problems)
	}
	sessionEvents := canonicalEvents(t, "session", runner.events)
	kernelEventsJSON := canonicalEvents(t, "kernel", kernelEvents(preview))
	if !bytes.Equal(sessionEvents, kernelEventsJSON) {
		t.Fatalf("events diverge: %s", firstDifference(sessionEvents, kernelEventsJSON))
	}
	sessionDiagnostics := canonicalJSON(t, runner.diagnostics)
	kernelDiagnostics := canonicalJSON(t, preview.Diagnostics)
	if !bytes.Equal(sessionDiagnostics, kernelDiagnostics) {
		t.Fatalf("diagnostics diverge: %s", firstDifference(sessionDiagnostics, kernelDiagnostics))
	}
}

func kernelEvents(preview kernel.PreviewResponse) []kernel.Event {
	return preview.Events
}

// canonicalEvents renders events as canonical JSON (sorted keys, no
// insignificant whitespace) with the volatile meta members replaced by a
// placeholder after checking they are present and well-formed.
func canonicalEvents(t *testing.T, side string, evts []kernel.Event) []byte {
	t.Helper()
	out := make([]any, 0, len(evts))
	for index, event := range evts {
		decoder := json.NewDecoder(bytes.NewReader(event.Payload))
		decoder.UseNumber()
		var payload map[string]any
		if err := decoder.Decode(&payload); err != nil {
			t.Fatalf("%s event %d payload: %v", side, index, err)
		}
		for key, pattern := range volatileEventKeys {
			value, ok := payload[key].(string)
			if !ok || !pattern.MatchString(value) {
				t.Fatalf("%s event %d: volatile member %q = %v is missing or malformed", side, index, key, payload[key])
			}
			payload[key] = "<volatile>"
		}
		out = append(out, map[string]any{"type": event.Type, "payload": payload})
	}
	return canonicalJSON(t, out)
}

func canonicalJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var generic any
	if err := decoder.Decode(&generic); err != nil {
		t.Fatalf("decode: %v", err)
	}
	canonical, err := json.Marshal(generic)
	if err != nil {
		t.Fatalf("remarshal: %v", err)
	}
	return canonical
}

// firstDifference reports the byte offset and a window of both documents at
// the first byte where they differ.
func firstDifference(want, got []byte) string {
	limit := min(len(want), len(got))
	offset := limit
	for i := 0; i < limit; i++ {
		if want[i] != got[i] {
			offset = i
			break
		}
	}
	if offset == limit && len(want) == len(got) {
		return "identical"
	}
	window := func(doc []byte) string {
		start := max(0, offset-40)
		end := min(len(doc), offset+40)
		return string(doc[start:end])
	}
	return fmt.Sprintf("offset %d: session …%s… kernel …%s…", offset, window(want), window(got))
}

func builtInProfileYAML(t *testing.T, id string) string {
	t.Helper()
	profiles, err := kernel.Profiles()
	if err != nil {
		t.Fatalf("Profiles: %v", err)
	}
	for _, prof := range profiles {
		if prof.ID == id {
			return prof.YAML
		}
	}
	t.Fatalf("no built-in profile %q", id)
	return ""
}
