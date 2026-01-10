package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/crb2nu/fi-fhir/pkg/events"
	"github.com/crb2nu/fi-fhir/pkg/fhir"
)

// logAction logs an event to stdout/stderr.
func logAction(event interface{}, config map[string]string) error {
	level := config["level"]
	if level == "" {
		level = "info"
	}

	message := config["message"]
	if message == "" {
		message = "Event processed"
	}

	// Render template in message
	message = renderTemplate(message, event)

	// Format output
	timestamp := time.Now().Format(time.RFC3339)
	output := fmt.Sprintf("[%s] %s: %s", timestamp, strings.ToUpper(level), message)

	// Add event data if debug level
	if level == "debug" {
		eventJSON, _ := json.Marshal(event)
		output = fmt.Sprintf("%s\n  Event: %s", output, string(eventJSON))
	}

	// Write to appropriate output
	switch level {
	case "error", "warn":
		fmt.Fprintln(os.Stderr, output)
	default:
		fmt.Println(output)
	}

	return nil
}

// webhookAction sends an event to an HTTP endpoint with rate limiting, retry, and circuit breaker support.
// Config options:
//   - url: Target URL (required, supports templates)
//   - method: HTTP method (default: POST)
//   - token: Bearer token for authorization
//   - authorization: Custom Authorization header
//   - user_agent: Custom User-Agent header
//   - timeout: Request timeout duration (default: 30s)
//   - retry_max: Max retry attempts (default: 3, set 0 to disable)
//   - retry_delay: Initial retry delay (default: 1s)
//   - retry_max_delay: Max retry delay cap (default: 30s)
//   - retry_multiplier: Backoff multiplier (default: 2.0)
//   - retry_jitter: Jitter factor 0.0-1.0 (default: 0.1)
//   - retry_on_status: Comma-separated status codes to retry (default: 429,500,502,503,504)
//   - circuit_breaker: "true" to enable circuit breaker (default: disabled)
//   - circuit_failure_threshold: Failures before opening (default: 5)
//   - circuit_success_threshold: Successes in half-open before closing (default: 2)
//   - circuit_timeout: Time in open state before half-open (default: 30s)
//   - rate_limit: "true" to enable rate limiting (default: disabled)
//   - rate_limit_rate: Requests per second (default: 10)
//   - rate_limit_burst: Maximum burst size (default: 20)
//   - rate_limit_wait: "true" to wait when limited, "false" to fail fast (default: true)
func webhookAction(ctx context.Context, event interface{}, config map[string]string) error {
	url := config["url"]
	if url == "" {
		return fmt.Errorf("webhook action requires 'url' config")
	}

	method := config["method"]
	if method == "" {
		method = "POST"
	}

	// Render URL template
	url = renderTemplate(url, event)

	// Marshal event to JSON
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

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

	// Build headers once (reused for retries)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	if userAgent := config["user_agent"]; userAgent != "" {
		headers.Set("User-Agent", userAgent)
	} else {
		headers.Set("User-Agent", "fi-fhir/1.0")
	}
	if token := config["token"]; token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	if authHeader := config["authorization"]; authHeader != "" {
		headers.Set("Authorization", authHeader)
	}

	// Parse circuit breaker configuration (nil if disabled)
	cbConfig := ParseCircuitBreakerConfig(config)
	var cb *CircuitBreaker
	if cbConfig != nil {
		cb = GetCircuitBreaker(url)
	}

	// Parse rate limit configuration (nil if disabled)
	rlConfig := ParseRateLimitConfig(config)
	var limiter RateLimiter
	if rlConfig != nil {
		// Get or create limiter for this endpoint with the specified config
		limiter = GetOrCreateRateLimiter(url, *rlConfig)
	}
	waitOnRateLimit := ShouldWaitOnRateLimit(config)

	// Start HTTP span for distributed tracing
	tracer := GetGlobalTracer()
	httpCtx, httpSpan := tracer.StartSpan(ctx, SpanNameHTTP,
		WithSpanKind(SpanKindClient),
		WithAttributes(
			Attr(AttrHTTPMethod, method),
			Attr(AttrHTTPURL, url),
		),
	)
	defer httpSpan.End()

	// Execute request with rate limit, circuit breaker, and retry
	var resp *http.Response
	httpStart := time.Now()

	err = WithRateLimit(httpCtx, limiter, waitOnRateLimit, func() error {
		var innerErr error
		resp, innerErr = WithCircuitBreaker(cb, func() (*http.Response, error) {
			return WithRetry(nil, retryConfig, func() (*http.Response, error) {
				req, reqErr := http.NewRequestWithContext(httpCtx, method, url, bytes.NewReader(body))
				if reqErr != nil {
					return nil, fmt.Errorf("failed to create request: %w", reqErr)
				}
				req.Header = headers.Clone()
				return client.Do(req)
			})
		})
		return innerErr
	})

	httpDuration := time.Since(httpStart)

	if err != nil {
		// Record failed HTTP request metric (status code 0 for network errors)
		GetGlobalMetrics().HTTPRequestCompleted(url, method, 0, httpDuration)
		httpSpan.SetAttribute(AttrHTTPStatus, 0)
		httpSpan.RecordError(err)
		httpSpan.SetStatus(SpanStatusError, err.Error())
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	// Record span attributes and metrics
	httpSpan.SetAttribute(AttrHTTPStatus, resp.StatusCode)
	GetGlobalMetrics().HTTPRequestCompleted(url, method, resp.StatusCode, httpDuration)

	// Check response
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		httpSpan.SetStatus(SpanStatusError, fmt.Sprintf("HTTP %d", resp.StatusCode))
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
	}

	httpSpan.SetStatus(SpanStatusOK, "")
	return nil
}

