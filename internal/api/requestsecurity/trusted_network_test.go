package requestsecurity

import (
	"net/http/httptest"
	"testing"
)

func TestTrustedNetworkAuthenticator(t *testing.T) {
	authenticator, err := NewTrustedNetworkAuthenticator(TrustedNetworkConfig{
		CIDRs:       "192.168.50.0/24, 2001:db8::/32",
		TenantID:    "tenant-a",
		PrincipalID: "engineer-1",
		Roles:       []string{"integration:preview", "author"},
	})
	if err != nil {
		t.Fatalf("NewTrustedNetworkAuthenticator: %v", err)
	}

	t.Run("uses ingress real IP", func(t *testing.T) {
		request := httptest.NewRequest("GET", "/api/auth/status", nil)
		request.RemoteAddr = "10.42.0.12:8080"
		request.Header.Set("X-Real-IP", "192.168.50.24")
		request.Header.Set("X-Forwarded-For", "203.0.113.9, 10.42.0.1")

		security, ok := authenticator.AuthenticateRequest(request)
		if !ok {
			t.Fatal("trusted ingress client was not authenticated")
		}
		if security.TenantID != "tenant-a" || security.Principal.ID != "engineer-1" {
			t.Fatalf("security = %#v", security)
		}
		if security.Principal.AuthMethod != "network" {
			t.Fatalf("auth method = %q, want network", security.Principal.AuthMethod)
		}
	})

	t.Run("uses leftmost forwarded IP", func(t *testing.T) {
		request := httptest.NewRequest("GET", "/api/auth/status", nil)
		request.RemoteAddr = "10.42.0.12:8080"
		request.Header.Set("X-Forwarded-For", "2001:db8::5, 10.42.0.1")
		if _, ok := authenticator.AuthenticateRequest(request); !ok {
			t.Fatal("trusted forwarded client was not authenticated")
		}
	})

	t.Run("rejects untrusted client", func(t *testing.T) {
		request := httptest.NewRequest("GET", "/api/auth/status", nil)
		request.RemoteAddr = "203.0.113.9:8080"
		if _, ok := authenticator.AuthenticateRequest(request); ok {
			t.Fatal("untrusted client was authenticated")
		}
	})
}

func TestTrustedNetworkAuthenticatorRejectsInvalidConfiguration(t *testing.T) {
	_, err := NewTrustedNetworkAuthenticator(TrustedNetworkConfig{
		CIDRs:       "192.168.50.0/24,not-a-network",
		TenantID:    "tenant-a",
		PrincipalID: "engineer-1",
		Roles:       []string{"integration:preview"},
	})
	if err == nil {
		t.Fatal("invalid trusted CIDR configuration was accepted")
	}
}
