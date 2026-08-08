package lifecycle

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// GetValidation returns one immutable connection-validation record.
func (c *PostgresCatalog) GetValidation(ctx context.Context, validationID string) (ValidationRecord, error) {
	if c == nil || c.db == nil || ctx == nil {
		return ValidationRecord{}, ErrUnavailable
	}
	var record ValidationRecord
	var definitionID, revisionID, digest string
	var sourceJSON, codesJSON, auditJSON []byte
	err := c.db.QueryRowContext(ctx, `
		SELECT validation_id, tenant_id, definition_id, revision_id, revision_digest,
			source_revision, passed, codes, checked_at, expires_at, audit_json
		FROM integration_connection_validations WHERE validation_id = $1
	`, validationID).Scan(
		&record.ID, &record.TenantID, &definitionID, &revisionID, &digest,
		&sourceJSON, &record.Passed, &codesJSON, &record.CheckedAt,
		&record.ExpiresAt, &auditJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ValidationRecord{}, ErrNotFound
	}
	if err != nil {
		return ValidationRecord{}, fmt.Errorf("load validation record: %w", err)
	}
	record.DefinitionRevision = integration.ArtifactRevisionRef{ArtifactID: definitionID, RevisionID: revisionID, Digest: digest}
	if json.Unmarshal(sourceJSON, &record.SourceRevision) != nil ||
		json.Unmarshal(codesJSON, &record.Codes) != nil ||
		json.Unmarshal(auditJSON, &record.Audit) != nil {
		return ValidationRecord{}, ErrImmutableRecord
	}
	record.CheckedAt = record.CheckedAt.UTC()
	record.ExpiresAt = record.ExpiresAt.UTC()
	record.Codes = append([]string(nil), record.Codes...)
	return record, nil
}

// GetRelease returns an immutable, digest-verified publication record.
func (c *PostgresCatalog) GetRelease(ctx context.Context, releaseID string) (ReleaseRecord, error) {
	if c == nil || c.db == nil || ctx == nil {
		return ReleaseRecord{}, ErrUnavailable
	}
	var release ReleaseRecord
	var definitionID, revisionID, digest string
	var publishedJSON []byte
	err := c.db.QueryRowContext(ctx, `
		SELECT release_id, tenant_id, definition_id, revision_id, revision_digest,
			validation_id, approval_event_id, published_json, release_digest
		FROM integration_release_records WHERE release_id = $1
	`, releaseID).Scan(
		&release.ID, &release.TenantID, &definitionID, &revisionID, &digest,
		&release.ValidationID, &release.ApprovalEventID, &publishedJSON, &release.Digest,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ReleaseRecord{}, ErrNotFound
	}
	if err != nil {
		return ReleaseRecord{}, fmt.Errorf("load release record: %w", err)
	}
	release.DefinitionRevision = integration.ArtifactRevisionRef{ArtifactID: definitionID, RevisionID: revisionID, Digest: digest}
	if err := json.Unmarshal(publishedJSON, &release.Published); err != nil {
		return ReleaseRecord{}, ErrImmutableRecord
	}
	expected, err := releaseRecordDigest(release)
	if err != nil || release.Digest != expected {
		return ReleaseRecord{}, ErrImmutableRecord
	}
	return release, nil
}