// renderTemplate renders a Go template string with event data.
func renderTemplate(tmplStr string, data interface{}) string {
	// Check if template contains any placeholders
	if !strings.Contains(tmplStr, "{{") {
		return tmplStr
	}

	tmpl, err := template.New("msg").Parse(tmplStr)
	if err != nil {
		log.Printf("Warning: failed to parse template: %v", err)
		return tmplStr
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Printf("Warning: failed to execute template: %v", err)
		return tmplStr
	}

	return buf.String()
}

// fhirAction sends events to a FHIR R4 server.
// Config options:
//   - endpoint: FHIR server base URL (required)
//   - resource: Resource type to create (Patient, Encounter, Observation) - auto-detected if not specified
//   - operation: create, update, or upsert (default: create)
//   - profile: us-core or base (default: us-core)
//   - token: Bearer token for authentication (static)
//   - authorization: Custom Authorization header
//   - token_url: OAuth2 token endpoint (enables OAuth2 client credentials)
//   - client_id: OAuth2 client ID
//   - client_secret: OAuth2 client secret
//   - scopes: OAuth2 scopes (space or comma separated)
//   - timeout: Request timeout duration (default: 30s)
//   - bundle: "true" to send as transaction bundle
//   - circuit_breaker: "true" to enable circuit breaker (default: disabled)
//   - circuit_failure_threshold: Failures before opening (default: 5)
//   - circuit_success_threshold: Successes in half-open before closing (default: 2)
//   - circuit_timeout: Time in open state before half-open (default: 30s)
//   - rate_limit: "true" to enable rate limiting (default: disabled)
//   - rate_limit_rate: Requests per second (default: 10)
//   - rate_limit_burst: Maximum burst size (default: 20)
//   - rate_limit_wait: "true" to wait when limited, "false" to fail fast (default: true)
func fhirAction(ctx context.Context, event interface{}, config map[string]string) error {
	endpoint := config["endpoint"]
	if endpoint == "" {
		return fmt.Errorf("fhir action requires 'endpoint' config")
	}

	// Ensure endpoint doesn't have trailing slash
	endpoint = strings.TrimSuffix(endpoint, "/")

	// Create mapper (us-core is default)
	mapper := fhir.NewUSCoreMapper()

	// Convert event to FHIR resources
	resources, err := eventToFHIRResources(event, mapper, config)
	if err != nil {
		return fmt.Errorf("failed to convert event to FHIR: %w", err)
	}

	if len(resources) == 0 {
		return fmt.Errorf("no FHIR resources generated from event")
	}

	// Parse timeout
	timeout := 30 * time.Second
	if timeoutStr := config["timeout"]; timeoutStr != "" {
		if parsed, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsed
		}
	}

	client := &http.Client{Timeout: timeout}

	// Determine if we should send as bundle
	useBundle := strings.ToLower(config["bundle"]) == "true" || len(resources) > 1

	if useBundle {
		return sendFHIRBundle(ctx, client, endpoint, resources, config)
	}

	// Send individual resources
	for _, resource := range resources {
		if err := sendFHIRResource(ctx, client, endpoint, resource, config); err != nil {
			return err
		}
	}

	return nil
}

