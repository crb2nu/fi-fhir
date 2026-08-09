package observability

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

// Status is a component's reported state.
//
// The first three values mirror workflow.HealthStatus so the shipped check
// engine can aggregate them. StatusNotConfigured is additive: it is not a
// failure, but it is also not a claim that something is working. Aggregation
// treats it as non-failing because a deployment without an MLLP listener is a
// correct deployment, not a degraded one.
type Status string

const (
	// StatusHealthy means the dependency was contacted and answered.
	StatusHealthy Status = "healthy"
	// StatusDegraded means the component works with reduced guarantees.
	StatusDegraded Status = "degraded"
	// StatusUnhealthy means the dependency could not be used.
	StatusUnhealthy Status = "unhealthy"
	// StatusNotConfigured means the component is deliberately absent.
	StatusNotConfigured Status = "not_configured"
	// StatusStopped means a configured background component is no longer running.
	StatusStopped Status = "stopped"
)

// Component is one entry in a health or readiness report.
type Component struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Message string `json:"message,omitempty"`
}

// Report is the aggregated result of one probe.
type Report struct {
	Status     Status      `json:"status"`
	Version    string      `json:"version,omitempty"`
	Components []Component `json:"components,omitempty"`
	CheckedAt  time.Time   `json:"checked_at"`
}

// HTTPStatus maps a report onto the probe's HTTP status code. Degraded stays
// 200: a degraded replica can still serve traffic, and removing it from the
// Service would concentrate load on the remaining ones.
func (r Report) HTTPStatus() int {
	if r.Status == StatusUnhealthy {
		return http.StatusServiceUnavailable
	}
	return http.StatusOK
}

// Reporter is the read side of the health surface.
//
// The GraphQL `health` resolver and the HTTP probes both consume this, so the
// resolver stops answering from a hardcoded literal and starts projecting the
// same component set the probes see.
type Reporter interface {
	Live(ctx context.Context) Report
	Ready(ctx context.Context) Report
}

// ComponentState is a background component's lifecycle state, reported without
// contacting anything.
type ComponentState string

const (
	// ComponentRunning means the component's goroutine was started.
	ComponentRunning ComponentState = "running"
	// ComponentStopped means the component's goroutine returned.
	ComponentStopped ComponentState = "stopped"
	// ComponentNotConfigured means the component was never constructed.
	ComponentNotConfigured ComponentState = "not_configured"
)

// Health wires the already-shipped workflow.HealthService — which had zero
// non-test callers before this slice — into the serve process.
//
// Liveness stays process-only on purpose: a liveness probe that fails when a
// database is down converts a dependency outage into a pod restart loop, which
// is strictly worse than serving 503 from readiness.
type Health struct {
	svc     *workflow.HealthService
	version string

	mu     sync.RWMutex
	states map[string]ComponentState
}

// NewHealth builds the health surface. Timeout bounds every readiness check;
// the underlying service caches readiness for one second so a probe storm
// cannot amplify into a dependency storm.
func NewHealth(version string, timeout time.Duration) *Health {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	svc := workflow.NewHealthService(&workflow.HealthConfig{
		Version:        version,
		Timeout:        timeout,
		IncludeDetails: true,
	})
	svc.RegisterLivenessCheck("process", func(context.Context) workflow.ComponentHealth {
		return workflow.ComponentHealth{
			Status:    workflow.StatusHealthy,
			Message:   "process is serving",
			CheckedAt: time.Now().UTC(),
		}
	})
	return &Health{svc: svc, version: version, states: make(map[string]ComponentState)}
}

// RegisterReadiness adds a dependency-touching readiness check.
func (h *Health) RegisterReadiness(name string, check func(ctx context.Context) Component) {
	if h == nil || h.svc == nil || check == nil {
		return
	}
	h.svc.RegisterReadinessCheck(name, func(ctx context.Context) workflow.ComponentHealth {
		component := check(ctx)
		return workflow.ComponentHealth{
			Status:    workflow.HealthStatus(component.Status),
			Message:   component.Message,
			CheckedAt: time.Now().UTC(),
		}
	})
}

// RegisterDatabase adds a readiness check that pings a database handle. A nil
// handle reports "not configured" rather than "healthy", because the whole
// point of this slice is that an absent dependency must not read as a working
// one.
func (h *Health) RegisterDatabase(name string, db *sql.DB, absentReason string) {
	h.RegisterReadiness(name, func(ctx context.Context) Component {
		if db == nil {
			return Component{Status: StatusNotConfigured, Message: absentReason}
		}
		if err := db.PingContext(ctx); err != nil {
			return Component{Status: StatusUnhealthy, Message: "database is unreachable"}
		}
		return Component{Status: StatusHealthy, Message: "database responded to ping"}
	})
}

