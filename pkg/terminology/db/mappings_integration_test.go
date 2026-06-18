//go:build integration

package db

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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

// =============================================================================
// PendingAutoroute Integration Tests
// =============================================================================

// initStoreForTest is a helper that sets up a clean schema and returns a MappingStore.
func initStoreForTest(t *testing.T) (*MappingStore, context.Context) {
	t.Helper()
	tc := setupPostgresContainer(t)
	if tc == nil {
		t.Skip("no postgres container available")
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	return NewMappingStore(tc.DB), ctx
}

func TestMappingStore_CreateAndGetPendingAutoroute(t *testing.T) {
	store, ctx := initStoreForTest(t)

	decisionTrace := json.RawMessage(`{"steps":["fuzzy","semantic"],"model":"gpt-4"}`)
	alternates := json.RawMessage(`[{"code":"2345-7","display":"Glucose alt","confidence":0.85,"reasoning":"close match"}]`)

	pending := &PendingAutoroute{
		SourceSystem:     "epic_labs",
		SourceCode:       "LAB999",
		SourceDisplay:    "Glucose Level",
		TargetSystem:     "http://loinc.org",
		SuggestedCode:    "2345-7",
		SuggestedDisplay: "Glucose [Mass/volume] in Serum or Plasma",
		Confidence:       0.92,
		Equivalence:      "equivalent",
		Reasoning:        "Strong semantic similarity for glucose lab",
		DecisionTrace:    decisionTrace,
		Alternates:       alternates,
	}

	err := store.CreatePendingAutoroute(ctx, pending)
	if err != nil {
		t.Fatalf("CreatePendingAutoroute failed: %v", err)
	}

	if pending.ID == 0 {
		t.Error("Expected ID to be set")
	}
	if pending.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
	if pending.Status != StatusPending {
		t.Errorf("Status = %s, want pending", pending.Status)
	}

	// Retrieve and verify all fields round-trip
	got, err := store.GetPendingAutoroute(ctx, pending.ID)
	if err != nil {
		t.Fatalf("GetPendingAutoroute failed: %v", err)
	}
	if got == nil {
		t.Fatal("Expected pending autoroute to exist")
	}

	if got.SourceSystem != "epic_labs" {
		t.Errorf("SourceSystem = %s, want epic_labs", got.SourceSystem)
	}
	if got.SourceCode != "LAB999" {
		t.Errorf("SourceCode = %s, want LAB999", got.SourceCode)
	}
	if got.SourceDisplay != "Glucose Level" {
		t.Errorf("SourceDisplay = %s, want Glucose Level", got.SourceDisplay)
	}
	if got.TargetSystem != "http://loinc.org" {
		t.Errorf("TargetSystem = %s, want http://loinc.org", got.TargetSystem)
	}
	if got.SuggestedCode != "2345-7" {
		t.Errorf("SuggestedCode = %s, want 2345-7", got.SuggestedCode)
	}
	if got.SuggestedDisplay != "Glucose [Mass/volume] in Serum or Plasma" {
		t.Errorf("SuggestedDisplay = %s, want Glucose [Mass/volume] in Serum or Plasma", got.SuggestedDisplay)
	}
	if got.Confidence != 0.92 {
		t.Errorf("Confidence = %f, want 0.92", got.Confidence)
	}
	if got.Equivalence != "equivalent" {
		t.Errorf("Equivalence = %s, want equivalent", got.Equivalence)
	}
	if got.Reasoning != "Strong semantic similarity for glucose lab" {
		t.Errorf("Reasoning = %s, want Strong semantic similarity for glucose lab", got.Reasoning)
	}
	if got.Status != StatusPending {
		t.Errorf("Status = %s, want pending", got.Status)
	}
	if len(got.DecisionTrace) == 0 {
		t.Error("Expected DecisionTrace to be populated")
	}
	if len(got.Alternates) == 0 {
		t.Error("Expected Alternates to be populated")
	}

	// Verify the JSON content round-trips correctly
	var alts []Alternate
	if err := json.Unmarshal(got.Alternates, &alts); err != nil {
		t.Fatalf("Unmarshal alternates failed: %v", err)
	}
	if len(alts) != 1 || alts[0].Code != "2345-7" {
		t.Errorf("Alternates content mismatch: got %+v", alts)
	}
}

func TestMappingStore_CreatePendingAutoroute_Upsert(t *testing.T) {
	store, ctx := initStoreForTest(t)

	pending := &PendingAutoroute{
		SourceSystem:  "epic_labs",
		SourceCode:    "LAB100",
		TargetSystem:  "http://loinc.org",
		SuggestedCode: "12345-6",
		Confidence:    0.80,
		Reasoning:     "initial suggestion",
	}

	err := store.CreatePendingAutoroute(ctx, pending)
	if err != nil {
		t.Fatalf("First create failed: %v", err)
	}
	firstID := pending.ID

	// Now approve it
	err = store.RejectPendingAutoroute(ctx, pending.ID, "reviewer", "testing upsert")
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}

	// Re-create with same key — should upsert back to pending
	pending2 := &PendingAutoroute{
		SourceSystem:  "epic_labs",
		SourceCode:    "LAB100",
		TargetSystem:  "http://loinc.org",
		SuggestedCode: "12345-6",
		Confidence:    0.95,
		Reasoning:     "improved suggestion",
	}

	err = store.CreatePendingAutoroute(ctx, pending2)
	if err != nil {
		t.Fatalf("Upsert create failed: %v", err)
	}

	if pending2.ID != firstID {
		t.Errorf("Expected same ID on upsert: got %d, want %d", pending2.ID, firstID)
	}

	got, err := store.GetPendingAutoroute(ctx, pending2.ID)
	if err != nil {
		t.Fatalf("Get after upsert failed: %v", err)
	}
	if got.Status != StatusPending {
		t.Errorf("Status after upsert = %s, want pending", got.Status)
	}
	if got.Confidence != 0.95 {
		t.Errorf("Confidence after upsert = %f, want 0.95", got.Confidence)
	}
}

