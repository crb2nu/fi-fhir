package delivery

import (
	"context"
	"errors"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Non-retryable failure codes produced by the destination identity decision.
// Both land in the existing DLQ through MarkFailed, so 4.2a's control plane sees
// them alongside every other terminal failure instead of a worker spinning.
const (
	// ForbiddenFailureCode marks a dispatch whose destination identity was not
	// authorized: an unknown destination, an unbound destination under strict
	// mode, or a principal without a deliver grant.
	ForbiddenFailureCode = "DELIVERY_FORBIDDEN"
	// UnverifiedDestinationFailureCode marks a dispatch whose attempt reference
	// did not match the deployed destination revision byte for byte.
	UnverifiedDestinationFailureCode = "DELIVERY_DESTINATION_UNVERIFIED"

	defaultRefusalDetail = "delivery destination identity is not authorized"
)

// ErrDeliveryForbidden is the catalog-safe error a refusal unwraps to.
var ErrDeliveryForbidden = errors.New("delivery destination identity forbidden")

// DestinationDecider evaluates the integration.deliver decision for one claimed
// work item.
//
// It runs after Claim and before the delivery command is built or published, so
// an unauthorized attempt never reaches the broker.
//
// The signature is deliberately primitive. The package that owns the destination
// contract implements this without importing the dispatch worker, and the worker
// evaluates the decision without importing the contract: neither depends on the
// other, and the destination package stays free of transport concerns.
//
// A nil decider means destination identity is not configured for this
// deployment, and the dispatch path behaves exactly as it did before Slice
// 4.1c-a.
type DestinationDecider interface {
	Decide(
		ctx context.Context,
		tenantID string,
		attemptID string,
		destination integration.DestinationRevisionRef,
	) error
}

// Refusal is a decision not to publish, as opposed to a failure to decide.
//
// A decider returns a Refusal for a denial and an ordinary error for an
// infrastructure problem. The dispatcher dead-letters the former and surfaces
// the latter, so a database outage in the decision path never discards work.
type Refusal interface {
	error
	// DeliveryRefusalCode is the DLQ failure code. It must be a bounded,
	// catalog-safe token free of destination-supplied content.
	DeliveryRefusalCode() string
	// DeliveryRefusalDetail is the DLQ failure detail. Same constraints.
	DeliveryRefusalDetail() string
}

// DenialError is this package's Refusal implementation, used by the dispatcher's
// own tests and available to any adapter that needs one.
type DenialError struct {
	Code   string
	Detail string
}

// NewDenial builds a bounded denial.
func NewDenial(code, detail string) *DenialError {
	return &DenialError{Code: code, Detail: detail}
}

func (d *DenialError) Error() string {
	if d == nil {
		return ErrDeliveryForbidden.Error()
	}
	return d.DeliveryRefusalDetail()
}

// Is lets every denial match ErrDeliveryForbidden without exposing its code to
// callers that only need the catalog-safe kind.
func (d *DenialError) Is(target error) bool { return target == ErrDeliveryForbidden }

// DeliveryRefusalCode implements Refusal.
func (d *DenialError) DeliveryRefusalCode() string {
	if d == nil {
		return ForbiddenFailureCode
	}
	return d.Code
}

// DeliveryRefusalDetail implements Refusal.
func (d *DenialError) DeliveryRefusalDetail() string {
	if d == nil {
		return defaultRefusalDetail
	}
	return d.Detail
}

// refusalFailure converts a refusal into the non-retryable terminal failure the
// durable store records, collapsing any out-of-range code or detail to the
// generic forbidden shape rather than producing an unwritable failure.
//
// Refusals are never retried: the destination set and the identity binding are
// properties of the deployed revision, so the same attempt would be refused
// identically on every subsequent poll.
func refusalFailure(refusal Refusal) Failure {
	failure := Failure{Retryable: false}
	if refusal != nil {
		failure.Code = refusal.DeliveryRefusalCode()
		failure.Detail = strings.TrimSpace(refusal.DeliveryRefusalDetail())
	}
	if !validToken(failure.Code, 128) {
		failure.Code = ForbiddenFailureCode
	}
	if len(failure.Detail) == 0 || len(failure.Detail) > 512 {
		failure.Detail = defaultRefusalDetail
	}
	return failure
}
