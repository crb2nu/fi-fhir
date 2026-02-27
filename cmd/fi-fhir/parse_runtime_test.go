package main

import (
	"os"
	"testing"
)

func TestParse_HL7FromStdinDash(t *testing.T) {
	inputPath := testdataPath(t, "adt_a01_sample.hl7")
	data, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read test HL7 message: %v", err)
	}

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	defer func() {
		os.Stdin = oldStdin
		_ = r.Close()
	}()
	os.Stdin = r

	if _, err := w.Write(data); err != nil {
		t.Fatalf("write stdin payload: %v", err)
	}
	_ = w.Close()

	stdout, _, err := runCLI(t, "parse", "--format", "hl7v2", "-")
	assertNoError(t, err)
	assertContains(t, stdout, "\"type\":\"patient_admit\"")
}

func TestParse_EDIWarningsPrintedWhenRequested(t *testing.T) {
	ediPath := testdataPath(t, "edi/837p_minimal.edi")

	_, stderr, err := runCLI(t,
		"parse",
		"--format", "edi",
		"--edi-companion", "auto",
		"--warnings",
		ediPath,
	)
	assertNoError(t, err)
	assertContains(t, stderr, "Warnings (")
	assertContains(t, stderr, "edi_companion")
}
