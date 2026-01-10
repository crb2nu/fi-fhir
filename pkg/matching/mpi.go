package matching

import (
	"context"
	"time"
)

// MPI defines the Master Patient Index interface.
// An MPI maintains a registry of patient identities and their relationships.
type MPI interface {
	// Add registers a new patient and returns an enterprise patient ID.
	// If the patient matches an existing record, returns the existing ID.
	Add(ctx context.Context, patient *Patient) (*MPIRecord, error)

	// Get retrieves a patient by enterprise ID.
	Get(ctx context.Context, enterpriseID string) (*MPIRecord, error)

	// Search finds patients matching the given criteria.
	Search(ctx context.Context, query *Patient, opts SearchOptions) ([]MPISearchResult, error)

	// Link marks two patient records as the same person.
	Link(ctx context.Context, enterpriseID1, enterpriseID2 string, linkType LinkType) error

	// Unlink removes a link between two patient records.
	Unlink(ctx context.Context, enterpriseID1, enterpriseID2 string) error

	// GetLinks returns all records linked to the given enterprise ID.
	GetLinks(ctx context.Context, enterpriseID string) ([]MPILink, error)

	// Merge combines two patient records into one master.
	// The survivor becomes the master; the victim is marked as merged.
	Merge(ctx context.Context, survivorID, victimID string, reason string) error

	// GetByIdentifier finds a patient by a specific identifier.
	GetByIdentifier(ctx context.Context, idType, idValue, idSystem string) (*MPIRecord, error)

	// Update modifies an existing patient record.
	Update(ctx context.Context, enterpriseID string, patient *Patient) error
}

// MPIRecord represents a patient record in the MPI.
type MPIRecord struct {
	// EnterpriseID is the unique identifier in the MPI.
	EnterpriseID string `json:"enterprise_id"`

	// Patient contains the demographic data.
	Patient *Patient `json:"patient"`

	// Status indicates the record status.
	Status RecordStatus `json:"status"`

	// MergedInto is set if this record was merged into another.
	MergedInto string `json:"merged_into,omitempty"`

	// CreatedAt is when the record was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the record was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// SourceRecords tracks identifiers from contributing systems.
	SourceRecords []SourceRecord `json:"source_records,omitempty"`
}

// RecordStatus indicates the status of an MPI record.
type RecordStatus string

const (
	// StatusActive indicates an active patient record.
	StatusActive RecordStatus = "active"

	// StatusMerged indicates the record was merged into another.
	StatusMerged RecordStatus = "merged"

	// StatusInactive indicates a deactivated record.
	StatusInactive RecordStatus = "inactive"
)

// SourceRecord tracks a patient identifier from a source system.
type SourceRecord struct {
	// Source is the system identifier (e.g., "HOSPITAL_A").
	Source string `json:"source"`

	// IDType is the identifier type (e.g., "MRN").
	IDType string `json:"id_type"`

	// IDValue is the identifier value.
	IDValue string `json:"id_value"`

	// IDSystem is the OID or URI for the identifier system.
	IDSystem string `json:"id_system,omitempty"`

	// AddedAt is when this identifier was added.
	AddedAt time.Time `json:"added_at"`
}

// MPILink represents a link between patient records.
type MPILink struct {
	// EnterpriseID1 is the first patient's enterprise ID.
	EnterpriseID1 string `json:"enterprise_id_1"`

	// EnterpriseID2 is the second patient's enterprise ID.
	EnterpriseID2 string `json:"enterprise_id_2"`

	// LinkType indicates how the records are related.
	LinkType LinkType `json:"link_type"`

	// Confidence is the match confidence (0.0 to 1.0).
	Confidence float64 `json:"confidence"`

	// CreatedAt is when the link was created.
	CreatedAt time.Time `json:"created_at"`

	// CreatedBy identifies who/what created the link.
	CreatedBy string `json:"created_by"`
}

// LinkType defines the relationship between linked records.
type LinkType string

const (
	// LinkConfirmed indicates a human-verified link.
	LinkConfirmed LinkType = "confirmed"

	// LinkAutomatic indicates a system-created link (high confidence).
	LinkAutomatic LinkType = "automatic"

	// LinkPossible indicates a potential link requiring review.
	LinkPossible LinkType = "possible"

	// LinkRejected indicates a reviewed and rejected link.
	LinkRejected LinkType = "rejected"
)

// SearchOptions configures MPI search behavior.
type SearchOptions struct {
	// MinScore is the minimum match score to include.
	MinScore float64

	// MaxResults limits the number of results.
	MaxResults int

	// IncludeMerged includes records that have been merged.
	IncludeMerged bool

	// IncludeInactive includes inactive records.
	IncludeInactive bool
}

// DefaultSearchOptions returns default search options.
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		MinScore:        0.50,
		MaxResults:      10,
		IncludeMerged:   false,
		IncludeInactive: false,
	}
}

// MPISearchResult contains a search result with match details.
type MPISearchResult struct {
	// Record is the matching patient record.
	Record *MPIRecord `json:"record"`

	// MatchResult contains the matching details.
	MatchResult CombinedMatchResult `json:"match_result"`

	// Score is the match score.
	Score float64 `json:"score"`
}

// MPIAuditEvent records an action on the MPI.
type MPIAuditEvent struct {
	// EventType is the type of event.
	EventType string `json:"event_type"` // "add", "update", "link", "unlink", "merge"

	// EnterpriseID is the primary patient ID affected.
	EnterpriseID string `json:"enterprise_id"`

	// SecondaryID is a secondary patient ID (for link/merge).
	SecondaryID string `json:"secondary_id,omitempty"`

	// Actor identifies who performed the action.
	Actor string `json:"actor"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Details contains event-specific information.
	Details map[string]any `json:"details,omitempty"`
}

// MPIMetrics tracks MPI performance metrics.
type MPIMetrics struct {
	// TotalRecords is the count of patient records.
	TotalRecords int64 `json:"total_records"`

	// ActiveRecords is the count of active records.
	ActiveRecords int64 `json:"active_records"`

	// MergedRecords is the count of merged records.
	MergedRecords int64 `json:"merged_records"`

	// TotalLinks is the count of links.
	TotalLinks int64 `json:"total_links"`

	// ConfirmedLinks is the count of confirmed links.
	ConfirmedLinks int64 `json:"confirmed_links"`

	// PossibleLinks is the count of links pending review.
	PossibleLinks int64 `json:"possible_links"`
}
