package session

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// countingStore counts the store round trips a run makes at the Store
// boundary. It is the regression guard for the 2026-09-27 change: a preview
// run used to perform ~25 sequential committed round trips, and the runner's
// wall-clock tracked their commit latency instead of the sub-millisecond parse.
type countingStore struct {
	Store
	mu        sync.Mutex
	calls     map[string]int
	updateErr error
}

func newCountingStore(inner Store) *countingStore {
	return &countingStore{Store: inner, calls: map[string]int{}}
}

func (c *countingStore) count(method string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls[method]++
}

func (c *countingStore) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for _, n := range c.calls {
		total += n
	}
	return total
}

func (c *countingStore) GetSample(ctx context.Context, sessionID, sampleID string) (*Sample, error) {
	c.count("GetSample")
	return c.Store.GetSample(ctx, sessionID, sampleID)
}

func (c *countingStore) GetArtifactRevision(ctx context.Context, sessionID, revisionID string) (*ArtifactDraft, error) {
	c.count("GetArtifactRevision")
	return c.Store.GetArtifactRevision(ctx, sessionID, revisionID)
}

func (c *countingStore) CreateRun(ctx context.Context, sessionID, sampleID, source string) (*Run, error) {
	c.count("CreateRun")
	return c.Store.CreateRun(ctx, sessionID, sampleID, source)
}

func (c *countingStore) UpdateRun(ctx context.Context, run Run) (*Run, error) {
	c.count("UpdateRun")
	if c.updateErr != nil {
		return nil, c.updateErr
	}
	return c.Store.UpdateRun(ctx, run)
}

func (c *countingStore) GetRun(ctx context.Context, sessionID, runID string) (*Run, error) {
	c.count("GetRun")
	return c.Store.GetRun(ctx, sessionID, runID)
}

// memoryStreamLog is a StreamLog for tests that counts single and batch
// appends. It implements StreamBatchLog unless batchErr is set to the sentinel
// that makes AppendStreamEvents fail, and singlesOnly hides the batch method
// behind a wrapper (see singleStreamLog).
type memoryStreamLog struct {
	mu       sync.Mutex
	events   []StreamEvent
	singles  int
	batches  int
	batchErr error
}

func (l *memoryStreamLog) AppendStreamEvent(_ context.Context, event StreamEvent) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.singles++
	event.Seq = int64(len(l.events) + 1)
	l.events = append(l.events, event)
	return event.Seq, nil
}

func (l *memoryStreamLog) AppendStreamEvents(_ context.Context, events []StreamEvent) ([]int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.batches++
	if l.batchErr != nil {
		return nil, l.batchErr
	}
	seqs := make([]int64, len(events))
	for i, event := range events {
		event.Seq = int64(len(l.events) + 1)
		l.events = append(l.events, event)
		seqs[i] = event.Seq
	}
	return seqs, nil
}

func (l *memoryStreamLog) ListStreamEventsAfter(_ context.Context, afterSeq int64, limit int) ([]StreamEvent, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]StreamEvent, 0, limit)
	for _, event := range l.events {
		if event.Seq > afterSeq {
			out = append(out, event)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (l *memoryStreamLog) LatestStreamSeq(context.Context) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return int64(len(l.events)), nil
}

func (l *memoryStreamLog) snapshot() []StreamEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]StreamEvent(nil), l.events...)
}

// singleStreamLog exposes only the StreamLog surface of a memoryStreamLog, the
// shape of a log that predates batch appends.
type singleStreamLog struct{ inner *memoryStreamLog }

func (s singleStreamLog) AppendStreamEvent(ctx context.Context, event StreamEvent) (int64, error) {
	return s.inner.AppendStreamEvent(ctx, event)
}

func (s singleStreamLog) ListStreamEventsAfter(ctx context.Context, afterSeq int64, limit int) ([]StreamEvent, error) {
	return s.inner.ListStreamEventsAfter(ctx, afterSeq, limit)
}

