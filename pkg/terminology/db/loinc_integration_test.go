//go:build integration

package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// =============================================================================
// LOINC Loader Integration Tests
// =============================================================================

func TestLOINCLoader_Integration_LoadLoincTable(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Initialize schema
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	// Get path to test data
	testDataPath := getTestDataPath(t, "LoincTable_sample.csv")

	// Create loader and load data
	loader := NewLOINCLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	result, err := loader.LoadLoincTable(ctx, testDataPath, "test-2.77", &releaseDate, nil)
	if err != nil {
		t.Fatalf("LoadLoincTable failed: %v", err)
	}

	// Verify results
	if result.ReleaseID <= 0 {
		t.Error("Expected positive release ID")
	}

	if result.CodesLoaded != 10 {
		t.Errorf("Expected 10 codes loaded, got %d", result.CodesLoaded)
	}

	if result.Version != "test-2.77" {
		t.Errorf("Expected version test-2.77, got %s", result.Version)
	}

	// Verify release was created
	release, err := migrator.GetActiveRelease(ctx, VocabLOINC)
	if err != nil {
		t.Fatalf("GetActiveRelease failed: %v", err)
	}
	if release == nil {
		t.Fatal("Expected release to exist")
	}
	if release.Version != "test-2.77" {
		t.Errorf("Expected release version test-2.77, got %s", release.Version)
	}
}

func TestLOINCLoader_Integration_LoadPanelHierarchy(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Initialize schema
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	// Load codes first
	testDataPath := getTestDataPath(t, "LoincTable_sample.csv")
	loader := NewLOINCLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	result, err := loader.LoadLoincTable(ctx, testDataPath, "test-2.77", &releaseDate, nil)
	if err != nil {
		t.Fatalf("LoadLoincTable failed: %v", err)
	}

	// Load panel hierarchy
	panelPath := getTestDataPath(t, "PanelHierarchy_sample.csv")
	panelsLoaded, err := loader.LoadPanelHierarchy(ctx, panelPath, result.ReleaseID, nil, "test-2.77")
	if err != nil {
		t.Fatalf("LoadPanelHierarchy failed: %v", err)
	}

	if panelsLoaded != 5 {
		t.Errorf("Expected 5 panel members loaded, got %d", panelsLoaded)
	}
}

func TestLOINCLoader_Integration_Reload(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Initialize schema
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	testDataPath := getTestDataPath(t, "LoincTable_sample.csv")
	loader := NewLOINCLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	// Load first time
	result1, err := loader.LoadLoincTable(ctx, testDataPath, "test-2.77", &releaseDate, nil)
	if err != nil {
		t.Fatalf("First load failed: %v", err)
	}

	// Load second time (reload)
	result2, err := loader.LoadLoincTable(ctx, testDataPath, "test-2.77", &releaseDate, nil)
	if err != nil {
		t.Fatalf("Second load (reload) failed: %v", err)
	}

	// Release ID should be the same
	if result1.ReleaseID != result2.ReleaseID {
		t.Errorf("Release ID changed on reload: %d -> %d", result1.ReleaseID, result2.ReleaseID)
	}

	// Should still have same number of codes
	if result2.CodesLoaded != 10 {
		t.Errorf("Expected 10 codes after reload, got %d", result2.CodesLoaded)
	}
}

// =============================================================================
// LOINC Query Integration Tests
// =============================================================================

func TestLOINCQueries_Integration_GetByCode(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Setup: Initialize and load data
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	migrator.Initialize(ctx)

	testDataPath := getTestDataPath(t, "LoincTable_sample.csv")
	loader := NewLOINCLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	loader.LoadLoincTable(ctx, testDataPath, "test-2.77", &releaseDate, nil)

	// Test queries
	queries := NewLOINCQueries(tc.DB)

	// Get glucose code
	code, err := queries.GetByCode(ctx, "2345-7")
	if err != nil {
		t.Fatalf("GetByCode failed: %v", err)
	}
	if code == nil {
		t.Fatal("Expected code to exist")
	}

	if code.LOINCNum != "2345-7" {
		t.Errorf("Expected LOINC_NUM 2345-7, got %s", code.LOINCNum)
	}
	if code.LongCommonName != "Glucose [Mass/volume] in Serum or Plasma" {
		t.Errorf("Unexpected long_common_name: %s", code.LongCommonName)
	}
	if code.DisplayName() != "Blood Sugar" {
		t.Errorf("Expected display name 'Blood Sugar', got %s", code.DisplayName())
	}

	// Get non-existent code
	code, err = queries.GetByCode(ctx, "99999-9")
	if err != nil {
		t.Fatalf("GetByCode for missing code failed: %v", err)
	}
	if code != nil {
		t.Error("Expected nil for non-existent code")
	}
}

func TestLOINCQueries_Integration_SearchByComponent(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Setup
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	migrator.Initialize(ctx)

	testDataPath := getTestDataPath(t, "LoincTable_sample.csv")
	loader := NewLOINCLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	loader.LoadLoincTable(ctx, testDataPath, "test-2.77", &releaseDate, nil)

	// Test search
	queries := NewLOINCQueries(tc.DB)

	// Search for "Hemoglobin" - should find 2 codes (718-7 and 4548-4)
	codes, err := queries.SearchByComponent(ctx, "Hemoglobin", 10)
	if err != nil {
		t.Fatalf("SearchByComponent failed: %v", err)
	}

	if len(codes) != 2 {
		t.Errorf("Expected 2 codes with 'Hemoglobin', got %d", len(codes))
	}

	// Search for "Glucose"
	codes, err = queries.SearchByComponent(ctx, "Glucose", 10)
	if err != nil {
		t.Fatalf("SearchByComponent failed: %v", err)
	}

	if len(codes) != 1 {
		t.Errorf("Expected 1 code with 'Glucose', got %d", len(codes))
	}
}

