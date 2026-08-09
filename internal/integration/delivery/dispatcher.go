package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Store is the durable state machine used by a dispatcher.
type Store interface {
	Claim(context.Context, string, time.Duration) (*WorkItem, error)
	MarkPublished(context.Context, WorkItem) error
	MarkFailed(context.Context, WorkItem, Failure, Config) (bool, error)
}

// Outcome describes one bounded dispatcher step.
type Outcome string

const (
	OutcomeIdle      Outcome = "idle"
	OutcomePublished Outcome = "published"
	OutcomeRetry     Outcome = "retry_scheduled"
	OutcomeDLQ       Outcome = "dead_lettered"
)

const (
	// OutcomeForbidden reports a claimed item dead-lettered by the destination
	// identity decision. It is distinguished from OutcomeDLQ so a caller can see
	// that no broker contact was attempted at all.
	OutcomeForbidden Outcome = "identity_forbidden"
)

// DispatchResult reports one bounded dispatcher step.
//
// It exists because RunOnce already computes a typed Outcome that Run then
// discards, so the durable delivery worker was invisible to the process it runs
// in. The shape mirrors autoroute.SweeperConfig.Observe exactly: a typed result,
// an optional non-blocking hook, and no metrics dependency inside this package.
type DispatchResult struct {
	// Outcome is the step's terminal classification.
	Outcome Outcome
	// Duration is how long the step took, including the database round trips.
	Duration time.Duration
}

// Dispatcher publishes durable work and records its terminal database outcome.
type Dispatcher struct {
	store     Store
	publisher Publisher
	workerID  string
	config    Config
	decider   DestinationDecider

	mu      sync.RWMutex
	observe func(DispatchResult, error)
}

// SetObserver binds an observation hook. It must not block.
//
// Binding is a setter rather than a constructor field so the four existing
// NewDispatcher call sites — production and tests — keep their signatures.
func (d *Dispatcher) SetObserver(observe func(DispatchResult, error)) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.observe = observe
}

func (d *Dispatcher) report(result DispatchResult, err error) {
	if d == nil {
		return
	}
	d.mu.RLock()
	observe := d.observe
	d.mu.RUnlock()
	if observe == nil {
		return
	}
	observe(result, err)
}

// NewDispatcher validates and constructs one worker without a destination
// identity decision. The dispatch path behaves exactly as it did before Slice
// 4.1c-a.
func NewDispatcher(store Store, publisher Publisher, workerID string, config Config) (*Dispatcher, error) {
	return NewDispatcherWithIdentity(store, publisher, workerID, config, nil)
}

