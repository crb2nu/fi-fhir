// Package mllp implements the deployed-release HL7v2 Minimal Lower Layer
// Protocol source adapter.
package mllp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	SourceSchemaVersion   = "1"
	StandardStartByte     = byte(0x0b)
	StandardEndByte       = byte(0x1c)
	StandardTrailerByte   = byte(0x0d)
	maxSourceRevisionSize = 1 << 20
	maxMLLPMessageBytes   = 1 << 20
	sourceDigestDomain    = "fi-fhir/mllp-source/v1\x00"
)

type TLSMode string

const (
	TLSModeDisabled TLSMode = "disabled"
	TLSModeMutual   TLSMode = "mutual"
)

type AcknowledgementMode string

const (
	AcknowledgementModeApplication AcknowledgementMode = "application"
	AcknowledgementModeCommit      AcknowledgementMode = "commit"
)

var (
	ErrInvalidSourceRevision = errors.New("invalid MLLP source revision")
	ErrSourceMismatch        = errors.New("MLLP source does not match deployed release")
)

type FramingPolicy struct {
	StartByte   uint8 `json:"start_byte"`
	EndByte     uint8 `json:"end_byte"`
	TrailerByte uint8 `json:"trailer_byte"`
}

type TimeoutPolicy struct {
	ReadSeconds    int64 `json:"read_seconds"`
	WriteSeconds   int64 `json:"write_seconds"`
	IdleSeconds    int64 `json:"idle_seconds"`
	ProcessSeconds int64 `json:"process_seconds"`
}

type TLSPolicy struct {
	Mode                     TLSMode `json:"mode"`
	ServerCertificateBinding string  `json:"server_certificate_binding,omitempty"`
	ServerPrivateKeyBinding  string  `json:"server_private_key_binding,omitempty"`
	ClientCABinding          string  `json:"client_ca_binding,omitempty"`
}

type ClientPolicy struct {
	AllowedCIDRs []string `json:"allowed_cidrs"`
}

type AcknowledgementPolicy struct {
	Mode                AcknowledgementMode `json:"mode"`
	IncludeErrorSegment bool                `json:"include_error_segment"`
}

type SourceRevisionInput struct {
	ArtifactID       string
	RevisionID       string
	SourceID         string
	ListenAddress    string
	Encoding         string
	Framing          FramingPolicy
	Timeouts         TimeoutPolicy
	TLS              TLSPolicy
	Clients          ClientPolicy
	Acknowledgements AcknowledgementPolicy
	MaxMessageBytes  int64
	MaxConnections   int
}

// SourceRevision is an immutable, content-addressed MLLP listener contract.
// Secret fields name bindings in the integration revision; values remain
// outside the document.
type SourceRevision struct {
	SchemaVersion    string                `json:"schema_version"`
	ArtifactID       string                `json:"artifact_id"`
	RevisionID       string                `json:"revision_id"`
	SourceID         string                `json:"source_id"`
	ListenAddress    string                `json:"listen_address"`
	Encoding         string                `json:"encoding"`
	Framing          FramingPolicy         `json:"framing"`
	Timeouts         TimeoutPolicy         `json:"timeouts"`
	TLS              TLSPolicy             `json:"tls"`
	Clients          ClientPolicy          `json:"clients"`
	Acknowledgements AcknowledgementPolicy `json:"acknowledgements"`
	MaxMessageBytes  int64                 `json:"max_message_bytes"`
	MaxConnections   int                   `json:"max_connections"`
	Digest           string                `json:"digest"`
}

func NewSourceRevision(input SourceRevisionInput) (SourceRevision, error) {
	revision := SourceRevision{
		SchemaVersion:    SourceSchemaVersion,
		ArtifactID:       input.ArtifactID,
		RevisionID:       input.RevisionID,
		SourceID:         input.SourceID,
		ListenAddress:    input.ListenAddress,
		Encoding:         input.Encoding,
		Framing:          input.Framing,
		Timeouts:         input.Timeouts,
		TLS:              input.TLS,
		Clients:          ClientPolicy{AllowedCIDRs: append([]string(nil), input.Clients.AllowedCIDRs...)},
		Acknowledgements: input.Acknowledgements,
		MaxMessageBytes:  input.MaxMessageBytes,
		MaxConnections:   input.MaxConnections,
	}
	if err := revision.validateSemanticFields(); err != nil {
		return SourceRevision{}, err
	}
	digest, err := revision.semanticDigest()
	if err != nil {
		return SourceRevision{}, fmt.Errorf("%w: compute digest", ErrInvalidSourceRevision)
	}
	revision.Digest = digest
	return revision, nil
}

