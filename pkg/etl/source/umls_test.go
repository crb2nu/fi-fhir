package source

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUMLSSource_Download(t *testing.T) {
	tgtPath := "/cas/v1/tickets/TGT-123"
	downloadPath := "/umls/kss/2024AB/umls-2024AB-full.zip"
	var downloadCalled bool

	var server *httptest.Server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cas/v1/api-key":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if r.Form.Get("apikey") != "apikey" {
				http.Error(w, "bad key", http.StatusForbidden)
				return
			}
			w.Header().Set("Location", server.URL+tgtPath)
			w.WriteHeader(http.StatusCreated)
		case tgtPath:
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if service := r.Form.Get("service"); service == "" {
				http.Error(w, "missing service", http.StatusBadRequest)
				return
			}
			_, _ = fmt.Fprint(w, "ST-abc")
		case downloadPath:
			if ticket := r.URL.Query().Get("ticket"); ticket != "ST-abc" {
				http.Error(w, "missing ticket", http.StatusUnauthorized)
				return
			}
			downloadCalled = true
			_, _ = fmt.Fprint(w, "content")
		default:
			http.NotFound(w, r)
		}
	})
	server = httptest.NewServer(handler)
	defer server.Close()

	src := NewUMLSSource(UMLSSourceConfig{
		APIKey:          "apikey",
		BaseLoginURL:    server.URL + "/cas/v1/api-key",
		BaseDownloadURL: server.URL + "/umls/kss",
		HTTPClient:      server.Client(),
		Versions: map[string]string{
			"2024AB": "umls-2024AB-full.zip",
		},
	})

	buf := &bytes.Buffer{}
	n, err := src.Download(context.Background(), "2024AB", buf)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if !downloadCalled {
		t.Fatalf("download endpoint was not called")
	}
	if buf.String() != "content" {
		t.Fatalf("unexpected content: %q", buf.String())
	}
	if n != int64(len("content")) {
		t.Fatalf("unexpected byte count %d", n)
	}
}

func TestUMLSSource_AvailableVersions(t *testing.T) {
	src := NewUMLSSource(UMLSSourceConfig{
		APIKey: "key",
		Versions: map[string]string{
			"2023AA": "umls-2023AA-full.zip",
			"2024AB": "umls-2024AB-full.zip",
		},
	})

	versions, err := src.AvailableVersions(context.Background())
	if err != nil {
		t.Fatalf("AvailableVersions error: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if !versions[1].IsLatest || versions[1].Version != "2024AB" {
		t.Fatalf("expected 2024AB as latest, got %+v", versions[1])
	}
	if !strings.Contains(versions[1].DownloadURL, "2024AB") {
		t.Fatalf("expected download url to include 2024AB, got %s", versions[1].DownloadURL)
	}
}

func TestUMLSSource_Validate_NoKey(t *testing.T) {
	src := NewUMLSSource(UMLSSourceConfig{})
	if err := src.Validate(context.Background()); err == nil {
		t.Fatalf("expected validate to fail without API key")
	}
}

func TestUMLSSource_Name(t *testing.T) {
	src := NewUMLSSource(UMLSSourceConfig{Name: "custom-umls"})
	if got := src.Name(); got != "custom-umls" {
		t.Errorf("Name() = %q, want %q", got, "custom-umls")
	}
}

func TestUMLSSource_Name_Default(t *testing.T) {
	src := NewUMLSSource(UMLSSourceConfig{APIKey: "k"})
	if got := src.Name(); got != "umls" {
		t.Errorf("Name() = %q, want default %q", got, "umls")
	}
}

func TestUMLSSource_Download_UnknownVersion(t *testing.T) {
	src := NewUMLSSource(UMLSSourceConfig{
		APIKey:   "key",
		Versions: map[string]string{"2024AB": "file.zip"},
	})

	var buf bytes.Buffer
	_, err := src.Download(context.Background(), "9999XX", &buf)
	if err == nil {
		t.Error("expected error for unknown version")
	}
	if !strings.Contains(err.Error(), "unknown UMLS version") {
		t.Errorf("error = %q, want to contain 'unknown UMLS version'", err.Error())
	}
}

func TestUMLSSource_Download_EmptyVersionUsesLatest(t *testing.T) {
	tgtPath := "/cas/v1/tickets/TGT-x"

	var server *httptest.Server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cas/v1/api-key":
			w.Header().Set("Location", server.URL+tgtPath)
			w.WriteHeader(http.StatusCreated)
		case tgtPath:
			_, _ = fmt.Fprint(w, "ST-x")
		default:
			// Accept any download path — we just want to verify it works.
			_, _ = fmt.Fprint(w, "downloaded")
		}
	})
	server = httptest.NewServer(handler)
	defer server.Close()

	src := NewUMLSSource(UMLSSourceConfig{
		APIKey:          "key",
		BaseLoginURL:    server.URL + "/cas/v1/api-key",
		BaseDownloadURL: server.URL + "/umls/kss",
		HTTPClient:      server.Client(),
		Versions: map[string]string{
			"2023AA": "a.zip",
			"2024AB": "b.zip",
		},
	})

	var buf bytes.Buffer
	_, err := src.Download(context.Background(), "", &buf)
	if err != nil {
		t.Fatalf("Download('') error = %v", err)
	}
	if buf.String() != "downloaded" {
		t.Errorf("content = %q, want %q", buf.String(), "downloaded")
	}
}

