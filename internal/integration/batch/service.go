package batch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

var (
	ErrUnavailable     = errors.New("batch ingestion unavailable")
	ErrInvalidMessage  = errors.New("invalid batch HL7v2 message")
	ErrRetryable       = errors.New("batch ingestion temporarily unavailable")
	ErrArchiveFailed   = errors.New("batch archive verification failed")
	ErrCompletedObject = errors.New("completed batch object cleanup failed")
)

type RunnableResolver interface {
	ResolveRunnable(context.Context, string, string) (lifecycle.RunnableBinding, error)
}

type MessageProcessor interface {
	Process(context.Context, integration.ProcessRequest) (integration.ProcessResult, error)
}

type RunnerConfig struct {
	TenantID     string
	DefinitionID string
	PrincipalID  string
	WorkerID     string
	Source       SourceRevision
	Resolver     RunnableResolver
	Processor    MessageProcessor
	Store        CheckpointStore
	Provider     Provider
	Clock        func() time.Time
}

// Runner polls one immutable source and drives exact deployed-release admission.
type Runner struct {
	tenantID     string
	definitionID string
	principalID  string
	workerID     string
	source       SourceRevision
	resolver     RunnableResolver
	processor    MessageProcessor
	store        CheckpointStore
	provider     Provider
	now          func() time.Time
	faultHook    func(string) error

	observeMu sync.RWMutex
	observe   func(PollResult, error)
}

// PollResult reports one batch poll cycle.
//
// Same shape as autoroute.SweeperConfig.Observe: a typed result, an optional
// non-blocking hook, no metrics dependency inside this package.
type PollResult struct {
	// Objects is how many source objects the poll processed.
	Objects int
	// Duration is how long the poll took.
	Duration time.Duration
}

// SetObserver binds an observation hook. It must not block.
func (r *Runner) SetObserver(observe func(PollResult, error)) {
	if r == nil {
		return
	}
	r.observeMu.Lock()
	defer r.observeMu.Unlock()
	r.observe = observe
}

func (r *Runner) report(result PollResult, err error) {
	if r == nil {
		return
	}
	r.observeMu.RLock()
	observe := r.observe
	r.observeMu.RUnlock()
	if observe == nil {
		return
	}
	observe(result, err)
}

