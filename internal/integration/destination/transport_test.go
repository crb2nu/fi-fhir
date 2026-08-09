package destination

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const transportTestSecret = "TRANSPORT-UNIT-SECRET-51ab"

func TestStatusClassReducesToAClosedVocabulary(t *testing.T) {
	for code, want := range map[int]string{
		100: "1xx", 200: "2xx", 204: "2xx", 299: "2xx",
		301: "3xx", 302: "3xx", 399: "3xx",
		400: "4xx", 403: "4xx", 404: "4xx", 429: "4xx",
		500: "5xx", 503: "5xx", 599: "5xx",
		0: "", 42: "", 600: "",
	} {
		if got := statusClass(code); got != want {
			t.Fatalf("statusClass(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestAuthorizationHeaderRefusesUnusableCredentials(t *testing.T) {
	tests := map[string]struct {
		material string
		want     string
		wantErr  bool
	}{
		"a file credential's trailing newline is trimmed": {
			material: "token-value\n", want: "Bearer token-value",
		},
		"surrounding whitespace is trimmed": {
			material: "  token-value  ", want: "Bearer token-value",
		},
		"an empty credential is refused":            {material: "", wantErr: true},
		"a whitespace-only credential is refused":   {material: "  \n\t ", wantErr: true},
		"an embedded newline is refused":            {material: "token\r\nX-Injected: 1", wantErr: true},
		"an interior space is refused":              {material: "token value", wantErr: true},
		"a control character is refused":            {material: "token\x00value", wantErr: true},
		"a non-ASCII credential is refused":         {material: "tokén", wantErr: true},
		"an oversized credential is refused":        {material: strings.Repeat("t", maxAuthorizationBytes+1), wantErr: true},
		"a credential exactly at the bound is kept": {material: strings.Repeat("t", maxAuthorizationBytes), want: "Bearer " + strings.Repeat("t", maxAuthorizationBytes)},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := authorizationHeader([]byte(test.material))
			if test.wantErr {
				if err == nil {
					t.Fatalf("authorizationHeader(%q) = %q, want an error", test.material, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("authorizationHeader(%q): %v", test.material, err)
			}
			if got != test.want {
				t.Fatalf("authorizationHeader(%q) = %q, want %q", test.material, got, test.want)
			}
		})
	}
}

func TestBoundedAdvisoryStripsAndBoundsDestinationDerivedText(t *testing.T) {
	if got := boundedAdvisory("CN=alpha\nX-Injected: 1"); got != "CN=alphaX-Injected: 1" {
		t.Fatalf("boundedAdvisory did not strip a control character: %q", got)
	}
	if got := boundedAdvisory(strings.Repeat("z", maxAdvisoryBytes*2)); len(got) != maxAdvisoryBytes {
		t.Fatalf("boundedAdvisory returned %d bytes, want %d", len(got), maxAdvisoryBytes)
	}
	if got := boundedAdvisory("CN=alpha, O=Acme"); got != "CN=alpha, O=Acme" {
		t.Fatalf("boundedAdvisory mangled printable ASCII: %q", got)
	}
}

func TestZeroMaterialOverwritesInPlace(t *testing.T) {
	material := []byte(transportTestSecret)
	zeroMaterial(material)
	for index, value := range material {
		if value != 0 {
			t.Fatalf("byte %d survived zeroing: %q", index, value)
		}
	}
}

func TestRegistryHasTransportReportsTheDeployedSet(t *testing.T) {
	kafkaOnly := newTransportTestRegistry(t, nil, transportTestKafkaRevision(t, "dest-kafka"))
	if kafkaOnly.HasTransport(TransportHTTPS) {
		t.Fatal("a kafka-only registry reported an https destination")
	}
	if !kafkaOnly.HasTransport(TransportKafka) {
		t.Fatal("a kafka-only registry did not report its kafka destination")
	}
	var nilRegistry *Registry
	if nilRegistry.HasTransport(TransportHTTPS) {
		t.Fatal("a nil registry reported a transport")
	}
}

// TestTransportRoutesKafkaClassToTheBroker is the router's contract: a
// kafka-class destination is not this transport's, so the dispatcher publishes
// it. That is the property TestDeliveryDispatch_ContactsNoDestination depends on.
func TestTransportRoutesKafkaClassToTheBroker(t *testing.T) {
	revision := transportTestKafkaRevision(t, "dest-kafka")
	registry := newTransportTestRegistry(t, nil, revision)
	recorder := &recordingDeliveryRecorder{}
	resolver := &mapSecretResolver{}
	transport := newTransportForTest(t, registry, resolver, recorder)

	owned, err := transport.DeliverDestination(
		context.Background(), "tenant-a", "attempt-kafka", revision.Reference(), []byte(`{}`),
	)
	if owned || err != nil {
		t.Fatalf("DeliverDestination(kafka) = %v, %v; want not-mine with no error", owned, err)
	}
	if len(recorder.records) != 0 {
		t.Fatalf("a kafka-class destination recorded %d delivery rows, want 0", len(recorder.records))
	}
	if resolver.calls != 0 {
		t.Fatalf("a kafka-class destination resolved %d credentials, want 0", resolver.calls)
	}
}

// TestTransportFailsClosedOnAnUnresolvableDestination proves an https
// destination the registry cannot resolve never falls through to the broker.
func TestTransportFailsClosedOnAnUnresolvableDestination(t *testing.T) {
	deployed := transportTestKafkaRevision(t, "dest-kafka")
	registry := newTransportTestRegistry(t, nil, deployed)
	transport := newTransportForTest(t, registry, &mapSecretResolver{}, &recordingDeliveryRecorder{})

	orphan := deployed.Reference()
	orphan.ArtifactID = "dest-orphan"
	owned, err := transport.DeliverDestination(
		context.Background(), "tenant-a", "attempt-orphan", orphan, []byte(`{}`),
	)
	if !owned {
		t.Fatal("an unresolvable destination was routed to the broker")
	}
	var failure *TransportError
	if !errors.As(err, &failure) || failure.Retryable ||
		failure.DeliveryFailureCode() != FailureUnconfigured {
		t.Fatalf("unresolvable destination error = %#v", err)
	}
}

// TestTransportDeliversUnderTheDeclaredIdentity exercises the whole client
// against a real TLS server whose certificate is trusted only through the
// destination's declared CA bundle, which is what proves trust comes from the
// deployment rather than from the destination.
func TestTransportDeliversUnderTheDeclaredIdentity(t *testing.T) {
	statuses := []int{http.StatusServiceUnavailable, http.StatusForbidden, http.StatusCreated}
	var served int
	var mu sync.Mutex
	captured := make([]string, 0, len(statuses))
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		index := served
		served++
		captured = append(captured, r.Header.Get("Authorization"))
		mu.Unlock()
		if index >= len(statuses) {
			index = len(statuses) - 1
		}
		w.WriteHeader(statuses[index])
	}))
	server.StartTLS()
	t.Cleanup(server.Close)

	revision := transportTestHTTPSRevision(t, "dest-https", server.URL, "token", "ca")
	registry := newTransportTestRegistry(t, map[string]string{"token": "token", "ca": "ca"}, revision)
	resolver := &mapSecretResolver{values: map[string]string{
		"token": transportTestSecret,
		"ca": string(pem.EncodeToMemory(&pem.Block{
			Type: "CERTIFICATE", Bytes: server.Certificate().Raw,
		})),
	}}
	recorder := &recordingDeliveryRecorder{}
	transport := newTransportForTest(t, registry, resolver, recorder)

	// 503 -> retryable, 403 -> terminal, 201 -> delivered.
	wantOutcomes := []string{outcomeRetryable, outcomeRefused, outcomeDelivered}
	wantCodes := []string{FailureUnavailable, FailureRejected, ""}
	wantClasses := []string{"5xx", "4xx", "2xx"}
	for index := range statuses {
		owned, err := transport.DeliverDestination(
			context.Background(), "tenant-a", fmt.Sprintf("attempt-%d", index),
			revision.Reference(), []byte(`{"schema":"integration.delivery.v1"}`),
		)
		if !owned {
			t.Fatalf("step %d: an https destination was routed to the broker", index)
		}
		if wantCodes[index] == "" {
			if err != nil {
				t.Fatalf("step %d: successful delivery returned %v", index, err)
			}
		} else {
			var failure *TransportError
			if !errors.As(err, &failure) || failure.DeliveryFailureCode() != wantCodes[index] {
				t.Fatalf("step %d: error = %#v, want %s", index, err, wantCodes[index])
			}
			if !errors.Is(err, ErrDeliveryNotCompleted) {
				t.Fatalf("step %d: error does not unwrap to the catalog kind", index)
			}
		}
	}
	if len(recorder.records) != len(statuses) {
		t.Fatalf("recorded %d deliveries, want %d", len(recorder.records), len(statuses))
	}
	for index, record := range recorder.records {
		if record.Outcome != wantOutcomes[index] || record.HTTPStatusClass != wantClasses[index] {
			t.Fatalf("record %d = %s/%s, want %s/%s", index,
				record.Outcome, record.HTTPStatusClass, wantOutcomes[index], wantClasses[index])
		}
		if record.EndpointAdvisory != server.URL {
			t.Fatalf("record %d advisory endpoint = %q, want %q", index, record.EndpointAdvisory, server.URL)
		}
		if record.ServedCertificateSubjectAdvisory == "" {
			t.Fatalf("record %d recorded no served certificate advisory", index)
		}
		if strings.Contains(record.FailureCode, transportTestSecret) {
			t.Fatalf("record %d carries the credential", index)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for index, authorization := range captured {
		if authorization != "Bearer "+transportTestSecret {
			t.Fatalf("request %d Authorization = %q, want the declared credential", index, authorization)
		}
	}
	// The credential is resolved once per dispatch and never cached.
	if resolver.tokenCalls != len(statuses) {
		t.Fatalf("credential resolved %d times for %d dispatches, want one each",
			resolver.tokenCalls, len(statuses))
	}
}

// TestTransportRefusesARedirectAndNeverDialsTheTarget is the trust-closure
// proof: a destination cannot choose a second address for this process to reach.
func TestTransportRefusesARedirectAndNeverDialsTheTarget(t *testing.T) {
	var targetHits int
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL+"/elsewhere")
		w.WriteHeader(http.StatusFound)
	}))
	server.StartTLS()
	t.Cleanup(server.Close)

	revision := transportTestHTTPSRevision(t, "dest-moved", server.URL, "token", "ca")
	registry := newTransportTestRegistry(t, map[string]string{"token": "token", "ca": "ca"}, revision)
	recorder := &recordingDeliveryRecorder{}
	transport := newTransportForTest(t, registry, &mapSecretResolver{values: map[string]string{
		"token": transportTestSecret,
		"ca": string(pem.EncodeToMemory(&pem.Block{
			Type: "CERTIFICATE", Bytes: server.Certificate().Raw,
		})),
	}}, recorder)

	owned, err := transport.DeliverDestination(
		context.Background(), "tenant-a", "attempt-moved", revision.Reference(), []byte(`{}`),
	)
	var failure *TransportError
	if !owned || !errors.As(err, &failure) ||
		failure.DeliveryFailureCode() != FailureRedirect || failure.Retryable {
		t.Fatalf("redirect result owned=%v err=%#v, want a terminal redirect refusal", owned, err)
	}
	if targetHits != 0 {
		t.Fatalf("the redirect target served %d requests, want 0", targetHits)
	}
	if len(recorder.records) != 1 || recorder.records[0].HTTPStatusClass != "3xx" {
		t.Fatalf("redirect provenance = %#v", recorder.records)
	}
}

