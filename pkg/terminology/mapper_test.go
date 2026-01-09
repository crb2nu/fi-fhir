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
