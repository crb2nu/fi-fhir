package destination

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Bounded, catalog-safe failure codes for one executed destination delivery.
// They are the DLQ codes the dispatch worker records, so they carry no
// destination-supplied content of any kind — not a response body, not a header,
// not a status line, not a URL.
const (
	// FailureUnreachable marks a delivery that never produced an HTTP response:
	// a dial failure, a TLS handshake failure, or a timeout. Retryable.
	FailureUnreachable = "DELIVERY_DESTINATION_UNREACHABLE"
	// FailureUnavailable marks a destination that answered with a status class
	// worth attempting again — 408, 429, or any 5xx. Retryable.
	FailureUnavailable = "DELIVERY_DESTINATION_UNAVAILABLE"
	// FailureRejected marks a destination that answered with a status class that
	// will not change on a retry. Terminal.
	FailureRejected = "DELIVERY_DESTINATION_REJECTED"
	// FailureRedirect marks a destination that attempted to redirect the
	// delivery. A redirect is a refusal, never a follow: the target is chosen by
	// the destination and is therefore not a trust input. Terminal.
	FailureRedirect = "DELIVERY_DESTINATION_REDIRECT_REFUSED"
	// FailureCredential marks a delivery whose declared credential did not
	// resolve at dispatch time. Retryable, because a rotation window closes.
	FailureCredential = "DELIVERY_DESTINATION_CREDENTIAL_UNRESOLVED"
	// FailureUnconfigured marks a destination whose deployed revision cannot be
	// executed as declared. Terminal: it cannot change without a new deployment.
	FailureUnconfigured = "DELIVERY_DESTINATION_UNCONFIGURED"
)

// Outcome labels recorded in the delivery provenance ledger.
const (
	outcomeDelivered = "delivered"
	outcomeRetryable = "retryable"
	outcomeRefused   = "refused"
)

const (
	// maxResponseDrainBytes bounds how much of a response body is read before it
	// is discarded. Nothing in the body is ever parsed, stored, logged, or
	// classified; it is drained only so the connection can be reused or closed
	// cleanly, and bounded so a destination cannot stream into this process.
	maxResponseDrainBytes = 1 << 16
	// maxAdvisoryBytes bounds every destination-derived advisory string.
	maxAdvisoryBytes = 256
	// maxAuthorizationBytes bounds the credential a destination binding may
	// resolve to, so a misconfigured binding cannot produce an unbounded header.
	maxAuthorizationBytes = 4096
)

var (
	// ErrTransportUnavailable means the destination transport is not configured.
	// It is never a signal to publish an https destination to the broker.
	ErrTransportUnavailable = errors.New("destination transport unavailable")
	// ErrDeliveryNotCompleted is the catalog-safe kind every TransportError
	// unwraps to.
	ErrDeliveryNotCompleted = errors.New("destination delivery did not complete")

	// errRedirectRefused is returned from the client's CheckRedirect so a
	// redirect surfaces as a refusal instead of a second, destination-chosen
	// request. It is matched with errors.Is through the *url.Error wrapper.
	errRedirectRefused = errors.New("destination redirect refused")
)

// TransportError is a destination delivery that was attempted and did not
// succeed.
//
// Its three accessor methods are the contract the dispatch worker asserts on,
// exactly as RefusalError's two are. They let the worker turn an outcome into a
// bounded retryable or terminal failure without this package importing the
// worker, or the worker importing this package.
type TransportError struct {
	Code      string
	Detail    string
	Retryable bool
}

func (e *TransportError) Error() string {
	if e == nil {
		return ErrDeliveryNotCompleted.Error()
	}
	return e.Detail
}

// Is makes every transport error match ErrDeliveryNotCompleted.
func (e *TransportError) Is(target error) bool { return target == ErrDeliveryNotCompleted }

// DeliveryFailureCode reports the DLQ failure code for this delivery.
func (e *TransportError) DeliveryFailureCode() string {
	if e == nil {
		return FailureUnreachable
	}
	return e.Code
}

// DeliveryFailureDetail reports the DLQ failure detail for this delivery.
func (e *TransportError) DeliveryFailureDetail() string {
	if e == nil {
		return ErrDeliveryNotCompleted.Error()
	}
	return e.Detail
}

// DeliveryFailureRetryable reports whether another attempt is worthwhile.
func (e *TransportError) DeliveryFailureRetryable() bool {
	return e != nil && e.Retryable
}

