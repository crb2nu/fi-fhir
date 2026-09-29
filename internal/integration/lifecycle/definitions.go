package lifecycle

import (
	"bytes"
	"context"
	"fmt"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// DefinitionRow is one lifecycle snapshot with the immutable definition
// revision it projects (.loom/42 E-1, the definition editor's inventory).
type DefinitionRow struct {
	Snapshot Snapshot
	Revision integration.IntegrationDefinitionRevision
}

// ListDefinitions returns, in one query, every snapshot of one tenant with
// its stored definition revision, ordered by definition and revision and
// bounded by limit. Retired revisions are included only when includeRetired
// is true. A stored revision that no longer decodes or verifies is
// ErrImmutableRecord, never silently skipped.
func (c *PostgresCatalog) ListDefinitions(ctx context.Context, tenantID string, includeRetired bool, limit int) ([]DefinitionRow, error) {
	if c == nil || c.db == nil || ctx == nil {
		return nil, ErrUnavailable
	}
	if !validIdentity(tenantID) || limit <= 0 || limit > 500 {
		return nil, ErrInvalidCommand
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT s.tenant_id, s.definition_id, s.revision_id, s.revision_digest, s.state, s.version,
			COALESCE(s.release_id, ''), s.health, COALESCE(s.last_validation_id, ''),
			s.validation_passed, s.validation_checked_at, s.validation_expires_at,
			COALESCE(s.approval_event_id, ''), s.updated_json, r.revision_json
		FROM integration_lifecycle_snapshots s
		JOIN integration_definition_revisions r
		  ON r.tenant_id = s.tenant_id AND r.definition_id = s.definition_id
		 AND r.revision_id = s.revision_id AND r.digest = s.revision_digest
		WHERE s.tenant_id = $1 AND ($2 OR s.state <> 'retired')
		ORDER BY s.definition_id, s.revision_id
		LIMIT $3
	`, tenantID, includeRetired, limit)
	if err != nil {
		return nil, fmt.Errorf("list lifecycle definitions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	definitions := make([]DefinitionRow, 0)
	for rows.Next() {
		var raw []byte
		snapshot, err := scanSnapshot(snapshotWithTrailer{rows: rows, trailer: &raw})
		if err != nil {
			return nil, fmt.Errorf("scan lifecycle definition: %w", err)
		}
		revision, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(raw))
		if err != nil || revision.Reference() != snapshot.DefinitionRevision {
			return nil, ErrImmutableRecord
		}
		definitions = append(definitions, DefinitionRow{Snapshot: snapshot, Revision: revision})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lifecycle definitions: %w", err)
	}
	return definitions, nil
}

// GetDefinition returns one snapshot with its verified definition revision.
func (c *PostgresCatalog) GetDefinition(ctx context.Context, tenantID, definitionID, revisionID string) (DefinitionRow, error) {
	snapshot, err := c.GetSnapshot(ctx, tenantID, definitionID, revisionID)
	if err != nil {
		return DefinitionRow{}, err
	}
	raw, err := c.LoadDefinitionRevision(ctx, tenantID, definitionID, revisionID)
	if err != nil {
		return DefinitionRow{}, err
	}
	revision, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(raw))
	if err != nil || revision.Reference() != snapshot.DefinitionRevision {
		return DefinitionRow{}, ErrImmutableRecord
	}
	return DefinitionRow{Snapshot: snapshot, Revision: revision}, nil
}

// snapshotWithTrailer scans the snapshot columns followed by one extra column.
type snapshotWithTrailer struct {
	rows    interface{ Scan(...any) error }
	trailer *[]byte
}

func (s snapshotWithTrailer) Scan(destinations ...any) error {
	return s.rows.Scan(append(destinations, s.trailer)...)
}
