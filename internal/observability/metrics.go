package observability

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Outcome is a bounded metric label value.
//
// Every counter in this package is labelled by an Outcome drawn from the
// constants below and nothing else. That is the cardinality contract: a new
// outcome is a code change and a review, not a runtime surprise, and no
// message-derived string can ever reach a label.
type Outcome string

const (
	// OutcomeAccepted is a message the engine durably admitted.
	OutcomeAccepted Outcome = "accepted"
	// OutcomeRejected is a message the engine refused on policy or validation.
	OutcomeRejected Outcome = "rejected"
	// OutcomeError is an infrastructure failure, not a caller error.
	OutcomeError Outcome = "error"
	// OutcomeDelivered is a successful outbound delivery or notification.
	OutcomeDelivered Outcome = "delivered"
	// OutcomeRetried is an attempt scheduled for another try.
	OutcomeRetried Outcome = "retried"
	// OutcomeFailed is a terminal failure of one unit of work.
	OutcomeFailed Outcome = "failed"
	// OutcomeIdle is a poll that found no work.
	OutcomeIdle Outcome = "idle"
	// OutcomeProcessed is a unit of work completed.
	OutcomeProcessed Outcome = "processed"
	// OutcomePublished is a stream event written to the durable log.
	OutcomePublished Outcome = "published"
	// OutcomeDropped is work discarded by a bounded buffer.
	OutcomeDropped Outcome = "dropped"
	// OutcomeReplayed is a stream event delivered from the durable log.
	OutcomeReplayed Outcome = "replayed"
	// OutcomeQueued is a notification accepted onto the dispatch queue.
	OutcomeQueued Outcome = "queued"
	// OutcomeSkipped is work another replica had already claimed.
	OutcomeSkipped Outcome = "skipped"
	// OutcomePurged is a retained record removed or tombstoned by retention policy.
	OutcomePurged Outcome = "purged"
)

// allOutcomes is the complete label allowlist. The PHI-label test asserts that
// every observed label value is a member, so an accidental
// `WithLabelValues(correlationID)` fails the build rather than shipping.
var allOutcomes = map[string]struct{}{
	string(OutcomeAccepted): {}, string(OutcomeRejected): {}, string(OutcomeError): {},
	string(OutcomeDelivered): {}, string(OutcomeRetried): {}, string(OutcomeFailed): {},
	string(OutcomeIdle): {}, string(OutcomeProcessed): {}, string(OutcomePublished): {},
	string(OutcomeDropped): {}, string(OutcomeReplayed): {}, string(OutcomeQueued): {},
	string(OutcomeSkipped): {}, string(OutcomePurged): {},
}

// KnownOutcome reports whether a label value is in the allowlist.
func KnownOutcome(value string) bool {
	_, ok := allOutcomes[value]
	return ok
}

// Component names. These are the only values that appear in a `component`
// label and are the same names runServe's background component table uses.
const (
	ComponentGraphQL          = "graphql"
	ComponentMetrics          = "metrics"
	ComponentMLLP             = "mllp"
	ComponentDelivery         = "delivery"
	ComponentBatch            = "batch"
	ComponentAutorouteSweep   = "autoroute_sweep"
	ComponentAutorouteNotify  = "autoroute_notify"
	ComponentSessionStream    = "session_stream"
	ComponentSubmissionDB     = "submission_db"
	ComponentTerminologyDB    = "terminology_db"
	ComponentSessionStore     = "session_store"
	ComponentProfileStore     = "profile_store"
	ComponentWorkflowStore    = "workflow_lifecycle_store"
	ComponentEventStore       = "event_store"
	ComponentMappingStore     = "mapping_store"
	ComponentProcessLiveness  = "process"
	ComponentLifecycleCatalog = "lifecycle_catalog"
	ComponentRetentionPurge   = "retention_purge"
)

// Metrics owns the one Prometheus registry the serve process exposes.
//
// It deliberately does not reuse internal/workflow's Prometheus adapter: that
// adapter's metric names are `workflow_*` and its interface is shaped for the
// legacy engine the durable integration path never executes. Emitting
// `workflow_*` names from integration-engine code would be a naming lie of the
// same class this slice exists to remove. See `.loom/40-decisions.md`
// (2026-08-08), rejected option C.
type Metrics struct {
	registry *prometheus.Registry

	buildInfo   *prometheus.GaugeVec
	componentUp *prometheus.GaugeVec
	readinessUp *prometheus.GaugeVec

	ingressSubmissions     *prometheus.CounterVec
	mllpMessages           *prometheus.CounterVec
	deliveryAttempts       *prometheus.CounterVec
	batchObjects           *prometheus.CounterVec
	sessionStreamEvents    *prometheus.CounterVec
	autorouteSweeps        *prometheus.CounterVec
	autorouteExpired       prometheus.Counter
	autorouteNotifications *prometheus.CounterVec
	retentionPurges        *prometheus.CounterVec
	retentionRecordsPurged *prometheus.CounterVec

	mu sync.Mutex
}

