package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func repoPath(relativePath string) string {
	return filepath.Join("..", "..", relativePath)
}

func TestVendorTemplateLint_CleanFixtures(t *testing.T) {
	tests := []struct {
		name         string
		templatePath string
		samplePath   string
	}{
		{
			name:         "epic",
			templatePath: repoPath(filepath.Join("profiles", "templates", "hl7v2", "epic_adt.yaml")),
			samplePath:   testdataPath(t, filepath.Join("hl7v2", "vendors", "epic", "adt_a01_clean.hl7")),
		},
		{
			name:         "cerner",
			templatePath: repoPath(filepath.Join("profiles", "templates", "hl7v2", "cerner_adt.yaml")),
			samplePath:   testdataPath(t, filepath.Join("hl7v2", "vendors", "cerner", "adt_a01_clean.hl7")),
		},
		{
			name:         "meditech",
			templatePath: repoPath(filepath.Join("profiles", "templates", "hl7v2", "meditech_adt.yaml")),
			samplePath:   testdataPath(t, filepath.Join("hl7v2", "vendors", "meditech", "adt_a01_clean.hl7")),
		},
		{
			name:         "allscripts",
			templatePath: repoPath(filepath.Join("profiles", "templates", "hl7v2", "allscripts_adt.yaml")),
			samplePath:   testdataPath(t, filepath.Join("hl7v2", "vendors", "allscripts", "adt_a01_clean.hl7")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _, err := runCLI(t, "profile", "lint", tt.templatePath, "--samples", tt.samplePath, "--allow-warnings")
			if err != nil {
				t.Fatalf("profile lint: %v", err)
			}
			if !strings.Contains(stdout, "passed lint") {
				t.Fatalf("expected lint success output, got: %s", stdout)
			}
		})
	}
}

func TestVendorTemplateLint_DriftFixtures(t *testing.T) {
	tests := []struct {
		name         string
		templatePath string
		samplePath   string
	}{
		{
			name:         "epic",
			templatePath: repoPath(filepath.Join("profiles", "templates", "hl7v2", "epic_adt.yaml")),
			samplePath:   testdataPath(t, filepath.Join("hl7v2", "vendors", "epic", "adt_a01_drift_missing_pv1.hl7")),
		},
		{
			name:         "cerner",
			templatePath: repoPath(filepath.Join("profiles", "templates", "hl7v2", "cerner_adt.yaml")),
			samplePath:   testdataPath(t, filepath.Join("hl7v2", "vendors", "cerner", "adt_a01_drift_missing_pv1.hl7")),
		},
		{
			name:         "meditech",
			templatePath: repoPath(filepath.Join("profiles", "templates", "hl7v2", "meditech_adt.yaml")),
			samplePath:   testdataPath(t, filepath.Join("hl7v2", "vendors", "meditech", "adt_a01_drift_missing_pv1.hl7")),
		},
		{
			name:         "allscripts",
			templatePath: repoPath(filepath.Join("profiles", "templates", "hl7v2", "allscripts_adt.yaml")),
			samplePath:   testdataPath(t, filepath.Join("hl7v2", "vendors", "allscripts", "adt_a01_drift_missing_pv1.hl7")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runCLI(t, "profile", "lint", tt.templatePath, "--samples", tt.samplePath)
			if err == nil {
				t.Fatalf("expected lint to fail on warnings by default")
			}

			stdout, _, err := runCLI(t, "profile", "lint", tt.templatePath, "--samples", tt.samplePath, "--allow-warnings")
			if err != nil {
				t.Fatalf("profile lint --allow-warnings: %v", err)
			}
			if !strings.Contains(stdout, "passed lint") {
				t.Fatalf("expected lint success output, got: %s", stdout)
			}
		})
	}
}
