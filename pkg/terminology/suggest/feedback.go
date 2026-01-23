package suggest

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/index"
)

// Feedback represents user feedback on a code suggestion.
type Feedback struct {
	// ID is the unique identifier for this feedback.
	ID string `json:"id"`

	// SourceCode is the original local code.
	SourceCode string `json:"source_code"`

	// SourceDisplay is the display text of the source code.
	SourceDisplay string `json:"source_display"`

	// SourceSystem identifies the source of the local code.
	SourceSystem string `json:"source_system,omitempty"`

	// SuggestedCode is the code that was suggested.
	SuggestedCode string `json:"suggested_code"`

	// SuggestedDisplay is the display name of the suggested code.
	SuggestedDisplay string `json:"suggested_display"`

	// TargetVocabulary is the vocabulary of the suggested code.
	TargetVocabulary index.Vocabulary `json:"target_vocabulary"`

	// Accepted indicates if the suggestion was accepted.
	Accepted bool `json:"accepted"`

	// AcceptedCode is the code that was actually accepted (may differ from suggested).
	AcceptedCode string `json:"accepted_code,omitempty"`

	// AcceptedDisplay is the display name of the accepted code.
	AcceptedDisplay string `json:"accepted_display,omitempty"`

	// Confidence is the original suggestion confidence.
	Confidence float64 `json:"confidence"`

	// Strategy is the strategy that produced the suggestion.
	Strategy SuggestionStrategy `json:"strategy"`

	// AcceptCount is how many times this mapping has been accepted.
	AcceptCount int `json:"accept_count,omitempty"`

	// CreatedAt is when this feedback was recorded.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this feedback was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// UserID is the user who provided the feedback (optional).
	UserID string `json:"user_id,omitempty"`

	// Notes contains any additional notes from the user.
	Notes string `json:"notes,omitempty"`

	// Similarity is used when finding similar feedback.
	Similarity float64 `json:"similarity,omitempty"`
}

// FeedbackStoreConfig configures the feedback store.
type FeedbackStoreConfig struct {
	QdrantURL    string `yaml:"qdrant_url" json:"qdrant_url"`
	QdrantAPIKey string `yaml:"qdrant_api_key" json:"qdrant_api_key"`
	Collection   string `yaml:"collection" json:"collection"`
}

// FeedbackStore manages feedback data in Qdrant.
type FeedbackStore struct {
	qdrant     *index.QdrantClient
	embedder   llm.EmbeddingClient
	collection string
	mu         sync.RWMutex

	// In-memory cache for fast lookup
	codeIndex map[string][]Feedback // sourceCode:sourceSystem -> feedbacks
	cacheTTL  time.Duration
	cacheTime time.Time
}

// NewFeedbackStore creates a new feedback store.
func NewFeedbackStore(cfg FeedbackStoreConfig) *FeedbackStore {
	return &FeedbackStore{
		qdrant:     index.NewQdrantClient(cfg.QdrantURL, cfg.QdrantAPIKey, 30*time.Second),
		collection: cfg.Collection,
		codeIndex:  make(map[string][]Feedback),
		cacheTTL:   10 * time.Minute,
	}
}

// Initialize creates the collection if it doesn't exist.
func (s *FeedbackStore) Initialize(ctx context.Context, embedder llm.EmbeddingClient) error {
	s.embedder = embedder

	exists, err := s.qdrant.CollectionExists(ctx, s.collection)
	if err != nil {
		return fmt.Errorf("check collection: %w", err)
	}

	if !exists {
		if err := s.qdrant.CreateCollection(ctx, s.collection, embedder.Dimensions()); err != nil {
			return fmt.Errorf("create collection: %w", err)
		}
	}

	return nil
}

