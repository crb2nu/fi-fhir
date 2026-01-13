//go:build !short

package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	urlpkg "net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq" // Postgres driver for integration tests

	"github.com/crb2nu/fi-fhir/pkg/storage"
)

// requireEnv ensures required environment variables are set or skips the test.
func requireEnv(t *testing.T, keys ...string) {
	t.Helper()
	loadDotEnv()
	for _, k := range keys {
		if os.Getenv(k) == "" {
			t.Skipf("skipping: %s not set", k)
		}
	}
}

var loadEnvOnce sync.Once

// loadDotEnv populates environment variables from a local .env if present.
func loadDotEnv() {
	loadEnvOnce.Do(func() {
		data, err := os.ReadFile(".env")
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			_ = os.Setenv(key, val)
		}
	})
}

func TestIntegration_MinIO_Live(t *testing.T) {
	requireEnv(t, "MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY")

	cfg := storage.MinIOConfig{
		Endpoint:        os.Getenv("MINIO_ENDPOINT"),
		AccessKeyID:     os.Getenv("MINIO_ACCESS_KEY"),
		SecretAccessKey: os.Getenv("MINIO_SECRET_KEY"),
		UseSSL:          strings.ToLower(os.Getenv("MINIO_USE_SSL")) == "true",
		DefaultBucket:   getEnvOrDefault("MINIO_BUCKET", "terminology"),
	}

	provider, err := storage.NewMinIOProvider(cfg)
	if err != nil {
		t.Fatalf("failed to create MinIO provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Use a unique object path to avoid collisions.
	objectPath := fmt.Sprintf("fi-fhir-integration/%d.txt", time.Now().UnixNano())
	content := "hello-minio"

	if err := provider.Put(ctx, objectPath, strings.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("put failed: %v", err)
	}

	exists, err := provider.Exists(ctx, objectPath)
	if err != nil {
		t.Fatalf("exists check failed: %v", err)
	}
	if !exists {
		t.Fatalf("expected object to exist after upload")
	}

	if err := provider.Delete(ctx, objectPath); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	exists, err = provider.Exists(ctx, objectPath)
	if err != nil {
		t.Fatalf("exists check after delete failed: %v", err)
	}
	if exists {
		t.Fatalf("expected object to be deleted")
	}
}

func TestIntegration_Postgres_Live(t *testing.T) {
	requireEnv(t, "FI_FHIR_DATABASE_URL")

	url := os.Getenv("FI_FHIR_DATABASE_URL")
	if url == "" {
		t.Skip("skipping: FI_FHIR_DATABASE_URL not set")
	}

	parsed, err := urlpkg.Parse(url)
	if err != nil {
		t.Fatalf("invalid FI_FHIR_DATABASE_URL: %v", err)
	}
	hostPort := parsed.Host
	if !strings.Contains(hostPort, ":") {
		hostPort += ":5432"
	}
	if _, err := net.DialTimeout("tcp", hostPort, 5*time.Second); err != nil {
		t.Skipf("skipping: postgres host %s unreachable: %v", hostPort, err)
	}

	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var one int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("ping query failed: %v", err)
	}
	if one != 1 {
		t.Fatalf("unexpected result from SELECT 1: %d", one)
	}
}
