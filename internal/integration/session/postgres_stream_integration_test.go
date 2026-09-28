//go:build integration

package session

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// The batch append is what lets a preview run publish its whole stream in one
// committed round trip (2026-09-27). Its contract on a real PostgreSQL: seqs
// are index-aligned with the input, contiguous and increasing, interleave
// correctly with single appends, and a refused batch writes nothing.
func TestPostgresSessionStream_BatchAppendAssignsContiguousSeqsInOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	db := openSessionPostgres(t, ctx)
	protector, err := NewAESGCMProtector(bytes.Repeat([]byte{0x53}, 32))
	if err != nil {
		t.Fatalf("NewAESGCMProtector: %v", err)
	}
	store := newMigratedSessionStore(t, ctx, db, protector)
	var _ StreamBatchLog = store

	first, err := store.AppendStreamEvent(ctx, StreamEvent{ID: "evt_single_1", Type: StreamEventSessionCreated, SessionID: "sess_a"})
	if err != nil {
		t.Fatalf("AppendStreamEvent: %v", err)
	}
	batch := []StreamEvent{
		{ID: "evt_b1", Type: StreamEventRunStarted, SessionID: "sess_a", RunID: "run_1", At: time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)},
		{ID: "evt_b2", Type: StreamEventStageStarted, SessionID: "sess_a", RunID: "run_1", At: time.Date(2026, 9, 27, 10, 0, 1, 0, time.UTC)},
		{ID: "evt_b3", Type: StreamEventStageCompleted, SessionID: "sess_a", RunID: "run_1"},
		{ID: "evt_b4", Type: StreamEventRunCompleted, SessionID: "sess_a", RunID: "run_1"},
	}
	seqs, err := store.AppendStreamEvents(ctx, batch)
	if err != nil {
		t.Fatalf("AppendStreamEvents: %v", err)
	}
	if len(seqs) != len(batch) {
		t.Fatalf("seqs = %v, want %d", seqs, len(batch))
	}
	for i, seq := range seqs {
		if seq != first+int64(i)+1 {
			t.Fatalf("seqs = %v, want contiguous from %d", seqs, first+1)
		}
	}
	last, err := store.AppendStreamEvent(ctx, StreamEvent{ID: "evt_single_2", Type: StreamEventSessionUpdated, SessionID: "sess_a"})
	if err != nil {
		t.Fatalf("AppendStreamEvent: %v", err)
	}
	if last != seqs[len(seqs)-1]+1 {
		t.Fatalf("single append after batch seq = %d, want %d", last, seqs[len(seqs)-1]+1)
	}

	listed, err := store.ListStreamEventsAfter(ctx, first, 10)
	if err != nil {
		t.Fatalf("ListStreamEventsAfter: %v", err)
	}
	wantIDs := []string{"evt_b1", "evt_b2", "evt_b3", "evt_b4", "evt_single_2"}
	if len(listed) != len(wantIDs) {
		t.Fatalf("listed %d events, want %d: %#v", len(listed), len(wantIDs), listed)
	}
	for i, event := range listed {
		if event.ID != wantIDs[i] {
			t.Fatalf("listed[%d].ID = %s, want %s", i, event.ID, wantIDs[i])
		}
		if i < len(batch) && event.Seq != seqs[i] {
			t.Fatalf("listed[%d].Seq = %d, want %d", i, event.Seq, seqs[i])
		}
	}
	if !listed[0].At.Equal(batch[0].At) || !listed[1].At.Equal(batch[1].At) {
		t.Fatalf("explicit At was not preserved: %v %v", listed[0].At, listed[1].At)
	}
	if listed[2].At.IsZero() || listed[3].At.IsZero() {
		t.Fatal("zero At was not stamped on append")
	}

	// A single-element batch takes the single path.
	one, err := store.AppendStreamEvents(ctx, []StreamEvent{{ID: "evt_one", Type: StreamEventSampleAdded, SessionID: "sess_a"}})
	if err != nil || len(one) != 1 || one[0] != last+1 {
		t.Fatalf("single-element batch = %v, %v", one, err)
	}
	none, err := store.AppendStreamEvents(ctx, nil)
	if err != nil || none != nil {
		t.Fatalf("empty batch = %v, %v", none, err)
	}

	// A refused batch writes nothing.
	tail, err := store.LatestStreamSeq(ctx)
	if err != nil {
		t.Fatalf("LatestStreamSeq: %v", err)
	}
	for name, bad := range map[string][]StreamEvent{
		"missing session": {{ID: "x1", Type: StreamEventRunStarted}, {ID: "x2", Type: StreamEventRunStarted, SessionID: "sess_a"}},
		"missing id":      {{Type: StreamEventRunStarted, SessionID: "sess_a"}, {ID: "x3", Type: StreamEventRunStarted, SessionID: "sess_a"}},
		"duplicate id":    {{ID: "dup", Type: StreamEventRunStarted, SessionID: "sess_a"}, {ID: "dup", Type: StreamEventRunStarted, SessionID: "sess_a"}},
	} {
		if _, err := store.AppendStreamEvents(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: error = %v, want ErrInvalid", name, err)
		}
	}
	after, err := store.LatestStreamSeq(ctx)
	if err != nil || after != tail {
		t.Fatalf("tail after refused batches = %d (%v), want %d", after, err, tail)
	}
}

