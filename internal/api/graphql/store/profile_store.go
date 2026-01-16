package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ProfileStore provides persistent storage for source profiles.
type ProfileStore interface {
	// InitSchema creates the profile tables if they don't exist.
	InitSchema(ctx context.Context) error

	// CreateProfile creates a new profile.
	CreateProfile(ctx context.Context, profile *Profile) error

	// GetProfile retrieves a profile by ID.
	GetProfile(ctx context.Context, id string) (*Profile, error)

	// ListProfiles retrieves all profiles.
	ListProfiles(ctx context.Context, activeOnly bool) ([]*Profile, error)

	// UpdateProfile updates an existing profile.
	UpdateProfile(ctx context.Context, profile *Profile) error

	// DeleteProfile marks a profile as inactive.
	DeleteProfile(ctx context.Context, id string) error

	// GetProfileRevisions retrieves revision history for a profile.
	GetProfileRevisions(ctx context.Context, id string) ([]*ProfileRevision, error)

	// DuplicateProfile creates a copy of an existing profile with a new ID and name.
	DuplicateProfile(ctx context.Context, sourceID, newID, newName string) (*Profile, error)
}

// Profile represents a source profile configuration.
type Profile struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Version   string          `json:"version"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	CreatedBy string          `json:"created_by,omitempty"`
	IsActive  bool            `json:"is_active"`
}

// ProfileConfig represents the full profile configuration structure.
type ProfileConfig struct {
	HL7v2       *HL7v2Config       `json:"hl7v2,omitempty"`
	Identifiers *IdentifierConfig  `json:"identifiers,omitempty"`
	Terminology *TerminologyConfig `json:"terminology,omitempty"`
}

// HL7v2Config contains HL7v2 parsing settings.
type HL7v2Config struct {
	DefaultVersion       string           `json:"default_version"`
	Timezone             string           `json:"timezone"`
	Tolerance            *ToleranceConfig `json:"tolerance,omitempty"`
	EventClassifications []EventClassRule `json:"event_classifications,omitempty"`
}

// ToleranceConfig defines what parsing anomalies to tolerate.
type ToleranceConfig struct {
	MissingSegments       []string `json:"missing_segments,omitempty"`
	NTEAnywhere           bool     `json:"nte_anywhere"`
	ExtraComponents       bool     `json:"extra_components"`
	UnknownSegments       bool     `json:"unknown_segments"`
	NonStandardDelimiters bool     `json:"non_standard_delimiters"`
}

// EventClassRule maps message types to semantic event types.
type EventClassRule struct {
	MessageType string `json:"message_type"`
	Condition   string `json:"condition,omitempty"`
	EventType   string `json:"event_type"`
	Priority    int    `json:"priority"`
}

// IdentifierConfig contains identifier handling settings.
type IdentifierConfig struct {
	AssigningAuthorities []AssigningAuthority `json:"assigning_authorities,omitempty"`
	PrimaryIDPreference  []IDPreferenceRule   `json:"primary_id_preference,omitempty"`
	Validation           *ValidationConfig    `json:"validation,omitempty"`
	Normalization        *NormalizationConfig `json:"normalization,omitempty"`
}

// AssigningAuthority maps local AA codes to standard systems.
type AssigningAuthority struct {
	Code   string `json:"code"`
	System string `json:"system"`
	Name   string `json:"name,omitempty"`
}

// IDPreferenceRule defines priority for selecting primary IDs.
type IDPreferenceRule struct {
	Type             string `json:"type"`
	AssignerContains string `json:"assigner_contains,omitempty"`
	Priority         int    `json:"priority"`
}

// ValidationConfig defines identifier validation settings.
type ValidationConfig struct {
	NPI *ValidatorSetting `json:"npi,omitempty"`
	MBI *ValidatorSetting `json:"mbi,omitempty"`
	SSN *ValidatorSetting `json:"ssn,omitempty"`
}

// ValidatorSetting configures a single validator.
type ValidatorSetting struct {
	Enabled   bool   `json:"enabled"`
	OnInvalid string `json:"on_invalid"` // "error", "warn", "pass"
}

// NormalizationConfig defines identifier normalization settings.
type NormalizationConfig struct {
	SSNStripDashes    bool     `json:"ssn_strip_dashes"`
	SSNRejectPatterns []string `json:"ssn_reject_patterns,omitempty"`
	PhoneNormalize    bool     `json:"phone_normalize"`
	PhoneFormat       string   `json:"phone_format,omitempty"`
}

// TerminologyConfig contains terminology mapping settings.
type TerminologyConfig struct {
	Mappings []TerminologyMapping `json:"mappings,omitempty"`
}

// TerminologyMapping defines a source-to-target code mapping table.
type TerminologyMapping struct {
	ID           string             `json:"id"`
	SourceSystem string             `json:"source_system"`
	TargetSystem string             `json:"target_system"`
	Entries      []TerminologyEntry `json:"entries,omitempty"`
}

// TerminologyEntry maps a single code.
type TerminologyEntry struct {
	SourceCode string `json:"source_code"`
	TargetCode string `json:"target_code"`
	Display    string `json:"display,omitempty"`
}

// ProfileRevision represents a historical version of a profile.
type ProfileRevision struct {
	ID            int             `json:"id"`
	ProfileID     string          `json:"profile_id"`
	Version       string          `json:"version"`
	Config        json.RawMessage `json:"config"`
	CreatedAt     time.Time       `json:"created_at"`
	CreatedBy     string          `json:"created_by,omitempty"`
	ChangeSummary string          `json:"change_summary,omitempty"`
}

// PostgresProfileStore is a PostgreSQL-backed profile store.
type PostgresProfileStore struct {
	db *sql.DB
}

// NewPostgresProfileStore creates a new PostgreSQL profile store.
func NewPostgresProfileStore(db *sql.DB) *PostgresProfileStore {
	return &PostgresProfileStore{db: db}
}

// InitSchema creates the profile tables and indexes.
func (s *PostgresProfileStore) InitSchema(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS source_profiles (
			id VARCHAR(64) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			version VARCHAR(32) NOT NULL DEFAULT '1.0.0',
			config JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_by VARCHAR(255),
			is_active BOOLEAN NOT NULL DEFAULT true
		);

		CREATE TABLE IF NOT EXISTS profile_revisions (
			id SERIAL PRIMARY KEY,
			profile_id VARCHAR(64) REFERENCES source_profiles(id) ON DELETE CASCADE,
			version VARCHAR(32) NOT NULL,
			config JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_by VARCHAR(255),
			change_summary TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_profiles_active ON source_profiles(is_active) WHERE is_active = true;
		CREATE INDEX IF NOT EXISTS idx_revisions_profile ON profile_revisions(profile_id);
	`

	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// CreateProfile creates a new profile.
