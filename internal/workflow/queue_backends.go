package workflow

import (
	"context"
	"errors"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/eventbus"
)

// ContextQueuePublisher extends legacy queue drivers with cancellation.
type ContextQueuePublisher interface {
	PublishWithContext(context.Context, string, []byte, []byte, map[string]string) error
}

type brokerQueuePublisher struct{ backend eventbus.Backend }

func (p *brokerQueuePublisher) Publish(topic string, key, value []byte, headers map[string]string) error {
	return p.PublishWithContext(context.Background(), topic, key, value, headers)
}

func (p *brokerQueuePublisher) PublishWithContext(ctx context.Context, topic string, key, value []byte, headers map[string]string) error {
	return p.backend.Publish(ctx, eventbus.Message{Topic: topic, Key: key, Value: value, Headers: headers})
}

func (p *brokerQueuePublisher) Close() error { return p.backend.Close() }

func init() {
	for _, driver := range []string{"kafka", "redis", "pubsub"} {
		RegisterQueueDriver(driver, func(config map[string]string) (QueuePublisher, error) {
			backend, err := eventbus.Open(context.Background(), driver, config)
			if err != nil {
				return nil, err
			}
			return &brokerQueuePublisher{backend: backend}, nil
		})
	}
}

// EventHandler runs canonical JSON events through the workflow. A transform or
// action failure prevents broker acknowledgment, even when the workflow's own
// DLQ also records the failure. Successful earlier actions may repeat on retry.
func (e *Engine) EventHandler() eventbus.Handler {
	return eventbus.JSONHandler(func(ctx context.Context, event map[string]any) error {
		if e == nil {
			return errors.New("workflow engine is required")
		}
		result := e.ProcessWithContext(ctx, event)
		if err := ctx.Err(); err != nil {
			return err
		}
		return errors.Join(result.AllErrors()...)
	})
}
