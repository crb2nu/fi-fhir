//go:build integration

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
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
		t.Fatalf("setupTestInfra: %v", sharedInfraErr)
	}

	return sharedInfra
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
	if ep := os.Getenv("MINIO_ENDPOINT"); ep != "" {
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
	os.Setenv("MINIO_ENDPOINT", infra.MinioEndpoint)
	os.Setenv("MINIO_ACCESS_KEY", infra.MinioAccess)
	os.Setenv("MINIO_SECRET_KEY", infra.MinioSecret)
	os.Setenv("MINIO_BUCKET", infra.MinioBucket)
	os.Setenv("MINIO_USE_SSL", "false")

	// Also set the FI_FHIR_MINIO_* variants used by cli_integration_test.go helpers.
	os.Setenv("FI_FHIR_MINIO_ENDPOINT", infra.MinioEndpoint)
	os.Setenv("FI_FHIR_MINIO_ACCESS_KEY", infra.MinioAccess)
	os.Setenv("FI_FHIR_MINIO_SECRET_KEY", infra.MinioSecret)

	// Create the MinIO bucket used by tests (best-effort, may already exist in CI).
	createMinioBucket(infra)

	return infra, nil
}

// startPostgresContainer starts a PostgreSQL testcontainer.
// Mirrors the pattern from pkg/eventsourcing/postgres_integration_test.go:56-121.
func startPostgresContainer(t *testing.T) (string, error) {
	t.Helper()

	// Docker availability check (skip in CI where Docker is always available)
	if os.Getenv("CI") == "" && os.Getenv("DOCKER_HOST") == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			Started: false,
			ContainerRequest: testcontainers.ContainerRequest{
				Image: "alpine:latest",
			},
		})
		if err != nil {
			t.Skip("Docker not available, skipping integration test")
			return "", fmt.Errorf("docker not available")
		}
	}

	ctx := context.Background()

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
		return "", fmt.Errorf("start postgres: %w", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
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

	ctx := context.Background()

	container, err := minio.Run(ctx, "minio/minio:latest")
	if err != nil {
		return "", "", "", fmt.Errorf("start minio: %w", err)
	}

	ep, err := container.ConnectionString(ctx)
	if err != nil {
		container.Terminate(ctx)
		return "", "", "", fmt.Errorf("connection string: %w", err)
	}

	return ep, container.Username, container.Password, nil
}

// createMinioBucket creates the test bucket in MinIO (best-effort).
// Uses the minio-go client already in the project's dependency tree.
func createMinioBucket(infra *TestInfra) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := minioClient.New(infra.MinioEndpoint, &minioClient.Options{
		Creds:  credentials.NewStaticV4(infra.MinioAccess, infra.MinioSecret, ""),
		Secure: false,
	})
	if err != nil {
		return // best-effort
	}

	exists, err := client.BucketExists(ctx, infra.MinioBucket)
	if err != nil || exists {
		return
	}
	_ = client.MakeBucket(ctx, infra.MinioBucket, minioClient.MakeBucketOptions{})
}
