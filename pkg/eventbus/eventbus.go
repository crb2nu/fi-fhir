// Package eventbus publishes and processes messages through Kafka, Redis Streams,
// and Google Cloud Pub/Sub. Delivery is at least once: handlers must be idempotent.
package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Message carries one event. ID is the broker's delivery identifier, not the
// canonical event ID. Key is a Kafka partition key or Pub/Sub ordering key.
type Message struct {
	ID      string
	Topic   string
	Key     []byte
	Value   []byte
	Headers map[string]string
}

// Handler succeeds only when processing has finished. Returning an error leaves
// the delivery unacknowledged and stops this consumer so an operator can repair
// or quarantine a poison message without silently dropping it.
type Handler interface {
	Handle(context.Context, Message) error
}

type HandlerFunc func(context.Context, Message) error

func (f HandlerFunc) Handle(ctx context.Context, m Message) error { return f(ctx, m) }

// Backend owns network clients. Cancel Consume and wait for it to return before
// Close. Publish may be called concurrently; run only one Consume per instance.
type Backend interface {
	Publish(context.Context, Message) error
	Consume(context.Context, string, Handler) error
	Close() error
}

// Open configures a backend without creating cloud topics or subscriptions.
// See docs/operations/EVENT-BACKENDS.md for each driver's options.
func Open(ctx context.Context, driver string, options map[string]string) (Backend, error) {
	if ctx == nil {
		return nil, errors.New("event backend requires context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch driver {
	case "kafka":
		return newKafka(options)
	case "redis":
		return newRedis(options)
	case "pubsub":
		return newPubSub(ctx, options)
	default:
		return nil, fmt.Errorf("unsupported event backend %q (kafka, redis, pubsub)", driver)
	}
}

// JSONHandler adapts a canonical event callback to the common broker contract.
// Invalid input never reaches a business handler. Error messages omit payloads.
func JSONHandler(handle func(context.Context, map[string]any) error) Handler {
	return HandlerFunc(func(ctx context.Context, m Message) error {
		if handle == nil {
			return errors.New("event handler is required")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		var event map[string]any
		if err := json.Unmarshal(m.Value, &event); err != nil || event == nil {
			return errors.New("message must contain one JSON event object")
		}
		for _, key := range []string{"id", "type", "source"} {
			if value, ok := event[key].(string); !ok || value == "" {
				return fmt.Errorf("event requires string %s", key)
			}
		}
		return handle(ctx, event)
	})
}

// Chain runs handlers in order, stopping on the first error. Earlier handlers
// can run again after a later failure, so every handler must be idempotent.
func Chain(handlers ...Handler) Handler {
	return HandlerFunc(func(ctx context.Context, m Message) error {
		for _, handler := range handlers {
			if err := ctx.Err(); err != nil {
				return err
			}
			if handler == nil {
				return errors.New("event handler is required")
			}
			if err := handler.Handle(ctx, m); err != nil {
				return err
			}
		}
		return nil
	})
}

func durationOption(options map[string]string, name string, fallback time.Duration) (time.Duration, error) {
	if options[name] == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(options[name])
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return v, nil
}

func boolOption(options map[string]string, name string) (bool, error) {
	if options[name] == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(options[name])
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return v, nil
}

func validatePublish(ctx context.Context, m Message) error {
	if ctx == nil {
		return errors.New("publish requires context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.Topic == "" || len(m.Value) == 0 {
		return errors.New("publish requires topic and value")
	}
	return nil
}

func validateConsume(ctx context.Context, subscription string, handler Handler) error {
	if ctx == nil || subscription == "" || handler == nil {
		return errors.New("consume requires context, subscription and handler")
	}
	return ctx.Err()
}
