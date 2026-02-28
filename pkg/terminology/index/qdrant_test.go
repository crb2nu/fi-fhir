package index

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(handler http.Handler) (*QdrantClient, *httptest.Server) {
	ts := httptest.NewServer(handler)
	client := NewQdrantClient(ts.URL, "test-key", 5*time.Second)
	return client, ts
}

func TestNewQdrantClient(t *testing.T) {
	c := NewQdrantClient("http://localhost:6333/", "my-key", 0)
	if c.baseURL != "http://localhost:6333" {
		t.Errorf("baseURL=%q, trailing slash not trimmed", c.baseURL)
	}
	if c.apiKey != "my-key" {
		t.Errorf("apiKey=%q", c.apiKey)
	}
	if c.httpClient.Timeout != 30*time.Second {
		t.Errorf("default timeout=%v want 30s", c.httpClient.Timeout)
	}
}

func TestNewQdrantClient_CustomTimeout(t *testing.T) {
	c := NewQdrantClient("http://localhost:6333", "", 10*time.Second)
	if c.httpClient.Timeout != 10*time.Second {
		t.Errorf("timeout=%v want 10s", c.httpClient.Timeout)
	}
}

func TestQdrantClient_Ping(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				t.Errorf("path=%q want /", r.URL.Path)
			}
			if r.Method != "GET" {
				t.Errorf("method=%q want GET", r.Method)
			}
			// Note: Ping does not call setHeaders, so no api-key is sent
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		if err := client.Ping(context.Background()); err != nil {
			t.Fatalf("Ping error: %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer ts.Close()

		if err := client.Ping(context.Background()); err == nil {
			t.Fatal("expected error on 503")
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		c := NewQdrantClient("http://127.0.0.1:1", "", 1*time.Second)
		if err := c.Ping(context.Background()); err == nil {
			t.Fatal("expected error on connection refused")
		}
	})
}

func TestQdrantClient_CreateCollection(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "PUT" {
				t.Errorf("method=%q want PUT", r.Method)
			}
			if !strings.HasSuffix(r.URL.Path, "/collections/test_coll") {
				t.Errorf("path=%q", r.URL.Path)
			}

			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			vectors := payload["vectors"].(map[string]interface{})
			if vectors["size"].(float64) != 768 {
				t.Errorf("size=%v", vectors["size"])
			}

			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		if err := client.CreateCollection(context.Background(), "test_coll", 768); err != nil {
			t.Fatalf("CreateCollection error: %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer ts.Close()

		err := client.CreateCollection(context.Background(), "test", 768)
		if err == nil {
			t.Fatal("expected error on 500")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("error=%v, want mention of status 500", err)
		}
	})
}

func TestQdrantClient_DeleteCollection(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "DELETE" {
				t.Errorf("method=%q want DELETE", r.Method)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		if err := client.DeleteCollection(context.Background(), "test"); err != nil {
			t.Fatalf("DeleteCollection error: %v", err)
		}
	})

	t.Run("not found is ok", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ts.Close()

		if err := client.DeleteCollection(context.Background(), "missing"); err != nil {
			t.Fatalf("expected no error for 404: %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
		}))
		defer ts.Close()

		if err := client.DeleteCollection(context.Background(), "test"); err == nil {
			t.Fatal("expected error on 500")
		}
	})
}

func TestQdrantClient_CollectionExists(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		exists, err := client.CollectionExists(context.Background(), "test")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if !exists {
			t.Error("expected exists=true")
		}
	})

	t.Run("not found", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ts.Close()

		exists, err := client.CollectionExists(context.Background(), "test")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if exists {
			t.Error("expected exists=false")
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("fail"))
		}))
		defer ts.Close()

		_, err := client.CollectionExists(context.Background(), "test")
		if err == nil {
			t.Fatal("expected error on 500")
		}
	})
}

func TestQdrantClient_GetCollectionInfo(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := map[string]interface{}{
				"result": map[string]interface{}{
					"status":        "green",
					"points_count":  1000,
					"vectors_count": 1000,
					"config": map[string]interface{}{
						"params": map[string]interface{}{
							"vectors": map[string]interface{}{
								"size":     1024,
								"distance": "Cosine",
							},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer ts.Close()

		info, err := client.GetCollectionInfo(context.Background(), "test")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if info.Status != "green" {
			t.Errorf("Status=%q", info.Status)
		}
		if info.PointsCount != 1000 {
			t.Errorf("PointsCount=%d", info.PointsCount)
		}
		if info.Config.Params.Vectors.Size != 1024 {
			t.Errorf("Vectors.Size=%d", info.Config.Params.Vectors.Size)
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("not found"))
		}))
		defer ts.Close()

		_, err := client.GetCollectionInfo(context.Background(), "missing")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("{invalid}"))
		}))
		defer ts.Close()

		_, err := client.GetCollectionInfo(context.Background(), "test")
		if err == nil {
			t.Fatal("expected error on invalid JSON")
		}
	})
}

