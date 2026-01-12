// Package source provides data source connectors for the ETL pipeline.
package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/crb2nu/fi-fhir/pkg/etl"
)

// HTTPSource downloads data from HTTP/HTTPS URLs.
type HTTPSource struct {
	name      string
	urls      map[string]string // version -> URL mapping
	client    *http.Client
	auth      *HTTPAuth
	userAgent string
}

// HTTPAuth holds authentication credentials.
type HTTPAuth struct {
	Type     string // "basic", "bearer", "api_key"
	Username string
	Password string
	Token    string
	APIKey   string
	Header   string // Header name for API key
}

// HTTPSourceConfig configures the HTTP source.
type HTTPSourceConfig struct {
	Name      string
	URLs      map[string]string // Version to URL mapping
	Auth      *HTTPAuth
	UserAgent string
	Timeout   time.Duration
}

// NewHTTPSource creates a new HTTP source.
func NewHTTPSource(cfg HTTPSourceConfig) *HTTPSource {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}

	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = "fi-fhir-etl/1.0"
	}

	return &HTTPSource{
		name:      cfg.Name,
		urls:      cfg.URLs,
		auth:      cfg.Auth,
		userAgent: userAgent,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Name returns the source identifier.
func (s *HTTPSource) Name() string {
	return s.name
}

// AvailableVersions returns configured versions.
func (s *HTTPSource) AvailableVersions(ctx context.Context) ([]etl.VersionInfo, error) {
	versions := make([]etl.VersionInfo, 0, len(s.urls))

	for version, url := range s.urls {
		info := etl.VersionInfo{
			Version:     version,
			DownloadURL: url,
		}
		versions = append(versions, info)
	}

	// Mark the first one as latest if we have any
	if len(versions) > 0 {
		versions[0].IsLatest = true
	}

	return versions, nil
}

// Download streams data from the URL.
func (s *HTTPSource) Download(ctx context.Context, version string, w io.Writer) (int64, error) {
	url, ok := s.urls[version]
	if !ok {
		return 0, fmt.Errorf("unknown version %q for source %s", version, s.name)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", s.userAgent)

	// Apply authentication
	if s.auth != nil {
		switch s.auth.Type {
		case "basic":
			req.SetBasicAuth(s.auth.Username, s.auth.Password)
		case "bearer":
			req.Header.Set("Authorization", "Bearer "+s.auth.Token)
		case "api_key":
			header := s.auth.Header
			if header == "" {
				header = "X-API-Key"
			}
			req.Header.Set(header, s.auth.APIKey)
		}
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return n, fmt.Errorf("failed to read response body: %w", err)
	}

	return n, nil
}

// Validate checks if the source URLs are accessible.
func (s *HTTPSource) Validate(ctx context.Context) error {
	// Try a HEAD request on the first URL
	for version, url := range s.urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request for %s: %w", version, err)
		}

		req.Header.Set("User-Agent", s.userAgent)

		if s.auth != nil {
			switch s.auth.Type {
			case "basic":
				req.SetBasicAuth(s.auth.Username, s.auth.Password)
			case "bearer":
				req.Header.Set("Authorization", "Bearer "+s.auth.Token)
			case "api_key":
				header := s.auth.Header
				if header == "" {
					header = "X-API-Key"
				}
				req.Header.Set(header, s.auth.APIKey)
			}
		}

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to reach %s: %w", url, err)
		}
		_ = resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
		}

		return nil // Just check the first one
	}

	return fmt.Errorf("no URLs configured for source %s", s.name)
}

// AddVersion adds or updates a version URL.
func (s *HTTPSource) AddVersion(version, url string) {
	if s.urls == nil {
		s.urls = make(map[string]string)
	}
	s.urls[version] = url
}
