package session

import (
	"context"
	"fmt"
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
	h.stamp(&event)

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

// PublishAll records events for delivery in slice order.
//
// The ordering contract is the same on every path Publish has: in process the
// events are delivered in slice order; through a StreamBatchLog they take
// contiguous seqs in slice order from one append; through a plain StreamLog
// they are appended one at a time, in order. The runner uses this to publish a
// whole run's envelopes after its terminal write, so the run's wall-clock no
// longer includes one committed INSERT per envelope.
//
// The slice is stamped in place (ID, At, and Seq when the log assigned one),
// so a caller that keeps it can see what was published.
func (h *Hub) PublishAll(events []StreamEvent) {
	if h == nil || len(events) == 0 {
		return
	}
	for i := range events {
		h.stamp(&events[i])
	}

	if h.log == nil {
		for _, event := range events {
			h.deliver(event)
		}
		return
	}
	batch, ok := h.log.(StreamBatchLog)
	if !ok {
		for _, event := range events {
			h.Publish(event)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.appendTimeout)
	defer cancel()
	seqs, err := batch.AppendStreamEvents(ctx, events)
	if err == nil && len(seqs) != len(events) {
		err = fmt.Errorf("session stream batch append returned %d seqs for %d events", len(seqs), len(events))
	}
	if err != nil {
		h.report(StreamOutcomeError, err)
		// Same degradation as Publish: the local subscribers still see the run.
		for _, event := range events {
			h.deliver(event)
		}
		return
	}
	for i := range events {
		events[i].Seq = seqs[i]
		h.report(StreamOutcomePublished, nil)
	}
}

// stamp fills the envelope fields the hub owns.
func (h *Hub) stamp(event *StreamEvent) {
	if event.ID == "" {
		event.ID = newID("evt")
	}
	if event.At.IsZero() {
		event.At = h.now()
	}
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
