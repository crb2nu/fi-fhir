package lifecycle

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const releaseDigestDomain = "fi-fhir/integration-release/v1\x00"

// ValidateConnection records bounded validation evidence and advances a draft on success.
func (c *PostgresCatalog) ValidateConnection(ctx context.Context, command Command) (Snapshot, error) {
	if c == nil || c.db == nil || c.clock == nil || c.validateConnection == nil || ctx == nil {
		return Snapshot{}, ErrUnavailable
	}
	now := c.clock().UTC()
	audit, err := command.audit(now)
	if err != nil {
		return Snapshot{}, err
	}
	revision, err := c.getRevision(ctx, command.TenantID, command.DefinitionID, command.RevisionID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot, err := c.GetSnapshot(ctx, command.TenantID, command.DefinitionID, command.RevisionID)
	if err != nil {
		return Snapshot{}, err
	}
	if snapshot.Version != command.ExpectedVersion {
		return Snapshot{}, ErrVersionConflict
	}
	if !validationAllowed(snapshot.State) {
		return Snapshot{}, ErrInvalidTransition
	}

	timeout := time.Duration(revision.Deployment.ConnectionValidation.TimeoutSeconds) * time.Second
	validationContext, cancel := context.WithTimeout(ctx, timeout)
	outcome, validationErr := c.validateConnection(validationContext, revision)
	cancel()
	if ctx.Err() != nil {
		return Snapshot{}, ctx.Err()
	}
	if validationErr != nil {
		outcome = ConnectionValidationOutcome{Codes: []string{"CONNECTION_CHECK_ERROR"}}
		if errors.Is(validationErr, context.DeadlineExceeded) {
			outcome.Codes[0] = "CONNECTION_CHECK_TIMEOUT"
		}
	}
	outcome, err = normalizeValidationOutcome(outcome)
	if err != nil {
		outcome = ConnectionValidationOutcome{Codes: []string{"VALIDATOR_INVALID_RESULT"}}
	}
	return c.recordValidation(ctx, command, audit, revision, outcome, now)
}

func (c *PostgresCatalog) recordValidation(
	ctx context.Context,
	command Command,
	audit integration.AuditEnvelope,
	revision integration.IntegrationDefinitionRevision,
	outcome ConnectionValidationOutcome,
	checkedAt time.Time,
) (Snapshot, error) {
	validationID, err := newRecordID("validation")
	if err != nil {
		return Snapshot{}, err
	}
	eventID, err := newRecordID("life")
	if err != nil {
		return Snapshot{}, err
	}
	expiresAt := checkedAt.Add(time.Duration(revision.Deployment.ConnectionValidation.MaxAgeSeconds) * time.Second)
	sourceJSON, _ := json.Marshal(revision.Source.ArtifactRevisionRef)
	codesJSON, _ := json.Marshal(outcome.Codes)
	auditJSON, _ := json.Marshal(audit)

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("begin connection validation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	snapshot, err := lockSnapshot(ctx, tx, command)
	if err != nil {
		return Snapshot{}, err
	}
	if !validationAllowed(snapshot.State) {
		return Snapshot{}, ErrInvalidTransition
	}
	target := snapshot.State
	if snapshot.State == integration.DeploymentStateDraft && outcome.Passed {
		target = integration.DeploymentStateValidated
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO integration_connection_validations (
			validation_id, tenant_id, definition_id, revision_id, revision_digest,
			source_revision, passed, codes, checked_at, expires_at, audit_json
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, validationID, command.TenantID, command.DefinitionID, command.RevisionID,
		revision.Digest, string(sourceJSON), outcome.Passed, string(codesJSON), checkedAt, expiresAt, string(auditJSON))
	if err != nil {
		return Snapshot{}, fmt.Errorf("record connection validation: %w", err)
	}
	nextVersion := snapshot.Version + 1
	result, err := tx.ExecContext(ctx, `
		UPDATE integration_lifecycle_snapshots
		SET state = $1, version = $2, last_validation_id = $3,
			validation_passed = $4, validation_checked_at = $5,
			validation_expires_at = $6, updated_json = $7, updated_at = $8
		WHERE tenant_id = $9 AND definition_id = $10 AND revision_id = $11 AND version = $12
	`, target, nextVersion, validationID, outcome.Passed, checkedAt, expiresAt,
		string(auditJSON), checkedAt, command.TenantID, command.DefinitionID,
		command.RevisionID, command.ExpectedVersion)
	if err != nil {
		return Snapshot{}, fmt.Errorf("advance validation snapshot: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return Snapshot{}, err
	}
	action := "validate_connection"
	if !outcome.Passed {
		action = "validate_connection_failed"
	}
	if err := insertEvent(ctx, tx, EventRecord{
		ID: eventID, TenantID: command.TenantID, DefinitionRevision: revision.Reference(),
		Version: nextVersion, Action: action, FromState: snapshot.State, ToState: target,
		Health: snapshot.Health, ReleaseID: snapshot.ReleaseID, Audit: audit,
	}); err != nil {
		return Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, fmt.Errorf("commit connection validation: %w", err)
	}
	snapshot.State = target
	snapshot.Version = nextVersion
	snapshot.LastValidationID = validationID
	snapshot.ValidationPassed = outcome.Passed
	snapshot.ValidationCheckedAt = checkedAt
	snapshot.ValidationExpiresAt = expiresAt
	snapshot.Updated = audit
	if !outcome.Passed {
		return cloneSnapshot(snapshot), ErrConnectionValidationFailed
	}
	return cloneSnapshot(snapshot), nil
}

// Approve advances a currently validated immutable revision.
func (c *PostgresCatalog) Approve(ctx context.Context, command Command) (Snapshot, error) {
	return c.changeState(ctx, command, transitionSpec{
		from: integration.DeploymentStateValidated, to: integration.DeploymentStateApproved,
		action: "approve", requireValidation: true,
	})
}

// Publish creates one immutable release record for an approved revision.
func (c *PostgresCatalog) Publish(ctx context.Context, command Command) (Snapshot, error) {
	return c.changeState(ctx, command, transitionSpec{
		from: integration.DeploymentStateApproved, to: integration.DeploymentStatePublished,
		action: "publish", requireValidation: true, createRelease: true,
	})
}

// Deploy makes a published release resolvable by a production adapter.
func (c *PostgresCatalog) Deploy(ctx context.Context, command Command) (Snapshot, error) {
	return c.changeState(ctx, command, transitionSpec{
		from: integration.DeploymentStatePublished, to: integration.DeploymentStateDeployed,
		action: "deploy", requireValidation: true, health: integration.DeploymentHealthStarting,
	})
}

// Pause removes a deployed release from runnable resolution without changing it.
func (c *PostgresCatalog) Pause(ctx context.Context, command Command) (Snapshot, error) {
	return c.changeState(ctx, command, transitionSpec{
		from: integration.DeploymentStateDeployed, to: integration.DeploymentStatePaused,
		action: "pause", health: integration.DeploymentHealthUnknown,
	})
}

// Resume returns a paused release to deployed after fresh validation evidence.
func (c *PostgresCatalog) Resume(ctx context.Context, command Command) (Snapshot, error) {
	return c.changeState(ctx, command, transitionSpec{
		from: integration.DeploymentStatePaused, to: integration.DeploymentStateDeployed,
		action: "resume", requireValidation: true, health: integration.DeploymentHealthStarting,
	})
}

// Retire permanently closes a published or active release.
func (c *PostgresCatalog) Retire(ctx context.Context, command Command) (Snapshot, error) {
	snapshot, err := c.GetSnapshot(ctx, command.TenantID, command.DefinitionID, command.RevisionID)
	if err != nil {
		return Snapshot{}, err
	}
	if snapshot.Version != command.ExpectedVersion {
		return Snapshot{}, ErrVersionConflict
	}
	if snapshot.State != integration.DeploymentStatePublished &&
		snapshot.State != integration.DeploymentStateDeployed &&
		snapshot.State != integration.DeploymentStatePaused {
		return Snapshot{}, ErrInvalidTransition
	}
	return c.changeState(ctx, command, transitionSpec{
		from: snapshot.State, to: integration.DeploymentStateRetired,
		action: "retire", health: integration.DeploymentHealthUnknown,
	})
}

type transitionSpec struct {
	from              integration.DeploymentState
	to                integration.DeploymentState
	action            string
	requireValidation bool
	createRelease     bool
	health            integration.DeploymentHealthStatus
}

func (c *PostgresCatalog) changeState(ctx context.Context, command Command, spec transitionSpec) (Snapshot, error) {
	if c == nil || c.db == nil || c.clock == nil || ctx == nil {
		return Snapshot{}, ErrUnavailable
	}
	now := c.clock().UTC()
	audit, err := command.audit(now)
	if err != nil {
		return Snapshot{}, err
	}
	if !allowedStateTransition(spec.from, spec.to) {
		return Snapshot{}, ErrInvalidTransition
	}
	eventID, err := newRecordID("life")
	if err != nil {
		return Snapshot{}, err
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("begin lifecycle transition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	snapshot, err := lockSnapshot(ctx, tx, command)
	if err != nil {
		return Snapshot{}, err
	}
	if snapshot.State != spec.from {
		return Snapshot{}, ErrInvalidTransition
	}
	if spec.requireValidation && !currentValidation(snapshot, now) {
		return Snapshot{}, ErrConnectionValidationRequired
	}

	releaseID := snapshot.ReleaseID
	if spec.createRelease {
		if snapshot.ApprovalEventID == "" || snapshot.LastValidationID == "" {
			return Snapshot{}, ErrInvalidTransition
		}
		release, releaseErr := newReleaseRecord(snapshot, audit)
		if releaseErr != nil {
			return Snapshot{}, releaseErr
		}
		if err := insertRelease(ctx, tx, release); err != nil {
			return Snapshot{}, err
		}
		releaseID = release.ID
	}
	health := spec.health
	if health == "" {
		health = snapshot.Health
	}
	nextVersion := snapshot.Version + 1
	approvalEventID := snapshot.ApprovalEventID
	if spec.to == integration.DeploymentStateApproved {
		approvalEventID = eventID
	}
	auditJSON, _ := json.Marshal(audit)
	result, err := tx.ExecContext(ctx, `
		UPDATE integration_lifecycle_snapshots
		SET state = $1, version = $2, release_id = NULLIF($3, ''), health = $4,
			approval_event_id = NULLIF($5, ''), updated_json = $6, updated_at = $7
		WHERE tenant_id = $8 AND definition_id = $9 AND revision_id = $10 AND version = $11
	`, spec.to, nextVersion, releaseID, health, approvalEventID, string(auditJSON), now,
		command.TenantID, command.DefinitionID, command.RevisionID, command.ExpectedVersion)
	if err != nil {
		return Snapshot{}, mapWriteError(err, ErrVersionConflict)
	}
	if err := requireOneRow(result); err != nil {
		return Snapshot{}, err
	}
	if err := insertEvent(ctx, tx, EventRecord{
		ID: eventID, TenantID: command.TenantID, DefinitionRevision: snapshot.DefinitionRevision,
		Version: nextVersion, Action: spec.action, FromState: snapshot.State, ToState: spec.to,
		Health: health, ReleaseID: releaseID, Audit: audit,
	}); err != nil {
		return Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, fmt.Errorf("commit lifecycle transition: %w", err)
	}
	snapshot.State = spec.to
	snapshot.Version = nextVersion
	snapshot.ReleaseID = releaseID
	snapshot.Health = health
	snapshot.ApprovalEventID = approvalEventID
	snapshot.Updated = audit
	return cloneSnapshot(snapshot), nil
}

func validationAllowed(state integration.DeploymentState) bool {
	switch state {
	case integration.DeploymentStateDraft, integration.DeploymentStateValidated,
		integration.DeploymentStateApproved, integration.DeploymentStatePublished,
		integration.DeploymentStatePaused:
		return true
	default:
		return false
	}
}

func currentValidation(snapshot Snapshot, now time.Time) bool {
	return snapshot.ValidationPassed && snapshot.LastValidationID != "" && snapshot.ValidationExpiresAt.After(now)
}

func lockSnapshot(ctx context.Context, tx *sql.Tx, command Command) (Snapshot, error) {
	row := tx.QueryRowContext(ctx, snapshotSelect+`
		WHERE tenant_id = $1 AND definition_id = $2 AND revision_id = $3
		FOR UPDATE
	`, command.TenantID, command.DefinitionID, command.RevisionID)
	snapshot, err := scanSnapshot(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, ErrNotFound
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("lock lifecycle snapshot: %w", err)
	}
	if snapshot.Version != command.ExpectedVersion {
		return Snapshot{}, ErrVersionConflict
	}
	return snapshot, nil
}

func requireOneRow(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read lifecycle write count: %w", err)
	}
	if count != 1 {
		return ErrVersionConflict
	}
	return nil
}

func insertEvent(ctx context.Context, tx *sql.Tx, event EventRecord) error {
	auditJSON, err := json.Marshal(event.Audit)
	if err != nil {
		return fmt.Errorf("marshal lifecycle event audit: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO integration_lifecycle_events (
			event_id, tenant_id, definition_id, revision_id, revision_digest,
			version, action, from_state, to_state, health, release_id, audit_json, occurred_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10, NULLIF($11, ''), $12, $13)
	`, event.ID, event.TenantID, event.DefinitionRevision.ArtifactID,
		event.DefinitionRevision.RevisionID, event.DefinitionRevision.Digest,
		event.Version, event.Action, event.FromState, event.ToState, event.Health,
		event.ReleaseID, string(auditJSON), event.Audit.OccurredAt)
	if err != nil {
		return fmt.Errorf("record lifecycle event: %w", err)
	}
	return nil
}

func newReleaseRecord(snapshot Snapshot, published integration.AuditEnvelope) (ReleaseRecord, error) {
	id, err := newRecordID("release")
	if err != nil {
		return ReleaseRecord{}, err
	}
	published.Principal = clonePrincipal(published.Principal)
	sort.Strings(published.Principal.Roles)
	release := ReleaseRecord{
		ID: id, TenantID: snapshot.TenantID, DefinitionRevision: snapshot.DefinitionRevision,
		ValidationID: snapshot.LastValidationID, ApprovalEventID: snapshot.ApprovalEventID,
		Published: published,
	}
	digest, err := releaseRecordDigest(release)
	if err != nil {
		return ReleaseRecord{}, err
	}
	release.Digest = digest
	return release, nil
}

func releaseRecordDigest(release ReleaseRecord) (string, error) {
	encoded, err := json.Marshal(struct {
		ID                 string                          `json:"id"`
		TenantID           string                          `json:"tenant_id"`
		DefinitionRevision integration.ArtifactRevisionRef `json:"definition_revision"`
		ValidationID       string                          `json:"validation_id"`
		ApprovalEventID    string                          `json:"approval_event_id"`
		Published          integration.AuditEnvelope       `json:"published"`
	}{release.ID, release.TenantID, release.DefinitionRevision, release.ValidationID, release.ApprovalEventID, release.Published})
	if err != nil {
		return "", fmt.Errorf("marshal release digest: %w", err)
	}
	sum := sha256.Sum256(append([]byte(releaseDigestDomain), encoded...))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func insertRelease(ctx context.Context, tx *sql.Tx, release ReleaseRecord) error {
	publishedJSON, err := json.Marshal(release.Published)
	if err != nil {
		return fmt.Errorf("marshal release publication: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO integration_release_records (
			release_id, tenant_id, definition_id, revision_id, revision_digest,
			validation_id, approval_event_id, published_json, published_at, release_digest
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, release.ID, release.TenantID, release.DefinitionRevision.ArtifactID,
		release.DefinitionRevision.RevisionID, release.DefinitionRevision.Digest,
		release.ValidationID, release.ApprovalEventID, string(publishedJSON),
		release.Published.OccurredAt, release.Digest)
	if err != nil {
		return fmt.Errorf("record immutable release: %w", err)
	}
	return nil
}
