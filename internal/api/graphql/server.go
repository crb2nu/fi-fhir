package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	gqlgengraphql "github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
)

// AuthStatusPath is the unauthenticated browser probe for optional trusted-
// network access. It never grants access by itself; GraphQL re-evaluates the
// request address on every operation.
const AuthStatusPath = "/api/auth/status"

// ServerConfig configures the GraphQL server.
type ServerConfig struct {
	// Host to bind to
	Host string
	// Port to listen on
	Port int
	// Path for GraphQL endpoint
	Path string
	// Enable GraphQL Playground
	PlaygroundEnabled bool
	// PlaygroundPath for playground endpoint
	PlaygroundPath string
	// WebSocket path for subscriptions
	WebSocketPath string
	// Max query depth
	MaxDepth int
	// Max query complexity
	MaxComplexity int
	// Request timeout
	Timeout time.Duration
	// Enable introspection
	Introspection bool
	// CORS allowed origins
	AllowedOrigins []string
	// MaxRequestBodyBytes bounds the complete GraphQL JSON request body.
	MaxRequestBodyBytes int64
	// IntegrationSessionStreaming enables the authenticated, session-only
	// GraphQL SSE transport on the existing bounded POST endpoint.
	IntegrationSessionStreaming bool
	// Authenticator establishes the deployment-owned tenant/principal context.
	Authenticator requestsecurity.Authenticator
	// TrustedNetworkAuthenticator optionally establishes the same deployment-
	// owned identity for explicitly allowlisted LAN clients.
	TrustedNetworkAuthenticator *requestsecurity.TrustedNetworkAuthenticator
	// HL7IngressPath is the exact authenticated raw-HL7v2 endpoint when enabled.
	HL7IngressPath string
	// HL7IngressHandler owns production authentication and durable submission.
	HL7IngressHandler http.Handler
	// Health reports real liveness and readiness. When nil the server keeps the
	// pre-Slice-4.3 literal `/health` and mounts no `/ready`, which is what
	// FI_FHIR_OBSERVABILITY_MODE=legacy and the in-package transport tests use.
	Health observability.Reporter
}

// ReadinessPath is the dependency-touching probe. It is unauthenticated for the
// same reason `/health` is: a kubelet has no bearer token, and the response
// carries component names and states only — never a dependency address, a
// credential, a tenant identifier, or any message-derived value.
const ReadinessPath = "/ready"

// LivenessPath is the process-only probe.
const LivenessPath = "/health"

// DefaultServerConfig returns a sensible default configuration.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Host:              "0.0.0.0",
		Port:              8081,
		Path:              "/graphql",
		PlaygroundEnabled: true,
		PlaygroundPath:    "/",
		WebSocketPath:     "/graphql/ws",
		MaxDepth:          10,
		MaxComplexity:     1000,
		Timeout:           30 * time.Second,
		Introspection:     true,
		AllowedOrigins: []string{
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		},
		MaxRequestBodyBytes: 1 << 20,
	}
}

// Server is the GraphQL HTTP server.
type Server struct {
	config   *ServerConfig
	resolver ResolverRoot
	handler  *handler.Server
	server   *http.Server
}

// NewServer creates a new GraphQL server.
// resolver must implement the ResolverRoot interface defined in generated.go.
func NewServer(resolver ResolverRoot, config *ServerConfig) (*Server, error) {
	if config == nil {
		config = DefaultServerConfig()
	}
	if resolver == nil {
		return nil, fmt.Errorf("GraphQL resolver is required")
	}
	if err := validateServerConfig(config); err != nil {
		return nil, err
	}

	// Create the GraphQL schema
	schema := NewExecutableSchema(Config{
		Resolvers: resolver,
	})

	// Create the handler with transport configuration
	srv := handler.New(schema)
	srv.SetErrorPresenter(catalogSafeErrorPresenter)

	// Raw clinical data is accepted only inside a bounded authenticated POST.
	// Session subscriptions deliberately reuse that boundary through SSE instead
	// of opening an unbounded pre-authentication WebSocket frame.
	if config.IntegrationSessionStreaming {
		srv.AddTransport(transport.SSE{KeepAlivePingInterval: 15 * time.Second})
	}
	srv.AddTransport(transport.POST{})

	// Cache parsed query documents. Persisted-query negotiation is intentionally
	// unavailable so every request passes the same bounded JSON envelope.
	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	// Add extensions
	if config.Introspection {
		srv.Use(extension.Introspection{})
	}
	srv.Use(operationAuthorization{})
	srv.Use(fixedQueryDepthLimit{limit: config.MaxDepth})
	srv.Use(extension.FixedComplexityLimit(config.MaxComplexity))

	return &Server{
		config:   config,
		resolver: resolver,
		handler:  srv,
	}, nil
}

