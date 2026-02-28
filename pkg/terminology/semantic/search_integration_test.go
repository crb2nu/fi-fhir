package semantic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/index"
)

// mockEmbeddingServer returns a fixed embedding vector for any request.
func mockEmbeddingServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// OpenAI-compatible /v1/embeddings endpoint
		if r.URL.Path == "/v1/embeddings" || r.URL.Path == "/embeddings" {
			var req struct {
				Input interface{} `json:"input"`
			}
			json.NewDecoder(r.Body).Decode(&req)

			// Return a deterministic 4-dim embedding for each input.
			var inputs []string
			switch v := req.Input.(type) {
			case string:
				inputs = []string{v}
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok {
						inputs = append(inputs, s)
					}
				}
			}

			embeddings := make([]map[string]interface{}, len(inputs))
			for i := range inputs {
				embeddings[i] = map[string]interface{}{
					"object":    "embedding",
					"index":     i,
					"embedding": []float64{0.1, 0.2, 0.3, 0.4},
				}
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"object": "list",
				"data":   embeddings,
				"model":  "test-model",
				"usage":  map[string]int{"total_tokens": 10},
			})
			return
		}
		http.NotFound(w, r)
	}))
}

// mockQdrantServer simulates the Qdrant REST API.
type mockQdrant struct {
	mu          sync.Mutex
	collections map[string][]index.Point
}

func newMockQdrantServer() (*httptest.Server, *mockQdrant) {
	mock := &mockQdrant{
		collections: make(map[string][]index.Point),
	}
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})

	// Collection + points endpoints
	mux.HandleFunc("/collections/", func(w http.ResponseWriter, r *http.Request) {
		mock.mu.Lock()
		defer mock.mu.Unlock()

		// Parse: /collections/{name} or /collections/{name}/points/...
		path := r.URL.Path[len("/collections/"):]
		parts := splitPath(path)
		if len(parts) == 0 {
			http.NotFound(w, r)
			return
		}
		collection := parts[0]

		if len(parts) == 1 {
			// /collections/{name}
			switch r.Method {
			case http.MethodGet:
				// CollectionExists / GetCollectionInfo
				if _, ok := mock.collections[collection]; ok {
					json.NewEncoder(w).Encode(map[string]interface{}{
						"result": map[string]interface{}{
							"status":        "green",
							"points_count":  len(mock.collections[collection]),
							"vectors_count": len(mock.collections[collection]),
							"config": map[string]interface{}{
								"params": map[string]interface{}{
									"vectors": map[string]interface{}{
										"size":     4,
										"distance": "Cosine",
									},
								},
							},
						},
					})
				} else {
					w.WriteHeader(http.StatusNotFound)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"status": map[string]interface{}{"error": "not found"},
					})
				}
			case http.MethodPut:
				// CreateCollection
				mock.collections[collection] = nil
				json.NewEncoder(w).Encode(map[string]interface{}{
					"result": true,
				})
			case http.MethodDelete:
				// DeleteCollection
				delete(mock.collections, collection)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"result": true,
				})
			}
			return
		}

		// /collections/{name}/points/...
		if parts[1] == "points" {
			if len(parts) == 2 {
				// PUT /collections/{name}/points — UpsertPoints
				if r.Method == http.MethodPut {
					var body struct {
						Points []struct {
							ID      string                 `json:"id"`
							Vector  []float64              `json:"vector"`
							Payload map[string]interface{} `json:"payload"`
						} `json:"points"`
					}
					json.NewDecoder(r.Body).Decode(&body)
					for _, p := range body.Points {
						mock.collections[collection] = append(mock.collections[collection], index.Point{
							ID:      p.ID,
							Vector:  p.Vector,
							Payload: p.Payload,
						})
					}
					json.NewEncoder(w).Encode(map[string]interface{}{
						"result": map[string]interface{}{"status": "completed"},
					})
					return
				}
			}
			if len(parts) == 3 && parts[2] == "search" && r.Method == http.MethodPost {
				// POST /collections/{name}/points/search
				var body struct {
					Vector         []float64 `json:"vector"`
					Limit          int       `json:"limit"`
					ScoreThreshold float64   `json:"score_threshold"`
				}
				json.NewDecoder(r.Body).Decode(&body)

				points := mock.collections[collection]
				limit := body.Limit
				if limit <= 0 || limit > len(points) {
					limit = len(points)
				}

				results := make([]map[string]interface{}, 0, limit)
				for i := 0; i < limit && i < len(points); i++ {
					score := 0.95 - float64(i)*0.05
					if score < body.ScoreThreshold {
						break
					}
					results = append(results, map[string]interface{}{
						"id":      points[i].ID,
						"score":   score,
						"payload": points[i].Payload,
					})
				}

				json.NewEncoder(w).Encode(map[string]interface{}{
					"result": results,
				})
				return
			}
		}
		http.NotFound(w, r)
	})

	server := httptest.NewServer(mux)
	return server, mock
}

