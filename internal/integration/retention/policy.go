package retention

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// ErrInvalidPolicy is returned when a retention policy document is missing,
// malformed, or declares a window the schema cannot honour.
var ErrInvalidPolicy = errors.New("invalid retention policy")

// StreamEventPruneFloor is the youngest a session stream envelope may be and
// still be prunable. It is enforced twice on purpose: here, so a misconfigured
// deployment is refused at startup with an explanation, and in the schema
// (internal/integration/session/migrations/0006_retention_expiry.sql), so no
// deployment can lower it by any route.
//
// The log is a resume cursor. A subscriber away longer than the window sees a
// gap — already the documented replica-flip behaviour — but a one-minute window
// would turn every reconnect into one.
const StreamEventPruneFloor = 24 * time.Hour

// maxPolicyDocumentBytes bounds what a deployment may hand the loader. A
// retention policy is a dozen fields; anything larger is a misconfiguration or
// an attack, and reading it into memory unbounded is neither.
const maxPolicyDocumentBytes = 64 << 10

// Policy is one tenant's retention policy as it stands in the durable record.
//
// A zero window means RETAIN INDEFINITELY for that class, which is also what an
// absent Policy means for every class. Fail-closed is the only safe default for
// a control whose failure mode is destroying clinical data.
type Policy struct {
	TenantID string
	Version  int64

	CanonicalEventRetain time.Duration
	SessionSampleRetain  time.Duration
	SessionExportRetain  time.Duration
	StreamEventRetain    time.Duration

	// Principal is the operator who authorized this retention window. It is
	// carried into the policy audit so a change of PHI retention is attributable.
	Principal integration.Principal
	Reason    string

	// DocumentDigest is the SHA-256 of the document this policy was decoded from.
	// A restart with an unchanged document is not a policy change and must not
	// mint a version or write an audit row.
	DocumentDigest string

	UpdatedAt time.Time
}

// PurgesNothing reports whether the policy authorizes no purge of any class. A
// policy that purges nothing is legal — an operator may declare
// retain-indefinitely explicitly — but the runtime says so out loud rather than
// starting a component that can never act.
func (p Policy) PurgesNothing() bool {
	return p.CanonicalEventRetain <= 0 && p.SessionSampleRetain <= 0 &&
		p.SessionExportRetain <= 0 && p.StreamEventRetain <= 0
}

// policyDocument is the deployment-supplied form, loaded the same way the
// destination registry is. Windows are Go duration strings; an omitted or empty
// window means retain indefinitely for that class.
type policyDocument struct {
	TenantID             string                `json:"tenant_id"`
	CanonicalEventRetain string                `json:"canonical_event_retain"`
	SessionSampleRetain  string                `json:"session_sample_retain"`
	SessionExportRetain  string                `json:"session_export_retain"`
	StreamEventRetain    string                `json:"stream_event_retain"`
	AuthorizedBy         integration.Principal `json:"authorized_by"`
	Reason               string                `json:"reason"`
}

// DecodePolicyDocument reads and validates a retention policy document.
//
// It refuses a document for a different tenant than the deployment's, because a
// single deployment security domain owns one tenant (the same rule the
// destination registry enforces) and a mismatched document silently purging
// nothing would be worse than a startup failure.
func DecodePolicyDocument(reader io.Reader, tenantID string) (Policy, error) {
	if reader == nil || strings.TrimSpace(tenantID) == "" {
		return Policy{}, fmt.Errorf("%w: reader and deployment tenant are required", ErrInvalidPolicy)
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxPolicyDocumentBytes+1))
	if err != nil {
		return Policy{}, fmt.Errorf("%w: read document: %w", ErrInvalidPolicy, err)
	}
	if len(raw) > maxPolicyDocumentBytes {
		return Policy{}, fmt.Errorf("%w: document exceeds %d bytes", ErrInvalidPolicy, maxPolicyDocumentBytes)
	}
	var document policyDocument
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return Policy{}, fmt.Errorf("%w: decode document: %w", ErrInvalidPolicy, err)
	}
	if document.TenantID != tenantID {
		return Policy{}, fmt.Errorf("%w: document tenant %q does not match deployment tenant %q",
			ErrInvalidPolicy, document.TenantID, tenantID)
	}
	if reason := strings.TrimSpace(document.Reason); reason == "" || len(reason) > 1024 {
		return Policy{}, fmt.Errorf("%w: reason must be 1-1024 bytes", ErrInvalidPolicy)
	}
	if err := validatePolicyPrincipal(document.AuthorizedBy); err != nil {
		return Policy{}, err
	}

	digest := sha256.Sum256(raw)
	policy := Policy{
		TenantID:       tenantID,
		Principal:      clonePrincipal(document.AuthorizedBy),
		Reason:         strings.TrimSpace(document.Reason),
		DocumentDigest: "sha256:" + hex.EncodeToString(digest[:]),
	}
	for _, field := range []struct {
		name  string
		value string
		into  *time.Duration
	}{
		{"canonical_event_retain", document.CanonicalEventRetain, &policy.CanonicalEventRetain},
		{"session_sample_retain", document.SessionSampleRetain, &policy.SessionSampleRetain},
		{"session_export_retain", document.SessionExportRetain, &policy.SessionExportRetain},
		{"stream_event_retain", document.StreamEventRetain, &policy.StreamEventRetain},
	} {
		window, err := parseRetentionWindow(field.name, field.value)
		if err != nil {
			return Policy{}, err
		}
		*field.into = window
	}
	if policy.StreamEventRetain > 0 && policy.StreamEventRetain < StreamEventPruneFloor {
		return Policy{}, fmt.Errorf("%w: stream_event_retain %s is below the %s schema prune floor",
			ErrInvalidPolicy, policy.StreamEventRetain, StreamEventPruneFloor)
	}
	return policy, nil
}

func parseRetentionWindow(name, value string) (time.Duration, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	window, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%w: %s is not a duration: %w", ErrInvalidPolicy, name, err)
	}
	if window <= 0 {
		return 0, fmt.Errorf("%w: %s must be positive; omit it to retain indefinitely", ErrInvalidPolicy, name)
	}
	// The durable record stores whole seconds, so a sub-second window would round
	// to zero and be refused by the column's CHECK at startup with a message about
	// a constraint rather than about the document. Refuse it here, where the
	// operator can see which field is wrong.
	if window < time.Second {
		return 0, fmt.Errorf("%w: %s must be at least 1s", ErrInvalidPolicy, name)
	}
	return window, nil
}

func validatePolicyPrincipal(principal integration.Principal) error {
	if strings.TrimSpace(principal.ID) == "" || strings.TrimSpace(principal.AuthMethod) == "" {
		return fmt.Errorf("%w: authorized_by requires an id and an auth_method", ErrInvalidPolicy)
	}
	switch principal.Kind {
	case integration.PrincipalKindHuman, integration.PrincipalKindService:
	default:
		return fmt.Errorf("%w: authorized_by kind %q must be human or service", ErrInvalidPolicy, principal.Kind)
	}
	return nil
}

func clonePrincipal(principal integration.Principal) integration.Principal {
	out := principal
	if principal.Roles != nil {
		out.Roles = append([]string(nil), principal.Roles...)
	}
	return out
}
