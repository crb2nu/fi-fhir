package terminology

import (
	"strings"
	"testing"
)

// =============================================================================
// Test Data - Simulating LOINC CSV format
// =============================================================================

const testLoincTable = `LOINC_NUM,LONG_COMMON_NAME,SHORTNAME,STATUS,COMPONENT,PROPERTY,TIME_ASPCT,SYSTEM,SCALE_TYP,METHOD_TYP,CLASS,CLASSTYPE,EXAMPLE_UNITS,ORDER_OBS,CONSUMER_NAME
6690-2,Leukocytes [#/volume] in Blood by Automated count,WBC Auto,ACTIVE,Leukocytes,NCnc,Pt,Bld,Qn,Automated count,HEMATOLOGY,1,10*3/uL,Both,White blood cell count
718-7,Hemoglobin [Mass/volume] in Blood,Hgb Bld,ACTIVE,Hemoglobin,MCnc,Pt,Bld,Qn,,HEMATOLOGY,1,g/dL,Both,Hemoglobin
789-8,Erythrocytes [#/volume] in Blood by Automated count,RBC Auto,ACTIVE,Erythrocytes,NCnc,Pt,Bld,Qn,Automated count,HEMATOLOGY,1,10*6/uL,Both,Red blood cell count
777-3,Platelets [#/volume] in Blood by Automated count,Platelet Auto,ACTIVE,Platelets,NCnc,Pt,Bld,Qn,Automated count,HEMATOLOGY,1,10*3/uL,Both,Platelet count
2345-7,Glucose [Mass/volume] in Serum or Plasma,Glucose SerPl,ACTIVE,Glucose,MCnc,Pt,Ser/Plas,Qn,,CHEMISTRY,1,mg/dL,Both,Glucose
2160-0,Creatinine [Mass/volume] in Serum or Plasma,Creat SerPl,ACTIVE,Creatinine,MCnc,Pt,Ser/Plas,Qn,,CHEMISTRY,1,mg/dL,Both,Creatinine
3094-0,Urea nitrogen [Mass/volume] in Serum or Plasma,BUN SerPl,ACTIVE,Urea nitrogen,MCnc,Pt,Ser/Plas,Qn,,CHEMISTRY,1,mg/dL,Both,Blood urea nitrogen
58410-2,CBC panel - Blood by Automated count,CBC Auto Bld,ACTIVE,CBC panel,Panel,Pt,Bld,-,Automated count,HEMATOLOGY,1,,Order,Complete blood count panel
51990-0,Basic metabolic panel - Blood,BMP Bld,ACTIVE,Basic metabolic panel,Panel,Pt,Bld,-,,CHEMISTRY,1,,Order,Basic metabolic panel
OLD-123,Deprecated test,Old Test,DEPRECATED,OldComponent,MCnc,Pt,Bld,Qn,,HEMATOLOGY,1,mg/dL,Both,`

const testPanelHierarchy = `PARENTLOINC,LOINC,CARDINALITY
58410-2,6690-2,R
58410-2,718-7,R
58410-2,789-8,R
58410-2,777-3,R
51990-0,2345-7,R
51990-0,2160-0,R
51990-0,3094-0,R`

// =============================================================================
// LOINCLoader Tests
// =============================================================================

func TestLOINCLoader_LoadLoincTable(t *testing.T) {
	loader := NewLOINCLoader()
	err := loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))
	if err != nil {
		t.Fatalf("LoadLoincTableFromReader failed: %v", err)
	}

	// Check count (should be 10 codes)
	if loader.Count() != 10 {
		t.Errorf("Count = %d, want 10", loader.Count())
	}
}

func TestLOINCLoader_GetCode(t *testing.T) {
	loader := NewLOINCLoader()
	loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))

	tests := []struct {
		code        string
		wantCode    string
		wantStatus  string
		wantDisplay string
	}{
		{"6690-2", "6690-2", "ACTIVE", "White blood cell count"},
		{"718-7", "718-7", "ACTIVE", "Hemoglobin"},
		{"2345-7", "2345-7", "ACTIVE", "Glucose"},
		{"OLD-123", "OLD-123", "DEPRECATED", ""},
	}

	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			code := loader.GetCode(tc.code)
			if code == nil {
				t.Fatalf("GetCode(%s) returned nil", tc.code)
			}
			if code.Code != tc.wantCode {
				t.Errorf("Code = %s, want %s", code.Code, tc.wantCode)
			}
			if code.Status != tc.wantStatus {
				t.Errorf("Status = %s, want %s", code.Status, tc.wantStatus)
			}
			if tc.wantDisplay != "" && code.DisplayName() != tc.wantDisplay {
				t.Errorf("DisplayName = %s, want %s", code.DisplayName(), tc.wantDisplay)
			}
		})
	}
}

