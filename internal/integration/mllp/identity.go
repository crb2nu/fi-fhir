package mllp

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strings"
)

// Authentication method names recorded on the verified MLLP service principal.
const (
	// AuthMethodAllowlist attributes a plaintext listener to its CIDR allowlist.
	AuthMethodAllowlist = "mllp-allowlist"
	// AuthMethodMutualTLS attributes a mutual-TLS listener without identity mapping.
	AuthMethodMutualTLS = "mllp-mtls"
	// AuthMethodCertificateIdentity attributes one mapped client certificate.
	AuthMethodCertificateIdentity = "mllp-mtls-identity"
)

const (
	maxClientIdentities      = 128
	maxClientIdentityGrants  = 16
	spkiPinPrefix            = "sha256:"
	spkiPinHexLength         = sha256.Size * 2
	maxClientIdentityURIByte = 512
)

var (
	// ErrClientIdentityUnmapped rejects a CA-valid certificate that no configured
	// identity claims, or that more than one configured identity claims.
	ErrClientIdentityUnmapped = errors.New("MLLP client certificate is not mapped to a configured service identity")
	// ErrClientIdentityUnavailable rejects a connection that presented no verified
	// peer certificate while identity mapping is required.
	ErrClientIdentityUnavailable = errors.New("MLLP connection presented no verified client certificate")
)

// ClientIdentity maps one verified client certificate to one canonical service
// subject. Matching criteria are authority-scoped: a URI subject alternative
// name, a subject public key info pin, or both. Common names are deliberately
// not accepted as identity.
type ClientIdentity struct {
	Subject    string   `json:"subject"`
	URISAN     string   `json:"uri_san,omitempty"`
	SPKISHA256 string   `json:"spki_sha256,omitempty"`
	Grants     []string `json:"grants,omitempty"`
}

// ConnectionIdentity is the verified per-connection service identity derived
// from the peer certificate. It is never influenced by message content.
type ConnectionIdentity struct {
	Subject    string
	AuthMethod string
	Grants     []string
}

// zero reports whether no verified certificate identity was derived.
func (i ConnectionIdentity) zero() bool {
	return i.Subject == "" && i.AuthMethod == "" && len(i.Grants) == 0
}

// IdentityMappingEnabled reports whether this listener binds connections to
// mapped certificate identities. Mapping is all-or-nothing per listener.
func (p ClientPolicy) IdentityMappingEnabled() bool {
	return len(p.Identities) > 0
}

// ResolveClientIdentity maps one verified peer certificate chain to exactly one
// configured identity. Zero matches and ambiguous multiple matches both fail.
func (p ClientPolicy) ResolveClientIdentity(peers []*x509.Certificate) (ConnectionIdentity, error) {
	if !p.IdentityMappingEnabled() {
		return ConnectionIdentity{}, ErrClientIdentityUnmapped
	}
	if len(peers) == 0 || peers[0] == nil {
		return ConnectionIdentity{}, ErrClientIdentityUnavailable
	}
	leaf := peers[0]
	pin := spkiPin(leaf)
	uris := make(map[string]struct{}, len(leaf.URIs))
	for _, uri := range leaf.URIs {
		if uri == nil {
			continue
		}
		uris[uri.String()] = struct{}{}
	}
	matched := ConnectionIdentity{}
	matches := 0
	for _, identity := range p.Identities {
		if identity.URISAN != "" {
			if _, present := uris[identity.URISAN]; !present {
				continue
			}
		}
		if identity.SPKISHA256 != "" && identity.SPKISHA256 != pin {
			continue
		}
		matches++
		matched = ConnectionIdentity{
			Subject:    identity.Subject,
			AuthMethod: AuthMethodCertificateIdentity,
			Grants:     append([]string(nil), identity.Grants...),
		}
	}
	if matches != 1 {
		return ConnectionIdentity{}, ErrClientIdentityUnmapped
	}
	return matched, nil
}

func spkiPin(certificate *x509.Certificate) string {
	digest := sha256.Sum256(certificate.RawSubjectPublicKeyInfo)
	return spkiPinPrefix + hex.EncodeToString(digest[:])
}

func cloneClientIdentities(values []ClientIdentity) []ClientIdentity {
	if len(values) == 0 {
		return nil
	}
	clone := make([]ClientIdentity, 0, len(values))
	for _, value := range values {
		value.Grants = append([]string(nil), value.Grants...)
		clone = append(clone, value)
	}
	return clone
}

func canonicalClientIdentities(values []ClientIdentity) []ClientIdentity {
	clone := cloneClientIdentities(values)
	for index := range clone {
		grants := clone[index].Grants
		sort.Strings(grants)
		clone[index].Grants = grants
	}
	sort.Slice(clone, func(left, right int) bool {
		return clone[left].Subject < clone[right].Subject
	})
	return clone
}

func validateClientIdentities(values []ClientIdentity) error {
	if len(values) == 0 {
		return nil
	}
	if len(values) > maxClientIdentities {
		return ErrInvalidSourceRevision
	}
	subjects := make(map[string]struct{}, len(values))
	sans := make(map[string]struct{}, len(values))
	pins := make(map[string]struct{}, len(values))
	for _, identity := range values {
		if !validIdentity(identity.Subject) {
			return ErrInvalidSourceRevision
		}
		if _, duplicate := subjects[identity.Subject]; duplicate {
			return ErrInvalidSourceRevision
		}
		subjects[identity.Subject] = struct{}{}
		if identity.URISAN == "" && identity.SPKISHA256 == "" {
			return ErrInvalidSourceRevision
		}
		if identity.URISAN != "" {
			if err := validateURISAN(identity.URISAN); err != nil {
				return err
			}
			if _, duplicate := sans[identity.URISAN]; duplicate {
				return ErrInvalidSourceRevision
			}
			sans[identity.URISAN] = struct{}{}
		}
		if identity.SPKISHA256 != "" {
			if !validSPKIPin(identity.SPKISHA256) {
				return ErrInvalidSourceRevision
			}
			if _, duplicate := pins[identity.SPKISHA256]; duplicate {
				return ErrInvalidSourceRevision
			}
			pins[identity.SPKISHA256] = struct{}{}
		}
		if err := validateClientGrants(identity.Grants); err != nil {
			return err
		}
	}
	return nil
}

func validateClientGrants(grants []string) error {
	if len(grants) > maxClientIdentityGrants {
		return ErrInvalidSourceRevision
	}
	seen := make(map[string]struct{}, len(grants))
	for _, grant := range grants {
		if !validIdentity(grant) {
			return ErrInvalidSourceRevision
		}
		if _, duplicate := seen[grant]; duplicate {
			return ErrInvalidSourceRevision
		}
		seen[grant] = struct{}{}
	}
	return nil
}

func validateURISAN(value string) error {
	if !validIdentity(value) || len(value) > maxClientIdentityURIByte {
		return ErrInvalidSourceRevision
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" {
		return ErrInvalidSourceRevision
	}
	if parsed.Host == "" && parsed.Opaque == "" && parsed.Path == "" {
		return ErrInvalidSourceRevision
	}
	if parsed.String() != value {
		return ErrInvalidSourceRevision
	}
	return nil
}

func validSPKIPin(value string) bool {
	hexDigits, found := strings.CutPrefix(value, spkiPinPrefix)
	if !found || len(hexDigits) != spkiPinHexLength {
		return false
	}
	for index := 0; index < len(hexDigits); index++ {
		character := hexDigits[index]
		if character >= '0' && character <= '9' || character >= 'a' && character <= 'f' {
			continue
		}
		return false
	}
	return true
}
