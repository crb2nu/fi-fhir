package requestsecurity

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	defaultOIDCTenantClaim        = "tenant_id"
	defaultOIDCRolesClaim         = "roles"
	defaultOIDCHTTPTimeout        = 10 * time.Second
	defaultJWKSRefreshMinInterval = 30 * time.Second
	maxOIDCBearerTokenBytes       = 16 * 1024
	maxOIDCProtectedHeaderBytes   = 4 * 1024
	maxOIDCResponseBytes          = 1024 * 1024
)

var errOIDCResponseTooLarge = errors.New("OIDC response exceeds limit")

var supportedOIDCSigningAlgorithms = map[string]struct{}{
	"RS256": {}, "RS384": {}, "RS512": {},
	"PS256": {}, "PS384": {}, "PS512": {},
	"ES256": {}, "ES384": {}, "ES512": {},
	"EdDSA": {},
}

// OIDCConfig defines the deployment-owned trust and claim mapping used to
// authenticate human GraphQL callers.
type OIDCConfig struct {
	IssuerURL            string
	Audience             string
	TenantID             string
	TenantClaim          string
	RolesClaim           string
	SupportedSigningAlgs []string
	// HTTPClient optionally supplies private trust roots or other deployment
	// transport settings. The client is cloned and hardened before use.
	HTTPClient *http.Client
	// JWKSRefreshMinInterval bounds outbound refreshes caused by unknown key IDs.
	// Zero selects the hardened default; negative values are rejected.
	JWKSRefreshMinInterval time.Duration
}

// OIDCAuthenticator verifies signed access tokens against one discovered OIDC
// issuer. The verifier is deliberately constructed once so its RemoteKeySet can
// cache keys and refresh the JWKS when a token presents an unknown key ID.
type OIDCAuthenticator struct {
	verifier    *oidc.IDTokenVerifier
	audience    string
	tenantID    string
	tenantClaim string
	rolesClaim  string
}

