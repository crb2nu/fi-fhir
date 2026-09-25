package fhirout

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

// bundleCaptureEnv names the directory the official-validator gate reads the
// delivered Bundles from (Slice 5.1c-β, `make fhir-official`,
// ci/test-fhir-official.yml). Unset — the default — nothing is written and
// the tests below are ordinary unit tests.
const bundleCaptureEnv = "FI_FHIR_FHIR_CAPTURE_DIR"

// captureBundle writes bundle into dir as `<event_type>.bundle.json`, encoded
// exactly as the durable `fhir` transport encodes its request body
// (json.Marshal in internal/integration/destination/fhir_transport.go), so the
// validator reads the bytes a FHIR server would receive. An empty dir is the
// opt-out and writes nothing.
func captureBundle(dir string, eventType events.EventType, bundle *fhir.Bundle) (string, error) {
	if dir == "" {
		return "", nil
	}
	body, err := json.Marshal(bundle)
	if err != nil {
		return "", fmt.Errorf("encode %s bundle: %w", eventType, err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create capture dir: %w", err)
	}
	path := filepath.Join(dir, string(eventType)+".bundle.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

// TestDeliveredBundlesForEverySupportedEventType builds, for every event type
// the durable `fhir` transport accepts, the transaction Bundle that transport
// delivers — through the same Project → CreateConditionalTransactionBundle
// path over the same stored-payload shape — and, when FI_FHIR_FHIR_CAPTURE_DIR
// is set, writes each one out for the HL7 validator. No PostgreSQL, no network.
func TestDeliveredBundlesForEverySupportedEventType(t *testing.T) {
	dir := os.Getenv(bundleCaptureEnv)
	supported := SupportedEventTypes()
	for _, eventType := range supported {
		projection, err := Project(eventType, projectTestPayload(t, projectTestEventFor(t, eventType)))
		if err != nil {
			t.Fatalf("Project(%s): %v", eventType, err)
		}
		bundle, err := CreateConditionalTransactionBundle(projection)
		if err != nil {
			t.Fatalf("CreateConditionalTransactionBundle(%s): %v", eventType, err)
		}
		if bundle.ResourceType != "Bundle" || bundle.Type != "transaction" || len(bundle.Entry) != len(projection.Resources) {
			t.Fatalf("%s bundle = %s/%s with %d entries, want a transaction of %d",
				eventType, bundle.ResourceType, bundle.Type, len(bundle.Entry), len(projection.Resources))
		}
		for index, entry := range bundle.Entry {
			if entry.Request == nil || entry.Request.Method != http.MethodPut || !strings.Contains(entry.Request.URL, "?identifier=") {
				t.Fatalf("%s entry %d is not a conditional update: %+v", eventType, index, entry.Request)
			}
		}
		if _, err := captureBundle(dir, eventType, bundle); err != nil {
			t.Fatalf("capture %s: %v", eventType, err)
		}
	}
	if dir == "" {
		return
	}
	for _, eventType := range supported {
		if _, err := os.Stat(filepath.Join(dir, string(eventType)+".bundle.json")); err != nil {
			t.Fatalf("%s bundle was not captured: %v", eventType, err)
		}
	}
	t.Logf("captured %d delivered bundles into %s", len(supported), dir)
}

// TestBundleCaptureWritesValidJSON proves the capture writes a parseable
// transaction Bundle under the `<event_type>.bundle.json` name the gate
// expects, byte-identical to the transport's encoding, and that the opt-out
// (an empty directory) writes nothing.
func TestBundleCaptureWritesValidJSON(t *testing.T) {
	t.Parallel()

	projection, err := Project(events.EventPatientAdmit, projectTestPayload(t, projectTestAdmit()))
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	bundle, err := CreateConditionalTransactionBundle(projection)
	if err != nil {
		t.Fatalf("CreateConditionalTransactionBundle: %v", err)
	}

	if path, err := captureBundle("", events.EventPatientAdmit, bundle); err != nil || path != "" {
		t.Fatalf("captureBundle with no directory = %q, %v; want a no-op", path, err)
	}

	dir := t.TempDir()
	path, err := captureBundle(dir, events.EventPatientAdmit, bundle)
	if err != nil {
		t.Fatalf("captureBundle: %v", err)
	}
	if filepath.Base(path) != "patient_admit.bundle.json" {
		t.Fatalf("captured as %q, want patient_admit.bundle.json", filepath.Base(path))
	}
	written, err := os.ReadFile(path) // #nosec G304 -- path is inside t.TempDir().
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	wire, _ := json.Marshal(bundle)
	if string(written) != string(wire) {
		t.Fatal("captured bytes differ from the transport's json.Marshal encoding")
	}
	var decoded struct {
		ResourceType string            `json:"resourceType"`
		Type         string            `json:"type"`
		Entry        []json.RawMessage `json:"entry"`
	}
	if err := json.Unmarshal(written, &decoded); err != nil {
		t.Fatalf("captured bundle is not JSON: %v", err)
	}
	if decoded.ResourceType != "Bundle" || decoded.Type != "transaction" || len(decoded.Entry) != 2 {
		t.Fatalf("captured %s/%s with %d entries, want a transaction Bundle of 2", decoded.ResourceType, decoded.Type, len(decoded.Entry))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("capture dir holds %d files, want exactly the one bundle", len(entries))
	}

	// A capture that cannot be written fails the test rather than leaving the
	// gate to discover a missing bundle.
	if _, err := captureBundle(filepath.Join(path, "not-a-dir"), events.EventPatientAdmit, bundle); err == nil {
		t.Fatal("capture under a regular file succeeded, want a create error")
	}
}