func TestQdrantClient_UpsertPoints(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "PUT" {
				t.Errorf("method=%q want PUT", r.Method)
			}
			if !strings.Contains(r.URL.String(), "wait=true") {
				t.Error("expected wait=true query param")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		points := []Point{
			{ID: "p1", Vector: []float64{0.1, 0.2}, Payload: map[string]interface{}{"code": "123"}},
		}
		if err := client.UpsertPoints(context.Background(), "test", points); err != nil {
			t.Fatalf("error: %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("fail"))
		}))
		defer ts.Close()

		if err := client.UpsertPoints(context.Background(), "test", nil); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestQdrantClient_Search(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("method=%q want POST", r.Method)
			}

			var payload map[string]interface{}
			json.NewDecoder(r.Body).Decode(&payload)
			if payload["with_payload"] != true {
				t.Error("expected with_payload=true")
			}

			resp := map[string]interface{}{
				"result": []map[string]interface{}{
					{"id": "loinc:123", "score": 0.95, "payload": map[string]interface{}{"code": "123"}},
					{"id": "loinc:456", "score": 0.85, "payload": map[string]interface{}{"code": "456"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer ts.Close()

		hits, err := client.Search(context.Background(), "test", []float64{0.1, 0.2}, 10, 0.0)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(hits) != 2 {
			t.Fatalf("hits=%d want 2", len(hits))
		}
		if hits[0].ID != "loinc:123" {
			t.Errorf("hits[0].ID=%q", hits[0].ID)
		}
		if hits[0].Score != 0.95 {
			t.Errorf("hits[0].Score=%f", hits[0].Score)
		}
	})

	t.Run("with score threshold", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var payload map[string]interface{}
			json.NewDecoder(r.Body).Decode(&payload)
			if _, ok := payload["score_threshold"]; !ok {
				t.Error("expected score_threshold in payload")
			}
			resp := map[string]interface{}{"result": []map[string]interface{}{}}
			json.NewEncoder(w).Encode(resp)
		}))
		defer ts.Close()

		_, err := client.Search(context.Background(), "test", []float64{0.1}, 5, 0.5)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
		}))
		defer ts.Close()

		_, err := client.Search(context.Background(), "test", []float64{0.1}, 5, 0.0)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid json response", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("{bad"))
		}))
		defer ts.Close()

		_, err := client.Search(context.Background(), "test", []float64{0.1}, 5, 0.0)
		if err == nil {
			t.Fatal("expected error on invalid JSON")
		}
	})
}

func TestQdrantClient_GetPoints(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("method=%q want POST", r.Method)
			}

			var payload map[string]interface{}
			json.NewDecoder(r.Body).Decode(&payload)
			if payload["with_vector"] != false {
				t.Error("expected with_vector=false")
			}

			resp := map[string]interface{}{
				"result": []map[string]interface{}{
					{"id": "p1", "payload": map[string]interface{}{"code": "A"}},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer ts.Close()

		points, err := client.GetPoints(context.Background(), "test", []string{"p1"})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(points) != 1 {
			t.Fatalf("points=%d want 1", len(points))
		}
		if points[0].ID != "p1" {
			t.Errorf("ID=%q", points[0].ID)
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("fail"))
		}))
		defer ts.Close()

		_, err := client.GetPoints(context.Background(), "test", []string{"p1"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestQdrantClient_DeletePoints(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("method=%q want POST", r.Method)
			}
			if !strings.Contains(r.URL.String(), "wait=true") {
				t.Error("expected wait=true")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		if err := client.DeletePoints(context.Background(), "test", []string{"p1", "p2"}); err != nil {
			t.Fatalf("error: %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("fail"))
		}))
		defer ts.Close()

		if err := client.DeletePoints(context.Background(), "test", []string{"p1"}); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestQdrantClient_ScrollPoints(t *testing.T) {
	t.Run("success without offset", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("method=%q want POST", r.Method)
			}

			var payload map[string]interface{}
			json.NewDecoder(r.Body).Decode(&payload)
			if _, ok := payload["offset"]; ok {
				t.Error("expected no offset when nil")
			}

			next := "page2"
			resp := map[string]interface{}{
				"result": map[string]interface{}{
					"points": []map[string]interface{}{
						{"id": "p1", "payload": map[string]interface{}{"code": "A"}},
					},
					"next_page_offset": next,
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer ts.Close()

		result, err := client.ScrollPoints(context.Background(), "test", 10, nil)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(result.Points) != 1 {
			t.Errorf("points=%d want 1", len(result.Points))
		}
		if result.NextOffset == nil || *result.NextOffset != "page2" {
			t.Errorf("NextOffset=%v", result.NextOffset)
		}
	})

	t.Run("with offset", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var payload map[string]interface{}
			json.NewDecoder(r.Body).Decode(&payload)
			if payload["offset"] != "page2" {
				t.Errorf("offset=%v want page2", payload["offset"])
			}

			resp := map[string]interface{}{
				"result": map[string]interface{}{
					"points": []map[string]interface{}{},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer ts.Close()

		offset := "page2"
		result, err := client.ScrollPoints(context.Background(), "test", 10, &offset)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if result.NextOffset != nil {
			t.Errorf("expected nil NextOffset, got %v", *result.NextOffset)
		}
	})

	t.Run("server error", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("fail"))
		}))
		defer ts.Close()

		_, err := client.ScrollPoints(context.Background(), "test", 10, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		client, ts := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("{bad json"))
		}))
		defer ts.Close()

		_, err := client.ScrollPoints(context.Background(), "test", 10, nil)
		if err == nil {
			t.Fatal("expected error on invalid JSON")
		}
	})
}

func TestQdrantClient_SetHeaders(t *testing.T) {
	t.Run("with api key", func(t *testing.T) {
		c := NewQdrantClient("http://localhost", "secret", 5*time.Second)
		req, _ := http.NewRequest("GET", "http://localhost", nil)
		c.setHeaders(req)

		if req.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type=%q", req.Header.Get("Content-Type"))
		}
		if req.Header.Get("api-key") != "secret" {
			t.Errorf("api-key=%q", req.Header.Get("api-key"))
		}
	})

	t.Run("without api key", func(t *testing.T) {
		c := NewQdrantClient("http://localhost", "", 5*time.Second)
		req, _ := http.NewRequest("GET", "http://localhost", nil)
		c.setHeaders(req)

		if req.Header.Get("api-key") != "" {
			t.Errorf("api-key should be empty, got %q", req.Header.Get("api-key"))
		}
	})
}