func NewRunner(config RunnerConfig) (*Runner, error) {
	if !validIdentity(config.TenantID) || !validIdentity(config.DefinitionID) ||
		!validIdentity(config.PrincipalID) || !validIdentity(config.WorkerID) ||
		config.Source.Validate() != nil || config.Resolver == nil || config.Processor == nil ||
		config.Store == nil || config.Provider == nil || config.Provider.Type() != config.Source.Provider {
		return nil, ErrUnavailable
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Runner{
		tenantID: config.TenantID, definitionID: config.DefinitionID,
		principalID: config.PrincipalID, workerID: config.WorkerID, source: config.Source,
		resolver: config.Resolver, processor: config.Processor, store: config.Store,
		provider: config.Provider, now: clock,
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	if r == nil || ctx == nil {
		return ErrUnavailable
	}
	interval := time.Duration(r.source.PollSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		started := time.Now()
		objects, err := r.PollOnce(ctx)
		r.report(PollResult{Objects: objects, Duration: time.Since(started)}, err)
		if err != nil && !recoverablePollError(err) {
			if errors.Is(err, context.Canceled) && ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *Runner) PollOnce(ctx context.Context) (int, error) {
	if r == nil || r.resolver == nil || r.processor == nil || r.store == nil || r.provider == nil || ctx == nil {
		return 0, ErrUnavailable
	}
	binding, err := r.resolver.ResolveRunnable(ctx, r.tenantID, r.definitionID)
	if err != nil || r.source.ValidateAgainst(binding) != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 0, err
		}
		return 0, ErrUnavailable
	}
	// Connector boundary. The workload identity is evaluated against the exact
	// deployed tenant, integration revision, and source before this poll lists,
	// leases, opens, reads, loads an artifact, or writes any durable row. A
	// denied source therefore leaves no lease or checkpoint state to poison a
	// retry once its grant is repaired.
	security, err := r.authorizeSource(binding)
	if err != nil {
		return 0, err
	}
	objects, err := r.provider.List(ctx, r.source.MaxFilesPerPoll)
	if err != nil {
		return 0, fmt.Errorf("%w: list source", ErrRetryable)
	}
	processed := 0
	for _, object := range objects {
		if object.Provider != r.source.Provider || object.validate() != nil {
			return processed, ErrInvalidObject
		}
		item, err := r.store.Claim(
			ctx, r.tenantID, r.source, binding.IntegrationRevision.Digest, object, r.workerID,
			time.Duration(r.source.LeaseSeconds)*time.Second,
		)
		if err != nil {
			return processed, err
		}
		if item == nil {
			continue
		}
		if err := r.processObject(ctx, binding, security, object, *item); err != nil {
			if errors.Is(err, ErrInvalidMessage) {
				processed++
				continue
			}
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func recoverablePollError(err error) bool {
	return errors.Is(err, ErrUnavailable) || errors.Is(err, ErrRetryable) ||
		errors.Is(err, ErrArchiveFailed) || errors.Is(err, ErrCompletedObject)
}

// authorizeSource evaluates the shared fail-closed submit decision for this
// source's workload identity. It returns the exact security context every
// later decision for this poll reuses, so the connector boundary, the processor
// boundary, and transaction-scoped admission cannot disagree.
func (r *Runner) authorizeSource(binding lifecycle.RunnableBinding) (integration.SecurityContext, error) {
	principal, err := r.resolvePrincipal(binding.SourceID)
	if err != nil {
		return integration.SecurityContext{}, err
	}
	security := integration.SecurityContext{TenantID: r.tenantID, Principal: principal}
	if err := authorization.AuthorizeSubmission(
		security, r.tenantID, binding.IntegrationRevision, binding.SourceID,
	); err != nil {
		return integration.SecurityContext{}, ErrUnavailable
	}
	return security, nil
}

func (r *Runner) processObject(
	ctx context.Context,
	binding lifecycle.RunnableBinding,
	security integration.SecurityContext,
	object Object,
	item WorkItem,
) error {
	if item.Phase == PhaseCompleted {
		if _, err := r.provider.PrepareArchive(ctx, object, item.ContentDigest); err != nil {
			return ErrCompletedObject
		}
		if err := r.provider.DeleteSource(ctx, object, item.ContentDigest); err != nil {
			return ErrCompletedObject
		}
		return nil
	}
	if item.Phase == PhaseAwaitingArchive {
		return r.archive(ctx, object, item)
	}

	// The streaming digest resumes the exact hash state persisted with the last
	// checkpoint, so it covers the whole object once, in order, even when the
	// worker that started it died mid-object.
	streaming, err := newStreamDigest(item.DigestState)
	if err != nil {
		_ = r.store.Release(ctx, item, "DIGEST_STATE_LOST")
		return fmt.Errorf("%w: resume streaming digest", ErrRetryable)
	}
	reader, err := r.provider.OpenAt(ctx, object, item.CheckpointOffset)
	if err != nil {
		_ = r.store.Release(ctx, item, "OPEN_FAILED")
		return fmt.Errorf("%w: open object: %w", ErrRetryable, err)
	}
	messageReader, err := NewMessageReader(reader, item.CheckpointOffset, r.source.MaxMessageBytes)
	if err != nil {
		_ = reader.Close()
		_ = r.store.Fail(ctx, item, "INVALID_STREAM")
		return ErrInvalidMessage
	}
	for {
		message, readErr := messageReader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			_ = reader.Close()
			_ = r.store.Fail(ctx, item, "INVALID_STREAM")
			return ErrInvalidMessage
		}
		if message.StartOffset != item.CheckpointOffset || message.EndOffset <= message.StartOffset ||
			int64(len(message.Raw)) != message.EndOffset-message.StartOffset {
			_ = reader.Close()
			_ = r.store.Fail(ctx, item, "INVALID_CHECKPOINT")
			return ErrInvalidMessage
		}
		processCtx, cancel := context.WithTimeout(ctx, time.Duration(r.source.ProcessSeconds)*time.Second)
		err := r.processMessage(processCtx, binding, security, item, message)
		cancel()
		if err != nil {
			_ = reader.Close()
			if errors.Is(err, ErrInvalidMessage) {
				_ = r.store.Fail(ctx, item, "INVALID_MESSAGE")
			} else {
				_ = r.store.Release(ctx, item, "PROCESS_RETRY")
			}
			return err
		}
		digestState, digestErr := advanceStreamDigest(streaming, message.Raw)
		if digestErr != nil {
			_ = reader.Close()
			_ = r.store.Release(ctx, item, "DIGEST_STATE_LOST")
			return fmt.Errorf("%w: extend streaming digest", ErrRetryable)
		}
		if err := r.checkpoint("after_admission"); err != nil {
			_ = reader.Close()
			return err
		}
		item, err = r.store.Advance(
			ctx, item, message.EndOffset, item.CheckpointMessage+1, digestState,
			time.Duration(r.source.LeaseSeconds)*time.Second,
		)
		if err != nil {
			_ = reader.Close()
			return err
		}
	}
	if err := reader.Close(); err != nil {
		_ = r.store.Release(ctx, item, "CLOSE_FAILED")
		return fmt.Errorf("%w: close object", ErrRetryable)
	}
	if item.CheckpointMessage == 0 || item.CheckpointOffset != object.Size {
		_ = r.store.Fail(ctx, item, "INVALID_STREAM")
		return ErrInvalidMessage
	}
	streamedDigest, err := streaming.sum()
	if err != nil || !validSHA256Digest(streamedDigest) {
		_ = r.store.Release(ctx, item, "DIGEST_STATE_LOST")
		return fmt.Errorf("%w: finalize streaming digest", ErrRetryable)
	}
	digest, err := r.provider.Digest(ctx, object)
	if err != nil || !validSHA256Digest(digest) {
		_ = r.store.Release(ctx, item, "DIGEST_FAILED")
		return fmt.Errorf("%w: digest object", ErrRetryable)
	}
	// The re-read must agree with the bytes actually admitted. A disagreement
	// means the remote object was rewritten under a preserved exact-version
	// identity, so the object is quarantined rather than archived.
	if digest != streamedDigest {
		_ = r.store.Fail(ctx, item, "DIGEST_MISMATCH")
		return ErrInvalidMessage
	}
	item, err = r.store.MarkArchivePending(
		ctx, item, streamedDigest, time.Duration(r.source.LeaseSeconds)*time.Second,
	)
	if err != nil {
		return err
	}
	if err := r.checkpoint("archive_pending"); err != nil {
		return err
	}
	return r.archive(ctx, object, item)
}

func (r *Runner) processMessage(
	ctx context.Context,
	binding lifecycle.RunnableBinding,
	security integration.SecurityContext,
	item WorkItem,
	message Message,
) error {
	// Authoritative received-at is the server-owned custody timestamp recorded
	// when this exact object version was first durably admitted. Remote object
	// modification time is advisory metadata and never reaches a receipt.
	if item.ReceivedAt.IsZero() {
		return ErrUnavailable
	}
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID: r.tenantID, SourceID: binding.SourceID, Format: events.FormatHL7v2,
		ContentType: "application/hl7-v2+er7", ReceivedAt: item.ReceivedAt.UTC(),
		Classification: binding.Classification,
	}, message.Payload)
	if err != nil {
		return ErrInvalidMessage
	}
	identity := messageIdentity(
		r.source, item.IntegrationRevisionDigest, item.ObjectID,
		item.CheckpointMessage, message.StartOffset,
	)
	request := integration.ProcessRequest{
		Mode: integration.ExecutionModeProduction, IntegrationRevision: binding.IntegrationRevision,
		Security: security,
		Envelope: envelope, IdempotencyKey: "batch:v1:" + identity,
		CorrelationID: deterministicUUID(identity),
	}
	// Re-evaluated immediately before the processor loads any artifact. The
	// same decision runs again inside transaction-scoped runnable admission.
	if err := authorization.AuthorizeSubmission(
		request.Security,
		r.tenantID,
		binding.IntegrationRevision,
		binding.SourceID,
	); err != nil {
		return ErrUnavailable
	}
	_, err = r.processor.Process(ctx, request)
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, processor.ErrInvalidSourceMessage),
		errors.Is(err, processor.ErrInvalidProcessRequest),
		errors.Is(err, processor.ErrIdempotencyConflict):
		return ErrInvalidMessage
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, lifecycle.ErrNotRunnable):
		return ErrRetryable
	default:
		return ErrRetryable
	}
}

