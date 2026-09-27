package connection

import (
	"fmt"
	"net/netip"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
)

// MLLP spec bounds, restated from mllp.SourceRevision.validateSemanticFields.
const (
	mllpMaxMessageBytes   = 1 << 20
	mllpMaxConnections    = 10000
	mllpMaxReadSeconds    = 300
	mllpMaxWriteSeconds   = 60
	mllpMaxIdleSeconds    = 3600
	mllpMaxProcessSeconds = 300
	mllpMaxCIDRs          = 128
	mllpMaxIdentities     = 128
	mllpMaxIdentityGrants = 16
	mllpEncodingUTF8      = "utf-8"
)

// MLLPSpec is the `mllp` connection spec. It is mllp.SourceRevisionInput field
// for field, minus the artifact and revision identifiers the catalog assigns.
// Framing may be omitted; it defaults to the standard 11/28/13 bytes.
type MLLPSpec struct {
	SourceID         string            `json:"source_id"`
	ListenAddress    string            `json:"listen_address"`
	Encoding         string            `json:"encoding"`
	Framing          *MLLPFramingSpec  `json:"framing,omitempty"`
	Timeouts         *MLLPTimeoutsSpec `json:"timeouts"`
	TLS              *MLLPTLSSpec      `json:"tls"`
	Clients          *MLLPClientsSpec  `json:"clients"`
	Acknowledgements *MLLPAckSpec      `json:"acknowledgements"`
	MaxMessageBytes  *int64            `json:"max_message_bytes"`
	MaxConnections   *int64            `json:"max_connections"`
}

// MLLPFramingSpec overrides the MLLP frame bytes. Each omitted byte keeps its
// standard value.
type MLLPFramingSpec struct {
	StartByte   *int64 `json:"start_byte,omitempty"`
	EndByte     *int64 `json:"end_byte,omitempty"`
	TrailerByte *int64 `json:"trailer_byte,omitempty"`
}

// MLLPTimeoutsSpec is mllp.TimeoutPolicy.
type MLLPTimeoutsSpec struct {
	ReadSeconds    *int64 `json:"read_seconds"`
	WriteSeconds   *int64 `json:"write_seconds"`
	IdleSeconds    *int64 `json:"idle_seconds"`
	ProcessSeconds *int64 `json:"process_seconds"`
}

// MLLPTLSSpec is mllp.TLSPolicy: a mode and, for mutual TLS, three binding names.
type MLLPTLSSpec struct {
	Mode                     string `json:"mode"`
	ServerCertificateBinding string `json:"server_certificate_binding,omitempty"`
	ServerPrivateKeyBinding  string `json:"server_private_key_binding,omitempty"`
	ClientCABinding          string `json:"client_ca_binding,omitempty"`
}

// MLLPClientsSpec is mllp.ClientPolicy.
type MLLPClientsSpec struct {
	AllowedCIDRs []string           `json:"allowed_cidrs"`
	Identities   []MLLPIdentitySpec `json:"identities,omitempty"`
}

// MLLPIdentitySpec is mllp.ClientIdentity.
type MLLPIdentitySpec struct {
	Subject    string   `json:"subject"`
	URISAN     string   `json:"uri_san,omitempty"`
	SPKISHA256 string   `json:"spki_sha256,omitempty"`
	Grants     []string `json:"grants,omitempty"`
}

// MLLPAckSpec is mllp.AcknowledgementPolicy. include_error_segment defaults
// to false.
type MLLPAckSpec struct {
	Mode                string `json:"mode"`
	IncludeErrorSegment *bool  `json:"include_error_segment,omitempty"`
}

func (s *MLLPSpec) check(c *checker) {
	c.identity("source_id", s.SourceID)
	switch {
	case s.ListenAddress == "":
		c.add(CodeRequired, "listen_address", "is required")
	case !validListenAddress(s.ListenAddress):
		c.add(CodeInvalidAddress, "listen_address", "must be host:port with a port between 1 and 65535")
	}
	c.enum("encoding", s.Encoding, mllpEncodingUTF8)
	s.checkFraming(c)
	s.checkTimeouts(c)
	tlsMode := s.checkTLS(c)
	s.checkClients(c, tlsMode)
	if s.Acknowledgements == nil {
		c.add(CodeRequired, "acknowledgements", "is required")
	} else {
		c.enum("acknowledgements.mode", s.Acknowledgements.Mode,
			string(mllp.AcknowledgementModeApplication), string(mllp.AcknowledgementModeCommit))
	}
	c.intRange("max_message_bytes", s.MaxMessageBytes, 1, mllpMaxMessageBytes)
	c.intRange("max_connections", s.MaxConnections, 1, mllpMaxConnections)
}

