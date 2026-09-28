package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func batchOfThree(sessionID string) []StreamEvent {
	return []StreamEvent{
		{Type: StreamEventRunStarted, SessionID: sessionID, RunID: "run_1"},
		{Type: StreamEventStageStarted, SessionID: sessionID, RunID: "run_1"},
		{Type: StreamEventRunCompleted, SessionID: sessionID, RunID: "run_1"},
	}
}

func drainStream(t *testing.T, stream <-chan StreamEvent, n int) []StreamEvent {
	t.Helper()
	got := make([]StreamEvent, 0, n)
	deadline := time.After(time.Second)
	for len(got) < n {
		select {
		case event := <-stream:
			got = append(got, event)
		case <-deadline:
			t.Fatalf("timed out with %d of %d events: %v", len(got), n, streamTypes(got))
		}
	}
	return got
}

func TestHubPublishAllStampsAndDeliversInProcessInOrder(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := hub.Subscribe(ctx, "sess_a")
	other := hub.Subscribe(ctx, "sess_b")

	batch := batchOfThree("sess_a")
	hub.PublishAll(batch)

	got := drainStream(t, stream, 3)
	assertStreamTypes(t, got, streamTypes(batchOfThree("sess_a")))
	for i, event := range batch {
		if event.ID == "" || event.At.IsZero() {
			t.Fatalf("batch[%d] was not stamped in place: %#v", i, event)
		}
		if got[i].ID != event.ID {
			t.Fatalf("delivered[%d].ID = %s, want %s", i, got[i].ID, event.ID)
		}
		if event.Seq != 0 {
			t.Fatalf("in-process delivery must leave Seq zero, got %d", event.Seq)
		}
	}
	select {
	case event := <-other:
		t.Fatalf("session filter leaked %s to another session", event.Type)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHubPublishAllUsesOneBatchAppendAndAssignsSeqs(t *testing.T) {
	log := &memoryStreamLog{}
	var mu sync.Mutex
	outcomes := map[StreamOutcome]int{}
	hub := NewDurableHub(log, func(outcome StreamOutcome, _ error) {
		mu.Lock()
		defer mu.Unlock()
		outcomes[outcome]++
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := hub.Subscribe(ctx, "sess_a")

	if _, err := log.AppendStreamEvent(ctx, StreamEvent{Type: StreamEventSessionCreated, SessionID: "sess_z"}); err != nil {
		t.Fatalf("seed append: %v", err)
	}
	batch := batchOfThree("sess_a")
	hub.PublishAll(batch)

	if log.batches != 1 || log.singles != 1 {
		t.Fatalf("appends: batches = %d, singles = %d; want one batch after the seed", log.batches, log.singles)
	}
	for i, event := range batch {
		if event.Seq != int64(i+2) {
			t.Fatalf("batch[%d].Seq = %d, want %d", i, event.Seq, i+2)
		}
	}
	if outcomes[StreamOutcomePublished] != 3 || outcomes[StreamOutcomeError] != 0 {
		t.Fatalf("outcomes = %v", outcomes)
	}
	// With a durable log the relay delivers, not Publish: nothing in process.
	select {
	case event := <-stream:
		t.Fatalf("durable publish delivered %s in process", event.Type)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHubPublishAllDegradesToInProcessWhenBatchAppendFails(t *testing.T) {
	log := &memoryStreamLog{batchErr: errors.New("log unavailable")}
	var mu sync.Mutex
	var reported []error
	hub := NewDurableHub(log, func(outcome StreamOutcome, err error) {
		mu.Lock()
		defer mu.Unlock()
		if outcome == StreamOutcomeError {
			reported = append(reported, err)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := hub.Subscribe(ctx, "sess_a")

	hub.PublishAll(batchOfThree("sess_a"))

	got := drainStream(t, stream, 3)
	assertStreamTypes(t, got, streamTypes(batchOfThree("sess_a")))
	if len(reported) != 1 || !errors.Is(reported[0], log.batchErr) {
		t.Fatalf("reported errors = %v, want the batch error once", reported)
	}
	if len(log.snapshot()) != 0 {
		t.Fatalf("a failed batch must not leave partial envelopes: %v", streamTypes(log.snapshot()))
	}
}

func TestHubPublishAllFallsBackToPublishPerEventWithoutBatchSupport(t *testing.T) {
	inner := &memoryStreamLog{}
	hub := NewDurableHub(singleStreamLog{inner: inner}, nil)

	hub.PublishAll(batchOfThree("sess_a"))

	if inner.batches != 0 || inner.singles != 3 {
		t.Fatalf("appends: batches = %d, singles = %d; want three singles", inner.batches, inner.singles)
	}
	assertStreamTypes(t, inner.snapshot(), streamTypes(batchOfThree("sess_a")))
}

func TestHubPublishAllIgnoresEmptyAndNil(t *testing.T) {
	log := &memoryStreamLog{}
	hub := NewDurableHub(log, nil)
	hub.PublishAll(nil)
	hub.PublishAll([]StreamEvent{})
	var nilHub *Hub
	nilHub.PublishAll(batchOfThree("sess_a"))
	if log.batches != 0 || log.singles != 0 {
		t.Fatalf("appends = %d/%d, want none", log.batches, log.singles)
	}
}
