package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type redisBackend struct {
	client         *redis.Client
	group          string
	consumer       string
	reclaimIdle    time.Duration
	handlerTimeout time.Duration
}

func newRedis(config map[string]string) (*redisBackend, error) {
	if config["url"] == "" {
		return nil, errors.New("redis requires url (redis:// or rediss://)")
	}
	opts, err := redis.ParseURL(config["url"])
	if err != nil {
		return nil, errors.New("invalid redis URL")
	}
	opts.ContextTimeoutEnabled = true
	timeout, err := durationOption(config, "timeout", 10*time.Second)
	if err != nil {
		return nil, err
	}
	opts.DialTimeout, opts.ReadTimeout, opts.WriteTimeout = timeout, timeout, timeout
	idle, err := durationOption(config, "reclaim_idle", time.Minute)
	if err != nil {
		return nil, err
	}
	handlerTimeout, err := durationOption(config, "handler_timeout", 30*time.Second)
	if err != nil {
		return nil, err
	}
	if idle < 2*handlerTimeout {
		return nil, errors.New("redis reclaim_idle must be at least twice handler_timeout")
	}
	return &redisBackend{client: redis.NewClient(opts), group: config["group"], consumer: uuid.NewString(), reclaimIdle: idle, handlerTimeout: handlerTimeout}, nil
}

func (b *redisBackend) Publish(ctx context.Context, m Message) error {
	if err := validatePublish(ctx, m); err != nil {
		return err
	}
	headers, err := json.Marshal(m.Headers)
	if err != nil {
		return err
	}
	// No MAXLEN trimming: deleting pending entries would discard failed events.
	return b.client.XAdd(ctx, &redis.XAddArgs{Stream: m.Topic, Values: map[string]any{"value": m.Value, "key": m.Key, "headers": string(headers)}}).Err()
}

func (b *redisBackend) Consume(ctx context.Context, stream string, handler Handler) error {
	if err := validateConsume(ctx, stream, handler); err != nil {
		return err
	}
	if b.group == "" {
		return errors.New("redis consumer requires group")
	}
	if err := b.client.XGroupCreateMkStream(ctx, stream, b.group, "0").Err(); err != nil && !strings.HasPrefix(err.Error(), "BUSYGROUP ") {
		return err
	}
	// Resume this consumer's failed deliveries immediately. XAUTOCLAIM below
	// recovers deliveries left by other, crashed consumer instances.
	for {
		batches, err := b.client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: b.group, Consumer: b.consumer, Streams: []string{stream, "0"}, Count: 1}).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if len(batches) == 0 || len(batches[0].Messages) == 0 {
			break
		}
		for _, batch := range batches {
			for _, m := range batch.Messages {
				if err := b.handle(ctx, stream, m, handler); err != nil {
					return err
				}
			}
		}
	}
	cursor := "0-0"
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Claim abandoned deliveries before reading fresh messages. The cursor
		// advances so a large pending list cannot starve later failed messages.
		pending, next, err := b.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{Stream: stream, Group: b.group, Consumer: b.consumer, MinIdle: b.reclaimIdle, Start: cursor, Count: 1}).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		cursor = next
		if cursor == "" {
			cursor = "0-0"
		}
		if len(pending) > 0 {
			for _, m := range pending {
				if err := b.handle(ctx, stream, m, handler); err != nil {
					return err
				}
			}
			continue
		}
		batches, err := b.client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: b.group, Consumer: b.consumer, Streams: []string{stream, ">"}, Count: 1, Block: time.Second}).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return err
		}
		for _, batch := range batches {
			for _, m := range batch.Messages {
				if err := b.handle(ctx, stream, m, handler); err != nil {
					return err
				}
			}
		}
	}
}

func (b *redisBackend) handle(ctx context.Context, stream string, record redis.XMessage, handler Handler) error {
	value, ok := record.Values["value"].(string)
	if !ok || value == "" {
		return errors.New("redis event is missing value")
	}
	key, _ := record.Values["key"].(string)
	var headers map[string]string
	if raw, ok := record.Values["headers"].(string); ok && raw != "" {
		if err := json.Unmarshal([]byte(raw), &headers); err != nil {
			return errors.New("redis event headers are invalid")
		}
	}
	processing, cancel := context.WithTimeout(ctx, b.handlerTimeout)
	defer cancel()
	if err := handler.Handle(processing, Message{ID: record.ID, Topic: stream, Key: []byte(key), Value: []byte(value), Headers: headers}); err != nil {
		return err
	}
	if err := processing.Err(); err != nil {
		return err
	}
	count, err := b.client.XAck(ctx, stream, b.group, record.ID).Result()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("redis delivery acknowledgment affected %d entries", count)
	}
	return nil
}

func (b *redisBackend) Close() error { return b.client.Close() }
