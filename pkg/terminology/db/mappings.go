// Package db provides PostgreSQL-backed terminology storage.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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

// ListMappingsFilter provides options for filtering mappings.
type ListMappingsFilter struct {
	SourceSystem  string
	TargetSystem  string
	ProfileID     string
	Origin        MappingOrigin
	UploadBatchID *uuid.UUID
	Equivalence   MappingEquivalence
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
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
	if filter.Equivalence != "" {
		query += fmt.Sprintf(" AND equivalence = $%d", argNum)
		countQuery += fmt.Sprintf(" AND equivalence = $%d", argNum)
		args = append(args, filter.Equivalence)
		argNum++
	}
	if filter.CreatedAfter != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argNum)
		countQuery += fmt.Sprintf(" AND created_at >= $%d", argNum)
		args = append(args, *filter.CreatedAfter)
		argNum++
	}
	if filter.CreatedBefore != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argNum)
		countQuery += fmt.Sprintf(" AND created_at <= $%d", argNum)
		args = append(args, *filter.CreatedBefore)
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

// UpdateMappingInput contains the fields that can be updated on a mapping.
type UpdateMappingInput struct {
	ID            int64
	SourceDisplay *string
	TargetDisplay *string
	Equivalence   *MappingEquivalence
	Confidence    *float64
	Comment       *string
}

