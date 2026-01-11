//nolint:gosec // Test file - G104 errors intentionally ignored in test setup
package workflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthServiceDefaults(t *testing.T) {
	svc := NewHealthService(nil)

	if svc.config.LivenessPath != "/health" {
		t.Errorf("Expected /health, got %s", svc.config.LivenessPath)
	}
	if svc.config.ReadinessPath != "/ready" {
		t.Errorf("Expected /ready, got %s", svc.config.ReadinessPath)
	}
	if svc.config.Timeout != 5*time.Second {
		t.Errorf("Expected 5s timeout, got %v", svc.config.Timeout)
	}
}

func TestHealthServiceLiveness(t *testing.T) {
	svc := NewHealthService(&HealthConfig{
		Version:        "1.0.0",
		IncludeDetails: true,
	})

	// Register a healthy check
	svc.RegisterLivenessCheck("app", AlwaysHealthy())

	response := svc.CheckLiveness(context.Background())

	if response.Status != StatusHealthy {
		t.Errorf("Expected healthy, got %s", response.Status)
	}
	if response.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", response.Version)
	}
	if len(response.Components) != 1 {
		t.Errorf("Expected 1 component, got %d", len(response.Components))
	}
	if response.Components[0].Name != "app" {
		t.Errorf("Expected component name 'app', got %s", response.Components[0].Name)
	}
}

func TestHealthServiceUnhealthy(t *testing.T) {
	svc := NewHealthService(&HealthConfig{IncludeDetails: true})

	// Register an unhealthy check
	svc.RegisterLivenessCheck("failing", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:  StatusUnhealthy,
			Message: "Component failed",
		}
	})

	response := svc.CheckLiveness(context.Background())

	if response.Status != StatusUnhealthy {
		t.Errorf("Expected unhealthy, got %s", response.Status)
	}
}

func TestHealthServiceDegraded(t *testing.T) {
	svc := NewHealthService(&HealthConfig{IncludeDetails: true})

	// Register healthy and degraded checks
	svc.RegisterReadinessCheck("healthy", AlwaysHealthy())
	svc.RegisterReadinessCheck("degraded", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:  StatusDegraded,
			Message: "Running slow",
		}
	})

	response := svc.CheckReadiness(context.Background())

	if response.Status != StatusDegraded {
		t.Errorf("Expected degraded, got %s", response.Status)
	}
}

func TestHealthServiceUnhealthyOverridesDegraded(t *testing.T) {
	svc := NewHealthService(&HealthConfig{IncludeDetails: true})

	svc.RegisterReadinessCheck("degraded", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{Status: StatusDegraded}
	})
	svc.RegisterReadinessCheck("unhealthy", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{Status: StatusUnhealthy}
	})

	response := svc.CheckReadiness(context.Background())

	if response.Status != StatusUnhealthy {
		t.Errorf("Expected unhealthy to override degraded, got %s", response.Status)
	}
}

func TestHealthServiceHTTPHandler(t *testing.T) {
	svc := NewHealthService(&HealthConfig{
		Version:        "2.0.0",
		IncludeDetails: true,
	})
	svc.RegisterLivenessCheck("app", AlwaysHealthy())
	svc.RegisterReadinessCheck("app", AlwaysHealthy())

	handler := svc.Handler()

	t.Run("liveness endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}

		var response HealthResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if response.Status != StatusHealthy {
			t.Errorf("Expected healthy, got %s", response.Status)
		}
		if response.Version != "2.0.0" {
			t.Errorf("Expected version 2.0.0, got %s", response.Version)
		}
	})

	t.Run("readiness endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}
	})

	t.Run("unhealthy returns 503", func(t *testing.T) {
		unhealthySvc := NewHealthService(nil)
		unhealthySvc.RegisterLivenessCheck("failing", func(ctx context.Context) ComponentHealth {
			return ComponentHealth{Status: StatusUnhealthy}
		})

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()

		unhealthySvc.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected 503, got %d", rec.Code)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/health", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected 405, got %d", rec.Code)
		}
	})
}