// TestTransportRefusesAnUntrustedServerCertificate proves InsecureSkipVerify is
// nowhere on this path: a destination whose certificate does not chain to the
// declared roots is unreachable, not implicitly trusted.
func TestTransportRefusesAnUntrustedServerCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	revision := transportTestHTTPSRevision(t, "dest-untrusted", server.URL, "token", "ca")
	registry := newTransportTestRegistry(t, map[string]string{"token": "token", "ca": "ca"}, revision)
	recorder := &recordingDeliveryRecorder{}
	// The declared trust bundle is a root the destination's certificate does not
	// chain to. Every httptest TLS server shares one built-in certificate, so an
	// unrelated root has to be minted rather than borrowed from a second server.
	transport := newTransportForTest(t, registry, &mapSecretResolver{values: map[string]string{
		"token": transportTestSecret,
		"ca":    unrelatedRootPEM(t),
	}}, recorder)

	owned, err := transport.DeliverDestination(
		context.Background(), "tenant-a", "attempt-untrusted", revision.Reference(), []byte(`{}`),
	)
	var failure *TransportError
	if !owned || !errors.As(err, &failure) ||
		failure.DeliveryFailureCode() != FailureUnreachable || !failure.Retryable {
		t.Fatalf("untrusted certificate result owned=%v err=%#v", owned, err)
	}
	if len(recorder.records) != 1 || recorder.records[0].Outcome != outcomeRetryable {
		t.Fatalf("untrusted certificate provenance = %#v", recorder.records)
	}
	if recorder.records[0].ServedCertificateSubjectAdvisory != "" {
		t.Fatal("a refused handshake recorded a served certificate advisory")
	}
}

