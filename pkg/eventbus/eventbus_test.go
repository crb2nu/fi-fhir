package eventbus

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONHandler(t *testing.T) {
	data, err := os.ReadFile("../../testdata/eventbus/patient_admit.json")
	require.NoError(t, err)
	calls := 0
	want := errors.New("handler failed")
	handler := JSONHandler(func(_ context.Context, event map[string]any) error {
		calls++
		require.Equal(t, "patient_admit", event["type"])
		return want
	})
	require.ErrorIs(t, handler.Handle(context.Background(), Message{Value: data}), want)
	for _, input := range []string{"null", "[]", "{}", "{invalid}", string(data) + string(data), strings.Replace(string(data), `"id": "event-1"`, `"id": 123`, 1)} {
		require.Error(t, handler.Handle(context.Background(), Message{Value: []byte(input)}))
	}
	require.Equal(t, 1, calls)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, handler.Handle(ctx, Message{Value: data}), context.Canceled)
	require.Equal(t, 1, calls)
}

func TestChainStopsAtFailure(t *testing.T) {
	var calls []int
	want := errors.New("failure")
	h := Chain(HandlerFunc(func(context.Context, Message) error { calls = append(calls, 1); return nil }), HandlerFunc(func(context.Context, Message) error { calls = append(calls, 2); return want }), HandlerFunc(func(context.Context, Message) error { calls = append(calls, 3); return nil }))
	require.ErrorIs(t, h.Handle(context.Background(), Message{}), want)
	require.Equal(t, []int{1, 2}, calls)
}

func TestInvalidBackendConfigurations(t *testing.T) {
	for _, tt := range []struct {
		driver  string
		options map[string]string
	}{
		{"unknown", nil}, {"kafka", nil}, {"redis", nil}, {"pubsub", nil},
		{"kafka", map[string]string{"brokers": "localhost:9092", "tls": "invalid"}},
		{"kafka", map[string]string{"brokers": "localhost:9092", "username": "user", "password": "secret"}},
		{"kafka", map[string]string{"brokers": "localhost:9092", "timeout": "-1s"}},
		{"redis", map[string]string{"url": "invalid://user:secret@localhost"}},
		{"redis", map[string]string{"url": "redis://localhost", "reclaim_idle": "1s", "handler_timeout": "1s"}},
	} {
		_, err := Open(context.Background(), tt.driver, tt.options)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}
