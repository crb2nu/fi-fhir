package lifecycle

import (
	"context"
	"database/sql"
	"errors"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// AuthorizeRunnableSubmission serializes durable admission with pause and
// retirement. The shared row lock remains held by the caller's transaction
// through commit. Admission therefore linearizes before a concurrent stop, or
// fails closed after a completed lifecycle stop transition.
func (c *PostgresCatalog) AuthorizeRunnableSubmission(
	ctx context.Context,
	tx *sql.Tx,
	request integration.ProcessRequest,
	revision integration.IntegrationDefinitionRevision,
) error {
	if c == nil || c.db == nil || ctx == nil || tx == nil {
		return ErrUnavailable
	}
	if request.Security.TenantID != revision.TenantID ||
		request.IntegrationRevision != revision.Reference() {
		return ErrNotRunnable
	}
	if err := authorization.AuthorizeSubmission(
		request.Security,
		revision.TenantID,
		revision.Reference(),
		revision.Source.SourceID,
	); err != nil {
		return ErrNotRunnable
	}

	var releaseID string
	err := tx.QueryRowContext(ctx, `
		SELECT s.release_id
		FROM integration_lifecycle_snapshots s
		JOIN integration_release_records r
		  ON r.release_id = s.release_id
		 AND r.tenant_id = s.tenant_id
		 AND r.definition_id = s.definition_id
		 AND r.revision_id = s.revision_id
		 AND r.revision_digest = s.revision_digest
		WHERE s.tenant_id = $1
		  AND s.definition_id = $2
		  AND s.revision_id = $3
		  AND s.revision_digest = $4
		  AND s.state = 'deployed'
		FOR SHARE OF s
	`, revision.TenantID, revision.DefinitionID, revision.RevisionID, revision.Digest).Scan(&releaseID)
	if errors.Is(err, sql.ErrNoRows) || releaseID == "" {
		return ErrNotRunnable
	}
	if err != nil {
		return err
	}
	return nil
}
