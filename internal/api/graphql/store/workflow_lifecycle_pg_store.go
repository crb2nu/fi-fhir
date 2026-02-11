package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const defaultWorkflowLifecycleActor = "service-account"

// PostgresWorkflowLifecycleStore is a PostgreSQL-backed workflow lifecycle store.
type PostgresWorkflowLifecycleStore struct {
	db *sql.DB
}

// NewPostgresWorkflowLifecycleStore creates a new PostgreSQL workflow lifecycle store.
func NewPostgresWorkflowLifecycleStore(db *sql.DB) *PostgresWorkflowLifecycleStore {
	return &PostgresWorkflowLifecycleStore{db: db}
}

func newLifecycleID(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, strings.ReplaceAll(uuid.NewString(), "-", ""))
}

func nullableString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func nullableStringValue(value *string) any {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func nullableTimeValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func marshalValidation(v WorkflowValidationRecord) ([]byte, error) {
	payload := struct {
		Valid    bool     `json:"valid"`
		Errors   []string `json:"errors"`
		Warnings []string `json:"warnings"`
		Info     []string `json:"info"`
	}{
		Valid:    v.Valid,
		Errors:   v.Errors,
		Warnings: v.Warnings,
		Info:     v.Info,
	}
	return json.Marshal(payload)
}

func unmarshalValidation(raw []byte) (WorkflowValidationRecord, error) {
	if len(raw) == 0 {
		return WorkflowValidationRecord{}, nil
	}

	var payload struct {
		Valid    bool     `json:"valid"`
		Errors   []string `json:"errors"`
		Warnings []string `json:"warnings"`
		Info     []string `json:"info"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return WorkflowValidationRecord{}, err
	}

	return WorkflowValidationRecord{
		Valid:    payload.Valid,
		Errors:   append([]string(nil), payload.Errors...),
		Warnings: append([]string(nil), payload.Warnings...),
		Info:     append([]string(nil), payload.Info...),
	}, nil
}

func marshalStringArray(values []string) ([]byte, error) {
	return json.Marshal(values)
}

func unmarshalStringArray(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

// InitSchema creates workflow lifecycle tables and indexes.
func (s *PostgresWorkflowLifecycleStore) InitSchema(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS workflow_definitions (
			id VARCHAR(64) PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			description TEXT,
			status VARCHAR(32) NOT NULL DEFAULT 'draft',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS workflow_versions (
			id VARCHAR(64) PRIMARY KEY,
			workflow_id VARCHAR(64) NOT NULL REFERENCES workflow_definitions(id) ON DELETE CASCADE,
			version_number INTEGER NOT NULL,
			yaml TEXT NOT NULL,
			validation JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_by VARCHAR(255) NOT NULL DEFAULT 'service-account',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			notes TEXT,
			UNIQUE(workflow_id, version_number)
		);

		CREATE TABLE IF NOT EXISTS workflow_releases (
			id VARCHAR(64) PRIMARY KEY,
			workflow_id VARCHAR(64) NOT NULL REFERENCES workflow_definitions(id) ON DELETE CASCADE,
			environment VARCHAR(64) NOT NULL,
			version_id VARCHAR(64) NOT NULL REFERENCES workflow_versions(id),
			published_by VARCHAR(255) NOT NULL DEFAULT 'service-account',
			published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			rollback_from_release_id VARCHAR(64) REFERENCES workflow_releases(id)
		);

		CREATE TABLE IF NOT EXISTS workflow_runs (
			id VARCHAR(64) PRIMARY KEY,
			workflow_id VARCHAR(64),
			workflow_name VARCHAR(255) NOT NULL,
			environment VARCHAR(64) NOT NULL DEFAULT 'production',
			version_id VARCHAR(64) REFERENCES workflow_versions(id),
			event_id VARCHAR(255),
			routes_matched INTEGER NOT NULL DEFAULT 0,
			actions_executed INTEGER NOT NULL DEFAULT 0,
			errors JSONB NOT NULL DEFAULT '[]'::jsonb,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			status VARCHAR(32) NOT NULL DEFAULT 'success',
			started_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		ALTER TABLE workflow_runs ADD COLUMN IF NOT EXISTS workflow_id VARCHAR(64);

		CREATE TABLE IF NOT EXISTS workflow_approval_requests (
			id VARCHAR(64) PRIMARY KEY,
			workflow_id VARCHAR(64) NOT NULL REFERENCES workflow_definitions(id) ON DELETE CASCADE,
			target_version_id VARCHAR(64) NOT NULL REFERENCES workflow_versions(id),
			environment VARCHAR(64) NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'pending',
			requested_by VARCHAR(255) NOT NULL DEFAULT 'service-account',
			reviewed_by VARCHAR(255),
			reviewed_at TIMESTAMPTZ,
			comment TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS workflow_audit_log (
			id VARCHAR(64) PRIMARY KEY,
			workflow_id VARCHAR(64) REFERENCES workflow_definitions(id) ON DELETE CASCADE,
			event_type VARCHAR(100) NOT NULL,
			actor VARCHAR(255) NOT NULL,
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_workflow_versions_workflow_id ON workflow_versions(workflow_id);
		CREATE INDEX IF NOT EXISTS idx_workflow_releases_workflow_env ON workflow_releases(workflow_id, environment, published_at DESC);
		CREATE INDEX IF NOT EXISTS idx_workflow_runs_name_env_started ON workflow_runs(workflow_name, environment, started_at DESC);
		CREATE INDEX IF NOT EXISTS idx_workflow_approvals_workflow_env_status ON workflow_approval_requests(workflow_id, environment, status);
		CREATE INDEX IF NOT EXISTS idx_workflow_audit_workflow_occurred ON workflow_audit_log(workflow_id, occurred_at DESC);
	`

	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// CreateWorkflowDefinition creates a managed workflow definition.
func (s *PostgresWorkflowLifecycleStore) CreateWorkflowDefinition(ctx context.Context, def *WorkflowDefinitionRecord) (*WorkflowDefinitionRecord, error) {
	if def == nil {
		return nil, fmt.Errorf("workflow definition is required")
	}
	name := strings.TrimSpace(def.Name)
	if name == "" {
		return nil, fmt.Errorf("workflow definition name is required")
	}

	id := strings.TrimSpace(def.ID)
	if id == "" {
		id = newLifecycleID("wf")
	}

	status := strings.TrimSpace(def.Status)
	if status == "" {
		status = WorkflowDefinitionStatusDraft
	}

	description := nullableString(def.Description)
	record := &WorkflowDefinitionRecord{
		ID:          id,
		Name:        name,
		Description: "",
		Status:      strings.ToLower(status),
	}
	if description != nil {
		record.Description = *description
	}

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO workflow_definitions (id, name, description, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING created_at, updated_at
	`, record.ID, record.Name, description, record.Status).Scan(&record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create workflow definition: %w", err)
	}
	return record, nil
}

// UpdateWorkflowDefinition updates mutable workflow definition metadata.
func (s *PostgresWorkflowLifecycleStore) UpdateWorkflowDefinition(ctx context.Context, def *WorkflowDefinitionRecord) (*WorkflowDefinitionRecord, error) {
	if def == nil {
		return nil, fmt.Errorf("workflow definition is required")
	}
	if strings.TrimSpace(def.ID) == "" {
		return nil, fmt.Errorf("workflow definition ID is required")
	}

	status := strings.TrimSpace(def.Status)
	if status == "" {
		status = WorkflowDefinitionStatusDraft
	}
	description := nullableString(def.Description)
	record := &WorkflowDefinitionRecord{
		ID:          def.ID,
		Name:        strings.TrimSpace(def.Name),
		Description: "",
		Status:      strings.ToLower(status),
	}
	if description != nil {
		record.Description = *description
	}

	err := s.db.QueryRowContext(ctx, `
		UPDATE workflow_definitions
		SET name = $2, description = $3, status = $4, updated_at = NOW()
		WHERE id = $1
		RETURNING created_at, updated_at
	`, record.ID, record.Name, description, record.Status).Scan(&record.CreatedAt, &record.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update workflow definition: %w", err)
	}
	return record, nil
}

// ArchiveWorkflowDefinition archives a definition.
func (s *PostgresWorkflowLifecycleStore) ArchiveWorkflowDefinition(ctx context.Context, workflowID string) (*WorkflowDefinitionRecord, error) {
	record, err := s.GetWorkflowDefinitionByID(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}

	err = s.db.QueryRowContext(ctx, `
		UPDATE workflow_definitions
		SET status = $2, updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at
	`, workflowID, WorkflowDefinitionStatusArchived).Scan(&record.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("archive workflow definition: %w", err)
	}
	record.Status = WorkflowDefinitionStatusArchived
	return record, nil
}

// GetWorkflowDefinitionByID gets a workflow definition by ID.
func (s *PostgresWorkflowLifecycleStore) GetWorkflowDefinitionByID(ctx context.Context, workflowID string) (*WorkflowDefinitionRecord, error) {
	var description sql.NullString
	record := &WorkflowDefinitionRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, status, created_at, updated_at
		FROM workflow_definitions
		WHERE id = $1
	`, workflowID).Scan(&record.ID, &record.Name, &description, &record.Status, &record.CreatedAt, &record.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get workflow definition by id: %w", err)
	}
	if description.Valid {
		record.Description = description.String
	}
	return record, nil
}

// GetWorkflowDefinitionByName gets a workflow definition by name.
func (s *PostgresWorkflowLifecycleStore) GetWorkflowDefinitionByName(ctx context.Context, name string) (*WorkflowDefinitionRecord, error) {
	var description sql.NullString
	record := &WorkflowDefinitionRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, status, created_at, updated_at
		FROM workflow_definitions
		WHERE lower(name) = lower($1)
	`, strings.TrimSpace(name)).Scan(&record.ID, &record.Name, &description, &record.Status, &record.CreatedAt, &record.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get workflow definition by name: %w", err)
	}
	if description.Valid {
		record.Description = description.String
	}
	return record, nil
}

// ListWorkflowDefinitions lists workflow definitions.
func (s *PostgresWorkflowLifecycleStore) ListWorkflowDefinitions(ctx context.Context, filter WorkflowDefinitionListFilter, paging Paging) ([]*WorkflowDefinitionRecord, error) {
	limit, offset := normalizePaging(paging)
	query := `
		SELECT id, name, description, status, created_at, updated_at
		FROM workflow_definitions
		WHERE 1=1
	`
	args := make([]any, 0, 4)
	argIndex := 1

	if filter.Name != nil && strings.TrimSpace(*filter.Name) != "" {
		query += fmt.Sprintf(" AND name ILIKE $%d", argIndex)
		args = append(args, "%"+strings.TrimSpace(*filter.Name)+"%")
		argIndex++
	}
	if filter.Status != nil && strings.TrimSpace(*filter.Status) != "" {
		query += fmt.Sprintf(" AND lower(status) = lower($%d)", argIndex)
		args = append(args, strings.TrimSpace(*filter.Status))
		argIndex++
	}

	query += fmt.Sprintf(" ORDER BY updated_at DESC, name ASC LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list workflow definitions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	definitions := make([]*WorkflowDefinitionRecord, 0)
	for rows.Next() {
		var description sql.NullString
		record := &WorkflowDefinitionRecord{}
		if err := rows.Scan(&record.ID, &record.Name, &description, &record.Status, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan workflow definition: %w", err)
		}
		if description.Valid {
			record.Description = description.String
		}
		definitions = append(definitions, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow definitions: %w", err)
	}
	return definitions, nil
}

// SaveWorkflowVersion saves an immutable workflow version.
func (s *PostgresWorkflowLifecycleStore) SaveWorkflowVersion(ctx context.Context, version *WorkflowVersionRecord) (*WorkflowVersionRecord, error) {
	if version == nil {
		return nil, fmt.Errorf("workflow version is required")
	}
	if strings.TrimSpace(version.WorkflowID) == "" {
		return nil, fmt.Errorf("workflow ID is required")
	}
	if strings.TrimSpace(version.Yaml) == "" {
		return nil, fmt.Errorf("workflow yaml is required")
	}

	validationJSON, err := marshalValidation(version.Validation)
	if err != nil {
		return nil, fmt.Errorf("marshal workflow validation: %w", err)
	}

	id := strings.TrimSpace(version.ID)
	if id == "" {
		id = newLifecycleID("wfv")
	}
	createdBy := strings.TrimSpace(version.CreatedBy)
	if createdBy == "" {
		createdBy = defaultWorkflowLifecycleActor
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin workflow version transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var nextVersion int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version_number), 0) + 1
		FROM workflow_versions
		WHERE workflow_id = $1
	`, version.WorkflowID).Scan(&nextVersion); err != nil {
		return nil, fmt.Errorf("compute next workflow version number: %w", err)
	}

	var notes any
	if n := nullableString(version.Notes); n != nil {
		notes = *n
	}

	record := &WorkflowVersionRecord{
		ID:            id,
		WorkflowID:    strings.TrimSpace(version.WorkflowID),
		VersionNumber: nextVersion,
		Yaml:          version.Yaml,
		Validation:    version.Validation,
		CreatedBy:     createdBy,
		Notes:         strings.TrimSpace(version.Notes),
	}

	if err := tx.QueryRowContext(ctx, `
		INSERT INTO workflow_versions (
			id, workflow_id, version_number, yaml, validation, created_by, created_at, notes
		)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, NOW(), $7)
		RETURNING created_at
	`, record.ID, record.WorkflowID, record.VersionNumber, record.Yaml, string(validationJSON), record.CreatedBy, notes).Scan(&record.CreatedAt); err != nil {
		return nil, fmt.Errorf("insert workflow version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit workflow version transaction: %w", err)
	}

	return record, nil
}

// GetWorkflowVersion gets a workflow version by ID.
func (s *PostgresWorkflowLifecycleStore) GetWorkflowVersion(ctx context.Context, versionID string) (*WorkflowVersionRecord, error) {
	var rawValidation []byte
	var notes sql.NullString
	record := &WorkflowVersionRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workflow_id, version_number, yaml, validation, created_by, created_at, notes
		FROM workflow_versions
		WHERE id = $1
	`, versionID).Scan(
		&record.ID,
		&record.WorkflowID,
		&record.VersionNumber,
		&record.Yaml,
		&rawValidation,
		&record.CreatedBy,
		&record.CreatedAt,
		&notes,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get workflow version: %w", err)
	}

	validation, err := unmarshalValidation(rawValidation)
	if err != nil {
		return nil, fmt.Errorf("unmarshal workflow validation: %w", err)
	}
	record.Validation = validation
	if notes.Valid {
		record.Notes = notes.String
	}
	return record, nil
}

// ListWorkflowVersions lists workflow versions for a definition.
func (s *PostgresWorkflowLifecycleStore) ListWorkflowVersions(ctx context.Context, workflowID string, paging Paging) ([]*WorkflowVersionRecord, error) {
	limit, offset := normalizePaging(paging)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workflow_id, version_number, yaml, validation, created_by, created_at, notes
		FROM workflow_versions
		WHERE workflow_id = $1
		ORDER BY version_number DESC
		LIMIT $2 OFFSET $3
	`, workflowID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list workflow versions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	versions := make([]*WorkflowVersionRecord, 0)
	for rows.Next() {
		var rawValidation []byte
		var notes sql.NullString
		record := &WorkflowVersionRecord{}
		if err := rows.Scan(
			&record.ID,
			&record.WorkflowID,
			&record.VersionNumber,
			&record.Yaml,
			&rawValidation,
			&record.CreatedBy,
			&record.CreatedAt,
			&notes,
		); err != nil {
			return nil, fmt.Errorf("scan workflow version: %w", err)
		}
		validation, err := unmarshalValidation(rawValidation)
		if err != nil {
			return nil, fmt.Errorf("unmarshal workflow validation: %w", err)
		}
		record.Validation = validation
		if notes.Valid {
			record.Notes = notes.String
		}
		versions = append(versions, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow versions: %w", err)
	}
	return versions, nil
}

// GetLatestWorkflowVersion gets the latest workflow version.
func (s *PostgresWorkflowLifecycleStore) GetLatestWorkflowVersion(ctx context.Context, workflowID string) (*WorkflowVersionRecord, error) {
	rows, err := s.ListWorkflowVersions(ctx, workflowID, Paging{Limit: 1, Offset: 0})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// PublishWorkflowVersion publishes a workflow version to an environment.
func (s *PostgresWorkflowLifecycleStore) PublishWorkflowVersion(ctx context.Context, release *WorkflowReleaseRecord) (*WorkflowReleaseRecord, error) {
	if release == nil {
		return nil, fmt.Errorf("workflow release is required")
	}
	if strings.TrimSpace(release.WorkflowID) == "" {
		return nil, fmt.Errorf("workflow ID is required")
	}
	if strings.TrimSpace(release.Environment) == "" {
		return nil, fmt.Errorf("environment is required")
	}
	if strings.TrimSpace(release.VersionID) == "" {
		return nil, fmt.Errorf("version ID is required")
	}

	id := strings.TrimSpace(release.ID)
	if id == "" {
		id = newLifecycleID("wfr")
	}
	publishedBy := strings.TrimSpace(release.PublishedBy)
	if publishedBy == "" {
		publishedBy = defaultWorkflowLifecycleActor
	}
	publishedAt := release.PublishedAt.UTC()
	if release.PublishedAt.IsZero() {
		publishedAt = time.Now().UTC()
	}

	record := &WorkflowReleaseRecord{
		ID:                    id,
		WorkflowID:            strings.TrimSpace(release.WorkflowID),
		Environment:           strings.TrimSpace(release.Environment),
		VersionID:             strings.TrimSpace(release.VersionID),
		PublishedBy:           publishedBy,
		PublishedAt:           publishedAt,
		RollbackFromReleaseID: release.RollbackFromReleaseID,
	}

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO workflow_releases (
			id, workflow_id, environment, version_id, published_by, published_at, rollback_from_release_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING published_at
	`, record.ID, record.WorkflowID, record.Environment, record.VersionID, record.PublishedBy, record.PublishedAt, nullableStringValue(record.RollbackFromReleaseID)).Scan(&record.PublishedAt)
	if err != nil {
		return nil, fmt.Errorf("publish workflow version: %w", err)
	}
	return record, nil
}

// GetWorkflowRelease gets a release by ID.
func (s *PostgresWorkflowLifecycleStore) GetWorkflowRelease(ctx context.Context, releaseID string) (*WorkflowReleaseRecord, error) {
	var rollbackID sql.NullString
	record := &WorkflowReleaseRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workflow_id, environment, version_id, published_by, published_at, rollback_from_release_id
		FROM workflow_releases
		WHERE id = $1
	`, releaseID).Scan(
		&record.ID,
		&record.WorkflowID,
		&record.Environment,
		&record.VersionID,
		&record.PublishedBy,
		&record.PublishedAt,
		&rollbackID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get workflow release: %w", err)
	}
	if rollbackID.Valid {
		record.RollbackFromReleaseID = &rollbackID.String
	}
	return record, nil
}

// GetPublishedWorkflowRelease gets the latest release for a workflow/environment.
func (s *PostgresWorkflowLifecycleStore) GetPublishedWorkflowRelease(ctx context.Context, workflowID, environment string) (*WorkflowReleaseRecord, error) {
	var rollbackID sql.NullString
	record := &WorkflowReleaseRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workflow_id, environment, version_id, published_by, published_at, rollback_from_release_id
		FROM workflow_releases
		WHERE workflow_id = $1 AND lower(environment) = lower($2)
		ORDER BY published_at DESC, id DESC
		LIMIT 1
	`, workflowID, environment).Scan(
		&record.ID,
		&record.WorkflowID,
		&record.Environment,
		&record.VersionID,
		&record.PublishedBy,
		&record.PublishedAt,
		&rollbackID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get published workflow release: %w", err)
	}
	if rollbackID.Valid {
		record.RollbackFromReleaseID = &rollbackID.String
	}
	return record, nil
}

// ListWorkflowReleases lists releases for a workflow.
func (s *PostgresWorkflowLifecycleStore) ListWorkflowReleases(ctx context.Context, workflowID string) ([]*WorkflowReleaseRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workflow_id, environment, version_id, published_by, published_at, rollback_from_release_id
		FROM workflow_releases
		WHERE workflow_id = $1
		ORDER BY published_at DESC, id DESC
	`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("list workflow releases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	releases := make([]*WorkflowReleaseRecord, 0)
	for rows.Next() {
		var rollbackID sql.NullString
		record := &WorkflowReleaseRecord{}
		if err := rows.Scan(
			&record.ID,
			&record.WorkflowID,
			&record.Environment,
			&record.VersionID,
			&record.PublishedBy,
			&record.PublishedAt,
			&rollbackID,
		); err != nil {
			return nil, fmt.Errorf("scan workflow release: %w", err)
		}
		if rollbackID.Valid {
			record.RollbackFromReleaseID = &rollbackID.String
		}
		releases = append(releases, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow releases: %w", err)
	}
	return releases, nil
}

// CreateWorkflowRun stores a workflow run.
func (s *PostgresWorkflowLifecycleStore) CreateWorkflowRun(ctx context.Context, run *WorkflowRunRecord) (*WorkflowRunRecord, error) {
	if run == nil {
		return nil, fmt.Errorf("workflow run is required")
	}
	if strings.TrimSpace(run.WorkflowName) == "" {
		return nil, fmt.Errorf("workflow name is required")
	}

	id := strings.TrimSpace(run.ID)
	if id == "" {
		id = newLifecycleID("wfrun")
	}

	environment := strings.TrimSpace(run.Environment)
	if environment == "" {
		environment = "production"
	}

	status := strings.TrimSpace(run.Status)
	if status == "" {
		status = WorkflowRunStatusSuccess
		if len(run.Errors) > 0 {
			status = WorkflowRunStatusFailed
		}
	}

	startedAt := run.StartedAt.UTC()
	if run.StartedAt.IsZero() {
		startedAt = time.Now().UTC()
	}

	errorsJSON, err := marshalStringArray(run.Errors)
	if err != nil {
		return nil, fmt.Errorf("marshal workflow run errors: %w", err)
	}

	record := &WorkflowRunRecord{
		ID:              id,
		WorkflowID:      strings.TrimSpace(run.WorkflowID),
		WorkflowName:    strings.TrimSpace(run.WorkflowName),
		Environment:     environment,
		VersionID:       run.VersionID,
		EventID:         run.EventID,
		RoutesMatched:   run.RoutesMatched,
		ActionsExecuted: run.ActionsExecuted,
		Errors:          append([]string(nil), run.Errors...),
		DurationMs:      run.DurationMs,
		StartedAt:       startedAt,
		Status:          strings.ToLower(status),
	}

	err = s.db.QueryRowContext(ctx, `
		INSERT INTO workflow_runs (
			id, workflow_id, workflow_name, environment, version_id, event_id,
			routes_matched, actions_executed, errors, duration_ms, status, started_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9::jsonb, $10, $11, $12
		)
		RETURNING started_at
	`,
		record.ID,
		nullableStringValue(nullableString(record.WorkflowID)),
		record.WorkflowName,
		record.Environment,
		nullableStringValue(record.VersionID),
		nullableStringValue(record.EventID),
		record.RoutesMatched,
		record.ActionsExecuted,
		string(errorsJSON),
		record.DurationMs,
		record.Status,
		record.StartedAt,
	).Scan(&record.StartedAt)
	if err != nil {
		return nil, fmt.Errorf("create workflow run: %w", err)
	}

	return record, nil
}

// GetWorkflowRun gets a workflow run by ID.
func (s *PostgresWorkflowLifecycleStore) GetWorkflowRun(ctx context.Context, runID string) (*WorkflowRunRecord, error) {
	var workflowID, versionID, eventID sql.NullString
	var rawErrors []byte
	record := &WorkflowRunRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workflow_id, workflow_name, environment, version_id, event_id,
		       routes_matched, actions_executed, errors, duration_ms, started_at, status
		FROM workflow_runs
		WHERE id = $1
	`, runID).Scan(
		&record.ID,
		&workflowID,
		&record.WorkflowName,
		&record.Environment,
		&versionID,
		&eventID,
		&record.RoutesMatched,
		&record.ActionsExecuted,
		&rawErrors,
		&record.DurationMs,
		&record.StartedAt,
		&record.Status,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get workflow run: %w", err)
	}

	if workflowID.Valid {
		record.WorkflowID = workflowID.String
	}
	if versionID.Valid {
		record.VersionID = &versionID.String
	}
	if eventID.Valid {
		record.EventID = &eventID.String
	}
	errorsList, err := unmarshalStringArray(rawErrors)
	if err != nil {
		return nil, fmt.Errorf("unmarshal workflow run errors: %w", err)
	}
	record.Errors = errorsList

	return record, nil
}

// ListWorkflowRuns lists workflow runs with filtering and paging.
func (s *PostgresWorkflowLifecycleStore) ListWorkflowRuns(ctx context.Context, filter WorkflowRunListFilter, paging Paging) ([]*WorkflowRunRecord, error) {
	limit, offset := normalizePaging(paging)
	query := `
		SELECT id, workflow_id, workflow_name, environment, version_id, event_id,
		       routes_matched, actions_executed, errors, duration_ms, started_at, status
		FROM workflow_runs
		WHERE 1=1
	`
	args := make([]any, 0, 8)
	argIndex := 1

	if filter.WorkflowName != nil && strings.TrimSpace(*filter.WorkflowName) != "" {
		query += fmt.Sprintf(" AND lower(workflow_name) = lower($%d)", argIndex)
		args = append(args, strings.TrimSpace(*filter.WorkflowName))
		argIndex++
	}
	if filter.Environment != nil && strings.TrimSpace(*filter.Environment) != "" {
		query += fmt.Sprintf(" AND lower(environment) = lower($%d)", argIndex)
		args = append(args, strings.TrimSpace(*filter.Environment))
		argIndex++
	}
	if filter.Status != nil && strings.TrimSpace(*filter.Status) != "" {
		query += fmt.Sprintf(" AND lower(status) = lower($%d)", argIndex)
		args = append(args, strings.TrimSpace(*filter.Status))
		argIndex++
	}
	if filter.FromStartedAt != nil {
		query += fmt.Sprintf(" AND started_at >= $%d", argIndex)
		args = append(args, filter.FromStartedAt.UTC())
		argIndex++
	}
	if filter.ToStartedAt != nil {
		query += fmt.Sprintf(" AND started_at <= $%d", argIndex)
		args = append(args, filter.ToStartedAt.UTC())
		argIndex++
	}

	query += fmt.Sprintf(" ORDER BY started_at DESC, id DESC LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list workflow runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	runs := make([]*WorkflowRunRecord, 0)
	for rows.Next() {
		var workflowID, versionID, eventID sql.NullString
		var rawErrors []byte
		record := &WorkflowRunRecord{}
		if err := rows.Scan(
			&record.ID,
			&workflowID,
			&record.WorkflowName,
			&record.Environment,
			&versionID,
			&eventID,
			&record.RoutesMatched,
			&record.ActionsExecuted,
			&rawErrors,
			&record.DurationMs,
			&record.StartedAt,
			&record.Status,
		); err != nil {
			return nil, fmt.Errorf("scan workflow run: %w", err)
		}
		if workflowID.Valid {
			record.WorkflowID = workflowID.String
		}
		if versionID.Valid {
			record.VersionID = &versionID.String
		}
		if eventID.Valid {
			record.EventID = &eventID.String
		}
		errorsList, err := unmarshalStringArray(rawErrors)
		if err != nil {
			return nil, fmt.Errorf("unmarshal workflow run errors: %w", err)
		}
		record.Errors = errorsList
		runs = append(runs, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow runs: %w", err)
	}
	return runs, nil
}

// CreateWorkflowApprovalRequest creates a new workflow approval request.
func (s *PostgresWorkflowLifecycleStore) CreateWorkflowApprovalRequest(ctx context.Context, req *WorkflowApprovalRequestRecord) (*WorkflowApprovalRequestRecord, error) {
	if req == nil {
		return nil, fmt.Errorf("workflow approval request is required")
	}
	if strings.TrimSpace(req.WorkflowID) == "" {
		return nil, fmt.Errorf("workflow ID is required")
	}
	if strings.TrimSpace(req.TargetVersionID) == "" {
		return nil, fmt.Errorf("target version ID is required")
	}
	if strings.TrimSpace(req.Environment) == "" {
		return nil, fmt.Errorf("environment is required")
	}

	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = newLifecycleID("wfapr")
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = WorkflowApprovalStatusPending
	}
	requestedBy := strings.TrimSpace(req.RequestedBy)
	if requestedBy == "" {
		requestedBy = defaultWorkflowLifecycleActor
	}

	record := &WorkflowApprovalRequestRecord{
		ID:              id,
		WorkflowID:      strings.TrimSpace(req.WorkflowID),
		TargetVersionID: strings.TrimSpace(req.TargetVersionID),
		Environment:     strings.TrimSpace(req.Environment),
		Status:          strings.ToLower(status),
		RequestedBy:     requestedBy,
		ReviewedBy:      req.ReviewedBy,
		ReviewedAt:      req.ReviewedAt,
		Comment:         req.Comment,
	}

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO workflow_approval_requests (
			id, workflow_id, target_version_id, environment, status,
			requested_by, reviewed_by, reviewed_at, comment, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		RETURNING reviewed_by, reviewed_at, comment
	`,
		record.ID,
		record.WorkflowID,
		record.TargetVersionID,
		record.Environment,
		record.Status,
		record.RequestedBy,
		nullableStringValue(record.ReviewedBy),
		nullableTimeValue(record.ReviewedAt),
		nullableStringValue(record.Comment),
	).Scan(new(sql.NullString), new(sql.NullTime), new(sql.NullString))
	if err != nil {
		return nil, fmt.Errorf("create workflow approval request: %w", err)
	}
	return record, nil
}

// UpdateWorkflowApprovalRequest updates workflow approval request status/reviewer details.
func (s *PostgresWorkflowLifecycleStore) UpdateWorkflowApprovalRequest(ctx context.Context, req *WorkflowApprovalRequestRecord) (*WorkflowApprovalRequestRecord, error) {
	if req == nil {
		return nil, fmt.Errorf("workflow approval request is required")
	}
	if strings.TrimSpace(req.ID) == "" {
		return nil, fmt.Errorf("workflow approval request ID is required")
	}

	var reviewedBy sql.NullString
	var reviewedAt sql.NullTime
	var comment sql.NullString
	record := &WorkflowApprovalRequestRecord{}
	err := s.db.QueryRowContext(ctx, `
		UPDATE workflow_approval_requests
		SET status = $2, reviewed_by = $3, reviewed_at = $4, comment = $5
		WHERE id = $1
		RETURNING id, workflow_id, target_version_id, environment, status, requested_by, reviewed_by, reviewed_at, comment
	`,
		req.ID,
		strings.ToLower(strings.TrimSpace(req.Status)),
		nullableStringValue(req.ReviewedBy),
		nullableTimeValue(req.ReviewedAt),
		nullableStringValue(req.Comment),
	).Scan(
		&record.ID,
		&record.WorkflowID,
		&record.TargetVersionID,
		&record.Environment,
		&record.Status,
		&record.RequestedBy,
		&reviewedBy,
		&reviewedAt,
		&comment,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update workflow approval request: %w", err)
	}

	if reviewedBy.Valid {
		record.ReviewedBy = &reviewedBy.String
	}
	if reviewedAt.Valid {
		t := reviewedAt.Time
		record.ReviewedAt = &t
	}
	if comment.Valid {
		record.Comment = &comment.String
	}
	return record, nil
}

// GetWorkflowApprovalRequest gets a workflow approval request by ID.
func (s *PostgresWorkflowLifecycleStore) GetWorkflowApprovalRequest(ctx context.Context, approvalID string) (*WorkflowApprovalRequestRecord, error) {
	var reviewedBy sql.NullString
	var reviewedAt sql.NullTime
	var comment sql.NullString
	record := &WorkflowApprovalRequestRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workflow_id, target_version_id, environment, status, requested_by, reviewed_by, reviewed_at, comment
		FROM workflow_approval_requests
		WHERE id = $1
	`, approvalID).Scan(
		&record.ID,
		&record.WorkflowID,
		&record.TargetVersionID,
		&record.Environment,
		&record.Status,
		&record.RequestedBy,
		&reviewedBy,
		&reviewedAt,
		&comment,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get workflow approval request: %w", err)
	}

	if reviewedBy.Valid {
		record.ReviewedBy = &reviewedBy.String
	}
	if reviewedAt.Valid {
		t := reviewedAt.Time
		record.ReviewedAt = &t
	}
	if comment.Valid {
		record.Comment = &comment.String
	}
	return record, nil
}

// ListWorkflowApprovalRequests lists workflow approval requests.
func (s *PostgresWorkflowLifecycleStore) ListWorkflowApprovalRequests(ctx context.Context, filter WorkflowApprovalRequestListFilter, paging Paging) ([]*WorkflowApprovalRequestRecord, error) {
	limit, offset := normalizePaging(paging)
	query := `
		SELECT id, workflow_id, target_version_id, environment, status, requested_by, reviewed_by, reviewed_at, comment
		FROM workflow_approval_requests
		WHERE 1=1
	`
	args := make([]any, 0, 5)
	argIndex := 1

	if filter.WorkflowID != nil && strings.TrimSpace(*filter.WorkflowID) != "" {
		query += fmt.Sprintf(" AND workflow_id = $%d", argIndex)
		args = append(args, strings.TrimSpace(*filter.WorkflowID))
		argIndex++
	}
	if filter.Environment != nil && strings.TrimSpace(*filter.Environment) != "" {
		query += fmt.Sprintf(" AND lower(environment) = lower($%d)", argIndex)
		args = append(args, strings.TrimSpace(*filter.Environment))
		argIndex++
	}
	if filter.Status != nil && strings.TrimSpace(*filter.Status) != "" {
		query += fmt.Sprintf(" AND lower(status) = lower($%d)", argIndex)
		args = append(args, strings.TrimSpace(*filter.Status))
		argIndex++
	}
	query += fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list workflow approval requests: %w", err)
	}
	defer func() { _ = rows.Close() }()

	requests := make([]*WorkflowApprovalRequestRecord, 0)
	for rows.Next() {
		var reviewedBy sql.NullString
		var reviewedAt sql.NullTime
		var comment sql.NullString
		record := &WorkflowApprovalRequestRecord{}
		if err := rows.Scan(
			&record.ID,
			&record.WorkflowID,
			&record.TargetVersionID,
			&record.Environment,
			&record.Status,
			&record.RequestedBy,
			&reviewedBy,
			&reviewedAt,
			&comment,
		); err != nil {
			return nil, fmt.Errorf("scan workflow approval request: %w", err)
		}
		if reviewedBy.Valid {
			record.ReviewedBy = &reviewedBy.String
		}
		if reviewedAt.Valid {
			t := reviewedAt.Time
			record.ReviewedAt = &t
		}
		if comment.Valid {
			record.Comment = &comment.String
		}
		requests = append(requests, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow approval requests: %w", err)
	}
	return requests, nil
}

// CreateWorkflowAuditLog creates a workflow lifecycle audit log entry.
func (s *PostgresWorkflowLifecycleStore) CreateWorkflowAuditLog(ctx context.Context, entry *WorkflowAuditLogRecord) (*WorkflowAuditLogRecord, error) {
	if entry == nil {
		return nil, fmt.Errorf("workflow audit entry is required")
	}

	id := strings.TrimSpace(entry.ID)
	if id == "" {
		id = newLifecycleID("wfaudit")
	}

	actor := strings.TrimSpace(entry.Actor)
	if actor == "" {
		actor = defaultWorkflowLifecycleActor
	}

	occurredAt := entry.OccurredAt.UTC()
	if entry.OccurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	metadata := entry.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal workflow audit metadata: %w", err)
	}

	record := &WorkflowAuditLogRecord{
		ID:         id,
		WorkflowID: strings.TrimSpace(entry.WorkflowID),
		EventType:  strings.TrimSpace(entry.EventType),
		Actor:      actor,
		OccurredAt: occurredAt,
		Metadata:   metadata,
	}

	err = s.db.QueryRowContext(ctx, `
		INSERT INTO workflow_audit_log (id, workflow_id, event_type, actor, metadata, occurred_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		RETURNING occurred_at
	`,
		record.ID,
		nullableStringValue(nullableString(record.WorkflowID)),
		record.EventType,
		record.Actor,
		string(metadataJSON),
		record.OccurredAt,
	).Scan(&record.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("create workflow audit log: %w", err)
	}

	return record, nil
}
