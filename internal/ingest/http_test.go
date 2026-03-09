package ingest

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

// stubLogger implements workflow.Logger for tests.
type stubLogger struct{}

func (s *stubLogger) Debug(_ context.Context, _ string, _ ...workflow.Field) {}
func (s *stubLogger) Info(_ context.Context, _ string, _ ...workflow.Field)  {}
func (s *stubLogger) Warn(_ context.Context, _ string, _ ...workflow.Field)  {}
func (s *stubLogger) Error(_ context.Context, _ string, _ ...workflow.Field) {}
func (s *stubLogger) WithFields(_ ...workflow.Field) workflow.Logger         { return s }

func TestHandler_EmptyBody(t *testing.T) {
	h := NewHandler(&stubLogger{}, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(""))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandler_Success_NoEngine(t *testing.T) {
	body := `{"event_type":"test","source":"unit-test"}`
	h := NewHandler(&stubLogger{}, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "accepted") {
		t.Fatalf("expected 'accepted' in response body, got %q", w.Body.String())
	}
}

func TestHandler_BodyTooLarge(t *testing.T) {
	cfg := &HandlerConfig{MaxBodySize: 16} // 16 bytes max
	h := NewHandler(&stubLogger{}, nil, cfg)

	largeBody := strings.Repeat("x", 100) // 100 bytes, exceeds 16
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(largeBody))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", w.Code)
	}
}

func TestHandler_Auth_Rejected(t *testing.T) {
	cfg := &HandlerConfig{
		Auth: &AuthConfig{
			Mode:   AuthBearer,
			Secret: "correct-token",
		},
	}
	h := NewHandler(&stubLogger{}, nil, cfg)

	body := `{"event":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandler_Auth_Bearer_Accepted(t *testing.T) {
	cfg := &HandlerConfig{
		Auth: &AuthConfig{
			Mode:   AuthBearer,
			Secret: "correct-token",
		},
	}
	h := NewHandler(&stubLogger{}, nil, cfg)

	body := `{"event":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer correct-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
}

func TestHandler_Auth_HMAC_Accepted(t *testing.T) {
	secret := "hmac-secret-key"
	body := []byte(`{"event":"test"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	cfg := &HandlerConfig{
		Auth: &AuthConfig{
			Mode:   AuthHMAC,
			Secret: secret,
		},
	}
	h := NewHandler(&stubLogger{}, nil, cfg)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Signature-256", sig)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
}

func TestHandler_Auth_HMAC_Rejected(t *testing.T) {
	cfg := &HandlerConfig{
		Auth: &AuthConfig{
			Mode:   AuthHMAC,
			Secret: "correct-secret",
		},
	}
	h := NewHandler(&stubLogger{}, nil, cfg)

	body := []byte(`{"event":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Signature-256", "sha256=0000000000000000000000000000000000000000000000000000000000000000")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandler_DefaultMaxBodySize(t *testing.T) {
	// Verify default config produces 10MB limit
	var cfg *HandlerConfig
	if cfg.maxBodySize() != DefaultMaxBodySize {
		t.Fatalf("expected %d, got %d", DefaultMaxBodySize, cfg.maxBodySize())
	}

	cfg = &HandlerConfig{MaxBodySize: 0}
	if cfg.maxBodySize() != DefaultMaxBodySize {
		t.Fatalf("expected default %d for zero MaxBodySize, got %d", DefaultMaxBodySize, cfg.maxBodySize())
	}
}

func TestHandler_EventMetaParsing(t *testing.T) {
	body := `{"event_type":"patient_update","source":"ehr-system","meta":{"id":"ext-123"}}`
	h := NewHandler(&stubLogger{}, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
}

func TestHandler_XSourceSystemHeader(t *testing.T) {
	body := `{"data":"value"}`
	h := NewHandler(&stubLogger{}, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Source-System", "athena-health")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
}

func TestHandler_ReadError(t *testing.T) {
	h := NewHandler(&stubLogger{}, nil, nil)

	// Use a reader that immediately fails
	req := httptest.NewRequest(http.MethodPost, "/webhook", &errReader{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// errReader is an io.ReadCloser that always returns an error.
type errReader struct{}

func (e *errReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func (e *errReader) Close() error { return nil }