func (r *Runner) archive(ctx context.Context, object Object, item WorkItem) error {
	if _, err := r.provider.PrepareArchive(ctx, object, item.ContentDigest); err != nil {
		_ = r.store.Release(ctx, item, "ARCHIVE_FAILED")
		return ErrArchiveFailed
	}
	if err := r.checkpoint("archive_verified"); err != nil {
		return err
	}
	if err := r.store.MarkCompleted(ctx, item); err != nil {
		return err
	}
	if err := r.checkpoint("completed"); err != nil {
		return err
	}
	if err := r.provider.DeleteSource(ctx, object, item.ContentDigest); err != nil {
		return ErrCompletedObject
	}
	return nil
}

func (r *Runner) checkpoint(name string) error {
	if r.faultHook == nil {
		return nil
	}
	return r.faultHook(name)
}

func advanceStreamDigest(streaming *streamDigest, raw []byte) (string, error) {
	if err := streaming.write(raw); err != nil {
		return "", err
	}
	return streaming.state()
}

func messageIdentity(source SourceRevision, integrationRevisionDigest, objectID string, message, offset int64) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("fi-fhir/batch-message/v1\x00"))
	_, _ = fmt.Fprintf(
		hasher, "%s\x00%s\x00%s\x00%d\x00%d",
		source.Digest, integrationRevisionDigest, objectID, message, offset,
	)
	return hex.EncodeToString(hasher.Sum(nil))
}

func deterministicUUID(identity string) string {
	bytes, _ := hex.DecodeString(identity)
	identifier := append([]byte(nil), bytes[:16]...)
	identifier[6] = (identifier[6] & 0x0f) | 0x80
	identifier[8] = (identifier[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(identifier)
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32])
}
