package connection

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// The HTTP source digests below were computed when the document was
// introduced (.loom/38 C-0). They are constants on purpose: a test that
// recomputed them would prove nothing. If one moves, every definition that
// names an HTTP source revision stops resolving it; fix the code, never the
// constant.
const (
	pinnedHTTPBearerSourceDigest = "sha256:73d4d612aae1df08adc7227c7755c9df80b0686d43259d6459fc374a320833e1"
	pinnedHTTPOAuthSourceDigest  = "sha256:7ac402903f3954dd12de37b90892e94bc184c1b0e4c55bc46add13149bbf52b9"
)

func bearerHTTPSourceInput() HTTPSourceRevisionInput {
	return HTTPSourceRevisionInput{
		ArtifactID: "source-adt-http", RevisionID: "1", SourceID: "adt-east",
		Path: DefaultHTTPSourcePath, AuthMode: HTTPAuthModeBearer,
		PrincipalID: "adt-east-sender", CredentialBinding: "http-ingress-credential",
		MaxBodyBytes: 1 << 20,
	}
}

func oauthHTTPSourceInput() HTTPSourceRevisionInput {
	return HTTPSourceRevisionInput{
		ArtifactID: "source-adt-http", RevisionID: "2", SourceID: "adt-east",
		Path: DefaultHTTPSourcePath, AuthMode: HTTPAuthModeOAuth2,
		OAuth: &HTTPOAuthPolicy{
			IssuerURL: "https://issuer.example.org/realms/fi-fhir", Audience: "fi-fhir-ingress",
			TenantClaim: "tenant_id", RolesClaim: "roles", ClientIDClaim: "client_id",
			SigningAlgs: []string{"RS256", "ES256"}, AllowedClientIDs: []string{"lab-b", "lab-a"},
		},
		MaxBodyBytes: 512 << 10,
	}
}

func TestHTTPSourceRevisionDigestsArePinned(t *testing.T) {
	for name, tc := range map[string]struct {
		input HTTPSourceRevisionInput
		want  string
	}{
		"bearer": {bearerHTTPSourceInput(), pinnedHTTPBearerSourceDigest},
		"oauth2": {oauthHTTPSourceInput(), pinnedHTTPOAuthSourceDigest},
	} {
		t.Run(name, func(t *testing.T) {
			revision, err := NewHTTPSourceRevision(tc.input)
			if err != nil {
				t.Fatalf("NewHTTPSourceRevision: %v", err)
			}
			if revision.Digest != tc.want {
				t.Fatalf("HTTP source digest moved:\n got  %s\n want %s", revision.Digest, tc.want)
			}
		})
	}
}

// TestHTTPSourceRevisionDigestIsDomainSeparatedAndOrderFree: the digest covers
// the domain tag, so the same bytes hashed as another document type do not
// collide, and the two OAuth sets are canonicalised, so listing them in
// another order names the same source.
func TestHTTPSourceRevisionDigestIsDomainSeparatedAndOrderFree(t *testing.T) {
	revision, err := NewHTTPSourceRevision(oauthHTTPSourceInput())
	if err != nil {
		t.Fatalf("NewHTTPSourceRevision: %v", err)
	}
	reordered := oauthHTTPSourceInput()
	reordered.OAuth.SigningAlgs = []string{"ES256", "RS256"}
	reordered.OAuth.AllowedClientIDs = []string{"lab-a", "lab-b"}
	again, err := NewHTTPSourceRevision(reordered)
	if err != nil || again.Digest != revision.Digest {
		t.Fatalf("reordered sets changed the digest: %s vs %s (%v)", again.Digest, revision.Digest, err)
	}
	canonical := revision
	canonical.Digest = ""
	canonical.OAuth = canonicalHTTPOAuth(revision.OAuth)
	encoded, _ := json.Marshal(canonical)
	undomained := sha256.Sum256(encoded)
	if revision.Digest == "sha256:"+hex.EncodeToString(undomained[:]) {
		t.Fatal("the HTTP source digest is not domain-separated")
	}
}

