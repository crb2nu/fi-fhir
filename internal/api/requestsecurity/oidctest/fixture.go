// Package oidctest provides a local TLS OIDC issuer for transport-level tests.
package oidctest

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Fixture is an OIDC discovery and JWKS server with a rotatable RSA key.
type Fixture struct {
	server *httptest.Server

	mu                 sync.RWMutex
	kid                string
	key                *rsa.PrivateKey
	jwksURI            string
	jwksRedirectTarget string
	jwksDelay          time.Duration

	jwksRequests atomic.Int64
}

// New creates a TLS issuer with one RS256 signing key.
func New() (*Fixture, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate OIDC test key: %w", err)
	}
	fixture := &Fixture{kid: "key-1", key: key}
	fixture.server = httptest.NewTLSServer(http.HandlerFunc(fixture.serveHTTP))
	return fixture, nil
}

// Close stops the issuer.
func (f *Fixture) Close() {
	f.server.Close()
}

// IssuerURL is the exact configured issuer URL.
func (f *Fixture) IssuerURL() string {
	return f.server.URL
}

// HTTPClient returns a client that trusts the fixture's TLS certificate.
func (f *Fixture) HTTPClient() *http.Client {
	return f.server.Client()
}

// Context configures go-oidc to trust the fixture's TLS certificate.
func (f *Fixture) Context() context.Context {
	return oidc.ClientContext(context.Background(), f.HTTPClient())
}

// JWKSRequests returns the number of key-set fetches.
func (f *Fixture) JWKSRequests() int64 {
	return f.jwksRequests.Load()
}

// Rotate installs a new signing key and key ID.
func (f *Fixture) Rotate(kid string) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate rotated OIDC test key: %w", err)
	}
	f.mu.Lock()
	f.kid = kid
	f.key = key
	f.mu.Unlock()
	return nil
}

// SetJWKSURI overrides the jwks_uri advertised during discovery.
func (f *Fixture) SetJWKSURI(value string) {
	f.mu.Lock()
	f.jwksURI = value
	f.mu.Unlock()
}

// SetJWKSRedirect makes /keys-redirect redirect to the supplied target and
// advertises that TLS endpoint during discovery.
func (f *Fixture) SetJWKSRedirect(target string) {
	f.mu.Lock()
	f.jwksURI = f.IssuerURL() + "/keys-redirect"
	f.jwksRedirectTarget = target
	f.mu.Unlock()
}

// SetJWKSDelay delays key responses until the request is canceled or elapsed.
func (f *Fixture) SetJWKSDelay(delay time.Duration) {
	f.mu.Lock()
	f.jwksDelay = delay
	f.mu.Unlock()
}

// Claims returns a valid baseline claim set. Callers may override or delete fields.
func (f *Fixture) Claims() map[string]any {
	return map[string]any{
		"iss":       f.IssuerURL(),
		"sub":       "clinician-1",
		"aud":       "fi-fhir-graphql",
		"exp":       unixNow() + 3600,
		"iat":       unixNow(),
		"tenant_id": "tenant-a",
		"roles":     []string{"integration:preview"},
	}
}

// Sign creates a compact JWT. The header algorithm can intentionally differ
// from RS256 so tests can prove algorithm rejection.
func (f *Fixture) Sign(claims map[string]any, algorithm string) (string, error) {
	return f.SignWithType(claims, algorithm, "at+jwt")
}

// SignWithType creates a token with an explicit protected typ value. An empty
// value omits typ so callers can exercise token-class rejection.
func (f *Fixture) SignWithType(claims map[string]any, algorithm, tokenType string) (string, error) {
	f.mu.RLock()
	kid := f.kid
	key := f.key
	f.mu.RUnlock()
	return sign(key, kid, algorithm, tokenType, claims)
}

func (f *Fixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		f.mu.RLock()
		jwksURI := f.jwksURI
		f.mu.RUnlock()
		if jwksURI == "" {
			jwksURI = f.IssuerURL() + "/keys"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                f.IssuerURL(),
			"authorization_endpoint":                f.IssuerURL() + "/authorize",
			"token_endpoint":                        f.IssuerURL() + "/token",
			"jwks_uri":                              jwksURI,
			"id_token_signing_alg_values_supported": []string{"RS256", "RS384", "RS512"},
		})
	case "/keys-redirect":
		f.mu.RLock()
		target := f.jwksRedirectTarget
		f.mu.RUnlock()
		http.Redirect(w, r, target, http.StatusFound)
	case "/keys":
		f.jwksRequests.Add(1)
		f.mu.RLock()
		kid := f.kid
		publicKey := f.key.PublicKey
		delay := f.jwksDelay
		f.mu.RUnlock()
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-r.Context().Done():
				return
			case <-timer.C:
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA",
				"use": "sig",
				"kid": kid,
				"n":   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(publicKey.E)).Bytes()),
			}},
		})
	default:
		http.NotFound(w, r)
	}
}

func sign(key *rsa.PrivateKey, kid, algorithm, tokenType string, claims map[string]any) (string, error) {
	header := map[string]string{"alg": algorithm, "kid": kid}
	if tokenType != "" {
		header["typ"] = tokenType
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	payload := encodedHeader + "." + encodedClaims
	var (
		hash   crypto.Hash
		digest []byte
	)
	switch algorithm {
	case "RS384":
		hash = crypto.SHA384
		sum := sha512.Sum384([]byte(payload))
		digest = sum[:]
	case "RS512":
		hash = crypto.SHA512
		sum := sha512.Sum512([]byte(payload))
		digest = sum[:]
	default:
		hash = crypto.SHA256
		sum := sha256.Sum256([]byte(payload))
		digest = sum[:]
	}
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, hash, digest)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{encodedHeader, encodedClaims, base64.RawURLEncoding.EncodeToString(signature)}, "."), nil
}

func unixNow() int64 {
	// httptest token lifetimes only need whole-second precision.
	return time.Now().Unix()
}
