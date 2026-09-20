package workflow

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/eventbus"
)

func TestQueueRegistryCreatesOneClientForEquivalentOptions(t *testing.T) {
	registry := NewQueueRegistry()
	var created atomic.Int32
	registry.RegisterDriver("test", func(map[string]string) (QueuePublisher, error) { created.Add(1); return &mockQueuePublisher{}, nil })
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Go(func() {
			_, err := registry.GetPublisher("test", map[string]string{"brokers": "host:9092", "password": "secret"})
			require.NoError(t, err)
		})
	}
	wg.Wait()
	require.Equal(t, int32(1), created.Load())
	require.NotEqual(t, registry.cacheKey("test", map[string]string{"a": "b:c=d"}), registry.cacheKey("test", map[string]string{"a": "b", "c": "d"}))
	require.NotContains(t, registry.cacheKey("test", map[string]string{"password": "secret"}), "secret")
	require.NoError(t, registry.Close())
}

type contextProbePublisher struct {
	called bool
	ctx    context.Context
}

func (p *contextProbePublisher) Publish(string, []byte, []byte, map[string]string) error {
	return errors.New("legacy method called")
}
func (p *contextProbePublisher) PublishWithContext(ctx context.Context, _ string, _, _ []byte, _ map[string]string) error {
	p.called = true
	p.ctx = ctx
	return nil
}
func (*contextProbePublisher) Close() error { return nil }

func TestQueueActionPropagatesContext(t *testing.T) {
	publisher := &contextProbePublisher{}
	RegisterQueueDriver("context-probe", func(map[string]string) (QueuePublisher, error) { return publisher, nil })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine, err := NewEngine(&Workflow{Name: "queue-context", Routes: []Route{{Name: "publish", Actions: []Action{{Type: "queue", Config: map[string]string{"driver": "context-probe", "topic": "events"}}}}}})
	require.NoError(t, err)
	require.False(t, engine.ProcessWithContext(ctx, map[string]any{"type": "patient_admit"}).HasErrors())
	require.True(t, publisher.called)
	cancel()
	require.ErrorIs(t, publisher.ctx.Err(), context.Canceled)
}

func TestWorkflowEventHandlerRejectsProcessingFailures(t *testing.T) {
	data, err := os.ReadFile("../../testdata/eventbus/patient_admit.json")
	require.NoError(t, err)
	engine, err := NewEngine(&Workflow{Name: "consumer", Routes: []Route{{Name: "admissions", Filter: Filter{EventType: StringOrSlice{"patient_admit"}}, Actions: []Action{{Type: "probe"}}}}})
	require.NoError(t, err)
	want := errors.New("action failed")
	calls := 0
	engine.RegisterAction("probe", ActionHandlerFunc(func(any, map[string]string) error { calls++; return want }))
	require.ErrorIs(t, engine.EventHandler().Handle(context.Background(), eventbus.Message{Value: data}), want)
	require.Equal(t, 1, calls)
	engine.RegisterAction("probe", ActionHandlerFunc(func(any, map[string]string) error { calls++; return nil }))
	require.NoError(t, engine.EventHandler().Handle(context.Background(), eventbus.Message{Value: data}))
	require.Equal(t, 2, calls)
}

func TestBuiltInBrokerDriversRegistered(t *testing.T) {
	for _, driver := range []string{"kafka", "redis", "pubsub"} {
		_, err := GetQueueRegistry().GetPublisher(driver, nil)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "not registered")
	}
}
