package index

import (
	"context"
	"errors"
	"testing"
)

func TestNewMockQdrantClient(t *testing.T) {
	m := NewMockQdrantClient()
	if m == nil {
		t.Fatal("expected non-nil mock")
	}
	if m.Collections == nil {
		t.Error("expected initialized Collections map")
	}
}

func TestMockQdrantClient_Ping(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		m := NewMockQdrantClient()
		if err := m.Ping(context.Background()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("with error", func(t *testing.T) {
		m := NewMockQdrantClient()
		m.Errors.Ping = errors.New("connection refused")
		if err := m.Ping(context.Background()); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestMockQdrantClient_CreateCollection(t *testing.T) {
	m := NewMockQdrantClient()
	ctx := context.Background()

	if err := m.CreateCollection(ctx, "test_coll", 768); err != nil {
		t.Fatalf("error: %v", err)
	}

	if len(m.Calls.CreateCollection) != 1 || m.Calls.CreateCollection[0] != "test_coll" {
		t.Errorf("calls=%v", m.Calls.CreateCollection)
	}
	coll, ok := m.Collections["test_coll"]
	if !ok {
		t.Fatal("collection not created")
	}
	if coll.Dimensions != 768 {
		t.Errorf("dimensions=%d", coll.Dimensions)
	}
}

func TestMockQdrantClient_CreateCollection_Error(t *testing.T) {
	m := NewMockQdrantClient()
	m.Errors.CreateCollection = errors.New("quota exceeded")

	if err := m.CreateCollection(context.Background(), "test", 768); err == nil {
		t.Fatal("expected error")
	}
}

func TestMockQdrantClient_DeleteCollection(t *testing.T) {
	m := NewMockQdrantClient()
	ctx := context.Background()

	m.Collections["to_delete"] = &mockCollection{Points: make(map[string]Point)}
	if err := m.DeleteCollection(ctx, "to_delete"); err != nil {
		t.Fatalf("error: %v", err)
	}
	if _, ok := m.Collections["to_delete"]; ok {
		t.Error("collection should be deleted")
	}
	if len(m.Calls.DeleteCollection) != 1 {
		t.Errorf("calls=%d", len(m.Calls.DeleteCollection))
	}
}

func TestMockQdrantClient_DeleteCollection_Error(t *testing.T) {
	m := NewMockQdrantClient()
	m.Errors.DeleteCollection = errors.New("fail")

	if err := m.DeleteCollection(context.Background(), "test"); err == nil {
		t.Fatal("expected error")
	}
}

func TestMockQdrantClient_CollectionExists(t *testing.T) {
	m := NewMockQdrantClient()
	ctx := context.Background()

	exists, err := m.CollectionExists(ctx, "missing")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if exists {
		t.Error("expected false for missing collection")
	}

	m.Collections["present"] = &mockCollection{Points: make(map[string]Point)}
	exists, err = m.CollectionExists(ctx, "present")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !exists {
		t.Error("expected true for present collection")
	}
}

func TestMockQdrantClient_CollectionExists_Error(t *testing.T) {
	m := NewMockQdrantClient()
	m.Errors.CollectionExists = errors.New("fail")

	_, err := m.CollectionExists(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockQdrantClient_GetCollectionInfo(t *testing.T) {
	m := NewMockQdrantClient()
	ctx := context.Background()

	m.AddMockCollection("test", 1024, []Point{
		{ID: "p1", Payload: map[string]interface{}{"code": "A"}},
	})

	info, err := m.GetCollectionInfo(ctx, "test")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if info.Status != "green" {
		t.Errorf("Status=%q", info.Status)
	}
	if info.PointsCount != 1 {
		t.Errorf("PointsCount=%d", info.PointsCount)
	}
	if info.Config.Params.Vectors.Size != 1024 {
		t.Errorf("Size=%d", info.Config.Params.Vectors.Size)
	}
}

func TestMockQdrantClient_GetCollectionInfo_NotFound(t *testing.T) {
	m := NewMockQdrantClient()
	_, err := m.GetCollectionInfo(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockQdrantClient_GetCollectionInfo_Error(t *testing.T) {
	m := NewMockQdrantClient()
	m.Errors.GetCollectionInfo = errors.New("fail")

	_, err := m.GetCollectionInfo(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockQdrantClient_UpsertPoints(t *testing.T) {
	m := NewMockQdrantClient()
	ctx := context.Background()

	points := []Point{
		{ID: "p1", Vector: []float64{0.1, 0.2}, Payload: map[string]interface{}{"code": "A"}},
		{ID: "p2", Vector: []float64{0.3, 0.4}, Payload: map[string]interface{}{"code": "B"}},
	}

	// Auto-creates collection
	if err := m.UpsertPoints(ctx, "auto_coll", points); err != nil {
		t.Fatalf("error: %v", err)
	}

	coll := m.Collections["auto_coll"]
	if coll == nil {
		t.Fatal("expected auto-created collection")
	}
	if len(coll.Points) != 2 {
		t.Errorf("points=%d want 2", len(coll.Points))
	}
	if len(m.Calls.UpsertPoints) != 1 {
		t.Errorf("calls=%d", len(m.Calls.UpsertPoints))
	}
}

func TestMockQdrantClient_UpsertPoints_CustomFunc(t *testing.T) {
	m := NewMockQdrantClient()
	called := false
	m.UpsertFunc = func(ctx context.Context, collection string, points []Point) error {
		called = true
		return nil
	}

	if err := m.UpsertPoints(context.Background(), "test", nil); err != nil {
		t.Fatalf("error: %v", err)
	}
	if !called {
		t.Error("UpsertFunc not called")
	}
}

func TestMockQdrantClient_UpsertPoints_Error(t *testing.T) {
	m := NewMockQdrantClient()
	m.Errors.UpsertPoints = errors.New("fail")

	if err := m.UpsertPoints(context.Background(), "test", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestMockQdrantClient_Search(t *testing.T) {
	m := NewMockQdrantClient()
	ctx := context.Background()

	m.AddMockCollection("test", 2, []Point{
		{ID: "p1", Vector: []float64{0.1, 0.2}, Payload: map[string]interface{}{"code": "A"}},
		{ID: "p2", Vector: []float64{0.3, 0.4}, Payload: map[string]interface{}{"code": "B"}},
	})

	hits, err := m.Search(ctx, "test", []float64{0.1, 0.2}, 10, 0.0)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(hits) != 2 {
		t.Errorf("hits=%d want 2", len(hits))
	}
	if len(m.Calls.Search) != 1 {
		t.Errorf("calls=%d", len(m.Calls.Search))
	}
}

func TestMockQdrantClient_Search_CustomFunc(t *testing.T) {
	m := NewMockQdrantClient()
	m.SearchFunc = func(ctx context.Context, collection string, vector []float64, limit int, scoreThreshold float64) ([]SearchHit, error) {
		return []SearchHit{{ID: "custom", Score: 0.99}}, nil
	}

	// Must have a collection for SearchFunc to be called (but actually SearchFunc is checked before collection lookup)
	hits, err := m.Search(context.Background(), "test", []float64{0.1}, 5, 0.0)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "custom" {
		t.Errorf("hits=%v", hits)
	}
}

func TestMockQdrantClient_Search_NotFound(t *testing.T) {
	m := NewMockQdrantClient()
	_, err := m.Search(context.Background(), "missing", []float64{0.1}, 5, 0.0)
	if err == nil {
		t.Fatal("expected error for missing collection")
	}
}

func TestMockQdrantClient_Search_Error(t *testing.T) {
	m := NewMockQdrantClient()
	m.Errors.Search = errors.New("fail")

	_, err := m.Search(context.Background(), "test", []float64{0.1}, 5, 0.0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockQdrantClient_GetPoints(t *testing.T) {
	m := NewMockQdrantClient()
	ctx := context.Background()

	m.AddMockCollection("test", 2, []Point{
		{ID: "p1", Payload: map[string]interface{}{"code": "A"}},
		{ID: "p2", Payload: map[string]interface{}{"code": "B"}},
	})

	points, err := m.GetPoints(ctx, "test", []string{"p1", "p3"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(points) != 1 {
		t.Errorf("points=%d want 1 (p1 found, p3 missing)", len(points))
	}
	if len(m.Calls.GetPoints) != 1 {
		t.Errorf("calls=%d", len(m.Calls.GetPoints))
	}
}

func TestMockQdrantClient_GetPoints_NotFound(t *testing.T) {
	m := NewMockQdrantClient()
	_, err := m.GetPoints(context.Background(), "missing", []string{"p1"})
	if err == nil {
		t.Fatal("expected error for missing collection")
	}
}

func TestMockQdrantClient_GetPoints_Error(t *testing.T) {
	m := NewMockQdrantClient()
	m.Errors.GetPoints = errors.New("fail")

	_, err := m.GetPoints(context.Background(), "test", []string{"p1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockQdrantClient_DeletePoints(t *testing.T) {
	m := NewMockQdrantClient()
	ctx := context.Background()

	m.AddMockCollection("test", 2, []Point{
		{ID: "p1", Payload: map[string]interface{}{"code": "A"}},
		{ID: "p2", Payload: map[string]interface{}{"code": "B"}},
	})

	if err := m.DeletePoints(ctx, "test", []string{"p1"}); err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(m.Collections["test"].Points) != 1 {
		t.Errorf("points=%d want 1", len(m.Collections["test"].Points))
	}
}

func TestMockQdrantClient_DeletePoints_MissingCollection(t *testing.T) {
	m := NewMockQdrantClient()
	// Deleting from missing collection is a no-op
	if err := m.DeletePoints(context.Background(), "missing", []string{"p1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMockQdrantClient_DeletePoints_Error(t *testing.T) {
	m := NewMockQdrantClient()
	m.Errors.DeletePoints = errors.New("fail")

	if err := m.DeletePoints(context.Background(), "test", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestMockQdrantClient_ScrollPoints(t *testing.T) {
	m := NewMockQdrantClient()
	ctx := context.Background()

	m.AddMockCollection("test", 2, []Point{
		{ID: "p1", Payload: map[string]interface{}{"code": "A"}},
	})

	result, err := m.ScrollPoints(ctx, "test", 10, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result.Points) != 1 {
		t.Errorf("points=%d want 1", len(result.Points))
	}
	if result.NextOffset != nil {
		t.Error("expected nil NextOffset")
	}
}

func TestMockQdrantClient_ScrollPoints_NotFound(t *testing.T) {
	m := NewMockQdrantClient()
	_, err := m.ScrollPoints(context.Background(), "missing", 10, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockQdrantClient_ScrollPoints_Error(t *testing.T) {
	m := NewMockQdrantClient()
	m.Errors.ScrollPoints = errors.New("fail")

	_, err := m.ScrollPoints(context.Background(), "test", 10, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockQdrantClient_Reset(t *testing.T) {
	m := NewMockQdrantClient()
	m.AddMockCollection("test", 768, nil)
	m.Calls.CreateCollection = append(m.Calls.CreateCollection, "test")

	m.Reset()

	if len(m.Collections) != 0 {
		t.Errorf("Collections=%d want 0", len(m.Collections))
	}
	if len(m.Calls.CreateCollection) != 0 {
		t.Errorf("Calls not cleared")
	}
}

func TestMockQdrantClient_AddMockCollection(t *testing.T) {
	m := NewMockQdrantClient()
	points := []Point{
		{ID: "p1", Vector: []float64{0.1}},
		{ID: "p2", Vector: []float64{0.2}},
	}
	m.AddMockCollection("test", 1, points)

	coll := m.Collections["test"]
	if coll == nil {
		t.Fatal("collection not added")
	}
	if coll.Dimensions != 1 {
		t.Errorf("dimensions=%d", coll.Dimensions)
	}
	if len(coll.Points) != 2 {
		t.Errorf("points=%d", len(coll.Points))
	}
}

func TestMockQdrantClient_WithSearchResults(t *testing.T) {
	m := NewMockQdrantClient()
	results := []SearchHit{
		{ID: "r1", Score: 0.9},
		{ID: "r2", Score: 0.8},
		{ID: "r3", Score: 0.7},
	}
	m.WithSearchResults(results)

	// With limit
	hits, err := m.Search(context.Background(), "test", []float64{0.1}, 2, 0.0)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(hits) != 2 {
		t.Errorf("hits=%d want 2 (limited)", len(hits))
	}

	// Without limit (0 means all)
	hits, err = m.Search(context.Background(), "test", []float64{0.1}, 0, 0.0)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(hits) != 3 {
		t.Errorf("hits=%d want 3", len(hits))
	}
}

func TestCollectionNotFoundError(t *testing.T) {
	err := ErrCollectionNotFound
	if err.Error() != "collection not found" {
		t.Errorf("Error()=%q", err.Error())
	}
}
