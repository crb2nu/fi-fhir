//go:build integration

package db

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// =============================================================================
// MappingStore Integration Tests
// =============================================================================

func TestMappingStore_CreateAndLookupMapping(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Initialize schema
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	// Create a mapping
	mapping := &CustomMapping{
		SourceSystem:  "epic_labs",
		SourceCode:    "LAB001",
		SourceDisplay: "Glucose Level",
		TargetSystem:  "http://loinc.org",
		TargetCode:    "2345-7",
		TargetDisplay: "Glucose [Mass/volume] in Serum or Plasma",
		Equivalence:   EquivalenceEquivalent,
		Origin:        OriginManual,
		CreatedBy:     "test@example.com",
	}

	err = store.CreateMapping(ctx, mapping)
	if err != nil {
		t.Fatalf("CreateMapping failed: %v", err)
	}

	if mapping.ID == 0 {
		t.Error("Expected mapping ID to be set")
	}
	if mapping.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}

	// Lookup the mapping
	found, err := store.LookupMapping(ctx, "epic_labs", "LAB001", "http://loinc.org", "")
	if err != nil {
		t.Fatalf("LookupMapping failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find mapping")
	}

	if found.TargetCode != "2345-7" {
		t.Errorf("TargetCode = %s, want 2345-7", found.TargetCode)
	}
	if found.Equivalence != EquivalenceEquivalent {
		t.Errorf("Equivalence = %s, want equivalent", found.Equivalence)
	}
}

func TestMappingStore_LookupMapping_ProfilePriority(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	// Create global mapping
	globalMapping := &CustomMapping{
		SourceSystem: "epic_labs",
		SourceCode:   "LAB001",
		TargetSystem: "http://loinc.org",
		TargetCode:   "2345-7",
		Equivalence:  EquivalenceEquivalent,
		Origin:       OriginManual,
	}
	err = store.CreateMapping(ctx, globalMapping)
	if err != nil {
		t.Fatalf("CreateMapping (global) failed: %v", err)
	}

	// Create profile-specific mapping
	profileMapping := &CustomMapping{
		SourceSystem: "epic_labs",
		SourceCode:   "LAB001",
		TargetSystem: "http://loinc.org",
		TargetCode:   "2339-0", // Different code for this profile
		Equivalence:  EquivalenceWider,
		ProfileID:    "profile-abc",
		Origin:       OriginManual,
	}
	err = store.CreateMapping(ctx, profileMapping)
	if err != nil {
		t.Fatalf("CreateMapping (profile) failed: %v", err)
	}

	// Lookup with profile - should return profile-specific
	found, err := store.LookupMapping(ctx, "epic_labs", "LAB001", "http://loinc.org", "profile-abc")
	if err != nil {
		t.Fatalf("LookupMapping failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find mapping")
	}
	if found.TargetCode != "2339-0" {
		t.Errorf("Expected profile-specific code 2339-0, got %s", found.TargetCode)
	}

	// Lookup without profile - should return global
	found, err = store.LookupMapping(ctx, "epic_labs", "LAB001", "http://loinc.org", "")
	if err != nil {
		t.Fatalf("LookupMapping failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find mapping")
	}
	if found.TargetCode != "2345-7" {
		t.Errorf("Expected global code 2345-7, got %s", found.TargetCode)
	}
}

func TestMappingStore_LookupMapping_NotFound(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	// Lookup non-existent mapping
	found, err := store.LookupMapping(ctx, "unknown", "XXX", "http://loinc.org", "")
	if err != nil {
		t.Fatalf("LookupMapping failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent mapping")
	}
}

