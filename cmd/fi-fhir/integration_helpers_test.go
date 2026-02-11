//go:build integration

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	minioClient "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/minio"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestInfra holds connection details for integration tests.
// Created once per test run via setupTestInfra().
type TestInfra struct {
	DatabaseURL   string
	MinioEndpoint string
	MinioAccess   string
	MinioSecret   string
	MinioBucket   string
}

var (
	sharedInfra     *TestInfra
	sharedInfraOnce sync.Once
	sharedInfraErr  error
)

var errDockerNotAvailable = errors.New("docker not available")

// setupTestInfra provisions PostgreSQL and MinIO for integration tests.
//
// Strategy (follows pkg/eventsourcing/postgres_integration_test.go pattern):
//  1. If FI_FHIR_DATABASE_URL is set, use it (CI service containers / manual .env).
//  2. If MINIO_ENDPOINT is set, use it.
//  3. If either is missing, start testcontainers (requires Docker).
//
// Env vars are set via os.Setenv so that production code (which reads
// os.Getenv("MINIO_ENDPOINT") etc.) picks them up without modification.
//
// The containers are started exactly once per test binary via sync.Once
// and cleaned up when the test process exits (testcontainers reaper).
func setupTestInfra(t *testing.T) *TestInfra {
	t.Helper()

	sharedInfraOnce.Do(func() {
		sharedInfra, sharedInfraErr = provisionInfra(t)
	})

	if sharedInfraErr != nil {
		if errors.Is(sharedInfraErr, errDockerNotAvailable) {
			t.Skipf("setupTestInfra: %v", sharedInfraErr)
		}
		t.Fatalf("setupTestInfra: %v", sharedInfraErr)
	}

	return sharedInfra
}

// getDatabaseURL is a small compatibility shim used by older integration tests.
// It ensures DB-backed integration tests consistently share the same infra
// provisioning (and skip behavior when Docker is unavailable).
func getDatabaseURL(t *testing.T) string {
	t.Helper()
	return setupTestInfra(t).DatabaseURL
}

// requireEnv ensures required environment variables are set or skips the test.
// This intentionally does not load .env files; integration tests should
// provision infra via setupTestInfra() or set env vars externally.
func requireEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if os.Getenv(k) == "" {
			t.Skipf("skipping: %s not set", k)
		}
	}
}

func provisionInfra(t *testing.T) (*TestInfra, error) {
	t.Helper()

	infra := &TestInfra{
		MinioBucket: "terminology",
	}

	// --- PostgreSQL ---
	if dbURL := os.Getenv("FI_FHIR_DATABASE_URL"); dbURL != "" {
		infra.DatabaseURL = dbURL
	} else {
		dsn, err := startPostgresContainer(t)
		if err != nil {
			return nil, fmt.Errorf("postgres container: %w", err)
		}
		infra.DatabaseURL = dsn
	}

	// --- MinIO ---
	// Prefer FI_FHIR_MINIO_* overrides for integration (avoid picking up unrelated
	// developer env vars like MINIO_ENDPOINT which may include a scheme).
	if ep := os.Getenv("FI_FHIR_MINIO_ENDPOINT"); ep != "" {
		infra.MinioEndpoint = ep
		infra.MinioAccess = os.Getenv("FI_FHIR_MINIO_ACCESS_KEY")
		infra.MinioSecret = os.Getenv("FI_FHIR_MINIO_SECRET_KEY")
		if b := os.Getenv("FI_FHIR_MINIO_BUCKET"); b != "" {
			infra.MinioBucket = b
		}
	} else if ep := os.Getenv("MINIO_ENDPOINT"); ep != "" {
		infra.MinioEndpoint = ep
		infra.MinioAccess = os.Getenv("MINIO_ACCESS_KEY")
		infra.MinioSecret = os.Getenv("MINIO_SECRET_KEY")
		if b := os.Getenv("MINIO_BUCKET"); b != "" {
			infra.MinioBucket = b
		}
	} else {
		ep, user, pass, err := startMinioContainer(t)
		if err != nil {
			return nil, fmt.Errorf("minio container: %w", err)
		}
		infra.MinioEndpoint = ep
		infra.MinioAccess = user
		infra.MinioSecret = pass
	}

	// Inject env vars so production code and existing helpers pick them up.
	os.Setenv("FI_FHIR_DATABASE_URL", infra.DatabaseURL)
	os.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", infra.DatabaseURL)
	minioEndpoint, minioSecure := normalizeMinIOEndpoint(infra.MinioEndpoint)
	infra.MinioEndpoint = minioEndpoint
	os.Setenv("MINIO_ENDPOINT", infra.MinioEndpoint)
	os.Setenv("MINIO_ACCESS_KEY", infra.MinioAccess)
	os.Setenv("MINIO_SECRET_KEY", infra.MinioSecret)
	os.Setenv("MINIO_BUCKET", infra.MinioBucket)
	if minioSecure {
		os.Setenv("MINIO_USE_SSL", "true")
	} else {
		os.Setenv("MINIO_USE_SSL", "false")
	}

	// Also set the FI_FHIR_MINIO_* variants used by cli_integration_test.go helpers.
	os.Setenv("FI_FHIR_MINIO_ENDPOINT", infra.MinioEndpoint)
	os.Setenv("FI_FHIR_MINIO_ACCESS_KEY", infra.MinioAccess)
	os.Setenv("FI_FHIR_MINIO_SECRET_KEY", infra.MinioSecret)
	os.Setenv("FI_FHIR_MINIO_BUCKET", infra.MinioBucket)

	// CI service containers can take a few seconds to become ready; wait here to
	// reduce integration test flake.
	if err := waitForPostgres(infra.DatabaseURL, 30*time.Second); err != nil {
		return nil, fmt.Errorf("postgres not ready: %w", err)
	}
	if err := ensureMinioBucket(infra, 30*time.Second); err != nil {
		return nil, fmt.Errorf("minio not ready: %w", err)
	}

	// Create the MinIO bucket used by tests (best-effort, may already exist in CI).
	// (ensureMinioBucket already did this)

	return infra, nil
}