func DecodeSourceRevision(reader io.Reader) (SourceRevision, error) {
	if reader == nil {
		return SourceRevision{}, ErrInvalidSourceRevision
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxSourceRevisionSize+1))
	if err != nil || len(raw) == 0 || len(raw) > maxSourceRevisionSize {
		return SourceRevision{}, ErrInvalidSourceRevision
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return SourceRevision{}, ErrInvalidSourceRevision
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var revision SourceRevision
	if err := decoder.Decode(&revision); err != nil {
		return SourceRevision{}, ErrInvalidSourceRevision
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return SourceRevision{}, ErrInvalidSourceRevision
	}
	if err := revision.Validate(); err != nil {
		return SourceRevision{}, err
	}
	return revision, nil
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var walk func(string) error
	walk = func(path string) error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return ErrInvalidSourceRevision
				}
				if _, duplicate := seen[key]; duplicate {
					return ErrInvalidSourceRevision
				}
				seen[key] = struct{}{}
				if err := walk(path + "." + key); err != nil {
					return err
				}
			}
			_, err := decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(path); err != nil {
					return err
				}
			}
			_, err := decoder.Token()
			return err
		default:
			return ErrInvalidSourceRevision
		}
	}
	return walk("$")
}

func (r SourceRevision) Validate() error {
	if err := r.validateSemanticFields(); err != nil {
		return err
	}
	expected, err := r.semanticDigest()
	if err != nil || r.Digest != expected {
		return ErrInvalidSourceRevision
	}
	return nil
}

func (r SourceRevision) Reference() integration.ArtifactRevisionRef {
	return integration.ArtifactRevisionRef{
		ArtifactID: r.ArtifactID,
		RevisionID: r.RevisionID,
		Digest:     r.Digest,
	}
}

func (r SourceRevision) ValidateAgainst(binding lifecycle.RunnableBinding) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if binding.SourceRevision != r.Reference() || binding.SourceID != r.SourceID ||
		binding.Format != events.FormatHL7v2 || binding.IntegrationRevision.ArtifactID == "" {
		return ErrSourceMismatch
	}
	if err := binding.Deployment.Validate(); err != nil {
		return ErrSourceMismatch
	}
	if r.TLS.Mode == TLSModeMutual {
		for _, name := range []string{
			r.TLS.ServerCertificateBinding,
			r.TLS.ServerPrivateKeyBinding,
			r.TLS.ClientCABinding,
		} {
			if !hasSecretBinding(binding.SecretBindings, name) {
				return ErrSourceMismatch
			}
		}
	}
	return nil
}

func (r SourceRevision) validateSemanticFields() error {
	if r.SchemaVersion != SourceSchemaVersion || !validIdentity(r.ArtifactID) ||
		!validIdentity(r.RevisionID) || !validIdentity(r.SourceID) {
		return ErrInvalidSourceRevision
	}
	if !validListenAddress(r.ListenAddress) || r.Encoding != "utf-8" {
		return ErrInvalidSourceRevision
	}
	if err := validateFraming(r.Framing); err != nil {
		return err
	}
	if r.Timeouts.ReadSeconds < 1 || r.Timeouts.ReadSeconds > 300 ||
		r.Timeouts.WriteSeconds < 1 || r.Timeouts.WriteSeconds > 60 ||
		r.Timeouts.IdleSeconds < r.Timeouts.ReadSeconds || r.Timeouts.IdleSeconds > 3600 ||
		r.Timeouts.ProcessSeconds < 1 || r.Timeouts.ProcessSeconds > 300 {
		return ErrInvalidSourceRevision
	}
	if err := validateTLS(r.TLS); err != nil {
		return err
	}
	if err := validateClientCIDRs(r.Clients.AllowedCIDRs); err != nil {
		return err
	}
	if r.Acknowledgements.Mode != AcknowledgementModeApplication &&
		r.Acknowledgements.Mode != AcknowledgementModeCommit {
		return ErrInvalidSourceRevision
	}
	if r.MaxMessageBytes < 1 || r.MaxMessageBytes > maxMLLPMessageBytes ||
		r.MaxConnections < 1 || r.MaxConnections > 10000 {
		return ErrInvalidSourceRevision
	}
	return nil
}

