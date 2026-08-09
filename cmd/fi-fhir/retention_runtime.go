package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	integrationretention "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/retention"
	integrationsession "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
)

const (
	retentionPolicyPathEnv  = "FI_FHIR_RETENTION_POLICY_PATH"
	retentionIntervalEnv    = "FI_FHIR_RETENTION_PURGE_INTERVAL"
	retentionBatchSizeEnv   = "FI_FHIR_RETENTION_PURGE_BATCH_SIZE"
	defaultRetentionCadence = time.Hour
	defaultRetentionBatch   = 200
)

// loadRetentionPurgerFromEnv builds the Slice 4.1e retention purge component.
//
// It is fail-closed at every step. No FI_FHIR_RETENTION_POLICY_PATH means no
// component, no policy record, and nothing purged — an unconfigured deployment
// must never destroy clinical data. A malformed document is a startup failure
// rather than a silently disabled purge, because a retention control that
// quietly does nothing is the failure mode this slice exists to remove.
//
// The policy document is loaded the way the destination registry is: a
// deployment-supplied file, read once at startup, upserted into the durable
// per-tenant record with a version bump and an append-only audit row. Restarting
// with an unchanged document is not a policy change and mints no version.
func loadRetentionPurgerFromEnv(
	ctx context.Context,
	db *sql.DB,
	tenantID string,
	observe func(integrationretention.PurgeResult, error),
) (*integrationretention.Purger, *integrationretention.Policy, error) {
	path := os.Getenv(retentionPolicyPathEnv)
	if path == "" {
		return nil, nil, nil
	}
	if ctx == nil {
		return nil, nil, fmt.Errorf("retention purge requires a context")
	}
	if db == nil {
		return nil, nil, fmt.Errorf("%s requires the PostgreSQL submission database", retentionPolicyPathEnv)
	}
	raw, err := loadBoundedRuntimeFile(retentionPolicyPathEnv, "retention policy document")
	if err != nil {
		return nil, nil, err
	}
	declared, err := integrationretention.DecodePolicyDocument(bytes.NewReader(raw), tenantID)
	if err != nil {
		return nil, nil, fmt.Errorf("load retention policy: %w", err)
	}

	// The purge spans tables owned by two migration sets, and either may be
	// unapplied in a deployment that runs only one half of the runtime. Both
	// migrators are idempotent and advisory-locked, so applying them here costs a
	// ledger read and guarantees the component's own schema exists.
	submissions, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{})
	if err != nil {
		return nil, nil, fmt.Errorf("configure retention submission migrations: %w", err)
	}
	if err := submissions.Migrate(ctx); err != nil {
		return nil, nil, fmt.Errorf("migrate submission store for retention purge: %w", err)
	}
	sessions, err := integrationsession.NewPostgresStore(db, integrationsession.PostgresConfig{TenantID: tenantID})
	if err != nil {
		return nil, nil, fmt.Errorf("configure retention session migrations: %w", err)
	}
	if err := sessions.Migrate(ctx); err != nil {
		return nil, nil, fmt.Errorf("migrate session store for retention purge: %w", err)
	}

	store, err := integrationretention.NewPostgresStore(db, integrationretention.PostgresConfig{TenantID: tenantID})
	if err != nil {
		return nil, nil, fmt.Errorf("configure retention store: %w", err)
	}
	recorded, err := store.PutPolicy(ctx, declared)
	if err != nil {
		return nil, nil, fmt.Errorf("record retention policy: %w", err)
	}

	interval, err := retentionDurationEnv(retentionIntervalEnv, defaultRetentionCadence)
	if err != nil {
		return nil, nil, err
	}
	batchSize, err := retentionIntEnv(retentionBatchSizeEnv, defaultRetentionBatch)
	if err != nil {
		return nil, nil, err
	}
	purger, err := integrationretention.NewPurger(integrationretention.PurgerConfig{
		Store:     store,
		Interval:  interval,
		BatchSize: batchSize,
		Observe:   observe,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("configure retention purge: %w", err)
	}
	return purger, &recorded, nil
}

func retentionDurationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", name)
	}
	return parsed, nil
}

func retentionIntEnv(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

// retentionPurgeObserver reports one purge tick to the metrics registry and to
// the operator log.
//
// It deliberately publishes counts only. Record identifiers live in the durable
// purge audit, which is queryable and access-controlled; a metric label is
// neither, and the label allowlist in internal/observability exists so that
// distinction cannot erode.
//
// It lives here rather than in serve_observability.go so this lane appends its
// own file instead of editing Slice 4.3's.
func retentionPurgeObserver(metrics *observability.Metrics) func(integrationretention.PurgeResult, error) {
	return func(result integrationretention.PurgeResult, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: retention purge failed: %v\n", err)
			metrics.RecordRetentionPurge(observability.OutcomeError, 0)
			// The counts and the backlog reading are still published on the
			// error path. PurgeExpired attempts every class and joins the
			// failures, so a failing class no longer means nothing happened —
			// and the backlog is exactly what an operator needs to see when the
			// purge is erroring.
			publishRetentionBacklog(metrics, result.Backlog)
			return
		}
		metrics.RecordRetentionPurge(observability.OutcomeProcessed, result.Total())
		publishRetentionBacklog(metrics, result.Backlog)
		if result.Total() > 0 || result.BudgetExhausted {
			fmt.Printf("Retention purge: canonical_events=%d session_samples=%d "+
				"session_exports=%d stream_events=%d passes=%d backlog=%d duration=%s\n",
				result.CanonicalEvents, result.SessionSamples, result.SessionExports,
				result.StreamEvents, result.Passes, result.Backlog.Total(),
				result.Duration.Round(time.Millisecond))
		}
		if result.BudgetExhausted {
			// Falling behind is an operating condition, not a failure, so it is
			// a warning with the backlog attached rather than a component error.
			fmt.Fprintf(os.Stderr,
				"Warning: retention purge spent its whole per-tick drain budget with %d records "+
					"still eligible; the purge is not keeping up with ingest\n",
				result.Backlog.Total())
		}
	}
}

// publishRetentionBacklog sets the per-class backlog gauge.
//
// Every class is published on every tick, including the zeroes: a gauge that
// stops being reported goes stale rather than going to zero, and "the backlog
// cleared" must be distinguishable from "the purge stopped running".
func publishRetentionBacklog(metrics *observability.Metrics, backlog integrationretention.BacklogCounts) {
	metrics.SetRetentionBacklog(observability.RetentionClassCanonicalEvent, backlog.CanonicalEvents)
	metrics.SetRetentionBacklog(observability.RetentionClassSessionSample, backlog.SessionSamples)
	metrics.SetRetentionBacklog(observability.RetentionClassSessionExport, backlog.SessionExports)
	metrics.SetRetentionBacklog(observability.RetentionClassStreamEvent, backlog.StreamEvents)
}
