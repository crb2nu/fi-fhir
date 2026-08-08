package mllp

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestClientPolicyResolvesExactlyOneMappedIdentity(t *testing.T) {
	senderA := identityCertificate(t, "spiffe://hospital-a/mllp/sender-a")
	senderB := identityCertificate(t, "spiffe://hospital-a/mllp/sender-b")
	policy := ClientPolicy{
		AllowedCIDRs: []string{"127.0.0.0/8"},
		Identities: []ClientIdentity{
			{Subject: "svc-sender-a", URISAN: "spiffe://hospital-a/mllp/sender-a", Grants: []string{SubmitRole}},
			{Subject: "svc-sender-b", URISAN: "spiffe://hospital-a/mllp/sender-b", SPKISHA256: certificateSPKIPin(senderB), Grants: []string{SubmitRole}},
		},
	}

	identity, err := policy.ResolveClientIdentity([]*x509.Certificate{senderA})
	if err != nil {
		t.Fatalf("resolve sender A: %v", err)
	}
	if identity.Subject != "svc-sender-a" || identity.AuthMethod != AuthMethodCertificateIdentity {
		t.Fatalf("sender A identity = %#v", identity)
	}
	other, err := policy.ResolveClientIdentity([]*x509.Certificate{senderB})
	if err != nil {
		t.Fatalf("resolve sender B: %v", err)
	}
	if other.Subject != "svc-sender-b" || other.Subject == identity.Subject {
		t.Fatalf("sender B identity = %#v", other)
	}
}

func TestClientPolicyRejectsUnmappedAmbiguousAndAbsentCertificates(t *testing.T) {
	senderA := identityCertificate(t, "spiffe://hospital-a/mllp/sender-a")
	unmapped := identityCertificate(t, "spiffe://hospital-a/mllp/sender-z")
	both := identityCertificate(t, "spiffe://hospital-a/mllp/sender-a", "spiffe://hospital-a/mllp/sender-b")
	policy := ClientPolicy{
		AllowedCIDRs: []string{"127.0.0.0/8"},
		Identities: []ClientIdentity{
			{Subject: "svc-sender-a", URISAN: "spiffe://hospital-a/mllp/sender-a", Grants: []string{SubmitRole}},
			{Subject: "svc-sender-b", URISAN: "spiffe://hospital-a/mllp/sender-b", Grants: []string{SubmitRole}},
		},
	}
	if _, err := policy.ResolveClientIdentity([]*x509.Certificate{unmapped}); !errors.Is(err, ErrClientIdentityUnmapped) {
		t.Fatalf("unmapped certificate error = %v", err)
	}
	if _, err := policy.ResolveClientIdentity([]*x509.Certificate{both}); !errors.Is(err, ErrClientIdentityUnmapped) {
		t.Fatalf("ambiguous certificate error = %v", err)
	}
	if _, err := policy.ResolveClientIdentity(nil); !errors.Is(err, ErrClientIdentityUnavailable) {
		t.Fatalf("absent certificate error = %v", err)
	}
	// A pinned SPKI that does not match the presented key never resolves.
	pinned := ClientPolicy{
		AllowedCIDRs: []string{"127.0.0.0/8"},
		Identities: []ClientIdentity{{
			Subject: "svc-sender-a", URISAN: "spiffe://hospital-a/mllp/sender-a",
			SPKISHA256: "sha256:" + hex.EncodeToString(make([]byte, sha256.Size)),
			Grants:     []string{SubmitRole},
		}},
	}
	if _, err := pinned.ResolveClientIdentity([]*x509.Certificate{senderA}); !errors.Is(err, ErrClientIdentityUnmapped) {
		t.Fatalf("mismatched pin error = %v", err)
	}
	if (ClientPolicy{AllowedCIDRs: []string{"127.0.0.0/8"}}).IdentityMappingEnabled() {
		t.Fatal("empty identity list must select compatibility mode")
	}
}

