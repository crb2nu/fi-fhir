package ingest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

// AuthMode defines the type of request authentication.
type AuthMode string

const (
	// AuthNone disables webhook authentication (default).
	AuthNone AuthMode = "none"
	// AuthHMAC validates the request body against an HMAC-SHA256 signature header.
	AuthHMAC AuthMode = "hmac"
	// AuthBearer validates a static Bearer token in the Authorization header.
	AuthBearer AuthMode = "bearer"
)

// AuthConfig defines how inbound webhooks are authenticated.
type AuthConfig struct {
	// Mode selects the auth strategy: "none", "hmac", or "bearer".
	Mode AuthMode
	// Secret is the shared secret: HMAC key for AuthHMAC, expected token for AuthBearer.
	Secret string
	// HeaderName overrides the signature header for HMAC mode (default: "X-Signature-256").
	HeaderName string
}

// signatureHeader returns the header name used for HMAC signatures.
func (c *AuthConfig) signatureHeader() string {
	if c.HeaderName != "" {
		return c.HeaderName
	}
	return "X-Signature-256"
}

// VerifyRequest checks the request against the configured auth strategy.
// For HMAC mode, body must be the raw request body bytes.
// Returns nil on success or an error describing the rejection reason.
func (c *AuthConfig) VerifyRequest(r *http.Request, body []byte) error {
	if c == nil || c.Mode == AuthNone || c.Mode == "" {
		return nil
	}

	switch c.Mode {
	case AuthHMAC:
		return c.verifyHMAC(r, body)
	case AuthBearer:
		return c.verifyBearer(r)
	default:
		return fmt.Errorf("unsupported auth mode: %q", c.Mode)
	}
}

// verifyHMAC validates the X-Signature-256 header using HMAC-SHA256.
// Expects header value in the format "sha256=<hex-encoded-signature>".
func (c *AuthConfig) verifyHMAC(r *http.Request, body []byte) error {
	sigHeader := r.Header.Get(c.signatureHeader())
	if sigHeader == "" {
		return fmt.Errorf("missing signature header %q", c.signatureHeader())
	}

	// Strip optional "sha256=" prefix
	sig := sigHeader
	if after, ok := strings.CutPrefix(sigHeader, "sha256="); ok {
		sig = after
	}

	expectedSig, err := hex.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("invalid hex in signature header: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(c.Secret))
	mac.Write(body)
	actualSig := mac.Sum(nil)

	if !hmac.Equal(actualSig, expectedSig) {
		return fmt.Errorf("HMAC signature mismatch")
	}

	return nil
}

// verifyBearer validates the Authorization header against the expected static token.
func (c *AuthConfig) verifyBearer(r *http.Request) error {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return fmt.Errorf("missing Authorization header")
	}

	token, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok {
		return fmt.Errorf("authorization header must use Bearer scheme")
	}

	if token != c.Secret {
		return fmt.Errorf("invalid bearer token")
	}

	return nil
}
