package graphql

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gorilla/websocket"
	"github.com/vektah/gqlparser/v2/ast"
)

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
}

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
		AllowedOrigins:    []string{"*"},
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
func NewServer(resolver ResolverRoot, config *ServerConfig) *Server {
	if config == nil {
		config = DefaultServerConfig()
	}

	// Create the GraphQL schema
	schema := NewExecutableSchema(Config{
		Resolvers: resolver,
	})

	// Create the handler with transport configuration
	srv := handler.New(schema)

	// Add transports
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})

	// WebSocket transport for subscriptions
	srv.AddTransport(&transport.Websocket{
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		KeepAlivePingInterval: 30 * time.Second,
	})

	// Add caching for persisted queries (using the auto-generated cache size)
	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	// Add extensions
	if config.Introspection {
		srv.Use(extension.Introspection{})
	}

	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	return &Server{
		config:   config,
		resolver: resolver,
		handler:  srv,
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// GraphQL endpoint
	mux.Handle(s.config.Path, corsMiddleware(s.handler, s.config.AllowedOrigins))

	// WebSocket endpoint (same handler)
	mux.Handle(s.config.WebSocketPath, s.handler)

	// Playground (if enabled)
	if s.config.PlaygroundEnabled {
		mux.Handle(s.config.PlaygroundPath, playground.Handler("fi-fhir GraphQL", s.config.Path))
	}

	// Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"healthy","service":"graphql"}`)
	})

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  s.config.Timeout,
		WriteTimeout: s.config.Timeout,
	}

	log.Printf("GraphQL server listening on http://%s%s", addr, s.config.Path)
	if s.config.PlaygroundEnabled {
		log.Printf("GraphQL Playground available at http://%s%s", addr, s.config.PlaygroundPath)
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

// corsMiddleware adds CORS headers.
func corsMiddleware(next http.Handler, allowedOrigins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := false

		for _, o := range allowedOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else if len(allowedOrigins) > 0 {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigins[0])
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
