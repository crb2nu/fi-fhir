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
}

type Hub struct {
	mu          sync.RWMutex
	now         func() time.Time
	subscribers map[string]subscription
	buffer      int
}

type subscription struct {
	sessionID string
	ch        chan StreamEvent
}

func NewHub() *Hub {
	return &Hub{
		now:         func() time.Time { return time.Now().UTC() },
		subscribers: make(map[string]subscription),
		buffer:      32,
	}
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

func (h *Hub) Publish(event StreamEvent) {
	if event.ID == "" {
		event.ID = newID("evt")
	}
	if event.At.IsZero() {
		event.At = h.now()
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, sub := range h.subscribers {
		if sub.sessionID != "" && event.SessionID != sub.sessionID {
			continue
		}
		select {
		case sub.ch <- event:
		default:
		}
	}
}