// Handler returns the production HTTP handler used by Start.
//
// Exposing the composed handler allows transport-level tests and embedders to
// exercise the same GraphQL, session stream, disabled-WebSocket, playground,
// and health routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// GraphQL endpoint: CORS preflight is the only unauthenticated method.
	graphqlHTTP := graphqlHTTPMiddleware(s.handler, s.config)
	mux.Handle(s.config.Path, corsMiddleware(graphqlHTTP, s.config.AllowedOrigins))

	// WebSocket remains unavailable. Session streaming uses authenticated SSE on
	// the bounded GraphQL POST endpoint, so mutations cannot bypass that boundary.
	mux.HandleFunc(s.config.WebSocketPath, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	if s.config.HL7IngressHandler != nil {
		mux.Handle(s.config.HL7IngressPath, s.config.HL7IngressHandler)
	}

	// Playground (if enabled)
	if s.config.PlaygroundEnabled {
		mux.Handle(s.config.PlaygroundPath, playground.Handler("fi-fhir GraphQL", s.config.Path))
	}

	// Liveness and readiness.
	//
	// Before Slice 4.3 `/health` was one handler that unconditionally wrote
	// `{"status":"healthy","service":"graphql"}` and `/ready` did not exist, so a
	// replica with a dead connection pool kept passing every probe and kept
	// receiving Service traffic. Both probes now project the same component set
	// the GraphQL `health` query returns.
	if s.config.Health != nil {
		mux.Handle(LivenessPath, observability.LivenessHandler(s.config.Health))
		mux.Handle(ReadinessPath, observability.ReadinessHandler(s.config.Health))
	} else {
		mux.Handle(LivenessPath, observability.LegacyHealthHandler())
	}

	mux.HandleFunc(AuthStatusPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "auth status requires GET", http.StatusMethodNotAllowed)
			return
		}
		_, trusted := s.config.TrustedNetworkAuthenticator.AuthenticateRequest(r)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		if trusted {
			_ = json.NewEncoder(w).Encode(map[string]any{"authenticated": true, "authVia": "network"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"authenticated": false})
	})

	return mux
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  s.config.Timeout,
		WriteTimeout: s.config.Timeout,
	}

	log.Printf("GraphQL server listening on http://%s%s", addr, s.config.Path)
	if s.config.PlaygroundEnabled {
		log.Printf("GraphQL Playground available at http://%s%s", addr, s.config.PlaygroundPath)
	}
	if s.config.HL7IngressHandler != nil {
		log.Printf("Authenticated HL7v2 ingress available at http://%s%s", addr, s.config.HL7IngressPath)
	}

	return s.server.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// corsMiddleware enforces an exact browser origin allowlist. Requests without
