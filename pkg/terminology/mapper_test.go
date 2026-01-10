package terminology

import (
	"strings"
	"testing"
)

func TestMapperLoadFromReader(t *testing.T) {
	csvData := `source_system,source_code,source_display,target_system,target_code,target_display,equivalence,comment
LOCAL_LAB,GLU,Glucose,http://loinc.org,2345-7,Glucose [Mass/volume] in Serum or Plasma,equivalent,Common glucose test
LOCAL_LAB,WBC,White Blood Cell Count,http://loinc.org,6690-2,Leukocytes [#/volume] in Blood,equivalent,CBC component
LOCAL_LAB,HGB,Hemoglobin,http://loinc.org,718-7,Hemoglobin [Mass/volume] in Blood,equivalent,CBC component
LOCAL_LAB,CREAT,Creatinine,http://loinc.org,2160-0,Creatinine [Mass/volume] in Serum or Plasma,equivalent,Kidney function`

	mapper := NewMapper()
	err := mapper.LoadFromReader(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("LoadFromReader failed: %v", err)
	}

	if mapper.Count() != 4 {
		t.Errorf("Count() = %d, want 4", mapper.Count())
	}
}

func TestMapperMap(t *testing.T) {
	csvData := `source_system,source_code,source_display,target_system,target_code,target_display,equivalence
LOCAL_LAB,GLU,Glucose,http://loinc.org,2345-7,Glucose [Mass/volume] in Serum or Plasma,equivalent
LOCAL_LAB,GLU,Glucose,http://snomed.info/sct,33747003,Glucose measurement,wider`

	mapper := NewMapper()
	if err := mapper.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("LoadFromReader failed: %v", err)
	}

	// Map to all targets
	mappings := mapper.Map("LOCAL_LAB", "GLU", "")
	if len(mappings) != 2 {
		t.Errorf("Map() returned %d mappings, want 2", len(mappings))
	}

	// Map to LOINC only
	loincMappings := mapper.Map("LOCAL_LAB", "GLU", SystemLOINC)
	if len(loincMappings) != 1 {
		t.Errorf("Map() to LOINC returned %d mappings, want 1", len(loincMappings))
	}
	if loincMappings[0].TargetCode != "2345-7" {
		t.Errorf("LOINC code = %q, want '2345-7'", loincMappings[0].TargetCode)
	}

	// Map to SNOMED
	snomedMappings := mapper.Map("LOCAL_LAB", "GLU", SystemSNOMED)
	if len(snomedMappings) != 1 {
		t.Errorf("Map() to SNOMED returned %d mappings, want 1", len(snomedMappings))
	}
	if snomedMappings[0].Equivalence != EquivalenceWider {
		t.Errorf("Equivalence = %q, want 'wider'", snomedMappings[0].Equivalence)
	}
}

func TestMapperMapToLOINC(t *testing.T) {
	csvData := `source_system,source_code,target_system,target_code,target_display
LOCAL_LAB,WBC,http://loinc.org,6690-2,Leukocytes [#/volume] in Blood`

	mapper := NewMapper()
	if err := mapper.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("LoadFromReader failed: %v", err)
	}

	// Found
	mapping := mapper.MapToLOINC("LOCAL_LAB", "WBC")
	if mapping == nil {
		t.Fatal("MapToLOINC() returned nil, want mapping")
	}
	if mapping.TargetCode != "6690-2" {
		t.Errorf("TargetCode = %q, want '6690-2'", mapping.TargetCode)
	}

	// Not found
	mapping = mapper.MapToLOINC("LOCAL_LAB", "UNKNOWN")
	if mapping != nil {
		t.Errorf("MapToLOINC(UNKNOWN) = %v, want nil", mapping)
	}
}

func TestMapperCaseInsensitive(t *testing.T) {
	csvData := `source_system,source_code,target_system,target_code
LOCAL_LAB,glu,http://loinc.org,2345-7`

	mapper := NewMapper()
	if err := mapper.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("LoadFromReader failed: %v", err)
	}

	// Should find regardless of case
	tests := []struct {
		system string
		code   string
	}{
		{"LOCAL_LAB", "GLU"},
		{"local_lab", "glu"},
		{"LOCAL_LAB", "glu"},
		{"Local_Lab", "Glu"},
	}

	for _, tt := range tests {
		t.Run(tt.system+":"+tt.code, func(t *testing.T) {
			if !mapper.HasMapping(tt.system, tt.code) {
				t.Errorf("HasMapping(%q, %q) = false, want true", tt.system, tt.code)
			}
		})
	}
}

func TestMapperMissingColumns(t *testing.T) {
	// Missing required column
	csvData := `source_system,source_code,target_system
LOCAL_LAB,GLU,http://loinc.org`

	mapper := NewMapper()
	err := mapper.LoadFromReader(strings.NewReader(csvData))
	if err == nil {
		t.Error("LoadFromReader should fail with missing target_code column")
	}
}

