package requestsecurity

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// TrustedNetworkConfig grants the deployment-owned GraphQL identity to
// requests arriving from an explicitly configured network.
type TrustedNetworkConfig struct {
	CIDRs       string
	TenantID    string
	PrincipalID string
	Roles       []string
}

// TrustedNetworkAuthenticator authenticates requests by their ingress-reported
// client address. It is intended for single-tenant LAN deployments only.
type TrustedNetworkAuthenticator struct {
	networks []netip.Prefix
	security integration.SecurityContext
}

// NewTrustedNetworkAuthenticator validates the complete allowlist and identity.
// Invalid CIDR entries fail startup instead of silently widening or weakening
// the configured boundary.
func NewTrustedNetworkAuthenticator(config TrustedNetworkConfig) (*TrustedNetworkAuthenticator, error) {
	networks, err := parseTrustedNetworks(config.CIDRs)
	if err != nil {
		return nil, err
	}
	if len(networks) == 0 {
		return nil, fmt.Errorf("at least one trusted network is required")
	}
	if err := validateIdentity("tenant ID", config.TenantID); err != nil {
		return nil, err
	}
	if err := validateIdentity("principal ID", config.PrincipalID); err != nil {
		return nil, err
	}
	if len(config.Roles) == 0 {
		return nil, fmt.Errorf("at least one role is required")
	}

	roles := make([]string, len(config.Roles))
	seen := make(map[string]struct{}, len(config.Roles))
	for index, role := range config.Roles {
		if err := validateIdentity("role", role); err != nil {
			return nil, err
		}
		if _, exists := seen[role]; exists {
			return nil, fmt.Errorf("role %q is duplicated", role)
		}
		seen[role] = struct{}{}
		roles[index] = role
	}

	return &TrustedNetworkAuthenticator{
		networks: networks,
		security: integration.SecurityContext{
			TenantID: config.TenantID,
			Principal: integration.Principal{
				ID:         config.PrincipalID,
				Kind:       integration.PrincipalKindHuman,
				AuthMethod: "network",
				Roles:      roles,
			},
		},
	}, nil
}

// AuthenticateRequest returns the configured identity when the detected client
// address is contained by the allowlist.
func (a *TrustedNetworkAuthenticator) AuthenticateRequest(request *http.Request) (integration.SecurityContext, bool) {
	if a == nil || request == nil {
		return integration.SecurityContext{}, false
	}
	address, ok := trustedClientAddress(request)
	if !ok {
		return integration.SecurityContext{}, false
	}
	for _, network := range a.networks {
		if network.Contains(address) {
			return cloneSecurityContext(a.security), true
		}
	}
	return integration.SecurityContext{}, false
}

func parseTrustedNetworks(value string) ([]netip.Prefix, error) {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	networks := make([]netip.Prefix, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(field)
		if err != nil {
			address, addressErr := netip.ParseAddr(field)
			if addressErr != nil {
				return nil, fmt.Errorf("trusted network %q is not a CIDR or IP address", field)
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		networks = append(networks, prefix.Masked())
	}
	return networks, nil
}

func trustedClientAddress(request *http.Request) (netip.Addr, bool) {
	if value := strings.TrimSpace(request.Header.Get("X-Real-IP")); value != "" {
		if address, err := netip.ParseAddr(value); err == nil {
			return address.Unmap(), true
		}
	}
	if value := request.Header.Get("X-Forwarded-For"); value != "" {
		first := strings.TrimSpace(strings.Split(value, ",")[0])
		if address, err := netip.ParseAddr(first); err == nil {
			return address.Unmap(), true
		}
	}
	host := strings.TrimSpace(request.RemoteAddr)
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	address, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap(), true
}
