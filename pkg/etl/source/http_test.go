package source

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestHTTPSource_Name(t *testing.T) {
	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test-source",
		URLs: map[string]string{
			"v1": "http://example.com/v1",
		},
	})

	if got := src.Name(); got != "test-source" {
		t.Errorf("Name() = %q, want %q", got, "test-source")
	}
}

func TestHTTPSource_AvailableVersions(t *testing.T) {
	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{
			"v1":     "http://example.com/v1",
			"v2":     "http://example.com/v2",
			"latest": "http://example.com/latest",
		},
	})

	versions, err := src.AvailableVersions(context.Background())
	if err != nil {
		t.Fatalf("AvailableVersions() error = %v", err)
	}

	if len(versions) != 3 {
		t.Errorf("AvailableVersions() returned %d versions, want 3", len(versions))
	}

	// Check that at least one is marked as latest
	hasLatest := false
	for _, v := range versions {
		if v.IsLatest {
			hasLatest = true
			break
		}
	}
	if !hasLatest {
		t.Error("AvailableVersions() should mark one version as latest")
	}
}

func TestHTTPSource_Download(t *testing.T) {
	content := "test content for download"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("Missing User-Agent header")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{
			"v1": server.URL,
		},
	})

	var buf bytes.Buffer
	n, err := src.Download(context.Background(), "v1", &buf)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	if n != int64(len(content)) {
		t.Errorf("Download() returned %d bytes, want %d", n, len(content))
	}

	if buf.String() != content {
		t.Errorf("Download() content = %q, want %q", buf.String(), content)
	}
}

func TestHTTPSource_Download_UnknownVersion(t *testing.T) {
	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{
			"v1": "http://example.com",
		},
	})

	var buf bytes.Buffer
	_, err := src.Download(context.Background(), "v2", &buf)
	if err == nil {
		t.Error("Download() should return error for unknown version")
	}
}

func TestHTTPSource_Download_WithBasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "testuser" || pass != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("authenticated"))
	}))
	defer server.Close()

	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{
			"v1": server.URL,
		},
		Auth: &HTTPAuth{
			Type:     "basic",
			Username: "testuser",
			Password: "testpass",
		},
	})

	var buf bytes.Buffer
	_, err := src.Download(context.Background(), "v1", &buf)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	if buf.String() != "authenticated" {
		t.Errorf("Download() with auth failed, got %q", buf.String())
	}
}

func TestHTTPSource_Download_WithBearerAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer my-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bearer-authenticated"))
	}))
	defer server.Close()

	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{
			"v1": server.URL,
		},
		Auth: &HTTPAuth{
			Type:  "bearer",
			Token: "my-token",
		},
	})

	var buf bytes.Buffer
	_, err := src.Download(context.Background(), "v1", &buf)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	if buf.String() != "bearer-authenticated" {
		t.Errorf("Download() with bearer auth failed")
	}
}

func TestHTTPSource_Download_WithAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key != "secret-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("api-key-authenticated"))
	}))
	defer server.Close()

	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{
			"v1": server.URL,
		},
		Auth: &HTTPAuth{
			Type:   "api_key",
			APIKey: "secret-key",
		},
	})

	var buf bytes.Buffer
	_, err := src.Download(context.Background(), "v1", &buf)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	if buf.String() != "api-key-authenticated" {
		t.Errorf("Download() with API key failed")
	}
}

func TestHTTPSource_Validate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("Validate() should use HEAD request, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{
			"v1": server.URL,
		},
	})

	if err := src.Validate(context.Background()); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestHTTPSource_Validate_NoURLs(t *testing.T) {
	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{},
	})

	if err := src.Validate(context.Background()); err == nil {
		t.Error("Validate() should return error when no URLs configured")
	}
}

func TestHTTPSource_AddVersion(t *testing.T) {
	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
	})

	src.AddVersion("v3", "http://example.com/v3")

	versions, _ := src.AvailableVersions(context.Background())
	found := false
	for _, v := range versions {
		if v.Version == "v3" {
			found = true
			break
		}
	}

	if !found {
		t.Error("AddVersion() did not add the version")
	}
}