// TestTransportFailsClosedWhenTheCredentialDoesNotResolve proves a rotation
// window produces a retryable failure rather than an unauthenticated request.
func TestTransportFailsClosedWhenTheCredentialDoesNotResolve(t *testing.T) {
	var hits int
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	server.StartTLS()
	t.Cleanup(server.Close)

	revision := transportTestHTTPSRevision(t, "dest-rotating", server.URL, "token", "ca")
	registry := newTransportTestRegistry(t, map[string]string{"token": "token", "ca": "ca"}, revision)
	recorder := &recordingDeliveryRecorder{}
	transport := newTransportForTest(t, registry, &mapSecretResolver{}, recorder)

	owned, err := transport.DeliverDestination(
		context.Background(), "tenant-a", "attempt-rotating", revision.Reference(), []byte(`{}`),
	)
	var failure *TransportError
	if !owned || !errors.As(err, &failure) ||
		failure.DeliveryFailureCode() != FailureCredential || !failure.Retryable {
		t.Fatalf("unresolvable credential result owned=%v err=%#v", owned, err)
	}
	if hits != 0 {
		t.Fatalf("the destination served %d requests without a credential, want 0", hits)
	}
}

// TestTransportSurfacesAProvenanceOutage proves a delivery whose outcome could
// not be written down surfaces as an infrastructure error rather than silently
// completing or dead-lettering.
func TestTransportSurfacesAProvenanceOutage(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.StartTLS()
	t.Cleanup(server.Close)

	revision := transportTestHTTPSRevision(t, "dest-outage", server.URL, "token", "ca")
	registry := newTransportTestRegistry(t, map[string]string{"token": "token", "ca": "ca"}, revision)
	outage := errors.New("provenance ledger unavailable")
	transport := newTransportForTest(t, registry, &mapSecretResolver{values: map[string]string{
		"token": transportTestSecret,
		"ca": string(pem.EncodeToMemory(&pem.Block{
			Type: "CERTIFICATE", Bytes: server.Certificate().Raw,
		})),
	}}, &recordingDeliveryRecorder{err: outage})

	owned, err := transport.DeliverDestination(
		context.Background(), "tenant-a", "attempt-outage", revision.Reference(), []byte(`{}`),
	)
	if !owned || !errors.Is(err, outage) {
		t.Fatalf("provenance outage result owned=%v err=%v, want the outage surfaced", owned, err)
	}
	var failure *TransportError
	if errors.As(err, &failure) {
		t.Fatal("a provenance outage was classified as a delivery failure and would dead-letter")
	}
}

