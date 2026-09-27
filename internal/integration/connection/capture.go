package connection

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Sample intake from connections (.loom/38, Lane C-2): the admission-time
// capture tap and its armed-capture cache, and the Service half of stream
// captures. The batch peek is in peek.go.
//
// Both mechanisms land messages in an Integration Session — never in the
// production kernel — through session.AddSample with PHIPolicyRedact and the
// capture redactor (session.RedactCapturedHL7v2), and both write one audited
// integration_connection_captures row per request.
//
// The tap's contract (.loom/38 Decision 6) is what shapes this file:
//
//   - it never changes the result, the ACK, or the receipt. It runs after the
//     inner Process has returned, reads only the request and the result, and
//     returns exactly what the inner processor returned. Every failure of its
//     own is reported to the observer (logged and counted in cmd/) and
//     swallowed, and a panic in it is recovered for the same reason;
//   - admission never queries the database: which sources are armed is an
//     in-memory cache each replica refreshes from the captures table every
//     CaptureRefreshInterval, so a frame of a source nobody is capturing costs
//     one map lookup;
//   - its work is bounded: for a frame of an armed source it locks the
//     capture row without waiting (SKIP LOCKED), writes one sample, and
//     counts it — completing the row on the last slot — under a timeout of
//     its own, with no retry (PostgresStore.FillCaptureSlot).

// Stream capture bounds.
const (
	// DefaultCaptureMessages is maxMessages when a request leaves it out.
	DefaultCaptureMessages = 5
	// DefaultCaptureTTLSeconds is ttlSeconds when a request leaves it out.
	DefaultCaptureTTLSeconds = 300
	// MaxCaptureTTLSeconds bounds how long a capture stays armed.
	MaxCaptureTTLSeconds = 900
	// CaptureRefreshInterval is how stale a replica's armed-capture cache may
	// be: a capture armed or cancelled on another replica takes effect here
	// within it.
	CaptureRefreshInterval = 2 * time.Second
	// captureTapTimeout bounds the tap's own work for one frame. It is the most
	// a capture can delay the ACK of a frame of an armed source.
	captureTapTimeout = 2 * time.Second
	// maxListedCaptures bounds ListCaptures.
	maxListedCaptures = 100
	// maxArmedCaptures bounds one refresh of the armed-capture cache.
	maxArmedCaptures = 1000
)

var (
	// ErrSessionsUnavailable means this deployment has no Integration Session
	// workspace, so there is nowhere to put a sample. /api/auth/status reports
	// the same fact as capabilities.integrationSessions.
	ErrSessionsUnavailable = errors.New("integration sessions are not configured")
	// ErrSessionNotFound means the target session does not exist in this
	// deployment.
	ErrSessionNotFound = errors.New("integration session not found")
	// ErrSessionArchived means the target session accepts no new sample.
	ErrSessionArchived = errors.New("integration session is archived")
	// ErrCaptureNotFound hides inventory like ErrNotFound does.
	ErrCaptureNotFound = errors.New("connection capture not found")
	// ErrCaptureConflict means the source already has an armed capture.
	ErrCaptureConflict = errors.New("a capture is already armed for this source")
	// ErrCaptureFinished means the capture completed, expired, failed, or was
	// cancelled before this request reached it.
	ErrCaptureFinished = errors.New("connection capture is already finished")
)

// Problem codes a peek or a capture can finish with. They are recorded on the
// audit row and returned to the caller; none carries a secret or message
// content.
const (
	// CodeSecretUnresolvable: a binding the revision names is not declared on
	// the draft, or did not resolve on this replica. Nothing was contacted.
	CodeSecretUnresolvable = "SECRET_UNRESOLVABLE"
	// CodeSourceUnavailable: the provider could not be built, listed, or read.
	CodeSourceUnavailable = "SOURCE_UNAVAILABLE"
	// CodeObjectNotFound: the requested object is not listed under the input
	// prefix or directory.
	CodeObjectNotFound = "OBJECT_NOT_FOUND"
	// CodeMessageUnreadable: the bounded batch reader refused the object's
	// stream (not HL7v2, or a message over the source's max_message_bytes).
	CodeMessageUnreadable = "MESSAGE_UNREADABLE"
	// CodeSampleWriteFailed: the session refused a sample. A stream capture
	// stops on it.
	CodeSampleWriteFailed = "SAMPLE_WRITE_FAILED"
)

// SessionSink is the part of the Integration Session store sample intake
// writes to. session.Store satisfies it.
type SessionSink interface {
	GetSession(ctx context.Context, sessionID string) (*session.Session, error)
	AddSample(ctx context.Context, sessionID string, request session.AddSampleRequest) (*session.Sample, error)
}