// eventToFHIRResources converts a canonical event to FHIR resources.
func eventToFHIRResources(event interface{}, mapper *fhir.USCoreMapper, config map[string]string) ([]fhir.Resource, error) {
	var resources []fhir.Resource

	// Handle map types (from JSON parsing in workflow engine)
	if m, ok := event.(map[string]interface{}); ok {
		return mapEventToFHIR(m, mapper, config)
	}

	// Handle typed events
	switch e := event.(type) {
	case *events.PatientAdmitEvent:
		patient := mapper.MapPatient(&e.Patient)
		if patient != nil {
			resources = append(resources, patient)
		}
		encounter := mapper.MapEncounter(&e.Encounter, fmt.Sprintf("Patient/%s", e.Patient.MRN))
		if encounter != nil {
			resources = append(resources, encounter)
		}

	case events.PatientAdmitEvent:
		patient := mapper.MapPatient(&e.Patient)
		if patient != nil {
			resources = append(resources, patient)
		}
		encounter := mapper.MapEncounter(&e.Encounter, fmt.Sprintf("Patient/%s", e.Patient.MRN))
		if encounter != nil {
			resources = append(resources, encounter)
		}

	case *events.PatientDischargeEvent:
		patient := mapper.MapPatient(&e.Patient)
		if patient != nil {
			resources = append(resources, patient)
		}
		encounter := mapper.MapEncounter(&e.Encounter, fmt.Sprintf("Patient/%s", e.Patient.MRN))
		if encounter != nil {
			resources = append(resources, encounter)
		}

	case events.PatientDischargeEvent:
		patient := mapper.MapPatient(&e.Patient)
		if patient != nil {
			resources = append(resources, patient)
		}
		encounter := mapper.MapEncounter(&e.Encounter, fmt.Sprintf("Patient/%s", e.Patient.MRN))
		if encounter != nil {
			resources = append(resources, encounter)
		}

	case *events.LabResultEvent:
		report, observations := mapper.MapLabResult(e)
		if report != nil {
			resources = append(resources, report)
		}
		for _, obs := range observations {
			resources = append(resources, obs)
		}

	case events.LabResultEvent:
		report, observations := mapper.MapLabResult(&e)
		if report != nil {
			resources = append(resources, report)
		}
		for _, obs := range observations {
			resources = append(resources, obs)
		}

	default:
		// Try to extract patient data from generic event
		return nil, fmt.Errorf("unsupported event type: %T", event)
	}

	return resources, nil
}

// mapEventToFHIR converts a map-based event (from JSON) to FHIR resources.
func mapEventToFHIR(m map[string]interface{}, mapper *fhir.USCoreMapper, config map[string]string) ([]fhir.Resource, error) {
	// Re-serialize to JSON and parse into typed event
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event map: %w", err)
	}

	// Determine event type
	eventType, _ := m["type"].(string)

	switch eventType {
	case string(events.EventPatientAdmit):
		var e events.PatientAdmitEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse patient_admit event: %w", err)
		}
		return eventToFHIRResources(&e, mapper, config)

	case string(events.EventPatientDischarge):
		var e events.PatientDischargeEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse patient_discharge event: %w", err)
		}
		return eventToFHIRResources(&e, mapper, config)

	case string(events.EventLabResult):
		var e events.LabResultEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse lab_result event: %w", err)
		}
		return eventToFHIRResources(&e, mapper, config)

	default:
		// Try to extract patient from generic event structure
		if patientData, ok := m["patient"].(map[string]interface{}); ok {
			patientJSON, _ := json.Marshal(patientData)
			var patient events.Patient
			if err := json.Unmarshal(patientJSON, &patient); err == nil {
				fhirPatient := mapper.MapPatient(&patient)
				if fhirPatient != nil {
					return []fhir.Resource{fhirPatient}, nil
				}
			}
		}
		return nil, fmt.Errorf("unsupported event type: %s", eventType)
	}
}

