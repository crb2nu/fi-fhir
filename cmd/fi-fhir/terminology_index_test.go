package main

import (
	"os"
	"strings"
	"testing"
)

// TestRunTerminologyIndex_Help verifies --help prints usage without error.
func TestRunTerminologyIndex_Help(t *testing.T) {
	err := runTerminologyIndex([]string{"--help"})
	if err != nil {
		t.Fatalf("expected no error for --help, got: %v", err)
	}
}

// TestRunTerminologyIndex_HelpShort verifies -h prints usage without error.
func TestRunTerminologyIndex_HelpShort(t *testing.T) {
	err := runTerminologyIndex([]string{"-h"})
	if err != nil {
		t.Fatalf("expected no error for -h, got: %v", err)
	}
}

// TestRunTerminologyIndex_MissingVocabulary verifies error when vocabulary is not given.
func TestRunTerminologyIndex_MissingVocabulary(t *testing.T) {
	err := runTerminologyIndex([]string{})
	if err == nil || !strings.Contains(err.Error(), "vocabulary required") {
		t.Fatalf("expected 'vocabulary required' error, got: %v", err)
	}
}

// TestRunTerminologyIndex_MissingSource verifies error when source is not given.
func TestRunTerminologyIndex_MissingSource(t *testing.T) {
	err := runTerminologyIndex([]string{"loinc"})
	if err == nil || !strings.Contains(err.Error(), "source file required") {
		t.Fatalf("expected 'source file required' error, got: %v", err)
	}
}

// TestRunTerminologyIndex_UnsupportedVocabulary verifies error for unknown vocabulary.
func TestRunTerminologyIndex_UnsupportedVocabulary(t *testing.T) {
	err := runTerminologyIndex([]string{"cpt", "/tmp/fake.csv"})
	if err == nil || !strings.Contains(err.Error(), "unsupported vocabulary") {
		t.Fatalf("expected 'unsupported vocabulary' error, got: %v", err)
	}
}

// TestRunTerminologyIndex_UnknownFlag verifies error for unrecognized flag.
func TestRunTerminologyIndex_UnknownFlag(t *testing.T) {
	err := runTerminologyIndex([]string{"--nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected 'unknown flag' error, got: %v", err)
	}
}

// TestRunTerminologyIndex_FlagParsing exercises all flag paths for coverage.
func TestRunTerminologyIndex_FlagParsing(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"vocabulary flag missing value", []string{"--vocabulary"}, "--vocabulary requires a value"},
		{"source flag missing value", []string{"--source"}, "--source requires a value"},
		{"version flag missing value", []string{"--version"}, "--version requires a value"},
		{"qdrant-url flag missing value", []string{"--qdrant-url"}, "--qdrant-url requires a value"},
		{"embedding-url flag missing value", []string{"--embedding-url"}, "--embedding-url requires a value"},
		{"model flag missing value", []string{"--model"}, "--model requires a value"},
		{"batch-size flag missing value", []string{"--batch-size"}, "--batch-size requires a value"},
		{"dimensions flag missing value", []string{"--dimensions"}, "--dimensions requires a value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runTerminologyIndex(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected error containing %q, got: %v", tt.want, err)
			}
		})
	}
}

// TestRunTerminologyIndex_FullFlagExercise exercises the complete flag-parsing
// path with all flags provided. The function should fail on builder creation
// (no Qdrant running), not on arg parsing.
func TestRunTerminologyIndex_FullFlagExercise(t *testing.T) {
	// Create a dummy source file
	f, err := os.CreateTemp("", "idx-test-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("code,display\n1234-5,Test Code\n")
	_ = f.Close()
	defer os.Remove(f.Name())

	err = runTerminologyIndex([]string{
		"--vocabulary", "loinc",
		"--source", f.Name(),
		"--version", "2.77",
		"--qdrant-url", "http://localhost:0", // invalid port to fail fast
		"--embedding-url", "http://localhost:0",
		"--model", "test-model",
		"--batch-size", "8",
		"--dimensions", "512",
		"--drop",
	})
	// Should fail at builder creation or build, NOT at flag parsing
	if err == nil {
		t.Fatal("expected error from builder, got nil")
	}
	if strings.Contains(err.Error(), "requires a value") || strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("failed at flag parsing, should have passed parsing: %v", err)
	}
}

// TestRunTerminologyIndex_PositionalArgs verifies positional vocabulary and source.
func TestRunTerminologyIndex_PositionalArgs(t *testing.T) {
	err := runTerminologyIndex([]string{"loinc", "/nonexistent/file.csv"})
	// Should pass arg parsing and fail at builder creation (no Qdrant)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "vocabulary required") || strings.Contains(err.Error(), "source file required") {
		t.Fatalf("failed at arg parsing, should have passed: %v", err)
	}
}