func TestLOINCLoader_GetCode_NotFound(t *testing.T) {
	loader := NewLOINCLoader()
	loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))

	code := loader.GetCode("NONEXISTENT")
	if code != nil {
		t.Error("Expected nil for nonexistent code")
	}
}

func TestLOINCLoader_LookupByCode(t *testing.T) {
	loader := NewLOINCLoader()
	loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))

	// LookupByCode is an alias for GetCode - verify it works the same
	tests := []struct {
		code        string
		wantCode    string
		wantStatus  string
		wantDisplay string
	}{
		{"6690-2", "6690-2", "ACTIVE", "White blood cell count"},
		{"718-7", "718-7", "ACTIVE", "Hemoglobin"},
		{"2345-7", "2345-7", "ACTIVE", "Glucose"},
	}

	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			code := loader.LookupByCode(tc.code)
			if code == nil {
				t.Fatalf("LookupByCode(%s) = nil, want code", tc.code)
			}
			if code.Code != tc.wantCode {
				t.Errorf("Code = %s, want %s", code.Code, tc.wantCode)
			}
			if code.Status != tc.wantStatus {
				t.Errorf("Status = %s, want %s", code.Status, tc.wantStatus)
			}
			if code.DisplayName() != tc.wantDisplay {
				t.Errorf("DisplayName() = %s, want %s", code.DisplayName(), tc.wantDisplay)
			}
		})
	}

	// Test not found case
	code := loader.LookupByCode("NONEXISTENT")
	if code != nil {
		t.Error("LookupByCode(NONEXISTENT) should return nil")
	}
}

func TestLOINCLoader_LookupByDisplay(t *testing.T) {
	loader := NewLOINCLoader()
	loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))

	tests := []struct {
		display    string
		wantCodes  []string
		wantMinLen int
	}{
		{"LEUKOCYTES", []string{"6690-2"}, 1},
		{"HEMOGLOBIN", []string{"718-7"}, 1},
		{"GLUCOSE", []string{"2345-7"}, 1},
		{"WBC Auto", []string{"6690-2"}, 1}, // Short name match
	}

	for _, tc := range tests {
		t.Run(tc.display, func(t *testing.T) {
			results := loader.LookupByDisplay(tc.display)
			if len(results) < tc.wantMinLen {
				t.Errorf("LookupByDisplay(%s) returned %d results, want at least %d",
					tc.display, len(results), tc.wantMinLen)
			}

			// Check that expected codes are in results
			for _, wantCode := range tc.wantCodes {
				found := false
				for _, r := range results {
					if r.Code == wantCode {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("LookupByDisplay(%s) missing expected code %s", tc.display, wantCode)
				}
			}
		})
	}
}

func TestLOINCLoader_SearchCodes(t *testing.T) {
	loader := NewLOINCLoader()
	loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))

	results := loader.SearchCodes("blood", 10)
	if len(results) == 0 {
		t.Error("SearchCodes('blood') returned no results")
	}

	// All results should be active
	for _, r := range results {
		if !r.IsActive() {
			t.Errorf("SearchCodes returned inactive code: %s", r.Code)
		}
	}

	// Test with limit
	results = loader.SearchCodes("blood", 2)
	if len(results) > 2 {
		t.Errorf("SearchCodes with limit 2 returned %d results", len(results))
	}
}

func TestLOINCLoader_LoadPanelHierarchy(t *testing.T) {
	loader := NewLOINCLoader()
	loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))
	err := loader.LoadPanelHierarchyFromReader(strings.NewReader(testPanelHierarchy))
	if err != nil {
		t.Fatalf("LoadPanelHierarchyFromReader failed: %v", err)
	}

	if loader.PanelCount() != 2 {
		t.Errorf("PanelCount = %d, want 2", loader.PanelCount())
	}
}