// DeliveryRecord is the server-owned provenance of one executed destination
// delivery.
//
// Every field except the two carrying an Advisory suffix is produced by this
// process from the deployed destination revision and the verified reference.
// HTTPStatusClass is this process's own classification of the response into a
// closed five-value vocabulary; it is the only property of the response that is
// recorded, and no other property is even read.
type DeliveryRecord struct {
	TenantID                         string
	AttemptID                        string
	Transport                        TransportKind
	DestinationArtifactID            string
	DestinationRevisionID            string
	DestinationClass                 string
	DestinationDigestVerified        string
	Outcome                          string
	FailureCode                      string
	HTTPStatusClass                  string
	EndpointAdvisory                 string
	ServedCertificateSubjectAdvisory string
	CompletedAt                      time.Time
}

// DeliveryRecorder durably records one executed destination delivery.
type DeliveryRecorder interface {
	RecordDelivery(ctx context.Context, record DeliveryRecord) error
}

// TransportConfig binds the deployed destination set to the credential resolver
// and the provenance ledger.
type TransportConfig struct {
	Registry *Registry
	Resolver integration.SecretResolver
	Recorder DeliveryRecorder
	Clock    func() time.Time
}

// Transport executes deliveries for destinations whose deployed revision
// declares a transport this process performs itself.
//
// It structurally satisfies the dispatch worker's DestinationTransport without
// importing it, the same way Authorizer satisfies DestinationDecider.
//
// Trust model: nothing observed on the destination side is a trust input. The
// URL, the method, the credential binding, and the trust roots all come from the
// content-addressed revision the registry resolved and the identity decision
// already verified. A redirect is refused rather than followed, response headers
// are read for nothing, and the body is drained and discarded unparsed.
type Transport struct {
	registry *Registry
	resolver integration.SecretResolver
	recorder DeliveryRecorder
	clock    func() time.Time
}