// UpdateMapping updates an existing custom mapping.
func (s *MappingStore) UpdateMapping(ctx context.Context, input UpdateMappingInput) (*CustomMapping, error) {
	// Build dynamic update query
	var setClauses []string
	var args []interface{}
	argNum := 1

	if input.SourceDisplay != nil {
		setClauses = append(setClauses, fmt.Sprintf("source_display = $%d", argNum))
		args = append(args, nullIfEmpty(*input.SourceDisplay))
		argNum++
	}
	if input.TargetDisplay != nil {
		setClauses = append(setClauses, fmt.Sprintf("target_display = $%d", argNum))
		args = append(args, nullIfEmpty(*input.TargetDisplay))
		argNum++
	}
	if input.Equivalence != nil {
		setClauses = append(setClauses, fmt.Sprintf("equivalence = $%d", argNum))
		args = append(args, *input.Equivalence)
		argNum++
	}
	if input.Confidence != nil {
		setClauses = append(setClauses, fmt.Sprintf("confidence = $%d", argNum))
		args = append(args, *input.Confidence)
		argNum++
	}
	if input.Comment != nil {
		setClauses = append(setClauses, fmt.Sprintf("comment = $%d", argNum))
		args = append(args, nullIfEmpty(*input.Comment))
		argNum++
	}

	if len(setClauses) == 0 {
		// No fields to update, just return the existing mapping
		return s.GetMapping(ctx, input.ID)
	}

	// Add the ID for the WHERE clause
	args = append(args, input.ID)

	query := fmt.Sprintf(`
		UPDATE terminology.custom_mappings
		SET %s
		WHERE id = $%d
		RETURNING id
	`, strings.Join(setClauses, ", "), argNum)

	var updatedID int64
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&updatedID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("mapping not found: %d", input.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("updating mapping: %w", err)
	}

	// Return the updated mapping
	return s.GetMapping(ctx, updatedID)
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

// =============================================================================
// Pending Autoroute Types and Operations
// =============================================================================

// PendingStatus represents the workflow state of a pending autoroute suggestion.
type PendingStatus string

const (
	StatusPending  PendingStatus = "pending"
	StatusApproved PendingStatus = "approved"
	StatusRejected PendingStatus = "rejected"
	StatusExpired  PendingStatus = "expired"
)

// PendingAutoroute represents an LLM-suggested mapping awaiting human review.
type PendingAutoroute struct {
	ID               int64           `json:"id"`
	SourceSystem     string          `json:"source_system"`
	SourceCode       string          `json:"source_code"`
	SourceDisplay    string          `json:"source_display,omitempty"`
	TargetSystem     string          `json:"target_system"`
	SuggestedCode    string          `json:"suggested_code"`
	SuggestedDisplay string          `json:"suggested_display,omitempty"`
	Confidence       float64         `json:"confidence"`
	Equivalence      string          `json:"equivalence,omitempty"`
	Reasoning        string          `json:"reasoning,omitempty"`
	DecisionTrace    json.RawMessage `json:"decision_trace"`
	Alternates       json.RawMessage `json:"alternates,omitempty"`
	Status           PendingStatus   `json:"status"`
	ReviewedAt       *time.Time      `json:"reviewed_at,omitempty"`
	ReviewedBy       string          `json:"reviewed_by,omitempty"`
	RejectionReason  string          `json:"rejection_reason,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	ExpiresAt        *time.Time      `json:"expires_at,omitempty"`
}

// Alternate represents an alternative mapping candidate considered during autorouting.
type Alternate struct {
	Code       string  `json:"code"`
	Display    string  `json:"display,omitempty"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning,omitempty"`
}

// CreatePendingAutoroute inserts a new pending autoroute suggestion.
func (s *MappingStore) CreatePendingAutoroute(ctx context.Context, p *PendingAutoroute) error {
	var expiresAt interface{}
	if p.ExpiresAt != nil {
		expiresAt = *p.ExpiresAt
	}

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO terminology.pending_autoroutes
			(source_system, source_code, source_display, target_system,
			 suggested_code, suggested_display, confidence, equivalence, reasoning,
			 decision_trace, alternates, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (source_system, source_code, target_system, suggested_code)
		DO UPDATE SET
			confidence = EXCLUDED.confidence,
			reasoning = EXCLUDED.reasoning,
			decision_trace = EXCLUDED.decision_trace,
			alternates = EXCLUDED.alternates,
			status = 'pending',
			reviewed_at = NULL,
			reviewed_by = NULL,
			rejection_reason = NULL,
			created_at = NOW()
		RETURNING id, created_at
	`,
		p.SourceSystem, p.SourceCode, nullIfEmpty(p.SourceDisplay), p.TargetSystem,
		p.SuggestedCode, nullIfEmpty(p.SuggestedDisplay), p.Confidence,
		nullIfEmpty(p.Equivalence), nullIfEmpty(p.Reasoning),
		nullJSON(p.DecisionTrace), nullJSON(p.Alternates), StatusPending, expiresAt,
	).Scan(&p.ID, &p.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating pending autoroute: %w", err)
	}
	p.Status = StatusPending
	return nil
}

// GetPendingAutoroute retrieves a pending autoroute by ID.
func (s *MappingStore) GetPendingAutoroute(ctx context.Context, id int64) (*PendingAutoroute, error) {
	var p PendingAutoroute
	var sourceDisplay, suggestedDisplay, equivalence, reasoning, reviewedBy, rejectionReason sql.NullString
	var decisionTrace, alternates []byte
	var reviewedAt, expiresAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, source_system, source_code, source_display, target_system,
		       suggested_code, suggested_display, confidence, equivalence, reasoning,
		       decision_trace, alternates, status, reviewed_at, reviewed_by,
		       rejection_reason, created_at, expires_at
		FROM terminology.pending_autoroutes
		WHERE id = $1
	`, id).Scan(
		&p.ID, &p.SourceSystem, &p.SourceCode, &sourceDisplay, &p.TargetSystem,
		&p.SuggestedCode, &suggestedDisplay, &p.Confidence, &equivalence, &reasoning,
		&decisionTrace, &alternates, &p.Status, &reviewedAt, &reviewedBy,
		&rejectionReason, &p.CreatedAt, &expiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting pending autoroute: %w", err)
	}

	p.SourceDisplay = sourceDisplay.String
	p.SuggestedDisplay = suggestedDisplay.String
	p.Equivalence = equivalence.String
	p.Reasoning = reasoning.String
	p.ReviewedBy = reviewedBy.String
	p.RejectionReason = rejectionReason.String
	if len(decisionTrace) > 0 {
		p.DecisionTrace = decisionTrace
	}
	if len(alternates) > 0 {
		p.Alternates = alternates
	}
	if reviewedAt.Valid {
		p.ReviewedAt = &reviewedAt.Time
	}
	if expiresAt.Valid {
		p.ExpiresAt = &expiresAt.Time
	}

	return &p, nil
}

// ListPendingAutoroutesFilter provides options for filtering pending autoroutes.
type ListPendingAutoroutesFilter struct {
	Status        PendingStatus
	MinConfidence *float64
	SourceSystem  string
	TargetSystem  string
	Limit         int
	Offset        int
}

// ListPendingAutoroutes returns pending autoroutes matching the filter.
func (s *MappingStore) ListPendingAutoroutes(ctx context.Context, filter ListPendingAutoroutesFilter) ([]*PendingAutoroute, int, error) {
	// Build query with filters
	query := `
		SELECT id, source_system, source_code, source_display, target_system,
		       suggested_code, suggested_display, confidence, equivalence, reasoning,
		       decision_trace, alternates, status, reviewed_at, reviewed_by,
		       rejection_reason, created_at, expires_at
		FROM terminology.pending_autoroutes
		WHERE 1=1
	`
	countQuery := `SELECT COUNT(*) FROM terminology.pending_autoroutes WHERE 1=1`

	var args []interface{}
	argNum := 1

	if filter.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argNum)
		countQuery += fmt.Sprintf(" AND status = $%d", argNum)
		args = append(args, filter.Status)
		argNum++
	}
	if filter.MinConfidence != nil {
		query += fmt.Sprintf(" AND confidence >= $%d", argNum)
		countQuery += fmt.Sprintf(" AND confidence >= $%d", argNum)
		args = append(args, *filter.MinConfidence)
		argNum++
	}
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

	// Get total count
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting pending autoroutes: %w", err)
	}

	// Add ordering and pagination
	query += " ORDER BY confidence DESC, created_at DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying pending autoroutes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*PendingAutoroute
	for rows.Next() {
		var p PendingAutoroute
		var sourceDisplay, suggestedDisplay, equivalence, reasoning, reviewedBy, rejectionReason sql.NullString
		var decisionTrace, alternates []byte
		var reviewedAt, expiresAt sql.NullTime

		err := rows.Scan(
			&p.ID, &p.SourceSystem, &p.SourceCode, &sourceDisplay, &p.TargetSystem,
			&p.SuggestedCode, &suggestedDisplay, &p.Confidence, &equivalence, &reasoning,
			&decisionTrace, &alternates, &p.Status, &reviewedAt, &reviewedBy,
			&rejectionReason, &p.CreatedAt, &expiresAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning pending autoroute: %w", err)
		}

		p.SourceDisplay = sourceDisplay.String
		p.SuggestedDisplay = suggestedDisplay.String
		p.Equivalence = equivalence.String
		p.Reasoning = reasoning.String
		p.ReviewedBy = reviewedBy.String
		p.RejectionReason = rejectionReason.String
		if len(decisionTrace) > 0 {
			p.DecisionTrace = decisionTrace
		}
		if len(alternates) > 0 {
			p.Alternates = alternates
		}
		if reviewedAt.Valid {
			p.ReviewedAt = &reviewedAt.Time
		}
		if expiresAt.Valid {
			p.ExpiresAt = &expiresAt.Time
		}

		results = append(results, &p)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating pending autoroutes: %w", err)
	}

	return results, total, nil
}

