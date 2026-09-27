package connection

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// connectionMigrationLockKey serializes this ledger's migrations across
// replicas. Lock keys share one global namespace; this one is distinct from
// every other *MigrationLockKey in the repository (AGENTS.md, "Migration
// authoring", rule 2).
const connectionMigrationLockKey = int64(5064657639792058909)

// SchemaVersion is the connection ledger version this binary expects. It is
// the seventh ledger `fi-fhir version` and fi_fhir_schema_ledger_version
// report; the migrationcompat proof asserts it equals the highest version
// actually applied.
const SchemaVersion = 1

//go:embed migrations/0001_connection_catalog.sql
var connectionCatalogMigration string

// connectionMigration is one numbered step in this package's own forward-only
// ledger, integration_connection_schema_migrations.
type connectionMigration struct {
	version    int64
	name       string
	statements string
}

// connectionMigrations is the fixed, ordered migration set. The ledger on
// origin/main is the authority on the next free number at every rebase.
func connectionMigrations() []connectionMigration {
	return []connectionMigration{
		{version: 1, name: "0001_connection_catalog", statements: connectionCatalogMigration},
	}
}

// PostgresStore is the durable connection catalog. It owns its own numbered
// migration set and version ledger, following the per-package go:embed idiom
// of processor, lifecycle, batch, session, and destination.
//
// Every method takes the tenant explicitly and puts it in every WHERE clause:
// another tenant's row is never read, so it is indistinguishable from absence.
type PostgresStore struct {
	db    *sql.DB
	clock func() time.Time
}

// NewPostgresStore constructs the catalog store. A nil clock selects time.Now.
func NewPostgresStore(db *sql.DB, clock func() time.Time) (*PostgresStore, error) {
	if db == nil {
		return nil, ErrUnavailable
	}
	if clock == nil {
		clock = time.Now
	}
	return &PostgresStore{db: db, clock: clock}, nil
}