func TestLOINCQueries_Integration_SearchByName(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Setup
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	migrator.Initialize(ctx)

	testDataPath := getTestDataPath(t, "LoincTable_sample.csv")
	loader := NewLOINCLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	loader.LoadLoincTable(ctx, testDataPath, "test-2.77", &releaseDate, nil)

	queries := NewLOINCQueries(tc.DB)

	// Search by consumer name "Blood Sugar"
	codes, err := queries.SearchByName(ctx, "Blood Sugar", 10)
	if err != nil {
		t.Fatalf("SearchByName failed: %v", err)
	}

	if len(codes) != 1 {
		t.Errorf("Expected 1 code with 'Blood Sugar', got %d", len(codes))
	}
	if len(codes) > 0 && codes[0].LOINCNum != "2345-7" {
		t.Errorf("Expected LOINC_NUM 2345-7, got %s", codes[0].LOINCNum)
	}
}

func TestLOINCQueries_Integration_GetPanelMembers(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Setup
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	migrator.Initialize(ctx)

	testDataPath := getTestDataPath(t, "LoincTable_sample.csv")
	loader := NewLOINCLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	result, _ := loader.LoadLoincTable(ctx, testDataPath, "test-2.77", &releaseDate, nil)

	panelPath := getTestDataPath(t, "PanelHierarchy_sample.csv")
	loader.LoadPanelHierarchy(ctx, panelPath, result.ReleaseID, nil, "test-2.77")

	queries := NewLOINCQueries(tc.DB)

	// Get CBC panel members (58410-2)
	members, err := queries.GetPanelMembers(ctx, "58410-2")
	if err != nil {
		t.Fatalf("GetPanelMembers failed: %v", err)
	}

	if len(members) != 5 {
		t.Errorf("Expected 5 CBC panel members, got %d", len(members))
	}

	// Verify members are in correct order by sequence
	expectedCodes := []string{"6690-2", "789-8", "718-7", "4544-3", "777-3"}
	for i, member := range members {
		if i < len(expectedCodes) && member.LOINCNum != expectedCodes[i] {
			t.Errorf("Member %d: expected %s, got %s", i, expectedCodes[i], member.LOINCNum)
		}
	}
}

func TestLOINCQueries_Integration_GetParentPanels(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Setup
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	migrator.Initialize(ctx)

	testDataPath := getTestDataPath(t, "LoincTable_sample.csv")
	loader := NewLOINCLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	result, _ := loader.LoadLoincTable(ctx, testDataPath, "test-2.77", &releaseDate, nil)

	panelPath := getTestDataPath(t, "PanelHierarchy_sample.csv")
	loader.LoadPanelHierarchy(ctx, panelPath, result.ReleaseID, nil, "test-2.77")

	queries := NewLOINCQueries(tc.DB)

	// Get parent panels for WBC (6690-2)
	parents, err := queries.GetParentPanels(ctx, "6690-2")
	if err != nil {
		t.Fatalf("GetParentPanels failed: %v", err)
	}

	if len(parents) != 1 {
		t.Errorf("Expected 1 parent panel for WBC, got %d", len(parents))
	}
	if len(parents) > 0 && parents[0].LOINCNum != "58410-2" {
		t.Errorf("Expected parent 58410-2, got %s", parents[0].LOINCNum)
	}
}

func TestLOINCQueries_Integration_IsPanel(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Setup
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	migrator.Initialize(ctx)

	testDataPath := getTestDataPath(t, "LoincTable_sample.csv")
	loader := NewLOINCLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	result, _ := loader.LoadLoincTable(ctx, testDataPath, "test-2.77", &releaseDate, nil)

	panelPath := getTestDataPath(t, "PanelHierarchy_sample.csv")
	loader.LoadPanelHierarchy(ctx, panelPath, result.ReleaseID, nil, "test-2.77")

	queries := NewLOINCQueries(tc.DB)

	// CBC (58410-2) should be a panel
	isPanel, err := queries.IsPanel(ctx, "58410-2")
	if err != nil {
		t.Fatalf("IsPanel failed: %v", err)
	}
	if !isPanel {
		t.Error("Expected 58410-2 to be a panel")
	}

	// Glucose (2345-7) should not be a panel
	isPanel, err = queries.IsPanel(ctx, "2345-7")
	if err != nil {
		t.Fatalf("IsPanel failed: %v", err)
	}
	if isPanel {
		t.Error("Expected 2345-7 to NOT be a panel")
	}
}

func TestLOINCQueries_Integration_Count(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Setup
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	migrator.Initialize(ctx)

	testDataPath := getTestDataPath(t, "LoincTable_sample.csv")
	loader := NewLOINCLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	loader.LoadLoincTable(ctx, testDataPath, "test-2.77", &releaseDate, nil)

	queries := NewLOINCQueries(tc.DB)

	count, err := queries.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}

	if count != 10 {
		t.Errorf("Expected 10 active codes, got %d", count)
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func getTestDataPath(t *testing.T, filename string) string {
	t.Helper()

	// Try relative path from working directory
	paths := []string{
		filepath.Join("testdata", "terminology", filename),
		filepath.Join("..", "..", "..", "testdata", "terminology", filename),
	}

	// Also try with GOPATH
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		paths = append(paths, filepath.Join(gopath, "src", "github.com", "crb2nu", "fi-fhir", "testdata", "terminology", filename))
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			abs, _ := filepath.Abs(path)
			return abs
		}
	}

	t.Fatalf("Could not find test data file: %s", filename)
	return ""
}