// NewDispatcherWithIdentity constructs one worker that evaluates the
// integration.deliver decision for every claimed item before publishing it.
func NewDispatcherWithIdentity(
	store Store,
	publisher Publisher,
	workerID string,
	config Config,
	decider DestinationDecider,
) (*Dispatcher, error) {
	if store == nil || publisher == nil || !validToken(workerID, 128) {
		return nil, errors.New("delivery dispatcher dependencies are required")
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Dispatcher{
		store: store, publisher: publisher, workerID: workerID,
		config: config, decider: decider,
	}, nil
}

// RunOnce claims at most one item, publishes it, and persists the outcome.
func (d *Dispatcher) RunOnce(ctx context.Context) (Outcome, error) {
	if d == nil || ctx == nil {
		return "", errors.New("delivery dispatcher is unavailable")
	}
	item, err := d.store.Claim(ctx, d.workerID, d.config.LeaseDuration)
	if err != nil {
		return "", err
	}
	if item == nil {
		return OutcomeIdle, nil
	}
	if outcome, denied, err := d.decideIdentity(ctx, *item); denied || err != nil {
		return outcome, err
	}
	message, err := messageForWorkItem(*item)
	if err != nil {
		return "", err
	}
	publishCtx, cancel := context.WithTimeout(ctx, d.config.PublishTimeout)
	err = d.publisher.Publish(publishCtx, message)
	cancel()
	if err != nil {
		retry, storeErr := d.store.MarkFailed(ctx, *item, Failure{
			Code:      "KAFKA_PUBLISH_FAILED",
			Detail:    "Kafka did not acknowledge the delivery command",
			Retryable: true,
		}, d.config)
		if storeErr != nil {
			return "", storeErr
		}
		if retry {
			return OutcomeRetry, nil
		}
		return OutcomeDLQ, nil
	}
	if err := d.store.MarkPublished(ctx, *item); err != nil {
		return "", err
	}
	return OutcomePublished, nil
}

// decideIdentity evaluates the destination identity decision for one claimed
// item, between Claim and any broker contact.
//
// A denial is terminal: it is recorded through the existing MarkFailed with a
// non-retryable failure, so it enters the DLQ and is visible to the operator
// control plane instead of spinning the worker against a decision that cannot
// change without a new deployed revision.
func (d *Dispatcher) decideIdentity(ctx context.Context, item WorkItem) (Outcome, bool, error) {
	if d.decider == nil {
		return "", false, nil
	}
	err := d.decider.Decide(ctx, item.TenantID, item.AttemptID, item.Destination)
	if err == nil {
		return "", false, nil
	}
	var refusal Refusal
	if !errors.As(err, &refusal) {
		return "", true, err
	}
	if _, storeErr := d.store.MarkFailed(ctx, item, refusalFailure(refusal), d.config); storeErr != nil {
		return "", true, storeErr
	}
	return OutcomeForbidden, true, nil
}

// Run polls until cancellation. Processed work drains without an idle delay.
func (d *Dispatcher) Run(ctx context.Context) error {
	if d == nil || ctx == nil {
		return errors.New("delivery dispatcher is unavailable")
	}
	for {
		started := time.Now()
		outcome, err := d.RunOnce(ctx)
		d.report(DispatchResult{Outcome: outcome, Duration: time.Since(started)}, err)
		if err != nil {
			if ctx.Err() != nil {
				return nil //nolint:nilerr // Context cancellation is graceful worker shutdown.
			}
			return err
		}
		if outcome != OutcomeIdle {
			continue
		}
		timer := time.NewTimer(d.config.PollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
	}
}

// Close releases broker resources.
func (d *Dispatcher) Close() error {
	if d == nil || d.publisher == nil {
		return nil
	}
	return d.publisher.Close()
}

type deliveryCommand struct {
	Schema       string                             `json:"schema"`
	TenantID     string                             `json:"tenant_id"`
	OutboxID     string                             `json:"outbox_id"`
	AttemptID    string                             `json:"attempt_id"`
	ReceiptID    string                             `json:"receipt_id"`
	EventID      string                             `json:"event_id"`
	TraceID      string                             `json:"trace_id"`
	Destination  integration.DestinationRevisionRef `json:"destination"`
	Route        string                             `json:"route"`
	Action       string                             `json:"action"`
	AttemptCount int                                `json:"attempt_count"`
	Event        json.RawMessage                    `json:"event"`
}

func messageForWorkItem(item WorkItem) (Message, error) {
	if !validToken(item.TenantID, 256) || !validToken(item.OutboxID, 256) ||
		!validToken(item.AttemptID, 256) || !validToken(item.ReceiptID, 256) ||
		!validToken(item.EventID, 256) || !validToken(item.TraceID, 256) ||
		!validToken(item.Topic, 249) || item.AttemptCount < 1 ||
		!validDestination(item.Destination) || len(item.EventPayload) == 0 ||
		!json.Valid(item.EventPayload) {
		return Message{}, errors.New("invalid durable delivery work item")
	}
	value, err := json.Marshal(deliveryCommand{
		Schema:       deliveryCommandSchema,
		TenantID:     item.TenantID,
		OutboxID:     item.OutboxID,
		AttemptID:    item.AttemptID,
		ReceiptID:    item.ReceiptID,
		EventID:      item.EventID,
		TraceID:      item.TraceID,
		Destination:  item.Destination,
		Route:        item.Route,
		Action:       item.Action,
		AttemptCount: item.AttemptCount,
		Event:        item.EventPayload,
	})
	if err != nil {
		return Message{}, fmt.Errorf("encode delivery command: %w", err)
	}
	return Message{
		Topic: item.Topic,
		Key:   []byte(item.AttemptID),
		Value: value,
		Headers: map[string]string{
			"fi-fhir-schema":     deliveryCommandSchema,
			"fi-fhir-tenant-id":  item.TenantID,
			"fi-fhir-outbox-id":  item.OutboxID,
			"fi-fhir-attempt-id": item.AttemptID,
			"fi-fhir-receipt-id": item.ReceiptID,
			"fi-fhir-event-id":   item.EventID,
			"fi-fhir-trace-id":   item.TraceID,
		},
	}, nil
}
