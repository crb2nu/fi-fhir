package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

	// GetProfileRevision retrieves one exact immutable revision owned by a profile.
	GetProfileRevision(ctx context.Context, profileID string, revisionID int) (*ProfileRevision, error)

	// GetCurrentProfileRevision retrieves the immutable revision selected by a profile's current pointer.
	GetCurrentProfileRevision(ctx context.Context, profileID string) (*ProfileRevision, error)

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
	// ChangeSummary is transient revision metadata consumed by UpdateProfile.
	ChangeSummary string `json:"-"`
	// CurrentRevisionID is the immutable revision selected by this mutable profile row.
	CurrentRevisionID int `json:"current_revision_id"`
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting profile schema transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Serialize the upgrade because CREATE TABLE IF NOT EXISTS does not protect the
	// subsequent backfill from two application instances starting concurrently.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('fi-fhir-profile-schema'))`); err != nil {
		return fmt.Errorf("locking profile schema upgrade: %w", err)
	}

	schema := `
		CREATE TABLE IF NOT EXISTS source_profiles (
			id VARCHAR(64) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			version VARCHAR(32) NOT NULL DEFAULT '1.0.0',
			config JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_by VARCHAR(255),
			is_active BOOLEAN NOT NULL DEFAULT true,
			current_revision_id INTEGER
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

		ALTER TABLE source_profiles
			ADD COLUMN IF NOT EXISTS current_revision_id INTEGER;

		LOCK TABLE source_profiles IN SHARE ROW EXCLUSIVE MODE;

		WITH profiles_without_pointer AS (
			SELECT id, version, config, updated_at, created_by
			FROM source_profiles
			WHERE current_revision_id IS NULL
			FOR UPDATE
		), inserted_revisions AS (
			INSERT INTO profile_revisions (
				profile_id, version, config, created_at, created_by, change_summary
			)
			SELECT id, version, config, updated_at, created_by, 'Backfilled current revision'
			FROM profiles_without_pointer
			RETURNING id, profile_id
		)
		UPDATE source_profiles AS profile
		SET current_revision_id = revision.id
		FROM inserted_revisions AS revision
		WHERE profile.id = revision.profile_id;

		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conrelid = 'profile_revisions'::regclass
				  AND conname = 'profile_revisions_profile_id_id_key'
			) THEN
				ALTER TABLE profile_revisions
					ADD CONSTRAINT profile_revisions_profile_id_id_key
					UNIQUE (profile_id, id);
			END IF;
		END
		$$;

		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conrelid = 'source_profiles'::regclass
				  AND conname = 'source_profiles_current_revision_fk'
			) THEN
				ALTER TABLE source_profiles
					ADD CONSTRAINT source_profiles_current_revision_fk
					FOREIGN KEY (id, current_revision_id)
					REFERENCES profile_revisions(profile_id, id)
					DEFERRABLE INITIALLY DEFERRED;
			END IF;
		END
		$$;

		CREATE OR REPLACE FUNCTION fi_fhir_profile_after_insert_revision()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		DECLARE
			revision_id INTEGER;
		BEGIN
			IF NEW.current_revision_id IS NOT NULL THEN
				RETURN NEW;
			END IF;

			INSERT INTO profile_revisions (
				profile_id, version, config, created_at, created_by, change_summary
			)
			VALUES (
				NEW.id, NEW.version, NEW.config, NEW.created_at, NEW.created_by,
				'Initial revision'
			)
			RETURNING id INTO revision_id;

			UPDATE source_profiles
			SET current_revision_id = revision_id
			WHERE id = NEW.id;
			RETURN NEW;
		END
		$$;

		CREATE OR REPLACE FUNCTION fi_fhir_profile_before_update_revision()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		DECLARE
			pointer_matches BOOLEAN;
			revision_actor TEXT;
			revision_summary TEXT;
			revision_id INTEGER;
		BEGIN
			SELECT EXISTS (
				SELECT 1
				FROM profile_revisions AS revision
				WHERE revision.profile_id = NEW.id
				  AND revision.id = NEW.current_revision_id
				  AND revision.version = NEW.version
				  AND revision.config = NEW.config
			) INTO pointer_matches;

			IF pointer_matches THEN
				RETURN NEW;
			END IF;

			IF OLD.version IS DISTINCT FROM NEW.version
			   OR OLD.config IS DISTINCT FROM NEW.config
			   OR NEW.current_revision_id IS NULL THEN
				revision_actor := COALESCE(
					NULLIF(current_setting('fi_fhir.profile_actor', true), ''),
					NEW.created_by
				);
				revision_summary := COALESCE(
					NULLIF(current_setting('fi_fhir.profile_change_summary', true), ''),
					'Profile updated'
				);
				INSERT INTO profile_revisions (
					profile_id, version, config, created_at, created_by, change_summary
				)
				VALUES (
					NEW.id, NEW.version, NEW.config, NEW.updated_at, revision_actor,
					revision_summary
				)
				RETURNING id INTO revision_id;
				NEW.current_revision_id := revision_id;
				RETURN NEW;
			END IF;

			RAISE EXCEPTION
				'profile % current revision does not match mutable content', NEW.id
				USING ERRCODE = '23514';
		END
		$$;

		DROP TRIGGER IF EXISTS fi_fhir_profile_after_insert_revision
			ON source_profiles;
		CREATE TRIGGER fi_fhir_profile_after_insert_revision
			AFTER INSERT ON source_profiles
			FOR EACH ROW
			EXECUTE FUNCTION fi_fhir_profile_after_insert_revision();

		DROP TRIGGER IF EXISTS fi_fhir_profile_before_update_revision
			ON source_profiles;
		CREATE TRIGGER fi_fhir_profile_before_update_revision
			BEFORE UPDATE ON source_profiles
			FOR EACH ROW
			EXECUTE FUNCTION fi_fhir_profile_before_update_revision();

		CREATE INDEX IF NOT EXISTS idx_profiles_active
			ON source_profiles(is_active) WHERE is_active = true;
		CREATE INDEX IF NOT EXISTS idx_revisions_profile
			ON profile_revisions(profile_id);
		CREATE INDEX IF NOT EXISTS idx_profiles_current_revision
			ON source_profiles(current_revision_id);
	`

	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("upgrading profile schema: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing profile schema upgrade: %w", err)
	}
	return nil
}