func TestLOINCLoader_ExpandPanel(t *testing.T) {
	loader := NewLOINCLoader()
	loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))
	loader.LoadPanelHierarchyFromReader(strings.NewReader(testPanelHierarchy))

	// Expand CBC panel
	cbcMembers := loader.ExpandPanel("58410-2")
	if len(cbcMembers) != 4 {
		t.Errorf("ExpandPanel(CBC) returned %d members, want 4", len(cbcMembers))
	}

	// Check for expected members
	expectedCodes := map[string]bool{
		"6690-2": false, // WBC
		"718-7":  false, // Hgb
		"789-8":  false, // RBC
		"777-3":  false, // Platelets
	}
	for _, member := range cbcMembers {
		if _, ok := expectedCodes[member.Code]; ok {
			expectedCodes[member.Code] = true
		}
	}
	for code, found := range expectedCodes {
		if !found {
			t.Errorf("ExpandPanel(CBC) missing expected code %s", code)
		}
	}

	// Expand BMP panel
	bmpMembers := loader.ExpandPanel("51990-0")
	if len(bmpMembers) != 3 {
		t.Errorf("ExpandPanel(BMP) returned %d members, want 3", len(bmpMembers))
	}
}

func TestLOINCLoader_GetPanelMembers(t *testing.T) {
	loader := NewLOINCLoader()
	loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))
	loader.LoadPanelHierarchyFromReader(strings.NewReader(testPanelHierarchy))

	members := loader.GetPanelMembers("58410-2")
	if len(members) != 4 {
		t.Errorf("GetPanelMembers(CBC) returned %d members, want 4", len(members))
	}

	// Check cardinality
	for _, m := range members {
		if m.Cardinality != "R" {
			t.Errorf("Member %s cardinality = %s, want R", m.Code, m.Cardinality)
		}
	}
}

func TestLOINCLoader_GetParentPanels(t *testing.T) {
	loader := NewLOINCLoader()
	loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))
	loader.LoadPanelHierarchyFromReader(strings.NewReader(testPanelHierarchy))

	// WBC should be in CBC panel
	parents := loader.GetParentPanels("6690-2")
	if len(parents) != 1 {
		t.Errorf("GetParentPanels(WBC) returned %d parents, want 1", len(parents))
	}
	if len(parents) > 0 && parents[0] != "58410-2" {
		t.Errorf("GetParentPanels(WBC)[0] = %s, want 58410-2", parents[0])
	}

	// Glucose should be in BMP panel
	parents = loader.GetParentPanels("2345-7")
	if len(parents) != 1 || parents[0] != "51990-0" {
		t.Errorf("GetParentPanels(Glucose) = %v, want [51990-0]", parents)
	}
}

func TestLOINCLoader_IsPanel(t *testing.T) {
	loader := NewLOINCLoader()
	loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))
	loader.LoadPanelHierarchyFromReader(strings.NewReader(testPanelHierarchy))

	if !loader.IsPanel("58410-2") {
		t.Error("IsPanel(CBC) should return true")
	}
	if !loader.IsPanel("51990-0") {
		t.Error("IsPanel(BMP) should return true")
	}
	if loader.IsPanel("6690-2") {
		t.Error("IsPanel(WBC) should return false")
	}
}

func TestLOINCLoader_Version(t *testing.T) {
	loader := NewLOINCLoader()
	loader.SetVersion("2.76")

	if loader.Version() != "2.76" {
		t.Errorf("Version = %s, want 2.76", loader.Version())
	}
}

// =============================================================================
// LOINCCode Tests
// =============================================================================

func TestLOINCCode_IsActive(t *testing.T) {
	active := &LOINCCode{Status: "ACTIVE"}
	if !active.IsActive() {
		t.Error("ACTIVE status should return true")
	}

	deprecated := &LOINCCode{Status: "DEPRECATED"}
	if deprecated.IsActive() {
		t.Error("DEPRECATED status should return false")
	}
}

func TestLOINCCode_IsLab(t *testing.T) {
	lab := &LOINCCode{ClassType: "1"}
	if !lab.IsLab() {
		t.Error("ClassType 1 should be lab")
	}

	clinical := &LOINCCode{ClassType: "2"}
	if clinical.IsLab() {
		t.Error("ClassType 2 should not be lab")
	}
}

