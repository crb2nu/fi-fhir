package eventsourcing

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/eventbus"
)

type outboxBrokerProbe struct {
	message eventbus.Message
	err     error
	closed  bool
}

func (b *outboxBrokerProbe) Publish(_ context.Context, m eventbus.Message) error {
	b.message = m
	return b.err
}
func (*outboxBrokerProbe) Consume(context.Context, string, eventbus.Handler) error { return nil }
func (b *outboxBrokerProbe) Close() error                                          { b.closed = true; return nil }

func TestBrokerPublisherPreservesOutboxAndBrokerFailure(t *testing.T) {
	failed := errors.New("broker did not acknowledge")
	backend := &outboxBrokerProbe{err: failed}
	publisher := &BrokerPublisher{Backend: backend}
	message := &OutboxMessage{ID: "outbox-1", AggregateID: "aggregate-1", EventType: "patient_admit", Payload: []byte(`{"id":"event-1"}`), Metadata: map[string]string{"trace_id": "trace-1"}}
	require.ErrorIs(t, publisher.Publish(context.Background(), "events", message), failed)
	require.Equal(t, "events", backend.message.Topic)
	require.Equal(t, []byte(message.AggregateID), backend.message.Key)
	require.Equal(t, message.Payload, backend.message.Value)
	require.Equal(t, "outbox-1", backend.message.Headers["outbox_id"])
	require.Equal(t, map[string]string{"trace_id": "trace-1"}, message.Metadata)
	require.NoError(t, publisher.Close())
	require.True(t, backend.closed)
}