// NewMetrics builds the registry with the process collectors plus this
// program's own metrics. Go and process collectors are included because a
// replica's goroutine count and RSS are the first two questions asked during a
// multi-replica incident.
func NewMetrics(version string) *Metrics {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		registry: registry,
		buildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "fi_fhir_build_info",
			Help: "Build information for the running fi-fhir process; always 1.",
		}, []string{"version"}),
		componentUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "fi_fhir_component_up",
			Help: "1 when a background component is running, 0 when configured but stopped. Absent when not configured.",
		}, []string{"component"}),
		readinessUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "fi_fhir_readiness_up",
			Help: "1 when a readiness component reported healthy or not configured, 0 when it reported unhealthy.",
		}, []string{"component"}),
		ingressSubmissions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fi_fhir_http_ingress_submissions_total",
			Help: "Authenticated HL7v2 HTTP ingress submissions by outcome.",
		}, []string{"outcome"}),
		mllpMessages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fi_fhir_mllp_messages_total",
			Help: "MLLP frames handled by the listener, by outcome.",
		}, []string{"outcome"}),
		deliveryAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fi_fhir_delivery_attempts_total",
			Help: "Durable delivery dispatch cycles by outcome.",
		}, []string{"outcome"}),
		batchObjects: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fi_fhir_batch_objects_total",
			Help: "Batch ingestion object cycles by outcome.",
		}, []string{"outcome"}),
		sessionStreamEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fi_fhir_session_stream_events_total",
			Help: "Integration Session stream events by outcome.",
		}, []string{"outcome"}),
		autorouteSweeps: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fi_fhir_autoroute_sweeps_total",
			Help: "Pending autoroute expiry sweeps by outcome.",
		}, []string{"outcome"}),
		autorouteExpired: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fi_fhir_autoroute_expired_total",
			Help: "Pending autoroute rows transitioned to expired by the sweeper.",
		}),
		autorouteNotifications: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fi_fhir_autoroute_notifications_total",
			Help: "Pending autoroute review notifications by outcome.",
		}, []string{"outcome"}),
		retentionPurges: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fi_fhir_retention_purges_total",
			Help: "Retention purge passes by outcome.",
		}, []string{"outcome"}),
		retentionRecordsPurged: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fi_fhir_retention_records_purged_total",
			Help: "Records tombstoned, deleted, or pruned by the retention purge, by outcome.",
		}, []string{"outcome"}),
	}

	registry.MustRegister(
		m.buildInfo, m.componentUp, m.readinessUp,
		m.ingressSubmissions, m.mllpMessages, m.deliveryAttempts, m.batchObjects,
		m.sessionStreamEvents, m.autorouteSweeps, m.autorouteExpired, m.autorouteNotifications,
		m.retentionPurges, m.retentionRecordsPurged,
	)
	m.buildInfo.WithLabelValues(version).Set(1)
	return m
}

// Registry exposes the registry for tests and for a caller that needs to
// gather without an HTTP round trip.
func (m *Metrics) Registry() *prometheus.Registry {
	if m == nil {
		return nil
	}
	return m.registry
}

// Handler serves the Prometheus exposition.
func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		// Scrape errors belong in the scrape result, not in the process log.
		ErrorHandling: promhttp.ContinueOnError,
	})
}

// SetComponentState publishes a background component's lifecycle state.
// A component that is not configured is omitted entirely rather than reported
// as 0, so "absent" and "broken" are distinguishable in a query.
func (m *Metrics) SetComponentState(component string, state ComponentState) {
	if m == nil {
		return
	}
	switch state {
	case ComponentRunning:
		m.componentUp.WithLabelValues(component).Set(1)
	case ComponentStopped:
		m.componentUp.WithLabelValues(component).Set(0)
	default:
		m.componentUp.DeleteLabelValues(component)
	}
}

// ObserveReadiness publishes the latest readiness report.
func (m *Metrics) ObserveReadiness(report Report) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, component := range report.Components {
		value := 1.0
		if component.Status == StatusUnhealthy {
			value = 0
		}
		m.readinessUp.WithLabelValues(component.Name).Set(value)
	}
}

// RecordIngressSubmission counts one authenticated HTTP ingress submission.
func (m *Metrics) RecordIngressSubmission(outcome Outcome) { inc(m, m.ingressSubmissions, outcome) }

// RecordMLLPMessage counts one MLLP frame.
func (m *Metrics) RecordMLLPMessage(outcome Outcome) { inc(m, m.mllpMessages, outcome) }

