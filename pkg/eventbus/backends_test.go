package eventbus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/pstest"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
)

// Real client protocols against in-process broker test servers. A failed second
// delivery must be redelivered, while successfully handled deliveries must not.
func TestBackendAcknowledgmentContract(t *testing.T) {
	for _, driver := range []string{"kafka", "redis", "pubsub"} {
		t.Run(driver, func(t *testing.T) {
			backend, topic, subscription := testBackend(t, driver)
			assertBackendAcknowledgment(t, backend, topic, subscription, 30*time.Second)
		})
	}
}

func assertBackendAcknowledgment(t *testing.T, backend Backend, topic, subscription string, phaseTimeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), phaseTimeout)
	defer cancel()
	data, err := os.ReadFile("../../testdata/eventbus/patient_admit.json")
	require.NoError(t, err)
	for i := 1; i <= 3; i++ {
		require.NoError(t, backend.Publish(ctx, Message{Topic: topic, Key: []byte("same-partition"), Value: []byte(strings.Replace(string(data), "event-1", fmt.Sprintf("event-%d", i), 1)), Headers: map[string]string{"kind": "synthetic"}}))
	}
	failure := errors.New("processing failed")
	for i, phase := range []struct {
		stop string
		want []string
	}{
		{"event-2", []string{"event-1", "event-2"}},
		{"event-3", []string{"event-2", "event-3"}},
	} {
		// Publishing and previous group assignments must not consume this
		// consumer restart's budget.
		phaseCtx, stop := context.WithTimeout(context.Background(), phaseTimeout)
		var got []string
		err := backend.Consume(phaseCtx, subscription, HandlerFunc(func(ctx context.Context, m Message) error {
			assert.Equal(t, []byte("same-partition"), m.Key)
			assert.Equal(t, "synthetic", m.Headers["kind"])
			assert.NotEmpty(t, m.ID)
			return JSONHandler(func(_ context.Context, event map[string]any) error {
				id := event["id"].(string)
				got = append(got, id)
				if id == phase.stop {
					return failure
				}
				return nil
			}).Handle(ctx, m)
		}))
		stop()
		require.ErrorIs(t, err, failure, "phase %d received %v", i+1, got)
		require.Equal(t, phase.want, got, "phase %d", i+1)
	}
	// Observe replay before canceling so startup cannot race a short idle window.
	lastCtx, stop := context.WithTimeout(context.Background(), phaseTimeout)
	defer stop()
	var got []string
	err = backend.Consume(lastCtx, subscription, JSONHandler(func(_ context.Context, event map[string]any) error {
		got = append(got, event["id"].(string))
		stop()
		return nil
	}))
	require.ErrorIs(t, err, context.Canceled, "final phase received %v", got)
	require.Equal(t, []string{"event-3"}, got)
}

func testBackend(t *testing.T, driver string) (Backend, string, string) {
	t.Helper()
	options := map[string]string{"group": "test-processors"}
	topic, subscription := "events", "events"
	switch driver {
	case "kafka":
		cluster, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, topic))
		require.NoError(t, err)
		t.Cleanup(cluster.Close)
		options["brokers"] = strings.Join(cluster.ListenAddrs(), ",")
	case "redis":
		server := miniredis.RunT(t)
		options["url"] = "redis://" + server.Addr()
		options["handler_timeout"] = "10ms"
		options["reclaim_idle"] = "30ms"
	case "pubsub":
		server := pstest.NewServer()
		t.Cleanup(func() { require.NoError(t, server.Close()) })
		t.Setenv("PUBSUB_EMULATOR_HOST", server.Addr)
		options["project_id"] = "test-project"
		subscription = "event-processors"
	}
	backend, err := Open(context.Background(), driver, options)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, backend.Close()) })
	if driver == "pubsub" {
		client := backend.(*pubsubBackend).client
		fullTopic := "projects/test-project/topics/" + topic
		_, err := client.TopicAdminClient.CreateTopic(context.Background(), &pubsubpb.Topic{Name: fullTopic})
		require.NoError(t, err)
		_, err = client.SubscriptionAdminClient.CreateSubscription(context.Background(), &pubsubpb.Subscription{Name: "projects/test-project/subscriptions/" + subscription, Topic: fullTopic, AckDeadlineSeconds: 10, EnableMessageOrdering: true})
		require.NoError(t, err)
	}
	return backend, topic, subscription
}

func TestBackendsHonorCanceledContext(t *testing.T) {
	for _, driver := range []string{"kafka", "redis", "pubsub"} {
		t.Run(driver, func(t *testing.T) {
			backend, topic, subscription := testBackend(t, driver)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			require.ErrorIs(t, backend.Publish(ctx, Message{Topic: topic, Value: []byte("event")}), context.Canceled)
			require.ErrorIs(t, backend.Consume(ctx, subscription, HandlerFunc(func(context.Context, Message) error { t.Error("handler must not run"); return nil })), context.Canceled)
		})
	}
}

func TestRedisAbandonedDeliveryReclaimed(t *testing.T) {
	backend, topic, subscription := testBackend(t, "redis")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, backend.Publish(ctx, Message{Topic: topic, Value: []byte("recover-me")}))
	failure := errors.New("worker failed")
	require.ErrorIs(t, backend.Consume(ctx, subscription, HandlerFunc(func(context.Context, Message) error { return failure })), failure)
	original := backend.(*redisBackend)
	replacement, err := newRedis(map[string]string{"url": "redis://" + original.client.Options().Addr, "group": original.group, "reclaim_idle": "30ms", "handler_timeout": "10ms"})
	require.NoError(t, err)
	defer func() { require.NoError(t, replacement.Close()) }()
	require.NotEqual(t, original.consumer, replacement.consumer)
	recovered := errors.New("recovered")
	err = replacement.Consume(ctx, subscription, HandlerFunc(func(_ context.Context, m Message) error {
		if string(m.Value) != "recover-me" {
			return fmt.Errorf("unexpected recovered value")
		}
		return recovered
	}))
	require.ErrorIs(t, err, recovered)
}
