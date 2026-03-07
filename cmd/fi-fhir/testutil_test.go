package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureOutput captures stdout and stderr during function execution.
func captureOutput(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	// Save original streams
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	// Create pipes
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create stderr pipe: %v", err)
	}

	os.Stdout = wOut
	os.Stderr = wErr

	// Run the function
	fn()

	// Close write ends
	wOut.Close()
	wErr.Close()

	// Read output
	var bufOut, bufErr bytes.Buffer
	io.Copy(&bufOut, rOut)
	io.Copy(&bufErr, rErr)

	return bufOut.String(), bufErr.String()
}

// runCLI simulates running the CLI with given args and returns stdout, stderr, and exit error.
// It does NOT call os.Exit, allowing tests to capture results.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	// Save original args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set new args (include program name)
	os.Args = append([]string{"fi-fhir"}, args...)

	// We need to test individual functions rather than main()
	// since main() calls os.Exit. Let's dispatch manually.
	if len(args) == 0 {
		return "", "", nil
	}

	var capturedErr error
	stdout, stderr = captureOutput(t, func() {
		switch args[0] {
		case "companion":
			capturedErr = runCompanion(args[1:])
		case "parse":
			capturedErr = runParse(args[1:])
		case "validate":
			capturedErr = runValidate(args[1:])
		case "profile":
			capturedErr = runProfile(args[1:])
		case "fhir":
			capturedErr = runFHIR(args[1:])
		case "workflow":
			capturedErr = runWorkflow(args[1:])
		case "config":
			capturedErr = runConfig(args[1:])
		case "eventstore":
			capturedErr = runEventStore(args[1:])
		case "projection":
			capturedErr = runProjection(args[1:])
		case "subscription":
			capturedErr = runSubscription(args[1:])
		case "serve":
			capturedErr = runServe(args[1:])
		case "storage":
			capturedErr = runStorage(args[1:])
		case "terminology":
			capturedErr = runTerminology(args[1:])
		case "etl":
			capturedErr = runETL(args[1:])
		case "llm":
			capturedErr = runLLM(args[1:])
		case "version", "--version", "-v":
			printVersion()
		case "help", "--help", "-h":
			printUsage()
		default:
			capturedErr = &unknownCommandError{command: args[0]}
		}
	})

	return stdout, stderr, capturedErr
}

// printVersion prints the version (extracted for testing).
func printVersion() {
	os.Stdout.WriteString("fi-fhir version " + version + "\n")
}

// unknownCommandError represents an unknown command error.
type unknownCommandError struct {
	command string
}

func (e *unknownCommandError) Error() string {
	return "unknown command: " + e.command
}

// testdataPath returns the path to a testdata file.
func testdataPath(t *testing.T, relativePath string) string {
	t.Helper()
	// Navigate from cmd/fi-fhir to project root, then to testdata
	return filepath.Join("..", "..", "testdata", relativePath)
}

// createTempFile creates a temporary file with the given content.
func createTempFile(t *testing.T, dir, pattern, content string) string {
	t.Helper()
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	return f.Name()
}

// assertContains checks that s contains substr.
func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("Expected output to contain %q, got:\n%s", substr, s)
	}
}

// assertNoError checks that err is nil.
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

// assertError checks that err is not nil.
func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Error("Expected an error, got nil")
	}
}

// assertErrorContains checks that err contains the expected message.
func assertErrorContains(t *testing.T, err error, expected string) {
	t.Helper()
	if err == nil {
		t.Errorf("Expected error containing %q, got nil", expected)
		return
	}
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("Expected error containing %q, got: %v", expected, err)
	}
}