// ApprovePendingAutoroute approves a pending suggestion and creates a persistent mapping.
// Returns the created mapping.
func (s *MappingStore) ApprovePendingAutoroute(ctx context.Context, id int64, approvedBy string, equivalenceOverride string, comment string) (*CustomMapping, error) {
	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Get the pending autoroute
	var p PendingAutoroute
	var sourceDisplay, suggestedDisplay, equivalence, reasoning sql.NullString
	var decisionTrace []byte

	err = tx.QueryRowContext(ctx, `
		SELECT id, source_system, source_code, source_display, target_system,
		       suggested_code, suggested_display, confidence, equivalence, reasoning,
		       decision_trace, status
		FROM terminology.pending_autoroutes
		WHERE id = $1
		FOR UPDATE
	`, id).Scan(
		&p.ID, &p.SourceSystem, &p.SourceCode, &sourceDisplay, &p.TargetSystem,
		&p.SuggestedCode, &suggestedDisplay, &p.Confidence, &equivalence, &reasoning,
		&decisionTrace, &p.Status,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("pending autoroute not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("getting pending autoroute: %w", err)
	}

	if p.Status != StatusPending {
		return nil, fmt.Errorf("pending autoroute already %s", p.Status)
	}

	p.SourceDisplay = sourceDisplay.String
	p.SuggestedDisplay = suggestedDisplay.String
	p.Equivalence = equivalence.String
	p.Reasoning = reasoning.String
	if len(decisionTrace) > 0 {
		p.DecisionTrace = decisionTrace
	}

	// Determine equivalence to use
	finalEquivalence := MappingEquivalence(p.Equivalence)
	if equivalenceOverride != "" {
		finalEquivalence = MappingEquivalence(equivalenceOverride)
	}
	if finalEquivalence == "" {
		finalEquivalence = EquivalenceEquivalent
	}

	// Create the persistent mapping
	now := time.Now()
	mapping := &CustomMapping{
		SourceSystem:  p.SourceSystem,
		SourceCode:    p.SourceCode,
		SourceDisplay: p.SourceDisplay,
		TargetSystem:  p.TargetSystem,
		TargetCode:    p.SuggestedCode,
		TargetDisplay: p.SuggestedDisplay,
		Equivalence:   finalEquivalence,
		Confidence:    &p.Confidence,
		Comment:       comment,
		Origin:        OriginApprovedAutoroute,
		ApprovedAt:    &now,
		ApprovedBy:    approvedBy,
		DecisionTrace: p.DecisionTrace,
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO terminology.custom_mappings
			(source_system, source_code, source_display, target_system, target_code, target_display,
			 equivalence, confidence, comment, origin, profile_id, created_by, approved_at, approved_by, decision_trace)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (source_system, source_code, target_system, COALESCE(profile_id, ''))
		DO UPDATE SET
			target_code = EXCLUDED.target_code,
			target_display = EXCLUDED.target_display,
			equivalence = EXCLUDED.equivalence,
			confidence = EXCLUDED.confidence,
			comment = EXCLUDED.comment,
			origin = EXCLUDED.origin,
			approved_at = EXCLUDED.approved_at,
			approved_by = EXCLUDED.approved_by,
			decision_trace = EXCLUDED.decision_trace
		RETURNING id, created_at
	`,
		mapping.SourceSystem, mapping.SourceCode, nullIfEmpty(mapping.SourceDisplay),
		mapping.TargetSystem, mapping.TargetCode, nullIfEmpty(mapping.TargetDisplay),
		mapping.Equivalence, mapping.Confidence, nullIfEmpty(mapping.Comment),
		mapping.Origin, nil, nullIfEmpty(approvedBy), mapping.ApprovedAt, nullIfEmpty(approvedBy),
		nullJSON(mapping.DecisionTrace),
	).Scan(&mapping.ID, &mapping.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating mapping from autoroute: %w", err)
	}

	// Update the pending autoroute status
	_, err = tx.ExecContext(ctx, `
		UPDATE terminology.pending_autoroutes
		SET status = $2, reviewed_at = $3, reviewed_by = $4
		WHERE id = $1
	`, id, StatusApproved, now, nullIfEmpty(approvedBy))
	if err != nil {
		return nil, fmt.Errorf("updating pending autoroute status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return mapping, nil
}

// RejectPendingAutoroute rejects a pending suggestion with a reason.
func (s *MappingStore) RejectPendingAutoroute(ctx context.Context, id int64, rejectedBy string, reason string) error {
	now := time.Now()
	result, err := s.db.ExecContext(ctx, `
		UPDATE terminology.pending_autoroutes
		SET status = $2, reviewed_at = $3, reviewed_by = $4, rejection_reason = $5
		WHERE id = $1 AND status = 'pending'
	`, id, StatusRejected, now, nullIfEmpty(rejectedBy), nullIfEmpty(reason))
	if err != nil {
		return fmt.Errorf("rejecting pending autoroute: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("pending autoroute not found or already reviewed: %d", id)
	}

	return nil
}

// BulkApprovePendingAutoroutes approves all pending autoroutes above a confidence threshold.
// Returns the number approved and the created mappings.
func (s *MappingStore) BulkApprovePendingAutoroutes(ctx context.Context, minConfidence float64, maxCount int, approvedBy string) (int, []*CustomMapping, error) {
	// Get pending autoroutes above threshold
	pending, _, err := s.ListPendingAutoroutes(ctx, ListPendingAutoroutesFilter{
		Status:        StatusPending,
		MinConfidence: &minConfidence,
		Limit:         maxCount,
	})
	if err != nil {
		return 0, nil, fmt.Errorf("listing pending autoroutes: %w", err)
	}

	var approved int
	var mappings []*CustomMapping
	for _, p := range pending {
		mapping, err := s.ApprovePendingAutoroute(ctx, p.ID, approvedBy, "", "")
		if err != nil {
			// Skip individual failures in bulk operations
			continue
		}
		approved++
		mappings = append(mappings, mapping)
	}

	return approved, mappings, nil
}

// ExpirePendingAutoroutes marks old pending autoroutes as expired.
// Returns the number of autoroutes expired.
func (s *MappingStore) ExpirePendingAutoroutes(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE terminology.pending_autoroutes
		SET status = 'expired'
		WHERE status = 'pending' AND expires_at IS NOT NULL AND expires_at < NOW()
	`)
	if err != nil {
		return 0, fmt.Errorf("expiring pending autoroutes: %w", err)
	}
	return result.RowsAffected()
}

// CountPendingAutoroutes returns counts of pending autoroutes by status.
func (s *MappingStore) CountPendingAutoroutes(ctx context.Context) (map[PendingStatus]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM terminology.pending_autoroutes GROUP BY status
	`)
	if err != nil {
		return nil, fmt.Errorf("counting pending autoroutes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[PendingStatus]int)
	for rows.Next() {
		var status PendingStatus
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scanning count: %w", err)
		}
		counts[status] = count
	}

	return counts, rows.Err()
}

// =============================================================================
// Mapping Decision Telemetry Types and Operations
// =============================================================================

// DecisionType classifies the result of a mapping resolution.
type DecisionType string

const (
	DecisionPersistentHit     DecisionType = "PERSISTENT_HIT"
	DecisionAutorouteHighConf DecisionType = "AUTOROUTE_HIGH_CONF"
	DecisionAutorouteMedConf  DecisionType = "AUTOROUTE_MED_CONF"
	DecisionAutorouteLowConf  DecisionType = "AUTOROUTE_LOW_CONF"
	DecisionNoMatch           DecisionType = "NO_MATCH"
)

// MappingDecision records telemetry for a mapping resolution attempt.
type MappingDecision struct {
	ID              int64           `json:"id"`
	TraceID         string          `json:"trace_id"`
	SourceSystem    string          `json:"source_system"`
	SourceCode      string          `json:"source_code"`
	SourceDisplay   string          `json:"source_display,omitempty"`
	TargetSystem    string          `json:"target_system"`
	DecisionType    DecisionType    `json:"decision_type"`
	Confidence      *float64        `json:"confidence,omitempty"`
	SelectedCode    string          `json:"selected_code,omitempty"`
	SelectedDisplay string          `json:"selected_display,omitempty"`
	DecisionTree    json.RawMessage `json:"decision_tree"`
	ProfileID       string          `json:"profile_id,omitempty"`
	RequestSource   string          `json:"request_source"` // graphql, cli, workflow, batch
	CreatedAt       time.Time       `json:"created_at"`
	DurationMs      int             `json:"duration_ms"`
}

// RecordMappingDecision inserts a decision telemetry record.
func (s *MappingStore) RecordMappingDecision(ctx context.Context, d *MappingDecision) error {
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO terminology.mapping_decisions
			(trace_id, source_system, source_code, source_display, target_system,
			 decision_type, confidence, selected_code, selected_display,
			 decision_tree, profile_id, request_source, duration_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at
	`,
		d.TraceID, d.SourceSystem, d.SourceCode, nullIfEmpty(d.SourceDisplay), d.TargetSystem,
		d.DecisionType, d.Confidence, nullIfEmpty(d.SelectedCode), nullIfEmpty(d.SelectedDisplay),
		nullJSON(d.DecisionTree), nullIfEmpty(d.ProfileID), nullIfEmpty(d.RequestSource), d.DurationMs,
	).Scan(&d.ID, &d.CreatedAt)
	if err != nil {
		return fmt.Errorf("recording mapping decision: %w", err)
	}
	return nil
}