// ListSnapshots returns the bounded current lifecycle projection for one
// tenant. It is the operator control plane's deployment/channel inventory and
// never returns another tenant's rows.
func (c *PostgresCatalog) ListSnapshots(ctx context.Context, tenantID string, limit int) ([]Snapshot, error) {
	if c == nil || c.db == nil || ctx == nil {
		return nil, ErrUnavailable
	}
	if !validIdentity(tenantID) || limit <= 0 || limit > 500 {
		return nil, ErrInvalidCommand
	}
	rows, err := c.db.QueryContext(ctx, snapshotSelect+`
		WHERE tenant_id = $1
		ORDER BY definition_id, revision_id
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list lifecycle snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()
	snapshots := make([]Snapshot, 0)
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("scan lifecycle snapshot: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lifecycle snapshots: %w", err)
	}
	return snapshots, nil
}

// ListEvents returns the append-only lifecycle history in snapshot-version order.
func (c *PostgresCatalog) ListEvents(ctx context.Context, tenantID, definitionID, revisionID string) ([]EventRecord, error) {
	if c == nil || c.db == nil || ctx == nil {
		return nil, ErrUnavailable
	}
	rows, err := c.db.QueryContext(ctx, `
		SELECT event_id, tenant_id, definition_id, revision_id, revision_digest,
			version, action, COALESCE(from_state, ''), to_state, health,
			COALESCE(release_id, ''), audit_json
		FROM integration_lifecycle_events
		WHERE tenant_id = $1 AND definition_id = $2 AND revision_id = $3
		ORDER BY version
	`, tenantID, definitionID, revisionID)
	if err != nil {
		return nil, fmt.Errorf("list lifecycle events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	events := make([]EventRecord, 0)
	for rows.Next() {
		var event EventRecord
		var eventDefinitionID, eventRevisionID, digest string
		var auditJSON []byte
		if err := rows.Scan(
			&event.ID, &event.TenantID, &eventDefinitionID, &eventRevisionID, &digest,
			&event.Version, &event.Action, &event.FromState, &event.ToState,
			&event.Health, &event.ReleaseID, &auditJSON,
		); err != nil {
			return nil, fmt.Errorf("scan lifecycle event: %w", err)
		}
		event.DefinitionRevision = integration.ArtifactRevisionRef{ArtifactID: eventDefinitionID, RevisionID: eventRevisionID, Digest: digest}
		if err := json.Unmarshal(auditJSON, &event.Audit); err != nil {
			return nil, ErrImmutableRecord
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lifecycle events: %w", err)
	}
	return events, nil
}

// ResolveRunnable returns the exact revision only while its release is deployed.
func (c *PostgresCatalog) ResolveRunnable(ctx context.Context, tenantID, definitionID string) (RunnableBinding, error) {
	if c == nil || c.db == nil || ctx == nil {
		return RunnableBinding{}, ErrUnavailable
	}
	var binding RunnableBinding
	var raw []byte
	err := c.db.QueryRowContext(ctx, `
		SELECT s.release_id, s.version, s.health, r.revision_json
		FROM integration_lifecycle_snapshots s
		JOIN integration_definition_revisions r
		  ON r.tenant_id = s.tenant_id
		 AND r.definition_id = s.definition_id
		 AND r.revision_id = s.revision_id
		 AND r.digest = s.revision_digest
		WHERE s.tenant_id = $1 AND s.definition_id = $2 AND s.state = 'deployed'
	`, tenantID, definitionID).Scan(&binding.ReleaseID, &binding.SnapshotVersion, &binding.Health, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return RunnableBinding{}, ErrNotFound
	}
	if err != nil {
		return RunnableBinding{}, fmt.Errorf("resolve runnable integration: %w", err)
	}
	revision, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(raw))
	if err != nil || revision.ValidateForDeployment() != nil || revision.TenantID != tenantID || revision.DefinitionID != definitionID {
		return RunnableBinding{}, ErrImmutableRecord
	}
	release, err := c.GetRelease(ctx, binding.ReleaseID)
	if err != nil || release.DefinitionRevision != revision.Reference() {
		return RunnableBinding{}, ErrImmutableRecord
	}
	binding.IntegrationRevision = revision.Reference()
	binding.SourceRevision = revision.Source.ArtifactRevisionRef
	binding.SourceID = revision.Source.SourceID
	binding.Format = revision.Format
	binding.Classification = revision.Policy.Classification
	binding.Deployment = *revision.Deployment
	binding.SecretBindings = append([]integration.SecretBinding(nil), revision.SecretBindings...)
	return binding, nil
}

// ReportHealth records a safe status projection while a release is deployed.
func (c *PostgresCatalog) ReportHealth(ctx context.Context, command Command, health integration.DeploymentHealthStatus) (Snapshot, error) {
	if health != integration.DeploymentHealthStarting &&
		health != integration.DeploymentHealthHealthy &&
		health != integration.DeploymentHealthDegraded &&
		health != integration.DeploymentHealthUnhealthy {
		return Snapshot{}, ErrInvalidCommand
	}
	if c == nil || c.db == nil || c.clock == nil || ctx == nil {
		return Snapshot{}, ErrUnavailable
	}
	now := c.clock().UTC()
	audit, err := command.audit(now)
	if err != nil {
		return Snapshot{}, err
	}
	eventID, err := newRecordID("life")
	if err != nil {
		return Snapshot{}, err
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("begin health report: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	snapshot, err := lockSnapshot(ctx, tx, command)
	if err != nil {
		return Snapshot{}, err
	}
	if snapshot.State != integration.DeploymentStateDeployed || snapshot.ReleaseID == "" {
		return Snapshot{}, ErrInvalidTransition
	}
	nextVersion := snapshot.Version + 1
	auditJSON, _ := json.Marshal(audit)
	result, err := tx.ExecContext(ctx, `
		UPDATE integration_lifecycle_snapshots
		SET version = $1, health = $2, updated_json = $3, updated_at = $4
		WHERE tenant_id = $5 AND definition_id = $6 AND revision_id = $7 AND version = $8
	`, nextVersion, health, string(auditJSON), now, command.TenantID,
		command.DefinitionID, command.RevisionID, command.ExpectedVersion)
	if err != nil {
		return Snapshot{}, fmt.Errorf("update deployment health: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return Snapshot{}, err
	}
	if err := insertEvent(ctx, tx, EventRecord{
		ID: eventID, TenantID: command.TenantID, DefinitionRevision: snapshot.DefinitionRevision,
		Version: nextVersion, Action: "report_health", FromState: snapshot.State,
		ToState: snapshot.State, Health: health, ReleaseID: snapshot.ReleaseID, Audit: audit,
	}); err != nil {
		return Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, fmt.Errorf("commit health report: %w", err)
	}
	snapshot.Version = nextVersion
	snapshot.Health = health
	snapshot.Updated = audit
	return cloneSnapshot(snapshot), nil
}
