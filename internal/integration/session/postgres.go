package session

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const sessionMigrationLockKey = int64(5064657639792058879)

//go:embed migrations/0001_session_workspace.sql
var sessionWorkspaceMigration string

//go:embed migrations/0002_workflow_simulations.sql
var workflowSimulationsMigration string

//go:embed migrations/0003_publications.sql
var publicationsMigration string

// PayloadProtector encrypts explicitly retained raw sample bytes outside SQL.
type PayloadProtector interface {
	Protect(context.Context, []byte, []byte) ([]byte, error)
	Unprotect(context.Context, []byte, []byte) ([]byte, error)
}

type PostgresConfig struct {
	TenantID  string
	Clock     func() time.Time
	Protector PayloadProtector
}

// PostgresStore persists one deployment security domain's session workspace.
type PostgresStore struct {
	db        *sql.DB
	tenantID  string
	clock     func() time.Time
	protector PayloadProtector
}

func NewPostgresStore(db *sql.DB, config PostgresConfig) (*PostgresStore, error) {
	if db == nil || !validStoreIdentity(config.TenantID) {
		return nil, fmt.Errorf("%w: PostgreSQL store configuration", ErrInvalid)
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &PostgresStore{db: db, tenantID: config.TenantID, clock: clock, protector: config.Protector}, nil
}

func (s *PostgresStore) Migrate(ctx context.Context) error {
	if !s.available(ctx) {
		return ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, sessionMigrationLockKey); err != nil {
		return fmt.Errorf("lock session migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS integration_session_schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
		)
	`); err != nil {
		return fmt.Errorf("create session migration ledger: %w", err)
	}
	var applied bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM integration_session_schema_migrations WHERE version = 1)`,
	).Scan(&applied); err != nil {
		return fmt.Errorf("read session migration ledger: %w", err)
	}
	if !applied {
		if _, err := tx.ExecContext(ctx, sessionWorkspaceMigration); err != nil {
			return fmt.Errorf("apply session workspace migration: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO integration_session_schema_migrations (version, name) VALUES (1, '0001_session_workspace')`,
		); err != nil {
			return fmt.Errorf("record session workspace migration: %w", err)
		}
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM integration_session_schema_migrations WHERE version = 2)`,
	).Scan(&applied); err != nil {
		return fmt.Errorf("read workflow simulation migration ledger: %w", err)
	}
	if !applied {
		if _, err := tx.ExecContext(ctx, workflowSimulationsMigration); err != nil {
			return fmt.Errorf("apply workflow simulation migration: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO integration_session_schema_migrations (version, name) VALUES (2, '0002_workflow_simulations')`,
		); err != nil {
			return fmt.Errorf("record workflow simulation migration: %w", err)
		}
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM integration_session_schema_migrations WHERE version = 3)`,
	).Scan(&applied); err != nil {
		return fmt.Errorf("read publication migration ledger: %w", err)
	}
	if !applied {
		if _, err := tx.ExecContext(ctx, publicationsMigration); err != nil {
			return fmt.Errorf("apply publication migration: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO integration_session_schema_migrations (version, name) VALUES (3, '0003_publications')`,
		); err != nil {
			return fmt.Errorf("record publication migration: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session migration: %w", err)
	}
	return nil
}

