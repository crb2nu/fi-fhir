package mllp

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

type TLSMaterial struct {
	CertificatePEM []byte
	PrivateKeyPEM  []byte
	ClientCAPEM    []byte
}

type ServerConfig struct {
	Service     ServiceConfig
	TLSMaterial TLSMaterial
}

type Server struct {
	source         SourceRevision
	service        *Service
	allowedClients []netip.Prefix
	tlsConfig      *tls.Config
	now            func() time.Time
	newID          func() string
}

func NewServer(config ServerConfig) (*Server, error) {
	service, err := NewService(config.Service)
	if err != nil {
		return nil, err
	}
	allowedClients := make([]netip.Prefix, 0, len(config.Service.Source.Clients.AllowedCIDRs))
	for _, raw := range config.Service.Source.Clients.AllowedCIDRs {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			return nil, ErrUnavailable
		}
		allowedClients = append(allowedClients, prefix)
	}
	tlsConfig, err := buildTLSConfig(config.Service.Source.TLS, config.TLSMaterial)
	if err != nil {
		return nil, err
	}
	// Identity mapping is meaningless without a verified peer certificate; a
	// listener that declares identities never accepts an unverified connection.
	if config.Service.Source.Clients.IdentityMappingEnabled() && tlsConfig == nil {
		return nil, ErrUnavailable
	}
	return &Server{
		source: config.Service.Source, service: service, allowedClients: allowedClients,
		tlsConfig: tlsConfig, now: service.now, newID: service.newID,
	}, nil
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	if s == nil || ctx == nil {
		return ErrUnavailable
	}
	listener, err := net.Listen("tcp", s.source.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen for MLLP: %w", err)
	}
	return s.Serve(ctx, listener)
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if s == nil || s.service == nil || s.now == nil || s.newID == nil || ctx == nil || listener == nil {
		return ErrUnavailable
	}
	connections := make(chan struct{}, s.source.MaxConnections)
	var handlers sync.WaitGroup
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	defer handlers.Wait()

	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept MLLP connection: %w", err)
		}
		select {
		case connections <- struct{}{}:
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				defer func() { <-connections }()
				s.handleConnection(ctx, connection)
			}()
		default:
			_ = connection.Close()
		}
	}
}

func (s *Server) handleConnection(ctx context.Context, raw net.Conn) {
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = raw.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	defer func() { _ = raw.Close() }()
	if !s.clientAllowed(raw.RemoteAddr()) {
		return
	}
	connection := raw
	var identity ConnectionIdentity
	if s.tlsConfig != nil {
		tlsConnection := tls.Server(raw, s.tlsConfig.Clone())
		handshakeCtx, cancel := context.WithTimeout(ctx, time.Duration(s.source.Timeouts.ReadSeconds)*time.Second)
		err := tlsConnection.HandshakeContext(handshakeCtx)
		cancel()
		if err != nil {
			return
		}
		// A CA-valid certificate is not yet an authorized identity. Resolve the
		// mapped service subject before any frame is read, parsed, processed, or
		// durably admitted, and close the connection when it is unmapped.
		if s.source.Clients.IdentityMappingEnabled() {
			identity, err = s.source.Clients.ResolveClientIdentity(tlsConnection.ConnectionState().PeerCertificates)
			if err != nil {
				return
			}
		}
		connection = tlsConnection
	}
	reader := bufio.NewReaderSize(connection, minInt64(s.source.MaxMessageBytes+3, 64<<10))
	for {
		if err := connection.SetReadDeadline(s.now().Add(time.Duration(s.source.Timeouts.IdleSeconds) * time.Second)); err != nil {
			return
		}
		if _, err := reader.Peek(1); err != nil {
			return
		}
		if err := connection.SetReadDeadline(s.now().Add(time.Duration(s.source.Timeouts.ReadSeconds) * time.Second)); err != nil {
			return
		}
		payload, err := readFrame(reader, s.source.Framing, s.source.MaxMessageBytes)
		if err != nil {
			return
		}
		header, err := parseMessageHeader(payload, s.source.Framing)
		if err != nil {
			return
		}

		result, submitErr := s.service.Submit(ctx, identity, payload)
		outcome, code := classifyAcknowledgement(result, submitErr)
		acknowledgement, err := buildAcknowledgement(
			header,
			s.source.Acknowledgements,
			outcome,
			code,
			s.now().UTC(),
			s.newID(),
		)
		if err != nil {
			return
		}
		if err := connection.SetWriteDeadline(s.now().Add(time.Duration(s.source.Timeouts.WriteSeconds) * time.Second)); err != nil {
			return
		}
		if err := writeFrame(connection, acknowledgement, s.source.Framing); err != nil {
			return
		}
	}
}

func classifyAcknowledgement(result integration.ProcessResult, err error) (acknowledgementOutcome, string) {
	if err == nil && result.Receipt != nil && result.Receipt.Status == integration.ReceiptStatusAccepted {
		return acknowledgementAccepted, ""
	}
	switch {
	case errors.Is(err, ErrInvalidMessage):
		return acknowledgementPermanentReject, "INVALID_HL7V2_MESSAGE"
	case errors.Is(err, ErrIdempotencyConflict):
		return acknowledgementPermanentReject, "IDEMPOTENCY_CONFLICT"
	case errors.Is(err, ErrCapacityExceeded):
		return acknowledgementTransientError, "CAPACITY_EXCEEDED"
	case errors.Is(err, ErrRateExceeded):
		return acknowledgementTransientError, "RATE_EXCEEDED"
	case errors.Is(err, ErrUnavailable):
		return acknowledgementTransientError, "INTEGRATION_UNAVAILABLE"
	default:
		return acknowledgementTransientError, "SUBMISSION_UNAVAILABLE"
	}
}

func (s *Server) clientAllowed(address net.Addr) bool {
	if s == nil || address == nil {
		return false
	}
	host, _, err := net.SplitHostPort(address.String())
	if err != nil {
		return false
	}
	client, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	client = client.Unmap()
	for _, prefix := range s.allowedClients {
		if prefix.Contains(client) {
			return true
		}
	}
	return false
}

func buildTLSConfig(policy TLSPolicy, material TLSMaterial) (*tls.Config, error) {
	if policy.Mode == TLSModeDisabled {
		if len(material.CertificatePEM) != 0 || len(material.PrivateKeyPEM) != 0 || len(material.ClientCAPEM) != 0 {
			return nil, ErrUnavailable
		}
		return nil, nil
	}
	if policy.Mode != TLSModeMutual {
		return nil, ErrUnavailable
	}
	certificate, err := tls.X509KeyPair(material.CertificatePEM, material.PrivateKeyPEM)
	if err != nil {
		return nil, ErrUnavailable
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(material.ClientCAPEM) {
		return nil, ErrUnavailable
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAs,
	}, nil
}

// Service exposes the admission service so the serve process can attach an
// observation hook. The listener owns the service's lifecycle; the process owns
// its observation.
func (s *Server) Service() *Service {
	if s == nil {
		return nil
	}
	return s.service
}
