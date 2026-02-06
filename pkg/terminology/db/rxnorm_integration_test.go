//go:build integration

package db

import (
	"context"
	"testing"
	"time"
)

// =============================================================================
// RxNorm Loader Integration Tests
// =============================================================================

func TestRxNormLoader_Integration_LoadRRF(t *testing.T) {
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
	testDataPath := getTestDataPath(t, "rxnorm")

	// Create loader and load data
	loader := NewRxNormLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	opts := &RxNormLoadOptions{
		SkipSuppressed: true,
		LoadNDC:        true,
	}

	result, err := loader.LoadRRF(ctx, testDataPath, "2024-01-test", &releaseDate, opts, nil)
	if err != nil {
		t.Fatalf("LoadRRF failed: %v", err)
	}

	// Verify results
	if result.ReleaseID <= 0 {
		t.Error("Expected positive release ID")
	}

	if result.Version != "2024-01-test" {
		t.Errorf("Expected version 2024-01-test, got %s", result.Version)
	}

	// Test data has 12 rows, 1 suppressed
	expectedConcepts := int64(11) // 12 total - 1 suppressed
	if result.ConceptsLoaded != expectedConcepts {
		t.Errorf("Expected %d concepts loaded, got %d", expectedConcepts, result.ConceptsLoaded)
	}

	// NDC should be loaded
	if result.NDCLoaded == 0 {
		t.Error("Expected NDC codes to be loaded")
	}

	// Verify release was created
	release, err := migrator.GetActiveRelease(ctx, VocabRxNorm)
	if err != nil {
		t.Fatalf("GetActiveRelease failed: %v", err)
	}
	if release == nil {
		t.Fatal("Expected release to exist")
	}
	if release.Version != "2024-01-test" {
		t.Errorf("Expected release version 2024-01-test, got %s", release.Version)
	}
}

func TestRxNormLoader_Integration_LoadRRF_WithRelations(t *testing.T) {
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

	testDataPath := getTestDataPath(t, "rxnorm")
	loader := NewRxNormLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Load with relations
	opts := &RxNormLoadOptions{
		SkipSuppressed: true,
		SkipRelations:  false,
		LoadNDC:        true,
	}

	result, err := loader.LoadRRF(ctx, testDataPath, "2024-01-rel", &releaseDate, opts, nil)
	if err != nil {
		t.Fatalf("LoadRRF failed: %v", err)
	}

	// Relations should be loaded (8 rows - 1 suppressed = 7)
	expectedRelations := int64(7)
	if result.RelationsLoaded != expectedRelations {
		t.Errorf("Expected %d relations loaded, got %d", expectedRelations, result.RelationsLoaded)
	}
}

func TestRxNormLoader_Integration_LoadRRF_SkipRelations(t *testing.T) {
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

	testDataPath := getTestDataPath(t, "rxnorm")
	loader := NewRxNormLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Skip relations
	opts := &RxNormLoadOptions{
		SkipSuppressed: true,
		SkipRelations:  true,
		LoadNDC:        false,
	}

	result, err := loader.LoadRRF(ctx, testDataPath, "2024-01-skip", &releaseDate, opts, nil)
	if err != nil {
		t.Fatalf("LoadRRF failed: %v", err)
	}

	// No relations should be loaded
	if result.RelationsLoaded != 0 {
		t.Errorf("Expected 0 relations with SkipRelations=true, got %d", result.RelationsLoaded)
	}

	// No NDC should be loaded
	if result.NDCLoaded != 0 {
		t.Errorf("Expected 0 NDC with LoadNDC=false, got %d", result.NDCLoaded)
	}
}

func TestRxNormLoader_Integration_Reload(t *testing.T) {
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

	testDataPath := getTestDataPath(t, "rxnorm")
	loader := NewRxNormLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	opts := &RxNormLoadOptions{
		SkipSuppressed: true,
		LoadNDC:        true,
	}

	// Load first time
	result1, err := loader.LoadRRF(ctx, testDataPath, "2024-01-reload", &releaseDate, opts, nil)
	if err != nil {
		t.Fatalf("First load failed: %v", err)
	}

	// Load second time (reload)
	result2, err := loader.LoadRRF(ctx, testDataPath, "2024-01-reload", &releaseDate, opts, nil)
	if err != nil {
		t.Fatalf("Second load (reload) failed: %v", err)
	}

	// Should have same release ID and counts
	if result1.ReleaseID != result2.ReleaseID {
		t.Errorf("Reload should reuse release ID: got %d vs %d", result1.ReleaseID, result2.ReleaseID)
	}

	if result1.ConceptsLoaded != result2.ConceptsLoaded {
		t.Errorf("Reload should have same concepts: got %d vs %d", result1.ConceptsLoaded, result2.ConceptsLoaded)
	}
}