// sendFHIRResource sends a single FHIR resource to the server with rate limiting, retry, and circuit breaker support.
// Handles OAuth 401 by invalidating token cache and retrying with fresh token.
func sendFHIRResource(ctx context.Context, client *http.Client, endpoint string, resource fhir.Resource, config map[string]string) error {
	resourceType := resource.GetResourceType()
	operation := config["operation"]
	if operation == "" {
		operation = "create"
	}

	// Marshal resource to JSON
	body, err := json.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal FHIR resource: %w", err)
	}

	// Build request URL and method
	var reqURL, method string
	switch operation {
	case "create":
		reqURL = fmt.Sprintf("%s/%s", endpoint, resourceType)
		method = "POST"
	case "update":
		// For update, we need the resource ID
		reqURL = fmt.Sprintf("%s/%s", endpoint, resourceType)
		method = "PUT"
	case "upsert":
		// Upsert using conditional create (POST with If-None-Exist)
		reqURL = fmt.Sprintf("%s/%s", endpoint, resourceType)
		method = "POST"
	default:
		return fmt.Errorf("unknown operation: %s", operation)
	}

	// Build headers once (reused for retries)
	headers := http.Header{}
	headers.Set("Content-Type", "application/fhir+json")
	headers.Set("Accept", "application/fhir+json")
	headers.Set("User-Agent", "fi-fhir/1.0")

	// Parse retry configuration
	retryConfig := ParseRetryConfig(config)

	// Parse circuit breaker configuration (nil if disabled)
	cbConfig := ParseCircuitBreakerConfig(config)
	var cb *CircuitBreaker
	if cbConfig != nil {
		cb = GetCircuitBreaker(endpoint)
	}

	// Parse rate limit configuration (nil if disabled)
	rlConfig := ParseRateLimitConfig(config)
	var limiter RateLimiter
	if rlConfig != nil {
		// Get or create limiter for this endpoint with the specified config
		limiter = GetOrCreateRateLimiter(endpoint, *rlConfig)
	}
	waitOnRateLimit := ShouldWaitOnRateLimit(config)

	// Start HTTP span for distributed tracing
	tracer := GetGlobalTracer()
	httpCtx, httpSpan := tracer.StartSpan(ctx, SpanNameHTTP,
		WithSpanKind(SpanKindClient),
		WithAttributes(
			Attr(AttrHTTPMethod, method),
			Attr(AttrHTTPURL, reqURL),
			Attr("fhir.resource_type", resourceType),
			Attr("fhir.operation", operation),
		),
	)
	defer httpSpan.End()

	// Execute request with rate limit, circuit breaker, OAuth 401 handling, and retry
	var resp *http.Response
	httpStart := time.Now()

	err = WithRateLimit(httpCtx, limiter, waitOnRateLimit, func() error {
		var innerErr error
		resp, innerErr = WithCircuitBreaker(cb, func() (*http.Response, error) {
			return WithOAuthRetry(retryConfig, config,
				func() (*http.Request, error) {
					req, reqErr := http.NewRequestWithContext(httpCtx, method, reqURL, bytes.NewReader(body))
					if reqErr != nil {
						return nil, fmt.Errorf("failed to create FHIR request: %w", reqErr)
					}
					req.Header = headers.Clone()
					return req, nil
				},
				client.Do,
			)
		})
		return innerErr
	})

	httpDuration := time.Since(httpStart)

	if err != nil {
		// Record failed HTTP request metric (status code 0 for network errors)
		GetGlobalMetrics().HTTPRequestCompleted(reqURL, method, 0, httpDuration)
		httpSpan.SetAttribute(AttrHTTPStatus, 0)
		httpSpan.RecordError(err)
		httpSpan.SetStatus(SpanStatusError, err.Error())
		return fmt.Errorf("FHIR request failed: %w", err)
	}
	defer resp.Body.Close()

	// Record span attributes and metrics
	httpSpan.SetAttribute(AttrHTTPStatus, resp.StatusCode)
	GetGlobalMetrics().HTTPRequestCompleted(reqURL, method, resp.StatusCode, httpDuration)

	// Check response
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		httpSpan.SetStatus(SpanStatusError, fmt.Sprintf("FHIR %d", resp.StatusCode))
		return fmt.Errorf("FHIR server returned %d: %s", resp.StatusCode, string(respBody))
	}

	httpSpan.SetStatus(SpanStatusOK, "")
	return nil
}