func TestMappingStore_GetPendingAutoroute_NotFound(t *testing.T) {
	store, ctx := initStoreForTest(t)

	got, err := store.GetPendingAutoroute(ctx, 99999)
	if err != nil {
		t.Fatalf("GetPendingAutoroute failed: %v", err)
	}
	if got != nil {
		t.Error("Expected nil for non-existent ID")
	}
}

func TestMappingStore_ListPendingAutoroutes_Filters(t *testing.T) {
	store, ctx := initStoreForTest(t)

	// Create pending autoroutes with varying attributes
	entries := []struct {
		sourceSystem  string
		sourceCode    string
		targetSystem  string
		suggestedCode string
		confidence    float64
	}{
		{"epic_labs", "LAB001", "http://loinc.org", "2345-7", 0.95},
		{"epic_labs", "LAB002", "http://loinc.org", "2345-8", 0.85},
		{"epic_labs", "LAB003", "http://loinc.org", "2345-9", 0.70},
		{"cerner_labs", "CLB001", "http://loinc.org", "1234-5", 0.90},
		{"epic_labs", "LAB004", "http://snomed.info/sct", "12345", 0.88},
	}

	for _, e := range entries {
		p := &PendingAutoroute{
			SourceSystem:  e.sourceSystem,
			SourceCode:    e.sourceCode,
			TargetSystem:  e.targetSystem,
			SuggestedCode: e.suggestedCode,
			Confidence:    e.confidence,
		}
		if err := store.CreatePendingAutoroute(ctx, p); err != nil {
			t.Fatalf("Create failed for %s: %v", e.sourceCode, err)
		}
	}

	// Get epic_labs count for later assertions
	results, total, err := store.ListPendingAutoroutes(ctx, ListPendingAutoroutesFilter{
		SourceSystem: "epic_labs",
	})
	if err != nil {
		t.Fatalf("ListPendingAutoroutes failed: %v", err)
	}
	_ = results // we just need total for epic_labs
	epicTotal := total

	// Filter by status: pending
	results, total, err = store.ListPendingAutoroutes(ctx, ListPendingAutoroutesFilter{Status: StatusPending})
	if err != nil {
		t.Fatalf("List by status failed: %v", err)
	}
	if total != 5 {
		t.Errorf("Total pending = %d, want 5", total)
	}

	// Filter by source system
	results, total, err = store.ListPendingAutoroutes(ctx, ListPendingAutoroutesFilter{SourceSystem: "epic_labs"})
	if err != nil {
		t.Fatalf("List by source system failed: %v", err)
	}
	if total != int(epicTotal) {
		t.Errorf("Total for epic_labs = %d, want %d", total, epicTotal)
	}

	// Filter by target system
	results, total, err = store.ListPendingAutoroutes(ctx, ListPendingAutoroutesFilter{TargetSystem: "http://snomed.info/sct"})
	if err != nil {
		t.Fatalf("List by target system failed: %v", err)
	}
	if total != 1 {
		t.Errorf("Total for SNOMED target = %d, want 1", total)
	}

	// Filter by min confidence
	minConf := 0.88
	results, total, err = store.ListPendingAutoroutes(ctx, ListPendingAutoroutesFilter{MinConfidence: &minConf})
	if err != nil {
		t.Fatalf("List by min confidence failed: %v", err)
	}
	if total != 3 {
		t.Errorf("Total with confidence >= 0.88 = %d, want 3", total)
	}

	// Verify ordering: highest confidence first
	if len(results) > 1 && results[0].Confidence < results[1].Confidence {
		t.Error("Expected results ordered by confidence DESC")
	}

	// Pagination
	results, _, err = store.ListPendingAutoroutes(ctx, ListPendingAutoroutesFilter{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("List with pagination failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Paginated results = %d, want 2", len(results))
	}
}

func TestMappingStore_ApprovePendingAutoroute(t *testing.T) {
	store, ctx := initStoreForTest(t)

	decisionTrace := json.RawMessage(`{"model":"gpt-4","steps":["lookup","semantic"]}`)

	pending := &PendingAutoroute{
		SourceSystem:     "epic_labs",
		SourceCode:       "LAB001",
		SourceDisplay:    "Glucose Level",
		TargetSystem:     "http://loinc.org",
		SuggestedCode:    "2345-7",
		SuggestedDisplay: "Glucose [Mass/volume]",
		Confidence:       0.93,
		Equivalence:      "equivalent",
		Reasoning:        "high confidence semantic match",
		DecisionTrace:    decisionTrace,
	}

	err := store.CreatePendingAutoroute(ctx, pending)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Approve
	mapping, err := store.ApprovePendingAutoroute(ctx, pending.ID, "reviewer@example.com", "", "approved via test")
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}

	// Verify the created CustomMapping
	if mapping == nil {
		t.Fatal("Expected mapping to be returned")
	}
	if mapping.ID == 0 {
		t.Error("Expected mapping ID to be set")
	}
	if mapping.Origin != OriginApprovedAutoroute {
		t.Errorf("Origin = %s, want approved_autoroute", mapping.Origin)
	}
	if mapping.SourceCode != "LAB001" {
		t.Errorf("SourceCode = %s, want LAB001", mapping.SourceCode)
	}
	if mapping.TargetCode != "2345-7" {
		t.Errorf("TargetCode = %s, want 2345-7", mapping.TargetCode)
	}
	if mapping.Equivalence != EquivalenceEquivalent {
		t.Errorf("Equivalence = %s, want equivalent", mapping.Equivalence)
	}
	if mapping.ApprovedBy != "reviewer@example.com" {
		t.Errorf("ApprovedBy = %s, want reviewer@example.com", mapping.ApprovedBy)
	}
	if mapping.ApprovedAt == nil {
		t.Error("Expected ApprovedAt to be set")
	}
	if mapping.Confidence == nil || *mapping.Confidence != 0.93 {
		t.Errorf("Confidence mismatch: got %v", mapping.Confidence)
	}
	if len(mapping.DecisionTrace) == 0 {
		t.Error("Expected DecisionTrace to be carried over from pending")
	}

	// Verify the pending autoroute was updated
	got, err := store.GetPendingAutoroute(ctx, pending.ID)
	if err != nil {
		t.Fatalf("GetPendingAutoroute after approve failed: %v", err)
	}
	if got.Status != StatusApproved {
		t.Errorf("Pending status = %s, want approved", got.Status)
	}
	if got.ReviewedBy != "reviewer@example.com" {
		t.Errorf("ReviewedBy = %s, want reviewer@example.com", got.ReviewedBy)
	}
	if got.ReviewedAt == nil {
		t.Error("Expected ReviewedAt to be set")
	}

	// Verify the mapping can be looked up
	found, err := store.LookupMapping(ctx, "epic_labs", "LAB001", "http://loinc.org", "")
	if err != nil {
		t.Fatalf("LookupMapping failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected mapping to be found via LookupMapping")
	}
	if found.ID != mapping.ID {
		t.Errorf("Lookup returned different ID: got %d, want %d", found.ID, mapping.ID)
	}
}

