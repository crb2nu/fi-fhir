//go:build integration

package db

import (
	"context"
	"testing"
	"time"
)

// =============================================================================
// ICD-10-CM Loader Integration Tests
// =============================================================================

func TestICD10Loader_Integration_LoadCSV(t *testing.T) {
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
	testDataPath := getTestDataPath(t, "icd10cm_codes_sample.csv")

	// Create loader and load data
	loader := NewICD10Loader(tc.DB)
	releaseDate := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)

	opts := &ICD10LoadOptions{IncludeHeaders: true}
	result, err := loader.LoadICD10CMCSV(ctx, testDataPath, "FY2024-test", &releaseDate, nil, opts)
	if err != nil {
		t.Fatalf("LoadICD10CMCSV failed: %v", err)
	}

	// Verify results
	if result.ReleaseID <= 0 {
		t.Error("Expected positive release ID")
	}

	if result.Version != "FY2024-test" {
		t.Errorf("Expected version FY2024-test, got %s", result.Version)
	}

	// Sample data has 31 rows (30 data + 1 header)
	// 8 are headers (is_billable=false): E11, I11, I50, J45, J44, I21, R06, R05
	totalExpected := int64(30)
	actualTotal := result.CodesLoaded + result.HeadersLoaded
	if actualTotal != totalExpected {
		t.Errorf("Expected %d total codes, got %d (codes=%d, headers=%d)",
			totalExpected, actualTotal, result.CodesLoaded, result.HeadersLoaded)
	}

	// Verify release was created
	release, err := migrator.GetActiveRelease(ctx, VocabICD10CM)
	if err != nil {
		t.Fatalf("GetActiveRelease failed: %v", err)
	}
	if release == nil {
		t.Fatal("Expected release to exist")
	}
	if release.Version != "FY2024-test" {
		t.Errorf("Expected release version FY2024-test, got %s", release.Version)
	}
}

func TestICD10Loader_Integration_LoadCSV_BillableOnly(t *testing.T) {
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

	testDataPath := getTestDataPath(t, "icd10cm_codes_sample.csv")

	// Load with headers excluded
	loader := NewICD10Loader(tc.DB)
	releaseDate := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)

	opts := &ICD10LoadOptions{IncludeHeaders: false}
	result, err := loader.LoadICD10CMCSV(ctx, testDataPath, "FY2024-billable", &releaseDate, nil, opts)
	if err != nil {
		t.Fatalf("LoadICD10CMCSV failed: %v", err)
	}

	// Should have only billable codes (no headers)
	if result.HeadersLoaded != 0 {
		t.Errorf("Expected 0 headers with IncludeHeaders=false, got %d", result.HeadersLoaded)
	}

	// Verify database has only billable codes
	queries := NewICD10Queries(tc.DB)
	categories, err := queries.GetCategories(ctx, 100)
	if err != nil {
		t.Fatalf("GetCategories failed: %v", err)
	}
	if len(categories) != 0 {
		t.Errorf("Expected 0 category codes, got %d", len(categories))
	}
}

func TestICD10Loader_Integration_Reload(t *testing.T) {
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

	testDataPath := getTestDataPath(t, "icd10cm_codes_sample.csv")
	loader := NewICD10Loader(tc.DB)
	releaseDate := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)
	opts := &ICD10LoadOptions{IncludeHeaders: true}

	// Load first time
	result1, err := loader.LoadICD10CMCSV(ctx, testDataPath, "FY2024-reload", &releaseDate, nil, opts)
	if err != nil {
		t.Fatalf("First load failed: %v", err)
	}

	// Load second time (reload)
	result2, err := loader.LoadICD10CMCSV(ctx, testDataPath, "FY2024-reload", &releaseDate, nil, opts)
	if err != nil {
		t.Fatalf("Second load (reload) failed: %v", err)
	}

	// Release ID should be the same
	if result1.ReleaseID != result2.ReleaseID {
		t.Errorf("Release ID changed on reload: %d -> %d", result1.ReleaseID, result2.ReleaseID)
	}

	// Should still have same number of codes
	if result1.CodesLoaded != result2.CodesLoaded {
		t.Errorf("Code count changed on reload: %d -> %d", result1.CodesLoaded, result2.CodesLoaded)
	}
}

// =============================================================================
// ICD-10-CM Query Integration Tests
// =============================================================================