func TestHTTPSource_Validate_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{"v1": server.URL},
	})

	err := src.Validate(context.Background())
	if err == nil {
		t.Error("Validate() should error on 403 response")
	}
}

func TestHTTPSource_Validate_WithAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer my-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{"v1": server.URL},
		Auth: &HTTPAuth{Type: "bearer", Token: "my-tok"},
	})

	if err := src.Validate(context.Background()); err != nil {
		t.Errorf("Validate() with auth error = %v", err)
	}
}

func TestHTTPSource_Download_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{"v1": server.URL},
	})

	var buf bytes.Buffer
	_, err := src.Download(context.Background(), "v1", &buf)
	if err == nil {
		t.Error("Download() should error on 500 response")
	}
}

func TestHTTPSource_Download_CustomAPIKeyHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Key") != "my-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{"v1": server.URL},
		Auth: &HTTPAuth{
			Type:   "api_key",
			APIKey: "my-key",
			Header: "X-Custom-Key",
		},
	})

	var buf bytes.Buffer
	_, err := src.Download(context.Background(), "v1", &buf)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if buf.String() != "ok" {
		t.Errorf("content = %q, want %q", buf.String(), "ok")
	}
}

func TestHTTPSource_NewDefaults(t *testing.T) {
	src := NewHTTPSource(HTTPSourceConfig{Name: "test"})
	if src.userAgent != "fi-fhir-etl/1.0" {
		t.Errorf("userAgent = %q, want default", src.userAgent)
	}
}

func TestHTTPSource_CustomUserAgent(t *testing.T) {
	src := NewHTTPSource(HTTPSourceConfig{
		Name:      "test",
		UserAgent: "custom/2.0",
	})
	if src.userAgent != "custom/2.0" {
		t.Errorf("userAgent = %q, want %q", src.userAgent, "custom/2.0")
	}
}

func TestHTTPSource_Validate_WithBasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "u" || pass != "p" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{"v1": server.URL},
		Auth: &HTTPAuth{Type: "basic", Username: "u", Password: "p"},
	})

	if err := src.Validate(context.Background()); err != nil {
		t.Errorf("Validate() with basic auth error = %v", err)
	}
}

func TestHTTPSource_Validate_WithAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{"v1": server.URL},
		Auth: &HTTPAuth{Type: "api_key", APIKey: "secret"},
	})

	if err := src.Validate(context.Background()); err != nil {
		t.Errorf("Validate() with API key error = %v", err)
	}
}

func TestHTTPSource_Validate_Unreachable(t *testing.T) {
	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{"v1": "http://127.0.0.1:1"}, // Unlikely to be listening.
	})

	err := src.Validate(context.Background())
	if err == nil {
		t.Error("Validate() should error for unreachable URL")
	}
}

func TestHTTPSource_Download_Unreachable(t *testing.T) {
	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{"v1": "http://127.0.0.1:1"}, // Unlikely to be listening.
	})

	var buf bytes.Buffer
	_, err := src.Download(context.Background(), "v1", &buf)
	if err == nil {
		t.Error("Download() should error for unreachable server")
	}
}

// failWriter always returns an error on Write.
type failWriter struct{}

func (w *failWriter) Write(p []byte) (int, error) {
	return 0, os.ErrPermission
}

func TestHTTPSource_Download_WriterError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response data"))
	}))
	defer server.Close()

	src := NewHTTPSource(HTTPSourceConfig{
		Name: "test",
		URLs: map[string]string{"v1": server.URL},
	})

	_, err := src.Download(context.Background(), "v1", &failWriter{})
	if err == nil {
		t.Error("expected error from failing writer")
	}
}

func TestHTTPSource_AvailableVersions_Empty(t *testing.T) {
	src := NewHTTPSource(HTTPSourceConfig{Name: "test"})
	versions, err := src.AvailableVersions(context.Background())
	if err != nil {
		t.Fatalf("AvailableVersions() error = %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("got %d versions, want 0", len(versions))
	}
}