func TestParseEquivalence(t *testing.T) {
	tests := []struct {
		input string
		want  MappingEquivalence
	}{
		{"equivalent", EquivalenceEquivalent},
		{"Equivalent", EquivalenceEquivalent},
		{"equal", EquivalenceEquivalent},
		{"exact", EquivalenceEquivalent},
		{"wider", EquivalenceWider},
		{"broader", EquivalenceWider},
		{"narrower", EquivalenceNarrower},
		{"more specific", EquivalenceNarrower},
		{"inexact", EquivalenceInexact},
		{"approximate", EquivalenceInexact},
		{"unknown", EquivalenceEquivalent}, // Default
		{"", EquivalenceEquivalent},        // Default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseEquivalence(tt.input)
			if got != tt.want {
				t.Errorf("parseEquivalence(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()

	// Create and register a mapper
	csvData := `source_system,source_code,target_system,target_code
LOCAL_LAB,GLU,http://loinc.org,2345-7`

	mapper := NewMapper()
	if err := mapper.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("LoadFromReader failed: %v", err)
	}

	reg.RegisterMapper("LOCAL_LAB", SystemLOINC, mapper)

	// Get mapper
	retrieved := reg.GetMapper("LOCAL_LAB", SystemLOINC)
	if retrieved == nil {
		t.Fatal("GetMapper returned nil")
	}

	// Map through registry
	mappings := reg.Map("LOCAL_LAB", "GLU", SystemLOINC)
	if len(mappings) != 1 {
		t.Errorf("Registry.Map() returned %d mappings, want 1", len(mappings))
	}
}

func TestMapperEmptyRows(t *testing.T) {
	// CSV with empty rows should be skipped
	csvData := `source_system,source_code,target_system,target_code
LOCAL_LAB,GLU,http://loinc.org,2345-7

LOCAL_LAB,WBC,http://loinc.org,6690-2
,,,`

	mapper := NewMapper()
	if err := mapper.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("LoadFromReader failed: %v", err)
	}

	// Should only have 2 mappings (empty rows skipped)
	if mapper.Count() != 2 {
		t.Errorf("Count() = %d, want 2", mapper.Count())
	}
}

func TestMapperMapToSNOMED(t *testing.T) {
	csvData := `source_system,source_code,target_system,target_code,target_display,equivalence
LOCAL_LAB,DM2,http://snomed.info/sct,44054006,Diabetes mellitus type 2,equivalent
LOCAL_LAB,HTN,http://snomed.info/sct,38341003,Hypertensive disorder,equivalent
LOCAL_LAB,GLU,http://loinc.org,2345-7,Glucose,equivalent`

	mapper := NewMapper()
	if err := mapper.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("LoadFromReader failed: %v", err)
	}

	// Found - mapping to SNOMED
	mapping := mapper.MapToSNOMED("LOCAL_LAB", "DM2")
	if mapping == nil {
		t.Fatal("MapToSNOMED() returned nil, want mapping")
	}
	if mapping.TargetCode != "44054006" {
		t.Errorf("TargetCode = %q, want '44054006'", mapping.TargetCode)
	}
	if mapping.TargetSystem != SystemSNOMED {
		t.Errorf("TargetSystem = %q, want %q", mapping.TargetSystem, SystemSNOMED)
	}
	if mapping.TargetDisplay != "Diabetes mellitus type 2" {
		t.Errorf("TargetDisplay = %q, want 'Diabetes mellitus type 2'", mapping.TargetDisplay)
	}

	// Found - different code
	mapping = mapper.MapToSNOMED("LOCAL_LAB", "HTN")
	if mapping == nil {
		t.Fatal("MapToSNOMED(HTN) returned nil, want mapping")
	}
	if mapping.TargetCode != "38341003" {
		t.Errorf("TargetCode = %q, want '38341003'", mapping.TargetCode)
	}

	// Not found - code exists but targets LOINC, not SNOMED
	mapping = mapper.MapToSNOMED("LOCAL_LAB", "GLU")
	if mapping != nil {
		t.Errorf("MapToSNOMED(GLU) = %v, want nil (GLU maps to LOINC, not SNOMED)", mapping)
	}

	// Not found - code doesn't exist
	mapping = mapper.MapToSNOMED("LOCAL_LAB", "UNKNOWN")
	if mapping != nil {
		t.Errorf("MapToSNOMED(UNKNOWN) = %v, want nil", mapping)
	}
}

func TestMapperMapToICD10(t *testing.T) {
	csvData := `source_system,source_code,target_system,target_code,target_display,equivalence
LOCAL_DX,DM2,http://hl7.org/fhir/sid/icd-10-cm,E11.9,Type 2 diabetes mellitus without complications,equivalent
LOCAL_DX,HTN,http://hl7.org/fhir/sid/icd-10-cm,I10,Essential hypertension,equivalent`

	mapper := NewMapper()
	if err := mapper.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("LoadFromReader failed: %v", err)
	}

	// Test mapping to ICD-10
	mappings := mapper.Map("LOCAL_DX", "DM2", SystemICD10CM)
	if len(mappings) != 1 {
		t.Fatalf("Map() returned %d mappings, want 1", len(mappings))
	}
	if mappings[0].TargetCode != "E11.9" {
		t.Errorf("TargetCode = %q, want 'E11.9'", mappings[0].TargetCode)
	}
}
