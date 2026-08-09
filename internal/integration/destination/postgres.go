package destination

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
)

const destinationMigrationLockKey = int64(5064657639792058897)

//go:embed migrations/0001_delivery_identity.sql
var deliveryIdentityMigration string

//go:embed migrations/0002_https_delivery_provenance.sql
var httpsDeliveryProvenanceMigration string

// destinationMigration is one numbered step in this package's own forward-only
// ledger, integration_destination_schema_migrations.
type destinationMigration struct {
	version    int64
	name       string
	statements string
}

// destinationMigrations is the fixed, ordered migration set. The ledger is the
// authority on the next free number at every rebase, not a planning document.
func destinationMigrations() []destinationMigration {
	return []destinationMigration{
		{version: 1, name: "0001_delivery_identity", statements: deliveryIdentityMigration},
		{version: 2, name: "0002_https_delivery_provenance", statements: httpsDeliveryProvenanceMigration},
	}
}

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
	for _, migration := range destinationMigrations() {
		var applied bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM integration_destination_schema_migrations WHERE version = $1)`,
			migration.version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("read destination migration ledger: %w", err)
		}
		if applied {
			continue
		}
		if _, err := tx.ExecContext(ctx, migration.statements); err != nil {
			return fmt.Errorf("apply destination migration %s: %w", migration.name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO integration_destination_schema_migrations (version, name) VALUES ($1, $2)`,
			migration.version, migration.name,
		); err != nil {
			return fmt.Errorf("record destination migration %s: %w", migration.name, err)
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

// RecordDelivery durably appends one executed destination delivery.
//
// It writes destination provenance and one closed-vocabulary status class only.
// It never receives secret material, raw bytes, canonical event content, a
// response body, or a response header.
func (p *PostgresProvenance) RecordDelivery(ctx context.Context, record DeliveryRecord) error {
	if p == nil || p.db == nil || ctx == nil {
		return ErrProvenanceUnavailable
	}
	if !validIdentity(record.TenantID) || !validIdentity(record.AttemptID) ||
		record.Transport != TransportHTTPS || record.CompletedAt.IsZero() ||
		!validIdentity(record.DestinationDigestVerified) {
		return ErrProvenanceUnavailable
	}
	switch record.Outcome {
	case outcomeDelivered, outcomeRetryable, outcomeRefused:
	default:
		return ErrProvenanceUnavailable
	}
	if _, err := p.db.ExecContext(ctx, `
		INSERT INTO integration_destination_deliveries (
			tenant_id, attempt_id, transport,
			destination_artifact_id, destination_revision_id, destination_class,
			destination_digest_verified, outcome, failure_code, http_status_class,
			destination_endpoint_advisory, served_certificate_subject_advisory,
			completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`,
		record.TenantID,
		record.AttemptID,
		string(record.Transport),
		record.DestinationArtifactID,
		record.DestinationRevisionID,
		record.DestinationClass,
		record.DestinationDigestVerified,
		record.Outcome,
		record.FailureCode,
		record.HTTPStatusClass,
		record.EndpointAdvisory,
		record.ServedCertificateSubjectAdvisory,
		record.CompletedAt.UTC(),
	); err != nil {
		return fmt.Errorf("record destination delivery: %w", err)
	}
	return nil
}
