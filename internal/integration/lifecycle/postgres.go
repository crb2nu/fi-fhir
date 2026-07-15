package lifecycle

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	lifecycleMigrationVersion = 1
	lifecycleMigrationLockKey = int64(5064657639792058891)
)

//go:embed migrations/0001_deployment_lifecycle.sql
var deploymentLifecycleMigration string

// PostgresCatalog owns the durable lifecycle state machine for integration revisions.
type PostgresCatalog struct {
	db                 *sql.DB
	clock              func() time.Time
	validateConnection ConnectionValidatorFunc
}

// NewPostgresCatalog constructs a PostgreSQL lifecycle catalog.
func NewPostgresCatalog(db *sql.DB, config Config) (*PostgresCatalog, error) {
	if db == nil {
		return nil, ErrUnavailable
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &PostgresCatalog{
		db:                 db,
		clock:              clock,
		validateConnection: config.ValidateConnection,
	}, nil
}

// Migrate applies the fixed lifecycle schema once under a replica-safe lock.
func (c *PostgresCatalog) Migrate(ctx context.Context) error {
	if c == nil || c.db == nil || ctx == nil {
		return ErrUnavailable
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin lifecycle migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, lifecycleMigrationLockKey); err != nil {
		return fmt.Errorf("lock lifecycle migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS integration_lifecycle_schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
		)
	`); err != nil {
		return fmt.Errorf("create lifecycle migration ledger: %w", err)
	}
	var applied bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM integration_lifecycle_schema_migrations WHERE version = $1)`,
		lifecycleMigrationVersion,
	).Scan(&applied); err != nil {
		return fmt.Errorf("read lifecycle migration ledger: %w", err)
	}
	if !applied {
		if _, err := tx.ExecContext(ctx, deploymentLifecycleMigration); err != nil {
			return fmt.Errorf("apply deployment lifecycle migration: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO integration_lifecycle_schema_migrations (version, name) VALUES ($1, $2)`,
			lifecycleMigrationVersion,
			"0001_deployment_lifecycle",
		); err != nil {
			return fmt.Errorf("record deployment lifecycle migration: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit lifecycle migration: %w", err)
	}
	return nil
}

// CreateDraft registers one complete, immutable deployment revision at version one.
func (c *PostgresCatalog) CreateDraft(ctx context.Context, revision integration.IntegrationDefinitionRevision) (Snapshot, error) {
	if c == nil || c.db == nil || ctx == nil {
		return Snapshot{}, ErrUnavailable
	}
	if err := revision.ValidateForDeployment(); err != nil {
		return Snapshot{}, fmt.Errorf("%w: deployment revision", ErrInvalidCommand)
	}
	revisionJSON, err := json.Marshal(revision)
	if err != nil {
		return Snapshot{}, fmt.Errorf("marshal deployment revision: %w", err)
	}
	audit := revision.Created
	auditJSON, err := json.Marshal(audit)
	if err != nil {
		return Snapshot{}, fmt.Errorf("marshal draft audit: %w", err)
	}
	eventID, err := newRecordID("life")
	if err != nil {
		return Snapshot{}, err
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("begin draft: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO integration_definition_revisions (
			tenant_id, definition_id, revision_id, digest, revision_json, created_at
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, revision.TenantID, revision.DefinitionID, revision.RevisionID, revision.Digest, string(revisionJSON), audit.OccurredAt)
	if err != nil {
		return Snapshot{}, mapWriteError(err, ErrAlreadyExists)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO integration_lifecycle_snapshots (
			tenant_id, definition_id, revision_id, revision_digest, state, version,
			health, updated_json, updated_at
		) VALUES ($1, $2, $3, $4, $5, 1, $6, $7, $8)
	`, revision.TenantID, revision.DefinitionID, revision.RevisionID, revision.Digest,
		integration.DeploymentStateDraft, integration.DeploymentHealthUnknown, string(auditJSON), audit.OccurredAt)
	if err != nil {
		return Snapshot{}, mapWriteError(err, ErrAlreadyExists)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO integration_lifecycle_events (
			event_id, tenant_id, definition_id, revision_id, revision_digest,
			version, action, from_state, to_state, health, audit_json, occurred_at
		) VALUES ($1, $2, $3, $4, $5, 1, 'create_draft', NULL, $6, $7, $8, $9)
	`, eventID, revision.TenantID, revision.DefinitionID, revision.RevisionID, revision.Digest,
		integration.DeploymentStateDraft, integration.DeploymentHealthUnknown, string(auditJSON), audit.OccurredAt)
	if err != nil {
		return Snapshot{}, fmt.Errorf("record draft event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, fmt.Errorf("commit draft: %w", err)
	}
	return Snapshot{
		TenantID:           revision.TenantID,
		DefinitionRevision: revision.Reference(),
		State:              integration.DeploymentStateDraft,
		Version:            1,
		Health:             integration.DeploymentHealthUnknown,
		Updated:            audit,
	}, nil
}

// GetSnapshot loads the current optimistic lifecycle projection.
func (c *PostgresCatalog) GetSnapshot(ctx context.Context, tenantID, definitionID, revisionID string) (Snapshot, error) {
	if c == nil || c.db == nil || ctx == nil {
		return Snapshot{}, ErrUnavailable
	}
	row := c.db.QueryRowContext(ctx, snapshotSelect+`
		WHERE tenant_id = $1 AND definition_id = $2 AND revision_id = $3
	`, tenantID, definitionID, revisionID)
	snapshot, err := scanSnapshot(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, ErrNotFound
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("load lifecycle snapshot: %w", err)
	}
	return snapshot, nil
}

// LoadDefinitionRevision returns exact immutable JSON for catalog consumers.
func (c *PostgresCatalog) LoadDefinitionRevision(ctx context.Context, tenantID, definitionID, revisionID string) ([]byte, error) {
	if c == nil || c.db == nil || ctx == nil {
		return nil, ErrUnavailable
	}
	var raw []byte
	err := c.db.QueryRowContext(ctx, `
		SELECT revision_json FROM integration_definition_revisions
		WHERE tenant_id = $1 AND definition_id = $2 AND revision_id = $3
	`, tenantID, definitionID, revisionID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load definition revision: %w", err)
	}
	return bytes.Clone(raw), nil
}

func (c *PostgresCatalog) getRevision(ctx context.Context, tenantID, definitionID, revisionID string) (integration.IntegrationDefinitionRevision, error) {
	raw, err := c.LoadDefinitionRevision(ctx, tenantID, definitionID, revisionID)
	if err != nil {
		return integration.IntegrationDefinitionRevision{}, err
	}
	revision, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(raw))
	if err != nil || revision.ValidateForDeployment() != nil {
		return integration.IntegrationDefinitionRevision{}, ErrImmutableRecord
	}
	return revision, nil
}

const snapshotSelect = `
	SELECT tenant_id, definition_id, revision_id, revision_digest, state, version,
		COALESCE(release_id, ''), health, COALESCE(last_validation_id, ''),
		validation_passed, validation_checked_at, validation_expires_at,
		COALESCE(approval_event_id, ''), updated_json
	FROM integration_lifecycle_snapshots
`

type rowScanner interface {
	Scan(...any) error
}

func scanSnapshot(row rowScanner) (Snapshot, error) {
	var snapshot Snapshot
	var definitionID, revisionID, digest string
	var checkedAt, expiresAt sql.NullTime
	var updatedJSON []byte
	err := row.Scan(
		&snapshot.TenantID, &definitionID, &revisionID, &digest, &snapshot.State,
		&snapshot.Version, &snapshot.ReleaseID, &snapshot.Health,
		&snapshot.LastValidationID, &snapshot.ValidationPassed, &checkedAt,
		&expiresAt, &snapshot.ApprovalEventID, &updatedJSON,
	)
	if err != nil {
		return Snapshot{}, err
	}
	if err := json.Unmarshal(updatedJSON, &snapshot.Updated); err != nil {
		return Snapshot{}, ErrImmutableRecord
	}
	snapshot.DefinitionRevision = integration.ArtifactRevisionRef{
		ArtifactID: definitionID,
		RevisionID: revisionID,
		Digest:     digest,
	}
	if checkedAt.Valid {
		snapshot.ValidationCheckedAt = checkedAt.Time.UTC()
	}
	if expiresAt.Valid {
		snapshot.ValidationExpiresAt = expiresAt.Time.UTC()
	}
	return snapshot, nil
}

func newRecordID(prefix string) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create lifecycle record ID: %w", err)
	}
	return prefix + "-" + hex.EncodeToString(value[:]), nil
}

func mapWriteError(err error, duplicate error) error {
	var postgresError *pq.Error
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		if postgresError.Constraint == "integration_one_active_deployment_per_definition" {
			return ErrActiveDeployment
		}
		return duplicate
	}
	return err
}
