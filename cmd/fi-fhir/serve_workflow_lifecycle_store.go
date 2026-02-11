package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/store"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/config"
)

func initWorkflowLifecycleStoreFromEnv(ctx context.Context) (store.WorkflowLifecycleStore, error) {
	cfg := config.LoadFromEnv()

	// Backwards/compat: support FI_FHIR_DATABASE_USER (used by k8s manifests).
	if cfg.Database.Username == "" {
		cfg.Database.Username = os.Getenv("FI_FHIR_DATABASE_USER")
	}

	if cfg.Database.Host == "" || cfg.Database.Database == "" || cfg.Database.Username == "" {
		return nil, nil
	}

	// In-cluster postgres commonly runs without TLS unless explicitly configured.
	if os.Getenv("FI_FHIR_DATABASE_SSL_MODE") == "" {
		cfg.Database.SSLMode = "disable"
	}

	driver := cfg.Database.Driver
	if driver == "" {
		driver = "postgres"
	}

	dsn := cfg.DatabaseDSN()
	if dsn == "" {
		return nil, fmt.Errorf("database DSN is empty (driver=%q)", driver)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	lifecycleStore := store.NewPostgresWorkflowLifecycleStore(db)
	if err := lifecycleStore.InitSchema(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init workflow lifecycle schema: %w", err)
	}

	return lifecycleStore, nil
}