func (s singleStreamLog) LatestStreamSeq(ctx context.Context) (int64, error) {
	return s.inner.LatestStreamSeq(ctx)
}

func seedRunnerSample(t *testing.T, store Store, raw string) (*Session, *Sample) {
	t.Helper()
	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionRequest{Name: "round trips"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sample, err := store.AddSample(ctx, sess.ID, AddSampleRequest{
		Name:      "sample",
		Format:    events.FormatHL7v2,
		Source:    "adt-feed",
		Raw:       raw,
		PHIPolicy: PHIPolicyRetain,
	})
	if err != nil {
		t.Fatalf("AddSample: %v", err)
	}
	return sess, sample
}

func missingPV1Sample(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../../testdata/integration-session/adt_a01_missing_pv1.hl7")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(raw)
}

func streamTypes(events []StreamEvent) []StreamEventType {
	out := make([]StreamEventType, 0, len(events))
	for _, event := range events {
		out = append(out, event.Type)
	}
	return out
}

func assertStreamTypes(t *testing.T, got []StreamEvent, want []StreamEventType) {
	t.Helper()
	gotTypes := streamTypes(got)
	if len(gotTypes) != len(want) {
		t.Fatalf("stream types = %v, want %v", gotTypes, want)
	}
	for i := range want {
		if gotTypes[i] != want[i] {
			t.Fatalf("stream type %d = %s, want %s; all = %v", i, gotTypes[i], want[i], gotTypes)
		}
	}
}

var succeededRunStreamTypes = []StreamEventType{
	StreamEventRunStarted,
	StreamEventStageStarted, StreamEventStageCompleted, // load_sample
	StreamEventStageStarted, StreamEventStageCompleted, // parse_hl7v2
	StreamEventStageStarted, StreamEventDiagnostic, StreamEventStageCompleted, // normalize_diagnostics
	StreamEventStageStarted, StreamEventStageCompleted, // build_lineage
	StreamEventRunCompleted,
}

func TestRunnerWritesTerminalRecordOnceAndPublishesOneBatch(t *testing.T) {
	ctx := context.Background()
	store := newCountingStore(NewMemoryStore())
	log := &memoryStreamLog{}
	runner := NewRunner(store, NewDurableHub(log, nil))
	sess, sample := seedRunnerSample(t, store, missingPV1Sample(t))
	before := store.total()

	run, err := runner.RunHL7v2(ctx, RunRequest{SessionID: sess.ID, SampleID: sample.ID})
	if err != nil {
		t.Fatalf("RunHL7v2: %v", err)
	}
	if run.Status != RunStatusSucceeded || len(run.Stages) != 4 || len(run.Events) != 1 {
		t.Fatalf("run = %#v", run)
	}

	// The store boundary: read the sample, claim the run, write it once.
	want := map[string]int{"GetSample": 1, "CreateRun": 1, "UpdateRun": 1}
	for method, n := range want {
		if store.calls[method] != n {
			t.Fatalf("%s calls = %d, want %d; all = %v", method, store.calls[method], n, store.calls)
		}
	}
	if got := store.total() - before; got != 3 {
		t.Fatalf("store round trips per run = %d, want 3; calls = %v", got, store.calls)
	}

	// The stream boundary: one durable append for the whole run, in order.
	if log.batches != 1 || log.singles != 0 {
		t.Fatalf("stream appends: batches = %d, singles = %d; want 1 batch and no singles", log.batches, log.singles)
	}
	logged := log.snapshot()
	assertStreamTypes(t, logged, succeededRunStreamTypes)
	for i, event := range logged {
		if event.SessionID != sess.ID || event.RunID != run.ID || event.ID == "" {
			t.Fatalf("envelope %d = %#v", i, event)
		}
		if event.Seq != int64(i+1) {
			t.Fatalf("envelope %d seq = %d, want %d", i, event.Seq, i+1)
		}
		if i > 0 && event.At.Before(logged[i-1].At) {
			t.Fatalf("envelope %d at %s is before envelope %d at %s", i, event.At, i-1, logged[i-1].At)
		}
	}

	// The durable record is the run the caller got back, and it is terminal.
	stored, err := store.Store.GetRun(ctx, sess.ID, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if stored.Status != run.Status || len(stored.Stages) != len(run.Stages) ||
		len(stored.Diagnostics) != len(run.Diagnostics) || len(stored.Lineage) != len(run.Lineage) ||
		stored.StartedAt == nil || stored.FinishedAt == nil {
		t.Fatalf("stored run = %#v, returned run = %#v", stored, run)
	}
	for i, stage := range stored.Stages {
		if stage.Status != StageStatusSucceeded || stage.FinishedAt == nil || stage.FinishedAt.Before(stage.StartedAt) {
			t.Fatalf("stored stage %d = %#v", i, stage)
		}
	}
	if _, err := store.Store.UpdateRun(ctx, *stored); !errors.Is(err, ErrImmutable) {
		t.Fatalf("terminal UpdateRun error = %v, want ErrImmutable", err)
	}
}

func TestRunnerFailedParsePublishesOneBatchEndingInRunFailed(t *testing.T) {
	ctx := context.Background()
	store := newCountingStore(NewMemoryStore())
	log := &memoryStreamLog{}
	runner := NewRunner(store, NewDurableHub(log, nil))
	sess, sample := seedRunnerSample(t, store, "this is not an HL7 message")

	run, err := runner.RunHL7v2(ctx, RunRequest{SessionID: sess.ID, SampleID: sample.ID})
	if err == nil || run == nil || run.Status != RunStatusFailed {
		t.Fatalf("RunHL7v2 = %#v, %v; want a failed run and its parse error", run, err)
	}
	if store.calls["UpdateRun"] != 1 || store.calls["CreateRun"] != 1 {
		t.Fatalf("store calls = %v, want one CreateRun and one UpdateRun", store.calls)
	}
	if log.batches != 1 || log.singles != 0 {
		t.Fatalf("stream appends: batches = %d, singles = %d", log.batches, log.singles)
	}
	assertStreamTypes(t, log.snapshot(), []StreamEventType{
		StreamEventRunStarted,
		StreamEventStageStarted, StreamEventStageCompleted, // load_sample
		StreamEventStageStarted, StreamEventStageCompleted, // parse_hl7v2, failed
		StreamEventRunFailed,
	})
	stored, err := store.Store.GetRun(ctx, sess.ID, run.ID)
	if err != nil || stored.Status != RunStatusFailed || stored.Error == "" {
		t.Fatalf("stored failed run = %#v, %v", stored, err)
	}
	if parse := stored.Stages[len(stored.Stages)-1]; parse.Name != "parse_hl7v2" || parse.Status != StageStatusFailed || parse.Error == "" {
		t.Fatalf("stored parse stage = %#v", parse)
	}
	if _, err := store.Store.UpdateRun(ctx, *stored); !errors.Is(err, ErrImmutable) {
		t.Fatalf("terminal UpdateRun error = %v, want ErrImmutable", err)
	}
}

func TestRunnerWithProfileRevisionAddsExactlyOneRead(t *testing.T) {
	ctx := context.Background()
	store := newCountingStore(NewMemoryStore())
	runner := NewRunner(store, NewDurableHub(&memoryStreamLog{}, nil))
	sess, sample := seedRunnerSample(t, store, missingPV1Sample(t))
	revision, err := store.SaveArtifactDraft(ctx, sess.ID, SaveArtifactDraftRequest{
		Kind: ArtifactKindMappingProfile, Name: "tolerant",
		Content: []byte(`{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","tolerance":{"missing_segments":["PV1"],"nte_anywhere":false,"extra_components":false,"unknown_segments":false,"non_standard_delimiters":false},"event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]}}`),
	})
	if err != nil {
		t.Fatalf("SaveArtifactDraft: %v", err)
	}
	before := store.total()
	run, err := runner.RunHL7v2(ctx, RunRequest{SessionID: sess.ID, SampleID: sample.ID, ProfileRevisionID: revision.RevisionID})
	if err != nil || run.Status != RunStatusSucceeded || run.ProfileRevisionID != revision.RevisionID {
		t.Fatalf("RunHL7v2 = %#v, %v", run, err)
	}
	if got := store.total() - before; got != 4 || store.calls["GetArtifactRevision"] != 1 {
		t.Fatalf("store round trips per profiled run = %d (%v), want 4 with one GetArtifactRevision", got, store.calls)
	}
}

func TestRunnerPublishesNothingWhenTerminalWriteFails(t *testing.T) {
	ctx := context.Background()
	store := newCountingStore(NewMemoryStore())
	store.updateErr = errors.New("database unavailable")
	hub := NewHub()
	runner := NewRunner(store, hub)
	sess, sample := seedRunnerSample(t, store, missingPV1Sample(t))

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := hub.Subscribe(streamCtx, sess.ID)

	run, err := runner.RunHL7v2(ctx, RunRequest{SessionID: sess.ID, SampleID: sample.ID})
	if !errors.Is(err, store.updateErr) || run != nil {
		t.Fatalf("RunHL7v2 = %#v, %v; want the store's error and no run", run, err)
	}
	select {
	case event := <-stream:
		t.Fatalf("stream delivered %s for a run whose record was never written", event.Type)
	case <-time.After(100 * time.Millisecond):
	}
	runs, err := store.Store.ListRuns(ctx, sess.ID)
	if err != nil || len(runs) != 1 || runs[0].Status != RunStatusPending {
		t.Fatalf("durable runs after a failed terminal write = %#v, %v; want one still-pending claim", runs, err)
	}
}

func TestRunnerInProcessHubDeliversBatchInOrder(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	hub := NewHub()
	runner := NewRunner(store, hub)
	sess, sample := seedRunnerSample(t, store, missingPV1Sample(t))

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := hub.Subscribe(streamCtx, sess.ID)

	run, err := runner.RunHL7v2(ctx, RunRequest{SessionID: sess.ID, SampleID: sample.ID})
	if err != nil {
		t.Fatalf("RunHL7v2: %v", err)
	}
	got := make([]StreamEvent, 0, len(succeededRunStreamTypes))
	deadline := time.After(2 * time.Second)
	for len(got) < len(succeededRunStreamTypes) {
		select {
		case event := <-stream:
			got = append(got, event)
		case <-deadline:
			t.Fatalf("timed out after %v", streamTypes(got))
		}
	}
	assertStreamTypes(t, got, succeededRunStreamTypes)
	terminal, ok := got[len(got)-1].Payload.(Run)
	if !ok || terminal.ID != run.ID || terminal.Status != RunStatusSucceeded {
		t.Fatalf("terminal payload = %#v", got[len(got)-1].Payload)
	}
	if first, ok := got[0].Payload.(Run); !ok || first.Status != RunStatusRunning || first.StartedAt == nil {
		t.Fatalf("run_started payload = %#v, want the running snapshot", got[0].Payload)
	}
}

func TestRunnerFallsBackToSingleAppendsForLogsWithoutBatchSupport(t *testing.T) {
	ctx := context.Background()
	inner := &memoryStreamLog{}
	runner := NewRunner(NewMemoryStore(), NewDurableHub(singleStreamLog{inner: inner}, nil))
	sess, sample := seedRunnerSample(t, runner.store, missingPV1Sample(t))

	if _, err := runner.RunHL7v2(ctx, RunRequest{SessionID: sess.ID, SampleID: sample.ID}); err != nil {
		t.Fatalf("RunHL7v2: %v", err)
	}
	if inner.batches != 0 || inner.singles != len(succeededRunStreamTypes) {
		t.Fatalf("stream appends: batches = %d, singles = %d; want %d singles", inner.batches, inner.singles, len(succeededRunStreamTypes))
	}
	assertStreamTypes(t, inner.snapshot(), succeededRunStreamTypes)
}
