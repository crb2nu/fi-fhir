package workflow

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// athenaAction sends an event/payload to the Athenahealth proprietary REST API.
// It includes full support for Athena's OAuth2 client credentials flow, circuit breaking,
// and rate limiting per-Practice ID.
//
// Config options:
//   - athena_practiceid: Practice ID (required, supports templates)
//   - athena_endpoint: Target operation endpoint e.g., /patients (required, supports templates)
//   - athena_base_url: Base API URL (default: https://api.preview.platform.athenahealth.com/v1)
//   - method: HTTP method (default: POST)
//   - content_type: Content-Type header (default: application/x-www-form-urlencoded, common for Athena)
//   - payload: Rendered payload string (required, typically url-encoded string built from a template)
//   - oauth_token_url: OAuth endpoint for client credentials (required if using Auth)
//   - client_id: Athena OAuth Client ID
//   - client_secret: Athena OAuth Client Secret
//   - timeout, retry_max, circuit_breaker... (Standard HTTP configs)
func athenaAction(ctx context.Context, event interface{}, config map[string]string) error {
	rawPracticeID := config["athena_practiceid"]
	if rawPracticeID == "" {
		return fmt.Errorf("athena action requires 'athena_practiceid' config")
	}
	practiceID := renderTemplate(rawPracticeID, event)

	rawEndpoint := config["athena_endpoint"]
	if rawEndpoint == "" {
		return fmt.Errorf("athena action requires 'athena_endpoint' config")
	}
	endpoint := renderTemplate(rawEndpoint, event)

	baseURL := config["athena_base_url"]
	if baseURL == "" {
		baseURL = "https://api.preview.platform.athenahealth.com/v1"
	}

	url := fmt.Sprintf("%s/%s%s", baseURL, practiceID, endpoint)

	method := config["method"]
	if method == "" {
		method = "POST"
	}

	// Payload comes typically rendered from a workflow template
	rawPayload := config["payload"]
	body := []byte(renderTemplate(rawPayload, event))

	// Parse timeout
	timeout := 30 * time.Second
	if timeoutStr := config["timeout"]; timeoutStr != "" {
		if parsed, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsed
		}
	}

	client := &http.Client{Timeout: timeout}

	// Parse retry configuration
	retryConfig := ParseRetryConfig(config)

	// Determine content-type (Athena uses x-www-form-urlencoded extensively)
	contentType := config["content_type"]
	if contentType == "" {
		contentType = "application/x-www-form-urlencoded"
	}

	// Build headers once
	headers := http.Header{}
	headers.Set("Content-Type", contentType)
	headers.Set("User-Agent", "fi-fhir/1.0 (Athena Connector)")

	// Rate limiter
	rlConfig := ParseRateLimitConfig(config)
	var limiter RateLimiter
	if rlConfig != nil {
		limiter = GetOrCreateRateLimiter(baseURL, *rlConfig) // limit against Athena base
	}
	waitOnRateLimit := ShouldWaitOnRateLimit(config)

	// Circuit breaker
	cbConfig := ParseCircuitBreakerConfig(config)
	var cb *CircuitBreaker
	if cbConfig != nil {
		cb = GetCircuitBreaker(baseURL)
	}

	// Distributed Tracing Span
	tracer := GetGlobalTracer()
	httpCtx, httpSpan := tracer.StartSpan(ctx, "http_athena_api",
		WithSpanKind(SpanKindClient),
		WithAttributes(
			Attr(AttrHTTPMethod, method),
			Attr(AttrHTTPURL, url),
			Attr("athena.practice_id", practiceID),
		),
	)
	defer httpSpan.End()

	var resp *http.Response
	httpStart := time.Now()

	// Execute with Standard Resilience Patterns
	err := WithRateLimit(httpCtx, limiter, waitOnRateLimit, func() error {
		var innerErr error
		resp, innerErr = WithCircuitBreaker(cb, func() (*http.Response, error) {

			// OAuth Support via existing WithOAuthRetry logic (handles 401s transparently)
			return WithOAuthRetry(retryConfig, config, func() (*http.Request, error) {
				req, reqErr := http.NewRequestWithContext(httpCtx, method, url, bytes.NewReader(body))
				if reqErr != nil {
					return nil, fmt.Errorf("failed to create athena request: %w", reqErr)
				}
				req.Header = headers.Clone()
				return req, nil
			}, client.Do)
		})
		return innerErr
	})

	httpDuration := time.Since(httpStart)

	if err != nil {
		GetGlobalMetrics().HTTPRequestCompleted(url, method, 0, httpDuration)
		httpSpan.SetAttribute(AttrHTTPStatus, 0)
		httpSpan.RecordError(err)
		httpSpan.SetStatus(SpanStatusError, err.Error())
		return fmt.Errorf("athena api request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	httpSpan.SetAttribute(AttrHTTPStatus, resp.StatusCode)
	GetGlobalMetrics().HTTPRequestCompleted(url, method, resp.StatusCode, httpDuration)

	// Check response
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		httpSpan.SetStatus(SpanStatusError, fmt.Sprintf("Athena API %d", resp.StatusCode))
		return fmt.Errorf("athena API returned error %d: %s", resp.StatusCode, string(respBody))
	}

	httpSpan.SetStatus(SpanStatusOK, "")
	return nil
}