func (s *PostgresStore) CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, error) {
	if strings.TrimSpace(req.Name) == "" || !s.available(ctx) {
		return nil, fmt.Errorf("%w: session name is required", ErrInvalid)
	}
	now := s.now()
	record := &Session{
		ID: newID("sess"), Name: strings.TrimSpace(req.Name), Description: req.Description,
		Status: SessionStatusActive, Tags: cloneStrings(req.Tags), Metadata: cloneMap(req.Metadata),
		CreatedAt: now, UpdatedAt: now,
	}
	raw, err := encodeRecord(record)
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO integration_sessions (tenant_id, session_id, status, created_at, record_json)
		VALUES ($1, $2, $3, $4, $5)
	`, s.tenantID, record.ID, record.Status, record.CreatedAt, raw)
	if err != nil {
		return nil, fmt.Errorf("create integration session: %w", err)
	}
	return cloneSession(record), nil
}

func (s *PostgresStore) UpdateSession(ctx context.Context, sessionID string, req UpdateSessionRequest) (*Session, error) {
	return s.mutateSession(ctx, sessionID, func(record *Session) error {
		if req.Name != nil {
			if strings.TrimSpace(*req.Name) == "" {
				return fmt.Errorf("%w: session name is required", ErrInvalid)
			}
			record.Name = strings.TrimSpace(*req.Name)
		}
		if req.Description != nil {
			record.Description = *req.Description
		}
		if req.Tags != nil {
			record.Tags = cloneStrings(req.Tags)
		}
		if req.Metadata != nil {
			record.Metadata = cloneMap(req.Metadata)
		}
		return nil
	})
}

func (s *PostgresStore) ArchiveSession(ctx context.Context, sessionID string) (*Session, error) {
	return s.mutateSession(ctx, sessionID, func(record *Session) error {
		if record.Status == SessionStatusArchived {
			return nil
		}
		now := s.now()
		record.Status = SessionStatusArchived
		record.ArchivedAt = &now
		return nil
	})
}

func (s *PostgresStore) mutateSession(ctx context.Context, sessionID string, mutate func(*Session) error) (*Session, error) {
	if !s.available(ctx) {
		return nil, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin session update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var raw []byte
	err = tx.QueryRowContext(ctx, `
		SELECT record_json FROM integration_sessions
		WHERE tenant_id = $1 AND session_id = $2 FOR UPDATE
	`, s.tenantID, sessionID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load integration session for update: %w", err)
	}
	var record Session
	if err := decodeRecord(raw, &record); err != nil || record.ID != sessionID {
		return nil, ErrImmutable
	}
	if err := mutate(&record); err != nil {
		return nil, err
	}
	record.UpdatedAt = s.now()
	raw, err = encodeRecord(record)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE integration_sessions SET status = $3, record_json = $4
		WHERE tenant_id = $1 AND session_id = $2
	`, s.tenantID, sessionID, record.Status, raw); err != nil {
		return nil, fmt.Errorf("update integration session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit integration session update: %w", err)
	}
	return cloneSession(&record), nil
}

func (s *PostgresStore) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT record_json FROM integration_sessions WHERE tenant_id = $1 AND session_id = $2
	`, s.tenantID, sessionID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load integration session: %w", err)
	}
	var record Session
	if err := decodeRecord(raw, &record); err != nil || record.ID != sessionID {
		return nil, ErrImmutable
	}
	return cloneSession(&record), nil
}

func (s *PostgresStore) ListSessions(ctx context.Context, opts ListSessionsOptions) ([]Session, error) {
	query := `SELECT record_json FROM integration_sessions WHERE tenant_id = $1`
	if !opts.IncludeArchived {
		query += ` AND status = 'active'`
	}
	query += ` ORDER BY created_at, session_id`
	rows, err := s.db.QueryContext(ctx, query, s.tenantID)
	if err != nil {
		return nil, fmt.Errorf("list integration sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	records, err := scanRecords[Session](rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *PostgresStore) AddSample(ctx context.Context, sessionID string, req AddSampleRequest) (*Sample, error) {
	if strings.TrimSpace(req.Name) == "" || req.Format == "" {
		return nil, fmt.Errorf("%w: sample name and format are required", ErrInvalid)
	}
	if _, err := s.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}
	policy := req.PHIPolicy
	if policy == "" {
		policy = PHIPolicyRedact
	}
	sampleID := newID("sample")
	raw := req.Raw
	redacted := false
	var cipher []byte
	if policy == PHIPolicyRedact {
		raw = redactSample(req.Format, raw)
		redacted = raw != req.Raw
	} else if policy == PHIPolicyRetain {
		if s.protector == nil {
			return nil, fmt.Errorf("%w: raw retention protector is required", ErrInvalid)
		}
		var err error
		cipher, err = s.protector.Protect(ctx, []byte(raw), s.sampleAAD(sessionID, sampleID))
		if err != nil {
			return nil, fmt.Errorf("protect retained sample: %w", err)
		}
		raw = ""
	} else {
		return nil, fmt.Errorf("%w: unsupported PHI policy %q", ErrInvalid, policy)
	}
	now := s.now()
	record := &Sample{
		ID: sampleID, SessionID: sessionID, Name: strings.TrimSpace(req.Name),
		Format: req.Format, Source: req.Source, Raw: raw, PHIPolicy: policy,
		PHIRedacted: redacted, CreatedAt: now, UpdatedAt: now,
	}
	stored := *record
	if stored.PHIPolicy == PHIPolicyRetain {
		stored.Raw = ""
	}
	encoded, err := encodeRecord(stored)
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO integration_session_samples
			(tenant_id, session_id, sample_id, created_at, record_json, raw_cipher)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, s.tenantID, sessionID, record.ID, record.CreatedAt, encoded, nullableBytes(cipher))
	if err != nil {
		return nil, fmt.Errorf("create session sample: %w", err)
	}
	if policy == PHIPolicyRetain {
		record.Raw = req.Raw
	}
	return cloneSample(record), nil
}

func (s *PostgresStore) GetSample(ctx context.Context, sessionID, sampleID string) (*Sample, error) {
	var raw, cipher []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT record_json, raw_cipher FROM integration_session_samples
		WHERE tenant_id = $1 AND session_id = $2 AND sample_id = $3
	`, s.tenantID, sessionID, sampleID).Scan(&raw, &cipher)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load session sample: %w", err)
	}
	return s.decodeSample(ctx, raw, cipher, sessionID, sampleID)
}

