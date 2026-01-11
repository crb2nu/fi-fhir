package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OAuthTokenManager handles OAuth2 client credentials token management.
type OAuthTokenManager struct {
	mu          sync.RWMutex
	tokens      map[string]*cachedToken
	client      *http.Client
	tokenBuffer time.Duration // Refresh token this much before expiry
	retryConfig RetryConfig   // Retry configuration for token fetch
}

// cachedToken stores a token with its expiration.
type cachedToken struct {
	AccessToken string
	ExpiresAt   time.Time
}

// OAuthConfig holds OAuth2 client credentials configuration.
type OAuthConfig struct {
	TokenURL     string   // Token endpoint URL
	ClientID     string   // Client ID
	ClientSecret string   // Client secret
	Scopes       []string // Optional scopes
}

// TokenResponse represents the OAuth2 token endpoint response.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"` // Seconds until expiry
	Scope       string `json:"scope,omitempty"`
}

// NewOAuthTokenManager creates a new token manager.
func NewOAuthTokenManager() *OAuthTokenManager {
	return &OAuthTokenManager{
		tokens:      make(map[string]*cachedToken),
		client:      &http.Client{Timeout: 30 * time.Second},
		tokenBuffer: 60 * time.Second, // Refresh 60s before expiry
		retryConfig: RetryConfig{
			MaxRetries:           3,
			InitialDelay:         500 * time.Millisecond,
			MaxDelay:             5 * time.Second,
			Multiplier:           2.0,
			Jitter:               0.1,
			RetryableStatusCodes: []int{408, 429, 500, 502, 503, 504},
		},
	}
}

// GetToken returns a valid access token, fetching a new one if necessary.
func (m *OAuthTokenManager) GetToken(config OAuthConfig) (string, error) {
	cacheKey := m.cacheKey(config)

	// Try to get cached token
	m.mu.RLock()
	cached, exists := m.tokens[cacheKey]
	m.mu.RUnlock()

	if exists && time.Now().Add(m.tokenBuffer).Before(cached.ExpiresAt) {
		return cached.AccessToken, nil
	}

	// Need to fetch a new token
	return m.fetchToken(config, cacheKey)
}

// fetchToken fetches a new token using client credentials grant with retry support.
func (m *OAuthTokenManager) fetchToken(config OAuthConfig, cacheKey string) (string, error) {
	// Build form data
	formData := url.Values{}
	formData.Set("grant_type", "client_credentials")
	formData.Set("client_id", config.ClientID)
	formData.Set("client_secret", config.ClientSecret)

	if len(config.Scopes) > 0 {
		formData.Set("scope", strings.Join(config.Scopes, " "))
	}

	formBody := formData.Encode()

	// Execute request with retry
	resp, err := WithRetry(context.Background(), m.retryConfig, func() (*http.Response, error) {
		req, err := http.NewRequest("POST", config.TokenURL, strings.NewReader(formBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create token request: %w", err)
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		return m.client.Do(req)
	})

	if err != nil {
		return "", fmt.Errorf("token request failed after retries: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("token response missing access_token")
	}

	// Calculate expiration
	expiresAt := time.Now()
	if tokenResp.ExpiresIn > 0 {
		expiresAt = expiresAt.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	} else {
		// Default to 1 hour if not specified
		expiresAt = expiresAt.Add(1 * time.Hour)
	}

	// Cache the token
	m.mu.Lock()
	m.tokens[cacheKey] = &cachedToken{
		AccessToken: tokenResp.AccessToken,
		ExpiresAt:   expiresAt,
	}
	m.mu.Unlock()

	return tokenResp.AccessToken, nil
}

// InvalidateToken removes a specific cached token, forcing refresh on next GetToken call.
// This is useful when a 401 is received, indicating the token was revoked server-side.
func (m *OAuthTokenManager) InvalidateToken(config OAuthConfig) {
	cacheKey := m.cacheKey(config)
	m.mu.Lock()
	delete(m.tokens, cacheKey)
	m.mu.Unlock()
}

// cacheKey generates a cache key from OAuth config.
func (m *OAuthTokenManager) cacheKey(config OAuthConfig) string {
	// Use token URL + client ID as key
	return config.TokenURL + ":" + config.ClientID
}

// ClearCache removes all cached tokens.
func (m *OAuthTokenManager) ClearCache() {
	m.mu.Lock()
	m.tokens = make(map[string]*cachedToken)
	m.mu.Unlock()
}

// Global token manager instance for the workflow package.
var globalTokenManager = NewOAuthTokenManager()

// GetOAuthToken is a convenience function using the global token manager.
func GetOAuthToken(config OAuthConfig) (string, error) {
	return globalTokenManager.GetToken(config)
}

// parseOAuthConfig extracts OAuth config from action config map.
func parseOAuthConfig(config map[string]string) *OAuthConfig {
	tokenURL := config["token_url"]
	clientID := config["client_id"]
	clientSecret := config["client_secret"]

	// All three are required for OAuth
	if tokenURL == "" || clientID == "" || clientSecret == "" {
		return nil
	}

	oauthConfig := &OAuthConfig{
		TokenURL:     tokenURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}

	// Parse optional scopes (space or comma separated)
	if scopes := config["scopes"]; scopes != "" {
		// Handle both space and comma separation
		scopes = strings.ReplaceAll(scopes, ",", " ")
		for _, s := range strings.Fields(scopes) {
			if s != "" {
				oauthConfig.Scopes = append(oauthConfig.Scopes, s)
			}
		}
	}

	return oauthConfig
}

// getAuthToken returns an authorization token from config, checking OAuth first, then static token.
func getAuthToken(config map[string]string) (string, error) {
	// Check for OAuth configuration first
	if oauthConfig := parseOAuthConfig(config); oauthConfig != nil {
		return GetOAuthToken(*oauthConfig)
	}

	// Fall back to static token
	if token := config["token"]; token != "" {
		return token, nil
	}

	// No authentication configured
	return "", nil
}

// WithOAuthRetry executes an HTTP request with OAuth 401 handling.
// If the request returns 401 and OAuth is configured, it invalidates the token
// cache and retries once with a fresh token.
// The requestFn is called to build the request (without auth), and authFn adds auth headers.
func WithOAuthRetry(
	retryConfig RetryConfig,
	config map[string]string,
	requestFn func() (*http.Request, error),
	doFn func(*http.Request) (*http.Response, error),
) (*http.Response, error) {
	oauthConfig := parseOAuthConfig(config)

	// Track if we've already retried due to 401
	retriedOn401 := false

	for {
		// Execute request with retry
		resp, err := WithRetry(context.Background(), retryConfig, func() (*http.Response, error) {
			req, err := requestFn()
			if err != nil {
				return nil, err
			}

			// Add authentication
			if err := addAuth(req, config); err != nil {
				return nil, fmt.Errorf("authentication failed: %w", err)
			}

			return doFn(req)
		})

		if err != nil {
			return nil, err
		}

		// Check for 401 Unauthorized with OAuth
		if resp.StatusCode == http.StatusUnauthorized && oauthConfig != nil && !retriedOn401 {
			// Close the response body before retrying
			_ = resp.Body.Close() // closing before retry, error not actionable

			// Invalidate the cached token
			globalTokenManager.InvalidateToken(*oauthConfig)

			// Mark that we've retried on 401 to prevent infinite loops
			retriedOn401 = true

			// Retry with fresh token
			continue
		}

		return resp, nil
	}
}