func (s *MLLPSpec) checkFraming(c *checker) {
	if s.Framing == nil {
		return
	}
	fields := []struct {
		path  string
		value *int64
	}{
		{"framing.start_byte", s.Framing.StartByte},
		{"framing.end_byte", s.Framing.EndByte},
		{"framing.trailer_byte", s.Framing.TrailerByte},
	}
	framing := s.framing()
	effective := []uint8{framing.StartByte, framing.EndByte, framing.TrailerByte}
	seen := make(map[uint8]string, len(fields))
	for index, field := range fields {
		if field.value != nil && (*field.value < 1 || *field.value > 255) {
			c.add(CodeOutOfRange, field.path, "must be between 1 and 255")
			continue
		}
		if first, duplicate := seen[effective[index]]; duplicate {
			c.add(CodeConflict, field.path, "must differ from "+first)
			continue
		}
		seen[effective[index]] = field.path
	}
}

func (s *MLLPSpec) checkTimeouts(c *checker) {
	if s.Timeouts == nil {
		c.add(CodeRequired, "timeouts", "is required")
		return
	}
	readOK := c.intRange("timeouts.read_seconds", s.Timeouts.ReadSeconds, 1, mllpMaxReadSeconds)
	c.intRange("timeouts.write_seconds", s.Timeouts.WriteSeconds, 1, mllpMaxWriteSeconds)
	idleLow := int64(1)
	if readOK {
		idleLow = *s.Timeouts.ReadSeconds
	}
	if s.Timeouts.IdleSeconds == nil {
		c.add(CodeRequired, "timeouts.idle_seconds", "is required")
	} else if *s.Timeouts.IdleSeconds < idleLow || *s.Timeouts.IdleSeconds > mllpMaxIdleSeconds {
		c.add(CodeOutOfRange, "timeouts.idle_seconds",
			fmt.Sprintf("must be at least read_seconds (%d) and at most %d", idleLow, mllpMaxIdleSeconds))
	}
	c.intRange("timeouts.process_seconds", s.Timeouts.ProcessSeconds, 1, mllpMaxProcessSeconds)
}

// checkTLS returns the mode when it is valid, "" otherwise.
func (s *MLLPSpec) checkTLS(c *checker) mllp.TLSMode {
	if s.TLS == nil {
		c.add(CodeRequired, "tls", "is required")
		return ""
	}
	if !c.enum("tls.mode", s.TLS.Mode, string(mllp.TLSModeDisabled), string(mllp.TLSModeMutual)) {
		return ""
	}
	bindings := []struct {
		path  string
		value string
	}{
		{"tls.server_certificate_binding", s.TLS.ServerCertificateBinding},
		{"tls.server_private_key_binding", s.TLS.ServerPrivateKeyBinding},
		{"tls.client_ca_binding", s.TLS.ClientCABinding},
	}
	mode := mllp.TLSMode(s.TLS.Mode)
	for _, binding := range bindings {
		if mode == mllp.TLSModeMutual {
			c.binding(binding.path, binding.value)
		} else if binding.value != "" {
			c.add(CodeForbidden, binding.path, "must be empty when tls.mode is disabled")
		}
	}
	return mode
}

func (s *MLLPSpec) checkClients(c *checker, tlsMode mllp.TLSMode) {
	if s.Clients == nil {
		c.add(CodeRequired, "clients", "is required")
		return
	}
	cidrs := s.Clients.AllowedCIDRs
	switch {
	case len(cidrs) == 0:
		c.add(CodeRequired, "clients.allowed_cidrs", "needs at least one CIDR")
	case len(cidrs) > mllpMaxCIDRs:
		c.add(CodeOutOfRange, "clients.allowed_cidrs", fmt.Sprintf("must hold at most %d CIDRs", mllpMaxCIDRs))
	}
	seen := make(map[netip.Prefix]struct{}, len(cidrs))
	for index, value := range cidrs {
		path := fmt.Sprintf("clients.allowed_cidrs[%d]", index)
		prefix, ok := canonicalCIDR(value)
		if !ok {
			c.add(CodeInvalidCIDR, path, "must be a canonical network prefix such as 10.0.0.0/8")
			continue
		}
		if _, duplicate := seen[prefix]; duplicate {
			c.add(CodeDuplicate, path, "is repeated")
		}
		seen[prefix] = struct{}{}
	}

	identities := s.Clients.Identities
	if len(identities) > mllpMaxIdentities {
		c.add(CodeOutOfRange, "clients.identities", fmt.Sprintf("must hold at most %d identities", mllpMaxIdentities))
	}
	if len(identities) > 0 && tlsMode != "" && tlsMode != mllp.TLSModeMutual {
		c.add(CodeConflict, "clients.identities", "client identities require tls.mode mutual")
	}
	subjects := make(map[string]struct{}, len(identities))
	sans := make(map[string]struct{}, len(identities))
	pins := make(map[string]struct{}, len(identities))
	for index, identity := range identities {
		path := fmt.Sprintf("clients.identities[%d]", index)
		if c.identity(path+".subject", identity.Subject) {
			if _, duplicate := subjects[identity.Subject]; duplicate {
				c.add(CodeDuplicate, path+".subject", "is repeated")
			}
			subjects[identity.Subject] = struct{}{}
		}
		if identity.URISAN == "" && identity.SPKISHA256 == "" {
			c.add(CodeRequired, path, "needs a uri_san, an spki_sha256, or both")
		}
		if identity.URISAN != "" {
			if !validURISAN(identity.URISAN) {
				c.add(CodeInvalidValue, path+".uri_san", "must be a canonical URI with a scheme")
			} else if _, duplicate := sans[identity.URISAN]; duplicate {
				c.add(CodeDuplicate, path+".uri_san", "is repeated")
			}
			sans[identity.URISAN] = struct{}{}
		}
		if identity.SPKISHA256 != "" {
			if !validSPKIPin(identity.SPKISHA256) {
				c.add(CodeInvalidValue, path+".spki_sha256", "must be sha256: followed by 64 lowercase hex digits")
			} else if _, duplicate := pins[identity.SPKISHA256]; duplicate {
				c.add(CodeDuplicate, path+".spki_sha256", "is repeated")
			}
			pins[identity.SPKISHA256] = struct{}{}
		}
		c.grants(path+".grants", identity.Grants, 0, mllpMaxIdentityGrants)
	}
}

