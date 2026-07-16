package batch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

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
		if _, err := r.PollOnce(ctx); err != nil && !recoverablePollError(err) {
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
		if err := r.processObject(ctx, binding, object, *item); err != nil {
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

func (r *Runner) processObject(
	ctx context.Context,
	binding lifecycle.RunnableBinding,
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
		if message.StartOffset != item.CheckpointOffset || message.EndOffset <= message.StartOffset {
			_ = reader.Close()
			_ = r.store.Fail(ctx, item, "INVALID_CHECKPOINT")
			return ErrInvalidMessage
		}
		processCtx, cancel := context.WithTimeout(ctx, time.Duration(r.source.ProcessSeconds)*time.Second)
		err := r.processMessage(processCtx, binding, object, item, message)
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
		if err := r.checkpoint("after_admission"); err != nil {
			_ = reader.Close()
			return err
		}
		item, err = r.store.Advance(
			ctx, item, message.EndOffset, item.CheckpointMessage+1,
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
	digest, err := r.provider.Digest(ctx, object)
	if err != nil || !validSHA256Digest(digest) {
		_ = r.store.Release(ctx, item, "DIGEST_FAILED")
		return fmt.Errorf("%w: digest object", ErrRetryable)
	}
	item, err = r.store.MarkArchivePending(
		ctx, item, digest, time.Duration(r.source.LeaseSeconds)*time.Second,
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
	object Object,
	item WorkItem,
	message Message,
) error {
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID: r.tenantID, SourceID: binding.SourceID, Format: events.FormatHL7v2,
		ContentType: "application/hl7-v2+er7", ReceivedAt: object.ModifiedAt.UTC(),
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
		Security: integration.SecurityContext{
			TenantID: r.tenantID,
			Principal: integration.Principal{
				ID: r.principalID, Kind: integration.PrincipalKindService,
				AuthMethod: "batch-" + string(r.source.Provider), Roles: []string{SubmitRole},
				SourceID: binding.SourceID,
			},
		},
		Envelope: envelope, IdempotencyKey: "batch:v1:" + identity,
		CorrelationID: deterministicUUID(identity),
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
