package index

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// newTestBuilder creates a Builder with mock dependencies.
func newTestBuilder() (*Builder, *MockQdrantClient, *llm.MockEmbeddingClient) {
	mockQ := NewMockQdrantClient()
	mockE := llm.NewMockEmbeddingClient()
	b := &Builder{
		config: IndexConfig{
			BatchSize:           2, // Small batches for testing
			EmbeddingDimensions: 4,
		},
		qdrant:          mockQ,
		embeddingClient: mockE,
		progressChan:    make(chan BuildProgress, 100),
	}
	return b, mockQ, mockE
}

// writeTempCSV creates a temporary CSV file and returns its path.
func writeTempCSV(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp CSV: %v", err)
	}
	return path
}

func TestBuilder_Progress(t *testing.T) {
	b, _, _ := newTestBuilder()
	ch := b.Progress()
	if ch == nil {
		t.Fatal("expected non-nil progress channel")
	}
}

func TestBuilder_Build_LOINC(t *testing.T) {
	b, mockQ, _ := newTestBuilder()
	ctx := context.Background()

	csv := `LOINC_NUM,COMPONENT,PROPERTY,TIME_ASPCT,SYSTEM,SCALE_TYP,METHOD_TYP,SHORTNAME,LONG_COMMON_NAME,CONSUMER_NAME,RELATEDNAMES2,STATUS
2345-7,Glucose,MCnc,Pt,Ser/Plas,Qn,,Glucose SerPl-mCnc,Glucose [Mass/volume] in Serum or Plasma,Glucose Blood,Blood sugar,ACTIVE
789-8,Erythrocytes,NCnc,Pt,Bld,Qn,,RBC # Bld Auto,Erythrocytes [#/volume] in Blood,Red Blood Cells,,ACTIVE
`
	path := writeTempCSV(t, "loinc.csv", csv)

	var progressUpdates []BuildProgress
	opts := BuildOptions{
		SourcePath:   path,
		Vocabulary:   VocabularyLOINC,
		Version:      "2.77",
		DropExisting: false,
		OnProgress: func(p BuildProgress) {
			progressUpdates = append(progressUpdates, p)
		},
	}

	if err := b.Build(ctx, opts); err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// Verify collection was created
	if _, ok := mockQ.Collections["fi_fhir_idx_loinc"]; !ok {
		t.Error("expected collection to be created")
	}

	// Verify progress updates
	if len(progressUpdates) == 0 {
		t.Fatal("expected progress updates")
	}

	last := progressUpdates[len(progressUpdates)-1]
	if last.Status != BuildStatusCompleted {
		t.Errorf("final status=%q want completed", last.Status)
	}
	if last.TotalItems != 2 {
		t.Errorf("TotalItems=%d want 2", last.TotalItems)
	}
	if last.IndexedItems != 2 {
		t.Errorf("IndexedItems=%d want 2", last.IndexedItems)
	}
}

func TestBuilder_Build_DropExisting(t *testing.T) {
	b, mockQ, _ := newTestBuilder()
	ctx := context.Background()

	// Pre-create collection
	mockQ.AddMockCollection("fi_fhir_idx_loinc", 1024, []Point{{ID: "old"}})

	csv := `LOINC_NUM,COMPONENT,STATUS
2345-7,Glucose,ACTIVE
`
	path := writeTempCSV(t, "loinc.csv", csv)

	opts := BuildOptions{
		SourcePath:   path,
		Vocabulary:   VocabularyLOINC,
		DropExisting: true,
	}

	if err := b.Build(ctx, opts); err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// Verify old collection was deleted and recreated
	if len(mockQ.Calls.DeleteCollection) != 1 {
		t.Errorf("DeleteCollection calls=%d want 1", len(mockQ.Calls.DeleteCollection))
	}
}

func TestBuilder_Build_DropExisting_NotFoundIgnored(t *testing.T) {
	b, mockQ, _ := newTestBuilder()
	ctx := context.Background()

	// DeleteCollection returns "not found" style error
	mockQ.Errors.DeleteCollection = errors.New("404 not found")

	csv := `LOINC_NUM,COMPONENT,STATUS
2345-7,Glucose,ACTIVE
`
	path := writeTempCSV(t, "loinc.csv", csv)

	opts := BuildOptions{
		SourcePath:   path,
		Vocabulary:   VocabularyLOINC,
		DropExisting: true,
	}

	// Should succeed even though DeleteCollection returned 404
	if err := b.Build(ctx, opts); err != nil {
		t.Fatalf("Build error: %v", err)
	}
}