func validateFraming(policy FramingPolicy) error {
	values := []uint8{policy.StartByte, policy.EndByte, policy.TrailerByte}
	seen := make(map[uint8]struct{}, len(values))
	for _, value := range values {
		if value == 0 {
			return ErrInvalidSourceRevision
		}
		if _, duplicate := seen[value]; duplicate {
			return ErrInvalidSourceRevision
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateTLS(policy TLSPolicy) error {
	switch policy.Mode {
	case TLSModeDisabled:
		if policy.ServerCertificateBinding != "" || policy.ServerPrivateKeyBinding != "" || policy.ClientCABinding != "" {
			return ErrInvalidSourceRevision
		}
	case TLSModeMutual:
		if !validIdentity(policy.ServerCertificateBinding) ||
			!validIdentity(policy.ServerPrivateKeyBinding) ||
			!validIdentity(policy.ClientCABinding) {
			return ErrInvalidSourceRevision
		}
	default:
		return ErrInvalidSourceRevision
	}
	return nil
}

func validateClientCIDRs(values []string) error {
	if len(values) == 0 || len(values) > 128 {
		return ErrInvalidSourceRevision
	}
	seen := make(map[netip.Prefix]struct{}, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || prefix.String() != value || prefix != prefix.Masked() {
			return ErrInvalidSourceRevision
		}
		if _, duplicate := seen[prefix]; duplicate {
			return ErrInvalidSourceRevision
		}
		seen[prefix] = struct{}{}
	}
	return nil
}

func validListenAddress(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\x00") {
		return false
	}
	_, port, err := net.SplitHostPort(value)
	if err != nil {
		return false
	}
	number, err := strconv.Atoi(port)
	return err == nil && number >= 1 && number <= 65535 && strconv.Itoa(number) == port
}

func validIdentity(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func hasSecretBinding(bindings []integration.SecretBinding, name string) bool {
	for _, binding := range bindings {
		if binding.Name == name {
			return true
		}
	}
	return false
}

func (r SourceRevision) semanticDigest() (string, error) {
	cidrs := append([]string(nil), r.Clients.AllowedCIDRs...)
	sort.Strings(cidrs)
	canonical := struct {
		SchemaVersion    string                `json:"schema_version"`
		ArtifactID       string                `json:"artifact_id"`
		RevisionID       string                `json:"revision_id"`
		SourceID         string                `json:"source_id"`
		ListenAddress    string                `json:"listen_address"`
		Encoding         string                `json:"encoding"`
		Framing          FramingPolicy         `json:"framing"`
		Timeouts         TimeoutPolicy         `json:"timeouts"`
		TLS              TLSPolicy             `json:"tls"`
		Clients          ClientPolicy          `json:"clients"`
		Acknowledgements AcknowledgementPolicy `json:"acknowledgements"`
		MaxMessageBytes  int64                 `json:"max_message_bytes"`
		MaxConnections   int                   `json:"max_connections"`
	}{
		SchemaVersion: r.SchemaVersion, ArtifactID: r.ArtifactID,
		RevisionID: r.RevisionID, SourceID: r.SourceID, ListenAddress: r.ListenAddress,
		Encoding: r.Encoding, Framing: r.Framing, Timeouts: r.Timeouts, TLS: r.TLS,
		Clients: ClientPolicy{AllowedCIDRs: cidrs}, Acknowledgements: r.Acknowledgements,
		MaxMessageBytes: r.MaxMessageBytes, MaxConnections: r.MaxConnections,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(sourceDigestDomain))
	_, _ = digest.Write(encoded)
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}