// Migrate applies the fixed, numbered connection schema exactly once. The
// advisory transaction lock is taken before the ledger version is read, so two
// replicas starting together cannot both observe "not applied".
func (s *PostgresStore) Migrate(ctx context.Context) error {
	if s == nil || s.db == nil || ctx == nil {
		return ErrUnavailable
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin connection migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, connectionMigrationLockKey); err != nil {
		return fmt.Errorf("lock connection migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS integration_connection_schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
		)
	`); err != nil {
		return fmt.Errorf("create connection migration ledger: %w", err)
	}
	for _, migration := range connectionMigrations() {
		var applied bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM integration_connection_schema_migrations WHERE version = $1)`,
			migration.version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("read connection migration ledger: %w", err)
		}
		if applied {
			continue
		}
		if _, err := tx.ExecContext(ctx, migration.statements); err != nil {
			return fmt.Errorf("apply connection migration %s: %w", migration.name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO integration_connection_schema_migrations (version, name) VALUES ($1, $2)`,
			migration.version, migration.name,
		); err != nil {
			return fmt.Errorf("record connection migration %s: %w", migration.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit connection migration: %w", err)
	}
	return nil
}

// Now returns the store's clock reading in UTC.
func (s *PostgresStore) Now() time.Time {
	return s.clock().UTC()
}

const draftColumns = `
	tenant_id, artifact_id, direction, kind, name, description, spec_json,
	secret_bindings_json, version, archived_at, created_json, updated_json
`

type rowScanner interface {
	Scan(...any) error
}

func scanDraft(row rowScanner) (Draft, error) {
	var draft Draft
	var direction, kind string
	var spec, bindings, created, updated []byte
	var archivedAt sql.NullTime
	if err := row.Scan(
		&draft.TenantID, &draft.ID, &direction, &kind, &draft.Name, &draft.Description,
		&spec, &bindings, &draft.Version, &archivedAt, &created, &updated,
	); err != nil {
		return Draft{}, err
	}
	draft.Direction = Direction(direction)
	draft.Kind = Kind(kind)
	draft.Spec = json.RawMessage(spec)
	if err := json.Unmarshal(bindings, &draft.SecretBindings); err != nil {
		return Draft{}, fmt.Errorf("decode connection secret bindings: %w", err)
	}
	if err := json.Unmarshal(created, &draft.Created); err != nil {
		return Draft{}, fmt.Errorf("decode connection creation audit: %w", err)
	}
	if err := json.Unmarshal(updated, &draft.Updated); err != nil {
		return Draft{}, fmt.Errorf("decode connection update audit: %w", err)
	}
	if archivedAt.Valid {
		archived := archivedAt.Time.UTC()
		draft.ArchivedAt = &archived
	}
	return draft, nil
}

// CreateDraft inserts a new draft at version one.
func (s *PostgresStore) CreateDraft(ctx context.Context, draft Draft) (Draft, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Draft{}, ErrUnavailable
	}
	bindings, created, err := marshalDraftJSON(draft.SecretBindings, draft.Created)
	if err != nil {
		return Draft{}, err
	}
	draft.Version = 1
	draft.Updated = draft.Created
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO integration_connection_drafts (
			tenant_id, artifact_id, direction, kind, name, description, spec_json,
			secret_bindings_json, version, created_json, created_at, updated_json, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, $9, $10, $9, $10)
		RETURNING `+draftColumns,
		draft.TenantID, draft.ID, string(draft.Direction), string(draft.Kind), draft.Name, draft.Description,
		string(draft.Spec), bindings, created, draft.Created.OccurredAt.UTC(),
	)
	stored, err := scanDraft(row)
	if err != nil {
		if uniqueViolation(err) {
			return Draft{}, ErrAlreadyExists
		}
		return Draft{}, fmt.Errorf("create connection draft: %w", err)
	}
	return stored, nil
}

// GetDraft loads one draft of one tenant.
func (s *PostgresStore) GetDraft(ctx context.Context, tenantID, id string) (Draft, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Draft{}, ErrUnavailable
	}
	draft, err := scanDraft(s.db.QueryRowContext(ctx, `
		SELECT `+draftColumns+` FROM integration_connection_drafts
		WHERE tenant_id = $1 AND artifact_id = $2
	`, tenantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, ErrNotFound
	}
	if err != nil {
		return Draft{}, fmt.Errorf("load connection draft: %w", err)
	}
	return draft, nil
}

// ListDrafts returns at most MaxConnections drafts of one tenant in ID order,
// optionally one direction only, archived drafts only when asked for.
func (s *PostgresStore) ListDrafts(ctx context.Context, tenantID string, direction Direction, includeArchived bool) ([]Draft, error) {
	if s == nil || s.db == nil || ctx == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+draftColumns+` FROM integration_connection_drafts
		WHERE tenant_id = $1
		  AND ($2::text = '' OR direction = $2::text)
		  AND ($3::boolean OR archived_at IS NULL)
		ORDER BY artifact_id
		LIMIT $4::integer
	`, tenantID, string(direction), includeArchived, MaxConnections)
	if err != nil {
		return nil, fmt.Errorf("list connection drafts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	drafts := make([]Draft, 0)
	for rows.Next() {
		draft, err := scanDraft(rows)
		if err != nil {
			return nil, fmt.Errorf("scan connection draft: %w", err)
		}
		drafts = append(drafts, draft)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connection drafts: %w", err)
	}
	return drafts, nil
}

// UpdateDraft writes the draft's mutable fields when the stored version still
// equals expectedVersion, and advances the version by one.
func (s *PostgresStore) UpdateDraft(ctx context.Context, draft Draft, expectedVersion int64) (Draft, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Draft{}, ErrUnavailable
	}
	bindings, updated, err := marshalDraftJSON(draft.SecretBindings, draft.Updated)
	if err != nil {
		return Draft{}, err
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE integration_connection_drafts
		SET name = $4, description = $5, spec_json = $6, secret_bindings_json = $7,
			version = version + 1, updated_json = $8, updated_at = $9
		WHERE tenant_id = $1 AND artifact_id = $2 AND version = $3 AND archived_at IS NULL
		RETURNING `+draftColumns,
		draft.TenantID, draft.ID, expectedVersion, draft.Name, draft.Description,
		string(draft.Spec), bindings, updated, draft.Updated.OccurredAt.UTC(),
	)
	stored, err := scanDraft(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, s.explainMissedWrite(ctx, draft.TenantID, draft.ID)
	}
	if err != nil {
		return Draft{}, fmt.Errorf("update connection draft: %w", err)
	}
	return stored, nil
}

// ArchiveDraft freezes a draft when the stored version still equals
// expectedVersion. Archiving advances the version like any other change.
func (s *PostgresStore) ArchiveDraft(ctx context.Context, tenantID, id string, expectedVersion int64, audit integration.AuditEnvelope) (Draft, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Draft{}, ErrUnavailable
	}
	updated, err := json.Marshal(audit)
	if err != nil {
		return Draft{}, fmt.Errorf("marshal connection archive audit: %w", err)
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE integration_connection_drafts
		SET archived_at = $4, version = version + 1, updated_json = $5, updated_at = $4
		WHERE tenant_id = $1 AND artifact_id = $2 AND version = $3 AND archived_at IS NULL
		RETURNING `+draftColumns,
		tenantID, id, expectedVersion, audit.OccurredAt.UTC(), string(updated),
	)
	stored, err := scanDraft(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, s.explainMissedWrite(ctx, tenantID, id)
	}
	if err != nil {
		return Draft{}, fmt.Errorf("archive connection draft: %w", err)
	}
	return stored, nil
}

// explainMissedWrite turns a zero-row conditional write into the reason it
// missed: absent (or another tenant's), archived, or a stale version.
func (s *PostgresStore) explainMissedWrite(ctx context.Context, tenantID, id string) error {
	current, err := s.GetDraft(ctx, tenantID, id)
	switch {
	case err != nil:
		return err
	case current.Archived():
		return ErrArchived
	default:
		return ErrVersionConflict
	}
}

func marshalDraftJSON(bindings []integration.SecretBinding, audit integration.AuditEnvelope) (string, string, error) {
	if bindings == nil {
		bindings = []integration.SecretBinding{}
	}
	bindingsJSON, err := json.Marshal(bindings)
	if err != nil {
		return "", "", fmt.Errorf("marshal connection secret bindings: %w", err)
	}
	auditJSON, err := json.Marshal(audit)
	if err != nil {
		return "", "", fmt.Errorf("marshal connection audit: %w", err)
	}
	return string(bindingsJSON), string(auditJSON), nil
}

const revisionColumns = `
	tenant_id, artifact_id, revision_id, revision_number, digest, direction, kind,
	revision_text, compiled_from_version, created_json
`

func scanRevision(row rowScanner) (Revision, error) {
	var revision Revision
	var direction, kind, document string
	var created []byte
	if err := row.Scan(
		&revision.TenantID, &revision.ArtifactID, &revision.RevisionID, &revision.Number,
		&revision.Digest, &direction, &kind, &document, &revision.CompiledFromVersion, &created,
	); err != nil {
		return Revision{}, err
	}
	revision.Direction = Direction(direction)
	revision.Kind = Kind(kind)
	revision.Document = []byte(document)
	if err := json.Unmarshal(created, &revision.Created); err != nil {
		return Revision{}, fmt.Errorf("decode connection revision audit: %w", err)
	}
	return revision, nil
}

// InsertRevision appends one compiled revision, but only while the draft is
// still at the version it was compiled from and not archived. A concurrent
// compile that claimed the same revision number loses with ErrVersionConflict.
func (s *PostgresStore) InsertRevision(ctx context.Context, revision Revision) (Revision, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Revision{}, ErrUnavailable
	}
	created, err := json.Marshal(revision.Created)
	if err != nil {
		return Revision{}, fmt.Errorf("marshal connection revision audit: %w", err)
	}
	// Every parameter is cast: an INSERT ... SELECT does not infer parameter
	// types from its target columns the way VALUES does.
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO integration_connection_revisions (
			tenant_id, artifact_id, revision_id, revision_number, digest, direction, kind,
			revision_json, revision_text, compiled_from_version, created_json, created_at
		)
		SELECT $1::text, $2::text, $3::text, $4::bigint, $5::text, $6::text, $7::text,
			$8::text::jsonb, $8::text, $9::bigint, $10::text::jsonb, $11::timestamptz
		WHERE EXISTS (
			SELECT 1 FROM integration_connection_drafts
			WHERE tenant_id = $1::text AND artifact_id = $2::text
			  AND version = $9::bigint AND archived_at IS NULL
		)
		RETURNING `+revisionColumns,
		revision.TenantID, revision.ArtifactID, revision.RevisionID, revision.Number, revision.Digest,
		string(revision.Direction), string(revision.Kind), string(revision.Document),
		revision.CompiledFromVersion, string(created), revision.Created.OccurredAt.UTC(),
	)
	stored, err := scanRevision(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Revision{}, s.explainMissedWrite(ctx, revision.TenantID, revision.ArtifactID)
	case uniqueViolation(err):
		return Revision{}, ErrVersionConflict
	case err != nil:
		return Revision{}, fmt.Errorf("insert connection revision: %w", err)
	}
	return stored, nil
}

// GetRevision loads one revision of one tenant's connection.
func (s *PostgresStore) GetRevision(ctx context.Context, tenantID, artifactID, revisionID string) (Revision, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Revision{}, ErrUnavailable
	}
	revision, err := scanRevision(s.db.QueryRowContext(ctx, `
		SELECT `+revisionColumns+` FROM integration_connection_revisions
		WHERE tenant_id = $1 AND artifact_id = $2 AND revision_id = $3
	`, tenantID, artifactID, revisionID))
	if errors.Is(err, sql.ErrNoRows) {
		return Revision{}, ErrNotFound
	}
	if err != nil {
		return Revision{}, fmt.Errorf("load connection revision: %w", err)
	}
	return revision, nil
}

// ListRevisions returns one connection's revisions, newest first, at most
// MaxRevisionsPerConnection.
func (s *PostgresStore) ListRevisions(ctx context.Context, tenantID, artifactID string) ([]Revision, error) {
	if s == nil || s.db == nil || ctx == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+revisionColumns+` FROM integration_connection_revisions
		WHERE tenant_id = $1 AND artifact_id = $2
		ORDER BY revision_number DESC
		LIMIT $3
	`, tenantID, artifactID, MaxRevisionsPerConnection)
	if err != nil {
		return nil, fmt.Errorf("list connection revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	revisions := make([]Revision, 0)
	for rows.Next() {
		revision, err := scanRevision(rows)
		if err != nil {
			return nil, fmt.Errorf("scan connection revision: %w", err)
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connection revisions: %w", err)
	}
	return revisions, nil
}

// LatestRevision returns one connection's newest revision, or nil when it has
// never been compiled.
func (s *PostgresStore) LatestRevision(ctx context.Context, tenantID, artifactID string) (*Revision, error) {
	if s == nil || s.db == nil || ctx == nil {
		return nil, ErrUnavailable
	}
	revision, err := scanRevision(s.db.QueryRowContext(ctx, `
		SELECT `+revisionColumns+` FROM integration_connection_revisions
		WHERE tenant_id = $1 AND artifact_id = $2
		ORDER BY revision_number DESC
		LIMIT 1
	`, tenantID, artifactID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load latest connection revision: %w", err)
	}
	return &revision, nil
}

// LatestRevisions returns the newest revision of every connection of one
// tenant that has one, keyed by artifact ID.
func (s *PostgresStore) LatestRevisions(ctx context.Context, tenantID string) (map[string]Revision, error) {
	if s == nil || s.db == nil || ctx == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT ON (artifact_id) `+revisionColumns+`
		FROM integration_connection_revisions
		WHERE tenant_id = $1
		ORDER BY artifact_id, revision_number DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list latest connection revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	latest := make(map[string]Revision)
	for rows.Next() {
		revision, err := scanRevision(rows)
		if err != nil {
			return nil, fmt.Errorf("scan latest connection revision: %w", err)
		}
		latest[revision.ArtifactID] = revision
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate latest connection revisions: %w", err)
	}
	return latest, nil
}

// RevisionDigests returns every revision digest of one tenant — or of one of
// its connections when artifactID is non-empty — grouped by artifact ID and
// ordered newest first. It reads no document bytes.
func (s *PostgresStore) RevisionDigests(ctx context.Context, tenantID, artifactID string) (map[string][]RevisionDigest, error) {
	if s == nil || s.db == nil || ctx == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT artifact_id, revision_id, revision_number, digest
		FROM integration_connection_revisions
		WHERE tenant_id = $1 AND ($2::text = '' OR artifact_id = $2::text)
		ORDER BY artifact_id, revision_number DESC
	`, tenantID, artifactID)
	if err != nil {
		return nil, fmt.Errorf("list connection revision digests: %w", err)
	}
	defer func() { _ = rows.Close() }()
	digests := make(map[string][]RevisionDigest)
	for rows.Next() {
		var digest RevisionDigest
		if err := rows.Scan(&digest.ArtifactID, &digest.RevisionID, &digest.Number, &digest.Digest); err != nil {
			return nil, fmt.Errorf("scan connection revision digest: %w", err)
		}
		digests[digest.ArtifactID] = append(digests[digest.ArtifactID], digest)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connection revision digests: %w", err)
	}
	return digests, nil
}

func uniqueViolation(err error) bool {
	var postgresError *pq.Error
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
