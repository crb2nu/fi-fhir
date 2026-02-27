package main

import (
	"os"
	"testing"
)

func TestProfileInferGolden_ADTA01(t *testing.T) {
	goldenPath := testdataPath(t, "profiles/inferred/adt_a01_inferred.yaml")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	sample := testdataPath(t, "adt_a01_sample.hl7")
	stdout, _, err := runCLI(t,
		"profile", "infer",
		"--id", "inferred_adt_a01",
		"--name", "Inferred ADT A01",
		"--version", "0.1.0",
		sample,
	)
	if err != nil {
		t.Fatalf("profile infer: %v", err)
	}

	if stdout != string(golden) {
		t.Fatalf("golden mismatch for %s", goldenPath)
	}
}