func (s *PostgresStore) ListSamples(ctx context.Context, sessionID string) ([]Sample, error) {
	if _, err := s.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT record_json, raw_cipher FROM integration_session_samples
		WHERE tenant_id = $1 AND session_id = $2 ORDER BY created_at, sample_id
	`, s.tenantID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session samples: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]Sample, 0)
	for rows.Next() {
		var raw, cipher []byte
		if err := rows.Scan(&raw, &cipher); err != nil {
			return nil, err
		}
		record, err := s.decodeSample(ctx, raw, cipher, sessionID, "")
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *PostgresStore) decodeSample(ctx context.Context, raw, cipher []byte, sessionID, sampleID string) (*Sample, error) {
	var record Sample
	if err := decodeRecord(raw, &record); err != nil || record.SessionID != sessionID || sampleID != "" && record.ID != sampleID {
		return nil, ErrImmutable
	}
	if record.PHIPolicy == PHIPolicyRetain {
		if s.protector == nil || len(cipher) == 0 {
			return nil, ErrImmutable
		}
		plaintext, err := s.protector.Unprotect(ctx, cipher, s.sampleAAD(record.SessionID, record.ID))
		if err != nil {
			return nil, fmt.Errorf("unprotect retained sample: %w", err)
		}
		record.Raw = string(plaintext)
	}
	return cloneSample(&record), nil
}

func (s *PostgresStore) SaveArtifactDraft(ctx context.Context, sessionID string, req SaveArtifactDraftRequest) (*ArtifactDraft, error) {
	if strings.TrimSpace(req.Name) == "" || req.Kind == "" || len(req.Content) == 0 {
		return nil, fmt.Errorf("%w: artifact name, kind, and content are required", ErrInvalid)
	}
	if _, err := s.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin artifact revision: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	artifactID := req.ID
	version := 1
	createdAt := s.now()
	if artifactID == "" {
		artifactID = newID("artifact")
	} else {
		var raw []byte
		err := tx.QueryRowContext(ctx, `
			SELECT record_json FROM integration_session_artifact_revisions
			WHERE tenant_id = $1 AND session_id = $2 AND artifact_id = $3
			ORDER BY version DESC LIMIT 1 FOR UPDATE
		`, s.tenantID, sessionID, artifactID).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("load artifact head: %w", err)
		}
		var head ArtifactDraft
		if err := decodeRecord(raw, &head); err != nil || head.Kind != req.Kind || head.ID != artifactID {
			return nil, ErrImmutable
		}
		version = head.Version + 1
		createdAt = head.CreatedAt
	}
	digest := recordDigest(req.Content)
	now := s.now()
	record := &ArtifactDraft{
		ID: artifactID, RevisionID: newID("revision"), SessionID: sessionID,
		Kind: req.Kind, Name: strings.TrimSpace(req.Name), Content: cloneRaw(req.Content),
		Version: version, Digest: digest, CreatedAt: createdAt, UpdatedAt: now,
	}
	stored := *record
	stored.Content = nil
	raw, err := encodeRecord(stored)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO integration_session_artifact_revisions
			(tenant_id, session_id, artifact_id, revision_id, version, kind, created_at, record_json, content_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, s.tenantID, sessionID, record.ID, record.RevisionID, record.Version, record.Kind, record.UpdatedAt, raw, record.Content)
	if err != nil {
		return nil, fmt.Errorf("create artifact revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit artifact revision: %w", err)
	}
	return cloneDraft(record), nil
}

func (s *PostgresStore) GetArtifactDraft(ctx context.Context, sessionID, artifactID string) (*ArtifactDraft, error) {
	return s.getArtifact(ctx, sessionID, `artifact_id = $3`, artifactID, `ORDER BY version DESC LIMIT 1`)
}

func (s *PostgresStore) GetArtifactRevision(ctx context.Context, sessionID, revisionID string) (*ArtifactDraft, error) {
	return s.getArtifact(ctx, sessionID, `revision_id = $3`, revisionID, ``)
}

func (s *PostgresStore) getArtifact(ctx context.Context, sessionID, predicate, value, suffix string) (*ArtifactDraft, error) {
	var raw, content []byte
	query := `SELECT record_json, content_bytes FROM integration_session_artifact_revisions WHERE tenant_id = $1 AND session_id = $2 AND ` + predicate + ` ` + suffix
	err := s.db.QueryRowContext(ctx, query, s.tenantID, sessionID, value).Scan(&raw, &content)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load artifact revision: %w", err)
	}
	return decodeArtifact(raw, content, sessionID)
}

func (s *PostgresStore) ListArtifactDrafts(ctx context.Context, sessionID string) ([]ArtifactDraft, error) {
	if _, err := s.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT record_json, content_bytes FROM integration_session_artifact_revisions
		WHERE tenant_id = $1 AND session_id = $2
		ORDER BY created_at, artifact_id, version
	`, s.tenantID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list artifact revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	records := make([]ArtifactDraft, 0)
	for rows.Next() {
		var raw, content []byte
		if err := rows.Scan(&raw, &content); err != nil {
			return nil, err
		}
		record, err := decodeArtifact(raw, content, sessionID)
		if err != nil {
			return nil, err
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *PostgresStore) CreateRun(ctx context.Context, sessionID, sampleID, source string) (*Run, error) {
	if _, err := s.GetSample(ctx, sessionID, sampleID); err != nil {
		return nil, err
	}
	now := s.now()
	record := &Run{ID: newID("run"), SessionID: sessionID, SampleID: sampleID, Status: RunStatusPending, Source: source, CreatedAt: now, UpdatedAt: now}
	raw, err := encodeRecord(record)
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO integration_session_runs (tenant_id, session_id, run_id, status, created_at, record_json)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, s.tenantID, sessionID, record.ID, record.Status, record.CreatedAt, raw)
	if err != nil {
		return nil, fmt.Errorf("create session run: %w", err)
	}
	return cloneRun(record), nil
}

func (s *PostgresStore) UpdateRun(ctx context.Context, run Run) (*Run, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin run update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var raw []byte
	err = tx.QueryRowContext(ctx, `
		SELECT record_json FROM integration_session_runs
		WHERE tenant_id = $1 AND run_id = $2 FOR UPDATE
	`, s.tenantID, run.ID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load run for update: %w", err)
	}
	var existing Run
	if err := decodeRecord(raw, &existing); err != nil {
		return nil, ErrImmutable
	}
	if terminalRun(existing.Status) || run.SessionID != existing.SessionID || run.SampleID != existing.SampleID || run.Source != existing.Source {
		return nil, ErrImmutable
	}
	run.CreatedAt = existing.CreatedAt
	run.UpdatedAt = s.now()
	raw, err = encodeRecord(run)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE integration_session_runs SET status = $3, record_json = $4
		WHERE tenant_id = $1 AND run_id = $2
	`, s.tenantID, run.ID, run.Status, raw); err != nil {
		return nil, fmt.Errorf("update session run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit session run: %w", err)
	}
	return cloneRun(&run), nil
}

func (s *PostgresStore) GetRun(ctx context.Context, sessionID, runID string) (*Run, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT record_json FROM integration_session_runs
		WHERE tenant_id = $1 AND session_id = $2 AND run_id = $3
	`, s.tenantID, sessionID, runID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load session run: %w", err)
	}
	var record Run
	if err := decodeRecord(raw, &record); err != nil || record.SessionID != sessionID || record.ID != runID {
		return nil, ErrImmutable
	}
	return cloneRun(&record), nil
}