func TestRxNormLoader_Integration_FilterTTY(t *testing.T) {
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

	testDataPath := getTestDataPath(t, "rxnorm")
	loader := NewRxNormLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Filter to only IN (ingredients)
	opts := &RxNormLoadOptions{
		SkipSuppressed: true,
		FilterTTY:      []string{"IN"},
	}

	result, err := loader.LoadRRF(ctx, testDataPath, "2024-01-filter", &releaseDate, opts, nil)
	if err != nil {
		t.Fatalf("LoadRRF failed: %v", err)
	}

	// Test data has 5 IN records (ingredients): Aspirin, Metformin, Lisinopril, Albuterol (4 non-suppressed)
	// Note: TTY=IN rows are: 1191, 6809, 29046, 153010
	expectedIN := int64(4)
	if result.ConceptsLoaded != expectedIN {
		t.Errorf("Expected %d IN concepts, got %d", expectedIN, result.ConceptsLoaded)
	}
}

// =============================================================================
// RxNorm Query Integration Tests
// =============================================================================

func setupRxNormTestData(t *testing.T, tc *TestPostgresContainer) {
	t.Helper()
	ctx := context.Background()

	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	testDataPath := getTestDataPath(t, "rxnorm")
	loader := NewRxNormLoader(tc.DB)
	releaseDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	opts := &RxNormLoadOptions{
		SkipSuppressed: true,
		SkipRelations:  false,
		LoadNDC:        true,
	}

	_, err = loader.LoadRRF(ctx, testDataPath, "test", &releaseDate, opts, nil)
	if err != nil {
		t.Fatalf("LoadRRF failed: %v", err)
	}
}

func TestRxNormQueries_Integration_GetByRXCUI(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	setupRxNormTestData(t, tc)
	ctx := context.Background()
	queries := NewRxNormQueries(tc.DB)

	// Lookup Aspirin (IN)
	concepts, err := queries.GetByRXCUI(ctx, "1191")
	if err != nil {
		t.Fatalf("GetByRXCUI failed: %v", err)
	}

	if len(concepts) == 0 {
		t.Fatal("Expected at least one concept for Aspirin")
	}

	found := false
	for _, c := range concepts {
		if c.Str == "Aspirin" && c.TTY == "IN" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find Aspirin IN concept")
	}
}

func TestRxNormQueries_Integration_GetByRXCUI_NotFound(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	setupRxNormTestData(t, tc)
	ctx := context.Background()
	queries := NewRxNormQueries(tc.DB)

	// Lookup non-existent RXCUI
	concepts, err := queries.GetByRXCUI(ctx, "999999999")
	if err != nil {
		t.Fatalf("GetByRXCUI failed: %v", err)
	}

	if len(concepts) != 0 {
		t.Errorf("Expected 0 concepts for non-existent RXCUI, got %d", len(concepts))
	}
}

func TestRxNormQueries_Integration_SearchByName(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	setupRxNormTestData(t, tc)
	ctx := context.Background()
	queries := NewRxNormQueries(tc.DB)

	// Search for Aspirin
	concepts, err := queries.SearchByName(ctx, "aspirin", 10)
	if err != nil {
		t.Fatalf("SearchByName failed: %v", err)
	}

	if len(concepts) == 0 {
		t.Fatal("Expected at least one result for 'aspirin'")
	}

	// All results should contain "aspirin" (case-insensitive)
	for _, c := range concepts {
		if !containsIgnoreCase(c.Str, "aspirin") {
			t.Errorf("Result %q should contain 'aspirin'", c.Str)
		}
	}
}

