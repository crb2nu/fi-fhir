package eventbus

import (
	"context"
	"errors"
	"maps"
	"sync"
	"time"

	"cloud.google.com/go/pubsub/v2"
)

type pubsubBackend struct {
	client     *pubsub.Client
	mu         sync.Mutex
	publishers map[string]*pubsub.Publisher
	timeout    time.Duration
}

func newPubSub(ctx context.Context, config map[string]string) (*pubsubBackend, error) {
	if config["project_id"] == "" {
		return nil, errors.New("pubsub requires project_id")
	}
	timeout, err := durationOption(config, "timeout", 10*time.Second)
	if err != nil {
		return nil, err
	}
	// The official client uses application default credentials. Its documented
	// PUBSUB_EMULATOR_HOST setting is the only path that disables authentication.
	client, err := pubsub.NewClient(ctx, config["project_id"])
	if err != nil {
		return nil, err
	}
	return &pubsubBackend{client: client, publishers: map[string]*pubsub.Publisher{}, timeout: timeout}, nil
}

func (b *pubsubBackend) Publish(ctx context.Context, m Message) error {
	if err := validatePublish(ctx, m); err != nil {
		return err
	}
	b.mu.Lock()
	publisher := b.publishers[m.Topic]
	if publisher == nil {
		publisher = b.client.Publisher(m.Topic)
		publisher.EnableMessageOrdering = true
		publisher.PublishSettings.Timeout = b.timeout
		b.publishers[m.Topic] = publisher
	}
	b.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	_, err := publisher.Publish(ctx, &pubsub.Message{Data: append([]byte(nil), m.Value...), Attributes: maps.Clone(m.Headers), OrderingKey: string(m.Key)}).Get(ctx)
	if err != nil && len(m.Key) > 0 {
		publisher.ResumePublish(string(m.Key))
	}
	return err
}

func (b *pubsubBackend) Consume(ctx context.Context, subscription string, handler Handler) error {
	if err := validateConsume(ctx, subscription, handler); err != nil {
		return err
	}
	receiveCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	subscriber := b.client.Subscriber(subscription)
	subscriber.ReceiveSettings.NumGoroutines = 1
	subscriber.ReceiveSettings.MaxOutstandingMessages = 1
	err := subscriber.Receive(receiveCtx, func(messageCtx context.Context, m *pubsub.Message) {
		if receiveCtx.Err() != nil || messageCtx.Err() != nil {
			m.Nack()
			return
		}
		if err := handler.Handle(messageCtx, Message{ID: m.ID, Topic: subscription, Key: []byte(m.OrderingKey), Value: m.Data, Headers: m.Attributes}); err != nil {
			m.Nack()
			cancel(err)
			return
		}
		if err := messageCtx.Err(); err != nil {
			m.Nack()
			return
		}
		m.Ack()
	})
	if cause := context.Cause(receiveCtx); cause != nil {
		return cause
	}
	return err
}

func (b *pubsubBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, publisher := range b.publishers {
		publisher.Stop()
	}
	return b.client.Close()
}
