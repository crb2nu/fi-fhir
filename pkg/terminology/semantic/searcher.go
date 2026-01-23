// Package semantic provides embedding-based semantic search for terminology codes.
package semantic

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/index"
)

// Searcher provides embedding-based semantic search across terminology indexes.
type Searcher struct {
	embedder llm.EmbeddingClient
	qdrant   *index.QdrantClient
	config   *SearchConfig
	cache    *searchCache
	mu       sync.RWMutex
}

// NewSearcher creates a new semantic searcher.
func NewSearcher(cfg SearchConfig) (*Searcher, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	embedder, err := llm.NewEmbeddingClient(llm.EmbeddingConfig{
		BaseURL:    cfg.EmbeddingBaseURL,
		APIKey:     cfg.EmbeddingAPIKey,
		Model:      cfg.EmbeddingModel,
		Dimensions: cfg.EmbeddingDimensions,
		Timeout:    cfg.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("create embedding client: %w", err)
	}

	qdrant := index.NewQdrantClient(cfg.QdrantURL, cfg.QdrantAPIKey, cfg.Timeout)

	var cache *searchCache
	if cfg.EnableCache {
		ttl := cfg.CacheTTL
		if ttl == 0 {
			ttl = 1 * time.Hour
		}
		cache = newSearchCache(ttl, cfg.CacheMaxSize)
	}

	return &Searcher{
		embedder: embedder,
		qdrant:   qdrant,
		config:   &cfg,
		cache:    cache,
	}, nil
}

// Search performs a semantic search for the given query across specified vocabularies.
func (s *Searcher) Search(ctx context.Context, query string, opts SearchOptions) ([]SemanticMatch, error) {
	if query == "" {
		return nil, nil
	}

	// Set defaults
	if opts.MaxResults == 0 {
		opts.MaxResults = s.config.DefaultMaxResults
	}
	if opts.MinScore == 0 {
		opts.MinScore = s.config.DefaultMinScore
	}
	if len(opts.Vocabularies) == 0 {
		opts.Vocabularies = []index.Vocabulary{index.VocabularyLOINC}
	}

	// Check cache
	if s.cache != nil {
		cacheKey := s.cacheKey(query, opts)
		if cached, ok := s.cache.get(cacheKey); ok {
			return cached, nil
		}
	}

	// Generate query embedding
	embedding, err := s.embedder.EmbedSingle(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// Search each vocabulary
	var allMatches []SemanticMatch
	for _, vocab := range opts.Vocabularies {
		matches, err := s.searchVocabulary(ctx, embedding, vocab, opts)
		if err != nil {
			// Log error but continue with other vocabularies
			continue
		}
		allMatches = append(allMatches, matches...)
	}

	// Sort by score and limit results
	sortByScore(allMatches)
	if len(allMatches) > opts.MaxResults {
		allMatches = allMatches[:opts.MaxResults]
	}

	// Cache results
	if s.cache != nil {
		cacheKey := s.cacheKey(query, opts)
		s.cache.set(cacheKey, allMatches)
	}

	return allMatches, nil
}

// SearchVocabulary performs a semantic search within a specific vocabulary.
func (s *Searcher) SearchVocabulary(ctx context.Context, query string, vocab index.Vocabulary, opts SearchOptions) ([]SemanticMatch, error) {
	opts.Vocabularies = []index.Vocabulary{vocab}
	return s.Search(ctx, query, opts)
}

// SearchLOINC is a convenience method for LOINC-specific searches.
func (s *Searcher) SearchLOINC(ctx context.Context, query string, maxResults int) ([]SemanticMatch, error) {
	return s.Search(ctx, query, SearchOptions{
		Vocabularies: []index.Vocabulary{index.VocabularyLOINC},
		MaxResults:   maxResults,
	})
}

// SearchSNOMED is a convenience method for SNOMED CT-specific searches.
func (s *Searcher) SearchSNOMED(ctx context.Context, query string, maxResults int) ([]SemanticMatch, error) {
	return s.Search(ctx, query, SearchOptions{
		Vocabularies: []index.Vocabulary{index.VocabularySNOMED},
		MaxResults:   maxResults,
	})
}

// SearchICD10 is a convenience method for ICD-10-CM-specific searches.
func (s *Searcher) SearchICD10(ctx context.Context, query string, maxResults int) ([]SemanticMatch, error) {
	return s.Search(ctx, query, SearchOptions{
		Vocabularies: []index.Vocabulary{index.VocabularyICD10CM},
		MaxResults:   maxResults,
	})
}

// BestMatch returns the single best semantic match for a query.
func (s *Searcher) BestMatch(ctx context.Context, query string, opts SearchOptions) (*SemanticMatch, error) {
	opts.MaxResults = 1
	matches, err := s.Search(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return &matches[0], nil
}

// searchVocabulary searches a single vocabulary index.
func (s *Searcher) searchVocabulary(ctx context.Context, embedding []float64, vocab index.Vocabulary, opts SearchOptions) ([]SemanticMatch, error) {
	collection := vocab.CollectionName()

	// Check if collection exists
	exists, err := s.qdrant.CollectionExists(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("check collection: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("index not found for vocabulary %s", vocab)
	}

	// Search Qdrant
	hits, err := s.qdrant.Search(ctx, collection, embedding, opts.MaxResults*2, opts.MinScore)
	if err != nil {
		return nil, fmt.Errorf("search qdrant: %w", err)
	}

	// Convert hits to SemanticMatch
	var matches []SemanticMatch
	for _, hit := range hits {
		match := hitToMatch(hit, vocab)
		if match.Score >= opts.MinScore {
			matches = append(matches, match)
		}
	}

	return matches, nil
}

// hitToMatch converts a Qdrant search hit to a SemanticMatch.
func hitToMatch(hit index.SearchHit, vocab index.Vocabulary) SemanticMatch {
	match := SemanticMatch{
		Code:       getString(hit.Payload, "code"),
		Display:    getString(hit.Payload, "display"),
		System:     getString(hit.Payload, "system"),
		Vocabulary: vocab,
		Score:      hit.Score,
		MatchType:  "semantic",
	}

	// Copy metadata
	match.Metadata = make(map[string]interface{})
	for k, v := range hit.Payload {
		if k != "code" && k != "display" && k != "system" && k != "vocabulary" {
			match.Metadata[k] = v
		}
	}

	return match
}

// getString safely extracts a string from a payload map.
func getString(payload map[string]interface{}, key string) string {
	if v, ok := payload[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// cacheKey generates a cache key for a search query.
func (s *Searcher) cacheKey(query string, opts SearchOptions) string {
	vocabs := ""
	for _, v := range opts.Vocabularies {
		vocabs += string(v) + ","
	}
	return fmt.Sprintf("%s:%s:%d:%.2f", query, vocabs, opts.MaxResults, opts.MinScore)
}

// sortByScore sorts matches by score in descending order.
func sortByScore(matches []SemanticMatch) {
	// Simple insertion sort for small slices
	for i := 1; i < len(matches); i++ {
		key := matches[i]
		j := i - 1
		for j >= 0 && matches[j].Score < key.Score {
			matches[j+1] = matches[j]
			j--
		}
		matches[j+1] = key
	}
}

// Ping checks connectivity to Qdrant.
func (s *Searcher) Ping(ctx context.Context) error {
	return s.qdrant.Ping(ctx)
}

// IndexStats returns statistics for a vocabulary index.
func (s *Searcher) IndexStats(ctx context.Context, vocab index.Vocabulary) (*index.IndexStats, error) {
	collection := vocab.CollectionName()

	exists, err := s.qdrant.CollectionExists(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("check collection: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("index not found for vocabulary %s", vocab)
	}

	info, err := s.qdrant.GetCollectionInfo(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("get collection info: %w", err)
	}

	return &index.IndexStats{
		Vocabulary:     vocab,
		Collection:     collection,
		TotalEntries:   info.PointsCount,
		EmbeddingModel: s.embedder.Model(),
		Dimensions:     info.Config.Params.Vectors.Size,
	}, nil
}

// ClearCache clears the search cache.
func (s *Searcher) ClearCache() {
	if s.cache != nil {
		s.cache.clear()
	}
}

// searchCache provides caching for search results.
type searchCache struct {
	entries map[string]*cacheEntry
	ttl     time.Duration
	maxSize int
	mu      sync.RWMutex
}

type cacheEntry struct {
	results   []SemanticMatch
	expiresAt time.Time
}

func newSearchCache(ttl time.Duration, maxSize int) *searchCache {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &searchCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

func (c *searchCache) get(key string) ([]SemanticMatch, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.results, true
}

func (c *searchCache) set(key string, results []SemanticMatch) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = &cacheEntry{
		results:   results,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *searchCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}

func (c *searchCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.expiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.expiresAt
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