// sendFHIRBundle sends multiple resources as a transaction bundle with rate limiting, retry, and circuit breaker support.
// Handles OAuth 401 by invalidating token cache and retrying with fresh token.
func sendFHIRBundle(ctx context.Context, client *http.Client, endpoint string, resources []fhir.Resource, config map[string]string) error {
	bundle := fhir.CreateTransactionBundle(resources)

	// Marshal bundle to JSON
	body, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("failed to marshal FHIR bundle: %w", err)
	}

	// Build headers once (reused for retries)
	headers := http.Header{}
	headers.Set("Content-Type", "application/fhir+json")
	headers.Set("Accept", "application/fhir+json")
	headers.Set("User-Agent", "fi-fhir/1.0")

	// Parse retry configuration
	retryConfig := ParseRetryConfig(config)

	// Parse circuit breaker configuration (nil if disabled)
	cbConfig := ParseCircuitBreakerConfig(config)
	var cb *CircuitBreaker
	if cbConfig != nil {
		cb = GetCircuitBreaker(endpoint)
	}

	// Parse rate limit configuration (nil if disabled)
	rlConfig := ParseRateLimitConfig(config)
	var limiter RateLimiter
	if rlConfig != nil {
		// Get or create limiter for this endpoint with the specified config
		limiter = GetOrCreateRateLimiter(endpoint, *rlConfig)
	}
	waitOnRateLimit := ShouldWaitOnRateLimit(config)

	// Start HTTP span for distributed tracing
	tracer := GetGlobalTracer()
	httpCtx, httpSpan := tracer.StartSpan(ctx, SpanNameHTTP,
		WithSpanKind(SpanKindClient),
		WithAttributes(
			Attr(AttrHTTPMethod, "POST"),
			Attr(AttrHTTPURL, endpoint),
			Attr("fhir.bundle_size", len(resources)),
		),
	)
	defer httpSpan.End()

	// Execute request with rate limit, circuit breaker, OAuth 401 handling, and retry
	var resp *http.Response
	httpStart := time.Now()

	err = WithRateLimit(httpCtx, limiter, waitOnRateLimit, func() error {
		var innerErr error
		resp, innerErr = WithCircuitBreaker(cb, func() (*http.Response, error) {
			return WithOAuthRetry(retryConfig, config,
				func() (*http.Request, error) {
					req, reqErr := http.NewRequestWithContext(httpCtx, "POST", endpoint, bytes.NewReader(body))
					if reqErr != nil {
						return nil, fmt.Errorf("failed to create FHIR bundle request: %w", reqErr)
					}
					req.Header = headers.Clone()
					return req, nil
				},
				client.Do,
			)
		})
		return innerErr
	})

	httpDuration := time.Since(httpStart)

	if err != nil {
		// Record failed HTTP request metric (status code 0 for network errors)
		GetGlobalMetrics().HTTPRequestCompleted(endpoint, "POST", 0, httpDuration)
		httpSpan.SetAttribute(AttrHTTPStatus, 0)
		httpSpan.RecordError(err)
		httpSpan.SetStatus(SpanStatusError, err.Error())
		return fmt.Errorf("FHIR bundle request failed: %w", err)
	}
	defer resp.Body.Close()

	// Record span attributes and metrics
	httpSpan.SetAttribute(AttrHTTPStatus, resp.StatusCode)
	GetGlobalMetrics().HTTPRequestCompleted(endpoint, "POST", resp.StatusCode, httpDuration)

	// Check response
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		httpSpan.SetStatus(SpanStatusError, fmt.Sprintf("FHIR %d", resp.StatusCode))
		return fmt.Errorf("FHIR server returned %d: %s", resp.StatusCode, string(respBody))
	}

	httpSpan.SetStatus(SpanStatusOK, "")
	return nil
}

// addAuth adds authentication to a request using OAuth2 or static token.
func addAuth(req *http.Request, config map[string]string) error {
	// Check for custom Authorization header first (highest priority)
	if authHeader := config["authorization"]; authHeader != "" {
		req.Header.Set("Authorization", authHeader)
		return nil
	}

	// Get token (OAuth2 or static)
	token, err := getAuthToken(config)
	if err != nil {
		return err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return nil
}
