package session

import (
	"context"
	"sync"
	"time"
)

type StreamEventType string

const (
	StreamEventSessionCreated  StreamEventType = "session_created"
	StreamEventSessionUpdated  StreamEventType = "session_updated"
	StreamEventSessionArchived StreamEventType = "session_archived"
	StreamEventSampleAdded     StreamEventType = "sample_added"
	StreamEventDraftSaved      StreamEventType = "draft_saved"
	StreamEventRunStarted      StreamEventType = "run_started"
	StreamEventStageStarted    StreamEventType = "stage_started"
	StreamEventStageCompleted  StreamEventType = "stage_completed"
	StreamEventDiagnostic      StreamEventType = "diagnostic"
	StreamEventRunCompleted    StreamEventType = "run_completed"
	StreamEventRunFailed       StreamEventType = "run_failed"
)

type StreamEvent struct {
	ID        string          `json:"id"`
	Type      StreamEventType `json:"type"`
	SessionID string          `json:"session_id"`
	RunID     string          `json:"run_id,omitempty"`
	Payload   any             `json:"payload,omitempty"`
	At        time.Time       `json:"at"`

	// Seq is the durable fanout cursor position. It is set by the durable log
	// on append and on replay, and is zero for purely in-process delivery.
	//
	// Payload is never persisted; see migrations/0005_session_stream_events.sql.
	Seq int64 `json:"seq,omitempty"`
}

// Hub fans session stream events out to this replica's SSE subscribers.
//
// With a durable log configured, Publish appends the envelope and returns; the
// StreamRelay on every replica — including this one — performs the actual local
// delivery. That single path is what makes a subscription on replica A see a
// run executed on replica B, and it keeps ordering identical on every replica
// because the log's seq is the only ordering authority.
//
// Without a durable log, Publish delivers in process, which is the pre-Slice-4.3
// behaviour and remains correct for the in-memory store and single-process
// tests.
type Hub struct {
	mu          sync.RWMutex
	now         func() time.Time
	subscribers map[string]subscription
	buffer      int

	log           StreamLog
	observe       func(StreamOutcome, error)
	appendTimeout time.Duration
}

type subscription struct {
	sessionID string
	ch        chan StreamEvent
}

// NewHub builds an in-process-only hub.
func NewHub() *Hub {
	return &Hub{
		now:           func() time.Time { return time.Now().UTC() },
		subscribers:   make(map[string]subscription),
		buffer:        32,
		appendTimeout: 5 * time.Second,
	}
}

// NewDurableHub builds a hub whose publishes go through the durable fanout log.
// A nil log degrades to NewHub, so a deployment without the durable session
// workspace keeps working rather than losing its stream.
func NewDurableHub(log StreamLog, observe func(StreamOutcome, error)) *Hub {
	hub := NewHub()
	hub.log = log
	hub.observe = observe
	return hub
}

// Durable reports whether publishes are logged for cross-replica fanout.
func (h *Hub) Durable() bool {
	return h != nil && h.log != nil
}

func (h *Hub) Subscribe(ctx context.Context, sessionID string) <-chan StreamEvent {
	ch := make(chan StreamEvent, h.buffer)
	id := newID("sub")

	h.mu.Lock()
	h.subscribers[id] = subscription{sessionID: sessionID, ch: ch}
	h.mu.Unlock()

	go func() {
		<-ctx.Done()
		h.mu.Lock()
		if sub, ok := h.subscribers[id]; ok {
			delete(h.subscribers, id)
			close(sub.ch)
		}
		h.mu.Unlock()
	}()

	return ch
}

// Publish records an event for delivery.
func (h *Hub) Publish(event StreamEvent) {
	if h == nil {
		return
	}
	if event.ID == "" {
		event.ID = newID("evt")
	}
	if event.At.IsZero() {
		event.At = h.now()
	}

	if h.log == nil {
		h.deliver(event)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.appendTimeout)
	defer cancel()
	seq, err := h.log.AppendStreamEvent(ctx, event)
	if err != nil {
		h.report(StreamOutcomeError, err)
		// Fall back to in-process delivery. Losing cross-replica fanout during a
		// database outage is a degradation; losing the stream entirely would be
		// a regression against the behaviour this slice replaced.
		h.deliver(event)
		return
	}
	event.Seq = seq
	h.report(StreamOutcomePublished, nil)
}

// deliver fans one event out to this process's subscribers.
func (h *Hub) deliver(event StreamEvent) {
	if h == nil {
		return
	}
	// The hook is captured before the read lock is taken: calling report inside
	// the fanout loop would re-acquire the same RWMutex for reading, which
	// deadlocks whenever a writer is already queued between the two RLocks.
	dropped := 0
	h.mu.RLock()
	observe := h.observe
	for _, sub := range h.subscribers {
		if sub.sessionID != "" && event.SessionID != sub.sessionID {
			continue
		}
		select {
		case sub.ch <- event:
		default:
			// A subscriber that cannot keep up loses this event rather than
			// stalling the run. The durable log still holds the envelope, so a
			// reconnecting client is not left with a silently truncated history.
			dropped++
		}
	}
	h.mu.RUnlock()

	if observe == nil {
		return
	}
	for i := 0; i < dropped; i++ {
		observe(StreamOutcomeDropped, nil)
	}
}

func (h *Hub) report(outcome StreamOutcome, err error) {
	if h == nil {
		return
	}
	observe := h.observer()
	if observe == nil {
		return
	}
	observe(outcome, err)
}

// observer reads the hook without assuming the caller already holds a lock.
// deliver holds the read lock while fanning out, so the hook is captured before
// delivery starts rather than per subscriber.
func (h *Hub) observer() func(StreamOutcome, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.observe
}

// SetObserver binds an observation hook after construction.
//
// The hub is built inside the resolver's session service, but the serve process
// owns the metrics registry, so the binding happens once in runServe before the
// listener accepts traffic.
func (h *Hub) SetObserver(observe func(StreamOutcome, error)) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.observe = observe
}