func TestMappingStore_ApprovePendingAutoroute_EquivalenceOverride(t *testing.T) {
	store, ctx := initStoreForTest(t)

	pending := &PendingAutoroute{
		SourceSystem:  "epic_labs",
		SourceCode:    "LAB050",
		TargetSystem:  "http://loinc.org",
		SuggestedCode: "9999-1",
		Confidence:    0.78,
		Equivalence:   "equivalent",
	}

	err := store.CreatePendingAutoroute(ctx, pending)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Approve with equivalence override: reviewer downgrades to "wider"
	mapping, err := store.ApprovePendingAutoroute(ctx, pending.ID, "reviewer", "wider", "")
	if err != nil {
		t.Fatalf("Approve with override failed: %v", err)
	}

	if mapping.Equivalence != EquivalenceWider {
		t.Errorf("Equivalence = %s, want wider (override should apply)", mapping.Equivalence)
	}
}

func TestMappingStore_ApprovePendingAutoroute_AlreadyApproved(t *testing.T) {
	store, ctx := initStoreForTest(t)

	pending := &PendingAutoroute{
		SourceSystem:  "epic_labs",
		SourceCode:    "LAB060",
		TargetSystem:  "http://loinc.org",
		SuggestedCode: "8888-1",
		Confidence:    0.90,
	}

	err := store.CreatePendingAutoroute(ctx, pending)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// First approve succeeds
	_, err = store.ApprovePendingAutoroute(ctx, pending.ID, "reviewer", "", "")
	if err != nil {
		t.Fatalf("First approve failed: %v", err)
	}

	// Second approve should fail
	_, err = store.ApprovePendingAutoroute(ctx, pending.ID, "reviewer", "", "")
	if err == nil {
		t.Fatal("Expected error on double approve")
	}
}