func TestHTTPSourceRevisionDecodeRoundTripAndTamper(t *testing.T) {
	revision, err := NewHTTPSourceRevision(bearerHTTPSourceInput())
	if err != nil {
		t.Fatalf("NewHTTPSourceRevision: %v", err)
	}
	encoded, err := json.Marshal(revision)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded, err := DecodeHTTPSourceRevision(bytes.NewReader(encoded))
	if err != nil || decoded.Digest != revision.Digest || decoded.Reference() != revision.Reference() {
		t.Fatalf("round trip: %+v, %v", decoded, err)
	}

	for name, mutate := range map[string]func(string) string{
		"tampered principal": func(s string) string {
			return strings.Replace(s, `"adt-east-sender"`, `"someone-else"`, 1)
		},
		"tampered body limit": func(s string) string {
			return strings.Replace(s, `"max_body_bytes":1048576`, `"max_body_bytes":1048575`, 1)
		},
		"unknown field": func(s string) string {
			return strings.Replace(s, `{"schema_version"`, `{"extra":true,"schema_version"`, 1)
		},
		"duplicate key": func(s string) string {
			return strings.Replace(s, `{"schema_version":"1"`, `{"schema_version":"1","schema_version":"1"`, 1)
		},
		"trailing value": func(s string) string { return s + ` {}` },
		"empty":          func(string) string { return "" },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeHTTPSourceRevision(strings.NewReader(mutate(string(encoded)))); !errors.Is(err, ErrInvalidHTTPSourceRevision) {
				t.Fatalf("decode error = %v, want ErrInvalidHTTPSourceRevision", err)
			}
		})
	}
	if _, err := DecodeHTTPSourceRevision(nil); !errors.Is(err, ErrInvalidHTTPSourceRevision) {
		t.Fatalf("nil reader error = %v", err)
	}
}

// TestHTTPSourceRevisionAuthModeInvariants is the runtime's own rule: bearer
// and HMAC name a principal and a credential binding and nothing OAuth;
// OAuth2 names neither and carries its policy.
func TestHTTPSourceRevisionAuthModeInvariants(t *testing.T) {
	for name, mutate := range map[string]func(*HTTPSourceRevisionInput){
		"bearer without principal":        func(in *HTTPSourceRevisionInput) { in.PrincipalID = "" },
		"bearer without credential":       func(in *HTTPSourceRevisionInput) { in.CredentialBinding = "" },
		"bearer with oauth":               func(in *HTTPSourceRevisionInput) { in.OAuth = oauthHTTPSourceInput().OAuth },
		"unknown mode":                    func(in *HTTPSourceRevisionInput) { in.AuthMode = "basic" },
		"oauth2 with a principal":         func(in *HTTPSourceRevisionInput) { *in = oauthHTTPSourceInput(); in.PrincipalID = "p" },
		"oauth2 with a credential":        func(in *HTTPSourceRevisionInput) { *in = oauthHTTPSourceInput(); in.CredentialBinding = "c" },
		"oauth2 without a policy":         func(in *HTTPSourceRevisionInput) { *in = oauthHTTPSourceInput(); in.OAuth = nil },
		"relative path":                   func(in *HTTPSourceRevisionInput) { in.Path = "v1/hl7v2" },
		"body limit above the kernel cap": func(in *HTTPSourceRevisionInput) { in.MaxBodyBytes = 1<<20 + 1 },
		"artifact id with whitespace":     func(in *HTTPSourceRevisionInput) { in.ArtifactID = "a b" },
	} {
		t.Run(name, func(t *testing.T) {
			input := bearerHTTPSourceInput()
			mutate(&input)
			if _, err := NewHTTPSourceRevision(input); !errors.Is(err, ErrInvalidHTTPSourceRevision) {
				t.Fatalf("error = %v, want ErrInvalidHTTPSourceRevision", err)
			}
		})
	}
	hmac := bearerHTTPSourceInput()
	hmac.AuthMode = HTTPAuthModeHMAC
	if _, err := NewHTTPSourceRevision(hmac); err != nil {
		t.Fatalf("hmac-sha256: %v", err)
	}
}
