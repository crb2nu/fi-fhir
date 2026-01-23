package index

import (
	"context"
	"sync"
)

// MockQdrantClient is a test double for QdrantClient operations.
type MockQdrantClient struct {
	// Collections stores mock collections with their points.
	Collections map[string]*mockCollection

	// SearchFunc is called when Search is invoked.
	// If nil, searches the mock collections.
	SearchFunc func(ctx context.Context, collection string, vector []float64, limit int, scoreThreshold float64) ([]SearchHit, error)

	// UpsertFunc is called when UpsertPoints is invoked.
	// If nil, stores points in the mock collection.
	UpsertFunc func(ctx context.Context, collection string, points []Point) error

	// Errors can be set to make specific methods return errors.
	Errors MockQdrantErrors

	// Calls records method invocations.
	Calls MockQdrantCalls

	mu sync.RWMutex
}

// MockQdrantErrors controls which methods return errors.
type MockQdrantErrors struct {
	Ping              error
	CreateCollection  error
	DeleteCollection  error
	CollectionExists  error
	GetCollectionInfo error
	Search            error
	UpsertPoints      error
	GetPoints         error
	DeletePoints      error
	ScrollPoints      error
}

// MockQdrantCalls records method invocations.
type MockQdrantCalls struct {
	CreateCollection []string
	DeleteCollection []string
	Search           []mockSearchCall
	UpsertPoints     []mockUpsertCall
	GetPoints        []mockGetPointsCall
}

type mockSearchCall struct {
	Collection     string
	Vector         []float64
	Limit          int
	ScoreThreshold float64
}

type mockUpsertCall struct {
	Collection string
	Points     []Point
}

type mockGetPointsCall struct {
	Collection string
	IDs        []string
}

type mockCollection struct {
	Dimensions int
	Points     map[string]Point
}

// NewMockQdrantClient creates a new MockQdrantClient.
func NewMockQdrantClient() *MockQdrantClient {
	return &MockQdrantClient{
		Collections: make(map[string]*mockCollection),
	}
}

// Ping checks connectivity (mock always succeeds unless error is set).
func (m *MockQdrantClient) Ping(ctx context.Context) error {
	if m.Errors.Ping != nil {
		return m.Errors.Ping
	}
	return nil
}