func TestMappingStore_RejectPendingAutoroute(t *testing.T) {
	store, ctx := initStoreForTest(t)

	pending := &PendingAutoroute{
		SourceSystem:  "epic_labs",
		SourceCode:    "LAB070",
		TargetSystem:  "http://loinc.org",
		SuggestedCode: "7777-1",
		Confidence:    0.65,
	}

	err := store.CreatePendingAutoroute(ctx, pending)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Reject
	err = store.RejectPendingAutoroute(ctx, pending.ID, "reviewer@example.com", "Incorrect mapping for this context")
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}

	// Verify status
	got, err := store.GetPendingAutoroute(ctx, pending.ID)
	if err != nil {
		t.Fatalf("Get after reject failed: %v", err)
	}
	if got.Status != StatusRejected {
		t.Errorf("Status = %s, want rejected", got.Status)
	}
	if got.ReviewedBy != "reviewer@example.com" {
		t.Errorf("ReviewedBy = %s, want reviewer@example.com", got.ReviewedBy)
	}
	if got.RejectionReason != "Incorrect mapping for this context" {
		t.Errorf("RejectionReason = %s, want 'Incorrect mapping for this context'", got.RejectionReason)
	}
	if got.ReviewedAt == nil {
		t.Error("Expected ReviewedAt to be set")
	}

	// Verify no CustomMapping was created
	found, err := store.LookupMapping(ctx, "epic_labs", "LAB070", "http://loinc.org", "")
	if err != nil {
		t.Fatalf("LookupMapping failed: %v", err)
	}
	if found != nil {
		t.Error("Expected no mapping to be created for rejected autoroute")
	}
}

func TestMappingStore_RejectPendingAutoroute_AlreadyRejected(t *testing.T) {
	store, ctx := initStoreForTest(t)

	pending := &PendingAutoroute{
		SourceSystem:  "epic_labs",
		SourceCode:    "LAB080",
		TargetSystem:  "http://loinc.org",
		SuggestedCode: "6666-1",
		Confidence:    0.50,
	}

	err := store.CreatePendingAutoroute(ctx, pending)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// First reject
	err = store.RejectPendingAutoroute(ctx, pending.ID, "reviewer", "reason")
	if err != nil {
		t.Fatalf("First reject failed: %v", err)
	}

	// Second reject should fail
	err = store.RejectPendingAutoroute(ctx, pending.ID, "reviewer", "another reason")
	if err == nil {
		t.Fatal("Expected error on double reject")
	}
}

