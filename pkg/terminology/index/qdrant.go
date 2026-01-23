package index

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// QdrantClient provides operations for interacting with Qdrant vector database.
type QdrantClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewQdrantClient creates a new Qdrant client.
func NewQdrantClient(baseURL, apiKey string, timeout time.Duration) *QdrantClient {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &QdrantClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// CreateCollection creates a new Qdrant collection.
func (c *QdrantClient) CreateCollection(ctx context.Context, name string, dimensions int) error {
	payload := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     dimensions,
			"distance": "Cosine",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", c.baseURL+"/collections/"+name, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("create collection failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// DeleteCollection deletes a Qdrant collection.
func (c *QdrantClient) DeleteCollection(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+"/collections/"+name, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete collection failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// CollectionExists checks if a collection exists.
func (c *QdrantClient) CollectionExists(ctx context.Context, name string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/collections/"+name, nil)
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return false, fmt.Errorf("check collection failed (status %d): %s", resp.StatusCode, string(respBody))
}

// GetCollectionInfo returns information about a collection.
func (c *QdrantClient) GetCollectionInfo(ctx context.Context, name string) (*CollectionInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/collections/"+name, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get collection failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Result CollectionInfo `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result.Result, nil
}

// CollectionInfo contains information about a Qdrant collection.
type CollectionInfo struct {
	Status       string `json:"status"`
	PointsCount  int64  `json:"points_count"`
	VectorsCount int64  `json:"vectors_count"`
	Config       struct {
		Params struct {
			Vectors struct {
				Size     int    `json:"size"`
				Distance string `json:"distance"`
			} `json:"vectors"`
		} `json:"params"`
	} `json:"config"`
}

// UpsertPoints upserts points into a collection.
func (c *QdrantClient) UpsertPoints(ctx context.Context, collection string, points []Point) error {
	payload := map[string]interface{}{
		"points": points,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", c.baseURL+"/collections/"+collection+"/points?wait=true", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upsert points failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Point represents a Qdrant point.
type Point struct {
	ID      string                 `json:"id"`
	Vector  []float64              `json:"vector"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// Search performs a vector similarity search.
func (c *QdrantClient) Search(ctx context.Context, collection string, vector []float64, limit int, scoreThreshold float64) ([]SearchHit, error) {
	payload := map[string]interface{}{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}
	if scoreThreshold > 0 {
		payload["score_threshold"] = scoreThreshold
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/collections/"+collection+"/points/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Result []SearchHit `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return result.Result, nil
}

// SearchHit represents a search result from Qdrant.
type SearchHit struct {
	ID      string                 `json:"id"`
	Score   float64                `json:"score"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// GetPoints retrieves points by ID.
func (c *QdrantClient) GetPoints(ctx context.Context, collection string, ids []string) ([]Point, error) {
	payload := map[string]interface{}{
		"ids":          ids,
		"with_payload": true,
		"with_vector":  false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/collections/"+collection+"/points", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get points failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Result []Point `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return result.Result, nil
}

// DeletePoints deletes points by ID.
func (c *QdrantClient) DeletePoints(ctx context.Context, collection string, ids []string) error {
	payload := map[string]interface{}{
		"points": ids,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/collections/"+collection+"/points/delete?wait=true", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete points failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// ScrollPoints scrolls through all points in a collection.
func (c *QdrantClient) ScrollPoints(ctx context.Context, collection string, limit int, offset *string) (*ScrollResult, error) {
	payload := map[string]interface{}{
		"limit":        limit,
		"with_payload": true,
		"with_vector":  false,
	}
	if offset != nil {
		payload["offset"] = *offset
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/collections/"+collection+"/points/scroll", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scroll points failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Result ScrollResult `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result.Result, nil
}

// ScrollResult contains the result of a scroll operation.
type ScrollResult struct {
	Points     []Point `json:"points"`
	NextOffset *string `json:"next_page_offset,omitempty"`
}

// setHeaders sets common headers for Qdrant requests.
func (c *QdrantClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}
}

// Ping checks if Qdrant is reachable.
func (c *QdrantClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping failed with status %d", resp.StatusCode)
	}

	return nil
}
