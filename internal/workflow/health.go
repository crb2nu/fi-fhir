package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HealthStatus represents the health state of a component.
type HealthStatus string

const (
	// StatusHealthy indicates the component is functioning normally.
	StatusHealthy HealthStatus = "healthy"
	// StatusDegraded indicates the component is functioning but with issues.
	StatusDegraded HealthStatus = "degraded"
	// StatusUnhealthy indicates the component is not functioning.
	StatusUnhealthy HealthStatus = "unhealthy"
)

// ComponentHealth represents the health of a single component.
type ComponentHealth struct {
	Name    string            `json:"name"`
	Status  HealthStatus      `json:"status"`
	Message string            `json:"message,omitempty"`
	Details map[string]string `json:"details,omitempty"`
	// CheckedAt is when this component was last checked
	CheckedAt time.Time `json:"checked_at"`
}

// HealthResponse represents the overall health check response.
type HealthResponse struct {
	Status     HealthStatus      `json:"status"`
	Version    string            `json:"version,omitempty"`
	Components []ComponentHealth `json:"components,omitempty"`
	CheckedAt  time.Time         `json:"checked_at"`
}

// HealthChecker is a function that checks the health of a component.
// It should return quickly (within a few seconds) to avoid blocking probes.
type HealthChecker func(ctx context.Context) ComponentHealth

// HealthConfig configures the health check system.
type HealthConfig struct {
	// Version is the application version to include in responses
	Version string

	// LivenessPath is the path for liveness probes (default: /health)
	LivenessPath string

	// ReadinessPath is the path for readiness probes (default: /ready)
	ReadinessPath string

	// Timeout is the maximum time to wait for health checks (default: 5s)
	Timeout time.Duration

	// IncludeDetails includes component details in responses (default: true)
	IncludeDetails bool
}

// DefaultHealthConfig returns sensible defaults.
func DefaultHealthConfig() *HealthConfig {
	return &HealthConfig{
		LivenessPath:   "/health",
		ReadinessPath:  "/ready",
		Timeout:        5 * time.Second,
		IncludeDetails: true,
	}
}

// HealthService manages health checks for the application.
type HealthService struct {
	config          *HealthConfig
	livenessChecks  map[string]HealthChecker
	readinessChecks map[string]HealthChecker
	mu              sync.RWMutex

	// Cache for readiness checks to avoid hammering dependencies
	cachedReadiness *HealthResponse
	cacheExpiry     time.Time
	cacheDuration   time.Duration
}

// NewHealthService creates a new health service.
func NewHealthService(config *HealthConfig) *HealthService {
	if config == nil {
		config = DefaultHealthConfig()
	}
	if config.LivenessPath == "" {
		config.LivenessPath = "/health"
	}
	if config.ReadinessPath == "" {
		config.ReadinessPath = "/ready"
	}
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}

	return &HealthService{
		config:          config,
		livenessChecks:  make(map[string]HealthChecker),
		readinessChecks: make(map[string]HealthChecker),
		cacheDuration:   time.Second, // Cache readiness for 1 second
	}
}

// RegisterLivenessCheck adds a check for liveness probes.
// Liveness checks should be lightweight - they indicate if the process is alive.
func (h *HealthService) RegisterLivenessCheck(name string, checker HealthChecker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.livenessChecks[name] = checker
}

// RegisterReadinessCheck adds a check for readiness probes.
// Readiness checks indicate if the service can handle traffic.
func (h *HealthService) RegisterReadinessCheck(name string, checker HealthChecker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.readinessChecks[name] = checker
}

// CheckLiveness runs all liveness checks and returns the aggregated result.
func (h *HealthService) CheckLiveness(ctx context.Context) *HealthResponse {
	return h.runChecks(ctx, h.livenessChecks)
}

// CheckReadiness runs all readiness checks and returns the aggregated result.
// Results are cached briefly to avoid overwhelming dependencies.
func (h *HealthService) CheckReadiness(ctx context.Context) *HealthResponse {
	h.mu.RLock()
	if h.cachedReadiness != nil && time.Now().Before(h.cacheExpiry) {
		cached := h.cachedReadiness
		h.mu.RUnlock()
		return cached
	}
	h.mu.RUnlock()

	result := h.runChecks(ctx, h.readinessChecks)

	h.mu.Lock()
	h.cachedReadiness = result
	h.cacheExpiry = time.Now().Add(h.cacheDuration)
	h.mu.Unlock()

	return result
}

