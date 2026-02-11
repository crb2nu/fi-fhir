//go:build integration

package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUMLSLoader_Integration_LoadMETA_Defaults(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	if _, err := migrator.Initialize(ctx); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	metaDir := t.TempDir()
	writeUMLSTestMETA(t, metaDir)

	loader := NewUMLSLoader(tc.DB)
	releaseDate := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	// opts=nil exercises the default option behavior (EnglishOnly + SkipSuppressed).
	res, err := loader.LoadMETA(ctx, metaDir, "2024AB-test", &releaseDate, nil, nil)
	if err != nil {
		t.Fatalf("LoadMETA: %v", err)
	}

	if res.ReleaseID == 0 {
		t.Fatalf("expected non-zero release id")
	}
	if res.ConceptsLoaded == 0 {
		t.Fatalf("expected concepts loaded > 0")
	}
	if res.RelationsLoaded == 0 {
		t.Fatalf("expected relations loaded > 0")
	}
	if res.SemTypesLoaded == 0 {
		t.Fatalf("expected semantic types loaded > 0")
	}

	active, err := migrator.GetActiveRelease(ctx, VocabUMLS)
	if err != nil {
		t.Fatalf("GetActiveRelease: %v", err)
	}
	if active == nil || active.Version != "2024AB-test" {
		t.Fatalf("expected active UMLS release version 2024AB-test, got %+v", active)
	}
}

func TestUMLSLoader_Integration_LoadMETA_FilterSources(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	if _, err := migrator.Initialize(ctx); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	metaDir := t.TempDir()
	writeUMLSTestMETA(t, metaDir)

	loader := NewUMLSLoader(tc.DB)
	opts := &UMLSLoadOptions{
		FilterSources:  []string{"SNOMEDCT_US"},
		EnglishOnly:    true,
		SkipSuppressed: true,
	}
	res, err := loader.LoadMETA(ctx, metaDir, "2024AB-snomed-only", nil, opts, nil)
	if err != nil {
		t.Fatalf("LoadMETA (filtered): %v", err)
	}

	if res.ConceptsLoaded != 2 {
		t.Fatalf("expected 2 SNOMEDCT_US concepts loaded from sample, got %d", res.ConceptsLoaded)
	}
	if res.RelationsLoaded != 1 {
		t.Fatalf("expected 1 SNOMEDCT_US relation loaded from sample, got %d", res.RelationsLoaded)
	}
	if res.SemTypesLoaded != 1 {
		t.Fatalf("expected 1 SNOMEDCT_US semantic type loaded from sample, got %d", res.SemTypesLoaded)
	}
}

func TestUMLSLoader_Integration_LoadMETA_SkipRelationsAndSemanticTypes(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	if _, err := migrator.Initialize(ctx); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	metaDir := t.TempDir()
	writeUMLSTestMETA(t, metaDir)

	loader := NewUMLSLoader(tc.DB)
	opts := &UMLSLoadOptions{
		EnglishOnly:       true,
		SkipSuppressed:    true,
		SkipRelations:     true,
		SkipSemanticTypes: true,
	}
	res, err := loader.LoadMETA(ctx, metaDir, "2024AB-concepts-only", nil, opts, nil)
	if err != nil {
		t.Fatalf("LoadMETA (skip rel/sty): %v", err)
	}

	if res.ConceptsLoaded == 0 {
		t.Fatalf("expected concepts loaded > 0")
	}
	if res.RelationsLoaded != 0 {
		t.Fatalf("expected relations loaded = 0, got %d", res.RelationsLoaded)
	}
	if res.SemTypesLoaded != 0 {
		t.Fatalf("expected semtypes loaded = 0, got %d", res.SemTypesLoaded)
	}
}

func writeUMLSTestMETA(t *testing.T, metaDir string) {
	t.Helper()

	// Copy small fixtures into a META-like directory layout.
	mustCopyFile(t, getTestDataPath(t, "MRCONSO_sample.RRF"), filepath.Join(metaDir, "MRCONSO.RRF"))
	mustCopyFile(t, getTestDataPath(t, "MRREL_sample.RRF"), filepath.Join(metaDir, "MRREL.RRF"))
	mustCopyFile(t, getTestDataPath(t, "MRSTY_sample.RRF"), filepath.Join(metaDir, "MRSTY.RRF"))
}

func mustCopyFile(t *testing.T, src, dst string) {
	t.Helper()

	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}