func TestMappingStore_BulkApprovePendingAutoroutes(t *testing.T) {
	store, ctx := initStoreForTest(t)

	// Create 5 pending autoroutes with varying confidence
	confidences := []float64{0.95, 0.90, 0.85, 0.70, 0.55}
	for i, conf := range confidences {
		p := &PendingAutoroute{
			SourceSystem:  "epic_labs",
			SourceCode:    fmt.Sprintf("BULK%03d", i+1),
			TargetSystem:  "http://loinc.org",
			SuggestedCode: fmt.Sprintf("%d-0", 1000+i),
			Confidence:    conf,
		}
		if err := store.CreatePendingAutoroute(ctx, p); err != nil {
			t.Fatalf("Create %d failed: %v", i, err)
		}
	}

	// Bulk approve with threshold 0.85 → should approve 3 (0.95, 0.90, 0.85)
	count, mappings, err := store.BulkApprovePendingAutoroutes(ctx, 0.85, 100, "bulk-reviewer")
	if err != nil {
		t.Fatalf("BulkApprove failed: %v", err)
	}

	if count != 3 {
		t.Errorf("Approved count = %d, want 3", count)
	}
	if len(mappings) != 3 {
		t.Errorf("Mappings returned = %d, want 3", len(mappings))
	}

	// Verify the remaining 2 are still pending
	counts, err := store.CountPendingAutoroutes(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if counts[StatusPending] != 2 {
		t.Errorf("Remaining pending = %d, want 2", counts[StatusPending])
	}
	if counts[StatusApproved] != 3 {
		t.Errorf("Approved = %d, want 3", counts[StatusApproved])
	}
}

func TestMappingStore_ExpirePendingAutoroutes(t *testing.T) {
	store, ctx := initStoreForTest(t)

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	// Create pending with past expiration
	expired := &PendingAutoroute{
		SourceSystem:  "epic_labs",
		SourceCode:    "EXP001",
		TargetSystem:  "http://loinc.org",
		SuggestedCode: "5555-1",
		Confidence:    0.75,
		ExpiresAt:     &past,
	}
	if err := store.CreatePendingAutoroute(ctx, expired); err != nil {
		t.Fatalf("Create expired failed: %v", err)
	}

	// Create pending with future expiration (should not expire)
	notExpired := &PendingAutoroute{
		SourceSystem:  "epic_labs",
		SourceCode:    "EXP002",
		TargetSystem:  "http://loinc.org",
		SuggestedCode: "5555-2",
		Confidence:    0.80,
		ExpiresAt:     &future,
	}
	if err := store.CreatePendingAutoroute(ctx, notExpired); err != nil {
		t.Fatalf("Create not-expired failed: %v", err)
	}

	// Create pending with no expiration (should not expire)
	noExpiry := &PendingAutoroute{
		SourceSystem:  "epic_labs",
		SourceCode:    "EXP003",
		TargetSystem:  "http://loinc.org",
		SuggestedCode: "5555-3",
		Confidence:    0.85,
	}
	if err := store.CreatePendingAutoroute(ctx, noExpiry); err != nil {
		t.Fatalf("Create no-expiry failed: %v", err)
	}

	// Run expire
	count, err := store.ExpirePendingAutoroutes(ctx)
	if err != nil {
		t.Fatalf("Expire failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expired count = %d, want 1", count)
	}

	// Verify the correct one was expired
	got, err := store.GetPendingAutoroute(ctx, expired.ID)
	if err != nil {
		t.Fatalf("Get expired failed: %v", err)
	}
	if got.Status != StatusExpired {
		t.Errorf("Status = %s, want expired", got.Status)
	}

	// Verify the others are still pending
	got2, _ := store.GetPendingAutoroute(ctx, notExpired.ID)
	if got2.Status != StatusPending {
		t.Errorf("Future expiry status = %s, want pending", got2.Status)
	}
	got3, _ := store.GetPendingAutoroute(ctx, noExpiry.ID)
	if got3.Status != StatusPending {
		t.Errorf("No expiry status = %s, want pending", got3.Status)
	}
}

// Time-expired pending rows must drop out of the review queue immediately,
// before any sweep flips their status column. List + Count must agree.
func TestMappingStore_ListPendingAutoroutes_HidesTimeExpired(t *testing.T) {
	store, ctx := initStoreForTest(t)

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	timeExpired := &PendingAutoroute{
		SourceSystem: "epic_labs", SourceCode: "TEXP001", TargetSystem: "http://loinc.org",
		SuggestedCode: "9001-1", Confidence: 0.90, ExpiresAt: &past,
	}
	future1 := &PendingAutoroute{
		SourceSystem: "epic_labs", SourceCode: "TEXP002", TargetSystem: "http://loinc.org",
		SuggestedCode: "9001-2", Confidence: 0.88, ExpiresAt: &future,
	}
	noExpiry := &PendingAutoroute{
		SourceSystem: "epic_labs", SourceCode: "TEXP003", TargetSystem: "http://loinc.org",
		SuggestedCode: "9001-3", Confidence: 0.86,
	}
	for _, p := range []*PendingAutoroute{timeExpired, future1, noExpiry} {
		if err := store.CreatePendingAutoroute(ctx, p); err != nil {
			t.Fatalf("Create %s failed: %v", p.SourceCode, err)
		}
	}

	// NOTE: no ExpirePendingAutoroutes() call — this proves the query-time
	// exclusion, independent of the sweep.

	// Filtering for pending must omit the time-expired row.
	list, total, err := store.ListPendingAutoroutes(ctx, ListPendingAutoroutesFilter{Status: StatusPending})
	if err != nil {
		t.Fatalf("ListPendingAutoroutes failed: %v", err)
	}
	if total != 2 {
		t.Errorf("pending total = %d, want 2 (time-expired row hidden)", total)
	}
	for _, p := range list {
		if p.SourceCode == "TEXP001" {
			t.Errorf("time-expired row TEXP001 leaked into the pending review queue")
		}
	}

	// No status filter must also omit the time-expired row (it is logically expired).
	allList, _, err := store.ListPendingAutoroutes(ctx, ListPendingAutoroutesFilter{})
	if err != nil {
		t.Fatalf("ListPendingAutoroutes (no filter) failed: %v", err)
	}
	for _, p := range allList {
		if p.SourceCode == "TEXP001" {
			t.Errorf("time-expired row TEXP001 leaked into the unfiltered list")
		}
	}

	// Counts must attribute the time-expired row to 'expired', not 'pending'.
	counts, err := store.CountPendingAutoroutes(ctx)
	if err != nil {
		t.Fatalf("CountPendingAutoroutes failed: %v", err)
	}
	if counts[StatusPending] != 2 {
		t.Errorf("counts[pending] = %d, want 2", counts[StatusPending])
	}
	if counts[StatusExpired] != 1 {
		t.Errorf("counts[expired] = %d, want 1 (time-expired pending row)", counts[StatusExpired])
	}
}

func TestMappingStore_CountPendingAutoroutes(t *testing.T) {
	store, ctx := initStoreForTest(t)

	// Create a mix: 3 pending, then approve 1 and reject 1
	for i := 0; i < 3; i++ {
		p := &PendingAutoroute{
			SourceSystem:  "epic_labs",
			SourceCode:    fmt.Sprintf("CNT%03d", i+1),
			TargetSystem:  "http://loinc.org",
			SuggestedCode: fmt.Sprintf("%d-C", 3000+i),
			Confidence:    0.80,
		}
		if err := store.CreatePendingAutoroute(ctx, p); err != nil {
			t.Fatalf("Create %d failed: %v", i, err)
		}
	}

	// Approve first
	results, _, _ := store.ListPendingAutoroutes(ctx, ListPendingAutoroutesFilter{Status: StatusPending, Limit: 1})
	if len(results) > 0 {
		store.ApprovePendingAutoroute(ctx, results[0].ID, "reviewer", "", "")
	}

	// Reject second
	results, _, _ = store.ListPendingAutoroutes(ctx, ListPendingAutoroutesFilter{Status: StatusPending, Limit: 1})
	if len(results) > 0 {
		store.RejectPendingAutoroute(ctx, results[0].ID, "reviewer", "not needed")
	}

	counts, err := store.CountPendingAutoroutes(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}

	if counts[StatusPending] != 1 {
		t.Errorf("Pending = %d, want 1", counts[StatusPending])
	}
	if counts[StatusApproved] != 1 {
		t.Errorf("Approved = %d, want 1", counts[StatusApproved])
	}
	if counts[StatusRejected] != 1 {
		t.Errorf("Rejected = %d, want 1", counts[StatusRejected])
	}
}

// =============================================================================
// MappingDecision Telemetry Integration Tests
// =============================================================================

func TestMappingStore_RecordAndGetMappingDecision(t *testing.T) {
	store, ctx := initStoreForTest(t)

	conf := 0.91
	decisionTree := json.RawMessage(`{"path":"cache→semantic→autoroute","duration_breakdown":{"cache_ms":2,"semantic_ms":150}}`)

	decision := &MappingDecision{
		TraceID:         "trace-abc-123",
		SourceSystem:    "epic_labs",
		SourceCode:      "LAB001",
		SourceDisplay:   "Glucose Level",
		TargetSystem:    "http://loinc.org",
		DecisionType:    DecisionAutorouteHighConf,
		Confidence:      &conf,
		SelectedCode:    "2345-7",
		SelectedDisplay: "Glucose [Mass/volume]",
		DecisionTree:    decisionTree,
		ProfileID:       "profile-1",
		RequestSource:   "graphql",
		DurationMs:      152,
	}

	err := store.RecordMappingDecision(ctx, decision)
	if err != nil {
		t.Fatalf("RecordMappingDecision failed: %v", err)
	}

	if decision.ID == 0 {
		t.Error("Expected ID to be set")
	}
	if decision.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}

	// Retrieve
	got, err := store.GetMappingDecision(ctx, decision.ID)
	if err != nil {
		t.Fatalf("GetMappingDecision failed: %v", err)
	}
	if got == nil {
		t.Fatal("Expected decision to exist")
	}

	if got.TraceID != "trace-abc-123" {
		t.Errorf("TraceID = %s, want trace-abc-123", got.TraceID)
	}
	if got.DecisionType != DecisionAutorouteHighConf {
		t.Errorf("DecisionType = %s, want AUTOROUTE_HIGH_CONF", got.DecisionType)
	}
	if got.Confidence == nil || *got.Confidence != 0.91 {
		t.Errorf("Confidence mismatch: got %v", got.Confidence)
	}
	if got.SelectedCode != "2345-7" {
		t.Errorf("SelectedCode = %s, want 2345-7", got.SelectedCode)
	}
	if got.ProfileID != "profile-1" {
		t.Errorf("ProfileID = %s, want profile-1", got.ProfileID)
	}
	if got.RequestSource != "graphql" {
		t.Errorf("RequestSource = %s, want graphql", got.RequestSource)
	}
	if got.DurationMs != 152 {
		t.Errorf("DurationMs = %d, want 152", got.DurationMs)
	}
	if len(got.DecisionTree) == 0 {
		t.Error("Expected DecisionTree to be populated")
	}
}