func TestHealthServiceCaching(t *testing.T) {
	callCount := 0
	svc := NewHealthService(nil)
	svc.RegisterReadinessCheck("counter", func(ctx context.Context) ComponentHealth {
		callCount++
		return ComponentHealth{Status: StatusHealthy}
	})

	// First call
	svc.CheckReadiness(context.Background())
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}

	// Second call should use cache
	svc.CheckReadiness(context.Background())
	if callCount != 1 {
		t.Errorf("Expected cached result (still 1 call), got %d", callCount)
	}

	// Wait for cache to expire
	time.Sleep(1100 * time.Millisecond)

	// Third call should run check again
	svc.CheckReadiness(context.Background())
	if callCount != 2 {
		t.Errorf("Expected 2 calls after cache expiry, got %d", callCount)
	}
}

func TestHealthServiceNoDetails(t *testing.T) {
	svc := NewHealthService(&HealthConfig{
		IncludeDetails: false,
	})
	svc.RegisterLivenessCheck("app", AlwaysHealthy())

	response := svc.CheckLiveness(context.Background())

	if response.Components != nil {
		t.Error("Expected no components when IncludeDetails is false")
	}
}

func TestDLQHealthChecker(t *testing.T) {
	dlq := NewMemoryDLQ()

	checker := DLQHealthChecker(dlq, 10, 100)

	t.Run("healthy when empty", func(t *testing.T) {
		result := checker(context.Background())
		if result.Status != StatusHealthy {
			t.Errorf("Expected healthy, got %s", result.Status)
		}
	})

	t.Run("degraded at warning threshold", func(t *testing.T) {
		// Add events to reach warning threshold
		for i := 0; i < 15; i++ {
			dlq.Push(&FailedEvent{ID: string(rune('a' + i))})
		}

		result := checker(context.Background())
		if result.Status != StatusDegraded {
			t.Errorf("Expected degraded, got %s", result.Status)
		}
	})

	t.Run("unhealthy at critical threshold", func(t *testing.T) {
		// Add more events to reach critical threshold
		for i := 0; i < 100; i++ {
			dlq.Push(&FailedEvent{ID: string(rune(i + 100))})
		}

		result := checker(context.Background())
		if result.Status != StatusUnhealthy {
			t.Errorf("Expected unhealthy, got %s", result.Status)
		}
	})

	t.Run("nil DLQ returns healthy", func(t *testing.T) {
		nilChecker := DLQHealthChecker(nil, 10, 100)
		result := nilChecker(context.Background())
		if result.Status != StatusHealthy {
			t.Errorf("Expected healthy for nil DLQ, got %s", result.Status)
		}
	})
}

func TestEngineHealthChecker(t *testing.T) {
	t.Run("nil engine", func(t *testing.T) {
		checker := EngineHealthChecker(nil)
		result := checker(context.Background())
		if result.Status != StatusUnhealthy {
			t.Errorf("Expected unhealthy for nil engine, got %s", result.Status)
		}
	})

	t.Run("valid engine", func(t *testing.T) {
		wf := &Workflow{
			Name:    "test-workflow",
			Version: "1.0",
			Routes: []Route{
				{Name: "route1"},
				{Name: "route2"},
			},
		}
		engine, err := NewEngine(wf)
		if err != nil {
			t.Fatalf("NewEngine() error = %v", err)
		}

		checker := EngineHealthChecker(engine)
		result := checker(context.Background())

		if result.Status != StatusHealthy {
			t.Errorf("Expected healthy, got %s", result.Status)
		}
		if result.Details["workflow_name"] != "test-workflow" {
			t.Errorf("Expected workflow_name test-workflow, got %s", result.Details["workflow_name"])
		}
		if result.Details["route_count"] != "2" {
			t.Errorf("Expected route_count 2, got %s", result.Details["route_count"])
		}
	})
}

func TestCircuitBreakerHealthChecker(t *testing.T) {
	// Use unique endpoints for each test to avoid global state issues
	t.Run("healthy when no breakers exist", func(t *testing.T) {
		// Use an endpoint that hasn't been accessed yet
		checker := CircuitBreakerHealthChecker("http://nonexistent-endpoint-12345.example.com")
		result := checker(context.Background())
		// GetCircuitBreaker creates a new breaker, so it will be closed (healthy)
		if result.Status != StatusHealthy {
			t.Errorf("Expected healthy, got %s", result.Status)
		}
	})

	t.Run("healthy when breaker closed", func(t *testing.T) {
		endpoint := "http://cb-test-closed.example.com"
		cb := GetCircuitBreaker(endpoint)
		cb.RecordSuccess()

		checker := CircuitBreakerHealthChecker(endpoint)
		result := checker(context.Background())
		if result.Status != StatusHealthy {
			t.Errorf("Expected healthy, got %s", result.Status)
		}
	})

	t.Run("degraded when breaker open", func(t *testing.T) {
		endpoint := "http://cb-test-open.example.com"
		// Force circuit open by recording many failures
		cb := GetCircuitBreaker(endpoint)
		for i := 0; i < 10; i++ {
			cb.RecordFailure()
		}

		checker := CircuitBreakerHealthChecker(endpoint)
		result := checker(context.Background())
		if result.Status != StatusDegraded {
			t.Errorf("Expected degraded, got %s", result.Status)
		}
	})
}