// Origin remain valid for non-browser clients and health tooling.
func corsMiddleware(next http.Handler, allowedOrigins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		w.Header().Add("Vary", "Origin")
		if origin != "" && !originAllowed(origin, allowedOrigins) {
			http.Error(w, "origin is not allowed", http.StatusForbidden)
			return
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		if r.Method == "OPTIONS" {
			if r.Header.Get("Access-Control-Request-Method") != http.MethodPost {
				http.Error(w, "CORS method is not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func validateServerConfig(config *ServerConfig) error {
	if config.Authenticator == nil {
		return fmt.Errorf("GraphQL authenticator is required")
	}
	if config.MaxRequestBodyBytes <= 0 {
		return fmt.Errorf("GraphQL request body limit must be positive")
	}
	if config.MaxDepth <= 0 {
		return fmt.Errorf("GraphQL query depth limit must be positive")
	}
	if config.MaxComplexity <= 0 {
		return fmt.Errorf("GraphQL query complexity limit must be positive")
	}
	if config.Timeout <= 0 {
		return fmt.Errorf("GraphQL request timeout must be positive")
	}
	if len(config.AllowedOrigins) == 0 {
		return fmt.Errorf("at least one explicit GraphQL origin is required")
	}
	seen := make(map[string]struct{}, len(config.AllowedOrigins))
	for _, origin := range config.AllowedOrigins {
		if err := validateOrigin(origin); err != nil {
			return err
		}
		if _, duplicate := seen[origin]; duplicate {
			return fmt.Errorf("GraphQL origin %q is duplicated", origin)
		}
		seen[origin] = struct{}{}
	}
	paths := map[string]string{
		"GraphQL HTTP":      config.Path,
		"GraphQL WebSocket": config.WebSocketPath,
	}
	if (config.HL7IngressHandler == nil) != (config.HL7IngressPath == "") {
		return fmt.Errorf("HL7v2 ingress path and handler must be configured together")
	}
	if config.HL7IngressHandler != nil {
		paths["HL7v2 ingress"] = config.HL7IngressPath
	}
	if config.PlaygroundEnabled {
		paths["GraphQL Playground"] = config.PlaygroundPath
	}
	seenPaths := map[string]string{
		LivenessPath:   "liveness probe",
		ReadinessPath:  "readiness probe",
		AuthStatusPath: "auth status",
	}
	for name, configuredPath := range paths {
		if err := validateServeMuxPath(name, configuredPath); err != nil {
			return err
		}
		if previous, duplicate := seenPaths[configuredPath]; duplicate {
			return fmt.Errorf("%s path %q conflicts with %s", name, configuredPath, previous)
		}
		seenPaths[configuredPath] = name
	}
	if config.Path == "/" || config.WebSocketPath == "/" {
		return fmt.Errorf("GraphQL HTTP and WebSocket paths must not use the root catch-all")
	}
	return nil
}

func validateServeMuxPath(name, configuredPath string) error {
	if configuredPath == "" || !strings.HasPrefix(configuredPath, "/") {
		return fmt.Errorf("%s path %q must be an absolute URL path", name, configuredPath)
	}
	if configuredPath != "/" && strings.HasSuffix(configuredPath, "/") {
		return fmt.Errorf("%s path %q must not be a subtree pattern", name, configuredPath)
	}
	if strings.Contains(configuredPath, "//") || strings.ContainsAny(configuredPath, "{}%?#\\") || strings.IndexFunc(configuredPath, func(r rune) bool {
		return r <= ' ' || r == 0x7f
	}) >= 0 {
		return fmt.Errorf("%s path %q contains unsupported ServeMux syntax", name, configuredPath)
	}
	for _, segment := range strings.Split(configuredPath, "/") {
		if segment == "." || segment == ".." {
			return fmt.Errorf("%s path %q must be canonical", name, configuredPath)
		}
	}
	return nil
}

func validateOrigin(origin string) error {
	if origin == "" || origin == "*" || strings.TrimSpace(origin) != origin {
		return fmt.Errorf("GraphQL origin %q is not explicit", origin)
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("GraphQL origin %q must contain only an http(s) scheme and authority", origin)
	}
	return nil
}

func originAllowed(origin string, allowedOrigins []string) bool {
	if origin == "" {
		return true
	}
	for _, allowed := range allowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func graphqlHTTPMiddleware(next http.Handler, config *ServerConfig) http.Handler {
	authenticated := authenticatedMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, config.MaxRequestBodyBytes+1))
		if err != nil {
			http.Error(w, "unable to read GraphQL request", http.StatusBadRequest)
			return
		}
		if int64(len(body)) > config.MaxRequestBodyBytes {
			http.Error(w, "GraphQL request body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err := validateGraphQLJSONBody(body); err != nil {
			http.Error(w, "invalid GraphQL JSON request", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		if acceptsEventStream(r) {
			if !config.IntegrationSessionStreaming {
				http.Error(w, "Integration Session streaming is unavailable", http.StatusNotFound)
				return
			}
			r = r.WithContext(withIntegrationSessionStream(r.Context()))
		}
		next.ServeHTTP(w, r)
	}), config.Authenticator, config.TrustedNetworkAuthenticator)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST, OPTIONS")
			http.Error(w, "GraphQL requires POST", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.RawQuery != "" {
			http.Error(w, "GraphQL query parameters are not allowed", http.StatusBadRequest)
			return
		}
		if encoding := r.Header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
			http.Error(w, "compressed GraphQL bodies are not supported", http.StatusUnsupportedMediaType)
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			http.Error(w, "GraphQL requires application/json", http.StatusUnsupportedMediaType)
			return
		}
		authenticated.ServeHTTP(w, r)
	})
}

func acceptsEventStream(request *http.Request) bool {
	return strings.Contains(strings.ToLower(request.Header.Get("Accept")), "text/event-stream")
}

type graphQLJSONEnvelope struct {
	Query         json.RawMessage `json:"query"`
	OperationName json.RawMessage `json:"operationName"`
	Variables     json.RawMessage `json:"variables"`
	Extensions    json.RawMessage `json:"extensions"`
}

func validateGraphQLJSONBody(body []byte) error {
	if err := validateGraphQLJSONStructure(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var envelope graphQLJSONEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	if !isJSONString(envelope.Query, false) {
		return fmt.Errorf("query must be a string")
	}
	var query string
	if err := json.Unmarshal(envelope.Query, &query); err != nil {
		return err
	}
	if _, err := parser.ParseQuery(&ast.Source{Input: query}); err != nil {
		return err
	}
	if len(envelope.OperationName) > 0 && !isJSONString(envelope.OperationName, true) {
		return fmt.Errorf("operationName must be a string or null")
	}
	if len(envelope.Variables) > 0 && !isJSONObject(envelope.Variables, true) {
		return fmt.Errorf("variables must be an object or null")
	}
	if len(envelope.Extensions) > 0 && !isJSONObject(envelope.Extensions, true) {
		return fmt.Errorf("extensions must be an object or null")
	}
	return nil
}

func validateGraphQLJSONStructure(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '{' {
		return fmt.Errorf("GraphQL JSON request must be an object")
	}
	if err := consumeJSONObject(decoder, true); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON token %v", token)
		}
		return err
	}
	return nil
}