// CreateProfile creates a new profile.
func (s *PostgresProfileStore) CreateProfile(ctx context.Context, profile *Profile) error {
	if profile == nil {
		return fmt.Errorf("creating profile: profile is nil")
	}

	version := profile.Version
	if version == "" {
		version = "1.0.0"
	}
	config := normalizedProfileConfig(profile.Config)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting create profile transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var createdAt, updatedAt time.Time
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO source_profiles (
			id, name, version, config, created_by, is_active, current_revision_id
		)
		VALUES ($1, $2, $3, $4, $5, true, NULL)
		RETURNING created_at, updated_at
	`, profile.ID, profile.Name, version, config, profile.CreatedBy).Scan(&createdAt, &updatedAt); err != nil {
		return fmt.Errorf("creating profile: %w", err)
	}

	var revisionID int
	if err := tx.QueryRowContext(ctx, `
		SELECT current_revision_id, config
		FROM source_profiles
		WHERE id = $1
	`, profile.ID).Scan(&revisionID, &config); err != nil {
		return fmt.Errorf("reading initial profile revision: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing profile creation: %w", err)
	}

	profile.Version = version
	profile.Config = cloneRawMessage(config)
	profile.CreatedAt = createdAt
	profile.UpdatedAt = updatedAt
	profile.IsActive = true
	profile.CurrentRevisionID = revisionID

	return nil
}

// GetProfile retrieves a profile by ID.
func (s *PostgresProfileStore) GetProfile(ctx context.Context, id string) (*Profile, error) {
	var profile Profile
	var createdBy sql.NullString
	var currentRevisionID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, version, config, created_at, updated_at, created_by, is_active,
		       current_revision_id
		FROM source_profiles
		WHERE id = $1
	`, id).Scan(
		&profile.ID,
		&profile.Name,
		&profile.Version,
		&profile.Config,
		&profile.CreatedAt,
		&profile.UpdatedAt,
		&createdBy,
		&profile.IsActive,
		&currentRevisionID,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting profile: %w", err)
	}

	profile.CreatedBy = createdBy.String
	profile.CurrentRevisionID = int(currentRevisionID.Int64)
	profile.Config = cloneRawMessage(profile.Config)
	return &profile, nil
}