func (s *PostgresProfileStore) CreateProfile(ctx context.Context, profile *Profile) error {
	if profile.Version == "" {
		profile.Version = "1.0.0"
	}
	if profile.Config == nil {
		profile.Config = json.RawMessage("{}")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO source_profiles (id, name, version, config, created_by, is_active)
		VALUES ($1, $2, $3, $4, $5, true)
	`, profile.ID, profile.Name, profile.Version, profile.Config, profile.CreatedBy)

	if err != nil {
		return fmt.Errorf("creating profile: %w", err)
	}

	return nil
}

// GetProfile retrieves a profile by ID.
func (s *PostgresProfileStore) GetProfile(ctx context.Context, id string) (*Profile, error) {
	var profile Profile
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, version, config, created_at, updated_at, created_by, is_active
		FROM source_profiles
		WHERE id = $1
	`, id).Scan(
		&profile.ID,
		&profile.Name,
		&profile.Version,
		&profile.Config,
		&profile.CreatedAt,
		&profile.UpdatedAt,
		&profile.CreatedBy,
		&profile.IsActive,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting profile: %w", err)
	}

	return &profile, nil
}

// ListProfiles retrieves all profiles.
func (s *PostgresProfileStore) ListProfiles(ctx context.Context, activeOnly bool) ([]*Profile, error) {
	query := `
		SELECT id, name, version, config, created_at, updated_at, created_by, is_active
		FROM source_profiles
	`
	if activeOnly {
		query += " WHERE is_active = true"
	}
	query += " ORDER BY name ASC"

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing profiles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var profiles []*Profile
	for rows.Next() {
		var p Profile
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Version, &p.Config,
			&p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.IsActive,
		); err != nil {
			return nil, fmt.Errorf("scanning profile: %w", err)
		}
		profiles = append(profiles, &p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating profiles: %w", err)
	}

	return profiles, nil
}

