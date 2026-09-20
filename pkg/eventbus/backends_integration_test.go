//go:build integration

package eventbus

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
)

func TestLiveEventBackends(t *testing.T) {
	for _, driver := range []string{"kafka", "redis"} {
		t.Run(driver, func(t *testing.T) {
			opts := map[string]string{"group": "eventbus-" + uuid.NewString()}
			if driver == "kafka" {
				opts["brokers"] = os.Getenv("EVENTBUS_KAFKA_BROKERS")
				if opts["brokers"] == "" {
					t.Skip("EVENTBUS_KAFKA_BROKERS is not set")
				}
			} else {
				opts["url"] = os.Getenv("EVENTBUS_REDIS_URL")
				if opts["url"] == "" {
					t.Skip("EVENTBUS_REDIS_URL is not set")
				}
			}
			backend, err := Open(context.Background(), driver, opts)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, backend.Close()) })
			topic := "fi-fhir-eventbus-" + uuid.NewString()
			if driver == "kafka" {
				admin := kadm.NewClient(backend.(*kafkaBackend).client)
				_, err = admin.CreateTopic(context.Background(), 1, 1, nil, topic)
				require.NoError(t, err)
			}
			assertBackendAcknowledgment(t, backend, topic, topic)
		})
	}
}
