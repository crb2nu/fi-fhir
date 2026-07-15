package mllp

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestServerDoesNotAcknowledgeBeforeDurableProcessorReturns(t *testing.T) {
	source := testSource(t)
	binding := testBinding(source)
	entered := make(chan struct{})
	release := make(chan struct{})
	server := newTestServer(t, source, binding, processorFunc(func(_ context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
		close(entered)
		<-release
		return acceptedResult(request), nil
	}))
	address, stop := serveTestServer(t, server)
	defer stop()

	connection, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	framed, _ := framePayload(testHL7("CTRL1"), source.Framing)
	if _, err := connection.Write(framed); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := connection.SetReadDeadline(time.Now().Add(75 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(connection).Peek(1); err == nil {
		t.Fatal("received positive ACK before durable processing returned")
	} else if networkError, ok := err.(net.Error); !ok || !networkError.Timeout() {
		t.Fatalf("expected read timeout, got %v", err)
	}
	close(release)
	if err := connection.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	payload, err := readFrame(bufio.NewReader(connection), source.Framing, source.MaxMessageBytes)
	if err != nil {
		t.Fatal(err)
	}
	if code := acknowledgementCodeFromPayload(t, payload); code != "AA" {
		t.Fatalf("got %s", code)
	}
}

func TestServerHandlesFragmentedAndSequentialFrames(t *testing.T) {
	source := testSource(t)
	binding := testBinding(source)
	server := newTestServer(t, source, binding, processorFunc(func(_ context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
		return acceptedResult(request), nil
	}))
	address, stop := serveTestServer(t, server)
	defer stop()
	connection, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	reader := bufio.NewReader(connection)
	for _, controlID := range []string{"CTRL1", "CTRL2"} {
		framed, _ := framePayload(testHL7(controlID), source.Framing)
		for _, value := range framed {
			if _, err := connection.Write([]byte{value}); err != nil {
				t.Fatal(err)
			}
		}
		payload, err := readFrame(reader, source.Framing, source.MaxMessageBytes)
		if err != nil {
			t.Fatal(err)
		}
		if code := acknowledgementCodeFromPayload(t, payload); code != "AA" {
			t.Fatalf("got %s", code)
		}
		if !strings.Contains(string(payload), controlID) {
			t.Fatalf("ACK did not safely correlate %s: %q", controlID, payload)
		}
	}
}

func TestServerMapsFailuresToNegativeAcknowledgements(t *testing.T) {
	source := testSource(t)
	binding := testBinding(source)
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"invalid", processor.ErrInvalidSourceMessage, "AR"},
		{"conflict", processor.ErrIdempotencyConflict, "AR"},
		{"unavailable", lifecycle.ErrNotRunnable, "AE"},
		{"retryable", errors.New("database unavailable"), "AE"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := newTestServer(t, source, binding, processorFunc(func(context.Context, integration.ProcessRequest) (integration.ProcessResult, error) {
				return integration.ProcessResult{}, test.err
			}))
			address, stop := serveTestServer(t, server)
			defer stop()
			connection, err := net.Dial("tcp", address)
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			framed, _ := framePayload(testHL7("CTRL1"), source.Framing)
			_, _ = connection.Write(framed)
			payload, err := readFrame(bufio.NewReader(connection), source.Framing, source.MaxMessageBytes)
			if err != nil {
				t.Fatal(err)
			}
			if code := acknowledgementCodeFromPayload(t, payload); code != test.want {
				t.Fatalf("got %s, want %s", code, test.want)
			}
		})
	}
}

func TestServerRejectsDisallowedClientsAndBoundsConnections(t *testing.T) {
	source := testSource(t)
	source.Clients.AllowedCIDRs = []string{"10.0.0.0/8"}
	digest, _ := source.semanticDigest()
	source.Digest = digest
	binding := testBinding(source)
	server := newTestServer(t, source, binding, processorFunc(func(_ context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
		return acceptedResult(request), nil
	}))
	address, stop := serveTestServer(t, server)
	defer stop()
	connection, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	framed, _ := framePayload(testHL7("CTRL1"), source.Framing)
	_, _ = connection.Write(framed)
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := connection.Read(make([]byte, 1)); err == nil {
		t.Fatal("disallowed client received a response")
	}
}

func TestServerCancellationClosesIdleConnections(t *testing.T) {
	source := testSource(t)
	binding := testBinding(source)
	server := newTestServer(t, source, binding, processorFunc(func(_ context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
		return acceptedResult(request), nil
	}))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener) }()
	connection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not close an idle connection during shutdown")
	}
}