func TestNewTransportRequiresEveryDependency(t *testing.T) {
	registry := newTransportTestRegistry(t, nil, transportTestKafkaRevision(t, "dest-kafka"))
	for name, config := range map[string]TransportConfig{
		"no registry": {Resolver: &mapSecretResolver{}, Recorder: &recordingDeliveryRecorder{}},
		"no resolver": {Registry: registry, Recorder: &recordingDeliveryRecorder{}},
		"no recorder": {Registry: registry, Resolver: &mapSecretResolver{}},
	} {
		if _, err := NewTransport(config); !errors.Is(err, ErrTransportUnavailable) {
			t.Fatalf("NewTransport(%s) = %v, want ErrTransportUnavailable", name, err)
		}
	}
	var nilTransport *Transport
	if owned, err := nilTransport.DeliverDestination(
		context.Background(), "tenant-a", "attempt", integration.DestinationRevisionRef{}, nil,
	); owned || !errors.Is(err, ErrTransportUnavailable) {
		t.Fatalf("nil transport = %v, %v", owned, err)
	}
}

// TestServedCertificateSubjectAdvisoryIsBounded proves the one destination
// derived string recorded is bounded and stripped.
func TestServedCertificateSubjectAdvisoryIsBounded(t *testing.T) {
	if got := servedCertificateSubjectAdvisory(nil); got != "" {
		t.Fatalf("a plaintext exchange produced an advisory: %q", got)
	}
	if got := servedCertificateSubjectAdvisory(&tls.ConnectionState{}); got != "" {
		t.Fatalf("a peerless handshake produced an advisory: %q", got)
	}
	certificate := &x509.Certificate{}
	certificate.Subject.CommonName = strings.Repeat("n", maxAdvisoryBytes*2)
	state := &tls.ConnectionState{PeerCertificates: []*x509.Certificate{certificate}}
	if got := servedCertificateSubjectAdvisory(state); len(got) != maxAdvisoryBytes {
		t.Fatalf("advisory subject is %d bytes, want %d", len(got), maxAdvisoryBytes)
	}
}