// ListProfiles retrieves all profiles.
func (s *PostgresProfileStore) ListProfiles(ctx context.Context, activeOnly bool) ([]*Profile, error) {
	query := `
		SELECT id, name, version, config, created_at, updated_at, created_by, is_active,
		       current_revision_id
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
		var createdBy sql.NullString
		var currentRevisionID sql.NullInt64
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Version, &p.Config,
			&p.CreatedAt, &p.UpdatedAt, &createdBy, &p.IsActive,
			&currentRevisionID,
		); err != nil {
			return nil, fmt.Errorf("scanning profile: %w", err)
		}
		p.CreatedBy = createdBy.String
		p.CurrentRevisionID = int(currentRevisionID.Int64)
		p.Config = cloneRawMessage(p.Config)
		profiles = append(profiles, &p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating profiles: %w", err)
	}

	return profiles, nil
}

// UpdateProfile updates an existing profile and creates a revision.
func (s *PostgresProfileStore) UpdateProfile(ctx context.Context, profile *Profile) error {
	if profile == nil {
		return fmt.Errorf("updating profile: profile is nil")
	}
	profile.ChangeSummary = strings.TrimSpace(profile.ChangeSummary)
	if profile.ChangeSummary == "" || len(profile.ChangeSummary) > 1024 {
		return fmt.Errorf("updating profile: change summary is required and must be at most 1024 bytes")
	}

	version := profile.Version
	if version == "" {
		version = "1.0.0"
	}
	config := normalizedProfileConfig(profile.Config)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the mutable pointer so concurrent updates serialize before allocating
	// and selecting a new immutable current revision.
	var currentRevisionID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT current_revision_id
		FROM source_profiles
		WHERE id = $1
		FOR UPDATE
	`, profile.ID).Scan(&currentRevisionID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("updating profile: profile not found: %s", profile.ID)
	}
	if err != nil {
		return fmt.Errorf("getting current profile: %w", err)
	}
	if !currentRevisionID.Valid || currentRevisionID.Int64 <= 0 {
		return fmt.Errorf("updating profile: profile %s has no current revision", profile.ID)
	}

	if _, err := tx.ExecContext(ctx, `
		SELECT set_config('fi_fhir.profile_actor', $1, true)
	`, profile.CreatedBy); err != nil {
		return fmt.Errorf("setting profile revision actor: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		SELECT set_config('fi_fhir.profile_change_summary', $1, true)
	`, profile.ChangeSummary); err != nil {
		return fmt.Errorf("setting profile revision change summary: %w", err)
	}

	var revisionID int
	var storedConfig json.RawMessage
	var updatedAt time.Time
	err = tx.QueryRowContext(ctx, `
		UPDATE source_profiles
		SET name = $2,
		    version = $3,
		    config = $4,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at, current_revision_id, config
	`, profile.ID, profile.Name, version, config).Scan(&updatedAt, &revisionID, &storedConfig)
	if err != nil {
		return fmt.Errorf("updating profile: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing profile update: %w", err)
	}

	profile.Version = version
	profile.Config = cloneRawMessage(storedConfig)
	profile.UpdatedAt = updatedAt
	profile.CurrentRevisionID = revisionID
	return nil
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
		ORDER BY created_at DESC, id DESC
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
		r.Config = cloneRawMessage(r.Config)
		revisions = append(revisions, &r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating revisions: %w", err)
	}

	return revisions, nil
}

// GetProfileRevision retrieves one exact immutable revision owned by a profile.
func (s *PostgresProfileStore) GetProfileRevision(
	ctx context.Context,
	profileID string,
	revisionID int,
) (*ProfileRevision, error) {
	revision, err := scanProfileRevision(s.db.QueryRowContext(ctx, `
		SELECT id, profile_id, version, config, created_at, created_by, change_summary
		FROM profile_revisions
		WHERE profile_id = $1 AND id = $2
	`, profileID, revisionID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting profile revision: %w", err)
	}
	return revision, nil
}

// GetCurrentProfileRevision retrieves the immutable revision selected by a profile's current pointer.
func (s *PostgresProfileStore) GetCurrentProfileRevision(
	ctx context.Context,
	profileID string,
) (*ProfileRevision, error) {
	revision, err := scanProfileRevision(s.db.QueryRowContext(ctx, `
		SELECT revision.id, revision.profile_id, revision.version, revision.config,
		       revision.created_at, revision.created_by, revision.change_summary
		FROM source_profiles AS profile
		JOIN profile_revisions AS revision
		  ON revision.profile_id = profile.id
		 AND revision.id = profile.current_revision_id
		WHERE profile.id = $1
	`, profileID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting current profile revision: %w", err)
	}
	return revision, nil
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
		Config:    cloneRawMessage(source.Config),
		CreatedBy: source.CreatedBy,
		IsActive:  true,
	}

	if err := s.CreateProfile(ctx, newProfile); err != nil {
		return nil, fmt.Errorf("creating duplicate: %w", err)
	}

	return s.GetProfile(ctx, newID)
}

type profileRevisionScanner interface {
	Scan(dest ...any) error
}

func scanProfileRevision(scanner profileRevisionScanner) (*ProfileRevision, error) {
	var revision ProfileRevision
	var createdBy, changeSummary sql.NullString
	if err := scanner.Scan(
		&revision.ID,
		&revision.ProfileID,
		&revision.Version,
		&revision.Config,
		&revision.CreatedAt,
		&createdBy,
		&changeSummary,
	); err != nil {
		return nil, err
	}
	revision.CreatedBy = createdBy.String
	revision.ChangeSummary = changeSummary.String
	revision.Config = cloneRawMessage(revision.Config)
	return &revision, nil
}

func normalizedProfileConfig(config json.RawMessage) json.RawMessage {
	if config == nil {
		return json.RawMessage("{}")
	}
	return cloneRawMessage(config)
}

func cloneRawMessage(message json.RawMessage) json.RawMessage {
	if message == nil {
		return nil
	}
	return append(json.RawMessage(nil), message...)
}
