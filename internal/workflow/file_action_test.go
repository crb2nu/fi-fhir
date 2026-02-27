package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

func TestFileAction_WritesPrettyJSONUnderBaseDir(t *testing.T) {
	baseDir := t.TempDir()

	event := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			Type:   events.EventPatientAdmit,
			Source: "test",
		},
		Patient: events.Patient{
			MRN: "123",
		},
	}

	cfg := map[string]string{
		"base_dir": baseDir,
		"path":     "{{.Type}}/{{.Patient.MRN}}.json",
		"format":   "pretty",
	}

	if err := fileAction(event, cfg); err != nil {
		t.Fatalf("fileAction: %v", err)
	}

	outPath := filepath.Join(baseDir, string(events.EventPatientAdmit), "123.json")
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if typ, _ := got["type"].(string); typ != "patient_admit" {
		t.Fatalf("type=%q, want %q", typ, "patient_admit")
	}
}

func TestFileAction_RejectsPathEscapingBaseDir(t *testing.T) {
	baseDir := t.TempDir()

	event := map[string]any{
		"type": "test",
		"ts":   time.Now().UTC().Format(time.RFC3339Nano),
	}

	cfg := map[string]string{
		"base_dir": baseDir,
		"path":     "../escape.json",
	}

	err := fileAction(event, cfg)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "escapes base_dir") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFileAction_AppendsNDJSON(t *testing.T) {
	baseDir := t.TempDir()

	cfg := map[string]string{
		"base_dir": baseDir,
		"path":     "events.ndjson",
		"format":   "ndjson",
	}

	if err := fileAction(map[string]any{"type": "a"}, cfg); err != nil {
		t.Fatalf("fileAction(1): %v", err)
	}
	if err := fileAction(map[string]any{"type": "b"}, cfg); err != nil {
		t.Fatalf("fileAction(2): %v", err)
	}

	data, err := os.ReadFile(filepath.Join(baseDir, "events.ndjson"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], `"type":"a"`) || !strings.Contains(lines[1], `"type":"b"`) {
		t.Fatalf("unexpected content:\n%s", string(data))
	}
}