func TestSourceRevisionValidatesClientIdentitiesFailClosed(t *testing.T) {
	mutualTLS := TLSPolicy{
		Mode: TLSModeMutual, ServerCertificateBinding: "mllp-cert",
		ServerPrivateKeyBinding: "mllp-key", ClientCABinding: "mllp-client-ca",
	}
	valid := ClientIdentity{Subject: "svc-sender-a", URISAN: "spiffe://hospital-a/mllp/sender-a", Grants: []string{SubmitRole}}
	cases := []struct {
		name       string
		tls        TLSPolicy
		identities []ClientIdentity
	}{
		{"plaintext listener", TLSPolicy{Mode: TLSModeDisabled}, []ClientIdentity{valid}},
		{"empty subject", mutualTLS, []ClientIdentity{{URISAN: valid.URISAN}}},
		{"no criteria", mutualTLS, []ClientIdentity{{Subject: "svc-sender-a"}}},
		{"duplicate subject", mutualTLS, []ClientIdentity{valid, {Subject: "svc-sender-a", URISAN: "spiffe://hospital-a/mllp/sender-b"}}},
		{"duplicate uri san", mutualTLS, []ClientIdentity{valid, {Subject: "svc-sender-b", URISAN: valid.URISAN}}},
		{"relative uri san", mutualTLS, []ClientIdentity{{Subject: "svc-sender-a", URISAN: "hospital-a/mllp/sender-a"}}},
		{"malformed pin", mutualTLS, []ClientIdentity{{Subject: "svc-sender-a", SPKISHA256: "sha256:zz"}}},
		{"duplicate grant", mutualTLS, []ClientIdentity{{Subject: "svc-sender-a", URISAN: valid.URISAN, Grants: []string{SubmitRole, SubmitRole}}}},
		{"whitespace grant", mutualTLS, []ClientIdentity{{Subject: "svc-sender-a", URISAN: valid.URISAN, Grants: []string{"integration mllp"}}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewSourceRevision(identitySourceInput(test.tls, test.identities)); !errors.Is(err, ErrInvalidSourceRevision) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	revision, err := NewSourceRevision(identitySourceInput(mutualTLS, []ClientIdentity{valid}))
	if err != nil {
		t.Fatalf("valid identity revision: %v", err)
	}
	if err := revision.Validate(); err != nil {
		t.Fatalf("revalidate: %v", err)
	}
	// Grant order must not change the content address.
	reordered, err := NewSourceRevision(identitySourceInput(mutualTLS, []ClientIdentity{{
		Subject: valid.Subject, URISAN: valid.URISAN, Grants: []string{SubmitRole},
	}}))
	if err != nil || reordered.Digest != revision.Digest {
		t.Fatalf("digest = %s, want %s (%v)", reordered.Digest, revision.Digest, err)
	}
	// A different identity map is a different immutable source revision.
	changed, err := NewSourceRevision(identitySourceInput(mutualTLS, []ClientIdentity{{
		Subject: "svc-sender-b", URISAN: "spiffe://hospital-a/mllp/sender-b", Grants: []string{SubmitRole},
	}}))
	if err != nil || changed.Digest == revision.Digest {
		t.Fatalf("identity map change did not alter digest: %s (%v)", changed.Digest, err)
	}
}

func TestSourceRevisionDigestUnchangedWithoutIdentities(t *testing.T) {
	withoutField, err := NewSourceRevision(identitySourceInput(TLSPolicy{Mode: TLSModeDisabled}, nil))
	if err != nil {
		t.Fatal(err)
	}
	withEmptyField, err := NewSourceRevision(identitySourceInput(TLSPolicy{Mode: TLSModeDisabled}, []ClientIdentity{}))
	if err != nil {
		t.Fatal(err)
	}
	if withoutField.Digest != withEmptyField.Digest {
		t.Fatalf("compatibility digest drift: %s vs %s", withoutField.Digest, withEmptyField.Digest)
	}
}

func TestServiceRejectsCrossModeIdentityUse(t *testing.T) {
	compatibility := testSource(t)
	service, err := NewService(testServiceConfig(compatibility,
		resolverFunc(func(context.Context, string, string) (lifecycle.RunnableBinding, error) {
			t.Fatal("compatibility listener resolved a binding for a mapped identity")
			return lifecycle.RunnableBinding{}, nil
		}),
		processorFunc(func(context.Context, integration.ProcessRequest) (integration.ProcessResult, error) {
			t.Fatal("compatibility listener processed a mapped identity")
			return integration.ProcessResult{}, nil
		}),
	))
	if err != nil {
		t.Fatal(err)
	}
	mapped := ConnectionIdentity{Subject: "svc-sender-a", AuthMethod: AuthMethodCertificateIdentity, Grants: []string{SubmitRole}}
	if _, err := service.Submit(context.Background(), mapped, testHL7("CTRL1")); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("compatibility listener accepted a mapped identity: %v", err)
	}

	mappedSource := mappedTLSSource(t, []ClientIdentity{{
		Subject: "svc-sender-a", URISAN: "spiffe://hospital-a/mllp/sender-a", Grants: []string{SubmitRole},
	}})
	mappedService, err := NewService(testServiceConfig(mappedSource,
		resolverFunc(func(context.Context, string, string) (lifecycle.RunnableBinding, error) {
			t.Fatal("mapped listener resolved a binding without a verified identity")
			return lifecycle.RunnableBinding{}, nil
		}),
		processorFunc(func(context.Context, integration.ProcessRequest) (integration.ProcessResult, error) {
			t.Fatal("mapped listener processed an unverified connection")
			return integration.ProcessResult{}, nil
		}),
	))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mappedService.Submit(context.Background(), ConnectionIdentity{}, testHL7("CTRL1")); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("mapped listener fell back to the compatibility principal: %v", err)
	}
}

func TestServerRejectsUnmappedCertificateBeforeAnyFrame(t *testing.T) {
	authority := newIdentityAuthority(t)
	source := mappedTLSSource(t, []ClientIdentity{
		{Subject: "svc-sender-a", URISAN: "spiffe://hospital-a/mllp/sender-a", Grants: []string{SubmitRole}},
	})
	binding := testBinding(source)
	binding.SecretBindings = []integration.SecretBinding{{Name: "mllp-cert"}, {Name: "mllp-key"}, {Name: "mllp-client-ca"}}
	var processed atomic.Int64
	var subjects []string
	var subjectsMu sync.Mutex
	server, err := NewServer(ServerConfig{
		Service: testServiceConfig(source,
			resolverFunc(func(context.Context, string, string) (lifecycle.RunnableBinding, error) { return binding, nil }),
			processorFunc(func(_ context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
				processed.Add(1)
				subjectsMu.Lock()
				subjects = append(subjects, request.Security.Principal.ID)
				subjectsMu.Unlock()
				return acceptedResult(request), nil
			}),
		),
		TLSMaterial: authority.material,
	})
	if err != nil {
		t.Fatal(err)
	}
	address, stop := serveTestServer(t, server)
	defer stop()

	mapped, err := tls.Dial("tcp", address, authority.clientConfig(t, "spiffe://hospital-a/mllp/sender-a"))
	if err != nil {
		t.Fatalf("mapped client handshake: %v", err)
	}
	framed, _ := framePayload(testHL7("CTRL1"), source.Framing)
	if _, err := mapped.Write(framed); err != nil {
		t.Fatal(err)
	}
	acknowledgement, err := readFrame(bufio.NewReader(mapped), source.Framing, source.MaxMessageBytes)
	if err != nil {
		t.Fatal(err)
	}
	if code := acknowledgementCodeFromPayload(t, acknowledgement); code != "AA" {
		t.Fatalf("mapped ACK = %s", code)
	}
	_ = mapped.Close()

	unmapped, err := tls.Dial("tcp", address, authority.clientConfig(t, "spiffe://hospital-a/mllp/sender-z"))
	if err == nil {
		_, _ = unmapped.Write(framed)
		_ = unmapped.SetReadDeadline(time.Now().Add(time.Second))
		if _, readErr := unmapped.Read(make([]byte, 1)); readErr == nil {
			t.Fatal("unmapped CA-valid certificate received an MLLP response")
		}
		_ = unmapped.Close()
	}
	if processed.Load() != 1 {
		t.Fatalf("processor calls = %d, want 1", processed.Load())
	}
	subjectsMu.Lock()
	defer subjectsMu.Unlock()
	if len(subjects) != 1 || subjects[0] != "svc-sender-a" {
		t.Fatalf("captured subjects = %v", subjects)
	}
}

// identityAuthority issues a CA plus client certificates carrying URI SANs.
type identityAuthority struct {
	material      TLSMaterial
	rootCAs       *x509.CertPool
	caCertificate *x509.Certificate
	caPrivate     ed25519.PrivateKey
}

func newIdentityAuthority(t *testing.T) *identityAuthority {
	t.Helper()
	now := time.Now().UTC()
	caPublic, caPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "MLLP identity CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPublic, caPrivate)
	if err != nil {
		t.Fatal(err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	authority := &identityAuthority{caCertificate: caCertificate, caPrivate: caPrivate}
	serverPEM, serverKeyPEM := authority.issue(t, &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: []string{"localhost"},
	})
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	authority.material = TLSMaterial{CertificatePEM: serverPEM, PrivateKeyPEM: serverKeyPEM, ClientCAPEM: caPEM}
	authority.rootCAs = x509.NewCertPool()
	authority.rootCAs.AppendCertsFromPEM(caPEM)
	return authority
}

func (a *identityAuthority) issue(t *testing.T, template *x509.Certificate) ([]byte, []byte) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, a.caCertificate, publicKey, a.caPrivate)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
}