func TestMappingStore_ListMappingDecisions_Filters(t *testing.T) {
	store, ctx := initStoreForTest(t)

	// Record decisions of different types
	decisions := []struct {
		traceID       string
		sourceSystem  string
		decisionType  DecisionType
		requestSource string
	}{
		{"t1", "epic_labs", DecisionPersistentHit, "graphql"},
		{"t2", "epic_labs", DecisionAutorouteHighConf, "cli"},
		{"t3", "cerner_labs", DecisionAutorouteMedConf, "graphql"},
		{"t4", "epic_labs", DecisionNoMatch, "batch"},
		{"t5", "epic_labs", DecisionPersistentHit, "graphql"},
	}

	for _, d := range decisions {
		decision := &MappingDecision{
			TraceID:       d.traceID,
			SourceSystem:  d.sourceSystem,
			SourceCode:    "CODE1",
			TargetSystem:  "http://loinc.org",
			DecisionType:  d.decisionType,
			RequestSource: d.requestSource,
			DurationMs:    100,
		}
		if err := store.RecordMappingDecision(ctx, decision); err != nil {
			t.Fatalf("Record %s failed: %v", d.traceID, err)
		}
	}

	// Filter by decision type
	results, total, err := store.ListMappingDecisions(ctx, ListMappingDecisionsFilter{
		DecisionType: DecisionPersistentHit,
	})
	if err != nil {
		t.Fatalf("List by type failed: %v", err)
	}
	if total != 2 {
		t.Errorf("Total PERSISTENT_HIT = %d, want 2", total)
	}
	if len(results) != 2 {
		t.Errorf("Results count = %d, want 2", len(results))
	}

	// Filter by source system
	_, total, err = store.ListMappingDecisions(ctx, ListMappingDecisionsFilter{
		SourceSystem: "cerner_labs",
	})
	if err != nil {
		t.Fatalf("List by source system failed: %v", err)
	}
	if total != 1 {
		t.Errorf("Total for cerner_labs = %d, want 1", total)
	}

	// Filter by trace ID
	results, total, err = store.ListMappingDecisions(ctx, ListMappingDecisionsFilter{
		TraceID: "t3",
	})
	if err != nil {
		t.Fatalf("List by trace ID failed: %v", err)
	}
	if total != 1 {
		t.Errorf("Total for trace t3 = %d, want 1", total)
	}

	// Pagination
	results, _, err = store.ListMappingDecisions(ctx, ListMappingDecisionsFilter{Limit: 2})
	if err != nil {
		t.Fatalf("List with pagination failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Paginated = %d, want 2", len(results))
	}
}