func TestHTTPHealthChecker(t *testing.T) {
	t.Run("healthy endpoint", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		checker := HTTPHealthChecker(server.URL, time.Second)
		result := checker(context.Background())

		if result.Status != StatusHealthy {
			t.Errorf("Expected healthy, got %s: %s", result.Status, result.Message)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		checker := HTTPHealthChecker(server.URL, time.Second)
		result := checker(context.Background())

		if result.Status != StatusUnhealthy {
			t.Errorf("Expected unhealthy, got %s", result.Status)
		}
	})

	t.Run("client error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		checker := HTTPHealthChecker(server.URL, time.Second)
		result := checker(context.Background())

		if result.Status != StatusDegraded {
			t.Errorf("Expected degraded, got %s", result.Status)
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		checker := HTTPHealthChecker("http://localhost:59999", 100*time.Millisecond)
		result := checker(context.Background())

		if result.Status != StatusUnhealthy {
			t.Errorf("Expected unhealthy, got %s", result.Status)
		}
	})
}

func TestCustomHealthChecker(t *testing.T) {
	t.Run("returns healthy", func(t *testing.T) {
		checker := CustomHealthChecker("custom", func(ctx context.Context) (bool, string) {
			return true, "All good"
		})

		result := checker(context.Background())
		if result.Status != StatusHealthy {
			t.Errorf("Expected healthy, got %s", result.Status)
		}
		if result.Message != "All good" {
			t.Errorf("Expected 'All good', got %s", result.Message)
		}
	})

	t.Run("returns unhealthy", func(t *testing.T) {
		checker := CustomHealthChecker("custom", func(ctx context.Context) (bool, string) {
			return false, "Something wrong"
		})

		result := checker(context.Background())
		if result.Status != StatusUnhealthy {
			t.Errorf("Expected unhealthy, got %s", result.Status)
		}
	})
}

func TestHealthServiceConcurrentChecks(t *testing.T) {
	svc := NewHealthService(&HealthConfig{
		Timeout:        2 * time.Second,
		IncludeDetails: true,
	})

	// Register multiple slow checks
	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		svc.RegisterReadinessCheck(name, func(ctx context.Context) ComponentHealth {
			time.Sleep(100 * time.Millisecond)
			return ComponentHealth{Status: StatusHealthy}
		})
	}

	start := time.Now()
	response := svc.CheckReadiness(context.Background())
	elapsed := time.Since(start)

	// Should complete in ~100ms (concurrent), not 500ms (sequential)
	if elapsed > 300*time.Millisecond {
		t.Errorf("Checks should run concurrently, took %v", elapsed)
	}

	if len(response.Components) != 5 {
		t.Errorf("Expected 5 components, got %d", len(response.Components))
	}
}

func TestHealthServiceTimeout(t *testing.T) {
	svc := NewHealthService(&HealthConfig{
		Timeout:        100 * time.Millisecond,
		IncludeDetails: true,
	})

	svc.RegisterLivenessCheck("slow", func(ctx context.Context) ComponentHealth {
		select {
		case <-time.After(5 * time.Second):
			return ComponentHealth{Status: StatusHealthy}
		case <-ctx.Done():
			return ComponentHealth{
				Status:  StatusUnhealthy,
				Message: "Check timed out",
			}
		}
	})

	start := time.Now()
	response := svc.CheckLiveness(context.Background())
	elapsed := time.Since(start)

	// Should timeout quickly
	if elapsed > 200*time.Millisecond {
		t.Errorf("Should have timed out, took %v", elapsed)
	}

	// Slow check should have received context cancellation
	if response.Status != StatusUnhealthy {
		t.Errorf("Expected unhealthy due to timeout, got %s", response.Status)
	}
}
