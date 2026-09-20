package eventsourcing

import (
	"context"
	"errors"
	"maps"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/eventbus"
)

// BrokerPublisher connects an OutboxRelay to an exclusively owned backend.
type BrokerPublisher struct{ Backend eventbus.Backend }

// Publish forwards the event payload, preserving the aggregate partition key
// and outbox identity. A broker acknowledgment is required before success.
func (p *BrokerPublisher) Publish(ctx context.Context, topic string, message *OutboxMessage) error {
	if p == nil || p.Backend == nil || message == nil {
		return errors.New("outbox broker publisher requires backend and message")
	}
	headers := maps.Clone(message.Metadata)
	if headers == nil {
		headers = map[string]string{}
	}
	headers["outbox_id"] = message.ID
	headers["event_type"] = message.EventType
	return p.Backend.Publish(ctx, eventbus.Message{Topic: topic, Key: []byte(message.AggregateID), Value: message.Payload, Headers: headers})
}

var _ OutboxPublisher = (*BrokerPublisher)(nil)

// Close releases the backend after the relay has stopped publishing.
func (p *BrokerPublisher) Close() error {
	if p == nil || p.Backend == nil {
		return nil
	}
	return p.Backend.Close()
}