func splitPath(path string) []string {
	var parts []string
	for _, p := range splitSlash(path) {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func splitSlash(s string) []string {
	result := []string{}
	start := 0
	for i, c := range s {
		if c == '/' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

// seedCollection adds test points to the mock Qdrant.
func (m *mockQdrant) seedCollection(name string, points []index.Point) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.collections[name] = points
}

func TestSearcher_Search_SingleVocabulary(t *testing.T) {
	embedSrv := mockEmbeddingServer()
	defer embedSrv.Close()

	qdrantSrv, mockQ := newMockQdrantServer()
	defer qdrantSrv.Close()

	mockQ.seedCollection("fi_fhir_idx_loinc", []index.Point{
		{ID: "loinc:2345-7", Payload: map[string]interface{}{"code": "2345-7", "display": "Glucose", "system": "http://loinc.org"}},
		{ID: "loinc:2339-0", Payload: map[string]interface{}{"code": "2339-0", "display": "Glucose [Mass]", "system": "http://loinc.org"}},
	})

	searcher, err := NewSearcher(SearchConfig{
		QdrantURL:           qdrantSrv.URL,
		EmbeddingBaseURL:    embedSrv.URL + "/v1",
		EmbeddingModel:      "test-model",
		EmbeddingDimensions: 4,
		DefaultMaxResults:   10,
		DefaultMinScore:     0.5,
		Timeout:             5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSearcher: %v", err)
	}

	matches, err := searcher.Search(context.Background(), "glucose blood test", SearchOptions{
		Vocabularies: []index.Vocabulary{index.VocabularyLOINC},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	if matches[0].Code != "2345-7" {
		t.Fatalf("first match code=%s, want 2345-7", matches[0].Code)
	}
	if matches[0].System != "http://loinc.org" {
		t.Fatalf("first match system=%s, want http://loinc.org", matches[0].System)
	}
	if matches[0].MatchType != "semantic" {
		t.Fatalf("match type=%s, want semantic", matches[0].MatchType)
	}
}

func TestSearcher_Search_EmptyQuery(t *testing.T) {
	embedSrv := mockEmbeddingServer()
	defer embedSrv.Close()
	qdrantSrv, _ := newMockQdrantServer()
	defer qdrantSrv.Close()

	searcher, err := NewSearcher(SearchConfig{
		QdrantURL:           qdrantSrv.URL,
		EmbeddingBaseURL:    embedSrv.URL + "/v1",
		EmbeddingModel:      "test-model",
		EmbeddingDimensions: 4,
		Timeout:             5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSearcher: %v", err)
	}

	matches, err := searcher.Search(context.Background(), "", SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected empty results for empty query, got %d", len(matches))
	}
}

func TestSearcher_Search_CollectionNotFound(t *testing.T) {
	embedSrv := mockEmbeddingServer()
	defer embedSrv.Close()
	qdrantSrv, _ := newMockQdrantServer()
	defer qdrantSrv.Close()

	searcher, err := NewSearcher(SearchConfig{
		QdrantURL:           qdrantSrv.URL,
		EmbeddingBaseURL:    embedSrv.URL + "/v1",
		EmbeddingModel:      "test-model",
		EmbeddingDimensions: 4,
		DefaultMaxResults:   10,
		DefaultMinScore:     0.5,
		Timeout:             5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSearcher: %v", err)
	}

	// Search for a vocabulary with no collection — should continue without error
	// because Search() logs errors from individual vocabs and continues.
	matches, err := searcher.Search(context.Background(), "test", SearchOptions{
		Vocabularies: []index.Vocabulary{index.VocabularySNOMED},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected empty results for missing collection, got %d", len(matches))
	}
}

func TestSearcher_Search_WithCache(t *testing.T) {
	embedSrv := mockEmbeddingServer()
	defer embedSrv.Close()
	qdrantSrv, mockQ := newMockQdrantServer()
	defer qdrantSrv.Close()

	mockQ.seedCollection("fi_fhir_idx_loinc", []index.Point{
		{ID: "loinc:1234-5", Payload: map[string]interface{}{"code": "1234-5", "display": "Test", "system": "http://loinc.org"}},
	})

	searcher, err := NewSearcher(SearchConfig{
		QdrantURL:           qdrantSrv.URL,
		EmbeddingBaseURL:    embedSrv.URL + "/v1",
		EmbeddingModel:      "test-model",
		EmbeddingDimensions: 4,
		DefaultMaxResults:   10,
		DefaultMinScore:     0.5,
		Timeout:             5 * time.Second,
		EnableCache:         true,
		CacheTTL:            1 * time.Minute,
		CacheMaxSize:        100,
	})
	if err != nil {
		t.Fatalf("NewSearcher: %v", err)
	}

	ctx := context.Background()

	// First search — populates cache.
	matches1, err := searcher.Search(ctx, "cached query", SearchOptions{
		Vocabularies: []index.Vocabulary{index.VocabularyLOINC},
	})
	if err != nil {
		t.Fatalf("Search 1: %v", err)
	}

	// Remove the collection to prove the second call uses cache.
	mockQ.mu.Lock()
	delete(mockQ.collections, "fi_fhir_idx_loinc")
	mockQ.mu.Unlock()

	// Second search — should hit cache.
	matches2, err := searcher.Search(ctx, "cached query", SearchOptions{
		Vocabularies: []index.Vocabulary{index.VocabularyLOINC},
	})
	if err != nil {
		t.Fatalf("Search 2: %v", err)
	}
	if len(matches2) != len(matches1) {
		t.Fatalf("cache miss: got %d results vs %d on first call", len(matches2), len(matches1))
	}

	// Clear cache and retry — should return empty now.
	searcher.ClearCache()
	matches3, err := searcher.Search(ctx, "cached query", SearchOptions{
		Vocabularies: []index.Vocabulary{index.VocabularyLOINC},
	})
	if err != nil {
		t.Fatalf("Search 3: %v", err)
	}
	if len(matches3) != 0 {
		t.Fatalf("expected empty results after cache clear, got %d", len(matches3))
	}
}

func TestSearcher_BestMatch(t *testing.T) {
	embedSrv := mockEmbeddingServer()
	defer embedSrv.Close()
	qdrantSrv, mockQ := newMockQdrantServer()
	defer qdrantSrv.Close()

	mockQ.seedCollection("fi_fhir_idx_loinc", []index.Point{
		{ID: "loinc:A", Payload: map[string]interface{}{"code": "A", "display": "Alpha", "system": "http://loinc.org"}},
		{ID: "loinc:B", Payload: map[string]interface{}{"code": "B", "display": "Beta", "system": "http://loinc.org"}},
	})

	searcher, err := NewSearcher(SearchConfig{
		QdrantURL:           qdrantSrv.URL,
		EmbeddingBaseURL:    embedSrv.URL + "/v1",
		EmbeddingModel:      "test-model",
		EmbeddingDimensions: 4,
		DefaultMaxResults:   10,
		DefaultMinScore:     0.5,
		Timeout:             5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSearcher: %v", err)
	}

	match, err := searcher.BestMatch(context.Background(), "alpha test", SearchOptions{
		Vocabularies: []index.Vocabulary{index.VocabularyLOINC},
	})
	if err != nil {
		t.Fatalf("BestMatch: %v", err)
	}
	if match == nil {
		t.Fatal("expected a match")
	}
	if match.Code != "A" {
		t.Fatalf("best match code=%s, want A", match.Code)
	}
}

func TestSearcher_BestMatch_NoResults(t *testing.T) {
	embedSrv := mockEmbeddingServer()
	defer embedSrv.Close()
	qdrantSrv, _ := newMockQdrantServer()
	defer qdrantSrv.Close()

	searcher, err := NewSearcher(SearchConfig{
		QdrantURL:           qdrantSrv.URL,
		EmbeddingBaseURL:    embedSrv.URL + "/v1",
		EmbeddingModel:      "test-model",
		EmbeddingDimensions: 4,
		DefaultMaxResults:   10,
		DefaultMinScore:     0.5,
		Timeout:             5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSearcher: %v", err)
	}

	match, err := searcher.BestMatch(context.Background(), "query with no collection", SearchOptions{
		Vocabularies: []index.Vocabulary{index.VocabularyICD10CM},
	})
	if err != nil {
		t.Fatalf("BestMatch: %v", err)
	}
	if match != nil {
		t.Fatalf("expected nil, got %+v", match)
	}
}

func TestSearcher_MultiVocabulary(t *testing.T) {
	embedSrv := mockEmbeddingServer()
	defer embedSrv.Close()
	qdrantSrv, mockQ := newMockQdrantServer()
	defer qdrantSrv.Close()

	mockQ.seedCollection("fi_fhir_idx_loinc", []index.Point{
		{ID: "loinc:1", Payload: map[string]interface{}{"code": "1", "display": "LOINC Test", "system": "http://loinc.org"}},
	})
	mockQ.seedCollection("fi_fhir_idx_snomedct", []index.Point{
		{ID: "snomedct:100", Payload: map[string]interface{}{"code": "100", "display": "SNOMED Test", "system": "http://snomed.info/sct"}},
	})

	searcher, err := NewSearcher(SearchConfig{
		QdrantURL:           qdrantSrv.URL,
		EmbeddingBaseURL:    embedSrv.URL + "/v1",
		EmbeddingModel:      "test-model",
		EmbeddingDimensions: 4,
		DefaultMaxResults:   10,
		DefaultMinScore:     0.5,
		Timeout:             5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSearcher: %v", err)
	}

	matches, err := searcher.Search(context.Background(), "test", SearchOptions{
		Vocabularies: []index.Vocabulary{index.VocabularyLOINC, index.VocabularySNOMED},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) < 2 {
		t.Fatalf("expected at least 2 matches from two vocabularies, got %d", len(matches))
	}

	// Results should be sorted by score (highest first).
	for i := 1; i < len(matches); i++ {
		if matches[i].Score > matches[i-1].Score {
			t.Fatalf("results not sorted by score: [%d]=%f > [%d]=%f",
				i, matches[i].Score, i-1, matches[i-1].Score)
		}
	}
}

func TestSearcher_Ping(t *testing.T) {
	embedSrv := mockEmbeddingServer()
	defer embedSrv.Close()
	qdrantSrv, _ := newMockQdrantServer()
	defer qdrantSrv.Close()

	searcher, err := NewSearcher(SearchConfig{
		QdrantURL:           qdrantSrv.URL,
		EmbeddingBaseURL:    embedSrv.URL + "/v1",
		EmbeddingModel:      "test-model",
		EmbeddingDimensions: 4,
		Timeout:             5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSearcher: %v", err)
	}

	if err := searcher.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestSearcher_IndexStats(t *testing.T) {
	embedSrv := mockEmbeddingServer()
	defer embedSrv.Close()
	qdrantSrv, mockQ := newMockQdrantServer()
	defer qdrantSrv.Close()

	mockQ.seedCollection("fi_fhir_idx_loinc", []index.Point{
		{ID: "loinc:1", Payload: map[string]interface{}{"code": "1"}},
		{ID: "loinc:2", Payload: map[string]interface{}{"code": "2"}},
		{ID: "loinc:3", Payload: map[string]interface{}{"code": "3"}},
	})

	searcher, err := NewSearcher(SearchConfig{
		QdrantURL:           qdrantSrv.URL,
		EmbeddingBaseURL:    embedSrv.URL + "/v1",
		EmbeddingModel:      "test-model",
		EmbeddingDimensions: 4,
		Timeout:             5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSearcher: %v", err)
	}

	stats, err := searcher.IndexStats(context.Background(), index.VocabularyLOINC)
	if err != nil {
		t.Fatalf("IndexStats: %v", err)
	}
	if stats.Vocabulary != index.VocabularyLOINC {
		t.Fatalf("vocabulary=%s, want loinc", stats.Vocabulary)
	}
	if stats.Collection != "fi_fhir_idx_loinc" {
		t.Fatalf("collection=%s, want fi_fhir_idx_loinc", stats.Collection)
	}
	if stats.TotalEntries != 3 {
		t.Fatalf("total entries=%d, want 3", stats.TotalEntries)
	}
}

func TestSearcher_IndexStats_NotFound(t *testing.T) {
	embedSrv := mockEmbeddingServer()
	defer embedSrv.Close()
	qdrantSrv, _ := newMockQdrantServer()
	defer qdrantSrv.Close()

	searcher, err := NewSearcher(SearchConfig{
		QdrantURL:           qdrantSrv.URL,
		EmbeddingBaseURL:    embedSrv.URL + "/v1",
		EmbeddingModel:      "test-model",
		EmbeddingDimensions: 4,
		Timeout:             5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSearcher: %v", err)
	}

	_, err = searcher.IndexStats(context.Background(), index.VocabularyRxNorm)
	if err == nil {
		t.Fatal("expected error for non-existent collection")
	}
}

// TestSearchCache_TTLExpiry verifies that cache entries expire after TTL.
func TestSearchCache_TTLExpiry(t *testing.T) {
	cache := newSearchCache(50*time.Millisecond, 100)

	matches := []SemanticMatch{{Code: "A", Score: 0.9}}
	cache.set("key1", matches)

	// Should be present immediately.
	got, ok := cache.get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 match, got %d", len(got))
	}

	// Wait for TTL to expire.
	time.Sleep(60 * time.Millisecond)
	_, ok = cache.get("key1")
	if ok {
		t.Fatal("expected cache miss after TTL")
	}
}

// TestSearchCache_MaxSize verifies eviction at capacity.
func TestSearchCache_MaxSize(t *testing.T) {
	cache := newSearchCache(1*time.Hour, 3)

	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		cache.set(key, []SemanticMatch{{Code: key}})
	}

	// Cache should not exceed maxSize.
	cache.mu.RLock()
	size := len(cache.entries)
	cache.mu.RUnlock()

	if size > 3 {
		t.Fatalf("cache size=%d, want <= 3", size)
	}
}

func TestNewSearcher_InvalidConfig(t *testing.T) {
	_, err := NewSearcher(SearchConfig{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

// newTestSearcher creates a Searcher backed by mock servers with seeded LOINC data.
func newTestSearcher(t *testing.T) (*Searcher, *httptest.Server, *httptest.Server, *mockQdrant) {
	t.Helper()
	embedSrv := mockEmbeddingServer()
	qdrantSrv, mockQ := newMockQdrantServer()

	mockQ.seedCollection("fi_fhir_idx_loinc", []index.Point{
		{ID: "loinc:2345-7", Payload: map[string]interface{}{"code": "2345-7", "display": "Glucose", "system": "http://loinc.org"}},
		{ID: "loinc:2339-0", Payload: map[string]interface{}{"code": "2339-0", "display": "Glucose [Mass]", "system": "http://loinc.org"}},
	})
	mockQ.seedCollection("fi_fhir_idx_snomedct", []index.Point{
		{ID: "snomedct:33747003", Payload: map[string]interface{}{"code": "33747003", "display": "Glucose (substance)", "system": "http://snomed.info/sct"}},
	})
	mockQ.seedCollection("fi_fhir_idx_icd10cm", []index.Point{
		{ID: "icd10:E11.9", Payload: map[string]interface{}{"code": "E11.9", "display": "Type 2 diabetes", "system": "http://hl7.org/fhir/sid/icd-10-cm"}},
	})

	searcher, err := NewSearcher(SearchConfig{
		QdrantURL:           qdrantSrv.URL,
		EmbeddingBaseURL:    embedSrv.URL + "/v1",
		EmbeddingModel:      "test-model",
		EmbeddingDimensions: 4,
		DefaultMaxResults:   10,
		DefaultMinScore:     0.5,
		Timeout:             5 * time.Second,
	})
	if err != nil {
		embedSrv.Close()
		qdrantSrv.Close()
		t.Fatalf("NewSearcher: %v", err)
	}

	return searcher, embedSrv, qdrantSrv, mockQ
}

func TestSearcher_SearchVocabulary(t *testing.T) {
	searcher, embedSrv, qdrantSrv, _ := newTestSearcher(t)
	defer embedSrv.Close()
	defer qdrantSrv.Close()

	matches, err := searcher.SearchVocabulary(context.Background(), "glucose", index.VocabularyLOINC, SearchOptions{
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("SearchVocabulary: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	if matches[0].Code != "2345-7" {
		t.Fatalf("code=%s, want 2345-7", matches[0].Code)
	}
}

func TestSearcher_SearchLOINC(t *testing.T) {
	searcher, embedSrv, qdrantSrv, _ := newTestSearcher(t)
	defer embedSrv.Close()
	defer qdrantSrv.Close()

	matches, err := searcher.SearchLOINC(context.Background(), "glucose", 5)
	if err != nil {
		t.Fatalf("SearchLOINC: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	if matches[0].System != "http://loinc.org" {
		t.Fatalf("system=%s, want http://loinc.org", matches[0].System)
	}
}

func TestSearcher_SearchSNOMED(t *testing.T) {
	searcher, embedSrv, qdrantSrv, _ := newTestSearcher(t)
	defer embedSrv.Close()
	defer qdrantSrv.Close()

	matches, err := searcher.SearchSNOMED(context.Background(), "glucose substance", 5)
	if err != nil {
		t.Fatalf("SearchSNOMED: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	if matches[0].System != "http://snomed.info/sct" {
		t.Fatalf("system=%s, want http://snomed.info/sct", matches[0].System)
	}
}

func TestSearcher_SearchICD10(t *testing.T) {
	searcher, embedSrv, qdrantSrv, _ := newTestSearcher(t)
	defer embedSrv.Close()
	defer qdrantSrv.Close()

	matches, err := searcher.SearchICD10(context.Background(), "diabetes", 5)
	if err != nil {
		t.Fatalf("SearchICD10: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	if matches[0].Code != "E11.9" {
		t.Fatalf("code=%s, want E11.9", matches[0].Code)
	}
}

func TestHybridSearcher_FuzzyMatchSufficient(t *testing.T) {
	searcher, embedSrv, qdrantSrv, _ := newTestSearcher(t)
	defer embedSrv.Close()
	defer qdrantSrv.Close()

	hybrid := NewHybridSearcher(searcher, DefaultHybridConfig())

	// Fuzzy matches above threshold — should return as-is without semantic search.
	fuzzyMatches := []FuzzyMatch{
		{Code: "1234-5", Display: "Existing Match", Confidence: 0.95, MatchType: "fuzzy"},
	}

	results, err := hybrid.SearchWithFallback(context.Background(), "test", fuzzyMatches, index.VocabularyLOINC)
	if err != nil {
		t.Fatalf("SearchWithFallback: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (fuzzy sufficient), got %d", len(results))
	}
	if results[0].Code != "1234-5" {
		t.Fatalf("code=%s, want 1234-5", results[0].Code)
	}
}

func TestHybridSearcher_FallbackToSemantic(t *testing.T) {
	searcher, embedSrv, qdrantSrv, _ := newTestSearcher(t)
	defer embedSrv.Close()
	defer qdrantSrv.Close()

	hybrid := NewHybridSearcher(searcher, DefaultHybridConfig())

	// Fuzzy matches below threshold — triggers semantic fallback.
	fuzzyMatches := []FuzzyMatch{
		{Code: "LOW-1", Display: "Low Match", Confidence: 0.3, MatchType: "fuzzy"},
	}

	results, err := hybrid.SearchWithFallback(context.Background(), "glucose", fuzzyMatches, index.VocabularyLOINC)
	if err != nil {
		t.Fatalf("SearchWithFallback: %v", err)
	}
	// Should have combined results: original fuzzy + semantic matches.
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results (fuzzy + semantic), got %d", len(results))
	}
}

func TestHybridSearcher_PreferSemantic(t *testing.T) {
	searcher, embedSrv, qdrantSrv, _ := newTestSearcher(t)
	defer embedSrv.Close()
	defer qdrantSrv.Close()

	cfg := DefaultHybridConfig()
	cfg.PreferSemantic = true
	hybrid := NewHybridSearcher(searcher, cfg)

	// Even with high fuzzy match, PreferSemantic triggers semantic search.
	fuzzyMatches := []FuzzyMatch{
		{Code: "2345-7", Display: "Glucose", Confidence: 0.95, MatchType: "fuzzy"},
	}

	results, err := hybrid.SearchWithFallback(context.Background(), "glucose", fuzzyMatches, index.VocabularyLOINC)
	if err != nil {
		t.Fatalf("SearchWithFallback: %v", err)
	}
	// Should have results from both fuzzy and semantic (deduped by code).
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// Code 2345-7 should have boosted confidence from semantic merge.
	for _, r := range results {
		if r.Code == "2345-7" && r.Confidence < 0.95 {
			t.Fatalf("code 2345-7 confidence=%f should be >= 0.95 after boost", r.Confidence)
		}
	}
}

func TestSemanticEnhancer_EnhanceCode(t *testing.T) {
	searcher, embedSrv, qdrantSrv, _ := newTestSearcher(t)
	defer embedSrv.Close()
	defer qdrantSrv.Close()

	enhancer := NewSemanticEnhancer(searcher)

	matches, err := enhancer.EnhanceCode(context.Background(), "GLU", "Glucose", index.VocabularyLOINC)
	if err != nil {
		t.Fatalf("EnhanceCode: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one enhanced match")
	}
	// Reason should mention the display text.
	for _, m := range matches {
		if m.Reason == "" {
			t.Fatal("expected non-empty reason")
		}
	}
}

func TestSemanticEnhancer_EnhanceCode_EmptyDisplay(t *testing.T) {
	searcher, embedSrv, qdrantSrv, _ := newTestSearcher(t)
	defer embedSrv.Close()
	defer qdrantSrv.Close()

	enhancer := NewSemanticEnhancer(searcher)

	// When display is empty, should use localCode as search text.
	matches, err := enhancer.EnhanceCode(context.Background(), "glucose", "", index.VocabularyLOINC)
	if err != nil {
		t.Fatalf("EnhanceCode: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one match using code as search text")
	}
}

func TestSemanticEnhancer_FindSimilarCodes(t *testing.T) {
	searcher, embedSrv, qdrantSrv, _ := newTestSearcher(t)
	defer embedSrv.Close()
	defer qdrantSrv.Close()

	enhancer := NewSemanticEnhancer(searcher)

	// Find codes similar to 2345-7 — should filter out the original code.
	matches, err := enhancer.FindSimilarCodes(context.Background(), "2345-7", "Glucose", index.VocabularyLOINC, 5)
	if err != nil {
		t.Fatalf("FindSimilarCodes: %v", err)
	}
	for _, m := range matches {
		if m.Code == "2345-7" {
			t.Fatal("FindSimilarCodes should filter out the original code")
		}
	}
}

func TestSemanticEnhancer_FindSimilarCodes_EmptyDisplay(t *testing.T) {
	searcher, embedSrv, qdrantSrv, _ := newTestSearcher(t)
	defer embedSrv.Close()
	defer qdrantSrv.Close()

	enhancer := NewSemanticEnhancer(searcher)

	// Empty display — should use code as search text.
	_, err := enhancer.FindSimilarCodes(context.Background(), "glucose", "", index.VocabularyLOINC, 5)
	if err != nil {
		t.Fatalf("FindSimilarCodes: %v", err)
	}
}

func TestMultiVocabularySearch_SearchAll(t *testing.T) {
	searcher, embedSrv, qdrantSrv, _ := newTestSearcher(t)
	defer embedSrv.Close()
	defer qdrantSrv.Close()

	mvs := NewMultiVocabularySearch(searcher)

	results, err := mvs.SearchAll(context.Background(), "glucose", 5)
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	// Should have results from LOINC and SNOMED (ICD10 has "diabetes" not "glucose"
	// but mock returns results regardless of query content).
	if len(results) == 0 {
		t.Fatal("expected results from at least one vocabulary")
	}
	// Verify results are grouped by vocabulary.
	if _, ok := results[index.VocabularyLOINC]; !ok {
		t.Fatal("expected LOINC results")
	}
}

func TestMultiVocabularySearch_SearchWithPreference(t *testing.T) {
	searcher, embedSrv, qdrantSrv, _ := newTestSearcher(t)
	defer embedSrv.Close()
	defer qdrantSrv.Close()

	mvs := NewMultiVocabularySearch(searcher)

	// Prefer SNOMED, then LOINC.
	results, err := mvs.SearchWithPreference(
		context.Background(),
		"glucose",
		[]index.Vocabulary{index.VocabularySNOMED, index.VocabularyLOINC},
		5,
	)
	if err != nil {
		t.Fatalf("SearchWithPreference: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// First result should be from SNOMED (preferred) since mock returns high-confidence matches.
	if results[0].System != "http://snomed.info/sct" {
		t.Fatalf("first result system=%s, want http://snomed.info/sct (preferred)", results[0].System)
	}
}