func TestBuilder_Build_DropExisting_RealError(t *testing.T) {
	b, mockQ, _ := newTestBuilder()

	mockQ.Errors.DeleteCollection = errors.New("permission denied")

	csv := `LOINC_NUM,COMPONENT,STATUS
2345-7,Glucose,ACTIVE
`
	path := writeTempCSV(t, "loinc.csv", csv)

	opts := BuildOptions{
		SourcePath:   path,
		Vocabulary:   VocabularyLOINC,
		DropExisting: true,
	}

	if err := b.Build(context.Background(), opts); err == nil {
		t.Fatal("expected error on real delete failure")
	}
}

func TestBuilder_Build_CollectionExistsError(t *testing.T) {
	b, mockQ, _ := newTestBuilder()
	mockQ.Errors.CollectionExists = errors.New("qdrant down")

	csv := `LOINC_NUM,STATUS
2345-7,ACTIVE
`
	path := writeTempCSV(t, "loinc.csv", csv)

	opts := BuildOptions{
		SourcePath: path,
		Vocabulary: VocabularyLOINC,
	}

	if err := b.Build(context.Background(), opts); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuilder_Build_CreateCollectionError(t *testing.T) {
	b, mockQ, _ := newTestBuilder()
	mockQ.Errors.CreateCollection = errors.New("quota exceeded")

	csv := `LOINC_NUM,STATUS
2345-7,ACTIVE
`
	path := writeTempCSV(t, "loinc.csv", csv)

	opts := BuildOptions{
		SourcePath: path,
		Vocabulary: VocabularyLOINC,
	}

	if err := b.Build(context.Background(), opts); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuilder_Build_LoadEntriesError(t *testing.T) {
	b, _, _ := newTestBuilder()

	opts := BuildOptions{
		SourcePath: "/nonexistent/file.csv",
		Vocabulary: VocabularyLOINC,
	}

	if err := b.Build(context.Background(), opts); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestBuilder_Build_EmbeddingError_Continues(t *testing.T) {
	b, _, mockE := newTestBuilder()
	ctx := context.Background()

	mockE.EmbedFunc = func(ctx context.Context, texts []string) ([][]float64, error) {
		return nil, errors.New("embedding unavailable")
	}

	csv := `LOINC_NUM,COMPONENT,STATUS
2345-7,Glucose,ACTIVE
789-8,RBC,ACTIVE
`
	path := writeTempCSV(t, "loinc.csv", csv)

	var progressUpdates []BuildProgress
	opts := BuildOptions{
		SourcePath: path,
		Vocabulary: VocabularyLOINC,
		OnProgress: func(p BuildProgress) {
			progressUpdates = append(progressUpdates, p)
		},
	}

	// Should still complete (errors are non-fatal for individual batches)
	if err := b.Build(ctx, opts); err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// But should have recorded errors
	last := progressUpdates[len(progressUpdates)-1]
	if len(last.Errors) == 0 {
		t.Error("expected errors in progress")
	}
}

func TestBuilder_Build_UpsertError_Continues(t *testing.T) {
	b, mockQ, _ := newTestBuilder()
	ctx := context.Background()

	mockQ.Errors.UpsertPoints = errors.New("disk full")

	csv := `LOINC_NUM,COMPONENT,STATUS
2345-7,Glucose,ACTIVE
`
	path := writeTempCSV(t, "loinc.csv", csv)

	var progressUpdates []BuildProgress
	opts := BuildOptions{
		SourcePath: path,
		Vocabulary: VocabularyLOINC,
		OnProgress: func(p BuildProgress) {
			progressUpdates = append(progressUpdates, p)
		},
	}

	if err := b.Build(ctx, opts); err != nil {
		t.Fatalf("Build error: %v", err)
	}

	last := progressUpdates[len(progressUpdates)-1]
	if len(last.Errors) == 0 {
		t.Error("expected upsert errors")
	}
}

func TestBuilder_Build_ContextCanceled(t *testing.T) {
	b, _, _ := newTestBuilder()

	csv := `LOINC_NUM,COMPONENT,STATUS
2345-7,Glucose,ACTIVE
789-8,RBC,ACTIVE
111-1,Test,ACTIVE
`
	path := writeTempCSV(t, "loinc.csv", csv)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	opts := BuildOptions{
		SourcePath: path,
		Vocabulary: VocabularyLOINC,
	}

	err := b.Build(ctx, opts)
	if err == nil {
		t.Fatal("expected context canceled error")
	}
}

func TestBuilder_Build_SNOMED(t *testing.T) {
	b, _, _ := newTestBuilder()
	ctx := context.Background()

	tsv := "id\tfsn\tterm\tsynonyms\tsemantictag\tactive\n" +
		"387517004\tParacetamol (substance)\tParacetamol\tAcetaminophen;Tylenol\tsubstance\t1\n" +
		"12345\tTest (finding)\tTest\t\tfinding\t1\n"
	path := writeTempCSV(t, "snomed.tsv", tsv)

	opts := BuildOptions{
		SourcePath: path,
		Vocabulary: VocabularySNOMED,
	}

	if err := b.Build(ctx, opts); err != nil {
		t.Fatalf("Build error: %v", err)
	}
}

func TestBuilder_Build_ICD10(t *testing.T) {
	b, _, _ := newTestBuilder()
	ctx := context.Background()

	csv := `code,short_description,long_description,category,valid
E11.9,Type 2 DM without complications,Type 2 diabetes mellitus without complications,E11,1
J06.9,Acute upper resp infection,Acute upper respiratory infection unspecified,J06,1
`
	path := writeTempCSV(t, "icd10.csv", csv)

	opts := BuildOptions{
		SourcePath: path,
		Vocabulary: VocabularyICD10CM,
	}

	if err := b.Build(ctx, opts); err != nil {
		t.Fatalf("Build error: %v", err)
	}
}

func TestBuilder_Build_ExistingCollection(t *testing.T) {
	b, mockQ, _ := newTestBuilder()
	ctx := context.Background()

	// Pre-create the collection
	mockQ.AddMockCollection("fi_fhir_idx_loinc", 1024, nil)

	csv := `LOINC_NUM,COMPONENT,STATUS
2345-7,Glucose,ACTIVE
`
	path := writeTempCSV(t, "loinc.csv", csv)

	opts := BuildOptions{
		SourcePath: path,
		Vocabulary: VocabularyLOINC,
	}

	if err := b.Build(ctx, opts); err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// Should NOT have created a new collection
	if len(mockQ.Calls.CreateCollection) != 0 {
		t.Errorf("CreateCollection calls=%d, should skip existing", len(mockQ.Calls.CreateCollection))
	}
}

func TestBuilder_loadEntries_UnsupportedVocabulary(t *testing.T) {
	b, _, _ := newTestBuilder()
	_, err := b.loadEntries("/tmp/test.csv", Vocabulary("unknown"))
	if err == nil {
		t.Fatal("expected error for unsupported vocabulary")
	}
}

func TestBuilder_loadLOINCEntries(t *testing.T) {
	t.Run("parses valid CSV", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		csv := `LOINC_NUM,COMPONENT,PROPERTY,TIME_ASPCT,SYSTEM,SCALE_TYP,METHOD_TYP,SHORTNAME,LONG_COMMON_NAME,CONSUMER_NAME,RELATEDNAMES2,STATUS
2345-7,Glucose,MCnc,Pt,Ser/Plas,Qn,,Glucose SerPl-mCnc,Glucose [Mass/volume] in Serum or Plasma,Glucose Blood,Blood sugar,ACTIVE
`
		path := writeTempCSV(t, "loinc.csv", csv)

		entries, err := b.loadLOINCEntries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("entries=%d want 1", len(entries))
		}
		if entries[0].Code != "2345-7" {
			t.Errorf("Code=%q", entries[0].Code)
		}
		if entries[0].System != "http://loinc.org" {
			t.Errorf("System=%q", entries[0].System)
		}
	})

	t.Run("skips deprecated codes", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		csv := `LOINC_NUM,COMPONENT,STATUS
2345-7,Glucose,ACTIVE
9999-0,Old,DEPRECATED
8888-0,Bad,DISCOURAGED
`
		path := writeTempCSV(t, "loinc.csv", csv)

		entries, err := b.loadLOINCEntries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("entries=%d want 1 (only ACTIVE)", len(entries))
		}
	})

	t.Run("skips empty codes", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		csv := `LOINC_NUM,COMPONENT,STATUS
,NoCode,ACTIVE
2345-7,Glucose,ACTIVE
`
		path := writeTempCSV(t, "loinc.csv", csv)

		entries, err := b.loadLOINCEntries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("entries=%d want 1", len(entries))
		}
	})

	t.Run("missing required column", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		csv := `COMPONENT,STATUS
Glucose,ACTIVE
`
		path := writeTempCSV(t, "loinc.csv", csv)

		_, err := b.loadLOINCEntries(path)
		if err == nil {
			t.Fatal("expected error for missing LOINC_NUM column")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		b, _, _ := newTestBuilder()
		_, err := b.loadLOINCEntries("/nonexistent/file.csv")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		b, _, _ := newTestBuilder()
		path := writeTempCSV(t, "empty.csv", "")
		_, err := b.loadLOINCEntries(path)
		if err == nil {
			t.Fatal("expected error for empty file")
		}
	})
}

