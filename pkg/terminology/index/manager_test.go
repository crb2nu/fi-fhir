package index

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// newTestManager creates a Manager with mock dependencies for testing.
func newTestManager() (*Manager, *MockQdrantClient, *llm.MockEmbeddingClient) {
	mockQ := NewMockQdrantClient()
	mockE := llm.NewMockEmbeddingClient()
	m := &Manager{
		config: IndexConfig{
			EmbeddingModel: "test-model",
		},
		qdrant:          mockQ,
		embeddingClient: mockE,
	}
	return m, mockQ, mockE
}

func TestManager_Search(t *testing.T) {
	t.Run("returns results", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()
		ctx := context.Background()

		mockQ.AddMockCollection("fi_fhir_idx_loinc", 1024, []Point{
			{ID: "loinc:2345-7", Payload: map[string]interface{}{
				"code": "2345-7", "system": "http://loinc.org",
				"display": "Glucose", "vocabulary": "loinc",
			}},
		})

		results, err := mgr.Search(ctx, VocabularyLOINC, "glucose", 5)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("results=%d want 1", len(results))
		}
		if results[0].Entry.Code != "2345-7" {
			t.Errorf("Code=%q", results[0].Entry.Code)
		}
		if results[0].Entry.System != "http://loinc.org" {
			t.Errorf("System=%q", results[0].Entry.System)
		}
		if results[0].Entry.Display != "Glucose" {
			t.Errorf("Display=%q", results[0].Entry.Display)
		}
	})

	t.Run("default limit", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()

		mockQ.WithSearchResults([]SearchHit{
			{ID: "loinc:1", Score: 0.9, Payload: map[string]interface{}{"code": "1", "vocabulary": "loinc"}},
		})

		results, err := mgr.Search(context.Background(), VocabularyLOINC, "test", 0)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("results=%d", len(results))
		}
		// Verify default limit was applied (10)
		if len(mockQ.Calls.Search) != 1 || mockQ.Calls.Search[0].Limit != 10 {
			t.Errorf("limit=%d want 10", mockQ.Calls.Search[0].Limit)
		}
	})

	t.Run("embedding error", func(t *testing.T) {
		mgr, _, mockE := newTestManager()
		mockE.EmbedSingleFunc = func(ctx context.Context, text string) ([]float64, error) {
			return nil, errors.New("embedding failed")
		}

		_, err := mgr.Search(context.Background(), VocabularyLOINC, "test", 5)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("search error", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()
		mockQ.Errors.Search = errors.New("qdrant down")

		_, err := mgr.Search(context.Background(), VocabularyLOINC, "test", 5)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestManager_SearchAll(t *testing.T) {
	t.Run("merges results from multiple vocabularies", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()
		ctx := context.Background()

		// Add two collections
		mockQ.AddMockCollection("fi_fhir_idx_loinc", 1024, []Point{
			{ID: "loinc:1", Payload: map[string]interface{}{
				"code": "1", "display": "LOINC Result", "vocabulary": "loinc",
			}},
		})
		mockQ.AddMockCollection("fi_fhir_idx_icd10cm", 1024, []Point{
			{ID: "icd10cm:E11", Payload: map[string]interface{}{
				"code": "E11", "display": "Diabetes", "vocabulary": "icd10cm",
			}},
		})

		results, err := mgr.SearchAll(ctx, "diabetes", 10)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(results) < 2 {
			t.Errorf("results=%d want >=2", len(results))
		}
	})

	t.Run("default limit", func(t *testing.T) {
		mgr, _, _ := newTestManager()
		results, err := mgr.SearchAll(context.Background(), "test", 0)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		// No collections exist, so no results
		if len(results) != 0 {
			t.Errorf("results=%d want 0", len(results))
		}
	})

	t.Run("limits total results", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()

		// Add many points to one collection
		var points []Point
		for i := 0; i < 20; i++ {
			points = append(points, Point{
				ID:      "loinc:" + string(rune('A'+i)),
				Payload: map[string]interface{}{"code": string(rune('A' + i)), "vocabulary": "loinc"},
			})
		}
		mockQ.AddMockCollection("fi_fhir_idx_loinc", 1024, points)

		results, err := mgr.SearchAll(context.Background(), "test", 5)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(results) > 5 {
			t.Errorf("results=%d want <=5", len(results))
		}
	})

	t.Run("embedding error", func(t *testing.T) {
		mgr, _, mockE := newTestManager()
		mockE.EmbedSingleFunc = func(ctx context.Context, text string) ([]float64, error) {
			return nil, errors.New("embedding failed")
		}

		_, err := mgr.SearchAll(context.Background(), "test", 5)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("sorts by score descending", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()

		// Use custom SearchFunc to control scores
		mockQ.SearchFunc = func(ctx context.Context, collection string, vector []float64, limit int, scoreThreshold float64) ([]SearchHit, error) {
			if collection == "fi_fhir_idx_loinc" {
				return []SearchHit{
					{ID: "loinc:1", Score: 0.5, Payload: map[string]interface{}{"code": "1", "vocabulary": "loinc"}},
				}, nil
			}
			if collection == "fi_fhir_idx_icd10cm" {
				return []SearchHit{
					{ID: "icd10:2", Score: 0.9, Payload: map[string]interface{}{"code": "2", "vocabulary": "icd10cm"}},
				}, nil
			}
			return nil, nil
		}

		// Mark collections as existing
		mockQ.AddMockCollection("fi_fhir_idx_loinc", 1024, nil)
		mockQ.AddMockCollection("fi_fhir_idx_icd10cm", 1024, nil)

		results, err := mgr.SearchAll(context.Background(), "test", 10)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("results=%d want 2", len(results))
		}
		// Higher score should be first
		if results[0].Score < results[1].Score {
			t.Errorf("results not sorted: [0].Score=%f [1].Score=%f", results[0].Score, results[1].Score)
		}
	})
}

func TestManager_GetStats(t *testing.T) {
	t.Run("returns stats", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()

		mockQ.AddMockCollection("fi_fhir_idx_loinc", 1024, []Point{
			{ID: "p1"}, {ID: "p2"}, {ID: "p3"},
		})

		stats, err := mgr.GetStats(context.Background(), VocabularyLOINC)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if stats.Vocabulary != VocabularyLOINC {
			t.Errorf("Vocabulary=%q", stats.Vocabulary)
		}
		if stats.Collection != "fi_fhir_idx_loinc" {
			t.Errorf("Collection=%q", stats.Collection)
		}
		if stats.TotalEntries != 3 {
			t.Errorf("TotalEntries=%d", stats.TotalEntries)
		}
		if stats.EmbeddingModel != "test-model" {
			t.Errorf("EmbeddingModel=%q", stats.EmbeddingModel)
		}
		if stats.Dimensions != 1024 {
			t.Errorf("Dimensions=%d", stats.Dimensions)
		}
	})

	t.Run("error", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()
		mockQ.Errors.GetCollectionInfo = errors.New("fail")

		_, err := mgr.GetStats(context.Background(), VocabularyLOINC)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestManager_GetAllStats(t *testing.T) {
	t.Run("returns stats for existing collections", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()

		mockQ.AddMockCollection("fi_fhir_idx_loinc", 1024, []Point{{ID: "p1"}})
		mockQ.AddMockCollection("fi_fhir_idx_icd10cm", 1024, []Point{{ID: "p2"}, {ID: "p3"}})

		stats, err := mgr.GetAllStats(context.Background())
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(stats) != 2 {
			t.Fatalf("stats=%d want 2", len(stats))
		}
	})

	t.Run("skips non-existent collections", func(t *testing.T) {
		mgr, _, _ := newTestManager()

		stats, err := mgr.GetAllStats(context.Background())
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(stats) != 0 {
			t.Errorf("stats=%d want 0", len(stats))
		}
	})
}

func TestManager_Lookup(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()

		mockQ.AddMockCollection("fi_fhir_idx_loinc", 1024, []Point{
			{ID: "loinc:2345-7", Payload: map[string]interface{}{
				"code": "2345-7", "system": "http://loinc.org",
				"display": "Glucose", "vocabulary": "loinc",
			}},
		})

		entry, err := mgr.Lookup(context.Background(), VocabularyLOINC, "2345-7")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if entry == nil {
			t.Fatal("expected entry")
		}
		if entry.Code != "2345-7" {
			t.Errorf("Code=%q", entry.Code)
		}
		if entry.Display != "Glucose" {
			t.Errorf("Display=%q", entry.Display)
		}
	})

	t.Run("not found", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()
		mockQ.AddMockCollection("fi_fhir_idx_loinc", 1024, nil)

		entry, err := mgr.Lookup(context.Background(), VocabularyLOINC, "99999")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if entry != nil {
			t.Error("expected nil for not found")
		}
	})

	t.Run("error", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()
		mockQ.Errors.GetPoints = errors.New("fail")

		_, err := mgr.Lookup(context.Background(), VocabularyLOINC, "2345-7")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestManager_DeleteIndex(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()
		mockQ.AddMockCollection("fi_fhir_idx_loinc", 1024, nil)

		if err := mgr.DeleteIndex(context.Background(), VocabularyLOINC); err != nil {
			t.Fatalf("error: %v", err)
		}
		if _, ok := mockQ.Collections["fi_fhir_idx_loinc"]; ok {
			t.Error("collection should be deleted")
		}
	})

	t.Run("error", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()
		mockQ.Errors.DeleteCollection = errors.New("fail")

		if err := mgr.DeleteIndex(context.Background(), VocabularyLOINC); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestManager_Ready(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mgr, _, _ := newTestManager()
		if err := mgr.Ready(context.Background()); err != nil {
			t.Fatalf("error: %v", err)
		}
	})

	t.Run("error", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()
		mockQ.Errors.Ping = errors.New("connection refused")

		if err := mgr.Ready(context.Background()); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestManager_WaitForReady(t *testing.T) {
	t.Run("immediately ready", func(t *testing.T) {
		mgr, _, _ := newTestManager()

		err := mgr.WaitForReady(context.Background(), 5*time.Second)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	})

	t.Run("becomes ready after retries", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()
		calls := 0
		originalPing := mockQ.Errors.Ping
		_ = originalPing
		// Fail first call, succeed on second
		mockQ.Errors.Ping = errors.New("not ready")

		go func() {
			time.Sleep(1500 * time.Millisecond)
			mockQ.Errors.Ping = nil
			calls++
		}()

		err := mgr.WaitForReady(context.Background(), 5*time.Second)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()
		mockQ.Errors.Ping = errors.New("never ready")

		err := mgr.WaitForReady(context.Background(), 100*time.Millisecond)
		if err == nil {
			t.Fatal("expected timeout error")
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		mgr, mockQ, _ := newTestManager()
		mockQ.Errors.Ping = errors.New("not ready")

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := mgr.WaitForReady(ctx, 5*time.Second)
		if err == nil {
			t.Fatal("expected context canceled error")
		}
	})
}
