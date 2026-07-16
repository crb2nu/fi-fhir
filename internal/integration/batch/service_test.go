package batch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestRunnerCrashAfterAdmissionResumesWithoutChangingIdentity(t *testing.T) {
	t.Parallel()
	source := testS3Source(t)
	source.PollSeconds = 1
	source.Digest, _ = source.semanticDigest()
	raw := []byte(
		"MSH|^~\\&|A|B|C|D|202601010000||ADT^A01|ONE|P|2.5\rPID|1||123\r" +
			"MSH|^~\\&|A|B|C|D|202601010000||ADT^A01|TWO|P|2.5\rPID|1||456\r",
	)
	provider := newMemoryProvider(raw)
	store := newMemoryCheckpointStore()
	processor := &recordingProcessor{}
	runner := newTestRunner(t, source, provider, store, processor)
	crashed := false
	runner.faultHook = func(checkpoint string) error {
		if checkpoint == "after_admission" && !crashed {
			crashed = true
			return errors.New("simulated crash")
		}
		return nil
	}
	if _, err := runner.PollOnce(context.Background()); err == nil || !crashed {
		t.Fatalf("first poll error = %v", err)
	}
	if provider.deleted {
		t.Fatal("source deleted before checkpoint/archive completion")
	}
	runner.faultHook = nil
	if processed, err := runner.PollOnce(context.Background()); err != nil || processed != 1 {
		t.Fatalf("resumed poll = %d, %v", processed, err)
	}
	keys := processor.idempotencyKeys()
	if len(keys) != 3 || keys[0] != keys[1] || keys[1] == keys[2] {
		t.Fatalf("idempotency keys = %v", keys)
	}
	if !provider.deleted || len(provider.archives) != 1 {
		t.Fatalf("archive/delete state = deleted:%t archives:%d", provider.deleted, len(provider.archives))
	}
	item := store.onlyItem(t)
	if item.Phase != PhaseCompleted || item.CheckpointMessage != 2 || item.CheckpointOffset != int64(len(raw)) {
		t.Fatalf("checkpoint = %#v", item)
	}
}

func TestRunnerArchiveFailureRetainsExactSource(t *testing.T) {
	t.Parallel()
	source := testS3Source(t)
	raw := []byte("MSH|^~\\&|A|B|C|D|202601010000||ADT^A01|ONE|P|2.5\rPID|1||123\r")
	provider := newMemoryProvider(raw)
	provider.archiveError = ErrArchiveCollision
	store := newMemoryCheckpointStore()
	runner := newTestRunner(t, source, provider, store, &recordingProcessor{})
	if _, err := runner.PollOnce(context.Background()); !errors.Is(err, ErrArchiveFailed) {
		t.Fatalf("poll error = %v", err)
	}
	if provider.deleted {
		t.Fatal("source deleted after archive failure")
	}
	item := store.onlyItem(t)
	if item.Phase != PhaseAwaitingArchive || item.ContentDigest == "" || item.LeaseOwner != "" {
		t.Fatalf("checkpoint = %#v", item)
	}
}

func TestRunnerQuarantinesInvalidObjectWithoutFailingPoll(t *testing.T) {
	t.Parallel()
	source := testS3Source(t)
	provider := newMemoryProvider([]byte("PID|1||123\r"))
	store := newMemoryCheckpointStore()
	runner := newTestRunner(t, source, provider, store, &recordingProcessor{})
	if processed, err := runner.PollOnce(context.Background()); err != nil || processed != 1 {
		t.Fatalf("invalid-object poll = %d, %v", processed, err)
	}
	item := store.onlyItem(t)
	if item.Phase != PhaseFailed || provider.deleted {
		t.Fatalf("invalid-object state = %#v, deleted:%t", item, provider.deleted)
	}
}

