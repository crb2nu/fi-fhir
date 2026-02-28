package index

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// qdrantAPI abstracts the Qdrant vector database for testability.
type qdrantAPI interface {
	Ping(ctx context.Context) error
	CreateCollection(ctx context.Context, name string, dimensions int) error
	DeleteCollection(ctx context.Context, name string) error
	CollectionExists(ctx context.Context, name string) (bool, error)
	GetCollectionInfo(ctx context.Context, name string) (*CollectionInfo, error)
	UpsertPoints(ctx context.Context, collection string, points []Point) error
	Search(ctx context.Context, collection string, vector []float64, limit int, scoreThreshold float64) ([]SearchHit, error)
	GetPoints(ctx context.Context, collection string, ids []string) ([]Point, error)
	DeletePoints(ctx context.Context, collection string, ids []string) error
	ScrollPoints(ctx context.Context, collection string, limit int, offset *string) (*ScrollResult, error)
}

// Manager provides high-level operations for terminology indexes.
type Manager struct {
	config          IndexConfig
	qdrant          qdrantAPI
	embeddingClient llm.EmbeddingClient
}

// NewManager creates a new index manager.
func NewManager(config IndexConfig) (*Manager, error) {
	// Create embedding client
	embCfg := llm.EmbeddingConfig{
		BaseURL:    config.EmbeddingBaseURL,
		APIKey:     config.EmbeddingAPIKey,
		Model:      config.EmbeddingModel,
		Dimensions: config.EmbeddingDimensions,
		Timeout:    config.Timeout,
		MaxRetries: 3,
		BatchSize:  config.BatchSize,
	}
	embClient, err := llm.NewEmbeddingClient(embCfg)
	if err != nil {
		return nil, fmt.Errorf("create embedding client: %w", err)
	}

	// Create Qdrant client
	qdrant := NewQdrantClient(config.QdrantURL, config.QdrantAPIKey, config.Timeout)

	return &Manager{
		config:          config,
		qdrant:          qdrant,
		embeddingClient: embClient,
	}, nil
}

// Search performs a semantic search across a vocabulary index.
func (m *Manager) Search(ctx context.Context, vocabulary Vocabulary, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// Generate embedding for query
	embedding, err := m.embeddingClient.EmbedSingle(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("generate embedding: %w", err)
	}

	// Search in Qdrant
	collectionName := vocabulary.CollectionName()
	hits, err := m.qdrant.Search(ctx, collectionName, embedding, limit, 0.0)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	// Convert to results
	results := make([]SearchResult, len(hits))
	for i, hit := range hits {
		entry := IndexEntry{
			ID:         hit.ID,
			Code:       getString(hit.Payload, "code"),
			System:     getString(hit.Payload, "system"),
			Display:    getString(hit.Payload, "display"),
			Vocabulary: Vocabulary(getString(hit.Payload, "vocabulary")),
			Metadata:   hit.Payload,
		}

		results[i] = SearchResult{
			Entry: entry,
			Score: hit.Score,
		}
	}

	return results, nil
}

// SearchAll searches across all vocabulary indexes.
func (m *Manager) SearchAll(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// Generate embedding for query
	embedding, err := m.embeddingClient.EmbedSingle(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("generate embedding: %w", err)
	}

	// Search all vocabularies in parallel
	vocabularies := []Vocabulary{VocabularyLOINC, VocabularySNOMED, VocabularyICD10CM}
	var wg sync.WaitGroup
	resultsChan := make(chan []SearchResult, len(vocabularies))
	errorsChan := make(chan error, len(vocabularies))

	for _, vocab := range vocabularies {
		wg.Add(1)
		go func(v Vocabulary) {
			defer wg.Done()

			collectionName := v.CollectionName()
			exists, err := m.qdrant.CollectionExists(ctx, collectionName)
			if err != nil || !exists {
				return // Skip non-existent collections
			}

			hits, err := m.qdrant.Search(ctx, collectionName, embedding, limit, 0.0)
			if err != nil {
				errorsChan <- fmt.Errorf("%s: %w", v, err)
				return
			}

			results := make([]SearchResult, len(hits))
			for i, hit := range hits {
				entry := IndexEntry{
					ID:         hit.ID,
					Code:       getString(hit.Payload, "code"),
					System:     getString(hit.Payload, "system"),
					Display:    getString(hit.Payload, "display"),
					Vocabulary: v,
					Metadata:   hit.Payload,
				}
				results[i] = SearchResult{
					Entry: entry,
					Score: hit.Score,
				}
			}
			resultsChan <- results
		}(vocab)
	}

	wg.Wait()
	close(resultsChan)
	close(errorsChan)

	// Collect all results
	var allResults []SearchResult
	for results := range resultsChan {
		allResults = append(allResults, results...)
	}

	// Sort by score (descending)
	for i := 0; i < len(allResults)-1; i++ {
		for j := i + 1; j < len(allResults); j++ {
			if allResults[i].Score < allResults[j].Score {
				allResults[i], allResults[j] = allResults[j], allResults[i]
			}
		}
	}

	// Limit results
	if len(allResults) > limit {
		allResults = allResults[:limit]
	}

	return allResults, nil
}

