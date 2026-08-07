package ingress

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const testBearerSecret = "correct-horse-battery-staple"

var testHMACKey = strings.Repeat("test-hmac-key-", 3)

func TestAuthenticatorBearer(t *testing.T) {
	authenticator := mustAuthenticator(t, AuthConfig{
		Mode: AuthModeBearer, Secret: testBearerSecret,
		TenantID: "tenant-a", PrincipalID: "source-service", IntegrationID: "adt-tolerant",
	})
	tests := []struct {
		name   string
		header []string
		want   error
	}{
		{name: "valid", header: []string{"Bearer " + testBearerSecret}},
		{name: "case insensitive scheme", header: []string{"bearer " + testBearerSecret}},
		{name: "missing", want: ErrMissingCredentials},
		{name: "wrong", header: []string{"Bearer wrong"}, want: ErrInvalidCredentials},
		{name: "duplicate", header: []string{"Bearer " + testBearerSecret, "Bearer " + testBearerSecret}, want: ErrMissingCredentials},
		{name: "tab", header: []string{"Bearer\t" + testBearerSecret}, want: ErrInvalidCredentials},
		{name: "repeated space", header: []string{"Bearer  " + testBearerSecret}, want: ErrInvalidCredentials},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", Path, nil)
			for _, value := range test.header {
				request.Header.Add("Authorization", value)
			}
			err := authenticator.Authenticate(request, nil)
			if !errors.Is(err, test.want) {
				t.Fatalf("Authenticate() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestAuthenticatorReturnsDeploymentBoundServiceIdentity(t *testing.T) {
	authenticator := mustAuthenticator(t, AuthConfig{
		Mode: AuthModeBearer, Secret: testBearerSecret,
		TenantID: "tenant-a", PrincipalID: "source-service", IntegrationID: "adt-tolerant",
	})
	request := httptest.NewRequest("POST", Path, nil)
	request.Header.Set("Authorization", "Bearer "+testBearerSecret)
	security, err := authenticator.AuthenticateRequest(request.Context(), request, nil)
	if err != nil {
		t.Fatalf("AuthenticateRequest: %v", err)
	}
	if security.TenantID != "tenant-a" || security.Principal.ID != "source-service" || security.Principal.Kind != integration.PrincipalKindService || security.Principal.AuthMethod != "bearer" || security.Principal.SourceID != "" {
		t.Fatalf("security context = %#v", security)
	}
	if len(security.Principal.Roles) != 1 || security.Principal.Roles[0] != SubmitRole {
		t.Fatalf("service roles = %#v", security.Principal.Roles)
	}
}

func TestAuthenticatorHMACBindsTransportFieldsAndBody(t *testing.T) {
	authenticator := mustAuthenticator(t, AuthConfig{
		Mode: AuthModeHMAC, Secret: testHMACKey,
		TenantID: "tenant-a", PrincipalID: "source-service", IntegrationID: "adt-tolerant",
	})
	body := []byte("MSH|^~\\&|APP|FAC")
	request := httptest.NewRequest("POST", Path, nil)
	request.Header.Set(idempotencyHeader, "idem-1")
	request.Header.Set(correlationHeader, "corr-1")
	request.Header.Set(signatureHeader, hmacHeader(testHMACKey, "adt-tolerant", "idem-1", "corr-1", body))
	if err := authenticator.Authenticate(request, body); err != nil {
		t.Fatalf("Authenticate(valid): %v", err)
	}

	for _, mutate := range []func(*http.Request, *[]byte){
		func(request *http.Request, _ *[]byte) { request.Header.Set(idempotencyHeader, "idem-2") },
		func(request *http.Request, _ *[]byte) { request.Header.Set(correlationHeader, "corr-2") },
		func(_ *http.Request, body *[]byte) { *body = append(*body, 'X') },
	} {
		candidate := request.Clone(request.Context())
		candidate.Header = request.Header.Clone()
		candidateBody := append([]byte(nil), body...)
		mutate(candidate, &candidateBody)
		if err := authenticator.Authenticate(candidate, candidateBody); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("tampered Authenticate() error = %v", err)
		}
	}
}

func TestNewAuthenticatorRejectsUnsafeConfig(t *testing.T) {
	tests := []AuthConfig{
		{},
		{Mode: AuthModeBearer, Secret: "short", TenantID: "tenant-a", PrincipalID: "source", IntegrationID: "adt"},
		{Mode: AuthModeHMAC, Secret: strings.Repeat("x", 31), TenantID: "tenant-a", PrincipalID: "source", IntegrationID: "adt"},
		{Mode: AuthModeHMAC, Secret: strings.Repeat("x", 32) + "\n", TenantID: "tenant-a", PrincipalID: "source", IntegrationID: "adt"},
		{Mode: AuthModeBearer, Secret: testBearerSecret, TenantID: "tenant-a", PrincipalID: "bad principal", IntegrationID: "adt"},
		{Mode: AuthModeBearer, Secret: testBearerSecret, TenantID: "tenant-a", PrincipalID: "source", IntegrationID: "bad integration"},
	}
	for index, config := range tests {
		if _, err := NewAuthenticator(config); err == nil {
			t.Fatalf("config %d unexpectedly accepted", index)
		}
	}
}

func mustAuthenticator(t *testing.T, config AuthConfig) *Authenticator {
	t.Helper()
	authenticator, err := NewAuthenticator(config)
	if err != nil {
		t.Fatalf("NewAuthenticator: %v", err)
	}
	return authenticator
}

func hmacHeader(secret, integrationID, idempotencyKey, correlationID string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(signaturePayload(integrationID, idempotencyKey, correlationID, body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
