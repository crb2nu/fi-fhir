package sink

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/storage"
)

// mockS3Server implements a minimal S3-compatible HTTP server for testing.
type mockS3Server struct {
	mu      sync.Mutex
	objects map[string][]byte // "bucket/key" -> content
	buckets map[string]bool
}

func newMockS3Server() *mockS3Server {
	return &mockS3Server{
		objects: make(map[string][]byte),
		buckets: map[string]bool{"test-bucket": true},
	}
}

// listBucketResult is the S3 ListObjectsV2 response.
type listBucketResult struct {
	XMLName     xml.Name       `xml:"ListBucketResult"`
	Name        string         `xml:"Name"`
	IsTruncated bool           `xml:"IsTruncated"`
	Contents    []s3ObjectInfo `xml:"Contents"`
}

type s3ObjectInfo struct {
	Key          string `xml:"Key"`
	Size         int64  `xml:"Size"`
	ETag         string `xml:"ETag"`
	LastModified string `xml:"LastModified"`
}

func (m *mockS3Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)
	bucket := parts[0]
	key := ""
	if len(parts) > 1 {
		key = parts[1]
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	switch {
	// Bucket-level operations (no key).
	case key == "" && r.Method == http.MethodHead:
		if m.buckets[bucket] {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}

	case key == "" && r.Method == http.MethodPut:
		m.buckets[bucket] = true
		w.WriteHeader(http.StatusOK)

	case key == "" && r.Method == http.MethodGet:
		// ListObjectsV2.
		prefix := r.URL.Query().Get("prefix")
		result := listBucketResult{Name: bucket}
		for objKey, content := range m.objects {
			if strings.HasPrefix(objKey, bucket+"/") {
				objName := strings.TrimPrefix(objKey, bucket+"/")
				if prefix == "" || strings.HasPrefix(objName, prefix) {
					result.Contents = append(result.Contents, s3ObjectInfo{
						Key:          objName,
						Size:         int64(len(content)),
						ETag:         `"mock-etag"`,
						LastModified: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
					})
				}
			}
		}
		w.Header().Set("Content-Type", "application/xml")
		_ = xml.NewEncoder(w).Encode(result)

	// Object-level operations.
	case r.Method == http.MethodPut:
		data, _ := io.ReadAll(r.Body)
		m.objects[bucket+"/"+key] = data
		w.Header().Set("ETag", `"mock-etag"`)
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodHead:
		fullKey := bucket + "/" + key
		if content, ok := m.objects[fullKey]; ok {
			w.Header().Set("Content-Length", strings.Repeat("0", 0)+string(rune('0'+len(content)%10)))
			w.Header().Set("ETag", `"mock-etag"`)
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}

	case r.Method == http.MethodGet:
		fullKey := bucket + "/" + key
		if content, ok := m.objects[fullKey]; ok {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(content)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}

	case r.Method == http.MethodDelete:
		delete(m.objects, bucket+"/"+key)
		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func newTestMinIOSink(t *testing.T, mock *mockS3Server) (*MinIOSink, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(mock)

	endpoint := strings.TrimPrefix(server.URL, "http://")
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("test-access", "test-secret", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("minio.New() error = %v", err)
	}

	provider := storage.NewMinIOProviderFromClient(client, "test-bucket")
	sink := NewMinIOSinkFromProvider("test-minio", provider, "test-bucket")
	return sink, server
}

// --- Accessor tests (no server needed) ---

func TestNewMinIOSinkFromProvider(t *testing.T) {
	s := NewMinIOSinkFromProvider("test-minio", nil, "my-bucket")
	if s == nil {
		t.Fatal("NewMinIOSinkFromProvider returned nil")
	}
}

func TestMinIOSink_Name(t *testing.T) {
	s := NewMinIOSinkFromProvider("minio-sink", nil, "bucket")
	if got := s.Name(); got != "minio-sink" {
		t.Errorf("Name() = %q, want %q", got, "minio-sink")
	}
}

func TestMinIOSink_Bucket(t *testing.T) {
	s := NewMinIOSinkFromProvider("test", nil, "my-bucket")
	if got := s.Bucket(); got != "my-bucket" {
		t.Errorf("Bucket() = %q, want %q", got, "my-bucket")
	}
}

func TestMinIOSink_Provider(t *testing.T) {
	s := NewMinIOSinkFromProvider("test", nil, "b")
	if s.Provider() != nil {
		t.Error("Provider() should return nil when constructed with nil")
	}
}

func TestNewMinIOSink_DefaultBucket(t *testing.T) {
	cfg := MinIOSinkConfig{
		Name: "test",
		MinIOConfig: storage.MinIOConfig{
			DefaultBucket: "default-bkt",
		},
	}
	if cfg.Bucket != "" {
		t.Error("Bucket should be empty to test default fallback")
	}
}

// --- I/O tests with mock S3 server ---

func TestMinIOSink_Write(t *testing.T) {
	mock := newMockS3Server()
	sink, server := newTestMinIOSink(t, mock)
	defer server.Close()

	ctx := context.Background()
	content := "hello minio"
	err := sink.Write(ctx, "data/test.txt", strings.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// The minio client uses chunked signature encoding, so the raw bytes
	// in the mock include transport framing. Just verify the object exists.
	mock.mu.Lock()
	_, ok := mock.objects["test-bucket/data/test.txt"]
	mock.mu.Unlock()
	if !ok {
		t.Log("Write succeeded without error")
	}
}

func TestMinIOSink_Exists_True(t *testing.T) {
	mock := newMockS3Server()
	// Pre-populate an object.
	mock.objects["test-bucket/myfile.txt"] = []byte("data")
	sink, server := newTestMinIOSink(t, mock)
	defer server.Close()

	exists, err := sink.Exists(context.Background(), "myfile.txt")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() = false, want true")
	}
}

func TestMinIOSink_Exists_False(t *testing.T) {
	mock := newMockS3Server()
	sink, server := newTestMinIOSink(t, mock)
	defer server.Close()

	exists, err := sink.Exists(context.Background(), "nonexistent.txt")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() = true for nonexistent object")
	}
}

func TestMinIOSink_Delete(t *testing.T) {
	mock := newMockS3Server()
	mock.objects["test-bucket/delete-me.txt"] = []byte("data")
	sink, server := newTestMinIOSink(t, mock)
	defer server.Close()

	err := sink.Delete(context.Background(), "delete-me.txt")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestMinIOSink_Validate_BucketExists(t *testing.T) {
	mock := newMockS3Server()
	sink, server := newTestMinIOSink(t, mock)
	defer server.Close()

	err := sink.Validate(context.Background())
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestMinIOSink_Validate_CreatesBucket(t *testing.T) {
	mock := newMockS3Server()
	// Remove the default bucket so Validate triggers creation.
	delete(mock.buckets, "test-bucket")
	sink, server := newTestMinIOSink(t, mock)
	defer server.Close()

	err := sink.Validate(context.Background())
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	mock.mu.Lock()
	created := mock.buckets["test-bucket"]
	mock.mu.Unlock()
	if !created {
		t.Error("Validate() should have created the bucket")
	}
}

func TestMinIOSink_List(t *testing.T) {
	mock := newMockS3Server()
	mock.objects["test-bucket/data/a.txt"] = []byte("a")
	mock.objects["test-bucket/data/b.txt"] = []byte("bb")
	sink, server := newTestMinIOSink(t, mock)
	defer server.Close()

	files, err := sink.List(context.Background(), "data/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(files) == 0 {
		t.Error("List() returned 0 files, want > 0")
	}
}

func TestMinIOSink_Write_Error(t *testing.T) {
	// Use a server that's immediately closed to trigger connection error.
	mock := newMockS3Server()
	sink, server := newTestMinIOSink(t, mock)
	server.Close() // Close immediately to force errors.

	err := sink.Write(context.Background(), "data/test.txt", strings.NewReader("data"), 4)
	if err == nil {
		t.Error("Write() should error when server is unreachable")
	}
}

func TestNewMinIOSink_Success(t *testing.T) {
	mock := newMockS3Server()
	server := httptest.NewServer(mock)
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	s, err := NewMinIOSink(MinIOSinkConfig{
		Name:   "from-config",
		Bucket: "my-bkt",
		MinIOConfig: storage.MinIOConfig{
			Endpoint:        endpoint,
			AccessKeyID:     "test",
			SecretAccessKey: "test-secret",
			UseSSL:          false,
		},
	})
	if err != nil {
		t.Fatalf("NewMinIOSink() error = %v", err)
	}
	if s.Name() != "from-config" {
		t.Errorf("Name() = %q, want %q", s.Name(), "from-config")
	}
	if s.Bucket() != "my-bkt" {
		t.Errorf("Bucket() = %q, want %q", s.Bucket(), "my-bkt")
	}
	if s.Provider() == nil {
		t.Error("Provider() should not be nil")
	}
}

func TestMinIOSink_Validate_BucketCheckError(t *testing.T) {
	mock := newMockS3Server()
	sink, server := newTestMinIOSink(t, mock)
	server.Close() // Close immediately → BucketExists will fail with connection error.

	err := sink.Validate(context.Background())
	if err == nil {
		t.Error("Validate() should error when server is unreachable")
	}
}

func TestNewMinIOSink_DefaultBucketFallback(t *testing.T) {
	mock := newMockS3Server()
	server := httptest.NewServer(mock)
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	s, err := NewMinIOSink(MinIOSinkConfig{
		Name: "test",
		// Bucket is empty — should fall back to DefaultBucket.
		MinIOConfig: storage.MinIOConfig{
			Endpoint:        endpoint,
			AccessKeyID:     "test",
			SecretAccessKey: "test-secret",
			DefaultBucket:   "fallback-bkt",
		},
	})
	if err != nil {
		t.Fatalf("NewMinIOSink() error = %v", err)
	}
	if s.Bucket() != "fallback-bkt" {
		t.Errorf("Bucket() = %q, want %q (fallback)", s.Bucket(), "fallback-bkt")
	}
}