// Record stores feedback for a suggestion.
func (s *FeedbackStore) Record(ctx context.Context, fb Feedback) error {
	if fb.ID == "" {
		fb.ID = generateFeedbackID(fb)
	}
	fb.CreatedAt = time.Now()
	fb.UpdatedAt = fb.CreatedAt

	// Generate embedding for similarity search
	embeddingText := fb.SourceDisplay
	if embeddingText == "" {
		embeddingText = fb.SourceCode
	}

	var embedding []float64
	if s.embedder != nil {
		var err error
		embedding, err = s.embedder.EmbedSingle(ctx, embeddingText)
		if err != nil {
			// Continue without embedding
			embedding = make([]float64, 1024) // Zero vector as fallback
		}
	} else {
		embedding = make([]float64, 1024)
	}

	// Store in Qdrant
	payload, _ := json.Marshal(fb)
	var payloadMap map[string]interface{}
	json.Unmarshal(payload, &payloadMap)

	point := index.Point{
		ID:      fb.ID,
		Vector:  embedding,
		Payload: payloadMap,
	}

	if err := s.qdrant.UpsertPoints(ctx, s.collection, []index.Point{point}); err != nil {
		return fmt.Errorf("upsert feedback: %w", err)
	}

	// Update cache
	s.invalidateCache()

	return nil
}

// FindBySourceCode finds feedback by source code.
func (s *FeedbackStore) FindBySourceCode(ctx context.Context, sourceCode, sourceSystem string) ([]Feedback, error) {
	// Check cache first
	s.mu.RLock()
	cacheKey := sourceCode + ":" + sourceSystem
	if time.Since(s.cacheTime) < s.cacheTTL {
		if feedbacks, ok := s.codeIndex[cacheKey]; ok {
			s.mu.RUnlock()
			return feedbacks, nil
		}
	}
	s.mu.RUnlock()

	// Scroll through collection looking for matches
	// This is not ideal for large collections, but works for moderate sizes
	var results []Feedback
	var offset *string

	for {
		scroll, err := s.qdrant.ScrollPoints(ctx, s.collection, 100, offset)
		if err != nil {
			return nil, fmt.Errorf("scroll points: %w", err)
		}

		for _, point := range scroll.Points {
			if getString(point.Payload, "source_code") == sourceCode {
				system := getString(point.Payload, "source_system")
				if sourceSystem == "" || system == sourceSystem {
					fb, err := payloadToFeedback(point.Payload)
					if err == nil {
						results = append(results, fb)
					}
				}
			}
		}

		if scroll.NextOffset == nil {
			break
		}
		offset = scroll.NextOffset
	}

	// Update cache
	s.mu.Lock()
	s.codeIndex[cacheKey] = results
	s.cacheTime = time.Now()
	s.mu.Unlock()

	return results, nil
}