// CreateCollection creates a mock collection.
func (m *MockQdrantClient) CreateCollection(ctx context.Context, name string, dimensions int) error {
	if m.Errors.CreateCollection != nil {
		return m.Errors.CreateCollection
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.Calls.CreateCollection = append(m.Calls.CreateCollection, name)
	m.Collections[name] = &mockCollection{
		Dimensions: dimensions,
		Points:     make(map[string]Point),
	}
	return nil
}

// DeleteCollection deletes a mock collection.
func (m *MockQdrantClient) DeleteCollection(ctx context.Context, name string) error {
	if m.Errors.DeleteCollection != nil {
		return m.Errors.DeleteCollection
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.Calls.DeleteCollection = append(m.Calls.DeleteCollection, name)
	delete(m.Collections, name)
	return nil
}

// CollectionExists checks if a collection exists.
func (m *MockQdrantClient) CollectionExists(ctx context.Context, name string) (bool, error) {
	if m.Errors.CollectionExists != nil {
		return false, m.Errors.CollectionExists
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.Collections[name]
	return exists, nil
}

// GetCollectionInfo returns collection metadata.
func (m *MockQdrantClient) GetCollectionInfo(ctx context.Context, name string) (*CollectionInfo, error) {
	if m.Errors.GetCollectionInfo != nil {
		return nil, m.Errors.GetCollectionInfo
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	coll, exists := m.Collections[name]
	if !exists {
		return nil, ErrCollectionNotFound
	}

	info := &CollectionInfo{
		Status:       "green",
		PointsCount:  int64(len(coll.Points)),
		VectorsCount: int64(len(coll.Points)),
	}
	info.Config.Params.Vectors.Size = coll.Dimensions
	info.Config.Params.Vectors.Distance = "Cosine"
	return info, nil
}

// UpsertPoints adds or updates points in a collection.
func (m *MockQdrantClient) UpsertPoints(ctx context.Context, collection string, points []Point) error {
	if m.Errors.UpsertPoints != nil {
		return m.Errors.UpsertPoints
	}

	if m.UpsertFunc != nil {
		return m.UpsertFunc(ctx, collection, points)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.Calls.UpsertPoints = append(m.Calls.UpsertPoints, mockUpsertCall{
		Collection: collection,
		Points:     points,
	})

	coll, exists := m.Collections[collection]
	if !exists {
		// Auto-create collection
		dims := 0
		if len(points) > 0 && len(points[0].Vector) > 0 {
			dims = len(points[0].Vector)
		}
		coll = &mockCollection{
			Dimensions: dims,
			Points:     make(map[string]Point),
		}
		m.Collections[collection] = coll
	}

	for _, p := range points {
		coll.Points[p.ID] = p
	}
	return nil
}

// Search performs a vector similarity search.
func (m *MockQdrantClient) Search(ctx context.Context, collection string, vector []float64, limit int, scoreThreshold float64) ([]SearchHit, error) {
	if m.Errors.Search != nil {
		return nil, m.Errors.Search
	}

	m.mu.Lock()
	m.Calls.Search = append(m.Calls.Search, mockSearchCall{
		Collection:     collection,
		Vector:         vector,
		Limit:          limit,
		ScoreThreshold: scoreThreshold,
	})
	m.mu.Unlock()

	if m.SearchFunc != nil {
		return m.SearchFunc(ctx, collection, vector, limit, scoreThreshold)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	coll, exists := m.Collections[collection]
	if !exists {
		return nil, ErrCollectionNotFound
	}

	// Simple mock: return all points with score based on order
	var hits []SearchHit
	i := 0
	for id, p := range coll.Points {
		score := 1.0 - float64(i)*0.1
		if score < scoreThreshold {
			continue
		}
		hits = append(hits, SearchHit{
			ID:      id,
			Score:   score,
			Payload: p.Payload,
		})
		i++
		if len(hits) >= limit {
			break
		}
	}
	return hits, nil
}

// GetPoints retrieves points by ID.
func (m *MockQdrantClient) GetPoints(ctx context.Context, collection string, ids []string) ([]Point, error) {
	if m.Errors.GetPoints != nil {
		return nil, m.Errors.GetPoints
	}

	m.mu.Lock()
	m.Calls.GetPoints = append(m.Calls.GetPoints, mockGetPointsCall{
		Collection: collection,
		IDs:        ids,
	})
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()

	coll, exists := m.Collections[collection]
	if !exists {
		return nil, ErrCollectionNotFound
	}

	var points []Point
	for _, id := range ids {
		if p, ok := coll.Points[id]; ok {
			points = append(points, p)
		}
	}
	return points, nil
}

// DeletePoints removes points from a collection.
func (m *MockQdrantClient) DeletePoints(ctx context.Context, collection string, ids []string) error {
	if m.Errors.DeletePoints != nil {
		return m.Errors.DeletePoints
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	coll, exists := m.Collections[collection]
	if !exists {
		return nil
	}

	for _, id := range ids {
		delete(coll.Points, id)
	}
	return nil
}

// ScrollPoints returns paginated points.
func (m *MockQdrantClient) ScrollPoints(ctx context.Context, collection string, limit int, offset *string) (*ScrollResult, error) {
	if m.Errors.ScrollPoints != nil {
		return nil, m.Errors.ScrollPoints
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	coll, exists := m.Collections[collection]
	if !exists {
		return nil, ErrCollectionNotFound
	}

	// Simple implementation: return all points
	var points []Point
	for _, p := range coll.Points {
		points = append(points, p)
		if len(points) >= limit {
			break
		}
	}

	return &ScrollResult{
		Points:     points,
		NextOffset: nil,
	}, nil
}

// Reset clears all collections and calls.
func (m *MockQdrantClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Collections = make(map[string]*mockCollection)
	m.Calls = MockQdrantCalls{}
}

// AddMockCollection adds a pre-populated collection for testing.
func (m *MockQdrantClient) AddMockCollection(name string, dimensions int, points []Point) {
	m.mu.Lock()
	defer m.mu.Unlock()

	coll := &mockCollection{
		Dimensions: dimensions,
		Points:     make(map[string]Point),
	}
	for _, p := range points {
		coll.Points[p.ID] = p
	}
	m.Collections[name] = coll
}

// WithSearchResults configures specific search results.
func (m *MockQdrantClient) WithSearchResults(results []SearchHit) *MockQdrantClient {
	m.SearchFunc = func(ctx context.Context, collection string, vector []float64, limit int, scoreThreshold float64) ([]SearchHit, error) {
		if limit > 0 && len(results) > limit {
			return results[:limit], nil
		}
		return results, nil
	}
	return m
}

// ErrCollectionNotFound is returned when a collection doesn't exist.
var ErrCollectionNotFound = &collectionNotFoundError{}

type collectionNotFoundError struct{}

func (e *collectionNotFoundError) Error() string {
	return "collection not found"
}