// UpdateProfile updates an existing profile and creates a revision.
func (s *PostgresProfileStore) UpdateProfile(ctx context.Context, profile *Profile) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Get current profile for revision
	var currentVersion string
	var currentConfig json.RawMessage
	err = tx.QueryRowContext(ctx, `
		SELECT version, config FROM source_profiles WHERE id = $1
	`, profile.ID).Scan(&currentVersion, &currentConfig)
	if err != nil {
		return fmt.Errorf("getting current profile: %w", err)
	}

	// Create revision
	_, err = tx.ExecContext(ctx, `
		INSERT INTO profile_revisions (profile_id, version, config, created_by, change_summary)
		VALUES ($1, $2, $3, $4, $5)
	`, profile.ID, currentVersion, currentConfig, profile.CreatedBy, "Updated via UI")
	if err != nil {
		return fmt.Errorf("creating revision: %w", err)
	}

	// Update profile
	_, err = tx.ExecContext(ctx, `
		UPDATE source_profiles
		SET name = $2, version = $3, config = $4, updated_at = NOW()
		WHERE id = $1
	`, profile.ID, profile.Name, profile.Version, profile.Config)
	if err != nil {
		return fmt.Errorf("updating profile: %w", err)
	}

	return tx.Commit()
}

// DeleteProfile marks a profile as inactive (soft delete).
func (s *PostgresProfileStore) DeleteProfile(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE source_profiles SET is_active = false, updated_at = NOW()
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("deleting profile: %w", err)
	}
	return nil
}

// GetProfileRevisions retrieves revision history for a profile.
func (s *PostgresProfileStore) GetProfileRevisions(ctx context.Context, id string) ([]*ProfileRevision, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, profile_id, version, config, created_at, created_by, change_summary
		FROM profile_revisions
		WHERE profile_id = $1
		ORDER BY created_at DESC
	`, id)
	if err != nil {
		return nil, fmt.Errorf("querying revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var revisions []*ProfileRevision
	for rows.Next() {
		var r ProfileRevision
		var createdBy, changeSummary sql.NullString
		if err := rows.Scan(
			&r.ID, &r.ProfileID, &r.Version, &r.Config,
			&r.CreatedAt, &createdBy, &changeSummary,
		); err != nil {
			return nil, fmt.Errorf("scanning revision: %w", err)
		}
		r.CreatedBy = createdBy.String
		r.ChangeSummary = changeSummary.String
		revisions = append(revisions, &r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating revisions: %w", err)
	}

	return revisions, nil
}

// DuplicateProfile creates a copy of an existing profile with a new ID and name.
func (s *PostgresProfileStore) DuplicateProfile(ctx context.Context, sourceID, newID, newName string) (*Profile, error) {
	source, err := s.GetProfile(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("getting source profile: %w", err)
	}
	if source == nil {
		return nil, fmt.Errorf("source profile not found: %s", sourceID)
	}

	newProfile := &Profile{
		ID:        newID,
		Name:      newName,
		Version:   "1.0.0",
		Config:    source.Config,
		CreatedBy: source.CreatedBy,
		IsActive:  true,
	}

	if err := s.CreateProfile(ctx, newProfile); err != nil {
		return nil, fmt.Errorf("creating duplicate: %w", err)
	}

	return s.GetProfile(ctx, newID)
}