// unrelatedRootPEM mints a self-signed certificate authority that has nothing to
// do with any destination under test.
func unrelatedRootPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate unrelated root key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "unrelated-root"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create unrelated root: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}))
}

func newTransportForTest(
	t *testing.T,
	registry *Registry,
	resolver integration.SecretResolver,
	recorder DeliveryRecorder,
) *Transport {
	t.Helper()
	transport, err := NewTransport(TransportConfig{
		Registry: registry, Resolver: resolver, Recorder: recorder,
		Clock: func() time.Time { return time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	return transport
}

func newTransportTestRegistry(
	t *testing.T,
	bindings map[string]string,
	revisions ...Revision,
) *Registry {
	t.Helper()
	declared := make([]map[string]any, 0, len(bindings))
	for name, key := range bindings {
		declared = append(declared, map[string]any{
			"name": name, "reference": map[string]string{"provider": "file", "key": key},
		})
	}
	document := map[string]any{
		"schema":    registryDocumentSchema,
		"tenant_id": "tenant-a",
		"integration_revision": map[string]string{
			"artifact_id": "integration-adt", "revision_id": "revision-1",
			"digest": "sha256:" + strings.Repeat("b", 64),
		},
		"secret_bindings": declared,
		"destinations":    revisions,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal registry document: %v", err)
	}
	registry, err := LoadRegistry(strings.NewReader(string(encoded)), ModeStrict)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	return registry
}

func transportTestHTTPSRevision(t *testing.T, artifactID, url, token, caBundle string) Revision {
	t.Helper()
	revision, err := NewRevision(RevisionInput{
		ArtifactID: artifactID, RevisionID: "destination-1", DestinationID: artifactID,
		Class: integration.DestinationClassProduction, Transport: TransportHTTPS,
		HTTPS: &HTTPSPolicy{
			URL: url, Method: "POST", TokenBinding: token, CABundleBinding: caBundle,
		},
		Identity: &ClientIdentity{
			Subject: artifactID + "-client",
			Grants:  []string{authorization.DestinationClientGrant},
		},
	})
	if err != nil {
		t.Fatalf("NewRevision(%s): %v", artifactID, err)
	}
	return revision
}

func transportTestKafkaRevision(t *testing.T, artifactID string) Revision {
	t.Helper()
	revision, err := NewRevision(RevisionInput{
		ArtifactID: artifactID, RevisionID: "destination-1", DestinationID: artifactID,
		Class: integration.DestinationClassProduction, Transport: TransportKafka,
		Kafka: &KafkaPolicy{Topic: "integration.delivery.v1"},
		Identity: &ClientIdentity{
			Subject: artifactID + "-client",
			Grants:  []string{authorization.DestinationClientGrant},
		},
	})
	if err != nil {
		t.Fatalf("NewRevision(%s): %v", artifactID, err)
	}
	return revision
}

// mapSecretResolver resolves file-provider references out of memory and counts
// how often each is asked for, so "resolved per dispatch, never cached" is
// asserted rather than assumed.
type mapSecretResolver struct {
	values     map[string]string
	calls      int
	tokenCalls int
}

func (r *mapSecretResolver) Resolve(
	ctx context.Context, reference integration.SecretReference,
) ([]byte, error) {
	if ctx == nil {
		return nil, integration.ErrSecretResolverUnavailable
	}
	r.calls++
	if reference.Key == "token" {
		r.tokenCalls++
	}
	if integration.ValidateSecretReference(reference) != nil {
		return nil, integration.ErrSecretUnresolvable
	}
	value, found := r.values[reference.Key]
	if !found || value == "" {
		return nil, integration.ErrSecretUnresolvable
	}
	return []byte(value), nil
}

type recordingDeliveryRecorder struct {
	records []DeliveryRecord
	err     error
}

func (r *recordingDeliveryRecorder) RecordDelivery(_ context.Context, record DeliveryRecord) error {
	r.records = append(r.records, record)
	return r.err
}