func TestLOINCCode_DisplayName(t *testing.T) {
	tests := []struct {
		name     string
		code     LOINCCode
		expected string
	}{
		{
			name:     "consumer name first",
			code:     LOINCCode{Consumer: "WBC", ShortName: "WBC Auto", LongName: "Leukocytes"},
			expected: "WBC",
		},
		{
			name:     "short name second",
			code:     LOINCCode{ShortName: "WBC Auto", LongName: "Leukocytes"},
			expected: "WBC Auto",
		},
		{
			name:     "long name last",
			code:     LOINCCode{LongName: "Leukocytes"},
			expected: "Leukocytes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.code.DisplayName() != tc.expected {
				t.Errorf("DisplayName = %s, want %s", tc.code.DisplayName(), tc.expected)
			}
		})
	}
}

func TestLOINCCode_ToCodeMapping(t *testing.T) {
	code := &LOINCCode{
		Code:      "6690-2",
		LongName:  "Leukocytes [#/volume] in Blood",
		ShortName: "WBC Auto",
		Consumer:  "White blood cell count",
	}

	mapping := code.ToCodeMapping("LOCAL_LAB", "WBC")

	if mapping.SourceSystem != "LOCAL_LAB" {
		t.Errorf("SourceSystem = %s, want LOCAL_LAB", mapping.SourceSystem)
	}
	if mapping.SourceCode != "WBC" {
		t.Errorf("SourceCode = %s, want WBC", mapping.SourceCode)
	}
	if mapping.TargetSystem != SystemLOINC {
		t.Errorf("TargetSystem = %s, want %s", mapping.TargetSystem, SystemLOINC)
	}
	if mapping.TargetCode != "6690-2" {
		t.Errorf("TargetCode = %s, want 6690-2", mapping.TargetCode)
	}
	if mapping.TargetDisplay != "White blood cell count" {
		t.Errorf("TargetDisplay = %s, want White blood cell count", mapping.TargetDisplay)
	}
}

// =============================================================================
// Common Code Maps Tests
// =============================================================================

func TestGetCommonPanelCode(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"CBC", "58410-2"},
		{"cbc", "58410-2"}, // Case insensitive
		{"BMP", "51990-0"},
		{"CMP", "24323-8"},
		{"LIPID", "57698-3"},
		{"NONEXISTENT", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code := GetCommonPanelCode(tc.name)
			if code != tc.expected {
				t.Errorf("GetCommonPanelCode(%s) = %s, want %s", tc.name, code, tc.expected)
			}
		})
	}
}

func TestGetCommonLabCode(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"WBC", "6690-2"},
		{"wbc", "6690-2"}, // Case insensitive
		{"GLUCOSE", "2345-7"},
		{"HGB", "718-7"},
		{"CREATININE", "2160-0"},
		{"TROPONIN_I", "10839-9"},
		{"NONEXISTENT", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code := GetCommonLabCode(tc.name)
			if code != tc.expected {
				t.Errorf("GetCommonLabCode(%s) = %s, want %s", tc.name, code, tc.expected)
			}
		})
	}
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestLOINCLoader_EmptyLoader(t *testing.T) {
	loader := NewLOINCLoader()

	if loader.Count() != 0 {
		t.Errorf("Empty loader Count = %d, want 0", loader.Count())
	}
	if loader.PanelCount() != 0 {
		t.Errorf("Empty loader PanelCount = %d, want 0", loader.PanelCount())
	}
	if loader.GetCode("ANY") != nil {
		t.Error("Empty loader GetCode should return nil")
	}
	if len(loader.ExpandPanel("ANY")) != 0 {
		t.Error("Empty loader ExpandPanel should return empty")
	}
}

func TestLOINCLoader_MissingColumns(t *testing.T) {
	// Missing required column
	badCSV := `CODE,NAME
123,Test`

	loader := NewLOINCLoader()
	err := loader.LoadLoincTableFromReader(strings.NewReader(badCSV))
	if err == nil {
		t.Error("Expected error for missing required columns")
	}
}

func TestLOINCLoader_Concurrent(t *testing.T) {
	loader := NewLOINCLoader()
	loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))
	loader.LoadPanelHierarchyFromReader(strings.NewReader(testPanelHierarchy))

	// Concurrent reads should be safe
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				loader.GetCode("6690-2")
				loader.LookupByDisplay("WBC")
				loader.SearchCodes("blood", 5)
				loader.ExpandPanel("58410-2")
				loader.Count()
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