// The runner over the real store: one run, one terminal write, one batch.
func TestPostgresSessionStream_PreviewRunPublishesOneBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	db := openSessionPostgres(t, ctx)
	protector, err := NewAESGCMProtector(bytes.Repeat([]byte{0x54}, 32))
	if err != nil {
		t.Fatalf("NewAESGCMProtector: %v", err)
	}
	store := newMigratedSessionStore(t, ctx, db, protector)

	workspace, err := store.CreateSession(ctx, CreateSessionRequest{Name: "batched stream"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sample, err := store.AddSample(ctx, workspace.ID, AddSampleRequest{
		Name: "missing PV1", Format: events.FormatHL7v2, Source: "adt-feed", Raw: rawSessionPHI,
	})
	if err != nil {
		t.Fatalf("AddSample: %v", err)
	}
	tail, err := store.LatestStreamSeq(ctx)
	if err != nil {
		t.Fatalf("LatestStreamSeq: %v", err)
	}

	run, err := NewRunner(store, NewDurableHub(store, nil)).RunHL7v2(ctx, RunRequest{SessionID: workspace.ID, SampleID: sample.ID})
	if err != nil || run.Status != RunStatusSucceeded {
		t.Fatalf("RunHL7v2 = %#v, %v", run, err)
	}
	logged, err := store.ListStreamEventsAfter(ctx, tail, 64)
	if err != nil {
		t.Fatalf("ListStreamEventsAfter: %v", err)
	}
	if len(logged) == 0 || logged[0].Type != StreamEventRunStarted || logged[len(logged)-1].Type != StreamEventRunCompleted {
		t.Fatalf("logged run envelopes = %v", streamTypes(logged))
	}
	for i, event := range logged {
		if event.RunID != run.ID || event.SessionID != workspace.ID {
			t.Fatalf("envelope %d = %#v", i, event)
		}
		if event.Seq != tail+int64(i)+1 {
			t.Fatalf("envelope %d seq = %d, want %d", i, event.Seq, tail+int64(i)+1)
		}
	}
	stored, err := store.GetRun(ctx, workspace.ID, run.ID)
	if err != nil || stored.Status != RunStatusSucceeded || len(stored.Stages) != 4 {
		t.Fatalf("stored run = %#v, %v", stored, err)
	}
	if _, err := store.UpdateRun(ctx, *stored); !errors.Is(err, ErrImmutable) {
		t.Fatalf("terminal UpdateRun error = %v, want ErrImmutable", err)
	}
}