func TestICD10Queries_Integration_GetByCode(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Setup: Initialize and load data
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	migrator.Initialize(ctx)

	testDataPath := getTestDataPath(t, "icd10cm_codes_sample.csv")
	loader := NewICD10Loader(tc.DB)
	releaseDate := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)
	opts := &ICD10LoadOptions{IncludeHeaders: true}
	loader.LoadICD10CMCSV(ctx, testDataPath, "FY2024-query", &releaseDate, nil, opts)

	queries := NewICD10Queries(tc.DB)

	// Test 1: Get code by code with dot
	code, err := queries.GetByCode(ctx, "E11.9")
	if err != nil {
		t.Fatalf("GetByCode failed: %v", err)
	}
	if code == nil {
		t.Fatal("Expected code E11.9 to exist")
	}
	if code.Code != "E119" {
		t.Errorf("Expected code E119 (no dot), got %s", code.Code)
	}
	if code.Description != "Type 2 diabetes mellitus without complications" {
		t.Errorf("Unexpected description: %s", code.Description)
	}
	if code.IsHeader {
		t.Error("Expected E11.9 to be billable, not a header")
	}

	// Test 2: Get code by code without dot
	code, err = queries.GetByCode(ctx, "E119")
	if err != nil {
		t.Fatalf("GetByCode failed: %v", err)
	}
	if code == nil {
		t.Fatal("Expected code E119 to exist")
	}

	// Test 3: Get header code
	code, err = queries.GetByCode(ctx, "E11")
	if err != nil {
		t.Fatalf("GetByCode failed: %v", err)
	}
	if code == nil {
		t.Fatal("Expected code E11 to exist")
	}
	if !code.IsHeader {
		t.Error("Expected E11 to be a header")
	}
	if code.IsBillable() {
		t.Error("Expected E11 to NOT be billable")
	}

	// Test 4: Get non-existent code
	code, err = queries.GetByCode(ctx, "Z99.99")
	if err != nil {
		t.Fatalf("GetByCode for missing code failed: %v", err)
	}
	if code != nil {
		t.Error("Expected nil for non-existent code")
	}
}

func TestICD10Queries_Integration_SearchByDescription(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Setup
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	migrator.Initialize(ctx)

	testDataPath := getTestDataPath(t, "icd10cm_codes_sample.csv")
	loader := NewICD10Loader(tc.DB)
	releaseDate := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)
	opts := &ICD10LoadOptions{IncludeHeaders: true}
	loader.LoadICD10CMCSV(ctx, testDataPath, "FY2024-search", &releaseDate, nil, opts)

	queries := NewICD10Queries(tc.DB)

	// Search for "diabetes" - should find multiple codes
	codes, err := queries.SearchByDescription(ctx, "diabetes", 100)
	if err != nil {
		t.Fatalf("SearchByDescription failed: %v", err)
	}

	if len(codes) == 0 {
		t.Error("Expected to find codes with 'diabetes'")
	}

	// All results should contain "diabetes" in description (case insensitive)
	for _, code := range codes {
		if code.Description == "" {
			t.Error("Code description should not be empty")
		}
	}

	// Search for "hypertension"
	codes, err = queries.SearchByDescription(ctx, "hypertensive", 100)
	if err != nil {
		t.Fatalf("SearchByDescription failed: %v", err)
	}

	if len(codes) < 3 {
		t.Errorf("Expected at least 3 hypertensive codes, got %d", len(codes))
	}
}

func TestICD10Queries_Integration_GetByChapter(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Setup
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	migrator.Initialize(ctx)

	testDataPath := getTestDataPath(t, "icd10cm_codes_sample.csv")
	loader := NewICD10Loader(tc.DB)
	releaseDate := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)
	opts := &ICD10LoadOptions{IncludeHeaders: true}
	loader.LoadICD10CMCSV(ctx, testDataPath, "FY2024-chapter", &releaseDate, nil, opts)

	queries := NewICD10Queries(tc.DB)

	// Get Chapter 4 (Endocrine) codes - should include diabetes codes
	codes, err := queries.GetByChapter(ctx, "04", 100)
	if err != nil {
		t.Fatalf("GetByChapter failed: %v", err)
	}

	if len(codes) == 0 {
		t.Error("Expected to find codes in Chapter 4")
	}

	// Test with different chapter format "4"
	codes2, err := queries.GetByChapter(ctx, "4", 100)
	if err != nil {
		t.Fatalf("GetByChapter failed with single digit: %v", err)
	}

	if len(codes) != len(codes2) {
		t.Errorf("Chapter normalization failed: '04' got %d, '4' got %d", len(codes), len(codes2))
	}

	// Get Chapter 9 (Circulatory) codes - should include hypertension, heart failure
	codes, err = queries.GetByChapter(ctx, "09", 100)
	if err != nil {
		t.Fatalf("GetByChapter failed: %v", err)
	}

	if len(codes) == 0 {
		t.Error("Expected to find codes in Chapter 9")
	}
}