func normalizeMinIOEndpoint(endpoint string) (hostPort string, secure bool) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", false
	}

	// minio-go expects host:port (no scheme). Some environments use URL-ish env vars.
	if strings.Contains(endpoint, "://") {
		u, err := url.Parse(endpoint)
		if err == nil && u.Host != "" {
			return u.Host, u.Scheme == "https"
		}
	}
	return endpoint, false
}

func waitForPostgres(dsn string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		db, err := sql.Open("postgres", dsn)
		if err == nil {
			err = db.PingContext(ctx)
			_ = db.Close()
		}
		cancel()

		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}

	if lastErr == nil {
		lastErr = errors.New("timeout waiting for postgres")
	}
	return lastErr
}

func ensureMinioBucket(infra *TestInfra, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		client, err := minioClient.New(infra.MinioEndpoint, &minioClient.Options{
			Creds:  credentials.NewStaticV4(infra.MinioAccess, infra.MinioSecret, ""),
			Secure: strings.ToLower(os.Getenv("MINIO_USE_SSL")) == "true",
		})
		if err == nil {
			exists, existsErr := client.BucketExists(ctx, infra.MinioBucket)
			if existsErr == nil && exists {
				cancel()
				return nil
			}
			if existsErr == nil && !exists {
				err = client.MakeBucket(ctx, infra.MinioBucket, minioClient.MakeBucketOptions{})
				if err == nil {
					cancel()
					return nil
				}
				errResp := minioClient.ToErrorResponse(err)
				if errResp.Code == "BucketAlreadyOwnedByYou" || errResp.Code == "BucketAlreadyExists" {
					cancel()
					return nil
				}
			} else if existsErr != nil {
				err = existsErr
			}
		}

		cancel()

		if err == nil {
			// Shouldn't happen, but avoid infinite loop.
			return nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}

	if lastErr == nil {
		lastErr = errors.New("timeout waiting for minio")
	}
	return lastErr
}

// startPostgresContainer starts a PostgreSQL testcontainer.
// Mirrors the pattern from pkg/eventsourcing/postgres_integration_test.go:56-121.
func startPostgresContainer(t *testing.T) (connStr string, err error) {
	t.Helper()

	// testcontainers-go may panic when Docker is not configured; convert to a
	// regular error so callers can decide whether to skip or fail.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", errDockerNotAvailable, r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("fi_fhir_test"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		if os.Getenv("CI") != "" {
			return "", fmt.Errorf("start postgres in CI: %w", err)
		}
		return "", fmt.Errorf("%w: start postgres: %w", errDockerNotAvailable, err)
	}

	connStr, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		container.Terminate(ctx)
		return "", fmt.Errorf("connection string: %w", err)
	}

	// Verify connectivity
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		container.Terminate(ctx)
		return "", fmt.Errorf("sql.Open: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		container.Terminate(ctx)
		return "", fmt.Errorf("ping: %w", err)
	}
	db.Close()

	// No explicit t.Cleanup — testcontainers reaper handles container lifecycle.
	// The reaper (Ryuk) automatically cleans up containers after the test process exits.

	return connStr, nil
}

// startMinioContainer starts a MinIO testcontainer and returns endpoint, user, password.
func startMinioContainer(t *testing.T) (endpoint, user, password string, err error) {
	t.Helper()

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", errDockerNotAvailable, r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := minio.Run(ctx, "minio/minio:latest")
	if err != nil {
		if os.Getenv("CI") != "" {
			return "", "", "", fmt.Errorf("start minio in CI: %w", err)
		}
		return "", "", "", fmt.Errorf("%w: start minio: %w", errDockerNotAvailable, err)
	}

	ep, err := container.ConnectionString(ctx)
	if err != nil {
		container.Terminate(ctx)
		return "", "", "", fmt.Errorf("connection string: %w", err)
	}

	return ep, container.Username, container.Password, nil
}
