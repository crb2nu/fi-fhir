// Package db provides PostgreSQL-backed terminology storage.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MappingOrigin indicates where a custom mapping came from.
type MappingOrigin string

const (
	OriginCSVUpload         MappingOrigin = "csv_upload"
	OriginApprovedAutoroute MappingOrigin = "approved_autoroute"
	OriginManual            MappingOrigin = "manual"
)

// MappingEquivalence indicates the quality of a code mapping.
type MappingEquivalence string

const (
	EquivalenceEquivalent MappingEquivalence = "equivalent"
	EquivalenceWider      MappingEquivalence = "wider"
	EquivalenceNarrower   MappingEquivalence = "narrower"
	EquivalenceInexact    MappingEquivalence = "inexact"
)

// CustomMapping represents a user-uploaded or approved mapping.
type CustomMapping struct {
	ID            int64              `json:"id"`
	SourceSystem  string             `json:"source_system"`
	SourceCode    string             `json:"source_code"`
	SourceDisplay string             `json:"source_display,omitempty"`
	TargetSystem  string             `json:"target_system"`
	TargetCode    string             `json:"target_code"`
	TargetDisplay string             `json:"target_display,omitempty"`
	Equivalence   MappingEquivalence `json:"equivalence"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Comment       string             `json:"comment,omitempty"`
	Origin        MappingOrigin      `json:"origin"`
	UploadBatchID *uuid.UUID         `json:"upload_batch_id,omitempty"`
	ProfileID     string             `json:"profile_id,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
	CreatedBy     string             `json:"created_by,omitempty"`
	ApprovedAt    *time.Time         `json:"approved_at,omitempty"`
	ApprovedBy    string             `json:"approved_by,omitempty"`
	DecisionTrace json.RawMessage    `json:"decision_trace,omitempty"`
}

// UploadBatch tracks a CSV upload session.
type UploadBatch struct {
	ID               uuid.UUID         `json:"id"`
	Filename         string            `json:"filename"`
	SourceSystem     string            `json:"source_system,omitempty"`
	TargetSystem     string            `json:"target_system,omitempty"`
	ProfileID        string            `json:"profile_id,omitempty"`
	TotalRows        int               `json:"total_rows"`
	ValidRows        int               `json:"valid_rows"`
	DuplicateRows    int               `json:"duplicate_rows"`
	ErrorRows        int               `json:"error_rows"`
	UploadedAt       time.Time         `json:"uploaded_at"`
	UploadedBy       string            `json:"uploaded_by,omitempty"`
	ValidationErrors []ValidationError `json:"validation_errors,omitempty"`
}

// ValidationError describes an error in a specific row/column of an upload.
type ValidationError struct {
	Row     int    `json:"row"`
	Column  string `json:"column,omitempty"`
	Message string `json:"message"`
}

// MappingStore provides CRUD operations for custom mappings.
type MappingStore struct {
	db *sql.DB
}

// NewMappingStore creates a new mapping store.
func NewMappingStore(db *sql.DB) *MappingStore {
	return &MappingStore{db: db}
}

// CreateBatch creates a new upload batch record and returns its ID.
func (s *MappingStore) CreateBatch(ctx context.Context, batch *UploadBatch) error {
	errorsJSON, err := json.Marshal(batch.ValidationErrors)
	if err != nil {
		return fmt.Errorf("marshaling validation errors: %w", err)
	}

	err = s.db.QueryRowContext(ctx, `
		INSERT INTO terminology.upload_batches
			(filename, source_system, target_system, profile_id, total_rows, valid_rows, duplicate_rows, error_rows, uploaded_by, validation_errors)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, uploaded_at
	`,
		batch.Filename,
		nullIfEmpty(batch.SourceSystem),
		nullIfEmpty(batch.TargetSystem),
		nullIfEmpty(batch.ProfileID),
		batch.TotalRows,
		batch.ValidRows,
		batch.DuplicateRows,
		batch.ErrorRows,
		nullIfEmpty(batch.UploadedBy),
		errorsJSON,
	).Scan(&batch.ID, &batch.UploadedAt)
	if err != nil {
		return fmt.Errorf("creating batch: %w", err)
	}
	return nil
}

