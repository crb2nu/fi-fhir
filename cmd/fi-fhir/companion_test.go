package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/edi/companion"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/edi/companion/builtin"
)

func TestCompanion_NoArgs_PrintsUsage(t *testing.T) {
	stdout, _, err := runCLI(t, "companion")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir companion")
	assertContains(t, stdout, "companion list")
}

func TestCompanion_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "companion", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir companion")
}

func TestCompanion_List_PrintsBuiltins(t *testing.T) {
	stdout, _, err := runCLI(t, "companion", "list")
	assertNoError(t, err)
	assertContains(t, stdout, "bcbs_sample")
	assertContains(t, stdout, "medicare_part_b")
	assertContains(t, stdout, "uhc_sample")
}

func TestCompanion_List_JSON(t *testing.T) {
	stdout, _, err := runCLI(t, "companion", "list", "--json")
	assertNoError(t, err)

	var items []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout), &items); err != nil {
		t.Fatalf("expected valid JSON output, got error: %v\noutput:\n%s", err, stdout)
	}
	if len(items) == 0 {
		t.Fatalf("expected at least one guide, got 0")
	}

	found := map[string]bool{}
	for _, it := range items {
		found[it.ID] = true
	}
	for _, id := range []string{"bcbs_sample", "medicare_part_b", "uhc_sample"} {
		if !found[id] {
			t.Fatalf("expected guide id %q in list output", id)
		}
	}
}

func TestCompanion_Show_YAML(t *testing.T) {
	stdout, _, err := runCLI(t, "companion", "show", "medicare_part_b")
	assertNoError(t, err)
	assertContains(t, stdout, "medicare_part_b")
}

func TestCompanion_Show_JSON(t *testing.T) {
	stdout, _, err := runCLI(t, "companion", "show", "medicare_part_b", "--format", "json")
	assertNoError(t, err)

	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("expected valid JSON output, got error: %v\noutput:\n%s", err, stdout)
	}
	if out.ID != "medicare_part_b" {
		t.Fatalf("expected id %q, got %q", "medicare_part_b", out.ID)
	}
}

func TestCompanion_Show_UnknownGuide(t *testing.T) {
	_, _, err := runCLI(t, "companion", "show", "does_not_exist")
	assertError(t, err)
	assertErrorContains(t, err, "unknown companion guide")
}

func TestCompanion_Validate_OK(t *testing.T) {
	tmpDir := t.TempDir()
	guidePath := filepath.Join(tmpDir, "guide.yaml")

	var buf bytes.Buffer
	guide := builtin.MedicarePartB()
	if err := companion.SaveGuideToYAML(guide, &buf); err != nil {
		t.Fatalf("failed to serialize built-in guide: %v", err)
	}
	if err := os.WriteFile(guidePath, buf.Bytes(), 0600); err != nil {
		t.Fatalf("failed to write guide file: %v", err)
	}

	stdout, _, err := runCLI(t, "companion", "validate", guidePath)
	assertNoError(t, err)
	assertContains(t, stdout, "ok: "+guide.ID)
}

func TestCompanion_Export_WritesFile(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "export.json")

	_, _, err := runCLI(t, "companion", "export", "medicare_part_b", outPath, "--format", "json")
	assertNoError(t, err)

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}
	assertContains(t, string(b), "medicare_part_b")
}

func TestCompanion_UnknownSubcommand(t *testing.T) {
	_, _, err := runCLI(t, "companion", "nope")
	assertError(t, err)
	assertErrorContains(t, err, "unknown companion subcommand")
}