// GetMappingDecision retrieves a decision by ID.
func (s *MappingStore) GetMappingDecision(ctx context.Context, id int64) (*MappingDecision, error) {
	var d MappingDecision
	var sourceDisplay, selectedCode, selectedDisplay, profileID, requestSource sql.NullString
	var confidence sql.NullFloat64
	var decisionTree []byte

	err := s.db.QueryRowContext(ctx, `
		SELECT id, trace_id, source_system, source_code, source_display, target_system,
		       decision_type, confidence, selected_code, selected_display,
		       decision_tree, profile_id, request_source, created_at, duration_ms
		FROM terminology.mapping_decisions
		WHERE id = $1
	`, id).Scan(
		&d.ID, &d.TraceID, &d.SourceSystem, &d.SourceCode, &sourceDisplay, &d.TargetSystem,
		&d.DecisionType, &confidence, &selectedCode, &selectedDisplay,
		&decisionTree, &profileID, &requestSource, &d.CreatedAt, &d.DurationMs,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting mapping decision: %w", err)
	}

	d.SourceDisplay = sourceDisplay.String
	d.SelectedCode = selectedCode.String
	d.SelectedDisplay = selectedDisplay.String
	d.ProfileID = profileID.String
	d.RequestSource = requestSource.String
	if confidence.Valid {
		d.Confidence = &confidence.Float64
	}
	if len(decisionTree) > 0 {
		d.DecisionTree = decisionTree
	}

	return &d, nil
}

