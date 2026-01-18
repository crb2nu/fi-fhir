package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferHL7v2ProfileFromPaths(t *testing.T) {
	tmpDir := t.TempDir()
	samplePath := filepath.Join(tmpDir, "sample.hl7")

	msg := "MSH|^~\\&|SND|FAC|RCV|FAC|202601180930||ADT^A01|MSG0001|P|2.5.1\r" +
		"EVN|A01|202601180930\r" +
		"PID|1||12345^^^MRN||Doe^John\r" +
		"ZPV|foo|bar\r"

	if err := os.WriteFile(samplePath, []byte(msg), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	p, report, err := InferHL7v2ProfileFromPaths([]string{samplePath}, InferHL7v2Options{
		ID:      "epic_adt",
		Name:    "Epic ADT",
		Version: "1.2.3",
	})
	if err != nil {
		t.Fatalf("InferHL7v2ProfileFromPaths: %v", err)
	}

	if p.ID != "epic_adt" {
		t.Fatalf("ID=%q, want epic_adt", p.ID)
	}
	if p.Name != "Epic ADT" {
		t.Fatalf("Name=%q, want Epic ADT", p.Name)
	}
	if p.Version != "1.2.3" {
		t.Fatalf("Version=%q, want 1.2.3", p.Version)
	}
	if p.HL7v2 == nil || p.HL7v2.DefaultVersion != "2.5.1" {
		t.Fatalf("HL7v2.DefaultVersion=%v, want 2.5.1", p.HL7v2)
	}
	if p.ZSegments == nil {
		t.Fatalf("ZSegments is nil")
	}
	if _, ok := p.ZSegments.Mappings["ZPV"]; !ok {
		t.Fatalf("expected inferred Z-segment mapping key ZPV")
	}

	if report == nil || report.Stats == nil || report.Stats.MessageCount != 1 {
		t.Fatalf("unexpected report stats: %+v", report)
	}
}