func TestMappingStore_CreateMappingsBatch(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	// Create batch of mappings
	mappings := []*CustomMapping{
		{
			SourceSystem: "epic_labs",
			SourceCode:   "LAB001",
			TargetSystem: "http://loinc.org",
			TargetCode:   "2345-7",
			Equivalence:  EquivalenceEquivalent,
			Origin:       OriginCSVUpload,
		},
		{
			SourceSystem: "epic_labs",
			SourceCode:   "LAB002",
			TargetSystem: "http://loinc.org",
			TargetCode:   "4548-4",
			Equivalence:  EquivalenceEquivalent,
			Origin:       OriginCSVUpload,
		},
		{
			SourceSystem: "epic_labs",
			SourceCode:   "LAB003",
			TargetSystem: "http://loinc.org",
			TargetCode:   "718-7",
			Equivalence:  EquivalenceEquivalent,
			Origin:       OriginCSVUpload,
		},
	}

	created, duplicates, err := store.CreateMappingsBatch(ctx, mappings)
	if err != nil {
		t.Fatalf("CreateMappingsBatch failed: %v", err)
	}

	if created != 3 {
		t.Errorf("Expected 3 created, got %d", created)
	}
	if duplicates != 0 {
		t.Errorf("Expected 0 duplicates, got %d", duplicates)
	}

	// Verify all mappings exist
	for _, m := range mappings {
		found, err := store.LookupMapping(ctx, m.SourceSystem, m.SourceCode, m.TargetSystem, "")
		if err != nil {
			t.Errorf("LookupMapping for %s failed: %v", m.SourceCode, err)
		}
		if found == nil {
			t.Errorf("Mapping %s not found", m.SourceCode)
		}
	}
}

func TestMappingStore_CreateMappingsBatch_Duplicates(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	// Create initial mapping
	mapping := &CustomMapping{
		SourceSystem: "epic_labs",
		SourceCode:   "LAB001",
		TargetSystem: "http://loinc.org",
		TargetCode:   "2345-7",
		Equivalence:  EquivalenceEquivalent,
		Origin:       OriginManual,
	}
	err = store.CreateMapping(ctx, mapping)
	if err != nil {
		t.Fatalf("CreateMapping failed: %v", err)
	}

	// Try to insert batch with duplicates
	batch := []*CustomMapping{
		{
			SourceSystem: "epic_labs",
			SourceCode:   "LAB001", // Duplicate!
			TargetSystem: "http://loinc.org",
			TargetCode:   "9999-9",
			Equivalence:  EquivalenceEquivalent,
			Origin:       OriginCSVUpload,
		},
		{
			SourceSystem: "epic_labs",
			SourceCode:   "LAB002", // New
			TargetSystem: "http://loinc.org",
			TargetCode:   "4548-4",
			Equivalence:  EquivalenceEquivalent,
			Origin:       OriginCSVUpload,
		},
	}

	created, duplicates, err := store.CreateMappingsBatch(ctx, batch)
	if err != nil {
		t.Fatalf("CreateMappingsBatch failed: %v", err)
	}

	if created != 1 {
		t.Errorf("Expected 1 created, got %d", created)
	}
	if duplicates != 1 {
		t.Errorf("Expected 1 duplicate, got %d", duplicates)
	}
}

func TestMappingStore_CreateMappingsBatch_Empty(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	created, duplicates, err := store.CreateMappingsBatch(ctx, nil)
	if err != nil {
		t.Fatalf("CreateMappingsBatch failed: %v", err)
	}
	if created != 0 || duplicates != 0 {
		t.Errorf("Expected 0/0, got %d/%d", created, duplicates)
	}
}