// GetStats returns statistics for a vocabulary index.
func (m *Manager) GetStats(ctx context.Context, vocabulary Vocabulary) (*IndexStats, error) {
	collectionName := vocabulary.CollectionName()

	info, err := m.qdrant.GetCollectionInfo(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("get collection info: %w", err)
	}

	return &IndexStats{
		Vocabulary:     vocabulary,
		Collection:     collectionName,
		TotalEntries:   info.PointsCount,
		EmbeddingModel: m.config.EmbeddingModel,
		Dimensions:     info.Config.Params.Vectors.Size,
	}, nil
}

// GetAllStats returns statistics for all vocabulary indexes.
func (m *Manager) GetAllStats(ctx context.Context) ([]IndexStats, error) {
	vocabularies := []Vocabulary{VocabularyLOINC, VocabularySNOMED, VocabularyICD10CM, VocabularyRxNorm, VocabularyCPT, VocabularyCVX}
	var stats []IndexStats

	for _, vocab := range vocabularies {
		collectionName := vocab.CollectionName()

		exists, err := m.qdrant.CollectionExists(ctx, collectionName)
		if err != nil {
			continue
		}
		if !exists {
			continue
		}

		info, err := m.qdrant.GetCollectionInfo(ctx, collectionName)
		if err != nil {
			continue
		}

		stats = append(stats, IndexStats{
			Vocabulary:     vocab,
			Collection:     collectionName,
			TotalEntries:   info.PointsCount,
			EmbeddingModel: m.config.EmbeddingModel,
			Dimensions:     info.Config.Params.Vectors.Size,
		})
	}

	return stats, nil
}

// Lookup retrieves a specific entry by code.
func (m *Manager) Lookup(ctx context.Context, vocabulary Vocabulary, code string) (*IndexEntry, error) {
	collectionName := vocabulary.CollectionName()
	id := string(vocabulary) + ":" + code

	points, err := m.qdrant.GetPoints(ctx, collectionName, []string{id})
	if err != nil {
		return nil, fmt.Errorf("get point: %w", err)
	}

	if len(points) == 0 {
		return nil, nil
	}

	point := points[0]
	entry := &IndexEntry{
		ID:         point.ID,
		Code:       getString(point.Payload, "code"),
		System:     getString(point.Payload, "system"),
		Display:    getString(point.Payload, "display"),
		Vocabulary: vocabulary,
		Metadata:   point.Payload,
	}

	return entry, nil
}

// DeleteIndex deletes an entire vocabulary index.
func (m *Manager) DeleteIndex(ctx context.Context, vocabulary Vocabulary) error {
	collectionName := vocabulary.CollectionName()
	return m.qdrant.DeleteCollection(ctx, collectionName)
}

// Ready checks if the index manager is ready (Qdrant is reachable).
func (m *Manager) Ready(ctx context.Context) error {
	return m.qdrant.Ping(ctx)
}

// WaitForReady waits for the index manager to become ready.
func (m *Manager) WaitForReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := m.Ready(ctx); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	return fmt.Errorf("index manager not ready after %v", timeout)
}

// getString safely extracts a string from a payload map.
func getString(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}
	v, ok := payload[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
