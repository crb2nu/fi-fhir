package batch

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"
)

const batchMigrationLockKey = int64(5064657639792058884)

var (
	ErrStoreUnavailable = errors.New("batch checkpoint store unavailable")
	ErrLeaseLost        = errors.New("batch object lease lost")
)

//go:embed migrations/0001_batch_ingestion.sql
var batchMigration string

type Phase string

const (
	PhaseProcessing      Phase = "processing"
	PhaseAwaitingArchive Phase = "awaiting_archive"
	PhaseCompleted       Phase = "completed"
	PhaseFailed          Phase = "failed"
)

// WorkItem is raw-free durable recovery state for one exact object version.
type WorkItem struct {
	TenantID                  string
	SourceID                  string
	SourceRevisionDigest      string
	IntegrationRevisionDigest string
	ObjectID                  string
	Provider                  ProviderType
	ObjectSize                int64
	Phase                     Phase
	CheckpointOffset          int64
	CheckpointMessage         int64
	ContentDigest             string
	LeaseOwner                string
	LeaseExpiresAt            time.Time
}

type CheckpointStore interface {
	Claim(context.Context, string, SourceRevision, string, Object, string, time.Duration) (*WorkItem, error)
	Advance(context.Context, WorkItem, int64, int64, time.Duration) (WorkItem, error)
	MarkArchivePending(context.Context, WorkItem, string, time.Duration) (WorkItem, error)
	MarkCompleted(context.Context, WorkItem) error
	Release(context.Context, WorkItem, string) error
	Fail(context.Context, WorkItem, string) error
}

type PostgresStore struct {
	db    *sql.DB
	clock func() time.Time
}

func NewPostgresStore(db *sql.DB, clock func() time.Time) (*PostgresStore, error) {
	if db == nil {
		return nil, ErrStoreUnavailable
	}
	if clock == nil {
		clock = time.Now
	}
	return &PostgresStore{db: db, clock: clock}, nil
}