func TestServerRequiresVerifiedTLS13ClientCertificate(t *testing.T) {
	source := testSource(t)
	source.TLS = TLSPolicy{
		Mode: TLSModeMutual, ServerCertificateBinding: "mllp-cert",
		ServerPrivateKeyBinding: "mllp-key", ClientCABinding: "mllp-client-ca",
	}
	digest, _ := source.semanticDigest()
	source.Digest = digest
	binding := testBinding(source)
	binding.SecretBindings = []integration.SecretBinding{{Name: "mllp-cert"}, {Name: "mllp-key"}, {Name: "mllp-client-ca"}}
	material, clientConfig := testMutualTLS(t)
	var processed atomic.Int64
	server, err := NewServer(ServerConfig{
		Service: testServiceConfig(source,
			resolverFunc(func(context.Context, string, string) (lifecycle.RunnableBinding, error) { return binding, nil }),
			processorFunc(func(_ context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
				processed.Add(1)
				return acceptedResult(request), nil
			}),
		),
		TLSMaterial: material,
	})
	if err != nil {
		t.Fatal(err)
	}
	address, stop := serveTestServer(t, server)
	defer stop()

	connection, err := tls.Dial("tcp", address, clientConfig)
	if err != nil {
		t.Fatalf("verified client handshake: %v", err)
	}
	framed, _ := framePayload(testHL7("CTRL1"), source.Framing)
	if _, err := connection.Write(framed); err != nil {
		t.Fatal(err)
	}
	acknowledgement, err := readFrame(bufio.NewReader(connection), source.Framing, source.MaxMessageBytes)
	if err != nil {
		t.Fatal(err)
	}
	if code := acknowledgementCodeFromPayload(t, acknowledgement); code != "AA" {
		t.Fatalf("mutual TLS ACK = %s", code)
	}
	if connection.ConnectionState().Version != tls.VersionTLS13 {
		t.Fatalf("TLS version = %x", connection.ConnectionState().Version)
	}
	_ = connection.Close()

	withoutCertificate := clientConfig.Clone()
	withoutCertificate.Certificates = nil
	unauthenticated, err := tls.Dial("tcp", address, withoutCertificate)
	if err == nil {
		_, _ = unauthenticated.Write(framed)
		_ = unauthenticated.SetReadDeadline(time.Now().Add(time.Second))
		_, readErr := unauthenticated.Read(make([]byte, 1))
		_ = unauthenticated.Close()
		if readErr == nil {
			t.Fatal("client without certificate received an MLLP response")
		}
	}
	if processed.Load() != 1 {
		t.Fatalf("processor calls = %d, want 1", processed.Load())
	}
}

func newTestServer(t *testing.T, source SourceRevision, binding lifecycle.RunnableBinding, messageProcessor MessageProcessor) *Server {
	t.Helper()
	server, err := NewServer(ServerConfig{Service: testServiceConfig(source,
		resolverFunc(func(context.Context, string, string) (lifecycle.RunnableBinding, error) { return binding, nil }),
		messageProcessor,
	)})
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func serveTestServer(t *testing.T, server *Server) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener) }()
	return listener.Addr().String(), func() {
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("serve: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("MLLP server did not stop")
		}
	}
}

func testMutualTLS(t *testing.T) (TLSMaterial, *tls.Config) {
	t.Helper()
	now := time.Now().UTC()
	caPublic, caPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "MLLP test CA"},
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
	issue := func(serial int64, name string, usage x509.ExtKeyUsage) ([]byte, []byte) {
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name},
			NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
		}
		if usage == x509.ExtKeyUsageServerAuth {
			template.DNSNames = []string{"localhost"}
		}
		certificateDER, err := x509.CreateCertificate(rand.Reader, template, caCertificate, publicKey, caPrivate)
		if err != nil {
			t.Fatal(err)
		}
		privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}),
			pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	}
	serverCertificate, serverKey := issue(2, "localhost", x509.ExtKeyUsageServerAuth)
	clientCertificatePEM, clientKeyPEM := issue(3, "sender", x509.ExtKeyUsageClientAuth)
	clientCertificate, err := tls.X509KeyPair(clientCertificatePEM, clientKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	rootCAs := x509.NewCertPool()
	rootCAs.AppendCertsFromPEM(caPEM)
	return TLSMaterial{CertificatePEM: serverCertificate, PrivateKeyPEM: serverKey, ClientCAPEM: caPEM}, &tls.Config{
		MinVersion: tls.VersionTLS13, ServerName: "localhost", RootCAs: rootCAs,
		Certificates: []tls.Certificate{clientCertificate},
	}
}