func (h *HealthService) runChecks(ctx context.Context, checks map[string]HealthChecker) *HealthResponse {
	h.mu.RLock()
	checksCopy := make(map[string]HealthChecker, len(checks))
	for name, checker := range checks {
		checksCopy[name] = checker
	}
	h.mu.RUnlock()

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, h.config.Timeout)
	defer cancel()

	// Run checks concurrently
	results := make(chan ComponentHealth, len(checksCopy))
	var wg sync.WaitGroup

	for name, checker := range checksCopy {
		wg.Add(1)
		go func(name string, checker HealthChecker) {
			defer wg.Done()
			result := checker(ctx)
			result.Name = name
			if result.CheckedAt.IsZero() {
				result.CheckedAt = time.Now()
			}
			results <- result
		}(name, checker)
	}

	// Close results channel when all checks complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	components := make([]ComponentHealth, 0, len(checksCopy))
	overallStatus := StatusHealthy

	for component := range results {
		components = append(components, component)

		// Determine worst status
		switch component.Status {
		case StatusUnhealthy:
			overallStatus = StatusUnhealthy
		case StatusDegraded:
			if overallStatus != StatusUnhealthy {
				overallStatus = StatusDegraded
			}
		}
	}

	response := &HealthResponse{
		Status:    overallStatus,
		Version:   h.config.Version,
		CheckedAt: time.Now(),
	}

	if h.config.IncludeDetails {
		response.Components = components
	}

	return response
}

// Handler returns an HTTP handler for health endpoints.
// This can be mounted on an existing mux.
func (h *HealthService) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(h.config.LivenessPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		response := h.CheckLiveness(r.Context())
		h.writeResponse(w, response)
	})

	mux.HandleFunc(h.config.ReadinessPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		response := h.CheckReadiness(r.Context())
		h.writeResponse(w, response)
	})

	return mux
}

func (h *HealthService) writeResponse(w http.ResponseWriter, response *HealthResponse) {
	w.Header().Set("Content-Type", "application/json")

	statusCode := http.StatusOK
	switch response.Status {
	case StatusDegraded:
		statusCode = http.StatusOK // Still OK for degraded (service can handle traffic)
	case StatusUnhealthy:
		statusCode = http.StatusServiceUnavailable
	}

	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// ---- Built-in Health Checkers ----

// AlwaysHealthy returns a checker that always reports healthy.
// Useful for basic liveness checks.
func AlwaysHealthy() HealthChecker {
	return func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:    StatusHealthy,
			CheckedAt: time.Now(),
		}
	}
}

// CircuitBreakerHealthChecker checks if any circuit breakers are open.
func CircuitBreakerHealthChecker(endpoints ...string) HealthChecker {
	return func(ctx context.Context) ComponentHealth {
		openBreakers := []string{}

		for _, endpoint := range endpoints {
			cb := GetCircuitBreaker(endpoint)
			if cb != nil {
				state := cb.State()
				if state == CircuitOpen {
					openBreakers = append(openBreakers, endpoint)
				}
			}
		}

		if len(openBreakers) > 0 {
			return ComponentHealth{
				Status:  StatusDegraded,
				Message: fmt.Sprintf("%d circuit breaker(s) open", len(openBreakers)),
				Details: map[string]string{
					"open_breakers": fmt.Sprintf("%v", openBreakers),
				},
				CheckedAt: time.Now(),
			}
		}

		return ComponentHealth{
			Status:    StatusHealthy,
			Message:   "All circuit breakers closed",
			CheckedAt: time.Now(),
		}
	}
}