// ListMappingDecisionsFilter provides options for filtering decisions.
type ListMappingDecisionsFilter struct {
	DecisionType DecisionType
	SourceSystem string
	SourceCode   string
	ProfileID    string
	TraceID      string
	Since        *time.Time
	Until        *time.Time
	Limit        int
	Offset       int
}

// ListMappingDecisions returns decisions matching the filter.
func (s *MappingStore) ListMappingDecisions(ctx context.Context, filter ListMappingDecisionsFilter) ([]*MappingDecision, int, error) {
	query := `
		SELECT id, trace_id, source_system, source_code, source_display, target_system,
		       decision_type, confidence, selected_code, selected_display,
		       decision_tree, profile_id, request_source, created_at, duration_ms
		FROM terminology.mapping_decisions
		WHERE 1=1
	`
	countQuery := `SELECT COUNT(*) FROM terminology.mapping_decisions WHERE 1=1`

	var args []interface{}
	argNum := 1

	if filter.DecisionType != "" {
		query += fmt.Sprintf(" AND decision_type = $%d", argNum)
		countQuery += fmt.Sprintf(" AND decision_type = $%d", argNum)
		args = append(args, filter.DecisionType)
		argNum++
	}
	if filter.SourceSystem != "" {
		query += fmt.Sprintf(" AND source_system = $%d", argNum)
		countQuery += fmt.Sprintf(" AND source_system = $%d", argNum)
		args = append(args, filter.SourceSystem)
		argNum++
	}
	if filter.SourceCode != "" {
		query += fmt.Sprintf(" AND source_code = $%d", argNum)
		countQuery += fmt.Sprintf(" AND source_code = $%d", argNum)
		args = append(args, filter.SourceCode)
		argNum++
	}
	if filter.ProfileID != "" {
		query += fmt.Sprintf(" AND profile_id = $%d", argNum)
		countQuery += fmt.Sprintf(" AND profile_id = $%d", argNum)
		args = append(args, filter.ProfileID)
		argNum++
	}
	if filter.TraceID != "" {
		query += fmt.Sprintf(" AND trace_id = $%d", argNum)
		countQuery += fmt.Sprintf(" AND trace_id = $%d", argNum)
		args = append(args, filter.TraceID)
		argNum++
	}
	if filter.Since != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argNum)
		countQuery += fmt.Sprintf(" AND created_at >= $%d", argNum)
		args = append(args, *filter.Since)
		argNum++
	}
	if filter.Until != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argNum)
		countQuery += fmt.Sprintf(" AND created_at <= $%d", argNum)
		args = append(args, *filter.Until)
		argNum++
	}

	// Get total count
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting mapping decisions: %w", err)
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
		return nil, 0, fmt.Errorf("querying mapping decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*MappingDecision
	for rows.Next() {
		var d MappingDecision
		var sourceDisplay, selectedCode, selectedDisplay, profileID, requestSource sql.NullString
		var confidence sql.NullFloat64
		var decisionTree []byte

		err := rows.Scan(
			&d.ID, &d.TraceID, &d.SourceSystem, &d.SourceCode, &sourceDisplay, &d.TargetSystem,
			&d.DecisionType, &confidence, &selectedCode, &selectedDisplay,
			&decisionTree, &profileID, &requestSource, &d.CreatedAt, &d.DurationMs,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning mapping decision: %w", err)
		}

		d.SourceDisplay = sourceDisplay.String
		d.SelectedCode = selectedCode.String
		d.SelectedDisplay = selectedDisplay.String
		d.ProfileID = profileID.String
		d.RequestSource = requestSource.String
		if confidence.Valid {
			d.Confidence = &confidence.Float64
		}
		if len(decisionTree) > 0 {
			d.DecisionTree = decisionTree
		}

		results = append(results, &d)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating mapping decisions: %w", err)
	}

	return results, total, nil
}