// RecordDeliveryAttempt counts one delivery dispatch cycle.
func (m *Metrics) RecordDeliveryAttempt(outcome Outcome) { inc(m, m.deliveryAttempts, outcome) }

// RecordBatchObject counts one batch object cycle.
func (m *Metrics) RecordBatchObject(outcome Outcome) { inc(m, m.batchObjects, outcome) }

// RecordSessionStreamEvent counts one Integration Session stream event.
func (m *Metrics) RecordSessionStreamEvent(outcome Outcome) { inc(m, m.sessionStreamEvents, outcome) }

// RecordAutorouteSweep counts one expiry sweep and the rows it expired.
func (m *Metrics) RecordAutorouteSweep(outcome Outcome, expired int) {
	inc(m, m.autorouteSweeps, outcome)
	if m != nil && expired > 0 {
		m.autorouteExpired.Add(float64(expired))
	}
}

// RecordAutorouteNotification counts one review notification transition.
func (m *Metrics) RecordAutorouteNotification(outcome Outcome) {
	inc(m, m.autorouteNotifications, outcome)
}

// RecordRetentionPurge counts one retention purge pass and the records it
// removed.
//
// Only the pass outcome and the record total are published. The record
// identifiers the purge acts on are durable identifiers, not clinical content,
// but they are unbounded, so they stay in the durable audit row and out of every
// metric label.
func (m *Metrics) RecordRetentionPurge(outcome Outcome, purged int64) {
	inc(m, m.retentionPurges, outcome)
	if m == nil || purged <= 0 {
		return
	}
	if !KnownOutcome(string(OutcomePurged)) {
		return
	}
	m.retentionRecordsPurged.WithLabelValues(string(OutcomePurged)).Add(float64(purged))
}

func inc(m *Metrics, vec *prometheus.CounterVec, outcome Outcome) {
	if m == nil || vec == nil {
		return
	}
	if !KnownOutcome(string(outcome)) {
		// Refuse an unbounded label rather than emitting it. An unknown outcome
		// is a programming error; charging it to "error" keeps cardinality
		// bounded and still shows up in a dashboard.
		outcome = OutcomeError
	}
	vec.WithLabelValues(string(outcome)).Inc()
}

// IngressMiddleware counts authenticated HL7v2 HTTP ingress submissions by
// response class.
//
// It wraps the handler in runServe rather than reaching into
// internal/integration/ingress, so the ingress package keeps no metrics
// dependency and this lane touches no file another lane owns.
func (m *Metrics) IngressMiddleware(next http.Handler) http.Handler {
	if m == nil || next == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		m.RecordIngressSubmission(outcomeForStatus(recorder.status))
	})
}

func outcomeForStatus(status int) Outcome {
	switch {
	case status >= 500:
		return OutcomeError
	case status >= 400:
		return OutcomeRejected
	default:
		return OutcomeAccepted
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if !r.written {
		r.status = status
		r.written = true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.written = true
	return r.ResponseWriter.Write(b)
}

// MetricPrefix namespaces every metric this program authors.
const MetricPrefix = "fi_fhir_"

// GatheredLabelValues returns every label value this program authors.
//
// The PHI-label proof uses it to assert that nothing outside the declared
// allowlists ever reaches an exposition. Families outside MetricPrefix — the Go
// and process collectors — are excluded because this program does not choose
// their labels; `go_info{version="go1.26.5"}` is not a cardinality decision
// anyone here made.
func GatheredLabelValues(registry *prometheus.Registry) ([]string, error) {
	if registry == nil {
		return nil, fmt.Errorf("registry is required")
	}
	families, err := registry.Gather()
	if err != nil {
		return nil, fmt.Errorf("gather metrics: %w", err)
	}
	values := make([]string, 0, len(families))
	for _, family := range families {
		if !strings.HasPrefix(family.GetName(), MetricPrefix) {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				values = append(values, label.GetValue())
			}
		}
	}
	return values, nil
}

// StartReadinessRefresh keeps fi_fhir_readiness_up current without waiting for
// a probe, so a scrape of a replica nothing is probing still tells the truth.
//
// The underlying readiness result is cached for one second by the health
// service, so a short interval here costs a map copy rather than a dependency
// round trip.
func StartReadinessRefresh(ctx context.Context, reporter Reporter, metrics *Metrics, interval time.Duration) {
	if ctx == nil || reporter == nil || metrics == nil {
		return
	}
	if interval <= 0 {
		interval = 15 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		metrics.ObserveReadiness(reporter.Ready(ctx))
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				metrics.ObserveReadiness(reporter.Ready(ctx))
			}
		}
	}()
}