// DLQHealthChecker checks the dead letter queue depth.
func DLQHealthChecker(dlq DeadLetterQueue, warningThreshold, criticalThreshold int) HealthChecker {
	return func(ctx context.Context) ComponentHealth {
		if dlq == nil {
			return ComponentHealth{
				Status:    StatusHealthy,
				Message:   "DLQ not configured",
				CheckedAt: time.Now(),
			}
		}

		depth := dlq.Len()

		if depth >= criticalThreshold {
			return ComponentHealth{
				Status:  StatusUnhealthy,
				Message: fmt.Sprintf("DLQ depth critical: %d events", depth),
				Details: map[string]string{
					"depth":              fmt.Sprintf("%d", depth),
					"critical_threshold": fmt.Sprintf("%d", criticalThreshold),
				},
				CheckedAt: time.Now(),
			}
		}

		if depth >= warningThreshold {
			return ComponentHealth{
				Status:  StatusDegraded,
				Message: fmt.Sprintf("DLQ depth warning: %d events", depth),
				Details: map[string]string{
					"depth":             fmt.Sprintf("%d", depth),
					"warning_threshold": fmt.Sprintf("%d", warningThreshold),
				},
				CheckedAt: time.Now(),
			}
		}

		return ComponentHealth{
			Status:  StatusHealthy,
			Message: fmt.Sprintf("DLQ depth: %d events", depth),
			Details: map[string]string{
				"depth": fmt.Sprintf("%d", depth),
			},
			CheckedAt: time.Now(),
		}
	}
}

// HTTPHealthChecker checks if an HTTP endpoint is reachable.
func HTTPHealthChecker(url string, timeout time.Duration) HealthChecker {
	return func(ctx context.Context) ComponentHealth {
		if timeout == 0 {
			timeout = 5 * time.Second
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
		if err != nil {
			return ComponentHealth{
				Status:    StatusUnhealthy,
				Message:   fmt.Sprintf("Failed to create request: %v", err),
				Details:   map[string]string{"url": url},
				CheckedAt: time.Now(),
			}
		}

		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			return ComponentHealth{
				Status:    StatusUnhealthy,
				Message:   fmt.Sprintf("Request failed: %v", err),
				Details:   map[string]string{"url": url},
				CheckedAt: time.Now(),
			}
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 500 {
			return ComponentHealth{
				Status:  StatusUnhealthy,
				Message: fmt.Sprintf("Server error: %d", resp.StatusCode),
				Details: map[string]string{
					"url":         url,
					"status_code": fmt.Sprintf("%d", resp.StatusCode),
				},
				CheckedAt: time.Now(),
			}
		}

		if resp.StatusCode >= 400 {
			return ComponentHealth{
				Status:  StatusDegraded,
				Message: fmt.Sprintf("Client error: %d", resp.StatusCode),
				Details: map[string]string{
					"url":         url,
					"status_code": fmt.Sprintf("%d", resp.StatusCode),
				},
				CheckedAt: time.Now(),
			}
		}

		return ComponentHealth{
			Status:  StatusHealthy,
			Message: "Endpoint reachable",
			Details: map[string]string{
				"url":         url,
				"status_code": fmt.Sprintf("%d", resp.StatusCode),
			},
			CheckedAt: time.Now(),
		}
	}
}

// EngineHealthChecker checks the workflow engine state.
func EngineHealthChecker(engine *Engine) HealthChecker {
	return func(ctx context.Context) ComponentHealth {
		if engine == nil {
			return ComponentHealth{
				Status:    StatusUnhealthy,
				Message:   "Engine not initialized",
				CheckedAt: time.Now(),
			}
		}

		// Check if workflow is loaded
		if engine.workflow == nil {
			return ComponentHealth{
				Status:    StatusUnhealthy,
				Message:   "No workflow configured",
				CheckedAt: time.Now(),
			}
		}

		routeCount := len(engine.workflow.Routes)
		actionCount := len(engine.actions)

		return ComponentHealth{
			Status:  StatusHealthy,
			Message: "Engine operational",
			Details: map[string]string{
				"workflow_name":    engine.workflow.Name,
				"workflow_version": engine.workflow.Version,
				"route_count":      fmt.Sprintf("%d", routeCount),
				"action_count":     fmt.Sprintf("%d", actionCount),
			},
			CheckedAt: time.Now(),
		}
	}
}

// CustomHealthChecker creates a checker from a simple function.
func CustomHealthChecker(name string, check func(ctx context.Context) (bool, string)) HealthChecker {
	return func(ctx context.Context) ComponentHealth {
		healthy, message := check(ctx)
		status := StatusHealthy
		if !healthy {
			status = StatusUnhealthy
		}
		return ComponentHealth{
			Name:      name,
			Status:    status,
			Message:   message,
			CheckedAt: time.Now(),
		}
	}
}