func (s *PostgresStore) ListRuns(ctx context.Context, sessionID string) ([]Run, error) {
	if _, err := s.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT record_json FROM integration_session_runs
		WHERE tenant_id = $1 AND session_id = $2 ORDER BY created_at, run_id
	`, s.tenantID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	records, err := scanRecords[Run](rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *PostgresStore) CreateWorkflowSimulation(ctx context.Context, sessionID string, req CreateWorkflowSimulationRequest) (*WorkflowSimulation, error) {
	if err := validateWorkflowSimulationReferences(ctx, s, sessionID, req); err != nil {
		return nil, err
	}
	record := &WorkflowSimulation{
		ID: newID("simulation"), SessionID: sessionID,
		WorkflowArtifactID:     req.WorkflowArtifactID,
		WorkflowRevisionID:     req.WorkflowRevisionID,
		WorkflowRevisionDigest: req.WorkflowRevisionDigest,
		SourceRunIDs:           cloneStrings(req.SourceRunIDs), Events: cloneWorkflowEventTraces(req.Events),
		CreatedAt: s.now(),
	}
	raw, err := encodeRecord(record)
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO integration_session_workflow_simulations
			(tenant_id, session_id, simulation_id, workflow_revision_id, created_at, record_json)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, s.tenantID, record.SessionID, record.ID, record.WorkflowRevisionID, record.CreatedAt, raw)
	if err != nil {
		return nil, fmt.Errorf("create workflow simulation: %w", err)
	}
	return cloneWorkflowSimulation(record), nil
}

func (s *PostgresStore) GetWorkflowSimulation(ctx context.Context, sessionID, simulationID string) (*WorkflowSimulation, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT record_json FROM integration_session_workflow_simulations
		WHERE tenant_id = $1 AND session_id = $2 AND simulation_id = $3
	`, s.tenantID, sessionID, simulationID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load workflow simulation: %w", err)
	}
	var record WorkflowSimulation
	if err := decodeRecord(raw, &record); err != nil || record.SessionID != sessionID || record.ID != simulationID {
		return nil, ErrImmutable
	}
	return cloneWorkflowSimulation(&record), nil
}