// TapFailure is the bounded reason a capture tap or its cache reports a
// failure with. It is a metric label value, so the set is closed.
type TapFailure string

const (
	// TapFailureLedger: the capture row could not be locked, counted, or
	// finished.
	TapFailureLedger TapFailure = "ledger"
	// TapFailureSessionStore: the session refused the captured sample.
	TapFailureSessionStore TapFailure = "session_store"
	// TapFailureRefresh: refreshing the armed-capture cache failed.
	TapFailureRefresh TapFailure = "refresh"
	// TapFailurePanic: the tap panicked and the panic was contained.
	TapFailurePanic TapFailure = "panic"
)

// TapFailures lists every TapFailure, for the metric label allowlist.
func TapFailures() []TapFailure {
	return []TapFailure{TapFailureLedger, TapFailureSessionStore, TapFailureRefresh, TapFailurePanic}
}

// CaptureObserver receives sample intake's two measurements. Either func may
// be nil; neither may block. The error passed to TapFailed never carries
// message content: every error here comes from a store or the tap itself.
type CaptureObserver struct {
	// Captured reports messages that reached a session by peek or stream.
	Captured func(mode CaptureMode, messages int)
	// TapFailed reports a failure the tap or the cache swallowed.
	TapFailed func(reason TapFailure, err error)
}

func (o CaptureObserver) captured(mode CaptureMode, messages int) {
	if o.Captured != nil && messages > 0 {
		o.Captured(mode, messages)
	}
}

func (o CaptureObserver) tapFailed(reason TapFailure, err error) {
	if o.TapFailed != nil {
		o.TapFailed(reason, err)
	}
}

// IntakeConfig wires sample intake into a catalog Service.
type IntakeConfig struct {
	// Sessions is where peeked and captured samples land. Required: without
	// it every intake method refuses with ErrSessionsUnavailable.
	Sessions SessionSink
	// Secrets resolves a batch connection's bindings for a peek. cmd/ injects
	// the env/file resolver; nil leaves every binding unresolvable.
	Secrets integration.SecretResolver
	// Registry, when set, is told about captures armed and cancelled here, so
	// they take effect on this replica at once rather than at its next refresh.
	Registry *CaptureRegistry
	// Observer receives the captured-message counts of peeks.
	Observer CaptureObserver
	// Providers builds a peek's batch provider. Nil selects the batch
	// package's own S3 and SFTP constructors.
	Providers BatchProviderFactory
}

// sampleIntake is an enabled IntakeConfig.
type sampleIntake struct {
	sessions  SessionSink
	secrets   integration.SecretResolver
	registry  *CaptureRegistry
	observer  CaptureObserver
	providers BatchProviderFactory
}

// EnableSampleIntake turns on peekBatchConnection, startConnectionCapture,
// cancelConnectionCapture, and connectionCaptures for this Service.
func (s *Service) EnableSampleIntake(config IntakeConfig) error {
	if s == nil || s.store == nil {
		return ErrUnavailable
	}
	if config.Sessions == nil {
		return ErrSessionsUnavailable
	}
	providers := config.Providers
	if providers == nil {
		providers = openBatchProvider
	}
	s.intake.Store(&sampleIntake{
		sessions: config.Sessions, secrets: config.Secrets, registry: config.Registry,
		observer: config.Observer, providers: providers,
	})
	return nil
}

// authorizeIntake is every intake method's gate: the operator read role, then
// a configured session workspace.
func (s *Service) authorizeIntake(ctx context.Context) (integration.SecurityContext, *sampleIntake, error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return integration.SecurityContext{}, nil, err
	}
	intake := s.intake.Load()
	if intake == nil {
		return integration.SecurityContext{}, nil, ErrSessionsUnavailable
	}
	return security, intake, nil
}

// activeSession refuses a session that does not exist or is archived.
func (i *sampleIntake) activeSession(ctx context.Context, sessionID string) error {
	found, err := i.sessions.GetSession(ctx, sessionID)
	switch {
	case errors.Is(err, session.ErrNotFound):
		return ErrSessionNotFound
	case err != nil:
		return fmt.Errorf("load integration session: %w", err)
	case found == nil:
		return ErrSessionNotFound
	case found.Status != session.SessionStatusActive:
		return ErrSessionArchived
	}
	return nil
}

