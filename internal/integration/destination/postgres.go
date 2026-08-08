package destination

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
)

const (
	destinationMigrationLockKey = int64(5064657639792058897)
	destinationMigrationVersion = int64(1)
	destinationMigrationName    = "0001_delivery_identity"
)

//go:embed migrations/0001_delivery_identity.sql
var deliveryIdentityMigration string

// ErrProvenanceUnavailable means the decision recorder is not configured.
var ErrProvenanceUnavailable = errors.New("destination identity provenance store unavailable")

// PostgresProvenance records deliver decisions in the same database as the
// durable delivery state machine.
//
// It owns its own numbered migration set and its own version ledger, following
// the per-package go:embed idiom already used by processor, lifecycle, batch,
// and session. It therefore claims no migration number in any other package's
// sequence.
type PostgresProvenance struct {
	db *sql.DB
}

// NewPostgresProvenance constructs the decision recorder.
func NewPostgresProvenance(db *sql.DB) (*PostgresProvenance, error) {
	if db == nil {
		return nil, ErrProvenanceUnavailable
	}
	return &PostgresProvenance{db: db}, nil
}

// Migrate applies the fixed, numbered destination schema exactly once. An
// advisory transaction lock serializes startup across replicas.
func (p *PostgresProvenance) Migrate(ctx context.Context) error {
	if p == nil || p.db == nil || ctx == nil {
		return ErrProvenanceUnavailable
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin destination migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, destinationMigrationLockKey); err != nil {
		return fmt.Errorf("lock destination migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS integration_destination_schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
		)
	`); err != nil {
		return fmt.Errorf("create destination migration ledger: %w", err)
	}
	var applied bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM integration_destination_schema_migrations WHERE version = $1)`,
		destinationMigrationVersion,
	).Scan(&applied); err != nil {
		return fmt.Errorf("read destination migration ledger: %w", err)
	}
	if !applied {
		if _, err := tx.ExecContext(ctx, deliveryIdentityMigration); err != nil {
			return fmt.Errorf("apply delivery identity migration: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO integration_destination_schema_migrations (version, name) VALUES ($1, $2)`,
			destinationMigrationVersion, destinationMigrationName,
		); err != nil {
			return fmt.Errorf("record delivery identity migration: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit destination migration: %w", err)
	}
	return nil
}

// RecordDecision durably appends one decision. It writes identity and
// destination provenance only; it never receives secret material, raw bytes, or
// canonical event content.
func (p *PostgresProvenance) RecordDecision(ctx context.Context, decision Decision) error {
	if p == nil || p.db == nil || ctx == nil {
		return ErrProvenanceUnavailable
	}
	if !validIdentity(decision.TenantID) || !validIdentity(decision.AttemptID) ||
		(decision.Mode != ModeStrict && decision.Mode != ModeCompatibility) ||
		decision.DecidedAt.IsZero() {
		return ErrProvenanceUnavailable
	}
	label := "denied"
	if decision.Authorized {
		label = "authorized"
	}
	if _, err := p.db.ExecContext(ctx, `
		INSERT INTO integration_delivery_identity_decisions (
			tenant_id, attempt_id, decision, identity_mode,
			principal_subject, principal_auth_method, granted_role,
			destination_artifact_id, destination_revision_id, destination_class,
			destination_digest_verified, denial_code,
			destination_endpoint_advisory, decided_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`,
		decision.TenantID,
		decision.AttemptID,
		label,
		string(decision.Mode),
		decision.Subject,
		decision.AuthMethod,
		decision.GrantedRole,
		decision.DestinationArtifactID,
		decision.DestinationRevisionID,
		decision.DestinationClass,
		decision.DestinationDigestVerified,
		decision.DenialCode,
		decision.EndpointAdvisory,
		decision.DecidedAt.UTC(),
	); err != nil {
		return fmt.Errorf("record delivery identity decision: %w", err)
	}
	return nil
}
