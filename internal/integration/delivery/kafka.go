package delivery

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

// KafkaConfig contains non-secret broker settings and out-of-band credentials.
type KafkaConfig struct {
	Brokers         []string
	ClientID        string
	TLS             bool
	RootCAPEM       []byte
	Username        string
	Password        string
	DialTimeout     time.Duration
	DeliveryTimeout time.Duration
}

// KafkaPublisher is a real, acknowledgement-aware Kafka transport.
type KafkaPublisher struct {
	client *kgo.Client
}

// NewKafkaPublisher configures idempotent production and all-ISR acknowledgement.
func NewKafkaPublisher(config KafkaConfig) (*KafkaPublisher, error) {
	if len(config.Brokers) == 0 {
		return nil, errors.New("Kafka brokers are required")
	}
	for _, broker := range config.Brokers {
		if !validToken(broker, 512) || !strings.Contains(broker, ":") {
			return nil, errors.New("Kafka broker address is invalid")
		}
	}
	if config.ClientID == "" {
		config.ClientID = "fi-fhir-delivery"
	}
	if !validToken(config.ClientID, 128) {
		return nil, errors.New("Kafka client ID is invalid")
	}
	if config.DialTimeout <= 0 {
		config.DialTimeout = 10 * time.Second
	}
	if config.DeliveryTimeout <= 0 {
		config.DeliveryTimeout = 10 * time.Second
	}
	if (config.Username == "") != (config.Password == "") {
		return nil, errors.New("Kafka username and password must be configured together")
	}
	if config.Username != "" && !config.TLS {
		return nil, errors.New("Kafka credentials require TLS")
	}

	options := []kgo.Opt{
		kgo.SeedBrokers(config.Brokers...),
		kgo.ClientID(config.ClientID),
		kgo.DialTimeout(config.DialTimeout),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordDeliveryTimeout(config.DeliveryTimeout),
		kgo.MaxProduceRequestsInflightPerBroker(1),
	}
	if config.TLS {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13}
		if len(config.RootCAPEM) > 0 {
			roots, err := x509.SystemCertPool()
			if err != nil || roots == nil {
				roots = x509.NewCertPool()
			}
			if !roots.AppendCertsFromPEM(config.RootCAPEM) {
				return nil, errors.New("Kafka root CA is invalid")
			}
			tlsConfig.RootCAs = roots
		}
		options = append(options, kgo.DialTLSConfig(tlsConfig))
	}
	if config.Username != "" {
		options = append(options, kgo.SASL(plain.Auth{
			User: config.Username,
			Pass: config.Password,
		}.AsMechanism()))
	}
	client, err := kgo.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("configure Kafka delivery publisher: %w", err)
	}
	return &KafkaPublisher{client: client}, nil
}

// Publish waits for Kafka acknowledgement of one record.
func (p *KafkaPublisher) Publish(ctx context.Context, message Message) error {
	if p == nil || p.client == nil || ctx == nil || !validToken(message.Topic, 249) ||
		len(message.Key) == 0 || len(message.Value) == 0 {
		return errors.New("Kafka publisher is unavailable")
	}
	record := &kgo.Record{Topic: message.Topic, Key: append([]byte(nil), message.Key...), Value: append([]byte(nil), message.Value...)}
	headerNames := make([]string, 0, len(message.Headers))
	for name := range message.Headers {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	for _, name := range headerNames {
		value := message.Headers[name]
		if !validToken(name, 128) || !validToken(value, 512) {
			return errors.New("Kafka delivery header is invalid")
		}
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: name, Value: []byte(value)})
	}
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return errors.New("Kafka delivery was not acknowledged")
	}
	return nil
}

// Close flushes and releases the Kafka client.
func (p *KafkaPublisher) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	p.client.Close()
	p.client = nil
	return nil
}