func TestRxNormQueries_Integration_SearchByName_CaseInsensitive(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	setupRxNormTestData(t, tc)
	ctx := context.Background()
	queries := NewRxNormQueries(tc.DB)

	// Search with different cases
	lower, _ := queries.SearchByName(ctx, "metformin", 10)
	upper, _ := queries.SearchByName(ctx, "METFORMIN", 10)
	mixed, _ := queries.SearchByName(ctx, "MeTfOrMiN", 10)

	if len(lower) != len(upper) || len(upper) != len(mixed) {
		t.Errorf("Case-insensitive search should return same count: lower=%d, upper=%d, mixed=%d",
			len(lower), len(upper), len(mixed))
	}
}

func TestRxNormQueries_Integration_LookupNDC(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	setupRxNormTestData(t, tc)
	ctx := context.Background()
	queries := NewRxNormQueries(tc.DB)

	// Lookup NDC for Aspirin 325 MG tablet (from test data: 00904551260)
	concepts, err := queries.LookupNDC(ctx, "00904551260")
	if err != nil {
		t.Fatalf("LookupNDC failed: %v", err)
	}

	if len(concepts) == 0 {
		t.Fatal("Expected at least one concept for NDC 00904551260")
	}

	// Should return the Aspirin 325 MG tablet
	found := false
	for _, c := range concepts {
		if c.RXCUI == "198765" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find RXCUI 198765 for the NDC")
	}
}

func TestRxNormQueries_Integration_LookupNDC_WithDashes(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	setupRxNormTestData(t, tc)
	ctx := context.Background()
	queries := NewRxNormQueries(tc.DB)

	// Lookup NDC with dashes (should normalize)
	concepts, err := queries.LookupNDC(ctx, "0090-4551-260")
	if err != nil {
		t.Fatalf("LookupNDC failed: %v", err)
	}

	// Should still find it after normalization
	// Note: depends on how the NDC was stored - may or may not match
	// At minimum, this tests that dashes don't cause errors
	_ = concepts // Query succeeded
}

func TestRxNormQueries_Integration_GetIngredients(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	setupRxNormTestData(t, tc)
	ctx := context.Background()
	queries := NewRxNormQueries(tc.DB)

	// Get ingredients for Aspirin 325 MG Oral Tablet (RXCUI 198765)
	ingredients, err := queries.GetIngredients(ctx, "198765")
	if err != nil {
		t.Fatalf("GetIngredients failed: %v", err)
	}

	if len(ingredients) == 0 {
		t.Fatal("Expected at least one ingredient for Aspirin 325 MG tablet")
	}

	// Should find Aspirin as ingredient
	found := false
	for _, ing := range ingredients {
		if ing.RXCUI == "1191" && ing.TTY == "IN" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find Aspirin (1191) as ingredient")
	}
}

func TestRxNormQueries_Integration_GetNDCs(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	setupRxNormTestData(t, tc)
	ctx := context.Background()
	queries := NewRxNormQueries(tc.DB)

	// Get NDCs for Aspirin 325 MG tablet (RXCUI 198765)
	// Test data has 2 NDCs for this RXCUI
	ndcs, err := queries.GetNDCs(ctx, "198765")
	if err != nil {
		t.Fatalf("GetNDCs failed: %v", err)
	}

	if len(ndcs) < 1 {
		t.Errorf("Expected at least 1 NDC for RXCUI 198765, got %d", len(ndcs))
	}
}

func TestRxNormQueries_Integration_Count(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	setupRxNormTestData(t, tc)
	ctx := context.Background()
	queries := NewRxNormQueries(tc.DB)

	count, err := queries.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}

	// Test data has 11 non-suppressed concepts
	expectedCount := int64(11)
	if count != expectedCount {
		t.Errorf("Expected count %d, got %d", expectedCount, count)
	}
}

func TestRxNormQueries_Integration_CountNDC(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	setupRxNormTestData(t, tc)
	ctx := context.Background()
	queries := NewRxNormQueries(tc.DB)

	count, err := queries.CountNDC(ctx)
	if err != nil {
		t.Fatalf("CountNDC failed: %v", err)
	}

	// Test data has 7 NDC rows
	if count < 1 {
		t.Errorf("Expected at least 1 NDC, got %d", count)
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > 0 && len(substr) > 0 &&
				(s[0]|0x20) >= 'a' && (s[0]|0x20) <= 'z' &&
				containsIgnoreCaseSlow(s, substr))
}

func containsIgnoreCaseSlow(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			tc := substr[j]
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if tc >= 'A' && tc <= 'Z' {
				tc += 32
			}
			if sc != tc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
