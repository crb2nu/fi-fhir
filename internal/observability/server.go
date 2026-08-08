package observability

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// MetricsServerConfig configures the second listener that carries `/metrics`.
//
// Metrics live on their own listener because every checked-in deployment
// artifact already says so — the `metrics` containerPort, both Services, the
// Prometheus scrape job, the pod annotations, the compose port mapping, and
// pkg/config.ObservabilityConfig — and because an unauthenticated scrape path
// does not belong on the same socket that accepts raw clinical POSTs.
type MetricsServerConfig struct {
	// Host is the bind address. Empty binds all interfaces.
	Host string
	// Port is the bind port. Must be positive.
	Port int
	// Path is the exposition path. Must be an absolute, non-root path.
	Path string
	// Handler serves Path. Required.
	Handler http.Handler
	// ReadTimeout bounds a scrape request. Zero uses a safe default.
	ReadTimeout time.Duration
}

// MetricsServer is a bound-but-not-yet-serving metrics listener.
//
// Binding happens in NewMetricsServer so a port conflict is a startup error the
// operator sees immediately, rather than a background goroutine failure that
// leaves the process running while every scrape fails.
type MetricsServer struct {
	listener net.Listener
	server   *http.Server
	path     string
}

// NewMetricsServer validates the configuration and binds the listener.
func NewMetricsServer(cfg MetricsServerConfig) (*MetricsServer, error) {
	if cfg.Handler == nil {
		return nil, fmt.Errorf("metrics handler is required")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("metrics port %d is out of range", cfg.Port)
	}
	path := cfg.Path
	if path == "" {
		path = "/metrics"
	}
	if !strings.HasPrefix(path, "/") || path == "/" || strings.HasSuffix(path, "/") {
		return nil, fmt.Errorf("metrics path %q must be an absolute non-root path", path)
	}
	readTimeout := cfg.ReadTimeout
	if readTimeout <= 0 {
		readTimeout = 10 * time.Second
	}

	mux := http.NewServeMux()
	mux.Handle(path, cfg.Handler)
	// Anything else on this listener is a misconfigured scrape, not a route.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == path {
			cfg.Handler.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("bind metrics listener on %s: %w", addr, err)
	}

	return &MetricsServer{
		listener: listener,
		path:     path,
		server: &http.Server{
			Handler:           mux,
			ReadTimeout:       readTimeout,
			ReadHeaderTimeout: readTimeout,
			WriteTimeout:      readTimeout,
		},
	}, nil
}

// Addr reports the bound address, resolved after binding.
func (s *MetricsServer) Addr() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Path reports the served exposition path.
func (s *MetricsServer) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Run serves until ctx is cancelled, then shuts down gracefully.
//
// A normal shutdown returns nil so runServe's component table does not report
// an orderly stop as a component failure.
func (s *MetricsServer) Run(ctx context.Context) error {
	if s == nil || s.server == nil || s.listener == nil {
		return errors.New("metrics server is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.server.Serve(s.listener)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down metrics listener: %w", err)
		}
		<-errCh
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("metrics listener stopped: %w", err)
	}
}

// Close releases the listener without serving. Used when startup fails after
// binding.
func (s *MetricsServer) Close() error {
	if s == nil || s.listener == nil {
		return nil
	}
	return s.listener.Close()
}