func TestMappingStore_ListMappings(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	// Create some mappings
	for i := 0; i < 5; i++ {
		m := &CustomMapping{
			SourceSystem: "epic_labs",
			SourceCode:   string(rune('A' + i)),
			TargetSystem: "http://loinc.org",
			TargetCode:   "1234-5",
			Equivalence:  EquivalenceEquivalent,
			Origin:       OriginManual,
		}
		if err := store.CreateMapping(ctx, m); err != nil {
			t.Fatalf("CreateMapping failed: %v", err)
		}
	}

	// List all
	mappings, total, err := store.ListMappings(ctx, ListMappingsFilter{})
	if err != nil {
		t.Fatalf("ListMappings failed: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(mappings) != 5 {
		t.Errorf("Expected 5 mappings, got %d", len(mappings))
	}

	// List with limit
	mappings, total, err = store.ListMappings(ctx, ListMappingsFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListMappings failed: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(mappings) != 2 {
		t.Errorf("Expected 2 mappings, got %d", len(mappings))
	}

	// List with filter
	mappings, total, err = store.ListMappings(ctx, ListMappingsFilter{SourceSystem: "epic_labs"})
	if err != nil {
		t.Fatalf("ListMappings failed: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
}

func TestMappingStore_GetAndDeleteMapping(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	// Create mapping
	mapping := &CustomMapping{
		SourceSystem: "epic_labs",
		SourceCode:   "LAB001",
		TargetSystem: "http://loinc.org",
		TargetCode:   "2345-7",
		Equivalence:  EquivalenceEquivalent,
		Origin:       OriginManual,
	}
	err = store.CreateMapping(ctx, mapping)
	if err != nil {
		t.Fatalf("CreateMapping failed: %v", err)
	}

	// Get mapping
	found, err := store.GetMapping(ctx, mapping.ID)
	if err != nil {
		t.Fatalf("GetMapping failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find mapping")
	}
	if found.SourceCode != "LAB001" {
		t.Errorf("SourceCode = %s, want LAB001", found.SourceCode)
	}

	// Delete mapping
	err = store.DeleteMapping(ctx, mapping.ID)
	if err != nil {
		t.Fatalf("DeleteMapping failed: %v", err)
	}

	// Verify deleted
	found, err = store.GetMapping(ctx, mapping.ID)
	if err != nil {
		t.Fatalf("GetMapping failed: %v", err)
	}
	if found != nil {
		t.Error("Expected mapping to be deleted")
	}
}

func TestMappingStore_DeleteMapping_NotFound(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	err = store.DeleteMapping(ctx, 999999)
	if err == nil {
		t.Error("Expected error for non-existent mapping")
	}
}

// =============================================================================
// UploadBatch Integration Tests
// =============================================================================

func TestMappingStore_CreateAndGetBatch(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	// Create batch
	batch := &UploadBatch{
		Filename:      "mappings.csv",
		SourceSystem:  "epic_labs",
		TargetSystem:  "http://loinc.org",
		TotalRows:     100,
		ValidRows:     95,
		DuplicateRows: 3,
		ErrorRows:     2,
		UploadedBy:    "admin@example.com",
		ValidationErrors: []ValidationError{
			{Row: 10, Column: "source_code", Message: "Missing value"},
			{Row: 50, Column: "equivalence", Message: "Invalid value"},
		},
	}

	err = store.CreateBatch(ctx, batch)
	if err != nil {
		t.Fatalf("CreateBatch failed: %v", err)
	}

	if batch.ID == uuid.Nil {
		t.Error("Expected batch ID to be set")
	}
	if batch.UploadedAt.IsZero() {
		t.Error("Expected UploadedAt to be set")
	}

	// Get batch
	found, err := store.GetBatch(ctx, batch.ID)
	if err != nil {
		t.Fatalf("GetBatch failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find batch")
	}

	if found.Filename != "mappings.csv" {
		t.Errorf("Filename = %s, want mappings.csv", found.Filename)
	}
	if found.TotalRows != 100 {
		t.Errorf("TotalRows = %d, want 100", found.TotalRows)
	}
	if len(found.ValidationErrors) != 2 {
		t.Errorf("ValidationErrors count = %d, want 2", len(found.ValidationErrors))
	}
}

func TestMappingStore_GetBatch_NotFound(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	found, err := store.GetBatch(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetBatch failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent batch")
	}
}

func TestMappingStore_UpdateBatchStats(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	// Create batch with initial stats
	batch := &UploadBatch{
		Filename:  "test.csv",
		TotalRows: 100,
	}
	err = store.CreateBatch(ctx, batch)
	if err != nil {
		t.Fatalf("CreateBatch failed: %v", err)
	}

	// Update stats
	err = store.UpdateBatchStats(ctx, batch.ID, 90, 5, 5)
	if err != nil {
		t.Fatalf("UpdateBatchStats failed: %v", err)
	}

	// Verify updated
	found, err := store.GetBatch(ctx, batch.ID)
	if err != nil {
		t.Fatalf("GetBatch failed: %v", err)
	}
	if found.ValidRows != 90 {
		t.Errorf("ValidRows = %d, want 90", found.ValidRows)
	}
	if found.DuplicateRows != 5 {
		t.Errorf("DuplicateRows = %d, want 5", found.DuplicateRows)
	}
	if found.ErrorRows != 5 {
		t.Errorf("ErrorRows = %d, want 5", found.ErrorRows)
	}
}

func TestMappingStore_DeleteMappingsByBatch(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	// Create batch
	batch := &UploadBatch{Filename: "test.csv", TotalRows: 3}
	err = store.CreateBatch(ctx, batch)
	if err != nil {
		t.Fatalf("CreateBatch failed: %v", err)
	}

	// Create mappings linked to batch
	for i := 0; i < 3; i++ {
		m := &CustomMapping{
			SourceSystem:  "test",
			SourceCode:    string(rune('A' + i)),
			TargetSystem:  "http://loinc.org",
			TargetCode:    "1234-5",
			Equivalence:   EquivalenceEquivalent,
			Origin:        OriginCSVUpload,
			UploadBatchID: &batch.ID,
		}
		if err := store.CreateMapping(ctx, m); err != nil {
			t.Fatalf("CreateMapping failed: %v", err)
		}
	}

	// Verify mappings exist
	mappings, _, err := store.ListMappings(ctx, ListMappingsFilter{UploadBatchID: &batch.ID})
	if err != nil {
		t.Fatalf("ListMappings failed: %v", err)
	}
	if len(mappings) != 3 {
		t.Fatalf("Expected 3 mappings, got %d", len(mappings))
	}

	// Delete by batch
	deleted, err := store.DeleteMappingsByBatch(ctx, batch.ID)
	if err != nil {
		t.Fatalf("DeleteMappingsByBatch failed: %v", err)
	}
	if deleted != 3 {
		t.Errorf("Expected 3 deleted, got %d", deleted)
	}

	// Verify deleted
	mappings, _, err = store.ListMappings(ctx, ListMappingsFilter{UploadBatchID: &batch.ID})
	if err != nil {
		t.Fatalf("ListMappings failed: %v", err)
	}
	if len(mappings) != 0 {
		t.Errorf("Expected 0 mappings after delete, got %d", len(mappings))
	}
}

func TestMappingStore_MappingWithConfidence(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	store := NewMappingStore(tc.DB)

	confidence := 0.92
	mapping := &CustomMapping{
		SourceSystem: "epic_labs",
		SourceCode:   "LAB001",
		TargetSystem: "http://loinc.org",
		TargetCode:   "2345-7",
		Equivalence:  EquivalenceEquivalent,
		Confidence:   &confidence,
		Origin:       OriginApprovedAutoroute,
	}

	err = store.CreateMapping(ctx, mapping)
	if err != nil {
		t.Fatalf("CreateMapping failed: %v", err)
	}

	found, err := store.GetMapping(ctx, mapping.ID)
	if err != nil {
		t.Fatalf("GetMapping failed: %v", err)
	}
	if found.Confidence == nil {
		t.Fatal("Expected confidence to be set")
	}
	if *found.Confidence != 0.92 {
		t.Errorf("Confidence = %f, want 0.92", *found.Confidence)
	}
	if found.Origin != OriginApprovedAutoroute {
		t.Errorf("Origin = %s, want approved_autoroute", found.Origin)
	}
}