// GetBatch retrieves an upload batch by ID.
func (s *MappingStore) GetBatch(ctx context.Context, id uuid.UUID) (*UploadBatch, error) {
	var batch UploadBatch
	var sourceSystem, targetSystem, profileID, uploadedBy sql.NullString
	var errorsJSON []byte

	err := s.db.QueryRowContext(ctx, `
		SELECT id, filename, source_system, target_system, profile_id,
		       total_rows, valid_rows, duplicate_rows, error_rows,
		       uploaded_at, uploaded_by, validation_errors
		FROM terminology.upload_batches
		WHERE id = $1
	`, id).Scan(
		&batch.ID, &batch.Filename, &sourceSystem, &targetSystem, &profileID,
		&batch.TotalRows, &batch.ValidRows, &batch.DuplicateRows, &batch.ErrorRows,
		&batch.UploadedAt, &uploadedBy, &errorsJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting batch: %w", err)
	}

	batch.SourceSystem = sourceSystem.String
	batch.TargetSystem = targetSystem.String
	batch.ProfileID = profileID.String
	batch.UploadedBy = uploadedBy.String

	if len(errorsJSON) > 0 {
		if err := json.Unmarshal(errorsJSON, &batch.ValidationErrors); err != nil {
			return nil, fmt.Errorf("unmarshaling validation errors: %w", err)
		}
	}

	return &batch, nil
}