// StartCaptureRequest arms a stream capture of one runtime source.
type StartCaptureRequest struct {
	SourceID    string
	SessionID   string
	MaxMessages int // 0 selects DefaultCaptureMessages; otherwise 1..MaxCaptureMessages
	TTLSeconds  int // 0 selects DefaultCaptureTTLSeconds; otherwise 1..MaxCaptureTTLSeconds
	Reason      string
}

// StartCapture arms a capture of the next MaxMessages frames the source
// admits on any replica, into an active session, for at most TTLSeconds.
func (s *Service) StartCapture(ctx context.Context, request StartCaptureRequest) (Capture, error) {
	security, intake, err := s.authorizeIntake(ctx)
	if err != nil {
		return Capture{}, err
	}
	reason, err := intakeReason(request.Reason)
	if err != nil {
		return Capture{}, err
	}
	maxMessages, ok := boundedDefault(request.MaxMessages, DefaultCaptureMessages, MaxCaptureMessages)
	if !ok || !validIdentity(request.SourceID) || !validIdentity(request.SessionID) {
		return Capture{}, ErrInvalidRequest
	}
	ttl, ok := boundedDefault(request.TTLSeconds, DefaultCaptureTTLSeconds, MaxCaptureTTLSeconds)
	if !ok {
		return Capture{}, ErrInvalidRequest
	}
	if err := intake.activeSession(ctx, request.SessionID); err != nil {
		return Capture{}, err
	}
	now := s.store.Now()
	capture, err := s.store.InsertCapture(ctx, Capture{
		TenantID: security.TenantID, ID: uuid.NewString(), SessionID: request.SessionID,
		Mode: CaptureModeStream, SourceID: request.SourceID, Status: CaptureStatusArmed,
		MaxMessages: maxMessages, Version: 1, RequestedBy: auditPrincipal(security),
		Reason: reason, RequestedAt: now, ExpiresAt: now.Add(time.Duration(ttl) * time.Second),
	})
	if err != nil {
		return Capture{}, err
	}
	intake.registry.arm(capture)
	return capture, nil
}

// CancelCapture stops an armed capture. The row keeps what it captured; who
// cancelled it and why are recorded beside it.
func (s *Service) CancelCapture(ctx context.Context, captureID, reason string) (Capture, error) {
	security, intake, err := s.authorizeIntake(ctx)
	if err != nil {
		return Capture{}, err
	}
	trimmed, err := intakeReason(reason)
	if err != nil {
		return Capture{}, err
	}
	if !validIdentity(captureID) {
		return Capture{}, ErrCaptureNotFound
	}
	now := s.store.Now()
	cancelled, applied, err := s.store.FinishCapture(ctx, security.TenantID, captureID, 0, captureFinish{
		status: CaptureStatusCancelled, at: now,
		cancellation: &captureCancellation{Principal: auditPrincipal(security), Reason: trimmed, At: now},
	})
	if err != nil {
		return Capture{}, err
	}
	intake.registry.forget(captureID)
	if !applied {
		return Capture{}, ErrCaptureFinished
	}
	return cancelled, nil
}

// ListCaptures returns one session's captures and peeks, newest first.
func (s *Service) ListCaptures(ctx context.Context, sessionID string) ([]Capture, error) {
	security, _, err := s.authorizeIntake(ctx)
	if err != nil {
		return nil, err
	}
	if !validIdentity(sessionID) {
		return nil, ErrInvalidRequest
	}
	return s.store.ListSessionCaptures(ctx, security.TenantID, sessionID, maxListedCaptures)
}

func auditPrincipal(security integration.SecurityContext) integration.Principal {
	principal := security.Principal
	principal.Roles = append([]string(nil), security.Principal.Roles...)
	return principal
}

// intakeReason trims and bounds the reason every intake request records.
func intakeReason(reason string) (string, error) {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" || len(trimmed) > MaxReasonBytes || !printable(trimmed, true) {
		return "", ErrInvalidRequest
	}
	return trimmed, nil
}

// boundedDefault applies a default to a zero value and bounds the rest to
// 1..limit.
func boundedDefault(value, defaultValue, limit int) (int, bool) {
	if value == 0 {
		return defaultValue, true
	}
	return value, value >= 1 && value <= limit
}

// armedCapture is one cache entry: what the tap needs to claim a slot and
// write a sample, and nothing it would have to keep in step with the row.
type armedCapture struct {
	id          string
	sessionID   string
	maxMessages int
	expiresAt   time.Time
}

