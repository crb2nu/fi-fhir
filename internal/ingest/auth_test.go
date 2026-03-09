package ingest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthConfig_VerifyRequest_None(t *testing.T) {
	cases := []struct {
		name string
		cfg  *AuthConfig
	}{
		{"nil config", nil},
		{"empty mode", &AuthConfig{}},
		{"explicit none", &AuthConfig{Mode: AuthNone}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/webhook", nil)
			if err := tc.cfg.VerifyRequest(r, []byte("body")); err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestAuthConfig_VerifyRequest_HMAC(t *testing.T) {
	secret := "test-secret-key"
	body := []byte(`{"event":"test"}`)

	// Compute valid signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := hex.EncodeToString(mac.Sum(nil))

	cases := []struct {
		name      string
		header    string
		body      []byte
		wantErr   bool
		headerKey string // custom header
	}{
		{"valid sig with prefix", "sha256=" + validSig, body, false, ""},
		{"valid sig without prefix", validSig, body, false, ""},
		{"missing header", "", body, true, ""},
		{"wrong signature", "sha256=0000000000000000000000000000000000000000000000000000000000000000", body, true, ""},
		{"invalid hex", "sha256=not-hex", body, true, ""},
		{"wrong body", "sha256=" + validSig, []byte("wrong"), true, ""},
		{"custom header name", "sha256=" + validSig, body, false, "X-Hub-Signature-256"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &AuthConfig{
				Mode:       AuthHMAC,
				Secret:     secret,
				HeaderName: tc.headerKey,
			}
			headerName := cfg.signatureHeader()

			r := httptest.NewRequest(http.MethodPost, "/webhook", nil)
			if tc.header != "" {
				r.Header.Set(headerName, tc.header)
			}

			err := cfg.VerifyRequest(r, tc.body)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestAuthConfig_VerifyRequest_Bearer(t *testing.T) {
	secret := "my-webhook-token"

	cases := []struct {
		name    string
		header  string
		wantErr bool
	}{
		{"valid token", "Bearer my-webhook-token", false},
		{"missing header", "", true},
		{"wrong token", "Bearer wrong-token", true},
		{"wrong scheme", "Basic my-webhook-token", true},
		{"empty bearer", "Bearer ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &AuthConfig{Mode: AuthBearer, Secret: secret}
			r := httptest.NewRequest(http.MethodPost, "/webhook", nil)
			if tc.header != "" {
				r.Header.Set("Authorization", tc.header)
			}

			err := cfg.VerifyRequest(r, nil)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestAuthConfig_VerifyRequest_UnsupportedMode(t *testing.T) {
	cfg := &AuthConfig{Mode: "oauth2"}
	r := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	err := cfg.VerifyRequest(r, nil)
	if err == nil {
		t.Fatal("expected error for unsupported mode")
	}
}