// RegisterPingable adds a readiness check for a store that may or may not
// expose a ping surface. A store without one reports "configured" — true, and
// distinguishable from a store that was actually contacted.
func (h *Health) RegisterPingable(name string, store any, absentReason string) {
	h.RegisterReadiness(name, func(ctx context.Context) Component {
		if store == nil {
			return Component{Status: StatusNotConfigured, Message: absentReason}
		}
		pinger, ok := store.(interface {
			PingContext(ctx context.Context) error
		})
		if !ok {
			return Component{Status: StatusHealthy, Message: "configured; store exposes no ping surface"}
		}
		if err := pinger.PingContext(ctx); err != nil {
			return Component{Status: StatusUnhealthy, Message: "store is unreachable"}
		}
		return Component{Status: StatusHealthy, Message: "store responded to ping"}
	})
}

// SetComponentState records a background component's lifecycle state and
// registers it as a readiness component the first time it is seen.
//
// The state table is the same one runServe's errCh / waitForBackgroundStops
// block drives, so readiness cannot disagree with what the process actually
// started.
func (h *Health) SetComponentState(name string, state ComponentState) {
	if h == nil {
		return
	}
	h.mu.Lock()
	_, known := h.states[name]
	h.states[name] = state
	h.mu.Unlock()
	// A lifecycle transition is exactly when a cached readiness report becomes a
	// lie, so drop the cache rather than serve a pre-transition answer.
	h.svc.InvalidateReadinessCache()
	if known {
		return
	}
	h.RegisterReadiness(name, func(context.Context) Component {
		h.mu.RLock()
		current := h.states[name]
		h.mu.RUnlock()
		switch current {
		case ComponentRunning:
			return Component{Status: StatusHealthy, Message: "component is running"}
		case ComponentStopped:
			// A configured component that stopped is a real readiness failure:
			// the replica is advertising an ingress it is no longer serving.
			return Component{Status: StatusUnhealthy, Message: "component stopped"}
		default:
			return Component{Status: StatusNotConfigured, Message: "component is not configured"}
		}
	})
}

// ComponentStates returns a copy of the lifecycle table, for metrics binding.
func (h *Health) ComponentStates() map[string]ComponentState {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[string]ComponentState, len(h.states))
	for name, state := range h.states {
		out[name] = state
	}
	return out
}

// Live runs the liveness checks.
func (h *Health) Live(ctx context.Context) Report {
	if h == nil || h.svc == nil {
		return Report{Status: StatusHealthy, CheckedAt: time.Now().UTC()}
	}
	return toReport(h.svc.CheckLiveness(ctx))
}

// Ready runs the readiness checks.
func (h *Health) Ready(ctx context.Context) Report {
	if h == nil || h.svc == nil {
		return Report{Status: StatusHealthy, CheckedAt: time.Now().UTC()}
	}
	return toReport(h.svc.CheckReadiness(ctx))
}

func toReport(response *workflow.HealthResponse) Report {
	if response == nil {
		return Report{Status: StatusUnhealthy, CheckedAt: time.Now().UTC()}
	}
	components := make([]Component, 0, len(response.Components))
	for _, component := range response.Components {
		components = append(components, Component{
			Name:    component.Name,
			Status:  Status(component.Status),
			Message: component.Message,
		})
	}
	// Stable ordering keeps the probe body diffable in evidence captures.
	sort.Slice(components, func(i, j int) bool { return components[i].Name < components[j].Name })
	return Report{
		Status:     Status(response.Status),
		Version:    response.Version,
		Components: components,
		CheckedAt:  response.CheckedAt.UTC(),
	}
}

// LivenessHandler serves the process-only liveness probe. It never returns 503
// for a dependency outage.
func LivenessHandler(reporter Reporter) http.Handler {
	return probeHandler(func(ctx context.Context) Report {
		report := reporter.Live(ctx)
		if report.Status == StatusUnhealthy {
			// Defensive: nothing registered as a liveness check may fail the
			// process, so downgrade rather than restart-loop a healthy pod.
			report.Status = StatusDegraded
		}
		return report
	})
}

// ReadinessHandler serves the dependency-touching readiness probe.
func ReadinessHandler(reporter Reporter) http.Handler {
	return probeHandler(reporter.Ready)
}

func probeHandler(run func(ctx context.Context) Report) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "health probes require GET", http.StatusMethodNotAllowed)
			return
		}
		report := run(r.Context())
		body, err := json.Marshal(report)
		if err != nil {
			http.Error(w, "health report is unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(report.HTTPStatus())
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(body)
	})
}

// LegacyHealthHandler reproduces the pre-slice `/health` byte-for-byte.
//
// It is reachable only under FI_FHIR_OBSERVABILITY_MODE=legacy and exists so
// the kill-test's negative control can demonstrate the exact behaviour this
// slice removes: 200 with `"status":"healthy"` while PostgreSQL is unreachable.
func LegacyHealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"healthy","service":"graphql"}`)
	})
}
