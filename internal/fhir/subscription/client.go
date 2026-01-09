// Package subscription provides FHIR R4 Subscription support for bidirectional integration.
package subscription

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SubscriptionStatus represents the status of a FHIR Subscription.
type SubscriptionStatus string

const (
	StatusRequested SubscriptionStatus = "requested"
	StatusActive    SubscriptionStatus = "active"
	StatusError     SubscriptionStatus = "error"
	StatusOff       SubscriptionStatus = "off"
)

// ChannelType represents the delivery mechanism for notifications.
type ChannelType string

const (
	ChannelRestHook   ChannelType = "rest-hook"
	ChannelWebSocket  ChannelType = "websocket"
	ChannelEmail      ChannelType = "email"
	ChannelMessage    ChannelType = "message"
)

// Subscription represents a FHIR R4 Subscription resource.
type Subscription struct {
	ResourceType string             `json:"resourceType"`
	ID           string             `json:"id,omitempty"`
	Status       SubscriptionStatus `json:"status"`
	Reason       string             `json:"reason,omitempty"`
	Criteria     string             `json:"criteria"`
	Channel      Channel            `json:"channel"`
	End          *time.Time         `json:"end,omitempty"`
	Error        string             `json:"error,omitempty"`
}

// Channel defines how notifications are delivered.
type Channel struct {
	Type     ChannelType `json:"type"`
	Endpoint string      `json:"endpoint"`
	Payload  string      `json:"payload,omitempty"`
	Header   []string    `json:"header,omitempty"`
}

// Client manages FHIR Subscriptions on a server.
type Client struct {
	fhirEndpoint string
	httpClient   *http.Client
	authProvider AuthProvider

	mu            sync.RWMutex
	subscriptions map[string]*Subscription
}

// AuthProvider provides authentication for FHIR requests.
type AuthProvider interface {
	GetAuthHeader(ctx context.Context) (string, error)
}

// StaticTokenAuth provides a static Bearer token.
type StaticTokenAuth struct {
	Token string
}

func (a *StaticTokenAuth) GetAuthHeader(ctx context.Context) (string, error) {
	return "Bearer " + a.Token, nil
}

// ClientConfig configures the subscription client.
type ClientConfig struct {
	FHIREndpoint string
	HTTPClient   *http.Client
	AuthProvider AuthProvider
	Timeout      time.Duration
}

// NewClient creates a new FHIR Subscription client.
func NewClient(config *ClientConfig) (*Client, error) {
	if config.FHIREndpoint == "" {
		return nil, fmt.Errorf("FHIR endpoint is required")
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		timeout := config.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		httpClient = &http.Client{Timeout: timeout}
	}

	return &Client{
		fhirEndpoint:  strings.TrimSuffix(config.FHIREndpoint, "/"),
		httpClient:    httpClient,
		authProvider:  config.AuthProvider,
		subscriptions: make(map[string]*Subscription),
	}, nil
}

// Create registers a new subscription on the FHIR server.
func (c *Client) Create(ctx context.Context, sub *Subscription) (*Subscription, error) {
	sub.ResourceType = "Subscription"
	if sub.Status == "" {
		sub.Status = StatusRequested
	}

	body, err := json.Marshal(sub)
	if err != nil {
		return nil, fmt.Errorf("marshal subscription: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.fhirEndpoint+"/Subscription", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/fhir+json")
	req.Header.Set("Accept", "application/fhir+json")

	if err := c.addAuth(ctx, req); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var created Subscription
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	c.mu.Lock()
	c.subscriptions[created.ID] = &created
	c.mu.Unlock()

	return &created, nil
}

// Get retrieves a subscription by ID.
func (c *Client) Get(ctx context.Context, id string) (*Subscription, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.fhirEndpoint+"/Subscription/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/fhir+json")

	if err := c.addAuth(ctx, req); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var sub Subscription
	if err := json.NewDecoder(resp.Body).Decode(&sub); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &sub, nil
}

// List retrieves all subscriptions.
func (c *Client) List(ctx context.Context) ([]*Subscription, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.fhirEndpoint+"/Subscription", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/fhir+json")

	if err := c.addAuth(ctx, req); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var bundle struct {
		Entry []struct {
			Resource Subscription `json:"resource"`
		} `json:"entry"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&bundle); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	subs := make([]*Subscription, 0, len(bundle.Entry))
	for i := range bundle.Entry {
		subs = append(subs, &bundle.Entry[i].Resource)
	}

	return subs, nil
}

// Delete removes a subscription from the FHIR server.
func (c *Client) Delete(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.fhirEndpoint+"/Subscription/"+id, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if err := c.addAuth(ctx, req); err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	c.mu.Lock()
	delete(c.subscriptions, id)
	c.mu.Unlock()

	return nil
}

// UpdateStatus changes the status of a subscription.
func (c *Client) UpdateStatus(ctx context.Context, id string, status SubscriptionStatus) error {
	// First get the current subscription
	sub, err := c.Get(ctx, id)
	if err != nil {
		return err
	}

	sub.Status = status

	body, err := json.Marshal(sub)
	if err != nil {
		return fmt.Errorf("marshal subscription: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", c.fhirEndpoint+"/Subscription/"+id, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/fhir+json")
	req.Header.Set("Accept", "application/fhir+json")

	if err := c.addAuth(ctx, req); err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}

	return nil
}

// Pause sets a subscription status to "off".
func (c *Client) Pause(ctx context.Context, id string) error {
	return c.UpdateStatus(ctx, id, StatusOff)
}

// Resume sets a subscription status to "requested" (server will activate it).
func (c *Client) Resume(ctx context.Context, id string) error {
	return c.UpdateStatus(ctx, id, StatusRequested)
}

func (c *Client) addAuth(ctx context.Context, req *http.Request) error {
	if c.authProvider == nil {
		return nil
	}

	header, err := c.authProvider.GetAuthHeader(ctx)
	if err != nil {
		return fmt.Errorf("get auth header: %w", err)
	}

	req.Header.Set("Authorization", header)
	return nil
}

func (c *Client) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	// Try to parse as FHIR OperationOutcome
	var outcome struct {
		Issue []struct {
			Severity    string `json:"severity"`
			Code        string `json:"code"`
			Diagnostics string `json:"diagnostics"`
		} `json:"issue"`
	}

	if err := json.Unmarshal(body, &outcome); err == nil && len(outcome.Issue) > 0 {
		return &FHIRError{
			StatusCode:  resp.StatusCode,
			Severity:    outcome.Issue[0].Severity,
			Code:        outcome.Issue[0].Code,
			Diagnostics: outcome.Issue[0].Diagnostics,
		}
	}

	return &FHIRError{
		StatusCode:  resp.StatusCode,
		Diagnostics: string(body),
	}
}

// FHIRError represents an error from a FHIR server.
type FHIRError struct {
	StatusCode  int
	Severity    string
	Code        string
	Diagnostics string
}

func (e *FHIRError) Error() string {
	if e.Diagnostics != "" {
		return fmt.Sprintf("FHIR error (HTTP %d): %s", e.StatusCode, e.Diagnostics)
	}
	return fmt.Sprintf("FHIR error: HTTP %d", e.StatusCode)
}

// ErrNotFound indicates the subscription was not found.
var ErrNotFound = fmt.Errorf("subscription not found")
