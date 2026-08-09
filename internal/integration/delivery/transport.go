package delivery

import (
	"context"
	"errors"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	// TransportFailureCode is the catalog-safe fallback for a destination
	// delivery that failed without producing a usable bounded classification.
	TransportFailureCode = "DELIVERY_DESTINATION_FAILED"

	defaultTransportDetail = "destination delivery did not complete"
)

// DestinationTransport delivers one claimed work item to a destination whose
// deployed revision declares a transport this process executes itself.
//
// It is a second seam parallel to DestinationDecider, not a widening of it.
// Slice 4.1c-a deliberately kept the destination contract free of transport
// concerns, and an earlier shape that merged the two produced an import cycle
// the moment a test needed both packages. So the rule is the same here: this
// package declares the interface in primitives, the package that owns the
// destination contract satisfies it structurally, and neither imports the other.
//
// The router lives on the implementation side. DeliverDestination reports
// whether the claimed item's destination is one it owns:
//
//   - (false, nil) means "not mine" — the dispatcher publishes the delivery
//     command to the constant Kafka topic exactly as it always has. Every
//     `kafka`-transport destination takes this path, which is why
//     TestDeliveryDispatch_ContactsNoDestination stays true.
//   - (true, nil) means the destination was contacted and accepted the
//     delivery. The dispatcher completes the lease through the existing
//     MarkPublished.
//   - (true, err) means the delivery was attempted and did not succeed. A
//     TransportFailure or a Refusal is converted to a bounded Failure and
//     recorded through the existing MarkFailed, so retry, backoff, the
//     per-destination-artifact circuit, and the DLQ all behave identically to a
//     broker failure. Any other error is an infrastructure problem and is
//     surfaced, never dead-lettered.
//
// The payload is the same delivery command the broker would have carried. That
// is deliberate: the HTTPS transport substitutes for the broker rather than
// inventing a second wire contract, and the command is raw-free and
// address-free by the same construction messageForWorkItem already enforces.
type DestinationTransport interface {
	DeliverDestination(
		ctx context.Context,
		tenantID string,
		attemptID string,
		destination integration.DestinationRevisionRef,
		payload []byte,
	) (bool, error)
}

// TransportFailure is a destination delivery that was attempted and did not
// succeed, as opposed to a failure to decide whether to attempt it.
//
// It is the retryable-aware sibling of Refusal. A Refusal is always terminal —
// the destination set and the identity binding are properties of the deployed
// revision, so the same attempt would be refused identically forever. A
// transport failure may or may not be: a 503 or a TLS handshake failure is
// worth another attempt under the existing backoff, a 403 is not.
type TransportFailure interface {
	error
	// DeliveryFailureCode is the DLQ failure code. It must be a bounded,
	// catalog-safe token free of destination-supplied content.
	DeliveryFailureCode() string
	// DeliveryFailureDetail is the DLQ failure detail. Same constraints.
	DeliveryFailureDetail() string
	// DeliveryFailureRetryable reports whether another attempt is worthwhile.
	DeliveryFailureRetryable() bool
}

// transportFailure converts a transport error into the bounded terminal or
// retryable failure the durable store records.
//
// It reports false for anything that is neither a TransportFailure nor a
// Refusal, so an infrastructure error inside the transport surfaces to the
// caller instead of being dead-lettered as if the destination had answered.
func transportFailure(err error) (Failure, bool) {
	var classified TransportFailure
	if errors.As(err, &classified) {
		return boundedFailure(
			classified.DeliveryFailureCode(),
			classified.DeliveryFailureDetail(),
			classified.DeliveryFailureRetryable(),
		), true
	}
	var refusal Refusal
	if errors.As(err, &refusal) {
		return refusalFailure(refusal), true
	}
	return Failure{}, false
}

// boundedFailure applies the same bounds refusalFailure enforces: a
// catalog-safe code of at most 128 bytes and a detail of at most 512, both
// collapsing to a generic shape rather than producing an unwritable failure.
//
// The bounds are the reason a destination cannot write its own response body
// into a durable record by way of a failure detail.
func boundedFailure(code, detail string, retryable bool) Failure {
	failure := Failure{Code: code, Detail: strings.TrimSpace(detail), Retryable: retryable}
	if !validToken(failure.Code, 128) {
		failure.Code = TransportFailureCode
	}
	if len(failure.Detail) == 0 || len(failure.Detail) > 512 {
		failure.Detail = defaultTransportDetail
	}
	return failure
}
