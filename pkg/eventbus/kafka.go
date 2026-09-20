package eventbus

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

type kafkaBackend struct {
	client  *kgo.Client
	options []kgo.Opt
	group   string
}

func newKafka(config map[string]string) (*kafkaBackend, error) {
	if strings.TrimSpace(config["brokers"]) == "" {
		return nil, errors.New("kafka requires brokers")
	}
	brokers := strings.Split(config["brokers"], ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
		if brokers[i] == "" {
			return nil, errors.New("kafka broker cannot be empty")
		}
	}
	timeout, err := durationOption(config, "timeout", 10*time.Second)
	if err != nil {
		return nil, err
	}
	secure, err := boolOption(config, "tls")
	if err != nil {
		return nil, err
	}
	if (config["username"] == "") != (config["password"] == "") {
		return nil, errors.New("kafka username and password must be configured together")
	}
	if config["username"] != "" && !secure {
		return nil, errors.New("kafka credentials require TLS")
	}
	if config["ca_file"] != "" && !secure {
		return nil, errors.New("kafka ca_file requires TLS")
	}
	opts := []kgo.Opt{kgo.SeedBrokers(brokers...), kgo.RequiredAcks(kgo.AllISRAcks()), kgo.RecordDeliveryTimeout(timeout), kgo.DialTimeout(timeout)}
	if config["client_id"] != "" {
		opts = append(opts, kgo.ClientID(config["client_id"]))
	}
	if secure {
		conf := &tls.Config{MinVersion: tls.VersionTLS12}
		if path := config["ca_file"]; path != "" {
			pem, err := os.ReadFile(path)
			if err != nil {
				return nil, errors.New("read kafka CA file failed")
			}
			roots, err := x509.SystemCertPool()
			if err != nil {
				roots = x509.NewCertPool()
			}
			if !roots.AppendCertsFromPEM(pem) {
				return nil, errors.New("kafka CA file contains no certificates")
			}
			conf.RootCAs = roots
		}
		opts = append(opts, kgo.DialTLSConfig(conf))
	}
	if config["username"] != "" {
		opts = append(opts, kgo.SASL(plain.Auth{User: config["username"], Pass: config["password"]}.AsMechanism()))
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("configure kafka: %w", err)
	}
	return &kafkaBackend{client: client, options: opts, group: config["group"]}, nil
}

func (b *kafkaBackend) Publish(ctx context.Context, m Message) error {
	if err := validatePublish(ctx, m); err != nil {
		return err
	}
	r := &kgo.Record{Topic: m.Topic, Key: append([]byte(nil), m.Key...), Value: append([]byte(nil), m.Value...)}
	for key, value := range m.Headers {
		r.Headers = append(r.Headers, kgo.RecordHeader{Key: key, Value: []byte(value)})
	}
	return b.client.ProduceSync(ctx, r).FirstErr()
}

func (b *kafkaBackend) Consume(ctx context.Context, topic string, handler Handler) error {
	if err := validateConsume(ctx, topic, handler); err != nil {
		return err
	}
	if b.group == "" {
		return errors.New("kafka consumer requires group")
	}
	opts := append([]kgo.Opt{}, b.options...)
	opts = append(opts, kgo.ConsumerGroup(b.group), kgo.ConsumeTopics(topic), kgo.DisableAutoCommit(), kgo.BlockRebalanceOnPoll(), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return err
	}
	defer client.CloseAllowingRebalance()
	for {
		fetches := client.PollRecords(ctx, 1)
		if err := fetches.Err(); err != nil {
			return err
		}
		for _, r := range fetches.Records() {
			m := Message{ID: fmt.Sprintf("%s/%d/%d", r.Topic, r.Partition, r.Offset), Topic: r.Topic, Key: r.Key, Value: r.Value, Headers: map[string]string{}}
			for _, header := range r.Headers {
				m.Headers[header.Key] = string(header.Value)
			}
			if err := handler.Handle(ctx, m); err != nil {
				return err
			}
			// A synchronous commit after success prevents a later record from
			// committing past a failed handler in this partition.
			if err := client.CommitRecords(ctx, r); err != nil {
				return err
			}
		}
		client.AllowRebalance()
	}
}

func (b *kafkaBackend) Close() error { b.client.Close(); return nil }
