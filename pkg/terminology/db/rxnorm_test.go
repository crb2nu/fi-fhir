package db

import (
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// RxNorm Unit Tests (no database required)
// =============================================================================

func TestFormatNDC(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"11-digit standard", "12345678901", "12345-6789-01"},
		{"11-digit all zeros", "00000000000", "00000-0000-00"},
		{"11-digit mixed", "59762044401", "59762-0444-01"},
		{"10-digit returns as-is", "1234567890", "1234567890"},
		{"12-digit returns as-is", "123456789012", "123456789012"},
		{"empty string", "", ""},
		{"short string", "123", "123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatNDC(tt.input)
			if got != tt.expected {
				t.Errorf("formatNDC(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestValidateRxNormDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Test missing required file
	err := validateRxNormDirectory(tmpDir)
	if err == nil {
		t.Error("Expected error for empty directory")
	}

	// Test with RXNCONSO.RRF present
	consoPath := filepath.Join(tmpDir, "RXNCONSO.RRF")
	if err := os.WriteFile(consoPath, []byte("test"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	err = validateRxNormDirectory(tmpDir)
	if err != nil {
		t.Errorf("Expected nil error with RXNCONSO.RRF present, got: %v", err)
	}
}

func TestValidateRxNormDirectory_GzAccepted(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with RXNCONSO.RRF.gz present (gzipped version)
	consoGzPath := filepath.Join(tmpDir, "RXNCONSO.RRF.gz")
	if err := os.WriteFile(consoGzPath, []byte("gzip content"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := validateRxNormDirectory(tmpDir)
	if err != nil {
		t.Errorf("Expected nil error with RXNCONSO.RRF.gz present, got: %v", err)
	}
}

func TestRXNCONSOColumnConstants(t *testing.T) {
	// Verify column constants match RxNorm RRF spec
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{"RXCUI", RXNCONSOColRXCUI, 0},
		{"LAT", RXNCONSOColLAT, 1},
		{"RXAUI", RXNCONSOColRXAUI, 7},
		{"SAB", RXNCONSOColSAB, 11},
		{"TTY", RXNCONSOColTTY, 12},
		{"CODE", RXNCONSOColCODE, 13},
		{"STR", RXNCONSOColSTR, 14},
		{"SUPPRESS", RXNCONSOColSUPPRESS, 16},
		{"CVF", RXNCONSOColCVF, 17},
		{"Total columns", RXNCONSOColumns, 18},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("RXNCONSO %s = %d, want %d", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestRXNRELColumnConstants(t *testing.T) {
	// Verify RXNREL column constants
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{"RXCUI1", RXNRELColRXCUI1, 0},
		{"REL", RXNRELColREL, 3},
		{"RXCUI2", RXNRELColRXCUI2, 4},
		{"RELA", RXNRELColRELA, 7},
		{"SAB", RXNRELColSAB, 10},
		{"SUPPRESS", RXNRELColSUPPRESS, 14},
		{"Total columns", RXNRELColumns, 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("RXNREL %s = %d, want %d", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestRXNSATColumnConstants(t *testing.T) {
	// Verify RXNSAT column constants
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{"RXCUI", RXNSATColRXCUI, 0},
		{"RXAUI", RXNSATColRXAUI, 3},
		{"ATN", RXNSATColATN, 8},
		{"ATV", RXNSATColATV, 10},
		{"Total columns", RXNSATColumns, 13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("RXNSAT %s = %d, want %d", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestRxNormLoadOptions_Defaults(t *testing.T) {
	// Test that nil options get sensible defaults in the loader
	opts := &RxNormLoadOptions{}

	// Default values should be zero/false
	if opts.SkipSuppressed {
		t.Error("Default SkipSuppressed should be false")
	}
	if opts.SkipRelations {
		t.Error("Default SkipRelations should be false")
	}
	if opts.LoadNDC {
		t.Error("Default LoadNDC should be false")
	}
	if len(opts.FilterTTY) != 0 {
		t.Error("Default FilterTTY should be empty")
	}
}

func TestRxNormConcept_Fields(t *testing.T) {
	// Test the RxNormConcept struct can hold expected values
	c := &RxNormConcept{
		ID:        12345,
		RXCUI:     "198765",
		RXAUI:     "1234567",
		SAB:       "RXNORM",
		TTY:       "SCD",
		Code:      "198765",
		Str:       "Aspirin 325 MG Oral Tablet",
		Suppress:  "N",
		ReleaseID: 1,
	}

	if c.TTY != "SCD" {
		t.Errorf("TTY = %q, want SCD", c.TTY)
	}
	if c.SAB != "RXNORM" {
		t.Errorf("SAB = %q, want RXNORM", c.SAB)
	}
}