func TestRunnerWaitsWhileDeploymentIsNotRunnable(t *testing.T) {
	t.Parallel()
	source := testS3Source(t)
	provider := newMemoryProvider([]byte("MSH|^~\\&|A|B|C|D|202601010000||ADT^A01|ONE|P|2.5\rPID|1||123\r"))
	runner := newTestRunner(t, source, provider, newMemoryCheckpointStore(), &recordingProcessor{})
	runner.resolver = failingRunnableResolver{err: lifecycle.ErrNotRunnable}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	select {
	case err := <-done:
		t.Fatalf("runner exited while deployment was paused: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("runner shutdown = %v", err)
	}
}

func newTestRunner(
	t *testing.T,
	source SourceRevision,
	provider Provider,
	store CheckpointStore,
	messageProcessor MessageProcessor,
) *Runner {
	t.Helper()
	binding := testBatchBinding(source)
	runner, err := NewRunner(RunnerConfig{
		TenantID: "tenant-a", DefinitionID: "definition", PrincipalID: "batch-worker",
		WorkerID: "worker-1", Source: source, Resolver: staticRunnableResolver{binding: binding},
		Processor: messageProcessor, Store: store, Provider: provider,
		Clock: func() time.Time { return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

type staticRunnableResolver struct{ binding lifecycle.RunnableBinding }

func (r staticRunnableResolver) ResolveRunnable(context.Context, string, string) (lifecycle.RunnableBinding, error) {
	return r.binding, nil
}

type failingRunnableResolver struct{ err error }

func (r failingRunnableResolver) ResolveRunnable(context.Context, string, string) (lifecycle.RunnableBinding, error) {
	return lifecycle.RunnableBinding{}, r.err
}

type recordingProcessor struct {
	mu   sync.Mutex
	keys []string
}

func (p *recordingProcessor) Process(_ context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys = append(p.keys, request.IdempotencyKey)
	return integration.ProcessResult{}, nil
}

func (p *recordingProcessor) idempotencyKeys() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.keys...)
}

type memoryProvider struct {
	object       Object
	raw          []byte
	archives     map[string][]byte
	archiveError error
	deleted      bool
}

func newMemoryProvider(raw []byte) *memoryProvider {
	modified := time.Date(2026, 7, 16, 11, 0, 0, 0, time.UTC)
	return &memoryProvider{
		object: Object{
			Provider: ProviderS3, Path: "incoming/messages.hl7", Version: "etag:version-1",
			Size: int64(len(raw)), ModifiedAt: modified,
		},
		raw: append([]byte(nil), raw...), archives: map[string][]byte{},
	}
}

func (p *memoryProvider) Type() ProviderType { return ProviderS3 }
func (p *memoryProvider) List(context.Context, int) ([]Object, error) {
	if p.deleted {
		return nil, nil
	}
	return []Object{p.object}, nil
}
func (p *memoryProvider) OpenAt(_ context.Context, object Object, offset int64) (io.ReadCloser, error) {
	if p.deleted || object != p.object || offset < 0 || offset > int64(len(p.raw)) {
		return nil, ErrObjectChanged
	}
	return io.NopCloser(bytes.NewReader(p.raw[offset:])), nil
}
func (p *memoryProvider) Digest(context.Context, Object) (string, error) {
	sum := sha256.Sum256(p.raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
func (p *memoryProvider) PrepareArchive(_ context.Context, object Object, digest string) (string, error) {
	if p.archiveError != nil {
		return "", p.archiveError
	}
	if p.deleted || object != p.object {
		return "", ErrObjectChanged
	}
	sum := sha256.Sum256(p.raw)
	if digest != "sha256:"+hex.EncodeToString(sum[:]) {
		return "", ErrArchiveCollision
	}
	p.archives[digest] = append([]byte(nil), p.raw...)
	return "archive/" + digest, nil
}
func (p *memoryProvider) DeleteSource(_ context.Context, object Object, expectedDigest string) error {
	if object != p.object {
		return ErrObjectChanged
	}
	sum := sha256.Sum256(p.raw)
	if expectedDigest != "sha256:"+hex.EncodeToString(sum[:]) {
		return ErrObjectChanged
	}
	p.deleted = true
	return nil
}
func (p *memoryProvider) Close() error { return nil }

type memoryCheckpointStore struct {
	mu    sync.Mutex
	items map[string]WorkItem
}

func newMemoryCheckpointStore() *memoryCheckpointStore {
	return &memoryCheckpointStore{items: map[string]WorkItem{}}
}

func (s *memoryCheckpointStore) Claim(
	_ context.Context,
	tenantID string,
	source SourceRevision,
	integrationRevisionDigest string,
	object Object,
	workerID string,
	lease time.Duration,
) (*WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, _ := objectID(source, object)
	item, ok := s.items[id]
	if !ok {
		item = WorkItem{
			TenantID: tenantID, SourceID: source.SourceID, SourceRevisionDigest: source.Digest,
			IntegrationRevisionDigest: integrationRevisionDigest,
			ObjectID:                  id, Provider: object.Provider, ObjectSize: object.Size, Phase: PhaseProcessing,
		}
	}
	if item.IntegrationRevisionDigest != integrationRevisionDigest {
		return nil, ErrObjectChanged
	}
	if item.Phase == PhaseFailed {
		return nil, nil
	}
	if item.Phase != PhaseCompleted {
		item.LeaseOwner = workerID
		item.LeaseExpiresAt = time.Now().Add(lease)
	}
	s.items[id] = item
	clone := item
	return &clone, nil
}

func (s *memoryCheckpointStore) Advance(_ context.Context, item WorkItem, offset, message int64, lease time.Duration) (WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := s.items[item.ObjectID]
	if stored.CheckpointOffset != item.CheckpointOffset || stored.CheckpointMessage != item.CheckpointMessage {
		return WorkItem{}, ErrLeaseLost
	}
	stored.CheckpointOffset = offset
	stored.CheckpointMessage = message
	stored.LeaseExpiresAt = time.Now().Add(lease)
	s.items[item.ObjectID] = stored
	return stored, nil
}

func (s *memoryCheckpointStore) MarkArchivePending(_ context.Context, item WorkItem, digest string, lease time.Duration) (WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := s.items[item.ObjectID]
	stored.Phase = PhaseAwaitingArchive
	stored.ContentDigest = digest
	stored.LeaseExpiresAt = time.Now().Add(lease)
	s.items[item.ObjectID] = stored
	return stored, nil
}

func (s *memoryCheckpointStore) MarkCompleted(_ context.Context, item WorkItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := s.items[item.ObjectID]
	stored.Phase = PhaseCompleted
	stored.LeaseOwner = ""
	stored.LeaseExpiresAt = time.Time{}
	s.items[item.ObjectID] = stored
	return nil
}

func (s *memoryCheckpointStore) Release(_ context.Context, item WorkItem, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := s.items[item.ObjectID]
	stored.LeaseOwner = ""
	stored.LeaseExpiresAt = time.Time{}
	s.items[item.ObjectID] = stored
	return nil
}

func (s *memoryCheckpointStore) Fail(_ context.Context, item WorkItem, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := s.items[item.ObjectID]
	stored.Phase = PhaseFailed
	stored.LeaseOwner = ""
	stored.LeaseExpiresAt = time.Time{}
	s.items[item.ObjectID] = stored
	return nil
}

func (s *memoryCheckpointStore) onlyItem(t *testing.T) WorkItem {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) != 1 {
		t.Fatalf("items = %d", len(s.items))
	}
	for _, item := range s.items {
		return item
	}
	return WorkItem{}
}