// DecisionStats provides aggregated statistics about mapping decisions.
type DecisionStats struct {
	TotalDecisions  int                  `json:"total_decisions"`
	DecisionsByType map[DecisionType]int `json:"decisions_by_type"`
	AvgDurationMs   float64              `json:"avg_duration_ms"`
	AvgConfidence   *float64             `json:"avg_confidence,omitempty"`
	Since           time.Time            `json:"since"`
	Until           time.Time            `json:"until"`
}

// GetDecisionStats returns aggregated statistics for a time period.
func (s *MappingStore) GetDecisionStats(ctx context.Context, since, until time.Time) (*DecisionStats, error) {
	stats := &DecisionStats{
		DecisionsByType: make(map[DecisionType]int),
		Since:           since,
		Until:           until,
	}

	// Get totals by type
	rows, err := s.db.QueryContext(ctx, `
		SELECT decision_type, COUNT(*), AVG(duration_ms), AVG(confidence)
		FROM terminology.mapping_decisions
		WHERE created_at >= $1 AND created_at <= $2
		GROUP BY decision_type
	`, since, until)
	if err != nil {
		return nil, fmt.Errorf("querying decision stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var totalDuration float64
	var confidenceSum float64
	var confidenceCount int

	for rows.Next() {
		var decisionType DecisionType
		var count int
		var avgDuration sql.NullFloat64
		var avgConfidence sql.NullFloat64

		if err := rows.Scan(&decisionType, &count, &avgDuration, &avgConfidence); err != nil {
			return nil, fmt.Errorf("scanning stats: %w", err)
		}

		stats.DecisionsByType[decisionType] = count
		stats.TotalDecisions += count

		if avgDuration.Valid {
			totalDuration += avgDuration.Float64 * float64(count)
		}
		if avgConfidence.Valid {
			confidenceSum += avgConfidence.Float64 * float64(count)
			confidenceCount += count
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating stats: %w", err)
	}

	if stats.TotalDecisions > 0 {
		stats.AvgDurationMs = totalDuration / float64(stats.TotalDecisions)
	}
	if confidenceCount > 0 {
		avgConf := confidenceSum / float64(confidenceCount)
		stats.AvgConfidence = &avgConf
	}

	return stats, nil
}

// CleanupOldDecisions removes decisions older than the specified duration.
// Returns the number of decisions deleted.
func (s *MappingStore) CleanupOldDecisions(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM terminology.mapping_decisions
		WHERE created_at < $1
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleaning up old decisions: %w", err)
	}
	return result.RowsAffected()
}