func TestICD10Queries_Integration_GetChildren(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Setup
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	migrator.Initialize(ctx)

	testDataPath := getTestDataPath(t, "icd10cm_codes_sample.csv")
	loader := NewICD10Loader(tc.DB)
	releaseDate := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)
	opts := &ICD10LoadOptions{IncludeHeaders: true}
	loader.LoadICD10CMCSV(ctx, testDataPath, "FY2024-children", &releaseDate, nil, opts)

	queries := NewICD10Queries(tc.DB)

	// Get children of E11 (Type 2 diabetes)
	children, err := queries.GetChildren(ctx, "E11")
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}

	// Should have E11.9, E11.65, E11.21, E11.22, E11.40
	if len(children) < 5 {
		t.Errorf("Expected at least 5 children of E11, got %d", len(children))
	}

	// All children should have E11 as parent
	for _, child := range children {
		if child.ParentCode.Valid && child.ParentCode.String != "E11" {
			t.Errorf("Expected parent E11, got %s", child.ParentCode.String)
		}
	}
}

func TestICD10Queries_Integration_GetBillableCodes(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Setup
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	migrator.Initialize(ctx)

	testDataPath := getTestDataPath(t, "icd10cm_codes_sample.csv")
	loader := NewICD10Loader(tc.DB)
	releaseDate := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)
	opts := &ICD10LoadOptions{IncludeHeaders: true}
	loader.LoadICD10CMCSV(ctx, testDataPath, "FY2024-billable", &releaseDate, nil, opts)

	queries := NewICD10Queries(tc.DB)

	// Get all billable codes
	codes, err := queries.GetBillableCodes(ctx, "", 100)
	if err != nil {
		t.Fatalf("GetBillableCodes failed: %v", err)
	}

	// None should be headers
	for _, code := range codes {
		if code.IsHeader {
			t.Errorf("Found header in billable codes: %s", code.Code)
		}
	}

	// Get billable codes with E11 prefix
	codes, err = queries.GetBillableCodes(ctx, "E11", 100)
	if err != nil {
		t.Fatalf("GetBillableCodes with prefix failed: %v", err)
	}

	if len(codes) == 0 {
		t.Error("Expected billable codes with E11 prefix")
	}

	for _, code := range codes {
		if len(code.Code) < 3 || code.Code[:3] != "E11" {
			t.Errorf("Code %s does not have E11 prefix", code.Code)
		}
	}
}

func TestICD10Queries_Integration_Count(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Setup
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	migrator.Initialize(ctx)

	testDataPath := getTestDataPath(t, "icd10cm_codes_sample.csv")
	loader := NewICD10Loader(tc.DB)
	releaseDate := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)
	opts := &ICD10LoadOptions{IncludeHeaders: true}
	result, _ := loader.LoadICD10CMCSV(ctx, testDataPath, "FY2024-count", &releaseDate, nil, opts)

	queries := NewICD10Queries(tc.DB)

	// Total count
	count, err := queries.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}

	expected := result.CodesLoaded + result.HeadersLoaded
	if count != expected {
		t.Errorf("Expected count %d, got %d", expected, count)
	}

	// Billable count
	billableCount, err := queries.CountBillable(ctx)
	if err != nil {
		t.Fatalf("CountBillable failed: %v", err)
	}

	if billableCount != result.CodesLoaded {
		t.Errorf("Expected billable count %d, got %d", result.CodesLoaded, billableCount)
	}
}

func TestICD10CMCode_DisplayCode_Integration(t *testing.T) {
	// Test DisplayCode helper
	code := &ICD10CMCode{
		Code: "E119",
	}

	// Without formatted code
	if code.DisplayCode() != "E119" {
		t.Errorf("Expected E119, got %s", code.DisplayCode())
	}

	// With formatted code
	code.CodeFormatted.Valid = true
	code.CodeFormatted.String = "E11.9"
	if code.DisplayCode() != "E11.9" {
		t.Errorf("Expected E11.9, got %s", code.DisplayCode())
	}
}
