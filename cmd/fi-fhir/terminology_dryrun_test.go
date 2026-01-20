//nolint:errcheck,gosec // Test file - error/security checks relaxed for test setup
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTerminology_Load_DryRun_LOINC_NoDBRequired(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")

	dir := t.TempDir()
	loincPath := filepath.Join(dir, "LoincTable.csv")
	if err := os.WriteFile(loincPath, []byte("LOINC_NUM,COMPONENT\n1234-5,Example\n"), 0o600); err != nil {
		t.Fatalf("write LoincTable.csv: %v", err)
	}

	panelPath := filepath.Join(dir, "PanelHierarchy.csv")
	if err := os.WriteFile(panelPath, []byte("PARENT,CHILD\n"), 0o600); err != nil {
		t.Fatalf("write PanelHierarchy.csv: %v", err)
	}

	stdout, _, err := runCLI(t, "terminology", "load", "loinc", loincPath, "--version", "2.77", "--dry-run")
	assertNoError(t, err)
	assertContains(t, stdout, "DRY RUN")
	assertContains(t, stdout, "LOINC")
	assertContains(t, stdout, "PanelHierarchy.csv")
}

func TestTerminology_Load_DryRun_UMLS_NoDBRequired(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")

	dir := t.TempDir()
	for _, name := range []string{"MRCONSO.RRF", "MRREL.RRF", "MRSTY.RRF"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	stdout, _, err := runCLI(t, "terminology", "load", "umls", dir, "--version", "2024AB", "--dry-run")
	assertNoError(t, err)
	assertContains(t, stdout, "DRY RUN")
	assertContains(t, stdout, "UMLS")
}

func TestTerminology_Load_DryRun_RxNorm_NoDBRequired(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "RXNCONSO.RRF"), []byte(""), 0o600); err != nil {
		t.Fatalf("write RXNCONSO.RRF: %v", err)
	}

	stdout, _, err := runCLI(t, "terminology", "load", "rxnorm", dir, "--version", "2024-01", "--dry-run")
	assertNoError(t, err)
	assertContains(t, stdout, "DRY RUN")
	assertContains(t, stdout, "RxNorm")
}

func TestTerminology_Load_DryRun_ICD10CM_NoDBRequired(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "icd10cm.csv")
	if err := os.WriteFile(path, []byte("CODE,DESCRIPTION\nE11.9,Type 2 diabetes mellitus without complications\n"), 0o600); err != nil {
		t.Fatalf("write icd10cm.csv: %v", err)
	}

	stdout, _, err := runCLI(t, "terminology", "load", "icd10cm", path, "--version", "FY2024", "--dry-run")
	assertNoError(t, err)
	assertContains(t, stdout, "DRY RUN")
	assertContains(t, stdout, "ICD-10-CM")
}
