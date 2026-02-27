package main

import (
	"os"
	"path/filepath"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// =============================================================================
// runParse — flag-parsing branches not yet covered by main_test.go
// =============================================================================

func TestParse_MissingSourceValue(t *testing.T) {
	_, _, err := runCLI(t, "parse", "--source")
	assertError(t, err)
	assertErrorContains(t, err, "--source requires a value")
}

func TestParse_ShortSourceFlagMissingValue(t *testing.T) {
	_, _, err := runCLI(t, "parse", "-s")
	assertError(t, err)
	assertErrorContains(t, err, "--source requires a value")
}

func TestParse_MissingProfileValue(t *testing.T) {
	_, _, err := runCLI(t, "parse", "--profile")
	assertError(t, err)
	assertErrorContains(t, err, "--profile requires a value")
}

func TestParse_MissingDelimiterValue(t *testing.T) {
	_, _, err := runCLI(t, "parse", "--delimiter")
	assertError(t, err)
	assertErrorContains(t, err, "--delimiter requires a value")
}

func TestParse_ShortDelimiterFlagMissingValue(t *testing.T) {
	_, _, err := runCLI(t, "parse", "-d")
	assertError(t, err)
	assertErrorContains(t, err, "--delimiter requires a value")
}

func TestParse_MissingEventTypeValue(t *testing.T) {
	_, _, err := runCLI(t, "parse", "--event-type")
	assertError(t, err)
	assertErrorContains(t, err, "--event-type requires a value")
}

func TestParse_ShortEventTypeFlagMissingValue(t *testing.T) {
	_, _, err := runCLI(t, "parse", "-t")
	assertError(t, err)
	assertErrorContains(t, err, "--event-type requires a value")
}

func TestParse_MissingEDICompanionValue(t *testing.T) {
	_, _, err := runCLI(t, "parse", "--edi-companion")
	assertError(t, err)
	assertErrorContains(t, err, "--edi-companion requires a value")
}

func TestParse_MissingEDICompanionDirValue(t *testing.T) {
	_, _, err := runCLI(t, "parse", "--edi-companion-dir")
	assertError(t, err)
	assertErrorContains(t, err, "--edi-companion-dir requires a value")
}

func TestParse_ShortHelpFlag(t *testing.T) {
	stdout, _, err := runCLI(t, "parse", "-h")
	assertNoError(t, err)
	assertContains(t, stdout, "parse")
}

func TestParse_NoHeaderFlag(t *testing.T) {
	tmpDir := t.TempDir()
	csvContent := "field1,field2\nvalue1,value2\n"
	csvPath := createTempFile(t, tmpDir, "noheader*.csv", csvContent)

	stdout, _, err := runCLI(t, "parse", "--format", "csv", "--no-header", csvPath)
	assertNoError(t, err)
	// The output should be JSON with parsed data
	assertContains(t, stdout, "{")
}

func TestParse_InferSchemaFlag(t *testing.T) {
	tmpDir := t.TempDir()
	csvContent := "name,age\nAlice,30\nBob,25\n"
	csvPath := createTempFile(t, tmpDir, "infer*.csv", csvContent)

	stdout, _, err := runCLI(t, "parse", "--format", "csv", "--infer-schema", csvPath)
	assertNoError(t, err)
	assertContains(t, stdout, "{")
}

func TestParse_ProfileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	hl7Content := "MSH|^~\\&|SRC|FAC|DEST|FAC|20240101120000||ADT^A01|MSG001|P|2.5\r"
	inputPath := createTempFile(t, tmpDir, "msg*.hl7", hl7Content)

	_, _, err := runCLI(t, "parse", "--profile", "/nonexistent/profile.yaml", inputPath)
	assertError(t, err)
	assertErrorContains(t, err, "failed to load profile")
}

// =============================================================================
// runCompanion — flag-parsing branches not covered by companion_test.go
// =============================================================================

func TestCompanion_DirFlagMissingValue(t *testing.T) {
	_, _, err := runCLI(t, "companion", "list", "--dir")
	assertError(t, err)
	assertErrorContains(t, err, "--dir requires a value")
}