// NewOIDCAuthenticator discovers the issuer and creates a long-lived verifier.
func NewOIDCAuthenticator(ctx context.Context, config OIDCConfig) (*OIDCAuthenticator, error) {
	if ctx == nil {
		return nil, fmt.Errorf("OIDC discovery context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	issuerURL, err := validateOIDCIssuerURL(config.IssuerURL)
	if err != nil {
		return nil, err
	}
	if err := validateOIDCAudience(config.Audience); err != nil {
		return nil, err
	}
	if err := validateIdentity("tenant ID", config.TenantID); err != nil {
		return nil, err
	}

	tenantClaim := config.TenantClaim
	if tenantClaim == "" {
		tenantClaim = defaultOIDCTenantClaim
	}
	rolesClaim := config.RolesClaim
	if rolesClaim == "" {
		rolesClaim = defaultOIDCRolesClaim
	}
	if err := validateClaimName("tenant claim", tenantClaim); err != nil {
		return nil, err
	}
	if err := validateClaimName("roles claim", rolesClaim); err != nil {
		return nil, err
	}
	if tenantClaim == rolesClaim {
		return nil, fmt.Errorf("OIDC tenant and roles claims must be distinct")
	}

	signingAlgorithms, err := validateOIDCSigningAlgorithms(config.SupportedSigningAlgs)
	if err != nil {
		return nil, err
	}
	refreshMinInterval := config.JWKSRefreshMinInterval
	if refreshMinInterval < 0 {
		return nil, fmt.Errorf("OIDC JWKS refresh minimum interval must not be negative")
	}
	if refreshMinInterval == 0 {
		refreshMinInterval = defaultJWKSRefreshMinInterval
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		// Preserve the go-oidc context convention for existing callers while the
		// explicit config field remains the preferred transport boundary.
		httpClient, _ = ctx.Value(oauth2.HTTPClient).(*http.Client)
	}
	discoveryClient := hardenedOIDCHTTPClient(httpClient)
	discoveryContext := oidc.ClientContext(ctx, discoveryClient)
	provider, err := oidc.NewProvider(discoveryContext, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC issuer: %w", err)
	}
	var metadata struct {
		JWKSURL string `json:"jwks_uri"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, fmt.Errorf("read OIDC discovery metadata: %w", err)
	}
	jwksURL, err := validateOIDCJWKSURL(metadata.JWKSURL)
	if err != nil {
		return nil, err
	}

	jwksClient := *discoveryClient
	jwksClient.Transport = newJWKSRefreshTransport(discoveryClient.Transport, refreshMinInterval)
	jwksContext := oidc.ClientContext(context.Background(), &jwksClient)
	keySet := oidc.NewRemoteKeySet(jwksContext, jwksURL)

	return &OIDCAuthenticator{
		verifier: oidc.NewVerifier(issuerURL, keySet, &oidc.Config{
			ClientID:             config.Audience,
			SupportedSigningAlgs: signingAlgorithms,
		}),
		audience:    config.Audience,
		tenantID:    config.TenantID,
		tenantClaim: tenantClaim,
		rolesClaim:  rolesClaim,
	}, nil
}

// Authenticate verifies one bearer JWT and builds server-owned identity data.
func (a *OIDCAuthenticator) Authenticate(ctx context.Context, authorization string) (integration.SecurityContext, error) {
	if ctx == nil || a == nil || a.verifier == nil {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	if err := ctx.Err(); err != nil {
		return integration.SecurityContext{}, err
	}
	credential, err := bearerCredential(authorization)
	if err != nil {
		return integration.SecurityContext{}, err
	}
	if !validOIDCAccessTokenHeader(credential) {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}

	token, err := a.verifier.Verify(ctx, credential)
	if err != nil {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	if err := validateOIDCSubject(token.Subject); err != nil {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	// This runtime accepts one API audience, not a token issued jointly to other
	// relying parties. go-oidc proves membership; tighten that to exact equality
	// so a multi-audience token cannot broaden its use here.
	if len(token.Audience) != 1 || token.Audience[0] != a.audience {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}

	claims := make(map[string]json.RawMessage)
	if err := token.Claims(&claims); err != nil {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	tenantID, ok := strictStringClaim(claims[a.tenantClaim])
	if !ok || tenantID != a.tenantID {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	roles, ok := strictRolesClaim(claims[a.rolesClaim])
	if !ok {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}

	return integration.SecurityContext{
		TenantID: tenantID,
		Principal: integration.Principal{
			ID:         token.Subject,
			Kind:       integration.PrincipalKindHuman,
			AuthMethod: "oidc",
			Roles:      roles,
		},
	}, nil
}

func validateOIDCIssuerURL(raw string) (string, error) {
	if raw == "" || strings.TrimSpace(raw) != raw || containsControl(raw) {
		return "", fmt.Errorf("OIDC issuer URL is invalid")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("OIDC issuer URL must be an absolute HTTPS URL without credentials, query, or fragment")
	}
	return parsed.String(), nil
}

func validateOIDCJWKSURL(raw string) (string, error) {
	if raw == "" || strings.TrimSpace(raw) != raw || containsControl(raw) {
		return "", fmt.Errorf("OIDC JWKS URL is invalid")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", fmt.Errorf("OIDC JWKS URL must be an absolute HTTPS URL without credentials or fragment")
	}
	return parsed.String(), nil
}

func validOIDCAccessTokenHeader(token string) bool {
	if len(token) == 0 || len(token) > maxOIDCBearerTokenBytes {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(parts[0]) == 0 || len(parts[0]) > base64.RawURLEncoding.EncodedLen(maxOIDCProtectedHeaderBytes) {
		return false
	}
	header, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil || len(header) == 0 || len(header) > maxOIDCProtectedHeaderBytes {
		return false
	}

	values, ok := uniqueJSONObject(header)
	if !ok {
		return false
	}
	var tokenType string
	if err := json.Unmarshal(values["typ"], &tokenType); err != nil || tokenType != "at+jwt" {
		return false
	}
	return true
}

func uniqueJSONObject(encoded []byte) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, false
	}
	values := make(map[string]json.RawMessage)
	for decoder.More() {
		name, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		key, ok := name.(string)
		if !ok {
			return nil, false
		}
		if _, duplicate := values[key]; duplicate {
			return nil, false
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, false
		}
		values[key] = value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, false
	}
	if token, err := decoder.Token(); !errors.Is(err, io.EOF) || token != nil {
		return nil, false
	}
	return values, true
}

func hardenedOIDCHTTPClient(configured *http.Client) *http.Client {
	if configured == nil {
		configured = http.DefaultClient
	}
	client := *configured
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = httpsOnlyRoundTripper{base: transport}
	if client.Timeout <= 0 || client.Timeout > defaultOIDCHTTPTimeout {
		client.Timeout = defaultOIDCHTTPTimeout
	}
	client.CheckRedirect = func(request *http.Request, _ []*http.Request) error {
		if !validHTTPSRequestURL(request.URL) {
			return fmt.Errorf("OIDC redirect target must use HTTPS")
		}
		return fmt.Errorf("OIDC redirects are not allowed")
	}
	return &client
}

func validHTTPSRequestURL(value *url.URL) bool {
	return value != nil && value.IsAbs() && value.Scheme == "https" && value.Hostname() != "" && value.User == nil && value.Fragment == ""
}

type httpsOnlyRoundTripper struct {
	base http.RoundTripper
}

func (transport httpsOnlyRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || !validHTTPSRequestURL(request.URL) {
		return nil, fmt.Errorf("OIDC request target must use HTTPS")
	}
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response == nil || response.Body == nil {
		return nil, fmt.Errorf("OIDC response body is required")
	}
	if response.ContentLength > maxOIDCResponseBytes {
		_ = response.Body.Close()
		return nil, errOIDCResponseTooLarge
	}
	response.Body = &maxBytesReadCloser{
		body:      response.Body,
		remaining: maxOIDCResponseBytes,
	}
	return response, nil
}

type maxBytesReadCloser struct {
	body      io.ReadCloser
	remaining int64
}

func (reader *maxBytesReadCloser) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if reader.remaining == 0 {
		var probe [1]byte
		count, err := reader.body.Read(probe[:])
		if count > 0 {
			return 0, errOIDCResponseTooLarge
		}
		return 0, err
	}
	if int64(len(buffer)) > reader.remaining {
		buffer = buffer[:reader.remaining]
	}
	count, err := reader.body.Read(buffer)
	reader.remaining -= int64(count)
	return count, err
}

func (reader *maxBytesReadCloser) Close() error {
	return reader.body.Close()
}

type cachedJWKSResponse struct {
	statusCode int
	header     http.Header
	body       []byte
}

type jwksRefreshTransport struct {
	base        http.RoundTripper
	minInterval time.Duration
	now         func() time.Time

	mu          sync.Mutex
	lastAttempt time.Time
	cached      *cachedJWKSResponse
}

func newJWKSRefreshTransport(base http.RoundTripper, minInterval time.Duration) *jwksRefreshTransport {
	return &jwksRefreshTransport{base: base, minInterval: minInterval, now: time.Now}
}

func (transport *jwksRefreshTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.mu.Lock()
	defer transport.mu.Unlock()

	now := transport.now()
	if !transport.lastAttempt.IsZero() && now.Sub(transport.lastAttempt) < transport.minInterval {
		if transport.cached == nil {
			return nil, fmt.Errorf("OIDC JWKS refresh is rate limited")
		}
		return transport.cached.response(request), nil
	}
	transport.lastAttempt = now

	response, err := transport.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxOIDCResponseBytes+1))
	closeErr := response.Body.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(body) > maxOIDCResponseBytes {
		return nil, fmt.Errorf("OIDC JWKS response exceeds limit")
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	if response.StatusCode == http.StatusOK {
		transport.cached = &cachedJWKSResponse{
			statusCode: response.StatusCode,
			header:     response.Header.Clone(),
			body:       append([]byte(nil), body...),
		}
	}
	return response, nil
}

func (cached *cachedJWKSResponse) response(request *http.Request) *http.Response {
	body := append([]byte(nil), cached.body...)
	return &http.Response{
		StatusCode:    cached.statusCode,
		Status:        fmt.Sprintf("%d %s", cached.statusCode, http.StatusText(cached.statusCode)),
		Header:        cached.header.Clone(),
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       request,
	}
}

func validateOIDCAudience(value string) error {
	if value == "" || len(value) > 512 || strings.TrimSpace(value) != value || containsControl(value) {
		return fmt.Errorf("OIDC audience is invalid")
	}
	return nil
}

func validateOIDCSubject(value string) error {
	if value == "" || len(value) > 255 || strings.TrimSpace(value) != value || containsControl(value) {
		return fmt.Errorf("OIDC subject is invalid")
	}
	return nil
}

func validateClaimName(label, value string) error {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value || containsControl(value) {
		return fmt.Errorf("OIDC %s is invalid", label)
	}
	return nil
}

func validateOIDCSigningAlgorithms(configured []string) ([]string, error) {
	if len(configured) == 0 {
		return []string{oidc.RS256}, nil
	}
	algorithms := make([]string, len(configured))
	seen := make(map[string]struct{}, len(configured))
	for index, algorithm := range configured {
		if _, supported := supportedOIDCSigningAlgorithms[algorithm]; !supported {
			return nil, fmt.Errorf("OIDC signing algorithm %q is not supported", algorithm)
		}
		if _, duplicate := seen[algorithm]; duplicate {
			return nil, fmt.Errorf("OIDC signing algorithm %q is duplicated", algorithm)
		}
		seen[algorithm] = struct{}{}
		algorithms[index] = algorithm
	}
	return algorithms, nil
}

func strictStringClaim(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	if err := validateIdentity("OIDC tenant", value); err != nil {
		return "", false
	}
	return value, true
}

func strictRolesClaim(raw json.RawMessage) ([]string, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var roles []string
	if err := json.Unmarshal(raw, &roles); err != nil || len(roles) == 0 {
		return nil, false
	}
	seen := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		if role == "" || len(role) > 256 || strings.TrimSpace(role) != role || containsControl(role) {
			return nil, false
		}
		if _, duplicate := seen[role]; duplicate {
			return nil, false
		}
		seen[role] = struct{}{}
	}
	// A canonical order prevents identity-equivalent token claims from producing
	// different security-context representations.
	slices.Sort(roles)
	return roles, true
}