func TestMappingStore_GetDecisionStats(t *testing.T) {
	store, ctx := initStoreForTest(t)

	since := time.Now().Add(-1 * time.Hour)

	// Record decisions of various types
	types := []DecisionType{
		DecisionPersistentHit,
		DecisionPersistentHit,
		DecisionAutorouteHighConf,
		DecisionNoMatch,
	}

	for i, dt := range types {
		var conf *float64
		if dt != DecisionNoMatch {
			c := 0.80 + float64(i)*0.05
			conf = &c
		}
		d := &MappingDecision{
			TraceID:      fmt.Sprintf("stat-%d", i),
			SourceSystem: "epic_labs",
			SourceCode:   fmt.Sprintf("S%d", i),
			TargetSystem: "http://loinc.org",
			DecisionType: dt,
			Confidence:   conf,
			DurationMs:   100 + i*50,
		}
		if err := store.RecordMappingDecision(ctx, d); err != nil {
			t.Fatalf("Record %d failed: %v", i, err)
		}
	}

	until := time.Now().Add(1 * time.Hour)
	stats, err := store.GetDecisionStats(ctx, since, until)
	if err != nil {
		t.Fatalf("GetDecisionStats failed: %v", err)
	}

	if stats.TotalDecisions != 4 {
		t.Errorf("TotalDecisions = %d, want 4", stats.TotalDecisions)
	}
	if stats.DecisionsByType[DecisionPersistentHit] != 2 {
		t.Errorf("PERSISTENT_HIT count = %d, want 2", stats.DecisionsByType[DecisionPersistentHit])
	}
	if stats.DecisionsByType[DecisionAutorouteHighConf] != 1 {
		t.Errorf("AUTOROUTE_HIGH_CONF count = %d, want 1", stats.DecisionsByType[DecisionAutorouteHighConf])
	}
	if stats.DecisionsByType[DecisionNoMatch] != 1 {
		t.Errorf("NO_MATCH count = %d, want 1", stats.DecisionsByType[DecisionNoMatch])
	}
	if stats.AvgDurationMs <= 0 {
		t.Error("Expected positive AvgDurationMs")
	}
	if stats.AvgConfidence == nil {
		t.Error("Expected AvgConfidence to be set (3 of 4 have confidence)")
	}
}