// FindSimilar finds feedback with similar source display text.
func (s *FeedbackStore) FindSimilar(ctx context.Context, displayText string, vocab index.Vocabulary, limit int) ([]Feedback, error) {
	if s.embedder == nil {
		return nil, fmt.Errorf("embedding client not initialized")
	}

	// Generate embedding for the display text
	embedding, err := s.embedder.EmbedSingle(ctx, displayText)
	if err != nil {
		return nil, fmt.Errorf("embed text: %w", err)
	}

	// Search Qdrant
	hits, err := s.qdrant.Search(ctx, s.collection, embedding, limit*2, 0.5)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	// Convert hits to Feedback
	var results []Feedback
	for _, hit := range hits {
		fb, err := payloadToFeedback(hit.Payload)
		if err != nil {
			continue
		}

		// Filter by vocabulary
		if fb.TargetVocabulary != vocab {
			continue
		}

		fb.Similarity = hit.Score
		results = append(results, fb)

		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// IncrementAcceptCount increments the accept count for a feedback entry.
func (s *FeedbackStore) IncrementAcceptCount(ctx context.Context, feedbackID string) error {
	// Get current feedback
	points, err := s.qdrant.GetPoints(ctx, s.collection, []string{feedbackID})
	if err != nil {
		return fmt.Errorf("get feedback: %w", err)
	}

	if len(points) == 0 {
		return fmt.Errorf("feedback not found: %s", feedbackID)
	}

	fb, err := payloadToFeedback(points[0].Payload)
	if err != nil {
		return fmt.Errorf("parse feedback: %w", err)
	}

	fb.AcceptCount++
	fb.UpdatedAt = time.Now()

	// Re-record with updated count
	return s.Record(ctx, fb)
}

// Delete removes feedback by ID.
func (s *FeedbackStore) Delete(ctx context.Context, feedbackID string) error {
	if err := s.qdrant.DeletePoints(ctx, s.collection, []string{feedbackID}); err != nil {
		return fmt.Errorf("delete feedback: %w", err)
	}
	s.invalidateCache()
	return nil
}

// Stats returns statistics about the feedback store.
type FeedbackStats struct {
	TotalEntries   int64            `json:"total_entries"`
	AcceptedCount  int64            `json:"accepted_count"`
	RejectedCount  int64            `json:"rejected_count"`
	ByVocabulary   map[string]int64 `json:"by_vocabulary"`
	ByStrategy     map[string]int64 `json:"by_strategy"`
	AvgAcceptCount float64          `json:"avg_accept_count"`
}

// GetStats returns statistics about the feedback store.
func (s *FeedbackStore) GetStats(ctx context.Context) (*FeedbackStats, error) {
	info, err := s.qdrant.GetCollectionInfo(ctx, s.collection)
	if err != nil {
		return nil, fmt.Errorf("get collection info: %w", err)
	}

	stats := &FeedbackStats{
		TotalEntries: info.PointsCount,
		ByVocabulary: make(map[string]int64),
		ByStrategy:   make(map[string]int64),
	}

	// Scroll through to compute detailed stats
	var offset *string
	var totalAcceptCount int64
	for {
		scroll, err := s.qdrant.ScrollPoints(ctx, s.collection, 100, offset)
		if err != nil {
			break
		}

		for _, point := range scroll.Points {
			if getBool(point.Payload, "accepted") {
				stats.AcceptedCount++
			} else {
				stats.RejectedCount++
			}

			vocab := getString(point.Payload, "target_vocabulary")
			stats.ByVocabulary[vocab]++

			strategy := getString(point.Payload, "strategy")
			stats.ByStrategy[strategy]++

			acceptCount := getInt(point.Payload, "accept_count")
			totalAcceptCount += int64(acceptCount)
		}

		if scroll.NextOffset == nil {
			break
		}
		offset = scroll.NextOffset
	}

	if stats.AcceptedCount > 0 {
		stats.AvgAcceptCount = float64(totalAcceptCount) / float64(stats.AcceptedCount)
	}

	return stats, nil
}

// invalidateCache clears the in-memory cache.
func (s *FeedbackStore) invalidateCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codeIndex = make(map[string][]Feedback)
	s.cacheTime = time.Time{}
}

// payloadToFeedback converts a Qdrant payload to Feedback.
func payloadToFeedback(payload map[string]interface{}) (Feedback, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Feedback{}, err
	}

	var fb Feedback
	if err := json.Unmarshal(data, &fb); err != nil {
		return Feedback{}, err
	}

	return fb, nil
}

// generateFeedbackID generates a unique ID for feedback.
func generateFeedbackID(fb Feedback) string {
	return fmt.Sprintf("%s:%s:%s:%s",
		fb.SourceCode,
		fb.SourceSystem,
		fb.SuggestedCode,
		fb.TargetVocabulary)
}

// Helper functions for extracting values from payload

func getString(payload map[string]interface{}, key string) string {
	if v, ok := payload[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getBool(payload map[string]interface{}, key string) bool {
	if v, ok := payload[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func getInt(payload map[string]interface{}, key string) int {
	if v, ok := payload[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}