// framing returns the effective frame bytes. An out-of-range override is
// truncated here; check has already refused it.
func (s *MLLPSpec) framing() mllp.FramingPolicy {
	framing := mllp.FramingPolicy{
		StartByte:   mllp.StandardStartByte,
		EndByte:     mllp.StandardEndByte,
		TrailerByte: mllp.StandardTrailerByte,
	}
	if s.Framing == nil {
		return framing
	}
	if s.Framing.StartByte != nil {
		framing.StartByte = uint8(*s.Framing.StartByte) // #nosec G115 -- range checked by checkFraming
	}
	if s.Framing.EndByte != nil {
		framing.EndByte = uint8(*s.Framing.EndByte) // #nosec G115 -- range checked by checkFraming
	}
	if s.Framing.TrailerByte != nil {
		framing.TrailerByte = uint8(*s.Framing.TrailerByte) // #nosec G115 -- range checked by checkFraming
	}
	return framing
}

// input maps the spec onto the constructor's input. Absent fields become zero
// values, which the constructor refuses; the checker has already said which.
func (s *MLLPSpec) input(artifactID, revisionID string) mllp.SourceRevisionInput {
	input := mllp.SourceRevisionInput{
		ArtifactID:      artifactID,
		RevisionID:      revisionID,
		SourceID:        s.SourceID,
		ListenAddress:   s.ListenAddress,
		Encoding:        s.Encoding,
		Framing:         s.framing(),
		MaxMessageBytes: int64Value(s.MaxMessageBytes),
		MaxConnections:  int(int64Value(s.MaxConnections)),
	}
	if s.Timeouts != nil {
		input.Timeouts = mllp.TimeoutPolicy{
			ReadSeconds:    int64Value(s.Timeouts.ReadSeconds),
			WriteSeconds:   int64Value(s.Timeouts.WriteSeconds),
			IdleSeconds:    int64Value(s.Timeouts.IdleSeconds),
			ProcessSeconds: int64Value(s.Timeouts.ProcessSeconds),
		}
	}
	if s.TLS != nil {
		input.TLS = mllp.TLSPolicy{
			Mode:                     mllp.TLSMode(s.TLS.Mode),
			ServerCertificateBinding: s.TLS.ServerCertificateBinding,
			ServerPrivateKeyBinding:  s.TLS.ServerPrivateKeyBinding,
			ClientCABinding:          s.TLS.ClientCABinding,
		}
	}
	if s.Clients != nil {
		input.Clients.AllowedCIDRs = append([]string(nil), s.Clients.AllowedCIDRs...)
		for _, identity := range s.Clients.Identities {
			input.Clients.Identities = append(input.Clients.Identities, mllp.ClientIdentity{
				Subject:    identity.Subject,
				URISAN:     identity.URISAN,
				SPKISHA256: identity.SPKISHA256,
				Grants:     append([]string(nil), identity.Grants...),
			})
		}
	}
	if s.Acknowledgements != nil {
		input.Acknowledgements = mllp.AcknowledgementPolicy{
			Mode:                mllp.AcknowledgementMode(s.Acknowledgements.Mode),
			IncludeErrorSegment: boolValue(s.Acknowledgements.IncludeErrorSegment),
		}
	}
	return input
}