func TestUMLSSource_Validate_WithValidKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://example.com/tgt/TGT-ok")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	src := NewUMLSSource(UMLSSourceConfig{
		APIKey:       "valid-key",
		BaseLoginURL: server.URL,
		HTTPClient:   server.Client(),
	})

	if err := src.Validate(context.Background()); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestUMLSSource_TGT_FailedAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, "bad api key")
	}))
	defer server.Close()

	src := NewUMLSSource(UMLSSourceConfig{
		APIKey:       "bad-key",
		BaseLoginURL: server.URL,
		HTTPClient:   server.Client(),
	})

	if err := src.Validate(context.Background()); err == nil {
		t.Error("expected Validate to fail with bad key")
	}
}

func TestUMLSSource_TGT_BodyFallback(t *testing.T) {
	// When no Location header is set, TGT should be read from body.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "TGT-from-body")
	}))
	defer server.Close()

	src := NewUMLSSource(UMLSSourceConfig{
		APIKey:       "key",
		BaseLoginURL: server.URL,
		HTTPClient:   server.Client(),
	})

	if err := src.Validate(context.Background()); err != nil {
		t.Errorf("Validate() error = %v (TGT from body should work)", err)
	}
}

func TestUMLSSource_TGT_EmptyBody(t *testing.T) {
	// When no Location header and empty body, should fail.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	src := NewUMLSSource(UMLSSourceConfig{
		APIKey:       "key",
		BaseLoginURL: server.URL,
		HTTPClient:   server.Client(),
	})

	if err := src.Validate(context.Background()); err == nil {
		t.Error("expected error for empty TGT response")
	}
}

func TestUMLSSource_ServiceTicket_Failure(t *testing.T) {
	tgtPath := "/cas/v1/tickets/TGT-fail"

	var server *httptest.Server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cas/v1/api-key":
			w.Header().Set("Location", server.URL+tgtPath)
			w.WriteHeader(http.StatusCreated)
		case tgtPath:
			// Return error for service ticket request.
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, "invalid request")
		default:
			http.NotFound(w, r)
		}
	})
	server = httptest.NewServer(handler)
	defer server.Close()

	src := NewUMLSSource(UMLSSourceConfig{
		APIKey:          "key",
		BaseLoginURL:    server.URL + "/cas/v1/api-key",
		BaseDownloadURL: server.URL + "/umls/kss",
		HTTPClient:      server.Client(),
		Versions:        map[string]string{"2024AB": "file.zip"},
	})

	var buf bytes.Buffer
	_, err := src.Download(context.Background(), "2024AB", &buf)
	if err == nil {
		t.Error("expected error from service ticket failure")
	}
	if !strings.Contains(err.Error(), "service ticket") {
		t.Errorf("error = %q, want to mention 'service ticket'", err.Error())
	}
}

func TestPickLatestVersion(t *testing.T) {
	tests := []struct {
		name     string
		versions map[string]string
		want     string
	}{
		{"empty", map[string]string{}, ""},
		{"single", map[string]string{"2024AB": "f.zip"}, "2024AB"},
		{"multiple", map[string]string{
			"2023AA": "a.zip",
			"2024AB": "b.zip",
			"2022AA": "c.zip",
		}, "2024AB"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pickLatestVersion(tc.versions)
			if got != tc.want {
				t.Errorf("pickLatestVersion() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewUMLSSource_Defaults(t *testing.T) {
	src := NewUMLSSource(UMLSSourceConfig{APIKey: "k"})

	if src.baseLoginURL == "" {
		t.Error("baseLoginURL should have a default")
	}
	if src.baseDownloadURL == "" {
		t.Error("baseDownloadURL should have a default")
	}
	if len(src.versions) == 0 {
		t.Error("versions should have defaults")
	}
	if src.latestVersion == "" {
		t.Error("latestVersion should be set")
	}
}