// NewTransport validates that the deployed destination set, the credential
// resolver, and the provenance ledger are all present.
//
// All three are required together. A transport without a resolver could not
// present the declared identity, and a transport without a recorder would
// contact destinations with no durable account of having done so.
func NewTransport(config TransportConfig) (*Transport, error) {
	if config.Registry == nil || config.Resolver == nil || config.Recorder == nil {
		return nil, ErrTransportUnavailable
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Transport{
		registry: config.Registry,
		resolver: config.Resolver,
		recorder: config.Recorder,
		clock:    clock,
	}, nil
}

// DeliverDestination routes one claimed item and, when this transport owns it,
// performs the delivery.
//
// It returns false with a nil error for any destination it does not own — every
// `kafka`-transport destination — so the dispatch worker publishes the command
// to the constant delivery topic exactly as it always has.
func (t *Transport) DeliverDestination(
	ctx context.Context,
	tenantID string,
	attemptID string,
	reference integration.DestinationRevisionRef,
	payload []byte,
) (bool, error) {
	if t == nil || t.registry == nil || t.resolver == nil || t.recorder == nil || ctx == nil {
		return false, ErrTransportUnavailable
	}
	revision, err := t.registry.Resolve(tenantID, reference)
	if err != nil {
		// The identity decision resolves the same reference against the same
		// registry immediately before this call and dead-letters anything that
		// does not resolve, so reaching here means the two disagree. Fail closed:
		// an unresolvable destination must never fall through to the broker.
		return true, &TransportError{
			Code:      FailureUnconfigured,
			Detail:    "destination is not resolvable in the deployed integration revision",
			Retryable: false,
		}
	}
	if revision.Transport != TransportHTTPS {
		return false, nil
	}
	if len(payload) == 0 {
		return true, &TransportError{
			Code:      FailureUnconfigured,
			Detail:    "destination delivery payload is empty",
			Retryable: false,
		}
	}
	result := t.deliverHTTPS(ctx, revision, attemptID, payload)
	if err := t.record(ctx, tenantID, attemptID, revision, result); err != nil {
		// A provenance outage supersedes the delivery outcome, exactly as it does
		// for a refusal in Authorizer.refuse: the dispatcher surfaces an
		// infrastructure error rather than completing or dead-lettering an
		// attempt whose outcome was never written down. The outbox is
		// at-least-once by construction, and the Idempotency-Key header carried
		// on every request is what lets a destination absorb the redelivery.
		return true, err
	}
	if result.failure == nil {
		return true, nil
	}
	return true, result.failure
}

// deliveryResult is one executed delivery, classified.
type deliveryResult struct {
	failure           *TransportError
	statusClass       string
	servedCertificate string
	completedAt       time.Time
}

// deliverHTTPS performs exactly one request against the destination's declared
// endpoint under its declared identity.
func (t *Transport) deliverHTTPS(
	ctx context.Context,
	revision Revision,
	attemptID string,
	payload []byte,
) deliveryResult {
	policy := *revision.HTTPS

	// The credential is resolved per dispatch and zeroed before returning. It is
	// never held across dispatches and never enters a struct that is marshaled,
	// logged, or labelled. File and environment references cannot be
	// version-pinned, so a rotation is a write in place with no cache to
	// invalidate — which is exactly why there is no cache.
	material, err := t.resolver.Resolve(ctx, t.bindingReference(policy.TokenBinding))
	if err != nil || len(material) == 0 {
		zeroMaterial(material)
		return t.failed(FailureCredential,
			"destination credential did not resolve at dispatch time", true, "")
	}
	authorization, headerErr := authorizationHeader(material)
	zeroMaterial(material)
	if headerErr != nil {
		return t.failed(FailureUnconfigured,
			"destination credential is not usable as a bearer credential", false, "")
	}

	roots, rootsErr := t.trustRoots(ctx, policy)
	if rootsErr != nil {
		return *rootsErr
	}

	httpTransport := &http.Transport{
		// No proxy. A proxy read from the process environment would be a trust
		// input this process did not declare in the destination revision.
		Proxy: nil,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// RootCAs nil means the system pool. InsecureSkipVerify is never set,
			// here or anywhere: a destination's served certificate is verified
			// against roots the deployment declared, never accepted on its word.
			RootCAs: roots,
		},
		ForceAttemptHTTP2:   true,
		MaxIdleConnsPerHost: 1,
		IdleConnTimeout:     time.Second,
	}
	defer httpTransport.CloseIdleConnections()
	client := &http.Client{
		Transport: httpTransport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errRedirectRefused
		},
	}

	request, err := http.NewRequestWithContext(
		ctx, policy.Method, policy.URL, bytes.NewReader(payload),
	)
	if err != nil {
		return t.failed(FailureUnconfigured,
			"destination endpoint is not a usable request target", false, "")
	}
	request.ContentLength = int64(len(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", authorization)
	// Server-owned, bounded, and derived from the durable attempt rather than
	// from anything the message asserts. It is what lets a destination absorb the
	// redelivery the outbox's at-least-once contract permits.
	request.Header.Set("Idempotency-Key", attemptID)

	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, errRedirectRefused) {
			return t.failed(FailureRedirect,
				"destination attempted a redirect, which is never followed", false, "3xx")
		}
		// The error string carries the URL, so it is never recorded, logged, or
		// wrapped into a failure detail.
		return t.failed(FailureUnreachable,
			"destination could not be reached over TLS", true, "")
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseDrainBytes))
		_ = response.Body.Close()
	}()

	class := statusClass(response.StatusCode)
	served := servedCertificateSubjectAdvisory(response.TLS)
	switch {
	case response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices:
		return deliveryResult{statusClass: class, servedCertificate: served, completedAt: t.clock().UTC()}
	case response.StatusCode == http.StatusRequestTimeout,
		response.StatusCode == http.StatusTooManyRequests,
		response.StatusCode >= http.StatusInternalServerError:
		result := t.failed(FailureUnavailable,
			"destination returned a retryable HTTP status class", true, class)
		result.servedCertificate = served
		return result
	default:
		result := t.failed(FailureRejected,
			"destination refused the delivery with a terminal HTTP status class", false, class)
		result.servedCertificate = served
		return result
	}
}

// trustRoots resolves the declared CA bundle, or reports that the deployment's
// trust configuration cannot be used. A nil pool means the system pool.
func (t *Transport) trustRoots(ctx context.Context, policy HTTPSPolicy) (*x509.CertPool, *deliveryResult) {
	if policy.CABundleBinding == "" {
		return nil, nil
	}
	bundle, err := t.resolver.Resolve(ctx, t.bindingReference(policy.CABundleBinding))
	if err != nil || len(bundle) == 0 {
		zeroMaterial(bundle)
		result := t.failed(FailureCredential,
			"destination trust bundle did not resolve at dispatch time", true, "")
		return nil, &result
	}
	pool := x509.NewCertPool()
	appended := pool.AppendCertsFromPEM(bundle)
	zeroMaterial(bundle)
	if !appended {
		result := t.failed(FailureUnconfigured,
			"destination trust bundle contains no usable certificate", false, "")
		return nil, &result
	}
	return pool, nil
}

