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
			fmt.Fprint(w, "ST-abc")
		case downloadPath:
			if ticket := r.URL.Query().Get("ticket"); ticket != "ST-abc" {
				http.Error(w, "missing ticket", http.StatusUnauthorized)
				return
			}
			downloadCalled = true
			fmt.Fprint(w, "content")
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