func TestCompanion_FormatFlagMissingValue(t *testing.T) {
	_, _, err := runCLI(t, "companion", "show", "test", "--format")
	assertError(t, err)
	assertErrorContains(t, err, "--format requires a value")
}

func TestCompanion_InvalidFormat(t *testing.T) {
	_, _, err := runCLI(t, "companion", "show", "test", "--format", "xml")
	assertError(t, err)
	assertErrorContains(t, err, "invalid --format")
}

func TestCompanion_ListUnknownFlag(t *testing.T) {
	_, _, err := runCLI(t, "companion", "list", "--unknown")
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestCompanion_ListUnexpectedArgs(t *testing.T) {
	_, _, err := runCLI(t, "companion", "list", "extra-arg")
	assertError(t, err)
	assertErrorContains(t, err, "unexpected args")
}

func TestCompanion_ShowMissingGuideID(t *testing.T) {
	_, _, err := runCLI(t, "companion", "show")
	assertError(t, err)
	assertErrorContains(t, err, "usage: fi-fhir companion show")
}

func TestCompanion_ShowTooManyArgs(t *testing.T) {
	_, _, err := runCLI(t, "companion", "show", "one", "two")
	assertError(t, err)
	assertErrorContains(t, err, "usage: fi-fhir companion show")
}

func TestCompanion_ValidateMissingArg(t *testing.T) {
	_, _, err := runCLI(t, "companion", "validate")
	assertError(t, err)
	assertErrorContains(t, err, "usage: fi-fhir companion validate")
}

func TestCompanion_ValidateFileNotFound(t *testing.T) {
	_, _, err := runCLI(t, "companion", "validate", "/nonexistent/guide.yaml")
	assertError(t, err)
}

func TestCompanion_ExportMissingArgs(t *testing.T) {
	_, _, err := runCLI(t, "companion", "export", "medicare_part_b")
	assertError(t, err)
	assertErrorContains(t, err, "usage: fi-fhir companion export")
}

func TestCompanion_ExportUnknownGuide(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "out.json")

	_, _, err := runCLI(t, "companion", "export", "nonexistent_guide", outPath)
	assertError(t, err)
	assertErrorContains(t, err, "unknown companion guide")
}

func TestCompanion_ExportYAML(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "export.yaml")

	_, _, err := runCLI(t, "companion", "export", "medicare_part_b", outPath, "--format", "yaml")
	assertNoError(t, err)

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}
	assertContains(t, string(b), "medicare_part_b")
}

func TestCompanion_ListHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "companion", "list", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "companion")
}

func TestCompanion_ShowHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "companion", "show", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "companion")
}

func TestCompanion_ValidateHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "companion", "validate", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "companion")
}

func TestCompanion_ExportHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "companion", "export", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "companion")
}

func TestCompanion_ListWithDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Use a directory with no guides — should still list built-in guides
	stdout, _, err := runCLI(t, "companion", "list", "--dir", tmpDir)
	assertNoError(t, err)
	assertContains(t, stdout, "medicare_part_b")
}

func TestCompanion_HelpSubcommand(t *testing.T) {
	stdout, _, err := runCLI(t, "companion", "help")
	assertNoError(t, err)
	assertContains(t, stdout, "companion")
}

func TestCompanion_ShortHelpFlag(t *testing.T) {
	stdout, _, err := runCLI(t, "companion", "-h")
	assertNoError(t, err)
	assertContains(t, stdout, "companion")
}

// =============================================================================
// runValidate — flag-parsing branches
// =============================================================================

func TestValidate_MissingProfileValue(t *testing.T) {
	_, _, err := runCLI(t, "validate", "--profile")
	assertError(t, err)
	assertErrorContains(t, err, "--profile requires a value")
}

func TestValidate_ShortProfileMissingValue(t *testing.T) {
	_, _, err := runCLI(t, "validate", "-p")
	assertError(t, err)
	assertErrorContains(t, err, "--profile requires a value")
}

func TestValidate_MissingMessageValue(t *testing.T) {
	_, _, err := runCLI(t, "validate", "--message")
	assertError(t, err)
	assertErrorContains(t, err, "--message requires a value")
}

func TestValidate_ShortMessageMissingValue(t *testing.T) {
	_, _, err := runCLI(t, "validate", "-m")
	assertError(t, err)
	assertErrorContains(t, err, "--message requires a value")
}