// bindingReference maps a destination's declared binding name to the
// deployment's secret reference. An unknown name yields a zero reference, which
// every resolver rejects, so a name the deployment never declared fails closed.
// LoadRegistry already refuses a destination naming an undeclared binding, so
// this is the second of two closed doors rather than the only one.
func (t *Transport) bindingReference(name string) integration.SecretReference {
	for _, binding := range t.registry.secretBindings {
		if binding.Name == name {
			return binding.Reference
		}
	}
	return integration.SecretReference{}
}

func (t *Transport) failed(code, detail string, retryable bool, class string) deliveryResult {
	return deliveryResult{
		failure:     &TransportError{Code: code, Detail: detail, Retryable: retryable},
		statusClass: class,
		completedAt: t.clock().UTC(),
	}
}

func (t *Transport) record(
	ctx context.Context,
	tenantID string,
	attemptID string,
	revision Revision,
	result deliveryResult,
) error {
	record := DeliveryRecord{
		TenantID:                         tenantID,
		AttemptID:                        attemptID,
		Transport:                        revision.Transport,
		DestinationArtifactID:            revision.ArtifactID,
		DestinationRevisionID:            revision.RevisionID,
		DestinationClass:                 string(revision.Class),
		DestinationDigestVerified:        revision.Digest,
		Outcome:                          outcomeDelivered,
		HTTPStatusClass:                  result.statusClass,
		EndpointAdvisory:                 revision.EndpointAdvisory(),
		ServedCertificateSubjectAdvisory: result.servedCertificate,
		CompletedAt:                      result.completedAt,
	}
	if record.CompletedAt.IsZero() {
		record.CompletedAt = t.clock().UTC()
	}
	if result.failure != nil {
		record.Outcome = outcomeRefused
		if result.failure.Retryable {
			record.Outcome = outcomeRetryable
		}
		record.FailureCode = result.failure.Code
	}
	return t.recorder.RecordDelivery(ctx, record)
}

// authorizationHeader turns resolved credential material into one bearer header
// value.
//
// The material must be printable ASCII with no interior whitespace, so a
// credential cannot smuggle a second header, a control character, or a header
// terminator into the request. Trimming surrounding whitespace is deliberate:
// file-backed credentials routinely end with a newline.
func authorizationHeader(material []byte) (string, error) {
	token := strings.TrimSpace(string(material))
	if token == "" || len(token) > maxAuthorizationBytes {
		return "", ErrDeliveryNotCompleted
	}
	for _, character := range token {
		if character < 0x21 || character > 0x7e {
			return "", ErrDeliveryNotCompleted
		}
	}
	return "Bearer " + token, nil
}

// zeroMaterial overwrites resolved credential material in place.
func zeroMaterial(material []byte) {
	for index := range material {
		material[index] = 0
	}
}

// statusClass reduces a response status to the closed vocabulary the provenance
// ledger records. It is the only property of a response this process derives.
func statusClass(code int) string {
	switch {
	case code < 100 || code >= 600:
		return ""
	case code < 200:
		return "1xx"
	case code < 300:
		return "2xx"
	case code < 400:
		return "3xx"
	case code < 500:
		return "4xx"
	default:
		return "5xx"
	}
}

// servedCertificateSubjectAdvisory reports the subject of the certificate the
// destination served, bounded and stripped to printable ASCII.
//
// It is destination-derived and therefore advisory only: it is recorded for
// operator diagnostics under a column whose name and COMMENT both say it is
// never a trust input. Trust came from verifying that certificate against roots
// the deployment declared, which already happened before this value existed.
func servedCertificateSubjectAdvisory(state *tls.ConnectionState) string {
	if state == nil || len(state.PeerCertificates) == 0 {
		return ""
	}
	return boundedAdvisory(state.PeerCertificates[0].Subject.String())
}

func boundedAdvisory(value string) string {
	var bounded strings.Builder
	for _, character := range value {
		if bounded.Len() >= maxAdvisoryBytes {
			break
		}
		if character < 0x20 || character > 0x7e {
			continue
		}
		bounded.WriteRune(character)
	}
	return bounded.String()
}
