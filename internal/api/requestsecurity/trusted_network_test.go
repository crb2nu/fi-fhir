package requestsecurity

import (
	"errors"
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

// TestTrustedNetworkAuthenticatorRefusesAnyAddressWithoutOptIn: a prefix of
// length 0 hands the configured roles to every caller that can reach the
// listener, so it is refused unless the deployment opts in explicitly.
func TestTrustedNetworkAuthenticatorRefusesAnyAddressWithoutOptIn(t *testing.T) {
	for _, cidrs := range []string{"0.0.0.0/0", "::/0", "192.168.50.0/24, ::/0", "10.0.0.1/0"} {
		t.Run(cidrs, func(t *testing.T) {
			_, err := NewTrustedNetworkAuthenticator(TrustedNetworkConfig{
				CIDRs:       cidrs,
				TenantID:    "tenant-a",
				PrincipalID: "engineer-1",
				Roles:       []string{"integration:preview"},
			})
			if !errors.Is(err, ErrTrustedNetworkAdmitsAnyAddress) {
				t.Fatalf("error = %v, want ErrTrustedNetworkAdmitsAnyAddress", err)
			}
		})
	}
}

func TestTrustedNetworkAuthenticatorAllowsAnyAddressWithOptIn(t *testing.T) {
	authenticator, err := NewTrustedNetworkAuthenticator(TrustedNetworkConfig{
		CIDRs:           "0.0.0.0/0,::/0",
		TenantID:        "tenant-a",
		PrincipalID:     "fi-fhir-demo-visitor",
		Roles:           []string{"integration:preview"},
		AllowAnyAddress: true,
	})
	if err != nil {
		t.Fatalf("NewTrustedNetworkAuthenticator: %v", err)
	}
	if !authenticator.AdmitsAnyAddress() {
		t.Fatal("AdmitsAnyAddress = false for 0.0.0.0/0,::/0")
	}
	for _, remote := range []string{"203.0.113.9:443", "[2001:db8::5]:443"} {
		request := httptest.NewRequest("GET", "/api/auth/status", nil)
		request.RemoteAddr = remote
		security, ok := authenticator.AuthenticateRequest(request)
		if !ok {
			t.Fatalf("%s was not admitted", remote)
		}
		if security.Principal.ID != "fi-fhir-demo-visitor" || len(security.Principal.Roles) != 1 {
			t.Fatalf("security = %#v", security)
		}
	}
}

// The opt-in does not widen a bounded allowlist or change what it reports.
func TestTrustedNetworkAuthenticatorOptInIsInertForBoundedAllowlist(t *testing.T) {
	authenticator, err := NewTrustedNetworkAuthenticator(TrustedNetworkConfig{
		CIDRs:           "192.168.50.0/24",
		TenantID:        "tenant-a",
		PrincipalID:     "engineer-1",
		Roles:           []string{"integration:preview"},
		AllowAnyAddress: true,
	})
	if err != nil {
		t.Fatalf("NewTrustedNetworkAuthenticator: %v", err)
	}
	if authenticator.AdmitsAnyAddress() {
		t.Fatal("AdmitsAnyAddress = true for a /24")
	}
	request := httptest.NewRequest("GET", "/api/auth/status", nil)
	request.RemoteAddr = "203.0.113.9:8080"
	if _, ok := authenticator.AuthenticateRequest(request); ok {
		t.Fatal("untrusted client was authenticated")
	}
}