// captureLedger is the part of PostgresStore the cache and the tap use; unit
// tests substitute it.
type captureLedger interface {
	Now() time.Time
	ArmedStreamCaptures(ctx context.Context, tenantID string, now time.Time, limit int) ([]Capture, error)
	ExpireCaptures(ctx context.Context, tenantID string, now time.Time) (int64, error)
	FillCaptureSlot(ctx context.Context, tenantID, captureID string, now time.Time,
		write func(ctx context.Context, slot int) error) (captureFill, error)
}

// CaptureRegistryConfig configures a replica's armed-capture cache.
type CaptureRegistryConfig struct {
	Store    *PostgresStore
	TenantID string
	// Interval is the refresh period; zero selects CaptureRefreshInterval.
	Interval time.Duration
	// Observer receives refresh failures.
	Observer CaptureObserver
}

// CaptureRegistry is one replica's in-memory view of the armed stream
// captures, keyed by source ID. Admission reads only this; the database is
// read once per Interval by Run.
type CaptureRegistry struct {
	ledger   captureLedger
	tenantID string
	interval time.Duration
	observer CaptureObserver

	mu    sync.RWMutex
	armed map[string]armedCapture
}

// NewCaptureRegistry builds an empty cache; Run fills it.
func NewCaptureRegistry(config CaptureRegistryConfig) (*CaptureRegistry, error) {
	if config.Store == nil || !validIdentity(config.TenantID) {
		return nil, ErrUnavailable
	}
	return newCaptureRegistry(config.Store, config.TenantID, config.Interval, config.Observer), nil
}

func newCaptureRegistry(ledger captureLedger, tenantID string, interval time.Duration, observer CaptureObserver) *CaptureRegistry {
	if interval <= 0 {
		interval = CaptureRefreshInterval
	}
	return &CaptureRegistry{
		ledger: ledger, tenantID: tenantID, interval: interval, observer: observer,
		armed: make(map[string]armedCapture),
	}
}