func TestBuilder_loadSNOMEDEntries(t *testing.T) {
	t.Run("simplified format", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		tsv := "id\tfsn\tterm\tsynonyms\tsemantictag\tactive\n" +
			"387517004\tParacetamol (substance)\tParacetamol\tAcetaminophen;Tylenol\tsubstance\t1\n" +
			"12345\tInactive (finding)\tInactive\t\tfinding\t0\n"
		path := writeTempCSV(t, "snomed.tsv", tsv)

		entries, err := b.loadSNOMEDEntries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("entries=%d want 1 (inactive skipped)", len(entries))
		}
		if entries[0].Code != "387517004" {
			t.Errorf("Code=%q", entries[0].Code)
		}
	})

	t.Run("simplified with true active", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		// Note: active column uses "True" (mixed case) — code checks strings.ToLower
		tsv := "id\tfsn\tterm\tsynonyms\tsemantictag\tactive\n" +
			"111\tTest (finding)\tTest\tAlternate\tfinding\tTrue\n"
		path := writeTempCSV(t, "snomed.tsv", tsv)

		entries, err := b.loadSNOMEDEntries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("entries=%d want 1", len(entries))
		}
	})

	t.Run("simplified with numeric active", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		tsv := "id\tfsn\tterm\tsemantictag\tactive\n" +
			"222\tAnother (finding)\tAnother\tfinding\t1\n"
		path := writeTempCSV(t, "snomed.tsv", tsv)

		entries, err := b.loadSNOMEDEntries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("entries=%d want 1", len(entries))
		}
	})

	t.Run("skips empty concept ID", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		tsv := "id\tfsn\tterm\tactive\n" +
			"\tNoID (finding)\tNoID\t1\n"
		path := writeTempCSV(t, "snomed.tsv", tsv)

		entries, err := b.loadSNOMEDEntries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("entries=%d want 0", len(entries))
		}
	})

	t.Run("RF2 format", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		// RF2 descriptions have effectiveTime column as signal
		tsv := "id\teffectiveTime\tactive\tmoduleId\tconceptId\tlanguageCode\ttypeId\tterm\tcaseSignificanceId\n" +
			"1001\t20230101\t1\t900000000000207008\t387517004\ten\t900000000000003001\tParacetamol (substance)\t900000000000448009\n" +
			"1002\t20230101\t1\t900000000000207008\t387517004\ten\t900000000000013009\tAcetaminophen\t900000000000448009\n" +
			"1003\t20230101\t0\t900000000000207008\t99999\ten\t900000000000003001\tInactive (finding)\t900000000000448009\n"
		path := writeTempCSV(t, "snomed_rf2.tsv", tsv)

		entries, err := b.loadSNOMEDEntries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("entries=%d want 1 (one concept with 2 active descriptions)", len(entries))
		}
		if entries[0].Code != "387517004" {
			t.Errorf("Code=%q", entries[0].Code)
		}
	})

	t.Run("RF2 without synonyms", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		tsv := "id\teffectiveTime\tactive\tmoduleId\tconceptId\tlanguageCode\ttypeId\tterm\tcaseSignificanceId\n" +
			"1001\t20230101\t1\t900000000000207008\t12345\ten\t900000000000003001\tTest (finding)\t900000000000448009\n"
		path := writeTempCSV(t, "snomed_rf2.tsv", tsv)

		entries, err := b.loadSNOMEDEntries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("entries=%d want 1", len(entries))
		}
		// When there are no synonyms, description falls back to FSN
		if entries[0].Display != "Test (finding)" {
			t.Errorf("Display=%q want FSN fallback", entries[0].Display)
		}
	})

	t.Run("RF2 multiple concepts merged", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		tsv := "id\teffectiveTime\tactive\tmoduleId\tconceptId\tlanguageCode\ttypeId\tterm\tcaseSignificanceId\n" +
			"1001\t20230101\t1\t900000000000207008\t111\ten\t900000000000003001\tAlpha (finding)\t900000000000448009\n" +
			"1002\t20230101\t1\t900000000000207008\t111\ten\t900000000000013009\tAlpha synonym\t900000000000448009\n" +
			"1003\t20230101\t1\t900000000000207008\t222\ten\t900000000000003001\tBeta (procedure)\t900000000000448009\n"
		path := writeTempCSV(t, "snomed_rf2.tsv", tsv)

		entries, err := b.loadSNOMEDEntries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("entries=%d want 2 (two concepts)", len(entries))
		}
	})

	t.Run("missing file", func(t *testing.T) {
		b, _, _ := newTestBuilder()
		_, err := b.loadSNOMEDEntries("/nonexistent/file.tsv")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestBuilder_loadICD10Entries(t *testing.T) {
	t.Run("parses valid CSV", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		csv := `code,short_description,long_description,category,valid
E11.9,Type 2 DM,Type 2 diabetes mellitus without complications,E11,1
`
		path := writeTempCSV(t, "icd10.csv", csv)

		entries, err := b.loadICD10Entries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("entries=%d want 1", len(entries))
		}
		if entries[0].Code != "E11.9" {
			t.Errorf("Code=%q", entries[0].Code)
		}
	})

	t.Run("alternate column names", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		csv := `icd10cm,description,valid
E11.9,Type 2 DM,1
`
		path := writeTempCSV(t, "icd10.csv", csv)

		entries, err := b.loadICD10Entries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("entries=%d want 1", len(entries))
		}
		if entries[0].Code != "E11.9" {
			t.Errorf("Code=%q (alternate column)", entries[0].Code)
		}
	})

	t.Run("skips empty code", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		csv := `code,short_description,valid
,Empty,1
E11.9,Valid,1
`
		path := writeTempCSV(t, "icd10.csv", csv)

		entries, err := b.loadICD10Entries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("entries=%d want 1", len(entries))
		}
	})

	t.Run("valid defaults to true when empty", func(t *testing.T) {
		b, _, _ := newTestBuilder()

		csv := `code,description,valid
E11.9,Type 2 DM,
`
		path := writeTempCSV(t, "icd10.csv", csv)

		entries, err := b.loadICD10Entries(path)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("entries=%d want 1 (empty valid defaults to true)", len(entries))
		}
	})

	t.Run("missing file", func(t *testing.T) {
		b, _, _ := newTestBuilder()
		_, err := b.loadICD10Entries("/nonexistent/file.csv")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestGetCSVCol(t *testing.T) {
	colIdx := map[string]int{"name": 0, "code": 1}
	record := []string{"Alice", "A01"}

	if got := getCSVCol(record, colIdx, "name"); got != "Alice" {
		t.Errorf("got=%q want Alice", got)
	}
	if got := getCSVCol(record, colIdx, "code"); got != "A01" {
		t.Errorf("got=%q want A01", got)
	}
	if got := getCSVCol(record, colIdx, "missing"); got != "" {
		t.Errorf("got=%q want empty for missing column", got)
	}

	// Out of bounds index
	smallRecord := []string{"only_one"}
	if got := getCSVCol(smallRecord, colIdx, "code"); got != "" {
		t.Errorf("got=%q want empty for out of bounds", got)
	}
}

func TestBuilder_Build_DefaultBatchSize(t *testing.T) {
	mockQ := NewMockQdrantClient()
	mockE := llm.NewMockEmbeddingClient()
	b := &Builder{
		config: IndexConfig{
			BatchSize: 0, // Should default to 32
		},
		qdrant:          mockQ,
		embeddingClient: mockE,
		progressChan:    make(chan BuildProgress, 100),
	}

	csv := `LOINC_NUM,COMPONENT,STATUS
2345-7,Glucose,ACTIVE
`
	path := writeTempCSV(t, "loinc.csv", csv)

	opts := BuildOptions{
		SourcePath: path,
		Vocabulary: VocabularyLOINC,
	}

	if err := b.Build(context.Background(), opts); err != nil {
		t.Fatalf("Build error: %v", err)
	}
}
