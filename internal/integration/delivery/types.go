package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	// OperatorRole authorizes durable replay and resubmit operations.
	OperatorRole = "integration.delivery.operator"

	deliveryCommandSchema = "integration.delivery.v1"
)

var (
	// ErrStoreUnavailable means the durable delivery store is not configured.
	ErrStoreUnavailable = errors.New("delivery store unavailable")
	// ErrLeaseLost means another worker owns or reclaimed the delivery lease.
	ErrLeaseLost = errors.New("delivery lease lost")
	// ErrInvalidOperation means replay or resubmit authorization is incomplete.
	ErrInvalidOperation = errors.New("invalid delivery operation")
	// ErrNotDeadLettered means recovery was requested for active or unknown work.
	ErrNotDeadLettered = errors.New("delivery attempt is not dead-lettered")
	// ErrOperationConflict means an idempotency key was reused for other work.
	ErrOperationConflict = errors.New("delivery operation idempotency conflict")
)

// WorkItem is one leased, raw-free delivery command and its canonical event.
type WorkItem struct {
	TenantID       string
	OutboxID       string
	AttemptID      string
	ReceiptID      string
	EventID        string
	TraceID        string
	Topic          string
	Route          string
	Action         string
	AttemptCount   int
	Destination    integration.DestinationRevisionRef
	EventPayload   json.RawMessage
	LeaseOwner     string
	LeaseExpiresAt time.Time
}

// Message is the broker-neutral record emitted by a delivery publisher.
type Message struct {
	Topic   string
	Key     []byte
	Value   []byte
	Headers map[string]string
}

// Publisher emits one message and returns only after broker acknowledgement.
type Publisher interface {
	Publish(context.Context, Message) error
	Close() error
}

// Failure is a bounded, redacted delivery failure classification.
type Failure struct {
	Code      string
	Detail    string
	Retryable bool
}

// Operation authorizes one idempotent replay or resubmit request.
type Operation struct {
	IdempotencyKey string
	Principal      integration.Principal
	Reason         string
}

// Config bounds worker leases, retries, backoff, and circuit behavior.
type Config struct {
	LeaseDuration           time.Duration
	PollInterval            time.Duration
	PublishTimeout          time.Duration
	MaxAttempts             int
	RetryBaseDelay          time.Duration
	RetryMaxDelay           time.Duration
	CircuitFailureThreshold int
	CircuitOpenDuration     time.Duration
}

// DefaultConfig returns conservative bounded production defaults.
func DefaultConfig() Config {
	return Config{
		LeaseDuration:           30 * time.Second,
		PollInterval:            250 * time.Millisecond,
		PublishTimeout:          10 * time.Second,
		MaxAttempts:             5,
		RetryBaseDelay:          time.Second,
		RetryMaxDelay:           time.Minute,
		CircuitFailureThreshold: 3,
		CircuitOpenDuration:     30 * time.Second,
	}
}

func (c Config) validate() error {
	if c.LeaseDuration <= 0 || c.PollInterval <= 0 || c.PublishTimeout <= 0 ||
		c.PublishTimeout >= c.LeaseDuration ||
		c.MaxAttempts < 1 || c.RetryBaseDelay <= 0 || c.RetryMaxDelay < c.RetryBaseDelay ||
		c.CircuitFailureThreshold < 1 || c.CircuitOpenDuration <= 0 {
		return errors.New("invalid delivery worker configuration")
	}
	return nil
}

func validDestination(destination integration.DestinationRevisionRef) bool {
	if !validToken(destination.ArtifactID, 256) || !validToken(destination.RevisionID, 256) ||
		(destination.Class != integration.DestinationClassProduction &&
			destination.Class != integration.DestinationClassSandbox) ||
		!strings.HasPrefix(destination.Digest, "sha256:") {
		return false
	}
	digest, err := hex.DecodeString(strings.TrimPrefix(destination.Digest, "sha256:"))
	return err == nil && len(digest) == sha256.Size
}