// clientConfig issues a fresh CA-valid client certificate for one URI SAN.
func (a *identityAuthority) clientConfig(t *testing.T, uris ...string) *tls.Config {
	t.Helper()
	now := time.Now().UTC()
	parsed := make([]*url.URL, 0, len(uris))
	for _, raw := range uris {
		value, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		parsed = append(parsed, value)
	}
	certificatePEM, keyPEM := a.issue(t, &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()), Subject: pkix.Name{CommonName: "sender"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs: parsed,
	})
	certificate, err := tls.X509KeyPair(certificatePEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS13, ServerName: "localhost", RootCAs: a.rootCAs,
		Certificates: []tls.Certificate{certificate},
	}
}

func mappedTLSSource(t *testing.T, identities []ClientIdentity) SourceRevision {
	t.Helper()
	revision, err := NewSourceRevision(identitySourceInput(TLSPolicy{
		Mode: TLSModeMutual, ServerCertificateBinding: "mllp-cert",
		ServerPrivateKeyBinding: "mllp-key", ClientCABinding: "mllp-client-ca",
	}, identities))
	if err != nil {
		t.Fatalf("create mapped source: %v", err)
	}
	return revision
}

func identitySourceInput(tlsPolicy TLSPolicy, identities []ClientIdentity) SourceRevisionInput {
	return SourceRevisionInput{
		ArtifactID: "mllp-source", RevisionID: "source-v1", SourceID: "hospital-a",
		ListenAddress: "127.0.0.1:2575", Encoding: "utf-8",
		Framing:          FramingPolicy{StartByte: StandardStartByte, EndByte: StandardEndByte, TrailerByte: StandardTrailerByte},
		Timeouts:         TimeoutPolicy{ReadSeconds: 2, WriteSeconds: 2, IdleSeconds: 3, ProcessSeconds: 2},
		TLS:              tlsPolicy,
		Clients:          ClientPolicy{AllowedCIDRs: []string{"127.0.0.0/8"}, Identities: identities},
		Acknowledgements: AcknowledgementPolicy{Mode: AcknowledgementModeApplication, IncludeErrorSegment: true},
		MaxMessageBytes:  4096, MaxConnections: 4,
	}
}

func certificateSPKIPin(certificate *x509.Certificate) string {
	return spkiPin(certificate)
}

func identityCertificate(t *testing.T, uris ...string) *x509.Certificate {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	parsed := make([]*url.URL, 0, len(uris))
	for _, raw := range uris {
		value, parseErr := url.Parse(raw)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		parsed = append(parsed, value)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()), Subject: pkix.Name{CommonName: "sender"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs: parsed,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return certificate
}