func consumeJSONObject(decoder *json.Decoder, topLevel bool) error {
	allowedTopLevel := map[string]struct{}{
		"query": {}, "operationName": {}, "variables": {}, "extensions": {},
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("JSON object member name must be a string")
		}
		if topLevel {
			if _, allowed := allowedTopLevel[key]; !allowed {
				return fmt.Errorf("unsupported GraphQL JSON member %q", key)
			}
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate JSON member %q", key)
		}
		seen[key] = struct{}{}
		if err := consumeJSONValue(decoder); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return fmt.Errorf("JSON object is not closed")
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '{':
		return consumeJSONObject(decoder, false)
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if end, ok := closing.(json.Delim); !ok || end != ']' {
			return fmt.Errorf("JSON array is not closed")
		}
		return nil
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}

func catalogSafeErrorPresenter(ctx context.Context, err error) *gqlerror.Error {
	presented := gqlgengraphql.DefaultErrorPresenter(ctx, err)

	code, _ := presented.Extensions["code"].(string)
	safeMessageByCode := map[string]string{
		"UNAUTHENTICATED":            "authentication required",
		"FORBIDDEN":                  "GraphQL operation forbidden",
		"GRAPHQL_PARSE_FAILED":       "GraphQL request is invalid",
		"GRAPHQL_VALIDATION_FAILED":  "GraphQL request is invalid",
		"QUERY_DEPTH_LIMIT_EXCEEDED": "GraphQL operation exceeds configured limits",
		"COMPLEXITY_LIMIT_EXCEEDED":  "GraphQL operation exceeds configured limits",
	}
	if message, ok := safeMessageByCode[code]; ok {
		return &gqlerror.Error{Message: message, Extensions: map[string]any{"code": code}}
	}

	switch presented.Message {
	case "authentication required",
		"integration preview unavailable",
		"integration preview forbidden",
		"invalid integration preview request",
		"integration preview payload too large",
		"integration preview failed",
		// Operator control-plane outcomes are deliberately catalog-safe: they
		// name the decision, never the inventory. An operator must be able to
		// tell a stale expected version from a spent idempotency key without
		// learning whether an unseen record exists.
		"operator control plane unavailable",
		"operator control-plane action forbidden",
		"invalid operator control-plane request",
		"operator control-plane record not found",
		"delivery attempt is not dead-lettered",
		"operator operation idempotency conflict",
		"integration deployment version conflict",
		"invalid integration deployment transition",
		"operator control-plane request failed":
		return &gqlerror.Error{Message: presented.Message}
	default:
		return &gqlerror.Error{Message: "GraphQL request failed"}
	}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func isJSONString(raw json.RawMessage, nullable bool) bool {
	trimmed := bytes.TrimSpace(raw)
	if nullable && bytes.Equal(trimmed, []byte("null")) {
		return true
	}
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false
	}
	var value string
	return json.Unmarshal(trimmed, &value) == nil
}

func isJSONObject(raw json.RawMessage, nullable bool) bool {
	trimmed := bytes.TrimSpace(raw)
	if nullable && bytes.Equal(trimmed, []byte("null")) {
		return true
	}
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	var value map[string]json.RawMessage
	return json.Unmarshal(trimmed, &value) == nil
}

func authenticatedMiddleware(next http.Handler, authenticator requestsecurity.Authenticator, trusted *requestsecurity.TrustedNetworkAuthenticator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if security, ok := trusted.AuthenticateRequest(r); ok {
			next.ServeHTTP(w, r.WithContext(requestsecurity.WithSecurityContext(r.Context(), security)))
			return
		}
		security, err := authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="fi-fhir"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(requestsecurity.WithSecurityContext(r.Context(), security)))
	})
}