// Run refreshes the cache now and then every interval until ctx is done. A
// failed refresh keeps the previous view and is reported, never returned: a
// capture that goes stale this way still stops at its TTL, which the tap
// checks from memory.
func (r *CaptureRegistry) Run(ctx context.Context) error {
	if r == nil || ctx == nil {
		return nil
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		if err := r.Refresh(ctx); err != nil && ctx.Err() == nil {
			r.observer.tapFailed(TapFailureRefresh, err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Refresh advances every armed capture or peek past its TTL to expired, then
// replaces the cache with the armed stream captures that still have a slot.
func (r *CaptureRegistry) Refresh(ctx context.Context) error {
	if r == nil {
		return nil
	}
	now := r.ledger.Now()
	if _, err := r.ledger.ExpireCaptures(ctx, r.tenantID, now); err != nil {
		return err
	}
	captures, err := r.ledger.ArmedStreamCaptures(ctx, r.tenantID, now, maxArmedCaptures)
	if err != nil {
		return err
	}
	armed := make(map[string]armedCapture, len(captures))
	for _, capture := range captures {
		// Oldest first, and the schema allows one armed stream capture per
		// source; keeping the first is what a second one would get anyway.
		if _, taken := armed[capture.SourceID]; taken {
			continue
		}
		armed[capture.SourceID] = armedCaptureOf(capture)
	}
	r.mu.Lock()
	r.armed = armed
	r.mu.Unlock()
	return nil
}

func armedCaptureOf(capture Capture) armedCapture {
	return armedCapture{
		id: capture.ID, sessionID: capture.SessionID,
		maxMessages: capture.MaxMessages, expiresAt: capture.ExpiresAt,
	}
}

// armedFor is the tap's one read on the admission path.
func (r *CaptureRegistry) armedFor(tenantID, sourceID string, now time.Time) (armedCapture, bool) {
	if r == nil || tenantID != r.tenantID {
		return armedCapture{}, false
	}
	r.mu.RLock()
	capture, ok := r.armed[sourceID]
	r.mu.RUnlock()
	if !ok || !now.Before(capture.expiresAt) {
		return armedCapture{}, false
	}
	return capture, true
}

// arm makes a capture armed on this replica effective here at once.
func (r *CaptureRegistry) arm(capture Capture) {
	if r == nil || capture.Mode != CaptureModeStream || capture.Status != CaptureStatusArmed {
		return
	}
	r.mu.Lock()
	r.armed[capture.SourceID] = armedCaptureOf(capture)
	r.mu.Unlock()
}

// forget drops a capture this replica saw finish.
func (r *CaptureRegistry) forget(captureID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for sourceID, capture := range r.armed {
		if capture.id == captureID {
			delete(r.armed, sourceID)
		}
	}
}

// Processor is the method set both the MLLP listener
// (mllp.MessageProcessor) and the HTTP ingress service (ingress.Processor)
// call, so a tap can stand in for either without changing their contracts.
type Processor interface {
	Process(ctx context.Context, request integration.ProcessRequest) (integration.ProcessResult, error)
}

// CaptureTapBinding is what a tap needs once the catalog and the session
// workspace exist.
type CaptureTapBinding struct {
	Registry *CaptureRegistry
	Sessions SessionSink
	Observer CaptureObserver
}

// CaptureTaps makes the taps one process wraps its admission processors in,
// and binds them all at once. serve wraps the MLLP listener's and the HTTP
// ingress service's processors where it constructs them, before the session
// store and the catalog exist; until Bind, every tap is a pass-through.
type CaptureTaps struct {
	binding atomic.Pointer[CaptureTapBinding]
}

// NewCaptureTaps returns an unbound tap set.
func NewCaptureTaps() *CaptureTaps {
	return &CaptureTaps{}
}

// Bind activates every tap this set made.
func (t *CaptureTaps) Bind(binding CaptureTapBinding) error {
	if t == nil || binding.Registry == nil || binding.Sessions == nil {
		return ErrUnavailable
	}
	t.binding.Store(&binding)
	return nil
}

// Wrap returns inner decorated with a capture tap.
func (t *CaptureTaps) Wrap(inner Processor) Processor {
	if t == nil || inner == nil {
		return inner
	}
	return &captureTap{inner: inner, taps: t}
}

// captureTap is one decorated processor.
type captureTap struct {
	inner Processor
	taps  *CaptureTaps
}

// Process returns exactly what the inner processor returned. The capture, if
// any, happens in between and cannot reach either value.
func (c *captureTap) Process(ctx context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
	result, err := c.inner.Process(ctx, request)
	if err == nil {
		c.taps.observe(ctx, request, result)
	}
	return result, err
}

// observe captures one durably accepted production frame when its source is
// armed on this replica.
func (t *CaptureTaps) observe(ctx context.Context, request integration.ProcessRequest, result integration.ProcessResult) {
	binding := t.binding.Load()
	if binding == nil || request.Mode != integration.ExecutionModeProduction ||
		result.Receipt == nil || result.Receipt.Status != integration.ReceiptStatusAccepted {
		return
	}
	defer func() {
		// The tap's guarantee is that admission cannot tell it ran. A panic
		// here would unwind into the listener and cost the frame its ACK.
		if recovered := recover(); recovered != nil {
			binding.Observer.tapFailed(TapFailurePanic, fmt.Errorf("capture tap panicked: %v", recovered))
		}
	}()
	binding.capture(ctx, request)
}

func (b *CaptureTapBinding) capture(ctx context.Context, request integration.ProcessRequest) {
	registry := b.Registry
	now := registry.ledger.Now()
	armed, ok := registry.armedFor(request.Envelope.TenantID, request.Envelope.SourceID, now)
	if !ok {
		return
	}
	// The tap's own budget, detached from the request's: a frame admitted
	// just before its processing deadline must not turn every capture of it
	// into a failure, and a slow session store must not hold the ACK longer
	// than captureTapTimeout.
	tapCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), captureTapTimeout)
	defer cancel()
	fill, err := registry.ledger.FillCaptureSlot(tapCtx, registry.tenantID, armed.id, now,
		func(writeCtx context.Context, slot int) error {
			_, err := b.Sessions.AddSample(writeCtx, armed.sessionID, session.AddSampleRequest{
				Name:      fmt.Sprintf("capture %s #%d", armed.id, slot),
				Format:    request.Envelope.Format,
				Source:    "capture:" + armed.id,
				Raw:       string(request.Envelope.Bytes()),
				PHIPolicy: session.PHIPolicyRedact,
				Redaction: session.SampleRedactionCapture,
			})
			return err
		})
	if fill.outcome == fillWriteFailed {
		b.Observer.tapFailed(TapFailureSessionStore, fill.writeErr)
		registry.forget(armed.id)
	}
	if err != nil {
		b.Observer.tapFailed(TapFailureLedger, err)
		return
	}
	switch fill.outcome {
	case fillWritten:
		b.Observer.captured(CaptureModeStream, 1)
	case fillCompleted:
		b.Observer.captured(CaptureModeStream, 1)
		registry.forget(armed.id)
	case fillSkipped, fillWriteFailed:
		// Skipped: finished elsewhere since the last refresh, or another
		// frame is being written right now; either way this frame is not
		// captured. The next refresh settles which.
	}
}