func TestMappingStore_CleanupOldDecisions(t *testing.T) {
	store, ctx := initStoreForTest(t)

	// Record a decision
	d := &MappingDecision{
		TraceID:      "cleanup-1",
		SourceSystem: "epic_labs",
		SourceCode:   "CL001",
		TargetSystem: "http://loinc.org",
		DecisionType: DecisionPersistentHit,
		DurationMs:   50,
	}
	if err := store.RecordMappingDecision(ctx, d); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	// Cleanup with short duration should not delete recent decisions
	deleted, err := store.CleanupOldDecisions(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("Deleted = %d, want 0 (decision is recent)", deleted)
	}

	// Backdate the decision by direct SQL
	_, err = store.db.ExecContext(ctx, `
		UPDATE terminology.mapping_decisions
		SET created_at = created_at - INTERVAL '100 days'
		WHERE id = $1
	`, d.ID)
	if err != nil {
		t.Fatalf("Backdate failed: %v", err)
	}

	// Now cleanup should remove it
	deleted, err = store.CleanupOldDecisions(ctx, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("Cleanup after backdate failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Deleted = %d, want 1", deleted)
	}

	// Verify it's gone
	got, err := store.GetMappingDecision(ctx, d.ID)
	if err != nil {
		t.Fatalf("Get after cleanup failed: %v", err)
	}
	if got != nil {
		t.Error("Expected decision to be deleted")
	}
}