func (s *PostgresStore) ListWorkflowSimulations(ctx context.Context, sessionID string) ([]WorkflowSimulation, error) {
	if _, err := s.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT record_json FROM integration_session_workflow_simulations
		WHERE tenant_id = $1 AND session_id = $2 ORDER BY created_at, simulation_id
	`, s.tenantID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list workflow simulations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	records, err := scanRecords[WorkflowSimulation](rows)
	if err != nil {
		return nil, err
	}
	for index := range records {
		if records[index].SessionID != sessionID {
			return nil, ErrImmutable
		}
		records[index] = *cloneWorkflowSimulation(&records[index])
	}
	return records, rows.Err()
}

func (s *PostgresStore) CreatePublication(ctx context.Context, sessionID string, req CreatePublicationRequest) (*Publication, error) {
	if err := validateCreatePublicationRequest(sessionID, req); err != nil {
		return nil, err
	}
	if _, err := s.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin session publication: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, s.tenantID+"\x00"+sessionID+"\x00publication"); err != nil {
		return nil, fmt.Errorf("lock session publication version: %w", err)
	}
	var version int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1
		FROM integration_session_publications
		WHERE tenant_id = $1 AND session_id = $2
	`, s.tenantID, sessionID).Scan(&version); err != nil {
		return nil, fmt.Errorf("allocate session publication version: %w", err)
	}
	record := &Publication{
		ID: req.ID, SessionID: sessionID, Version: version,
		ProfileArtifactID: req.ProfileArtifactID, ProfileRevisionID: req.ProfileRevisionID,
		ProfileRevisionDigest: req.ProfileRevisionDigest, WorkflowArtifactID: req.WorkflowArtifactID,
		WorkflowRevisionID: req.WorkflowRevisionID, WorkflowRevisionDigest: req.WorkflowRevisionDigest,
		WorkflowSimulationID: req.WorkflowSimulationID, DefinitionRevision: req.DefinitionRevision,
		DefinitionVersion: req.DefinitionVersion,
		ProductionProfile: req.ProductionProfile, ProductionWorkflow: req.ProductionWorkflow,
		SourceRunIDs: cloneStrings(req.SourceRunIDs), Manifest: cloneRaw(req.Manifest),
		ManifestDigest: req.ManifestDigest, Signature: cloneRaw(req.Signature),
		SignatureAlgorithm: req.SignatureAlgorithm, SigningKeyID: req.SigningKeyID,
		PublishedBy: req.PublishedBy, Reason: req.Reason, CreatedAt: req.CreatedAt,
	}
	metadata := *clonePublication(record)
	metadata.Manifest = nil
	metadata.Signature = nil
	raw, err := encodeRecord(metadata)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO integration_session_publications (
			tenant_id, session_id, publication_id, version, definition_id,
			definition_revision_id, workflow_simulation_id, created_at,
			record_json, manifest_bytes, signature_bytes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, s.tenantID, sessionID, record.ID, record.Version,
		record.DefinitionRevision.ArtifactID, record.DefinitionRevision.RevisionID,
		record.WorkflowSimulationID, record.CreatedAt, raw, record.Manifest, record.Signature)
	if err != nil {
		return nil, fmt.Errorf("create session publication: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit session publication: %w", err)
	}
	return clonePublication(record), nil
}