func (s *PostgresStore) Migrate(ctx context.Context) error {
	if s == nil || s.db == nil || ctx == nil {
		return ErrStoreUnavailable
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin batch migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, batchMigrationLockKey); err != nil {
		return fmt.Errorf("lock batch migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS integration_batch_schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
		)
	`); err != nil {
		return fmt.Errorf("create batch migration ledger: %w", err)
	}
	var applied bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM integration_batch_schema_migrations WHERE version = 1)`,
	).Scan(&applied); err != nil {
		return fmt.Errorf("read batch migration ledger: %w", err)
	}
	if !applied {
		if _, err := tx.ExecContext(ctx, batchMigration); err != nil {
			return fmt.Errorf("apply batch migration: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO integration_batch_schema_migrations (version, name) VALUES (1, '0001_batch_ingestion')`,
		); err != nil {
			return fmt.Errorf("record batch migration: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch migration: %w", err)
	}
	return nil
}

func (s *PostgresStore) Claim(
	ctx context.Context,
	tenantID string,
	source SourceRevision,
	integrationRevisionDigest string,
	object Object,
	workerID string,
	leaseDuration time.Duration,
) (*WorkItem, error) {
	if s == nil || s.db == nil || ctx == nil || !validIdentity(tenantID) ||
		source.Validate() != nil || !validSHA256Digest(integrationRevisionDigest) ||
		object.validate() != nil || !validIdentity(workerID) || leaseDuration <= 0 {
		return nil, ErrStoreUnavailable
	}
	id, err := objectID(source, object)
	if err != nil {
		return nil, err
	}
	now := s.clock().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin batch claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO integration_batch_objects (
			tenant_id, source_id, source_revision_digest, integration_revision_digest,
			object_id, provider, object_size, object_modified_at, phase, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'processing', $9, $9)
		ON CONFLICT DO NOTHING
	`, tenantID, source.SourceID, source.Digest, integrationRevisionDigest, id,
		object.Provider, object.Size, object.ModifiedAt.UTC(), now); err != nil {
		return nil, fmt.Errorf("discover batch object: %w", err)
	}

	item, err := scanWorkItem(tx.QueryRowContext(ctx, `
		SELECT tenant_id, source_id, source_revision_digest, integration_revision_digest,
			object_id, provider,
			object_size, phase, checkpoint_offset, checkpoint_message,
			content_digest, lease_owner, lease_expires_at
		FROM integration_batch_objects
		WHERE tenant_id = $1 AND source_id = $2
		  AND source_revision_digest = $3 AND object_id = $4
		FOR UPDATE
	`, tenantID, source.SourceID, source.Digest, id))
	if err != nil {
		return nil, fmt.Errorf("lock batch object: %w", err)
	}
	if item.IntegrationRevisionDigest != integrationRevisionDigest ||
		item.ObjectSize != object.Size || item.Provider != object.Provider {
		return nil, ErrObjectChanged
	}
	if item.Phase == PhaseFailed {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit failed batch observation: %w", err)
		}
		return nil, nil
	}
	if item.Phase == PhaseCompleted {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit completed batch observation: %w", err)
		}
		return &item, nil
	}
	reclaimed := item.LeaseOwner != "" && !item.LeaseExpiresAt.After(now)
	if item.LeaseOwner != "" && item.LeaseExpiresAt.After(now) && item.LeaseOwner != workerID {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit busy batch claim: %w", err)
		}
		return nil, nil
	}
	item.LeaseOwner = workerID
	item.LeaseExpiresAt = now.Add(leaseDuration)
	if _, err := tx.ExecContext(ctx, `
		UPDATE integration_batch_objects
		SET lease_owner = $1, lease_expires_at = $2, updated_at = $3
		WHERE tenant_id = $4 AND source_id = $5
		  AND source_revision_digest = $6 AND object_id = $7
	`, item.LeaseOwner, item.LeaseExpiresAt, now, item.TenantID, item.SourceID,
		item.SourceRevisionDigest, item.ObjectID); err != nil {
		return nil, fmt.Errorf("lease batch object: %w", err)
	}
	if reclaimed {
		if err := insertBatchAudit(ctx, tx, item, "lease_reclaimed", "{}", now); err != nil {
			return nil, err
		}
	}
	if err := insertBatchAudit(ctx, tx, item, "claimed", "{}", now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit batch claim: %w", err)
	}
	return &item, nil
}

func (s *PostgresStore) Advance(ctx context.Context, item WorkItem, offset, message int64, leaseDuration time.Duration) (WorkItem, error) {
	if offset <= item.CheckpointOffset || message != item.CheckpointMessage+1 || leaseDuration <= 0 {
		return WorkItem{}, ErrLeaseLost
	}
	now := s.clock().UTC()
	nextExpiry := now.Add(leaseDuration)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("begin batch checkpoint: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE integration_batch_objects
		SET checkpoint_offset = $1, checkpoint_message = $2,
			lease_expires_at = $3, updated_at = $4, last_error_code = ''
		WHERE tenant_id = $5 AND source_id = $6
		  AND source_revision_digest = $7 AND object_id = $8
		  AND phase = 'processing' AND lease_owner = $9 AND lease_expires_at > $4
		  AND checkpoint_offset = $10 AND checkpoint_message = $11
	`, offset, message, nextExpiry, now, item.TenantID, item.SourceID,
		item.SourceRevisionDigest, item.ObjectID, item.LeaseOwner,
		item.CheckpointOffset, item.CheckpointMessage)
	if err != nil {
		return WorkItem{}, fmt.Errorf("advance batch checkpoint: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return WorkItem{}, err
	}
	item.CheckpointOffset = offset
	item.CheckpointMessage = message
	item.LeaseExpiresAt = nextExpiry
	if err := insertBatchAudit(ctx, tx, item, "checkpoint_advanced", "{}", now); err != nil {
		return WorkItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkItem{}, fmt.Errorf("commit batch checkpoint: %w", err)
	}
	return item, nil
}

func (s *PostgresStore) MarkArchivePending(ctx context.Context, item WorkItem, digest string, leaseDuration time.Duration) (WorkItem, error) {
	if !validSHA256Digest(digest) || leaseDuration <= 0 {
		return WorkItem{}, ErrLeaseLost
	}
	now := s.clock().UTC()
	nextExpiry := now.Add(leaseDuration)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkItem{}, fmt.Errorf("begin batch archive pending: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE integration_batch_objects
		SET phase = 'awaiting_archive', content_digest = $1,
			lease_expires_at = $2, updated_at = $3
		WHERE tenant_id = $4 AND source_id = $5
		  AND source_revision_digest = $6 AND object_id = $7
		  AND phase = 'processing' AND lease_owner = $8 AND lease_expires_at > $3
		  AND checkpoint_offset = $9 AND checkpoint_message = $10
	`, digest, nextExpiry, now, item.TenantID, item.SourceID,
		item.SourceRevisionDigest, item.ObjectID, item.LeaseOwner,
		item.CheckpointOffset, item.CheckpointMessage)
	if err != nil {
		return WorkItem{}, fmt.Errorf("mark batch archive pending: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return WorkItem{}, err
	}
	item.Phase = PhaseAwaitingArchive
	item.ContentDigest = digest
	item.LeaseExpiresAt = nextExpiry
	if err := insertBatchAudit(ctx, tx, item, "archive_pending", "{}", now); err != nil {
		return WorkItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkItem{}, fmt.Errorf("commit batch archive pending: %w", err)
	}
	return item, nil
}

func (s *PostgresStore) MarkCompleted(ctx context.Context, item WorkItem) error {
	now := s.clock().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin batch completion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE integration_batch_objects
		SET phase = 'completed', lease_owner = '', lease_expires_at = NULL,
			completed_at = $1, updated_at = $1
		WHERE tenant_id = $2 AND source_id = $3
		  AND source_revision_digest = $4 AND object_id = $5
		  AND phase = 'awaiting_archive' AND content_digest = $6
		  AND lease_owner = $7 AND lease_expires_at > $1
	`, now, item.TenantID, item.SourceID, item.SourceRevisionDigest,
		item.ObjectID, item.ContentDigest, item.LeaseOwner)
	if err != nil {
		return fmt.Errorf("complete batch object: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	item.Phase = PhaseCompleted
	if err := insertBatchAudit(ctx, tx, item, "completed", "{}", now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch completion: %w", err)
	}
	return nil
}

func (s *PostgresStore) Release(ctx context.Context, item WorkItem, code string) error {
	return s.finishLease(ctx, item, code, false)
}

func (s *PostgresStore) Fail(ctx context.Context, item WorkItem, code string) error {
	return s.finishLease(ctx, item, code, true)
}

func (s *PostgresStore) finishLease(ctx context.Context, item WorkItem, code string, failed bool) error {
	if !validErrorCode(code) {
		return ErrLeaseLost
	}
	now := s.clock().UTC()
	phase := item.Phase
	eventKind := "released"
	if failed {
		phase = PhaseFailed
		eventKind = "failed"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin batch lease finish: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE integration_batch_objects
		SET phase = $1, lease_owner = '', lease_expires_at = NULL,
			last_error_code = $2, updated_at = $3
		WHERE tenant_id = $4 AND source_id = $5
		  AND source_revision_digest = $6 AND object_id = $7
		  AND phase = $8 AND lease_owner = $9
	`, phase, code, now, item.TenantID, item.SourceID,
		item.SourceRevisionDigest, item.ObjectID, item.Phase, item.LeaseOwner)
	if err != nil {
		return fmt.Errorf("finish batch lease: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	item.Phase = phase
	if err := insertBatchAudit(ctx, tx, item, eventKind, fmt.Sprintf(`{"code":%q}`, code), now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch lease finish: %w", err)
	}
	return nil
}

type rowScanner interface{ Scan(...any) error }

func scanWorkItem(row rowScanner) (WorkItem, error) {
	var item WorkItem
	var leaseExpires sql.NullTime
	err := row.Scan(&item.TenantID, &item.SourceID, &item.SourceRevisionDigest,
		&item.IntegrationRevisionDigest, &item.ObjectID, &item.Provider, &item.ObjectSize, &item.Phase,
		&item.CheckpointOffset, &item.CheckpointMessage, &item.ContentDigest,
		&item.LeaseOwner, &leaseExpires)
	if leaseExpires.Valid {
		item.LeaseExpiresAt = leaseExpires.Time.UTC()
	}
	return item, err
}

func insertBatchAudit(ctx context.Context, tx *sql.Tx, item WorkItem, eventKind, detail string, recordedAt time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO integration_batch_audit (
			tenant_id, source_id, source_revision_digest, object_id, event_kind,
			checkpoint_offset, checkpoint_message, detail_json, recorded_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)
	`, item.TenantID, item.SourceID, item.SourceRevisionDigest, item.ObjectID,
		eventKind, item.CheckpointOffset, item.CheckpointMessage, detail, recordedAt.UTC())
	if err != nil {
		return fmt.Errorf("audit batch claim: %w", err)
	}
	return nil
}

func requireOneRow(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return ErrLeaseLost
	}
	return nil
}

func validErrorCode(code string) bool {
	if code == "" || len(code) > 64 {
		return false
	}
	for _, character := range code {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}