func TestValidate_MissingFormatValue(t *testing.T) {
	_, _, err := runCLI(t, "validate", "--format")
	assertError(t, err)
	assertErrorContains(t, err, "--format requires a value")
}

func TestValidate_ShortFormatMissingValue(t *testing.T) {
	_, _, err := runCLI(t, "validate", "-f")
	assertError(t, err)
	assertErrorContains(t, err, "--format requires a value")
}

func TestValidate_UnknownFlag(t *testing.T) {
	_, _, err := runCLI(t, "validate", "--unknown-flag")
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestValidate_NoProfileSpecified(t *testing.T) {
	_, _, err := runCLI(t, "validate")
	assertError(t, err)
	assertErrorContains(t, err, "no profile specified")
}

// =============================================================================
// appendParseWarningsToOutputData — uncovered type switch branches
// =============================================================================

func TestAppendParseWarningsToOutputData_NilOutputData(t *testing.T) {
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}
	// Should not panic
	appendParseWarningsToOutputData(nil, extra)
}

func TestAppendParseWarningsToOutputData_EmptyExtra(t *testing.T) {
	evt := &dummyEvent{EventMeta: events.EventMeta{}}
	out := map[string]interface{}{"event": evt}
	// Should not modify anything when extra is empty
	appendParseWarningsToOutputData(out, nil)
	if len(evt.ParseWarnings) != 0 {
		t.Fatalf("expected no warnings when extra is nil, got %d", len(evt.ParseWarnings))
	}
}

func TestAppendParseWarningsToOutputData_SliceOfInterfaces(t *testing.T) {
	evt1 := &dummyEvent{EventMeta: events.EventMeta{}}
	evt2 := &dummyEvent{EventMeta: events.EventMeta{}}
	out := []interface{}{evt1, evt2}
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}

	appendParseWarningsToOutputData(out, extra)

	if len(evt1.ParseWarnings) != 1 {
		t.Fatalf("expected 1 warning on evt1, got %d", len(evt1.ParseWarnings))
	}
	if len(evt2.ParseWarnings) != 1 {
		t.Fatalf("expected 1 warning on evt2, got %d", len(evt2.ParseWarnings))
	}
}

func TestAppendParseWarningsToOutputData_MapEventsArray(t *testing.T) {
	evt1 := &dummyEvent{EventMeta: events.EventMeta{}}
	evt2 := &dummyEvent{EventMeta: events.EventMeta{}}
	out := map[string]interface{}{
		"events": []interface{}{evt1, evt2},
	}
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}

	appendParseWarningsToOutputData(out, extra)

	if len(evt1.ParseWarnings) != 1 {
		t.Fatalf("expected 1 warning on evt1, got %d", len(evt1.ParseWarnings))
	}
	if len(evt2.ParseWarnings) != 1 {
		t.Fatalf("expected 1 warning on evt2, got %d", len(evt2.ParseWarnings))
	}
}

func TestAppendParseWarningsToOutputData_DefaultPath(t *testing.T) {
	evt := &dummyEvent{EventMeta: events.EventMeta{}}
	// Passing a single event directly (not wrapped in map or slice)
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}

	appendParseWarningsToOutputData(evt, extra)

	if len(evt.ParseWarnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(evt.ParseWarnings))
	}
}

func TestAppendParseWarningsToEvent_NilEvent(t *testing.T) {
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}
	if ok := appendParseWarningsToEvent(nil, extra); ok {
		t.Fatalf("expected false for nil event")
	}
}

func TestAppendParseWarningsToEvent_EmptyExtra(t *testing.T) {
	evt := &dummyEvent{EventMeta: events.EventMeta{}}
	if ok := appendParseWarningsToEvent(evt, nil); ok {
		t.Fatalf("expected false for nil extra")
	}
}

func TestAppendParseWarningsToEvent_NonStructPointer(t *testing.T) {
	extra := []events.ParseWarning{{Phase: "p", Code: "C"}}
	s := "not a struct"
	if ok := appendParseWarningsToEvent(&s, extra); ok {
		t.Fatalf("expected false for pointer to non-struct")
	}
}