func (s *PostgresStore) GetPublication(ctx context.Context, sessionID, publicationID string) (*Publication, error) {
	var raw, manifest, signature []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT record_json, manifest_bytes, signature_bytes
		FROM integration_session_publications
		WHERE tenant_id = $1 AND session_id = $2 AND publication_id = $3
	`, s.tenantID, sessionID, publicationID).Scan(&raw, &manifest, &signature)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load session publication: %w", err)
	}
	return decodePublication(raw, manifest, signature, sessionID, publicationID)
}

func (s *PostgresStore) ListPublications(ctx context.Context, sessionID string) ([]Publication, error) {
	if _, err := s.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT record_json, manifest_bytes, signature_bytes
		FROM integration_session_publications
		WHERE tenant_id = $1 AND session_id = $2
		ORDER BY version, publication_id
	`, s.tenantID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session publications: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]Publication, 0)
	for rows.Next() {
		var raw, manifest, signature []byte
		if err := rows.Scan(&raw, &manifest, &signature); err != nil {
			return nil, err
		}
		record, err := decodePublication(raw, manifest, signature, sessionID, "")
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *PostgresStore) AcceptDecision(ctx context.Context, req AcceptDecisionRequest) (*Decision, error) {
	if strings.TrimSpace(req.AcceptedBy) == "" {
		return nil, fmt.Errorf("%w: accepted by is required", ErrInvalid)
	}
	run, err := s.GetRun(ctx, req.SessionID, req.RunID)
	if err != nil {
		return nil, err
	}
	if !runHasDiagnostic(run, req.DiagnosticID) {
		return nil, ErrNotFound
	}
	record := &Decision{
		ID: newID("decision"), SessionID: req.SessionID, RunID: req.RunID,
		DiagnosticID: req.DiagnosticID, AcceptedBy: strings.TrimSpace(req.AcceptedBy),
		Reason: strings.TrimSpace(req.Reason), AcceptedAt: s.now(),
	}
	raw, err := encodeRecord(record)
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO integration_session_decisions
			(tenant_id, session_id, decision_id, run_id, diagnostic_id, accepted_at, record_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, run_id, diagnostic_id) DO NOTHING
	`, s.tenantID, record.SessionID, record.ID, record.RunID, record.DiagnosticID, record.AcceptedAt, raw)
	if err != nil {
		return nil, fmt.Errorf("accept session decision: %w", err)
	}
	var stored []byte
	err = s.db.QueryRowContext(ctx, `
		SELECT record_json FROM integration_session_decisions
		WHERE tenant_id = $1 AND run_id = $2 AND diagnostic_id = $3
	`, s.tenantID, req.RunID, req.DiagnosticID).Scan(&stored)
	if err != nil {
		return nil, fmt.Errorf("load accepted session decision: %w", err)
	}
	if err := decodeRecord(stored, record); err != nil {
		return nil, ErrImmutable
	}
	result := *record
	return &result, nil
}

func (s *PostgresStore) ListDecisions(ctx context.Context, sessionID string) ([]Decision, error) {
	if _, err := s.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT record_json FROM integration_session_decisions
		WHERE tenant_id = $1 AND session_id = $2 ORDER BY accepted_at, decision_id
	`, s.tenantID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	records, err := scanRecords[Decision](rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *PostgresStore) ExportBundle(ctx context.Context, sessionID string) (*ExportBundle, error) {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	samples, err := s.ListSamples(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	for index := range samples {
		if samples[index].PHIPolicy == PHIPolicyRetain {
			samples[index].Raw = ""
		}
	}
	drafts, err := s.ListArtifactDrafts(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	runs, err := s.ListRuns(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	simulations, err := s.ListWorkflowSimulations(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	publications, err := s.ListPublications(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	decisions, err := s.ListDecisions(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	bundle := &ExportBundle{
		ID: newID("export"), Session: *session, Samples: samples, Drafts: drafts,
		Runs: runs, Simulations: simulations, Publications: publications,
		Decisions: decisions, ExportedAt: s.now(),
	}
	raw, err := encodeRecord(bundle)
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO integration_session_exports
			(tenant_id, session_id, export_id, exported_at, record_json)
		VALUES ($1, $2, $3, $4, $5)
	`, s.tenantID, sessionID, bundle.ID, bundle.ExportedAt, raw)
	if err != nil {
		return nil, fmt.Errorf("record session export: %w", err)
	}
	return cloneBundle(bundle), nil
}

func (s *PostgresStore) GetExport(ctx context.Context, sessionID, exportID string) (*ExportBundle, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT record_json FROM integration_session_exports
		WHERE tenant_id = $1 AND session_id = $2 AND export_id = $3
	`, s.tenantID, sessionID, exportID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load session export: %w", err)
	}
	var bundle ExportBundle
	if err := decodeRecord(raw, &bundle); err != nil || bundle.ID != exportID || bundle.Session.ID != sessionID {
		return nil, ErrImmutable
	}
	return cloneBundle(&bundle), nil
}

func (s *PostgresStore) ListExports(ctx context.Context, sessionID string) ([]ExportBundle, error) {
	if _, err := s.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT record_json FROM integration_session_exports
		WHERE tenant_id = $1 AND session_id = $2 ORDER BY exported_at, export_id
	`, s.tenantID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session exports: %w", err)
	}
	defer func() { _ = rows.Close() }()
	records, err := scanRecords[ExportBundle](rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, bundle := range records {
		if bundle.Session.ID != sessionID {
			return nil, ErrImmutable
		}
	}
	return records, nil
}

func (s *PostgresStore) available(ctx context.Context) bool {
	return s != nil && s.db != nil && ctx != nil && ctx.Err() == nil
}

func (s *PostgresStore) now() time.Time {
	return s.clock().UTC()
}

func (s *PostgresStore) sampleAAD(sessionID, sampleID string) []byte {
	return []byte(s.tenantID + "\x00" + sessionID + "\x00" + sampleID)
}

func validStoreIdentity(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n\t")
}

func encodeRecord(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode session record: %w", err)
	}
	return raw, nil
}

func decodeRecord(raw []byte, value any) error {
	if len(raw) == 0 || !json.Valid(raw) {
		return ErrImmutable
	}
	if err := json.Unmarshal(raw, value); err != nil {
		return ErrImmutable
	}
	return nil
}

func decodeArtifact(raw, content []byte, sessionID string) (*ArtifactDraft, error) {
	var record ArtifactDraft
	if err := decodeRecord(raw, &record); err != nil || record.SessionID != sessionID || len(content) == 0 {
		return nil, ErrImmutable
	}
	record.Content = cloneRaw(content)
	if record.Digest != recordDigest(record.Content) {
		return nil, ErrImmutable
	}
	return cloneDraft(&record), nil
}

func decodePublication(raw, manifest, signature []byte, sessionID, publicationID string) (*Publication, error) {
	var record Publication
	if err := decodeRecord(raw, &record); err != nil || record.SessionID != sessionID ||
		(publicationID != "" && record.ID != publicationID) || len(manifest) == 0 || len(signature) == 0 {
		return nil, ErrImmutable
	}
	record.Manifest = cloneRaw(manifest)
	record.Signature = cloneRaw(signature)
	if err := validateStoredPublication(record); err != nil {
		return nil, ErrImmutable
	}
	return clonePublication(&record), nil
}

type jsonRows interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func scanRecords[T any](rows jsonRows) ([]T, error) {
	out := make([]T, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var record T
		if err := decodeRecord(raw, &record); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func nullableBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func recordDigest(raw []byte) string {
	return fmt.Sprintf("sha256:%x", sha256Sum(raw))
}

func sha256Sum(raw []byte) [32]byte {
	return sha256.Sum256(raw)
}

var _ Store = (*PostgresStore)(nil)