// CreateMapping inserts a new custom mapping.
func (s *MappingStore) CreateMapping(ctx context.Context, m *CustomMapping) error {
	var batchID interface{}
	if m.UploadBatchID != nil {
		batchID = *m.UploadBatchID
	}

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO terminology.custom_mappings
			(source_system, source_code, source_display, target_system, target_code, target_display,
			 equivalence, confidence, comment, origin, upload_batch_id, profile_id, created_by, decision_trace)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, created_at
	`,
		m.SourceSystem, m.SourceCode, nullIfEmpty(m.SourceDisplay),
		m.TargetSystem, m.TargetCode, nullIfEmpty(m.TargetDisplay),
		m.Equivalence, m.Confidence, nullIfEmpty(m.Comment),
		m.Origin, batchID, nullIfEmpty(m.ProfileID), nullIfEmpty(m.CreatedBy),
		nullJSON(m.DecisionTrace),
	).Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating mapping: %w", err)
	}
	return nil
}

// CreateMappingsBatch inserts multiple mappings in a single transaction.
// Returns the number of mappings created and any duplicates skipped.
func (s *MappingStore) CreateMappingsBatch(ctx context.Context, mappings []*CustomMapping) (created, duplicates int, err error) {
	if len(mappings) == 0 {
		return 0, 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO terminology.custom_mappings
			(source_system, source_code, source_display, target_system, target_code, target_display,
			 equivalence, confidence, comment, origin, upload_batch_id, profile_id, created_by, decision_trace)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (source_system, source_code, target_system, COALESCE(profile_id, ''))
		DO NOTHING
		RETURNING id
	`)
	if err != nil {
		return 0, 0, fmt.Errorf("preparing statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, m := range mappings {
		var batchID interface{}
		if m.UploadBatchID != nil {
			batchID = *m.UploadBatchID
		}

		var id sql.NullInt64
		err := stmt.QueryRowContext(ctx,
			m.SourceSystem, m.SourceCode, nullIfEmpty(m.SourceDisplay),
			m.TargetSystem, m.TargetCode, nullIfEmpty(m.TargetDisplay),
			m.Equivalence, m.Confidence, nullIfEmpty(m.Comment),
			m.Origin, batchID, nullIfEmpty(m.ProfileID), nullIfEmpty(m.CreatedBy),
			nullJSON(m.DecisionTrace),
		).Scan(&id)

		if err == sql.ErrNoRows {
			// ON CONFLICT DO NOTHING - duplicate
			duplicates++
		} else if err != nil {
			return created, duplicates, fmt.Errorf("inserting mapping row: %w", err)
		} else {
			m.ID = id.Int64
			created++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("committing transaction: %w", err)
	}

	return created, duplicates, nil
}

// LookupMapping finds a custom mapping by source code and target system.
// If profileID is empty, looks for global mappings.
func (s *MappingStore) LookupMapping(ctx context.Context, sourceSystem, sourceCode, targetSystem, profileID string) (*CustomMapping, error) {
	var m CustomMapping
	var sourceDisplay, targetDisplay, comment, createdBy, approvedBy sql.NullString
	var confidence sql.NullFloat64
	var batchID *uuid.UUID
	var approvedAt sql.NullTime
	var decisionTrace []byte

	// First try profile-specific mapping, then global
	query := `
		SELECT id, source_system, source_code, source_display, target_system, target_code, target_display,
		       equivalence, confidence, comment, origin, upload_batch_id, profile_id,
		       created_at, created_by, approved_at, approved_by, decision_trace
		FROM terminology.custom_mappings
		WHERE source_system = $1 AND source_code = $2 AND target_system = $3
		  AND (profile_id = $4 OR profile_id IS NULL OR profile_id = '')
		ORDER BY CASE WHEN profile_id = $4 THEN 0 ELSE 1 END
		LIMIT 1
	`

	err := s.db.QueryRowContext(ctx, query, sourceSystem, sourceCode, targetSystem, profileID).Scan(
		&m.ID, &m.SourceSystem, &m.SourceCode, &sourceDisplay, &m.TargetSystem, &m.TargetCode, &targetDisplay,
		&m.Equivalence, &confidence, &comment, &m.Origin, &batchID, &m.ProfileID,
		&m.CreatedAt, &createdBy, &approvedAt, &approvedBy, &decisionTrace,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("looking up mapping: %w", err)
	}

	m.SourceDisplay = sourceDisplay.String
	m.TargetDisplay = targetDisplay.String
	m.Comment = comment.String
	m.CreatedBy = createdBy.String
	m.ApprovedBy = approvedBy.String
	if confidence.Valid {
		m.Confidence = &confidence.Float64
	}
	m.UploadBatchID = batchID
	if approvedAt.Valid {
		m.ApprovedAt = &approvedAt.Time
	}
	if len(decisionTrace) > 0 {
		m.DecisionTrace = decisionTrace
	}

	return &m, nil
}

// ListMappings returns mappings with optional filters.
type ListMappingsFilter struct {
	SourceSystem  string
	TargetSystem  string
	ProfileID     string
	Origin        MappingOrigin
	UploadBatchID *uuid.UUID
	Limit         int
	Offset        int
}

// ListMappings returns custom mappings matching the filter.
func (s *MappingStore) ListMappings(ctx context.Context, filter ListMappingsFilter) ([]*CustomMapping, int, error) {
	// Build query with filters
	query := `
		SELECT id, source_system, source_code, source_display, target_system, target_code, target_display,
		       equivalence, confidence, comment, origin, upload_batch_id, profile_id,
		       created_at, created_by, approved_at, approved_by
		FROM terminology.custom_mappings
		WHERE 1=1
	`
	countQuery := `SELECT COUNT(*) FROM terminology.custom_mappings WHERE 1=1`

	var args []interface{}
	argNum := 1

	if filter.SourceSystem != "" {
		query += fmt.Sprintf(" AND source_system = $%d", argNum)
		countQuery += fmt.Sprintf(" AND source_system = $%d", argNum)
		args = append(args, filter.SourceSystem)
		argNum++
	}
	if filter.TargetSystem != "" {
		query += fmt.Sprintf(" AND target_system = $%d", argNum)
		countQuery += fmt.Sprintf(" AND target_system = $%d", argNum)
		args = append(args, filter.TargetSystem)
		argNum++
	}
	if filter.ProfileID != "" {
		query += fmt.Sprintf(" AND profile_id = $%d", argNum)
		countQuery += fmt.Sprintf(" AND profile_id = $%d", argNum)
		args = append(args, filter.ProfileID)
		argNum++
	}
	if filter.Origin != "" {
		query += fmt.Sprintf(" AND origin = $%d", argNum)
		countQuery += fmt.Sprintf(" AND origin = $%d", argNum)
		args = append(args, filter.Origin)
		argNum++
	}
	if filter.UploadBatchID != nil {
		query += fmt.Sprintf(" AND upload_batch_id = $%d", argNum)
		countQuery += fmt.Sprintf(" AND upload_batch_id = $%d", argNum)
		args = append(args, *filter.UploadBatchID)
		argNum++
	}

	// Get total count
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting mappings: %w", err)
	}

	// Add ordering and pagination
	query += " ORDER BY created_at DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying mappings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var mappings []*CustomMapping
	for rows.Next() {
		var m CustomMapping
		var sourceDisplay, targetDisplay, comment, profileID, createdBy, approvedBy sql.NullString
		var confidence sql.NullFloat64
		var batchID *uuid.UUID
		var approvedAt sql.NullTime

		err := rows.Scan(
			&m.ID, &m.SourceSystem, &m.SourceCode, &sourceDisplay, &m.TargetSystem, &m.TargetCode, &targetDisplay,
			&m.Equivalence, &confidence, &comment, &m.Origin, &batchID, &profileID,
			&m.CreatedAt, &createdBy, &approvedAt, &approvedBy,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning mapping: %w", err)
		}

		m.SourceDisplay = sourceDisplay.String
		m.TargetDisplay = targetDisplay.String
		m.Comment = comment.String
		m.ProfileID = profileID.String
		m.CreatedBy = createdBy.String
		m.ApprovedBy = approvedBy.String
		if confidence.Valid {
			m.Confidence = &confidence.Float64
		}
		m.UploadBatchID = batchID
		if approvedAt.Valid {
			m.ApprovedAt = &approvedAt.Time
		}

		mappings = append(mappings, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating mappings: %w", err)
	}

	return mappings, total, nil
}

// GetMapping retrieves a single mapping by ID.
func (s *MappingStore) GetMapping(ctx context.Context, id int64) (*CustomMapping, error) {
	var m CustomMapping
	var sourceDisplay, targetDisplay, comment, profileID, createdBy, approvedBy sql.NullString
	var confidence sql.NullFloat64
	var batchID *uuid.UUID
	var approvedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, source_system, source_code, source_display, target_system, target_code, target_display,
		       equivalence, confidence, comment, origin, upload_batch_id, profile_id,
		       created_at, created_by, approved_at, approved_by
		FROM terminology.custom_mappings
		WHERE id = $1
	`, id).Scan(
		&m.ID, &m.SourceSystem, &m.SourceCode, &sourceDisplay, &m.TargetSystem, &m.TargetCode, &targetDisplay,
		&m.Equivalence, &confidence, &comment, &m.Origin, &batchID, &profileID,
		&m.CreatedAt, &createdBy, &approvedAt, &approvedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting mapping: %w", err)
	}

	m.SourceDisplay = sourceDisplay.String
	m.TargetDisplay = targetDisplay.String
	m.Comment = comment.String
	m.ProfileID = profileID.String
	m.CreatedBy = createdBy.String
	m.ApprovedBy = approvedBy.String
	if confidence.Valid {
		m.Confidence = &confidence.Float64
	}
	m.UploadBatchID = batchID
	if approvedAt.Valid {
		m.ApprovedAt = &approvedAt.Time
	}

	return &m, nil
}

// DeleteMapping removes a custom mapping by ID.
func (s *MappingStore) DeleteMapping(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM terminology.custom_mappings WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting mapping: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("mapping not found")
	}
	return nil
}

// DeleteMappingsByBatch removes all mappings for a batch.
func (s *MappingStore) DeleteMappingsByBatch(ctx context.Context, batchID uuid.UUID) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM terminology.custom_mappings WHERE upload_batch_id = $1`, batchID)
	if err != nil {
		return 0, fmt.Errorf("deleting mappings by batch: %w", err)
	}
	return result.RowsAffected()
}

// UpdateBatchStats updates the row counts for an upload batch.
func (s *MappingStore) UpdateBatchStats(ctx context.Context, batchID uuid.UUID, valid, duplicates, errors int) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE terminology.upload_batches
		SET valid_rows = $2, duplicate_rows = $3, error_rows = $4
		WHERE id = $1
	`, batchID, valid, duplicates, errors)
	if err != nil {
		return fmt.Errorf("updating batch stats: %w", err)
	}
	return nil
}

// nullJSON returns nil if data is empty, otherwise returns the data.
func nullJSON(data json.RawMessage) interface{} {
	if len(data) == 0 {
		return nil
	}
	return data
}
